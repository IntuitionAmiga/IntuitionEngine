; ---------------------------------------------------------------------------
; ECHO — echo arguments to console
; ---------------------------------------------------------------------------

prog_echo_cmd:
    dc.l    0, 0
    dc.l    prog_echo_cmd_code_end - prog_echo_cmd_code
    dc.l    prog_echo_cmd_data_end - prog_echo_cmd_data
    dc.l    0
    ds.b    12
prog_echo_cmd_code:

    sub     sp, sp, #16
.echo_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.echo_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .echo_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_open_retry
.echo_open_ok:
    store.q r1, 16(r29)

    add     r20, r29, #DATA_ARGS_OFFSET
    jsr     .echo_send_string

    load.q  r29, (sp)
.echo_cr_retry:
    move.l  r3, #0x0D
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .echo_cr_full
    bra     .echo_lf
.echo_cr_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_cr_retry
.echo_lf:
.echo_lf_retry:
    move.l  r3, #0x0A
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .echo_lf_full
    bra     .echo_exit
.echo_lf_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_lf_retry

.echo_exit:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.echo_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.echo_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .echo_ss_done
    store.q r20, 8(sp)
.echo_ss_retry:
    load.q  r20, 8(sp)
    load.b  r3, (r20)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .echo_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .echo_ss_loop
.echo_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .echo_ss_retry
.echo_ss_done:
    add     sp, sp, #16
    rts

prog_echo_cmd_code_end:

prog_echo_cmd_data:
    dc.b    "console.handler", 0
    ds.b    8
    align   8
prog_echo_cmd_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    0
    dc.b    "Echo", 0
    ds.b    IOSM_NAME_SIZE - 6
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-22", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_echo_cmd_data_end:
    align   8
prog_echo_cmd_end:
