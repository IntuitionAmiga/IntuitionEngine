package main

import "testing"

func TestZ80EDLoadIAndR(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0x47, // LD I,A
		0xED, 0x57, // LD A,I
		0xED, 0x4F, // LD R,A
		0xED, 0x5F, // LD A,R
	})
	rig.cpu.A = 0x80
	rig.cpu.IFF2 = true
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	requireZ80EqualU8(t, "I", rig.cpu.I, 0x80)
	requireZ80EqualU8(t, "F", rig.cpu.F, z80FlagC)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x80)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x85)

	rig.cpu.Step()
	requireZ80EqualU8(t, "R", rig.cpu.R, 0x80)
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x82)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x85)
	if rig.cpu.Cycles != 36 {
		t.Fatalf("Cycles = %d, want 36", rig.cpu.Cycles)
	}
}

func TestZ80EDINOUT(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0x40, // IN B,(C)
		0xED, 0x41, // OUT (C),B
	})
	rig.cpu.SetBC(0x1234)
	rig.bus.io[0x1234] = 0x55
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x55)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x05)

	rig.bus.io[0x1234] = 0x00
	rig.cpu.Step()
	if rig.bus.io[0x5534] != 0x55 {
		t.Fatalf("port 0x5534 = %02X, want 55", rig.bus.io[0x5534])
	}
	if rig.cpu.Cycles != 24 {
		t.Fatalf("Cycles = %d, want 24", rig.cpu.Cycles)
	}
}

func TestZ80EDNEG(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xED, 0x44}) // NEG
	rig.cpu.A = 0x01

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0xFF)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xBB)
	if rig.cpu.Cycles != 8 {
		t.Fatalf("Cycles = %d, want 8", rig.cpu.Cycles)
	}
}

func TestZ80EDIMAndRETN(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0x46, // IM 0
		0xED, 0x56, // IM 1
		0xED, 0x5E, // IM 2
		0xED, 0x45, // RETN
	})
	rig.cpu.SP = 0x9000
	rig.bus.mem[0x9000] = 0x34
	rig.bus.mem[0x9001] = 0x12
	rig.cpu.IFF2 = true
	rig.cpu.IFF1 = false

	rig.cpu.Step()
	if rig.cpu.IM != 0 {
		t.Fatalf("IM = %d, want 0", rig.cpu.IM)
	}
	rig.cpu.Step()
	if rig.cpu.IM != 1 {
		t.Fatalf("IM = %d, want 1", rig.cpu.IM)
	}
	rig.cpu.Step()
	if rig.cpu.IM != 2 {
		t.Fatalf("IM = %d, want 2", rig.cpu.IM)
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x1234)
	if !rig.cpu.IFF1 {
		t.Fatalf("IFF1 should be restored from IFF2")
	}
	if rig.cpu.Cycles != 38 {
		t.Fatalf("Cycles = %d, want 38", rig.cpu.Cycles)
	}
}

func TestZ80EDRRDRLD(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0x67, // RRD
		0xED, 0x6F, // RLD
	})
	rig.cpu.A = 0x12
	rig.cpu.SetHL(0x4000)
	rig.bus.mem[0x4000] = 0x34
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x14)
	if rig.bus.mem[0x4000] != 0x23 {
		t.Fatalf("mem[0x4000] = %02X, want 23", rig.bus.mem[0x4000])
	}
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x05)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x12)
	if rig.bus.mem[0x4000] != 0x34 {
		t.Fatalf("mem[0x4000] = %02X, want 34", rig.bus.mem[0x4000])
	}
	if rig.cpu.Cycles != 36 {
		t.Fatalf("Cycles = %d, want 36", rig.cpu.Cycles)
	}
}

func TestZ80EDLoad16AndAdcSbc(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xED, 0x43, 0x00, 0x80, // LD (0x8000),BC
		0xED, 0x4B, 0x00, 0x80, // LD BC,(0x8000)
		0xED, 0x4A, // ADC HL,BC
		0xED, 0x42, // SBC HL,BC
	})
	rig.cpu.SetBC(0x1234)
	rig.cpu.SetHL(0x0000)
	rig.cpu.F = 0

	rig.cpu.Step()
	if rig.bus.mem[0x8000] != 0x34 || rig.bus.mem[0x8001] != 0x12 {
		t.Fatalf("mem = %02X %02X, want 34 12", rig.bus.mem[0x8000], rig.bus.mem[0x8001])
	}

	rig.cpu.SetBC(0x0000)
	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x1234)

	rig.cpu.SetHL(0xFFFF)
	rig.cpu.SetBC(0x0001)
	rig.cpu.F = 0
	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x0000)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x51)

	rig.cpu.SetHL(0x0000)
	rig.cpu.SetBC(0x0001)
	rig.cpu.F = z80FlagC
	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0xFFFE)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xBB)
}
