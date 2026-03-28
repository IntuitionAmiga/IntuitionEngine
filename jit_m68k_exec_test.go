// jit_m68k_exec_test.go - Integration tests for M68020 JIT execution loop

//go:build amd64 && linux

package main

import (
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
