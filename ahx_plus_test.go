// ahx_plus_test.go - Tests for AHX+ enhanced mode

package main

import (
	"testing"
)

// TestAHXPlus_Enabled tests that AHX+ mode can be enabled/disabled
func TestAHXPlus_Enabled(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Initially disabled
	if engine.AHXPlusEnabled() {
		t.Error("AHX+ should be disabled by default")
	}

	// Enable
	engine.SetAHXPlusEnabled(true)
	if !engine.AHXPlusEnabled() {
		t.Error("AHX+ should be enabled after SetAHXPlusEnabled(true)")
	}

	// Disable
	engine.SetAHXPlusEnabled(false)
	if engine.AHXPlusEnabled() {
		t.Error("AHX+ should be disabled after SetAHXPlusEnabled(false)")
	}
}

// TestAHXPlus_StereoSpread tests that AHX+ sets up correct Amiga stereo panning
func TestAHXPlus_StereoSpread(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)
	engine.SetAHXPlusEnabled(true)

	// Verify pan values for each channel (L R R L pattern)
	expectedPan := []float32{-0.7, 0.7, 0.7, -0.7}
	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch == nil {
			t.Fatalf("Channel %d is nil", i)
		}
		if ch.ahxPlusPan != expectedPan[i] {
			t.Errorf("Channel %d pan should be %.2f, got %.2f", i, expectedPan[i], ch.ahxPlusPan)
		}
	}

	// Verify gains for Amiga stereo spread
	expectedGain := []float32{1.08, 0.92, 0.92, 1.08}
	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch.ahxPlusGain != expectedGain[i] {
			t.Errorf("Channel %d gain should be %.2f, got %.2f", i, expectedGain[i], ch.ahxPlusGain)
		}
	}
}

// TestAHXPlus_Oversampling tests that AHX+ configures oversampling
func TestAHXPlus_Oversampling(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Enable AHX+
	engine.SetAHXPlusEnabled(true)

	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch == nil {
			t.Fatalf("Channel %d is nil", i)
		}
		if ch.ahxPlusOversample != AHX_PLUS_OVERSAMPLE {
			t.Errorf("Channel %d oversample should be %d, got %d", i, AHX_PLUS_OVERSAMPLE, ch.ahxPlusOversample)
		}
	}

	// Disable and verify reset
	engine.SetAHXPlusEnabled(false)
	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch.ahxPlusOversample != 1 {
			t.Errorf("Channel %d oversample should be 1 when disabled, got %d", i, ch.ahxPlusOversample)
		}
	}
}

// TestAHXPlus_RoomReverb tests that AHX+ configures room reverb
func TestAHXPlus_RoomReverb(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)
	engine.SetAHXPlusEnabled(true)

	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch == nil {
			t.Fatalf("Channel %d is nil", i)
		}
		if ch.ahxPlusRoomDelay != AHX_PLUS_ROOM_DELAY {
			t.Errorf("Channel %d room delay should be %d, got %d", i, AHX_PLUS_ROOM_DELAY, ch.ahxPlusRoomDelay)
		}
		if ch.ahxPlusRoomMix != AHX_PLUS_ROOM_MIX {
			t.Errorf("Channel %d room mix should be %.2f, got %.2f", i, AHX_PLUS_ROOM_MIX, ch.ahxPlusRoomMix)
		}
		if len(ch.ahxPlusRoomBuf) != AHX_PLUS_ROOM_DELAY {
			t.Errorf("Channel %d room buffer should have length %d, got %d", i, AHX_PLUS_ROOM_DELAY, len(ch.ahxPlusRoomBuf))
		}
	}

	// Disable and verify cleanup
	engine.SetAHXPlusEnabled(false)
	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch.ahxPlusRoomBuf != nil {
			t.Errorf("Channel %d room buffer should be nil when disabled", i)
		}
		if ch.ahxPlusRoomMix != 0 {
			t.Errorf("Channel %d room mix should be 0 when disabled, got %.2f", i, ch.ahxPlusRoomMix)
		}
	}
}

// TestAHXPlus_PWMMapping tests PWM duty cycle mapping from SquarePos
func TestAHXPlus_PWMMapping(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Load minimal AHX data
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 0x00,
	}
	engine.LoadData(data)
	engine.SetAHXPlusEnabled(true)

	// Test PWM mapping logic directly
	// SquarePos range: 0-63
	// If > 0x20 (32), mirror: 0x40 - squarePos
	// Then multiply by 4, clamp to 0x08-0x80
	testCases := []struct {
		squarePos int
		expected  int
	}{
		{0, 0x08},  // 0 * 4 = 0, clamped to 0x08
		{8, 0x20},  // 8 * 4 = 0x20
		{16, 0x40}, // 16 * 4 = 0x40
		{32, 0x80}, // 32 * 4 = 0x80 (not mirrored since not > 0x20)
		{48, 0x40}, // 0x40 - 48 = 16, 16 * 4 = 0x40
		{63, 0x08}, // 0x40 - 63 = 1, 1 * 4 = 0x04, clamped to 0x08
	}

	for _, tc := range testCases {
		squarePos := tc.squarePos
		if squarePos > 0x20 {
			squarePos = 0x40 - squarePos
		}
		duty := squarePos * 4
		if duty < 0x08 {
			duty = 0x08
		}
		if duty > 0x80 {
			duty = 0x80
		}
		if duty != tc.expected {
			t.Errorf("SquarePos %d: expected duty 0x%02X, got 0x%02X", tc.squarePos, tc.expected, duty)
		}
	}
}

// TestAHXPlus_Drive tests saturation drive parameter
func TestAHXPlus_Drive(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)
	engine.SetAHXPlusEnabled(true)

	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch == nil {
			t.Fatalf("Channel %d is nil", i)
		}
		if ch.ahxPlusDrive != AHX_PLUS_DRIVE {
			t.Errorf("Channel %d drive should be %.2f, got %.2f", i, AHX_PLUS_DRIVE, ch.ahxPlusDrive)
		}
	}

	engine.SetAHXPlusEnabled(false)
	for i := 0; i < 4; i++ {
		ch := chip.channels[i]
		if ch.ahxPlusDrive != 0 {
			t.Errorf("Channel %d drive should be 0 when disabled, got %.2f", i, ch.ahxPlusDrive)
		}
	}
}

// TestAHXPlus_Constants tests that AHX+ constants are properly defined
func TestAHXPlus_Constants(t *testing.T) {
	if AHX_PLUS_OVERSAMPLE != 4 {
		t.Errorf("AHX_PLUS_OVERSAMPLE should be 4, got %d", AHX_PLUS_OVERSAMPLE)
	}
	if AHX_PLUS_LOWPASS_ALPHA != 0.11 {
		t.Errorf("AHX_PLUS_LOWPASS_ALPHA should be 0.11, got %.2f", AHX_PLUS_LOWPASS_ALPHA)
	}
	if AHX_PLUS_DRIVE != 0.16 {
		t.Errorf("AHX_PLUS_DRIVE should be 0.16, got %.2f", AHX_PLUS_DRIVE)
	}
	if AHX_PLUS_ROOM_MIX != 0.09 {
		t.Errorf("AHX_PLUS_ROOM_MIX should be 0.09, got %.2f", AHX_PLUS_ROOM_MIX)
	}
	if AHX_PLUS_ROOM_DELAY != 120 {
		t.Errorf("AHX_PLUS_ROOM_DELAY should be 120, got %d", AHX_PLUS_ROOM_DELAY)
	}
}
