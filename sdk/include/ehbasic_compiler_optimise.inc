; ehbasic_compiler_optimise.inc - correctness-preserving typed IR passes

; Optimise a completed linear IR in place. The initial implementation performs
; conservative I64 constant/type propagation. R8=IR base. Returns R8=status,
; R9=line and R10=token offset. Status 4 is a compile-time numeric overflow.
compiler_optimise_ir:
    move.q  r18, r8
    move.q  r19, r8
    la      r1, COMPILER_IR_CURSOR
    load.q  r20, (r1)
.coi_record:
    bge     r19, r20, .coi_ok
    load.q  r1, (r19)
    move.q  r2, #IR_OP_ADD
    beq     r1, r2, .coi_fold_add
.coi_next:
    add.q   r19, r19, #IR_RECORD_BYTES
    bra     .coi_record
.coi_fold_add:
    load.q  r3, 40(r19)
    lsr.q   r4, r3, #16
    and.q   r4, r4, #IR_VALUE_MASK
    lsr.q   r5, r3, #32
    and.q   r5, r5, #IR_VALUE_MASK
    move.q  r6, r18
    move.q  r11, r0
    move.q  r12, r0
    move.q  r9, r0
    move.q  r10, r0
.coi_find_values:
    bge     r6, r19, .coi_values_found
    load.q  r1, (r6)
    move.q  r2, #IR_OP_CONST
    blt     r1, r2, .coi_find_next
    move.q  r3, #IR_OP_CONVERT
    ble     r1, r3, .coi_definition
    move.q  r3, #IR_OP_ADDRESS
    blt     r1, r3, .coi_find_next
    move.q  r3, #IR_OP_LOAD_SCALAR
    bgt     r1, r3, .coi_find_next
.coi_definition:
    load.q  r2, 40(r6)
    and.q   r2, r2, #IR_VALUE_MASK
    bne     r2, r4, .coi_check_rhs
    move.q  r9, r0
    move.q  r3, #IR_OP_CONST
    bne     r1, r3, .coi_check_rhs
    load.q  r11, 48(r6)
    move.q  r9, #1
.coi_check_rhs:
    bne     r2, r5, .coi_find_next
    move.q  r10, r0
    move.q  r3, #IR_OP_CONST
    bne     r1, r3, .coi_find_next
    load.q  r12, 48(r6)
    move.q  r10, #1
.coi_find_next:
    add.q   r6, r6, #IR_RECORD_BYTES
    bra     .coi_find_values
.coi_values_found:
    beqz    r9, .coi_next
    beqz    r10, .coi_next
    ; Keep folding deliberately conservative until the FP64 promotion pass is
    ; present. Two non-negative signed 32-bit constants cannot overflow I64.
    blt     r11, r0, .coi_next
    blt     r12, r0, .coi_next
    move.q  r1, #0x7FFFFFFF
    bgt     r11, r1, .coi_next
    bgt     r12, r1, .coi_next
    add.q   r13, r11, r12
.coi_store_fold:
    move.q  r1, #IR_OP_CONST
    store.q r1, (r19)
    move.q  r1, #IR_TYPE_I64
    store.q r1, 8(r19)
    store.q r13, 48(r19)
    load.q  r1, 56(r19)
    or.q    r1, r1, #IR_FLAG_CONSTANT
    store.q r1, 56(r19)
    bra     .coi_next
.coi_ok:
    move.q  r8, r18
    jsr     compiler_optimise_cleanup
    move.q  r8, r0
    move.q  r9, r0
    move.q  r10, r0
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
