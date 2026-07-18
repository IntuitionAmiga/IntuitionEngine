// jit_m68k_wasm_diff_test.go - differential tests for the minimal wasm M68020
// JIT backend (parity plan milestone 5).
//
// Each test assembles a short M68020 program, runs it on the interpreter and
// on the wasm-compiled block under wazero, and asserts the data/address
// registers, CCR, resume PC and retired count agree exactly. The harness
// mirrors the IE64 wasm differential harness (jit_wasm_ie64_diff_test.go) but
// against the M68K context layout and register files.

package main

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
)

const (
	m68kWasmTestPC       = uint32(0x1000)
	m68kWasmTestCtxOff   = uint32(0x4000)
	m68kWasmTestDRegsOff = uint32(0x4400)
	m68kWasmTestARegsOff = uint32(0x4440)
	m68kWasmTestSROff    = uint32(0x4480)
	m68kWasmTestFPRegs   = uint32(0x4500)
	m68kWasmTestFPSR     = uint32(0x4548)
	m68kWasmTestFPCR     = uint32(0x454C)
	m68kWasmTestFPIAR    = uint32(0x4550)
	m68kWasmTestGuestOff = uint32(0x8000)
	m68kWasmTestGuestLen = uint32(0x8000)
)

type m68kWasmState struct {
	dregs          [8]uint32
	aregs          [8]uint32
	sr             uint16
	pc             uint32
	count          uint32
	needIOFallback uint32
	guest          []byte // guest RAM readback (indexed by guest address)
	fp             [8]uint64
	fpsr           uint32
	fpiar          uint32
}

// m68kFPSeed holds an optional FPU seed for the differential harness.
type m68kFPSeed struct {
	fp   [8]uint64
	fpsr uint32
}

// runM68KWasmBlock compiles the supported prefix at m68kWasmTestPC and executes
// it under wazero, returning the resulting register state.
func runM68KWasmBlock(t *testing.T, program []byte, initD, initA [8]uint32, initSR uint16, prep func([]byte), fp *m68kFPSeed) m68kWasmState {
	t.Helper()

	// Scan and admit the same way the dispatcher will. `mem` is the guest
	// memory image indexed by guest address (guest address A maps to linear
	// MemBase+A), so the program sits at guest PC and MemSize == len(mem).
	mem := make([]byte, m68kWasmTestGuestLen)
	copy(mem[m68kWasmTestPC:], program)
	if prep != nil {
		prep(mem)
	}
	all := m68kScanBlock(mem, m68kWasmTestPC)
	// The scanner runs past the program into zero-filled RAM; keep only the
	// instructions that lie within the program image.
	var instrs []M68KJITInstr
	for i := range all {
		if all[i].pcOffset >= uint32(len(program)) {
			break
		}
		instrs = append(instrs, all[i])
	}
	prefix := m68kWasmSupportedPrefix(instrs, mem, m68kWasmTestPC)
	if prefix != len(instrs) {
		t.Fatalf("supported prefix = %d, want %d (whole program)", prefix, len(instrs))
	}
	modBytes, err := m68kWasmCompileBlock(instrs[:prefix], mem, m68kWasmTestPC)
	if err != nil {
		t.Fatalf("m68kWasmCompileBlock: %v", err)
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	envB := newWasmModuleBuilder()
	pages := uint32((uint64(m68kWasmTestGuestOff)+uint64(len(mem))+0xFFFF)/0x10000) + 1
	envB.defineMemory(pages)
	envB.exportMemory("mem")
	env, err := r.InstantiateWithConfig(ctx, envB.build(), wazero.NewModuleConfig().WithName("env"))
	if err != nil {
		t.Fatalf("instantiate env: %v", err)
	}
	lm := env.ExportedMemory("mem")

	// ctx pointer fields hold linear-memory offsets.
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffDataRegsPtr, m68kWasmTestDRegsOff)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffAddrRegsPtr, m68kWasmTestARegsOff)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffMemPtr, m68kWasmTestGuestOff)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffMemSize, uint32(len(mem)))
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffSRPtr, m68kWasmTestSROff)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffFPRegsPtr, m68kWasmTestFPRegs)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffFPSRPtr, m68kWasmTestFPSR)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffFPCRPtr, m68kWasmTestFPCR)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffFPIARPtr, m68kWasmTestFPIAR)
	if fp != nil {
		for i, v := range fp.fp {
			lm.WriteUint64Le(m68kWasmTestFPRegs+uint32(i)*8, v)
		}
		lm.WriteUint32Le(m68kWasmTestFPSR, fp.fpsr)
	}

	for i := 0; i < 8; i++ {
		lm.WriteUint32Le(m68kWasmTestDRegsOff+uint32(i)*4, initD[i])
		lm.WriteUint32Le(m68kWasmTestARegsOff+uint32(i)*4, initA[i])
	}
	lm.WriteUint16Le(m68kWasmTestSROff, initSR)
	lm.Write(m68kWasmTestGuestOff, mem)

	mod, err := r.Instantiate(ctx, modBytes)
	if err != nil {
		t.Fatalf("instantiate block: %v", err)
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(m68kWasmTestCtxOff)); err != nil {
		t.Fatalf("block call: %v", err)
	}

	var s m68kWasmState
	for i := 0; i < 8; i++ {
		s.dregs[i], _ = lm.ReadUint32Le(m68kWasmTestDRegsOff + uint32(i)*4)
		s.aregs[i], _ = lm.ReadUint32Le(m68kWasmTestARegsOff + uint32(i)*4)
	}
	s.sr, _ = lm.ReadUint16Le(m68kWasmTestSROff)
	s.pc, _ = lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffRetPC)
	s.count, _ = lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffRetCount)
	s.needIOFallback, _ = lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffNeedIOFallback)
	s.guest, _ = lm.Read(m68kWasmTestGuestOff, uint32(len(mem)))
	for i := 0; i < 8; i++ {
		s.fp[i], _ = lm.ReadUint64Le(m68kWasmTestFPRegs + uint32(i)*8)
	}
	s.fpsr, _ = lm.ReadUint32Le(m68kWasmTestFPSR)
	s.fpiar, _ = lm.ReadUint32Le(m68kWasmTestFPIAR)
	return s
}

// runM68KWasmInterp runs the same program on the interpreter.
func runM68KWasmInterp(t *testing.T, program []byte, initD, initA [8]uint32, initSR uint16, steps int, prep func([]byte), fp *m68kFPSeed) *M68KCPU {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	copy(cpu.memory[m68kWasmTestPC:], program)
	if prep != nil {
		prep(cpu.memory)
	}
	for i := 0; i < 8; i++ {
		cpu.DataRegs[i] = initD[i]
		cpu.AddrRegs[i] = initA[i]
	}
	if fp != nil && cpu.FPU != nil {
		for i, v := range fp.fp {
			cpu.FPU.SetFP64(i, math.Float64frombits(v))
		}
		cpu.FPU.FPSR = fp.fpsr
	}
	cpu.SR = initSR
	cpu.PC = m68kWasmTestPC
	// The minimal wasm backend does not yet enforce the stack floor/ceiling
	// (deferred to milestone 6, like the arm64 review fix). Relax the
	// interpreter's bounds so the differential compares the push/pop mechanics
	// against the small test RAM rather than the default 0x00FE0000 floor.
	cpu.stackLowerBound = 0
	cpu.stackUpperBound = 0xFFFFFFF0
	for i := 0; i < steps; i++ {
		cpu.StepOne()
	}
	return cpu
}

func m68kWasmWords(words ...uint16) []byte {
	out := make([]byte, 0, len(words)*2)
	for _, w := range words {
		out = append(out, byte(w>>8), byte(w))
	}
	return out
}

// m68kWasmDiff assembles words, runs both engines and compares register state.
func m68kWasmDiff(t *testing.T, name string, initD, initA [8]uint32, initSR uint16, steps int, words ...uint16) {
	t.Helper()
	m68kWasmDiffMem(t, name, initD, initA, initSR, steps, nil, 0, 0, words...)
}

// m68kWasmDiffMem is m68kWasmDiff with an optional guest-memory seed and an
// optional [dataLo,dataHi) window whose bytes are compared after execution
// (for store correctness). prep writes into the guest image (indexed by guest
// address) on both engines.
func m68kWasmDiffMem(t *testing.T, name string, initD, initA [8]uint32, initSR uint16, steps int, prep func([]byte), dataLo, dataHi uint32, words ...uint16) {
	t.Helper()
	program := m68kWasmWords(words...)
	interp := runM68KWasmInterp(t, program, initD, initA, initSR, steps, prep, nil)
	w := runM68KWasmBlock(t, program, initD, initA, initSR, prep, nil)

	for i := 0; i < 8; i++ {
		if interp.DataRegs[i] != w.dregs[i] {
			t.Errorf("%s: D%d interp=%08X wasm=%08X", name, i, interp.DataRegs[i], w.dregs[i])
		}
		if interp.AddrRegs[i] != w.aregs[i] {
			t.Errorf("%s: A%d interp=%08X wasm=%08X", name, i, interp.AddrRegs[i], w.aregs[i])
		}
	}
	if interp.SR&0x1F != w.sr&0x1F {
		t.Errorf("%s: CCR interp=%02X wasm=%02X", name, interp.SR&0x1F, w.sr&0x1F)
	}
	if interp.PC != w.pc {
		t.Errorf("%s: PC interp=%08X wasm=%08X", name, interp.PC, w.pc)
	}
	if int(w.count) != steps {
		t.Errorf("%s: RetCount wasm=%d want=%d", name, w.count, steps)
	}
	if dataHi > dataLo {
		for a := dataLo; a < dataHi; a++ {
			if interp.memory[a] != w.guest[a] {
				t.Errorf("%s: guest[%04X] interp=%02X wasm=%02X", name, a, interp.memory[a], w.guest[a])
				break
			}
		}
	}
}

var m68kWasmGrid = []uint32{
	0x00000000, 0x00000001, 0x00000080, 0x0000FFFF,
	0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0x12345678,
}

func TestM68KWasm_IntegerALUGrid(t *testing.T) {
	cases := []struct {
		name  string
		words []uint16
		count int
	}{
		{"ADD.L D0,D1", []uint16{0xD280}, 1},
		{"SUB.L D0,D1", []uint16{0x9280}, 1},
		{"CMP.L D0,D1", []uint16{0xB280}, 1},
		{"AND.L D0,D1", []uint16{0xC280}, 1},
		{"OR.L D0,D1", []uint16{0x8280}, 1},
		{"EOR.L D1,D0", []uint16{0xB380}, 1},
		{"ADD.W D0,D1", []uint16{0xD240}, 1},
		{"SUB.W D0,D1", []uint16{0x9240}, 1},
		{"ADD.B D0,D1", []uint16{0xD200}, 1},
		{"CMP.W D0,D1", []uint16{0xB240}, 1},
		{"AND.B D0,D1", []uint16{0xC200}, 1},
		{"MOVE.L D0,D2", []uint16{0x2400}, 1},
		{"MOVE.W D0,D2", []uint16{0x3400}, 1},
		{"MOVE.B D0,D2", []uint16{0x1400}, 1},
		{"MOVEA.L D0,A2", []uint16{0x2440}, 1},
		{"MOVEA.W D0,A2", []uint16{0x3440}, 1},
		{"TST.L D0", []uint16{0x4A80}, 1},
		{"TST.W D0", []uint16{0x4A40}, 1},
		{"TST.B D0", []uint16{0x4A00}, 1},
		{"CLR.L D3", []uint16{0x4283}, 1},
		{"CLR.W D3", []uint16{0x4243}, 1},
		{"CLR.B D3", []uint16{0x4203}, 1},
		{"MOVEQ #-1,D4", []uint16{0x78FF}, 1},
		{"MOVEQ #0,D4", []uint16{0x7800}, 1},
		{"MOVEQ #42,D4", []uint16{0x782A}, 1},
		{"ADDQ.L #8,D1", []uint16{0x5081}, 1},
		{"ADDQ.L #1,D1", []uint16{0x5281}, 1},
		{"SUBQ.L #1,D1", []uint16{0x5381}, 1},
		{"SUBQ.W #1,D1", []uint16{0x5341}, 1},
		{"ADDQ.L #4,A1", []uint16{0x5889}, 1},
		{"SUBQ.L #2,A1", []uint16{0x5589}, 1},
		{"ADDI.L #imm,D1", []uint16{0x0681, 0x8000, 0x0001}, 1},
		{"SUBI.W #imm,D1", []uint16{0x0441, 0x1234}, 1},
		{"ANDI.B #imm,D1", []uint16{0x0201, 0x00AA}, 1},
		{"CMPI.L #imm,D1", []uint16{0x0C81, 0x7FFF, 0xFFFF}, 1},
		{"NOP", []uint16{0x4E71}, 1},
		{"MOVE.L #imm,D5", []uint16{0x2A3C, 0x8000, 0x0001}, 1},
		{"chain ADD/CMP/MOVE/SUBQ", []uint16{0xD280, 0xB280, 0x2400, 0x5281}, 4},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, dv := range m68kWasmGrid {
				for _, sv := range m68kWasmGrid {
					var initD, initA [8]uint32
					initD[0] = sv
					initD[1] = dv
					initD[3] = dv
					for _, ccr := range []uint16{0x00, 0x1F} { // clear + all-set incoming CCR
						m68kWasmDiff(t, tc.name, initD, initA, 0x2000|ccr, tc.count, tc.words...)
					}
				}
			}
		})
	}
}

// seedMemPattern fills a guest window with a deterministic pattern so both
// engines read identical source data.
func seedMemPattern(lo, hi uint32) func([]byte) {
	return func(mem []byte) {
		for a := lo; a < hi && int(a) < len(mem); a++ {
			mem[a] = byte(a*7 + 0x11)
		}
	}
}

func TestM68KWasm_MemoryEAGrid(t *testing.T) {
	prep := seedMemPattern(0x5F00, 0x6200)
	cases := []struct {
		name  string
		words []uint16
		count int
		store bool
	}{
		{"MOVE.L (A0),D1", []uint16{0x2210}, 1, false},
		{"MOVE.W (A0),D1", []uint16{0x3210}, 1, false},
		{"MOVE.B (A0),D1", []uint16{0x1210}, 1, false},
		{"MOVE.L (A0)+,D1", []uint16{0x2218}, 1, false},
		{"MOVE.W (A0)+,D1", []uint16{0x3218}, 1, false},
		{"MOVE.L -(A6),D1", []uint16{0x2226}, 1, false},
		{"MOVE.L (8,A0),D1", []uint16{0x2228, 0x0008}, 1, false},
		{"MOVE.L (-4,A0),D1", []uint16{0x2228, 0xFFFC}, 1, false},
		{"MOVE.L (abs.W),D1", []uint16{0x2238, 0x6000}, 1, false},
		{"MOVE.L (abs.L),D1", []uint16{0x2239, 0x0000, 0x6000}, 1, false},
		{"MOVE.L (d16,PC),D1", []uint16{0x223A, 0x4FFE}, 1, false},
		{"ADD.L (A0),D1", []uint16{0xD290}, 1, false},
		{"SUB.W (A0),D1", []uint16{0x9250}, 1, false},
		{"CMP.L (A0),D1", []uint16{0xB290}, 1, false},
		{"AND.L (A0),D1", []uint16{0xC290}, 1, false},
		{"OR.B (A0),D1", []uint16{0x8210}, 1, false},
		{"TST.L (A0)", []uint16{0x4A90}, 1, false},
		{"TST.W (A0)", []uint16{0x4A50}, 1, false},
		{"MOVE.L D0,(A1)", []uint16{0x2280}, 1, true},
		{"MOVE.W D0,(A1)", []uint16{0x3280}, 1, true},
		{"MOVE.B D0,(A1)", []uint16{0x1280}, 1, true},
		{"MOVE.L D0,(A1)+", []uint16{0x22C0}, 1, true},
		{"MOVE.L D0,-(A6)", []uint16{0x2D00}, 1, true},
		{"MOVE.L D0,(8,A1)", []uint16{0x2940, 0x0008}, 1, true},
		{"CLR.L (A1)", []uint16{0x4290}, 1, true},
		{"CLR.W (A1)", []uint16{0x4250}, 1, true},
		{"CLR.B (A1)", []uint16{0x4210}, 1, true},
		{"chain MOVE (A0)+,D1 / ADD D1,D2 / MOVE.L D2,(A1)", []uint16{0x2218, 0xD481, 0x2282}, 3, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, d0 := range []uint32{0x00000000, 0xCAFEF00D, 0x80000000, 0x0000FFFF} {
				var initD, initA [8]uint32
				initD[0] = d0
				initD[1] = 0x11112222
				initD[2] = 0x0F0F0F0F
				initA[0] = 0x6000
				initA[1] = 0x6010
				initA[6] = 0x6100
				for _, ccr := range []uint16{0x00, 0x1F} {
					lo, hi := uint32(0x5F00), uint32(0x6200)
					m68kWasmDiffMem(t, tc.name, initD, initA, 0x2000|ccr, tc.count, prep, lo, hi, tc.words...)
				}
			}
		})
	}
}

// TestM68KWasm_MemoryIOBail proves a guarded access into an I/O page bails to
// the interpreter with the faulting PC and partial retired count, leaving the
// block's earlier instructions committed.
func TestM68KWasm_MemoryIOBail(t *testing.T) {
	// moveq #5,d2 ; move.l (a0),d1  where a0 points at the MMIO region.
	program := m68kWasmWords(0x7405, 0x2210)
	var initD, initA [8]uint32
	initA[0] = uint32(IO_REGION_START)
	w := runM68KWasmBlock(t, program, initD, initA, 0x2000, nil, nil)
	if w.needIOFallback == 0 {
		t.Fatalf("expected NeedIOFallback set on I/O access")
	}
	if w.count != 1 {
		t.Errorf("RetCount = %d, want 1 (moveq retired, move bailed)", w.count)
	}
	if w.pc != m68kWasmTestPC+2 {
		t.Errorf("bail PC = %08X, want %08X (the faulting move)", w.pc, m68kWasmTestPC+2)
	}
	if w.dregs[2] != 5 {
		t.Errorf("D2 = %08X, want 5 (moveq committed before bail)", w.dregs[2])
	}
}

func TestM68KWasm_BranchGrid(t *testing.T) {
	// Return address seed for RTS (big-endian 0x00002468 at the stack pointer).
	rtsPrep := func(mem []byte) {
		mem[0x7000] = 0x00
		mem[0x7001] = 0x00
		mem[0x7002] = 0x24
		mem[0x7003] = 0x68
	}
	cases := []struct {
		name  string
		words []uint16
		count int
		prep  func([]byte)
	}{
		{"BRA.W", []uint16{0x6000, 0x0FFE}, 1, nil},
		{"BRA.B", []uint16{0x600E}, 1, nil},
		{"BEQ.W", []uint16{0x6700, 0x0FFE}, 1, nil},
		{"BNE.W", []uint16{0x6600, 0x0FFE}, 1, nil},
		{"BGE.W", []uint16{0x6C00, 0x0FFE}, 1, nil},
		{"BLT.B", []uint16{0x6D0E}, 1, nil},
		{"BSR.W", []uint16{0x6100, 0x0FFE}, 1, nil},
		{"JMP (A1)", []uint16{0x4ED1}, 1, nil},
		{"JMP abs.W", []uint16{0x4EF8, 0x2000}, 1, nil},
		{"JMP (8,A1)", []uint16{0x4EE9, 0x0008}, 1, nil},
		{"JSR (A1)", []uint16{0x4E91}, 1, nil},
		{"RTS", []uint16{0x4E75}, 1, rtsPrep},
		{"moveq;BRA.W", []uint16{0x7203, 0x6000, 0x0FFE}, 2, nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for ccr := uint16(0); ccr < 0x20; ccr++ {
				var initD, initA [8]uint32
				initA[1] = 0x2000
				initA[7] = 0x7000
				m68kWasmDiffMem(t, tc.name, initD, initA, 0x2000|ccr, tc.count, tc.prep, 0x6F00, 0x7100, tc.words...)
			}
		})
	}
}

func TestM68KWasm_DBccGrid(t *testing.T) {
	cases := []struct {
		name  string
		words []uint16
	}{
		{"DBF D0", []uint16{0x51C8, 0xFFFE}},
		{"DBEQ D0", []uint16{0x57C8, 0xFFFE}},
		{"DBNE D0", []uint16{0x56C8, 0xFFFE}},
		{"DBF D3", []uint16{0x51CB, 0xFFFE}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, cv := range []uint32{0x00000000, 0x00000001, 0x00000002, 0x0000FFFF, 0x00010000, 0x0001FFFF, 0xABCD0000} {
				for _, ccr := range []uint16{0x00, 0x04, 0x1F} { // vary Z for DBEQ/DBNE
					var initD, initA [8]uint32
					initD[0] = cv
					initD[3] = cv
					m68kWasmDiffMem(t, tc.name, initD, initA, 0x2000|ccr, 1, nil, 0, 0, tc.words...)
				}
			}
		})
	}
}

// m68kWasmDiffFP runs an FPU program on both engines and compares the FP
// register file, FPSR condition codes, FPIAR and the integer state bit-exact.
func m68kWasmDiffFP(t *testing.T, name string, fp *m68kFPSeed, steps int, words ...uint16) {
	t.Helper()
	var initD, initA [8]uint32
	program := m68kWasmWords(words...)
	interp := runM68KWasmInterp(t, program, initD, initA, 0x2000, steps, nil, fp)
	w := runM68KWasmBlock(t, program, initD, initA, 0x2000, nil, fp)

	for i := 0; i < 8; i++ {
		want := math.Float64bits(interp.FPU.GetFP64(i))
		if want != w.fp[i] {
			t.Errorf("%s: FP%d interp=%016X wasm=%016X", name, i, want, w.fp[i])
		}
	}
	if interp.FPU.FPSR&0x0F000000 != w.fpsr&0x0F000000 {
		t.Errorf("%s: FPSR CC interp=%08X wasm=%08X", name, interp.FPU.FPSR&0x0F000000, w.fpsr&0x0F000000)
	}
	if interp.FPU.FPIAR != w.fpiar {
		t.Errorf("%s: FPIAR interp=%08X wasm=%08X", name, interp.FPU.FPIAR, w.fpiar)
	}
	if interp.PC != w.pc {
		t.Errorf("%s: PC interp=%08X wasm=%08X", name, interp.PC, w.pc)
	}
}

func TestM68KWasm_FPUGrid(t *testing.T) {
	vals := []float64{0.0, 1.0, -1.0, 2.5, -3.75, 1e300, -1e-300,
		math.Inf(1), math.Inf(-1), math.NaN(), 123456.789, -0.0}
	// cmdWord layout: 0 RM=0 SRC(10-12) DST(7-9) opmode(0-6).
	fpCase := func(src, dst, opmode int) []uint16 {
		return []uint16{0xF200, uint16(src<<10 | dst<<7 | opmode)}
	}
	ops := []struct {
		name   string
		opmode int
	}{
		{"FMOVE", 0x00},
		{"FADD", 0x22},
		{"FSUB", 0x28},
		{"FMUL", 0x23},
		{"FDIV", 0x20},
		{"FABS", 0x18},
		{"FNEG", 0x1A},
		{"FSQRT", 0x04},
		{"FCMP", 0x38},
		{"FTST", 0x3A},
		{"FADD.S", 0x62}, // single-precision result
		{"FADD.D", 0x66}, // double-precision result
		{"FMUL.S", 0x63},
	}
	for _, o := range ops {
		o := o
		t.Run(o.name, func(t *testing.T) {
			for _, a := range vals {
				for _, b := range vals {
					seed := &m68kFPSeed{}
					seed.fp[0] = math.Float64bits(a) // src FP0
					seed.fp[1] = math.Float64bits(b) // dst FP1
					m68kWasmDiffFP(t, o.name, seed, 1, fpCase(0, 1, o.opmode)...)
				}
			}
		})
	}
}
