; ehbasic_expr.inc - IE64 BASIC Expression Evaluator
;
; Recursive-descent expression evaluator for numeric expressions.
; Supports: integer/float literals, +, -, *, /, ^, <, >, =, AND, OR,
;           EOR, NOT, unary minus, parentheses.
;
; Numeric expressions return R8=payload and R9=VAL_F64 or VAL_I64. Most of the
; legacy evaluator still computes FP64 internally; expr_eval first recognises the
; integer-compatible subset so exact qword values can flow through public APIs.
;
; Calling convention:
;   expr_eval:
;     Input:  R17 = pointer to tokenized expression (null-terminated)
;     Output: R8  = result as FP64 bits
;             R17 = updated past consumed tokens
;     Clobbers: R1-R15
;
; Register usage during evaluation:
;   R17 = text pointer (advanced as tokens are consumed)
;   R16 = interpreter state base (preserved)
;   R26/R27 = terminal I/O (preserved)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; expr_eval - Evaluate expression at R17
; ============================================================================
; Entry point: evaluates a full expression with all precedence levels.
; Returns result in R8, tag in R9.

expr_eval:
    push    r14
    push    r15
    push    r22
    push    r23
    push    r24
    push    r25

    move.q  r25, r17             ; fallback cursor
    jsr     exec_eval_i64
    beqz    r9, .expr_fallback_fp64_restore
    jsr     expr_i64_boundary
    beqz    r9, .expr_fallback_fp64_restore
    move.q  r9, #VAL_I64
    bra     .expr_done

.expr_fallback_fp64_restore:
    move.q  r17, r25
.expr_fallback_fp64:
    move.q  r9, r0
    jsr     expr_or              ; evaluate legacy FP64 precedence tree
    move.q  r1, #VAL_I64
    beq     r9, r1, .expr_done
    move.q  r9, #VAL_F64

.expr_done:

    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r15
    pop     r14
    rts

; expr_i64_boundary - true if R17 is at a valid end of the current expression.
; This prevents a numeric prefix such as 1.5 or 123*4 being accepted as I64.
; Output: R9=1 at a boundary, R9=0 otherwise.
expr_i64_boundary:
.eib_skip_spaces:
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .eib_after_spaces
    add.q   r17, r17, #1
    bra     .eib_skip_spaces
.eib_after_spaces:
    load.b  r1, (r17)
    beqz    r1, .eib_yes
    move.q  r2, #0x29            ; ')'
    beq     r1, r2, .eib_yes
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .eib_yes
    move.q  r2, #0x3B            ; ';'
    beq     r1, r2, .eib_yes
    move.q  r2, #0x3A            ; ':'
    beq     r1, r2, .eib_yes
    move.q  r9, r0
    rts
.eib_yes:
    move.q  r9, #1
    rts

; ============================================================================
; Precedence levels (lowest to highest):
;   1. OR, EOR       (tokens TK_OR=0xB8, TK_EOR=0xB7)
;   2. AND           (token TK_AND=0xB6)
;   3. NOT           (token TK_NOT=0xAD, unary prefix)
;   4. Comparison    (<, >, =  tokens TK_LT=0xBD, TK_GT=0xBB, TK_EQUAL=0xBC)
;   5. Addition      (+, -  tokens TK_PLUS=0xB1, TK_MINUS=0xB2)
;   6. Multiplication(*, /  tokens TK_MULT=0xB3, TK_DIV=0xB4)
;   7. Power         (^     token TK_POWER=0xB5)
;   8. Unary minus   (leading -)
;   9. Primary       (number, parentheses, function call)
; ============================================================================

; --- Level 1: OR / EOR ---
expr_or:
    move.q  r9, #VAL_F64
    jsr     expr_and
.eor_loop:
    ; Skip spaces before checking for OR/EOR token
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .eor_check
    add.q   r17, r17, #1
    bra     .eor_loop
.eor_check:
    move.q  r2, #TK_OR
    beq     r1, r2, .do_or
    move.q  r2, #TK_EOR
    beq     r1, r2, .do_eor
    rts

.do_or:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_and
    jsr     expr_value_to_fp64
    pop     r22
    ; OR: convert both to int, bitwise OR
    jsr     fp64_to_int_pair     ; R22→R1(int), R8→R2(int)
    or.l    r1, r1, r2
    move.q  r8, r1
    jsr     fp_float             ; int→FP64
    move.q  r9, #VAL_F64
    bra     .eor_loop

.do_eor:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_and
    jsr     expr_value_to_fp64
    pop     r22
    jsr     fp64_to_int_pair
    eor.l   r1, r1, r2
    move.q  r8, r1
    jsr     fp_float
    move.q  r9, #VAL_F64
    bra     .eor_loop

; --- Level 2: AND (with inlined NOT check) ---
expr_and:
    jsr     expr_not_inline
.and_loop:
    ; Skip spaces before checking for AND token
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .and_check
    add.q   r17, r17, #1
    bra     .and_loop
.and_check:
    move.q  r2, #TK_AND
    bne     r1, r2, .and_done
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_not_inline
    jsr     expr_value_to_fp64
    pop     r22
    jsr     fp64_to_int_pair
    and.l   r1, r1, r2
    move.q  r8, r1
    jsr     fp_float
    move.q  r9, #VAL_F64
    bra     .and_loop
.and_done:
    rts

; --- Level 3: NOT (unary prefix, inlined skip-spaces + compare check) ---
expr_not_inline:
    ; Skip spaces then check for NOT token; if not found, tail-call expr_compare
    load.b  r1, (r17)
    move.q  r2, #0x20
    beq     r1, r2, .not_skip_sp
    bra     .not_check
.not_skip_sp:
    add.q   r17, r17, #1
    load.b  r1, (r17)
    move.q  r2, #0x20
    beq     r1, r2, .not_skip_sp
.not_check:
    move.q  r2, #TK_NOT
    bne     r1, r2, expr_compare ; tail-call: no NOT → go straight to compare
    add.q   r17, r17, #1
    jsr     expr_not_inline      ; recursive for NOT NOT x
    ; NOT: convert to int, complement, convert back
    jsr     fp_int               ; truncate to integer
    push    r8
    jsr     fp_fix               ; FP64→signed int32 in R8
    move.q  r1, r8
    pop     r8                   ; (discard)
    move.q  r2, #0xFFFFFFFF
    eor.l   r1, r1, r2           ; bitwise NOT
    move.q  r8, r1
    jsr     fp_float             ; int→FP64
    rts

; --- Level 4: Comparison (<, >, =) ---
expr_compare:
    jsr     str_detect
    bnez    r8, .cmp_string
    jsr     expr_add
.cmp_loop:
    ; Skip spaces before checking for a comparison token. Without this,
    ; `PRINT A > B` (with spaces) would mis-parse as two separate
    ; PRINT items because the operator check only sees the space.
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .cmp_check
    add.q   r17, r17, #1
    bra     .cmp_loop
.cmp_check:
    move.q  r2, #TK_LT
    beq     r1, r2, .do_lt
    move.q  r2, #TK_GT
    beq     r1, r2, .do_gt
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .do_eq
    rts

.do_lt:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    move.q  r22, r8
    move.q  r24, r0               ; 0 = <
    load.b  r1, (r17)
    move.q  r2, #0x3D             ; raw '=' marker for <=
    beq     r1, r2, .lt_set_le
    move.q  r2, #TK_EQUAL         ; tolerate old accidental token stream
    beq     r1, r2, .lt_set_le
    move.q  r2, #0x3E             ; raw '>' marker for <>
    beq     r1, r2, .lt_set_ne
    move.q  r2, #TK_GT            ; tolerate old accidental token stream
    beq     r1, r2, .lt_set_ne
    bra     .lt_eval_right
.lt_set_le:
    move.q  r24, #1               ; 1 = <=
    add.q   r17, r17, #1
    bra     .lt_eval_right
.lt_set_ne:
    move.q  r24, #2               ; 2 = <>
    add.q   r17, r17, #1
.lt_eval_right:
    push    r22
    push    r24
    jsr     expr_add
    jsr     expr_value_to_fp64
    pop     r24
    pop     r22
    ; Compare: R22 < R8?
    move.q  r9, r8               ; R9 = right
    move.q  r8, r22              ; R8 = left
    jsr     fp_cmp               ; R8 = -1/0/1
    move.q  r2, #2
    beq     r24, r2, .cmp_ne_check
    move.q  r2, #1
    beq     r24, r2, .cmp_le_check
    ; If result = -1 (0xFFFFFFFF), left < right → TRUE (-1.0)
    move.q  r2, #0xFFFFFFFF
    beq     r8, r2, .cmp_true
    bra     .cmp_false

.cmp_le_check:
    move.q  r2, #0xFFFFFFFF
    beq     r8, r2, .cmp_true
    beqz    r8, .cmp_true
    bra     .cmp_false

.cmp_ne_check:
    bnez    r8, .cmp_true
    bra     .cmp_false

.do_gt:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    move.q  r22, r8
    move.q  r24, r0               ; 0 = >
    load.b  r1, (r17)
    move.q  r2, #0x3D             ; raw '=' marker for >=
    beq     r1, r2, .gt_set_ge
    move.q  r2, #TK_EQUAL         ; tolerate old accidental token stream
    beq     r1, r2, .gt_set_ge
    bra     .gt_eval_right
.gt_set_ge:
    move.q  r24, #1               ; 1 = >=
    add.q   r17, r17, #1
.gt_eval_right:
    push    r22
    push    r24
    jsr     expr_add
    jsr     expr_value_to_fp64
    pop     r24
    pop     r22
    move.q  r9, r8
    move.q  r8, r22
    jsr     fp_cmp
    move.q  r2, #1
    beq     r24, r2, .cmp_ge_check
    move.q  r2, #1
    beq     r8, r2, .cmp_true
    bra     .cmp_false

.cmp_ge_check:
    move.q  r2, #1
    beq     r8, r2, .cmp_true
    beqz    r8, .cmp_true
    bra     .cmp_false

.do_eq:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_add
    jsr     expr_value_to_fp64
    pop     r22
    move.q  r9, r8
    move.q  r8, r22
    jsr     fp_cmp
    beqz    r8, .cmp_true
    bra     .cmp_false

.cmp_true:
    move.q  r8, r0
    not.q   r8, r8
    move.q  r9, #VAL_I64
    bra     .cmp_loop

.cmp_false:
    move.q  r8, r0
    move.q  r9, #VAL_I64
    bra     .cmp_loop

.cmp_string:
    jsr     str_eval
    move.q  r22, r8              ; left string pointer
.cmp_str_skip_op_sp:
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .cmp_str_op
    add.q   r17, r17, #1
    bra     .cmp_str_skip_op_sp
.cmp_str_op:
    move.q  r24, r0              ; 0 = =
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .cmp_str_eq
    move.q  r2, #TK_LT
    beq     r1, r2, .cmp_str_lt
    move.q  r2, #TK_GT
    beq     r1, r2, .cmp_str_gt
    move.q  r8, r0
    rts
.cmp_str_eq:
    add.q   r17, r17, #1
    bra     .cmp_str_rhs
.cmp_str_lt:
    move.q  r24, #1              ; 1 = <
    add.q   r17, r17, #1
    load.b  r1, (r17)
    move.q  r2, #0x3D
    beq     r1, r2, .cmp_str_lt_eq
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .cmp_str_lt_eq
    move.q  r2, #0x3E
    beq     r1, r2, .cmp_str_lt_ne
    move.q  r2, #TK_GT
    beq     r1, r2, .cmp_str_lt_ne
    bra     .cmp_str_rhs
.cmp_str_lt_eq:
    move.q  r24, #2              ; 2 = <=
    add.q   r17, r17, #1
    bra     .cmp_str_rhs
.cmp_str_lt_ne:
    move.q  r24, #5              ; 5 = <>
    add.q   r17, r17, #1
    bra     .cmp_str_rhs
.cmp_str_gt:
    move.q  r24, #3              ; 3 = >
    add.q   r17, r17, #1
    load.b  r1, (r17)
    move.q  r2, #0x3D
    beq     r1, r2, .cmp_str_gt_eq
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .cmp_str_gt_eq
    bra     .cmp_str_rhs
.cmp_str_gt_eq:
    move.q  r24, #4              ; 4 = >=
    add.q   r17, r17, #1
.cmp_str_rhs:
    move.q  r8, r22
    jsr     str_gc_push
    push    r24
.cmp_str_skip_rhs_sp:
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .cmp_str_eval_rhs
    add.q   r17, r17, #1
    bra     .cmp_str_skip_rhs_sp
.cmp_str_eval_rhs:
    jsr     str_eval
    move.q  r9, r8               ; right string pointer
    pop     r24
    jsr     str_gc_pop           ; updated left string pointer
    jsr     str_cmp              ; R8 = -1/0/1
    move.q  r2, #5
    beq     r24, r2, .cmp_str_ne_check
    move.q  r2, #4
    beq     r24, r2, .cmp_str_ge_check
    move.q  r2, #3
    beq     r24, r2, .cmp_str_gt_check
    move.q  r2, #2
    beq     r24, r2, .cmp_str_le_check
    move.q  r2, #1
    beq     r24, r2, .cmp_str_lt_check
    beqz    r8, .cmp_true
    bra     .cmp_false
.cmp_str_lt_check:
    move.q  r2, #0xFFFFFFFF
    beq     r8, r2, .cmp_true
    bra     .cmp_false
.cmp_str_le_check:
    move.q  r2, #0xFFFFFFFF
    beq     r8, r2, .cmp_true
    beqz    r8, .cmp_true
    bra     .cmp_false
.cmp_str_gt_check:
    move.q  r2, #1
    beq     r8, r2, .cmp_true
    bra     .cmp_false
.cmp_str_ge_check:
    move.q  r2, #1
    beq     r8, r2, .cmp_true
    beqz    r8, .cmp_true
    bra     .cmp_false
.cmp_str_ne_check:
    bnez    r8, .cmp_true
    bra     .cmp_false

; --- Level 5: Addition (+, -) ---
expr_add:
    jsr     expr_mul
.add_loop:
    ; Skip spaces before checking for +/-/<</>> operator. Without this,
    ; `PRINT 1 + 2` mis-parses: expr_add sees space, returns 1; PRINT
    ; restarts and prints +2 as a separate item, yielding `1 2` not `3`.
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .add_check
    add.q   r17, r17, #1
    bra     .add_loop
.add_check:
    move.q  r2, #TK_PLUS
    beq     r1, r2, .do_add
    move.q  r2, #TK_MINUS
    beq     r1, r2, .do_sub
    move.q  r2, #TK_LSHIFT
    beq     r1, r2, .do_lshift
    move.q  r2, #TK_RSHIFT
    beq     r1, r2, .do_rshift
    rts

.do_add:
    add.q   r17, r17, #1
    push    r8
    push    r9
    jsr     expr_mul
    move.q  r23, r8              ; right payload
    move.q  r24, r9              ; right tag
    pop     r25                  ; left tag
    pop     r22                  ; left payload
    move.q  r1, #VAL_I64
    bne     r25, r1, .add_fp64
    bne     r24, r1, .add_fp64
    add.q   r8, r22, r23
    move.q  r9, #VAL_I64
    bra     .add_loop
.add_fp64:
    move.q  r8, r22
    move.q  r9, r25
    jsr     expr_value_to_fp64
    push    r8
    move.q  r8, r23
    move.q  r9, r24
    jsr     expr_value_to_fp64
    pop     r22
    move.q  r9, r8               ; R9 = right
    move.q  r8, r22              ; R8 = left
    jsr     fp_add               ; R8 = left + right
    move.q  r9, #VAL_F64
    bra     .add_loop

.do_sub:
    add.q   r17, r17, #1
    push    r8
    push    r9
    jsr     expr_mul
    move.q  r23, r8              ; right payload
    move.q  r24, r9              ; right tag
    pop     r25                  ; left tag
    pop     r22                  ; left payload
    move.q  r1, #VAL_I64
    bne     r25, r1, .sub_fp64
    bne     r24, r1, .sub_fp64
    sub.q   r8, r22, r23
    move.q  r9, #VAL_I64
    bra     .add_loop
.sub_fp64:
    move.q  r8, r22
    move.q  r9, r25
    jsr     expr_value_to_fp64
    push    r8
    move.q  r8, r23
    move.q  r9, r24
    jsr     expr_value_to_fp64
    pop     r22
    move.q  r9, r8               ; R9 = right
    move.q  r8, r22              ; R8 = left
    jsr     fp_sub               ; R8 = left - right
    move.q  r9, #VAL_F64
    bra     .add_loop

.do_lshift:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_mul
    jsr     expr_value_to_fp64
    pop     r22
    jsr     fp64_to_int_pair
    and.l   r2, r2, #31
    lsl.l   r1, r1, r2
    move.q  r8, r1
    jsr     fp_float
    move.q  r9, #VAL_F64
    bra     .add_loop

.do_rshift:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_mul
    jsr     expr_value_to_fp64
    pop     r22
    jsr     fp64_to_int_pair
    and.l   r2, r2, #31
    lsr.l   r1, r1, r2
    move.q  r8, r1
    jsr     fp_float
    move.q  r9, #VAL_F64
    bra     .add_loop

; --- Level 6: Multiplication (*, /) ---
expr_mul:
    jsr     expr_power
.mul_loop:
    ; Skip spaces before checking for `*` or `/`. Without this,
    ; `PRINT 10 * (A>B)` mis-parses because the operator check at
    ; this point reads the space between `10` and `*`, returns the
    ; left operand alone, and PRINT then re-enters expr_eval on `*`
    ; (which expr_primary cannot evaluate, yielding the famous
    ; `100-1` artefact for the chapter-1 comparison example).
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .mul_check
    add.q   r17, r17, #1
    bra     .mul_loop
.mul_check:
    move.q  r2, #TK_MULT
    beq     r1, r2, .do_mul
    move.q  r2, #TK_DIV
    beq     r1, r2, .do_div
    rts

.do_mul:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_power
    jsr     expr_value_to_fp64
    pop     r22
	    move.q  r9, r8
	    move.q  r8, r22
	    jsr     fp_mul
	    move.q  r9, #VAL_F64
	    bra     .mul_loop

.do_div:
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_power
    jsr     expr_value_to_fp64
    pop     r22
    move.q  r23, r8
    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r23
    jsr     fp_cmp
    beqz    r8, .div_zero
    move.q  r8, r23
	    move.q  r9, r8
	    move.q  r8, r22
	    jsr     fp_div
	    move.q  r9, #VAL_F64
	    bra     .mul_loop
.div_zero:
    move.q  r8, #ERR_DIV0
    la      r9, err_msg_div0
    jsr     raise_error
    rts

; --- Level 7: Power (^) ---
expr_power:
    jsr     expr_unary
.pow_loop:
    ; Skip spaces before checking for the power operator `^`. Same
    ; pattern as expr_add/expr_mul/expr_compare — without it,
    ; `2 ^ 10` would stop at the first space.
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .pow_check
    add.q   r17, r17, #1
    bra     .pow_loop
.pow_check:
    move.q  r2, #TK_POWER
    bne     r1, r2, .pow_done
    add.q   r17, r17, #1
    jsr     expr_value_to_fp64
    push    r8
    jsr     expr_unary           ; right-associative
    jsr     expr_value_to_fp64
    pop     r22
	    move.q  r9, r8               ; R9 = exponent
	    move.q  r8, r22              ; R8 = base
	    jsr     fp_pow               ; R8 = base ^ exponent
	    move.q  r9, #VAL_F64
	    bra     .pow_loop
.pow_done:
    rts

; --- Level 8: Unary minus ---
expr_unary:
    ; Skip any leading spaces before reading the operand. The
    ; operator-level loops (expr_add/mul/power/compare) each call
    ; their right-recursion through this path; if those callers
    ; consume `<op>` and then the operand starts with a space (as it
    ; will whenever the source has spaces around the operator, e.g.
    ; `10 * (A>B)`), we must absorb the space before expr_primary
    ; tries to classify the byte. Without this skip, expr_primary
    ; reads the space, treats it as an unknown token, returns 0 with
    ; R17 unchanged, and the caller multiplies/adds against 0.
.unary_skip_sp:
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .unary_check
    add.q   r17, r17, #1
    bra     .unary_skip_sp

.unary_check:
    ; Check for unary minus (raw '-' character 0x2D OR TK_MINUS token 0xB2)
    move.q  r2, #TK_MINUS
    beq     r1, r2, .do_neg
    move.q  r2, #0x2D             ; raw '-' (before tokenization)
    beq     r1, r2, .do_neg

    ; Check for unary plus (just skip it)
    move.q  r2, #TK_PLUS
    beq     r1, r2, .do_uplus
    move.q  r2, #0x2B             ; raw '+'
    beq     r1, r2, .do_uplus

    ; No unary operator - fall through to primary
    bra     expr_primary

.do_neg:
	    add.q   r17, r17, #1
	    jsr     expr_unary           ; recursive for --x
	    jsr     expr_value_to_fp64
	    jsr     fp_neg               ; negate result
	    move.q  r9, #VAL_F64
	    rts

.do_uplus:
    add.q   r17, r17, #1
    jsr     expr_unary
    rts

; --- Level 9: Primary (number, parens, function) ---
expr_primary:
    load.b  r1, (r17)

    ; Check for opening parenthesis '('
    move.q  r2, #0x28             ; '('
    beq     r1, r2, .do_parens

    ; Check for digit or '.'
    move.q  r2, #0x30             ; '0'
    blt     r1, r2, .not_digit
    move.q  r2, #0x39             ; '9'
    ble     r1, r2, .do_number

.not_digit:
    move.q  r2, #0x2E             ; '.'
    beq     r1, r2, .do_number
    ; Check for '&' (hex literal prefix &H)
    move.q  r2, #0x26             ; '&'
    beq     r1, r2, .do_hex

    ; Check for DEF FN invocation.
    move.q  r2, #TK_FN
    beq     r1, r2, .do_fn_call
    move.q  r2, #0x46             ; raw "FN" prefix in names like FND
    beq     r1, r2, .maybe_raw_fn_call

    ; Check for extended functions (TK_EXT, subtoken).
    move.q  r2, #TK_EXT
    beq     r1, r2, .do_ext_function

    ; Check for built-in function tokens (0xBE..0xE1)
    move.q  r2, #TK_SGN
    blt     r1, r2, .not_func
    move.q  r2, #TK_MIDS
    ble     r1, r2, .do_function

    ; Check for chip STATUS functions (hardware tokens in expression context)
    move.q  r2, #TK_PSG_CMD
    beq     r1, r2, .do_chip_status
    move.q  r2, #TK_SID_CMD
    beq     r1, r2, .do_chip_status
    move.q  r2, #TK_POKEY_CMD
    beq     r1, r2, .do_chip_status
    move.q  r2, #TK_TED_CMD
    beq     r1, r2, .do_chip_status
    move.q  r2, #TK_AHX
    beq     r1, r2, .do_chip_status
    move.q  r2, #TK_SAP
    beq     r1, r2, .do_chip_status

.not_func:
    ; --- Intercept MOD STATUS (raw-text, no token) ---
    or.q    r3, r1, #0x20        ; normalize to lowercase
    move.q  r2, #0x6D            ; 'm'
    bne     r3, r2, .check_co_funcs
    load.b  r3, 1(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6F            ; 'o'
    bne     r3, r2, .check_co_funcs
    load.b  r3, 2(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x64            ; 'd'
    bne     r3, r2, .check_co_funcs
    ; Found "MOD" - skip 3 chars, skip spaces, verify "STATUS"
    add.q   r17, r17, #3
    ; Skip spaces
.mod_st_skip_sp:
    load.b  r2, (r17)
    move.q  r3, #0x20
    bne     r2, r3, .mod_st_after_sp
    add.q   r17, r17, #1
    bra     .mod_st_skip_sp
.mod_st_after_sp:
    ; Check first char is 'S'
    or.q    r2, r2, #0x20
    move.q  r3, #0x73            ; 's'
    bne     r2, r3, .mod_st_not_status
    add.q   r17, r17, #1
    ; Skip remaining alpha (TATUS...)
.mod_st_skip_alpha:
    load.b  r2, (r17)
    move.q  r3, #0x41            ; 'A'
    blt     r2, r3, .mod_st_done
    move.q  r3, #0x5A            ; 'Z'
    ble     r2, r3, .mod_st_next
    move.q  r3, #0x61            ; 'a'
    blt     r2, r3, .mod_st_done
    move.q  r3, #0x7A            ; 'z'
    bgt     r2, r3, .mod_st_done
.mod_st_next:
    add.q   r17, r17, #1
    bra     .mod_st_skip_alpha
.mod_st_done:
    ; Read MOD_PLAY_STATUS, return full status byte as FP64
    move.l  r1, #MOD_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts
.mod_st_not_status:
    ; Not "STATUS" after "MOD" - back up 3 chars so variable parsing can try
    sub.q   r17, r17, #3
    bra     .check_var_range

.check_co_funcs:
    ; --- Intercept coprocessor functions (COCALL, COSTATUS) ---
    move.q  r2, #0x63            ; 'c'
    bne     r3, r2, .check_var_range
    load.b  r3, 1(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6F            ; 'o'
    bne     r3, r2, .check_var_range
    load.b  r3, 2(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x63            ; 'c' → COCALL
    beq     r3, r2, .check_cocall_func
    move.q  r2, #0x73            ; 's' → COSTATUS
    beq     r3, r2, .check_costatus_func
    bra     .check_var_range

.check_cocall_func:
    ; Match "COCALL" (6 chars) + '('
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x61            ; 'a'
    bne     r3, r2, .check_var_range
    load.b  r3, 4(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6C            ; 'l'
    bne     r3, r2, .check_var_range
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x6C            ; 'l'
    bne     r3, r2, .check_var_range
    ; Check for '(' after keyword
    load.b  r3, 6(r17)
    move.q  r2, #0x28            ; '('
    bne     r3, r2, .check_var_range
    add.q   r17, r17, #7         ; consume "COCALL("
    jsr     fn_cocall
    rts

.check_costatus_func:
    ; Match "COSTATUS" (8 chars) + '('
    load.b  r3, 3(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .check_var_range
    load.b  r3, 4(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x61            ; 'a'
    bne     r3, r2, .check_var_range
    load.b  r3, 5(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x74            ; 't'
    bne     r3, r2, .check_var_range
    load.b  r3, 6(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x75            ; 'u'
    bne     r3, r2, .check_var_range
    load.b  r3, 7(r17)
    or.q    r3, r3, #0x20
    move.q  r2, #0x73            ; 's'
    bne     r3, r2, .check_var_range
    ; Check for '(' after keyword
    load.b  r3, 8(r17)
    move.q  r2, #0x28            ; '('
    bne     r3, r2, .check_var_range
    add.q   r17, r17, #9         ; consume "COSTATUS("
    jsr     fn_costatus
    rts

.check_var_range:
    ; Check for variable name (A-Z or a-z)
    move.q  r2, #0x41            ; 'A'
    blt     r1, r2, .not_var
    move.q  r2, #0x5A            ; 'Z'
    ble     r1, r2, .do_var
    move.q  r2, #0x61            ; 'a'
    blt     r1, r2, .not_var
    move.q  r2, #0x7A            ; 'z'
    ble     r1, r2, .do_var

.not_var:
    ; Unknown token - return 0
    move.q  r8, r0
    rts

.do_var:
    ; Variable reference - look it up
    jsr     var_read             ; R8 = FP64 value, R17 advanced past name
    rts

.do_parens:
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or              ; evaluate sub-expression
    ; Expect ')'
    load.b  r1, (r17)
    move.q  r2, #0x29             ; ')'
    bne     r1, r2, .paren_done
    add.q   r17, r17, #1         ; skip ')'
.paren_done:
    rts

.do_number:
    ; Parse numeric literal from token stream
    jsr     parse_number          ; R8 = FP64 result, R17 advanced
    rts

.do_hex:
    ; &Hxxxx hex literal
    add.q   r17, r17, #1         ; skip '&'
    load.b  r1, (r17)
    ; Accept 'H' or 'h'
    move.q  r2, #0x48             ; 'H'
    beq     r1, r2, .hex_start
    move.q  r2, #0x68             ; 'h'
    bne     r1, r2, .hex_bad
.hex_start:
    add.q   r17, r17, #1         ; skip 'H'
    move.q  r8, r0               ; accumulator
.hex_loop:
    load.b  r1, (r17)
    ; Check 0-9
    move.q  r2, #0x30
    blt     r1, r2, .hex_end
    move.q  r2, #0x39
    ble     r1, r2, .hex_digit_09
    ; Check A-F
    move.q  r2, #0x41
    blt     r1, r2, .hex_end
    move.q  r2, #0x46
    ble     r1, r2, .hex_digit_AF
    ; Check a-f
    move.q  r2, #0x61
    blt     r1, r2, .hex_end
    move.q  r2, #0x66
    ble     r1, r2, .hex_digit_af
    bra     .hex_end
.hex_digit_09:
    sub.q   r1, r1, #0x30
    bra     .hex_accum
.hex_digit_AF:
    sub.q   r1, r1, #0x37        ; 'A'-10 = 55
    bra     .hex_accum
.hex_digit_af:
    sub.q   r1, r1, #0x57        ; 'a'-10 = 87
.hex_accum:
    lsl.q   r8, r8, #4
    or.q    r8, r8, r1
    add.q   r17, r17, #1
    bra     .hex_loop
.hex_end:
    ; R8 = raw qword integer value.
    move.q  r9, #VAL_I64
    rts
.hex_bad:
    move.q  r8, r0
    rts

.maybe_raw_fn_call:
    load.b  r2, 1(r17)
    move.q  r3, #0x4E
    beq     r2, r3, .do_raw_fn_call
    move.q  r3, #0x6E
    beq     r2, r3, .do_raw_fn_call
    bra     .check_var_range

.do_fn_call:
    add.q   r17, r17, #1         ; consume FN
    bra     .fn_call_skip_sp
.do_raw_fn_call:
    add.q   r17, r17, #2         ; consume raw FN prefix
.fn_call_skip_sp:
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .fn_call_name
    add.q   r17, r17, #1
    bra     .fn_call_skip_sp
.fn_call_name:
    jsr     var_parse_name       ; R14 = function name tag
    la      r25, fn_table
    move.q  r3, #8
.fn_call_find:
    beqz    r3, .fn_call_undef
    load.l  r2, (r25)
    beq     r14, r2, .fn_call_found
    add.q   r25, r25, #24
    sub.q   r3, r3, #1
    bra     .fn_call_find
.fn_call_found:
.fn_call_skip_arg_sp:
    load.b  r1, (r17)
    move.q  r2, #0x20
    bne     r1, r2, .fn_call_lparen
    add.q   r17, r17, #1
    bra     .fn_call_skip_arg_sp
.fn_call_lparen:
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .fn_call_syntax
    add.q   r17, r17, #1
    push    r25
    jsr     expr_or              ; argument value
    pop     r25
    bnez    r28, .fn_call_done
    move.q  r22, r8
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .fn_call_syntax
    add.q   r17, r17, #1
    load.l  r2, 16(r25)
    bnez    r2, .fn_call_recursive
    move.q  r2, #1
    store.l r2, 16(r25)

    push    r17
    load.l  r14, 4(r25)
    jsr     var_lookup_tag       ; R8 = parameter variable slot
    move.q  r23, r8
    load.q  r24, (r23)           ; save caller value
    load.l  r1, -8(r23)          ; save caller scalar tag
    move.q  r2, #VAL_F64
    store.l r2, -8(r23)          ; parameter binding is a legacy FP64 slot
    store.q r22, (r23)           ; bind argument
    load.q  r17, 8(r25)
    push    r25
    push    r23
    push    r24
    push    r1
    jsr     expr_or
    move.q  r25, r8
    pop     r2
    pop     r24
    pop     r23
    pop     r1
    store.q r24, (r23)           ; restore caller value
    store.l r2, -8(r23)          ; restore caller scalar tag
    store.l r0, 16(r1)
    pop     r17
    move.q  r8, r25
    move.q  r9, #VAL_F64
.fn_call_done:
    rts
.fn_call_undef:
.fn_call_syntax:
    move.q  r8, #ERR_SYNTAX
    la      r9, err_msg_syntax
    jsr     raise_error
    rts
.fn_call_recursive:
    move.q  r8, #ERR_ILLEGAL_QTY
    la      r9, err_msg_illegal_qty
    jsr     raise_error
    rts

.do_function:
    ; Function dispatch
    load.b  r1, (r17)
    add.q   r17, r17, #1         ; consume function token

    ; Functions that take STRING arguments and return numbers
    ; must be handled before the generic numeric arg path
    move.q  r2, #TK_LEN
    beq     r1, r2, .fn_len
    move.q  r2, #TK_ASC
    beq     r1, r2, .fn_asc
    move.q  r2, #TK_VAL
    beq     r1, r2, .fn_val
    move.q  r2, #TK_SADD
    beq     r1, r2, .fn_sadd
    move.q  r2, #TK_VPTR
    beq     r1, r2, .fn_varptr

    ; Two-argument functions (must be intercepted before generic single-arg path)
    move.q  r2, #TK_BITTST
    beq     r1, r2, .fn_bittst
    move.q  r2, #TK_MAX
    beq     r1, r2, .fn_max
    move.q  r2, #TK_MIN
    beq     r1, r2, .fn_min

    ; PEEK / PEEK8 - intercepted for byte access variant
    move.q  r2, #TK_PEEK
    beq     r1, r2, .fn_peek_dispatch

    ; Expect '(' after function name
    load.b  r2, (r17)
    move.q  r3, #0x28             ; '('
    bne     r2, r3, .func_noarg

    push    r1                   ; save function token (clobbered by expr_or)
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_eval             ; evaluate argument
    ; Expect ')'
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .func_got_arg
    add.q   r17, r17, #1         ; skip ')'

.func_got_arg:
    pop     r1                   ; restore function token

.func_call:
    ; R1 = function token, R8 = argument
    ; Jump table dispatch: index = token - 0xBE, range 0..16
    sub.q   r1, r1, #0xBE
    move.q  r2, #17
    bge     r1, r2, .fn_unknown     ; out of range → return arg unchanged
    blt     r1, r0, .fn_unknown
    la      r2, .func_jump_table
    lsl.q   r3, r1, #2             ; index * 4
    add.q   r2, r2, r3
    load.l  r2, (r2)               ; handler address
    jsr     (r2)                   ; dispatch
    rts

.fn_unknown:
    ; Unknown function - return argument unchanged
    rts

.fn_sgn:
    move.q  r1, #VAL_I64
    beq     r9, r1, .fn_sgn_i64
    jsr     fp_cmp_zero
    move.q  r1, #0xFFFFFFFF
    bne     r8, r1, .fn_sgn_fp_done
    move.q  r8, r0
    not.q   r8, r8
.fn_sgn_fp_done:
    move.q  r9, #VAL_I64
    rts
.fn_sgn_i64:
    beqz    r8, .fn_sgn_i64_zero
    bgez    r8, .fn_sgn_i64_pos
    move.q  r8, r0
    not.q   r8, r8
    move.q  r9, #VAL_I64
    rts
.fn_sgn_i64_pos:
    move.q  r8, #1
    move.q  r9, #VAL_I64
    rts
.fn_sgn_i64_zero:
    move.q  r8, r0
    move.q  r9, #VAL_I64
    rts
.fn_int:
    move.q  r1, #VAL_I64
    beq     r9, r1, .fn_int_done
    jsr     fp_int
    jsr     fp_fix
    move.q  r9, #VAL_I64
.fn_int_done:
    rts
.fn_abs:
    move.q  r1, #VAL_I64
    beq     r9, r1, .fn_abs_i64
    jsr     fp_abs
    move.q  r9, #VAL_F64
    rts
.fn_abs_i64:
    bgez    r8, .fn_abs_i64_done
    neg.q   r8, r8
.fn_abs_i64_done:
    move.q  r9, #VAL_I64
    rts
.fn_sqr:
    jsr     expr_value_to_fp64
    jsr     fp_sqr
    move.q  r9, #VAL_F64
    rts
.fn_sin:
    jsr     expr_value_to_fp64
    jsr     fp_sin
    move.q  r9, #VAL_F64
    rts
.fn_cos:
    jsr     expr_value_to_fp64
    jsr     fp_cos
    move.q  r9, #VAL_F64
    rts
.fn_tan:
    jsr     expr_value_to_fp64
    jsr     fp_tan
    move.q  r9, #VAL_F64
    rts
.fn_atn:
    jsr     expr_value_to_fp64
    jsr     fp_atn
    move.q  r9, #VAL_F64
    rts
.fn_log:
    jsr     expr_value_to_fp64
    jsr     fp_log
    move.q  r9, #VAL_F64
    rts
.fn_exp:
    jsr     expr_value_to_fp64
    jsr     fp_exp
    move.q  r9, #VAL_F64
    rts
.fn_rnd:
    ; Simple LCG: state = state*1664525 + 1013904223.
    ; Return FP64 in [0,1): (state & 0x007FFFFF) / 2^23.
    la      r3, rnd_state
    load.l  r2, (r3)
    mulu.l  r2, r2, #1664525
    add.l   r2, r2, #1013904223
    store.l r2, (r3)

    and.l   r2, r2, #0x007FFFFF
    move.q  r8, r2
    jsr     fp_float
    li      r9, #0x4160000000000000 ; 2^23
    jsr     fp_div
    rts
.fn_peek:
    ; PEEK is the byte alias for PEEK8.
    ; This preserves the 8-bit home computer BASIC convention: bare PEEK reads
    ; one byte, and wider reads must use PEEK16/32/64.
    move.q  r23, #1
    bra     .peek_do_width

.fn_peek_dispatch:
    move.q  r23, #1              ; PEEK is byte-width by default
.peek_std_arg:
    ; Expect '('
    load.b  r2, (r17)
    move.q  r3, #0x28            ; '('
    bne     r2, r3, .peek_no_arg
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_eval            ; evaluate argument → R8/R9
    ; Expect ')'
    load.b  r2, (r17)
    move.q  r3, #0x29            ; ')'
    bne     r2, r3, .peek_do
    add.q   r17, r17, #1         ; skip ')'
.peek_do:
    move.q  r2, #VAL_I64
    beq     r9, r2, .peek_addr_i64
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
.peek_addr_i64:
    move.q  r1, r8
    bra     .peek_read_width
.peek_do_width:
    ; Generic single-argument path already evaluated the address into R8.
    move.q  r2, #VAL_I64
    beq     r9, r2, .peek_width_addr_i64
    jsr     fp_int
    jsr     fp_fix
.peek_width_addr_i64:
    move.q  r1, r8
.peek_read_width:
    move.q  r2, #1
    beq     r23, r2, .peek_read8
    move.q  r2, #2
    beq     r23, r2, .peek_read16
    move.q  r2, #4
    beq     r23, r2, .peek_read32
    and.l   r2, r1, #7
    bnez    r2, .peek_fc_error
    load.q  r8, (r1)
    move.q  r9, #VAL_I64
    rts
.peek_read8:
    load.b  r8, (r1)
    and.l   r8, r8, #0xFF
    move.q  r9, #VAL_I64
    rts
.peek_read16:
    and.l   r2, r1, #1
    bnez    r2, .peek_fc_error
    load.w  r8, (r1)
    and.l   r8, r8, #0xFFFF
    move.q  r9, #VAL_I64
    rts
.peek_read32:
    and.l   r2, r1, #3
    bnez    r2, .peek_fc_error
    load.l  r8, (r1)
    move.q  r9, #VAL_I64
    rts
.peek_fc_error:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    rts
.peek_no_arg:
    move.q  r8, r0
    jsr     fp_float
    rts

.do_ext_function:
    add.q   r17, r17, #1         ; consume TK_EXT
    load.b  r1, (r17)            ; subtoken
    add.q   r17, r17, #1
    move.q  r2, #EXT_MEMALLOC
    beq     r1, r2, .fn_memalloc
    move.q  r2, #EXT_PEEK8
    beq     r1, r2, .ext_peek8
    move.q  r2, #EXT_PEEK16
    beq     r1, r2, .ext_peek16
    move.q  r2, #EXT_PEEK32
    beq     r1, r2, .ext_peek32
    move.q  r2, #EXT_PEEK64
    beq     r1, r2, .ext_peek64
    move.q  r8, r0
    rts
.ext_peek8:
    move.q  r23, #1
    bra     .peek_std_arg
.ext_peek16:
    move.q  r23, #2
    bra     .peek_std_arg
.ext_peek32:
    move.q  r23, #4
    bra     .peek_std_arg
.ext_peek64:
    move.q  r23, #8
    bra     .peek_std_arg

.fn_memalloc:
    ; MEMALLOC(size[,align]) from public low32 DMA ranges. Alignment defaults
    ; to 4096 and is constrained to powers of two up to 1 MiB.
    load.b  r2, (r17)
    move.q  r3, #0x28            ; '('
    bne     r2, r3, .memalloc_fc_error
    add.q   r17, r17, #1
    jsr     expr_or
    jsr     .memalloc_fp_to_u32
    beqz    r9, .memalloc_fc_error
    move.q  r22, r8              ; size
    push    r22
    move.q  r23, #4096           ; alignment
    jsr     .expr_skip_spaces
    load.b  r2, (r17)
    move.q  r3, #0x2C            ; ','
    bne     r2, r3, .memalloc_after_align
    add.q   r17, r17, #1
    jsr     expr_or
    jsr     .memalloc_fp_to_u32
    beqz    r9, .memalloc_fc_error_pop
    move.q  r23, r8
.memalloc_after_align:
    pop     r22
    jsr     .expr_skip_spaces
    load.b  r2, (r17)
    move.q  r3, #0x29            ; ')'
    bne     r2, r3, .memalloc_fc_error
    add.q   r17, r17, #1
    beqz    r22, .memalloc_fc_error
    beqz    r23, .memalloc_fc_error
    move.q  r2, #1048576
    bgt     r23, r2, .memalloc_fc_error
    sub.q   r2, r23, #1
    and.q   r2, r2, r23
    bnez    r2, .memalloc_fc_error
    jsr     .basic_memalloc
    move.q  r9, #VAL_I64
.memalloc_done:
    rts
.memalloc_fc_error:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    rts
.memalloc_fc_error_pop:
    pop     r22
    bra     .memalloc_fc_error

; Convert the current tagged expression value in R8/R9 to an exact unsigned
; 32-bit integer.
; Output: R8 = integer, R9 = 1 on success; R9 = 0 on negative/fractional/overflow.
.memalloc_fp_to_u32:
    move.q  r1, #VAL_I64
    beq     r9, r1, .mftu_i64
    move.q  r22, r8               ; original FP64 payload
    jsr     fp_fix                ; truncates toward zero
    move.q  r23, r8               ; integer candidate
    move.q  r8, r23
    jsr     fp_float
    move.q  r9, r22
    jsr     fp_cmp
    bnez    r8, .mftu_fail
    move.q  r8, r23
.mftu_i64:
    blt     r8, r0, .mftu_fail
    li      r1, #0xFFFFFFFF
    bgt     r8, r1, .mftu_fail
    move.q  r9, #1
    rts
.mftu_fail:
    move.q  r9, r0
    rts

; Input R22=size, R23=align. Output R8=allocated pointer or raises ?OM.
.basic_memalloc:
    add.q   r10, r16, #ST_PRIV_MEMALLOC_CURSOR
    add.q   r11, r16, #ST_PRIV_MEMALLOC_RANGE
    load.q  r12, (r10)           ; cursor
    load.q  r13, (r11)           ; range index
    bnez    r12, .memalloc_have_cursor
    move.q  r12, #BASIC_MEMALLOC_BASE0
    move.q  r13, r0
.memalloc_have_cursor:
    sub.q   r1, r23, #1
    add.q   r8, r12, r1
    not.q   r1, r1
    and.q   r8, r8, r1           ; aligned result
    add.q   r2, r8, r22          ; new cursor
    blt     r2, r8, .memalloc_oom
    move.q  r3, #0
    beq     r13, r3, .memalloc_check0
    move.q  r3, #1
    beq     r13, r3, .memalloc_check1
    move.q  r3, #2
    beq     r13, r3, .memalloc_check2
    bra     .memalloc_oom
.memalloc_check0:
    move.q  r3, #BASIC_MEMALLOC_END0
    ble     r2, r3, .memalloc_commit
    move.q  r3, #BASIC_MEMALLOC_BASE1
    beqz    r3, .memalloc_oom
    move.q  r12, r3
    move.q  r13, #1
    bra     .memalloc_have_cursor
.memalloc_check1:
    move.q  r3, #BASIC_MEMALLOC_END1
    ble     r2, r3, .memalloc_commit
    move.q  r3, #BASIC_MEMALLOC_BASE2
    beqz    r3, .memalloc_oom
    move.q  r12, r3
    move.q  r13, #2
    bra     .memalloc_have_cursor
.memalloc_check2:
    move.q  r3, #BASIC_MEMALLOC_END2
    ble     r2, r3, .memalloc_commit
    bra     .memalloc_oom
.memalloc_commit:
    store.q r2, (r10)
    store.q r13, (r11)
    rts
.memalloc_oom:
    move.q  r8, #ERR_OOM
    la      r9, err_msg_oom
    jsr     raise_error
    rts

.expr_skip_spaces:
    load.b  r2, (r17)
    move.q  r3, #0x20
    bne     r2, r3, .expr_skip_done
    add.q   r17, r17, #1
    bra     .expr_skip_spaces
.expr_skip_done:
    rts
.fn_fre:
    ; Approximate free internal arena bytes: effective top - ST_HEAP_BOTTOM.
    add.q   r1, r16, #ST_FILE_BRIDGE_NEXT
    load.q  r2, (r1)
    bnez    r2, .fre_have_top
    add.q   r1, r16, #ST_HEAP_TOP
    load.q  r2, (r1)
.fre_have_top:
    add.q   r1, r16, #ST_HEAP_BOTTOM
    load.q  r8, (r1)
    sub.q   r8, r2, r8
    jsr     fp_float
    rts
.fn_usr:
    ; USR(addr) - call machine code at addr, return R8 as integer result
    ; R8/R9 = tagged address from generic single-arg path
    move.q  r1, #VAL_I64
    beq     r9, r1, .fn_usr_addr_ready
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
.fn_usr_addr_ready:
    move.q  r9, r8               ; R9 = call target
    ; Save BASIC state registers across machine-language call
    push    r14
    push    r16
    push    r17
    push    r26
    jsr     (r9)                 ; register-indirect JSR; ML returns result in R8
    pop     r26
    pop     r17
    pop     r16
    pop     r14
    ; R8 = integer result from machine code
    move.q  r9, #VAL_I64
    rts

.fn_bittst:
    ; BITTST(addr, bit) - returns -1 if set, 0 if not
    load.b  r2, (r17)
    move.q  r3, #0x28             ; '('
    bne     r2, r3, .fn_bittst_err
    add.q   r17, r17, #1
    jsr     expr_or               ; R8 = address
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8               ; R22 = address
    ; Expect comma
    load.b  r2, (r17)
    move.q  r3, #0x2C
    bne     r2, r3, .fn_bittst_err
    add.q   r17, r17, #1
    push    r22                   ; save address (expr_or clobbers R22)
    jsr     expr_or               ; R8 = bit number
    pop     r22                   ; restore address
    jsr     fp_int
    jsr     fp_fix
    ; Skip ')'
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_bittst_do
    add.q   r17, r17, #1
.fn_bittst_do:
    ; Test bit
    move.q  r1, #1
    lsl.l   r1, r1, r8           ; mask = 1 << bit
    load.b  r2, (r22)
    and.l   r2, r2, r1
    bnez    r2, .fn_bittst_true
    move.q  r8, r0               ; 0.0 (FALSE)
    rts
.fn_bittst_true:
    move.q  r8, r0
    not.q   r8, r8
    move.q  r9, #VAL_I64
    rts
.fn_bittst_err:
    move.q  r8, r0
    rts

.fn_max:
    ; MAX(a, b) - returns larger
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_max_err
    add.q   r17, r17, #1
    jsr     expr_or               ; R8 = first arg
    move.q  r22, r8
    load.b  r2, (r17)
    move.q  r3, #0x2C
    bne     r2, r3, .fn_max_err
    add.q   r17, r17, #1
    push    r22                   ; save first arg (expr_or clobbers R22)
    jsr     expr_or               ; R8 = second arg
    pop     r22                   ; restore first arg
    ; Skip ')'
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_max_do
    add.q   r17, r17, #1
.fn_max_do:
    move.q  r9, r8               ; R9 = second
    move.q  r8, r22              ; R8 = first
    push    r22                  ; save first value
    push    r9                   ; save second value
    jsr     fp_cmp               ; R8 = -1/0/1
    pop     r9
    pop     r22
    ; If R8 >= 0, first >= second → return first (R22)
    ; If R8 = -1 (0xFFFFFFFF), first < second → return second (R9)
    move.q  r1, #0xFFFFFFFF
    beq     r8, r1, .fn_max_second
    move.q  r8, r22
    rts
.fn_max_second:
    move.q  r8, r9
    rts
.fn_max_err:
    move.q  r8, r0
    rts

.fn_min:
    ; MIN(a, b) - returns smaller
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_min_err
    add.q   r17, r17, #1
    jsr     expr_or               ; R8 = first arg
    move.q  r22, r8
    load.b  r2, (r17)
    move.q  r3, #0x2C
    bne     r2, r3, .fn_min_err
    add.q   r17, r17, #1
    push    r22                   ; save first arg (expr_or clobbers R22)
    jsr     expr_or               ; R8 = second arg
    pop     r22                   ; restore first arg
    ; Skip ')'
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_min_do
    add.q   r17, r17, #1
.fn_min_do:
    move.q  r9, r8               ; R9 = second
    move.q  r8, r22              ; R8 = first
    push    r22
    push    r9
    jsr     fp_cmp
    pop     r9
    pop     r22
    ; If R8 = 1, first > second → return second (R9)
    ; Otherwise return first (R22)
    move.q  r1, #1
    beq     r8, r1, .fn_min_second
    move.q  r8, r22
    rts
.fn_min_second:
    move.q  r8, r9
    rts
.fn_min_err:
    move.q  r8, r0
    rts

.fn_len:
    ; LEN(string$) - string length as number
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_len_err
    add.q   r17, r17, #1         ; skip '('
    jsr     str_eval             ; R8 = string pointer
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_len_do
    add.q   r17, r17, #1         ; skip ')'
.fn_len_do:
    jsr     str_len              ; R8 = length (integer)
    jsr     fp_float             ; R8 = FP64
    rts
.fn_len_err:
    move.q  r8, r0
    rts

.fn_asc:
    ; ASC(string$) - ASCII code of first character
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_asc_err
    add.q   r17, r17, #1
    jsr     str_eval             ; R8 = string pointer
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_asc_do
    add.q   r17, r17, #1
.fn_asc_do:
    load.b  r8, (r8)             ; first byte
    and.l   r8, r8, #0xFF
    jsr     fp_float
    rts
.fn_asc_err:
    move.q  r8, r0
    rts

.fn_val:
    ; VAL(string$) - parse string as number
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_val_err
    add.q   r17, r17, #1
    jsr     str_eval             ; R8 = string pointer
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_val_do
    add.q   r17, r17, #1
.fn_val_do:
    ; Fast-path plain decimal strings as exact I64 before falling back to the
    ; tagged expression parser for legacy expression-shaped VAL inputs.
    push    r17
    push    r8
    jsr     val_parse_i64_string
    move.q  r1, #VAL_I64
    beq     r9, r1, .fn_val_i64_done
    pop     r8
    move.q  r17, r8
    jsr     expr_eval             ; R8/R9 = tagged value
    pop     r17
    rts
.fn_val_i64_done:
    pop     r2                    ; discard saved string pointer
    pop     r17
    rts
.fn_val_err:
    move.q  r8, r0
    rts

.fn_sadd:
    ; SADD(A$) returns the live address of string data, so POKE8 through it
    ; mutates the variable bytes like historical BASIC implementations.
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_sadd_err
    add.q   r17, r17, #1
    jsr     var_lookup           ; R8 = str_ptr slot, R9 = 1 for string
    beqz    r9, .fn_sadd_err
    load.q  r8, (r8)             ; R8 = internal string data pointer
    beqz    r8, .fn_sadd_err
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_sadd_return
    add.q   r17, r17, #1
.fn_sadd_return:
    move.q  r9, #VAL_I64
    rts
.fn_sadd_err:
    move.q  r8, r0
    move.q  r9, #VAL_I64
    rts
.fn_varptr:
    ; VARPTR(A) - return the public variable cell address.
    load.b  r2, (r17)
    move.q  r3, #0x28
    bne     r2, r3, .fn_varptr_err
    add.q   r17, r17, #1
    jsr     var_lookup           ; R8 = numeric value slot or string pointer slot
    bnez    r9, .fn_varptr_after_adjust
    load.l  r1, -8(r8)
    move.q  r2, #VAL_F64
    beq     r1, r2, .fn_varptr_scalar
    move.q  r2, #VAL_I64
    bne     r1, r2, .fn_varptr_after_adjust
.fn_varptr_scalar:
    sub.q   r8, r8, #8           ; scalar numeric public cell: tag,reserved,payload
.fn_varptr_after_adjust:
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .fn_varptr_return
    add.q   r17, r17, #1
.fn_varptr_return:
    move.q  r9, #VAL_I64
    rts
.fn_varptr_err:
    move.q  r8, r0
    move.q  r9, #VAL_I64
    rts

.func_noarg:
    ; Function with no parentheses (PI, TWOPI)
    move.q  r2, #TK_PI
    beq     r1, r2, .fn_pi
    move.q  r2, #TK_TWOPI
    beq     r1, r2, .fn_twopi
    move.q  r8, r0
    rts

.fn_pi:
    li      r8, #0x400921FB54442D18 ; PI as FP64
    move.q  r9, #VAL_F64
    rts
.fn_twopi:
    li      r8, #0x401921FB54442D18 ; 2*PI as FP64
    move.q  r9, #VAL_F64
    rts

; ============================================================================
; Chip STATUS functions - PSG/SID/POKEY/TED/AHX/SAP STATUS
; ============================================================================
; Reached when a chip token (TK_PSG_CMD, etc.) appears in expression context.
; R1 = chip token, R17 = text pointer (pointing at the token byte, not consumed)
.do_chip_status:
    add.q   r17, r17, #1         ; consume chip token
    push    r1                   ; save chip token
    ; Skip spaces (inline - exec_skip_spaces may not be available in expr-only tests)
.cs_skip_sp:
    load.b  r2, (r17)
    move.q  r3, #0x20
    bne     r2, r3, .cs_after_sp
    add.q   r17, r17, #1
    bra     .cs_skip_sp
.cs_after_sp:
    ; Check for "STATUS" - verify first char 'S', skip remaining alpha
    load.b  r2, (r17)
    move.q  r3, #0x53            ; 'S'
    bne     r2, r3, .chip_status_err
    add.q   r17, r17, #1         ; skip 'S'
    ; Inline skip_alpha: skip "TATUS" and any trailing alpha
.cs_skip_alpha:
    load.b  r2, (r17)
    move.q  r3, #0x41            ; 'A'
    blt     r2, r3, .cs_alpha_done
    move.q  r3, #0x5A            ; 'Z'
    ble     r2, r3, .cs_alpha_next
    move.q  r3, #0x61            ; 'a'
    blt     r2, r3, .cs_alpha_done
    move.q  r3, #0x7A            ; 'z'
    bgt     r2, r3, .cs_alpha_done
.cs_alpha_next:
    add.q   r17, r17, #1
    bra     .cs_skip_alpha
.cs_alpha_done:
    ; Dispatch based on chip token to load correct MMIO address
    pop     r1
    move.q  r2, #TK_PSG_CMD
    beq     r1, r2, .chip_psg_st
    move.q  r2, #TK_SID_CMD
    beq     r1, r2, .chip_sid_st
    move.q  r2, #TK_POKEY_CMD
    beq     r1, r2, .chip_pokey_st
    move.q  r2, #TK_TED_CMD
    beq     r1, r2, .chip_ted_st
    move.q  r2, #TK_AHX
    beq     r1, r2, .chip_ahx_st
    move.q  r2, #TK_SAP
    beq     r1, r2, .chip_sap_st
    ; Unknown chip - return 0
    move.q  r8, r0
    jsr     fp_float
    rts

.chip_psg_st:
    move.l  r1, #PSG_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts
.chip_sid_st:
    move.l  r1, #SID_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts
.chip_pokey_st:
    move.l  r1, #SAP_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts
.chip_ted_st:
    move.l  r1, #TED_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts
.chip_ahx_st:
    move.l  r1, #AHX_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts
.chip_sap_st:
    move.l  r1, #SAP_PLAY_STATUS
    load.l  r8, (r1)
    jsr     fp_float
    rts

.chip_status_err:
    ; Not "STATUS" - pop saved token, return 0
    pop     r1
    move.q  r8, r0
    jsr     fp_float
    rts

.fn_pos:
    ; POS(x) - current terminal output column.
    add.q   r1, r16, #ST_TERM_COL
    load.l  r8, (r1)
    jsr     fp_float
    rts

; Function dispatch jump table (17 entries, tokens 0xBE-0xCE)
    align 4
.func_jump_table:
    dc.l    .fn_sgn              ; 0xBE TK_SGN
    dc.l    .fn_int              ; 0xBF TK_INT
    dc.l    .fn_abs              ; 0xC0 TK_ABS
    dc.l    .fn_usr              ; 0xC1 TK_USR
    dc.l    .fn_fre              ; 0xC2 TK_FRE
    dc.l    .fn_pos              ; 0xC3 TK_POS
    dc.l    .fn_sqr              ; 0xC4 TK_SQR
    dc.l    .fn_rnd              ; 0xC5 TK_RND
    dc.l    .fn_log              ; 0xC6 TK_LOG
    dc.l    .fn_exp              ; 0xC7 TK_EXP
    dc.l    .fn_cos              ; 0xC8 TK_COS
    dc.l    .fn_sin              ; 0xC9 TK_SIN
    dc.l    .fn_tan              ; 0xCA TK_TAN
    dc.l    .fn_atn              ; 0xCB TK_ATN
    dc.l    .fn_peek             ; 0xCC TK_PEEK
    dc.l    .fn_unknown          ; 0xCD removed
    dc.l    .fn_unknown          ; 0xCE removed

rnd_state:
    dc.l    0x12345678

; ============================================================================
; parse_number - Parse a numeric literal from the token stream
; ============================================================================
; Input:  R17 = pointer to start of number in token stream
; Output: R8  = FP64 result
;         R17 = advanced past the number
; Clobbers: R1-R7
;
; Handles: integer (123), float (3.14), leading dot (.5), scientific notation.

parse_number:
    push    r14
    push    r15

    ; Accumulate integer part as raw int
    move.q  r1, r0               ; R1 = integer accumulator
    move.q  r14, r0              ; R14 = saw decimal point flag
    move.q  r15, r0              ; R15 = decimal divisor (FP64 bits of 10^n)
    move.q  r2, r0               ; R2 = number of fractional digits

.pn_loop:
    load.b  r3, (r17)

    ; Check for digit
    move.q  r4, #0x30             ; '0'
    blt     r3, r4, .pn_not_digit
    move.q  r4, #0x39             ; '9'
    bgt     r3, r4, .pn_not_digit

    ; It's a digit: accumulate
    sub.q   r3, r3, #0x30        ; digit value
    ; R1 = R1 * 10 + digit
    move.q  r5, r1
    mulu.q  r5, r5, #10
    add.q   r1, r5, r3
    add.q   r17, r17, #1

    ; If past decimal point, count fractional digits
    bnez    r14, .pn_frac_digit
    bra     .pn_loop

.pn_frac_digit:
    add.q   r2, r2, #1           ; fractional digit count++
    bra     .pn_loop

.pn_not_digit:
    ; Check for decimal point
    move.q  r4, #0x2E             ; '.'
    bne     r3, r4, .pn_done
    bnez    r14, .pn_done         ; already saw decimal point
    move.q  r14, #1               ; set decimal flag
    add.q   r17, r17, #1
    bra     .pn_loop

.pn_done:
    ; Save fractional digit count before fp_float clobbers R2
    push    r2

    ; Convert integer accumulator to FP64
    move.q  r8, r1
    jsr     fp_float              ; R8 = FP64 of integer value

    ; Restore fractional digit count
    pop     r2

    ; If there were fractional digits, divide by 10^n using lookup table
    beqz    r2, .pn_end
    ; Clamp n to 8 (max table entries)
    move.q  r3, #8
    ble     r2, r3, .pn_div_lookup
    move.q  r2, r3
.pn_div_lookup:
    ; Table index: (n-1) * 8
    sub.q   r3, r2, #1
    lsl.q   r3, r3, #3
    la      r4, .fp_pow10_table
    add.q   r4, r4, r3
    load.q  r9, (r4)              ; R9 = 10^n (FP64)
    jsr     fp_div                ; R8 = value / 10^n

.pn_end:
    ; Optional scientific exponent: E[+|-]digits
    load.b  r3, (r17)
    move.q  r4, #0x45             ; 'E'
    beq     r3, r4, .pn_exp_start
    move.q  r4, #0x65             ; 'e'
    bne     r3, r4, .pn_return
.pn_exp_start:
    add.q   r17, r17, #1
    move.q  r6, r0                ; sign flag: 0 positive, 1 negative
    load.b  r3, (r17)
    move.q  r4, #TK_MINUS
    beq     r3, r4, .pn_exp_neg
    move.q  r4, #0x2D             ; '-'
    beq     r3, r4, .pn_exp_neg
    move.q  r4, #TK_PLUS
    beq     r3, r4, .pn_exp_pos
    move.q  r4, #0x2B             ; '+'
    beq     r3, r4, .pn_exp_pos
    bra     .pn_exp_digits
.pn_exp_neg:
    move.q  r6, #1
.pn_exp_pos:
    add.q   r17, r17, #1
.pn_exp_digits:
    move.q  r5, r0                ; exponent accumulator
.pn_exp_loop:
    load.b  r3, (r17)
    move.q  r4, #0x30
    blt     r3, r4, .pn_exp_apply
    move.q  r4, #0x39
    bgt     r3, r4, .pn_exp_apply
    sub.q   r3, r3, #0x30
    mulu.l  r5, r5, #10
    add.l   r5, r5, r3
    add.q   r17, r17, #1
    bra     .pn_exp_loop
.pn_exp_apply:
    beqz    r5, .pn_return
    bnez    r6, .pn_exp_div_loop
.pn_exp_mul_loop:
    li      r9, #0x4024000000000000 ; 10.0
    jsr     fp_mul
    sub.q   r5, r5, #1
    bnez    r5, .pn_exp_mul_loop
    bra     .pn_return
.pn_exp_div_loop:
    li      r9, #0x4024000000000000 ; 10.0
    jsr     fp_div
    sub.q   r5, r5, #1
    bnez    r5, .pn_exp_div_loop

.pn_return:
    pop     r15
    pop     r14
    rts

; Power-of-10 lookup table (FP64 representations)
    align 8
.fp_pow10_table:
    dc.q    0x4024000000000000    ; 10^1 = 10
    dc.q    0x4059000000000000    ; 10^2 = 100
    dc.q    0x408F400000000000    ; 10^3 = 1000
    dc.q    0x40C3880000000000    ; 10^4 = 10000
    dc.q    0x40F86A0000000000    ; 10^5 = 100000
    dc.q    0x412E848000000000    ; 10^6 = 1000000
    dc.q    0x416312D000000000    ; 10^7 = 10000000
    dc.q    0x4197D78400000000    ; 10^8 = 100000000

; ============================================================================
; fp64_to_int_pair - Convert R22 (FP64) and R8 (FP64) to integers
; ============================================================================
; Output: R1 = int32 from R22, R2 = int32 from R8
; Clobbers: R3-R7

fp64_to_int_pair:
    push    r8
    move.q  r8, r22
    jsr     fp_int
    jsr     fp_fix               ; R8 = signed int32 of left
    move.q  r1, r8
    pop     r8
    push    r1                   ; save left int32 (fp_fix clobbers R1)
    jsr     fp_int
    jsr     fp_fix               ; R8 = signed int32 of right
    move.q  r2, r8
    pop     r1                   ; restore left int32
    rts

; expr_value_to_fp64 - Coerce the current tagged expression value to FP64.
; Input: R8=payload, R9=tag. Output: R8=FP64 bits, R9=VAL_F64.
expr_value_to_fp64:
    move.q  r1, #VAL_I64
    bne     r9, r1, .evtf_done
    jsr     fp_float
.evtf_done:
    move.q  r9, #VAL_F64
    rts

; expr_truthy - Convert a tagged expression result to BASIC truthiness.
; Input: R8=payload, R9=tag. Output: R8=1 true or 0 false, R9=VAL_I64.
expr_truthy:
    move.q  r1, #VAL_I64
    beq     r9, r1, .etr_i64
    bra     .etr_f64
.etr_i64:
    beqz    r8, .etr_false
    bra     .etr_true
.etr_f64:
    move.q  r7, r8
    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r7
    jsr     fp_cmp
    bnez    r8, .etr_true
    bra     .etr_false
.etr_true:
    move.q  r8, #1
    move.q  r9, #VAL_I64
    rts
.etr_false:
    move.q  r8, r0
    move.q  r9, #VAL_I64
    rts

; val_parse_i64_string - Parse a NUL-terminated decimal string exactly.
; Input: R8=string pointer. Output: R8=value, R9=VAL_I64 on success; R9=0 on fail.
val_parse_i64_string:
    move.q  r1, r8                ; cursor
    move.q  r3, r0                ; accumulator
    move.q  r4, r0                ; sign flag
    move.q  r6, r0                ; saw digit flag
.vpi_skip_lead:
    load.b  r2, (r1)
    move.q  r5, #0x20
    bne     r2, r5, .vpi_sign
    add.q   r1, r1, #1
    bra     .vpi_skip_lead
.vpi_sign:
    move.q  r5, #0x2D             ; '-'
    bne     r2, r5, .vpi_plus
    move.q  r4, #1
    add.q   r1, r1, #1
    bra     .vpi_digits
.vpi_plus:
    move.q  r5, #0x2B             ; '+'
    bne     r2, r5, .vpi_digits
    add.q   r1, r1, #1
.vpi_digits:
    load.b  r2, (r1)
    beqz    r2, .vpi_done
    move.q  r5, #0x22
    beq     r2, r5, .vpi_done
    move.q  r5, #0x20
    beq     r2, r5, .vpi_trailing
    move.q  r5, #0x30
    blt     r2, r5, .vpi_fail
    move.q  r5, #0x39
    bgt     r2, r5, .vpi_fail
    sub.q   r2, r2, #0x30
    mulu.q  r3, r3, #10
    add.q   r3, r3, r2
    move.q  r6, #1
    add.q   r1, r1, #1
    bra     .vpi_digits
.vpi_trailing:
    add.q   r1, r1, #1
    load.b  r2, (r1)
    beqz    r2, .vpi_done
    move.q  r5, #0x20
    beq     r2, r5, .vpi_trailing
    bra     .vpi_fail
.vpi_done:
    beqz    r6, .vpi_fail
    beqz    r4, .vpi_positive
    neg.q   r3, r3
.vpi_positive:
    move.q  r8, r3
    move.q  r9, #VAL_I64
    rts
.vpi_fail:
    move.q  r8, r0
    move.q  r9, r0
    rts

; ============================================================================
; fp_fix - Convert FP64 to signed int32
; ============================================================================
; Input:  R8 = FP64 value (must be an integer value, e.g. after fp_int)
; Output: R8 = signed 32-bit integer
; Clobbers: R1-R4
;
; Strategy: extract sign, exponent, mantissa from IEEE 754 bits,
; reconstruct integer value.

fp_fix:
    move.q  r1, #VAL_I64
    beq     r9, r1, .fp_fix_i64
    jsr     fp64_load_r8_f0
    dcvtfi  r8, f0            ; double -> int64 (truncates toward zero, saturates)
.fp_fix_i64:
    rts

; ============================================================================
; fp_cmp_zero - Compare R8 with zero, return SGN result
; ============================================================================
; Output: R8 = -1 if negative, 0 if zero, 1 if positive
; Clobbers: R1, R2

fp_cmp_zero:
    ; Check if zero (both +0 and -0)
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .cz_zero

    ; Check sign bit
    lsr.l   r1, r8, #31
    bnez    r1, .cz_neg

    ; Positive
    move.q  r8, #1
    rts

.cz_neg:
    move.q  r8, #0xFFFFFFFF      ; -1 as 32-bit signed
    ; Sign-extend to 64-bit for fp_float
    move.l  r8, #0xFFFFFFFF
    rts

.cz_zero:
    move.q  r8, r0
    rts

; basic_memalloc_public - public low32 allocator used by hardware builders.
; Input R22=size, R23=align. Output R8=allocated pointer or raises ?OM.
; Clobbers R1-R3, R8, R10-R13.
basic_memalloc_public:
    add.q   r10, r16, #ST_PRIV_MEMALLOC_CURSOR
    add.q   r11, r16, #ST_PRIV_MEMALLOC_RANGE
    load.q  r12, (r10)
    load.q  r13, (r11)
    bnez    r12, bmp_have_cursor
    move.q  r12, #BASIC_MEMALLOC_BASE0
    move.q  r13, r0
bmp_have_cursor:
    sub.q   r1, r23, #1
    add.q   r8, r12, r1
    not.q   r1, r1
    and.q   r8, r8, r1
    add.q   r2, r8, r22
    blt     r2, r8, bmp_oom
    move.q  r3, #0
    beq     r13, r3, bmp_check0
    move.q  r3, #1
    beq     r13, r3, bmp_check1
    move.q  r3, #2
    beq     r13, r3, bmp_check2
    bra     bmp_oom
bmp_check0:
    move.q  r3, #BASIC_MEMALLOC_END0
    ble     r2, r3, bmp_commit
    move.q  r12, #BASIC_MEMALLOC_BASE1
    move.q  r13, #1
    bra     bmp_have_cursor
bmp_check1:
    move.q  r3, #BASIC_MEMALLOC_END1
    ble     r2, r3, bmp_commit
    move.q  r12, #BASIC_MEMALLOC_BASE2
    move.q  r13, #2
    bra     bmp_have_cursor
bmp_check2:
    move.q  r3, #BASIC_MEMALLOC_END2
    ble     r2, r3, bmp_commit
    bra     bmp_oom
bmp_commit:
    store.q r2, (r10)
    store.q r13, (r11)
    rts
bmp_oom:
    move.q  r8, #ERR_OOM
    la      r9, err_msg_oom
    jsr     raise_error
    rts

basic_memalloc_reset:
    add.q   r1, r16, #ST_PRIV_MEMALLOC_CURSOR
    store.q r0, (r1)
    add.q   r1, r16, #ST_PRIV_MEMALLOC_RANGE
    store.q r0, (r1)
    add.q   r1, r16, #ST_COPPER_BUILD
    store.q r0, (r1)
    rts

; ============================================================================
; EOF
; ============================================================================
