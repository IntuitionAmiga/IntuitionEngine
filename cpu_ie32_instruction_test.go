package main

import (
	"encoding/binary"
	"testing"
)

// ==============================================================================
// Test Helpers
// ==============================================================================

// createInstruction builds an 8-byte IE32 instruction.
// Format: [opcode, reg, addrMode, reserved, operand (4 bytes little-endian)]
func createInstruction(opcode, reg, addrMode byte, operand uint32) []byte {
	instr := make([]byte, INSTRUCTION_SIZE)
	instr[OPCODE_OFFSET] = opcode
	instr[REG_OFFSET] = reg
	instr[ADDRMODE_OFFSET] = addrMode
	instr[3] = 0 // reserved
	binary.LittleEndian.PutUint32(instr[OPERAND_OFFSET:], operand)
	return instr
}

// ie32TestRig wraps CPU and bus for testing.
type ie32TestRig struct {
	bus *SystemBus
	cpu *CPU
}

// newIE32TestRig creates a fresh CPU and bus for testing.
func newIE32TestRig() *ie32TestRig {
	bus := NewSystemBus()
	cpu := NewCPU(bus)
	return &ie32TestRig{bus: bus, cpu: cpu}
}

// loadInstructions loads a sequence of instructions at PROG_START.
func (r *ie32TestRig) loadInstructions(instructions ...[]byte) {
	offset := uint32(PROG_START)
	for _, instr := range instructions {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	r.cpu.PC = PROG_START
}

// executeOne executes a single instruction and returns.
// It sets the CPU to halt after one instruction by placing a HALT after the instruction.
func (r *ie32TestRig) executeOne(instr []byte) {
	r.loadInstructions(instr, createInstruction(HALT, 0, 0, 0))
	r.cpu.running.Store(true)
	r.cpu.Execute()
}

// executeN executes N instructions and halts.
func (r *ie32TestRig) executeN(n int, instructions ...[]byte) {
	// Add HALT after the instructions
	allInstrs := append(instructions, createInstruction(HALT, 0, 0, 0))
	r.loadInstructions(allInstrs...)
	r.cpu.running.Store(true)
	r.cpu.Execute()
}

// write32At writes a 32-bit value at the specified address in CPU memory.
func (r *ie32TestRig) write32At(addr, value uint32) {
	binary.LittleEndian.PutUint32(r.cpu.memory[addr:], value)
}

// read32At reads a 32-bit value from the specified address in CPU memory.
func (r *ie32TestRig) read32At(addr uint32) uint32 {
	return binary.LittleEndian.Uint32(r.cpu.memory[addr:])
}

// assertRegister checks that a register has the expected value.
func assertRegister(t *testing.T, cpu *CPU, regName string, expected uint32) {
	t.Helper()
	var got uint32
	switch regName {
	case "A":
		got = cpu.A
	case "B":
		got = cpu.B
	case "C":
		got = cpu.C
	case "D":
		got = cpu.D
	case "E":
		got = cpu.E
	case "F":
		got = cpu.F
	case "G":
		got = cpu.G
	case "H":
		got = cpu.H
	case "S":
		got = cpu.S
	case "T":
		got = cpu.T
	case "U":
		got = cpu.U
	case "V":
		got = cpu.V
	case "W":
		got = cpu.W
	case "X":
		got = cpu.X
	case "Y":
		got = cpu.Y
	case "Z":
		got = cpu.Z
	case "PC":
		got = cpu.PC
	case "SP":
		got = cpu.SP
	default:
		t.Fatalf("unknown register: %s", regName)
	}
	if got != expected {
		t.Fatalf("%s = 0x%08X, want 0x%08X", regName, got, expected)
	}
}

// Register index constants for instruction encoding
const (
	REG_A = 0
	REG_X = 1
	REG_Y = 2
	REG_Z = 3
	REG_B = 4
	REG_C = 5
	REG_D = 6
	REG_E = 7
	REG_F = 8
	REG_G = 9
	REG_H = 10
	REG_S = 11
	REG_T = 12
	REG_U = 13
	REG_V = 14
	REG_W = 15
)

// ==============================================================================
// Load Instruction Tests (LDA-LDZ)
// ==============================================================================

func TestLDA_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDA, 0, ADDR_IMMEDIATE, 0x12345678))
	assertRegister(t, r.cpu, "A", 0x12345678)
}

func TestLDA_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 0xDEADBEEF
	r.executeOne(createInstruction(LDA, 0, ADDR_REGISTER, REG_X))
	assertRegister(t, r.cpu, "A", 0xDEADBEEF)
}

func TestLDA_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.write32At(0x5000, 0xCAFEBABE)
	r.executeOne(createInstruction(LDA, 0, ADDR_DIRECT, 0x5000))
	assertRegister(t, r.cpu, "A", 0xCAFEBABE)
}

func TestLDA_Indirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.B = 0x5000
	r.write32At(0x5000, 0xFEEDFACE)
	r.executeOne(createInstruction(LDA, 0, ADDR_REG_IND, REG_B))
	assertRegister(t, r.cpu, "A", 0xFEEDFACE)
}

func TestLDX_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDX, 0, ADDR_IMMEDIATE, 0x11111111))
	assertRegister(t, r.cpu, "X", 0x11111111)
}

func TestLDX_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x22222222
	r.executeOne(createInstruction(LDX, 0, ADDR_REGISTER, REG_A))
	assertRegister(t, r.cpu, "X", 0x22222222)
}

func TestLDY_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDY, 0, ADDR_IMMEDIATE, 0x33333333))
	assertRegister(t, r.cpu, "Y", 0x33333333)
}

func TestLDY_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.write32At(0x6000, 0x44444444)
	r.executeOne(createInstruction(LDY, 0, ADDR_DIRECT, 0x6000))
	assertRegister(t, r.cpu, "Y", 0x44444444)
}

func TestLDZ_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDZ, 0, ADDR_IMMEDIATE, 0x55555555))
	assertRegister(t, r.cpu, "Z", 0x55555555)
}

func TestLDZ_Indirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x7000
	r.write32At(0x7000, 0x66666666)
	r.executeOne(createInstruction(LDZ, 0, ADDR_REG_IND, REG_A))
	assertRegister(t, r.cpu, "Z", 0x66666666)
}

func TestLDB_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDB, 0, ADDR_IMMEDIATE, 0x77777777))
	assertRegister(t, r.cpu, "B", 0x77777777)
}

func TestLDC_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDC, 0, ADDR_IMMEDIATE, 0x88888888))
	assertRegister(t, r.cpu, "C", 0x88888888)
}

func TestLDD_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDD, 0, ADDR_IMMEDIATE, 0x99999999))
	assertRegister(t, r.cpu, "D", 0x99999999)
}

func TestLDE_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDE, 0, ADDR_IMMEDIATE, 0xAAAAAAAA))
	assertRegister(t, r.cpu, "E", 0xAAAAAAAA)
}

func TestLDF_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDF, 0, ADDR_IMMEDIATE, 0xBBBBBBBB))
	assertRegister(t, r.cpu, "F", 0xBBBBBBBB)
}

func TestLDG_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDG, 0, ADDR_IMMEDIATE, 0xCCCCCCCC))
	assertRegister(t, r.cpu, "G", 0xCCCCCCCC)
}

func TestLDH_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDH, 0, ADDR_IMMEDIATE, 0xDDDDDDDD))
	assertRegister(t, r.cpu, "H", 0xDDDDDDDD)
}

func TestLDS_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDS, 0, ADDR_IMMEDIATE, 0xEEEEEEEE))
	assertRegister(t, r.cpu, "S", 0xEEEEEEEE)
}

func TestLDT_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDT, 0, ADDR_IMMEDIATE, 0x01010101))
	assertRegister(t, r.cpu, "T", 0x01010101)
}

func TestLDU_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDU, 0, ADDR_IMMEDIATE, 0x02020202))
	assertRegister(t, r.cpu, "U", 0x02020202)
}

func TestLDV_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDV, 0, ADDR_IMMEDIATE, 0x03030303))
	assertRegister(t, r.cpu, "V", 0x03030303)
}

func TestLDW_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.executeOne(createInstruction(LDW, 0, ADDR_IMMEDIATE, 0x04040404))
	assertRegister(t, r.cpu, "W", 0x04040404)
}

// ==============================================================================
// Store Instruction Tests (STA-STZ)
// ==============================================================================

func TestSTA_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x12345678
	r.executeOne(createInstruction(STA, 0, ADDR_DIRECT, 0x5000))
	got := r.read32At(0x5000)
	if got != 0x12345678 {
		t.Fatalf("memory[0x5000] = 0x%08X, want 0x12345678", got)
	}
}

func TestSTA_Indirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xDEADBEEF
	r.cpu.B = 0x6000
	r.executeOne(createInstruction(STA, 0, ADDR_REG_IND, REG_B))
	got := r.read32At(0x6000)
	if got != 0xDEADBEEF {
		t.Fatalf("memory[0x6000] = 0x%08X, want 0xDEADBEEF", got)
	}
}

func TestSTX_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 0x11111111
	r.executeOne(createInstruction(STX, 0, ADDR_DIRECT, 0x5004))
	got := r.read32At(0x5004)
	if got != 0x11111111 {
		t.Fatalf("memory[0x5004] = 0x%08X, want 0x11111111", got)
	}
}

func TestSTX_Indirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 0x22222222
	r.cpu.A = 0x6004
	r.executeOne(createInstruction(STX, 0, ADDR_REG_IND, REG_A))
	got := r.read32At(0x6004)
	if got != 0x22222222 {
		t.Fatalf("memory[0x6004] = 0x%08X, want 0x22222222", got)
	}
}

func TestSTY_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.Y = 0x33333333
	r.executeOne(createInstruction(STY, 0, ADDR_DIRECT, 0x5008))
	got := r.read32At(0x5008)
	if got != 0x33333333 {
		t.Fatalf("memory[0x5008] = 0x%08X, want 0x33333333", got)
	}
}

func TestSTZ_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.Z = 0x44444444
	r.executeOne(createInstruction(STZ, 0, ADDR_DIRECT, 0x500C))
	got := r.read32At(0x500C)
	if got != 0x44444444 {
		t.Fatalf("memory[0x500C] = 0x%08X, want 0x44444444", got)
	}
}

func TestSTB_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.B = 0x55555555
	r.executeOne(createInstruction(STB, 0, ADDR_DIRECT, 0x5010))
	got := r.read32At(0x5010)
	if got != 0x55555555 {
		t.Fatalf("memory[0x5010] = 0x%08X, want 0x55555555", got)
	}
}

func TestSTC_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.C = 0x66666666
	r.executeOne(createInstruction(STC, 0, ADDR_DIRECT, 0x5014))
	got := r.read32At(0x5014)
	if got != 0x66666666 {
		t.Fatalf("memory[0x5014] = 0x%08X, want 0x66666666", got)
	}
}

func TestSTD_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.D = 0x77777777
	r.executeOne(createInstruction(STD, 0, ADDR_DIRECT, 0x5018))
	got := r.read32At(0x5018)
	if got != 0x77777777 {
		t.Fatalf("memory[0x5018] = 0x%08X, want 0x77777777", got)
	}
}

func TestSTE_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.E = 0x88888888
	r.executeOne(createInstruction(STE, 0, ADDR_DIRECT, 0x501C))
	got := r.read32At(0x501C)
	if got != 0x88888888 {
		t.Fatalf("memory[0x501C] = 0x%08X, want 0x88888888", got)
	}
}

func TestSTF_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.F = 0x99999999
	r.executeOne(createInstruction(STF, 0, ADDR_DIRECT, 0x5020))
	got := r.read32At(0x5020)
	if got != 0x99999999 {
		t.Fatalf("memory[0x5020] = 0x%08X, want 0x99999999", got)
	}
}

func TestSTG_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.G = 0xAAAAAAAA
	r.executeOne(createInstruction(STG, 0, ADDR_DIRECT, 0x5024))
	got := r.read32At(0x5024)
	if got != 0xAAAAAAAA {
		t.Fatalf("memory[0x5024] = 0x%08X, want 0xAAAAAAAA", got)
	}
}

func TestSTH_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.H = 0xBBBBBBBB
	r.executeOne(createInstruction(STH, 0, ADDR_DIRECT, 0x5028))
	got := r.read32At(0x5028)
	if got != 0xBBBBBBBB {
		t.Fatalf("memory[0x5028] = 0x%08X, want 0xBBBBBBBB", got)
	}
}

func TestSTS_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.S = 0xCCCCCCCC
	r.executeOne(createInstruction(STS, 0, ADDR_DIRECT, 0x502C))
	got := r.read32At(0x502C)
	if got != 0xCCCCCCCC {
		t.Fatalf("memory[0x502C] = 0x%08X, want 0xCCCCCCCC", got)
	}
}

func TestSTT_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.T = 0xDDDDDDDD
	r.executeOne(createInstruction(STT, 0, ADDR_DIRECT, 0x5030))
	got := r.read32At(0x5030)
	if got != 0xDDDDDDDD {
		t.Fatalf("memory[0x5030] = 0x%08X, want 0xDDDDDDDD", got)
	}
}

func TestSTU_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.U = 0xEEEEEEEE
	r.executeOne(createInstruction(STU, 0, ADDR_DIRECT, 0x5034))
	got := r.read32At(0x5034)
	if got != 0xEEEEEEEE {
		t.Fatalf("memory[0x5034] = 0x%08X, want 0xEEEEEEEE", got)
	}
}

func TestSTV_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.V = 0x01010101
	r.executeOne(createInstruction(STV, 0, ADDR_DIRECT, 0x5038))
	got := r.read32At(0x5038)
	if got != 0x01010101 {
		t.Fatalf("memory[0x5038] = 0x%08X, want 0x01010101", got)
	}
}

func TestSTW_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.W = 0x02020202
	r.executeOne(createInstruction(STW, 0, ADDR_DIRECT, 0x503C))
	got := r.read32At(0x503C)
	if got != 0x02020202 {
		t.Fatalf("memory[0x503C] = 0x%08X, want 0x02020202", got)
	}
}

// ==============================================================================
// Jump Instruction Tests
// ==============================================================================

func TestJMP(t *testing.T) {
	r := newIE32TestRig()
	// JMP to 0x2000, then HALT at 0x2000
	jmp := createInstruction(JMP, 0, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jmp)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	// PC should be at 0x2000 after executing HALT
	// (HALT doesn't increment PC)
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJNZ_Branch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 1 // Non-zero, should branch
	// JNZ A, 0x2000
	jnz := createInstruction(JNZ, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jnz)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJNZ_NoBranch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0 // Zero, should NOT branch
	jnz := createInstruction(JNZ, REG_A, 0, 0x2000)
	r.executeOne(jnz)
	// Should have advanced past the JNZ instruction
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJZ_Branch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 0 // Zero, should branch
	jz := createInstruction(JZ, REG_X, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jz)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJZ_NoBranch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 1 // Non-zero, should NOT branch
	jz := createInstruction(JZ, REG_X, 0, 0x2000)
	r.executeOne(jz)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJGT_Branch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 5 // Positive, should branch
	jgt := createInstruction(JGT, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jgt)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJGT_NoBranch_Zero(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0 // Zero, should NOT branch (> 0 is false)
	jgt := createInstruction(JGT, REG_A, 0, 0x2000)
	r.executeOne(jgt)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJGT_NoBranch_Negative(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFFFFFFFF // -1 as signed, should NOT branch
	jgt := createInstruction(JGT, REG_A, 0, 0x2000)
	r.executeOne(jgt)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJGE_Branch_Positive(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 5 // Positive, should branch
	jge := createInstruction(JGE, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jge)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJGE_Branch_Zero(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0 // Zero, should branch (>= 0)
	jge := createInstruction(JGE, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jge)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJGE_NoBranch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFFFFFFFF // -1 as signed, should NOT branch
	jge := createInstruction(JGE, REG_A, 0, 0x2000)
	r.executeOne(jge)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJLT_Branch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFFFFFFFF // -1 as signed, should branch
	jlt := createInstruction(JLT, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jlt)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJLT_NoBranch_Zero(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0 // Zero, should NOT branch (< 0 is false)
	jlt := createInstruction(JLT, REG_A, 0, 0x2000)
	r.executeOne(jlt)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJLT_NoBranch_Positive(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 5 // Positive, should NOT branch
	jlt := createInstruction(JLT, REG_A, 0, 0x2000)
	r.executeOne(jlt)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestJLE_Branch_Negative(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFFFFFFFF // -1 as signed, should branch
	jle := createInstruction(JLE, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jle)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJLE_Branch_Zero(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0 // Zero, should branch (<= 0)
	jle := createInstruction(JLE, REG_A, 0, 0x2000)
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(jle)
	copy(r.cpu.memory[0x2000:], halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	assertRegister(t, r.cpu, "PC", 0x2000)
}

func TestJLE_NoBranch(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 5 // Positive, should NOT branch
	jle := createInstruction(JLE, REG_A, 0, 0x2000)
	r.executeOne(jle)
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

// ==============================================================================
// Arithmetic Instruction Tests
// ==============================================================================

func TestADD_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 10
	r.executeOne(createInstruction(ADD, REG_A, ADDR_IMMEDIATE, 5))
	assertRegister(t, r.cpu, "A", 15)
}

func TestADD_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 100
	r.cpu.B = 50
	r.executeOne(createInstruction(ADD, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 150)
}

func TestADD_Overflow(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFFFFFFFF
	r.executeOne(createInstruction(ADD, REG_A, ADDR_IMMEDIATE, 1))
	assertRegister(t, r.cpu, "A", 0) // Wraps around
}

func TestSUB_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 20
	r.executeOne(createInstruction(SUB, REG_A, ADDR_IMMEDIATE, 5))
	assertRegister(t, r.cpu, "A", 15)
}

func TestSUB_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 100
	r.cpu.Y = 30
	r.executeOne(createInstruction(SUB, REG_X, ADDR_REGISTER, REG_Y))
	assertRegister(t, r.cpu, "X", 70)
}

func TestSUB_Underflow(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0
	r.executeOne(createInstruction(SUB, REG_A, ADDR_IMMEDIATE, 1))
	assertRegister(t, r.cpu, "A", 0xFFFFFFFF) // Wraps around
}

func TestMUL_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 7
	r.executeOne(createInstruction(MUL, REG_A, ADDR_IMMEDIATE, 6))
	assertRegister(t, r.cpu, "A", 42)
}

func TestMUL_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 12
	r.cpu.B = 11
	r.executeOne(createInstruction(MUL, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 132)
}

func TestDIV_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 100
	r.executeOne(createInstruction(DIV, REG_A, ADDR_IMMEDIATE, 10))
	assertRegister(t, r.cpu, "A", 10)
}

func TestDIV_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 99
	r.cpu.B = 3
	r.executeOne(createInstruction(DIV, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 33)
}

func TestDIV_Truncates(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 10
	r.executeOne(createInstruction(DIV, REG_A, ADDR_IMMEDIATE, 3))
	assertRegister(t, r.cpu, "A", 3) // 10/3 = 3 (truncated)
}

func TestMOD_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 17
	r.executeOne(createInstruction(MOD, REG_A, ADDR_IMMEDIATE, 5))
	assertRegister(t, r.cpu, "A", 2) // 17 % 5 = 2
}

func TestMOD_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 100
	r.cpu.B = 30
	r.executeOne(createInstruction(MOD, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 10) // 100 % 30 = 10
}

// Power-of-2 fast path tests (uses shift/AND instead of mul/div/mod)
func TestMUL_PowerOf2(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 25
	r.executeOne(createInstruction(MUL, REG_A, ADDR_IMMEDIATE, 8)) // 25 * 8 = 200
	assertRegister(t, r.cpu, "A", 200)
}

func TestMUL_PowerOf2_By2(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 123
	r.executeOne(createInstruction(MUL, REG_A, ADDR_IMMEDIATE, 2)) // 123 * 2 = 246
	assertRegister(t, r.cpu, "A", 246)
}

func TestDIV_PowerOf2(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 256
	r.executeOne(createInstruction(DIV, REG_A, ADDR_IMMEDIATE, 16)) // 256 / 16 = 16
	assertRegister(t, r.cpu, "A", 16)
}

func TestDIV_PowerOf2_By2(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 100
	r.executeOne(createInstruction(DIV, REG_A, ADDR_IMMEDIATE, 2)) // 100 / 2 = 50
	assertRegister(t, r.cpu, "A", 50)
}

func TestMOD_PowerOf2(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 100
	r.executeOne(createInstruction(MOD, REG_A, ADDR_IMMEDIATE, 16)) // 100 % 16 = 4
	assertRegister(t, r.cpu, "A", 4)
}

func TestMOD_PowerOf2_By8(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 75
	r.executeOne(createInstruction(MOD, REG_A, ADDR_IMMEDIATE, 8)) // 75 % 8 = 3
	assertRegister(t, r.cpu, "A", 3)
}

// ==============================================================================
// Logical Instruction Tests
// ==============================================================================

func TestAND_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFF00FF00
	r.executeOne(createInstruction(AND, REG_A, ADDR_IMMEDIATE, 0x0F0F0F0F))
	assertRegister(t, r.cpu, "A", 0x0F000F00)
}

func TestAND_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xAAAAAAAA
	r.cpu.B = 0x55555555
	r.executeOne(createInstruction(AND, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 0x00000000)
}

func TestOR_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFF00FF00
	r.executeOne(createInstruction(OR, REG_A, ADDR_IMMEDIATE, 0x00FF00FF))
	assertRegister(t, r.cpu, "A", 0xFFFFFFFF)
}

func TestOR_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xAAAAAAAA
	r.cpu.B = 0x55555555
	r.executeOne(createInstruction(OR, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 0xFFFFFFFF)
}

func TestXOR_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFF00FF00
	r.executeOne(createInstruction(XOR, REG_A, ADDR_IMMEDIATE, 0xFFFFFFFF))
	assertRegister(t, r.cpu, "A", 0x00FF00FF)
}

func TestXOR_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x12345678
	r.cpu.B = 0x12345678
	r.executeOne(createInstruction(XOR, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 0x00000000) // Same value XORs to 0
}

func TestNOT(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFF00FF00
	r.executeOne(createInstruction(NOT, REG_A, 0, 0))
	assertRegister(t, r.cpu, "A", 0x00FF00FF)
}

func TestSHL_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x00000001
	r.executeOne(createInstruction(SHL, REG_A, ADDR_IMMEDIATE, 4))
	assertRegister(t, r.cpu, "A", 0x00000010)
}

func TestSHL_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x00000001
	r.cpu.B = 8
	r.executeOne(createInstruction(SHL, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 0x00000100)
}

func TestSHR_Immediate(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x80000000
	r.executeOne(createInstruction(SHR, REG_A, ADDR_IMMEDIATE, 4))
	assertRegister(t, r.cpu, "A", 0x08000000)
}

func TestSHR_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x00001000
	r.cpu.B = 8
	r.executeOne(createInstruction(SHR, REG_A, ADDR_REGISTER, REG_B))
	assertRegister(t, r.cpu, "A", 0x00000010)
}

// ==============================================================================
// Stack Instruction Tests
// ==============================================================================

func TestPUSH_POP(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xDEADBEEF
	r.cpu.B = 0
	// PUSH A, POP B
	push := createInstruction(PUSH, REG_A, 0, 0)
	pop := createInstruction(POP, REG_B, 0, 0)
	r.executeN(2, push, pop)
	assertRegister(t, r.cpu, "B", 0xDEADBEEF)
}

func TestPUSH_POP_Multiple(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x11111111
	r.cpu.B = 0x22222222
	r.cpu.C = 0
	r.cpu.D = 0
	// PUSH A, PUSH B, POP D, POP C (LIFO order)
	pushA := createInstruction(PUSH, REG_A, 0, 0)
	pushB := createInstruction(PUSH, REG_B, 0, 0)
	popD := createInstruction(POP, REG_D, 0, 0)
	popC := createInstruction(POP, REG_C, 0, 0)
	r.executeN(4, pushA, pushB, popD, popC)
	assertRegister(t, r.cpu, "C", 0x11111111) // Was pushed first, popped last
	assertRegister(t, r.cpu, "D", 0x22222222) // Was pushed last, popped first
}

func TestJSR_RTS(t *testing.T) {
	r := newIE32TestRig()
	// JSR to subroutine at 0x3000, subroutine has RTS, then HALT
	jsr := createInstruction(JSR, 0, 0, 0x3000)
	rts := createInstruction(RTS, 0, 0, 0)
	halt := createInstruction(HALT, 0, 0, 0)
	lda := createInstruction(LDA, 0, ADDR_IMMEDIATE, 0x99) // After return

	r.loadInstructions(jsr, lda, halt)
	copy(r.cpu.memory[0x3000:], rts)
	r.cpu.running.Store(true)
	r.cpu.Execute()

	// After RTS, should return to instruction after JSR (LDA), then HALT
	// A should be loaded with 0x99
	assertRegister(t, r.cpu, "A", 0x99)
}

// ==============================================================================
// INC/DEC Instruction Tests
// ==============================================================================

func TestINC_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 10
	r.executeOne(createInstruction(INC, 0, ADDR_REGISTER, REG_A))
	assertRegister(t, r.cpu, "A", 11)
}

func TestINC_Memory(t *testing.T) {
	r := newIE32TestRig()
	r.write32At(0x5000, 100)
	r.executeOne(createInstruction(INC, 0, ADDR_DIRECT, 0x5000))
	got := r.read32At(0x5000)
	if got != 101 {
		t.Fatalf("memory[0x5000] = %d, want 101", got)
	}
}

func TestINC_RegisterIndirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.B = 0x6000
	r.write32At(0x6000, 50)
	r.executeOne(createInstruction(INC, 0, ADDR_REG_IND, REG_B))
	got := r.read32At(0x6000)
	if got != 51 {
		t.Fatalf("memory[0x6000] = %d, want 51", got)
	}
}

func TestDEC_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 20
	r.executeOne(createInstruction(DEC, 0, ADDR_REGISTER, REG_X))
	assertRegister(t, r.cpu, "X", 19)
}

func TestDEC_Memory(t *testing.T) {
	r := newIE32TestRig()
	r.write32At(0x5000, 200)
	r.executeOne(createInstruction(DEC, 0, ADDR_DIRECT, 0x5000))
	got := r.read32At(0x5000)
	if got != 199 {
		t.Fatalf("memory[0x5000] = %d, want 199", got)
	}
}

func TestDEC_RegisterIndirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.B = 0x6000
	r.write32At(0x6000, 75)
	r.executeOne(createInstruction(DEC, 0, ADDR_REG_IND, REG_B))
	got := r.read32At(0x6000)
	if got != 74 {
		t.Fatalf("memory[0x6000] = %d, want 74", got)
	}
}

// ==============================================================================
// Addressing Mode Tests
// ==============================================================================

func TestResolveOperand_Immediate(t *testing.T) {
	r := newIE32TestRig()
	got := r.cpu.resolveOperand(ADDR_IMMEDIATE, 0x12345678)
	if got != 0x12345678 {
		t.Fatalf("resolveOperand(ADDR_IMMEDIATE, 0x12345678) = 0x%08X, want 0x12345678", got)
	}
}

func TestResolveOperand_Register(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.X = 0xDEADBEEF
	got := r.cpu.resolveOperand(ADDR_REGISTER, REG_X)
	if got != 0xDEADBEEF {
		t.Fatalf("resolveOperand(ADDR_REGISTER, X) = 0x%08X, want 0xDEADBEEF", got)
	}
}

func TestResolveOperand_RegisterIndirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.B = 0x5000
	r.write32At(0x5000, 0xCAFEBABE)
	got := r.cpu.resolveOperand(ADDR_REG_IND, REG_B)
	if got != 0xCAFEBABE {
		t.Fatalf("resolveOperand(ADDR_REG_IND, B) = 0x%08X, want 0xCAFEBABE", got)
	}
}

func TestResolveOperand_MemoryIndirect(t *testing.T) {
	r := newIE32TestRig()
	r.write32At(0x5000, 0xFEEDFACE)
	got := r.cpu.resolveOperand(ADDR_MEM_IND, 0x5000)
	if got != 0xFEEDFACE {
		t.Fatalf("resolveOperand(ADDR_MEM_IND, 0x5000) = 0x%08X, want 0xFEEDFACE", got)
	}
}

func TestResolveOperand_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.write32At(0x6000, 0xBADC0DE0)
	got := r.cpu.resolveOperand(ADDR_DIRECT, 0x6000)
	if got != 0xBADC0DE0 {
		t.Fatalf("resolveOperand(ADDR_DIRECT, 0x6000) = 0x%08X, want 0xBADC0DE0", got)
	}
}

// ==============================================================================
// Generic LOAD/STORE Instruction Tests
// ==============================================================================

func TestLOAD_Generic(t *testing.T) {
	r := newIE32TestRig()
	// LOAD reg=X, immediate 0x12345678
	r.executeOne(createInstruction(LOAD, REG_X, ADDR_IMMEDIATE, 0x12345678))
	assertRegister(t, r.cpu, "X", 0x12345678)
}

func TestSTORE_Generic_Direct(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.Y = 0xABCDEF01
	// STORE reg=Y to address 0x5000
	r.executeOne(createInstruction(STORE, REG_Y, ADDR_DIRECT, 0x5000))
	got := r.read32At(0x5000)
	if got != 0xABCDEF01 {
		t.Fatalf("memory[0x5000] = 0x%08X, want 0xABCDEF01", got)
	}
}

func TestSTORE_Generic_RegisterIndirect(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0xFACE1234
	r.cpu.B = 0x7000
	// STORE reg=A to [B]
	r.executeOne(createInstruction(STORE, REG_A, ADDR_REG_IND, REG_B))
	got := r.read32At(0x7000)
	if got != 0xFACE1234 {
		t.Fatalf("memory[0x7000] = 0x%08X, want 0xFACE1234", got)
	}
}

// ==============================================================================
// NOP and HALT Tests
// ==============================================================================

func TestNOP(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x12345678
	nop := createInstruction(NOP, 0, 0, 0)
	r.executeOne(nop)
	// NOP should not modify any registers
	assertRegister(t, r.cpu, "A", 0x12345678)
	// PC should advance past NOP to HALT
	assertRegister(t, r.cpu, "PC", PROG_START+INSTRUCTION_SIZE)
}

func TestHALT(t *testing.T) {
	r := newIE32TestRig()
	halt := createInstruction(HALT, 0, 0, 0)
	r.loadInstructions(halt)
	r.cpu.running.Store(true)
	r.cpu.Execute()
	// Running should be false after HALT
	if r.cpu.running.Load() {
		t.Fatal("CPU should not be running after HALT")
	}
}

// ==============================================================================
// Register Indirect with Offset Tests
// ==============================================================================

func TestLoad_RegisterIndirect_WithOffset(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.B = 0x5000
	r.write32At(0x5010, 0x11223344) // B + 0x10
	// Load from [B+0x10] - offset is encoded in upper bits of operand
	// operand = (offset & ^0x0F) | reg_index
	operand := uint32(0x10) | REG_B // 0x14 = offset 0x10, reg B (index 4)
	r.executeOne(createInstruction(LDA, 0, ADDR_REG_IND, operand))
	assertRegister(t, r.cpu, "A", 0x11223344)
}

func TestStore_RegisterIndirect_WithOffset(t *testing.T) {
	r := newIE32TestRig()
	r.cpu.A = 0x55667788
	r.cpu.B = 0x5000
	// Store to [B+0x20]
	operand := uint32(0x20) | REG_B
	r.executeOne(createInstruction(STA, 0, ADDR_REG_IND, operand))
	got := r.read32At(0x5020)
	if got != 0x55667788 {
		t.Fatalf("memory[0x5020] = 0x%08X, want 0x55667788", got)
	}
}
