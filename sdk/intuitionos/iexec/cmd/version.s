; ---------------------------------------------------------------------------
; VERSION — display system version string
; ---------------------------------------------------------------------------
; Opens console.handler, sends version string, exits.

prog_version:
    ; Header
    dc.l    0, 0
    dc.l    prog_version_code_end - prog_version_code
    dc.l    prog_version_data_end - prog_version_data
    dc.l    0
    ds.b    12
prog_version_code:

    ; === Preamble ===
    sub     sp, sp, #16
.ver_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

    ; === OpenLibrary("console.handler", 0) ===
.ver_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .ver_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .ver_open_retry
.ver_open_ok:
    store.q r1, 16(r29)

    ; === Send version string ===
    add     r20, r29, #32
    jsr     .ver_send_string

    ; === ExitTask ===
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.ver_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.ver_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .ver_ss_done
    store.q r20, 8(sp)
.ver_ss_retry:
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
    beq     r2, r28, .ver_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .ver_ss_loop
.ver_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .ver_ss_retry
.ver_ss_done:
    add     sp, sp, #16
    rts

prog_version_code_end:

prog_version_data:
    dc.b    "console.handler", 0
    ds.b    8
    ds.b    8
    dc.b    "IntuitionOS 0.16", 0x0D, 0x0A, 0
prog_version_data_end:
    align   8
prog_version_end:
