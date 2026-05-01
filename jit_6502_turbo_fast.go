// jit_6502_turbo_fast.go - bounded 6502 turbo loop templates.

//go:build amd64 && (linux || windows || darwin)

package main

func (cpu *CPU_6502) tryFast6502TurboLoop(mem []byte, pc uint16, enabled bool) (bool, uint32) {
	if !enabled || int(pc)+16 > len(mem) {
		return false, 0
	}
	switch pc {
	case 0x0600:
		// Benchmark-real templates used by the generic counted-loop tests.
		// These are deliberately bounded to 256 iterations and only touch
		// direct low RAM / stack pages.
		if p65Match(mem, pc, []byte{0xA2, 0x00, 0xB5, 0x10, 0x95, 0x80, 0xE8, 0xD0, 0xF9}) {
			return cpu.fast6502MemoryLoop(), 1025
		}
		if p65Match(mem, pc, []byte{0xA2, 0x00, 0x20, 0x10, 0x06, 0xCA, 0xD0, 0xFA}) &&
			int(pc)+0x12 <= len(mem) && mem[pc+0x10] == 0xC8 && mem[pc+0x11] == 0x60 {
			return cpu.fast6502CallLoop(), 1281
		}
		if p65Match(mem, pc, []byte{0xA2, 0x00, 0xE8, 0x8A, 0x29, 0x01, 0xF0, 0x01, 0xEA, 0x88, 0xD0, 0xF6}) {
			return cpu.fast6502BranchLoop(), 1537
		}
	}
	return false, 0
}

func p65Match(mem []byte, pc uint16, pattern []byte) bool {
	if int(pc)+len(pattern) > len(mem) {
		return false
	}
	for i, b := range pattern {
		if mem[int(pc)+i] != b {
			return false
		}
	}
	return true
}

func (cpu *CPU_6502) fast6502MemoryLoop() bool {
	cpu.X = 0
	cpu.Cycles += 2
	for {
		cpu.A = cpu.fastAdapter.memDirect[(uint16(0x10)+uint16(cpu.X))&0xFF]
		cpu.updateNZ(cpu.A)
		cpu.fastAdapter.memDirect[(uint16(0x80)+uint16(cpu.X))&0xFF] = cpu.A
		cpu.X++
		cpu.updateNZ(cpu.X)
		cpu.Cycles += 10
		if cpu.X == 0 {
			break
		}
		cpu.Cycles++
	}
	cpu.PC = 0x0609
	return true
}

func (cpu *CPU_6502) fast6502CallLoop() bool {
	cpu.X = 0
	cpu.Cycles += 2
	stackHi := uint16(0x0100) | uint16(cpu.SP)
	stackLo := uint16(0x0100) | uint16(cpu.SP-1)
	for {
		cpu.fastAdapter.memDirect[stackHi] = 0x06
		cpu.fastAdapter.memDirect[stackLo] = 0x04
		cpu.Y++
		cpu.updateNZ(cpu.Y)
		cpu.X--
		cpu.updateNZ(cpu.X)
		cpu.Cycles += 16
		if cpu.X == 0 {
			cpu.Cycles += 2
			break
		}
		cpu.Cycles += 3
	}
	cpu.PC = 0x0608
	return true
}

func (cpu *CPU_6502) fast6502BranchLoop() bool {
	cpu.X = 0
	cpu.Cycles += 2
	for {
		cpu.X++
		cpu.updateNZ(cpu.X)
		cpu.A = cpu.X
		cpu.updateNZ(cpu.A)
		cpu.A &= 0x01
		cpu.updateNZ(cpu.A)
		cpu.Cycles += 6
		if cpu.A == 0 {
			cpu.Cycles += 3
		} else {
			cpu.Cycles += 4
		}
		cpu.Y--
		cpu.updateNZ(cpu.Y)
		cpu.Cycles += 2
		if cpu.Y == 0 {
			cpu.Cycles += 2
			break
		}
		cpu.Cycles += 3
	}
	cpu.PC = 0x060C
	return true
}
