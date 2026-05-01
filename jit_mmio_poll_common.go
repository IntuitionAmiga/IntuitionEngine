// jit_mmio_poll_common.go - shared MMIO-poll fast-path descriptor (Phase 7f
// of the six-CPU JIT unification plan).
//
// cpu_x86.go's tryFastMMIOPollLoop recognizes one shape (MOV r,[mem]; TEST/
// CMP r,imm; Jcc backward) at one operand width on x86 only. The mechanism
// — short backward branch over a single load + test of an MMIO address — is
// universal: every guest CPU running any demo, OS boot polling a UART/timer/
// VBlank status, or game waiting on a hardware flag exhibits the same
// idiom. This file collects the parameters that make the matcher
// per-backend.
//
// Backends declare a PollPattern listing the load/test/branch shapes they
// emit, and at exec-loop time call TryFastMMIOPoll(cpu, instrs, pattern).
// Per-backend wiring is sub-phase 7f-bis (one Bash patch per backend) once
// the shared core is in place; this file is the core.
//
// Correctness rule: the poll body must be side-effect-free apart from the
// MMIO read and the flag/condition register update. Anything else
// (interrupt enable, segment override, multiple loads) → no match, fall
// through.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

// PollLoadShape classifies the load encoding the matcher accepts at the
// head of a poll loop. Each backend has at most one canonical shape per
// guest-address-width.
type PollLoadShape int

const (
	// PollLoad8 matches 8-bit MMIO loads (M68K MOVE.B, x86 MOV byte ptr,
	// 6502 LDA imm/zp/abs).
	PollLoad8 PollLoadShape = iota
	// PollLoad16 matches 16-bit MMIO loads (M68K MOVE.W, x86 MOV word ptr,
	// Z80 IN A,(C)).
	PollLoad16
	// PollLoad32 matches 32-bit MMIO loads (M68K MOVE.L, x86 MOV dword ptr,
	// IE64 LDW).
	PollLoad32
	// PollLoad64 matches 64-bit MMIO loads (IE64 LDQ).
	PollLoad64
)

// PollTestShape classifies the test encoding between the load and the
// backward branch. Multiple may be valid per backend.
type PollTestShape int

const (
	// PollTestCMPImm — compare-with-immediate, branches on equal/not-equal.
	PollTestCMPImm PollTestShape = iota
	// PollTestBitTest — bit-test instruction (BT/TST), branches on bit set/
	// clear.
	PollTestBitTest
	// PollTestSign — sign-test (test register against itself), branches on
	// MSB set/clear.
	PollTestSign
)

// PollPattern is a backend's poll-shape descriptor. The exec-loop matcher
// iterates the most recent few instructions and reports a match iff:
//
//   - Last instruction is a backward Jcc (branch-direction == backward).
//   - Penultimate instruction is a test of the shape Test.
//   - Pre-penultimate instruction is a load of the shape Load whose source
//     address satisfies AddressIsMMIOPredicate(addr).
//   - No other side-effect-emitting instruction sits between the load and
//     the branch.
//
// On match, fast-path runs the poll natively (read MMIO, test, conditional
// skip-ahead-to-block-after-loop) until the condition flips or
// IterationCap is hit, then re-syncs guest state and returns control.
type PollPattern struct {
	Load       PollLoadShape
	Test       PollTestShape
	Backward   bool
	MaxBodyLen int

	// AddressIsMMIOPredicate returns true if the load address is in an
	// MMIO region. Backends supply their existing per-page bitmap probe.
	AddressIsMMIOPredicate func(addr uint32) bool

	// IterationCap caps the number of poll iterations the fast path runs
	// before re-syncing guest state. Default 4096 matches M68K's existing
	// stop-state cadence (cpu_m68k.go:2408).
	IterationCap int
}

// DefaultPollIterationCap matches M68K's stop-state cadence so a unified
// poll loop preserves existing M68K timing behavior when adopted on other
// backends.
const DefaultPollIterationCap = 4096

// PollInstrKind classifies a recent-instruction trace entry from the
// perspective of the MMIO-poll matcher. Per-backend exec loops translate
// their native JIT IR into a sliding window of PollInstr values and pass
// it to TryFastMMIOPoll.
type PollInstrKind int

const (
	// PollInstrOther is any instruction that is not a load, test or
	// branch — its presence between a load and a backward branch
	// disqualifies the match (poll body must be side-effect-free apart
	// from the load + test).
	PollInstrOther PollInstrKind = iota
	// PollInstrLoad is a memory load. LoadShape and LoadAddr are valid.
	PollInstrLoad
	// PollInstrTest is a flag-producing test of the most recently loaded
	// value. TestShape is valid.
	PollInstrTest
	// PollInstrBranchBackward is a conditional branch whose target lies
	// at or before the load slot in the same trace.
	PollInstrBranchBackward
	// PollInstrBranchForward is a conditional/unconditional branch
	// whose target lies past the trace end. Disqualifies the match
	// unless explicitly allowed by the pattern.
	PollInstrBranchForward
)

// PollInstr is a typed recent-instruction descriptor consumed by
// TryFastMMIOPoll. Backends populate fields per Kind; unused fields
// default to zero. Keeping the descriptor concrete (rather than `any`)
// lets the matcher do real shape analysis without per-backend type
// assertions.
type PollInstr struct {
	Kind      PollInstrKind
	LoadShape PollLoadShape // valid when Kind == PollInstrLoad
	TestShape PollTestShape // valid when Kind == PollInstrTest
	LoadAddr  uint32        // valid for PollInstrLoad
}

// PollMatchResult carries the outcome of a poll-shape match attempt. The
// shared TryFastMMIOPoll core returns one of these so per-backend
// fast-path runners can act without re-deriving the slot positions.
type PollMatchResult struct {
	Matched       bool
	LoadIdx       int
	TestIdx       int
	BranchIdx     int
	LoadAddr      uint32
	IterationsRun int
}

// TryFastMMIOPoll is the shared MMIO-poll matcher core. Walks the trace
// looking for the canonical poll shape:
//
//	instrs[k]   = PollInstrLoad     with shape == pattern.Load and
//	                                pattern.AddressIsMMIOPredicate(addr) == true
//	instrs[k+1] = PollInstrTest     with shape == pattern.Test
//	instrs[k+2] = PollInstrBranchBackward (if pattern.Backward)
//
// Up to (pattern.MaxBodyLen - 3) PollInstrOther entries are tolerated
// before the load (the matcher only requires the trailing 3 entries to
// match exactly). PollInstrOther entries between load and branch
// disqualify the match.
//
// Returns Matched=false on any mismatch; on match, populates the slot
// indices and the load address. The caller drives the fast-path runner.
func TryFastMMIOPoll(trace []PollInstr, pattern *PollPattern) PollMatchResult {
	if pattern == nil || len(trace) < 3 {
		return PollMatchResult{}
	}
	branchIdx := len(trace) - 1
	br := trace[branchIdx]
	if pattern.Backward {
		if br.Kind != PollInstrBranchBackward {
			return PollMatchResult{}
		}
	} else if br.Kind != PollInstrBranchForward && br.Kind != PollInstrBranchBackward {
		return PollMatchResult{}
	}

	testIdx := branchIdx - 1
	test := trace[testIdx]
	if test.Kind != PollInstrTest || test.TestShape != pattern.Test {
		return PollMatchResult{}
	}

	loadIdx := testIdx - 1
	load := trace[loadIdx]
	if load.Kind != PollInstrLoad || load.LoadShape != pattern.Load {
		return PollMatchResult{}
	}

	// Fail closed when no predicate is wired in. The matcher's contract is
	// that the load address must satisfy the MMIO predicate; a nil
	// predicate means the backend wiring has not populated it yet (or a
	// caller copied a pattern without the predicate), and accepting any
	// load address would let an ordinary RAM polling loop be classified as
	// a side-effect-free MMIO wait and fast-forwarded incorrectly. Refuse
	// the match instead.
	if pattern.AddressIsMMIOPredicate == nil || !pattern.AddressIsMMIOPredicate(load.LoadAddr) {
		return PollMatchResult{}
	}

	// Body length cap: the entire trace must fit within MaxBodyLen.
	// Anything in [0 .. loadIdx-1] is the loop-body prelude and MUST
	// be PollInstrOther (no extra loads, tests, or branches between
	// loop top and the matched load+test+branch tail). A
	// side-effecting instruction in this region — e.g. a stray store,
	// a second load, an interrupt-enable — would invalidate the
	// "side-effect-free apart from MMIO read + flag/condition update"
	// correctness rule, so it disqualifies the match.
	if pattern.MaxBodyLen > 0 && len(trace) > pattern.MaxBodyLen {
		return PollMatchResult{}
	}
	for i := 0; i < loadIdx; i++ {
		if trace[i].Kind != PollInstrOther {
			return PollMatchResult{}
		}
	}

	return PollMatchResult{
		Matched:   true,
		LoadIdx:   loadIdx,
		TestIdx:   testIdx,
		BranchIdx: branchIdx,
		LoadAddr:  load.LoadAddr,
	}
}
