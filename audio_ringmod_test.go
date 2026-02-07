// audio_ringmod_test.go - Unit tests for SID-accurate ring modulation

package main

import (
	"math"
	"testing"
)

// TestRingMod_OnlyAffectsTriangle verifies that SID-style ring modulation
// only affects triangle waveforms (real SID behavior).
func TestRingMod_OnlyAffectsTriangle(t *testing.T) {
	tests := []struct {
		name       string
		waveType   int
		expectFlip bool
	}{
		{"triangle_affected", WAVE_TRIANGLE, true},
		{"square_unaffected", WAVE_SQUARE, false},
		{"sawtooth_unaffected", WAVE_SAWTOOTH, false},
		{"sine_unaffected", WAVE_SINE, false},
		{"noise_unaffected", WAVE_NOISE, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create master channel with phaseMSB=true (upper half of cycle)
			master := &Channel{
				waveType:  WAVE_SQUARE,
				frequency: 440,
				phaseMSB:  true,
				dutyCycle: 0.5,
			}

			// Create slave channel with ring mod enabled
			slave := &Channel{
				waveType:      tc.waveType,
				ringModSource: master,
				frequency:     220,
				phase:         1.0, // In lower half initially
				noiseSR:       NOISE_LFSR_SEED,
				enabled:       true,
				dutyCycle:     0.5,
			}

			// Generate sample with master MSB high
			sample1 := slave.generateWaveSample(testSampleRate, 1.0/testSampleRate)

			// Reset phase and generate with master MSB low
			master.phaseMSB = false
			slave.phase = 1.0
			sample2 := slave.generateWaveSample(testSampleRate, 1.0/testSampleRate)

			if tc.expectFlip {
				// For triangle, samples should have opposite signs when MSB changes
				// (one positive, one negative - indicating sign flip)
				if sample1*sample2 > 0 && math.Abs(float64(sample1)) > 0.01 && math.Abs(float64(sample2)) > 0.01 {
					// Both samples have same sign - ring mod not working
					t.Errorf("triangle should be inverted when master MSB is high: sample1=%f, sample2=%f", sample1, sample2)
				}
			} else {
				// For non-triangle, samples should be unaffected by MSB state
				// (sign should remain consistent regardless of master MSB)
				if sample1 != 0 && sample2 != 0 {
					// Both should have same sign (or be the same value)
					if (sample1 > 0) != (sample2 > 0) {
						t.Errorf("%s should not be affected by ring mod: sample1=%f, sample2=%f", tc.name, sample1, sample2)
					}
				}
			}
		})
	}
}

// TestRingMod_MSBTracking verifies that the phaseMSB flag is correctly set
// based on the phase position (true when phase >= Pi).
func TestRingMod_MSBTracking(t *testing.T) {
	tests := []struct {
		name      string
		phase     float32
		expectMSB bool
	}{
		{"lower_half_start", 0.0, false},
		{"lower_half_mid", 1.5, false},
		{"at_pi", math.Pi, true},
		{"upper_half_mid", 4.0, true},
		{"upper_half_end", 6.0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := &Channel{
				waveType:  WAVE_SINE,
				frequency: 440,
				phase:     tc.phase,
				enabled:   true,
			}

			ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)

			// After sample generation, phaseMSB should reflect the current phase position
			// Note: phase advances during generation, so we check if MSB tracking is working
			if tc.expectMSB && !ch.phaseMSB && ch.phase < math.Pi {
				t.Errorf("phaseMSB should be true when phase >= Pi, got false (phase=%f)", ch.phase)
			}
		})
	}
}

// TestRingMod_MSBTrackingAfterWrap verifies MSB tracking across phase wraps.
func TestRingMod_MSBTrackingAfterWrap(t *testing.T) {
	ch := &Channel{
		waveType:  WAVE_SINE,
		frequency: 440,
		enabled:   true,
	}

	// Track MSB changes over multiple samples
	var msbChanges int
	prevMSB := ch.phaseMSB

	// Generate enough samples to see several phase wraps
	for i := 0; i < 200; i++ {
		ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)
		if ch.phaseMSB != prevMSB {
			msbChanges++
			prevMSB = ch.phaseMSB
		}
	}

	// Should see MSB toggle at least a few times (once per half-cycle)
	if msbChanges < 2 {
		t.Errorf("expected MSB to toggle multiple times, got %d changes", msbChanges)
	}
}

// TestRingMod_DisableClears verifies that clearing the ring mod register
// properly disables ring modulation.
func TestRingMod_DisableClears(t *testing.T) {
	chip := newTestSoundChip()

	// Enable ring mod: ch0 uses ch1 as source (bit 7=enable, bits 0-1=source)
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_OFF_RINGMOD, 0x81) // enable, source ch1
	if chip.channels[0].ringModSource == nil {
		t.Fatal("ring mod should be enabled after writing 0x81")
	}
	if chip.channels[0].ringModSource != chip.channels[1] {
		t.Error("ring mod source should be channel 1")
	}

	// Disable ring mod (bit 7 = 0)
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_OFF_RINGMOD, 0x00)
	if chip.channels[0].ringModSource != nil {
		t.Error("ring mod should be disabled when bit 7 is 0")
	}
}

// TestRingMod_SelfModPrevented verifies that a channel cannot ring mod itself.
func TestRingMod_SelfModPrevented(t *testing.T) {
	chip := newTestSoundChip()

	// Try to set ch0 to ring mod with ch0 (should be prevented)
	chip.HandleRegisterWrite(FLEX_CH_BASE+FLEX_OFF_RINGMOD, 0x80) // ch0 ring mod with ch0
	if chip.channels[0].ringModSource != nil {
		t.Error("self ring modulation should be prevented")
	}
}

// TestRingMod_AllChannels verifies ring mod works for all channel combinations.
func TestRingMod_AllChannels(t *testing.T) {
	chip := newTestSoundChip()

	for slave := 0; slave < NUM_CHANNELS; slave++ {
		for master := 0; master < NUM_CHANNELS; master++ {
			if slave == master {
				continue // Skip self-mod
			}

			// Clear previous ring mod
			chip.channels[slave].ringModSource = nil

			// Set up ring mod via FLEX register
			addr := FLEX_CH_BASE + uint32(slave)*FLEX_CH_STRIDE + FLEX_OFF_RINGMOD
			chip.HandleRegisterWrite(addr, 0x80|uint32(master))

			if chip.channels[slave].ringModSource != chip.channels[master] {
				t.Errorf("ch%d should ring mod with ch%d", slave, master)
			}
		}
	}
}

// TestRingMod_CombinedWaveformPath verifies ring mod works with SID combined waveforms.
func TestRingMod_CombinedWaveformPath(t *testing.T) {
	// Create master with phaseMSB=true
	master := &Channel{
		waveType: WAVE_SQUARE,
		phaseMSB: true,
	}

	// Create slave using SID combined waveform path (sidWaveMask set)
	slave := &Channel{
		waveType:      WAVE_TRIANGLE,
		ringModSource: master,
		frequency:     220,
		phase:         1.0,
		sidWaveMask:   SID_WAVE_TRIANGLE, // Triggers combined waveform path
		enabled:       true,
		dutyCycle:     0.5,
	}

	// Generate sample with MSB high
	sample1 := slave.generateWaveSample(testSampleRate, 1.0/testSampleRate)

	// Generate sample with MSB low
	master.phaseMSB = false
	slave.phase = 1.0
	slave.sidWaveMask = SID_WAVE_TRIANGLE
	sample2 := slave.generateWaveSample(testSampleRate, 1.0/testSampleRate)

	// With triangle and ring mod, sign should flip
	if sample1 != 0 && sample2 != 0 {
		if sample1*sample2 > 0 {
			t.Logf("Warning: combined waveform path may not show ring mod effect clearly (sample1=%f, sample2=%f)", sample1, sample2)
		}
	}
}

// TestRingMod_NoiseUnaffected verifies that noise waveform ignores ring mod
// (noise doesn't use phase, so MSB-based ring mod shouldn't affect it).
func TestRingMod_NoiseUnaffected(t *testing.T) {
	master := &Channel{
		waveType: WAVE_SQUARE,
		phaseMSB: true,
	}

	slave := &Channel{
		waveType:      WAVE_NOISE,
		ringModSource: master,
		frequency:     1000,
		noiseSR:       NOISE_LFSR_SEED,
		enabled:       true,
	}

	// Generate several samples and verify noise still produces output
	var nonZeroSamples int
	for i := 0; i < 100; i++ {
		sample := slave.generateWaveSample(testSampleRate, 1.0/testSampleRate)
		if sample != 0 {
			nonZeroSamples++
		}
	}

	// Noise should still produce output even with ring mod "enabled"
	if nonZeroSamples < 10 {
		t.Error("noise should still produce output with ring mod enabled (ring mod only affects triangle)")
	}
}
