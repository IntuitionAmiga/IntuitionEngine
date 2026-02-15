; ehbasic_strings.inc - EhBASIC IE64 String Operations
;
; String expression evaluator and string functions for EhBASIC.
;
; Strings are stored as null-terminated bytes in the string heap
; (BASIC_STR_TEMP area, 16KB). String variables hold a 4-byte pointer
; to their string data in the heap.
;
; Key routines:
;   str_detect    - Non-consuming peek: is current token a string expression?
;   str_eval      - Evaluate string expression; returns pointer in R8
;   str_alloc     - Allocate bytes in string heap
;   str_len       - Length of null-terminated string
;   str_copy      - Copy null-terminated string
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; str_detect - Check if current position starts a string expression
; ============================================================================
; Input:  R17 = text pointer (NOT consumed)
; Output: R8  = 1 if string expression, 0 if not
; Clobbers: R1-R3

str_detect:
    load.b  r1, (r17)
    ; String function tokens that RETURN strings
    move.q  r2, #TK_CHRS
    beq     r1, r2, .sd_yes
    move.q  r2, #TK_LEFTS
    beq     r1, r2, .sd_yes
    move.q  r2, #TK_RIGHTS
    beq     r1, r2, .sd_yes
    move.q  r2, #TK_MIDS
    beq     r1, r2, .sd_yes
    move.q  r2, #TK_STRS
    beq     r1, r2, .sd_yes
    move.q  r2, #TK_HEXS
    beq     r1, r2, .sd_yes
    move.q  r2, #TK_BINS
    beq     r1, r2, .sd_yes
    ; Check for string variable: letter(s) followed by '$'
    move.q  r2, #0x41            ; 'A'
    blt     r1, r2, .sd_no
    move.q  r2, #0x7A            ; 'z'
    bgt     r1, r2, .sd_no
    ; Scan ahead past variable name chars to find '$'
    move.q  r3, r17
.sd_scan:
    add.q   r3, r3, #1
    load.b  r1, (r3)
    move.q  r2, #0x41
    blt     r1, r2, .sd_ck_dig
    move.q  r2, #0x5A
    ble     r1, r2, .sd_scan
    move.q  r2, #0x61
    blt     r1, r2, .sd_ck_dig
    move.q  r2, #0x7A
    ble     r1, r2, .sd_scan
.sd_ck_dig:
    move.q  r2, #0x30
    blt     r1, r2, .sd_ck_dlr
    move.q  r2, #0x39
    ble     r1, r2, .sd_scan
.sd_ck_dlr:
    move.q  r2, #0x24            ; '$'
    beq     r1, r2, .sd_yes
.sd_no:
    move.q  r8, r0
    rts
.sd_yes:
    move.q  r8, #1
    rts

; ============================================================================
; str_eval - Evaluate a string expression (with concatenation)
; ============================================================================
; Input:  R17 = text pointer
; Output: R8  = pointer to null-terminated string data
;         R17 = advanced past expression
; Clobbers: R1-R15

str_eval:
    push    r22
    jsr     str_primary          ; R8 = first string pointer
    move.q  r22, r8
    ; Check for concatenation '+'
.se_cat_loop:
    load.b  r1, (r17)
    move.q  r2, #TK_PLUS
    beq     r1, r2, .se_do_cat
    move.q  r2, #0x2B            ; raw '+'
    beq     r1, r2, .se_do_cat
    move.q  r8, r22
    pop     r22
    rts

.se_do_cat:
    add.q   r17, r17, #1         ; consume '+'
    push    r22                  ; save left ptr
    jsr     str_primary          ; R8 = right
    move.q  r14, r8              ; R14 = right
    pop     r15                  ; R15 = left
    ; Get left length
    push    r14
    push    r15
    move.q  r8, r15
    jsr     str_len              ; R8 = left_len
    move.q  r3, r8
    pop     r15
    pop     r14
    ; Get right length
    push    r3
    push    r14
    push    r15
    move.q  r8, r14
    jsr     str_len              ; R8 = right_len
    pop     r15
    pop     r14
    pop     r3
    ; Allocate total
    add.q   r1, r3, r8
    push    r14
    push    r15
    move.q  r8, r1
    jsr     str_alloc            ; R8 = buffer
    move.q  r22, r8
    pop     r15
    pop     r14
    ; Copy left
    push    r14
    move.q  r8, r22
    move.q  r9, r15
    jsr     str_copy             ; R8 = at null terminator
    move.q  r3, r8               ; R3 = append point
    pop     r14
    ; Copy right
    move.q  r8, r3
    move.q  r9, r14
    jsr     str_copy
    ; R22 = concatenated string
    bra     .se_cat_loop

; ============================================================================
; str_primary - Parse a single string value
; ============================================================================
; Input:  R17 = text pointer
; Output: R8  = pointer to string data
;         R17 = advanced
; Clobbers: R1-R15

str_primary:
    load.b  r1, (r17)
    ; String literal
    move.q  r2, #0x22            ; '"'
    beq     r1, r2, .sp_literal
    ; String function tokens
    move.q  r2, #TK_CHRS
    beq     r1, r2, .sp_chr
    move.q  r2, #TK_LEFTS
    beq     r1, r2, .sp_left
    move.q  r2, #TK_RIGHTS
    beq     r1, r2, .sp_right
    move.q  r2, #TK_MIDS
    beq     r1, r2, .sp_mid
    move.q  r2, #TK_STRS
    beq     r1, r2, .sp_str
    move.q  r2, #TK_HEXS
    beq     r1, r2, .sp_hex
    move.q  r2, #TK_BINS
    beq     r1, r2, .sp_bin
    ; Must be string variable: name + '$'
    jsr     svar_read            ; R8 = string data pointer
    rts

; --- String literal ---
.sp_literal:
    add.q   r17, r17, #1         ; skip opening '"'
    ; Count length
    move.q  r3, r17
    move.q  r4, r0
.sp_lit_cnt:
    load.b  r1, (r3)
    beqz    r1, .sp_lit_alloc
    move.q  r2, #0x22
    beq     r1, r2, .sp_lit_alloc
    add.q   r3, r3, #1
    add.q   r4, r4, #1
    bra     .sp_lit_cnt
.sp_lit_alloc:
    push    r4
    move.q  r8, r4
    jsr     str_alloc            ; R8 = buffer
    pop     r4
    move.q  r5, r8               ; R5 = save base for return
    move.q  r3, r8               ; R3 = write cursor
    move.q  r6, r4               ; R6 = count
.sp_lit_cp:
    beqz    r6, .sp_lit_end
    load.b  r1, (r17)
    store.b r1, (r3)
    add.q   r17, r17, #1
    add.q   r3, r3, #1
    sub.q   r6, r6, #1
    bra     .sp_lit_cp
.sp_lit_end:
    store.b r0, (r3)             ; null terminator
    ; Skip closing '"'
    load.b  r1, (r17)
    move.q  r2, #0x22
    bne     r1, r2, .sp_lit_ret
    add.q   r17, r17, #1
.sp_lit_ret:
    move.q  r8, r5               ; return base pointer
    rts

; --- CHR$(n) ---
.sp_chr:
    add.q   r17, r17, #1         ; consume TK_CHRS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_chr_err
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or
    jsr     fp_int
    jsr     fp_fix               ; R8 = ASCII code
    move.q  r3, r8
    ; skip ')'
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_chr_mk
    add.q   r17, r17, #1
.sp_chr_mk:
    push    r3
    move.q  r8, #1
    jsr     str_alloc
    pop     r3
    store.b r3, (r8)
    add.q   r1, r8, #1
    store.b r0, (r1)
    rts
.sp_chr_err:
    move.q  r8, r0
    rts

; --- LEFT$(s$, n) ---
.sp_left:
    add.q   r17, r17, #1         ; consume TK_LEFTS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_func_err
    add.q   r17, r17, #1         ; skip '('
    jsr     str_eval             ; R8 = string pointer
    move.q  r14, r8
    ; skip ','
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sp_func_err
    add.q   r17, r17, #1
    push    r14
    jsr     expr_or
    jsr     fp_int
    jsr     fp_fix               ; R8 = count
    move.q  r15, r8
    pop     r14
    ; skip ')'
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_left_do
    add.q   r17, r17, #1
.sp_left_do:
    ; Clamp count to actual string length
    push    r14
    push    r15
    move.q  r8, r14
    jsr     str_len              ; R8 = actual length
    pop     r15
    pop     r14
    bge     r8, r15, .sp_left_ok
    move.q  r15, r8              ; clamp to actual
.sp_left_ok:
    push    r14
    push    r15
    move.q  r8, r15
    jsr     str_alloc
    pop     r15
    pop     r14
    move.q  r5, r8               ; save base
    move.q  r3, r8               ; write cursor
    move.q  r4, r15              ; count
.sp_left_cp:
    beqz    r4, .sp_left_end
    load.b  r1, (r14)
    store.b r1, (r3)
    add.q   r14, r14, #1
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    bra     .sp_left_cp
.sp_left_end:
    store.b r0, (r3)
    move.q  r8, r5
    rts

; --- RIGHT$(s$, n) ---
.sp_right:
    add.q   r17, r17, #1         ; consume TK_RIGHTS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_func_err
    add.q   r17, r17, #1         ; skip '('
    jsr     str_eval             ; R8 = string pointer
    move.q  r14, r8
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sp_func_err
    add.q   r17, r17, #1
    push    r14
    jsr     expr_or
    jsr     fp_int
    jsr     fp_fix               ; R8 = count
    move.q  r15, r8
    pop     r14
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_right_do
    add.q   r17, r17, #1
.sp_right_do:
    ; Get string length
    push    r14
    push    r15
    move.q  r8, r14
    jsr     str_len              ; R8 = length
    pop     r15
    pop     r14
    ; start = length - count
    sub.q   r3, r8, r15
    bgez    r3, .sp_right_ofs
    move.q  r3, r0
    move.q  r15, r8              ; clamp count to full length
.sp_right_ofs:
    add.q   r14, r14, r3         ; advance source to start offset
    push    r14
    push    r15
    move.q  r8, r15
    jsr     str_alloc
    pop     r15
    pop     r14
    move.q  r5, r8               ; save base
    move.q  r3, r8               ; write cursor
    move.q  r4, r15
.sp_right_cp:
    beqz    r4, .sp_right_end
    load.b  r1, (r14)
    beqz    r1, .sp_right_end
    store.b r1, (r3)
    add.q   r14, r14, #1
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    bra     .sp_right_cp
.sp_right_end:
    store.b r0, (r3)
    move.q  r8, r5
    rts

.sp_func_err:
    move.q  r8, r0
    rts

; --- MID$(s$, pos, len) ---
.sp_mid:
    add.q   r17, r17, #1         ; consume TK_MIDS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_mid_err
    add.q   r17, r17, #1         ; skip '('
    jsr     str_eval             ; R8 = string pointer
    move.q  r14, r8
    ; skip ','
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sp_mid_err
    add.q   r17, r17, #1
    ; Second arg: position (1-based)
    push    r14
    jsr     expr_or
    jsr     fp_int
    jsr     fp_fix               ; R8 = position
    move.q  r15, r8
    pop     r14
    ; skip ','
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sp_mid_err
    add.q   r17, r17, #1
    ; Third arg: length
    push    r14
    push    r15
    jsr     expr_or
    jsr     fp_int
    jsr     fp_fix               ; R8 = length
    move.q  r3, r8
    pop     r15
    pop     r14
    ; skip ')'
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_mid_do
    add.q   r17, r17, #1
.sp_mid_do:
    ; Convert 1-based to 0-based
    sub.q   r15, r15, #1
    add.q   r14, r14, r15        ; advance source to start
    ; Allocate and copy
    push    r14
    push    r3
    move.q  r8, r3
    jsr     str_alloc
    pop     r3
    pop     r14
    move.q  r5, r8               ; save base
    move.q  r4, r8               ; write cursor
    move.q  r6, r3               ; count
.sp_mid_cp:
    beqz    r6, .sp_mid_end
    load.b  r1, (r14)
    beqz    r1, .sp_mid_end
    store.b r1, (r4)
    add.q   r14, r14, #1
    add.q   r4, r4, #1
    sub.q   r6, r6, #1
    bra     .sp_mid_cp
.sp_mid_end:
    store.b r0, (r4)
    move.q  r8, r5
    rts
.sp_mid_err:
    move.q  r8, r0
    rts

; --- STR$(n) ---
.sp_str:
    add.q   r17, r17, #1         ; consume TK_STRS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_str_err
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or              ; R8 = FP32 value
    ; skip ')'
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_str_do
    add.q   r17, r17, #1
.sp_str_do:
    push    r14
    push    r15
    jsr     fp_print_to_buf      ; R8 = pointer to heap string
    pop     r15
    pop     r14
    rts
.sp_str_err:
    move.q  r8, r0
    rts

; --- HEX$(n) ---
.sp_hex:
    add.q   r17, r17, #1         ; consume TK_HEXS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_hex_err
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or              ; R8 = FP32 value
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer value
    ; skip ')'
    move.q  r3, r8               ; R3 = integer to convert
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_hex_do
    add.q   r17, r17, #1
.sp_hex_do:
    ; Convert R3 to hex string
    ; First pass: count hex digits (at least 1)
    move.q  r4, r3               ; R4 = working copy
    move.q  r5, r0               ; R5 = digit count
    ; Handle zero specially
    bnez    r4, .sp_hex_cnt
    move.q  r5, #1
    bra     .sp_hex_alloc
.sp_hex_cnt:
    beqz    r4, .sp_hex_alloc
    lsr.l   r4, r4, #4
    add.q   r5, r5, #1
    bra     .sp_hex_cnt
.sp_hex_alloc:
    push    r3
    push    r5
    move.q  r8, r5
    jsr     str_alloc            ; R8 = buffer
    pop     r5
    pop     r3
    move.q  r6, r8               ; R6 = buffer base (for return)
    ; Write digits from end to start (reverse order)
    add.q   r7, r8, r5           ; R7 = one past last digit
    store.b r0, (r7)             ; null terminator
    sub.q   r7, r7, #1           ; R7 = last digit position
    move.q  r4, r3               ; R4 = working copy
    ; Handle zero
    bnez    r4, .sp_hex_wr
    move.q  r1, #0x30            ; '0'
    store.b r1, (r7)
    move.q  r8, r6
    rts
.sp_hex_wr:
    beqz    r4, .sp_hex_done
    and.l   r1, r4, #0x0F       ; low nibble
    move.q  r2, #10
    blt     r1, r2, .sp_hex_dig
    ; A-F
    sub.q   r1, r1, #10
    add.q   r1, r1, #0x41       ; 'A'
    bra     .sp_hex_st
.sp_hex_dig:
    add.q   r1, r1, #0x30       ; '0'
.sp_hex_st:
    store.b r1, (r7)
    sub.q   r7, r7, #1
    lsr.l   r4, r4, #4
    bra     .sp_hex_wr
.sp_hex_done:
    move.q  r8, r6
    rts
.sp_hex_err:
    move.q  r8, r0
    rts

; --- BIN$(n) ---
.sp_bin:
    add.q   r17, r17, #1         ; consume TK_BINS
    load.b  r1, (r17)
    move.q  r2, #0x28
    bne     r1, r2, .sp_bin_err
    add.q   r17, r17, #1         ; skip '('
    jsr     expr_or              ; R8 = FP32 value
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer value
    ; skip ')'
    move.q  r3, r8               ; R3 = integer to convert
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .sp_bin_do
    add.q   r17, r17, #1
.sp_bin_do:
    ; Convert R3 to binary string
    ; First pass: count bits (at least 1)
    move.q  r4, r3               ; R4 = working copy
    move.q  r5, r0               ; R5 = digit count
    ; Handle zero specially
    bnez    r4, .sp_bin_cnt
    move.q  r5, #1
    bra     .sp_bin_alloc
.sp_bin_cnt:
    beqz    r4, .sp_bin_alloc
    lsr.l   r4, r4, #1
    add.q   r5, r5, #1
    bra     .sp_bin_cnt
.sp_bin_alloc:
    push    r3
    push    r5
    move.q  r8, r5
    jsr     str_alloc            ; R8 = buffer
    pop     r5
    pop     r3
    move.q  r6, r8               ; R6 = buffer base (for return)
    ; Write digits from end to start (reverse order)
    add.q   r7, r8, r5           ; R7 = one past last digit
    store.b r0, (r7)             ; null terminator
    sub.q   r7, r7, #1           ; R7 = last digit position
    move.q  r4, r3               ; R4 = working copy
    ; Handle zero
    bnez    r4, .sp_bin_wr
    move.q  r1, #0x30            ; '0'
    store.b r1, (r7)
    move.q  r8, r6
    rts
.sp_bin_wr:
    beqz    r4, .sp_bin_done
    and.l   r1, r4, #1           ; low bit
    add.q   r1, r1, #0x30       ; '0' or '1'
    store.b r1, (r7)
    sub.q   r7, r7, #1
    lsr.l   r4, r4, #1
    bra     .sp_bin_wr
.sp_bin_done:
    move.q  r8, r6
    rts
.sp_bin_err:
    move.q  r8, r0
    rts

; ============================================================================
; str_alloc - Allocate bytes in string heap
; ============================================================================
; Input:  R8  = number of characters needed (null terminator added)
; Output: R8  = pointer to allocated buffer
; Clobbers: R1-R3

str_alloc:
    add.q   r1, r16, #ST_HEAP_TOP
    load.l  r2, (r1)             ; R2 = current heap top
    move.q  r3, r2               ; R3 = return value (old top)
    add.q   r8, r8, #1           ; +1 for null terminator
    add.q   r2, r2, r8           ; advance top
    store.l r2, (r1)
    move.q  r8, r3
    rts

; ============================================================================
; str_len - Get length of null-terminated string
; ============================================================================
; Input:  R8 = pointer to null-terminated string
; Output: R8 = length (not including null)
; Clobbers: R1, R2

str_len:
    move.q  r2, r0               ; counter
.slen_loop:
    load.b  r1, (r8)
    beqz    r1, .slen_done
    add.q   r2, r2, #1
    add.q   r8, r8, #1
    bra     .slen_loop
.slen_done:
    move.q  r8, r2
    rts

; ============================================================================
; str_copy - Copy null-terminated string
; ============================================================================
; Input:  R8 = dst, R9 = src
; Output: R8 = pointer to null terminator in dst (for appending)
; Clobbers: R1

str_copy:
.scpy_loop:
    load.b  r1, (r9)
    store.b r1, (r8)
    beqz    r1, .scpy_done
    add.q   r8, r8, #1
    add.q   r9, r9, #1
    bra     .scpy_loop
.scpy_done:
    rts

; ============================================================================
; EOF
; ============================================================================
