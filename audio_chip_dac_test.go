//go:build headless

package main

import (
	"math"
	"testing"
)

// newTestSoundChipForDAC creates a minimal SoundChip suitable for DAC tests.
func newTestSoundChipForDAC() *SoundChip {
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	return chip
}

func TestFlexChannelDACMode(t *testing.T) {
	chip := newTestSoundChipForDAC()
	ch := chip.channels[0]

	// Enable channel with gate
	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3)  // enable + gate
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 255) // full volume

	// Write a positive signed value to DAC register
	// int8(100) -> dacValue = 100/127.0 in symmetric DAC mode.
	chip.applyFlexRegister(0, FLEX_OFF_DAC, uint32(byte(100)))

	if !ch.dacMode {
		t.Fatal("expected dacMode to be true after writing FLEX_OFF_DAC")
	}

	expected := float32(100) / 127.0
	if math.Abs(float64(ch.dacValue-expected)) > 0.001 {
		t.Fatalf("expected dacValue ≈ %f, got %f", expected, ch.dacValue)
	}

	sample := ch.generateSample()
	// output = dacValue * volume.
	if math.Abs(float64(sample-expected)) > 0.001 {
		t.Fatalf("expected sample ≈ %f, got %f", expected, sample)
	}

	// Test with a negative signed value
	// byte(0x80) = 128, int8(0x80) = -128 → dacValue = -128/128.0 = -1.0
	chip.applyFlexRegister(0, FLEX_OFF_DAC, 0x80)
	expectedNeg := float32(-1.0)
	if math.Abs(float64(ch.dacValue-expectedNeg)) > 0.001 {
		t.Fatalf("expected dacValue = %f, got %f", expectedNeg, ch.dacValue)
	}

	sample = ch.generateSample()
	if math.Abs(float64(sample-expectedNeg)) > 0.001 {
		t.Fatalf("expected sample = %f, got %f", expectedNeg, sample)
	}
}

func TestFlexChannelDACModeZeroCentered(t *testing.T) {
	chip := newTestSoundChipForDAC()
	ch := chip.channels[0]

	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3)
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 255)

	// Write alternating -64/+64 values and average
	var sum float64
	const iterations = 1000
	for i := range iterations {
		if i%2 == 0 {
			// int8(-64) = byte(0xC0) = 192
			chip.applyFlexRegister(0, FLEX_OFF_DAC, 0xC0)
		} else {
			chip.applyFlexRegister(0, FLEX_OFF_DAC, uint32(byte(64)))
		}
		sum += float64(ch.generateSample())
	}
	avg := sum / float64(iterations)
	if math.Abs(avg) > 0.01 {
		t.Fatalf("expected average ≈ 0 (zero-centered), got %f", avg)
	}
}

func TestFlexChannelDACModeExitOnWaveType(t *testing.T) {
	chip := newTestSoundChipForDAC()
	ch := chip.channels[0]

	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3)
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 255)

	// Enter DAC mode
	chip.applyFlexRegister(0, FLEX_OFF_DAC, uint32(byte(100)))
	if !ch.dacMode {
		t.Fatal("expected dacMode to be true")
	}

	// Writing wave type should clear DAC mode
	chip.applyFlexRegister(0, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
	if ch.dacMode {
		t.Fatal("expected dacMode to be false after writing FLEX_OFF_WAVE_TYPE")
	}
}

func TestFlexChannelDACModeWithVolume(t *testing.T) {
	chip := newTestSoundChipForDAC()
	ch := chip.channels[0]

	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3)
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 128) // 50% volume

	// int8(127) -> dacValue = 1.0.
	chip.applyFlexRegister(0, FLEX_OFF_DAC, 127)

	sample := ch.generateSample()
	// output = dacValue * volume.
	expectedVol := float32(128) / 255.0
	expected := expectedVol
	if math.Abs(float64(sample-expected)) > 0.01 {
		t.Fatalf("expected sample ≈ %f, got %f", expected, sample)
	}
}

func TestFlexChannelDACModeBypassesEnvelope(t *testing.T) {
	chip := newTestSoundChipForDAC()
	ch := chip.channels[0]

	// Set a very slow attack (255ms) so envelope would ramp slowly
	chip.applyFlexRegister(0, FLEX_OFF_ATK, 255)
	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3) // enable + gate (triggers attack)
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 255)

	// Enter DAC mode
	chip.applyFlexRegister(0, FLEX_OFF_DAC, 100)

	// Generate a sample immediately — if envelope were used, output would be
	// near 0 because attack just started. DAC mode should give full output.
	sample := ch.generateSample()
	expected := float32(100) / 127.0
	if math.Abs(float64(sample-expected)) > 0.01 {
		t.Fatalf("expected immediate output ≈ %f (envelope bypassed), got %f", expected, sample)
	}
}

func TestFlexChannelDACModeNoFrequencyNeeded(t *testing.T) {
	chip := newTestSoundChipForDAC()
	ch := chip.channels[0]

	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3)
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 255)
	// Explicitly set frequency to 0
	chip.applyFlexRegister(0, FLEX_OFF_FREQ, 0)

	// Enter DAC mode with a non-zero value
	chip.applyFlexRegister(0, FLEX_OFF_DAC, 100)

	sample := ch.generateSample()
	expected := float32(100) / 127.0
	if sample == 0 {
		t.Fatal("expected non-zero output with frequency=0 in DAC mode")
	}
	if math.Abs(float64(sample-expected)) > 0.001 {
		t.Fatalf("expected sample ≈ %f, got %f", expected, sample)
	}
}

func TestFlexChannelDACModeMixerNoDCBias(t *testing.T) {
	chip := newTestSoundChipForDAC()

	// Set up all 4 channels in DAC mode with symmetric data
	for i := range NUM_CHANNELS {
		chip.applyFlexRegister(uint32(i), FLEX_OFF_CTRL, 3)
		chip.applyFlexRegister(uint32(i), FLEX_OFF_VOL, 255)
	}

	// Write symmetric values: ch0=+64, ch1=-64, ch2=+32, ch3=-32
	values := []uint32{64, 0xC0, 32, 0xE0} // 0xC0=int8(-64), 0xE0=int8(-32)
	for i, v := range values {
		chip.applyFlexRegister(uint32(i), FLEX_OFF_DAC, v)
	}

	// Sum should be 0 since values cancel out
	var sum float32
	for i := range NUM_CHANNELS {
		sum += chip.channels[i].generateSample()
	}
	if math.Abs(float64(sum)) > 0.01 {
		t.Fatalf("expected mixer sum ≈ 0 (no DC bias), got %f", sum)
	}
}
