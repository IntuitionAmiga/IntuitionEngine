//go:build headless

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAROSInterpreterBootEnvironment_ConfiguresInterpreterHarness(t *testing.T) {
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, t.TempDir())
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	if env.Runner.cpu.m68kJitEnabled {
		t.Fatal("AROS interpreter boot environment unexpectedly enabled JIT")
	}
	if env.Harness.CPU != env.Runner.cpu {
		t.Fatal("boot harness CPU does not match runner CPU")
	}
	if env.Harness.Loader != env.Loader {
		t.Fatal("boot harness loader does not match AROS loader")
	}
	if env.Bus.Read32(CLIP_REGION_BASE) != 0 {
		t.Fatalf("clipboard bridge MMIO not mapped as expected at 0x%X", CLIP_REGION_BASE)
	}
}

func TestAROSInterpreterBoundedBootAcceptance(t *testing.T) {
	rom, err := os.ReadFile("sdk/roms/aros-ie-m68k.rom")
	if err != nil {
		t.Skipf("AROS ROM not available: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	driveRoot := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if !isAROSDrivePath(driveRoot) {
		t.Skip("AROS drive tree not available; skipping bounded boot acceptance")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := env.BootAndWait(ctx)
	if err != nil {
		t.Fatalf("BootAndWait() failed: %v", err)
	}
	if result.TimedOut {
		t.Fatalf("AROS bounded boot timed out: ready=%+v faults=%v", result.Ready, result.Faults)
	}
	if len(result.Faults) != 0 {
		t.Fatalf("AROS bounded boot hit structured faults: %+v", result.Faults)
	}
	if !result.Ready.Ready {
		t.Fatalf("AROS bounded boot did not reach ready state: %+v", result.Ready)
	}
}
