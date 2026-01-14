package main

import "testing"

func TestZ80CPLFlags(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x2F}) // CPL
	rig.cpu.A = 0x55
	rig.cpu.F = z80FlagS | z80FlagZ | z80FlagPV | z80FlagC

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0xAA)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xFF)
}

func TestZ80SCFAndCCF(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x37, 0x3F}) // SCF, CCF
	rig.cpu.A = 0x28
	rig.cpu.F = z80FlagS | z80FlagZ | z80FlagPV

	rig.cpu.Step()
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xED)

	rig.cpu.Step()
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xFC)
}

func TestZ80DAAAdd(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x27}) // DAA
	rig.cpu.A = 0x9A
	rig.cpu.F = 0

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x00)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x55)
}

func TestZ80DAASub(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x27}) // DAA
	rig.cpu.A = 0x15
	rig.cpu.F = z80FlagN | z80FlagH

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x0F)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x1E)
}
