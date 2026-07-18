// jit_m68k_region_form.go - backend-neutral M68020 region formation.
//
// Extracted from jit_m68k_emit_amd64.go (M68K JIT parity plan, milestone 2
// decision): m68kFormRegion is pure frontend analysis — it walks
// ScanRegionM68K's static control-flow result and applies the shared
// native-admission predicates. It emits nothing, so it is shared analysis,
// not a per-backend routine. Backend-specific region COMPILATION
// (m68kCompileRegion on amd64) stays in each backend's emitter.
//
// This file must stay free of build tags and emitter symbols.

package main

// m68kRegion is the compiled-region descriptor produced by m68kFormRegion.
// blocks[i] is the pre-scanned instruction list for block i; blockPCs[i]
// is the guest start PC of that block. entryPC == blockPCs[0].
type m68kRegion struct {
	blocks   [][]M68KJITInstr
	blockPCs []uint32
	entryPC  uint32
}

// m68kFormRegion is the cache-aware region builder consumed by the M68K
// JIT exec loop. It walks the static control-flow graph from hotPC via
// ScanRegionM68K's per-backend rules, then refuses any region whose
// constituent blocks are not safe for region compile (fused-leaf
// markers, fallback-required first instruction, scan failure). Returns
// nil for single-block "regions" — caller falls back to per-block
// compile.
//
// Unlike x86FormRegion this implementation does not gate on cache
// presence: the region is built directly from memory. Cache-presence
// gating can be layered on later if region recompile thrash becomes a
// measured problem.
func m68kFormRegion(hotPC uint32, memory []byte) *m68kRegion {
	res := ScanRegionM68K(memory, hotPC)
	if len(res.BlockPCs) < 2 {
		return nil
	}
	region := &m68kRegion{entryPC: hotPC, blockPCs: res.BlockPCs}
	for _, pc := range res.BlockPCs {
		instrs := m68kScanBlock(memory, pc)
		if len(instrs) == 0 ||
			m68kNeedsConservativeFallback(memory, pc, instrs) ||
			!m68kCanUseProductionNativeBlock(memory, pc, instrs) {
			return nil
		}
		// Reject region if the block contains fused-leaf markers — the
		// region path does not handle the synthetic-RTS bookkeeping that
		// fused-leaf compile depends on.
		for _, ji := range instrs {
			if ji.fusedFlag != 0 {
				return nil
			}
		}
		region.blocks = append(region.blocks, instrs)
	}
	return region
}
