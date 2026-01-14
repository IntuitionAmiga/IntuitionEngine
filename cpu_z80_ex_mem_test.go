package main

import "testing"

func TestZ80EXSPHL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xE3}) // EX (SP),HL
	rig.cpu.SP = 0x9000
	rig.cpu.SetHL(0x1234)
	rig.bus.mem[0x9000] = 0xAA
	rig.bus.mem[0x9001] = 0xBB

	rig.cpu.Step()

	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0xBBAA)
	if rig.bus.mem[0x9000] != 0x34 || rig.bus.mem[0x9001] != 0x12 {
		t.Fatalf("stack swap failed: mem=%02X %02X", rig.bus.mem[0x9000], rig.bus.mem[0x9001])
	}
	requireZ80EqualU16(t, "WZ", rig.cpu.WZ, 0xBBAA)
	if rig.cpu.Cycles != 19 {
		t.Fatalf("Cycles = %d, want 19", rig.cpu.Cycles)
	}
}

func TestZ80EXAF(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0x08}) // EX AF,AF'
	rig.cpu.A = 0x12
	rig.cpu.F = 0x34
	rig.cpu.A2 = 0x56
	rig.cpu.F2 = 0x78

	rig.cpu.Step()

	requireZ80EqualU8(t, "A", rig.cpu.A, 0x56)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x78)
	if rig.cpu.Cycles != 4 {
		t.Fatalf("Cycles = %d, want 4", rig.cpu.Cycles)
	}
}

func TestZ80JPHL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{0xE9}) // JP (HL)
	rig.cpu.SetHL(0x3456)

	rig.cpu.Step()

	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x3456)
	requireZ80EqualU16(t, "WZ", rig.cpu.WZ, 0x3456)
	if rig.cpu.Cycles != 4 {
		t.Fatalf("Cycles = %d, want 4", rig.cpu.Cycles)
	}
}

func TestZ80LDNNHLAndLDHLNN(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x22, 0x00, 0x80, // LD (0x8000),HL
		0x2A, 0x00, 0x80, // LD HL,(0x8000)
	})
	rig.cpu.SetHL(0xABCD)

	rig.cpu.Step()
	if rig.bus.mem[0x8000] != 0xCD || rig.bus.mem[0x8001] != 0xAB {
		t.Fatalf("mem = %02X %02X, want CD AB", rig.bus.mem[0x8000], rig.bus.mem[0x8001])
	}
	requireZ80EqualU16(t, "WZ", rig.cpu.WZ, 0x8001)

	rig.cpu.SetHL(0x0000)
	rig.cpu.Step()
	requireZ80EqualU16(t, "HL", rig.cpu.HL(), 0xABCD)
	requireZ80EqualU16(t, "WZ", rig.cpu.WZ, 0x8001)
}

func TestZ80LDNNAAndLDANN(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x32, 0x00, 0x90, // LD (0x9000),A
		0x3A, 0x00, 0x90, // LD A,(0x9000)
	})
	rig.cpu.A = 0x55

	rig.cpu.Step()
	if rig.bus.mem[0x9000] != 0x55 {
		t.Fatalf("mem[0x9000] = %02X, want 55", rig.bus.mem[0x9000])
	}
	requireZ80EqualU16(t, "WZ", rig.cpu.WZ, 0x9000)

	rig.cpu.A = 0x00
	rig.cpu.Step()
	requireZ80EqualU8(t, "A", rig.cpu.A, 0x55)
	requireZ80EqualU16(t, "WZ", rig.cpu.WZ, 0x9000)
}
