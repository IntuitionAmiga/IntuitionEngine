; ehbasic_compiler_ir.inc - typed linear IR for the replacement BASIC compiler

IR_RECORD_BYTES       equ 64
IR_OP_BLOCK           equ 1
IR_OP_TOKEN           equ 2
IR_OP_CONST           equ 3
IR_OP_MOVE            equ 4
IR_OP_ADD             equ 5
IR_OP_SUB             equ 6
IR_OP_MUL             equ 7
IR_OP_CONVERT         equ 8
IR_OP_TAG_CHECK       equ 9
IR_OP_BRANCH          equ 10
IR_OP_BRANCH_FALSE    equ 11
IR_OP_BOUNDS_CHECK    equ 12
IR_OP_ADDRESS         equ 13
IR_OP_LOAD_SCALAR     equ 14
IR_OP_STORE_SCALAR    equ 15
IR_OP_FOR             equ 16
IR_OP_NEXT            equ 17
IR_OP_DEAD            equ 18

IR_OP_MAX             equ IR_OP_DEAD

IR_TYPE_UNKNOWN       equ 0
IR_TYPE_I64           equ 1
IR_TYPE_FP64          equ 2
IR_TYPE_STRING        equ 3
IR_TYPE_ADDRESS       equ 4

IR_EFFECT_NONE        equ 0
IR_EFFECT_BASIC       equ 1
IR_EFFECT_RAM         equ 2
IR_EFFECT_MMIO        equ 3
IR_EFFECT_CONTROL     equ 4
IR_EFFECT_FULL        equ 5

IR_FLAG_ARENA_ONLY    equ 1
IR_FLAG_REMOVED       equ 2
IR_FLAG_CONSTANT      equ 4
IR_FLAG_LOOP_INVARIANT equ 8
IR_FLAG_BOUNDS_PROVEN equ 16
IR_FLAG_RESIDENT      equ 32
IR_FLAG_REACHABLE     equ 64

; Operand/result packing for the compact record. IDs are unsigned 16-bit
; compiler values: result in bits 0..15, lhs in 16..31 and rhs in 32..47.
IR_VALUE_MASK         equ 0xFFFF

; The temporary compiler context is reserved in ie64.inc so its ownership is
; visible beside the legacy AOT state with which it must coexist. Records and
; their bounds remain in the allocated arena.

; R8=start, R9=end. Returns R8=0, or R8=1 for an invalid range.
compiler_ir_init:
    blt     r8, r0, .cii_bad
    ble     r9, r8, .cii_bad
    la      r1, COMPILER_IR_BASE
    store.q r8, (r1)
    la      r1, COMPILER_IR_CURSOR
    store.q r8, (r1)
    la      r1, COMPILER_IR_END
    store.q r9, (r1)
    la      r1, COMPILER_IR_COUNT
    store.q r0, (r1)
    move.q  r8, r0
    rts
.cii_bad:
    move.q  r8, #1
    rts

; Append one complete record.
; R9=operation, R10=type, R11=effect, R12=line, R13=token offset,
; R14=operand/result payload, R15=branch target/helper, R7=flags.
; Returns R8=status (0 success, 1 full, 2 invalid metadata), R9=record or line.
compiler_ir_append:
    move.q  r6, #IR_OP_BLOCK
    blt     r9, r6, .cia_invalid
    move.q  r6, #IR_OP_MAX
    bgt     r9, r6, .cia_invalid
    blt     r10, r0, .cia_invalid
    move.q  r6, #IR_TYPE_ADDRESS
    bgt     r10, r6, .cia_invalid
    blt     r11, r0, .cia_invalid
    move.q  r6, #IR_EFFECT_FULL
    bgt     r11, r6, .cia_invalid
    la      r1, COMPILER_IR_CURSOR
    load.q  r2, (r1)
    add.q   r3, r2, #IR_RECORD_BYTES
    blt     r3, r2, .cia_full
    la      r4, COMPILER_IR_END
    load.q  r5, (r4)
    bgt     r3, r5, .cia_full
    store.q r9, (r2)
    store.q r10, 8(r2)
    store.q r11, 16(r2)
    store.q r12, 24(r2)
    store.q r13, 32(r2)
    store.q r14, 40(r2)
    store.q r15, 48(r2)
    store.q r7, 56(r2)
    store.q r3, (r1)
    la      r4, COMPILER_IR_COUNT
    load.q  r5, (r4)
    add.q   r5, r5, #1
    store.q r5, (r4)
    move.q  r9, r2
    move.q  r8, r0
    rts
.cia_full:
    move.q  r9, r12
    move.q  r8, #1
    rts
.cia_invalid:
    move.q  r9, r12
    move.q  r8, #2
    rts
