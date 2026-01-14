package main

import "testing"

func TestZ80LDRegReg(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x41}) // LD B,C
	rig.cpu.C = 0xAA

	rig.cpu.Step()

	requireZ80EqualU8(t, "B", rig.cpu.B, 0xAA)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0001)
	if rig.cpu.Cycles != 4 {
		t.Fatalf("Cycles = %d, want 4", rig.cpu.Cycles)
	}
}

func TestZ80LDRegMem(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x7E}) // LD A,(HL)
	rig.cpu.SetHL(0x2000)
	rig.bus.mem[0x2000] = 0x55

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x55)
	if rig.cpu.Cycles != 7 {
		t.Fatalf("Cycles = %d, want 7", rig.cpu.Cycles)
	}
}

func TestZ80LDMemReg(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x72}) // LD (HL),D
	rig.cpu.SetHL(0x3000)
	rig.cpu.D = 0x66

	rig.cpu.Step()

	if rig.bus.mem[0x3000] != 0x66 {
		t.Fatalf("mem[0x3000] = 0x%02X, want 0x66", rig.bus.mem[0x3000])
	}
	if rig.cpu.Cycles != 7 {
		t.Fatalf("Cycles = %d, want 7", rig.cpu.Cycles)
	}
}

func TestZ80LDRegImm(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x1E, 0x99}) // LD E,n

	rig.cpu.Step()

	requireZ80EqualU8(t, "E", rig.cpu.E, 0x99)
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	if rig.cpu.Cycles != 7 {
		t.Fatalf("Cycles = %d, want 7", rig.cpu.Cycles)
	}
}

func TestZ80LDMemImm(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x36, 0x77}) // LD (HL),n
	rig.cpu.SetHL(0x4000)

	rig.cpu.Step()

	if rig.bus.mem[0x4000] != 0x77 {
		t.Fatalf("mem[0x4000] = 0x%02X, want 0x77", rig.bus.mem[0x4000])
	}
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0002)
	if rig.cpu.Cycles != 10 {
		t.Fatalf("Cycles = %d, want 10", rig.cpu.Cycles)
	}
}
