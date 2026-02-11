// sid_mixer_test.go - Unit tests for SID DC offset and mixer saturation

package main

import (
	"math"
	"testing"
)

// TestSIDMixer_6581DCOffset verifies that the 6581 model applies DC offset
// while the 8580 model does not.
func TestSIDMixer_6581DCOffset(t *testing.T) {
	// Disable all channels for this test
	for i := range NUM_CHANNELS {
		chip.channels[i].enabled = false
	}

	// Configure for 6581 mode with DC offset
	chip.SetSIDMixerMode(true, SID_6581_DC_OFFSET, false)

	// Generate samples - should have DC offset
	var sum float64
	numSamples := 100
	for range numSamples {
		sample := chip.GenerateSample()
		sum += float64(sample)
	}
	avg6581 := sum / float64(numSamples)

	// Reset for 8580 mode (no DC offset)
	chip.SetSIDMixerMode(true, SID_8580_DC_OFFSET, false)

	sum = 0
	for range numSamples {
		sample := chip.GenerateSample()
		sum += float64(sample)
	}
	avg8580 := sum / float64(numSamples)

	// Disable mixer mode to not affect other tests
	chip.SetSIDMixerMode(false, 0, false)

	// 6581 should have noticeable DC offset, 8580 should be near zero
	if math.Abs(avg6581) < 0.1 {
		t.Errorf("6581 should have DC offset, got average: %f", avg6581)
	}
	if math.Abs(avg8580) > 0.05 {
		t.Errorf("8580 should have minimal DC offset, got average: %f", avg8580)
	}
}

// TestSIDMixer_DCOffsetValue verifies the specific DC offset values for each model.
func TestSIDMixer_DCOffsetValue(t *testing.T) {
	tests := []struct {
		name           string
		dcOffset       float32
		expectedOffset float64
		tolerance      float64
	}{
		{"6581", SID_6581_DC_OFFSET, 0.38, 0.05},
		{"8580", SID_8580_DC_OFFSET, 0.0, 0.01},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Disable all channels
			for i := range NUM_CHANNELS {
				chip.channels[i].enabled = false
			}

			chip.SetSIDMixerMode(true, tc.dcOffset, false) // No saturation for this test

			// Generate samples and measure DC
			var sum float64
			numSamples := 100
			for range numSamples {
				sample := chip.GenerateSample()
				sum += float64(sample)
			}
			avg := sum / float64(numSamples)

			// Cleanup
			chip.SetSIDMixerMode(false, 0, false)

			if math.Abs(avg-tc.expectedOffset) > tc.tolerance {
				t.Errorf("expected DC offset ~%f, got %f", tc.expectedOffset, avg)
			}
		})
	}
}

// TestSIDMixer_SoftSaturation verifies that the mixer applies soft saturation
// when multiple voices are at high amplitude.
func TestSIDMixer_SoftSaturation(t *testing.T) {
	// Enable saturation, no DC offset for cleaner test
	chip.SetSIDMixerMode(true, 0, true)

	// Configure three channels at full volume with same frequency
	for i := range 3 {
		chip.channels[i].waveType = WAVE_SQUARE
		chip.channels[i].frequency = 440
		chip.channels[i].volume = 1.0
		chip.channels[i].enabled = true
		chip.channels[i].gate = true
		chip.channels[i].envelopeLevel = 1.0
		chip.channels[i].envelopePhase = ENV_SUSTAIN
		chip.channels[i].sustainLevel = 1.0
		chip.channels[i].dutyCycle = 0.5
		chip.channels[i].phase = 0
	}
	// Disable channel 4
	chip.channels[3].enabled = false

	// Generate samples and find peak
	var maxSample float32
	for range 1000 {
		sample := chip.GenerateSample()
		absSample := sample
		if absSample < 0 {
			absSample = -absSample
		}
		if absSample > maxSample {
			maxSample = absSample
		}
	}

	// Cleanup
	chip.SetSIDMixerMode(false, 0, false)
	for i := range 3 {
		chip.channels[i].enabled = false
	}

	// With soft saturation, peak should be compressed
	// but still above single channel output
	if maxSample > 1.5 {
		t.Errorf("saturation should compress output, got peak: %f", maxSample)
	}
	if maxSample < 0.3 {
		t.Errorf("output should still be significant, got peak: %f", maxSample)
	}
}

// TestSIDMixer_SaturationDisabled verifies that mixer mode can be disabled.
func TestSIDMixer_SaturationDisabled(t *testing.T) {
	// Disable mixer mode entirely
	chip.SetSIDMixerMode(false, 0, false)

	// Configure one channel
	chip.channels[0].waveType = WAVE_SQUARE
	chip.channels[0].frequency = 440
	chip.channels[0].volume = 1.0
	chip.channels[0].enabled = true
	chip.channels[0].gate = true
	chip.channels[0].envelopeLevel = 1.0
	chip.channels[0].envelopePhase = ENV_SUSTAIN
	chip.channels[0].sustainLevel = 1.0
	chip.channels[0].dutyCycle = 0.5

	// Disable other channels
	for i := 1; i < NUM_CHANNELS; i++ {
		chip.channels[i].enabled = false
	}

	// Generate some samples
	var sum float64
	for range 100 {
		sample := chip.GenerateSample()
		sum += float64(sample)
	}
	avg := sum / 100.0

	// Cleanup
	chip.channels[0].enabled = false

	// With mixer mode disabled, DC should be near zero (just channel output)
	if math.Abs(avg) > 0.5 {
		t.Errorf("with mixer disabled, DC should be minimal, got: %f", avg)
	}
}

// TestSIDMixer_SoftClipFunction tests the soft clip function directly.
func TestSIDMixer_SoftClipFunction(t *testing.T) {
	tests := []struct {
		name     string
		input    float32
		checkFn  func(float32) bool
		checkMsg string
	}{
		{
			"below_threshold_unchanged",
			0.5,
			func(out float32) bool { return math.Abs(float64(out-0.5)) < 0.01 },
			"values below threshold should be unchanged",
		},
		{
			"positive_peak_clipped",
			2.0,
			func(out float32) bool { return out < 2.0 && out > 0.85 },
			"positive peaks should be soft clipped",
		},
		{
			"negative_peak_clipped",
			-2.0,
			func(out float32) bool { return out > -2.0 && out < -0.80 },
			"negative peaks should be soft clipped",
		},
		{
			"asymmetric_clipping",
			1.5,
			func(out float32) bool {
				posOut := sidMixerSoftClip(1.5)
				negOut := sidMixerSoftClip(-1.5)
				// Should be asymmetric
				return math.Abs(float64(posOut+negOut)) > 0.01
			},
			"clipping should be asymmetric",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := sidMixerSoftClip(tc.input)
			if !tc.checkFn(out) {
				t.Errorf("%s: input=%f, output=%f", tc.checkMsg, tc.input, out)
			}
		})
	}
}
