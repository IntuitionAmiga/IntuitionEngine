//go:build m68k_test

package main

import (
	"testing"
)

// =============================================================================
// ABCD (Add BCD with Extend) Tests
// =============================================================================

// TestAbcdRegisterSystematic tests ABCD Dx,Dy - add BCD values in data registers
func TestAbcdRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "ABCD_D0_D1_simple",
			DataRegs: [8]uint32{0x09, 0x01, 0, 0, 0, 0, 0, 0}, // 09 + 01 = 10 (BCD)
			// ABCD D0,D1: 1100 001 10000 0000 = 0xC300
			Opcodes:       []uint16{0xC300},
			ExpectedRegs:  Reg("D1", 0x10),
			ExpectedFlags: FlagsAll(0, 0, -1, 0, 0), // N=0, Z=0, C=0, X=0
		},
		{
			Name:     "ABCD_D0_D1_with_carry",
			DataRegs: [8]uint32{0x99, 0x01, 0, 0, 0, 0, 0, 0}, // 99 + 01 = 00 with carry
			// ABCD D0,D1: 0xC300
			Opcodes:       []uint16{0xC300},
			ExpectedRegs:  Reg("D1", 0x00),
			ExpectedFlags: FlagsAll(-1, -1, -1, 1, 1), // C=1, X=1 (carry)
		},
		{
			Name:     "ABCD_D0_D1_with_X_flag",
			DataRegs: [8]uint32{0x09, 0x00, 0, 0, 0, 0, 0, 0}, // 09 + 00 + X(1) = 10
			SR:       M68K_SR_X,                               // X flag set
			// ABCD D0,D1: 0xC300
			Opcodes:       []uint16{0xC300},
			ExpectedRegs:  Reg("D1", 0x10),
			ExpectedFlags: FlagsAll(-1, 0, -1, 0, 0), // Z=0 (non-zero result)
		},
		{
			Name:     "ABCD_D0_D1_both_nibbles_adjust",
			DataRegs: [8]uint32{0x55, 0x66, 0, 0, 0, 0, 0, 0}, // 55 + 66 = 121 (BCD overflow) -> 21
			// ABCD D0,D1: 0xC300
			Opcodes:       []uint16{0xC300},
			ExpectedRegs:  Reg("D1", 0x21),
			ExpectedFlags: FlagsAll(-1, 0, -1, 1, 1), // C=1, X=1
		},
		{
			Name:     "ABCD_D2_D3_zero_result_preserves_Z",
			DataRegs: [8]uint32{0, 0, 0x00, 0x00, 0, 0, 0, 0}, // 00 + 00 = 00
			SR:       M68K_SR_Z,                               // Z was set
			// ABCD D2,D3: 1100 011 10000 0010 = 0xC702
			Opcodes:       []uint16{0xC702},
			ExpectedRegs:  Reg("D3", 0x00),
			ExpectedFlags: FlagsAll(-1, 1, -1, 0, 0), // Z preserved
		},
	}

	RunM68KTests(t, tests)
}

// TestAbcdMemorySystematic tests ABCD -(Ax),-(Ay) - add BCD values in memory with predecrement
func TestAbcdMemorySystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "ABCD_memory_simple",
			AddrRegs: [8]uint32{0x1001, 0x2001, 0, 0, 0, 0, 0, 0x8000}, // A0, A1 point past data
			InitialMem: map[uint32]interface{}{
				0x1000: uint8(0x09), // Source
				0x2000: uint8(0x01), // Destination
			},
			// ABCD -(A0),-(A1): 1100 001 10000 1000 = 0xC308
			Opcodes:      []uint16{0xC308},
			ExpectedRegs: Regs("A0", uint32(0x1000), "A1", uint32(0x2000)),
			ExpectedMem:  []MemoryExpectation{ExpectByte(0x2000, 0x10)},
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// SBCD (Subtract BCD with Extend) Tests
// =============================================================================

// TestSbcdRegisterSystematic tests SBCD Dx,Dy - subtract BCD values in data registers
func TestSbcdRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "SBCD_D0_D1_simple",
			DataRegs: [8]uint32{0x01, 0x10, 0, 0, 0, 0, 0, 0}, // 10 - 01 = 09 (BCD)
			// SBCD D0,D1: 1000 001 10000 0000 = 0x8300
			Opcodes:       []uint16{0x8300},
			ExpectedRegs:  Reg("D1", 0x09),
			ExpectedFlags: FlagsAll(-1, 0, -1, 0, 0), // No borrow
		},
		{
			Name:     "SBCD_D0_D1_with_borrow",
			DataRegs: [8]uint32{0x01, 0x00, 0, 0, 0, 0, 0, 0}, // 00 - 01 = 99 with borrow
			// SBCD D0,D1: 0x8300
			Opcodes:       []uint16{0x8300},
			ExpectedRegs:  Reg("D1", 0x99),
			ExpectedFlags: FlagsAll(-1, 0, -1, 1, 1), // C=1, X=1 (borrow)
		},
		{
			Name:     "SBCD_D0_D1_with_X_flag",
			DataRegs: [8]uint32{0x00, 0x10, 0, 0, 0, 0, 0, 0}, // 10 - 00 - X(1) = 09
			SR:       M68K_SR_X,                               // X flag set
			// SBCD D0,D1: 0x8300
			Opcodes:       []uint16{0x8300},
			ExpectedRegs:  Reg("D1", 0x09),
			ExpectedFlags: FlagsAll(-1, 0, -1, 0, 0),
		},
		{
			Name:     "SBCD_D0_D1_low_nibble_borrow",
			DataRegs: [8]uint32{0x05, 0x22, 0, 0, 0, 0, 0, 0}, // 22 - 05 = 17 (BCD)
			// SBCD D0,D1: 0x8300
			Opcodes:       []uint16{0x8300},
			ExpectedRegs:  Reg("D1", 0x17),
			ExpectedFlags: FlagsAll(-1, 0, -1, 0, 0),
		},
	}

	RunM68KTests(t, tests)
}

// TestSbcdMemorySystematic tests SBCD -(Ax),-(Ay) - subtract BCD in memory
func TestSbcdMemorySystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "SBCD_memory_simple",
			AddrRegs: [8]uint32{0x1001, 0x2001, 0, 0, 0, 0, 0, 0x8000},
			InitialMem: map[uint32]interface{}{
				0x1000: uint8(0x01), // Source (subtrahend)
				0x2000: uint8(0x10), // Destination (minuend)
			},
			// SBCD -(A0),-(A1): 1000 001 10000 1000 = 0x8308
			Opcodes:      []uint16{0x8308},
			ExpectedRegs: Regs("A0", uint32(0x1000), "A1", uint32(0x2000)),
			ExpectedMem:  []MemoryExpectation{ExpectByte(0x2000, 0x09)},
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// NBCD (Negate BCD) Tests
// =============================================================================

// TestNbcdRegisterSystematic tests NBCD Dn - negate BCD value in data register
func TestNbcdRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "NBCD_D0_simple",
			DataRegs: [8]uint32{0x23, 0, 0, 0, 0, 0, 0, 0}, // -23 BCD = 77
			// NBCD D0: 0100 1000 0000 0000 = 0x4800
			Opcodes:       []uint16{0x4800},
			ExpectedRegs:  Reg("D0", 0x77),
			ExpectedFlags: FlagsAll(-1, 0, -1, 1, 1), // Non-zero result sets C,X
		},
		{
			Name:     "NBCD_D0_zero",
			DataRegs: [8]uint32{0x00, 0, 0, 0, 0, 0, 0, 0},
			// NBCD D0: 0x4800
			Opcodes:       []uint16{0x4800},
			ExpectedRegs:  Reg("D0", 0x00),
			ExpectedFlags: FlagsAll(-1, -1, -1, 0, 0), // Zero result clears C,X
		},
		{
			Name:     "NBCD_D0_99",
			DataRegs: [8]uint32{0x99, 0, 0, 0, 0, 0, 0, 0}, // -99 BCD = 01
			// NBCD D0: 0x4800
			Opcodes:       []uint16{0x4800},
			ExpectedRegs:  Reg("D0", 0x01),
			ExpectedFlags: FlagsAll(-1, 0, -1, 1, 1),
		},
		{
			Name:     "NBCD_D1_with_X_flag",
			DataRegs: [8]uint32{0, 0x50, 0, 0, 0, 0, 0, 0}, // 50 with X=1 -> 49
			SR:       M68K_SR_X,
			// NBCD D1: 0100 1000 0000 0001 = 0x4801
			Opcodes:       []uint16{0x4801},
			ExpectedRegs:  Reg("D1", 0x49),
			ExpectedFlags: FlagsAll(-1, 0, -1, 1, 1),
		},
	}

	RunM68KTests(t, tests)
}

// TestNbcdMemorySystematic tests NBCD <ea> - negate BCD in memory
func TestNbcdMemorySystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "NBCD_memory",
			AddrRegs: [8]uint32{0x1000, 0, 0, 0, 0, 0, 0, 0x8000},
			InitialMem: map[uint32]interface{}{
				0x1000: uint8(0x23),
			},
			// NBCD (A0): 0100 1000 0001 0000 = 0x4810
			Opcodes:       []uint16{0x4810},
			ExpectedMem:   []MemoryExpectation{ExpectByte(0x1000, 0x77)},
			ExpectedFlags: FlagsAll(-1, 0, -1, 1, 1), // Non-zero result, borrow
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// PACK (Pack BCD to Binary) Tests - 68020+
// =============================================================================

// TestPackRegisterSystematic tests PACK Dx,Dy,#<adjustment>
func TestPackRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "PACK_D0_D1_no_adjust",
			DataRegs: [8]uint32{0x0302, 0, 0, 0, 0, 0, 0, 0}, // Unpack source: 03 02 -> 32
			// PACK D0,D1,#0: 1000 001 10100 0000 = 0x8140, adjustment word
			Opcodes:      []uint16{0x8340, 0x0000}, // adjustment = 0
			ExpectedRegs: Reg("D1", 0x32),
		},
		{
			Name:     "PACK_D0_D1_with_adjust",
			DataRegs: [8]uint32{0x0302, 0, 0, 0, 0, 0, 0, 0}, // 03 02 -> 32 + 0x10 = 42
			// PACK D0,D1,#$10
			Opcodes:      []uint16{0x8340, 0x0010}, // adjustment = $10
			ExpectedRegs: Reg("D1", 0x42),
		},
		{
			Name:     "PACK_D2_D3_ascii_to_bcd",
			DataRegs: [8]uint32{0, 0, 0x3735, 0, 0, 0, 0, 0}, // ASCII "75" -> BCD 75
			// PACK D2,D3,#-$30: extracts low byte nibbles and packs
			// High nibble: (0x3735 >> 8) & 0xF = 0x07
			// Low nibble: 0x3735 & 0xF = 0x05
			// Packed: 0x75, then add adjustment -$30 = 0x45
			// PACK D2,D3: 1000 011 10100 0 010 = 0x8742
			Opcodes:      []uint16{0x8742, 0xFFD0}, // adjustment = -$30 = $FFD0
			ExpectedRegs: Reg("D3", 0x45),          // 0x75 + 0xFFD0 = 0x45 (byte)
		},
	}

	RunM68KTests(t, tests)
}

// TestPackMemorySystematic tests PACK -(Ax),-(Ay),#<adjustment>
func TestPackMemorySystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "PACK_memory_no_adjust",
			AddrRegs: [8]uint32{0x1002, 0x2001, 0, 0, 0, 0, 0, 0x8000}, // A0 past word, A1 past byte
			InitialMem: map[uint32]interface{}{
				0x1000: uint16(0x0905), // Source word
			},
			// PACK -(A0),-(A1),#0: 1000 001 10100 1000 = 0x8348
			Opcodes:      []uint16{0x8348, 0x0000},
			ExpectedRegs: Regs("A0", uint32(0x1000), "A1", uint32(0x2000)),
			ExpectedMem:  []MemoryExpectation{ExpectByte(0x2000, 0x95)},
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// UNPK (Unpack Binary to BCD) Tests - 68020+
// =============================================================================

// TestUnpkRegisterSystematic tests UNPK Dx,Dy,#<adjustment>
func TestUnpkRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "UNPK_D0_D1_no_adjust",
			DataRegs: [8]uint32{0x32, 0, 0, 0, 0, 0, 0, 0}, // Pack source: 32 -> 03 02
			// UNPK D0,D1,#0: 1000 001 11000 0000 = 0x8380
			Opcodes:      []uint16{0x8380, 0x0000}, // adjustment = 0
			ExpectedRegs: Reg("D1", 0x0302),
		},
		{
			Name:     "UNPK_D0_D1_with_adjust",
			DataRegs: [8]uint32{0x42, 0, 0, 0, 0, 0, 0, 0}, // 42 -> 04 02 + $3030 = $3432
			// UNPK D0,D1,#$3030 - adds $30 to each nibble for ASCII
			Opcodes:      []uint16{0x8380, 0x3030},
			ExpectedRegs: Reg("D1", 0x3432), // "42" in ASCII
		},
		{
			Name:     "UNPK_D2_D3_bcd_to_ascii",
			DataRegs: [8]uint32{0, 0, 0x75, 0, 0, 0, 0, 0}, // BCD 75 -> ASCII "75"
			// UNPK D2,D3,#$3030: 1000 011 11000 0 010 = 0x8782
			Opcodes:      []uint16{0x8782, 0x3030},
			ExpectedRegs: Reg("D3", 0x3735), // "75" in ASCII
		},
	}

	RunM68KTests(t, tests)
}

// TestUnpkMemorySystematic tests UNPK -(Ax),-(Ay),#<adjustment>
func TestUnpkMemorySystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "UNPK_memory_no_adjust",
			AddrRegs: [8]uint32{0x1001, 0x2002, 0, 0, 0, 0, 0, 0x8000}, // A0 past byte, A1 past word
			InitialMem: map[uint32]interface{}{
				0x1000: uint8(0x32), // Source byte
			},
			// UNPK -(A0),-(A1),#0: 1000 001 11000 1000 = 0x8388
			Opcodes:      []uint16{0x8388, 0x0000},
			ExpectedRegs: Regs("A0", uint32(0x1000), "A1", uint32(0x2000)),
			ExpectedMem:  []MemoryExpectation{ExpectWord(0x2000, 0x0302)},
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// BCD Multi-Precision Chain Tests
// =============================================================================

// TestBcdMultiPrecisionChain tests chained BCD operations for multi-byte arithmetic
func TestBcdMultiPrecisionChain(t *testing.T) {
	// This test simulates adding two 2-digit BCD numbers using ABCD chain
	cpu := setupTestCPU()

	// First add: 99 + 01 = 00 with carry
	cpu.DataRegs[0] = 0x99
	cpu.DataRegs[1] = 0x01
	cpu.SR &= ^uint16(M68K_SR_X) // Clear X initially

	// ABCD D0,D1
	cpu.Write16(cpu.PC, 0xC300)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 0x00 {
		t.Errorf("First ABCD: D1 = 0x%02X, expected 0x00", cpu.DataRegs[1])
	}
	if (cpu.SR & M68K_SR_X) == 0 {
		t.Error("First ABCD: X flag not set (should indicate carry)")
	}

	// Second add with extend: 00 + 00 + X = 01
	cpu.DataRegs[0] = 0x00
	cpu.DataRegs[2] = 0x00

	// ABCD D0,D2
	cpu.Write16(cpu.PC, 0xC500)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[2] != 0x01 {
		t.Errorf("Second ABCD (with X): D2 = 0x%02X, expected 0x01", cpu.DataRegs[2])
	}
}
