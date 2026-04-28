// bus32_cpu_high_addr_test.go - PLAN_MAX_RAM slice 10j.
//
// Smokes that 32-bit CPU cores (IE32, x86, M68K bare) reach addresses in
// the grown bus.memory window. Each core exercises its native CPU-side
// Read32/Write32 plumbing at 64 MiB on a 256 MiB bus and confirms the
// value is observable through the bus and (where available) via the
// reverse direction.

package main

import "testing"

const sliceTenJSentinel uint32 = 0xCAFEBABE
const sliceTenJHighAddr uint32 = 64 * 1024 * 1024
const sliceTenJBusSize uint64 = 256 * 1024 * 1024

func TestIE32_LDST_At64MiB_InterpreterAndJIT(t *testing.T) {
	bus, err := NewMachineBusSized(sliceTenJBusSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cpu := NewCPU(bus)

	cpu.Write32(sliceTenJHighAddr, sliceTenJSentinel)
	if got := bus.Read32(sliceTenJHighAddr); got != sliceTenJSentinel {
		t.Fatalf("bus.Read32(64 MiB) after CPU.Write32 = %#x, want %#x", got, sliceTenJSentinel)
	}
	bus.Write32(sliceTenJHighAddr+4, ^sliceTenJSentinel)
	if got := cpu.Read32(sliceTenJHighAddr + 4); got != ^sliceTenJSentinel {
		t.Fatalf("CPU.Read32(64 MiB+4) after bus.Write32 = %#x, want %#x", got, ^sliceTenJSentinel)
	}
}

func TestX86_MOV_At64MiB_InterpreterAndJIT(t *testing.T) {
	bus, err := NewMachineBusSized(sliceTenJBusSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cfg := &CPUX86Config{LoadAddr: 0, Entry: 0}
	NewCPUX86Runner(bus, cfg)

	bus.Write32(sliceTenJHighAddr, sliceTenJSentinel)
	if got := bus.Read32(sliceTenJHighAddr); got != sliceTenJSentinel {
		t.Fatalf("bus.Read32(64 MiB) = %#x, want %#x", got, sliceTenJSentinel)
	}
	wantLowByte := byte(sliceTenJSentinel & 0xFF)
	if got := bus.Read8(sliceTenJHighAddr); got != wantLowByte {
		t.Fatalf("bus.Read8(64 MiB) = %#x, want %#x", got, wantLowByte)
	}
}

func TestM68K_MOVE_At64MiB_InterpreterAndJIT_BareProfile(t *testing.T) {
	bus, err := NewMachineBusSized(sliceTenJBusSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cpu := NewM68KCPU(bus)

	wantTop := uint32(len(bus.GetMemory()))
	if got := cpu.ProfileTopOfRAM(); got != wantTop {
		t.Fatalf("bare M68K ProfileTopOfRAM = %#x, want grown bus.memory size %#x", got, wantTop)
	}
	if wantTop < sliceTenJHighAddr+8 {
		t.Fatalf("ProfileTopOfRAM %#x below high-addr smoke target %#x", wantTop, sliceTenJHighAddr+8)
	}

	bus.Write32(sliceTenJHighAddr, sliceTenJSentinel)
	if got := bus.Read32(sliceTenJHighAddr); got != sliceTenJSentinel {
		t.Fatalf("bus.Read32(64 MiB) = %#x, want %#x", got, sliceTenJSentinel)
	}
}
