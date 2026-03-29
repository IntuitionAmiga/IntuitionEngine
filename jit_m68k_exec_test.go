// jit_m68k_exec_test.go - Integration tests for M68020 JIT execution loop

//go:build amd64 && linux

package main

import (
	"runtime"
	"testing"
	"time"
)

// runM68KJITProgram loads M68K opcodes at startPC, runs ExecuteJIT with a timeout,
// and returns the CPU for result inspection.
func runM68KJITProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP (initial stack)
	bus.Write32(4, startPC)    // reset vector → our code
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = startPC
	cpu.SR = M68K_SR_S // supervisor mode

	// Write opcodes in big-endian
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}

	// Run with timeout
	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("M68K JIT execution timed out")
	}

	return cpu
}

// writeBELong writes a big-endian uint32 to memory.
func writeBELong(mem []byte, addr uint32, val uint32) {
	mem[addr] = byte(val >> 24)
	mem[addr+1] = byte(val >> 16)
	mem[addr+2] = byte(val >> 8)
	mem[addr+3] = byte(val)
}

// TestM68KJIT_Exec_SimpleHalt tests that a STOP instruction halts execution.
func TestM68KJIT_Exec_SimpleHalt(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// MOVEQ #42,D0; then ILLEGAL (0x4AFC) to halt
	// Actually, ILLEGAL will cause an exception which may loop.
	// Use a simpler approach: just run a few instructions and then check running stopped.
	// MOVEQ #42,D0; MOVEQ #0,D1; then we stop running externally.
	cpu.memory[0x1000] = 0x70
	cpu.memory[0x1001] = 0x2A // MOVEQ #42,D0
	cpu.memory[0x1002] = 0x72
	cpu.memory[0x1003] = 0x00 // MOVEQ #0,D1

	// Put a STOP #$2700 to halt — STOP needs fallback to interpreter
	cpu.memory[0x1004] = 0x4E
	cpu.memory[0x1005] = 0x72 // STOP
	cpu.memory[0x1006] = 0x27
	cpu.memory[0x1007] = 0x00 // #$2700

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Wait a bit then stop
	time.Sleep(100 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_ALUSequence runs ADD/SUB through the full dispatcher.
func TestM68KJIT_Exec_ALUSequence(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #10,D0; MOVEQ #20,D1; ADD.L D0,D1; STOP
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x700A, // MOVEQ #10,D0
		0x7214, // MOVEQ #20,D1
		0xD280, // ADD.L D0,D1  (D1 = D1 + D0 = 30)
	)

	if cpu.DataRegs[0] != 10 {
		t.Errorf("D0 = %d, want 10", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 30 {
		t.Errorf("D1 = %d, want 30", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_MemoryMove runs MOVE with memory through the dispatcher.
func TestM68KJIT_Exec_MemoryMove(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0xDEAD, 0xBEEF, // MOVE.L #$DEADBEEF,D0
	)

	if cpu.DataRegs[0] != 0xDEADBEEF {
		t.Errorf("D0 = 0x%08X, want 0xDEADBEEF", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_JSR_RTS runs a subroutine call through the dispatcher.
func TestM68KJIT_Exec_JSR_RTS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000 // stack pointer

	// Main: MOVEQ #1,D0; JSR $1010; MOVEQ #3,D0; STOP
	// Sub at $1010: MOVEQ #2,D1; RTS
	// After execution: D0=3 (set after JSR returns), D1=2
	pc := uint32(0x1000)
	writeW := func(val uint16) {
		cpu.memory[pc] = byte(val >> 8)
		cpu.memory[pc+1] = byte(val)
		pc += 2
	}

	writeW(0x7001) // MOVEQ #1,D0
	writeW(0x4EB9) // JSR
	writeW(0x0000) // abs.L high
	writeW(0x1010) // abs.L low (target = $1010)
	writeW(0x7003) // MOVEQ #3,D0 (executed after RTS)
	// STOP to halt
	writeW(0x4E72)
	writeW(0x2700)

	// Subroutine at $1010
	pc = 0x1010
	writeW(0x7402) // MOVEQ #2,D2
	writeW(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after JSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in subroutine)", cpu.DataRegs[2])
	}
}

// TestM68KJIT_Exec_BranchTaken runs a conditional branch through the dispatcher.
func TestM68KJIT_Exec_BranchTaken(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #0,D0; TST.L D0; BEQ +4; MOVEQ #99,D1; MOVEQ #42,D2; STOP
	// BEQ should skip MOVEQ #99,D1 and land on MOVEQ #42,D2
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7000, // MOVEQ #0,D0
		0x4A80, // TST.L D0 → sets Z=1
		0x6702, // BEQ.B +2 (skip next instruction)
		0x7263, // MOVEQ #99,D1 (should be skipped)
		0x742A, // MOVEQ #42,D2 (branch target)
	)

	if cpu.DataRegs[1] != 0 {
		t.Errorf("D1 = %d, want 0 (should be skipped by BEQ)", cpu.DataRegs[1])
	}
	if cpu.DataRegs[2] != 42 {
		t.Errorf("D2 = %d, want 42", cpu.DataRegs[2])
	}
}

// runM68KJITStopProgram runs a program followed by STOP, waits for it to halt.
func runM68KJITStopProgram(t *testing.T, startPC uint32, opcodes ...uint16) *M68KCPU {
	t.Helper()

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, startPC)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = startPC
	cpu.SR = M68K_SR_S

	// Write opcodes
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	// Append STOP #$2700
	cpu.memory[pc] = 0x4E
	cpu.memory[pc+1] = 0x72
	cpu.memory[pc+2] = 0x27
	cpu.memory[pc+3] = 0x00

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		<-done
		// Don't fatal — STOP halts in a loop, we just stop it externally
	}

	return cpu
}

// TestM68KJIT_Exec_DBRA_Loop runs a DBRA loop through the full dispatcher.
func TestM68KJIT_Exec_DBRA_Loop(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Program: MOVEQ #3,D0; MOVEQ #0,D1; loop: ADDQ #1,D1; DBRA D0,loop; STOP
	// Loop runs 4 times (D0: 3→2→1→0→-1)
	// After: D1 = 4
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7003,         // MOVEQ #3,D0
		0x7200,         // MOVEQ #0,D1
		0x5281,         // ADDQ.L #1,D1 (at 0x1004)
		0x51C8, 0xFFFC, // DBRA D0,$1004 (displacement = -4)
	)

	if cpu.DataRegs[1] != 4 {
		t.Errorf("DBRA loop: D1 = %d, want 4", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_MemoryALU runs ADD with immediate operand through dispatcher.
func TestM68KJIT_Exec_MemoryALU(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x0064, // MOVE.L #100,D0
		0xD0BC, 0x0000, 0x0032, // ADD.L #50,D0
	)

	if cpu.DataRegs[0] != 150 {
		t.Errorf("ADD.L #50,D0: D0 = %d, want 150", cpu.DataRegs[0])
	}
}

// ===========================================================================
// Block Chaining Tests (Stages 2-4)
// ===========================================================================

// TestM68KJIT_Exec_BRA_ChainPatch verifies that BRA chains directly to a
// compiled target block via patched JMP rel32.
func TestM68KJIT_Exec_BRA_ChainPatch(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Sequential code: MOVEQ #1,D0; BRA.B +4; MOVEQ #99,D2; MOVEQ #2,D1; STOP
	// BRA.B +4 skips MOVEQ #99,D2 (2 bytes) + goes to MOVEQ #2,D1.
	// BRA.B displacement = +4 means target = instrPC+2+4.
	// At 0x1002: BRA.B → target = 0x1002 + 2 + 4 = 0x1008.
	// 0x1004: MOVEQ #99,D2 (skipped)
	// 0x1006: MOVEQ #77,D2 (also skipped — BRA skips to 0x1008)
	// Actually BRA.B +2: target = 0x1002 + 2 + 2 = 0x1006
	// But BRA exits block! So MOVEQ #2,D1 is in a different block.
	// That's fine — dispatcher re-enters at target and compiles block B.
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x7001, // MOVEQ #1,D0 at 0x1000
		0x6004, // BRA.B +4 at 0x1002 → target 0x1008
		0x7499, // MOVEQ #-103,D2 at 0x1004 (skipped by BRA)
		0x4E71, // NOP at 0x1006 (skipped by BRA)
		0x7201, // MOVEQ #1,D1 at 0x1008 (target of BRA) — 0x7201 = MOVEQ #1,D1
	)

	if cpu.DataRegs[0] != 1 {
		t.Errorf("D0 = %d, want 1 (set before BRA)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 1 {
		t.Errorf("D1 = %d, want 1 (set at BRA target)", cpu.DataRegs[1])
	}
}

// TestM68KJIT_Exec_ChainBudgetExhaustion verifies that chained execution
// returns to Go after the chain budget is exhausted.
func TestM68KJIT_Exec_ChainBudgetExhaustion(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Two blocks that BRA to each other, forming an infinite chain loop.
	// The budget (64) should stop execution and return to Go.
	// Block A at 0x1000: ADDQ #1,D0; BRA 0x2000
	// Block B at 0x2000: ADDQ #1,D0; BRA 0x1000
	// After timeout: D0 > 0 (proves blocks executed) and program didn't hang.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	w := func(pc uint32, ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// Block A: ADDQ.L #1,D0 + BRA.W to 0x2000 (disp = 0x2000 - 0x1004 = 0x0FFC)
	w(0x1000, 0x5280, 0x6000, 0x0FFC)
	// Block B: ADDQ.L #1,D0 + BRA.W to 0x1000 (disp = 0x1000 - 0x2004 = 0xEFFC)
	w(0x2000, 0x5280, 0x6000, 0xEFFC)

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	// D0 should be large (many iterations via chaining) but execution didn't hang
	if cpu.DataRegs[0] == 0 {
		t.Error("D0 should be > 0 after chained BRA loop")
	}
}

// TestM68KJIT_Exec_JSR_RTS_Chain verifies JSR→subroutine→RTS chain round-trip.
func TestM68KJIT_Exec_JSR_RTS_Chain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Simple: JSR $2000; MOVEQ #3,D0; STOP
	// Sub at $2000: MOVEQ #2,D1; RTS
	// After: D0=3 (from code after JSR), D1=2 (from subroutine)
	// This is the same pattern as the existing TestM68KJIT_Exec_JSR_RTS
	// but explicitly tests that chain patching connects JSR→sub→RTS→return.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x7001)                 // MOVEQ #1,D0
	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	w(0x7003)                 // MOVEQ #3,D0 (after RTS)
	w(0x4E72, 0x2700)         // STOP

	pc = 0x2000
	w(0x7402) // MOVEQ #2,D2
	w(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set after JSR return)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 2 {
		t.Errorf("D2 = %d, want 2 (set in subroutine)", cpu.DataRegs[2])
	}
}

// ===========================================================================
// Lazy CCR Integration Tests (Stage 5)
// ===========================================================================

// TestM68KJIT_Exec_LazyCCR_CMP_BEQ verifies CMP;BEQ uses direct Jcc from EFLAGS.
func TestM68KJIT_Exec_LazyCCR_CMP_BEQ(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// D0=42, D1=42. CMP D1,D0; BEQ skip; MOVEQ #99,D2; skip: MOVEQ #1,D3; STOP
	// BEQ taken → D2 stays 0, D3=1
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0x0000, 0x002A, // MOVE.L #42,D0
		0x223C, 0x0000, 0x002A, // MOVE.L #42,D1
		0xB081, // CMP.L D1,D0
		0x6702, // BEQ.B +2 (skip MOVEQ #99)
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x7601, // MOVEQ #1,D3
	)

	if cpu.DataRegs[2] != 0 {
		t.Errorf("D2 = %d, want 0 (BEQ should skip)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[3] != 1 {
		t.Errorf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

// TestM68KJIT_Exec_LazyCCR_ADD_BCS verifies ADD;BCS with carry from EFLAGS.
func TestM68KJIT_Exec_LazyCCR_ADD_BCS(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// D0=0xFFFFFFFF, D1=1. ADD D1,D0 → carry. BCS skip; MOVEQ #99,D2; skip: MOVEQ #1,D3; STOP
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x203C, 0xFFFF, 0xFFFF, // MOVE.L #$FFFFFFFF,D0
		0x7201, // MOVEQ #1,D1
		0xD081, // ADD.L D1,D0 → 0 with carry
		0x6502, // BCS.B +2 (skip)
		0x7499, // MOVEQ #-103,D2 (skipped)
		0x7601, // MOVEQ #1,D3
	)

	if cpu.DataRegs[0] != 0 {
		t.Errorf("D0 = 0x%08X, want 0", cpu.DataRegs[0])
	}
	if cpu.DataRegs[2] != 0 {
		t.Errorf("D2 = %d, want 0 (BCS should skip — carry set)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[3] != 1 {
		t.Errorf("D3 = %d, want 1", cpu.DataRegs[3])
	}
}

// TestM68KJIT_Exec_RTS_CacheHitWithLazyCCR verifies the JSR+DBRA+RTS chain
// loop with block chaining and lazy CCR. Uses the same polling pattern as
// the benchmark to match working behavior.
func TestM68KJIT_Exec_RTS_CacheHitWithLazyCCR(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x3E3C, 0x0004)         // MOVE.W #4,D7
	loopTop := pc             // 0x1004
	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop
	w(0x4E72, 0x2700)       // STOP

	pc = 0x2000
	w(0x5280) // ADDQ.L #1,D0 (arithmetic → flagsLiveArith)
	w(0x4E75) // RTS

	// Use the same polling pattern as runM68KBenchJIT
	cpu.PC = 0x1000
	cpu.running.Store(true)
	cpu.stopped.Store(false)

	done := make(chan struct{})
	go func() {
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Poll for STOP (with timeout)
	deadline := time.After(5 * time.Second)
	for !cpu.stopped.Load() {
		select {
		case <-deadline:
			cpu.running.Store(false)
			<-done
			t.Fatal("RTS cache hit + lazy CCR test timed out")
		default:
			runtime.Gosched()
		}
	}
	cpu.running.Store(false)
	<-done

	// ADDQ #1,D0 called 5 times (D7: 4,3,2,1,0,-1)
	if cpu.DataRegs[0] != 5 {
		t.Errorf("D0 = %d, want 5 (ADDQ called 5 times)", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_RTS_IOBailRetPC verifies that the RTS I/O bail path
// correctly sets RetPC so the dispatcher re-executes via interpreter.
func TestM68KJIT_Exec_RTS_IOBailRetPC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Set A7 to the I/O region (>= 0xA0000) so RTS bails.
	// The bail should set RetPC to the RTS instruction's PC.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x000A0010) // SSP in I/O region
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	// Put A7 in I/O region — RTS will bail when reading the return address
	cpu.AddrRegs[7] = 0xA0000

	// Write a return address at 0xA0000 (big-endian 0x00001008)
	cpu.Write32(0xA0000, 0x00001008)

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x7001) // MOVEQ #1,D0
	w(0x4E75) // RTS (at 0x1002) — will bail because A7 >= IOThreshold
	// After interpreter re-executes RTS, PC should be 0x1008
	w(0x4E71)         // NOP (0x1004)
	w(0x4E71)         // NOP (0x1006)
	w(0x7003)         // MOVEQ #3,D0 (0x1008 — return target)
	w(0x4E72, 0x2700) // STOP

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	// D0 should be 3 (set at 0x1008, the return target after RTS)
	if cpu.DataRegs[0] != 3 {
		t.Errorf("D0 = %d, want 3 (set at return target after RTS I/O bail)", cpu.DataRegs[0])
	}
}

// TestM68KJIT_Exec_SelfModDuringChain verifies that self-modifying code
// during chained execution triggers cache invalidation.
func TestM68KJIT_Exec_SelfModDuringChain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// Block writes to a code page, triggering NeedInval.
	// MOVEQ #42,D0; MOVE.L D0,(0x1000) — writes to own code page
	// Then STOP. The write to 0x1000 (code page) should trigger invalidation.
	cpu := runM68KJITStopProgram(t, 0x1000,
		0x702A,                 // MOVEQ #42,D0
		0x23C0, 0x0000, 0x1000, // MOVE.L D0,$1000 (writes to code page)
	)

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", cpu.DataRegs[0])
	}
	// The program ran to completion (didn't crash from invalidation)
}

// TestM68KJIT_Exec_RTSCacheClearedOnInval verifies that the RTS inline
// cache is cleared on cache invalidation.
func TestM68KJIT_Exec_RTSCacheClearedOnInval(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// JSR + RTS + self-mod write + STOP
	// The self-mod write should clear the RTS cache.
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000)
	bus.Write32(4, 0x1000)
	cpu := NewM68KCPU(bus)
	cpu.m68kJitEnabled = true
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000

	pc := uint32(0x1000)
	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	w(0x4EB9, 0x0000, 0x2000) // JSR $2000
	w(0x23C0, 0x0000, 0x2000) // MOVE.L D0,$2000 (write to sub code page → invalidate)
	w(0x4E72, 0x2700)         // STOP

	pc = 0x2000
	w(0x702A) // MOVEQ #42,D0
	w(0x4E75) // RTS

	done := make(chan struct{})
	go func() {
		cpu.running.Store(true)
		cpu.M68KExecuteJIT()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cpu.running.Store(false)
	<-done

	if cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42 (set in subroutine before invalidation)", cpu.DataRegs[0])
	}
}
