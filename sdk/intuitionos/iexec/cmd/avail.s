; ---------------------------------------------------------------------------
; AVAIL — display physical vs allocatable memory
; ---------------------------------------------------------------------------

prog_avail:
    dc.l    0, 0
    dc.l    prog_avail_code_end - prog_avail_code
    dc.l    prog_avail_data_end - prog_avail_data
    dc.l    0
    ds.b    12
prog_avail_code:

    sub     sp, sp, #16
.av_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.av_open_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .av_open_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .av_open_retry
.av_open_ok:
    store.q r1, 64(r29)

    add     r20, r29, #16
    jsr     .av_send_string

    move.l  r1, #MMU_NUM_PAGES
    lsl     r1, r1, #2
    store.q r1, 8(sp)
    jsr     .av_print_number

    load.q  r29, (sp)
    add     r20, r29, #24
    jsr     .av_send_string

    move.l  r1, #SYSINFO_TOTAL_PAGES
    syscall #SYS_GET_SYS_INFO
    load.q  r29, (sp)
    lsl     r1, r1, #2
    store.q r1, 8(sp)
    jsr     .av_print_number

    load.q  r29, (sp)
    add     r20, r29, #40
    jsr     .av_send_string

    move.l  r1, #SYSINFO_FREE_PAGES
    syscall #SYS_GET_SYS_INFO
    load.q  r29, (sp)
    lsl     r1, r1, #2
    store.q r1, 8(sp)
    jsr     .av_print_number

    load.q  r29, (sp)
    add     r20, r29, #52
    jsr     .av_send_string

    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.av_print_number:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
    load.q  r14, 32(sp)
    add     r15, r29, #80
    add     r16, r15, #15
    store.b r0, (r16)
    bnez    r14, .av_divloop
    sub     r16, r16, #1
    move.l  r17, #0x30
    store.b r17, (r16)
    bra     .av_send_digits
.av_divloop:
    beqz    r14, .av_send_digits
    move.q  r17, r0
    move.q  r18, r14
.av_div10:
    move.l  r19, #10
    blt     r18, r19, .av_div10_done
    sub     r18, r18, #10
    add     r17, r17, #1
    bra     .av_div10
.av_div10_done:
    add     r18, r18, #0x30
    sub     r16, r16, #1
    store.b r18, (r16)
    move.q  r14, r17
    bra     .av_divloop
.av_send_digits:
    move.q  r20, r16
    jsr     .av_send_string
    add     sp, sp, #16
    rts

.av_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.av_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .av_ss_done
    store.q r20, 8(sp)
.av_ss_retry:
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
    beq     r2, r28, .av_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .av_ss_loop
.av_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .av_ss_retry
.av_ss_done:
    add     sp, sp, #16
    rts

prog_avail_code_end:

prog_avail_data:
    dc.b    "console.handler", 0
    dc.b    "Phys: ", 0, 0
    dc.b    " KB  Alloc: ", 0, 0, 0, 0
    dc.b    " KB  Free: ", 0
    dc.b    " KB", 0x0D, 0x0A, 0
    ds.b    5
    ds.b    8
    ds.b    8
    ds.b    16
    align   8
prog_avail_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    1
    dc.b    "Avail", 0
    ds.b    IOSM_NAME_SIZE - 7
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_avail_data_end:
    align   8
prog_avail_end:
