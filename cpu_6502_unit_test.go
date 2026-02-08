package main

import "testing"

func Test6502LDAImmediate(t *testing.T) {
	rig := newCPU6502TestRig()
	rig.resetAndLoad(0x0200, []byte{
		0xA9, 0x42, // LDA #$42
		0xEA, // NOP
	})

	runSingleInstruction(t, rig.cpu, 0x0200)

	if rig.cpu.A != 0x42 {
		t.Fatalf("A=0x%02X, want 0x42", rig.cpu.A)
	}
	if rig.cpu.getFlag(ZERO_FLAG) {
		t.Fatalf("ZERO flag set unexpectedly")
	}
	if rig.cpu.getFlag(NEGATIVE_FLAG) {
		t.Fatalf("NEGATIVE flag set unexpectedly")
	}
}

func Test6502STAZeroPage(t *testing.T) {
	rig := newCPU6502TestRig()
	rig.resetAndLoad(0x0200, []byte{
		0xA9, 0x55, // LDA #$55
		0x85, 0x10, // STA $10
		0xEA, // NOP
	})

	runSingleInstruction(t, rig.cpu, 0x0200)
	runSingleInstruction(t, rig.cpu, 0x0202)

	if got := rig.bus.Read8(0x0010); got != 0x55 {
		t.Fatalf("memory[0x0010]=0x%02X, want 0x55", got)
	}
}

func Test6502VRAMBankRegister(t *testing.T) {
	rig := newCPU6502TestRig()
	rig.resetAndLoad(0x0200, []byte{
		0xA9, 0x02, // LDA #$02
		0x8D, 0xF0, 0xF7, // STA $F7F0
		0xA9, 0xAB, // LDA #$AB
		0x8D, 0x00, 0x80, // STA $8000
		0xEA, // NOP
	})

	runSingleInstruction(t, rig.cpu, 0x0200)
	runSingleInstruction(t, rig.cpu, 0x0202)
	runSingleInstruction(t, rig.cpu, 0x0205)
	runSingleInstruction(t, rig.cpu, 0x0207)

	adapter := rig.cpu.memory.(*Bus6502Adapter)
	if adapter.vramBank != 0x02 {
		t.Fatalf("vramBank=%d, want 2", adapter.vramBank)
	}

	expectedAddr := uint32(VRAM_START) + (2 * VRAM_BANK_WINDOW_SIZE)
	if got := rig.bus.Read8(expectedAddr); got != 0xAB {
		t.Fatalf("VRAM[0x%X]=0x%02X, want 0xAB", expectedAddr, got)
	}
}
