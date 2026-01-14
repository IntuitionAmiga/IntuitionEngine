package main

import "testing"

func TestZ80DIAndEIDelay(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x0000, []byte{
		0xF3, // DI
		0xFB, // EI
		0x00, // NOP
		0x00, // NOP
	})
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	rig.cpu.SetIRQLine(false)

	rig.cpu.Step()
	if rig.cpu.IFF1 || rig.cpu.IFF2 {
		t.Fatalf("DI should clear IFF1/IFF2")
	}

	rig.cpu.Step()
	if rig.cpu.IFF1 || rig.cpu.IFF2 {
		t.Fatalf("EI should not enable interrupts immediately")
	}

	rig.cpu.Step()
	if !rig.cpu.IFF1 || !rig.cpu.IFF2 {
		t.Fatalf("EI should enable interrupts after one instruction")
	}

	rig.cpu.SetIRQLine(true)
	rig.cpu.Step()
	if rig.cpu.PC != 0x0038 {
		t.Fatalf("IRQ should jump to 0x0038, got 0x%04X", rig.cpu.PC)
	}
}

func TestZ80IM1Interrupt(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x1000, []byte{0x00})
	rig.cpu.PC = 0x1000
	rig.cpu.SP = 0xFF00
	rig.cpu.IM = 1
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	rig.cpu.SetIRQLine(true)

	rig.cpu.Step()

	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0038)
	if rig.cpu.SP != 0xFEFE {
		t.Fatalf("SP = 0x%04X, want 0xFEFE", rig.cpu.SP)
	}
	if rig.bus.mem[0xFEFE] != 0x00 || rig.bus.mem[0xFEFF] != 0x10 {
		t.Fatalf("stack push incorrect: %02X %02X", rig.bus.mem[0xFEFE], rig.bus.mem[0xFEFF])
	}
	if rig.cpu.IFF1 || rig.cpu.IFF2 {
		t.Fatalf("IRQ should clear IFF1/IFF2")
	}
	if rig.cpu.Cycles != 13 {
		t.Fatalf("Cycles = %d, want 13", rig.cpu.Cycles)
	}
}

func TestZ80NMIInterrupt(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.resetAndLoad(0x2000, []byte{0x00})
	rig.cpu.PC = 0x2000
	rig.cpu.SP = 0xFF00
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	rig.cpu.SetNMILine(true)

	rig.cpu.Step()

	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0066)
	if rig.cpu.SP != 0xFEFE {
		t.Fatalf("SP = 0x%04X, want 0xFEFE", rig.cpu.SP)
	}
	if rig.bus.mem[0xFEFE] != 0x00 || rig.bus.mem[0xFEFF] != 0x20 {
		t.Fatalf("stack push incorrect: %02X %02X", rig.bus.mem[0xFEFE], rig.bus.mem[0xFEFF])
	}
	if rig.cpu.IFF1 {
		t.Fatalf("NMI should clear IFF1")
	}
	if !rig.cpu.IFF2 {
		t.Fatalf("NMI should preserve IFF2")
	}
	if rig.cpu.Cycles != 11 {
		t.Fatalf("Cycles = %d, want 11", rig.cpu.Cycles)
	}
}

func TestZ80IM2InterruptVector(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.cpu.PC = 0x3000
	rig.cpu.SP = 0xFF00
	rig.cpu.IM = 2
	rig.cpu.I = 0x12
	rig.cpu.SetIRQVector(0x34)
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	rig.bus.mem[0x1234] = 0x78
	rig.bus.mem[0x1235] = 0x56
	rig.cpu.SetIRQLine(true)

	rig.cpu.Step()

	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x5678)
	if rig.cpu.SP != 0xFEFE {
		t.Fatalf("SP = 0x%04X, want 0xFEFE", rig.cpu.SP)
	}
	if rig.cpu.WZ != 0x1235 {
		t.Fatalf("WZ = 0x%04X, want 0x1235", rig.cpu.WZ)
	}
}

func TestZ80IM0RSTVector(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.cpu.PC = 0x4000
	rig.cpu.SP = 0xFF00
	rig.cpu.IM = 0
	rig.cpu.SetIRQVector(0xC7)
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	rig.cpu.SetIRQLine(true)

	rig.cpu.Step()

	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0000)
}

func TestZ80HALTInterruptExit(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.cpu.PC = 0x5000
	rig.cpu.SP = 0xFF00
	rig.cpu.IM = 1
	rig.cpu.IFF1 = true
	rig.cpu.IFF2 = true
	rig.cpu.Halted = true
	rig.cpu.SetIRQLine(true)

	rig.cpu.Step()

	if rig.cpu.Halted {
		t.Fatalf("HALT should exit on interrupt")
	}
	requireZ80EqualU16(t, "PC", rig.cpu.PC, 0x0038)
}
