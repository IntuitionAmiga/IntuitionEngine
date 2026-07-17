; ehbasic_compiler_emit.inc - canonical IE64 assembly source emission

COMPILER_EMIT_ARRAYS equ 0x100
COMPILER_EMIT_NUMERIC equ 0x200
COMPILER_EMIT_TRUTHY equ 0x400
COMPILER_EMIT_DATA equ 0x800
COMPILER_EMIT_HEAP equ 0x1000
COMPILER_EMIT_COPPER equ 0x2000
COMPILER_EMIT_FILES equ 0x4000
COMPILER_EMIT_STRINGS equ 0x8000
COMPILER_EMIT_POKE equ 0x10000

; Emit a lowered stream as deterministic standalone source. R8=lowered base,
; R9=record count, R10=output start, R11=output end, R12=target, R13=arena
; link base. Returns R8=status, R9=length, R10=cursor.
compiler_emit_assembly:
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
    jsr     compiler_emit_assembly_inner
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
compiler_emit_assembly_inner:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r28, r8
    move.q  r29, r9
    move.q  r20, r10
    move.q  r21, r10
    move.q  r22, r11
    move.q  r23, r12
    move.q  r24, r13
    move.q  r1, #COMPILER_TARGET_STANDALONE
    bne     r23, r1, .cea_arena_header
    la      r9, compiler_emit_standalone_header
    jsr     compiler_emit_text
    bra     .cea_begin_records
.cea_arena_header:
    ; The private assembler receives its full-width link base out of band.
    ; Embedding an org directive would both duplicate that authority and make
    ; high arena links depend on a directive the private assembler rejects.
.cea_begin_records:
    move.q  r24, r0                 ; referenced-helper mask for this emission
.cea_records:
    beqz    r21, .cea_oom
    beqz    r19, .cea_variables
    load.q  r25, (r18)
    move.q  r1, #LOWER_OP_LABEL
    beq     r25, r1, .cea_label
    move.q  r1, #LOWER_OP_CONST
    beq     r25, r1, .cea_const
    move.q  r1, #LOWER_OP_ADDRESS
    beq     r25, r1, .cea_address
    move.q  r1, #LOWER_OP_ADD
    beq     r25, r1, .cea_add
    move.q  r1, #LOWER_OP_SUB
    beq     r25, r1, .cea_sub
    move.q  r1, #LOWER_OP_MUL
    beq     r25, r1, .cea_mul
    move.q  r1, #LOWER_OP_DIV
    beq     r25, r1, .cea_div
    move.q  r1, #LOWER_OP_LOAD_SCALAR
    beq     r25, r1, .cea_load
    move.q  r1, #LOWER_OP_STORE_SCALAR
    beq     r25, r1, .cea_store
    move.q  r1, #LOWER_OP_COMPARE_EQ
    beq     r25, r1, .cea_compare_eq
    move.q  r1, #LOWER_OP_COMPARE_NE
    beq     r25, r1, .cea_compare_ne
    move.q  r1, #LOWER_OP_COMPARE_LT
    beq     r25, r1, .cea_compare_lt
    move.q  r1, #LOWER_OP_COMPARE_LE
    beq     r25, r1, .cea_compare_le
    move.q  r1, #LOWER_OP_COMPARE_GT
    beq     r25, r1, .cea_compare_gt
    move.q  r1, #LOWER_OP_COMPARE_GE
    beq     r25, r1, .cea_compare_ge
    move.q  r1, #LOWER_OP_BRANCH
    beq     r25, r1, .cea_branch
    move.q  r1, #LOWER_OP_BRANCH_FALSE
    beq     r25, r1, .cea_branch_false
    move.q  r1, #LOWER_OP_GOSUB
    beq     r25, r1, .cea_gosub
    move.q  r1, #LOWER_OP_RETURN
    beq     r25, r1, .cea_return
    move.q  r1, #LOWER_OP_HALT
    beq     r25, r1, .cea_halt
    move.q  r1, #LOWER_OP_STOP
    beq     r25, r1, .cea_stop
    move.q  r1, #LOWER_OP_PRINT
    beq     r25, r1, .cea_print
    move.q  r1, #LOWER_OP_BRANCH_TRUE
    beq     r25, r1, .cea_branch_true
    move.q  r1, #LOWER_OP_FOR
    beq     r25, r1, .cea_for
    move.q  r1, #LOWER_OP_NEXT
    beq     r25, r1, .cea_next_loop
    move.q  r1, #LOWER_OP_DIM_ARRAY
    beq     r25, r1, .cea_dim_array
    move.q  r1, #LOWER_OP_BOUNDS_CHECK
    beq     r25, r1, .cea_bounds_check
    move.q  r1, #LOWER_OP_LOAD_ARRAY
    beq     r25, r1, .cea_load_array
    move.q  r1, #LOWER_OP_STORE_ARRAY
    beq     r25, r1, .cea_store_array
    move.q  r1, #LOWER_OP_PEEK8
    beq     r25, r1, .cea_peek8
    move.q  r1, #LOWER_OP_POKE8
    beq     r25, r1, .cea_poke8
    move.q  r1, #LOWER_OP_BITSET
    beq     r25, r1, .cea_bitset
    move.q  r1, #LOWER_OP_BITCLR
    beq     r25, r1, .cea_bitclr
    move.q  r1, #LOWER_OP_WHILE
    beq     r25, r1, .cea_while
    move.q  r1, #LOWER_OP_WEND
    beq     r25, r1, .cea_wend
    move.q  r1, #LOWER_OP_DO
    beq     r25, r1, .cea_next
    move.q  r1, #LOWER_OP_LOOP
    beq     r25, r1, .cea_loop
    move.q  r1, #LOWER_OP_ON_START
    beq     r25, r1, .cea_on_start
    move.q  r1, #LOWER_OP_ON_TARGET
    beq     r25, r1, .cea_on_target
    move.q  r1, #LOWER_OP_PEEK16
    beq     r25, r1, .cea_peek16
    move.q  r1, #LOWER_OP_POKE16
    beq     r25, r1, .cea_poke16
    move.q  r1, #LOWER_OP_PEEK32
    beq     r25, r1, .cea_peek32
    move.q  r1, #LOWER_OP_POKE32
    beq     r25, r1, .cea_poke32
    move.q  r1, #LOWER_OP_PEEK64
    beq     r25, r1, .cea_peek64
    move.q  r1, #LOWER_OP_POKE64
    beq     r25, r1, .cea_poke64
    move.q  r1, #LOWER_OP_WAIT
    beq     r25, r1, .cea_wait
    move.q  r1, #LOWER_OP_CALL
    beq     r25, r1, .cea_call
    move.q  r1, #LOWER_OP_USR
    beq     r25, r1, .cea_usr
    move.q  r1, #LOWER_OP_PRINT_STRING
    beq     r25, r1, .cea_print_string
    move.q  r1, #LOWER_OP_STORE_STRING_LITERAL
    beq     r25, r1, .cea_store_string_literal
    move.q  r1, #LOWER_OP_PRINT_STRING_VAR
    beq     r25, r1, .cea_print_string_var
    move.q  r1, #LOWER_OP_BIT_AND
    beq     r25, r1, .cea_bit_and
    move.q  r1, #LOWER_OP_BIT_XOR
    beq     r25, r1, .cea_bit_xor
    move.q  r1, #LOWER_OP_BIT_OR
    beq     r25, r1, .cea_bit_or
    move.q  r1, #LOWER_OP_BIT_NOT
    beq     r25, r1, .cea_bit_not
    move.q  r1, #LOWER_OP_POW
    beq     r25, r1, .cea_pow
    move.q  r1, #LOWER_OP_LSHIFT
    beq     r25, r1, .cea_lshift
    move.q  r1, #LOWER_OP_RSHIFT
    beq     r25, r1, .cea_rshift
    move.q  r1, #LOWER_OP_NUMERIC_FN
    beq     r25, r1, .cea_numeric_fn
    move.q  r1, #LOWER_OP_DATA
    beq     r25, r1, .cea_data
    move.q  r1, #LOWER_OP_READ_SCALAR
    beq     r25, r1, .cea_read_scalar
    move.q  r1, #LOWER_OP_RESTORE_DATA
    beq     r25, r1, .cea_restore_data
    move.q  r1, #LOWER_OP_BLIT_LENGTH
    beq     r25, r1, .cea_blit_length
    move.q  r1, #LOWER_OP_MEMALLOC
    beq     r25, r1, .cea_memalloc
    move.q  r1, #LOWER_OP_COPPER_LIST
    beq     r25, r1, .cea_copper_list
    move.q  r1, #LOWER_OP_COPPER_WAIT
    beq     r25, r1, .cea_copper_wait
    move.q  r1, #LOWER_OP_COPPER_MOVE
    beq     r25, r1, .cea_copper_move
    move.q  r1, #LOWER_OP_COPPER_END
    beq     r25, r1, .cea_copper_end
    move.q  r1, #LOWER_OP_BLOAD
    beq     r25, r1, .cea_bload
    move.q  r1, #LOWER_OP_SOUND_PLAY
    beq     r25, r1, .cea_sound_play
    move.q  r1, #LOWER_OP_STORED_LOAD
    beq     r25, r1, .cea_stored_load
    bra     .cea_unsupported
.cea_label:
    la      r9, compiler_emit_label
    jsr     compiler_emit_text
    load.q  r9, 16(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_const:
    la      r9, compiler_emit_const_low
    jsr     compiler_emit_text
    load.q  r6, 24(r18)
    move.q  r9, r6
    jsr     compiler_emit_hex32_uint
    la      r9, compiler_emit_const_high
    jsr     compiler_emit_text
    lsr.q   r9, r6, #32
    jsr     compiler_emit_hex32_uint
    load.q  r1, 8(r18)
    move.q  r2, #IR_TYPE_FP64
    beq     r1, r2, .cea_const_fp64
    la      r9, compiler_emit_push_i64
    jsr     compiler_emit_text
    bra     .cea_next
.cea_const_fp64:
    la      r9, compiler_emit_push_fp64
    jsr     compiler_emit_text
    bra     .cea_next
.cea_address:
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_address_arena
    la      r9, compiler_emit_address_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_address_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_address_arena:
    load.q  r8, 24(r18)
    jsr     compiler_native_var_payload
    move.q  r26, r8
    la      r9, compiler_emit_address_arena_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_address_arena_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_add:
    load.q  r1, 8(r18)
    move.q  r2, #IR_TYPE_I64
    bne     r1, r2, .cea_add_generic
    la      r9, compiler_emit_add
    jsr     compiler_emit_text
    bra     .cea_next
.cea_add_generic:
    la      r9, compiler_emit_numeric_add
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_sub:
    load.q  r1, 8(r18)
    move.q  r2, #IR_TYPE_I64
    bne     r1, r2, .cea_sub_generic
    la      r9, compiler_emit_sub
    jsr     compiler_emit_text
    bra     .cea_next
.cea_sub_generic:
    la      r9, compiler_emit_numeric_sub
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_mul:
    load.q  r1, 8(r18)
    move.q  r2, #IR_TYPE_I64
    bne     r1, r2, .cea_mul_generic
    la      r9, compiler_emit_mul
    jsr     compiler_emit_text
    bra     .cea_next
.cea_mul_generic:
    la      r9, compiler_emit_numeric_mul
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_div:
    la      r9, compiler_emit_numeric_div
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_load:
    load.q  r1, 8(r18)
    and.q   r2, r1, #LOWER_FLAG_RESIDENT
    bnez    r2, .cea_load_resident
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_load_arena
    la      r9, compiler_emit_var_la
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_load_tail
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_load_tag_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_load_arena:
    load.q  r8, 24(r18)
    jsr     compiler_native_var_payload
    move.q  r26, r8
    la      r9, compiler_emit_arena_load_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_arena_load_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_load_resident:
    and.q   r2, r1, #LOWER_FLAG_RESIDENT_HOT
    beqz    r2, .cea_load_resident_induction
    la      r9, compiler_emit_resident_hot_load
    jsr     compiler_emit_text
    bra     .cea_next
.cea_load_resident_induction:
    la      r9, compiler_emit_resident_induction_load
    jsr     compiler_emit_text
    bra     .cea_next
.cea_store:
    load.q  r1, 8(r18)
    and.q   r2, r1, #LOWER_FLAG_RESIDENT
    bnez    r2, .cea_store_resident
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_store_arena
    la      r9, compiler_emit_store_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_store_tail
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_store_tag_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_store_arena:
    load.q  r8, 24(r18)
    jsr     compiler_native_var_payload
    move.q  r26, r8
    la      r9, compiler_emit_arena_store_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_arena_store_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_store_resident:
    and.q   r2, r1, #LOWER_FLAG_RESIDENT_HOT
    beqz    r2, .cea_store_resident_induction
    la      r9, compiler_emit_resident_hot_store
    jsr     compiler_emit_text
    bra     .cea_next
.cea_store_resident_induction:
    la      r9, compiler_emit_resident_induction_store
    jsr     compiler_emit_text
    bra     .cea_next
.cea_compare_eq:
    move.q  r27, #4
    bra     .cea_compare
.cea_compare_ne:
    move.q  r27, #5
    bra     .cea_compare
.cea_compare_lt:
    move.q  r27, #6
    bra     .cea_compare
.cea_compare_le:
    move.q  r27, #7
    bra     .cea_compare
.cea_compare_gt:
    move.q  r27, #8
    bra     .cea_compare
.cea_compare_ge:
    move.q  r27, #9
.cea_compare:
    load.q  r1, 8(r18)
    and.q   r1, r1, #LOWER_FLAG_I64_OPERANDS
    beqz    r1, .cea_compare_generic
    la      r9, compiler_emit_i64_compare_head
    jsr     compiler_emit_text
    move.q  r1, #4
    beq     r27, r1, .cea_compare_i64_eq
    move.q  r1, #5
    beq     r27, r1, .cea_compare_i64_ne
    move.q  r1, #6
    beq     r27, r1, .cea_compare_i64_lt
    move.q  r1, #7
    beq     r27, r1, .cea_compare_i64_le
    move.q  r1, #8
    beq     r27, r1, .cea_compare_i64_gt
    la      r9, compiler_emit_i64_compare_ge
    bra     .cea_compare_i64_branch
.cea_compare_i64_eq:
    la      r9, compiler_emit_i64_compare_eq
    bra     .cea_compare_i64_branch
.cea_compare_i64_ne:
    la      r9, compiler_emit_i64_compare_ne
    bra     .cea_compare_i64_branch
.cea_compare_i64_lt:
    la      r9, compiler_emit_i64_compare_lt
    bra     .cea_compare_i64_branch
.cea_compare_i64_le:
    la      r9, compiler_emit_i64_compare_le
    bra     .cea_compare_i64_branch
.cea_compare_i64_gt:
    la      r9, compiler_emit_i64_compare_gt
.cea_compare_i64_branch:
    jsr     compiler_emit_text
    sub.q   r26, r18, r28
    lsr.q   r26, r26, #5
    add.q   r26, r26, #1
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_i64_compare_false
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_i64_compare_true
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_i64_compare_done
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_i64_compare_result
    jsr     compiler_emit_text
    bra     .cea_next
.cea_compare_generic:
    la      r9, compiler_emit_compare_mode
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_compare_call
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_branch:
    la      r9, compiler_emit_bra
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    bra     .cea_next
.cea_branch_true:
    la      r9, compiler_emit_branch_true_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_TRUTHY
    bra     .cea_next
.cea_branch_false:
    la      r9, compiler_emit_branch_false_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_TRUTHY
    bra     .cea_next
.cea_for:
    load.q  r1, 8(r18)
    and.q   r2, r1, #LOWER_FLAG_RESIDENT
    bnez    r2, .cea_for_resident
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    load.q  r1, 24(r18)
    move.q  r26, r1
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32
    lsr.q   r27, r1, #32
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r23, r1, .cea_for_static
    move.q  r8, r27
    jsr     compiler_native_var_payload
    move.q  r27, r8
    la      r9, compiler_emit_for_arena_head
    bra     .cea_for_head_ready
.cea_for_static:
    la      r9, compiler_emit_for_head
.cea_for_head_ready:
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r23, r1, .cea_for_store_static
    la      r9, compiler_emit_for_arena_store
    bra     .cea_for_store_ready
.cea_for_store_static:
    la      r9, compiler_emit_for_store
.cea_for_store_ready:
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_for_limit_store
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_for_step_store
    jsr     compiler_emit_text
    la      r9, compiler_emit_for_loop_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_for_resident:
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    load.q  r1, 24(r18)
    move.q  r26, r1
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32          ; FOR source line
    lsr.q   r27, r1, #32           ; induction symbol
    load.q  r25, 8(r18)
    lsr.q   r25, r25, #32          ; optional hot scalar symbol
    la      r9, compiler_emit_resident_for_head
    jsr     compiler_emit_text
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_for_resident_arena
    la      r9, compiler_emit_resident_for_static_var
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_static_tag
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_static_tag_tail
    jsr     compiler_emit_text
    bra     .cea_for_resident_hot
.cea_for_resident_arena:
    move.q  r8, r27
    jsr     compiler_native_var_payload
    move.q  r27, r8
    la      r9, compiler_emit_resident_for_arena_var
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_arena_var_tail
    jsr     compiler_emit_text
.cea_for_resident_hot:
    beqz    r25, .cea_for_resident_labels
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_for_resident_hot_arena
    la      r9, compiler_emit_resident_hot_static
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_hot_static_tag
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_hot_static_tag_tail
    jsr     compiler_emit_text
    bra     .cea_for_resident_labels
.cea_for_resident_hot_arena:
    move.q  r8, r25
    jsr     compiler_native_var_payload
    move.q  r25, r8
    la      r9, compiler_emit_resident_hot_arena
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_hot_arena_tail
    jsr     compiler_emit_text
.cea_for_resident_labels:
    la      r9, compiler_emit_resident_for_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_test
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_positive
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_body_branch
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_negative
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_body_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_for_body_name
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_next_loop:
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    load.q  r1, 24(r18)
    move.q  r26, r1
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32
    lsr.q   r27, r1, #32
    load.q  r1, 8(r18)
    and.q   r2, r1, #LOWER_FLAG_RESIDENT
    bnez    r2, .cea_next_resident
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r23, r1, .cea_next_static
    move.q  r8, r27
    jsr     compiler_native_var_payload
    move.q  r27, r8
    la      r9, compiler_emit_next_arena_head
    bra     .cea_next_head_ready
.cea_next_static:
    la      r9, compiler_emit_next_head
.cea_next_head_ready:
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r23, r1, .cea_next_step_static
    la      r9, compiler_emit_next_arena_step
    bra     .cea_next_step_ready
.cea_next_step_static:
    la      r9, compiler_emit_next_step
.cea_next_step_ready:
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r23, r1, .cea_next_store_static
    la      r9, compiler_emit_next_arena_store
    bra     .cea_next_store_ready
.cea_next_store_static:
    la      r9, compiler_emit_next_store
.cea_next_store_ready:
    jsr     compiler_emit_text
    la      r9, compiler_emit_next_limit
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_sign
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_positive
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_end_branch
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_exit_branch
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_negative_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_negative
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_next_end_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_next_resident:
    lsr.q   r25, r1, #32           ; optional hot scalar symbol
    la      r9, compiler_emit_resident_next
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_exit_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_next_resident_arena
    la      r9, compiler_emit_resident_flush_static
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_flush_static_tag
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_flush_static_tag_tail
    jsr     compiler_emit_text
    beqz    r25, .cea_next_resident_done
    la      r9, compiler_emit_resident_flush_hot_static
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_flush_hot_static_tag
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_flush_hot_static_tag_tail
    jsr     compiler_emit_text
    bra     .cea_next_resident_done
.cea_next_resident_arena:
    move.q  r8, r27
    jsr     compiler_native_var_payload
    move.q  r27, r8
    la      r9, compiler_emit_resident_flush_arena
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_flush_arena_tail
    jsr     compiler_emit_text
    beqz    r25, .cea_next_resident_done
    move.q  r8, r25
    jsr     compiler_native_var_payload
    move.q  r25, r8
    la      r9, compiler_emit_resident_flush_hot_arena
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_resident_flush_hot_arena_tail
    jsr     compiler_emit_text
.cea_next_resident_done:
    bra     .cea_next
.cea_dim_array:
    or.q    r24, r24, #COMPILER_EMIT_ARRAYS
    la      r9, compiler_emit_dim_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_dim_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_bounds_check:
    or.q    r24, r24, #COMPILER_EMIT_ARRAYS
    la      r9, compiler_emit_bounds_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_bounds_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_load_array:
    or.q    r24, r24, #COMPILER_EMIT_ARRAYS
    la      r9, compiler_emit_array_load_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_array_load_tail
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_array_load_tag_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_store_array:
    or.q    r24, r24, #COMPILER_EMIT_ARRAYS
    la      r9, compiler_emit_array_store_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_array_store_tail
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_array_store_tag_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_peek8:
    la      r9, compiler_emit_peek8
    jsr     compiler_emit_text
    bra     .cea_next
.cea_poke8:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_POKE)
    la      r9, compiler_emit_poke8
    jsr     compiler_emit_text
    bra     .cea_next
.cea_bitset:
    la      r9, compiler_emit_bitset
    jsr     compiler_emit_text
    bra     .cea_next
.cea_bitclr:
    la      r9, compiler_emit_bitclr
    jsr     compiler_emit_text
    bra     .cea_next
.cea_while:
    or.q    r24, r24, #COMPILER_EMIT_TRUTHY
    load.q  r26, 24(r18)
    beqz    r26, .cea_while_program_end
    la      r9, compiler_emit_while_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    bra     .cea_next
.cea_while_program_end:
    la      r9, compiler_emit_while_end
    jsr     compiler_emit_text
    bra     .cea_next
.cea_wend:
    la      r9, compiler_emit_wend_branch
    jsr     compiler_emit_text
    load.q  r26, 24(r18)
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_wend_label
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_loop:
    load.q  r1, 24(r18)
    move.q  r26, r1
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32
    lsr.q   r27, r1, #32
    beqz    r27, .cea_loop_unconditional
    or.q    r24, r24, #COMPILER_EMIT_TRUTHY
    move.q  r1, #1
    beq     r27, r1, .cea_loop_while
    la      r9, compiler_emit_loop_until
    bra     .cea_loop_condition
.cea_loop_while:
    la      r9, compiler_emit_loop_while
.cea_loop_condition:
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    bra     .cea_next
.cea_loop_unconditional:
    la      r9, compiler_emit_loop_branch
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    bra     .cea_next
.cea_on_start:
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    la      r9, compiler_emit_on_start
    jsr     compiler_emit_text
    bra     .cea_next
.cea_on_target:
    load.q  r1, 16(r18)
    move.q  r26, r1
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32          ; ordinal
    lsr.q   r27, r1, #32           ; source line
    load.q  r1, 24(r18)
    move.q  r6, r1
    lsl.q   r6, r6, #32
    lsr.q   r6, r6, #32            ; target line
    lsr.q   r7, r1, #32            ; 0 GOTO, 1 GOSUB
    la      r9, compiler_emit_on_compare
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_on_skip
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_underscore
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    beqz    r7, .cea_on_goto
    la      r9, compiler_emit_on_gosub
    bra     .cea_on_branch
.cea_on_goto:
    la      r9, compiler_emit_on_goto
.cea_on_branch:
    jsr     compiler_emit_text
    move.q  r9, r6
    jsr     compiler_emit_uint
    la      r9, compiler_emit_on_next_label
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_underscore
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_peek16:
    la      r9, compiler_emit_peek16
    jsr     compiler_emit_text
    bra     .cea_next
.cea_poke16:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_POKE)
    la      r9, compiler_emit_poke16
    jsr     compiler_emit_text
    bra     .cea_next
.cea_peek32:
    la      r9, compiler_emit_peek32
    jsr     compiler_emit_text
    bra     .cea_next
.cea_poke32:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_POKE)
    la      r9, compiler_emit_poke32
    jsr     compiler_emit_text
    bra     .cea_next
.cea_peek64:
    la      r9, compiler_emit_peek64
    jsr     compiler_emit_text
    bra     .cea_next
.cea_poke64:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_POKE)
    la      r9, compiler_emit_poke64
    jsr     compiler_emit_text
    bra     .cea_next
.cea_wait:
    load.q  r26, 24(r18)
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32
    la      r9, compiler_emit_wait_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_wait_test
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_wait_retry
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_wait_done
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_call:
    la      r9, compiler_emit_call
    jsr     compiler_emit_text
    bra     .cea_next
.cea_usr:
    la      r9, compiler_emit_usr
    jsr     compiler_emit_text
    bra     .cea_next
.cea_store_string_literal:
    or.q    r24, r24, #COMPILER_EMIT_STRINGS
    load.q  r1, 24(r18)
    move.q  r26, r1
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32          ; literal pointer
    lsr.q   r27, r1, #32           ; symbol
    load.q  r25, 16(r18)
    lsr.q   r25, r25, #32          ; length
    la      r9, compiler_emit_string_store_var_head
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    move.q  r7, r0
.cea_store_string_literal_byte:
    bge     r7, r25, .cea_store_string_literal_length
    la      r9, compiler_emit_string_char
    jsr     compiler_emit_text
    load.b  r9, (r26)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_store_offset
    jsr     compiler_emit_text
    move.q  r9, r7
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_store_offset_tail
    jsr     compiler_emit_text
    add.q   r26, r26, #1
    add.q   r7, r7, #1
    bra     .cea_store_string_literal_byte
.cea_store_string_literal_length:
    la      r9, compiler_emit_string_len_head
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_len_store
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_len_store_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_print_string_var:
    or.q    r24, r24, #COMPILER_EMIT_STRINGS
    load.q  r26, 24(r18)
    load.q  r27, 16(r18)
    lsr.q   r27, r27, #32
    la      r9, compiler_emit_string_print_var_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_print_len
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_print_mode
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_print_call
    jsr     compiler_emit_text
    bra     .cea_next
.cea_print_string:
    la      r9, compiler_emit_string_begin
    jsr     compiler_emit_text
    load.q  r26, 16(r18)
    move.q  r1, r26
    lsr.q   r6, r1, #32
    lsl.q   r26, r26, #32
    lsr.q   r26, r26, #32
    load.q  r27, 24(r18)
.cea_print_string_byte:
    beqz    r26, .cea_print_string_newline
    la      r9, compiler_emit_string_char
    jsr     compiler_emit_text
    load.b  r9, (r27)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_store
    jsr     compiler_emit_text
    add.q   r27, r27, #1
    sub.q   r26, r26, #1
    bra     .cea_print_string_byte
.cea_print_string_newline:
    beqz    r6, .cea_next
    move.q  r1, #2
    beq     r6, r1, .cea_print_string_tab
    la      r9, compiler_emit_string_newline
    jsr     compiler_emit_text
    bra     .cea_next
.cea_print_string_tab:
    la      r9, compiler_emit_string_tab
    jsr     compiler_emit_text
    bra     .cea_next
.cea_bit_and:
    move.q  r27, #10
    bra     .cea_bit_binary
.cea_bit_xor:
    move.q  r27, #11
    bra     .cea_bit_binary
.cea_bit_or:
    move.q  r27, #12
.cea_bit_binary:
    la      r9, compiler_emit_compare_mode
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_compare_call
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_bit_not:
    la      r9, compiler_emit_bit_not
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_pow:
    la      r9, compiler_emit_numeric_pow
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_lshift:
    move.q  r27, #15
    bra     .cea_shift
.cea_rshift:
    move.q  r27, #16
.cea_shift:
    load.q  r1, 8(r18)
    and.q   r1, r1, #LOWER_FLAG_I64_OPERANDS
    beqz    r1, .cea_shift_generic
    move.q  r1, #15
    beq     r27, r1, .cea_shift_i64_left
    la      r9, compiler_emit_i64_rshift
    jsr     compiler_emit_text
    bra     .cea_next
.cea_shift_i64_left:
    la      r9, compiler_emit_i64_lshift
    jsr     compiler_emit_text
    bra     .cea_next
.cea_shift_generic:
    la      r9, compiler_emit_compare_mode
    jsr     compiler_emit_text
    move.q  r9, r27
    jsr     compiler_emit_uint
    la      r9, compiler_emit_compare_call
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_numeric_fn:
    la      r9, compiler_emit_numeric_fn_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_compare_call
    jsr     compiler_emit_text
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    bra     .cea_next
.cea_data:
    or.q    r24, r24, #COMPILER_EMIT_DATA
    bra     .cea_next
.cea_read_scalar:
    or.q    r24, r24, #COMPILER_EMIT_DATA
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_read_scalar_arena
    la      r9, compiler_emit_read_head
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_read_store
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_read_tag_store
    jsr     compiler_emit_text
    bra     .cea_next
.cea_read_scalar_arena:
    load.q  r8, 24(r18)
    jsr     compiler_native_var_payload
    move.q  r26, r8
    la      r9, compiler_emit_read_arena_head
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_read_arena_store
    jsr     compiler_emit_text
    bra     .cea_next
.cea_restore_data:
    or.q    r24, r24, #COMPILER_EMIT_DATA
    la      r9, compiler_emit_restore_data
    jsr     compiler_emit_text
    bra     .cea_next
.cea_blit_length:
    or.q    r24, r24, #COMPILER_EMIT_NUMERIC
    la      r9, compiler_emit_blit_length
    jsr     compiler_emit_text
    bra     .cea_next
.cea_memalloc:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_HEAP)
    la      r9, compiler_emit_memalloc_head
    jsr     compiler_emit_text
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_memalloc_arena
    la      r9, compiler_emit_memalloc_cursor
    jsr     compiler_emit_text
    la      r9, compiler_emit_memalloc_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_memalloc_arena:
    la      r9, compiler_emit_memalloc_arena_head
    jsr     compiler_emit_text
    la      r26, basic_memalloc_public
    move.q  r9, r26
    lsl.q   r9, r9, #32
    lsr.q   r9, r9, #32
    jsr     compiler_emit_uint
    la      r9, compiler_emit_address_high
    jsr     compiler_emit_text
    move.q  r9, r26
    lsr.q   r9, r9, #32
    jsr     compiler_emit_uint
    la      r9, compiler_emit_memalloc_arena_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_copper_list:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_COPPER)
    la      r9, compiler_emit_copper_list
    jsr     compiler_emit_text
    bra     .cea_next
.cea_copper_wait:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_COPPER)
    la      r9, compiler_emit_copper_wait
    jsr     compiler_emit_text
    bra     .cea_next
.cea_copper_move:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_COPPER)
    la      r9, compiler_emit_copper_move
    jsr     compiler_emit_text
    bra     .cea_next
.cea_copper_end:
    or.q    r24, r24, #COMPILER_EMIT_COPPER
    la      r9, compiler_emit_copper_end
    jsr     compiler_emit_text
    bra     .cea_next
.cea_bload:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_FILES)
    la      r9, compiler_emit_bload_head
    jsr     compiler_emit_text
    load.q  r9, 16(r18)
    and.q   r9, r9, #0xFFFF
    jsr     compiler_emit_uint
    la      r9, compiler_emit_bload_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_stored_load:
    or.q    r24, r24, #COMPILER_EMIT_FILES
    la      r9, compiler_emit_stored_load_head
    jsr     compiler_emit_text
    load.q  r9, 16(r18)
    and.q   r9, r9, #0xFFFF
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stored_load_call
    jsr     compiler_emit_text
    la      r26, basic_load_file
    move.q  r9, r26
    lsl.q   r9, r9, #32
    lsr.q   r9, r9, #32
    jsr     compiler_emit_uint
    la      r9, compiler_emit_address_high
    jsr     compiler_emit_text
    move.q  r9, r26
    lsr.q   r9, r9, #32
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stored_load_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_sound_play:
    or.q    r24, r24, #(COMPILER_EMIT_NUMERIC | COMPILER_EMIT_FILES)
    la      r9, compiler_emit_sound_head
    jsr     compiler_emit_text
    load.q  r9, 16(r18)
    and.q   r9, r9, #0xFFFF
    jsr     compiler_emit_uint
    la      r9, compiler_emit_sound_tail
    jsr     compiler_emit_text
    bra     .cea_next
.cea_gosub:
    la      r9, compiler_emit_jsr
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    bra     .cea_next
.cea_return:
    load.q  r26, 16(r18)
    la      r9, compiler_emit_return_guard
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_return_error
    jsr     compiler_emit_text
    move.q  r9, r26
    lsr.q   r9, r9, #8
    jsr     compiler_emit_uint
    la      r9, compiler_emit_return_valid
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon_rts
    jsr     compiler_emit_text
    bra     .cea_next
.cea_halt:
    move.q  r1, #COMPILER_TARGET_STANDALONE
    beq     r23, r1, .cea_halt_standalone
    la      r9, compiler_emit_program_exit_branch
    jsr     compiler_emit_text
    bra     .cea_next
.cea_halt_standalone:
    la      r9, compiler_emit_halt
    jsr     compiler_emit_text
    bra     .cea_next
.cea_stop:
    move.q  r1, #COMPILER_TARGET_STANDALONE
    beq     r23, r1, .cea_halt
    load.q  r26, 16(r18)
    la      r9, compiler_emit_stop_skip
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_helper
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_capture
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_unwind
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_done_branch
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_done
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_capture_unwind
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_skip_helper
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_resume
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_call
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_stop_cont
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    bra     .cea_next
.cea_print:
    la      r9, compiler_emit_print_mode
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    lsr.q   r9, r9, #32
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    la      r9, compiler_emit_print
    jsr     compiler_emit_text
    or.q    r24, r24, #(1 << COMPILER_HELPER_PRINT_NUM)
.cea_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_records
.cea_variables:
    la      r9, compiler_emit_program_end_label
    jsr     compiler_emit_text
    move.q  r1, #COMPILER_TARGET_STANDALONE
    bne     r23, r1, .cea_variables_arena_end
    la      r9, compiler_emit_halt
    jsr     compiler_emit_text
    bra     .cea_variables_begin
.cea_variables_arena_end:
    la      r9, compiler_emit_program_exit
    jsr     compiler_emit_text
.cea_variables_begin:
    and.q   r1, r24, #(1 << COMPILER_HELPER_PRINT_NUM)
    beqz    r1, .cea_numeric_helper
    la      r9, compiler_emit_print_helper
    jsr     compiler_emit_text
.cea_numeric_helper:
    and.q   r1, r24, #COMPILER_EMIT_NUMERIC
    beqz    r1, .cea_truthy_helper
    la      r9, compiler_emit_numeric_helper
    jsr     compiler_emit_text
    la      r9, compiler_emit_rnd_state
    jsr     compiler_emit_text
.cea_poke_helper:
    and.q   r1, r24, #COMPILER_EMIT_POKE
    beqz    r1, .cea_truthy_helper
    la      r9, compiler_emit_poke_helper
    jsr     compiler_emit_text
.cea_truthy_helper:
    and.q   r1, r24, #COMPILER_EMIT_TRUTHY
    beqz    r1, .cea_string_helper
    la      r9, compiler_emit_truthy_helper
    jsr     compiler_emit_text
.cea_string_helper:
    and.q   r1, r24, #COMPILER_EMIT_STRINGS
    beqz    r1, .cea_variable_table
    la      r9, compiler_emit_string_var_helper
    jsr     compiler_emit_text
.cea_variable_table:
    and.q   r1, r24, #COMPILER_EMIT_ARRAYS
    beqz    r1, .cea_scalars
    la      r9, compiler_emit_bounds_error
    jsr     compiler_emit_text
.cea_scalars:
    move.q  r18, r28
    move.q  r19, r29
.cea_extra_scalar_scan:
    beqz    r19, .cea_fixed_scalars
    load.q  r1, (r18)
    move.q  r2, #LOWER_OP_LOAD_SCALAR
    beq     r1, r2, .cea_extra_scalar_direct
    move.q  r2, #LOWER_OP_STORE_SCALAR
    beq     r1, r2, .cea_extra_scalar_direct
    move.q  r2, #LOWER_OP_READ_SCALAR
    beq     r1, r2, .cea_extra_scalar_direct
    move.q  r2, #LOWER_OP_FOR
    beq     r1, r2, .cea_extra_scalar_loop
    move.q  r2, #LOWER_OP_NEXT
    bne     r1, r2, .cea_extra_scalar_next
.cea_extra_scalar_loop:
    load.q  r25, 24(r18)
    lsr.q   r25, r25, #32
    bra     .cea_extra_scalar_candidate
.cea_extra_scalar_direct:
    load.q  r25, 24(r18)
.cea_extra_scalar_candidate:
    move.q  r1, #0x41
    blt     r25, r1, .cea_extra_scalar_check_prior
    move.q  r1, #0x5A
    ble     r25, r1, .cea_extra_scalar_next
.cea_extra_scalar_check_prior:
    move.q  r6, r28
.cea_extra_scalar_prior:
    bge     r6, r18, .cea_extra_scalar_emit
    load.q  r1, (r6)
    move.q  r2, #LOWER_OP_LOAD_SCALAR
    beq     r1, r2, .cea_extra_scalar_prior_direct
    move.q  r2, #LOWER_OP_STORE_SCALAR
    beq     r1, r2, .cea_extra_scalar_prior_direct
    move.q  r2, #LOWER_OP_READ_SCALAR
    beq     r1, r2, .cea_extra_scalar_prior_direct
    move.q  r2, #LOWER_OP_FOR
    beq     r1, r2, .cea_extra_scalar_prior_loop
    move.q  r2, #LOWER_OP_NEXT
    bne     r1, r2, .cea_extra_scalar_prior_next
.cea_extra_scalar_prior_loop:
    load.q  r2, 24(r6)
    lsr.q   r2, r2, #32
    bra     .cea_extra_scalar_prior_compare
.cea_extra_scalar_prior_direct:
    load.q  r2, 24(r6)
.cea_extra_scalar_prior_compare:
    beq     r2, r25, .cea_extra_scalar_next
.cea_extra_scalar_prior_next:
    add.q   r6, r6, #LOWER_RECORD_BYTES
    bra     .cea_extra_scalar_prior
.cea_extra_scalar_emit:
    la      r9, compiler_emit_var_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_var_decl_tail
    jsr     compiler_emit_text
    la      r9, compiler_emit_var_tag_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_var_tag_tail
    jsr     compiler_emit_text
.cea_extra_scalar_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_extra_scalar_scan
.cea_fixed_scalars:
    move.q  r25, #0x41
.cea_variable:
    la      r9, compiler_emit_var_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_var_decl_tail
    jsr     compiler_emit_text
    la      r9, compiler_emit_var_tag_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_var_tag_tail
    jsr     compiler_emit_text
    add.q   r25, r25, #1
    move.q  r1, #0x5A
    ble     r25, r1, .cea_variable
.cea_string_declarations:
    move.q  r18, r28
    move.q  r19, r29
.cea_string_decl_scan:
    beqz    r19, .cea_arrays_begin
    load.q  r1, (r18)
    move.q  r2, #LOWER_OP_STORE_STRING_LITERAL
    beq     r1, r2, .cea_string_decl_store
    move.q  r2, #LOWER_OP_PRINT_STRING_VAR
    bne     r1, r2, .cea_string_decl_next
    load.q  r25, 24(r18)
    bra     .cea_string_decl_candidate
.cea_string_decl_store:
    load.q  r25, 24(r18)
    lsr.q   r25, r25, #32
.cea_string_decl_candidate:
    move.q  r6, r28
.cea_string_decl_prior:
    bge     r6, r18, .cea_string_decl_emit
    load.q  r1, (r6)
    move.q  r2, #LOWER_OP_STORE_STRING_LITERAL
    beq     r1, r2, .cea_string_decl_prior_store
    move.q  r2, #LOWER_OP_PRINT_STRING_VAR
    bne     r1, r2, .cea_string_decl_prior_next
    load.q  r2, 24(r6)
    bra     .cea_string_decl_prior_compare
.cea_string_decl_prior_store:
    load.q  r2, 24(r6)
    lsr.q   r2, r2, #32
.cea_string_decl_prior_compare:
    beq     r2, r25, .cea_string_decl_next
.cea_string_decl_prior_next:
    add.q   r6, r6, #LOWER_RECORD_BYTES
    bra     .cea_string_decl_prior
.cea_string_decl_emit:
    la      r9, compiler_emit_string_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_string_decl_tail
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_qzero
    jsr     compiler_emit_text
.cea_string_decl_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_string_decl_scan
.cea_arrays_begin:
    and.q   r1, r24, #COMPILER_EMIT_ARRAYS
    beqz    r1, .cea_for_data_begin
.cea_extra_array_begin:
    move.q  r18, r28
    move.q  r19, r29
.cea_extra_array_scan:
    beqz    r19, .cea_for_data_begin
    load.q  r1, (r18)
    move.q  r2, #LOWER_OP_DIM_ARRAY
    beq     r1, r2, .cea_extra_array_candidate
    move.q  r2, #LOWER_OP_BOUNDS_CHECK
    beq     r1, r2, .cea_extra_array_candidate
    move.q  r2, #LOWER_OP_LOAD_ARRAY
    beq     r1, r2, .cea_extra_array_candidate
    move.q  r2, #LOWER_OP_STORE_ARRAY
    bne     r1, r2, .cea_extra_array_next
.cea_extra_array_candidate:
    load.q  r25, 24(r18)
.cea_extra_array_check_prior:
    move.q  r6, r28
.cea_extra_array_prior:
    bge     r6, r18, .cea_extra_array_emit
    load.q  r1, (r6)
    move.q  r2, #LOWER_OP_DIM_ARRAY
    beq     r1, r2, .cea_extra_array_prior_symbol
    move.q  r2, #LOWER_OP_BOUNDS_CHECK
    beq     r1, r2, .cea_extra_array_prior_symbol
    move.q  r2, #LOWER_OP_LOAD_ARRAY
    beq     r1, r2, .cea_extra_array_prior_symbol
    move.q  r2, #LOWER_OP_STORE_ARRAY
    bne     r1, r2, .cea_extra_array_prior_next
.cea_extra_array_prior_symbol:
    load.q  r2, 24(r6)
    beq     r2, r25, .cea_extra_array_next
.cea_extra_array_prior_next:
    add.q   r6, r6, #LOWER_RECORD_BYTES
    bra     .cea_extra_array_prior
.cea_extra_array_emit:
    la      r9, compiler_emit_array_len_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_qzero
    jsr     compiler_emit_text
    la      r9, compiler_emit_array_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_array_decl_tail
    jsr     compiler_emit_text
    la      r9, compiler_emit_array_tag_decl
    jsr     compiler_emit_text
    move.q  r9, r25
    jsr     compiler_emit_uint
    la      r9, compiler_emit_array_decl_tail
    jsr     compiler_emit_text
.cea_extra_array_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_extra_array_scan
.cea_for_data_begin:
    and.q   r1, r24, #COMPILER_EMIT_FILES
    beqz    r1, .cea_data_begin
    la      r9, compiler_emit_file_error
    jsr     compiler_emit_text
    move.q  r18, r28
    move.q  r19, r29
.cea_file_table_scan:
    beqz    r19, .cea_data_begin
    load.q  r1, (r18)
    move.q  r2, #LOWER_OP_BLOAD
    beq     r1, r2, .cea_file_bload
    move.q  r2, #LOWER_OP_SOUND_PLAY
    beq     r1, r2, .cea_file_sound
    move.q  r2, #LOWER_OP_STORED_LOAD
    bne     r1, r2, .cea_file_table_next
    la      r9, compiler_emit_stored_load_name_head
    jsr     compiler_emit_text
    bra     .cea_file_name_label
.cea_file_sound:
    la      r9, compiler_emit_sound_name_head
    jsr     compiler_emit_text
    bra     .cea_file_name_label
.cea_file_bload:
    la      r9, compiler_emit_bload_name_head
    jsr     compiler_emit_text
.cea_file_name_label:
    load.q  r9, 16(r18)
    and.q   r9, r9, #0xFFFF
    jsr     compiler_emit_uint
    la      r9, compiler_emit_colon
    jsr     compiler_emit_text
    load.q  r26, 16(r18)
    lsr.q   r26, r26, #32
    load.q  r27, 24(r18)
.cea_file_name_byte:
    beqz    r26, .cea_file_name_end
    la      r9, compiler_emit_byte_directive
    jsr     compiler_emit_text
    load.b  r9, (r27)
    jsr     compiler_emit_uint
    la      r9, compiler_emit_newline
    jsr     compiler_emit_text
    add.q   r27, r27, #1
    sub.q   r26, r26, #1
    bra     .cea_file_name_byte
.cea_file_name_end:
    la      r9, compiler_emit_byte_zero
    jsr     compiler_emit_text
.cea_file_table_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_file_table_scan
.cea_data_begin:
    and.q   r1, r24, #COMPILER_EMIT_DATA
    beqz    r1, .cea_for_data_reset
    la      r9, compiler_emit_data_header
    jsr     compiler_emit_text
    move.q  r18, r28
    move.q  r19, r29
.cea_data_table_scan:
    beqz    r19, .cea_data_table_end
    load.q  r1, (r18)
    move.q  r2, #LOWER_OP_DATA
    bne     r1, r2, .cea_data_table_next
    la      r9, compiler_emit_data_item
    jsr     compiler_emit_text
    load.q  r9, 24(r18)
    jsr     compiler_emit_hex_uint
    load.q  r1, 8(r18)
    move.q  r2, #IR_TYPE_FP64
    beq     r1, r2, .cea_data_item_fp
    la      r9, compiler_emit_data_i64
    jsr     compiler_emit_text
    bra     .cea_data_table_next
.cea_data_item_fp:
    la      r9, compiler_emit_data_fp64
    jsr     compiler_emit_text
.cea_data_table_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_data_table_scan
.cea_data_table_end:
    la      r9, compiler_emit_data_footer
    jsr     compiler_emit_text
.cea_for_data_reset:
    move.q  r18, r28
    move.q  r19, r29
.cea_for_data_scan:
    beqz    r19, .cea_finish
    load.q  r1, (r18)
    move.q  r2, #LOWER_OP_FOR
    bne     r1, r2, .cea_for_data_next
    load.q  r1, 24(r18)
    lsl.q   r26, r1, #32
    lsr.q   r26, r26, #32
    la      r9, compiler_emit_for_limit_decl
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_typed_zero
    jsr     compiler_emit_text
    la      r9, compiler_emit_for_step_decl
    jsr     compiler_emit_text
    move.q  r9, r26
    jsr     compiler_emit_uint
    la      r9, compiler_emit_typed_zero
    jsr     compiler_emit_text
.cea_for_data_next:
    add.q   r18, r18, #LOWER_RECORD_BYTES
    sub.q   r19, r19, #1
    bra     .cea_for_data_scan
.cea_finish:
    and.q   r1, r24, #COMPILER_EMIT_COPPER
    beqz    r1, .cea_finish_heap
    la      r9, compiler_emit_copper_data
    jsr     compiler_emit_text
.cea_finish_heap:
    and.q   r1, r24, #COMPILER_EMIT_HEAP
    beqz    r1, .cea_finish_zero
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r23, r1, .cea_finish_arena_heap
    la      r9, compiler_emit_heap_data
    jsr     compiler_emit_text
    bra     .cea_finish_zero
.cea_finish_arena_heap:
    move.q  r1, #(BASIC_STATE + ST_PRIV_MEMALLOC_CURSOR)
    load.q  r2, (r1)
    bnez    r2, .cea_finish_arena_heap_ready
    move.q  r2, #BASIC_MEMALLOC_BASE0
    store.q r2, (r1)
.cea_finish_arena_heap_ready:
    la      r9, compiler_emit_arena_heap_data
    jsr     compiler_emit_text
.cea_finish_zero:
    beqz    r21, .cea_oom
    bge     r21, r22, .cea_oom
    store.b r0, (r21)
    sub.q   r9, r21, r20
    move.q  r10, r21
    move.q  r8, r0
    rts
.cea_unsupported:
    move.q  r8, #5
    move.q  r9, r0
    move.q  r10, r21
    rts
.cea_oom:
    move.q  r8, #1
    move.q  r9, r0
    move.q  r10, r0
    rts

; Append zero-terminated R9 at the shared R21 cursor within R22.
compiler_emit_text:
.cet_one:
    beqz    r21, .cet_fail
    load.b  r1, (r9)
    beqz    r1, .cet_done
    bge     r21, r22, .cet_fail
    store.b r1, (r21)
    add.q   r21, r21, #1
    add.q   r9, r9, #1
    bra     .cet_one
.cet_fail:
    move.q  r21, r0
.cet_done:
    rts

compiler_emit_char:
    beqz    r21, .cec_fail
    bge     r21, r22, .cec_fail
    store.b r1, (r21)
    add.q   r21, r21, #1
    rts
.cec_fail:
    move.q  r21, r0
    rts

; Append unsigned R9 in decimal.
compiler_emit_uint:
    bnez    r9, .ceu_convert
    move.q  r1, #0x30
    bra     compiler_emit_char
.ceu_convert:
    move.q  r2, r0
    move.q  r3, r9
.ceu_divide:
    beqz    r3, .ceu_digits
    move.q  r4, r3
    divu.q  r4, r4, #10
    mulu.q  r5, r4, #10
    sub.q   r5, r3, r5
    add.q   r5, r5, #0x30
    push    r5
    add.q   r2, r2, #1
    move.q  r3, r4
    bra     .ceu_divide
.ceu_digits:
    beqz    r2, .ceu_done
    pop     r1
    jsr     compiler_emit_char
    beqz    r21, .ceu_unwind
    sub.q   r2, r2, #1
    bra     .ceu_digits
.ceu_unwind:
    sub.q   r2, r2, #1
.ceu_unwind_one:
    beqz    r2, .ceu_done
    pop     r1
    sub.q   r2, r2, #1
    bra     .ceu_unwind_one
.ceu_done:
    rts

; Append one full-width hexadecimal R9 value. Fixed width keeps signed qword
; payloads and arbitrary FP64 bit patterns accepted by both assemblers.
compiler_emit_hex_uint:
    move.q  r2, #60
.ceh_digit:
    move.q  r3, r9
    lsr.q   r3, r3, r2
    and.q   r3, r3, #15
    move.q  r1, #9
    bgt     r3, r1, .ceh_alpha
    add.q   r1, r3, #0x30
    bra     .ceh_emit
.ceh_alpha:
    add.q   r1, r3, #0x37
.ceh_emit:
    jsr     compiler_emit_char
    sub.q   r2, r2, #4
    bgez    r2, .ceh_digit
    rts

; Append the low 32 bits of R9 as eight hexadecimal digits. This is used for
; the real move.l/movt pair accepted byte-identically by both assemblers.
compiler_emit_hex32_uint:
    move.q  r2, #28
.ceh32_digit:
    move.q  r3, r9
    lsr.q   r3, r3, r2
    and.q   r3, r3, #15
    move.q  r1, #9
    bgt     r3, r1, .ceh32_alpha
    add.q   r1, r3, #0x30
    bra     .ceh32_emit
.ceh32_alpha:
    add.q   r1, r3, #0x37
.ceh32_emit:
    jsr     compiler_emit_char
    sub.q   r2, r2, #4
    bgez    r2, .ceh32_digit
    rts

; Resolve a scalar symbol tag to its ordinary BASIC payload cell in the native
; variable segment prepared by the RUN AOT wrapper. R8=tag, returns R8=payload
; address or zero if the prescan did not intern the symbol.
compiler_native_var_payload:
    move.q  r7, r8
    la      r1, AOT_NATIVE_VAR_TAB
    load.q  r2, (r1)
    la      r1, AOT_NATIVE_VAR_COUNT
    load.q  r3, (r1)
    move.q  r4, r0
.cnvp_find:
    bge     r4, r3, .cnvp_missing
    lsl.q   r5, r4, #2
    add.q   r6, r2, r5
    load.l  r6, (r6)
    beq     r6, r7, .cnvp_found
    add.q   r4, r4, #1
    bra     .cnvp_find
.cnvp_found:
    move.q  r8, r4
    mulu.q  r8, r8, #24
    add.q   r8, r8, #(AOT_NATIVE_VAR_SEG + 24)
    rts
.cnvp_missing:
    move.q  r8, r0
    rts

; Return whether the legacy whole-programme prescan proved a scalar symbol I64.
; R8=tag, returns R8=0/1.
compiler_native_var_known_i64:
    move.q  r7, r8
    la      r1, AOT_NATIVE_VAR_TAB
    load.q  r2, (r1)
    la      r1, AOT_NATIVE_VAR_COUNT
    load.q  r3, (r1)
    move.q  r4, r0
.cnvki_find:
    bge     r4, r3, .cnvki_no
    lsl.q   r5, r4, #2
    add.q   r6, r2, r5
    load.l  r6, (r6)
    beq     r6, r7, .cnvki_found
    add.q   r4, r4, #1
    bra     .cnvki_find
.cnvki_found:
    add.q   r6, r2, #0x4000
    add.q   r6, r6, r4
    load.b  r8, (r6)
    rts
.cnvki_no:
    move.q  r8, r0
    rts

compiler_emit_standalone_header: dc.b "include \"ie64.inc\"",10,"org PROG_START",10,"start:",10,0
compiler_emit_label:             dc.b "L",0
compiler_emit_colon:             dc.b ":",10,0
compiler_emit_newline:           dc.b 10,0
compiler_emit_const_low:         dc.b "    move.l r8, #0x",0
compiler_emit_const_high:        dc.b 10,"    movt r8, #0x",0
compiler_emit_push_i64:          dc.b 10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_push_fp64:         dc.b 10,"    push r8",10,"    move.q r9, #VAL_F64",10,"    push r9",10,0
compiler_emit_add:               dc.b "    pop r4",10,"    pop r2",10,"    pop r3",10,"    pop r1",10,"    add.q r8, r1, r2",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_sub:               dc.b "    pop r4",10,"    pop r2",10,"    pop r3",10,"    pop r1",10,"    sub.q r8, r1, r2",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_mul:               dc.b "    pop r4",10,"    pop r2",10,"    pop r3",10,"    pop r1",10,"    mulu.q r8, r1, r2",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_numeric_add:       dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #0",10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_numeric_sub:       dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #1",10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_numeric_mul:       dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #2",10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_numeric_div:       dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #3",10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_numeric_pow:       dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #14",10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_var_la:            dc.b "    la r1, compiler_var_",0
compiler_emit_load_tail:         dc.b 10,"    load.q r8, (r1)",10,"    push r8",10,"    la r1, compiler_var_tag_",0
compiler_emit_load_tag_tail:     dc.b 10,"    load.q r9, (r1)",10,"    push r9",10,0
compiler_emit_store_head:        dc.b "    pop r9",10,"    pop r8",10,"    la r1, compiler_var_",0
compiler_emit_store_tail:        dc.b 10,"    store.q r8, (r1)",10,"    la r1, compiler_var_tag_",0
compiler_emit_store_tag_tail:    dc.b 10,"    store.q r9, (r1)",10,0
compiler_emit_arena_load_head:   dc.b "    move.q r1, #",0
compiler_emit_arena_load_tail:   dc.b 10,"    load.q r8, 8(r1)",10,"    push r8",10,"    load.q r9, (r1)",10,"    push r9",10,0
compiler_emit_arena_store_head:  dc.b "    pop r9",10,"    pop r8",10,"    move.q r1, #",0
compiler_emit_arena_store_tail:  dc.b 10,"    store.q r8, 8(r1)",10,"    store.q r9, (r1)",10,0
compiler_emit_resident_induction_load: dc.b "    push r18",10,"    push r19",10,0
compiler_emit_resident_hot_load: dc.b "    push r24",10,"    push r25",10,0
compiler_emit_resident_induction_store: dc.b "    pop r19",10,"    pop r18",10,0
compiler_emit_resident_hot_store: dc.b "    pop r25",10,"    pop r24",10,0
compiler_emit_resident_for_head: dc.b "    ; compiler resident induction",10,"    pop r23",10,"    pop r22",10,"    pop r21",10,"    pop r20",10,"    pop r19",10,"    pop r18",10,0
compiler_emit_resident_for_static_var: dc.b "    la r7, compiler_var_",0
compiler_emit_resident_for_static_tag: dc.b 10,"    store.q r18, (r7)",10,"    la r7, compiler_var_tag_",0
compiler_emit_resident_for_static_tag_tail: dc.b 10,"    store.q r19, (r7)",10,0
compiler_emit_resident_for_arena_var: dc.b "    move.q r7, #",0
compiler_emit_resident_for_arena_var_tail: dc.b 10,"    store.q r18, 8(r7)",10,"    store.q r19, (r7)",10,0
compiler_emit_resident_hot_static: dc.b "    ; compiler resident hot scalar",10,"    la r7, compiler_var_",0
compiler_emit_resident_hot_static_tag: dc.b 10,"    load.q r24, (r7)",10,"    la r7, compiler_var_tag_",0
compiler_emit_resident_hot_static_tag_tail: dc.b 10,"    load.q r25, (r7)",10,0
compiler_emit_resident_hot_arena: dc.b "    ; compiler resident hot scalar",10,"    move.q r7, #",0
compiler_emit_resident_hot_arena_tail: dc.b 10,"    load.q r24, 8(r7)",10,"    load.q r25, (r7)",10,0
compiler_emit_resident_for_label: dc.b "FRES",0
compiler_emit_resident_for_test: dc.b ":",10,"    move.q r1, r22",10,"    move.q r2, r23",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #6",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FRESNEG",0
compiler_emit_resident_for_positive: dc.b 10,"    move.q r1, r18",10,"    move.q r2, r19",10,"    move.q r3, r20",10,"    move.q r4, r21",10,"    move.q r5, #8",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FRESE",0
compiler_emit_resident_for_body_branch: dc.b 10,"    bra FRESBODY",0
compiler_emit_resident_for_negative: dc.b 10,"FRESNEG",0
compiler_emit_resident_for_body_label: dc.b ":",10,"    move.q r1, r18",10,"    move.q r2, r19",10,"    move.q r3, r20",10,"    move.q r4, r21",10,"    move.q r5, #6",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FRESE",0
compiler_emit_resident_for_body_name: dc.b 10,"FRESBODY",0
compiler_emit_resident_next: dc.b "    move.q r1, r18",10,"    move.q r2, r19",10,"    move.q r3, r22",10,"    move.q r4, r23",10,"    move.q r5, #0",10,"    jsr compiler_runtime_numeric",10,"    move.q r18, r8",10,"    move.q r19, r9",10,"    bra FRES",0
compiler_emit_resident_exit_label: dc.b 10,"FRESE",0
compiler_emit_resident_flush_static: dc.b "    la r7, compiler_var_",0
compiler_emit_resident_flush_static_tag: dc.b 10,"    store.q r18, (r7)",10,"    la r7, compiler_var_tag_",0
compiler_emit_resident_flush_static_tag_tail: dc.b 10,"    store.q r19, (r7)",10,0
compiler_emit_resident_flush_hot_static: dc.b "    la r7, compiler_var_",0
compiler_emit_resident_flush_hot_static_tag: dc.b 10,"    store.q r24, (r7)",10,"    la r7, compiler_var_tag_",0
compiler_emit_resident_flush_hot_static_tag_tail: dc.b 10,"    store.q r25, (r7)",10,0
compiler_emit_resident_flush_arena: dc.b "    move.q r7, #",0
compiler_emit_resident_flush_hot_arena: dc.b "    move.q r7, #",0
compiler_emit_resident_flush_arena_tail: dc.b 10,"    store.q r18, 8(r7)",10,"    store.q r19, (r7)",10,0
compiler_emit_resident_flush_hot_arena_tail: dc.b 10,"    store.q r24, 8(r7)",10,"    store.q r25, (r7)",10,0
compiler_emit_compare_mode:      dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #",0
compiler_emit_compare_call:      dc.b 10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_i64_compare_head:  dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,0
compiler_emit_i64_compare_eq:    dc.b "    beq r1, r3, CTRUE",0
compiler_emit_i64_compare_ne:    dc.b "    bne r1, r3, CTRUE",0
compiler_emit_i64_compare_lt:    dc.b "    blt r1, r3, CTRUE",0
compiler_emit_i64_compare_le:    dc.b "    ble r1, r3, CTRUE",0
compiler_emit_i64_compare_gt:    dc.b "    bgt r1, r3, CTRUE",0
compiler_emit_i64_compare_ge:    dc.b "    bge r1, r3, CTRUE",0
compiler_emit_i64_compare_false: dc.b 10,"    move.q r8, #0",10,"    bra CDONE",0
compiler_emit_i64_compare_true:  dc.b 10,"CTRUE",0
compiler_emit_i64_compare_done:  dc.b ":",10,"    move.q r8, #1",10,"CDONE",0
compiler_emit_i64_compare_result: dc.b ":",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_i64_lshift:        dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    and.q r3, r3, #63",10,"    lsl.q r8, r1, r3",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_i64_rshift:        dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    and.q r3, r3, #63",10,"    lsr.q r8, r1, r3",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_bit_not:           dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #13",10,"    jsr compiler_runtime_numeric",10,"    push r8",10,"    push r9",10,0
compiler_emit_numeric_fn_head:   dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #",0
compiler_emit_read_head:         dc.b "    move.q r1, #compiler_data_cursor",10,"    load.q r2, (r1)",10,"    move.q r3, #compiler_data_end",10,"    bge r2, r3, compiler_data_error",10,"    load.q r8, (r2)",10,"    load.q r9, 8(r2)",10,"    add.q r2, r2, #16",10,"    store.q r2, (r1)",10,"    move.q r1, #compiler_var_",0
compiler_emit_read_store:        dc.b 10,"    store.q r8, (r1)",10,"    la r1, compiler_var_tag_",0
compiler_emit_read_tag_store:    dc.b 10,"    store.q r9, (r1)",10,0
compiler_emit_read_arena_head:   dc.b "    move.q r1, #compiler_data_cursor",10,"    load.q r2, (r1)",10,"    move.q r3, #compiler_data_end",10,"    bge r2, r3, compiler_data_error",10,"    load.q r8, (r2)",10,"    load.q r9, 8(r2)",10,"    add.q r2, r2, #16",10,"    store.q r2, (r1)",10,"    move.q r1, #",0
compiler_emit_read_arena_store:  dc.b 10,"    store.q r8, 8(r1)",10,"    store.q r9, (r1)",10,0
compiler_emit_restore_data:      dc.b "    move.q r1, #compiler_data_cursor",10,"    move.q r2, #compiler_data_table",10,"    store.q r2, (r1)",10,0
compiler_emit_blit_length:       dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.l r1, #0xF002C",10,"    store.l r8, (r1)",10,"    move.l r1, #0xF0030",10,"    move.q r2, #1",10,"    store.l r2, (r1)",10,"    move.l r1, #0xF0034",10,"    store.l r8, (r1)",10,"    move.l r1, #0xF0038",10,"    store.l r8, (r1)",10,0
compiler_emit_memalloc_head:     dc.b "    pop r2",10,"    pop r1",10,"    pop r19",10,"    pop r18",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.q r20, r8",10,"    move.q r1, r18",10,"    move.q r2, r19",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.q r6, r8",10,0
compiler_emit_memalloc_cursor:   dc.b "    la r1, compiler_heap_cursor",10,0
compiler_emit_memalloc_tail:     dc.b "    load.q r8, (r1)",10,"    sub.q r2, r20, #1",10,"    add.q r8, r8, r2",10,"    sub.q r2, r0, r20",10,"    and.q r8, r8, r2",10,"    add.q r2, r8, r6",10,"    store.q r2, (r1)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_memalloc_arena_head: dc.b "    move.q r22, r6",10,"    move.q r23, r20",10,"    move.l r2, #",0
compiler_emit_memalloc_arena_tail: dc.b 10,"    jsr (r2)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_address_high:      dc.b 10,"    movt r2, #",0
compiler_emit_heap_data:        dc.b "compiler_heap_cursor: dc.q compiler_heap_start",10,"align 4096",10,"compiler_heap_start:",10,0
compiler_emit_arena_heap_data:  dc.b 0
compiler_emit_copper_list:      dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.l r1, #0xF0010",10,"    store.l r8, (r1)",10,"    move.q r1, #compiler_copper_cursor",10,"    store.q r8, (r1)",10,0
compiler_emit_copper_wait:      dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    lsl.l r8, r8, #12",10,"    move.q r1, #compiler_copper_cursor",10,"    load.q r2, (r1)",10,"    store.l r8, (r2)",10,"    add.q r2, r2, #4",10,"    store.q r2, (r1)",10,0
compiler_emit_copper_move:      dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.q r18, r8",10,"    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.l r1, #0xA0000",10,"    blt r8, r1, compiler_copper_error",10,"    lsr.l r8, r8, #2",10,"    or.l r8, r8, #0x80000000",10,"    move.q r1, #compiler_copper_cursor",10,"    load.q r2, (r1)",10,"    store.l r8, (r2)",10,"    add.q r2, r2, #4",10,"    move.l r3, #0x40000000",10,"    store.l r3, (r2)",10,"    add.q r2, r2, #4",10,"    store.l r18, (r2)",10,"    add.q r2, r2, #4",10,"    store.q r2, (r1)",10,0
compiler_emit_copper_end:       dc.b "    move.q r1, #compiler_copper_cursor",10,"    load.q r2, (r1)",10,"    move.l r3, #0xC0000000",10,"    store.l r3, (r2)",10,"    add.q r2, r2, #4",10,"    store.q r2, (r1)",10,0
compiler_emit_copper_data:      dc.b "compiler_copper_error: halt",10,"compiler_copper_cursor: dc.q 0",10,0
compiler_emit_bload_head:       dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    la r1, FILE_NAME_PTR",10,"    move.q r2, #compiler_bload_name_",0
compiler_emit_bload_tail:       dc.b 10,"    store.l r2, (r1)",10,"    la r1, FILE_DATA_PTR64",10,"    store.q r8, (r1)",10,"    la r1, FILE_READ_MAX",10,"    store.l r0, (r1)",10,"    la r1, FILE_CTRL",10,"    move.q r2, #FILE_OP_READ",10,"    store.l r2, (r1)",10,"    la r1, FILE_STATUS",10,"    load.l r2, (r1)",10,"    bnez r2, compiler_file_error",10,0
compiler_emit_file_error:       dc.b "compiler_file_error: halt",10,0
compiler_emit_bload_name_head:  dc.b "compiler_bload_name_",0
compiler_emit_byte_directive:   dc.b "    dc.b ",0
compiler_emit_byte_zero:        dc.b "    dc.b 0",10,0
compiler_emit_sound_head:       dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    la r1, MEDIA_SUBSONG",10,"    store.l r8, (r1)",10,"    la r1, MEDIA_NAME_PTR",10,"    move.q r2, #compiler_sound_name_",0
compiler_emit_sound_tail:       dc.b 10,"    store.l r2, (r1)",10,"    la r1, MEDIA_CTRL",10,"    move.q r2, #2",10,"    store.l r2, (r1)",10,"    move.q r2, #1",10,"    store.l r2, (r1)",10,0
compiler_emit_sound_name_head:  dc.b "compiler_sound_name_",0
compiler_emit_bra:               dc.b "    bra L",0
compiler_emit_branch_true_head:  dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_truthy",10,"    bnez r8, L",0
compiler_emit_branch_false_head: dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_truthy",10,"    beqz r8, L",0
compiler_emit_jsr:               dc.b "    jsr L",0
compiler_emit_rts:               dc.b "    rts",10,0
compiler_emit_address_head:      dc.b "    la r8, compiler_var_tag_",0
compiler_emit_address_tail:      dc.b 10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_address_arena_head: dc.b "    move.q r8, #",0
compiler_emit_address_arena_tail: dc.b 10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_colon_rts:         dc.b ":",10,"    rts",10,0
compiler_emit_program_exit_branch: dc.b "    bra compiler_program_exit",10,0
compiler_emit_program_exit:      dc.b "    bra compiler_program_exit",10,"compiler_program_exit:",10,"    jsr compiler_program_exit_capture",10,"compiler_program_exit_unwind:",10,"    move.l r6, #0x00042850",10,"    load.q r6, (r6)",10,"    sub.q r6, r6, #8",10,"    bge r31, r6, compiler_program_exit_done",10,"    store.q r7, (r31)",10,"    rts",10,"compiler_program_exit_done:",10,"    rts",10,"compiler_program_exit_capture:",10,"    pop r7",10,"    bra compiler_program_exit_unwind",10,0
compiler_emit_return_guard:      dc.b "    move.l r6, #0x00042850",10,"    load.q r6, (r6)",10,"    sub.q r6, r6, #8",10,"    blt r31, r6, G",0
compiler_emit_return_error:      dc.b 10,"    add.q r6, r16, #0x200",10,"    move.l r5, #",0
compiler_emit_return_valid:      dc.b 10,"    store.l r5, (r6)",10,"    move.l r6, #0x00042860",10,"    move.l r5, #5",10,"    store.q r5, (r6)",10,"    rts",10,"G",0
compiler_emit_halt:              dc.b "    halt",10,0
compiler_emit_print_mode:        dc.b "    move.q r3, #",0
compiler_emit_print:             dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_print_num",10,0
compiler_emit_var_decl:          dc.b "compiler_var_",0
compiler_emit_var_decl_tail:     dc.b ": dc.q 0",10,0
compiler_emit_var_tag_decl:      dc.b "compiler_var_tag_",0
compiler_emit_var_tag_tail:      dc.b ": dc.q VAL_I64",10,0
compiler_emit_for_head:          dc.b "    pop r6",10,"    pop r3",10,"    pop r5",10,"    pop r2",10,"    pop r4",10,"    pop r1",10,"    la r7, compiler_var_",0
compiler_emit_for_arena_head:    dc.b "    pop r6",10,"    pop r3",10,"    pop r5",10,"    pop r2",10,"    pop r4",10,"    pop r1",10,"    move.q r7, #",0
compiler_emit_for_store:         dc.b 10,"    store.q r1, (r7)",10,"    store.q r4, 8(r7)",10,"    move.q r7, #compiler_for_limit_",0
compiler_emit_for_arena_store:   dc.b 10,"    store.q r1, 8(r7)",10,"    store.q r4, (r7)",10,"    move.q r7, #compiler_for_limit_",0
compiler_emit_for_limit_store:   dc.b 10,"    store.q r2, (r7)",10,"    store.q r5, 8(r7)",10,"    move.q r7, #compiler_for_step_",0
compiler_emit_for_step_store:    dc.b 10,"    store.q r3, (r7)",10,"    store.q r6, 8(r7)",10,0
compiler_emit_for_loop_label:    dc.b 10,"F",0
compiler_emit_for_label_mid:     dc.b ":",10,"    move.q r7, #compiler_for_step_",0
compiler_emit_for_check_step:    dc.b 10,"    load.q r20, (r7)",10,"    load.q r21, 8(r7)",10,"    move.q r7, #compiler_for_limit_",0
compiler_emit_for_check_var:     dc.b 10,"    load.q r22, (r7)",10,"    load.q r23, 8(r7)",10,"    la r7, compiler_var_",0
compiler_emit_for_check_limit:   dc.b 10,"    load.q r18, (r7)",10,"    load.q r19, 8(r7)",10,"    move.q r1, r20",10,"    move.q r2, r21",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #6",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FNEG",0
compiler_emit_for_positive_end:  dc.b 10,"    move.q r1, r18",10,"    move.q r2, r19",10,"    move.q r3, r22",10,"    move.q r4, r23",10,"    move.q r5, #8",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FE",0
compiler_emit_for_body_branch:   dc.b 10,"    bra FBODY",0
compiler_emit_for_negative_label: dc.b 10,"FNEG",0
compiler_emit_for_negative_end:  dc.b ":",10,"    move.q r1, r18",10,"    move.q r2, r19",10,"    move.q r3, r22",10,"    move.q r4, r23",10,"    move.q r5, #6",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FE",0
compiler_emit_for_body_label:    dc.b 10,"FBODY",0
compiler_emit_next_head:         dc.b "    la r18, compiler_var_",0
compiler_emit_next_arena_head:   dc.b "    move.q r18, #",0
compiler_emit_next_step:         dc.b 10,"    load.q r1, (r18)",10,"    load.q r2, 8(r18)",10,"    move.q r7, #compiler_for_step_",0
compiler_emit_next_arena_step:   dc.b 10,"    load.q r1, 8(r18)",10,"    load.q r2, (r18)",10,"    move.q r7, #compiler_for_step_",0
compiler_emit_next_store:        dc.b 10,"    load.q r3, (r7)",10,"    load.q r4, 8(r7)",10,"    move.q r20, r3",10,"    move.q r21, r4",10,"    move.q r5, #0",10,"    jsr compiler_runtime_numeric",10,"    store.q r8, (r18)",10,"    store.q r9, 8(r18)",10,"    move.q r22, r8",10,"    move.q r23, r9",10,"    move.q r7, #compiler_for_limit_",0
compiler_emit_next_arena_store:  dc.b 10,"    load.q r3, (r7)",10,"    load.q r4, 8(r7)",10,"    move.q r20, r3",10,"    move.q r21, r4",10,"    move.q r5, #0",10,"    jsr compiler_runtime_numeric",10,"    store.q r8, 8(r18)",10,"    store.q r9, (r18)",10,"    move.q r22, r8",10,"    move.q r23, r9",10,"    move.q r7, #compiler_for_limit_",0
compiler_emit_next_limit:        dc.b 0
compiler_emit_next_sign:         dc.b 10,"    load.q r24, (r7)",10,"    load.q r25, 8(r7)",10,"    move.q r7, #compiler_for_step_",0
compiler_emit_next_positive:     dc.b 10,"    load.q r1, (r7)",10,"    load.q r2, 8(r7)",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #6",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, FNEXTNEG",0
compiler_emit_next_end_branch:   dc.b 10,"    move.q r1, r22",10,"    move.q r2, r23",10,"    move.q r3, r24",10,"    move.q r4, r25",10,"    move.q r5, #7",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, F",0
compiler_emit_next_exit_branch:  dc.b 10,"    bra FE",0
compiler_emit_next_negative_label: dc.b 10,"FNEXTNEG",0
compiler_emit_next_negative:     dc.b ":",10,"    move.q r1, r22",10,"    move.q r2, r23",10,"    move.q r3, r24",10,"    move.q r4, r25",10,"    move.q r5, #9",10,"    jsr compiler_runtime_numeric",10,"    bnez r8, F",0
compiler_emit_next_end_label:    dc.b 10,"FE",0
compiler_emit_for_limit_decl:    dc.b "compiler_for_limit_",0
compiler_emit_for_step_decl:     dc.b "compiler_for_step_",0
compiler_emit_qzero:             dc.b ": dc.q 0",10,0
compiler_emit_typed_zero:       dc.b ": dc.q 0, VAL_I64",10,0
compiler_emit_dim_head:          dc.b "    pop r2",10,"    pop r1",10,"    blt r1, r0, compiler_bounds_error",10,"    move.q r2, #4096",10,"    bgt r1, r2, compiler_bounds_error",10,"    move.q r2, #compiler_array_len_",0
compiler_emit_dim_tail:          dc.b 10,"    store.q r1, (r2)",10,0
compiler_emit_bounds_head:       dc.b "    load.q r1, 8(r31)",10,"    blt r1, r0, compiler_bounds_error",10,"    move.q r2, #compiler_array_len_",0
compiler_emit_bounds_tail:       dc.b 10,"    load.q r2, (r2)",10,"    bgt r1, r2, compiler_bounds_error",10,0
compiler_emit_array_load_head:   dc.b "    pop r3",10,"    pop r2",10,"    lsl.q r2, r2, #3",10,"    move.q r1, #compiler_array_",0
compiler_emit_array_load_tail:   dc.b 10,"    add.q r1, r1, r2",10,"    load.q r8, (r1)",10,"    push r8",10,"    move.q r1, #compiler_array_tag_",0
compiler_emit_array_load_tag_tail: dc.b 10,"    add.q r1, r1, r2",10,"    load.q r9, (r1)",10,"    push r9",10,0
compiler_emit_array_store_head:  dc.b "    pop r9",10,"    pop r8",10,"    pop r3",10,"    pop r2",10,"    lsl.q r2, r2, #3",10,"    move.q r1, #compiler_array_",0
compiler_emit_array_store_tail:  dc.b 10,"    add.q r1, r1, r2",10,"    store.q r8, (r1)",10,"    move.q r1, #compiler_array_tag_",0
compiler_emit_array_store_tag_tail: dc.b 10,"    add.q r1, r1, r2",10,"    store.q r9, (r1)",10,0
compiler_emit_peek8:             dc.b "    pop r2",10,"    pop r1",10,"    load.b r8, (r1)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_poke8:             dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #1",10,"    jsr compiler_runtime_poke",10,0
compiler_emit_bitset:            dc.b "    pop r4",10,"    pop r2",10,"    pop r3",10,"    pop r1",10,"    move.q r5, #1",10,"    lsl.q r5, r5, r2",10,"    load.b r6, (r1)",10,"    or.q r6, r6, r5",10,"    store.b r6, (r1)",10,0
compiler_emit_bitclr:            dc.b "    pop r4",10,"    pop r2",10,"    pop r3",10,"    pop r1",10,"    move.q r5, #1",10,"    lsl.q r5, r5, r2",10,"    not.q r5, r5",10,"    load.b r6, (r1)",10,"    and.q r6, r6, r5",10,"    store.b r6, (r1)",10,0
compiler_emit_while_head:        dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_truthy",10,"    beqz r8, L",0
compiler_emit_while_end:         dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_truthy",10,"    beqz r8, compiler_program_end",10,0
compiler_emit_wend_branch:       dc.b "    bra L",0
compiler_emit_wend_label:        dc.b 10,"WEND",0
compiler_emit_loop_branch:       dc.b "    bra L",0
compiler_emit_loop_while:        dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_truthy",10,"    bnez r8, L",0
compiler_emit_loop_until:        dc.b "    pop r2",10,"    pop r1",10,"    jsr compiler_runtime_truthy",10,"    beqz r8, L",0
compiler_emit_program_end_label: dc.b "compiler_program_end:",10,0
compiler_emit_on_start:          dc.b "    pop r2",10,"    pop r1",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.q r20, r8",10,0
compiler_emit_on_compare:        dc.b "    move.q r1, #",0
compiler_emit_on_skip:           dc.b 10,"    bne r20, r1, ONNEXT",0
compiler_emit_on_goto:           dc.b 10,"    bra L",0
compiler_emit_on_gosub:          dc.b 10,"    jsr L",0
compiler_emit_on_next_label:     dc.b 10,"ONNEXT",0
compiler_emit_underscore:        dc.b "_",0
compiler_emit_stop_skip:         dc.b "    bra P",0
compiler_emit_stop_helper:       dc.b 10,"V",0
compiler_emit_stop_capture:      dc.b ":",10,"    pop r5",10,"    move.l r6, #0x000428B8",10,"    store.q r5, (r6)",10,"    jsr W",0
compiler_emit_stop_unwind:       dc.b 10,"U",0
compiler_emit_stop_done_branch:  dc.b ":",10,"    move.l r6, #0x00042850",10,"    load.q r6, (r6)",10,"    sub.q r6, r6, #8",10,"    bge r31, r6, D",0
compiler_emit_stop_done:         dc.b 10,"    store.q r7, (r31)",10,"    rts",10,"D",0
compiler_emit_stop_capture_unwind: dc.b ":",10,"    rts",10,"W",0
compiler_emit_stop_skip_helper:  dc.b ":",10,"    pop r7",10,"    bra U",0
compiler_emit_stop_resume:       dc.b 10,"P",0
compiler_emit_stop_call:         dc.b ":",10,"    jsr V",0
compiler_emit_stop_cont:         dc.b 10,"C",0
compiler_emit_peek16:            dc.b "    pop r2",10,"    pop r1",10,"    load.w r8, (r1)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_poke16:            dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #2",10,"    jsr compiler_runtime_poke",10,0
compiler_emit_peek32:            dc.b "    pop r2",10,"    pop r1",10,"    load.l r8, (r1)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_poke32:            dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #4",10,"    jsr compiler_runtime_poke",10,0
compiler_emit_peek64:            dc.b "    pop r2",10,"    pop r1",10,"    load.q r8, (r1)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_poke64:            dc.b "    pop r4",10,"    pop r3",10,"    pop r2",10,"    pop r1",10,"    move.q r5, #8",10,"    jsr compiler_runtime_poke",10,0
compiler_emit_stored_load_head:  dc.b "    la r1, compiler_stored_load_name_",0
compiler_emit_stored_load_call:  dc.b 10,"    move.l r2, #",0
compiler_emit_stored_load_tail:  dc.b 10,"    jsr (r2)",10,"    beqz r8, compiler_program_end",10,0
compiler_emit_stored_load_name_head: dc.b "compiler_stored_load_name_",0
compiler_emit_poke_helper:       dc.b "compiler_runtime_poke:",10,"    push r3",10,"    push r4",10,"    push r5",10,"    move.q r6, #VAL_I64",10,"    beq r2, r6, compiler_poke_address_ready",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.q r1, r8",10,"compiler_poke_address_ready:",10,"    pop r5",10,"    pop r4",10,"    pop r3",10,"    push r1",10,"    push r5",10,"    move.q r6, #VAL_I64",10,"    beq r4, r6, compiler_poke_value_ready",10,"    move.q r1, r3",10,"    move.q r2, r4",10,"    move.q r3, #0",10,"    move.q r4, #VAL_I64",10,"    move.q r5, #191",10,"    jsr compiler_runtime_numeric",10,"    move.q r3, r8",10,"compiler_poke_value_ready:",10,"    pop r5",10,"    pop r1",10,"    move.q r6, #1",10,"    beq r5, r6, compiler_poke_byte",10,"    move.q r6, #2",10,"    beq r5, r6, compiler_poke_word",10,"    move.q r6, #4",10,"    beq r5, r6, compiler_poke_long",10,"    store.q r3, (r1)",10,"    rts",10,"compiler_poke_byte:",10,"    store.b r3, (r1)",10,"    rts",10,"compiler_poke_word:",10,"    store.w r3, (r1)",10,"    rts",10,"compiler_poke_long:",10,"    store.l r3, (r1)",10,"    rts",10,0
compiler_emit_wait_head:         dc.b "    pop r6",10,"    pop r3",10,"    pop r5",10,"    pop r2",10,"    pop r5",10,"    pop r1",10,"    move.q r4, #0x100000",10,"WAIT",0
compiler_emit_wait_test:         dc.b ":",10,"    load.l r5, (r1)",10,"    eor.l r5, r5, r3",10,"    and.l r5, r5, r2",10,"    bnez r5, WAITDONE",0
compiler_emit_wait_retry:        dc.b 10,"    sub.q r4, r4, #1",10,"    bnez r4, WAIT",0
compiler_emit_wait_done:         dc.b 10,"WAITDONE",0
compiler_emit_call:              dc.b "    pop r2",10,"    pop r1",10,"    jsr (r1)",10,0
compiler_emit_usr:               dc.b "    pop r2",10,"    pop r1",10,"    jsr (r1)",10,"    push r8",10,"    move.q r9, #VAL_I64",10,"    push r9",10,0
compiler_emit_string_begin:      dc.b "    move.l r6, #TERM_OUT",10,0
compiler_emit_string_char:       dc.b "    move.q r1, #",0
compiler_emit_string_store:      dc.b 10,"    store.b r1, (r6)",10,0
compiler_emit_string_newline:    dc.b "    move.q r1, #13",10,"    store.b r1, (r6)",10,"    move.q r1, #10",10,"    store.b r1, (r6)",10,0
compiler_emit_string_tab:        dc.b "    move.q r1, #9",10,"    store.b r1, (r6)",10,0
compiler_emit_string_store_var_head: dc.b "    la r6, compiler_string_",0
compiler_emit_string_store_offset: dc.b 10,"    store.b r1, ",0
compiler_emit_string_store_offset_tail: dc.b "(r6)",10,0
compiler_emit_string_len_head:   dc.b "    la r6, compiler_string_len_",0
compiler_emit_string_len_store:  dc.b 10,"    move.q r1, #",0
compiler_emit_string_len_store_tail: dc.b 10,"    store.q r1, (r6)",10,0
compiler_emit_string_print_var_head: dc.b "    la r1, compiler_string_",0
compiler_emit_string_print_len:  dc.b 10,"    la r2, compiler_string_len_",0
compiler_emit_string_print_mode: dc.b 10,"    load.q r2, (r2)",10,"    move.q r3, #",0
compiler_emit_string_print_call: dc.b 10,"    jsr compiler_runtime_print_string_var",10,0
compiler_emit_string_decl:       dc.b "compiler_string_",0
compiler_emit_string_decl_tail:  dc.b ": ds.q 32",10,"compiler_string_len_",0
compiler_emit_bounds_error:      dc.b "compiler_bounds_error:",10,"    halt",10,0
compiler_emit_array_len_decl:    dc.b "compiler_array_len_",0
compiler_emit_array_decl:        dc.b "compiler_array_",0
compiler_emit_array_tag_decl:    dc.b "compiler_array_tag_",0
compiler_emit_array_decl_tail:   dc.b ": ds.q 4097",10,0
compiler_emit_rnd_state:         dc.b "compiler_rnd_state: dc.l 305419896",10,0
compiler_emit_data_header:       dc.b "compiler_data_cursor: dc.q compiler_data_table",10,"compiler_data_table:",10,0
compiler_emit_data_item:         dc.b "    dc.q 0x",0
compiler_emit_data_i64:          dc.b ", VAL_I64",10,0
compiler_emit_data_fp64:         dc.b ", VAL_F64",10,0
compiler_emit_data_footer:       dc.b "compiler_data_end:",10,"compiler_data_error:",10,"    halt",10,0
compiler_emit_string_var_helper:
    dc.b "compiler_runtime_print_string_var:",10
    dc.b "    move.l r6, #TERM_OUT",10
    dc.b ".string_var_loop:",10
    dc.b "    beqz r2, .string_var_suffix",10
    dc.b "    load.b r4, (r1)",10
    dc.b "    store.b r4, (r6)",10
    dc.b "    add.q r1, r1, #1",10
    dc.b "    sub.q r2, r2, #1",10
    dc.b "    bra .string_var_loop",10
    dc.b ".string_var_suffix:",10
    dc.b "    beqz r3, .string_var_done",10
    dc.b "    move.q r4, #2",10
    dc.b "    beq r3, r4, .string_var_tab",10
    dc.b "    move.q r4, #13",10
    dc.b "    store.b r4, (r6)",10
    dc.b "    move.q r4, #10",10
    dc.b "    store.b r4, (r6)",10
    dc.b "    bra .string_var_done",10
    dc.b ".string_var_tab:",10
    dc.b "    move.q r4, #9",10
    dc.b "    store.b r4, (r6)",10
    dc.b ".string_var_done:",10
    dc.b "    rts",10,0
compiler_emit_print_helper:
    dc.b "compiler_runtime_print_num:",10
    dc.b "    move.q r11, r3",10
    dc.b "    move.q r3, #VAL_F64",10
    dc.b "    beq r2, r3, .print_float",10
    dc.b ".print_integer:",10
    dc.b "    move.l r6, #TERM_OUT",10
    dc.b "    bgez r1, .print_abs",10
    dc.b "    move.q r2, #45",10
    dc.b "    store.b r2, (r6)",10
    dc.b "    sub.q r1, r0, r1",10
    dc.b ".print_abs:",10
    dc.b "    bnez r1, .print_convert",10
    dc.b "    move.q r2, #48",10
    dc.b "    store.b r2, (r6)",10
    dc.b "    bra .print_newline",10
    dc.b ".print_convert:",10
    dc.b "    move.q r2, #0",10
    dc.b ".print_divide:",10
    dc.b "    move.q r3, r1",10
    dc.b "    divu.q r3, r3, #10",10
    dc.b "    mulu.q r4, r3, #10",10
    dc.b "    sub.q r4, r1, r4",10
    dc.b "    add.q r4, r4, #48",10
    dc.b "    push r4",10
    dc.b "    add.q r2, r2, #1",10
    dc.b "    move.q r1, r3",10
    dc.b "    bnez r1, .print_divide",10
    dc.b ".print_digits:",10
    dc.b "    pop r3",10
    dc.b "    store.b r3, (r6)",10
    dc.b "    sub.q r2, r2, #1",10
    dc.b "    bnez r2, .print_digits",10
    dc.b ".print_newline:",10
    dc.b "    beqz r11, .print_done",10
    dc.b "    move.q r3, #2",10
    dc.b "    bne r11, r3, .print_emit_newline",10
    dc.b "    move.q r2, #9",10
    dc.b "    store.b r2, (r6)",10
    dc.b "    bra .print_done",10
    dc.b ".print_emit_newline:",10
    dc.b "    move.q r2, #13",10
    dc.b "    store.b r2, (r6)",10
    dc.b "    move.q r2, #10",10
    dc.b "    store.b r2, (r6)",10
    dc.b ".print_done:",10
    dc.b "    move.q r10, #0",10
    dc.b "    rts",10
    dc.b ".print_float:",10
    dc.b "    move.l r6, #TERM_OUT",10
    dc.b "    push r1",10
    dc.b "    dload f0, (r31)",10
    dc.b "    pop r3",10
    dc.b "    lsr.q r3, r1, #63",10
    dc.b "    beqz r3, .print_float_abs",10
    dc.b "    move.q r3, #45",10
    dc.b "    store.b r3, (r6)",10
    dc.b ".print_float_abs:",10
    dc.b "    dabs f0, f0",10
    dc.b "    dcvtfi r1, f0",10
    dc.b "    move.q r7, #0",10
    dc.b "    move.q r3, r1",10
    dc.b "    bnez r3, .print_float_divide",10
    dc.b "    move.q r4, #48",10
    dc.b "    store.b r4, (r6)",10
    dc.b "    bra .print_float_fraction",10
    dc.b ".print_float_divide:",10
    dc.b "    move.q r4, r3",10
    dc.b "    divu.q r4, r4, #10",10
    dc.b "    mulu.q r5, r4, #10",10
    dc.b "    sub.q r5, r3, r5",10
    dc.b "    add.q r5, r5, #48",10
    dc.b "    push r5",10
    dc.b "    add.q r7, r7, #1",10
    dc.b "    move.q r3, r4",10
    dc.b "    bnez r3, .print_float_divide",10
    dc.b ".print_float_digits:",10
    dc.b "    pop r4",10
    dc.b "    store.b r4, (r6)",10
    dc.b "    sub.q r7, r7, #1",10
    dc.b "    bnez r7, .print_float_digits",10
    dc.b ".print_float_fraction:",10
    dc.b "    dcvtif f2, r1",10
    dc.b "    dsub f0, f0, f2",10
    dc.b "    dcvtif f2, r0",10
    dc.b "    dcmp r5, f0, f2",10
    dc.b "    beqz r5, .print_newline",10
    dc.b "    move.q r3, #46",10
    dc.b "    store.b r3, (r6)",10
    dc.b "    move.q r7, #6",10
    dc.b "    move.q r3, #10",10
    dc.b "    dcvtif f2, r3",10
    dc.b ".print_float_fraction_digit:",10
    dc.b "    dmul f0, f0, f2",10
    dc.b "    dcvtfi r3, f0",10
    dc.b "    add.q r4, r3, #48",10
    dc.b "    store.b r4, (r6)",10
    dc.b "    dcvtif f4, r3",10
    dc.b "    dsub f0, f0, f4",10
    dc.b "    sub.q r7, r7, #1",10
    dc.b "    beqz r7, .print_newline",10
    dc.b "    dcvtif f4, r0",10
    dc.b "    dcmp r5, f0, f4",10
    dc.b "    bnez r5, .print_float_fraction_digit",10
    dc.b "    bra .print_newline",10,0

; Mixed numeric operations. Inputs are lhs payload/type in R1/R2, rhs in
; R3/R4, and operation 0..9 in R5. Results use payload/type R8/R9.
compiler_emit_numeric_helper:
    dc.b "compiler_runtime_numeric:",10
    dc.b "    move.q r6, #100",10
    dc.b "    bge r5, r6, .numeric_unary",10
    dc.b "    move.q r6, #10",10
    dc.b "    blt r5, r6, .numeric_not_high",10
    dc.b "    move.q r6, #14",10
    dc.b "    beq r5, r6, .numeric_power",10
    dc.b "    bgt r5, r6, .numeric_shift",10
    dc.b "    bra .numeric_bitwise",10
    dc.b ".numeric_not_high:",10
    dc.b "    move.q r6, #3",10
    dc.b "    bgt r5, r6, .numeric_compare",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    beq r2, r6, .numeric_float",10
    dc.b "    beq r4, r6, .numeric_float",10
    dc.b "    move.q r6, #0",10
    dc.b "    beq r5, r6, .numeric_iadd",10
    dc.b "    move.q r6, #1",10
    dc.b "    beq r5, r6, .numeric_isub",10
    dc.b "    move.q r6, #2",10
    dc.b "    beq r5, r6, .numeric_imul",10
    dc.b "    bra .numeric_float",10
    dc.b ".numeric_iadd:",10
    dc.b "    add.q r8, r1, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_isub:",10
    dc.b "    sub.q r8, r1, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_imul:",10
    dc.b "    mulu.q r8, r1, r3",10
    dc.b ".numeric_integer_result:",10
    dc.b "    move.q r9, #VAL_I64",10
    dc.b "    move.q r10, #0",10
    dc.b "    rts",10
    dc.b ".numeric_bitwise:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    beq r2, r6, .numeric_bitwise_float",10
    dc.b "    beq r4, r6, .numeric_bitwise_float",10
    dc.b "    move.q r8, r1",10
    dc.b "    move.q r6, #10",10
    dc.b "    beq r5, r6, .numeric_iand",10
    dc.b "    move.q r6, #11",10
    dc.b "    beq r5, r6, .numeric_ixor",10
    dc.b "    move.q r6, #12",10
    dc.b "    beq r5, r6, .numeric_ior",10
    dc.b "    not.q r8, r8",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_iand:",10
    dc.b "    and.q r8, r8, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_ixor:",10
    dc.b "    eor.q r8, r8, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_ior:",10
    dc.b "    or.q r8, r8, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_bitwise_float:",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dcvtfi r1, f0",10
    dc.b "    dcvtfi r3, f2",10
    dc.b "    move.l r8, r1",10
    dc.b "    move.q r6, #10",10
    dc.b "    beq r5, r6, .numeric_fand",10
    dc.b "    move.q r6, #11",10
    dc.b "    beq r5, r6, .numeric_fxor",10
    dc.b "    move.q r6, #12",10
    dc.b "    beq r5, r6, .numeric_for",10
    dc.b "    not.l r8, r8",10
    dc.b "    bra .numeric_bitwise_float_result",10
    dc.b ".numeric_fand:",10
    dc.b "    and.l r8, r8, r3",10
    dc.b "    bra .numeric_bitwise_float_result",10
    dc.b ".numeric_fxor:",10
    dc.b "    eor.l r8, r8, r3",10
    dc.b "    bra .numeric_bitwise_float_result",10
    dc.b ".numeric_for:",10
    dc.b "    or.l r8, r8, r3",10
    dc.b ".numeric_bitwise_float_result:",10
    dc.b "    dcvtif f0, r8",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_power:",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dpow f0, f0, f2",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_shift:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    beq r2, r6, .numeric_shift_float",10
    dc.b "    beq r4, r6, .numeric_shift_float",10
    dc.b "    and.q r3, r3, #63",10
    dc.b "    move.q r8, r1",10
    dc.b "    move.q r6, #15",10
    dc.b "    beq r5, r6, .numeric_shift_left_i64",10
    dc.b "    lsr.q r8, r8, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_shift_left_i64:",10
    dc.b "    lsl.q r8, r8, r3",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_shift_float:",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dcvtfi r1, f0",10
    dc.b "    dcvtfi r3, f2",10
    dc.b "    and.q r3, r3, #31",10
    dc.b "    move.l r8, r1",10
    dc.b "    move.q r6, #15",10
    dc.b "    beq r5, r6, .numeric_shift_left_fp",10
    dc.b "    lsr.l r8, r8, r3",10
    dc.b "    bra .numeric_shift_fp_result",10
    dc.b ".numeric_shift_left_fp:",10
    dc.b "    lsl.l r8, r8, r3",10
    dc.b ".numeric_shift_fp_result:",10
    dc.b "    dcvtif f0, r8",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_unary:",10
    dc.b "    move.q r6, #190",10
    dc.b "    beq r5, r6, .numeric_sgn",10
    dc.b "    move.q r6, #191",10
    dc.b "    beq r5, r6, .numeric_int",10
    dc.b "    move.q r6, #192",10
    dc.b "    beq r5, r6, .numeric_abs",10
    dc.b "    move.q r6, #197",10
    dc.b "    beq r5, r6, .numeric_rnd",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    move.q r6, #196",10
    dc.b "    beq r5, r6, .numeric_sqr",10
    dc.b "    move.q r6, #198",10
    dc.b "    beq r5, r6, .numeric_log",10
    dc.b "    move.q r6, #199",10
    dc.b "    beq r5, r6, .numeric_exp",10
    dc.b "    move.q r6, #200",10
    dc.b "    beq r5, r6, .numeric_cos",10
    dc.b "    move.q r6, #201",10
    dc.b "    beq r5, r6, .numeric_sin",10
    dc.b "    move.q r6, #202",10
    dc.b "    beq r5, r6, .numeric_tan",10
    dc.b "    datan f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_sqr:",10
    dc.b "    dsqrt f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_log:",10
    dc.b "    dlog f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_exp:",10
    dc.b "    dexp f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_cos:",10
    dc.b "    dcos f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_sin:",10
    dc.b "    dsin f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_tan:",10
    dc.b "    dtan f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_sgn:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    bne r2, r6, .numeric_sgn_i64",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dcvtif f2, r0",10
    dc.b "    dcmp r8, f0, f2",10
    dc.b "    move.q r9, #VAL_I64",10
    dc.b "    move.q r10, #0",10
    dc.b "    rts",10
    dc.b ".numeric_sgn_i64:",10
    dc.b "    move.q r8, #0",10
    dc.b "    beqz r1, .numeric_integer_result",10
    dc.b "    move.q r8, #1",10
    dc.b "    bgtz r1, .numeric_integer_result",10
    dc.b "    move.q r8, r0",10
    dc.b "    not.q r8, r8",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_int:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    bne r2, r6, .numeric_unary_i64",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dcvtfi r8, f0",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_abs:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    beq r2, r6, .numeric_abs_fp",10
    dc.b "    move.q r8, r1",10
    dc.b "    bgez r8, .numeric_integer_result",10
    dc.b "    neg.q r8, r8",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_abs_fp:",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dabs f0, f0",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_rnd:",10
    dc.b "    move.q r6, #compiler_rnd_state",10
    dc.b "    load.l r8, (r6)",10
    dc.b "    mulu.l r8, r8, #1664525",10
    dc.b "    add.l r8, r8, #1013904223",10
    dc.b "    store.l r8, (r6)",10
    dc.b "    and.l r8, r8, #0x007FFFFF",10
    dc.b "    dcvtif f0, r8",10
    dc.b "    move.q r6, #8388608",10
    dc.b "    dcvtif f2, r6",10
    dc.b "    ddiv f0, f0, f2",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_unary_i64:",10
    dc.b "    move.q r8, r1",10
    dc.b "    bra .numeric_integer_result",10
    dc.b ".numeric_compare:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    beq r2, r6, .numeric_float_compare",10
    dc.b "    beq r4, r6, .numeric_float_compare",10
    dc.b "    sub.q r6, r1, r3",10
    dc.b "    bra .numeric_predicate",10
    dc.b ".numeric_float_compare:",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    dcmp r6, f0, f2",10
    dc.b "    bra .numeric_predicate",10
    dc.b ".numeric_float:",10
    dc.b "    jsr .numeric_load_floats",10
    dc.b "    move.q r6, #0",10
    dc.b "    beq r5, r6, .numeric_fadd",10
    dc.b "    move.q r6, #1",10
    dc.b "    beq r5, r6, .numeric_fsub",10
    dc.b "    move.q r6, #2",10
    dc.b "    beq r5, r6, .numeric_fmul",10
    dc.b "    ddiv f0, f0, f2",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_fadd:",10
    dc.b "    dadd f0, f0, f2",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_fsub:",10
    dc.b "    dsub f0, f0, f2",10
    dc.b "    bra .numeric_float_result",10
    dc.b ".numeric_fmul:",10
    dc.b "    dmul f0, f0, f2",10
    dc.b ".numeric_float_result:",10
    dc.b "    push r0",10
    dc.b "    dstore f0, (r31)",10
    dc.b "    load.q r8, (r31)",10
    dc.b "    pop r6",10
    dc.b "    move.q r9, #VAL_F64",10
    dc.b "    move.q r10, #0",10
    dc.b "    rts",10
    dc.b ".numeric_load_floats:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    bne r2, r6, .numeric_lhs_integer",10
    dc.b "    push r1",10
    dc.b "    dload f0, (r31)",10
    dc.b "    pop r6",10
    dc.b "    bra .numeric_rhs",10
    dc.b ".numeric_lhs_integer:",10
    dc.b "    dcvtif f0, r1",10
    dc.b ".numeric_rhs:",10
    dc.b "    move.q r6, #VAL_F64",10
    dc.b "    bne r4, r6, .numeric_rhs_integer",10
    dc.b "    push r3",10
    dc.b "    dload f2, (r31)",10
    dc.b "    pop r6",10
    dc.b "    rts",10
    dc.b ".numeric_rhs_integer:",10
    dc.b "    dcvtif f2, r3",10
    dc.b "    rts",10
    dc.b ".numeric_predicate:",10
    dc.b "    move.q r8, #0",10
    dc.b "    move.q r7, #4",10
    dc.b "    beq r5, r7, .numeric_eq",10
    dc.b "    move.q r7, #5",10
    dc.b "    beq r5, r7, .numeric_ne",10
    dc.b "    move.q r7, #6",10
    dc.b "    beq r5, r7, .numeric_lt",10
    dc.b "    move.q r7, #7",10
    dc.b "    beq r5, r7, .numeric_le",10
    dc.b "    move.q r7, #8",10
    dc.b "    beq r5, r7, .numeric_gt",10
    dc.b "    bgez r6, .numeric_true",10
    dc.b "    bra .numeric_bool",10
    dc.b ".numeric_eq:",10
    dc.b "    beqz r6, .numeric_true",10
    dc.b "    bra .numeric_bool",10
    dc.b ".numeric_ne:",10
    dc.b "    bnez r6, .numeric_true",10
    dc.b "    bra .numeric_bool",10
    dc.b ".numeric_lt:",10
    dc.b "    bltz r6, .numeric_true",10
    dc.b "    bra .numeric_bool",10
    dc.b ".numeric_le:",10
    dc.b "    blez r6, .numeric_true",10
    dc.b "    bra .numeric_bool",10
    dc.b ".numeric_gt:",10
    dc.b "    bgtz r6, .numeric_true",10
    dc.b "    bra .numeric_bool",10
    dc.b ".numeric_true:",10
    dc.b "    move.q r8, #1",10
    dc.b ".numeric_bool:",10
    dc.b "    move.q r9, #VAL_I64",10
    dc.b "    move.q r10, #0",10
    dc.b "    rts",10,0

compiler_emit_truthy_helper:
    dc.b "compiler_runtime_truthy:",10
    dc.b "    move.q r3, #VAL_F64",10
    dc.b "    bne r2, r3, .truthy_integer",10
    dc.b "    lsl.q r3, r1, #1",10
    dc.b "    move.q r8, #0",10
    dc.b "    beqz r3, .truthy_done",10
    dc.b "    move.q r8, #1",10
    dc.b "    bra .truthy_done",10
    dc.b ".truthy_integer:",10
    dc.b "    move.q r8, #0",10
    dc.b "    beqz r1, .truthy_done",10
    dc.b "    move.q r8, #1",10
    dc.b ".truthy_done:",10
    dc.b "    move.q r9, #VAL_I64",10
    dc.b "    move.q r10, #0",10
    dc.b "    rts",10,0
