package main

import "testing"

func TestIE64BasicCompilerMilestone1ConstantFoldAndTypePropagation(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #10
    move.q  r13, #1
    move.q  r14, #1
    move.q  r15, #20
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #10
    move.q  r13, #2
    move.q  r14, #2
    move.q  r15, #22
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_ADD
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #10
    move.q  r13, #3
    move.q  r14, #2
    lsl.q   r14, r14, #32
    add.q   r14, r14, #0x10003
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    la      r1, 0x032000
    store.q r8, (r1)
    la      r2, 0x034080
    load.q  r3, (r2)
    store.q r3, 8(r1)
    load.q  r3, 8(r2)
    store.q r3, 16(r1)
    load.q  r3, 48(r2)
    store.q r3, 24(r1)
    load.q  r3, 56(r2)
    store.q r3, 32(r1)
`)
	if got := h.bus.Read64(0x032000); got != 0 {
		t.Fatalf("optimiser status = %d, want success", got)
	}
	if op := h.bus.Read64(0x032008); op != 3 {
		t.Fatalf("folded operation = %d, want IR_OP_CONST", op)
	}
	if typ := h.bus.Read64(0x032010); typ != 1 {
		t.Fatalf("folded type = %d, want IR_TYPE_I64", typ)
	}
	if value := h.bus.Read64(0x032018); value != 42 {
		t.Fatalf("folded value = %d, want 42", value)
	}
	if flags := h.bus.Read64(0x032020); flags&4 == 0 {
		t.Fatalf("folded flags = %#x, want constant", flags)
	}
}

func TestIE64BasicCompilerMilestone1DoesNotFoldUnresolvedOperand(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #20
    move.q  r13, #1
    move.q  r14, #1
    move.q  r15, #9
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, #20
    move.q  r13, #2
    move.q  r14, #2
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_ADD
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #20
    move.q  r13, #3
    move.q  r14, #2
    lsl.q   r14, r14, #32
    add.q   r14, r14, #0x10003
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	if op := h.bus.Read64(0x034080); op != 5 {
		t.Fatalf("mixed constant/runtime operation = %d, want IR_OP_ADD retained", op)
	}
	if typ := h.bus.Read64(0x034088); typ != 0 {
		t.Fatalf("mixed constant/runtime type = %d, want IR_TYPE_UNKNOWN", typ)
	}
}

func TestIE64BasicCompilerPhase2ClassifiesTokenSemantics(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #30
    store.l r2, 8(r1)
    move.q  r2, #TK_PLUS
    store.b r2, 16(r1)
    move.q  r2, #TK_LET
    store.b r2, 17(r1)
    move.q  r2, #TK_GOTO
    store.b r2, 18(r1)
    move.q  r2, #TK_VSYNC_CMD
    store.b r2, 19(r1)
    move.q  r2, #TK_LOAD
    store.b r2, 20(r1)
    store.b r0, 21(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050040
    la      r10, 0x034000
    jsr     compiler_parse_program
`)
	tests := []struct {
		name   string
		index  uint32
		typ    uint64
		effect uint64
		flags  uint64
	}{
		{"numeric operator", 1, 0, 0, 0},
		{"state statement", 2, 0, 1, 0},
		{"control statement", 3, 0, 4, 0},
		{"MMIO statement", 4, 0, 3, 0},
		{"arena-only statement", 5, 0, 5, 1},
	}
	for _, tc := range tests {
		record := uint32(0x034000) + tc.index*64
		if typ, effect, flags := h.bus.Read64(record+8), h.bus.Read64(record+16), h.bus.Read64(record+56); typ != tc.typ || effect != tc.effect || flags != tc.flags {
			t.Errorf("%s metadata = (type %d, effect %d, flags %#x), want (%d,%d,%#x)", tc.name, typ, effect, flags, tc.typ, tc.effect, tc.flags)
		}
	}
}

func TestIE64BasicCompilerPhase3HelperMetadataClosure(t *testing.T) {
	h := runCompilerUnit(t, `
    move.q  r8, #COMPILER_HELPER_PRINT_NUM
    jsr     compiler_helper_lookup
    la      r1, 0x032000
    store.q r8, (r1)
    load.q  r2, (r9)
    store.q r2, 8(r1)
    move.q  r8, #(1 << COMPILER_HELPER_PRINT_NUM)
    la      r9, 0x033000
    move.q  r10, #8
    jsr     compiler_helper_closure
    la      r1, 0x032000
    store.q r8, 16(r1)
    store.q r9, 24(r1)
    store.q r10, 32(r1)
    move.q  r8, #63
    jsr     compiler_helper_lookup
    la      r1, 0x032000
    store.q r8, 40(r1)
`)
	if status, id := h.bus.Read64(0x032000), h.bus.Read64(0x032008); status != 0 || id != 3 {
		t.Fatalf("helper lookup = (%d,%d), want print helper metadata", status, id)
	}
	if status, count, mask := h.bus.Read64(0x032010), h.bus.Read64(0x032018), h.bus.Read64(0x032020); status != 0 || count != 3 || mask != 0xE {
		t.Fatalf("helper closure = (%d,%d,%#x), want (0,3,0xe)", status, count, mask)
	}
	for i, want := range []uint64{1, 2, 3} {
		if got := h.bus.Read64(uint32(0x033000 + i*8)); got != want {
			t.Fatalf("helper order[%d] = %d, want %d", i, got, want)
		}
	}
	if status := h.bus.Read64(0x032028); status != 1 {
		t.Fatalf("missing helper status = %d, want unavailable", status)
	}
}

func TestIE64BasicCompilerPhase4DirectLoweringRejectsTokens(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #40
    move.q  r13, #1
    move.q  r14, #1
    move.q  r15, #7
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_ADD
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #40
    move.q  r13, #2
    move.q  r14, #0x10002
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_ARENA
    jsr     compiler_lower_ir
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)

    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_TOKEN
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, #90
    move.q  r13, #6
    move.q  r14, #TK_PRINT
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_ARENA
    jsr     compiler_lower_ir
    la      r1, 0x032000
    store.q r8, 16(r1)
    store.q r9, 24(r1)
    store.q r10, 32(r1)
`)
	if status, count := h.bus.Read64(0x032000), h.bus.Read64(0x032008); status != 0 || count != 2 {
		t.Fatalf("direct lowering = (%d,%d), want two operations", status, count)
	}
	if h.bus.Read64(0x036000) != 1 || h.bus.Read64(0x036020) != 2 {
		t.Fatalf("lowered stream does not contain direct CONST, ADD operations")
	}
	if status, line, offset := h.bus.Read64(0x032010), h.bus.Read64(0x032018), h.bus.Read64(0x032020); status != 5 || line != 90 || offset != 6 {
		t.Fatalf("unsupported token diagnostic = (%d,%d,%d), want (5,90,6)", status, line, offset)
	}
}

func TestIE64BasicCompilerPhase5RemovesOnlyProvenRedundancies(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_TAG_CHECK
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #50
    move.q  r13, #1
    move.q  r14, #1
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_BOUNDS_CHECK
    move.q  r10, #IR_TYPE_ADDRESS
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #50
    move.q  r13, #2
    move.q  r14, #2
    move.q  r15, r0
    move.q  r7, #IR_FLAG_BOUNDS_PROVEN
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_BRANCH
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, #50
    move.q  r13, #3
    move.q  r14, r0
    move.q  r15, #99
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_BLOCK
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, #60
    move.q  r13, r0
    move.q  r14, #99
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	for _, address := range []uint32{0x034000, 0x034040, 0x034080} {
		if flags := h.bus.Read64(address + 56); flags&2 == 0 {
			t.Errorf("record at %#x flags %#x, want IR_FLAG_REMOVED", address, flags)
		}
	}
	if flags := h.bus.Read64(0x0340c0 + 56); flags&2 != 0 {
		t.Fatalf("branch target block was removed: flags %#x", flags)
	}
}

func TestIE64BasicCompilerOptimiserUsesLatestNonSSAValueDefinition(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #80
    move.q  r13, #1
    move.q  r14, #1
    move.q  r15, #10
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, #80
    move.q  r13, #2
    move.q  r14, #1
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #80
    move.q  r13, #3
    move.q  r14, #2
    move.q  r15, #5
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_ADD
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #80
    move.q  r13, #4
    move.q  r14, #2
    lsl.q   r14, r14, #32
    add.q   r14, r14, #0x10003
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	if op := h.bus.Read64(0x0340c0); op != 5 {
		t.Fatalf("addition after non-constant redefinition became operation %d, want IR_OP_ADD", op)
	}
}

func TestIE64BasicCompilerPhase2ExpressionFunctionEffects(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #100
    store.l r2, 8(r1)
    move.q  r2, #TK_USR
    store.b r2, 16(r1)
    move.q  r2, #TK_RND
    store.b r2, 17(r1)
    move.q  r2, #TK_PEEK
    store.b r2, 18(r1)
    store.b r0, 19(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050040
    la      r10, 0x034000
    jsr     compiler_parse_program
`)
	for i, want := range []uint64{5, 1, 2} {
		if got := h.bus.Read64(uint32(0x034000+(i+1)*64) + 16); got != want {
			t.Errorf("function %d effect = %d, want %d", i, got, want)
		}
	}
}

func TestIE64BasicCompilerPhase3RejectsHelperBitZero(t *testing.T) {
	h := runCompilerUnit(t, `
    move.q  r8, #1
    la      r9, 0x033000
    move.q  r10, #8
    jsr     compiler_helper_closure
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    store.q r10, 16(r1)
`)
	if status, count, mask := h.bus.Read64(0x032000), h.bus.Read64(0x032008), h.bus.Read64(0x032010); status != 1 || count != 0 || mask != 0 {
		t.Fatalf("bit-zero helper request = (%d,%d,%#x), want rejected", status, count, mask)
	}
}
