// jit_6502_policy.go - 6502 JIT tier policy and diagnostics.

package main

import (
	"sync/atomic"
)

const p65JITTier1 = 0

var p65TierController = func() *TierController {
	c := NewTierController(P65RegProfile)
	return applyPerfTuningProfileToTierController("p65", c)
}()

type p65JITStats struct {
	tier1Blocks   atomic.Uint64
	nativeEntries atomic.Uint64
	bails         atomic.Uint64
	invalidations atomic.Uint64
	chainExits    atomic.Uint64
}

type p65JITStatsSnapshot struct {
	tier1Blocks   uint64
	nativeEntries uint64
	bails         uint64
	invalidations uint64
	chainExits    uint64
}

func (s *p65JITStats) snapshot() p65JITStatsSnapshot {
	return p65JITStatsSnapshot{
		tier1Blocks:   s.tier1Blocks.Load(),
		nativeEntries: s.nativeEntries.Load(),
		bails:         s.bails.Load(),
		invalidations: s.invalidations.Load(),
		chainExits:    s.chainExits.Load(),
	}
}

func (s p65JITStatsSnapshot) Sub(base p65JITStatsSnapshot) p65JITStatsSnapshot {
	return p65JITStatsSnapshot{
		tier1Blocks:   s.tier1Blocks - base.tier1Blocks,
		nativeEntries: s.nativeEntries - base.nativeEntries,
		bails:         s.bails - base.bails,
		invalidations: s.invalidations - base.invalidations,
		chainExits:    s.chainExits - base.chainExits,
	}
}

func (cpu *CPU_6502) resetJITStats() {
	cpu.jitStats.tier1Blocks.Store(0)
	cpu.jitStats.nativeEntries.Store(0)
	cpu.jitStats.bails.Store(0)
	cpu.jitStats.invalidations.Store(0)
	cpu.jitStats.chainExits.Store(0)
}

func (cpu *CPU_6502) jit6502StatsSnapshot() p65JITStatsSnapshot {
	return cpu.jitStats.snapshot()
}
