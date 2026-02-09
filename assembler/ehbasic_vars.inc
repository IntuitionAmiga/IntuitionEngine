; ehbasic_vars.inc - EhBASIC IE64 Variable Storage
;
; Manages BASIC variables: simple numeric scalars and numeric arrays.
;
; Simple variable storage format (8 bytes each):
;   +0 (4 bytes): Name tag (packed uppercase, first char in MSB)
;   +4 (4 bytes): Value (FP32 bits)
;
; Array storage format (variable-length):
;   +0  (4 bytes): Name tag
;   +4  (4 bytes): Total element count
;   +8  (4 bytes): Dim2 size (0 = 1D)
;   +12 (N*4 bytes): Element data (FP32)
;
; Register conventions:
;   R16 = interpreter state base (BASIC_STATE)
;   R17 = execution pointer (text cursor)
;
; Calling conventions:
;   var_init:    Initialise variable storage. R16 must be set.
;   var_lookup:  R17 = points to variable name in token stream.
;                Returns: R8 = pointer to 4-byte value slot (or array element).
;                         R17 = advanced past variable name (and subscript if array).
;                Clobbers: R1-R15
;   var_read:    Same as var_lookup but returns FP32 value in R8.
;   arr_dim:     DIM handler: R17 = points past DIM token.
;                Creates array with specified dimensions.
;                Clobbers: R1-R15
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; var_init - Initialise variable and array storage
; ============================================================================

var_init:
    ; Simple variables: 0x050000 (above 128KB code reservation at 0x1000-0x20FFF)
    la      r1, 0x050000
    add.q   r2, r16, #ST_VAR_START
    store.l r1, (r2)
    add.q   r2, r16, #ST_VAR_END
    store.l r1, (r2)
    ; String variables: 0x058000
    la      r1, 0x058000
    add.q   r2, r16, #ST_SVAR_START
    store.l r1, (r2)
    add.q   r2, r16, #ST_SVAR_END
    store.l r1, (r2)
    ; Arrays: 0x060000
    la      r1, 0x060000
    add.q   r2, r16, #ST_ARRAY_START
    store.l r1, (r2)
    add.q   r2, r16, #ST_ARRAY_END
    store.l r1, (r2)
    ; String heap: BASIC_STR_TEMP
    la      r1, BASIC_STR_TEMP
    add.q   r2, r16, #ST_HEAP_TOP
    store.l r1, (r2)
    ; Clear variable cache (4 slots × 8 bytes = 32 bytes)
    la      r2, var_cache
    store.q r0, (r2)
    store.q r0, 8(r2)
    store.q r0, 16(r2)
    store.q r0, 24(r2)
    rts

; ============================================================================
; var_parse_name - Parse a variable name from the token stream
; ============================================================================
; Input:  R17 = points to variable name
; Output: R14 = packed name tag
;         R15 = character count (0 if no name)
;         R17 = advanced past name characters
; Clobbers: R1, R2

var_parse_name:
    move.q  r14, r0              ; R14 = packed name
    move.q  r15, r0              ; R15 = char count

.vpn_collect:
    load.b  r1, (r17)

    ; Check if letter (A-Z, a-z)
    move.q  r2, #0x41            ; 'A'
    blt     r1, r2, .vpn_check_digit
    move.q  r2, #0x5A            ; 'Z'
    ble     r1, r2, .vpn_is_alpha
    move.q  r2, #0x61            ; 'a'
    blt     r1, r2, .vpn_check_digit
    move.q  r2, #0x7A            ; 'z'
    ble     r1, r2, .vpn_is_alpha
    bra     .vpn_check_digit

.vpn_is_alpha:
    ; Convert to uppercase
    move.q  r2, #0x61
    blt     r1, r2, .vpn_store_char
    sub.q   r1, r1, #0x20

.vpn_store_char:
    move.q  r2, #4
    bge     r15, r2, .vpn_skip_char
    lsl.l   r14, r14, #8
    or.l    r14, r14, r1
    add.q   r15, r15, #1

.vpn_skip_char:
    add.q   r17, r17, #1
    bra     .vpn_collect

.vpn_check_digit:
    move.q  r2, #0x30            ; '0'
    blt     r1, r2, .vpn_done
    move.q  r2, #0x39            ; '9'
    bgt     r1, r2, .vpn_done
    beqz    r15, .vpn_done       ; can't start with digit
    bra     .vpn_store_char

.vpn_done:
    rts

; ============================================================================
; var_lookup - Find or create a variable, return pointer to value slot
; ============================================================================
; Handles both simple variables and array elements.
; If the name is followed by '(', treats as array reference.
;
; Input:  R17 = points to variable name in token stream
; Output: R8  = pointer to 4-byte value slot (or array element)
;         R17 = advanced past variable name (and subscript)
; Clobbers: R1-R15

var_lookup:
    push    r14
    push    r15

    ; Parse variable name
    jsr     var_parse_name       ; R14 = name tag, R15 = count, R17 advanced
    beqz    r15, .vl_not_found_empty

    ; Check for '$' → string variable
    load.b  r1, (r17)
    move.q  r2, #0x24            ; '$'
    beq     r1, r2, .vl_string_var

    ; Check for array subscript '('
    move.q  r2, #0x28            ; '('
    beq     r1, r2, .vl_array_ref

    ; --- Simple numeric variable: check 4-slot cache first ---
    la      r5, var_cache
    load.l  r6, (r5)             ; slot 0 name_tag
    beq     r6, r14, .vl_cache_hit_0
    load.l  r6, 8(r5)            ; slot 1 name_tag
    beq     r6, r14, .vl_cache_hit_1
    load.l  r6, 16(r5)           ; slot 2 name_tag
    beq     r6, r14, .vl_cache_hit_2
    load.l  r6, 24(r5)           ; slot 3 name_tag
    beq     r6, r14, .vl_cache_hit_3

    ; Cache miss: linear scan
    add.q   r1, r16, #ST_VAR_START
    load.l  r2, (r1)            ; R2 = var_start
    add.q   r1, r16, #ST_VAR_END
    load.l  r3, (r1)            ; R3 = var_end

.vl_search:
    bge     r2, r3, .vl_create
    load.l  r4, (r2)
    beq     r4, r14, .vl_found
    add.q   r2, r2, #8
    bra     .vl_search

.vl_found:
    add.q   r8, r2, #4
    ; Update cache: shift slots 0→1, 1→2, 2→3, insert at 0
    load.q  r1, 16(r5)          ; old slot 2
    store.q r1, 24(r5)          ; → slot 3
    load.q  r1, 8(r5)           ; old slot 1
    store.q r1, 16(r5)          ; → slot 2
    load.q  r1, (r5)            ; old slot 0
    store.q r1, 8(r5)           ; → slot 1
    store.l r14, (r5)           ; new slot 0 name_tag
    store.l r8, 4(r5)           ; new slot 0 var_ptr
    move.q  r9, r0               ; R9 = 0 (numeric)
    pop     r15
    pop     r14
    rts

.vl_create:
    store.l r14, (r3)            ; name tag
    add.q   r1, r3, #4
    store.l r0, (r1)             ; value = 0
    move.q  r8, r1
    ; Update cache: shift and insert at slot 0
    load.q  r2, 16(r5)
    store.q r2, 24(r5)
    load.q  r2, 8(r5)
    store.q r2, 16(r5)
    load.q  r2, (r5)
    store.q r2, 8(r5)
    store.l r14, (r5)
    store.l r8, 4(r5)
    move.q  r9, r0               ; R9 = 0 (numeric)
    add.q   r3, r3, #8
    add.q   r1, r16, #ST_VAR_END
    store.l r3, (r1)
    pop     r15
    pop     r14
    rts

.vl_cache_hit_0:
    load.l  r8, 4(r5)           ; var_ptr from slot 0
    move.q  r9, r0
    pop     r15
    pop     r14
    rts
.vl_cache_hit_1:
    load.l  r8, 12(r5)          ; var_ptr from slot 1
    ; Promote to slot 0: swap slots 0 and 1
    load.q  r1, (r5)            ; old slot 0
    load.q  r2, 8(r5)           ; old slot 1 (the hit)
    store.q r2, (r5)            ; → slot 0
    store.q r1, 8(r5)           ; → slot 1
    move.q  r9, r0
    pop     r15
    pop     r14
    rts
.vl_cache_hit_2:
    load.l  r8, 20(r5)          ; var_ptr from slot 2
    ; Promote to slot 0: save hit, shift 0→1, 1→2, put hit at 0
    load.q  r2, 16(r5)          ; save hit (slot 2) in r2
    load.q  r1, 8(r5)           ; old slot 1
    store.q r1, 16(r5)          ; → slot 2
    load.q  r1, (r5)            ; old slot 0
    store.q r1, 8(r5)           ; → slot 1
    store.q r2, (r5)            ; hit → slot 0
    move.q  r9, r0
    pop     r15
    pop     r14
    rts
.vl_cache_hit_3:
    load.l  r8, 28(r5)          ; var_ptr from slot 3
    ; Promote to slot 0: shift 0→1, 1→2, 2→3, put hit at 0
    load.q  r1, 16(r5)          ; old slot 2
    store.q r1, 24(r5)          ; → slot 3
    load.q  r1, 8(r5)           ; old slot 1
    store.q r1, 16(r5)          ; → slot 2
    load.q  r1, (r5)            ; old slot 0
    store.q r1, 8(r5)           ; → slot 1
    store.l r14, (r5)           ; hit name_tag → slot 0
    store.l r8, 4(r5)           ; hit var_ptr → slot 0
    move.q  r9, r0
    pop     r15
    pop     r14
    rts

.vl_not_found_empty:
    move.q  r8, r0
    move.q  r9, r0               ; R9 = 0
    pop     r15
    pop     r14
    rts

; --- String variable: name followed by '$' ---
.vl_string_var:
    add.q   r17, r17, #1         ; consume '$'
    jsr     svar_lookup          ; R8 = pointer to str_ptr slot
    move.q  r9, #1               ; R9 = 1 (string)
    pop     r15
    pop     r14
    rts

; --- Array reference: name followed by '(' ---
.vl_array_ref:
    add.q   r17, r17, #1        ; skip '('

    ; Save name tag, evaluate first index
    push    r14
    jsr     expr_or              ; R8 = first index (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer index
    move.q  r15, r8              ; R15 = first index

    ; Check for comma (2D array)
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .vl_arr_2d

    ; 1D reference — skip ')'
    load.b  r1, (r17)
    move.q  r2, #0x29            ; ')'
    bne     r1, r2, .vl_arr_skip_close1
    add.q   r17, r17, #1
.vl_arr_skip_close1:

    pop     r14                  ; restore name tag
    ; R15 = index, R14 = name tag
    ; Search array table
    push    r17                  ; save text ptr
    jsr     arr_find             ; R8 = array base (or 0)
    pop     r17
    bnez    r8, .vl_arr_elem_1d

    ; Not found — auto-DIM with 11 elements
    push    r17
    move.q  r8, #11              ; total elements
    move.q  r9, r0               ; dim2 = 0 (1D)
    jsr     arr_create           ; R8 = base of new array
    pop     r17

.vl_arr_elem_1d:
    ; R8 = array base, R15 = index
    ; Element address = base + 12 + index * 4
    lsl.l   r1, r15, #2
    add.q   r8, r8, #12
    add.q   r8, r8, r1
    move.q  r9, r0               ; R9 = 0 (numeric array)
    pop     r15
    pop     r14
    rts

.vl_arr_2d:
    ; 2D: skip comma, evaluate second index
    add.q   r17, r17, #1        ; skip ','
    push    r15                  ; save first index
    jsr     expr_or              ; R8 = second index (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer index
    move.q  r1, r8               ; R1 = second index
    pop     r15                  ; R15 = first index

    ; Skip ')'
    load.b  r2, (r17)
    move.q  r3, #0x29            ; ')'
    bne     r2, r3, .vl_arr_skip_close2
    add.q   r17, r17, #1
.vl_arr_skip_close2:

    pop     r14                  ; restore name tag (pushed before .vl_array_ref)
    ; R15 = first index, R1 = second index, R14 = name tag
    push    r1                   ; save second index
    push    r15                  ; save first index
    push    r17                  ; save text ptr
    jsr     arr_find             ; R8 = array base (or 0)
    pop     r17
    pop     r15                  ; first index
    pop     r1                   ; second index
    bnez    r8, .vl_arr_elem_2d

    ; Not found — auto-DIM with 11*11=121 elements, dim2=11
    push    r1
    push    r15
    push    r17
    move.q  r8, #121             ; total elements (11*11)
    move.q  r9, #11              ; dim2 = 11
    jsr     arr_create           ; R8 = base of new array
    pop     r17
    pop     r15
    pop     r1

.vl_arr_elem_2d:
    ; R8 = array base, R15 = first index, R1 = second index
    ; dim2 is stored at base+8
    add.q   r2, r8, #8
    load.l  r2, (r2)             ; R2 = dim2
    mulu.l  r3, r15, r2          ; first_index * dim2
    add.l   r3, r3, r1           ; + second_index
    lsl.l   r3, r3, #2           ; * 4
    add.q   r8, r8, #12
    add.q   r8, r8, r3
    move.q  r9, r0               ; R9 = 0 (numeric array)
    pop     r15
    pop     r14
    rts

; ============================================================================
; arr_find - Search array table for a name
; ============================================================================
; Input:  R14 = packed name tag
;         R16 = state base
; Output: R8  = pointer to array entry (0 if not found)
; Clobbers: R1-R5

arr_find:
    add.q   r1, r16, #ST_ARRAY_START
    load.l  r2, (r1)             ; R2 = scan pointer
    add.q   r1, r16, #ST_ARRAY_END
    load.l  r3, (r1)             ; R3 = end

.af_loop:
    bge     r2, r3, .af_not_found
    load.l  r4, (r2)             ; name tag at this entry
    beq     r4, r14, .af_found
    ; Skip past this entry: header(12) + total_elems*4
    add.q   r5, r2, #4
    load.l  r5, (r5)             ; total_elems
    lsl.l   r5, r5, #2           ; *4
    add.q   r2, r2, #12          ; header
    add.q   r2, r2, r5           ; data
    bra     .af_loop

.af_found:
    move.q  r8, r2
    rts

.af_not_found:
    move.q  r8, r0
    rts

; ============================================================================
; arr_create - Create a new array entry
; ============================================================================
; Input:  R14 = packed name tag
;         R8  = total element count
;         R9  = dim2 size (0 for 1D)
;         R16 = state base
; Output: R8  = pointer to the new array entry
; Clobbers: R1-R5

arr_create:
    add.q   r1, r16, #ST_ARRAY_END
    load.l  r2, (r1)             ; R2 = current array_end (insertion point)
    move.q  r3, r2               ; R3 = base of new entry (return value)

    ; Write header
    store.l r14, (r2)            ; name tag
    add.q   r2, r2, #4
    store.l r8, (r2)             ; total elements
    add.q   r2, r2, #4
    store.l r9, (r2)             ; dim2
    add.q   r2, r2, #4

    ; Zero out element data
    move.q  r4, r8               ; counter = total_elems
.ac_zero:
    beqz    r4, .ac_done
    store.l r0, (r2)
    add.q   r2, r2, #4
    sub.q   r4, r4, #1
    bra     .ac_zero

.ac_done:
    ; Update array_end
    store.l r2, (r1)
    ; Return base pointer
    move.q  r8, r3
    rts

; ============================================================================
; arr_dim - DIM statement handler (called from exec)
; ============================================================================
; Input:  R17 = points past DIM token (at variable name)
;         R16 = state base
; Output: R17 = advanced past the declaration
; Clobbers: R1-R15

arr_dim:
    push    r14
    push    r15

.ad_next_var:
    ; Skip spaces
    load.b  r1, (r17)
    move.q  r2, #0x20
    beq     r1, r2, .ad_skip_space
    bra     .ad_parse_name
.ad_skip_space:
    add.q   r17, r17, #1
    bra     .ad_next_var

.ad_parse_name:
    jsr     var_parse_name       ; R14 = name tag, R15 = count
    beqz    r15, .ad_done

    ; Expect '('
    load.b  r1, (r17)
    move.q  r2, #0x28            ; '('
    bne     r1, r2, .ad_done
    add.q   r17, r17, #1         ; skip '('

    ; Evaluate first dimension size
    push    r14                  ; save name tag
    jsr     expr_or              ; R8 = dim1 size (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer
    add.q   r15, r8, #1          ; R15 = dim1 elements (size+1, 0-based)

    ; Check for comma (2D)
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .ad_2d

    ; 1D: skip ')'
    load.b  r1, (r17)
    move.q  r2, #0x29
    bne     r1, r2, .ad_1d_no_close
    add.q   r17, r17, #1
.ad_1d_no_close:
    pop     r14                  ; restore name tag
    ; Create 1D array: total=R15, dim2=0
    push    r17
    move.q  r8, r15              ; total elements
    move.q  r9, r0               ; dim2 = 0
    jsr     arr_create
    pop     r17
    bra     .ad_check_comma

.ad_2d:
    add.q   r17, r17, #1         ; skip ','
    push    r15                  ; save dim1 elements
    jsr     expr_or              ; R8 = dim2 size (FP32)
    jsr     fp_int
    jsr     fp_fix               ; R8 = integer
    add.q   r1, r8, #1           ; R1 = dim2 elements (size+1)
    pop     r15                  ; R15 = dim1 elements

    ; Skip ')'
    load.b  r2, (r17)
    move.q  r3, #0x29
    bne     r2, r3, .ad_2d_no_close
    add.q   r17, r17, #1
.ad_2d_no_close:
    pop     r14                  ; restore name tag
    ; Create 2D array: total=dim1*dim2, dim2=dim2_elems
    push    r17
    push    r1                   ; save dim2 elements
    mulu.l  r8, r15, r1          ; total = dim1 * dim2
    pop     r9                   ; R9 = dim2 elements
    jsr     arr_create
    pop     r17

.ad_check_comma:
    ; Check for ',' (multiple DIM declarations)
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    bne     r1, r2, .ad_done
    add.q   r17, r17, #1
    bra     .ad_next_var

.ad_done:
    pop     r15
    pop     r14
    rts

; ============================================================================
; svar_lookup - Find or create a string variable
; ============================================================================
; Input:  R14 = packed name tag
;         R16 = state base
; Output: R8  = pointer to str_ptr slot (4 bytes)
; Clobbers: R1-R5

svar_lookup:
    add.q   r1, r16, #ST_SVAR_START
    load.l  r2, (r1)             ; R2 = scan pointer
    add.q   r1, r16, #ST_SVAR_END
    load.l  r3, (r1)             ; R3 = end

.svl_search:
    bge     r2, r3, .svl_create
    load.l  r4, (r2)
    beq     r4, r14, .svl_found
    add.q   r2, r2, #8           ; next entry (name + ptr)
    bra     .svl_search

.svl_found:
    add.q   r8, r2, #4           ; pointer to str_ptr field
    rts

.svl_create:
    ; Create new entry at end
    store.l r14, (r3)            ; name tag
    add.q   r1, r3, #4
    store.l r0, (r1)             ; str_ptr = 0 (uninitialized)
    move.q  r8, r1               ; return pointer to str_ptr slot
    add.q   r3, r3, #8
    add.q   r1, r16, #ST_SVAR_END
    store.l r3, (r1)
    rts

; ============================================================================
; svar_read - Look up string variable and return pointer to string data
; ============================================================================
; Input:  R17 = points to variable name (will parse name + skip '$')
; Output: R8  = pointer to null-terminated string data
;         R17 = advanced past name and '$'
; Clobbers: R1-R15

svar_read:
    push    r14
    push    r15
    jsr     var_parse_name       ; R14 = name, R15 = count
    beqz    r15, .svr_empty
    ; Skip '$'
    load.b  r1, (r17)
    move.q  r2, #0x24            ; '$'
    bne     r1, r2, .svr_empty
    add.q   r17, r17, #1
    jsr     svar_lookup          ; R8 = pointer to str_ptr slot
    load.l  r8, (r8)             ; R8 = pointer to string data
    bnez    r8, .svr_done
    ; Null pointer → return empty string
    la      r8, .svr_empty_str
.svr_done:
    pop     r15
    pop     r14
    rts
.svr_empty:
    la      r8, .svr_empty_str
    pop     r15
    pop     r14
    rts
.svr_empty_str:
    dc.b    0
    align 8

; ============================================================================
; var_read - Look up variable and return its FP32 value
; ============================================================================
; Input:  R17 = points to variable name
; Output: R8  = FP32 value of the variable
;         R17 = advanced past variable name
; Clobbers: R1-R15

var_read:
    jsr     var_lookup           ; R8 = pointer to value slot
    beqz    r8, .vr_zero
    load.l  r8, (r8)            ; read FP32 value
    rts
.vr_zero:
    move.q  r8, r0               ; undefined → 0
    rts

; ============================================================================
; Variable cache: 4 slots × [name_tag(4) + var_ptr(4)] = 32 bytes
; ============================================================================
var_cache:
    dc.q    0                    ; slot 0: name_tag(4) + var_ptr(4)
    dc.q    0                    ; slot 1
    dc.q    0                    ; slot 2
    dc.q    0                    ; slot 3

; ============================================================================
; EOF
; ============================================================================
