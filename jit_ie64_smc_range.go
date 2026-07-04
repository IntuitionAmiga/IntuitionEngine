// jit_ie64_smc_range.go - IE64 range-scoped SMC invalidation helpers.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

func ie64MarkCodePagesForBlock(bitmap []byte, block *JITBlock) {
	ie64MarkCodePagesForBlockContext(bitmap, nil, block)
}

func ie64MarkCodePagesForBlockContext(bitmap []byte, ctx *JITContext, block *JITBlock) {
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
				ie64ExtendHighCodePageSpan(ctx, p, endPage)
				break
			}
			bitmap[p] = 1
		}
	}
}

func ie64MarkPhysicalCodePagesForBlock(bitmap []byte, bus *MachineBus, block *JITBlock) {
	if len(bitmap) == 0 || bus == nil || block == nil {
		return
	}
	for _, r := range JITBlockCoveredRanges(block) {
		if r[1] <= r[0] {
			continue
		}
		ie64MarkPhysicalCodePagesForVirtualRange(bitmap, bus, block.ptbr, r[0], r[1])
	}
}

func ie64MarkPhysicalCodePagesForVirtualRange(bitmap []byte, bus *MachineBus, ptbr, vaLo, vaHi uint64) {
	for va := vaLo; va < vaHi; {
		pageOff := va & MMU_PAGE_MASK
		chunkEnd := (va &^ MMU_PAGE_MASK) + MMU_PAGE_SIZE
		if chunkEnd < va || chunkEnd > vaHi {
			chunkEnd = vaHi
		}
		walk := sharedMMUWalkPageTable(bus, ptbr, (va>>MMU_PAGE_SHIFT)&PTE_PPN_MASK)
		if !walk.Fault && walk.Flags&PTE_P != 0 {
			physLo := walk.PhysicalBase + pageOff
			physHi := physLo + (chunkEnd - va)
			if physHi < physLo {
				physHi = ^uint64(0)
			}
			ie64MarkPhysicalCodePageRange(bitmap, physLo, physHi)
		}
		va = chunkEnd
	}
}

func ie64MarkPhysicalCodePageRange(bitmap []byte, physLo, physHi uint64) {
	if len(bitmap) == 0 || physHi <= physLo {
		return
	}
	startPage := physLo >> 8
	endPage := (physHi - 1) >> 8
	for p := startPage; p <= endPage && p < uint64(len(bitmap)); p++ {
		bitmap[p] = 1
	}
}

func ie64ExtendHighCodePageSpan(ctx *JITContext, startPage, endPage uint64) {
	if ctx == nil || endPage < startPage {
		return
	}
	if ctx.CodeHighEndPage == 0 || startPage < ctx.CodeHighStartPage {
		ctx.CodeHighStartPage = startPage
	}
	if endPage > ctx.CodeHighEndPage {
		ctx.CodeHighEndPage = endPage
	}
}

func ie64ClearHighCodePageSpan(ctx *JITContext) {
	if ctx == nil {
		return
	}
	ctx.CodeHighStartPage = 0
	ctx.CodeHighEndPage = 0
}

func ie64RebuildCodePageBitmapFromCache(bitmap []byte, cache *CodeCache) {
	ie64RebuildCodePageTrackingFromCache(bitmap, nil, cache)
}

func ie64RebuildCodePageTrackingFromCache(bitmap []byte, ctx *JITContext, cache *CodeCache) {
	ie64RebuildCodePageTrackingFromCacheWithPhysical(bitmap, nil, ctx, nil, cache)
}

func ie64RebuildCodePageTrackingFromCacheWithPhysical(bitmap, physBitmap []byte, ctx *JITContext, bus *MachineBus, cache *CodeCache) {
	if len(bitmap) == 0 {
		return
	}
	clear(bitmap)
	clear(physBitmap)
	ie64ClearHighCodePageSpan(ctx)
	if cache == nil {
		return
	}
	for _, block := range cache.blocks {
		ie64MarkCodePagesForBlockContext(bitmap, ctx, block)
	}
	for _, block := range cache.mmuBlocks {
		ie64MarkCodePagesForBlockContext(bitmap, ctx, block)
		ie64MarkPhysicalCodePagesForBlock(physBitmap, bus, block)
	}
}

func ie64ClearRTSCache(ctx *JITContext) {
	if ctx == nil {
		return
	}
	ctx.RTSCache0PC = 0
	ctx.RTSCache0Addr = 0
	ctx.RTSCache1PC = 0
	ctx.RTSCache1Addr = 0
	ctx.RTSCache2PC = 0
	ctx.RTSCache2Addr = 0
	ctx.RTSCache3PC = 0
	ctx.RTSCache3Addr = 0
}

func ie64PageMarked(bitmap []byte, ctx *JITContext, addr uint64, size uint32) bool {
	if len(bitmap) == 0 || size == 0 {
		return false
	}
	hi := addr + uint64(size) - 1
	if hi < addr {
		hi = ^uint64(0)
	}
	startPage := addr >> 8
	endPage := hi >> 8
	for p := startPage; p <= endPage; p++ {
		if p >= uint64(len(bitmap)) {
			break
		}
		if bitmap[p] != 0 {
			return true
		}
	}
	if ctx != nil && ctx.CodeHighEndPage != 0 && endPage >= ctx.CodeHighStartPage && startPage <= ctx.CodeHighEndPage {
		return true
	}
	return false
}

func (cpu *CPU64) markJITSMCWrite(addr uint64, size uint32) {
	if cpu == nil || cpu.jitCtx == nil || size == 0 {
		return
	}
	if cpu.mmuEnabled && cpu.markJITMMUSMCWrite(addr, size) {
		return
	}
	if !ie64PageMarked(cpu.jitCodePageBitmap, cpu.jitCtx, addr, size) {
		return
	}
	cpu.jitCtx.NeedInval = 1
	cpu.jitCtx.InvalAddr = addr
	cpu.jitCtx.InvalSize = size
}

func (cpu *CPU64) markJITMMUSMCWrite(addr uint64, size uint32) bool {
	if cpu == nil || cpu.jitCache == nil || size == 0 {
		return false
	}
	hi := addr + uint64(size)
	if hi < addr {
		hi = ^uint64(0)
	}
	for va := addr; va < hi; {
		chunkEnd := (va &^ MMU_PAGE_MASK) + MMU_PAGE_SIZE
		if chunkEnd < va || chunkEnd > hi {
			chunkEnd = hi
		}
		phys, fault, _ := cpu.translateAddr(va, ACCESS_WRITE)
		if !fault && ie64MMUStoreOverlapsCompiledPhys(cpu.jitCache, cpu.bus, phys, uint32(chunkEnd-va)) {
			cpu.jitCtx.NeedInval = 1
			cpu.jitCtx.InvalAddr = 0
			cpu.jitCtx.InvalSize = 0
			return true
		}
		va = chunkEnd
	}
	return false
}

func ie64MMUStoreOverlapsCompiledPhys(cache *CodeCache, bus *MachineBus, phys uint64, size uint32) bool {
	if cache == nil || bus == nil || size == 0 {
		return false
	}
	physHi := phys + uint64(size)
	if physHi < phys {
		physHi = ^uint64(0)
	}
	for key, block := range cache.mmuBlocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if ie64VirtualRangeOverlapsPhys(bus, key.ptbr, r[0], r[1], phys, physHi) {
				return true
			}
		}
	}
	return false
}

func ie64VirtualRangeOverlapsPhys(bus *MachineBus, ptbr, vaLo, vaHi, physLo, physHi uint64) bool {
	if bus == nil || vaHi <= vaLo || physHi <= physLo {
		return false
	}
	for va := vaLo; va < vaHi; {
		pageOff := va & MMU_PAGE_MASK
		chunkEnd := (va &^ MMU_PAGE_MASK) + MMU_PAGE_SIZE
		if chunkEnd < va || chunkEnd > vaHi {
			chunkEnd = vaHi
		}
		walk := sharedMMUWalkPageTable(bus, ptbr, (va>>MMU_PAGE_SHIFT)&PTE_PPN_MASK)
		if !walk.Fault && walk.Flags&PTE_P != 0 {
			blockPhysLo := walk.PhysicalBase + pageOff
			blockPhysHi := blockPhysLo + (chunkEnd - va)
			if blockPhysHi < blockPhysLo {
				blockPhysHi = ^uint64(0)
			}
			if blockPhysHi > physLo && blockPhysLo < physHi {
				return true
			}
		}
		va = chunkEnd
	}
	return false
}

func ie64InvalidateSMCRange(cache *CodeCache, bitmap []byte, ctx *JITContext) int {
	return ie64InvalidateSMCRangeWithPhysical(cache, bitmap, nil, ctx, nil)
}

func ie64InvalidateSMCRangeWithPhysical(cache *CodeCache, bitmap, physBitmap []byte, ctx *JITContext, bus *MachineBus) int {
	if cache == nil || ctx == nil || ctx.InvalSize == 0 {
		if cache != nil {
			cache.Invalidate()
		}
		clear(bitmap)
		clear(physBitmap)
		ie64ClearRTSCache(ctx)
		return 0
	}
	lo := ctx.InvalAddr
	hi := lo + uint64(ctx.InvalSize)
	if hi < lo {
		hi = ^uint64(0)
	}
	removed := cache.InvalidateRange(lo, hi)
	ie64RebuildCodePageTrackingFromCacheWithPhysical(bitmap, physBitmap, ctx, bus, cache)
	if removed != 0 {
		ie64ClearRTSCache(ctx)
	}
	return removed
}
