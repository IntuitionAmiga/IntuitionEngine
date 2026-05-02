package main

import "testing"

func TestPOKEY_WideWrite_M68KStyle(t *testing.T) {
	bus := NewMachineBus()
	engine := NewPOKEYEngine(nil, 44100)
	bus.MapIO(POKEY_BASE, POKEY_END, engine.HandleRead, engine.HandleWrite)
	bus.MapIOByte(POKEY_BASE, POKEY_END, engine.HandleWrite8)
	bus.MapIOWideWriteFanout(POKEY_BASE, POKEY_END)
	cpu := NewM68KCPU(bus)

	cpu.Write32(POKEY_BASE, 0x11223344)
	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if got := engine.regs[i]; got != want {
			t.Fatalf("M68K Write32 POKEY reg[%d]=0x%02X, want 0x%02X", i, got, want)
		}
	}

	engine.Reset()
	cpu.Write16(POKEY_BASE, 0x5566)
	for i, want := range []uint8{0x55, 0x66} {
		if got := engine.regs[i]; got != want {
			t.Fatalf("M68K Write16 POKEY reg[%d]=0x%02X, want 0x%02X", i, got, want)
		}
	}
}
