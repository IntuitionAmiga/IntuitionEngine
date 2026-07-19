// jit_m68k_observed_region_test.go - Milestone 7 observed regions.
// Recorder-protocol unit tests, path-admission tests, and an integration
// test installing an observed region through the dispatcher hook.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestM68KJIT_ObservedRecorderProtocol(t *testing.T) {
	var r m68kObservedRecorder

	// Path A -> B -> A closes.
	r.start(0x100, 7)
	if done, reject := r.appendSuccessor(0x200); done || reject {
		t.Fatal("B should extend the path")
	}
	done, reject := r.appendSuccessor(0x100)
	if !done || reject {
		t.Fatalf("closing on the entry: done=%v reject=%v", done, reject)
	}
	if got := r.path(); len(got) != 2 || got[0] != 0x100 || got[1] != 0x200 {
		t.Fatalf("path: %v", got)
	}

	// Interior revisit rejects.
	r.start(0x100, 7)
	r.appendSuccessor(0x200)
	r.appendSuccessor(0x300)
	if done, reject := r.appendSuccessor(0x200); done || !reject {
		t.Fatal("interior revisit must reject")
	}

	// Immediate self-loop rejects (single block, no region payoff).
	r.start(0x100, 7)
	if done, reject := r.appendSuccessor(0x100); done || !reject {
		t.Fatal("self-loop must reject")
	}

	// Cap closes the path.
	r.start(0x100, 7)
	var lastDone bool
	for i := uint32(1); i < m68kObservedMaxBlocks; i++ {
		lastDone, _ = r.appendSuccessor(0x100 + i*0x100)
	}
	if !lastDone {
		t.Fatal("cap did not close the path")
	}
}

func TestM68KJIT_ObservedRegionBuild(t *testing.T) {
	mem := make([]byte, 0x10000)
	w := func(pc uint32, words ...uint16) {
		for i, x := range words {
			mem[pc+uint32(i*2)] = byte(x >> 8)
			mem[pc+uint32(i*2)+1] = byte(x)
		}
	}
	// Two straight-line blocks joined dynamically (Bcc taken edge that the
	// static walker cannot follow): each ends in a terminator.
	w(0x100, 0x7005, 0x6700, 0x0100) // MOVEQ; BEQ.W +0x100
	w(0x204, 0x5280, 0x4E75)         // ADDQ; RTS

	region := m68kBuildObservedRegion([]uint32{0x100, 0x204}, mem)
	if region == nil || len(region.blocks) != 2 {
		t.Fatalf("observed region not built: %+v", region)
	}
	// Path with an empty/unsafe block refuses.
	if region := m68kBuildObservedRegion([]uint32{0x100, 0x20000}, mem); region != nil {
		t.Fatal("unsafe path built a region")
	}
}

// Integration: the dispatcher hook compiles and installs the observed
// region once the path closes.
func TestM68KJIT_ObservedRegionInstall(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	if m68kJITObservedRegionsDisabled {
		t.Skip("observed regions disabled")
	}
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	defer cpu.freeM68KJIT()

	w := func(pc uint32, words ...uint16) {
		for i, x := range words {
			cpu.Write16(pc+uint32(i*2), x)
		}
	}
	// A @0x1000: MOVEQ; BEQ.W to B (dynamic edge). B @0x1104: ADDQ; BRA.W
	// back to A (dynamic-looking for the recorder's purposes).
	w(0x1000, 0x7000, 0x6700, 0x0100)
	w(0x1104, 0x5281, 0x6000, 0xFEFA)

	before := m68kObservedRegionPromotions.Load()
	cpu.m68kJitObserved.start(0x1000, cpu.m68kJitInvalGen.Load())
	cpu.m68kObserveJITDispatch(0x1104, cpu.m68kGetJITExecMem(), false)
	cpu.m68kObserveJITDispatch(0x1000, cpu.m68kGetJITExecMem(), false)

	if m68kObservedRegionPromotions.Load() == before {
		t.Fatal("observed region was not installed")
	}
	block := cpu.m68kJitCache.Get(0x1000)
	if block == nil || block.tier != 1 {
		t.Fatalf("observed region block missing or wrong tier: %+v", block)
	}
	covered := JITBlockCoveredRanges(block)
	if len(covered) != 2 {
		t.Fatalf("observed region covered ranges: %v", covered)
	}
}
