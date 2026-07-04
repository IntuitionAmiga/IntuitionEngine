package main

import "os"

const jitDispatchCacheEntries = 4096

var jitDispatchCacheDisabled = os.Getenv("IE_JIT_DISPATCH_CACHE") == "0"

type jitDispatchCacheEntry struct {
	tag        uint64
	pc         uint64
	ptbr       uint64
	generation uint64
	block      *JITBlock
}

type JITDispatchCache struct {
	entries [jitDispatchCacheEntries]jitDispatchCacheEntry
}

func newJITDispatchCache() *JITDispatchCache {
	return &JITDispatchCache{}
}

func jitDispatchCacheTag(pc, ptbr uint64) uint64 {
	tag := pc ^ (ptbr * 0x9E3779B97F4A7C15)
	tag ^= tag >> 33
	tag *= 0xC2B2AE3D27D4EB4F
	tag ^= tag >> 29
	if tag == 0 {
		return 1
	}
	return tag
}

func jitDispatchCacheIndex(tag uint64) uint64 {
	return tag & (jitDispatchCacheEntries - 1)
}

func (dc *JITDispatchCache) get(pc, ptbr, generation uint64) *JITBlock {
	if dc == nil {
		return nil
	}
	tag := jitDispatchCacheTag(pc, ptbr)
	e := &dc.entries[jitDispatchCacheIndex(tag)]
	if e.tag == tag && e.pc == pc && e.ptbr == ptbr && e.generation == generation {
		return e.block
	}
	return nil
}

func (dc *JITDispatchCache) put(pc, ptbr, generation uint64, block *JITBlock) {
	if dc == nil || block == nil {
		return
	}
	tag := jitDispatchCacheTag(pc, ptbr)
	dc.entries[jitDispatchCacheIndex(tag)] = jitDispatchCacheEntry{
		tag:        tag,
		pc:         pc,
		ptbr:       ptbr,
		generation: generation,
		block:      block,
	}
}

func (dc *JITDispatchCache) reset() {
	if dc == nil {
		return
	}
	clear(dc.entries[:])
}
