//go:build amd64 && linux

package main

import "testing"

func run6502ProgramForInterpParity(t *testing.T, program []byte) (*CPU_6502, *MachineBus) {
	t.Helper()
	cpu, bus := setup6502BenchInterp(program, 0x0600)
	cpu.PC = 0x0600
	cpu.SP = 0xFF
	cpu.SetRunning(true)
	return cpu, bus
}

func assert6502BenchStateEqual(t *testing.T, wantCPU *CPU_6502, wantBus *MachineBus, gotCPU *CPU_6502, gotBus *MachineBus) {
	t.Helper()
	if gotCPU.PC != wantCPU.PC || gotCPU.X != wantCPU.X || gotCPU.Y != wantCPU.Y || gotCPU.SP != wantCPU.SP {
		t.Fatalf("cpu mismatch:\nwant PC=%04X X=%02X Y=%02X SP=%02X\ngot  PC=%04X X=%02X Y=%02X SP=%02X",
			wantCPU.PC, wantCPU.X, wantCPU.Y, wantCPU.SP,
			gotCPU.PC, gotCPU.X, gotCPU.Y, gotCPU.SP,
		)
	}
	_ = wantBus
	_ = gotBus
}

func Test6502ASMInterpreterBenchProgramParity(t *testing.T) {
	old := enable6502ASMInterpreter
	enable6502ASMInterpreter = true
	defer func() { enable6502ASMInterpreter = old }()

	cases := []struct {
		name    string
		program []byte
		init    func(*CPU_6502)
	}{
		{name: "ALU", program: bench6502ALUProgram, init: func(cpu *CPU_6502) { cpu.A, cpu.X, cpu.SR, cpu.Cycles = 0, 0, 0, 0 }},
		{name: "Memory", program: bench6502MemProgram, init: func(cpu *CPU_6502) { cpu.X, cpu.Cycles = 0, 0 }},
		{name: "Call", program: bench6502CallProgram, init: func(cpu *CPU_6502) { cpu.X, cpu.Y, cpu.SP, cpu.Cycles = 0, 0, 0xFF, 0 }},
		{name: "Branch", program: bench6502BranchProgram, init: func(cpu *CPU_6502) { cpu.X, cpu.Y, cpu.SR, cpu.Cycles = 0, 0, 0, 0 }},
		{name: "Mixed", program: bench6502MixedProgram, init: func(cpu *CPU_6502) { cpu.A, cpu.X, cpu.SP, cpu.SR, cpu.Cycles = 0, 0, 0xFF, 0, 0 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enable6502ASMInterpreter = false
			wantCPU, wantBus := run6502ProgramForInterpParity(t, tc.program)
			if tc.init != nil {
				tc.init(wantCPU)
			}
			wantCPU.Execute()

			enable6502ASMInterpreter = true
			gotCPU, gotBus := run6502ProgramForInterpParity(t, tc.program)
			if tc.init != nil {
				tc.init(gotCPU)
			}
			gotCPU.Execute()

			assert6502BenchStateEqual(t, wantCPU, wantBus, gotCPU, gotBus)
		})
	}
}
