//go:build amd64 && linux

package main

import "testing"

const m68kOpcodeBenchIterations = 512

type m68kOpcodeBenchCase struct {
	name         string
	body         []uint16
	instrPerIter int
	setup        func(*M68KCPU)
}

func m68kOpcodeBenchCases() []m68kOpcodeBenchCase {
	return []m68kOpcodeBenchCase{
		{
			name:         "MOVEQ_ADDQ_L",
			body:         []uint16{0x7001, 0x5280}, // MOVEQ #1,D0; ADDQ.L #1,D0
			instrPerIter: 3,
		},
		{
			name:         "MOVE_L_EA_To_Dn",
			body:         []uint16{0x2010}, // MOVE.L (A0),D0
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = m68kBenchDataAddr
				cpu.Write32(m68kBenchDataAddr, 0x11223344)
			},
		},
		{
			name:         "MOVE_B_Dn_To_Postinc",
			body:         []uint16{0x10C0}, // MOVE.B D0,(A0)+
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x80
				cpu.AddrRegs[0] = m68kBenchDataAddr
			},
		},
		{
			name:         "MOVE_W_Dn_To_Postinc",
			body:         []uint16{0x30C0}, // MOVE.W D0,(A0)+
			instrPerIter: 2,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x8001
				cpu.AddrRegs[0] = m68kBenchDataAddr
			},
		},
		{
			name: "ADD_SUB_CMP_L_EA",
			body: []uint16{
				0xD090, // ADD.L (A0),D0
				0x9090, // SUB.L (A0),D0
				0xB090, // CMP.L (A0),D0
			},
			instrPerIter: 4,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x1000
				cpu.AddrRegs[0] = m68kBenchDataAddr
				cpu.Write32(m68kBenchDataAddr, 3)
			},
		},
		{
			name: "Bcc_DBF",
			body: []uint16{
				0x5280, // ADDQ.L #1,D0
				0xB081, // CMP.L D1,D0
				0x6702, // BEQ.B +2 (not taken)
				0x7C00, // MOVEQ #0,D6
			},
			instrPerIter: 5,
			setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0
				cpu.DataRegs[1] = 0xFFFFFFFF
			},
		},
	}
}

func buildM68KOpcodeBenchProgram(cpu *M68KCPU, tc m68kOpcodeBenchCase) (uint32, uint32, int) {
	startPC := uint32(0x1000)
	pc := startPC
	write := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	write(0x3E3C, uint16(m68kOpcodeBenchIterations-1)) // MOVE.W #iterations-1,D7
	loopTop := pc
	write(tc.body...)
	disp := int16(int32(loopTop) - int32(pc) - 2)
	write(0x51CF, uint16(disp))       // DBF D7,loop_top
	bodyInstrs := tc.instrPerIter - 1 // instrPerIter includes DBF.
	fillerInstrs := m68kJitMaxBlockSize - 2 - bodyInstrs
	for range fillerInstrs {
		write(0x7C00) // MOVEQ #0,D6; keep STOP out of the first scanned native block.
	}
	endPC := pc

	return startPC, endPC, m68kOpcodeBenchIterations*tc.instrPerIter + fillerInstrs
}

func runM68KBenchInterpreterUntilPC(cpu *M68KCPU, startPC, endPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)
	for cpu.running.Load() && cpu.PC != endPC {
		cpu.StepOne()
	}
	cpu.running.Store(false)
	cpu.stopped.Store(false)
}

func runM68KBenchJITUntilPC(cpu *M68KCPU, startPC, endPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)
	cpu.debugBreakIn = func(pc uint64) bool {
		return uint32(pc) == endPC
	}
	cpu.M68KExecuteJIT()
	cpu.debugBreakIn = nil
	cpu.running.Store(false)
	cpu.stopped.Store(false)
}

func resetM68KOpcodeBenchCPU(cpu *M68KCPU, tc m68kOpcodeBenchCase) {
	for i := range cpu.DataRegs {
		cpu.DataRegs[i] = 0
		cpu.AddrRegs[i] = 0
	}
	cpu.SR = M68K_SR_S
	if tc.setup != nil {
		tc.setup(cpu)
	}
}

func BenchmarkM68K_OpcodeShape_Interpreter(b *testing.B) {
	for _, tc := range m68kOpcodeBenchCases() {
		b.Run(tc.name, func(b *testing.B) {
			cpu := setupM68KJITBenchCPU()
			startPC, endPC, totalInstrs := buildM68KOpcodeBenchProgram(cpu, tc)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resetM68KOpcodeBenchCPU(cpu, tc)
				runM68KBenchInterpreterUntilPC(cpu, startPC, endPC)
			}
			b.ReportMetric(float64(totalInstrs), "instructions/op")
			ReportMIPSHostNormalized(b, totalInstrs)
		})
	}
}

func BenchmarkM68K_OpcodeShape_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	for _, tc := range m68kOpcodeBenchCases() {
		b.Run(tc.name, func(b *testing.B) {
			cpu := setupM68KJITBenchCPU()
			startPC, endPC, totalInstrs := buildM68KOpcodeBenchProgram(cpu, tc)
			cpu.m68kJitEnabled = true
			cpu.m68kJitPersist = true

			resetM68KOpcodeBenchCPU(cpu, tc)
			runM68KBenchJITUntilPC(cpu, startPC, endPC)
			if got := cpu.m68kJitNativeBlocksExecuted.Load(); got == 0 {
				b.Fatalf("%s executed no native M68K blocks", tc.name)
			}

			resetM68KOpcodeBenchCPU(cpu, tc)
			runM68KBenchJITUntilPC(cpu, startPC, endPC)
			if got := cpu.m68kJitBailoutCount.Load(); got != 0 {
				b.Fatalf("%s bailed out %d times during hot-cache validation", tc.name, got)
			}
			if got := cpu.m68kJitFallbackInstructions.Load(); got != 0 {
				instrs := m68kScanBlock(cpu.memory, startPC)
				firstUnsafe := uint16(0)
				firstUnsafeIndex := -1
				for i := range instrs {
					if !m68kInstrProductionNativeSafe(&instrs[i]) {
						firstUnsafe = instrs[i].opcode
						firstUnsafeIndex = i
						break
					}
				}
				b.Fatalf("%s executed %d fallback instructions during hot-cache validation: lastFallbackPC=0x%08X lastOpcode=0x%04X instrs=%d fallback=%v conservative=%v productionSafe=%v genericIO=%v firstUnsafe[%d]=0x%04X",
					tc.name, got, cpu.lastExecPC, cpu.lastExecOpcode,
					len(instrs), m68kNeedsFallback(instrs),
					m68kNeedsConservativeFallback(cpu.memory, startPC, instrs),
					m68kBlockProductionNativeSafe(instrs),
					m68kBlockMayUseGenericIOFallback(instrs),
					firstUnsafeIndex, firstUnsafe)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resetM68KOpcodeBenchCPU(cpu, tc)
				runM68KBenchJITUntilPC(cpu, startPC, endPC)
			}
			b.ReportMetric(float64(totalInstrs), "instructions/op")
			ReportMIPSHostNormalized(b, totalInstrs)
		})
	}
}
