// jit_6502_exec_test.go - Integration tests for the 6502 JIT dispatcher

//go:build (amd64 || arm64) && linux

package main

import (
	"testing"
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
