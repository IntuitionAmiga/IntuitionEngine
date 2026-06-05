; ehbasic_vars.inc - EhBASIC IE64 Variable Storage
;
; Manages BASIC variables: simple numeric scalars and numeric arrays.
;
; Simple variable storage format (16 bytes each):
;   +0 (4 bytes): Name tag (packed uppercase, first char in MSB)
;   +4 (4 bytes): Payload tag (0 = FP64, 1 = qword integer)
;   +8 (8 bytes): Value payload (FP64 bits in low 32 bits for legacy numeric maths)
;
; Array storage format (variable-length):
;   +0  (4 bytes): Name tag
;   +4  (4 bytes): Total element count
;   +8  (4 bytes): Dim2 size (0 = 1D)
;   +12 (N*4 bytes): Element data (FP64)
;
; Register conventions:
;   R16 = interpreter state base (BASIC_STATE)
;   R17 = execution pointer (text cursor)
;
; Calling conventions:
;   var_init:    Initialise variable storage. R16 must be set.
;   var_lookup:  R17 = points to variable name in token stream.
;                Returns: R8 = pointer to numeric payload slot (or array element).
;                         R17 = advanced past variable name (and subscript if array).
;                Clobbers: R1-R15
;   var_read:    Same as var_lookup but returns FP64 value in R8.
;   arr_dim:     DIM handler: R17 = points past DIM token.
;                Creates array with specified dimensions.
;                Clobbers: R1-R15
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; var_init - Initialise variable and array storage
; ============================================================================

var_init:
    ; Numeric scalar variables use pinned dynamic segments allocated lazily from
    ; the internal BASIC arena. Empty table: ST_VAR_START == ST_VAR_END == 0.
    move.q  r1, r0
    add.q   r2, r16, #ST_VAR_START
    store.q r1, (r2)
    add.q   r2, r16, #ST_VAR_END
    store.q r1, (r2)
    ; String variables
    move.q  r1, r0
    add.q   r2, r16, #ST_SVAR_START
    store.q r1, (r2)
    add.q   r2, r16, #ST_SVAR_END
    store.q r1, (r2)
    ; Arrays
    move.q  r1, r0
    add.q   r2, r16, #ST_ARRAY_START
    store.q r1, (r2)
    add.q   r2, r16, #ST_ARRAY_END
    store.q r1, (r2)
    ; Internal string data now uses the common internal arena published in
    ; ST_HEAP_BOTTOM/ST_HEAP_TOP. var_init must not reset the high limit.
    ; Exported string low32 cursor for SADD/hardware-facing string pointers.
    move.q  r1, #BASIC_EXPORT_BASE
    add.q   r2, r16, #ST_EXPORT_CURSOR
    store.q r1, (r2)
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
    bge     r15, r2, .vpn_mix_extra_char
    lsl.l   r14, r14, #8
    or.l    r14, r14, r1
    add.q   r15, r15, #1
    bra     .vpn_advance

.vpn_mix_extra_char:
    ; Preserve the existing 4-character packed tag for short names. For longer
    ; names, mix each extra uppercase character into the 32-bit tag so names
    ; such as COUNT and COUNTER do not alias.
    mulu.l  r14, r14, #33
    add.l   r14, r14, r1

.vpn_advance:
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
; var_lookup - Find or create a variable, return pointer to payload slot
; ============================================================================
; Handles both simple variables and array elements.
; If the name is followed by '(', treats as array reference.
;
; Input:  R17 = points to variable name in token stream
; Output: R8  = pointer to value payload slot (or array element)
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

    ; Cache miss: segment-chain lookup/create.
    jsr     var_lookup_tag
    la      r5, var_cache
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

.vl_search:
    bge     r2, r3, .vl_create
    load.l  r4, (r2)
    beq     r4, r14, .vl_found
    add.q   r2, r2, #24
    bra     .vl_search

.vl_found:
    add.q   r8, r2, #16
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
    store.l r0, (r1)             ; reserved
    add.q   r1, r3, #8
    move.q  r2, #VAL_F64
    store.l r2, (r1)             ; public value cell tag
    add.q   r1, r3, #12
    store.l r0, (r1)             ; public value cell reserved
    add.q   r1, r3, #16
    store.q r0, (r1)             ; value payload = 0
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
    add.q   r3, r3, #24
    add.q   r1, r16, #ST_VAR_END
    store.q r3, (r1)
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
    push    r14                  ; preserve array name while expressions run
    move.q  r10, r0              ; index count
    la      r11, arr_index_buf

.vl_arr_parse_index:
    move.q  r1, #8
    bge     r10, r1, .vl_array_fc_keep_name
    push    r10
    push    r11
    jsr     expr_or
    jsr     fp_int
    jsr     fp_fix
    pop     r11
    pop     r10
    lsl.l   r12, r10, #2
    add.q   r13, r11, r12
    store.l r8, (r13)
    add.q   r10, r10, #1
    load.b  r1, (r17)
    move.q  r2, #0x2C            ; ','
    beq     r1, r2, .vl_arr_more_index
    move.q  r2, #0x29            ; ')'
    beq     r1, r2, .vl_arr_indices_done
    bra     .vl_array_fc_keep_name
.vl_arr_more_index:
    add.q   r17, r17, #1
    bra     .vl_arr_parse_index
.vl_arr_indices_done:
    add.q   r17, r17, #1
    pop     r14

    push    r10
    push    r17
    jsr     arr_find
    pop     r17
    pop     r10
    bnez    r8, .vl_arr_have_base

    ; Auto-DIM missing arrays with 11 elements per referenced dimension.
    la      r11, arr_dim_buf
    move.q  r12, r0
    move.q  r8, #1
.vl_arr_auto_dim_loop:
    bge     r12, r10, .vl_arr_auto_create
    lsl.l   r13, r12, #2
    add.q   r13, r11, r13
    move.q  r1, #11
    store.l r1, (r13)
    mulu.l  r8, r8, #11
    add.q   r12, r12, #1
    bra     .vl_arr_auto_dim_loop
.vl_arr_auto_create:
    move.q  r9, r10
    push    r10
    push    r17
    jsr     arr_create
    pop     r17
    pop     r10

.vl_arr_have_base:
    ; Header: next.q, name.l, dims.l, total.q, dim extents.q..., cells...
    add.q   r1, r8, #12
    load.l  r1, (r1)             ; stored dim_count
    bne     r1, r10, .vl_array_fc
    add.q   r11, r8, #24         ; dims pointer
    lsl.l   r12, r10, #3
    add.q   r13, r11, r12        ; data pointer
    move.q  r3, r0               ; row-major offset
    move.q  r4, r0               ; loop index
    la      r5, arr_index_buf
.vl_arr_offset_loop:
    bge     r4, r10, .vl_arr_offset_done
    lsl.l   r6, r4, #3
    add.q   r7, r11, r6
    load.q  r7, (r7)             ; dim size
    lsl.l   r6, r4, #2
    add.q   r2, r5, r6
    load.l  r2, (r2)             ; index
    bgez    r2, .vl_arr_idx_nonneg
    bra     .vl_array_fc
.vl_arr_idx_nonneg:
    bge     r2, r7, .vl_array_fc
    mulu.l  r3, r3, r7
    add.l   r3, r3, r2
    add.q   r4, r4, #1
    bra     .vl_arr_offset_loop
.vl_arr_offset_done:
    lsl.q   r3, r3, #4
    add.q   r8, r13, r3
    add.q   r8, r8, #8           ; return payload slot inside public cell
    move.q  r9, r0
    pop     r15
    pop     r14
    rts

.vl_array_fc_keep_name:
    pop     r14

.vl_array_fc:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    move.q  r8, r0
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
    load.q  r2, (r1)             ; R2 = scan pointer

.af_loop:
    beqz    r2, .af_not_found
    load.l  r4, 8(r2)            ; name tag at this record
    beq     r4, r14, .af_found
    load.q  r2, (r2)             ; next.q
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
;         R9  = dimension count
;         arr_dim_buf = dimension element counts
;         R16 = state base
; Output: R8  = pointer to the new array entry
; Clobbers: R1-R5

arr_create:
    move.q  r10, r8              ; total elements
    move.q  r11, r9              ; dimension count

    ; bytes = align8(24 + dims*8 + total*16)
    move.q  r12, r11
    lsl.q   r12, r12, #3
    add.q   r12, r12, #24
    move.q  r13, r10
    lsl.q   r13, r13, #4
    add.q   r12, r12, r13
    add.q   r12, r12, #7
    and.q   r12, r12, #0xFFFFFFFFFFFFFFF8

    add.q   r1, r16, #ST_HEAP_BOTTOM
    load.q  r3, (r1)             ; R3 = new record base
    move.q  r2, r3
    add.q   r2, r2, r12          ; new heap bottom
    add.q   r4, r16, #ST_FILE_BRIDGE_NEXT
    load.q  r5, (r4)
    bnez    r5, .ac_have_limit
    add.q   r4, r16, #ST_HEAP_TOP
    load.q  r5, (r4)
    bnez    r5, .ac_have_limit
    move.q  r5, #0x02000000
.ac_have_limit:
    bgt     r2, r5, .ac_oom
    store.q r2, (r1)

    ; Write header
    store.q r0, (r3)             ; next.q
    store.l r14, 8(r3)           ; name tag
    store.l r11, 12(r3)          ; dim_count
    store.q r10, 16(r3)          ; total elements
    add.q   r2, r3, #24          ; dimension cursor

    ; Copy dimension sizes.
    move.q  r5, r0
    la      r6, arr_dim_buf
.ac_dim_loop:
    bge     r5, r11, .ac_dim_done
    lsl.l   r7, r5, #2
    add.q   r1, r6, r7
    load.l  r1, (r1)
    store.q r1, (r2)
    add.q   r2, r2, #8
    add.q   r5, r5, #1
    bra     .ac_dim_loop
.ac_dim_done:

    ; Initialise public value cells to VAL_F64, 0.
    move.q  r4, r10              ; counter = total_elems
.ac_zero:
    beqz    r4, .ac_done
    move.q  r1, #VAL_F64
    store.l r1, (r2)
    store.l r0, 4(r2)
    store.q r0, 8(r2)
    add.q   r2, r2, #16
    sub.q   r4, r4, #1
    bra     .ac_zero

.ac_done:
    ; Link into the array record chain.
    add.q   r1, r16, #ST_ARRAY_END
    load.q  r2, (r1)
    beqz    r2, .ac_first
    store.q r3, (r2)             ; old tail next = new record
    bra     .ac_store_tail
.ac_first:
    add.q   r1, r16, #ST_ARRAY_START
    store.q r3, (r1)
.ac_store_tail:
    add.q   r1, r16, #ST_ARRAY_END
    store.q r3, (r1)
    move.q  r8, r3
    rts
.ac_oom:
    move.q  r8, #ERR_OOM
    la      r9, err_msg_oom
    jsr     raise_error
    move.q  r8, r0
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

    push    r14                  ; save name tag
    move.q  r10, r0              ; dim_count
    move.q  r11, #1              ; total elements
    la      r12, arr_dim_buf
.ad_dim_parse_loop:
    move.q  r1, #8
    bge     r10, r1, .ad_dim_too_many
    push    r10
    push    r11
    push    r12
    jsr     expr_or              ; R8 = dimension upper bound
    jsr     fp_int
    jsr     fp_fix
    pop     r12
    pop     r11
    pop     r10
    bgez    r8, .ad_dim_nonneg
    pop     r14
    bra     .ad_fc
.ad_dim_nonneg:
    add.q   r8, r8, #1
    lsl.l   r13, r10, #2
    add.q   r1, r12, r13
    store.l r8, (r1)
    mulu.l  r11, r11, r8
    add.q   r10, r10, #1
    load.b  r1, (r17)
    move.q  r2, #0x2C
    beq     r1, r2, .ad_dim_more
    move.q  r2, #0x29
    beq     r1, r2, .ad_dim_done
    pop     r14
    bra     .ad_fc
.ad_dim_more:
    add.q   r17, r17, #1
    bra     .ad_dim_parse_loop
.ad_dim_too_many:
    pop     r14
    bra     .ad_fc
.ad_dim_done:
    add.q   r17, r17, #1
    pop     r14
    push    r17
    jsr     arr_find
    bnez    r8, .ad_redim_from_push
    move.q  r8, r11              ; total elements
    move.q  r9, r10              ; dimension count
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
.ad_redim_from_push:
    pop     r17
.ad_redim:
    move.q  r8, #ERR_REDIM
    la      r9, err_msg_redim
    jsr     raise_error
    bra     .ad_done
.ad_fc:
    move.q  r8, #ERR_FC
    la      r9, err_msg_fc
    jsr     raise_error
    bra     .ad_done

; ============================================================================
; svar_lookup - Find or create a string variable
; ============================================================================
; Input:  R14 = packed name tag
;         R16 = state base
; Output: R8  = pointer to internal_ptr slot
; Clobbers: R1-R13

svar_lookup:
    add.q   r1, r16, #ST_SVAR_START
    load.q  r2, (r1)             ; R2 = segment pointer

.svl_seg_loop:
    beqz    r2, .svl_create
    add.q   r3, r2, #8
    load.l  r4, (r3)             ; records_used
    add.q   r5, r2, #16          ; record cursor
    move.q  r6, r0
.svl_record_loop:
    bge     r6, r4, .svl_next_segment
    load.l  r7, (r5)
    beq     r7, r14, .svl_found
    add.q   r5, r5, #32
    add.q   r6, r6, #1
    bra     .svl_record_loop
.svl_next_segment:
    load.q  r2, (r2)
    bra     .svl_seg_loop

.svl_found:
    add.q   r8, r5, #8           ; pointer to internal_ptr field
    rts

.svl_create:
    add.q   r1, r16, #ST_SVAR_END
    load.q  r2, (r1)             ; tail segment
    beqz    r2, .svl_alloc_first_segment

    add.q   r3, r2, #8
    load.l  r4, (r3)             ; records_used
    add.q   r5, r2, #12
    load.l  r6, (r5)             ; records_cap
    blt     r4, r6, .svl_have_segment

    push    r2
    jsr     svar_alloc_segment
    pop     r2
    beqz    r8, .svl_oom
    store.q r8, (r2)
    add.q   r1, r16, #ST_SVAR_END
    store.q r8, (r1)
    move.q  r2, r8
    bra     .svl_have_segment_reload

.svl_alloc_first_segment:
    jsr     svar_alloc_segment
    beqz    r8, .svl_oom
    add.q   r1, r16, #ST_SVAR_START
    store.q r8, (r1)
    add.q   r1, r16, #ST_SVAR_END
    store.q r8, (r1)
    move.q  r2, r8

.svl_have_segment_reload:
    add.q   r3, r2, #8
    load.l  r4, (r3)
.svl_have_segment:
    move.q  r5, r4
    lsl.q   r5, r5, #5           ; *32
    add.q   r5, r5, r2
    add.q   r5, r5, #16          ; new record
    store.l r14, (r5)
    store.l r0, 4(r5)
    store.q r0, 8(r5)            ; internal_ptr
    store.q r0, 16(r5)           ; length
    store.l r0, 24(r5)           ; export_ptr
    store.l r0, 28(r5)           ; export_cap
    add.q   r8, r5, #8
    add.q   r4, r4, #1
    store.l r4, (r3)
    rts
.svl_oom:
    move.q  r8, r0
    rts

; ============================================================================
; svar_alloc_segment - Allocate one string-variable segment
; ============================================================================
; Segment layout: next_segment.q, records_used.l, records_cap.l,
; then 32 records of 32 bytes each. Total size = 1040 bytes.
; Output: R8 = segment pointer, or 0 after ?OM ERROR.
; Clobbers: R1-R6

svar_alloc_segment:
    add.q   r1, r16, #ST_HEAP_BOTTOM
    load.q  r8, (r1)
    move.q  r2, r8
    add.q   r2, r2, #1040
    add.q   r3, r16, #ST_FILE_BRIDGE_NEXT
    load.q  r4, (r3)
    bnez    r4, .sas_have_limit
    add.q   r3, r16, #ST_HEAP_TOP
    load.q  r4, (r3)
    bnez    r4, .sas_have_limit
    move.q  r4, #0x02000000
.sas_have_limit:
    bgt     r2, r4, .sas_oom
    store.q r2, (r1)
    store.q r0, (r8)
    add.q   r5, r8, #8
    store.l r0, (r5)
    add.q   r5, r8, #12
    move.q  r6, #32
    store.l r6, (r5)
    rts
.sas_oom:
    move.q  r8, #ERR_OOM
    la      r9, err_msg_oom
    jsr     raise_error
    move.q  r8, r0
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
; var_read - Look up variable and return its tagged numeric value
; ============================================================================
; Input:  R17 = points to variable name
; Output: R8  = value payload of the variable
;         R9  = VAL_F64 or VAL_I64
;         R17 = advanced past variable name
; Clobbers: R1-R15

var_read:
    jsr     var_lookup           ; R8 = pointer to value slot
    bnez    r28, .vr_done
    beqz    r8, .vr_zero
    move.q  r1, r8
    load.l  r3, -8(r1)
    move.q  r4, #VAL_I64
    beq     r3, r4, .vr_load_i64
    load.q  r8, (r1)
    move.q  r9, #VAL_F64
    rts
.vr_load_i64:
    load.q  r8, (r1)
    move.q  r9, #VAL_I64
.vr_done:
    rts
.vr_zero:
    move.q  r8, r0               ; undefined → 0
    move.q  r9, #VAL_F64
    rts

; ============================================================================
; var_lookup_tag - Find or create a numeric variable by pre-parsed name tag
; ============================================================================
; Input:  R14 = packed name tag
; Output: R8  = pointer to value payload slot
; Clobbers: R1-R13

var_lookup_tag:
    add.q   r1, r16, #ST_VAR_START
    load.q  r2, (r1)             ; R2 = segment pointer
.vlt_seg_loop:
    beqz    r2, .vlt_create
    add.q   r3, r2, #8
    load.l  r4, (r3)             ; records_used
    add.q   r5, r2, #16          ; record cursor
    move.q  r6, r0               ; index
.vlt_record_loop:
    bge     r6, r4, .vlt_next_segment
    load.l  r7, (r5)             ; name_tag
    beq     r7, r14, .vlt_found
    add.q   r5, r5, #24
    add.q   r6, r6, #1
    bra     .vlt_record_loop
.vlt_next_segment:
    load.q  r2, (r2)             ; next_segment.q
    bra     .vlt_seg_loop
.vlt_found:
    add.q   r8, r5, #16
    rts
.vlt_create:
    add.q   r1, r16, #ST_VAR_END
    load.q  r2, (r1)             ; R2 = tail segment, or 0
    beqz    r2, .vlt_alloc_first_segment

    add.q   r3, r2, #8
    load.l  r4, (r3)             ; records_used
    add.q   r5, r2, #12
    load.l  r6, (r5)             ; records_cap
    blt     r4, r6, .vlt_have_segment

    push    r2                   ; old tail
    jsr     var_alloc_segment
    pop     r2
    beqz    r8, .vlt_oom
    store.q r8, (r2)             ; old_tail.next_segment = new segment
    add.q   r1, r16, #ST_VAR_END
    store.q r8, (r1)
    move.q  r2, r8
    bra     .vlt_have_segment_reload

.vlt_alloc_first_segment:
    jsr     var_alloc_segment
    beqz    r8, .vlt_oom
    add.q   r1, r16, #ST_VAR_START
    store.q r8, (r1)
    add.q   r1, r16, #ST_VAR_END
    store.q r8, (r1)
    move.q  r2, r8

.vlt_have_segment_reload:
    add.q   r3, r2, #8
    load.l  r4, (r3)             ; records_used
.vlt_have_segment:
    ; R2 = segment, R3 = &records_used, R4 = used count.
    move.q  r5, r4
    mulu.q  r5, r5, #24
    add.q   r5, r5, r2
    add.q   r5, r5, #16          ; R5 = new record
    store.l r14, (r5)
    add.q   r1, r5, #4
    store.l r0, (r1)
    add.q   r1, r5, #8
    move.q  r2, #VAL_F64
    store.l r2, (r1)
    add.q   r1, r5, #12
    store.l r0, (r1)
    add.q   r8, r5, #16
    store.q r0, (r8)
    add.q   r4, r4, #1
    store.l r4, (r3)
    rts
.vlt_oom:
    move.q  r8, r0
    rts

; ============================================================================
; var_alloc_segment - Allocate one numeric variable segment from ST_HEAP_BOTTOM
; ============================================================================
; Segment layout: next_segment.q, records_used.l, records_cap.l,
; then 64 records of 24 bytes each. Total size = 1552 bytes.
; Output: R8 = segment pointer, or 0 after ?OM ERROR.
; Clobbers: R1-R6

var_alloc_segment:
    add.q   r1, r16, #ST_HEAP_BOTTOM
    load.q  r8, (r1)              ; segment base
    move.q  r2, r8
    add.q   r2, r2, #1552         ; new heap bottom

    add.q   r3, r16, #ST_FILE_BRIDGE_NEXT
    load.q  r4, (r3)
    bnez    r4, .vas_have_limit
    add.q   r3, r16, #ST_HEAP_TOP
    load.q  r4, (r3)
    bnez    r4, .vas_have_limit
    move.q  r4, #0x02000000
.vas_have_limit:
    bgt     r2, r4, .vas_oom
    store.q r2, (r1)

    store.q r0, (r8)              ; next_segment = 0
    add.q   r5, r8, #8
    store.l r0, (r5)              ; records_used = 0
    add.q   r5, r8, #12
    move.q  r6, #64
    store.l r6, (r5)              ; records_cap = 64
    rts
.vas_oom:
    move.q  r8, #ERR_OOM
    la      r9, err_msg_oom
    jsr     raise_error
    move.q  r8, r0
    rts

; ============================================================================
; Variable cache: 4 slots × [name_tag(4) + var_ptr(4)] = 32 bytes
; ============================================================================
arr_dim_buf:
    dc.l    0,0,0,0,0,0,0,0
arr_index_buf:
    dc.l    0,0,0,0,0,0,0,0

var_cache:
    dc.q    0                    ; slot 0: name_tag(4) + var_ptr(4)
    dc.q    0                    ; slot 1
    dc.q    0                    ; slot 2
    dc.q    0                    ; slot 3

; ============================================================================
; EOF
; ============================================================================
