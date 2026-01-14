package main

import "testing"

func TestZ80CBRotateShift(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xCB, 0x00, // RLC B
		0xCB, 0x08, // RRC B
		0xCB, 0x10, // RL B
		0xCB, 0x18, // RR B
		0xCB, 0x20, // SLA B
		0xCB, 0x28, // SRA B
		0xCB, 0x38, // SRL B
	})
	rig.cpu.B = 0x81

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x03)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x05)

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x85)

	rig.cpu.F = z80FlagC
	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x03)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x05)

	rig.cpu.F = z80FlagC
	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x81)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x85)

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x02)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x01)

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x01)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x00)

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x00)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x45)
}

func TestZ80CBBIT(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xCB, 0x47, // BIT 0,A
		0xCB, 0x7F, // BIT 7,A
	})
	rig.cpu.A = 0x01

	rig.cpu.Step()
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x10)

	rig.cpu.Step()
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x54)
}

func TestZ80CBRESSET(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xCB, 0x80, // RES 0,B
		0xCB, 0xC0, // SET 0,B
	})
	rig.cpu.B = 0x01

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x00)
	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x01)
}

func TestZ80CBMemoryTiming(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xCB, 0x06, // RLC (HL)
		0xCB, 0x46, // BIT 0,(HL)
		0xCB, 0x86, // RES 0,(HL)
		0xCB, 0xC6, // SET 0,(HL)
	})
	rig.cpu.SetHL(0x4000)
	rig.bus.mem[0x4000] = 0x80

	rig.cpu.Step()
	if rig.cpu.Cycles != 15 {
		t.Fatalf("Cycles = %d, want 15", rig.cpu.Cycles)
	}
	if rig.bus.mem[0x4000] != 0x01 {
		t.Fatalf("mem[0x4000] = %02X, want 01", rig.bus.mem[0x4000])
	}

	rig.cpu.Step()
	if rig.cpu.Cycles != 27 {
		t.Fatalf("Cycles = %d, want 27", rig.cpu.Cycles)
	}

	rig.cpu.Step()
	if rig.bus.mem[0x4000] != 0x00 {
		t.Fatalf("mem[0x4000] = %02X, want 00", rig.bus.mem[0x4000])
	}

	rig.cpu.Step()
	if rig.bus.mem[0x4000] != 0x01 {
		t.Fatalf("mem[0x4000] = %02X, want 01", rig.bus.mem[0x4000])
	}
}

func TestZ80CBSLL(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xCB, 0x30, // SLL B
	})
	rig.cpu.B = 0x80

	rig.cpu.Step()

	requireZ80EqualU8(t, "B", rig.cpu.B, 0x01)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x01)
}
