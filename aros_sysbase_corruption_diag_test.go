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

func TestAROSSysBaseCorruptionDiagnostic(t *testing.T) {
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
		if addr > 4 || addr+uint32(size) <= 4 {
			return
		}
		if pc == 0x00605DD4 {
			return
		}
		cpu.SetRunning(false)
		select {
		case done <- fmt.Sprintf("sysbase write pc=%08X addr=%08X value=%08X size=%d", pc, addr, value, size):
		default:
		}
	}
	defer func() { cpu.DebugWatchFn = nil }()

	select {
	case msg := <-done:
		t.Logf("stopped pc=%08X sr=%04X sp=%08X sysbase=%08X d0=%08X d1=%08X d2=%08X a0=%08X a1=%08X a2=%08X a3=%08X",
			cpu.PC, cpu.SR, cpu.AddrRegs[7], cpu.Read32(4), cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2],
			cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3])
		t.Fatal(msg)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for SysBase corruption writes")
	}
}
