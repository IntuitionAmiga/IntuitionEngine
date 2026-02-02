// cycles_samples_benchmark_test.go - Benchmarks for cycle-to-sample conversion optimization

package main

import (
	"testing"
)

// BenchmarkCyclesToSamples_Division benchmarks the original division-based approach
func BenchmarkCyclesToSamples_Division(b *testing.B) {
	sampleRate := uint64(44100)
	clockHz := uint64(985248) // PAL SID clock

	cycles := []uint64{0, 1000, 10000, 100000, 1000000}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c := cycles[i%len(cycles)]
		_ = c * sampleRate / clockHz
	}
}

// BenchmarkCyclesToSamples_FixedPoint benchmarks the optimized fixed-point approach
func BenchmarkCyclesToSamples_FixedPoint(b *testing.B) {
	sampleRate := uint64(44100)
	clockHz := uint64(985248) // PAL SID clock

	// Pre-compute multiplier (done once at init)
	sampleMultiplier := (sampleRate << 32) / clockHz

	cycles := []uint64{0, 1000, 10000, 100000, 1000000}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c := cycles[i%len(cycles)]
		_ = (c * sampleMultiplier) >> 32
	}
}

// TestCyclesToSamples_Accuracy verifies the fixed-point conversion accuracy
func TestCyclesToSamples_Accuracy(t *testing.T) {
	testCases := []struct {
		name       string
		sampleRate uint64
		clockHz    uint64
	}{
		{"PAL SID", 44100, 985248},
		{"NTSC SID", 44100, 1022727},
		{"PAL POKEY", 44100, 1773447},
		{"NTSC POKEY", 44100, 1789772},
		{"Z80 3.5MHz", 44100, 3500000},
		{"Z80 4MHz", 44100, 4000000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sampleMultiplier := (tc.sampleRate << 32) / tc.clockHz

			// Test a range of cycle values
			testCycles := []uint64{0, 1, 10, 100, 1000, 10000, 100000, 1000000, 10000000}

			for _, cycles := range testCycles {
				// Original division-based result
				expected := cycles * tc.sampleRate / tc.clockHz

				// Optimized fixed-point result
				actual := (cycles * sampleMultiplier) >> 32

				// Allow ±1 sample deviation due to rounding
				if actual < expected && expected-actual > 1 {
					t.Errorf("cycles=%d: expected %d, got %d (diff=%d)",
						cycles, expected, actual, expected-actual)
				}
				if actual > expected && actual-expected > 1 {
					t.Errorf("cycles=%d: expected %d, got %d (diff=%d)",
						cycles, expected, actual, actual-expected)
				}
			}
		})
	}
}

// BenchmarkEventCollection_Allocations benchmarks event collection allocation behavior
func BenchmarkEventCollection_Allocations(b *testing.B) {
	// Create a mock SID 6502 bus with some events
	bus := newSID6502Bus(false)

	b.Run("OldCollectEvents", func(b *testing.B) {
		// Simulate the old pattern that allocates per frame
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bus.StartFrame()
			// Simulate some SID writes
			bus.Write(0xD400, 0x12)
			bus.Write(0xD401, 0x34)
			bus.Write(0xD404, 0x41)
			// Old allocation path
			events := bus.CollectEvents()
			_ = events
		}
	})

	b.Run("NewGetEvents", func(b *testing.B) {
		// Zero-allocation path
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bus.StartFrame()
			// Simulate some SID writes
			bus.Write(0xD400, 0x12)
			bus.Write(0xD401, 0x34)
			bus.Write(0xD404, 0x41)
			// Zero-allocation path
			events := bus.GetEvents()
			_ = events
			bus.ClearEvents()
		}
	})
}

// TestCyclesToSamples_EdgeCases tests edge cases for the conversion
func TestCyclesToSamples_EdgeCases(t *testing.T) {
	sampleRate := uint64(44100)
	clockHz := uint64(985248)
	sampleMultiplier := (sampleRate << 32) / clockHz

	// Test zero cycles
	if result := (uint64(0) * sampleMultiplier) >> 32; result != 0 {
		t.Errorf("zero cycles should give zero samples, got %d", result)
	}

	// Test very large cycle count (1 hour at PAL rate)
	oneHourCycles := uint64(985248 * 3600)
	expected := oneHourCycles * sampleRate / clockHz
	actual := (oneHourCycles * sampleMultiplier) >> 32

	// Allow larger tolerance for large numbers
	tolerance := expected / 10000 // 0.01%
	if actual < expected-tolerance || actual > expected+tolerance {
		t.Errorf("one hour: expected ~%d, got %d", expected, actual)
	}

	// Test that samples increase monotonically
	var prevSamples uint64
	for cycles := uint64(0); cycles < 100000; cycles += 1000 {
		samples := (cycles * sampleMultiplier) >> 32
		if samples < prevSamples {
			t.Errorf("samples decreased at cycles=%d: %d < %d", cycles, samples, prevSamples)
		}
		prevSamples = samples
	}
}
