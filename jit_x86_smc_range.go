// jit_x86_smc_range.go - x86 range-scoped SMC helpers.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

func x86MarkCodePagesForBlock(bitmap []byte, block *JITBlock) {
	if len(bitmap) == 0 || block == nil {
		return
	}
	for _, r := range JITBlockCoveredRanges(block) {
		if r[1] <= r[0] {
			continue
		}
		startPage := r[0] >> 8
		endPage := (r[1] - 1) >> 8
		for p := startPage; p <= endPage; p++ {
			if p >= uint64(len(bitmap)) {
				break
			}
			bitmap[p] = 1
		}
	}
}

func x86RebuildCodePageBitmapFromCache(bitmap []byte, cache *CodeCache) {
	if len(bitmap) == 0 {
		return
	}
	clear(bitmap)
	if cache == nil {
		return
	}
	for _, block := range cache.blocks {
		x86MarkCodePagesForBlock(bitmap, block)
	}
	for _, block := range cache.mmuBlocks {
		x86MarkCodePagesForBlock(bitmap, block)
	}
}

func x86ClearRTSCache(ctx *X86JITContext) {
	if ctx == nil {
		return
	}
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.RTSCache0RegMap = 0
	ctx.RTSCache1PC = 0
	ctx.RTSCache1Addr = 0
	ctx.RTSCache1RegMap = 0
}

func x86InvalidateSMCRange(cache *CodeCache, bitmap []byte, ctx *X86JITContext) int {
	if cache == nil || ctx == nil || ctx.InvalSize == 0 {
		if cache != nil {
			cache.Invalidate()
		}
		clear(bitmap)
		x86ClearRTSCache(ctx)
		return 0
	}
	lo := uint64(ctx.InvalAddr)
	hi := lo + uint64(ctx.InvalSize)
	if hi < lo || hi > 1<<32 {
		hi = 1 << 32
	}
	removed := cache.InvalidateRange(lo, hi)
	x86RebuildCodePageBitmapFromCache(bitmap, cache)
	if removed != 0 {
		x86ClearRTSCache(ctx)
	}
	return removed
}
