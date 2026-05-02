package main

import "testing"

func TestZ80InterruptSinkCreatesFreshEdgeEachPulse(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x2000, []byte{0x00})
	rig.cpu.SP = 0xFF00
	sink := NewZ80InterruptSink(rig.cpu)

	for i := 0; i < 3; i++ {
		rig.cpu.PC = 0x2000
		rig.cpu.SP = 0xFF00
		rig.cpu.IFF1 = true
		rig.cpu.IFF2 = true

		sink.Pulse(IntMaskVBI)
		rig.cpu.Step()
		if rig.cpu.PC != 0x0066 {
			t.Fatalf("pulse %d PC = 0x%04X, want NMI vector 0x0066", i, rig.cpu.PC)
		}
		if rig.cpu.nmiLine.Load() {
			t.Fatalf("pulse %d left NMI line asserted", i)
		}
	}
}

func TestZ80InterruptSinkRapidDoublePulseCoalescesBeforeInstructionBoundary(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x2000, []byte{0x00})
	rig.bus.mem[0x0066] = 0xED
	rig.bus.mem[0x0067] = 0x45 // RETN
	rig.cpu.PC = 0x2000
	rig.cpu.SP = 0xFF00
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	sink := NewZ80InterruptSink(rig.cpu)

	sink.Pulse(IntMaskVBI)
	sink.Pulse(IntMaskVBI)

	rig.cpu.Step()
	if rig.cpu.PC != 0x0066 {
		t.Fatalf("PC after NMI service = 0x%04X, want 0x0066", rig.cpu.PC)
	}

	rig.cpu.Step()
	if rig.cpu.PC != 0x2000 {
		t.Fatalf("second step did not return from single NMI; PC = 0x%04X, want 0x2000", rig.cpu.PC)
	}
}
