//go:build headless

package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
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
	if env.Terminal == nil {
		t.Fatal("boot harness terminal MMIO is nil")
	}
	if !env.Terminal.amigaScancodeMode.Load() {
		t.Fatal("boot harness terminal MMIO did not enable Amiga rawkey mode")
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
	driveRoot, driveErr := resolveAROSDrivePath("", filepath.Join(wd, "IntuitionEngine"))
	if driveErr != nil || !isAROSDrivePath(driveRoot) {
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

func TestAROSInterpreterPostReadySoakAcceptance(t *testing.T) {
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
		t.Skip("AROS drive tree not available; skipping post-ready soak acceptance")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	var (
		mu     sync.Mutex
		faults []M68KFaultRecord
	)
	prevHook := env.CPU.FaultHook
	env.CPU.FaultHook = func(record M68KFaultRecord) {
		record = NormalizeM68KFaultRecord(env.CPU, record)
		mu.Lock()
		faults = append(faults, record)
		mu.Unlock()
		if prevHook != nil {
			prevHook(record)
		}
	}
	defer func() { env.CPU.FaultHook = prevHook }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	soakDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(soakDeadline) {
		mu.Lock()
		gotFaults := append([]M68KFaultRecord(nil), faults...)
		mu.Unlock()
		if len(gotFaults) != 0 {
			t.Fatalf("AROS post-ready soak hit structured faults: %+v", gotFaults)
		}
		if !env.CPU.Running() {
			t.Fatalf("AROS CPU halted during post-ready soak at pc=%08X sr=%04X", env.CPU.PC, env.CPU.SR)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestAROSInterpreterPostReadyMouseInputSoakAcceptance(t *testing.T) {
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
		t.Skip("AROS drive tree not available; skipping mouse input soak acceptance")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	if env.Terminal == nil {
		t.Fatal("AROS interpreter boot environment did not expose terminal MMIO")
	}

	var (
		mu     sync.Mutex
		faults []M68KFaultRecord
	)
	prevHook := env.CPU.FaultHook
	env.CPU.FaultHook = func(record M68KFaultRecord) {
		record = NormalizeM68KFaultRecord(env.CPU, record)
		mu.Lock()
		faults = append(faults, record)
		mu.Unlock()
		if prevHook != nil {
			prevHook(record)
		}
	}
	defer func() { env.CPU.FaultHook = prevHook }()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
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

	type mouseSample struct {
		x, y int32
		btn  uint32
	}
	samples := []mouseSample{
		{x: 40, y: 40, btn: 0},
		{x: 320, y: 120, btn: 0},
		{x: 600, y: 220, btn: 1},
		{x: 600, y: 220, btn: 0},
		{x: 180, y: 360, btn: 0},
		{x: 180, y: 360, btn: 2},
		{x: 180, y: 360, btn: 0},
		{x: 760, y: 560, btn: 0},
	}

	for _, sample := range samples {
		env.Terminal.mouseOverride.Store(true)
		env.Terminal.mouseX.Store(sample.x)
		env.Terminal.mouseY.Store(sample.y)
		env.Terminal.mouseButtons.Store(sample.btn)
		env.Terminal.mouseChanged.Store(true)

		stepDeadline := time.Now().Add(250 * time.Millisecond)
		for time.Now().Before(stepDeadline) {
			mu.Lock()
			gotFaults := append([]M68KFaultRecord(nil), faults...)
			mu.Unlock()
			if len(gotFaults) != 0 {
				t.Fatalf("AROS mouse input soak hit structured faults: %+v", gotFaults)
			}
			if !env.CPU.Running() {
				t.Fatalf("AROS CPU halted during mouse input soak at pc=%08X sr=%04X", env.CPU.PC, env.CPU.SR)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	soakDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(soakDeadline) {
		mu.Lock()
		gotFaults := append([]M68KFaultRecord(nil), faults...)
		mu.Unlock()
		if len(gotFaults) != 0 {
			t.Fatalf("AROS mouse input post-soak hit structured faults: %+v", gotFaults)
		}
		if !env.CPU.Running() {
			t.Fatalf("AROS CPU halted after mouse input soak at pc=%08X sr=%04X", env.CPU.PC, env.CPU.SR)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestAROSInterpreterPostReadyKeyboardInputSoakAcceptance(t *testing.T) {
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
		t.Skip("AROS drive tree not available; skipping keyboard input soak acceptance")
	}

	env, err := NewAROSInterpreterBootEnvironment(rom, driveRoot)
	if err != nil {
		t.Fatalf("NewAROSInterpreterBootEnvironment() failed: %v", err)
	}
	defer env.Close()

	if env.Terminal == nil {
		t.Fatal("AROS interpreter boot environment did not expose terminal MMIO")
	}

	var (
		mu     sync.Mutex
		faults []M68KFaultRecord
	)
	prevHook := env.CPU.FaultHook
	env.CPU.FaultHook = func(record M68KFaultRecord) {
		record = NormalizeM68KFaultRecord(env.CPU, record)
		mu.Lock()
		faults = append(faults, record)
		mu.Unlock()
		if prevHook != nil {
			prevHook(record)
		}
	}
	defer func() { env.CPU.FaultHook = prevHook }()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
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

	checkHealthy := func(context string) {
		mu.Lock()
		gotFaults := append([]M68KFaultRecord(nil), faults...)
		mu.Unlock()
		if len(gotFaults) != 0 {
			t.Fatalf("AROS keyboard input soak hit structured faults during %s: %+v", context, gotFaults)
		}
		if !env.CPU.Running() {
			t.Fatalf("AROS CPU halted during %s at pc=%08X sr=%04X", context, env.CPU.PC, env.CPU.SR)
		}
	}

	pressRawKey := func(makeCode uint8) {
		env.Terminal.EnqueueScancode(makeCode)
		time.Sleep(60 * time.Millisecond)
		checkHealthy("rawkey make")
		env.Terminal.EnqueueScancode(makeCode | 0x80)
		time.Sleep(90 * time.Millisecond)
		checkHealthy("rawkey break")
	}

	// Amiga rawkeys: arrows, space, enter, escape, and a plain letter.
	keys := []uint8{
		0x4C, // Up
		0x4D, // Down
		0x4E, // Right
		0x4F, // Left
		0x40, // Space
		0x44, // Enter
		0x45, // Escape
		0x20, // A
	}
	for _, key := range keys {
		pressRawKey(key)
	}

	soakDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(soakDeadline) {
		checkHealthy("keyboard input post-soak")
		time.Sleep(25 * time.Millisecond)
	}
}
