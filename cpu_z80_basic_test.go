package main

import "testing"

func TestZ80ResetDefaults(t *testing.T) {
	rig := newCPUZ80TestRig()
	cpu := rig.cpu

	cpu.A = 0x11
	cpu.F = 0x22
	cpu.B = 0x33
	cpu.C = 0x44
	cpu.D = 0x55
	cpu.E = 0x66
	cpu.H = 0x77
	cpu.L = 0x88
	cpu.A2 = 0x99
	cpu.F2 = 0xAA
	cpu.B2 = 0xBB
	cpu.C2 = 0xCC
	cpu.D2 = 0xDD
	cpu.E2 = 0xEE
	cpu.H2 = 0xFF
	cpu.L2 = 0x01
	cpu.IX = 0x1234
	cpu.IY = 0x4567
	cpu.SP = 0xABCD
	cpu.PC = 0xFEED
	cpu.I = 0x12
	cpu.R = 0x34
	cpu.IM = 2
	cpu.WZ = 0x2222
	cpu.IFF1 = true
	cpu.IFF2 = true
	cpu.irqLine = true
	cpu.nmiLine = true
	cpu.nmiPending = true
	cpu.nmiPrev = true
	cpu.iffDelay = 1
	cpu.irqVector = 0x00
	cpu.Halted = true
	cpu.Cycles = 999

	cpu.Reset()

	requireZ80EqualU16(t, "PC", cpu.PC, 0x0000)
	requireZ80EqualU16(t, "SP", cpu.SP, 0xFFFF)
	requireZ80EqualU8(t, "A", cpu.A, 0x00)
	requireZ80EqualU8(t, "F", cpu.F, 0x00)
	requireZ80EqualU8(t, "B", cpu.B, 0x00)
	requireZ80EqualU8(t, "C", cpu.C, 0x00)
	requireZ80EqualU8(t, "D", cpu.D, 0x00)
	requireZ80EqualU8(t, "E", cpu.E, 0x00)
	requireZ80EqualU8(t, "H", cpu.H, 0x00)
	requireZ80EqualU8(t, "L", cpu.L, 0x00)
	requireZ80EqualU8(t, "A'", cpu.A2, 0x00)
	requireZ80EqualU8(t, "F'", cpu.F2, 0x00)
	requireZ80EqualU8(t, "B'", cpu.B2, 0x00)
	requireZ80EqualU8(t, "C'", cpu.C2, 0x00)
	requireZ80EqualU8(t, "D'", cpu.D2, 0x00)
	requireZ80EqualU8(t, "E'", cpu.E2, 0x00)
	requireZ80EqualU8(t, "H'", cpu.H2, 0x00)
	requireZ80EqualU8(t, "L'", cpu.L2, 0x00)
	requireZ80EqualU16(t, "IX", cpu.IX, 0x0000)
	requireZ80EqualU16(t, "IY", cpu.IY, 0x0000)
	requireZ80EqualU8(t, "I", cpu.I, 0x00)
	requireZ80EqualU8(t, "R", cpu.R, 0x00)
	requireZ80EqualU16(t, "WZ", cpu.WZ, 0x0000)
	if cpu.IFF1 || cpu.IFF2 {
		t.Fatalf("IFF1/IFF2 should be cleared on reset")
	}
	if cpu.irqLine || cpu.nmiLine || cpu.nmiPending || cpu.nmiPrev {
		t.Fatalf("interrupt lines should be cleared on reset")
	}
	if cpu.iffDelay != 0 {
		t.Fatalf("iffDelay should be cleared on reset")
	}
	if cpu.irqVector != 0xFF {
		t.Fatalf("irqVector = 0x%02X, want 0xFF", cpu.irqVector)
	}
	if cpu.IM != 0 {
		t.Fatalf("IM = %d, want 0", cpu.IM)
	}
	if cpu.Halted {
		t.Fatalf("Halted should be false on reset")
	}
	if cpu.Cycles != 0 {
		t.Fatalf("Cycles = %d, want 0", cpu.Cycles)
	}
	if !cpu.Running {
		t.Fatalf("Running should be true after reset")
	}
}

func TestZ80RegisterPairs(t *testing.T) {
	rig := newCPUZ80TestRig()
	cpu := rig.cpu

	cpu.SetAF(0x1234)
	cpu.SetBC(0x2345)
	cpu.SetDE(0x3456)
	cpu.SetHL(0x4567)
	cpu.SetAF2(0x6789)
	cpu.SetBC2(0x789A)
	cpu.SetDE2(0x89AB)
	cpu.SetHL2(0x9ABC)

	requireZ80EqualU16(t, "AF", cpu.AF(), 0x1234)
	requireZ80EqualU16(t, "BC", cpu.BC(), 0x2345)
	requireZ80EqualU16(t, "DE", cpu.DE(), 0x3456)
	requireZ80EqualU16(t, "HL", cpu.HL(), 0x4567)
	requireZ80EqualU16(t, "AF'", cpu.AF2(), 0x6789)
	requireZ80EqualU16(t, "BC'", cpu.BC2(), 0x789A)
	requireZ80EqualU16(t, "DE'", cpu.DE2(), 0x89AB)
	requireZ80EqualU16(t, "HL'", cpu.HL2(), 0x9ABC)
}

func TestZ80StepNOP(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x00})

	cpu := rig.cpu
	cpu.Step()

	requireZ80EqualU16(t, "PC", cpu.PC, 0x0001)
	if cpu.Cycles != 4 {
		t.Fatalf("Cycles = %d, want 4", cpu.Cycles)
	}
	if rig.bus.ticks != 4 {
		t.Fatalf("bus ticks = %d, want 4", rig.bus.ticks)
	}
}
