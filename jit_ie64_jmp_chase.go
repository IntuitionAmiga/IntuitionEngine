// jit_ie64_jmp_chase.go - Shared, untagged static-JMP chase for the IE64 JIT
// dispatcher (Technique 1, native exits and dispatch).
//
// When a block exits onto a static unconditional jump (BRA, or JMP with base
// R0), the dispatcher would otherwise compile and dispatch a native block per
// trampoline hop. The chase collapses a run of such jumps in Go, advancing the
// guest PC directly and reporting how many jumps retired, so trampolines and
// forwarders cost no native dispatch. It runs only in MMU-off (flat physical)
// mode. Decoding lives here, untagged, so the amd64/arm64 dispatcher and the
// wasm backend share one definition of "static unconditional jump".

package main

import "encoding/binary"

// ie64StaticJumpChaseCap bounds how many static unconditional jumps a single
// chase collapses before yielding to normal dispatch. It also doubles as the
// visited-set size for cycle detection.
const ie64StaticJumpChaseCap = 64

// ie64DecodeStaticJumpTarget decodes an IE64 instruction word and, when it is a
// static unconditional jump, returns its statically known target PC. A static
// jump is BRA (PC-relative imm32) or JMP with base register R0 (absolute imm32,
// since rs == R0/XZR is hardwired zero). Register-based JMP, conditional
// branches, calls, returns and all other opcodes return (0, false). instrPC is
// the PC of the instruction word itself.
func ie64DecodeStaticJumpTarget(instrWord, instrPC uint64) (uint64, bool) {
	opcode := byte(instrWord)
	if opcode != OP_BRA && opcode != OP_JMP {
		return 0, false
	}
	rs := byte(instrWord>>16) >> 3
	imm32 := uint32(instrWord >> 32)
	return ie64ResolveTerminatorTarget(opcode, rs, imm32, instrPC)
}

// ie64ChaseStaticJumps follows a chain of static unconditional jumps starting
// at startPC in flat (MMU-off) physical space. It returns the PC of the first
// non-jump instruction reached and the number of jumps collapsed. fetch reads
// the 8-byte instruction word at a physical PC and reports whether the page is
// mapped; an unmapped fetch stops the chase at that PC (retired counts only the
// jumps already collapsed).
//
// The chase is bounded by ie64StaticJumpChaseCap and stops before re-entering
// any PC visited earlier in the same chase (including a self-loop). A spin-wait
// jump is therefore left for normal block dispatch, where the top-of-loop
// interrupt poll can service it; the chase never spins.
func ie64ChaseStaticJumps(startPC uint64, fetch func(pc uint64) (uint64, bool)) (uint64, uint32) {
	pc := startPC
	var retired uint32
	var visited [ie64StaticJumpChaseCap]uint64
	for retired < ie64StaticJumpChaseCap {
		word, ok := fetch(pc)
		if !ok {
			return pc, retired
		}
		target, isJump := ie64DecodeStaticJumpTarget(word, pc)
		if !isJump {
			return pc, retired
		}
		if target == pc {
			return pc, retired
		}
		for i := uint32(0); i < retired; i++ {
			if visited[i] == target {
				return pc, retired
			}
		}
		visited[retired] = pc
		pc = target
		retired++
	}
	return pc, retired
}

// ie64ChaseStaticJumpsFlat is the low-window/bus fetch adapter used by the
// native dispatcher. memory is the low physical window; addresses above it are
// read through the bus. It exists so the dispatcher call site stays a single
// line and the closure is defined once.
func ie64ChaseStaticJumpsFlat(startPC uint64, memory []byte, bus *MachineBus) (uint64, uint32) {
	memLen := uint64(len(memory))
	return ie64ChaseStaticJumps(startPC, func(p uint64) (uint64, bool) {
		if memLen >= IE64_INSTR_SIZE && p <= memLen-IE64_INSTR_SIZE {
			return binary.LittleEndian.Uint64(memory[p:]), true
		}
		return bus.ReadPhys64WithFault(p)
	})
}
