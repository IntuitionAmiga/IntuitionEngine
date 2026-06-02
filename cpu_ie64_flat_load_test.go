package main

import (
	"os"
	"testing"
)

// Phase: Loader And Build. LoadFlatProgramBytes must return an error on
// overflow instead of silently clamping, and must not mutate guest memory or
// PC when the image does not fit. White-box tests shrink cpu.memory so an
// oversize image can be constructed without allocating multi-GB slices.

func TestCPU64LoadFlatProgramBytes_OverflowReturnsError(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	cpu.memory = make([]byte, PROG_START+16)
	const sentinel byte = 0x5A
	for i := PROG_START; i < len(cpu.memory); i++ {
		cpu.memory[i] = sentinel
	}
	cpu.PC = 0xDEAD

	program := make([]byte, 17) // one byte past the 16-byte window
	for i := range program {
		program[i] = 0xFF
	}

	if err := cpu.LoadFlatProgramBytes(program); err == nil {
		t.Fatalf("LoadFlatProgramBytes accepted oversize image, want error")
	}
	for i := PROG_START; i < len(cpu.memory); i++ {
		if cpu.memory[i] != sentinel {
			t.Fatalf("overflow mutated guest memory at %#x: got %#x, want sentinel %#x", i, cpu.memory[i], sentinel)
		}
	}
	if cpu.PC != 0xDEAD {
		t.Fatalf("overflow mutated PC: got %#x, want unchanged 0xDEAD", cpu.PC)
	}
}

func TestCPU64LoadFlatProgramBytes_FitReturnsNilAndLoads(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	cpu.memory = make([]byte, PROG_START+32)
	program := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	if err := cpu.LoadFlatProgramBytes(program); err != nil {
		t.Fatalf("LoadFlatProgramBytes rejected fitting image: %v", err)
	}
	for i, b := range program {
		if got := cpu.memory[PROG_START+i]; got != b {
			t.Fatalf("byte %d = %#x, want %#x", i, got, b)
		}
	}
	if cpu.PC != PROG_START {
		t.Fatalf("PC=%#x, want PROG_START %#x", cpu.PC, PROG_START)
	}
}

func TestCPU64FlatProgramFits(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.memory = make([]byte, PROG_START+16)

	if !cpu.FlatProgramFits(16) {
		t.Fatalf("FlatProgramFits(16) = false, want true at exact capacity")
	}
	if cpu.FlatProgramFits(17) {
		t.Fatalf("FlatProgramFits(17) = true, want false past capacity")
	}
}

func TestCPU64LoadFlatProgram_FileOverflowReturnsError(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.memory = make([]byte, PROG_START+8)

	tmp := t.TempDir() + "/big.ie64"
	if err := os.WriteFile(tmp, make([]byte, 9), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := cpu.LoadFlatProgram(tmp); err == nil {
		t.Fatalf("LoadFlatProgram accepted oversize file image, want error")
	}
}

func TestFlatProgramFitsRAM(t *testing.T) {
	memLen := PROG_START + 100
	if !flatProgramFitsRAM(memLen, 100) {
		t.Fatalf("flatProgramFitsRAM exact capacity = false, want true")
	}
	if flatProgramFitsRAM(memLen, 101) {
		t.Fatalf("flatProgramFitsRAM over capacity = true, want false")
	}
	if !flatProgramFitsRAM(memLen, 0) {
		t.Fatalf("flatProgramFitsRAM empty image = false, want true")
	}
}
