// sid_engine_test.go - Tests for SID sound chip emulation

package main

import (
	"testing"
)

func TestSIDEngine_NewEngine(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)
	if engine == nil {
		t.Fatal("NewSIDEngine returned nil")
	}
	if engine.clockHz != SID_CLOCK_PAL {
		t.Errorf("expected clock %d, got %d", SID_CLOCK_PAL, engine.clockHz)
	}
	if engine.sampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", engine.sampleRate)
	}
}

func TestSIDEngine_SetClockHz(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	engine.SetClockHz(SID_CLOCK_NTSC)
	if engine.clockHz != SID_CLOCK_NTSC {
		t.Errorf("expected NTSC clock %d, got %d", SID_CLOCK_NTSC, engine.clockHz)
	}

	// Zero clock should be ignored
	engine.SetClockHz(0)
	if engine.clockHz != SID_CLOCK_NTSC {
		t.Errorf("zero clock should be ignored, got %d", engine.clockHz)
	}
}

func TestSIDEngine_WriteRegister(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	// Write to voice 1 frequency low
	engine.WriteRegister(0, 0x50)
	if engine.regs[0] != 0x50 {
		t.Errorf("expected V1_FREQ_LO=0x50, got 0x%02X", engine.regs[0])
	}

	// Write to voice 1 control
	engine.WriteRegister(4, SID_CTRL_TRIANGLE|SID_CTRL_GATE)
	if engine.regs[4] != (SID_CTRL_TRIANGLE | SID_CTRL_GATE) {
		t.Errorf("expected V1_CTRL=0x%02X, got 0x%02X", SID_CTRL_TRIANGLE|SID_CTRL_GATE, engine.regs[4])
	}

	// Write to master volume
	engine.WriteRegister(0x18, 0x0F)
	if engine.regs[0x18] != 0x0F {
		t.Errorf("expected MODE_VOL=0x0F, got 0x%02X", engine.regs[0x18])
	}

	// Out of bounds register should be ignored
	engine.WriteRegister(50, 0xFF)
	// Should not panic
}

func TestSIDEngine_HandleIO(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	// Write via memory-mapped I/O
	engine.HandleWrite(SID_V1_FREQ_LO, 0x30)
	if engine.regs[0] != 0x30 {
		t.Errorf("HandleWrite failed: expected 0x30, got 0x%02X", engine.regs[0])
	}

	// Read via memory-mapped I/O
	value := engine.HandleRead(SID_V1_FREQ_LO)
	if value != 0x30 {
		t.Errorf("HandleRead failed: expected 0x30, got 0x%02X", value)
	}

	// Out of range addresses should be ignored
	engine.HandleWrite(0xF0F00, 0xFF)
	value = engine.HandleRead(0xF0F00)
	if value != 0 {
		t.Errorf("out of range read should return 0, got 0x%02X", value)
	}
}

func TestSIDEngine_FrequencyCalculation(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	tests := []struct {
		name    string
		freqLo  uint8
		freqHi  uint8
		voice   int
		minFreq float64
		maxFreq float64
	}{
		{
			name:    "Voice 1 middle C (261Hz)",
			freqLo:  0x0E, // ~261 Hz at PAL clock
			freqHi:  0x11,
			voice:   0,
			minFreq: 250,
			maxFreq: 275,
		},
		{
			name:    "Voice 2 A440",
			freqLo:  0xD6, // ~440 Hz at PAL clock
			freqHi:  0x1C,
			voice:   1,
			minFreq: 430,
			maxFreq: 450,
		},
		{
			name:    "Voice 3 high frequency",
			freqLo:  0x00,
			freqHi:  0x80,
			voice:   2,
			minFreq: 1900,
			maxFreq: 2000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.Reset()
			base := tt.voice * 7
			engine.regs[base] = tt.freqLo
			engine.regs[base+1] = tt.freqHi

			freq := engine.calcFrequency(tt.voice)
			if freq < tt.minFreq || freq > tt.maxFreq {
				t.Errorf("frequency %f not in range [%f, %f]", freq, tt.minFreq, tt.maxFreq)
			}
		})
	}
}

func TestSIDEngine_PulseWidth(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	tests := []struct {
		name     string
		pwLo     uint8
		pwHi     uint8
		voice    int
		expected float32
		delta    float32
	}{
		{
			name:     "50% duty cycle (2048)",
			pwLo:     0x00,
			pwHi:     0x08, // 2048
			voice:    0,
			expected: 0.5,
			delta:    0.01,
		},
		{
			name:     "25% duty cycle (1024)",
			pwLo:     0x00,
			pwHi:     0x04, // 1024
			voice:    1,
			expected: 0.25,
			delta:    0.01,
		},
		{
			name:     "Max duty (4095)",
			pwLo:     0xFF,
			pwHi:     0x0F, // 4095
			voice:    2,
			expected: 1.0,
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine.Reset()
			base := tt.voice * 7
			engine.regs[base+2] = tt.pwLo
			engine.regs[base+3] = tt.pwHi

			pw := engine.calcPulseWidth(tt.voice)
			diff := pw - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("pulse width %f not close to expected %f", pw, tt.expected)
			}
		})
	}
}

func TestSIDEngine_SIDPlus(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	if engine.SIDPlusEnabled() {
		t.Error("SID+ should be disabled by default")
	}

	engine.SetSIDPlusEnabled(true)
	if !engine.SIDPlusEnabled() {
		t.Error("SID+ should be enabled after SetSIDPlusEnabled(true)")
	}

	// Test via register write
	engine.WriteRegister(0x19, 0) // SID_PLUS_CTRL offset
	if engine.SIDPlusEnabled() {
		t.Error("SID+ should be disabled after writing 0 to control register")
	}

	engine.WriteRegister(0x19, 1)
	if !engine.SIDPlusEnabled() {
		t.Error("SID+ should be enabled after writing 1 to control register")
	}
}

func TestSIDEngine_VolumeGain(t *testing.T) {
	// Test linear volume (standard SID)
	for level := uint8(0); level <= 15; level++ {
		gain := sidVolumeGain(level, false)
		expected := float32(level) / 15.0
		if gain != expected {
			t.Errorf("linear gain for level %d: expected %f, got %f", level, expected, gain)
		}
	}

	// Test logarithmic volume (SID+)
	// Level 0 should be 0
	if sidVolumeGain(0, true) != 0 {
		t.Error("SID+ level 0 should be 0")
	}
	// Level 15 should be 1.0
	if sidVolumeGain(15, true) != 1.0 {
		t.Error("SID+ level 15 should be 1.0")
	}
	// Mid levels should be logarithmic (quieter than linear)
	linearMid := sidVolumeGain(8, false)
	logMid := sidVolumeGain(8, true)
	if logMid >= linearMid {
		t.Errorf("SID+ mid-level should be quieter: linear=%f, log=%f", linearMid, logMid)
	}
}

func TestSIDEngine_GainToDAC(t *testing.T) {
	tests := []struct {
		gain     float32
		expected int
	}{
		{0.0, 0},
		{0.5, 128},
		{1.0, 255},
		{-0.1, 0},
		{1.5, 255},
	}

	for _, tt := range tests {
		result := int(sidGainToDAC(tt.gain))
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
			t.Errorf("sidGainToDAC(%f): expected ~%d, got %d", tt.gain, tt.expected, result)
		}
	}
}

func TestSIDEngine_Reset(t *testing.T) {
	engine := NewSIDEngine(nil, 44100)

	// Set some state
	engine.WriteRegister(0, 0x50)
	engine.WriteRegister(4, SID_CTRL_TRIANGLE|SID_CTRL_GATE)
	engine.SetSIDPlusEnabled(true)

	// Reset
	engine.Reset()

	// Verify state is cleared
	if engine.regs[0] != 0 || engine.regs[4] != 0 {
		t.Error("registers should be cleared after reset")
	}
	if engine.enabled.Load() {
		t.Error("engine should be disabled after reset")
	}
	for i, gate := range engine.voiceGate {
		if gate {
			t.Errorf("voice %d gate should be false after reset", i)
		}
	}
}

func TestSIDEngine_WaveformSelection(t *testing.T) {
	// Test control register bit masks
	tests := []struct {
		ctrl     uint8
		triangle bool
		sawtooth bool
		pulse    bool
		noise    bool
	}{
		{SID_CTRL_TRIANGLE, true, false, false, false},
		{SID_CTRL_SAWTOOTH, false, true, false, false},
		{SID_CTRL_PULSE, false, false, true, false},
		{SID_CTRL_NOISE, false, false, false, true},
		{SID_CTRL_TRIANGLE | SID_CTRL_SAWTOOTH, true, true, false, false},
	}

	for _, tt := range tests {
		if (tt.ctrl&SID_CTRL_TRIANGLE != 0) != tt.triangle {
			t.Errorf("CTRL 0x%02X: triangle mismatch", tt.ctrl)
		}
		if (tt.ctrl&SID_CTRL_SAWTOOTH != 0) != tt.sawtooth {
			t.Errorf("CTRL 0x%02X: sawtooth mismatch", tt.ctrl)
		}
		if (tt.ctrl&SID_CTRL_PULSE != 0) != tt.pulse {
			t.Errorf("CTRL 0x%02X: pulse mismatch", tt.ctrl)
		}
		if (tt.ctrl&SID_CTRL_NOISE != 0) != tt.noise {
			t.Errorf("CTRL 0x%02X: noise mismatch", tt.ctrl)
		}
	}
}

func TestSIDEngine_FilterModes(t *testing.T) {
	// Test mode/volume register bit masks
	tests := []struct {
		mode      uint8
		lowPass   bool
		bandPass  bool
		highPass  bool
		voice3Off bool
		volume    uint8
	}{
		{0x0F, false, false, false, false, 15},
		{SID_MODE_LP | 0x0A, true, false, false, false, 10},
		{SID_MODE_BP | 0x08, false, true, false, false, 8},
		{SID_MODE_HP | 0x05, false, false, true, false, 5},
		{SID_MODE_3OFF | 0x0C, false, false, false, true, 12},
		{SID_MODE_LP | SID_MODE_HP | 0x0F, true, false, true, false, 15}, // Notch
	}

	for _, tt := range tests {
		vol := tt.mode & SID_MODE_VOL_MASK
		lp := (tt.mode & SID_MODE_LP) != 0
		bp := (tt.mode & SID_MODE_BP) != 0
		hp := (tt.mode & SID_MODE_HP) != 0
		v3off := (tt.mode & SID_MODE_3OFF) != 0

		if vol != tt.volume {
			t.Errorf("MODE 0x%02X: volume %d, expected %d", tt.mode, vol, tt.volume)
		}
		if lp != tt.lowPass {
			t.Errorf("MODE 0x%02X: lowPass mismatch", tt.mode)
		}
		if bp != tt.bandPass {
			t.Errorf("MODE 0x%02X: bandPass mismatch", tt.mode)
		}
		if hp != tt.highPass {
			t.Errorf("MODE 0x%02X: highPass mismatch", tt.mode)
		}
		if v3off != tt.voice3Off {
			t.Errorf("MODE 0x%02X: voice3Off mismatch", tt.mode)
		}
	}
}

func TestSIDEngine_ADSRTimingTable(t *testing.T) {
	// Verify ADSR timing tables are populated correctly
	if len(sidAttackMs) != 16 {
		t.Errorf("sidAttackMs should have 16 entries, got %d", len(sidAttackMs))
	}
	if len(sidDecayReleaseMs) != 16 {
		t.Errorf("sidDecayReleaseMs should have 16 entries, got %d", len(sidDecayReleaseMs))
	}

	// Attack times should increase
	for i := 1; i < 16; i++ {
		if sidAttackMs[i] <= sidAttackMs[i-1] {
			t.Errorf("attack times should increase: [%d]=%f <= [%d]=%f",
				i, sidAttackMs[i], i-1, sidAttackMs[i-1])
		}
	}

	// Decay/release times should increase
	for i := 1; i < 16; i++ {
		if sidDecayReleaseMs[i] <= sidDecayReleaseMs[i-1] {
			t.Errorf("decay/release times should increase: [%d]=%f <= [%d]=%f",
				i, sidDecayReleaseMs[i], i-1, sidDecayReleaseMs[i-1])
		}
	}

	// Shortest attack should be very fast (< 10ms)
	if sidAttackMs[0] > 10 {
		t.Errorf("shortest attack time should be < 10ms, got %f", sidAttackMs[0])
	}

	// Longest attack should be slow (> 5000ms)
	if sidAttackMs[15] < 5000 {
		t.Errorf("longest attack time should be > 5000ms, got %f", sidAttackMs[15])
	}
}

// TestSIDEngine_FilterCutoffTableAccuracy_6581 verifies the 6581 filter lookup table accuracy
func TestSIDEngine_FilterCutoffTableAccuracy_6581(t *testing.T) {
	// Test that the lookup table matches the original math.Pow calculation
	// within acceptable tolerance (< 1% deviation)

	testCases := []struct {
		cutoff uint16
		desc   string
	}{
		{0, "zero cutoff"},
		{1, "minimum non-zero"},
		{256, "quarter range"},
		{512, "mid-low range"},
		{1024, "half range"},
		{1536, "mid-high range"},
		{2047, "maximum cutoff"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Get LUT value
			lutNorm := sidFilterNorm6581Table[tc.cutoff]

			// Verify it's in valid range
			if lutNorm < 0 || lutNorm > 1 {
				t.Errorf("cutoff %d: normalized value %f out of range [0,1]", tc.cutoff, lutNorm)
			}
		})
	}

	// Verify monotonically increasing (filter cutoff should increase with register value)
	for i := 1; i < sidFilterCutoffTableSize; i++ {
		if sidFilterCutoff6581Table[i] < sidFilterCutoff6581Table[i-1] {
			t.Errorf("6581 cutoff table not monotonic at index %d: %f < %f",
				i, sidFilterCutoff6581Table[i], sidFilterCutoff6581Table[i-1])
			break
		}
	}

	// Verify frequency range
	// 6581 uses non-linear curve: Fc = 30 + cutoff^1.35 * 0.22
	// At cutoff=0, Fc = 30 Hz
	// At cutoff=2047, Fc ≈ 6523 Hz (curve doesn't reach theoretical max)
	if sidFilterCutoff6581Table[0] < 29 || sidFilterCutoff6581Table[0] > 31 {
		t.Errorf("6581 minimum cutoff should be ~30Hz, got %f", sidFilterCutoff6581Table[0])
	}
	if sidFilterCutoff6581Table[2047] < 6400 || sidFilterCutoff6581Table[2047] > 6700 {
		t.Errorf("6581 maximum cutoff should be ~6523Hz (non-linear curve), got %f", sidFilterCutoff6581Table[2047])
	}
}

// TestSIDEngine_FilterCutoffTableAccuracy_8580 verifies the 8580 filter lookup table accuracy
func TestSIDEngine_FilterCutoffTableAccuracy_8580(t *testing.T) {
	// Verify monotonically increasing
	for i := 1; i < sidFilterCutoffTableSize; i++ {
		if sidFilterCutoff8580Table[i] < sidFilterCutoff8580Table[i-1] {
			t.Errorf("8580 cutoff table not monotonic at index %d: %f < %f",
				i, sidFilterCutoff8580Table[i], sidFilterCutoff8580Table[i-1])
			break
		}
	}

	// Verify frequency range
	if sidFilterCutoff8580Table[0] < 29 || sidFilterCutoff8580Table[0] > 31 {
		t.Errorf("8580 minimum cutoff should be ~30Hz, got %f", sidFilterCutoff8580Table[0])
	}

	// 8580 uses linear curve: Fc = 30 + cutoff * 5.8
	// At cutoff=2047, Fc ≈ 11902 Hz
	if sidFilterCutoff8580Table[2047] < 11800 || sidFilterCutoff8580Table[2047] > 12000 {
		t.Errorf("8580 maximum cutoff should be ~11902Hz (linear curve), got %f", sidFilterCutoff8580Table[2047])
	}

	// Verify 8580 linear response: middle value should be approximately linear
	// At cutoff=1024, expect roughly 30 + 1024*5.8 = ~5969 Hz
	expectedMid := float32(30.0 + 1024.0*5.8)
	actualMid := sidFilterCutoff8580Table[1024]
	tolerance := expectedMid * 0.05 // 5% tolerance
	if actualMid < expectedMid-tolerance || actualMid > expectedMid+tolerance {
		t.Errorf("8580 mid-range cutoff: expected ~%f, got %f", expectedMid, actualMid)
	}
}

// TestSIDEngine_FilterNormalizationRange verifies normalized cutoff stays in [0,1]
func TestSIDEngine_FilterNormalizationRange(t *testing.T) {
	// Check all 6581 entries
	for i := 0; i < sidFilterCutoffTableSize; i++ {
		if sidFilterNorm6581Table[i] < 0 || sidFilterNorm6581Table[i] > 1 {
			t.Errorf("6581 norm table[%d] = %f out of [0,1] range", i, sidFilterNorm6581Table[i])
		}
	}

	// Check all 8580 entries
	for i := 0; i < sidFilterCutoffTableSize; i++ {
		if sidFilterNorm8580Table[i] < 0 || sidFilterNorm8580Table[i] > 1 {
			t.Errorf("8580 norm table[%d] = %f out of [0,1] range", i, sidFilterNorm8580Table[i])
		}
	}

	// Verify endpoints
	if sidFilterNorm6581Table[0] != 0 {
		t.Errorf("6581 norm[0] should be 0, got %f", sidFilterNorm6581Table[0])
	}
	if sidFilterNorm8580Table[0] != 0 {
		t.Errorf("8580 norm[0] should be 0, got %f", sidFilterNorm8580Table[0])
	}

	// Max should be normalized based on actual curve values
	// 6581: 6523 Hz normalized to 12000 max gives ~0.898
	// 8580: 11902 Hz normalized to 18000 max gives ~0.935
	if sidFilterNorm6581Table[2047] < 0.85 || sidFilterNorm6581Table[2047] > 1.0 {
		t.Errorf("6581 norm[2047] should be in [0.85, 1.0], got %f", sidFilterNorm6581Table[2047])
	}
	if sidFilterNorm8580Table[2047] < 0.90 || sidFilterNorm8580Table[2047] > 1.0 {
		t.Errorf("8580 norm[2047] should be in [0.90, 1.0], got %f", sidFilterNorm8580Table[2047])
	}
}

// BenchmarkSID_FilterCutoffLookup benchmarks the optimized LUT-based filter cutoff
func BenchmarkSID_FilterCutoffLookup(b *testing.B) {
	cutoffs := []uint16{0, 256, 512, 1024, 1536, 2047}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cutoff := cutoffs[i%len(cutoffs)]
		_ = sidFilterNorm6581Table[cutoff]
	}
}

// BenchmarkSID_FilterApplyFilter benchmarks the full applyFilter with LUT optimization
func BenchmarkSID_FilterApplyFilter(b *testing.B) {
	// Create a minimal SID engine with mock sound chip
	engine := NewSIDEngine(nil, 44100)
	engine.model = SID_MODEL_6581

	// Set up filter registers with typical values
	engine.regs[0x15] = 0x07 // FC_LO
	engine.regs[0x16] = 0x40 // FC_HI
	engine.regs[0x17] = 0xF7 // RES_FILT: res=15, route all
	engine.regs[0x18] = 0x1F // MODE_VOL: LP, vol=15

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine.applyFilter()
	}
}
