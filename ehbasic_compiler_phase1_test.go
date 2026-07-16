package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func assembleCompilerUnit(t *testing.T, body string) []byte {
	t.Helper()
	asmBin := buildAssembler(t)
	source := fmt.Sprintf(`include "ie64.inc"
include "ehbasic_tokens.inc"

    org 0x1000
test_entry:
    la      r31, STACK_TOP
    la      r16, BASIC_STATE
%s
    halt

include "ehbasic_compiler_ir.inc"
include "ehbasic_compiler.inc"
`, body)
	dir := t.TempDir()
	src := filepath.Join(dir, "compiler_unit.asm")
	if err := os.WriteFile(src, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(asmBin, "-I", filepath.Join(repoRootDir(t), "sdk", "include"), src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assembly failed: %v\n%s\n%s", err, out, source)
	}
	bin, err := os.ReadFile(filepath.Join(dir, "compiler_unit.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

func runCompilerUnit(t *testing.T, body string) *ehbasicTestHarness {
	t.Helper()
	h := newEhbasicHarness(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	h.loadBytes(assembleCompilerUnit(t, body))
	h.runCycles(1_000_000)
	return h
}

func TestIE64BasicCompilerArenaLayout(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x030000       ; descriptor
    la      r9, 0x050000       ; stored programme start
    la      r10, 0x060000      ; stored programme end
    move.q  r17, #120
    jsr     compiler_arena_init
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
`)
	if got := h.bus.Read64(0x032000); got != 0 {
		t.Fatalf("status = %d, want success", got)
	}
	if got := h.bus.Read64(0x032008); got != 0 {
		t.Fatalf("diagnostic line = %d, want zero", got)
	}
	const descriptor = 0x030000
	previousEnd := uint64(0)
	for i := uint64(0); i < 7; i++ {
		start := h.bus.Read64(uint32(descriptor + 24 + i*24))
		cursor := h.bus.Read64(uint32(descriptor + 32 + i*24))
		end := h.bus.Read64(uint32(descriptor + 40 + i*24))
		if start == 0 || start != cursor || start >= end || start&15 != 0 || end&15 != 0 {
			t.Fatalf("region %d invalid: start=%#x cursor=%#x end=%#x", i, start, cursor, end)
		}
		if i > 0 && start != previousEnd {
			t.Fatalf("region %d starts at %#x, previous ended at %#x", i, start, previousEnd)
		}
		previousEnd = end
	}
	if previousEnd > aotTestGuestRAM {
		t.Fatalf("arena end %#x exceeds active RAM %#x", previousEnd, uint64(aotTestGuestRAM))
	}
}

func TestIE64BasicCompilerArenaRejectsInvalidRangeWithoutProgrammeMutation(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    move.q  r2, #0x11223344
    store.q r2, (r1)
    la      r8, 0x030000
    move.q  r9, #-1
    la      r10, 0x060000
    move.q  r17, #440
    jsr     compiler_arena_init
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
`)
	if got := h.bus.Read64(0x032000); got != 1 {
		t.Fatalf("status = %d, want invalid range", got)
	}
	if got := h.bus.Read64(0x032008); got != 440 {
		t.Fatalf("diagnostic line = %d, want 440", got)
	}
	if got := h.bus.Read64(0x050000); got != 0x11223344 {
		t.Fatalf("stored programme mutated: %#x", got)
	}
}

func TestIE64BasicCompilerArenaAllocationIsTransactional(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x030000
    la      r9, 0x050000
    la      r10, 0x060000
    move.q  r17, #70
    jsr     compiler_arena_init
    la      r8, 0x030000
    move.q  r9, #0
    move.q  r10, #33
    move.q  r17, #80
    jsr     compiler_arena_alloc
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    store.q r10, 16(r1)
    la      r8, 0x030000
    move.q  r9, #0
    move.q  r10, #-1
    move.q  r17, #90
    jsr     compiler_arena_alloc
    la      r1, 0x032000
    store.q r8, 24(r1)
    store.q r9, 32(r1)
    store.q r10, 40(r1)
    la      r1, 0x030000
    load.q  r2, 32(r1)
    la      r1, 0x032000
    store.q r2, 48(r1)
`)
	start := h.bus.Read64(0x030018)
	if status, address, line := h.bus.Read64(0x032000), h.bus.Read64(0x032008), h.bus.Read64(0x032010); status != 0 || address != start || line != 0 {
		t.Fatalf("allocation = (%d,%#x,%d), want (0,%#x,0)", status, address, line, start)
	}
	wantCursor := start + 48
	if status, line := h.bus.Read64(0x032018), h.bus.Read64(0x032028); status != 1 || line != 90 {
		t.Fatalf("overflow diagnostic = (%d,%d), want (1,90)", status, line)
	}
	if got := h.bus.Read64(0x032030); got != wantCursor {
		t.Fatalf("cursor after rejected allocation = %#x, want %#x", got, wantCursor)
	}
}

func TestIE64BasicCompilerIRAppendAndBounds(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x034080
    jsr     compiler_ir_init
    la      r8, 0x034000
    move.q  r9, #IR_OP_TOKEN
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #10
    move.q  r13, #3
    move.q  r14, #0x91
    jsr     compiler_ir_append
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    la      r8, 0x034000
    move.q  r9, #IR_OP_TOKEN
    move.q  r10, #IR_TYPE_STRING
    move.q  r11, #IR_EFFECT_RAM
    move.q  r12, #20
    move.q  r13, #4
    move.q  r14, #0x92
    jsr     compiler_ir_append
    la      r1, 0x032000
    store.q r8, 16(r1)
    store.q r9, 24(r1)
    la      r8, 0x034000
    move.q  r9, #IR_OP_TOKEN
    move.q  r10, #99
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #30
    move.q  r13, #5
    move.q  r14, #0
    jsr     compiler_ir_append
    la      r1, 0x032000
    store.q r8, 32(r1)
    store.q r9, 40(r1)
`)
	if h.bus.Read64(0x032000) != 0 || h.bus.Read64(0x032008) != 0x034000 {
		t.Fatalf("first append failed")
	}
	if h.bus.Read64(0x032010) != 0 || h.bus.Read64(0x032018) != 0x034040 {
		t.Fatalf("second append failed")
	}
	if h.bus.Read64(0x032020) != 2 || h.bus.Read64(0x032028) != 30 {
		t.Fatalf("invalid type diagnostic = (%d,%d), want (2,30)", h.bus.Read64(0x032020), h.bus.Read64(0x032028))
	}
	if got := h.bus.Read64(0x034080); got != 0 {
		t.Fatalf("out-of-bounds record written: %#x", got)
	}
}

func TestIE64BasicCompilerIRContextCoexistsWithLegacyAOTState(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, AOT_LAST_LINE
    move.q  r2, #0x1111
    store.q r2, (r1)
    la      r1, AOT_LAST_TOKEN
    move.q  r2, #0x2222
    store.q r2, (r1)
    la      r1, AOT_EMIT_OVERFLOW
    move.q  r2, #0x3333
    store.q r2, (r1)
    la      r1, AOT_NATIVE_TYPES_OFF
    move.q  r2, #0x4444
    store.q r2, (r1)

    la      r8, 0x034000
    la      r9, 0x034080
    jsr     compiler_ir_init
    la      r8, 0x034000
    move.q  r9, #IR_OP_TOKEN
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #100
    move.q  r13, #2
    move.q  r14, #TK_END
    jsr     compiler_ir_append
    la      r8, 0x034000
    move.q  r9, #COMPILER_TARGET_ARENA
    jsr     compiler_validate_target

    la      r1, 0x032000
    la      r2, AOT_LAST_LINE
    load.q  r3, (r2)
    store.q r3, (r1)
    la      r2, AOT_LAST_TOKEN
    load.q  r3, (r2)
    store.q r3, 8(r1)
    la      r2, AOT_EMIT_OVERFLOW
    load.q  r3, (r2)
    store.q r3, 16(r1)
    la      r2, AOT_NATIVE_TYPES_OFF
    load.q  r3, (r2)
    store.q r3, 24(r1)
`)
	want := []uint64{0x1111, 0x2222, 0x3333, 0x4444}
	for i, value := range want {
		if got := h.bus.Read64(uint32(0x032000 + i*8)); got != value {
			t.Fatalf("legacy AOT state cell %d = %#x, want sentinel %#x", i, got, value)
		}
	}
}

func TestIE64BasicCompilerParseAndTargetValidation(t *testing.T) {
	h := runCompilerUnit(t, `
    ; Two stored lines: 10 LOAD, 20 END. Header is next qword, line dword,
    ; reserved dword, then NUL-terminated token content.
    la      r1, 0x050000
    la      r2, 0x050020
    store.q r2, (r1)
    move.q  r2, #10
    store.l r2, 8(r1)
    move.q  r2, #TK_LOAD
    store.b r2, 16(r1)
    store.b r0, 17(r1)
    la      r1, 0x050020
    store.q r0, (r1)
    move.q  r2, #20
    store.l r2, 8(r1)
    move.q  r2, #TK_END
    store.b r2, 16(r1)
    store.b r0, 17(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050040
    la      r10, 0x034000
    jsr     compiler_parse_program
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    la      r8, 0x034000
    move.q  r9, #COMPILER_TARGET_ARENA
    jsr     compiler_validate_target
    la      r1, 0x032000
    store.q r8, 16(r1)
    store.q r9, 24(r1)
    store.q r10, 32(r1)
    la      r8, 0x034000
    move.q  r9, #COMPILER_TARGET_STANDALONE
    jsr     compiler_validate_target
    la      r1, 0x032000
    store.q r8, 40(r1)
    store.q r9, 48(r1)
    store.q r10, 56(r1)
`)
	if got := h.bus.Read64(0x032000); got != 0 {
		t.Fatalf("parse status = %d, line %d", got, h.bus.Read64(0x032008))
	}
	if got := h.bus.Read64(0x032010); got != 0 {
		t.Fatalf("arena validation status = %d", got)
	}
	if got, line, off := h.bus.Read64(0x032028), h.bus.Read64(0x032030), h.bus.Read64(0x032038); got != 3 || line != 10 || off != 0 {
		t.Fatalf("standalone diagnostic = (%d,%d,%d), want (3,10,0)", got, line, off)
	}
}
