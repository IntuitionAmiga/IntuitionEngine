; ie64_fp.inc - IEEE 754 Floating-Point Library for IE64 BASIC
;
; Calling convention:
;   Operands in R8, R9 (as raw IEEE 754 FP64 bit patterns)
;   Result in R8
;   Clobbers R1-R7 only (caller-saved scratch)
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
    jsr     fp64_load_r8_f0
    dneg    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_abs: R8 = |R8|
; ============================================================================
fp_abs:
    jsr     fp64_load_r8_f0
    dabs    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_cmp: Compare R8 vs R9, result in R8: 0xFFFFFFFF (less), 0 (equal), 1 (greater)
; ============================================================================
fp_cmp:
    jsr     fp64_load_r8_f0
    move.q  r8, r9
    jsr     fp64_load_r8_f2
    dcmp    r8, f0, f2
    bltz    r8, .fp_cmp_less
    rts
.fp_cmp_less:
    move.l  r8, #0xFFFFFFFF
    rts

; ============================================================================
; fp_add: R8 = R8 + R9
; ============================================================================
fp_add:
    jsr     fp64_load_r8_f0
    move.q  r8, r9
    jsr     fp64_load_r8_f2
    dadd    f0, f0, f2
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_sub: R8 = R8 - R9
; ============================================================================
fp_sub:
    jsr     fp64_load_r8_f0
    move.q  r8, r9
    jsr     fp64_load_r8_f2
    dsub    f0, f0, f2
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_mul: R8 = R8 * R9
; ============================================================================
fp_mul:
    jsr     fp64_load_r8_f0
    move.q  r8, r9
    jsr     fp64_load_r8_f2
    dmul    f0, f0, f2
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_div: R8 = R8 / R9
; ============================================================================
fp_div:
    jsr     fp64_load_r8_f0
    move.q  r8, r9
    jsr     fp64_load_r8_f2
    ddiv    f0, f0, f2
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_int: R8 = INT(R8) -- truncate towards zero
; ============================================================================
fp_int:
    move.q  r1, #0x02
    beq     r9, r1, .fp_int_i64
    jsr     fp64_load_r8_f0
    dint    f0, f0
    jsr     fp64_store_f0_r8
.fp_int_i64:
    rts

; ============================================================================
; fp_float: R8 = int64 in R8 -> IEEE 754 double in R8
; ============================================================================
fp_float:
    dcvtif  f0, r8
    jsr     fp64_store_f0_r8
    move.q  r9, #0x01
    rts

; ============================================================================
; fp_sqr: R8 = sqrt(R8)
; BASIC convention: finite negatives return 0.0
; NaN inputs are passed to FSQRT to preserve NaN propagation.
; ============================================================================
fp_sqr:
    bgez    r8, .fp_sqr_do
    move.l  r1, #0xFFFFFFFF
    movt    r1, #0x7FFFFFFF
    and.q   r2, r8, r1
    beqz    r2, .fp_sqr_do        ; -0.0
    move.l  r1, #0
    movt    r1, #0x7FF00000
    and.q   r2, r8, r1
    beq     r2, r1, .fp_sqr_do    ; NaN or infinity
    move.q  r8, #0                ; finite negative
    rts
.fp_sqr_do:
    jsr     fp64_load_r8_f0
    dsqrt   f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_sin: R8 = sin(R8)
; ============================================================================
fp_sin:
    jsr     fp64_load_r8_f0
    dsin    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_cos: R8 = cos(R8)
; ============================================================================
fp_cos:
    jsr     fp64_load_r8_f0
    dcos    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_tan: R8 = tan(R8)
; ============================================================================
fp_tan:
    jsr     fp64_load_r8_f0
    dtan    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_atn: R8 = atan(R8)
; ============================================================================
fp_atn:
    jsr     fp64_load_r8_f0
    datan   f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_log: R8 = ln(R8)
; ============================================================================
fp_log:
    jsr     fp64_load_r8_f0
    dlog    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_exp: R8 = e^R8
; ============================================================================
fp_exp:
    jsr     fp64_load_r8_f0
    dexp    f0, f0
    jsr     fp64_store_f0_r8
    rts

; ============================================================================
; fp_pow: R8 = R8 ^ R9
; ============================================================================
fp_pow:
    jsr     fp64_load_r8_f0
    move.q  r8, r9
    jsr     fp64_load_r8_f2
    dpow    f0, f0, f2
    jsr     fp64_store_f0_r8
    rts

; FP64 bit staging helpers. They use DLOAD/DSTORE so FPSR only reflects the
; requested FP64 operation, never a half-payload bitcast.
fp64_load_r8_f0:
    la      r1, fp64_scratch
    store.q r8, (r1)
    dload   f0, (r1)
    rts

fp64_load_r8_f2:
    la      r1, fp64_scratch
    store.q r8, (r1)
    dload   f2, (r1)
    rts

fp64_store_f0_r8:
    la      r1, fp64_scratch
    dstore  f0, (r1)
    load.q  r8, (r1)
    rts

fp64_scratch:
    dc.q    0

fp64_const_10:
    dc.q    0x4024000000000000

; ============================================================================
; fp_print: Print R8 as decimal string to terminal
; ============================================================================
; Input:  R8  = FP64 value to print
;         R26 = cached TERM_OUT address
; Output: Printed to terminal
; Clobbers: R1-R7, R8-R11, R14, R15

fp_print:
    bra     fp_print_f64

; ============================================================================
; fp_print_f64: Print R8 as decimal string to terminal.
; ============================================================================
; Compact FP64 formatter for BASIC's value model. It prints integer values
; without a decimal point and otherwise emits up to six fractional digits. Keep
; this before fp_print_to_buf so the AOT fp_print closure remains self-contained.
fp_print_f64:
    push    r14
    push    r15

    move.q  r14, r8
    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_cmp
    beqz    r8, .f64p_zero
    move.q  r8, r14

    lsr.q   r1, r8, #63
    beqz    r1, .f64p_positive
    move.q  r1, #0x2D
    store.b r1, (r26)
    jsr     fp_abs

.f64p_positive:
    move.q  r14, r8
    jsr     fp_fix_f64_local
    move.q  r15, r8
    jsr     f64p_print_uint

    move.q  r8, r15
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_sub
    move.q  r14, r8

    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_cmp
    beqz    r8, .f64p_done

    move.q  r1, #0x2E
    store.b r1, (r26)
    move.q  r15, #6

.f64p_frac_loop:
    beqz    r15, .f64p_done
    move.q  r8, r14
    la      r1, fp64_const_10
    load.q  r9, (r1)
    jsr     fp_mul
    move.q  r14, r8
    jsr     fp_fix_f64_local
    move.q  r11, r8
    add.q   r1, r11, #0x30
    store.b r1, (r26)

    move.q  r8, r11
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_sub
    move.q  r14, r8

    sub.q   r15, r15, #1
    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_cmp
    beqz    r8, .f64p_done
    bra     .f64p_frac_loop

.f64p_zero:
    move.q  r1, #0x30
    store.b r1, (r26)

.f64p_done:
    pop     r15
    pop     r14
    rts

fp_fix_f64_local:
    jsr     fp64_load_r8_f0
    dcvtfi  r8, f0
    rts

f64p_print_uint:
    beqz    r8, .f64p_pi_zero
    move.q  r7, r0
    move.q  r1, r8

.f64p_pi_div:
    beqz    r1, .f64p_pi_emit
    move.q  r2, r1
    move.q  r3, #10
    divu.q  r2, r2, r3
    mulu.q  r4, r2, r3
    sub.q   r3, r1, r4
    add.q   r3, r3, #0x30
    push    r3
    add.q   r7, r7, #1
    move.q  r1, r2
    bra     .f64p_pi_div

.f64p_pi_emit:
    beqz    r7, .f64p_pi_ret
    pop     r1
    store.b r1, (r26)
    sub.q   r7, r7, #1
    bra     .f64p_pi_emit

.f64p_pi_zero:
    move.q  r1, #0x30
    store.b r1, (r26)
.f64p_pi_ret:
    rts

; ============================================================================
; fp_print_to_buf: Convert FP64 to decimal string in heap
; ============================================================================
; Input:  R8  = FP64 value
;         R16 = state base (for str_alloc)
; Output: R8  = pointer to null-terminated string in heap
; Clobbers: R1-R11, R14, R15

fp_print_to_buf:
    push    r14
    push    r15

    la      r10, .fptb_scratch   ; R10 = write cursor

    li      r1, #0x7FFFFFFFFFFFFFFF
    and.q   r2, r8, r1
    beqz    r2, .fptb_zero

    ; Handle sign
    lsr.q   r1, r8, #63
    beqz    r1, .fptb_pos
    move.q  r1, #0x2D            ; '-'
    store.b r1, (r10)
    add.q   r10, r10, #1
    jsr     fp_neg

.fptb_pos:
    move.q  r14, r8              ; R14 = absolute value
    jsr     fp_fix_f64_local
    move.q  r15, r8              ; R15 = integer part

    move.q  r8, r15
    jsr     .fptb_uint

    move.q  r8, r15
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_sub
    move.q  r14, r8

    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_cmp
    beqz    r8, .fptb_finish

.fptb_frac:
    move.q  r1, #0x2E
    store.b r1, (r10)
    add.q   r10, r10, #1
    move.q  r15, #6              ; max fractional digits

.fptb_frac_loop:
    beqz    r15, .fptb_finish
    move.q  r8, r14
    la      r1, fp64_const_10
    load.q  r9, (r1)
    jsr     fp_mul
    move.q  r14, r8
    jsr     fp_fix_f64_local
    move.q  r11, r8
    add.q   r1, r11, #0x30       ; ASCII digit
    store.b r1, (r10)
    add.q   r10, r10, #1

    move.q  r8, r11
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_sub
    move.q  r14, r8

    sub.q   r15, r15, #1
    move.q  r8, r0
    jsr     fp_float
    move.q  r9, r8
    move.q  r8, r14
    jsr     fp_cmp
    beqz    r8, .fptb_finish
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
    bnez    r28, .fptb_oom
    beqz    r8, .fptb_oom
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
.fptb_oom:
    move.q  r8, r0
    pop     r15
    pop     r14
    rts

; Helper: write unsigned integer in R8 to buffer at R10
.fptb_uint:
    beqz    r8, .fptb_int_zero
    move.q  r1, r8
    move.q  r7, r0               ; digit count
.fptb_int_div:
    beqz    r1, .fptb_int_emit
    move.q  r2, r1
    divu.q  r2, r2, #10
    mulu.q  r4, r2, #10
    sub.q   r3, r1, r4
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
    jsr     fp_float             ; R8 = FP64(linenum)
    push    r14
    push    r15
    jsr     fp_print             ; prints number to terminal
    pop     r15
    pop     r14
    move.q  r2, #0x5D            ; ']'
    store.b r2, (r26)
.tc_done:
    rts
