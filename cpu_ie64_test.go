package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// ===========================================================================
// Test Rig
// ===========================================================================

type ie64TestRig struct {
	bus *SystemBus
	cpu *CPU64
}

func newIE64TestRig() *ie64TestRig {
	bus := NewSystemBus()
	cpu := NewCPU64(bus)
	return &ie64TestRig{bus: bus, cpu: cpu}
}

// ie64Instr builds an 8-byte IE64 instruction.
func ie64Instr(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	instr[1] = (rd << 3) | (size << 1) | xbit
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

func (r *ie64TestRig) loadInstructions(instructions ...[]byte) {
	offset := uint32(PROG_START)
	for _, instr := range instructions {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	r.cpu.PC = PROG_START
}

func (r *ie64TestRig) executeOne(instr []byte) {
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	r.loadInstructions(instr, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
}

func (r *ie64TestRig) executeN(instructions ...[]byte) {
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	all := append(instructions, halt)
	r.loadInstructions(all...)
	r.cpu.running.Store(true)
	r.cpu.Execute()
}

// ===========================================================================
// Step 2a: Constructor and Basic State
// ===========================================================================

func TestIE64_NewCPU(t *testing.T) {
	bus := NewSystemBus()
	cpu := NewCPU64(bus)

	if cpu.PC != PROG_START {
		t.Fatalf("PC = 0x%X, want 0x%X", cpu.PC, PROG_START)
	}
	if cpu.regs[31] != STACK_START {
		t.Fatalf("R31 (SP) = 0x%X, want 0x%X", cpu.regs[31], STACK_START)
	}
	if cpu.regs[0] != 0 {
		t.Fatalf("R0 = %d, want 0", cpu.regs[0])
	}
	// All other registers should be zero
	for i := 1; i < 31; i++ {
		if cpu.regs[i] != 0 {
			t.Fatalf("R%d = %d, want 0", i, cpu.regs[i])
		}
	}
	if !cpu.running.Load() {
		t.Fatal("running should be true after NewCPU64")
	}
}

func TestIE64_R0_Hardwired(t *testing.T) {
	bus := NewSystemBus()
	cpu := NewCPU64(bus)

	cpu.setReg(0, 0x123)
	if cpu.regs[0] != 0 {
		t.Fatalf("R0 = 0x%X after setReg(0, 0x123), want 0", cpu.regs[0])
	}
	if cpu.getReg(0) != 0 {
		t.Fatalf("getReg(0) = 0x%X, want 0", cpu.getReg(0))
	}
}

func TestIE64_EmulatorCPU_Interface(t *testing.T) {
	var _ EmulatorCPU = (*CPU64)(nil)
}

// ===========================================================================
// Step 2b: Data Movement
// ===========================================================================

func TestIE64_MOVE_Register(t *testing.T) {
	r := newIE64TestRig()
	// Set R2 = 42
	r.cpu.regs[2] = 42
	// MOVE.Q R1, R2 — rd=1, size=Q(3), xbit=0, rs=2, rt=0
	instr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestIE64_MOVE_Immediate(t *testing.T) {
	r := newIE64TestRig()
	// MOVE.Q R1, #42 — rd=1, size=Q(3), xbit=1, rs=0, rt=0, imm32=42
	instr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42)
	r.executeOne(instr)
	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestIE64_MOVE_Sizes(t *testing.T) {
	r := newIE64TestRig()

	tests := []struct {
		name   string
		size   byte
		input  uint64
		expect uint64
	}{
		{"Byte", IE64_SIZE_B, 0x12345678DEADBEEF, 0xEF},
		{"Word", IE64_SIZE_W, 0x12345678DEADBEEF, 0xBEEF},
		{"Long", IE64_SIZE_L, 0x12345678DEADBEEF, 0xDEADBEEF},
		{"Quad", IE64_SIZE_Q, 0x12345678DEADBEEF, 0x12345678DEADBEEF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.cpu.regs[2] = tt.input
			// MOVE.size R1, R2
			instr := ie64Instr(OP_MOVE, 1, tt.size, 0, 2, 0, 0)
			r.executeOne(instr)
			if r.cpu.regs[1] != tt.expect {
				t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], tt.expect)
			}
		})
	}
}

func TestIE64_MOVT(t *testing.T) {
	r := newIE64TestRig()
	// Set R1 lower 32 bits first
	r.cpu.regs[1] = 0x0000000012345678
	// MOVT R1, #0xDEADBEEF — loads upper 32 bits
	instr := ie64Instr(OP_MOVT, 1, 0, 0, 0, 0, 0xDEADBEEF)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0xDEADBEEF12345678 {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF12345678", r.cpu.regs[1])
	}
}

func TestIE64_MOVEQ_SignExtend(t *testing.T) {
	r := newIE64TestRig()
	// MOVEQ R1, #0xFFFFFFFF (-1 as int32, sign-extends to 0xFFFFFFFFFFFFFFFF)
	instr := ie64Instr(OP_MOVEQ, 1, 0, 0, 0, 0, 0xFFFFFFFF)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("R1 = 0x%X, want 0xFFFFFFFFFFFFFFFF", r.cpu.regs[1])
	}
}

func TestIE64_64BitConstant(t *testing.T) {
	r := newIE64TestRig()
	// Build 0xDEADBEEF12345678 using MOVE.L + MOVT
	// MOVE.L R1, #0x12345678
	mov := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x12345678)
	// MOVT R1, #0xDEADBEEF
	movt := ie64Instr(OP_MOVT, 1, 0, 0, 0, 0, 0xDEADBEEF)
	r.executeN(mov, movt)
	if r.cpu.regs[1] != 0xDEADBEEF12345678 {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF12345678", r.cpu.regs[1])
	}
}

// ===========================================================================
// Step 2c: Load / Store
// ===========================================================================

func TestIE64_LOAD_STORE_Quad(t *testing.T) {
	r := newIE64TestRig()
	addr := uint32(0x5000)
	r.cpu.regs[2] = uint64(addr)
	r.cpu.regs[1] = 0xDEADBEEF12345678

	// STORE.Q R1, (R2) — store R1 at address in R2
	store := ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	// Clear R1
	clr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0)
	// LOAD.Q R1, (R2) — load from address in R2 into R1
	load := ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	r.executeN(store, clr, load)

	if r.cpu.regs[1] != 0xDEADBEEF12345678 {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF12345678", r.cpu.regs[1])
	}
}

func TestIE64_LOAD_STORE_Long(t *testing.T) {
	r := newIE64TestRig()
	addr := uint32(0x5000)
	r.cpu.regs[2] = uint64(addr)
	r.cpu.regs[1] = 0xCAFEBABE

	store := ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 2, 0, 0)
	clr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0)
	load := ie64Instr(OP_LOAD, 1, IE64_SIZE_L, 0, 2, 0, 0)
	r.executeN(store, clr, load)

	if r.cpu.regs[1] != 0xCAFEBABE {
		t.Fatalf("R1 = 0x%X, want 0xCAFEBABE", r.cpu.regs[1])
	}
}

func TestIE64_LOAD_STORE_Word(t *testing.T) {
	r := newIE64TestRig()
	addr := uint32(0x5000)
	r.cpu.regs[2] = uint64(addr)
	r.cpu.regs[1] = 0xBEEF

	store := ie64Instr(OP_STORE, 1, IE64_SIZE_W, 0, 2, 0, 0)
	clr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0)
	load := ie64Instr(OP_LOAD, 1, IE64_SIZE_W, 0, 2, 0, 0)
	r.executeN(store, clr, load)

	if r.cpu.regs[1] != 0xBEEF {
		t.Fatalf("R1 = 0x%X, want 0xBEEF", r.cpu.regs[1])
	}
}

func TestIE64_LOAD_STORE_Byte(t *testing.T) {
	r := newIE64TestRig()
	addr := uint32(0x5000)
	r.cpu.regs[2] = uint64(addr)
	r.cpu.regs[1] = 0xAB

	store := ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0)
	clr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0)
	load := ie64Instr(OP_LOAD, 1, IE64_SIZE_B, 0, 2, 0, 0)
	r.executeN(store, clr, load)

	if r.cpu.regs[1] != 0xAB {
		t.Fatalf("R1 = 0x%X, want 0xAB", r.cpu.regs[1])
	}
}

func TestIE64_LOAD_Displacement(t *testing.T) {
	r := newIE64TestRig()
	// Store a value at address 0x5010
	binary.LittleEndian.PutUint64(r.cpu.memory[0x5010:], 0x1122334455667788)
	// R2 = 0x5000
	r.cpu.regs[2] = 0x5000
	// LOAD.Q R1, 16(R2) — disp = 16
	instr := ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 16)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x1122334455667788 {
		t.Fatalf("R1 = 0x%X, want 0x1122334455667788", r.cpu.regs[1])
	}
}

func TestIE64_STORE_Displacement(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[1] = 0xAABBCCDDEEFF0011
	r.cpu.regs[2] = 0x5000
	// STORE.Q R1, 16(R2)
	instr := ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 16)
	r.executeOne(instr)
	got := binary.LittleEndian.Uint64(r.cpu.memory[0x5010:])
	if got != 0xAABBCCDDEEFF0011 {
		t.Fatalf("mem[0x5010] = 0x%X, want 0xAABBCCDDEEFF0011", got)
	}
}

func TestIE64_LEA(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0x5000
	// LEA R1, 0x100(R2)
	instr := ie64Instr(OP_LEA, 1, 0, 0, 2, 0, 0x100)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x5100 {
		t.Fatalf("R1 = 0x%X, want 0x5100", r.cpu.regs[1])
	}
}

func TestIE64_BusVisibility(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[1] = 0xCAFEBABE
	r.cpu.regs[2] = 0x5000
	// STORE.L R1, (R2)
	instr := ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 2, 0, 0)
	r.executeOne(instr)

	got := r.bus.Read32(0x5000)
	if got != 0xCAFEBABE {
		t.Fatalf("Bus read 0x%08X at 0x5000, want 0xCAFEBABE", got)
	}
}

// ===========================================================================
// Step 2d: Arithmetic
// ===========================================================================

func TestIE64_ADD_Register(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42
	// ADD.Q R1, R2, R3 — rd=1, rs=2, rt=3, xbit=0
	instr := ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 142 {
		t.Fatalf("R1 = %d, want 142", r.cpu.regs[1])
	}
}

func TestIE64_ADD_Immediate(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	// ADD.Q R1, R2, #42 — xbit=1, imm32=42
	instr := ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 2, 0, 42)
	r.executeOne(instr)
	if r.cpu.regs[1] != 142 {
		t.Fatalf("R1 = %d, want 142", r.cpu.regs[1])
	}
}

func TestIE64_SUB_Register(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42
	// SUB.Q R1, R2, R3
	instr := ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 58 {
		t.Fatalf("R1 = %d, want 58", r.cpu.regs[1])
	}
}

func TestIE64_SUB_Immediate(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	// SUB.Q R1, R2, #30
	instr := ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 2, 0, 30)
	r.executeOne(instr)
	if r.cpu.regs[1] != 70 {
		t.Fatalf("R1 = %d, want 70", r.cpu.regs[1])
	}
}

func TestIE64_MULU(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 6
	r.cpu.regs[3] = 7
	// MULU.Q R1, R2, R3
	instr := ie64Instr(OP_MULU, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestIE64_MULS(t *testing.T) {
	r := newIE64TestRig()
	var neg6 int64 = -6
	r.cpu.regs[2] = uint64(neg6)
	r.cpu.regs[3] = 7
	// MULS.Q R1, R2, R3
	instr := ie64Instr(OP_MULS, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if int64(r.cpu.regs[1]) != -42 {
		t.Fatalf("R1 = %d (signed), want -42", int64(r.cpu.regs[1]))
	}
}

func TestIE64_DIVU(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 7
	// DIVU.Q R1, R2, R3
	instr := ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 14 {
		t.Fatalf("R1 = %d, want 14", r.cpu.regs[1])
	}
}

func TestIE64_DIVS(t *testing.T) {
	r := newIE64TestRig()
	var neg100 int64 = -100
	r.cpu.regs[2] = uint64(neg100)
	r.cpu.regs[3] = 7
	// DIVS.Q R1, R2, R3
	instr := ie64Instr(OP_DIVS, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if int64(r.cpu.regs[1]) != -14 {
		t.Fatalf("R1 = %d (signed), want -14", int64(r.cpu.regs[1]))
	}
}

func TestIE64_DIVU_ByZero(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 0
	// DIVU.Q R1, R2, R3 — divisor is zero
	instr := ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	// Should not panic
	r.executeOne(instr)
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (div by zero)", r.cpu.regs[1])
	}
}

func TestIE64_MOD(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 7
	// MOD.Q R1, R2, R3
	instr := ie64Instr(OP_MOD64, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 2 {
		t.Fatalf("R1 = %d, want 2", r.cpu.regs[1])
	}
}

func TestIE64_NEG(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 42
	// NEG.Q R1, R2
	instr := ie64Instr(OP_NEG, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	r.executeOne(instr)
	if int64(r.cpu.regs[1]) != -42 {
		t.Fatalf("R1 = %d (signed), want -42", int64(r.cpu.regs[1]))
	}
}

func TestIE64_Arithmetic_Sizes(t *testing.T) {
	r := newIE64TestRig()

	// ADD with byte masking: 0x1FF + 0x01 = 0x200, masked to byte = 0x00
	r.cpu.regs[2] = 0x1FF
	r.cpu.regs[3] = 0x01
	instr := ie64Instr(OP_ADD, 1, IE64_SIZE_B, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x00 {
		t.Fatalf("ADD.B: R1 = 0x%X, want 0x00", r.cpu.regs[1])
	}

	// ADD with word masking: 0x1FFFF + 0x01 = 0x20000, masked to word = 0x0000
	r.cpu.regs[2] = 0x1FFFF
	r.cpu.regs[3] = 0x01
	instr = ie64Instr(OP_ADD, 1, IE64_SIZE_W, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x0000 {
		t.Fatalf("ADD.W: R1 = 0x%X, want 0x0000", r.cpu.regs[1])
	}

	// ADD with long masking: 0x1FFFFFFFF + 0x01 = 0x200000000, masked to long = 0x00000000
	r.cpu.regs[2] = 0x1FFFFFFFF
	r.cpu.regs[3] = 0x01
	instr = ie64Instr(OP_ADD, 1, IE64_SIZE_L, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x00000000 {
		t.Fatalf("ADD.L: R1 = 0x%X, want 0x00000000", r.cpu.regs[1])
	}
}

// ===========================================================================
// Step 2e: Logic and Shifts
// ===========================================================================

func TestIE64_AND(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0xFF00FF00FF00FF00
	r.cpu.regs[3] = 0x00FF00FF00FF00FF
	// AND.Q R1, R2, R3
	instr := ie64Instr(OP_AND64, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0", r.cpu.regs[1])
	}
}

func TestIE64_OR(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0xFF00FF00FF00FF00
	r.cpu.regs[3] = 0x00FF00FF00FF00FF
	// OR.Q R1, R2, R3
	instr := ie64Instr(OP_OR64, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("R1 = 0x%X, want 0xFFFFFFFFFFFFFFFF", r.cpu.regs[1])
	}
}

func TestIE64_EOR(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0xAAAAAAAAAAAAAAAA
	r.cpu.regs[3] = 0xFFFFFFFFFFFFFFFF
	// EOR.Q R1, R2, R3
	instr := ie64Instr(OP_EOR, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x5555555555555555 {
		t.Fatalf("R1 = 0x%X, want 0x5555555555555555", r.cpu.regs[1])
	}
}

func TestIE64_NOT(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0
	// NOT.Q R1, R2
	instr := ie64Instr(OP_NOT64, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("R1 = 0x%X, want 0xFFFFFFFFFFFFFFFF", r.cpu.regs[1])
	}
}

func TestIE64_LSL(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 1
	r.cpu.regs[3] = 4
	// LSL.Q R1, R2, R3
	instr := ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 16 {
		t.Fatalf("R1 = %d, want 16", r.cpu.regs[1])
	}
}

func TestIE64_LSR(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0x8000000000000000
	r.cpu.regs[3] = 4
	// LSR.Q R1, R2, R3
	instr := ie64Instr(OP_LSR, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0x0800000000000000 {
		t.Fatalf("R1 = 0x%X, want 0x0800000000000000", r.cpu.regs[1])
	}
}

func TestIE64_ASR(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0x8000000000000000 // negative in signed
	r.cpu.regs[3] = 4
	// ASR.Q R1, R2, R3
	instr := ie64Instr(OP_ASR, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0xF800000000000000 {
		t.Fatalf("R1 = 0x%X, want 0xF800000000000000", r.cpu.regs[1])
	}
}

func TestIE64_ASR_SignExtend(t *testing.T) {
	r := newIE64TestRig()

	// ASR.B on 0x80 (negative byte) >> 1 should sign-extend
	r.cpu.regs[2] = 0x80 // -128 as int8
	r.cpu.regs[3] = 1
	instr := ie64Instr(OP_ASR, 1, IE64_SIZE_B, 0, 2, 3, 0)
	r.executeOne(instr)
	// int8(-128) >> 1 = int8(-64) = 0xC0
	if r.cpu.regs[1] != 0xC0 {
		t.Fatalf("ASR.B: R1 = 0x%X, want 0xC0", r.cpu.regs[1])
	}

	// ASR.W on 0x8000 (negative word) >> 1
	r.cpu.regs[2] = 0x8000
	r.cpu.regs[3] = 1
	instr = ie64Instr(OP_ASR, 1, IE64_SIZE_W, 0, 2, 3, 0)
	r.executeOne(instr)
	// int16(-32768) >> 1 = int16(-16384) = 0xC000
	if r.cpu.regs[1] != 0xC000 {
		t.Fatalf("ASR.W: R1 = 0x%X, want 0xC000", r.cpu.regs[1])
	}

	// ASR.L on 0x80000000 (negative long) >> 1
	r.cpu.regs[2] = 0x80000000
	r.cpu.regs[3] = 1
	instr = ie64Instr(OP_ASR, 1, IE64_SIZE_L, 0, 2, 3, 0)
	r.executeOne(instr)
	// int32(-2147483648) >> 1 = int32(-1073741824) = 0xC0000000
	if r.cpu.regs[1] != 0xC0000000 {
		t.Fatalf("ASR.L: R1 = 0x%X, want 0xC0000000", r.cpu.regs[1])
	}
}

func TestIE64_Shift_Immediate(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 1
	// LSL.Q R1, R2, #8 (xbit=1, imm32=8)
	instr := ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 1, 2, 0, 8)
	r.executeOne(instr)
	if r.cpu.regs[1] != 256 {
		t.Fatalf("R1 = %d, want 256", r.cpu.regs[1])
	}
}

func TestIE64_Shift_Register(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 256
	r.cpu.regs[3] = 4
	// LSR.Q R1, R2, R3
	instr := ie64Instr(OP_LSR, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 16 {
		t.Fatalf("R1 = %d, want 16", r.cpu.regs[1])
	}
}

// ===========================================================================
// Step 2f: Branches
// ===========================================================================

func TestIE64_BRA(t *testing.T) {
	r := newIE64TestRig()
	// BRA +16 (forward jump, skipping one instruction)
	bra := ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16)
	// This instruction should be skipped
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	// Landing point: HALT (at PROG_START + 16)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bra, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	// R1 should still be 0 since the MOVE was skipped
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (branch should have skipped MOVE)", r.cpu.regs[1])
	}
}

func TestIE64_BEQ_Taken(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 42
	// BEQ R2, R3, +16 — should branch (equal)
	beq := ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(beq, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BEQ should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BEQ_NotTaken(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 99
	// BEQ R2, R3, +16 — should NOT branch (not equal)
	beq := ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 16)
	// This should execute (branch not taken)
	mov := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xBEEF)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(beq, mov, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0xBEEF {
		t.Fatalf("R1 = 0x%X, want 0xBEEF (BEQ should not have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BNE_Taken(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 99
	// BNE R2, R3, +16 — should branch (not equal)
	bne := ie64Instr(OP_BNE, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bne, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BNE should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BNE_NotTaken(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 42
	// BNE R2, R3, +16 — should NOT branch (equal)
	bne := ie64Instr(OP_BNE, 0, 0, 0, 2, 3, 16)
	mov := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xBEEF)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bne, mov, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0xBEEF {
		t.Fatalf("R1 = 0x%X, want 0xBEEF (BNE should not have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BLT(t *testing.T) {
	r := newIE64TestRig()
	var neg5 int64 = -5
	r.cpu.regs[2] = uint64(neg5) // -5 (signed)
	r.cpu.regs[3] = 10
	// BLT R2, R3, +16 — -5 < 10, should branch
	blt := ie64Instr(OP_BLT, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(blt, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BLT should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BGE(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 10
	r.cpu.regs[3] = 10
	// BGE R2, R3, +16 — 10 >= 10, should branch
	bge := ie64Instr(OP_BGE, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bge, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BGE should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BGT(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 11
	r.cpu.regs[3] = 10
	// BGT R2, R3, +16 — 11 > 10, should branch
	bgt := ie64Instr(OP_BGT, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bgt, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BGT should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BLE(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 10
	r.cpu.regs[3] = 10
	// BLE R2, R3, +16 — 10 <= 10, should branch
	ble := ie64Instr(OP_BLE, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(ble, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BLE should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BHI(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 0xFFFFFFFFFFFFFFFF // large unsigned
	r.cpu.regs[3] = 10
	// BHI R2, R3, +16 — unsigned: 0xFFFF... > 10, should branch
	bhi := ie64Instr(OP_BHI, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bhi, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BHI should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_BLS(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 5
	r.cpu.regs[3] = 10
	// BLS R2, R3, +16 — unsigned: 5 <= 10, should branch
	bls := ie64Instr(OP_BLS, 0, 0, 0, 2, 3, 16)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(bls, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (BLS should have branched)", r.cpu.regs[1])
	}
}

func TestIE64_Branch_ZeroCompare(t *testing.T) {
	r := newIE64TestRig()
	// R0 is always zero. Compare R1 (=0) with R0 (=0) using BEQ.
	r.cpu.regs[1] = 0
	// BEQ R1, R0, +16 — both zero, should branch
	beq := ie64Instr(OP_BEQ, 0, 0, 0, 1, 0, 16)
	skipped := ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(beq, skipped, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[5] != 0 {
		t.Fatalf("R5 = 0x%X, want 0 (BEQ with zero should have branched)", r.cpu.regs[5])
	}
}

func TestIE64_Branch_Backward(t *testing.T) {
	r := newIE64TestRig()
	// Simple loop: decrement R1 until zero
	// R1 = 3 (loop counter)
	r.cpu.regs[1] = 3
	// R2 = 1 (decrement value)
	r.cpu.regs[2] = 1

	// Instruction layout at PROG_START:
	// +0:  SUB.Q R1, R1, R2      (R1 = R1 - 1)
	// +8:  BNE R1, R0, -8        (if R1 != 0, branch back to +0: offset = 8 + (-8) = 0? No.)
	//      BNE is at PC=PROG_START+8, offset=-8 makes PC=PROG_START+8+(-8)=PROG_START
	// +16: HALT

	sub := ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 1, 2, 0)
	var backOffset int32 = -8
	bne := ie64Instr(OP_BNE, 0, 0, 0, 1, 0, uint32(backOffset)) // backward branch
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(sub, bne, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (loop should have decremented to zero)", r.cpu.regs[1])
	}
}

// ===========================================================================
// Step 2g: Subroutines and Stack
// ===========================================================================

func TestIE64_JSR_RTS(t *testing.T) {
	r := newIE64TestRig()
	spBefore := r.cpu.regs[31]

	// Layout:
	// +0:  JSR +16 (jump to subroutine at +16)
	// +8:  HALT     (return here after RTS)
	// +16: MOVE.Q R1, #42
	// +24: RTS

	jsr := ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	mov := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(jsr, halt, mov, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
	// SP should be restored after RTS
	if r.cpu.regs[31] != spBefore {
		t.Fatalf("SP = 0x%X, want 0x%X (SP should be restored)", r.cpu.regs[31], spBefore)
	}
	// PC should be at the HALT instruction (PROG_START + 8)
	// (Execute stopped at HALT, so PC is wherever HALT was)
}

func TestIE64_PUSH_POP(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[5] = 0x1234567890ABCDEF
	spBefore := r.cpu.regs[31]

	// PUSH R5 (rs=5)
	push := ie64Instr(OP_PUSH64, 0, 0, 0, 5, 0, 0)
	// Clear R5
	clr := ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 1, 0, 0, 0)
	// POP R6 (rd=6)
	pop := ie64Instr(OP_POP64, 6, 0, 0, 0, 0, 0)

	r.executeN(push, clr, pop)

	if r.cpu.regs[6] != 0x1234567890ABCDEF {
		t.Fatalf("R6 = 0x%X, want 0x1234567890ABCDEF", r.cpu.regs[6])
	}
	// SP should be restored
	if r.cpu.regs[31] != spBefore {
		t.Fatalf("SP = 0x%X, want 0x%X", r.cpu.regs[31], spBefore)
	}
}

func TestIE64_Nested_JSR(t *testing.T) {
	r := newIE64TestRig()
	spBefore := r.cpu.regs[31]

	// Layout:
	// +0:  JSR +16       -> jumps to sub1 at +16
	// +8:  HALT          (final return)
	// +16: MOVE R1, #1   (sub1 body)
	// +24: JSR +16       -> jumps to sub2 at +40
	// +32: RTS           (return from sub1)
	// +40: MOVE R2, #2   (sub2 body)
	// +48: RTS           (return from sub2)

	jsr1 := ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16)         // +0
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)         // +8
	mov1 := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1) // +16
	jsr2 := ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16)         // +24
	rts1 := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)          // +32
	mov2 := ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 2) // +40
	rts2 := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)          // +48

	r.loadInstructions(jsr1, halt, mov1, jsr2, rts1, mov2, rts2)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 1 {
		t.Fatalf("R1 = %d, want 1", r.cpu.regs[1])
	}
	if r.cpu.regs[2] != 2 {
		t.Fatalf("R2 = %d, want 2", r.cpu.regs[2])
	}
	if r.cpu.regs[31] != spBefore {
		t.Fatalf("SP = 0x%X, want 0x%X (stack not balanced)", r.cpu.regs[31], spBefore)
	}
}

func TestIE64_PUSH_POP_SP(t *testing.T) {
	r := newIE64TestRig()
	spBefore := r.cpu.regs[31]

	// Push two values, pop in reverse order
	r.cpu.regs[1] = 0xAAAA
	r.cpu.regs[2] = 0xBBBB

	push1 := ie64Instr(OP_PUSH64, 0, 0, 0, 1, 0, 0) // push R1
	push2 := ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0) // push R2
	pop3 := ie64Instr(OP_POP64, 3, 0, 0, 0, 0, 0)   // pop R3 (should get R2's value)
	pop4 := ie64Instr(OP_POP64, 4, 0, 0, 0, 0, 0)   // pop R4 (should get R1's value)

	r.executeN(push1, push2, pop3, pop4)

	if r.cpu.regs[3] != 0xBBBB {
		t.Fatalf("R3 = 0x%X, want 0xBBBB (LIFO order)", r.cpu.regs[3])
	}
	if r.cpu.regs[4] != 0xAAAA {
		t.Fatalf("R4 = 0x%X, want 0xAAAA (LIFO order)", r.cpu.regs[4])
	}
	if r.cpu.regs[31] != spBefore {
		t.Fatalf("SP = 0x%X, want 0x%X", r.cpu.regs[31], spBefore)
	}
}

// ===========================================================================
// Step 2h: System Instructions
// ===========================================================================

func TestIE64_NOP(t *testing.T) {
	r := newIE64TestRig()
	pcBefore := uint64(PROG_START)
	// Execute a NOP, then HALT
	nop := ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0)
	r.executeOne(nop)
	// PC should advance by one instruction past NOP, then stop at HALT
	// After HALT, PC is at PROG_START + 8 (NOP) and HALT stops execution.
	// Just verify it did not crash and ran through.
	if r.cpu.PC < pcBefore+IE64_INSTR_SIZE {
		t.Fatalf("PC = 0x%X, should have advanced past NOP", r.cpu.PC)
	}
}

func TestIE64_HALT(t *testing.T) {
	r := newIE64TestRig()
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	r.loadInstructions(halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	if r.cpu.running.Load() {
		t.Fatal("running should be false after HALT")
	}
}

func TestIE64_SEI_CLI(t *testing.T) {
	r := newIE64TestRig()

	// Initially interrupts should be disabled
	if r.cpu.interruptEnabled.Load() {
		t.Fatal("interruptEnabled should be false initially")
	}

	// SEI enables interrupts
	sei := ie64Instr(OP_SEI64, 0, 0, 0, 0, 0, 0)
	r.executeOne(sei)
	if !r.cpu.interruptEnabled.Load() {
		t.Fatal("interruptEnabled should be true after SEI")
	}

	// CLI disables interrupts
	r.cpu.running.Store(true)
	cli := ie64Instr(OP_CLI64, 0, 0, 0, 0, 0, 0)
	r.executeOne(cli)
	if r.cpu.interruptEnabled.Load() {
		t.Fatal("interruptEnabled should be false after CLI")
	}
}

func TestIE64_Timer_Interrupt(t *testing.T) {
	r := newIE64TestRig()

	// Verify timer atomic fields work correctly
	r.cpu.timerPeriod.Store(100)
	if r.cpu.timerPeriod.Load() != 100 {
		t.Fatal("timerPeriod should be 100")
	}

	r.cpu.timerCount.Store(50)
	if r.cpu.timerCount.Load() != 50 {
		t.Fatal("timerCount should be 50")
	}

	r.cpu.timerEnabled.Store(true)
	if !r.cpu.timerEnabled.Load() {
		t.Fatal("timerEnabled should be true")
	}

	r.cpu.timerState.Store(TIMER_RUNNING)
	if r.cpu.timerState.Load() != TIMER_RUNNING {
		t.Fatal("timerState should be TIMER_RUNNING")
	}

	// Verify timer can be stopped
	r.cpu.timerEnabled.Store(false)
	if r.cpu.timerEnabled.Load() {
		t.Fatal("timerEnabled should be false after disable")
	}
}

func TestIE64_RTI(t *testing.T) {
	r := newIE64TestRig()

	// Simulate being in an interrupt handler:
	// Push a return address onto the stack, set inInterrupt, then execute RTI
	returnAddr := uint64(PROG_START + 0x100)
	r.cpu.regs[31] -= 8
	sp := uint32(r.cpu.regs[31])
	binary.LittleEndian.PutUint64(r.cpu.memory[sp:], returnAddr)
	r.cpu.inInterrupt.Store(true)

	// RTI should pop return address and clear inInterrupt
	rti := ie64Instr(OP_RTI64, 0, 0, 0, 0, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	// Place RTI at PROG_START, and HALT at the return address
	r.loadInstructions(rti)
	// Place HALT at the return address
	copy(r.cpu.memory[returnAddr:], halt)

	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.inInterrupt.Load() {
		t.Fatal("inInterrupt should be false after RTI")
	}
}

// ===========================================================================
// Step 2i: LoadProgram
// ===========================================================================

func TestIE64_LoadProgram(t *testing.T) {
	r := newIE64TestRig()

	// Create a minimal program
	program := make([]byte, 16)
	program[0] = OP_HALT64 // HALT as first instruction
	binary.LittleEndian.PutUint32(program[8:], 0xCAFEBABE)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.ie64")
	if err := os.WriteFile(tmpFile, program, 0644); err != nil {
		t.Fatalf("failed to write test program: %v", err)
	}

	if err := r.cpu.LoadProgram(tmpFile); err != nil {
		t.Fatalf("LoadProgram failed: %v", err)
	}

	if r.cpu.PC != PROG_START {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, PROG_START)
	}

	// Verify data at PROG_START + 8
	got := binary.LittleEndian.Uint32(r.cpu.memory[PROG_START+8:])
	if got != 0xCAFEBABE {
		t.Fatalf("mem[PROG_START+8] = 0x%08X, want 0xCAFEBABE", got)
	}

	// Verify bus can see it too
	busGot := r.bus.Read32(uint32(PROG_START + 8))
	if busGot != 0xCAFEBABE {
		t.Fatalf("bus.Read32(PROG_START+8) = 0x%08X, want 0xCAFEBABE", busGot)
	}
}

// ===========================================================================
// Step 2j: Reset
// ===========================================================================

func TestIE64_Reset(t *testing.T) {
	r := newIE64TestRig()

	// Dirty state
	r.cpu.regs[1] = 0xDEADBEEF
	r.cpu.regs[5] = 0x12345678
	r.cpu.PC = 0x9000
	r.cpu.interruptEnabled.Store(true)
	r.cpu.inInterrupt.Store(true)
	r.cpu.running.Store(false)

	r.cpu.Reset()

	if r.cpu.PC != PROG_START {
		t.Fatalf("PC = 0x%X, want 0x%X after reset", r.cpu.PC, PROG_START)
	}
	if r.cpu.regs[31] != STACK_START {
		t.Fatalf("R31 = 0x%X, want 0x%X after reset", r.cpu.regs[31], STACK_START)
	}
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 after reset", r.cpu.regs[1])
	}
	if r.cpu.regs[5] != 0 {
		t.Fatalf("R5 = 0x%X, want 0 after reset", r.cpu.regs[5])
	}
	if r.cpu.interruptEnabled.Load() {
		t.Fatal("interruptEnabled should be false after reset")
	}
	if r.cpu.inInterrupt.Load() {
		t.Fatal("inInterrupt should be false after reset")
	}
	if !r.cpu.running.Load() {
		t.Fatal("running should be true after reset")
	}
}

// ===========================================================================
// Step 2k: Edge Cases
// ===========================================================================

func TestIE64_MOVE_R0_NoEffect(t *testing.T) {
	r := newIE64TestRig()
	// Try to MOVE #42 into R0 — should be silently discarded
	instr := ie64Instr(OP_MOVE, 0, IE64_SIZE_Q, 1, 0, 0, 42)
	r.executeOne(instr)
	if r.cpu.regs[0] != 0 {
		t.Fatalf("R0 = %d, want 0 (writes to R0 must be discarded)", r.cpu.regs[0])
	}
}

func TestIE64_DIVS_ByZero(t *testing.T) {
	r := newIE64TestRig()
	var neg100s int64 = -100
	r.cpu.regs[2] = uint64(neg100s)
	r.cpu.regs[3] = 0
	instr := ie64Instr(OP_DIVS, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (signed div by zero)", r.cpu.regs[1])
	}
}

func TestIE64_MOD_ByZero(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 0
	instr := ie64Instr(OP_MOD64, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (mod by zero)", r.cpu.regs[1])
	}
}

func TestIE64_InvalidOpcode(t *testing.T) {
	r := newIE64TestRig()
	// Use an undefined opcode (0xFE)
	instr := make([]byte, 8)
	instr[0] = 0xFE
	r.loadInstructions(instr)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	// Should halt on invalid opcode
	if r.cpu.running.Load() {
		t.Fatal("running should be false after invalid opcode")
	}
}
