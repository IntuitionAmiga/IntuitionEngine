; ehbasic_compiler_lower.inc - direct target-level lowering

LOWER_RECORD_BYTES equ 32
LOWER_OP_CONST     equ 1
LOWER_OP_ADD       equ 2
LOWER_OP_SUB       equ 3
LOWER_OP_MUL       equ 4
LOWER_OP_BRANCH    equ 5

; R8=IR base, R9=output start, R10=output end, R11=target.
; Returns status/count/output cursor. Unknown semantic records fail with their
; source location and are never delegated to interpreter entry points.
compiler_lower_ir:
    move.q  r18, r8
    move.q  r19, r9
    move.q  r20, r10
    move.q  r21, r0
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
    beq     r1, r2, .cli_next
    move.q  r2, #IR_OP_CONST
    beq     r1, r2, .cli_const
    move.q  r2, #IR_OP_ADD
    beq     r1, r2, .cli_add
    move.q  r2, #IR_OP_SUB
    beq     r1, r2, .cli_sub
    move.q  r2, #IR_OP_MUL
    beq     r1, r2, .cli_mul
    move.q  r2, #IR_OP_BRANCH
    beq     r1, r2, .cli_branch
    bra     .cli_unsupported
.cli_const:
    move.q  r3, #LOWER_OP_CONST
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
.cli_emit:
    add.q   r4, r19, #LOWER_RECORD_BYTES
    bgt     r4, r20, .cli_oom
    store.q r3, (r19)
    load.q  r5, 8(r18)
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
