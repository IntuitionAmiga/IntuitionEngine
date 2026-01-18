//go:build m68k_test

package main

import (
	"testing"
)

// ============================================================================
// ADD Instruction Tests
// ============================================================================

func TestAddDataRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "ADD.L_D1_D0_basic",
			DataRegs:      [8]uint32{0x00000010, 0x00000005},
			Opcodes:       []uint16{0xD081}, // ADD.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000015),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADD.L_D1_D0_overflow_positive",
			DataRegs:      [8]uint32{0x7FFFFFFF, 0x00000001},
			Opcodes:       []uint16{0xD081}, // ADD.L D1,D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZVC(1, 0, 1, 0), // N=1, V=1 (overflow)
		},
		{
			Name:          "ADD.L_D1_D0_zero_result",
			DataRegs:      [8]uint32{0xFFFFFFFF, 0x00000001},
			Opcodes:       []uint16{0xD081}, // ADD.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 1), // Z=1, C=1 (carry)
		},
		{
			Name:          "ADD.W_D1_D0_word_operation",
			DataRegs:      [8]uint32{0xFFFF0010, 0x00000005},
			Opcodes:       []uint16{0xD041}, // ADD.W D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFF0015),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADD.B_D1_D0_byte_operation",
			DataRegs:      [8]uint32{0xFFFFFF10, 0x00000005},
			Opcodes:       []uint16{0xD001}, // ADD.B D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFF15),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADD.B_D1_D0_byte_negative",
			DataRegs:      [8]uint32{0x00000070, 0x00000020},
			Opcodes:       []uint16{0xD001}, // ADD.B D1,D0
			ExpectedRegs:  Reg("D0", 0x00000090),
			ExpectedFlags: FlagsNZVC(1, 0, 1, 0), // N=1, V=1 (0x70+0x20=0x90, overflow in signed byte)
		},
	}

	RunM68KTests(t, tests)
}

func TestAddImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "ADDI.L_#$100_D0",
			DataRegs:      [8]uint32{0x00000050},
			Opcodes:       []uint16{0x0680, 0x0000, 0x0100}, // ADDI.L #$100,D0
			ExpectedRegs:  Reg("D0", 0x00000150),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADDI.W_#$FF_D0",
			DataRegs:      [8]uint32{0x00000001},
			Opcodes:       []uint16{0x0640, 0x00FF}, // ADDI.W #$FF,D0
			ExpectedRegs:  Reg("D0", 0x00000100),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADDI.B_#$10_D0",
			DataRegs:      [8]uint32{0x000000F0},
			Opcodes:       []uint16{0x0600, 0x0010}, // ADDI.B #$10,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 1), // Z=1, C=1
		},
	}

	RunM68KTests(t, tests)
}

func TestAddQuickSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "ADDQ.L_#1_D0",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x5280}, // ADDQ.L #1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADDQ.L_#8_D0",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x5080}, // ADDQ.L #8,D0
			ExpectedRegs:  Reg("D0", 0x00000008),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADDQ.W_#1_A0_no_flags",
			AddrRegs:      [8]uint32{0x00001000},
			Opcodes:       []uint16{0x5248}, // ADDQ.W #1,A0
			ExpectedRegs:  Reg("A0", 0x00001001),
			ExpectedFlags: FlagDontCare(), // Address register ops don't affect flags
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// SUB Instruction Tests
// ============================================================================

func TestSubDataRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "SUB.L_D1_D0_basic",
			DataRegs:      [8]uint32{0x00000015, 0x00000005},
			Opcodes:       []uint16{0x9081}, // SUB.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000010),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "SUB.L_D1_D0_zero_result",
			DataRegs:      [8]uint32{0x00000010, 0x00000010},
			Opcodes:       []uint16{0x9081}, // SUB.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "SUB.L_D1_D0_borrow",
			DataRegs:      [8]uint32{0x00000000, 0x00000001},
			Opcodes:       []uint16{0x9081}, // SUB.L D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1 (borrow)
		},
		{
			Name:          "SUB.L_D1_D0_overflow_negative",
			DataRegs:      [8]uint32{0x80000000, 0x00000001},
			Opcodes:       []uint16{0x9081}, // SUB.L D1,D0
			ExpectedRegs:  Reg("D0", 0x7FFFFFFF),
			ExpectedFlags: FlagsNZVC(0, 0, 1, 0), // V=1 (overflow)
		},
		{
			Name:          "SUB.W_D1_D0_word",
			DataRegs:      [8]uint32{0xFFFF0020, 0x00000010},
			Opcodes:       []uint16{0x9041}, // SUB.W D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFF0010),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "SUB.B_D1_D0_byte",
			DataRegs:      [8]uint32{0xFFFFFF20, 0x00000010},
			Opcodes:       []uint16{0x9001}, // SUB.B D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFF10),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

func TestSubImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "SUBI.L_#$100_D0",
			DataRegs:      [8]uint32{0x00000150},
			Opcodes:       []uint16{0x0480, 0x0000, 0x0100}, // SUBI.L #$100,D0
			ExpectedRegs:  Reg("D0", 0x00000050),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "SUBI.W_#$10_D0",
			DataRegs:      [8]uint32{0x00000010},
			Opcodes:       []uint16{0x0440, 0x0010}, // SUBI.W #$10,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "SUBI.B_#$01_D0_borrow",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x0400, 0x0001}, // SUBI.B #$01,D0
			ExpectedRegs:  Reg("D0", 0x000000FF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
	}

	RunM68KTests(t, tests)
}

func TestSubQuickSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "SUBQ.L_#1_D0",
			DataRegs:      [8]uint32{0x00000010},
			Opcodes:       []uint16{0x5380}, // SUBQ.L #1,D0
			ExpectedRegs:  Reg("D0", 0x0000000F),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "SUBQ.L_#8_D0",
			DataRegs:      [8]uint32{0x00000008},
			Opcodes:       []uint16{0x5180}, // SUBQ.L #8,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "SUBQ.L_#1_D0_underflow",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x5380}, // SUBQ.L #1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// MUL Instruction Tests
// ============================================================================

func TestMuluSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MULU.W_D1_D0_basic",
			DataRegs:      [8]uint32{0x00000010, 0x00000010},
			Opcodes:       []uint16{0xC0C1}, // MULU.W D1,D0
			ExpectedRegs:  Reg("D0", 0x00000100),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "MULU.W_D1_D0_zero",
			DataRegs:      [8]uint32{0x00001234, 0x00000000},
			Opcodes:       []uint16{0xC0C1}, // MULU.W D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "MULU.W_D1_D0_large",
			DataRegs:      [8]uint32{0x0000FFFF, 0x0000FFFF},
			Opcodes:       []uint16{0xC0C1}, // MULU.W D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFE0001),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1 (MSB set)
		},
	}

	RunM68KTests(t, tests)
}

func TestMulsSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "MULS.W_D1_D0_positive",
			DataRegs:      [8]uint32{0x00000010, 0x00000010},
			Opcodes:       []uint16{0xC1C1}, // MULS.W D1,D0
			ExpectedRegs:  Reg("D0", 0x00000100),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "MULS.W_D1_D0_negative_result",
			DataRegs:      [8]uint32{0x00000010, 0x0000FFF0}, // 16 * -16
			Opcodes:       []uint16{0xC1C1},                  // MULS.W D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFF00),             // -256
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),             // N=1
		},
		{
			Name:          "MULS.W_D1_D0_both_negative",
			DataRegs:      [8]uint32{0x0000FFFF, 0x0000FFFF}, // -1 * -1
			Opcodes:       []uint16{0xC1C1},                  // MULS.W D1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),             // 1
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// DIV Instruction Tests
// ============================================================================

func TestDivuSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "DIVU.W_D1_D0_basic",
			DataRegs:      [8]uint32{0x00000064, 0x0000000A}, // 100 / 10
			Opcodes:       []uint16{0x80C1},                  // DIVU.W D1,D0
			ExpectedRegs:  Reg("D0", 0x0000000A),             // quotient=10, remainder=0
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "DIVU.W_D1_D0_with_remainder",
			DataRegs:      [8]uint32{0x00000065, 0x0000000A}, // 101 / 10
			Opcodes:       []uint16{0x80C1},                  // DIVU.W D1,D0
			ExpectedRegs:  Reg("D0", 0x0001000A),             // quotient=10, remainder=1
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

func TestDivsSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "DIVS.W_D1_D0_positive",
			DataRegs:      [8]uint32{0x00000064, 0x0000000A}, // 100 / 10
			Opcodes:       []uint16{0x81C1},                  // DIVS.W D1,D0
			ExpectedRegs:  Reg("D0", 0x0000000A),             // quotient=10, remainder=0
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "DIVS.W_D1_D0_negative_dividend",
			DataRegs:      [8]uint32{0xFFFFFF9C, 0x0000000A}, // -100 / 10
			Opcodes:       []uint16{0x81C1},                  // DIVS.W D1,D0
			ExpectedRegs:  Reg("D0", 0x0000FFF6),             // quotient=-10, remainder=0
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),             // N=1
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// CMP Instruction Tests
// ============================================================================

func TestCmpDataRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "CMP.L_D1_D0_equal",
			DataRegs:      [8]uint32{0x00001234, 0x00001234},
			Opcodes:       []uint16{0xB081}, // CMP.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00001234),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "CMP.L_D1_D0_greater",
			DataRegs:      [8]uint32{0x00001235, 0x00001234},
			Opcodes:       []uint16{0xB081}, // CMP.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00001235),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "CMP.L_D1_D0_less",
			DataRegs:      [8]uint32{0x00001233, 0x00001234},
			Opcodes:       []uint16{0xB081}, // CMP.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00001233),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1 (borrow)
		},
		{
			Name:          "CMP.W_D1_D0",
			DataRegs:      [8]uint32{0xFFFF0010, 0x00000010},
			Opcodes:       []uint16{0xB041}, // CMP.W D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFF0010),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1 (word comparison)
		},
		{
			Name:          "CMP.B_D1_D0",
			DataRegs:      [8]uint32{0xFFFFFF10, 0x00000010},
			Opcodes:       []uint16{0xB001}, // CMP.B D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFF10),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1 (byte comparison)
		},
	}

	RunM68KTests(t, tests)
}

func TestCmpImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "CMPI.L_#$1234_D0_equal",
			DataRegs:      [8]uint32{0x00001234},
			Opcodes:       []uint16{0x0C80, 0x0000, 0x1234}, // CMPI.L #$1234,D0
			ExpectedRegs:  Reg("D0", 0x00001234),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "CMPI.W_#$10_D0_less",
			DataRegs:      [8]uint32{0x00000005},
			Opcodes:       []uint16{0x0C40, 0x0010}, // CMPI.W #$10,D0
			ExpectedRegs:  Reg("D0", 0x00000005),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
		{
			Name:          "CMPI.B_#$10_D0_greater",
			DataRegs:      [8]uint32{0x00000020},
			Opcodes:       []uint16{0x0C00, 0x0010}, // CMPI.B #$10,D0
			ExpectedRegs:  Reg("D0", 0x00000020),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

func TestCmpAddressSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "CMPA.L_A1_A0_equal",
			AddrRegs:      [8]uint32{0x00001000, 0x00001000},
			Opcodes:       []uint16{0xB1C9}, // CMPA.L A1,A0
			ExpectedRegs:  Reg("A0", 0x00001000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "CMPA.W_A1_A0_sign_extend",
			AddrRegs:      [8]uint32{0x00001000, 0x0000FFFF}, // A1.W = 0xFFFF sign-extended to 0xFFFFFFFF
			Opcodes:       []uint16{0xB0C9},                  // CMPA.W A1,A0
			ExpectedRegs:  Reg("A0", 0x00001000),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 1), // C=1 because source (0xFFFFFFFF) > dest (0x1000) unsigned
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// ADDX/SUBX Extended Arithmetic Tests
// ============================================================================

func TestAddxSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "ADDX.L_D1_D0_no_extend",
			DataRegs:      [8]uint32{0x00000010, 0x00000005},
			SR:            0x0000,           // X=0
			Opcodes:       []uint16{0xD181}, // ADDX.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000015),
			ExpectedFlags: FlagsAll(0, 0, 0, 0, 0),
		},
		{
			Name:          "ADDX.L_D1_D0_with_extend",
			DataRegs:      [8]uint32{0x00000010, 0x00000005},
			SR:            M68K_SR_X,        // X=1
			Opcodes:       []uint16{0xD181}, // ADDX.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000016),
			ExpectedFlags: FlagsAll(0, 0, 0, 0, 0),
		},
		{
			Name:          "ADDX.L_D1_D0_carry_propagation",
			DataRegs:      [8]uint32{0xFFFFFFFF, 0x00000000},
			SR:            M68K_SR_X,        // X=1
			Opcodes:       []uint16{0xD181}, // ADDX.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsAll(0, 0, 0, 1, 1), // C=1, X=1 (Z unchanged per M68K spec)
		},
	}

	RunM68KTests(t, tests)
}

func TestSubxSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "SUBX.L_D1_D0_no_extend",
			DataRegs:      [8]uint32{0x00000015, 0x00000005},
			SR:            0x0000,           // X=0
			Opcodes:       []uint16{0x9181}, // SUBX.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000010),
			ExpectedFlags: FlagsAll(0, 0, 0, 0, 0),
		},
		{
			Name:          "SUBX.L_D1_D0_with_extend",
			DataRegs:      [8]uint32{0x00000015, 0x00000005},
			SR:            M68K_SR_X,        // X=1
			Opcodes:       []uint16{0x9181}, // SUBX.L D1,D0
			ExpectedRegs:  Reg("D0", 0x0000000F),
			ExpectedFlags: FlagsAll(0, 0, 0, 0, 0),
		},
		{
			Name:          "SUBX.L_D1_D0_borrow_propagation",
			DataRegs:      [8]uint32{0x00000000, 0x00000000},
			SR:            M68K_SR_X,        // X=1
			Opcodes:       []uint16{0x9181}, // SUBX.L D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsAll(1, 0, 0, 1, 1), // N=1, C=1, X=1
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// NEG/NEGX Tests
// ============================================================================

func TestNegArithmeticSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "NEG.L_D0_positive",
			DataRegs:      [8]uint32{0x00000010},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFF0),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
		{
			Name:          "NEG.L_D0_negative",
			DataRegs:      [8]uint32{0xFFFFFFF0},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0x00000010),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 1), // C=1
		},
		{
			Name:          "NEG.L_D0_zero",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "NEG.L_D0_min_signed",
			DataRegs:      [8]uint32{0x80000000},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZVC(1, 0, 1, 1), // N=1, V=1, C=1
		},
		{
			Name:          "NEG.W_D0",
			DataRegs:      [8]uint32{0xFFFF0010},
			Opcodes:       []uint16{0x4440}, // NEG.W D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFF0),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
		{
			Name:          "NEG.B_D0",
			DataRegs:      [8]uint32{0xFFFFFF10},
			Opcodes:       []uint16{0x4400}, // NEG.B D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFF0),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
	}

	RunM68KTests(t, tests)
}

func TestNegxSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "NEGX.L_D0_no_extend",
			DataRegs:      [8]uint32{0x00000010},
			SR:            0x0000,           // X=0
			Opcodes:       []uint16{0x4080}, // NEGX.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFF0),
			ExpectedFlags: FlagsAll(1, 0, 0, 1, 1), // N=1, C=1, X=1
		},
		{
			Name:          "NEGX.L_D0_with_extend",
			DataRegs:      [8]uint32{0x00000010},
			SR:            M68K_SR_X,        // X=1
			Opcodes:       []uint16{0x4080}, // NEGX.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFEF),
			ExpectedFlags: FlagsAll(1, 0, 0, 1, 1), // N=1, C=1, X=1
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// EXT (Sign Extend) Tests
// ============================================================================

func TestExtSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "EXT.W_D0_positive",
			DataRegs:      [8]uint32{0xFFFFFF7F}, // byte 0x7F
			Opcodes:       []uint16{0x4880},      // EXT.W D0
			ExpectedRegs:  Reg("D0", 0xFFFF007F),
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "EXT.W_D0_negative",
			DataRegs:      [8]uint32{0xFFFFFF80}, // byte 0x80
			Opcodes:       []uint16{0x4880},      // EXT.W D0
			ExpectedRegs:  Reg("D0", 0xFFFFFF80),
			ExpectedFlags: FlagsNZ(1, 0), // N=1
		},
		{
			Name:          "EXT.L_D0_positive",
			DataRegs:      [8]uint32{0xFFFF7FFF}, // word 0x7FFF
			Opcodes:       []uint16{0x48C0},      // EXT.L D0
			ExpectedRegs:  Reg("D0", 0x00007FFF),
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "EXT.L_D0_negative",
			DataRegs:      [8]uint32{0xFFFF8000}, // word 0x8000
			Opcodes:       []uint16{0x48C0},      // EXT.L D0
			ExpectedRegs:  Reg("D0", 0xFFFF8000),
			ExpectedFlags: FlagsNZ(1, 0), // N=1
		},
		{
			Name:          "EXT.W_D0_zero",
			DataRegs:      [8]uint32{0xFFFFFF00},
			Opcodes:       []uint16{0x4880}, // EXT.W D0
			ExpectedRegs:  Reg("D0", 0xFFFF0000),
			ExpectedFlags: FlagsNZ(0, 1), // Z=1
		},
	}

	RunM68KTests(t, tests)
}
