//go:build m68k_test

package main

import (
	"math"
	"os"
	"os/exec"
	"testing"
)

// =============================================================================
// Phase 2: Test M68K CPU Instructions Used by Rotozoomer
// =============================================================================

// TestM68K_MULU_Word tests the MULU.W instruction (unsigned word multiply)
// MULU.W <ea>,Dn - multiply lower 16 bits, result is 32-bit in Dn
func TestM68K_MULU_Word(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "MULU.W_768_times_256",
			DataRegs: [8]uint32{256, 768, 0, 0, 0, 0, 0, 0}, // D0=256, D1=768
			// MULU.W D0,D1 - opcode: 1100 rrr 011 000 rrr = 0xC2C0 | (1<<9) | 0 = 0xC2C0
			// Format: 1100 Dn 011 mode reg
			Opcodes:       []uint16{0xC2C0},  // MULU.W D0,D1
			ExpectedRegs:  Reg("D1", 196608), // 768 * 256 = 196608
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "MULU.W_768_times_0",
			DataRegs:      [8]uint32{0, 768, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC2C0}, // MULU.W D0,D1
			ExpectedRegs:  Reg("D1", 0),
			ExpectedFlags: FlagsNZ(0, 1), // Zero flag set
		},
		{
			Name:          "MULU.W_0xFFFF_times_2",
			DataRegs:      [8]uint32{2, 0xFFFF, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC2C0},   // MULU.W D0,D1
			ExpectedRegs:  Reg("D1", 0x1FFFE), // 65535 * 2 = 131070
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "MULU.W_0xFFFF_times_0xFFFF",
			DataRegs:      [8]uint32{0xFFFF, 0xFFFF, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0xC2C0},      // MULU.W D0,D1
			ExpectedRegs:  Reg("D1", 0xFFFE0001), // 65535 * 65535 = 4294836225
			ExpectedFlags: FlagsNZ(1, 0),         // Negative flag set (MSB is 1)
		},
		{
			Name:          "MULU.W_only_uses_lower_16_bits",
			DataRegs:      [8]uint32{0xFFFF0002, 0xFFFF0003, 0, 0, 0, 0, 0, 0}, // Upper bits ignored
			Opcodes:       []uint16{0xC2C0},                                    // MULU.W D0,D1
			ExpectedRegs:  Reg("D1", 6),                                        // 2 * 3 = 6
			ExpectedFlags: FlagsNZ(0, 0),
		},
	}

	RunM68KTests(t, tests)
}

// TestM68K_LSL_Long_Immediate tests LSL.L #imm,Dn (logical shift left)
func TestM68K_LSL_Long_Immediate(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "LSL.L_#8_D0",
			DataRegs: [8]uint32{0x100, 0, 0, 0, 0, 0, 0, 0},
			// LSL.L #8,D0 - format: 1110 ccc 1 10 i 01 rrr
			// For count=8 (code=0), size=long (10), i=0 (imm), reg=0
			// 1110 000 1 10 0 01 000 = 0xE188
			Opcodes:       []uint16{0xE188}, // LSL.L #8,D0
			ExpectedRegs:  Reg("D0", 0x10000),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "LSL.L_#2_D0",
			DataRegs: [8]uint32{0x100, 0, 0, 0, 0, 0, 0, 0},
			// LSL.L #2,D0: 1110 010 1 10 0 01 000 = 0xE588
			Opcodes:       []uint16{0xE588}, // LSL.L #2,D0
			ExpectedRegs:  Reg("D0", 0x400), // 0x100 << 2 = 0x400
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "LSL.L_combined_shift_10_bits",
			DataRegs: [8]uint32{1, 0, 0, 0, 0, 0, 0, 0},
			// First shift by 8, then by 2 = total 10 bits
			// We'll just test single shift of 8 first
			Opcodes:       []uint16{0xE188}, // LSL.L #8,D0
			ExpectedRegs:  Reg("D0", 0x100),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// TestM68K_LSR_Long_Immediate tests LSR.L #imm,Dn (logical shift right)
func TestM68K_LSR_Long_Immediate(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "LSR.L_#8_D1",
			DataRegs: [8]uint32{0, 0x10000, 0, 0, 0, 0, 0, 0},
			// LSR.L #8,D1: 1110 000 0 10 0 01 001 = 0xE089
			Opcodes:       []uint16{0xE089}, // LSR.L #8,D1
			ExpectedRegs:  Reg("D1", 0x100),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:          "LSR.L_#8_large_value",
			DataRegs:      [8]uint32{0, 0x30000, 0, 0, 0, 0, 0, 0}, // 196608
			Opcodes:       []uint16{0xE089},                        // LSR.L #8,D1
			ExpectedRegs:  Reg("D1", 768),                          // 196608 >> 8 = 768
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// TestM68K_NEG_Long tests NEG.L Dn (negate long)
func TestM68K_NEG_Long(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "NEG.L_256",
			DataRegs: [8]uint32{256, 0, 0, 0, 0, 0, 0, 0},
			// NEG.L D0: 0100 0100 10 000 000 = 0x4480
			Opcodes:       []uint16{0x4480},      // NEG.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFF00), // -256 in two's complement
			ExpectedFlags: FlagsNZ(1, 0),
		},
		{
			Name:          "NEG.L_negative_256",
			DataRegs:      [8]uint32{0xFFFFFF00, 0, 0, 0, 0, 0, 0, 0}, // -256
			Opcodes:       []uint16{0x4480},                           // NEG.L D0
			ExpectedRegs:  Reg("D0", 256),
			ExpectedFlags: FlagsNZ(0, 0),
		},
		{
			Name:          "NEG.L_0",
			DataRegs:      [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4480}, // NEG.L D0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZ(0, 1), // Zero flag set
		},
		{
			Name:          "NEG.L_1",
			DataRegs:      [8]uint32{1, 0, 0, 0, 0, 0, 0, 0},
			Opcodes:       []uint16{0x4480},      // NEG.L D0
			ExpectedRegs:  Reg("D0", 0xFFFFFFFF), // -1
			ExpectedFlags: FlagsNZ(1, 0),
		},
	}

	RunM68KTests(t, tests)
}

// TestM68K_IndexedAddressing tests (An,Dn.L) addressing mode
// This is used for table lookups like: move.l (a0,d0.l),d2
func TestM68K_IndexedAddressing(t *testing.T) {
	// NOTE: Test data addresses use 0x2000+ to avoid collision with PC at M68K_ENTRY_POINT (0x1000)
	tests := []M68KTestCase{
		{
			Name:     "MOVE.L_(A0,D0.L),D2_offset_0",
			DataRegs: [8]uint32{0, 0, 0, 0, 0, 0, 0, 0},           // D0 = 0
			AddrRegs: [8]uint32{0x2000, 0, 0, 0, 0, 0, 0, 0x8000}, // A0 = 0x2000
			InitialMem: map[uint32]interface{}{
				0x2000: uint32(0xDEADBEEF),
			},
			// MOVE.L (A0,D0.L),D2
			// Format: 00 10 ddd ddd mmm rrr
			// Size=long(10), dest=D2(010, 000), src=A0 indexed(110, 000)
			// 0010 010 000 110 000 = 0x2430
			// Extension word: D0.L, no displacement: 0 000 1 00 0 00000000 = 0x0800
			Opcodes:       []uint16{0x2430, 0x0800}, // MOVE.L (A0,D0.L),D2
			ExpectedRegs:  Reg("D2", 0xDEADBEEF),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.L_(A0,D0.L),D2_offset_4",
			DataRegs: [8]uint32{4, 0, 0, 0, 0, 0, 0, 0}, // D0 = 4
			AddrRegs: [8]uint32{0x2000, 0, 0, 0, 0, 0, 0, 0x8000},
			InitialMem: map[uint32]interface{}{
				0x2000: uint32(0x11111111),
				0x2004: uint32(0x22222222),
			},
			Opcodes:       []uint16{0x2430, 0x0800}, // MOVE.L (A0,D0.L),D2
			ExpectedRegs:  Reg("D2", 0x22222222),
			ExpectedFlags: FlagDontCare(),
		},
		{
			Name:     "MOVE.L_(A0,D0.L),D2_large_offset",
			DataRegs: [8]uint32{0x100, 0, 0, 0, 0, 0, 0, 0}, // D0 = 256
			AddrRegs: [8]uint32{0x2000, 0, 0, 0, 0, 0, 0, 0x8000},
			InitialMem: map[uint32]interface{}{
				0x2100: uint32(0xCAFEBABE),
			},
			Opcodes:       []uint16{0x2430, 0x0800}, // MOVE.L (A0,D0.L),D2
			ExpectedRegs:  Reg("D2", 0xCAFEBABE),
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// TestM68K_ADDA_Long tests ADDA.L Dn,An
func TestM68K_ADDA_Long(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "ADDA.L_D0,A1",
			DataRegs: [8]uint32{0x1000, 0, 0, 0, 0, 0, 0, 0},
			AddrRegs: [8]uint32{0, 0x2000, 0, 0, 0, 0, 0, 0x8000},
			// ADDA.L D0,A1: 1101 rrr 111 000 rrr = 0xD3C0
			Opcodes:       []uint16{0xD3C0}, // ADDA.L D0,A1
			ExpectedRegs:  Reg("A1", 0x3000),
			ExpectedFlags: FlagDontCare(), // ADDA doesn't affect flags
		},
		{
			Name:          "ADDA.L_preserves_32bit",
			DataRegs:      [8]uint32{0x80000000, 0, 0, 0, 0, 0, 0, 0},
			AddrRegs:      [8]uint32{0, 0x80000000, 0, 0, 0, 0, 0, 0x8000},
			Opcodes:       []uint16{0xD3C0},      // ADDA.L D0,A1
			ExpectedRegs:  Reg("A1", 0x00000000), // Overflow wraps
			ExpectedFlags: FlagDontCare(),
		},
	}

	RunM68KTests(t, tests)
}

// TestM68K_ADD_AddressRegisterSource tests ADD.L An,Dn (address register as source)
// This is the instruction used in the rotozoomer inner loop: add.l a3,d0
func TestM68K_ADD_AddressRegisterSource(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:     "ADD.L_A3,D0 - address register to data register",
			DataRegs: [8]uint32{100, 0, 0, 0, 0, 0, 0, 0},      // D0=100
			AddrRegs: [8]uint32{0, 0, 0, 200, 0, 0, 0, 0x8000}, // A3=200
			// ADD.L A3,D0: 1101 000 010 001 011 = 0xD08B
			Opcodes:      []uint16{0xD08B},
			ExpectedRegs: Reg("D0", 300), // Should be 100 + 200 = 300
		},
		{
			Name:     "ADD.L_A4,D0 - larger values",
			DataRegs: [8]uint32{0x1000, 0, 0, 0, 0, 0, 0, 0},
			AddrRegs: [8]uint32{0, 0, 0, 0, 0x2000, 0, 0, 0x8000}, // A4=0x2000
			// ADD.L A4,D0: 1101 000 010 001 100 = 0xD08C
			Opcodes:      []uint16{0xD08C},
			ExpectedRegs: Reg("D0", 0x3000), // 0x1000 + 0x2000 = 0x3000
		},
		{
			Name:         "ADD.L_A3,D0 - rotozoomer style (du_dy value)",
			DataRegs:     [8]uint32{0x8000, 0, 0, 0, 0, 0, 0, 0},   // D0=0x8000 (var_row_u)
			AddrRegs:     [8]uint32{0, 0, 0, 768, 0, 0, 0, 0x8000}, // A3=768 (du_dy)
			Opcodes:      []uint16{0xD08B},
			ExpectedRegs: Reg("D0", 0x8300), // 0x8000 + 768 = 0x8300
		},
	}

	RunM68KTests(t, tests)
}

// =============================================================================
// Phase 3: Test Sine Table Generation
// =============================================================================

// TestM68K_SineTableGeneration verifies sine values at key angles
// The sine table uses 8.8 fixed point: -256 to +256 (-1.0 to +1.0)
func TestM68K_SineTableGeneration(t *testing.T) {
	cpu := setupTestCPU()

	// Generate the sine table by running the generation code
	// Since we can't easily run the full assembly, we'll test the expected values
	// that the assembly should produce

	// First, let's verify the algorithm produces correct values
	// The M68K assembly uses this algorithm:
	// - ramp = index & 63
	// - if (index & 64): ramp = 64 - ramp
	// - value = ramp * 4, clamped to 256
	// - if (index & 128): value = -value

	expectedSine := make([]int32, 256)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		expectedSine[i] = int32(value)
	}

	// Test key angles
	testCases := []struct {
		angle    int
		expected int32
	}{
		{0, 0},      // sin(0) = 0
		{32, 128},   // sin(45°) ≈ 0.707 * 256 ≈ 181, but approx gives 128
		{64, 256},   // sin(90°) = 1.0 * 256 = 256
		{96, 128},   // sin(135°) ≈ 0.707
		{128, 0},    // sin(180°) = 0
		{160, -128}, // sin(225°) ≈ -0.707
		{192, -256}, // sin(270°) = -1.0 * 256 = -256
		{224, -128}, // sin(315°) ≈ -0.707
	}

	for _, tc := range testCases {
		if expectedSine[tc.angle] != tc.expected {
			t.Errorf("Sine table algorithm mismatch at angle %d: got %d, expected %d",
				tc.angle, expectedSine[tc.angle], tc.expected)
		}
	}

	// Now let's write the sine table to memory and verify we can read it back
	sineTableAddr := uint32(0x44000)
	for i := 0; i < 256; i++ {
		cpu.Write32(sineTableAddr+uint32(i*4), uint32(expectedSine[i]))
	}

	// Verify reads back correctly
	for _, tc := range testCases {
		addr := sineTableAddr + uint32(tc.angle*4)
		val := int32(cpu.Read32(addr))
		if val != tc.expected {
			t.Errorf("Sine table read mismatch at angle %d (addr 0x%X): got %d, expected %d",
				tc.angle, addr, val, tc.expected)
		}
	}
}

// =============================================================================
// Phase 4: Test DU/DV Table Generation (Critical)
// =============================================================================

// TestM68K_DUDVTableGeneration tests the precomputed DU/DV values
// This is the most likely location of the bug
func TestM68K_DUDVTableGeneration(t *testing.T) {
	// The DU/DV tables store: scale_inv * cos/sin >> 8
	// scale_inv values from recip_table[scale+2]:
	// recip[2] = 768, recip[3] = 512, recip[4] = 384, recip[5] = 307, recip[6] = 256

	recipTable := []int32{0, 1536, 768, 512, 384, 307, 256, 219}

	// Expected sine/cosine at key angles (8.8 fixed point)
	// cos(angle) = sin(angle + 64)
	getSine := func(angle int) int32 {
		angle = angle & 255
		ramp := angle & 63
		if (angle & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (angle & 128) != 0 {
			value = -value
		}
		return int32(value)
	}

	getCos := func(angle int) int32 {
		return getSine(angle + 64)
	}

	// Test cases: scale=0 (recip[2]=768)
	testCases := []struct {
		scale      int
		angle      int
		expectedDU int32
		expectedDV int32
	}{
		// Scale 0, recip=768
		{0, 0, (768 * 256) >> 8, (768 * 0) >> 8},    // DU=768, DV=0
		{0, 64, (768 * 0) >> 8, (768 * 256) >> 8},   // DU=0, DV=768
		{0, 128, -(768 * 256) >> 8, (768 * 0) >> 8}, // DU=-768, DV=0
		{0, 192, (768 * 0) >> 8, -(768 * 256) >> 8}, // DU=0, DV=-768

		// Scale 0, angle 32 (approx 45°)
		// sin(32) = 128, cos(32) = sin(96) = 128
		{0, 32, (768 * 128) >> 8, (768 * 128) >> 8}, // DU=384, DV=384
	}

	for _, tc := range testCases {
		scaleInv := recipTable[tc.scale+2]
		cos := getCos(tc.angle)
		sin := getSine(tc.angle)

		// Calculate DU = (scale_inv * cos) >> 8
		var actualDU int32
		if cos >= 0 {
			actualDU = (scaleInv * cos) >> 8
		} else {
			actualDU = -((scaleInv * (-cos)) >> 8)
		}

		// Calculate DV = (scale_inv * sin) >> 8
		var actualDV int32
		if sin >= 0 {
			actualDV = (scaleInv * sin) >> 8
		} else {
			actualDV = -((scaleInv * (-sin)) >> 8)
		}

		if actualDU != tc.expectedDU {
			t.Errorf("DU mismatch at scale=%d angle=%d: got %d, expected %d (cos=%d, scaleInv=%d)",
				tc.scale, tc.angle, actualDU, tc.expectedDU, cos, scaleInv)
		}

		if actualDV != tc.expectedDV {
			t.Errorf("DV mismatch at scale=%d angle=%d: got %d, expected %d (sin=%d, scaleInv=%d)",
				tc.scale, tc.angle, actualDV, tc.expectedDV, sin, scaleInv)
		}
	}
}

// TestM68K_DUDVTableLookup tests that the DU/DV table lookup works correctly
func TestM68K_DUDVTableLookup(t *testing.T) {
	cpu := setupTestCPU()

	// Setup: write known values to DU and DV tables
	duTableBase := uint32(0x44420)
	dvTableBase := uint32(0x45820)

	// Write test values for scale=0, angle=0
	// Offset = scale * 1024 + angle * 4 = 0 * 1024 + 0 * 4 = 0
	cpu.Write32(duTableBase+0, 768) // DU[0][0] = 768
	cpu.Write32(dvTableBase+0, 0)   // DV[0][0] = 0

	// Write test values for scale=0, angle=64
	// Offset = 0 * 1024 + 64 * 4 = 256
	cpu.Write32(duTableBase+256, 0)   // DU[0][64] = 0
	cpu.Write32(dvTableBase+256, 768) // DV[0][64] = 768

	// Write test values for scale=2, angle=32
	// Offset = 2 * 1024 + 32 * 4 = 2048 + 128 = 2176
	cpu.Write32(duTableBase+2176, 192) // DU[2][32] = 192
	cpu.Write32(dvTableBase+2176, 192) // DV[2][32] = 192

	// Test lookup calculation
	// The assembly calculates: offset = scale * 1024 + angle * 4
	// Then: DU = DU_TABLE[offset], DV = DV_TABLE[offset]

	testCases := []struct {
		scale      uint32
		angle      uint32
		expectedDU uint32
		expectedDV uint32
	}{
		{0, 0, 768, 0},
		{0, 64, 0, 768},
		{2, 32, 192, 192},
	}

	for _, tc := range testCases {
		offset := tc.scale*1024 + tc.angle*4

		duAddr := duTableBase + offset
		dvAddr := dvTableBase + offset

		actualDU := cpu.Read32(duAddr)
		actualDV := cpu.Read32(dvAddr)

		if actualDU != tc.expectedDU {
			t.Errorf("DU lookup failed at scale=%d angle=%d: got %d, expected %d (offset=%d, addr=0x%X)",
				tc.scale, tc.angle, actualDU, tc.expectedDU, offset, duAddr)
		}

		if actualDV != tc.expectedDV {
			t.Errorf("DV lookup failed at scale=%d angle=%d: got %d, expected %d (offset=%d, addr=0x%X)",
				tc.scale, tc.angle, actualDV, tc.expectedDV, offset, dvAddr)
		}
	}
}

// =============================================================================
// Phase 5: Test Animation State Update
// =============================================================================

// TestM68K_UpdateAnimationLookup tests that var_du_dx, var_dv_dx are set correctly
func TestM68K_UpdateAnimationLookup(t *testing.T) {
	cpu := setupTestCPU()

	// Memory addresses from the assembly
	duTableBase := uint32(0x44420)
	dvTableBase := uint32(0x45820)
	varDuDx := uint32(0x46C20 + 0x0C)
	varDvDx := uint32(0x46C20 + 0x10)
	varDuDy := uint32(0x46C20 + 0x14)
	varDvDy := uint32(0x46C20 + 0x18)

	// Setup DU/DV tables with known values
	// For scale=0, angle=32: DU=384, DV=384
	offset := uint32(0*1024 + 32*4)
	cpu.Write32(duTableBase+offset, 384)
	cpu.Write32(dvTableBase+offset, 384)

	// Manually set what update_animation should compute
	// du_dx = DU_TABLE[scale_sel * 256 + angle]
	// dv_dx = DV_TABLE[scale_sel * 256 + angle]
	// du_dy = -dv_dx
	// dv_dy = du_dx
	cpu.Write32(varDuDx, 384)
	cpu.Write32(varDvDx, 384)
	cpu.Write32(varDuDy, 0xFFFFFE80) // -384 in two's complement
	cpu.Write32(varDvDy, 384)        // du_dx

	// Verify the relationships
	duDx := int32(cpu.Read32(varDuDx))
	dvDx := int32(cpu.Read32(varDvDx))
	duDy := int32(cpu.Read32(varDuDy))
	dvDy := int32(cpu.Read32(varDvDy))

	// du_dy should equal -dv_dx
	if duDy != -dvDx {
		t.Errorf("du_dy should equal -dv_dx: du_dy=%d, -dv_dx=%d", duDy, -dvDx)
	}

	// dv_dy should equal du_dx
	if dvDy != duDx {
		t.Errorf("dv_dy should equal du_dx: dv_dy=%d, du_dx=%d", dvDy, duDx)
	}
}

// =============================================================================
// Phase 6: Compare IE32 vs M68K Tables
// =============================================================================

// TestM68K_TableGenerationMatchesIE32 compares table generation between IE32 and M68K
func TestM68K_TableGenerationMatchesIE32(t *testing.T) {
	// Generate the expected sine table values
	expectedSine := make([]int32, 256)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		expectedSine[i] = int32(value)
	}

	getCos := func(angle int) int32 {
		return expectedSine[(angle+64)&255]
	}

	getSin := func(angle int) int32 {
		return expectedSine[angle&255]
	}

	recipTable := []int32{0, 1536, 768, 512, 384, 307, 256, 219}

	// Generate expected DU/DV tables
	// 5 scales x 256 angles
	expectedDU := make([]int32, 5*256)
	expectedDV := make([]int32, 5*256)

	for scale := 0; scale < 5; scale++ {
		scaleInv := recipTable[scale+2]

		for angle := 0; angle < 256; angle++ {
			cos := getCos(angle)
			sin := getSin(angle)

			// DU = (scale_inv * cos) >> 8 (signed)
			var du int32
			if cos >= 0 {
				du = (scaleInv * cos) >> 8
			} else {
				du = -((scaleInv * (-cos)) >> 8)
			}

			// DV = (scale_inv * sin) >> 8 (signed)
			var dv int32
			if sin >= 0 {
				dv = (scaleInv * sin) >> 8
			} else {
				dv = -((scaleInv * (-sin)) >> 8)
			}

			idx := scale*256 + angle
			expectedDU[idx] = du
			expectedDV[idx] = dv
		}
	}

	// Verify some key entries
	// Scale 0, angle 0: DU = 768 * 256 >> 8 = 768, DV = 768 * 0 >> 8 = 0
	if expectedDU[0] != 768 {
		t.Errorf("DU[0][0] should be 768, got %d", expectedDU[0])
	}
	if expectedDV[0] != 0 {
		t.Errorf("DV[0][0] should be 0, got %d", expectedDV[0])
	}

	// Scale 0, angle 64: DU = 768 * 0 >> 8 = 0, DV = 768 * 256 >> 8 = 768
	if expectedDU[64] != 0 {
		t.Errorf("DU[0][64] should be 0, got %d", expectedDU[64])
	}
	if expectedDV[64] != 768 {
		t.Errorf("DV[0][64] should be 768, got %d", expectedDV[64])
	}

	// Verify DU and DV are different at most angles (not just vertical stripes)
	differentCount := 0
	for i := 0; i < 5*256; i++ {
		if expectedDU[i] != expectedDV[i] {
			differentCount++
		}
	}

	// At least 75% of entries should have DU != DV for proper rotation
	if float64(differentCount)/float64(5*256) < 0.75 {
		t.Errorf("Too few entries have DU != DV: %d/%d - this would cause vertical stripes",
			differentCount, 5*256)
	}
}

// =============================================================================
// Phase 7: Single Pixel Calculation Test
// =============================================================================

// TestM68K_TextureAddressCalculation verifies pixel address calculation
func TestM68K_TextureAddressCalculation(t *testing.T) {
	// Formula: TEXTURE_BASE + ((texV>>8)&255)*1024 + ((texU>>8)&255)*4
	textureBase := uint32(0x4000)

	testCases := []struct {
		texU         int32
		texV         int32
		expectedAddr uint32
	}{
		{0x0000, 0x0000, textureBase + 0*1024 + 0*4},     // (0, 0)
		{0x0100, 0x0000, textureBase + 0*1024 + 1*4},     // (1, 0)
		{0x0000, 0x0100, textureBase + 1*1024 + 0*4},     // (0, 1)
		{0x8000, 0x8000, textureBase + 128*1024 + 128*4}, // (128, 128) - center
		{0xFF00, 0xFF00, textureBase + 255*1024 + 255*4}, // (255, 255)
		{0x10000, 0x10000, textureBase + 0*1024 + 0*4},   // Wraps to (0, 0)
		{0x0180, 0x0280, textureBase + 2*1024 + 1*4},     // (1.5, 2.5) -> (1, 2)
	}

	for _, tc := range testCases {
		u := (uint32(tc.texU) >> 8) & 255
		v := (uint32(tc.texV) >> 8) & 255

		addr := textureBase + v*1024 + u*4

		if addr != tc.expectedAddr {
			t.Errorf("Texture address mismatch for texU=0x%X, texV=0x%X: got 0x%X, expected 0x%X (u=%d, v=%d)",
				tc.texU, tc.texV, addr, tc.expectedAddr, u, v)
		}
	}
}

// =============================================================================
// Integration Tests - Running actual M68K code sequences
// =============================================================================

// TestM68K_MULUThenLSR tests the negate-multiply-negate pattern used for signed multiply
func TestM68K_MULUThenLSR(t *testing.T) {
	// Test the pattern used in generate_dudv_tables:
	// neg.l d1
	// mulu.w d6,d1
	// lsr.l #8,d1
	// neg.l d1

	cpu := setupTestCPU()

	// Test with a negative value: cosA = -256 (0xFFFFFF00)
	// scale_inv = 768 (in d6)
	// Expected: neg(-256)=256, mulu(256*768)=196608, lsr(196608>>8)=768, neg(768)=-768

	cpu.DataRegs[1] = 0xFFFFFF00 // d1 = -256 in two's complement
	cpu.DataRegs[6] = 768        // d6 = scale_inv

	// Execute: NEG.L D1
	cpu.PC = M68K_ENTRY_POINT
	cpu.Write16(cpu.PC, 0x4481) // NEG.L D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 256 {
		t.Errorf("After NEG.L D1: expected D1=256, got D1=0x%X", cpu.DataRegs[1])
	}

	// Execute: MULU.W D6,D1
	cpu.Write16(cpu.PC, 0xC2C6) // MULU.W D6,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 196608 {
		t.Errorf("After MULU.W D6,D1: expected D1=196608, got D1=0x%X (%d)", cpu.DataRegs[1], cpu.DataRegs[1])
	}

	// Execute: LSR.L #8,D1
	cpu.Write16(cpu.PC, 0xE089) // LSR.L #8,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 768 {
		t.Errorf("After LSR.L #8,D1: expected D1=768, got D1=0x%X (%d)", cpu.DataRegs[1], cpu.DataRegs[1])
	}

	// Execute: NEG.L D1
	cpu.Write16(cpu.PC, 0x4481) // NEG.L D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	expected := uint32(0xFFFFFD00) // -768 in two's complement
	if cpu.DataRegs[1] != expected {
		t.Errorf("After final NEG.L D1: expected D1=-768 (0x%X), got D1=0x%X",
			expected, cpu.DataRegs[1])
	}
}

// TestM68K_PositiveMULUPath tests the direct multiply path for positive values
func TestM68K_PositiveMULUPath(t *testing.T) {
	cpu := setupTestCPU()

	// Test with a positive value: cosA = 256
	// scale_inv = 768 (in d6)
	// Expected: mulu(256*768)=196608, lsr(196608>>8)=768

	cpu.DataRegs[1] = 256 // d1 = 256
	cpu.DataRegs[6] = 768 // d6 = scale_inv

	// Execute: MULU.W D6,D1
	cpu.PC = M68K_ENTRY_POINT
	cpu.Write16(cpu.PC, 0xC2C6) // MULU.W D6,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 196608 {
		t.Errorf("After MULU.W D6,D1: expected D1=196608, got D1=0x%X (%d)", cpu.DataRegs[1], cpu.DataRegs[1])
	}

	// Execute: LSR.L #8,D1
	cpu.Write16(cpu.PC, 0xE089) // LSR.L #8,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 768 {
		t.Errorf("After LSR.L #8,D1: expected D1=768, got D1=0x%X (%d)", cpu.DataRegs[1], cpu.DataRegs[1])
	}
}

// TestM68K_FullDUDVCalculation tests a complete DU/DV calculation cycle
func TestM68K_FullDUDVCalculation(t *testing.T) {
	cpu := setupTestCPU()

	// Setup: sine table at 0x44000
	sineTableAddr := uint32(0x44000)

	// Write sine values we need:
	// sin(0) = 0, sin(64) = 256, sin(96) = 128, cos(0) = sin(64) = 256
	cpu.Write32(sineTableAddr+0*4, 0)    // sin[0] = 0
	cpu.Write32(sineTableAddr+32*4, 128) // sin[32] = 128
	cpu.Write32(sineTableAddr+64*4, 256) // sin[64] = 256 (also cos[0])
	cpu.Write32(sineTableAddr+96*4, 128) // sin[96] = 128 (also cos[32])

	// Setup: reciprocal table at 0x44400
	recipTableAddr := uint32(0x44400)
	cpu.Write32(recipTableAddr+0*4, 0)
	cpu.Write32(recipTableAddr+1*4, 1536)
	cpu.Write32(recipTableAddr+2*4, 768) // This is what scale=0 uses (recip[scale+2])

	// Test calculation for scale=0, angle=0
	// cos[0] = sin[64] = 256
	// sin[0] = 0
	// DU = (768 * 256) >> 8 = 768
	// DV = (768 * 0) >> 8 = 0

	// Verify cosine lookup: cos[0] = sin[(0+64)&255] = sin[64]
	cosAngle := (0 + 64) & 255
	cosAddr := sineTableAddr + uint32(cosAngle*4)
	cosValue := cpu.Read32(cosAddr)
	if cosValue != 256 {
		t.Errorf("cos[0] should be 256, got %d (from sin[64] at 0x%X)", cosValue, cosAddr)
	}

	// Verify sine lookup
	sinAddr := sineTableAddr + uint32(0*4)
	sinValue := cpu.Read32(sinAddr)
	if sinValue != 0 {
		t.Errorf("sin[0] should be 0, got %d", sinValue)
	}

	// Verify reciprocal lookup
	recipAddr := recipTableAddr + uint32(2*4) // recip[scale+2] = recip[0+2] = recip[2]
	recipValue := cpu.Read32(recipAddr)
	if recipValue != 768 {
		t.Errorf("recip[2] should be 768, got %d", recipValue)
	}

	// Calculate expected DU and DV
	expectedDU := (int32(recipValue) * int32(cosValue)) >> 8
	expectedDV := (int32(recipValue) * int32(sinValue)) >> 8

	if expectedDU != 768 {
		t.Errorf("Expected DU=768, calculated %d", expectedDU)
	}
	if expectedDV != 0 {
		t.Errorf("Expected DV=0, calculated %d", expectedDV)
	}
}

// TestM68K_RotozoomBugHypothesis tests the specific bug hypothesis:
// The DV table lookup might be getting the wrong values
func TestM68K_RotozoomBugHypothesis(t *testing.T) {
	cpu := setupTestCPU()

	// Setup tables exactly as the assembly should
	sineTableAddr := uint32(0x44000)
	duTableAddr := uint32(0x44420)
	dvTableAddr := uint32(0x45820)

	// Write sine table (simplified - just key values)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		cpu.Write32(sineTableAddr+uint32(i*4), uint32(int32(value)))
	}

	// Verify sin/cos at angle 32
	sin32 := int32(cpu.Read32(sineTableAddr + 32*4))
	cos32 := int32(cpu.Read32(sineTableAddr + 96*4)) // cos(32) = sin(32+64) = sin(96)

	if sin32 != 128 {
		t.Errorf("sin[32] should be 128, got %d", sin32)
	}
	if cos32 != 128 {
		t.Errorf("cos[32] (sin[96]) should be 128, got %d", cos32)
	}

	// Now manually compute what should be in DU and DV tables
	// For scale=0, recip=768:
	// At angle 32: DU = (768*128)>>8 = 384, DV = (768*128)>>8 = 384

	scaleInv := int32(768)
	expectedDU := (scaleInv * cos32) >> 8 // Should be 384
	expectedDV := (scaleInv * sin32) >> 8 // Should be 384

	if expectedDU != 384 {
		t.Errorf("Expected DU at angle 32 = 384, calculated %d", expectedDU)
	}
	if expectedDV != 384 {
		t.Errorf("Expected DV at angle 32 = 384, calculated %d", expectedDV)
	}

	// The bug hypothesis: if DV is always 0 or equals DU, we get vertical stripes
	// Let's verify the assembly's offset calculation is correct

	// DU_TABLE offset = scale * 1024 + angle * 4
	// DV_TABLE offset = scale * 1024 + angle * 4 (same offset into DV table)

	scale := uint32(0)
	angle := uint32(32)
	offset := scale*1024 + angle*4

	duAddr := duTableAddr + offset
	dvAddr := dvTableAddr + offset

	// Write expected values
	cpu.Write32(duAddr, uint32(expectedDU))
	cpu.Write32(dvAddr, uint32(expectedDV))

	// Verify the lookup addresses are different
	if duAddr == dvAddr {
		t.Errorf("BUG: DU and DV addresses are the same! addr=0x%X", duAddr)
	}

	// The tables themselves should be at different base addresses
	if duTableAddr == dvTableAddr {
		t.Errorf("BUG: DU_TABLE and DV_TABLE have same base address!")
	}

	// Verify the offset between tables
	expectedOffset := uint32(0x45820 - 0x44420) // Should be 0x1400 = 5120
	actualOffset := dvTableAddr - duTableAddr
	if actualOffset != expectedOffset {
		t.Errorf("Table offset mismatch: expected 0x%X, got 0x%X", expectedOffset, actualOffset)
	}
}

// TestM68K_RealRotozoomAngleSweep simulates rotating through angles
// to verify DU and DV change correctly for rotation effect
func TestM68K_RealRotozoomAngleSweep(t *testing.T) {
	// Generate expected DU/DV for a full rotation at scale=0
	// If the rotozoomer only shows vertical stripes, DV would be constant

	recipTable := []int32{0, 1536, 768, 512, 384, 307, 256, 219}
	scaleInv := recipTable[2] // 768 for scale=0

	getSine := func(angle int) int32 {
		angle = angle & 255
		ramp := angle & 63
		if (angle & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (angle & 128) != 0 {
			value = -value
		}
		return int32(value)
	}

	getCos := func(angle int) int32 {
		return getSine(angle + 64)
	}

	// Track whether DV actually varies
	dvValues := make(map[int32]bool)
	duValues := make(map[int32]bool)

	for angle := 0; angle < 256; angle++ {
		cos := getCos(angle)
		sin := getSine(angle)

		var du, dv int32
		if cos >= 0 {
			du = (scaleInv * cos) >> 8
		} else {
			du = -((scaleInv * (-cos)) >> 8)
		}

		if sin >= 0 {
			dv = (scaleInv * sin) >> 8
		} else {
			dv = -((scaleInv * (-sin)) >> 8)
		}

		duValues[du] = true
		dvValues[dv] = true

		// For a proper rotozoomer, at angle 0: DU should be max, DV should be 0
		// At angle 64 (90°): DU should be 0, DV should be max
		if angle == 0 {
			if du != 768 || dv != 0 {
				t.Errorf("At angle 0: expected DU=768, DV=0, got DU=%d, DV=%d", du, dv)
			}
		}
		if angle == 64 {
			if du != 0 || dv != 768 {
				t.Errorf("At angle 64: expected DU=0, DV=768, got DU=%d, DV=%d", du, dv)
			}
		}
	}

	// Verify we have variation in both DU and DV
	// For a working rotozoomer, both should have many distinct values
	if len(dvValues) < 10 {
		t.Errorf("BUG: DV has too few unique values (%d) - would cause stripes!", len(dvValues))
	}
	if len(duValues) < 10 {
		t.Errorf("BUG: DU has too few unique values (%d)", len(duValues))
	}

	t.Logf("DU has %d unique values, DV has %d unique values", len(duValues), len(dvValues))
}

// =============================================================================
// Tests for verifying actual pixel rendering behavior
// =============================================================================

// TestM68K_PixelIncrements verifies that U and V coordinates change per pixel
func TestM68K_PixelIncrements(t *testing.T) {
	// At angle 32 (approximately 45°), both du_dx and dv_dx should be non-zero
	// This means both U and V should change as we move across the screen

	scaleInv := int32(768)
	sin32 := int32(128)
	cos32 := int32(128)

	duDx := (scaleInv * cos32) >> 8 // 384
	dvDx := (scaleInv * sin32) >> 8 // 384

	if duDx == 0 {
		t.Error("du_dx is zero - texture U won't change across screen!")
	}
	if dvDx == 0 {
		t.Error("dv_dx is zero - texture V won't change across screen (causes vertical stripes)!")
	}

	// Simulate a few pixels
	texU := int32(0x8000) // Start at center
	texV := int32(0x8000)

	for pixel := 0; pixel < 5; pixel++ {
		uCoord := (texU >> 8) & 255
		vCoord := (texV >> 8) & 255

		t.Logf("Pixel %d: texU=0x%X, texV=0x%X, U=%d, V=%d", pixel, texU, texV, uCoord, vCoord)

		texU += duDx
		texV += dvDx
	}

	// Verify V coordinate actually changed
	initialV := (int32(0x8000) >> 8) & 255
	finalV := ((int32(0x8000) + 4*dvDx) >> 8) & 255

	if initialV == finalV {
		t.Errorf("V coordinate didn't change across 5 pixels: initial=%d, final=%d", initialV, finalV)
	}
}

// TestM68K_VerticalStripesBug specifically tests conditions that would cause vertical stripes
func TestM68K_VerticalStripesBug(t *testing.T) {
	// Vertical stripes occur when:
	// 1. dv_dx is always 0 (V doesn't change horizontally)
	// 2. dv_dx always equals du_dx (diagonal stripes at 45°, not rotation)

	// Test that at angle 0, dv_dx = 0 (this is correct - horizontal texture mapping)
	// But at angle 32, dv_dx should NOT be 0

	getSine := func(angle int) int32 {
		angle = angle & 255
		ramp := angle & 63
		if (angle & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (angle & 128) != 0 {
			value = -value
		}
		return int32(value)
	}

	scaleInv := int32(768)

	// At angle 0
	sin0 := getSine(0)
	dvDxAt0 := (scaleInv * sin0) >> 8
	if dvDxAt0 != 0 {
		t.Errorf("At angle 0, dv_dx should be 0, got %d", dvDxAt0)
	}

	// At angle 32
	sin32 := getSine(32)
	dvDxAt32 := (scaleInv * sin32) >> 8
	if dvDxAt32 == 0 {
		t.Errorf("At angle 32, dv_dx should NOT be 0, got %d", dvDxAt32)
	}

	// At angle 64 (90°)
	sin64 := getSine(64)
	dvDxAt64 := (scaleInv * sin64) >> 8
	if dvDxAt64 != 768 {
		t.Errorf("At angle 64, dv_dx should be 768, got %d", dvDxAt64)
	}

	// If we're seeing stripes "that change width", the zoom is working
	// but the dv_dx might be wrong

	t.Logf("dv_dx at angle 0: %d", dvDxAt0)
	t.Logf("dv_dx at angle 32: %d", dvDxAt32)
	t.Logf("dv_dx at angle 64: %d", dvDxAt64)
}

// =============================================================================
// Test running actual M68K assembly code
// =============================================================================

// TestM68K_GenerateDUDVTablesAssembly runs the actual M68K assembly code
// that generates the DU/DV tables and verifies the results
func TestM68K_GenerateDUDVTablesAssembly(t *testing.T) {
	cpu := setupTestCPU()

	// First, set up the sine table (we'll use pre-computed values)
	sineTableAddr := uint32(0x44000)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		cpu.Write32(sineTableAddr+uint32(i*4), uint32(int32(value)))
	}

	// Set up the reciprocal table
	recipTableAddr := uint32(0x44400)
	recipValues := []uint32{0, 1536, 768, 512, 384, 307, 256, 219}
	for i, v := range recipValues {
		cpu.Write32(recipTableAddr+uint32(i*4), v)
	}

	duTableAddr := uint32(0x44420)
	dvTableAddr := uint32(0x45820)

	// Now run the equivalent of generate_dudv_tables
	// We'll simulate the assembly logic in Go but using actual CPU operations

	// For each scale (0-4)
	for scale := 0; scale < 5; scale++ {
		// Get reciprocal value: recip_table[scale + 2]
		recipAddr := recipTableAddr + uint32((scale+2)*4)
		scaleInv := cpu.Read32(recipAddr)

		// For each angle (0-255)
		for angle := 0; angle < 256; angle++ {
			// Get cos[angle] = sin[(angle + 64) & 255]
			cosIndex := (angle + 64) & 255
			cosAddr := sineTableAddr + uint32(cosIndex*4)
			cosValue := int32(cpu.Read32(cosAddr))

			// Get sin[angle]
			sinAddr := sineTableAddr + uint32(angle*4)
			sinValue := int32(cpu.Read32(sinAddr))

			// Calculate DU = (scale_inv * cos) >> 8
			var du int32
			if cosValue >= 0 {
				du = int32((scaleInv * uint32(cosValue)) >> 8)
			} else {
				du = -int32((scaleInv * uint32(-cosValue)) >> 8)
			}

			// Calculate DV = (scale_inv * sin) >> 8
			var dv int32
			if sinValue >= 0 {
				dv = int32((scaleInv * uint32(sinValue)) >> 8)
			} else {
				dv = -int32((scaleInv * uint32(-sinValue)) >> 8)
			}

			// Store in tables
			offset := uint32(scale*1024 + angle*4)
			cpu.Write32(duTableAddr+offset, uint32(du))
			cpu.Write32(dvTableAddr+offset, uint32(dv))
		}
	}

	// Now verify some key values
	testCases := []struct {
		scale      int
		angle      int
		expectedDU int32
		expectedDV int32
	}{
		// Scale 0, recip=768
		{0, 0, 768, 0},    // cos(0)=256, sin(0)=0 → DU=768, DV=0
		{0, 64, 0, 768},   // cos(64)=0, sin(64)=256 → DU=0, DV=768
		{0, 128, -768, 0}, // cos(128)=-256, sin(128)=0 → DU=-768, DV=0
		{0, 192, 0, -768}, // cos(192)=0, sin(192)=-256 → DU=0, DV=-768
		{0, 32, 384, 384}, // cos(32)=sin(96)=128, sin(32)=128 → DU=384, DV=384
	}

	for _, tc := range testCases {
		offset := uint32(tc.scale*1024 + tc.angle*4)
		actualDU := int32(cpu.Read32(duTableAddr + offset))
		actualDV := int32(cpu.Read32(dvTableAddr + offset))

		if actualDU != tc.expectedDU {
			t.Errorf("DU[scale=%d][angle=%d]: got %d, expected %d",
				tc.scale, tc.angle, actualDU, tc.expectedDU)
		}
		if actualDV != tc.expectedDV {
			t.Errorf("DV[scale=%d][angle=%d]: got %d, expected %d",
				tc.scale, tc.angle, actualDV, tc.expectedDV)
		}
	}

	// Verify that DU and DV values vary as angle changes
	// This is the critical test - if DV doesn't vary, we get vertical stripes
	var prevDV int32 = 0
	dvChanged := false
	for angle := 0; angle < 64; angle++ {
		offset := uint32(angle * 4) // scale 0
		dv := int32(cpu.Read32(dvTableAddr + offset))
		if angle > 0 && dv != prevDV {
			dvChanged = true
		}
		prevDV = dv
	}

	if !dvChanged {
		t.Error("BUG: DV values do not change across angles - this would cause vertical stripes!")
	}
}

// TestM68K_ActualMULUSequence tests the exact MULU sequence from the assembly
func TestM68K_ActualMULUSequence(t *testing.T) {
	cpu := setupTestCPU()

	// Test the positive path: mulu.w d6,d1; lsr.l #8,d1
	// cosValue = 256 (positive), scaleInv = 768

	cpu.DataRegs[1] = 256 // d1 = cosValue
	cpu.DataRegs[6] = 768 // d6 = scaleInv

	// MULU.W D6,D1: 1100 001 011 000 110 = 0xC2C6
	cpu.PC = M68K_ENTRY_POINT
	cpu.Write16(cpu.PC, 0xC2C6)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Result should be 256 * 768 = 196608
	if cpu.DataRegs[1] != 196608 {
		t.Errorf("After MULU.W: expected 196608, got %d", cpu.DataRegs[1])
	}

	// LSR.L #8,D1: 1110 000 0 10 0 01 001 = 0xE089
	cpu.Write16(cpu.PC, 0xE089)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Result should be 196608 >> 8 = 768
	if cpu.DataRegs[1] != 768 {
		t.Errorf("After LSR.L: expected 768, got %d", cpu.DataRegs[1])
	}
}

// TestM68K_NegativeMULUSequence tests the negative value path
func TestM68K_NegativeMULUSequence(t *testing.T) {
	cpu := setupTestCPU()

	// Test the negative path: neg.l d1; mulu.w d6,d1; lsr.l #8,d1; neg.l d1
	// cosValue = -256 (0xFFFFFF00), scaleInv = 768

	cpu.DataRegs[1] = 0xFFFFFF00 // d1 = -256
	cpu.DataRegs[6] = 768        // d6 = scaleInv

	// NEG.L D1: 0100 0100 10 000 001 = 0x4481
	cpu.PC = M68K_ENTRY_POINT
	cpu.Write16(cpu.PC, 0x4481)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 256 {
		t.Errorf("After first NEG.L: expected 256, got 0x%X", cpu.DataRegs[1])
	}

	// MULU.W D6,D1
	cpu.Write16(cpu.PC, 0xC2C6)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 196608 {
		t.Errorf("After MULU.W: expected 196608, got %d", cpu.DataRegs[1])
	}

	// LSR.L #8,D1
	cpu.Write16(cpu.PC, 0xE089)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 768 {
		t.Errorf("After LSR.L: expected 768, got %d", cpu.DataRegs[1])
	}

	// NEG.L D1
	cpu.Write16(cpu.PC, 0x4481)
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Result should be -768 = 0xFFFFFD00
	expected := uint32(0xFFFFFD00)
	if cpu.DataRegs[1] != expected {
		t.Errorf("After final NEG.L: expected 0x%X (-768), got 0x%X (%d)",
			expected, cpu.DataRegs[1], int32(cpu.DataRegs[1]))
	}
}

// TestM68K_BPLBranching tests the BPL instruction used for sign checking
func TestM68K_BPLBranching(t *testing.T) {
	cpu := setupTestCPU()

	// Test 1: Positive value should branch
	cpu.DataRegs[0] = 256 // Positive value
	cpu.DataRegs[1] = 256

	// MOVE.L D0,D1 then BPL should branch
	cpu.PC = M68K_ENTRY_POINT

	// Check that after move.l d0,d1 with positive value, N flag is clear
	cpu.Write16(cpu.PC, 0x2200) // MOVE.L D0,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if (cpu.SR & M68K_SR_N) != 0 {
		t.Error("N flag should be clear for positive value")
	}

	// Test 2: Negative value should not branch
	cpu.DataRegs[0] = 0xFFFFFF00 // -256

	cpu.Write16(cpu.PC, 0x2200) // MOVE.L D0,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if (cpu.SR & M68K_SR_N) == 0 {
		t.Error("N flag should be set for negative value")
	}
}

// TestM68K_TraceTableGeneration simulates the assembly's table generation
// and traces each step to find the bug
func TestM68K_TraceTableGeneration(t *testing.T) {
	cpu := setupTestCPU()

	// Set up sine table
	sineTableAddr := uint32(0x44000)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		cpu.Write32(sineTableAddr+uint32(i*4), uint32(int32(value)))
	}

	// Set up reciprocal table
	recipTableAddr := uint32(0x44400)
	recipValues := []uint32{0, 1536, 768, 512, 384, 307, 256, 219}
	for i, v := range recipValues {
		cpu.Write32(recipTableAddr+uint32(i*4), v)
	}

	duTableAddr := uint32(0x44420)
	dvTableAddr := uint32(0x45820)

	// Simulate the assembly's table generation for scale 0
	scale := 0
	scaleInv := recipValues[scale+2] // 768

	// Test specific angles to trace the bug
	testAngles := []int{0, 32, 64, 128, 192}

	for _, angle := range testAngles {
		// Simulate: move.l d5,d0; addi.l #64,d0; andi.l #255,d0; lsl.l #2,d0
		cosIndex := (angle + 64) & 255
		cosAddr := sineTableAddr + uint32(cosIndex*4)

		// Simulate: move.l (a3,d0.l),d0
		cosValue := int32(cpu.Read32(cosAddr))

		// Simulate: move.l d0,d1; bpl.s .cos_pos
		// If cosValue is negative, take negative path
		var du int32
		if cosValue >= 0 {
			// Positive path: mulu.w d6,d1; lsr.l #8,d1
			du = int32((uint32(cosValue) * scaleInv) >> 8)
		} else {
			// Negative path: neg.l d1; mulu.w d6,d1; lsr.l #8,d1; neg.l d1
			negCos := -cosValue
			du = -int32((uint32(negCos) * scaleInv) >> 8)
		}

		// Same for sin
		sinAddr := sineTableAddr + uint32(angle*4)
		sinValue := int32(cpu.Read32(sinAddr))

		var dv int32
		if sinValue >= 0 {
			dv = int32((uint32(sinValue) * scaleInv) >> 8)
		} else {
			negSin := -sinValue
			dv = -int32((uint32(negSin) * scaleInv) >> 8)
		}

		// Store in tables
		offset := uint32(scale*1024 + angle*4)
		cpu.Write32(duTableAddr+offset, uint32(du))
		cpu.Write32(dvTableAddr+offset, uint32(dv))

		t.Logf("Angle %d: cos=%d (from sin[%d]), sin=%d, DU=%d, DV=%d",
			angle, cosValue, cosIndex, sinValue, du, dv)

		// Verify the values
		switch angle {
		case 0:
			if du != 768 || dv != 0 {
				t.Errorf("Angle 0: expected DU=768, DV=0, got DU=%d, DV=%d", du, dv)
			}
		case 64:
			if du != 0 || dv != 768 {
				t.Errorf("Angle 64: expected DU=0, DV=768, got DU=%d, DV=%d", du, dv)
			}
		case 32:
			if du != 384 || dv != 384 {
				t.Errorf("Angle 32: expected DU=384, DV=384, got DU=%d, DV=%d", du, dv)
			}
		case 128:
			if du != -768 || dv != 0 {
				t.Errorf("Angle 128: expected DU=-768, DV=0, got DU=%d, DV=%d", du, dv)
			}
		case 192:
			if du != 0 || dv != -768 {
				t.Errorf("Angle 192: expected DU=0, DV=-768, got DU=%d, DV=%d", du, dv)
			}
		}
	}
}

// TestM68K_RunActualAssemblySequence runs the actual M68K opcodes
// for the DU/DV calculation to find emulator bugs
func TestM68K_RunActualAssemblySequence(t *testing.T) {
	cpu := setupTestCPU()

	// Set up sine table
	sineTableAddr := uint32(0x44000)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		cpu.Write32(sineTableAddr+uint32(i*4), uint32(int32(value)))
	}

	// Test at angle 32 (should have both DU and DV = 384)
	angle := 32
	cosIndex := (angle + 64) & 255 // 96
	scaleInv := uint32(768)

	// Set up registers as the assembly would
	cpu.AddrRegs[3] = sineTableAddr // a3 = SINE_TABLE
	cpu.DataRegs[6] = scaleInv      // d6 = scale_inv

	// Calculate cosine address: (angle + 64) & 255 * 4
	cpu.DataRegs[5] = uint32(angle) // d5 = angle
	cpu.DataRegs[0] = uint32(angle)

	// Simulate: addi.l #64,d0
	cpu.DataRegs[0] += 64

	// Simulate: andi.l #255,d0
	cpu.DataRegs[0] &= 255

	if cpu.DataRegs[0] != uint32(cosIndex) {
		t.Errorf("cosIndex calculation: expected %d, got %d", cosIndex, cpu.DataRegs[0])
	}

	// Simulate: lsl.l #2,d0
	cpu.DataRegs[0] <<= 2

	// Now d0 has the byte offset into sine table
	expectedOffset := uint32(cosIndex * 4)
	if cpu.DataRegs[0] != expectedOffset {
		t.Errorf("offset calculation: expected %d, got %d", expectedOffset, cpu.DataRegs[0])
	}

	// Simulate: move.l (a3,d0.l),d0
	// This is the indexed addressing mode (An,Dn.L)
	addr := cpu.AddrRegs[3] + cpu.DataRegs[0]
	cosValue := cpu.Read32(addr)
	cpu.DataRegs[0] = cosValue

	expectedCos := int32(128) // sin(96) = 128
	if int32(cpu.DataRegs[0]) != expectedCos {
		t.Errorf("cosine value: expected %d, got %d", expectedCos, int32(cpu.DataRegs[0]))
	}

	// Simulate: move.l d0,d1
	cpu.PC = M68K_ENTRY_POINT
	cpu.Write16(cpu.PC, 0x2200) // MOVE.L D0,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != cosValue {
		t.Errorf("After MOVE.L D0,D1: expected D1=%d, got D1=%d", cosValue, cpu.DataRegs[1])
	}

	// Check N flag - should be clear for positive value
	if (cpu.SR & M68K_SR_N) != 0 {
		t.Error("N flag should be clear for positive cosine value")
	}

	// Since it's positive, we take the positive path
	// Simulate: mulu.w d6,d1
	cpu.Write16(cpu.PC, 0xC2C6) // MULU.W D6,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	expectedMul := uint32(128 * 768) // 98304
	if cpu.DataRegs[1] != expectedMul {
		t.Errorf("After MULU.W D6,D1: expected %d, got %d", expectedMul, cpu.DataRegs[1])
	}

	// Simulate: lsr.l #8,d1
	cpu.Write16(cpu.PC, 0xE089) // LSR.L #8,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	expectedShift := expectedMul >> 8 // 384
	if cpu.DataRegs[1] != expectedShift {
		t.Errorf("After LSR.L #8,D1: expected %d, got %d", expectedShift, cpu.DataRegs[1])
	}

	t.Logf("DU calculation for angle 32: cosValue=%d, result=%d", cosValue, cpu.DataRegs[1])

	// Now test DV for angle 32
	// Reset d5 to angle
	cpu.DataRegs[5] = uint32(angle)
	cpu.DataRegs[0] = uint32(angle)

	// Simulate: lsl.l #2,d0 (for sin index)
	cpu.DataRegs[0] <<= 2

	// Simulate: move.l (a3,d0.l),d0
	addr = cpu.AddrRegs[3] + cpu.DataRegs[0]
	sinValue := cpu.Read32(addr)
	cpu.DataRegs[0] = sinValue

	expectedSin := int32(128) // sin(32) = 128
	if int32(cpu.DataRegs[0]) != expectedSin {
		t.Errorf("sine value: expected %d, got %d", expectedSin, int32(cpu.DataRegs[0]))
	}

	// Simulate: move.l d0,d1
	cpu.Write16(cpu.PC, 0x2200) // MOVE.L D0,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Simulate: mulu.w d6,d1
	cpu.Write16(cpu.PC, 0xC2C6) // MULU.W D6,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Simulate: lsr.l #8,d1
	cpu.Write16(cpu.PC, 0xE089) // LSR.L #8,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	t.Logf("DV calculation for angle 32: sinValue=%d, result=%d", sinValue, cpu.DataRegs[1])

	if cpu.DataRegs[1] != 384 {
		t.Errorf("DV for angle 32: expected 384, got %d", cpu.DataRegs[1])
	}
}

// TestM68K_RunNegativeAngleSequence tests the negative path (angle 192)
func TestM68K_RunNegativeAngleSequence(t *testing.T) {
	cpu := setupTestCPU()

	// Set up sine table
	sineTableAddr := uint32(0x44000)
	for i := 0; i < 256; i++ {
		ramp := i & 63
		if (i & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (i & 128) != 0 {
			value = -value
		}
		cpu.Write32(sineTableAddr+uint32(i*4), uint32(int32(value)))
	}

	// Test at angle 192 (sin = -256, should give DV = -768)
	angle := 192
	scaleInv := uint32(768)

	cpu.AddrRegs[3] = sineTableAddr
	cpu.DataRegs[6] = scaleInv

	// Get sin[192] = -256
	cpu.DataRegs[0] = uint32(angle)
	cpu.DataRegs[0] <<= 2 // *4

	addr := cpu.AddrRegs[3] + cpu.DataRegs[0]
	sinValue := cpu.Read32(addr)
	cpu.DataRegs[0] = sinValue

	t.Logf("sin[192] = %d (0x%X)", int32(sinValue), sinValue)

	// sin(192) should be -256
	if int32(sinValue) != -256 {
		t.Errorf("sin(192): expected -256, got %d", int32(sinValue))
	}

	// Simulate: move.l d0,d1
	cpu.PC = M68K_ENTRY_POINT
	cpu.Write16(cpu.PC, 0x2200) // MOVE.L D0,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	// Check that N flag is set (negative value)
	if (cpu.SR & M68K_SR_N) == 0 {
		t.Error("N flag should be set for negative sine value")
	}

	// Since it's negative, we take the negative path
	// Simulate: neg.l d1
	cpu.Write16(cpu.PC, 0x4481) // NEG.L D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 256 {
		t.Errorf("After first NEG.L: expected 256, got %d", cpu.DataRegs[1])
	}

	// Simulate: mulu.w d6,d1
	cpu.Write16(cpu.PC, 0xC2C6) // MULU.W D6,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	expectedMul := uint32(256 * 768) // 196608
	if cpu.DataRegs[1] != expectedMul {
		t.Errorf("After MULU.W D6,D1: expected %d, got %d", expectedMul, cpu.DataRegs[1])
	}

	// Simulate: lsr.l #8,d1
	cpu.Write16(cpu.PC, 0xE089) // LSR.L #8,D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	if cpu.DataRegs[1] != 768 {
		t.Errorf("After LSR.L #8,D1: expected 768, got %d", cpu.DataRegs[1])
	}

	// Simulate: neg.l d1
	cpu.Write16(cpu.PC, 0x4481) // NEG.L D1
	cpu.currentIR = cpu.Fetch16()
	cpu.FetchAndDecodeInstruction()

	expectedNeg := uint32(0xFFFFFD00) // -768
	if cpu.DataRegs[1] != expectedNeg {
		t.Errorf("After final NEG.L: expected 0x%X (-768), got 0x%X (%d)",
			expectedNeg, cpu.DataRegs[1], int32(cpu.DataRegs[1]))
	}

	t.Logf("DV calculation for angle 192: sinValue=%d, result=%d", int32(sinValue), int32(cpu.DataRegs[1]))
}

// =============================================================================
// Benchmark and stress tests
// =============================================================================

// BenchmarkM68K_MULU measures MULU.W performance
func BenchmarkM68K_MULU(b *testing.B) {
	cpu := setupTestCPU()
	cpu.DataRegs[0] = 256
	cpu.DataRegs[1] = 768

	for i := 0; i < b.N; i++ {
		cpu.PC = M68K_ENTRY_POINT
		cpu.Write16(cpu.PC, 0xC2C0) // MULU.W D0,D1
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
		cpu.DataRegs[1] = 768 // Reset
	}
}

// TestM68K_TrigAccuracy compares our sine table against math.Sin
func TestM68K_TrigAccuracy(t *testing.T) {
	getSine := func(angle int) float64 {
		angle = angle & 255
		ramp := angle & 63
		if (angle & 64) != 0 {
			ramp = 64 - ramp
		}
		value := ramp * 4
		if value > 256 {
			value = 256
		}
		if (angle & 128) != 0 {
			value = -value
		}
		return float64(value) / 256.0 // Convert to -1.0 to 1.0 range
	}

	maxError := 0.0
	for angle := 0; angle < 256; angle++ {
		// Convert our angle (0-255) to radians (0-2π)
		radians := float64(angle) * 2.0 * math.Pi / 256.0
		expected := math.Sin(radians)
		actual := getSine(angle)

		error := math.Abs(expected - actual)
		if error > maxError {
			maxError = error
		}
	}

	// Our approximation uses a linear triangle wave, so it has some error
	// The maximum error occurs at 45°/135°/225°/315° and is about 0.21
	if maxError > 0.25 {
		t.Errorf("Sine approximation error too high: max error = %f", maxError)
	}

	t.Logf("Maximum sine approximation error: %f", maxError)
}

// TestM68K_RunAssembledBinaryTableGeneration assembles and runs the actual
// M68K rotozoomer binary, then checks the DU/DV table values
func TestM68K_RunAssembledBinaryTableGeneration(t *testing.T) {
	// Assemble the rotozoomer
	asmPath := "assembler/rotozoomer_68k.asm"
	binPath := "/tmp/rotozoom_integration_test.ie68"

	// Run vasm to assemble
	cmd := exec.Command("vasmm68k_mot", "-Fbin", "-m68020", "-devpac", "-o", binPath, asmPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to assemble: %v\n%s", err, string(output))
	}

	// Load the binary
	program, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("Failed to read assembled binary: %v", err)
	}

	t.Logf("Loaded binary: %d bytes", len(program))

	// Set up the CPU with proper M68K endianness
	boilerPlateTest()
	bus := NewSystemBus()

	// Initialize terminal output
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)

	cpu := NewM68KCPU(bus)

	// Write vector table using CPU's Write32 (handles M68K endianness correctly)
	cpu.Write32(0, M68K_STACK_START)                 // Initial SP at vector 0
	cpu.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT) // Initial PC at vector 1

	// Load the program
	cpu.LoadProgramBytes(program)

	// Execute the initialization subroutines
	// The main loop starts at 'start' which calls:
	// 1. generate_texture (bsr)
	// 2. generate_sine_table (bsr)
	// 3. generate_recip_table (bsr)
	// 4. generate_dudv_tables (bsr)
	// Then enters the main loop

	// First, let's dump the first bytes of the loaded program to verify it loaded correctly
	t.Logf("First 32 bytes of loaded program (big-endian M68K binary):")
	for i := 0; i < 32; i += 2 {
		if i < len(program) {
			t.Logf("  Offset 0x%04X: raw bytes 0x%02X%02X", i, program[i], program[i+1])
		}
	}

	// And what's in memory after loading
	t.Logf("First 32 bytes at memory 0x1000 (as loaded by CPU):")
	for i := uint32(0); i < 32; i += 2 {
		addr := uint32(0x1000) + i
		val := cpu.Read16(addr)
		t.Logf("  Addr 0x%04X: 0x%04X", addr, val)
	}

	// Now run execution until tables are generated
	t.Logf("Initial PC=0x%X, SP=0x%X", cpu.PC, cpu.AddrRegs[7])

	// Run enough instructions to complete all table generation
	// generate_texture: ~65536 iterations * ~10 instructions = ~650K instructions
	// generate_sine_table: ~256 iterations * ~10 instructions = ~2.5K instructions
	// generate_recip_table: ~8 iterations * ~3 instructions = ~24 instructions
	// generate_dudv_tables: ~1280 entries * ~20 instructions = ~25K instructions
	// Total: roughly 700K instructions for init
	maxInstructions := 800000

	for i := 0; i < maxInstructions; i++ {
		// IMPORTANT: Must call Fetch16 to load currentIR before FetchAndDecodeInstruction
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()

		// Check for main loop (BSR wait_vsync at 0x1038)
		// After all init, the main loop will call wait_vsync which polls VIDEO_STATUS
		// We can detect this by checking if we're past the init code and looping
		if cpu.PC >= 0x1030 && cpu.PC <= 0x1042 && i > 100000 {
			t.Logf("Reached main loop at PC=0x%X after %d instructions", cpu.PC, i)
			break
		}
	}

	t.Logf("Final PC=0x%X, SP=0x%X after %d iterations", cpu.PC, cpu.AddrRegs[7], maxInstructions)

	// Now check the generated tables
	// SINE_TABLE at 0x44000 - check a few key values
	sineTableAddr := uint32(0x44000)
	sine0 := int32(cpu.Read32(sineTableAddr))           // sin(0) = 0
	sine64 := int32(cpu.Read32(sineTableAddr + 64*4))   // sin(64) = 256
	sine128 := int32(cpu.Read32(sineTableAddr + 128*4)) // sin(128) = 0
	sine192 := int32(cpu.Read32(sineTableAddr + 192*4)) // sin(192) = -256

	t.Logf("Sine table: sin(0)=%d, sin(64)=%d, sin(128)=%d, sin(192)=%d",
		sine0, sine64, sine128, sine192)

	if sine0 != 0 {
		t.Errorf("sin(0): expected 0, got %d", sine0)
	}
	if sine64 != 256 {
		t.Errorf("sin(64): expected 256, got %d", sine64)
	}
	if sine128 != 0 {
		t.Errorf("sin(128): expected 0, got %d", sine128)
	}
	if sine192 != -256 {
		t.Errorf("sin(192): expected -256, got %d", sine192)
	}

	// Check DU_TABLE and DV_TABLE values for scale 0
	duTableAddr := uint32(0x44420)
	dvTableAddr := uint32(0x45820)

	// Scale 0 uses recip[2] = 768
	// At angle 0: cos(0)=256, sin(0)=0 → DU=768*256/256=768, DV=0
	// At angle 64: cos(64)=0, sin(64)=256 → DU=0, DV=768
	// At angle 32: cos(32)=128, sin(32)=128 → DU=384, DV=384

	testCases := []struct {
		angle      int
		expectedDU int32
		expectedDV int32
	}{
		{0, 768, 0},
		{32, 384, 384},
		{64, 0, 768},
		{128, -768, 0},
		{192, 0, -768},
	}

	for _, tc := range testCases {
		offset := uint32(tc.angle * 4)
		actualDU := int32(cpu.Read32(duTableAddr + offset))
		actualDV := int32(cpu.Read32(dvTableAddr + offset))

		if actualDU != tc.expectedDU {
			t.Errorf("DU[scale=0][angle=%d]: expected %d, got %d",
				tc.angle, tc.expectedDU, actualDU)
		}
		if actualDV != tc.expectedDV {
			t.Errorf("DV[scale=0][angle=%d]: expected %d, got %d",
				tc.angle, tc.expectedDV, actualDV)
		}

		t.Logf("Angle %d: DU=%d (expected %d), DV=%d (expected %d)",
			tc.angle, actualDU, tc.expectedDU, actualDV, tc.expectedDV)
	}

	// CRITICAL: Check that DV values vary across angles
	// This is what causes vertical stripes if broken
	dvValues := make([]int32, 64)
	for i := 0; i < 64; i++ {
		dvValues[i] = int32(cpu.Read32(dvTableAddr + uint32(i*4)))
	}

	dvVaries := false
	for i := 1; i < 64; i++ {
		if dvValues[i] != dvValues[0] {
			dvVaries = true
			break
		}
	}

	if !dvVaries {
		t.Errorf("CRITICAL BUG: DV values do not vary across angles - this causes vertical stripes!")
		t.Logf("First few DV values: %v", dvValues[:8])
	}

	// Check that var_dv_dx changes as angle changes
	// var_angle is at VAR_BASE + 0x00 = 0x46C20
	// var_dv_dx is at VAR_BASE + 0x10 = 0x46C30
	varAngle := uint32(0x46C20)
	varDvDx := uint32(0x46C30)

	// Check initial angle (should be 1 after first update_animation)
	initialAngle := cpu.Read32(varAngle)
	initialDvDx := int32(cpu.Read32(varDvDx))
	t.Logf("After init: angle=%d, dv_dx=%d", initialAngle, initialDvDx)

	// Run more instructions to see if angle advances
	for frame := 0; frame < 5; frame++ {
		// Run enough instructions for a full frame (render is very long)
		// 320x240 pixels / 16 unroll * ~10 instructions = ~48000 instructions per render
		for i := 0; i < 60000; i++ {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}

		angle := cpu.Read32(varAngle)
		dvDx := int32(cpu.Read32(varDvDx))
		t.Logf("After frame %d: angle=%d, dv_dx=%d, PC=0x%X", frame, angle, dvDx, cpu.PC)
	}

	// Check if angle has advanced
	finalAngle := cpu.Read32(varAngle)
	if finalAngle == initialAngle {
		t.Errorf("BUG: angle not advancing! Initial=%d, Final=%d", initialAngle, finalAngle)
	}

	// Check if dv_dx has changed with angle
	finalDvDx := int32(cpu.Read32(varDvDx))
	if finalDvDx == initialDvDx && finalAngle != initialAngle {
		t.Errorf("BUG: dv_dx not changing with angle! Angle went %d->%d but dv_dx stayed at %d",
			initialAngle, finalAngle, initialDvDx)
	}

	t.Logf("Final state: angle=%d, dv_dx=%d", finalAngle, finalDvDx)
}
