; ---------------------------------------------------------------------------
; RESIDENT — list resident inventory or toggle universal userland residency.
; Usage: Resident [<name> ADD|REMOVE]
; M16.4.3 command residency is DOS-owned through #DOS_RESIDENT_ADD,
; #DOS_RESIDENT_REMOVE, and #DOS_RESIDENT_LIST; protected service rows keep
; using SYS_SET_RESIDENT.
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
    jsr     .resident_open_console

    add     r20, r29, #DATA_ARGS_OFFSET
    jsr     .resident_skip_ws
    load.b  r21, (r20)
    beqz    r21, .resident_list
    add     r18, r29, #128             ; module name scratch
    store.q r18, 32(r29)               ; module name ptr
    move.l  r17, #0

.resident_find_name_end:
    load.b  r21, (r20)
    beqz    r21, .resident_name_done
    move.l  r22, #0x20
    beq     r21, r22, .resident_name_done
    move.l  r23, #31
    bge     r17, r23, .resident_usage
    store.b r21, (r18)
    add     r18, r18, #1
    add     r20, r20, #1
    add     r17, r17, #1
    bra     .resident_find_name_end
.resident_name_done:
    store.b r0, (r18)
    beqz    r21, .resident_name_terminated
    add     r20, r20, #1
.resident_name_terminated:
    load.q  r29, (sp)
    load.q  r1, 32(r29)
    jsr     .resident_name_is_all
    bnez    r1, .resident_all_candidate
.resident_after_all_check:
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
    add     r20, r20, #3
    jsr     .resident_require_eol
    bnez    r21, .resident_usage
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
    add     r20, r20, #6
    jsr     .resident_require_eol
    bnez    r21, .resident_usage
    load.q  r29, (sp)
    load.q  r1, 32(r29)
    jsr     .resident_name_is_all
    bnez    r1, .resident_all_remove_unsupported

.resident_do_remove:
    move.q  r2, r0
    load.q  r29, (sp)
    store.l r2, 80(r29)                ; resident mutation direction
    load.q  r1, 32(r29)
    jsr     .resident_name_has_dot
    beqz    r1, .resident_do_dos_remove
    bra     .resident_do_call
.resident_do_add:
    move.l  r2, #1
    load.q  r29, (sp)
    store.l r2, 80(r29)                ; resident mutation direction
    load.q  r1, 32(r29)
    jsr     .resident_name_has_dot
    beqz    r1, .resident_do_dos_add
    jsr     .resident_autoload_library
.resident_do_call:
    load.q  r29, (sp)
    store.l r2, 80(r29)                ; resident mutation direction
    load.q  r1, 32(r29)
    syscall #SYS_SET_RESIDENT
    load.q  r29, (sp)
    store.q r2, 120(r29)               ; preserve mutation result while closing autoload token
    load.q  r1, 96(r29)
    beqz    r1, .resident_after_autoload_close
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 96(r29)
.resident_after_autoload_close:
    load.q  r2, 120(r29)
    beqz    r2, .resident_mutation_ok
    move.l  r23, #ERR_NOTFOUND
    beq     r2, r23, .resident_not_found_or_unsupported_file
    move.l  r23, #ERR_UNSUPPORTED
    beq     r2, r23, .resident_unsupported
    bra     .resident_usage

.resident_mutation_ok:
    load.l  r24, 80(r29)
    bnez    r24, .resident_added
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_removed - prog_resident_cmd_data)
    jsr     .resident_send_string
    bra     .resident_exit
.resident_added:
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_added - prog_resident_cmd_data)
    jsr     .resident_send_string
    bra     .resident_exit
.resident_not_found:
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_not_found - prog_resident_cmd_data)
    jsr     .resident_send_string
    bra     .resident_exit
.resident_not_found_or_unsupported_file:
    load.q  r29, (sp)
    load.q  r1, 32(r29)
    jsr     .resident_name_has_dot
    beqz    r1, .resident_unsupported
    bra     .resident_not_found
.resident_unsupported:
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_unsupported - prog_resident_cmd_data)
    jsr     .resident_send_string
    bra     .resident_exit

.resident_list:
    load.q  r29, (sp)
    add     r1, r29, #(resident_exec_name - prog_resident_cmd_data)
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .resident_inventory_err
    store.q r1, 56(r29)                ; exec.library port
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .resident_inventory_err
    store.q r1, 24(r29)
    move.l  r1, #4096
    move.l  r2, #0x10001               ; MEMF_PUBLIC | MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    bnez    r2, .resident_inventory_err
    store.q r1, 40(r29)
    store.q r3, 48(r29)
    load.q  r1, 56(r29)
    move.l  r2, #EXEC_MSG_LIST_RESIDENT_INVENTORY
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 24(r29)
    load.q  r6, 48(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .resident_inventory_err
    load.q  r1, 24(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .resident_inventory_err
    bnez    r4, .resident_inventory_err
    load.q  r25, 40(r29)               ; inventory buffer VA
    load.l  r26, 12(r25)               ; record_count
    store.l r26, 64(r29)               ; resident inventory record count
    store.l r0, 72(r29)                ; resident inventory loop index
.resident_list_loop:
    load.q  r29, (sp)
    load.q  r25, 40(r29)
    load.l  r26, 64(r29)
    load.l  r27, 72(r29)
    bge     r27, r26, .resident_list_dos
    move.q  r20, r27
    lsl     r20, r20, #6
    add     r20, r20, #32
    add     r20, r25, r20
    jsr     .resident_send_string
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_crlf - prog_resident_cmd_data)
    jsr     .resident_send_string
    load.q  r29, (sp)
    load.l  r27, 72(r29)
    add     r27, r27, #1
    store.l r27, 72(r29)
    bra     .resident_list_loop

.resident_list_dos:
    load.q  r29, (sp)
    add     r1, r29, #(resident_dos_name - prog_resident_cmd_data)
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .resident_exit
    store.q r1, 120(r29)               ; dos.library port
    load.q  r1, 120(r29)
    move.l  r2, #DOS_RESIDENT_LIST
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 24(r29)
    load.q  r6, 48(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .resident_exit
    load.q  r1, 24(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .resident_exit
    bnez    r4, .resident_exit
    load.q  r25, 40(r29)
    load.l  r26, 0(r25)
    move.l  r27, #DOS_RCLI_MAGIC
    bne     r26, r27, .resident_exit
    load.l  r26, 12(r25)
    store.l r26, 64(r29)
    store.l r0, 72(r29)
.resident_list_dos_loop:
    load.q  r29, (sp)
    load.q  r25, 40(r29)
    load.l  r26, 64(r29)
    load.l  r27, 72(r29)
    bge     r27, r26, .resident_exit
    move.q  r20, r27
    lsl     r20, r20, #6
    add     r20, r20, #DOS_RCLI_HDR_SIZE
    add     r20, r25, r20
    jsr     .resident_send_string
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_crlf - prog_resident_cmd_data)
    jsr     .resident_send_string
    load.q  r29, (sp)
    load.l  r27, 72(r29)
    add     r27, r27, #1
    store.l r27, 72(r29)
    bra     .resident_list_dos_loop
.resident_inventory_err:
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_inventory_err - prog_resident_cmd_data)
    jsr     .resident_send_string
    bra     .resident_exit

.resident_usage:
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_usage - prog_resident_cmd_data)
    jsr     .resident_send_string

.resident_exit:
    load.q  r29, (sp)
    load.q  r1, 40(r29)
    beqz    r1, .resident_exit_now
    move.l  r2, #4096
    syscall #SYS_FREE_MEM
.resident_exit_now:
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

.resident_require_eol:
    jsr     .resident_skip_ws
    load.b  r21, (r20)
    rts

.resident_all_candidate:
    jsr     .resident_skip_ws
    load.b  r21, (r20)
    beqz    r21, .resident_list
    bra     .resident_after_all_check

.resident_all_remove_unsupported:
    load.q  r29, (sp)
    add     r20, r29, #(resident_msg_all_remove_unsupported - prog_resident_cmd_data)
    jsr     .resident_send_string
    bra     .resident_exit

.resident_do_dos_add:
    move.l  r2, #DOS_RESIDENT_ADD
    bra     .resident_do_dos_mutation
.resident_do_dos_remove:
    move.l  r2, #DOS_RESIDENT_REMOVE
.resident_do_dos_mutation:
    load.q  r29, (sp)
    store.l r2, 112(r29)
    add     r1, r29, #(resident_dos_name - prog_resident_cmd_data)
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .resident_not_found
    store.q r1, 120(r29)
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    bnez    r2, .resident_not_found
    store.q r1, 24(r29)
    move.l  r1, #4096
    move.l  r2, #0x10001               ; MEMF_PUBLIC | MEMF_CLEAR
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    bnez    r2, .resident_not_found
    store.q r1, 40(r29)
    store.q r3, 48(r29)
    load.q  r20, 32(r29)
    load.q  r21, 40(r29)
    move.l  r22, #0
.resident_dos_copy_name:
    move.l  r23, #DOS_RCMD_NAME_MAX
    sub     r23, r23, #1
    bge     r22, r23, .resident_usage
    load.b  r24, (r20)
    store.b r24, (r21)
    beqz    r24, .resident_dos_send
    add     r20, r20, #1
    add     r21, r21, #1
    add     r22, r22, #1
    bra     .resident_dos_copy_name
.resident_dos_send:
    load.q  r1, 120(r29)
    load.l  r2, 112(r29)
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 24(r29)
    load.q  r6, 48(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .resident_not_found
    load.q  r1, 24(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .resident_not_found
    load.q  r25, 40(r29)
    load.l  r2, DOS_RCMD_RC_OFF(r25)
    beqz    r2, .resident_mutation_ok
    move.l  r23, #ERR_NOTFOUND
    beq     r2, r23, .resident_not_found
    bra     .resident_unsupported

.resident_name_is_all:
    load.b  r21, (r1)
    or      r21, r21, #0x20
    move.l  r22, #0x61                 ; 'a'
    bne     r21, r22, .resident_name_is_all_no
    load.b  r21, 1(r1)
    or      r21, r21, #0x20
    move.l  r22, #0x6C                 ; 'l'
    bne     r21, r22, .resident_name_is_all_no
    load.b  r21, 2(r1)
    or      r21, r21, #0x20
    bne     r21, r22, .resident_name_is_all_no
    load.b  r21, 3(r1)
    bnez    r21, .resident_name_is_all_no
    move.l  r1, #1
    rts
.resident_name_is_all_no:
    move.q  r1, r0
    rts

.resident_name_has_dot:
    load.b  r21, (r1)
    beqz    r21, .resident_name_has_dot_no
    move.l  r22, #0x2E                 ; '.'
    beq     r21, r22, .resident_name_has_dot_yes
    add     r1, r1, #1
    bra     .resident_name_has_dot
.resident_name_has_dot_yes:
    move.l  r1, #1
    rts
.resident_name_has_dot_no:
    move.q  r1, r0
    rts

.resident_autoload_library:
    load.q  r29, 8(sp)
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, 8(sp)
    bnez    r2, .resident_autoload_done
    store.q r1, 88(r29)                ; OpenLibraryEx signal bit
.resident_autoload_retry:
    load.q  r29, 8(sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 104(r29)              ; waiter status sentinel
    store.l r0, 108(r29)
    store.l r0, 112(r29)
    store.l r0, 116(r29)
    load.q  r1, 32(r29)
    move.q  r2, r0
    load.q  r3, 88(r29)
    add     r4, r29, #104
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, 8(sp)
    beqz    r2, .resident_autoload_opened
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .resident_autoload_free_signal
    load.l  r14, 104(r29)
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .resident_autoload_wait_done
    load.q  r14, 88(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, 8(sp)
    bnez    r2, .resident_autoload_free_signal
    load.l  r14, 104(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .resident_autoload_retry
.resident_autoload_wait_done:
    bnez    r14, .resident_autoload_free_signal
.resident_autoload_opened:
    store.q r1, 96(r29)                ; library token to close after pin attempt
.resident_autoload_free_signal:
    load.q  r1, 88(r29)
    beqz    r1, .resident_autoload_done
    syscall #SYS_FREE_SIGNAL
    load.q  r29, 8(sp)
    store.q r0, 88(r29)
.resident_autoload_done:
    move.l  r2, #1
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
    move.l  r2, #CON_MSG_CHAR
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

.resident_open_console:
    load.q  r29, 8(sp)
.resident_open_console_retry:
    add     r1, r29, #(resident_console_name - prog_resident_cmd_data)
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, 8(sp)
    beqz    r2, .resident_open_console_ok
    syscall #SYS_YIELD
    load.q  r29, 8(sp)
    bra     .resident_open_console_retry
.resident_open_console_ok:
    store.q r1, 16(r29)
    rts

prog_resident_cmd_code_end:

prog_resident_cmd_data:
    ds.b    16                        ; +0 legacy arg0 scratch/pad
    ds.b    8                         ; +16 reserved
    ds.b    8                         ; +24 reply port cache
    ds.b    8                         ; +32 module name pointer
    ds.b    8                         ; +40 inventory buffer VA
    ds.b    8                         ; +48 inventory share handle
    ds.b    8                         ; +56 exec.library port
    ds.b    8                         ; +64 resident inventory record count
    ds.b    8                         ; +72 resident inventory loop index
    ds.b    8                         ; +80 resident mutation direction
    ds.b    8                         ; +88 OpenLibraryEx signal bit
    ds.b    8                         ; +96 autoload library token
    ds.b    16                        ; +104 OpenLibraryEx waiter outcome
    ds.b    8                         ; +120 mutation result scratch
    ds.b    32                        ; +128 module name scratch
resident_console_name:
    dc.b    "console.handler", 0
    align   8
resident_exec_name:
    dc.b    "exec.library", 0
    align   8
resident_dos_name:
    dc.b    "dos.library", 0
    align   8
resident_msg_usage:
    dc.b    "Resident usage: Resident [<name> ADD|REMOVE]", 0x0D, 0x0A, 0
    align   8
resident_msg_crlf:
    dc.b    0x0D, 0x0A, 0
    align   8
resident_msg_added:
    dc.b    "Resident: added", 0x0D, 0x0A, 0
    align   8
resident_msg_removed:
    dc.b    "Resident: removed", 0x0D, 0x0A, 0
    align   8
resident_msg_not_found:
    dc.b    "Resident: not found", 0x0D, 0x0A, 0
    align   8
resident_msg_unsupported:
    dc.b    "Resident: unsupported target", 0x0D, 0x0A, 0
    align   8
resident_msg_all_remove_unsupported:
    dc.b    "Resident: all remove unsupported", 0x0D, 0x0A, 0
    align   8
resident_msg_inventory_err:
    dc.b    "Resident: inventory unavailable", 0x0D, 0x0A, 0
    align   8
prog_resident_cmd_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    2
    dc.w    0
    dc.b    "Resident", 0
    ds.b    IOSM_NAME_SIZE - 10
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_resident_cmd_data_end:
    align   8
prog_resident_cmd_end:
