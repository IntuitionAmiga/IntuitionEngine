// jit_m68k_observed_region.go - Observed (trace-recorded) region formation
// for the M68020 JIT (milestone 7 slice, the M68020 analogue of
// ie64ObservedRecorder / ie64BuildObservedRegion).
//
// Static region formation (m68kFormRegion) follows only statically-known
// BRA/JMP edges. Hot entries whose successors are dynamic (taken Bcc
// paths, computed jumps, returns) never form a static region. The
// observed recorder instead captures the dispatcher-visible successor
// path actually executed from the hot entry: each dispatch appends the
// next block PC until the path closes back on the entry (a real dynamic
// cycle) or the block cap is reached. The recorded path then compiles
// through the ordinary region compiler, whose per-block guest-byte stamp
// guards, chain exits and SMC covered-ranges apply unchanged; a run that
// diverges from the recorded path simply chain-exits at the divergence
// point, so the observed layout is a performance hint, never a
// correctness assumption.
//
// The recorder is generation-tagged: any cache invalidation between
// recording start and completion abandons the path, because the recorded
// PCs may describe overwritten code.
//
// Untagged shared analysis; backend compilation is the existing
// m68kCompileRegion.

package main

import (
	"os"
	"sync/atomic"
)

// m68kObservedRegionPromotions counts observed regions installed. Shape
// tests and diagnostics read it.
var m68kObservedRegionPromotions atomic.Uint64

const m68kObservedMaxBlocks = 8

// Kill switch: IE_M68K_JIT_DISABLE_OBSERVED_REGIONS=1 disables recording.
var m68kJITObservedRegionsDisabled = os.Getenv("IE_M68K_JIT_DISABLE_OBSERVED_REGIONS") == "1"

// m68kObservedRecorder tracks one in-flight observed path.
type m68kObservedRecorder struct {
	entryPC    uint32
	pcs        [m68kObservedMaxBlocks]uint32
	count      uint8
	active     bool
	generation uint64
}

func (r *m68kObservedRecorder) start(entry uint32, generation uint64) {
	*r = m68kObservedRecorder{entryPC: entry, count: 1, active: true, generation: generation}
	r.pcs[0] = entry
}

func (r *m68kObservedRecorder) reset() { *r = m68kObservedRecorder{} }

func (r *m68kObservedRecorder) path() []uint32 { return r.pcs[:r.count] }

// appendSuccessor records the next dispatched block PC. done means the
// path closed on the entry (or hit the cap) and may compile; reject means
// the path revisited an interior block (an inner loop the region compiler
// would mislay) and recording stops.
func (r *m68kObservedRecorder) appendSuccessor(pc uint32) (done, reject bool) {
	if !r.active {
		return false, true
	}
	for i := uint8(0); i < r.count; i++ {
		if r.pcs[i] != pc {
			continue
		}
		if i == 0 && r.count >= 2 {
			r.active = false
			return true, false
		}
		r.active = false
		return false, true
	}
	r.pcs[r.count] = pc
	r.count++
	if r.count == m68kObservedMaxBlocks {
		r.active = false
		return true, false
	}
	return false, false
}

// m68kBuildObservedRegion validates a recorded path against the same
// admission predicates as static region formation and returns the region,
// or nil when any block is unsafe for region compilation.
func m68kBuildObservedRegion(path []uint32, memory []byte) *m68kRegion {
	if len(path) < 2 {
		return nil
	}
	region := &m68kRegion{entryPC: path[0], blockPCs: append([]uint32(nil), path...)}
	for _, pc := range path {
		instrs := m68kScanBlock(memory, pc)
		if len(instrs) == 0 ||
			m68kNeedsConservativeFallback(memory, pc, instrs) ||
			!m68kCanUseProductionNativeBlock(memory, pc, instrs) {
			return nil
		}
		for _, ji := range instrs {
			if ji.fusedFlag != 0 {
				return nil
			}
		}
		region.blocks = append(region.blocks, instrs)
	}
	return region
}
