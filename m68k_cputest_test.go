//go:build m68k_test

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
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

type memReader interface {
	Read8(addr uint32) byte
	Read32(addr uint32) uint32
}

func readCString(m memReader, addr uint32) string {
	if addr == 0 {
		return "<nil>"
	}
	var buf []byte
	for {
		b := m.Read8(addr)
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

func readFailRecord(m memReader, addr uint32) failRecord {
	return failRecord{
		NamePtr:   m.Read32(addr),
		InputPtr:  m.Read32(addr + 4),
		ExpectPtr: m.Read32(addr + 8),
		ActualD0:  m.Read32(addr + 12),
		ActualD1:  m.Read32(addr + 16),
		ActualD2:  m.Read32(addr + 20),
		FailType:  m.Read32(addr + 24),
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

func runM68KCPUTestSuite(t *testing.T, useJIT bool) {
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
	cpu.m68kJitEnabled = useJIT

	// Execute until ct_suite_done is set, cycle limit, or runaway detected
	cycles := 0
	stuckCount := 0
	lastCaseCount := uint32(0)
	if useJIT {
		done := make(chan struct{})
		go func() {
			cpu.M68KExecuteJIT()
			close(done)
		}()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(30 * time.Second)
	loopJIT:
		for {
			select {
			case <-done:
				break loopJIT
			case <-timeout:
				cpu.running.Store(false)
				<-done
				t.Fatalf("JIT CPU test suite timed out (PC=$%08X SP=$%08X pass=%d fail=%d)",
					cpu.PC, cpu.AddrRegs[7], cpu.Read32(ctPassCount), cpu.Read32(ctFailCount))
			case <-ticker.C:
				cycles += 100_000
				if cpu.Read32(ctSuiteDone) != 0 {
					cpu.running.Store(false)
					<-done
					break loopJIT
				}
				if !cpu.running.Load() {
					break loopJIT
				}
				currentCases := cpu.Read32(ctPassCount) + cpu.Read32(ctFailCount)
				if currentCases == lastCaseCount {
					stuckCount++
					if stuckCount >= 300 {
						cpu.running.Store(false)
						<-done
						t.Logf("Suite stuck at %d cases in JIT mode (PC=$%08X SP=$%08X)",
							currentCases, cpu.PC, cpu.AddrRegs[7])
						break loopJIT
					}
				} else {
					stuckCount = 0
				}
				lastCaseCount = currentCases
				runtime.Gosched()
			}
		}
	} else {
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

func TestM68KCPUTestSuite(t *testing.T) {
	runM68KCPUTestSuite(t, false)
}

func TestM68KCPUTestSuiteJIT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	runM68KCPUTestSuite(t, true)
}

func TestM68KCPUTest_FirstBFMemCaseJIT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bin := buildCPUTestBinary(t)

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	for i, b := range bin {
		cpu.Write8(M68K_ENTRY_POINT+uint32(i), b)
	}
	for addr := uint32(ctMailbox); addr < ctMailbox+ctCaseLogSize+ctFailLogSize+64; addr += 4 {
		cpu.Write32(addr, 0)
	}

	cpu.Write32(0, M68K_STACK_START)
	cpu.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)
	cpu.PC = 0x12C8 // case_bf_mem_bftst_0_8
	cpu.SR = M68K_SR_S | 0x0700
	cpu.AddrRegs[7] = M68K_STACK_START - 4
	cpu.SSP = M68K_STACK_START
	cpu.USP = M68K_STACK_START
	cpu.Write32(M68K_STACK_START-4, 0x2000)
	cpu.Write16(0x2000, 0x4E72) // STOP
	cpu.Write16(0x2002, 0x2700)
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	cpu.m68kJitEnabled = true

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			goto finished
		case <-timeout:
			cpu.running.Store(false)
			<-done
			t.Fatalf("first bf_mem JIT case timed out (PC=$%08X SP=$%08X pass=%d fail=%d)",
				cpu.PC, cpu.AddrRegs[7], cpu.Read32(ctPassCount), cpu.Read32(ctFailCount))
		case <-ticker.C:
			if cpu.stopped.Load() {
				cpu.running.Store(false)
			}
		}
	}

finished:
	if !cpu.stopped.Load() {
		t.Fatalf("first bf_mem JIT case did not stop cleanly (PC=$%08X SP=$%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(ctPassCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1 (PC=$%08X fail=%d case_log_pos=%d)", got, cpu.PC, cpu.Read32(ctFailCount), cpu.Read32(ctCaseLogPos))
	}
	if got := cpu.Read32(ctFailCount); got != 0 {
		t.Fatalf("fail_count = %d, want 0", got)
	}
}

func TestM68KCPUTest_LogPassHelperJIT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bin := buildCPUTestBinary(t)

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	for i, b := range bin {
		cpu.Write8(M68K_ENTRY_POINT+uint32(i), b)
	}
	for addr := uint32(ctMailbox); addr < ctMailbox+ctCaseLogSize+ctFailLogSize+64; addr += 4 {
		cpu.Write32(addr, 0)
	}

	cpu.PC = 0x1032 // ct_log_pass
	cpu.SR = M68K_SR_S | 0x0700
	cpu.AddrRegs[7] = M68K_STACK_START - 4
	cpu.SSP = M68K_STACK_START
	cpu.USP = M68K_STACK_START
	cpu.Write32(M68K_STACK_START-4, 0x2000)
	cpu.Write16(0x2000, 0x4E72) // STOP
	cpu.Write16(0x2002, 0x2700)
	cpu.AddrRegs[0] = 0x12345678
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	cpu.m68kJitEnabled = true

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			goto finished
		case <-timeout:
			cpu.running.Store(false)
			<-done
			t.Fatalf("ct_log_pass JIT timed out (PC=$%08X SP=$%08X)", cpu.PC, cpu.AddrRegs[7])
		case <-ticker.C:
			if cpu.stopped.Load() {
				cpu.running.Store(false)
			}
		}
	}

finished:
	if !cpu.stopped.Load() {
		t.Fatalf("ct_log_pass JIT did not stop cleanly (PC=$%08X SP=$%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(ctPassCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1", got)
	}
	if got := cpu.Read32(ctCaseLogPos); got != 8 {
		t.Fatalf("case_log_pos = %d, want 8", got)
	}
	if got := cpu.Read32(ctCaseLog); got != 0x12345678 {
		t.Fatalf("case_log[0].name_ptr = $%08X, want $12345678", got)
	}
	if got := cpu.Read32(ctCaseLog + 4); got != 1 {
		t.Fatalf("case_log[0].result = %d, want 1", got)
	}
}

func TestM68KCPUTest_FirstCHK2BInRangeCaseJIT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bin := buildCPUTestBinary(t)

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	for i, b := range bin {
		cpu.Write8(M68K_ENTRY_POINT+uint32(i), b)
	}
	for addr := uint32(ctMailbox); addr < ctMailbox+ctCaseLogSize+ctFailLogSize+64; addr += 4 {
		cpu.Write32(addr, 0)
	}

	cpu.PC = 0x43B0 // case_chk2_cmp2_chk2b_inrange
	cpu.SR = M68K_SR_S | 0x0700
	cpu.AddrRegs[7] = M68K_STACK_START - 4
	cpu.SSP = M68K_STACK_START
	cpu.USP = M68K_STACK_START
	cpu.Write32(M68K_STACK_START-4, 0x2000)
	cpu.Write16(0x2000, 0x4E72) // STOP
	cpu.Write16(0x2002, 0x2700)
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	cpu.m68kJitEnabled = true

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			goto finished
		case <-timeout:
			cpu.running.Store(false)
			<-done
			t.Fatalf("first CHK2.B in-range JIT case timed out (PC=$%08X SP=$%08X pass=%d fail=%d trap=%d vec=%d)",
				cpu.PC, cpu.AddrRegs[7], cpu.Read32(ctPassCount), cpu.Read32(ctFailCount), cpu.Read32(ctMailbox+24), cpu.Read32(ctMailbox+28))
		case <-ticker.C:
			if cpu.stopped.Load() {
				cpu.running.Store(false)
			}
		}
	}

finished:
	if !cpu.stopped.Load() {
		t.Fatalf("first CHK2.B in-range JIT case did not stop cleanly (PC=$%08X SP=$%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(ctPassCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1 (PC=$%08X fail=%d trap=%d vec=%d SP=$%08X)",
			got, cpu.PC, cpu.Read32(ctFailCount), cpu.Read32(ctMailbox+24), cpu.Read32(ctMailbox+28), cpu.AddrRegs[7])
	}
	if got := cpu.Read32(ctFailCount); got != 0 {
		t.Fatalf("fail_count = %d, want 0", got)
	}
}

func TestM68KCPUTest_FirstCHK2BOutOfRangeCaseJIT(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bin := buildCPUTestBinary(t)

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	for i, b := range bin {
		cpu.Write8(M68K_ENTRY_POINT+uint32(i), b)
	}
	for addr := uint32(ctMailbox); addr < ctMailbox+ctCaseLogSize+ctFailLogSize+64; addr += 4 {
		cpu.Write32(addr, 0)
	}

	cpu.PC = 0x443C // case_chk2_cmp2_chk2b_outrange
	cpu.SR = M68K_SR_S | 0x0700
	cpu.AddrRegs[7] = M68K_STACK_START - 4
	cpu.SSP = M68K_STACK_START
	cpu.USP = M68K_STACK_START
	cpu.Write32(M68K_STACK_START-4, 0x2000)
	cpu.Write16(0x2000, 0x4E72) // STOP
	cpu.Write16(0x2002, 0x2700)
	cpu.stackLowerBound = 0x00002000
	cpu.stackUpperBound = M68K_MEMORY_SIZE
	cpu.m68kJitEnabled = true

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			goto finished
		case <-timeout:
			cpu.running.Store(false)
			<-done
			t.Fatalf("first CHK2.B out-of-range JIT case timed out (PC=$%08X SP=$%08X pass=%d fail=%d trap=%d vec=%d)",
				cpu.PC, cpu.AddrRegs[7], cpu.Read32(ctPassCount), cpu.Read32(ctFailCount), cpu.Read32(ctMailbox+24), cpu.Read32(ctMailbox+28))
		case <-ticker.C:
			if cpu.stopped.Load() {
				cpu.running.Store(false)
			}
		}
	}

finished:
	if !cpu.stopped.Load() {
		t.Fatalf("first CHK2.B out-of-range JIT case did not stop cleanly (PC=$%08X SP=$%08X)", cpu.PC, cpu.AddrRegs[7])
	}
	if got := cpu.Read32(ctPassCount); got != 1 {
		t.Fatalf("pass_count = %d, want 1 (PC=$%08X fail=%d trap=%d vec=%d SP=$%08X)",
			got, cpu.PC, cpu.Read32(ctFailCount), cpu.Read32(ctMailbox+24), cpu.Read32(ctMailbox+28), cpu.AddrRegs[7])
	}
	if got := cpu.Read32(ctFailCount); got != 0 {
		t.Fatalf("fail_count = %d, want 0", got)
	}
}
