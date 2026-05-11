package main

import (
	"fmt"
	"strings"
)

// Emit accumulates IE64 source lines for one transpiled m68k routine.
// Internal scratch labels are unique per Emit instance.
type Emit struct {
	sb       strings.Builder
	labelSeq int
}

// L appends one IE64 source line, indented one tab.
func (e *Emit) L(line string) {
	e.sb.WriteByte('\t')
	e.sb.WriteString(line)
	e.sb.WriteByte('\n')
}

// Lf is the printf form of L.
func (e *Emit) Lf(format string, args ...any) {
	e.L(fmt.Sprintf(format, args...))
}

// Label emits a bare label line ("name:") at column 0.
func (e *Emit) Label(name string) {
	e.sb.WriteString(name)
	e.sb.WriteString(":\n")
}

// NewLabel returns a unique transpiler-internal label name.
func (e *Emit) NewLabel(prefix string) string {
	e.labelSeq++
	return fmt.Sprintf("__m68kto64_%s_%d", prefix, e.labelSeq)
}

// String returns the emitted source.
func (e *Emit) String() string { return e.sb.String() }

// Scratch register reservations (per plan §"Register File Mapping"):
//
//	r16 — primary EA scratch
//	r17 — primary value scratch (loaded src or computed result)
//	r18 — secondary value scratch
//	r19, r20 — partial-update / mul-div pair lowering
//	r24..r27 — flag shadows (set in Phase 3)
//	r30 — emulated guest a7
//	r31 — host SP, do not touch
const (
	ScrEA  = "r16"
	ScrV1  = "r17"
	ScrV2  = "r18"
	ScrAux = "r19"
	ScrDC  = "r20" // dbcc/condition materialization

	// Phase-A shadow-CCR helper scratches. These are reserved for the
	// shadow-update emitters so they never trample ScrV1/ScrV2/ScrAux/ScrDC,
	// which the producer's primary lowering may still hold live values in.
	ShadowSnap = "r21" // pre-op destination capture for add/sub/cmp shadows
	ShadowTmp1 = "r22" // shadow-helper internal scratch
	ShadowTmp2 = "r23" // shadow-helper internal scratch

	// Shadow X (extend) flag — m68k separates X from C. Most arith ops
	// update both X and C identically; logical ops (AND/OR/EOR/NOT) and
	// MOVE clear C but leave X unchanged. ABCD/SBCD/NBCD read X as the
	// chain-in carry. Cannot collapse to ShadowC.
	ShadowX = "r28"

	// Phase-7 FPU shadow. Mirrors the 4-bit m68k FPSR cc field (bits 27:24)
	// after every FPSR-cc-affecting FPU op. Layout (matches m68k FPSR for
	// direct readability):
	//   bit 3 = N, bit 2 = Z, bit 1 = I (infinity), bit 0 = NaN
	// Producers: FCMP/FTST + cc-affecting arithmetic. Consumers: FBcc /
	// FDBcc / FScc / FTRAPcc / FMOVE.L FPSR,Dn. See plan §"Shadow FPSR
	// maintenance" and `sdk/docs/m68Kto64.md` §7.FP.
	ShadowFPCC = "r29"

	GuestSP = "r30"

	// FP scratch convention. f10 and f12 are reserved as synthesis scratch for
	// FPU lowerings that need temporaries (FTST, FSCALE, FNEG, FGETEXP/MAN,
	// transcendentals). These ALSO happen to be the canonical m68k FP5 and FP6
	// register slots (FPGuestRegToHost(5)="f10", FPGuestRegToHost(6)="f12") —
	// any synth op that touches scratch must spill live FP5/FP6 to dedicated
	// memory slots first; see emitFP56SpillPrologue/Epilogue in fpu.go.
	ScrFP1 = "f10" // also m68k FP5 — spilled around clobbering ops
	ScrFP2 = "f12" // also m68k FP6 — spilled around clobbering ops
)

// FPU memory-slot reservations. These are per-output-file BSS-style globals
// (single-thread guest assumed). See `sdk/docs/m68Kto64.md` §4.FP and
// §15.FP for the reentrancy caveat under guest interrupts.
const (
	FPSlotFPCRSave  = "__m68kto64_fpcr_save"     // FINTRZ FPCR save/restore
	FPSlotScratchQ  = "__m68kto64_fp_scratch_q"  // FSCALE bit-pattern round-trip
	FPSlotConstPool = "__m68kto64_fp_const_pool" // FP-immediate pool prefix
)

// FPGuestRegToHost maps m68k FPn (n ∈ 0..7) to the canonical IE64 even FP
// register. Caller must validate n.
func FPGuestRegToHost(n int) string { return fmt.Sprintf("f%d", 2*n) }

// SizeBytes maps the m68k size suffix (".b"/".w"/".l"/"") to a byte count.
// Default (no suffix) is .w (m68k convention). Unknown suffix returns 0.
func SizeBytes(suffix string) int {
	switch strings.ToLower(suffix) {
	case ".b":
		return 1
	case ".w", "":
		return 2
	case ".l":
		return 4
	default:
		return 0
	}
}

// IE64Size returns the IE64 size suffix matching a m68k size byte count.
//
//	1 -> ".b"   2 -> ".w"   4 -> ".l"
func IE64Size(bytes int) string {
	switch bytes {
	case 1:
		return ".b"
	case 2:
		return ".w"
	default:
		return ".l"
	}
}

// SizeMask returns a hex literal of the low-bytes mask for a width.
//
//	1 -> "$FF", 2 -> "$FFFF", 4 -> "$FFFFFFFF"
func SizeMask(bytes int) string {
	switch bytes {
	case 1:
		return "$FF"
	case 2:
		return "$FFFF"
	default:
		return "$FFFFFFFF"
	}
}

// SizeInvMask returns a hex literal masking off the LOW `bytes` bytes (so the
// result preserves the upper bits of a 64-bit register).
//
//	1 -> "$FFFFFFFFFFFFFF00"
//	2 -> "$FFFFFFFFFFFF0000"
//	4 -> "$FFFFFFFF00000000"
func SizeInvMask(bytes int) string {
	switch bytes {
	case 1:
		return "$FFFFFFFFFFFFFF00"
	case 2:
		return "$FFFFFFFFFFFF0000"
	default:
		return "$FFFFFFFF00000000"
	}
}
