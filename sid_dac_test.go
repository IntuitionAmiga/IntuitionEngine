// sid_dac_test.go - Unit tests for SID 12-bit DAC quantization

package main

import (
	"math"
	"testing"
)

// TestSIDDAC_StandardPathQuantization verifies that the standard waveform path
// produces 12-bit quantized output when SID DAC mode is enabled.
func TestSIDDAC_StandardPathQuantization(t *testing.T) {
	ch := &Channel{
		waveType:      WAVE_TRIANGLE,
		frequency:     440,
		phase:         0,
		enabled:       true,
		sidDACEnabled: true,
		sidWaveMask:   0, // Standard path (not combined waveform path)
	}

	// Generate samples across a full cycle
	samples := make([]float32, 100)
	for i := range samples {
		samples[i] = ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)
	}

	// Verify samples are quantized to 12-bit levels
	// 12-bit means 4096 possible levels, so step size is 2.0/4095
	stepSize := 2.0 / 4095.0

	quantizedCount := 0
	for _, sample := range samples {
		// Convert sample from [-1, 1] to [0, 4095] range
		normalized := (sample + 1.0) / 2.0 * 4095.0

		// Check if it's close to an integer (quantized)
		rounded := math.Round(float64(normalized))
		diff := math.Abs(float64(normalized) - rounded)

		if diff < 0.01 { // Allow small floating point tolerance
			quantizedCount++
		}
	}

	// At least 90% of samples should be quantized
	if float64(quantizedCount)/float64(len(samples)) < 0.9 {
		t.Errorf("expected samples to be 12-bit quantized, got %d/%d quantized (step size: %f)",
			quantizedCount, len(samples), stepSize)
	}
}

// TestSIDDAC_OnlyWhenSIDModeEnabled verifies that quantization is only applied
// when SID DAC mode is explicitly enabled.
func TestSIDDAC_OnlyWhenSIDModeEnabled(t *testing.T) {
	// Channel without SID DAC enabled
	ch := &Channel{
		waveType:      WAVE_TRIANGLE,
		frequency:     440,
		phase:         0.5,
		enabled:       true,
		sidDACEnabled: false, // DAC disabled
		sidWaveMask:   0,
	}

	sample := ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)

	// Without DAC, sample should have full float precision
	// Convert to 12-bit space and check it's NOT an integer
	normalized := (sample + 1.0) / 2.0 * 4095.0
	rounded := math.Round(float64(normalized))
	diff := math.Abs(float64(normalized) - rounded)

	// With full precision, most samples won't land exactly on 12-bit levels
	// This is a probabilistic test - we're checking the feature is actually disabled
	// by verifying at least one sample is NOT quantized
	if diff < 0.0001 {
		// This could happen by chance, so generate more samples
		nonQuantizedFound := false
		for i := 0; i < 100; i++ {
			ch.phase = float32(i) * 0.1
			sample = ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)
			normalized = (sample + 1.0) / 2.0 * 4095.0
			rounded = math.Round(float64(normalized))
			diff = math.Abs(float64(normalized) - rounded)
			if diff > 0.001 {
				nonQuantizedFound = true
				break
			}
		}
		if !nonQuantizedFound {
			t.Error("expected full-precision samples when DAC disabled, but all samples appear quantized")
		}
	}
}

// TestSIDDAC_MatchesCombinedPath verifies that a single waveform through the
// standard path with DAC enabled produces output matching the combined waveform path.
func TestSIDDAC_MatchesCombinedPath(t *testing.T) {
	// Channel using combined waveform path (sidWaveMask set)
	chCombined := &Channel{
		waveType:    WAVE_TRIANGLE,
		frequency:   440,
		phase:       1.0,
		enabled:     true,
		sidWaveMask: SID_WAVE_TRIANGLE, // Uses combined path
		dutyCycle:   0.5,
	}

	// Channel using standard path with DAC enabled
	chStandard := &Channel{
		waveType:      WAVE_TRIANGLE,
		frequency:     440,
		phase:         1.0,
		enabled:       true,
		sidDACEnabled: true,
		sidWaveMask:   0, // Uses standard path
		dutyCycle:     0.5,
	}

	// Generate samples from both paths
	sampleCombined := chCombined.generateWaveSample(testSampleRate, 1.0/testSampleRate)
	sampleStandard := chStandard.generateWaveSample(testSampleRate, 1.0/testSampleRate)

	// Both should produce 12-bit quantized output
	// Allow some tolerance due to different code paths
	diff := math.Abs(float64(sampleCombined - sampleStandard))
	stepSize := 2.0 / 4095.0

	// Should be within a few quantization steps
	if diff > stepSize*3 {
		t.Errorf("standard path with DAC should match combined path: combined=%f, standard=%f, diff=%f",
			sampleCombined, sampleStandard, diff)
	}
}

// TestSIDDAC_AllWaveforms verifies that all tonal waveforms are correctly
// quantized when DAC mode is enabled.
func TestSIDDAC_AllWaveforms(t *testing.T) {
	waveforms := []struct {
		name     string
		waveType int
	}{
		{"triangle", WAVE_TRIANGLE},
		{"sawtooth", WAVE_SAWTOOTH},
		{"square", WAVE_SQUARE},
	}

	for _, wf := range waveforms {
		t.Run(wf.name, func(t *testing.T) {
			ch := &Channel{
				waveType:      wf.waveType,
				frequency:     440,
				phase:         0,
				enabled:       true,
				sidDACEnabled: true,
				sidWaveMask:   0,
				dutyCycle:     0.5,
				noiseSR:       NOISE_LFSR_SEED,
			}

			// Generate samples
			quantizedCount := 0
			totalSamples := 50

			for i := 0; i < totalSamples; i++ {
				sample := ch.generateWaveSample(testSampleRate, 1.0/testSampleRate)

				// Check quantization
				normalized := (sample + 1.0) / 2.0 * 4095.0
				rounded := math.Round(float64(normalized))
				diff := math.Abs(float64(normalized) - rounded)

				if diff < 0.01 {
					quantizedCount++
				}
			}

			// At least 80% should be quantized
			ratio := float64(quantizedCount) / float64(totalSamples)
			if ratio < 0.8 {
				t.Errorf("%s: expected 12-bit quantization, got %.1f%% quantized",
					wf.name, ratio*100)
			}
		})
	}
}

// TestSIDDAC_QuantizationStepSize verifies the exact 12-bit step size.
func TestSIDDAC_QuantizationStepSize(t *testing.T) {
	ch := &Channel{
		waveType:      WAVE_SAWTOOTH, // Linear ramp makes step size visible
		frequency:     100,           // Low frequency for slow ramp
		phase:         0,
		enabled:       true,
		sidDACEnabled: true,
		sidWaveMask:   0,
	}

	// Collect samples from a ramp
	var samples []float32
	for i := 0; i < 200; i++ {
		samples = append(samples, ch.generateWaveSample(testSampleRate, 1.0/testSampleRate))
	}

	// Find minimum non-zero difference between consecutive samples
	minDiff := float32(2.0) // Start with max possible
	for i := 1; i < len(samples); i++ {
		diff := float32(math.Abs(float64(samples[i] - samples[i-1])))
		if diff > 0.0001 && diff < minDiff {
			minDiff = diff
		}
	}

	// Expected step size for 12-bit: 2.0 / 4095 ≈ 0.000488
	expectedStep := float32(2.0 / 4095.0)

	// Minimum difference should be close to the quantization step
	// (or a multiple of it)
	ratio := minDiff / expectedStep
	roundedRatio := math.Round(float64(ratio))
	ratioError := math.Abs(float64(ratio) - roundedRatio)

	if ratioError > 0.1 || roundedRatio < 1 {
		t.Errorf("expected step size multiple of %f, got %f (ratio: %f)",
			expectedStep, minDiff, ratio)
	}
}
