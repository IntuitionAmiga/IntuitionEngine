package main

import "testing"

func TestZ80FlagHelpers(t *testing.T) {
	rig := newCPUZ80TestRig()
	cpu := rig.cpu

	cpu.F = 0
	cpu.SetFlag(z80FlagS, true)
	cpu.SetFlag(z80FlagZ, true)
	cpu.SetFlag(z80FlagH, true)
	cpu.SetFlag(z80FlagPV, true)
	cpu.SetFlag(z80FlagN, true)
	cpu.SetFlag(z80FlagC, true)
	cpu.SetFlag(z80FlagX, true)
	cpu.SetFlag(z80FlagY, true)

	if cpu.F != 0xFF {
		t.Fatalf("F = 0x%02X, want 0xFF", cpu.F)
	}

	cpu.SetFlag(z80FlagZ, false)
	cpu.SetFlag(z80FlagN, false)

	if cpu.Flag(z80FlagZ) || cpu.Flag(z80FlagN) {
		t.Fatalf("Z or N flag should be cleared")
	}
	if cpu.F != 0xBD {
		t.Fatalf("F = 0x%02X, want 0xBD", cpu.F)
	}
}

func TestZ80ExchangeRegisters(t *testing.T) {
	rig := newCPUZ80TestRig()
	cpu := rig.cpu

	cpu.A = 0x12
	cpu.F = 0x34
	cpu.A2 = 0x56
	cpu.F2 = 0x78
	cpu.ExAF()
	requireZ80EqualU8(t, "A", cpu.A, 0x56)
	requireZ80EqualU8(t, "F", cpu.F, 0x78)
	requireZ80EqualU8(t, "A'", cpu.A2, 0x12)
	requireZ80EqualU8(t, "F'", cpu.F2, 0x34)

	cpu.B = 0x01
	cpu.C = 0x02
	cpu.D = 0x03
	cpu.E = 0x04
	cpu.H = 0x05
	cpu.L = 0x06
	cpu.B2 = 0x11
	cpu.C2 = 0x12
	cpu.D2 = 0x13
	cpu.E2 = 0x14
	cpu.H2 = 0x15
	cpu.L2 = 0x16
	cpu.Exx()

	requireZ80EqualU8(t, "B", cpu.B, 0x11)
	requireZ80EqualU8(t, "C", cpu.C, 0x12)
	requireZ80EqualU8(t, "D", cpu.D, 0x13)
	requireZ80EqualU8(t, "E", cpu.E, 0x14)
	requireZ80EqualU8(t, "H", cpu.H, 0x15)
	requireZ80EqualU8(t, "L", cpu.L, 0x16)
	requireZ80EqualU8(t, "B'", cpu.B2, 0x01)
	requireZ80EqualU8(t, "C'", cpu.C2, 0x02)
	requireZ80EqualU8(t, "D'", cpu.D2, 0x03)
	requireZ80EqualU8(t, "E'", cpu.E2, 0x04)
	requireZ80EqualU8(t, "H'", cpu.H2, 0x05)
	requireZ80EqualU8(t, "L'", cpu.L2, 0x06)
}
