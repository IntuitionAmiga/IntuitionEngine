//go:build m68k_test

package main

import (
	"testing"
)

// ============================================================================
// Phase 2: Systematic Test Coverage Using Table-Driven Tests
// ============================================================================
// This file adds systematic edge-case and flag behavior tests using the
// table-driven test infrastructure from m68k_test_helpers_test.go
// ============================================================================

// TestMoveqSystematic tests MOVEQ with all edge cases and flag behaviors
func TestMoveqSystematic(t *testing.T) {
	tests := []M68KTestCase{
		// Basic values
		{
			Name:          "MOVEQ_#0_sets_Z_flag",
			Opcodes:       []uint16{MakeOpcodeMoveq(0, 0)},
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "MOVEQ_#1_clears_flags",
			Opcodes:       []uint16{MakeOpcodeMoveq(1, 0)},
			ExpectedRegs:  Reg("D0", 1),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "MOVEQ_#127_max_positive",
			Opcodes:       []uint16{MakeOpcodeMoveq(127, 0)},
			ExpectedRegs:  Reg("D0", 127),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		// Negative values (sign extension)
		{
			Name:          "MOVEQ_#-1_sign_extends",
			Opcodes:       []uint16{MakeOpcodeMoveq(-1, 0)},
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		{
			Name:          "MOVEQ_#-128_min_negative",
			Opcodes:       []uint16{MakeOpcodeMoveq(-128, 0)},
			ExpectedRegs:  Reg("D0", 0xFFFFFF80),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		// Different destination registers
		{
			Name:          "MOVEQ_#42_to_D3",
			Opcodes:       []uint16{MakeOpcodeMoveq(42, 3)},
			ExpectedRegs:  Reg("D3", 42),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "MOVEQ_#-50_to_D7",
			Opcodes:       []uint16{MakeOpcodeMoveq(-50, 7)},
			ExpectedRegs:  Reg("D7", 0xFFFFFFCE),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

// TestAddqSystematic tests ADDQ instruction edge cases
func TestAddqSystematic(t *testing.T) {
	tests := []M68KTestCase{
		// Basic addition to data register
		{
			Name:          "ADDQ_#1_D0_basic",
			DataRegs:      [8]uint32{5, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5240}, // ADDQ.W #1,D0
			ExpectedRegs:  Reg("D0", 6),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "ADDQ_#8_D0_max_quick",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5040}, // ADDQ.W #8,D0 (0 encodes as 8)
			ExpectedRegs:  Reg("D0", 8),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		// Zero result
		{
			Name:          "ADDQ_#1_to_-1_byte_zeros",
			DataRegs:      [8]uint32{0x000000FF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5200}, // ADDQ.B #1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 1), // Z=1, C=1 (byte overflow)
		},
		// Overflow detection
		{
			Name:          "ADDQ_#1_to_0x7FFF_word_overflow",
			DataRegs:      [8]uint32{0x00007FFF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5240}, // ADDQ.W #1,D0
			ExpectedRegs:  Reg("D0", 0x00008000),
			ExpectedFlags: FlagsNZVC(1, 0, 1, 0), // N=1, V=1 (pos+pos=neg)
		},
	}

	RunM68KTests(t, tests)
}

// TestSubqSystematic tests SUBQ instruction edge cases
func TestSubqSystematic(t *testing.T) {
	tests := []M68KTestCase{
		// Basic subtraction
		{
			Name:          "SUBQ_#1_D0_basic",
			DataRegs:      [8]uint32{10, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5340}, // SUBQ.W #1,D0
			ExpectedRegs:  Reg("D0", 9),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		// Zero result
		{
			Name:          "SUBQ_#1_from_1_zeros",
			DataRegs:      [8]uint32{1, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5340}, // SUBQ.W #1,D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		// Borrow (underflow)
		{
			Name:          "SUBQ_#1_from_0_borrows",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5340}, // SUBQ.W #1,D0
			ExpectedRegs:  Reg("D0", 0x0000FFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1
		},
		// Signed overflow
		{
			Name:          "SUBQ_#1_from_0x8000_overflow",
			DataRegs:      [8]uint32{0x00008000, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x5340}, // SUBQ.W #1,D0
			ExpectedRegs:  Reg("D0", 0x00007FFF),
			ExpectedFlags: FlagsNZVC(0, 0, 1, 0), // V=1 (neg-pos=pos is overflow)
		},
	}

	RunM68KTests(t, tests)
}

// TestLogicalAndSystematic tests AND instruction edge cases
func TestLogicalAndSystematic(t *testing.T) {
	tests := []M68KTestCase{
		// Basic AND operations
		{
			Name:          "AND_D1_D0_all_bits_set",
			DataRegs:      [8]uint32{0xFFFFFFFF, 0xFFFFFFFF, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC081}, // AND.L D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		{
			Name:          "AND_D1_D0_zero_result",
			DataRegs:      [8]uint32{0xAAAAAAAA, 0x55555555, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC081}, // AND.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "AND_D1_D0_partial_mask",
			DataRegs:      [8]uint32{0x12345678, 0x0000FF00, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC081}, // AND.L D1,D0
			ExpectedRegs:  Reg("D0", 0x00005600),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

// TestLogicalOrSystematic tests OR instruction edge cases
func TestLogicalOrSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "OR_D1_D0_combine_bits",
			DataRegs:      [8]uint32{0xAAAAAAAA, 0x55555555, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x8081}, // OR.L D1,D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		{
			Name:          "OR_D1_D0_zero_operand",
			DataRegs:      [8]uint32{0x12345678, 0x00000000, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x8081}, // OR.L D1,D0
			ExpectedRegs:  Reg("D0", 0x12345678),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "OR_D1_D0_both_zero",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x8081}, // OR.L D1,D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
	}

	RunM68KTests(t, tests)
}

// TestLogicalEorSystematic tests EOR instruction edge cases
// EOR encoding: 1011 rrr ooo mmm rrr where rrr=source Dn, ooo=opmode, mmm/rrr=EA
// opmode 100=byte, 101=word, 110=long (EOR Dn,<ea> stores result in <ea>)
func TestLogicalEorSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "EOR_D1_D0_toggle_all",
			DataRegs:      [8]uint32{0xAAAAAAAA, 0xFFFFFFFF, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xB380}, // EOR.L D1,D0 (1011 001 110 000 000)
			ExpectedRegs:  Reg("D0", 0x55555555),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "EOR_D1_D0_same_values_zero",
			DataRegs:      [8]uint32{0x12345678, 0x12345678, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xB380}, // EOR.L D1,D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "EOR_D1_D0_negative_result",
			DataRegs:      [8]uint32{0x00000000, 0x80000000, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xB380}, // EOR.L D1,D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
	}

	RunM68KTests(t, tests)
}

// TestNotSystematic tests NOT instruction
func TestNotSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "NOT_D0_all_ones",
			DataRegs:      [8]uint32{0xFFFFFFFF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4680}, // NOT.L D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "NOT_D0_all_zeros",
			DataRegs:      [8]uint32{0x00000000, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4680}, // NOT.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		{
			Name:          "NOT_D0_byte_size",
			DataRegs:      [8]uint32{0xFFFFFF00, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4600}, // NOT.B D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1 (byte MSB is set)
		},
	}

	RunM68KTests(t, tests)
}

// TestNegSystematic tests NEG instruction
func TestNegSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "NEG_D0_positive_to_negative",
			DataRegs:      [8]uint32{5, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4480},      // NEG.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFB), // -5
			ExpectedFlags: FlagsNZVC(1, 0, 0, 1), // N=1, C=1 (borrow)
		},
		{
			Name:          "NEG_D0_negative_to_positive",
			DataRegs:      [8]uint32{0xFFFFFFFB, 0, 0, 0, 0, 0, 0, 0}, // -5
			Opcodes:       []uint16{0x4480},                           // NEG.L D0
			ExpectedRegs:  Reg("D0", 5),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 1), // C=1 (always set for non-zero)
		},
		{
			Name:          "NEG_D0_zero",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1, C=0 for zero
		},
		{
			Name:          "NEG_D0_min_signed_overflow",
			DataRegs:      [8]uint32{0x80000000, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZVC(1, 0, 1, 1), // N=1, V=1, C=1 (overflow: -MIN_INT = MIN_INT)
		},
	}

	RunM68KTests(t, tests)
}

// TestClrSystematic tests CLR instruction
func TestClrSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "CLR_D0_long",
			DataRegs:      [8]uint32{0xDEADBEEF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4280}, // CLR.L D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "CLR_D0_word",
			DataRegs:      [8]uint32{0xDEADBEEF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4240}, // CLR.W D0
			ExpectedRegs:  Reg("D0", 0xDEAD0000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "CLR_D0_byte",
			DataRegs:      [8]uint32{0xDEADBEEF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4200}, // CLR.B D0
			ExpectedRegs:  Reg("D0", 0xDEADBE00),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
	}

	RunM68KTests(t, tests)
}

// TestTstSystematic tests TST instruction
func TestTstSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "TST_D0_zero",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4A80}, // TST.L D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:          "TST_D0_positive",
			DataRegs:      [8]uint32{0x12345678, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4A80}, // TST.L D0
			ExpectedRegs:  Reg("D0", 0x12345678),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "TST_D0_negative",
			DataRegs:      [8]uint32{0x80000000, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4A80}, // TST.L D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		{
			Name:          "TST_D0_byte_negative",
			DataRegs:      [8]uint32{0x000000FF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4A00}, // TST.B D0
			ExpectedRegs:  Reg("D0", 0x000000FF),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1 (byte MSB set)
		},
	}

	RunM68KTests(t, tests)
}

// TestSwapSystematic tests SWAP instruction
func TestSwapSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "SWAP_D0_basic",
			DataRegs:      [8]uint32{0x12345678, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4840}, // SWAP D0
			ExpectedRegs:  Reg("D0", 0x56781234),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:          "SWAP_D0_high_word_negative",
			DataRegs:      [8]uint32{0x0000FFFF, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4840}, // SWAP D0
			ExpectedRegs:  Reg("D0", 0xFFFF0000),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
		{
			Name:          "SWAP_D0_zero",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4840}, // SWAP D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
	}

	RunM68KTests(t, tests)
}
