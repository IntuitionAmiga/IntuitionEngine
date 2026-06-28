//go:build headless && m68k_test && amd64

package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestM68KJIT_AROSNativeBoundaryLockstep(t *testing.T) {
	if os.Getenv("IE_M68K_JIT_LOCKSTEP_AROS") != "1" {
		t.Skip("set IE_M68K_JIT_LOCKSTEP_AROS=1 to run the AROS native-boundary lockstep oracle")
	}
	fromPC := m68kJITLockstepEnvUint32("IE_M68K_JIT_LOCKSTEP_FROM_PC", 0x0064D9C0)
	toPC := m68kJITLockstepEnvUint32("IE_M68K_JIT_LOCKSTEP_TO_PC", 0x0064DA40)
	maxSamples := m68kJITLockstepEnvInt("IE_M68K_JIT_LOCKSTEP_MAX", 1<<20)
	armAfterDOS := m68kJITLockstepEnvInt("IE_M68K_JIT_LOCKSTEP_AFTER_DOS", 0)
	waitSeconds := m68kJITLockstepEnvInt("IE_M68K_JIT_LOCKSTEP_WAIT_SECONDS", 90)
	target := m68kJITAROSDiagnosticTraceTarget(4096)

	ref := newM68KJITLockstepReference(fromPC, toPC, maxSamples)
	refEvents := runM68KJITAROSLockstepPass(t, "reference-interpreter", ref, armAfterDOS, target, time.Duration(waitSeconds)*time.Second, true)
	refTrace := ref.ReferenceSnapshot()
	if len(refTrace) == 0 {
		t.Fatalf("reference pass recorded no lockstep snapshots in PC range %08X-%08X after %d DOS events", fromPC, toPC, refEvents)
	}
	t.Logf("reference pass recorded %d lockstep snapshots in PC range %08X-%08X after %d DOS events", len(refTrace), fromPC, toPC, refEvents)

	candidate := newM68KJITLockstepCandidate(fromPC, toPC, maxSamples, refTrace)
	candEvents := runM68KJITAROSLockstepPass(t, "candidate-native", candidate, armAfterDOS, target, time.Duration(waitSeconds)*time.Second, false)
	if mismatch := candidate.Mismatch(); mismatch != nil {
		t.Fatalf("%s", mismatch.String())
	}
	t.Logf("candidate pass matched reference through %d DOS events in PC range %08X-%08X", candEvents, fromPC, toPC)
}

func runM68KJITAROSLockstepPass(t *testing.T, label string, session *m68kJITLockstepSession, armAfterDOS, targetEvents int, wait time.Duration, interpreterOnly bool) int {
	t.Helper()
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}
	env, err := NewAROSBootEnvironmentWithOptions(rom, isolatedAROSDriveRoot(t, label), AROSBootEnvironmentOptions{
		DeterministicIRQs: true,
		InterpreterOnly:   interpreterOnly,
		NoNativeCeiling:   true,
	})
	if err != nil {
		t.Fatalf("%s: new AROS boot environment failed: %v", label, err)
	}
	defer env.Close()

	startAROSDiagnosticCompositor(t, env)
	bootWait := 30 * time.Second
	if wait > bootWait {
		bootWait = wait
	}
	env.Harness.Timeout = bootWait
	env.CPU.m68kJitPersist = true
	if armAfterDOS <= 0 {
		env.CPU.m68kJitLockstep = session
	} else {
		env.DOS.CommandRecordedHook = func(count int, _ ArosDOSCommandEvent) {
			if count >= armAfterDOS && env.CPU.m68kJitLockstep == nil {
				env.CPU.m68kJitLockstep = session
			}
		}
	}
	defer func() {
		env.CPU.m68kJitLockstep = nil
		if env.DOS != nil {
			env.DOS.CommandRecordedHook = nil
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), bootWait)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("%s: BootAndWait() failed: %v", label, err)
	}
	if mismatch := session.Mismatch(); mismatch != nil {
		env.Runner.Stop()
		t.Fatalf("%s", mismatch.String())
	}
	if result.TimedOut || len(result.Faults) != 0 || !result.Ready.Ready {
		t.Fatalf("%s: initial boot failed: ready=%+v timedOut=%v faults=%+v", label, result.Ready, result.TimedOut, result.Faults)
	}

	deadline := time.After(wait)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for len(env.DOS.FullCommandsSnapshot()) < targetEvents {
		if mismatch := session.Mismatch(); mismatch != nil {
			env.Runner.Stop()
			t.Fatalf("%s", mismatch.String())
		}
		select {
		case <-deadline:
			got := len(env.DOS.FullCommandsSnapshot())
			ready := ProbeAROSReadyState(env.CPU, env.Loader)
			env.Runner.Stop()
			t.Fatalf("%s: timed out waiting for %d DOS events: got=%d ready=%+v instructions=%d native_blocks=%d fallback=%d pc=%08X sr=%04X",
				label, targetEvents, got, ready, env.CPU.InstructionCount,
				env.CPU.m68kJitNativeBlocksExecuted.Load(), env.CPU.m68kJitFallbackInstructions.Load(),
				env.CPU.PC, env.CPU.SR)
		case <-ticker.C:
		}
	}
	got := len(env.DOS.FullCommandsSnapshot())
	env.Runner.Stop()
	t.Logf("%s: reached %d DOS events instructions=%d native_blocks=%d fallback=%d pc=%08X sr=%04X",
		label, got, env.CPU.InstructionCount,
		env.CPU.m68kJitNativeBlocksExecuted.Load(), env.CPU.m68kJitFallbackInstructions.Load(),
		env.CPU.PC, env.CPU.SR)
	return got
}

func m68kJITLockstepEnvUint32(name string, def uint32) uint32 {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return def
	}
	return uint32(v)
}

func m68kJITLockstepEnvInt(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
