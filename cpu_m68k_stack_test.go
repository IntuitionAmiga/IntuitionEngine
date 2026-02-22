package main

import "testing"

func TestM68K_Reset_TunesStackBoundsForLowVectorSP(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.Write32(0, 0x00001000)
	cpu.Write32(M68K_RESET_VECTOR, 0x00002000)
	cpu.Reset()

	if cpu.AddrRegs[7] != 0x00001000 {
		t.Fatalf("A7 got 0x%08X want 0x00001000", cpu.AddrRegs[7])
	}
	if cpu.stackLowerBound != 0 {
		t.Fatalf("stackLowerBound got 0x%08X want 0", cpu.stackLowerBound)
	}
	if cpu.stackUpperBound < 0x00002000 {
		t.Fatalf("stackUpperBound unexpectedly low: 0x%08X", cpu.stackUpperBound)
	}
}

func TestM68K_TuneStackBounds_DefaultsForNormalStack(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.tuneStackBounds(0x00100000)
	if cpu.stackLowerBound != 0x000F0000 {
		t.Fatalf("stackLowerBound got 0x%08X want 0x000F0000", cpu.stackLowerBound)
	}
	if cpu.stackUpperBound != 0x00110000 {
		t.Fatalf("stackUpperBound got 0x%08X want 0x00110000", cpu.stackUpperBound)
	}
}
