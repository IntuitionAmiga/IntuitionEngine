// ahx_waves_test.go - Tests for AHX waveform generation

package main

import (
	"testing"
)

// TestGenerateTriangle tests triangle waveform generation
func TestGenerateTriangle(t *testing.T) {
	waves := NewAHXWaves()

	// Test Triangle04 (4 bytes)
	// Triangle goes: 0 -> 127 -> 0 -> -128 -> 0
	// For 4 samples: should be 0, 127, 0, -128 (approximately)
	tri04 := waves.Triangle04[:]
	if len(tri04) != 4 {
		t.Fatalf("Triangle04 should be 4 bytes, got %d", len(tri04))
	}
	// First sample should be near 0
	if tri04[0] < -10 || tri04[0] > 10 {
		t.Errorf("Triangle04[0] should be near 0, got %d", tri04[0])
	}
	// Peak should be around 127
	if tri04[1] < 120 {
		t.Errorf("Triangle04[1] should be near 127, got %d", tri04[1])
	}

	// Test Triangle80 (128 bytes)
	tri80 := waves.Triangle80[:]
	if len(tri80) != 0x80 {
		t.Fatalf("Triangle80 should be 128 bytes, got %d", len(tri80))
	}
	// Check that we have a proper triangle shape
	// Should start near 0, go up to 127, back to 0, down to -128
	maxVal := int8(-128)
	minVal := int8(127)
	for _, v := range tri80 {
		if v > maxVal {
			maxVal = v
		}
		if v < minVal {
			minVal = v
		}
	}
	if maxVal < 120 {
		t.Errorf("Triangle80 max should be near 127, got %d", maxVal)
	}
	if minVal > -120 {
		t.Errorf("Triangle80 min should be near -128, got %d", minVal)
	}
}

// TestGenerateSawtooth tests sawtooth waveform generation
func TestGenerateSawtooth(t *testing.T) {
	waves := NewAHXWaves()

	// Test Sawtooth04 (4 bytes)
	saw04 := waves.Sawtooth04[:]
	if len(saw04) != 4 {
		t.Fatalf("Sawtooth04 should be 4 bytes, got %d", len(saw04))
	}
	// Sawtooth should go from -128 to ~127 linearly
	if saw04[0] > -100 {
		t.Errorf("Sawtooth04[0] should be near -128, got %d", saw04[0])
	}

	// Test Sawtooth80 (128 bytes)
	saw80 := waves.Sawtooth80[:]
	if len(saw80) != 0x80 {
		t.Fatalf("Sawtooth80 should be 128 bytes, got %d", len(saw80))
	}
	// First sample should be -128
	if saw80[0] != -128 {
		t.Errorf("Sawtooth80[0] should be -128, got %d", saw80[0])
	}
	// Should increase monotonically
	for i := 1; i < len(saw80); i++ {
		if saw80[i] < saw80[i-1] {
			t.Errorf("Sawtooth should increase: saw80[%d]=%d < saw80[%d]=%d", i, saw80[i], i-1, saw80[i-1])
			break
		}
	}
}

// TestAHXWavesTableSizes verifies all waveform table sizes
func TestAHXWavesTableSizes(t *testing.T) {
	waves := NewAHXWaves()

	testCases := []struct {
		name     string
		actual   int
		expected int
	}{
		{"Triangle04", len(waves.Triangle04), 0x04},
		{"Triangle08", len(waves.Triangle08), 0x08},
		{"Triangle10", len(waves.Triangle10), 0x10},
		{"Triangle20", len(waves.Triangle20), 0x20},
		{"Triangle40", len(waves.Triangle40), 0x40},
		{"Triangle80", len(waves.Triangle80), 0x80},
		{"Sawtooth04", len(waves.Sawtooth04), 0x04},
		{"Sawtooth08", len(waves.Sawtooth08), 0x08},
		{"Sawtooth10", len(waves.Sawtooth10), 0x10},
		{"Sawtooth20", len(waves.Sawtooth20), 0x20},
		{"Sawtooth40", len(waves.Sawtooth40), 0x40},
		{"Sawtooth80", len(waves.Sawtooth80), 0x80},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.actual != tc.expected {
				t.Errorf("%s: expected %d bytes, got %d", tc.name, tc.expected, tc.actual)
			}
		})
	}
}

// TestVibratoTable tests the vibrato table values
func TestVibratoTable(t *testing.T) {
	// VibratoTable should be a 64-entry sine-like table
	if len(AHXVibratoTable) != 64 {
		t.Fatalf("VibratoTable should have 64 entries, got %d", len(AHXVibratoTable))
	}

	// Check key values from the C++ reference
	expectedValues := map[int]int{
		0:  0,
		16: 255,
		32: 0,
		48: -255,
	}

	for idx, expected := range expectedValues {
		if AHXVibratoTable[idx] != expected {
			t.Errorf("VibratoTable[%d]: expected %d, got %d", idx, expected, AHXVibratoTable[idx])
		}
	}
}

// TestPeriodTable tests the period table values
func TestPeriodTable(t *testing.T) {
	// PeriodTable should have 61 entries (notes 0-60)
	if len(AHXPeriodTable) != 61 {
		t.Fatalf("PeriodTable should have 61 entries, got %d", len(AHXPeriodTable))
	}

	// Check key values from the C++ reference
	if AHXPeriodTable[0] != 0x0000 {
		t.Errorf("PeriodTable[0]: expected 0x0000, got 0x%04X", AHXPeriodTable[0])
	}
	if AHXPeriodTable[1] != 0x0D60 {
		t.Errorf("PeriodTable[1]: expected 0x0D60, got 0x%04X", AHXPeriodTable[1])
	}
	if AHXPeriodTable[60] != 0x0071 {
		t.Errorf("PeriodTable[60]: expected 0x0071, got 0x%04X", AHXPeriodTable[60])
	}
}
