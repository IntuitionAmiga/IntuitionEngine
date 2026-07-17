; ehbasic_compiler_optimise.inc - correctness-preserving typed IR passes

; Optimise a completed linear IR in place. The initial implementation performs
; conservative I64 constant/type propagation. R8=IR base. Returns R8=status,
; R9=line and R10=token offset. Status 4 is a compile-time numeric overflow.
compiler_optimise_ir:
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
    jsr     compiler_optimise_ir_inner
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
compiler_optimise_ir_inner:
    move.q  r18, r8
    move.q  r19, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r20, (r1)
.coi_record:
    bge     r19, r20, .coi_ok
    load.q  r1, (r19)
    move.q  r2, #IR_OP_ADD
    beq     r1, r2, .coi_fold_integer
    move.q  r2, #IR_OP_SUB
    beq     r1, r2, .coi_fold_integer
    move.q  r2, #IR_OP_MUL
    beq     r1, r2, .coi_fold_integer
    move.q  r2, #IR_OP_DIV
    beq     r1, r2, .coi_fold_integer
    move.q  r2, #IR_OP_COMPARE_EQ
    blt     r1, r2, .coi_not_comparison
    move.q  r2, #IR_OP_COMPARE_GE
    ble     r1, r2, .coi_fold_integer
.coi_not_comparison:
    move.q  r2, #IR_OP_MOVE
    beq     r1, r2, .coi_propagate_copy
    move.q  r2, #IR_OP_CONVERT
    beq     r1, r2, .coi_propagate_copy
.coi_next:
    add.q   r19, r19, #IR_RECORD_BYTES
    bra     .coi_record
.coi_fold_integer:
    load.q  r3, 40(r19)
    lsr.q   r4, r3, #16
    and.q   r4, r4, #IR_VALUE_MASK
    lsr.q   r5, r3, #32
    and.q   r5, r5, #IR_VALUE_MASK
    move.q  r6, r18
    move.q  r11, r0
    move.q  r12, r0
    move.q  r13, r0
    move.q  r14, r0
    move.q  r25, r0                ; latest constant definition for lhs
    move.q  r26, r0                ; latest constant definition for rhs
    move.q  r9, r0
    move.q  r10, r0
.coi_find_values:
    bge     r6, r19, .coi_values_found
    load.q  r1, 56(r6)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .coi_find_next
    load.q  r1, (r6)
    move.q  r2, #IR_OP_CONST
    blt     r1, r2, .coi_find_next
    move.q  r3, #IR_OP_CONVERT
    ble     r1, r3, .coi_definition
    move.q  r3, #IR_OP_ADDRESS
    blt     r1, r3, .coi_find_next
    move.q  r3, #IR_OP_LOAD_SCALAR
    ble     r1, r3, .coi_definition
    move.q  r3, #IR_OP_DIV
    blt     r1, r3, .coi_find_next
    move.q  r3, #IR_OP_COMPARE_GE
    ble     r1, r3, .coi_definition
    move.q  r3, #IR_OP_LOAD_ARRAY
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_PEEK8
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_PEEK16
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_PEEK32
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_PEEK64
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_CALL
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_USR
    beq     r1, r3, .coi_definition
    move.q  r3, #IR_OP_BIT_AND
    blt     r1, r3, .coi_find_next
    move.q  r3, #IR_OP_NUMERIC_FN
    ble     r1, r3, .coi_definition
    move.q  r3, #IR_OP_MEMALLOC
    bne     r1, r3, .coi_find_next
.coi_definition:
    load.q  r2, 40(r6)
    and.q   r2, r2, #IR_VALUE_MASK
    bne     r2, r4, .coi_check_rhs
    move.q  r9, r0
    move.q  r25, r0
    load.q  r13, 8(r6)
    move.q  r3, #IR_OP_CONST
    bne     r1, r3, .coi_check_rhs
    load.q  r11, 48(r6)
    move.q  r9, #1
    move.q  r25, r6
.coi_check_rhs:
    bne     r2, r5, .coi_find_next
    move.q  r10, r0
    move.q  r26, r0
    load.q  r14, 8(r6)
    move.q  r3, #IR_OP_CONST
    bne     r1, r3, .coi_find_next
    load.q  r12, 48(r6)
    move.q  r10, #1
    move.q  r26, r6
.coi_find_next:
    add.q   r6, r6, #IR_RECORD_BYTES
    bra     .coi_find_values
.coi_values_found:
    beqz    r9, .coi_propagate_arithmetic_type
    beqz    r10, .coi_propagate_arithmetic_type
    move.q  r1, #IR_TYPE_I64
    bne     r13, r1, .coi_propagate_arithmetic_type
    bne     r14, r1, .coi_propagate_arithmetic_type
    ; Keep folding deliberately conservative until the FP64 promotion pass is
    ; present. Two non-negative signed 32-bit constants cannot overflow I64.
    blt     r11, r0, .coi_propagate_arithmetic_type
    blt     r12, r0, .coi_propagate_arithmetic_type
    move.q  r1, #0x7FFFFFFF
    bgt     r11, r1, .coi_propagate_arithmetic_type
    bgt     r12, r1, .coi_propagate_arithmetic_type
    load.q  r1, (r19)
    move.q  r15, #IR_TYPE_I64
    move.q  r2, #IR_OP_ADD
    beq     r1, r2, .coi_add_constants
    move.q  r2, #IR_OP_SUB
    beq     r1, r2, .coi_sub_constants
    move.q  r2, #IR_OP_MUL
    beq     r1, r2, .coi_mul_constants
    move.q  r2, #IR_OP_DIV
    beq     r1, r2, .coi_div_constants
    move.q  r13, r0
    move.q  r2, #IR_OP_COMPARE_EQ
    beq     r1, r2, .coi_compare_eq
    move.q  r2, #IR_OP_COMPARE_NE
    beq     r1, r2, .coi_compare_ne
    move.q  r2, #IR_OP_COMPARE_LT
    beq     r1, r2, .coi_compare_lt
    move.q  r2, #IR_OP_COMPARE_LE
    beq     r1, r2, .coi_compare_le
    move.q  r2, #IR_OP_COMPARE_GT
    beq     r1, r2, .coi_compare_gt
    bge     r11, r12, .coi_compare_true
    bra     .coi_store_fold
.coi_compare_eq:
    beq     r11, r12, .coi_compare_true
    bra     .coi_store_fold
.coi_compare_ne:
    bne     r11, r12, .coi_compare_true
    bra     .coi_store_fold
.coi_compare_lt:
    blt     r11, r12, .coi_compare_true
    bra     .coi_store_fold
.coi_compare_le:
    ble     r11, r12, .coi_compare_true
    bra     .coi_store_fold
.coi_compare_gt:
    bgt     r11, r12, .coi_compare_true
    bra     .coi_store_fold
.coi_compare_true:
    move.q  r13, #1
    bra     .coi_store_fold
.coi_mul_constants:
    mulu.q  r13, r11, r12
    bra     .coi_store_fold
.coi_div_constants:
    beqz    r12, .coi_propagate_arithmetic_type
    dcvtif  f0, r11
    dcvtif  f2, r12
    ddiv    f0, f0, f2
    push    r0
    dstore  f0, (r31)
    load.q  r13, (r31)
    pop     r1
    move.q  r15, #IR_TYPE_FP64
    bra     .coi_store_fold
.coi_add_constants:
    add.q   r13, r11, r12
    bra     .coi_store_fold
.coi_sub_constants:
    sub.q   r13, r11, r12
.coi_store_fold:
    ; The stack emitter evaluates every live value record. Once a binary
    ; operation is replaced by its constant result, the parser's two adjacent
    ; expression temporaries must no longer emit pushes. Do not consume merely
    ; matching non-SSA definitions from elsewhere in the stream: handcrafted
    ; IR and later compiler passes may legitimately share those definitions.
    sub.q   r6, r19, #IR_RECORD_BYTES
    sub.q   r7, r6, #IR_RECORD_BYTES
    beq     r25, r6, .coi_fold_lhs_latest
    bne     r25, r7, .coi_fold_inputs_removed
    bne     r26, r6, .coi_fold_inputs_removed
    bra     .coi_remove_fold_inputs
.coi_fold_lhs_latest:
    bne     r26, r7, .coi_fold_inputs_removed
.coi_remove_fold_inputs:
    load.q  r1, 56(r25)
    or.q    r1, r1, #IR_FLAG_REMOVED
    store.q r1, 56(r25)
    beq     r26, r25, .coi_fold_inputs_removed
    load.q  r1, 56(r26)
    or.q    r1, r1, #IR_FLAG_REMOVED
    store.q r1, 56(r26)
.coi_fold_inputs_removed:
    move.q  r1, #IR_OP_CONST
    store.q r1, (r19)
    load.q  r2, 40(r19)
    and.q   r2, r2, #IR_VALUE_MASK
    store.q r2, 40(r19)
    store.q r15, 8(r19)
    store.q r13, 48(r19)
    load.q  r1, 56(r19)
    or.q    r1, r1, #IR_FLAG_CONSTANT
    store.q r1, 56(r19)
    bra     .coi_next
.coi_propagate_arithmetic_type:
    load.q  r2, (r19)
    move.q  r1, #IR_OP_DIV
    bne     r2, r1, .coi_propagate_i64_type
    move.q  r1, #IR_TYPE_FP64
    store.q r1, 8(r19)
    bra     .coi_next
.coi_propagate_i64_type:
    move.q  r1, #IR_TYPE_I64
    bne     r13, r1, .coi_next
    bne     r14, r1, .coi_next
    store.q r1, 8(r19)
    load.q  r2, 56(r19)
    or.q    r2, r2, #IR_FLAG_I64_OPERANDS
    store.q r2, 56(r19)
    bra     .coi_next
.coi_propagate_copy:
    load.q  r3, 40(r19)
    lsr.q   r4, r3, #16
    and.q   r4, r4, #IR_VALUE_MASK
    move.q  r6, r18
    move.q  r9, r0
    move.q  r11, r0
    move.q  r12, r0
.coi_find_copy_source:
    bge     r6, r19, .coi_copy_source_found
    load.q  r1, 56(r6)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .coi_copy_next
    load.q  r1, (r6)
    move.q  r2, #IR_OP_CONST
    blt     r1, r2, .coi_copy_next
    move.q  r3, #IR_OP_CONVERT
    ble     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_ADDRESS
    blt     r1, r3, .coi_copy_next
    move.q  r3, #IR_OP_LOAD_SCALAR
    ble     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_DIV
    blt     r1, r3, .coi_copy_next
    move.q  r3, #IR_OP_COMPARE_GE
    ble     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_LOAD_ARRAY
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_PEEK8
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_PEEK16
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_PEEK32
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_PEEK64
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_CALL
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_USR
    beq     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_BIT_AND
    blt     r1, r3, .coi_copy_next
    move.q  r3, #IR_OP_NUMERIC_FN
    ble     r1, r3, .coi_copy_definition
    move.q  r3, #IR_OP_MEMALLOC
    bne     r1, r3, .coi_copy_next
.coi_copy_definition:
    load.q  r2, 40(r6)
    and.q   r2, r2, #IR_VALUE_MASK
    bne     r2, r4, .coi_copy_next
    move.q  r9, r0
    move.q  r3, #IR_OP_CONST
    bne     r1, r3, .coi_copy_next
    load.q  r11, 48(r6)
    load.q  r12, 8(r6)
    move.q  r9, #1
.coi_copy_next:
    add.q   r6, r6, #IR_RECORD_BYTES
    bra     .coi_find_copy_source
.coi_copy_source_found:
    beqz    r9, .coi_next
    ; A conversion is redundant only when its requested type already equals
    ; the latest constant definition's proven type.
    load.q  r1, (r19)
    move.q  r2, #IR_OP_CONVERT
    bne     r1, r2, .coi_store_copy
    load.q  r3, 8(r19)
    bne     r3, r12, .coi_next
.coi_store_copy:
    move.q  r1, #IR_OP_CONST
    store.q r1, (r19)
    load.q  r2, 40(r19)
    and.q   r2, r2, #IR_VALUE_MASK
    store.q r2, 40(r19)
    store.q r12, 8(r19)
    store.q r11, 48(r19)
    load.q  r1, 56(r19)
    or.q    r1, r1, #IR_FLAG_CONSTANT
    store.q r1, 56(r19)
    bra     .coi_next
.coi_ok:
    move.q  r8, r18
    jsr     compiler_pair_structured_loops
    bnez    r8, .coi_return
    move.q  r8, r18
    jsr     compiler_optimise_reachability
    move.q  r8, r18
    jsr     compiler_optimise_loops
    move.q  r8, r18
    jsr     compiler_optimise_array_bounds
    move.q  r8, r18
    jsr     compiler_optimise_dead_values
    move.q  r8, r18
    jsr     compiler_optimise_cleanup
    la      r1, COMPILER_IR_BASE
    load.q  r8, (r1)
    jsr     compiler_optimise_residency
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.coi_return:
    rts

; Pair nested WHILE/WEND and DO/LOOP records without imposing SSA. Each pair
; receives the opening source line as its stable emitted label identity.
compiler_pair_structured_loops:
    move.q  r19, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r20, (r1)
    move.q  r23, r0
.cpsp_scan:
    bge     r19, r20, .cpsp_done
    load.q  r1, 56(r19)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cpsp_next
    load.q  r1, (r19)
    move.q  r2, #IR_OP_WHILE
    beq     r1, r2, .cpsp_push
    move.q  r2, #IR_OP_DO
    beq     r1, r2, .cpsp_push
    move.q  r2, #IR_OP_WEND
    beq     r1, r2, .cpsp_close_while
    move.q  r2, #IR_OP_LOOP
    beq     r1, r2, .cpsp_close_do
.cpsp_next:
    add.q   r19, r19, #IR_RECORD_BYTES
    bra     .cpsp_scan
.cpsp_push:
    push    r19
    add.q   r23, r23, #1
    bra     .cpsp_next
.cpsp_close_while:
    beqz    r23, .cpsp_bad
    pop     r21
    sub.q   r23, r23, #1
    load.q  r1, (r21)
    move.q  r2, #IR_OP_WHILE
    bne     r1, r2, .cpsp_bad
    load.q  r22, 24(r21)
    store.q r22, 48(r19)           ; WEND branches to the opening line
    add.q   r24, r19, #IR_RECORD_BYTES
.cpsp_while_exit_scan:
    bge     r24, r20, .cpsp_while_exit_end
    load.q  r1, (r24)
    move.q  r2, #IR_OP_BLOCK
    beq     r1, r2, .cpsp_while_exit_found
    add.q   r24, r24, #IR_RECORD_BYTES
    bra     .cpsp_while_exit_scan
.cpsp_while_exit_found:
    load.q  r22, 24(r24)
    store.q r22, 48(r21)           ; false condition skips to line after WEND
    bra     .cpsp_next
.cpsp_while_exit_end:
    store.q r0, 48(r21)
    bra     .cpsp_next
.cpsp_close_do:
    beqz    r23, .cpsp_bad
    pop     r21
    sub.q   r23, r23, #1
    load.q  r1, (r21)
    move.q  r2, #IR_OP_DO
    bne     r1, r2, .cpsp_bad
.cpsp_pair:
    load.q  r22, 24(r21)
    store.q r22, 48(r21)
    load.q  r1, 48(r19)
    lsr.q   r1, r1, #32
    lsl.q   r1, r1, #32
    or.q    r1, r1, r22
    store.q r1, 48(r19)
    bra     .cpsp_next
.cpsp_done:
    bnez    r23, .cpsp_bad
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
    rts
.cpsp_bad:
    beqz    r23, .cpsp_bad_ready
    pop     r1
    sub.q   r23, r23, #1
    bra     .cpsp_bad
.cpsp_bad_ready:
    move.q  r8, #6
    load.q  r9, 24(r19)
    load.q  r10, 32(r19)
    rts

; Mark blocks reachable through fallthrough and explicit branches, then remove
; records belonging to blocks that have no path from the entry block. The
; fixed point is required because source order need not match branch order.
compiler_optimise_reachability:
    move.q  r18, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r19, (r1)
    move.q  r21, r18
.cor_find_entry:
    bge     r21, r19, .cor_done
    load.q  r1, (r21)
    move.q  r2, #IR_OP_BLOCK
    beq     r1, r2, .cor_mark_entry
    add.q   r21, r21, #IR_RECORD_BYTES
    bra     .cor_find_entry
.cor_mark_entry:
    load.q  r1, 56(r21)
    or.q    r1, r1, #IR_FLAG_REACHABLE
    store.q r1, 56(r21)
.cor_again:
    move.q  r20, r0
    move.q  r21, r18
    move.q  r22, r0
    move.q  r23, r0
.cor_record:
    bge     r21, r19, .cor_iteration_done
    load.q  r1, (r21)
    move.q  r2, #IR_OP_BLOCK
    bne     r1, r2, .cor_branch
    beqz    r23, .cor_read_block
    load.q  r3, 56(r21)
    and.q   r4, r3, #IR_FLAG_REACHABLE
    bnez    r4, .cor_read_block
    or.q    r3, r3, #IR_FLAG_REACHABLE
    store.q r3, 56(r21)
    move.q  r20, #1
.cor_read_block:
    load.q  r3, 56(r21)
    and.q   r22, r3, #IR_FLAG_REACHABLE
    move.q  r23, r22
    bra     .cor_next
.cor_branch:
    beqz    r22, .cor_next
    move.q  r2, #IR_OP_HALT
    bne     r1, r2, .cor_branch_kind
    move.q  r23, r0
    bra     .cor_next
.cor_branch_kind:
    move.q  r2, #IR_OP_BRANCH
    beq     r1, r2, .cor_have_branch
    move.q  r2, #IR_OP_BRANCH_FALSE
    beq     r1, r2, .cor_have_branch
    move.q  r2, #IR_OP_BRANCH_TRUE
    beq     r1, r2, .cor_have_branch
    move.q  r2, #IR_OP_ON_TARGET
    beq     r1, r2, .cor_have_branch
    move.q  r2, #IR_OP_GOSUB
    bne     r1, r2, .cor_next
.cor_have_branch:
    load.q  r24, 48(r21)
    load.q  r1, (r21)
    move.q  r2, #IR_OP_ON_TARGET
    bne     r1, r2, .cor_branch_target_ready
    lsl.q   r24, r24, #32
    lsr.q   r24, r24, #32
.cor_branch_target_ready:
    move.q  r25, r18
.cor_find_target:
    bge     r25, r19, .cor_target_done
    load.q  r3, (r25)
    move.q  r4, #IR_OP_BLOCK
    bne     r3, r4, .cor_target_next
    load.q  r3, 40(r25)
    bne     r3, r24, .cor_target_next
    load.q  r3, 56(r25)
    and.q   r4, r3, #IR_FLAG_REACHABLE
    bnez    r4, .cor_target_done
    or.q    r3, r3, #IR_FLAG_REACHABLE
    store.q r3, 56(r25)
    move.q  r20, #1
    bra     .cor_target_done
.cor_target_next:
    add.q   r25, r25, #IR_RECORD_BYTES
    bra     .cor_find_target
.cor_target_done:
    load.q  r1, (r21)
    move.q  r2, #IR_OP_BRANCH
    bne     r1, r2, .cor_next
    move.q  r23, r0
.cor_next:
    add.q   r21, r21, #IR_RECORD_BYTES
    bra     .cor_record
.cor_iteration_done:
    bnez    r20, .cor_again
    move.q  r21, r18
    move.q  r22, #1
.cor_sweep:
    bge     r21, r19, .cor_done
    load.q  r1, (r21)
    move.q  r2, #IR_OP_BLOCK
    bne     r1, r2, .cor_remove_if_dead
    load.q  r3, 56(r21)
    and.q   r22, r3, #IR_FLAG_REACHABLE
.cor_remove_if_dead:
    bnez    r22, .cor_sweep_next
    ; DATA is declarative programme storage, not an executable statement.
    ; READ traverses every DATA item in source order even when ordinary
    ; control flow cannot enter the block containing its declaration.
    move.q  r2, #IR_OP_DATA
    beq     r1, r2, .cor_sweep_next
    load.q  r3, 56(r21)
    or.q    r3, r3, #IR_FLAG_REMOVED
    store.q r3, 56(r21)
.cor_sweep_next:
    add.q   r21, r21, #IR_RECORD_BYTES
    bra     .cor_sweep
.cor_done:
    rts

; Pair simple scalar FOR/NEXT records without inventing loop semantics for an
; ambiguous or already paired construct. Limit and step values are computed
; before IR_OP_FOR, making their storage addresses loop invariant.
compiler_optimise_loops:
    move.q  r18, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r19, (r1)
    move.q  r20, r18
    move.q  r26, #1               ; monotonic identity for each paired loop
.col_next_record:
    bge     r20, r19, .col_done
    load.q  r3, 56(r20)
    and.q   r3, r3, #IR_FLAG_REMOVED
    bnez    r3, .col_advance
    load.q  r1, (r20)
    move.q  r2, #IR_OP_NEXT
    bne     r1, r2, .col_advance
    load.q  r1, 48(r20)
    lsr.q   r21, r1, #32           ; requested variable, zero means innermost
    move.q  r22, r0                ; best unmatched FOR record
    move.q  r23, r18
.col_scan_for:
    bge     r23, r20, .col_scanned
    load.q  r1, (r23)
    move.q  r2, #IR_OP_FOR
    bne     r1, r2, .col_scan_next
    load.q  r3, 56(r23)
    and.q   r4, r3, #IR_FLAG_REMOVED
    bnez    r4, .col_scan_next
    and.q   r4, r3, #IR_FLAG_LOOP_PAIRED
    bnez    r4, .col_scan_next
    load.q  r3, 48(r23)
    lsr.q   r4, r3, #32
    beqz    r21, .col_candidate
    bne     r4, r21, .col_scan_next
.col_candidate:
    move.q  r22, r23
.col_scan_next:
    add.q   r23, r23, #IR_RECORD_BYTES
    bra     .col_scan_for
.col_scanned:
    beqz    r22, .col_advance
    load.q  r3, 56(r22)
    or.q    r3, r3, #(IR_FLAG_LOOP_INVARIANT | IR_FLAG_LOOP_PAIRED)
    store.q r3, 56(r22)
    load.q  r3, 56(r20)
    or.q    r3, r3, #(IR_FLAG_LOOP_INVARIANT | IR_FLAG_LOOP_PAIRED)
    store.q r3, 56(r20)
    load.q  r5, 48(r22)
    ; Loop identity belongs to the FOR/NEXT pair, not to an IR position.
    ; Parser bookkeeping and later removal passes therefore cannot renumber it.
    move.q  r3, r26
    add.q   r26, r26, #1
    move.q  r6, r5
    lsr.q   r6, r6, #32
    lsl.q   r6, r6, #32
    or.q    r6, r6, r3
    store.q r6, 48(r22)
    load.q  r4, 48(r20)
    bnez    r21, .col_store_target
    lsr.q   r5, r5, #32
    lsl.q   r5, r5, #32
    or.q    r4, r4, r5
.col_store_target:
    lsr.q   r4, r4, #32
    lsl.q   r4, r4, #32
    or.q    r4, r4, r3
    store.q r4, 48(r20)
.col_advance:
    add.q   r20, r20, #IR_RECORD_BYTES
    bra     .col_next_record
.col_done:
    rts

; Prove a bounds check only when both its latest index definition and the size
; feeding the latest preceding DIM of the same array are I64 constants.
compiler_optimise_array_bounds:
    move.q  r18, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r19, (r1)
    ; A linear latest-definition scan is a proof only for straight-line IR.
    ; Retain runtime checks when control flow can bypass or revisit a defining
    ; record until predecessor-aware dataflow is available.
    move.q  r20, r18
.cob_control_scan:
    bge     r20, r19, .cob_control_safe
    load.q  r1, 56(r20)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cob_control_next
    load.q  r1, (r20)
    move.q  r2, #IR_OP_BRANCH
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_BRANCH_FALSE
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_FOR
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_NEXT
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_GOSUB
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_RETURN
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_BRANCH_TRUE
    beq     r1, r2, .cob_done
    move.q  r2, #IR_OP_WHILE
    blt     r1, r2, .cob_control_next
    move.q  r2, #IR_OP_ON_TARGET
    ble     r1, r2, .cob_done
.cob_control_next:
    add.q   r20, r20, #IR_RECORD_BYTES
    bra     .cob_control_scan
.cob_control_safe:
    move.q  r20, r18
.cob_record:
    bge     r20, r19, .cob_done
    load.q  r1, (r20)
    move.q  r2, #IR_OP_BOUNDS_CHECK
    bne     r1, r2, .cob_next
    load.q  r1, 40(r20)
    lsr.q   r21, r1, #16
    and.q   r21, r21, #IR_VALUE_MASK
    load.q  r22, 48(r20)
    move.q  r23, r0                ; latest matching DIM record
    move.q  r24, r18
.cob_find_dim:
    bge     r24, r20, .cob_dim_found
    load.q  r1, 56(r24)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cob_dim_next
    load.q  r1, (r24)
    move.q  r2, #IR_OP_DIM_ARRAY
    bne     r1, r2, .cob_dim_next
    load.q  r1, 48(r24)
    bne     r1, r22, .cob_dim_next
    move.q  r23, r24
.cob_dim_next:
    add.q   r24, r24, #IR_RECORD_BYTES
    bra     .cob_find_dim
.cob_dim_found:
    beqz    r23, .cob_next
    ; Resolve the latest definition of the index before the check.
    move.q  r24, r18
    move.q  r25, r0
    move.q  r26, r0
.cob_index_scan:
    bge     r24, r20, .cob_index_done
    load.q  r1, 56(r24)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cob_index_next
    load.q  r1, 40(r24)
    and.q   r2, r1, #IR_VALUE_MASK
    bne     r2, r21, .cob_index_next
    load.q  r3, (r24)
    move.q  r4, #IR_OP_CONST
    blt     r3, r4, .cob_index_next
    move.q  r4, #IR_OP_CONVERT
    ble     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_ADDRESS
    blt     r3, r4, .cob_index_late
    move.q  r4, #IR_OP_LOAD_SCALAR
    ble     r3, r4, .cob_index_definition
.cob_index_late:
    move.q  r4, #IR_OP_DIV
    blt     r3, r4, .cob_index_array
    move.q  r4, #IR_OP_COMPARE_GE
    ble     r3, r4, .cob_index_definition
.cob_index_array:
    move.q  r4, #IR_OP_LOAD_ARRAY
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_PEEK8
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_PEEK16
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_PEEK32
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_PEEK64
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_CALL
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_USR
    beq     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_BIT_AND
    blt     r3, r4, .cob_index_next
    move.q  r4, #IR_OP_NUMERIC_FN
    ble     r3, r4, .cob_index_definition
    move.q  r4, #IR_OP_MEMALLOC
    bne     r3, r4, .cob_index_next
.cob_index_definition:
    move.q  r25, r0
    move.q  r4, #IR_OP_CONST
    bne     r3, r4, .cob_index_next
    load.q  r4, 8(r24)
    move.q  r5, #IR_TYPE_I64
    bne     r4, r5, .cob_index_next
    load.q  r26, 48(r24)
    move.q  r25, #1
.cob_index_next:
    add.q   r24, r24, #IR_RECORD_BYTES
    bra     .cob_index_scan
.cob_index_done:
    beqz    r25, .cob_next
    blt     r26, r0, .cob_next
    ; Resolve the size value used by the latest DIM, stopping at that DIM.
    load.q  r1, 40(r23)
    lsr.q   r21, r1, #16
    and.q   r21, r21, #IR_VALUE_MASK
    move.q  r24, r18
    move.q  r25, r0
    move.q  r27, r0
.cob_size_scan:
    bge     r24, r23, .cob_size_done
    load.q  r1, 56(r24)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cob_size_next
    load.q  r1, 40(r24)
    and.q   r2, r1, #IR_VALUE_MASK
    bne     r2, r21, .cob_size_next
    load.q  r3, (r24)
    move.q  r4, #IR_OP_CONST
    blt     r3, r4, .cob_size_next
    move.q  r4, #IR_OP_CONVERT
    ble     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_ADDRESS
    blt     r3, r4, .cob_size_late
    move.q  r4, #IR_OP_LOAD_SCALAR
    ble     r3, r4, .cob_size_definition
.cob_size_late:
    move.q  r4, #IR_OP_DIV
    blt     r3, r4, .cob_size_array
    move.q  r4, #IR_OP_COMPARE_GE
    ble     r3, r4, .cob_size_definition
.cob_size_array:
    move.q  r4, #IR_OP_LOAD_ARRAY
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_PEEK8
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_PEEK16
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_PEEK32
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_PEEK64
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_CALL
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_USR
    beq     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_BIT_AND
    blt     r3, r4, .cob_size_next
    move.q  r4, #IR_OP_NUMERIC_FN
    ble     r3, r4, .cob_size_definition
    move.q  r4, #IR_OP_MEMALLOC
    bne     r3, r4, .cob_size_next
.cob_size_definition:
    move.q  r25, r0
    move.q  r4, #IR_OP_CONST
    bne     r3, r4, .cob_size_next
    load.q  r4, 8(r24)
    move.q  r5, #IR_TYPE_I64
    bne     r4, r5, .cob_size_next
    load.q  r27, 48(r24)
    move.q  r25, #1
.cob_size_next:
    add.q   r24, r24, #IR_RECORD_BYTES
    bra     .cob_size_scan
.cob_size_done:
    beqz    r25, .cob_next
    bgt     r26, r27, .cob_next
    load.q  r1, 56(r20)
    or.q    r1, r1, #IR_FLAG_BOUNDS_PROVEN
    store.q r1, 56(r20)
.cob_next:
    add.q   r20, r20, #IR_RECORD_BYTES
    bra     .cob_record
.cob_done:
    rts

; Remove unused side-effect-free value definitions. The IR is non-SSA, so a
; later definition of the same compact value id ends the candidate's live
; range before subsequent uses are considered.
compiler_optimise_dead_values:
    move.q  r18, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r19, (r1)
    ; Linear non-SSA liveness cannot safely remove loop-carried definitions
    ; without a control-flow fixed point. Retain them whenever a FOR/NEXT is
    ; present; straight-line programmes still receive dead-value cleanup.
    move.q  r20, r18
.cod_loop_scan:
    bge     r20, r19, .cod_loop_free
    load.q  r1, (r20)
    move.q  r2, #IR_OP_FOR
    beq     r1, r2, .cod_done
    move.q  r2, #IR_OP_NEXT
    beq     r1, r2, .cod_done
    add.q   r20, r20, #IR_RECORD_BYTES
    bra     .cod_loop_scan
.cod_loop_free:
    move.q  r20, r18
.cod_candidate:
    bge     r20, r19, .cod_done
    load.q  r1, 56(r20)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cod_next_candidate
    load.q  r21, (r20)
    move.q  r1, #IR_OP_CONST
    blt     r21, r1, .cod_next_candidate
    move.q  r1, #IR_OP_CONVERT
    ble     r21, r1, .cod_candidate_effect
    move.q  r1, #IR_OP_ADDRESS
    blt     r21, r1, .cod_candidate_late
    move.q  r1, #IR_OP_LOAD_SCALAR
    ble     r21, r1, .cod_candidate_effect
.cod_candidate_late:
    move.q  r1, #IR_OP_DIV
    blt     r21, r1, .cod_next_candidate
    move.q  r1, #IR_OP_COMPARE_GE
    ble     r21, r1, .cod_candidate_effect
    move.q  r1, #IR_OP_BIT_AND
    blt     r21, r1, .cod_next_candidate
    move.q  r1, #IR_OP_NUMERIC_FN
    bgt     r21, r1, .cod_next_candidate
.cod_candidate_effect:
    load.q  r1, 16(r20)
    beqz    r1, .cod_have_candidate
    move.q  r2, #IR_OP_LOAD_SCALAR
    bne     r21, r2, .cod_next_candidate
.cod_have_candidate:
    load.q  r22, 40(r20)
    and.q   r22, r22, #IR_VALUE_MASK
    beqz    r22, .cod_next_candidate
    add.q   r23, r20, #IR_RECORD_BYTES
.cod_future:
    bge     r23, r19, .cod_remove
    load.q  r1, 56(r23)
    and.q   r1, r1, #IR_FLAG_REMOVED
    bnez    r1, .cod_future_next
    load.q  r2, 40(r23)
    load.q  r4, (r23)
    move.q  r1, #IR_OP_FOR
    bne     r4, r1, .cod_future_lhs
    move.q  r3, r2
    and.q   r3, r3, #IR_VALUE_MASK
    beq     r3, r22, .cod_live
.cod_future_lhs:
    lsr.q   r3, r2, #16
    and.q   r3, r3, #IR_VALUE_MASK
    beq     r3, r22, .cod_live
    lsr.q   r3, r2, #32
    and.q   r3, r3, #IR_VALUE_MASK
    beq     r3, r22, .cod_live
    ; Stop at the next value-producing redefinition of this non-SSA id.
    move.q  r1, #IR_OP_CONST
    blt     r4, r1, .cod_future_next
    move.q  r1, #IR_OP_CONVERT
    ble     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_ADDRESS
    blt     r4, r1, .cod_future_late
    move.q  r1, #IR_OP_LOAD_SCALAR
    ble     r4, r1, .cod_future_definition
.cod_future_late:
    move.q  r1, #IR_OP_DIV
    blt     r4, r1, .cod_future_next
    move.q  r1, #IR_OP_COMPARE_GE
    ble     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_LOAD_ARRAY
    beq     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_PEEK8
    beq     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_PEEK16
    beq     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_PEEK32
    beq     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_PEEK64
    beq     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_USR
    beq     r4, r1, .cod_future_definition
    move.q  r1, #IR_OP_BIT_AND
    blt     r4, r1, .cod_future_next
    move.q  r1, #IR_OP_NUMERIC_FN
    bgt     r4, r1, .cod_future_next
.cod_future_definition:
    and.q   r2, r2, #IR_VALUE_MASK
    beq     r2, r22, .cod_remove
.cod_future_next:
    add.q   r23, r23, #IR_RECORD_BYTES
    bra     .cod_future
.cod_remove:
    load.q  r1, 56(r20)
    or.q    r1, r1, #IR_FLAG_REMOVED
    store.q r1, 56(r20)
.cod_live:
.cod_next_candidate:
    add.q   r20, r20, #IR_RECORD_BYTES
    bra     .cod_candidate
.cod_done:
    rts

; Remove redundancies only when their proof is present in the record itself.
; Also remove an unconditional branch whose target is the immediately following
; block. This pass never crosses an effectful record or guesses type facts.
compiler_optimise_cleanup:
    move.q  r18, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r19, (r1)
.coc_one:
    bge     r18, r19, .coc_done
    load.q  r1, (r18)
    move.q  r2, #IR_OP_TAG_CHECK
    bne     r1, r2, .coc_bounds
    load.q  r3, 8(r18)
    move.q  r4, #IR_TYPE_I64
    beq     r3, r4, .coc_remove
    move.q  r4, #IR_TYPE_FP64
    beq     r3, r4, .coc_remove
    bra     .coc_next
.coc_bounds:
    move.q  r2, #IR_OP_BOUNDS_CHECK
    bne     r1, r2, .coc_branch
    load.q  r3, 56(r18)
    and.q   r3, r3, #IR_FLAG_BOUNDS_PROVEN
    bnez    r3, .coc_remove
    bra     .coc_next
.coc_branch:
    move.q  r2, #IR_OP_BRANCH
    bne     r1, r2, .coc_next
    add.q   r3, r18, #IR_RECORD_BYTES
    bge     r3, r19, .coc_next
    load.q  r4, (r3)
    move.q  r5, #IR_OP_BLOCK
    bne     r4, r5, .coc_next
    load.q  r4, 48(r18)
    load.q  r5, 40(r3)
    bne     r4, r5, .coc_next
.coc_remove:
    load.q  r3, 56(r18)
    or.q    r3, r3, #IR_FLAG_REMOVED
    store.q r3, 56(r18)
.coc_next:
    add.q   r18, r18, #IR_RECORD_BYTES
    bra     .coc_one
.coc_done:
    rts

; Keep at most one induction variable and one repeatedly accessed scalar in
; r18:r19 and r24:r25 for a proven straight-line scalar FOR body. Any nested
; loop, branch, array/memory operation or externally visible effect abandons
; residency, leaving the normal BASIC cells as the authoritative fallback.
compiler_optimise_residency:
    move.q  r18, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r19, (r1)
    move.q  r20, r18
.corz_for:
    bge     r20, r19, .corz_done
    load.q  r1, (r20)
    move.q  r2, #IR_OP_FOR
    bne     r1, r2, .corz_next_for
    load.q  r1, 56(r20)
    and.q   r2, r1, #IR_FLAG_LOOP_PAIRED
    beqz    r2, .corz_next_for
    load.q  r21, 48(r20)
    lsr.q   r21, r21, #32          ; induction-variable symbol
    beqz    r21, .corz_next_for
    ; Any non-FOR control transfer can enter or leave a labelled loop body
    ; without executing the resident prologue/epilogue. Keep such programmes
    ; memory-backed until a full control-flow predecessor proof exists.
    move.q  r29, r18
.corz_control_scan:
    bge     r29, r19, .corz_control_safe
    load.q  r1, (r29)
    move.q  r2, #IR_OP_BRANCH
    beq     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_BRANCH_FALSE
    beq     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_GOSUB
    beq     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_RETURN
    beq     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_BRANCH_TRUE
    beq     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_WHILE
    blt     r1, r2, .corz_control_next
    move.q  r2, #IR_OP_ON_TARGET
    ble     r1, r2, .corz_next_for
.corz_control_next:
    add.q   r29, r29, #IR_RECORD_BYTES
    bra     .corz_control_scan
.corz_control_safe:
    add.q   r22, r20, #IR_RECORD_BYTES
    move.q  r23, r0                ; matching NEXT
.corz_body:
    bge     r22, r19, .corz_next_for
    load.q  r1, (r22)
    move.q  r2, #IR_OP_NEXT
    beq     r1, r2, .corz_maybe_next
    move.q  r2, #IR_OP_FOR
    beq     r1, r2, .corz_next_for ; nested loop needs another resident set
    move.q  r2, #IR_OP_BLOCK
    beq     r1, r2, .corz_body_next
    move.q  r2, #IR_OP_CONST
    blt     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_CONVERT
    ble     r1, r2, .corz_body_next
    move.q  r2, #IR_OP_ADDRESS
    beq     r1, r2, .corz_body_next
    move.q  r2, #IR_OP_LOAD_SCALAR
    beq     r1, r2, .corz_body_next
    move.q  r2, #IR_OP_STORE_SCALAR
    beq     r1, r2, .corz_body_next
    move.q  r2, #IR_OP_DIV
    beq     r1, r2, .corz_body_next
    move.q  r2, #IR_OP_COMPARE_EQ
    blt     r1, r2, .corz_next_for
    move.q  r2, #IR_OP_COMPARE_GE
    ble     r1, r2, .corz_body_next
    bra     .corz_next_for
.corz_body_next:
    add.q   r22, r22, #IR_RECORD_BYTES
    bra     .corz_body
.corz_maybe_next:
    load.q  r2, 48(r22)
    lsr.q   r2, r2, #32
    bne     r2, r21, .corz_next_for
    move.q  r23, r22
    ; Select the first non-induction scalar referenced at least twice.
    move.q  r24, r0                ; hot symbol
    add.q   r26, r20, #IR_RECORD_BYTES
.corz_candidate:
    bge     r26, r23, .corz_mark
    load.q  r1, (r26)
    move.q  r2, #IR_OP_LOAD_SCALAR
    beq     r1, r2, .corz_count_candidate
    move.q  r2, #IR_OP_STORE_SCALAR
    bne     r1, r2, .corz_candidate_next
.corz_count_candidate:
    load.q  r27, 48(r26)
    beq     r27, r21, .corz_candidate_next
    move.q  r28, r0
    add.q   r29, r20, #IR_RECORD_BYTES
.corz_count:
    bge     r29, r23, .corz_counted
    load.q  r1, (r29)
    move.q  r2, #IR_OP_LOAD_SCALAR
    beq     r1, r2, .corz_count_symbol
    move.q  r2, #IR_OP_STORE_SCALAR
    bne     r1, r2, .corz_count_next
.corz_count_symbol:
    load.q  r2, 48(r29)
    bne     r2, r27, .corz_count_next
    add.q   r28, r28, #1
.corz_count_next:
    add.q   r29, r29, #IR_RECORD_BYTES
    bra     .corz_count
.corz_counted:
    move.q  r1, #2
    blt     r28, r1, .corz_candidate_next
    move.q  r24, r27
    bra     .corz_mark
.corz_candidate_next:
    add.q   r26, r26, #IR_RECORD_BYTES
    bra     .corz_candidate
.corz_mark:
    load.q  r1, 56(r20)
    or.q    r1, r1, #IR_FLAG_RESIDENT
    store.q r1, 56(r20)
    load.q  r1, 56(r23)
    or.q    r1, r1, #IR_FLAG_RESIDENT
    store.q r1, 56(r23)
    load.q  r1, 8(r20)
    lsl.q   r2, r24, #32
    or.q    r1, r1, r2
    store.q r1, 8(r20)
    store.q r1, 8(r23)
    add.q   r26, r20, #IR_RECORD_BYTES
.corz_mark_body:
    bge     r26, r23, .corz_after_loop
    load.q  r1, (r26)
    move.q  r2, #IR_OP_LOAD_SCALAR
    beq     r1, r2, .corz_mark_scalar
    move.q  r2, #IR_OP_STORE_SCALAR
    bne     r1, r2, .corz_mark_next
.corz_mark_scalar:
    load.q  r2, 48(r26)
    beq     r2, r21, .corz_mark_induction
    beqz    r24, .corz_mark_next
    bne     r2, r24, .corz_mark_next
    load.q  r3, 56(r26)
    or.q    r3, r3, #(IR_FLAG_RESIDENT | IR_FLAG_RESIDENT_HOT)
    store.q r3, 56(r26)
    bra     .corz_mark_next
.corz_mark_induction:
    load.q  r3, 56(r26)
    or.q    r3, r3, #IR_FLAG_RESIDENT
    store.q r3, 56(r26)
.corz_mark_next:
    add.q   r26, r26, #IR_RECORD_BYTES
    bra     .corz_mark_body
.corz_after_loop:
    move.q  r20, r23
.corz_next_for:
    add.q   r20, r20, #IR_RECORD_BYTES
    bra     .corz_for
.corz_done:
    rts
