package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ===========================================================================
// Test Rig
// ===========================================================================

type ie64TestRig struct {
	bus *MachineBus
	cpu *CPU64
}

func newIE64TestRig() *ie64TestRig {
	bus := NewMachineBus()
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
	bus := NewMachineBus()
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
	bus := NewMachineBus()
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
	// MOVE.Q R1, R2 - rd=1, size=Q(3), xbit=0, rs=2, rt=0
	instr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestIE64_MOVE_Immediate(t *testing.T) {
	r := newIE64TestRig()
	// MOVE.Q R1, #42 - rd=1, size=Q(3), xbit=1, rs=0, rt=0, imm32=42
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
	// MOVT R1, #0xDEADBEEF - loads upper 32 bits
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

	// STORE.Q R1, (R2) - store R1 at address in R2
	store := ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0)
	// Clear R1
	clr := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0)
	// LOAD.Q R1, (R2) - load from address in R2 into R1
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
	// LOAD.Q R1, 16(R2) - disp = 16
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
	// ADD.Q R1, R2, R3 - rd=1, rs=2, rt=3, xbit=0
	instr := ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 2, 3, 0)
	r.executeOne(instr)
	if r.cpu.regs[1] != 142 {
		t.Fatalf("R1 = %d, want 142", r.cpu.regs[1])
	}
}

func TestIE64_ADD_Immediate(t *testing.T) {
	r := newIE64TestRig()
	r.cpu.regs[2] = 100
	// ADD.Q R1, R2, #42 - xbit=1, imm32=42
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
	// DIVU.Q R1, R2, R3 - divisor is zero
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
	// BEQ R2, R3, +16 - should branch (equal)
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
	// BEQ R2, R3, +16 - should NOT branch (not equal)
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
	// BNE R2, R3, +16 - should branch (not equal)
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
	// BNE R2, R3, +16 - should NOT branch (equal)
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
	// BLT R2, R3, +16 - -5 < 10, should branch
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
	// BGE R2, R3, +16 - 10 >= 10, should branch
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
	// BGT R2, R3, +16 - 11 > 10, should branch
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
	// BLE R2, R3, +16 - 10 <= 10, should branch
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
	// BHI R2, R3, +16 - unsigned: 0xFFFF... > 10, should branch
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
	// BLS R2, R3, +16 - unsigned: 5 <= 10, should branch
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
	// BEQ R1, R0, +16 - both zero, should branch
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
	// Try to MOVE #42 into R0 - should be silently discarded
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

// ===========================================================================
// Regression tests for fast-path refactor
// ===========================================================================

func TestIE64_HALT_Immediate(t *testing.T) {
	// Verify HALT stops immediately - no instructions execute after it
	r := newIE64TestRig()
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	// Place a MOVE after HALT that would change R1
	mov := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	r.loadInstructions(halt, mov)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (HALT should stop immediately)", r.cpu.regs[1])
	}
	if r.cpu.running.Load() {
		t.Fatal("running should be false after HALT")
	}
}

func TestIE64_InvalidOpcode_Immediate(t *testing.T) {
	// Verify invalid opcode stops immediately - no instructions execute after it
	r := newIE64TestRig()
	invalid := make([]byte, 8)
	invalid[0] = 0xFE
	mov := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xDEAD)
	r.loadInstructions(invalid, mov)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (invalid opcode should stop immediately)", r.cpu.regs[1])
	}
	if r.cpu.running.Load() {
		t.Fatal("running should be false after invalid opcode")
	}
}

func TestIE64_StackOverflow_Halt(t *testing.T) {
	// Push with SP near 0 - should halt cleanly without panic
	r := newIE64TestRig()
	r.cpu.regs[31] = 4 // SP too low for an 8-byte push
	r.cpu.regs[5] = 0xCAFE
	push := ie64Instr(OP_PUSH64, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	r.loadInstructions(push, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	// SP wraps to a huge value - bounds check should halt
	if r.cpu.running.Load() {
		t.Fatal("running should be false after stack overflow")
	}
}

func TestIE64_StackUnderflow_Halt(t *testing.T) {
	// RTS with SP beyond memory bounds - should halt cleanly
	r := newIE64TestRig()
	r.cpu.regs[31] = uint64(len(r.cpu.memory)) // SP at end of memory
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	r.loadInstructions(rts, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	if r.cpu.running.Load() {
		t.Fatal("running should be false after stack underflow")
	}
}

// ===========================================================================
// Benchmarks
// ===========================================================================

func BenchmarkIE64_TightLoop(b *testing.B) {
	// Tight decrement-and-branch loop: measures raw instruction throughput
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// R1 = loop count (set per iteration)
	// R2 = 1 (decrement value)
	// +0:  SUB.Q R1, R1, R2
	// +8:  BNE R1, R0, -8
	// +16: HALT
	sub := ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 1, 2, 0)
	var backOffset int32 = -8
	bne := ie64Instr(OP_BNE, 0, 0, 0, 1, 0, uint32(backOffset))
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	offset := uint32(PROG_START)
	copy(cpu.memory[offset:], sub)
	offset += 8
	copy(cpu.memory[offset:], bne)
	offset += 8
	copy(cpu.memory[offset:], halt)

	const loopIterations = 100000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[1] = loopIterations
		cpu.regs[2] = 1
		cpu.running.Store(true)
		cpu.Execute()
	}
	// 2 instructions per loop iteration + 1 HALT
	b.ReportMetric(float64(loopIterations*2+1), "instructions/op")
}

func BenchmarkIE64_MemoryIntensive(b *testing.B) {
	// Load/store loop: measures memory access throughput
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// R1 = loop counter
	// R2 = 1 (decrement)
	// R3 = base address for load/store
	// R4 = scratch register
	//
	// +0:  STORE.Q R1, (R3)      - store counter to memory
	// +8:  LOAD.Q R4, (R3)       - load it back
	// +16: SUB.Q R1, R1, R2      - decrement counter
	// +24: BNE R1, R0, -24       - loop back to +0
	// +32: HALT
	store := ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 3, 0, 0)
	load := ie64Instr(OP_LOAD, 4, IE64_SIZE_Q, 0, 3, 0, 0)
	sub := ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 1, 2, 0)
	var backOffset int32 = -24
	bne := ie64Instr(OP_BNE, 0, 0, 0, 1, 0, uint32(backOffset))
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	offset := uint32(PROG_START)
	for _, instr := range [][]byte{store, load, sub, bne, halt} {
		copy(cpu.memory[offset:], instr)
		offset += 8
	}

	const loopIterations = 100000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[1] = loopIterations
		cpu.regs[2] = 1
		cpu.regs[3] = 0x5000 // data address
		cpu.running.Store(true)
		cpu.Execute()
	}
	// 4 instructions per loop iteration + 1 HALT
	b.ReportMetric(float64(loopIterations*4+1), "instructions/op")
}

func BenchmarkIE64FPU_FADD_ViaExecute(b *testing.B) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.FPU.setFReg(1, 1.5)
	cpu.FPU.setFReg(2, 2.5)

	// FADD F0, F1, F2; HALT
	fadd := ie64Instr(OP_FADD, 0, 0, 0, 1, 2, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	// We need a loop without re-loading memory every time if possible to measure dispatch overhead
	// But Execute() runs until HALT.
	// So we'll put FADD in a loop:
	// +0: FADD F0, F1, F2
	// +8: SUB.Q R1, R1, R2
	// +16: BNE R1, R0, -16
	// +24: HALT

	sub := ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 1, 2, 0)
	bne := ie64Instr(OP_BNE, 0, 0, 0, 1, 0, uint32(0xFFFFFFF0)) // -16 = 0xFFFFFFF0

	offset := uint32(PROG_START)
	for _, instr := range [][]byte{fadd, sub, bne, halt} {
		copy(cpu.memory[offset:], instr)
		offset += 8
	}

	const loopIterations = 1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[1] = loopIterations
		cpu.regs[2] = 1
		cpu.running.Store(true)
		cpu.Execute()
	}
	// 3 instructions per loop iteration + 1 HALT
	b.ReportMetric(float64(loopIterations*3+1), "instructions/op")
}

// ===========================================================================
// JMP (register-indirect jump)
// ===========================================================================

func TestIE64_JMP_Register(t *testing.T) {
	// Load target address into R5, JMP (R5), verify target executes
	// Layout:
	// +0:  MOVE.L R5, #targetAddr  (load target into R5)
	// +8:  JMP (R5)                (jump to target)
	// +16: MOVE.Q R1, #99          (SHOULD BE SKIPPED)
	// +24: HALT
	// +32: MOVE.Q R2, #42          (target - should execute)
	// +40: HALT
	r := newIE64TestRig()
	targetAddr := uint32(PROG_START + 32)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, targetAddr)
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 5, 0, 0)
	skipped := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 99)
	halt1 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	target := ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 42)
	halt2 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jmp, skipped, halt1, target, halt2)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (skipped instruction should not execute)", r.cpu.regs[1])
	}
	if r.cpu.regs[2] != 42 {
		t.Fatalf("R2 = %d, want 42", r.cpu.regs[2])
	}
}

func TestIE64_JMP_NoStackEffect(t *testing.T) {
	r := newIE64TestRig()
	spBefore := r.cpu.regs[31]

	targetAddr := uint32(PROG_START + 16)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, targetAddr)
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jmp, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[31] != spBefore {
		t.Fatalf("SP = 0x%X, want 0x%X (JMP should not change SP)", r.cpu.regs[31], spBefore)
	}
}

func TestIE64_JMP_Displacement(t *testing.T) {
	// JMP 8(R5) should jump to R5 + 8
	r := newIE64TestRig()
	baseAddr := uint32(PROG_START + 16)
	// target is at baseAddr + 8 = PROG_START + 24
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, baseAddr)
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 5, 0, 8)
	halt1 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0) // +16 (base, skipped)
	target := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 77)
	halt2 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jmp, halt1, target, halt2)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 77 {
		t.Fatalf("R1 = %d, want 77", r.cpu.regs[1])
	}
}

func TestIE64_JMP_NegativeDisplacement(t *testing.T) {
	// Layout:
	// +0:  MOVE.Q R2, #55   (target)
	// +8:  HALT
	// +16: MOVE.L R5, #(PROG_START+16) (point R5 to +16)
	// +24: JMP -16(R5)       (jump to +0)
	// +32: HALT
	r := newIE64TestRig()
	target := ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 55)
	halt1 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(PROG_START+16))
	negDisp := uint32(0xFFFFFFF0) // int32(-16)
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 5, 0, negDisp)
	halt2 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	// We need to start execution at +16 to set up R5, then JMP back to +0
	r.loadInstructions(target, halt1, loadR5, jmp, halt2)
	r.cpu.PC = PROG_START + 16
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[2] != 55 {
		t.Fatalf("R2 = %d, want 55", r.cpu.regs[2])
	}
}

func TestIE64_JMP_AddrMask(t *testing.T) {
	// Address above 32MB should be masked to 25-bit range
	r := newIE64TestRig()
	// Set R5 directly to an address with bit 25 set; when masked it gives PROG_START+24
	targetAddr := uint64(0x2000000 + PROG_START + 24) // bit 25 set
	r.cpu.regs[5] = targetAddr
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 5, 0, 0)
	halt1 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	halt2 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	target := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 88) // at PROG_START+24
	halt3 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(jmp, halt1, halt2, target, halt3)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 88 {
		t.Fatalf("R1 = %d, want 88 (address mask should wrap)", r.cpu.regs[1])
	}
}

func TestIE64_JMP_R0(t *testing.T) {
	// JMP 8(R0) - R0 is always 0, so displacement is the absolute target
	r := newIE64TestRig()
	targetAddr := uint32(PROG_START + 16)
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 0, 0, targetAddr)
	halt1 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	target := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 33)
	halt2 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(jmp, halt1, target, halt2)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 33 {
		t.Fatalf("R1 = %d, want 33", r.cpu.regs[1])
	}
}

// ===========================================================================
// JSR Indirect (register-indirect subroutine call)
// ===========================================================================

func TestIE64_JSR_Indirect_Basic(t *testing.T) {
	// Layout:
	// +0:  MOVE.L R5, #(PROG_START+24)  (load subroutine address)
	// +8:  JSR (R5)                       (call subroutine)
	// +16: HALT                           (return here after RTS)
	// +24: MOVE.Q R1, #42                (subroutine body)
	// +32: RTS
	r := newIE64TestRig()
	subAddr := uint32(PROG_START + 24)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, subAddr)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsrInd, halt, body, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestIE64_JSR_Indirect_ReturnAddress(t *testing.T) {
	// Return address on stack should be PC + 8 (address of next instruction after JSR)
	r := newIE64TestRig()
	subAddr := uint32(PROG_START + 24)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, subAddr)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	// +16: this is the return address (PROG_START+16)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	// Subroutine: read return address from stack into R6 for verification
	loadRA := ie64Instr(OP_LOAD, 6, IE64_SIZE_Q, 0, 31, 0, 0) // load.q r6, (sp)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsrInd, halt, loadRA, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	expectedRA := uint64(PROG_START + 16) // PC of instruction after JSR
	if r.cpu.regs[6] != expectedRA {
		t.Fatalf("return address = 0x%X, want 0x%X", r.cpu.regs[6], expectedRA)
	}
}

func TestIE64_JSR_Indirect_SP(t *testing.T) {
	// SP decremented by 8 during call, restored after RTS
	r := newIE64TestRig()
	spBefore := r.cpu.regs[31]
	subAddr := uint32(PROG_START + 24)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, subAddr)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsrInd, halt, body, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[31] != spBefore {
		t.Fatalf("SP = 0x%X, want 0x%X (SP should be restored after RTS)", r.cpu.regs[31], spBefore)
	}
}

func TestIE64_JSR_Indirect_Displacement(t *testing.T) {
	// JSR 8(R5) calls subroutine at R5+8
	r := newIE64TestRig()
	baseAddr := uint32(PROG_START + 16)
	// subroutine is at baseAddr + 8 = PROG_START + 24
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, baseAddr)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 8)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 77)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	// Layout: loadR5(+0), jsrInd(+8), halt(+16), body(+24), rts(+32)
	r.loadInstructions(loadR5, jsrInd, halt, body, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 77 {
		t.Fatalf("R1 = %d, want 77", r.cpu.regs[1])
	}
}

func TestIE64_JSR_Indirect_NegativeDisplacement(t *testing.T) {
	// Layout:
	// +0:  MOVE.Q R1, #55    (subroutine)
	// +8:  RTS
	// +16: MOVE.L R5, #(PROG_START+16)
	// +24: JSR -16(R5)        (calls PROG_START+0)
	// +32: HALT
	r := newIE64TestRig()
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 55)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(PROG_START+16))
	negDisp := uint32(0xFFFFFFF0) // int32(-16)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, negDisp)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(body, rts, loadR5, jsrInd, halt)
	r.cpu.PC = PROG_START + 16
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 55 {
		t.Fatalf("R1 = %d, want 55", r.cpu.regs[1])
	}
}

func TestIE64_JSR_Indirect_Nested(t *testing.T) {
	// Nested indirect calls both return correctly
	// Layout:
	// +0:  MOVE.L R5, #(PROG_START+24)   (addr of sub1)
	// +8:  JSR (R5)                        (call sub1)
	// +16: HALT                            (return here after sub1)
	// +24: MOVE.L R6, #(PROG_START+48)   (sub1: load addr of sub2)
	// +32: JSR (R6)                        (sub1: call sub2)
	// +40: RTS                             (sub1: return)
	// +48: MOVE.Q R1, #42                 (sub2: body)
	// +56: RTS                             (sub2: return)
	r := newIE64TestRig()
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(PROG_START+24))
	jsr1 := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	loadR6 := ie64Instr(OP_MOVE, 6, IE64_SIZE_L, 1, 0, 0, uint32(PROG_START+48))
	jsr2 := ie64Instr(OP_JSR_IND, 0, 0, 0, 6, 0, 0)
	rts1 := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42)
	rts2 := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsr1, halt, loadR6, jsr2, rts1, body, rts2)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestIE64_JSR_Indirect_AddrMask(t *testing.T) {
	// Target address should be masked to 32MB
	r := newIE64TestRig()
	bigAddr := uint64(0x2000000 + PROG_START + 24) // bit 25 set
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(bigAddr))
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 88)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsrInd, halt, body, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 88 {
		t.Fatalf("R1 = %d, want 88", r.cpu.regs[1])
	}
}

func TestIE64_JSR_Indirect_StackOverflow(t *testing.T) {
	// CPU should halt when SP is too low for the push
	r := newIE64TestRig()
	r.cpu.regs[31] = 4 // SP too low for 8-byte push
	subAddr := uint32(PROG_START + 16)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, subAddr)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsrInd, halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.running.Load() {
		t.Fatal("CPU should have halted due to stack overflow")
	}
}

// ===========================================================================
// Integration: Jump Table
// ===========================================================================

func TestIE64_JumpTable(t *testing.T) {
	// Build a jump table in memory, index into it, and JMP
	r := newIE64TestRig()

	// Layout:
	// +0:  MOVE.L R3, #tableAddr   (load table base)
	// +8:  LOAD.Q R5, 8(R3)        (load second entry - offset 8)
	// +16: JMP (R5)                 (jump to entry)
	// +24: MOVE.Q R1, #11          (entry 0 - not taken)
	// +32: HALT
	// +40: MOVE.Q R1, #22          (entry 1 - taken)
	// +48: HALT
	// +56: table: dc.q entry0, entry1
	tableAddr := uint32(PROG_START + 56)
	entry0Addr := uint64(PROG_START + 24)
	entry1Addr := uint64(PROG_START + 40)

	loadR3 := ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, tableAddr)
	loadR5 := ie64Instr(OP_LOAD, 5, IE64_SIZE_Q, 0, 3, 0, 8)
	jmp := ie64Instr(OP_JMP, 0, 0, 0, 5, 0, 0)
	e0body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 11)
	halt0 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	e1body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 22)
	halt1 := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR3, loadR5, jmp, e0body, halt0, e1body, halt1)

	// Write the jump table at tableAddr
	binary.LittleEndian.PutUint64(r.cpu.memory[tableAddr:], entry0Addr)
	binary.LittleEndian.PutUint64(r.cpu.memory[tableAddr+8:], entry1Addr)

	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 22 {
		t.Fatalf("R1 = %d, want 22 (should have jumped via table entry 1)", r.cpu.regs[1])
	}
}

// ===========================================================================
// Integration: Function Pointer
// ===========================================================================

func TestIE64_FunctionPointer(t *testing.T) {
	// Store function address, load it, JSR (Rs), return
	r := newIE64TestRig()

	// Layout:
	// +0:  MOVE.L R5, #funcAddr    (load function pointer)
	// +8:  JSR (R5)                 (call function)
	// +16: HALT                     (return here)
	// +24: MOVE.Q R1, #99          (function body)
	// +32: RTS
	funcAddr := uint32(PROG_START + 24)
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, funcAddr)
	jsrInd := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 99)
	rts := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(loadR5, jsrInd, halt, body, rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 99 {
		t.Fatalf("R1 = %d, want 99", r.cpu.regs[1])
	}
}

// ===========================================================================
// Integration: Mixing PC-relative JSR and register-indirect JSR
// ===========================================================================

func TestIE64_JMP_JSR_Indirect_Coexist(t *testing.T) {
	// Use PC-relative JSR to call one function, then register-indirect JSR for another
	r := newIE64TestRig()

	// Layout:
	// +0:  JSR +32 (PC-relative to sub1 at +32)
	// +8:  MOVE.L R5, #(PROG_START+48)  (load addr of sub2)
	// +16: JSR (R5)                       (call sub2)
	// +24: HALT
	// +32: MOVE.Q R1, #10               (sub1 body)
	// +40: RTS
	// +48: MOVE.Q R2, #20               (sub2 body)
	// +56: RTS
	jsr1 := ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 32) // PC-relative
	loadR5 := ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, uint32(PROG_START+48))
	jsr2 := ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0) // register-indirect
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	sub1body := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10)
	rts1 := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)
	sub2body := ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 20)
	rts2 := ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0)

	r.loadInstructions(jsr1, loadR5, jsr2, halt, sub1body, rts1, sub2body, rts2)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	if r.cpu.regs[1] != 10 {
		t.Fatalf("R1 = %d, want 10 (sub1 via PC-relative JSR)", r.cpu.regs[1])
	}
	if r.cpu.regs[2] != 20 {
		t.Fatalf("R2 = %d, want 20 (sub2 via register-indirect JSR)", r.cpu.regs[2])
	}
}

// ===========================================================================
// FPU Integration Tests
// ===========================================================================

func TestIE64_FPU_Integration(t *testing.T) {
	rig := newIE64TestRig()

	t.Run("FADD", func(t *testing.T) {
		rig.cpu.Reset()
		// Load F1=1.5, F2=2.5 via bits
		rig.cpu.FPU.setFReg(1, 1.5)
		rig.cpu.FPU.setFReg(2, 2.5)

		// FADD F0, F1, F2
		instr := ie64Instr(OP_FADD, 0, 0, 0, 1, 2, 0)
		rig.executeOne(instr)

		if rig.cpu.FPU.getFReg(0) != 4.0 {
			t.Errorf("FPU FADD via CPU failed: got %v, want 4.0", rig.cpu.FPU.getFReg(0))
		}
	})

	t.Run("FCVTFI", func(t *testing.T) {
		rig.cpu.Reset()
		rig.cpu.FPU.setFReg(1, 42.0)

		// FCVTFI R1, F1
		instr := ie64Instr(OP_FCVTFI, 1, 0, 0, 1, 0, 0)
		rig.executeOne(instr)

		if rig.cpu.getReg(1) != 42 {
			t.Errorf("FPU FCVTFI via CPU failed: got %v, want 42", rig.cpu.getReg(1))
		}
	})

	t.Run("FSTORE_FLOAD", func(t *testing.T) {
		rig.cpu.Reset()
		rig.cpu.FPU.setFReg(5, 3.14)
		rig.cpu.regs[10] = 0x2000 // base address

		// FSTORE F5, 8(R10)
		instr1 := ie64Instr(OP_FSTORE, 5, 0, 1, 10, 0, 8)
		rig.executeOne(instr1)

		// Verify memory
		memVal := binary.LittleEndian.Uint32(rig.cpu.memory[0x2008:])
		if math.Float32frombits(memVal) != 3.14 {
			t.Errorf("FSTORE failed: memory got %v, want 3.14", math.Float32frombits(memVal))
		}

		// FLOAD F0, 8(R10)
		instr2 := ie64Instr(OP_FLOAD, 0, 0, 1, 10, 0, 8)
		rig.executeOne(instr2)

		if rig.cpu.FPU.getFReg(0) != 3.14 {
			t.Errorf("FLOAD failed: got %v, want 3.14", rig.cpu.FPU.getFReg(0))
		}
	})

	t.Run("FCMP_INF", func(t *testing.T) {
		rig.cpu.Reset()
		inf := float32(math.Inf(1))
		rig.cpu.FPU.setFReg(1, inf)
		rig.cpu.FPU.setFReg(2, inf)

		// FCMP R1, F1, F2
		instr := ie64Instr(OP_FCMP, 1, 0, 0, 1, 2, 0)
		rig.executeOne(instr)

		if rig.cpu.getReg(1) != 0 {
			t.Errorf("FCMP Inf, Inf result: got %v, want 0", rig.cpu.getReg(1))
		}
		if (rig.cpu.FPU.FPSR & IE64_FPU_CC_Z) == 0 {
			t.Error("FCMP Inf, Inf: Zero bit not set")
		}
		if (rig.cpu.FPU.FPSR & IE64_FPU_CC_I) == 0 {
			t.Error("FCMP Inf, Inf: Infinity bit not set")
		}
		if (rig.cpu.FPU.FPSR & IE64_FPU_CC_NAN) != 0 {
			t.Error("FCMP Inf, Inf: NaN bit incorrectly set")
		}
	})
}
