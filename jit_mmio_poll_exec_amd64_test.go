//go:build amd64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

func TestIE64JITFastMMIOPollLoop_AND_BNE(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 3 {
			return 0x80
		}
		return 0
	}, nil)
	cpu := NewCPU64(bus)
	cpu.PC = PROG_START
	cpu.regs[1] = 0xF0008
	cpu.running.Store(true)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_AND64, 2, IE64_SIZE_L, 1, 2, 0, 0x80))
	copy(cpu.memory[PROG_START+16:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 0, 0xFFFFFFF0))

	matched, retired := cpu.tryFastIE64MMIOPollLoop()
	if !matched {
		t.Fatal("expected IE64 MMIO poll loop to match")
	}
	if cpu.PC != PROG_START+24 {
		t.Fatalf("PC = 0x%08X, want 0x%08X", cpu.PC, uint64(PROG_START+24))
	}
	if reads != 3 {
		t.Fatalf("reads = %d, want 3", reads)
	}
	if retired != 9 {
		t.Fatalf("retired = %d, want 9", retired)
	}
}

func TestIE64JITFastMMIOPollLoopRejectsRegisterAND(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 { return 0x80 }, nil)
	cpu := NewCPU64(bus)
	cpu.PC = PROG_START
	cpu.regs[1] = 0xF0008
	cpu.running.Store(true)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_AND64, 2, IE64_SIZE_L, 0, 2, 3, 0))
	copy(cpu.memory[PROG_START+16:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 0, 0xFFFFFFF0))

	matched, _ := cpu.tryFastIE64MMIOPollLoop()
	if matched {
		t.Fatal("register-form IE64 AND must not match immediate-mask MMIO poll fast path")
	}
}

func TestM68KJITFastMMIOPollLoop_TST_BNE(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 4 {
			return 0x80
		}
		return 0
	}, nil)
	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.running.Store(true)
	mem := bus.GetMemory()
	binary.BigEndian.PutUint16(mem[0x1000:], 0x1039)     // MOVE.B abs.l,D0
	binary.BigEndian.PutUint32(mem[0x1002:], 0x000F0008) // VIDEO_STATUS
	binary.BigEndian.PutUint16(mem[0x1006:], 0x4A00)     // TST.B D0
	binary.BigEndian.PutUint16(mem[0x1008:], 0x66F6)     // BNE $1000

	matched, retired := cpu.tryFastM68KMMIOPollLoop()
	if !matched {
		t.Fatal("expected M68K MMIO poll loop to match")
	}
	if cpu.PC != 0x100A {
		t.Fatalf("PC = 0x%08X, want 0x0000100A", cpu.PC)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want 4", reads)
	}
	if retired != 12 {
		t.Fatalf("retired = %d, want 12", retired)
	}
}

func TestM68KJITFastMMIOPollLoopPreservesUpperBitsForByteAndWord(t *testing.T) {
	for _, tc := range []struct {
		name     string
		moveOp   uint16
		tstOp    uint16
		read     uint32
		initial  uint32
		expected uint32
	}{
		{name: "byte", moveOp: 0x1039, tstOp: 0x4A00, read: 0x34, initial: 0xAABBCCDD, expected: 0xAABBCC34},
		{name: "word", moveOp: 0x3039, tstOp: 0x4A40, read: 0x3456, initial: 0xAABBCCDD, expected: 0xAABB3456},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 { return tc.read }, nil)
			cpu := NewM68KCPU(bus)
			cpu.PC = 0x1000
			cpu.DataRegs[0] = tc.initial
			cpu.running.Store(true)
			mem := bus.GetMemory()
			binary.BigEndian.PutUint16(mem[0x1000:], tc.moveOp)
			binary.BigEndian.PutUint32(mem[0x1002:], 0x000F0008)
			binary.BigEndian.PutUint16(mem[0x1006:], tc.tstOp)
			binary.BigEndian.PutUint16(mem[0x1008:], 0x67F6) // BEQ $1000, not taken for non-zero read

			matched, _ := cpu.tryFastM68KMMIOPollLoop()
			if !matched {
				t.Fatal("expected M68K MMIO poll loop to match")
			}
			if cpu.DataRegs[0] != tc.expected {
				t.Fatalf("D0 = 0x%08X, want 0x%08X", cpu.DataRegs[0], tc.expected)
			}
		})
	}
}

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

func Test6502JITFastMMIOPollLoopStoresMaskedAccumulator(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 { return 0x81 }, nil)
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
		0xF0, 0xF9, // BEQ $0600, not taken
	})

	matched, _ := cpu.tryFast6502MMIOPollLoop(adapter)
	if !matched {
		t.Fatal("expected 6502 MMIO poll loop to match")
	}
	if cpu.A != 0x80 {
		t.Fatalf("A = 0x%02X, want masked value 0x80", cpu.A)
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
