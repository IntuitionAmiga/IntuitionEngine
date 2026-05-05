//go:build headless

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func bootAROSAndWaitForFaultRegionWrite(t *testing.T, env *AROSInterpreterBootEnvironment) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut || !result.Ready.Ready || len(result.Faults) != 0 {
		t.Fatalf("bounded boot failed: ready=%+v faults=%+v timeout=%v", result.Ready, result.Faults, result.TimedOut)
	}

	cpu := env.CPU
	done := make(chan string, 1)
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		if addr < 0x009E3D80 || addr >= 0x009E3E00 {
			return
		}
		cpu.SetRunning(false)
		select {
		case done <- fmt.Sprintf("write pc=%08X addr=%08X value=%08X size=%d", pc, addr, value, size):
		default:
		}
	}
	defer func() { cpu.DebugWatchFn = nil }()

	select {
	case msg := <-done:
		return msg
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for post-ready fault-region write")
		return ""
	}
}

func TestAROSPostReadyFaultRegionWriteOccursWithoutCoprocessorMMIO(t *testing.T) {
	if os.Getenv("IE_AROS_DIAG") == "" {
		t.Skip("AROS post-ready diagnostic; set IE_AROS_DIAG=1 to run")
	}
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	driveRoot, driveErr := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if driveErr != nil || !isAROSDrivePath(driveRoot) {
		t.Skip("AROS drive tree not available")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	if env.Coproc != nil {
		t.Fatal("default AROS interpreter boot environment unexpectedly mapped coprocessor MMIO")
	}
	if env.Bus.IsIOAddress(COPROC_BASE) || env.Bus.IsIOAddress(COPROC_OP) {
		t.Fatal("coprocessor MMIO is unexpectedly mapped in default AROS interpreter boot environment")
	}

	msg := bootAROSAndWaitForFaultRegionWrite(t, env)
	t.Logf("fault-region write reproduced without coprocessor MMIO: %s", msg)
}

func TestAROSPostReadyFaultRegionWriteWithCoprocessorMMIOMapped(t *testing.T) {
	if os.Getenv("IE_AROS_DIAG") == "" {
		t.Skip("AROS post-ready diagnostic; set IE_AROS_DIAG=1 to run")
	}
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	driveRoot, driveErr := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if driveErr != nil || !isAROSDrivePath(driveRoot) {
		t.Skip("AROS drive tree not available")
	}

	env, err := NewAROSInterpreterBootEnvironmentWithCoprocessor(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironmentWithCoprocessor() failed: %v", err)
	}
	defer env.Close()

	if env.Coproc == nil {
		t.Fatal("coprocessor-enabled AROS interpreter boot environment did not create a coprocessor manager")
	}
	if !env.Bus.IsIOAddress(COPROC_BASE) || !env.Bus.IsIOAddress(COPROC_OP) {
		t.Fatal("coprocessor MMIO is not mapped in coprocessor-enabled AROS interpreter boot environment")
	}

	msg := bootAROSAndWaitForFaultRegionWrite(t, env)
	t.Logf("fault-region write reproduced with coprocessor MMIO: %s opsDispatched=%d workerState=0x%X",
		msg, env.Coproc.opsDispatched, env.Coproc.computeWorkerState())
}
