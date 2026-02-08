package main

import "testing"

func TestSIDPlaybackBus6502_RAM(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	bus.Write(0x1000, 0xAA)
	if got := bus.Read(0x1000); got != 0xAA {
		t.Fatalf("expected 0xAA, got 0x%02X", got)
	}
}

func TestSIDPlaybackBus6502_SIDWrites(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	bus.StartFrame()
	bus.Write(0xD400, 0x12)
	events := bus.CollectEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Reg != 0x00 || events[0].Value != 0x12 {
		t.Fatalf("unexpected event %+v", events[0])
	}
}

func TestSIDPlaybackBus6502_SIDReadsStub(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	bus.Write(0xD410, 0x34)
	if got := bus.Read(0xD410); got != 0x34 {
		t.Fatalf("expected 0x34, got 0x%02X", got)
	}
	bus.Write(0xD41B, 0xFF)
	if got := bus.Read(0xD41B); got != 0x00 {
		t.Fatalf("expected 0x00 for OSC3, got 0x%02X", got)
	}
	bus.Write(0xD41C, 0xFF)
	if got := bus.Read(0xD41C); got != 0x00 {
		t.Fatalf("expected 0x00 for ENV3, got 0x%02X", got)
	}
}

func TestSIDPlaybackBus6502_CIATimerIRQ(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	bus.Write(ciaICR, 0x81)
	bus.Write(ciaTimerALo, 0x02)
	bus.Write(ciaTimerAHi, 0x00)
	bus.Write(ciaCRA, 0x11)
	bus.AddCycles(2)
	if !bus.irqPending {
		t.Fatalf("expected IRQ pending")
	}
	icr := bus.Read(ciaICR)
	if (icr & 0x81) == 0 {
		t.Fatalf("expected ICR timer A flag, got 0x%02X", icr)
	}
}

func TestSIDPlaybackBus6502_IRQVectorStub(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	if bus.ram[0xFF00] != 0x6C || bus.ram[0xFF01] != 0x14 || bus.ram[0xFF02] != 0x03 {
		t.Fatalf("IRQ stub not installed")
	}
	if bus.ram[0xFFFE] != 0x00 || bus.ram[0xFFFF] != 0xFF {
		t.Fatalf("IRQ vector not pointing to stub")
	}
}

func TestSIDPlaybackBus6502_VICRaster(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	bus.vicRegs[0x11] = 0x1B
	bus.SetRaster(0x123)
	if got := bus.Read(0xD012); got != 0x23 {
		t.Fatalf("expected raster low 0x23, got 0x%02X", got)
	}
	if got := bus.Read(0xD011); got != 0x9B {
		t.Fatalf("expected raster high bit set in D011, got 0x%02X", got)
	}
}

func TestSIDPlaybackBus6502_LoadBinary(t *testing.T) {
	bus := newSIDPlaybackBus6502(false)
	data := []byte{0xAA, 0xBB, 0xCC}
	bus.LoadBinary(0x2000, data)
	if bus.Read(0x2000) != 0xAA || bus.Read(0x2001) != 0xBB || bus.Read(0x2002) != 0xCC {
		t.Fatalf("binary data not loaded correctly")
	}
}
