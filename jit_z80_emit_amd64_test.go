// jit_z80_emit_amd64_test.go - x86-64 emitter unit tests for Z80 JIT

//go:build amd64 && linux

package main

import (
	"testing"
	"unsafe"
)

// z80EmitTestRig compiles a small Z80 program into a native block and
// executes it via callNative, verifying register/flag state after execution.
type z80EmitTestRig struct {
	bus     *MachineBus
	adapter *Z80BusAdapter
	cpu     *CPU_Z80
	execMem *ExecMem
	ctx     *Z80JITContext
	mem     []byte
}

func newZ80EmitTestRig(t *testing.T) *z80EmitTestRig {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.SP = 0x1FFE
	bus.SealMappings()
	cpu.initDirectPageBitmapZ80(adapter)

	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	ctx := newZ80JITContext(cpu, adapter)
	mem := bus.GetMemory()

	t.Cleanup(func() { execMem.Free() })
	return &z80EmitTestRig{bus: bus, adapter: adapter, cpu: cpu, execMem: execMem, ctx: ctx, mem: mem}
}

// compileAndRun scans, compiles, and executes a block starting at startPC.
func (r *z80EmitTestRig) compileAndRun(t *testing.T, startPC uint16) {
	t.Helper()
	instrs := z80JITScanBlock(r.mem, startPC, len(r.mem), &r.cpu.directPageBitmap)
	if len(instrs) == 0 {
		t.Fatal("scanner returned empty block")
	}
	lastInstr := instrs[len(instrs)-1]
	endPC := startPC + lastInstr.pcOffset + uint16(lastInstr.length)
	totalR := 0
	for _, instr := range instrs {
		totalR += int(instr.rIncrements)
	}
	for page := startPC >> 8; page <= (endPC-1)>>8; page++ {
		r.cpu.codePageBitmap[page] = 1
	}
	block, err := compileBlockZ80Stub(instrs, startPC, endPC, r.execMem, totalR)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))
	r.cpu.PC = uint16(r.ctx.RetPC)
}

// ===========================================================================
// Per-Instruction Emission Tests
// ===========================================================================

func TestAMD64_Z80_Emit_NOP(t *testing.T) {
	r := newZ80EmitTestRig(t)
	r.cpu.A = 0x42
	r.bus.Write8(0x0100, 0x00) // NOP
	r.bus.Write8(0x0101, 0xC3) // JP 0x0200 (terminator)
	r.bus.Write8(0x0102, 0x00)
	r.bus.Write8(0x0103, 0x02)
	r.compileAndRun(t, 0x0100)
	if r.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42 (NOP shouldn't change A)", r.cpu.A)
	}
	if r.cpu.PC != 0x0200 {
		t.Errorf("PC = 0x%04X, want 0x0200", r.cpu.PC)
	}
}

func TestAMD64_Z80_Emit_LD_r_n(t *testing.T) {
	r := newZ80EmitTestRig(t)
	r.bus.Write8(0x0100, 0x3E) // LD A, 0xAB
	r.bus.Write8(0x0101, 0xAB)
	r.bus.Write8(0x0102, 0x06) // LD B, 0xCD
	r.bus.Write8(0x0103, 0xCD)
	r.bus.Write8(0x0104, 0xC3) // JP 0x0200
	r.bus.Write8(0x0105, 0x00)
	r.bus.Write8(0x0106, 0x02)
	r.compileAndRun(t, 0x0100)
	if r.cpu.A != 0xAB {
		t.Errorf("A = 0x%02X, want 0xAB", r.cpu.A)
	}
	if r.cpu.B != 0xCD {
		t.Errorf("B = 0x%02X, want 0xCD", r.cpu.B)
	}
}

func TestAMD64_Z80_Emit_LD_r_r(t *testing.T) {
	r := newZ80EmitTestRig(t)
	r.cpu.A = 0x55
	r.bus.Write8(0x0100, 0x47) // LD B, A
	r.bus.Write8(0x0101, 0xC3)
	r.bus.Write8(0x0102, 0x00)
	r.bus.Write8(0x0103, 0x02)
	r.compileAndRun(t, 0x0100)
	if r.cpu.B != 0x55 {
		t.Errorf("B = 0x%02X, want 0x55", r.cpu.B)
	}
}

func TestAMD64_Z80_Emit_ADD_SUB_Flags(t *testing.T) {
	r := newZ80EmitTestRig(t)
	// LD A, 0xFF; ADD A, 1 → A=0x00, Z=1, C=1
	r.bus.Write8(0x0100, 0x3E)
	r.bus.Write8(0x0101, 0xFF)
	r.bus.Write8(0x0102, 0xC6) // ADD A, 1
	r.bus.Write8(0x0103, 0x01)
	r.bus.Write8(0x0104, 0xC3)
	r.bus.Write8(0x0105, 0x00)
	r.bus.Write8(0x0106, 0x02)
	r.compileAndRun(t, 0x0100)
	if r.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00 (0xFF + 0x01)", r.cpu.A)
	}
	if r.cpu.F&0x40 == 0 {
		t.Errorf("Z flag should be set, F=0x%02X", r.cpu.F)
	}
}

func TestAMD64_Z80_Emit_CB_BIT_SET_RES(t *testing.T) {
	r := newZ80EmitTestRig(t)
	// LD A, 0x00; SET 3, A; BIT 3, A; RES 3, A
	r.bus.Write8(0x0100, 0x3E)
	r.bus.Write8(0x0101, 0x00)
	r.bus.Write8(0x0102, 0xCB) // SET 3, A
	r.bus.Write8(0x0103, 0xDF)
	r.bus.Write8(0x0104, 0xCB) // BIT 3, A
	r.bus.Write8(0x0105, 0x5F)
	r.bus.Write8(0x0106, 0xCB) // RES 3, A
	r.bus.Write8(0x0107, 0x9F)
	r.bus.Write8(0x0108, 0xC3)
	r.bus.Write8(0x0109, 0x00)
	r.bus.Write8(0x010A, 0x02)
	r.compileAndRun(t, 0x0100)
	if r.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00 (SET then RES)", r.cpu.A)
	}
	// After BIT 3, A (bit was set), Z should have been clear
	// After RES 3, A doesn't affect flags — Z still from BIT
	if r.cpu.F&0x40 != 0 {
		t.Errorf("Z should be clear (BIT 3 was set before RES), F=0x%02X", r.cpu.F)
	}
}

func TestAMD64_Z80_Emit_RegisterPreservation(t *testing.T) {
	r := newZ80EmitTestRig(t)
	// Set all registers to known values, run NOP + JP, verify all preserved
	r.cpu.A = 0x11
	r.cpu.F = 0x22
	r.cpu.B = 0x33
	r.cpu.C = 0x44
	r.cpu.D = 0x55
	r.cpu.E = 0x66
	r.cpu.H = 0x77
	r.cpu.L = 0x88

	r.bus.Write8(0x0100, 0x00) // NOP
	r.bus.Write8(0x0101, 0xC3)
	r.bus.Write8(0x0102, 0x00)
	r.bus.Write8(0x0103, 0x02)
	r.compileAndRun(t, 0x0100)

	regs := map[string]struct{ got, want byte }{
		"A": {r.cpu.A, 0x11}, "F": {r.cpu.F, 0x22},
		"B": {r.cpu.B, 0x33}, "C": {r.cpu.C, 0x44},
		"D": {r.cpu.D, 0x55}, "E": {r.cpu.E, 0x66},
		"H": {r.cpu.H, 0x77}, "L": {r.cpu.L, 0x88},
	}
	for name, rv := range regs {
		if rv.got != rv.want {
			t.Errorf("%s = 0x%02X, want 0x%02X", name, rv.got, rv.want)
		}
	}
}

func TestAMD64_Z80_Emit_LazyFlagElimination(t *testing.T) {
	// ADD A,B; ADD A,C; JP nn — first ADD's flags are dead (overwritten by second)
	// The emitter should skip flag materialization for the first ADD.
	// This test verifies correctness: second ADD's flags must still be correct.
	r := newZ80EmitTestRig(t)
	r.cpu.A = 0x10
	r.cpu.B = 0x20
	r.cpu.C = 0xD0 // 0x10 + 0x20 + 0xD0 = 0x00 (with carry)

	r.bus.Write8(0x0100, 0x80) // ADD A, B → A=0x30 (flags dead)
	r.bus.Write8(0x0101, 0x81) // ADD A, C → A=0x00 (flags live: Z=1)
	r.bus.Write8(0x0102, 0xC3)
	r.bus.Write8(0x0103, 0x00)
	r.bus.Write8(0x0104, 0x02)
	r.compileAndRun(t, 0x0100)

	if r.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", r.cpu.A)
	}
	// Z flag from second ADD should be set
	if r.cpu.F&0x40 == 0 {
		t.Errorf("Z should be set (result is 0), F=0x%02X", r.cpu.F)
	}
}

func TestAMD64_Z80_Emit_MemReadBail(t *testing.T) {
	// LD A, (HL) where HL points to non-direct page → bail
	r := newZ80EmitTestRig(t)
	r.cpu.H = 0x20 // HL = 0x2000 (non-direct)
	r.cpu.L = 0x00

	r.bus.Write8(0x0100, 0x7E) // LD A, (HL) — will bail
	r.bus.Write8(0x0101, 0xC3)
	r.bus.Write8(0x0102, 0x00)
	r.bus.Write8(0x0103, 0x02)
	r.compileAndRun(t, 0x0100)

	// Should have bailed — NeedBail should be set
	if r.ctx.NeedBail == 0 {
		t.Error("NeedBail should be set (HL points to non-direct page)")
	}
	if r.ctx.RetPC != 0x0100 {
		t.Errorf("RetPC = 0x%04X, want 0x0100 (bail PC = current instruction)", r.ctx.RetPC)
	}
}
