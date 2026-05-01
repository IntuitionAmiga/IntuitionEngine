// jit_x86_turbo_test.go - parity tests for x86 counted-loop turbo tier
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func x86TurboTestCPU(t *testing.T, code []byte) (*CPU_X86, *MachineBus) {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.x86JitCodeBM = make([]byte, len(cpu.x86JitIOBitmap))
	loadX86BenchProgram(cpu, 0x1000, code)
	resetX86BenchState(cpu, 0x1000)
	return cpu, bus
}

func x86TurboRunFromLoop(t *testing.T, code []byte, setupSteps int) *CPU_X86 {
	t.Helper()
	cpu, _ := x86TurboTestCPU(t, code)
	for i := 0; i < setupSteps; i++ {
		cpu.Step()
	}
	cpu.syncJITRegsFromNamed()
	retired, ok := cpu.tryX86TurboTrace()
	if !ok || retired == 0 {
		t.Fatalf("turbo did not accept loop at EIP=%08X", cpu.EIP)
	}
	cpu.syncJITRegsToNamed()
	for cpu.Running() && !cpu.Halted {
		cpu.Step()
	}
	return cpu
}

func x86TurboCompareCPU(t *testing.T, name string, interp, turbo *CPU_X86, memLo, memHi uint32) {
	t.Helper()
	regs := [...]struct {
		name string
		a    uint32
		b    uint32
	}{
		{"EAX", interp.EAX, turbo.EAX}, {"ECX", interp.ECX, turbo.ECX},
		{"EDX", interp.EDX, turbo.EDX}, {"EBX", interp.EBX, turbo.EBX},
		{"ESP", interp.ESP, turbo.ESP}, {"EBP", interp.EBP, turbo.EBP},
		{"ESI", interp.ESI, turbo.ESI}, {"EDI", interp.EDI, turbo.EDI},
		{"EIP", interp.EIP, turbo.EIP}, {"Flags", interp.Flags, turbo.Flags},
	}
	for _, r := range regs {
		if r.a != r.b {
			t.Fatalf("%s %s: interp=%08X turbo=%08X", name, r.name, r.a, r.b)
		}
	}
	if interp.Halted != turbo.Halted || interp.Running() != turbo.Running() {
		t.Fatalf("%s run state: interp halted=%v running=%v turbo halted=%v running=%v",
			name, interp.Halted, interp.Running(), turbo.Halted, turbo.Running())
	}
	for addr := memLo; addr < memHi; addr++ {
		if interp.memory[addr] != turbo.memory[addr] {
			t.Fatalf("%s memory[%08X]: interp=%02X turbo=%02X",
				name, addr, interp.memory[addr], turbo.memory[addr])
		}
	}
}

func TestX86JITTurbo_ALUCountedLoopParity(t *testing.T) {
	code, _ := buildX86ALUProgram(37)
	interp := runX86InterpreterProgram(t, 0x1000, code...)
	turbo := x86TurboRunFromLoop(t, code, 3)
	x86TurboCompareCPU(t, "alu", interp, turbo, x86BenchDataAddr, x86BenchDataAddr+64)
}

func TestX86JITTurbo_DirectMemoryLoopParity(t *testing.T) {
	code, _ := buildX86MemoryProgram(37)
	interp := runX86InterpreterProgram(t, 0x1000, code...)
	turbo := x86TurboRunFromLoop(t, code, 3)
	x86TurboCompareCPU(t, "memory", interp, turbo, x86BenchDataAddr, x86BenchDataAddr+37*4)
}

func TestX86JITTurbo_MixedLoopParity(t *testing.T) {
	code, _ := buildX86MixedProgram(37)
	interp := runX86InterpreterProgram(t, 0x1000, code...)
	turbo := x86TurboRunFromLoop(t, code, 4)
	x86TurboCompareCPU(t, "mixed", interp, turbo, x86BenchDataAddr, x86BenchDataAddr+37*4)
}

func TestX86JITTurbo_StaticLeafCallLoopParity(t *testing.T) {
	code, _ := buildX86CallProgram(37)
	interp := runX86InterpreterProgram(t, 0x1000, code...)
	turbo := x86TurboRunFromLoop(t, code, 3)
	x86TurboCompareCPU(t, "call", interp, turbo, x86BenchStackAddr-4, x86BenchStackAddr)
}

func TestX86JITTurbo_RejectsMMIOAndCodeOverlap(t *testing.T) {
	code, _ := buildX86MemoryProgram(3)
	cpu, _ := x86TurboTestCPU(t, code)
	for i := 0; i < 3; i++ {
		cpu.Step()
	}
	cpu.syncJITRegsFromNamed()
	cpu.jitRegs[6] = 0xF000
	if _, ok := cpu.tryX86TurboTrace(); ok {
		t.Fatal("turbo accepted MMIO memory loop")
	}

	cpu.jitRegs[6] = 0x5000
	cpu.x86JitCodeBM[0x50] = 1
	if _, ok := cpu.tryX86TurboTrace(); ok {
		t.Fatal("turbo accepted code-overlapping store loop")
	}
}
