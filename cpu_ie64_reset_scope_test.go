// cpu_ie64_reset_scope_test.go - PLAN_MAX_RAM slice 10 reviewer P1 fix.
//
// Pins that CPU64 reset/load paths never iterate over guest RAM beyond
// the fixed program staging window. With mmap-backed bus.memory at
// multi-GiB sizes, an unbounded loop would touch every page and eagerly
// commit RSS for the whole bus window — defeating the lazy-residency
// design that lets the appliance advertise host-scale guest RAM.

package main

import "testing"

// TestCPU64Reset_DoesNotTouchMemoryOutsideProgramWindow asserts CPU64.Reset
// does not zero or otherwise touch cpu.memory. Memory zeroing belongs to
// MachineBus.Reset, which routes through madvise on mmap-allocated slices.
func TestCPU64Reset_DoesNotTouchMemoryOutsideProgramWindow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Sentinels both inside and outside the program window.
	const sentinel byte = 0xA5
	probes := []uint32{
		PROG_START + 0x100,              // inside program window
		STACK_START + 0x10,              // just above stack
		uint32(len(cpu.memory)) - 0x100, // far end of bus.memory
	}
	for _, addr := range probes {
		cpu.memory[addr] = sentinel
	}

	cpu.Reset()

	for _, addr := range probes {
		if got := cpu.memory[addr]; got != sentinel {
			t.Fatalf("CPU64.Reset clobbered cpu.memory[%#x] = %#x, want %#x (Reset must not touch memory)",
				addr, got, sentinel)
		}
	}
}

// TestCPU64LoadProgramBytes_OnlyTouchesProgramWindow asserts the load
// path zeroes only [PROG_START, STACK_START) and copies the program into
// the same bounded window. No byte outside this range may change.
func TestCPU64LoadProgramBytes_OnlyTouchesProgramWindow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	const sentinel byte = 0x5A
	probesAbove := []uint32{
		STACK_START,
		STACK_START + 0x1000,
		uint32(len(cpu.memory)) - 0x100,
	}
	for _, addr := range probesAbove {
		cpu.memory[addr] = sentinel
	}
	// Sentinel BELOW PROG_START must also be untouched.
	const lowAddr uint32 = 0x800
	cpu.memory[lowAddr] = sentinel

	program := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	cpu.LoadProgramBytes(program)

	for _, addr := range probesAbove {
		if got := cpu.memory[addr]; got != sentinel {
			t.Fatalf("LoadProgramBytes clobbered cpu.memory[%#x] = %#x, want %#x (only PROG_START..STACK_START may be touched)",
				addr, got, sentinel)
		}
	}
	if got := cpu.memory[lowAddr]; got != sentinel {
		t.Fatalf("LoadProgramBytes clobbered low sentinel cpu.memory[%#x] = %#x, want %#x",
			lowAddr, got, sentinel)
	}
	// Program bytes must be at PROG_START.
	for i, b := range program {
		if got := cpu.memory[uint32(int(PROG_START)+i)]; got != b {
			t.Fatalf("program byte %d = %#x, want %#x", i, got, b)
		}
	}
}

// TestCPU64LoadProgramBytes_OversizeProgramTruncatesAtStackStart pins that
// a program larger than the program window does not spill past STACK_START.
func TestCPU64LoadProgramBytes_OversizeProgramTruncatesAtStackStart(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	const sentinel byte = 0x77
	cpu.memory[STACK_START] = sentinel
	cpu.memory[STACK_START+1] = sentinel

	// Program large enough to span the full window plus overflow.
	program := make([]byte, int(STACK_START-PROG_START)+1024)
	for i := range program {
		program[i] = 0xCC
	}
	cpu.LoadProgramBytes(program)

	if got := cpu.memory[STACK_START]; got != sentinel {
		t.Fatalf("oversize program spilled to STACK_START: got %#x, want sentinel %#x", got, sentinel)
	}
	if got := cpu.memory[STACK_START+1]; got != sentinel {
		t.Fatalf("oversize program spilled past STACK_START: got %#x, want sentinel %#x", got, sentinel)
	}
}
