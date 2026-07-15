// jit_ie64_fpsr_liveness.go - backward FPSR condition-code liveness for the
// IE64 amd64 and arm64 JITs (Technique 3 of IE64_JIT_PERFORMANCE_PLAN.md).
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
// Both backends emit the same set of CC writers and share this analysis.
// They differ only in what they do with the result: amd64 honours both
// fpsrCCDead and fpsrCCSink, arm64 honours fpsrCCDead alone (it has no exit
// -funnel materialisation, so a sunk write falls back to an inline update).
// The two flags are set on disjoint cases, so honouring one without the
// other is sound.
//
// Observability model matches the rest of the IE64 JIT on both backends: a
// native block runs to completion, so interrupts and traps are only serviced at
// block boundaries. The only in-block FPSR reader is FMOVSR. Any
// instruction that can fault/trap/bail (memory FP ops, transcendentals,
// system opcodes) is treated as an observer because a resulting trap
// handler could read the stale CC field. The block end is always an
// observer (initial live = true), which also covers regions safely: the
// pass runs per sub-block, so cross-block CC elision is simply forgone,
// never mis-applied.

//go:build (amd64 || arm64) && (linux || windows || darwin)

package main

// ie64FPSRCCWriterElidable reports whether the emitter appends an
// emitSetFPCondCodes* update to this opcode that can be skipped when the
// CC write is proven dead. These are exactly the FP opcodes whose emitters
// call emitSetFPCondCodes(64)AMD64, or emitFPCondCodes(64)ARM64, as their
// final step. The two backends' lists are identical; if they ever diverge,
// this predicate has to be split per backend rather than widened.
func ie64FPSRCCWriterElidable(op byte) bool {
	switch op {
	case OP_FABS, OP_FNEG, OP_FMOVI, OP_FMOVECR, OP_FSQRT, OP_FINT, OP_FCVTIF, OP_FLOAD,
		OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV:
		return true
	// FP64: emitXMMToDPairAMD64 appends the CC update to these, and DLOAD's
	// native success path sets it directly.
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
	case OP_FABS, OP_FNEG, OP_FMOVI, OP_FMOVECR, OP_FSQRT, OP_FINT, OP_FCVTIF, OP_FMOVSC,
		// FP32 binary ops qualify on both backends: each unconditionally
		// rewrites the whole CC field and cannot fault. On amd64 because SSE
		// runs with exceptions masked, so ADDSS/SUBSS/MULSS/DIVSS cannot trap;
		// on arm64 because neither the JIT nor the guest can enable an FP trap
		// — the emitter never writes the host FPCR (FMOVSC writes the guest's
		// FPCR field in the IE64FPU struct), so the host default of untrapped
		// exceptions stands. Their sticky exception updates are emitted eagerly
		// and are unaffected by CC elision.
		OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV:
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

// ie64FPSRCCSinkable reports whether an emitter can defer this opcode's CC
// update to the block's exit funnels. These are exactly the FP32 writers
// routed through emitFPCCUpdate32AMD64, whose classified value is the FP32
// register ji.rd and so can be reconstructed at an exit by re-reading it.
//
// Only amd64 acts on the resulting fpsrCCSink; arm64 has no funnel
// materialisation and emits those updates inline instead. That costs arm64
// performance, not correctness: sinking is an optimisation, and declining it
// leaves the ordinary in-place update.
//
// FLOAD and the FP64 writers are excluded: they still emit their CC update in
// place. Both are inline observers in this analysis anyway (they can fault or
// bail), so an earlier writer is never sunk across them and no pending update
// can be outstanding when they run.
func ie64FPSRCCSinkable(op byte) bool {
	switch op {
	case OP_FABS, OP_FNEG, OP_FMOVI, OP_FMOVECR, OP_FSQRT, OP_FINT, OP_FCVTIF,
		OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV:
		return true
	}
	return false
}

// ie64FPRegWriteOverlaps reports whether executing ji can change the contents
// of FP32 register slot reg.
//
// Anything not positively known to leave the FP register file alone is assumed
// to write it. FP64 ops, helper calls and unmodelled opcodes therefore answer
// true; they are all inline observers in this analysis, so the conservatism
// costs nothing.
func ie64FPRegWriteOverlaps(ji *JITInstr, reg byte) bool {
	switch ji.opcode {
	case OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV, OP_FABS, OP_FNEG, OP_FMOVI,
		OP_FMOVECR, OP_FSQRT, OP_FINT, OP_FCVTIF, OP_FLOAD, OP_FMOV:
		return ji.rd == reg
	}
	if ie64FPSRTransparent(ji.opcode) || ie64FPSRExitEdge(ji.opcode) {
		// Integer ALU, branches, and the GPR/FPCR-destined FP moves (FMOVO,
		// FMOVCR, FMOVCC) write no FP32 slot. FMOV does, and is caught above.
		return false
	}
	return true
}

// ie64CCSinkSafe reports whether the CC update of the writer at index w can
// safely be reconstructed at the block's exit funnels.
//
// Sinking rebuilds the CC by re-reading FPRegs[rd] at the exit, and the exit
// code is emitted at a fixed point in the instruction stream. Two things can
// break that, and both are rejected here:
//
//   - The value is overwritten before the exit reads it. A CC-transparent op
//     such as FMOV can rewrite rd without disturbing the pending update, so the
//     exit would classify the wrong value.
//
//   - An exit is reachable with the update pending but has no materialisation
//     emitted at it. The pending slot is a compile-time linear notion, so an
//     exit sitting *before* the writer carries no pending update at emit time,
//     yet a back-edge can reach it at run time on a later iteration with the
//     writer's CC outstanding. That covers branches and any faulting op whose
//     bail leaves through the epilogue.
func ie64CCSinkSafe(instrs []JITInstr, w int) bool {
	rd := instrs[w].rd
	for j := w + 1; j < len(instrs); j++ {
		if ie64FPRegWriteOverlaps(&instrs[j], rd) {
			return false
		}
	}
	// Any back-edge at or after w that lands at or before w re-runs the range
	// [t, w) with this update still pending.
	for j := w; j < len(instrs); j++ {
		t, ok := ie64CCBackEdgeTarget(instrs, j)
		if !ok || t > w {
			continue
		}
		for k := t; k < w; k++ {
			op := instrs[k].opcode
			if !ie64FPSRTransparent(op) && !ie64FPSRCCKiller(op) {
				return false // could leave the block with nothing materialised
			}
			if ie64FPRegWriteOverlaps(&instrs[k], rd) {
				return false
			}
		}
	}
	return true
}

// ie64FPSRExitEdge reports whether this opcode can transfer control out of
// the block, and does so only through an exit funnel (emitEpilogue or
// emitLightweightStoreRegs) where a sunk CC update is materialised.
//
// Such an opcode neither reads nor writes the CC field itself. It therefore
// does not force an earlier CC write to be emitted inline: on every path
// that leaves the block the funnel recomputes the CC, and on every path that
// stays in the block the write is still pending and can be sunk further.
//
// A branch whose target is inside the block (a native back-edge) is handled
// separately by the caller, which joins in the loop-entry state as well.
func ie64FPSRExitEdge(op byte) bool {
	switch op {
	case OP_BRA, OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS,
		OP_JMP, OP_HALT64:
		return true
	}
	return false
}

// ie64CCObs is the CC observability lattice, ordered None < Exit < Inline.
// It answers "how can the CC field written at this point be observed?".
type ie64CCObs uint8

const (
	ie64CCObsNone   ie64CCObs = iota // killed before anything can read it
	ie64CCObsExit                    // only after leaving the block through an exit funnel
	ie64CCObsInline                  // readable without leaving the block
)

func ie64CCJoin(a, b ie64CCObs) ie64CCObs {
	if a > b {
		return a
	}
	return b
}

// ie64CCBackEdgeTarget resolves a branch's taken target to an index in this
// block, reporting ok only when the taken path re-enters the block at an
// earlier instruction: a native back-edge that does not pass through an exit
// funnel.
//
// Targets are PC-relative (target = instrPC + int32(imm32)) and instrPC =
// blockStart + pcOffset, so the block start cancels and the test can be made
// on offsets alone. This condition is deliberately a superset of the
// emitter's own native-back-edge test (which additionally requires
// hasBackwardBranch): where the emitter declines the native back-edge it
// emits an epilogue exit instead, which materialises the CC, so
// over-approximating here stays sound.
func ie64CCBackEdgeTarget(instrs []JITInstr, i int) (int, bool) {
	ji := &instrs[i]
	if ji.opcode == OP_HALT64 {
		return 0, false
	}
	if ji.opcode == OP_JMP && ji.rs != 0 {
		return 0, false // dynamic target: always leaves through a funnel
	}
	target := int64(ji.pcOffset) + int64(int32(ji.imm32))
	if target < 0 || target >= int64(ji.pcOffset) {
		return 0, false
	}
	for j := 0; j < i; j++ {
		if int64(instrs[j].pcOffset) == target {
			return j, true
		}
	}
	return 0, false
}

// ie64ScanFPSRCC runs one backward pass over the block, writing into obs the
// observability holding immediately before each instruction. prev supplies
// the previous pass's vector, read at back-edge targets whose taken path
// re-enters the block without passing an exit funnel. When mark is set the
// elision decision is recorded on each CC writer.
func ie64ScanFPSRCC(instrs []JITInstr, prev, obs []ie64CCObs, mark bool) {
	// The block end always falls through to emitEpilogue, which materialises
	// a sunk CC. So it observes, but only via the exit funnel.
	cur := ie64CCObsExit
	for i := len(instrs) - 1; i >= 0; i-- {
		ji := &instrs[i]
		op := ji.opcode
		if mark && ie64FPSRCCWriterElidable(op) {
			switch cur {
			case ie64CCObsNone:
				ji.fpsrCCDead = true
			case ie64CCObsExit:
				// Not sinkable means emit in place: both flags stay false.
				ji.fpsrCCSink = ie64FPSRCCSinkable(op) && ie64CCSinkSafe(instrs, i)
			}
		}
		switch {
		case ie64FPSRTransparent(op):
			// cur unchanged
		case ie64FPSRCCKiller(op):
			cur = ie64CCObsNone
		case ie64FPSRExitEdge(op):
			// Join the fall-through state with the taken path. The taken
			// path leaves through a funnel (Exit); if it is a native
			// back-edge it instead re-enters the block at its target, so it
			// also carries whatever holds on entry to that target.
			taken := ie64CCObsExit
			if j, ok := ie64CCBackEdgeTarget(instrs, i); ok {
				taken = ie64CCJoin(taken, prev[j])
			}
			cur = ie64CCJoin(cur, taken)
		default:
			cur = ie64CCObsInline // reads FPSR, or observes via an unmodelled path
		}
		obs[i] = cur
	}
}

// ie64MarkFPSRCCDead analyses a block's decoded instructions and records, on
// every elidable CC writer, whether its FPSR condition-code update can be
// dropped entirely (fpsrCCDead: a later killer overwrites the whole field
// first) or deferred to the block's exit funnels (fpsrCCSink: nothing inside
// the block can read it).
//
// Sinking is what removes the classifier from hot loop bodies: a loop whose
// only CC observers are its exits pays for the CC once on the way out rather
// than once per iteration.
//
// Back-edges make this a real dataflow problem rather than a single backward
// pass, so it is solved as a least fixpoint of the may-observe equations:
// start optimistic (nothing observes) and re-scan until the per-instruction
// vector stops growing. Each pass either raises at least one entry or
// terminates, and the lattice is three tall, so it converges; the iteration
// bound is a backstop, not a correctness condition. Marking runs only on the
// final, converged vector.
func ie64MarkFPSRCCDead(instrs []JITInstr) {
	if len(instrs) == 0 {
		return
	}
	prev := make([]ie64CCObs, len(instrs)) // all ie64CCObsNone: optimistic start
	cur := make([]ie64CCObs, len(instrs))
	for iter := 0; iter < 2*len(instrs)+2; iter++ {
		ie64ScanFPSRCC(instrs, prev, cur, false)
		same := true
		for i := range cur {
			if cur[i] != prev[i] {
				same = false
				break
			}
		}
		if same {
			break
		}
		copy(prev, cur)
	}
	ie64ScanFPSRCC(instrs, prev, cur, true)
}
