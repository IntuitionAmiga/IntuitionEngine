// jit_6502_policy.go - 6502 JIT tier policy and diagnostics.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"sync/atomic"
)

const p65JITTier1 = 0

var p65TierController = func() *TierController {
	c := NewTierController(P65RegProfile)
	return applyPerfTuningProfileToTierController("p65", c)
}()

func p65JITStatsEnabled() bool {
	return os.Getenv("P65_JIT_STATS") == "1"
}

type p65JITStats struct {
	tier1Blocks   atomic.Uint64
	bails         atomic.Uint64
	invalidations atomic.Uint64
	chainExits    atomic.Uint64
}

var globalP65JITStats p65JITStats

type p65JITStatsSnapshot struct {
	tier1Blocks   uint64
	bails         uint64
	invalidations uint64
	chainExits    uint64
}

func p65JITStatsLoad() p65JITStatsSnapshot {
	return p65JITStatsSnapshot{
		tier1Blocks:   globalP65JITStats.tier1Blocks.Load(),
		bails:         globalP65JITStats.bails.Load(),
		invalidations: globalP65JITStats.invalidations.Load(),
		chainExits:    globalP65JITStats.chainExits.Load(),
	}
}

func (s p65JITStatsSnapshot) Sub(base p65JITStatsSnapshot) p65JITStatsSnapshot {
	return p65JITStatsSnapshot{
		tier1Blocks:   s.tier1Blocks - base.tier1Blocks,
		bails:         s.bails - base.bails,
		invalidations: s.invalidations - base.invalidations,
		chainExits:    s.chainExits - base.chainExits,
	}
}

func (s p65JITStatsSnapshot) Print() {
	fmt.Printf("6502 JIT stats: tier1=%d bails=%d invalidations=%d chain_exits=%d\n",
		s.tier1Blocks,
		s.bails,
		s.invalidations,
		s.chainExits,
	)
}
