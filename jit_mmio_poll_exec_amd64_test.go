//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func Test6502JITFastMMIOPollLoop_AND_BNE(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 3 {
			return 0x80
		}
		return 0
	}, nil)
	cpu := NewCPU_6502(bus)
	adapter, ok := cpu.memory.(*Bus6502Adapter)
	if !ok {
		t.Fatal("6502 CPU did not install Bus6502Adapter")
	}
	cpu.PC = 0x0600
	cpu.running.Store(true)
	copy(cpu.fastAdapter.memDirect[0x0600:], []byte{
		0xAD, 0x08, 0xF0, // LDA $F008
		0x29, 0x80, // AND #$80
		0xD0, 0xF9, // BNE $0600
	})

	matched, retired := cpu.tryFast6502MMIOPollLoop(adapter)
	if !matched {
		t.Fatal("expected 6502 MMIO poll loop to match")
	}
	if cpu.PC != 0x0607 {
		t.Fatalf("PC = 0x%04X, want 0x0607", cpu.PC)
	}
	if reads != 3 {
		t.Fatalf("reads = %d, want 3", reads)
	}
	if retired != 9 {
		t.Fatalf("retired = %d, want 9", retired)
	}
}

func TestZ80JITFastMMIOPollLoop_AND_JRNZ(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 4 {
			return 0x80
		}
		return 0
	}, nil)
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.PC = 0x0600
	cpu.running.Store(true)
	bus.SealMappings()
	cpu.initDirectPageBitmapZ80(adapter)
	mem := bus.GetMemory()
	copy(mem[0x0600:], []byte{
		0x3A, 0x08, 0xF0, // LD A,($F008)
		0xE6, 0x80, // AND $80
		0x20, 0xF9, // JR NZ,$0600
	})

	matched, retired, rInc := cpu.tryFastZ80MMIOPollLoop(adapter)
	if !matched {
		t.Fatal("expected Z80 MMIO poll loop to match")
	}
	if cpu.PC != 0x0607 {
		t.Fatalf("PC = 0x%04X, want 0x0607", cpu.PC)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want 4", reads)
	}
	if retired != 12 {
		t.Fatalf("retired = %d, want 12", retired)
	}
	if rInc != 4 {
		t.Fatalf("rInc = %d, want 4", rInc)
	}
}

func TestZ80JITFastMMIOPollLoopRejectsRAM(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.PC = 0x0600
	cpu.running.Store(true)
	bus.SealMappings()
	cpu.initDirectPageBitmapZ80(adapter)
	copy(bus.GetMemory()[0x0600:], []byte{
		0x3A, 0x00, 0x10, // LD A,($1000)
		0xE6, 0x80, // AND $80
		0x20, 0xF9, // JR NZ,$0600
	})

	matched, _, _ := cpu.tryFastZ80MMIOPollLoop(adapter)
	if matched {
		t.Fatal("RAM poll loop must not match MMIO fast path")
	}
}
