//go:build !js

// perf_report_exit_signal.go - dump the subsystem perf report on terminal interrupt.

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// installPerfReportExit prints the subsystem perf report when the process is
// interrupted from the terminal (Ctrl-C / SIGTERM), then re-raises so the
// process still terminates. Installed only when accounting is on.
func installPerfReportExit() {
	if !perfAcctOn {
		return
	}
	// When CPU profiling is also active, its interrupt handler owns the signal
	// and flushes the perf report itself (see installCPUProfileSignalStop). A
	// second handler here would race it and could exit before the profile is
	// flushed, truncating it. exitProfiled and the normal end-of-main path still
	// dump the report on a clean exit.
	if os.Getenv(cpuProfileEnvVar) != "" {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-ch
		dumpSubsysPerfReport()
		signal.Stop(ch)
		signal.Reset(os.Interrupt, syscall.SIGTERM)
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			_ = p.Signal(sig)
		}
		os.Exit(1)
	}()
}
