; ehbasic_compiler_helpers.inc - typed runtime helper metadata and closure

COMPILER_HELPER_TRUTHY       equ 1
COMPILER_HELPER_TO_I64       equ 2
COMPILER_HELPER_PRINT_NUM    equ 3
COMPILER_HELPER_COUNT        equ 3
COMPILER_HELPER_RECORD_BYTES equ 80

; id, signature, clobbers, state-read, state-written, effect, errors,
; dependency mask, target flags, typed entry symbol.
compiler_helper_table:
    dc.q COMPILER_HELPER_TRUTHY, 0x0101, 0x8FFE, 0, 0, IR_EFFECT_NONE, 1, (1 << COMPILER_HELPER_TO_I64), 3, compiler_runtime_truthy
    dc.q COMPILER_HELPER_TO_I64, 0x0101, 0x8FFE, 0, 0, IR_EFFECT_NONE, 1, (1 << COMPILER_HELPER_TRUTHY), 3, compiler_runtime_to_i64
    dc.q COMPILER_HELPER_PRINT_NUM, 0x0100, 0x8FFE, 1, 0, IR_EFFECT_MMIO, 1, (1 << COMPILER_HELPER_TO_I64), 3, compiler_runtime_print_num

; R8=id. Returns R8=status and R9=metadata pointer.
compiler_helper_lookup:
    move.q  r9, r0
    move.q  r1, #1
    blt     r8, r1, .chl_missing
    move.q  r1, #COMPILER_HELPER_COUNT
    bgt     r8, r1, .chl_missing
    sub.q   r1, r8, #1
    lsl.q   r2, r1, #6
    lsl.q   r3, r1, #4
    add.q   r2, r2, r3
    la      r9, compiler_helper_table
    add.q   r9, r9, r2
    move.q  r8, r0
    rts
.chl_missing:
    move.q  r8, #1
    rts

; Resolve requested helper bits and dependencies. Cycles are harmless because
; closure is a monotonic bit set. Output is ascending helper ID for deterministic
; standalone source emission. R8=request mask, R9=output, R10=capacity entries.
; Returns R8=status, R9=count and R10=closure mask.
compiler_helper_closure:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r20, r10
    move.q  r1, #1
    lsl.q   r1, r1, #(COMPILER_HELPER_COUNT + 1)
    sub.q   r1, r1, #2
    and.q   r2, r18, r1
    bne     r2, r18, .chc_fail
    move.q  r18, r2
.chc_expand:
    move.q  r21, r18
    move.q  r22, #1
.chc_dep_one:
    move.q  r1, #COMPILER_HELPER_COUNT
    bgt     r22, r1, .chc_expanded
    move.q  r2, #1
    lsl.q   r2, r2, r22
    and.q   r3, r18, r2
    beqz    r3, .chc_dep_next
    move.q  r8, r22
    jsr     compiler_helper_lookup
    bnez    r8, .chc_fail
    load.q  r3, 56(r9)
    or.q    r18, r18, r3
.chc_dep_next:
    add.q   r22, r22, #1
    bra     .chc_dep_one
.chc_expanded:
    bne     r18, r21, .chc_expand
    move.q  r22, #1
    move.q  r23, r0
.chc_emit:
    move.q  r1, #COMPILER_HELPER_COUNT
    bgt     r22, r1, .chc_ok
    move.q  r2, #1
    lsl.q   r2, r2, r22
    and.q   r3, r18, r2
    beqz    r3, .chc_emit_next
    bge     r23, r20, .chc_fail
    lsl.q   r3, r23, #3
    add.q   r3, r19, r3
    store.q r22, (r3)
    add.q   r23, r23, #1
.chc_emit_next:
    add.q   r22, r22, #1
    bra     .chc_emit
.chc_ok:
    move.q  r8, r0
    move.q  r9, r23
    move.q  r10, r18
    rts
.chc_fail:
    move.q  r8, #1
    move.q  r9, r0
    move.q  r10, r0
    rts

compiler_runtime_truthy:
    rts
compiler_runtime_to_i64:
    rts
compiler_runtime_print_num:
    rts
