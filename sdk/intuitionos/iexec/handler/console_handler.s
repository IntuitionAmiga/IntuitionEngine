prog_console:
    ; Header
    dc.l    0, 0
    dc.l    prog_console_code_end - prog_console_code   ; code_size
    dc.l    prog_console_data_end - prog_console_data   ; data_size
    dc.l    0                           ; flags
    ds.b    12                          ; reserved
prog_console_code:
    ; === Preamble: compute data page base (preemption-safe) ===
    sub     sp, sp, #16                 ; reserve [sp]=R29, [sp+8]=scratch
.con_preamble:
    load.q  r30, (sp)                  ; r30 = startup_base
    load.q  r29, 8(sp)                 ; r29 = data_base
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)                ; data[128] = task_id

    ; === Map terminal MMIO page (M11.5: console.handler now owns terminal I/O) ===
    ; SYS_MAP_IO(0xF0, 1) maps physical page 0xF0 (TERM_*, SCAN_*, MOUSE_*, video MMIO)
    ; into our user address space. The returned VA is cached at data[144] and is the
    ; base for the inlined readline loop in .con_no_msg below. M11.5 removed the
    ; legacy SYS_READ_INPUT kernel helper; line input is now read from MMIO directly
    ; here in user mode.
    move.l  r1, #0xF0                   ; R1 = base PPN
    move.l  r2, #1                      ; R2 = page count
    syscall #SYS_MAP_IO                 ; R1 = mapped VA, R2 = err
    load.q  r29, (sp)
    bnez    r2, .con_mapio_failed       ; non-zero err is unrecoverable
    store.q r1, 144(r29)                ; data[144] = term_io_va
    bra     .con_after_mapio
.con_mapio_failed:
    ; Cannot recover — exit task. Subsequent FindPort retries by clients will
    ; eventually fail when no console.handler port exists.
    syscall #SYS_EXIT_TASK
.con_after_mapio:

    ; === Create "console.handler" port ===
    load.q  r29, (sp)
    move.q  r1, r29                     ; R1 = name_ptr (data[0])
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT            ; R1 = port_id
    load.q  r29, (sp)
    store.q r1, 136(r29)                ; data[136] = console_port

    ; === Clear readline_pending flag ===
    store.b r0, 176(r29)

    ; === Print "console.handler ONLINE [Taskn]\r\n" via DebugPutChar ===
    add     r20, r29, #16               ; r20 = &data[16] = banner string
.con_banner_loop:
    load.b  r1, (r20)
    beqz    r1, .con_banner_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .con_banner_loop
.con_banner_id:
    load.q  r29, (sp)
    load.q  r1, 128(r29)               ; task_id
    add     r1, r1, #0x30              ; ASCII '0'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x5D                   ; ']'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0D                   ; '\r'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0A                   ; '\n'
    syscall #SYS_DEBUG_PUTCHAR

    ; === Main polling loop ===
.con_poll_loop:
    ; --- Try to get a message (non-blocking) ---
    load.q  r29, (sp)
    load.q  r1, 136(r29)               ; console_port
    syscall #SYS_GET_MSG                ; R1=type, R2=data0, R3=err, R4=data1, R5=reply_port, R6=share_handle
    load.q  r29, (sp)
    bnez    r3, .con_no_msg             ; R3 != ERR_OK → no message (ERR_AGAIN)

    ; --- Got a message. Dispatch on type. ---
    ; R1=type, R2=data0, R5=reply_port, R6=share_handle
    beqz    r1, .con_print_char         ; type 0 = CON_MSG_CHAR

    move.l  r11, #CON_MSG_READLINE
    beq     r1, r11, .con_readline_req
    move.l  r11, #MSG_GET_IOSM
    beq     r1, r11, .con_get_iosm

    ; Unknown type — ignore, loop back
    bra     .con_poll_loop

    ; --- CON_MSG_CHAR: print data0 low byte ---
.con_print_char:
    move.q  r1, r2                      ; char from data0
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    bra     .con_poll_loop

    ; --- CON_MSG_READLINE request ---
.con_readline_req:
    ; Save reply_port and share_handle before checking pending flag
    load.q  r29, (sp)
    store.q r5, 8(sp)                   ; save reply_port to stack scratch
    store.l r6, 160(r29)                ; save share_handle to data[160]

    ; Check if readline is already pending
    load.b  r20, 176(r29)              ; readline_pending
    bnez    r20, .con_readline_busy     ; already pending → reject

    ; Accept readline request
    load.q  r5, 8(sp)                  ; restore reply_port
    store.q r5, 152(r29)               ; data[152] = readline_reply_port
    ; share_handle already saved at data[160]

    ; MapShared on first use (check if cached VA is 0)
    load.q  r20, 168(r29)             ; readline_mapped_va
    bnez    r20, .con_readline_mapped  ; already mapped

    ; First time: MapShared to get VA
    load.l  r1, 160(r29)              ; share_handle
    move.l  r2, #(MAPF_READ | MAPF_WRITE)
    syscall #SYS_MAP_SHARED            ; R1 = VA, R2 = err
    load.q  r29, (sp)
    beqz    r1, .con_poll_loop         ; MapShared failed, drop request
    store.q r1, 168(r29)              ; cache mapped VA

.con_readline_mapped:
    ; Set readline_pending = 1
    move.b  r20, #1
    store.b r20, 176(r29)
    bra     .con_poll_loop

    ; --- Reject: readline already pending → reply with ERR_AGAIN ---
.con_readline_busy:
    load.q  r5, 8(sp)                 ; reply_port from stack
    move.q  r1, r5                     ; R1 = reply_port_id
    move.l  r2, #ERR_AGAIN             ; R2 = type = error code
    move.q  r3, r0                     ; R3 = data0 = 0
    move.q  r4, r0                     ; R4 = data1 = 0
    move.q  r5, r0                     ; R5 = share_handle = 0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .con_poll_loop

.con_get_iosm:
    move.q  r25, r5
    store.q r5, 8(sp)
    beqz    r6, .con_get_iosm_badarg
    move.q  r1, r6
    move.l  r2, #MAPF_WRITE
    syscall #SYS_MAP_SHARED
    load.q  r29, (sp)
    bnez    r2, .con_get_iosm_maperr
    move.q  r23, r1
    move.q  r24, r1
    move.l  r11, #1
    bne     r3, r11, .con_get_iosm_badarg_free
    add     r14, r29, #(prog_console_iosm - prog_console_data)
    move.l  r15, #(IOSM_SIZE / 8)
.con_get_iosm_copy:
    load.q  r16, (r14)
    store.q r16, (r24)
    add     r14, r14, #8
    add     r24, r24, #8
    sub     r15, r15, #1
    bnez    r15, .con_get_iosm_copy
    move.q  r1, r23
    move.l  r2, #IOSM_SIZE
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
    load.q  r1, 8(sp)
    move.q  r2, r0
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .con_poll_loop
.con_get_iosm_badarg_free:
    move.q  r1, r23
    move.q  r2, r3
    lsl     r2, r2, #12
    syscall #SYS_FREE_MEM
    load.q  r29, (sp)
.con_get_iosm_badarg:
    load.q  r1, 8(sp)
    move.l  r2, #ERR_BADARG
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .con_poll_loop
.con_get_iosm_maperr:
    load.q  r1, 8(sp)
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    load.q  r29, (sp)
    bra     .con_poll_loop

    ; --- No message: check if readline pending + keyboard ready ---
.con_no_msg:
    load.b  r20, 176(r29)             ; readline_pending
    beqz    r20, .con_yield            ; not pending → just yield

    ; M11.5: Inline terminal MMIO read loop (formerly SYS_READ_INPUT in kernel).
    ; Reads from page 0xF0 via the cached VA at data[144]. Layout within the
    ; mapped page (matches the physical TERM_* register addresses):
    ;   +0x70C TERM_LINE_STATUS   (bit 0 = complete line ready)
    ;   +0x704 TERM_STATUS        (bit 0 = char available)
    ;   +0x708 TERM_IN            (read dequeues one char)
    ; Destination is the readline_mapped_va shared buffer at data[168].
    load.q  r28, 144(r29)             ; r28 = term_io_va (page 0xF0 base)
    load.q  r20, 168(r29)             ; r20 = dest buffer VA
    move.l  r21, #126                 ; r21 = max_len

    ; Check TERM_LINE_STATUS — bail with ERR_AGAIN if no complete line yet
    add     r24, r28, #0x70C           ; r24 = &TERM_LINE_STATUS
    load.l  r14, (r24)
    and     r14, r14, #1
    beqz    r14, .con_yield            ; no complete line — yield and retry

    ; Line is ready — drain TERM_IN until \n or buffer full
    add     r23, r28, #0x708           ; r23 = &TERM_IN
    add     r24, r28, #0x704           ; r24 = &TERM_STATUS
    move.l  r22, #0                    ; r22 = byte count

.con_ri_loop:
    bge     r22, r21, .con_ri_done    ; max_len reached

    load.l  r14, (r24)                ; TERM_STATUS
    and     r14, r14, #1
    beqz    r14, .con_ri_done          ; no more chars (shouldn't happen mid-line)

    load.l  r14, (r23)                ; dequeue char from TERM_IN

    move.l  r15, #0x0D                 ; '\r' — skip
    beq     r14, r15, .con_ri_loop

    move.l  r15, #0x0A                 ; '\n' — end of line
    beq     r14, r15, .con_ri_done

    add     r15, r20, r22              ; dest = buffer + count
    store.b r14, (r15)
    add     r22, r22, #1
    bra     .con_ri_loop

.con_ri_done:
    ; Null-terminate
    add     r15, r20, r22
    store.b r0, (r15)

    ; Restore data page pointer (r29) and use byte count from r22
    load.q  r29, (sp)
    move.q  r25, r22                   ; save byte count

    ; Reply to readline requester
    load.q  r1, 152(r29)              ; readline_reply_port
    move.l  r2, #0                     ; type = 0 (success)
    move.q  r3, r25                    ; data0 = byte count
    move.q  r4, r0                     ; data1 = 0
    move.q  r5, r0                     ; share_handle = 0
    syscall #SYS_REPLY_MSG

    ; Clear readline_pending
    load.q  r29, (sp)
    store.b r0, 176(r29)
    bra     .con_poll_loop

.con_yield:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .con_poll_loop

prog_console_code_end:

prog_console_data:
    dc.b    "console.handler", 0       ; offset 0: port name (16 bytes exactly)
    dc.b    "console.handler M11.5 [Task ", 0  ; offset 16: banner string
    ; offset 128+ is scratch (task_id, port_id, etc.) — zeroed by loader
    align   8
prog_console_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    "console.handler", 0
    ds.b    IOSM_NAME_SIZE - 16
    dc.w    1
    dc.w    0
    dc.l    IOSM_KIND_HANDLER
    dc.l    MODF_COMPAT_PORT
    dc.l    0
    ds.b    IOSM_SIZE - 56
prog_console_data_end:
    align   8
prog_console_end:
