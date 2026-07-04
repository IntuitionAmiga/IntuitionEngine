// jit_x86_smc_range_test.go - x86 range-scoped SMC invalidation tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import "testing"

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
