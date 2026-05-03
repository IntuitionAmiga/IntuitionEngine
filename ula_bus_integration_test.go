//go:build headless

package main

import "testing"

func installRealULA(bus *MachineBus) *ULAEngine {
	ula := NewULAEngine(bus)
	bus.MapIO(ULA_BASE, ULA_REG_END, ula.HandleRead, ula.HandleWrite)
	bus.MapIOByteRead(ULA_BASE, ULA_REG_END, ula.HandleRead8)
	bus.MapIOByte(ULA_BASE, ULA_REG_END, ula.HandleWrite8)
	bus.MapIO(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleBusVRAMRead, ula.HandleBusVRAMWrite)
	bus.MapIOByteRead(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleRead8)
	bus.MapIOByte(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleWrite8)
	bus.MapIO64(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleRead64, ula.HandleWrite64)
	bus.MapIOWideWriteFanout(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END)
	return ula
}

func TestULA_ApertureWrite32UpdatesAllBytes(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)

	bus.Write32(ULA_VRAM_AP_BASE+0x20, 0x44332211)

	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if got := ula.HandleVRAMRead(uint16(0x20 + i)); got != want {
			t.Fatalf("vram[%d] = %#02x, want %#02x", 0x20+i, got, want)
		}
	}
}

func TestULA_ApertureWrite64UpdatesAllBytes(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)

	bus.Write64(ULA_VRAM_AP_BASE+0x30, 0x8877665544332211)

	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88} {
		if got := ula.HandleVRAMRead(uint16(0x30 + i)); got != want {
			t.Fatalf("vram[%d] = %#02x, want %#02x", 0x30+i, got, want)
		}
	}
}

func TestULA_ApertureRead64AllBytes(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)

	for i, value := range []uint8{0x10, 0x21, 0x32, 0x43, 0x54, 0x65, 0x76, 0x87} {
		ula.HandleVRAMWrite(uint16(0x40+i), value)
	}

	if got := bus.Read64(ULA_VRAM_AP_BASE + 0x40); got != 0x8776655443322110 {
		t.Fatalf("Read64 aperture = %#016x, want 0x8776655443322110", got)
	}
}

func TestULA_ApertureWrite64Boundary(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)

	bus.Write64(ULA_VRAM_AP_END-7, 0x8877665544332211)
	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88} {
		if got := ula.HandleVRAMRead(uint16(ULA_VRAM_SIZE - 8 + i)); got != want {
			t.Fatalf("last full write vram[%d] = %#02x, want %#02x", ULA_VRAM_SIZE-8+i, got, want)
		}
	}

	for i := ULA_VRAM_SIZE - 5; i < ULA_VRAM_SIZE; i++ {
		ula.HandleVRAMWrite(uint16(i), 0)
		bus.memory[ULA_VRAM_AP_BASE+uint32(i)] = 0
	}
	bus.Write64(ULA_VRAM_AP_END-4, 0x0123456789ABCDEF)
	for i := ULA_VRAM_SIZE - 5; i < ULA_VRAM_SIZE; i++ {
		if got := ula.HandleVRAMRead(uint16(i)); got != 0 {
			t.Fatalf("end-4 spilling Write64 partially committed vram[%d] = %#02x", i, got)
		}
		if got := bus.memory[ULA_VRAM_AP_BASE+uint32(i)]; got != 0 {
			t.Fatalf("end-4 spilling Write64 updated backing memory[%#x] = %#02x", ULA_VRAM_AP_BASE+uint32(i), got)
		}
	}
	if got := bus.Read64(ULA_VRAM_AP_END - 4); got != 0 {
		t.Fatalf("end-4 spilling Read64 = %#016x, want 0", got)
	}

	bus.Write64(ULA_VRAM_AP_END-3, 0xFFEEDDCCBBAA9988)
	for i := ULA_VRAM_SIZE - 4; i < ULA_VRAM_SIZE; i++ {
		if got := ula.HandleVRAMRead(uint16(i)); got != 0 {
			t.Fatalf("spilling Write64 partially committed vram[%d] = %#02x", i, got)
		}
	}
	if got := bus.Read64(ULA_VRAM_AP_END - 3); got != 0 {
		t.Fatalf("spilling Read64 = %#016x, want 0", got)
	}
}

func TestULA_DATAWide32WriteOnlyLowByte(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)

	bus.Write8(ULA_ADDR_LO, 0x22)
	bus.Write8(ULA_ADDR_HI, 0x00)
	bus.Write32(ULA_DATA, 0xAABBCCDD)

	if got := ula.HandleVRAMRead(0x22); got != 0xDD {
		t.Fatalf("ULA_DATA wide write byte = %#02x, want 0xDD", got)
	}
	for _, off := range []uint16{0x23, 0x24, 0x25} {
		if got := ula.HandleVRAMRead(off); got != 0 {
			t.Fatalf("ULA_DATA wide write touched vram[%#x] = %#02x", off, got)
		}
	}
}

func TestULA_NoLegacy4000Alias(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)

	bus.Write8(0x4000, 0xA5)

	if got := bus.Read8(0x4000); got != 0xA5 {
		t.Fatalf("RAM $4000 = %#02x, want 0xA5", got)
	}
	if got := ula.HandleVRAMRead(0); got != 0 {
		t.Fatalf("ULA VRAM[0] changed through legacy $4000 alias: %#02x", got)
	}
}

func TestULA_Z80PagedVRAMRoundTripAndControl(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)
	z80bus := NewZ80BusAdapter(bus)

	z80bus.Out(Z80_ULA_PORT_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_AUTO_INC)
	if !ula.IsEnabled() {
		t.Fatal("Z80 CTRL port did not enable ULA")
	}

	z80bus.Out(Z80_ULA_PORT_ADDR_LO, 0x34)
	z80bus.Out(Z80_ULA_PORT_ADDR_HI, 0x12)
	z80bus.Out(Z80_ULA_PORT_DATA, 0xAA)
	z80bus.Out(Z80_ULA_PORT_DATA, 0xBB)

	if got := ula.HandleVRAMRead(0x1234); got != 0xAA {
		t.Fatalf("VRAM[0x1234] = %#02x, want 0xAA", got)
	}
	if got := ula.HandleVRAMRead(0x1235); got != 0xBB {
		t.Fatalf("VRAM[0x1235] = %#02x, want 0xBB", got)
	}

	z80bus.Out(Z80_ULA_PORT_ADDR_LO, 0x34)
	z80bus.Out(Z80_ULA_PORT_ADDR_HI, 0x12)
	if got := z80bus.In(Z80_ULA_PORT_DATA); got != 0xAA {
		t.Fatalf("Z80 DATA read = %#02x, want 0xAA", got)
	}
}

func TestULA_6502PagedVRAMRoundTrip(t *testing.T) {
	bus := NewMachineBus()
	ula := installRealULA(bus)
	c65bus := NewBus6502Adapter(bus)

	c65bus.Write(C6502_ULA_CTRL, ULA_CTRL_AUTO_INC)
	c65bus.Write(C6502_ULA_ADDR_LO, 0x00)
	c65bus.Write(C6502_ULA_ADDR_HI, 0x18)
	c65bus.Write(C6502_ULA_DATA, 0x47)
	c65bus.Write(C6502_ULA_DATA, 0x3A)

	if got := ula.HandleVRAMRead(0x1800); got != 0x47 {
		t.Fatalf("VRAM[0x1800] = %#02x, want 0x47", got)
	}
	if got := ula.HandleVRAMRead(0x1801); got != 0x3A {
		t.Fatalf("VRAM[0x1801] = %#02x, want 0x3A", got)
	}

	c65bus.Write(C6502_ULA_ADDR_LO, 0x00)
	c65bus.Write(C6502_ULA_ADDR_HI, 0x18)
	if got := c65bus.Read(C6502_ULA_DATA); got != 0x47 {
		t.Fatalf("6502 DATA read = %#02x, want 0x47", got)
	}
}

func TestULA_TickFrameAdvancesWhenDisabled(t *testing.T) {
	ula := NewULAEngine(nil)

	ula.TickFrame()

	if got := ula.HandleRead(ULA_STATUS); got != ULA_STATUS_VBLANK {
		t.Fatalf("status after disabled tick = %#x, want VBlank", got)
	}
}

type fakeULAIRQSink struct {
	asserts   int
	deasserts int
}

func (s *fakeULAIRQSink) AssertVBlankIRQ()   { s.asserts++ }
func (s *fakeULAIRQSink) DeassertVBlankIRQ() { s.deasserts++ }

func TestULA_VBlankIRQAssertAndStatusAck(t *testing.T) {
	ula := NewULAEngine(nil)
	sink := &fakeULAIRQSink{}
	ula.SetIRQSink(sink)
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN)

	ula.TickFrame()
	if sink.asserts != 1 {
		t.Fatalf("asserts = %d, want 1", sink.asserts)
	}
	if got := ula.HandleRead(ULA_STATUS); got != ULA_STATUS_VBLANK {
		t.Fatalf("status = %#x, want VBlank", got)
	}
	if sink.deasserts != 1 {
		t.Fatalf("deasserts = %d, want 1", sink.deasserts)
	}
}

func TestULA_VBlankIRQDeassertsWhenControlMasked(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctrl uint32
	}{
		{name: "irq masked", ctrl: ULA_CTRL_ENABLE},
		{name: "chip disabled", ctrl: ULA_CTRL_VBLANK_IRQ_EN},
		{name: "both disabled", ctrl: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ula := NewULAEngine(nil)
			sink := &fakeULAIRQSink{}
			ula.SetIRQSink(sink)
			ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN)
			ula.TickFrame()
			if sink.asserts != 1 {
				t.Fatalf("asserts before mask = %d, want 1", sink.asserts)
			}

			ula.HandleWrite(ULA_CTRL, tc.ctrl)
			if sink.deasserts != 1 {
				t.Fatalf("deasserts after mask = %d, want 1", sink.deasserts)
			}
		})
	}
}

func TestULA_ResetClearsNewStateAndDeassertsIRQ(t *testing.T) {
	ula := NewULAEngine(nil)
	sink := &fakeULAIRQSink{}
	ula.SetIRQSink(sink)
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_AUTO_INC|ULA_CTRL_VBLANK_IRQ_EN)
	ula.HandleWrite(ULA_ADDR_LO, 0x34)
	ula.HandleWrite(ULA_ADDR_HI, 0x12)
	ula.TickFrame()

	ula.Reset()

	if ula.IsEnabled() {
		t.Fatal("reset left ULA enabled")
	}
	if got := ula.HandleRead(ULA_CTRL); got != 0 {
		t.Fatalf("control after reset = %#x, want 0", got)
	}
	if got := ula.HandleRead(ULA_ADDR_LO); got != 0 {
		t.Fatalf("addr lo after reset = %#x, want 0", got)
	}
	if got := ula.HandleRead(ULA_ADDR_HI); got != 0 {
		t.Fatalf("addr hi after reset = %#x, want 0", got)
	}
	if got := ula.HandleRead(ULA_STATUS); got != 0 {
		t.Fatalf("status after reset = %#x, want 0", got)
	}
	if sink.deasserts == 0 {
		t.Fatal("reset did not deassert IRQ")
	}
}

func TestX86IRQAssertThenClearBeforeService(t *testing.T) {
	cpu := NewCPU_X86(&X86BusAdapter{bus: NewMachineBus()})

	cpu.SetIRQ(true, IRQ_VECTOR_VBLANK)
	cpu.ClearIRQ(IRQ_VECTOR_VBLANK)

	if cpu.irqPending.Load() {
		t.Fatal("ClearIRQ left x86 IRQ pending")
	}
}

func Test6502SetIRQLine(t *testing.T) {
	cpu := NewCPU_6502(NewMachineBus())

	cpu.SetIRQLine(true)
	if !cpu.irqPending.Load() {
		t.Fatal("SetIRQLine(true) did not assert 6502 IRQ")
	}
	cpu.SetIRQLine(false)
	if cpu.irqPending.Load() {
		t.Fatal("SetIRQLine(false) did not clear 6502 IRQ")
	}
}

func TestULA_VBlankIRQ_Z80Adapter(t *testing.T) {
	cpu := NewCPU_Z80(NewZ80BusAdapter(NewMachineBus()))
	ula := NewULAEngine(nil)
	ula.SetIRQSink(newZ80ULAIRQAdapter(cpu))
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN)

	ula.TickFrame()
	if !cpu.irqLine.Load() {
		t.Fatal("ULA TickFrame did not assert Z80 IRQ line")
	}
	ula.HandleRead(ULA_STATUS)
	if cpu.irqLine.Load() {
		t.Fatal("ULA status read did not deassert Z80 IRQ line")
	}
}

func TestULA_VBlankIRQ_6502HeldUntilStatusAck(t *testing.T) {
	cpu := NewCPU_6502(NewMachineBus())
	ula := NewULAEngine(nil)
	ula.SetIRQSink(new6502ULAIRQAdapter(cpu))
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN)

	ula.TickFrame()
	if !cpu.irqPending.Load() {
		t.Fatal("ULA TickFrame did not assert 6502 IRQ")
	}
	cpu.irqPending.Store(false) // CPU serviced the edge-like pending bit.
	ula.TickFrame()
	if !cpu.irqPending.Load() {
		t.Fatal("ULA did not reassert held 6502 IRQ before status ack")
	}
	ula.HandleRead(ULA_STATUS)
	if cpu.irqPending.Load() {
		t.Fatal("ULA status read did not deassert 6502 IRQ")
	}
}

func TestULA_VBlankIRQ_X86Adapter(t *testing.T) {
	cpu := NewCPU_X86(NewX86BusAdapter(NewMachineBus()))
	ula := NewULAEngine(nil)
	ula.SetIRQSink(newX86ULAIRQAdapter(cpu))
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN)

	ula.TickFrame()
	if !cpu.irqPending.Load() {
		t.Fatal("ULA TickFrame did not assert x86 IRQ")
	}
	if got := byte(cpu.irqVector.Load()); got != IRQ_VECTOR_VBLANK {
		t.Fatalf("x86 IRQ vector = %#02x, want %#02x", got, IRQ_VECTOR_VBLANK)
	}
	ula.HandleRead(ULA_STATUS)
	if cpu.irqPending.Load() {
		t.Fatal("ULA status read did not clear x86 IRQ")
	}
}

func TestULA_VBlankPollingOnlyNoopSink(t *testing.T) {
	ula := NewULAEngine(nil)
	ula.SetIRQSink(noopULAIRQAdapter{})
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN)

	ula.TickFrame()
	if got := ula.HandleRead(ULA_STATUS); got != ULA_STATUS_VBLANK {
		t.Fatalf("polling status = %#x, want VBlank", got)
	}
}
