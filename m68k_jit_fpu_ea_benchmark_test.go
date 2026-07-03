//go:build amd64 && linux

package main

import (
	"math"
	"testing"
)

// FPU opcode-shape benchmarks. These mirror m68k_jit_opcode_benchmark_test.go
// but cover 68881 shapes whose EA/branch forms historically left the native
// block through the FPU helper (full epilogue + Go roundtrip per instruction).
// The JIT runner therefore does NOT assert zero helper/fallback usage — it
// only requires that native blocks executed — so the same harness measures
// both the helper baseline and the native EA implementation.

func m68kFPUBenchCases() []m68kOpcodeBenchCase {
	writeF32 := func(cpu *M68KCPU, addr uint32, v float32) {
		cpu.Write32(addr, math.Float32bits(v))
	}
	writeF64 := func(cpu *M68KCPU, addr uint32, v float64) {
		bits := math.Float64bits(v)
		cpu.Write32(addr, uint32(bits>>32))
		cpu.Write32(addr+4, uint32(bits))
	}
	return []m68kOpcodeBenchCase{
		{
			// Native reg-reg sanity anchor: FADD FP1,FP0.
			name:         "FPU_FADD_RegReg",
			body:         []uint16{0xF200, 0x0422},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.FPU.SetFP64(0, 0)
				cpu.FPU.SetFP64(1, 1.5)
			},
		},
		{
			// FMOVE.S (A0)+,FP0 — postincrement single-precision load.
			name:         "FPU_FMOVE_S_Postinc",
			body:         []uint16{0xF218, 0x4400},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = m68kBenchDataAddr
				writeF32(cpu, m68kBenchDataAddr, 1.25)
			},
		},
		{
			// FADD.D (A0),FP0 — double-precision memory operand.
			name:         "FPU_FADD_D_AnInd",
			body:         []uint16{0xF210, 0x5422},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = m68kBenchDataAddr
				cpu.FPU.SetFP64(0, 0)
				writeF64(cpu, m68kBenchDataAddr, 0.5)
			},
		},
		{
			// FMUL.S #2.5,FP0 — single-precision immediate operand.
			name:         "FPU_FMUL_S_Imm",
			body:         []uint16{0xF23C, 0x4423, 0x4020, 0x0000},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.FPU.SetFP64(0, 1.0)
			},
		},
		{
			// FMOVE.L D0,FP0 — integer register to float (gcc int->float idiom).
			name:         "FPU_FMOVE_L_Dn",
			body:         []uint16{0xF200, 0x4000},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 42
			},
		},
		{
			// FMOVE.S FP0,(A0) — single-precision store.
			name:         "FPU_FMOVE_S_ToMem",
			body:         []uint16{0xF210, 0x6400},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = m68kBenchDataAddr
				cpu.FPU.SetFP64(0, 3.75)
			},
		},
		{
			// FCMP FP1,FP0 then FBEQ.W (not taken) — FPU compare + branch.
			name:         "FPU_FCMP_FBcc",
			body:         []uint16{0xF200, 0x0438, 0xF281, 0x0004},
			instrPerIter: 3,
			setup: func(cpu *M68KCPU) {
				cpu.FPU.SetFP64(0, 1.0)
				cpu.FPU.SetFP64(1, 2.0)
			},
		},
		{
			// Horner-style reg-reg accumulate: five FADD FPn,FP0 in a row. Every
			// intermediate FPSR condition-code update is dead (each op's CC is
			// overwritten by the next reg-reg op), so lazy FPSR elides all but
			// the last setCC.
			name: "FPU_FADDChainRegReg",
			body: []uint16{
				0xF200, (1 << 10) | (0 << 7) | FPU_OP_FADD,
				0xF200, (2 << 10) | (0 << 7) | FPU_OP_FADD,
				0xF200, (3 << 10) | (0 << 7) | FPU_OP_FADD,
				0xF200, (4 << 10) | (0 << 7) | FPU_OP_FADD,
				0xF200, (5 << 10) | (0 << 7) | FPU_OP_FADD,
			},
			instrPerIter: 6,
			setup: func(cpu *M68KCPU) {
				for i := 0; i < 8; i++ {
					cpu.FPU.SetFP64(i, 1.0+float64(i)*0.5)
				}
			},
		},
		{
			// FSIN FP1,FP0 — a transcendental reg-reg op. Not natively emittable;
			// exercises the FPU helper path (pre-decoded descriptor vs full
			// re-fetch+decode).
			name:         "FPU_FSIN_RegReg",
			body:         []uint16{0xF200, (1 << 10) | (0 << 7) | FPU_OP_FSIN},
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.FPU.SetFP64(0, 0)
				cpu.FPU.SetFP64(1, 0.5)
			},
		},
		{
			// Dot-product step: FMOVE.S (A0)+,FP0; FMUL.S (A1)+,FP0; FADD FP0,FP1.
			name: "FPU_DotProduct",
			body: []uint16{
				0xF218, 0x4400, // FMOVE.S (A0)+,FP0
				0xF219, 0x4423, // FMUL.S (A1)+,FP0
				0xF200, 0x00A2, // FADD FP0,FP1
			},
			instrPerIter: 4,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = m68kBenchDataAddr
				cpu.AddrRegs[1] = m68kBenchDataAddr + 0x1000
				for i := uint32(0); i < uint32(m68kOpcodeBenchIterations); i++ {
					writeF32(cpu, m68kBenchDataAddr+i*4, 1.5)
					writeF32(cpu, m68kBenchDataAddr+0x1000+i*4, 2.0)
				}
				cpu.FPU.SetFP64(1, 0)
			},
		},
	}
}

func resetM68KFPUBenchCPU(cpu *M68KCPU, tc m68kOpcodeBenchCase) {
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0
		cpu.AddrRegs[i] = 0
	}
	for i := range 8 {
		cpu.FPU.SetFP64(i, 0)
	}
	cpu.FPU.FPCR = 0
	cpu.FPU.FPSR = 0
	cpu.FPU.FPIAR = 0
	cpu.SR = M68K_SR_S
	if tc.setup != nil {
		tc.setup(cpu)
	}
}

func BenchmarkM68K_FPUShape_Interpreter(b *testing.B) {
	for _, tc := range m68kFPUBenchCases() {
		b.Run(tc.name, func(b *testing.B) {
			cpu := setupM68KJITBenchCPU()
			startPC, endPC, totalInstrs := buildM68KOpcodeBenchProgram(cpu, tc)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resetM68KFPUBenchCPU(cpu, tc)
				runM68KBenchInterpreterUntilPC(cpu, startPC, endPC)
			}
			b.ReportMetric(float64(totalInstrs), "instructions/op")
			ReportMIPSHostNormalized(b, totalInstrs)
		})
	}
}

func BenchmarkM68K_FPUShape_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	for _, tc := range m68kFPUBenchCases() {
		b.Run(tc.name, func(b *testing.B) {
			cpu := setupM68KJITBenchCPU()
			startPC, endPC, totalInstrs := buildM68KOpcodeBenchProgram(cpu, tc)
			cpu.m68kJitEnabled = true
			cpu.m68kJitPersist = true

			resetM68KFPUBenchCPU(cpu, tc)
			runM68KBenchJITUntilPC(cpu, startPC, endPC)
			if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				b.Fatalf("%s executed no native M68K blocks", tc.name)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resetM68KFPUBenchCPU(cpu, tc)
				runM68KBenchJITUntilPC(cpu, startPC, endPC)
			}
			b.ReportMetric(float64(totalInstrs), "instructions/op")
			ReportMIPSHostNormalized(b, totalInstrs)
		})
	}
}
