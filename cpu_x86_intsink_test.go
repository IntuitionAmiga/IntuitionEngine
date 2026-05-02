package main

import "testing"

func TestX86InterruptSinkVectorsToNMIWithIFClear(t *testing.T) {
	bus := NewMachineBus()
	x86Bus := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(x86Bus)

	bus.Write8(0x0008, 0x00) // IVT vector 2 IP low
	bus.Write8(0x0009, 0x01) // IP high = 0x0100
	bus.Write8(0x000A, 0x00) // CS low
	bus.Write8(0x000B, 0x00) // CS high
	bus.Write8(0x0000, 0x90) // NOP at original PC
	bus.Write8(0x0100, 0xF4) // HLT at NMI handler

	cpu.EIP = 0
	cpu.CS = 0
	cpu.SS = 0
	cpu.ESP = 0x1000
	cpu.Flags = 0 // IF clear: NMI must still be accepted.

	NewX86InterruptSink(cpu).Pulse(IntMaskVBI)
	cpu.Step()

	if cpu.IP() != 0x0101 || cpu.CS != 0 || !cpu.Halted {
		t.Fatalf("NMI handler state CS:IP=%04X:%04X halted=%v, want 0000:0101 halted", cpu.CS, cpu.IP(), cpu.Halted)
	}
	if cpu.nmiPending.Load() {
		t.Fatal("NMI pending not cleared after vectoring")
	}
}

func TestX86InterruptSinkNilSafe(t *testing.T) {
	var sink *X86InterruptSink
	sink.Pulse(IntMaskDLI)
	NewX86InterruptSink(nil).Pulse(IntMaskVBI)
}
