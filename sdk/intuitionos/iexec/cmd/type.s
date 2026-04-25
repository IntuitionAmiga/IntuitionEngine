; ---------------------------------------------------------------------------
; TYPE — display contents of a RAM: file
; ---------------------------------------------------------------------------

prog_type:
    dc.l    0, 0
    dc.l    prog_type_code_end - prog_type_code
    dc.l    prog_type_data_end - prog_type_data
    dc.l    0
    ds.b    12
prog_type_code:

    sub     sp, sp, #16
.typ_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

    add     r14, r29, #DATA_ARGS_OFFSET
    load.b  r15, (r14)
    beqz    r15, .typ_no_file

    load.b  r15, (r14)
    move.l  r16, #0x61
    move.l  r17, #0x7A
    blt     r15, r16, .typ_chk_r
    bgt     r15, r17, .typ_chk_r
    and     r15, r15, #0xDF
.typ_chk_r:
    move.l  r16, #0x52
    bne     r15, r16, .typ_no_strip
    add     r18, r14, #1
    load.b  r15, (r18)
    move.l  r16, #0x61
    move.l  r17, #0x7A
    blt     r15, r16, .typ_chk_a
    bgt     r15, r17, .typ_chk_a
    and     r15, r15, #0xDF
.typ_chk_a:
    move.l  r16, #0x41
    bne     r15, r16, .typ_no_strip
    add     r18, r14, #2
    load.b  r15, (r18)
    move.l  r16, #0x61
    move.l  r17, #0x7A
    blt     r15, r16, .typ_chk_m
    bgt     r15, r17, .typ_chk_m
    and     r15, r15, #0xDF
.typ_chk_m:
    move.l  r16, #0x4D
    bne     r15, r16, .typ_no_strip
    add     r18, r14, #3
    load.b  r15, (r18)
    move.l  r16, #0x3A
    bne     r15, r16, .typ_no_strip
    add     r14, r14, #4
.typ_no_strip:
    store.q r14, 8(sp)

.typ_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .typ_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_open_con_retry
.typ_open_con_ok:
    store.q r1, 64(r29)

.typ_open_dos_alloc:
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .typ_open_dos_alloc_wait
    store.q r1, 120(r29)               ; dos_open_sigbit
.typ_open_dos_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 128(r29)              ; waiter status sentinel
    store.l r0, 132(r29)
    store.l r0, 136(r29)
    store.l r0, 140(r29)
    add     r1, r29, #16
    move.l  r2, #0
    load.q  r3, 120(r29)
    add     r4, r29, #128
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .typ_find_dos
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .typ_open_dos_wait
    load.l  r14, 128(r29)
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .typ_open_dos_done
    load.q  r14, 120(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .typ_open_dos_wait
    load.l  r14, 128(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .typ_open_dos_wait
.typ_open_dos_done:
    store.q r1, 144(r29)               ; dos_library_token
    bnez    r14, .typ_open_dos_wait
.typ_find_dos:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .typ_open_dos_wait
.typ_open_dos_ok:
    store.q r1, 72(r29)
    load.q  r14, 120(r29)
    beqz    r14, .typ_open_dos_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 120(r29)
.typ_open_dos_sigfree_done:
    bra     .typ_open_dos_ready

.typ_open_dos_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_open_dos_retry
.typ_open_dos_alloc_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_open_dos_alloc

.typ_open_dos_ready:

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

    load.q  r14, 8(sp)
    load.q  r15, 88(r29)
    move.l  r16, #0
.typ_copy_name:
    add     r17, r14, r16
    load.b  r18, (r17)
    add     r17, r15, r16
    store.b r18, (r17)
    beqz    r18, .typ_copy_name_done
    add     r16, r16, #1
    move.l  r19, #255
    blt     r16, r19, .typ_copy_name
    add     r17, r15, r16
    store.b r0, (r17)
.typ_copy_name_done:

    load.q  r29, (sp)
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
    bnez    r1, .typ_not_found
    store.q r2, 104(r29)

    move.l  r2, #DOS_READ
    load.q  r3, 104(r29)
    move.l  r4, #4096
    load.q  r5, 80(r29)
    load.l  r6, 96(r29)
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    store.q r2, 112(r29)

    move.l  r16, #0
    store.q r16, 8(sp)
.typ_print_loop:
    load.q  r29, (sp)
    load.q  r16, 8(sp)
    load.q  r15, 112(r29)
    bge     r16, r15, .typ_close
.typ_print_retry:
    load.q  r29, (sp)
    load.q  r16, 8(sp)
    load.q  r14, 88(r29)
    add     r17, r14, r16
    load.b  r3, (r17)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 64(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .typ_print_full
    load.q  r16, 8(sp)
    add     r16, r16, #1
    store.q r16, 8(sp)
    bra     .typ_print_loop
.typ_print_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_print_retry

.typ_close:
    load.q  r29, (sp)
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

    bra     .typ_cleanup_exit

.typ_not_found:
    load.q  r29, (sp)
    add     r20, r29, #40
    jsr     .typ_send_string
    bra     .typ_cleanup_exit

.typ_no_file:
.typ_cleanup_exit:
    load.q  r1, 144(r29)               ; dos_library_token
    beqz    r1, .typ_exit_task
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 144(r29)
.typ_exit_task:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.typ_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.typ_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .typ_ss_done
    store.q r20, 8(sp)
.typ_ss_retry:
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
    beq     r2, r28, .typ_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .typ_ss_loop
.typ_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .typ_ss_retry
.typ_ss_done:
    add     sp, sp, #16
    rts

prog_type_code_end:

prog_type_data:
    dc.b    "console.handler", 0
    dc.b    "dos.library", 0, 0, 0, 0, 0
    dc.b    "RAM:", 0, 0, 0, 0
    dc.b    "File not found", 0x0D, 0x0A, 0
    ds.b    7
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8                           ; 120: dos_open_sigbit
    ds.b    16                          ; 128: dos_open outcome scratch
    ds.b    8                           ; 144: dos_library_token
    align   8
prog_type_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    0
    dc.b    "Type", 0
    ds.b    IOSM_NAME_SIZE - 6
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_type_data_end:
    align   8
prog_type_end:
