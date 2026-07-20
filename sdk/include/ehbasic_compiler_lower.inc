; ehbasic_compiler_lower.inc - direct target-level lowering

LOWER_RECORD_BYTES equ 32
LOWER_OP_CONST     equ 1
LOWER_OP_ADD       equ 2
LOWER_OP_SUB       equ 3
LOWER_OP_MUL       equ 4
LOWER_OP_BRANCH    equ 5
LOWER_OP_DIV       equ 6
LOWER_OP_COMPARE_EQ equ 7
LOWER_OP_COMPARE_NE equ 8
LOWER_OP_COMPARE_LT equ 9
LOWER_OP_COMPARE_LE equ 10
LOWER_OP_COMPARE_GT equ 11
LOWER_OP_COMPARE_GE equ 12
LOWER_OP_LOAD_SCALAR equ 13
LOWER_OP_STORE_SCALAR equ 14
LOWER_OP_HALT      equ 15
LOWER_OP_GOSUB     equ 16
LOWER_OP_RETURN    equ 17
LOWER_OP_STOP      equ 18
LOWER_OP_LABEL     equ 19
LOWER_OP_PRINT     equ 20
LOWER_OP_BRANCH_TRUE equ 21
LOWER_OP_FOR       equ 22
LOWER_OP_NEXT      equ 23
LOWER_OP_DIM_ARRAY equ 24
LOWER_OP_BOUNDS_CHECK equ 25
LOWER_OP_LOAD_ARRAY equ 26
LOWER_OP_STORE_ARRAY equ 27
LOWER_OP_PEEK8      equ 28
LOWER_OP_POKE8      equ 29
LOWER_OP_PEEK16     equ 30
LOWER_OP_POKE16     equ 31
LOWER_OP_PEEK32     equ 32
LOWER_OP_POKE32     equ 33
LOWER_OP_PEEK64     equ 34
LOWER_OP_POKE64     equ 35
LOWER_OP_WAIT       equ 36
LOWER_OP_CALL       equ 37
LOWER_OP_USR        equ 38
LOWER_OP_PRINT_STRING equ 39
LOWER_OP_BIT_AND      equ 40
LOWER_OP_BIT_XOR      equ 41
LOWER_OP_BIT_OR       equ 42
LOWER_OP_BIT_NOT      equ 43
LOWER_OP_POW          equ 44
LOWER_OP_LSHIFT       equ 45
LOWER_OP_RSHIFT       equ 46
LOWER_OP_NUMERIC_FN   equ 47
LOWER_OP_DATA         equ 48
LOWER_OP_READ_SCALAR  equ 49
LOWER_OP_RESTORE_DATA equ 50
LOWER_OP_BRANCH_FALSE equ 51
LOWER_OP_BLIT_LENGTH  equ 52
LOWER_OP_MEMALLOC     equ 53
LOWER_OP_COPPER_LIST  equ 54
LOWER_OP_COPPER_WAIT  equ 55
LOWER_OP_COPPER_MOVE  equ 56
LOWER_OP_COPPER_END   equ 57
LOWER_OP_BLOAD        equ 58
LOWER_OP_SOUND_PLAY   equ 59
LOWER_OP_BITSET       equ 60
LOWER_OP_BITCLR       equ 61
LOWER_OP_WHILE        equ 62
LOWER_OP_WEND         equ 63
LOWER_OP_DO           equ 64
LOWER_OP_LOOP         equ 65
LOWER_OP_ON_START     equ 66
LOWER_OP_ON_TARGET    equ 67
LOWER_OP_STORE_STRING_LITERAL equ 68
LOWER_OP_PRINT_STRING_VAR equ 69
LOWER_OP_ADDRESS     equ 70
LOWER_OP_STORED_LOAD equ 71
LOWER_OP_PAD         equ 72
LOWER_OP_PADX        equ 73
LOWER_OP_PADY        equ 74
LOWER_FLAG_RESIDENT equ 0x100
LOWER_FLAG_RESIDENT_HOT equ 0x200
LOWER_FLAG_I64_OPERANDS equ 0x400

; R8=IR base, R9=output start, R10=output end, R11=target.
; Returns status/count/output cursor. Unknown semantic records fail with their
; source location and are never delegated to interpreter entry points.
compiler_lower_ir:
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
    jsr     compiler_lower_ir_inner
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
compiler_lower_ir_inner:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r20, r10
    move.q  r21, r0
    move.q  r23, r11
    move.q  r1, #COMPILER_TARGET_ARENA
    beq     r11, r1, .cli_target_ok
    move.q  r1, #COMPILER_TARGET_STANDALONE
    bne     r11, r1, .cli_invalid
.cli_target_ok:
    la      r1, COMPILER_IR_CURSOR
    load.q  r22, (r1)
.cli_one:
    bge     r18, r22, .cli_ok
    load.q  r1, 56(r18)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cli_next
    load.q  r1, (r18)
    move.q  r2, #IR_OP_BLOCK
    beq     r1, r2, .cli_label
    move.q  r2, #IR_OP_CONST
    beq     r1, r2, .cli_const
    move.q  r2, #IR_OP_ADDRESS
    beq     r1, r2, .cli_address
    move.q  r2, #IR_OP_ADD
    beq     r1, r2, .cli_add
    move.q  r2, #IR_OP_SUB
    beq     r1, r2, .cli_sub
    move.q  r2, #IR_OP_MUL
    beq     r1, r2, .cli_mul
    move.q  r2, #IR_OP_BRANCH
    beq     r1, r2, .cli_branch
    move.q  r2, #IR_OP_BRANCH_FALSE
    beq     r1, r2, .cli_branch_false
    move.q  r2, #IR_OP_DIV
    beq     r1, r2, .cli_div
    move.q  r2, #IR_OP_COMPARE_EQ
    beq     r1, r2, .cli_compare_eq
    move.q  r2, #IR_OP_COMPARE_NE
    beq     r1, r2, .cli_compare_ne
    move.q  r2, #IR_OP_COMPARE_LT
    beq     r1, r2, .cli_compare_lt
    move.q  r2, #IR_OP_COMPARE_LE
    beq     r1, r2, .cli_compare_le
    move.q  r2, #IR_OP_COMPARE_GT
    beq     r1, r2, .cli_compare_gt
    move.q  r2, #IR_OP_COMPARE_GE
    beq     r1, r2, .cli_compare_ge
    move.q  r2, #IR_OP_LOAD_SCALAR
    beq     r1, r2, .cli_load_scalar
    move.q  r2, #IR_OP_STORE_SCALAR
    beq     r1, r2, .cli_store_scalar
    move.q  r2, #IR_OP_HALT
    beq     r1, r2, .cli_halt
    move.q  r2, #IR_OP_GOSUB
    beq     r1, r2, .cli_gosub
    move.q  r2, #IR_OP_RETURN
    beq     r1, r2, .cli_return
    move.q  r2, #IR_OP_STOP
    beq     r1, r2, .cli_stop
    move.q  r2, #IR_OP_PRINT
    beq     r1, r2, .cli_print
    move.q  r2, #IR_OP_BRANCH_TRUE
    beq     r1, r2, .cli_branch_true
    move.q  r2, #IR_OP_FOR
    beq     r1, r2, .cli_for
    move.q  r2, #IR_OP_NEXT
    beq     r1, r2, .cli_next_loop
    move.q  r2, #IR_OP_DIM_ARRAY
    beq     r1, r2, .cli_dim_array
    move.q  r2, #IR_OP_BOUNDS_CHECK
    beq     r1, r2, .cli_bounds_check
    move.q  r2, #IR_OP_LOAD_ARRAY
    beq     r1, r2, .cli_load_array
    move.q  r2, #IR_OP_STORE_ARRAY
    beq     r1, r2, .cli_store_array
    move.q  r2, #IR_OP_PEEK8
    beq     r1, r2, .cli_peek8
    move.q  r2, #IR_OP_POKE8
    beq     r1, r2, .cli_poke8
    move.q  r2, #IR_OP_PEEK16
    beq     r1, r2, .cli_peek16
    move.q  r2, #IR_OP_POKE16
    beq     r1, r2, .cli_poke16
    move.q  r2, #IR_OP_PEEK32
    beq     r1, r2, .cli_peek32
    move.q  r2, #IR_OP_POKE32
    beq     r1, r2, .cli_poke32
    move.q  r2, #IR_OP_PEEK64
    beq     r1, r2, .cli_peek64
    move.q  r2, #IR_OP_PAD
    beq     r1, r2, .cli_pad
    move.q  r2, #IR_OP_PADX
    beq     r1, r2, .cli_padx
    move.q  r2, #IR_OP_PADY
    beq     r1, r2, .cli_pady
    move.q  r2, #IR_OP_POKE64
    beq     r1, r2, .cli_poke64
    move.q  r2, #IR_OP_WAIT
    beq     r1, r2, .cli_wait
    move.q  r2, #IR_OP_CALL
    beq     r1, r2, .cli_call
    move.q  r2, #IR_OP_USR
    beq     r1, r2, .cli_usr
    move.q  r2, #IR_OP_PRINT_STRING
    beq     r1, r2, .cli_print_string
    move.q  r2, #IR_OP_BIT_AND
    beq     r1, r2, .cli_bit_and
    move.q  r2, #IR_OP_BIT_XOR
    beq     r1, r2, .cli_bit_xor
    move.q  r2, #IR_OP_BIT_OR
    beq     r1, r2, .cli_bit_or
    move.q  r2, #IR_OP_BIT_NOT
    beq     r1, r2, .cli_bit_not
    move.q  r2, #IR_OP_POW
    beq     r1, r2, .cli_pow
    move.q  r2, #IR_OP_LSHIFT
    beq     r1, r2, .cli_lshift
    move.q  r2, #IR_OP_RSHIFT
    beq     r1, r2, .cli_rshift
    move.q  r2, #IR_OP_NUMERIC_FN
    beq     r1, r2, .cli_numeric_fn
    move.q  r2, #IR_OP_DATA
    beq     r1, r2, .cli_data
    move.q  r2, #IR_OP_READ_SCALAR
    beq     r1, r2, .cli_read_scalar
    move.q  r2, #IR_OP_RESTORE_DATA
    beq     r1, r2, .cli_restore_data
    move.q  r2, #IR_OP_BLIT_LENGTH
    beq     r1, r2, .cli_blit_length
    move.q  r2, #IR_OP_MEMALLOC
    beq     r1, r2, .cli_memalloc
    move.q  r2, #IR_OP_COPPER_LIST
    beq     r1, r2, .cli_copper_list
    move.q  r2, #IR_OP_COPPER_WAIT
    beq     r1, r2, .cli_copper_wait
    move.q  r2, #IR_OP_COPPER_MOVE
    beq     r1, r2, .cli_copper_move
    move.q  r2, #IR_OP_COPPER_END
    beq     r1, r2, .cli_copper_end
    move.q  r2, #IR_OP_BLOAD
    beq     r1, r2, .cli_bload
    move.q  r2, #IR_OP_SOUND_PLAY
    beq     r1, r2, .cli_sound_play
    move.q  r2, #IR_OP_BITSET
    beq     r1, r2, .cli_bitset
    move.q  r2, #IR_OP_BITCLR
    beq     r1, r2, .cli_bitclr
    move.q  r2, #IR_OP_WHILE
    beq     r1, r2, .cli_while
    move.q  r2, #IR_OP_WEND
    beq     r1, r2, .cli_wend
    move.q  r2, #IR_OP_DO
    beq     r1, r2, .cli_do
    move.q  r2, #IR_OP_LOOP
    beq     r1, r2, .cli_loop
    move.q  r2, #IR_OP_ON_START
    beq     r1, r2, .cli_on_start
    move.q  r2, #IR_OP_ON_TARGET
    beq     r1, r2, .cli_on_target
    move.q  r2, #IR_OP_STORE_STRING_LITERAL
    beq     r1, r2, .cli_store_string_literal
    move.q  r2, #IR_OP_PRINT_STRING_VAR
    beq     r1, r2, .cli_print_string_var
    move.q  r2, #IR_OP_STORED_LOAD
    beq     r1, r2, .cli_stored_load
    bra     .cli_unsupported
.cli_const:
    move.q  r3, #LOWER_OP_CONST
    bra     .cli_emit
.cli_address:
    move.q  r3, #LOWER_OP_ADDRESS
    bra     .cli_emit
.cli_label:
    move.q  r3, #LOWER_OP_LABEL
    bra     .cli_emit
.cli_add:
    move.q  r3, #LOWER_OP_ADD
    bra     .cli_emit
.cli_sub:
    move.q  r3, #LOWER_OP_SUB
    bra     .cli_emit
.cli_mul:
    move.q  r3, #LOWER_OP_MUL
    bra     .cli_emit
.cli_branch:
    move.q  r3, #LOWER_OP_BRANCH
    bra     .cli_emit
.cli_branch_false:
    move.q  r3, #LOWER_OP_BRANCH_FALSE
    bra     .cli_emit
.cli_div:
    move.q  r3, #LOWER_OP_DIV
    bra     .cli_emit
.cli_compare_eq:
    move.q  r3, #LOWER_OP_COMPARE_EQ
    bra     .cli_emit
.cli_compare_ne:
    move.q  r3, #LOWER_OP_COMPARE_NE
    bra     .cli_emit
.cli_compare_lt:
    move.q  r3, #LOWER_OP_COMPARE_LT
    bra     .cli_emit
.cli_compare_le:
    move.q  r3, #LOWER_OP_COMPARE_LE
    bra     .cli_emit
.cli_compare_gt:
    move.q  r3, #LOWER_OP_COMPARE_GT
    bra     .cli_emit
.cli_compare_ge:
    move.q  r3, #LOWER_OP_COMPARE_GE
    bra     .cli_emit
.cli_load_scalar:
    move.q  r3, #LOWER_OP_LOAD_SCALAR
    bra     .cli_emit
.cli_store_scalar:
    move.q  r3, #LOWER_OP_STORE_SCALAR
    bra     .cli_emit
.cli_halt:
    move.q  r3, #LOWER_OP_HALT
    bra     .cli_emit
.cli_gosub:
    move.q  r3, #LOWER_OP_GOSUB
    bra     .cli_emit
.cli_return:
    move.q  r3, #LOWER_OP_RETURN
    bra     .cli_emit
.cli_stop:
    move.q  r3, #LOWER_OP_STOP
    bra     .cli_emit
.cli_print:
    move.q  r3, #LOWER_OP_PRINT
    bra     .cli_emit
.cli_branch_true:
    move.q  r3, #LOWER_OP_BRANCH_TRUE
    bra     .cli_emit
.cli_for:
    move.q  r3, #LOWER_OP_FOR
    bra     .cli_emit
.cli_next_loop:
    move.q  r3, #LOWER_OP_NEXT
    bra     .cli_emit
.cli_dim_array:
    move.q  r3, #LOWER_OP_DIM_ARRAY
    bra     .cli_emit
.cli_bounds_check:
    move.q  r3, #LOWER_OP_BOUNDS_CHECK
    bra     .cli_emit
.cli_load_array:
    move.q  r3, #LOWER_OP_LOAD_ARRAY
    bra     .cli_emit
.cli_store_array:
    move.q  r3, #LOWER_OP_STORE_ARRAY
    bra     .cli_emit
.cli_peek8:
    move.q  r3, #LOWER_OP_PEEK8
    bra     .cli_emit
.cli_poke8:
    move.q  r3, #LOWER_OP_POKE8
    bra     .cli_emit
.cli_peek16:
    move.q  r3, #LOWER_OP_PEEK16
    bra     .cli_emit
.cli_poke16:
    move.q  r3, #LOWER_OP_POKE16
    bra     .cli_emit
.cli_peek32:
    move.q  r3, #LOWER_OP_PEEK32
    bra     .cli_emit
.cli_poke32:
    move.q  r3, #LOWER_OP_POKE32
    bra     .cli_emit
.cli_peek64:
    move.q  r3, #LOWER_OP_PEEK64
    bra     .cli_emit
.cli_pad:
    move.q  r3, #LOWER_OP_PAD
    bra     .cli_emit
.cli_padx:
    move.q  r3, #LOWER_OP_PADX
    bra     .cli_emit
.cli_pady:
    move.q  r3, #LOWER_OP_PADY
    bra     .cli_emit
.cli_poke64:
    move.q  r3, #LOWER_OP_POKE64
    bra     .cli_emit
.cli_wait:
    move.q  r3, #LOWER_OP_WAIT
    bra     .cli_emit
.cli_call:
    move.q  r3, #LOWER_OP_CALL
    bra     .cli_emit
.cli_usr:
    move.q  r3, #LOWER_OP_USR
    bra     .cli_emit
.cli_print_string:
    move.q  r3, #LOWER_OP_PRINT_STRING
    bra     .cli_emit
.cli_bit_and:
    move.q  r3, #LOWER_OP_BIT_AND
    bra     .cli_emit
.cli_bit_xor:
    move.q  r3, #LOWER_OP_BIT_XOR
    bra     .cli_emit
.cli_bit_or:
    move.q  r3, #LOWER_OP_BIT_OR
    bra     .cli_emit
.cli_bit_not:
    move.q  r3, #LOWER_OP_BIT_NOT
    bra     .cli_emit
.cli_pow:
    move.q  r3, #LOWER_OP_POW
    bra     .cli_emit
.cli_lshift:
    move.q  r3, #LOWER_OP_LSHIFT
    bra     .cli_emit
.cli_rshift:
    move.q  r3, #LOWER_OP_RSHIFT
    bra     .cli_emit
.cli_numeric_fn:
    move.q  r3, #LOWER_OP_NUMERIC_FN
    bra     .cli_emit
.cli_data:
    move.q  r3, #LOWER_OP_DATA
    bra     .cli_emit
.cli_read_scalar:
    move.q  r3, #LOWER_OP_READ_SCALAR
    bra     .cli_emit
.cli_restore_data:
    move.q  r3, #LOWER_OP_RESTORE_DATA
    bra     .cli_emit
.cli_blit_length:
    move.q  r3, #LOWER_OP_BLIT_LENGTH
    bra     .cli_emit
.cli_memalloc:
    move.q  r3, #LOWER_OP_MEMALLOC
    bra     .cli_emit
.cli_copper_list:
    move.q  r3, #LOWER_OP_COPPER_LIST
    bra     .cli_emit
.cli_copper_wait:
    move.q  r3, #LOWER_OP_COPPER_WAIT
    bra     .cli_emit
.cli_copper_move:
    move.q  r3, #LOWER_OP_COPPER_MOVE
    bra     .cli_emit
.cli_copper_end:
    move.q  r3, #LOWER_OP_COPPER_END
    bra     .cli_emit
.cli_bload:
    move.q  r3, #LOWER_OP_BLOAD
    bra     .cli_emit
.cli_sound_play:
    move.q  r3, #LOWER_OP_SOUND_PLAY
    bra     .cli_emit
.cli_bitset:
    move.q  r3, #LOWER_OP_BITSET
    bra     .cli_emit
.cli_bitclr:
    move.q  r3, #LOWER_OP_BITCLR
    bra     .cli_emit
.cli_while:
    move.q  r3, #LOWER_OP_WHILE
    bra     .cli_emit
.cli_wend:
    move.q  r3, #LOWER_OP_WEND
    bra     .cli_emit
.cli_do:
    move.q  r3, #LOWER_OP_DO
    bra     .cli_emit
.cli_loop:
    move.q  r3, #LOWER_OP_LOOP
    bra     .cli_emit
.cli_on_start:
    move.q  r3, #LOWER_OP_ON_START
    bra     .cli_emit
.cli_on_target:
    move.q  r3, #LOWER_OP_ON_TARGET
    bra     .cli_emit
.cli_store_string_literal:
    move.q  r3, #LOWER_OP_STORE_STRING_LITERAL
    bra     .cli_emit
.cli_print_string_var:
    move.q  r3, #LOWER_OP_PRINT_STRING_VAR
    bra     .cli_emit
.cli_stored_load:
    move.q  r1, #COMPILER_TARGET_ARENA
    bne     r23, r1, .cli_unsupported
    move.q  r3, #LOWER_OP_STORED_LOAD
.cli_emit:
    add.q   r4, r19, #LOWER_RECORD_BYTES
    bgt     r4, r20, .cli_oom
    store.q r3, (r19)
    load.q  r5, 8(r18)
    load.q  r6, 56(r18)
    and.q   r7, r6, #IR_FLAG_RESIDENT
    beqz    r7, .cli_type_flags_done
    or.q    r5, r5, #LOWER_FLAG_RESIDENT
    and.q   r7, r6, #IR_FLAG_RESIDENT_HOT
    beqz    r7, .cli_type_flags_done
    or.q    r5, r5, #LOWER_FLAG_RESIDENT_HOT
.cli_type_flags_done:
    and.q   r7, r6, #IR_FLAG_I64_OPERANDS
    beqz    r7, .cli_operand_flags_done
    or.q    r5, r5, #LOWER_FLAG_I64_OPERANDS
.cli_operand_flags_done:
    store.q r5, 8(r19)
    load.q  r5, 40(r18)
    store.q r5, 16(r19)
    load.q  r5, 48(r18)
    store.q r5, 24(r19)
    move.q  r19, r4
    add.q   r21, r21, #1
.cli_next:
    add.q   r18, r18, #IR_RECORD_BYTES
    bra     .cli_one
.cli_unsupported:
    move.q  r8, #5
    load.q  r9, 24(r18)
    load.q  r10, 32(r18)
    rts
.cli_oom:
    move.q  r8, #1
    load.q  r9, 24(r18)
    load.q  r10, 32(r18)
    rts
.cli_invalid:
    move.q  r8, #2
    move.q  r9, r0
    move.q  r10, r0
    rts
.cli_ok:
    move.q  r8, r0
    move.q  r9, r21
    move.q  r10, r19
    rts
