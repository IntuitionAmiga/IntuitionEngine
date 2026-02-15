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

    ; Read next-line pointer. If it's 0, R14 is the end terminator - stop.
    ; Invariant: line_rechain sets every real line's next-ptr to a valid address
    ; (either the next line or the terminator). Only the terminator itself has 0.
    load.l  r15, (r14)           ; R15 = next-line pointer
    beqz    r15, .exec_done      ; R14 is the terminator - done

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
    ; If R8 = 1, a GOTO/GOSUB changed R14 - use the new value
    ; If R8 = 2, END was hit - stop
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

    ; --- Jump table dispatch ---
    ; R1 = token index (0-127), guaranteed in range by load.b - 0x80
    la      r9, stmt_jump_table
    lsl.q   r2, r1, #2              ; index * 4
    add.q   r9, r9, r2
    load.l  r9, (r9)                ; handler address
    jsr     (r9)                    ; dispatch to handler
    ; fall through to check return code
.el_check_rc:
    ; After handler returns, check R28
    ; 0 = continue to next stmt, 1 = GOTO (R14 changed), 2 = END/STOP
    bnez    r28, .el_done
    bra     .el_next_stmt

.el_colon:
    ; Statement separator - advance and continue
    add.q   r17, r17, #1
    bra     .el_next_stmt

.el_implied_let:
    ; Not a token and not ':' - must be variable assignment (implied LET)
    ; Check if it's a letter (A-Z or a-z)
    move.q  r2, #0x41            ; 'A'
    blt     r1, r2, .el_done
    move.q  r2, #0x7A            ; 'z'
    bgt     r1, r2, .el_done

    ; --- Intercept coprocessor statements (COSTART, COSTOP, COWAIT) ---
    or.q    r3, r1, #0x20        ; normalize to lowercase
    move.q  r2, #0x63            ; 'c'
    bne     r3, r2, .el_do_let
    load.b  r3, 1(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6F            ; 'o'
    bne     r3, r2, .el_do_let
    ; "CO..." - check third char
    load.b  r3, 2(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x73            ; 's' → COSTART or COSTOP
    beq     r3, r2, .el_check_cos
    move.q  r2, #0x77            ; 'w' → COWAIT
    beq     r3, r2, .el_check_cowait
    bra     .el_do_let

.el_check_cos:
    ; Check for COSTART (7 chars) or COSTOP (6 chars)
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    load.b  r3, 4(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x61            ; 'a' → COSTART
    beq     r3, r2, .el_match_costart
    move.q  r2, #0x6F            ; 'o' → COSTOP
    beq     r3, r2, .el_match_costop
    bra     .el_do_let

.el_match_costart:
    ; Verify remaining chars: R, T + word boundary
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x72            ; 'r'
    bne     r3, r2, .el_do_let
    load.b  r3, 6(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    ; Word boundary: next char must not be alpha, digit, or '$'
    load.b  r3, 7(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_costart
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_costart
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let   ; alpha → variable
.el_is_costart:
    add.q   r17, r17, #7         ; consume "COSTART"
    jsr     exec_do_costart
    bra     .el_check_rc

.el_match_costop:
    ; Verify remaining char: P + word boundary
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x70            ; 'p'
    bne     r3, r2, .el_do_let
    ; Word boundary: next char must not be alpha, digit, or '$'
    load.b  r3, 6(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_costop
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_costop
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let
.el_is_costop:
    add.q   r17, r17, #6         ; consume "COSTOP"
    jsr     exec_do_costop
    bra     .el_check_rc

.el_check_cowait:
    ; Match "COWAIT" (6 chars)
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x61            ; 'a'
    bne     r3, r2, .el_do_let
    load.b  r3, 4(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x69            ; 'i'
    bne     r3, r2, .el_do_let
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .el_do_let
    ; Word boundary: next char must not be alpha, digit, or '$'
    load.b  r3, 6(r17)
    move.q  r2, #0x24            ; '$'
    beq     r3, r2, .el_do_let   ; string suffix → variable
    or.q    r3, r3, #0x20
    move.q  r2, #0x30            ; '0'
    blt     r3, r2, .el_is_cowait
    move.q  r2, #0x39            ; '9'
    ble     r3, r2, .el_do_let   ; digit → variable
    move.q  r2, #0x61            ; 'a'
    blt     r3, r2, .el_is_cowait
    move.q  r2, #0x7A            ; 'z'
    ble     r3, r2, .el_do_let
.el_is_cowait:
    add.q   r17, r17, #6         ; consume "COWAIT"
    jsr     exec_do_cowait
    bra     .el_check_rc

.el_do_let:
    ; It's a letter - implied LET (don't consume, var_lookup will read name)
    jsr     exec_do_let
    bra     .el_check_rc

.el_done:
    move.q  r8, r28
    pop     r28
    rts

; ============================================================================
; Statement handlers
; ============================================================================

; --- END ---
exec_do_end:
    move.q  r28, #2
    rts
    ; === END exec_do_end ===

; --- STOP ---
exec_do_stop:
    ; Save state for CONT
    add.q   r1, r16, #ST_CONT_LINE_PTR
    store.l r14, (r1)
    add.q   r1, r16, #ST_CONT_TEXT_PTR
    store.l r17, (r1)
    move.q  r28, #2
    rts
    ; === END exec_do_stop ===

; --- CONT ---
exec_do_cont:
    ; Restore saved execution position from STOP
    add.q   r1, r16, #ST_CONT_LINE_PTR
    load.l  r14, (r1)
    beqz    r14, .cont_done      ; no saved state → ignore
    add.q   r1, r16, #ST_CONT_TEXT_PTR
    load.l  r17, (r1)
    ; Clear saved state
    add.q   r1, r16, #ST_CONT_LINE_PTR
    store.l r0, (r1)
    ; Signal exec_run to use updated R14
    move.q  r28, #1
.cont_done:
    rts
    ; === END exec_do_cont ===

; --- REM ---
exec_do_rem:
    ; Skip rest of line (everything after REM is a comment)
    ; Advance R17 to end of line (null terminator)
.rem_skip:
    load.b  r1, (r17)
    beqz    r1, .rem_done
    add.q   r17, r17, #1
    bra     .rem_skip
.rem_done:
    rts
    ; === END exec_do_rem ===

; --- DATA ---
exec_do_data:
    ; Skip rest of DATA statement (skip to colon or EOL)
.data_skip:
    load.b  r1, (r17)
    beqz    r1, .data_done
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .data_done
    add.q   r17, r17, #1
    bra     .data_skip
.data_done:
    rts
    ; === END exec_do_data ===

; --- LET ---
exec_do_let:
    jsr     exec_skip_spaces
    ; Parse variable name and get pointer to value slot
    jsr     var_lookup           ; R8 = value slot, R9 = 0(num)/1(str)
    beqz    r8, .let_done        ; error: no variable name
    move.q  r22, r8              ; R22 = save value slot pointer
    move.q  r23, r9              ; R23 = type flag (0=num, 1=str)

    jsr     exec_skip_spaces

    ; Expect '=' token (TK_EQUAL = 0xBC)
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .let_has_eq
    ; Also check raw '='
    move.q  r2, #0x3D
    bne     r1, r2, .let_done   ; no '=' → syntax error (silently skip)
.let_has_eq:
    add.q   r17, r17, #1        ; consume '='
    jsr     exec_skip_spaces

    ; Branch on type
    bnez    r23, .let_str_rhs

    ; Numeric RHS
    jsr     expr_eval            ; R8 = FP32 result
    store.l r8, (r22)
    rts

.let_str_rhs:
    ; String RHS
    jsr     str_eval             ; R8 = pointer to string data
    store.l r8, (r22)
.let_done:
    rts
    ; === END exec_do_let ===

; --- PRINT ---
exec_do_print:
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
    rts
    ; === END exec_do_print ===

; --- GOTO ---
exec_do_goto:
    jsr     exec_skip_spaces
    ; Parse line number (numeric literal)
    jsr     exec_parse_linenum   ; R8 = line number
    ; Search for the target line
    jsr     exec_find_line       ; R14 = new line pointer (or 0)
    beqz    r14, .goto_done      ; line not found → error (silently stop)
    move.q  r28, #1              ; signal: R14 was changed
.goto_done:
    rts
    ; === END exec_do_goto ===

; --- GOSUB ---
exec_do_gosub:
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
    beqz    r14, .gosub_done     ; not found → error
    move.q  r28, #1              ; R14 changed
.gosub_done:
    rts
    ; === END exec_do_gosub ===

; --- RETURN ---
exec_do_return:
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
    beq     r1, r2, .return_at_colon
    add.q   r17, r17, #1
    bra     .return_skip

.return_at_colon:
    ; Found colon - R28=0 means continue, caller will see .el_next_stmt
    rts

.return_next_line:
    ; End of the GOSUB's line - advance to next line
    load.l  r15, (r14)           ; R15 = next line pointer from saved line
    move.q  r14, r15
    move.q  r28, #1              ; R14 changed
    rts
    ; === END exec_do_return ===

; --- IF ---
exec_do_if:
    jsr     exec_skip_spaces
    ; Evaluate condition expression
    jsr     expr_eval            ; R8 = FP32 condition value

    ; Check if true (non-zero): any FP32 with exponent ≠ 0 and value ≠ 0
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .if_false        ; value is ±0.0 → false

    ; Condition is TRUE - look for THEN and execute what follows
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_THEN
    bne     r1, r2, .if_done     ; no THEN → just continue (R28=0)
    add.q   r17, r17, #1        ; consume THEN

    ; Check if what follows THEN is a line number (GOTO shorthand)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .if_done    ; not a digit → execute as statement (R28=0)
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .if_done    ; not a digit → execute as statement (R28=0)
    ; It's a line number - implicit GOTO
    bra     exec_do_goto

.if_false:
    ; Condition is FALSE.
    ; First, skip forward to the THEN token (the one for THEN, not ELSE).
    ; Both THEN and ELSE tokenize to TK_THEN in EhBASIC.
.if_find_then:
    load.b  r1, (r17)
    beqz    r1, .if_done          ; no THEN found → end of line
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
    beqz    r1, .if_done          ; no ELSE → done, produce no output
    move.q  r2, #TK_THEN
    beq     r1, r2, .if_else_found
    add.q   r17, r17, #1
    bra     .if_seek_else

.if_else_found:
    add.q   r17, r17, #1         ; consume ELSE marker (TK_THEN)
    ; R28=0, return to dispatch loop which will continue to .el_next_stmt
.if_done:
    rts
    ; === END exec_do_if ===

; --- FOR ---
exec_do_for:
    jsr     exec_skip_spaces

    ; Get loop variable
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .for_done
    move.q  r22, r8              ; R22 = var value slot pointer

    jsr     exec_skip_spaces

    ; Expect '=' (TK_EQUAL or raw '=')
    load.b  r1, (r17)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .for_eq
    move.q  r2, #0x3D
    bne     r1, r2, .for_done
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
    bne     r1, r2, .for_done
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
.for_done:
    rts
    ; === END exec_do_for ===

; --- NEXT ---
exec_do_next:
    ; Find the most recent FOR frame on GOSUB stack
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)            ; R2 = top of GOSUB stack

    ; Walk back to find FOR marker
    sub.q   r2, r2, #24         ; back up 24 bytes (FOR frame size)
    load.l  r3, (r2)            ; should be "FOR " marker
    move.l  r4, #0x464F5220
    bne     r3, r4, .next_err   ; not FOR → error (silently)

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
    rts

.next_done:
    ; Loop complete: pop FOR frame from GOSUB stack
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #24
    store.l r2, (r1)
    ; R28=0, continue to next statement
.next_err:
    rts
    ; === END exec_do_next ===

; --- WHILE ---
exec_do_while:
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
    rts                          ; R28=0, continue

.while_false:
    ; FALSE: scan forward to matching WEND (nested - tracks WHILE/WEND depth)
    move.q  r7, #1               ; nesting depth = 1 (we are inside this WHILE)
    load.l  r2, (r14)            ; start from next line
.while_scan_lines:
    beqz    r2, .while_done
    load.l  r3, (r2)             ; next pointer
    beqz    r3, .while_done      ; terminator
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
    rts

.while_next_line:
    move.q  r2, r3
    bra     .while_scan_lines
.while_done:
    rts
    ; === END exec_do_while ===

; --- WEND ---
exec_do_wend:
    ; Pop latest WHILE frame and jump back to WHILE line.
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #8
    load.l  r3, (r2)             ; marker
    move.l  r4, #0x5748494C      ; "WHIL"
    bne     r3, r4, .wend_done
    add.q   r5, r2, #4
    load.l  r14, (r5)            ; WHILE line pointer
    store.l r2, (r1)             ; pop frame
    move.q  r28, #1
.wend_done:
    rts
    ; === END exec_do_wend ===

; --- RESTORE ---
exec_do_restore:
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)            ; reset DATA pointer to 0
    rts
    ; === END exec_do_restore ===

; --- INC ---
exec_do_inc:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .inc_done
    move.q  r22, r8
    load.l  r8, (r22)           ; current FP32 value
    move.l  r9, #0x3F800000     ; 1.0
    jsr     fp_add
    store.l r8, (r22)
.inc_done:
    rts
    ; === END exec_do_inc ===

; --- DEC ---
exec_do_dec:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .dec_done
    move.q  r22, r8
    load.l  r8, (r22)           ; current FP32 value
    move.l  r9, #0x3F800000     ; 1.0
    jsr     fp_sub
    store.l r8, (r22)
.dec_done:
    rts
    ; === END exec_do_dec ===

; --- DOKE ---
exec_do_doke:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .doke_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = value (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer value
    store.w r8, (r22)            ; write 16-bit to address
.doke_done:
    rts
    ; === END exec_do_doke ===

; --- LOKE ---
exec_do_loke:
    ; LOKE is identical to POKE (32-bit write)
    bra     exec_do_poke
    ; === END exec_do_loke ===

; --- CLEAR ---
exec_do_clear:
    jsr     var_init             ; reset all variables
    ; Reset DATA pointer
    add.q   r1, r16, #ST_DATA_PTR
    store.l r0, (r1)
    ; Reset GOSUB stack
    la      r1, BASIC_GOSUB_STACK
    add.q   r2, r16, #ST_GOSUB_SP
    store.l r1, (r2)
    rts
    ; === END exec_do_clear ===

; --- SWAP ---
exec_do_swap:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to first var
    beqz    r8, .swap_done
    move.q  r22, r8              ; R22 = first var ptr
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .swap_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to second var
    beqz    r8, .swap_done
    move.q  r23, r8              ; R23 = second var ptr
    ; Exchange values
    load.l  r1, (r22)
    load.l  r2, (r23)
    store.l r2, (r22)
    store.l r1, (r23)
.swap_done:
    rts
    ; === END exec_do_swap ===

; --- BITSET ---
exec_do_bitset:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .bitset_done
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
.bitset_done:
    rts
    ; === END exec_do_bitset ===

; --- BITCLR ---
exec_do_bitclr:
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = address
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .bitclr_done
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
.bitclr_done:
    rts
    ; === END exec_do_bitclr ===

; --- GET ---
exec_do_get:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = ptr to value slot, R9 = type
    beqz    r8, .get_done
    move.q  r22, r8              ; R22 = value slot
    move.q  r23, r9              ; R23 = type (0=num, 1=str)
    ; Check if key available
    la      r1, TERM_KEY_STATUS
    load.l  r1, (r1)
    and.l   r1, r1, #1          ; bit 0 = char available
    beqz    r1, .get_empty
    ; Read char
    la      r3, TERM_KEY_IN
    load.b  r8, (r3)
    and.l   r8, r8, #0xFF
    ; If string var, store as single-char string
    bnez    r23, .get_str
    ; Numeric: store ASCII code as FP32
    jsr     fp_float
    store.l r8, (r22)
    rts
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
    rts
.get_empty:
    ; No char available: store 0 (num) or "" (str)
    bnez    r23, .get_empty_str
    store.l r0, (r22)            ; 0
    rts
.get_empty_str:
    add.q   r1, r16, #ST_HEAP_TOP
    load.l  r3, (r1)
    store.b r0, (r3)             ; empty string (just null)
    store.l r3, (r22)
    add.q   r3, r3, #1
    store.l r3, (r1)
.get_done:
    rts
    ; === END exec_do_get ===

; --- ON ---
exec_do_on:
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
    rts
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
    bne     r1, r2, .on_done     ; out of range → fall through
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    add.q   r23, r23, #1
    bra     .on_goto_scan
.on_goto_target:
    bra     exec_do_goto         ; parse line number and GOTO
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
    bne     r1, r2, .on_done     ; out of range → fall through
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    add.q   r23, r23, #1
    bra     .on_gosub_scan
.on_gosub_target:
    bra     exec_do_gosub        ; parse line number and GOSUB
.on_done:
    rts
    ; === END exec_do_on ===

; --- DO ---
exec_do_do:
    ; Push DO frame on GOSUB stack: ["DO  "][line ptr] = 8 bytes
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    move.l  r3, #0x444F2020      ; "DO  "
    store.l r3, (r2)
    add.q   r2, r2, #4
    store.l r14, (r2)            ; line pointer of DO statement
    add.q   r2, r2, #4
    store.l r2, (r1)             ; update SP
    rts
    ; === END exec_do_do ===

; --- LOOP ---
exec_do_loop:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_WHILE
    beq     r1, r2, .loop_while
    move.q  r2, #TK_UNTIL
    beq     r1, r2, .loop_until
    ; Bare LOOP - unconditional loop back to DO
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
    bne     r3, r4, .loop_done   ; not DO → error
    add.q   r5, r2, #4
    load.l  r14, (r5)            ; DO line pointer
    store.l r2, (r1)             ; pop frame
    move.q  r28, #1              ; R14 changed
    rts
.loop_exit:
    ; Pop DO frame and continue
    add.q   r1, r16, #ST_GOSUB_SP
    load.l  r2, (r1)
    sub.q   r2, r2, #8
    store.l r2, (r1)
.loop_done:
    rts
    ; === END exec_do_loop ===

; --- INPUT ---
exec_do_input:
    ; Minimal INPUT implementation: INPUT A[,B,...]
    ; Reads one line per variable from terminal and parses numeric expression.
.input_loop:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = value slot pointer
    beqz    r8, .input_done
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
    bne     r1, r2, .input_done
    add.q   r17, r17, #1
    bra     .input_loop
.input_done:
    rts
    ; === END exec_do_input ===

; --- DIM ---
exec_do_dim:
    jsr     arr_dim
    rts
    ; === END exec_do_dim ===

; --- POKE ---
; POKE addr, value  - write 32-bit value to addr (matches PEEK which reads 32-bit)
exec_do_poke:
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
    bne     r1, r2, .poke_done
    add.q   r17, r17, #1         ; consume ','
    jsr     exec_skip_spaces
    jsr     expr_eval            ; R8 = value (FP32)
    jsr     fp_int               ; R8 = INT(value) FP32
    jsr     fp_fix               ; R8 = integer
    beqz    r23, .poke_32bit
    store.b r8, (r22)            ; write byte to address
    rts
.poke_32bit:
    store.l r8, (r22)            ; write 32-bit to address
.poke_done:
    rts
    ; === END exec_do_poke ===

; --- READ ---
exec_do_read:
    ; READ A[,B,...] from DATA statements in program text
.read_loop:
    jsr     exec_skip_spaces
    jsr     var_lookup           ; R8 = value slot pointer
    beqz    r8, .read_done
    move.q  r22, r8
    push    r17                  ; save text pointer (clobbered by data scanner)
    jsr     exec_read_next_data_value ; R8 = FP32 value
    pop     r17                  ; restore text pointer
    store.l r8, (r22)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .read_done
    add.q   r17, r17, #1
    bra     .read_loop
.read_done:
    rts
    ; === END exec_do_read ===

; --- Hardware command wrappers ---
; Each wraps a hw_xxx call, then returns. Since the hw_xxx handlers
; all return via rts, we can tail-call them with bra instead of jsr+rts.
exec_do_screen:   bra hw_screen
exec_do_cls:      bra hw_cls
exec_do_plot:     bra hw_plot
exec_do_palette:  bra hw_palette
exec_do_vsync:    bra hw_vsync
exec_do_locate:   bra hw_locate
exec_do_color:    bra hw_color
exec_do_line:     bra hw_line
exec_do_circle:   bra hw_circle
exec_do_box:      bra hw_box
exec_do_scroll:   bra hw_scroll
exec_do_sound:    bra hw_sound
exec_do_envelope: bra hw_envelope
exec_do_gate:     bra hw_gate
exec_do_wait:     bra hw_wait
exec_do_copper:   bra hw_copper
exec_do_blit:     bra hw_blit
exec_do_ula:      bra hw_ula
exec_do_ted_cmd:  bra hw_ted_cmd
exec_do_antic:    bra hw_antic
exec_do_gtia:     bra hw_gtia
exec_do_voodoo:   bra hw_voodoo
exec_do_psg:      bra hw_psg
exec_do_sid:      bra hw_sid
exec_do_pokey:    bra hw_pokey
exec_do_ahx:      bra hw_ahx
exec_do_sap:      bra hw_sap
exec_do_zbuffer:  bra hw_zbuffer
exec_do_vertex:   bra hw_vertex
exec_do_triangle: bra hw_triangle
exec_do_texture:  bra hw_texture

; --- CALL ---
exec_do_call:
    ; CALL addr - JSR to machine code at given address
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
    rts
    ; === END exec_do_call ===

; --- TRON ---
exec_do_tron:
    ; TRON - enable trace mode (print line numbers during execution)
    add.q   r1, r16, #ST_TRACE_FLAG
    move.q  r2, #1
    store.l r2, (r1)
    rts
    ; === END exec_do_tron ===

; --- LOAD and SAVE are implemented in ehbasic_file_io.inc ---

exec_do_troff:
    ; TROFF - disable trace mode
    add.q   r1, r16, #ST_TRACE_FLAG
    store.l r0, (r1)
    rts
    ; === END exec_do_troff ===

; --- Unknown token handler ---
exec_do_unknown:
    rts
    ; === END exec_do_unknown ===

; ============================================================================
; Statement Jump Table (128 entries, indexed by token - 0x80)
; ============================================================================
; Indices: 0=END, 1=FOR, 2=NEXT, 3=DATA, 4=INPUT, 5=DIM, 6=READ, 7=LET,
;          8=DEC, 9=GOTO, 10=unused(RUN), 11=IF, 12=RESTORE, 13=GOSUB,
;          14=RETURN, 15=REM, 16=STOP, 17=ON(TRON), 18=TRON, 19=INC,
;          20=WAIT, 21=TROFF, 22-23=unused, 24=POKE, 25=DOKE, 26=LOKE,
;          27=CALL, 28=DO, 29=LOOP, 30=PRINT, 31=CONT
;          32-46=mixed (LIST/NEW/BLOAD and parser keywords), 47=WEND, 48=WHILE, 49-97=unused
;          98=SCREEN..127=TEXTURE

    align 4
stmt_jump_table:
    dc.l    exec_do_end          ;  0 = TK_END      (0x80)
    dc.l    exec_do_for          ;  1 = TK_FOR      (0x81)
    dc.l    exec_do_next         ;  2 = TK_NEXT     (0x82)
    dc.l    exec_do_data         ;  3 = TK_DATA     (0x83)
    dc.l    exec_do_input        ;  4 = TK_INPUT    (0x84)
    dc.l    exec_do_dim          ;  5 = TK_DIM      (0x85)
    dc.l    exec_do_read         ;  6 = TK_READ     (0x86)
    dc.l    exec_do_let          ;  7 = TK_LET      (0x87)
    dc.l    exec_do_dec          ;  8 = TK_DEC      (0x88)
    dc.l    exec_do_goto         ;  9 = TK_GOTO     (0x89)
    dc.l    exec_do_unknown      ; 10 = TK_RUN      (0x8A) - not a stmt token
    dc.l    exec_do_if           ; 11 = TK_IF       (0x8B)
    dc.l    exec_do_restore      ; 12 = TK_RESTORE  (0x8C)
    dc.l    exec_do_gosub        ; 13 = TK_GOSUB    (0x8D)
    dc.l    exec_do_return       ; 14 = TK_RETURN   (0x8E)
    dc.l    exec_do_rem          ; 15 = TK_REM      (0x8F)
    dc.l    exec_do_stop         ; 16 = TK_STOP     (0x90)
    dc.l    exec_do_on           ; 17 = TK_ON       (0x91)
    dc.l    exec_do_tron         ; 18 = TK_NULL→TRON(0x92)
    dc.l    exec_do_inc          ; 19 = TK_INC      (0x93)
    dc.l    exec_do_wait         ; 20 = TK_WAIT     (0x94)
    dc.l    exec_do_load         ; 21 = TK_LOAD     (0x95)
    dc.l    exec_do_save         ; 22 = TK_SAVE     (0x96)
    dc.l    exec_do_troff        ; 23 = TK_DEF→TROFF(0x97)
    dc.l    exec_do_poke         ; 24 = TK_POKE     (0x98)
    dc.l    exec_do_doke         ; 25 = TK_DOKE     (0x99)
    dc.l    exec_do_loke         ; 26 = TK_LOKE     (0x9A)
    dc.l    exec_do_call         ; 27 = TK_CALL     (0x9B)
    dc.l    exec_do_do           ; 28 = TK_DO       (0x9C)
    dc.l    exec_do_loop         ; 29 = TK_LOOP     (0x9D)
    dc.l    exec_do_print        ; 30 = TK_PRINT    (0x9E)
    dc.l    exec_do_cont         ; 31 = TK_CONT     (0x9F)
    dc.l    exec_do_unknown      ; 32 = unused      (0xA0)
    dc.l    exec_do_clear        ; 33 = TK_CLEAR    (0xA1)
    dc.l    exec_do_unknown      ; 34 = unused      (0xA2)
    dc.l    exec_do_bload        ; 35 = TK_WIDTH→BLOAD (0xA3)
    dc.l    exec_do_get          ; 36 = TK_GET      (0xA4)
    dc.l    exec_do_swap         ; 37 = TK_SWAP     (0xA5)
    dc.l    exec_do_bitset       ; 38 = TK_BITSET   (0xA6)
    dc.l    exec_do_bitclr       ; 39 = TK_BITCLR   (0xA7)
    dc.l    exec_do_unknown      ; 40 = TK_THEN     (0xA8)
    dc.l    exec_do_unknown      ; 41 = TK_TO       (0xA9)
    dc.l    exec_do_unknown      ; 42 = TK_STEP     (0xAA)
    dc.l    exec_do_unknown      ; 43 = unused      (0xAB)
    dc.l    exec_do_unknown      ; 44 = unused      (0xAC)
    dc.l    exec_do_unknown      ; 45 = TK_NOT      (0xAD)
    dc.l    exec_do_unknown      ; 46 = unused      (0xAE)
    dc.l    exec_do_wend         ; 47 = TK_UNTIL    (0xAF) → WEND
    dc.l    exec_do_while        ; 48 = TK_WHILE    (0xB0)
    ; 49-97: function/operator tokens - not statements
    dc.l    exec_do_unknown      ; 49  (0xB1) TK_PLUS
    dc.l    exec_do_unknown      ; 50  (0xB2) TK_MINUS
    dc.l    exec_do_unknown      ; 51  (0xB3) TK_MULT
    dc.l    exec_do_unknown      ; 52  (0xB4) TK_DIV
    dc.l    exec_do_unknown      ; 53  (0xB5) TK_POWER
    dc.l    exec_do_unknown      ; 54  (0xB6) TK_AND
    dc.l    exec_do_unknown      ; 55  (0xB7) TK_EOR
    dc.l    exec_do_unknown      ; 56  (0xB8) TK_OR
    dc.l    exec_do_unknown      ; 57  (0xB9)
    dc.l    exec_do_unknown      ; 58  (0xBA)
    dc.l    exec_do_unknown      ; 59  (0xBB) TK_GT
    dc.l    exec_do_unknown      ; 60  (0xBC) TK_EQUAL
    dc.l    exec_do_unknown      ; 61  (0xBD) TK_LT
    dc.l    exec_do_unknown      ; 62  (0xBE) TK_SGN
    dc.l    exec_do_unknown      ; 63  (0xBF) TK_INT
    dc.l    exec_do_unknown      ; 64  (0xC0) TK_ABS
    dc.l    exec_do_unknown      ; 65  (0xC1) TK_USR
    dc.l    exec_do_unknown      ; 66  (0xC2) TK_FRE
    dc.l    exec_do_unknown      ; 67  (0xC3)
    dc.l    exec_do_unknown      ; 68  (0xC4) TK_SQR
    dc.l    exec_do_unknown      ; 69  (0xC5) TK_RND
    dc.l    exec_do_unknown      ; 70  (0xC6) TK_SIN
    dc.l    exec_do_unknown      ; 71  (0xC7) TK_LOG
    dc.l    exec_do_unknown      ; 72  (0xC8) TK_EXP
    dc.l    exec_do_unknown      ; 73  (0xC9) TK_COS
    dc.l    exec_do_unknown      ; 74  (0xCA) TK_TAN
    dc.l    exec_do_unknown      ; 75  (0xCB) TK_ATN
    dc.l    exec_do_unknown      ; 76  (0xCC) TK_PEEK
    dc.l    exec_do_unknown      ; 77  (0xCD) TK_DEEK
    dc.l    exec_do_unknown      ; 78  (0xCE) TK_LEEK
    dc.l    exec_do_unknown      ; 79  (0xCF) TK_LEN
    dc.l    exec_do_unknown      ; 80  (0xD0) TK_STR
    dc.l    exec_do_unknown      ; 81  (0xD1) TK_VAL
    dc.l    exec_do_unknown      ; 82  (0xD2) TK_ASC
    dc.l    exec_do_unknown      ; 83  (0xD3) TK_CHR
    dc.l    exec_do_unknown      ; 84  (0xD4) TK_LEFTS
    dc.l    exec_do_unknown      ; 85  (0xD5) TK_RIGHTS
    dc.l    exec_do_unknown      ; 86  (0xD6) TK_MIDS
    dc.l    exec_do_unknown      ; 87  (0xD7) TK_PI
    dc.l    exec_do_unknown      ; 88  (0xD8) TK_TWOPI
    dc.l    exec_do_unknown      ; 89  (0xD9) TK_HEXS
    dc.l    exec_do_unknown      ; 90  (0xDA) TK_BINS
    dc.l    exec_do_unknown      ; 91  (0xDB) TK_BITTST
    dc.l    exec_do_unknown      ; 92  (0xDC) TK_MAX
    dc.l    exec_do_unknown      ; 93  (0xDD) TK_MIN
    dc.l    exec_do_unknown      ; 94  (0xDE)
    dc.l    exec_do_unknown      ; 95  (0xDF)
    dc.l    exec_do_unknown      ; 96  (0xE0)
    dc.l    exec_do_unknown      ; 97  (0xE1)
    ; 98-127: Hardware extension tokens (0xE2-0xFF)
    dc.l    exec_do_screen       ; 98  (0xE2) TK_SCREEN
    dc.l    exec_do_cls          ; 99  (0xE3) TK_CLS
    dc.l    exec_do_plot         ;100  (0xE4) TK_PLOT
    dc.l    exec_do_palette      ;101  (0xE5) TK_PALETTE
    dc.l    exec_do_vsync        ;102  (0xE6) TK_VSYNC
    dc.l    exec_do_locate       ;103  (0xE7) TK_LOCATE
    dc.l    exec_do_color        ;104  (0xE8) TK_COLOR
    dc.l    exec_do_line         ;105  (0xE9) TK_LINE_CMD
    dc.l    exec_do_circle       ;106  (0xEA) TK_CIRCLE
    dc.l    exec_do_box          ;107  (0xEB) TK_BOX
    dc.l    exec_do_scroll       ;108  (0xEC) TK_SCROLL_CMD
    dc.l    exec_do_copper       ;109  (0xED) TK_COPPER
    dc.l    exec_do_blit         ;110  (0xEE) TK_BLIT
    dc.l    exec_do_sound        ;111  (0xEF) TK_SOUND
    dc.l    exec_do_envelope     ;112  (0xF0) TK_ENVELOPE
    dc.l    exec_do_gate         ;113  (0xF1) TK_GATE
    dc.l    exec_do_ula          ;114  (0xF2) TK_ULA
    dc.l    exec_do_ted_cmd      ;115  (0xF3) TK_TED_CMD
    dc.l    exec_do_antic        ;116  (0xF4) TK_ANTIC
    dc.l    exec_do_gtia         ;117  (0xF5) TK_GTIA
    dc.l    exec_do_voodoo       ;118  (0xF6) TK_VOODOO
    dc.l    exec_do_psg          ;119  (0xF7) TK_PSG_CMD
    dc.l    exec_do_sid          ;120  (0xF8) TK_SID_CMD
    dc.l    exec_do_pokey        ;121  (0xF9) TK_POKEY_CMD
    dc.l    exec_do_ahx          ;122  (0xFA) TK_AHX
    dc.l    exec_do_sap          ;123  (0xFB) TK_SAP
    dc.l    exec_do_zbuffer      ;124  (0xFC) TK_ZBUFFER
    dc.l    exec_do_vertex       ;125  (0xFD) TK_VERTEX
    dc.l    exec_do_triangle     ;126  (0xFE) TK_TRIANGLE
    dc.l    exec_do_texture      ;127  (0xFF) TK_TEXTURE

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
    ; parse_number clobbers R1-R7, so preserve DATA indices (R6=target, R7=current)
    push    r6
    push    r7
    move.q  r17, r12
    jsr     parse_number          ; R8 = FP32, R17 advanced
    move.q  r12, r17

    ; apply sign if needed
    beqz    r13, .fd_restore_idx
    jsr     fp_neg

.fd_restore_idx:
    pop     r7
    pop     r6

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
