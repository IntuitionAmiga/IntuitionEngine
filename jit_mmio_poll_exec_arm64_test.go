//go:build arm64 && linux

package main

import "testing"

func newARM64IE64PollCPU(t *testing.T, read func(uint32) uint32) *CPU64 {
	t.Helper()
	bus := NewMachineBus()
	bus.MapIO(0xF0008, 0xF0008, read, nil)
	cpu := NewCPU64(bus)
	cpu.PC = PROG_START
	cpu.regs[1] = 0xF0008
	cpu.running.Store(true)
	return cpu
}

func TestARM64_IE64FastMMIOPollCountdownAccounting(t *testing.T) {
	reads := 0
	cpu := newARM64IE64PollCPU(t, func(uint32) uint32 {
		reads++
		if reads < 4 {
			return 0
		}
		return 2
	})
	cpu.regs[3] = 5
	copy(cpu.memory[PROG_START:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_AND64, 2, IE64_SIZE_L, 1, 2, 0, 2))
	copy(cpu.memory[PROG_START+16:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 0, 24))
	copy(cpu.memory[PROG_START+24:], ie64Instr(OP_SUB, 3, IE64_SIZE_Q, 1, 3, 0, 1))
	copy(cpu.memory[PROG_START+32:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 3, 0, 0xFFFFFFE0))

	matched, retired := cpu.tryFastIE64MMIOPollLoop()
	if !matched || reads != 4 || cpu.PC != PROG_START+40 || retired != 18 {
		t.Fatalf("matched=%v reads=%d PC=0x%X retired=%d, want true, 4, 0x%X, 18", matched, reads, cpu.PC, retired, PROG_START+40)
	}
}

func TestARM64_IE64FastMMIOPollEqualityAccounting(t *testing.T) {
	reads := 0
	cpu := newARM64IE64PollCPU(t, func(uint32) uint32 {
		reads++
		if reads < 3 {
			return 7
		}
		return 9
	})
	cpu.regs[4] = 5
	copy(cpu.memory[PROG_START:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 7))
	copy(cpu.memory[PROG_START+16:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 3, 24))
	copy(cpu.memory[PROG_START+24:], ie64Instr(OP_SUB, 4, IE64_SIZE_Q, 1, 4, 0, 1))
	copy(cpu.memory[PROG_START+32:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 4, 0, 0xFFFFFFE0))

	matched, retired := cpu.tryFastIE64MMIOPollLoop()
	if !matched || reads != 3 || cpu.PC != PROG_START+40 || retired != 13 {
		t.Fatalf("matched=%v reads=%d PC=0x%X retired=%d, want true, 3, 0x%X, 13", matched, reads, cpu.PC, retired, PROG_START+40)
	}
}

func TestARM64_IE64FastMMIOPollYieldsForPendingIRQ(t *testing.T) {
	var cpu *CPU64
	cpu = newARM64IE64PollCPU(t, func(uint32) uint32 {
		NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
		return 0x80
	})
	cpu.interruptEnabled.Store(true)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_AND64, 2, IE64_SIZE_L, 1, 2, 0, 0x80))
	copy(cpu.memory[PROG_START+16:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 0, 0xFFFFFFF0))

	matched, retired := cpu.tryFastIE64MMIOPollLoop()
	if !matched || cpu.pendingIRQMask.Load() == 0 || cpu.PC != PROG_START || retired != 3 {
		t.Fatalf("matched=%v pending=%d PC=0x%X retired=%d, want true, pending, 0x%X, 3", matched, cpu.pendingIRQMask.Load(), cpu.PC, retired, PROG_START)
	}
}
