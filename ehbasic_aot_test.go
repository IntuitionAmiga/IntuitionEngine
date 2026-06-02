package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// aotTestGuestRAM is the guest RAM size published on the test bus so
// CR_RAM_SIZE_BYTES is nonzero. 32 MiB keeps the whole arena below
// AOT_LOW32_CAP, exercising the shared-frontier path.
const aotTestGuestRAM = 0x2000000

// assembleAOTUnit assembles a self-contained snippet against ie64.inc and the
// AOT compiler-support module ehbasic_aot.inc, for unit-testing the allocators
// in isolation (no full interpreter).
func assembleAOTUnit(t *testing.T, asmBin string, body string) []byte {
	t.Helper()
	// Dummy fp_print closure symbols: aot_bundle_fp (in ehbasic_aot.inc) references
	// these labels from ie64_fp.inc, which these assembler unit tests do not pull
	// in. They are never executed here, only resolved at assembly time.
	source := fmt.Sprintf(`include "ie64.inc"

fp_neg          equ 0
fp_print        equ 0
fp_print_to_buf equ 0
stmt_jump_table equ 0
if_else_boundary equ 0
expr_eval       equ 0
exec_do_for     equ 0
exec_do_next    equ 0

    org 0x1000

test_entry:
    la      r31, STACK_TOP
    ; Default symbol-table base for direct aot_asm_program/aot_sym_* unit tests
    ; (the RUN AOT / COMPILE paths allocate this from the compiler workspace).
    la      r1, AOT_SYMTAB_PTR
    la      r2, 0x040000
    store.q r2, (r1)

%s

    halt

include "ehbasic_aot.inc"
`, body)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "aot_unit.asm")
	if err := os.WriteFile(srcPath, []byte(source), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	// Resolve includes via -I so the test does not need symlink privileges
	// (os.Symlink fails on Windows without Developer Mode / elevation).
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")
	cmd := exec.Command(asmBin, "-I", incDir, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assembly failed: %v\n%s\nsource:\n%s", err, out, source)
	}
	bin, err := os.ReadFile(filepath.Join(dir, "aot_unit.ie64"))
	if err != nil {
		t.Fatalf("read assembled binary: %v", err)
	}
	return bin
}

// TestAOT_Allocators drives the alloc64/allocLow32 bump allocators and checks
// alignment, floor, top-down ordering, exhaustion, and reset.
func TestAOT_Allocators(t *testing.T) {
	asmBin := buildAssembler(t)
	body := `    jsr     aot_alloc_reset
    mfcr    r8, cr15
    la      r1, 0x030000
    store.q r8, (r1)            ; [0] CR_RAM_SIZE_BYTES

    move.q  r8, #1
    jsr     aot_alloc64
    la      r1, 0x030008
    store.q r8, (r1)            ; [1] addr1 (size 1 -> 4 KiB)
    la      r1, 0x030010
    store.q r9, (r1)            ; [2] status1

    move.q  r8, #0x2001
    jsr     aot_alloc64
    la      r1, 0x030018
    store.q r8, (r1)            ; [3] addr2 (size 0x2001 -> 0x3000)

    move.q  r8, #0x1000
    jsr     aot_alloc_low32
    la      r1, 0x030020
    store.q r8, (r1)            ; [4] low addr

    mfcr    r8, cr15
    jsr     aot_alloc64
    la      r1, 0x030028
    store.q r9, (r1)            ; [5] exhaustion status
    la      r1, 0x030030
    store.q r8, (r1)            ; [6] exhaustion addr

    jsr     aot_alloc_reset
    move.q  r8, #1
    jsr     aot_alloc64
    la      r1, 0x030038
    store.q r8, (r1)            ; [7] addr after reset`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	// Publish a guest RAM size so CR_RAM_SIZE_BYTES (mfcr cr15) is nonzero;
	// otherwise aot_alloc_reset seeds the arena with 0 and the test no-ops.
	// 32 MiB exercises the RAM < AOT_LOW32_CAP path (where the windows share a
	// frontier).
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	h.loadBytes(bin)
	h.runCycles(1_000_000)

	cr := h.bus.Read64(0x030000)
	addr1 := h.bus.Read64(0x030008)
	status1 := h.bus.Read64(0x030010)
	addr2 := h.bus.Read64(0x030018)
	low := h.bus.Read64(0x030020)
	exStatus := h.bus.Read64(0x030028)
	exAddr := h.bus.Read64(0x030030)
	afterReset := h.bus.Read64(0x030038)

	const floor = 0x00800000
	const cap = 0xFFFF0000
	if cr < floor+0x10000 {
		t.Fatalf("CR_RAM_SIZE_BYTES too small or unpublished: CR=%#x", cr)
	}
	crAligned := cr &^ uint64(0xFFF)

	if status1 != 0 {
		t.Fatalf("alloc64 size 1 failed: status=%d", status1)
	}
	if addr1 != crAligned-0x1000 {
		t.Fatalf("addr1 = %#x, want %#x (top - 4KiB)", addr1, crAligned-0x1000)
	}
	if addr1&0xFFF != 0 {
		t.Fatalf("addr1 = %#x not 4 KiB aligned", addr1)
	}
	if addr2 != addr1-0x3000 {
		t.Fatalf("addr2 = %#x, want %#x (addr1 - 0x3000 for size 0x2001)", addr2, addr1-0x3000)
	}
	if addr2 < floor {
		t.Fatalf("addr2 = %#x below floor %#x", addr2, floor)
	}

	// allocLow32 shares the frontier with alloc64, so it allocates beneath the
	// last alloc64 block (addr2), clamping the base to the 32-bit cap if higher.
	base := addr2
	if base > cap {
		base = cap
	}
	if low != base-0x1000 {
		t.Fatalf("low alloc = %#x, want %#x (shared frontier base %#x - 4KiB)", low, base-0x1000, base)
	}
	if low > cap {
		t.Fatalf("allocLow32 returned %#x above 32-bit cap %#x", low, uint64(cap))
	}
	// Disjoint from the alloc64 region: the low block must sit at or below the
	// alloc64 frontier, never overlapping addr1/addr2.
	if low+0x1000 > addr2 {
		t.Fatalf("low block [%#x,%#x) overlaps alloc64 region (frontier %#x)", low, low+0x1000, addr2)
	}

	if exStatus != 1 || exAddr != 0 {
		t.Fatalf("exhaustion: status=%d addr=%#x, want status=1 addr=0", exStatus, exAddr)
	}
	if afterReset != addr1 {
		t.Fatalf("after reset alloc = %#x, want %#x (same as addr1)", afterReset, addr1)
	}
}

// TestAOT_AllocatorModesDisjoint pins the regression where alloc64 and
// allocLow32 could hand out the same block when guest RAM was below the cap:
// a 64-bit allocation followed by a low-32 allocation must never overlap.
func TestAOT_AllocatorModesDisjoint(t *testing.T) {
	asmBin := buildAssembler(t)
	body := `    jsr     aot_alloc_reset
    move.q  r8, #0x1000
    jsr     aot_alloc64
    la      r1, 0x031000
    store.q r8, (r1)            ; [0] A (alloc64)
    move.q  r8, #0x1000
    jsr     aot_alloc_low32
    la      r1, 0x031008
    store.q r8, (r1)            ; [1] B (allocLow32)
    la      r1, 0x031010
    store.q r9, (r1)            ; [2] B status`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	h.loadBytes(bin)
	h.runCycles(1_000_000)

	a := h.bus.Read64(0x031000)
	b := h.bus.Read64(0x031008)
	bStatus := h.bus.Read64(0x031010)

	if a == 0 || bStatus != 0 || b == 0 {
		t.Fatalf("allocation failed: A=%#x B=%#x status=%d", a, b, bStatus)
	}
	if a == b {
		t.Fatalf("alloc64 and allocLow32 returned the same block %#x", a)
	}
	// Both 0x1000 blocks; the low block must lie entirely below the 64-bit block.
	if b+0x1000 > a {
		t.Fatalf("blocks overlap: alloc64 [%#x,%#x), allocLow32 [%#x,%#x)", a, a+0x1000, b, b+0x1000)
	}
}

// TestAOT_AllocatorRejectsOverflowSize pins the regression where a size whose
// alignment add (size + 0xFFF) overflows wrapped to a tiny aligned size and
// falsely succeeded. A request of 0xFFFFFFFFFFFFFFFF must fail and leave the
// frontier untouched.
func TestAOT_AllocatorRejectsOverflowSize(t *testing.T) {
	asmBin := buildAssembler(t)
	body := `    jsr     aot_alloc_reset
    sub.q   r8, r0, #1         ; R8 = 0xFFFFFFFFFFFFFFFF
    jsr     aot_alloc64
    la      r1, 0x031000
    store.q r9, (r1)           ; [0] status (expect 1)
    la      r1, 0x031008
    store.q r8, (r1)           ; [1] addr (expect 0)
    move.q  r8, #1
    jsr     aot_alloc64
    la      r1, 0x031010
    store.q r8, (r1)           ; [2] next alloc (frontier must be intact)
    mfcr    r8, cr15
    la      r1, 0x031018
    store.q r8, (r1)           ; [3] CR`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	h.loadBytes(bin)
	h.runCycles(1_000_000)

	status := h.bus.Read64(0x031000)
	addr := h.bus.Read64(0x031008)
	next := h.bus.Read64(0x031010)
	cr := h.bus.Read64(0x031018)

	if status != 1 || addr != 0 {
		t.Fatalf("overflow size: status=%d addr=%#x, want status=1 addr=0", status, addr)
	}
	want := (cr &^ uint64(0xFFF)) - 0x1000
	if next != want {
		t.Fatalf("frontier moved by failed alloc: next=%#x, want %#x (top - 4KiB)", next, want)
	}
}

// assembleInstrs assembles raw instruction text (no wrapper) with ie64asm and
// returns the flat image bytes - the canonical encoding to test parity against.
func assembleInstrs(t *testing.T, asmBin string, instrs string) []byte {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "ref.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+instrs+"\n"), 0644); err != nil {
		t.Fatalf("write ref source: %v", err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ref.ie64"))
	if err != nil {
		t.Fatalf("read ref binary: %v", err)
	}
	return b
}

// TestAOT_EmitInstrParity checks the private instruction encoder
// (aot_emit_instr) emits byte-for-byte the same words as the Go ie64asm
// oracle, across the main operand shapes (3-reg ALU, ALU immediate, store with
// displacement, register-indirect load).
func TestAOT_EmitInstrParity(t *testing.T) {
	asmBin := buildAssembler(t)

	// Reference encoding from the real assembler.
	ref := assembleInstrs(t, asmBin, `    add.q   r3, r1, r2
    add.q   r3, r1, #5
    store.l r15, 4(r8)
    load.q  r2, (r4)`)
	if len(ref) < 32 {
		t.Fatalf("ref image too short: %d bytes", len(ref))
	}
	ref = ref[:32]

	// Same four instructions emitted field-by-field through aot_emit_instr.
	// Fields per assembler/ie64asm.go: ALU3 reg X=0 / imm X=1; load/store X=1
	// when displacement != 0; sizes B/W/L/Q = 0/1/2/3.
	body := `    la      r8, 0x031000

    move.q  r9, #0x20          ; add, rd=3 rs=1 rt=2 size=Q
    move.q  r10, #3
    move.q  r11, #3
    move.q  r12, #0
    move.q  r13, #1
    move.q  r14, #2
    move.q  r15, #0
    jsr     aot_emit_instr

    move.q  r9, #0x20          ; add.q r3, r1, #5  (imm, X=1)
    move.q  r10, #3
    move.q  r11, #3
    move.q  r12, #1
    move.q  r13, #1
    move.q  r14, #0
    move.q  r15, #5
    jsr     aot_emit_instr

    move.q  r9, #0x11          ; store.l r15, 4(r8)  (rd=15 size=L rs=8 disp=4 X=1)
    move.q  r10, #15
    move.q  r11, #2
    move.q  r12, #1
    move.q  r13, #8
    move.q  r14, #0
    move.q  r15, #4
    jsr     aot_emit_instr

    move.q  r9, #0x10          ; load.q r2, (r4)  (rd=2 size=Q rs=4 disp=0 X=0)
    move.q  r10, #2
    move.q  r11, #3
    move.q  r12, #0
    move.q  r13, #4
    move.q  r14, #0
    move.q  r15, #0
    jsr     aot_emit_instr`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(1_000_000)

	got := make([]byte, 32)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < 32; i++ {
		if got[i] != ref[i] {
			t.Fatalf("encoder parity mismatch at byte %d (instr %d): got %#02x, want %#02x\ngot:  % x\nwant: % x",
				i, i/8, got[i], ref[i], got, ref)
		}
	}
}

// TestAOT_AsmLineParity checks the full single-line assembler emits byte-for-
// byte the same machine code as the Go ie64asm oracle across all supported
// operand classes (move imm/reg, ALU3 reg/imm, ALU2, load/store with and
// without displacement) and sizes.
func TestAOT_AsmLineParity(t *testing.T) {
	asmBin := buildAssembler(t)
	lines := []string{
		"move.q r5, r6",
		"move.l r1, #1000",
		"move.b r2, #255",
		"add.q r3, r1, r2",
		"sub.l r4, r5, #7",
		"and.q r1, r2, r3",
		"or.q r10, r11, #0xFF",
		"lsl.q r1, r2, #4",
		"neg.q r3, r4",
		"not.l r1, r2",
		"sext.l r7, r8",
		"load.q r2, (r4)",
		"load.l r3, 16(r5)",
		"store.q r7, (r8)",
		"store.b r1, 4(r2)",
		"la r1, 0x1234",
		"la r9, 4096",
		"lea r3, 8(r4)",
		"lea r5, (r6)",
		"push r5",
		"pop r9",
		"mfcr r8, cr15",
		"tlbflush",
		"fmovi f3, r4",
		"fmovo r5, f6",
		"fcvtif f1, r2",
		"fcvtfi r3, f4",
		"fadd f1, f2, f3",
		"fsub f4, f5, f6",
		"fmul f7, f8, f9",
		"fdiv f10, f11, f12",
		"fabs f1, f2",
		"fneg f1, f2",
		"fsqrt f3, f4",
		"fint f5, f6",
		"fsin f1, f2",
		"fcos f3, f4",
		"ftan f5, f6",
		"fatan f7, f8",
		"flog f1, f2",
		"fexp f3, f4",
		"fpow f1, f2, f3",
		"fcmp r1, f2, f3",
	}

	// Reference: assemble all lines with the real assembler.
	ref := assembleInstrs(t, asmBin, "    "+strings.Join(lines, "\n    "))
	want := len(lines) * 8
	if len(ref) < want {
		t.Fatalf("ref image too short: %d bytes, want >= %d", len(ref), want)
	}
	ref = ref[:want]

	// Drive aot_asm_line over the same lines, accumulating into 0x031000, with
	// per-line status flags at 0x032000.
	var code, data strings.Builder
	code.WriteString("    la      r9, 0x031000\n")
	for i := range lines {
		fmt.Fprintf(&code, `    la      r8, .ln%d
    jsr     aot_asm_line
    la      r1, %#x
    store.q r8, (r1)
`, i, 0x032000+i*8)
		fmt.Fprintf(&data, ".ln%d:\n    dc.b ", i)
		for j, b := range []byte(lines[i]) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", b)
		}
		data.WriteString(", 0\n    align 4\n")
	}
	body := code.String() + "    bra .asm_done\n" + data.String() + ".asm_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	for i := range lines {
		if st := h.bus.Read64(uint32(0x032000 + i*8)); st != 1 {
			t.Errorf("aot_asm_line(%q): status=%d, want 1", lines[i], st)
		}
	}
	got := make([]byte, want)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < want; i++ {
		if got[i] != ref[i] {
			li := i / 8
			t.Fatalf("parity mismatch at byte %d (line %d %q): got %#02x, want %#02x\ngot  word: % x\nwant word: % x",
				i, li, lines[li], got[i], ref[i], got[li*8:li*8+8], ref[li*8:li*8+8])
		}
	}
}

// TestAOT_Transpile checks the first transpiler slice: empty and END/REM-only
// programmes lower to "rts" (return to the REPL trampoline); any other
// statement is reported as not-yet-lowerable. The emitted text is then run
// through the program assembler to confirm it is valid asm.
func TestAOT_Transpile(t *testing.T) {
	asmBin := buildAssembler(t)

	rtsRef := assembleInstrs(t, asmBin, "    rts")
	if len(rtsRef) < 8 {
		t.Fatalf("rts ref too short: %d", len(rtsRef))
	}
	rtsRef = rtsRef[:8]

	// Arena (RUN AOT) programmes save the entry SP at the top and restore it
	// before every rts so END/STOP unwind a compiled GOSUB. AOT_SAVED_SP =
	// 0x022858 = 141400.
	const arenaProlog = "move.l r6, #141400\nstore.q r31, (r6)\n"
	const arenaEpilog = "move.l r6, #141400\nload.q r31, (r6)\nrts\n"

	// Standalone (COMPILE) programmes boot with no resident setup, so the
	// transpiler emits a bootstrap that sets the stack, the TERM_OUT pointer
	// (R26) the print helpers use, TERM_STATUS (R27) and the state base (R16).
	// STACK_TOP=0x09F000=651264, TERM_OUT=0xF0700=984832,
	// TERM_STATUS=0xF0704=984836, BASIC_STATE=0x022000=139264.
	const standaloneProlog = "move.l r31, #651264\nmove.l r26, #984832\n" +
		"move.l r27, #984836\nmove.l r16, #139264\n"

	// Line records are the live format: a real line's next points to the
	// terminator record (next == 0). Empty == just a terminator.
	body := `    ; Pre-fill the output buffer with stale non-null bytes: without a
    ; terminator the assembler would parse these as extra lines.
    la      r3, 0x031000
    move.q  r4, #64
    move.q  r5, #0x41
.fill_stale:
    store.b r5, (r3)
    add.q   r3, r3, #1
    sub.q   r4, r4, #1
    bnez    r4, .fill_stale

    ; empty programme: a lone terminator record (next == 0)
    la      r1, 0x030080
    store.l r0, (r1)
    la      r8, 0x030080
    la      r9, 0x031000
    move.q  r10, #0            ; arena mode -> rts
    jsr     aot_transpile
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)

    ; END programme: line [next=term, lineNo=10, END,0] + terminator
    la      r1, 0x030180
    store.l r0, (r1)
    la      r1, 0x030100
    la      r2, 0x030180
    store.l r2, (r1)
    move.q  r2, #10
    store.l r2, 4(r1)
    move.q  r2, #0x80
    store.b r2, 8(r1)
    store.b r0, 9(r1)
    la      r8, 0x030100
    la      r9, 0x031100
    move.q  r10, #0
    jsr     aot_transpile
    la      r1, 0x032010
    store.q r8, (r1)
    la      r1, 0x032018
    store.q r9, (r1)

    ; standalone mode on the same programme -> halt
    la      r8, 0x030100
    la      r9, 0x031300
    move.q  r10, #1
    jsr     aot_transpile
    la      r1, 0x032038
    store.q r8, (r1)
    la      r1, 0x032040
    store.q r9, (r1)

    ; unsupported (CONT token 0x9F - REPL command, not lowerable): line + terminator
    la      r1, 0x030280
    store.l r0, (r1)
    la      r1, 0x030200
    la      r2, 0x030280
    store.l r2, (r1)
    move.q  r2, #20
    store.l r2, 4(r1)
    move.q  r2, #0x9F
    store.b r2, 8(r1)
    store.b r0, 9(r1)
    la      r8, 0x030200
    la      r9, 0x031200
    move.q  r10, #0
    jsr     aot_transpile
    la      r1, 0x032020
    store.q r8, (r1)

    ; assemble the empty-programme output to confirm it is valid asm
    la      r8, 0x031000
    move.q  r9, #0x1000
    la      r10, 0x031400
    jsr     aot_asm_program
    la      r1, 0x032028
    store.q r8, (r1)
    la      r1, 0x032030
    store.q r9, (r1)`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	read := func(addr uint32, n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = h.cpu.memory[int(addr)+i]
		}
		return b
	}

	emptyWant := arenaProlog + arenaEpilog
	if st := h.bus.Read64(0x032000); st != 1 {
		t.Errorf("empty transpile status=%d, want 1", st)
	}
	if l := h.bus.Read64(0x032008); l != uint64(len(emptyWant)) {
		t.Errorf("empty transpile len=%d, want %d", l, len(emptyWant))
	}
	if got := string(read(0x031000, len(emptyWant))); got != emptyWant {
		t.Errorf("empty transpile output=%q, want %q", got, emptyWant)
	}
	// Every line is labelled "L<n>:"; END emits its own terminator and the
	// transpiler appends a trailing terminator as a fall-off-the-end safety net.
	endWant := arenaProlog + "L10:\n" + arenaEpilog + arenaEpilog
	if st := h.bus.Read64(0x032010); st != 1 {
		t.Errorf("END transpile status=%d, want 1", st)
	}
	if got := string(read(0x031100, len(endWant))); got != endWant {
		t.Errorf("END transpile output=%q, want %q", got, endWant)
	}
	if st := h.bus.Read64(0x032038); st != 1 {
		t.Errorf("standalone transpile status=%d, want 1", st)
	}
	standaloneWant := standaloneProlog + "L10:\nhalt\nhalt\n"
	if l := h.bus.Read64(0x032040); l != uint64(len(standaloneWant)) {
		t.Errorf("standalone transpile len=%d, want %d", l, len(standaloneWant))
	}
	if got := string(read(0x031300, len(standaloneWant))); got != standaloneWant {
		t.Errorf("standalone transpile output=%q, want %q", got, standaloneWant)
	}
	if st := h.bus.Read64(0x032020); st != 0 {
		t.Errorf("PRINT transpile status=%d, want 0 (unsupported)", st)
	}
	// Empty arena programme assembles to prologue (2) + epilogue (3) = 5 words;
	// the final word is the rts.
	if st := h.bus.Read64(0x032028); st != 1 {
		t.Errorf("assemble status=%d, want 1", st)
	}
	if cl := h.bus.Read64(0x032030); cl != 40 {
		t.Errorf("assembled codeLen=%d, want 40", cl)
	}
	if got := read(0x031400+32, 8); !bytesEqual(got, rtsRef) {
		t.Errorf("assembled rts = % x, want % x", got, rtsRef)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAOT_TranspilePoke8 checks POKE8 with integer-literal operands lowers to
// the expected load-immediate + store.b sequence, and that the text assembles.
func TestAOT_TranspilePoke8(t *testing.T) {
	asmBin := buildAssembler(t)

	// Tokens for "POKE8 100, 66": TK_POKE '8' ' ' '1''0''0' ',' ' ' '6''6'
	body := `    ; terminator
    la      r1, 0x030380
    store.l r0, (r1)
    ; line record at 0x030300
    la      r1, 0x030300
    la      r2, 0x030380
    store.l r2, (r1)
    move.q  r2, #10
    store.l r2, 4(r1)
    ; tokens at +8: 0x98 '8' ' ' "100" ',' ' ' "66" 0
    la      r3, 0x030308
    move.q  r2, #0x98
    store.b r2, (r3)
    move.q  r2, #0x38
    store.b r2, 1(r3)
    move.q  r2, #0x20
    store.b r2, 2(r3)
    move.q  r2, #0x31
    store.b r2, 3(r3)
    move.q  r2, #0x30
    store.b r2, 4(r3)
    move.q  r2, #0x30
    store.b r2, 5(r3)
    move.q  r2, #0x2C
    store.b r2, 6(r3)
    move.q  r2, #0x20
    store.b r2, 7(r3)
    move.q  r2, #0x36
    store.b r2, 8(r3)
    move.q  r2, #0x36
    store.b r2, 9(r3)
    store.b r0, 10(r3)

    la      r8, 0x030300
    la      r9, 0x031000
    move.q  r10, #0           ; arena mode
    jsr     aot_transpile
    la      r1, 0x032000
    store.q r8, (r1)          ; status
    la      r1, 0x032008
    store.q r9, (r1)          ; len

    ; assemble the emitted text
    la      r8, 0x031000
    move.q  r9, #0x1000
    la      r10, 0x031400
    jsr     aot_asm_program
    la      r1, 0x032010
    store.q r8, (r1)          ; asm status
    la      r1, 0x032018
    store.q r9, (r1)          ; asm codeLen`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("transpile status=%d, want 1", st)
	}
	textLen := int(h.bus.Read64(0x032008))
	got := make([]byte, textLen)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	// Arena prologue (save SP) + label + POKE8 lowering + arena epilogue
	// (restore SP, rts). AOT_SAVED_SP = 141400.
	const arenaProlog = "move.l r6, #141400\nstore.q r31, (r6)\n"
	const arenaEpilog = "move.l r6, #141400\nload.q r31, (r6)\nrts\n"
	want := arenaProlog + "L10:\nmove.l r1, #100\nmove.l r2, #66\nstore.b r2, (r1)\n" + arenaEpilog
	if string(got) != want {
		t.Fatalf("transpiled asm = %q, want %q", got, want)
	}
	if st := h.bus.Read64(0x032010); st != 1 {
		t.Fatalf("assemble status=%d, want 1", st)
	}
	// prologue(2) + move.l, move.l, store.b + epilogue(3) = 8 words.
	if cl := h.bus.Read64(0x032018); cl != 64 {
		t.Fatalf("assembled codeLen=%d, want 64", cl)
	}
}

// TestAOT_OutputOverflow drives the transpiler and assembler with deliberately
// tiny output windows and confirms they report status 2 (buffer overflow)
// instead of running off the end of the fixed AOT arenas. AOT_TEXT_END /
// AOT_CODE_END == 0 means "unbounded" (the parity unit tests rely on that), so
// the final unbounded assemble confirms the normal path still succeeds.
func TestAOT_OutputOverflow(t *testing.T) {
	asmBin := buildAssembler(t)

	// One "POKE 100, 66" line: TK_POKE ' ' "100" ',' ' ' "66".
	body := `    ; terminator
    la      r1, 0x030380
    store.l r0, (r1)
    la      r1, 0x030300
    la      r2, 0x030380
    store.l r2, (r1)
    move.q  r2, #10
    store.l r2, 4(r1)
    la      r3, 0x030308
    move.q  r2, #0x98             ; TK_POKE
    store.b r2, (r3)
    move.q  r2, #0x20             ; ' ' (not '8' -> 32-bit poke)
    store.b r2, 1(r3)
    move.q  r2, #0x31
    store.b r2, 2(r3)
    move.q  r2, #0x30
    store.b r2, 3(r3)
    move.q  r2, #0x30
    store.b r2, 4(r3)
    move.q  r2, #0x2C
    store.b r2, 5(r3)
    move.q  r2, #0x20
    store.b r2, 6(r3)
    move.q  r2, #0x36
    store.b r2, 7(r3)
    move.q  r2, #0x36
    store.b r2, 8(r3)
    store.b r0, 9(r3)

    ; (1) transpile into a 16-byte window -> no headroom -> overflow (status 2)
    la      r8, 0x030300
    la      r9, 0x031000
    move.q  r10, #0
    add.q   r11, r9, #16
    jsr     aot_transpile
    la      r1, 0x032000
    store.q r8, (r1)

    ; (2) transpile into a generous window -> success, text at 0x031100
    la      r8, 0x030300
    la      r9, 0x031100
    move.q  r10, #0
    add.q   r11, r9, #0x1000
    jsr     aot_transpile
    la      r1, 0x032008
    store.q r8, (r1)

    ; (3) assemble with AOT_CODE_END one instruction past the buffer start ->
    ; the POKE's 3 instructions overrun -> overflow (status 2)
    la      r1, AOT_CODE_END
    la      r2, 0x031408
    store.q r2, (r1)
    la      r8, 0x031100
    move.q  r9, #0x1000
    la      r10, 0x031400
    jsr     aot_asm_program
    la      r1, 0x032010
    store.q r8, (r1)

    ; (4) assemble unbounded (AOT_CODE_END = 0) -> success (status 1)
    la      r1, AOT_CODE_END
    store.q r0, (r1)
    la      r8, 0x031100
    move.q  r9, #0x1000
    la      r10, 0x031600
    jsr     aot_asm_program
    la      r1, 0x032018
    store.q r8, (r1)`

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	if st := h.bus.Read64(0x032000); st != 2 {
		t.Errorf("transpile text-overflow status=%d, want 2", st)
	}
	if st := h.bus.Read64(0x032008); st != 1 {
		t.Errorf("transpile success status=%d, want 1", st)
	}
	if st := h.bus.Read64(0x032010); st != 2 {
		t.Errorf("assemble code-overflow status=%d, want 2", st)
	}
	if st := h.bus.Read64(0x032018); st != 1 {
		t.Errorf("assemble unbounded status=%d, want 1", st)
	}
}

// TestREPL_RunAOT_Poke8 compiles and runs POKE8 natively, then verifies the
// byte was written to guest memory.
func TestREPL_RunAOT_Poke8(t *testing.T) {
	h, _ := startREPL(t)
	const addr = 0x50000 // 327680, plain guest RAM (variable area, unused here)
	storeLine(t, h, "10 POKE8 327680, 66")
	output := h.runCommand("RUN AOT")
	if strings.Contains(output, "ERROR") || strings.Contains(output, aotStubMarker) {
		t.Fatalf("RUN AOT POKE8 should compile and run, got: %q", output)
	}
	if got := h.cpu.memory[addr]; got != 66 {
		t.Fatalf("POKE8 native write: memory[%#x] = %d, want 66", addr, got)
	}
}

// runAOTProg stores a one-line programme, RUN AOT-compiles and runs it, and
// fails if the compile reported an error or fell back to the stub.
func runAOTProg(t *testing.T, h *ehbasicTestHarness, line string) {
	t.Helper()
	storeLine(t, h, line)
	out := h.runCommand("RUN AOT")
	if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
		t.Fatalf("RUN AOT on %q failed: %q", line, out)
	}
}

// TestREPL_RunAOT_MemoryStatements compiles and runs the integer-literal memory
// statements natively and verifies their guest-memory effects.
func TestREPL_RunAOT_MemoryStatements(t *testing.T) {
	const addr = 0x50000 // 327680

	t.Run("POKE_32bit", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTProg(t, h, "10 POKE 327680, 305419896") // 0x12345678
		if got := h.bus.Read32(addr); got != 0x12345678 {
			t.Fatalf("POKE: mem32=%#x, want 0x12345678", got)
		}
	})

	t.Run("LOKE_32bit", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTProg(t, h, "10 LOKE 327680, 287454020") // 0x11223344
		if got := h.bus.Read32(addr); got != 0x11223344 {
			t.Fatalf("LOKE: mem32=%#x, want 0x11223344", got)
		}
	})

	t.Run("DOKE_16bit", func(t *testing.T) {
		h, _ := startREPL(t)
		// pre-clear with POKE so the upper bytes are known, then DOKE the low word.
		runAOTProg(t, h, "10 POKE 327680, 0 : DOKE 327680, 4660") // 0x1234
		if got := h.bus.Read32(addr); got != 0x1234 {
			t.Fatalf("DOKE: mem32=%#x, want 0x1234", got)
		}
	})

	t.Run("BITSET", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTProg(t, h, "10 POKE8 327680, 0 : BITSET 327680, 3")
		if got := h.cpu.memory[addr]; got != 0x08 {
			t.Fatalf("BITSET: mem8=%#x, want 0x08", got)
		}
	})

	t.Run("BITCLR", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTProg(t, h, "10 POKE8 327680, 255 : BITCLR 327680, 3")
		if got := h.cpu.memory[addr]; got != 0xF7 {
			t.Fatalf("BITCLR: mem8=%#x, want 0xF7", got)
		}
	})

	t.Run("CALL", func(t *testing.T) {
		h, _ := startREPL(t)
		// POKE an rts (opcode 0x51) at 0x60000, then CALL it. jsr -> rts must
		// return cleanly; the REPL must survive afterwards.
		runAOTProg(t, h, "10 POKE 393216, 81 : CALL 393216")
		if got := h.cpu.memory[0x60000]; got != 0x51 {
			t.Fatalf("CALL setup: mem[0x60000]=%#x, want 0x51 (rts)", got)
		}
		if out := h.runCommand("LIST"); !strings.Contains(out, "10") {
			t.Fatalf("REPL broken after CALL; LIST: %q", out)
		}
	})
}

// runAOTLines stores a multi-line programme, RUN AOT-compiles and runs it, and
// fails if the compile reported an error or fell back to the stub.
func runAOTLines(t *testing.T, h *ehbasicTestHarness, lines ...string) {
	t.Helper()
	for _, l := range lines {
		storeLine(t, h, l)
	}
	out := h.runCommand("RUN AOT")
	if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
		t.Fatalf("RUN AOT failed: %q", out)
	}
}

// TestREPL_RunAOT_ControlFlow compiles and runs GOTO/GOSUB/RETURN/END natively
// and verifies the control flow through guest-memory side effects. Addresses
// 0x50000..0x50002 are plain RAM.
func TestREPL_RunAOT_ControlFlow(t *testing.T) {
	const a, b, c = 0x50000, 0x50001, 0x50002 // 327680, 327681, 327682

	t.Run("END_halts", func(t *testing.T) {
		h, _ := startREPL(t)
		// If END fell through, line 30 would overwrite a with 2.
		runAOTLines(t, h,
			"10 POKE8 327680, 1",
			"20 END",
			"30 POKE8 327680, 2")
		if got := h.cpu.memory[a]; got != 1 {
			t.Fatalf("END did not halt: memory[%#x]=%d, want 1", a, got)
		}
	})

	t.Run("GOTO_skips", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTLines(t, h,
			"10 POKE8 327680, 1",
			"20 GOTO 40",
			"30 POKE8 327680, 2", // skipped
			"40 POKE8 327681, 3",
			"50 END")
		if got := h.cpu.memory[a]; got != 1 {
			t.Fatalf("GOTO did not skip line 30: memory[%#x]=%d, want 1", a, got)
		}
		if got := h.cpu.memory[b]; got != 3 {
			t.Fatalf("GOTO target not reached: memory[%#x]=%d, want 3", b, got)
		}
	})

	t.Run("GOSUB_RETURN", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTLines(t, h,
			"10 POKE8 327680, 1",
			"20 GOSUB 100",
			"30 POKE8 327681, 3",
			"40 END",
			"100 POKE8 327682, 2",
			"110 RETURN")
		if got := h.cpu.memory[a]; got != 1 {
			t.Fatalf("line 10 not run: memory[%#x]=%d, want 1", a, got)
		}
		if got := h.cpu.memory[c]; got != 2 {
			t.Fatalf("GOSUB target not run: memory[%#x]=%d, want 2", c, got)
		}
		if got := h.cpu.memory[b]; got != 3 {
			t.Fatalf("RETURN did not resume at line 30: memory[%#x]=%d, want 3", b, got)
		}
		// REPL must survive: GOSUB/RETURN use the hardware stack and END returns
		// to the REPL trampoline.
		if out := h.runCommand("LIST"); !strings.Contains(out, "10") {
			t.Fatalf("REPL broken after control-flow RUN AOT; LIST: %q", out)
		}
	})

	t.Run("END_in_GOSUB_terminates", func(t *testing.T) {
		h, _ := startREPL(t)
		// END is reached inside the GOSUB. It must terminate the whole programme,
		// not pop the GOSUB return and resume at line 20. A bare rts terminator
		// would run line 20 and set b.
		runAOTLines(t, h,
			"10 POKE8 327680, 1 : GOSUB 100",
			"20 POKE8 327681, 2",
			"100 END")
		if got := h.cpu.memory[a]; got != 1 {
			t.Fatalf("line 10 not run: memory[%#x]=%d, want 1", a, got)
		}
		if got := h.cpu.memory[b]; got != 0 {
			t.Fatalf("END inside GOSUB resumed after the call: memory[%#x]=%d, want 0", b, got)
		}
		if out := h.runCommand("LIST"); !strings.Contains(out, "10") {
			t.Fatalf("REPL broken after END-in-GOSUB; LIST: %q", out)
		}
	})
}

// TestREPL_RunAOT_StopCont exercises the native STOP/CONT continuation. STOP in a
// RUN AOT programme saves a native resume address (AOT_CONT_PC) and unwinds to the
// REPL; a typed CONT re-enters the compiled arena where it stopped, with variables,
// DATA, FOR and GOSUB state preserved. Side effects are observed through guest RAM
// at 0x50000.. (plain RAM, same region as TestREPL_RunAOT_ControlFlow).
func TestREPL_RunAOT_StopCont(t *testing.T) {
	const a, b, c, d = 0x50000, 0x50001, 0x50002, 0x50003

	runAOT := func(t *testing.T, h *ehbasicTestHarness) string {
		t.Helper()
		out := h.runCommand("RUN AOT")
		if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
			t.Fatalf("RUN AOT failed: %q", out)
		}
		return out
	}

	t.Run("top_level_resume", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, "10 POKE8 327680, 1")
		storeLine(t, h, "20 STOP")
		storeLine(t, h, "30 POKE8 327681, 2")
		storeLine(t, h, "40 END")
		runAOT(t, h)
		if h.cpu.memory[a] != 1 {
			t.Fatalf("line 10 not run before STOP: memory[%#x]=%d, want 1", a, h.cpu.memory[a])
		}
		if h.cpu.memory[b] != 0 {
			t.Fatalf("STOP did not halt before line 30: memory[%#x]=%d, want 0", b, h.cpu.memory[b])
		}
		h.runCommand("CONT")
		if h.cpu.memory[b] != 2 {
			t.Fatalf("CONT did not resume at line 30: memory[%#x]=%d, want 2", b, h.cpu.memory[b])
		}
	})

	t.Run("statements_after_stop_same_line", func(t *testing.T) {
		h, _ := startREPL(t)
		// CONT must resume at the statement after STOP, even mid-line.
		storeLine(t, h, "10 POKE8 327680, 1 : STOP : POKE8 327681, 2")
		storeLine(t, h, "20 END")
		runAOT(t, h)
		if h.cpu.memory[a] != 1 || h.cpu.memory[b] != 0 {
			t.Fatalf("mid-line STOP: memory[a]=%d memory[b]=%d, want 1,0", h.cpu.memory[a], h.cpu.memory[b])
		}
		h.runCommand("CONT")
		if h.cpu.memory[b] != 2 {
			t.Fatalf("CONT did not run the post-STOP statement: memory[%#x]=%d, want 2", b, h.cpu.memory[b])
		}
	})

	t.Run("for_loop_state_survives", func(t *testing.T) {
		h, _ := startREPL(t)
		// STOP inside a FOR body. Each CONT continues the loop with the counter
		// intact (the FOR frame lives in the resident control stack, not the
		// hardware stack), so the printed I advances across the resumes.
		storeLine(t, h, "10 FOR I=1 TO 3")
		storeLine(t, h, "20 PRINT I")
		storeLine(t, h, "30 STOP")
		storeLine(t, h, "40 NEXT I")
		storeLine(t, h, `50 PRINT "DONE"`)
		storeLine(t, h, "60 END")
		if out := runAOT(t, h); !strings.Contains(out, "1") {
			t.Fatalf("first iteration did not print 1: %q", out)
		}
		if out := h.runCommand("CONT"); !strings.Contains(out, "2") {
			t.Fatalf("second iteration after CONT did not print 2: %q", out)
		}
		if out := h.runCommand("CONT"); !strings.Contains(out, "3") {
			t.Fatalf("third iteration after CONT did not print 3: %q", out)
		}
		if out := h.runCommand("CONT"); !strings.Contains(out, "DONE") {
			t.Fatalf("loop did not finish after final CONT: %q", out)
		}
	})

	t.Run("stop_inside_gosub_unwinds_frame", func(t *testing.T) {
		h, _ := startREPL(t)
		// Documented limitation: arena GOSUB uses the hardware stack (jsr/rts), so a
		// STOP inside a subroutine unwinds the return frame to the REPL. CONT resumes
		// the post-STOP statements, but the following RETURN has no frame and reports
		// ?RETURN WITHOUT GOSUB. Top-level STOP/CONT is the supported case; the
		// software-stack approach that preserved the frame was removed because it
		// relied on `la` to capture return addresses, which truncates a >4 GiB arena.
		storeLine(t, h, "10 POKE8 327680, 1")
		storeLine(t, h, "20 GOSUB 100")
		storeLine(t, h, "30 POKE8 327681, 2") // only runs if the RETURN succeeds
		storeLine(t, h, "40 END")
		storeLine(t, h, "100 POKE8 327682, 3")
		storeLine(t, h, "110 STOP")
		storeLine(t, h, "120 POKE8 327683, 4")
		storeLine(t, h, "130 RETURN")
		runAOT(t, h)
		if h.cpu.memory[a] != 1 || h.cpu.memory[c] != 3 {
			t.Fatalf("pre-STOP: memory[a]=%d memory[c]=%d, want 1,3", h.cpu.memory[a], h.cpu.memory[c])
		}
		out := h.runCommand("CONT")
		// CONT resumes the statement after STOP (d := 4)...
		if h.cpu.memory[d] != 4 {
			t.Fatalf("CONT did not resume after STOP: memory[%#x]=%d, want 4", d, h.cpu.memory[d])
		}
		// ...but the GOSUB frame was unwound, so RETURN errors and line 30 never runs.
		if !strings.Contains(out, "RETURN WITHOUT GOSUB") {
			t.Fatalf("expected RETURN WITHOUT GOSUB after CONT from inside a GOSUB, got: %q", out)
		}
		if h.cpu.memory[b] != 0 {
			t.Fatalf("RETURN should have failed (frame unwound): memory[%#x]=%d, want 0", b, h.cpu.memory[b])
		}
	})

	t.Run("multiple_stops", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, "10 POKE8 327680, 1")
		storeLine(t, h, "20 STOP")
		storeLine(t, h, "30 POKE8 327681, 2")
		storeLine(t, h, "40 STOP")
		storeLine(t, h, "50 POKE8 327682, 3")
		storeLine(t, h, "60 END")
		runAOT(t, h)
		if h.cpu.memory[a] != 1 || h.cpu.memory[b] != 0 {
			t.Fatalf("first STOP: memory[a]=%d memory[b]=%d, want 1,0", h.cpu.memory[a], h.cpu.memory[b])
		}
		h.runCommand("CONT")
		if h.cpu.memory[b] != 2 || h.cpu.memory[c] != 0 {
			t.Fatalf("second STOP: memory[b]=%d memory[c]=%d, want 2,0", h.cpu.memory[b], h.cpu.memory[c])
		}
		h.runCommand("CONT")
		if h.cpu.memory[c] != 3 {
			t.Fatalf("final CONT: memory[%#x]=%d, want 3", c, h.cpu.memory[c])
		}
	})

	t.Run("edit_invalidates_continuation", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, "10 POKE8 327680, 1")
		storeLine(t, h, "20 STOP")
		storeLine(t, h, "30 POKE8 327681, 2")
		runAOT(t, h)
		if h.cpu.memory[b] != 0 {
			t.Fatalf("STOP did not halt: memory[%#x]=%d, want 0", b, h.cpu.memory[b])
		}
		// Editing the programme clears the pending native continuation, so a CONT
		// must NOT re-enter the stale arena (line 30 stays unexecuted).
		storeLine(t, h, "25 REM EDIT")
		h.runCommand("CONT")
		if h.cpu.memory[b] != 0 {
			t.Fatalf("CONT resumed a stale arena after an edit: memory[%#x]=%d, want 0", b, h.cpu.memory[b])
		}
		if list := h.runCommand("LIST"); !strings.Contains(list, "25") {
			t.Fatalf("REPL broken after edit+CONT; LIST: %q", list)
		}
	})

	t.Run("cont_without_stop_is_harmless", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, `10 PRINT "HI"`)
		// No RUN AOT yet, so no native continuation. CONT must fall through to the
		// interpreted no-op and leave the REPL usable.
		h.runCommand("CONT")
		if list := h.runCommand("LIST"); !strings.Contains(list, "10") {
			t.Fatalf("REPL broken after a stray CONT; LIST: %q", list)
		}
	})

	t.Run("nested_gosub_no_stop", func(t *testing.T) {
		h, _ := startREPL(t)
		// Two-deep GOSUB/RETURN on the hardware stack (no STOP): basic nesting check.
		storeLine(t, h, "10 GOSUB 100")
		storeLine(t, h, "20 POKE8 327680, 9")
		storeLine(t, h, "30 END")
		storeLine(t, h, "100 GOSUB 200")
		storeLine(t, h, "110 POKE8 327681, 8")
		storeLine(t, h, "120 RETURN")
		storeLine(t, h, "200 POKE8 327682, 7")
		storeLine(t, h, "210 RETURN")
		runAOT(t, h)
		if h.cpu.memory[c] != 7 || h.cpu.memory[b] != 8 || h.cpu.memory[a] != 9 {
			t.Fatalf("nested GOSUB/RETURN: memory[a]=%d memory[b]=%d memory[c]=%d, want 9,8,7",
				h.cpu.memory[a], h.cpu.memory[b], h.cpu.memory[c])
		}
	})
}

// TestREPL_RunAOT_HighArena is the regression for the high-address arena bug: when
// the AOT arena loads at a high address, every code address it materialises must be
// PC-relative (hardware jsr/rts for GOSUB, jsr+pop to capture the STOP resume PC).
// The earlier software-GOSUB/STOP path used "la rN, label" - a 32-bit, sign-extending
// lea - which produced a wrong address once the arena base had bit 31 set (on a
// large-RAM host the arena sits tens of GiB up) and hung on the first RETURN.
//
// The headless bus caps at busMemMaxBytes (0xFFFF0000), below the 4 GiB sign-extension
// alias zone, so this forces the arena near that ceiling: its base has bit 31 set,
// which is exactly where the old sign-extending "la" went wrong. The bus is mmap-
// backed and sparse, so a ~4 GiB span only commits the pages actually touched (the
// low BASIC regions plus the arena), keeping the test cheap.
func TestREPL_RunAOT_HighArena(t *testing.T) {
	binary := assembleREPL(t)
	bus, err := NewMachineBusSized(0xFFFF0000)
	if err != nil {
		t.Fatalf("NewMachineBusSized(0xFFFF0000): %v", err)
	}
	h := newEhbasicHarnessOnBus(t, bus)
	// No TotalGuestRAM set, so the ceiling is honoured verbatim: CR_RAM_SIZE_BYTES
	// reads ~4 GiB and the AOT allocator places the arena top-down from there.
	h.bus.ApplyProfileVisibleCeiling(0xFFFF0000)
	h.loadBytes(binary)
	if out := h.runUntilPrompt(); !strings.Contains(out, "Ready") {
		t.Fatalf("no Ready prompt at high arena; got: %q", out)
	}

	const a, b, c, d = 0x50000, 0x50001, 0x50002, 0x50003
	storeLine(t, h, "10 POKE8 327680, 1")
	storeLine(t, h, "20 GOSUB 100")
	storeLine(t, h, "30 POKE8 327681, 2") // runs only if RETURN works at a high arena
	storeLine(t, h, "40 STOP")
	storeLine(t, h, "50 POKE8 327682, 3") // runs only after CONT
	storeLine(t, h, "60 END")
	storeLine(t, h, "100 POKE8 327683, 9 : RETURN")
	out := h.runCommand("RUN AOT")
	if strings.Contains(out, "ERROR") || strings.Contains(out, aotStubMarker) {
		t.Fatalf("RUN AOT failed at high arena: %q", out)
	}

	// Confirm the arena really loaded high (bit 31 set) - otherwise the test would
	// pass without exercising the path that broke.
	arenaBase := h.bus.Read64(0x022828) // AOT_DC_CODE
	if arenaBase < 0x80000000 {
		t.Fatalf("arena base %#x is not a high address; high-arena path not exercised", arenaBase)
	}
	// GOSUB ran its body (d=9) and RETURN came back so line 30 ran (b=2); STOP then
	// halted before line 50 (c=0). The old sign-extending `la` would have hung here.
	if h.cpu.memory[a] != 1 || h.cpu.memory[d] != 9 {
		t.Fatalf("GOSUB body did not run at high arena: a=%d d=%d, want 1,9", h.cpu.memory[a], h.cpu.memory[d])
	}
	if h.cpu.memory[b] != 2 {
		t.Fatalf("RETURN failed at high arena: b=%d, want 2", h.cpu.memory[b])
	}
	if h.cpu.memory[c] != 0 {
		t.Fatalf("STOP did not halt: c=%d, want 0", h.cpu.memory[c])
	}
	// The STOP capture stored the full high resume PC (bit 31 set) - proof it used the
	// jsr+pop hardware capture, not a truncating `la`.
	contPC := h.bus.Read64(0x0228C0) // AOT_CONT_PC
	if contPC < 0x80000000 {
		t.Fatalf("AOT_CONT_PC=%#x is not a high address; STOP capture truncated it", contPC)
	}
	h.runCommand("CONT")
	if h.cpu.memory[c] != 3 {
		t.Fatalf("CONT did not resume at high arena: c=%d, want 3", h.cpu.memory[c])
	}
}

// TestREPL_RunAOT_Bload proves BLOAD under RUN AOT: the arena delegates BLOAD
// (token TK_WIDTH=0xA3) to the resident exec_do_bload via stmt_jump_table, which
// loads raw bytes to the destination through the File I/O MMIO. The compiled arena
// runs in the REPL machine, which already has File I/O mapped to a scratch dir.
func TestREPL_RunAOT_Bload(t *testing.T) {
	asmBin := buildAssembler(t)
	dir := t.TempDir()
	payload := []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	const dst = 0x710000
	storeLine(t, h, `10 BLOAD "blob.bin", &H710000`)
	storeLine(t, h, `20 PRINT "DONE"`)
	out := h.runCommand("RUN AOT")
	if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
		t.Fatalf("RUN AOT BLOAD failed: %q", out)
	}
	if !strings.Contains(out, "DONE") {
		t.Fatalf("RUN AOT BLOAD did not finish: %q", out)
	}
	for i, b := range payload {
		if h.cpu.memory[dst+i] != b {
			t.Fatalf("BLOAD byte %d: memory[%#x]=%#02x, want %#02x", i, dst+i, h.cpu.memory[dst+i], b)
		}
	}
}

// TestREPL_RunAOT_PokeExpression covers POKE/DOKE/LOKE with expression operands
// (variables/arithmetic). Integer-literal operands take the fast native-store path;
// expression operands fall back to delegating the whole statement to the resident
// exec_do_poke/doke/loke (which run expr_eval), so RUN AOT honours "every tokenised
// statement runs". Side effects observed in plain RAM at 0x50000.
func TestREPL_RunAOT_PokeExpression(t *testing.T) {
	const base = 0x50000 // 327680, plain RAM, 4-byte aligned

	t.Run("poke8_expr_value", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTLines(t, h, "10 V=42", "20 POKE8 327680, V", "30 END")
		if got := h.cpu.memory[base]; got != 42 {
			t.Fatalf("POKE8 expr value: memory[%#x]=%d, want 42", base, got)
		}
	})

	t.Run("poke_expr_addr_and_value", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTLines(t, h, "10 B=327680", "20 V=100", "30 POKE B, V+5", "40 END")
		// 32-bit store of 105; little-endian low byte at base.
		if got := h.cpu.memory[base]; got != 105 {
			t.Fatalf("POKE expr addr+value: memory[%#x]=%d, want 105", base, got)
		}
	})

	t.Run("doke_expr", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTLines(t, h, "10 B=327680", "20 V=513", "30 DOKE B, V", "40 END")
		// 513 = 0x0201 -> low byte 0x01 at base, high byte 0x02 at base+1.
		if h.cpu.memory[base] != 1 || h.cpu.memory[base+1] != 2 {
			t.Fatalf("DOKE expr: memory[%#x..]=%d,%d, want 1,2", base, h.cpu.memory[base], h.cpu.memory[base+1])
		}
	})

	t.Run("literal_still_native", func(t *testing.T) {
		// A literal POKE must still take the fast native-store path (no delegation).
		h, _ := startREPL(t)
		runAOTLines(t, h, "10 POKE8 327680, 7", "20 END")
		if got := h.cpu.memory[base]; got != 7 {
			t.Fatalf("POKE8 literal: memory[%#x]=%d, want 7", base, got)
		}
	})
}

// TestREPL_RunAOT_PrintString exercises the bundled-helper pipeline end to end:
// PRINT "literal" emits inline string data + a call to aot_print_str, which the
// compiler appends as helper source and assembles at the link base. Output must
// match interpreted RUN (string + CRLF).
func TestREPL_RunAOT_PrintString(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, `10 PRINT "HELLO"`)
		out := h.runCommand("RUN AOT")
		if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
			t.Fatalf("RUN AOT PRINT should compile, got: %q", out)
		}
		if !strings.Contains(out, "HELLO\r\n") {
			t.Fatalf("RUN AOT did not print HELLO+CRLF, got: %q", out)
		}
		if list := h.runCommand("LIST"); !strings.Contains(list, "10") {
			t.Fatalf("REPL broken after PRINT; LIST: %q", list)
		}
	})

	t.Run("parity_with_interpreter", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, `10 PRINT "ABCDE"`)
		interp := h.runCommand("RUN")
		aot := h.runCommand("RUN AOT")
		if !strings.Contains(interp, "ABCDE\r\n") {
			t.Fatalf("interpreted RUN missing output: %q", interp)
		}
		if !strings.Contains(aot, "ABCDE\r\n") {
			t.Fatalf("RUN AOT output differs from interpreter: %q", aot)
		}
	})

	t.Run("semicolon_suppresses_crlf", func(t *testing.T) {
		h, _ := startREPL(t)
		// "AB"; (no CRLF) then "CD" -> the two run together as "ABCD".
		storeLine(t, h, `10 PRINT "AB";`)
		storeLine(t, h, `20 PRINT "CD"`)
		out := h.runCommand("RUN AOT")
		if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
			t.Fatalf("RUN AOT should compile, got: %q", out)
		}
		if !strings.Contains(out, "ABCD\r\n") {
			t.Fatalf("semicolon did not suppress CRLF (want ABCD together), got: %q", out)
		}
	})

	t.Run("two_lines", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, `10 PRINT "ONE"`)
		storeLine(t, h, `20 PRINT "TWO"`)
		out := h.runCommand("RUN AOT")
		if !strings.Contains(out, "ONE\r\nTWO\r\n") {
			t.Fatalf("RUN AOT two-line PRINT, got: %q", out)
		}
	})
}

// TestREPL_RunAOT_Delegation exercises leaf-statement delegation to resident
// handlers: LET (variables), PRINT of variables/expressions, arithmetic — all
// must match interpreted RUN exactly.
func TestREPL_RunAOT_Delegation(t *testing.T) {
	progs := [][]string{
		{"10 A=42", "20 PRINT A"},
		{"10 A=10", "20 B=20", "30 PRINT A+B"},
		{"10 X=7", `20 PRINT "X=";X`},
		{"10 A=3.5", "20 PRINT A*2"},
		{"10 PRINT 2+3*4"},
		{"10 A=5", "20 A=A+1", "30 PRINT A"},
		{"10 DATA 5,10", "20 READ A", "30 READ B", "40 PRINT A+B"}, // READ/DATA: needs DATA ptr init
		{"10 A$=\"HI\"", `20 PRINT A$`},                            // string variables
	}
	for _, prog := range progs {
		t.Run(strings.Join(prog, "/"), func(t *testing.T) {
			h, _ := startREPL(t)
			for _, l := range prog {
				storeLine(t, h, l)
			}
			interp := h.runCommand("RUN")
			aot := h.runCommand("RUN AOT")
			if strings.Contains(aot, aotStubMarker) || strings.Contains(aot, "ERROR") {
				t.Fatalf("RUN AOT %v should compile, got: %q", prog, aot)
			}
			// Compare the program-output portion (after the compile banner) to RUN.
			ip := interpOutput(interp)
			ap := aotOutput(aot)
			if ip != ap {
				t.Fatalf("RUN AOT output %q != interpreted %q (prog %v)", ap, ip, prog)
			}
		})
	}
}

// interpOutput / aotOutput strip the REPL framing to compare program output.
func interpOutput(s string) string {
	if i := strings.Index(s, "\r\n"); i >= 0 { // drop the echoed RUN command line
		s = s[i+2:]
	}
	return strings.TrimSuffix(s, "Ready\r\n")
}

func aotOutput(s string) string {
	const banner = "Compiling to native code...\r\n"
	if i := strings.Index(s, banner); i >= 0 {
		s = s[i+len(banner):]
	}
	return strings.TrimSuffix(s, "Ready\r\n")
}

// TestREPL_RunAOT_ForNext covers native FOR/NEXT (resident exec_do_for/next for
// setup+increment, native loop-back branch).
func TestREPL_RunAOT_ForNext(t *testing.T) {
	progs := [][]string{
		{"10 FOR I=1 TO 3", "20 PRINT I", "30 NEXT"},                                        // basic count
		{"10 FOR I=1 TO 5 STEP 2", "20 PRINT I", "30 NEXT I"},                               // STEP + NEXT var
		{"10 FOR I=3 TO 1 STEP -1", "20 PRINT I", "30 NEXT"},                                // negative step
		{"10 S=0", "20 FOR I=1 TO 4", "30 S=S+I", "40 NEXT", "50 PRINT S"},                  // accumulate
		{"10 FOR I=1 TO 2", "20 FOR J=1 TO 2", "30 PRINT I*10+J", "40 NEXT J", "50 NEXT I"}, // nested
		{"10 FOR I=2 TO 1", "20 PRINT I", "30 NEXT", `40 PRINT "DONE"`},                     // zero-trip (1<2, step+1)
	}
	for _, prog := range progs {
		t.Run(prog[0], func(t *testing.T) {
			h, _ := startREPL(t)
			for _, l := range prog {
				storeLine(t, h, l)
			}
			interp := h.runCommand("RUN")
			aot := h.runCommand("RUN AOT")
			if strings.Contains(aot, aotStubMarker) || strings.Contains(aot, "ERROR") {
				t.Fatalf("RUN AOT %v should compile, got: %q", prog, aot)
			}
			if ip, ap := interpOutput(interp), aotOutput(aot); ip != ap {
				t.Fatalf("RUN AOT %q != interpreted %q (prog %v)", ap, ip, prog)
			}
		})
	}
}

// TestREPL_RunAOT_Loops covers native WHILE/WEND and DO/LOOP.
func TestREPL_RunAOT_Loops(t *testing.T) {
	progs := [][]string{
		{"10 I=1", "20 WHILE I<=3", "30 PRINT I", "40 I=I+1", "50 WEND"},
		{"10 I=5", "20 WHILE I>10", "30 PRINT I", "40 WEND", `50 PRINT "DONE"`},
		{"10 I=1", "20 DO", "30 PRINT I", "40 I=I+1", "50 LOOP WHILE I<=3"},
		{"10 I=1", "20 DO", "30 PRINT I", "40 I=I+1", "50 LOOP UNTIL I>3"},
		{"10 S=0", "20 I=1", "30 WHILE I<=2", "40 J=1", "50 WHILE J<=2", "60 S=S+I*10+J", "70 J=J+1", "80 WEND", "90 I=I+1", "100 WEND", "110 PRINT S"},
	}
	for _, prog := range progs {
		t.Run(prog[1], func(t *testing.T) {
			h, _ := startREPL(t)
			for _, l := range prog {
				storeLine(t, h, l)
			}
			interp := h.runCommand("RUN")
			aot := h.runCommand("RUN AOT")
			if strings.Contains(aot, aotStubMarker) || strings.Contains(aot, "ERROR") {
				t.Fatalf("RUN AOT %v should compile, got: %q", prog, aot)
			}
			if ip, ap := interpOutput(interp), aotOutput(aot); ip != ap {
				t.Fatalf("RUN AOT %q != interpreted %q (prog %v)", ap, ip, prog)
			}
		})
	}
}

// TestREPL_RunAOT_On covers native ON <expr> GOTO/GOSUB.
func TestREPL_RunAOT_On(t *testing.T) {
	progs := [][]string{
		{"10 ON 2 GOTO 100,200", `20 PRINT "FT"`, "30 END", `100 PRINT "ONE"`, "110 END", `200 PRINT "TWO"`, "210 END"},
		{"10 ON 1 GOTO 100,200", `20 PRINT "FT"`, "30 END", `100 PRINT "ONE"`, "110 END", `200 PRINT "TWO"`, "210 END"},
		{"10 ON 3 GOTO 100,200", `20 PRINT "FT"`, "30 END", `100 PRINT "ONE"`, "110 END", `200 PRINT "TWO"`, "210 END"}, // out of range
		{"10 A=2", "20 ON A GOTO 100,200", "30 END", `100 PRINT "ONE"`, "110 END", `200 PRINT "TWO"`, "210 END"},
		{"10 ON 1 GOSUB 100", `20 PRINT "BACK"`, "30 END", `100 PRINT "SUB"`, "110 RETURN"},
		{"10 ON 2 GOSUB 100,200", `20 PRINT "BACK"`, "30 END", `100 PRINT "S1"`, "110 RETURN", `200 PRINT "S2"`, "210 RETURN"},
	}
	for _, prog := range progs {
		t.Run(prog[0], func(t *testing.T) {
			h, _ := startREPL(t)
			for _, l := range prog {
				storeLine(t, h, l)
			}
			interp := h.runCommand("RUN")
			aot := h.runCommand("RUN AOT")
			if strings.Contains(aot, aotStubMarker) || strings.Contains(aot, "ERROR") {
				t.Fatalf("RUN AOT %v should compile, got: %q", prog, aot)
			}
			if ip, ap := interpOutput(interp), aotOutput(aot); ip != ap {
				t.Fatalf("RUN AOT %q != interpreted %q (prog %v)", ap, ip, prog)
			}
		})
	}
}

// TestREPL_RunAOT_If covers native IF...THEN lowering: condition via resident
// expr_eval, false branches to the per-line end label, THEN <line> is a GOTO.
func TestREPL_RunAOT_If(t *testing.T) {
	progs := [][]string{
		{`10 IF 1 THEN PRINT "YES"`},                                                    // true literal
		{`10 IF 0 THEN PRINT "NO"`, `20 PRINT "DONE"`},                                  // false -> skip THEN
		{"10 A=5", `20 IF A>3 THEN PRINT "BIG"`},                                        // relational true
		{"10 A=2", `20 IF A>3 THEN PRINT "BIG"`, `30 PRINT "END"`},                      // relational false
		{"10 A=5", "20 IF A=5 THEN A=10", "30 PRINT A"},                                 // THEN assignment
		{"10 IF 1 THEN 30", `20 PRINT "SKIP"`, `30 PRINT "HERE"`},                       // THEN <line> = GOTO
		{"10 IF 0 THEN 30", `20 PRINT "RUN"`, `30 PRINT "END"`},                         // THEN <line>, false
		{"10 A=3", `20 IF A>1 THEN IF A<5 THEN PRINT "MID"`},                            // nested IF on one line
		{`10 IF 1 THEN PRINT "T" ELSE PRINT "F"`},                                       // ELSE, true
		{`10 IF 0 THEN PRINT "T" ELSE PRINT "F"`},                                       // ELSE, false
		{"10 A=5", `20 IF A>3 THEN PRINT "BIG" ELSE PRINT "SMALL"`, `30 PRINT "AFTER"`}, // ELSE + next line
	}
	for _, prog := range progs {
		t.Run(strings.Join(prog, "/"), func(t *testing.T) {
			h, _ := startREPL(t)
			for _, l := range prog {
				storeLine(t, h, l)
			}
			interp := h.runCommand("RUN")
			aot := h.runCommand("RUN AOT")
			if strings.Contains(aot, aotStubMarker) || strings.Contains(aot, "ERROR") {
				t.Fatalf("RUN AOT %v should compile, got: %q", prog, aot)
			}
			if ip, ap := interpOutput(interp), aotOutput(aot); ip != ap {
				t.Fatalf("RUN AOT %q != interpreted %q (prog %v)", ap, ip, prog)
			}
		})
	}
}

// TestREPL_RunAOT_NestedIfElseRejected covers nested same-line IF/ELSE, which BASIC
// binds ELSE to the nearest (inner) IF. The single-level IF/ELSE lowering cannot
// represent that, so it must reject the shape as unsupported (the clean stub path)
// rather than emit mis-associated labels that fault the private assembler with an
// internal assembler error.
func TestREPL_RunAOT_NestedIfElseRejected(t *testing.T) {
	progs := [][]string{
		{`10 IF 0 THEN IF 1 THEN PRINT "A" ELSE PRINT "B"`, `20 PRINT "DONE"`},
		{`10 IF 1 THEN IF 0 THEN PRINT "A" ELSE PRINT "B"`, `20 PRINT "DONE"`},
	}
	for _, prog := range progs {
		t.Run(strings.Join(prog, "/"), func(t *testing.T) {
			h, _ := startREPL(t)
			for _, l := range prog {
				storeLine(t, h, l)
			}
			aot := h.runCommand("RUN AOT")
			if !strings.Contains(aot, aotStubMarker) {
				t.Fatalf("nested IF/ELSE should report unsupported (stub), got: %q", aot)
			}
			if strings.Contains(aot, "internal assembler error") {
				t.Fatalf("nested IF/ELSE must not reach the assembler with broken labels, got: %q", aot)
			}
		})
	}
}

// TestREPL_RunAOT_PrintNumber exercises bundling the real fp_print closure via
// machine-code copy: PRINT <integer literal> converts to FP32 at runtime and
// calls the bundled fp_print, so output matches interpreted RUN exactly.
func TestREPL_RunAOT_PrintNumber(t *testing.T) {
	cases := []string{"0", "1", "42", "100", "32767", "1000000"}
	for _, n := range cases {
		t.Run(n, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, "10 PRINT "+n)
			interp := h.runCommand("RUN")
			aot := h.runCommand("RUN AOT")
			if strings.Contains(aot, aotStubMarker) || strings.Contains(aot, "ERROR") {
				t.Fatalf("RUN AOT PRINT %s should compile, got: %q", n, aot)
			}
			// The interpreter prints the FP-formatted number then CRLF; the AOT
			// path must produce the identical bytes.
			want := n + "\r\n"
			if !strings.Contains(interp, want) {
				t.Fatalf("interpreted RUN of PRINT %s: %q (want substring %q)", n, interp, want)
			}
			if !strings.Contains(aot, want) {
				t.Fatalf("RUN AOT of PRINT %s differs from interpreter: %q (want %q)", n, aot, want)
			}
		})
	}

	t.Run("mixed_with_string", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, `10 PRINT "N="`)
		storeLine(t, h, "20 PRINT 7")
		out := h.runCommand("RUN AOT")
		if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
			t.Fatalf("RUN AOT mixed PRINT: %q", out)
		}
		if !strings.Contains(out, "N=\r\n7\r\n") {
			t.Fatalf("RUN AOT mixed PRINT output: %q", out)
		}
	})
}

// TestREPL_RunAOT_Wait compiles and runs WAIT natively. The polled condition is
// arranged to be already true so the loop exits on the first iteration; the
// following statement's side effect proves WAIT completed and execution carried
// on (the ~1M-iteration timeout also guarantees termination).
func TestREPL_RunAOT_Wait(t *testing.T) {
	const addr, flag = 0x50000, 0x50001 // 327680, 327681

	t.Run("immediate", func(t *testing.T) {
		h, _ := startREPL(t)
		// (5 EOR 0) AND 1 = 1 != 0 -> exits at once.
		runAOTLines(t, h,
			"10 POKE 327680, 5",
			"20 WAIT 327680, 1",
			"30 POKE8 327681, 42")
		if got := h.cpu.memory[flag]; got != 42 {
			t.Fatalf("WAIT did not continue: memory[%#x]=%d, want 42", flag, got)
		}
	})

	t.Run("with_xor", func(t *testing.T) {
		h, _ := startREPL(t)
		// (0 EOR 255) AND 255 = 255 != 0 -> exits at once (exercises the 3-arg form).
		runAOTLines(t, h,
			"10 POKE 327680, 0",
			"20 WAIT 327680, 255, 255",
			"30 POKE8 327681, 7")
		if got := h.cpu.memory[flag]; got != 7 {
			t.Fatalf("WAIT xor did not continue: memory[%#x]=%d, want 7", flag, got)
		}
	})
}

// TestREPL_Compile_WaitVsyncEmitsLoop checks the transpiled asm for WAIT and
// VSYNC contains the poll-loop skeleton with correctly numbered branch labels.
func TestREPL_Compile_WaitVsyncEmitsLoop(t *testing.T) {
	asmBin := buildAssembler(t)

	read := func(t *testing.T, line string) string {
		t.Helper()
		tmpDir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		storeLine(t, h, line)
		if out := h.runCommand(`COMPILE "demo"`); strings.Contains(out, "ERROR") {
			t.Fatalf("COMPILE %q: %q", line, out)
		}
		b, err := os.ReadFile(filepath.Join(tmpDir, "demo.asm"))
		if err != nil {
			t.Fatalf("demo.asm not written: %v", err)
		}
		return string(b)
	}

	for _, want := range []string{
		"move.l r1, #327680\n", "move.l r3, #1\n", "move.l r4, #0\n",
		"move.l r5, #1048576\n", "W0:\n", "load.l r2, (r1)\n",
		"eor.l r2, r2, r4\n", "and.l r2, r2, r3\n", "bne r2, r0, X0\n",
		"sub.q r5, r5, #1\n", "bne r5, r0, W0\n", "X0:\n",
	} {
		got := read(t, "10 WAIT 327680, 1")
		if !strings.Contains(got, want) {
			t.Fatalf("WAIT asm missing %q in:\n%s", want, got)
		}
	}

	// VSYNC = WAIT on VGA_STATUS (0xF1004 = 987140), mask VGA_STATUS_VSYNC (1).
	vs := read(t, "10 VSYNC")
	for _, want := range []string{"move.l r1, #987140\n", "move.l r3, #1\n", "W0:\n", "bne r5, r0, W0\n"} {
		if !strings.Contains(vs, want) {
			t.Fatalf("VSYNC asm missing %q in:\n%s", want, vs)
		}
	}
}

// TestREPL_Transpile writes only the NAME.asm sidecar (the first half of
// COMPILE) and no NAME.ie64. The emitted asm must be byte-for-byte identical to
// what COMPILE writes for the same programme, so TRANSPILE is a faithful
// transpile-only front-end.
func TestREPL_Transpile(t *testing.T) {
	asmBin := buildAssembler(t)

	prog := []string{
		`10 FOR I = 1 TO 10`,
		`20 PRINT I`,
		`30 NEXT I`,
		`40 PRINT "DONE"`,
	}

	// COMPILE reference: capture the .asm it writes.
	compileDir := t.TempDir()
	hc := newEhbasicREPLHarnessWithFileIO(t, asmBin, compileDir)
	hc.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	for _, l := range prog {
		storeLine(t, hc, l)
	}
	if out := hc.runCommand(`COMPILE "demo"`); strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	wantAsm, err := os.ReadFile(filepath.Join(compileDir, "demo.asm"))
	if err != nil {
		t.Fatalf("COMPILE demo.asm not written: %v", err)
	}

	// TRANSPILE: writes demo.asm, not demo.ie64.
	transDir := t.TempDir()
	ht := newEhbasicREPLHarnessWithFileIO(t, asmBin, transDir)
	ht.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	for _, l := range prog {
		storeLine(t, ht, l)
	}
	if out := ht.runCommand(`TRANSPILE "demo"`); strings.Contains(out, "ERROR") {
		t.Fatalf("TRANSPILE failed: %q", out)
	}
	gotAsm, err := os.ReadFile(filepath.Join(transDir, "demo.asm"))
	if err != nil {
		t.Fatalf("TRANSPILE demo.asm not written: %v", err)
	}
	if !bytes.Equal(gotAsm, wantAsm) {
		t.Fatalf("TRANSPILE asm differs from COMPILE asm:\n--- transpile ---\n%s\n--- compile ---\n%s", gotAsm, wantAsm)
	}
	if _, err := os.Stat(filepath.Join(transDir, "demo.ie64")); !os.IsNotExist(err) {
		t.Fatalf("TRANSPILE wrote demo.ie64 (err=%v); it must only emit the .asm sidecar", err)
	}

	// A bad name still raises ?FC ERROR, and an unsupported root still rejects.
	hb := newEhbasicREPLHarnessWithFileIO(t, asmBin, t.TempDir())
	hb.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	storeLine(t, hb, `10 PRINT "X"`)
	if out := hb.runCommand(`TRANSPILE "../escape"`); !strings.Contains(out, "?FC ERROR") {
		t.Fatalf("TRANSPILE bad name: want ?FC ERROR, got %q", out)
	}
	if out := hb.runCommand(`TRANSPILE`); !strings.Contains(out, "?SYNTAX ERROR") {
		t.Fatalf("TRANSPILE no arg: want ?SYNTAX ERROR, got %q", out)
	}
}

// TestREPL_CompileTranspileAssemble_RoundTrip proves the self-contained .asm is a
// true source artifact: for a programme that needs the runtime blob (variables)
// and the fp_print closure (PRINT of a number), TRANSPILE then ASSEMBLE produces
// the same .ie64 image as COMPILE does directly. This exercises the full
// pipeline split (transpile -> assemble) on the case that previously could not
// round-trip, because the blob/closure/programme are now inlined as dc.b data.
func TestREPL_CompileTranspileAssemble_RoundTrip(t *testing.T) {
	asmBin := buildAssembler(t)

	prog := []string{
		`10 DIM A(3)`,
		`20 FOR I = 0 TO 3`,
		`30 A(I) = I * I`,
		`40 PRINT A(I)`,
		`50 NEXT I`,
		`60 DATA 7, 8, 9`,
		`70 READ X`,
		`80 PRINT X`,
	}

	// COMPILE: the reference image and its self-contained sidecar.
	dir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	for _, l := range prog {
		storeLine(t, h, l)
	}
	if out := h.runCommand(`COMPILE "ref"`); strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	wantIe64, err := os.ReadFile(filepath.Join(dir, "ref.ie64"))
	if err != nil {
		t.Fatalf("COMPILE ref.ie64 not written: %v", err)
	}

	// TRANSPILE the same programme to a separate name, then ASSEMBLE that .asm.
	if out := h.runCommand(`TRANSPILE "rt"`); strings.Contains(out, "ERROR") {
		t.Fatalf("TRANSPILE failed: %q", out)
	}
	if out := h.runCommand(`ASSEMBLE "rt"`); strings.Contains(out, "ERROR") {
		t.Fatalf("ASSEMBLE of transpiled .asm failed: %q", out)
	}
	gotIe64, err := os.ReadFile(filepath.Join(dir, "rt.ie64"))
	if err != nil {
		t.Fatalf("ASSEMBLE did not write rt.ie64: %v", err)
	}

	if !bytes.Equal(gotIe64, wantIe64) {
		t.Fatalf("TRANSPILE+ASSEMBLE image differs from COMPILE: got %d bytes, want %d bytes",
			len(gotIe64), len(wantIe64))
	}
	if len(wantIe64) < 0x8000 {
		t.Fatalf("self-contained image unexpectedly small (%d bytes); the runtime blob may not be bundled", len(wantIe64))
	}
}

// TestREPL_Assemble covers the ASSEMBLE command: it reads a user-written
// NAME.asm from the File I/O root, assembles it at PROGRAM_START with the
// in-guest private assembler, and writes NAME.ie64. The output must match the
// host ie64asm oracle byte-for-byte for the same self-contained source, which
// exercises labels, immediates, PC-relative branches, ie64.inc named constants
// (via the baked aot_consttab), dc.b/l + align, and the `include "ie64.inc"`
// no-op. The source is deliberately base-independent (no absolute label refs)
// so PROGRAM_START (0x1000) and the oracle agree regardless of base.
func TestREPL_Assemble(t *testing.T) {
	asmBin := buildAssembler(t)

	const src = `include "ie64.inc"
start:
    move.l r26, #TERM_OUT
    move.q r1, #72
    store.b r1, (r26)
    move.q r1, #73
    store.b r1, (r26)
    move.q r2, #3
loop:
    sub.q r2, r2, #1
    bne r2, r0, loop
    halt
    align 8
table:
    dc.l 1, 2, 3
    dc.b 65, 66, 0
    align 4
`

	t.Run("matches_ie64asm_oracle", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "foo.asm"), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		// Oracle: assemble the identical source with the host ie64asm.
		inc := filepath.Join(repoRootDir(t), "sdk", "include")
		oracle := filepath.Join(dir, "oracle.asm")
		if err := os.WriteFile(oracle, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(asmBin, "-I", inc, oracle).CombinedOutput(); err != nil {
			t.Fatalf("oracle assembly failed: %v\n%s", err, out)
		}
		want, err := os.ReadFile(filepath.Join(dir, "oracle.ie64"))
		if err != nil {
			t.Fatal(err)
		}

		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "foo"`); strings.Contains(out, "ERROR") {
			t.Fatalf("ASSEMBLE failed: %q", out)
		}
		got, err := os.ReadFile(filepath.Join(dir, "foo.ie64"))
		if err != nil {
			t.Fatalf("ASSEMBLE did not write foo.ie64: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("ASSEMBLE output differs from ie64asm oracle: got %d bytes, want %d bytes\ngot=%x\nwant=%x",
				len(got), len(want), got, want)
		}
	})

	t.Run("unknown_directive_rejected", func(t *testing.T) {
		dir := t.TempDir()
		// org is not part of the in-guest assembler -> clean assembler error.
		if err := os.WriteFile(filepath.Join(dir, "bad.asm"), []byte("    org 0x2000\n    halt\n"), 0644); err != nil {
			t.Fatal(err)
		}
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "bad"`); !strings.Contains(out, "ERROR") {
			t.Fatalf("ASSEMBLE of org source: want assembler error, got %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "bad.ie64")); !os.IsNotExist(err) {
			t.Fatalf("ASSEMBLE wrote bad.ie64 despite assembler error (err=%v)", err)
		}
	})

	t.Run("foreign_include_rejected", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "inc.asm"), []byte("include \"other.inc\"\n    halt\n"), 0644); err != nil {
			t.Fatal(err)
		}
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "inc"`); !strings.Contains(out, "ERROR") {
			t.Fatalf("ASSEMBLE of foreign include: want assembler error, got %q", out)
		}
	})

	t.Run("include_trailing_junk_rejected", func(t *testing.T) {
		// include "ie64.inc" with trailing non-comment text must error, not be
		// silently dropped (which could hide a typo or lost source line).
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "jnk.asm"), []byte("include \"ie64.inc\" bogus\n    halt\n"), 0644); err != nil {
			t.Fatal(err)
		}
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "jnk"`); !strings.Contains(out, "ERROR") {
			t.Fatalf("ASSEMBLE of include with trailing junk: want assembler error, got %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "jnk.ie64")); !os.IsNotExist(err) {
			t.Fatalf("ASSEMBLE wrote jnk.ie64 despite trailing junk on the include line")
		}
	})

	t.Run("include_trailing_comment_ok", func(t *testing.T) {
		// A trailing comment after include "ie64.inc" is fine (still a no-op).
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "cmt.asm"), []byte("include \"ie64.inc\"  ; constants\n    halt\n"), 0644); err != nil {
			t.Fatal(err)
		}
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "cmt"`); strings.Contains(out, "ERROR") {
			t.Fatalf("ASSEMBLE of include with trailing comment: %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "cmt.ie64")); err != nil {
			t.Fatalf("ASSEMBLE did not write cmt.ie64: %v", err)
		}
	})

	t.Run("missing_file_is_file_error", func(t *testing.T) {
		dir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "nope"`); !strings.Contains(out, "?FILE ERROR") {
			t.Fatalf("ASSEMBLE of missing file: want ?FILE ERROR, got %q", out)
		}
	})

	t.Run("oversized_source_rejected", func(t *testing.T) {
		// A source larger than the ASSEMBLE buffer (just under 1 MiB) must be
		// refused cleanly by the File I/O range guard (the staging buffer is the
		// topmost low32 allocation, so an over-read runs past visible RAM, never
		// into the code/symbol workspace) rather than corrupting AOT workspace.
		dir := t.TempDir()
		big := append(bytes.Repeat([]byte("    halt\n"), 0x100000/9+512), 0)
		if len(big) <= 0x100000 {
			t.Fatalf("test source not larger than the cap: %d", len(big))
		}
		if err := os.WriteFile(filepath.Join(dir, "big.asm"), big, 0644); err != nil {
			t.Fatal(err)
		}
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "big"`); !strings.Contains(out, "?FILE ERROR") {
			t.Fatalf("ASSEMBLE of oversized source: want ?FILE ERROR, got %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "big.ie64")); !os.IsNotExist(err) {
			t.Fatalf("ASSEMBLE wrote big.ie64 for an oversized source (err=%v)", err)
		}
		// The REPL must still work afterwards: a normal command runs unscathed.
		if err := os.WriteFile(filepath.Join(dir, "ok.asm"), []byte("    halt\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if out := h.runCommand(`ASSEMBLE "ok"`); strings.Contains(out, "ERROR") {
			t.Fatalf("REPL did not survive an oversized ASSEMBLE: %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "ok.ie64")); err != nil {
			t.Fatalf("post-oversized ASSEMBLE did not write ok.ie64: %v", err)
		}
	})

	t.Run("oversized_backing_gt_active_rejected", func(t *testing.T) {
		// The case the address-range guard alone would miss: active visible RAM (the
		// alloc frontier, CR_RAM_SIZE_BYTES) is smaller than the bus backing, so a file
		// that overruns the source buffer would still fit under backingVisibleSize. The
		// device-side FILE_READ_MAX cap must refuse it before any byte is copied, so the
		// command reports ?FILE ERROR, writes no .ie64, and leaves the workspace intact.
		dir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		const active = 0x1000000 // 16 MiB active ceiling, below the 32 MiB bus backing
		h.bus.ApplyProfileVisibleCeiling(active)
		// 4 MiB source: larger than the 1 MiB cap, but base+len would stay under the
		// 32 MiB backing, so only the FILE_READ_MAX cap (not the range guard) stops it.
		big := append(bytes.Repeat([]byte("    halt\n"), 4*1024*1024/9), 0)
		if err := os.WriteFile(filepath.Join(dir, "huge.asm"), big, 0644); err != nil {
			t.Fatal(err)
		}
		if out := h.runCommand(`ASSEMBLE "huge"`); !strings.Contains(out, "?FILE ERROR") {
			t.Fatalf("ASSEMBLE oversized (backing>active): want ?FILE ERROR, got %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "huge.ie64")); !os.IsNotExist(err) {
			t.Fatalf("ASSEMBLE wrote huge.ie64 for an oversized source")
		}
		// Workspace intact: a normal ASSEMBLE still produces a correct image.
		if err := os.WriteFile(filepath.Join(dir, "ok.asm"), []byte("    halt\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if out := h.runCommand(`ASSEMBLE "ok"`); strings.Contains(out, "ERROR") {
			t.Fatalf("workspace corrupted by oversized ASSEMBLE (backing>active): %q", out)
		}
		if _, err := os.Stat(filepath.Join(dir, "ok.ie64")); err != nil {
			t.Fatalf("post-oversized ASSEMBLE did not write ok.ie64: %v", err)
		}
	})

	t.Run("bad_name_and_syntax", func(t *testing.T) {
		dir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		if out := h.runCommand(`ASSEMBLE "../escape"`); !strings.Contains(out, "?FC ERROR") {
			t.Fatalf("ASSEMBLE bad name: want ?FC ERROR, got %q", out)
		}
		if out := h.runCommand(`ASSEMBLE`); !strings.Contains(out, "?SYNTAX ERROR") {
			t.Fatalf("ASSEMBLE no arg: want ?SYNTAX ERROR, got %q", out)
		}
	})
}

// TestREPL_Type drives the direct TYPE "path" command: it prints valid
// ASCII/UTF-8 files to the terminal and refuses binary ones, mapping File I/O
// failures to the usual REPL messages. The whole file is read into the resident
// FILE_DATA_BUF, capped at the device by FILE_READ_MAX, so an over-large file is
// refused as ?FILE TOO LARGE before any byte is staged.
func TestREPL_Type(t *testing.T) {
	asmBin := buildAssembler(t)

	newH := func(t *testing.T, dir string) *ehbasicTestHarness {
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		return h
	}

	t.Run("prints_text_file", func(t *testing.T) {
		dir := t.TempDir()
		const body = "Hello, TYPE!\nsecond line\n"
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "hello.txt"`)
		if strings.Contains(out, "ERROR") || strings.Contains(out, "?") {
			t.Fatalf("TYPE of a text file reported an error: %q", out)
		}
		// type_print normalises line endings to CR+LF, so assert the lines
		// individually rather than depending on a bare '\n' between them.
		if !strings.Contains(out, "Hello, TYPE!") || !strings.Contains(out, "second line") {
			t.Fatalf("TYPE did not print the file body: %q", out)
		}
		if !strings.Contains(out, "Hello, TYPE!\r\nsecond line") {
			t.Fatalf("TYPE did not normalise LF to CR+LF: %q", out)
		}
	})

	t.Run("path_separators_allowed", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "inner.txt"), []byte("NESTED\n"), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "sub/inner.txt"`)
		if !strings.Contains(out, "NESTED") || strings.Contains(out, "?") {
			t.Fatalf("TYPE of a nested path: want NESTED, got %q", out)
		}
	})

	t.Run("no_trailing_newline_gets_one", func(t *testing.T) {
		// repl_do_type appends a CRLF when the file lacks a final LF, so the
		// "Ready" prompt is never glued onto the last line of the file.
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "noeol.txt"), []byte("NOEOL"), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "noeol.txt"`)
		if !strings.Contains(out, "NOEOL") {
			t.Fatalf("TYPE did not print the file body: %q", out)
		}
		if strings.Contains(out, "NOEOLReady") {
			t.Fatalf("TYPE glued the prompt to the file (no separating newline): %q", out)
		}
	})

	t.Run("trailing_newline_no_double", func(t *testing.T) {
		// A file that already ends in LF must not gain an extra blank line.
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "eol.txt"), []byte("LINE1\n"), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "eol.txt"`)
		if !strings.Contains(out, "LINE1") {
			t.Fatalf("TYPE did not print the file body: %q", out)
		}
		if strings.Contains(out, "LINE1\n\r\nReady") || strings.Contains(out, "LINE1\n\nReady") {
			t.Fatalf("TYPE added an extra blank line after an LF-terminated file: %q", out)
		}
	})

	t.Run("prints_utf8", func(t *testing.T) {
		dir := t.TempDir()
		// "café ✓ 𝓍" - 2-, 3- and 4-byte UTF-8 sequences.
		body := "café ✓ \U0001D4CD\n"
		if err := os.WriteFile(filepath.Join(dir, "utf8.txt"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "utf8.txt"`)
		if strings.Contains(out, "?NOT A TEXT FILE") {
			t.Fatalf("TYPE rejected a valid UTF-8 file: %q", out)
		}
		if !strings.Contains(out, "caf") {
			t.Fatalf("TYPE did not print the UTF-8 file: %q", out)
		}
	})

	t.Run("rejects_binary", func(t *testing.T) {
		dir := t.TempDir()
		// A NUL and a 0xFF make this unambiguously non-text; the ASCII MARKER must
		// never reach the screen because nothing is printed on rejection.
		blob := []byte("MARKER\x00\xff\x01\x02binary")
		if err := os.WriteFile(filepath.Join(dir, "blob.bin"), blob, 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "blob.bin"`)
		if !strings.Contains(out, "?NOT A TEXT FILE") {
			t.Fatalf("TYPE of a binary file: want ?NOT A TEXT FILE, got %q", out)
		}
		if strings.Contains(out, "MARKER") {
			t.Fatalf("TYPE printed bytes from a rejected binary file: %q", out)
		}
	})

	t.Run("prints_latin1", func(t *testing.T) {
		dir := t.TempDir()
		// ISO-8859-1 (Latin-1): "café" with é = 0xE9, a lone high byte that is NOT
		// valid UTF-8 but is accepted as a printable extended character.
		if err := os.WriteFile(filepath.Join(dir, "latin1.txt"), []byte("caf\xe9 \xa9 ok\n"), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "latin1.txt"`)
		if strings.Contains(out, "?NOT A TEXT FILE") {
			t.Fatalf("TYPE rejected a Latin-1 file: %q", out)
		}
		if !strings.Contains(out, "caf") || !strings.Contains(out, "ok") {
			t.Fatalf("TYPE did not print the Latin-1 file: %q", out)
		}
	})

	t.Run("rejects_control_without_nul", func(t *testing.T) {
		dir := t.TempDir()
		// No NUL, but ESC (0x1B) and BEL (0x07) are control bytes -> still binary.
		if err := os.WriteFile(filepath.Join(dir, "ctrl.txt"), []byte("text\x1bmore\x07end"), 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "ctrl.txt"`)
		if !strings.Contains(out, "?NOT A TEXT FILE") {
			t.Fatalf("TYPE of a control-byte file: want ?NOT A TEXT FILE, got %q", out)
		}
		if strings.Contains(out, "text") && strings.Contains(out, "more") {
			t.Fatalf("TYPE printed bytes from a rejected control file: %q", out)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		dir := t.TempDir()
		out := newH(t, dir).runCommand(`TYPE "nope.txt"`)
		if !strings.Contains(out, "?FILE NOT FOUND") {
			t.Fatalf("TYPE of a missing file: want ?FILE NOT FOUND, got %q", out)
		}
	})

	t.Run("too_large", func(t *testing.T) {
		dir := t.TempDir()
		// Larger than the FILE_DATA_BUF span (just under 1 MiB), so the device
		// FILE_READ_MAX cap refuses it before staging a byte.
		big := bytes.Repeat([]byte("A"), 0x100000+0x100)
		if err := os.WriteFile(filepath.Join(dir, "big.txt"), big, 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "big.txt"`)
		if !strings.Contains(out, "?FILE TOO LARGE") {
			t.Fatalf("TYPE of an over-large file: want ?FILE TOO LARGE, got %q", out)
		}
		// The REPL must survive: a normal TYPE still works afterwards.
		if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("STILL OK\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if out := newH(t, dir).runCommand(`TYPE "ok.txt"`); !strings.Contains(out, "STILL OK") {
			t.Fatalf("REPL did not survive an over-large TYPE: %q", out)
		}
	})

	t.Run("empty_file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		out := newH(t, dir).runCommand(`TYPE "empty.txt"`)
		if strings.Contains(out, "?") {
			t.Fatalf("TYPE of an empty file reported an error: %q", out)
		}
	})

	t.Run("no_argument_is_syntax_error", func(t *testing.T) {
		dir := t.TempDir()
		h := newH(t, dir)
		if out := h.runCommand(`TYPE`); !strings.Contains(out, "?SYNTAX ERROR") {
			t.Fatalf("TYPE no arg: want ?SYNTAX ERROR, got %q", out)
		}
		if out := h.runCommand(`TYPE hello`); !strings.Contains(out, "?SYNTAX ERROR") {
			t.Fatalf("TYPE unquoted arg: want ?SYNTAX ERROR, got %q", out)
		}
		if out := h.runCommand(`TYPE ""`); !strings.Contains(out, "?SYNTAX ERROR") {
			t.Fatalf("TYPE empty path: want ?SYNTAX ERROR, got %q", out)
		}
	})
}

// TestREPL_RunAOT_ReturnWithoutGosub: a RETURN reached without a matching
// compiled GOSUB frame must report "?RETURN WITHOUT GOSUB" (like the interpreter
// raising ERR_RET_NO_GOSUB), not silently end with success. Following statements
// must not run, and the REPL must survive.
func TestREPL_RunAOT_ReturnWithoutGosub(t *testing.T) {
	const a, b = 0x50000, 0x50001

	// State block fields (BASIC_STATE = 0x022000): ST_ERROR_FLAG +0x38,
	// ST_ERROR_LINE +0x6C. raise_error must persist both, exactly like the
	// interpreter's exec_do_return, and report the offending line.
	const stErrorFlag, stErrorLine = 0x022038, 0x02206C

	t.Run("bare", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, "10 RETURN")
		out := h.runCommand("RUN AOT")
		if !strings.Contains(out, "RETURN WITHOUT GOSUB ERROR IN 10") {
			t.Fatalf("expected RETURN WITHOUT GOSUB error on line 10, got: %q", out)
		}
		if f := h.bus.Read32(stErrorFlag); f != 5 { // ERR_RET_NO_GOSUB
			t.Fatalf("ST_ERROR_FLAG=%d, want 5", f)
		}
		if l := h.bus.Read32(stErrorLine); l != 10 {
			t.Fatalf("ST_ERROR_LINE=%d, want 10", l)
		}
		if list := h.runCommand("LIST"); !strings.Contains(list, "10") {
			t.Fatalf("REPL broken after bad RETURN; LIST: %q", list)
		}
	})

	t.Run("halts_following", func(t *testing.T) {
		h, _ := startREPL(t)
		storeLine(t, h, "10 POKE8 327680, 1 : RETURN")
		storeLine(t, h, "20 POKE8 327681, 2")
		out := h.runCommand("RUN AOT")
		if !strings.Contains(out, "RETURN WITHOUT GOSUB ERROR IN 10") {
			t.Fatalf("expected RETURN WITHOUT GOSUB error on line 10, got: %q", out)
		}
		if got := h.cpu.memory[a]; got != 1 {
			t.Fatalf("line 10 not run: memory[%#x]=%d, want 1", a, got)
		}
		if got := h.cpu.memory[b]; got != 0 {
			t.Fatalf("execution continued past bad RETURN: memory[%#x]=%d, want 0", b, got)
		}
		if l := h.bus.Read32(stErrorLine); l != 10 {
			t.Fatalf("ST_ERROR_LINE=%d, want 10", l)
		}
	})

	// A matched GOSUB/RETURN must still work (the SP guard only fires when no
	// frame is active).
	t.Run("matched_ok", func(t *testing.T) {
		h, _ := startREPL(t)
		runAOTLines(t, h,
			"10 GOSUB 100",
			"20 POKE8 327680, 7",
			"30 END",
			"100 RETURN")
		if got := h.cpu.memory[a]; got != 7 {
			t.Fatalf("matched GOSUB/RETURN broke: memory[%#x]=%d, want 7", a, got)
		}
	})
}

// TestREPL_RunAOT_GotoUndefinedLine: a branch to a non-existent line leaves the
// label unresolved, so the private assembler must report a compile error (not
// crash or hang) and the REPL must survive.
func TestREPL_RunAOT_GotoUndefinedLine(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 GOTO 999")
	out := h.runCommand("RUN AOT")
	if !strings.Contains(out, "COMPILE ERROR") {
		t.Fatalf("GOTO to undefined line should be a compile error, got: %q", out)
	}
	if list := h.runCommand("LIST"); !strings.Contains(list, "10") {
		t.Fatalf("REPL broken after failed RUN AOT; LIST: %q", list)
	}
}

// TestREPL_RunAOT_RecompileInvalidatesJIT pins the regression where a second
// RUN AOT (after editing the programme) ran the previous JIT-translated block
// because the arena address is reused. With JIT enabled, two compiles of POKE8
// to the same address but different values must both take effect.
func TestREPL_RunAOT_RecompileInvalidatesJIT(t *testing.T) {
	if !jitAvailable {
		t.Skip("IE64 JIT not available on this host")
	}
	h, _ := startREPL(t)
	h.cpu.jitEnabled = true
	// runCommand stops ExecuteJIT between commands, which frees the JIT cache
	// unless jitPersist is set. Keep the cache so the first RUN AOT's translated
	// arena block survives into the second - that is the stale block tlbflush
	// must invalidate. Without this the test would pass even without the fix.
	h.cpu.jitPersist = true
	t.Cleanup(func() { h.cpu.jitPersist = false })
	const addr = 0x50000

	storeLine(t, h, "10 POKE8 327680, 11")
	if out := h.runCommand("RUN AOT"); strings.Contains(out, "ERROR") {
		t.Fatalf("first RUN AOT: %q", out)
	}
	if got := h.cpu.memory[addr]; got != 11 {
		t.Fatalf("first RUN AOT: memory[%#x]=%d, want 11", addr, got)
	}

	// Edit the programme and recompile into the same arena.
	storeLine(t, h, "10 POKE8 327680, 99")
	if out := h.runCommand("RUN AOT"); strings.Contains(out, "ERROR") {
		t.Fatalf("second RUN AOT: %q", out)
	}
	if got := h.cpu.memory[addr]; got != 99 {
		t.Fatalf("recompiled RUN AOT ran stale JIT block: memory[%#x]=%d, want 99", addr, got)
	}
}

// TestAOTConsttabInSync guards against the committed generated constant table
// going stale after constants are added/changed in ie64.inc. It regenerates to a
// temp file and compares; if this fails, run `make gen-aot-consttab`.
func TestAOTConsttabInSync(t *testing.T) {
	root := repoRootDir(t)
	tmp := filepath.Join(t.TempDir(), "aot_consttab.inc")
	cmd := exec.Command("go", "run", "./tools/gen_aot_consttab", "-out", tmp)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gen_aot_consttab failed: %v\n%s", err, out)
	}
	got, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(root, "sdk", "include", "aot_consttab.inc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("sdk/include/aot_consttab.inc is stale - run `make gen-aot-consttab` (or `go run ./tools/gen_aot_consttab`)")
	}
}

// TestEhbasicImageFitsBelowState guards the prebuilt EhBASIC image against growing
// into the live low-RAM working region. The flat image loads at PROGRAM_START
// (0x1000). The first live region above the code is the input line buffer at
// BASIC_LINE_BUF (0x021000), NOT the state block at 0x022000: RUN AOT uses the line
// buffer while executing, so an image whose end crosses 0x021000 overwrites/executes
// the line-buffer region and corrupts RUN AOT (invalid opcodes), even though it
// still sits below the state block. The bound is therefore BASIC_LINE_BUF, not
// BASIC_STATE. (Regression: adding ~360 bytes of AOT constant-table entries pushed
// the image to 0x2111C and broke FOR/NEXT/WHILE/DO/ON RUN AOT.)
func TestEhbasicImageFitsBelowState(t *testing.T) {
	const programStart = 0x001000
	const basicLineBuf = 0x021000
	const budget = basicLineBuf - programStart // 0x20000
	img := filepath.Join(repoRootDir(t), "sdk", "examples", "prebuilt", "ehbasic_ie64.ie64")
	fi, err := os.Stat(img)
	if err != nil {
		t.Skipf("prebuilt image not built: %v", err)
	}
	if fi.Size() > budget {
		t.Fatalf("EhBASIC image %d bytes exceeds budget %d (would overwrite the line buffer at %#x and corrupt RUN AOT)",
			fi.Size(), budget, basicLineBuf)
	}
	if fi.Size() > budget*9/10 {
		t.Logf("WARNING: EhBASIC image %d bytes is within 10%% of the %d-byte budget (line buffer at %#x)",
			fi.Size(), budget, basicLineBuf)
	}
}

// TestAOT_FpPrintClosureParity proves the private assembler handles real, large
// interpreter source at scale: it assembles fp_print and its self-contained
// closure (fp_neg..fp_fix_local, ~350 lines of FP code with local labels and
// named constants) and compares byte-for-byte to ie64asm. This de-risks bundling
// the FP runtime before wiring it to statements.
func TestAOT_FpPrintClosureParity(t *testing.T) {
	asmBin := buildAssembler(t)
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")

	// Extract the closure (from "fp_neg:" up to, but not including,
	// "fp_print_to_buf:", which is the only routine pulling in an external label).
	fpSrc, err := os.ReadFile(filepath.Join(incDir, "ie64_fp.inc"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(fpSrc), "\n")
	start, end := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "fp_neg:") && start < 0 {
			start = i
		}
		if strings.HasPrefix(l, "fp_print_to_buf:") {
			end = i
			break
		}
	}
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("could not locate fp_print closure (start=%d end=%d)", start, end)
	}
	closure := strings.Join(lines[start:end], "\n")

	dir := t.TempDir()
	src := filepath.Join(dir, "fpc.asm")
	if err := os.WriteFile(src, []byte("    include \"ie64.inc\"\n    org 0x1000\n"+closure+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, "-I", incDir, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "fpc.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	codeLen := len(ref)
	if codeLen < 256 {
		t.Fatalf("closure too small (%d bytes) - extraction likely wrong", codeLen)
	}

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(closure) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x033000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .fpc_done
` + data.String() + ".fpc_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(32_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1 (closure %d bytes)", st, codeLen)
	}
	if cl := int(h.bus.Read64(0x032008)); cl != codeLen {
		t.Fatalf("codeLen=%d, want %d", cl, codeLen)
	}
	for i := 0; i < codeLen; i++ {
		if h.cpu.memory[0x033000+i] != ref[i] {
			t.Fatalf("FP closure parity mismatch at byte %d (instr %d): got %#02x want %#02x",
				i, i/8, h.cpu.memory[0x033000+i], ref[i])
		}
	}
}

// TestAOT_SymbolCapacity assembles a programme with far more than the old
// 64-symbol limit, confirming the symbol table now lives in (larger) workspace.
// Byte-for-byte parity with ie64asm.
func TestAOT_SymbolCapacity(t *testing.T) {
	asmBin := buildAssembler(t)

	const n = 200 // > old AOT_SYM_MAX (64)
	var p strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&p, "L%d:\n    add.q r1, r1, #1\n", i)
	}
	p.WriteString("    bra L0") // resolve a label defined far earlier
	prog := p.String()

	dir := t.TempDir()
	src := filepath.Join(dir, "cap.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+prog+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "cap.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	codeLen := len(ref)

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(prog) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x033000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .cap_done
` + data.String() + ".cap_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(16_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1 (%d labels)", st, n)
	}
	if cl := int(h.bus.Read64(0x032008)); cl != codeLen {
		t.Fatalf("codeLen=%d, want %d", cl, codeLen)
	}
	for i := 0; i < codeLen; i++ {
		if h.cpu.memory[0x033000+i] != ref[i] {
			t.Fatalf("capacity parity mismatch at byte %d", i)
		}
	}
}

// TestAOT_LocalLabelParity checks scoped local labels (".loop" reused under
// different global labels resolves to the local within its own scope), matching
// ie64asm byte-for-byte. Helper source relies on local labels pervasively.
func TestAOT_LocalLabelParity(t *testing.T) {
	asmBin := buildAssembler(t)

	prog := `first:
    move.q r1, #3
.loop:
    sub.q r1, r1, #1
    bne r1, r0, .loop
    bnez r1, .done
.done:
    rts
second:
    move.q r2, #5
.loop:
    sub.q r2, r2, #1
    bnez r2, .loop
    bra .done
.done:
    rts`

	dir := t.TempDir()
	src := filepath.Join(dir, "ll2.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+prog+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "ll2.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	codeLen := len(ref)

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(prog) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .ll2_done
` + data.String() + ".ll2_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(8_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1", st)
	}
	if cl := int(h.bus.Read64(0x032008)); cl != codeLen {
		t.Fatalf("aot_asm_program codeLen=%d, want %d", cl, codeLen)
	}
	got := make([]byte, codeLen)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < codeLen; i++ {
		if got[i] != ref[i] {
			li := i / 8
			t.Fatalf("local-label parity mismatch byte %d (instr %d): got %#02x want %#02x\ngot  % x\nwant % x",
				i, li, got[i], ref[i], got[li*8:li*8+8], ref[li*8:li*8+8])
		}
	}
}

// TestAOT_ConstantParity checks the private assembler resolves named equ
// constants (e.g. #ST_ERROR_FLAG, la r1, BASIC_STATE) via its built-in constant
// table, byte-for-byte against ie64asm (which resolves them from ie64.inc).
func TestAOT_ConstantParity(t *testing.T) {
	asmBin := buildAssembler(t)
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")

	// Exercises the in-guest assembler resolving named constants through the
	// build-time generated table across the BASIC/system/assembler namespaces it
	// keeps (BASIC_, ST_, ERR_, TERM_, SYS_). Hardware peripheral registers
	// (VGA_/SID_/...) are intentionally not in the table - BASIC reaches hardware via
	// numeric-lowered statements, not symbolic bundled source - so they are not
	// sampled here. See tools/gen_aot_consttab includeConsttabPrefixes.
	snippet := `    la r1, BASIC_STATE
    move.q r2, #ST_ERROR_FLAG
    add.q r3, r16, #ST_CURRENT_LINE
    move.l r4, #ERR_RET_NO_GOSUB
    la r7, TERM_OUT
    la r8, TERM_STATUS
    add.q r9, r16, #ST_TERM_COL
    la r10, BASIC_VAR_START
    la r11, BASIC_GOSUB_STACK
    la r12, SYS_GC_TRIGGER`

	dir := t.TempDir()
	src := filepath.Join(dir, "ct.asm")
	if err := os.WriteFile(src, []byte("    include \"ie64.inc\"\n    org 0x1000\n"+snippet+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, "-I", incDir, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "ct.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	const codeLen = 80 // 10 instructions
	if len(ref) < codeLen {
		t.Fatalf("ref too short: %d", len(ref))
	}
	ref = ref[:codeLen]

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(snippet) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .ct_done
` + data.String() + ".ct_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1", st)
	}
	if cl := int(h.bus.Read64(0x032008)); cl != codeLen {
		t.Fatalf("aot_asm_program codeLen=%d, want %d", cl, codeLen)
	}
	got := make([]byte, codeLen)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < codeLen; i++ {
		if got[i] != ref[i] {
			li := i / 8
			t.Fatalf("constant parity mismatch byte %d (instr %d): got %#02x want %#02x\ngot  % x\nwant % x",
				i, li, got[i], ref[i], got[li*8:li*8+8], ref[li*8:li*8+8])
		}
	}
}

// TestAOT_ProgramParity checks the two-pass multi-line program assembler:
// labels defined and resolved across the program, branches PC-relative, output
// byte-for-byte equal to the Go ie64asm oracle.
func TestAOT_ProgramParity(t *testing.T) {
	asmBin := buildAssembler(t)

	prog := `start:
    move.q r1, #10
loop:
    sub.q r1, r1, #1
    bne r1, r0, loop
    add.q r2, r2, #5
done:
    jmp (r3)`

	// Reference: same program at link base 0x1000 (the assembler default).
	dir := t.TempDir()
	src := filepath.Join(dir, "prog.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+prog+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "prog.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	const codeLen = 40 // 5 instructions
	if len(ref) < codeLen {
		t.Fatalf("ref too short: %d", len(ref))
	}
	ref = ref[:codeLen]

	// Private side: assemble the same text (no org) at link base 0x1000.
	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(prog) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .pp_done
` + data.String() + ".pp_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(8_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1", st)
	}
	if cl := h.bus.Read64(0x032008); cl != codeLen {
		t.Fatalf("aot_asm_program codeLen=%d, want %d", cl, codeLen)
	}
	got := make([]byte, codeLen)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < codeLen; i++ {
		if got[i] != ref[i] {
			li := i / 8
			t.Fatalf("program parity mismatch byte %d (instr %d): got %#02x want %#02x\ngot  % x\nwant % x",
				i, li, got[i], ref[i], got[li*8:li*8+8], ref[li*8:li*8+8])
		}
	}
}

// TestAOT_StopCaptureParity validates the exact instruction forms the arena STOP
// lowering (aot_emit_stop) emits to capture the full 64-bit resume PC without `la`:
// "bra P<id>" to skip the helper, "pop r5" to take the hardware return address, and
// "jsr V<id>" to push it. The private assembler must encode them byte-for-byte like
// the Go ie64asm oracle, otherwise a STOP/CONT image would store a corrupt resume PC.
func TestAOT_StopCaptureParity(t *testing.T) {
	asmBin := buildAssembler(t)

	// Mirrors the aot_emit_stop capture sequence: skip the helper, capture PC+8 via
	// jsr/pop, unwind, rts; the resume label C is reached only via CONT's jsr.
	prog := `bra past
cap:
    pop r5
    move.l r6, #141504
    store.q r5, (r6)
    move.l r6, #141400
    load.q r31, (r6)
    rts
past:
    jsr cap
cont:
    add.q r1, r1, #1`

	dir := t.TempDir()
	src := filepath.Join(dir, "gc.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+prog+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "gc.ie64"))
	if err != nil {
		t.Fatal(err)
	}

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(prog) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .gc_done
` + data.String() + ".gc_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(8_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1", st)
	}
	codeLen := h.bus.Read64(0x032008)
	if codeLen == 0 || int(codeLen) > len(ref) {
		t.Fatalf("private codeLen=%d, ref len=%d", codeLen, len(ref))
	}
	if int(codeLen) != len(ref) {
		t.Fatalf("private codeLen=%d, ref len=%d (length mismatch)", codeLen, len(ref))
	}
	for i := 0; i < int(codeLen); i++ {
		if h.cpu.memory[0x031000+i] != ref[i] {
			li := i / 8
			t.Fatalf("stop-capture parity mismatch byte %d (instr %d): got %#02x want %#02x",
				i, li, h.cpu.memory[0x031000+i], ref[i])
		}
	}
}

// TestAOT_DataDirectiveParity checks the private assembler's data directives
// (dc.b strings/bytes, dc.w/l/q, align) and la label resolution against the Go
// ie64asm oracle byte-for-byte, with labels defined across instructions and data
// and the variable-length two-pass layout.
func TestAOT_DataDirectiveParity(t *testing.T) {
	asmBin := buildAssembler(t)

	prog := `start:
    la r1, msg
    la r2, tbl
    bra done
msg:
    dc.b "Hi!", 0
    align 4
tbl:
    dc.l start, done
    dc.w 1, 2, 3
    dc.q 0x1122334455667788
    dc.b 0xAA, 0xBB
    align 8
done:
    rts`

	dir := t.TempDir()
	src := filepath.Join(dir, "prog.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+prog+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "prog.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	codeLen := len(ref)

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(prog) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .dd_done
` + data.String() + ".dd_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(8_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1", st)
	}
	if cl := int(h.bus.Read64(0x032008)); cl != codeLen {
		t.Fatalf("aot_asm_program codeLen=%d, want %d", cl, codeLen)
	}
	got := make([]byte, codeLen)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < codeLen; i++ {
		if got[i] != ref[i] {
			t.Fatalf("data directive parity mismatch byte %d: got %#02x want %#02x\ngot  % x\nwant % x",
				i, got[i], ref[i], got, ref)
		}
	}
}

// TestAOT_LongLabelParity checks the private assembler resolves arbitrary-length
// labels (helper-style names like exec_do_return, err_msg_ret_no_gosub) defined
// and referenced across la, branches and dc.l, byte-for-byte against ie64asm.
// This is the prerequisite for assembling bundled runtime helper source.
func TestAOT_LongLabelParity(t *testing.T) {
	asmBin := buildAssembler(t)

	prog := `exec_do_return:
    la r1, err_msg_ret_no_gosub
    bra exec_do_return_done
    bne r1, r0, exec_do_return
    beqz r2, ptr_table
err_msg_ret_no_gosub:
    dc.b "RETURN WITHOUT GOSUB", 0
    align 8
ptr_table:
    dc.l exec_do_return, err_msg_ret_no_gosub, exec_do_return_done
exec_do_return_done:
    rts`

	dir := t.TempDir()
	src := filepath.Join(dir, "ll.asm")
	if err := os.WriteFile(src, []byte("    org 0x1000\n"+prog+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "ll.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	codeLen := len(ref)

	var data strings.Builder
	data.WriteString(".prog:\n    dc.b ")
	for j, b := range []byte(prog) {
		if j > 0 {
			data.WriteString(", ")
		}
		fmt.Fprintf(&data, "0x%02X", b)
	}
	data.WriteString(", 0\n    align 4\n")

	body := `    la      r8, .prog
    move.q  r9, #0x1000
    la      r10, 0x031000
    jsr     aot_asm_program
    la      r1, 0x032000
    store.q r8, (r1)
    la      r1, 0x032008
    store.q r9, (r1)
    bra     .ll_done
` + data.String() + ".ll_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(8_000_000)

	if st := h.bus.Read64(0x032000); st != 1 {
		t.Fatalf("aot_asm_program status=%d, want 1", st)
	}
	if cl := int(h.bus.Read64(0x032008)); cl != codeLen {
		t.Fatalf("aot_asm_program codeLen=%d, want %d", cl, codeLen)
	}
	got := make([]byte, codeLen)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < codeLen; i++ {
		if got[i] != ref[i] {
			t.Fatalf("long-label parity mismatch byte %d: got %#02x want %#02x\ngot  % x\nwant % x",
				i, got[i], ref[i], got, ref)
		}
	}
}

// TestAOT_BranchParity checks branch/jsr/jmp assembly with symbol resolution
// matches the Go ie64asm oracle byte-for-byte, including PC-relative offsets
// and the register-indirect jsr/jmp forms.
func TestAOT_BranchParity(t *testing.T) {
	asmBin := buildAssembler(t)

	// Reference program at a known base. Labels resolved by the real assembler;
	// the first six instructions are the branches under test.
	refSrc := `    org 0x1000
L0:
    bra L2
    beq r1, r2, L0
    bne r3, r4, L2
    jsr L1
    jmp (r5)
    jsr (r6)
L1:
    add.q r1, r1, r1
L2:
    sub.q r2, r2, r2`
	dir := t.TempDir()
	src := filepath.Join(dir, "br.asm")
	if err := os.WriteFile(src, []byte(refSrc+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "br.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) < 48 {
		t.Fatalf("ref too short: %d", len(ref))
	}
	ref = ref[:48]

	// Private side: same labels at the same absolute addresses, same PCs.
	type bl struct {
		line string
		pc   uint32
	}
	branches := []bl{
		{"bra L2", 0x1000},
		{"beq r1, r2, L0", 0x1008},
		{"bne r3, r4, L2", 0x1010},
		{"jsr L1", 0x1018},
		{"jmp (r5)", 0x1020},
		{"jsr (r6)", 0x1028},
	}
	syms := []struct {
		name string
		addr uint32
	}{{"L0", 0x1000}, {"L1", 0x1030}, {"L2", 0x1038}}

	var code, data strings.Builder
	code.WriteString("    jsr     aot_sym_reset\n")
	for i, s := range syms {
		fmt.Fprintf(&code, `    la      r8, .n%d
    jsr     aot_parse_label
    move.q  r9, r11
    move.q  r10, #%#x
    jsr     aot_sym_add
`, i, s.addr)
		fmt.Fprintf(&data, ".n%d:\n    dc.b ", i)
		for j, b := range []byte(s.name) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", b)
		}
		data.WriteString(", 0\n    align 4\n")
	}
	code.WriteString("    la      r9, 0x031000\n")
	for i, b := range branches {
		fmt.Fprintf(&code, `    la      r8, .b%d
    move.q  r10, #%#x
    jsr     aot_asm_line
    la      r1, %#x
    store.q r8, (r1)
`, i, b.pc, 0x032000+i*8)
		fmt.Fprintf(&data, ".b%d:\n    dc.b ", i)
		for j, c := range []byte(b.line) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", c)
		}
		data.WriteString(", 0\n    align 4\n")
	}
	body := code.String() + "    bra .br_done\n" + data.String() + ".br_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	for i, b := range branches {
		if st := h.bus.Read64(uint32(0x032000 + i*8)); st != 1 {
			t.Errorf("aot_asm_line(%q): status=%d, want 1", b.line, st)
		}
	}
	got := make([]byte, 48)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < 48; i++ {
		if got[i] != ref[i] {
			li := i / 8
			t.Fatalf("branch parity mismatch byte %d (line %d %q): got %#02x want %#02x\ngot  % x\nwant % x",
				i, li, branches[li].line, got[i], ref[i], got[li*8:li*8+8], ref[li*8:li*8+8])
		}
	}
}

// TestAOT_BzParity checks the beqz/bnez pseudo-branches (single register +
// label, lowered to beq/bne rs, r0, label) match the ie64asm oracle, including
// PC-relative offsets resolved through the symbol table.
func TestAOT_BzParity(t *testing.T) {
	asmBin := buildAssembler(t)

	refSrc := `    org 0x1000
L0:
    beqz r1, L1
    bnez r2, L0
L1:
    rts`
	dir := t.TempDir()
	src := filepath.Join(dir, "bz.asm")
	if err := os.WriteFile(src, []byte(refSrc+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(asmBin, src).CombinedOutput(); err != nil {
		t.Fatalf("ref assembly failed: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(filepath.Join(dir, "bz.ie64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) < 16 {
		t.Fatalf("ref too short: %d", len(ref))
	}
	ref = ref[:16]

	lines := []struct {
		line string
		pc   uint32
	}{
		{"beqz r1, L1", 0x1000},
		{"bnez r2, L0", 0x1008},
	}
	syms := []struct {
		name string
		addr uint32
	}{{"L0", 0x1000}, {"L1", 0x1010}}

	var code, data strings.Builder
	code.WriteString("    jsr     aot_sym_reset\n")
	for i, s := range syms {
		fmt.Fprintf(&code, "    la      r8, .n%d\n    jsr     aot_parse_label\n    move.q  r9, r11\n    move.q  r10, #%#x\n    jsr     aot_sym_add\n", i, s.addr)
		fmt.Fprintf(&data, ".n%d:\n    dc.b ", i)
		for j, b := range []byte(s.name) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", b)
		}
		data.WriteString(", 0\n    align 4\n")
	}
	code.WriteString("    la      r9, 0x031000\n")
	for i, b := range lines {
		fmt.Fprintf(&code, "    la      r8, .b%d\n    move.q  r10, #%#x\n    jsr     aot_asm_line\n    la      r1, %#x\n    store.q r8, (r1)\n", i, b.pc, 0x032000+i*8)
		fmt.Fprintf(&data, ".b%d:\n    dc.b ", i)
		for j, c := range []byte(b.line) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", c)
		}
		data.WriteString(", 0\n    align 4\n")
	}
	body := code.String() + "    bra .bz_done\n" + data.String() + ".bz_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	for i, b := range lines {
		if st := h.bus.Read64(uint32(0x032000 + i*8)); st != 1 {
			t.Errorf("aot_asm_line(%q): status=%d, want 1", b.line, st)
		}
	}
	got := make([]byte, 16)
	for i := range got {
		got[i] = h.cpu.memory[0x031000+i]
	}
	for i := 0; i < 16; i++ {
		if got[i] != ref[i] {
			t.Fatalf("bz parity mismatch byte %d: got %#02x want %#02x\ngot  % x\nwant % x",
				i, got[i], ref[i], got, ref)
		}
	}
}

// TestAOT_AsmLineRejectsTrailingTokens ensures the line assembler fails (rather
// than silently emitting and ignoring the suffix) when valid operands are
// followed by extra non-space text. A valid line with only trailing spaces must
// still succeed.
func TestAOT_AsmLineRejectsTrailingTokens(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		line string
		ok   bool
	}{
		{"move.q r1, r2 garbage", false},
		{"add.q r1, r2, #1x", false},
		{"add.q r1, r2, r3 extra", false},
		{"neg.q r1, r2 x", false},
		{"load.l r3, 16(r5) junk", false},
		{"store.q r7, (r8))", false},
		{"move.q r1 r2", false},     // missing comma (rd/src)
		{"add.q r1 r2, r3", false},  // missing first comma
		{"add.q r1, r2 r3", false},  // missing second comma
		{"load.l r3 16(r5)", false}, // missing comma (rd/address)
		{"neg.q r1 r2", false},      // ALU2 missing comma
		{"move.qr1, r2", false},     // size suffix glued to operand
		{"bra.qtarget", false},      // suffixed branch glued to label
		{"add.qr1, r2, r3", false},  // suffix glued, ALU3
		{"move.q r1, r2   ", true},  // trailing spaces only
		{"add.q r1, r2, #5", true},
		{"load.l r3, 16(r5)", true},
	}

	var code, data strings.Builder
	code.WriteString("    la      r9, 0x031000\n")
	for i, c := range cases {
		fmt.Fprintf(&code, `    la      r8, .tl%d
    jsr     aot_asm_line
    la      r1, %#x
    store.q r8, (r1)
`, i, 0x032000+i*8)
		fmt.Fprintf(&data, ".tl%d:\n    dc.b ", i)
		for j, b := range []byte(c.line) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", b)
		}
		data.WriteString(", 0\n    align 4\n")
	}
	body := code.String() + "    bra .tl_done\n" + data.String() + ".tl_done:"

	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(4_000_000)

	for i, c := range cases {
		st := h.bus.Read64(uint32(0x032000 + i*8))
		want := uint64(0)
		if c.ok {
			want = 1
		}
		if st != want {
			t.Errorf("aot_asm_line(%q): status=%d, want %d", c.line, st, want)
		}
	}
}

// aotMnemCase is one mnemonic-tokenizer test vector.
type aotMnemCase struct {
	in    string
	found bool
	op    byte
	class byte
	size  byte
	stop  byte // char where the mnemonic+suffix ends
}

func TestAOT_ParseMnem(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []aotMnemCase{
		{"add.q r1,r2,r3", true, 0x20, 0, 3, ' '},
		{"sub.l x", true, 0x21, 0, 2, ' '},
		{"store r1,(r2)", true, 0x11, 4, 3, ' '},
		{"move.b r1,#5", true, 0x01, 2, 0, ' '},
		{"neg r1", true, 0x27, 1, 3, ' '},
		{"or r1,r2,r3", true, 0x31, 0, 3, ' '},
		{"add", true, 0x20, 0, 3, 0},
		{"xyz r1", false, 0, 0, 0, 0},
		{"add.z r1", false, 0, 0, 0, 0},
		{"move.qr1, r2", false, 0, 0, 0, 0}, // suffix glued to operand
		{"bra.qtarget", false, 0, 0, 0, 0},  // suffixed branch glued to label
		{"add.qr1", false, 0, 0, 0, 0},      // suffix glued, no separator
	}

	var code, data strings.Builder
	const base = 0x031000
	for i, c := range cases {
		slot := base + i*40
		fmt.Fprintf(&code, `    la      r8, .m%d
    jsr     aot_parse_mnem
    la      r1, %#x
    store.q r12, (r1)
    move.q  r2, r0
    beqz    r12, .ms%d
    load.b  r2, (r11)
.ms%d:
    add.q   r1, r1, #8
    store.q r8, (r1)
    add.q   r1, r1, #8
    store.q r9, (r1)
    add.q   r1, r1, #8
    store.q r10, (r1)
    add.q   r1, r1, #8
    store.q r2, (r1)
`, i, slot, i, i)
		fmt.Fprintf(&data, ".m%d:\n    dc.b ", i)
		for j, b := range []byte(c.in) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", b)
		}
		data.WriteString(", 0\n    align 4\n")
	}

	body := code.String() + "    bra .mnem_done\n" + data.String() + ".mnem_done:"
	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(2_000_000)

	for i, c := range cases {
		slot := uint32(base + i*40)
		found := h.bus.Read64(slot)
		op := byte(h.bus.Read64(slot + 8))
		class := byte(h.bus.Read64(slot + 16))
		size := byte(h.bus.Read64(slot + 24))
		stop := byte(h.bus.Read64(slot + 32))
		if !c.found {
			if found != 0 {
				t.Errorf("%q: found=%d, want 0 (unrecognised)", c.in, found)
			}
			continue
		}
		if found != 1 {
			t.Errorf("%q: found=%d, want 1", c.in, found)
			continue
		}
		if op != c.op || class != c.class || size != c.size || stop != c.stop {
			t.Errorf("%q: op=%#x class=%d size=%d stop=%#x; want op=%#x class=%d size=%d stop=%#x",
				c.in, op, class, size, stop, c.op, c.class, c.size, c.stop)
		}
	}
}

// aotParseCase is one operand-parser test vector.
type aotParseCase struct {
	in      string // operand text
	routine string // aot_parse_reg or aot_parse_uint
	valid   bool   // expect a successful parse
	val     uint64 // expected parsed value (when valid)
	stop    byte   // expected char where parsing stopped (when valid)
}

// runAotParseCases assembles a driver that runs each case through its parser and
// stores [valid, value, stopChar] (3 q-words per case) at 0x031000, then checks.
func runAotParseCases(t *testing.T, cases []aotParseCase) {
	t.Helper()
	asmBin := buildAssembler(t)

	var code, data strings.Builder
	const base = 0x031000
	for i, c := range cases {
		slot := base + i*24
		fmt.Fprintf(&code, `    la      r8, .s%d
    jsr     %s
    la      r1, %#x
    store.q r9, (r1)
    move.q  r2, r0
    beqz    r9, .skip%d
    load.b  r2, (r10)
.skip%d:
    add.q   r1, r1, #8
    store.q r8, (r1)
    add.q   r1, r1, #8
    store.q r2, (r1)
`, i, c.routine, slot, i, i)
		// Encode the operand text as bytes so quotes/escapes are irrelevant.
		fmt.Fprintf(&data, ".s%d:\n    dc.b ", i)
		for j, b := range []byte(c.in) {
			if j > 0 {
				data.WriteString(", ")
			}
			fmt.Fprintf(&data, "0x%02X", b)
		}
		if len(c.in) > 0 {
			data.WriteString(", ")
		}
		data.WriteString("0\n    align 4\n")
	}

	body := code.String() + "    bra .aot_pt_done\n" + data.String() + ".aot_pt_done:"
	bin := assembleAOTUnit(t, asmBin, body)
	h := newEhbasicHarness(t)
	h.loadBytes(bin)
	h.runCycles(2_000_000)

	for i, c := range cases {
		slot := uint32(base + i*24)
		valid := h.bus.Read64(slot)
		val := h.bus.Read64(slot + 8)
		stop := byte(h.bus.Read64(slot + 16))
		if c.valid {
			if valid != 1 {
				t.Errorf("%s(%q): valid=%d, want 1", c.routine, c.in, valid)
				continue
			}
			if val != c.val {
				t.Errorf("%s(%q): value=%d, want %d", c.routine, c.in, val, c.val)
			}
			if stop != c.stop {
				t.Errorf("%s(%q): stop char=%#x, want %#x", c.routine, c.in, stop, c.stop)
			}
		} else if valid != 0 {
			t.Errorf("%s(%q): valid=%d, want 0 (reject)", c.routine, c.in, valid)
		}
	}
}

func TestAOT_ParseReg(t *testing.T) {
	runAotParseCases(t, []aotParseCase{
		{in: "r0", routine: "aot_parse_reg", valid: true, val: 0, stop: 0},
		{in: "r31", routine: "aot_parse_reg", valid: true, val: 31, stop: 0},
		{in: "sp", routine: "aot_parse_reg", valid: true, val: 31, stop: 0},
		{in: "r5,", routine: "aot_parse_reg", valid: true, val: 5, stop: ','},
		{in: "  r12 ", routine: "aot_parse_reg", valid: true, val: 12, stop: ' '},
		{in: "r32", routine: "aot_parse_reg", valid: false},
		{in: "r3x", routine: "aot_parse_reg", valid: false},
		{in: "rq", routine: "aot_parse_reg", valid: false},
		{in: "x1", routine: "aot_parse_reg", valid: false},
	})
}

func TestAOT_ParseUint(t *testing.T) {
	runAotParseCases(t, []aotParseCase{
		{in: "0", routine: "aot_parse_uint", valid: true, val: 0, stop: 0},
		{in: "42", routine: "aot_parse_uint", valid: true, val: 42, stop: 0},
		{in: "0xFF", routine: "aot_parse_uint", valid: true, val: 255, stop: 0},
		{in: "&HA0", routine: "aot_parse_uint", valid: true, val: 160, stop: 0},
		{in: "255,", routine: "aot_parse_uint", valid: true, val: 255, stop: ','},
		{in: "0xdeadBEEF", routine: "aot_parse_uint", valid: true, val: 0xDEADBEEF, stop: 0},
		{in: "4000000000", routine: "aot_parse_uint", valid: true, val: 4000000000, stop: 0},
		{in: "0x", routine: "aot_parse_uint", valid: false},
		{in: "&Z", routine: "aot_parse_uint", valid: false},
		{in: "abc", routine: "aot_parse_uint", valid: false},
	})
}

// =============================================================================
// EhBASIC IE64 AOT compiler - Phase 2 (front-end) tests
//
// These exercise the REPL recognition and COMPILE filename validation layer.
// The code-generation backend is stubbed at this phase: an accepted RUN AOT or
// COMPILE reaches a placeholder that reports it has not yet been lowered. The
// tests assert the front-end behaviour (recognition, banner, reasoned
// validation errors), not native output.
// =============================================================================

// aotStubMarker is the temporary message the stubbed backend prints once the
// front-end has accepted a compile request. Backend phases remove it.
const aotStubMarker = "native code generation not yet implemented"

func TestREPL_RunAOT_PrintsCompilingBanner(t *testing.T) {
	h, _ := startREPL(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)

	// Empty programme: RUN AOT compiles an immediate END (rts) into the arena,
	// runs it, and returns to the prompt - banner printed, no error/stub.
	output := h.runCommand("RUN AOT")

	if !strings.Contains(output, "Compiling to native code...") {
		t.Fatalf("RUN AOT: expected compiling banner, got: %q", output)
	}
	if strings.Contains(output, "ERROR") || strings.Contains(output, aotStubMarker) {
		t.Fatalf("RUN AOT on empty programme should run cleanly, got: %q", output)
	}
}

// A stored programme with an unsupported statement still reaches the stub.
func TestREPL_RunAOT_UnsupportedShowsStub(t *testing.T) {
	h, _ := startREPL(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	storeLine(t, h, "10 CONT") // CONT (REPL command) not lowerable
	output := h.runCommand("RUN AOT")
	if !strings.Contains(output, "Compiling to native code...") {
		t.Fatalf("RUN AOT: expected banner, got: %q", output)
	}
	if !strings.Contains(output, aotStubMarker) {
		t.Fatalf("RUN AOT on unsupported programme: expected stub, got: %q", output)
	}
}

func TestREPL_RunAOT_TrailingSpacesStillCompile(t *testing.T) {
	h, _ := startREPL(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)

	output := h.runCommand("RUN AOT   ")
	if !strings.Contains(output, "Compiling to native code...") {
		t.Fatalf("RUN AOT with trailing spaces should still compile, got: %q", output)
	}
	if strings.Contains(output, "ERROR") {
		t.Fatalf("RUN AOT with trailing spaces should run cleanly, got: %q", output)
	}
}

func TestREPL_RunAOT_RejectsExtraArguments(t *testing.T) {
	cases := []string{
		`RUN AOT "file"`,
		`RUN AOT demo`,
		`RUN AOT 10`,
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			h, _ := startREPL(t)
			output := h.runCommand(cmd)
			if !strings.Contains(output, "SYNTAX ERROR") {
				t.Fatalf("%q: expected syntax error for trailing args, got: %q", cmd, output)
			}
			if strings.Contains(output, "Compiling to native code...") {
				t.Fatalf("%q: must not compile with trailing args, got: %q", cmd, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: must not reach backend with trailing args, got: %q", cmd, output)
			}
		})
	}
}

// Punctuation immediately after AOT must be treated as a (rejected) trailing
// argument, never falling through to interpreted RUN of the stored program.
func TestREPL_RunAOT_RejectsPunctuationTails(t *testing.T) {
	cmds := []string{
		`RUN AOT,1`,
		`RUN AOT=1`,
		`RUN AOT"file"`,
		`RUN AOT;`,
	}
	for _, cmd := range cmds {
		t.Run(cmd, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, "10 PRINT 99")
			output := h.runCommand(cmd)
			if !strings.Contains(output, "SYNTAX ERROR") {
				t.Fatalf("%q: expected syntax error, got: %q", cmd, output)
			}
			if strings.Contains(output, "Compiling to native code...") || strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: must not compile, got: %q", cmd, output)
			}
			for line := range strings.SplitSeq(output, "\n") {
				if strings.TrimRight(line, "\r") == "99" {
					t.Fatalf("%q: must not run the stored program, got: %q", cmd, output)
				}
			}
		})
	}
}

func TestREPL_RunAOT_DoesNotBreakPlainRun(t *testing.T) {
	h, _ := startREPL(t)

	h.sendInput("10 PRINT 99\n")
	_ = h.runUntilPrompt()

	output := h.runCommand("RUN")
	if !strings.Contains(output, "99") {
		t.Fatalf("plain RUN regressed after adding RUN AOT, got: %q", output)
	}
	if strings.Contains(output, "Compiling to native code...") {
		t.Fatalf("plain RUN must not trigger AOT compile, got: %q", output)
	}
}

func TestREPL_Compile_MissingArgIsSyntaxError(t *testing.T) {
	h, _ := startREPL(t)

	output := h.runCommand("COMPILE")
	if !strings.Contains(output, "SYNTAX ERROR") {
		t.Fatalf("COMPILE with no argument: expected syntax error, got: %q", output)
	}
}

func TestREPL_Compile_UnquotedArgIsSyntaxError(t *testing.T) {
	h, _ := startREPL(t)

	output := h.runCommand("COMPILE demo")
	if !strings.Contains(output, "SYNTAX ERROR") {
		t.Fatalf("COMPILE with unquoted argument: expected syntax error, got: %q", output)
	}
}

func TestREPL_Compile_BadFilenames(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"empty", `COMPILE ""`},
		{"absolute", `COMPILE "/tmp/demo"`},
		{"dotdot", `COMPILE "../demo"`},
		{"separator", `COMPILE "sub/demo"`},
		{"backslash", `COMPILE "sub\demo"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := startREPL(t)
			output := h.runCommand(tc.cmd)
			if !strings.Contains(output, "FC ERROR IN 0") {
				t.Fatalf("%s: expected ?FC ERROR IN 0, got: %q", tc.cmd, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%s: bad name must not reach backend, got: %q", tc.cmd, output)
			}
		})
	}
}

func TestREPL_Compile_RejectsTrailingJunk(t *testing.T) {
	cases := []string{
		`COMPILE "demo" junk`,
		`COMPILE "demo" "x"`,
		`COMPILE "demo",`,
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			h, _ := startREPL(t)
			output := h.runCommand(cmd)
			if !strings.Contains(output, "SYNTAX ERROR") {
				t.Fatalf("%q: expected syntax error for trailing text, got: %q", cmd, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: must not reach backend, got: %q", cmd, output)
			}
		})
	}
}

// TestREPL_Compile_RejectsDelegatedStatements: delegated statements call resident
// handlers via stmt_jump_table, which do not exist in a standalone .ie64. COMPILE
// must report unsupported and write no (broken) binary, while standalone-native
// statements still compile.
func TestREPL_Compile_RejectsDelegatedStatements(t *testing.T) {
	asmBin := buildAssembler(t)

	t.Run("delegated_rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		storeLine(t, h, "10 GET A") // GET delegates in RUN AOT; not lowered standalone
		out := h.runCommand(`COMPILE "demo"`)
		if !strings.Contains(out, aotStubMarker) {
			t.Fatalf("COMPILE of a delegated statement must report unsupported, got: %q", out)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "demo.ie64")); err == nil {
			t.Fatalf("COMPILE wrote a binary for an unsupported (delegated) program")
		}
	})

	t.Run("native_string_compiles", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		storeLine(t, h, `10 PRINT "HI"`) // standalone-native bundled string lowering
		out := h.runCommand(`COMPILE "demo"`)
		if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
			t.Fatalf("COMPILE of PRINT string should succeed standalone, got: %q", out)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "demo.ie64")); err != nil {
			t.Fatalf("COMPILE did not write the .ie64: %v", err)
		}
	})
}

// COMPILE of a lowerable programme writes both NAME.ie64 (machine code) and
// NAME.asm (transpiled source) into the File I/O sandbox.
func TestREPL_Compile_WritesIE64AndAsm(t *testing.T) {
	asmBin := buildAssembler(t)
	tmpDir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
	// Publish a guest RAM size so the AOT allocator (mfcr cr15) has memory.
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)

	storeLine(t, h, "10 END")
	output := h.runCommand(`COMPILE "demo"`)
	if strings.Contains(output, "ERROR") {
		t.Fatalf("COMPILE printed an error: %q", output)
	}

	asmBytes, err := os.ReadFile(filepath.Join(tmpDir, "demo.asm"))
	if err != nil {
		t.Fatalf("demo.asm not written: %v", err)
	}
	// Standalone bootstrap (stack + R26/R27/R16) precedes the labelled line; END
	// emits a halt and the transpiler appends a trailing halt safety net.
	const standaloneProlog = "move.l r31, #651264\nmove.l r26, #984832\n" +
		"move.l r27, #984836\nmove.l r16, #139264\n"
	wantAsm := standaloneProlog + "L10:\nhalt\nhalt\n"
	if string(asmBytes) != wantAsm {
		t.Errorf("demo.asm = %q, want %q", asmBytes, wantAsm)
	}

	ie64Bytes, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
	if err != nil {
		t.Fatalf("demo.ie64 not written: %v", err)
	}
	// Four bootstrap move.l words then two "halt" words (the label emits no
	// code). Entry is the first bootstrap instruction; the halts use opcode 0xE1.
	if len(ie64Bytes) != 48 || ie64Bytes[32] != 0xE1 || ie64Bytes[40] != 0xE1 {
		t.Errorf("demo.ie64 = % x, want 4 move.l words + two 8-byte halts (0xE1 ...)", ie64Bytes)
	}

	// The REPL must survive COMPILE: the compiler clobbers callee-saved
	// registers, so without save/restore the next command would hang.
	if out := h.runCommand("LIST"); !strings.Contains(out, "10") {
		t.Fatalf("REPL broken after COMPILE; LIST gave: %q", out)
	}
}

// TestREPL_Compile_WritesBesideLoadedProgramme checks the source-directory
// lifecycle: a successful LOAD records the loaded path's directory, so COMPILE
// writes its output there rather than the File I/O root; NEW clears it again.
func TestREPL_Compile_WritesBesideLoadedProgramme(t *testing.T) {
	asmBin := buildAssembler(t)
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "sub", "prog.bas"), []byte("10 PRINT 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)

	// LOAD from a subdirectory, then COMPILE: output lands beside the source.
	if out := h.runCommand(`LOAD "sub/prog.bas"`); strings.Contains(out, "ERROR") || strings.Contains(out, "NOT FOUND") {
		t.Fatalf("LOAD failed: %q", out)
	}
	if out := h.runCommand(`COMPILE "out"`); strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "sub", "out.ie64")); err != nil {
		t.Fatalf("COMPILE did not write beside the loaded programme (sub/out.ie64): %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "out.ie64")); err == nil {
		t.Fatalf("COMPILE wrote to the root instead of the source directory")
	}

	// NEW clears the tracked directory: a subsequent COMPILE goes to the root.
	h.runCommand("NEW")
	storeLine(t, h, "10 PRINT 2")
	if out := h.runCommand(`COMPILE "rooted"`); strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE after NEW failed: %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "rooted.ie64")); err != nil {
		t.Fatalf("COMPILE after NEW did not write to the root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "sub", "rooted.ie64")); err == nil {
		t.Fatalf("COMPILE after NEW still used the stale source directory")
	}
}

// TestREPL_Compile_StandaloneRuns compiles PRINT programmes to standalone .ie64
// images, then loads and runs each image in a fresh machine with no resident
// interpreter present, proving the bundled helpers (fp_print closure for numbers,
// aot_print_str source for strings) and the standalone bootstrap actually work.
func TestREPL_Compile_StandaloneRuns(t *testing.T) {
	asmBin := buildAssembler(t)

	cases := []struct {
		name string
		prog string
		want string
	}{
		{"number", "10 PRINT 42", "42"},
		{"string", `10 PRINT "HI"`, "HI"},
		{"both", "10 PRINT 7" + "\n" + `20 PRINT "OK"`, "OK"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, line := range strings.Split(tc.prog, "\n") {
				storeLine(t, h, line)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}

			// Run the standalone image in a clean machine (no interpreter image).
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(4_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %q output=%q, want it to contain %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneIf is the Phase 2 end-to-end proof: COMPILE lowers an
// IF condition through the bundled runtime blob. The compiled standalone image, with
// no resident interpreter, boots by reading aot_runtime_blob.bin from the File I/O
// root into AOT_RT_BASE, runs var_init, then evaluates the IF condition via the blob
// expr_eval and branches. Tested against interpreted output.
func TestREPL_Compile_StandaloneIf(t *testing.T) {
	asmBin := buildAssembler(t)

	cases := []struct {
		name string
		prog []string
		want string
	}{
		{"true", []string{`10 IF 1 THEN PRINT "YES"`}, "YES"},
		{"false", []string{`10 IF 0 THEN PRINT "NO"`, `20 PRINT "DONE"`}, "DONE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			// No sidecar seeded: the harness serves the runtime blob virtually
			// (SetRuntimeBlob), mirroring the host's embedded blob, so COMPILE needs
			// no aot_runtime_blob.bin in the File I/O root.
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, line := range tc.prog {
				storeLine(t, h, line)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}

			// Run the standalone image in a clean machine with NO File I/O and no
			// sidecar: the blob is bundled into the image, so it is self-contained.
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandalonePrintExpr proves standalone PRINT of a numeric
// expression: COMPILE bundles the expression tokens, the standalone image evaluates
// them via the bundled expr_eval and prints the result through the bundled fp_print
// closure, with no resident interpreter.
func TestREPL_Compile_StandalonePrintExpr(t *testing.T) {
	asmBin := buildAssembler(t)

	cases := []struct {
		prog string
		want string
	}{
		{`10 PRINT 2+3`, "5"},
		{`10 PRINT 7*6`, "42"},
		{`10 PRINT 100-1`, "99"},
		{`10 PRINT (2+3)*4`, "20"},
	}
	for _, tc := range cases {
		t.Run(tc.prog, func(t *testing.T) {
			tmpDir := t.TempDir()
			// No sidecar: the harness serves the runtime blob virtually.
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			storeLine(t, h, tc.prog)
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			fio := NewFileIODevice(run.bus, tmpDir)
			run.bus.MapIO(FILE_IO_BASE, FILE_IO_END, fio.HandleRead, fio.HandleWrite)
			run.bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fio.HandleWrite8)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %q output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneVariables proves standalone variable assignment and
// use: COMPILE lowers an implied LET to the bundled exec_do_let (var_lookup +
// expr_eval + store) and reads variables back through the bundled expr_eval, all in
// the runtime blob, with no resident interpreter.
func TestREPL_Compile_StandaloneVariables(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		prog []string
		want string
	}{
		{[]string{`10 A=5`, `20 PRINT A`}, "5"},
		{[]string{`10 A=5`, `20 B=A*2`, `30 PRINT B`}, "10"},
		{[]string{`10 X=3`, `20 IF X>2 THEN PRINT "BIG"`}, "BIG"},
		{[]string{`10 A=10`, `20 A=A+5`, `30 PRINT A`}, "15"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.prog, "/"), func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, l := range tc.prog {
				storeLine(t, h, l)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneStringsAndMixed proves standalone string variables and
// mixed PRINT: COMPILE delegates the whole PRINT to the bundled exec_do_print (string
// and numeric items, concatenation, semicolons) and string assignment to exec_do_let.
func TestREPL_Compile_StandaloneStringsAndMixed(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		name string
		prog []string
		want string
	}{
		{"strvar", []string{`10 A$="HELLO"`, `20 PRINT A$`}, "HELLO"},
		{"concat", []string{`10 A$="AB"`, `20 B$=A$+"CD"`, `30 PRINT B$`}, "ABCD"},
		{"mixed", []string{`10 A=5`, `20 PRINT "X=";A`}, "X=5"},
		{"two-items", []string{`10 A=3`, `20 B=4`, `30 PRINT A;",";B`}, "3,4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, l := range tc.prog {
				storeLine(t, h, l)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneArrays proves standalone arrays: COMPILE delegates DIM
// to the bundled exec_do_dim (arr_dim), and array element assignment/read flow
// through the bundled exec_do_let / exec_do_print (var_lookup handles subscripts).
func TestREPL_Compile_StandaloneArrays(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		name string
		prog []string
		want string
	}{
		{"set-get", []string{`10 DIM A(5)`, `20 A(2)=10`, `30 PRINT A(2)`}, "10"},
		{"loop-fill", []string{`10 DIM A(3)`, `20 FOR I=1 TO 3`, `30 A(I)=I*I`, `40 NEXT`, `50 PRINT A(3)`}, "9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, l := range tc.prog {
				storeLine(t, h, l)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneControlFlow proves standalone FOR/NEXT, WHILE/WEND,
// DO/LOOP and ON GOTO, all lowered through the bundled runtime (exec_do_for/next and
// blob expr_eval), running with no resident interpreter.
func TestREPL_Compile_StandaloneControlFlow(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		name string
		prog []string
		want string
	}{
		{"for", []string{`10 FOR I=1 TO 3`, `20 PRINT I`, `30 NEXT`}, "1\r\n2\r\n3\r\n"},
		{"for-sum", []string{`10 S=0`, `20 FOR I=1 TO 5`, `30 S=S+I`, `40 NEXT`, `50 PRINT S`}, "15"},
		{"while", []string{`10 I=1`, `20 WHILE I<=3`, `30 PRINT I`, `40 I=I+1`, `50 WEND`}, "1\r\n2\r\n3\r\n"},
		{"do-loop", []string{`10 I=1`, `20 DO`, `30 PRINT I`, `40 I=I+1`, `50 LOOP UNTIL I>3`}, "1\r\n2\r\n3\r\n"},
		{"on-goto", []string{`10 ON 2 GOTO 100,200`, `20 END`, `100 PRINT "ONE"`, `110 END`, `200 PRINT "TWO"`, `210 END`}, "TWO"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, l := range tc.prog {
				storeLine(t, h, l)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneReadData proves standalone READ/DATA/RESTORE. The
// transpiler bundles the tokenised programme (the DATA records live in it) after
// the runtime blob and the bootstrap copies it to AOT_RT_PROG, pointing
// state[ST_PROG_START] at the relocated copy with ST_DATA_PTR = 0. The bundled
// exec_do_read/exec_do_restore then scan the copied programme for DATA values.
func TestREPL_Compile_StandaloneReadData(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		name string
		prog []string
		want string
	}{
		{"read", []string{`10 READ A`, `20 PRINT A`, `30 DATA 42`}, "42"},
		{"read-loop", []string{`10 FOR I=1 TO 3`, `20 READ A`, `30 PRINT A`, `40 NEXT I`, `50 DATA 11,22,33`}, "11\r\n22\r\n33\r\n"},
		{"restore", []string{`10 READ A`, `20 RESTORE`, `30 READ B`, `40 PRINT B`, `50 DATA 14,99`}, "14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, l := range tc.prog {
				storeLine(t, h, l)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v output=%q, want contains %q", tc.prog, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneInput proves standalone INPUT: COMPILE bundles the
// INPUT operand span (optional prompt + variable list) and delegates to the bundled
// exec_do_input, which prints the prompt, reads a line per variable via the bundled
// read_line, and stores through var_lookup, all with no resident interpreter.
func TestREPL_Compile_StandaloneInput(t *testing.T) {
	asmBin := buildAssembler(t)
	cases := []struct {
		name  string
		prog  []string
		input string
		want  string
	}{
		{"numeric", []string{`10 INPUT A`, `20 PRINT A*2`}, "5\n", "10"},
		{"prompt", []string{`10 INPUT "N"; A`, `20 PRINT A+1`}, "7\n", "N8"},
		{"string", []string{`10 INPUT A$`, `20 PRINT A$`}, "HI\n", "HI"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
			for _, l := range tc.prog {
				storeLine(t, h, l)
			}
			out := h.runCommand(`COMPILE "demo"`)
			if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
				t.Fatalf("COMPILE failed: %q", out)
			}
			img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
			if err != nil {
				t.Fatalf("demo.ie64 not written: %v", err)
			}
			run := newEhbasicHarness(t)
			run.loadBytes(img)
			run.sendInput(tc.input) // pre-queue; read_line polls TERM_STATUS for it
			run.runCycles(8_000_000)
			got := run.terminal.DrainOutput()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("standalone %v (input %q) output=%q, want contains %q", tc.prog, tc.input, got, tc.want)
			}
		})
	}
}

// TestREPL_Compile_StandaloneList proves standalone LIST: COMPILE bundles the
// tokenised programme (AOT_RT_PROG) and the bootstrap sets state[ST_PROG_START/END]
// at it; the bundled exec_do_list -> line_list -> detokenize walks and prints the
// listing, with no resident interpreter.
func TestREPL_Compile_StandaloneList(t *testing.T) {
	asmBin := buildAssembler(t)
	tmpDir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	for _, l := range []string{`10 PRINT "X"`, `20 LIST`} {
		storeLine(t, h, l)
	}
	out := h.runCommand(`COMPILE "demo"`)
	if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
	if err != nil {
		t.Fatalf("demo.ie64 not written: %v", err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(img)
	run.runCycles(8_000_000)
	got := run.terminal.DrainOutput()
	// PRINT "X" runs, then LIST detokenises the bundled programme.
	for _, want := range []string{"X\r\n", "10 ", `PRINT "X"`, "20 ", "LIST"} {
		if !strings.Contains(got, want) {
			t.Fatalf("standalone LIST output=%q, want contains %q", got, want)
		}
	}
}

// TestREPL_Examples_ThreeMode is the regression guard required by the AOT plan: the
// four shipped BASIC examples must behave consistently across interpreted RUN,
// RUN AOT, and standalone COMPILE. The demos end in an infinite hardware render loop
// (BLIT/COPPER/MIDI), so they cannot be run to completion headless; this test instead
// pins each mode's classification, which is where the AOT pipeline's regression risk
// lives:
//
//   - Interpreted front-end: LOAD + LIST must round-trip the tokenised programme.
//   - RUN AOT: the arena compiles every example through resident delegation, so the
//     compile phase must not report ?COMPILE ERROR. (Bounded by the harness deadline
//     because of the render loop; skipped under -short.)
//   - Standalone COMPILE: all four are rejected because they use a construct with no
//     standalone (self-contained) lowering yet. resonance hits POKE with expression
//     operands (the arena delegates those to the resident handler; standalone has no
//     resident handler); the other three use BLIT COPY/MEMCOPY/MODE. BLOAD, by
//     contrast, IS lowered standalone (File I/O MMIO) and no longer blocks them.
//
// If a future change adds standalone lowering for those constructs, the
// standaloneCompiles expectation below flags the affected example so this matrix
// stays honest.
func TestREPL_Examples_ThreeMode(t *testing.T) {
	asmBin := buildAssembler(t)
	repo := repoRootDir(t)
	cases := []struct {
		name               string
		standaloneCompiles bool
	}{
		{"resonance", false},
		{"rotozoomer_basic", false},
		{"splash_wobble", false},
		{"wobble_zoom", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(repo, "sdk", "examples", "basic", tc.name+".bas"))
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.name+".bas"), src, 0644); err != nil {
				t.Fatal(err)
			}
			h := newEhbasicREPLHarnessWithFileIO(t, asmBin, dir)
			h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)

			// Interpreted front-end: LOAD tokenises the programme, LIST detokenises it.
			if out := h.runCommand(`LOAD "` + tc.name + `.bas"`); strings.Contains(out, "ERROR") {
				t.Fatalf("interpreted LOAD failed: %q", out)
			}
			if list := h.runCommand("LIST"); !strings.Contains(list, "REM") {
				t.Fatalf("LIST did not round-trip the loaded programme")
			}

			// Standalone COMPILE: pin the documented classification.
			comp := h.runCommand(`COMPILE "out"`)
			_, statErr := os.Stat(filepath.Join(dir, "out.ie64"))
			if tc.standaloneCompiles {
				if strings.Contains(comp, "ERROR") || strings.Contains(comp, aotStubMarker) {
					t.Fatalf("standalone COMPILE of %s should succeed, got: %q", tc.name, comp)
				}
				if statErr != nil {
					t.Fatalf("standalone COMPILE of %s wrote no image: %v", tc.name, statErr)
				}
			} else {
				if !strings.Contains(comp, aotStubMarker) {
					t.Fatalf("standalone COMPILE of %s should be rejected (no standalone BLIT subcommand lowering), got: %q", tc.name, comp)
				}
				if statErr == nil {
					t.Fatalf("standalone COMPILE of %s wrote an image despite an unsupported statement", tc.name)
				}
			}

			// RUN AOT: every example must reach native code (no compile error); the
			// arena delegates the hardware statements to their resident handlers. The
			// demo then enters its render loop and runs to the harness deadline, so this
			// portion is skipped under -short.
			if testing.Short() {
				t.Skip("RUN AOT runs the demo render loop to the harness deadline; skipped in -short")
			}
			ra := h.runCommand("RUN AOT")
			if strings.Contains(ra, "?COMPILE ERROR") || strings.Contains(ra, aotStubMarker) {
				t.Fatalf("RUN AOT should compile %s via delegation, got compile error: %q", tc.name, ra)
			}
		})
	}
}

// TestREPL_Compile_StandaloneSave proves standalone SAVE: the bundled exec_do_save
// detokenises the bundled programme to FILE_DATA_BUF and writes it over the File I/O
// ABI. The compiled image is run on a machine with File I/O mapped to a scratch dir;
// the written file must contain the detokenised source.
func TestREPL_Compile_StandaloneSave(t *testing.T) {
	asmBin := buildAssembler(t)
	cdir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, cdir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	for _, l := range []string{`10 PRINT "HI"`, `20 SAVE "out.bas"`} {
		storeLine(t, h, l)
	}
	out := h.runCommand(`COMPILE "demo"`)
	if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	img, err := os.ReadFile(filepath.Join(cdir, "demo.ie64"))
	if err != nil {
		t.Fatalf("demo.ie64 not written: %v", err)
	}
	// Run standalone with File I/O mapped to a fresh dir, so SAVE has somewhere to write.
	rdir := t.TempDir()
	run := newEhbasicHarness(t)
	fio := NewFileIODevice(run.bus, rdir)
	run.bus.MapIO(FILE_IO_BASE, FILE_IO_END, fio.HandleRead, fio.HandleWrite)
	run.bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fio.HandleWrite8)
	run.loadBytes(img)
	run.runCycles(8_000_000)
	saved, err := os.ReadFile(filepath.Join(rdir, "out.bas"))
	if err != nil {
		t.Fatalf("SAVE did not write out.bas: %v", err)
	}
	for _, want := range []string{"10 ", `PRINT "HI"`, "20 ", "SAVE"} {
		if !strings.Contains(string(saved), want) {
			t.Fatalf("saved file=%q, want contains %q", string(saved), want)
		}
	}
}

// TestREPL_Compile_StandaloneRejectsLoad: LOAD reconstructs a tokenised programme,
// which needs the resident tokeniser and a REPL loop to execute it; both are absent
// in a standalone image, so COMPILE rejects LOAD (reaches the backend stub) rather
// than emitting a broken binary. BLOAD is a raw binary load and IS supported (see
// TestREPL_Compile_StandaloneBload).
func TestREPL_Compile_StandaloneRejectsLoad(t *testing.T) {
	asmBin := buildAssembler(t)
	for _, prog := range []string{`10 LOAD "x.bas"`} {
		tmpDir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		storeLine(t, h, prog)
		out := h.runCommand(`COMPILE "demo"`)
		if !strings.Contains(out, aotStubMarker) {
			t.Fatalf("COMPILE of %q must report unsupported, got: %q", prog, out)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "demo.ie64")); err == nil {
			t.Fatalf("COMPILE wrote a binary for unsupported %q", prog)
		}
	}
}

// TestREPL_Compile_StandaloneBload proves standalone BLOAD: COMPILE bundles the
// BLOAD operand span (filename string, comma, destination expression) and calls the
// bundled exec_do_bload, which loads raw bytes to the destination through the File
// I/O MMIO (FILE_NAME_PTR / FILE_DATA_PTR / FILE_CTRL = OP_READ) - the same path the
// interpreter uses. The compiled image is run on a machine with File I/O mapped to a
// scratch dir holding the source file; the loaded bytes must appear at the address.
func TestREPL_Compile_StandaloneBload(t *testing.T) {
	asmBin := buildAssembler(t)
	cdir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, cdir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	const dst = 0x710000 // scratch RAM, clear of code/blob/prog/vars/stack
	for _, l := range []string{
		`10 BLOAD "blob.bin", &H710000`,
		`20 PRINT "DONE"`,
	} {
		storeLine(t, h, l)
	}
	out := h.runCommand(`COMPILE "demo"`)
	if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE of standalone BLOAD failed: %q", out)
	}
	img, err := os.ReadFile(filepath.Join(cdir, "demo.ie64"))
	if err != nil {
		t.Fatalf("demo.ie64 not written: %v", err)
	}
	// Run standalone with File I/O mapped to a dir holding the binary to load.
	rdir := t.TempDir()
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x11, 0x22, 0x33, 0x44}
	if err := os.WriteFile(filepath.Join(rdir, "blob.bin"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	run := newEhbasicHarness(t)
	fio := NewFileIODevice(run.bus, rdir)
	run.bus.MapIO(FILE_IO_BASE, FILE_IO_END, fio.HandleRead, fio.HandleWrite)
	run.bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fio.HandleWrite8)
	run.loadBytes(img)
	run.runCycles(8_000_000)
	got := run.terminal.DrainOutput()
	if !strings.Contains(got, "DONE") {
		t.Fatalf("standalone BLOAD programme did not finish: %q", got)
	}
	for i, b := range payload {
		if run.cpu.memory[dst+i] != b {
			t.Fatalf("BLOAD byte %d: memory[%#x]=%#02x, want %#02x", i, dst+i, run.cpu.memory[dst+i], b)
		}
	}
}

// TestREPL_Compile_StandaloneUSR proves the supported indirect-call policy: a
// standalone programme calling USR(addr) jumps into user machine code (jsr (r9) inside
// the bundled expr_eval) and returns the routine's R8 as the expression value. A tiny
// ML stub (move.q r8,#42 / rts) is assembled and written into the run machine's RAM at
// a scratch address; the compiled programme calls USR on that address and prints 42.
func TestREPL_Compile_StandaloneUSR(t *testing.T) {
	asmBin := buildAssembler(t)
	const stubAddr = 0x70000 // scratch RAM, clear of code/blob/prog/vars/stack

	// Assemble the ML stub at its run address and slice out the emitted bytes (the
	// flat image is based at PROGRAM_START 0x1000, so guest stubAddr is at file offset
	// stubAddr-0x1000).
	stubSrc := "include \"ie64.inc\"\n    org " + fmt.Sprintf("%#x", stubAddr) + "\nstub:\n    move.q r8, #42\n    rts\n"
	incDir := filepath.Join(repoRootDir(t), "sdk", "include")
	stubDir := t.TempDir()
	stubAsm := filepath.Join(stubDir, "stub.asm")
	if err := os.WriteFile(stubAsm, []byte(stubSrc), 0644); err != nil {
		t.Fatal(err)
	}
	stubOut := filepath.Join(stubDir, "stub.ie64")
	if out, err := exec.Command(asmBin, "-I", incDir, "-o", stubOut, stubAsm).CombinedOutput(); err != nil {
		t.Fatalf("assemble USR stub: %v\n%s", err, out)
	}
	stubImg, err := os.ReadFile(stubOut)
	if err != nil {
		t.Fatal(err)
	}
	const progStart = 0x1000
	stubBytes := stubImg[stubAddr-progStart : stubAddr-progStart+16] // move.q(8) + rts(8)

	// Compile a programme that calls USR on the stub address and prints the result.
	cdir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, cdir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	storeLine(t, h, fmt.Sprintf("10 X=USR(%d)", stubAddr))
	storeLine(t, h, "20 PRINT X")
	out := h.runCommand(`COMPILE "demo"`)
	if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	img, err := os.ReadFile(filepath.Join(cdir, "demo.ie64"))
	if err != nil {
		t.Fatalf("demo.ie64 not written: %v", err)
	}

	run := newEhbasicHarness(t)
	run.loadBytes(img)
	copy(run.cpu.memory[stubAddr:stubAddr+len(stubBytes)], stubBytes) // install the ML stub
	run.runCycles(8_000_000)
	got := run.terminal.DrainOutput()
	if !strings.Contains(got, "42") {
		t.Fatalf("standalone USR output=%q, want it to contain 42 (the stub's R8)", got)
	}
}

// TestREPL_Compile_StandaloneErrorHalts proves the standalone error policy: when a
// bundled runtime call raises an error (the bundled raise_error prints the message and
// sets state[ST_ERROR_FLAG]), the generated code halts instead of running on into
// corrupt state the way the interpreter's exec loop would otherwise prevent. The error
// line must match the offending statement (ST_CURRENT_LINE is tracked per line).
func TestREPL_Compile_StandaloneErrorHalts(t *testing.T) {
	asmBin := buildAssembler(t)
	tmpDir := t.TempDir()
	h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	// Line 20 indexes a DIM A(2) array out of range -> ?FC ERROR; line 30 must NOT run.
	for _, l := range []string{`10 DIM A(2)`, `20 A(99)=1`, `30 PRINT "AFTER"`} {
		storeLine(t, h, l)
	}
	out := h.runCommand(`COMPILE "demo"`)
	if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
		t.Fatalf("COMPILE failed: %q", out)
	}
	img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
	if err != nil {
		t.Fatalf("demo.ie64 not written: %v", err)
	}
	run := newEhbasicHarness(t)
	run.loadBytes(img)
	run.runCycles(8_000_000)
	got := run.terminal.DrainOutput()
	if !strings.Contains(got, "ERROR IN 20") {
		t.Fatalf("standalone error output=%q, want %q (accurate error line)", got, "ERROR IN 20")
	}
	if strings.Contains(got, "AFTER") {
		t.Fatalf("standalone ran a statement after an error (output=%q): the error must halt", got)
	}
}

// TestREPL_Compile_StandaloneOutOfMemory exercises the blob+programme layout's
// OUT OF MEMORY boundary: a programme whose generated code plus the bundled runtime
// blob and tokenised programme would overflow the compile buffers must report
// ?OUT OF MEMORY and write NO (truncated/corrupt) binary, while a programme below the
// bound compiles to a runnable image. This guards the resized-buffer / placement bounds
// the runtime bundle introduced.
func TestREPL_Compile_StandaloneOutOfMemory(t *testing.T) {
	asmBin := buildAssembler(t)
	bigLine := strings.TrimSuffix(strings.Repeat("A=A+1:", 30), ":") // ~30 delegated LETs

	// Below the bound: compiles, writes a binary, and the image actually runs.
	t.Run("fits", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		for i := 0; i < 10; i++ {
			storeLine(t, h, fmt.Sprintf("%d %s", 10+i, bigLine))
		}
		storeLine(t, h, "200 PRINT A")
		out := h.runCommand(`COMPILE "demo"`)
		if strings.Contains(out, aotStubMarker) || strings.Contains(out, "ERROR") {
			t.Fatalf("COMPILE below the bound failed: %q", out)
		}
		img, err := os.ReadFile(filepath.Join(tmpDir, "demo.ie64"))
		if err != nil {
			t.Fatalf("demo.ie64 not written: %v", err)
		}
		run := newEhbasicHarness(t)
		run.loadBytes(img)
		run.runCycles(8_000_000)
		if got := run.terminal.DrainOutput(); !strings.Contains(got, "300") {
			t.Fatalf("below-bound image output=%q, want 300 (10 lines * 30 increments)", got)
		}
	})

	// Over the bound: must report OUT OF MEMORY and write no binary.
	t.Run("overflows", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := newEhbasicREPLHarnessWithFileIO(t, asmBin, tmpDir)
		h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
		for i := 0; i < 24; i++ {
			storeLine(t, h, fmt.Sprintf("%d %s", 10+i, bigLine))
		}
		out := h.runCommand(`COMPILE "demo"`)
		if !strings.Contains(out, "OUT OF MEMORY") {
			t.Fatalf("COMPILE over the bound must report OUT OF MEMORY, got: %q", out)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "demo.ie64")); err == nil {
			t.Fatalf("COMPILE wrote a (truncated) binary on OUT OF MEMORY")
		}
	})
}

// A stored programme with a statement the transpiler cannot yet lower reaches
// the backend and reports the not-yet-implemented stub (proving validation
// passed). Empty/END programmes are now actually compiled, so an unsupported
// statement is used here to keep exercising the "reached backend" path.
func TestREPL_Compile_AllowsTrailingSpaces(t *testing.T) {
	h, _ := startREPL(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	storeLine(t, h, "10 GET A") // GET is not lowerable standalone -> stub after validation
	output := h.runCommand(`COMPILE "demo"   `)
	if !strings.Contains(output, aotStubMarker) {
		t.Fatalf(`COMPILE "demo" + trailing spaces should reach backend, got: %q`, output)
	}
}

func TestREPL_Compile_ValidNameReachesBackend(t *testing.T) {
	h, _ := startREPL(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	storeLine(t, h, "10 GET A") // GET is not lowerable standalone -> stub after validation
	output := h.runCommand(`COMPILE "demo"`)
	if !strings.Contains(output, aotStubMarker) {
		t.Fatalf(`COMPILE "demo": expected to pass validation and reach backend, got: %q`, output)
	}
	if strings.Contains(output, "FC ERROR") {
		t.Fatalf(`COMPILE "demo": valid name should not raise ?FC ERROR, got: %q`, output)
	}
}

func TestREPL_Compile_ValidNameWithExtensionReachesBackend(t *testing.T) {
	h, _ := startREPL(t)
	h.bus.ApplyProfileVisibleCeiling(aotTestGuestRAM)
	storeLine(t, h, "10 GET A") // GET is not lowerable standalone -> stub after validation
	// Mixed-case .IE64 already present must be accepted unchanged.
	output := h.runCommand(`COMPILE "DEMO.IE64"`)
	if !strings.Contains(output, aotStubMarker) {
		t.Fatalf(`COMPILE "DEMO.IE64": expected to reach backend, got: %q`, output)
	}
}

// storeLine stores a single numbered programme line and waits for the prompt.
// pump runs the CPU for a bounded wall-clock window, then stops it cleanly.
// Unlike runCycles it does not fail on timeout, so it suits the never-halting
// REPL: the CPU is left stopped and resumable.
func (h *ehbasicTestHarness) pump(d time.Duration) {
	h.cpu.running.Store(true)
	done := make(chan struct{})
	go func() {
		h.execCPU()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		h.cpu.running.Store(false)
		h.waitDone(done)
	}
}

// pumpUntil runs the CPU until cond() holds (polled), the CPU exits, or max
// elapses, then stops it cleanly. Lets tests wait on guest progress instead of
// a fixed sleep.
func (h *ehbasicTestHarness) pumpUntil(cond func() bool, max time.Duration) {
	h.cpu.running.Store(true)
	done := make(chan struct{})
	go func() {
		h.execCPU()
		close(done)
	}()
	deadline := time.After(max)
	tick := time.NewTicker(1 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-deadline:
			h.cpu.running.Store(false)
			h.waitDone(done)
			return
		case <-tick.C:
			if cond() {
				h.cpu.running.Store(false)
				h.waitDone(done)
				return
			}
		}
	}
}

func storeLine(t *testing.T, h *ehbasicTestHarness, line string) {
	t.Helper()
	h.sendInput(line + "\n")
	// Numbered-line entry returns to repl_read without printing "Ready", so
	// runUntilPrompt() would block its full deadline. Instead wait for the REPL
	// to drain the submitted line (input queue empties as read_line consumes
	// it), then a brief tail to let it tokenise and store. Condition-driven, so
	// each call costs a few ms rather than a fixed 200ms.
	h.pumpUntil(func() bool { return h.terminal.InputPending() == 0 }, 2*time.Second)
	h.pump(10 * time.Millisecond)
}

// Direct-only / non-compilable raw roots must be rejected with the canonical
// ?COMPILE ERROR IN <line>: <reason>. Exercised through RUN AOT; COMPILE shares
// the same aot_compile_check precheck, so it rejects identically.
func TestREPL_AOT_RejectsDirectOnlyRoots(t *testing.T) {
	cases := []struct {
		name string
		prog string
		want string
	}{
		{"dir", "10 DIR", "?COMPILE ERROR IN 10: DIR is direct-only"},
		{"host", "10 HOST", "?COMPILE ERROR IN 10: HOST cannot be compiled"},
		{"costart", `10 COSTART 2,"svc"`, "?COMPILE ERROR IN 10: COSTART cannot be compiled"},
		{"costop", "10 COSTOP 2", "?COMPILE ERROR IN 10: COSTOP cannot be compiled"},
		{"cowait", "10 COWAIT 1", "?COMPILE ERROR IN 10: COWAIT cannot be compiled"},
		{"cocall", "10 COCALL(2,0,0,0,0,0)", "?COMPILE ERROR IN 10: COCALL cannot be compiled"},
		{"costatus", "10 COSTATUS(1)", "?COMPILE ERROR IN 10: COSTATUS cannot be compiled"},
		{"compile", `10 COMPILE "x"`, "?COMPILE ERROR IN 10: COMPILE is direct-only"},
		{"type", `10 TYPE "readme.txt"`, "?COMPILE ERROR IN 10: TYPE is direct-only"},
		{"runaot", "10 RUN AOT", "?COMPILE ERROR IN 10: RUN AOT is direct-only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, tc.prog)
			output := h.runCommand("RUN AOT")
			if !strings.Contains(output, tc.want) {
				t.Fatalf("RUN AOT on %q: want %q, got: %q", tc.prog, tc.want, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: rejected root must not reach backend, got: %q", tc.prog, output)
			}
		})
	}
}

// Tokenised hardware roots with raw subverbs (SOUND PLAY, SID PLAY, PSG PLAY,
// and a graphics subcommand) are NOT direct-only, so RUN AOT must accept them
// (reach the backend / delegate), never report the raw-root rejection.
func TestREPL_AOT_AcceptsRawSubverbRoots(t *testing.T) {
	cases := []struct {
		name string
		prog string
	}{
		{"sound_play", `10 SOUND PLAY "music.mid"`},
		{"sid_play", "10 SID PLAY &HC000,4096,0"},
		{"psg_play", "10 PSG PLAY &H8000,2048"},
		{"blit_fill", "10 BLIT FILL &H100000,16,16,0,64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, tc.prog)
			output := h.runCommand("RUN AOT")
			if strings.Contains(output, "cannot be compiled") ||
				strings.Contains(output, "is direct-only") ||
				strings.Contains(output, "?COMPILE ERROR") {
				t.Fatalf("RUN AOT on %q: raw-subverb root must not be rejected, got: %q", tc.prog, output)
			}
		})
	}
}

// A stored "RUN AOT" with a punctuation tail is still the direct-only RUN AOT
// form and must be rejected by the compile scan, not treated as plain RUN.
func TestREPL_AOT_RejectsStoredRunAotWithPunctuation(t *testing.T) {
	progs := []string{
		"10 RUN AOT,1",
		"10 RUN AOT=1",
		`10 RUN AOT"file"`,
		"10 RUN AOT;",
	}
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, prog)
			output := h.runCommand("RUN AOT")
			if !strings.Contains(output, "?COMPILE ERROR IN 10: RUN AOT is direct-only") {
				t.Fatalf("RUN AOT on %q: want RUN AOT direct-only error, got: %q", prog, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: must not reach backend, got: %q", prog, output)
			}
		})
	}
}

// A direct-only root anywhere on the line (after a colon) must also be caught.
func TestREPL_AOT_RejectsDirectOnlyRootAfterColon(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 X=1:DIR")
	output := h.runCommand("RUN AOT")
	if !strings.Contains(output, "?COMPILE ERROR IN 10: DIR is direct-only") {
		t.Fatalf("RUN AOT on multi-statement line: got: %q", output)
	}
}

// Direct-only roots inside IF THEN/ELSE clauses must be reclassified and
// rejected; the interpreter executes those tails as statements.
func TestREPL_AOT_RejectsDirectOnlyRootInIfClause(t *testing.T) {
	cases := []struct {
		name string
		prog string
		want string
	}{
		{"then", "10 IF 1 THEN DIR", "?COMPILE ERROR IN 10: DIR is direct-only"},
		{"else", "10 IF 0 THEN PRINT 1 ELSE HOST", "?COMPILE ERROR IN 10: HOST cannot be compiled"},
		{"then-host", "10 IF 1 THEN HOST ELSE PRINT 1", "?COMPILE ERROR IN 10: HOST cannot be compiled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, tc.prog)
			output := h.runCommand("RUN AOT")
			if !strings.Contains(output, tc.want) {
				t.Fatalf("RUN AOT on %q: want %q, got: %q", tc.prog, tc.want, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: rejected IF clause must not reach backend, got: %q", tc.prog, output)
			}
		})
	}
}

// COCALL/COSTATUS are non-compilable even in their normal expression form,
// where the statement root is an assignment or PRINT rather than the function.
func TestREPL_AOT_RejectsCoprocFunctionsInExpressions(t *testing.T) {
	cases := []struct {
		name string
		prog string
		want string
	}{
		{"assign-cocall", "10 X=COCALL(2,0,0,0,0,0)", "?COMPILE ERROR IN 10: COCALL cannot be compiled"},
		{"print-costatus", "10 PRINT COSTATUS(1)", "?COMPILE ERROR IN 10: COSTATUS cannot be compiled"},
		{"nested-cocall", "10 Y=1+COCALL(2,0,0,0,0,0)*3", "?COMPILE ERROR IN 10: COCALL cannot be compiled"},
		{"after-colon", "10 X=1:Z=COSTATUS(1)", "?COMPILE ERROR IN 10: COSTATUS cannot be compiled"},
		{"in-if", "10 IF COSTATUS(1) THEN PRINT 1", "?COMPILE ERROR IN 10: COSTATUS cannot be compiled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, tc.prog)
			output := h.runCommand("RUN AOT")
			if !strings.Contains(output, tc.want) {
				t.Fatalf("RUN AOT on %q: want %q, got: %q", tc.prog, tc.want, output)
			}
			if strings.Contains(output, aotStubMarker) {
				t.Fatalf("%q: banned function must not reach backend, got: %q", tc.prog, output)
			}
		})
	}
}

// A banned function name inside a string literal must not be misflagged, and a
// variable sharing only a prefix (COCALLER) must not match.
func TestREPL_AOT_AcceptsCoprocLookalikes(t *testing.T) {
	progs := []string{
		`10 PRINT "COCALL"`, // inside a string literal
		"10 COCALLER=5",     // variable that merely starts with COCALL
	}
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, prog)
			output := h.runCommand("RUN AOT")
			// The front-end must not reject these (they reach the backend, which
			// either lowers them or reports the unsupported stub - never a reasoned
			// compile error).
			assertNotRejected(t, prog, output)
		})
	}
}

// DATA payloads are literal data, not expressions: a value that looks like a
// coprocessor call must not be flagged.
func TestREPL_AOT_AcceptsDataPayloads(t *testing.T) {
	progs := []string{
		"10 DATA COCALL(1)",
		"10 DATA 1,2,3",
		"10 DATA COSTATUS(1),HOST,DIR",
		"10 X=1:DATA COCALL(9):Y=2",
	}
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, prog)
			output := h.runCommand("RUN AOT")
			assertNotRejected(t, prog, output) // accepted: lowers or stubs, never a scan rejection
			if strings.Contains(output, "cannot be compiled") || strings.Contains(output, "direct-only") {
				t.Fatalf("RUN AOT on %q: DATA payload should not be rejected, got: %q", prog, output)
			}
		})
	}
}

// COCALL/COSTATUS used as plain identifiers (no following '(') are ordinary
// variables and must be accepted, matching the interpreter.
func TestREPL_AOT_AcceptsCoprocNamesAsVariables(t *testing.T) {
	progs := []string{
		"10 X=COCALL+1",
		"10 COCALL=5",
		"10 PRINT COSTATUS",
		"10 COSTATUS=COCALL*2",
	}
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, prog)
			output := h.runCommand("RUN AOT")
			assertNotRejected(t, prog, output) // accepted: lowers or stubs, never a scan rejection
			if strings.Contains(output, "cannot be compiled") {
				t.Fatalf("RUN AOT on %q: variable should not be rejected, got: %q", prog, output)
			}
		})
	}
}

// The space-less "RUN AOT:tail" form must be claimed as RUN AOT and rejected
// for its trailing argument, not fall through to the plain interpreted RUN.
func TestREPL_RunAOT_RejectsColonTail(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 PRINT 99")
	output := h.runCommand("RUN AOT:PRINT 1")
	if !strings.Contains(output, "SYNTAX ERROR") {
		t.Fatalf("RUN AOT:tail: expected syntax error, got: %q", output)
	}
	if strings.Contains(output, "Compiling to native code...") || strings.Contains(output, aotStubMarker) {
		t.Fatalf("RUN AOT:tail must not compile, got: %q", output)
	}
	// Must not have run the stored program interpreted.
	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimRight(line, "\r") == "99" {
			t.Fatalf("RUN AOT:tail must not run the stored program, got: %q", output)
		}
	}
}

// DIR and COMPILE are not intercepted by the interpreter, so a stored program
// may use them as ordinary variables/arrays; the AOT scan must accept those.
func TestREPL_AOT_AcceptsDirCompileAsVariables(t *testing.T) {
	progs := []string{
		"10 DIR=1:PRINT DIR",
		"10 COMPILE=1",
		"10 DIR(2)=5",
		"10 COMPILE(0)=9",
		"10 DIR = 7", // space before '='
		"10 TYPE=3:PRINT TYPE",
		"10 TYPE(1)=8",
	}
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, prog)
			output := h.runCommand("RUN AOT")
			assertNotRejected(t, prog, output) // accepted: lowers or stubs, never a scan rejection
			if strings.Contains(output, "direct-only") {
				t.Fatalf("RUN AOT on %q: variable should not be rejected, got: %q", prog, output)
			}
		})
	}
}

// HOST/COSTART/COSTOP/COWAIT are intercepted as commands by the interpreter
// even in assignment form, so they remain rejected (no variable exemption).
func TestREPL_AOT_StillRejectsInterceptedRootsInAssignForm(t *testing.T) {
	cases := []struct {
		prog string
		want string
	}{
		{"10 HOST=1", "?COMPILE ERROR IN 10: HOST cannot be compiled"},
		{"10 COSTART=1", "?COMPILE ERROR IN 10: COSTART cannot be compiled"},
	}
	for _, tc := range cases {
		t.Run(tc.prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, tc.prog)
			output := h.runCommand("RUN AOT")
			if !strings.Contains(output, tc.want) {
				t.Fatalf("RUN AOT on %q: want %q, got: %q", tc.prog, tc.want, output)
			}
		})
	}
}

// A compilable IF...THEN...ELSE is lowered natively (no stub, no rejection) and
// runs the taken arm.
func TestREPL_AOT_AcceptsPlainIf(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 IF 1 THEN PRINT 1 ELSE PRINT 2")
	output := h.runCommand("RUN AOT")
	if strings.Contains(output, aotStubMarker) {
		t.Fatalf("IF...ELSE should compile natively, got stub: %q", output)
	}
	if strings.Contains(output, "direct-only") || strings.Contains(output, "cannot be compiled") {
		t.Fatalf("plain IF should not be rejected, got: %q", output)
	}
	// Inspect only the post-compile output (the echoed program line contains "2").
	if i := strings.Index(output, "native code"); i >= 0 {
		tail := output[i:]
		if !strings.Contains(tail, "1") || strings.Contains(tail, "2") {
			t.Fatalf("IF 1 THEN PRINT 1 ELSE PRINT 2 should print 1, got: %q", output)
		}
	}
}

// COMPILE failures print only the reasoned error, with no compile banner.
func TestREPL_Compile_RejectsDirectOnlyRootNoBanner(t *testing.T) {
	h, _ := startREPL(t)
	storeLine(t, h, "10 HOST")
	output := h.runCommand(`COMPILE "demo"`)
	if !strings.Contains(output, "?COMPILE ERROR IN 10: HOST cannot be compiled") {
		t.Fatalf(`COMPILE with HOST: want reasoned error, got: %q`, output)
	}
	if strings.Contains(output, "Compiling to native code...") {
		t.Fatalf("COMPILE must not print the compile banner, got: %q", output)
	}
}

// Compilable roots - including tokenised hardware roots with raw subverbs and
// variables that merely share a prefix with a rejected keyword - must pass the
// scan and reach the backend.
func TestREPL_AOT_AcceptsCompilableRoots(t *testing.T) {
	progs := []string{
		"10 PRINT 1",
		"10 SOUND 0,440,200",
		"10 DIRX=1",      // variable, not DIR
		"10 COST=5",      // variable, not COSTART/COSTOP
		`10 PRINT "DIR"`, // DIR inside a string literal, not a root
	}
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			h, _ := startREPL(t)
			storeLine(t, h, prog)
			output := h.runCommand("RUN AOT")
			// The scan accepted the root if it is not rejected with a reasoned
			// scan error; accepted programmes either lower or hit the stub.
			assertNotRejected(t, prog, output)
		})
	}
}

// assertNotRejected verifies the AOT front-end scan did not reject a programme
// with a reasoned compile error ("direct-only" / "cannot be compiled"). Accepted
// programmes either lower to native code or hit the unsupported stub - neither is
// a scan rejection.
func assertNotRejected(t *testing.T, prog, output string) {
	t.Helper()
	for _, bad := range []string{"direct-only", "cannot be compiled"} {
		if strings.Contains(output, bad) {
			t.Fatalf("RUN AOT on %q: should not be rejected (%q), got: %q", prog, bad, output)
		}
	}
}
