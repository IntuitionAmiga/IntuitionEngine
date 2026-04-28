// cpu_ie32_reset_scope_test.go - PLAN_MAX_RAM slice 10 reviewer P2 fix.
//
// Pins that IE32 reset/load paths never iterate over guest RAM beyond
// the fixed program staging window. With mmap-backed bus.memory at
// multi-GiB sizes, an unbounded loop on F10/reload would touch every
// page and reproduce the IntuitionOS RSS-spike regression.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCPUIE32Reset_DoesNotTouchMemoryOutsideProgramWindow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	const sentinel byte = 0xA5
	probes := []uint32{
		PROG_START + 0x100,
		STACK_START + 0x10,
		uint32(len(cpu.memory)) - 0x100,
	}
	for _, addr := range probes {
		cpu.memory[addr] = sentinel
	}

	cpu.Reset()

	for _, addr := range probes {
		if got := cpu.memory[addr]; got != sentinel {
			t.Fatalf("CPU.Reset clobbered cpu.memory[%#x] = %#x, want %#x", addr, got, sentinel)
		}
	}
}

func TestCPUIE32LoadProgram_OnlyTouchesProgramWindow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	const sentinel byte = 0x5A
	probesAbove := []uint32{
		STACK_START,
		STACK_START + 0x1000,
		uint32(len(cpu.memory)) - 0x100,
	}
	for _, addr := range probesAbove {
		cpu.memory[addr] = sentinel
	}
	const lowAddr uint32 = 0x800
	cpu.memory[lowAddr] = sentinel

	tmp := filepath.Join(t.TempDir(), "p.iex")
	if err := os.WriteFile(tmp, []byte{0xDE, 0xAD, 0xBE, 0xEF}, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := cpu.LoadProgram(tmp); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	for _, addr := range probesAbove {
		if got := cpu.memory[addr]; got != sentinel {
			t.Fatalf("LoadProgram clobbered cpu.memory[%#x] = %#x, want %#x", addr, got, sentinel)
		}
	}
	if got := cpu.memory[lowAddr]; got != sentinel {
		t.Fatalf("LoadProgram clobbered low sentinel cpu.memory[%#x] = %#x, want %#x", lowAddr, got, sentinel)
	}
}

// TestCPUIE32LoadProgramBytes_OversizeBoundedAtStackStart pins reviewer
// P3: the F10/reload closure in runtime_helpers.go now routes through
// CPU.LoadProgramBytes instead of an open-ended copy(cpu.memory[PROG_
// START:], bytes). An oversize cached program must NOT spill past
// STACK_START on reload.
func TestCPUIE32LoadProgramBytes_OversizeBoundedAtStackStart(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	const sentinel byte = 0x33
	cpu.memory[STACK_START] = sentinel
	cpu.memory[STACK_START+0x100] = sentinel

	program := make([]byte, int(STACK_START-PROG_START)+4096)
	for i := range program {
		program[i] = 0xCC
	}
	cpu.LoadProgramBytes(program)

	if got := cpu.memory[STACK_START]; got != sentinel {
		t.Fatalf("LoadProgramBytes spilled to STACK_START: got %#x, want sentinel %#x", got, sentinel)
	}
	if got := cpu.memory[STACK_START+0x100]; got != sentinel {
		t.Fatalf("LoadProgramBytes spilled past STACK_START: got %#x, want sentinel %#x", got, sentinel)
	}
}

func TestCPUIE32LoadProgram_OversizeProgramTruncatesAtStackStart(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	const sentinel byte = 0x77
	cpu.memory[STACK_START] = sentinel
	cpu.memory[STACK_START+1] = sentinel

	program := make([]byte, int(STACK_START-PROG_START)+1024)
	for i := range program {
		program[i] = 0xCC
	}
	tmp := filepath.Join(t.TempDir(), "big.iex")
	if err := os.WriteFile(tmp, program, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := cpu.LoadProgram(tmp); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	if got := cpu.memory[STACK_START]; got != sentinel {
		t.Fatalf("oversize program spilled to STACK_START: got %#x, want %#x", got, sentinel)
	}
	if got := cpu.memory[STACK_START+1]; got != sentinel {
		t.Fatalf("oversize program spilled past STACK_START: got %#x, want %#x", got, sentinel)
	}
}
