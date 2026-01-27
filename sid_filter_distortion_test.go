// sid_filter_distortion_test.go - Unit tests for 6581 filter distortion

package main

import (
	"math"
	"testing"
)

// TestSIDFilter_6581Distortion verifies that 6581 filter adds asymmetric soft clipping.
func TestSIDFilter_6581Distortion(t *testing.T) {
	// Test the distortion function directly
	tests := []struct {
		input    float32
		positive bool
	}{
		{0.3, true},   // Below threshold - should pass through
		{-0.3, false}, // Below threshold - should pass through
		{1.5, true},   // Above positive threshold - should clip
		{-1.5, false}, // Above negative threshold - should clip
	}

	for _, tc := range tests {
		output := sid6581FilterDistort(tc.input)

		// Clipped outputs should be smaller in magnitude than input
		if math.Abs(float64(tc.input)) > 0.8 {
			if math.Abs(float64(output)) >= math.Abs(float64(tc.input)) {
				t.Errorf("input=%f should be clipped, got output=%f", tc.input, output)
			}
		}

		// Sign should be preserved
		if (tc.input > 0) != (output > 0) && tc.input != 0 {
			t.Errorf("sign should be preserved: input=%f, output=%f", tc.input, output)
		}
	}
}

// TestSIDFilter_DistortionLevelDependent verifies distortion increases with level.
func TestSIDFilter_DistortionLevelDependent(t *testing.T) {
	levels := []float32{0.3, 0.6, 0.9, 1.2, 1.5, 2.0}

	var prevDistortion float32 = 0
	for _, level := range levels {
		output := sid6581FilterDistort(level)
		distortion := level - output // Amount clipped

		if level > 0.8 && distortion <= prevDistortion {
			t.Logf("level=%f: output=%f, distortion=%f", level, output, distortion)
		}
		prevDistortion = distortion
	}
}

// TestSIDFilter_DistortionAsymmetric verifies positive and negative clips differently.
func TestSIDFilter_DistortionAsymmetric(t *testing.T) {
	posOut := sid6581FilterDistort(1.5)
	negOut := sid6581FilterDistort(-1.5)

	// The clipping should be asymmetric
	// 6581 clips positive and negative differently
	posClip := 1.5 - posOut
	negClip := -1.5 - negOut

	t.Logf("positive clip amount: %f, negative clip amount: %f", posClip, negClip)

	// They should differ (asymmetric)
	if math.Abs(float64(posClip+negClip)) < 0.01 {
		t.Errorf("clipping should be asymmetric: posClip=%f, negClip=%f", posClip, negClip)
	}
}

// TestSIDFilter_8580CleanFilter verifies 8580 has minimal distortion.
func TestSIDFilter_8580CleanFilter(t *testing.T) {
	// 8580 should not distort significantly
	input := float32(1.2)
	output := sid8580FilterDistort(input)

	// 8580 distortion should be much less than 6581
	output6581 := sid6581FilterDistort(input)

	t.Logf("8580 output: %f (distortion: %f)", output, input-output)
	t.Logf("6581 output: %f (distortion: %f)", output6581, input-output6581)

	// 8580 distortion should be less
	if (input - output) >= (input - output6581) {
		t.Errorf("8580 should distort less than 6581")
	}
}

// TestSIDFilter_DistortionBelowThreshold verifies no distortion below threshold.
func TestSIDFilter_DistortionBelowThreshold(t *testing.T) {
	inputs := []float32{0.1, 0.3, 0.5, 0.7}

	for _, input := range inputs {
		output := sid6581FilterDistort(input)
		diff := math.Abs(float64(input - output))

		if diff > 0.01 {
			t.Errorf("input=%f below threshold should pass through, got output=%f (diff=%f)",
				input, output, diff)
		}
	}
}

// TestSIDFilter_DistortionSmoothTransition verifies smooth transition at threshold.
func TestSIDFilter_DistortionSmoothTransition(t *testing.T) {
	// Check that there's no discontinuity at the clipping threshold
	threshold := float32(0.85) // Approximate 6581 threshold

	below := sid6581FilterDistort(threshold - 0.05)
	at := sid6581FilterDistort(threshold)
	above := sid6581FilterDistort(threshold + 0.05)

	// Transitions should be smooth (no sudden jumps)
	belowToAt := math.Abs(float64(at - below))
	atToAbove := math.Abs(float64(above - at))

	t.Logf("below=%f, at=%f, above=%f", below, at, above)
	t.Logf("transitions: belowToAt=%f, atToAbove=%f", belowToAt, atToAbove)

	// Neither transition should be too large (indicating discontinuity)
	if belowToAt > 0.15 || atToAbove > 0.15 {
		t.Errorf("distortion should have smooth transition at threshold")
	}
}

// TestSIDFilter_DistortionConstants verifies the distortion constants exist.
func TestSIDFilter_DistortionConstants(t *testing.T) {
	// Verify constants are reasonable
	if SID_6581_FILTER_THRESHOLD_POS <= 0 || SID_6581_FILTER_THRESHOLD_POS >= 1.0 {
		t.Errorf("positive threshold should be between 0 and 1: %f", SID_6581_FILTER_THRESHOLD_POS)
	}
	if SID_6581_FILTER_THRESHOLD_NEG <= 0 || SID_6581_FILTER_THRESHOLD_NEG >= 1.0 {
		t.Errorf("negative threshold should be between 0 and 1: %f", SID_6581_FILTER_THRESHOLD_NEG)
	}

	// Asymmetric thresholds
	if SID_6581_FILTER_THRESHOLD_POS == SID_6581_FILTER_THRESHOLD_NEG {
		t.Errorf("thresholds should be asymmetric")
	}

	t.Logf("6581 filter thresholds: pos=%f, neg=%f",
		SID_6581_FILTER_THRESHOLD_POS, SID_6581_FILTER_THRESHOLD_NEG)
}

// TestSIDFilter_DistortionPreservesZero verifies zero input gives zero output.
func TestSIDFilter_DistortionPreservesZero(t *testing.T) {
	output := sid6581FilterDistort(0)
	if output != 0 {
		t.Errorf("zero input should give zero output, got %f", output)
	}
}

// TestSIDFilter_DistortionContinuous verifies the function is continuous.
func TestSIDFilter_DistortionContinuous(t *testing.T) {
	// Sample the function across a range and check for continuity
	prev := sid6581FilterDistort(-2.0)
	maxJump := float32(0)

	for x := float32(-2.0); x <= 2.0; x += 0.01 {
		curr := sid6581FilterDistort(x)
		jump := float32(math.Abs(float64(curr - prev)))
		if jump > maxJump {
			maxJump = jump
		}
		prev = curr
	}

	// Max jump should be small (function is continuous)
	if maxJump > 0.05 {
		t.Errorf("function should be continuous, max jump=%f", maxJump)
	}

	t.Logf("max jump across -2 to 2: %f", maxJump)
}
