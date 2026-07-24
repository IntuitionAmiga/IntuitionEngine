// boot_gc_test.go - Slice 12: boot GC sweep tidy

package main

import (
	"runtime/metrics"
	"testing"
)

func gcCycles() uint64 {
	const key = "/gc/cycles/total:gc-cycles"
	samples := []metrics.Sample{{Name: key}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return samples[0].Value.Uint64()
}

// TestBootForcedGC_RunsExactlyOneCollection proves each boot sweep site forces
// exactly one collection: the test-only counter advances by one and the
// runtime records at least one additional GC cycle. This stands in for the two
// boot sites (initial BASIC boot and full reset), which each call bootForcedGC
// once before execution.
func TestBootForcedGC_RunsExactlyOneCollection(t *testing.T) {
	startCount := bootForcedGCCount.Load()
	startCycles := gcCycles()

	bootForcedGC()

	if got := bootForcedGCCount.Load(); got != startCount+1 {
		t.Fatalf("bootForcedGCCount = %d, want %d (exactly one sweep)", got, startCount+1)
	}
	if got := gcCycles(); got < startCycles+1 {
		t.Fatalf("gc-cycles delta = %d, want >= 1 (a real collection ran)", got-startCycles)
	}
}
