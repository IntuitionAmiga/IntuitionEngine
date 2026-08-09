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
	m68kWasmTestStackLo  = uint32(0x4560) // cell holding cpu.stackLowerBound
	m68kWasmTestStackHi  = uint32(0x4564) // cell holding cpu.stackUpperBound
	m68kWasmTestCodeBmp  = uint32(0x4600) // exact code-byte map (SMC tests)
	m68kWasmTestCodeLeaf = uint32(0x5000)
	m68kWasmTestGuestOff = uint32(0x8000)
	m68kWasmTestGuestLen = uint32(0x8000)
)

// m68kWasmTestStackBounds is the stack floor/ceiling pair the harness installs
// in the wasm context and the interpreter alike. Individual tests override and
// restore it to exercise the milestone 6 stack-bound guards.
var m68kWasmTestStackBounds = [2]uint32{0, 0xFFFFFFF0}

// m68kWasmTestReinvoke, when true, makes the harness re-enter a block that
// exits at its own start PC (a budget-bounded loop), accumulating RetCount
// across invocations, exactly as the dispatcher does.
var m68kWasmTestReinvoke = false

// m68kWasmTestCodePages, when true, marks the program's exact first byte so
// stores to compiled code set NeedInval (loop SMC tests).
var m68kWasmTestCodePages = false

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
	fpcr uint32
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
	all = m68kFuseJSRLeafCalls(all, m68kWasmTestPC, mem, uint32(len(mem)))
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
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffStackLowerBoundPtr, m68kWasmTestStackLo)
	lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffStackUpperBoundPtr, m68kWasmTestStackHi)
	lm.WriteUint32Le(m68kWasmTestStackLo, m68kWasmTestStackBounds[0])
	lm.WriteUint32Le(m68kWasmTestStackHi, m68kWasmTestStackBounds[1])
	if m68kWasmTestCodePages {
		lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffCodePageMinPtr, m68kWasmTestCodeBmp)
		lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffCodePageBoundsLen, (uint32(len(mem))+4095)>>12)
		page := m68kWasmTestPC >> 12
		lm.WriteUint32Le(m68kWasmTestCodeBmp+page*4, m68kWasmTestCodeLeaf)
		off := m68kWasmTestPC & 0xFFF
		lm.WriteUint32Le(m68kWasmTestCodeLeaf+(off>>5)*4, 1<<(off&31))
	}
	if fp != nil {
		for i, v := range fp.fp {
			lm.WriteUint64Le(m68kWasmTestFPRegs+uint32(i)*8, v)
		}
		lm.WriteUint32Le(m68kWasmTestFPSR, fp.fpsr)
		lm.WriteUint32Le(m68kWasmTestFPCR, fp.fpcr)
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
	blockFn := mod.ExportedFunction("block")
	if _, err := blockFn.Call(ctx, uint64(m68kWasmTestCtxOff)); err != nil {
		t.Fatalf("block call: %v", err)
	}
	totalCount, _ := lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffRetCount)
	if m68kWasmTestReinvoke {
		// Emulate the dispatcher's loop: a budget-bounded loop block exits at
		// its own head; re-enter until it leaves (or bails), accumulating the
		// retired count across invocations.
		for i := 0; i < 512; i++ {
			pc, _ := lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffRetPC)
			fb, _ := lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffNeedIOFallback)
			if pc != m68kWasmTestPC || fb != 0 {
				break
			}
			lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffRetCount, 0)
			lm.WriteUint32Le(m68kWasmTestCtxOff+m68kCtxOffNeedIOFallback, 0)
			if _, err := blockFn.Call(ctx, uint64(m68kWasmTestCtxOff)); err != nil {
				t.Fatalf("block re-invoke: %v", err)
			}
			c, _ := lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffRetCount)
			totalCount += c
		}
	}

	var s m68kWasmState
	for i := 0; i < 8; i++ {
		s.dregs[i], _ = lm.ReadUint32Le(m68kWasmTestDRegsOff + uint32(i)*4)
		s.aregs[i], _ = lm.ReadUint32Le(m68kWasmTestARegsOff + uint32(i)*4)
	}
	s.sr, _ = lm.ReadUint16Le(m68kWasmTestSROff)
	s.pc, _ = lm.ReadUint32Le(m68kWasmTestCtxOff + m68kCtxOffRetPC)
	s.count = totalCount
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
		cpu.FPU.FPCR = fp.fpcr
	}
	cpu.SR = initSR
	cpu.PC = m68kWasmTestPC
	// Both engines run under the same stack floor/ceiling; the harness default
	// relaxes the interpreter's 0x00FE0000 floor to fit the small test RAM,
	// and the milestone 6 stack-bound tests override the pair.
	cpu.stackLowerBound = m68kWasmTestStackBounds[0]
	cpu.stackUpperBound = m68kWasmTestStackBounds[1]
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

func TestM68KWasm_JSRLeafFusionParity(t *testing.T) {
	const leaf = uint32(0x1800)
	prep := func(mem []byte) {
		mem[leaf] = 0x70 // MOVEQ #5,D0
		mem[leaf+1] = 0x05
		mem[leaf+2] = 0x4E // RTS
		mem[leaf+3] = 0x75
	}
	var initD, initA [8]uint32
	initA[7] = 0x7000
	m68kWasmDiffMem(t, "fused JSR leaf", initD, initA, 0x201F, 5, prep, 0x6FF0, 0x7010,
		0x4EB9, uint16(leaf>>16), uint16(leaf), // JSR leaf
		0x7207, // MOVEQ #7,D1 after return
		0x6002, // BRA.S terminator
	)
}

func TestM68KWasm_FusedLeafRTSBailPreservesCommittedEffects(t *testing.T) {
	const (
		leaf  = uint32(0x1800)
		stack = uint32(0x7004)
	)
	saved := m68kWasmTestStackBounds
	m68kWasmTestStackBounds = [2]uint32{0, stack - 4}
	defer func() { m68kWasmTestStackBounds = saved }()
	prep := func(mem []byte) {
		mem[leaf], mem[leaf+1] = 0x70, 0x05
		mem[leaf+2], mem[leaf+3] = 0x4E, 0x75
	}
	var initD, initA [8]uint32
	initA[7] = stack
	got := runM68KWasmBlock(t, m68kWasmWords(
		0x4EB9, uint16(leaf>>16), uint16(leaf), 0x7207, 0x6002), initD, initA, 0x201F, prep, nil)
	if got.needIOFallback == 0 || got.pc != leaf+2 || got.count != 2 {
		t.Fatalf("synthetic RTS bail: NeedIO=%d RetPC=%08X RetCount=%d", got.needIOFallback, got.pc, got.count)
	}
	if got.dregs[0] != 5 || got.aregs[7] != stack-4 {
		t.Fatalf("committed effects lost: D0=%08X A7=%08X", got.dregs[0], got.aregs[7])
	}
}

func TestM68KWasm_ConstFoldShapeAndParity(t *testing.T) {
	var initD, initA [8]uint32
	before := m68kFoldedConstEmits.Load()
	m68kWasmDiffMem(t, "constant fold", initD, initA, 0x201F, 3, nil, 0, 0,
		0x7005, 0x5680, 0x6002)
	if got := m68kFoldedConstEmits.Load() - before; got != 2 {
		t.Fatalf("wasm folded emits = %d, want 2", got)
	}
}

// m68kWasmDiffLoop runs a (possibly self-looping) block with dispatcher-style
// re-invocation on the wasm side, then drives the interpreter for exactly the
// number of instructions the block retired, and compares the full state. This
// covers the milestone 6 structured in-block loops, whose retired count is
// dynamic, including the budget exit at the loop head.
func m68kWasmDiffLoop(t *testing.T, name string, initD, initA [8]uint32, initSR uint16, prep func([]byte), dataLo, dataHi uint32, words ...uint16) {
	t.Helper()
	saved := m68kWasmTestReinvoke
	m68kWasmTestReinvoke = true
	defer func() { m68kWasmTestReinvoke = saved }()

	program := m68kWasmWords(words...)
	w := runM68KWasmBlock(t, program, initD, initA, initSR, prep, nil)
	if w.needIOFallback != 0 {
		t.Fatalf("%s: unexpected NeedIOFallback", name)
	}
	if w.count == 0 {
		t.Fatalf("%s: zero retired count", name)
	}
	interp := runM68KWasmInterp(t, program, initD, initA, initSR, int(w.count), prep, nil)

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
		t.Errorf("%s: PC interp=%08X wasm=%08X (retired=%d)", name, interp.PC, w.pc, w.count)
	}
	for a := dataLo; a < dataHi; a++ {
		if interp.memory[a] != w.guest[a] {
			t.Errorf("%s: guest[%04X] interp=%02X wasm=%02X", name, a, interp.memory[a], w.guest[a])
			break
		}
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
					// These single-instruction DBcc blocks target their own
					// start PC, so milestone 6 compiles them as structured
					// in-block loops with a dynamic retired count.
					m68kWasmDiffLoop(t, tc.name, initD, initA, 0x2000|ccr, nil, 0, 0, tc.words...)
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

// ---------------------------------------------------------------------------
// Milestone 6
// ---------------------------------------------------------------------------

// TestM68KWasm_RMWMemGrid covers the read-modify-write ALU forms with a memory
// destination: ADD/SUB/AND/OR/EOR Dn,<ea> and ADDQ/SUBQ #q,<ea>.
func TestM68KWasm_RMWMemGrid(t *testing.T) {
	prep := seedMemPattern(0x5F00, 0x6200)
	cases := []struct {
		name  string
		words []uint16
		count int
	}{
		{"ADD.L D0,(A1)", []uint16{0xD191}, 1},
		{"ADD.W D0,(A1)", []uint16{0xD151}, 1},
		{"ADD.B D0,(A1)", []uint16{0xD111}, 1},
		{"SUB.L D0,(A1)", []uint16{0x9191}, 1},
		{"SUB.W D0,(A1)+", []uint16{0x9159}, 1},
		{"SUB.B D0,-(A6)", []uint16{0x9126}, 1},
		{"AND.L D0,(A1)", []uint16{0xC191}, 1},
		{"AND.B D0,(8,A1)", []uint16{0xC129, 0x0008}, 1},
		{"OR.W D0,(A1)", []uint16{0x8151}, 1},
		{"OR.L D0,(abs.W)", []uint16{0x81B8, 0x6000}, 1},
		{"EOR.L D0,(A1)", []uint16{0xB191}, 1},
		{"EOR.W D0,(A1)+", []uint16{0xB159}, 1},
		{"EOR.B D0,(-4,A1)", []uint16{0xB129, 0xFFFC}, 1},
		{"ADDQ.L #1,(A1)", []uint16{0x5291}, 1},
		{"ADDQ.W #8,(A1)", []uint16{0x5051}, 1},
		{"ADDQ.B #4,(A1)+", []uint16{0x5819}, 1},
		{"SUBQ.L #2,(A1)", []uint16{0x5591}, 1},
		{"SUBQ.W #1,-(A6)", []uint16{0x5366}, 1},
		{"SUBQ.B #3,(8,A1)", []uint16{0x5729, 0x0008}, 1},
		{"ADDQ.L #1,(abs.W)", []uint16{0x52B8, 0x6000}, 1},
		{"chain ADD.L D0,(A1)/BEQ", []uint16{0xD191, 0x6702}, 2},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, d0 := range []uint32{0x00000000, 0x00000001, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0xCAFEF00D} {
				var initD, initA [8]uint32
				initD[0] = d0
				initA[1] = 0x6000
				initA[6] = 0x6100
				for _, ccr := range []uint16{0x00, 0x1F} {
					m68kWasmDiffMem(t, tc.name, initD, initA, 0x2000|ccr, tc.count, prep, 0x5F00, 0x6200, tc.words...)
				}
			}
		})
	}
}

// TestM68KWasm_StackBounds proves the milestone 6 stack floor/ceiling guards:
// a push that would cross the floor and a pop at or above the ceiling bail to
// the interpreter before any side effect, matching Push32/Pop32 bus errors.
func TestM68KWasm_StackBounds(t *testing.T) {
	saved := m68kWasmTestStackBounds
	defer func() { m68kWasmTestStackBounds = saved }()

	t.Run("BSR under floor bails", func(t *testing.T) {
		m68kWasmTestStackBounds = [2]uint32{0x7000, 0xFFFFFFF0}
		program := m68kWasmWords(0x7405, 0x6100, 0x0FFC) // moveq #5,d2 ; bsr.w
		var initD, initA [8]uint32
		initA[7] = 0x7002 // push would land at 0x6FFE < floor
		w := runM68KWasmBlock(t, program, initD, initA, 0x2000, nil, nil)
		if w.needIOFallback == 0 {
			t.Fatalf("expected NeedIOFallback on stack-floor violation")
		}
		if w.count != 1 || w.pc != m68kWasmTestPC+2 {
			t.Errorf("bail count=%d pc=%08X, want 1 / %08X", w.count, w.pc, m68kWasmTestPC+2)
		}
		if w.aregs[7] != 0x7002 {
			t.Errorf("A7 changed to %08X on bailed push", w.aregs[7])
		}
	})

	t.Run("JSR wrap bails", func(t *testing.T) {
		m68kWasmTestStackBounds = [2]uint32{0, 0xFFFFFFF0}
		program := m68kWasmWords(0x4EA9, 0x0008) // jsr (8,A1)
		var initD, initA [8]uint32
		initA[1] = 0x2000
		initA[7] = 0x0002 // oldSP < 4: Push32 underflow wrap
		w := runM68KWasmBlock(t, program, initD, initA, 0x2000, nil, nil)
		if w.needIOFallback == 0 {
			t.Fatalf("expected NeedIOFallback on push wrap")
		}
		if w.aregs[7] != 0x0002 {
			t.Errorf("A7 changed to %08X on bailed push", w.aregs[7])
		}
	})

	t.Run("RTS above ceiling bails", func(t *testing.T) {
		m68kWasmTestStackBounds = [2]uint32{0, 0x7000}
		program := m68kWasmWords(0x4E75) // rts
		var initD, initA [8]uint32
		initA[7] = 0x7000 // A7 >= ceiling: Pop32 bus error
		w := runM68KWasmBlock(t, program, initD, initA, 0x2000, nil, nil)
		if w.needIOFallback == 0 {
			t.Fatalf("expected NeedIOFallback on stack-ceiling violation")
		}
		if w.aregs[7] != 0x7000 {
			t.Errorf("A7 changed to %08X on bailed pop", w.aregs[7])
		}
	})

	t.Run("bounded push/pop still runs", func(t *testing.T) {
		m68kWasmTestStackBounds = [2]uint32{0x6000, 0x7800}
		var initD, initA [8]uint32
		initA[1] = 0x2000
		initA[7] = 0x7000
		m68kWasmDiffMem(t, "JSR in bounds", initD, initA, 0x2000, 1, nil, 0x6F00, 0x7100, 0x4E91)
	})
}

// TestM68KWasm_FPUGridM6 covers the milestone 6 FPU additions: FSGLMUL,
// FSGLDIV, FINTRZ and FINT across all four FPCR rounding modes.
func TestM68KWasm_FPUGridM6(t *testing.T) {
	vals := []float64{0.0, 1.0, -1.0, 2.5, -3.75, 0.5, -0.5, 1.5, -2.5,
		1e300, -1e-300, math.Inf(1), math.Inf(-1), math.NaN(), 123456.789, -0.0}
	fpCase := func(src, dst, opmode int) []uint16 {
		return []uint16{0xF200, uint16(src<<10 | dst<<7 | opmode)}
	}
	t.Run("FSGLMUL", func(t *testing.T) {
		for _, a := range vals {
			for _, b := range vals {
				seed := &m68kFPSeed{}
				seed.fp[0] = math.Float64bits(a)
				seed.fp[1] = math.Float64bits(b)
				m68kWasmDiffFP(t, "FSGLMUL", seed, 1, fpCase(0, 1, 0x27)...)
			}
		}
	})
	t.Run("FSGLDIV", func(t *testing.T) {
		for _, a := range vals {
			for _, b := range vals {
				seed := &m68kFPSeed{}
				seed.fp[0] = math.Float64bits(a)
				seed.fp[1] = math.Float64bits(b)
				m68kWasmDiffFP(t, "FSGLDIV", seed, 1, fpCase(0, 1, 0x24)...)
			}
		}
	})
	t.Run("FINTRZ", func(t *testing.T) {
		for _, a := range vals {
			seed := &m68kFPSeed{}
			seed.fp[0] = math.Float64bits(a)
			m68kWasmDiffFP(t, "FINTRZ", seed, 1, fpCase(0, 1, 0x03)...)
		}
	})
	for mode := uint32(0); mode < 4; mode++ {
		mode := mode
		t.Run("FINT", func(t *testing.T) {
			for _, a := range vals {
				seed := &m68kFPSeed{fpcr: mode << 4}
				seed.fp[0] = math.Float64bits(a)
				m68kWasmDiffFP(t, "FINT", seed, 1, fpCase(0, 1, 0x01)...)
			}
		})
	}
}

// TestM68KWasm_LoopGrid covers multi-instruction structured in-block loops:
// DBcc and backward Bcc whose target is the block start, with register and
// memory bodies, across counter values that exercise the not-taken path, the
// full loop and the budget exit with dispatcher-style re-entry.
func TestM68KWasm_LoopGrid(t *testing.T) {
	prep := seedMemPattern(0x5F00, 0x6200)
	counters := []uint32{0x00000000, 0x00000001, 0x00000005, 0x000000FF, 0x00000801, 0x0000FFFF, 0x00010003}
	cases := []struct {
		name  string
		words []uint16
	}{
		// addq.l #1,D1 ; dbf D0,<-4>  (target = block start)
		{"ADDQ/DBF", []uint16{0x5281, 0x51C8, 0xFFFC}},
		// add.l D1,D2 ; addq.l #1,D1 ; dbf D0,<-6>
		{"ADD/ADDQ/DBF", []uint16{0xD481, 0x5281, 0x51C8, 0xFFFA}},
		// move.b D1,(A1)+ ; addq.b #1,D1 ; dbf D0,<-6>  (memory store body)
		{"MOVE(A1)+/DBF", []uint16{0x1281, 0x5201, 0x51C8, 0xFFFA}},
		// add.l #-1 via subq to D0 ; bne <-4>  (Bcc backward loop)
		{"SUBQ/BNE", []uint16{0x5380, 0x66FC}},
		// clr.w (A1) ; addq.l #2,A1 ; subq.l #1,D0 ; bne <-8>
		{"CLR/ADDQ.A/SUBQ/BNE", []uint16{0x4251, 0x5489, 0x5380, 0x66F8}},
		// RMW body: addq.w #1,(A1) ; dbf D0,<-4>
		{"ADDQ.W(A1)/DBF", []uint16{0x5251, 0x51C8, 0xFFFC}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, cv := range counters {
				subqFirst := tc.name == "SUBQ/BNE" || tc.name == "CLR/ADDQ.A/SUBQ/BNE"
				if subqFirst && cv == 0 {
					continue // subq-first loop with 0 wraps 4G iterations
				}
				if tc.name == "CLR/ADDQ.A/SUBQ/BNE" && cv > 0x801 {
					continue // A1 += 2 per iteration would run past guest RAM
				}
				var initD, initA [8]uint32
				initD[0] = cv
				initD[1] = 0x00000041
				initA[1] = 0x6000
				for _, ccr := range []uint16{0x00, 0x1F} {
					m68kWasmDiffLoop(t, tc.name, initD, initA, 0x2000|ccr, prep, 0x5F00, 0x6200, tc.words...)
				}
			}
		})
	}
}

// TestM68KWasm_LoopSMC proves a loop body store that touches compiled code
// exits the loop with NeedInval-style early return:
// the store lands, the instruction is retired, and the resume PC is the next
// instruction, so the dispatcher can invalidate and recompile.
func TestM68KWasm_LoopSMC(t *testing.T) {
	savedPages := m68kWasmTestCodePages
	m68kWasmTestCodePages = true
	defer func() { m68kWasmTestCodePages = savedPages }()

	// move.w D1,(A1) ; dbf D0,<-4>. A1 targets the block's first instruction.
	program := m68kWasmWords(0x3281, 0x51C8, 0xFFFC)
	var initD, initA [8]uint32
	initD[0] = 5      // would loop 6 times without the SMC exit
	initD[1] = 0x4E71 // value stored
	initA[1] = m68kWasmTestPC
	w := runM68KWasmBlock(t, program, initD, initA, 0x2000, nil, nil)
	if w.needIOFallback != 0 {
		t.Fatalf("SMC exit must not be an I/O bail")
	}
	if w.count != 1 {
		t.Errorf("RetCount = %d, want 1 (store retired, loop exited)", w.count)
	}
	if w.pc != m68kWasmTestPC+2 {
		t.Errorf("resume PC = %08X, want %08X (the dbf)", w.pc, m68kWasmTestPC+2)
	}
	if w.guest[m68kWasmTestPC] != 0x4E || w.guest[m68kWasmTestPC+1] != 0x71 {
		t.Errorf("store did not land: %02X %02X", w.guest[m68kWasmTestPC], w.guest[m68kWasmTestPC+1])
	}
	if w.dregs[0] != 5 {
		t.Errorf("D0 = %08X, want 5 (dbf not executed)", w.dregs[0])
	}
}

// TestM68KWasm_CCRLivenessShape proves the milestone 6 within-block CCR
// liveness elision changes the emitted module: a producer whose condition
// codes are fully overwritten before any observation point compiles smaller
// with elision on than off, while a block whose every producer is live emits
// identical bytes under both settings.
func TestM68KWasm_CCRLivenessShape(t *testing.T) {
	compile := func(t *testing.T, elide bool, words ...uint16) []byte {
		t.Helper()
		if elide {
			t.Setenv("M68K_WASM_CCR_LIVENESS", "1")
		} else {
			t.Setenv("M68K_WASM_CCR_LIVENESS", "0")
		}
		program := m68kWasmWords(words...)
		mem := make([]byte, m68kWasmTestGuestLen)
		copy(mem[m68kWasmTestPC:], program)
		all := m68kScanBlock(mem, m68kWasmTestPC)
		var instrs []M68KJITInstr
		for i := range all {
			if all[i].pcOffset >= uint32(len(program)) {
				break
			}
			instrs = append(instrs, all[i])
		}
		bytes, err := m68kWasmCompileBlock(instrs, mem, m68kWasmTestPC)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		return bytes
	}

	// add.l D0,D1 ; add.l D2,D1 ; bra — the first ADD's full CCR output is
	// overwritten by the second before the branch reads it: dead producer.
	deadOn := compile(t, true, 0xD280, 0xD282, 0x6002)
	deadOff := compile(t, false, 0xD280, 0xD282, 0x6002)
	if len(deadOn) >= len(deadOff) {
		t.Errorf("dead-producer block: elided %d bytes >= unelided %d bytes", len(deadOn), len(deadOff))
	}

	// add.l D0,D1 ; beq — the ADD feeds the branch: live producer, no change.
	liveOn := compile(t, true, 0xD280, 0x6702)
	liveOff := compile(t, false, 0xD280, 0x6702)
	if len(liveOn) != len(liveOff) {
		t.Errorf("live-producer block: elided %d bytes != unelided %d bytes", len(liveOn), len(liveOff))
	}
}

// TestM68KWasm_CCRLivenessParity re-runs representative dead-producer chains
// with elision explicitly off and on, asserting identical architectural state
// against the interpreter both ways. (The whole differential suite already
// runs with the default-on setting.)
func TestM68KWasm_CCRLivenessParity(t *testing.T) {
	cases := []struct {
		name  string
		words []uint16
		count int
	}{
		{"ADD;ADD dead first", []uint16{0xD280, 0xD282}, 2},
		{"ADD;AND keeps X", []uint16{0xD280, 0xC282}, 2},
		{"MOVEQ;ADDQ;CMP chain", []uint16{0x7A05, 0x5285, 0xBA80}, 3},
		{"CLR;TST;MOVE chain", []uint16{0x4283, 0x4A80, 0x2400}, 3},
	}
	for _, setting := range []string{"0", "1"} {
		setting := setting
		t.Run("elide="+setting, func(t *testing.T) {
			t.Setenv("M68K_WASM_CCR_LIVENESS", setting)
			for _, tc := range cases {
				for _, dv := range m68kWasmGrid {
					for _, sv := range m68kWasmGrid {
						var initD, initA [8]uint32
						initD[0] = sv
						initD[1] = dv
						initD[2] = dv ^ 0x5A5A5A5A
						initD[5] = sv
						for _, ccr := range []uint16{0x00, 0x1F} {
							m68kWasmDiff(t, tc.name, initD, initA, 0x2000|ccr, tc.count, tc.words...)
						}
					}
				}
			}
		})
	}
}
