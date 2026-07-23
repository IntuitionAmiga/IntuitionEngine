// timing_idle_profile_test.go - measurement harness for the timing-service decision gate.
//
// Tranche 4 item 12 is measurement first: a timing service is built only if an
// idle booted machine spends material CPU on wakeups, or wakes far more often
// than the frames it produces. This harness runs the production idle clocks (the
// compositor scheduler at COMPOSITOR_REFRESH_INTERVAL and a video-source clock at
// REFRESH_INTERVAL, the two guest-independent 60 Hz cadences an idle machine
// runs) for a fixed window, counts wakeups, and optionally writes a CPU profile.
//
// It is skipped unless IE_PROFILE=1 so it never costs CI time; it is a bench-style
// instrument, run by hand, and its numbers are recorded in the tranche notes.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"time"
)

// TestIdleMachine_ProfileHarness measures the wakeup cadence and CPU cost of the
// idle machine's guest-independent clocks. Skipped unless IE_PROFILE=1.
func TestIdleMachine_ProfileHarness(t *testing.T) {
	if os.Getenv("IE_PROFILE") != "1" {
		t.Skip("set IE_PROFILE=1 to run the idle-machine profile harness")
	}

	const window = 2 * time.Second

	var compositorWakes atomic.Int64
	var sourceWakes atomic.Int64

	compositor := NewVideoScheduler(COMPOSITOR_REFRESH_INTERVAL)
	compositor.Register(func() { compositorWakes.Add(1) })

	source := NewVideoScheduler(REFRESH_INTERVAL)
	source.Register(func() { sourceWakes.Add(1) })

	goroutinesBefore := runtime.NumGoroutine()

	var profFile *os.File
	if path := os.Getenv("IE_CPUPROFILE"); path != "" {
		var err error
		profFile, err = os.Create(path)
		if err != nil {
			t.Fatalf("create cpu profile: %v", err)
		}
		if err := pprof.StartCPUProfile(profFile); err != nil {
			t.Fatalf("start cpu profile: %v", err)
		}
	}

	var msBefore runtime.MemStats
	runtime.ReadMemStats(&msBefore)

	compositor.Start()
	source.Start()
	time.Sleep(window)
	compositor.Stop()
	source.Stop()

	if profFile != nil {
		pprof.StopCPUProfile()
		_ = profFile.Close()
	}

	var msAfter runtime.MemStats
	runtime.ReadMemStats(&msAfter)

	cw := compositorWakes.Load()
	sw := sourceWakes.Load()
	totalWakes := cw + sw
	seconds := window.Seconds()

	t.Logf("window                : %v", window)
	t.Logf("compositor wakeups    : %d (%.1f/s)", cw, float64(cw)/seconds)
	t.Logf("source wakeups        : %d (%.1f/s)", sw, float64(sw)/seconds)
	t.Logf("total wakeups         : %d (%.1f/s)", totalWakes, float64(totalWakes)/seconds)
	t.Logf("frames produced (60Hz): ~%.0f expected", 2*60*seconds)
	t.Logf("goroutines before/after idle infra: %d / %d", goroutinesBefore, runtime.NumGoroutine())
	t.Logf("heap allocs over window: %d", msAfter.Mallocs-msBefore.Mallocs)
	t.Logf("(set IE_CPUPROFILE=<path> for a CPU profile of the idle window)")
}
