package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func compilerCorpusBody(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := make(map[uint64]string)
	for _, raw := range strings.Split(string(data), "\n") {
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			continue
		}
		line, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			continue
		}
		lines[line] = strings.TrimSpace(strings.TrimPrefix(raw, fields[0]))
	}
	lineNumbers := make([]uint64, 0, len(lines))
	for line := range lines {
		lineNumbers = append(lineNumbers, line)
	}
	sort.Slice(lineNumbers, func(i, j int) bool { return lineNumbers[i] < lineNumbers[j] })
	var entries, sources strings.Builder
	for count, line := range lineNumbers {
		text := lines[line]
		fmt.Fprintf(&entries, "    dc.q .corpus_source_%d, %d\n", count, line)
		fmt.Fprintf(&sources, ".corpus_source_%d:\n    dc.b ", count)
		for _, b := range []byte(text) {
			fmt.Fprintf(&sources, "%d,", b)
		}
		sources.WriteString("0\n")
	}
	entries.WriteString("    dc.q 0, 0\n")
	return fmt.Sprintf(`
    la      r18, 0x200000
    move.q  r19, r0
    la      r20, .corpus_table
.corpus_tokenise:
    load.q  r8, (r20)
    beqz    r8, .corpus_compile
    load.q  r21, 8(r20)
    la      r9, 0x1F0000
    jsr     tokenize
    move.q  r22, r18
    beqz    r19, .corpus_header
    store.q r22, (r19)
.corpus_header:
    store.q r0, (r22)
    store.l r21, 8(r22)
    store.l r0, 12(r22)
    la      r1, 0x1F0000
    add.q   r2, r22, #16
    add.q   r3, r8, #1
.corpus_copy:
    load.b  r4, (r1)
    store.b r4, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    sub.q   r3, r3, #1
    bnez    r3, .corpus_copy
    add.q   r2, r2, #7
    and.q   r18, r2, #-8
    move.q  r19, r22
    add.q   r20, r20, #16
    bra     .corpus_tokenise
.corpus_compile:
    la      r8, 0x400000
    la      r9, 0x800000
    jsr     compiler_ir_init
    la      r8, 0x200000
    move.q  r9, r18
    la      r10, 0x400000
    jsr     compiler_parse_program
    bnez    r8, .corpus_save
    la      r8, 0x400000
    jsr     compiler_optimise_ir
    la      r8, 0x400000
    la      r9, 0x800000
    la      r10, 0xA00000
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
.corpus_save:
    la      r1, 0x1E0000
    store.q r8, (r1)
    store.q r9, 8(r1)
    store.q r10, 16(r1)
    bnez    r8, .corpus_after_compile
    la      r8, 0x800000
    la      r10, 0xA00000
    la      r11, 0x1D00000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
    la      r1, 0x1E0020
    store.q r8, (r1)
    store.q r9, 8(r1)
    store.q r10, 16(r1)
.corpus_after_compile:
    bra     .corpus_after_data
.corpus_table:
%s%s.corpus_after_data:
`, entries.String(), sources.String())
}

func TestIE64BasicCompilerMilestone1FoldsIntegerArithmetic(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r18, #1
.constants:
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #10
    move.q  r13, r18
    move.q  r14, r18
    move.q  r15, r18
    add.q   r15, r15, #5
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    add.q   r18, r18, #1
    move.q  r1, #3
    ble     r18, r1, .constants

    ; v4 = v1 + v2, v5 = v3 - v1, v6 = v2 * v3
    move.q  r18, #IR_OP_ADD
    move.q  r19, #1
    move.q  r20, #2
    move.q  r21, #4
.operation:
    move.q  r9, r18
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #10
    move.q  r13, r21
    move.q  r14, r20
    lsl.q   r14, r14, #32
    lsl.q   r1, r19, #16
    or.q    r14, r14, r1
    or.q    r14, r14, r21
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    add.q   r18, r18, #1
    add.q   r21, r21, #1
    move.q  r1, #IR_OP_SUB
    bne     r18, r1, .set_mul_operands
    move.q  r19, #3
    move.q  r20, #1
    bra     .operation
.set_mul_operands:
    move.q  r19, #2
    move.q  r20, #3
    move.q  r1, #IR_OP_MUL
    ble     r18, r1, .operation

    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	for i, want := range []uint64{13, 2, 56} {
		record := uint32(0x0340c0 + i*64)
		if op, typ, value := h.bus.Read64(record), h.bus.Read64(record+8), h.bus.Read64(record+48); op != 3 || typ != 1 || value != want {
			t.Errorf("operation %d = (op %d, type %d, value %d), want constant I64 %d", i, op, typ, value, want)
		}
	}
}

func TestIE64BasicCompilerMilestone1PropagatesConstantsThroughCopies(t *testing.T) {
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
    move.q  r15, #73
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_MOVE
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #20
    move.q  r13, #2
    move.q  r14, #0x10002
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_CONVERT
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #20
    move.q  r13, #3
    move.q  r14, #0x20003
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	for i := 1; i <= 2; i++ {
		record := uint32(0x034000 + i*64)
		if op, typ, value := h.bus.Read64(record), h.bus.Read64(record+8), h.bus.Read64(record+48); op != 3 || typ != 1 || value != 73 {
			t.Errorf("copy %d = (op %d, type %d, value %d), want constant I64 73", i, op, typ, value)
		}
	}
}

func TestIE64BasicCompilerMilestone1RemovesUnreachableBlocks(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r18, #1
.block:
    move.q  r9, #IR_OP_BLOCK
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r18
    move.q  r13, r0
    move.q  r14, r18
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r1, #1
    bne     r18, r1, .value
    move.q  r9, #IR_OP_BRANCH
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, r18
    move.q  r13, r0
    move.q  r14, r0
    move.q  r15, #3
    move.q  r7, r0
    jsr     compiler_ir_append
    bra     .next
.value:
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, r18
    move.q  r13, r0
    move.q  r14, r18
    move.q  r15, r18
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
.next:
    add.q   r18, r18, #1
    move.q  r1, #3
    ble     r18, r1, .block
    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	// Layout: block 1, branch, block 2, const, block 3, const.
	for _, record := range []uint32{0x034080, 0x0340c0} {
		if flags := h.bus.Read64(record + 56); flags&2 == 0 {
			t.Errorf("unreachable record at %#x flags %#x, want removed", record, flags)
		}
	}
	for _, record := range []uint32{0x034000, 0x034040, 0x034100} {
		if flags := h.bus.Read64(record + 56); flags&2 != 0 {
			t.Errorf("reachable record at %#x was removed, flags %#x", record, flags)
		}
	}
}

func TestIE64BasicCompilerSemanticParserBuildsAssignmentExpressionIR(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050020
    move.q  r10, #120
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    store.q r10, 16(r1)
    bra     .done
.line:
    dc.b TK_LET, 0x20, 0x41, 0x3D, 0x32, TK_PLUS, 0x33, TK_MULT, 0x34, 0
.done:
`)
	if status, line, offset := h.bus.Read64(0x032000), h.bus.Read64(0x032008), h.bus.Read64(0x032010); status != 0 || line != 0 || offset != 0 {
		t.Fatalf("semantic parse = (%d,%d,%d), want success", status, line, offset)
	}
	wantOps := []uint64{3, 3, 3, 7, 5, 15}
	for i, want := range wantOps {
		if got := h.bus.Read64(uint32(0x034000 + i*64)); got != want {
			t.Fatalf("IR operation %d = %d, want %d", i, got, want)
		}
	}
	if value := h.bus.Read64(0x034000 + 2*64 + 48); value != 4 {
		t.Fatalf("third constant = %d, want 4", value)
	}
	if symbol := h.bus.Read64(0x034000 + 5*64 + 48); symbol != 'A' {
		t.Fatalf("store symbol = %#x, want A", symbol)
	}
}

func TestIE64BasicCompilerSemanticParserHandlesScalarsParenthesesAndDivision(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050020
    move.q  r10, #130
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    store.q r10, 16(r1)
    bra     .done
.line:
    dc.b 0x41, 0x3D, 0x28, 0x42, TK_PLUS, 0x36, 0x29, TK_DIV, 0x32, 0
.done:
`)
	if status, line, offset := h.bus.Read64(0x032000), h.bus.Read64(0x032008), h.bus.Read64(0x032010); status != 0 || line != 0 || offset != 0 {
		t.Fatalf("semantic parse = (%d,%d,%d), want success", status, line, offset)
	}
	for i, want := range []uint64{14, 3, 5, 3, 19, 15} {
		if got := h.bus.Read64(uint32(0x034000 + i*64)); got != want {
			t.Fatalf("IR operation %d = %d, want %d", i, got, want)
		}
	}
}

func TestIE64BasicCompilerSemanticParserBuildsComparisonIR(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050020
    move.q  r10, #140
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.line:
    dc.b 0x41, 0x3D, 0x42, TK_LT, TK_EQUAL, 0x31, 0x30, 0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("semantic comparison parse status = %d, want success", status)
	}
	for i, want := range []uint64{14, 3, 23, 15} {
		if got := h.bus.Read64(uint32(0x034000 + i*64)); got != want {
			t.Fatalf("IR operation %d = %d, want %d", i, got, want)
		}
	}
}

func TestIE64BasicCompilerSemanticProgrammeLowersWithoutTokenFallback(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #150
    store.l r2, 8(r1)
    move.q  r2, #TK_LET
    store.b r2, 16(r1)
    move.q  r2, #0x20
    store.b r2, 17(r1)
    move.q  r2, #0x41
    store.b r2, 18(r1)
    move.q  r2, #0x3D
    store.b r2, 19(r1)
    move.q  r2, #0x32
    store.b r2, 20(r1)
    move.q  r2, #TK_PLUS
    store.b r2, 21(r1)
    move.q  r2, #0x33
    store.b r2, 22(r1)
    store.b r0, 23(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050100
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_ARENA
    jsr     compiler_lower_ir
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
`)
	if status, count := h.bus.Read64(0x032000), h.bus.Read64(0x032008); status != 0 || count != 5 {
		t.Fatalf("semantic programme lowering = (%d,%d), want label and four direct records", status, count)
	}
	for i, want := range []uint64{19, 1, 1, 2, 14} {
		if got := h.bus.Read64(uint32(0x036000 + i*32)); got != want {
			t.Fatalf("lowered operation %d = %d, want %d", i, got, want)
		}
	}
}

func TestIE64BasicCompilerParsesColonSeparatedStatementsWithoutMutatingProgramme(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #155
    store.l r2, 8(r1)
    move.q  r2, #0x41
    store.b r2, 16(r1)
    move.q  r2, #0x3D
    store.b r2, 17(r1)
    move.q  r2, #0x31
    store.b r2, 18(r1)
    move.q  r2, #0x3A
    store.b r2, 19(r1)
    move.q  r2, #0x42
    store.b r2, 20(r1)
    move.q  r2, #0x3D
    store.b r2, 21(r1)
    move.q  r2, #0x32
    store.b r2, 22(r1)
    store.b r0, 23(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050100
    la      r10, 0x034000
    jsr     compiler_parse_program
    la      r1, 0x032000
    store.q r8, (r1)
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("colon-separated parse status = %d, want success", status)
	}
	for i, want := range []uint64{1, 3, 15, 3, 15} {
		if got := h.bus.Read64(uint32(0x034000 + i*64)); got != want {
			t.Fatalf("colon-separated IR operation %d = %d, want %d", i, got, want)
		}
	}
	if got := string(h.cpu.memory[0x050000+16 : 0x050000+24]); got != "A=1:B=2\x00" {
		t.Fatalf("compiler mutated stored programme: %q", got)
	}
}

func TestIE64BasicCompilerSemanticParserBuildsControlFlowIR(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    move.q  r2, #TK_GOTO
    store.b r2, (r1)
    move.q  r2, #0x20
    store.b r2, 1(r1)
    move.q  r2, #0x32
    store.b r2, 2(r1)
    move.q  r2, #0x30
    store.b r2, 3(r1)
    store.b r0, 4(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050010
    move.q  r10, #10
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("GOTO semantic parse status = %d, want success", status)
	}
	if op, target := h.bus.Read64(0x034000), h.bus.Read64(0x034030); op != 10 || target != 20 {
		t.Fatalf("GOTO IR = (op %d, target %d), want branch to line 20", op, target)
	}
}

func TestIE64BasicCompilerEmitsCanonicalStandaloneAssembly(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #160
    store.l r2, 8(r1)
    move.q  r2, #0x43
    store.b r2, 16(r1)
    move.q  r2, #0x4F
    store.b r2, 17(r1)
    move.q  r2, #0x55
    store.b r2, 18(r1)
    move.q  r2, #0x4E
    store.b r2, 19(r1)
    move.q  r2, #0x54
    store.b r2, 20(r1)
    move.q  r2, #0x3D
    store.b r2, 21(r1)
    move.q  r2, #0x32
    store.b r2, 22(r1)
    move.q  r2, #TK_PLUS
    store.b r2, 23(r1)
    move.q  r2, #0x33
    store.b r2, 24(r1)
    store.b r0, 25(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050100
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03C000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("standalone emission = (%d,%d), want non-empty success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	for _, required := range []string{`include "ie64.inc"`, "org PROG_START", "L160:", "jsr compiler_runtime_numeric", "compiler_var_2906259298: dc.q 0"} {
		if !strings.Contains(source, required) {
			t.Errorf("generated source lacks %q\n%s", required, source)
		}
	}
	for _, forbidden := range []string{"ehbasic_aot.inc", "expr_eval", "stmt_jump_table", "incbin"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("generated source contains forbidden dependency %q", forbidden)
		}
	}
	if strings.Contains(source, "compiler_runtime_print_num:") {
		t.Fatal("unreferenced print helper was emitted")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "generated.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("host assembler rejected generated source: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "generated.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(image) < 27*16 {
		t.Fatalf("generated image too short: %d", len(image))
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(100_000)
	count := uint32(PROG_START + len(image) - 27*16)
	if got := run.bus.Read64(count); got != 5 {
		t.Fatalf("standalone scalar COUNT = %d, want 5", got)
	}
}

func TestIE64BasicCompilerStandalonePrintUsesTypedHelper(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #170
    store.l r2, 8(r1)
    move.q  r2, #TK_PRINT
    store.b r2, 16(r1)
    move.q  r2, #0x34
    store.b r2, 17(r1)
    move.q  r2, #0x32
    store.b r2, 18(r1)
    store.b r0, 19(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050100
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03C000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("PRINT emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	if strings.Count(source, "compiler_runtime_print_num:") != 1 {
		t.Fatalf("print helper count = %d, want one", strings.Count(source, "compiler_runtime_print_num:"))
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "print.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble PRINT: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "print.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(100_000)
	if got := run.readOutput(); got != "42\r\n" {
		t.Fatalf("standalone PRINT output = %q, want %q", got, "42\r\n")
	}
}

func TestIE64BasicCompilerStandalonePrintsStringLiteralDirectly(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    store.q r0, (r1)
    move.q  r2, #175
    store.l r2, 8(r1)
    move.q  r2, #TK_PRINT
    store.b r2, 16(r1)
    move.q  r2, #0x22
    store.b r2, 17(r1)
    move.q  r2, #0x4F
    store.b r2, 18(r1)
    move.q  r2, #0x4B
    store.b r2, 19(r1)
    move.q  r2, #0x22
    store.b r2, 20(r1)
    store.b r0, 21(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050100
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03C000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("string PRINT emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	if strings.Contains(source, "compiler_runtime_print_num:") || strings.Contains(source, "exec_do_print") {
		t.Fatalf("string literal PRINT delegated to a numeric or interpreter helper\n%s", source)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "print_string.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble string PRINT: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "print_string.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(100_000)
	if got := run.readOutput(); got != "OK\r\n" {
		t.Fatalf("standalone string PRINT output = %q, want %q", got, "OK\r\n")
	}
}

func TestIE64BasicCompilerStandalonePrintsFP64WithoutTruncation(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, .line
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037000
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line:
    dc.q 0
    dc.l 176, 0
    dc.b TK_PRINT, 0x22, 0x41, 0x22, 0x3B, 0x31, 0x2E, 0x35, 0x2C, 0x22, 0x42, 0x22, 0x3B, TK_NOT, 0x30, 0x3B, 0x22, 0x3A, 0x22, 0x3B, 0x31, TK_OR, 0x32, TK_AND, 0x30, 0x3B, 0x22, 0x3A, 0x22, 0x3B, 0x32, TK_POWER, 0x33, TK_POWER, 0x32, 0x3B, 0x22, 0x3A, 0x22, 0x3B, 0x31, TK_LSHIFT, 0x34, 0x30, 0x3B, 0x22, 0x3A, 0x22, 0x3B, TK_INT, 0x28, TK_PI, 0x29, 0x3B, 0x22, 0x3A, 0x22, 0x3B, TK_SQR, 0x28, 0x39, 0x29, 0x3B, 0x22, 0x3A, 0x22, 0x3B, TK_RND, 0x28, 0x30, 0x29, 0
    align 8
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("FP64 PRINT emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	dir := t.TempDir()
	path := filepath.Join(dir, "print_fp64.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble FP64 PRINT: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "print_fp64.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(150_000)
	if got := run.readOutput(); got != "A1.5\tB-1:1:512:1099511627776:3:3:0.524641\r\n" {
		t.Fatalf("standalone mixed PRINT output = %q, want %q", got, "A1.5\tB-1:1:512:1099511627776:3:3:0.524641\r\n")
	}
}

func TestIE64BasicCompilerSemanticParserBuildsIfThenBranch(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050020
    move.q  r10, #20
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.line:
    dc.b TK_IF, 0x41, TK_EQUAL, 0x31, TK_THEN, 0x34, 0x30, 0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("IF semantic parse status = %d, want success", status)
	}
	for i, want := range []uint64{14, 3, 20, 31} {
		if got := h.bus.Read64(uint32(0x034000 + i*64)); got != want {
			t.Fatalf("IF IR operation %d = %d, want %d", i, got, want)
		}
	}
	if target := h.bus.Read64(0x034000 + 3*64 + 48); target != 40 {
		t.Fatalf("IF target = %d, want 40", target)
	}
}

func TestIE64BasicCompilerOptimiserFoldsDivisionAndComparison(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050020
    move.q  r10, #180
    jsr     compiler_parse_semantic_line
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    bra     .done
.line:
    dc.b 0x41, 0x3D, 0x38, TK_DIV, 0x32, TK_EQUAL, 0x34, 0
.done:
`)
	if op, typ, value := h.bus.Read64(0x034080), h.bus.Read64(0x034088), h.bus.Read64(0x0340b0); op != 3 || typ != 2 || value != 0x4010000000000000 {
		t.Fatalf("division fold = (op %d, type %d, value %#x), want FP64 4.0", op, typ, value)
	}
	if op := h.bus.Read64(0x034100); op != 20 {
		t.Fatalf("mixed FP64/I64 comparison folded without conversion, operation %d", op)
	}
}

func TestIE64BasicCompilerParsesDecimalLiteralAsFP64(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050020
    move.q  r10, #185
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.line:
    dc.b 0x41, 0x3D, 0x31, 0x2E, 0x35, 0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("decimal parse status = %d, want success", status)
	}
	if op, typ, bits := h.bus.Read64(0x034000), h.bus.Read64(0x034008), h.bus.Read64(0x034030); op != 3 || typ != 2 || bits != 0x3ff8000000000000 {
		t.Fatalf("decimal constant = (op %d, type %d, bits %#x), want FP64 1.5", op, typ, bits)
	}
}

func TestIE64BasicCompilerParsesExactHexadecimalLiteral(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, .line
    la      r9, .line_end
    move.q  r10, #186
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.line:
    dc.b 0x41, 0x3D, 0x26, 0x48, 0x46, 0x46, 0x30, 0x30, 0
.line_end:
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("hexadecimal parse status = %d, want success", status)
	}
	if op, typ, value := h.bus.Read64(0x034000), h.bus.Read64(0x034008), h.bus.Read64(0x034030); op != 3 || typ != 1 || value != 0xff00 {
		t.Fatalf("hexadecimal constant = (op %d, type %d, value %#x), want exact I64 %#x", op, typ, value, 0xff00)
	}
}

func TestIE64BasicCompilerOptimiserRemovesOnlyDeadValues(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r18, #1
.constant:
    move.q  r9, #IR_OP_CONST
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #190
    move.q  r13, r18
    move.q  r14, r18
    move.q  r15, r18
    move.q  r7, #IR_FLAG_CONSTANT
    jsr     compiler_ir_append
    add.q   r18, r18, #1
    move.q  r1, #2
    ble     r18, r1, .constant
    move.q  r9, #IR_OP_STORE_SCALAR
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, #190
    move.q  r13, #2
    move.q  r14, #0x20000
    move.q  r15, #0x41
    move.q  r7, r0
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_ir
`)
	if flags := h.bus.Read64(0x034038); flags&2 == 0 {
		t.Fatalf("unused value flags %#x, want removed", flags)
	}
	if flags := h.bus.Read64(0x034078); flags&2 != 0 {
		t.Fatalf("stored value flags %#x, must remain live", flags)
	}
}

func TestIE64BasicCompilerSemanticParserBuildsPairedForNextIR(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    la      r2, 0x050040
    store.q r2, (r1)
    move.q  r2, #10
    store.l r2, 8(r1)
    move.q  r2, #TK_FOR
    store.b r2, 16(r1)
    move.q  r2, #0x49
    store.b r2, 17(r1)
    move.q  r2, #0x3D
    store.b r2, 18(r1)
    move.q  r2, #0x31
    store.b r2, 19(r1)
    move.q  r2, #TK_TO
    store.b r2, 20(r1)
    move.q  r2, #0x33
    store.b r2, 21(r1)
    store.b r0, 22(r1)
    la      r1, 0x050040
    store.q r0, (r1)
    move.q  r2, #20
    store.l r2, 8(r1)
    move.q  r2, #TK_NEXT
    store.b r2, 16(r1)
    move.q  r2, #0x49
    store.b r2, 17(r1)
    store.b r0, 18(r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050100
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    jsr     compiler_optimise_ir
.save:
    la      r1, 0x032000
    store.q r8, (r1)
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("FOR/NEXT parse status = %d, want success", status)
	}
	for i, want := range []uint64{1, 3, 3, 3, 16, 1, 17} {
		if got := h.bus.Read64(uint32(0x034000 + i*64)); got != want {
			t.Fatalf("FOR/NEXT IR operation %d = %d, want %d", i, got, want)
		}
	}
	forRecord := uint32(0x034000 + 4*64)
	nextRecord := uint32(0x034000 + 6*64)
	if flags := h.bus.Read64(forRecord + 56); flags&8 == 0 {
		t.Fatalf("FOR flags %#x, want recognised invariant loop", flags)
	}
	forID := h.bus.Read64(forRecord+48) & 0xffffffff
	nextID := h.bus.Read64(nextRecord+48) & 0xffffffff
	if forID == 0 || nextID != forID {
		t.Fatalf("paired loop identity FOR=%d NEXT=%d, want one non-zero shared identity", forID, nextID)
	}
	if flags := h.bus.Read64(forRecord + 56); flags&32 == 0 {
		t.Fatalf("FOR flags %#x, want safe induction-variable residency; FOR target=%#x NEXT target=%#x NEXT operand=%#x", flags, h.bus.Read64(forRecord+48), h.bus.Read64(nextRecord+48), h.bus.Read64(nextRecord+40))
	}
	if flags := h.bus.Read64(nextRecord + 56); flags&32 == 0 {
		t.Fatalf("NEXT flags %#x, want safe induction-variable residency", flags)
	}
}

func TestIE64BasicCompilerStandaloneExecutesRecognisedScalarForLoop(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037800
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line10:
    dc.q .line20
    dc.l 10, 0
    dc.b 0x53, 0x3D, 0x30, 0
    align 8
.line20:
    dc.q .line30
    dc.l 20, 0
    dc.b TK_FOR, 0x49, 0x3D, 0x31, TK_TO, 0x32, TK_STEP, 0x2E, 0x35, 0
    align 8
.line30:
    dc.q .line40
    dc.l 30, 0
    dc.b 0x53, 0x3D, 0x53, TK_PLUS, 0x49, 0
    align 8
.line40:
    dc.q .line50
    dc.l 40, 0
    dc.b TK_NEXT, 0x49, 0
    align 8
.line50:
    dc.q .line60
    dc.l 50, 0
    dc.b TK_PRINT, 0x53, 0
    align 8
.line60:
    dc.q .line70
    dc.l 60, 0
    dc.b 0x53, 0x3D, 0x30, 0
    align 8
.line70:
    dc.q .line80
    dc.l 70, 0
    dc.b TK_FOR, 0x49, 0x3D, 0x32, TK_TO, 0x31, TK_STEP, TK_MINUS, 0x2E, 0x35, 0
    align 8
.line80:
    dc.q .line90
    dc.l 80, 0
    dc.b 0x53, 0x3D, 0x53, TK_PLUS, 0x49, 0
    align 8
.line90:
    dc.q .line100
    dc.l 90, 0
    dc.b TK_NEXT, 0x49, 0
    align 8
.line100:
    dc.q 0
    dc.l 100, 0
    dc.b TK_PRINT, 0x53, 0
    align 8
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("FOR standalone emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	for _, want := range []string{"compiler resident induction", "compiler resident hot scalar"} {
		if !strings.Contains(source, want) {
			t.Fatalf("recognised scalar FOR loop lacks residency shape %q\n%s", want, source)
		}
	}
	residentLabels := regexp.MustCompile(`(?m)^FRES([0-9]+):$`).FindAllStringSubmatch(source, -1)
	if len(residentLabels) != 2 || residentLabels[0][1] == residentLabels[1][1] {
		t.Fatalf("resident loop labels=%v, want two distinct paired identities\n%s", residentLabels, source)
	}
	for _, label := range residentLabels {
		if !strings.Contains(source, "bra FRES"+label[1]+"\n") {
			t.Fatalf("resident loop %s has no NEXT back-edge\n%s", label[1], source)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "for_loop.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble FOR loop: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "for_loop.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(250_000)
	if got := run.readOutput(); got != "4.5\r\n4.5\r\n" {
		vars := uint32(PROG_START + len(image) - 64 - 26*16)
		t.Fatalf("FOR loop output = %q, want %q; I=%#x S=%#x limit=%#x step=%#x; refs I=%d S=%d add=%d", got, "4.5\r\n4.5\r\n",
			run.bus.Read64(vars+8*16), run.bus.Read64(vars+18*16),
			run.bus.Read64(vars+26*16), run.bus.Read64(vars+26*16+16),
			strings.Count(source, "compiler_var_73"), strings.Count(source, "compiler_var_83"), strings.Count(source, "add.q r8, r1, r2"))
	}
}

func TestIE64BasicCompilerArenaExecutesResidentScalarForLoop(t *testing.T) {
	h, _ := startREPL(t)
	for _, line := range []string{
		"10 S=0",
		"20 FOR I=1 TO 3",
		"30 S=S+I",
		"40 NEXT I",
		"50 POKE64 500000,S",
		"60 END",
	} {
		storeLine(t, h, line)
	}
	out := h.runCommand("RUN AOT")
	if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
		t.Fatalf("resident arena FOR failed: %q", out)
	}
	if got := h.bus.Read64(500000); got != 6 {
		t.Fatalf("resident arena FOR result = %d, want 6; output=%q", got, out)
	}
}

func TestIE64BasicCompilerResidencyRejectsBarrierLoop(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r9, #IR_OP_FOR
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, #10
    move.q  r13, r0
    move.q  r14, r0
    move.q  r15, #0x49
    lsl.q   r15, r15, #32
    add.q   r15, r15, #10
    move.q  r7, #IR_FLAG_LOOP_PAIRED
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_POKE32
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_FULL
    move.q  r12, #20
    move.q  r13, r0
    move.q  r14, r0
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    move.q  r9, #IR_OP_NEXT
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_CONTROL
    move.q  r12, #30
    move.q  r13, r0
    move.q  r14, r0
    move.q  r15, #0x49
    lsl.q   r15, r15, #32
    add.q   r15, r15, #10
    move.q  r7, #IR_FLAG_LOOP_PAIRED
    jsr     compiler_ir_append
    la      r8, 0x034000
    jsr     compiler_optimise_residency
`)
	for _, address := range []uint32{0x034000, 0x034080} {
		if flags := h.bus.Read64(address + 56); flags&32 != 0 {
			t.Fatalf("barrier loop record at %#x became resident: flags=%#x", address, flags)
		}
	}
}

func TestIE64BasicCompilerStandaloneExecutesTypedDataReadAndRestore(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037800
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line10:
    dc.q .line20
    dc.l 10, 0
    dc.b TK_READ, 0x43, 0x4F, 0x55, 0x4E, 0x54, 0x3A, TK_PRINT, 0x43, 0x4F, 0x55, 0x4E, 0x54, 0
    align 8
.line20:
    dc.q .line30
    dc.l 20, 0
    dc.b TK_READ, 0x43, 0x4F, 0x55, 0x4E, 0x54, 0x3A, TK_PRINT, 0x43, 0x4F, 0x55, 0x4E, 0x54, 0
    align 8
.line30:
    dc.q .line40
    dc.l 30, 0
    dc.b TK_RESTORE, 0
    align 8
.line40:
    dc.q .line100
    dc.l 40, 0
    dc.b TK_READ, 0x43, 0x4F, 0x55, 0x4E, 0x54, 0x3A, TK_PRINT, 0x43, 0x4F, 0x55, 0x4E, 0x54, 0
    align 8
.line100:
    dc.q 0
    dc.l 100, 0
    dc.b TK_DATA, TK_MINUS, 0x32, 0x2C, 0x31, 0x2E, 0x35, 0
    align 8
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("DATA standalone emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	for _, forbidden := range []string{"exec_do_read", "expr_eval", "stmt_jump_table"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("DATA/READ source delegates through %q\n%s", forbidden, source)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "data_read.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble DATA/READ: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "data_read.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(250_000)
	if got := run.readOutput(); got != "-2\r\n1.5\r\n-2\r\n" {
		t.Fatalf("typed DATA/READ output = %q, want %q", got, "-2\r\n1.5\r\n-2\r\n")
	}
}

func TestIE64BasicCompilerStandaloneExecutesInlineIfStatementTail(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037800
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line10:
    dc.q .line20
    dc.l 10, 0
    dc.b 0x41, 0x3D, 0x30, 0x3A, 0x42, 0x3D, 0x30, 0x3A, 0x43, 0x3D, 0x30, 0x3A, TK_INC, 0x41, 0x3A, TK_DEC, 0x41, 0
    align 8
.line20:
    dc.q .line30
    dc.l 20, 0
    dc.b TK_IF, 0x30, TK_THEN, 0x41, 0x3D, 0x31, 0x3A, 0x42, 0x3D, 0x32, 0
    align 8
.line30:
    dc.q .line40
    dc.l 30, 0
    dc.b TK_PRINT, 0x41, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x42, 0
    align 8
.line40:
    dc.q .line50
    dc.l 40, 0
    dc.b TK_IF, 0x31, TK_THEN, 0x41, 0x3D, 0x33, 0x3A, 0x42, 0x3D, 0x34, 0
    align 8
.line50:
    dc.q .line60
    dc.l 50, 0
    dc.b TK_PRINT, 0x41, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x42, 0
    align 8
.line60:
    dc.q .line70
    dc.l 60, 0
    dc.b TK_IF, 0x31, TK_THEN, TK_IF, 0x30, TK_THEN, 0x41, 0x3D, 0x39, 0x3A, 0x42, 0x3D, 0x39, 0x3A, 0x43, 0x3D, 0x39, 0
    align 8
.line70:
    dc.q .line80
    dc.l 70, 0
    dc.b TK_PRINT, 0x41, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x42, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x43, 0
    align 8
.line80:
    dc.q .line90
    dc.l 80, 0
    dc.b TK_IF, 0x30, TK_THEN, 0x41, 0x3D, 0x38, TK_ELSE, 0x41, 0x3D, 0x35, 0x3A, 0x42, 0x3D, 0x36, 0
    align 8
.line90:
    dc.q .line100
    dc.l 90, 0
    dc.b TK_PRINT, 0x41, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x42, 0
    align 8
.line100:
    dc.q .line110
    dc.l 100, 0
    dc.b TK_IF, 0x31, TK_THEN, 0x41, 0x3D, 0x37, TK_ELSE, 0x42, 0x3D, 0x39, 0x3A, 0x43, 0x3D, 0x39, 0
    align 8
.line110:
    dc.q .line120
    dc.l 110, 0
    dc.b TK_PRINT, 0x41, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x42, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x43, 0
    align 8
.line120:
    dc.q 0
    dc.l 120, 0
    dc.b TK_SWAP, 0x41, 0x2C, 0x42, 0x3A, TK_PRINT, 0x41, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x42, 0x3B, 0x22, 0x2C, 0x22, 0x3B, 0x43, 0
    align 8
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("inline IF emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	dir := t.TempDir()
	path := filepath.Join(dir, "inline_if.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble inline IF: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "inline_if.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(250_000)
	if got := run.readOutput(); got != "0,0\r\n3,4\r\n3,4,0\r\n5,6\r\n7,6,0\r\n6,7,0\r\n" {
		t.Fatalf("inline IF output = %q, want %q", got, "0,0\r\n3,4\r\n3,4,0\r\n5,6\r\n7,6,0\r\n6,7,0\r\n")
	}
}

func TestIE64BasicCompilerSemanticParserLowersUnaryMinus(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, .line
    la      r2, 0x050000
.copy:
    load.b  r3, (r1)
    store.b r3, (r2)
    add.q   r1, r1, #1
    add.q   r2, r2, #1
    bnez    r3, .copy
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, 0x050000
    la      r9, 0x050010
    move.q  r10, #200
    jsr     compiler_parse_semantic_line
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    bra     .done
.line:
    dc.b 0x41, 0x3D, TK_MINUS, 0x31, 0
.done:
`)
	if op, value := h.bus.Read64(0x034080), h.bus.Read64(0x0340b0); op != 3 || value != ^uint64(0) {
		t.Fatalf("unary minus IR = (op %d, value %#x), want constant -1", op, value)
	}
}

func TestIE64BasicCompilerStandaloneExecutesDimensionedArrayAccess(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037800
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line10:
    dc.q .line20
    dc.l 10, 0
    dc.b TK_DIM, 0x41, 0x28, 0x33, 0x29, 0
    align 8
.line20:
    dc.q .line30
    dc.l 20, 0
    dc.b 0x41, 0x28, 0x31, 0x29, 0x3D, 0x37, 0
    align 8
.line30:
    dc.q 0
    dc.l 30, 0
    dc.b TK_PRINT, 0x41, 0x28, 0x31, 0x29, 0
    align 8
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("array standalone emission = (%d,%d), want success", status, length)
	}
	for _, index := range []int{5, 10} {
		if flags := h.bus.Read64(uint32(0x034000+index*64) + 56); flags&2 == 0 || flags&16 == 0 {
			t.Fatalf("proven bounds record %d flags %#x, want proven and removed", index, flags)
		}
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	dir := t.TempDir()
	path := filepath.Join(dir, "array.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble array programme: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "array.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(250_000)
	if got := run.readOutput(); got != "7\r\n" {
		t.Fatalf("array programme output = %q, want %q", got, "7\r\n")
	}
}

func TestIE64BasicCompilerOptimiserRetainsUnprovenArrayBounds(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    bra     .done
.line10:
    dc.q .line20
    dc.l 10, 0
    dc.b TK_DIM, 0x41, 0x28, 0x33, 0x29, 0
    align 8
.line20:
    dc.q 0
    dc.l 20, 0
    dc.b TK_PRINT, 0x41, 0x28, 0x34, 0x29, 0
    align 8
.program_end:
.done:
`)
	// block,const,DIM,block,const,BOUNDS,LOAD,PRINT
	if flags := h.bus.Read64(0x034000 + 5*64 + 56); flags&(2|16) != 0 {
		t.Fatalf("out-of-range bounds flags %#x, must remain checked", flags)
	}
}

func TestIE64BasicCompilerStandaloneExecutesDirectPeekPoke(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037800
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line10:
    dc.q .line15
    dc.l 10, 0
    dc.b 0x41,0x3D,TK_EXT,EXT_MEMALLOC,0x28,"256,16",0x29,0
    align 8
.line15:
    dc.q .line20
    dc.l 15, 0
    dc.b 0x42,0x3D,TK_EXT,EXT_MEMALLOC,0x28,"300000,16",0x29,0
    align 8
.line20:
    dc.q .line25
    dc.l 20, 0
    dc.b TK_POKE,0x41,TK_PLUS,"32",0x2C,"42",0
    align 8
.line25:
    dc.q .line30
    dc.l 25, 0
    dc.b TK_WIDTH,0x22,"sdk/examples/assets/splash_640x92.rgba",0x22,0x2C,0x42,0
    align 8
.line30:
    dc.q .line40
    dc.l 30, 0
    dc.b TK_COPPER,TK_LIST,0x41,0
    align 8
.line40:
    dc.q .line50
    dc.l 40, 0
    dc.b TK_COPPER,TK_WAIT,"84",0
    align 8
.line50:
    dc.q .line60
    dc.l 50, 0
    dc.b TK_COPPER,"MOVE ",0x26,"H000F0050,",0x26,"H0018202C",0
    align 8
.line60:
    dc.q .line70
    dc.l 60, 0
    dc.b TK_COPPER,TK_END,0
    align 8
.line70:
    dc.q .line80
    dc.l 70, 0
    dc.b TK_BLIT,"FILL A",TK_PLUS,"64,1,1,99,4",0
    align 8
.line80:
    dc.q .line90
    dc.l 80, 0
    dc.b TK_BLIT,"WAIT",0
    align 8
.line90:
    dc.q .line100
    dc.l 90, 0
    dc.b TK_PRINT,TK_PEEK,0x28,0x41,TK_PLUS,"64",0x29,0
    align 8
.line100:
    dc.q .line110
    dc.l 100, 0
    dc.b TK_PRINT,TK_EXT,EXT_PEEK32,0x28,0x41,0x29,0
    align 8
.line110:
    dc.q 0
    dc.l 110, 0
    dc.b TK_PRINT,TK_PEEK,0x28,0x42,TK_PLUS,"3",0x29,0
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("PEEK/POKE/Copper emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	dir := t.TempDir()
	path := filepath.Join(dir, "peek_poke.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble PEEK/POKE: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "peek_poke.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	fio := NewFileIODevice(run.bus, repoRootDir(t))
	run.bus.MapIO(FILE_IO_BASE, FILE_IO_END, fio.HandleRead, fio.HandleWrite)
	run.bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fio.HandleWrite8)
	run.bus.MapIO64(FILE_DATA_PTR64, FILE_DATA_PTR64_END, fio.HandleRead64, fio.HandleWrite64)
	run.loadBytes(image)
	run.runCycles(20_000_000)
	if got := run.readOutput(); got != "0\r\n344064\r\n255\r\n" {
		t.Fatalf("memory/Copper/BLIT/BLOAD output = %q, want %q", got, "0\r\n344064\r\n255\r\n")
	}
	if colour, op := run.bus.Read32(BLT_COLOR), run.bus.Read32(BLT_OP); colour != 99 || op != 1 {
		t.Fatalf("BLIT registers = colour %d op %d, want colour 99 op 1", colour, op)
	}
	if !strings.Contains(source, "compiler_heap_cursor:") {
		t.Fatal("MEMALLOC did not emit the standalone heap cursor")
	}
}

func TestIE64BasicCompilerSemanticParserClassifiesExtendedMemoryWidths(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, .poke
    la      r9, .peek
    move.q  r10, #210
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, .peek
    la      r9, .done
    move.q  r10, #220
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, 8(r1)
    bra     .done
.poke:
    dc.b TK_EXT, EXT_POKE16, 0x31,0x30,0x30, 0x2C, 0x37, 0
.peek:
    dc.b 0x41,0x3D, TK_EXT, EXT_PEEK16, 0x28, 0x31,0x30,0x30, 0x29, 0
.done:
`)
	if a, b := h.bus.Read64(0x032000), h.bus.Read64(0x032008); a != 0 || b != 0 {
		t.Fatalf("extended memory parse statuses = (%d,%d)", a, b)
	}
	// Second parse: CONST address, PEEK16, STORE.
	if op := h.bus.Read64(0x034040); op != 37 {
		t.Fatalf("extended PEEK operation = %d, want IR_OP_PEEK16", op)
	}
}

func TestIE64BasicCompilerStandaloneExecutesOrderedWait(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .line10
    la      r9, .program_end
    la      r10, 0x034000
    jsr     compiler_parse_program
    bnez    r8, .save
    la      r8, 0x034000
    jsr     compiler_optimise_ir
    la      r8, 0x034000
    la      r9, 0x036000
    la      r10, 0x037800
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    bnez    r8, .save
    move.q  r8, #0x036000
    move.q  r10, #0x038000
    move.q  r11, #0x03F000
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
.save:
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    bra     .done
.line10:
    dc.q .line20
    dc.l 10, 0
    dc.b TK_POKE, 0x36,0x35,0x35,0x33,0x36, 0x2C, 0x31, 0
    align 8
.line20:
    dc.q .line30
    dc.l 20, 0
    dc.b TK_WAIT, 0x36,0x35,0x35,0x33,0x36, 0x2C, 0x31, 0
    align 8
.line30:
    dc.q 0
    dc.l 30, 0
    dc.b TK_PRINT, 0x31, 0
    align 8
.program_end:
.done:
`)
	status, length := h.bus.Read64(0x032000), h.bus.Read64(0x032008)
	if status != 0 || length == 0 {
		t.Fatalf("WAIT emission = (%d,%d), want success", status, length)
	}
	source := string(h.cpu.memory[0x038000 : 0x038000+length])
	dir := t.TempDir()
	path := filepath.Join(dir, "wait.asm")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), path).CombinedOutput(); err != nil {
		t.Fatalf("assemble WAIT: %v\n%s\n%s", err, out, source)
	}
	image, err := os.ReadFile(filepath.Join(dir, "wait.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(image)
	run.runCycles(20_000_000)
	if got := run.readOutput(); got != "1\r\n" {
		t.Fatalf("WAIT output = %q, want %q", got, "1\r\n")
	}
}

func TestIE64BasicCompilerSemanticParserLowersBlitFillToOrderedMMIO(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .statement
    la      r9, .done
    move.q  r10, #240
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 16(r1)
    store.q r10, 24(r1)
    la      r1, COMPILER_IR_COUNT
    load.q  r2, (r1)
    la      r1, 0x032008
    store.q r2, (r1)
    bra     .done
.statement:
    dc.b TK_BLIT, "FILL 65536,8,4,255,32", 0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("BLIT FILL parser status = %d at line %d offset %d, want success", status, h.bus.Read64(0x032010), h.bus.Read64(0x032018))
	}
	// Five expression writes plus BLT_OP and BLT_CTRL. Each ordered write is
	// represented by two constants and a POKE32 record.
	if count := h.bus.Read64(0x032008); count != 21 {
		t.Fatalf("BLIT FILL IR count = %d, want 21", count)
	}
	wantAddresses := []uint64{0xF0028, 0xF002C, 0xF0030, 0xF003C, 0xF0038, 0xF0020, 0xF001C}
	for i, want := range wantAddresses {
		record := uint32(0x034000 + i*3*64)
		if got := h.bus.Read64(record + 48); got != want {
			t.Fatalf("BLIT write %d address = %#x, want %#x", i, got, want)
		}
		if op := h.bus.Read64(record + 2*64); op != 40 {
			t.Fatalf("BLIT write %d operation = %d, want IR_OP_POKE32", i, op)
		}
	}
}

func TestIE64BasicCompilerSemanticParserLowersRemainingBlitCommands(t *testing.T) {
	cases := []string{
		`"COPY 4096,8192,64,32,256,256"`,
		`"MEMCOPY 4096,8192,1024"`,
		`"MODE7 4096,8192,640,480,0,0,1,0,0,1,255,255,1024,2560"`,
		`"WAIT"`,
	}
	for _, statement := range cases {
		t.Run(statement, func(t *testing.T) {
			h := runCompilerUnit(t, fmt.Sprintf(`
    la      r8, 0x034000
    la      r9, 0x03A000
    jsr     compiler_ir_init
    la      r8, .statement
    la      r9, .done
    move.q  r10, #250
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.statement:
    dc.b TK_BLIT, %s, 0
.done:
`, statement))
			if status := h.bus.Read64(0x032000); status != 0 {
				t.Fatalf("BLIT parser status = %d, want success", status)
			}
		})
	}
}

func TestIE64BasicCompilerSemanticParserLowersMemallocExpression(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .statement
    la      r9, .done
    move.q  r10, #260
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.statement:
    dc.b 0x41,0x3D,TK_EXT,EXT_MEMALLOC,0x28,"4096,256",0x29,0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("MEMALLOC parser status = %d, want success", status)
	}
	if op := h.bus.Read64(0x034080); op != 59 {
		t.Fatalf("MEMALLOC operation = %d, want IR_OP_MEMALLOC", op)
	}
}

func TestIE64BasicCompilerSemanticParserLowersCopperCommands(t *testing.T) {
	cases := []string{
		"TK_COPPER,TK_LIST,0x41,0",
		"TK_COPPER,TK_WAIT,\"84\",0",
		"TK_COPPER,\"MOVE &H000F0050,&H0018202C\",0",
		"TK_COPPER,TK_END,0",
		"TK_COPPER,TK_ON,0",
		"TK_COPPER,\"OFF\",0",
	}
	for _, statement := range cases {
		t.Run(statement, func(t *testing.T) {
			h := runCompilerUnit(t, fmt.Sprintf(`
    la      r8, 0x034000
    la      r9, 0x038000
    jsr     compiler_ir_init
    la      r8, .statement
    la      r9, .done
    move.q  r10, #270
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.statement:
    dc.b %s
.done:
`, statement))
			if status := h.bus.Read64(0x032000); status != 0 {
				t.Fatalf("COPPER parser status = %d, want success", status)
			}
		})
	}
}

func TestIE64BasicCompilerSemanticParserLowersBloadWithFullPath(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .statement
    la      r9, .done
    move.q  r10, #280
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.statement:
    dc.b TK_WIDTH,0x22,"sdk/examples/assets/splash_640x92.rgba",0x22,0x2C,0x41,0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("BLOAD parser status = %d, want success", status)
	}
	if op := h.bus.Read64(0x034040); op != 64 {
		t.Fatalf("BLOAD operation = %d, want IR_OP_BLOAD", op)
	}
}

func TestIE64BasicCompilerSemanticParserLowersSoundPlayAndStop(t *testing.T) {
	for _, statement := range []string{
		`TK_SOUND,"PLAY ",0x22,"sdk/examples/assets/music/enjoythesilence.mid",0x22,0`,
		`TK_SOUND,TK_STOP,0`,
	} {
		t.Run(statement, func(t *testing.T) {
			h := runCompilerUnit(t, fmt.Sprintf(`
    la      r8, 0x034000
    la      r9, 0x036000
    jsr     compiler_ir_init
    la      r8, .statement
    la      r9, .done
    move.q  r10, #290
    jsr     compiler_parse_semantic_line
    la      r1, 0x032000
    store.q r8, (r1)
    bra     .done
.statement:
    dc.b %s
.done:
`, statement))
			if status := h.bus.Read64(0x032000); status != 0 {
				t.Fatalf("SOUND parser status = %d, want success", status)
			}
		})
	}
}

func TestIE64BasicCompilerLowersShippedResonanceProgramme(t *testing.T) {
	path := filepath.Join(repoRootDir(t), "sdk", "examples", "basic", "resonance.bas")
	h := runCompilerUnitCycles(t, compilerCorpusBody(t, path), 100_000_000)
	if status, line, offset := h.bus.Read64(0x1E0000), h.bus.Read64(0x1E0008), h.bus.Read64(0x1E0010); status != 0 {
		var tokenBytes []byte
		for p := uint32(0x200000); p != 0; p = uint32(h.bus.Read64(p)) {
			if h.bus.Read32(p+8) == uint32(line) {
				for q := p + 16; h.bus.Read8(q) != 0; q++ {
					tokenBytes = append(tokenBytes, h.bus.Read8(q))
				}
				break
			}
		}
		t.Fatalf("resonance compiler diagnostic = status %d, line %d, offset %d, tokens %v", status, line, offset, tokenBytes)
	}
	if status, length := h.bus.Read64(0x1E0020), h.bus.Read64(0x1E0028); status != 0 || length == 0 {
		t.Fatalf("resonance emitter result = status %d, length %d", status, length)
	}
}

func TestIE64BasicCompilerLowersStoredLoadOnlyForArena(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x038000
    jsr     compiler_ir_init
    la      r8, .programme
    la      r9, .done
    la      r10, 0x034000
    jsr     compiler_parse_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r8, 0x034000
    la      r9, 0x039000
    la      r10, 0x03A000
    move.q  r11, #COMPILER_TARGET_ARENA
    jsr     compiler_lower_ir
    la      r1, 0x032000
    store.q r8, 8(r1)
    store.q r9, 16(r1)
    move.q  r25, r9
    la      r8, 0x039000
    move.q  r9, r25
    la      r10, 0x03B000
    la      r11, 0x03F000
    move.q  r12, #COMPILER_TARGET_ARENA
    move.q  r13, #0x100000
    jsr     compiler_emit_assembly
    la      r1, 0x032000
    store.q r8, 40(r1)
    store.q r9, 48(r1)
    la      r8, 0x034000
    la      r9, 0x039000
    la      r10, 0x03A000
    move.q  r11, #COMPILER_TARGET_STANDALONE
    jsr     compiler_lower_ir
    la      r1, 0x032000
    store.q r8, 24(r1)
    store.q r9, 32(r1)
    bra     .done
.programme:
    dc.q 0
    dc.l 10,0
    dc.b TK_LOAD,0x22,"next.bas",0x22,0
.done:
`)
	if status := h.bus.Read64(0x032000); status != 0 {
		t.Fatalf("stored LOAD parser status = %d, want success", status)
	}
	if status, count := h.bus.Read64(0x032008), h.bus.Read64(0x032010); status != 0 || count != 2 {
		t.Fatalf("arena stored LOAD lowering = (%d,%d), want success and two records; IR ops=(%d,%d)", status, count, h.bus.Read64(0x034000), h.bus.Read64(0x034040))
	}
	if status, line := h.bus.Read64(0x032018), h.bus.Read64(0x032020); status != 5 || line != 10 {
		t.Fatalf("standalone stored LOAD lowering = (%d,%d), want unsupported at line 10", status, line)
	}
	if status, length := h.bus.Read64(0x032028), h.bus.Read64(0x032030); status != 0 || length == 0 {
		t.Fatalf("arena stored LOAD emission = (%d,%d), want non-empty success", status, length)
	}
	length := h.bus.Read64(0x032030)
	source := string(h.cpu.memory[0x03B000 : 0x03B000+length])
	if !strings.Contains(source, "movt r2, #") || !strings.Contains(source, "jsr (r2)") {
		t.Fatalf("stored LOAD must materialise a full-width helper address:\n%s", source)
	}
}

func TestIE64BasicCompilerEmitterIsIndependentOfLoweredRecordAddress(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x034000
    move.q  r2, #LOWER_OP_COMPARE_EQ
    store.q r2, (r1)
    move.q  r2, #IR_TYPE_I64
    store.q r2, 8(r1)
    move.q  r2, #LOWER_FLAG_I64_OPERANDS
    or.q    r2, r2, #IR_TYPE_I64
    store.q r2, 8(r1)
    la      r2, 0x035000
    load.q  r3, (r1)
    store.q r3, (r2)
    load.q  r3, 8(r1)
    store.q r3, 8(r2)
    move.q  r8, r1
    move.q  r9, #1
    la      r10, 0x050000
    la      r11, 0x051000
    move.q  r12, #COMPILER_TARGET_ARENA
    move.q  r13, #0x100000
    jsr     compiler_emit_assembly
    la      r1, 0x032000
    store.q r8, (r1)
    store.q r9, 8(r1)
    la      r8, 0x035000
    move.q  r9, #1
    la      r10, 0x052000
    la      r11, 0x053000
    move.q  r12, #COMPILER_TARGET_ARENA
    move.q  r13, #0x100000
    jsr     compiler_emit_assembly
    la      r1, 0x032000
    store.q r8, 16(r1)
    store.q r9, 24(r1)
`)
	if a, b := h.bus.Read64(0x032000), h.bus.Read64(0x032010); a != 0 || b != 0 {
		t.Fatalf("deterministic emitter statuses=(%d,%d), want success", a, b)
	}
	la, lb := h.bus.Read64(0x032008), h.bus.Read64(0x032018)
	if la != lb {
		t.Fatalf("deterministic emitter lengths=(%d,%d)", la, lb)
	}
	a := append([]byte(nil), h.cpu.memory[0x050000:0x050000+la]...)
	b := h.cpu.memory[0x052000 : 0x052000+lb]
	if !bytes.Equal(a, b) {
		t.Fatalf("emission depends on lowered arena address:\nA=%s\nB=%s", a, b)
	}
}

func TestIE64BasicCompilerTwoForStatementsOnOneLineUseDistinctLabels(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 X=0:FOR I=1 TO 2:FOR J=1 TO 2:X=X+1:NEXT J:NEXT I")
	storeLine(t, h, "20 POKE32 500000,X")
	storeLine(t, h, "30 END")
	out := h.runCommand("RUN AOT")
	if got := h.bus.Read32(500000); got != 4 {
		t.Fatalf("two same-line FOR loops result=%d, want 4; output=%q pc=%#x", got, out, h.cpu.PC)
	}
}

func TestIE64BasicCompilerCompiledLoadEndsAfterSuccess(t *testing.T) {
	data, err := os.ReadFile("sdk/include/ehbasic_compiler_emit.inc")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	want := `jsr (r2)",10,"    beqz r8, compiler_program_end`
	if !strings.Contains(source, want) {
		t.Fatalf("compiled LOAD must terminate only after basic_load_file succeeds")
	}
}

func TestIE64BasicCompilerArenaLoadSuccessDoesNotExecuteStaleTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "replacement.bas"), []byte("10 POKE32 500004,77\n20 END\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newEhbasicAOTREPLHarnessWithFileIO(t, buildAssembler(t), dir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	for _, line := range []string{
		`10 LOAD "replacement.bas"`,
		"20 POKE32 500000,123",
		"30 END",
	} {
		storeLine(t, h, line)
	}
	out := h.runCommand("RUN AOT")
	if got := h.bus.Read32(500000); got != 0 {
		t.Fatalf("compiled stale tail executed after successful LOAD: memory=%d, want 0; output=%q", got, out)
	}
	if list := h.runCommand("LIST"); !strings.Contains(list, "POKE32 500004,77") || strings.Contains(list, "POKE32 500000,123") {
		t.Fatalf("successful compiled LOAD did not replace the resident programme: %q", list)
	}
}

func TestIE64BasicCompilerLowersEveryShippedBasicProgramme(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(repoRootDir(t), "sdk", "examples", "basic", "*.bas"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no shipped BASIC programmes found")
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			h := runCompilerUnitCycles(t, compilerCorpusBody(t, path), 100_000_000)
			if status, line, offset := h.bus.Read64(0x1E0000), h.bus.Read64(0x1E0008), h.bus.Read64(0x1E0010); status != 0 {
				t.Fatalf("compiler diagnostic = status %d, line %d, offset %d", status, line, offset)
			}
			if status, length := h.bus.Read64(0x1E0020), h.bus.Read64(0x1E0028); status != 0 || length == 0 {
				t.Fatalf("emitter result = status %d, length %d", status, length)
			}
			length := h.bus.Read64(0x1E0028)
			source := append([]byte(nil), h.cpu.memory[0xA00000:0xA00000+length]...)
			if bytes.Contains(source, []byte("FRES")) {
				t.Fatal("complex shipped programme incorrectly received scalar residency")
			}
			for _, forbidden := range []string{"expr_eval", "str_eval", "stmt_jump_table", "exec_loop", "ehbasic_aot.inc", "aot_runtime_blob"} {
				if bytes.Contains(source, []byte(forbidden)) {
					t.Fatalf("generated standalone source delegates through %q", forbidden)
				}
			}
			dir := t.TempDir()
			asmPath := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), ".bas")+".asm")
			if err := os.WriteFile(asmPath, source, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(buildAssembler(t), "-I", filepath.Join(repoRootDir(t), "sdk", "include"), asmPath)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("host assembly failed: %v\n%s", err, out)
			}
		})
	}
}

func TestIE64BasicCompilerShippedResonanceKernelPerformance(t *testing.T) {
	if os.Getenv("IE_BASIC_RUN_PERF_TESTS") != "1" {
		t.Skip("wall-clock compiler performance checks are opt-in via IE_BASIC_RUN_PERF_TESTS=1")
	}
	// This is the shipped resonance.bas sine-table initialisation loop, with an
	// outer repetition so execution dominates command and compiler setup costs.
	lines := []string{
		"10 DIM SN(255)",
		"20 FOR R=1 TO 20",
		"30 FOR I=0 TO 255",
		"40 SN(I)=I*4",
		"50 NEXT I",
		"60 NEXT R",
		"70 END",
	}
	h, _ := startREPL(t)
	for _, line := range lines {
		storeLine(t, h, line)
	}
	measure := func(command string) time.Duration {
		if out := h.runCommand("CLEAR"); strings.Contains(out, "ERROR") {
			t.Fatalf("CLEAR before %s failed: %q", command, out)
		}
		started := time.Now()
		out := h.runCommand(command)
		if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
			t.Fatalf("%s failed: %q", command, out)
		}
		return time.Since(started)
	}
	measure("RUN")
	measure("RUN AOT")
	interpreted := make([]time.Duration, 3)
	compiled := make([]time.Duration, 3)
	for i := range interpreted {
		interpreted[i] = measure("RUN")
		compiled[i] = measure("RUN AOT")
	}
	sort.Slice(interpreted, func(i, j int) bool { return interpreted[i] < interpreted[j] })
	sort.Slice(compiled, func(i, j int) bool { return compiled[i] < compiled[j] })
	if compiled[1] > interpreted[1]+interpreted[1]/20 {
		t.Fatalf("compiled shipped-example kernel regressed by more than 5 per cent: interpreter median %s, compiled median %s", interpreted[1], compiled[1])
	}
	t.Logf("shipped-example kernel medians: interpreter=%s compiled=%s", interpreted[1], compiled[1])
}

func TestIE64BasicCompilerSemanticParserMarksCallAsFullBarrier(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    la      r8, .line
    la      r9, .done
    move.q  r10, #230
    jsr     compiler_parse_semantic_line
    bra     .done
.line:
    dc.b TK_CALL, 0x34,0x30,0x39,0x36, 0
.done:
`)
	// CONST target then CALL.
	if op, effect := h.bus.Read64(0x034040), h.bus.Read64(0x034050); op != 44 || effect != 5 {
		t.Fatalf("CALL IR = (op %d, effect %d), want full-barrier CALL", op, effect)
	}
}

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
    move.q  r9, #IR_OP_PEEK32
    move.q  r10, #IR_TYPE_UNKNOWN
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

func TestIE64BasicCompilerMilestone1PropagatesProvenIntegerType(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r8, 0x034000
    la      r9, 0x035000
    jsr     compiler_ir_init
    move.q  r18, #1
.load:
    move.q  r9, #IR_OP_LOAD_SCALAR
    move.q  r10, #IR_TYPE_I64
    move.q  r11, #IR_EFFECT_BASIC
    move.q  r12, #30
    move.q  r13, r18
    move.q  r14, r18
    move.q  r15, r0
    move.q  r7, r0
    jsr     compiler_ir_append
    add.q   r18, r18, #1
    move.q  r1, #2
    ble     r18, r1, .load
    move.q  r9, #IR_OP_ADD
    move.q  r10, #IR_TYPE_UNKNOWN
    move.q  r11, #IR_EFFECT_NONE
    move.q  r12, #30
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
	if op, typ := h.bus.Read64(0x034080), h.bus.Read64(0x034088); op != 5 || typ != 1 {
		t.Fatalf("runtime integer addition = (op %d, type %d), want retained ADD with proven I64 type", op, typ)
	}
}

func TestIE64BasicCompilerExecutesDirectIntegerShifts(t *testing.T) {
	h, _ := startREPL(t)
	for _, line := range []string{
		"10 X=256 >> 8",
		"20 Y=3 << 4",
		"30 POKE32 500000,X",
		"40 POKE32 500004,Y",
		"50 END",
	} {
		storeLine(t, h, line)
	}
	out := h.runCommand("RUN AOT")
	if got := h.bus.Read32(500000); got != 1 {
		t.Fatalf("compiled right shift = %d, want 1; output=%q pc=%#x", got, out, h.cpu.PC)
	}
	if got := h.bus.Read32(500004); got != 48 {
		t.Fatalf("compiled left shift = %d, want 48; output=%q pc=%#x", got, out, h.cpu.PC)
	}
}

func TestIE64BasicCompilerRetainsDataInUnreachableStatementBlocks(t *testing.T) {
	h, _ := startREPL(t)
	for _, line := range []string{
		"10 GOTO 30",
		"20 DATA 42",
		"30 READ X",
		"40 POKE32 500000,X",
		"50 END",
	} {
		storeLine(t, h, line)
	}
	out := h.runCommand("RUN AOT")
	if got := h.bus.Read32(500000); got != 42 {
		t.Fatalf("compiled READ from control-flow-unreachable DATA = %d, want 42; output=%q pc=%#x", got, out, h.cpu.PC)
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

func TestIE64BasicCompilerHardwareStatementsPreserveBasicStatePointer(t *testing.T) {
	for _, statement := range []string{
		"SID STOP",
		"SOUND STOP",
		"COPPER ON",
		"BLIT FILL 65536,1,1,7,4",
	} {
		t.Run(statement, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, "10 "+statement)
			storeLine(t, h, "20 POKE32 500000,123")
			storeLine(t, h, "30 END")
			out := h.runCommand("RUN AOT")
			if got := h.bus.Read32(500000); got != 123 {
				t.Fatalf("statement %q corrupted compiler state: memory=%d, want 123; output=%q pc=%#x", statement, got, out, h.cpu.PC)
			}
		})
	}
}

func TestIE64BasicCompilerNestedInlineElseMatchesInterpreterBinding(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    int
		b    int
		want uint32
	}{
		{name: "inner false", a: 1, b: 0, want: 0},
		{name: "inner true", a: 1, b: 1, want: 1},
		{name: "outer false", a: 0, b: 0, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, fmt.Sprintf("10 A=%d:B=%d:X=0", tc.a, tc.b))
			storeLine(t, h, "20 IF A THEN IF B THEN X=1 ELSE X=2")
			storeLine(t, h, "30 POKE32 500000,X")
			storeLine(t, h, "40 END")
			out := h.runCommand("RUN AOT")
			if got := h.bus.Read32(500000); got != tc.want {
				t.Fatalf("nested IF result=%d, want %d; output=%q pc=%#x", got, tc.want, out, h.cpu.PC)
			}
		})
	}
}

func TestIE64BasicCompilerMemallocTransitionsPublicRanges(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 A=MEMALLOC(32,16)")
	storeLine(t, h, "20 POKE32 500000,A")
	storeLine(t, h, "30 END")
	h.bus.Write64(0x042000+0x280, 0x01000000-16)
	h.bus.Write64(0x042000+0x288, 0)
	out := h.runCommand("RUN AOT")
	if got := h.bus.Read32(500000); got != 0x00793000 {
		t.Fatalf("compiled MEMALLOC transition=%#x, want BASIC_MEMALLOC_BASE1; output=%q pc=%#x", got, out, h.cpu.PC)
	}
}

func TestIE64BasicCompilerEmitUintUnwindsDigitsOnOverflow(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    move.q  r2, #0xA5
    store.b r2, (r1)
    la      r21, 0x051000
    la      r22, 0x051002
    move.q  r9, #12345
    jsr     compiler_emit_uint
    la      r1, 0x050008
    move.q  r2, #1
    store.q r2, (r1)
`)
	if got := h.bus.Read64(0x050008); got != 1 {
		t.Fatalf("compiler_emit_uint did not return normally after overflow: marker=%d pc=%#x", got, h.cpu.PC)
	}
	if got := h.bus.Read8(0x050000); got != 0xA5 {
		t.Fatalf("compiler_emit_uint overflow wrote through address zero: canary=%#x", got)
	}
}

func TestIE64BasicCompilerEmitterRejectsMidRecordOverflowWithoutNullWrite(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    move.q  r2, #0xA5
    store.b r2, (r1)
    la      r1, 0x052000
    move.q  r2, #LOWER_OP_LABEL
    store.q r2, (r1)
    move.q  r2, #12345
    store.q r2, 24(r1)
    move.q  r8, r1
    move.q  r9, #1
    la      r10, 0x051000
    la      r11, 0x051002
    move.q  r12, #COMPILER_TARGET_ARENA
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
    la      r1, 0x050008
    store.q r8, (r1)
`)
	if got := h.bus.Read64(0x050008); got != 1 {
		t.Fatalf("mid-record overflow status=%d, want OOM", got)
	}
	if got := h.bus.Read8(0x050000); got != 0xA5 {
		t.Fatalf("mid-record overflow wrote through address zero: canary=%#x", got)
	}
}

func TestIE64BasicCompilerEmitterRejectsFooterOverflowWithoutNullWrite(t *testing.T) {
	h := runCompilerUnit(t, `
    la      r1, 0x050000
    move.q  r2, #0xA5
    store.b r2, (r1)
    la      r8, 0x052000
    move.q  r9, r0
    la      r10, 0x051000
    la      r11, 0x051100
    move.q  r12, #COMPILER_TARGET_STANDALONE
    move.q  r13, #PROG_START
    jsr     compiler_emit_assembly
    la      r1, 0x050008
    store.q r8, (r1)
`)
	if got := h.bus.Read64(0x050008); got != 1 {
		t.Fatalf("footer overflow status=%d, want OOM", got)
	}
	if got := h.bus.Read8(0x050000); got != 0xA5 {
		t.Fatalf("footer overflow wrote through address zero: canary=%#x", got)
	}
}

func TestIE64BasicCompilerSwapDoesNotInheritVariableTagAsFlags(t *testing.T) {
	h, _ := startREPL(t)
	for _, line := range []string{
		"10 A=11:B=22",
		"20 SWAP A,B",
		"30 POKE32 500000,A",
		"40 POKE32 500004,B",
		"50 END",
	} {
		storeLine(t, h, line)
	}
	out := h.runCommand("RUN AOT")
	if a, b := h.bus.Read32(500000), h.bus.Read32(500004); a != 22 || b != 11 {
		t.Fatalf("compiled SWAP result=(%d,%d), want (22,11); output=%q pc=%#x", a, b, out, h.cpu.PC)
	}
}

func TestIE64BasicCompilerSwapPreservesEachDestinationType(t *testing.T) {
	h, _ := startREPL(t)
	for _, line := range []string{
		"10 A=11:B=1.5",
		"20 SWAP A,B",
		`30 PRINT A;",";B`,
		"40 END",
	} {
		storeLine(t, h, line)
	}
	if out := aotOutput(h.runCommand("RUN AOT")); out != "1.5,11\r\n" {
		t.Fatalf("compiled mixed-type SWAP output=%q, want %q", out, "1.5,11\r\n")
	}
}
