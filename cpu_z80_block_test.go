package main

import "testing"

func TestZ80LDIAndLDIR(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0xA0, // LDI
		0xED, 0xB0, // LDIR
	})
	rig.cpu.A = 0x10
	rig.cpu.SetHL(0x4000)
	rig.cpu.SetDE(0x5000)
	rig.cpu.SetBC(0x0001)
	rig.bus.mem[0x4000] = 0x22
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	if rig.bus.mem[0x5000] != 0x22 {
		t.Fatalf("mem[0x5000] = %02X, want 22", rig.bus.mem[0x5000])
	}
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4001)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x5001)
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0000)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x21)
	if rig.cpu.Cycles != 16 {
		t.Fatalf("Cycles = %d, want 16", rig.cpu.Cycles)
	}

	rig.resetAndLoad(0x0000, []byte{
		0xED, 0xB0, // LDIR
	})
	rig.cpu.A = 0x00
	rig.cpu.SetHL(0x4100)
	rig.cpu.SetDE(0x5100)
	rig.cpu.SetBC(0x0002)
	rig.bus.mem[0x4100] = 0x11
	rig.bus.mem[0x4101] = 0x22

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0001)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4101)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x5101)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
	if rig.cpu.Cycles != 21 {
		t.Fatalf("Cycles = %d, want 21", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0000)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4102)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x5102)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	if rig.cpu.Cycles != 37 {
		t.Fatalf("Cycles = %d, want 37", rig.cpu.Cycles)
	}
	if rig.bus.mem[0x5100] != 0x11 || rig.bus.mem[0x5101] != 0x22 {
		t.Fatalf("mem copy failed")
	}
}

func TestZ80LDDAndLDDR(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0xA8, // LDD
		0xED, 0xB8, // LDDR
	})
	rig.cpu.A = 0x00
	rig.cpu.SetHL(0x4201)
	rig.cpu.SetDE(0x5201)
	rig.cpu.SetBC(0x0001)
	rig.bus.mem[0x4201] = 0x33

	rig.cpu.Step()
	if rig.bus.mem[0x5201] != 0x33 {
		t.Fatalf("mem[0x5201] = %02X, want 33", rig.bus.mem[0x5201])
	}
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4200)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x5200)
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0000)
	if rig.cpu.Cycles != 16 {
		t.Fatalf("Cycles = %d, want 16", rig.cpu.Cycles)
	}

	rig.resetAndLoad(0x0000, []byte{
		0xED, 0xB8, // LDDR
	})
	rig.cpu.SetHL(0x4301)
	rig.cpu.SetDE(0x5301)
	rig.cpu.SetBC(0x0002)
	rig.bus.mem[0x4301] = 0x44
	rig.bus.mem[0x4300] = 0x55

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0001)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4300)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x5300)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
	if rig.cpu.Cycles != 21 {
		t.Fatalf("Cycles = %d, want 21", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0000)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x42FF)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x52FF)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	if rig.cpu.Cycles != 37 {
		t.Fatalf("Cycles = %d, want 37", rig.cpu.Cycles)
	}
	if rig.bus.mem[0x5301] != 0x44 || rig.bus.mem[0x5300] != 0x55 {
		t.Fatalf("mem copy failed")
	}
}

func TestZ80CPIAndCPIR(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0xA1, // CPI
		0xED, 0xB1, // CPIR
	})
	rig.cpu.A = 0x20
	rig.cpu.SetHL(0x4400)
	rig.cpu.SetBC(0x0001)
	rig.bus.mem[0x4400] = 0x10

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0000)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4401)
	if rig.cpu.Cycles != 16 {
		t.Fatalf("Cycles = %d, want 16", rig.cpu.Cycles)
	}

	rig.resetAndLoad(0x0000, []byte{
		0xED, 0xB1, // CPIR
	})
	rig.cpu.A = 0x20
	rig.cpu.SetHL(0x4500)
	rig.cpu.SetBC(0x0002)
	rig.bus.mem[0x4500] = 0x10
	rig.bus.mem[0x4501] = 0x20

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0001)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4501)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
	if rig.cpu.Cycles != 21 {
		t.Fatalf("Cycles = %d, want 21", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0000)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x4502)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	if rig.cpu.Cycles != 37 {
		t.Fatalf("Cycles = %d, want 37", rig.cpu.Cycles)
	}
	if !rig.cpu.Flag(z80FlagZ) {
		t.Fatalf("Z should be set after match")
	}
}
