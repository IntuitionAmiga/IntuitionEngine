package main

import (
	"strings"
	"testing"
)

// TestSIMDStatusReportsState is an always-built smoke check: SIMDStatus must
// describe the dispatch state without triggering SIMD execution, on every build.
func TestSIMDStatusReportsState(t *testing.T) {
	status := SIMDStatus()
	if !strings.Contains(status, "SIMD:") {
		t.Fatalf("SIMDStatus missing prefix: %q", status)
	}
	// active implies both requested and host-supported; never active without them.
	if simdKernelsActive && (!simdRequested || !simdHostSupported()) {
		t.Fatalf("simdKernelsActive true but requested=%v supported=%v", simdRequested, simdHostSupported())
	}
}
