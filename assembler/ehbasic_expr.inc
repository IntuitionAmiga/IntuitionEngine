; ehbasic_expr.inc - EhBASIC IE64 Expression Evaluator
;
; Recursive-descent expression evaluator for numeric expressions.
; Supports: integer/float literals, +, -, *, /, ^, <, >, =, AND, OR,
;           EOR, NOT, unary minus, parentheses.
;
; All values are IEEE 754 FP32 stored in the lower 32 bits of 64-bit regs.
;
; Calling convention:
;   expr_eval:
;     Input:  R17 = pointer to tokenized expression (null-terminated)
;     Output: R8  = result as FP32 bits
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
; Returns result in R8 (FP32).

expr_eval:
    push    r14
    push    r15
    push    r22
    push    r23
    push    r24
    push    r25

    jsr     expr_or              ; evaluate lowest-precedence level
    ; Result in R8

    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r15
    pop     r14
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
    jsr     expr_and
    move.q  r22, r8              ; save left operand
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
    move.q  r8, r22
    rts

.do_or:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_and
    pop     r22
    ; OR: convert both to int, bitwise OR
    jsr     fp32_to_int_pair     ; R22→R1(int), R8→R2(int)
    or.l    r1, r1, r2
    move.q  r8, r1
    jsr     fp_float             ; int→FP32
    move.q  r22, r8
    bra     .eor_loop

.do_eor:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_and
    pop     r22
    jsr     fp32_to_int_pair
    eor.l   r1, r1, r2
    move.q  r8, r1
    jsr     fp_float
    move.q  r22, r8
    bra     .eor_loop

; --- Level 2: AND (with inlined NOT check) ---
expr_and:
    jsr     expr_not_inline
    move.q  r22, r8
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
    push    r22
    jsr     expr_not_inline
    pop     r22
    jsr     fp32_to_int_pair
    and.l   r1, r1, r2
    move.q  r8, r1
    jsr     fp_float
    move.q  r22, r8
    bra     .and_loop
.and_done:
    move.q  r8, r22
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
    jsr     fp_fix               ; FP32→signed int32 in R8
    move.q  r1, r8
    pop     r8                   ; (discard)
    move.q  r2, #0xFFFFFFFF
    eor.l   r1, r1, r2           ; bitwise NOT
    move.q  r8, r1
    jsr     fp_float             ; int→FP32
    rts

; --- Level 4: Comparison (<, >, =) ---
expr_compare:
    jsr     expr_add
    move.q  r22, r8
.cmp_loop:
    load.b  r1, (r17)
    move.q  r2, #TK_LT
    beq     r1, r2, .do_lt
    move.q  r2, #TK_GT
    beq     r1, r2, .do_gt
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .do_eq
    move.q  r8, r22
    rts

.do_lt:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_add
    pop     r22
    ; Compare: R22 < R8?
    move.q  r9, r8               ; R9 = right
    move.q  r8, r22              ; R8 = left
    jsr     fp_cmp               ; R8 = -1/0/1
    ; If result = -1 (0xFFFFFFFF), left < right → TRUE (-1.0)
    move.q  r2, #0xFFFFFFFF
    beq     r8, r2, .cmp_true
    bra     .cmp_false

.do_gt:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_add
    pop     r22
    move.q  r9, r8
    move.q  r8, r22
    jsr     fp_cmp
    move.q  r2, #1
    beq     r8, r2, .cmp_true
    bra     .cmp_false

.do_eq:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_add
    pop     r22
    move.q  r9, r8
    move.q  r8, r22
    jsr     fp_cmp
    beqz    r8, .cmp_true
    bra     .cmp_false

.cmp_true:
    move.l  r8, #0xBF800000      ; -1.0 (TRUE)
    move.q  r22, r8
    bra     .cmp_loop

.cmp_false:
    move.q  r8, r0               ; 0.0 (FALSE)
    move.q  r22, r8
    bra     .cmp_loop

; --- Level 5: Addition (+, -) ---
expr_add:
    jsr     expr_mul
    move.q  r22, r8
.add_loop:
    load.b  r1, (r17)
    move.q  r2, #TK_PLUS
    beq     r1, r2, .do_add
    move.q  r2, #TK_MINUS
    beq     r1, r2, .do_sub
    move.q  r8, r22
    rts

.do_add:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_mul
    pop     r22
    move.q  r9, r8               ; R9 = right
    move.q  r8, r22              ; R8 = left
    jsr     fp_add               ; R8 = left + right
    move.q  r22, r8
    bra     .add_loop

.do_sub:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_mul
    pop     r22
    move.q  r9, r8               ; R9 = right
    move.q  r8, r22              ; R8 = left
    jsr     fp_sub               ; R8 = left - right
    move.q  r22, r8
    bra     .add_loop

; --- Level 6: Multiplication (*, /) ---
expr_mul:
    jsr     expr_power
    move.q  r22, r8
.mul_loop:
    load.b  r1, (r17)
    move.q  r2, #TK_MULT
    beq     r1, r2, .do_mul
    move.q  r2, #TK_DIV
    beq     r1, r2, .do_div
    move.q  r8, r22
    rts

.do_mul:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_power
    pop     r22
    move.q  r9, r8
    move.q  r8, r22
    jsr     fp_mul
    move.q  r22, r8
    bra     .mul_loop

.do_div:
    add.q   r17, r17, #1
    push    r22
    jsr     expr_power
    pop     r22
    move.q  r9, r8
    move.q  r8, r22
    jsr     fp_div
    move.q  r22, r8
    bra     .mul_loop

; --- Level 7: Power (^) ---
expr_power:
    jsr     expr_unary
    move.q  r22, r8
.pow_loop:
    load.b  r1, (r17)
    move.q  r2, #TK_POWER
    bne     r1, r2, .pow_done
    add.q   r17, r17, #1
    push    r22
    jsr     expr_unary           ; right-associative
    pop     r22
    move.q  r9, r8               ; R9 = exponent
    move.q  r8, r22              ; R8 = base
    jsr     fp_pow               ; R8 = base ^ exponent
    move.q  r22, r8
    bra     .pow_loop
.pow_done:
    move.q  r8, r22
    rts

; --- Level 8: Unary minus ---
expr_unary:
    load.b  r1, (r17)

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

    ; No unary operator — fall through to primary
    bra     expr_primary

.do_neg:
    add.q   r17, r17, #1
    jsr     expr_unary           ; recursive for --x
    jsr     fp_neg               ; negate result
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
    ; --- Intercept coprocessor functions (COCALL, COSTATUS) ---
    or.q    r3, r1, #0x20        ; normalize to lowercase
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
    ; Unknown token — return 0
    move.q  r8, r0
    rts

.do_var:
    ; Variable reference — look it up
    jsr     var_read             ; R8 = FP32 value, R17 advanced past name
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
    jsr     parse_number          ; R8 = FP32 result, R17 advanced
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
    lsl.l   r8, r8, #4
    or.l    r8, r8, r1
    add.q   r17, r17, #1
    bra     .hex_loop
.hex_end:
    ; R8 = integer value, convert to FP32
    jsr     fp_float
    rts
.hex_bad:
    move.q  r8, r0
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

    ; Two-argument functions (must be intercepted before generic single-arg path)
    move.q  r2, #TK_BITTST
    beq     r1, r2, .fn_bittst
    move.q  r2, #TK_MAX
    beq     r1, r2, .fn_max
    move.q  r2, #TK_MIN
    beq     r1, r2, .fn_min

    ; PEEK / PEEK8 — intercepted for byte access variant
    move.q  r2, #TK_PEEK
    beq     r1, r2, .fn_peek_dispatch

    ; Expect '(' after function name
    load.b  r2, (r17)
    move.q  r3, #0x28             ; '('
    bne     r2, r3, .func_noarg

    push    r1                   ; save function token (clobbered by expr_or)
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or              ; evaluate argument
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
    ; Unknown function — return argument unchanged
    rts

.fn_sgn:
    jsr     fp_cmp_zero
    jsr     fp_float
    rts
.fn_int:
    jsr     fp_int
    rts
.fn_abs:
    jsr     fp_abs
    rts
.fn_sqr:
    jsr     fp_sqr
    rts
.fn_sin:
    jsr     fp_sin
    rts
.fn_cos:
    jsr     fp_cos
    rts
.fn_tan:
    jsr     fp_tan
    rts
.fn_atn:
    jsr     fp_atn
    rts
.fn_log:
    jsr     fp_log
    rts
.fn_exp:
    jsr     fp_exp
    rts
.fn_rnd:
    ; Simple LCG: state = state*1664525 + 1013904223
    ; Return FP32 in [0,1): ((state & 0x007FFFFF) | 0x3F800000) - 1.0
    la      r3, rnd_state
    load.l  r2, (r3)
    mulu.l  r2, r2, #1664525
    add.l   r2, r2, #1013904223
    store.l r2, (r3)

    and.l   r2, r2, #0x007FFFFF
    move.l  r8, #0x3F800000
    or.l    r8, r8, r2
    move.l  r9, #0x3F800000
    jsr     fp_sub
    rts
.fn_peek:
    ; Argument is address (numeric). Return 32-bit value at address as FP32.
    ; (reached from generic single-arg path — always 32-bit)
    jsr     fp_int
    jsr     fp_fix               ; R8 = int32 address
    move.q  r1, r8
    load.l  r8, (r1)
    jsr     fp_float
    rts

.fn_peek_dispatch:
    ; Check for PEEK8 (byte access) vs PEEK (32-bit)
    load.b  r2, (r17)
    move.q  r3, #0x38            ; '8'
    move.q  r23, r0              ; R23 = 0 (32-bit mode)
    bne     r2, r3, .peek_std_arg
    move.q  r23, #1              ; R23 = 1 (byte mode)
    add.q   r17, r17, #1         ; consume '8'
.peek_std_arg:
    ; Expect '('
    load.b  r2, (r17)
    move.q  r3, #0x28            ; '('
    bne     r2, r3, .peek_no_arg
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or              ; evaluate argument → R8
    ; Expect ')'
    load.b  r2, (r17)
    move.q  r3, #0x29            ; ')'
    bne     r2, r3, .peek_do
    add.q   r17, r17, #1         ; skip ')'
.peek_do:
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
    move.q  r1, r8
    beqz    r23, .peek_read32
    ; PEEK8 — byte read
    load.b  r8, (r1)
    and.l   r8, r8, #0xFF
    jsr     fp_float
    rts
.peek_read32:
    ; PEEK — 32-bit read
    load.l  r8, (r1)
    jsr     fp_float
    rts
.peek_no_arg:
    move.q  r8, r0
    jsr     fp_float
    rts
.fn_fre:
    ; Approximate free bytes: BASIC_STR_END - ST_VAR_END
    add.q   r1, r16, #ST_VAR_END
    load.l  r1, (r1)
    move.l  r8, #BASIC_STR_END
    sub.l   r8, r8, r1
    jsr     fp_float
    rts
.fn_deek:
    ; DEEK(addr) — read 16-bit value at address
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
    move.q  r1, r8
    load.w  r8, (r1)             ; 16-bit read
    and.l   r8, r8, #0xFFFF
    jsr     fp_float
    rts
.fn_leek:
    ; LEEK(addr) — read 32-bit value (same as PEEK)
    bra     .fn_peek

.fn_usr:
    ; USR(addr) — call machine code at addr, return R8 as integer result
    ; R8 = FP32 address from generic single-arg path
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer address
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
    jsr     fp_float             ; convert to FP32
    rts

.fn_bittst:
    ; BITTST(addr, bit) — returns -1 if set, 0 if not
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
    move.l  r8, #0xBF800000      ; -1.0 (TRUE)
    rts
.fn_bittst_err:
    move.q  r8, r0
    rts

.fn_max:
    ; MAX(a, b) — returns larger
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
    ; MIN(a, b) — returns smaller
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
    ; LEN(string$) — string length as number
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
    jsr     fp_float             ; R8 = FP32
    rts
.fn_len_err:
    move.q  r8, r0
    rts

.fn_asc:
    ; ASC(string$) — ASCII code of first character
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
    ; VAL(string$) — parse string as number
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
    ; Set R17 to string data, call parse_number, restore R17
    push    r17
    move.q  r17, r8
    jsr     parse_number          ; R8 = FP32
    pop     r17
    rts
.fn_val_err:
    move.q  r8, r0
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
    move.l  r8, #0x40490FDB      ; PI as FP32
    rts
.fn_twopi:
    move.l  r8, #0x40C90FDB      ; 2*PI as FP32
    rts

; ============================================================================
; Chip STATUS functions — PSG/SID/POKEY/TED/AHX/SAP STATUS
; ============================================================================
; Reached when a chip token (TK_PSG_CMD, etc.) appears in expression context.
; R1 = chip token, R17 = text pointer (pointing at the token byte, not consumed)
.do_chip_status:
    add.q   r17, r17, #1         ; consume chip token
    push    r1                   ; save chip token
    ; Skip spaces (inline — exec_skip_spaces may not be available in expr-only tests)
.cs_skip_sp:
    load.b  r2, (r17)
    move.q  r3, #0x20
    bne     r2, r3, .cs_after_sp
    add.q   r17, r17, #1
    bra     .cs_skip_sp
.cs_after_sp:
    ; Check for "STATUS" — verify first char 'S', skip remaining alpha
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
    ; Unknown chip — return 0
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
    ; Not "STATUS" — pop saved token, return 0
    pop     r1
    move.q  r8, r0
    jsr     fp_float
    rts

.fn_pos:
    ; POS(x) — cursor column position (stub: returns 0)
    move.q  r8, r0
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
    dc.l    .fn_deek             ; 0xCD TK_DEEK
    dc.l    .fn_leek             ; 0xCE TK_LEEK

rnd_state:
    dc.l    0x12345678

; ============================================================================
; parse_number - Parse a numeric literal from the token stream
; ============================================================================
; Input:  R17 = pointer to start of number in token stream
; Output: R8  = FP32 result
;         R17 = advanced past the number
; Clobbers: R1-R7
;
; Handles: integer (123), float (3.14), leading dot (.5)
; Does NOT handle scientific notation yet.

parse_number:
    push    r14
    push    r15

    ; Accumulate integer part as raw int
    move.q  r1, r0               ; R1 = integer accumulator
    move.q  r14, r0              ; R14 = saw decimal point flag
    move.q  r15, r0              ; R15 = decimal divisor (FP32 bits of 10^n)
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
    mulu.l  r5, r5, #10
    add.l   r1, r5, r3
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

    ; Convert integer accumulator to FP32
    move.q  r8, r1
    and.l   r8, r8, #0xFFFFFFFF
    jsr     fp_float              ; R8 = FP32 of integer value

    ; Restore fractional digit count
    pop     r2

    ; If there were fractional digits, divide by 10^n using lookup table
    beqz    r2, .pn_end
    ; Clamp n to 8 (max table entries)
    move.q  r3, #8
    ble     r2, r3, .pn_div_lookup
    move.q  r2, r3
.pn_div_lookup:
    ; Table index: (n-1) * 4
    sub.q   r3, r2, #1
    lsl.q   r3, r3, #2
    la      r4, .fp_pow10_table
    add.q   r4, r4, r3
    load.l  r9, (r4)              ; R9 = 10^n (exact FP32)
    jsr     fp_div                ; R8 = value / 10^n

.pn_end:
    pop     r15
    pop     r14
    rts

; Power-of-10 lookup table (exact FP32 representations)
    align 4
.fp_pow10_table:
    dc.l    0x41200000            ; 10^1 = 10
    dc.l    0x42C80000            ; 10^2 = 100
    dc.l    0x447A0000            ; 10^3 = 1000
    dc.l    0x461C4000            ; 10^4 = 10000
    dc.l    0x47C35000            ; 10^5 = 100000
    dc.l    0x49742400            ; 10^6 = 1000000
    dc.l    0x4B189680            ; 10^7 = 10000000
    dc.l    0x4CBEBC20            ; 10^8 = 100000000

; ============================================================================
; fp32_to_int_pair - Convert R22 (FP32) and R8 (FP32) to integers
; ============================================================================
; Output: R1 = int32 from R22, R2 = int32 from R8
; Clobbers: R3-R7

fp32_to_int_pair:
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

; ============================================================================
; fp_fix - Convert FP32 to signed int32
; ============================================================================
; Input:  R8 = FP32 value (must be an integer value, e.g. after fp_int)
; Output: R8 = signed 32-bit integer
; Clobbers: R1-R4
;
; Strategy: extract sign, exponent, mantissa from IEEE 754 bits,
; reconstruct integer value.

fp_fix:
    fmovi   f0, r8
    fcvtfi  r8, f0            ; float -> int32 (truncates toward zero, saturates)
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

; ============================================================================
; EOF
; ============================================================================
