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

func TestIE64SMC_OverlapIndexDropsReplacedAndRemovedBlocks(t *testing.T) {
	cache := NewCodeCache()
	first := &JITBlock{startPC: 0x100, endPC: 0x110}
	replacement := &JITBlock{startPC: 0x100, endPC: 0x130, coveredRanges: [][2]uint64{{0x120, 0x130}}}
	other := &JITBlock{startPC: 0x300, endPC: 0x310}

	cache.Put(first)
	if !cache.OverlapsRange(0x108, 0x10C) {
		t.Fatal("initial block did not overlap indexed range")
	}

	cache.Put(replacement)
	if cache.OverlapsRange(0x108, 0x10C) {
		t.Fatal("replaced block left stale overlap for old covered range")
	}
	if !cache.OverlapsRange(0x120, 0x124) {
		t.Fatal("replacement block did not overlap new covered range")
	}

	cache.Put(other)
	if !cache.RemoveBlock(replacement) {
		t.Fatal("RemoveBlock returned false for cached replacement")
	}
	if cache.OverlapsRange(0x120, 0x124) {
		t.Fatal("removed block left stale overlap in page index")
	}
	if !cache.OverlapsRange(0x308, 0x30C) {
		t.Fatal("unrelated cached block disappeared from page index")
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

func TestIE64SMC_MarkWriteIgnoresMarkedPageOutsideCachedCode(t *testing.T) {
	cpu := &CPU64{
		jitCtx:            &JITContext{},
		jitCache:          NewCodeCache(),
		jitCodePageBitmap: make([]byte, 4),
	}
	block := &JITBlock{startPC: 0x100, endPC: 0x110}
	cpu.jitCache.Put(block)
	ie64MarkCodePagesForBlock(cpu.jitCodePageBitmap, block)

	cpu.markJITSMCWrite(0x180, 4)
	if cpu.jitCtx.NeedInval != 0 {
		t.Fatalf("NeedInval = %d, want 0 for marked page outside cached code", cpu.jitCtx.NeedInval)
	}
	if cpu.jitCtx.InvalAddr != 0 || cpu.jitCtx.InvalSize != 0 {
		t.Fatalf("invalidation range = [0x%X,+%d), want cleared", cpu.jitCtx.InvalAddr, cpu.jitCtx.InvalSize)
	}
}

func TestIE64SMC_MarkWriteIgnoresHighMarkedSpanOutsideCachedCode(t *testing.T) {
	const highPC = uint64(0x1_0000)
	cpu := &CPU64{
		jitCtx:            &JITContext{},
		jitCache:          NewCodeCache(),
		jitCodePageBitmap: make([]byte, 4),
	}
	block := &JITBlock{startPC: highPC, endPC: highPC + IE64_INSTR_SIZE}
	cpu.jitCache.Put(block)
	ie64MarkCodePagesForBlockContext(cpu.jitCodePageBitmap, cpu.jitCtx, block)

	cpu.markJITSMCWrite(highPC+0x80, 4)
	if cpu.jitCtx.NeedInval != 0 {
		t.Fatalf("NeedInval = %d, want 0 for high marked span outside cached code", cpu.jitCtx.NeedInval)
	}
	if cpu.jitCtx.InvalAddr != 0 || cpu.jitCtx.InvalSize != 0 {
		t.Fatalf("invalidation range = [0x%X,+%d), want cleared", cpu.jitCtx.InvalAddr, cpu.jitCtx.InvalSize)
	}
}

func TestIE64SMC_MarkWriteMMUScopesOverlapToCurrentPTBR(t *testing.T) {
	mmuTestResetPools()
	defer mmuTestResetPools()

	const (
		currentPTBR = uint64(0x100000)
		otherPTBR   = uint64(0x200000)
		va          = uint64(0x3100)
		currentPhys = uint64(0x8000)
		otherPhys   = uint64(0x9000)
	)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitCtx = &JITContext{}
	cpu.jitCache = NewCodeCache()
	cpu.jitCodePageBitmap = make([]byte, 0x40)
	cpu.mmuEnabled = true
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	cpu.ptbr = currentPTBR
	writePTE(cpu, va>>MMU_PAGE_SHIFT, makePTE(currentPhys>>MMU_PAGE_SHIFT, flags))
	cpu.ptbr = otherPTBR
	writePTE(cpu, va>>MMU_PAGE_SHIFT, makePTE(otherPhys>>MMU_PAGE_SHIFT, flags))
	cpu.ptbr = currentPTBR
	cpu.tlbFlush()

	otherBlock := &JITBlock{startPC: va, endPC: va + IE64_INSTR_SIZE}
	cpu.jitCache.PutMMU(otherPTBR, va, otherBlock)
	ie64MarkCodePagesForBlockContext(cpu.jitCodePageBitmap, cpu.jitCtx, otherBlock)

	cpu.markJITSMCWrite(va, IE64_INSTR_SIZE)
	if cpu.jitCtx.NeedInval != 0 {
		t.Fatalf("NeedInval = %d, want 0 for same-VA block in another PTBR with different physical backing", cpu.jitCtx.NeedInval)
	}
	if got := cpu.jitCache.GetMMU(otherPTBR, va); got != otherBlock {
		t.Fatalf("other PTBR block changed during mark: %#v", got)
	}
}

func TestIE64SMC_MarkWriteMMUPhysicalAliasOtherPTBRForcesFullInvalidation(t *testing.T) {
	mmuTestResetPools()
	defer mmuTestResetPools()

	const (
		currentPTBR = uint64(0x300000)
		otherPTBR   = uint64(0x400000)
		va          = uint64(0x3100)
		sharedPhys  = uint64(0x9000)
	)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitCtx = &JITContext{}
	cpu.jitCache = NewCodeCache()
	cpu.jitCodePageBitmap = make([]byte, 0x40)
	cpu.mmuEnabled = true
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	cpu.ptbr = currentPTBR
	writePTE(cpu, va>>MMU_PAGE_SHIFT, makePTE(sharedPhys>>MMU_PAGE_SHIFT, flags))
	cpu.ptbr = otherPTBR
	writePTE(cpu, va>>MMU_PAGE_SHIFT, makePTE(sharedPhys>>MMU_PAGE_SHIFT, flags))
	cpu.ptbr = currentPTBR
	cpu.tlbFlush()

	otherBlock := &JITBlock{startPC: va, endPC: va + IE64_INSTR_SIZE}
	cpu.jitCache.PutMMU(otherPTBR, va, otherBlock)
	ie64MarkCodePagesForBlockContext(cpu.jitCodePageBitmap, cpu.jitCtx, otherBlock)
	if !ie64PageMarked(cpu.jitCodePageBitmap, cpu.jitCtx, va, IE64_INSTR_SIZE) {
		t.Fatal("test setup did not mark the written virtual page")
	}

	cpu.markJITSMCWrite(va, IE64_INSTR_SIZE)
	if cpu.jitCtx.NeedInval != 1 {
		t.Fatal("NeedInval was not set for MMU physical alias in another PTBR")
	}
	if cpu.jitCtx.InvalSize != 0 {
		t.Fatalf("InvalSize = %d, want 0 to force full invalidation for cross-PTBR physical alias", cpu.jitCtx.InvalSize)
	}
	if got := cpu.jitCache.GetMMU(otherPTBR, va); got != otherBlock {
		t.Fatalf("marking should defer cache mutation to dispatcher, got %#v", got)
	}
}

func TestIE64SMC_DispatchMMUPhysicalAliasOtherPTBRInvalidatesWholeCache(t *testing.T) {
	const (
		currentPTBR = uint64(0x500000)
		otherPTBR   = uint64(0x600000)
		va          = uint64(0x3100)
	)
	cpu := &CPU64{
		jitCtx:                &JITContext{},
		jitCache:              NewCodeCache(),
		jitCodePageBitmap:     make([]byte, 0x40),
		jitPhysCodePageBitmap: make([]byte, 0x40),
		mmuEnabled:            true,
		ptbr:                  currentPTBR,
	}
	otherBlock := &JITBlock{startPC: va, endPC: va + IE64_INSTR_SIZE}
	cpu.jitCache.PutMMU(otherPTBR, va, otherBlock)
	ie64MarkCodePagesForBlockContext(cpu.jitCodePageBitmap, cpu.jitCtx, otherBlock)
	cpu.jitCtx.NeedInval = 1
	cpu.jitCtx.InvalSize = 0

	if !cpu.handleJITSMCInvalidation(otherBlock, nil) {
		t.Fatal("dispatcher did not process zero-size MMU physical alias invalidation")
	}
	if got := cpu.jitCache.GetMMU(otherPTBR, va); got != nil {
		t.Fatalf("cross-PTBR alias block still cached after full invalidation: %#v", got)
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

func TestIE64SMC_DispatchIgnoresNativeStoreMarkedPageOutsideCachedCode(t *testing.T) {
	cpu := &CPU64{
		jitCtx:            &JITContext{},
		jitCache:          NewCodeCache(),
		jitCodePageBitmap: make([]byte, 4),
	}
	block := &JITBlock{startPC: 0x100, endPC: 0x110, chainEntry: 0xAAAA}
	cpu.jitCache.Put(block)
	ie64MarkCodePagesForBlock(cpu.jitCodePageBitmap, block)
	cpu.jitCtx.NeedInval = 1
	cpu.jitCtx.InvalAddr = 0x180
	cpu.jitCtx.InvalSize = 4
	cpu.jitCtx.RTSCache0PC = block.startPC
	cpu.jitCtx.RTSCache0Addr = block.chainEntry
	baseInvalidations := globalIE64JITStats.invalidations.Load()

	if cpu.handleJITSMCInvalidation(block, nil) {
		t.Fatal("dispatcher reported an invalidation for a marked-page false positive")
	}
	if cpu.jitCtx.NeedInval != 0 || cpu.jitCtx.InvalAddr != 0 || cpu.jitCtx.InvalSize != 0 {
		t.Fatalf("dispatcher did not clear false-positive invalidation: ctx=%+v", cpu.jitCtx)
	}
	if got := cpu.jitCache.Get(block.startPC); got != block {
		t.Fatalf("cached block changed after false-positive invalidation: %#v", got)
	}
	if cpu.jitCtx.RTSCache0Addr != block.chainEntry {
		t.Fatalf("RTS cache was cleared for false positive: got %#x want %#x", cpu.jitCtx.RTSCache0Addr, block.chainEntry)
	}
	if got := globalIE64JITStats.invalidations.Load(); got != baseInvalidations {
		t.Fatalf("invalidations = %d, want unchanged %d", got, baseInvalidations)
	}
	if block.dominantDeopt != DeoptNone {
		t.Fatalf("dominantDeopt = %v, want none for false positive", block.dominantDeopt)
	}
}

func TestIE64SMC_WriteOverlappingCachedCodeStillInvalidates(t *testing.T) {
	cpu := &CPU64{
		jitCtx:            &JITContext{},
		jitCache:          NewCodeCache(),
		jitCodePageBitmap: make([]byte, 4),
	}
	first := &JITBlock{startPC: 0x100, endPC: 0x110, chainEntry: 0xAAAA}
	second := &JITBlock{startPC: 0x180, endPC: 0x190, chainEntry: 0xBBBB}
	cpu.jitCache.Put(first)
	cpu.jitCache.Put(second)
	ie64MarkCodePagesForBlock(cpu.jitCodePageBitmap, first)
	ie64MarkCodePagesForBlock(cpu.jitCodePageBitmap, second)
	cpu.jitCtx.NeedInval = 1
	cpu.jitCtx.InvalAddr = 0x104
	cpu.jitCtx.InvalSize = 4
	cpu.jitCtx.RTSCache0PC = first.startPC
	cpu.jitCtx.RTSCache0Addr = first.chainEntry
	cpu.jitCtx.RTSCache1PC = second.startPC
	cpu.jitCtx.RTSCache1Addr = second.chainEntry
	baseInvalidations := globalIE64JITStats.invalidations.Load()

	if !cpu.handleJITSMCInvalidation(first, nil) {
		t.Fatal("dispatcher did not process overlapping SMC write")
	}
	if cpu.jitCtx.NeedInval != 0 || cpu.jitCtx.InvalAddr != 0 || cpu.jitCtx.InvalSize != 0 {
		t.Fatalf("dispatcher did not clear processed invalidation: ctx=%+v", cpu.jitCtx)
	}
	if got := cpu.jitCache.Get(first.startPC); got != nil {
		t.Fatalf("overlapping block still cached: %#v", got)
	}
	if got := cpu.jitCache.Get(second.startPC); got != second {
		t.Fatalf("same-page non-overlapping block = %#v, want survivor", got)
	}
	if cpu.jitCodePageBitmap[1] == 0 {
		t.Fatal("surviving same-page block was not rebuilt into code bitmap")
	}
	if cpu.jitCtx.RTSCache0Addr != 0 || cpu.jitCtx.RTSCache1Addr != 0 {
		t.Fatalf("RTS cache not cleared after real invalidation: ctx=%+v", cpu.jitCtx)
	}
	if got := globalIE64JITStats.invalidations.Load(); got != baseInvalidations+1 {
		t.Fatalf("invalidations = %d, want %d", got, baseInvalidations+1)
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
