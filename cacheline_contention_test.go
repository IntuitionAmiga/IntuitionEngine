// cacheline_contention_test.go - measurement for the cache-line separation item.
//
// Tranche 4 item 13 pads a struct only where false sharing is confirmed by
// measurement, never on suspicion. This bench establishes whether false sharing
// is observable on the host at all: two atomic counters bumped by parallel
// goroutines, once adjacent in the same cache line and once padded onto separate
// lines. If the padded variant is not materially faster under contention there is
// nothing to pad, and the item descopes to nothing.
//
// Run: go test -tags headless -run=^$ -bench BenchmarkSharedCounters_Parallel -cpu 8 .
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

// adjacentCounters puts two hot atomics in the same cache line.
type adjacentCounters struct {
	a atomic.Int64
	b atomic.Int64
}

// paddedCounters separates the two atomics onto distinct cache lines.
type paddedCounters struct {
	a atomic.Int64
	_ [cacheLineBytes - 8]byte
	b atomic.Int64
}

// benchTwoWriters hammers two counters from two goroutines for the whole run and
// returns only after both have finished, so the wall-clock reflects the contended
// steady state, not goroutine start-up.
func benchTwoWriters(b *testing.B, bumpA, bumpB func()) {
	b.Helper()
	half := b.N / 2
	var wg sync.WaitGroup
	wg.Add(2)
	b.ResetTimer()
	go func() {
		defer wg.Done()
		for i := 0; i < half; i++ {
			bumpA()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < half; i++ {
			bumpB()
		}
	}()
	wg.Wait()
}

func BenchmarkSharedCounters_Parallel(b *testing.B) {
	b.Run("Adjacent", func(b *testing.B) {
		var c adjacentCounters
		benchTwoWriters(b,
			func() { c.a.Add(1) },
			func() { c.b.Add(1) })
	})
	b.Run("Padded", func(b *testing.B) {
		var c paddedCounters
		benchTwoWriters(b,
			func() { c.a.Add(1) },
			func() { c.b.Add(1) })
	})
}
