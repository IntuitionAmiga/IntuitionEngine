// jit_ie64_fpsr_liveness.go - backward FPSR condition-code liveness for the
// IE64 amd64 JIT (Technique 3 of IE64_JIT_PERFORMANCE_PLAN.md).
//
// Several IE64 FP instructions update the FPSR condition-code field
// (bits 27:24) after producing their result. That update is dead when a
// later, guaranteed-non-faulting FP instruction overwrites the whole CC
// field before any observer reads it. This pass marks such writes so the
// emitter can elide emitSetFPCondCodes* for them.
//
// Sticky exception flags (bits 3:0) are never touched here: the plan
// keeps them eager, and only the CC field is analysed.
//
// Observability model matches the rest of the IE64 amd64 JIT: a native
// block runs to completion, so interrupts and traps are only serviced at
// block boundaries. The only in-block FPSR reader is FMOVSR. Any
// instruction that can fault/trap/bail (memory FP ops, transcendentals,
// system opcodes) is treated as an observer because a resulting trap
// handler could read the stale CC field. The block end is always an
// observer (initial live = true), which also covers regions safely: the
// pass runs per sub-block, so cross-block CC elision is simply forgone,
// never mis-applied.

//go:build amd64 && (linux || windows || darwin)

package main

// ie64FPSRCCWriterElidable reports whether the amd64 emitter appends an
// emitSetFPCondCodes* update to this opcode that can be skipped when the
// CC write is proven dead. These are exactly the FP opcodes whose
// emitters call emitSetFPCondCodes(64)AMD64 as their final step.
func ie64FPSRCCWriterElidable(op byte) bool {
	switch op {
	case OP_FABS, OP_FNEG, OP_FMOVI, OP_FMOVECR, OP_FSQRT, OP_FINT, OP_FCVTIF, OP_FLOAD:
		return true
	// FP64: emitXMMToDPairAMD64 appends the CC update to these, and DLOAD's
	// native success path sets it directly. Unlike FP32 binary ops, FP64
	// binary ops do update the CC field in the amd64 JIT.
	case OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV, OP_DINT, OP_DCVTIF, OP_DLOAD:
		return true
	}
	return false
}

// ie64FPSRCCKiller reports whether this opcode unconditionally overwrites
// the entire FPSR CC field and cannot fault before doing so. Such an
// instruction kills any earlier CC write: the earlier value can never be
// observed. Only register-only, non-faulting CC writers qualify. FLOAD is
// deliberately excluded — it can fault, so an earlier CC write must
// survive for a trap handler.
func ie64FPSRCCKiller(op byte) bool {
	switch op {
	case OP_FABS, OP_FNEG, OP_FMOVI, OP_FMOVECR, OP_FSQRT, OP_FINT, OP_FCVTIF, OP_FMOVSC:
		return true
	}
	// No FP64 op qualifies as a killer. Every native FP64 CC writer also has a
	// runtime non-finite (or compile-time invalid-register) bail to the
	// interpreter. On bail the interpreter loop delivers any pending external
	// interrupt *before* re-executing the bailed PC, so its handler could
	// observe an earlier CC field. Treating an FP64 op as an unconditional
	// killer would let us wrongly elide that earlier, observable write.
	return false
}

// ie64FPSRTransparent reports whether this opcode neither reads nor writes
// the FPSR CC field and cannot fault/trap/bail. Such instructions are
// invisible to CC liveness: they leave the live state unchanged. Anything
// not transparent and not a killer is treated as an observer barrier.
func ie64FPSRTransparent(op byte) bool {
	switch op {
	// Integer register/immediate ALU: no memory access, no trap.
	case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA,
		OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_MULHU, OP_MULHS, OP_NEG,
		OP_AND64, OP_OR64, OP_EOR, OP_NOT64,
		OP_LSL, OP_LSR, OP_ASR, OP_CLZ, OP_SEXT, OP_ROL, OP_ROR,
		OP_CTZ, OP_POPCNT, OP_BSWAP, OP_NOP64:
		return true
	// FP ops that touch neither FPSR CC nor memory: register copies and
	// FPCR moves. (FMOVSR reads FPSR and is an observer; FMOVSC writes it
	// and is a killer — both excluded here.)
	case OP_FMOV, OP_FMOVO, OP_FMOVCR, OP_FMOVCC:
		return true
	}
	// FP64 ops are deliberately absent. Even CC-neutral ones such as DMOV can
	// bail (invalid register pair), and a bail hands control to the interpreter
	// which may deliver an interrupt before continuing. A transparent op must
	// never break that way, so FP64 ops stay barriers.
	return false
}

// ie64MarkFPSRCCDead runs a single backward pass over a block's decoded
// instructions and sets fpsrCCDead on every elidable CC writer whose
// output is overwritten before any observer reads it.
func ie64MarkFPSRCCDead(instrs []JITInstr) {
	live := true // block end is an observer (interrupts/traps at boundary)
	for i := len(instrs) - 1; i >= 0; i-- {
		op := instrs[i].opcode
		if !live && ie64FPSRCCWriterElidable(op) {
			instrs[i].fpsrCCDead = true
		}
		switch {
		case ie64FPSRTransparent(op):
			// live unchanged
		case ie64FPSRCCKiller(op):
			live = false
		default:
			live = true // observer / faulting / unknown barrier
		}
	}
}
