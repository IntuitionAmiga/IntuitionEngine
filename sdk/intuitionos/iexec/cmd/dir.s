; ---------------------------------------------------------------------------
; DIR — list DOS filesystem contents
; ---------------------------------------------------------------------------

prog_dir:
    dc.l    0, 0
    dc.l    prog_dir_code_end - prog_dir_code
    dc.l    prog_dir_data_end - prog_dir_data
    dc.l    0
    ds.b    12
prog_dir_code:

    sub     sp, sp, #16
.dir_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.dir_open_con_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .dir_open_con_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_open_con_retry
.dir_open_con_ok:
    store.q r1, 64(r29)

.dir_open_dos_alloc:
    move.l  r1, #16
    syscall #SYS_ALLOC_SIGNAL
    load.q  r29, (sp)
    bnez    r2, .dir_open_dos_alloc_wait
    store.q r1, 160(r29)               ; dos_open_sigbit
.dir_open_dos_retry:
    load.q  r29, (sp)
    move.l  r14, #0xFFFFFFFF
    store.l r14, 168(r29)              ; waiter status sentinel
    store.l r0, 172(r29)
    store.l r0, 176(r29)
    store.l r0, 180(r29)
    add     r1, r29, #16
    move.l  r2, #0
    load.q  r3, 160(r29)               ; waiter-owned signal bit
    add     r4, r29, #168              ; 16-byte outcome scratch
    syscall #SYS_OPEN_LIBRARY_EX
    load.q  r29, (sp)
    beqz    r2, .dir_find_dos
    move.l  r14, #ERR_AGAIN
    bne     r2, r14, .dir_open_dos_wait
    load.l  r14, 168(r29)
    move.l  r15, #0xFFFFFFFF
    bne     r14, r15, .dir_open_dos_done
    load.q  r14, 160(r29)
    move.q  r1, #1
    lsl     r1, r1, r14
    syscall #SYS_WAIT
    load.q  r29, (sp)
    bnez    r2, .dir_open_dos_wait
    load.l  r14, 168(r29)
    move.l  r15, #0xFFFFFFFF
    beq     r14, r15, .dir_open_dos_wait
.dir_open_dos_done:
    store.q r1, 184(r29)               ; dos_library_token
    bnez    r14, .dir_open_dos_wait
.dir_find_dos:
    load.q  r29, (sp)
    add     r1, r29, #16
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .dir_open_dos_wait
.dir_open_dos_ok:
    store.q r1, 72(r29)
    load.q  r14, 160(r29)
    beqz    r14, .dir_open_dos_sigfree_done
    move.q  r1, r14
    syscall #SYS_FREE_SIGNAL
    load.q  r29, (sp)
    store.q r0, 160(r29)
.dir_open_dos_sigfree_done:
    bra     .dir_open_dos_ready

.dir_open_dos_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_open_dos_retry
.dir_open_dos_alloc_wait:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_open_dos_alloc

.dir_open_dos_ready:

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

    store.q r0, 104(r29)
    store.q r0, 112(r29)
    store.q r0, 136(r29)

    load.q  r14, 88(r29)
    store.b r0, (r14)
    add     r14, r29, #DATA_ARGS_OFFSET
.dir_skip_ws:
    load.b  r15, (r14)
    beqz    r15, .dir_args_done
    move.l  r16, #0x20
    bne     r15, r16, .dir_have_arg
    add     r14, r14, #1
    bra     .dir_skip_ws
.dir_have_arg:
    move.q  r17, r14
    move.l  r18, #0
.dir_arg_len:
    add     r19, r17, r18
    load.b  r20, (r19)
    beqz    r20, .dir_arg_len_done
    move.l  r21, #0x20
    beq     r20, r21, .dir_arg_len_done
    add     r18, r18, #1
    move.l  r21, #16
    blt     r18, r21, .dir_arg_len
.dir_arg_len_done:
    move.l  r21, #2
    blt     r18, r21, .dir_args_done
    sub     r19, r18, #1
    add     r19, r17, r19
    load.b  r20, (r19)
    move.l  r21, #0x3A
    bne     r20, r21, .dir_copy_explicit_path

    sub     r18, r18, #1
    load.q  r14, 88(r29)
    move.l  r20, #0
.dir_copy_query_name:
    bge     r20, r18, .dir_copy_query_done
    add     r21, r17, r20
    load.b  r22, (r21)
    add     r21, r14, r20
    store.b r22, (r21)
    add     r20, r20, #1
    bra     .dir_copy_query_name
.dir_copy_query_done:
    add     r21, r14, r20
    store.b r0, (r21)

    move.l  r2, #DOS_ASSIGN
    move.l  r3, #DOS_ASSIGN_QUERY
    move.q  r4, r0
    load.q  r5, 80(r29)
    load.l  r6, 96(r29)
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .dir_args_done

    ; Bare volume arguments like TMP: or IOSSYS: should list the resolved
    ; target directory directly, not feed through the assign:path suffix
    ; builder below.
    load.q  r14, 88(r29)
    add     r15, r14, #DOS_ASSIGN_TARGET_OFF
    add     r16, r29, #192
    move.l  r20, #0
.dir_copy_query_target_only:
    move.l  r21, #31
    bge     r20, r21, .dir_copy_query_target_only_done
    add     r22, r15, r20
    load.b  r23, (r22)
    beqz    r23, .dir_copy_query_target_only_done
    move.l  r21, #0x3A
    bne     r23, r21, .dir_copy_query_target_only_store
    move.l  r23, #0x2F
.dir_copy_query_target_only_store:
    add     r22, r16, r20
    store.b r23, (r22)
    add     r20, r20, #1
    bra     .dir_copy_query_target_only
.dir_copy_query_target_only_done:
    add     r22, r16, r20
    store.b r0, (r22)
    move.q  r1, r16
    move.q  r2, r14
    jsr     .dir_copy_zstr
    store.q r0, 104(r29)
    store.q r0, 112(r29)
    move.l  r21, #1
    store.q r21, 136(r29)
    bra     .dir_args_done

.dir_copy_explicit_path:
    ; Canonicalize assign:subdir arguments into a slash-form path that
    ; dos.library can traverse directly, e.g. IOSSYS:C -> SYS/IOSSYS/C/
    ; and TMP:LIBS -> SYS/IOSSYS/LIBS/. Build in scratch first so the
    ; DOS_ASSIGN_QUERY reply row in the shared buffer is not clobbered.
    move.l  r24, #0
.dir_explicit_find_colon:
    bge     r24, r18, .dir_copy_explicit_path_plain
    add     r25, r17, r24
    load.b  r26, (r25)
    move.l  r27, #0x3A
    beq     r26, r27, .dir_explicit_have_colon
    add     r24, r24, #1
    bra     .dir_explicit_find_colon
.dir_explicit_have_colon:
    move.q  r1, r17
    add     r2, r29, #160
    jsr     .dir_copy_zstr
    store.q r24, 152(r29)

    load.q  r14, 88(r29)
    move.l  r20, #0
.dir_explicit_copy_query_name:
    bge     r20, r24, .dir_explicit_copy_query_done
    add     r21, r17, r20
    load.b  r22, (r21)
    add     r21, r14, r20
    store.b r22, (r21)
    add     r20, r20, #1
    bra     .dir_explicit_copy_query_name
.dir_explicit_copy_query_done:
    add     r21, r14, r20
    store.b r0, (r21)

    move.l  r2, #DOS_ASSIGN
    move.l  r3, #DOS_ASSIGN_QUERY
    move.q  r4, r0
    load.q  r5, 80(r29)
    load.l  r6, 96(r29)
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .dir_copy_explicit_path_plain

    add     r17, r29, #160
    load.q  r24, 152(r29)
.dir_explicit_build_from_query:
    load.q  r14, 88(r29)
    add     r15, r14, #DOS_ASSIGN_TARGET_OFF
    add     r16, r29, #192
    move.l  r20, #0
.dir_explicit_copy_target:
    move.l  r21, #31
    bge     r20, r21, .dir_explicit_copy_suffix
    add     r22, r15, r20
    load.b  r23, (r22)
    beqz    r23, .dir_explicit_copy_suffix
    move.l  r21, #0x3A
    bne     r23, r21, .dir_explicit_target_store
    move.l  r23, #0x2F
.dir_explicit_target_store:
    add     r22, r16, r20
    store.b r23, (r22)
    add     r20, r20, #1
    bra     .dir_explicit_copy_target

.dir_explicit_copy_suffix:
    add     r21, r17, r24
    add     r21, r21, #1
    move.l  r22, #0
.dir_explicit_suffix_loop:
    move.l  r23, #31
    bge     r20, r23, .dir_explicit_maybe_term
    add     r23, r21, r22
    load.b  r25, (r23)
    beqz    r25, .dir_explicit_maybe_term
    add     r23, r16, r20
    store.b r25, (r23)
    add     r20, r20, #1
    add     r22, r22, #1
    bra     .dir_explicit_suffix_loop

.dir_explicit_maybe_term:
    beqz    r22, .dir_explicit_copyback
    beqz    r20, .dir_explicit_copyback
    add     r23, r16, r20
    sub     r23, r23, #1
    load.b  r25, (r23)
    move.l  r24, #0x2F
    beq     r25, r24, .dir_explicit_copyback
    move.l  r24, #0x3A
    beq     r25, r24, .dir_explicit_copyback
    move.l  r23, #31
    bge     r20, r23, .dir_explicit_copyback
    add     r23, r16, r20
    move.l  r24, #0x2F
    store.b r24, (r23)
    add     r20, r20, #1

.dir_explicit_copyback:
    add     r23, r16, r20
    store.b r0, (r23)
    move.q  r1, r16
    move.q  r2, r14
    jsr     .dir_copy_zstr
    store.q r0, 104(r29)
    store.q r0, 112(r29)
    move.l  r21, #1
    store.q r21, 136(r29)
    bra     .dir_args_done

.dir_copy_explicit_path_plain:
    load.q  r14, 88(r29)
    move.l  r20, #0
.dir_copy_explicit_path_loop:
    bge     r20, r18, .dir_copy_explicit_path_done
    add     r21, r17, r20
    load.b  r22, (r21)
    add     r21, r14, r20
    store.b r22, (r21)
    add     r20, r20, #1
    bra     .dir_copy_explicit_path_loop
.dir_copy_explicit_path_done:
    add     r21, r14, r20
    store.b r0, (r21)
    store.q r0, 104(r29)
    store.q r0, 112(r29)
    move.l  r21, #1
    store.q r21, 136(r29)
.dir_args_done:
    load.q  r14, 88(r29)
    load.q  r15, 136(r29)
    bnez    r15, .dir_args_preserved
    store.b r0, (r14)
.dir_args_preserved:
    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 80(r29)
    move.l  r2, #DOS_DIR
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 80(r29)
    load.l  r6, 96(r29)
    load.q  r1, 72(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)

    load.q  r1, 80(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r1, .dir_empty
    store.q r2, 8(sp)

    beqz    r2, .dir_empty

.dir_print_ready:
    load.q  r14, 88(r29)
    load.q  r15, 8(sp)
    move.l  r16, #0
.dir_print_loop:
    bge     r16, r15, .dir_done
    add     r17, r14, r16
    load.b  r18, (r17)
    beqz    r18, .dir_done
    store.q r16, 8(sp)
.dir_char_retry:
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
    beq     r2, r28, .dir_char_full
    load.q  r16, 8(sp)
    add     r16, r16, #1
    store.q r16, 8(sp)
    load.b  r18, (r17)
    bnez    r18, .dir_print_loop
    bra     .dir_done
.dir_char_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_char_retry

.dir_filter_listing:
    load.q  r14, 88(r29)
    load.q  r15, 8(sp)
    move.l  r16, #0
    move.l  r24, #0
.dir_filter_loop:
    bge     r16, r15, .dir_filter_done
    add     r17, r14, r16
    move.q  r18, r17
    move.q  r19, r16
.dir_find_lf:
    bge     r19, r15, .dir_have_line
    load.b  r20, (r18)
    move.l  r21, #0x0A
    beq     r20, r21, .dir_have_line
    add     r18, r18, #1
    add     r19, r19, #1
    bra     .dir_find_lf
.dir_have_line:
    load.q  r20, 112(r29)
    beqz    r20, .dir_match_root
    add     r21, r29, #120
    move.l  r22, #0
.dir_match_prefix:
    bge     r22, r20, .dir_copy_prefixed
    add     r23, r17, r22
    load.b  r25, (r23)
    add     r26, r21, r22
    load.b  r27, (r26)
    bne     r25, r27, .dir_skip_line
    add     r22, r22, #1
    bra     .dir_match_prefix
.dir_copy_prefixed:
    add     r23, r17, r20
    bra     .dir_copy_line
.dir_match_root:
    move.q  r23, r17
.dir_root_scan:
    load.b  r25, (r23)
    move.l  r26, #0x20
    beq     r25, r26, .dir_copy_root
    move.l  r26, #0x0D
    beq     r25, r26, .dir_copy_root
    move.l  r26, #0x2F
    beq     r25, r26, .dir_skip_line
    add     r23, r23, #1
    bra     .dir_root_scan
.dir_copy_root:
    move.q  r23, r17
.dir_copy_line:
    add     r26, r14, r24
.dir_copy_line_loop:
    load.b  r27, (r23)
    store.b r27, (r26)
    add     r23, r23, #1
    add     r26, r26, #1
    add     r24, r24, #1
    move.l  r28, #0x0A
    bne     r27, r28, .dir_copy_line_loop
.dir_skip_line:
    move.q  r16, r19
    add     r16, r16, #1
    bra     .dir_filter_loop
.dir_filter_done:
    beqz    r24, .dir_empty
    add     r25, r14, r24
    store.b r0, (r25)
    store.q r24, 8(sp)
    bra     .dir_print_ready

.dir_empty:
    load.q  r29, (sp)
    add     r20, r29, #32
    jsr     .dir_send_string
    bra     .dir_done

.dir_done:
    bra     .dir_cleanup_exit

.dir_send_string:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
.dir_ss_loop:
    load.b  r1, (r20)
    beqz    r1, .dir_ss_done
    store.q r20, 8(sp)
.dir_ss_retry:
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
    beq     r2, r28, .dir_ss_full
    load.q  r20, 8(sp)
    add     r20, r20, #1
    bra     .dir_ss_loop
.dir_ss_full:
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .dir_ss_retry
.dir_ss_done:
    add     sp, sp, #16
    rts

.dir_copy_zstr:
    move.q  r20, r1
    move.q  r21, r2
.dir_copy_zstr_loop:
    load.b  r22, (r20)
    store.b r22, (r21)
    beqz    r22, .dir_copy_zstr_done
    add     r20, r20, #1
    add     r21, r21, #1
    bra     .dir_copy_zstr_loop
.dir_copy_zstr_done:
    move.q  r1, r21
    rts

.dir_cleanup_exit:
    load.q  r1, 184(r29)               ; dos_library_token
    beqz    r1, .dir_exit_task
    syscall #SYS_CLOSE_LIBRARY
    load.q  r29, (sp)
    store.q r0, 184(r29)
.dir_exit_task:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

prog_dir_code_end:

prog_dir_data:
    dc.b    "console.handler", 0
    dc.b    "dos.library", 0, 0, 0, 0, 0
    dc.b    "RAM: is empty", 0x0D, 0x0A, 0
    ds.b    16
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    16
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    8
    ds.b    256
    ds.b    8                           ; 160: dos_open_sigbit
    ds.b    16                          ; 168: dos_open outcome scratch
    ds.b    8                           ; 184: dos_library_token
prog_dir_data_end:
    align   8
prog_dir_end:
