// jit_m68k_emit_amd64_test.go - Tests for M68020 x86-64 JIT emitter

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// M68K JIT Test Rig
// ===========================================================================

type m68kJITTestRig struct {
	cpu     *M68KCPU
	execMem *ExecMem
	ctx     *M68KJITContext
	bitmap  []byte
}

func newM68KJITTestRig(t *testing.T) *m68kJITTestRig {
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP
	bus.Write32(4, 0x00001000) // PC
	cpu := NewM68KCPU(bus)

	em, err := AllocExecMem(1 << 20) // 1MB
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	t.Cleanup(func() { em.Free() })

	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	ctx := newM68KJITContext(cpu, bitmap)

	return &m68kJITTestRig{cpu: cpu, execMem: em, ctx: ctx, bitmap: bitmap}
}

// compileAndRun writes M68K instructions to memory, appends TRAP #0 as
// block terminator, scans, strips the TRAP, compiles, and executes.
func (r *m68kJITTestRig) compileAndRun(t *testing.T, startPC uint32, opcodes ...uint16) {
	t.Helper()

	// Write opcodes to memory in big-endian, followed by TRAP #0 (terminator)
	pc := startPC
	for _, op := range opcodes {
		r.cpu.memory[pc] = byte(op >> 8)
		r.cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	// Append TRAP #0 as block terminator so scanner stops
	r.cpu.memory[pc] = 0x4E
	r.cpu.memory[pc+1] = 0x40
	pc += 2

	// Scan block
	instrs := m68kScanBlock(r.cpu.memory, startPC)
	if len(instrs) == 0 {
		t.Fatal("m68kScanBlock returned 0 instructions")
	}

	// Strip the TRAP #0 terminator — we don't want to compile/execute it
	compilable := instrs
	if compilable[len(compilable)-1].opcode&0xFFF0 == 0x4E40 {
		compilable = compilable[:len(compilable)-1]
	}
	if len(compilable) == 0 {
		t.Fatal("no compilable instructions after stripping terminator")
	}

	// Compile (pass memory for branch displacement decoding)
	r.execMem.Reset()
	block, err := m68kCompileBlockWithMem(compilable, startPC, r.execMem, r.cpu.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlock: %v", err)
	}

	// Update context pointers (may have changed)
	r.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&r.cpu.DataRegs[0]))
	r.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&r.cpu.AddrRegs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	r.ctx.SRPtr = uintptr(unsafe.Pointer(&r.cpu.SR))

	// Execute
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	// Read back RetPC
	r.cpu.PC = r.ctx.RetPC
}

// writeWord writes a big-endian word to memory at addr
func (r *m68kJITTestRig) writeWord(addr uint32, val uint16) {
	r.cpu.memory[addr] = byte(val >> 8)
	r.cpu.memory[addr+1] = byte(val)
}

// writeLong writes a big-endian long to memory at addr
func (r *m68kJITTestRig) writeLong(addr uint32, val uint32) {
	r.cpu.memory[addr] = byte(val >> 24)
	r.cpu.memory[addr+1] = byte(val >> 16)
	r.cpu.memory[addr+2] = byte(val >> 8)
	r.cpu.memory[addr+3] = byte(val)
}

// ===========================================================================
// MOVEQ Tests
// ===========================================================================

func TestM68KJIT_AMD64_MOVEQ_Positive(t *testing.T) {
	r := newM68KJITTestRig(t)
	// MOVEQ #42,D0 + NOP (NOP to avoid empty block after scan)
	r.compileAndRun(t, 0x1000, 0x702A) // MOVEQ #42,D0
	if r.cpu.DataRegs[0] != 42 {
		t.Errorf("D0 = %d, want 42", r.cpu.DataRegs[0])
	}
	// Check flags: N=0, Z=0, V=0, C=0
	if r.cpu.SR&0x0F != 0 { // NZVC should be 0
		t.Errorf("SR flags = 0x%02X, want N=0 Z=0 V=0 C=0", r.cpu.SR&0x1F)
	}
}

func TestM68KJIT_AMD64_MOVEQ_Negative(t *testing.T) {
	r := newM68KJITTestRig(t)
	// MOVEQ #-1,D0
	r.compileAndRun(t, 0x1000, 0x70FF) // MOVEQ #-1,D0
	if r.cpu.DataRegs[0] != 0xFFFFFFFF {
		t.Errorf("D0 = 0x%08X, want 0xFFFFFFFF", r.cpu.DataRegs[0])
	}
	// N flag should be set
	if r.cpu.SR&M68K_SR_N == 0 {
		t.Error("N flag should be set for negative value")
	}
}

func TestM68KJIT_AMD64_MOVEQ_Zero(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x12345678     // pre-set to non-zero
	r.compileAndRun(t, 0x1000, 0x7000) // MOVEQ #0,D0
	if r.cpu.DataRegs[0] != 0 {
		t.Errorf("D0 = 0x%08X, want 0", r.cpu.DataRegs[0])
	}
	// Z flag should be set
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be set for zero value")
	}
}

func TestM68KJIT_AMD64_MOVEQ_D3(t *testing.T) {
	r := newM68KJITTestRig(t)
	// MOVEQ #100,D3 (D3 is spilled, not mapped)
	r.compileAndRun(t, 0x1000, 0x7664) // MOVEQ #100,D3
	if r.cpu.DataRegs[3] != 100 {
		t.Errorf("D3 = %d, want 100", r.cpu.DataRegs[3])
	}
}

// ===========================================================================
// MOVE Dn,Dm Tests
// ===========================================================================

func TestM68KJIT_AMD64_MOVE_L_D0_D1(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xDEADBEEF
	// MOVE.L D0,D1 (0x2200)
	r.compileAndRun(t, 0x1000, 0x2200)
	if r.cpu.DataRegs[1] != 0xDEADBEEF {
		t.Errorf("D1 = 0x%08X, want 0xDEADBEEF", r.cpu.DataRegs[1])
	}
}

func TestM68KJIT_AMD64_MOVE_L_D2_D3(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[2] = 0x12345678
	// MOVE.L D2,D3 (0x2602)
	r.compileAndRun(t, 0x1000, 0x2602)
	if r.cpu.DataRegs[3] != 0x12345678 {
		t.Errorf("D3 = 0x%08X, want 0x12345678", r.cpu.DataRegs[3])
	}
}

// ===========================================================================
// ADD Tests
// ===========================================================================

func TestM68KJIT_AMD64_ADD_L_D0_D1(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 100
	r.cpu.DataRegs[1] = 200
	// ADD.L D0,D1 (0xD280)
	r.compileAndRun(t, 0x1000, 0xD280)
	if r.cpu.DataRegs[1] != 300 {
		t.Errorf("D1 = %d, want 300", r.cpu.DataRegs[1])
	}
}

func TestM68KJIT_AMD64_ADD_L_Overflow(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x7FFFFFFF
	r.cpu.DataRegs[1] = 1
	// ADD.L D0,D1
	r.compileAndRun(t, 0x1000, 0xD280)
	if r.cpu.DataRegs[1] != 0x80000000 {
		t.Errorf("D1 = 0x%08X, want 0x80000000", r.cpu.DataRegs[1])
	}
	// V flag should be set (signed overflow)
	if r.cpu.SR&M68K_SR_V == 0 {
		t.Error("V flag should be set for signed overflow")
	}
	// N flag should be set (negative result)
	if r.cpu.SR&M68K_SR_N == 0 {
		t.Error("N flag should be set for negative result")
	}
}

func TestM68KJIT_AMD64_ADD_L_Carry(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFFFFFFFF
	r.cpu.DataRegs[1] = 1
	// ADD.L D0,D1
	r.compileAndRun(t, 0x1000, 0xD280)
	if r.cpu.DataRegs[1] != 0 {
		t.Errorf("D1 = 0x%08X, want 0", r.cpu.DataRegs[1])
	}
	// C and X flags should be set
	if r.cpu.SR&M68K_SR_C == 0 {
		t.Error("C flag should be set for carry")
	}
	if r.cpu.SR&M68K_SR_X == 0 {
		t.Error("X flag should be set (X=C for arithmetic)")
	}
	// Z flag should be set
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be set for zero result")
	}
}

// ===========================================================================
// SUB Tests
// ===========================================================================

func TestM68KJIT_AMD64_SUB_L_D0_D1(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 50
	r.cpu.DataRegs[1] = 200
	// SUB.L D0,D1 (0x9280)
	r.compileAndRun(t, 0x1000, 0x9280)
	if r.cpu.DataRegs[1] != 150 {
		t.Errorf("D1 = %d, want 150", r.cpu.DataRegs[1])
	}
}

// ===========================================================================
// AND/OR/EOR/NOT Tests
// ===========================================================================

func TestM68KJIT_AMD64_AND_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFF00FF00
	r.cpu.DataRegs[1] = 0x12345678
	// AND.L D0,D1 (0xC280)
	r.compileAndRun(t, 0x1000, 0xC280)
	if r.cpu.DataRegs[1] != 0x12005600 {
		t.Errorf("D1 = 0x%08X, want 0x12005600", r.cpu.DataRegs[1])
	}
}

func TestM68KJIT_AMD64_OR_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x00FF0000
	r.cpu.DataRegs[1] = 0x000000FF
	// OR.L D0,D1 (0x8280)
	r.compileAndRun(t, 0x1000, 0x8280)
	if r.cpu.DataRegs[1] != 0x00FF00FF {
		t.Errorf("D1 = 0x%08X, want 0x00FF00FF", r.cpu.DataRegs[1])
	}
}

func TestM68KJIT_AMD64_NOT_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x00000000
	// NOT.L D0 (0x4680)
	r.compileAndRun(t, 0x1000, 0x4680)
	if r.cpu.DataRegs[0] != 0xFFFFFFFF {
		t.Errorf("D0 = 0x%08X, want 0xFFFFFFFF", r.cpu.DataRegs[0])
	}
	if r.cpu.SR&M68K_SR_N == 0 {
		t.Error("N flag should be set")
	}
}

// ===========================================================================
// NEG Tests
// ===========================================================================

func TestM68KJIT_AMD64_NEG_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 42
	// NEG.L D0 (0x4480)
	r.compileAndRun(t, 0x1000, 0x4480)
	want := uint32(0xFFFFFFD6) // -42 as uint32
	if r.cpu.DataRegs[0] != want {
		t.Errorf("D0 = 0x%08X, want 0x%08X", r.cpu.DataRegs[0], want)
	}
}

// ===========================================================================
// CLR Tests
// ===========================================================================

func TestM68KJIT_AMD64_CLR_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xDEADBEEF
	// CLR.L D0 (0x4280)
	r.compileAndRun(t, 0x1000, 0x4280)
	if r.cpu.DataRegs[0] != 0 {
		t.Errorf("D0 = 0x%08X, want 0", r.cpu.DataRegs[0])
	}
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be set")
	}
}

// ===========================================================================
// TST Tests
// ===========================================================================

func TestM68KJIT_AMD64_TST_L_Zero(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0
	// TST.L D0 (0x4A80)
	r.compileAndRun(t, 0x1000, 0x4A80)
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be set for zero")
	}
	if r.cpu.SR&M68K_SR_N != 0 {
		t.Error("N flag should be clear for zero")
	}
}

func TestM68KJIT_AMD64_TST_L_Negative(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x80000000
	// TST.L D0
	r.compileAndRun(t, 0x1000, 0x4A80)
	if r.cpu.SR&M68K_SR_N == 0 {
		t.Error("N flag should be set for negative")
	}
	if r.cpu.SR&M68K_SR_Z != 0 {
		t.Error("Z flag should be clear for non-zero")
	}
}

// ===========================================================================
// SWAP Tests
// ===========================================================================

func TestM68KJIT_AMD64_SWAP(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x12345678
	// SWAP D0 (0x4840)
	r.compileAndRun(t, 0x1000, 0x4840)
	if r.cpu.DataRegs[0] != 0x56781234 {
		t.Errorf("D0 = 0x%08X, want 0x56781234", r.cpu.DataRegs[0])
	}
}

// ===========================================================================
// EXT Tests
// ===========================================================================

func TestM68KJIT_AMD64_EXT_W(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x000000FF // byte -1
	// EXT.W D0 (0x4880)
	r.compileAndRun(t, 0x1000, 0x4880)
	// Byte 0xFF sign-extends to word 0xFFFF, upper word preserved
	if r.cpu.DataRegs[0]&0xFFFF != 0xFFFF {
		t.Errorf("D0 low word = 0x%04X, want 0xFFFF", r.cpu.DataRegs[0]&0xFFFF)
	}
}

func TestM68KJIT_AMD64_EXT_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x0000FFFF // word -1
	// EXT.L D0 (0x48C0)
	r.compileAndRun(t, 0x1000, 0x48C0)
	if r.cpu.DataRegs[0] != 0xFFFFFFFF {
		t.Errorf("D0 = 0x%08X, want 0xFFFFFFFF", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_EXTB_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x000000FE // byte -2
	// EXTB.L D0 (0x49C0)
	r.compileAndRun(t, 0x1000, 0x49C0)
	if r.cpu.DataRegs[0] != 0xFFFFFFFE {
		t.Errorf("D0 = 0x%08X, want 0xFFFFFFFE", r.cpu.DataRegs[0])
	}
}

// ===========================================================================
// Multi-instruction Tests
// ===========================================================================

func TestM68KJIT_AMD64_MOVEQ_ADD_Sequence(t *testing.T) {
	r := newM68KJITTestRig(t)
	// MOVEQ #10,D0; MOVEQ #20,D1; ADD.L D0,D1
	r.compileAndRun(t, 0x1000,
		0x700A, // MOVEQ #10,D0
		0x7214, // MOVEQ #20,D1
		0xD280, // ADD.L D0,D1
	)
	if r.cpu.DataRegs[0] != 10 {
		t.Errorf("D0 = %d, want 10", r.cpu.DataRegs[0])
	}
	if r.cpu.DataRegs[1] != 30 {
		t.Errorf("D1 = %d, want 30", r.cpu.DataRegs[1])
	}
}

func TestM68KJIT_AMD64_RetPC(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Two 2-byte instructions at 0x1000
	r.compileAndRun(t, 0x1000,
		0x702A, // MOVEQ #42,D0 (2 bytes)
		0x7200, // MOVEQ #0,D1 (2 bytes)
	)
	// RetPC should be 0x1004 (after both instructions)
	if r.cpu.PC != 0x1004 {
		t.Errorf("PC = 0x%08X, want 0x1004", r.cpu.PC)
	}
}

// ===========================================================================
// Control Flow Tests — Stage 3
// ===========================================================================

func TestM68KJIT_AMD64_BRA_Byte(t *testing.T) {
	r := newM68KJITTestRig(t)
	// BRA.B +4 at 0x1000 → target = 0x1000 + 2 + 4 = 0x1006
	r.compileAndRun(t, 0x1000, 0x6004) // BRA.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BRA_Word(t *testing.T) {
	r := newM68KJITTestRig(t)
	// BRA.W +$100 at 0x1000 → target = 0x1000 + 2 + 0x100 = 0x1102
	r.writeWord(0x1000, 0x6000)
	r.writeWord(0x1002, 0x0100)
	r.compileAndRun(t, 0x1000, 0x6000, 0x0100) // BRA.W +256
	if r.cpu.PC != 0x1102 {
		t.Errorf("PC = 0x%08X, want 0x1102", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BEQ_Taken(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Set Z flag (CCR Z=1)
	r.cpu.SR |= M68K_SR_Z
	// BEQ.B +4 → should be taken
	r.compileAndRun(t, 0x1000, 0x6704) // BEQ.B +4
	// Target = 0x1000 + 2 + 4 = 0x1006
	if r.cpu.PC != 0x1006 {
		t.Errorf("BEQ taken: PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BEQ_NotTaken(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Clear Z flag
	r.cpu.SR &^= M68K_SR_Z
	// BEQ.B +4 followed by MOVEQ
	// If not taken, falls through to MOVEQ, then block ends
	r.compileAndRun(t, 0x1000,
		0x6704, // BEQ.B +4 (not taken, Z=0)
		0x702A, // MOVEQ #42,D0 (falls through here)
	)
	if r.cpu.DataRegs[0] != 42 {
		t.Errorf("BEQ not taken: D0 = %d, want 42", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_BNE_Taken(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.SR &^= M68K_SR_Z             // Z=0 → NE is true
	r.compileAndRun(t, 0x1000, 0x6604) // BNE.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("BNE taken: PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BCC_Taken(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.SR &^= M68K_SR_C             // C=0 → CC is true
	r.compileAndRun(t, 0x1000, 0x6404) // BCC.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("BCC taken: PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BMI_Taken(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.SR |= M68K_SR_N              // N=1 → MI is true
	r.compileAndRun(t, 0x1000, 0x6B04) // BMI.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("BMI taken: PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_RTS(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Set up stack: A7 = 0x10000, push return address 0x2000 (big-endian)
	r.cpu.AddrRegs[7] = 0x10000
	r.writeLong(0x10000, 0x00002000)   // return address at [A7] in big-endian
	r.compileAndRun(t, 0x1000, 0x4E75) // RTS
	if r.cpu.PC != 0x2000 {
		t.Errorf("RTS: PC = 0x%08X, want 0x2000", r.cpu.PC)
	}
	if r.cpu.AddrRegs[7] != 0x10004 {
		t.Errorf("RTS: A7 = 0x%08X, want 0x10004", r.cpu.AddrRegs[7])
	}
}

func TestM68KJIT_AMD64_JSR_AbsLong(t *testing.T) {
	r := newM68KJITTestRig(t)
	// A7 = 0x10004 (stack pointer)
	r.cpu.AddrRegs[7] = 0x10004
	// JSR $00002000 at 0x1000 (6 bytes: 0x4EB9 + 32-bit address)
	r.compileAndRun(t, 0x1000, 0x4EB9, 0x0000, 0x2000) // JSR $00002000
	// PC should be 0x2000 (target)
	if r.cpu.PC != 0x2000 {
		t.Errorf("JSR: PC = 0x%08X, want 0x2000", r.cpu.PC)
	}
	// A7 should be 0x10000 (pushed 4 bytes)
	if r.cpu.AddrRegs[7] != 0x10000 {
		t.Errorf("JSR: A7 = 0x%08X, want 0x10000", r.cpu.AddrRegs[7])
	}
	// Return address at [0x10000] should be 0x1006 (after JSR instruction)
	retAddr := uint32(r.cpu.memory[0x10000])<<24 | uint32(r.cpu.memory[0x10001])<<16 |
		uint32(r.cpu.memory[0x10002])<<8 | uint32(r.cpu.memory[0x10003])
	if retAddr != 0x1006 {
		t.Errorf("JSR: return addr = 0x%08X, want 0x1006", retAddr)
	}
}

func TestM68KJIT_AMD64_Scc_True(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.SR |= M68K_SR_Z // Z=1 → SEQ sets byte to 0xFF
	r.cpu.DataRegs[0] = 0x12345600
	// SEQ D0 (Scc with condition EQ=7): opcode = 0x57C0
	r.compileAndRun(t, 0x1000, 0x57C0) // SEQ D0
	if r.cpu.DataRegs[0]&0xFF != 0xFF {
		t.Errorf("SEQ true: D0 low byte = 0x%02X, want 0xFF", r.cpu.DataRegs[0]&0xFF)
	}
	// Upper bytes should be preserved
	if r.cpu.DataRegs[0]&0xFFFFFF00 != 0x12345600 {
		t.Errorf("SEQ true: D0 upper = 0x%08X, want 0x12345600", r.cpu.DataRegs[0]&0xFFFFFF00)
	}
}

func TestM68KJIT_AMD64_Scc_False(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.SR &^= M68K_SR_Z // Z=0 → SEQ sets byte to 0x00
	r.cpu.DataRegs[0] = 0x123456FF
	r.compileAndRun(t, 0x1000, 0x57C0) // SEQ D0
	if r.cpu.DataRegs[0]&0xFF != 0x00 {
		t.Errorf("SEQ false: D0 low byte = 0x%02X, want 0x00", r.cpu.DataRegs[0]&0xFF)
	}
}

func TestM68KJIT_AMD64_BGE_Taken(t *testing.T) {
	r := newM68KJITTestRig(t)
	// GE: N⊕V=0. Set N=1, V=1 → N⊕V=0 → GE true
	r.cpu.SR |= M68K_SR_N | M68K_SR_V
	r.compileAndRun(t, 0x1000, 0x6C04) // BGE.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("BGE taken (N=V=1): PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BLT_Taken(t *testing.T) {
	r := newM68KJITTestRig(t)
	// LT: N⊕V=1. Set N=1, V=0 → N⊕V=1 → LT true
	r.cpu.SR |= M68K_SR_N
	r.cpu.SR &^= M68K_SR_V
	r.compileAndRun(t, 0x1000, 0x6D04) // BLT.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("BLT taken (N=1,V=0): PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
}

// ===========================================================================
// Memory Access Tests — Stage 4
// ===========================================================================

func TestM68KJIT_AMD64_MOVE_L_Indirect_Read(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Store 0xDEADBEEF in big-endian at address 0x2000
	r.writeLong(0x2000, 0xDEADBEEF)
	r.cpu.AddrRegs[0] = 0x2000
	// MOVE.L (A0),D0 = 0x2010
	r.compileAndRun(t, 0x1000, 0x2010)
	if r.cpu.DataRegs[0] != 0xDEADBEEF {
		t.Errorf("MOVE.L (A0),D0: D0 = 0x%08X, want 0xDEADBEEF", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_L_Indirect_Write(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x12345678
	r.cpu.AddrRegs[0] = 0x2000
	// MOVE.L D0,(A0) = 0x2080
	r.compileAndRun(t, 0x1000, 0x2080)
	// Read back in big-endian
	got := uint32(r.cpu.memory[0x2000])<<24 | uint32(r.cpu.memory[0x2001])<<16 |
		uint32(r.cpu.memory[0x2002])<<8 | uint32(r.cpu.memory[0x2003])
	if got != 0x12345678 {
		t.Errorf("MOVE.L D0,(A0): [0x2000] = 0x%08X, want 0x12345678", got)
	}
}

func TestM68KJIT_AMD64_MOVE_L_PostIncrement(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.writeLong(0x2000, 0xAABBCCDD)
	r.cpu.AddrRegs[0] = 0x2000
	// MOVE.L (A0)+,D0 = 0x2018
	r.compileAndRun(t, 0x1000, 0x2018)
	if r.cpu.DataRegs[0] != 0xAABBCCDD {
		t.Errorf("MOVE.L (A0)+,D0: D0 = 0x%08X, want 0xAABBCCDD", r.cpu.DataRegs[0])
	}
	if r.cpu.AddrRegs[0] != 0x2004 {
		t.Errorf("MOVE.L (A0)+,D0: A0 = 0x%08X, want 0x2004", r.cpu.AddrRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_L_PreDecrement_Write(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFEEDFACE
	r.cpu.AddrRegs[7] = 0x10004
	// MOVE.L D0,-(A7) = 0x2F00
	r.compileAndRun(t, 0x1000, 0x2F00)
	if r.cpu.AddrRegs[7] != 0x10000 {
		t.Errorf("MOVE.L D0,-(A7): A7 = 0x%08X, want 0x10000", r.cpu.AddrRegs[7])
	}
	got := uint32(r.cpu.memory[0x10000])<<24 | uint32(r.cpu.memory[0x10001])<<16 |
		uint32(r.cpu.memory[0x10002])<<8 | uint32(r.cpu.memory[0x10003])
	if got != 0xFEEDFACE {
		t.Errorf("MOVE.L D0,-(A7): [A7] = 0x%08X, want 0xFEEDFACE", got)
	}
}

func TestM68KJIT_AMD64_MOVE_L_Displacement(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.writeLong(0x2010, 0x11223344) // data at A0 + 16
	r.cpu.AddrRegs[0] = 0x2000
	// MOVE.L (16,A0),D0 = 0x2028 0x0010
	r.compileAndRun(t, 0x1000, 0x2028, 0x0010)
	if r.cpu.DataRegs[0] != 0x11223344 {
		t.Errorf("MOVE.L (d16,A0),D0: D0 = 0x%08X, want 0x11223344", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_L_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	// MOVE.L #$DEADBEEF,D0 = 0x203C 0xDEAD 0xBEEF
	r.compileAndRun(t, 0x1000, 0x203C, 0xDEAD, 0xBEEF)
	if r.cpu.DataRegs[0] != 0xDEADBEEF {
		t.Errorf("MOVE.L #imm,D0: D0 = 0x%08X, want 0xDEADBEEF", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_W_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFFFF0000 // upper word preset
	// MOVE.W #$1234,D0 = 0x303C 0x1234
	r.compileAndRun(t, 0x1000, 0x303C, 0x1234)
	// Low word should be 0x1234, upper word preserved
	if r.cpu.DataRegs[0] != 0xFFFF1234 {
		t.Errorf("MOVE.W #imm,D0: D0 = 0x%08X, want 0xFFFF1234", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_L_AbsLong(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.writeLong(0x3000, 0x55667788)
	// MOVE.L ($00003000).L,D0 = 0x2039 0x0000 0x3000
	r.compileAndRun(t, 0x1000, 0x2039, 0x0000, 0x3000)
	if r.cpu.DataRegs[0] != 0x55667788 {
		t.Errorf("MOVE.L abs.L,D0: D0 = 0x%08X, want 0x55667788", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_L_PostInc_Chain(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.writeLong(0x2000, 0x11111111)
	r.writeLong(0x2004, 0x22222222)
	r.cpu.AddrRegs[0] = 0x2000
	// MOVE.L (A0)+,D0; MOVE.L (A0)+,D1
	r.compileAndRun(t, 0x1000,
		0x2018, // MOVE.L (A0)+,D0
		0x2218, // MOVE.L (A0)+,D1
	)
	if r.cpu.DataRegs[0] != 0x11111111 {
		t.Errorf("D0 = 0x%08X, want 0x11111111", r.cpu.DataRegs[0])
	}
	if r.cpu.DataRegs[1] != 0x22222222 {
		t.Errorf("D1 = 0x%08X, want 0x22222222", r.cpu.DataRegs[1])
	}
	if r.cpu.AddrRegs[0] != 0x2008 {
		t.Errorf("A0 = 0x%08X, want 0x2008", r.cpu.AddrRegs[0])
	}
}

func TestM68KJIT_AMD64_MOVE_B_Indirect(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.memory[0x2000] = 0x42
	r.cpu.AddrRegs[0] = 0x2000
	// MOVE.B (A0),D0 = 0x1010
	r.compileAndRun(t, 0x1000, 0x1010)
	if r.cpu.DataRegs[0]&0xFF != 0x42 {
		t.Errorf("MOVE.B (A0),D0: D0 low byte = 0x%02X, want 0x42", r.cpu.DataRegs[0]&0xFF)
	}
}

// ===========================================================================
// Stage 5: Extended Instruction Tests
// ===========================================================================

func TestM68KJIT_AMD64_LEA_Disp(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[0] = 0x2000
	// LEA (16,A0),A1 = 0x43E8 0x0010
	r.compileAndRun(t, 0x1000, 0x43E8, 0x0010)
	if r.cpu.AddrRegs[1] != 0x2010 {
		t.Errorf("LEA (d16,A0),A1: A1 = 0x%08X, want 0x2010", r.cpu.AddrRegs[1])
	}
}

func TestM68KJIT_AMD64_LEA_Indirect(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[0] = 0x5000
	// LEA (A0),A2 = 0x45D0
	r.compileAndRun(t, 0x1000, 0x45D0)
	if r.cpu.AddrRegs[2] != 0x5000 {
		t.Errorf("LEA (A0),A2: A2 = 0x%08X, want 0x5000", r.cpu.AddrRegs[2])
	}
}

func TestM68KJIT_AMD64_LINK_UNLK(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[6] = 0xAAAAAAAA // old A6
	r.cpu.AddrRegs[7] = 0x10020    // SP
	// LINK A6,#-8 = 0x4E56 0xFFF8
	// UNLK A6 = 0x4E5E
	r.compileAndRun(t, 0x1000,
		0x4E56, 0xFFF8, // LINK A6,#-8
		0x4E5E, // UNLK A6
	)
	// After LINK+UNLK: A6 should be restored, A7 should be back to original
	if r.cpu.AddrRegs[6] != 0xAAAAAAAA {
		t.Errorf("LINK+UNLK: A6 = 0x%08X, want 0xAAAAAAAA", r.cpu.AddrRegs[6])
	}
	if r.cpu.AddrRegs[7] != 0x10020 {
		t.Errorf("LINK+UNLK: A7 = 0x%08X, want 0x10020", r.cpu.AddrRegs[7])
	}
}

func TestM68KJIT_AMD64_ADDQ_Dn(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 100
	// ADDQ.L #3,D0 = 0x5680
	r.compileAndRun(t, 0x1000, 0x5680)
	if r.cpu.DataRegs[0] != 103 {
		t.Errorf("ADDQ #3,D0: D0 = %d, want 103", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_ADDQ_8(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0
	// ADDQ.L #8,D0 (data=0 encodes 8) = 0x5080
	r.compileAndRun(t, 0x1000, 0x5080)
	if r.cpu.DataRegs[0] != 8 {
		t.Errorf("ADDQ #8,D0: D0 = %d, want 8", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_SUBQ_Dn(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 100
	// SUBQ.L #1,D0 = 0x5380
	r.compileAndRun(t, 0x1000, 0x5380)
	if r.cpu.DataRegs[0] != 99 {
		t.Errorf("SUBQ #1,D0: D0 = %d, want 99", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_ADDQ_An(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[0] = 0x1000
	// ADDQ.L #4,A0: 0101_100_0_10_001_000 = 0x5888
	r.compileAndRun(t, 0x1000, 0x5888)
	if r.cpu.AddrRegs[0] != 0x1004 {
		t.Errorf("ADDQ #4,A0: A0 = 0x%08X, want 0x1004", r.cpu.AddrRegs[0])
	}
}

func TestM68KJIT_AMD64_LSL_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 1
	// LSL.L #4,D0: 0xE988
	// Encoding: 1110 ccc 1 10 i 01 rrr where c=count, i=0(imm), r=reg
	// count=4, size=10(.L), type=01(LS), dir=1(left), reg=0
	// 1110 100 1 10 0 01 000 = 0xE988
	r.compileAndRun(t, 0x1000, 0xE988)
	if r.cpu.DataRegs[0] != 16 {
		t.Errorf("LSL.L #4,D0: D0 = %d, want 16", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_LSR_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 256
	// LSR.L #4,D0: 0xE888
	// 1110 100 0 10 0 01 000 = 0xE888
	r.compileAndRun(t, 0x1000, 0xE888)
	if r.cpu.DataRegs[0] != 16 {
		t.Errorf("LSR.L #4,D0: D0 = %d, want 16", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_ASR_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0x80000000 // -2147483648
	// ASR.L #1,D0: 0xE280
	// 1110 001 0 10 0 00 000 = 0xE280
	r.compileAndRun(t, 0x1000, 0xE280)
	if r.cpu.DataRegs[0] != 0xC0000000 { // sign-extended right shift
		t.Errorf("ASR.L #1,D0: D0 = 0x%08X, want 0xC0000000", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_ADDA_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 100
	r.cpu.AddrRegs[1] = 0x2000
	// ADDA.L D0,A1: 0xD3C0
	// Group D: 1101 rrr 111 mmm sss where rrr=1(A1), opmode=111(.L), mmm=000(Dn), sss=000(D0)
	// 1101 001 111 000 000 = 0xD3C0
	r.compileAndRun(t, 0x1000, 0xD3C0)
	if r.cpu.AddrRegs[1] != 0x2064 {
		t.Errorf("ADDA.L D0,A1: A1 = 0x%08X, want 0x2064", r.cpu.AddrRegs[1])
	}
}

func TestM68KJIT_AMD64_SUBA_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 100
	r.cpu.AddrRegs[1] = 0x2064
	// SUBA.L D0,A1: 0x93C0
	r.compileAndRun(t, 0x1000, 0x93C0)
	if r.cpu.AddrRegs[1] != 0x2000 {
		t.Errorf("SUBA.L D0,A1: A1 = 0x%08X, want 0x2000", r.cpu.AddrRegs[1])
	}
}

// ===========================================================================
// Gap Fix Tests — DBcc, Memory ALU
// ===========================================================================

func TestM68KJIT_AMD64_DBRA_SinglePass(t *testing.T) {
	// Test that DBRA correctly decrements and exits block when not exhausted.
	// The emitter test rig only runs one block execution, so DBRA will decrement
	// D0 from 3→2 and exit with RetPC=target (0x1000). We verify the decrement worked.
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 3
	r.compileAndRun(t, 0x1000,
		0x51C8, 0xFFFC, // DBRA D0,target (disp=-4, target = 0x1000+2+(-4) = 0xFFE)
	)
	// D0 low word should be 2 (decremented from 3)
	if r.cpu.DataRegs[0]&0xFFFF != 2 {
		t.Errorf("DBRA: D0.W = %d, want 2", r.cpu.DataRegs[0]&0xFFFF)
	}
	// RetPC should be 0xFFE (branch target, not exhausted)
	if r.cpu.PC != 0xFFE {
		t.Errorf("DBRA: PC = 0x%08X, want 0xFFE (branch taken)", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_ADD_L_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 100
	// ADD.L #50,D0: source is immediate (#imm), dest is D0
	// Actually this is ADDI.L which is group 0. Let me use ADD.L #imm,D0 instead.
	// ADD.L <ea>,Dn with ea=#imm: opcode for ADD.L #imm,D0
	// Group D: 1101 rrr ooo mmm sss
	// ADD.L <ea>,D0: rrr=000(D0), ooo=010(.L, EA→Dn), mmm=111, sss=100(#imm)
	// 1101 000 010 111 100 = 0xD0BC
	r.compileAndRun(t, 0x1000, 0xD0BC, 0x0000, 0x0032) // ADD.L #50,D0
	if r.cpu.DataRegs[0] != 150 {
		t.Errorf("ADD.L #50,D0: D0 = %d, want 150", r.cpu.DataRegs[0])
	}
}

func TestM68KJIT_AMD64_CMP_L_Immediate(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 42
	// CMP.L #42,D0: Group B, opmode=010(.L), src=#imm
	// 1011 000 010 111 100 = 0xB0BC
	r.compileAndRun(t, 0x1000, 0xB0BC, 0x0000, 0x002A) // CMP.L #42,D0
	// Z flag should be set (D0 - 42 = 0)
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Errorf("CMP.L #42,D0: Z flag should be set (values equal)")
	}
}

func TestM68KJIT_AMD64_SUB_L_Indirect(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 200
	r.cpu.AddrRegs[0] = 0x2000
	r.writeLong(0x2000, 50)
	// SUB.L (A0),D0: 1001 000 010 010 000 = 0x9090
	r.compileAndRun(t, 0x1000, 0x9090) // SUB.L (A0),D0
	if r.cpu.DataRegs[0] != 150 {
		t.Errorf("SUB.L (A0),D0: D0 = %d, want 150", r.cpu.DataRegs[0])
	}
}

// ===========================================================================
// Missing Unit Tests (audit gaps)
// ===========================================================================

func TestM68KJIT_AMD64_EOR_L(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFF00FF00
	r.cpu.DataRegs[1] = 0x0F0F0F0F
	// EOR.L D0,D1: Group B, opmode=110(.L, Dn→EA), mode=000(Dn), reg=001(D1)
	// 1011 000 110 000 001 = 0xB181
	r.compileAndRun(t, 0x1000, 0xB181)
	if r.cpu.DataRegs[1] != 0xF00FF00F {
		t.Errorf("EOR.L D0,D1: D1 = 0x%08X, want 0xF00FF00F", r.cpu.DataRegs[1])
	}
}

func TestM68KJIT_AMD64_JMP_AbsLong(t *testing.T) {
	r := newM68KJITTestRig(t)
	// JMP $00002000: 0x4EF9 + abs.L
	r.compileAndRun(t, 0x1000, 0x4EF9, 0x0000, 0x2000)
	if r.cpu.PC != 0x2000 {
		t.Errorf("JMP abs.L: PC = 0x%08X, want 0x2000", r.cpu.PC)
	}
}

func TestM68KJIT_AMD64_BSR_Byte(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[7] = 0x10004
	// BSR.B +4 at 0x1000: target = 0x1000 + 2 + 4 = 0x1006
	r.compileAndRun(t, 0x1000, 0x6104) // BSR.B +4
	if r.cpu.PC != 0x1006 {
		t.Errorf("BSR.B: PC = 0x%08X, want 0x1006", r.cpu.PC)
	}
	if r.cpu.AddrRegs[7] != 0x10000 {
		t.Errorf("BSR.B: A7 = 0x%08X, want 0x10000 (pushed 4 bytes)", r.cpu.AddrRegs[7])
	}
	// Return address at stack should be 0x1002 (after BSR.B instruction)
	retAddr := uint32(r.cpu.memory[0x10000])<<24 | uint32(r.cpu.memory[0x10001])<<16 |
		uint32(r.cpu.memory[0x10002])<<8 | uint32(r.cpu.memory[0x10003])
	if retAddr != 0x1002 {
		t.Errorf("BSR.B: return addr = 0x%08X, want 0x1002", retAddr)
	}
}

func TestM68KJIT_AMD64_PEA_Disp(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[0] = 0x2000
	r.cpu.AddrRegs[7] = 0x10004
	// PEA (16,A0) = push address A0+16 = 0x2010
	// PEA: 0x4868 (mode 5 = d16,An for A0) + disp
	// 0100 1000 01 101 000 = 0x4868
	r.compileAndRun(t, 0x1000, 0x4868, 0x0010)
	if r.cpu.AddrRegs[7] != 0x10000 {
		t.Errorf("PEA: A7 = 0x%08X, want 0x10000", r.cpu.AddrRegs[7])
	}
	// Pushed value should be 0x2010
	pushed := uint32(r.cpu.memory[0x10000])<<24 | uint32(r.cpu.memory[0x10001])<<16 |
		uint32(r.cpu.memory[0x10002])<<8 | uint32(r.cpu.memory[0x10003])
	if pushed != 0x2010 {
		t.Errorf("PEA: pushed = 0x%08X, want 0x2010", pushed)
	}
}

// ===========================================================================
// Lazy CCR Tests (Stage 5)
// ===========================================================================

// TestM68KJIT_AMD64_LazyCCR_XPreserved verifies that X flag is preserved
// across logical operations in lazy CCR mode.
func TestM68KJIT_AMD64_LazyCCR_XPreserved(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFFFFFFFF
	r.cpu.DataRegs[1] = 1

	// ADD.L D1,D0 → 0 with C=1, X=1; AND.L D0,D0 (=0, should preserve X)
	// ADD: 0xD081; AND.L D0,D0: 0xC080 (1100 000 000 000 000)
	r.compileAndRun(t, 0x1000, 0xD081, 0xC080)

	// X flag should be set (from ADD's carry, preserved by AND)
	if r.cpu.SR&M68K_SR_X == 0 {
		t.Error("X flag should be preserved across AND (set by ADD carry)")
	}
	// C and V should be cleared by AND
	if r.cpu.SR&M68K_SR_C != 0 {
		t.Error("C flag should be cleared by AND")
	}
	if r.cpu.SR&M68K_SR_V != 0 {
		t.Error("V flag should be cleared by AND")
	}
}

// TestM68KJIT_AMD64_LazyCCR_BlockExit verifies that lazy CCR is correctly
// materialized into SR at block exit.
func TestM68KJIT_AMD64_LazyCCR_BlockExit(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0

	// TST.L D0 (result is 0 → Z=1, N=0)
	// TST.L D0: 0x4A80
	r.compileAndRun(t, 0x1000, 0x4A80)

	// Z should be set at block exit
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be set for TST of zero (materialized at block exit)")
	}
	if r.cpu.SR&M68K_SR_N != 0 {
		t.Error("N flag should be clear for TST of zero")
	}
}

// TestM68KJIT_AMD64_LazyCCR_XInitFromPrologue verifies that the X flag
// stack slot is correctly seeded from SR on block entry.
func TestM68KJIT_AMD64_LazyCCR_XInitFromPrologue(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Set X=1 in SR before block entry
	r.cpu.SR |= M68K_SR_X
	r.cpu.DataRegs[0] = 0xFF

	// AND.L D0,D0 — logical op, X should be preserved from incoming SR
	// AND.L D0,D0: 0xC080
	r.compileAndRun(t, 0x1000, 0xC080)

	// X should still be set (seeded from prologue, preserved by AND)
	if r.cpu.SR&M68K_SR_X == 0 {
		t.Error("X flag should be preserved from incoming SR through AND (logical op)")
	}
}

// TestM68KJIT_AMD64_LazyCCR_CMP_PreservesX verifies that CMP does NOT
// overwrite the X flag. X should be preserved from the previous arithmetic op.
func TestM68KJIT_AMD64_LazyCCR_CMP_PreservesX(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.DataRegs[0] = 0xFFFFFFFF
	r.cpu.DataRegs[1] = 1
	r.cpu.DataRegs[2] = 5
	r.cpu.DataRegs[3] = 5

	// ADD.L D1,D0 → carry, X=1; CMP.L D3,D2 → equal, X should stay 1
	// ADD: 0xD081; CMP.L D3,D2: 0xB483 (1011 010 010 000 011)
	r.compileAndRun(t, 0x1000, 0xD081, 0xB483)

	// X must be 1 (from ADD's carry, preserved by CMP)
	if r.cpu.SR&M68K_SR_X == 0 {
		t.Error("X flag should be 1 (set by ADD carry, preserved by CMP)")
	}
	// Z must be 1 (from CMP of equal values)
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be 1 (CMP of equal values)")
	}
	// C must be 0 (CMP 5-5 = 0, no borrow)
	if r.cpu.SR&M68K_SR_C != 0 {
		t.Error("C flag should be 0 (CMP 5-5 has no borrow)")
	}
}

// TestM68KJIT_AMD64_LazyCCR_XInitFromPrologue2 verifies X preservation
// with a block that has no arithmetic ops (only logical), ensuring the
// prologue seeds [RSP+24] correctly.
func TestM68KJIT_AMD64_LazyCCR_XInitFromPrologue2(t *testing.T) {
	r := newM68KJITTestRig(t)
	// Set X=1 in SR
	r.cpu.SR |= M68K_SR_X
	r.cpu.DataRegs[0] = 0

	// MOVEQ #0,D1 — sets Z=1, N=0, V=0, C=0, X unchanged
	r.compileAndRun(t, 0x1000, 0x7200)

	// X should still be 1 (from incoming SR, preserved by MOVEQ which is logical)
	if r.cpu.SR&M68K_SR_X == 0 {
		t.Error("X flag should be preserved from incoming SR through MOVEQ")
	}
	if r.cpu.SR&M68K_SR_Z == 0 {
		t.Error("Z flag should be set for MOVEQ #0")
	}
}
