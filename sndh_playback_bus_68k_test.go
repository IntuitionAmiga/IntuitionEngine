// sndh_68k_bus_test.go - Tests and benchmarks for SNDH 68K memory bus

package main

import (
	"encoding/binary"
	"testing"
)

// =============================================================================
// Regression tests for correctness
// =============================================================================

func TestSNDH68K_Read8_RAM(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.memory[0x1000] = 0x42
	if got := bus.Read8(0x1000); got != 0x42 {
		t.Errorf("Read8 = 0x%02X, want 0x42", got)
	}
}

func TestSNDH68K_Write8_RAM(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write8(0x1000, 0xAB)
	if bus.memory[0x1000] != 0xAB {
		t.Errorf("memory[0x1000] = 0x%02X, want 0xAB", bus.memory[0x1000])
	}
}

func TestSNDH68K_Read16_Correctness(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write16(0x1000, 0xABCD)
	if got := bus.Read16(0x1000); got != 0xABCD {
		t.Errorf("Read16 = 0x%04X, want 0xABCD", got)
	}
}

func TestSNDH68K_Read32_Correctness(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write32(0x1000, 0x12345678)
	if got := bus.Read32(0x1000); got != 0x12345678 {
		t.Errorf("Read32 = 0x%08X, want 0x12345678", got)
	}
}

func TestSNDH68K_ByteOrder_16bit(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write16(0x1000, 0xABCD)
	// Verify little-endian byte order
	if bus.memory[0x1000] != 0xCD || bus.memory[0x1001] != 0xAB {
		t.Errorf("Byte order incorrect: got %02X %02X, want CD AB",
			bus.memory[0x1000], bus.memory[0x1001])
	}
}

func TestSNDH68K_ByteOrder_32bit(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write32(0x1000, 0x12345678)
	// Verify little-endian byte order
	if bus.memory[0x1000] != 0x78 || bus.memory[0x1001] != 0x56 ||
		bus.memory[0x1002] != 0x34 || bus.memory[0x1003] != 0x12 {
		t.Errorf("Byte order incorrect: got %02X %02X %02X %02X, want 78 56 34 12",
			bus.memory[0x1000], bus.memory[0x1001], bus.memory[0x1002], bus.memory[0x1003])
	}
}

func TestSNDH68K_Read16_AllValues(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	testCases := []uint16{0, 1, 0xFF, 0x100, 0xFFFF, 0xABCD, 0x1234}
	for _, want := range testCases {
		bus.Write16(0x1000, want)
		if got := bus.Read16(0x1000); got != want {
			t.Errorf("Read16 = 0x%04X, want 0x%04X", got, want)
		}
	}
}

func TestSNDH68K_Read32_AllValues(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	testCases := []uint32{0, 1, 0xFF, 0xFFFF, 0xFFFFFF, 0xFFFFFFFF, 0x12345678, 0xDEADBEEF}
	for _, want := range testCases {
		bus.Write32(0x1000, want)
		if got := bus.Read32(0x1000); got != want {
			t.Errorf("Read32 = 0x%08X, want 0x%08X", got, want)
		}
	}
}

func TestSNDH68K_YMRegisterStillWorks(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write8(YM_REG_SELECT&0xFFFFFF, 0x07) // Select mixer register
	bus.Write8(YM_REG_DATA&0xFFFFFF, 0x38)   // Set value

	if bus.regSelect != 0x07 {
		t.Errorf("regSelect = %d, want 7", bus.regSelect)
	}
	if bus.regs[7] != 0x38 {
		t.Errorf("regs[7] = 0x%02X, want 0x38", bus.regs[7])
	}
	if len(bus.writes) != 1 {
		t.Errorf("writes = %d, want 1", len(bus.writes))
	}
}

func TestSNDH68K_YMRegisterWord(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	// Word write to $FF8800 selects register and writes data
	// The bus expects little-endian input and swaps to big-endian
	// For reg 7 with value 0x55: pass 0x5507 (little-endian: 0x07, 0x55)
	// After swap: big-endian 0x0755 -> reg=0x07, data=0x55
	bus.Write16(YM_REG_SELECT, 0x5507)

	if len(bus.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(bus.writes))
	}
	if bus.writes[0].Reg != 0x07 {
		t.Errorf("write reg = %d, want 7", bus.writes[0].Reg)
	}
	if bus.writes[0].Value != 0x55 {
		t.Errorf("write value = 0x%02X, want 0x55", bus.writes[0].Value)
	}
}

func TestSNDH68K_MFPRegisters(t *testing.T) {
	bus := newSndhPlaybackBus68K()

	// Write to timer A data register
	bus.Write8(MFP_TADR, 0x50)
	if bus.timerA.data != 0x50 {
		t.Errorf("timerA.data = 0x%02X, want 0x50", bus.timerA.data)
	}

	// Read it back
	if got := bus.Read8(MFP_TADR); got != 0x50 {
		t.Errorf("Read MFP_TADR = 0x%02X, want 0x50", got)
	}
}

func TestSNDH68K_AddressMasking(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	// Address should be masked to 24-bit for Atari ST compatibility
	bus.Write8(0x01001000, 0xAA) // High byte should be ignored
	if bus.memory[0x1000] != 0xAA {
		t.Errorf("Address masking failed")
	}
}

func TestSNDH68K_BoundsCheck(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	// Should not panic at boundary
	bus.Write8(SNDH_BUS_SIZE-1, 0xFF)
	if got := bus.Read8(SNDH_BUS_SIZE - 1); got != 0xFF {
		t.Errorf("Boundary read = 0x%02X, want 0xFF", got)
	}
}

func TestSNDH68K_LoadSNDH(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	data := []byte{0x60, 0x00, 0x00, 0x0C} // BRA.S +12
	bus.LoadSNDH(data)

	for i, v := range data {
		if bus.memory[i] != v {
			t.Errorf("memory[%d] = 0x%02X, want 0x%02X", i, bus.memory[i], v)
		}
	}
}

func TestSNDH68K_Reset(t *testing.T) {
	bus := newSndhPlaybackBus68K()
	bus.Write8(0x1000, 0xFF)
	bus.regSelect = 5
	bus.regs[5] = 0xAA
	bus.AddCycles(1000)

	bus.Reset()

	if bus.memory[0x1000] != 0 {
		t.Error("memory not cleared after reset")
	}
	if bus.regSelect != 0 {
		t.Error("regSelect not cleared after reset")
	}
	if bus.regs[5] != 0 {
		t.Error("regs not cleared after reset")
	}
	if bus.cycles != 0 {
		t.Error("cycles not cleared after reset")
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkSNDH68K_Read8_RAM(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	bus.memory[0x1000] = 0x42
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read8(0x1000)
	}
}

func BenchmarkSNDH68K_Write8_RAM(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write8(0x1000, uint8(i))
	}
}

func BenchmarkSNDH68K_Read16_RAM(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	binary.LittleEndian.PutUint16(bus.memory[0x1000:], 0x1234)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read16(0x1000)
	}
}

func BenchmarkSNDH68K_Write16_RAM(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write16(0x1000, uint16(i))
	}
}

func BenchmarkSNDH68K_Read32_RAM(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	binary.LittleEndian.PutUint32(bus.memory[0x1000:], 0x12345678)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read32(0x1000)
	}
}

func BenchmarkSNDH68K_Write32_RAM(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write32(0x1000, uint32(i))
	}
}

func BenchmarkSNDH68K_Read8_YMRegion(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	bus.regSelect = 7
	bus.regs[7] = 0x38
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read8(YM_REG_SELECT & 0xFFFFFF)
	}
}

func BenchmarkSNDH68K_Write8_YMRegion(b *testing.B) {
	bus := newSndhPlaybackBus68K()
	bus.regSelect = 7
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write8(YM_REG_DATA&0xFFFFFF, uint8(i))
	}
}
