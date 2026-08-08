//go:build js && wasm

// jit_mmio_poll_exec_wasm.go - IE64 MMIO poll-loop parking for the js/wasm build.
//
// The native backends recognise a guest spinning on an MMIO status register
// (jit_mmio_poll_exec_amd64.go) and service the spin host-side; the OS
// scheduler keeps the device threads advancing underneath it. On js/wasm
// there is one cooperatively-scheduled thread, so the same spin starves the
// very machinery that would flip the polled bit: the compositor only ticks
// (and VBlank only edges) while the CPU goroutine is parked. A guest doing
// WAIT-VSYNC by polling VIDEO_STATUS therefore burnt its whole 16 ms slice
// per observable edge and demos crawled at single-digit frame rates.
//
// wasmRunMMIOPollLoop matches the IE64 three-instruction bit-test poll.
//
//	pc+0:  LOAD  rd, (rs)+imm     ; rd != 0, MMIO address
//	pc+8:  AND   rd, rd, #imm
//	pc+16: BEQ/BNE rd, R0, -16    ; back to pc
//
// and, instead of spinning, parks the goroutine until the browser has
// rendered a frame between re-reads. Each park is one compositor tick, so
// the poll observes every VBlank edge at display cadence with near-zero
// burnt CPU. Exit conditions mirror the native matcher: loop exit, pending
// interrupt, trap, stop, or the shared iteration cap.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "encoding/binary"

// wasmPollStreakThreshold is how many consecutive LOAD helper exits at one
// PC arm the matcher. Productive MMIO readers (input sampling, status
// checks between work) never accumulate a streak; only a genuine spin does.
const wasmPollStreakThreshold = 32

// wasmPollIterationCap mirrors DefaultPollIterationCap in the native
// recogniser file (native-tagged, so restated here): a bounded stay in the
// poll service before control returns to the dispatcher.
const wasmPollIterationCap = 4096

// ie64InstrFieldsWasm decodes the fixed 8-byte IE64 instruction at pc.
// Duplicate of the native recognisers' ie64InstrFields, which lives in a
// native-tagged file.
func ie64InstrFieldsWasm(mem []byte, pc uint64) (op, rd, size, xbit, rs, rt byte, imm uint32, ok bool) {
	if pc+IE64_INSTR_SIZE > uint64(len(mem)) {
		return 0, 0, 0, 0, 0, 0, 0, false
	}
	instr := mem[pc : pc+IE64_INSTR_SIZE]
	byte1 := instr[1]
	return instr[0], byte1 >> 3, (byte1 >> 1) & 0x03, byte1 & 1, instr[2] >> 3, instr[3] >> 3, binary.LittleEndian.Uint32(instr[4:]), true
}

// wasmRunMMIOPollLoop runs a matched poll loop with a frame park between
// iterations. Returns (matched, retired instruction count). On any exit it
// leaves PC either at the loop head (interrupt, trap, cap: the dispatcher
// re-runs the loop after servicing) or at the fall-through (loop done).
func (cpu *CPU64) wasmRunMMIOPollLoop(pc uint64) (bool, uint32) {
	if cpu.bus == nil || cpu.mmuEnabled || pc > uint64(len(cpu.memory)) {
		return false, 0
	}
	op0, rd0, size0, _, rs0, _, imm0, ok := ie64InstrFieldsWasm(cpu.memory, pc)
	if !ok || op0 != OP_LOAD || rd0 == 0 {
		return false, 0
	}
	addr := uint64(int64(cpu.regs[rs0]) + int64(int32(imm0)))
	op1, rd1, size1, xbit1, rs1, _, imm1, ok := ie64InstrFieldsWasm(cpu.memory, pc+IE64_INSTR_SIZE)
	if !ok || op1 != OP_AND64 || rd1 != rd0 || rs1 != rd0 || size1 != size0 || xbit1 != 1 {
		return false, 0
	}
	op2, _, _, _, rs2, rt2, imm2, ok := ie64InstrFieldsWasm(cpu.memory, pc+2*IE64_INSTR_SIZE)
	if !ok || (op2 != OP_BEQ && op2 != OP_BNE) || rs2 != rd0 || rt2 != 0 {
		return false, 0
	}
	if uint64(int64(pc+2*IE64_INSTR_SIZE)+int64(int32(imm2))) != pc {
		return false, 0
	}
	if addr > 0xFFFFFFFF || !cpu.bus.IsIOAddress(uint32(addr)) {
		return false, 0
	}

	iterations := uint32(0)
	for cpu.running.Load() && !cpu.inInterrupt.Load() && cpu.pendingIRQMask.Load() == 0 {
		value := maskToSize(cpu.loadMem(addr, size0), size0)
		if cpu.trapped {
			cpu.trapped = false
			cpu.PC = pc
			return true, iterations * 3
		}
		value = maskToSize(value&uint64(imm1), size0)
		cpu.regs[rd0] = value
		iterations++

		branchTaken := value == 0
		if op2 == OP_BNE {
			branchTaken = !branchTaken
		}
		if !branchTaken {
			cpu.PC = pc + 3*IE64_INSTR_SIZE
			return true, iterations * 3
		}
		if iterations >= wasmPollIterationCap {
			cpu.PC = pc
			return true, iterations * 3
		}
		wasmPollFramePark()
	}
	cpu.PC = pc
	return true, iterations * 3
}
