package main

import "testing"

func TestZ80LD16Immediate(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x01, 0x34, 0x12, // LD BC,0x1234
		0x11, 0x78, 0x56, // LD DE,0x5678
		0x21, 0xCD, 0xAB, // LD HL,0xABCD
		0x31, 0x00, 0x80, // LD SP,0x8000
	})

	rig.cpu.Step()
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x1234)
	rig.cpu.Step()
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x5678)
	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0xABCD)
	rig.cpu.Step()
	requireZ80EqualU16(t, "SP", rig.cpu.SP, 0x8000)
	if rig.cpu.Cycles != 40 {
		t.Fatalf("Cycles = %d, want 40", rig.cpu.Cycles)
	}
}

func TestZ80ADDHL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x09, 0x19, 0x29, 0x39})
	rig.cpu.SetHL(0x0FFF)
	rig.cpu.SetBC(0x0001)
	rig.cpu.SetDE(0x0001)
	rig.cpu.SP = 0x0001

	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x1000)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x10)

	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x1001)

	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x2002)

	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x2003)
	if rig.cpu.Cycles != 44 {
		t.Fatalf("Cycles = %d, want 44", rig.cpu.Cycles)
	}
}

func TestZ80IncDec16(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x03, // INC BC
		0x13, // INC DE
		0x23, // INC HL
		0x33, // INC SP
		0x0B, // DEC BC
		0x1B, // DEC DE
		0x2B, // DEC HL
		0x3B, // DEC SP
	})
	rig.cpu.SetBC(0x0001)
	rig.cpu.SetDE(0x0002)
	rig.cpu.SetHL(0x0003)
	rig.cpu.SP = 0x0004

	for range 4 {
		rig.cpu.Step()
	}
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0002)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x0003)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x0004)
	requireZ80EqualU16(t, "SP", rig.cpu.SP, 0x0005)

	for range 4 {
		rig.cpu.Step()
	}
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x0001)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x0002)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x0003)
	requireZ80EqualU16(t, "SP", rig.cpu.SP, 0x0004)

	if rig.cpu.Cycles != 48 {
		t.Fatalf("Cycles = %d, want 48", rig.cpu.Cycles)
	}
}

func TestZ80PushPop(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xC5, // PUSH BC
		0xD5, // PUSH DE
		0xE5, // PUSH HL
		0xF5, // PUSH AF
		0xF1, // POP AF
		0xE1, // POP HL
		0xD1, // POP DE
		0xC1, // POP BC
	})
	rig.cpu.SetBC(0x1122)
	rig.cpu.SetDE(0x3344)
	rig.cpu.SetHL(0x5566)
	rig.cpu.SetAF(0x7788)
	rig.cpu.SP = 0x9000

	for range 4 {
		rig.cpu.Step()
	}
	if rig.cpu.SP != 0x8FF8 {
		t.Fatalf("SP = 0x%04X, want 0x8FF8", rig.cpu.SP)
	}

	for range 4 {
		rig.cpu.Step()
	}
	requireZ80EqualU16(t, "AF", rig.cpu.AF(), 0x7788)
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0x5566)
	requireZ80EqualU16(t, "DE", rig.cpu.DE(), 0x3344)
	requireZ80EqualU16(t, "BC", rig.cpu.BC(), 0x1122)
	if rig.cpu.SP != 0x9000 {
		t.Fatalf("SP = 0x%04X, want 0x9000", rig.cpu.SP)
	}
}

func TestZ80JPJRCallRet(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x18, 0x02, // JR +2
		0x00, 0x00, // NOP, NOP
		0xC3, 0x08, 0x00, // JP 0x0008
		0x00,             // NOP
		0xCD, 0x0C, 0x00, // CALL 0x000C
		0x00, // NOP (return target)
		0xC9, // RET
	})
	rig.cpu.SP = 0x8000

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0004)
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0008)
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x000C)
	if rig.cpu.SP != 0x7FFE {
		t.Fatalf("SP = 0x%04X, want 0x7FFE", rig.cpu.SP)
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x000B)
	if rig.cpu.SP != 0x8000 {
		t.Fatalf("SP = 0x%04X, want 0x8000", rig.cpu.SP)
	}
}

func TestZ80DJNZTiming(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x10, 0xFE, // DJNZ -2
	})
	rig.cpu.B = 0x02

	rig.cpu.Step()
	if rig.cpu.PC != 0x0000 {
		t.Fatalf("PC = 0x%04X, want 0x0000", rig.cpu.PC)
	}
	if rig.cpu.Cycles != 13 {
		t.Fatalf("Cycles = %d, want 13", rig.cpu.Cycles)
	}
	rig.cpu.Step()
	if rig.cpu.PC != 0x0002 {
		t.Fatalf("PC = 0x%04X, want 0x0002", rig.cpu.PC)
	}
	if rig.cpu.Cycles != 21 {
		t.Fatalf("Cycles = %d, want 21", rig.cpu.Cycles)
	}
}
