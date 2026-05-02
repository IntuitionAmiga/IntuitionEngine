package main

import "testing"

func TestM68KInterruptSinkAssertsLevel7(t *testing.T) {
	cpu := NewM68KCPU(NewMachineBus())
	cpu.pendingInterrupt.Store(0)

	sink := NewM68KInterruptSink(cpu)
	sink.Pulse(IntMaskVBI)

	if got := cpu.pendingInterrupt.Load(); got&(1<<7) == 0 {
		t.Fatalf("pendingInterrupt = 0x%X, want level 7 bit set", got)
	}
}

func TestM68KInterruptSinkNilSafe(t *testing.T) {
	var sink *M68KInterruptSink
	sink.Pulse(IntMaskDLI)
	NewM68KInterruptSink(nil).Pulse(IntMaskVBI)
}
