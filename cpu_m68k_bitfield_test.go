//go:build m68k_test

package main

import "testing"

// Bit field opcode helpers
const (
	bfOp_BFTST  = 0xE8C0
	bfOp_BFEXTU = 0xE9C0
	bfOp_BFEXTS = 0xEBC0
	bfOp_BFFFO  = 0xEDC0
	bfOp_BFINS  = 0xEFC0
	bfOp_BFSET  = 0xEEC0
	bfOp_BFCLR  = 0xECC0
	bfOp_BFCHG  = 0xEAC0
)

// bfMemEA returns the opcode for a bit field instruction with (An) addressing
func bfMemEA(base uint16, areg uint16) uint16 {
	return base | (M68K_AM_AR_IND << 3) | areg
}

// bfDispEA returns the opcode for a bit field instruction with (d16,An) addressing
func bfDispEA(base uint16, areg uint16) uint16 {
	return base | (M68K_AM_AR_DISP << 3) | areg
}

// bfExt builds the bit field extension word
// destReg: destination/source register (D0-D7)
// offset: immediate offset (0-31)
// width: immediate width (0=32, 1-31)
func bfExt(destReg, offset, width uint16) uint16 {
	return (destReg << 12) | (offset << 6) | width
}

// bfExtRegOff builds the bit field extension word with register offset
// destReg: destination/source register
// offReg: register containing offset (D0-D7)
// width: immediate width (0=32, 1-31)
func bfExtRegOff(destReg, offReg, width uint16) uint16 {
	return (destReg << 12) | M68K_BF_OFFSET_REG | (offReg << 6) | width
}

const bfTestAddr = 0x2000 // Address for test data

// === Bug 1: Memory Bit Field Extraction Shift ===

func TestBitField_Memory_BFEXTU(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFEXTU_(A0){0:8}_byte_aligned",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xAB),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 0xAB),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // 0xAB MSB=1
		},
		{
			Name:  "BFEXTU_(A0){4:8}_cross_byte",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xABCD),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 4, 8)},
			ExpectedRegs:  Reg("D1", 0xBC),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFEXTU_(A0){3:8}_cross_byte",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xABCD),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 3, 8)},
			ExpectedRegs:  Reg("D1", 0x5E),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0), // 0x5E = 01011110, MSB=0
		},
		{
			Name:  "BFEXTU_(A0){0:4}_upper_nibble",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xAB),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 0, 4)},
			ExpectedRegs:  Reg("D1", 0x0A),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // bit 3 of 0xA = 1
		},
		{
			Name:  "BFEXTU_(A0){4:4}_lower_nibble",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xAB),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 4, 4)},
			ExpectedRegs:  Reg("D1", 0x0B),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // bit 3 of 0xB = 1
		},
		{
			Name:  "BFEXTU_(A0){0:16}_word_aligned",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xABCD),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 0, 16)},
			ExpectedRegs:  Reg("D1", 0xABCD),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFEXTU_(A0){0:32}_long_aligned",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint32(0x12345678),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 0, 0)}, // width 0 = 32
			ExpectedRegs:  Reg("D1", 0x12345678),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:  "BFEXTU_(A0){8:16}_byte_offset",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0xAA),
				bfTestAddr + 1: uint8(0xBB),
				bfTestAddr + 2: uint8(0xCC),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 8, 16)},
			ExpectedRegs:  Reg("D1", 0xBBCC),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFEXTU_(A0){4:16}_3byte_span",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0xAB),
				bfTestAddr + 1: uint8(0xCD),
				bfTestAddr + 2: uint8(0xEF),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 4, 16)},
			ExpectedRegs:  Reg("D1", 0xBCDE),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFEXTU_(A0){0:8}_zero_field",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x00),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 0x00),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
	}
	RunM68KTests(t, tests)
}

// === Bug 1 also: BFTST memory extraction ===

func TestBitField_Memory_BFTST(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFTST_(A0){0:8}_nonzero",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xAB),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFTST, 0), bfExt(0, 0, 8)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFTST_(A0){3:8}_checks_correct_bits",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xABCD),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFTST, 0), bfExt(0, 3, 8)},
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0), // field=0x5E, N=0
		},
		{
			Name:  "BFTST_(A0){0:8}_zero",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x00),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFTST, 0), bfExt(0, 0, 8)},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name:  "BFTST_(A0){0:4}_upper_nibble",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x0F),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFTST, 0), bfExt(0, 0, 4)},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // upper nibble=0, Z=1
		},
	}
	RunM68KTests(t, tests)
}

// === Bug 2: Memory Bit Field Write-Back Position ===

func TestBitField_Memory_BFINS(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name: "BFINS_D1_(A0){0:8}_aligned",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0x55
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xFF00),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFINS, 0), bfExt(1, 0, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectWord(bfTestAddr, 0x5500)},
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0), // inserted 0x55
		},
		{
			Name: "BFINS_D1_(A0){4:8}_cross_byte",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0x00
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xFFFF),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFINS, 0), bfExt(1, 4, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectWord(bfTestAddr, 0xF00F)},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // inserted 0, Z=1
		},
		{
			Name: "BFINS_D1_(A0){3:8}_cross_byte",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0xFF
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0x0000),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFINS, 0), bfExt(1, 3, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectWord(bfTestAddr, 0x1FE0)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // inserted 0xFF
		},
		{
			Name: "BFINS_D1_(A0){0:4}_upper_nibble",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0x05
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x0F),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFINS, 0), bfExt(1, 0, 4)},
			ExpectedMem:   []MemoryExpectation{ExpectByte(bfTestAddr, 0x5F)},
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "BFINS_D1_(A0){0:32}_full_long",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0xDEADBEEF
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint32(0x00000000),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFINS, 0), bfExt(1, 0, 0)}, // width 0 = 32
			ExpectedMem:   []MemoryExpectation{ExpectLong(bfTestAddr, 0xDEADBEEF)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}

// === BFFFO tests ===

func TestBitField_Memory_BFFFO(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFFFO_(A0){0:8}_first_one_at_4",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x0F), // 00001111: first one at offset 4
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFFFO, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 4),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:  "BFFFO_(A0){0:8}_first_one_at_0",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x80), // 10000000: first one at offset 0
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFFFO, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 0),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // MSB=1 → N=1
		},
		{
			Name:  "BFFFO_(A0){0:8}_all_zero",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x00),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFFFO, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 8), // offset + width when no one found
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),
		},
		{
			Name:  "BFFFO_(A0){4:8}_cross_byte",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xABCD),
			},
			// field at {4:8} = 0xBC = 10111100, first one at bit 0 of field
			Opcodes:       []uint16{bfMemEA(bfOp_BFFFO, 0), bfExt(1, 4, 8)},
			ExpectedRegs:  Reg("D1", 4), // offset(4) + firstOne(0) = 4
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}

// === BFSET / BFCLR tests ===

func TestBitField_Memory_BFSET(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFSET_(A0){0:8}_zero_mem",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x00),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFSET, 0), bfExt(0, 0, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectByte(bfTestAddr, 0xFF)},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // original was 0 → Z=1
		},
		{
			Name:  "BFSET_(A0){4:4}_preserves_upper",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xA0), // 10100000
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFSET, 0), bfExt(0, 4, 4)},
			ExpectedMem:   []MemoryExpectation{ExpectByte(bfTestAddr, 0xAF)},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // original lower nibble was 0 → Z=1
		},
		{
			Name:  "BFSET_(A0){3:8}_cross_byte",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0x0000),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFSET, 0), bfExt(0, 3, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectWord(bfTestAddr, 0x1FE0)},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}

func TestBitField_Memory_BFCLR(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFCLR_(A0){0:8}_all_ones",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xFF),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFCLR, 0), bfExt(0, 0, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectByte(bfTestAddr, 0x00)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // original was 0xFF, N=1
		},
		{
			Name:  "BFCLR_(A0){4:4}_preserves_upper",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xFF),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFCLR, 0), bfExt(0, 4, 4)},
			ExpectedMem:   []MemoryExpectation{ExpectByte(bfTestAddr, 0xF0)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // original lower nibble 0xF, bit 3=1
		},
		{
			Name:  "BFCLR_(A0){3:8}_cross_byte",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint16(0xFFFF),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFCLR, 0), bfExt(0, 3, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectWord(bfTestAddr, 0xE01F)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // original was 0xFF
		},
	}
	RunM68KTests(t, tests)
}

// === Bug 3: Register Offset Masked to 5 Bits for Memory ===

func TestBitField_Memory_RegisterOffset(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name: "BFEXTU_(A0){D2:8}_large_positive_offset",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[2] = 100 // byteOffset=12, bitOffset=4
			},
			InitialMem: map[uint32]interface{}{
				// Data at A0+12 (2 bytes for bitOffset=4, width=8)
				bfTestAddr + 12: uint16(0xABCD),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExtRegOff(1, 2, 8)},
			ExpectedRegs:  Reg("D1", 0xBC),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name: "BFEXTU_(A0){D2:8}_zero_offset",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[2] = 0
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x55),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExtRegOff(1, 2, 8)},
			ExpectedRegs:  Reg("D1", 0x55),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "BFINS_D1_(A0){D2:8}_positive_offset",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0xAA
				cpu.DataRegs[2] = 64 // byteOffset=8, bitOffset=0
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr + 8: uint8(0x00),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFINS, 0), bfExtRegOff(1, 2, 8)},
			ExpectedMem:   []MemoryExpectation{ExpectByte(bfTestAddr+8, 0xAA)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // 0xAA bit 7 = 1 → N=1
		},
		{
			Name: "BFEXTU_(d16_A0){D2:8}_negative_offset",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[2] = 0xFFFFFFFB // -5: byteOffset=-1, bitOffset=3
			},
			InitialMem: map[uint32]interface{}{
				// EA = A0+16 = 0x2010, then byteOffset=-1 → read from 0x200F
				// bitOffset=3, width=8 → read 2 bytes from 0x200F
				bfTestAddr + 15: uint16(0xABCD),
			},
			// BFEXTU (16,A0){D2:8},D1 — displacement EA
			Opcodes:       []uint16{bfDispEA(bfOp_BFEXTU, 0), bfExtRegOff(1, 2, 8), 0x0010},
			ExpectedRegs:  Reg("D1", 0x5E), // {3:8} of 0xABCD = 0x5E
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "BFINS_D1_(d16_A0){D2:8}_negative_offset",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0xFF
				cpu.DataRegs[2] = 0xFFFFFFF5 // -11: byteOffset=-2, bitOffset=5
			},
			InitialMem: map[uint32]interface{}{
				// EA = A0+16 = 0x2010, byteOffset=-2 → write at 0x200E
				// bitOffset=5, width=8 → 2 bytes from 0x200E
				bfTestAddr + 14: uint16(0x0000),
			},
			Opcodes: []uint16{bfDispEA(bfOp_BFINS, 0), bfExtRegOff(1, 2, 8), 0x0010},
			// shift = 16 - 5 - 8 = 3, insert 0xFF at shift 3: 0xFF << 3 = 0x07F8
			ExpectedMem:   []MemoryExpectation{ExpectWord(bfTestAddr+14, 0x07F8)},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}

// === Bug 4: 5-Byte Span Not Supported ===

func TestBitField_Memory_5ByteSpan(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFEXTU_(A0){7:32}_5byte_extract",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0x01),
				bfTestAddr + 1: uint8(0x23),
				bfTestAddr + 2: uint8(0x45),
				bfTestAddr + 3: uint8(0x67),
				bfTestAddr + 4: uint8(0x89),
			},
			// fd64 = 0x0123456789, shift = 40-7-32 = 1
			// (0x0123456789 >> 1) & 0xFFFFFFFF = 0x91A2B3C4
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTU, 0), bfExt(1, 7, 0)}, // width 0=32
			ExpectedRegs:  Reg("D1", 0x91A2B3C4),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFTST_(A0){1:32}_5byte_test",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0x01),
				bfTestAddr + 1: uint8(0x23),
				bfTestAddr + 2: uint8(0x45),
				bfTestAddr + 3: uint8(0x67),
				bfTestAddr + 4: uint8(0x89),
			},
			// fd64 = 0x0123456789, shift = 40-1-32 = 7
			// (0x0123456789 >> 7) & 0xFFFFFFFF = 0x02468ACF
			Opcodes:       []uint16{bfMemEA(bfOp_BFTST, 0), bfExt(0, 1, 0)},
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0), // MSB of 0x02468ACF = 0
		},
		{
			Name: "BFINS_D1_(A0){7:32}_5byte_insert",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[1] = 0xDEADBEEF
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0x00),
				bfTestAddr + 1: uint8(0x00),
				bfTestAddr + 2: uint8(0x00),
				bfTestAddr + 3: uint8(0x00),
				bfTestAddr + 4: uint8(0x00),
			},
			// shift=1, 0xDEADBEEF << 1 = 0x01BD5B7DDE (40-bit)
			// byte 0: 0x01, bytes 1-4: 0xBD5B7DDE
			Opcodes: []uint16{bfMemEA(bfOp_BFINS, 0), bfExt(1, 7, 0)},
			ExpectedMem: []MemoryExpectation{
				ExpectByte(bfTestAddr, 0x01),
				ExpectByte(bfTestAddr+1, 0xBD),
				ExpectByte(bfTestAddr+2, 0x5B),
				ExpectByte(bfTestAddr+3, 0x7D),
				ExpectByte(bfTestAddr+4, 0xDE),
			},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // inserted 0xDEADBEEF
		},
		{
			Name:  "BFSET_(A0){3:32}_5byte_set",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0x00),
				bfTestAddr + 1: uint8(0x00),
				bfTestAddr + 2: uint8(0x00),
				bfTestAddr + 3: uint8(0x00),
				bfTestAddr + 4: uint8(0x00),
			},
			// shift=5, mask=0xFFFFFFFF<<5 = sets bits 5-36
			// Result: 0x1FFFFFFFE0 → bytes: 1F FF FF FF E0
			Opcodes: []uint16{bfMemEA(bfOp_BFSET, 0), bfExt(0, 3, 0)},
			ExpectedMem: []MemoryExpectation{
				ExpectByte(bfTestAddr, 0x1F),
				ExpectByte(bfTestAddr+1, 0xFF),
				ExpectByte(bfTestAddr+2, 0xFF),
				ExpectByte(bfTestAddr+3, 0xFF),
				ExpectByte(bfTestAddr+4, 0xE0),
			},
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // original all zeros → Z=1
		},
		{
			Name:  "BFCLR_(A0){5:32}_5byte_clear",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0xFF),
				bfTestAddr + 1: uint8(0xFF),
				bfTestAddr + 2: uint8(0xFF),
				bfTestAddr + 3: uint8(0xFF),
				bfTestAddr + 4: uint8(0xFF),
			},
			// shift=3, clear mask=0xFFFFFFFF<<3 at bits 3-34
			// ~(0xFFFFFFFF<<3) & 0xFFFFFFFFFF = 0xF800000007
			// 0xFFFFFFFFFF & 0xF800000007 = 0xF800000007
			// bytes: F8 00 00 00 07
			Opcodes: []uint16{bfMemEA(bfOp_BFCLR, 0), bfExt(0, 5, 0)},
			ExpectedMem: []MemoryExpectation{
				ExpectByte(bfTestAddr, 0xF8),
				ExpectByte(bfTestAddr+1, 0x00),
				ExpectByte(bfTestAddr+2, 0x00),
				ExpectByte(bfTestAddr+3, 0x00),
				ExpectByte(bfTestAddr+4, 0x07),
			},
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // original field was all 1s
		},
		{
			Name:  "BFCHG_(A0){7:32}_5byte_toggle",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(0xFF),
				bfTestAddr + 1: uint8(0x00),
				bfTestAddr + 2: uint8(0xFF),
				bfTestAddr + 3: uint8(0x00),
				bfTestAddr + 4: uint8(0xFF),
			},
			// fd64 = 0xFF00FF00FF, shift=1
			// original field = (0xFF00FF00FF >> 1) & 0xFFFFFFFF = 0x7F807F80 (wait...)
			// Let me compute: 0xFF00FF00FF >> 1 = 0x7F807F807F, lower 32 = 0x7F807F7F
			// Actually: 0xFF00FF00FF in binary:
			// 11111111 00000000 11111111 00000000 11111111
			// >> 1: 01111111 10000000 01111111 10000000 01111111
			// lower 32: 10000000 01111111 10000000 01111111 = 0x807F807F
			// XOR mask: 0xFFFFFFFF << 1 = 0x1FFFFFFFE
			// fd64 ^ (0x1FFFFFFFE) = 0xFF00FF00FF ^ 0x1FFFFFFFE
			// = 0xFF00FF00FF ^ 0x01FFFFFFFE = 0xFEFF00FF01
			// bytes: FE FF 00 FF 01
			Opcodes: []uint16{bfMemEA(bfOp_BFCHG, 0), bfExt(0, 7, 0)},
			ExpectedMem: []MemoryExpectation{
				ExpectByte(bfTestAddr, 0xFE),
				ExpectByte(bfTestAddr+1, 0xFF),
				ExpectByte(bfTestAddr+2, 0x00),
				ExpectByte(bfTestAddr+3, 0xFF),
				ExpectByte(bfTestAddr+4, 0x01),
			},
			// original field = 0x807F807F → N=1 (MSB=1)
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}

// === Bug 5: MULS.L 64-bit N Flag ===

func TestMulL_64bit(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name: "MULS.L_div_by_10_reciprocal",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 100 // multiplicand in Dl
			},
			// MULS.L #0x66666667, D1:D0
			// ext: Dl=D0(bits14-12=000), signed(bit11), 64bit(bit10), Dh=D1(bits2-0=001)
			Opcodes: []uint16{0x4C3C, 0x0C01, 0x6666, 0x6667},
			ExpectedRegs: Regs(
				"D1", uint32(0x28), // Dh = high word = 40
				"D0", uint32(0x3C), // Dl = low word = 60
			),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "MULS.L_neg_times_pos",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 100 // multiplicand in Dl
			},
			// MULS.L #0xFFFFFFFF (-1), D1:D0
			// ext: Dl=D0(bits14-12=000), signed(bit11), 64bit(bit10), Dh=D1(bits2-0=001)
			Opcodes: []uint16{0x4C3C, 0x0C01, 0xFFFF, 0xFFFF},
			ExpectedRegs: Regs(
				"D1", uint32(0xFFFFFFFF), // Dh = -1 sign extension
				"D0", uint32(0xFFFFFF9C), // Dl = -100
			),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // N=1 (negative result)
		},
		{
			Name: "MULS.L_zero_multiplicand",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0 // multiplicand in Dl
			},
			// ext: Dl=D0(bits14-12=000), signed(bit11), 64bit(bit10), Dh=D1(bits2-0=001)
			Opcodes: []uint16{0x4C3C, 0x0C01, 0x6666, 0x6667},
			ExpectedRegs: Regs(
				"D1", uint32(0),
				"D0", uint32(0),
			),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name: "MULS.L_64bit_N_flag_positive_large",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x7FFFFFFF // large positive in Dl
			},
			// MULS.L #2, D1:D0 → result = 0x00000000_FFFFFFFE
			// ext: Dl=D0(bits14-12=000), signed(bit11), 64bit(bit10), Dh=D1(bits2-0=001)
			Opcodes: []uint16{0x4C3C, 0x0C01, 0x0000, 0x0002},
			ExpectedRegs: Regs(
				"D1", uint32(0x00000000), // Dh = 0 (positive)
				"D0", uint32(0xFFFFFFFE), // Dl has MSB set but result is positive!
			),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0), // N=0 (positive, checks Dh MSB)
		},
		{
			Name: "MULS.L_neg_times_neg",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFFFFFF // -1 in Dl
			},
			// MULS.L #0xFFFFFFFF (-1), D1:D0 → 1
			// ext: Dl=D0(bits14-12=000), signed(bit11), 64bit(bit10), Dh=D1(bits2-0=001)
			Opcodes: []uint16{0x4C3C, 0x0C01, 0xFFFF, 0xFFFF},
			ExpectedRegs: Regs(
				"D1", uint32(0),
				"D0", uint32(1),
			),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0), // positive
		},
		{
			Name: "MULS.L_32bit_overflow",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x7FFFFFFF
			},
			// MULS.L #0x7FFFFFFF, D0 (32-bit form, no 64-bit result)
			// ext: Dl=D0(bits14-12=000), signed(bit11), NO 64bit(bit10=0)
			Opcodes: []uint16{0x4C3C, 0x0800, 0x7FFF, 0xFFFF},
			// Result overflows 32 bits → V=1
			ExpectedFlags: FlagExpectation{N: -1, Z: 0, V: 1, C: 0, X: -1},
		},
	}
	RunM68KTests(t, tests)
}

// === DIVL tests ===

func TestDivL_68020(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name: "DIVS.L_100_div_10",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 100
			},
			// DIVS.L #10, D0 (32÷32, Dr=Dq=D0)
			// ext: Dq=D0(bits15-12=0000), signed(bit11), Dl=D0(bits2-0=000)
			Opcodes:       []uint16{0x4C7C, 0x0800, 0x0000, 0x000A},
			ExpectedRegs:  Reg("D0", 10),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "DIVU.L_100_div_10",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 100
			},
			// DIVU.L #10, D0
			Opcodes:       []uint16{0x4C7C, 0x0000, 0x0000, 0x000A},
			ExpectedRegs:  Reg("D0", 10),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "DIVS.L_with_remainder",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 103
			},
			// DIVS.L #10, D1:D0 (Dr=D1, Dq=D0, 32-bit dividend)
			// ext: Dq=D0(bits15-12=0000), signed(bit11), Dr=D1(bits2-0=001)
			Opcodes: []uint16{0x4C7C, 0x0801, 0x0000, 0x000A},
			ExpectedRegs: Regs(
				"D0", uint32(10), // quotient
				"D1", uint32(3), // remainder
			),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "DIVS.L_negative_dividend",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFFFF9C // -100
			},
			// DIVS.L #10, D1:D0
			Opcodes: []uint16{0x4C7C, 0x0801, 0x0000, 0x000A},
			ExpectedRegs: Regs(
				"D0", uint32(0xFFFFFFF6), // -10
				"D1", uint32(0), // 0 remainder
			),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0), // negative quotient
		},
		{
			Name: "DIVS.L_zero_quotient",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 5
			},
			Opcodes:       []uint16{0x4C7C, 0x0800, 0x0000, 0x000A}, // 5/10 = 0
			ExpectedRegs:  Reg("D0", 0),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
	}
	RunM68KTests(t, tests)
}

// === EXTB.L tests ===

func TestExtBL(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name: "EXTB.L_positive_byte",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFF007F // byte = 0x7F (positive)
			},
			Opcodes:       []uint16{0x49C0}, // EXTB.L D0
			ExpectedRegs:  Reg("D0", 0x0000007F),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name: "EXTB.L_negative_byte",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0x000000FE // byte = 0xFE (-2)
			},
			Opcodes:       []uint16{0x49C0},
			ExpectedRegs:  Reg("D0", 0xFFFFFFFE),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name: "EXTB.L_zero",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[0] = 0xFFFF0000 // byte = 0x00
			},
			Opcodes:       []uint16{0x49C0},
			ExpectedRegs:  Reg("D0", 0x00000000),
			ExpectedFlags: FlagsNZVC(0, 1, 0, 0), // Z=1
		},
		{
			Name: "EXTB.L_D3_0x80",
			Setup: func(cpu *M68KCPU) {
				cpu.DataRegs[3] = 0x12345680 // byte = 0x80 (-128)
			},
			Opcodes:       []uint16{0x49C3}, // EXTB.L D3
			ExpectedRegs:  Reg("D3", 0xFFFFFF80),
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}

// === CMP2.B tests ===

func TestCmp2B(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name: "CMP2.B_in_range",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[0] = 5
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(3),  // lower bound
				bfTestAddr + 1: uint8(10), // upper bound
			},
			// CMP2.B (A0),D0 — opcode = 0x00D0, ext = 0x0000
			Opcodes:       []uint16{0x00D0, 0x0000},
			ExpectedFlags: FlagExpectation{N: -1, Z: 0, V: -1, C: 0, X: -1},
		},
		{
			Name: "CMP2.B_equal_lower",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[0] = 3
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(3),
				bfTestAddr + 1: uint8(10),
			},
			Opcodes:       []uint16{0x00D0, 0x0000},
			ExpectedFlags: FlagExpectation{N: -1, Z: 1, V: -1, C: 0, X: -1},
		},
		{
			Name: "CMP2.B_equal_upper",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[0] = 10
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(3),
				bfTestAddr + 1: uint8(10),
			},
			Opcodes:       []uint16{0x00D0, 0x0000},
			ExpectedFlags: FlagExpectation{N: -1, Z: 1, V: -1, C: 0, X: -1},
		},
		{
			Name: "CMP2.B_below_range",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[0] = 2
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(3),
				bfTestAddr + 1: uint8(10),
			},
			Opcodes:       []uint16{0x00D0, 0x0000},
			ExpectedFlags: FlagExpectation{N: -1, Z: -1, V: -1, C: 1, X: -1},
		},
		{
			Name: "CMP2.B_above_range",
			Setup: func(cpu *M68KCPU) {
				cpu.AddrRegs[0] = bfTestAddr
				cpu.DataRegs[0] = 11
			},
			InitialMem: map[uint32]interface{}{
				bfTestAddr:     uint8(3),
				bfTestAddr + 1: uint8(10),
			},
			Opcodes:       []uint16{0x00D0, 0x0000},
			ExpectedFlags: FlagExpectation{N: -1, Z: -1, V: -1, C: 1, X: -1},
		},
	}
	RunM68KTests(t, tests)
}

// === BFEXTS (signed extraction) ===

func TestBitField_Memory_BFEXTS(t *testing.T) {
	tests := []M68KTestCase{
		{
			Name:  "BFEXTS_(A0){0:8}_negative",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xFE), // -2 as 8-bit signed
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTS, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 0xFFFFFFFE), // sign-extended to 32 bits
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
		{
			Name:  "BFEXTS_(A0){0:8}_positive",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0x7F),
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTS, 0), bfExt(1, 0, 8)},
			ExpectedRegs:  Reg("D1", 0x0000007F),
			ExpectedFlags: FlagsNZVC(0, 0, 0, 0),
		},
		{
			Name:  "BFEXTS_(A0){4:4}_negative",
			Setup: func(cpu *M68KCPU) { cpu.AddrRegs[0] = bfTestAddr },
			InitialMem: map[uint32]interface{}{
				bfTestAddr: uint8(0xAB), // nibble at {4:4} = 0xB = 1011, MSB=1 → negative
			},
			Opcodes:       []uint16{bfMemEA(bfOp_BFEXTS, 0), bfExt(1, 4, 4)},
			ExpectedRegs:  Reg("D1", 0xFFFFFFFB), // -5
			ExpectedFlags: FlagsNZVC(1, 0, 0, 0),
		},
	}
	RunM68KTests(t, tests)
}
