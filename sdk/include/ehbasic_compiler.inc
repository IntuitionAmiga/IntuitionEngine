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
.cpp_token:
    bge     r23, r19, .cpp_malformed_line
    bnez    r21, .cpp_have_next_bound
    bra     .cpp_read_token
.cpp_have_next_bound:
    bge     r23, r21, .cpp_malformed_line
.cpp_read_token:
    load.b  r14, (r23)
    beqz    r14, .cpp_next_line
    move.q  r7, r0
    move.q  r1, #TK_LOAD
    bne     r14, r1, .cpp_flags_ready
    move.q  r7, #IR_FLAG_ARENA_ONLY
.cpp_flags_ready:
    move.q  r9, #IR_OP_TOKEN
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
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
    move.q  r10, r24
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
