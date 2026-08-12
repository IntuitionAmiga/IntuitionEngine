// profile_cpu_test.go - CPU profile capture hook.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCPUProfile_StartWritesProfileAndStopIsIdempotent checks the capture
// mechanism used to produce default.pgo inputs: a profile is written, a
// second start is refused while one is running, and stop can be called from
// any number of shutdown paths.
func TestCPUProfile_StartWritesProfileAndStopIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cpu.pprof")

	if err := startCPUProfile(path); err != nil {
		t.Fatalf("startCPUProfile: %v", err)
	}
	t.Cleanup(stopCPUProfile)

	if err := startCPUProfile(path); err == nil {
		t.Fatal("second startCPUProfile should fail while a profile is running")
	}

	// Burn a little CPU so the profile has at least one sample.
	sum := 0
	for i := 0; i < 5_000_000; i++ {
		sum += i % 7
	}
	_ = sum

	stopCPUProfile()
	stopCPUProfile() // must be safe to repeat

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("CPU profile is empty")
	}
}

// TestCPUProfile_EmptyPathIsNoOp confirms that an unset IE_CPUPROFILE leaves
// profiling entirely disabled.
func TestCPUProfile_EmptyPathIsNoOp(t *testing.T) {
	if err := startCPUProfile(""); err != nil {
		t.Fatalf("startCPUProfile(\"\"): %v", err)
	}
	cpuProfileMu.Lock()
	running := cpuProfileFile != nil
	cpuProfileMu.Unlock()
	if running {
		t.Fatal("empty path started a profile")
	}
	stopCPUProfile()
}

func TestRuntimeAudioCleanupRunsOnce(t *testing.T) {
	var calls int
	registerRuntimeAudioCleanup(func() { calls++ })
	runRuntimeAudioCleanup()
	runRuntimeAudioCleanup()
	if calls != 1 {
		t.Fatalf("audio cleanup calls = %d, want 1", calls)
	}
}
