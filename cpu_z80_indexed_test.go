package main

import "testing"

func TestZ80DDLoadAndStack(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x21, 0x34, 0x12, // LD IX,0x1234
		0xDD, 0x22, 0x00, 0x80, // LD (0x8000),IX
		0xDD, 0x2A, 0x00, 0x80, // LD IX,(0x8000)
		0xDD, 0xE5, // PUSH IX
		0xDD, 0xE1, // POP IX
		0xDD, 0xF9, // LD SP,IX
	})

	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1234)
	rig.cpu.Step()
	if rig.bus.mem[0x8000] != 0x34 || rig.bus.mem[0x8001] != 0x12 {
		t.Fatalf("mem = %02X %02X, want 34 12", rig.bus.mem[0x8000], rig.bus.mem[0x8001])
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1234)

	rig.cpu.SP = 0x9000
	rig.cpu.Step()
	if rig.cpu.SP != 0x8FFE {
		t.Fatalf("SP = 0x%04X, want 0x8FFE", rig.cpu.SP)
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "IX", rig.cpu.IX, 0x1234)

	rig.cpu.Step()
	if rig.cpu.SP != 0x1234 {
		t.Fatalf("SP = 0x%04X, want 0x1234", rig.cpu.SP)
	}
}

func TestZ80DDIndexOps(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0x36, 0x05, 0xAA, // LD (IX+5),0xAA
		0xDD, 0x34, 0x05, // INC (IX+5)
		0xDD, 0x35, 0x05, // DEC (IX+5)
	})
	rig.cpu.IX = 0x2000

	rig.cpu.Step()
	if rig.bus.mem[0x2005] != 0xAA {
		t.Fatalf("mem[0x2005] = %02X, want AA", rig.bus.mem[0x2005])
	}
	rig.cpu.Step()
	if rig.bus.mem[0x2005] != 0xAB {
		t.Fatalf("mem[0x2005] = %02X, want AB", rig.bus.mem[0x2005])
	}
	rig.cpu.Step()
	if rig.bus.mem[0x2005] != 0xAA {
		t.Fatalf("mem[0x2005] = %02X, want AA", rig.bus.mem[0x2005])
	}
	if rig.cpu.Cycles != 65 {
		t.Fatalf("Cycles = %d, want 65", rig.cpu.Cycles)
	}
}

func TestZ80DDCBBitOps(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0xCB, 0x02, 0x06, // RLC (IX+2)
		0xDD, 0xCB, 0x02, 0x46, // BIT 0,(IX+2)
		0xDD, 0xCB, 0x02, 0x86, // RES 0,(IX+2)
		0xDD, 0xCB, 0x02, 0xC6, // SET 0,(IX+2)
	})
	rig.cpu.IX = 0x3000
	rig.bus.mem[0x3002] = 0x80

	rig.cpu.Step()
	if rig.bus.mem[0x3002] != 0x01 {
		t.Fatalf("mem[0x3002] = %02X, want 01", rig.bus.mem[0x3002])
	}
	if rig.cpu.Cycles != 23 {
		t.Fatalf("Cycles = %d, want 23", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	if rig.cpu.Cycles != 43 {
		t.Fatalf("Cycles = %d, want 43", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	if rig.bus.mem[0x3002] != 0x00 {
		t.Fatalf("mem[0x3002] = %02X, want 00", rig.bus.mem[0x3002])
	}

	rig.cpu.Step()
	if rig.bus.mem[0x3002] != 0x01 {
		t.Fatalf("mem[0x3002] = %02X, want 01", rig.bus.mem[0x3002])
	}
}

func TestZ80DDCBSLL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xDD, 0xCB, 0x01, 0x36, // SLL (IX+1)
	})
	rig.cpu.IX = 0x4000
	rig.bus.mem[0x4001] = 0x80

	rig.cpu.Step()

	if rig.bus.mem[0x4001] != 0x01 {
		t.Fatalf("mem[0x4001] = %02X, want 01", rig.bus.mem[0x4001])
	}
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x01)
	if rig.cpu.Cycles != 23 {
		t.Fatalf("Cycles = %d, want 23", rig.cpu.Cycles)
	}
}
