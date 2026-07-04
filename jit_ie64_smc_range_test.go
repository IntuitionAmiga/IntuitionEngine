// jit_ie64_smc_range_test.go - IE64 range-scoped SMC invalidation tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "testing"

func TestIE64SMC_SharedPageBitmapSurvivesRangeInval(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 4)
	first := &JITBlock{startPC: 0x100, endPC: 0x110, chainEntry: 0xAAAA}
	second := &JITBlock{startPC: 0x180, endPC: 0x190, chainEntry: 0xBBBB}
	cache.Put(first)
	cache.Put(second)
	ie64MarkCodePagesForBlock(bitmap, first)
	ie64MarkCodePagesForBlock(bitmap, second)

	ctx := &JITContext{
		InvalAddr:     0x104,
		InvalSize:     4,
		RTSCache0PC:   first.startPC,
		RTSCache0Addr: first.chainEntry,
		RTSCache1PC:   second.startPC,
		RTSCache1Addr: second.chainEntry,
	}
	removed := ie64InvalidateSMCRange(cache, bitmap, ctx)
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
		t.Fatal("shared code page bitmap was cleared while a surviving block still covers page 1")
	}
	if ctx.RTSCache0Addr != 0 || ctx.RTSCache1Addr != 0 {
		t.Fatalf("RTS cache not cleared after range invalidation: ctx=%+v", ctx)
	}
}

func TestIE64SMC_RangePreservesUnrelatedPages(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 8)
	first := &JITBlock{startPC: 0x100, endPC: 0x110}
	second := &JITBlock{startPC: 0x300, endPC: 0x310}
	cache.Put(first)
	cache.Put(second)
	ie64MarkCodePagesForBlock(bitmap, first)
	ie64MarkCodePagesForBlock(bitmap, second)

	ctx := &JITContext{InvalAddr: 0x108, InvalSize: 8}
	removed := ie64InvalidateSMCRange(cache, bitmap, ctx)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := cache.Get(second.startPC); got != second {
		t.Fatalf("unrelated block = %#v, want second block", got)
	}
	if bitmap[1] != 0 {
		t.Fatal("invalidated page 1 still marked in code bitmap")
	}
	if bitmap[3] == 0 {
		t.Fatal("unrelated page 3 was cleared from code bitmap")
	}
}

func TestIE64SMC_MarkWriteSetsExactRange(t *testing.T) {
	cpu := &CPU64{
		jitCtx:            &JITContext{},
		jitCodePageBitmap: make([]byte, 8),
	}
	cpu.jitCodePageBitmap[2] = 1

	cpu.markJITSMCWrite(0x1FC, 8)
	if cpu.jitCtx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for a write that crosses into a code page")
	}
	if cpu.jitCtx.InvalAddr != 0x1FC || cpu.jitCtx.InvalSize != 8 {
		t.Fatalf("invalid range = [0x%X, +%d), want [0x1FC, +8)", cpu.jitCtx.InvalAddr, cpu.jitCtx.InvalSize)
	}
}

func TestIE64SMC_HighCodePageTrackedOutsideBitmap(t *testing.T) {
	const highPC = uint64(0x1_0000)
	cpu := &CPU64{
		jitCtx:            &JITContext{},
		jitCodePageBitmap: make([]byte, 4),
	}
	block := &JITBlock{startPC: highPC, endPC: highPC + IE64_INSTR_SIZE}

	ie64MarkCodePagesForBlockContext(cpu.jitCodePageBitmap, cpu.jitCtx, block)
	wantPage := highPC >> 8
	if cpu.jitCtx.CodeHighStartPage != wantPage || cpu.jitCtx.CodeHighEndPage != wantPage {
		t.Fatalf("high code span = [%d,%d], want [%d,%d]",
			cpu.jitCtx.CodeHighStartPage, cpu.jitCtx.CodeHighEndPage, wantPage, wantPage)
	}
	for i, v := range cpu.jitCodePageBitmap {
		if v != 0 {
			t.Fatalf("low bitmap[%d] = %d, want 0 for high-only block", i, v)
		}
	}

	cpu.markJITSMCWrite(highPC, IE64_INSTR_SIZE)
	if cpu.jitCtx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for a write to high compiled code")
	}
	if cpu.jitCtx.InvalAddr != highPC || cpu.jitCtx.InvalSize != IE64_INSTR_SIZE {
		t.Fatalf("invalid range = [0x%X,+%d), want [0x%X,+%d)",
			cpu.jitCtx.InvalAddr, cpu.jitCtx.InvalSize, highPC, IE64_INSTR_SIZE)
	}
}

func TestIE64SMC_MMUAliasForcesFullInvalidation(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitCtx = &JITContext{}
	cpu.jitCache = NewCodeCache()
	cpu.jitCodePageBitmap = make([]byte, len(cpu.memory)>>8)
	setupIdentityMMU(cpu, 16)

	const (
		vaCode  = uint64(0x3000)
		vaAlias = uint64(0x7000)
		phys    = uint64(0x5000)
	)
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	writePTE(cpu, vaCode>>MMU_PAGE_SHIFT, makePTE(phys>>MMU_PAGE_SHIFT, flags))
	writePTE(cpu, vaAlias>>MMU_PAGE_SHIFT, makePTE(phys>>MMU_PAGE_SHIFT, flags))
	cpu.tlbFlush()

	block := &JITBlock{startPC: vaCode, endPC: vaCode + IE64_INSTR_SIZE}
	cpu.jitCache.PutMMU(cpu.ptbr, vaCode, block)
	ie64MarkCodePagesForBlockContext(cpu.jitCodePageBitmap, cpu.jitCtx, block)

	if ie64PageMarked(cpu.jitCodePageBitmap, cpu.jitCtx, vaAlias, 4) {
		t.Fatal("test setup invalid: alias VA is already virtually marked")
	}
	cpu.markJITSMCWrite(vaAlias, 4)
	if cpu.jitCtx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for MMU physical alias SMC")
	}
	if cpu.jitCtx.InvalSize != 0 {
		t.Fatalf("InvalSize = %d, want 0 to force full invalidation for MMU alias", cpu.jitCtx.InvalSize)
	}
}

func TestIE64SMC_HighCodePageSpanRebuiltAfterRangeInvalidation(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 4)
	ctx := &JITContext{}
	first := &JITBlock{startPC: 0x1_0000, endPC: 0x1_0008}
	second := &JITBlock{startPC: 0x2_0000, endPC: 0x2_0008}
	cache.Put(first)
	cache.Put(second)
	ie64MarkCodePagesForBlockContext(bitmap, ctx, first)
	ie64MarkCodePagesForBlockContext(bitmap, ctx, second)

	ctx.InvalAddr = first.startPC
	ctx.InvalSize = IE64_INSTR_SIZE
	removed := ie64InvalidateSMCRange(cache, bitmap, ctx)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := cache.Get(first.startPC); got != nil {
		t.Fatalf("first high block still cached: %#v", got)
	}
	if got := cache.Get(second.startPC); got != second {
		t.Fatalf("second high block = %#v, want survivor", got)
	}
	wantPage := second.startPC >> 8
	if ctx.CodeHighStartPage != wantPage || ctx.CodeHighEndPage != wantPage {
		t.Fatalf("rebuilt high code span = [%d,%d], want [%d,%d]",
			ctx.CodeHighStartPage, ctx.CodeHighEndPage, wantPage, wantPage)
	}
}

func TestIE64SMC_ZeroSizeFallsBackToFullInvalidation(t *testing.T) {
	cache := NewCodeCache()
	bitmap := make([]byte, 4)
	block := &JITBlock{startPC: 0x100, endPC: 0x110}
	cache.Put(block)
	ie64MarkCodePagesForBlock(bitmap, block)

	ie64InvalidateSMCRange(cache, bitmap, &JITContext{})
	if got := cache.Get(block.startPC); got != nil {
		t.Fatalf("block still cached after zero-size fallback: %#v", got)
	}
	if bitmap[1] != 0 {
		t.Fatal("bitmap page remained marked after zero-size fallback")
	}
}
