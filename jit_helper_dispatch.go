// jit_helper_dispatch.go - Phase 5 JITContext helper-exit dispatcher.
//
// Emitted native JIT code cannot safely call back into Go. When it
// encounters a memory, stack, or control-flow operation it cannot
// service locally (MMU translation required, high physical address
// outside the dense cpu.memory[] window, etc.) it writes a structured
// request to JITContext via the HELPER_* protocol and returns. This
// file owns the Go-side dispatch: read the request, perform the
// equivalent semantic operation through the interpreter helpers
// (cpu.loadMem / cpu.storeMem / cpu.mmuStackWriteU64 / mmuStackReadU64
// / FPU pair helpers), advance PC, propagate faults, and clear the
// request so the JIT loop can re-enter.
//
// The dispatcher is wired into ExecuteJIT immediately after every
// callNative. While Phase 5 is still in flight the AMD64 / ARM64
// emitters do not yet set NeedHelper; the dispatcher therefore is a
// no-op on the hot path. As each opcode emitter is rewritten to use
// the helper exit it goes live without any further dispatcher work.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"fmt"
	"math"
	"unsafe"
)

// haltStackFault mirrors the interpreter's non-trapping stack failure
// path: when mmuStackWriteU64 / mmuStackReadU64 returns false without
// setting cpu.trapped, the bus could not service the access (high
// address outside backing, sparse hole, etc.). The interpreter halts
// the CPU in that case (cpu_ie64.go:1943, 1958, 1975, 1988, 2009).
// Replicate that here so a handled helper does not loop forever via
// the suppressed I/O fallback. When cpu.trapped IS set, only clear it
// — trapFault has already redirected PC to the trap vector.
func (cpu *CPU64) haltStackFault() {
	if cpu.trapped {
		cpu.trapped = false
		return
	}
	cpu.running.Store(false)
}

// haltFPUFault mirrors the interpreter's fpu_missing / invalid_freg
// labels (cpu_ie64.go:2714-2722): print and halt. The JIT cannot
// route back to the interpreter from a handled helper because the
// caller suppresses the I/O fallback for handled helpers; without
// this, an MMU-on FP load/store against a missing FPU would re-enter
// the same block.
func (cpu *CPU64) haltFPUFault(rd byte) {
	if cpu.FPU == nil {
		fmt.Printf("IE64: FPU instruction executed but FPU is missing at PC=0x%X\n", cpu.PC)
	} else {
		fmt.Printf("IE64: Invalid FP register index %d at PC=0x%X\n", rd, cpu.PC)
	}
	cpu.running.Store(false)
}

// handleJITHelper inspects ctx.NeedHelper and, if non-zero, performs
// the requested semantic operation, advances PC, and reports how many
// instructions retired (0 or 1). Returns:
//
//   - retired: 0 if no helper request OR the helper faulted (the
//     instruction will re-execute after trap return); 1 if the helper
//     completed cleanly. Caller adds this to InstructionCount.
//   - handled: true if a helper request was serviced (regardless of
//     fault). Caller uses this to decide whether the post-callNative
//     I/O fallback path should also fire (it should not — the helper
//     already handled the bailing instruction).
//
// Side effects on cpu.PC, cpu.regs[31] (SP), cpu.FPU, and cpu.trapped
// mirror the interpreter exactly. On clean completion PC is advanced
// by IE64_INSTR_SIZE for data/stack ops; control-flow ops (JSR/RTS/
// JSR_IND) set PC to the resolved target. On fault, trapFault has
// already set cpu.PC to the trap vector; we leave it alone and only
// clear cpu.trapped so the JIT loop continues.
func (cpu *CPU64) handleJITHelper() (retired uint64, handled bool) {
	op := cpu.jitCtx.NeedHelper
	if op == HELPER_NONE {
		return 0, false
	}
	cpu.jitCtx.NeedHelper = HELPER_NONE

	// Sync host SP back into the architectural register file. The
	// emitted code flushes its live SP into LiveSP immediately before
	// the helper exit so the dispatcher and trapFault see the most
	// recent value.
	cpu.regs[31] = cpu.jitCtx.LiveSP

	// Stage cpu.PC at the requesting instruction's PC. trapFault
	// captures cpu.PC into cpu.faultPC; if the helper takes a fault we
	// need that to be the original instruction, not whatever block-exit
	// PC the emitter wrote into RetPC.
	cpu.PC = cpu.jitCtx.HelperPC

	addr := cpu.jitCtx.HelperAddr
	size := byte(cpu.jitCtx.HelperSize)
	rd := byte(cpu.jitCtx.HelperRd)
	val := cpu.jitCtx.HelperVal

	// Stack helpers go through the memBase/memSize variant of
	// mmuStackWrite/mmuStackRead so a translated phys still inside
	// the legacy memory window stays on the direct unsafe.Pointer
	// fast path (no spurious MMIO callbacks). Build the pointer
	// arguments once.
	var memBase unsafe.Pointer
	var memSize uint64
	if len(cpu.memory) > 0 {
		memBase = unsafe.Pointer(&cpu.memory[0])
		memSize = uint64(len(cpu.memory))
	}

	switch op {
	case HELPER_LOAD:
		// Match interpreter (cpu_ie64.go:1682-1690) byte for byte:
		//   1. R0 destination → loadMem is NOT called, so spurious MMU
		//      faults / MMIO read side effects on discarded loads are
		//      suppressed.
		//   2. For Rd != 0 the result is written to Rd BEFORE the
		//      trapped check, so a fault clobbers Rd with the zero
		//      that loadMem returned after setting cpu.trapped. The
		//      trap handler observes the destination cleared — keep
		//      that visible state identical to the interpreter.
		if rd != 0 {
			v := cpu.loadMem(addr, size)
			cpu.setReg(rd, v)
			if cpu.trapped {
				cpu.trapped = false
				return 0, true
			}
		}
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_STORE:
		cpu.storeMem(addr, val, size)
		if cpu.trapped {
			cpu.trapped = false
			return 0, true
		}
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_FLOAD:
		if cpu.FPU == nil || rd > 15 {
			// Match the interpreter's fpu_missing / invalid_freg
			// labels (cpu_ie64.go:2714-2722): halt execution. Without
			// this, ExecuteJIT would suppress the I/O fallback for a
			// handled helper and re-enter the same block, looping
			// forever instead of stopping like the interpreter does.
			cpu.haltFPUFault(rd)
			return 0, true
		}
		v := uint32(cpu.loadMem(addr, IE64_SIZE_L))
		if cpu.trapped {
			cpu.trapped = false
			return 0, true
		}
		cpu.FPU.FPRegs[rd] = v
		cpu.FPU.setConditionCodesBits(v)
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_FSTORE:
		if cpu.FPU == nil || rd > 15 {
			cpu.haltFPUFault(rd)
			return 0, true
		}
		cpu.storeMem(addr, uint64(cpu.FPU.FPRegs[rd]), IE64_SIZE_L)
		if cpu.trapped {
			cpu.trapped = false
			return 0, true
		}
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_DLOAD:
		if cpu.FPU == nil || !isValidDPairReg(rd) {
			cpu.haltFPUFault(rd)
			return 0, true
		}
		v := cpu.loadMem(addr, IE64_SIZE_Q)
		if cpu.trapped {
			cpu.trapped = false
			return 0, true
		}
		cpu.FPU.setDPair(rd, math.Float64frombits(v))
		cpu.FPU.setConditionCodesBits64(v)
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_DSTORE:
		if cpu.FPU == nil || !isValidDPairReg(rd) {
			cpu.haltFPUFault(rd)
			return 0, true
		}
		cpu.storeMem(addr, math.Float64bits(cpu.FPU.getDPair(rd)), IE64_SIZE_Q)
		if cpu.trapped {
			cpu.trapped = false
			return 0, true
		}
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_PUSH:
		// Match interpreter (cpu_ie64.go:1966-1978) byte for byte.
		// Pre-decrement SP BEFORE the write so trapFault sees the
		// post-decrement value (CR_USP / stack-fault diagnostics).
		// Use the memBase/memSize variant so a translated phys still
		// inside the legacy memory window goes through the direct
		// fast-path instead of the bus phys helper — otherwise an
		// MMIO page in the low window would fire its callbacks where
		// the interpreter would have just stored to RAM.
		cpu.regs[31] -= 8
		if !cpu.mmuStackWrite(cpu.regs[31], val, memBase, memSize) {
			if cpu.trapped {
				cpu.regs[31] += 8 // roll back trapping fault
				cpu.trapped = false
				return 0, true
			}
			cpu.haltStackFault()
			return 0, true
		}
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_POP:
		v, ok := cpu.mmuStackRead(cpu.regs[31], memBase, memSize)
		if !ok {
			if cpu.trapped {
				cpu.trapped = false
				return 0, true
			}
			cpu.haltStackFault()
			return 0, true
		}
		cpu.setReg(rd, v)
		cpu.regs[31] += 8
		cpu.PC += IE64_INSTR_SIZE
		return 1, true

	case HELPER_JSR:
		// HelperVal already holds the return address (cpu.PC +
		// IE64_INSTR_SIZE computed by the emitter); HelperAddr holds
		// the call target. Same SP / fast-path discipline as
		// HELPER_PUSH; trapping fault rolls SP back, non-trapping bus
		// failure leaves it decremented at the halt.
		cpu.regs[31] -= 8
		if !cpu.mmuStackWrite(cpu.regs[31], val, memBase, memSize) {
			if cpu.trapped {
				cpu.regs[31] += 8
				cpu.trapped = false
				return 0, true
			}
			cpu.haltStackFault()
			return 0, true
		}
		cpu.PC = addr
		return 1, true

	case HELPER_RTS:
		v, ok := cpu.mmuStackRead(cpu.regs[31], memBase, memSize)
		if !ok {
			if cpu.trapped {
				cpu.trapped = false
				return 0, true
			}
			cpu.haltStackFault()
			return 0, true
		}
		cpu.regs[31] += 8
		cpu.PC = v
		return 1, true

	case HELPER_JSR_IND:
		cpu.regs[31] -= 8
		if !cpu.mmuStackWrite(cpu.regs[31], val, memBase, memSize) {
			if cpu.trapped {
				cpu.regs[31] += 8
				cpu.trapped = false
				return 0, true
			}
			cpu.haltStackFault()
			return 0, true
		}
		cpu.PC = addr
		return 1, true

	default:
		// Unknown helper opcode — defensive: drop the request and let
		// the JIT loop continue. Should never fire in practice.
		return 0, true
	}
}
