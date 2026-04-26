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
    dc.b    "IntuitionOS 1.16.7 help", 0x0D, 0x0A
    dc.b    "Commands: VERSION AVAIL DIR TYPE ECHO ASSIGN LIST WHICH HELP", 0x0D, 0x0A
    dc.b    "Assigns: RAM: C: L: LIBS: DEVS: T: S: RESOURCES:", 0x0D, 0x0A
    dc.b    "Universal Userland Residency: resident command cache uses DOS_RESIDENT_ADD.", 0x0D, 0x0A, 0
    align   8
prog_help_app_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    1
    dc.b    "Help", 0
    ds.b    IOSM_NAME_SIZE - 6
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_help_app_data_end:
    align   8
prog_help_app_end:
