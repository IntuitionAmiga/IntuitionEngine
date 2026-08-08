//go:build js && wasm

package main

// wasm8BitPollPark is replaceable by tests so they can prove a matched poll
// gives the browser event loop a turn between unchanged device reads.
var wasm8BitPollPark = wasmPollFramePark

// wasmRun6502MMIOPollLoop recognises LDA abs, AND immediate, BEQ/BNE back to
// the load. It preserves the canonical 6502 read, flag and cycle behaviour
// while parking between iterations so a browser-side device can advance.
func (cpu *CPU_6502) wasmRun6502MMIOPollLoop(adapter *Bus6502Adapter) (bool, uint32) {
	if cpu == nil || adapter == nil || cpu.fastAdapter == nil {
		return false, 0
	}
	pc := cpu.PC
	mem := cpu.fastAdapter.memDirect
	if int(pc)+7 > len(mem) || mem[pc] != 0xAD || mem[pc+3] != 0x29 {
		return false, 0
	}
	jcc := mem[pc+5]
	if jcc != 0xF0 && jcc != 0xD0 || int32(uint32(pc)+7)+int32(int8(mem[pc+6])) != int32(pc) {
		return false, 0
	}
	addr := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
	if !wasm6502AddressIsMMIO(adapter, addr) {
		return false, 0
	}
	mask := mem[pc+4]
	iterations := uint32(0)
	for iterations < wasmPollIterationCap && cpu.running.Load() && !cpu.nmiPending.Load() && !(cpu.irqPending.Load() && cpu.SR&INTERRUPT_FLAG == 0) {
		value := adapter.Read(addr)
		cpu.A = value & mask
		cpu.SR = (cpu.SR &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[cpu.A]
		iterations++
		retired := iterations * 3
		branchTaken := cpu.A == 0
		if jcc == 0xD0 {
			branchTaken = !branchTaken
		}
		// LDA abs and AND immediate cost six cycles. The branch costs two
		// cycles, plus one when taken and one more when it crosses a page.
		cycleCost := uint64(8)
		if branchTaken {
			cycleCost++
			if pc&0xFF00 != (pc+7)&0xFF00 {
				cycleCost++
			}
		}
		cpu.Cycles += cycleCost
		if !branchTaken {
			cpu.PC = pc + 7
			return true, retired
		}
		if iterations < wasmPollIterationCap {
			wasm8BitPollPark()
		}
	}
	cpu.PC = pc
	return true, iterations * 3
}

// wasmRunZ80MMIOPollLoop recognises LD A,(nn), AND immediate, JR Z/NZ back to
// the load. It uses the adapter for every device read and reports the same R
// increments and cycle accounting as the native fast-poll path.
func (cpu *CPU_Z80) wasmRunZ80MMIOPollLoop(adapter *Z80BusAdapter) (bool, uint32, uint32) {
	if cpu == nil || adapter == nil || adapter.bus == nil {
		return false, 0, 0
	}
	pc := cpu.PC
	mem := adapter.bus.GetMemory()
	if int(pc)+7 > len(mem) || mem[pc] != 0x3A || mem[pc+3] != 0xE6 {
		return false, 0, 0
	}
	jcc := mem[pc+5]
	if jcc != 0x28 && jcc != 0x20 || int32(uint32(pc)+7)+int32(int8(mem[pc+6])) != int32(pc) {
		return false, 0, 0
	}
	addr := uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
	if !adapter.bus.IsIOAddress(translateIO8Bit(addr)) {
		return false, 0, 0
	}
	mask := mem[pc+4]
	iterations := uint32(0)
	cycles := 0
	for iterations < wasmPollIterationCap && cpu.running.Load() && !cpu.nmiPending.Load() && !(cpu.irqLine.Load() && cpu.IFF1) && !cpu.Halted && cpu.iffDelay == 0 {
		cpu.A = adapter.Read(addr)
		// andA reproduces AND n, including clearing carry and setting H, S,
		// Z, parity, and the undocumented X and Y flags.
		cpu.andA(mask)
		iterations++
		retired := iterations * 3
		branchTaken := cpu.A == 0
		if jcc == 0x20 {
			branchTaken = !branchTaken
		}
		// LD A,(nn), AND n, and a falling-through JR cost 27 cycles. A taken
		// JR costs five additional cycles.
		cycleCost := 27
		if branchTaken {
			cycleCost += 5
		}
		cycles += cycleCost
		if !branchTaken {
			cpu.PC = pc + 7
			cpu.tick(cycles)
			return true, retired, iterations
		}
		if iterations < wasmPollIterationCap {
			wasm8BitPollPark()
		}
	}
	cpu.PC = pc
	cpu.tick(cycles)
	return true, iterations * 3, iterations
}

func wasm6502AddressIsMMIO(adapter *Bus6502Adapter, addr uint16) bool {
	if adapter == nil {
		return false
	}
	bus, ok := adapter.bus.(*MachineBus)
	return ok && bus.IsIOAddress(translateIO8Bit_6502(addr))
}
