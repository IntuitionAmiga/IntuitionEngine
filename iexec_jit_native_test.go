// iexec_jit_native_test.go - IExec boot test on the native IE64 JIT.
//
// Split from iexec_test.go: ExecuteJIT only exists on native JIT platforms,
// and the shared IExec test helpers must stay buildable on js/wasm for the
// wasm JIT backend's node-run tests.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"strings"
	"testing"
	"time"
)

func TestIExec_M152_HostBackedBootWithJIT(t *testing.T) {
	hostRoot := makeM152Phase5GeneratedHostRoot(t)
	rig, term := assembleAndLoadKernelWithBootstrapHostRootOptions(t, hostRoot, true)
	rig.cpu.jitEnabled = true
	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.ExecuteJIT(); close(done) }()
	time.Sleep(15 * time.Second)
	rig.cpu.running.Store(false)
	waitDoneWithGuard(t, done)

	output := term.DrainOutput()
	for _, substr := range []string{
		"exec.library 1.16.7",
		"1>",
	} {
		if !strings.Contains(output, substr) {
			t.Fatalf("M152_HostBackedBootWithJIT: missing %q output=%q", substr, output[:min(len(output), 1200)])
		}
	}
}
