// jit_x86_region_policy.go - x86 JIT region-tier policy and diagnostics
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"sync/atomic"
)

var (
	x86JITStatsOn             = os.Getenv("X86_JIT_STATS") == "1"
	x86RegionPromotionEnabled = x86RegionPromotionDefaultEnabled()
	x86RTSChainingEnabled     = os.Getenv("X86_JIT_RTS") == "1"
	x86BlockChainingEnabled   = os.Getenv("X86_JIT_CHAINS") == "1"
	x86JITStats               x86JITCounters
)

func x86RegionPromotionDefaultEnabled() bool {
	return os.Getenv("X86_JIT_REGIONS") != "0"
}

type x86JITCounters struct {
	tier1Blocks      atomic.Uint64
	regionCandidates atomic.Uint64
	invalidations    atomic.Uint64
	chainExits       atomic.Uint64
}

func x86JITStatsReport() {
	if !x86JITStatsOn {
		return
	}
	fmt.Printf("x86 JIT region stats: tier1=%d region_candidates=%d invalidations=%d chain_exits=%d\n",
		x86JITStats.tier1Blocks.Load(),
		x86JITStats.regionCandidates.Load(),
		x86JITStats.invalidations.Load(),
		x86JITStats.chainExits.Load())
}
