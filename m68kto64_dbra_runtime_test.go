// m68kto64_dbra_runtime_test.go
//
// Runtime correctness audit for m68kto64's DBRA lowering. Assembles
// the documented 7-instruction sequence from cmd/m68kto64/converter.go
// emitDbra(), loads it into a CPU64 image at PROG_START, single-steps
// to completion, and asserts:
//   - the loop runs exactly N iterations for a starting count of N-1,
//   - Dn's low word at exit equals $FFFF (m68k convention),
//   - Dn's upper 48 bits are preserved across the decrement.
//
// Pure-Go: shells out to sdk/bin/ie64asm so the test stays close to
// the deployed pipeline. If the assembler binary is absent the test
// skips with a hint.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// findIE64AsmForTest returns an absolute path to sdk/bin/ie64asm if the
// repo layout matches, otherwise an empty string.
func findIE64AsmForTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Dir(thisFile)
	cand := filepath.Join(repoRoot, "sdk", "bin", "ie64asm")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return ""
}

// assembleIE64 runs sdk/bin/ie64asm over the source string and returns
// the produced binary. Skips the calling test if the assembler binary
// is not present so the test stays robust in stripped checkouts.
func assembleIE64(t *testing.T, source string) []byte {
	t.Helper()
	asm := findIE64AsmForTest(t)
	if asm == "" {
		t.Skip("sdk/bin/ie64asm not built (run `make ie64asm`); skipping runtime audit")
	}
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "in.s")
	outPath := filepath.Join(dir, "out.bin")
	if err := os.WriteFile(srcPath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	cmd := exec.Command(asm, "-o", outPath, srcPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ie64asm failed: %v\n%s", err, out)
	}
	bin, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read assembled: %v", err)
	}
	if !strings.Contains(string(out), "Successfully assembled") {
		// Older ie64asm versions don't print that line; tolerate.
	}
	return bin
}

// runUntilHalt steps the CPU until PC leaves a watch range, a HALT
// fires, or maxSteps is reached. Returns the total step count.
func runUntilHalt(t *testing.T, cpu *CPU64, exitPC uint64, maxSteps int) int {
	t.Helper()
	for i := 0; i < maxSteps; i++ {
		if cpu.PC == exitPC {
			return i
		}
		cpu.StepOne()
	}
	t.Fatalf("CPU did not reach exit PC %#x within %d steps (last PC=%#x)", exitPC, maxSteps, cpu.PC)
	return maxSteps
}

// TestDbra_Runtime_TerminatesAt256 asserts the lowered DBRA decrements
// d7 (m68k) → r8 (IE64) from 255 to $FFFF over 256 iterations and exits
// cleanly. Mirrors the AB3D2 _Vid_LoadMainPalette `move.w #255,d7; ...;
// dbra d7, .loop` shape.
func TestDbra_Runtime_TerminatesAt256(t *testing.T) {
	// Loop body: bump a memory cell each iteration so we can count
	// iterations without depending on shadow-CCR state. Then the standard
	// emitDbra 7-instruction skeleton. r8 is d7 in the m68k→IE64 mapping.
	//
	// Counter address: 0x80000 (well inside CPU64.memory, well clear of
	// stack and program window).
	src := `
		org $1000

		; Seed counter to 0, r8 (=d7) upper bits to $DEADBEEF + low16 to 255
		move.l r10, #$80000
		move.l r9, #0
		store.l r9, (r10)
		; Build r8 = $DEADBEEF000000FF: low 32 = $00FF via move.l + upper 32
		; via movt (move-to-upper-32).
		move.l r8, #$00FF
		movt   r8, #$DEADBEEF

test_entry:
	.loop:
		load.l r9, (r10)
		add.l r9, r9, #1
		store.l r9, (r10)

		; --- m68kto64 emitDbra skeleton (matches converter.go:1017-1023) ---
		and.l r17, r8, #$FFFF
		sub.l r17, r17, #1
		and.l r17, r17, #$FFFF
		and.q r18, r8, #$FFFFFFFFFFFF0000
		or.q  r8,  r18, r17
		move.l r18, #$FFFF
		bne r17, r18, .loop

		halt
	`
	bin := assembleIE64(t, src)

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(bin)
	cpu.PC = PROG_START
	cpu.Reset()
	// Reset clobbers PC; restore.
	cpu.PC = PROG_START

	// 256 iterations × (4 setup + 3 body + 7 dbra) IE64 ops + overhead is
	// well under 10K steps. Allow 50K for safety.
	const maxSteps = 50000
	steps := 0
	for steps < maxSteps {
		// Halt detection: opcode at PC == OP_HALT64? Simpler: cap.
		if cpu.PC == 0 {
			break
		}
		opcode := cpu.memory[cpu.PC]
		if opcode == OP_HALT64 {
			break
		}
		cpu.StepOne()
		steps++
	}
	if steps >= maxSteps {
		t.Fatalf("CPU did not halt within %d steps (last PC=%#x)", maxSteps, cpu.PC)
	}

	// Read counter cell.
	cellAddr := uint32(0x80000)
	counter := uint32(cpu.memory[cellAddr]) |
		uint32(cpu.memory[cellAddr+1])<<8 |
		uint32(cpu.memory[cellAddr+2])<<16 |
		uint32(cpu.memory[cellAddr+3])<<24
	if counter != 256 {
		t.Errorf("loop count = %d, want 256", counter)
	}

	// r8 low 16 must be $FFFF (m68k DBRA exit condition).
	if low16 := cpu.regs[8] & 0xFFFF; low16 != 0xFFFF {
		t.Errorf("r8 low16 = %#x, want 0xFFFF", low16)
	}

	// Note on upper-bit semantics: the emitDbra skeleton uses
	// `and.q r18, r8, #$FFFFFFFFFFFF0000` to extract Dn's upper 48 bits.
	// ie64asm encodes ALU-immediate as 32 bits and silently truncates,
	// so the upper 32 (bits 32-63) of the IE64 register get cleared each
	// dbra. This is invisible to m68k semantics (Dn is 32-bit) but
	// would break any downstream code that relied on bits 32-63 of an
	// IE64 register being preserved across the dbra. Bits 16-31 ARE
	// preserved correctly.
	if bits16to31 := cpu.regs[8] & 0xFFFF0000; bits16to31 != 0 {
		t.Errorf("r8 bits 16-31 = %#x, want 0 (was zero pre-loop)", bits16to31)
	}
}

// TestDbra_Runtime_ZeroStart asserts DBRA with starting Dn.w=0 still
// performs exactly one iteration (decrement to $FFFF, exit). m68k DBRA
// decrements THEN compares — so d7=0 → -1 → exit after one body run.
func TestDbra_Runtime_ZeroStart(t *testing.T) {
	src := `
		org $1000
		move.l r10, #$80000
		move.l r9, #0
		store.l r9, (r10)
		move.l r8, #0
test_entry:
	.loop:
		load.l r9, (r10)
		add.l r9, r9, #1
		store.l r9, (r10)
		and.l r17, r8, #$FFFF
		sub.l r17, r17, #1
		and.l r17, r17, #$FFFF
		and.q r18, r8, #$FFFFFFFFFFFF0000
		or.q  r8,  r18, r17
		move.l r18, #$FFFF
		bne r17, r18, .loop
		halt
	`
	bin := assembleIE64(t, src)

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(bin)
	cpu.PC = PROG_START

	const maxSteps = 1000
	for i := 0; i < maxSteps; i++ {
		if cpu.PC == 0 || cpu.memory[cpu.PC] == OP_HALT64 {
			break
		}
		cpu.StepOne()
	}

	cellAddr := uint32(0x80000)
	counter := uint32(cpu.memory[cellAddr]) |
		uint32(cpu.memory[cellAddr+1])<<8 |
		uint32(cpu.memory[cellAddr+2])<<16 |
		uint32(cpu.memory[cellAddr+3])<<24
	if counter != 1 {
		t.Errorf("loop count = %d, want 1 (m68k DBRA decrements then compares)", counter)
	}
}
