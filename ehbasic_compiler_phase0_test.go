package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestIE64BasicCompilerPrivateAssemblerHighLinkBase(t *testing.T) {
	asmBin := buildAssembler(t)
	program := []byte("start:\n    move.q r1, #3\n    move.q r31, r1\n    load.q r31, (r6)\n.loop:\n    sub.q r1, r1, #1\n    bnez r1, .loop\n    jsr done\ndone:\n    rts\n")
	var data strings.Builder
	data.WriteString(".source:\n    dc.b ")
	for i, b := range program {
		if i != 0 {
			data.WriteString(", ")
		}
		data.WriteString(strconv.Itoa(int(b)))
	}
	data.WriteString(", 0\n    align 4\n")
	body := `    la      r8, .source
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x030000
    store.q r8, 0(r1)
    store.q r9, 8(r1)
    la      r8, .source
    move.q  r9, #1
    lsl.q   r9, r9, #32
    add.q   r9, r9, #0x1000
    la      r10, 0x032000
    jsr     aot_asm_program
    la      r1, 0x030000
    store.q r8, 16(r1)
    store.q r9, 24(r1)
    bra     .done
` + data.String() + ".done:"
	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(8_000_000)
	if low, high := h.bus.Read64(0x030000), h.bus.Read64(0x030010); low != 1 || high != 1 {
		t.Fatalf("assembler status low=%d high=%d, want both 1", low, high)
	}
	lowLen, highLen := h.bus.Read64(0x030008), h.bus.Read64(0x030018)
	if lowLen != highLen || lowLen == 0 {
		t.Fatalf("assembler lengths low=%d high=%d", lowLen, highLen)
	}
	low := append([]byte(nil), h.cpu.memory[0x031000:0x031000+lowLen]...)
	high := h.cpu.memory[0x032000 : 0x032000+highLen]
	if !bytes.Equal(low, high) {
		t.Fatalf("PC-relative output differs above 4 GiB:\nlow  % x\nhigh % x", low, high)
	}
}

var (
	phase0Equ      = regexp.MustCompile(`(?m)^(TK_[A-Z0-9_]+|EXT_[A-Z0-9_]+)\s+equ\s+0x[0-9A-Fa-f]+\s*$`)
	phase0Keyword  = regexp.MustCompile(`(?m)^\s*dc\.b\s+"([^"]+)",\s*0,\s*(TK_[A-Z0-9_]+)(?:,\s*(EXT_[A-Z0-9_]+))?`)
	phase0Stmt     = regexp.MustCompile(`(?m)^\s*dc\.l\s+([a-zA-Z0-9_]+)\s*;\s*([0-9]+)\s*=?.*\(0x([0-9A-Fa-f]{2})\)`)
	phase0Function = regexp.MustCompile(`(?m)^(fn_[a-zA-Z0-9_]+):`)
	phase0HWLabel  = regexp.MustCompile(`(?m)^(\.[a-zA-Z][a-zA-Z0-9_]*):`)
)

// TestIE64BasicCompilerPhase0Inventory is structural, not a claim of semantic
// compiler coverage. It makes every live parser surface explicit. A coverage
// cell is either "pending" or the name of a real Go test function.
func TestIE64BasicCompilerPhase0Inventory(t *testing.T) {
	live := discoverPhase0Constructs(t)
	tests := discoverGoTests(t)
	f, err := os.Open("sdk/tests/ie64_basic_compiler_phase0.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(bufio.NewReader(f)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || strings.Join(records[0], ",") != "key,parser_test,differential_test,arena,standalone" {
		t.Fatal("invalid Phase 0 inventory header")
	}
	seen := make(map[string]bool, len(records)-1)
	for i, row := range records[1:] {
		if len(row) != 5 {
			t.Fatalf("inventory row %d has %d fields", i+2, len(row))
		}
		if seen[row[0]] {
			t.Fatalf("duplicate inventory key %s", row[0])
		}
		seen[row[0]] = true
		for _, name := range row[1:3] {
			if name == "pending" {
				t.Errorf("%s has unresolved executable coverage", row[0])
				continue
			}
			if !tests[name] {
				t.Errorf("%s claims missing test %s", row[0], name)
			}
		}
		if row[3] != "compile" && row[3] != "reject" || row[4] != "compile" && row[4] != "reject" {
			t.Errorf("%s has invalid target result", row[0])
		}
		if strings.HasPrefix(row[0], "statement-slot:") && strings.HasSuffix(row[0], ":exec_do_unknown") && (row[3] != "reject" || row[4] != "reject") {
			t.Errorf("%s dispatches to syntax-error handler but is not rejected by both targets", row[0])
		}
	}
	for key := range live {
		if !seen[key] {
			t.Errorf("live parser construct is not inventoried: %s", key)
		}
	}
	for key := range seen {
		if !live[key] {
			t.Errorf("stale parser inventory entry: %s", key)
		}
	}
}

func discoverPhase0Constructs(t *testing.T) map[string]bool {
	t.Helper()
	live := make(map[string]bool)
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tokens := read("sdk/include/ehbasic_tokens.inc")
	for _, m := range phase0Equ.FindAllStringSubmatch(tokens, -1) {
		live["constant:"+m[1]] = true
	}
	for _, m := range phase0Keyword.FindAllStringSubmatch(read("sdk/include/ehbasic_tokenizer.inc"), -1) {
		live["keyword:"+m[1]+":"+m[2]] = true
		if m[3] != "" {
			live["keyword:"+m[1]+":"+m[3]] = true
		}
	}
	for _, m := range phase0Stmt.FindAllStringSubmatch(read("sdk/include/ehbasic_exec.inc"), -1) {
		live["statement-slot:0x"+strings.ToUpper(m[3])+":"+m[1]] = true
	}
	for _, path := range []string{"sdk/include/ehbasic_expr.inc", "sdk/include/ehbasic_exec.inc"} {
		for _, m := range phase0Function.FindAllStringSubmatch(read(path), -1) {
			live["expression-handler:"+m[1]] = true
		}
	}
	for _, path := range []string{"sdk/include/ehbasic_hw_audio.inc", "sdk/include/ehbasic_hw_system.inc", "sdk/include/ehbasic_hw_video.inc", "sdk/include/ehbasic_hw_voodoo.inc"} {
		base := filepath.Base(path)
		for _, m := range phase0HWLabel.FindAllStringSubmatch(read(path), -1) {
			live["hardware-parser:"+base+":"+m[1]] = true
		}
	}
	return live
}

func discoverGoTests(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool)
	for _, path := range files {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = true
			}
		}
	}
	return out
}

func TestIE64BasicCompilerLegacyTestMigrationInventory(t *testing.T) {
	allTests := discoverGoTests(t)
	legacy := make(map[string]bool)
	for _, path := range []string{"ehbasic_aot_test.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "Test") {
				legacy[fn.Name.Name] = true
			}
		}
	}
	f, err := os.Open("sdk/tests/ie64_basic_compiler_legacy_tests.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(bufio.NewReader(f)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || strings.Join(records[0], ",") != "legacy_test,classification,disposition,replacement_test" {
		t.Fatal("invalid legacy migration inventory header")
	}
	seen := make(map[string]bool)
	for i, row := range records[1:] {
		if len(row) != 4 {
			t.Fatalf("legacy inventory row %d has %d fields", i+2, len(row))
		}
		if !legacy[row[0]] || seen[row[0]] {
			t.Errorf("stale or duplicate legacy test %s", row[0])
		}
		seen[row[0]] = true
		if row[1] != "implementation-specific" && row[1] != "behavioural" && row[1] != "mixed" {
			t.Errorf("%s has invalid classification %s", row[0], row[1])
		}
		if row[2] != "retain" && row[2] != "replace" && row[2] != "delete" {
			t.Errorf("%s has invalid disposition %s", row[0], row[2])
		}
		if row[2] == "replace" && !allTests[row[3]] {
			t.Errorf("%s replacement test %s does not exist", row[0], row[3])
		}
		if row[2] == "delete" && row[1] != "implementation-specific" {
			t.Errorf("%s may only be deleted when implementation-specific", row[0])
		}
	}
	for name := range legacy {
		if !seen[name] {
			t.Errorf("legacy AOT test is unclassified: %s", name)
		}
	}
}

func sortedPhase0Keys(t *testing.T) []string {
	keys := make([]string, 0)
	for key := range discoverPhase0Constructs(t) {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
