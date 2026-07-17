; ehbasic_compiler.inc - parser, validation and arena for the replacement compiler

COMPILER_ARENA_MAGIC          equ 0x434F4D50
COMPILER_ARENA_HEADER_BYTES   equ 24
COMPILER_ARENA_REGION_BYTES   equ 24
COMPILER_ARENA_REGION_COUNT   equ 7
COMPILER_ARENA_FLOOR          equ 0x01000000
COMPILER_TARGET_ARENA         equ 1
COMPILER_TARGET_STANDALONE    equ 2

; Initialise seven bounded, contiguous regions for IR, symbols, control-flow,
; literals, assembly text, fixups and native output. Each descriptor is
; start/cursor/end. R8=descriptor, R9=programme start, R10=programme end,
; R17=current line. Returns R8=status and R9=diagnostic line.
compiler_arena_init:
    blt     r8, r0, .cai_bad
    blt     r9, r0, .cai_bad
    blt     r10, r0, .cai_bad
    ble     r10, r9, .cai_bad
    move.q  r7, r8
    add.q   r1, r10, #0xFFF
    blt     r1, r10, .cai_bad
    and.q   r2, r1, #0xFFF
    sub.q   r1, r1, r2
    move.q  r3, #COMPILER_ARENA_FLOOR
    bge     r1, r3, .cai_floor_ready
    move.q  r1, r3
.cai_floor_ready:
    mfcr    r4, cr15
    and.q   r2, r4, #0xFFF
    sub.q   r4, r4, r2
    add.q   r2, r16, #ST_CTRL_LOW
    load.q  r3, (r2)
    beqz    r3, .cai_top_ready
    bge     r3, r4, .cai_top_ready
    ble     r3, r1, .cai_bad
    move.q  r4, r3
.cai_top_ready:
    add.q   r2, r7, #(COMPILER_ARENA_HEADER_BYTES + COMPILER_ARENA_REGION_BYTES * COMPILER_ARENA_REGION_COUNT)
    blt     r2, r7, .cai_bad
    bgt     r2, r4, .cai_bad
    ; The descriptor may live in BASIC compiler state, but never in the stored
    ; programme whose bytes must survive every failure path.
    bge     r7, r10, .cai_descriptor_clear
    ble     r2, r9, .cai_descriptor_clear
    bra     .cai_bad
.cai_descriptor_clear:
    sub.q   r5, r4, r1
    lsr.q   r5, r5, #3
    and.q   r2, r5, #0xF
    sub.q   r5, r5, r2
    move.q  r2, #0x1000
    blt     r5, r2, .cai_bad
    move.q  r2, #COMPILER_ARENA_MAGIC
    store.q r2, (r7)
    store.q r1, 8(r7)
    store.q r4, 16(r7)
    add.q   r6, r7, #COMPILER_ARENA_HEADER_BYTES
    move.q  r2, #COMPILER_ARENA_REGION_COUNT
.cai_region:
    store.q r1, (r6)
    store.q r1, 8(r6)
    add.q   r1, r1, r5
    store.q r1, 16(r6)
    add.q   r6, r6, #COMPILER_ARENA_REGION_BYTES
    sub.q   r2, r2, #1
    bnez    r2, .cai_region
    move.q  r8, r0
    move.q  r9, r0
    rts
.cai_bad:
    move.q  r8, #1
    move.q  r9, r17
    rts

; Allocate from one arena region without partially advancing it.
; R8=descriptor, R9=region index, R10=byte count, R17=current line.
; Allocations are 16-byte aligned. Returns R8=status, R9=address, R10=line.
compiler_arena_alloc:
    blt     r9, r0, .caa_fail
    move.q  r1, #COMPILER_ARENA_REGION_COUNT
    bge     r9, r1, .caa_fail
    blt     r10, r0, .caa_fail
    add.q   r1, r10, #15
    blt     r1, r10, .caa_fail
    and.q   r2, r1, #15
    sub.q   r1, r1, r2
    ; region offset = header + index * 24 = header + index*16 + index*8
    lsl.q   r2, r9, #4
    lsl.q   r3, r9, #3
    add.q   r2, r2, r3
    add.q   r2, r2, #COMPILER_ARENA_HEADER_BYTES
    add.q   r2, r8, r2
    load.q  r3, 8(r2)
    load.q  r4, 16(r2)
    add.q   r5, r3, r1
    blt     r5, r3, .caa_fail
    bgt     r5, r4, .caa_fail
    store.q r5, 8(r2)
    move.q  r8, r0
    move.q  r9, r3
    move.q  r10, r0
    rts
.caa_fail:
    move.q  r8, #1
    move.q  r9, r0
    move.q  r10, r17
    rts

; Parse linked stored-programme lines into labelled blocks and token records.
; R8=first line, R9=exclusive programme bound, R10=IR context (reserved).
; Returns R8=status, R9=line, R10=token offset.
compiler_parse_program:
    push    r18
    push    r19
    push    r20
    push    r21
    push    r22
    push    r23
    push    r24
    push    r25
    push    r26
    push    r27
    push    r28
    push    r29
    jsr     compiler_parse_program_inner
    move.q  r1, r8
    move.q  r2, r9
    move.q  r3, r10
    pop     r29
    pop     r28
    pop     r27
    pop     r26
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r21
    pop     r20
    pop     r19
    pop     r18
    move.q  r8, r1
    move.q  r9, r2
    move.q  r10, r3
    rts
compiler_parse_program_inner:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r20, r0
.cpp_line:
    beqz    r18, .cpp_done
    blt     r18, r0, .cpp_malformed
    bge     r18, r19, .cpp_malformed
    add.q   r1, r18, #16
    bgt     r1, r19, .cpp_malformed
    load.q  r21, (r18)
    load.l  r22, 8(r18)
    ble     r22, r20, .cpp_malformed_line
    move.q  r20, r22
    move.q  r9, #IR_OP_BLOCK
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r22
    move.q  r13, r0
    move.q  r14, r22
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpp_ir_error
    add.q   r23, r18, #16
    move.q  r24, r0
    ; Statements with a complete semantic parser never enter the token-record
    ; compatibility path. This keeps unsupported syntax explicit while parser
    ; coverage is migrated construct by construct.
.cpp_statement:
    move.q  r1, r23
    move.q  r4, r19
    beqz    r21, .cpp_semantic_bound_scan
    move.q  r4, r21
.cpp_semantic_bound_scan:
.cpp_semantic_spaces:
    bge     r1, r4, .cpp_malformed_line
    load.b  r2, (r1)
    move.q  r3, #0x20
    bne     r2, r3, .cpp_semantic_kind
    add.q   r1, r1, #1
    bra     .cpp_semantic_spaces
.cpp_semantic_kind:
    move.q  r3, #TK_LET
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_END
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_STOP
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_RETURN
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_GOTO
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_GOSUB
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_PRINT
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_IF
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_FOR
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_NEXT
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_DIM
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_POKE
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_BITSET
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_BITCLR
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_WHILE
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_UNTIL          ; WEND in statement position
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_DO
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_LOOP
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_ON
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_EXT
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_WAIT
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_VSYNC_CMD
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_CALL
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_REM
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_DATA
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_READ
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_RESTORE
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_INC
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_DEC
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_SWAP
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_BLIT
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_COPPER
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_WIDTH          ; token slot used by BLOAD
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_LOAD
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_SOUND
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #TK_SID_CMD
    beq     r2, r3, .cpp_semantic_assignment
    move.q  r3, #0x41
    blt     r2, r3, .cpp_semantic_lowercase
    move.q  r3, #0x5A
    ble     r2, r3, .cpp_semantic_assignment
.cpp_semantic_lowercase:
    move.q  r3, #0x61
    blt     r2, r3, .cpp_token
    move.q  r3, #0x7A
    bgt     r2, r3, .cpp_token
.cpp_semantic_assignment:
    ; The semantic statement parser consumes a zero-terminated statement.
    ; A stored line may contain more statements separated by ':'. Temporarily
    ; terminate only the current statement, restore the byte immediately after
    ; parsing on every status path, then continue at the following statement.
    ; This avoids copying syntax into a fixed scratch buffer and leaves the
    ; stored programme byte-for-byte unchanged even when parsing fails.
    move.q  r5, r23
.cpp_statement_kind_spaces:
    load.b  r1, (r5)
    move.q  r2, #0x20
    bne     r1, r2, .cpp_statement_kind_ready
    add.q   r5, r5, #1
    bra     .cpp_statement_kind_spaces
.cpp_statement_kind_ready:
    load.b  r1, (r5)
    move.q  r5, r0
    move.q  r2, #TK_IF
    bne     r1, r2, .cpp_statement_kind_set
    move.q  r5, #1
.cpp_statement_kind_set:
    move.q  r25, r23
    move.q  r26, r0
    move.q  r27, r0
.cpp_find_statement_end:
    bge     r25, r4, .cpp_malformed_line
    load.b  r1, (r25)
    beqz    r1, .cpp_statement_end_ready
    move.q  r2, #0x22
    beq     r1, r2, .cpp_statement_quote
    bnez    r27, .cpp_statement_scan_next
    move.q  r2, #TK_REM
    beq     r1, r2, .cpp_statement_comment
    move.q  r2, #0x3A
    bne     r1, r2, .cpp_statement_scan_next
    bnez    r5, .cpp_statement_scan_next
    bra     .cpp_statement_colon
.cpp_statement_scan_next:
    add.q   r25, r25, #1
    bra     .cpp_find_statement_end
.cpp_statement_quote:
    eor.q   r27, r27, #1
    bra     .cpp_statement_scan_next
.cpp_statement_comment:
    ; REM owns the remainder of its physical line, including any colons.
    add.q   r25, r25, #1
.cpp_statement_comment_scan:
    bge     r25, r4, .cpp_malformed_line
    load.b  r1, (r25)
    beqz    r1, .cpp_statement_end_ready
    add.q   r25, r25, #1
    bra     .cpp_statement_comment_scan
.cpp_statement_colon:
    move.q  r26, #1
    store.b r0, (r25)
.cpp_statement_end_ready:
    move.q  r8, r23
    move.q  r9, r25
    add.q   r9, r9, #1
    move.q  r10, r22
    jsr     compiler_parse_semantic_line
    move.q  r27, r8
    move.q  r28, r9
    move.q  r29, r10
    beqz    r26, .cpp_statement_restored
    move.q  r1, #0x3A
    store.b r1, (r25)
.cpp_statement_restored:
    move.q  r8, r27
    move.q  r9, r28
    move.q  r10, r29
    bnez    r8, .cpp_semantic_error
    beqz    r26, .cpp_next_line
    add.q   r23, r25, #1
    bra     .cpp_statement
.cpp_semantic_error:
    rts
.cpp_token:
    bge     r23, r19, .cpp_malformed_line
    bnez    r21, .cpp_have_next_bound
    bra     .cpp_read_token
.cpp_have_next_bound:
    bge     r23, r21, .cpp_malformed_line
.cpp_read_token:
    load.b  r14, (r23)
    beqz    r14, .cpp_next_line
    jsr     compiler_classify_token
    move.q  r9, #IR_OP_TOKEN
    move.q  r12, r22
    move.q  r13, r24
    move.q  r15, r0
    jsr     compiler_ir_append
    bnez    r8, .cpp_ir_error
    add.q   r23, r23, #1
    add.q   r24, r24, #1
    bra     .cpp_token

.cpp_next_line:
    beqz    r21, .cpp_done
    add.q   r1, r21, #8
    beq     r1, r19, .cpp_done     ; live programmes end at an 8-byte sentinel
    ble     r21, r18, .cpp_malformed_line
    bgt     r21, r19, .cpp_malformed_line
    move.q  r18, r21
    bra     .cpp_line
.cpp_done:
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpp_ir_error:
    move.q  r10, r28
    ; compiler_ir_append already returned status and line.
    rts
.cpp_malformed_line:
    move.q  r9, r22
    move.q  r10, r24
    move.q  r8, #1
    rts
.cpp_malformed:
    move.q  r9, r0
    move.q  r10, r0
    move.q  r8, #1
    rts

; Parse one tokenised assignment statement into semantic IR. This is the first
; typed parser entry point and deliberately accepts only syntax it lowers
; completely: [LET] scalar = unsigned-I64 arithmetic using +, - and *.
; R8=token cursor, R9=exclusive bound, R10=line. Returns status,line,offset.
compiler_parse_semantic_line:
    push    r18
    push    r19
    push    r20
    push    r21
    push    r22
    push    r23
    push    r24
    push    r25
    push    r26
    push    r27
    push    r28
    push    r29
    jsr     compiler_parse_semantic_line_inner
    la      r4, COMPILER_VALUE_NEXT
    store.q r22, (r4)
    move.q  r1, r8
    move.q  r2, r9
    move.q  r3, r10
    pop     r29
    pop     r28
    pop     r27
    pop     r26
    pop     r25
    pop     r24
    pop     r23
    pop     r22
    pop     r21
    pop     r20
    pop     r19
    pop     r18
    move.q  r8, r1
    move.q  r9, r2
    move.q  r10, r3
    rts
compiler_parse_semantic_line_inner:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r20, r10
    move.q  r21, r8                 ; diagnostic offset origin
    la      r1, COMPILER_VALUE_NEXT
    load.q  r22, (r1)               ; programme-unique compact value id
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_END
    beq     r1, r2, .cpsl_end
    move.q  r2, #TK_STOP
    beq     r1, r2, .cpsl_stop
    move.q  r2, #TK_RETURN
    beq     r1, r2, .cpsl_return
    move.q  r2, #TK_GOTO
    beq     r1, r2, .cpsl_goto
    move.q  r2, #TK_GOSUB
    beq     r1, r2, .cpsl_gosub
    move.q  r2, #TK_PRINT
    beq     r1, r2, .cpsl_print
    move.q  r2, #TK_IF
    beq     r1, r2, .cpsl_if
    move.q  r2, #TK_FOR
    beq     r1, r2, .cpsl_for
    move.q  r2, #TK_NEXT
    beq     r1, r2, .cpsl_next
    move.q  r2, #TK_DIM
    beq     r1, r2, .cpsl_dim
    move.q  r2, #TK_POKE
    beq     r1, r2, .cpsl_poke
    move.q  r2, #TK_BITSET
    beq     r1, r2, .cpsl_bitset
    move.q  r2, #TK_BITCLR
    beq     r1, r2, .cpsl_bitclr
    move.q  r2, #TK_WHILE
    beq     r1, r2, .cpsl_while
    move.q  r2, #TK_UNTIL
    beq     r1, r2, .cpsl_wend
    move.q  r2, #TK_DO
    beq     r1, r2, .cpsl_do
    move.q  r2, #TK_LOOP
    beq     r1, r2, .cpsl_loop
    move.q  r2, #TK_ON
    beq     r1, r2, .cpsl_on
    move.q  r2, #TK_EXT
    beq     r1, r2, .cpsl_ext
    move.q  r2, #TK_WAIT
    beq     r1, r2, .cpsl_wait
    move.q  r2, #TK_VSYNC_CMD
    beq     r1, r2, .cpsl_vsync
    move.q  r2, #TK_CALL
    beq     r1, r2, .cpsl_call
    move.q  r2, #TK_REM
    beq     r1, r2, .cpsl_rem
    move.q  r2, #TK_DATA
    beq     r1, r2, .cpsl_data
    move.q  r2, #TK_READ
    beq     r1, r2, .cpsl_read
    move.q  r2, #TK_RESTORE
    beq     r1, r2, .cpsl_restore
    move.q  r2, #TK_INC
    beq     r1, r2, .cpsl_inc
    move.q  r2, #TK_DEC
    beq     r1, r2, .cpsl_dec
    move.q  r2, #TK_SWAP
    beq     r1, r2, .cpsl_swap
    move.q  r2, #TK_BLIT
    beq     r1, r2, .cpsl_blit
    move.q  r2, #TK_COPPER
    beq     r1, r2, .cpsl_copper
    move.q  r2, #TK_LOAD
    beq     r1, r2, .cpsl_stored_load
    move.q  r2, #TK_WIDTH          ; token slot used by BLOAD
    beq     r1, r2, .cpsl_bload
    move.q  r2, #TK_SOUND
    beq     r1, r2, .cpsl_sound
    move.q  r2, #TK_SID_CMD
    beq     r1, r2, .cpsl_sid
    move.q  r2, #TK_LET
    bne     r1, r2, .cpsl_variable
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
.cpsl_variable:
    bge     r18, r19, .cpsl_syntax
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    beq     r1, r2, .cpsl_array_assignment
    load.b  r1, (r18)
    move.q  r2, #0x3D
    beq     r1, r2, .cpsl_assignment_equal
    move.q  r2, #TK_EQUAL
    bne     r1, r2, .cpsl_syntax
.cpsl_assignment_equal:
    add.q   r18, r18, #1
    move.q  r1, r29
    lsr.q   r1, r1, #63
    and.q   r1, r1, #1
    bnez    r1, .cpsl_string_assignment
    push    r29
    push    r0
    jsr     compiler_semantic_expression
    pop     r1
    pop     r29
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    ; STORE_SCALAR source is the expression value in the lhs field. The stable
    ; source-level symbol is carried separately in the auxiliary field.
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r15, r29
    move.q  r9, #IR_OP_STORE_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_string_assignment:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x22
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    move.q  r25, r18
.cpsl_string_assignment_scan:
    bge     r18, r19, .cpsl_syntax
    load.b  r1, (r18)
    beqz    r1, .cpsl_syntax
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_string_assignment_end
    add.q   r18, r18, #1
    bra     .cpsl_string_assignment_scan
.cpsl_string_assignment_end:
    sub.q   r26, r18, r25
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r15, r25
    lsl.q   r1, r29, #32
    or.q    r15, r15, r1
    move.q  r9, #IR_OP_STORE_STRING_LITERAL
    move.q  r10, #IR_TYPE_STRING
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_rem:
    ; Tokenisation has already established the comment boundary. Comments have
    ; no runtime effect and deliberately produce no IR operation.
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_data:
    add.q   r18, r18, #1
.cpsl_data_item:
    jsr     compiler_semantic_spaces
    move.q  r27, r0
    load.b  r1, (r18)
    move.q  r2, #TK_MINUS
    beq     r1, r2, .cpsl_data_negative
    move.q  r2, #0x2D
    bne     r1, r2, .cpsl_data_value
.cpsl_data_negative:
    move.q  r27, #1
    add.q   r18, r18, #1
.cpsl_data_value:
    push    r27
    push    r0
    load.b  r1, (r18)
    move.q  r2, #0x26
    beq     r1, r2, .cpsl_data_hex
    jsr     compiler_semantic_number
    bra     .cpsl_data_record
.cpsl_data_hex:
    jsr     compiler_semantic_hex
.cpsl_data_record:
    pop     r1
    pop     r27
    bnez    r8, .cpsl_error
    beqz    r27, .cpsl_data_positive
    load.q  r1, 8(r9)
    move.q  r2, #IR_TYPE_FP64
    bne     r1, r2, .cpsl_data_neg_i64
    load.q  r1, 48(r9)
    li      r2, #0x8000000000000000
    eor.q   r1, r1, r2
    store.q r1, 48(r9)
    bra     .cpsl_data_positive
.cpsl_data_neg_i64:
    load.q  r1, 48(r9)
    neg.q   r1, r1
    store.q r1, 48(r9)
.cpsl_data_positive:
    move.q  r1, #IR_OP_DATA
    store.q r1, (r9)
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    beqz    r1, .cpsl_data_done
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    bra     .cpsl_data_item
.cpsl_data_done:
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_read:
    add.q   r18, r18, #1
.cpsl_read_item:
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    move.q  r9, #IR_OP_READ_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r15, r29
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    beqz    r1, .cpsl_read_done
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    bra     .cpsl_read_item
.cpsl_read_done:
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_restore:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r9, #IR_OP_RESTORE_DATA
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_inc:
    move.q  r27, #TK_PLUS
    bra     .cpsl_incdec
.cpsl_dec:
    move.q  r27, #TK_MINUS
.cpsl_incdec:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    move.q  r26, r29
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r8, r29
    jsr     compiler_native_var_known_i64
    beqz    r8, .csf_name_type_ready
    move.q  r10, #IR_TYPE_I64
.csf_name_type_ready:
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r23
    move.q  r15, r26
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    push    r23
    push    r27
    move.q  r15, #1
    jsr     compiler_semantic_append_const
    move.q  r25, r23
    pop     r27
    pop     r23
    bnez    r8, .cpsl_ir_error
    move.q  r26, r27
    jsr     compiler_semantic_binary
    bnez    r8, .cpsl_ir_error
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r15, r29
    move.q  r9, #IR_OP_STORE_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_swap:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    move.q  r26, r29
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    move.q  r27, r29
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r8, r26
    jsr     compiler_native_var_known_i64
    beqz    r8, .csf_scalar_type_ready
    move.q  r10, #IR_TYPE_I64
.csf_scalar_type_ready:
    move.q  r24, r10
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r23
    move.q  r15, r26
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r25, r23
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r8, r27
    jsr     compiler_native_var_known_i64
    beqz    r8, .cpsl_swap_second_type_ready
    move.q  r10, #IR_TYPE_I64
.cpsl_swap_second_type_ready:
    move.q  r28, r10
    move.q  r7, r0
    move.q  r14, r23
    move.q  r15, r27
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r15, r26
    move.q  r9, #IR_OP_STORE_SCALAR
    move.q  r10, r28
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r14, r25
    lsl.q   r14, r14, #16
    move.q  r15, r27
    move.q  r9, #IR_OP_STORE_SCALAR
    move.q  r10, r24
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_array_assignment:
    add.q   r18, r18, #1
    push    r29
    push    r0
    jsr     compiler_semantic_expression
    move.q  r25, r23
    pop     r1
    pop     r29
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    ; Check the index before evaluating the value, so it is the top generated
    ; stack item at the BOUNDS_CHECK record.
    move.q  r14, r25
    lsl.q   r14, r14, #16
    move.q  r15, r29
    move.q  r9, #IR_OP_BOUNDS_CHECK
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x3D
    beq     r1, r2, .cpsl_for_equal
    move.q  r2, #TK_EQUAL
    bne     r1, r2, .cpsl_syntax
.cpsl_for_equal:
    add.q   r18, r18, #1
    push    r25
    push    r29
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r29
    pop     r25
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    move.q  r15, r29
    move.q  r9, #IR_OP_STORE_ARRAY
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_end:
    move.q  r9, #IR_OP_HALT
    bra     .cpsl_noarg_control
.cpsl_stop:
    move.q  r9, #IR_OP_STOP
    bra     .cpsl_noarg_control
.cpsl_return:
    move.q  r9, #IR_OP_RETURN
.cpsl_noarg_control:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r15, r0
    bra     .cpsl_emit_control
.cpsl_goto:
    move.q  r9, #IR_OP_BRANCH
    bra     .cpsl_target_control
.cpsl_gosub:
    move.q  r9, #IR_OP_GOSUB
.cpsl_target_control:
    push    r9
    push    r0
    add.q   r18, r18, #1
    jsr     compiler_semantic_uint
    pop     r1
    pop     r9
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
.cpsl_emit_control:
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r1, #IR_OP_STOP
    beq     r9, r1, .cpsl_control_identity
    move.q  r1, #IR_OP_RETURN
    bne     r9, r1, .cpsl_control_identity_done
.cpsl_control_identity:
    move.q  r14, r20
    lsl.q   r14, r14, #8
    and.q   r1, r13, #0xFF
    or.q    r14, r14, r1
.cpsl_control_identity_done:
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_print:
    add.q   r18, r18, #1
.cpsl_print_item:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    beqz    r1, .cpsl_print_empty
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_print_string
    move.q  r25, r18
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_print_numeric_restore
    move.q  r1, r29
    lsr.q   r1, r1, #63
    and.q   r1, r1, #1
    bnez    r1, .cpsl_print_string_variable
.cpsl_print_numeric_restore:
    move.q  r18, r25
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    jsr     .cpsl_print_separator
    bnez    r8, .cpsl_error
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r15, #COMPILER_HELPER_PRINT_NUM
    lsl.q   r1, r28, #32
    or.q    r15, r15, r1
    move.q  r9, #IR_OP_PRINT
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bnez    r27, .cpsl_print_item
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_print_string:
    add.q   r18, r18, #1
    move.q  r25, r18
.cpsl_print_string_scan:
    bge     r18, r19, .cpsl_syntax
    load.b  r1, (r18)
    beqz    r1, .cpsl_syntax
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_print_string_end
    add.q   r18, r18, #1
    bra     .cpsl_print_string_scan
.cpsl_print_string_end:
    sub.q   r26, r18, r25
    add.q   r18, r18, #1
    jsr     .cpsl_print_separator
    bnez    r8, .cpsl_error
    move.q  r14, r26
    lsl.q   r1, r28, #32
    or.q    r14, r14, r1
    move.q  r15, r25
    move.q  r9, #IR_OP_PRINT_STRING
    move.q  r10, #IR_TYPE_STRING
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bnez    r27, .cpsl_print_item
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_print_string_variable:
    move.q  r25, r29
    lsl.q   r25, r25, #32
    lsr.q   r25, r25, #32
    jsr     .cpsl_print_separator
    bnez    r8, .cpsl_error
    move.q  r14, r28
    lsl.q   r14, r14, #32
    move.q  r15, r25
    move.q  r9, #IR_OP_PRINT_STRING_VAR
    move.q  r10, #IR_TYPE_STRING
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bnez    r27, .cpsl_print_item
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_print_empty:
    move.q  r14, #1
    lsl.q   r14, r14, #32
    move.q  r15, r18
    move.q  r9, #IR_OP_PRINT_STRING
    move.q  r10, #IR_TYPE_STRING
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts

; Determine output following one PRINT item. R28 is 0 for no terminator, 1
; for newline and 2 for BASIC's comma tab. R27 says another item follows.
.cpsl_print_separator:
    jsr     compiler_semantic_spaces
    move.q  r28, #1
    move.q  r27, r0
    load.b  r1, (r18)
    beqz    r1, .cpsl_print_separator_ok
    move.q  r2, #0x3B
    beq     r1, r2, .cpsl_print_semicolon
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_print_separator_bad
    move.q  r28, #2
    bra     .cpsl_print_separator_consume
.cpsl_print_semicolon:
    move.q  r28, r0
.cpsl_print_separator_consume:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    beqz    r1, .cpsl_print_separator_ok
    move.q  r27, #1
.cpsl_print_separator_ok:
    move.q  r8, r0
    rts
.cpsl_print_separator_bad:
    move.q  r8, #2
    rts
.cpsl_if:
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_THEN
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x30
    blt     r1, r2, .cpsl_if_inline
    move.q  r2, #0x39
    bgt     r1, r2, .cpsl_if_inline
    push    r23
    push    r0
    jsr     compiler_semantic_uint
    pop     r1
    pop     r23
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r9, #IR_OP_BRANCH_TRUE
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_if_inline:
    move.q  r26, r23
    li      r27, #0x8000000000000000
    move.q  r1, r20
    lsl.q   r1, r1, #16
    or.q    r27, r27, r1
    sub.q   r1, r18, r21
    and.q   r1, r1, #0xFFFF
    or.q    r27, r27, r1
    ; Match the first ELSE in the remaining statement, as the resident
    ; interpreter does for tokenised single-line IF statements.
    move.q  r24, r0
    move.q  r28, r18
    move.q  r29, r0
.cpsl_if_else_scan:
    load.b  r1, (r28)
    beqz    r1, .cpsl_if_else_ready
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_if_else_quote
    bnez    r29, .cpsl_if_else_next
    move.q  r2, #TK_REM
    beq     r1, r2, .cpsl_if_else_ready
    move.q  r2, #TK_ELSE
    bne     r1, r2, .cpsl_if_else_next
    bra     .cpsl_if_else_found
.cpsl_if_else_quote:
    eor.q   r29, r29, #1
.cpsl_if_else_next:
    add.q   r28, r28, #1
    bra     .cpsl_if_else_scan
.cpsl_if_else_found:
    move.q  r24, r28
    store.b r0, (r24)
.cpsl_if_else_ready:
    move.q  r14, r26
    lsl.q   r14, r14, #16
    move.q  r15, r27
    move.q  r9, #IR_OP_BRANCH_FALSE
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    add.q   r26, r27, #1
    push    r24
    push    r26
    move.q  r25, r18
.cpsl_if_inline_statement:
    jsr     compiler_semantic_spaces
    move.q  r25, r18
    load.b  r1, (r25)
    beqz    r1, .cpsl_if_inline_done
    move.q  r24, r0
    move.q  r2, #TK_IF
    bne     r1, r2, .cpsl_if_inline_kind_ready
    move.q  r24, #1
.cpsl_if_inline_kind_ready:
    move.q  r28, r25
    move.q  r29, r0
    move.q  r23, r0
.cpsl_if_inline_scan:
    bge     r28, r19, .cpsl_if_inline_scan_syntax
    load.b  r1, (r28)
    beqz    r1, .cpsl_if_inline_segment
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_if_inline_quote
    bnez    r29, .cpsl_if_inline_scan_next
    move.q  r2, #TK_REM
    beq     r1, r2, .cpsl_if_inline_to_end
    move.q  r2, #0x3A
    bne     r1, r2, .cpsl_if_inline_scan_next
    bnez    r24, .cpsl_if_inline_scan_next
    bra     .cpsl_if_inline_colon
.cpsl_if_inline_quote:
    eor.q   r29, r29, #1
.cpsl_if_inline_scan_next:
    add.q   r28, r28, #1
    bra     .cpsl_if_inline_scan
.cpsl_if_inline_to_end:
    add.q   r28, r28, #1
.cpsl_if_inline_to_end_scan:
    load.b  r1, (r28)
    beqz    r1, .cpsl_if_inline_segment
    add.q   r28, r28, #1
    bra     .cpsl_if_inline_to_end_scan
.cpsl_if_inline_colon:
    move.q  r23, #1
    store.b r0, (r28)
.cpsl_if_inline_segment:
    move.q  r8, r25
    add.q   r9, r28, #1
    move.q  r10, r20
    jsr     compiler_parse_semantic_line
    move.q  r24, r8
    ; Nested inline statements allocate from the shared value namespace.
    ; Refresh the outer cursor so its wrapper cannot roll VALUE_NEXT back.
    la      r1, COMPILER_VALUE_NEXT
    load.q  r22, (r1)
    beqz    r23, .cpsl_if_inline_parsed
    move.q  r1, #0x3A
    store.b r1, (r28)
.cpsl_if_inline_parsed:
    bnez    r24, .cpsl_if_inline_nested_error
    beqz    r23, .cpsl_if_inline_at_end
    add.q   r18, r28, #1
    bra     .cpsl_if_inline_statement
.cpsl_if_inline_at_end:
    move.q  r18, r28
.cpsl_if_inline_done:
    pop     r26
    pop     r24
    beqz    r24, .cpsl_if_inline_final_block
    move.q  r1, #TK_ELSE
    store.b r1, (r24)
    move.q  r9, #IR_OP_BRANCH
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r15, r26
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r9, #IR_OP_BLOCK
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r27
    move.q  r15, r27
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    add.q   r18, r24, #1
    move.q  r27, r26
    push    r0
    push    r26
    bra     .cpsl_if_inline_statement
.cpsl_if_inline_final_block:
    move.q  r9, #IR_OP_BLOCK
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r27
    move.q  r15, r27
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_if_inline_nested_error:
    pop     r26
    pop     r1
    beqz    r1, .cpsl_if_inline_nested_restored
    move.q  r2, #TK_ELSE
    store.b r2, (r1)
.cpsl_if_inline_nested_restored:
    move.q  r8, r24
    move.q  r9, r20
    sub.q   r10, r25, r21
    rts
.cpsl_if_inline_scan_syntax:
    pop     r1
    pop     r2
    beqz    r2, .cpsl_if_inline_scan_syntax_done
    move.q  r1, #TK_ELSE
    store.b r1, (r2)
.cpsl_if_inline_scan_syntax_done:
    bra     .cpsl_syntax
.cpsl_for:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x3D
    beq     r1, r2, .cpsl_array_equal
    move.q  r2, #TK_EQUAL
    bne     r1, r2, .cpsl_syntax
.cpsl_array_equal:
    add.q   r18, r18, #1
    push    r29
    push    r0
    jsr     compiler_semantic_expression
    pop     r1
    pop     r29
    bnez    r8, .cpsl_error
    push    r23
    push    r0
    push    r29
    push    r0
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_TO
    bne     r1, r2, .cpsl_for_pop_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    move.q  r27, r23
    pop     r1
    pop     r29
    pop     r1
    pop     r26
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_STEP
    bne     r1, r2, .cpsl_for_default_step
    add.q   r18, r18, #1
    push    r26
    push    r27
    push    r29
    push    r0
    jsr     compiler_semantic_expression
    move.q  r28, r23
    pop     r1
    pop     r29
    pop     r27
    pop     r26
    bnez    r8, .cpsl_error
    bra     .cpsl_for_ready
.cpsl_for_default_step:
    move.q  r28, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r28
    move.q  r15, #1
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
.cpsl_for_ready:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r27
    lsl.q   r14, r14, #32
    move.q  r1, r26
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r28
    move.q  r15, r29
    lsl.q   r15, r15, #32
    or.q    r15, r15, r20
    move.q  r9, #IR_OP_FOR
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_for_pop_syntax:
    pop     r1
    pop     r1
    pop     r1
    pop     r1
    bra     .cpsl_syntax
.cpsl_next:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r29, r0
    load.b  r1, (r18)
    beqz    r1, .cpsl_next_ready
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
.cpsl_next_ready:
    move.q  r15, r29
    lsl.q   r15, r15, #32
    move.q  r9, #IR_OP_NEXT
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_dim:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .cpsl_syntax
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    push    r29
    push    r0
    jsr     compiler_semantic_expression
    move.q  r25, r23
    pop     r1
    pop     r29
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r25
    lsl.q   r14, r14, #16
    move.q  r15, r29
    move.q  r9, #IR_OP_DIM_ARRAY
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_poke:
    move.q  r28, #IR_OP_POKE8
    add.q   r18, r18, #1
    bra     .cpsl_poke_body
.cpsl_sid:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_STOP
    beq     r1, r2, .cpsl_sid_stop
    or.q    r1, r1, #0x20
    move.q  r2, #0x73
    beq     r1, r2, .cpsl_sid_stop
    move.q  r2, #0x70
    bne     r1, r2, .cpsl_syntax
    jsr     compiler_semantic_skip_word
    move.q  r15, #SID_PLAY_PTR
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #SID_PLAY_LEN
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_sid_start
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r15, #SID_SUBSONG
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
.cpsl_sid_start:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r15, #SID_PLAY_CTRL
    move.q  r17, #1
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_bitset:
    move.q  r28, #IR_OP_BITSET
    bra     .cpsl_bit_modify
.cpsl_bitclr:
    move.q  r28, #IR_OP_BITCLR
.cpsl_bit_modify:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    push    r28
    jsr     compiler_semantic_expression
    move.q  r25, r23
    pop     r28
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    push    r25
    push    r28
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r28
    pop     r25
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    lsl.q   r14, r25, #16
    lsl.q   r1, r26, #32
    or.q    r14, r14, r1
    move.q  r9, r28
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_RAM
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_while:
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    lsl.q   r14, r23, #16
    move.q  r15, r0
    move.q  r9, #IR_OP_WHILE
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_wend:
    move.q  r28, #IR_OP_WEND
    bra     .cpsl_control_marker
.cpsl_do:
    move.q  r28, #IR_OP_DO
.cpsl_control_marker:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r9, r28
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_loop:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r25, r0
    load.b  r1, (r18)
    beqz    r1, .cpsl_loop_ready
    move.q  r2, #TK_WHILE
    beq     r1, r2, .cpsl_loop_while
    move.q  r2, #TK_UNTIL
    bne     r1, r2, .cpsl_syntax
    move.q  r25, #2
    bra     .cpsl_loop_condition
.cpsl_loop_while:
    move.q  r25, #1
.cpsl_loop_condition:
    add.q   r18, r18, #1
    push    r25
    jsr     compiler_semantic_expression
    pop     r25
    bnez    r8, .cpsl_error
    move.q  r26, r23
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    lsl.q   r14, r26, #16
.cpsl_loop_ready:
    lsl.q   r15, r25, #32
    move.q  r9, #IR_OP_LOOP
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_on:
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    move.q  r25, r23
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r26, r0
    move.q  r2, #TK_GOTO
    beq     r1, r2, .cpsl_on_kind_ready
    move.q  r2, #TK_GOSUB
    bne     r1, r2, .cpsl_syntax
    move.q  r26, #1
.cpsl_on_kind_ready:
    add.q   r18, r18, #1
    lsl.q   r14, r25, #16
    move.q  r15, r0
    move.q  r9, #IR_OP_ON_START
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r27, #1
.cpsl_on_target:
    jsr     compiler_semantic_spaces
    push    r26
    push    r27
    jsr     compiler_semantic_uint
    pop     r27
    pop     r26
    bnez    r8, .cpsl_error
    move.q  r28, r15
    move.q  r15, r28
    lsl.q   r1, r26, #32
    or.q    r15, r15, r1
    move.q  r14, r27
    move.q  r9, #IR_OP_ON_TARGET
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    add.q   r27, r27, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    beqz    r1, .cpsl_on_done
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    bra     .cpsl_on_target
.cpsl_on_done:
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_sid_stop:
    jsr     compiler_semantic_skip_word
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r15, #SID_PLAY_CTRL
    move.q  r17, r0
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_sound:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    or.q    r1, r1, #0x20
    move.q  r2, #0x70              ; PLAY
    bne     r1, r2, .cpsl_sound_stop_check
    jsr     compiler_semantic_skip_word
    load.b  r1, (r18)
    move.q  r2, #TK_STOP
    beq     r1, r2, .cpsl_sound_stop
    move.q  r2, #0x53
    beq     r1, r2, .cpsl_sound_stop
    move.q  r2, #0x22
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    move.q  r25, r18
.cpsl_sound_name_scan:
    bge     r18, r19, .cpsl_syntax
    load.b  r1, (r18)
    beqz    r1, .cpsl_syntax
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_sound_name_end
    add.q   r18, r18, #1
    bra     .cpsl_sound_name_scan
.cpsl_sound_name_end:
    sub.q   r27, r18, r25
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_sound_default_subsong
    add.q   r18, r18, #1
    push    r25
    push    r27
    jsr     compiler_semantic_expression
    pop     r27
    pop     r25
    bnez    r8, .cpsl_error
    bra     .cpsl_sound_emit
.cpsl_sound_default_subsong:
    push    r25
    push    r27
    move.q  r15, r0
    jsr     compiler_semantic_append_const
    pop     r27
    pop     r25
    bnez    r8, .cpsl_ir_error
.cpsl_sound_emit:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r27
    lsl.q   r14, r14, #32
    move.q  r1, r23
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    move.q  r1, r20
    and.q   r1, r1, #0xFFFF
    or.q    r14, r14, r1
    move.q  r15, r25
    move.q  r9, #IR_OP_SOUND_PLAY
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_sound_stop_check:
    load.b  r1, (r18)
    move.q  r2, #TK_STOP
    beq     r1, r2, .cpsl_sound_stop
    or.q    r1, r1, #0x20
    move.q  r2, #0x73
    bne     r1, r2, .cpsl_syntax
.cpsl_sound_stop:
    jsr     compiler_semantic_skip_word
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r15, #MEDIA_CTRL
    move.q  r17, #2
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_bload:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x22
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    move.q  r25, r18
.cpsl_bload_name_scan:
    bge     r18, r19, .cpsl_syntax
    load.b  r1, (r18)
    beqz    r1, .cpsl_syntax
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_bload_name_end
    add.q   r18, r18, #1
    bra     .cpsl_bload_name_scan
.cpsl_bload_name_end:
    sub.q   r27, r18, r25
    add.q   r18, r18, #1
    push    r25
    push    r27
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_bload_pop_syntax
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r27
    pop     r25
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r27
    lsl.q   r14, r14, #32
    move.q  r1, r26
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    move.q  r1, r20
    and.q   r1, r1, #0xFFFF
    or.q    r14, r14, r1
    move.q  r15, r25
    move.q  r9, #IR_OP_BLOAD
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_stored_load:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x22
    bne     r1, r2, .cpsl_syntax
    add.q   r18, r18, #1
    move.q  r25, r18
.cpsl_stored_load_name_scan:
    bge     r18, r19, .cpsl_syntax
    load.b  r1, (r18)
    beqz    r1, .cpsl_syntax
    move.q  r2, #0x22
    beq     r1, r2, .cpsl_stored_load_name_end
    add.q   r18, r18, #1
    bra     .cpsl_stored_load_name_scan
.cpsl_stored_load_name_end:
    sub.q   r27, r18, r25
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r27
    lsl.q   r14, r14, #32
    move.q  r1, r20
    and.q   r1, r1, #0xFFFF
    or.q    r14, r14, r1
    move.q  r15, r25
    move.q  r9, #IR_OP_STORED_LOAD
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, r20
    move.q  r13, r0
    move.q  r7, #IR_FLAG_ARENA_ONLY
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_bload_pop_syntax:
    pop     r1
    pop     r1
    bra     .cpsl_syntax
.cpsl_copper:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_LIST
    beq     r1, r2, .cpsl_copper_list
    move.q  r2, #TK_WAIT
    beq     r1, r2, .cpsl_copper_wait
    move.q  r2, #TK_END
    beq     r1, r2, .cpsl_copper_end
    move.q  r2, #TK_ON
    beq     r1, r2, .cpsl_copper_on
    or.q    r1, r1, #0x20
    move.q  r2, #0x6C
    beq     r1, r2, .cpsl_copper_list
    move.q  r2, #0x77
    beq     r1, r2, .cpsl_copper_wait
    move.q  r2, #0x6D
    beq     r1, r2, .cpsl_copper_move
    move.q  r2, #0x65
    beq     r1, r2, .cpsl_copper_end
    move.q  r2, #0x6F
    beq     r1, r2, .cpsl_copper_o
    bra     .cpsl_syntax
.cpsl_copper_o:
    load.b  r1, 1(r18)
    or.q    r1, r1, #0x20
    move.q  r2, #0x66
    beq     r1, r2, .cpsl_copper_off
    bra     .cpsl_copper_on
.cpsl_copper_list:
    jsr     compiler_semantic_skip_word
    load.b  r1, (r18)
    bnez    r1, .cpsl_copper_list_expression
    move.q  r15, #4096
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r25, r23
    move.q  r15, #4
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r26, r23
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r23
    move.q  r9, #IR_OP_MEMALLOC
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bra     .cpsl_copper_unary_list
.cpsl_copper_list_expression:
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
.cpsl_copper_unary_list:
    move.q  r9, #IR_OP_COPPER_LIST
    bra     .cpsl_copper_unary
.cpsl_copper_wait:
    jsr     compiler_semantic_skip_word
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    move.q  r9, #IR_OP_COPPER_WAIT
.cpsl_copper_unary:
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bra     .cpsl_copper_done
.cpsl_copper_move:
    jsr     compiler_semantic_skip_word
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    push    r23
    push    r0
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_copper_move_pop_syntax
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r1
    pop     r25
    bnez    r8, .cpsl_error
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    move.q  r9, #IR_OP_COPPER_MOVE
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bra     .cpsl_copper_done
.cpsl_copper_move_pop_syntax:
    pop     r1
    pop     r1
    bra     .cpsl_syntax
.cpsl_copper_end:
    jsr     compiler_semantic_skip_word
    move.q  r9, #IR_OP_COPPER_END
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r0
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    bra     .cpsl_copper_done
.cpsl_copper_on:
    jsr     compiler_semantic_skip_word
    move.q  r15, #COPPER_CTRL
    move.q  r17, #1
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    bra     .cpsl_copper_done
.cpsl_copper_off:
    jsr     compiler_semantic_skip_word
    move.q  r15, #COPPER_CTRL
    move.q  r17, r0
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
.cpsl_copper_done:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_blit:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    or.q    r1, r1, #0x20
    move.q  r2, #0x66              ; FILL
    beq     r1, r2, .cpsl_blit_fill
    move.q  r2, #0x63              ; COPY
    beq     r1, r2, .cpsl_blit_copy
    move.q  r2, #0x6D              ; MEMCOPY or MODE7
    beq     r1, r2, .cpsl_blit_m
    move.q  r2, #0x77              ; WAIT
    beq     r1, r2, .cpsl_blit_wait
    bra     .cpsl_syntax
.cpsl_blit_fill:
    jsr     compiler_semantic_skip_alpha
.cpsl_blit_fill_args:
    jsr     compiler_semantic_spaces
    move.q  r15, #BLT_DST
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_WIDTH
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_HEIGHT
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_COLOR
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_blit_fill_start
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r15, #BLT_DST_STRIDE
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
.cpsl_blit_fill_start:
    move.q  r17, #BLT_OP_FILL
    bra     .cpsl_blit_start
.cpsl_blit_copy:
    jsr     compiler_semantic_skip_alpha
    move.q  r15, #BLT_SRC
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_DST
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_WIDTH
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_HEIGHT
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_blit_copy_start
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r15, #BLT_SRC_STRIDE
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_blit_copy_start
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r15, #BLT_DST_STRIDE
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
.cpsl_blit_copy_start:
    move.q  r17, #BLT_OP_COPY
    bra     .cpsl_blit_start
.cpsl_blit_wait:
    jsr     compiler_semantic_skip_alpha
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r15, #BLT_CTRL
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r25, r23
    move.q  r15, r0
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r26, r23
    move.q  r15, r0
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r27, r23
    bra     .cpsl_wait_ready
.cpsl_blit_start:
    push    r16
    push    r0
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_blit_start_syntax
    pop     r1
    pop     r16
    move.q  r15, #BLT_OP
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    move.q  r15, #BLT_CTRL
    move.q  r17, #1
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_blit_start_syntax:
    pop     r1
    pop     r1
    bra     .cpsl_syntax
.cpsl_blit_m:
    load.b  r1, 1(r18)
    or.q    r1, r1, #0x20
    move.q  r2, #0x6F              ; MODE7
    beq     r1, r2, .cpsl_blit_mode7
    jsr     compiler_semantic_skip_alpha
    move.q  r15, #BLT_SRC
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_DST
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r15, r0
    move.q  r9, #IR_OP_BLIT_LENGTH
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r17, #BLT_OP_MEMCOPY
    bra     .cpsl_blit_start
.cpsl_blit_mode7:
    jsr     compiler_semantic_skip_alpha
    load.b  r1, (r18)
    move.q  r2, #0x37
    bne     r1, r2, .cpsl_blit_mode7_args
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
.cpsl_blit_mode7_args:
    la      r28, compiler_blit_mode7_addresses
    move.q  r27, #12
    jsr     compiler_semantic_mmio32_arguments
    bnez    r8, .cpsl_error
    ; The short form explicitly clears both staged strides.
    move.q  r15, #BLT_SRC_STRIDE
    move.q  r17, r0
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    move.q  r15, #BLT_DST_STRIDE
    move.q  r17, r0
    jsr     compiler_semantic_mmio32_const
    bnez    r8, .cpsl_ir_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_blit_mode7_start
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r15, #BLT_SRC_STRIDE
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_expect_comma
    bnez    r8, .cpsl_syntax
    move.q  r15, #BLT_DST_STRIDE
    jsr     compiler_semantic_mmio32_expression
    bnez    r8, .cpsl_error
.cpsl_blit_mode7_start:
    move.q  r17, #BLT_OP_MODE7
    bra     .cpsl_blit_start
.cpsl_ext:
    add.q   r18, r18, #1
    load.b  r1, (r18)
    move.q  r2, #EXT_POKE8
    beq     r1, r2, .cpsl_ext_poke8
    move.q  r2, #EXT_POKE16
    beq     r1, r2, .cpsl_ext_poke16
    move.q  r2, #EXT_POKE32
    beq     r1, r2, .cpsl_ext_poke32
    move.q  r2, #EXT_POKE64
    bne     r1, r2, .cpsl_syntax
    move.q  r28, #IR_OP_POKE64
    bra     .cpsl_ext_poke_ready
.cpsl_ext_poke8:
    move.q  r28, #IR_OP_POKE8
    bra     .cpsl_ext_poke_ready
.cpsl_ext_poke16:
    move.q  r28, #IR_OP_POKE16
    bra     .cpsl_ext_poke_ready
.cpsl_ext_poke32:
    move.q  r28, #IR_OP_POKE32
.cpsl_ext_poke_ready:
    add.q   r18, r18, #1
.cpsl_poke_body:
    push    r28
    push    r0
    jsr     compiler_semantic_expression
    pop     r1
    pop     r28
    bnez    r8, .cpsl_error
    push    r23
    push    r28
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_poke_pop_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r28
    pop     r25
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    move.q  r15, r0
    move.q  r9, r28
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_poke_pop_syntax:
    pop     r1
    pop     r1
    bra     .cpsl_syntax
.cpsl_wait:
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    push    r23
    push    r0
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_wait_pop_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r1
    pop     r25
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .cpsl_wait_default_xor
    add.q   r18, r18, #1
    push    r25
    push    r26
    jsr     compiler_semantic_expression
    move.q  r27, r23
    pop     r26
    pop     r25
    bnez    r8, .cpsl_error
    bra     .cpsl_wait_ready
.cpsl_wait_default_xor:
    move.q  r15, r0
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r27, r23
    bra     .cpsl_wait_ready
.cpsl_vsync:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r15, #VIDEO_STATUS
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r25, r23
    move.q  r15, #STATUS_VBLANK
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r26, r23
    move.q  r15, r0
    jsr     compiler_semantic_append_const
    bnez    r8, .cpsl_ir_error
    move.q  r27, r23
.cpsl_wait_ready:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r27
    move.q  r15, r20
    move.q  r9, #IR_OP_WAIT
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_wait_pop_syntax:
    pop     r1
    pop     r1
    bra     .cpsl_syntax
.cpsl_call:
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .cpsl_error
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    bnez    r1, .cpsl_syntax
    move.q  r14, r23
    lsl.q   r14, r14, #16
    move.q  r15, r0
    move.q  r9, #IR_OP_CALL
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .cpsl_ir_error
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsl_ir_error:
    move.q  r10, r13
    rts
.cpsl_syntax:
    move.q  r8, #2
.cpsl_error:
    move.q  r9, r20
    sub.q   r10, r18, r21
    rts

; R18 is the shared cursor and R19 the exclusive bound.
compiler_semantic_spaces:
    bge     r18, r19, .css_done
    load.b  r1, (r18)
    move.q  r2, #0x20
    bne     r1, r2, .css_done
    add.q   r18, r18, #1
    bra     compiler_semantic_spaces
.css_done:
    rts

; Parse the live BASIC identifier format and produce the same 32-bit identity
; tag as var_parse_name. R29=tag, R28=non-zero character count, R18 advanced.
compiler_semantic_name:
    move.q  r29, r0
    move.q  r28, r0
.csname_collect:
    load.b  r1, (r18)
    move.q  r2, #0x41
    blt     r1, r2, .csname_digit
    move.q  r2, #0x5A
    ble     r1, r2, .csname_alpha
    move.q  r2, #0x61
    blt     r1, r2, .csname_digit
    move.q  r2, #0x7A
    bgt     r1, r2, .csname_digit
    sub.q   r1, r1, #0x20
.csname_alpha:
    move.q  r2, #4
    bge     r28, r2, .csname_mix
    lsl.l   r29, r29, #8
    or.l    r29, r29, r1
    add.q   r28, r28, #1
    bra     .csname_advance
.csname_mix:
    mulu.l  r29, r29, #33
    add.l   r29, r29, r1
.csname_advance:
    add.q   r18, r18, #1
    bra     .csname_collect
.csname_digit:
    move.q  r2, #0x30
    blt     r1, r2, .csname_done
    move.q  r2, #0x39
    bgt     r1, r2, .csname_done
    beqz    r28, .csname_done
    bra     .csname_alpha
.csname_done:
    load.b  r1, (r18)
    move.q  r2, #0x24              ; '$' string suffix
    bne     r1, r2, .csname_return
    li      r2, #0x8000000000000000
    or.q    r29, r29, r2
    add.q   r18, r18, #1
.csname_return:
    rts

; BASIC logical precedence is OR/EOR, AND, NOT, comparison, arithmetic.
compiler_semantic_expression:
    jsr     compiler_semantic_logical_or
    rts

compiler_semantic_logical_or:
    jsr     compiler_semantic_logical_and
    bnez    r8, .cslo_done
.cslo_operator:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r26, #IR_OP_BIT_OR
    move.q  r2, #TK_OR
    beq     r1, r2, .cslo_rhs
    move.q  r26, #IR_OP_BIT_XOR
    move.q  r2, #TK_EOR
    bne     r1, r2, .cslo_done
.cslo_rhs:
    add.q   r18, r18, #1
    push    r23
    push    r26
    jsr     compiler_semantic_logical_and
    move.q  r25, r23
    pop     r26
    pop     r23
    bnez    r8, .cslo_done
    jsr     compiler_semantic_logic_binary
    bnez    r8, .cslo_done
    bra     .cslo_operator
.cslo_done:
    rts

compiler_semantic_logical_and:
    jsr     compiler_semantic_logical_not
    bnez    r8, .csla_done
.csla_operator:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_AND
    bne     r1, r2, .csla_done
    add.q   r18, r18, #1
    push    r23
    push    r0
    jsr     compiler_semantic_logical_not
    move.q  r25, r23
    pop     r1
    pop     r23
    bnez    r8, .csla_done
    move.q  r26, #IR_OP_BIT_AND
    jsr     compiler_semantic_logic_binary
    bnez    r8, .csla_done
    bra     .csla_operator
.csla_done:
    rts

compiler_semantic_logical_not:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_NOT
    bne     r1, r2, compiler_semantic_compare
    add.q   r18, r18, #1
    jsr     compiler_semantic_logical_not
    bnez    r8, .csln_done
    move.q  r14, r23
    lsl.q   r14, r14, #16
    or.q    r14, r14, r22
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_BIT_NOT
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
.csln_done:
    rts

compiler_semantic_logic_binary:
    move.q  r9, r26
    move.q  r14, r25
    lsl.q   r14, r14, #32
    move.q  r1, r23
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r22
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    rts

compiler_semantic_compare:
    jsr     compiler_semantic_arithmetic
    bnez    r8, .csex_done
    jsr     compiler_semantic_spaces
    load.b  r26, (r18)
    move.q  r1, #TK_EQUAL
    beq     r26, r1, .csex_eq
    move.q  r1, #TK_LT
    beq     r26, r1, .csex_lt
    move.q  r1, #TK_GT
    bne     r26, r1, .csex_success
    move.q  r27, #IR_OP_COMPARE_GT
    bra     .csex_consume
.csex_eq:
    move.q  r27, #IR_OP_COMPARE_EQ
    bra     .csex_consume
.csex_lt:
    move.q  r27, #IR_OP_COMPARE_LT
.csex_consume:
    add.q   r18, r18, #1
    load.b  r1, (r18)
    move.q  r2, #TK_EQUAL
    beq     r1, r2, .csex_equal_marker
    move.q  r2, #0x3D
    bne     r1, r2, .csex_opposite
.csex_equal_marker:
    move.q  r2, #IR_OP_COMPARE_LT
    bne     r27, r2, .csex_ge_check
    move.q  r27, #IR_OP_COMPARE_LE
    bra     .csex_second
.csex_ge_check:
    move.q  r2, #IR_OP_COMPARE_GT
    bne     r27, r2, .csex_rhs
    move.q  r27, #IR_OP_COMPARE_GE
    bra     .csex_second
.csex_opposite:
    move.q  r2, #TK_GT
    beq     r1, r2, .csex_ne_check
    move.q  r2, #0x3E
    beq     r1, r2, .csex_ne_check
    move.q  r2, #TK_LT
    beq     r1, r2, .csex_ne_check
    move.q  r2, #0x3C
    bne     r1, r2, .csex_rhs
.csex_ne_check:
    move.q  r2, #IR_OP_COMPARE_LT
    beq     r27, r2, .csex_ne
    move.q  r2, #IR_OP_COMPARE_GT
    bne     r27, r2, .csex_rhs
.csex_ne:
    move.q  r27, #IR_OP_COMPARE_NE
.csex_second:
    add.q   r18, r18, #1
.csex_rhs:
    push    r23
    push    r27
    jsr     compiler_semantic_arithmetic
    move.q  r25, r23
    pop     r27
    pop     r23
    bnez    r8, .csex_done
    move.q  r9, r27
    move.q  r14, r25
    lsl.q   r14, r14, #32
    move.q  r1, r23
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r22
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csex_success:
    move.q  r8, r0
.csex_done:
    rts

compiler_semantic_arithmetic:
    jsr     compiler_semantic_term
    bnez    r8, .cse_done
    move.q  r28, r23
.cse_operator:
    jsr     compiler_semantic_spaces
    load.b  r27, (r18)
    move.q  r1, #TK_PLUS
    beq     r27, r1, .cse_rhs
    move.q  r1, #TK_MINUS
    beq     r27, r1, .cse_rhs
    move.q  r1, #TK_LSHIFT
    beq     r27, r1, .cse_rhs
    move.q  r1, #TK_RSHIFT
    bne     r27, r1, .cse_success
.cse_rhs:
    add.q   r18, r18, #1
    push    r28
    push    r27
    jsr     compiler_semantic_term
    move.q  r25, r23
    pop     r27
    pop     r28
    bnez    r8, .cse_done
    move.q  r23, r28
    move.q  r26, r27
    jsr     compiler_semantic_binary
    bnez    r8, .cse_done
    move.q  r28, r23
    bra     .cse_operator
.cse_success:
    move.q  r23, r28
    move.q  r8, r0
.cse_done:
    rts

; term := number (* number)*. Returns the value id in R23.
compiler_semantic_term:
    jsr     compiler_semantic_power
    bnez    r8, .cst_done
.cst_operator:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_MULT
    beq     r1, r2, .cst_rhs
    move.q  r2, #TK_DIV
    bne     r1, r2, .cst_success
.cst_rhs:
    push    r23
    push    r1
    add.q   r18, r18, #1
    jsr     compiler_semantic_power
    move.q  r25, r23
    pop     r26
    pop     r23
    bnez    r8, .cst_done
    jsr     compiler_semantic_binary
    bnez    r8, .cst_done
    bra     .cst_operator
.cst_success:
    move.q  r8, r0
.cst_done:
    rts

compiler_semantic_power:
    jsr     compiler_semantic_factor
    bnez    r8, .csp_done
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_POWER
    bne     r1, r2, .csp_done
    add.q   r18, r18, #1
    push    r23
    push    r0
    jsr     compiler_semantic_power
    move.q  r25, r23
    pop     r1
    pop     r23
    bnez    r8, .csp_done
    move.q  r26, #IR_OP_POW
    jsr     compiler_semantic_logic_binary
.csp_done:
    rts

; factor := number | scalar | '(' expression ')'.
compiler_semantic_factor:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #TK_PLUS
    beq     r1, r2, .csf_unary_plus
    move.q  r2, #TK_MINUS
    beq     r1, r2, .csf_unary_minus
    move.q  r2, #TK_PEEK
    beq     r1, r2, .csf_peek8
    move.q  r2, #TK_EXT
    beq     r1, r2, .csf_ext_peek
    move.q  r2, #TK_USR
    beq     r1, r2, .csf_usr
    move.q  r2, #TK_VPTR
    beq     r1, r2, .csf_varptr
    move.q  r2, #TK_PSG_CMD
    beq     r1, r2, .csf_status_psg
    move.q  r2, #TK_SID_CMD
    beq     r1, r2, .csf_status_sid
    move.q  r2, #TK_POKEY_CMD
    beq     r1, r2, .csf_status_sap
    move.q  r2, #TK_SAP
    beq     r1, r2, .csf_status_sap
    move.q  r2, #TK_TED_CMD
    beq     r1, r2, .csf_status_ted
    move.q  r2, #TK_AHX
    beq     r1, r2, .csf_status_ahx
    move.q  r2, #TK_PI
    beq     r1, r2, .csf_pi
    move.q  r2, #TK_TWOPI
    beq     r1, r2, .csf_twopi
    move.q  r2, #TK_SGN
    blt     r1, r2, .csf_not_numeric_fn
    move.q  r2, #TK_ATN
    ble     r1, r2, .csf_numeric_fn
.csf_not_numeric_fn:
    move.q  r2, #0x26
    beq     r1, r2, compiler_semantic_hex
    move.q  r2, #0x28
    bne     r1, r2, .csf_scalar
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .csf_done
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r8, r0
    rts
.csf_scalar:
    move.q  r2, #0x41
    blt     r1, r2, compiler_semantic_number
    move.q  r2, #0x5A
    ble     r1, r2, .csf_name
    move.q  r2, #0x61
    blt     r1, r2, compiler_semantic_number
    move.q  r2, #0x7A
    bgt     r1, r2, compiler_semantic_number
.csf_name:
    jsr     compiler_semantic_name
    beqz    r28, .csf_syntax
    move.q  r15, r29
    load.b  r1, (r18)
    move.q  r2, #0x28
    beq     r1, r2, .csf_array
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r8, r15
    jsr     compiler_native_var_known_i64
    beqz    r8, .csf_expr_name_type_ready
    move.q  r10, #IR_TYPE_I64
.csf_expr_name_type_ready:
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    sub.q   r13, r13, #1
    move.q  r14, r23
    move.q  r7, r0
    jsr     compiler_ir_append
.csf_done:
    rts
.csf_array:
    add.q   r18, r18, #1
    push    r15
    push    r0
    jsr     compiler_semantic_expression
    move.q  r25, r23
    pop     r1
    pop     r15
    bnez    r8, .csf_done
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r14, r25
    lsl.q   r14, r14, #16
    move.q  r9, #IR_OP_BOUNDS_CHECK
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    bnez    r8, .csf_done
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r25
    lsl.q   r14, r14, #16
    or.q    r14, r14, r23
    move.q  r9, #IR_OP_LOAD_ARRAY
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_peek8:
    move.q  r28, #IR_OP_PEEK8
    add.q   r18, r18, #1
    bra     .csf_peek_body
.csf_ext_peek:
    add.q   r18, r18, #1
    load.b  r1, (r18)
    move.q  r2, #EXT_MEMALLOC
    beq     r1, r2, .csf_memalloc
    move.q  r2, #EXT_PEEK8
    beq     r1, r2, .csf_ext_peek8
    move.q  r2, #EXT_PEEK16
    beq     r1, r2, .csf_ext_peek16
    move.q  r2, #EXT_PEEK32
    beq     r1, r2, .csf_ext_peek32
    move.q  r2, #EXT_PEEK64
    bne     r1, r2, .csf_syntax
    move.q  r28, #IR_OP_PEEK64
    bra     .csf_ext_peek_ready
.csf_ext_peek8:
    move.q  r28, #IR_OP_PEEK8
    bra     .csf_ext_peek_ready
.csf_ext_peek16:
    move.q  r28, #IR_OP_PEEK16
    bra     .csf_ext_peek_ready
.csf_ext_peek32:
    move.q  r28, #IR_OP_PEEK32
.csf_ext_peek_ready:
    add.q   r18, r18, #1
.csf_peek_body:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    push    r28
    push    r0
    jsr     compiler_semantic_expression
    pop     r1
    pop     r28
    bnez    r8, .csf_done
    move.q  r25, r23
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r25
    lsl.q   r14, r14, #16
    or.q    r14, r14, r23
    move.q  r15, r0
    move.q  r9, r28
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_RAM
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_memalloc:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .csf_done
    push    r23
    push    r0
    jsr     compiler_semantic_expect_comma
    bnez    r8, .csf_memalloc_pop_syntax
    jsr     compiler_semantic_expression
    move.q  r26, r23
    pop     r1
    pop     r25
    bnez    r8, .csf_done
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r23
    move.q  r15, r0
    move.q  r9, #IR_OP_MEMALLOC
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_memalloc_pop_syntax:
    pop     r1
    pop     r1
    bra     .csf_syntax
.csf_status_psg:
    move.q  r15, #PSG_PLAY_STATUS
    bra     .csf_status
.csf_status_sid:
    move.q  r15, #SID_PLAY_STATUS
    bra     .csf_status
.csf_status_sap:
    move.q  r15, #SAP_PLAY_STATUS
    bra     .csf_status
.csf_status_ted:
    move.q  r15, #TED_PLAY_STATUS
    bra     .csf_status
.csf_status_ahx:
    move.q  r15, #AHX_PLAY_STATUS
.csf_status:
    add.q   r18, r18, #1
    push    r15
    push    r0
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    or.q    r1, r1, #0x20
    move.q  r2, #0x73
    bne     r1, r2, .csf_status_pop_syntax
    jsr     compiler_semantic_skip_alpha
    pop     r1
    pop     r15
    jsr     compiler_semantic_append_const
    bnez    r8, .csf_done
    move.q  r25, r23
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r25
    lsl.q   r14, r14, #16
    or.q    r14, r14, r23
    move.q  r15, r0
    move.q  r9, #IR_OP_PEEK32
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_status_pop_syntax:
    pop     r1
    pop     r1
    bra     .csf_syntax
.csf_usr:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_expression
    bnez    r8, .csf_done
    move.q  r25, r23
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r25
    lsl.q   r14, r14, #16
    or.q    r14, r14, r23
    move.q  r15, r0
    move.q  r9, #IR_OP_USR
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_varptr:
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    jsr     compiler_semantic_name
    beqz    r28, .csf_syntax
    move.q  r25, r29
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_ADDRESS
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r23
    move.q  r15, r25
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_pi:
    li      r15, #0x400921FB54442D18
    bra     .csf_fp_constant
.csf_twopi:
    li      r15, #0x401921FB54442D18
.csf_fp_constant:
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_FP64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    sub.q   r13, r13, #1
    move.q  r14, r23
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    rts
.csf_numeric_fn:
    move.q  r28, r1
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x28
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    push    r28
    push    r0
    jsr     compiler_semantic_expression
    move.q  r25, r23
    pop     r1
    pop     r28
    bnez    r8, .csf_done
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x29
    bne     r1, r2, .csf_syntax
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r14, r25
    lsl.q   r14, r14, #16
    or.q    r14, r14, r23
    move.q  r15, r28
    move.q  r9, #IR_OP_NUMERIC_FN
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r1, #TK_RND
    bne     r28, r1, .csf_numeric_effect_ready
    move.q  r11, #IR_EFFECT_BASIC
.csf_numeric_effect_ready:
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
    rts
.csf_unary_plus:
    add.q   r18, r18, #1
    bra     compiler_semantic_factor
.csf_unary_minus:
    add.q   r18, r18, #1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r23
    move.q  r15, r0
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    bnez    r8, .csf_done
    push    r23
    push    r0
    jsr     compiler_semantic_factor
    move.q  r25, r23
    pop     r1
    pop     r23
    bnez    r8, .csf_done
    move.q  r26, #TK_MINUS
    bra     compiler_semantic_binary
.csf_syntax:
    move.q  r8, #2
    rts

; Parse and emit one unsigned decimal I64 constant.
compiler_semantic_number:
    jsr     compiler_semantic_spaces
    move.q  r15, r0
    move.q  r24, r18
.csn_digit:
    bge     r18, r19, .csn_end
    load.b  r1, (r18)
    move.q  r2, #0x30
    blt     r1, r2, .csn_end
    move.q  r2, #0x39
    bgt     r1, r2, .csn_end
    mulu.q  r2, r15, #10
    blt     r2, r15, .csn_overflow
    sub.q   r1, r1, #0x30
    add.q   r15, r2, r1
    blt     r15, r2, .csn_overflow
    add.q   r18, r18, #1
    bra     .csn_digit
.csn_end:
    load.b  r1, (r18)
    move.q  r2, #0x2E
    beq     r1, r2, .csn_fraction
    move.q  r2, #0x45
    beq     r1, r2, .csn_integer_exponent
    move.q  r2, #0x65
    beq     r1, r2, .csn_integer_exponent
    beq     r18, r24, .csn_syntax
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r24, r21
    move.q  r14, r23
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    rts
.csn_integer_exponent:
    beq     r18, r24, .csn_syntax
    dcvtif  f0, r15
    bra     .csn_exponent
.csn_fraction:
    add.q   r18, r18, #1
    move.q  r25, r0
    move.q  r26, #1
    move.q  r27, r18
.csn_fraction_digit:
    bge     r18, r19, .csn_fraction_end
    load.b  r1, (r18)
    move.q  r2, #0x30
    blt     r1, r2, .csn_fraction_end
    move.q  r2, #0x39
    bgt     r1, r2, .csn_fraction_end
    mulu.q  r2, r25, #10
    blt     r2, r25, .csn_overflow
    sub.q   r1, r1, #0x30
    add.q   r25, r2, r1
    blt     r25, r2, .csn_overflow
    mulu.q  r2, r26, #10
    blt     r2, r26, .csn_overflow
    move.q  r26, r2
    add.q   r18, r18, #1
    bra     .csn_fraction_digit
.csn_fraction_end:
    beq     r18, r27, .csn_syntax
    dcvtif  f0, r15
    dcvtif  f2, r25
    dcvtif  f4, r26
    ddiv    f2, f2, f4
    dadd    f0, f0, f2
.csn_fraction_exponent_check:
    load.b  r1, (r18)
    move.q  r2, #0x45
    beq     r1, r2, .csn_exponent
    move.q  r2, #0x65
    bne     r1, r2, .csn_store_fp
.csn_exponent:
    add.q   r18, r18, #1
    move.q  r27, r0
    load.b  r1, (r18)
    move.q  r2, #TK_MINUS
    beq     r1, r2, .csn_exponent_negative
    move.q  r2, #0x2D
    beq     r1, r2, .csn_exponent_negative
    move.q  r2, #TK_PLUS
    beq     r1, r2, .csn_exponent_positive
    move.q  r2, #0x2B
    bne     r1, r2, .csn_exponent_digits
.csn_exponent_positive:
    add.q   r18, r18, #1
    bra     .csn_exponent_digits
.csn_exponent_negative:
    move.q  r27, #1
    add.q   r18, r18, #1
.csn_exponent_digits:
    move.q  r25, r0
    move.q  r26, r18
.csn_exponent_digit:
    load.b  r1, (r18)
    move.q  r2, #0x30
    blt     r1, r2, .csn_exponent_apply
    move.q  r2, #0x39
    bgt     r1, r2, .csn_exponent_apply
    mulu.q  r2, r25, #10
    blt     r2, r25, .csn_overflow
    sub.q   r1, r1, #0x30
    add.q   r25, r2, r1
    blt     r25, r2, .csn_overflow
    add.q   r18, r18, #1
    bra     .csn_exponent_digit
.csn_exponent_apply:
    beq     r18, r26, .csn_syntax
    ; Repeated scaling preserves the interpreter's defined binary64 path.
    move.q  r1, #10
    dcvtif  f2, r1
    beqz    r25, .csn_store_fp
.csn_exponent_scale:
    bnez    r27, .csn_exponent_divide
    dmul    f0, f0, f2
    bra     .csn_exponent_scaled
.csn_exponent_divide:
    ddiv    f0, f0, f2
.csn_exponent_scaled:
    sub.q   r25, r25, #1
    bnez    r25, .csn_exponent_scale
.csn_store_fp:
    push    r0
    dstore  f0, (r31)
    load.q  r15, (r31)
    pop     r1
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_FP64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r24, r21
    move.q  r14, r23
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    rts
.csn_overflow:
    move.q  r8, #4
    rts
.csn_syntax:
    move.q  r8, #2
    rts

; Parse &H followed by one or more hexadecimal digits as an exact I64 value.
compiler_semantic_hex:
    add.q   r18, r18, #1
    load.b  r1, (r18)
    move.q  r2, #0x48
    beq     r1, r2, .csh_prefix
    move.q  r2, #0x68
    bne     r1, r2, .csh_syntax
.csh_prefix:
    add.q   r18, r18, #1
    move.q  r15, r0
    move.q  r24, r18
.csh_digit:
    load.b  r1, (r18)
    move.q  r2, #0x30
    blt     r1, r2, .csh_done
    move.q  r2, #0x39
    ble     r1, r2, .csh_decimal
    move.q  r2, #0x41
    blt     r1, r2, .csh_lower
    move.q  r2, #0x46
    ble     r1, r2, .csh_upper
.csh_lower:
    move.q  r2, #0x61
    blt     r1, r2, .csh_done
    move.q  r2, #0x66
    bgt     r1, r2, .csh_done
    sub.q   r1, r1, #0x57
    bra     .csh_accumulate
.csh_upper:
    sub.q   r1, r1, #0x37
    bra     .csh_accumulate
.csh_decimal:
    sub.q   r1, r1, #0x30
.csh_accumulate:
    lsr.q   r2, r15, #60
    bnez    r2, .csh_overflow
    lsl.q   r15, r15, #4
    or.q    r15, r15, r1
    add.q   r18, r18, #1
    bra     .csh_digit
.csh_done:
    beq     r18, r24, .csh_syntax
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r24, r21
    sub.q   r13, r13, #2
    move.q  r14, r23
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    rts
.csh_syntax:
    move.q  r8, #2
    rts
.csh_overflow:
    move.q  r8, #4
    rts

; Emit one compiler-created I64 constant. R15=value, returns its id in R23.
compiler_semantic_append_const:
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r14, r23
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    rts

; Consume a comma between hardware arguments. Returns R8=0 or syntax status.
compiler_semantic_skip_alpha:
    load.b  r1, (r18)
    or.q    r1, r1, #0x20
    move.q  r2, #0x61
    blt     r1, r2, .cssa_done
    move.q  r2, #0x7A
    bgt     r1, r2, .cssa_done
    add.q   r18, r18, #1
    bra     compiler_semantic_skip_alpha
.cssa_done:
    jsr     compiler_semantic_spaces
    rts

compiler_semantic_skip_word:
    load.b  r1, (r18)
    move.q  r2, #0x80
    blt     r1, r2, compiler_semantic_skip_alpha
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    rts

compiler_semantic_expect_comma:
    jsr     compiler_semantic_spaces
    load.b  r1, (r18)
    move.q  r2, #0x2C
    bne     r1, r2, .csec_syntax
    add.q   r18, r18, #1
    jsr     compiler_semantic_spaces
    move.q  r8, r0
    rts
.csec_syntax:
    move.q  r8, #2
    rts

; Parse R27 comma-separated expressions and write them to the ordered MMIO
; addresses at R28. The table contains full-width addresses.
compiler_semantic_mmio32_arguments:
    load.q  r15, (r28)
    push    r27
    push    r28
    jsr     compiler_semantic_mmio32_expression
    pop     r28
    pop     r27
    bnez    r8, .csma_done
    add.q   r28, r28, #8
    sub.q   r27, r27, #1
    beqz    r27, .csma_done
    jsr     compiler_semantic_expect_comma
    bnez    r8, .csma_done
    bra     compiler_semantic_mmio32_arguments
.csma_done:
    rts

compiler_blit_mode7_addresses:
    dc.q BLT_SRC, BLT_DST, BLT_WIDTH, BLT_HEIGHT
    dc.q BLT_MODE7_U0, BLT_MODE7_V0, BLT_MODE7_DU_COL, BLT_MODE7_DV_COL
    dc.q BLT_MODE7_DU_ROW, BLT_MODE7_DV_ROW, BLT_MODE7_TEX_W, BLT_MODE7_TEX_H

; Append an ordered 32-bit MMIO write. R15=address, R17=constant value.
compiler_semantic_mmio32_const:
    jsr     compiler_semantic_append_const
    bnez    r8, .csmc_done
    move.q  r25, r23
    move.q  r15, r17
    jsr     compiler_semantic_append_const
    bnez    r8, .csmc_done
    move.q  r26, r23
    bra     compiler_semantic_mmio32_record
.csmc_done:
    rts

; Append an ordered 32-bit MMIO write. R15=address, expression at R18.
compiler_semantic_mmio32_expression:
    jsr     compiler_semantic_append_const
    bnez    r8, compiler_semantic_mmio32_done
    push    r23
    push    r0
    jsr     compiler_semantic_expression
    pop     r1
    pop     r25
    bnez    r8, compiler_semantic_mmio32_done
    move.q  r26, r23
compiler_semantic_mmio32_record:
    move.q  r14, r26
    lsl.q   r14, r14, #32
    move.q  r1, r25
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    move.q  r15, r0
    move.q  r9, #IR_OP_POKE32
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_MMIO
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r7, r0
    jsr     compiler_ir_append
compiler_semantic_mmio32_done:
    rts

; Parse an unsigned decimal control-flow target into R15 without emitting IR.
compiler_semantic_uint:
    jsr     compiler_semantic_spaces
    move.q  r15, r0
    move.q  r24, r18
.csu_digit:
    load.b  r1, (r18)
    move.q  r2, #0x30
    blt     r1, r2, .csu_done
    move.q  r2, #0x39
    bgt     r1, r2, .csu_done
    mulu.q  r2, r15, #10
    blt     r2, r15, .csu_overflow
    sub.q   r1, r1, #0x30
    add.q   r15, r2, r1
    blt     r15, r2, .csu_overflow
    add.q   r18, r18, #1
    bra     .csu_digit
.csu_done:
    beq     r18, r24, .csu_syntax
    move.q  r8, r0
    rts
.csu_overflow:
    move.q  r8, #4
    rts
.csu_syntax:
    move.q  r8, #2
    rts

; Emit R23(lhs) R26(operator) R25(rhs), returning a new result in R23.
compiler_semantic_binary:
    move.q  r9, #IR_OP_ADD
    move.q  r1, #TK_PLUS
    beq     r26, r1, .csb_op_ready
    move.q  r9, #IR_OP_SUB
    move.q  r1, #TK_MINUS
    beq     r26, r1, .csb_op_ready
    move.q  r9, #IR_OP_MUL
    move.q  r1, #TK_MULT
    beq     r26, r1, .csb_op_ready
    move.q  r9, #IR_OP_LSHIFT
    move.q  r1, #TK_LSHIFT
    beq     r26, r1, .csb_op_ready
    move.q  r9, #IR_OP_RSHIFT
    move.q  r1, #TK_RSHIFT
    beq     r26, r1, .csb_op_ready
    move.q  r9, #IR_OP_DIV
.csb_op_ready:
    move.q  r14, r25
    lsl.q   r14, r14, #32
    move.q  r1, r23
    lsl.q   r1, r1, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r22
    move.q  r23, r22
    add.q   r22, r22, #1
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r1, #IR_OP_DIV
    bne     r9, r1, .csb_type_ready
    move.q  r10, #IR_TYPE_FP64
.csb_type_ready:
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r20
    sub.q   r13, r18, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    rts

; Classify a live token for typed parsing. R14=token. Returns R10=type,
; R11=effect and R7=flags while preserving R14.
compiler_classify_token:
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r7, r0
    move.q  r1, #TK_LOAD
    bne     r14, r1, .cct_not_load
    move.q  r7, #IR_FLAG_ARENA_ONLY
    rts
.cct_not_load:
    move.q  r1, #TK_PLUS
    blt     r14, r1, .cct_statements
    move.q  r1, #TK_LT
    bgt     r14, r1, .cct_functions
    move.q  r11, #IR_EFFECT_NONE
    rts
.cct_functions:
    move.q  r1, #TK_USR
    beq     r14, r1, .cct_full
    move.q  r1, #TK_PEEK
    beq     r14, r1, .cct_ram
    move.q  r1, #TK_RND
    beq     r14, r1, .cct_basic
    move.q  r1, #TK_MIDS
    bgt     r14, r1, .cct_hardware
    move.q  r11, #IR_EFFECT_BASIC
    rts
.cct_hardware:
    move.q  r1, #TK_SCREEN
    blt     r14, r1, .cct_statements
    move.q  r11, #IR_EFFECT_MMIO
    rts
.cct_statements:
    move.q  r1, #TK_LET
    beq     r14, r1, .cct_basic
    move.q  r1, #TK_DIM
    beq     r14, r1, .cct_basic
    move.q  r1, #TK_READ
    beq     r14, r1, .cct_basic
    move.q  r1, #TK_RESTORE
    beq     r14, r1, .cct_basic
    move.q  r1, #TK_GOTO
    beq     r14, r1, .cct_control
    move.q  r1, #TK_GOSUB
    beq     r14, r1, .cct_control
    move.q  r1, #TK_RETURN
    beq     r14, r1, .cct_control
    move.q  r1, #TK_IF
    beq     r14, r1, .cct_control
    move.q  r1, #TK_FOR
    beq     r14, r1, .cct_control
    move.q  r1, #TK_NEXT
    beq     r14, r1, .cct_control
    rts
.cct_basic:
    move.q  r11, #IR_EFFECT_BASIC
    rts
.cct_ram:
    move.q  r11, #IR_EFFECT_RAM
    rts
.cct_full:
    move.q  r11, #IR_EFFECT_FULL
    rts
.cct_control:
    move.q  r11, #IR_EFFECT_CONTROL
    rts

; Validate target-specific availability across the completed IR.
; R8=IR base, R9=target. Returns status, line and token offset.
compiler_validate_target:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r19, r1, .cvt_target_ok
    move.q  r1, #COMPILER_TARGET_STANDALONE
    bne     r19, r1, .cvt_invalid
.cvt_target_ok:
    la      r1, COMPILER_IR_CURSOR
    load.q  r20, (r1)
.cvt_record:
    bge     r18, r20, .cvt_ok
    move.q  r1, #COMPILER_TARGET_STANDALONE
    bne     r19, r1, .cvt_next
    load.q  r2, 56(r18)
    and.q   r2, r2, #IR_FLAG_ARENA_ONLY
    bnez    r2, .cvt_unavailable
.cvt_next:
    add.q   r18, r18, #IR_RECORD_BYTES
    bra     .cvt_record
.cvt_unavailable:
    move.q  r8, #3
    load.q  r9, 24(r18)
    load.q  r10, 32(r18)
    rts
.cvt_invalid:
    move.q  r8, #2
    move.q  r9, r0
    move.q  r10, r0
    rts
.cvt_ok:
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
