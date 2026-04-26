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

.which_open_dos_alloc:
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .which_open_dos_alloc_wait
    store.q r1, 112(r29)               ; dos_open_sigbit
.which_open_dos_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 120(r29)              ; waiter status sentinel
    store.l r0, 124(r29)
    store.l r0, 128(r29)
    store.l r0, 132(r29)
    add     r1, r29, #16
    move.l  r2, #0
    load.q  r3, 112(r29)
    add     r4, r29, #120
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .which_find_dos
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .which_open_dos_wait
    load.l  r14, 120(r29)
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .which_open_dos_done
    load.q  r14, 112(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .which_open_dos_wait
    load.l  r14, 120(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .which_open_dos_wait
.which_open_dos_done:
    store.q r1, 136(r29)               ; dos_library_token
    bnez    r14, .which_open_dos_wait
.which_find_dos:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .which_open_dos_wait
.which_open_dos_ok:
    store.q r1, 72(r29)
    load.q  r14, 112(r29)
    beqz    r14, .which_open_dos_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 112(r29)
.which_open_dos_sigfree_done:
    bra     .which_open_dos_ready

.which_open_dos_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .which_open_dos_retry
.which_open_dos_alloc_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .which_open_dos_alloc

.which_open_dos_ready:

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
    load.q  r1, 136(r29)               ; dos_library_token
    beqz    r1, .which_exit_task
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 136(r29)
.which_exit_task:
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
    ds.b    8                           ; 112: dos_open_sigbit
    ds.b    16                          ; 120: dos_open outcome scratch
    ds.b    8                           ; 136: dos_library_token
    align   8
prog_which_cmd_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    1
    dc.b    "Which", 0
    ds.b    IOSM_NAME_SIZE - 7
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_which_cmd_data_end:
    align   8
prog_which_cmd_end:
