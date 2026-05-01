// jit_flags_common.go - shared lazy-flag state machine (Phase 2 of the
// six-CPU JIT unification plan).
//
// Each JIT backend (x86, M68K, Z80, 6502) tracks whether the host EFLAGS
// register currently encodes the guest's last flag-producing operation, so
// that downstream consumers (Jcc, conditional moves, materialized flag reads)
// can use the host flags directly instead of re-deriving them from the
// guest's flag-word memory image. Before this file the state machine was
// duplicated per backend with conflicting symbol names (m68kFlagState +
// flagsMaterialized in jit_m68k_emit_amd64.go, x86FlagState + x86FlagsDead in
// jit_x86_emit_amd64.go). This file consolidates the core enum so backends
// share the same vocabulary; backend-specific extensions (M68K's
// LiveArithNoX, x86's LiveInc) are defined alongside the shared values.
//
// The values here are stable — they are referenced from compile-time emitters
// (jit_*_emit_amd64.go) and can leak into block-state structs that survive
// across compile/decompile cycles. Do NOT renumber.

//go:build amd64 && (linux || windows || darwin)

package main

// JITFlagState records whether the host EFLAGS register currently holds the
// guest's last flag-producing result, and if so under what semantics.
//
// Backends distinguish:
//
//   - JITFlagDead         — no valid flag state; consumers must re-derive.
//   - JITFlagLiveArith    — host EFLAGS from arithmetic (ADD/SUB/CMP/ADC/SBB
//     on x86; ADD/SUB/NEG with X stored to slot on
//     M68K). Consumer Jcc maps directly.
//   - JITFlagLiveLogic    — host EFLAGS from logical (AND/OR/XOR/TEST). On
//     x86: CF=OF=0. On M68K: V=C=0 in CCR semantics.
//   - JITFlagLiveInc      — host EFLAGS from INC/DEC (CF preserved). x86 only
//     in the shared enum; other backends do not produce
//     this state.
//   - JITFlagMaterialized — host EFLAGS no longer reflect the guest; the
//     guest flag-word memory image is up-to-date.
//
// Backend-specific extensions (e.g. JITFlagLiveArithNoX for M68K's CMP, which
// modifies NZVC but NOT the X bit) live at JITFlagBackendBase + offset so
// they cannot collide with the core enum or with extensions added by other
// backends.
type JITFlagState int

// Numeric ordering note: JITFlagMaterialized is value 0 because M68K's
// existing block-local compile state (`var cs m68kCompileState` at the top of
// the M68K JIT compile loop) relies on Go's zero-initialization defaulting
// flagState to "guest CCR is up-to-date in memory" — the M68K block prologue
// loads the live CCR before any flag-producing op runs, so the safe default
// is Materialized. Other backends (x86) explicitly initialize flagState in
// the constructor, so they are agnostic to the numeric ordering.
const (
	JITFlagMaterialized JITFlagState = iota // 0 — guest flag word is canonical (default zero).
	JITFlagDead                             // 1 — no valid flag state; consumer re-derives.
	JITFlagLiveArith                        // 2 — host EFLAGS from arithmetic.
	JITFlagLiveLogic                        // 3 — host EFLAGS from logical (CF=OF=0 on x86; V=C=0 on M68K).
	JITFlagLiveInc                          // 4 — host EFLAGS from INC/DEC (CF preserved, x86 only in shared core).

	// JITFlagBackendBase is the first value reserved for backend-specific
	// flag-state extensions. Backends that need additional states declare
	// constants like:
	//
	//   const JITFlagLiveArithNoX JITFlagState = JITFlagBackendBase + iota
	//
	// alongside their backend code so the shared core enum stays small.
	JITFlagBackendBase JITFlagState = 100
)

// JITFlagLiveArithNoX is M68K's "CMP-style" arithmetic state: host EFLAGS
// encode N/Z/V/C from a CMP/SUB-without-writeback, but the guest CCR's X bit
// was deliberately not modified by the producing instruction (only CMP and a
// handful of other ops behave this way on M68K). Defined here so it shares
// numbering with the shared core enum and is callable from any future
// cross-backend liveness analyzer.
const JITFlagLiveArithNoX JITFlagState = JITFlagBackendBase + 0

// String returns a short symbolic name for diagnostic output. Backends are
// free to print their own narrower names; this is the default.
func (s JITFlagState) String() string {
	switch s {
	case JITFlagDead:
		return "Dead"
	case JITFlagLiveArith:
		return "LiveArith"
	case JITFlagLiveLogic:
		return "LiveLogic"
	case JITFlagLiveInc:
		return "LiveInc"
	case JITFlagMaterialized:
		return "Materialized"
	case JITFlagLiveArithNoX:
		return "LiveArithNoX"
	}
	return "JITFlagState(?)"
}

// JITFlagLiveness is a per-instruction bitmap recording, for each instruction
// in a JIT block, whether that instruction's flag output is consumed by a
// downstream instruction inside the same block (true) or is dead (false).
// Producers that find their slot false can skip emitting the flag-update
// micro-ops; final-block-exit materialization is still required because the
// guest may observe flags after the block returns.
//
// Producers (per-backend `*PeepholeFlags` analyses) populate this slice; the
// emitter consults it as it walks instructions. The shared type lets a future
// Phase 7 cross-backend analyzer share allocation/reset code.
type JITFlagLiveness []bool

// MaterializeFn is the signature backends provide for "given the previous
// flag state, emit code that brings the guest flag word up to date and
// transitions the state to JITFlagMaterialized." Phase 2 introduces the type
// so future shared liveness logic can call into the backend's lowering
// without depending on backend-private code-buffer types directly. Phase 2a
// does not yet rewire the per-backend materializers through this hook —
// existing backend code keeps doing it inline. The type is the placeholder
// for that future refactor.
//
// cb is the backend's CodeBuffer (each backend uses *CodeBuffer); prev is
// the state on entry; the returned state is the state after emission (always
// JITFlagMaterialized in practice, but typed as JITFlagState for symmetry).
type MaterializeFn func(cb *CodeBuffer, prev JITFlagState) JITFlagState
