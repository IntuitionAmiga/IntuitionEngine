; ehbasic_file_io.inc - LOAD/SAVE and Detokenizer for EhBASIC IE64

; ============================================================================
; Keyword String Constants
; ============================================================================

tk_end_str:      dc.b "END", 0
tk_for_str:      dc.b "FOR", 0
tk_next_str:     dc.b "NEXT", 0
tk_data_str:     dc.b "DATA", 0
tk_input_str:    dc.b "INPUT", 0
tk_dim_str:      dc.b "DIM", 0
tk_read_str:     dc.b "READ", 0
tk_let_str:      dc.b "LET", 0
tk_dec_str:      dc.b "DEC", 0
tk_goto_str:     dc.b "GOTO", 0
tk_run_str:      dc.b "RUN", 0
tk_if_str:       dc.b "IF", 0
tk_restore_str:  dc.b "RESTORE", 0
tk_gosub_str:    dc.b "GOSUB", 0
tk_return_str:   dc.b "RETURN", 0
tk_rem_str:      dc.b "REM", 0
tk_stop_str:     dc.b "STOP", 0
tk_on_str:       dc.b "ON", 0
tk_tron_str:     dc.b "TRON", 0
tk_inc_str:      dc.b "INC", 0
tk_wait_str:     dc.b "WAIT", 0
tk_load_str:     dc.b "LOAD", 0
tk_save_str:     dc.b "SAVE", 0
tk_troff_str:    dc.b "TROFF", 0
tk_poke_str:     dc.b "POKE", 0
tk_doke_str:     dc.b "DOKE", 0
tk_loke_str:     dc.b "LOKE", 0
tk_call_str:     dc.b "CALL", 0
tk_do_str:       dc.b "DO", 0
tk_loop_str:     dc.b "LOOP", 0
tk_print_str:    dc.b "PRINT", 0
tk_cont_str:     dc.b "CONT", 0
tk_list_str:     dc.b "LIST", 0
tk_clear_str:    dc.b "CLEAR", 0
tk_new_str:      dc.b "NEW", 0
tk_width_str:    dc.b "WIDTH", 0
tk_get_str:      dc.b "GET", 0
tk_swap_str:     dc.b "SWAP", 0
tk_bitset_str:   dc.b "BITSET", 0
tk_bitclr_str:   dc.b "BITCLR", 0
tk_tab_str:      dc.b "TAB", 0
tk_to_str:       dc.b "TO", 0
tk_fn_str:       dc.b "FN", 0
tk_spc_str:      dc.b "SPC", 0
tk_then_str:     dc.b "THEN", 0
tk_not_str:      dc.b "NOT", 0
tk_step_str:     dc.b "STEP", 0
tk_until_str:    dc.b "UNTIL", 0
tk_while_str:    dc.b "WHILE", 0
tk_plus_str:     dc.b "+", 0
tk_minus_str:    dc.b "-", 0
tk_mult_str:     dc.b "*", 0
tk_div_str:      dc.b "/", 0
tk_power_str:    dc.b "^", 0
tk_and_str:      dc.b "AND", 0
tk_eor_str:      dc.b "EOR", 0
tk_or_str:       dc.b "OR", 0
tk_rshift_str:   dc.b ">>", 0
tk_lshift_str:   dc.b "<<", 0
tk_gt_str:       dc.b ">", 0
tk_equal_str:    dc.b "=", 0
tk_lt_str:       dc.b "<", 0
tk_sgn_str:      dc.b "SGN", 0
tk_int_str:      dc.b "INT", 0
tk_abs_str:      dc.b "ABS", 0
tk_usr_str:      dc.b "USR", 0
tk_fre_str:      dc.b "FRE", 0
tk_pos_str:      dc.b "POS", 0
tk_sqr_str:      dc.b "SQR", 0
tk_rnd_str:      dc.b "RND", 0
tk_log_str:      dc.b "LOG", 0
tk_exp_str:      dc.b "EXP", 0
tk_cos_str:      dc.b "COS", 0
tk_sin_str:      dc.b "SIN", 0
tk_tan_str:      dc.b "TAN", 0
tk_atn_str:      dc.b "ATN", 0
tk_peek_str:     dc.b "PEEK", 0
tk_deek_str:     dc.b "DEEK", 0
tk_leek_str:     dc.b "LEEK", 0
tk_sadd_str:     dc.b "SADD", 0
tk_len_str:      dc.b "LEN", 0
tk_strs_str:     dc.b "STR$", 0
tk_val_str:      dc.b "VAL", 0
tk_asc_str:      dc.b "ASC", 0
tk_ucases_str:   dc.b "UCASE$", 0
tk_lcases_str:   dc.b "LCASE$", 0
tk_chrs_str:     dc.b "CHR$", 0
tk_hexs_str:     dc.b "HEX$", 0
tk_bins_str:     dc.b "BIN$", 0
tk_bittst_str:   dc.b "BITTST", 0
tk_max_str:      dc.b "MAX", 0
tk_min_str:      dc.b "MIN", 0
tk_pi_str:       dc.b "PI", 0
tk_twopi_str:    dc.b "TWOPI", 0
tk_vptr_str:     dc.b "VARPTR", 0
tk_lefts_str:    dc.b "LEFT$", 0
tk_rights_str:   dc.b "RIGHT$", 0
tk_mids_str:     dc.b "MID$", 0

; Hardware extensions
tk_screen_str:   dc.b "SCREEN", 0
tk_cls_str:      dc.b "CLS", 0
tk_plot_str:     dc.b "PLOT", 0
tk_palette_str:  dc.b "PALETTE", 0
tk_vsync_str:    dc.b "VSYNC", 0
tk_locate_str:   dc.b "LOCATE", 0
tk_color_str:    dc.b "COLOR", 0
tk_line_str:     dc.b "LINE", 0
tk_circle_str:   dc.b "CIRCLE", 0
tk_box_str:      dc.b "BOX", 0
tk_scroll_str:   dc.b "SCROLL", 0
tk_copper_str:   dc.b "COPPER", 0
tk_blit_str:     dc.b "BLIT", 0
tk_sound_str:    dc.b "SOUND", 0
tk_envelope_str: dc.b "ENVELOPE", 0
tk_gate_str:     dc.b "GATE", 0
tk_ula_str:      dc.b "ULA", 0
tk_ted_str:      dc.b "TED", 0
tk_antic_str:    dc.b "ANTIC", 0
tk_gtia_str:     dc.b "GTIA", 0
tk_voodoo_str:   dc.b "VOODOO", 0
tk_psg_str:      dc.b "PSG", 0
tk_sid_str:      dc.b "SID", 0
tk_pokey_str:    dc.b "POKEY", 0
tk_ahx_str:      dc.b "AHX", 0
tk_sap_str:      dc.b "SAP", 0
tk_zbuffer_str:  dc.b "ZBUFFER", 0
tk_vertex_str:   dc.b "VERTEX", 0
tk_triangle_str: dc.b "TRIANGLE", 0
tk_texture_str:  dc.b "TEXTURE", 0

; ============================================================================
; Detokenizer Jump Table (128 pointers)
; ============================================================================

detok_table:
    dc.l tk_end_str, tk_for_str, tk_next_str, tk_data_str
    dc.l tk_input_str, tk_dim_str, tk_read_str, tk_let_str
    dc.l tk_dec_str, tk_goto_str, tk_run_str, tk_if_str
    dc.l tk_restore_str, tk_gosub_str, tk_return_str, tk_rem_str
    dc.l tk_stop_str, tk_on_str, tk_tron_str, tk_inc_str
    dc.l tk_wait_str, tk_load_str, tk_save_str, tk_troff_str
    dc.l tk_poke_str, tk_doke_str, tk_loke_str, tk_call_str
    dc.l tk_do_str, tk_loop_str, tk_print_str, tk_cont_str
    dc.l tk_list_str, tk_clear_str, tk_new_str, tk_width_str
    dc.l tk_get_str, tk_swap_str, tk_bitset_str, tk_bitclr_str
    dc.l tk_tab_str, tk_to_str, tk_fn_str, tk_spc_str
    dc.l tk_then_str, tk_not_str, tk_step_str, tk_until_str
    dc.l tk_while_str, tk_plus_str, tk_minus_str, tk_mult_str
    dc.l tk_div_str, tk_power_str, tk_and_str, tk_eor_str
    dc.l tk_or_str, tk_rshift_str, tk_lshift_str, tk_gt_str
    dc.l tk_equal_str, tk_lt_str, tk_sgn_str, tk_int_str
    dc.l tk_abs_str, tk_usr_str, tk_fre_str, tk_pos_str
    dc.l tk_sqr_str, tk_rnd_str, tk_log_str, tk_exp_str
    dc.l tk_cos_str, tk_sin_str, tk_tan_str, tk_atn_str
    dc.l tk_peek_str, tk_deek_str, tk_leek_str, tk_sadd_str
    dc.l tk_len_str, tk_strs_str, tk_val_str, tk_asc_str
    dc.l tk_ucases_str, tk_lcases_str, tk_chrs_str, tk_hexs_str
    dc.l tk_bins_str, tk_bittst_str, tk_max_str, tk_min_str
    dc.l tk_pi_str, tk_twopi_str, tk_vptr_str, tk_lefts_str
    dc.l tk_rights_str, tk_mids_str
    ; Hardware extensions ($E2-$FF)
    dc.l tk_screen_str, tk_cls_str, tk_plot_str, tk_palette_str
    dc.l tk_vsync_str, tk_locate_str, tk_color_str, tk_line_str
    dc.l tk_circle_str, tk_box_str, tk_scroll_str, tk_copper_str
    dc.l tk_blit_str, tk_sound_str, tk_envelope_str, tk_gate_str
    dc.l tk_ula_str, tk_ted_str, tk_antic_str, tk_gtia_str
    dc.l tk_voodoo_str, tk_psg_str, tk_sid_str, tk_pokey_str
    dc.l tk_ahx_str, tk_sap_str, tk_zbuffer_str, tk_vertex_str
    dc.l tk_triangle_str, tk_texture_str

; ============================================================================
; uint32_to_buf - Write decimal number to buffer
; ============================================================================
; Input:  R8 = number
;         R9 = output buffer
; Output: R8 = number of digits written
;         R9 = advanced buffer pointer
; Clobbers: R1, R2, R3, R4

uint32_to_buf:
    push    r10
    push    r14
    move.q  r10, r9                 ; save start
    
    ; Handle 0 separately
    bnez    r8, .u32_nonzero
    move.q  r1, #0x30              ; '0'
    store.b r1, (r9)
    add.q   r9, r9, #1
    bra     .u32_done

.u32_nonzero:
    ; Use stack to reverse digits
    move.q  r14, r0                 ; digit count
    move.q  r1, r8
    li      r2, #10
.u32_loop:
    beqz    r1, .u32_pop
    move.q  r4, r1                  ; r4 = original value
    divu.l  r1, r1, r2              ; r1 = r1 / 10
    mulu.l  r3, r1, r2              ; r3 = quotient * 10
    sub.l   r3, r4, r3              ; r3 = remainder
    add.q   r3, r3, #0x30           ; to ASCII
    push    r3
    add.q   r14, r14, #1
    bra     .u32_loop

.u32_pop:
    beqz    r14, .u32_done
    pop     r1
    store.b r1, (r9)
    add.q   r9, r9, #1
    sub.q   r14, r14, #1
    bra     .u32_pop

.u32_done:
    sub.q   r8, r9, r10
    pop     r14
    pop     r10
    rts

; ============================================================================
; detokenize - Convert tokens back to BASIC keywords
; ============================================================================
; Input:  R8 = pointer to tokenized line (null-terminated)
;         R9 = output buffer
; Output: R8 = output length
;         R9 = advanced buffer pointer
; Clobbers: R1-R7

detokenize:
    push    r10
    push    r11
    push    r12
    push    r13
    
    move.q  r10, r8                 ; input
    move.q  r11, r9                 ; output start
    move.q  r12, r9                 ; output write ptr
    move.q  r13, r0                 ; mode: 0=normal, 1=QUOTE, 2=REM, 3=DATA

.detok_main:
    load.b  r1, (r10)
    beqz    r1, .detok_eol
    
    ; If in REM mode, copy everything raw
    move.q  r2, #2
    beq     r13, r2, .detok_copy_raw
    
    ; If in QUOTE mode, copy until next quote
    move.q  r2, #1
    beq     r13, r2, .detok_quote_copy
    
    ; Normal or DATA mode
    move.q  r2, #0x80
    blt     r1, r2, .detok_literal
    
    ; It's a token
    sub.q   r2, r1, #0x80
    li      r3, #4
    mulu.l  r2, r2, r3
    la      r3, detok_table
    add.q   r3, r3, r2
    load.l  r4, (r3)
    beqz    r4, .detok_copy_raw      ; fallback
    
.detok_copy_kw:
    load.b  r5, (r4)
    beqz    r5, .detok_kw_done
    store.b r5, (r12)
    add.q   r12, r12, #1
    add.q   r4, r4, #1
    bra     .detok_copy_kw
    
.detok_kw_done:
    ; Change mode if needed
    move.q  r2, #TK_REM
    beq     r1, r2, .detok_set_rem
    move.q  r2, #TK_DATA
    beq     r1, r2, .detok_set_data
    
    add.q   r10, r10, #1
    bra     .detok_main

.detok_literal:
    ; Check for quote
    move.q  r2, #0x22
    beq     r1, r2, .detok_set_quote
    
    ; If in DATA mode, check for ":" to exit DATA mode
    move.q  r2, #3
    bne     r13, r2, .detok_copy_raw
    move.q  r2, #0x3A              ; ':'
    bne     r1, r2, .detok_copy_raw
    move.q  r13, r0                ; exit DATA mode
    bra     .detok_copy_raw

.detok_set_rem:
    move.q  r13, #2
    add.q   r10, r10, #1
    bra     .detok_main

.detok_set_data:
    move.q  r13, #3
    add.q   r10, r10, #1
    bra     .detok_main

.detok_set_quote:
    move.q  r13, #1
    bra     .detok_copy_raw

.detok_quote_copy:
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r10, r10, #1
    move.q  r2, #0x22
    bne     r1, r2, .detok_main
    move.q  r13, r0                ; exit QUOTE mode
    bra     .detok_main

.detok_copy_raw:
    store.b r1, (r12)
    add.q   r12, r12, #1
    add.q   r10, r10, #1
    bra     .detok_main

.detok_eol:
    store.b r0, (r12)
    sub.q   r8, r12, r11
    move.q  r9, r12                 ; advanced R9
    pop     r13
    pop     r12
    pop     r11
    pop     r10
    rts

; ============================================================================
; exec_do_save - SAVE "filename"
; ============================================================================
exec_do_save:
    jsr     exec_skip_spaces
    jsr     str_eval                ; R8 = filename pointer
    move.q  r10, r8                 ; R10 = filename
    
    ; Copy filename to FILE_NAME_BUF
    la      r1, FILE_NAME_BUF
.save_copy_name:
    load.b  r2, (r10)
    store.b r2, (r1)
    add.q   r10, r10, #1
    add.q   r1, r1, #1
    bnez    r2, .save_copy_name
    
    ; Start walking lines
    la      r11, FILE_DATA_BUF      ; R11 = output write ptr
    load.l  r14, (r16)              ; R14 = first line ptr (ST_PROG_START)
    
.save_line_loop:
    load.l  r1, (r14)               ; get next-pointer
    beqz    r1, .save_trigger       ; if NULL, this is the end sentinel
    
    ; Line number
    load.l  r8, 4(r14)
    move.q  r9, r11
    jsr     uint32_to_buf           ; R8 = len, R9 = advanced ptr
    move.q  r11, r9
    
    ; Space
    move.q  r1, #0x20
    store.b r1, (r11)
    add.q   r11, r11, #1
    
    ; Detokenize content (at r14+8)
    add.q   r8, r14, #8
    move.q  r9, r11
    jsr     detokenize              ; R8 = len, R9 = advanced ptr
    move.q  r11, r9
    
    ; Newline
    move.q  r1, #0x0A
    store.b r1, (r11)
    add.q   r11, r11, #1
    
    ; Next line
    load.l  r14, (r14)
    bra     .save_line_loop

.save_trigger:
    ; Setup MMIO
    la      r1, FILE_NAME_PTR
    la      r2, FILE_NAME_BUF
    store.l r2, (r1)
    
    la      r1, FILE_DATA_PTR
    la      r2, FILE_DATA_BUF
    store.l r2, (r1)
    
    la      r1, FILE_DATA_LEN
    la      r2, FILE_DATA_BUF
    sub.q   r2, r11, r2             ; length
    store.l r2, (r1)
    
    la      r1, FILE_CTRL
    li      r2, #2                  ; OP_WRITE
    store.l r2, (r1)
    
    ; Check status
    la      r1, FILE_STATUS
    load.l  r1, (r1)
    beqz    r1, .save_success
    
    ; Print error
    la      r8, .msg_file_error
    jsr     print_string
    jsr     print_crlf
    
.save_success:
    rts

.msg_file_error: dc.b "?FILE ERROR", 0

; ============================================================================
; exec_do_load - LOAD "filename"
; ============================================================================
exec_do_load:
    jsr     exec_skip_spaces
    jsr     str_eval                ; R8 = filename
    move.q  r10, r8
    
    ; Copy to FILE_NAME_BUF
    la      r1, FILE_NAME_BUF
.load_copy_name:
    load.b  r2, (r10)
    store.b r2, (r1)
    add.q   r10, r10, #1
    add.q   r1, r1, #1
    bnez    r2, .load_copy_name
    
    ; Setup MMIO
    la      r1, FILE_NAME_PTR
    la      r2, FILE_NAME_BUF
    store.l r2, (r1)
    
    la      r1, FILE_DATA_PTR
    la      r2, FILE_DATA_BUF
    store.l r2, (r1)
    
    la      r1, FILE_CTRL
    li      r2, #1                  ; OP_READ
    store.l r2, (r1)
    
    ; Check status
    la      r1, FILE_STATUS
    load.l  r1, (r1)
    beqz    r1, .load_process
    
    la      r8, .msg_not_found
    jsr     print_string
    jsr     print_crlf
    rts

.load_process:
    ; NEW (clear current program first)
    jsr     line_init
    jsr     var_init

    ; State management: R25=read ptr, R26=end ptr, R27=line#
    push    r25
    push    r26
    push    r27

    la      r25, FILE_DATA_BUF
    la      r1, FILE_RESULT_LEN
    load.l  r3, (r1)                ; R3 = bytes read
    la      r1, FILE_DATA_BUF
    add.q   r26, r1, r3             ; R26 = end of data

.l_line_loop:
    beq     r25, r26, .l_finished

    ; 1. Skip leading whitespace/newlines
.l_skip_ws:
    beq     r25, r26, .l_finished
    load.b  r1, (r25)
    beqz    r1, .l_finished
    move.q  r2, #32
    bgt     r1, r2, .l_parse_num
    add.q   r25, r25, #1
    bra     .l_skip_ws

    ; 2. Parse decimal line number into R27
.l_parse_num:
    move.q  r27, r0
    move.q  r10, r0                 ; digit counter
.l_num_loop:
    beq     r25, r26, .l_num_check
    load.b  r1, (r25)
    ; Check digit range: '0' (0x30) <= r1 <= '9' (0x39)
    move.q  r2, #0x30
    blt     r1, r2, .l_num_check
    move.q  r2, #0x3A
    bge     r1, r2, .l_num_check
    ; Digit value = r1 - '0'
    sub.q   r2, r1, #0x30

    ; R27 = R27 * 10 + R2
    lsl.q   r4, r27, #3
    lsl.q   r5, r27, #1
    add.q   r27, r4, r5
    add.q   r27, r27, r2
    add.q   r10, r10, #1

    add.q   r25, r25, #1
    bra     .l_num_loop

.l_num_check:
    beqz    r10, .l_skip_line       ; no digits? skip this line
    
    ; Skip spaces after line number
.l_skip_spc:
    beq     r25, r26, .l_copy_line
    load.b  r1, (r25)
    move.q  r2, #32
    bne     r1, r2, .l_copy_line
    add.q   r25, r25, #1
    bra     .l_skip_spc

    ; 3. Copy content until newline to BASIC_LINE_BUF
.l_copy_line:
    la      r12, BASIC_LINE_BUF
    move.q  r13, r12
.l_cp_loop:
    beq     r25, r26, .l_cp_done
    load.b  r1, (r25)
    move.q  r2, #10
    beq     r1, r2, .l_cp_done
    move.q  r2, #13
    beq     r1, r2, .l_cp_done
    store.b r1, (r13)
    add.q   r13, r13, #1
    add.q   r25, r25, #1
    bra     .l_cp_loop
.l_cp_done:
    store.b r0, (r13)               ; null term
    
    ; 4. Tokenize and Store
    push    r25
    push    r26
    push    r27
    la      r8, BASIC_LINE_BUF
    la      r9, 0x021800            ; temp tokenize buffer
    jsr     tokenize
    move.q  r10, r8                 ; length

    pop     r27                     ; restore line#
    move.q  r8, r27
    la      r9, 0x021800
    jsr     line_store

    pop     r26
    pop     r25
    
    ; 5. Skip trailing newline chars
.l_skip_eol:
    beq     r25, r26, .l_line_loop
    load.b  r1, (r25)
    move.q  r2, #10
    beq     r1, r2, .l_next_eol
    move.q  r2, #13
    beq     r1, r2, .l_next_eol
    bra     .l_line_loop
.l_next_eol:
    add.q   r25, r25, #1
    bra     .l_skip_eol

.l_skip_line:
    beq     r25, r26, .l_line_loop
    load.b  r1, (r25)
    add.q   r25, r25, #1
    move.q  r2, #10
    beq     r1, r2, .l_line_loop
    move.q  r2, #13
    beq     r1, r2, .l_line_loop
    bra     .l_skip_line

.l_finished:
    pop     r27
    pop     r26
    pop     r25
    ; Signal end of execution
    move.q  r28, #2
    rts

.msg_not_found: dc.b "?FILE NOT FOUND", 0