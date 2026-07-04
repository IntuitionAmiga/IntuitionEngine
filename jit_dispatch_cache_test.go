package main

import "testing"

func newDispatchCacheForTest() *CodeCache {
	cc := NewCodeCache()
	if cc.dispatch == nil {
		cc.dispatch = newJITDispatchCache()
	}
	return cc
}

func TestDispatchCache_HitMissEvict(t *testing.T) {
	cc := newDispatchCacheForTest()
	blockA := &JITBlock{startPC: 0x1000, endPC: 0x1004}
	cc.Put(blockA)

	if got := cc.Get(blockA.startPC); got != blockA {
		t.Fatalf("Get(blockA) = %p, want %p", got, blockA)
	}
	if got := cc.dispatch.get(blockA.startPC, 0, cc.generation); got != blockA {
		t.Fatalf("dispatch cache hit = %p, want %p", got, blockA)
	}

	tagA := jitDispatchCacheTag(blockA.startPC, 0)
	var collidingPC uint64
	for pc := uint64(0x1001); pc < 0x200000; pc++ {
		if jitDispatchCacheIndex(jitDispatchCacheTag(pc, 0)) == jitDispatchCacheIndex(tagA) &&
			jitDispatchCacheTag(pc, 0) != tagA {
			collidingPC = pc
			break
		}
	}
	if collidingPC == 0 {
		t.Fatal("test could not find dispatch-cache collision")
	}

	blockB := &JITBlock{startPC: collidingPC, endPC: collidingPC + 4}
	cc.Put(blockB)
	if got := cc.dispatch.get(blockA.startPC, 0, cc.generation); got != nil {
		t.Fatalf("dispatch cache retained evicted blockA entry: %p", got)
	}
	if got := cc.Get(blockA.startPC); got != blockA {
		t.Fatalf("map fallback after dispatch eviction = %p, want %p", got, blockA)
	}
}

func TestDispatchCache_GenerationInvalidation(t *testing.T) {
	cc := newDispatchCacheForTest()
	block := &JITBlock{startPC: 0x2000, endPC: 0x2010}
	cc.Put(block)
	if got := cc.Get(0x2000); got != block {
		t.Fatalf("Get before invalidation = %p, want %p", got, block)
	}
	oldGeneration := cc.generation

	cc.InvalidateRange(0x2000, 0x2004)

	if cc.generation == oldGeneration {
		t.Fatal("generation did not advance after range invalidation")
	}
	if got := cc.dispatch.get(0x2000, 0, oldGeneration); got != nil {
		t.Fatalf("old-generation dispatch entry survived invalidation: %p", got)
	}
	if got := cc.Get(0x2000); got != nil {
		t.Fatalf("Get after invalidation = %p, want nil", got)
	}
}

func TestDispatchCache_MMUKeying(t *testing.T) {
	cc := newDispatchCacheForTest()
	const vPC = uint64(0x4000)
	const ptbrA = uint64(0x100000)
	const ptbrB = uint64(0x200000)
	blockA := &JITBlock{startPC: vPC, endPC: vPC + 4}
	blockB := &JITBlock{startPC: vPC, endPC: vPC + 4}

	cc.PutMMU(ptbrA, vPC, blockA)
	cc.PutMMU(ptbrB, vPC, blockB)

	if got := cc.GetMMU(ptbrA, vPC); got != blockA {
		t.Fatalf("GetMMU(ptbrA) = %p, want %p", got, blockA)
	}
	if got := cc.GetMMU(ptbrB, vPC); got != blockB {
		t.Fatalf("GetMMU(ptbrB) = %p, want %p", got, blockB)
	}
	if blockA.ptbr != ptbrA || blockB.ptbr != ptbrB {
		t.Fatalf("PutMMU did not stamp ptbr: blockA=%x blockB=%x", blockA.ptbr, blockB.ptbr)
	}
}

func TestDispatchCache_RejectsHashTagCollisionWithDifferentKey(t *testing.T) {
	dc := newJITDispatchCache()
	const pc = uint64(0x4000)
	const ptbr = uint64(0x100000)
	block := &JITBlock{startPC: 0xDEAD, endPC: 0xDEB1}
	tag := jitDispatchCacheTag(pc, ptbr)
	dc.entries[jitDispatchCacheIndex(tag)] = jitDispatchCacheEntry{
		tag:        tag,
		pc:         pc + 4,
		ptbr:       ptbr,
		generation: 7,
		block:      block,
	}

	if got := dc.get(pc, ptbr, 7); got != nil {
		t.Fatalf("dispatch cache returned block for mismatched exact key: %p", got)
	}

	dc.entries[jitDispatchCacheIndex(tag)].pc = pc
	dc.entries[jitDispatchCacheIndex(tag)].ptbr = ptbr + 0x1000
	if got := dc.get(pc, ptbr, 7); got != nil {
		t.Fatalf("dispatch cache returned block for mismatched PTBR: %p", got)
	}
}
