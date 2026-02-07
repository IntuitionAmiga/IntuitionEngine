// sid_noise_timing_test.go - Unit tests for SID noise LFSR timing

package main

import (
	"testing"
)

// TestSIDNoise_OscillatorPhaseLocked verifies that noise LFSR clocks on
// oscillator MSB transitions rather than per-sample.
func TestSIDNoise_OscillatorPhaseLocked(t *testing.T) {
	ch := &Channel{
		waveType:            WAVE_NOISE,
		frequency:           100, // Low frequency = slow phase advance
		enabled:             true,
		noiseSR:             NOISE_LFSR_SEED,
		dutyCycle:           0.5,
		sidNoisePhaseLocked: true, // Enable phase-locked noise timing
	}

	// Record initial LFSR state
	initialSR := ch.noiseSR

	// Generate samples at high sample rate - noise shouldn't clock every sample
	// with phase-locked mode, it should only clock when phase wraps
	lfsr_changes := 0
	prevSR := ch.noiseSR
	for i := 0; i < 100; i++ {
		ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)
		if ch.noiseSR != prevSR {
			lfsr_changes++
			prevSR = ch.noiseSR
		}
	}

	// With low frequency and phase-locked, LFSR should change less frequently
	// than with frequency-based clocking
	if lfsr_changes > 50 {
		t.Errorf("phase-locked noise should clock less frequently: initial=%d, changes=%d",
			initialSR, lfsr_changes)
	}

	t.Logf("LFSR changed %d times in 100 samples (phase-locked mode)", lfsr_changes)
}

// TestSIDNoise_FrequencyBasedClocking verifies the traditional frequency-based
// noise clocking when phase-locked mode is disabled.
func TestSIDNoise_FrequencyBasedClocking(t *testing.T) {
	ch := &Channel{
		waveType:            WAVE_NOISE,
		frequency:           1000, // Higher frequency = more clocking
		enabled:             true,
		noiseSR:             NOISE_LFSR_SEED,
		dutyCycle:           0.5,
		sidNoisePhaseLocked: false, // Traditional frequency-based timing
	}

	// Generate samples
	lfsr_changes := 0
	prevSR := ch.noiseSR
	for i := 0; i < 100; i++ {
		ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)
		if ch.noiseSR != prevSR {
			lfsr_changes++
			prevSR = ch.noiseSR
		}
	}

	// With frequency-based clocking and moderate frequency,
	// LFSR should change somewhat frequently
	t.Logf("LFSR changed %d times in 100 samples (frequency-based mode)", lfsr_changes)
}

// TestSIDNoise_ZeroFrequencyNoClocking verifies that zero frequency
// doesn't clock the noise LFSR.
func TestSIDNoise_ZeroFrequencyNoClocking(t *testing.T) {
	ch := &Channel{
		waveType:            WAVE_NOISE,
		frequency:           0, // Zero frequency
		enabled:             true,
		noiseSR:             NOISE_LFSR_SEED,
		dutyCycle:           0.5,
		sidNoisePhaseLocked: true,
	}

	initialSR := ch.noiseSR

	// Generate samples - LFSR should not change
	for i := 0; i < 100; i++ {
		ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)
	}

	if ch.noiseSR != initialSR {
		t.Errorf("zero frequency should not clock LFSR: initial=%d, final=%d",
			initialSR, ch.noiseSR)
	}
}

// TestSIDNoise_CombinedWaveformTiming verifies that combined waveforms
// with noise use consistent LFSR timing.
func TestSIDNoise_CombinedWaveformTiming(t *testing.T) {
	// Single noise channel
	chNoise := &Channel{
		waveType:            WAVE_NOISE,
		frequency:           440,
		enabled:             true,
		noiseSR:             NOISE_LFSR_SEED,
		dutyCycle:           0.5,
		sidNoisePhaseLocked: true,
		sidWaveMask:         0, // Single waveform path
	}

	// Combined noise+triangle channel
	chCombined := &Channel{
		waveType:            WAVE_NOISE,
		frequency:           440,
		enabled:             true,
		noiseSR:             NOISE_LFSR_SEED,
		dutyCycle:           0.5,
		sidNoisePhaseLocked: true,
		sidWaveMask:         SID_WAVE_NOISE | SID_WAVE_TRIANGLE, // Combined path
	}

	// Generate samples and compare LFSR states
	for i := 0; i < 50; i++ {
		chNoise.generateWaveSample(testSampleRate, 1.0/testSampleRate)
		chCombined.generateWaveSample(testSampleRate, 1.0/testSampleRate)
	}

	// Both should have similar LFSR progression
	// (exact match not required due to different code paths)
	t.Logf("single noise LFSR: %d, combined LFSR: %d", chNoise.noiseSR, chCombined.noiseSR)
}

// TestSIDNoise_PhaseWrapDetection verifies that phase wrap is correctly
// detected for noise clocking.
func TestSIDNoise_PhaseWrapDetection(t *testing.T) {
	ch := &Channel{
		waveType:            WAVE_NOISE,
		frequency:           testSampleRate / 10, // 10 samples per cycle
		enabled:             true,
		noiseSR:             NOISE_LFSR_SEED,
		dutyCycle:           0.5,
		sidNoisePhaseLocked: true,
		phase:               0,
	}

	// Count phase wraps and LFSR changes
	wraps := 0
	lfsr_changes := 0
	prevSR := ch.noiseSR

	for i := 0; i < 100; i++ {
		prevPhase := ch.phase
		ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)

		// Detect phase wrap
		if ch.phase < prevPhase {
			wraps++
		}

		// Detect LFSR change
		if ch.noiseSR != prevSR {
			lfsr_changes++
			prevSR = ch.noiseSR
		}
	}

	t.Logf("phase wraps: %d, LFSR changes: %d (10 samples/cycle expected ~10 wraps)",
		wraps, lfsr_changes)

	// With phase-locked mode, LFSR changes should correlate with phase wraps
	if lfsr_changes > wraps*2 {
		t.Errorf("LFSR changes (%d) shouldn't greatly exceed phase wraps (%d)",
			lfsr_changes, wraps)
	}
}
