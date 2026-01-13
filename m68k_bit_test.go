//go:build m68k

package main

import (
	"testing"
)

// ============================================================================
// BTST (Bit Test) Tests
// ============================================================================

func TestBtstRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BTST_D1_D0_bit0_set",
			DataRegs:      [8]uint32{0x00000001, 0x00000000}, // D0=1, D1=0 (test bit 0)
			Opcodes:       []uint16{0x0300},                  // BTST D1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 because bit is set
		},
		{
			Name:          "BTST_D1_D0_bit0_clear",
			DataRegs:      [8]uint32{0x00000000, 0x00000000}, // D0=0, D1=0 (test bit 0)
			Opcodes:       []uint16{0x0300},                  // BTST D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZ(-1, 1), // Z=1 because bit is clear
		},
		{
			Name:          "BTST_D1_D0_bit31",
			DataRegs:      [8]uint32{0x80000000, 0x0000001F}, // D0=0x80000000, D1=31 (test bit 31)
			Opcodes:       []uint16{0x0300},                  // BTST D1,D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 because bit 31 is set
		},
		{
			Name:          "BTST_D1_D0_bit_modulo",
			DataRegs:      [8]uint32{0x00000001, 0x00000020}, // D0=1, D1=32 (test bit 32 % 32 = 0)
			Opcodes:       []uint16{0x0300},                  // BTST D1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 because bit 0 is set
		},
	}

	RunM68KTests(t, tests)
}

func TestBtstImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BTST_#0_D0_bit0_set",
			DataRegs:      [8]uint32{0x00000001},
			Opcodes:       []uint16{0x0800, 0x0000}, // BTST #0,D0
			ExpectedRegs:  Reg("D0", 0x00000001),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 because bit is set
		},
		{
			Name:          "BTST_#7_D0_bit7_clear",
			DataRegs:      [8]uint32{0x0000007F},
			Opcodes:       []uint16{0x0800, 0x0007}, // BTST #7,D0
			ExpectedRegs:  Reg("D0", 0x0000007F),
			ExpectedFlags: FlagsNZ(-1, 1), // Z=1 because bit 7 is clear
		},
		{
			Name:          "BTST_#15_D0_bit15_set",
			DataRegs:      [8]uint32{0x00008000},
			Opcodes:       []uint16{0x0800, 0x000F}, // BTST #15,D0
			ExpectedRegs:  Reg("D0", 0x00008000),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 because bit 15 is set
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// BCHG (Bit Change) Tests
// ============================================================================

func TestBchgRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BCHG_D1_D0_toggle_bit0",
			DataRegs:      [8]uint32{0x00000001, 0x00000000}, // D0=1, D1=0 (toggle bit 0)
			Opcodes:       []uint16{0x0340},                  // BCHG D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),             // Bit 0 toggled to 0
			ExpectedFlags: FlagsNZ(-1, 0),                    // Z=0 (original bit was set)
		},
		{
			Name:          "BCHG_D1_D0_toggle_bit0_to_1",
			DataRegs:      [8]uint32{0x00000000, 0x00000000}, // D0=0, D1=0 (toggle bit 0)
			Opcodes:       []uint16{0x0340},                  // BCHG D1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),             // Bit 0 toggled to 1
			ExpectedFlags: FlagsNZ(-1, 1),                    // Z=1 (original bit was clear)
		},
		{
			Name:          "BCHG_D1_D0_toggle_bit31",
			DataRegs:      [8]uint32{0x00000000, 0x0000001F}, // D0=0, D1=31 (toggle bit 31)
			Opcodes:       []uint16{0x0340},                  // BCHG D1,D0
			ExpectedRegs:  Reg("D0", 0x80000000),             // Bit 31 toggled to 1
			ExpectedFlags: FlagsNZ(-1, 1),                    // Z=1 (original bit was clear)
		},
	}

	RunM68KTests(t, tests)
}

func TestBchgImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BCHG_#0_D0_toggle_bit0",
			DataRegs:      [8]uint32{0x00000001},
			Opcodes:       []uint16{0x0840, 0x0000}, // BCHG #0,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 (original bit was set)
		},
		{
			Name:          "BCHG_#8_D0_toggle_bit8",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x0840, 0x0008}, // BCHG #8,D0
			ExpectedRegs:  Reg("D0", 0x00000100),
			ExpectedFlags: FlagsNZ(-1, 1), // Z=1 (original bit was clear)
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// BCLR (Bit Clear) Tests
// ============================================================================

func TestBclrRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BCLR_D1_D0_clear_bit0",
			DataRegs:      [8]uint32{0x00000001, 0x00000000}, // D0=1, D1=0 (clear bit 0)
			Opcodes:       []uint16{0x0380},                  // BCLR D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),             // Bit 0 cleared
			ExpectedFlags: FlagsNZ(-1, 0),                    // Z=0 (original bit was set)
		},
		{
			Name:          "BCLR_D1_D0_clear_already_clear",
			DataRegs:      [8]uint32{0x00000000, 0x00000000}, // D0=0, D1=0 (clear bit 0)
			Opcodes:       []uint16{0x0380},                  // BCLR D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),             // Bit 0 still clear
			ExpectedFlags: FlagsNZ(-1, 1),                    // Z=1 (original bit was clear)
		},
		{
			Name:          "BCLR_D1_D0_clear_bit31",
			DataRegs:      [8]uint32{0x80000000, 0x0000001F}, // D0=0x80000000, D1=31 (clear bit 31)
			Opcodes:       []uint16{0x0380},                  // BCLR D1,D0
			ExpectedRegs:  Reg("D0", 0x00000000),             // Bit 31 cleared
			ExpectedFlags: FlagsNZ(-1, 0),                    // Z=0 (original bit was set)
		},
	}

	RunM68KTests(t, tests)
}

func TestBclrImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BCLR_#0_D0_clear_bit0",
			DataRegs:      [8]uint32{0x00000001},
			Opcodes:       []uint16{0x0880, 0x0000}, // BCLR #0,D0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 (original bit was set)
		},
		{
			Name:          "BCLR_#4_D0_clear_bit4",
			DataRegs:      [8]uint32{0x000000FF},
			Opcodes:       []uint16{0x0880, 0x0004}, // BCLR #4,D0
			ExpectedRegs:  Reg("D0", 0x000000EF),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 (original bit was set)
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// BSET (Bit Set) Tests
// ============================================================================

func TestBsetRegisterSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BSET_D1_D0_set_bit0",
			DataRegs:      [8]uint32{0x00000000, 0x00000000}, // D0=0, D1=0 (set bit 0)
			Opcodes:       []uint16{0x03C0},                  // BSET D1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),             // Bit 0 set
			ExpectedFlags: FlagsNZ(-1, 1),                    // Z=1 (original bit was clear)
		},
		{
			Name:          "BSET_D1_D0_set_already_set",
			DataRegs:      [8]uint32{0x00000001, 0x00000000}, // D0=1, D1=0 (set bit 0)
			Opcodes:       []uint16{0x03C0},                  // BSET D1,D0
			ExpectedRegs:  Reg("D0", 0x00000001),             // Bit 0 still set
			ExpectedFlags: FlagsNZ(-1, 0),                    // Z=0 (original bit was set)
		},
		{
			Name:          "BSET_D1_D0_set_bit31",
			DataRegs:      [8]uint32{0x00000000, 0x0000001F}, // D0=0, D1=31 (set bit 31)
			Opcodes:       []uint16{0x03C0},                  // BSET D1,D0
			ExpectedRegs:  Reg("D0", 0x80000000),             // Bit 31 set
			ExpectedFlags: FlagsNZ(-1, 1),                    // Z=1 (original bit was clear)
		},
	}

	RunM68KTests(t, tests)
}

func TestBsetImmediateSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BSET_#0_D0_set_bit0",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x08C0, 0x0000}, // BSET #0,D0
			ExpectedRegs:  Reg("D0", 0x00000001),
			ExpectedFlags: FlagsNZ(-1, 1), // Z=1 (original bit was clear)
		},
		{
			Name:          "BSET_#8_D0_set_bit8",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0x08C0, 0x0008}, // BSET #8,D0
			ExpectedRegs:  Reg("D0", 0x00000100),
			ExpectedFlags: FlagsNZ(-1, 1), // Z=1 (original bit was clear)
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// Bit Field Operations (68020+) Tests
// ============================================================================

func TestBftstSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFTST_D0_offset0_width8_all_zero",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0xE8C0, 0x0008}, // BFTST D0{0:8} - width=8 in bits 4-0
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1 (field is zero)
		},
		{
			Name:          "BFTST_D0_offset0_width8_all_ones",
			DataRegs:      [8]uint32{0xFF000000},
			Opcodes:       []uint16{0xE8C0, 0x0008}, // BFTST D0{0:8}
			ExpectedRegs:  Reg("D0", 0xFF000000),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1 (MSB of field is set)
		},
		{
			Name:          "BFTST_D0_offset8_width8_nonzero",
			DataRegs:      [8]uint32{0x00FF0000},
			Opcodes:       []uint16{0xE8C0, 0x0208}, // BFTST D0{8:8} - offset=8 in bits 10-6, width=8 in bits 4-0
			ExpectedRegs:  Reg("D0", 0x00FF0000),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1
		},
	}

	RunM68KTests(t, tests)
}

func TestBfchgSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFCHG_D0_offset0_width8_toggle",
			DataRegs:      [8]uint32{0xFF000000},
			Opcodes:       []uint16{0xEAC0, 0x0008}, // BFCHG D0{0:8}
			ExpectedRegs:  Reg("D0", 0x00000000),    // Upper 8 bits toggled
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1 (original field had MSB set)
		},
		{
			Name:          "BFCHG_D0_offset8_width8_toggle",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0xEAC0, 0x0208}, // BFCHG D0{8:8}
			ExpectedRegs:  Reg("D0", 0x00FF0000),    // Middle 8 bits toggled to 1
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),    // Z=1 (original field was zero)
		},
	}

	RunM68KTests(t, tests)
}

func TestBfclrSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFCLR_D0_offset0_width8_clear",
			DataRegs:      [8]uint32{0xFF000000},
			Opcodes:       []uint16{0xECC0, 0x0008}, // BFCLR D0{0:8}
			ExpectedRegs:  Reg("D0", 0x00000000),    // Upper 8 bits cleared
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1 (original field had MSB set)
		},
		{
			Name:          "BFCLR_D0_offset16_width8_clear",
			DataRegs:      [8]uint32{0x0000FF00},
			Opcodes:       []uint16{0xECC0, 0x0408}, // BFCLR D0{16:8}
			ExpectedRegs:  Reg("D0", 0x00000000),    // Bits 16-23 cleared
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1
		},
	}

	RunM68KTests(t, tests)
}

func TestBfsetSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFSET_D0_offset0_width8_set",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0xEEC0, 0x0008}, // BFSET D0{0:8}
			ExpectedRegs:  Reg("D0", 0xFF000000),    // Upper 8 bits set
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),    // Z=1 (original field was zero)
		},
		{
			Name:          "BFSET_D0_offset24_width8_set",
			DataRegs:      [8]uint32{0x00000000},
			Opcodes:       []uint16{0xEEC0, 0x0608}, // BFSET D0{24:8}
			ExpectedRegs:  Reg("D0", 0x000000FF),    // Lower 8 bits set
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),    // Z=1
		},
	}

	RunM68KTests(t, tests)
}

func TestBfextuSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFEXTU_D0_offset0_width8_to_D1",
			DataRegs:      [8]uint32{0xFF000000, 0x00000000},
			Opcodes:       []uint16{0xE9C0, 0x1008}, // BFEXTU D0{0:8},D1
			ExpectedRegs:  Reg("D1", 0x000000FF),    // Extracted unsigned
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1 (field MSB set)
		},
		{
			Name:          "BFEXTU_D0_offset0_width4_to_D2",
			DataRegs:      [8]uint32{0x80000000, 0x00000000, 0x00000000},
			Opcodes:       []uint16{0xE9C0, 0x2004}, // BFEXTU D0{0:4},D2
			ExpectedRegs:  Reg("D2", 0x00000008),    // Extracted 0x8 (high 4 bits)
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1
		},
	}

	RunM68KTests(t, tests)
}

func TestBfextsSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFEXTS_D0_offset0_width8_to_D1",
			DataRegs:      [8]uint32{0xFF000000, 0x00000000},
			Opcodes:       []uint16{0xEBC0, 0x1008}, // BFEXTS D0{0:8},D1
			ExpectedRegs:  Reg("D1", 0xFFFFFFFF),    // Sign-extended to -1
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1
		},
		{
			Name:          "BFEXTS_D0_offset0_width8_positive",
			DataRegs:      [8]uint32{0x7F000000, 0x00000000},
			Opcodes:       []uint16{0xEBC0, 0x1008}, // BFEXTS D0{0:8},D1
			ExpectedRegs:  Reg("D1", 0x0000007F),    // No sign extension (positive)
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),    // N=0
		},
	}

	RunM68KTests(t, tests)
}

func TestBfffoSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFFFO_D0_offset0_width8_first_at_0",
			DataRegs:      [8]uint32{0x80000000, 0x00000000},
			Opcodes:       []uint16{0xEDC0, 0x1008}, // BFFFO D0{0:8},D1
			ExpectedRegs:  Reg("D1", 0x00000000),    // First 1-bit at offset 0
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1
		},
		{
			Name:          "BFFFO_D0_offset0_width8_first_at_7",
			DataRegs:      [8]uint32{0x01000000, 0x00000000},
			Opcodes:       []uint16{0xEDC0, 0x1008}, // BFFFO D0{0:8},D1
			ExpectedRegs:  Reg("D1", 0x00000007),    // First 1-bit at offset 7
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),    // N=0
		},
		{
			Name:          "BFFFO_D0_offset0_width8_all_zero",
			DataRegs:      [8]uint32{0x00000000, 0x00000000},
			Opcodes:       []uint16{0xEDC0, 0x1008}, // BFFFO D0{0:8},D1
			ExpectedRegs:  Reg("D1", 0x00000008),    // No 1-bit found, returns offset+width
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),    // Z=1 (field is zero)
		},
	}

	RunM68KTests(t, tests)
}

func TestBfinsSystematic(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:          "BFINS_D1_D0_offset0_width8",
			DataRegs:      [8]uint32{0x00000000, 0x000000FF},
			Opcodes:       []uint16{0xEFC0, 0x1008}, // BFINS D1,D0{0:8}
			ExpectedRegs:  Reg("D0", 0xFF000000),    // D1's low 8 bits inserted at offset 0
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1 (inserted field has MSB set)
		},
		{
			Name:          "BFINS_D1_D0_offset24_width8",
			DataRegs:      [8]uint32{0x00000000, 0x000000AA},
			Opcodes:       []uint16{0xEFC0, 0x1608}, // BFINS D1,D0{24:8}
			ExpectedRegs:  Reg("D0", 0x000000AA),    // D1's low 8 bits inserted at offset 24
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),    // N=1
		},
		{
			Name:          "BFINS_D1_D0_offset8_width16",
			DataRegs:      [8]uint32{0xFFFF0000, 0x00005678},
			Opcodes:       []uint16{0xEFC0, 0x1210}, // BFINS D1,D0{8:16}
			ExpectedRegs:  Reg("D0", 0xFF567800),    // 16 bits inserted at offset 8
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),    // N=0
		},
	}

	RunM68KTests(t, tests)
}

// ============================================================================
// Edge Cases and Special Scenarios
// ============================================================================

func TestBitEdgeCases(t *testing.T) {
	tests := []M68KTestCase{
		// Bit operations with byte memory accesses use modulo 8
		// Register accesses use modulo 32
		{
			Name:          "BTST_bit63_modulo32",
			DataRegs:      [8]uint32{0x80000000, 0x0000003F}, // D0=0x80000000, D1=63 (63 % 32 = 31)
			Opcodes:       []uint16{0x0300},                  // BTST D1,D0
			ExpectedRegs:  Reg("D0", 0x80000000),
			ExpectedFlags: FlagsNZ(-1, 0), // Z=0 (bit 31 is set)
		},
	}

	RunM68KTests(t, tests)
}
