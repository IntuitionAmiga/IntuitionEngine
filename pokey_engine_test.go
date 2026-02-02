// pokey_engine_test.go - Tests for POKEY sound chip emulation

package main

import (
	"testing"
)

func TestPOKEYEngine_NewEngine(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)
	if engine == nil {
		t.Fatal("NewPOKEYEngine returned nil")
	}
	if engine.clockHz != POKEY_CLOCK_NTSC {
		t.Errorf("expected clock %d, got %d", POKEY_CLOCK_NTSC, engine.clockHz)
	}
	if engine.sampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", engine.sampleRate)
	}
}

func TestPOKEYEngine_SetClockHz(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	engine.SetClockHz(POKEY_CLOCK_PAL)
	if engine.clockHz != POKEY_CLOCK_PAL {
		t.Errorf("expected PAL clock %d, got %d", POKEY_CLOCK_PAL, engine.clockHz)
	}

	// Zero clock should be ignored
	engine.SetClockHz(0)
	if engine.clockHz != POKEY_CLOCK_PAL {
		t.Errorf("zero clock should be ignored, got %d", engine.clockHz)
	}
}

func TestPOKEYEngine_WriteRegister(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	// Write to AUDF1
	engine.WriteRegister(0, 0x50)
	if engine.regs[0] != 0x50 {
		t.Errorf("expected AUDF1=0x50, got 0x%02X", engine.regs[0])
	}

	// Write to AUDC1
	engine.WriteRegister(1, 0xAF)
	if engine.regs[1] != 0xAF {
		t.Errorf("expected AUDC1=0xAF, got 0x%02X", engine.regs[1])
	}

	// Write to AUDCTL
	engine.WriteRegister(8, AUDCTL_CH1_179MHZ)
	if engine.regs[8] != AUDCTL_CH1_179MHZ {
		t.Errorf("expected AUDCTL=0x%02X, got 0x%02X", AUDCTL_CH1_179MHZ, engine.regs[8])
	}

	// Out of bounds register should be ignored
	engine.WriteRegister(20, 0xFF)
	// Should not panic
}

func TestPOKEYEngine_HandleIO(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	// Write via memory-mapped I/O
	engine.HandleWrite(POKEY_AUDF1, 0x30)
	if engine.regs[0] != 0x30 {
		t.Errorf("HandleWrite failed: expected 0x30, got 0x%02X", engine.regs[0])
	}

	// Read via memory-mapped I/O
	value := engine.HandleRead(POKEY_AUDF1)
	if value != 0x30 {
		t.Errorf("HandleRead failed: expected 0x30, got 0x%02X", value)
	}

	// Out of range addresses should be ignored
	engine.HandleWrite(0xF0E00, 0xFF)
	value = engine.HandleRead(0xF0E00)
	if value != 0 {
		t.Errorf("out of range read should return 0, got 0x%02X", value)
	}
}

func TestPOKEYEngine_FrequencyCalculation(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	tests := []struct {
		name    string
		audf    uint8
		audctl  uint8
		channel int
		minFreq float64
		maxFreq float64
	}{
		{
			name:    "Ch1 64kHz base, AUDF=60",
			audf:    60,
			audctl:  0,
			channel: 0,
			minFreq: 500,
			maxFreq: 600,
		},
		{
			name:    "Ch1 1.79MHz, AUDF=60",
			audf:    60,
			audctl:  AUDCTL_CH1_179MHZ,
			channel: 0,
			minFreq: 14000,
			maxFreq: 15000,
		},
		{
			name:    "Ch1 15kHz base, AUDF=60",
			audf:    60,
			audctl:  AUDCTL_CLOCK_15KHZ,
			channel: 0,
			minFreq: 100,
			maxFreq: 150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.Reset()
			engine.regs[tt.channel*2] = tt.audf
			engine.regs[8] = tt.audctl

			freq := engine.calcFrequency(tt.channel)
			if freq < tt.minFreq || freq > tt.maxFreq {
				t.Errorf("frequency %f not in range [%f, %f]", freq, tt.minFreq, tt.maxFreq)
			}
		})
	}
}

func TestPOKEYEngine_16BitMode(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	// Set up 16-bit mode: Ch1+Ch2 linked
	engine.regs[0] = 0x10 // AUDF1 (low byte)
	engine.regs[2] = 0x01 // AUDF2 (high byte)
	engine.regs[8] = AUDCTL_CH2_BY_CH1

	freq := engine.calcFrequency(1) // Channel 2 in 16-bit mode
	if freq <= 0 {
		t.Error("16-bit mode frequency calculation failed")
	}

	// Combined period = 0x0110 = 272
	// Expected freq = 64kHz / (2 * 273) ≈ 117 Hz
	if freq < 100 || freq > 130 {
		t.Errorf("16-bit frequency %f not in expected range", freq)
	}
}

func TestPOKEYEngine_POKEYPlus(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	if engine.POKEYPlusEnabled() {
		t.Error("POKEY+ should be disabled by default")
	}

	engine.SetPOKEYPlusEnabled(true)
	if !engine.POKEYPlusEnabled() {
		t.Error("POKEY+ should be enabled after SetPOKEYPlusEnabled(true)")
	}

	// Test via register write
	engine.WriteRegister(9, 0) // POKEY_PLUS_CTRL offset
	if engine.POKEYPlusEnabled() {
		t.Error("POKEY+ should be disabled after writing 0 to control register")
	}

	engine.WriteRegister(9, 1)
	if !engine.POKEYPlusEnabled() {
		t.Error("POKEY+ should be enabled after writing 1 to control register")
	}
}

func TestPOKEYEngine_VolumeGain(t *testing.T) {
	// Test linear volume (standard POKEY)
	for level := uint8(0); level <= 15; level++ {
		gain := pokeyVolumeGain(level, false)
		expected := float32(level) / 15.0
		if gain != expected {
			t.Errorf("linear gain for level %d: expected %f, got %f", level, expected, gain)
		}
	}

	// Test logarithmic volume (POKEY+)
	// Level 0 should be 0
	if pokeyVolumeGain(0, true) != 0 {
		t.Error("POKEY+ level 0 should be 0")
	}
	// Level 15 should be 1.0
	if pokeyVolumeGain(15, true) != 1.0 {
		t.Error("POKEY+ level 15 should be 1.0")
	}
	// Mid levels should be logarithmic (quieter than linear)
	linearMid := pokeyVolumeGain(8, false)
	logMid := pokeyVolumeGain(8, true)
	if logMid >= linearMid {
		t.Errorf("POKEY+ mid-level should be quieter: linear=%f, log=%f", linearMid, logMid)
	}
}

func TestPOKEYEngine_GainToDAC(t *testing.T) {
	tests := []struct {
		gain     float32
		expected int // Use int to avoid uint8 underflow in comparisons
	}{
		{0.0, 0},
		{0.5, 128},
		{1.0, 255},
		{-0.1, 0},
		{1.5, 255},
	}

	for _, tt := range tests {
		result := int(pokeyGainToDAC(tt.gain))
		// Allow ±1 for rounding
		minExpected := tt.expected - 1
		if minExpected < 0 {
			minExpected = 0
		}
		maxExpected := tt.expected + 1
		if maxExpected > 255 {
			maxExpected = 255
		}
		if result < minExpected || result > maxExpected {
			t.Errorf("pokeyGainToDAC(%f): expected ~%d, got %d", tt.gain, tt.expected, result)
		}
	}
}

func TestPOKEYEngine_Reset(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	// Set some state
	engine.WriteRegister(0, 0x50)
	engine.WriteRegister(1, 0xAF)
	engine.SetPOKEYPlusEnabled(true)

	// Reset
	engine.Reset()

	// Verify state is cleared
	if engine.regs[0] != 0 || engine.regs[1] != 0 {
		t.Error("registers should be cleared after reset")
	}
	if engine.enabled {
		t.Error("engine should be disabled after reset")
	}
}

func TestPOKEYEngine_DistortionModes(t *testing.T) {
	// Test AUDC distortion field extraction
	tests := []struct {
		audc       uint8
		distortion uint8
		volume     uint8
	}{
		{0xAF, 5, 15}, // Pure tone, max volume
		{0x0A, 0, 10}, // Poly17+Poly5, volume 10
		{0xC8, 6, 8},  // Poly4, volume 8
		{0x10, 0, 0},  // Volume-only mode
	}

	for _, tt := range tests {
		dist := (tt.audc & AUDC_DISTORTION_MASK) >> AUDC_DISTORTION_SHIFT
		vol := tt.audc & AUDC_VOLUME_MASK
		volOnly := (tt.audc & AUDC_VOLUME_ONLY) != 0

		if dist != tt.distortion {
			t.Errorf("AUDC 0x%02X: expected distortion %d, got %d", tt.audc, tt.distortion, dist)
		}
		if vol != tt.volume {
			t.Errorf("AUDC 0x%02X: expected volume %d, got %d", tt.audc, tt.volume, vol)
		}
		if tt.audc == 0x10 && !volOnly {
			t.Errorf("AUDC 0x%02X: volume-only bit should be set", tt.audc)
		}
	}
}

// BenchmarkPOKEY_CalcFrequency benchmarks the optimized frequency calculation
func BenchmarkPOKEY_CalcFrequency(b *testing.B) {
	engine := NewPOKEYEngine(nil, 44100)
	engine.SetClockHz(POKEY_CLOCK_NTSC)

	// Set up some typical register values
	engine.regs[0] = 0x50 // AUDF1
	engine.regs[2] = 0x80 // AUDF2
	engine.regs[4] = 0x40 // AUDF3
	engine.regs[6] = 0x60 // AUDF4
	engine.regs[8] = 0x00 // AUDCTL (64kHz mode)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = engine.calcFrequency(i % 4)
	}
}

// BenchmarkPOKEY_CalcFrequency_16Bit benchmarks 16-bit mode frequency calculation
func BenchmarkPOKEY_CalcFrequency_16Bit(b *testing.B) {
	engine := NewPOKEYEngine(nil, 44100)
	engine.SetClockHz(POKEY_CLOCK_NTSC)

	// Set up 16-bit mode (ch1+ch2 linked)
	engine.regs[0] = 0x50              // AUDF1 (low byte)
	engine.regs[2] = 0x01              // AUDF2 (high byte)
	engine.regs[8] = AUDCTL_CH2_BY_CH1 // 16-bit mode

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = engine.calcFrequency(1) // Channel 2 in 16-bit mode
	}
}

// TestPOKEY_ClockDivisors verifies pre-computed clock divisors are correct
func TestPOKEY_ClockDivisors(t *testing.T) {
	engine := NewPOKEYEngine(nil, 44100)

	// Test NTSC clock
	engine.SetClockHz(POKEY_CLOCK_NTSC)
	expectedNTSC179 := float64(POKEY_CLOCK_NTSC)
	expected64K := float64(POKEY_CLOCK_NTSC) / float64(POKEY_DIV_64KHZ)
	expected15K := float64(POKEY_CLOCK_NTSC) / float64(POKEY_DIV_15KHZ)

	if engine.clock179MHz != expectedNTSC179 {
		t.Errorf("NTSC 179MHz clock: expected %f, got %f", expectedNTSC179, engine.clock179MHz)
	}
	if engine.clock64KHz != expected64K {
		t.Errorf("NTSC 64kHz clock: expected %f, got %f", expected64K, engine.clock64KHz)
	}
	if engine.clock15KHz != expected15K {
		t.Errorf("NTSC 15kHz clock: expected %f, got %f", expected15K, engine.clock15KHz)
	}

	// Test PAL clock
	engine.SetClockHz(POKEY_CLOCK_PAL)
	expectedPAL179 := float64(POKEY_CLOCK_PAL)
	expectedPAL64K := float64(POKEY_CLOCK_PAL) / float64(POKEY_DIV_64KHZ)
	expectedPAL15K := float64(POKEY_CLOCK_PAL) / float64(POKEY_DIV_15KHZ)

	if engine.clock179MHz != expectedPAL179 {
		t.Errorf("PAL 179MHz clock: expected %f, got %f", expectedPAL179, engine.clock179MHz)
	}
	if engine.clock64KHz != expectedPAL64K {
		t.Errorf("PAL 64kHz clock: expected %f, got %f", expectedPAL64K, engine.clock64KHz)
	}
	if engine.clock15KHz != expectedPAL15K {
		t.Errorf("PAL 15kHz clock: expected %f, got %f", expectedPAL15K, engine.clock15KHz)
	}
}
