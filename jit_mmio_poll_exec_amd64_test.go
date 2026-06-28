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

// TestIE64JITFastMMIOPollLoop_ExitsOnPendingIRQ: a guest spinning on an MMIO
// status flag must yield to the dispatcher when an external interrupt is
// recorded. External IRQs now set pendingIRQMask only (they no longer flip
// inInterrupt), so the poll loop must watch the pending mask. The MMIO read
// callback records the interrupt on the first read; the loop must then exit with
// cpu.PC left at the loop head so the dispatcher can deliver and resume there.
func TestIE64JITFastMMIOPollLoop_ExitsOnPendingIRQ(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	sink := NewIE64InterruptSink(cpu)
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		// Record an external interrupt during the poll; value stays nonzero so
		// the loop would otherwise continue spinning.
		sink.Pulse(IntMaskBlitter)
		return 0x80
	}, nil)
	cpu.PC = PROG_START
	cpu.regs[1] = 0xF0008
	cpu.interruptEnabled.Store(true)
	cpu.running.Store(true)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_AND64, 2, IE64_SIZE_L, 1, 2, 0, 0x80))
	copy(cpu.memory[PROG_START+16:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 0, 0xFFFFFFF0))

	matched, retired := cpu.tryFastIE64MMIOPollLoop()
	if !matched {
		t.Fatal("expected IE64 MMIO poll loop to match")
	}
	if cpu.pendingIRQMask.Load() == 0 {
		t.Fatal("expected the MMIO read to have recorded a pending IRQ")
	}
	if cpu.PC != PROG_START {
		t.Fatalf("PC = 0x%08X, want loop head 0x%08X so the dispatcher can deliver", cpu.PC, uint64(PROG_START))
	}
	if retired != 3 {
		t.Fatalf("retired = %d, want 3 (one iteration before the pending-IRQ exit)", retired)
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

func TestM68KJITFastMMIOPollLoop_DeclinesWhenInterruptPending(t *testing.T) {
	// A pending interrupt must make the fast path decline (matched=false) so the
	// dispatcher runs a real block, advances the instruction count, and delivers
	// the interrupt. Matching would execute zero iterations, return retired=0,
	// leave PC at the loop head, and livelock (checkPending only delivers on a
	// 256-instruction boundary the static count never crosses).
	bus := NewMachineBus()
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 { return 0x80 }, nil)
	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.running.Store(true)
	mem := bus.GetMemory()
	binary.BigEndian.PutUint16(mem[0x1000:], 0x1039)     // MOVE.B abs.l,D0
	binary.BigEndian.PutUint32(mem[0x1002:], 0x000F0008) // VIDEO_STATUS
	binary.BigEndian.PutUint16(mem[0x1006:], 0x4A00)     // TST.B D0
	binary.BigEndian.PutUint16(mem[0x1008:], 0x66F6)     // BNE $1000

	cpu.pendingInterrupt.Store(1 << 6)
	if matched, retired := cpu.tryFastM68KMMIOPollLoop(); matched || retired != 0 {
		t.Fatalf("interrupt pending: matched=%v retired=%d, want false/0", matched, retired)
	}
	if cpu.PC != 0x1000 {
		t.Fatalf("PC = 0x%08X, want 0x00001000 (loop head untouched)", cpu.PC)
	}

	// With the interrupt cleared the same loop matches again.
	cpu.pendingInterrupt.Store(0)
	if matched, _ := cpu.tryFastM68KMMIOPollLoop(); !matched {
		t.Fatal("expected match once interrupt cleared")
	}
}

func TestM68KJITFastMMIOPollLoop_BTST_BNE(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 4 {
			return 0
		}
		return 0x2
	}, nil)
	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.running.Store(true)
	mem := bus.GetMemory()
	binary.BigEndian.PutUint16(mem[0x1000:], 0x2239)     // MOVE.L abs.l,D1
	binary.BigEndian.PutUint32(mem[0x1002:], 0x000F0008) // VIDEO_STATUS
	binary.BigEndian.PutUint16(mem[0x1006:], 0x0801)     // BTST #1,D1
	binary.BigEndian.PutUint16(mem[0x1008:], 0x0001)
	binary.BigEndian.PutUint16(mem[0x100A:], 0x67F4) // BEQ $1000

	matched, retired := cpu.tryFastM68KMMIOPollLoop()
	if !matched {
		t.Fatal("expected M68K BTST MMIO poll loop to match")
	}
	if cpu.PC != 0x100C {
		t.Fatalf("PC = 0x%08X, want 0x0000100C", cpu.PC)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want 4", reads)
	}
	if retired != 12 {
		t.Fatalf("retired = %d, want 12", retired)
	}
	if cpu.SR&M68K_SR_Z != 0 {
		t.Fatalf("Z flag set after bit became set: SR=0x%04X", cpu.SR)
	}
}

func TestM68KJITFastMMIOPollLoop_BTSTCountdownFunction(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 4 {
			return 0
		}
		return 0x2
	}, nil)
	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.running.Store(true)
	mem := bus.GetMemory()
	binary.BigEndian.PutUint16(mem[0x1000:], 0x0801) // BTST #1,D1
	binary.BigEndian.PutUint16(mem[0x1002:], 0x0001)
	binary.BigEndian.PutUint16(mem[0x1004:], 0x6704) // BEQ second phase
	binary.BigEndian.PutUint16(mem[0x1006:], 0x5380) // SUBQ.L #1,D0
	binary.BigEndian.PutUint16(mem[0x1008:], 0x66F0) // BNE previous poll
	binary.BigEndian.PutUint16(mem[0x100A:], 0x203C) // MOVE.L #timeout,D0
	binary.BigEndian.PutUint32(mem[0x100C:], 0x000F4240)
	binary.BigEndian.PutUint16(mem[0x1010:], 0x2239) // MOVE.L abs.l,D1
	binary.BigEndian.PutUint32(mem[0x1012:], 0x000F0008)
	binary.BigEndian.PutUint16(mem[0x1016:], 0x0801) // BTST #1,D1
	binary.BigEndian.PutUint16(mem[0x1018:], 0x0001)
	binary.BigEndian.PutUint16(mem[0x101A:], 0x6604) // BNE RTS
	binary.BigEndian.PutUint16(mem[0x101C:], 0x5380) // SUBQ.L #1,D0
	binary.BigEndian.PutUint16(mem[0x101E:], 0x66F0) // BNE second phase load
	binary.BigEndian.PutUint16(mem[0x1020:], 0x4E75) // RTS

	matched, retired := cpu.tryFastM68KMMIOPollLoop()
	if !matched {
		t.Fatal("expected countdown BTST MMIO poll loop to match")
	}
	if cpu.PC != 0x1020 {
		t.Fatalf("PC = 0x%08X, want 0x00001020", cpu.PC)
	}
	if reads != 4 {
		t.Fatalf("reads = %d, want 4", reads)
	}
	if cpu.DataRegs[1] != 0x2 {
		t.Fatalf("D1 = 0x%08X, want 0x00000002", cpu.DataRegs[1])
	}
	if retired == 0 {
		t.Fatal("retired instruction count was zero")
	}
}

func TestM68KJITFastMMIOPollLoop_FullBTSTCountdownFunction(t *testing.T) {
	bus := NewMachineBus()
	reads := 0
	bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
		reads++
		if reads < 5 {
			return 0
		}
		return 0x2
	}, nil)
	cpu := NewM68KCPU(bus)
	cpu.PC = 0x1000
	cpu.DataRegs[0] = 3
	cpu.running.Store(true)
	mem := bus.GetMemory()
	binary.BigEndian.PutUint16(mem[0x1000:], 0x2239) // MOVE.L abs.l,D1
	binary.BigEndian.PutUint32(mem[0x1002:], 0x000F0008)
	binary.BigEndian.PutUint16(mem[0x1006:], 0x0801) // BTST #1,D1
	binary.BigEndian.PutUint16(mem[0x1008:], 0x0001)
	binary.BigEndian.PutUint16(mem[0x100A:], 0x6704) // BEQ second phase
	binary.BigEndian.PutUint16(mem[0x100C:], 0x5380) // SUBQ.L #1,D0
	binary.BigEndian.PutUint16(mem[0x100E:], 0x66F0) // BNE first phase load
	binary.BigEndian.PutUint16(mem[0x1010:], 0x203C) // MOVE.L #timeout,D0
	binary.BigEndian.PutUint32(mem[0x1012:], 0x000F4240)
	binary.BigEndian.PutUint16(mem[0x1016:], 0x2239) // MOVE.L abs.l,D1
	binary.BigEndian.PutUint32(mem[0x1018:], 0x000F0008)
	binary.BigEndian.PutUint16(mem[0x101C:], 0x0801) // BTST #1,D1
	binary.BigEndian.PutUint16(mem[0x101E:], 0x0001)
	binary.BigEndian.PutUint16(mem[0x1020:], 0x6604) // BNE RTS
	binary.BigEndian.PutUint16(mem[0x1022:], 0x5380) // SUBQ.L #1,D0
	binary.BigEndian.PutUint16(mem[0x1024:], 0x66F0) // BNE second phase load
	binary.BigEndian.PutUint16(mem[0x1026:], 0x4E75) // RTS

	matched, retired := cpu.tryFastM68KMMIOPollLoop()
	if !matched {
		t.Fatal("expected full countdown BTST MMIO poll loop to match")
	}
	if cpu.PC != 0x1026 {
		t.Fatalf("PC = 0x%08X, want 0x00001026", cpu.PC)
	}
	if reads != 5 {
		t.Fatalf("reads = %d, want 5", reads)
	}
	if cpu.DataRegs[1] != 0x2 {
		t.Fatalf("D1 = 0x%08X, want 0x00000002", cpu.DataRegs[1])
	}
	if retired == 0 {
		t.Fatal("retired instruction count was zero")
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

func TestM68KPollFastPath_LoadSizes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		moveOp uint16
		tstOp  uint16
	}{
		{name: "word", moveOp: 0x3039, tstOp: 0x4A40},
		{name: "byte", moveOp: 0x1039, tstOp: 0x4A00},
		{name: "long", moveOp: 0x2039, tstOp: 0x4A80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			reads := 0
			bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
				reads++
				return 0
			}, nil)
			cpu := NewM68KCPU(bus)
			cpu.PC = 0x1000
			cpu.running.Store(true)
			mem := bus.GetMemory()
			binary.BigEndian.PutUint16(mem[0x1000:], tc.moveOp)
			binary.BigEndian.PutUint32(mem[0x1002:], 0x000F0008)
			binary.BigEndian.PutUint16(mem[0x1006:], tc.tstOp)
			binary.BigEndian.PutUint16(mem[0x1008:], 0x66F6)

			matched, _ := cpu.tryFastM68KMMIOPollLoop()
			if !matched {
				t.Fatal("expected M68K MMIO poll loop to match")
			}
			if reads != 1 {
				t.Fatalf("reads = %d, want 1", reads)
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

// m68kPollProgram writes a MOVE.L abs.l,D1 ; <test> ; BEQ-back poll loop and
// returns it. test==0x0801 0001 selects BTST #1,D1 (4-byte), else TST.L D1.
func m68kBuildPollProgram(mem []byte, base uint32, addr uint32, useBTST bool) (endPC uint32) {
	binary.BigEndian.PutUint16(mem[base:], 0x2239) // MOVE.L abs.l,D1
	binary.BigEndian.PutUint32(mem[base+2:], addr)
	if useBTST {
		binary.BigEndian.PutUint16(mem[base+6:], 0x0801) // BTST #1,D1
		binary.BigEndian.PutUint16(mem[base+8:], 0x0001)
		binary.BigEndian.PutUint16(mem[base+10:], 0x67F4) // BEQ back to base
		return base + 12
	}
	binary.BigEndian.PutUint16(mem[base+6:], 0x4A81) // TST.L D1
	binary.BigEndian.PutUint16(mem[base+8:], 0x66F6) // BNE back to base
	return base + 10
}

// TestM68KFastMMIOPoll_FullCCRMatchesInterpreter asserts the fast poll loop
// leaves the COMPLETE CCR (N/Z/V/C/X) identical to the interpreter, including
// the MOVE's "clear V/C, set N" effect that BTST/TST do not touch (regression
// guard for the stale-N/V/C fast-path bug).
func TestM68KFastMMIOPoll_FullCCRMatchesInterpreter(t *testing.T) {
	for _, useBTST := range []bool{true, false} {
		// Status value 0x80000002: bit1 set (poll exits) and bit31 set (N must be
		// set by MOVE.L). Pre-dirty V, C, X so a stale CCR would be detected.
		const status = uint32(0x80000002)
		newCPU := func() (*M68KCPU, *MachineBus) {
			bus := NewMachineBus()
			reads := 0
			bus.MapIO(0xF0008, 0xF0008, func(addr uint32) uint32 {
				reads++
				if reads < 3 {
					return 0
				}
				return status
			}, nil)
			cpu := NewM68KCPU(bus)
			cpu.PC = 0x1000
			cpu.SR = M68K_SR_S | M68K_SR_V | M68K_SR_C | M68K_SR_X
			cpu.running.Store(true)
			m68kBuildPollProgram(bus.GetMemory(), 0x1000, 0x000F0008, useBTST)
			return cpu, bus
		}

		fast, _ := newCPU()
		matched, _ := fast.tryFastM68KMMIOPollLoop()
		if !matched {
			t.Fatalf("useBTST=%v: fast poll loop did not match", useBTST)
		}

		interp, _ := newCPU()
		for i := 0; i < 64 && interp.PC != fast.PC; i++ {
			interp.StepOne()
		}
		if interp.PC != fast.PC {
			t.Fatalf("useBTST=%v: interp PC=0x%08X never reached fast PC=0x%08X", useBTST, interp.PC, fast.PC)
		}
		if fast.SR != interp.SR {
			t.Fatalf("useBTST=%v: SR mismatch fast=0x%04X interp=0x%04X (N/V/C/X parity)", useBTST, fast.SR, interp.SR)
		}
		if fast.DataRegs[1] != interp.DataRegs[1] {
			t.Fatalf("useBTST=%v: D1 mismatch fast=0x%08X interp=0x%08X", useBTST, fast.DataRegs[1], interp.DataRegs[1])
		}
	}
}
