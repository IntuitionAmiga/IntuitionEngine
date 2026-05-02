package main

import "testing"

func TestSIDWideWritesFanOutToByteRegisters(t *testing.T) {
	bus := NewMachineBus()
	engine := NewSIDEngine(nil, 44100)
	bus.MapIO(SID_BASE, SID_END, engine.HandleRead, engine.HandleWrite)
	bus.MapIOByte(SID_BASE, SID_END, engine.HandleWrite8)
	bus.MapIOWideWriteFanout(SID_BASE, SID_END)

	bus.Write32(SID_BASE, 0x44332211)
	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if got := engine.regs[i]; got != want {
			t.Fatalf("Write32 SID reg[%d]=0x%02X, want 0x%02X", i, got, want)
		}
	}

	engine.Reset()
	bus.Write16(SID_BASE, 0x2211)
	for i, want := range []uint8{0x11, 0x22} {
		if got := engine.regs[i]; got != want {
			t.Fatalf("Write16 SID reg[%d]=0x%02X, want 0x%02X", i, got, want)
		}
	}
}

func TestM68KSIDWideWritesFanOutInCPUByteOrder(t *testing.T) {
	bus := NewMachineBus()
	engine := NewSIDEngine(nil, 44100)
	bus.MapIO(SID_BASE, SID_END, engine.HandleRead, engine.HandleWrite)
	bus.MapIOByte(SID_BASE, SID_END, engine.HandleWrite8)
	bus.MapIOWideWriteFanout(SID_BASE, SID_END)
	cpu := NewM68KCPU(bus)

	cpu.Write32(SID_BASE, 0x11223344)
	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if got := engine.regs[i]; got != want {
			t.Fatalf("M68K Write32 SID reg[%d]=0x%02X, want 0x%02X", i, got, want)
		}
	}

	engine.Reset()
	cpu.Write16(SID_BASE, 0x1122)
	for i, want := range []uint8{0x11, 0x22} {
		if got := engine.regs[i]; got != want {
			t.Fatalf("M68K Write16 SID reg[%d]=0x%02X, want 0x%02X", i, got, want)
		}
	}
}
