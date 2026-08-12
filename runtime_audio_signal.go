//go:build !js

package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// installRuntimeAudioSignalCleanup covers ordinary runs, where neither the
// CPU-profile nor performance-accounting handlers are installed. The other
// handlers invoke runRuntimeAudioCleanup themselves before they re-raise.
func installRuntimeAudioSignalCleanup() {
	if os.Getenv(cpuProfileEnvVar) != "" || perfAcctOn {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-ch
		runRuntimeAudioCleanup()
		signal.Stop(ch)
		signal.Reset(os.Interrupt, syscall.SIGTERM)
		if process, err := os.FindProcess(os.Getpid()); err == nil && process.Signal(sig) == nil {
			time.Sleep(500 * time.Millisecond)
		}
		os.Exit(1)
	}()
}
