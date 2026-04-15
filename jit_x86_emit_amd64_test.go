// jit_x86_emit_amd64_test.go - Tests for x86 JIT x86-64 host emitter

//go:build amd64 && linux

package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// x86 JIT Test Rig
// ===========================================================================

type x86JITTestRig struct {
	cpu     *CPU_X86
	adapter *X86BusAdapter
	bus     *MachineBus
	execMem *ExecMem
	ctx     *X86JITContext
	bitmap  []byte
	codeBM  []byte
}

func newX86JITTestRig(t *testing.T) *x86JITTestRig {
	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()

	em, err := AllocExecMem(1 << 20) // 1MB
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	t.Cleanup(func() { em.Free() })

	ioBitmap := buildX86IOBitmap(adapter, bus)
	codeBM := make([]byte, len(ioBitmap))
	ctx := newX86JITContext(cpu, codeBM, ioBitmap)

	return &x86JITTestRig{
		cpu: cpu, adapter: adapter, bus: bus,
		execMem: em, ctx: ctx, bitmap: ioBitmap, codeBM: codeBM,
	}
}

// compileAndRun writes raw x86 machine code to memory at startPC, appends HLT,
// scans, compiles, and executes via JIT.
func (r *x86JITTestRig) compileAndRun(t *testing.T, startPC uint32, code ...byte) {
	t.Helper()

	// Write code to memory
	pc := startPC
	for _, b := range code {
		r.cpu.memory[pc] = b
		pc++
	}
	// Append HLT as block terminator
	r.cpu.memory[pc] = 0xF4

	// Sync named registers to jitRegs for JIT to access
	r.cpu.syncJITRegsFromNamed()
	r.cpu.syncJITSegRegsFromNamed()

	// Scan block
	instrs := x86ScanBlock(r.cpu.memory, startPC)
	if len(instrs) == 0 {
		t.Fatal("x86ScanBlock returned 0 instructions")
	}

	// Strip HLT terminator (we don't compile it, just terminate the block)
	compilable := instrs
	if compilable[len(compilable)-1].opcode == 0x00F4 {
		compilable = compilable[:len(compilable)-1]
	}
	if len(compilable) == 0 {
		// Just a HLT - nothing to compile, but that's OK for some tests
		return
	}

	// Compile
	r.execMem.Reset()
	block, err := x86CompileBlock(compilable, startPC, r.execMem, r.cpu.memory)
	if err != nil {
		t.Fatalf("x86CompileBlock: %v", err)
	}

	// Update context pointers
	r.ctx.JITRegsPtr = uintptr(unsafe.Pointer(&r.cpu.jitRegs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	r.ctx.FlagsPtr = uintptr(unsafe.Pointer(&r.cpu.Flags))
	r.ctx.EIPPtr = uintptr(unsafe.Pointer(&r.cpu.EIP))
	r.ctx.SegRegsPtr = uintptr(unsafe.Pointer(&r.cpu.jitSegRegs[0]))

	// Execute
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	// Sync jitRegs back to named registers
	r.cpu.syncJITRegsToNamed()
	r.cpu.syncJITSegRegsToNamed()

	// Update EIP from context
	r.cpu.EIP = r.ctx.RetPC
}

// ===========================================================================
// MOV r32, imm32 Tests
// ===========================================================================

func TestX86JIT_MOV_r32_imm32(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV EAX, 0x12345678 (B8 78 56 34 12)
	r.compileAndRun(t, 0x1000, 0xB8, 0x78, 0x56, 0x34, 0x12)

	if r.cpu.EAX != 0x12345678 {
		t.Errorf("EAX = 0x%08X, want 0x12345678", r.cpu.EAX)
	}
}

func TestX86JIT_MOV_ECX_imm32(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV ECX, 0xDEADBEEF (B9 EF BE AD DE)
	r.compileAndRun(t, 0x1000, 0xB9, 0xEF, 0xBE, 0xAD, 0xDE)

	if r.cpu.ECX != 0xDEADBEEF {
		t.Errorf("ECX = 0x%08X, want 0xDEADBEEF", r.cpu.ECX)
	}
}

func TestX86JIT_MOV_multiple(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV EAX, 1; MOV ECX, 2; MOV EDX, 3
	r.compileAndRun(t, 0x1000,
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX, 1
		0xB9, 0x02, 0x00, 0x00, 0x00, // MOV ECX, 2
		0xBA, 0x03, 0x00, 0x00, 0x00, // MOV EDX, 3
	)

	if r.cpu.EAX != 1 {
		t.Errorf("EAX = %d, want 1", r.cpu.EAX)
	}
	if r.cpu.ECX != 2 {
		t.Errorf("ECX = %d, want 2", r.cpu.ECX)
	}
	if r.cpu.EDX != 3 {
		t.Errorf("EDX = %d, want 3", r.cpu.EDX)
	}
}

// ===========================================================================
// ADD r32, r32 Tests
// ===========================================================================

func TestX86JIT_ADD_r32_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 100
	r.cpu.EBX = 200

	// ADD EAX, EBX (01 D8: mod=11, reg=011(EBX), rm=000(EAX))
	r.compileAndRun(t, 0x1000, 0x01, 0xD8)

	if r.cpu.EAX != 300 {
		t.Errorf("EAX = %d, want 300", r.cpu.EAX)
	}
	if r.cpu.EBX != 200 {
		t.Errorf("EBX = %d, want 200 (unchanged)", r.cpu.EBX)
	}
}

func TestX86JIT_ADD_with_MOV(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV EAX, 10; MOV EBX, 20; ADD EAX, EBX
	r.compileAndRun(t, 0x1000,
		0xB8, 0x0A, 0x00, 0x00, 0x00, // MOV EAX, 10
		0xBB, 0x14, 0x00, 0x00, 0x00, // MOV EBX, 20
		0x01, 0xD8, // ADD EAX, EBX
	)

	if r.cpu.EAX != 30 {
		t.Errorf("EAX = %d, want 30", r.cpu.EAX)
	}
}

// ===========================================================================
// SUB r32, r32 Tests
// ===========================================================================

func TestX86JIT_SUB_r32_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 300
	r.cpu.EBX = 100

	// SUB EAX, EBX (29 D8: opcode=0x29, mod=11, reg=011(EBX), rm=000(EAX))
	r.compileAndRun(t, 0x1000, 0x29, 0xD8)

	if r.cpu.EAX != 200 {
		t.Errorf("EAX = %d, want 200", r.cpu.EAX)
	}
}

// ===========================================================================
// AND/OR/XOR r32, r32 Tests
// ===========================================================================

func TestX86JIT_AND_r32_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xFF00FF00
	r.cpu.EBX = 0x0F0F0F0F

	// AND EAX, EBX (21 D8)
	r.compileAndRun(t, 0x1000, 0x21, 0xD8)

	if r.cpu.EAX != 0x0F000F00 {
		t.Errorf("EAX = 0x%08X, want 0x0F000F00", r.cpu.EAX)
	}
}

func TestX86JIT_OR_r32_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xFF000000
	r.cpu.EBX = 0x000000FF

	// OR EAX, EBX (09 D8)
	r.compileAndRun(t, 0x1000, 0x09, 0xD8)

	if r.cpu.EAX != 0xFF0000FF {
		t.Errorf("EAX = 0x%08X, want 0xFF0000FF", r.cpu.EAX)
	}
}

func TestX86JIT_XOR_r32_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xAAAAAAAA
	r.cpu.EBX = 0x55555555

	// XOR EAX, EBX (31 D8)
	r.compileAndRun(t, 0x1000, 0x31, 0xD8)

	if r.cpu.EAX != 0xFFFFFFFF {
		t.Errorf("EAX = 0x%08X, want 0xFFFFFFFF", r.cpu.EAX)
	}
}

// ===========================================================================
// NOP Test
// ===========================================================================

func TestX86JIT_NOP(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0x42

	// NOP; NOP; NOP
	r.compileAndRun(t, 0x1000, 0x90, 0x90, 0x90)

	if r.cpu.EAX != 0x42 {
		t.Errorf("EAX = 0x%08X, want 0x42 (unchanged after NOPs)", r.cpu.EAX)
	}
}

// ===========================================================================
// INC/DEC r32 Tests
// ===========================================================================

func TestX86JIT_INC_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 41

	// INC EAX (0x40)
	r.compileAndRun(t, 0x1000, 0x40)

	if r.cpu.EAX != 42 {
		t.Errorf("EAX = %d, want 42", r.cpu.EAX)
	}
}

func TestX86JIT_DEC_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 43

	// DEC EAX (0x48)
	r.compileAndRun(t, 0x1000, 0x48)

	if r.cpu.EAX != 42 {
		t.Errorf("EAX = %d, want 42", r.cpu.EAX)
	}
}

// ===========================================================================
// MOV r32, r32 Tests
// ===========================================================================

func TestX86JIT_MOV_r32_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x12345678

	// MOV EAX, EBX (8B C3: opcode=0x8B, mod=11, reg=000(EAX), rm=011(EBX))
	r.compileAndRun(t, 0x1000, 0x8B, 0xC3)

	if r.cpu.EAX != 0x12345678 {
		t.Errorf("EAX = 0x%08X, want 0x12345678", r.cpu.EAX)
	}
}

// ===========================================================================
// Grp1 Tests (ADD/SUB/AND/OR/XOR/CMP with immediate)
// ===========================================================================

func TestX86JIT_ADD_r32_imm32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 100

	// ADD EBX, 50: 81 C3 32 00 00 00
	r.compileAndRun(t, 0x1000, 0x81, 0xC3, 0x32, 0x00, 0x00, 0x00)

	if r.cpu.EBX != 150 {
		t.Errorf("EBX = %d, want 150", r.cpu.EBX)
	}
}

func TestX86JIT_ADD_r32_imm8(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 100

	// ADD EBX, 50: 83 C3 32
	r.compileAndRun(t, 0x1000, 0x83, 0xC3, 0x32)

	if r.cpu.EBX != 150 {
		t.Errorf("EBX = %d, want 150", r.cpu.EBX)
	}
}

// ===========================================================================
// ADD AL/EAX, imm Tests
// ===========================================================================

func TestX86JIT_ADD_EAX_imm32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 100

	// ADD EAX, 200 (05 C8 00 00 00)
	r.compileAndRun(t, 0x1000, 0x05, 0xC8, 0x00, 0x00, 0x00)

	if r.cpu.EAX != 300 {
		t.Errorf("EAX = %d, want 300", r.cpu.EAX)
	}
}

// ===========================================================================
// LEA Tests
// ===========================================================================

func TestX86JIT_LEA_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x1000

	// LEA EAX, [EBX+0x10] = 8D 43 10
	r.compileAndRun(t, 0x2000, 0x8D, 0x43, 0x10)

	if r.cpu.EAX != 0x1010 {
		t.Errorf("EAX = 0x%08X, want 0x1010", r.cpu.EAX)
	}
}

// ===========================================================================
// RetPC/RetCount Tests
// ===========================================================================

func TestX86JIT_RetPC_RetCount(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV EAX, 1; MOV EBX, 2; (then HLT at offset +10)
	r.compileAndRun(t, 0x1000,
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX, 1 (5 bytes)
		0xBB, 0x02, 0x00, 0x00, 0x00, // MOV EBX, 2 (5 bytes)
	)

	// RetPC should be past the last compiled instruction (0x1000 + 10 = 0x100A)
	if r.ctx.RetPC != 0x100A {
		t.Errorf("RetPC = 0x%X, want 0x100A", r.ctx.RetPC)
	}
	if r.ctx.RetCount != 2 {
		t.Errorf("RetCount = %d, want 2", r.ctx.RetCount)
	}
}

// ===========================================================================
// Spilled Register Tests (EBP, ESI, EDI - not mapped to host regs)
// ===========================================================================

func TestX86JIT_MOV_spilled_reg(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV ESI, 0xAABBCCDD (BE DD CC BB AA)
	r.compileAndRun(t, 0x1000, 0xBE, 0xDD, 0xCC, 0xBB, 0xAA)

	if r.cpu.ESI != 0xAABBCCDD {
		t.Errorf("ESI = 0x%08X, want 0xAABBCCDD", r.cpu.ESI)
	}
}

func TestX86JIT_MOV_EDI_imm32(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV EDI, 0x11223344 (BF 44 33 22 11)
	r.compileAndRun(t, 0x1000, 0xBF, 0x44, 0x33, 0x22, 0x11)

	if r.cpu.EDI != 0x11223344 {
		t.Errorf("EDI = 0x%08X, want 0x11223344", r.cpu.EDI)
	}
}

// ===========================================================================
// Phase 5: Shift Tests
// ===========================================================================

func TestX86JIT_SHL_r32_imm8(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 1

	// SHL EAX, 4: C1 E0 04
	r.compileAndRun(t, 0x1000, 0xC1, 0xE0, 0x04)

	if r.cpu.EAX != 16 {
		t.Errorf("EAX = %d, want 16", r.cpu.EAX)
	}
}

func TestX86JIT_SHR_r32_imm8(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 256

	// SHR EAX, 4: C1 E8 04
	r.compileAndRun(t, 0x1000, 0xC1, 0xE8, 0x04)

	if r.cpu.EAX != 16 {
		t.Errorf("EAX = %d, want 16", r.cpu.EAX)
	}
}

func TestX86JIT_SAR_r32_imm8(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xFFFFFFF0 // -16

	// SAR EAX, 2: C1 F8 02
	r.compileAndRun(t, 0x1000, 0xC1, 0xF8, 0x02)

	if r.cpu.EAX != 0xFFFFFFFC { // -4
		t.Errorf("EAX = 0x%08X, want 0xFFFFFFFC", r.cpu.EAX)
	}
}

func TestX86JIT_SHL_r32_1(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x40000000

	// SHL EBX, 1: D1 E3
	r.compileAndRun(t, 0x1000, 0xD1, 0xE3)

	if r.cpu.EBX != 0x80000000 {
		t.Errorf("EBX = 0x%08X, want 0x80000000", r.cpu.EBX)
	}
}

func TestX86JIT_SHL_r32_CL(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 1
	r.cpu.ECX = 8

	// SHL EAX, CL: D3 E0
	r.compileAndRun(t, 0x1000, 0xD3, 0xE0)

	if r.cpu.EAX != 256 {
		t.Errorf("EAX = %d, want 256", r.cpu.EAX)
	}
}

// ===========================================================================
// BMI2 Non-Flag-Affecting Shift Tests
// ===========================================================================

func TestX86JIT_BMI2_SHL_imm(t *testing.T) {
	if !x86Host.HasBMI2 {
		t.Skip("BMI2 not available on this CPU")
	}
	r := newX86JITTestRig(t)
	r.cpu.EAX = 1

	// SHL EAX, 4 (C1 E0 04) -- should use SHLX when flags not needed
	// Followed by HLT (no flag consumer), so flagsNeeded should be false
	r.compileAndRun(t, 0x1000, 0xC1, 0xE0, 0x04)

	if r.cpu.EAX != 16 {
		t.Errorf("EAX = %d, want 16", r.cpu.EAX)
	}
}

func TestX86JIT_BMI2_FlagPreserve(t *testing.T) {
	if !x86Host.HasBMI2 {
		t.Skip("BMI2 not available on this CPU")
	}
	r := newX86JITTestRig(t)
	r.cpu.EAX = 1

	// SHL EAX, 3 followed by ADD EAX, 0 -- the SHL's flags are dead because
	// ADD overwrites them. Verifies SHLX is used (no flag clobber) when the
	// next flag-producing instruction makes this shift's flags dead.
	r.compileAndRun(t, 0x1000,
		0xC1, 0xE0, 0x03, // SHL EAX, 3 (flags dead -- ADD below overwrites)
		0x83, 0xC0, 0x00, // ADD EAX, 0 (overwrites flags)
		0xF4, // HLT
	)

	if r.cpu.EAX != 8 {
		t.Errorf("EAX = %d, want 8 (1 << 3)", r.cpu.EAX)
	}
}

// ===========================================================================
// Phase 5: NOT/NEG Tests
// ===========================================================================

func TestX86JIT_NOT_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xFF00FF00

	// NOT EAX: F7 D0
	r.compileAndRun(t, 0x1000, 0xF7, 0xD0)

	if r.cpu.EAX != 0x00FF00FF {
		t.Errorf("EAX = 0x%08X, want 0x00FF00FF", r.cpu.EAX)
	}
}

func TestX86JIT_NEG_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 42

	// NEG EAX: F7 D8
	r.compileAndRun(t, 0x1000, 0xF7, 0xD8)

	want := uint32(0xFFFFFFD6) // -42 as uint32
	if r.cpu.EAX != want {
		t.Errorf("EAX = 0x%08X, want 0x%08X", r.cpu.EAX, want)
	}
}

// ===========================================================================
// Phase 5: MUL/IMUL/DIV Tests
// ===========================================================================

func TestX86JIT_MUL_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 100
	r.cpu.EBX = 200

	// MUL EBX: F7 E3 (EDX:EAX = EAX * EBX)
	r.compileAndRun(t, 0x1000, 0xF7, 0xE3)

	if r.cpu.EAX != 20000 {
		t.Errorf("EAX = %d, want 20000", r.cpu.EAX)
	}
	if r.cpu.EDX != 0 {
		t.Errorf("EDX = %d, want 0 (no overflow)", r.cpu.EDX)
	}
}

func TestX86JIT_IMUL_Gv_Ev(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 7
	r.cpu.EBX = 6

	// IMUL EAX, EBX: 0F AF C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xAF, 0xC3)

	if r.cpu.EAX != 42 {
		t.Errorf("EAX = %d, want 42", r.cpu.EAX)
	}
}

func TestX86JIT_DIV_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 100
	r.cpu.EDX = 0
	r.cpu.EBX = 7

	// DIV EBX: F7 F3 (EAX = EDX:EAX / EBX, EDX = remainder)
	r.compileAndRun(t, 0x1000, 0xF7, 0xF3)

	if r.cpu.EAX != 14 {
		t.Errorf("EAX (quotient) = %d, want 14", r.cpu.EAX)
	}
	if r.cpu.EDX != 2 {
		t.Errorf("EDX (remainder) = %d, want 2", r.cpu.EDX)
	}
}

// ===========================================================================
// Phase 5: MOVZX/MOVSX Tests
// ===========================================================================

func TestX86JIT_MOVZX_Gv_Eb(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0xDEAD00FF

	// MOVZX EAX, BL: 0F B6 C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xB6, 0xC3)

	if r.cpu.EAX != 0xFF {
		t.Errorf("EAX = 0x%08X, want 0xFF", r.cpu.EAX)
	}
}

func TestX86JIT_MOVSX_Gv_Eb(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x000000F0 // BL = 0xF0 = -16

	// MOVSX EAX, BL: 0F BE C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xBE, 0xC3)

	if r.cpu.EAX != 0xFFFFFFF0 {
		t.Errorf("EAX = 0x%08X, want 0xFFFFFFF0", r.cpu.EAX)
	}
}

func TestX86JIT_MOVZX_Gv_Ew(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0xDEADFFFF

	// MOVZX EAX, BX: 0F B7 C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xB7, 0xC3)

	if r.cpu.EAX != 0xFFFF {
		t.Errorf("EAX = 0x%08X, want 0xFFFF", r.cpu.EAX)
	}
}

// ===========================================================================
// Phase 5: PUSH/POP Tests
// ===========================================================================

func TestX86JIT_PUSH_POP_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0x12345678
	r.cpu.ESP = 0x10000

	// PUSH EAX; POP EBX
	r.compileAndRun(t, 0x1000,
		0x50, // PUSH EAX
		0x5B, // POP EBX
	)

	if r.cpu.EBX != 0x12345678 {
		t.Errorf("EBX = 0x%08X, want 0x12345678", r.cpu.EBX)
	}
	if r.cpu.ESP != 0x10000 {
		t.Errorf("ESP = 0x%08X, want 0x10000 (balanced push/pop)", r.cpu.ESP)
	}
}

func TestX86JIT_PUSH_imm32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.ESP = 0x10000

	// PUSH 0xDEADBEEF; POP EAX
	r.compileAndRun(t, 0x1000,
		0x68, 0xEF, 0xBE, 0xAD, 0xDE, // PUSH 0xDEADBEEF
		0x58, // POP EAX
	)

	if r.cpu.EAX != 0xDEADBEEF {
		t.Errorf("EAX = 0x%08X, want 0xDEADBEEF", r.cpu.EAX)
	}
}

// ===========================================================================
// Phase 5: TEST Tests
// ===========================================================================

func TestX86JIT_TEST_Ev_Gv(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xFF
	r.cpu.EBX = 0x0F

	// TEST EAX, EBX: 85 D8
	r.compileAndRun(t, 0x1000, 0x85, 0xD8)

	// TEST doesn't modify operands
	if r.cpu.EAX != 0xFF {
		t.Errorf("EAX = 0x%08X, want 0xFF (unchanged)", r.cpu.EAX)
	}
}

// ===========================================================================
// Phase 5: MOV Ev,Iv (0xC7) Tests
// ===========================================================================

func TestX86JIT_MOV_Ev_Iv(t *testing.T) {
	r := newX86JITTestRig(t)

	// MOV EAX, 0x42 via C7 C0 42 00 00 00
	r.compileAndRun(t, 0x1000, 0xC7, 0xC0, 0x42, 0x00, 0x00, 0x00)

	if r.cpu.EAX != 0x42 {
		t.Errorf("EAX = 0x%08X, want 0x42", r.cpu.EAX)
	}
}

func TestX86JIT_MOV_EAX_moffs32(t *testing.T) {
	r := newX86JITTestRig(t)
	addr := uint32(0x3000)
	r.bus.Write32(addr, 0x11223344)

	// A1 id: MOV EAX, moffs32
	r.compileAndRun(t, 0x1000,
		0xA1, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24),
	)

	if r.cpu.EAX != 0x11223344 {
		t.Errorf("EAX = 0x%08X, want 0x11223344", r.cpu.EAX)
	}
}

func TestX86JIT_MOV_moffs32_EAX(t *testing.T) {
	r := newX86JITTestRig(t)
	addr := uint32(0x3000)
	r.cpu.EAX = 0x55667788

	// A3 id: MOV moffs32, EAX
	r.compileAndRun(t, 0x1000,
		0xA3, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24),
	)

	if got := r.bus.Read32(addr); got != 0x55667788 {
		t.Errorf("[0x%X] = 0x%08X, want 0x55667788", addr, got)
	}
}

func TestX86JIT_MOV_memabs_imm32(t *testing.T) {
	r := newX86JITTestRig(t)
	addr := uint32(0x3000)

	// C7 /0 with modrm 00 000 101 + disp32: MOV dword [disp32], imm32
	r.compileAndRun(t, 0x1000,
		0xC7, 0x05, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24),
		0x78, 0x56, 0x34, 0x12,
	)

	if got := r.bus.Read32(addr); got != 0x12345678 {
		t.Errorf("[0x%X] = 0x%08X, want 0x12345678", addr, got)
	}
}

// ===========================================================================
// Phase 5: LEAVE Test
// ===========================================================================

// ===========================================================================
// Phase 7: Memory Operand Tests
// ===========================================================================

func TestX86JIT_MOV_mem_r32(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xDEADBEEF
	r.cpu.EBX = 0x5000 // address

	// MOV [EBX], EAX: 89 03 (mod=00, reg=000(EAX), rm=011(EBX))
	r.compileAndRun(t, 0x1000, 0x89, 0x03)

	// Verify memory was written
	val := uint32(r.cpu.memory[0x5000]) | uint32(r.cpu.memory[0x5001])<<8 |
		uint32(r.cpu.memory[0x5002])<<16 | uint32(r.cpu.memory[0x5003])<<24
	if val != 0xDEADBEEF {
		t.Errorf("[0x5000] = 0x%08X, want 0xDEADBEEF", val)
	}
}

func TestX86JIT_MOV_r32_mem(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x5000
	// Write a value to memory
	r.cpu.memory[0x5000] = 0x78
	r.cpu.memory[0x5001] = 0x56
	r.cpu.memory[0x5002] = 0x34
	r.cpu.memory[0x5003] = 0x12

	// MOV EAX, [EBX]: 8B 03 (mod=00, reg=000(EAX), rm=011(EBX))
	r.compileAndRun(t, 0x1000, 0x8B, 0x03)

	if r.cpu.EAX != 0x12345678 {
		t.Errorf("EAX = 0x%08X, want 0x12345678", r.cpu.EAX)
	}
}

func TestX86JIT_MOV_r32_mem_disp8(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x5000
	r.cpu.memory[0x5010] = 0xAA
	r.cpu.memory[0x5011] = 0xBB
	r.cpu.memory[0x5012] = 0xCC
	r.cpu.memory[0x5013] = 0xDD

	// MOV EAX, [EBX+0x10]: 8B 43 10
	r.compileAndRun(t, 0x1000, 0x8B, 0x43, 0x10)

	if r.cpu.EAX != 0xDDCCBBAA {
		t.Errorf("EAX = 0x%08X, want 0xDDCCBBAA", r.cpu.EAX)
	}
}

func TestX86JIT_ADD_r32_mem(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 100
	r.cpu.EBX = 0x5000
	r.cpu.memory[0x5000] = 0xC8 // 200
	r.cpu.memory[0x5001] = 0x00
	r.cpu.memory[0x5002] = 0x00
	r.cpu.memory[0x5003] = 0x00

	// ADD EAX, [EBX]: 03 03
	r.compileAndRun(t, 0x1000, 0x03, 0x03)

	if r.cpu.EAX != 300 {
		t.Errorf("EAX = %d, want 300", r.cpu.EAX)
	}
}

func TestX86JIT_CMP_r32_mem(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 42
	r.cpu.EBX = 0x5000
	r.cpu.memory[0x5000] = 42
	r.cpu.memory[0x5001] = 0
	r.cpu.memory[0x5002] = 0
	r.cpu.memory[0x5003] = 0

	// CMP EAX, [EBX]: 3B 03 -- sets ZF because equal
	r.compileAndRun(t, 0x1000, 0x3B, 0x03)

	// EAX should be unchanged (CMP doesn't store)
	if r.cpu.EAX != 42 {
		t.Errorf("EAX = %d, want 42 (unchanged after CMP)", r.cpu.EAX)
	}
}

// ===========================================================================
// REP String Operation Tests
// ===========================================================================

func TestX86JIT_REP_STOSB(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0x42   // fill byte
	r.cpu.ECX = 8      // count
	r.cpu.EDI = 0x5000 // dest

	// REP STOSB: F3 AA
	r.compileAndRun(t, 0x1000, 0xF3, 0xAA)

	for i := uint32(0); i < 8; i++ {
		if r.cpu.memory[0x5000+i] != 0x42 {
			t.Errorf("memory[0x%X] = 0x%02X, want 0x42", 0x5000+i, r.cpu.memory[0x5000+i])
		}
	}
	if r.cpu.ECX != 0 {
		t.Errorf("ECX = %d, want 0", r.cpu.ECX)
	}
	if r.cpu.EDI != 0x5008 {
		t.Errorf("EDI = 0x%X, want 0x5008", r.cpu.EDI)
	}
}

func TestX86JIT_REP_MOVSB(t *testing.T) {
	r := newX86JITTestRig(t)
	// Write source data
	for i := byte(0); i < 16; i++ {
		r.cpu.memory[0x5000+uint32(i)] = i + 1
	}
	r.cpu.ESI = 0x5000
	r.cpu.EDI = 0x6000
	r.cpu.ECX = 16

	// REP MOVSB: F3 A4
	r.compileAndRun(t, 0x1000, 0xF3, 0xA4)

	for i := uint32(0); i < 16; i++ {
		if r.cpu.memory[0x6000+i] != r.cpu.memory[0x5000+i] {
			t.Errorf("memory[0x%X] = 0x%02X, want 0x%02X", 0x6000+i, r.cpu.memory[0x6000+i], r.cpu.memory[0x5000+i])
		}
	}
	if r.cpu.ECX != 0 {
		t.Errorf("ECX = %d, want 0", r.cpu.ECX)
	}
}

// ===========================================================================
// 16-bit Operand Tests
// ===========================================================================

func TestX86JIT_MOV_r16_imm16(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xDEAD0000

	// 66 B8 34 12: MOV AX, 0x1234 (16-bit, preserves upper 16)
	r.compileAndRun(t, 0x1000, 0x66, 0xB8, 0x34, 0x12)

	if r.cpu.EAX != 0xDEAD1234 {
		t.Errorf("EAX = 0x%08X, want 0xDEAD1234", r.cpu.EAX)
	}
}

// ===========================================================================
// x87 FPU Tests
// ===========================================================================

func TestX86JIT_FPU_FADD(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 1.5 // ST(0) when TOP=0
	r.cpu.FPU.regs[1] = 2.5 // ST(1) when TOP=0
	r.cpu.FPU.setTop(0)

	// FADD ST(0), ST(1): D8 C1
	r.compileAndRun(t, 0x1000, 0xD8, 0xC1)

	if r.cpu.FPU.regs[0] != 4.0 {
		t.Errorf("ST(0) = %f, want 4.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FMUL(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 3.0
	r.cpu.FPU.regs[1] = 7.0
	r.cpu.FPU.setTop(0)

	// FMUL ST(0), ST(1): D8 C9
	r.compileAndRun(t, 0x1000, 0xD8, 0xC9)

	if r.cpu.FPU.regs[0] != 21.0 {
		t.Errorf("ST(0) = %f, want 21.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FSUB(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 10.0
	r.cpu.FPU.regs[1] = 3.0
	r.cpu.FPU.setTop(0)

	// FSUB ST(0), ST(1): D8 E1
	r.compileAndRun(t, 0x1000, 0xD8, 0xE1)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FDIV(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 21.0
	r.cpu.FPU.regs[1] = 3.0
	r.cpu.FPU.setTop(0)

	// FDIV ST(0), ST(1): D8 F1
	r.compileAndRun(t, 0x1000, 0xD8, 0xF1)

	if r.cpu.FPU.regs[0] != 7.0 {
		t.Errorf("ST(0) = %f, want 7.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FCHS(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = 42.0
	r.cpu.FPU.setTop(0)

	// FCHS: D9 E0
	r.compileAndRun(t, 0x1000, 0xD9, 0xE0)

	if r.cpu.FPU.regs[0] != -42.0 {
		t.Errorf("ST(0) = %f, want -42.0", r.cpu.FPU.regs[0])
	}
}

func TestX86JIT_FPU_FABS(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.FPU.Reset()
	r.cpu.FPU.regs[0] = -42.0
	r.cpu.FPU.setTop(0)

	// FABS: D9 E1
	r.compileAndRun(t, 0x1000, 0xD9, 0xE1)

	if r.cpu.FPU.regs[0] != 42.0 {
		t.Errorf("ST(0) = %f, want 42.0", r.cpu.FPU.regs[0])
	}
}

// ===========================================================================
// SETcc / CMOVcc / BSF Tests
// ===========================================================================

func TestX86JIT_SETcc(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 42
	r.cpu.EBX = 42

	// CMP EAX, EBX; SETE CL -- sets CL=1 if equal
	// CMP: 39 D8, SETE: 0F 94 C1 (mod=11, reg=000, rm=001=CL)
	r.compileAndRun(t, 0x1000,
		0x39, 0xD8, // CMP EAX, EBX
		0x0F, 0x94, 0xC1, // SETE CL
	)

	if r.cpu.ECX&0xFF != 1 {
		t.Errorf("CL = %d, want 1 (equal)", r.cpu.ECX&0xFF)
	}
}

func TestX86JIT_BSF(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x80 // bit 7 is lowest set bit

	// BSF EAX, EBX: 0F BC C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xBC, 0xC3)

	if r.cpu.EAX != 7 {
		t.Errorf("EAX = %d, want 7 (lowest set bit of 0x80)", r.cpu.EAX)
	}
}

func TestX86JIT_BSR(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x80 // bit 7 is highest set bit

	// BSR EAX, EBX: 0F BD C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xBD, 0xC3)

	if r.cpu.EAX != 7 {
		t.Errorf("EAX = %d, want 7 (highest set bit of 0x80)", r.cpu.EAX)
	}
}

// ===========================================================================
// LZCNT/TZCNT Tests
// ===========================================================================

func TestX86JIT_BSF_NonZero_TZCNT(t *testing.T) {
	if !x86Host.HasLZCNT {
		t.Skip("LZCNT/TZCNT not available")
	}
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x80 // bit 7

	// BSF EAX, EBX: 0F BC C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xBC, 0xC3)

	if r.cpu.EAX != 7 {
		t.Errorf("EAX = %d, want 7", r.cpu.EAX)
	}
}

func TestX86JIT_BSR_NonZero_LZCNT(t *testing.T) {
	if !x86Host.HasLZCNT {
		t.Skip("LZCNT/TZCNT not available")
	}
	r := newX86JITTestRig(t)
	r.cpu.EBX = 0x80 // bit 7

	// BSR EAX, EBX: 0F BD C3
	r.compileAndRun(t, 0x1000, 0x0F, 0xBD, 0xC3)

	if r.cpu.EAX != 7 {
		t.Errorf("EAX = %d, want 7", r.cpu.EAX)
	}
}

func TestX86JIT_BSF_Zero_DestUnchanged(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0x42 // pre-set destination
	r.cpu.EBX = 0    // zero source

	// BSF EAX, EBX: 0F BC C3 -- EAX should remain 0x42 on zero input
	r.compileAndRun(t, 0x1000, 0x0F, 0xBC, 0xC3)

	if r.cpu.EAX != 0x42 {
		t.Errorf("EAX = 0x%X, want 0x42 (destination unchanged on zero input)", r.cpu.EAX)
	}
}

// ===========================================================================
// Hardware REP (ERMS) Tests
// ===========================================================================

func TestX86JIT_ERMS_STOSB_Large(t *testing.T) {
	if !x86Host.HasERMS {
		t.Skip("ERMS not available")
	}
	r := newX86JITTestRig(t)
	r.cpu.EAX = 0xAB // fill byte
	r.cpu.ECX = 128  // large count, exercises ERMS hardware REP path
	r.cpu.EDI = 0x5000

	// REP STOSB: F3 AA -- on ERMS CPUs this uses native hardware REP STOSB
	r.compileAndRun(t, 0x1000, 0xF3, 0xAA)

	for i := uint32(0); i < 128; i++ {
		if r.cpu.memory[0x5000+i] != 0xAB {
			t.Errorf("memory[0x%X] = 0x%02X, want 0xAB", 0x5000+i, r.cpu.memory[0x5000+i])
			break
		}
	}
	if r.cpu.ECX != 0 {
		t.Errorf("ECX = %d, want 0", r.cpu.ECX)
	}
}

func TestX86JIT_ERMS_MOVSB_Large(t *testing.T) {
	if !x86Host.HasERMS {
		t.Skip("ERMS not available")
	}
	r := newX86JITTestRig(t)
	// Write source data
	for i := byte(0); i < 128; i++ {
		r.cpu.memory[0x5000+uint32(i)] = i + 1
	}
	r.cpu.ESI = 0x5000
	r.cpu.EDI = 0x6000
	r.cpu.ECX = 128

	// REP MOVSB -- on ERMS CPUs uses native hardware REP MOVSB
	r.compileAndRun(t, 0x1000, 0xF3, 0xA4)

	for i := uint32(0); i < 128; i++ {
		if r.cpu.memory[0x6000+i] != byte(i+1) {
			t.Errorf("memory[0x%X] = 0x%02X, want 0x%02X", 0x6000+i, r.cpu.memory[0x6000+i], byte(i+1))
			break
		}
	}
	if r.cpu.ECX != 0 {
		t.Errorf("ECX = %d, want 0", r.cpu.ECX)
	}
	if r.cpu.ESI != 0x5080 { // 0x5000 + 128
		t.Errorf("ESI = 0x%X, want 0x5080", r.cpu.ESI)
	}
	if r.cpu.EDI != 0x6080 { // 0x6000 + 128
		t.Errorf("EDI = 0x%X, want 0x6080", r.cpu.EDI)
	}
}

func TestX86JIT_LEAVE(t *testing.T) {
	r := newX86JITTestRig(t)
	r.cpu.EBP = 0x10004
	r.cpu.ESP = 0x0FF00
	// Write a value at [0x10004] for POP EBP to read
	r.cpu.memory[0x10004] = 0x78
	r.cpu.memory[0x10005] = 0x56
	r.cpu.memory[0x10006] = 0x34
	r.cpu.memory[0x10007] = 0x12

	// LEAVE: C9
	r.compileAndRun(t, 0x1000, 0xC9)

	if r.cpu.ESP != 0x10008 {
		t.Errorf("ESP = 0x%08X, want 0x10008", r.cpu.ESP)
	}
	if r.cpu.EBP != 0x12345678 {
		t.Errorf("EBP = 0x%08X, want 0x12345678", r.cpu.EBP)
	}
}
