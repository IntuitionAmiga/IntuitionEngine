// jit_ie64_fpu_residency.go - FP register residency planning for the IE64
// amd64 region tier (Technique 3 of IE64_JIT_PERFORMANCE_PLAN.md).
//
// The shared IE64 FP register file is FPRegs[16]uint32: sixteen 32-bit slots.
// An FP32 register Fn occupies the single slot n. An FP64 register Dn occupies
// the even/odd slot pair (n&0x0E, n|0x01) and holds the little-endian bit
// pattern of the double (low word in the even slot). D-registers are named by
// their even base (D0, D2, ... D14).
//
// This pass ranks the hottest FP registers in a region and binds up to eight of
// them to host XMM8..XMM15 for the duration of that region, so arithmetic runs
// register-to-register instead of round-tripping through the FPRegs array. It
// mirrors the GPR region planner (Technique 2) but over the FP file.
//
// ABI note: there are two distinct preservation obligations, and they must not
// be conflated.
//
//  1. Guest FP state: on both SysV and Win64 a call into Go clobbers the XMM
//     registers Go is free to use, so a resident guest value cannot survive a
//     helper/bail. Any FP opcode that can call a helper, fault, or bail to the
//     interpreter (memory FP ops, transcendentals, cross-format conversions
//     routed through Go, FPSR/FPCR moves) is therefore a residency barrier: the
//     planner never keeps a resident across one, and the emitter (sub-slice B)
//     spills dirty residents before such an exit.
//
//  2. Host caller state: the ABIs disagree on XMM volatility. On SysV (Linux,
//     macOS) all of XMM0..XMM15 are caller-saved, so the region may use
//     XMM8..XMM15 freely. On Win64 (Windows) XMM6..XMM15 are callee-saved: the
//     JIT is entered from the host, so on Windows the region prologue MUST save
//     the XMM8..XMM15 it uses and the epilogue MUST restore them, exactly as the
//     block already preserves the callee-saved GPRs RBX/RBP/R12/R13. Spilling
//     guest values (obligation 1) does not discharge this; it preserves guest
//     state, not the host caller's registers. Sub-slice B carries that
//     save/restore on the Windows path.
//
// This file is analysis metadata only: it produces a tested ownership contract
// that the region emitter consumes. It changes no code generation on its own.

package main

import (
	"os"
	"runtime"
	"strings"
)

// ie64FPResidencySysV reports whether the host uses the SysV amd64 ABI, where
// all XMM registers are caller-saved. FP residency is currently restricted to
// SysV: on Win64 XMM6..XMM15 are callee-saved and would require the region
// prologue/epilogue to save and restore the host's copies (see the ABI note
// above). The plan scopes Technique 3 amd64 to Linux, so Windows keeps the
// memory-backed FP path until that save/restore lands.
func ie64FPResidencySysV() bool {
	if runtime.GOARCH == "arm64" {
		return true
	}
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin"
}

// ie64FPResidencyEnabled reports whether FP register residency is switched on.
// Off by default; opt in with IE64_JIT_FP_RESIDENCY=1 on a SysV host.
func ie64FPResidencyEnabled() bool {
	if !ie64FPResidencySysV() {
		return false
	}
	return strings.TrimSpace(os.Getenv("IE64_JIT_FP_RESIDENCY")) == "1"
}

// ie64BuildBlockFPPlan builds an FP32-single and FP64-pair residency plan for
// one block, or returns ok=false when the block is ineligible. Any residency
// barrier (helper/memory/fault/FPSR op) disqualifies the block, so the sole live
// FP state at every exit is the resident XMMs and a block-boundary spill
// suffices. FP64 arithmetic ops can bail mid-block on a non-finite input, but
// every bail routes through emitEpilogue, which spills residents to canonical
// memory before the interpreter (or an interrupt handler) resumes; the
// non-finite check itself reads live values through the residency-hooked pair
// accessor. Non-overlapping singles and pairs may coexist.
func ie64BuildBlockFPPlan(instrs []JITInstr) (ie64FPResidencyPlan, bool) {
	var sw [16]int
	var pw [8]int
	var singleTouched, pairTouched [16]bool
	sawFP := false
	mark := func(slots []byte, touched *[16]bool) {
		for _, s := range slots {
			if s < 16 {
				touched[s] = true
			}
		}
	}
	for i := range instrs {
		u := ie64ClassifyFPUsage(&instrs[i])
		if u.barrier {
			return ie64FPResidencyPlan{}, false
		}
		if len(u.readSingles)+len(u.writeSingles)+len(u.readPairs)+len(u.writePairs) > 0 {
			sawFP = true
		}
		mark(u.readSingles, &singleTouched)
		mark(u.writeSingles, &singleTouched)
		for _, p := range u.readPairs {
			mark([]byte{p, p | 0x01}, &pairTouched)
		}
		for _, p := range u.writePairs {
			mark([]byte{p, p | 0x01}, &pairTouched)
		}
		ie64AccumulateFPWeights(&instrs[i], &sw, &pw)
	}
	// Aliasing: an FP32 single Fn and the FP64 pair covering slot n share the
	// same physical FPRegs storage. If the region touches a slot at BOTH widths,
	// making either one resident would let its XMM spill clobber the other's
	// memory slot. Exclude both from residency so the shared memory stays
	// authoritative, matching interpreter aliasing semantics.
	for s := 0; s < 16; s++ {
		if singleTouched[s] && pairTouched[s] {
			sw[s] = 0
			pw[s>>1] = 0 // the pair whose base is s&0x0E (same index for even/odd s)
		}
	}
	if !sawFP {
		return ie64FPResidencyPlan{}, false
	}
	plan := ie64BuildFPResidencyPlan(sw, pw)
	if len(plan.bindings) == 0 {
		return ie64FPResidencyPlan{}, false
	}
	return plan, true
}

// ie64FPResidentHostXMMs are the host XMM registers a promoted region may bind
// to its hottest FP registers, ordered hottest-first. XMM0..XMM7 stay free as
// JIT scratch for the memory/helper/CC paths; XMM8..XMM15 carry residents.
var ie64FPResidentHostXMMs = func() []byte {
	if runtime.GOARCH == "arm64" {
		return []byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}
	}
	return []byte{8, 9, 10, 11, 12, 13, 14, 15}
}()

// ie64FPResidentKind classifies what an ownership-map slot holds.
type ie64FPResidentKind uint8

const (
	ie64FPResNone   ie64FPResidentKind = iota // slot unused
	ie64FPResSingle                           // FP32: one file slot
	ie64FPResPair                             // FP64: even+odd file slots
)

// ie64FPResidentBinding pins one FP register (single or pair) to a host XMM for
// the duration of a single region compilation.
type ie64FPResidentBinding struct {
	kind     ie64FPResidentKind
	baseSlot byte // FP32: the slot n; FP64: the even base (n & 0x0E)
	xmm      byte // host XMM 8..15
}

// slots returns the FPRegs slot indices this binding owns.
func (b ie64FPResidentBinding) slots() []byte {
	if b.kind == ie64FPResPair {
		return []byte{b.baseSlot, b.baseSlot | 0x01}
	}
	return []byte{b.baseSlot}
}

// ie64FPRegUsage records the FP file slots one instruction reads and writes,
// split by width, plus whether the instruction is a residency barrier.
type ie64FPRegUsage struct {
	readSingles  []byte // FP32 slots read
	writeSingles []byte // FP32 slots written
	readPairs    []byte // FP64 even bases read
	writePairs   []byte // FP64 even bases written
	barrier      bool   // helper/fault/bail: residency cannot cross it
}

// dpairBase returns the even base slot of the FP64 register named by idx.
func dpairBase(idx byte) byte { return idx & 0x0E }

// ie64ClassifyFPUsage decodes one instruction's FP register file usage. Only
// the amd64 region-eligible FP opcodes are modelled precisely; every other FP
// opcode (memory ops, transcendentals, FPSR/FPCR moves, cross-format
// conversions routed through Go) is reported as a barrier with no residency
// participation. Non-FP opcodes return the zero usage (no FP file access).
func ie64ClassifyFPUsage(ji *JITInstr) ie64FPRegUsage {
	var u ie64FPRegUsage
	switch ji.opcode {
	// --- FP32 register-to-register arithmetic (never bail) ---
	case OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV:
		u.readSingles = []byte{ji.rs, ji.rt}
		u.writeSingles = []byte{ji.rd}
	case OP_FABS, OP_FNEG, OP_FSQRT, OP_FINT, OP_FMOV:
		u.readSingles = []byte{ji.rs}
		u.writeSingles = []byte{ji.rd}
	case OP_FMOVI, OP_FMOVECR:
		// GPR/const -> FP32 rd. rs is not an FP register.
		u.writeSingles = []byte{ji.rd}
	case OP_FCVTIF:
		// GPR rs -> FP32 rd.
		u.writeSingles = []byte{ji.rd}
	case OP_FCVTFI:
		// FP32 rs -> GPR rd.
		u.readSingles = []byte{ji.rs}

	// --- FP64 register-to-register arithmetic (can bail on non-finite) ---
	case OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV:
		u.readPairs = []byte{dpairBase(ji.rs), dpairBase(ji.rt)}
		u.writePairs = []byte{dpairBase(ji.rd)}
	case OP_DABS, OP_DNEG, OP_DMOV, OP_DINT:
		u.readPairs = []byte{dpairBase(ji.rs)}
		u.writePairs = []byte{dpairBase(ji.rd)}
	case OP_DCVTIF:
		// GPR rs -> FP64 rd.
		u.writePairs = []byte{dpairBase(ji.rd)}
	case OP_DCVTFI:
		// FP64 rs -> GPR rd.
		u.readPairs = []byte{dpairBase(ji.rs)}

	// --- Residency barriers: helper call, memory, fault, or FPSR/FPCR ---
	case OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE,
		OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW,
		OP_DMOD, OP_DSQRT, OP_DSIN, OP_DCOS, OP_DTAN, OP_DATAN, OP_DLOG, OP_DEXP, OP_DPOW,
		OP_FCMP, OP_DCMP, OP_FMOVO, OP_FCVTSD, OP_FCVTDS,
		OP_FMOVSR, OP_FMOVSC, OP_FMOVCR, OP_FMOVCC:
		u.barrier = true
	}
	return u
}

// ie64AccumulateFPWeights adds one instruction's FP register hotness to the
// single-slot and pair weight tables. A barrier contributes nothing (its
// operands cannot stay resident across it). Reads and writes both count: a
// resident value saves a load on read and a store on write.
func ie64AccumulateFPWeights(ji *JITInstr, singleW *[16]int, pairW *[8]int) {
	u := ie64ClassifyFPUsage(ji)
	if u.barrier {
		return
	}
	for _, s := range u.readSingles {
		if s < 16 {
			singleW[s] += 2
		}
	}
	for _, s := range u.writeSingles {
		if s < 16 {
			singleW[s] += 2
		}
	}
	for _, p := range u.readPairs {
		if p < 16 {
			pairW[p>>1] += 3
		}
	}
	for _, p := range u.writePairs {
		if p < 16 {
			pairW[p>>1] += 3
		}
	}
}

// ie64FPResidencyPlan is the ownership contract for one region: the bindings
// plus a 16-slot reverse map from FPRegs slot to its owning binding index
// (-1 when a slot is not resident). Selected residents never overlap.
type ie64FPResidencyPlan struct {
	bindings []ie64FPResidentBinding
	owner    [16]int8 // slot -> index into bindings, or -1
}

// resident reports the binding owning an FPRegs slot, and true, or a zero
// binding and false when the slot is not resident.
func (p *ie64FPResidencyPlan) resident(slot byte) (ie64FPResidentBinding, bool) {
	if slot >= 16 || p.owner[slot] < 0 {
		return ie64FPResidentBinding{}, false
	}
	return p.bindings[p.owner[slot]], true
}

// ie64BuildFPResidencyPlan greedily selects up to len(ie64FPResidentHostXMMs)
// non-overlapping residents from the weighted single and pair candidates,
// hottest-first, and binds each to the next free host XMM. A pair and a single
// that would share a file slot are mutually exclusive; the hotter one wins. The
// resulting plan's owner map has no overlap by construction.
func ie64BuildFPResidencyPlan(singleW [16]int, pairW [8]int) ie64FPResidencyPlan {
	type cand struct {
		kind   ie64FPResidentKind
		base   byte
		weight int
	}
	cands := make([]cand, 0, 24)
	for s := 0; s < 16; s++ {
		if singleW[s] > 0 {
			cands = append(cands, cand{ie64FPResSingle, byte(s), singleW[s]})
		}
	}
	for p := 0; p < 8; p++ {
		if pairW[p] > 0 {
			cands = append(cands, cand{ie64FPResPair, byte(p) << 1, pairW[p]})
		}
	}
	// Stable, deterministic hottest-first ordering: weight desc, then a fixed
	// tiebreak (pairs before singles, then base asc) so codegen is reproducible.
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0; j-- {
			a, b := cands[j-1], cands[j]
			swap := b.weight > a.weight ||
				(b.weight == a.weight && b.kind == ie64FPResPair && a.kind == ie64FPResSingle) ||
				(b.weight == a.weight && b.kind == a.kind && b.base < a.base)
			if !swap {
				break
			}
			cands[j-1], cands[j] = cands[j], cands[j-1]
		}
	}

	plan := ie64FPResidencyPlan{}
	for i := range plan.owner {
		plan.owner[i] = -1
	}
	var used [16]bool
	for _, c := range cands {
		if len(plan.bindings) >= len(ie64FPResidentHostXMMs) {
			break
		}
		b := ie64FPResidentBinding{kind: c.kind, baseSlot: c.base, xmm: ie64FPResidentHostXMMs[len(plan.bindings)]}
		free := true
		for _, s := range b.slots() {
			if used[s] {
				free = false
				break
			}
		}
		if !free {
			continue // overlaps an already-selected resident
		}
		idx := int8(len(plan.bindings))
		for _, s := range b.slots() {
			used[s] = true
			plan.owner[s] = idx
		}
		plan.bindings = append(plan.bindings, b)
	}
	return plan
}
