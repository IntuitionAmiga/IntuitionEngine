package main

import "testing"

func TestZ80INIFlagsAndTiming(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xED, 0xA2}) // INI
	rig.cpu.SetBC(0x1007)
	rig.cpu.SetHL(0x2000)
	rig.bus.io[0x1007] = 0x7B
	rig.cpu.F = z80FlagC | z80FlagS

	rig.cpu.Step()

	if rig.bus.mem[0x2000] != 0x7B {
		t.Fatalf("mem[0x2000] = %02X, want 7B", rig.bus.mem[0x2000])
	}
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x0F)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x2001)
	requireZ80EqualU8(t, "F", rig.cpu.F, z80FlagS|z80FlagN|z80FlagC)
	if rig.cpu.Cycles != 16 {
		t.Fatalf("Cycles = %d, want 16", rig.cpu.Cycles)
	}
}

func TestZ80OUTIUsesDecrementedB(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xED, 0xA3}) // OUTI
	rig.cpu.SetBC(0x1007)
	rig.cpu.SetHL(0x3000)
	rig.bus.mem[0x3000] = 0x59
	rig.cpu.F = z80FlagC

	rig.cpu.Step()

	if rig.bus.io[0x0F07] != 0x59 {
		t.Fatalf("port 0x0F07 = %02X, want 59", rig.bus.io[0x0F07])
	}
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x0F)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x3001)
	requireZ80EqualU8(t, "F", rig.cpu.F, z80FlagN|z80FlagC)
	if rig.cpu.Cycles != 16 {
		t.Fatalf("Cycles = %d, want 16", rig.cpu.Cycles)
	}
}

func TestZ80INIRRepeatTiming(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xED, 0xB2}) // INIR
	rig.cpu.SetBC(0x0207)
	rig.cpu.SetHL(0x4000)
	rig.bus.io[0x0207] = 0x11
	rig.bus.io[0x0107] = 0x22

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x01)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4001)
	if rig.cpu.Cycles != 21 {
		t.Fatalf("Cycles = %d, want 21", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x00)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4002)
	if rig.cpu.Cycles != 37 {
		t.Fatalf("Cycles = %d, want 37", rig.cpu.Cycles)
	}
	if rig.bus.mem[0x4000] != 0x11 || rig.bus.mem[0x4001] != 0x22 {
		t.Fatalf("memory input failed")
	}
}

func TestZ80OTDRRepeatTiming(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xED, 0xBB}) // OTDR
	rig.cpu.SetBC(0x0207)
	rig.cpu.SetHL(0x5001)
	rig.bus.mem[0x5001] = 0x33
	rig.bus.mem[0x5000] = 0x44

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x01)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x5000)
	if rig.cpu.Cycles != 21 {
		t.Fatalf("Cycles = %d, want 21", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x00)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4FFF)
	if rig.cpu.Cycles != 37 {
		t.Fatalf("Cycles = %d, want 37", rig.cpu.Cycles)
	}
	if rig.bus.io[0x0107] != 0x33 || rig.bus.io[0x0007] != 0x44 {
		t.Fatalf("port output failed")
	}
}
