// jit_6502_emit_arm64.go - Linux ARM64 6502 native emitter.
//
// The ARM64 backend deliberately uses the same JIT6502Context return contract
// as AMD64. A block always enters with X0 = *JIT6502Context and returns to the
// Go dispatcher through RET. Forms not yet lowered set NeedBail without any
// guest-side effect, so the dispatcher re-executes the original instruction in
// the interpreter at its normal observation point.

//go:build arm64 && linux

package main

import (
	"encoding/binary"
	"fmt"
)

// p65ARM64MovW emits a 32-bit constant materialisation without depending on
// the Z80 emitter's similarly named helpers.
func p65ARM64MovW(cb *CodeBuffer, reg byte, value uint32) {
	cb.Emit32(arm64MOVZ_W(reg, uint16(value), 0))
	if value>>16 != 0 {
		cb.Emit32(arm64MOVK_W(reg, uint16(value>>16), 16))
	}
}

func p65ARM64StoreW(cb *CodeBuffer, src, base byte, offset uint32) {
	if offset%4 != 0 {
		panic("unaligned ARM64 6502 context word store")
	}
	cb.Emit32(arm64STR_W_imm(src, base, offset/4))
}

func p65ARM64StoreX(cb *CodeBuffer, src, base byte, offset uint32) {
	if offset%8 != 0 {
		panic("unaligned ARM64 6502 context doubleword store")
	}
	cb.Emit32(arm64STR_imm(src, base, offset/8))
}

// emitP65ARM64StoreCycles publishes this block's cycle charge. Once a
// patched chain has entered a successor, RetCycles already contains the
// predecessor charge and must be accumulated instead of replaced.
func emitP65ARM64StoreCycles(cb *CodeBuffer, cycles uint32) {
	p65ARM64MovW(cb, 1, cycles)
	cb.Emit32(arm64LDR_W_imm(3, 0, j65CtxOffChainCount/4))
	store := cb.Len()
	cb.Emit32(arm64CBZ(3, 0))
	cb.Emit32(arm64LDR_imm(4, 0, j65CtxOffRetCycles/8))
	cb.Emit32(arm64ADD(1, 4, 1))
	cb.Emit32(arm64STR_imm(1, 0, j65CtxOffRetCycles/8))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	storePC := cb.Len()
	cb.PatchUint32(store, arm64CBZ(3, int32(storePC-store)))
	p65ARM64StoreX(cb, 1, 0, j65CtxOffRetCycles)
	donePC := cb.Len()
	cb.PatchUint32(done, arm64B(int32(donePC-done)))
}

// emitP65ARM64Return writes the dispatcher-visible return triple, tags the
// execution as an ARM64 native entry, and returns. X0 remains the context
// pointer throughout; W1 is scratch.
func emitP65ARM64Return(cb *CodeBuffer, retPC, retCount, cycles uint32, needBail bool) {
	p65ARM64MovW(cb, 1, retPC)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffRetPC)
	p65ARM64MovW(cb, 1, retCount)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffRetCount)
	p65ARM64MovW(cb, 1, p65ARM64BackendMarker)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffBackendMarker)
	if needBail {
		p65ARM64MovW(cb, 1, 1)
		p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedBail)
	}
	if cycles == 0 {
		// STR XZR, [X0, #RetCycles]
		p65ARM64StoreX(cb, 31, 0, j65CtxOffRetCycles)
	} else {
		emitP65ARM64StoreCycles(cb, cycles)
	}
	cb.Emit32(0xD65F03C0) // RET
}

func emitP65ARM64ReturnDynamicPC(cb *CodeBuffer, pcReg byte, retCount, cycles uint32) {
	p65ARM64StoreW(cb, pcReg, 0, j65CtxOffRetPC)
	p65ARM64MovW(cb, 1, retCount)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffRetCount)
	p65ARM64MovW(cb, 1, p65ARM64BackendMarker)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffBackendMarker)
	emitP65ARM64StoreCycles(cb, cycles)
	cb.Emit32(0xD65F03C0) // RET
}

// emitP65ARM64ChainTail turns a straight-line block's final RET into a
// bounded, generation-safe patchable tail branch. The normal unpatched path
// keeps the existing return state; a patched path reaches the successor with
// X0 intact and accumulates retired instructions and cycles in the context.
func emitP65ARM64ChainTail(cb *CodeBuffer) (patchOffset, fallbackOffset int) {
	if cb.Len() < 4 {
		panic("ARM64 6502 chain tail without return")
	}
	retOffset := cb.Len() - 4
	if got := binary.LittleEndian.Uint32(cb.Bytes()[retOffset:]); got != arm64RET() {
		panic("ARM64 6502 chain tail requires final RET")
	}

	// Replace the terminal RET with a branch to the accounting stub below.
	entryBranch := retOffset
	cb.PatchUint32(entryBranch, arm64B(int32(cb.Len()-entryBranch)))

	// ChainCount++ and ChainBudget--. The normal cold exit below clears the
	// speculative count; a successor's cold exit observes count > 1 and
	// publishes it as the dispatcher's RetCount instead.
	cb.Emit32(arm64LDR_W_imm(1, 0, j65CtxOffChainCount/4))
	p65ARM64MovW(cb, 2, 1)
	cb.Emit32(arm64ADD_W(1, 1, 2))
	p65ARM64StoreW(cb, 1, 0, j65CtxOffChainCount)
	cb.Emit32(arm64LDR_W_imm(1, 0, j65CtxOffChainBudget/4))
	cb.Emit32(arm64SUB_W(1, 1, 2))
	p65ARM64StoreW(cb, 1, 0, j65CtxOffChainBudget)
	budgetExhausted := cb.Len()
	cb.Emit32(arm64CBZ(1, 0))
	cb.Emit32(arm64LDR_W_imm(1, 0, j65CtxOffNeedInval/4))
	needInval := cb.Len()
	cb.Emit32(arm64CBNZ(1, 0))
	// External writers publish a new dispatch generation before the owning
	// 6502 thread drains invalidations. Do not enter a patched successor after
	// that publication, even if the local invalidation flag has not yet been
	// observed.
	cb.Emit32(arm64LDR_imm(3, 0, j65CtxOffDispatchGenPtr/8))
	cb.Emit32(arm64LDR_imm(3, 3, 0))
	cb.Emit32(arm64LDR_imm(4, 0, j65CtxOffDispatchGeneration/8))
	cb.Emit32(arm64CMP(3, 4))
	generationChanged := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))

	// The branch word is overwritten atomically through ExecMem's RW alias
	// when a compatible target is cached. It initially selects the cold exit.
	patchOffset = cb.Len()
	cb.Emit32(arm64B(0))
	fallbackOffset = cb.Len()
	cb.PatchUint32(budgetExhausted, arm64CBZ(1, int32(fallbackOffset-budgetExhausted)))
	cb.PatchUint32(needInval, arm64CBNZ(1, int32(fallbackOffset-needInval)))
	cb.PatchUint32(generationChanged, arm64Bcond(arm64CondNE, int32(fallbackOffset-generationChanged)))
	cb.PatchUint32(patchOffset, arm64B(int32(fallbackOffset-patchOffset)))

	cb.Emit32(arm64LDR_W_imm(1, 0, j65CtxOffChainCount/4))
	p65ARM64MovW(cb, 2, 1)
	cb.Emit32(arm64CMP_W(1, 2))
	chainedExit := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHI, 0))
	// Not patched: discard the speculative count and retain RetCount=1.
	p65ARM64StoreW(cb, 31, 0, j65CtxOffChainCount)
	cb.Emit32(arm64RET())
	chainedExitPC := cb.Len()
	cb.PatchUint32(chainedExit, arm64Bcond(arm64CondHI, int32(chainedExitPC-chainedExit)))
	// A real chain: RetCount is represented by ChainCount, including this
	// final block, so suppress the per-block value emitted above.
	p65ARM64StoreW(cb, 31, 0, j65CtxOffRetCount)
	cb.Emit32(arm64RET())
	return patchOffset, fallbackOffset
}

func patchP65ARM64ChainBranch(patchAddr, target uintptr) {
	p, writableAddr, ok := lookupWritableBytes(patchAddr, 4)
	if !ok {
		return
	}
	disp := int64(target) - int64(patchAddr)
	if disp&3 != 0 || disp < -(1<<27) || disp >= 1<<27 {
		panic("ARM64 6502 chain target outside B range")
	}
	binary.LittleEndian.PutUint32(p, arm64B(int32(disp)))
	flushICacheDual(writableAddr, patchAddr, 4)
}

// emitP65ARM64StackPageInvalidation requests a cache invalidation only when
// page one has actually been used as source code. Ordinary JSR/PHA traffic
// must not throw away the RTS target cache merely because it uses the stack.
func emitP65ARM64StackPageInvalidation(cb *CodeBuffer) {
	cb.Emit32(arm64LDR_imm(3, 0, j65CtxOffCodePageBitmap/8))
	p65ARM64MovW(cb, 4, 1)
	cb.Emit32(arm64LDRB_reg(3, 3, 4))
	skip := cb.Len()
	cb.Emit32(arm64CBZ(3, 0))
	p65ARM64MovW(cb, 3, 1)
	p65ARM64StoreW(cb, 3, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 3, 0, j65CtxOffInvalPage)
	done := cb.Len()
	cb.PatchUint32(skip, arm64CBZ(3, int32(done-skip)))
}

func p65ARM64BR(rn byte) uint32 { return 0xD61F0000 | uint32(rn)<<5 }

// emitP65ARM64RTSCacheExit publishes the completed RTS then uses the
// CPU-owned two-entry MRU cache to jump to a previously compiled return
// target. A cache miss, budget expiry, generation change or pending SMC
// returns normally with the dynamically computed PC.
func emitP65ARM64RTSCacheExit(cb *CodeBuffer, pcReg byte) {
	p65ARM64StoreW(cb, pcReg, 0, j65CtxOffRetPC)
	p65ARM64MovW(cb, 3, 1)
	p65ARM64StoreW(cb, 3, 0, j65CtxOffRetCount)
	p65ARM64MovW(cb, 3, p65ARM64BackendMarker)
	p65ARM64StoreW(cb, 3, 0, j65CtxOffBackendMarker)
	emitP65ARM64StoreCycles(cb, 6)

	// Entry 0, then entry 1. X5 is the cached native entry on a hit.
	cb.Emit32(arm64LDR_W_imm(3, 0, j65CtxOffRTSCache0PC/4))
	cb.Emit32(arm64CMP_W(pcReg, 3))
	miss0 := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffRTSCache0Addr/8))
	hit := cb.Len()
	cb.Emit32(arm64B(0))
	check1 := cb.Len()
	cb.PatchUint32(miss0, arm64Bcond(arm64CondNE, int32(check1-miss0)))
	cb.Emit32(arm64LDR_W_imm(3, 0, j65CtxOffRTSCache1PC/4))
	cb.Emit32(arm64CMP_W(pcReg, 3))
	miss1 := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffRTSCache1Addr/8))
	hitPC := cb.Len()
	cb.PatchUint32(hit, arm64B(int32(hitPC-hit)))

	// Account for RTS before a cached tail jump. The successor's chain tail
	// converts the final result to ChainCount and accumulates RetCycles.
	cb.Emit32(arm64LDR_W_imm(3, 0, j65CtxOffChainCount/4))
	p65ARM64MovW(cb, 4, 1)
	cb.Emit32(arm64ADD_W(3, 3, 4))
	p65ARM64StoreW(cb, 3, 0, j65CtxOffChainCount)
	cb.Emit32(arm64LDR_W_imm(3, 0, j65CtxOffChainBudget/4))
	cb.Emit32(arm64SUB_W(3, 3, 4))
	p65ARM64StoreW(cb, 3, 0, j65CtxOffChainBudget)
	exhausted := cb.Len()
	cb.Emit32(arm64CBZ(3, 0))
	cb.Emit32(arm64LDR_W_imm(3, 0, j65CtxOffNeedInval/4))
	pendingInval := cb.Len()
	cb.Emit32(arm64CBNZ(3, 0))
	cb.Emit32(arm64LDR_imm(3, 0, j65CtxOffDispatchGenPtr/8))
	cb.Emit32(arm64LDR_imm(3, 3, 0))
	cb.Emit32(arm64LDR_imm(4, 0, j65CtxOffDispatchGeneration/8))
	cb.Emit32(arm64CMP(3, 4))
	generationChanged := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	cb.Emit32(p65ARM64BR(5))

	miss := cb.Len()
	cb.PatchUint32(miss1, arm64Bcond(arm64CondNE, int32(miss-miss1)))
	cb.PatchUint32(exhausted, arm64CBZ(3, int32(miss-exhausted)))
	cb.PatchUint32(pendingInval, arm64CBNZ(3, int32(miss-pendingInval)))
	cb.PatchUint32(generationChanged, arm64Bcond(arm64CondNE, int32(miss-generationChanged)))
	// The speculative count is only meaningful after a real cache jump.
	p65ARM64StoreW(cb, 31, 0, j65CtxOffChainCount)
	cb.Emit32(arm64RET())
}

// emitP65ARM64LoadImmediate lowers the three documented immediate load forms.
// Their N/Z result is known at compile time, so no host condition-code state
// leaks into the guest status register.
func emitP65ARM64LoadImmediate(cb *CodeBuffer, startPC uint16, operand byte, cpuOffset uint32) {
	// X2 = ctx.CpuPtr.
	cb.Emit32(arm64LDR_imm(2, 0, j65CtxOffCpuPtr/8))
	p65ARM64MovW(cb, 1, uint32(operand))
	cb.Emit32(arm64STRB_imm(1, 2, cpuOffset))

	// SR = (SR & ^(N|Z)) | knownNZ.
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(ZERO_FLAG|NEGATIVE_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	flags := uint32(0)
	if operand == 0 {
		flags |= ZERO_FLAG
	}
	if operand&0x80 != 0 {
		flags |= NEGATIVE_FLAG
	}
	p65ARM64MovW(cb, 4, flags)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, 2, false)
}

func emitP65ARM64LoadCPU(cb *CodeBuffer) {
	cb.Emit32(arm64LDR_imm(2, 0, j65CtxOffCpuPtr/8))
}

// emitP65ARM64SetNZ updates only the canonical N and Z bits from W1. The
// shared nzTable keeps this path identical to the interpreter for every byte.
func emitP65ARM64SetNZ(cb *CodeBuffer) {
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(ZERO_FLAG|NEGATIVE_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	cb.Emit32(arm64LDR_imm(4, 0, j65CtxOffNZTablePtr/8))
	cb.Emit32(arm64LDRB_reg(4, 4, 1))
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
}

func emitP65ARM64Transfer(cb *CodeBuffer, startPC uint16, sourceOffset, destOffset uint32) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(1, 2, sourceOffset))
	cb.Emit32(arm64STRB_imm(1, 2, destOffset))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+1, 1, 2, false)
}

func emitP65ARM64TransferNoFlags(cb *CodeBuffer, startPC uint16, sourceOffset, destOffset uint32) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(1, 2, sourceOffset))
	cb.Emit32(arm64STRB_imm(1, 2, destOffset))
	emitP65ARM64Return(cb, uint32(startPC)+1, 1, 2, false)
}

func emitP65ARM64IncDec(cb *CodeBuffer, startPC uint16, offset uint32, decrement bool) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(1, 2, offset))
	p65ARM64MovW(cb, 4, 1)
	if decrement {
		cb.Emit32(arm64SUB_W(1, 1, 4))
	} else {
		cb.Emit32(arm64ADD_W(1, 1, 4))
	}
	cb.Emit32(arm64STRB_imm(1, 2, offset))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+1, 1, 2, false)
}

func emitP65ARM64AccumulatorShift(cb *CodeBuffer, startPC uint16, opcode byte) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffA))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	switch opcode {
	case 0x0A: // ASL A
		cb.Emit32(arm64LSR_W_imm(4, 1, 7))
		p65ARM64MovW(cb, 5, 1)
		cb.Emit32(arm64LSL_W(1, 1, 5))
	case 0x4A: // LSR A
		p65ARM64MovW(cb, 5, 1)
		cb.Emit32(arm64AND_W(4, 1, 5))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
	case 0x2A: // ROL A
		cb.Emit32(arm64LSR_W_imm(4, 1, 7))
		p65ARM64MovW(cb, 5, CARRY_FLAG)
		cb.Emit32(arm64AND_W(5, 3, 5))
		p65ARM64MovW(cb, 3, 1)
		cb.Emit32(arm64LSL_W(1, 1, 3))
		cb.Emit32(arm64ORR_W(1, 1, 5))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	case 0x6A: // ROR A
		p65ARM64MovW(cb, 5, CARRY_FLAG)
		cb.Emit32(arm64AND_W(4, 1, 5))
		cb.Emit32(arm64AND_W(5, 3, 5))
		p65ARM64MovW(cb, 3, 7)
		cb.Emit32(arm64LSL_W(5, 5, 3))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
		cb.Emit32(arm64ORR_W(1, 1, 5))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	}
	cb.Emit32(arm64STRB_imm(1, 2, cpu6502OffA))
	p65ARM64MovW(cb, 5, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+1, 1, 2, false)
}

func emitP65ARM64CompareImmediate(cb *CodeBuffer, startPC uint16, operand byte, sourceOffset uint32) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(3, 2, sourceOffset))
	p65ARM64MovW(cb, 4, uint32(operand))
	cb.Emit32(arm64SUB_W(1, 3, 4)) // N/Z derive from the low byte.
	cb.Emit32(arm64CMP_W(3, 4))
	carryOffset := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHS, 0))
	// C clear path.
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, 2, false)
	carryPC := cb.Len()
	cb.PatchUint32(carryOffset, arm64Bcond(arm64CondHS, int32(carryPC-carryOffset)))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, CARRY_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, 2, false)
}

func emitP65ARM64CompareDirect(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, sourceOffset uint32, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4)) // operand
	cb.Emit32(arm64LDRB_imm(3, 2, sourceOffset))
	cb.Emit32(arm64SUB_W(4, 3, 1))
	cb.Emit32(arm64CMP_W(3, 1))
	carryOffset := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHS, 0))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	carryPC := cb.Len()
	cb.PatchUint32(carryOffset, arm64Bcond(arm64CondHS, int32(carryPC-carryOffset)))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, CARRY_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(4, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64Flag(cb *CodeBuffer, startPC uint16, flag byte, set bool) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffSR))
	if set {
		p65ARM64MovW(cb, 4, uint32(flag))
		cb.Emit32(arm64ORR_W(1, 1, 4))
	} else {
		p65ARM64MovW(cb, 4, ^uint32(flag))
		cb.Emit32(arm64AND_W(1, 1, 4))
	}
	cb.Emit32(arm64STRB_imm(1, 2, cpu6502OffSR))
	emitP65ARM64Return(cb, uint32(startPC)+1, 1, 2, false)
}

// emitP65ARM64Branch lowers the eight relative conditional branches. The
// interpreter's historical cycle accounting charges only the taken and
// page-cross penalties here, so the published totals intentionally differ
// from the NMOS data-sheet base-cycle convention.
func emitP65ARM64Branch(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, flag byte, branchWhenSet bool) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, uint32(flag))
	cb.Emit32(arm64AND_W(4, 3, 4))
	branchOffset := cb.Len()
	if branchWhenSet {
		cb.Emit32(arm64CBNZ(4, 0))
	} else {
		cb.Emit32(arm64CBZ(4, 0))
	}

	fallPC := uint16(uint32(startPC) + 2)
	// Branch handlers in CPU_6502 add only these penalties. Keep this exact
	// until the interpreter's cycle model is deliberately changed as a unit.
	emitP65ARM64Return(cb, uint32(fallPC), 1, 0, false)
	takenOffset := cb.Len()
	if branchWhenSet {
		cb.PatchUint32(branchOffset, arm64CBNZ(4, int32(takenOffset-branchOffset)))
	} else {
		cb.PatchUint32(branchOffset, arm64CBZ(4, int32(takenOffset-branchOffset)))
	}
	target := uint16(int32(fallPC) + int32(int8(instr.operand)))
	cycles := uint32(1)
	if fallPC&0xFF00 != target&0xFF00 {
		cycles++
	}
	emitP65ARM64Return(cb, uint32(target), 1, cycles, false)
}

// emitP65ARM64Stack lowers the four byte stack forms against plain RAM page
// one. Stack writes request invalidation because guest code is permitted to
// place executable bytes on the stack page.
func emitP65ARM64Stack(cb *CodeBuffer, startPC uint16, opcode byte) {
	guard := emitP65ARM64DirectPageGuard(cb, 1)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	switch opcode {
	case 0x48, 0x08: // PHA, PHP
		p65ARM64MovW(cb, 5, 0x100)
		cb.Emit32(arm64ADD_W(4, 4, 5))
		if opcode == 0x48 {
			cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffA))
		} else {
			cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffSR))
			p65ARM64MovW(cb, 5, BREAK_FLAG|UNUSED_FLAG)
			cb.Emit32(arm64ORR_W(1, 1, 5))
		}
		cb.Emit32(arm64STRB_reg(1, 6, 4))
		cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
		p65ARM64MovW(cb, 5, 1)
		cb.Emit32(arm64SUB_W(4, 4, 5))
		cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
		p65ARM64MovW(cb, 1, 1)
		p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
		p65ARM64StoreW(cb, 1, 0, j65CtxOffInvalPage)
		emitP65ARM64Return(cb, uint32(startPC)+1, 1, 3, false)
	case 0x68, 0x28: // PLA, PLP
		p65ARM64MovW(cb, 5, 1)
		cb.Emit32(arm64ADD_W(4, 4, 5))
		cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
		p65ARM64MovW(cb, 5, 0x100)
		cb.Emit32(arm64ADD_W(4, 4, 5))
		cb.Emit32(arm64LDRB_reg(1, 6, 4))
		if opcode == 0x68 {
			cb.Emit32(arm64STRB_imm(1, 2, cpu6502OffA))
			emitP65ARM64SetNZ(cb)
		} else {
			p65ARM64MovW(cb, 5, ^uint32(BREAK_FLAG))
			cb.Emit32(arm64AND_W(1, 1, 5))
			p65ARM64MovW(cb, 5, UNUSED_FLAG)
			cb.Emit32(arm64ORR_W(1, 1, 5))
			cb.Emit32(arm64STRB_imm(1, 2, cpu6502OffSR))
		}
		emitP65ARM64Return(cb, uint32(startPC)+1, 1, 4, false)
	}
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64JSR(cb *CodeBuffer, startPC uint16, target uint16) {
	guard := emitP65ARM64DirectPageGuard(cb, 1)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 5, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 1, uint32((uint16(uint32(startPC)+2))>>8))
	cb.Emit32(arm64STRB_reg(1, 6, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 5, 1)
	cb.Emit32(arm64SUB_W(4, 4, 5))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 5, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 1, uint32(byte(uint16(uint32(startPC)+2))))
	cb.Emit32(arm64STRB_reg(1, 6, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 5, 1)
	cb.Emit32(arm64SUB_W(4, 4, 5))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	emitP65ARM64StackPageInvalidation(cb)
	emitP65ARM64Return(cb, uint32(target), 1, 6, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64RTS(cb *CodeBuffer, startPC uint16) {
	guard := emitP65ARM64DirectPageGuard(cb, 1)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, 8)
	cb.Emit32(arm64LSL_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(1, 1, 3))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(1, 1, 6))
	p65ARM64MovW(cb, 6, 0xFFFF)
	cb.Emit32(arm64AND_W(1, 1, 6))
	emitP65ARM64RTSCacheExit(cb, 1)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64JMPIndirect(cb *CodeBuffer, startPC uint16, pointer uint16) {
	guardLow := emitP65ARM64DirectPageGuard(cb, byte(pointer>>8))
	highAddr := (pointer & 0xFF00) | ((pointer + 1) & 0x00FF) // NMOS page-wrap bug.
	guardHigh := emitP65ARM64DirectPageGuard(cb, byte(highAddr>>8))
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(pointer))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 4, uint32(highAddr))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 4, 8)
	cb.Emit32(arm64LSL_W(3, 3, 4))
	cb.Emit32(arm64ORR_W(1, 1, 3))
	emitP65ARM64ReturnDynamicPC(cb, 1, 1, 5)
	bailPC := cb.Len()
	cb.PatchUint32(guardLow, arm64CBNZ(4, int32(bailPC-guardLow)))
	cb.PatchUint32(guardHigh, arm64CBNZ(4, int32(bailPC-guardHigh)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64RTI(cb *CodeBuffer, startPC uint16) {
	guard := emitP65ARM64DirectPageGuard(cb, 1)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	// Pop status, then low and high PC bytes.
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, ^uint32(BREAK_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 6))
	p65ARM64MovW(cb, 6, UNUSED_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 6))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, 8)
	cb.Emit32(arm64LSL_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(1, 1, 3))
	emitP65ARM64ReturnDynamicPC(cb, 1, 1, 6)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64BRK(cb *CodeBuffer, startPC uint16) {
	stackGuard := emitP65ARM64DirectPageGuard(cb, 1)
	vectorGuard := emitP65ARM64IOPageGuard(cb, 0xFF)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	returnPC := uint16(uint32(startPC) + 2)
	// Push PC high, PC low, then SR with B/U asserted.
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	p65ARM64MovW(cb, 1, uint32(returnPC>>8))
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64SUB_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	// Keep the low byte in W3: W1 is also the dynamic return-PC channel and
	// must not be relied on across the preceding indexed store sequence.
	p65ARM64MovW(cb, 3, uint32(byte(returnPC)))
	cb.Emit32(arm64STRB_reg(3, 5, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64SUB_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 0x100)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 6, BREAK_FLAG|UNUSED_FLAG)
	cb.Emit32(arm64ORR_W(1, 1, 6))
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffSP))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64SUB_W(4, 4, 6))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffSP))
	// Live SR gets I/U, with B clear.
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 6, INTERRUPT_FLAG|UNUSED_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 6))
	p65ARM64MovW(cb, 6, ^uint32(BREAK_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 6))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	// Defined vector path: $FFFE/$FFFF bypasses the broad I/O-page guard.
	p65ARM64MovW(cb, 4, IRQ_VECTOR)
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 4, IRQ_VECTOR+1)
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, 8)
	cb.Emit32(arm64LSL_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(1, 1, 3))
	p65ARM64MovW(cb, 3, 1)
	p65ARM64StoreW(cb, 3, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 3, 0, j65CtxOffInvalPage)
	emitP65ARM64ReturnDynamicPC(cb, 1, 1, 7)
	patchP65ARM64GuardToBail(cb, stackGuard)
	patchP65ARM64GuardToBail(cb, vectorGuard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64DirectPageGuard branches around a native memory access when the
// live mapping says the addressed page is not plain RAM. The caller appends
// the exact no-side-effect bailout after its normal RET and patches this branch
// to it. X0 stays the context pointer; X5/W4 are scratch.
func emitP65ARM64DirectPageGuard(cb *CodeBuffer, page byte) int {
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffDirectPageBitmapPtr/8))
	p65ARM64MovW(cb, 4, uint32(page))
	cb.Emit32(arm64LDRB_reg(4, 5, 4))
	offset := cb.Len()
	cb.Emit32(arm64CBNZ(4, 0))
	return offset
}

// emitP65ARM64IOPageGuard preserves the defined $FFFA-$FFFF vector path:
// vector RAM is native-eligible despite the broad $Fxxx direct-page policy,
// but an explicit device mapping on page $FF must re-execute in the interpreter.
func emitP65ARM64IOPageGuard(cb *CodeBuffer, page byte) int {
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffIOBitmapPtr/8))
	p65ARM64MovW(cb, 4, uint32(page))
	cb.Emit32(arm64LDRB_reg(4, 5, 4))
	offset := cb.Len()
	cb.Emit32(arm64CBNZ(4, 0))
	return offset
}

func patchP65ARM64GuardToBail(cb *CodeBuffer, branchOffset int) {
	bailOffset := cb.Len()
	cb.PatchUint32(branchOffset, arm64CBNZ(4, int32(bailOffset-branchOffset)))
}

func emitP65ARM64DynamicPageGuard(cb *CodeBuffer, addrReg byte) int {
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffDirectPageBitmapPtr/8))
	cb.Emit32(arm64LSR_W_imm(6, addrReg, 8))
	cb.Emit32(arm64LDRB_reg(6, 5, 6))
	offset := cb.Len()
	cb.Emit32(arm64CBNZ(6, 0))
	return offset
}

func emitP65ARM64DirectLoad(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, cpuOffset uint32, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64STRB_imm(1, 2, cpuOffset))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64DirectStore performs a mapping-stable plain-RAM store. It
// conservatively requests a page invalidation after every native store: this
// is more frequent than the bitmap fast path used by AMD64, but it preserves
// self-modifying-code correctness while ARM64 lowering is expanded.
func emitP65ARM64DirectStore(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, cpuOffset uint32, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, cpuOffset))
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64MovW(cb, 1, uint32(instr.operand>>8))
	p65ARM64StoreW(cb, 1, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64DecimalImmediate selects immutable decimal or binary result
// tables, so both ADC and SBC modes share exactly the interpreter's flags.
func emitP65ARM64DecimalImmediate(cb *CodeBuffer, startPC uint16, operand byte, decimalTableOffset, binaryTableOffset uint32) {
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, DECIMAL_FLAG)
	cb.Emit32(arm64AND_W(5, 3, 5))
	branchToBinary := cb.Len()
	cb.Emit32(arm64CBZ(5, 0))
	cb.Emit32(arm64LDR_imm(6, 0, decimalTableOffset/8))
	branchToLookup := cb.Len()
	cb.Emit32(arm64B(0))
	binaryPC := cb.Len()
	cb.PatchUint32(branchToBinary, arm64CBZ(5, int32(binaryPC-branchToBinary)))
	cb.Emit32(arm64LDR_imm(6, 0, binaryTableOffset/8))
	lookupPC := cb.Len()
	cb.PatchUint32(branchToLookup, arm64B(int32(lookupPC-branchToLookup)))

	// Table index is A | operand<<8 | carry<<16; entries are two bytes.
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffA))
	p65ARM64MovW(cb, 4, uint32(operand))
	p65ARM64MovW(cb, 5, 8)
	cb.Emit32(arm64LSL_W(4, 4, 5))
	cb.Emit32(arm64ADD_W(1, 1, 4))
	p65ARM64MovW(cb, 5, CARRY_FLAG)
	cb.Emit32(arm64AND_W(3, 3, 5))
	p65ARM64MovW(cb, 5, 16)
	cb.Emit32(arm64LSL_W(3, 3, 5))
	cb.Emit32(arm64ADD_W(1, 1, 3))
	// The table has two bytes per entry: A then C/V/N/Z flags.
	p65ARM64MovW(cb, 5, 1)
	cb.Emit32(arm64LSL_W(1, 1, 5))
	cb.Emit32(arm64LDRH_reg(4, 6, 1))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffA))
	cb.Emit32(arm64LSR_W_imm(4, 4, 8))
	// W3 was used to form the table index. Reload the unmodified status so
	// I/D/B and the architectural unused bit survive the C/V/N/Z replacement.
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, ^uint32(CARRY_FLAG|OVERFLOW_FLAG|NEGATIVE_FLAG|ZERO_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, 2, false)
}

// emitP65ARM64ArithmeticDirect performs ADC/SBC with an already loaded
// direct-RAM operand. It shares the packed result-table contract used by the
// immediate forms, avoiding host-flag dependence in either decimal mode.
func emitP65ARM64ArithmeticDirect(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, decimalTableOffset, binaryTableOffset, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, DECIMAL_FLAG)
	cb.Emit32(arm64AND_W(5, 3, 5))
	branchToBinary := cb.Len()
	cb.Emit32(arm64CBZ(5, 0))
	cb.Emit32(arm64LDR_imm(6, 0, decimalTableOffset/8))
	branchToLookup := cb.Len()
	cb.Emit32(arm64B(0))
	binaryPC := cb.Len()
	cb.PatchUint32(branchToBinary, arm64CBZ(5, int32(binaryPC-branchToBinary)))
	cb.Emit32(arm64LDR_imm(6, 0, binaryTableOffset/8))
	lookupPC := cb.Len()
	cb.PatchUint32(branchToLookup, arm64B(int32(lookupPC-branchToLookup)))
	// index = A | operand<<8 | carry<<16, then byte offset *= 2.
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffA))
	p65ARM64MovW(cb, 5, 8)
	cb.Emit32(arm64LSL_W(1, 1, 5))
	cb.Emit32(arm64ADD_W(1, 1, 4))
	p65ARM64MovW(cb, 5, CARRY_FLAG)
	cb.Emit32(arm64AND_W(3, 3, 5))
	p65ARM64MovW(cb, 5, 16)
	cb.Emit32(arm64LSL_W(3, 3, 5))
	cb.Emit32(arm64ADD_W(1, 1, 3))
	p65ARM64MovW(cb, 5, 1)
	cb.Emit32(arm64LSL_W(1, 1, 5))
	cb.Emit32(arm64LDRH_reg(4, 6, 1))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffA))
	cb.Emit32(arm64LSR_W_imm(4, 4, 8))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, ^uint32(CARRY_FLAG|OVERFLOW_FLAG|NEGATIVE_FLAG|ZERO_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(4, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64ArithmeticLoadedOperand applies the shared binary/decimal
// result-table ABI to the byte already present in W1. X2 is CpuPtr.
func emitP65ARM64ArithmeticLoadedOperand(cb *CodeBuffer, decimalTableOffset, binaryTableOffset uint32) {
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, DECIMAL_FLAG)
	cb.Emit32(arm64AND_W(5, 3, 5))
	branchToBinary := cb.Len()
	cb.Emit32(arm64CBZ(5, 0))
	cb.Emit32(arm64LDR_imm(6, 0, decimalTableOffset/8))
	branchToLookup := cb.Len()
	cb.Emit32(arm64B(0))
	binaryPC := cb.Len()
	cb.PatchUint32(branchToBinary, arm64CBZ(5, int32(binaryPC-branchToBinary)))
	cb.Emit32(arm64LDR_imm(6, 0, binaryTableOffset/8))
	lookupPC := cb.Len()
	cb.PatchUint32(branchToLookup, arm64B(int32(lookupPC-branchToLookup)))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffA))
	p65ARM64MovW(cb, 5, 8)
	cb.Emit32(arm64LSL_W(1, 1, 5))
	cb.Emit32(arm64ADD_W(1, 1, 4))
	p65ARM64MovW(cb, 5, CARRY_FLAG)
	cb.Emit32(arm64AND_W(3, 3, 5))
	p65ARM64MovW(cb, 5, 16)
	cb.Emit32(arm64LSL_W(3, 3, 5))
	cb.Emit32(arm64ADD_W(1, 1, 3))
	p65ARM64MovW(cb, 5, 1)
	cb.Emit32(arm64LSL_W(1, 1, 5))
	cb.Emit32(arm64LDRH_reg(4, 6, 1))
	cb.Emit32(arm64STRB_imm(4, 2, cpu6502OffA))
	cb.Emit32(arm64LSR_W_imm(4, 4, 8))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, ^uint32(CARRY_FLAG|OVERFLOW_FLAG|NEGATIVE_FLAG|ZERO_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
}

type p65ARM64LogicOp byte

const (
	p65ARM64And p65ARM64LogicOp = iota
	p65ARM64Ora
	p65ARM64Eor
)

func emitP65ARM64LogicOp(cb *CodeBuffer, op p65ARM64LogicOp) {
	// W1 is the operand and W3 is A. Keep the result in W1 for SetNZ.
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffA))
	switch op {
	case p65ARM64And:
		cb.Emit32(arm64AND_W(1, 3, 1))
	case p65ARM64Ora:
		cb.Emit32(arm64ORR_W(1, 3, 1))
	case p65ARM64Eor:
		cb.Emit32(arm64EOR_W(1, 3, 1))
	}
	cb.Emit32(arm64STRB_imm(1, 2, cpu6502OffA))
	emitP65ARM64SetNZ(cb)
}

func emitP65ARM64LogicImmediate(cb *CodeBuffer, startPC uint16, operand byte, op p65ARM64LogicOp) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 1, uint32(operand))
	emitP65ARM64LogicOp(cb, op)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, 2, false)
}

func emitP65ARM64LogicDirect(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, op p65ARM64LogicOp, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	emitP65ARM64LogicOp(cb, op)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64BITDirect(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4)) // memory value
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 5, ^uint32(ZERO_FLAG|OVERFLOW_FLAG|NEGATIVE_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	p65ARM64MovW(cb, 5, OVERFLOW_FLAG|NEGATIVE_FLAG)
	cb.Emit32(arm64AND_W(5, 1, 5))
	cb.Emit32(arm64ORR_W(3, 3, 5))
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffA))
	cb.Emit32(arm64AND_W(4, 4, 1))
	zeroOffset := cb.Len()
	cb.Emit32(arm64CBZ(4, 0))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	zeroPC := cb.Len()
	cb.PatchUint32(zeroOffset, arm64CBZ(4, int32(zeroPC-zeroOffset)))
	p65ARM64MovW(cb, 5, ZERO_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 5))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64RMWDirect(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, decrement bool, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 6, 1)
	if decrement {
		cb.Emit32(arm64SUB_W(1, 1, 6))
	} else {
		cb.Emit32(arm64ADD_W(1, 1, 6))
	}
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	emitP65ARM64SetNZ(cb)
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64MovW(cb, 1, uint32(instr.operand>>8))
	p65ARM64StoreW(cb, 1, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64RMWShiftDirect(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, opcode byte, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, byte(instr.operand>>8))
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	switch opcode {
	case 0x06, 0x0E, 0x16, 0x1E: // ASL
		cb.Emit32(arm64LSR_W_imm(7, 1, 7))
		p65ARM64MovW(cb, 6, 1)
		cb.Emit32(arm64LSL_W(1, 1, 6))
	case 0x46, 0x4E, 0x56, 0x5E: // LSR
		p65ARM64MovW(cb, 6, 1)
		cb.Emit32(arm64AND_W(7, 1, 6))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
	case 0x26, 0x2E, 0x36, 0x3E: // ROL
		cb.Emit32(arm64LSR_W_imm(7, 1, 7))
		p65ARM64MovW(cb, 6, CARRY_FLAG)
		cb.Emit32(arm64AND_W(6, 3, 6))
		p65ARM64MovW(cb, 3, 1)
		cb.Emit32(arm64LSL_W(1, 1, 3))
		cb.Emit32(arm64ORR_W(1, 1, 6))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	case 0x66, 0x6E, 0x76, 0x7E: // ROR
		p65ARM64MovW(cb, 6, CARRY_FLAG)
		cb.Emit32(arm64AND_W(7, 1, 6))
		cb.Emit32(arm64AND_W(6, 3, 6))
		p65ARM64MovW(cb, 3, 7)
		cb.Emit32(arm64LSL_W(6, 6, 3))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
		cb.Emit32(arm64ORR_W(1, 1, 6))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	}
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 6, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(3, 3, 7))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64MovW(cb, 1, uint32(instr.operand>>8))
	p65ARM64StoreW(cb, 1, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+uint32(instr.length), 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedRMW(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, decrement bool, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 6, 4))
	p65ARM64MovW(cb, 5, 1)
	if decrement {
		cb.Emit32(arm64SUB_W(1, 1, 5))
	} else {
		cb.Emit32(arm64ADD_W(1, 1, 5))
	}
	cb.Emit32(arm64STRB_reg(1, 6, 4))
	emitP65ARM64SetNZ(cb)
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 31, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedRMWShift(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 6, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	switch instr.opcode {
	case 0x16: // ASL zp,X
		cb.Emit32(arm64LSR_W_imm(7, 1, 7))
		p65ARM64MovW(cb, 5, 1)
		cb.Emit32(arm64LSL_W(1, 1, 5))
	case 0x56: // LSR zp,X
		p65ARM64MovW(cb, 5, 1)
		cb.Emit32(arm64AND_W(7, 1, 5))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
	case 0x36: // ROL zp,X
		cb.Emit32(arm64LSR_W_imm(7, 1, 7))
		p65ARM64MovW(cb, 5, CARRY_FLAG)
		cb.Emit32(arm64AND_W(5, 3, 5))
		p65ARM64MovW(cb, 3, 1)
		cb.Emit32(arm64LSL_W(1, 1, 3))
		cb.Emit32(arm64ORR_W(1, 1, 5))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	case 0x76: // ROR zp,X
		p65ARM64MovW(cb, 5, CARRY_FLAG)
		cb.Emit32(arm64AND_W(7, 1, 5))
		cb.Emit32(arm64AND_W(5, 3, 5))
		p65ARM64MovW(cb, 3, 7)
		cb.Emit32(arm64LSL_W(5, 5, 3))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
		cb.Emit32(arm64ORR_W(1, 1, 5))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	}
	cb.Emit32(arm64STRB_reg(1, 6, 4))
	p65ARM64MovW(cb, 5, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	cb.Emit32(arm64ORR_W(3, 3, 7))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 31, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64AbsIndexedRMW(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, decrement bool, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffX))
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 6, 1)
	if decrement {
		cb.Emit32(arm64SUB_W(1, 1, 6))
	} else {
		cb.Emit32(arm64ADD_W(1, 1, 6))
	}
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	cb.Emit32(arm64LSR_W_imm(6, 4, 8))
	emitP65ARM64SetNZ(cb)
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 6, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64AbsIndexedRMWShift(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffX))
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	switch instr.opcode {
	case 0x1E: // ASL abs,X
		cb.Emit32(arm64LSR_W_imm(7, 1, 7))
		p65ARM64MovW(cb, 6, 1)
		cb.Emit32(arm64LSL_W(1, 1, 6))
	case 0x5E: // LSR abs,X
		p65ARM64MovW(cb, 6, 1)
		cb.Emit32(arm64AND_W(7, 1, 6))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
	case 0x3E: // ROL abs,X
		cb.Emit32(arm64LSR_W_imm(7, 1, 7))
		p65ARM64MovW(cb, 6, CARRY_FLAG)
		cb.Emit32(arm64AND_W(6, 3, 6))
		p65ARM64MovW(cb, 3, 1)
		cb.Emit32(arm64LSL_W(1, 1, 3))
		cb.Emit32(arm64ORR_W(1, 1, 6))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	case 0x7E: // ROR abs,X
		p65ARM64MovW(cb, 6, CARRY_FLAG)
		cb.Emit32(arm64AND_W(7, 1, 6))
		cb.Emit32(arm64AND_W(6, 3, 6))
		p65ARM64MovW(cb, 3, 7)
		cb.Emit32(arm64LSL_W(6, 6, 3))
		cb.Emit32(arm64LSR_W_imm(1, 1, 1))
		cb.Emit32(arm64ORR_W(1, 1, 6))
		cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	}
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	cb.Emit32(arm64LSR_W_imm(6, 4, 8))
	p65ARM64MovW(cb, 5, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 5))
	cb.Emit32(arm64ORR_W(3, 3, 7))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 6, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedLoad(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset, destOffset uint32, cycles uint32) {
	// The address wraps in zero page, so a single page-zero mapping guard is
	// sufficient regardless of the runtime index value.
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, indexOffset))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 6, 4))
	cb.Emit32(arm64STRB_imm(1, 2, destOffset))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedLogic(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, op p65ARM64LogicOp, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 6, 4))
	emitP65ARM64LogicOp(cb, op)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedArithmetic(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, decimalTableOffset, binaryTableOffset, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	emitP65ARM64ArithmeticLoadedOperand(cb, decimalTableOffset, binaryTableOffset)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedCompare(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffA))
	cb.Emit32(arm64SUB_W(4, 3, 1))
	cb.Emit32(arm64CMP_W(3, 1))
	carry := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHS, 0))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	carryPC := cb.Len()
	cb.PatchUint32(carry, arm64Bcond(arm64CondHS, int32(carryPC-carry)))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, CARRY_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64ZPIndexedStore(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset, sourceOffset uint32, cycles uint32) {
	guard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDRB_imm(4, 2, indexOffset))
	p65ARM64MovW(cb, 5, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(4, 4, 5))
	p65ARM64MovW(cb, 5, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 5))
	cb.Emit32(arm64LDR_imm(6, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_imm(1, 2, sourceOffset))
	cb.Emit32(arm64STRB_reg(1, 6, 4))
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	p65ARM64StoreW(cb, 31, 0, j65CtxOffInvalPage) // zero page
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, cycles, false)
	patchP65ARM64GuardToBail(cb, guard)
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64AbsIndexedStore(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset uint32, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, indexOffset))
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffA))
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	cb.Emit32(arm64LSR_W_imm(6, 4, 8))
	p65ARM64StoreW(cb, 6, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64IndirectStore lowers STA (zp,X) and STA (zp),Y. Both pointer
// bytes are read from zero page with NMOS wraparound, then the resolved store
// page is checked before the first guest-visible write.
func emitP65ARM64IndirectStore(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexedX bool) {
	zpGuard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	if indexedX {
		cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
		p65ARM64MovW(cb, 6, uint32(byte(instr.operand)))
		cb.Emit32(arm64ADD_W(4, 4, 6))
	} else {
		p65ARM64MovW(cb, 4, uint32(byte(instr.operand)))
	}
	p65ARM64MovW(cb, 6, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	p65ARM64MovW(cb, 6, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, 8)
	cb.Emit32(arm64LSL_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(4, 1, 3))
	if !indexedX {
		cb.Emit32(arm64LDRB_imm(6, 2, cpu6502OffY))
		cb.Emit32(arm64ADD_W(4, 4, 6))
		p65ARM64MovW(cb, 6, 0xFFFF)
		cb.Emit32(arm64AND_W(4, 4, 6))
	}
	destGuard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_imm(1, 2, cpu6502OffA))
	cb.Emit32(arm64STRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 1, 1)
	p65ARM64StoreW(cb, 1, 0, j65CtxOffNeedInval)
	cb.Emit32(arm64LSR_W_imm(6, 4, 8))
	p65ARM64StoreW(cb, 6, 0, j65CtxOffInvalPage)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, 6, false)
	bailPC := cb.Len()
	cb.PatchUint32(destGuard, arm64CBNZ(6, int32(bailPC-destGuard)))
	cb.PatchUint32(zpGuard, arm64CBNZ(4, int32(bailPC-zpGuard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64IndirectLoad(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexedX bool) {
	zpGuard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	if indexedX {
		cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
		p65ARM64MovW(cb, 6, uint32(byte(instr.operand)))
		cb.Emit32(arm64ADD_W(4, 4, 6))
	} else {
		p65ARM64MovW(cb, 4, uint32(byte(instr.operand)))
	}
	p65ARM64MovW(cb, 6, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	p65ARM64MovW(cb, 6, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, 8)
	cb.Emit32(arm64LSL_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(4, 1, 3))
	if !indexedX {
		cb.Emit32(arm64LDRB_imm(6, 2, cpu6502OffY))
		cb.Emit32(arm64ADD_W(4, 4, 6))
		p65ARM64MovW(cb, 6, 0xFFFF)
		cb.Emit32(arm64AND_W(4, 4, 6))
	}
	destGuard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64STRB_imm(1, 2, cpu6502OffA))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, uint32(jit6502BaseCycles[instr.opcode]), false)
	bailPC := cb.Len()
	cb.PatchUint32(destGuard, arm64CBNZ(6, int32(bailPC-destGuard)))
	cb.PatchUint32(zpGuard, arm64CBNZ(4, int32(bailPC-zpGuard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64IndirectAddress leaves the resolved 16-bit address in W4 and
// returns guards for its zero-page pointer fetch and resolved memory page.
// The caller must patch both guards to the same no-side-effect bailout.
func emitP65ARM64IndirectAddress(cb *CodeBuffer, instr JIT6502Instr, indexedX bool) (int, int) {
	zpGuard := emitP65ARM64DirectPageGuard(cb, 0)
	emitP65ARM64LoadCPU(cb)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	if indexedX {
		cb.Emit32(arm64LDRB_imm(4, 2, cpu6502OffX))
		p65ARM64MovW(cb, 6, uint32(byte(instr.operand)))
		cb.Emit32(arm64ADD_W(4, 4, 6))
	} else {
		p65ARM64MovW(cb, 4, uint32(byte(instr.operand)))
	}
	p65ARM64MovW(cb, 6, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	p65ARM64MovW(cb, 6, 1)
	cb.Emit32(arm64ADD_W(4, 4, 6))
	p65ARM64MovW(cb, 6, 0xFF)
	cb.Emit32(arm64AND_W(4, 4, 6))
	cb.Emit32(arm64LDRB_reg(3, 5, 4))
	p65ARM64MovW(cb, 6, 8)
	cb.Emit32(arm64LSL_W(3, 3, 6))
	cb.Emit32(arm64ORR_W(4, 1, 3))
	if !indexedX {
		cb.Emit32(arm64LDRB_imm(6, 2, cpu6502OffY))
		cb.Emit32(arm64ADD_W(4, 4, 6))
		p65ARM64MovW(cb, 6, 0xFFFF)
		cb.Emit32(arm64AND_W(4, 4, 6))
	}
	return zpGuard, emitP65ARM64DynamicPageGuard(cb, 4)
}

func emitP65ARM64IndirectLogic(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexedX bool, op p65ARM64LogicOp) {
	zpGuard, destGuard := emitP65ARM64IndirectAddress(cb, instr, indexedX)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	emitP65ARM64LogicOp(cb, op)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, uint32(jit6502BaseCycles[instr.opcode]), false)
	bailPC := cb.Len()
	cb.PatchUint32(destGuard, arm64CBNZ(6, int32(bailPC-destGuard)))
	cb.PatchUint32(zpGuard, arm64CBNZ(4, int32(bailPC-zpGuard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64IndirectArithmetic(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexedX bool, decimalTableOffset, binaryTableOffset uint32) {
	zpGuard, destGuard := emitP65ARM64IndirectAddress(cb, instr, indexedX)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	emitP65ARM64ArithmeticLoadedOperand(cb, decimalTableOffset, binaryTableOffset)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, uint32(jit6502BaseCycles[instr.opcode]), false)
	bailPC := cb.Len()
	cb.PatchUint32(destGuard, arm64CBNZ(6, int32(bailPC-destGuard)))
	cb.PatchUint32(zpGuard, arm64CBNZ(4, int32(bailPC-zpGuard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64IndirectCompare(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexedX bool) {
	zpGuard, destGuard := emitP65ARM64IndirectAddress(cb, instr, indexedX)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffA))
	cb.Emit32(arm64SUB_W(4, 3, 1))
	cb.Emit32(arm64CMP_W(3, 1))
	carryOffset := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHS, 0))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, uint32(jit6502BaseCycles[instr.opcode]), false)
	carryPC := cb.Len()
	cb.PatchUint32(carryOffset, arm64Bcond(arm64CondHS, int32(carryPC-carryOffset)))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, CARRY_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	emitP65ARM64Return(cb, uint32(startPC)+2, 1, uint32(jit6502BaseCycles[instr.opcode]), false)
	bailPC := cb.Len()
	cb.PatchUint32(destGuard, arm64CBNZ(6, int32(bailPC-destGuard)))
	cb.PatchUint32(zpGuard, arm64CBNZ(4, int32(bailPC-zpGuard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64AbsIndexedLoad(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset, destOffset uint32, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, indexOffset))
	p65ARM64MovW(cb, 7, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(7, 7, 1))
	cb.Emit32(arm64LSR_W_imm(7, 7, 8)) // page-cross boolean
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64STRB_imm(1, 2, destOffset))
	emitP65ARM64SetNZ(cb)
	noCross := cb.Len()
	cb.Emit32(arm64CBZ(7, 0))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles+1, false)
	noCrossPC := cb.Len()
	cb.PatchUint32(noCross, arm64CBZ(7, int32(noCrossPC-noCross)))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64AbsIndexedArithmetic(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset, decimalTableOffset, binaryTableOffset, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, indexOffset))
	p65ARM64MovW(cb, 7, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(7, 7, 1))
	cb.Emit32(arm64LSR_W_imm(7, 7, 8))
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	emitP65ARM64ArithmeticLoadedOperand(cb, decimalTableOffset, binaryTableOffset)
	noCross := cb.Len()
	cb.Emit32(arm64CBZ(7, 0))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles+1, false)
	noCrossPC := cb.Len()
	cb.PatchUint32(noCross, arm64CBZ(7, int32(noCrossPC-noCross)))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

func emitP65ARM64AbsIndexedLogic(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset uint32, op p65ARM64LogicOp, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, indexOffset))
	p65ARM64MovW(cb, 7, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(7, 7, 1))
	cb.Emit32(arm64LSR_W_imm(7, 7, 8))
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	emitP65ARM64LogicOp(cb, op)
	noCross := cb.Len()
	cb.Emit32(arm64CBZ(7, 0))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles+1, false)
	noCrossPC := cb.Len()
	cb.PatchUint32(noCross, arm64CBZ(7, int32(noCrossPC-noCross)))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// emitP65ARM64AbsIndexedCompare lowers CMP abs,X and CMP abs,Y, including the
// NMOS page-cross cycle. W7 retains the crossing bit while the compare paths
// update the guest C/N/Z flags.
func emitP65ARM64AbsIndexedCompare(cb *CodeBuffer, startPC uint16, instr JIT6502Instr, indexOffset, cycles uint32) {
	emitP65ARM64LoadCPU(cb)
	p65ARM64MovW(cb, 4, uint32(instr.operand))
	cb.Emit32(arm64LDRB_imm(1, 2, indexOffset))
	p65ARM64MovW(cb, 7, uint32(byte(instr.operand)))
	cb.Emit32(arm64ADD_W(7, 7, 1))
	cb.Emit32(arm64LSR_W_imm(7, 7, 8))
	cb.Emit32(arm64ADD_W(4, 4, 1))
	p65ARM64MovW(cb, 1, 0xFFFF)
	cb.Emit32(arm64AND_W(4, 4, 1))
	guard := emitP65ARM64DynamicPageGuard(cb, 4)
	cb.Emit32(arm64LDR_imm(5, 0, j65CtxOffMemPtr/8))
	cb.Emit32(arm64LDRB_reg(1, 5, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffA))
	cb.Emit32(arm64SUB_W(4, 3, 1))
	cb.Emit32(arm64CMP_W(3, 1))
	carry := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHS, 0))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, ^uint32(CARRY_FLAG))
	cb.Emit32(arm64AND_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	noCross := cb.Len()
	cb.Emit32(arm64CBZ(7, 0))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles+1, false)
	noCrossPC := cb.Len()
	cb.PatchUint32(noCross, arm64CBZ(7, int32(noCrossPC-noCross)))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	carryPC := cb.Len()
	cb.PatchUint32(carry, arm64Bcond(arm64CondHS, int32(carryPC-carry)))
	cb.Emit32(arm64MOV_W(1, 4))
	cb.Emit32(arm64LDRB_imm(3, 2, cpu6502OffSR))
	p65ARM64MovW(cb, 4, CARRY_FLAG)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STRB_imm(3, 2, cpu6502OffSR))
	emitP65ARM64SetNZ(cb)
	noCross = cb.Len()
	cb.Emit32(arm64CBZ(7, 0))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles+1, false)
	noCrossPC = cb.Len()
	cb.PatchUint32(noCross, arm64CBZ(7, int32(noCrossPC-noCross)))
	emitP65ARM64Return(cb, uint32(startPC)+3, 1, cycles, false)
	bailPC := cb.Len()
	cb.PatchUint32(guard, arm64CBNZ(6, int32(bailPC-guard)))
	emitP65ARM64Return(cb, uint32(startPC), 0, 0, true)
}

// compileBlock6502 emits one ARM64 instruction boundary at a time. This keeps
// the fallback contract exact while the lowering grows: each native block is
// either a complete semantic lowering or exits with NeedBail before touching
// guest state. The scanner may provide a larger candidate block, but retaining
// only its first instruction prevents a later unlowered form from becoming an
// implicit native side effect.
func compileBlock6502(instrs []JIT6502Instr, startPC uint16, execMem *ExecMem, codePageBitmap *[256]byte) (*JITBlock, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("ARM64 6502: empty block")
	}
	instr := instrs[0]
	var cb CodeBuffer
	chainable := false
	switch instr.opcode {
	case 0xEA: // NOP: complete native semantic lowering
		emitP65ARM64Return(&cb, uint32(startPC)+1, 1, 2, false)
		chainable = true
	case 0x4C: // JMP absolute
		emitP65ARM64Return(&cb, uint32(instr.operand), 1, 3, false)
	case 0x20: // JSR absolute
		emitP65ARM64JSR(&cb, startPC, instr.operand)
	case 0x60: // RTS
		emitP65ARM64RTS(&cb, startPC)
	case 0x6C: // JMP indirect, preserving the NMOS page-wrap behaviour
		emitP65ARM64JMPIndirect(&cb, startPC, instr.operand)
	case 0x40: // RTI
		emitP65ARM64RTI(&cb, startPC)
	case 0x00: // BRK (debug observers are dispatched before native entry)
		emitP65ARM64BRK(&cb, startPC)
	case 0x48, 0x68, 0x08, 0x28: // PHA, PLA, PHP, PLP
		emitP65ARM64Stack(&cb, startPC, instr.opcode)
	case 0x90: // BCC
		emitP65ARM64Branch(&cb, startPC, instr, CARRY_FLAG, false)
	case 0xB0: // BCS
		emitP65ARM64Branch(&cb, startPC, instr, CARRY_FLAG, true)
	case 0xF0: // BEQ
		emitP65ARM64Branch(&cb, startPC, instr, ZERO_FLAG, true)
	case 0xD0: // BNE
		emitP65ARM64Branch(&cb, startPC, instr, ZERO_FLAG, false)
	case 0x30: // BMI
		emitP65ARM64Branch(&cb, startPC, instr, NEGATIVE_FLAG, true)
	case 0x10: // BPL
		emitP65ARM64Branch(&cb, startPC, instr, NEGATIVE_FLAG, false)
	case 0x70: // BVS
		emitP65ARM64Branch(&cb, startPC, instr, OVERFLOW_FLAG, true)
	case 0x50: // BVC
		emitP65ARM64Branch(&cb, startPC, instr, OVERFLOW_FLAG, false)
	case 0xA9: // LDA #imm
		emitP65ARM64LoadImmediate(&cb, startPC, byte(instr.operand), cpu6502OffA)
		chainable = true
	case 0xA2: // LDX #imm
		emitP65ARM64LoadImmediate(&cb, startPC, byte(instr.operand), cpu6502OffX)
		chainable = true
	case 0xA0: // LDY #imm
		emitP65ARM64LoadImmediate(&cb, startPC, byte(instr.operand), cpu6502OffY)
		chainable = true
	case 0x69: // ADC #imm
		emitP65ARM64DecimalImmediate(&cb, startPC, byte(instr.operand), j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr)
	case 0xE9: // SBC #imm
		emitP65ARM64DecimalImmediate(&cb, startPC, byte(instr.operand), j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr)
	case 0x29: // AND #imm
		emitP65ARM64LogicImmediate(&cb, startPC, byte(instr.operand), p65ARM64And)
	case 0x09: // ORA #imm
		emitP65ARM64LogicImmediate(&cb, startPC, byte(instr.operand), p65ARM64Ora)
	case 0x49: // EOR #imm
		emitP65ARM64LogicImmediate(&cb, startPC, byte(instr.operand), p65ARM64Eor)
	case 0xC9: // CMP #imm
		emitP65ARM64CompareImmediate(&cb, startPC, byte(instr.operand), cpu6502OffA)
	case 0xE0: // CPX #imm
		emitP65ARM64CompareImmediate(&cb, startPC, byte(instr.operand), cpu6502OffX)
	case 0xC0: // CPY #imm
		emitP65ARM64CompareImmediate(&cb, startPC, byte(instr.operand), cpu6502OffY)
	case 0xC5, 0xCD: // CMP zp, abs
		emitP65ARM64CompareDirect(&cb, startPC, instr, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xD5: // CMP zp,X
		emitP65ARM64ZPIndexedCompare(&cb, startPC, instr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xDD: // CMP abs,X
		emitP65ARM64AbsIndexedCompare(&cb, startPC, instr, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xD9: // CMP abs,Y
		emitP65ARM64AbsIndexedCompare(&cb, startPC, instr, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xE4, 0xEC: // CPX zp, abs
		emitP65ARM64CompareDirect(&cb, startPC, instr, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xC4, 0xCC: // CPY zp, abs
		emitP65ARM64CompareDirect(&cb, startPC, instr, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x65, 0x6D: // ADC zp, abs
		emitP65ARM64ArithmeticDirect(&cb, startPC, instr, j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x75: // ADC zp,X
		emitP65ARM64ZPIndexedArithmetic(&cb, startPC, instr, j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x7D: // ADC abs,X
		emitP65ARM64AbsIndexedArithmetic(&cb, startPC, instr, cpu6502OffX, j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x79: // ADC abs,Y
		emitP65ARM64AbsIndexedArithmetic(&cb, startPC, instr, cpu6502OffY, j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xE5, 0xED: // SBC zp, abs
		emitP65ARM64ArithmeticDirect(&cb, startPC, instr, j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xF5: // SBC zp,X
		emitP65ARM64ZPIndexedArithmetic(&cb, startPC, instr, j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xFD: // SBC abs,X
		emitP65ARM64AbsIndexedArithmetic(&cb, startPC, instr, cpu6502OffX, j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xF9: // SBC abs,Y
		emitP65ARM64AbsIndexedArithmetic(&cb, startPC, instr, cpu6502OffY, j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x25, 0x2D: // AND zp, abs
		emitP65ARM64LogicDirect(&cb, startPC, instr, p65ARM64And, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x05, 0x0D: // ORA zp, abs
		emitP65ARM64LogicDirect(&cb, startPC, instr, p65ARM64Ora, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x45, 0x4D: // EOR zp, abs
		emitP65ARM64LogicDirect(&cb, startPC, instr, p65ARM64Eor, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x35: // AND zp,X
		emitP65ARM64ZPIndexedLogic(&cb, startPC, instr, p65ARM64And, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x15: // ORA zp,X
		emitP65ARM64ZPIndexedLogic(&cb, startPC, instr, p65ARM64Ora, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x55: // EOR zp,X
		emitP65ARM64ZPIndexedLogic(&cb, startPC, instr, p65ARM64Eor, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x1D, 0x19:
		index := uint32(cpu6502OffX)
		if instr.opcode == 0x19 {
			index = cpu6502OffY
		}
		emitP65ARM64AbsIndexedLogic(&cb, startPC, instr, index, p65ARM64Ora, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x3D, 0x39:
		index := uint32(cpu6502OffX)
		if instr.opcode == 0x39 {
			index = cpu6502OffY
		}
		emitP65ARM64AbsIndexedLogic(&cb, startPC, instr, index, p65ARM64And, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x5D, 0x59:
		index := uint32(cpu6502OffX)
		if instr.opcode == 0x59 {
			index = cpu6502OffY
		}
		emitP65ARM64AbsIndexedLogic(&cb, startPC, instr, index, p65ARM64Eor, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x24, 0x2C: // BIT zp, abs
		emitP65ARM64BITDirect(&cb, startPC, instr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xE6, 0xEE: // INC zp, abs
		emitP65ARM64RMWDirect(&cb, startPC, instr, false, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xC6, 0xCE: // DEC zp, abs
		emitP65ARM64RMWDirect(&cb, startPC, instr, true, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xF6: // INC zp,X
		emitP65ARM64ZPIndexedRMW(&cb, startPC, instr, false, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xD6: // DEC zp,X
		emitP65ARM64ZPIndexedRMW(&cb, startPC, instr, true, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xFE: // INC abs,X
		emitP65ARM64AbsIndexedRMW(&cb, startPC, instr, false, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xDE: // DEC abs,X
		emitP65ARM64AbsIndexedRMW(&cb, startPC, instr, true, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x16, 0x36, 0x56, 0x76: // ASL, ROL, LSR, ROR zp,X
		emitP65ARM64ZPIndexedRMWShift(&cb, startPC, instr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x1E, 0x3E, 0x5E, 0x7E: // ASL, ROL, LSR, ROR abs,X
		emitP65ARM64AbsIndexedRMWShift(&cb, startPC, instr, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x06, 0x0E, 0x46, 0x4E, 0x26, 0x2E, 0x66, 0x6E:
		emitP65ARM64RMWShiftDirect(&cb, startPC, instr, instr.opcode, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xA5, 0xAD: // LDA zp, abs
		emitP65ARM64DirectLoad(&cb, startPC, instr, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xA6, 0xAE: // LDX zp, abs
		emitP65ARM64DirectLoad(&cb, startPC, instr, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xA4, 0xAC: // LDY zp, abs
		emitP65ARM64DirectLoad(&cb, startPC, instr, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xB5: // LDA zp,X
		emitP65ARM64ZPIndexedLoad(&cb, startPC, instr, cpu6502OffX, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xB6: // LDX zp,Y
		emitP65ARM64ZPIndexedLoad(&cb, startPC, instr, cpu6502OffY, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xB4: // LDY zp,X
		emitP65ARM64ZPIndexedLoad(&cb, startPC, instr, cpu6502OffX, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x95: // STA zp,X
		emitP65ARM64ZPIndexedStore(&cb, startPC, instr, cpu6502OffX, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x94: // STY zp,X
		emitP65ARM64ZPIndexedStore(&cb, startPC, instr, cpu6502OffX, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x96: // STX zp,Y
		emitP65ARM64ZPIndexedStore(&cb, startPC, instr, cpu6502OffY, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x85, 0x8D: // STA zp, abs
		emitP65ARM64DirectStore(&cb, startPC, instr, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x86, 0x8E: // STX zp, abs
		emitP65ARM64DirectStore(&cb, startPC, instr, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x84, 0x8C: // STY zp, abs
		emitP65ARM64DirectStore(&cb, startPC, instr, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x9D: // STA abs,X
		emitP65ARM64AbsIndexedStore(&cb, startPC, instr, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x99: // STA abs,Y
		emitP65ARM64AbsIndexedStore(&cb, startPC, instr, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0x81: // STA (zp,X)
		emitP65ARM64IndirectStore(&cb, startPC, instr, true)
	case 0x91: // STA (zp),Y
		emitP65ARM64IndirectStore(&cb, startPC, instr, false)
	case 0xA1: // LDA (zp,X)
		emitP65ARM64IndirectLoad(&cb, startPC, instr, true)
	case 0xB1: // LDA (zp),Y
		emitP65ARM64IndirectLoad(&cb, startPC, instr, false)
	case 0x01: // ORA (zp,X)
		emitP65ARM64IndirectLogic(&cb, startPC, instr, true, p65ARM64Ora)
	case 0x11: // ORA (zp),Y
		emitP65ARM64IndirectLogic(&cb, startPC, instr, false, p65ARM64Ora)
	case 0x21: // AND (zp,X)
		emitP65ARM64IndirectLogic(&cb, startPC, instr, true, p65ARM64And)
	case 0x31: // AND (zp),Y
		emitP65ARM64IndirectLogic(&cb, startPC, instr, false, p65ARM64And)
	case 0x41: // EOR (zp,X)
		emitP65ARM64IndirectLogic(&cb, startPC, instr, true, p65ARM64Eor)
	case 0x51: // EOR (zp),Y
		emitP65ARM64IndirectLogic(&cb, startPC, instr, false, p65ARM64Eor)
	case 0x61: // ADC (zp,X)
		emitP65ARM64IndirectArithmetic(&cb, startPC, instr, true, j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr)
	case 0x71: // ADC (zp),Y
		emitP65ARM64IndirectArithmetic(&cb, startPC, instr, false, j65CtxOffDecimalADCPtr, j65CtxOffBinaryADCPtr)
	case 0xE1: // SBC (zp,X)
		emitP65ARM64IndirectArithmetic(&cb, startPC, instr, true, j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr)
	case 0xF1: // SBC (zp),Y
		emitP65ARM64IndirectArithmetic(&cb, startPC, instr, false, j65CtxOffDecimalSBCPtr, j65CtxOffBinarySBCPtr)
	case 0xC1: // CMP (zp,X)
		emitP65ARM64IndirectCompare(&cb, startPC, instr, true)
	case 0xD1: // CMP (zp),Y
		emitP65ARM64IndirectCompare(&cb, startPC, instr, false)
	case 0xBD: // LDA abs,X
		emitP65ARM64AbsIndexedLoad(&cb, startPC, instr, cpu6502OffX, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xB9: // LDA abs,Y
		emitP65ARM64AbsIndexedLoad(&cb, startPC, instr, cpu6502OffY, cpu6502OffA, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xBE: // LDX abs,Y
		emitP65ARM64AbsIndexedLoad(&cb, startPC, instr, cpu6502OffY, cpu6502OffX, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xBC: // LDY abs,X
		emitP65ARM64AbsIndexedLoad(&cb, startPC, instr, cpu6502OffX, cpu6502OffY, uint32(jit6502BaseCycles[instr.opcode]))
	case 0xAA: // TAX
		emitP65ARM64Transfer(&cb, startPC, cpu6502OffA, cpu6502OffX)
		chainable = true
	case 0xA8: // TAY
		emitP65ARM64Transfer(&cb, startPC, cpu6502OffA, cpu6502OffY)
		chainable = true
	case 0x8A: // TXA
		emitP65ARM64Transfer(&cb, startPC, cpu6502OffX, cpu6502OffA)
		chainable = true
	case 0x98: // TYA
		emitP65ARM64Transfer(&cb, startPC, cpu6502OffY, cpu6502OffA)
		chainable = true
	case 0xBA: // TSX
		emitP65ARM64Transfer(&cb, startPC, cpu6502OffSP, cpu6502OffX)
		chainable = true
	case 0x9A: // TXS (does not update N/Z)
		emitP65ARM64TransferNoFlags(&cb, startPC, cpu6502OffX, cpu6502OffSP)
		chainable = true
	case 0xE8: // INX
		emitP65ARM64IncDec(&cb, startPC, cpu6502OffX, false)
		chainable = true
	case 0xC8: // INY
		emitP65ARM64IncDec(&cb, startPC, cpu6502OffY, false)
		chainable = true
	case 0xCA: // DEX
		emitP65ARM64IncDec(&cb, startPC, cpu6502OffX, true)
		chainable = true
	case 0x88: // DEY
		emitP65ARM64IncDec(&cb, startPC, cpu6502OffY, true)
		chainable = true
	case 0x0A, 0x4A, 0x2A, 0x6A: // ASL, LSR, ROL, ROR A
		emitP65ARM64AccumulatorShift(&cb, startPC, instr.opcode)
	case 0x18: // CLC
		emitP65ARM64Flag(&cb, startPC, CARRY_FLAG, false)
		chainable = true
	case 0x38: // SEC
		emitP65ARM64Flag(&cb, startPC, CARRY_FLAG, true)
		chainable = true
	case 0x58: // CLI
		emitP65ARM64Flag(&cb, startPC, INTERRUPT_FLAG, false)
		chainable = true
	case 0x78: // SEI
		emitP65ARM64Flag(&cb, startPC, INTERRUPT_FLAG, true)
		chainable = true
	case 0xB8: // CLV
		emitP65ARM64Flag(&cb, startPC, OVERFLOW_FLAG, false)
		chainable = true
	case 0xD8: // CLD
		emitP65ARM64Flag(&cb, startPC, DECIMAL_FLAG, false)
		chainable = true
	case 0xF8: // SED
		emitP65ARM64Flag(&cb, startPC, DECIMAL_FLAG, true)
		chainable = true
	default:
		// Exact interpreter bailout: no 6502 register or memory write occurs
		// before the dispatcher restores instrPC and executes the opcode.
		emitP65ARM64Return(&cb, uint32(startPC), 0, 0, true)
	}

	chainPatchOff, chainFallbackOff := 0, 0
	if chainable {
		chainPatchOff, chainFallbackOff = emitP65ARM64ChainTail(&cb)
	}
	addr, err := execMem.Write(cb.Bytes())
	if err != nil {
		return nil, fmt.Errorf("ARM64 6502 block: %w", err)
	}
	// ExecMem.Write already performs the dual-alias cache sequence: clean the
	// writable mapping, then invalidate the executable mapping. A second
	// single-address flush on the RX alias can clean an older alias cache line
	// back over newly emitted code, which manifests as a QEMU fall-through past
	// RET on reused executable pages.

	source := make([]byte, instr.length)
	source[0] = instr.opcode
	if instr.length >= 2 {
		source[1] = byte(instr.operand)
	}
	if instr.length == 3 {
		source[2] = byte(instr.operand >> 8)
	}
	endPC := uint64(startPC) + uint64(instr.length)
	block := &JITBlock{
		startPC:    uint64(startPC),
		endPC:      endPC,
		instrCount: 1,
		execAddr:   addr,
		execSize:   len(cb.Bytes()),
		p65Source:  source,
	}
	if chainable {
		patchAddr := addr + uintptr(chainPatchOff)
		block.chainEntry = addr
		block.chainSlots = []chainSlot{{
			targetPC:     endPC,
			patchAddr:    patchAddr,
			fallbackAddr: addr + uintptr(chainFallbackOff),
			patch: func(target uintptr) {
				patchP65ARM64ChainBranch(patchAddr, target)
			},
		}}
	}
	if codePageBitmap != nil {
		codePageBitmap[startPC>>8] = 1
	}
	return block, nil
}
