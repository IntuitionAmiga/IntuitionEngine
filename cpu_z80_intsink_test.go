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

func TestZ80LevelTriggeredAckReedgesHeldNMI(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x2000, []byte{0x00})
	rig.cpu.SP = 0xFF00
	sink := NewZ80InterruptSink(rig.cpu)
	var level LevelTriggeredInterruptSink = sink

	level.Assert(IntMaskVBI)
	if !rig.cpu.nmiPending.Load() {
		t.Fatal("Assert did not create initial NMI edge")
	}

	rig.cpu.serviceNMI()
	if rig.cpu.nmiPending.Load() {
		t.Fatal("serviceNMI did not clear pending NMI")
	}
	if !rig.cpu.nmiLine.Load() {
		t.Fatal("held level source should leave NMI line asserted")
	}

	level.Ack(IntMaskVBI)
	if !rig.cpu.nmiPending.Load() {
		t.Fatal("Ack did not create a fresh NMI edge for held level source")
	}
}

func TestZ80LevelTriggeredOtherSourceReedgesAfterDeassert(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x2000, []byte{0x00})
	rig.cpu.SP = 0xFF00
	sink := NewZ80InterruptSink(rig.cpu)
	var level LevelTriggeredInterruptSink = sink

	level.Assert(IntMaskVBI)
	level.Assert(IntMaskBlitter)
	rig.cpu.serviceNMI()
	level.Deassert(IntMaskBlitter)
	if !rig.cpu.nmiPending.Load() {
		t.Fatal("Deassert of one source did not re-edge remaining held NMI source")
	}
}
