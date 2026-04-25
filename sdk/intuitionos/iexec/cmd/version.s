; ---------------------------------------------------------------------------
; VERSION — query IOSM manifests from residents or disk
; ---------------------------------------------------------------------------

prog_version:
    ; Header
    dc.l    0, 0
    dc.l    prog_version_code_end - prog_version_code
    dc.l    prog_version_data_end - prog_version_data
    dc.l    0
    ds.b    12
prog_version_code:

    sub     sp, sp, #16
.ver_preamble:
    load.q  r29, 8(sp)
    store.q r29, (sp)

.ver_open_console_retry:
    load.q  r29, (sp)
    move.q  r1, r29
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .ver_open_console_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .ver_open_console_retry
.ver_open_console_ok:
    store.q r1, 16(r29)

.ver_open_exec_retry:
    load.q  r29, (sp)
    add     r1, r29, #32
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    beqz    r2, .ver_open_exec_ok
    syscall #SYS_YIELD
    load.q  r29, (sp)
    bra     .ver_open_exec_retry
.ver_open_exec_ok:
    store.q r1, 24(r29)

    move.q  r1, r0
    move.q  r2, r0
    syscall #SYS_CREATE_PORT
    load.q  r29, (sp)
    store.q r1, 64(r29)

    move.l  r1, #4096
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    bnez    r2, .ver_exit
    store.q r1, 72(r29)
    store.q r3, 80(r29)

    move.l  r1, #4096
    move.l  r2, #0x10001
    syscall #SYS_ALLOC_MEM
    load.q  r29, (sp)
    bnez    r2, .ver_exit
    store.q r1, 88(r29)
    store.q r3, 96(r29)

    add     r14, r29, #DATA_ARGS_OFFSET
.ver_skip_ws:
    load.b  r15, (r14)
    beqz    r15, .ver_no_args
    move.l  r16, #0x20
    bne     r15, r16, .ver_have_arg
    add     r14, r14, #1
    bra     .ver_skip_ws
.ver_have_arg:
    add     r20, r29, #(prog_version_token_scratch - prog_version_data)
    move.l  r21, #0
    move.l  r22, #0
.ver_copy_arg:
    load.b  r15, (r14)
    beqz    r15, .ver_copy_arg_done
    move.l  r16, #0x20
    beq     r15, r16, .ver_copy_arg_done
    move.l  r16, #0x3A
    bne     r15, r16, .ver_copy_arg_not_colon
    move.l  r22, #1
.ver_copy_arg_not_colon:
    store.b r15, (r20)
    add     r20, r20, #1
    add     r14, r14, #1
    add     r21, r21, #1
    move.l  r16, #63
    blt     r21, r16, .ver_copy_arg
.ver_copy_arg_done:
    store.b r0, (r20)

    add     r1, r29, #(prog_version_token_scratch - prog_version_data)
    jsr     .ver_is_all
    bnez    r1, .ver_all
    bnez    r22, .ver_by_path

    add     r1, r29, #(prog_version_token_scratch - prog_version_data)
    jsr     .ver_query_resident
    beqz    r2, .ver_print_iosm_base
    jsr     .ver_try_fallbacks
    beqz    r2, .ver_print_iosm_ptr
    bra     .ver_not_found

.ver_by_path:
    add     r1, r29, #(prog_version_token_scratch - prog_version_data)
    jsr     .ver_parse_manifest
    beqz    r2, .ver_print_iosm_ptr
    bra     .ver_not_found

.ver_no_args:
    add     r1, r29, #32
    jsr     .ver_query_resident
    bnez    r2, .ver_exit
    load.q  r20, (sp)
    add     r20, r20, #112
    jsr     .ver_send_string
    load.q  r20, 72(r29)
    add     r20, r20, #IOSM_OFF_VERSION
    jsr     .ver_send_version_from
    jsr     .ver_send_crlf
    add     r20, r29, #32
    jsr     .ver_send_string
    move.l  r1, #0x20
    jsr     .ver_send_char
    load.q  r20, 72(r29)
    add     r20, r20, #IOSM_OFF_VERSION
    jsr     .ver_send_version_from
    add     r20, r29, #(prog_version_build_suffix - prog_version_data)
    jsr     .ver_send_string
    add     r20, r29, #(prog_version_copyright_line - prog_version_data)
    jsr     .ver_send_string
    bra     .ver_exit

.ver_all:
    jsr     .ver_list_residents
    bnez    r2, .ver_exit
    load.q  r29, (sp)
    store.q r0, 104(r29)
.ver_all_loop:
    load.q  r24, 72(r29)
    load.l  r25, (r24)                 ; resident count
    load.q  r26, 104(r29)              ; current index
    bge     r26, r25, .ver_all_done
    move.q  r27, r26
    add     r27, r27, #1               ; skip count header
    lsl     r27, r27, #5               ; 32-byte fixed names
    add     r1, r24, r27
    jsr     .ver_all_print_one
    load.q  r29, (sp)
    load.q  r26, 104(r29)
    add     r26, r26, #1
    store.q r26, 104(r29)
    bra     .ver_all_loop
.ver_all_done:
    bra     .ver_exit

.ver_print_iosm_base:
    load.q  r20, 72(r29)
    jsr     .ver_print_iosm
    bra     .ver_exit
.ver_print_iosm_ptr:
    jsr     .ver_print_iosm
    bra     .ver_exit

.ver_not_found:
    load.q  r29, (sp)
    add     r20, r29, #160
    jsr     .ver_send_string
    bra     .ver_exit

.ver_exit:
    move.q  r1, r0
    syscall #SYS_EXIT_TASK

; In: R1=name ptr. Out: R2=ERR_*. IOSM written at iosm buffer offset 0.
.ver_query_resident:
    sub     sp, sp, #24
    load.q  r29, 32(sp)
    store.q r29, (sp)
    store.q r1, 8(sp)
    jsr     .ver_is_iosm_port_name
    bnez    r1, .ver_qr_known_iosm
    move.q  r2, #ERR_NOTFOUND
    bra     .ver_qr_done
.ver_qr_known_iosm:
    load.q  r1, 8(sp)
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .ver_qr_done
    move.q  r8, r1
    move.q  r1, r8
    move.l  r2, #MSG_GET_IOSM
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.q  r6, 80(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .ver_qr_done
    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .ver_qr_wait_err
    move.q  r2, r2
    bra     .ver_qr_done
.ver_qr_wait_err:
    move.q  r2, r3
.ver_qr_done:
    add     sp, sp, #24
    rts

; In: R1=path ptr. Out: R2=ERR_*, R20=IOSM ptr on success.
.ver_parse_manifest:
    sub     sp, sp, #24
    load.q  r29, 32(sp)
    store.q r29, (sp)
    store.q r1, 8(sp)
    load.q  r20, 88(r29)
    move.l  r21, #0
.ver_pm_clear:
    move.l  r11, #4096
    bge     r21, r11, .ver_pm_copy
    add     r22, r20, r21
    store.q r0, (r22)
    add     r21, r21, #8
    bra     .ver_pm_clear
.ver_pm_copy:
    load.q  r14, 8(sp)
    load.q  r15, 88(r29)
    move.l  r16, #0
.ver_pm_copy_loop:
    load.b  r17, (r14)
    add     r18, r15, r16
    store.b r17, (r18)
    beqz    r17, .ver_pm_send
    add     r14, r14, #1
    add     r16, r16, #1
    move.l  r11, #(DOS_PMP_PATH_MAX - 1)
    blt     r16, r11, .ver_pm_copy_loop
    add     r18, r15, r16
    store.b r0, (r18)
.ver_pm_send:
    add     r1, r29, #(prog_version_name_dos - prog_version_data)
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .ver_pm_done
    move.l  r2, #DOS_OP_PARSE_MANIFEST
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.q  r6, 96(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .ver_pm_done
    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .ver_pm_wait_err
    load.q  r20, 88(r29)
    load.l  r2, DOS_PMP_RC_OFF(r20)
    add     r20, r20, #DOS_PMP_IOSM_OFF
    bra     .ver_pm_done
.ver_pm_wait_err:
    move.q  r2, r3
.ver_pm_done:
    add     sp, sp, #24
    rts

; Out: R2=ERR_*, R20=IOSM ptr on success.
.ver_try_fallbacks:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
    add     r1, r29, #(prog_version_prefix_libs - prog_version_data)
    jsr     .ver_build_prefixed_path
    add     r1, r29, #(prog_version_path_scratch - prog_version_data)
    jsr     .ver_parse_manifest
    beqz    r2, .ver_tf_done
    add     r1, r29, #(prog_version_prefix_devs - prog_version_data)
    jsr     .ver_build_prefixed_path
    add     r1, r29, #(prog_version_path_scratch - prog_version_data)
    jsr     .ver_parse_manifest
    beqz    r2, .ver_tf_done
    add     r1, r29, #(prog_version_prefix_resources - prog_version_data)
    jsr     .ver_build_prefixed_path
    add     r1, r29, #(prog_version_path_scratch - prog_version_data)
    jsr     .ver_parse_manifest
    beqz    r2, .ver_tf_done
    add     r1, r29, #(prog_version_prefix_l - prog_version_data)
    jsr     .ver_build_prefixed_path
    add     r1, r29, #(prog_version_path_scratch - prog_version_data)
    jsr     .ver_parse_manifest
    beqz    r2, .ver_tf_done
    add     r1, r29, #(prog_version_prefix_c - prog_version_data)
    jsr     .ver_build_prefixed_path
    add     r1, r29, #(prog_version_path_scratch - prog_version_data)
    jsr     .ver_parse_manifest
.ver_tf_done:
    add     sp, sp, #16
    rts

; In: R1=prefix ptr. Writes path scratch at data+512.
.ver_build_prefixed_path:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
    move.q  r14, r1
    add     r15, r29, #(prog_version_path_scratch - prog_version_data)
.ver_bpp_prefix:
    load.b  r16, (r14)
    beqz    r16, .ver_bpp_token
    store.b r16, (r15)
    add     r14, r14, #1
    add     r15, r15, #1
    bra     .ver_bpp_prefix
.ver_bpp_token:
    add     r14, r29, #(prog_version_token_scratch - prog_version_data)
.ver_bpp_token_loop:
    load.b  r16, (r14)
    store.b r16, (r15)
    beqz    r16, .ver_bpp_done
    add     r14, r14, #1
    add     r15, r15, #1
    bra     .ver_bpp_token_loop
.ver_bpp_done:
    add     sp, sp, #16
    rts

.ver_list_residents:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
    load.q  r1, 24(r29)
    move.l  r2, #MSG_LIST_RESIDENTS
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.q  r6, 80(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .ver_lr_done
    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .ver_lr_wait_err
    load.q  r23, 72(r29)
    store.l r2, (r23)
    move.q  r2, r0
    add     sp, sp, #16
    rts
.ver_lr_wait_err:
    move.q  r2, r3
.ver_lr_done:
    add     sp, sp, #16
    rts

.ver_all_print_one:
    sub     sp, sp, #24
    load.q  r29, 32(sp)
    store.q r29, (sp)
    store.q r1, 8(sp)
    jsr     .ver_is_iosm_port_name
    beqz    r1, .ver_apo_done
    load.q  r1, 8(sp)
    move.l  r2, #0
    syscall #SYS_FIND_PORT
    load.q  r29, (sp)
    bnez    r2, .ver_apo_done
    move.q  r8, r1
    move.q  r1, r8
    move.l  r2, #MSG_GET_IOSM
    move.q  r3, r0
    move.q  r4, r0
    load.q  r5, 64(r29)
    load.q  r6, 96(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    bnez    r2, .ver_apo_done
    load.q  r1, 64(r29)
    syscall #SYS_WAIT_PORT
    load.q  r29, (sp)
    bnez    r3, .ver_apo_done
    load.q  r20, 88(r29)
    jsr     .ver_print_iosm
.ver_apo_done:
    add     sp, sp, #24
    rts

.ver_is_all:
    load.b  r14, (r1)
    move.l  r15, #0x41
    beq     r14, r15, .ver_ia_l1
    move.l  r15, #0x61
    bne     r14, r15, .ver_ia_no
.ver_ia_l1:
    load.b  r14, 1(r1)
    move.l  r15, #0x4C
    beq     r14, r15, .ver_ia_l2
    move.l  r15, #0x6C
    bne     r14, r15, .ver_ia_no
.ver_ia_l2:
    load.b  r14, 2(r1)
    move.l  r15, #0x4C
    beq     r14, r15, .ver_ia_end
    move.l  r15, #0x6C
    bne     r14, r15, .ver_ia_no
.ver_ia_end:
    load.b  r14, 3(r1)
    bnez    r14, .ver_ia_no
    move.l  r1, #1
    rts
.ver_ia_no:
    move.q  r1, r0
    rts

.ver_is_iosm_port_name:
    sub     sp, sp, #16
    store.q r29, (sp)
    store.q r1, 8(sp)
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_exec - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_console - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_dos - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_hwres - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_input - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_graphics - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    load.q  r1, 8(sp)
    add     r2, r29, #(prog_version_name_intuition - prog_version_data)
    jsr     .ver_name_eq
    bnez    r1, .ver_iip_yes
    move.q  r1, r0
    add     sp, sp, #16
    rts
.ver_iip_yes:
    move.l  r1, #1
    add     sp, sp, #16
    rts

.ver_name_eq:
    move.q  r14, r1
    move.q  r15, r2
    move.l  r16, #0
.ver_ne_loop:
    move.l  r28, #32
    bge     r16, r28, .ver_ne_yes
    load.b  r17, (r14)
    load.b  r18, (r15)
    bne     r17, r18, .ver_ne_no
    beqz    r17, .ver_ne_yes
    add     r14, r14, #1
    add     r15, r15, #1
    add     r16, r16, #1
    bra     .ver_ne_loop
.ver_ne_yes:
    move.l  r1, #1
    rts
.ver_ne_no:
    move.q  r1, r0
    rts

; In: R20=IOSM ptr.
.ver_print_iosm:
    sub     sp, sp, #16
    load.q  r29, 24(sp)
    store.q r29, (sp)
    store.q r20, 8(sp)
    add     r20, r20, #IOSM_OFF_NAME
    move.l  r21, #IOSM_NAME_SIZE
    jsr     .ver_send_fixed_string
    move.l  r1, #0x20
    jsr     .ver_send_char
    load.q  r20, 8(sp)
    add     r20, r20, #IOSM_OFF_VERSION
    jsr     .ver_send_version_from
.ver_pi_crlf:
    jsr     .ver_send_crlf
    add     sp, sp, #16
    rts

; In: R20 points to u16 major, revision, patch.
.ver_send_version_from:
    sub     sp, sp, #16
    store.q r29, (sp)
    store.q r20, 8(sp)
    load.w  r1, (r20)
    jsr     .ver_send_decimal
    move.l  r1, #0x2E
    jsr     .ver_send_char
    load.q  r20, 8(sp)
    load.w  r1, 2(r20)
    jsr     .ver_send_decimal
    load.q  r20, 8(sp)
    load.w  r1, 4(r20)
    beqz    r1, .ver_svf_done
    move.l  r1, #0x2E
    jsr     .ver_send_char
    load.q  r20, 8(sp)
    load.w  r1, 4(r20)
    jsr     .ver_send_decimal
.ver_svf_done:
    add     sp, sp, #16
    rts

.ver_send_decimal:
    sub     sp, sp, #32
    store.q r29, (sp)
    store.w r1, 8(sp)
    store.l r0, 16(sp)                  ; emitted any leading digit

    load.w  r14, 8(sp)
    move.l  r15, #0
.ver_dec_10000_loop:
    move.l  r28, #10000
    blt     r14, r28, .ver_dec_10000_done
    sub     r14, r14, r28
    add     r15, r15, #1
    bra     .ver_dec_10000_loop
.ver_dec_10000_done:
    store.w r14, 8(sp)
    beqz    r15, .ver_dec_1000
    move.l  r16, #1
    store.l r16, 16(sp)
    move.q  r1, r15
    add     r1, r1, #0x30
    jsr     .ver_send_char

.ver_dec_1000:
    load.w  r14, 8(sp)
    move.l  r15, #0
.ver_dec_1000_loop:
    move.l  r28, #1000
    blt     r14, r28, .ver_dec_1000_done
    sub     r14, r14, r28
    add     r15, r15, #1
    bra     .ver_dec_1000_loop
.ver_dec_1000_done:
    store.w r14, 8(sp)
    load.l  r16, 16(sp)
    bnez    r16, .ver_dec_1000_emit
    beqz    r15, .ver_dec_100
.ver_dec_1000_emit:
    move.l  r16, #1
    store.l r16, 16(sp)
    move.q  r1, r15
    add     r1, r1, #0x30
    jsr     .ver_send_char

.ver_dec_100:
    load.w  r14, 8(sp)
    move.l  r15, #0
.ver_dec_100_loop:
    move.l  r28, #100
    blt     r14, r28, .ver_dec_100_done
    sub     r14, r14, r28
    add     r15, r15, #1
    bra     .ver_dec_100_loop
.ver_dec_100_done:
    store.w r14, 8(sp)
    load.l  r16, 16(sp)
    bnez    r16, .ver_dec_100_emit
    beqz    r15, .ver_dec_10
.ver_dec_100_emit:
    move.l  r16, #1
    store.l r16, 16(sp)
    move.q  r1, r15
    add     r1, r1, #0x30
    jsr     .ver_send_char

.ver_dec_10:
    load.w  r14, 8(sp)
    move.l  r15, #0
.ver_dec_10_loop:
    move.l  r28, #10
    blt     r14, r28, .ver_dec_10_done
    sub     r14, r14, r28
    add     r15, r15, #1
    bra     .ver_dec_10_loop
.ver_dec_10_done:
    store.w r14, 8(sp)
    load.l  r16, 16(sp)
    bnez    r16, .ver_dec_10_emit
    beqz    r15, .ver_dec_1
.ver_dec_10_emit:
    move.l  r16, #1
    store.l r16, 16(sp)
    move.q  r1, r15
    add     r1, r1, #0x30
    jsr     .ver_send_char

.ver_dec_1:
    load.w  r1, 8(sp)
    add     r1, r1, #0x30
    jsr     .ver_send_char
    add     sp, sp, #32
    rts

.ver_send_crlf:
    move.l  r1, #0x0D
    jsr     .ver_send_char
    move.l  r1, #0x0A
    jsr     .ver_send_char
    rts

.ver_send_fixed_string:
    sub     sp, sp, #24
    store.q r29, (sp)
    store.q r20, 8(sp)
    store.q r21, 16(sp)
.ver_sfs_loop:
    load.q  r20, 8(sp)
    load.q  r21, 16(sp)
    load.b  r1, (r20)
    beqz    r1, .ver_sfs_done
    beqz    r21, .ver_sfs_done
    jsr     .ver_send_char
    load.q  r20, 8(sp)
    load.q  r21, 16(sp)
    add     r20, r20, #1
    sub     r21, r21, #1
    store.q r20, 8(sp)
    store.q r21, 16(sp)
    bra     .ver_sfs_loop
.ver_sfs_done:
    add     sp, sp, #24
    rts

.ver_send_string:
    sub     sp, sp, #16
    store.q r29, (sp)
    store.q r20, 8(sp)
.ver_ss_loop:
    load.q  r20, 8(sp)
    load.b  r1, (r20)
    beqz    r1, .ver_ss_done
    jsr     .ver_send_char
    load.q  r20, 8(sp)
    add     r20, r20, #1
    store.q r20, 8(sp)
    bra     .ver_ss_loop
.ver_ss_done:
    add     sp, sp, #16
    rts

.ver_send_char:
    sub     sp, sp, #16
    store.q r29, (sp)
    store.b r1, 8(sp)
.ver_sc_retry:
    load.q  r29, (sp)
    load.b  r3, 8(sp)
    move.l  r2, #0
    move.q  r4, r0
    move.l  r5, #REPLY_PORT_NONE
    move.q  r6, r0
    load.q  r1, 16(r29)
    syscall #SYS_PUT_MSG
    load.q  r29, (sp)
    move.l  r28, #ERR_FULL
    beq     r2, r28, .ver_sc_full
    add     sp, sp, #16
    rts
.ver_sc_full:
    syscall #SYS_YIELD
    bra     .ver_sc_retry

prog_version_code_end:

prog_version_data:
    ; Source-level OS version marker for M16.1 audits: IntuitionOS 1.16.4
prog_version_name_console:
    dc.b    "console.handler", 0
    ds.b    16
prog_version_name_exec:
    dc.b    "exec.library", 0
    ds.b    19
    ds.b    8                           ; 64: reply port
    ds.b    8                           ; 72: iosm buffer VA
    ds.b    8                           ; 80: iosm share handle
    ds.b    8                           ; 88: parse/list buffer VA
    ds.b    8                           ; 96: parse/list share handle
    ds.b    8
    dc.b    "IntuitionOS ", 0
    ds.b    35
    dc.b    "VERSION: not found in resident ports or LIBS:, DEVS:, RESOURCES:, L:, C:", 0x0D, 0x0A, 0
prog_version_name_dos:
    dc.b    "dos.library", 0
    ds.b    14
prog_version_name_hwres:
    dc.b    "hardware.resource", 0
    ds.b    14
prog_version_name_input:
    dc.b    "input.device", 0
    ds.b    19
prog_version_name_graphics:
    dc.b    "graphics.library", 0
    ds.b    15
prog_version_name_intuition:
    dc.b    "intuition.library", 0
    ds.b    14
prog_version_prefix_libs:
    dc.b    "LIBS:", 0
    ds.b    2
prog_version_prefix_devs:
    dc.b    "DEVS:", 0
    ds.b    2
prog_version_prefix_resources:
    dc.b    "RESOURCES:", 0
    ds.b    5
prog_version_prefix_l:
    dc.b    "L:", 0
    ds.b    12
prog_version_prefix_c:
    dc.b    "C:", 0
    ds.b    189
prog_version_token_scratch:
    ds.b    256
prog_version_path_scratch:
    ds.b    256
prog_version_build_suffix:
    dc.b    " (2026-04-25)", 0x0D, 0x0A, 0
    ds.b    18
prog_version_copyright_line:
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68
    dc.b    0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32
    dc.b    0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F
    dc.b    0x74, 0x6C, 0x65, 0x79, 0x0D, 0x0A, 0
    align   8
prog_version_iosm:
    dc.l    IOSM_MAGIC
    dc.l    IOSM_SCHEMA_VERSION
    dc.b    IOSM_KIND_COMMAND
    dc.b    0
    dc.w    1
    dc.w    0
    dc.w    0
    dc.b    "Version", 0
    ds.b    IOSM_NAME_SIZE - 8
    dc.l    MODF_ASLR_CAPABLE
    dc.l    0
    dc.b    "2026-04-25", 0
    ds.b    IOSM_BUILD_DATE_SIZE - 11
    dc.b    0x43, 0x6F, 0x70, 0x79, 0x72, 0x69, 0x67, 0x68, 0x74, 0x20, 0xA9, 0x20, 0x32, 0x30, 0x32, 0x36, 0x20, 0x5A, 0x61, 0x79, 0x6E, 0x20, 0x4F, 0x74, 0x6C, 0x65, 0x79, 0
    ds.b    IOSM_COPYRIGHT_SIZE - 28
    ds.b    8
prog_version_data_end:
    align   8
prog_version_end:
