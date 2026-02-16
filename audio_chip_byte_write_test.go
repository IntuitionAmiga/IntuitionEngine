// audio_chip_byte_write_test.go - Tests for byte-level flex register writes

package main

import (
	"math"
	"testing"
)

// TestFlexRegisterByteWrites exercises the Write8 accumulation path for flex registers.
func TestFlexRegisterByteWrites(t *testing.T) {
	t.Run("Write8x4_Frequency", func(t *testing.T) {
		// Write 0x00011800 (= 280.375 Hz in 16.8 fixed-point: 71680 / 256) via 4 byte writes
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		// Enable the chip
		bus.Write32(AUDIO_CTRL, 1)

		// Write frequency register byte-by-byte (LE order)
		addr := uint32(FLEX_CH0_BASE + FLEX_OFF_FREQ)
		value := uint32(0x00011800) // 71680 → 280.0 Hz
		bus.Write8(addr, uint8(value))
		bus.Write8(addr+1, uint8(value>>8))
		bus.Write8(addr+2, uint8(value>>16))

		// Channel frequency should NOT be set yet (only 3 of 4 bytes written)
		if chip.channels[0].frequency != 0 {
			t.Errorf("frequency set before 4th byte: got %f", chip.channels[0].frequency)
		}

		bus.Write8(addr+3, uint8(value>>24))

		// Now the full value should be assembled and applied
		expected := float32(value) / 256.0
		if math.Abs(float64(chip.channels[0].frequency-expected)) > 0.01 {
			t.Errorf("frequency mismatch: got %f, want %f", chip.channels[0].frequency, expected)
		}
	})

	t.Run("Write8x4_Volume", func(t *testing.T) {
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		bus.Write32(AUDIO_CTRL, 1)

		// Write volume = 200 (0x000000C8) via byte writes
		addr := uint32(FLEX_CH0_BASE + FLEX_OFF_VOL)
		bus.Write8(addr, 0xC8)
		bus.Write8(addr+1, 0x00)
		bus.Write8(addr+2, 0x00)
		bus.Write8(addr+3, 0x00)

		expected := float32(200) / NORMALISE_8BIT
		if math.Abs(float64(chip.channels[0].volume-expected)) > 0.001 {
			t.Errorf("volume mismatch: got %f, want %f", chip.channels[0].volume, expected)
		}
	})

	t.Run("Write32_Then_Write8x4_ShadowSync", func(t *testing.T) {
		// Verify that Write32 syncs the shadow buffer so a subsequent Write8×4
		// doesn't use stale shadow bytes from before the Write32.
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		bus.Write32(AUDIO_CTRL, 1)

		// First: Write32 a known frequency value
		addr := uint32(FLEX_CH0_BASE + FLEX_OFF_FREQ)
		bus.Write32(addr, 0x00011800) // 280.0 Hz

		first := chip.channels[0].frequency
		if first == 0 {
			t.Fatal("Write32 did not set frequency")
		}

		// Second: Write8×4 a different frequency value
		newValue := uint32(0x00023000) // 143360 → 560.0 Hz
		bus.Write8(addr, uint8(newValue))
		bus.Write8(addr+1, uint8(newValue>>8))
		bus.Write8(addr+2, uint8(newValue>>16))
		bus.Write8(addr+3, uint8(newValue>>24))

		expected := float32(newValue) / 256.0
		if math.Abs(float64(chip.channels[0].frequency-expected)) > 0.01 {
			t.Errorf("frequency mismatch after Write32+Write8x4: got %f, want %f",
				chip.channels[0].frequency, expected)
		}
	})

	t.Run("Write16x2_Frequency", func(t *testing.T) {
		// Write frequency via two Write16 calls (decomposed into 4 byte writes by bus)
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		bus.Write32(AUDIO_CTRL, 1)

		addr := uint32(FLEX_CH0_BASE + FLEX_OFF_FREQ)
		value := uint32(0x00011800) // 280.0 Hz

		bus.Write16(addr, uint16(value))       // Low 16 bits
		bus.Write16(addr+2, uint16(value>>16)) // High 16 bits

		expected := float32(value) / 256.0
		if math.Abs(float64(chip.channels[0].frequency-expected)) > 0.01 {
			t.Errorf("frequency mismatch via Write16x2: got %f, want %f",
				chip.channels[0].frequency, expected)
		}
	})

	t.Run("Write8x4_Channel1", func(t *testing.T) {
		// Verify byte writes to channel 1 work correctly
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		bus.Write32(AUDIO_CTRL, 1)

		addr := uint32(FLEX_CH1_BASE + FLEX_OFF_FREQ)
		value := uint32(0x00006E00) // 28160 → 110.0 Hz
		bus.Write8(addr, uint8(value))
		bus.Write8(addr+1, uint8(value>>8))
		bus.Write8(addr+2, uint8(value>>16))
		bus.Write8(addr+3, uint8(value>>24))

		expected := float32(value) / 256.0
		if math.Abs(float64(chip.channels[1].frequency-expected)) > 0.01 {
			t.Errorf("ch1 frequency mismatch: got %f, want %f",
				chip.channels[1].frequency, expected)
		}
	})

	t.Run("Write8_NonFlex_AUDIO_CTRL", func(t *testing.T) {
		// Verify that byte writes to AUDIO_CTRL delegate to HandleRegisterWrite
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		// Enable via byte write
		bus.Write8(AUDIO_CTRL, 1)

		if !chip.enabled.Load() {
			t.Error("AUDIO_CTRL byte write did not enable chip")
		}

		// Disable via byte write
		bus.Write8(AUDIO_CTRL, 0)

		if chip.enabled.Load() {
			t.Error("AUDIO_CTRL byte write did not disable chip")
		}
	})

	t.Run("Write8x4_WaveType", func(t *testing.T) {
		chip := newTestSoundChip()
		bus := NewMachineBus()

		bus.MapIO(AUDIO_CTRL, AUDIO_REG_END,
			chip.HandleRegisterRead,
			chip.HandleRegisterWrite)
		bus.MapIOByte(AUDIO_CTRL, AUDIO_REG_END, chip.HandleRegisterWrite8)

		bus.Write32(AUDIO_CTRL, 1)

		// Write wave type = WAVE_SAWTOOTH (4) via byte writes
		addr := uint32(FLEX_CH0_BASE + FLEX_OFF_WAVE_TYPE)
		bus.Write8(addr, uint8(WAVE_SAWTOOTH))
		bus.Write8(addr+1, 0)
		bus.Write8(addr+2, 0)
		bus.Write8(addr+3, 0)

		if chip.channels[0].waveType != WAVE_SAWTOOTH {
			t.Errorf("wave type mismatch: got %d, want %d",
				chip.channels[0].waveType, WAVE_SAWTOOTH)
		}
	})
}
