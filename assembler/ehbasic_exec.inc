; ehbasic_exec.inc - EhBASIC IE64 Statement Executor
;
; Main interpreter loop and statement handlers for the EhBASIC IE64 port.
;
; The interpreter walks through stored program lines (linked list),
; tokenizes each statement, and dispatches to the appropriate handler.
;
; Register conventions (preserved across statements):
;   R16 = interpreter state base (BASIC_STATE)
;   R17 = text pointer (current position in tokenized line content)
;   R26 = cached TERM_OUT address
;   R27 = cached TERM_STATUS address
;   R31 = hardware stack pointer
;
; Key state block fields:
;   ST_CURRENT_LINE (+0x20) = current line number
;   ST_DIRECT_MODE  (+0x40) = 1 if immediate mode, 0 if running
;   ST_GOSUB_SP     (+0x2C) = GOSUB/FOR stack pointer
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; exec_run - Execute stored BASIC program from beginning
; ============================================================================
; Input:  R16 = state base (program already stored via line_store)
; Output: Returns when END/STOP reached or program falls off last line
; Clobbers: All registers except R16, R26, R27, R31

exec_run:
    push    r14
    push    r15
    push    r22
    push    r23
    push    r24
    push    r25

    ; Clear direct mode
    add.q   r1, r16, #ST_DIRECT_MODE
    store.l r0, (r1)

    ; Initialise GOSUB stack pointer
    la      r1, BASIC_GOSUB_STACK
    add.q   r2, r16, #ST_GOSUB_SP
    store.l r1, (r2)

    ; Reset DATA/READ pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)

    ; Start at first line
    load.l  r14, (r16)           ; R14 = address of first line
    ; R14 is our "current line pointer" throughout execution

; ============================================================================
; exec_next_line - Advance to the next line and execute it
; ============================================================================
; R14 = pointer to current line entry (header: next-ptr + linenum + content)

exec_next_line:
    ; Check end of program
    beqz    r14, .exec_done

    ; Read next-line pointer. If it's 0, R14 is the end terminator — stop.
    ; Invariant: line_rechain sets every real line's next-ptr to a valid address
    ; (either the next line or the terminator). Only the terminator itself has 0.
    load.l  r15, (r14)           ; R15 = next-line pointer
    beqz    r15, .exec_done      ; R14 is the terminator — done

    ; Valid line: read line number
    add.q   r1, r14, #4
    load.l  r1, (r1)            ; R1 = line number

    ; Store current line number in state
    add.q   r2, r16, #ST_CURRENT_LINE
    store.l r1, (r2)

    ; Set text pointer to start of content (offset +8)
    add.q   r17, r14, #8

    ; TRON trace: print [linenum] if enabled (R1 = line number)
    jsr     tron_check

    ; Execute the statements on this line
    jsr     exec_line

    ; If exec_line returns 0 in R8, continue to next line normally
    ; If R8 = 1, a GOTO/GOSUB changed R14 — use the new value
    ; If R8 = 2, END was hit — stop
    move.q  r1, #2
    beq     r8, r1, .exec_done
    move.q  r1, #1
    beq     r8, r1, exec_next_line  ; R14 already updated by GOTO/GOSUB

    ; Normal: advance to next line
    ; Re-read next-ptr from R14 (R15 may have been clobbered by statement handlers)
    load.l  r14, (r14)          ; R14 = next-line pointer
    bra     exec_next_line

.exec_done:
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts

; ============================================================================
; exec_line - Execute all statements on the current line
; ============================================================================
; Input:  R17 = pointer to tokenized content (null-terminated)
;         R14 = current line pointer (for GOTO to update)
; Output: R8  = 0 (normal), 1 (GOTO/GOSUB changed R14), 2 (END/STOP)
; Clobbers: R1-R13, R15, R22-R25

exec_line:
    push    r28
    move.q  r28, r0              ; R28 = return code (0 = normal)

.el_next_stmt:
    ; Skip spaces
    jsr     exec_skip_spaces

    ; Get current token/character
    load.b  r1, (r17)
    beqz    r1, .el_done         ; null = end of line

    ; Check for ':' statement separator
    move.q  r2, #0x3A
    beq     r1, r2, .el_colon

    ; Check if it's a token (>= 0x80)
    move.q  r2, #0x80
    blt     r1, r2, .el_implied_let

    ; Token dispatch
    ; Subtract 0x80 to get index
    sub.q   r1, r1, #0x80

    ; Dispatch to handler based on token index
    ; TK_END     = 0x80 → index 0
    ; TK_FOR     = 0x81 → index 1
    ; TK_NEXT    = 0x82 → index 2
    ; TK_DATA    = 0x83 → index 3
    ; TK_INPUT   = 0x84 → index 4
    ; TK_DIM     = 0x85 → index 5
    ; TK_READ    = 0x86 → index 6
    ; TK_LET     = 0x87 → index 7
    ; ...
    ; TK_GOTO    = 0x89 → index 9
    ; TK_RUN     = 0x8A → index 10
    ; TK_IF      = 0x8B → index 11
    ; TK_RESTORE = 0x8C → index 12
    ; TK_GOSUB   = 0x8D → index 13
    ; TK_RETURN  = 0x8E → index 14
    ; TK_REM     = 0x8F → index 15
    ; TK_STOP    = 0x90 → index 16
    ; TK_PRINT   = 0x9E → index 30

    add.q   r17, r17, #1        ; consume the token byte

    ; Statement tokens are 0x80..0xA9 (END through TO)
    ; Function tokens are 0xAD+ (NOT, SGN, etc.) — not statements

    ; Check each statement token
    beqz    r1, .do_end          ; 0 = END
    move.q  r2, #1
    beq     r1, r2, .do_for     ; 1 = FOR
    move.q  r2, #2
    beq     r1, r2, .do_next    ; 2 = NEXT
    move.q  r2, #3
    beq     r1, r2, .do_data    ; 3 = DATA (skip)
    move.q  r2, #4
    beq     r1, r2, .do_input   ; 4 = INPUT
    move.q  r2, #5
    beq     r1, r2, .do_dim     ; 5 = DIM
    move.q  r2, #6
    beq     r1, r2, .do_read    ; 6 = READ
    move.q  r2, #7
    beq     r1, r2, .do_let     ; 7 = LET
    move.q  r2, #8
    beq     r1, r2, .do_dec     ; 8 = DEC
    move.q  r2, #9
    beq     r1, r2, .do_goto    ; 9 = GOTO
    move.q  r2, #11
    beq     r1, r2, .do_if      ; 11 = IF
    move.q  r2, #12
    beq     r1, r2, .do_restore ; 12 = RESTORE
    move.q  r2, #13
    beq     r1, r2, .do_gosub   ; 13 = GOSUB
    move.q  r2, #14
    beq     r1, r2, .do_return  ; 14 = RETURN
    move.q  r2, #15
    beq     r1, r2, .do_rem     ; 15 = REM
    move.q  r2, #16
    beq     r1, r2, .do_stop    ; 16 = STOP
    move.q  r2, #17
    beq     r1, r2, .do_on      ; 17 = ON
    move.q  r2, #19
    beq     r1, r2, .do_inc     ; 19 = INC
    move.q  r2, #20
    beq     r1, r2, .do_wait    ; 20 = WAIT
    move.q  r2, #24
    beq     r1, r2, .do_poke    ; 24 = POKE
    move.q  r2, #25
    beq     r1, r2, .do_doke    ; 25 = DOKE
    move.q  r2, #26
    beq     r1, r2, .do_loke    ; 26 = LOKE
    move.q  r2, #28
    beq     r1, r2, .do_do      ; 28 = DO
    move.q  r2, #29
    beq     r1, r2, .do_loop    ; 29 = LOOP
    move.q  r2, #30
    beq     r1, r2, .do_print   ; 30 = PRINT
    move.q  r2, #31
    beq     r1, r2, .do_cont    ; 31 = CONT (TK_CONT=0x9F, idx=31)
    move.q  r2, #18
    beq     r1, r2, .do_tron    ; 18 = TRON (via TK_NULL)
    move.q  r2, #21
    beq     r1, r2, .do_troff   ; 21 = TROFF (via TK_LOAD)
    move.q  r2, #27
    beq     r1, r2, .do_call    ; 27 = CALL
    move.q  r2, #33
    beq     r1, r2, .do_clear   ; 33 = CLEAR
    move.q  r2, #36
    beq     r1, r2, .do_get     ; 36 = GET
    move.q  r2, #37
    beq     r1, r2, .do_swap    ; 37 = SWAP
    move.q  r2, #38
    beq     r1, r2, .do_bitset  ; 38 = BITSET
    move.q  r2, #39
    beq     r1, r2, .do_bitclr  ; 39 = BITCLR
    move.q  r2, #47
    beq     r1, r2, .do_wend    ; 47 = WEND (tokenized as TK_UNTIL)
    move.q  r2, #48
    beq     r1, r2, .do_while   ; 48 = WHILE

    ; Hardware extension tokens (0xE2+ → index 98+)
    move.q  r2, #98
    beq     r1, r2, .do_screen  ; 98 = SCREEN
    move.q  r2, #99
    beq     r1, r2, .do_cls     ; 99 = CLS
    move.q  r2, #100
    beq     r1, r2, .do_plot    ; 100 = PLOT
    move.q  r2, #101
    beq     r1, r2, .do_palette ; 101 = PALETTE
    move.q  r2, #102
    beq     r1, r2, .do_vsync   ; 102 = VSYNC
    move.q  r2, #103
    beq     r1, r2, .do_locate  ; 103 = LOCATE
    move.q  r2, #104
    beq     r1, r2, .do_color   ; 104 = COLOR
    move.q  r2, #105
    beq     r1, r2, .do_line    ; 105 = LINE (TK_LINE_CMD=0xE9, idx=105)
    move.q  r2, #106
    beq     r1, r2, .do_circle  ; 106 = CIRCLE (TK_CIRCLE=0xEA, idx=106)
    move.q  r2, #107
    beq     r1, r2, .do_box     ; 107 = BOX (TK_BOX=0xEB, idx=107)
    move.q  r2, #108
    beq     r1, r2, .do_scroll  ; 108 = SCROLL (TK_SCROLL_CMD=0xEC, idx=108)
    move.q  r2, #111
    beq     r1, r2, .do_sound   ; 111 = SOUND (TK_SOUND=0xEF, idx=111)
    move.q  r2, #112
    beq     r1, r2, .do_envelope ; 112 = ENVELOPE (TK_ENVELOPE=0xF0, idx=112)
    move.q  r2, #113
    beq     r1, r2, .do_gate    ; 113 = GATE (TK_GATE=0xF1, idx=113)
    move.q  r2, #109
    beq     r1, r2, .do_copper  ; 109 = COPPER (TK_COPPER=0xED, idx=109)
    move.q  r2, #110
    beq     r1, r2, .do_blit   ; 110 = BLIT (TK_BLIT=0xEE, idx=110)
    move.q  r2, #114
    beq     r1, r2, .do_ula    ; 114 = ULA (TK_ULA=0xF2, idx=114)
    move.q  r2, #115
    beq     r1, r2, .do_ted_cmd ; 115 = TED (TK_TED_CMD=0xF3, idx=115)
    move.q  r2, #116
    beq     r1, r2, .do_antic  ; 116 = ANTIC (TK_ANTIC=0xF4, idx=116)
    move.q  r2, #117
    beq     r1, r2, .do_gtia   ; 117 = GTIA (TK_GTIA=0xF5, idx=117)
    move.q  r2, #118
    beq     r1, r2, .do_voodoo ; 118 = VOODOO (TK_VOODOO=0xF6, idx=118)
    move.q  r2, #119
    beq     r1, r2, .do_psg    ; 119 = PSG (TK_PSG_CMD=0xF7, idx=119)
    move.q  r2, #120
    beq     r1, r2, .do_sid    ; 120 = SID (TK_SID_CMD=0xF8, idx=120)
    move.q  r2, #121
    beq     r1, r2, .do_pokey  ; 121 = POKEY (TK_POKEY_CMD=0xF9, idx=121)
    move.q  r2, #122
    beq     r1, r2, .do_ahx    ; 122 = AHX (TK_AHX=0xFA, idx=122)
    move.q  r2, #123
    beq     r1, r2, .do_sap    ; 123 = SAP (TK_SAP=0xFB, idx=123)
    move.q  r2, #124
    beq     r1, r2, .do_zbuffer ; 124 = ZBUFFER (TK_ZBUFFER=0xFC, idx=124)
    move.q  r2, #125
    beq     r1, r2, .do_vertex ; 125 = VERTEX (TK_VERTEX=0xFD, idx=125)
    move.q  r2, #126
    beq     r1, r2, .do_triangle ; 126 = TRIANGLE (TK_TRIANGLE=0xFE, idx=126)
    move.q  r2, #127
    beq     r1, r2, .do_texture ; 127 = TEXTURE (TK_TEXTURE=0xFF, idx=127)

    ; Unknown token — skip rest of line
    bra     .el_done

.el_colon:
    ; Statement separator — advance and continue
    add.q   r17, r17, #1
    bra     .el_next_stmt

.el_implied_let:
    ; Not a token and not ':' — must be variable assignment (implied LET)
    ; Check if it's a letter (A-Z or a-z)
    move.q  r2, #0x41            ; 'A'
    blt     r1, r2, .el_done
    move.q  r2, #0x7A            ; 'z'
    bgt     r1, r2, .el_done
    ; It's a letter — implied LET (don't consume, var_lookup will read name)
    bra     .do_let

.el_done:
    move.q  r8, r28
    pop     r28
    rts

; ============================================================================
; Statement handlers
; ============================================================================

; --- END ---
.do_end:
    move.q  r28, #2
    bra     .el_done

; --- STOP ---
.do_stop:
    ; Save state for CONT
    add.q   r1, r16, #ST_CONT_LINE_PTR
    store.l r14, (r1)
    add.q   r1, r16, #ST_CONT_TEXT_PTR
    store.l r17, (r1)
    move.q  r28, #2
    bra     .el_done

; --- CONT ---
.do_cont:
    ; Restore saved execution position from STOP
    add.q   r1, r16, #ST_CONT_LINE_PTR
    load.l  r14, (r1)
    beqz    r14, .el_done        ; no saved state → ignore
    add.q   r1, r16, #ST_CONT_TEXT_PTR
    load.l  r17, (r1)
    ; Clear saved state
    add.q   r1, r16, #ST_CONT_LINE_PTR
    store.l r0, (r1)
    ; Signal exec_run to use updated R14
    move.q  r28, #1
    bra     .el_done

; --- REM ---
.do_rem:
    ; Skip rest of line (everything after REM is a comment)
    bra     .el_done

; --- DATA ---
.do_data:
    ; Skip rest of DATA statement (skip to colon or EOL)
.data_skip:
    load.b  r1, (r17)
    beqz    r1, .el_done
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    bra     .data_skip

; --- LET ---
.do_let:
    jsr     exec_skip_spaces
    ; Parse variable name and get pointer to value slot
    jsr     var_lookup           ; R8 = value slot, R9 = 0(num)/1(str)
    beqz    r8, .el_done         ; error: no variable name
    move.q  r22, r8              ; R22 = save value slot pointer
    move.q  r23, r9              ; R23 = type flag (0=num, 1=str)

    jsr     exec_skip_spaces

    ; Expect '=' token (TK_EQUAL = 0xBC)
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .let_has_eq
    ; Also check raw '='
    move.q  r2, #0x3D
    bne     r1, r2, .el_done     ; no '=' → syntax error (silently skip)
.let_has_eq:
    add.q   r17, r17, #1        ; consume '='
    jsr     exec_skip_spaces

    ; Branch on type
    bnez    r23, .let_str_rhs

    ; Numeric RHS
    jsr     expr_eval            ; R8 = FP32 result
    store.l r8, (r22)
    bra     .el_next_stmt

.let_str_rhs:
    ; String RHS
    jsr     str_eval             ; R8 = pointer to string data
    store.l r8, (r22)
    bra     .el_next_stmt

; --- PRINT ---
.do_print:
    jsr     exec_skip_spaces
    load.b  r1, (r17)

    ; PRINT with nothing → just CRLF
    beqz    r1, .print_crlf
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .print_crlf

.print_loop:
    load.b  r1, (r17)
    beqz    r1, .print_crlf      ; end of line → CRLF
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .print_crlf

    ; Check for semicolon (suppress CRLF)
    move.q  r2, #0x3B            ; ';'
    beq     r1, r2, .print_semi

    ; Check for comma (tab to next zone)
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .print_comma

    ; Check for string literal (quote)
    move.q  r2, #0x22            ; '"'
    beq     r1, r2, .print_string

    ; Check for string expression before numeric
    jsr     str_detect           ; R8 = 1 if string, 0 if not
    bnez    r8, .print_str_expr

    ; Numeric: evaluate and print
    jsr     expr_eval            ; R8 = FP32 value
    push    r14                  ; fp_print clobbers R14/R15
    push    r15
    jsr     fp_print             ; print the number
    pop     r15
    pop     r14
    bra     .print_loop

.print_str_expr:
    jsr     str_eval             ; R8 = pointer to string data
    move.q  r3, r8
.print_str_chars:
    load.b  r2, (r3)
    beqz    r2, .print_loop
    store.b r2, (r26)
    add.q   r3, r3, #1
    bra     .print_str_chars

.print_semi:
    add.q   r17, r17, #1        ; consume ';'
    ; Check if end of line after semicolon → suppress CRLF
    load.b  r1, (r17)
    beqz    r1, .print_no_crlf
    move.q  r2, #0x3A
    beq     r1, r2, .print_no_crlf
    bra     .print_loop

.print_comma:
    add.q   r17, r17, #1        ; consume ','
    ; Print tab (spaces to next tab stop)
    move.q  r8, #0x09            ; TAB character
    jsr     putchar
    bra     .print_loop

.print_string:
    ; Print quoted string literal
    add.q   r17, r17, #1        ; skip opening '"'
.print_str_loop:
    load.b  r1, (r17)
    beqz    r1, .print_no_crlf   ; unterminated string → end
    move.q  r2, #0x22            ; closing '"'
    beq     r1, r2, .print_str_end
    ; Output the character
    store.b r1, (r26)
    add.q   r17, r17, #1
    bra     .print_str_loop
.print_str_end:
    add.q   r17, r17, #1        ; skip closing '"'
    bra     .print_loop

.print_crlf:
    jsr     print_crlf
.print_no_crlf:
    bra     .el_next_stmt

; --- GOTO ---
.do_goto:
    jsr     exec_skip_spaces
    ; Parse line number (numeric literal)
    jsr     exec_parse_linenum   ; R8 = line number
    ; Search for the target line
    jsr     exec_find_line       ; R14 = new line pointer (or 0)
    beqz    r14, .el_done        ; line not found → error (silently stop)
    move.q  r28, #1              ; signal: R14 was changed
    bra     .el_done

; --- GOSUB ---
.do_gosub:
    jsr     exec_skip_spaces
    ; Save return context on GOSUB stack
    ; Stack frame: [return line ptr (4)] [return text ptr offset (4)] [marker (4)]
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)            ; R2 = current GOSUB stack pointer

    ; Push marker (0x47535542 = "GSUB")
    move.l  r3, #0x47535542
    store.l r3, (r2)
    add.q   r2, r2, #4
    ; Push return line pointer (R14 = current line)
    store.l r14, (r2)
    add.q   r2, r2, #4
    ; Push return text offset (R17 - current line - 8 = offset within content)
    sub.q   r3, r17, r14
    store.l r3, (r2)
    add.q   r2, r2, #4
    ; Update GOSUB SP
    store.l r2, (r1)

    ; Now do the GOTO part
    jsr     exec_parse_linenum   ; R8 = target line number
    jsr     exec_find_line       ; R14 = target line pointer
    beqz    r14, .el_done        ; not found → error
    move.q  r28, #1              ; R14 changed
    bra     .el_done

; --- RETURN ---
.do_return:
    ; Pop GOSUB stack
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)            ; R2 = GOSUB stack pointer

    ; Pop text offset
    sub.q   r2, r2, #4
    load.l  r3, (r2)            ; R3 = return text offset
    ; Pop line pointer
    sub.q   r2, r2, #4
    load.l  r14, (r2)           ; R14 = return line pointer
    ; Pop marker
    sub.q   r2, r2, #4
    load.l  r4, (r2)            ; R4 = marker (should be "GSUB")
    ; Update GOSUB SP
    store.l r2, (r1)

    ; Restore R17 to the saved position
    add.q   r17, r14, r3        ; R17 = line ptr + text offset

    ; After RETURN, execution continues on the NEXT statement
    ; The text pointer is pointing just past the GOSUB call's line number
    ; We need to advance to the next statement
    ; Skip to ':' or EOL
.return_skip:
    load.b  r1, (r17)
    beqz    r1, .return_next_line
    move.q  r2, #0x3A
    beq     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    bra     .return_skip

.return_next_line:
    ; End of the GOSUB's line — advance to next line
    load.l  r15, (r14)           ; R15 = next line pointer from saved line
    move.q  r14, r15
    move.q  r28, #1              ; R14 changed
    bra     .el_done

; --- IF ---
.do_if:
    jsr     exec_skip_spaces
    ; Evaluate condition expression
    jsr     expr_eval            ; R8 = FP32 condition value

    ; Check if true (non-zero): any FP32 with exponent ≠ 0 and value ≠ 0
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .if_false        ; value is ±0.0 → false

    ; Condition is TRUE — look for THEN and execute what follows
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_THEN
    bne     r1, r2, .el_next_stmt  ; no THEN → just continue
    add.q   r17, r17, #1        ; consume THEN

    ; Check if what follows THEN is a line number (GOTO shorthand)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .el_next_stmt  ; not a digit → execute as statement
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .el_next_stmt  ; not a digit → execute as statement
    ; It's a line number — implicit GOTO
    bra     .do_goto

.if_false:
    ; Condition is FALSE.
    ; First, skip forward to the THEN token (the one for THEN, not ELSE).
    ; Both THEN and ELSE tokenize to TK_THEN in EhBASIC.
.if_find_then:
    load.b  r1, (r17)
    beqz    r1, .el_done          ; no THEN found → end of line
    move.q  r2, #TK_THEN
    beq     r1, r2, .if_past_then
    add.q   r17, r17, #1
    bra     .if_find_then

.if_past_then:
    add.q   r17, r17, #1         ; consume the THEN token
    ; Now scan for ELSE (a second TK_THEN) or end of line.
    ; Everything between here and ELSE is the THEN-clause (skipped).
.if_seek_else:
    load.b  r1, (r17)
    beqz    r1, .el_done          ; no ELSE → done, produce no output
    move.q  r2, #TK_THEN
    beq     r1, r2, .if_else_found
    add.q   r17, r17, #1
    bra     .if_seek_else

.if_else_found:
    add.q   r17, r17, #1         ; consume ELSE marker (TK_THEN)
    bra     .el_next_stmt         ; execute ELSE clause

; --- FOR ---
.do_for:
    jsr     exec_skip_spaces

    ; Get loop variable
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .el_done
    move.q  r22, r8              ; R22 = var value slot pointer

    jsr     exec_skip_spaces

    ; Expect '=' (TK_EQUAL or raw '=')
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .for_eq
    move.q  r2, #0x3D
    bne     r1, r2, .el_done
.for_eq:
    add.q   r17, r17, #1

    ; Evaluate initial value
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = FP32 initial value
    store.l r8, (r22)            ; store in loop variable

    jsr     exec_skip_spaces

    ; Expect TO token
    load.b  r1, (r17)
    move.q  r2, #TK_TO
    bne     r1, r2, .el_done
    add.q   r17, r17, #1

    ; Evaluate limit value
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = FP32 limit
    move.q  r23, r8              ; R23 = limit value

    ; Check for STEP
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_STEP
    bne     r1, r2, .for_default_step

    ; Has STEP
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = FP32 step value
    move.q  r24, r8              ; R24 = step
    bra     .for_push

.for_default_step:
    move.l  r24, #0x3F800000     ; default STEP = 1.0

.for_push:
    ; Push FOR frame on GOSUB stack
    ; Frame: [marker "FOR " (4)] [var ptr (4)] [limit FP32 (4)]
    ;        [step FP32 (4)] [line ptr (4)] [text ptr (4)] = 24 bytes
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)

    move.l  r3, #0x464F5220      ; "FOR "
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r22, (r2)             ; var pointer
    add.q   r2, r2, #4
    store.l r23, (r2)             ; limit
    add.q   r2, r2, #4
    store.l r24, (r2)             ; step
    add.q   r2, r2, #4
    store.l r14, (r2)             ; line pointer (for loop back)
    add.q   r2, r2, #4
    ; Save text ptr: we need the position AFTER the entire FOR statement
    ; R17 currently points past the FOR line. When NEXT loops back,
    ; we advance to the next line (R14's next pointer).
    store.l r17, (r2)
    add.q   r2, r2, #4

    ; Update GOSUB SP
    store.l r2, (r1)

    ; Continue execution normally (body of loop follows)
    bra     .el_next_stmt

; --- NEXT ---
.do_next:
    ; Find the most recent FOR frame on GOSUB stack
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)            ; R2 = top of GOSUB stack

    ; Walk back to find FOR marker
    sub.q   r2, r2, #24         ; back up 24 bytes (FOR frame size)
    load.l  r3, (r2)            ; should be "FOR " marker
    move.l  r4, #0x464F5220
    bne     r3, r4, .el_done     ; not FOR → error (silently)

    ; Read FOR frame
    add.q   r3, r2, #4
    load.l  r22, (r3)           ; R22 = var pointer
    add.q   r3, r3, #4
    load.l  r23, (r3)           ; R23 = limit (FP32)
    add.q   r3, r3, #4
    load.l  r24, (r3)           ; R24 = step (FP32)
    add.q   r3, r3, #4
    load.l  r25, (r3)           ; R25 = loop line pointer
    add.q   r3, r3, #4
    load.l  r15, (r3)           ; R15 = saved text ptr

    ; Add STEP to loop variable
    load.l  r8, (r22)           ; current value
    move.q  r9, r24             ; step
    jsr     fp_add              ; R8 = value + step
    store.l r8, (r22)           ; store back

    ; Compare with limit
    ; If step > 0: continue while value <= limit
    ; If step < 0: continue while value >= limit
    move.q  r9, r23             ; R9 = limit
    jsr     fp_cmp              ; R8 = -1/0/1

    ; Check step sign
    lsr.l   r3, r24, #31        ; R3 = sign bit of step (0=positive, 1=negative)
    bnez    r3, .next_neg_step

    ; Positive step: loop while value <= limit (cmp returns -1 or 0)
    ; R8 = 1 means value > limit → done
    move.q  r3, #1
    beq     r8, r3, .next_done
    bra     .next_loop_back

.next_neg_step:
    ; Negative step: loop while value >= limit (cmp returns 1 or 0)
    ; R8 = -1 (0xFFFFFFFF) means value < limit → done
    move.q  r3, #0xFFFFFFFF
    beq     r8, r3, .next_done

.next_loop_back:
    ; Loop continues: jump back to the line after FOR
    ; R25 = line pointer of the FOR line
    ; We need to go to the NEXT line after the FOR line
    load.l  r14, (r25)          ; R14 = next line after FOR line
    move.q  r28, #1              ; signal R14 changed
    bra     .el_done

.next_done:
    ; Loop complete: pop FOR frame from GOSUB stack
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #24
    store.l r2, (r1)
    ; Continue to next statement
    bra     .el_next_stmt

; --- WHILE ---
.do_while:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = condition
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .while_false

    ; TRUE: push WHILE frame [marker "WHIL"][line ptr]
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    move.l  r3, #0x5748494C      ; "WHIL"
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r14, (r2)            ; line pointer of WHILE
    add.q   r2, r2, #4
    store.l r2, (r1)
    bra     .el_next_stmt

.while_false:
    ; FALSE: scan forward to matching WEND (nested — tracks WHILE/WEND depth)
    move.q  r7, #1               ; nesting depth = 1 (we are inside this WHILE)
    load.l  r2, (r14)            ; start from next line
.while_scan_lines:
    beqz    r2, .el_done
    load.l  r3, (r2)             ; next pointer
    beqz    r3, .el_done         ; terminator
    add.q   r4, r2, #8           ; content ptr

    ; skip leading spaces
.while_lead_space:
    load.b  r5, (r4)
    move.q  r6, #0x20
    bne     r5, r6, .while_check_token
    add.q   r4, r4, #1
    bra     .while_lead_space

.while_check_token:
    ; Check for nested WHILE (increases depth)
    move.q  r6, #TK_WHILE
    bne     r5, r6, .while_check_wend
    add.q   r7, r7, #1
    bra     .while_next_line

.while_check_wend:
    move.q  r6, #TK_UNTIL        ; WEND token
    bne     r5, r6, .while_next_line
    sub.q   r7, r7, #1
    bnez    r7, .while_next_line  ; not our WEND yet

    ; depth=0 → found matching WEND. Continue after this line.
    load.l  r14, (r2)
    move.q  r28, #1
    bra     .el_done

.while_next_line:
    move.q  r2, r3
    bra     .while_scan_lines

; --- WEND ---
.do_wend:
    ; Pop latest WHILE frame and jump back to WHILE line.
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #8
    load.l  r3, (r2)             ; marker
    move.l  r4, #0x5748494C      ; "WHIL"
    bne     r3, r4, .el_done
    add.q   r5, r2, #4
    load.l  r14, (r5)            ; WHILE line pointer
    store.l r2, (r1)             ; pop frame
    move.q  r28, #1
    bra     .el_done

; --- RESTORE ---
.do_restore:
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)            ; reset DATA pointer to 0
    bra     .el_next_stmt

; --- INC ---
.do_inc:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .el_done
    move.q  r22, r8
    load.l  r8, (r22)           ; current FP32 value
    move.l  r9, #0x3F800000     ; 1.0
    jsr     fp_add
    store.l r8, (r22)
    bra     .el_next_stmt

; --- DEC ---
.do_dec:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .el_done
    move.q  r22, r8
    load.l  r8, (r22)           ; current FP32 value
    move.l  r9, #0x3F800000     ; 1.0
    jsr     fp_sub
    store.l r8, (r22)
    bra     .el_next_stmt

; --- DOKE ---
.do_doke:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = value (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer value
    store.w r8, (r22)            ; write 16-bit to address
    bra     .el_next_stmt

; --- LOKE ---
.do_loke:
    ; LOKE is identical to POKE (32-bit write)
    bra     .do_poke

; --- CLEAR ---
.do_clear:
    jsr     var_init             ; reset all variables
    ; Reset DATA pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)
    ; Reset GOSUB stack
    la      r1, BASIC_GOSUB_STACK
    add.q   r2, r16, #ST_GOSUB_SP
    store.l r1, (r2)
    bra     .el_next_stmt

; --- SWAP ---
.do_swap:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to first var
    beqz    r8, .el_done
    move.q  r22, r8              ; R22 = first var ptr
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to second var
    beqz    r8, .el_done
    move.q  r23, r8              ; R23 = second var ptr
    ; Exchange values
    load.l  r1, (r22)
    load.l  r2, (r23)
    store.l r2, (r22)
    store.l r1, (r23)
    bra     .el_next_stmt

; --- BITSET ---
.do_bitset:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = bit number (0-7)
    jsr     fp_int
    jsr     fp_fix
    ; Create mask: 1 << bit
    move.q  r1, #1
    lsl.l   r1, r1, r8
    ; Set bit
    load.b  r2, (r22)
    or.l    r2, r2, r1
    store.b r2, (r22)
    bra     .el_next_stmt

; --- BITCLR ---
.do_bitclr:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = bit number (0-7)
    jsr     fp_int
    jsr     fp_fix
    ; Create mask: ~(1 << bit)
    move.q  r1, #1
    lsl.l   r1, r1, r8
    move.q  r3, #0xFF
    eor.l   r1, r1, r3           ; invert within byte
    ; Clear bit
    load.b  r2, (r22)
    and.l   r2, r2, r1
    store.b r2, (r22)
    bra     .el_next_stmt

; --- GET ---
.do_get:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to value slot, R9 = type
    beqz    r8, .el_done
    move.q  r22, r8              ; R22 = value slot
    move.q  r23, r9              ; R23 = type (0=num, 1=str)
    ; Check if char available
    load.l  r1, (r27)           ; TERM_STATUS
    and.l   r1, r1, #1          ; bit 0 = char available
    beqz    r1, .get_empty
    ; Read char
    add.q   r3, r27, #4         ; TERM_IN = TERM_STATUS + 4
    load.b  r8, (r3)
    and.l   r8, r8, #0xFF
    ; If string var, store as single-char string
    bnez    r23, .get_str
    ; Numeric: store ASCII code as FP32
    jsr     fp_float
    store.l r8, (r22)
    bra     .el_next_stmt
.get_str:
    ; Allocate a 2-byte string (char + null)
    add.q   r1, r16, #ST_HEAP_TOP
    load.l  r3, (r1)
    store.b r8, (r3)             ; the character
    add.q   r4, r3, #1
    store.b r0, (r4)             ; null terminator
    store.l r3, (r22)            ; store pointer in var
    add.q   r3, r3, #2
    store.l r3, (r1)             ; advance heap
    bra     .el_next_stmt
.get_empty:
    ; No char available: store 0 (num) or "" (str)
    bnez    r23, .get_empty_str
    store.l r0, (r22)            ; 0
    bra     .el_next_stmt
.get_empty_str:
    add.q   r1, r16, #ST_HEAP_TOP
    load.l  r3, (r1)
    store.b r0, (r3)             ; empty string (just null)
    store.l r3, (r22)
    add.q   r3, r3, #1
    store.l r3, (r1)
    bra     .el_next_stmt

; --- ON ---
.do_on:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = selector (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer selector (1-based)
    move.q  r22, r8              ; R22 = selector
    jsr     exec_skip_spaces
    ; Check for GOTO or GOSUB token
    load.b  r1, (r17)
    move.q  r2, #TK_GOTO
    beq     r1, r2, .on_goto
    move.q  r2, #TK_GOSUB
    beq     r1, r2, .on_gosub
    ; Neither → error
    bra     .el_done
.on_goto:
    add.q   r17, r17, #1         ; consume GOTO token
    jsr     exec_skip_spaces
    move.q  r23, #1              ; R23 = current count
.on_goto_scan:
    beq     r23, r22, .on_goto_target
    jsr     exec_skip_number
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .el_next_stmt  ; out of range → fall through
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    add.q   r23, r23, #1
    bra     .on_goto_scan
.on_goto_target:
    bra     .do_goto             ; parse line number and GOTO
.on_gosub:
    add.q   r17, r17, #1         ; consume GOSUB token
    jsr     exec_skip_spaces
    move.q  r23, #1
.on_gosub_scan:
    beq     r23, r22, .on_gosub_target
    jsr     exec_skip_number
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .el_next_stmt  ; out of range → fall through
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    add.q   r23, r23, #1
    bra     .on_gosub_scan
.on_gosub_target:
    bra     .do_gosub            ; parse line number and GOSUB

; --- DO ---
.do_do:
    ; Push DO frame on GOSUB stack: ["DO  "][line ptr] = 8 bytes
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    move.l  r3, #0x444F2020      ; "DO  "
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r14, (r2)            ; line pointer of DO statement
    add.q   r2, r2, #4
    store.l r2, (r1)             ; update SP
    bra     .el_next_stmt

; --- LOOP ---
.do_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_WHILE
    beq     r1, r2, .loop_while
    move.q  r2, #TK_UNTIL
    beq     r1, r2, .loop_until
    ; Bare LOOP — unconditional loop back to DO
    bra     .loop_back
.loop_while:
    add.q   r17, r17, #1         ; consume WHILE
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = condition
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .loop_exit       ; false → exit
    bra     .loop_back           ; true → loop
.loop_until:
    add.q   r17, r17, #1         ; consume UNTIL
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = condition
    and.l   r1, r8, #0x7FFFFFFF
    bnez    r1, .loop_exit       ; true → exit
    ; false → loop
.loop_back:
    ; Pop DO frame, jump back to DO line (which re-pushes)
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #8           ; DO frame = 8 bytes
    load.l  r3, (r2)             ; marker
    move.l  r4, #0x444F2020
    bne     r3, r4, .el_done     ; not DO → error
    add.q   r5, r2, #4
    load.l  r14, (r5)            ; DO line pointer
    store.l r2, (r1)             ; pop frame
    move.q  r28, #1              ; R14 changed
    bra     .el_done
.loop_exit:
    ; Pop DO frame and continue
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #8
    store.l r2, (r1)
    bra     .el_next_stmt

; --- INPUT ---
.do_input:
    ; Minimal INPUT implementation: INPUT A[,B,...]
    ; Reads one line per variable from terminal and parses numeric expression.
.input_loop:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = value slot pointer
    beqz    r8, .el_done
    move.q  r22, r8              ; save value slot pointer
    move.q  r23, r17             ; save statement cursor

    ; Read line into BASIC_LINE_BUF and parse as expression
    la      r8, BASIC_LINE_BUF
    move.q  r9, #80
    jsr     read_line
    la      r17, BASIC_LINE_BUF
    jsr     expr_eval            ; R8 = FP32 value
    store.l r8, (r22)

    ; Restore statement cursor and continue list parsing
    move.q  r17, r23
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    bra     .input_loop

; --- DIM ---
.do_dim:
    jsr     arr_dim
    bra     .el_next_stmt

; --- POKE ---
; POKE addr, value  — write 32-bit value to addr (matches PEEK which reads 32-bit)
.do_poke:
    ; Check for POKE8 (byte-level access)
    load.b  r1, (r17)
    move.q  r2, #0x38            ; '8'
    move.q  r23, r0              ; R23 = 0 (32-bit mode)
    bne     r1, r2, .poke_parse
    move.q  r23, #1              ; R23 = 1 (byte mode)
    add.q   r17, r17, #1         ; consume '8'
.poke_parse:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address (FP32)
    jsr     fp_int               ; R8 = INT(addr) FP32
    jsr     fp_fix               ; R8 = integer
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1         ; consume ','
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = value (FP32)
    jsr     fp_int               ; R8 = INT(value) FP32
    jsr     fp_fix               ; R8 = integer
    beqz    r23, .poke_32bit
    store.b r8, (r22)            ; write byte to address
    bra     .el_next_stmt
.poke_32bit:
    store.l r8, (r22)            ; write 32-bit to address
    bra     .el_next_stmt

; --- READ ---
.do_read:
    ; READ A[,B,...] from DATA statements in program text
.read_loop:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = value slot pointer
    beqz    r8, .el_done
    move.q  r22, r8
    push    r17                  ; save text pointer (clobbered by data scanner)
    jsr     exec_read_next_data_value ; R8 = FP32 value
    pop     r17                  ; restore text pointer
    store.l r8, (r22)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .el_next_stmt
    add.q   r17, r17, #1
    bra     .read_loop

; --- SCREEN ---
.do_screen:
    jsr     hw_screen
    bra     .el_next_stmt

; --- CLS ---
.do_cls:
    jsr     hw_cls
    bra     .el_next_stmt

; --- PLOT ---
.do_plot:
    jsr     hw_plot
    bra     .el_next_stmt

; --- PALETTE ---
.do_palette:
    jsr     hw_palette
    bra     .el_next_stmt

; --- VSYNC ---
.do_vsync:
    jsr     hw_vsync
    bra     .el_next_stmt

; --- LOCATE ---
.do_locate:
    jsr     hw_locate
    bra     .el_next_stmt

; --- COLOR ---
.do_color:
    jsr     hw_color
    bra     .el_next_stmt

; --- LINE ---
.do_line:
    jsr     hw_line
    bra     .el_next_stmt

; --- CIRCLE ---
.do_circle:
    jsr     hw_circle
    bra     .el_next_stmt

; --- BOX ---
.do_box:
    jsr     hw_box
    bra     .el_next_stmt

; --- SCROLL ---
.do_scroll:
    jsr     hw_scroll
    bra     .el_next_stmt

; --- SOUND ---
.do_sound:
    jsr     hw_sound
    bra     .el_next_stmt

; --- ENVELOPE ---
.do_envelope:
    jsr     hw_envelope
    bra     .el_next_stmt

; --- GATE ---
.do_gate:
    jsr     hw_gate
    bra     .el_next_stmt

; --- WAIT ---
.do_wait:
    jsr     hw_wait
    bra     .el_next_stmt

; --- COPPER ---
.do_copper:
    jsr     hw_copper
    bra     .el_next_stmt

; --- BLIT ---
.do_blit:
    jsr     hw_blit
    bra     .el_next_stmt

; --- ULA ---
.do_ula:
    jsr     hw_ula
    bra     .el_next_stmt

; --- TED ---
.do_ted_cmd:
    jsr     hw_ted_cmd
    bra     .el_next_stmt

; --- ANTIC ---
.do_antic:
    jsr     hw_antic
    bra     .el_next_stmt

; --- GTIA ---
.do_gtia:
    jsr     hw_gtia
    bra     .el_next_stmt

; --- VOODOO ---
.do_voodoo:
    jsr     hw_voodoo
    bra     .el_next_stmt

; --- PSG ---
.do_psg:
    jsr     hw_psg
    bra     .el_next_stmt

; --- SID ---
.do_sid:
    jsr     hw_sid
    bra     .el_next_stmt

; --- POKEY ---
.do_pokey:
    jsr     hw_pokey
    bra     .el_next_stmt

; --- AHX ---
.do_ahx:
    jsr     hw_ahx
    bra     .el_next_stmt

; --- SAP ---
.do_sap:
    jsr     hw_sap
    bra     .el_next_stmt

; --- ZBUFFER ---
.do_zbuffer:
    jsr     hw_zbuffer
    bra     .el_next_stmt

; --- VERTEX ---
.do_vertex:
    jsr     hw_vertex
    bra     .el_next_stmt

; --- TRIANGLE ---
.do_triangle:
    jsr     hw_triangle
    bra     .el_next_stmt

; --- TEXTURE ---
.do_texture:
    jsr     hw_texture
    bra     .el_next_stmt

; --- CALL ---
.do_call:
    ; CALL addr — JSR to machine code at given address
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = FP32 address
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
    move.q  r9, r8               ; R9 = call target
    ; Save BASIC state registers across machine-language call
    push    r14
    push    r16
    push    r17
    push    r26
    jsr     (r9)                 ; register-indirect JSR
    pop     r26
    pop     r17
    pop     r16
    pop     r14
    bra     .el_next_stmt

; --- TRON ---
.do_tron:
    ; TRON — enable trace mode (print line numbers during execution)
    add.q   r1, r16, #ST_TRACE_FLAG
    move.q  r2, #1
    store.l r2, (r1)
    bra     .el_next_stmt

; --- TROFF ---
.do_troff:
    ; TROFF — disable trace mode
    add.q   r1, r16, #ST_TRACE_FLAG
    store.l r0, (r1)
    bra     .el_next_stmt

; ============================================================================
; Helper: exec_skip_spaces
; ============================================================================
exec_skip_spaces:
    load.b  r1, (r17)
    move.q  r2, #0x20            ; space
    bne     r1, r2, .ess_done
    add.q   r17, r17, #1
    bra     exec_skip_spaces
.ess_done:
    rts

; ============================================================================
; Helper: exec_parse_linenum - Parse a decimal line number from token stream
; ============================================================================
; Input:  R17 = text pointer
; Output: R8  = line number (integer)
;         R17 = advanced past number
; Clobbers: R1-R3

exec_parse_linenum:
    move.q  r8, r0               ; accumulator = 0

.epln_loop:
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .epln_done
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .epln_done
    sub.q   r1, r1, #0x30       ; digit value
    mulu.l  r8, r8, #10
    add.l   r8, r8, r1
    add.q   r17, r17, #1
    bra     .epln_loop

.epln_done:
    rts

; ============================================================================
; Helper: exec_skip_number - Skip decimal digits at R17
; ============================================================================
; Input:  R17 = text pointer
; Output: R17 = advanced past digits
; Clobbers: R1, R2

exec_skip_number:
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .esn_done
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .esn_done
    add.q   r17, r17, #1
    bra     exec_skip_number
.esn_done:
    rts

; ============================================================================
; Helper: exec_find_line - Find a line by number in the program
; ============================================================================
; Input:  R8 = line number to find
; Output: R14 = pointer to the line (0 if not found)
; Clobbers: R1-R3

exec_find_line:
    ; Start from beginning of program
    load.l  r1, (r16)           ; R1 = first line pointer

.efl_loop:
    beqz    r1, .efl_not_found
    ; Read line number at R1+4
    add.q   r2, r1, #4
    load.l  r2, (r2)
    beq     r2, r8, .efl_found
    bgt     r2, r8, .efl_not_found  ; passed it
    ; Advance to next line
    load.l  r1, (r1)            ; follow next pointer
    bra     .efl_loop

.efl_found:
    move.q  r14, r1
    rts

.efl_not_found:
    move.q  r14, r0
    rts

; ============================================================================
; Helper: exec_read_next_data_value
; ============================================================================
; Uses ST_DATA_PTR as the count of values already consumed by READ.
; Input:  R16 = state base
; Output: R8  = next DATA value as FP32 (0 if unavailable)
; Clobbers: R1-R15, R22-R25

exec_read_next_data_value:
    ; target index = state[ST_DATA_PTR]
    add.q   r1, r16, #ST_DATA_PTR
    load.l  r6, (r1)             ; R6 = target data index
    push    r6                   ; save target index
    jsr     exec_find_data_value_by_index
    ; consume one slot regardless (keeps READ monotonic)
    pop     r6                   ; restore target index
    add.q   r6, r6, #1
    add.q   r1, r16, #ST_DATA_PTR  ; recompute (R1 was clobbered)
    store.l r6, (r1)
    rts

; ============================================================================
; Helper: exec_find_data_value_by_index
; ============================================================================
; Input:  R6 = target DATA element index (0-based)
; Output: R8 = FP32 value (0 if not found)
; Clobbers: R1-R15, R22-R25

exec_find_data_value_by_index:
    push    r14
    push    r15

    move.q  r7, r0               ; R7 = current data index
    load.l  r10, (r16)           ; R10 = first line pointer

.fd_line_loop:
    beqz    r10, .fd_not_found
    load.l  r11, (r10)           ; next line pointer
    beqz    r11, .fd_not_found   ; terminator reached

    add.q   r12, r10, #8         ; R12 = content pointer
.fd_stmt_loop:
    load.b  r1, (r12)
    beqz    r1, .fd_next_line
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .fd_after_colon
    move.q  r2, #TK_DATA
    beq     r1, r2, .fd_data_stmt
    add.q   r12, r12, #1
    bra     .fd_stmt_loop

.fd_after_colon:
    add.q   r12, r12, #1
    bra     .fd_stmt_loop

.fd_data_stmt:
    add.q   r12, r12, #1         ; skip TK_DATA

.fd_values_loop:
    ; skip spaces
.fd_skip_spaces:
    load.b  r1, (r12)
    move.q  r2, #0x20
    bne     r1, r2, .fd_value_start
    add.q   r12, r12, #1
    bra     .fd_skip_spaces

.fd_value_start:
    load.b  r1, (r12)
    beqz    r1, .fd_next_line
    move.q  r2, #0x3A
    beq     r1, r2, .fd_after_colon

    ; Optional sign for numeric literal
    move.q  r13, r0              ; sign flag (0=+, 1=-)
    move.q  r2, #0x2D            ; '-'
    beq     r1, r2, .fd_neg_sign
    move.q  r2, #TK_MINUS
    beq     r1, r2, .fd_neg_sign
    move.q  r2, #0x2B            ; '+'
    beq     r1, r2, .fd_pos_sign
    move.q  r2, #TK_PLUS
    bne     r1, r2, .fd_parse_value
.fd_pos_sign:
    add.q   r12, r12, #1
    bra     .fd_parse_value
.fd_neg_sign:
    add.q   r12, r12, #1
    move.q  r13, #1

.fd_parse_value:
    move.q  r17, r12
    jsr     parse_number          ; R8 = FP32, R17 advanced
    move.q  r12, r17

    ; apply sign if needed
    beqz    r13, .fd_check_index
    jsr     fp_neg

.fd_check_index:
    beq     r7, r6, .fd_found
    add.q   r7, r7, #1

    ; skip spaces after value
.fd_post_spaces:
    load.b  r1, (r12)
    move.q  r2, #0x20
    bne     r1, r2, .fd_after_value
    add.q   r12, r12, #1
    bra     .fd_post_spaces

.fd_after_value:
    load.b  r1, (r12)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .fd_stmt_end
    add.q   r12, r12, #1
    bra     .fd_values_loop

.fd_stmt_end:
    ; End of DATA statement segment; continue scanning line for next statements.
    bra     .fd_stmt_loop

.fd_next_line:
    move.q  r10, r11
    bra     .fd_line_loop

.fd_found:
    pop     r15
    pop     r14
    rts

.fd_not_found:
    move.q  r8, r0
    pop     r15
    pop     r14
    rts

; ============================================================================
; EOF
; ============================================================================
