// video_gpu_convert_gate_native_test.go - out-of-process gate runner.

//go:build !headless && !js

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runGPUGate runs a registered gate body in a re-executed copy of this binary,
// because Ebiten needs the main OS thread to render or read pixels back; see
// video_gpu_convert_gate.go. It skips when no backend can be started rather
// than failing, so headless and display-less machines still run the suite.
func runGPUGate(t *testing.T, name string) {
	t.Helper()
	runGPUGateOutput(t, name)
}

// runGPUGateOutput is runGPUGate for callers that want what the child printed,
// such as the frame time measurement, whose whole result is its output.
func runGPUGateOutput(t *testing.T, name string) string {
	t.Helper()
	// Only the Unix display backends advertise themselves through the
	// environment. Windows and macOS have a native backend with no such
	// variable, so asking about DISPLAY there would skip on every supported
	// desktop; the child reports backend availability through exit status 3.
	if usesUnixDisplayEnv() && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no display reachable; this gate needs a real graphics backend")
	}
	cmd := exec.Command(os.Args[0], "-test.run=XXXNoTestMatchesThisName")
	cmd.Env = append(os.Environ(), gpuGateEnv+"="+name)
	out, err := runWithDeadline(cmd, 90*time.Second)
	if err == nil {
		return out
	}
	if strings.Contains(err.Error(), "exit status 3") {
		t.Skipf("graphics backend unavailable: %s", out)
	}
	t.Fatalf("gate %q failed: %v\n%s", name, err, out)
	return out
}

// usesUnixDisplayEnv reports whether this platform names its display server in
// the environment, which is what makes an absent DISPLAY meaningful.
func usesUnixDisplayEnv() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "js", "android", "ios":
		return false
	default:
		return true
	}
}

// runWithDeadline runs cmd and kills it if it outlives the deadline, so a
// backend that hangs on window creation fails the test rather than the suite.
func runWithDeadline(cmd *exec.Cmd, limit time.Duration) (string, error) {
	var sb strings.Builder
	cmd.Stdout = &sb
	cmd.Stderr = &sb
	if err := cmd.Start(); err != nil {
		return sb.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return sb.String(), err
	case <-time.After(limit):
		_ = cmd.Process.Kill()
		<-done
		return sb.String(), os.ErrDeadlineExceeded
	}
}
