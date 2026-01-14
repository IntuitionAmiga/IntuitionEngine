package main

import "testing"

func TestZ80DDPrefixIXHIXL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x26, 0x12, // LD IXH,0x12
		0xDD, 0x2E, 0x34, // LD IXL,0x34
		0xDD, 0x44, // LD B,IXH
		0xDD, 0x4D, // LD C,IXL
		0xDD, 0x84, // ADD A,IXH
	})
	rig.cpu.A = 0x01

	rig.cpu.Step()
	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1234)

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x12)
	rig.cpu.Step()
	requireZ80EqualU8(t, "C", rig.cpu.C, 0x34)
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x13)

	if rig.cpu.Cycles != 46 {
		t.Fatalf("Cycles = %d, want 46", rig.cpu.Cycles)
	}
}

func TestZ80DDPrefixIgnoredNOP(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xDD, 0x00}) // DD NOP

	rig.cpu.Step()
	if rig.cpu.Cycles != 8 {
		t.Fatalf("Cycles = %d, want 8", rig.cpu.Cycles)
	}
}

func TestZ80DDIndexedLoadAndALU(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x46, 0x01, // LD B,(IX+1)
		0xDD, 0x70, 0x02, // LD (IX+2),B
		0xDD, 0x86, 0x03, // ADD A,(IX+3)
	})
	rig.cpu.IX = 0x4000
	rig.cpu.A = 0x10
	rig.bus.mem[0x4001] = 0x22
	rig.bus.mem[0x4003] = 0x05

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x22)
	rig.cpu.Step()
	if rig.bus.mem[0x4002] != 0x22 {
		t.Fatalf("mem[0x4002] = %02X, want 22", rig.bus.mem[0x4002])
	}
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x15)
	if rig.cpu.Cycles != 57 {
		t.Fatalf("Cycles = %d, want 57", rig.cpu.Cycles)
	}
}

func TestZ80DDIndexArithmeticAndInc(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x09, // ADD IX,BC
		0xDD, 0x23, // INC IX
		0xDD, 0x2B, // DEC IX
	})
	rig.cpu.IX = 0x1000
	rig.cpu.SetBC(0x0001)

	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1001)
	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1002)
	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1001)
	if rig.cpu.Cycles != 35 {
		t.Fatalf("Cycles = %d, want 35", rig.cpu.Cycles)
	}
}

func TestZ80FDPrefixIYLoad(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xFD, 0x26, 0x55, // LD IYH,0x55
		0xFD, 0x2E, 0x66, // LD IYL,0x66
		0xFD, 0x46, 0x01, // LD B,(IY+1)
	})
	rig.cpu.IY = 0x2000
	rig.bus.mem[0x5567] = 0x77

	rig.cpu.Step()
	rig.cpu.Step()
	requireZ80EqualU16(t, "IY", rig.cpu.IY, 0x5566)
	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x77)
}

func TestZ80DDLDRegIXdUsesHL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x66, 0x01, // LD H,(IX+1)
		0xDD, 0x75, 0x02, // LD (IX+2),L
	})
	rig.cpu.IX = 0x3000
	rig.cpu.H = 0x11
	rig.cpu.L = 0x22
	rig.bus.mem[0x3001] = 0x99

	rig.cpu.Step()
	requireZ80EqualU8(t, "H", rig.cpu.H, 0x99)
	rig.cpu.Step()
	if rig.bus.mem[0x3002] != 0x22 {
		t.Fatalf("mem[0x3002] = %02X, want 22", rig.bus.mem[0x3002])
	}
}

func TestZ80EXSPIXAndIY(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0xE3, // EX (SP),IX
		0xFD, 0xE3, // EX (SP),IY
	})
	rig.cpu.SP = 0x9000
	rig.bus.mem[0x9000] = 0xAA
	rig.bus.mem[0x9001] = 0xBB
	rig.cpu.IX = 0x1122
	rig.cpu.IY = 0x3344

	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0xBBAA)
	if rig.bus.mem[0x9000] != 0x22 || rig.bus.mem[0x9001] != 0x11 {
		t.Fatalf("stack swap failed: %02X %02X", rig.bus.mem[0x9000], rig.bus.mem[0x9001])
	}
	if rig.cpu.Cycles != 23 {
		t.Fatalf("Cycles = %d, want 23", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	requireZ80EqualU16(t, "IY", rig.cpu.IY, 0x1122)
	if rig.bus.mem[0x9000] != 0x44 || rig.bus.mem[0x9001] != 0x33 {
		t.Fatalf("stack swap failed: %02X %02X", rig.bus.mem[0x9000], rig.bus.mem[0x9001])
	}
	if rig.cpu.Cycles != 46 {
		t.Fatalf("Cycles = %d, want 46", rig.cpu.Cycles)
	}
}

func TestZ80DDPrefixIncDecIndexHighLow(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x24, // INC IXH
		0xDD, 0x2D, // DEC IXL
	})
	rig.cpu.IX = 0x12FF

	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x13FF)
	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x13FE)
	if rig.cpu.Cycles != 16 {
		t.Fatalf("Cycles = %d, want 16", rig.cpu.Cycles)
	}
}
