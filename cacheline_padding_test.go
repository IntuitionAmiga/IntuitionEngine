// cacheline_padding_test.go - static guard on the padded hot structs.
//
// Tranche 4 item 13 pads only where false sharing is confirmed by measurement
// (BenchmarkSharedCounters_Parallel and the ring's own producer/consumer paths).
// The one struct padded is audioEventRing: its head and tail, and its published
// and applied counters, are each stored by a different goroutine on the hot
// path. This test pins that separation so a later field reshuffle cannot quietly
// fold two of them back onto one cache line.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"testing"
	"unsafe"
)

func TestStructPadding_HotStructsAligned(t *testing.T) {
	var r audioEventRing

	pairs := []struct {
		name string
		a, b uintptr
	}{
		{"head/tail", unsafe.Offsetof(r.head), unsafe.Offsetof(r.tail)},
		{"published/applied", unsafe.Offsetof(r.published), unsafe.Offsetof(r.applied)},
	}
	for _, p := range pairs {
		gap := p.b - p.a
		if p.b < p.a {
			gap = p.a - p.b
		}
		if gap < cacheLineBytes {
			t.Errorf("%s only %d bytes apart, want at least %d so they fall on separate cache lines",
				p.name, gap, cacheLineBytes)
		}
	}
}
