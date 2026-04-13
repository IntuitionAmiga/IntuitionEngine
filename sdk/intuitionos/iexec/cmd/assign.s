; ---------------------------------------------------------------------------
; ASSIGN — list or set DOS assigns through DOS_ASSIGN
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

.asn_open_dos_retry:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .asn_open_dos_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .asn_open_dos_retry
.asn_open_dos_ok:
    store.q r1, 56(r29)

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

    add     r14, r29, #DATA_ARGS_OFFSET
.asn_skip_ws:
    load.b  r15, (r14)
    beqz    r15, .asn_do_list
    move.l  r16, #0x20
    bne     r15, r16, .asn_have_args
    add     r14, r14, #1
    bra     .asn_skip_ws
.asn_have_args:
    store.q r14, 88(r29)
    move.l  r17, #0
.asn_name_len:
    add     r18, r14, r17
    load.b  r19, (r18)
    beqz    r19, .asn_bad_args
    move.l  r20, #0x20
    beq     r19, r20, .asn_name_done
    add     r17, r17, #1
    move.l  r20, #16
    blt     r17, r20, .asn_name_len
    bra     .asn_bad_args
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
    bge     r18, r16, .asn_send_set
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

.asn_send_set:
    load.q  r29, (sp)
    move.l  r2, #DOS_ASSIGN
    move.l  r3, #DOS_ASSIGN_SET
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
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

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
prog_assign_cmd_data_end:
    align   8
prog_assign_cmd_end:
