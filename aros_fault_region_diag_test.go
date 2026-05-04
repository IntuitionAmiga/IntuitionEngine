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

func TestAROSFaultRegionWriteDiagnostic(t *testing.T) {
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
	var watchPC uint32
	cpu.DebugWatchFn = func(addr, value, pc uint32, size int) {
		if addr < 0x009E3D80 || addr >= 0x009E3E00 {
			return
		}
		watchPC = pc
		cpu.SetRunning(false)
		select {
		case done <- fmt.Sprintf("write pc=%08X addr=%08X value=%08X size=%d", pc, addr, value, size):
		default:
		}
	}
	defer func() { cpu.DebugWatchFn = nil }()

	select {
	case msg := <-done:
		t.Logf("stopped pc=%08X sr=%04X sp=%08X d0=%08X d1=%08X d2=%08X d3=%08X a0=%08X a1=%08X a2=%08X a3=%08X",
			cpu.PC, cpu.SR, cpu.AddrRegs[7], cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[2], cpu.DataRegs[3],
			cpu.AddrRegs[0], cpu.AddrRegs[1], cpu.AddrRegs[2], cpu.AddrRegs[3])
		readMem := func(addr uint64, size int) []byte {
			out := make([]byte, size)
			for i := 0; i < size; i++ {
				out[i] = cpu.Read8(uint32(addr) + uint32(i))
			}
			return out
		}
		for _, line := range disassembleM68K(readMem, uint64(watchPC-16), 12) {
			t.Logf("disasm %08X: %-18s %s", uint32(line.Address), line.HexBytes, line.Mnemonic)
		}
		t.Fatal(msg)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for writes into 0x009E3D80..0x009E3DFF")
	}
}
