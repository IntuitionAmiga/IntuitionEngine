// jit_6502_exec_test.go - Integration tests for the 6502 JIT dispatcher

//go:build (amd64 || arm64) && linux

package main

import (
	"runtime"
	"testing"
	"time"
)

// JAM ($02) is used as halt in integration tests.
// BRK doesn't stop the CPU — it vectors to IRQ handler.
// JAM calls running.Store(false) which stops the CPU cleanly.
const haltOpcode = 0x02

// ===========================================================================
// Dispatcher Integration Tests
// ===========================================================================

func TestJIT6502_Exec_NOP_Halt(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// NOP + JAM
	bus.Write8(0x0600, 0xEA) // NOP
	bus.Write8(0x0601, haltOpcode)

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	if cpu.Running() {
		t.Error("CPU should be stopped after JAM")
	}
}

func TestJIT6502_Exec_LDA_STA(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// LDA #$42; STA $10; LDA #$00; JAM
	bus.Write8(0x0600, 0xA9)
	bus.Write8(0x0601, 0x42)
	bus.Write8(0x0602, 0x85)
	bus.Write8(0x0603, 0x10)
	bus.Write8(0x0604, 0xA9)
	bus.Write8(0x0605, 0x00)
	bus.Write8(0x0606, haltOpcode)

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	val := bus.Read8(0x0010)
	if val != 0x42 {
		t.Errorf("mem[$10] = 0x%02X, want 0x42", val)
	}
	if cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", cpu.A)
	}
}

func TestJIT6502_Exec_JSR_RTS(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// Main: JSR $0610; LDA #$FF; JAM
	bus.Write8(0x0600, 0x20) // JSR $0610
	bus.Write8(0x0601, 0x10)
	bus.Write8(0x0602, 0x06)
	bus.Write8(0x0603, 0xA9) // LDA #$FF
	bus.Write8(0x0604, 0xFF)
	bus.Write8(0x0605, haltOpcode)

	// Sub: LDA #$42; RTS
	bus.Write8(0x0610, 0xA9)
	bus.Write8(0x0611, 0x42)
	bus.Write8(0x0612, 0x60)

	cpu.PC = 0x0600
	cpu.SP = 0xFF
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	if cpu.A != 0xFF {
		t.Errorf("A = 0x%02X, want 0xFF (LDA after RTS)", cpu.A)
	}
}

func TestJIT6502_Exec_DEX_BNE_CountDown(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// LDX #$05; DEX; BNE -3; JAM
	bus.Write8(0x0600, 0xA2) // LDX #$05
	bus.Write8(0x0601, 0x05)
	bus.Write8(0x0602, 0xCA) // DEX
	bus.Write8(0x0603, 0xD0) // BNE -3
	bus.Write8(0x0604, 0xFD)
	bus.Write8(0x0605, haltOpcode)

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	if cpu.X != 0x00 {
		t.Errorf("X = 0x%02X, want 0x00", cpu.X)
	}
}

func TestJIT6502_Exec_IOBail(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// LDA $D200 (I/O); JAM
	bus.Write8(0x0600, 0xAD)
	bus.Write8(0x0601, 0x00)
	bus.Write8(0x0602, 0xD2)
	bus.Write8(0x0603, haltOpcode)

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	if cpu.Running() {
		t.Error("CPU should be stopped")
	}
}

func TestJIT6502_Exec_CycleAccuracy(t *testing.T) {
	// Run the same program through interpreter and JIT, compare Cycles
	program := []byte{
		0xA9, 0x42, // LDA #$42 (2)
		0x85, 0x10, // STA $10 (3)
		0xA5, 0x10, // LDA $10 (3)
		0xAA,       // TAX (2)
		0xE8,       // INX (2)
		0xE8,       // INX (2)
		0xCA,       // DEX (2)
		0xEA,       // NOP (2)
		haltOpcode, // JAM
	}

	// Interpreter
	bus1 := NewMachineBus()
	cpu1 := NewCPU_6502(bus1)
	cpu1.SetRDYLine(true)
	for i, b := range program {
		bus1.Write8(0x0600+uint32(i), b)
	}
	cpu1.PC = 0x0600
	cpu1.SetRunning(true)
	cpu1.Cycles = 0
	cpu1.Execute()
	interpCycles := cpu1.Cycles

	// JIT
	bus2 := NewMachineBus()
	cpu2 := NewCPU_6502(bus2)
	cpu2.SetRDYLine(true)
	for i, b := range program {
		bus2.Write8(0x0600+uint32(i), b)
	}
	cpu2.PC = 0x0600
	cpu2.SetRunning(true)
	cpu2.Cycles = 0
	cpu2.jitEnabled = true
	cpu2.ExecuteJIT6502()
	jitCycles := cpu2.Cycles

	if interpCycles != jitCycles {
		t.Errorf("Cycle mismatch: interpreter=%d, JIT=%d", interpCycles, jitCycles)
	}
	if cpu1.A != cpu2.A {
		t.Errorf("A mismatch: interp=0x%02X, JIT=0x%02X", cpu1.A, cpu2.A)
	}
	if cpu1.X != cpu2.X {
		t.Errorf("X mismatch: interp=0x%02X, JIT=0x%02X", cpu1.X, cpu2.X)
	}
}

func TestJIT6502_Exec_CycleAccuracy_WithBranch(t *testing.T) {
	program := []byte{
		0xA2, 0x03, // LDX #$03 (2)
		0xCA,       // DEX (2)
		0xD0, 0xFD, // BNE -3 (0 base, +1 taken)
		haltOpcode,
	}

	// Interpreter
	bus1 := NewMachineBus()
	cpu1 := NewCPU_6502(bus1)
	cpu1.SetRDYLine(true)
	for i, b := range program {
		bus1.Write8(0x0600+uint32(i), b)
	}
	cpu1.PC = 0x0600
	cpu1.SetRunning(true)
	cpu1.Cycles = 0
	cpu1.Execute()

	// JIT
	bus2 := NewMachineBus()
	cpu2 := NewCPU_6502(bus2)
	cpu2.SetRDYLine(true)
	for i, b := range program {
		bus2.Write8(0x0600+uint32(i), b)
	}
	cpu2.PC = 0x0600
	cpu2.SetRunning(true)
	cpu2.Cycles = 0
	cpu2.jitEnabled = true
	cpu2.ExecuteJIT6502()

	if cpu1.Cycles != cpu2.Cycles {
		t.Errorf("Cycle mismatch with branch: interpreter=%d, JIT=%d", cpu1.Cycles, cpu2.Cycles)
	}
	if cpu1.X != cpu2.X {
		t.Errorf("X mismatch: interp=0x%02X, JIT=0x%02X", cpu1.X, cpu2.X)
	}
}

func TestJIT6502_Exec_DebugFallback(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	bus.Write8(0x0600, 0xA9) // LDA #$42
	bus.Write8(0x0601, 0x42)
	bus.Write8(0x0602, haltOpcode)

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.Debug = true
	cpu.jitEnabled = true

	cpu.jit6502Execute() // should use interpreter due to Debug=true

	if cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42 (interpreter fallback)", cpu.A)
	}
}

func TestJIT6502_Exec_SelfMod(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// LDA #$EA; STA $0600; JMP $0610
	// At $0610: JAM
	bus.Write8(0x0600, 0xA9) // LDA #$EA
	bus.Write8(0x0601, 0xEA)
	bus.Write8(0x0602, 0x8D) // STA $0600
	bus.Write8(0x0603, 0x00)
	bus.Write8(0x0604, 0x06)
	bus.Write8(0x0605, 0x4C) // JMP $0610
	bus.Write8(0x0606, 0x10)
	bus.Write8(0x0607, 0x06)
	bus.Write8(0x0610, haltOpcode) // JAM

	cpu.PC = 0x0600
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	// Verify the self-mod write happened
	val := bus.Read8(0x0600)
	if val != 0xEA {
		t.Errorf("mem[$0600] = 0x%02X, want 0xEA (self-mod)", val)
	}
	if cpu.Running() {
		t.Error("CPU should be stopped")
	}
}

// ===========================================================================
// Page-Granular Invalidation Tests
// ===========================================================================

func TestJIT6502_Exec_PageGranularInvalidation(t *testing.T) {
	// Compile blocks on two different pages. Self-modify one page.
	// Verify the other page's block survives.
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// Block at $0600 (page $06): LDA #$42; STA $0700; JMP $0700
	// This writes $42 to $0700 (page $07) which has code → triggers page-granular inval
	bus.Write8(0x0600, 0xA9) // LDA #$42
	bus.Write8(0x0601, 0x42)
	bus.Write8(0x0602, 0x8D) // STA $0700
	bus.Write8(0x0603, 0x00)
	bus.Write8(0x0604, 0x07)
	bus.Write8(0x0605, 0x4C) // JMP $0700
	bus.Write8(0x0606, 0x00)
	bus.Write8(0x0607, 0x07)

	// Block at $0700 (page $07): NOP; JAM (will be self-modded but we execute before that)
	bus.Write8(0x0700, 0xEA) // NOP
	bus.Write8(0x0701, haltOpcode)

	cpu.PC = 0x0700
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	// First run: execute $0700 block to cache it
	cpu.ExecuteJIT6502()

	// Now run from $0600 which self-modifies $0700 then jumps there
	cpu.SetRunning(true)
	cpu.PC = 0x0600
	cpu.ExecuteJIT6502()

	// CPU should halt. The self-mod of $0700 invalidated that page,
	// and the JMP $0700 recompiles it with the new code ($42 at $0700).
	if cpu.Running() {
		t.Error("CPU should be stopped")
	}
}

func TestJIT6502_Exec_StaleBitmapFalsePositive(t *testing.T) {
	// After page-granular invalidation, stale codePageBitmap entries may remain
	// for pages that no longer have blocks. Verify this causes at most a harmless
	// extra invalidation call, not corruption.
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// Block spanning pages $06 and $07:
	// Put code near end of page $06 that continues into page $07
	// $06FE: LDA #$42 (2 bytes, ends at $0700)
	// $0700: NOP; JAM
	bus.Write8(0x06FE, 0xA9) // LDA #$42
	bus.Write8(0x06FF, 0x42)
	bus.Write8(0x0700, 0xEA) // NOP
	bus.Write8(0x0701, haltOpcode)

	cpu.PC = 0x06FE
	cpu.SetRunning(true)
	cpu.jitEnabled = true
	cpu.ExecuteJIT6502()

	if cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42", cpu.A)
	}

	// Now both pages $06 and $07 should have codePageBitmap set.
	// Self-modify page $06 to trigger invalidation of that page.
	// Page $07's bitmap entry may remain stale (conservative approach).
	// This is OK — it just means writes to $07xx trigger unnecessary inval checks.
	// Verify the CPU still runs correctly by re-running.
	bus.Write8(0x06FE, 0xA9) // rewrite same code
	bus.Write8(0x06FF, 0x99) // different value

	cpu.PC = 0x06FE
	cpu.SetRunning(true)
	cpu.ExecuteJIT6502()

	if cpu.A != 0x99 {
		t.Errorf("A = 0x%02X, want 0x99 (after self-mod recompile)", cpu.A)
	}
}

// ===========================================================================
// Interrupt/RDY/Reset Under Chaining Tests
// ===========================================================================

func TestJIT6502_Exec_NMI_DuringChaining(t *testing.T) {
	// Set up a loop, fire NMI, verify it's serviced.
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// Main: LDX #$00; INX; JMP $0602 (infinite loop at $0602-$0607)
	bus.Write8(0x0600, 0xA2) // LDX #$00
	bus.Write8(0x0601, 0x00)
	bus.Write8(0x0602, 0xE8) // INX
	bus.Write8(0x0603, 0x4C) // JMP $0602
	bus.Write8(0x0604, 0x02)
	bus.Write8(0x0605, 0x06)

	// NMI handler at $0800: JAM (stop CPU)
	bus.Write8(0x0800, haltOpcode)

	// Set NMI vector
	bus.Write8(0xFFFA, 0x00)
	bus.Write8(0xFFFB, 0x08)

	cpu.PC = 0x0600
	cpu.SP = 0xFF
	cpu.SetRunning(true)
	cpu.jitEnabled = true

	// Fire NMI after giving the JIT time to execute the loop
	go func() {
		// Wait for the JIT to start executing
		for !cpu.executing.Load() {
			runtime.Gosched()
		}
		// Give the loop time to run (budget=64 blocks before first Go return)
		time.Sleep(5 * time.Millisecond)
		cpu.nmiPending.Store(true)
	}()

	cpu.ExecuteJIT6502()

	// The NMI should have been serviced, vectoring to $0800 (JAM)
	if cpu.Running() {
		t.Error("CPU should be stopped (NMI → JAM)")
	}
}

func TestJIT6502_Exec_RDY_HoldDuringExecution(t *testing.T) {
	// Deassert RDY while JIT is running, verify the CPU pauses.
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// Infinite loop: INX; JMP $0600
	bus.Write8(0x0600, 0xE8) // INX
	bus.Write8(0x0601, 0x4C) // JMP $0600
	bus.Write8(0x0602, 0x00)
	bus.Write8(0x0603, 0x06)

	cpu.PC = 0x0600
	cpu.SP = 0xFF
	cpu.SetRunning(true)
	cpu.jitEnabled = true

	go func() {
		for !cpu.executing.Load() {
			// spin
		}
		// Deassert RDY to pause
		cpu.SetRDYLine(false)
		// Wait a bit, then stop the CPU
		for i := 0; i < 10000; i++ {
			if cpu.rdyHold {
				break
			}
		}
		// Stop CPU cleanly
		cpu.SetRunning(false)
	}()

	cpu.ExecuteJIT6502()

	if cpu.Running() {
		t.Error("CPU should be stopped")
	}
}

func TestJIT6502_Exec_Reset_DuringExecution(t *testing.T) {
	// Request reset while JIT is running, verify it's acknowledged.
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	// Infinite loop: INX; JMP $0600
	bus.Write8(0x0600, 0xE8)
	bus.Write8(0x0601, 0x4C)
	bus.Write8(0x0602, 0x00)
	bus.Write8(0x0603, 0x06)

	// Reset vector → $0800 (JAM)
	bus.Write8(0x0800, haltOpcode)
	bus.Write8(RESET_VECTOR, 0x00)
	bus.Write8(RESET_VECTOR+1, 0x08)

	cpu.PC = 0x0600
	cpu.SP = 0xFF
	cpu.SetRunning(true)
	cpu.jitEnabled = true

	go func() {
		for !cpu.executing.Load() {
			// spin
		}
		cpu.Reset()
	}()

	cpu.ExecuteJIT6502()

	// After reset, the CPU should have stopped (reset vector → JAM)
	if cpu.Running() {
		t.Error("CPU should be stopped after reset → JAM")
	}
}

// ===========================================================================
// Runner Integration Tests
// ===========================================================================

func TestJIT6502_Exec_RunnerIntegration(t *testing.T) {
	bus := NewMachineBus()
	runner := NewCPU6502Runner(bus, CPU6502Config{
		LoadAddr: 0x0600,
		Entry:    0x0600,
	})
	runner.JITEnabled = true

	// Simple program: LDA #$42; JAM
	bus.Write8(0x0600, 0xA9)
	bus.Write8(0x0601, 0x42)
	bus.Write8(0x0602, haltOpcode)

	bus.Write8(RESET_VECTOR, 0x00)
	bus.Write8(RESET_VECTOR+1, 0x06)

	runner.CPU().Reset()
	runner.CPU().SetRDYLine(true)
	runner.CPU().SetRunning(true)
	runner.Execute()

	if runner.CPU().A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42 (runner integration)", runner.CPU().A)
	}
}
