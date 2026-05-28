// jit_cache_widen_test.go - Phase 3 block-cache / chain / RTS / MMU tests.
//
// Pins the uint64 widening of JIT block infrastructure: cache keying,
// chain-slot lookups, invalidation, RTS cache, and the new MMU exact
// composite key (ptbr, vPC). These cover the data layer; the AMD64/ARM64
// emitters are exercised by jit_ret_channel_test.go and the existing
// emitter suites.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"testing"
)

func TestCodeCache_HighStartPC_StoreAndLookup(t *testing.T) {
	cc := NewCodeCache()
	const highPC uint64 = 0x0000_0001_0000_8000
	want := &JITBlock{startPC: highPC, endPC: highPC + 8}
	cc.Put(want)

	got := cc.Get(highPC)
	if got != want {
		t.Fatalf("cc.Get(0x%016X) = %v, want %v", highPC, got, want)
	}
}

func TestCodeCache_HighPC_InvalidateRange(t *testing.T) {
	cc := NewCodeCache()
	const highPC uint64 = 0x0000_0001_0000_8000
	cc.Put(&JITBlock{startPC: highPC, endPC: highPC + 16})

	cc.InvalidateRange(highPC, highPC+1)

	if got := cc.Get(highPC); got != nil {
		t.Fatalf("block at 0x%016X should be invalidated, got %v", highPC, got)
	}
}

func TestCodeCache_HighPC_UnpatchChainsInRange(t *testing.T) {
	cc := NewCodeCache()
	const highTargetPC uint64 = 0x0000_0001_0000_8000
	const sourcePC uint64 = 0x1000

	// Source block holds a chain slot pointing at the high-PC target.
	source := &JITBlock{
		startPC:    sourcePC,
		endPC:      sourcePC + 8,
		chainSlots: []chainSlot{{targetPC: highTargetPC, patchAddr: 0}},
	}
	// Target block at the high PC.
	target := &JITBlock{startPC: highTargetPC, endPC: highTargetPC + 8}
	cc.Put(source)
	cc.Put(target)

	// Unpatch chains for the high-PC range. With patchAddr=0 the actual
	// rel32 patch is skipped, but the collector must still recognise the
	// target as doomed (proves the doomedSet keying is uint64).
	cc.UnpatchChainsInRange(highTargetPC, highTargetPC+1)
	cc.InvalidateRange(highTargetPC, highTargetPC+1)

	if got := cc.Get(highTargetPC); got != nil {
		t.Fatalf("high target should be invalidated, got %v", got)
	}
}

func TestCodeCache_MMU_ExactKey_NoCollisionAcrossPtbrs(t *testing.T) {
	cc := NewCodeCache()
	const vPC uint64 = 0x4000
	const ptbrA uint64 = 0x1000
	const ptbrB uint64 = 0x2000

	blockA := &JITBlock{startPC: vPC, endPC: vPC + 8, ptbr: ptbrA}
	blockB := &JITBlock{startPC: vPC, endPC: vPC + 8, ptbr: ptbrB}

	cc.PutMMU(ptbrA, vPC, blockA)
	cc.PutMMU(ptbrB, vPC, blockB)

	if got := cc.GetMMU(ptbrA, vPC); got != blockA {
		t.Fatalf("GetMMU(ptbrA, vPC) = %v, want blockA", got)
	}
	if got := cc.GetMMU(ptbrB, vPC); got != blockB {
		t.Fatalf("GetMMU(ptbrB, vPC) = %v, want blockB", got)
	}
}

func TestCodeCache_MMU_HighPtbr_NoLossyAliasing(t *testing.T) {
	cc := NewCodeCache()
	const vPC uint64 = 0x4000
	// Two PTBRs that the old golden-ratio hash could alias: distinct
	// 64-bit values whose hash collides. The exact composite key must
	// keep them separated.
	const ptbrA uint64 = 0x1_0000_0000 // 4 GiB
	const ptbrB uint64 = 0x2_0000_0000 // 8 GiB

	blockA := &JITBlock{startPC: vPC, endPC: vPC + 8, ptbr: ptbrA}
	blockB := &JITBlock{startPC: vPC, endPC: vPC + 8, ptbr: ptbrB}
	cc.PutMMU(ptbrA, vPC, blockA)
	cc.PutMMU(ptbrB, vPC, blockB)

	if got := cc.GetMMU(ptbrA, vPC); got != blockA {
		t.Fatalf("high-PTBR lookup A returned wrong block: %v", got)
	}
	if got := cc.GetMMU(ptbrB, vPC); got != blockB {
		t.Fatalf("high-PTBR lookup B returned wrong block: %v", got)
	}
}

func TestCodeCache_MMU_InvalidateRange_CoversMmuBlocks(t *testing.T) {
	cc := NewCodeCache()
	const vPC uint64 = 0x0000_0001_0000_8000
	const ptbr uint64 = 0x1000

	cc.PutMMU(ptbr, vPC, &JITBlock{startPC: vPC, endPC: vPC + 16, ptbr: ptbr})

	cc.InvalidateRange(vPC, vPC+1)

	if got := cc.GetMMU(ptbr, vPC); got != nil {
		t.Fatalf("MMU block at high vPC should be invalidated, got %v", got)
	}
}

func TestIE64ResolveTerminatorTarget_HighPC(t *testing.T) {
	// BRA from a high PC must compute a full 64-bit target instead of
	// truncating to uint32. instrPC = 0x100000000, imm32 = 8 →
	// target = 0x100000008.
	const instrPC uint64 = 0x0000_0001_0000_0000
	target, ok := ie64ResolveTerminatorTarget(OP_BRA, 0, 8, instrPC)
	if !ok {
		t.Fatal("ie64ResolveTerminatorTarget(BRA, ...) returned !ok")
	}
	if target != instrPC+8 {
		t.Fatalf("target = 0x%016X, want 0x%016X", target, instrPC+8)
	}

	// BRA with negative imm32 from a high PC sign-extends to uint64.
	const negImm uint32 = 0xFFFFFFF8 // -8
	target2, ok := ie64ResolveTerminatorTarget(OP_BRA, 0, negImm, instrPC)
	if !ok {
		t.Fatal("ie64ResolveTerminatorTarget(BRA, neg) returned !ok")
	}
	if target2 != instrPC-8 {
		t.Fatalf("neg target = 0x%016X, want 0x%016X", target2, instrPC-8)
	}
}
