; ---------------------------------------------------------------------------
; HELP — print the built-in help text
; ---------------------------------------------------------------------------

prog_help_app:
    dc.l    0, 0
    dc.l    prog_help_app_code_end - prog_help_app_code
    dc.l    prog_help_app_data_end - prog_help_app_data
    dc.l    0
    ds.b    12
prog_help_app_code:

    sub     sp, sp, #16
.help_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.help_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .help_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .help_open_retry
.help_open_ok:
    store.q r1, 16(r29)

    load.q  r29, (sp)
    add     r20, r29, #32
    jsr     .help_send_string

    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.help_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.help_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .help_ss_done
    store.q r20, 8(sp)
.help_ss_retry:
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
    beq     r2, r28, .help_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .help_ss_loop
.help_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .help_ss_retry
.help_ss_done:
    add     sp, sp, #16
    rts

prog_help_app_code_end:

prog_help_app_data:
    dc.b    "console.handler", 0
    ds.b    8
    ds.b    8
    dc.b    "M15 help surface:", 0x0D, 0x0A
    dc.b    "Commands: VERSION AVAIL DIR TYPE ECHO ASSIGN LIST WHICH HELP", 0x0D, 0x0A
    dc.b    "Assigns: RAM: C: L: LIBS: DEVS: T: S: RESOURCES:", 0x0D, 0x0A, 0
prog_help_app_data_end:
    align   8
prog_help_app_end:
