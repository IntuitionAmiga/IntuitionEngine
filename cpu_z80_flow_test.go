package main

import "testing"

func TestZ80IncDec8(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x04, // INC B
		0x05, // DEC B
		0x34, // INC (HL)
		0x35, // DEC (HL)
	})
	rig.cpu.B = 0x7F
	rig.cpu.SetHL(0x2000)
	rig.bus.mem[0x2000] = 0x00

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x80)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x94)

	rig.cpu.Step()
	requireZ80EqualU8(t, "B", rig.cpu.B, 0x7F)
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x3E)

	rig.cpu.Step()
	if rig.bus.mem[0x2000] != 0x01 {
		t.Fatalf("mem[0x2000] = %02X, want 01", rig.bus.mem[0x2000])
	}
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x00)

	rig.cpu.Step()
	if rig.bus.mem[0x2000] != 0x00 {
		t.Fatalf("mem[0x2000] = %02X, want 00", rig.bus.mem[0x2000])
	}
	requireZ80EqualU8(t, "F", rig.cpu.F, 0x42)
}

func TestZ80ConditionalJumps(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xC2, 0x08, 0x00, // JP NZ,0x0008
		0xC3, 0x0B, 0x00, // JP 0x000B
		0x00, // NOP (0x0006)
		0x00, // NOP (0x0007)
		0x00, // NOP (0x0008)
		0x00, // NOP (0x0009)
		0x00, // NOP (0x000A)
		0x00, // NOP (0x000B)
	})

	rig.cpu.F = 0
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0008)
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0009)

	rig.resetAndLoad(0x0000, []byte{
		0xC2, 0x08, 0x00, // JP NZ,0x0008
		0xC3, 0x0B, 0x00, // JP 0x000B
		0x00, // NOP (0x0006)
		0x00, // NOP (0x0007)
		0x00, // NOP (0x0008)
		0x00, // NOP (0x0009)
		0x00, // NOP (0x000A)
		0x00, // NOP (0x000B)
	})
	rig.cpu.F = z80FlagZ
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0003)
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x000B)
}

func TestZ80ConditionalJR(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0x20, 0x02, // JR NZ,+2
		0x00, 0x00, // NOP, NOP
		0x28, 0xFE, // JR Z,-2
	})
	rig.cpu.F = 0

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0004)
	if rig.cpu.Cycles != 12 {
		t.Fatalf("Cycles = %d, want 12", rig.cpu.Cycles)
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0006)

	rig.resetAndLoad(0x0000, []byte{
		0x28, 0xFE, // JR Z,-2
	})
	rig.cpu.F = z80FlagZ
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
	if rig.cpu.Cycles != 12 {
		t.Fatalf("Cycles = %d, want 12", rig.cpu.Cycles)
	}
}

func TestZ80ConditionalCallRet(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xC4, 0x06, 0x00, // CALL NZ,0x0006
		0xC9,       // RET (if call not taken)
		0x00, 0x00, // padding
		0xC9, // RET (call target)
		0x00, // NOP
	})
	rig.cpu.SP = 0x9000
	rig.cpu.F = 0

	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0006)
	if rig.cpu.SP != 0x8FFE {
		t.Fatalf("SP = 0x%04X, want 0x8FFE", rig.cpu.SP)
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0003)

	rig.resetAndLoad(0x0000, []byte{
		0xC4, 0x06, 0x00, // CALL NZ,0x0006
		0xC9,       // RET (if call not taken)
		0x00, 0x00, // padding
		0xC9, // RET (call target)
		0x00, // NOP
	})
	rig.cpu.SP = 0x9000
	rig.cpu.F = z80FlagZ
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0003)
	if rig.cpu.SP != 0x9000 {
		t.Fatalf("SP = 0x%04X, want 0x9000", rig.cpu.SP)
	}
	rig.cpu.Step()
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
}
