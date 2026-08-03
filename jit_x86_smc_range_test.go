// jit_x86_smc_range_test.go - x86 range-scoped SMC invalidation tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"encoding/binary"
	"testing"
)

func TestX86SMC_SharedPageBitmapSurvivesRangeInval(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 4)
	first := &JITBlock{startPC: 0x100, endPC: 0x110, chainEntry: 0xAAAA}
	second := &JITBlock{startPC: 0x180, endPC: 0x190, chainEntry: 0xBBBB}
	cache.Put(first)
	cache.Put(second)
	x86MarkCodePagesForBlock(bitmap, first)
	x86MarkCodePagesForBlock(bitmap, second)

	ctx := &X86JITContext{
		InvalAddr:     0x104,
		InvalSize:     1,
		RTSCache0PC:   uint32(first.startPC),
		RTSCache0Addr: first.chainEntry,
		RTSCache1PC:   uint32(second.startPC),
		RTSCache1Addr: second.chainEntry,
	}
	removed := x86InvalidateSMCRange(cache, bitmap, ctx)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := cache.Get(first.startPC); got != nil {
		t.Fatalf("invalidated block still cached: %#v", got)
	}
	if got := cache.Get(second.startPC); got != second {
		t.Fatalf("surviving block = %#v, want second block", got)
	}
	if bitmap[1] == 0 {
		t.Fatalf("shared code page bitmap was cleared while a surviving block still covers page 1")
	}
	if ctx.RTSCache0Addr != 0 || ctx.RTSCache1Addr != 0 {
		t.Fatalf("RTS cache not cleared after range invalidation: ctx=%+v", ctx)
	}
}

func TestX86SMC_RangePreservesUnrelatedPages(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 8)
	first := &JITBlock{startPC: 0x100, endPC: 0x110}
	second := &JITBlock{startPC: 0x300, endPC: 0x310}
	cache.Put(first)
	cache.Put(second)
	x86MarkCodePagesForBlock(bitmap, first)
	x86MarkCodePagesForBlock(bitmap, second)

	ctx := &X86JITContext{InvalAddr: 0x108, InvalSize: 4}
	removed := x86InvalidateSMCRange(cache, bitmap, ctx)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := cache.Get(second.startPC); got != second {
		t.Fatalf("unrelated block = %#v, want second block", got)
	}
	if bitmap[1] != 0 {
		t.Fatalf("invalidated page 1 still marked in code bitmap")
	}
	if bitmap[3] == 0 {
		t.Fatalf("unrelated page 3 was cleared from code bitmap")
	}
}

func TestX86SMC_ZeroSizeFallsBackToFullInvalidation(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 4)
	block := &JITBlock{startPC: 0x100, endPC: 0x110}
	cache.Put(block)
	x86MarkCodePagesForBlock(bitmap, block)

	x86InvalidateSMCRange(cache, bitmap, &X86JITContext{})
	if got := cache.Get(block.startPC); got != nil {
		t.Fatalf("block still cached after zero-size fallback: %#v", got)
	}
	if bitmap[1] != 0 {
		t.Fatalf("bitmap page remained marked after zero-size fallback")
	}
}

func TestX86SMC_PruneAuxBlockCacheDropsOnlyInvalidatedEntries(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 8)
	first := &JITBlock{startPC: 0x100, endPC: 0x110}
	second := &JITBlock{startPC: 0x300, endPC: 0x310}
	cache.Put(first)
	cache.Put(second)
	x86MarkCodePagesForBlock(bitmap, first)
	x86MarkCodePagesForBlock(bitmap, second)

	type auxBlock struct{ meta *JITBlock }
	aux := map[uint32]auxBlock{
		uint32(first.startPC):  {meta: first},
		uint32(second.startPC): {meta: second},
	}
	var cleared []uint32
	ctx := &X86JITContext{InvalAddr: 0x108, InvalSize: 1}
	if removed := x86InvalidateSMCRange(cache, bitmap, ctx); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	pruned := x86PruneAuxBlockCache(cache, aux, func(b auxBlock) *JITBlock { return b.meta }, func(pc uint32, _ auxBlock) {
		cleared = append(cleared, pc)
	})
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if len(cleared) != 1 || cleared[0] != uint32(first.startPC) {
		t.Fatalf("cleared = %v, want [%#x]", cleared, uint32(first.startPC))
	}
	if _, ok := aux[uint32(first.startPC)]; ok {
		t.Fatalf("invalidated block %#x still present in aux cache", uint32(first.startPC))
	}
	if got := aux[uint32(second.startPC)].meta; got != second {
		t.Fatalf("surviving aux block = %#v, want second block", got)
	}
}

func TestX86SMC_PruneAuxBlockCacheDropsReplacedEntries(t *testing.T) {
	cache := NewCodeCache()
	original := &JITBlock{startPC: 0x100, endPC: 0x110}
	replacement := &JITBlock{startPC: 0x100, endPC: 0x120}
	cache.Put(original)

	type auxBlock struct{ meta *JITBlock }
	aux := map[uint32]auxBlock{0x100: {meta: original}}
	cache.Put(replacement)

	pruned := x86PruneAuxBlockCache(cache, aux, func(b auxBlock) *JITBlock { return b.meta }, nil)
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if len(aux) != 0 {
		t.Fatalf("aux cache still contains replaced entry: %#v", aux)
	}
}

func TestX86SMC_WasmDriverCacheDropKeepsCollidingLiveEntry(t *testing.T) {
	const mask = uint32(0x0F)
	pcCache := make([]byte, (mask+1)*8)
	store := func(pc, slot uint32) {
		off := int((pc & mask) << 3)
		binary.LittleEndian.PutUint32(pcCache[off:off+4], pc)
		binary.LittleEndian.PutUint32(pcCache[off+4:off+8], slot)
	}
	read := func(pc uint32) (tag, slot uint32) {
		off := int((pc & mask) << 3)
		return readLE32(pcCache, uint32(off)), readLE32(pcCache, uint32(off+4))
	}

	store(0x110, 7)
	x86WasmDropDriverCacheEntry(pcCache, mask, 0x100)
	if tag, slot := read(0x110); tag != 0x110 || slot != 7 {
		t.Fatalf("colliding live entry cleared: tag=%#x slot=%d", tag, slot)
	}

	x86WasmDropDriverCacheEntry(pcCache, mask, 0x110)
	if tag, slot := read(0x110); tag != 0 || slot != 0 {
		t.Fatalf("owner entry not cleared: tag=%#x slot=%d", tag, slot)
	}
}

func TestX86SMC_WasmDriverCacheResetRewindsSlots(t *testing.T) {
	pcCache := make([]byte, 16)
	binary.LittleEndian.PutUint32(pcCache[0:4], 0x100)
	binary.LittleEndian.PutUint32(pcCache[4:8], 3)
	binary.LittleEndian.PutUint32(pcCache[8:12], 0x200)
	binary.LittleEndian.PutUint32(pcCache[12:16], 4)
	nextSlot := 9

	x86WasmResetDriverCache(pcCache, &nextSlot)

	for i, b := range pcCache {
		if b != 0 {
			t.Fatalf("pcCache[%d]=%#x want 0", i, b)
		}
	}
	if nextSlot != 0 {
		t.Fatalf("nextSlot=%d want 0", nextSlot)
	}
}
