package main

import "testing"

func TestZ80LDIndirectBCDE(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x02, // LD (BC),A
		0x0A, // LD A,(BC)
		0x12, // LD (DE),A
		0x1A, // LD A,(DE)
	})
	rig.cpu.SetBC(0x1000)
	rig.cpu.SetDE(0x2000)
	rig.cpu.A = 0x55

	rig.cpu.Step()
	if rig.bus.mem[0x1000] != 0x55 {
		t.Fatalf("mem[0x1000] = %02X, want 55", rig.bus.mem[0x1000])
	}
	rig.bus.mem[0x1000] = 0x66
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x66)

	rig.cpu.A = 0x77
	rig.cpu.Step()
	if rig.bus.mem[0x2000] != 0x77 {
		t.Fatalf("mem[0x2000] = %02X, want 77", rig.bus.mem[0x2000])
	}
	rig.bus.mem[0x2000] = 0x88
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x88)
}

func TestZ80LDSPHL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xF9}) // LD SP,HL
	rig.cpu.SetHL(0xABCD)

	rig.cpu.Step()

	requireZ80EqualU16(t, "SP", rig.cpu.SP, 0xABCD)
	if rig.cpu.Cycles != 6 {
		t.Fatalf("Cycles = %d, want 6", rig.cpu.Cycles)
	}
}

func TestZ80INOUTN(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xD3, 0x34, // OUT (0x34),A
		0xDB, 0x34, // IN A,(0x34)
	})
	rig.cpu.A = 0x12
	rig.bus.io[0x1234] = 0x99
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	if rig.bus.io[0x1234] != 0x12 {
		t.Fatalf("port 0x1234 = %02X, want 12", rig.bus.io[0x1234])
	}

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x12)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x05)
}

func TestZ80RotateAOps(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x07, // RLCA
		0x0F, // RRCA
		0x17, // RLA
		0x1F, // RRA
	})
	rig.cpu.A = 0x81
	rig.cpu.F = z80FlagS | z80FlagZ | z80FlagPV

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x03)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xC5)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xC5)

	rig.cpu.F = z80FlagC | z80FlagS
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x03)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x81)

	rig.cpu.F = z80FlagC | z80FlagZ
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x41)
}

func TestZ80RST(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x1234, []byte{0xCF}) // RST 08h
	rig.cpu.PC = 0x1234
	rig.cpu.SP = 0xFF00

	rig.cpu.Step()

	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0008)
	if rig.cpu.SP != 0xFEFE {
		t.Fatalf("SP = 0x%04X, want 0xFEFE", rig.cpu.SP)
	}
	if rig.bus.mem[0xFEFE] != 0x35 || rig.bus.mem[0xFEFF] != 0x12 {
		t.Fatalf("stack push incorrect: %02X %02X", rig.bus.mem[0xFEFE], rig.bus.mem[0xFEFF])
	}
}

func TestZ80EXDEHLAndEXXOps(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xEB, // EX DE,HL
		0xD9, // EXX
	})
	rig.cpu.SetDE(0x1122)
	rig.cpu.SetHL(0x3344)
	rig.cpu.SetBC(0x5566)
	rig.cpu.SetBC2(0x7788)
	rig.cpu.SetDE2(0x99AA)
	rig.cpu.SetHL2(0xBBCC)

	rig.cpu.Step()
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x3344)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x1122)

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x7788)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x99AA)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0xBBCC)
}
