; ---------------------------------------------------------------------------
; WHICH — resolve a command through DOS bare-command search
; ---------------------------------------------------------------------------

prog_which_cmd:
    dc.l    0, 0
    dc.l    prog_which_cmd_code_end - prog_which_cmd_code
    dc.l    prog_which_cmd_data_end - prog_which_cmd_data
    dc.l    0
    ds.b    12
prog_which_cmd_code:

    sub     sp, sp, #16
.which_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

    add     r14, r29, #DATA_ARGS_OFFSET
.which_skip_ws:
    load.b  r15, (r14)
    beqz    r15, .which_exit
    move.l  r16, #0x20
    bne     r15, r16, .which_have_arg
    add     r14, r14, #1
    bra     .which_skip_ws
.which_have_arg:
    store.q r14, 104(r29)

.which_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .which_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .which_open_con_retry
.which_open_con_ok:
    store.q r1, 64(r29)

.which_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .which_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .which_open_dos_retry
.which_open_dos_ok:
    store.q r1, 72(r29)

    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 80(r29)

    move.l  r1, #0x1000
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    store.q r1, 88(r29)
    store.l r3, 96(r29)

    load.q  r14, 104(r29)
    load.q  r15, 88(r29)
    move.l  r16, #0x43
    store.b r16, (r15)
    add     r17, r15, #1
    move.l  r16, #0x3A
    store.b r16, (r17)
    move.l  r18, #2
.which_copy_arg:
    add     r17, r14, r18
    sub     r17, r17, #2
    load.b  r19, (r17)
    beqz    r19, .which_copy_done
    move.l  r20, #0x20
    beq     r19, r20, .which_copy_done
    add     r21, r15, r18
    store.b r19, (r21)
    add     r18, r18, #1
    move.l  r20, #63
    blt     r18, r20, .which_copy_arg
.which_copy_done:
    add     r21, r15, r18
    store.b r0, (r21)

    move.l  r2, #DOS_OPEN
    move.l  r3, #DOS_MODE_READ
    move.q  r4, r0
    load.q  r5, 80(r29)
    load.l  r6, 96(r29)
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .which_not_found
    store.q r2, 104(r29)

    move.l  r2, #DOS_CLOSE
    load.q  r3, 104(r29)
    move.q  r4, r0
    load.q  r5, 80(r29)
    move.q  r6, r0
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)

    load.q  r20, 88(r29)
    jsr     .which_send_string
    load.q  r29, (sp)
    add     r20, r29, #32
    jsr     .which_send_string
    bra     .which_exit

.which_not_found:
    load.q  r29, (sp)
    add     r20, r29, #48
    jsr     .which_send_string

.which_exit:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.which_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.which_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .which_ss_done
    store.q r20, 8(sp)
.which_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 64(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .which_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .which_ss_loop
.which_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .which_ss_retry
.which_ss_done:
    add     sp, sp, #16
    rts

prog_which_cmd_code_end:

prog_which_cmd_data:
    dc.b    "console.handler", 0
    dc.b    "dos.library", 0, 0, 0, 0, 0
    dc.b    0x0D, 0x0A, 0
    ds.b    13
    dc.b    "not found", 0x0D, 0x0A, 0
    ds.b    4
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
prog_which_cmd_data_end:
    align   8
prog_which_cmd_end:
