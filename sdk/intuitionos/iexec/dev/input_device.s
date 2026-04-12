prog_input_device:
    dc.l    0, 0
    dc.l    prog_input_device_code_end - prog_input_device_code
    dc.l    prog_input_device_data_end - prog_input_device_data
    dc.l    0
    ds.b    12
prog_input_device_code:

    ; ===== Preamble: compute data page base (preempt-safe) =====
    sub     sp, sp, #16
.idev_preamble:
    load.q  r30, (sp)                  ; r30 = startup_base
    load.q  r29, 8(sp)                 ; r29 = data_base
    store.q r29, (sp)
    load.l  r1, TASKSB_TASK_ID(r30)
    store.q r1, 128(r29)               ; data[128] = task_id

    ; ===== M12.5: Request CHIP grant from hardware.resource =====
    ; SYS_MAP_IO is now gated by the kernel grant table; we must hold a
    ; grant for PPN 0xF0 before calling it. The broker is the only producer
    ; of grants for a non-bootstrap task. Spin on FindPort until the broker
    ; is up (boot launch order in S/Startup-Sequence puts hardware.resource
    ; first, but we still poll to be safe across launch-order edits).
.idev_find_hwres:
    add     r1, r29, #192              ; r1 = "hardware.resource" string
    syscall #SYS_FIND_PORT             ; R1=port_id, R2=err
    bnez    r2, .idev_hwres_retry
    load.q  r29, (sp)
    bra     .idev_have_hwres
.idev_hwres_retry:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .idev_find_hwres
.idev_have_hwres:
    store.q r1, 224(r29)               ; data[224] = hwres_port

    ; Create anonymous reply port
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .idev_halt
    store.q r1, 232(r29)               ; data[232] = reply_port

    ; PutMsg(hwres_port, HWRES_MSG_REQUEST, tag=CHIP, data1=task_id, reply=reply_port)
    load.q  r1, 224(r29)               ; hwres_port
    move.l  r2, #HWRES_MSG_REQUEST
    move.l  r3, #HWRES_TAG_CHIP        ; data0 = tag
    load.q  r4, 128(r29)               ; data1 = my task_id
    load.q  r5, 232(r29)               ; reply_port
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .idev_halt

    ; Wait for reply (WaitPort returns message data directly)
    load.q  r1, 232(r29)
    syscall #SYS_WAIT_PORT             ; R1=type, R2=data0, R3=err
    load.q  r29, (sp)
    bnez    r3, .idev_halt
    move.l  r28, #HWRES_MSG_GRANTED
    bne     r1, r28, .idev_halt        ; broker denied → halt

    ; ===== SYS_MAP_IO(R1=0xF0, R2=1) =====
    ; Now we hold a CHIP grant; the kernel grant check will succeed.
    move.l  r1, #TERM_IO_PAGE
    move.l  r2, #1
    syscall #SYS_MAP_IO
    load.q  r29, (sp)
    bnez    r2, .idev_halt
    store.q r1, 152(r29)               ; data[152] = chip_mmio_va

    ; ===== CreatePort("input.device") =====
    add     r1, r29, #16
    move.l  r2, #PF_PUBLIC
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .idev_halt
    store.q r1, 144(r29)               ; data[144] = input_port

    ; ===== Print banner via SYS_DEBUG_PUTCHAR =====
    add     r20, r29, #32              ; r20 = &data[32] = banner
.idev_ban_loop:
    load.b  r1, (r20)
    beqz    r1, .idev_ban_id
    store.q r20, 8(sp)
    syscall #SYS_DEBUG_PUTCHAR
    load.q  r29, (sp)
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .idev_ban_loop
.idev_ban_id:
    load.q  r29, (sp)
    load.q  r1, 128(r29)
    add     r1, r1, #0x30              ; '0' + task_id
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x5D                  ; ']'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0D
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0x0A
    syscall #SYS_DEBUG_PUTCHAR

    ; ===== Main loop =====
.idev_main:
    load.q  r29, (sp)

    ; --- Try to get a message (non-blocking) ---
    load.q  r1, 144(r29)               ; input_port
    syscall #SYS_GET_MSG               ; R1=type R2=data0 R3=err R4=data1 R5=reply R6=share
    bnez    r3, .idev_poll             ; ERR_AGAIN → no msg

    ; --- Got message: dispatch ---
    move.q  r24, r2                    ; r24 = data0 (subscriber port for OPEN)
    move.q  r25, r5                    ; r25 = reply_port

    move.l  r28, #INPUT_OPEN
    beq     r1, r28, .idev_do_open
    move.l  r28, #INPUT_CLOSE
    beq     r1, r28, .idev_do_close
    bra     .idev_main                 ; unknown opcode, drop

.idev_do_open:
    load.q  r29, (sp)
    load.q  r14, 160(r29)              ; current subscriber
    bnez    r14, .idev_open_busy
    store.q r24, 160(r29)              ; subscriber = data0
    move.q  r1, r25                    ; reply_port
    move.l  r2, #INPUT_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .idev_main

.idev_open_busy:
    move.q  r1, r25
    move.l  r2, #INPUT_ERR_BUSY
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .idev_main

.idev_do_close:
    load.q  r29, (sp)
    store.q r0, 160(r29)               ; clear subscriber
    move.q  r1, r25
    move.l  r2, #INPUT_ERR_OK
    move.q  r3, r0
    move.q  r4, r0
    move.q  r5, r0
    syscall #SYS_REPLY_MSG
    bra     .idev_main

    ; --- No message: poll input registers ---
.idev_poll:
    load.q  r29, (sp)
    load.q  r24, 160(r29)              ; subscriber
    beqz    r24, .idev_yield           ; no subscriber → just yield
    load.q  r25, 152(r29)              ; chip_mmio_va

    ; --- Drain keyboard scancodes ---
.idev_kbd_drain:
    add     r14, r25, #0x744           ; SCAN_STATUS
    load.l  r14, (r14)
    and     r14, r14, #1
    beqz    r14, .idev_kbd_done
    add     r15, r25, #0x740           ; SCAN_CODE (read auto-dequeues)
    load.l  r15, (r15)
    add     r16, r25, #0x748           ; SCAN_MODIFIERS
    load.l  r16, (r16)

    ; Build event word: (IE_KEY_DOWN<<24) | (scancode<<16) | (modifiers<<8)
    move.l  r17, #IE_KEY_DOWN
    lsl     r17, r17, #24
    and     r15, r15, #0xFF
    lsl     r15, r15, #16
    or      r17, r17, r15
    and     r16, r16, #0xFF
    lsl     r16, r16, #8
    or      r17, r17, r16

    ; Build mn_Data1: (mx16<<48) | (my16<<32) | event_seq32
    add     r18, r25, #0x730           ; MOUSE_X
    load.l  r18, (r18)
    and     r18, r18, #0xFFFF
    lsl     r18, r18, #48
    add     r19, r25, #0x734           ; MOUSE_Y
    load.l  r19, (r19)
    and     r19, r19, #0xFFFF
    lsl     r19, r19, #32
    or      r18, r18, r19
    load.l  r19, 180(r29)
    add     r19, r19, #1
    store.l r19, 180(r29)
    or      r18, r18, r19

    ; PutMsg(subscriber, INPUT_EVENT, r17, r18, NONE, 0)
    move.q  r1, r24
    move.l  r2, #INPUT_EVENT
    move.q  r3, r17
    move.q  r4, r18
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r24, 160(r29)              ; reload subscriber
    beqz    r24, .idev_yield
    load.q  r25, 152(r29)
    bra     .idev_kbd_drain

.idev_kbd_done:
    ; --- Mouse: check status (reading clears change flag) ---
    add     r14, r25, #0x73C           ; MOUSE_STATUS
    load.l  r14, (r14)
    and     r14, r14, #1
    beqz    r14, .idev_yield

    add     r15, r25, #0x730           ; MOUSE_X
    load.l  r15, (r15)
    and     r15, r15, #0xFFFF
    add     r16, r25, #0x734           ; MOUSE_Y
    load.l  r16, (r16)
    and     r16, r16, #0xFFFF
    add     r17, r25, #0x738           ; MOUSE_BUTTONS
    load.l  r17, (r17)
    and     r17, r17, #0xFF

    load.l  r18, 168(r29)              ; last_mouse_x
    load.l  r19, 172(r29)              ; last_mouse_y
    load.l  r20, 176(r29)              ; last_mouse_buttons

    ; --- Position changed? Emit IE_MOUSE_MOVE ---
    bne     r15, r18, .idev_mv_emit
    bne     r16, r19, .idev_mv_emit
    bra     .idev_mv_check_btn

.idev_mv_emit:
    move.l  r21, #IE_MOUSE_MOVE
    lsl     r21, r21, #24
    move.q  r22, r15
    lsl     r22, r22, #48
    move.q  r23, r16
    lsl     r23, r23, #32
    or      r22, r22, r23
    load.l  r19, 180(r29)
    add     r19, r19, #1
    store.l r19, 180(r29)
    or      r22, r22, r19
    move.q  r1, r24
    move.l  r2, #INPUT_EVENT
    move.q  r3, r21
    move.q  r4, r22
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r24, 160(r29)
    beqz    r24, .idev_save_state
    load.l  r20, 176(r29)              ; reload last_buttons (mouse coords r15/r16 unchanged in r registers)

.idev_mv_check_btn:
    ; --- Buttons changed? Emit IE_MOUSE_BTN ---
    beq     r17, r20, .idev_save_state
    move.l  r21, #IE_MOUSE_BTN
    lsl     r21, r21, #24
    move.q  r23, r17
    lsl     r23, r23, #16
    or      r21, r21, r23
    move.q  r22, r15
    lsl     r22, r22, #48
    move.q  r23, r16
    lsl     r23, r23, #32
    or      r22, r22, r23
    load.l  r19, 180(r29)
    add     r19, r19, #1
    store.l r19, 180(r29)
    or      r22, r22, r19
    move.q  r1, r24
    move.l  r2, #INPUT_EVENT
    move.q  r3, r21
    move.q  r4, r22
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

.idev_save_state:
    store.l r15, 168(r29)
    store.l r16, 172(r29)
    store.l r17, 176(r29)

.idev_yield:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .idev_main

.idev_halt:
    syscall #SYS_YIELD
    bra     .idev_halt
prog_input_device_code_end:

prog_input_device_data:
    dc.b    "console.handler", 0
    dc.b    "input.device", 0, 0, 0, 0
    dc.b    "input.device M11 [Task ", 0
    ds.b    8                           ; pad to offset 64
    ds.b    64                          ; pad to offset 128
    ds.b    8                           ; 128: task_id
    ds.b    8                           ; 136: (unused)
    ds.b    8                           ; 144: input_port
    ds.b    8                           ; 152: chip_mmio_va
    ds.b    8                           ; 160: subscriber_port
    ds.b    4                           ; 168: last_mouse_x
    ds.b    4                           ; 172: last_mouse_y
    ds.b    4                           ; 176: last_mouse_buttons
    ds.b    4                           ; 180: event_seq
    ds.b    8                           ; 184: pad
    ; --- M12.5 additions ---
    dc.b    "hardware.resource", 0      ; 192: broker port name
    ds.b    14                          ; pad to offset 224 (192+32)
    ds.b    8                           ; 224: hwres_port
    ds.b    8                           ; 232: reply_port
prog_input_device_data_end:
    align   8
prog_input_device_end:
