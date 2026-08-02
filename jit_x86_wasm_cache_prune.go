// jit_x86_wasm_cache_prune.go - shared helpers for pruning stale wasm block caches.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

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
