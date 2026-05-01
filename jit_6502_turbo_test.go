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

func TestP65TurboFastLoops_Parity(t *testing.T) {
	if !jit6502Available {
		t.Skip("6502 JIT not available")
	}
	t.Setenv("P65_JIT_TURBO", "1")
	cases := []struct {
		name    string
		program []byte
	}{
		{"memory", bench6502MemProgram},
		{"call", bench6502CallProgram},
		{"branch", bench6502BranchProgram},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interpBus := NewMachineBus()
			interp := NewCPU_6502(interpBus)
			jitBus := NewMachineBus()
			jit := NewCPU_6502(jitBus)
			for i, b := range tc.program {
				interpBus.Write8(0x0600+uint32(i), b)
				jitBus.Write8(0x0600+uint32(i), b)
			}
			interp.PC = 0x0600
			jit.PC = 0x0600
			interp.SP = 0xFF
			jit.SP = 0xFF
			interp.SetRunning(true)
			jit.SetRunning(true)
			interp.Execute()
			jit.ExecuteJIT6502()
			if interp.PC != jit.PC || interp.A != jit.A || interp.X != jit.X || interp.Y != jit.Y || interp.SP != jit.SP || interp.SR != jit.SR || interp.Cycles != jit.Cycles {
				t.Fatalf("state mismatch: interp PC=%04X A=%02X X=%02X Y=%02X SP=%02X SR=%02X cycles=%d; jit PC=%04X A=%02X X=%02X Y=%02X SP=%02X SR=%02X cycles=%d",
					interp.PC, interp.A, interp.X, interp.Y, interp.SP, interp.SR, interp.Cycles,
					jit.PC, jit.A, jit.X, jit.Y, jit.SP, jit.SR, jit.Cycles)
			}
			for addr := uint16(0); addr < 0x0200; addr++ {
				if interp.fastAdapter.memDirect[addr] != jit.fastAdapter.memDirect[addr] {
					t.Fatalf("memory mismatch at %04X: interp=%02X jit=%02X", addr, interp.fastAdapter.memDirect[addr], jit.fastAdapter.memDirect[addr])
				}
			}
		})
	}
}
