//go:build headless

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAROSPostReadySingleStepDiagnostic(t *testing.T) {
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
	done := make(chan struct{}, 1)
	cpu.InstructionHook = func(cpu *M68KCPU) {
		if cpu.lastExecPC != 0x009E31EC {
			return
		}
		cpu.SetRunning(false)
		select {
		case done <- struct{}{}:
		default:
		}
	}
	defer func() { cpu.InstructionHook = nil }()
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		if addr >= 0x009E2FB6 && addr < 0x009E3300 {
			t.Logf("write pc=%08X addr=%08X value=%08X size=%d", pc, addr, value, size)
		}
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for live runner to hit 0x009E31EC")
	}

	t.Logf("stopped live pc=%08X sr=%04X sp=%08X a0=%08X a1=%08X a2=%08X a3=%08X d0=%08X d1=%08X d2=%08X d3=%08X",
		cpu.lastExecPC, cpu.SR, cpu.AddrRegs[7], cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
		cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3])

	for i := 0; i < 80; i++ {
		t.Logf("step=%d pc=%08X op=%04X sr=%04X sp=%08X a0=%08X a1=%08X a2=%08X a3=%08X d0=%08X d1=%08X d2=%08X d3=%08X",
			i, cpu.PC, cpu.Read16(cpu.PC), cpu.SR, cpu.AddrRegs[7], cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3],
			cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3])
		if cpu.StepOne() == 0 {
			t.Fatalf("step halted at pc=%08X", cpu.PC)
		}
	}

	t.Fatal("diagnostic complete")
}
