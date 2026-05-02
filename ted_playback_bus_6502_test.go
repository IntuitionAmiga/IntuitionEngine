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

func advanceTEDBusToLine(bus *TEDPlaybackBus6502, line uint16) {
	bus.AddCycles(int(line)*TED_CYCLES_PER_LINE + 1)
}

func TestTEDBus_RasterCompareSetsIRQFlag_Low8(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0xCD)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)

	advanceTEDBusToLine(bus, 0xCD)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("raster flag not set")
	}
	if !bus.CheckIRQ() {
		t.Fatalf("raster compare should set pending IRQ when enabled")
	}
}

func TestTEDBus_RasterCompareSetsIRQFlag_Bit8(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x2C)
	bus.Write(PLUS4_TED_IRQ_MASK, 0x01|TED_IRQ_RASTER)

	advanceTEDBusToLine(bus, 0x12C)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("9-bit raster compare did not match line 300")
	}
}

func TestTEDBus_RasterCompareGatedByMask(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x40)
	bus.Write(PLUS4_TED_IRQ_MASK, 0x00)

	advanceTEDBusToLine(bus, 0x40)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("raster flag should latch even when masked")
	}
	if bus.CheckIRQ() {
		t.Fatalf("masked raster compare should not set pending IRQ")
	}
}

func TestTEDBus_RasterCompareAckClearsFlag(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x20)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)
	advanceTEDBusToLine(bus, 0x20)

	bus.Write(PLUS4_TED_IRQ_FLAGS, TED_IRQ_RASTER)

	if bus.irqFlags&TED_IRQ_RASTER != 0 {
		t.Fatalf("raster flag not cleared by write-1 ack")
	}
}

func TestTEDBus_FF0A_PreservesT1Enable(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)

	bus.Write(PLUS4_TED_IRQ_MASK, 0x0A)
	if got := bus.Read(PLUS4_TED_IRQ_MASK); got != 0x0A {
		t.Fatalf("$FF0A read = %#02x, want 0x0A", got)
	}
	if bus.irqMask&TED_IRQ_TIMER1 == 0 {
		t.Fatalf("Timer 1 mask bit not preserved")
	}

	bus.Write(PLUS4_TED_IRQ_MASK, 0x03)
	if got := bus.Read(PLUS4_TED_IRQ_MASK); got != 0x03 {
		t.Fatalf("$FF0A read = %#02x, want 0x03", got)
	}
	if bus.irqMask&TED_IRQ_TIMER1 != 0 {
		t.Fatalf("Timer 1 mask bit should be cleared by full-byte write")
	}
}

func TestTEDBus_FF0A_CmpHi_DoesNotClobberMask(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.irqMask = 0x0A

	bus.Write(PLUS4_TED_IRQ_MASK, 0x01)
	if bus.irqMask != 0x01 {
		t.Fatalf("irqMask = %#02x, want 0x01", bus.irqMask)
	}
	if bus.rasterCmp&0x100 == 0 {
		t.Fatalf("raster compare high bit not set")
	}

	bus.Write(PLUS4_TED_IRQ_MASK, 0x09)
	if bus.irqMask != 0x09 {
		t.Fatalf("irqMask = %#02x, want 0x09", bus.irqMask)
	}
	if bus.irqMask&TED_IRQ_TIMER1 == 0 {
		t.Fatalf("Timer 1 enable should be preserved")
	}
}

func TestTEDBus_FF09_SummaryBit_Raster(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x10)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)
	advanceTEDBusToLine(bus, 0x10)

	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 == 0 {
		t.Fatalf("$FF09 summary bit not set for enabled raster flag: %#02x", got)
	}
}

func TestTEDBus_FF09_SummaryBit_Timer1(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.timer1Latch = 1
	bus.timer1Counter = 0
	bus.timer1Running = true
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_TIMER1)
	bus.AddCycles(1)

	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 == 0 {
		t.Fatalf("$FF09 summary bit not set for enabled Timer 1 flag: %#02x", got)
	}
}

func TestTEDBus_FF09_SummaryBit_GatedByEnable_Raster(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x10)
	bus.Write(PLUS4_TED_IRQ_MASK, 0x00)
	advanceTEDBusToLine(bus, 0x10)

	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 != 0 {
		t.Fatalf("$FF09 summary should be clear for masked raster flag: %#02x", got)
	}
}

func TestTEDBus_FF09_SummaryBit_GatedByEnable_T1(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.irqFlags = TED_IRQ_TIMER1
	bus.Write(PLUS4_TED_IRQ_MASK, 0x00)

	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 != 0 {
		t.Fatalf("$FF09 summary should be clear for masked Timer 1 flag: %#02x", got)
	}
}

func TestTEDBus_FF09_SummaryBit_DerivedNotStored(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.irqFlags = TED_IRQ_TIMER1
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_TIMER1)
	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 == 0 {
		t.Fatalf("precondition failed: summary not set")
	}

	bus.Write(PLUS4_TED_IRQ_FLAGS, TED_IRQ_TIMER1)

	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 != 0 {
		t.Fatalf("summary should clear after source ack: %#02x", got)
	}
}

func TestTEDBus_FF09_SummaryBit_NotDirectlyClearable(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.irqFlags = TED_IRQ_TIMER1
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_TIMER1)

	bus.Write(PLUS4_TED_IRQ_FLAGS, 0x80)

	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 == 0 {
		t.Fatalf("summary bit should ignore direct clear while source is active: %#02x", got)
	}
}

func TestTEDBus_RasterCompare_LatchesOnAddCycles(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x30)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)

	bus.AddCycles(0x31 * TED_CYCLES_PER_LINE)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("raster compare did not latch during AddCycles")
	}
}

func TestTEDBus_RasterCompare_LatchesAcrossSkippedLines(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x40)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)

	bus.AddCycles(0x45 * TED_CYCLES_PER_LINE)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("raster compare did not latch when AddCycles skipped over the line")
	}
}

func TestTEDBus_RasterCompare_OnePerFrame(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x20)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)
	advanceTEDBusToLine(bus, 0x20)
	bus.Write(PLUS4_TED_IRQ_FLAGS, TED_IRQ_RASTER)

	bus.AddCycles(10 * TED_CYCLES_PER_LINE)
	if bus.irqFlags&TED_IRQ_RASTER != 0 {
		t.Fatalf("raster compare re-latched within same frame")
	}

	bus.AddCycles((TED_PAL_LINES - 0x2A + 0x21) * TED_CYCLES_PER_LINE)
	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("raster compare did not re-latch after frame wrap")
	}
}

func TestTEDBus_RasterCompare_WrapUsesFrameRelativeCycles(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x06)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)

	bus.AddCycles(int((uint64(TED_CLOCK_PAL) / 50) - 1))
	bus.Write(PLUS4_TED_IRQ_FLAGS, TED_IRQ_RASTER)
	_ = bus.CheckIRQ()
	bus.StartFrame()
	bus.AddCycles(TED_CYCLES_PER_LINE + 1)

	if bus.irqFlags&TED_IRQ_RASTER != 0 {
		t.Fatalf("raster compare fired early after StartFrame at line %d", bus.rasterLine)
	}
	if bus.CheckIRQ() {
		t.Fatalf("early raster compare should not assert IRQ")
	}

	bus.AddCycles(5 * TED_CYCLES_PER_LINE)
	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("raster compare did not fire at frame-relative line 6")
	}
}

func TestTEDBus_IRQMaskWriteAssertsPendingForLatchedRaster(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0x10)
	bus.Write(PLUS4_TED_IRQ_MASK, 0x00)
	advanceTEDBusToLine(bus, 0x10)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("precondition failed: raster flag did not latch while masked")
	}
	if bus.CheckIRQ() {
		t.Fatalf("masked raster flag should not assert IRQ before unmask")
	}

	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)
	if !bus.CheckIRQ() {
		t.Fatalf("unmasking latched raster flag should assert pending IRQ")
	}
}

func TestTEDBus_IRQMaskWriteAssertsPendingForLatchedTimer(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.timer1Latch = 1
	bus.timer1Counter = 0
	bus.timer1Running = true
	bus.Write(PLUS4_TED_IRQ_MASK, 0x00)
	bus.AddCycles(1)

	if bus.irqFlags&TED_IRQ_TIMER1 == 0 {
		t.Fatalf("precondition failed: Timer 1 flag did not latch while masked")
	}
	if bus.CheckIRQ() {
		t.Fatalf("masked Timer 1 flag should not assert IRQ before unmask")
	}

	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_TIMER1)
	if !bus.CheckIRQ() {
		t.Fatalf("unmasking latched Timer 1 flag should assert pending IRQ")
	}
}

func TestTEDBus_StartFrame_DoesNotSetRasterFlag(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_IRQ_MASK, 0x00)
	bus.rasterIRQEnabled = true

	bus.StartFrame()

	if bus.irqFlags&TED_IRQ_RASTER != 0 {
		t.Fatalf("StartFrame should not synthesize raster flags")
	}
	if bus.irqPending {
		t.Fatalf("StartFrame should not synthesize pending IRQs")
	}
}

func TestTEDBus_SyntheticFrameIRQ_Retired(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0xFF)
	bus.Write(PLUS4_TED_IRQ_MASK, 0x01)
	bus.rasterIRQEnabled = true

	bus.AddCycles(TED_PAL_LINES * TED_CYCLES_PER_LINE)

	if bus.irqFlags&TED_IRQ_RASTER != 0 {
		t.Fatalf("out-of-range compare should not latch raster flag")
	}
	if bus.irqPending {
		t.Fatalf("masked/out-of-range compare should not set pending IRQ")
	}
}

func TestTEDBus_RasterCompare_OutOfRange_NeverMatches(t *testing.T) {
	for _, cmp := range []uint16{TED_PAL_LINES, 0x1FF} {
		bus := newTEDPlaybackBus6502(false)
		bus.Write(PLUS4_TED_RASTER_CMP_LO, byte(cmp))
		bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER|byte((cmp>>8)&1))

		bus.AddCycles(TED_PAL_LINES * TED_CYCLES_PER_LINE)

		if bus.irqFlags&TED_IRQ_RASTER != 0 {
			t.Fatalf("out-of-range compare %#03x latched raster flag", cmp)
		}
	}
}

func TestTEDBus_RasterCompare_AtLineZero(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_RASTER_CMP_LO, 0)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_RASTER)

	bus.AddCycles(TED_PAL_LINES * TED_CYCLES_PER_LINE)

	if bus.irqFlags&TED_IRQ_RASTER == 0 {
		t.Fatalf("line zero compare should latch once per frame")
	}
}

func TestTEDBus_Timer2UnderflowSetsIRQFlag(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_TIMER2_LO, 0x01)
	bus.Write(PLUS4_TED_TIMER2_HI, 0x00)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_TIMER2)

	bus.AddCycles(2)

	if bus.irqFlags&TED_IRQ_TIMER2 == 0 {
		t.Fatalf("Timer 2 underflow flag not set")
	}
	if !bus.CheckIRQ() {
		t.Fatalf("Timer 2 underflow should set pending IRQ when enabled")
	}
	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 == 0 {
		t.Fatalf("$FF09 summary bit not set for Timer 2: %#02x", got)
	}
}

func TestTEDBus_Timer3UnderflowSetsIRQFlag(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_TIMER3_LO, 0x01)
	bus.Write(PLUS4_TED_TIMER3_HI, 0x00)
	bus.Write(PLUS4_TED_IRQ_MASK, TED_IRQ_TIMER3)

	bus.AddCycles(2)

	if bus.irqFlags&TED_IRQ_TIMER3 == 0 {
		t.Fatalf("Timer 3 underflow flag not set")
	}
	if !bus.CheckIRQ() {
		t.Fatalf("Timer 3 underflow should set pending IRQ when enabled")
	}
	if got := bus.Read(PLUS4_TED_IRQ_FLAGS); got&0x80 == 0 {
		t.Fatalf("$FF09 summary bit not set for Timer 3: %#02x", got)
	}
}

func TestTEDBus_Timer2And3Readback(t *testing.T) {
	bus := newTEDPlaybackBus6502(false)
	bus.Write(PLUS4_TED_TIMER2_LO, 0x34)
	bus.Write(PLUS4_TED_TIMER2_HI, 0x12)
	bus.Write(PLUS4_TED_TIMER3_LO, 0x78)
	bus.Write(PLUS4_TED_TIMER3_HI, 0x56)
	bus.AddCycles(2)

	if got := uint16(bus.Read(PLUS4_TED_TIMER2_LO)) | uint16(bus.Read(PLUS4_TED_TIMER2_HI))<<8; got != 0x1232 {
		t.Fatalf("Timer 2 readback = %#04x, want 0x1232", got)
	}
	if got := uint16(bus.Read(PLUS4_TED_TIMER3_LO)) | uint16(bus.Read(PLUS4_TED_TIMER3_HI))<<8; got != 0x5676 {
		t.Fatalf("Timer 3 readback = %#04x, want 0x5676", got)
	}
}
