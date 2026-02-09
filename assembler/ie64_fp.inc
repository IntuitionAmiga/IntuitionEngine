; ie64_fp.inc - IEEE 754 Single-Precision Floating-Point Library for IE64
;
; Calling convention:
;   Operands in R8, R9 (as raw IEEE 754 FP32 bit patterns in lower 32 bits)
;   Result in R8
;   Clobbers R1-R7 only (caller-saved scratch)
;
; IEEE 754 FP32 layout (32 bits):
;   Bit 31:    Sign (0=positive, 1=negative)
;   Bits 30-23: Exponent (biased by 127)
;   Bits 22-0:  Mantissa (implicit leading 1 for normalised)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; Constants
FP_SIGN_BIT       equ 0x80000000
FP_EXP_MASK       equ 0x7F800000
FP_MANT_MASK      equ 0x007FFFFF
FP_IMPLICIT_BIT   equ 0x00800000
FP_EXP_BIAS       equ 127
FP_EXP_SHIFT      equ 23
FP_INF            equ 0x7F800000
FP_NEG_INF        equ 0xFF800000
FP_ZERO           equ 0x00000000
FP_NEG_ZERO       equ 0x80000000
FP_ONE            equ 0x3F800000
FP_TWO            equ 0x40000000
FP_HALF           equ 0x3F000000
FP_PI             equ 0x40490FDB
FP_HALF_PI        equ 0x3FC90FDB
FP_TWO_PI         equ 0x40C90FDB
FP_INV_TWO_PI     equ 0x3E22F983
FP_LN2            equ 0x3F317218
FP_INV_LN2        equ 0x3FB8AA3B
FP_E              equ 0x402DF854

; Precomputed reciprocals for replacing fp_div with fp_mul
FP_INV_2          equ 0x3F000000    ; 1/2 = 0.5
FP_INV_3          equ 0x3EAAAAAB    ; 1/3
FP_INV_4          equ 0x3E800000    ; 1/4 = 0.25
FP_INV_5          equ 0x3E4CCCCD    ; 1/5
FP_INV_6          equ 0x3E2AAAAB    ; 1/6
FP_INV_7          equ 0x3E124925    ; 1/7
FP_INV_9          equ 0x3DE38E39    ; 1/9
FP_INV_11         equ 0x3DBA2E8C    ; 1/11
FP_INV_20         equ 0x3D4CCCCD    ; 1/20
FP_INV_42         equ 0x3CC2F8BC    ; 1/42
; Combined ratio constants for fp_log
FP_FOUR_FIFTHS    equ 0x3F4CCCCD    ; 4/5
FP_THREE_QUARTERS equ 0x3F400000    ; 3/4
FP_TWO_THIRDS     equ 0x3F2AAAAB    ; 2/3

; ============================================================================
; fp_neg: R8 = -R8 (flip sign bit)
; ============================================================================
fp_neg:
    eor.l   r8, r8, #FP_SIGN_BIT
    rts

; ============================================================================
; fp_abs: R8 = |R8| (clear sign bit)
; ============================================================================
fp_abs:
    and.l   r8, r8, #0x7FFFFFFF
    rts

; ============================================================================
; fp_cmp: Compare R8 vs R9, result in R8: -1 (less), 0 (equal), 1 (greater)
; ============================================================================
fp_cmp:
    ; Handle both-zero (positive and negative zero are equal)
    and.l   r1, r8, #0x7FFFFFFF
    and.l   r2, r9, #0x7FFFFFFF
    or.l    r3, r1, r2
    beqz    r3, .fp_cmp_equal

    ; Same bit pattern = equal
    beq     r8, r9, .fp_cmp_equal

    ; Extract signs
    lsr.l   r3, r8, #31             ; sign of R8
    lsr.l   r4, r9, #31             ; sign of R9

    ; If signs differ: negative < positive
    bne     r3, r4, .fp_cmp_diff_sign

    ; Same sign: compare magnitudes
    ; For positive: larger magnitude = greater
    ; For negative: larger magnitude = less
    bhi     r1, r2, .fp_cmp_a_bigger_mag
    ; a magnitude <= b magnitude (and not equal, handled above)
    ; If positive: a < b. If negative: a > b.
    beqz    r3, .fp_cmp_less
    bra     .fp_cmp_greater

.fp_cmp_a_bigger_mag:
    beqz    r3, .fp_cmp_greater
    bra     .fp_cmp_less

.fp_cmp_diff_sign:
    ; r3=sign(a), r4=sign(b). If a is negative, a < b.
    bnez    r3, .fp_cmp_less
    bra     .fp_cmp_greater

.fp_cmp_equal:
    move.l  r8, r0                  ; R8 = 0
    rts

.fp_cmp_less:
    move.l  r8, #0xFFFFFFFF         ; R8 = -1
    rts

.fp_cmp_greater:
    move.l  r8, #1                  ; R8 = 1
    rts

; ============================================================================
; fp_add: R8 = R8 + R9
; ============================================================================
fp_add:
    push    r10
    push    r11
    push    r12
    push    r13

    ; Check for zero operands
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fp_add_ret_b       ; a=0: return b
    and.l   r1, r9, #0x7FFFFFFF
    beqz    r1, .fp_add_ret_a       ; b=0: return a

    ; Extract components of A (R8)
    lsr.l   r10, r8, #31            ; sign_a
    lsr.l   r1, r8, #FP_EXP_SHIFT
    and.l   r11, r1, #0xFF          ; exp_a
    and.l   r12, r8, #FP_MANT_MASK
    or.l    r12, r12, #FP_IMPLICIT_BIT  ; mant_a with implicit 1

    ; Extract components of B (R9)
    lsr.l   r13, r9, #31            ; sign_b
    lsr.l   r1, r9, #FP_EXP_SHIFT
    and.l   r2, r1, #0xFF          ; exp_b
    and.l   r3, r9, #FP_MANT_MASK
    or.l    r3, r3, #FP_IMPLICIT_BIT  ; mant_b with implicit 1

    ; Align mantissas by comparing exponents directly (avoid signed subtraction)
    ; r11 = exp_a, r2 = exp_b
    beq     r11, r2, .fp_add_do_add ; Same exponent: no shift needed

    bhi     r11, r2, .fp_add_a_bigger_exp

    ; exp_b > exp_a: shift mant_a right by (exp_b - exp_a)
    sub.l   r4, r2, r11             ; exp_b - exp_a (positive, fits in .l)
    move.l  r5, #24
    bhi     r4, r5, .fp_add_ret_b   ; b dominates (diff > 24)
    lsr.l   r12, r12, r4            ; shift mant_a right
    move.l  r11, r2                 ; use exp_b as result exponent
    bra     .fp_add_do_add

.fp_add_a_bigger_exp:
    ; exp_a > exp_b: shift mant_b right by (exp_a - exp_b)
    sub.l   r4, r11, r2             ; exp_a - exp_b (positive)
    move.l  r5, #24
    bhi     r4, r5, .fp_add_ret_a   ; a dominates
    lsr.l   r3, r3, r4              ; shift mant_b right
    ; result exponent stays as exp_a (r11)
    bra     .fp_add_do_add

.fp_add_do_add:
    ; r10=sign_a, r13=sign_b, r12=mant_a (aligned), r3=mant_b (aligned), r11=result exp
    ; Same sign? Add mantissas. Different sign? Subtract.
    beq     r10, r13, .fp_add_same_sign

    ; Different signs: subtract smaller from larger
    bhi     r12, r3, .fp_add_sub_ab
    ; mant_b >= mant_a
    sub.l   r12, r3, r12            ; result = mant_b - mant_a
    move.l  r10, r13                ; result sign = sign of b
    bra     .fp_add_normalise

.fp_add_sub_ab:
    sub.l   r12, r12, r3            ; result = mant_a - mant_b
    ; sign stays as sign_a (r10)
    bra     .fp_add_normalise

.fp_add_same_sign:
    add.l   r12, r12, r3
    ; Check for mantissa overflow (bit 24 set)
    lsr.l   r1, r12, #24
    beqz    r1, .fp_add_normalise
    ; Overflow: shift right, increment exponent
    lsr.l   r12, r12, #1
    add.l   r11, r11, #1
    bra     .fp_add_pack

.fp_add_normalise:
    ; Normalise: shift left until bit 23 is set (or mantissa is zero)
    beqz    r12, .fp_add_zero_result
    ; CLZ-based O(1) normalisation
    clz.l   r1, r12             ; count leading zeros of mantissa
    sub.q   r1, r1, #8          ; bit 23 = 8 leading zeros in 32-bit (.q for signed result)
    beqz    r1, .fp_add_pack    ; already normalised
    blt     r1, r0, .fp_add_pack ; negative shift means already >=bit23 (shouldn't happen in sub path)
    ; Check for exponent underflow: if shift > exponent, denormalise
    bge     r1, r11, .fp_add_denorm
    lsl.l   r12, r12, r1       ; normalise mantissa
    sub.l   r11, r11, r1       ; adjust exponent
    bra     .fp_add_pack

.fp_add_denorm:
    ; Denormalised result (exponent = 0)
    move.l  r11, r0
    bra     .fp_add_pack

.fp_add_pack:
    ; Pack: sign(r10) | exp(r11) << 23 | mant(r12) & MANT_MASK
    and.l   r12, r12, #FP_MANT_MASK
    lsl.l   r11, r11, #FP_EXP_SHIFT
    lsl.l   r10, r10, #31
    or.l    r8, r10, r11
    or.l    r8, r8, r12
    bra     .fp_add_done

.fp_add_zero_result:
    move.l  r8, r0
    bra     .fp_add_done

.fp_add_ret_a:
    bra     .fp_add_done

.fp_add_ret_b:
    move.l  r8, r9
    bra     .fp_add_done

.fp_add_done:
    pop     r13
    pop     r12
    pop     r11
    pop     r10
    rts

; ============================================================================
; fp_sub: R8 = R8 - R9
; ============================================================================
fp_sub:
    ; Negate R9 and add
    eor.l   r9, r9, #FP_SIGN_BIT
    bra     fp_add

; ============================================================================
; fp_mul: R8 = R8 * R9
; ============================================================================
fp_mul:
    push    r10
    push    r11
    push    r12
    push    r13

    ; Result sign = XOR of input signs
    eor.l   r10, r8, r9
    lsr.l   r10, r10, #31           ; result sign bit

    ; Check for zero
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fp_mul_zero
    and.l   r1, r9, #0x7FFFFFFF
    beqz    r1, .fp_mul_zero

    ; Check for infinity
    and.l   r1, r8, #FP_EXP_MASK
    move.l  r2, #FP_EXP_MASK
    beq     r1, r2, .fp_mul_inf
    and.l   r1, r9, #FP_EXP_MASK
    beq     r1, r2, .fp_mul_inf

    ; Extract exponents
    lsr.l   r11, r8, #FP_EXP_SHIFT
    and.l   r11, r11, #0xFF         ; exp_a
    lsr.l   r1, r9, #FP_EXP_SHIFT
    and.l   r1, r1, #0xFF           ; exp_b

    ; Result exponent = exp_a + exp_b - bias
    add.l   r11, r11, r1
    sub.l   r11, r11, #FP_EXP_BIAS

    ; Extract mantissas with implicit bit
    and.l   r12, r8, #FP_MANT_MASK
    or.l    r12, r12, #FP_IMPLICIT_BIT  ; mant_a (24 bits)
    and.l   r13, r9, #FP_MANT_MASK
    or.l    r13, r13, #FP_IMPLICIT_BIT  ; mant_b (24 bits)

    ; Multiply: (mant_a >> 12) * (mant_b >> 11) -> result has implicit 1 at bit 23
    ; 12-bit x 13-bit = max 25 bits, fits in 32-bit register
    lsr.l   r1, r12, #12            ; 12-bit value with high bit at bit 11
    lsr.l   r2, r13, #11            ; 13-bit value with high bit at bit 12
    mulu.l  r12, r1, r2             ; product: high bit at bit 23 (11+12)

    ; Normalise: if bit 24 is set (overflow), shift right
    lsr.l   r1, r12, #24
    beqz    r1, .fp_mul_check23
    lsr.l   r12, r12, #1
    add.l   r11, r11, #1
    bra     .fp_mul_pack_chk
.fp_mul_check23:
    ; If bit 23 is not set, shift left
    and.l   r1, r12, #FP_IMPLICIT_BIT
    bnez    r1, .fp_mul_pack_chk
    lsl.l   r12, r12, #1
    sub.l   r11, r11, #1
.fp_mul_pack_chk:
    ; Check exponent overflow/underflow
    move.l  r1, #0xFF
    bhi     r11, r1, .fp_mul_inf    ; overflow
    move.l  r1, #1
    blt     r11, r1, .fp_mul_zero   ; underflow

    ; Pack result
    and.l   r12, r12, #FP_MANT_MASK
    lsl.l   r11, r11, #FP_EXP_SHIFT
    lsl.l   r10, r10, #31
    or.l    r8, r10, r11
    or.l    r8, r8, r12
    bra     .fp_mul_done

.fp_mul_zero:
    lsl.l   r10, r10, #31
    move.l  r8, r10                 ; signed zero
    bra     .fp_mul_done

.fp_mul_inf:
    lsl.l   r10, r10, #31
    move.l  r1, #FP_INF
    or.l    r8, r10, r1
    bra     .fp_mul_done

.fp_mul_done:
    pop     r13
    pop     r12
    pop     r11
    pop     r10
    rts

; ============================================================================
; fp_div: R8 = R8 / R9
; ============================================================================
fp_div:
    push    r10
    push    r11
    push    r12
    push    r13

    ; Result sign
    eor.l   r10, r8, r9
    lsr.l   r10, r10, #31

    ; Check for zero divisor
    and.l   r1, r9, #0x7FFFFFFF
    beqz    r1, .fp_div_by_zero

    ; Check for zero dividend
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fp_div_zero

    ; Extract exponents
    lsr.l   r11, r8, #FP_EXP_SHIFT
    and.l   r11, r11, #0xFF
    lsr.l   r1, r9, #FP_EXP_SHIFT
    and.l   r1, r1, #0xFF

    ; Result exponent = exp_a - exp_b + bias
    sub.l   r11, r11, r1
    add.l   r11, r11, #FP_EXP_BIAS

    ; Extract mantissas
    and.l   r12, r8, #FP_MANT_MASK
    or.l    r12, r12, #FP_IMPLICIT_BIT  ; dividend mantissa
    and.l   r13, r9, #FP_MANT_MASK
    or.l    r13, r13, #FP_IMPLICIT_BIT  ; divisor mantissa

    ; Division: shift dividend left 8, divide, shift quotient left 15
    lsl.l   r12, r12, #8
    divu.l  r12, r12, r13
    lsl.l   r12, r12, #15

    ; Normalise: ensure bit 23 is set
.fp_div_norm:
    and.l   r1, r12, #FP_IMPLICIT_BIT
    bnez    r1, .fp_div_pack
    beqz    r12, .fp_div_zero
    lsl.l   r12, r12, #1
    sub.l   r11, r11, #1
    bra     .fp_div_norm

.fp_div_pack:
    ; Check overflow/underflow
    move.l  r1, #0xFF
    bhi     r11, r1, .fp_div_inf
    move.l  r1, #1
    blt     r11, r1, .fp_div_zero

    and.l   r12, r12, #FP_MANT_MASK
    lsl.l   r11, r11, #FP_EXP_SHIFT
    lsl.l   r10, r10, #31
    or.l    r8, r10, r11
    or.l    r8, r8, r12
    bra     .fp_div_done

.fp_div_by_zero:
    lsl.l   r10, r10, #31
    move.l  r1, #FP_INF
    or.l    r8, r10, r1
    bra     .fp_div_done

.fp_div_zero:
    lsl.l   r10, r10, #31
    move.l  r8, r10
    bra     .fp_div_done

.fp_div_inf:
    lsl.l   r10, r10, #31
    move.l  r1, #FP_INF
    or.l    r8, r10, r1
    bra     .fp_div_done

.fp_div_done:
    pop     r13
    pop     r12
    pop     r11
    pop     r10
    rts

; ============================================================================
; fp_int: R8 = INT(R8) -- truncate towards zero
; ============================================================================
fp_int:
    ; Extract exponent
    lsr.l   r1, r8, #FP_EXP_SHIFT
    and.l   r1, r1, #0xFF
    sub.l   r1, r1, #FP_EXP_BIAS   ; unbiased exponent

    ; If exponent < 0, |x| < 1, result is 0 (or -0)
    lsr.l   r2, r1, #31             ; check if negative (sign bit)
    bnez    r2, .fp_int_zero

    ; If exponent >= 23, all mantissa bits are integer bits, no truncation needed
    move.l  r3, #23
    bge     r1, r3, .fp_int_done

    ; Mask off fractional bits: shift = 23 - exponent
    sub.l   r2, r3, r1              ; shift amount
    ; Create mask: ~((1 << shift) - 1)
    move.l  r3, #1
    lsl.l   r3, r3, r2              ; 1 << shift
    sub.l   r3, r3, #1              ; (1 << shift) - 1
    not.l   r3, r3                  ; mask for integer bits
    and.l   r8, r8, r3
.fp_int_done:
    rts

.fp_int_zero:
    and.l   r8, r8, #FP_SIGN_BIT   ; preserve sign, zero the rest
    rts

; ============================================================================
; fp_float: R8 = int32 in R8 -> IEEE 754 float in R8
; ============================================================================
fp_float:
    ; Handle zero
    beqz    r8, .fp_float_done

    push    r10

    ; Save sign and work with absolute value
    lsr.l   r10, r8, #31            ; sign
    ; If negative, negate
    bnez    r10, .fp_float_negate
    bra     .fp_float_have_abs
.fp_float_negate:
    neg.l   r8, r8
.fp_float_have_abs:
    ; CLZ-based O(1) normalisation
    clz.l   r1, r8              ; count leading zeros
    ; Target: bit 23 set = 8 leading zeros in 32-bit
    ; shift_left = clz - 8 (positive = shift left, negative = shift right)
    sub.q   r1, r1, #8          ; .q for signed result
    beqz    r1, .fp_float_pack  ; already normalised (bit 23 set)
    blt     r1, r0, .fp_float_shr
    ; Shift left (mantissa too small)
    move.q  r2, #31
    bgt     r1, r2, .fp_float_zero_result
    lsl.l   r8, r8, r1
    bra     .fp_float_pack
.fp_float_shr:
    ; Shift right (mantissa too large) — negate shift count
    neg.q   r1, r1
    lsr.l   r8, r8, r1
    neg.q   r1, r1              ; restore negative for exponent calc

.fp_float_pack:
    ; Exponent = 127 + 23 - shift_count = 150 - shift_count
    and.l   r3, r8, #FP_MANT_MASK  ; save mantissa bits BEFORE overwriting r8
    move.l  r2, #150
    sub.l   r2, r2, r1
    lsl.l   r2, r2, #FP_EXP_SHIFT
    lsl.l   r10, r10, #31
    or.l    r8, r10, r2             ; sign | exponent
    or.l    r8, r8, r3              ; | mantissa

    pop     r10
    rts

.fp_float_zero_result:
    move.l  r8, r0
    pop     r10
    rts

.fp_float_done:
    rts

; ============================================================================
; fp_sqr: R8 = sqrt(R8) -- Newton-Raphson, 4 iterations
; ============================================================================
fp_sqr:
    push    r14
    push    r15
    push    r9

    ; Handle zero
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fp_sqr_done        ; sqrt(0) = 0

    ; Handle negative: return 0
    lsr.l   r1, r8, #31
    bnez    r1, .fp_sqr_neg

    ; Initial estimate: halve the exponent
    lsr.l   r14, r8, #1
    add.l   r14, r14, #0x1FC00000   ; Magic constant for sqrt estimate

    ; Save input
    move.l  r15, r8

    ; Newton-Raphson: x_{n+1} = (x_n + S/x_n) / 2
    ; 4 iterations for good FP32 precision
    move.l  r1, #4
.fp_sqr_iter:
    push    r1                      ; save iteration counter

    ; R8 = S (input), R9 = x_n (current estimate)
    move.l  r8, r15                 ; S
    move.l  r9, r14                 ; x_n
    jsr     fp_div                  ; R8 = S / x_n
    move.l  r9, r14                 ; x_n
    jsr     fp_add                  ; R8 = S/x_n + x_n
    move.l  r9, #FP_HALF            ; 0.5
    jsr     fp_mul                  ; R8 = (S/x_n + x_n) * 0.5
    move.l  r14, r8                 ; update estimate

    pop     r1
    sub.l   r1, r1, #1
    bnez    r1, .fp_sqr_iter

    move.l  r8, r14
    bra     .fp_sqr_done

.fp_sqr_neg:
    move.l  r8, r0              ; return 0.0

.fp_sqr_done:
    pop     r9
    pop     r15
    pop     r14
    rts

; ============================================================================
; fp_sin: R8 = sin(R8)
; Range reduction to [-pi/2, pi/2], then Horner-form Taylor series (7th order)
; sin(x) = x * (1 - x^2/6 * (1 - x^2/20 * (1 - x^2/42)))
; ============================================================================
fp_sin:
    push    r14
    push    r15
    push    r10
    push    r11
    push    r9

    ; Save original sign
    lsr.l   r10, r8, #31            ; r10 = original sign (0 or 1)

    ; Work with |x|
    and.l   r8, r8, #0x7FFFFFFF

    ; --- Range reduction: reduce to [0, 2*pi) ---
    move.l  r15, r8                 ; r15 = |x|
    move.l  r9, #FP_INV_TWO_PI
    jsr     fp_mul                  ; |x| * (1/2pi) = |x| / 2pi
    jsr     fp_int                  ; floor(|x| / 2pi)
    move.l  r9, #FP_TWO_PI
    jsr     fp_mul                  ; floor * 2pi
    move.l  r9, r8
    move.l  r8, r15
    jsr     fp_sub                  ; |x| mod 2pi, in [0, 2pi)
    move.l  r15, r8                 ; r15 = reduced angle in [0, 2pi)

    ; --- Map to [-pi, pi]: if > pi, subtract 2*pi ---
    move.l  r14, r15                ; save reduced
    move.l  r9, #FP_PI
    jsr     fp_cmp
    move.l  r1, r8                  ; cmp result
    move.l  r8, r14                 ; restore reduced
    move.l  r2, #1
    bne     r1, r2, .fp_sin_half_check
    ; reduced > pi: subtract 2*pi
    move.l  r9, #FP_TWO_PI
    jsr     fp_sub

.fp_sin_half_check:
    ; --- Map to [-pi/2, pi/2]: if |x| > pi/2, use sin(x) = sin(pi-x) ---
    move.l  r15, r8                 ; r15 = angle in [-pi, pi]
    and.l   r1, r8, #0x7FFFFFFF    ; |angle|
    move.l  r14, r1                 ; save |angle|
    move.l  r8, r1
    move.l  r9, #FP_HALF_PI
    jsr     fp_cmp
    move.l  r1, r8                  ; cmp result
    move.l  r8, r15                 ; restore angle
    move.l  r2, #1
    bne     r1, r2, .fp_sin_taylor  ; |angle| <= pi/2, go to Taylor
    ; |angle| > pi/2: use identity
    lsr.l   r1, r15, #31            ; sign of angle
    beqz    r1, .fp_sin_pos_large
    ; Negative: sin(x) = -sin(-pi - x) => sin(pi + x) negated
    move.l  r9, #FP_PI
    jsr     fp_add                  ; pi + x (x is negative)
    eor.l   r8, r8, #FP_SIGN_BIT   ; negate
    bra     .fp_sin_taylor
.fp_sin_pos_large:
    ; Positive: sin(x) = sin(pi - x)
    move.l  r9, r8
    move.l  r8, #FP_PI
    jsr     fp_sub                  ; pi - x

.fp_sin_taylor:
    ; r8 = angle in [-pi/2, pi/2]
    ; sin(x) = x * (1 - x^2/6 * (1 - x^2/20 * (1 - x^2/42)))
    move.l  r15, r8                 ; r15 = x
    move.l  r9, r8
    jsr     fp_mul                  ; x^2
    move.l  r14, r8                 ; r14 = x^2

    ; Innermost: 1 - x^2/42
    move.l  r9, #FP_INV_42          ; 1/42
    jsr     fp_mul                  ; x^2 * (1/42) = x^2/42
    eor.l   r8, r8, #FP_SIGN_BIT   ; -x^2/42
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - x^2/42

    ; Next: * x^2/20, then 1 - that
    move.l  r9, r14                 ; x^2
    jsr     fp_mul
    move.l  r9, #FP_INV_20          ; 1/20
    jsr     fp_mul                  ; * (1/20) = /20
    eor.l   r8, r8, #FP_SIGN_BIT
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - x^2/20*(...)

    ; Next: * x^2/6, then 1 - that
    move.l  r9, r14                 ; x^2
    jsr     fp_mul
    move.l  r9, #FP_INV_6           ; 1/6
    jsr     fp_mul                  ; * (1/6) = /6
    eor.l   r8, r8, #FP_SIGN_BIT
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - x^2/6*(...)

    ; Multiply by x
    move.l  r9, r15
    jsr     fp_mul                  ; sin(x)

    ; Apply original sign
    bnez    r10, .fp_sin_neg
    bra     .fp_sin_done
.fp_sin_neg:
    eor.l   r8, r8, #FP_SIGN_BIT
.fp_sin_done:
    pop     r9
    pop     r11
    pop     r10
    pop     r15
    pop     r14
    rts

; ============================================================================
; fp_cos: R8 = cos(R8) = sin(R8 + pi/2)
; ============================================================================
fp_cos:
    push    r9
    move.l  r9, #FP_HALF_PI
    jsr     fp_add
    jsr     fp_sin
    pop     r9
    rts

; ============================================================================
; fp_tan: R8 = tan(R8) = sin(R8) / cos(R8)
; ============================================================================
fp_tan:
    push    r14
    push    r15
    push    r9

    move.l  r14, r8                 ; save x
    jsr     fp_sin
    move.l  r15, r8                 ; r15 = sin(x)
    move.l  r8, r14
    jsr     fp_cos                  ; r8 = cos(x)
    move.l  r9, r8                  ; r9 = cos(x)
    move.l  r8, r15                 ; r8 = sin(x)
    jsr     fp_div                  ; sin/cos

    pop     r9
    pop     r15
    pop     r14
    rts

; ============================================================================
; fp_atn: R8 = atan(R8)
; Three ranges:
;   |x| > 1:      atan(x) = pi/2 - atan(1/x)
;   |x| > 0.4142: atan(x) = pi/4 + atan((x-1)/(x+1))  [maps x=1 -> 0]
;   |x| <= 0.4142: 11th-order Taylor via Horner
; ============================================================================
FP_TAN_PI_8      equ 0x3ED413CD    ; tan(pi/8) = 0.41421356
FP_QUARTER_PI    equ 0x3F490FDB    ; pi/4

fp_atn:
    push    r14
    push    r15
    push    r10
    push    r11
    push    r9

    ; Save input sign
    lsr.l   r10, r8, #31            ; r10 = sign (0 or 1)
    and.l   r8, r8, #0x7FFFFFFF    ; |x|

    ; r11 = offset mode: 0=none, 1=pi/4 offset, 2=pi/2 offset
    move.l  r11, r0

    ; Check if |x| > 1: use atan(x) = pi/2 - atan(1/x)
    move.l  r15, r8                 ; r15 = |x|
    move.l  r9, #FP_ONE
    jsr     fp_cmp
    move.l  r1, r8
    move.l  r8, r15                 ; restore |x|
    move.l  r2, #1
    bne     r1, r2, .fp_atn_check_mid
    ; |x| > 1: compute atan(1/|x|)
    move.l  r11, #2                 ; pi/2 offset
    move.l  r9, r8
    move.l  r8, #FP_ONE
    jsr     fp_div                  ; 1/|x|
    bra     .fp_atn_poly

.fp_atn_check_mid:
    ; Check if |x| > tan(pi/8) = 0.4142: use pi/4 + atan((x-1)/(x+1))
    move.l  r15, r8
    move.l  r9, #FP_TAN_PI_8
    jsr     fp_cmp
    move.l  r1, r8
    move.l  r8, r15
    move.l  r2, #1
    bne     r1, r2, .fp_atn_poly   ; |x| <= 0.4142
    ; |x| > 0.4142: transform t = (x-1)/(x+1)
    move.l  r11, #1                 ; pi/4 offset
    move.l  r15, r8                 ; save x
    ; numerator: x - 1
    move.l  r9, #FP_ONE
    jsr     fp_sub                  ; x - 1
    push    r8                      ; save numerator
    ; denominator: x + 1
    move.l  r8, r15
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; x + 1
    move.l  r9, r8                  ; denominator in r9
    pop     r8                      ; numerator in r8
    jsr     fp_div                  ; (x-1)/(x+1)

.fp_atn_poly:
    ; r8 = t, |t| <= 0.4142, atan(t) via 11th-order Horner
    ; atan(t) = t*(1 + t^2*(-1/3 + t^2*(1/5 + t^2*(-1/7 + t^2*(1/9 + t^2*(-1/11))))))
    move.l  r15, r8                 ; r15 = t
    move.l  r9, r8
    jsr     fp_mul                  ; t^2
    move.l  r14, r8                 ; r14 = t^2

    ; Innermost: -1/11 (use precomputed constant directly)
    move.l  r8, #FP_INV_11
    eor.l   r8, r8, #FP_SIGN_BIT   ; -1/11

    ; * t^2 + 1/9
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_INV_9           ; 1/9 (precomputed)
    jsr     fp_add                  ; 1/9 - t^2/11

    ; * t^2 + (-1/7)
    move.l  r9, r14
    jsr     fp_mul
    push    r8
    move.l  r8, #FP_INV_7           ; 1/7 (precomputed)
    eor.l   r8, r8, #FP_SIGN_BIT   ; -1/7
    pop     r9
    jsr     fp_add                  ; -1/7 + t^2*(1/9 - ...)

    ; * t^2 + 1/5
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_INV_5           ; 1/5 (precomputed)
    jsr     fp_add                  ; 1/5 + t^2*(...)

    ; * t^2 + (-1/3)
    move.l  r9, r14
    jsr     fp_mul
    push    r8
    move.l  r8, #FP_INV_3           ; 1/3 (precomputed)
    eor.l   r8, r8, #FP_SIGN_BIT   ; -1/3
    pop     r9
    jsr     fp_add                  ; -1/3 + t^2*(...)

    ; * t^2 + 1
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 + t^2*(...)

    ; * t
    move.l  r9, r15
    jsr     fp_mul                  ; atan(t)

    ; Apply offset based on r11
    move.l  r1, #2
    beq     r11, r1, .fp_atn_pi2_offset
    move.l  r1, #1
    beq     r11, r1, .fp_atn_pi4_offset
    bra     .fp_atn_apply_sign

.fp_atn_pi2_offset:
    ; atan(x) = pi/2 - atan(1/x)
    eor.l   r8, r8, #FP_SIGN_BIT   ; -atan(1/x)
    move.l  r9, #FP_HALF_PI
    jsr     fp_add                  ; pi/2 - atan(1/x)
    bra     .fp_atn_apply_sign

.fp_atn_pi4_offset:
    ; atan(x) = pi/4 + atan((x-1)/(x+1))
    move.l  r9, #FP_QUARTER_PI
    jsr     fp_add                  ; pi/4 + atan(t)

.fp_atn_apply_sign:
    ; Apply original sign
    bnez    r10, .fp_atn_make_neg
    bra     .fp_atn_done
.fp_atn_make_neg:
    eor.l   r8, r8, #FP_SIGN_BIT
.fp_atn_done:
    pop     r9
    pop     r11
    pop     r10
    pop     r15
    pop     r14
    rts

; ============================================================================
; fp_log: R8 = ln(R8) -- Natural logarithm
; Uses: ln(x) = ln(m * 2^e) = ln(m) + e*ln(2)
; where 1 <= m < 2 (the normalised mantissa)
; ln(1+t) via 5-term Horner: t*(1 - t/2*(1 - 2t/3*(1 - 3t/4*(1 - 4t/5))))
; ============================================================================
fp_log:
    push    r14
    push    r15
    push    r10
    push    r9

    ; Extract biased exponent
    lsr.l   r1, r8, #FP_EXP_SHIFT
    and.l   r1, r1, #0xFF
    ; Compute unbiased exponent safely (avoid .l signed subtraction issue)
    move.l  r10, r0                 ; r10 = exponent sign (0 = positive)
    move.l  r2, #FP_EXP_BIAS
    bge     r1, r2, .fp_log_exp_pos
    ; biased < 127: negative exponent
    sub.l   r14, r2, r1             ; 127 - biased = |e|
    move.l  r10, #1                 ; negative exponent
    bra     .fp_log_have_exp
.fp_log_exp_pos:
    sub.l   r14, r1, r2             ; biased - 127 = e (positive)
.fp_log_have_exp:
    ; r14 = |exponent|, r10 = sign of exponent (0=pos, 1=neg)

    ; Construct m = 1.mantissa (set exponent to 127)
    and.l   r15, r8, #FP_MANT_MASK
    move.l  r1, #FP_EXP_BIAS
    lsl.l   r1, r1, #FP_EXP_SHIFT  ; 127 << 23
    or.l    r15, r15, r1            ; m as float in [1.0, 2.0)

    ; t = m - 1 (in [0, 1))
    move.l  r8, r15
    move.l  r9, #FP_ONE
    jsr     fp_sub                  ; t = m - 1

    ; If t is zero (m=1.0), ln(m)=0, skip to e*ln2
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fp_log_t_zero

    move.l  r15, r8                 ; r15 = t

    ; ln(1+t) = t*(1 - t/2*(1 - 2t/3*(1 - 3t/4*(1 - 4t/5))))
    ; Start from innermost: 1 - 4t/5

    ; 4t/5 → t * (4/5)
    move.l  r8, r15                 ; t
    move.l  r9, #FP_FOUR_FIFTHS     ; 4/5 precomputed
    jsr     fp_mul                  ; t * (4/5)
    eor.l   r8, r8, #FP_SIGN_BIT   ; -4t/5
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - 4t/5

    ; * 3t/4 → * t * (3/4), then 1 - ...
    move.l  r9, r15                 ; t
    jsr     fp_mul
    move.l  r9, #FP_THREE_QUARTERS  ; 3/4 precomputed
    jsr     fp_mul                  ; * (3/4)
    eor.l   r8, r8, #FP_SIGN_BIT
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - 3t/4*(...)

    ; * 2t/3 → * t * (2/3), then 1 - ...
    move.l  r9, r15                 ; t
    jsr     fp_mul
    move.l  r9, #FP_TWO_THIRDS      ; 2/3 precomputed
    jsr     fp_mul                  ; * (2/3)
    eor.l   r8, r8, #FP_SIGN_BIT
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - 2t/3*(...)

    ; * t/2 → * t * (1/2), then 1 - ...
    move.l  r9, r15                 ; t
    jsr     fp_mul
    move.l  r9, #FP_INV_2           ; 1/2 precomputed
    jsr     fp_mul                  ; * (1/2)
    eor.l   r8, r8, #FP_SIGN_BIT
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 - t/2*(...)

    ; * t to get ln(1+t)
    move.l  r9, r15
    jsr     fp_mul                  ; ln(m)
    move.l  r15, r8                 ; r15 = ln(m)
    bra     .fp_log_add_exp

.fp_log_t_zero:
    move.l  r15, r0                 ; ln(m) = 0

.fp_log_add_exp:
    ; Result = ln(m) + e * ln(2)
    ; Convert |exponent| to float
    move.l  r8, r14                 ; |e|
    jsr     fp_float
    ; Apply exponent sign
    bnez    r10, .fp_log_neg_exp
    bra     .fp_log_mul_ln2
.fp_log_neg_exp:
    eor.l   r8, r8, #FP_SIGN_BIT
.fp_log_mul_ln2:
    move.l  r9, #FP_LN2
    jsr     fp_mul                  ; e * ln(2) (signed)
    move.l  r9, r15                 ; ln(m)
    jsr     fp_add                  ; ln(m) + e*ln(2)

    pop     r9
    pop     r10
    pop     r15
    pop     r14
    rts

; ============================================================================
; fp_exp: R8 = e^R8
; Uses: e^x = 2^(x / ln(2)) = 2^n * 2^f where n=int, f=frac
; 2^f via Horner Taylor: e^u where u = f*ln(2)
; e^u = 1 + u*(1 + u/2*(1 + u/3*(1 + u/4*(1 + u/5))))
; ============================================================================
fp_exp:
    push    r14
    push    r15
    push    r10
    push    r9

    ; y = x / ln(2) = x * (1/ln2)
    move.l  r9, #FP_INV_LN2
    jsr     fp_mul                  ; y = x / ln2
    move.l  r14, r8                 ; r14 = y

    ; n = int(y) as float
    jsr     fp_int
    move.l  r15, r8                 ; r15 = n as float

    ; f = y - n (fractional part)
    move.l  r9, r8                  ; n
    move.l  r8, r14                 ; y
    jsr     fp_sub                  ; f = y - n
    move.l  r14, r8                 ; r14 = f

    ; Convert n to integer for exponent manipulation
    ; Handle n_float = 0.0 case
    and.l   r1, r15, #0x7FFFFFFF
    beqz    r1, .fp_exp_n_is_zero
    ; Extract integer from n_float
    lsr.l   r1, r15, #FP_EXP_SHIFT
    and.l   r1, r1, #0xFF
    move.l  r2, #FP_EXP_BIAS
    blt     r1, r2, .fp_exp_n_is_zero  ; |n| < 1 => 0
    sub.l   r1, r1, r2              ; unbiased exp
    and.l   r2, r15, #FP_MANT_MASK
    or.l    r2, r2, #FP_IMPLICIT_BIT
    ; Shift right by (23 - exp) to get integer
    move.l  r3, #23
    sub.l   r3, r3, r1
    lsr.l   r2, r2, r3              ; |n| as integer
    ; Save |n| and sign of n separately
    move.l  r10, r2                 ; r10 = |n| as integer
    lsr.l   r3, r15, #31            ; sign of n
    push    r3                      ; push n_sign (0 or 1)
    bra     .fp_exp_compute_2f

.fp_exp_n_is_zero:
    move.l  r10, r0                 ; n = 0
    push    r0                      ; n_sign = 0

.fp_exp_compute_2f:
    ; Compute e^(f * ln2) using Horner Taylor
    ; e^u = 1 + u*(1 + u/2*(1 + u/3*(1 + u/4*(1 + u/5))))
    ; where u = f * ln(2)
    move.l  r8, r14                 ; f
    move.l  r9, #FP_LN2
    jsr     fp_mul                  ; u = f * ln2
    move.l  r14, r8                 ; r14 = u

    ; Start from inside: 1 + u/5
    move.l  r8, r14
    move.l  r9, #FP_INV_5           ; 1/5
    jsr     fp_mul                  ; u * (1/5)
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 1 + u/5

    ; * u/4, then + 1
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_INV_4           ; 1/4
    jsr     fp_mul                  ; * (1/4)
    move.l  r9, #FP_ONE
    jsr     fp_add

    ; * u/3, then + 1
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_INV_3           ; 1/3
    jsr     fp_mul                  ; * (1/3)
    move.l  r9, #FP_ONE
    jsr     fp_add

    ; * u/2, then + 1
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_INV_2           ; 1/2
    jsr     fp_mul                  ; * (1/2)
    move.l  r9, #FP_ONE
    jsr     fp_add

    ; * u, then + 1
    move.l  r9, r14
    jsr     fp_mul
    move.l  r9, #FP_ONE
    jsr     fp_add                  ; 2^f = e^(f*ln2)

    ; Now multiply by 2^n: add |n| to (or subtract from) the exponent field
    pop     r1                      ; n_sign
    lsr.l   r2, r8, #FP_EXP_SHIFT
    and.l   r2, r2, #0xFF          ; current exponent
    beqz    r1, .fp_exp_add_n
    ; n is negative: subtract |n| from exponent
    blt     r2, r10, .fp_exp_underflow
    sub.l   r2, r2, r10
    bra     .fp_exp_apply_exp
.fp_exp_add_n:
    add.l   r2, r2, r10
    move.l  r3, #0xFF
    bhi     r2, r3, .fp_exp_overflow
.fp_exp_apply_exp:
    beqz    r2, .fp_exp_underflow
    and.l   r8, r8, #0x807FFFFF    ; clear old exponent
    lsl.l   r2, r2, #FP_EXP_SHIFT
    or.l    r8, r8, r2

    pop     r9
    pop     r10
    pop     r15
    pop     r14
    rts

.fp_exp_overflow:
    move.l  r8, #FP_INF
    pop     r9
    pop     r10
    pop     r15
    pop     r14
    rts

.fp_exp_underflow:
    move.l  r8, r0
    pop     r9
    pop     r10
    pop     r15
    pop     r14
    rts

; ============================================================================
; fp_pow: R8 = R8 ^ R9  (R8 raised to the power R9)
; Uses: x^y = e^(y * ln(x))
; ============================================================================
fp_pow:
    push    r14
    push    r9

    move.l  r14, r9                 ; save y
    ; Handle x=0
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fp_pow_zero

    jsr     fp_log                  ; ln(x)
    move.l  r9, r14                 ; y
    jsr     fp_mul                  ; y * ln(x)
    jsr     fp_exp                  ; e^(y * ln(x))

    pop     r9
    pop     r14
    rts

.fp_pow_zero:
    ; 0^y = 0 for y > 0
    move.l  r8, r0
    pop     r9
    pop     r14
    rts

; ============================================================================
; fp_print: Print R8 as decimal string to terminal
; ============================================================================
; Input:  R8  = FP32 value to print
;         R26 = cached TERM_OUT address
; Output: Printed to terminal
; Clobbers: R1-R7, R8-R11, R14, R15
;
; Strategy:
;   1. Handle special cases: zero, negative
;   2. Convert to integer if exact, otherwise print with decimals
;   3. For integer values: extract int, print digits
;   4. For fractional: multiply by 10 repeatedly to extract digits

fp_print:
    push    r14
    push    r15

    ; Check for zero (both +0 and -0)
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fpp_zero

    ; Check sign — print '-' if negative
    lsr.l   r1, r8, #31
    beqz    r1, .fpp_positive
    ; Negative: print '-' and negate
    move.q  r1, #0x2D            ; '-'
    store.b r1, (r26)
    jsr     fp_neg               ; R8 = |R8|

.fpp_positive:
    ; Save the original value
    move.q  r14, r8              ; R14 = original |value|

    ; Check if this is an integer value (no fractional part)
    ; fp_int truncates to integer, then compare
    push    r8
    jsr     fp_int               ; R8 = truncated
    move.q  r15, r8              ; R15 = integer part (FP32)
    pop     r8
    ; Compare: if R8 == R15, it's an exact integer
    move.q  r9, r15
    jsr     fp_cmp
    bnez    r8, .fpp_has_frac

    ; === Integer path: print the truncated integer ===
    move.q  r8, r15
    jsr     .fpp_print_int_fp32
    bra     .fpp_done

.fpp_has_frac:
    ; === Fractional path ===
    ; Print integer part
    move.q  r8, r15
    jsr     .fpp_print_int_fp32

    ; Print decimal point
    move.q  r1, #0x2E            ; '.'
    store.b r1, (r26)

    ; Get fractional part: frac = |value| - integer_part
    move.q  r8, r14              ; original |value|
    move.q  r9, r15              ; integer part
    jsr     fp_sub               ; R8 = fractional part

    ; Print up to 6 fractional digits
    move.q  r14, #6              ; digit count
    move.q  r15, r0              ; trailing zero count

.fpp_frac_loop:
    beqz    r14, .fpp_frac_done

    ; Multiply by 10
    move.l  r9, #0x41200000      ; 10.0
    jsr     fp_mul               ; R8 = frac * 10

    ; Extract integer digit
    push    r8
    jsr     fp_int               ; R8 = floor(frac*10)
    move.q  r11, r8              ; R11 = digit as FP32
    ; Convert to actual integer
    jsr     fp_fix_local         ; R8 = integer digit (0-9)
    move.q  r10, r8              ; R10 = digit int

    ; Print the digit
    add.q   r1, r10, #0x30      ; ASCII
    store.b r1, (r26)

    ; Subtract integer digit from scaled value to get remaining frac
    pop     r8                   ; R8 = scaled value
    move.q  r9, r11              ; R9 = digit as FP32
    jsr     fp_sub               ; R8 = remainder

    sub.q   r14, r14, #1

    ; Check if remainder is zero (no more digits needed)
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fpp_frac_done

    bra     .fpp_frac_loop

.fpp_frac_done:
    bra     .fpp_done

.fpp_zero:
    ; Print "0"
    move.q  r1, #0x30
    store.b r1, (r26)

.fpp_done:
    pop     r15
    pop     r14
    rts

; --- Helper: Print integer part of FP32 ---
; Input: R8 = FP32 value (non-negative integer)
; Clobbers: R1-R7, R8
.fpp_print_int_fp32:
    ; Convert FP32 to raw integer
    jsr     fp_fix_local         ; R8 = integer value
    ; If zero, print "0"
    beqz    r8, .fpp_pi_zero

    ; Push digits in reverse order
    move.q  r7, r0               ; digit count
    move.q  r1, r8
    and.l   r1, r1, #0xFFFFFFFF

.fpp_pi_div:
    beqz    r1, .fpp_pi_emit
    move.q  r2, r1
    move.q  r3, #10
    divu.l  r2, r2, r3           ; quotient
    mulu.l  r4, r2, r3
    sub.l   r3, r1, r4           ; remainder
    add.q   r3, r3, #0x30        ; ASCII digit
    push    r3
    add.q   r7, r7, #1
    move.q  r1, r2
    bra     .fpp_pi_div

.fpp_pi_emit:
    beqz    r7, .fpp_pi_ret
    pop     r1
    store.b r1, (r26)
    sub.q   r7, r7, #1
    bra     .fpp_pi_emit

.fpp_pi_zero:
    move.q  r1, #0x30
    store.b r1, (r26)
.fpp_pi_ret:
    rts

; --- Helper: fp_fix_local - Convert FP32 to signed int32 ---
; Same as fp_fix in ehbasic_expr.inc but local to avoid cross-include dependency
; Input: R8 = FP32 value
; Output: R8 = signed 32-bit integer
; Clobbers: R1-R4
fp_fix_local:
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fixl_zero
    lsr.l   r2, r8, #31          ; sign
    lsr.l   r3, r8, #23
    and.l   r3, r3, #0xFF        ; biased exponent
    sub.q   r3, r3, #127         ; unbiased exponent
    and.l   r4, r8, #0x7FFFFF
    or.l    r4, r4, #0x800000    ; mantissa with implicit 1
    move.q  r1, #23
    sub.q   r1, r3, r1           ; shift amount = exp - 23
    bgez    r1, .fixl_shl
    move.q  r1, #23
    sub.q   r1, r1, r3
    lsr.l   r4, r4, r1
    bra     .fixl_sign
.fixl_shl:
    lsl.l   r4, r4, r1
.fixl_sign:
    beqz    r2, .fixl_pos
    move.q  r1, r0
    sub.l   r4, r1, r4
.fixl_pos:
    move.q  r8, r4
    rts
.fixl_zero:
    move.q  r8, r0
    rts

; ============================================================================
; fp_print_to_buf: Convert FP32 to decimal string in heap
; ============================================================================
; Input:  R8  = FP32 value
;         R16 = state base (for str_alloc)
; Output: R8  = pointer to null-terminated string in heap
; Clobbers: R1-R11, R14, R15

fp_print_to_buf:
    push    r14
    push    r15

    la      r10, .fptb_scratch   ; R10 = write cursor

    ; Handle zero
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fptb_zero

    ; Handle sign
    lsr.l   r1, r8, #31
    beqz    r1, .fptb_pos
    move.q  r1, #0x2D            ; '-'
    store.b r1, (r10)
    add.q   r10, r10, #1
    jsr     fp_neg

.fptb_pos:
    move.q  r14, r8              ; R14 = |value|
    push    r8
    jsr     fp_int
    move.q  r15, r8              ; R15 = integer part (FP32)
    pop     r8
    move.q  r9, r15
    jsr     fp_cmp
    bnez    r8, .fptb_frac

    ; Integer only
    move.q  r8, r15
    jsr     .fptb_int
    bra     .fptb_finish

.fptb_frac:
    ; Print integer part
    move.q  r8, r15
    jsr     .fptb_int
    ; Print '.'
    move.q  r1, #0x2E
    store.b r1, (r10)
    add.q   r10, r10, #1
    ; Fractional part
    move.q  r8, r14
    move.q  r9, r15
    jsr     fp_sub
    move.q  r14, #6              ; max fractional digits

.fptb_frac_loop:
    beqz    r14, .fptb_finish
    move.l  r9, #0x41200000      ; 10.0
    jsr     fp_mul
    push    r8
    jsr     fp_int
    move.q  r11, r8              ; R11 = digit as FP32
    jsr     fp_fix_local
    add.q   r1, r8, #0x30        ; ASCII digit
    store.b r1, (r10)
    add.q   r10, r10, #1
    pop     r8
    move.q  r9, r11
    jsr     fp_sub
    sub.q   r14, r14, #1
    and.l   r1, r8, #0x7FFFFFFF
    beqz    r1, .fptb_finish
    bra     .fptb_frac_loop

.fptb_zero:
    move.q  r1, #0x30
    store.b r1, (r10)
    add.q   r10, r10, #1

.fptb_finish:
    ; Null-terminate
    store.b r0, (r10)
    ; Calculate length
    la      r11, .fptb_scratch
    sub.q   r1, r10, r11
    ; Allocate heap string and copy
    push    r1
    move.q  r8, r1
    jsr     str_alloc
    pop     r1
    move.q  r9, r8               ; R9 = save heap pointer (return value)
    push    r9
    la      r3, .fptb_scratch
.fptb_copy:
    load.b  r2, (r3)
    store.b r2, (r8)
    beqz    r2, .fptb_copied
    add.q   r3, r3, #1
    add.q   r8, r8, #1
    bra     .fptb_copy
.fptb_copied:
    pop     r8                   ; R8 = heap string pointer
    pop     r15
    pop     r14
    rts

; Helper: write integer part to buffer at R10
.fptb_int:
    jsr     fp_fix_local
    beqz    r8, .fptb_int_zero
    move.q  r1, r8
    and.l   r1, r1, #0xFFFFFFFF
    move.q  r7, r0               ; digit count
.fptb_int_div:
    beqz    r1, .fptb_int_emit
    move.q  r2, r1
    divu.l  r2, r2, #10
    mulu.l  r4, r2, #10
    sub.l   r3, r1, r4
    add.q   r3, r3, #0x30
    push    r3
    add.q   r7, r7, #1
    move.q  r1, r2
    bra     .fptb_int_div
.fptb_int_emit:
    beqz    r7, .fptb_int_ret
    pop     r1
    store.b r1, (r10)
    add.q   r10, r10, #1
    sub.q   r7, r7, #1
    bra     .fptb_int_emit
.fptb_int_zero:
    move.q  r1, #0x30
    store.b r1, (r10)
    add.q   r10, r10, #1
.fptb_int_ret:
    rts

.fptb_scratch:
    dc.b    0, 0, 0, 0, 0, 0, 0, 0
    dc.b    0, 0, 0, 0, 0, 0, 0, 0
    dc.b    0, 0, 0, 0, 0, 0, 0, 0
    dc.b    0, 0, 0, 0, 0, 0, 0, 0
    align 8

; ============================================================================
; fp_input: Parse decimal string -> R8
; (Stub -- will be implemented later)
; ============================================================================
fp_input:
    rts

; ============================================================================
; tron_check - Print [linenum] if TRON trace mode is active
; ============================================================================
; Input:  R1  = line number (integer), R16 = state base, R26 = TERM_OUT
; Output: none (trace printed if flag set)
; Clobbers: R2-R7, R8 (only when trace is on)

tron_check:
    add.q   r2, r16, #ST_TRACE_FLAG
    load.l  r2, (r2)
    beqz    r2, .tc_done
    ; Print '[' linenum ']'
    move.q  r2, #0x5B            ; '['
    store.b r2, (r26)
    move.q  r8, r1               ; R8 = line number (integer)
    jsr     fp_float             ; R8 = FP32(linenum)
    push    r14
    push    r15
    jsr     fp_print             ; prints number to terminal
    pop     r15
    pop     r14
    move.q  r2, #0x5D            ; ']'
    store.b r2, (r26)
.tc_done:
    rts
