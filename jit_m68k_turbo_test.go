// jit_m68k_turbo_test.go - parity tests for the M68020 turbo tier
//
//go:build amd64 && linux

package main

import (
	"bytes"
	"fmt"
	"testing"
)

type m68kTurboProgramBuilder func(*M68KCPU) (uint32, int)

func runM68KTurboParityInterpreter(cpu *M68KCPU, startPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)
	for cpu.running.Load() && !cpu.stopped.Load() {
		cpu.StepOne()
	}
	cpu.running.Store(false)
}

func runM68KTurboParityJIT(cpu *M68KCPU, startPC uint32, turbo bool) {
	old := m68kTurboDisabled
	m68kTurboDisabled = !turbo
	defer func() { m68kTurboDisabled = old }()

	cpu.m68kJitEnabled = true
	cpu.m68kJitForceNative = true
	runM68KBenchJIT(cpu, startPC)
}

func m68kTurboStateDiff(aName string, a *M68KCPU, bName string, b *M68KCPU) string {
	if a.PC != b.PC {
		return fmt.Sprintf("PC: %s=%08X %s=%08X", aName, a.PC, bName, b.PC)
	}
	if a.SR != b.SR {
		return fmt.Sprintf("SR: %s=%04X %s=%04X", aName, a.SR, bName, b.SR)
	}
	if a.running.Load() != b.running.Load() {
		return fmt.Sprintf("running: %s=%v %s=%v", aName, a.running.Load(), bName, b.running.Load())
	}
	if a.stopped.Load() != b.stopped.Load() {
		return fmt.Sprintf("stopped: %s=%v %s=%v", aName, a.stopped.Load(), bName, b.stopped.Load())
	}
	for i := range 8 {
		if a.DataRegs[i] != b.DataRegs[i] {
			return fmt.Sprintf("D%d: %s=%08X %s=%08X", i, aName, a.DataRegs[i], bName, b.DataRegs[i])
		}
		if a.AddrRegs[i] != b.AddrRegs[i] {
			return fmt.Sprintf("A%d: %s=%08X %s=%08X", i, aName, a.AddrRegs[i], bName, b.AddrRegs[i])
		}
	}
	return ""
}

func TestM68KTurboFocusedBenchParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available on this platform")
	}
	tests := []struct {
		name     string
		build    m68kTurboProgramBuilder
		memStart uint32
		memEnd   uint32
		init     func(*M68KCPU)
	}{
		{name: "ALU", build: buildM68KALUProgram, memStart: 0x1000, memEnd: 0x2100},
		{name: "MemCopy", build: buildM68KMemCopyProgram, memStart: m68kBenchDataAddr, memEnd: m68kBenchDataAddr + uint32(m68kBenchIterations)*8},
		{name: "Call", build: buildM68KCallProgram, memStart: 0xFFF0, memEnd: 0x10010, init: func(cpu *M68KCPU) { cpu.AddrRegs[7] = 0x10000 }},
		{name: "ChainBRA", build: buildM68KChainBRAProgram, memStart: 0x1000, memEnd: 0x3010, init: func(cpu *M68KCPU) { cpu.DataRegs[7] = uint32(m68kBenchIterations) }},
		{name: "LazyCCR", build: buildM68KLazyCCRProgram, memStart: 0x1000, memEnd: 0x1100, init: func(cpu *M68KCPU) {
			cpu.DataRegs[0] = 0
			cpu.DataRegs[1] = 0xFFFFFFFF
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			interp := setupM68KJITBenchCPU()
			jitBase := setupM68KJITBenchCPU()
			jitTurbo := setupM68KJITBenchCPU()

			startPC, _ := tc.build(interp)
			tc.build(jitBase)
			tc.build(jitTurbo)
			if tc.init != nil {
				tc.init(interp)
				tc.init(jitBase)
				tc.init(jitTurbo)
			}

			runM68KTurboParityInterpreter(interp, startPC)
			runM68KTurboParityJIT(jitBase, startPC, false)
			runM68KTurboParityJIT(jitTurbo, startPC, true)

			if diff := m68kTurboStateDiff("interp", interp, "jitBase", jitBase); diff != "" {
				t.Fatalf("interpreter vs turbo-disabled JIT mismatch: %s", diff)
			}
			if diff := m68kTurboStateDiff("jitBase", jitBase, "jitTurbo", jitTurbo); diff != "" {
				t.Fatalf("turbo-disabled JIT vs turbo mismatch: %s", diff)
			}
			if !bytes.Equal(interp.memory[tc.memStart:tc.memEnd], jitTurbo.memory[tc.memStart:tc.memEnd]) {
				t.Fatalf("memory mismatch in [%08X,%08X)", tc.memStart, tc.memEnd)
			}
		})
	}
}

func TestM68KTurboRejectsMemCopyMMIO(t *testing.T) {
	cpu := setupM68KJITBenchCPU()
	startPC, _ := buildM68KMemCopyProgram(cpu)
	cpu.PC = startPC + 16
	cpu.AddrRegs[0] = 0xA0000
	cpu.AddrRegs[1] = m68kBenchDataAddr
	cpu.DataRegs[7] = 3
	if _, ok := cpu.tryM68KTurboTrace(); ok {
		t.Fatalf("turbo accepted MMIO source range")
	}
}

func TestM68KTurboRejectsMemCopyOverlap(t *testing.T) {
	cpu := setupM68KJITBenchCPU()
	startPC, _ := buildM68KMemCopyProgram(cpu)
	cpu.PC = startPC + 16
	cpu.AddrRegs[0] = m68kBenchDataAddr
	cpu.AddrRegs[1] = m68kBenchDataAddr + 4
	cpu.DataRegs[7] = 3
	before := append([]byte(nil), cpu.memory[m68kBenchDataAddr:m68kBenchDataAddr+32]...)
	if _, ok := cpu.tryM68KTurboTrace(); ok {
		t.Fatalf("turbo accepted overlapping memcopy ranges")
	}
	if !bytes.Equal(before, cpu.memory[m68kBenchDataAddr:m68kBenchDataAddr+32]) {
		t.Fatalf("rejected overlapping memcopy mutated memory")
	}
}

func TestM68KTurboProgramRejectRestoresState(t *testing.T) {
	tests := []struct {
		name  string
		build func(*M68KCPU) uint32
	}{
		{
			name: "ALU bad loop",
			build: func(cpu *M68KCPU) uint32 {
				startPC, _ := buildM68KALUProgram(cpu)
				cpu.memory[startPC+8] = 0x4E
				cpu.memory[startPC+9] = 0x71
				return startPC
			},
		},
		{
			name: "MemCopy MMIO destination",
			build: func(cpu *M68KCPU) uint32 {
				startPC, _ := buildM68KMemCopyProgram(cpu)
				cpu.memory[startPC+8] = 0x00
				cpu.memory[startPC+9] = 0x0A
				cpu.memory[startPC+10] = 0x00
				cpu.memory[startPC+11] = 0x00
				return startPC
			},
		},
		{
			name: "Call bad leaf",
			build: func(cpu *M68KCPU) uint32 {
				startPC, _ := buildM68KCallProgram(cpu)
				cpu.memory[0x2002] = 0x4E
				cpu.memory[0x2003] = 0x71
				return startPC
			},
		},
		{
			name: "LazyCCR reachable branch",
			build: func(cpu *M68KCPU) uint32 {
				startPC, _ := buildM68KLazyCCRProgram(cpu)
				cpu.DataRegs[0] = 0
				cpu.DataRegs[1] = 1
				return startPC
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cpu := setupM68KJITBenchCPU()
			startPC := tc.build(cpu)
			cpu.PC = startPC
			cpu.SR = M68K_SR_S | M68K_SR_C | M68K_SR_X
			cpu.DataRegs[2] = 0xAABBCCDD
			cpu.DataRegs[7] = 0x12345678
			cpu.AddrRegs[0] = 0x11112222
			cpu.AddrRegs[1] = 0x33334444
			cpu.stopped.Store(true)
			snap := cpu.m68kTurboSnapshot()
			if _, ok := cpu.tryM68KTurboTrace(); ok {
				t.Fatalf("turbo accepted intentionally rejected trace")
			}
			if diff := m68kTurboSnapshotDiff(snap, cpu.m68kTurboSnapshot()); diff != "" {
				t.Fatalf("rejected trace mutated CPU state: %s", diff)
			}
		})
	}
}

func m68kTurboSnapshotDiff(a, b m68kTurboStateSnapshot) string {
	if a.pc != b.pc {
		return fmt.Sprintf("PC: %08X != %08X", a.pc, b.pc)
	}
	if a.sr != b.sr {
		return fmt.Sprintf("SR: %04X != %04X", a.sr, b.sr)
	}
	if a.stopped != b.stopped {
		return fmt.Sprintf("stopped: %v != %v", a.stopped, b.stopped)
	}
	for i := range 8 {
		if a.dataRegs[i] != b.dataRegs[i] {
			return fmt.Sprintf("D%d: %08X != %08X", i, a.dataRegs[i], b.dataRegs[i])
		}
		if a.addrRegs[i] != b.addrRegs[i] {
			return fmt.Sprintf("A%d: %08X != %08X", i, a.addrRegs[i], b.addrRegs[i])
		}
	}
	return ""
}

func TestM68KTurboRejectsLazyCCRTakenBEQ(t *testing.T) {
	cpu := setupM68KJITBenchCPU()
	startPC, _ := buildM68KLazyCCRProgram(cpu)
	cpu.PC = startPC + 4
	cpu.DataRegs[0] = 0
	cpu.DataRegs[1] = 1
	cpu.DataRegs[7] = 3
	if _, ok := cpu.tryM68KTurboTrace(); ok {
		t.Fatalf("turbo accepted LazyCCR loop with reachable BEQ")
	}
}
