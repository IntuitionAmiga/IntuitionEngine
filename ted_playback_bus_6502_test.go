// ted_6502_bus_test.go - Tests for Plus/4 6502 memory bus emulation

package main

import (
	"testing"
)

func TestTEDPlaybackBus6502Creation(t *testing.T) {
	bus := newTEDPlaybackBus6502(false) // PAL
	if bus == nil {
		t.Fatal("newTEDPlaybackBus6502 returned nil")
	}
	if bus.ntsc {
		t.Error("should be PAL (ntsc=false)")
	}

	bus = newTEDPlaybackBus6502(true) // NTSC
	if !bus.ntsc {
		t.Error("should be NTSC (ntsc=true)")
	}
}

func TestTEDPlaybackBus6502RAMReadWrite(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Write to RAM
	bus.Write(0x1000, 0xAB)
	val := bus.Read(0x1000)
	if val != 0xAB {
		t.Errorf("RAM read = 0x%02X, want 0xAB", val)
	}

	// Write to zero page
	bus.Write(0x00FF, 0xCD)
	val = bus.Read(0x00FF)
	if val != 0xCD {
		t.Errorf("zero page read = 0x%02X, want 0xCD", val)
	}
}

func TestTEDPlaybackBus6502TEDRegisterCapture(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Write to TED sound registers (Plus/4 addresses $FF0E-$FF12)
	bus.Write(PLUS4_TED_SND_CTRL, 0x18) // Voice 1 on, volume 8

	// Check register was captured
	if bus.tedRegs[TED_REG_SND_CTRL] != 0x18 {
		t.Errorf("TED SND_CTRL = 0x%02X, want 0x18", bus.tedRegs[TED_REG_SND_CTRL])
	}

	// Check event was generated
	if len(bus.events) != 1 {
		t.Errorf("events count = %d, want 1", len(bus.events))
	}

	if bus.events[0].Reg != TED_REG_SND_CTRL {
		t.Errorf("event reg = %d, want %d", bus.events[0].Reg, TED_REG_SND_CTRL)
	}
	if bus.events[0].Value != 0x18 {
		t.Errorf("event value = 0x%02X, want 0x18", bus.events[0].Value)
	}
}

func TestTEDPlaybackBus6502TEDFrequencyRegisters(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Write voice 1 frequency
	bus.Write(PLUS4_TED_FREQ1_LO, 0x55)
	bus.Write(PLUS4_TED_FREQ1_HI, 0x02)

	if bus.tedRegs[TED_REG_FREQ1_LO] != 0x55 {
		t.Errorf("FREQ1_LO = 0x%02X, want 0x55", bus.tedRegs[TED_REG_FREQ1_LO])
	}
	if bus.tedRegs[TED_REG_FREQ1_HI] != 0x02 {
		t.Errorf("FREQ1_HI = 0x%02X, want 0x02", bus.tedRegs[TED_REG_FREQ1_HI])
	}
}

func TestTEDPlaybackBus6502TEDRead(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Write a value
	bus.Write(PLUS4_TED_FREQ1_LO, 0x77)

	// Read it back
	val := bus.Read(PLUS4_TED_FREQ1_LO)
	if val != 0x77 {
		t.Errorf("TED read = 0x%02X, want 0x77", val)
	}
}

func TestTEDPlaybackBus6502AddCycles(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	if bus.cycles != 0 {
		t.Errorf("initial cycles = %d, want 0", bus.cycles)
	}

	bus.AddCycles(100)
	if bus.cycles != 100 {
		t.Errorf("cycles = %d, want 100", bus.cycles)
	}

	bus.AddCycles(50)
	if bus.cycles != 150 {
		t.Errorf("cycles = %d, want 150", bus.cycles)
	}
}

func TestTEDPlaybackBus6502StartFrame(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Add some cycles and events
	bus.AddCycles(1000)
	bus.Write(PLUS4_TED_SND_CTRL, 0x18)

	if len(bus.events) != 1 {
		t.Fatalf("events = %d, want 1", len(bus.events))
	}

	// Start new frame
	bus.StartFrame()

	if len(bus.events) != 0 {
		t.Errorf("events after StartFrame = %d, want 0", len(bus.events))
	}
}

func TestTEDPlaybackBus6502CollectEvents(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Generate some events
	bus.Write(PLUS4_TED_FREQ1_LO, 0x55)
	bus.AddCycles(100)
	bus.Write(PLUS4_TED_FREQ1_HI, 0x01)

	events := bus.CollectEvents()
	if len(events) != 2 {
		t.Errorf("collected events = %d, want 2", len(events))
	}

	// Events should be cleared
	if len(bus.events) != 0 {
		t.Errorf("bus.events after collect = %d, want 0", len(bus.events))
	}

	// Second event should have cycle offset
	if events[1].Cycle <= events[0].Cycle {
		t.Error("second event should have higher cycle count")
	}
}

func TestTEDPlaybackBus6502LoadBinary(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	data := []byte{0xA9, 0x00, 0x8D, 0x11, 0xFF, 0x60} // LDA #$00, STA $FF11, RTS
	bus.LoadBinary(0x1000, data)

	for i, v := range data {
		if bus.ram[0x1000+i] != v {
			t.Errorf("RAM[0x%04X] = 0x%02X, want 0x%02X", 0x1000+i, bus.ram[0x1000+i], v)
		}
	}
}

func TestTEDPlaybackBus6502Reset(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Add state
	bus.Write(0x1000, 0xFF)
	bus.Write(PLUS4_TED_SND_CTRL, 0x18)
	bus.AddCycles(1000)

	// Reset
	bus.Reset()

	if bus.cycles != 0 {
		t.Errorf("cycles after reset = %d, want 0", bus.cycles)
	}
	if len(bus.events) != 0 {
		t.Errorf("events after reset = %d, want 0", len(bus.events))
	}
	if bus.tedRegs[TED_REG_SND_CTRL] != 0 {
		t.Errorf("TED regs not cleared after reset")
	}
}

func TestTEDPlaybackBus6502GetCycles(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.AddCycles(500)

	if bus.GetCycles() != 500 {
		t.Errorf("GetCycles = %d, want 500", bus.GetCycles())
	}
}

func TestTEDPlaybackBus6502GetFrameCycles(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.AddCycles(1000)
	bus.StartFrame()
	bus.AddCycles(250)

	frameCycles := bus.GetFrameCycles()
	if frameCycles != 250 {
		t.Errorf("GetFrameCycles = %d, want 250", frameCycles)
	}
}

func TestTEDPlaybackBus6502VectorSetup(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	// Check IRQ vector is set up
	irqLo := bus.Read(0xFFFE)
	irqHi := bus.Read(0xFFFF)
	irqAddr := uint16(irqLo) | (uint16(irqHi) << 8)

	if irqAddr == 0 {
		t.Error("IRQ vector should be non-zero")
	}
}
