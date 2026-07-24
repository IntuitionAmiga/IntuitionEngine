//go:build !js

// profile_cpu_signal.go - flush a CPU profile when the process is interrupted.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// installCPUProfileSignalStop arranges for an in-progress CPU profile to be
// flushed when the process is interrupted from the terminal. It is installed
// only while profiling is active, so the default signal disposition is
// unchanged for ordinary runs.
func installCPUProfileSignalStop() {
	if os.Getenv(cpuProfileEnvVar) == "" {
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-ch
		stopCPUProfile()
		// This handler owns the interrupt when profiling is active, so it also
		// flushes the subsystem perf report (a no-op unless IE_PERF_ACCT=1).
		// installPerfReportExit stands down in that case, so the report is not
		// raced by a second handler that could exit before the profile flushes.
		dumpSubsysPerfReport()
		signal.Stop(ch)
		// Re-raise so the process still terminates the way it would have.
		// os.Process.Signal is not implemented on Windows, so fall back to an
		// explicit exit there; without it the interrupt would be swallowed and
		// the emulator would keep running.
		signal.Reset(os.Interrupt, syscall.SIGTERM)
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			if err := p.Signal(sig); err == nil {
				// Give the re-raised signal a moment to terminate the process
				// before falling through to the explicit exit below.
				time.Sleep(500 * time.Millisecond)
			}
		}
		os.Exit(1)
	}()
}
