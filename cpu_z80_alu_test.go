package main

import "testing"

func TestZ80ALUAdd(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x80}) // ADD A,B
	rig.cpu.A = 0x0F
	rig.cpu.B = 0x01

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x10)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x10)
}

func TestZ80ALUAddOverflow(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x80}) // ADD A,B
	rig.cpu.A = 0x7F
	rig.cpu.B = 0x01

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x80)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x94)
}

func TestZ80ALUAdcWithCarry(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x88}) // ADC A,B
	rig.cpu.A = 0xFF
	rig.cpu.B = 0x00
	rig.cpu.F = z80FlagC

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x00)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x51)
}

func TestZ80ALUSub(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x90}) // SUB B
	rig.cpu.A = 0x10
	rig.cpu.B = 0x01

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x0F)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x1A)
}

func TestZ80ALUSbcWithCarry(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x98}) // SBC A,B
	rig.cpu.A = 0x00
	rig.cpu.B = 0x00
	rig.cpu.F = z80FlagC

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0xFF)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xBB)
}

func TestZ80ALUAnd(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xA0}) // AND B
	rig.cpu.A = 0xF0
	rig.cpu.B = 0x0F

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x00)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x54)
}

func TestZ80ALUXor(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xA8}) // XOR B
	rig.cpu.A = 0xFF
	rig.cpu.B = 0x0F

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0xF0)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xA4)
}

func TestZ80ALUOr(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xB0}) // OR B
	rig.cpu.A = 0x01
	rig.cpu.B = 0x80

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x84)
}

func TestZ80ALUCp(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xFE, 0x20}) // CP 0x20
	rig.cpu.A = 0x10

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x10)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xA3)
}

func TestZ80ALUTiming(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x80,       // ADD A,B
		0x86,       // ADD A,(HL)
		0xC6, 0x01, // ADD A,0x01
	})
	rig.cpu.B = 0x01
	rig.cpu.SetHL(0x2000)
	rig.bus.mem[0x2000] = 0x01

	rig.cpu.Step()
	if rig.cpu.Cycles != 4 {
		t.Fatalf("Cycles after ADD A,B = %d, want 4", rig.cpu.Cycles)
	}
	rig.cpu.Step()
	if rig.cpu.Cycles != 11 {
		t.Fatalf("Cycles after ADD A,(HL) = %d, want 11", rig.cpu.Cycles)
	}
	rig.cpu.Step()
	if rig.cpu.Cycles != 18 {
		t.Fatalf("Cycles after ADD A,n = %d, want 18", rig.cpu.Cycles)
	}
}

func TestZ80ALURegVariants(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x88, // ADC A,B
		0x98, // SBC A,B
		0xA0, // AND B
		0xA8, // XOR B
		0xB0, // OR B
		0xB8, // CP B
	})
	rig.cpu.A = 0x10
	rig.cpu.B = 0x01
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x12)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x00)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x11)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x02)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x01)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x10)

	rig.cpu.A = 0x0F
	rig.cpu.B = 0xF0
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0xFF)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xAC)

	rig.cpu.A = 0x80
	rig.cpu.B = 0x01
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x84)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x82)
}

func TestZ80ALUImmediateVariants(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xCE, 0x01, // ADC A,0x01
		0xDE, 0x01, // SBC A,0x01
		0xE6, 0x0F, // AND 0x0F
		0xEE, 0xF0, // XOR 0xF0
		0xF6, 0x01, // OR 0x01
		0xFE, 0x80, // CP 0x80
	})
	rig.cpu.A = 0x00
	rig.cpu.F = z80FlagC

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x02)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x00)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x01)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x02)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x01)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x10)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0xF1)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xA0)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0xF1)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0xA0)

	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0xF1)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x22)
}
