//go:build m68k

package main

import (
	"fmt"
	"testing"
)

// FlagExpectation defines expected CPU flag states after instruction execution
// Use -1 for "don't care" (flag not checked)
type FlagExpectation struct {
	N int8 // -1 = don't care, 0 = clear, 1 = set
	Z int8
	V int8
	C int8
	X int8
}

// FlagDontCare returns a FlagExpectation where all flags are ignored
func FlagDontCare() FlagExpectation {
	return FlagExpectation{N: -1, Z: -1, V: -1, C: -1, X: -1}
}

// FlagsClear returns a FlagExpectation where all flags should be clear
func FlagsClear() FlagExpectation {
	return FlagExpectation{N: 0, Z: 0, V: 0, C: 0, X: 0}
}

// FlagsNZ returns a FlagExpectation checking only N and Z flags
func FlagsNZ(n, z int8) FlagExpectation {
	return FlagExpectation{N: n, Z: z, V: -1, C: -1, X: -1}
}

// FlagsNZVC returns a FlagExpectation checking N, Z, V, C flags (X don't care)
func FlagsNZVC(n, z, v, c int8) FlagExpectation {
	return FlagExpectation{N: n, Z: z, V: v, C: c, X: -1}
}

// FlagsAll returns a FlagExpectation checking all flags
func FlagsAll(n, z, v, c, x int8) FlagExpectation {
	return FlagExpectation{N: n, Z: z, V: v, C: c, X: x}
}

// MemoryExpectation defines expected memory content at a specific address
type MemoryExpectation struct {
	Address uint32
	Size    int // 1=byte, 2=word, 4=long
	Value   uint32
}

// M68KTestCase defines a single test case for table-driven testing
type M68KTestCase struct {
	Name string // Test name (used in t.Run)

	// Initial CPU state setup
	Setup func(*M68KCPU) // Optional custom setup function

	// Register initialization (applied after Setup)
	DataRegs [8]uint32 // D0-D7 initial values
	AddrRegs [8]uint32 // A0-A7 initial values (A7 is SP)
	PC       uint32    // Starting PC (0 = use default)
	SR       uint16    // Initial status register

	// Memory initialization
	InitialMem map[uint32]interface{} // Address -> value (uint8/uint16/uint32)

	// Instruction(s) to execute
	Opcodes []uint16 // Opcode words (main + extension words)

	// Expected results after execution
	ExpectedRegs  map[string]uint32   // "D0", "D1", "A0", etc.
	ExpectedMem   []MemoryExpectation // Memory expectations
	ExpectedFlags FlagExpectation

	// Exception handling
	ShouldTrap bool   // Whether instruction should trigger exception
	TrapVector uint16 // Expected exception vector (if ShouldTrap)

	// PC advancement check
	ExpectedPCDelta uint32 // Expected PC advancement (0 = don't check)
}

// RunM68KTests executes a slice of table-driven test cases
func RunM68KTests(t *testing.T, tests []M68KTestCase) {
	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			cpu := setupTestCPU()
			runSingleM68KTest(t, cpu, tc)
		})
	}
}

// RunM68KTestsWithCPU executes tests with a shared CPU (for sequence testing)
func RunM68KTestsWithCPU(t *testing.T, cpu *M68KCPU, tests []M68KTestCase) {
	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			runSingleM68KTest(t, cpu, tc)
		})
	}
}

// runSingleM68KTest executes a single test case
func runSingleM68KTest(t *testing.T, cpu *M68KCPU, tc M68KTestCase) {
	// Apply custom setup if provided
	if tc.Setup != nil {
		tc.Setup(cpu)
	}

	// Initialize data registers
	for i, val := range tc.DataRegs {
		if val != 0 || tc.DataRegs != [8]uint32{} {
			cpu.DataRegs[i] = val
		}
	}

	// Initialize address registers
	for i, val := range tc.AddrRegs {
		if val != 0 || tc.AddrRegs != [8]uint32{} {
			cpu.AddrRegs[i] = val
		}
	}

	// Set PC if specified
	if tc.PC != 0 {
		cpu.PC = tc.PC
	}

	// Set SR if specified
	if tc.SR != 0 {
		cpu.SR = tc.SR
	}

	// Initialize memory
	for addr, value := range tc.InitialMem {
		switch v := value.(type) {
		case uint8:
			cpu.Write8(addr, v)
		case uint16:
			cpu.Write16(addr, v)
		case uint32:
			cpu.Write32(addr, v)
		case int:
			cpu.Write32(addr, uint32(v))
		}
	}

	// Write opcodes to memory at PC
	opcodePC := cpu.PC
	for i, opcode := range tc.Opcodes {
		cpu.Write16(opcodePC+uint32(i*2), opcode)
	}

	// Track initial PC for delta check
	initialPC := cpu.PC

	// Execute instruction
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Check expected registers
	for regName, expected := range tc.ExpectedRegs {
		actual := getRegisterValue(cpu, regName)
		if actual != expected {
			t.Errorf("%s: got 0x%08X, expected 0x%08X", regName, actual, expected)
		}
	}

	// Check expected memory
	for _, mem := range tc.ExpectedMem {
		var actual uint32
		switch mem.Size {
		case 1:
			actual = uint32(cpu.Read8(mem.Address))
		case 2:
			actual = uint32(cpu.Read16(mem.Address))
		case 4:
			actual = cpu.Read32(mem.Address)
		}
		if actual != mem.Value {
			t.Errorf("Memory[0x%08X]: got 0x%X, expected 0x%X", mem.Address, actual, mem.Value)
		}
	}

	// Check flags
	checkFlags(t, cpu, tc.ExpectedFlags)

	// Check PC delta if specified
	if tc.ExpectedPCDelta > 0 {
		actualDelta := cpu.PC - initialPC
		if actualDelta != tc.ExpectedPCDelta {
			t.Errorf("PC delta: got %d, expected %d", actualDelta, tc.ExpectedPCDelta)
		}
	}
}

// getRegisterValue returns the value of a named register
func getRegisterValue(cpu *M68KCPU, name string) uint32 {
	switch name {
	case "D0":
		return cpu.DataRegs[0]
	case "D1":
		return cpu.DataRegs[1]
	case "D2":
		return cpu.DataRegs[2]
	case "D3":
		return cpu.DataRegs[3]
	case "D4":
		return cpu.DataRegs[4]
	case "D5":
		return cpu.DataRegs[5]
	case "D6":
		return cpu.DataRegs[6]
	case "D7":
		return cpu.DataRegs[7]
	case "A0":
		return cpu.AddrRegs[0]
	case "A1":
		return cpu.AddrRegs[1]
	case "A2":
		return cpu.AddrRegs[2]
	case "A3":
		return cpu.AddrRegs[3]
	case "A4":
		return cpu.AddrRegs[4]
	case "A5":
		return cpu.AddrRegs[5]
	case "A6":
		return cpu.AddrRegs[6]
	case "A7", "SP":
		return cpu.AddrRegs[7]
	case "PC":
		return cpu.PC
	case "SR":
		return uint32(cpu.SR)
	case "USP":
		return cpu.USP
	default:
		panic(fmt.Sprintf("Unknown register: %s", name))
	}
}

// checkFlags verifies CPU flags against expectations
func checkFlags(t *testing.T, cpu *M68KCPU, expected FlagExpectation) {
	if expected.N != -1 {
		actual := int8(0)
		if (cpu.SR & M68K_SR_N) != 0 {
			actual = 1
		}
		if actual != expected.N {
			t.Errorf("N flag: got %d, expected %d", actual, expected.N)
		}
	}

	if expected.Z != -1 {
		actual := int8(0)
		if (cpu.SR & M68K_SR_Z) != 0 {
			actual = 1
		}
		if actual != expected.Z {
			t.Errorf("Z flag: got %d, expected %d", actual, expected.Z)
		}
	}

	if expected.V != -1 {
		actual := int8(0)
		if (cpu.SR & M68K_SR_V) != 0 {
			actual = 1
		}
		if actual != expected.V {
			t.Errorf("V flag: got %d, expected %d", actual, expected.V)
		}
	}

	if expected.C != -1 {
		actual := int8(0)
		if (cpu.SR & M68K_SR_C) != 0 {
			actual = 1
		}
		if actual != expected.C {
			t.Errorf("C flag: got %d, expected %d", actual, expected.C)
		}
	}

	if expected.X != -1 {
		actual := int8(0)
		if (cpu.SR & M68K_SR_X) != 0 {
			actual = 1
		}
		if actual != expected.X {
			t.Errorf("X flag: got %d, expected %d", actual, expected.X)
		}
	}
}

// Helper functions for creating common opcodes

// MakeOpcodeMove creates a MOVE opcode
// size: M68K_SIZE_BYTE, M68K_SIZE_WORD, M68K_SIZE_LONG
func MakeOpcodeMove(size, srcMode, srcReg, destMode, destReg uint16) uint16 {
	var sizeField uint16
	switch size {
	case M68K_SIZE_BYTE:
		sizeField = 1 // 01
	case M68K_SIZE_WORD:
		sizeField = 3 // 11
	case M68K_SIZE_LONG:
		sizeField = 2 // 10
	}
	return (sizeField << 12) | (destReg << 9) | (destMode << 6) | (srcMode << 3) | srcReg
}

// MakeOpcodeMoveq creates a MOVEQ opcode (move quick)
func MakeOpcodeMoveq(data int8, destReg uint16) uint16 {
	return 0x7000 | (destReg << 9) | uint16(uint8(data))
}

// MakeOpcodeAddSub creates ADD or SUB opcode
// opmode: 0-2 for Dn op <ea>, 4-6 for <ea> op Dn
func MakeOpcodeAddSub(isAdd bool, reg, opmode, mode, eareg uint16) uint16 {
	var base uint16
	if isAdd {
		base = 0xD000 // ADD
	} else {
		base = 0x9000 // SUB
	}
	return base | (reg << 9) | (opmode << 6) | (mode << 3) | eareg
}

// MakeOpcodeLogical creates AND, OR, EOR opcode
func MakeOpcodeLogical(op string, reg, opmode, mode, eareg uint16) uint16 {
	var base uint16
	switch op {
	case "AND":
		base = 0xC000
	case "OR":
		base = 0x8000
	case "EOR":
		base = 0xB000
	}
	return base | (reg << 9) | (opmode << 6) | (mode << 3) | eareg
}

// MakeOpcodeBcc creates a Bcc (branch conditional) opcode
func MakeOpcodeBcc(condition, displacement uint16) uint16 {
	return 0x6000 | (condition << 8) | (displacement & 0xFF)
}

// MakeOpcodeDBcc creates a DBcc (decrement and branch) opcode
func MakeOpcodeDBcc(condition, reg uint16) uint16 {
	return 0x50C8 | (condition << 8) | reg
}

// Byte/Word/Long helper functions for memory expectations
func ExpectByte(addr uint32, val uint8) MemoryExpectation {
	return MemoryExpectation{Address: addr, Size: 1, Value: uint32(val)}
}

func ExpectWord(addr uint32, val uint16) MemoryExpectation {
	return MemoryExpectation{Address: addr, Size: 2, Value: uint32(val)}
}

func ExpectLong(addr uint32, val uint32) MemoryExpectation {
	return MemoryExpectation{Address: addr, Size: 4, Value: val}
}

// Reg helper creates a map with a single register expectation
func Reg(name string, value uint32) map[string]uint32 {
	return map[string]uint32{name: value}
}

// Regs helper creates a map with multiple register expectations
func Regs(pairs ...interface{}) map[string]uint32 {
	result := make(map[string]uint32)
	for i := 0; i < len(pairs); i += 2 {
		name := pairs[i].(string)
		value := pairs[i+1].(uint32)
		result[name] = value
	}
	return result
}

// TestTableDrivenHelpers validates the table-driven test infrastructure
func TestTableDrivenHelpers(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MOVEQ_#5_to_D0",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{MakeOpcodeMoveq(5, 0)}, // MOVEQ #5,D0
			ExpectedRegs:  Reg("D0", 5),
			ExpectedFlags: FlagsNZ(0, 0), // Not negative, not zero
		},
		{
			Name:          "MOVEQ_#0_to_D1_sets_Z",
			DataRegs:      [8]uint32{0, 0xFF, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{MakeOpcodeMoveq(0, 1)}, // MOVEQ #0,D1
			ExpectedRegs:  Reg("D1", 0),
			ExpectedFlags: FlagsNZ(0, 1), // Not negative, IS zero
		},
		{
			Name:          "MOVEQ_#-1_to_D2_sets_N",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{MakeOpcodeMoveq(-1, 2)}, // MOVEQ #-1,D2
			ExpectedRegs:  Reg("D2", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZ(1, 0), // IS negative, not zero
		},
	}

	RunM68KTests(t, tests)
}
