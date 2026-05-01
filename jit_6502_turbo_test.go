//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func newP65TurboTestCPU() *CPU_6502 {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)
	cpu.initDirectPageBitmap()
	return cpu
}

func TestP65TurboAnalyze_AcceptsDirectXCountedLoop(t *testing.T) {
	cpu := newP65TurboTestCPU()
	instrs := []JIT6502Instr{
		{opcode: 0xA2, operand: 0x00, length: 2, pcOffset: 0},               // LDX #0
		{opcode: 0xB5, operand: 0x10, length: 2, pcOffset: 2},               // LDA $10,X
		{opcode: 0x95, operand: 0x80, length: 2, pcOffset: 4},               // STA $80,X
		{opcode: 0xE8, length: 1, pcOffset: 6},                              // INX
		{opcode: 0xD0, operand: uint16(byte(0xF9)), length: 2, pcOffset: 7}, // BNE $0602
	}
	plan, reason := p65AnalyzeTurboRegion(cpu, instrs, 0x0600)
	if reason != p65TurboRejectNone {
		t.Fatalf("p65AnalyzeTurboRegion rejected direct loop: %v", reason)
	}
	if plan.directMemoryProofs != 2 {
		t.Fatalf("directMemoryProofs=%d, want 2", plan.directMemoryProofs)
	}
	if !plan.loopSpecialized {
		t.Fatal("expected X counted-loop specialization")
	}
}

func TestP65TurboAnalyze_RejectsXLoopBranchAroundCounterSpecialization(t *testing.T) {
	cpu := newP65TurboTestCPU()
	instrs := []JIT6502Instr{
		{opcode: 0xA9, operand: 0x01, length: 2, pcOffset: 0},               // LDA #1
		{opcode: 0xD0, operand: 0x01, length: 2, pcOffset: 2},               // BNE skip
		{opcode: 0xCA, length: 1, pcOffset: 4},                              // DEX
		{opcode: 0xD0, operand: uint16(byte(0xF9)), length: 2, pcOffset: 5}, // BNE loop
	}
	plan, reason := p65AnalyzeTurboRegion(cpu, instrs, 0x0600)
	if reason != p65TurboRejectNone {
		t.Fatalf("p65AnalyzeTurboRegion rejected branchy loop: %v", reason)
	}
	if plan.loopSpecialized {
		t.Fatal("branch that can skip DEX must not be counted-loop specialized")
	}
	if p65IsBoundedCounterBranchTurbo(instrs, 3, 0) {
		t.Fatal("turbo bounded-loop proof accepted a branch-around-counter pattern")
	}
}

func TestP65TurboAnalyze_RejectsDecimalWhenDUnknown(t *testing.T) {
	cpu := newP65TurboTestCPU()
	cpu.SR |= DECIMAL_FLAG
	instrs := []JIT6502Instr{
		{opcode: 0x69, operand: 0x01, length: 2, pcOffset: 0}, // ADC #1
	}
	_, reason := p65AnalyzeTurboRegion(cpu, instrs, 0x0600)
	if reason != p65TurboRejectDecimal {
		t.Fatalf("reject reason=%v, want decimal", reason)
	}
}

func TestP65TurboAnalyze_RejectsTranslatedMemory(t *testing.T) {
	cpu := newP65TurboTestCPU()
	instrs := []JIT6502Instr{
		{opcode: 0xAD, operand: 0x2000, length: 3, pcOffset: 0}, // LDA $2000, bank window
	}
	_, reason := p65AnalyzeTurboRegion(cpu, instrs, 0x0600)
	if reason != p65TurboRejectMemory {
		t.Fatalf("reject reason=%v, want memory", reason)
	}
}

func TestP65TurboAnalyze_RejectsIndirectJump(t *testing.T) {
	cpu := newP65TurboTestCPU()
	instrs := []JIT6502Instr{
		{opcode: 0x6C, operand: 0x1000, length: 3, pcOffset: 0}, // JMP ($1000)
	}
	_, reason := p65AnalyzeTurboRegion(cpu, instrs, 0x0600)
	if reason != p65TurboRejectDynamicJump {
		t.Fatalf("reject reason=%v, want dynamic jump", reason)
	}
}
