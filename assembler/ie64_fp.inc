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
; fp_neg: R8 = -R8
; ============================================================================
fp_neg:
    fmovi   f0, r8
    fneg    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_abs: R8 = |R8|
; ============================================================================
fp_abs:
    fmovi   f0, r8
    fabs    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_cmp: Compare R8 vs R9, result in R8: -1 (less), 0 (equal), 1 (greater)
; ============================================================================
fp_cmp:
    fmovi   f0, r8
    fmovi   f1, r9
    fcmp    r8, f0, f1
    and.l   r8, r8, #0xFFFFFFFF   ; truncate to 32-bit (FCMP sign-extends -1 to 64-bit)
    rts

; ============================================================================
; fp_add: R8 = R8 + R9
; ============================================================================
fp_add:
    fmovi   f0, r8
    fmovi   f1, r9
    fadd    f0, f0, f1
    fmovo   r8, f0
    rts

; ============================================================================
; fp_sub: R8 = R8 - R9
; ============================================================================
fp_sub:
    fmovi   f0, r8
    fmovi   f1, r9
    fsub    f0, f0, f1
    fmovo   r8, f0
    rts

; ============================================================================
; fp_mul: R8 = R8 * R9
; ============================================================================
fp_mul:
    fmovi   f0, r8
    fmovi   f1, r9
    fmul    f0, f0, f1
    fmovo   r8, f0
    rts

; ============================================================================
; fp_div: R8 = R8 / R9
; ============================================================================
fp_div:
    fmovi   f0, r8
    fmovi   f1, r9
    fdiv    f0, f0, f1
    fmovo   r8, f0
    rts

; ============================================================================
; fp_int: R8 = INT(R8) -- truncate towards zero
; ============================================================================
fp_int:
    fmovi   f0, r8
    fint    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_float: R8 = int32 in R8 -> IEEE 754 float in R8
; ============================================================================
fp_float:
    fcvtif  f0, r8
    fmovo   r8, f0
    rts

; ============================================================================
; fp_sqr: R8 = sqrt(R8)
; BASIC convention: finite negatives return 0.0
; NaN inputs are passed to FSQRT to preserve NaN propagation.
; ============================================================================
fp_sqr:
    lsr.l   r1, r8, #23
    and.l   r1, r1, #0xFF          ; exponent
    move.l  r2, #0xFF
    bne     r1, r2, .fp_sqr_check_neg
    and.l   r1, r8, #0x7FFFFF      ; mantissa
    bnez    r1, .fp_sqr_do_sqrt     ; NaN -> propagate via FSQRT

.fp_sqr_check_neg:
    lsr.l   r1, r8, #31
    bnez    r1, .fp_sqr_neg

.fp_sqr_do_sqrt:
    fmovi   f0, r8
    fsqrt   f0, f0
    fmovo   r8, f0
    rts

.fp_sqr_neg:
    move.l  r8, #0
    rts

; ============================================================================
; fp_sin: R8 = sin(R8)
; ============================================================================
fp_sin:
    fmovi   f0, r8
    fsin    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_cos: R8 = cos(R8)
; ============================================================================
fp_cos:
    fmovi   f0, r8
    fcos    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_tan: R8 = tan(R8)
; ============================================================================
fp_tan:
    fmovi   f0, r8
    ftan    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_atn: R8 = atan(R8)
; ============================================================================
fp_atn:
    fmovi   f0, r8
    fatan   f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_log: R8 = ln(R8)
; ============================================================================
fp_log:
    fmovi   f0, r8
    flog    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_exp: R8 = e^R8
; ============================================================================
fp_exp:
    fmovi   f0, r8
    fexp    f0, f0
    fmovo   r8, f0
    rts

; ============================================================================
; fp_pow: R8 = R8 ^ R9
; ============================================================================
fp_pow:
    fmovi   f0, r8
    fmovi   f1, r9
    fpow    f0, f0, f1
    fmovo   r8, f0
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
    fmovi   f0, r8
    fcvtfi  r8, f0
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
