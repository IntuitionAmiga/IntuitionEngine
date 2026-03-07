//go:build musashi && m68k_test

package main

import (
	"os"
	"os/exec"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/musashi"
)

// writeMusashiBE32 writes a 32-bit big-endian value byte-by-byte into Musashi memory.
func writeMusashiBE32(cpu *musashi.CPU, addr, val uint32) {
	cpu.WriteByte(addr, byte(val>>24))
	cpu.WriteByte(addr+1, byte(val>>16))
	cpu.WriteByte(addr+2, byte(val>>8))
	cpu.WriteByte(addr+3, byte(val))
}

func TestMusashiFPUSmoke(t *testing.T) {
	cpu := musashi.New()
	cpu.Init()
	cpu.ClearMem()

	// fmove.l #42,fp0 = F2 3C 44 00 00 00 00 2A
	// fmove.l fp0,d0  = F2 00 64 00
	// bra.s *          = 60 FE
	prog := []byte{
		0xF2, 0x3C, 0x44, 0x00, 0x00, 0x00, 0x00, 0x2A,
		0xF2, 0x00, 0x64, 0x00,
		0x60, 0xFE,
	}
	for i, b := range prog {
		cpu.WriteByte(0x1000+uint32(i), b)
	}
	writeMusashiBE32(cpu, 0, 0x00090000)
	writeMusashiBE32(cpu, 4, 0x00001000)
	const haltAddr = 0x0F00
	cpu.WriteByte(haltAddr, 0x60)
	cpu.WriteByte(haltAddr+1, 0xFE)
	for v := uint32(2); v < 256; v++ {
		writeMusashiBE32(cpu, v*4, haltAddr)
	}
	cpu.Reset()
	cpu.SetReg(musashi.RegSR, 0x2700)

	cpu.Execute(1000)
	pc := cpu.GetReg(musashi.RegPC)
	d0 := cpu.GetReg(musashi.RegD0)
	if pc == haltAddr {
		t.Fatalf("FPU instruction caused exception (F-line trap), PC=$%08X D0=$%08X", pc, d0)
	}
	if d0 != 42 {
		t.Fatalf("Expected D0=42, got D0=%d (PC=$%08X)", d0, pc)
	}
	t.Logf("FPU smoke test passed: D0=%d, PC=$%08X", d0, pc)
}

func buildMusashiTestBinary(t *testing.T) []byte {
	t.Helper()
	// Generate with -musashi flag to skip unsupported FPU opcodes
	gen := exec.Command("go", "run", "./cmd/gen_m68k_cputest", "-musashi")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("gen_m68k_cputest -musashi failed: %v\n%s", err, out)
	}
	binPath := "sdk/cputest/cputest_suite.bin"
	asm := exec.Command("vasmm68k_mot",
		"-Fbin", "-m68020", "-m68881", "-devpac",
		"-I", "sdk/cputest/include",
		"-o", binPath,
		"sdk/cputest/cputest_suite_bare.asm")
	if out, err := asm.CombinedOutput(); err != nil {
		t.Fatalf("vasmm68k_mot failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("cannot read assembled binary: %v", err)
	}
	// Regenerate full set for Go CPU tests
	regen := exec.Command("go", "run", "./cmd/gen_m68k_cputest")
	if out, err := regen.CombinedOutput(); err != nil {
		t.Logf("Warning: regenerating full catalog failed: %v\n%s", err, out)
	}
	return data
}

func TestM68KCPUTestSuiteMusashi(t *testing.T) {
	bin := buildMusashiTestBinary(t)

	cpu := musashi.New()
	cpu.Init()
	cpu.ClearMem()

	// Load binary byte-by-byte at M68K_ENTRY_POINT (big-endian in Musashi memory)
	for i, b := range bin {
		cpu.WriteByte(M68K_ENTRY_POINT+uint32(i), b)
	}

	// Write a small "halt loop" at $0F00 (BRA.S $-2 = 0x60FE).
	// This catches unhandled exceptions so they don't jump through address 0
	// and corrupt the mailbox by executing vector table data as code.
	const haltAddr = 0x0F00
	cpu.WriteByte(haltAddr, 0x60)   // BRA.S
	cpu.WriteByte(haltAddr+1, 0xFE) // $-2 (self-loop)

	// Vector table: SSP at 0, PC at 4, all other vectors point to halt loop
	writeMusashiBE32(cpu, 0, M68K_STACK_START)
	writeMusashiBE32(cpu, 4, M68K_ENTRY_POINT)
	for vec := uint32(2); vec < 256; vec++ {
		writeMusashiBE32(cpu, vec*4, haltAddr)
	}

	// Reset reads SSP from 0 and PC from 4, initializes internal state
	cpu.Reset()

	// Set SR to supervisor + interrupts masked
	cpu.SetReg(musashi.RegSR, uint32(M68K_SR_S|0x0700))

	// Execute in chunks, polling ctSuiteDone
	totalCycles := 0
	for totalCycles < maxCycles {
		if cpu.Read32(ctSuiteDone) != 0 {
			break
		}
		totalCycles += cpu.Execute(100_000)
	}

	passes := cpu.Read32(ctPassCount)
	fails := cpu.Read32(ctFailCount)
	expected := cpu.Read32(ctExpTotal)

	completed := cpu.Read32(ctSuiteDone) != 0
	if completed {
		t.Logf("Musashi: suite complete in %d cycles: %d pass, %d fail, %d expected",
			totalCycles, passes, fails, expected)
	} else {
		t.Logf("Musashi: suite incomplete after %d cycles: %d pass, %d fail of %d expected (PC=$%08X)",
			totalCycles, passes, fails, expected, cpu.GetReg(musashi.RegPC))
	}

	// Parse case log — create subtests (cap to case log area size)
	caseLogBytes := cpu.Read32(ctCaseLogPos)
	if caseLogBytes > ctCaseLogSize {
		caseLogBytes = ctCaseLogSize
	}
	failIdx := uint32(0)
	for off := uint32(0); off < caseLogBytes; off += ctCaseRecSize {
		namePtr := cpu.Read32(ctCaseLog + off)
		result := cpu.Read32(ctCaseLog + off + 4)
		caseName := readCString(cpu, namePtr)

		if result == 0 {
			fidx := failIdx
			failIdx++
			t.Run(caseName, func(t *testing.T) {
				rec := readFailRecord(cpu, ctFailLog+fidx*ctFailRecSize)
				inputStr := readCString(cpu, rec.InputPtr)
				expectStr := readCString(cpu, rec.ExpectPtr)
				t.Errorf("ORACLE MISMATCH %s\n  input:    %s\n  expected: %s\n  actual:   %s",
					caseName, inputStr, expectStr, rec.FormatActual())
			})
		} else {
			t.Run(caseName, func(t *testing.T) {})
		}
	}

	if !completed {
		t.Errorf("Musashi suite did not complete: %d/%d cases ran", passes+fails, expected)
	}
	if fails > 0 {
		t.Errorf("Musashi oracle: %d cases mismatched — triage: bridge bug, config issue, or catalog error", fails)
	}
}
