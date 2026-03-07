//go:build m68k_test

package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

const (
	ctMailbox     = 0x80000
	ctPassCount   = ctMailbox + 0
	ctFailCount   = ctMailbox + 4
	ctExpTotal    = ctMailbox + 8
	ctSuiteDone   = ctMailbox + 12
	ctFailLogPos  = ctMailbox + 16
	ctCaseLogPos  = ctMailbox + 20
	ctCaseLog     = ctMailbox + 64
	ctCaseLogSize = 4096
	ctFailLog     = ctCaseLog + ctCaseLogSize
	ctFailLogSize = 16384
	ctFailRecSize = 28
	ctCaseRecSize = 8
	maxCycles     = 200_000_000
)

const (
	ftRegSR    = 1
	ftRegOnly  = 2
	ftFP64     = 3
	ftMem32    = 4
	ftMultiReg = 5
	ftExcept   = 6
)

type failRecord struct {
	NamePtr   uint32
	InputPtr  uint32
	ExpectPtr uint32
	ActualD0  uint32
	ActualD1  uint32
	ActualD2  uint32
	FailType  uint32
}

func (r failRecord) FormatActual() string {
	switch r.FailType {
	case ftRegSR:
		return fmt.Sprintf("reg=$%08X sr=$%04X", r.ActualD0, r.ActualD1)
	case ftRegOnly:
		return fmt.Sprintf("reg=$%08X", r.ActualD0)
	case ftFP64:
		return fmt.Sprintf("fp0=$%08X%08X fpsr=$%08X", r.ActualD0, r.ActualD1, r.ActualD2)
	case ftMem32:
		return fmt.Sprintf("mem=$%08X", r.ActualD0)
	case ftMultiReg:
		return fmt.Sprintf("r0=$%08X r1=$%08X", r.ActualD0, r.ActualD1)
	case ftExcept:
		return fmt.Sprintf("trap_taken=%d vector=%d", r.ActualD0, r.ActualD1)
	default:
		return fmt.Sprintf("d0=$%08X d1=$%08X d2=$%08X type=%d", r.ActualD0, r.ActualD1, r.ActualD2, r.FailType)
	}
}

func readCString(cpu *M68KCPU, addr uint32) string {
	if addr == 0 {
		return "<nil>"
	}
	var buf []byte
	for {
		b := cpu.Read8(addr)
		if b == 0 {
			break
		}
		buf = append(buf, b)
		addr++
		if len(buf) > 256 {
			break
		}
	}
	return string(buf)
}

func readFailRecord(cpu *M68KCPU, addr uint32) failRecord {
	return failRecord{
		NamePtr:   cpu.Read32(addr),
		InputPtr:  cpu.Read32(addr + 4),
		ExpectPtr: cpu.Read32(addr + 8),
		ActualD0:  cpu.Read32(addr + 12),
		ActualD1:  cpu.Read32(addr + 16),
		ActualD2:  cpu.Read32(addr + 20),
		FailType:  cpu.Read32(addr + 24),
	}
}

// buildCPUTestBinary regenerates the test case includes and assembles
// the flat binary fresh every run, so the binary can never be stale.
func buildCPUTestBinary(t *testing.T) []byte {
	t.Helper()

	// Generate test case includes
	gen := exec.Command("go", "run", "./cmd/gen_m68k_cputest")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("gen_m68k_cputest failed: %v\n%s", err, out)
	}

	// Assemble flat binary
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
	if len(data) == 0 {
		t.Fatal("assembled binary is empty")
	}
	return data
}

func TestM68KCPUTestSuite(t *testing.T) {
	bin := buildCPUTestBinary(t)

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	// Load flat binary at M68K_ENTRY_POINT
	for i, b := range bin {
		cpu.Write8(M68K_ENTRY_POINT+uint32(i), b)
	}

	// Zero the mailbox region before execution
	for addr := uint32(ctMailbox); addr < ctMailbox+ctCaseLogSize+ctFailLogSize+64; addr += 4 {
		cpu.Write32(addr, 0)
	}

	// Vector table
	cpu.Write32(0, M68K_STACK_START)
	cpu.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)

	// CPU state
	cpu.PC = M68K_ENTRY_POINT
	cpu.SR = M68K_SR_S | 0x0700
	cpu.AddrRegs[7] = M68K_STACK_START
	cpu.SSP = M68K_STACK_START
	cpu.USP = M68K_STACK_START
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	cpu.running.Store(true)

	// Execute until ct_suite_done is set, cycle limit, or runaway detected
	cycles := 0
	stuckCount := 0
	lastCaseCount := uint32(0)
	for cycles = 0; cycles < maxCycles; cycles++ {
		if cpu.Read32(ctSuiteDone) != 0 {
			break
		}
		if !cpu.running.Load() {
			break
		}
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Every 1M cycles, check if progress is being made
		if cycles%1_000_000 == 0 && cycles > 0 {
			currentCases := cpu.Read32(ctPassCount) + cpu.Read32(ctFailCount)
			if currentCases == lastCaseCount {
				stuckCount++
				if stuckCount >= 3 {
					t.Logf("Suite stuck at %d cases for 3M cycles, stopping (PC=$%08X SP=$%08X)",
						currentCases, cpu.PC, cpu.AddrRegs[7])
					break
				}
			} else {
				stuckCount = 0
			}
			lastCaseCount = currentCases
		}
	}

	passes := cpu.Read32(ctPassCount)
	fails := cpu.Read32(ctFailCount)
	expected := cpu.Read32(ctExpTotal)

	completed := cpu.Read32(ctSuiteDone) != 0
	if completed {
		t.Logf("Suite complete in %d cycles: %d pass, %d fail, %d expected", cycles, passes, fails, expected)
	} else {
		t.Logf("Suite incomplete after %d cycles: %d pass, %d fail of %d expected (PC=$%08X SP=$%08X)",
			cycles, passes, fails, expected, cpu.PC, cpu.AddrRegs[7])
	}

	if passes+fails != expected && expected > 0 && completed {
		t.Errorf("Case count mismatch: pass(%d) + fail(%d) = %d, expected %d",
			passes, fails, passes+fails, expected)
	}

	// Parse case log and create subtests for completed cases
	caseLogBytes := cpu.Read32(ctCaseLogPos)
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
				t.Errorf("FAIL %s\n  input:    %s\n  expected: %s\n  actual:   %s",
					caseName, inputStr, expectStr, rec.FormatActual())
			})
		} else {
			t.Run(caseName, func(t *testing.T) {})
		}
	}

	if !completed {
		t.Errorf("Suite did not complete: %d/%d cases ran before CPU runaway", passes+fails, expected)
	}
}
