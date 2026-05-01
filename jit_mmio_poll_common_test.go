// jit_mmio_poll_common_test.go - shape gates for the shared MMIO-poll
// matcher (Phase 7f).

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "testing"

// allMMIO is a permissive predicate used in unit tests so the matcher's
// addr-classifier branch is exercised without per-backend setup.
func allMMIO(_ uint32) bool { return true }

// noMMIO disqualifies every load address.
func noMMIO(_ uint32) bool { return false }

func TestTryFastMMIOPoll_MatchesCanonicalShape(t *testing.T) {
	pat := &PollPattern{
		Load:                   PollLoad32,
		Test:                   PollTestCMPImm,
		Backward:               true,
		MaxBodyLen:             3,
		IterationCap:           DefaultPollIterationCap,
		AddressIsMMIOPredicate: allMMIO,
	}
	trace := []PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	res := TryFastMMIOPoll(trace, pat)
	if !res.Matched {
		t.Fatal("canonical poll shape should match")
	}
	if res.LoadIdx != 0 || res.TestIdx != 1 || res.BranchIdx != 2 {
		t.Errorf("indices = (%d,%d,%d) want (0,1,2)", res.LoadIdx, res.TestIdx, res.BranchIdx)
	}
	if res.LoadAddr != 0xFFFF0000 {
		t.Errorf("LoadAddr = %#x want 0xFFFF0000", res.LoadAddr)
	}
}

func TestTryFastMMIOPoll_RejectsForwardBranchWhenBackwardRequired(t *testing.T) {
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchForward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("forward branch must not match a backward pattern")
	}
}

func TestTryFastMMIOPoll_RejectsWrongTestShape(t *testing.T) {
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestBitTest},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("wrong TestShape must not match")
	}
}

func TestTryFastMMIOPoll_RejectsWrongLoadShape(t *testing.T) {
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad8, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("wrong LoadShape must not match")
	}
}

func TestTryFastMMIOPoll_RejectsNonMMIOAddress(t *testing.T) {
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, AddressIsMMIOPredicate: noMMIO}
	trace := []PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0x1000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("non-MMIO address must not match")
	}
}

func TestTryFastMMIOPoll_RejectsShortTrace(t *testing.T) {
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, AddressIsMMIOPredicate: allMMIO}
	short := []PollInstr{
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(short, pat).Matched {
		t.Fatal("trace shorter than 3 must not match")
	}
}

func TestTryFastMMIOPoll_RejectsNilPattern(t *testing.T) {
	if TryFastMMIOPoll([]PollInstr{{}, {}, {}}, nil).Matched {
		t.Fatal("nil pattern must not match")
	}
}

func TestTryFastMMIOPoll_AllRegistryPatternsScaffoldShape(t *testing.T) {
	// Verify each registered backend pattern matches its canonical trace
	// shape. Phase 7f-bis wires real predicates into these patterns, so we
	// override with a permissive predicate for the duration of the test
	// (and restore on cleanup) to isolate shape correctness from
	// classifier identity.
	for tag, pat := range BackendPollPatterns {
		saved := pat.AddressIsMMIOPredicate
		pat.AddressIsMMIOPredicate = allMMIO
		var loadShape PollLoadShape = pat.Load
		trace := []PollInstr{
			{Kind: PollInstrLoad, LoadShape: loadShape, LoadAddr: 0xFFFF0000},
			{Kind: PollInstrTest, TestShape: pat.Test},
			{Kind: PollInstrBranchBackward},
		}
		res := TryFastMMIOPoll(trace, pat)
		if !res.Matched {
			t.Errorf("backend %q canonical trace failed to match (Load=%v Test=%v)",
				tag, pat.Load, pat.Test)
		}
		pat.AddressIsMMIOPredicate = saved
	}
}

func TestTryFastMMIOPoll_RejectsOversizedBody(t *testing.T) {
	// MaxBodyLen=3 means trace longer than 3 must be rejected even if
	// the last 3 entries match. Backends should never trip this in
	// production traces, but if they do, the matcher must refuse.
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, MaxBodyLen: 3, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrOther},
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("trace longer than MaxBodyLen must not match")
	}
}

func TestTryFastMMIOPoll_AllowsLeadingOtherWhenWithinCap(t *testing.T) {
	// MaxBodyLen=4 with 4-entry trace whose lead is PollInstrOther:
	// permitted (the trace fits the cap and the lead has no side
	// effects the matcher can detect).
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, MaxBodyLen: 4, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrOther},
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	res := TryFastMMIOPoll(trace, pat)
	if !res.Matched {
		t.Fatal("lead Other within MaxBodyLen should match")
	}
	if res.LoadIdx != 1 || res.TestIdx != 2 || res.BranchIdx != 3 {
		t.Errorf("indices = (%d,%d,%d) want (1,2,3)", res.LoadIdx, res.TestIdx, res.BranchIdx)
	}
}

func TestTryFastMMIOPoll_RejectsExtraLoadInBody(t *testing.T) {
	// Lead slot is a second Load — any non-Other before the matched
	// load disqualifies. A second load means the body has more than
	// one MMIO read and the fast-path semantics no longer hold.
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, MaxBodyLen: 4, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0x1000},
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("extra Load in lead must disqualify")
	}
}

func TestTryFastMMIOPoll_RejectsBranchInBody(t *testing.T) {
	// Forward branch in the lead means the loop body has a second
	// branch — disqualifying.
	pat := &PollPattern{Load: PollLoad32, Test: PollTestCMPImm, Backward: true, MaxBodyLen: 4, AddressIsMMIOPredicate: allMMIO}
	trace := []PollInstr{
		{Kind: PollInstrBranchForward},
		{Kind: PollInstrLoad, LoadShape: PollLoad32, LoadAddr: 0xFFFF0000},
		{Kind: PollInstrTest, TestShape: PollTestCMPImm},
		{Kind: PollInstrBranchBackward},
	}
	if TryFastMMIOPoll(trace, pat).Matched {
		t.Fatal("branch in lead must disqualify")
	}
}

func TestDefaultPollIterationCap_NonZero(t *testing.T) {
	if DefaultPollIterationCap == 0 {
		t.Fatal("DefaultPollIterationCap must be > 0")
	}
}
