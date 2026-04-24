; ---------------------------------------------------------------------------
; RESIDENT — toggle MODF_RESIDENT on a library row
; Usage: Resident <name> ADD|REMOVE
; ---------------------------------------------------------------------------

prog_resident_cmd:
    dc.l    0, 0
    dc.l    prog_resident_cmd_code_end - prog_resident_cmd_code
    dc.l    prog_resident_cmd_data_end - prog_resident_cmd_data
    dc.l    0
    ds.b    12
prog_resident_cmd_code:

    sub     sp, sp, #16
.resident_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

    add     r20, r29, #DATA_ARGS_OFFSET
    jsr     .resident_skip_ws
    beqz    r20, .resident_usage
    store.q r20, 32(r29)               ; module name ptr

.resident_find_name_end:
    load.b  r21, (r20)
    beqz    r21, .resident_usage
    move.l  r22, #0x20
    beq     r21, r22, .resident_name_done
    add     r20, r20, #1
    bra     .resident_find_name_end
.resident_name_done:
    store.b r0, (r20)
    add     r20, r20, #1
    jsr     .resident_skip_ws
    beqz    r20, .resident_usage

    load.b  r21, (r20)
    move.l  r22, #0x41                 ; 'A'
    beq     r21, r22, .resident_parse_add
    move.l  r22, #0x61                 ; 'a'
    beq     r21, r22, .resident_parse_add
    move.l  r22, #0x52                 ; 'R'
    beq     r21, r22, .resident_parse_remove
    move.l  r22, #0x72                 ; 'r'
    beq     r21, r22, .resident_parse_remove
    bra     .resident_usage

.resident_parse_add:
    load.b  r22, 1(r20)
    move.l  r23, #0x44                 ; 'D'
    beq     r22, r23, .resident_parse_add_2
    move.l  r23, #0x64                 ; 'd'
    bne     r22, r23, .resident_usage
.resident_parse_add_2:
    load.b  r22, 2(r20)
    move.l  r23, #0x44
    beq     r22, r23, .resident_parse_add_3
    move.l  r23, #0x64
    bne     r22, r23, .resident_usage
.resident_parse_add_3:
    load.b  r22, 3(r20)
    beqz    r22, .resident_do_add
    move.l  r23, #0x20
    bne     r22, r23, .resident_usage
    bra     .resident_do_add

.resident_parse_remove:
    load.b  r22, 1(r20)
    move.l  r23, #0x45                 ; 'E'
    beq     r22, r23, .resident_parse_remove_2
    move.l  r23, #0x65
    bne     r22, r23, .resident_usage
.resident_parse_remove_2:
    load.b  r22, 2(r20)
    move.l  r23, #0x4D                 ; 'M'
    beq     r22, r23, .resident_parse_remove_3
    move.l  r23, #0x6D
    bne     r22, r23, .resident_usage
.resident_parse_remove_3:
    load.b  r22, 3(r20)
    move.l  r23, #0x4F                 ; 'O'
    beq     r22, r23, .resident_parse_remove_4
    move.l  r23, #0x6F
    bne     r22, r23, .resident_usage
.resident_parse_remove_4:
    load.b  r22, 4(r20)
    move.l  r23, #0x56                 ; 'V'
    beq     r22, r23, .resident_parse_remove_5
    move.l  r23, #0x76
    bne     r22, r23, .resident_usage
.resident_parse_remove_5:
    load.b  r22, 5(r20)
    move.l  r23, #0x45                 ; 'E'
    beq     r22, r23, .resident_parse_remove_6
    move.l  r23, #0x65
    bne     r22, r23, .resident_usage
.resident_parse_remove_6:
    load.b  r22, 6(r20)
    beqz    r22, .resident_do_remove
    move.l  r23, #0x20
    bne     r22, r23, .resident_usage

.resident_do_remove:
    move.q  r2, r0
    bra     .resident_do_call
.resident_do_add:
    move.l  r2, #1
.resident_do_call:
    load.q  r29, (sp)
    load.q  r1, 32(r29)
    syscall #SYS_SET_RESIDENT
    load.q  r29, (sp)
    beqz    r2, .resident_exit

.resident_usage:
    add     r20, r29, #40
    jsr     .resident_open_console
    jsr     .resident_send_string

.resident_exit:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

.resident_skip_ws:
    load.b  r21, (r20)
    beqz    r21, .resident_skip_done
    move.l  r22, #0x20
    bne     r21, r22, .resident_skip_done
    add     r20, r20, #1
    bra     .resident_skip_ws
.resident_skip_done:
    rts

.resident_open_console:
    load.q  r29, (sp)
    load.q  r1, 16(r29)
    bnez    r1, .resident_open_console_done
.resident_open_console_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .resident_open_console_ok
    syscall #SYS_YIELD
    bra     .resident_open_console_retry
.resident_open_console_ok:
    store.q r1, 16(r29)
.resident_open_console_done:
    rts

.resident_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.resident_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .resident_ss_done
    store.q r20, 8(sp)
.resident_ss_retry:
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
    beq     r2, r28, .resident_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .resident_ss_loop
.resident_ss_full:
    syscall #SYS_YIELD
    bra     .resident_ss_retry
.resident_ss_done:
    add     sp, sp, #16
    rts

prog_resident_cmd_code_end:

prog_resident_cmd_data:
    dc.b    "console.handler", 0
    ds.b    8
    ds.b    8
    dc.b    "Resident usage: Resident <name> ADD|REMOVE", 0x0D, 0x0A, 0
    align   8
prog_resident_cmd_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    0
    dc.b    "Resident", 0
    ds.b    IOSM_NAME_SIZE - 10
    dc.l    0
    dc.l    0
    dc.b    "2026-04-22", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_resident_cmd_data_end:
    align   8
prog_resident_cmd_end:
