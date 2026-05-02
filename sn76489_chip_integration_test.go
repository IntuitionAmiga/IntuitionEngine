package main

import "testing"

func newMappedSNBus(t *testing.T) (*MachineBus, *SN76489Chip) {
	t.Helper()
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	sn := NewSN76489Chip(sound)
	bus := NewMachineBus()
	bus.MapIO(SN_BASE, SN_END, sn.HandleRead, sn.HandleWrite)
	bus.MapIOByte(SN_BASE, SN_END, sn.HandleWrite8)
	return bus, sn
}

func TestSN_BusWrite8_ViaMachineBus(t *testing.T) {
	bus, sn := newMappedSNBus(t)
	bus.Write8(SN_PORT_WRITE, 0x90)

	if snAtten(sn, 0) != 0 {
		t.Fatalf("atten: got %d, want 0", snAtten(sn, 0))
	}
}

func TestSN_BusWrite32_ViaMachineBus(t *testing.T) {
	bus, sn := newMappedSNBus(t)
	bus.Write32(SN_PORT_WRITE, 0xDEADBE90)

	if snAtten(sn, 0) != 0 {
		t.Fatalf("atten: got %d, want low-byte latch", snAtten(sn, 0))
	}
}

func TestSN_Z80_PortWrite(t *testing.T) {
	bus, sn := newMappedSNBus(t)
	z80Bus := NewZ80BusAdapter(bus)
	z80Bus.Out(Z80_SN_PORT_DATA, 0x90)

	if snAtten(sn, 0) != 0 {
		t.Fatalf("atten: got %d, want 0", snAtten(sn, 0))
	}
}

func TestSN_Z80_PortStatus(t *testing.T) {
	bus, _ := newMappedSNBus(t)
	z80Bus := NewZ80BusAdapter(bus)

	if got := z80Bus.In(Z80_SN_PORT_STATUS); got&1 == 0 {
		t.Fatalf("status: got 0x%02X, want bit 0 set", got)
	}
}

func TestSN_Z80_PortReadLast(t *testing.T) {
	bus, _ := newMappedSNBus(t)
	z80Bus := NewZ80BusAdapter(bus)
	z80Bus.Out(Z80_SN_PORT_DATA, 0x9F)

	if got := z80Bus.In(Z80_SN_PORT_DATA); got != 0x9F {
		t.Fatalf("last byte: got 0x%02X, want 0x9F", got)
	}
}

func TestSN_IOViewEntry(t *testing.T) {
	dev := ioDevices["sn76489"]
	if dev == nil {
		t.Fatal("missing sn76489 IO view entry")
	}
	if len(dev.Registers) < 3 || dev.Registers[0].Addr != SN_PORT_WRITE {
		t.Fatalf("unexpected SN IO view registers: %+v", dev.Registers)
	}
}

func TestSN_IE64_GuestWrite(t *testing.T) {
	rig := newIE64TestRig()
	sn := mapSNOnBus(t, rig.bus)
	rig.cpu.regs[1] = 0x90
	rig.cpu.regs[2] = SN_PORT_WRITE

	rig.executeOne(ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0))

	if snAtten(sn, 0) != 0 {
		t.Fatalf("IE64 SN atten: got %d, want 0", snAtten(sn, 0))
	}
}

func TestSN_IE32_GuestWrite(t *testing.T) {
	rig := newIE32TestRig()
	sn := mapSNOnBus(t, rig.bus)
	rig.cpu.A = 0x90

	rig.executeOne(createInstruction(STORE, REG_A, ADDR_DIRECT, SN_PORT_WRITE))

	if snAtten(sn, 0) != 0 {
		t.Fatalf("IE32 SN atten: got %d, want 0", snAtten(sn, 0))
	}
}

func TestSN_M68K_GuestWrite(t *testing.T) {
	bus := NewMachineBus()
	sn := mapSNOnBus(t, bus)
	cpu := NewM68KCPU(bus)
	cpu.PC = M68K_ENTRY_POINT

	// move.b #$90,$000F0C30.l
	opcodes := []uint16{0x13FC, 0x0090, 0x000F, 0x0C30}
	for i, op := range opcodes {
		addr := uint32(M68K_ENTRY_POINT + i*2)
		cpu.memory[addr] = byte(op >> 8)
		cpu.memory[addr+1] = byte(op)
	}

	runM68KInstructions(cpu, 1)

	if snAtten(sn, 0) != 0 {
		t.Fatalf("M68K SN atten: got %d, want 0", snAtten(sn, 0))
	}
}

func TestSN_6502_GuestWrite(t *testing.T) {
	rig := newCPU6502TestRig()
	sn := mapSNOnBus(t, rig.bus)
	start := uint16(0x0200)
	rig.resetAndLoad(start, []byte{
		0xA9, 0x90, // LDA #$90
		0x8D, 0x30, 0xFC, // STA $FC30 -> SN_PORT_WRITE
	})

	runSingleInstruction(t, rig.cpu, start)
	runSingleInstruction(t, rig.cpu, start+2)

	if snAtten(sn, 0) != 0 {
		t.Fatalf("6502 SN atten: got %d, want 0", snAtten(sn, 0))
	}
}

func TestSN_X86_GuestWrite(t *testing.T) {
	bus := NewMachineBus()
	sn := mapSNOnBus(t, bus)
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	program := []byte{
		0xB0, 0x90, // MOV AL,0x90
		0xA2, 0x30, 0xFC, 0x00, 0x00, // MOV [0x0000FC30],AL -> SN_PORT_WRITE
	}
	for i, b := range program {
		bus.Write8(uint32(i), b)
	}
	cpu.EIP = 0

	cpu.Step()
	cpu.Step()

	if snAtten(sn, 0) != 0 {
		t.Fatalf("x86 SN atten: got %d, want 0", snAtten(sn, 0))
	}
}

func mapSNOnBus(t *testing.T, bus *MachineBus) *SN76489Chip {
	t.Helper()
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	sn := NewSN76489Chip(sound)
	bus.MapIO(SN_BASE, SN_END, sn.HandleRead, sn.HandleWrite)
	bus.MapIOByte(SN_BASE, SN_END, sn.HandleWrite8)
	return sn
}
