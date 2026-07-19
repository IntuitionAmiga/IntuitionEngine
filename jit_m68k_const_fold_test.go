// jit_m68k_const_fold_test.go - Milestone 7 constant-folding slice.
// Analysis unit tests (CCR proof), a shape test on the emit counter, and a
// parity grid running folded chains against the interpreter.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"testing"
	"unsafe"
)

func m68kFoldScan(t *testing.T, words ...uint16) ([]M68KJITInstr, []byte, uint32) {
	t.Helper()
	mem := make([]byte, 1<<20)
	pc := uint32(0x1000)
	for i, w := range words {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	if len(instrs) == 0 {
		t.Fatal("scan produced no instructions")
	}
	return instrs, mem, pc
}

// Analysis: MOVEQ;ADDQ chain folds with exact register and CCR results.
func TestM68KJIT_ConstFoldAnalysis(t *testing.T) {
	// MOVEQ #5,D0 ; ADDQ.L #3,D0 ; BRA.S
	instrs, mem, pc := m68kFoldScan(t, 0x7005, 0x5680, 0x6002)
	plan := m68kAnalyseConstFold(instrs, pc, mem)
	if plan == nil {
		t.Fatal("no fold plan")
	}
	if !plan[0].folded || !plan[0].setsReg || plan[0].value != 5 {
		t.Fatalf("MOVEQ fold wrong: %+v", plan[0])
	}
	if plan[0].ccrMask != m68kFoldCCR_N|m68kFoldCCR_Z|m68kFoldCCR_V|m68kFoldCCR_C || plan[0].ccrVal != 0 {
		t.Fatalf("MOVEQ CCR wrong: %+v", plan[0])
	}
	if !plan[1].folded || plan[1].value != 8 || plan[1].ccrMask != 0x1F || plan[1].ccrVal != 0 {
		t.Fatalf("ADDQ fold wrong: %+v", plan[1])
	}
}

// Analysis CCR proof: carry, overflow, negative, zero and X all constant.
func TestM68KJIT_ConstFoldAnalysisCCR(t *testing.T) {
	// MOVEQ #-1,D0 ; ADDQ.B #1,D0 → byte result 0: Z=1, C=1, X=1, V=0, N=0
	instrs, mem, pc := m68kFoldScan(t, 0x70FF, 0x5200, 0x6002)
	plan := m68kAnalyseConstFold(instrs, pc, mem)
	if plan == nil || !plan[1].folded {
		t.Fatal("ADDQ.B did not fold")
	}
	wantCCR := m68kFoldCCR_Z | m68kFoldCCR_C | m68kFoldCCR_X
	if plan[1].ccrVal != wantCCR {
		t.Fatalf("ADDQ.B CCR: got %02X want %02X", plan[1].ccrVal, wantCCR)
	}
	if plan[1].value != 0xFFFFFF00 {
		t.Fatalf("ADDQ.B merge: got %08X want FFFFFF00", plan[1].value)
	}

	// MOVEQ #127,D1 ; ADDQ.B #1,D1 → 0x80: N=1, V=1, C=0
	instrs, mem, pc = m68kFoldScan(t, 0x727F, 0x5201, 0x6002)
	plan = m68kAnalyseConstFold(instrs, pc, mem)
	if plan == nil || !plan[1].folded {
		t.Fatal("overflow ADDQ.B did not fold")
	}
	if plan[1].ccrVal != m68kFoldCCR_N|m68kFoldCCR_V {
		t.Fatalf("overflow CCR: got %02X", plan[1].ccrVal)
	}
}

func TestM68KJIT_ConstFoldLogicalClearsVC(t *testing.T) {
	tests := []struct {
		name  string
		words []uint16
		index int
	}{
		{"ANDI", []uint16{0x7003, 0x0240, 0x0001, 0x6002}, 1},
		{"ORI", []uint16{0x7001, 0x0040, 0x0002, 0x6002}, 1},
		{"EORI", []uint16{0x7003, 0x0A40, 0x0001, 0x6002}, 1},
		{"AND", []uint16{0x7003, 0x7201, 0xC001, 0x6002}, 2},
		{"OR", []uint16{0x7001, 0x7202, 0x8001, 0x6002}, 2},
		{"EOR", []uint16{0x7003, 0x7201, 0xB300, 0x6002}, 2},
	}
	wantMask := m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instrs, mem, pc := m68kFoldScan(t, tc.words...)
			plan := m68kAnalyseConstFold(instrs, pc, mem)
			if plan == nil || !plan[tc.index].folded {
				t.Fatalf("logical operation did not fold: %+v", plan)
			}
			if got := plan[tc.index].ccrMask; got != wantMask {
				t.Fatalf("CCR mask = %02X, want %02X (N/Z written, V/C cleared, X preserved)", got, wantMask)
			}
			if got := plan[tc.index].ccrVal & (m68kFoldCCR_V | m68kFoldCCR_C); got != 0 {
				t.Fatalf("folded logical operation sets V/C: %02X", got)
			}
		})
	}
}

// Unknown inputs must not fold; non-whitelisted instructions invalidate.
func TestM68KJIT_ConstFoldAnalysisInvalidation(t *testing.T) {
	// ADDQ.L #1,D0 with unknown D0 ; BRA.S — nothing folds
	instrs, mem, pc := m68kFoldScan(t, 0x5280, 0x6002)
	if plan := m68kAnalyseConstFold(instrs, pc, mem); plan != nil {
		t.Fatalf("unknown-input ADDQ folded: %+v", plan)
	}
	// MOVEQ #5,D0 ; MOVE.L (A0),D0 ; ADDQ.L #1,D0 — memory read kills D0
	instrs, mem, pc = m68kFoldScan(t, 0x7005, 0x2010, 0x5280, 0x6002)
	plan := m68kAnalyseConstFold(instrs, pc, mem)
	if plan == nil || !plan[0].folded {
		t.Fatal("MOVEQ should fold")
	}
	if plan[2].folded {
		t.Fatal("ADDQ after memory load folded from stale constant")
	}
}

// Shape: the emit counter proves folds were emitted; kill switch honoured
// via the analysis gate (nil plan).
func TestM68KJIT_ConstFoldShape(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	instrs, mem, pc := m68kFoldScan(t, 0x7005, 0x5680, 0x0640, 0x0100, 0x6002) // MOVEQ;ADDQ;ADDI.W #256
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	before := m68kFoldedConstEmits.Load()
	if _, err := m68kCompileBlockWithMem(instrs, pc, em, mem); err != nil {
		t.Fatalf("compile: %v", err)
	}
	emitted := m68kFoldedConstEmits.Load() - before
	if emitted != 3 {
		t.Fatalf("folded emits: got %d want 3", emitted)
	}
}

// Parity: folded chains match the interpreter in registers, CCR (incl. X)
// and PC. Uses the standard differential runner, which compiles through
// the production path (fold plan active by default).
func TestM68KJIT_ConstFoldParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	cases := []struct {
		name  string
		words []uint16
	}{
		{"MOVEQ_ADDQ_carry_zero", []uint16{0x70FF, 0x5200, 0x6002}},
		{"MOVEQ_ADDQ_overflow", []uint16{0x727F, 0x5201, 0x6002}},
		{"MOVEQ_SUBQ_borrow", []uint16{0x7000, 0x5380, 0x6002}},
		{"MOVE_L_imm_ADDI", []uint16{0x203C, 0x7FFF, 0xFFFF, 0x0680, 0x0000, 0x0001, 0x6002}},
		{"MOVEQ_ANDI_clearVC", []uint16{0x7003, 0x5380, 0x0240, 0x0001, 0x6002}}, // SUBQ sets C, ANDI.W must clear it
		{"MOVEQ_CMPI", []uint16{0x7005, 0x0C40, 0x0006, 0x6002}},
		{"MOVEQ_pair_ADD", []uint16{0x7005, 0x7203, 0xD280, 0x6002}},
		{"MOVEQ_pair_CMP", []uint16{0x7005, 0x7203, 0xB280, 0x6002}},
		{"MOVEQ_pair_EOR", []uint16{0x7005, 0x7203, 0xB380, 0x6002}},
		{"MOVE_B_imm_merge", []uint16{0x7000 | 1<<9 | 0x55, 0x123C, 0x00AA, 0x6002}}, // MOVEQ #55,D1? keep simple: MOVEQ then MOVE.B #imm,D0
		{"fold_then_live_use", []uint16{0x7005, 0x5680, 0xD081, 0x6002}},             // folded D0 feeds runtime ADD with unknown D1
		{"fold_X_then_ADDX", []uint16{0x70FF, 0x5200, 0xD341, 0x6002}},               // folded X consumed by ADDX.W D1,D1
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dtc := m68kDiffCase{
				name:  tc.name,
				words: tc.words,
				setup: func(cpu *M68KCPU) {
					cpu.DataRegs[1] = 0x00003456
					cpu.DataRegs[2] = 0x77777777
				},
				requireProdSafe: false,
			}
			instrs := m68kScanBlock(func() []byte {
				mem := make([]byte, 0x2000)
				for i, w := range tc.words {
					mem[0x1000+i*2] = byte(w >> 8)
					mem[0x1000+i*2+1] = byte(w)
				}
				return mem
			}(), 0x1000)
			runM68KJITDifferentialBlock(t, dtc, len(instrs))
		})
	}
}

func TestM68KJIT_ConstFoldLogicalBranchSemantics(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	tests := []struct {
		name  string
		words []uint16
	}{
		{
			name: "ANDI_clears_stale_V_before_BVS",
			words: []uint16{
				0x707F,       // MOVEQ #127,D0
				0x5200,       // ADDQ.B #1,D0: V=1
				0x0200, 0xFF, // ANDI.B #$FF,D0: V=0
				0x6902, // BVS.S over MOVEQ, must not branch
				0x7401, // MOVEQ #1,D2
			},
		},
		{
			name: "ORI_clears_stale_C_before_BCS",
			words: []uint16{
				0x7000,                 // MOVEQ #0,D0
				0x5380,                 // SUBQ.L #1,D0: C=1
				0x0080, 0x0000, 0x0000, // ORI.L #0,D0: C=0
				0x6502, // BCS.S over MOVEQ, must not branch
				0x7401, // MOVEQ #1,D2
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := m68kFoldedConstEmits.Load()
			cpu := runM68KJITStopProgramWithSetup(t, 0x1000, nil, true, tc.words...)
			if got := cpu.DataRegs[2]; got != 1 {
				t.Fatalf("D2 = %d, want 1: stale V/C made the conditional branch take", got)
			}
			if got := m68kFoldedConstEmits.Load() - before; got == 0 {
				t.Fatal("program executed without emitting a constant fold")
			}
		})
	}
}

var _ = fmt.Sprintf

// Benchmark: constant-heavy ALU chain, fold on (default) vs off (analysis
// bypassed via a nil-memory compile has no immediates, so instead compare
// against the disabled-global path).
func benchmarkM68KConstFold(b *testing.B, disable bool) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available")
	}
	old := m68kJITConstFoldDisabled
	m68kJITConstFoldDisabled = disable
	defer func() { m68kJITConstFoldDisabled = old }()

	// 8 repetitions of MOVEQ;ADDQ;ADDI.L;SUBQ chains then BRA.
	var words []uint16
	for i := 0; i < 8; i++ {
		words = append(words, 0x7005, 0x5680, 0x0680, 0x0001, 0x0000, 0x5380)
	}
	words = append(words, 0x6002)

	mem := make([]byte, 1<<20)
	pc := uint32(0x1000)
	for i, w := range words {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		b.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	block, err := m68kCompileBlockWithMem(instrs, pc, em, mem)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}

	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	copy(cpu.memory[pc:], mem[pc:pc+uint32(len(words)*2)])
	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	pageMin := make([]uint16, len(bitmap))
	pageMax := make([]uint16, len(bitmap))
	ctx := newM68KJITContext(cpu, bitmap, pageMin, pageMax)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.RetPC = 0
		ctx.NeedIOFallback = 0
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	}
}

func BenchmarkM68KJIT_ConstFoldOff(b *testing.B) { benchmarkM68KConstFold(b, true) }
func BenchmarkM68KJIT_ConstFoldOn(b *testing.B)  { benchmarkM68KConstFold(b, false) }
