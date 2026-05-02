//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

type z80TurboSnapshot struct {
	A, F, B, C, D, E, H, L byte
	SP, PC                 uint16
	Cycles                 uint64
	mem                    []byte
}

func runZ80TurboParityProgram(t *testing.T, program []byte, startPC uint16, turboEnv string, init func(*CPU_Z80, *MachineBus)) z80TurboSnapshot {
	t.Helper()
	if turboEnv != "" {
		t.Setenv("Z80_JIT_TURBO", turboEnv)
	}
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.SP = 0x1FFE
	for i, byt := range program {
		bus.Write8(uint32(startPC)+uint32(i), byt)
	}
	if init != nil {
		init(cpu, bus)
	}
	cpu.PC = startPC
	cpu.Halted = false
	cpu.SetRunning(true)
	cpu.ExecuteJITZ80()
	cpu.jitPersist = false
	cpu.freeZ80JIT()
	mem := append([]byte(nil), bus.GetMemory()[:0x2000]...)
	return z80TurboSnapshot{
		A: cpu.A, F: cpu.F, B: cpu.B, C: cpu.C, D: cpu.D, E: cpu.E, H: cpu.H, L: cpu.L,
		SP: cpu.SP, PC: cpu.PC, Cycles: cpu.Cycles, mem: mem,
	}
}

func runZ80InterpParityProgram(program []byte, startPC uint16, init func(*CPU_Z80, *MachineBus)) z80TurboSnapshot {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.SP = 0x1FFE
	for i, byt := range program {
		bus.Write8(uint32(startPC)+uint32(i), byt)
	}
	if init != nil {
		init(cpu, bus)
	}
	cpu.PC = startPC
	cpu.Halted = false
	cpu.SetRunning(true)
	for cpu.running.Load() && !cpu.Halted {
		cpu.Step()
	}
	cpu.SetRunning(false)
	mem := append([]byte(nil), bus.GetMemory()[:0x2000]...)
	return z80TurboSnapshot{
		A: cpu.A, F: cpu.F, B: cpu.B, C: cpu.C, D: cpu.D, E: cpu.E, H: cpu.H, L: cpu.L,
		SP: cpu.SP, PC: cpu.PC, Cycles: cpu.Cycles, mem: mem,
	}
}

func assertZ80TurboParity(t *testing.T, name string, program []byte, startPC uint16, init func(*CPU_Z80, *MachineBus)) {
	t.Helper()
	interp := runZ80InterpParityProgram(program, startPC, init)
	jitBase := runZ80TurboParityProgram(t, program, startPC, "0", init)
	jitTurbo := runZ80TurboParityProgram(t, program, startPC, "1", init)
	for label, got := range map[string]z80TurboSnapshot{"jit-base": jitBase, "jit-turbo": jitTurbo} {
		if got.A != interp.A || got.F != interp.F || got.B != interp.B || got.C != interp.C ||
			got.D != interp.D || got.E != interp.E || got.H != interp.H || got.L != interp.L ||
			got.SP != interp.SP || got.PC != interp.PC {
			t.Fatalf("%s %s regs mismatch: got A=%02X F=%02X B=%02X C=%02X D=%02X E=%02X H=%02X L=%02X SP=%04X PC=%04X cycles=%d want A=%02X F=%02X B=%02X C=%02X D=%02X E=%02X H=%02X L=%02X SP=%04X PC=%04X cycles=%d",
				name, label,
				got.A, got.F, got.B, got.C, got.D, got.E, got.H, got.L, got.SP, got.PC, got.Cycles,
				interp.A, interp.F, interp.B, interp.C, interp.D, interp.E, interp.H, interp.L, interp.SP, interp.PC, interp.Cycles)
		}
		for i := range interp.mem {
			if got.mem[i] != interp.mem[i] {
				t.Fatalf("%s %s mem[%04X]=%02X want %02X", name, label, i, got.mem[i], interp.mem[i])
			}
		}
	}
}

func TestZ80JITTurboBenchmarkParity(t *testing.T) {
	tests := []struct {
		name  string
		build func() ([]byte, uint16)
		init  func(*CPU_Z80, *MachineBus)
	}{
		{"ALU", buildZ80ALUProgram, func(cpu *CPU_Z80, _ *MachineBus) {
			cpu.A, cpu.C, cpu.D, cpu.E, cpu.H = 0x42, 0x5A, 0xF0, 0x0C, 0x11
		}},
		{"Memory", buildZ80MemoryProgram, func(_ *CPU_Z80, bus *MachineBus) {
			for i := 0; i < 256; i++ {
				bus.Write8(0x0500+uint32(i), byte(i^0xA5))
			}
		}},
		{"Mixed", buildZ80MixedProgram, func(cpu *CPU_Z80, bus *MachineBus) {
			cpu.C = 0xA7
			cpu.SP = 0x1FFE
			for i := 0; i < 256; i++ {
				bus.Write8(0x0500+uint32(i), byte(i*3))
			}
		}},
		{"Call", buildZ80CallProgram, func(cpu *CPU_Z80, _ *MachineBus) {
			cpu.A = 0x7E
			cpu.SP = 0x1FFE
		}},
	}
	for _, tt := range tests {
		program, startPC := tt.build()
		assertZ80TurboParity(t, tt.name, program, startPC, tt.init)
	}
}

func TestZ80JitExecuteDispatchInitializesJIT(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 JIT not available")
	}
	program, startPC := buildZ80ALUProgram()
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	for i, byt := range program {
		bus.Write8(uint32(startPC)+uint32(i), byt)
	}
	cpu.PC = startPC
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.SetRunning(true)
	cpu.z80JitExecute()
	defer func() {
		cpu.jitPersist = false
		cpu.freeZ80JIT()
	}()
	if cpu.jitExecMem == nil || cpu.jitCache == nil {
		t.Fatal("z80JitExecute did not initialize JIT state")
	}
}

func TestZ80JITTurboRejectsUnsafeRanges(t *testing.T) {
	program, startPC := buildZ80MemoryProgram()
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	for i, byt := range program {
		bus.Write8(uint32(startPC)+uint32(i), byt)
	}
	if err := cpu.initZ80JIT(adapter); err != nil {
		t.Fatal(err)
	}
	defer cpu.freeZ80JIT()
	mem := bus.GetMemory()
	cpu.directPageBitmap[0x05] = 1
	if tb := cpu.z80ProbeTurboBlock(startPC, adapter, mem); tb != nil {
		t.Fatalf("accepted turbo block with non-direct source page: %+v", tb)
	}
}
