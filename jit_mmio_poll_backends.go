// jit_mmio_poll_backends.go - per-backend MMIO-poll pattern descriptors
// scaffold (Phase 7f sub-phases of the six-CPU JIT unification plan).
//
// Each backend declares the canonical shape its emitter produces for
// MMIO poll loops; the shared TryFastMMIOPoll core (Phase 7f real impl)
// matches against these descriptors. Today the predicates are nil — the
// per-backend exec-loop wiring patch fills them in when the matcher
// lands; the descriptors themselves are stable.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

// X86PollPattern - canonical x86 MMIO poll shape recognized by
// tryFastMMIOPollLoop: MOV r,[mem]; TEST/CMP r,imm; Jcc back.
var X86PollPattern = PollPattern{
	Load:         PollLoad32,
	Test:         PollTestCMPImm,
	Backward:     true,
	MaxBodyLen:   3,
	IterationCap: DefaultPollIterationCap,
}

// M68KPollPattern - canonical M68K poll: MOVE.W or MOVE.L from MMIO,
// CMP/BTST against an immediate or bit, Bcc backward.
var M68KPollPattern = PollPattern{
	Load:         PollLoad16,
	Test:         PollTestCMPImm,
	Backward:     true,
	MaxBodyLen:   4,
	IterationCap: DefaultPollIterationCap,
}

// Z80PollPattern - canonical Z80 poll: IN A,(C) (PollLoad8), bit test
// against a flag, JR backward.
var Z80PollPattern = PollPattern{
	Load:         PollLoad8,
	Test:         PollTestBitTest,
	Backward:     true,
	MaxBodyLen:   4,
	IterationCap: DefaultPollIterationCap,
}

// P65PollPattern - canonical 6502 poll: LDA $abs (PollLoad8), test bit
// or compare immediate, BPL/BMI/BNE/BEQ backward.
var P65PollPattern = PollPattern{
	Load:         PollLoad8,
	Test:         PollTestSign, // BPL/BMI: sign-bit test
	Backward:     true,
	MaxBodyLen:   3,
	IterationCap: DefaultPollIterationCap,
}

// IE64PollPattern - canonical IE64 poll: LDW from MMIO, CMP imm, Bcc back.
var IE64PollPattern = PollPattern{
	Load:         PollLoad32,
	Test:         PollTestCMPImm,
	Backward:     true,
	MaxBodyLen:   3,
	IterationCap: DefaultPollIterationCap,
}

// BackendPollPatterns is the registry consumed by per-backend exec
// loops once the matcher lands.
var BackendPollPatterns = map[string]*PollPattern{
	"x86":  &X86PollPattern,
	"m68k": &M68KPollPattern,
	"z80":  &Z80PollPattern,
	"6502": &P65PollPattern,
	"ie64": &IE64PollPattern,
}
