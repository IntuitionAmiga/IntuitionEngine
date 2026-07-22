// profile_cpu.go - optional CPU profile capture for profile-guided builds.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"sync"
)

// cpuProfileEnvVar names the environment variable that enables CPU profile
// capture. Setting it to a writable path starts a profile at boot and stops
// it on clean shutdown. The profiles produced this way are the input to
// default.pgo; see sdk/docs/architecture.md.
const cpuProfileEnvVar = "IE_CPUPROFILE"

var (
	cpuProfileMu   sync.Mutex
	cpuProfileFile *os.File
)

// startCPUProfile begins writing a CPU profile to path. It is a no-op when
// path is empty. A profile already in progress is left untouched.
func startCPUProfile(path string) error {
	if path == "" {
		return nil
	}

	cpuProfileMu.Lock()
	defer cpuProfileMu.Unlock()

	if cpuProfileFile != nil {
		return fmt.Errorf("CPU profile already running")
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CPU profile %q: %w", path, err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("start CPU profile %q: %w", path, err)
	}
	cpuProfileFile = f
	return nil
}

// stopCPUProfile flushes and closes an in-progress CPU profile. It is safe to
// call when no profile is running and safe to call more than once, so every
// shutdown path can invoke it unconditionally.
func stopCPUProfile() {
	cpuProfileMu.Lock()
	defer cpuProfileMu.Unlock()

	if cpuProfileFile == nil {
		return
	}
	pprof.StopCPUProfile()
	_ = cpuProfileFile.Close()
	cpuProfileFile = nil
}

// exitProfiled flushes any in-progress CPU profile and then terminates the
// process. os.Exit skips deferred cleanup, so every exit in main.go goes
// through here; without a profile running it is exactly os.Exit.
func exitProfiled(code int) {
	stopCPUProfile()
	os.Exit(code)
}

// startCPUProfileFromEnv starts a profile when IE_CPUPROFILE is set. Failures
// are reported but never fatal: profiling is a diagnostic aid, not a
// precondition for running the machine.
func startCPUProfileFromEnv() {
	path := os.Getenv(cpuProfileEnvVar)
	if path == "" {
		return
	}
	if err := startCPUProfile(path); err != nil {
		fmt.Printf("Warning: %v\n", err)
		return
	}
	fmt.Printf("CPU profile: writing to %s\n", path)
}
