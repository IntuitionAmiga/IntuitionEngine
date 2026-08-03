// jit_x86_wasm_cache_prune.go - shared helpers for pruning stale wasm block caches.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import "encoding/binary"

// x86PruneAuxBlockCache removes any auxiliary per-PC block entries whose JIT
// metadata is no longer the live cache entry. It is generic so host tests can
// verify the pruning contract without depending on js/wasm runtime types.
func x86PruneAuxBlockCache[T any](cache *CodeCache, blocks map[uint32]T, meta func(T) *JITBlock, onRemove func(uint32, T)) int {
	if cache == nil || len(blocks) == 0 {
		return 0
	}
	removed := 0
	for pc, block := range blocks {
		if cache.Get(uint64(pc)) == meta(block) {
			continue
		}
		if onRemove != nil {
			onRemove(pc, block)
		}
		delete(blocks, pc)
		removed++
	}
	return removed
}

// x86WasmDropDriverCacheEntry clears the direct-mapped driver-cache slot for
// pc only when that slot still belongs to pc. This prevents range invalidation
// pruning from dropping a newer colliding entry that has already reused the
// same cache line.
func x86WasmDropDriverCacheEntry(pcCache []byte, mask uint32, pc uint32) {
	if len(pcCache) < 8 {
		return
	}
	idx := pc & mask
	off := int(idx << 3)
	if off < 0 || off+8 > len(pcCache) {
		return
	}
	if binary.LittleEndian.Uint32(pcCache[off:off+4]) != pc {
		return
	}
	clear(pcCache[off : off+8])
}

// x86WasmResetDriverCache clears the direct-mapped driver cache and rewinds the
// table-slot cursor so a fully invalidated live cache reuses slots from zero.
func x86WasmResetDriverCache(pcCache []byte, nextSlot *int) {
	clear(pcCache)
	if nextSlot != nil {
		*nextSlot = 0
	}
}
