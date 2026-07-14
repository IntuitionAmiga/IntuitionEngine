// jit_x86_region_policy.go - x86 JIT region-tier policy and diagnostics
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

// Temporary profiling: histogram of opcodes that force per-instruction
// interpreter fallback. Gated on X86_JIT_STATS=1.
var (
	x86FallbackOpMu   sync.Mutex
	x86FallbackOpHist = map[uint16]uint64{}
)

func x86RecordFallbackOpcode(memory []byte, pc uint32) {
	if !x86JITStatsOn || int(pc) >= len(memory) {
		return
	}
	// Skip legacy prefixes to reach the real opcode.
	i := pc
	for int(i) < len(memory) {
		b := memory[i]
		switch b {
		case 0x66, 0x67, 0xF0, 0xF2, 0xF3, 0x2E, 0x36, 0x3E, 0x26, 0x64, 0x65:
			i++
			continue
		}
		break
	}
	if int(i) >= len(memory) {
		return
	}
	key := uint16(memory[i])
	if memory[i] == 0x0F && int(i)+1 < len(memory) {
		key = 0x0F00 | uint16(memory[i+1])
	}
	x86FallbackOpMu.Lock()
	x86FallbackOpHist[key]++
	x86FallbackOpMu.Unlock()
}

func x86FallbackOpcodeReport() {
	if !x86JITStatsOn {
		return
	}
	x86FallbackOpMu.Lock()
	type kv struct {
		op    uint16
		count uint64
	}
	list := make([]kv, 0, len(x86FallbackOpHist))
	var total uint64
	for op, c := range x86FallbackOpHist {
		list = append(list, kv{op, c})
		total += c
	}
	x86FallbackOpMu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].count > list[j].count })
	fmt.Printf("x86 JIT fallback opcode histogram (total=%d):\n", total)
	for i, e := range list {
		if i >= 24 {
			break
		}
		pct := float64(0)
		if total > 0 {
			pct = float64(e.count) / float64(total) * 100
		}
		if e.op >= 0x0F00 {
			fmt.Printf("  0F %02X : %d (%.1f%%)\n", e.op&0xFF, e.count, pct)
		} else {
			fmt.Printf("  %02X    : %d (%.1f%%)\n", e.op, e.count, pct)
		}
	}
}

var (
	x86JITStatsOn             = os.Getenv("X86_JIT_STATS") == "1"
	x86RegionPromotionEnabled = x86RegionPromotionDefaultEnabled()
	x86RTSChainingEnabled     = os.Getenv("X86_JIT_RTS") == "1"
	x86BlockChainingEnabled   = os.Getenv("X86_JIT_CHAINS") != "0"
	x86JITStats               x86JITCounters
)

func x86RegionPromotionDefaultEnabled() bool {
	return os.Getenv("X86_JIT_REGIONS") == "1"
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
