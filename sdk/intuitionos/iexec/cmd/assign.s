; ---------------------------------------------------------------------------
; ASSIGN — list/show/set/add/remove DOS assigns through DOS_ASSIGN
;
; Syntax (M15.3):
;   ASSIGN                          → list every visible assign + first effective target
;   ASSIGN NAME:                    → show NAME's full effective ordered list
;                                     (overlay entries first, then built-in base list)
;   ASSIGN NAME: TARGET:            → replace NAME's mutable overlay with [TARGET]
;   ASSIGN ADD NAME: TARGET:        → append TARGET to NAME's mutable overlay
;   ASSIGN REMOVE NAME: TARGET:     → remove TARGET from NAME's mutable overlay
;
; Trailing colons on NAME and TARGET are stripped/converted to slashes
; before being handed off to dos.library, matching the DOS_ASSIGN ABI.
; ---------------------------------------------------------------------------

prog_assign_cmd:
    dc.l    0, 0
    dc.l    prog_assign_cmd_code_end - prog_assign_cmd_code
    dc.l    prog_assign_cmd_data_end - prog_assign_cmd_data
    dc.l    0
    ds.b    12
prog_assign_cmd_code:

    sub     sp, sp, #16
.asn_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.asn_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .asn_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .asn_open_con_retry
.asn_open_con_ok:
    store.q r1, 48(r29)

.asn_open_dos_alloc:
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .asn_open_dos_alloc_wait
    store.q r1, 144(r29)               ; dos_open_sigbit
.asn_open_dos_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 152(r29)              ; waiter status sentinel
    store.l r0, 156(r29)
    store.l r0, 160(r29)
    store.l r0, 164(r29)
    add     r1, r29, #16
    move.q  r2, r0
    load.q  r3, 144(r29)               ; waiter-owned signal bit
    add     r4, r29, #152              ; 16-byte outcome scratch
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .asn_find_dos
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .asn_open_dos_wait
    load.l  r14, 152(r29)
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .asn_open_dos_done
    load.q  r14, 144(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .asn_open_dos_wait
    load.l  r14, 152(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .asn_open_dos_wait
.asn_open_dos_done:
    store.q r1, 168(r29)               ; dos_library_token
    bnez    r14, .asn_open_dos_wait
.asn_find_dos:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .asn_open_dos_wait
.asn_open_dos_ok:
    store.q r1, 56(r29)
    load.q  r14, 144(r29)
    beqz    r14, .asn_open_dos_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 144(r29)
.asn_open_dos_sigfree_done:
    bra     .asn_open_dos_ready

.asn_open_dos_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .asn_open_dos_retry
.asn_open_dos_alloc_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .asn_open_dos_alloc

.asn_open_dos_ready:

    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 64(r29)

    move.l  r1, #0x1000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    store.q r1, 72(r29)
    store.l r3, 80(r29)

    ; M15.3: default op = DOS_ASSIGN_SET. ASSIGN ADD / REMOVE rewrite this
    ; field after the keyword is recognised.
    move.l  r1, #DOS_ASSIGN_SET
    store.l r1, 132(r29)

    add     r14, r29, #DATA_ARGS_OFFSET
.asn_skip_ws:
    load.b  r15, (r14)
    beqz    r15, .asn_do_list
    move.l  r16, #0x20
    bne     r15, r16, .asn_check_keyword
    add     r14, r14, #1
    bra     .asn_skip_ws

    ; --- M15.3 keyword detection: ADD or REMOVE prefixes the row args ---
.asn_check_keyword:
    ; ADD: 3 chars then space
    load.b  r15, (r14)
    move.l  r16, #0x41
    beq     r15, r16, .asn_check_add
    move.l  r16, #0x61
    beq     r15, r16, .asn_check_add
    move.l  r16, #0x52
    beq     r15, r16, .asn_check_remove
    move.l  r16, #0x72
    beq     r15, r16, .asn_check_remove
    bra     .asn_have_args

.asn_check_add:
    ; second char 'D' / 'd'?
    load.b  r15, 1(r14)
    move.l  r16, #0x44
    beq     r15, r16, .asn_check_add_d
    move.l  r16, #0x64
    beq     r15, r16, .asn_check_add_d
    bra     .asn_have_args
.asn_check_add_d:
    load.b  r15, 2(r14)
    move.l  r16, #0x44
    beq     r15, r16, .asn_check_add_dd
    move.l  r16, #0x64
    beq     r15, r16, .asn_check_add_dd
    bra     .asn_have_args
.asn_check_add_dd:
    load.b  r15, 3(r14)
    move.l  r16, #0x20
    bne     r15, r16, .asn_have_args
    move.l  r1, #DOS_ASSIGN_ADD
    store.l r1, 132(r29)
    add     r14, r14, #4
    bra     .asn_skip_ws_after_kw

.asn_check_remove:
    load.b  r15, 1(r14)
    move.l  r16, #0x45
    beq     r15, r16, .asn_check_rem_e
    move.l  r16, #0x65
    beq     r15, r16, .asn_check_rem_e
    bra     .asn_have_args
.asn_check_rem_e:
    load.b  r15, 2(r14)
    move.l  r16, #0x4D
    beq     r15, r16, .asn_check_rem_m
    move.l  r16, #0x6D
    beq     r15, r16, .asn_check_rem_m
    bra     .asn_have_args
.asn_check_rem_m:
    load.b  r15, 3(r14)
    move.l  r16, #0x4F
    beq     r15, r16, .asn_check_rem_o
    move.l  r16, #0x6F
    beq     r15, r16, .asn_check_rem_o
    bra     .asn_have_args
.asn_check_rem_o:
    load.b  r15, 4(r14)
    move.l  r16, #0x56
    beq     r15, r16, .asn_check_rem_v
    move.l  r16, #0x76
    beq     r15, r16, .asn_check_rem_v
    bra     .asn_have_args
.asn_check_rem_v:
    load.b  r15, 5(r14)
    move.l  r16, #0x45
    beq     r15, r16, .asn_check_rem_e2
    move.l  r16, #0x65
    beq     r15, r16, .asn_check_rem_e2
    bra     .asn_have_args
.asn_check_rem_e2:
    load.b  r15, 6(r14)
    move.l  r16, #0x20
    bne     r15, r16, .asn_have_args
    move.l  r1, #DOS_ASSIGN_REMOVE
    store.l r1, 132(r29)
    add     r14, r14, #7
    bra     .asn_skip_ws_after_kw

.asn_skip_ws_after_kw:
    load.b  r15, (r14)
    beqz    r15, .asn_bad_args
    move.l  r16, #0x20
    bne     r15, r16, .asn_have_args
    add     r14, r14, #1
    bra     .asn_skip_ws_after_kw

.asn_have_args:
    store.q r14, 88(r29)
    move.l  r17, #0
.asn_name_len:
    add     r18, r14, r17
    load.b  r19, (r18)
    beqz    r19, .asn_name_done_eos
    move.l  r20, #0x20
    beq     r19, r20, .asn_name_done
    add     r17, r17, #1
    move.l  r20, #16
    blt     r17, r20, .asn_name_len
    bra     .asn_bad_args
.asn_name_done_eos:
    ; End of args after just NAME. For SET op (no keyword) and a NAME-only
    ; invocation we treat this as `ASSIGN NAME:` → show layered list.
    beqz    r17, .asn_bad_args
    load.l  r3, 132(r29)
    move.l  r1, #DOS_ASSIGN_SET
    bne     r3, r1, .asn_bad_args              ; ADD/REMOVE require TARGET too
    bra     .asn_send_show_one
.asn_name_done:
    load.b  r19, (r18)
    beqz    r17, .asn_bad_args
    add     r14, r18, #1
.asn_skip_ws2:
    load.b  r15, (r14)
    beqz    r15, .asn_bad_args
    move.l  r16, #0x20
    bne     r15, r16, .asn_have_target
    add     r14, r14, #1
    bra     .asn_skip_ws2
.asn_have_target:
    store.q r14, 104(r29)
    move.l  r21, #0
.asn_target_len:
    add     r18, r14, r21
    load.b  r19, (r18)
    beqz    r19, .asn_target_done
    move.l  r20, #0x20
    beq     r19, r20, .asn_target_done
    add     r21, r21, #1
    move.l  r20, #16
    blt     r21, r20, .asn_target_len
    bra     .asn_bad_args
.asn_target_done:
    beqz    r21, .asn_bad_args

    load.q  r15, 72(r29)
    move.l  r16, #0
.asn_zero_row:
    move.l  r20, #32
    bge     r16, r20, .asn_strip_name_colon
    add     r18, r15, r16
    store.b r0, (r18)
    add     r16, r16, #1
    bra     .asn_zero_row

.asn_strip_name_colon:
    load.q  r14, 88(r29)
    move.q  r16, r17
    sub     r18, r16, #1
    add     r18, r14, r18
    load.b  r19, (r18)
    move.l  r20, #0x3A
    bne     r19, r20, .asn_name_copy
    sub     r16, r16, #1
    beqz    r16, .asn_bad_args
.asn_name_copy:
    move.l  r18, #0
.asn_name_copy_loop:
    bge     r18, r16, .asn_strip_target_colon
    add     r19, r14, r18
    load.b  r20, (r19)
    add     r19, r15, r18
    store.b r20, (r19)
    add     r18, r18, #1
    bra     .asn_name_copy_loop

.asn_strip_target_colon:
    load.q  r14, 104(r29)
    move.q  r16, r21
    sub     r18, r16, #1
    add     r18, r14, r18
    load.b  r19, (r18)
.asn_target_copy:
    move.l  r18, #0
.asn_target_copy_loop:
    bge     r18, r16, .asn_send_op
    add     r19, r14, r18
    load.b  r20, (r19)
    move.l  r21, #0x3A
    bne     r20, r21, .asn_target_store
    move.l  r20, #0x2F
.asn_target_store:
    add     r19, r15, r18
    add     r19, r19, #DOS_ASSIGN_TARGET_OFF
    store.b r20, (r19)
    add     r18, r18, #1
    bra     .asn_target_copy_loop

    ; --- Send DOS_ASSIGN_{SET|ADD|REMOVE} with the prepared row ---
.asn_send_op:
    load.q  r29, (sp)
    move.l  r2, #DOS_ASSIGN
    load.l  r3, 132(r29)
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.l  r6, 80(r29)
    load.q  r1, 56(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .asn_bad_args
    bra     .asn_exit

    ; --- ASSIGN NAME: → DOS_ASSIGN_LAYERED_QUERY: print N targets ---
.asn_send_show_one:
    ; Stash NAME at the start of share buffer, NUL-terminated.
    load.q  r15, 72(r29)
    load.q  r14, 88(r29)
    move.q  r16, r17
    sub     r18, r16, #1
    add     r18, r14, r18
    load.b  r19, (r18)
    move.l  r20, #0x3A
    bne     r19, r20, .asn_show_name_copy
    sub     r16, r16, #1
    beqz    r16, .asn_bad_args
.asn_show_name_copy:
    move.l  r18, #0
.asn_show_name_copy_loop:
    bge     r18, r16, .asn_show_name_copied
    add     r19, r14, r18
    load.b  r20, (r19)
    add     r19, r15, r18
    store.b r20, (r19)
    add     r18, r18, #1
    bra     .asn_show_name_copy_loop
.asn_show_name_copied:
    add     r19, r15, r18
    store.b r0, (r19)

    load.q  r29, (sp)
    move.l  r2, #DOS_ASSIGN
    move.l  r3, #DOS_ASSIGN_LAYERED_QUERY
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.l  r6, 80(r29)
    load.q  r1, 56(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .asn_bad_args
    store.q r2, 120(r29)               ; layered count
    load.q  r14, 88(r29)               ; reuse name ptr for header
    move.q  r20, r14
    jsr     .asn_send_string
    move.l  r3, #0x3A
    jsr     .asn_putc
    move.l  r3, #0x0D
    jsr     .asn_putc
    move.l  r3, #0x0A
    jsr     .asn_putc
    load.q  r14, 72(r29)               ; share buffer (32-byte target slots)
    move.l  r15, #0
.asn_show_one_loop:
    load.q  r16, 120(r29)
    bge     r15, r16, .asn_exit
    move.l  r3, #0x20
    jsr     .asn_putc
    move.l  r3, #0x20
    jsr     .asn_putc
    load.q  r14, 72(r29)
    move.l  r3, #DOS_ASSIGN_LAYERED_TGT_SZ
    mulu.q  r4, r15, r3
    add     r20, r14, r4
    jsr     .asn_send_string
    move.l  r3, #0x0D
    jsr     .asn_putc
    move.l  r3, #0x0A
    jsr     .asn_putc
    add     r15, r15, #1
    bra     .asn_show_one_loop

    ; --- ASSIGN (no args) → LIST ---
.asn_do_list:
    move.l  r2, #DOS_ASSIGN
    move.l  r3, #DOS_ASSIGN_LIST
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.l  r6, 80(r29)
    load.q  r1, 56(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .asn_exit
    store.q r2, 120(r29)
    load.q  r14, 72(r29)
    move.l  r15, #0
.asn_row_loop:
    load.q  r16, 120(r29)
    bge     r15, r16, .asn_exit
    store.q r14, 8(sp)
    move.q  r20, r14
    jsr     .asn_send_string
    load.q  r14, 8(sp)
    move.l  r3, #0x3A
    jsr     .asn_putc
    load.q  r14, 8(sp)
    add     r18, r14, #DOS_ASSIGN_TARGET_OFF
    load.b  r19, (r18)
    beqz    r19, .asn_row_crlf
    move.l  r3, #0x20
    jsr     .asn_putc
    load.q  r14, 8(sp)
    add     r18, r14, #DOS_ASSIGN_TARGET_OFF
    move.q  r20, r18
    jsr     .asn_send_string
.asn_row_crlf:
    move.l  r3, #0x0D
    jsr     .asn_putc
    move.l  r3, #0x0A
    jsr     .asn_putc
    load.q  r14, 8(sp)
    add     r14, r14, #32
    add     r15, r15, #1
    bra     .asn_row_loop

.asn_bad_args:
    load.q  r29, (sp)
    add     r20, r29, #32
    jsr     .asn_send_string

.asn_exit:
    bra     .asn_cleanup_exit

.asn_putc:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
    store.q r3, 8(sp)
.asn_putc_retry:
    load.q  r3, 8(sp)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 48(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .asn_putc_full
    add     sp, sp, #16
    rts
.asn_putc_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .asn_putc_retry

.asn_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.asn_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .asn_ss_done
    store.q r20, 8(sp)
.asn_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 48(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .asn_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .asn_ss_loop
.asn_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .asn_ss_retry
.asn_ss_done:
    add     sp, sp, #16
    rts

.asn_cleanup_exit:
    load.q  r1, 168(r29)               ; dos_library_token
    beqz    r1, .asn_exit_task
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 168(r29)
.asn_exit_task:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

prog_assign_cmd_code_end:

prog_assign_cmd_data:
    dc.b    "console.handler", 0
    dc.b    "dos.library", 0, 0, 0, 0, 0
    dc.b    "Bad arguments", 0x0D, 0x0A, 0
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8                           ; 144: dos_open_sigbit
    ds.b    16                          ; 152: dos_open outcome scratch
    ds.b    8                           ; 168: dos_library_token
    align   8
prog_assign_cmd_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    1
    dc.b    "Assign", 0
    ds.b    IOSM_NAME_SIZE - 8
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_assign_cmd_data_end:
    align   8
prog_assign_cmd_end:
