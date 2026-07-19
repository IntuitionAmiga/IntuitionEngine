// jit_m68k_dispatch_cache_test.go - Proof that the M68020 JIT dispatch path
// goes through the generation-tagged direct dispatch cache (milestone 7
// slice: dispatch cache). The cache itself is shared infrastructure
// (jit_dispatch_cache.go embedded in CodeCache); these tests pin that the
// M68K cache instance actually uses it and that every M68K invalidation
// path expires its entries.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func m68kDispatchCacheTestCPU(t *testing.T) *M68KCPU {
	t.Helper()
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	if jitDispatchCacheDisabled {
		t.Skip("dispatch cache disabled via IE_JIT_DISPATCH_CACHE=0")
	}
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	t.Cleanup(cpu.freeM68KJIT)
	return cpu
}

func m68kDispatchCacheCompile(t *testing.T, cpu *M68KCPU, pc uint32) *JITBlock {
	t.Helper()
	writeM68KWords(cpu, pc, 0x7001, 0x7202, 0xD280) // moveq;moveq;add.l
	instrs := m68kScanBlock(cpu.memory, pc)
	block, err := m68kCompileBlockWithMem(instrs, pc, cpu.m68kGetJITExecMem(), cpu.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	cpu.m68kJitCache.Put(block)
	cpu.m68kMarkJITCodeRanges(block)
	return block
}

// The M68K code cache must consult the direct dispatch cache before the map.
// Proven by removing the block from the backing map only: a direct-cache hit
// still returns it, so a subsequent Get observes the cached entry.
func TestM68KJIT_DispatchCacheDirectHit(t *testing.T) {
	cpu := m68kDispatchCacheTestCPU(t)
	const pc = uint32(0x1000)
	block := m68kDispatchCacheCompile(t, cpu, pc)

	if got := cpu.m68kJitCache.Get(uint64(pc)); got != block {
		t.Fatal("compiled block not returned by Get")
	}
	// Bypass the public API: empty the backing map without touching the
	// generation. A dispatch-cache hit is now the only way Get can succeed.
	delete(cpu.m68kJitCache.blocks, uint64(pc))
	if got := cpu.m68kJitCache.Get(uint64(pc)); got != block {
		t.Fatal("M68K Get did not hit the direct dispatch cache")
	}
	// Restore so cleanup paths see a consistent cache.
	cpu.m68kJitCache.blocks[uint64(pc)] = block
}

// Every M68K invalidation path must expire dispatch-cache entries in O(1)
// via the generation tag: exact-range invalidation, single-block removal,
// and the full cache reset.
func TestM68KJIT_DispatchCacheExpiryOnInvalidation(t *testing.T) {
	const pc = uint32(0x1000)

	t.Run("InvalidateRange", func(t *testing.T) {
		cpu := m68kDispatchCacheTestCPU(t)
		block := m68kDispatchCacheCompile(t, cpu, pc)
		if got := cpu.m68kJitCache.Get(uint64(pc)); got != block {
			t.Fatal("block not cached")
		}
		cpu.m68kInvalidateJITCodeRange(pc, pc+2)
		if got := cpu.m68kJitCache.Get(uint64(pc)); got != nil {
			t.Fatal("dispatch cache served a range-invalidated M68K block")
		}
	})

	t.Run("RemoveBlock", func(t *testing.T) {
		cpu := m68kDispatchCacheTestCPU(t)
		block := m68kDispatchCacheCompile(t, cpu, pc)
		if !cpu.m68kJitCache.RemoveBlock(block) {
			t.Fatal("RemoveBlock failed")
		}
		if got := cpu.m68kJitCache.Get(uint64(pc)); got != nil {
			t.Fatal("dispatch cache served a removed M68K block")
		}
	})

	t.Run("FullReset", func(t *testing.T) {
		cpu := m68kDispatchCacheTestCPU(t)
		m68kDispatchCacheCompile(t, cpu, pc)
		cpu.m68kResetJITCodeCache()
		if got := cpu.m68kJitCache.Get(uint64(pc)); got != nil {
			t.Fatal("dispatch cache served a block across a full cache reset")
		}
	})
}
