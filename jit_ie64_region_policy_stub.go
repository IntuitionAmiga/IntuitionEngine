// jit_ie64_region_policy_stub.go - non-AMD64 IE64 region policy stubs.

//go:build arm64 && (linux || windows || darwin)

package main

import "sync/atomic"

const (
	ie64JITTier1      = 0
	ie64JITTierRegion = 1
)

func ie64RegionPromotionEnabled() bool { return false }

func ie64RegionMMUEnabled() bool { return false }

func ie64JITStatsEnabled() bool { return false }

type ie64JITStats struct {
	tier1Blocks         atomic.Uint64
	regionCandidates    atomic.Uint64
	regions             atomic.Uint64
	regionRejected      atomic.Uint64
	spills              atomic.Uint64
	fpuSpills           atomic.Uint64
	directRAMProofs     atomic.Uint64
	inlinedCalls        atomic.Uint64
	ioBails             atomic.Uint64
	ioBailOpcodes       [256]atomic.Uint64
	invalidations       atomic.Uint64
	helperExits         [HELPER_DTRANS + 1]atomic.Uint64
	helperResumes       atomic.Uint64
	helperResumeCancels atomic.Uint64
}

var globalIE64JITStats ie64JITStats

type ie64JITStatsSnapshot struct {
	tier1Blocks         uint64
	regionCandidates    uint64
	regions             uint64
	regionRejected      uint64
	spills              uint64
	fpuSpills           uint64
	directRAMProofs     uint64
	inlinedCalls        uint64
	ioBails             uint64
	ioBailOpcodes       [256]uint64
	invalidations       uint64
	helperExits         [HELPER_DTRANS + 1]uint64
	helperResumes       uint64
	helperResumeCancels uint64
}

func ie64JITStatsLoad() ie64JITStatsSnapshot {
	snap := ie64JITStatsSnapshot{
		tier1Blocks:         globalIE64JITStats.tier1Blocks.Load(),
		regionCandidates:    globalIE64JITStats.regionCandidates.Load(),
		regions:             globalIE64JITStats.regions.Load(),
		regionRejected:      globalIE64JITStats.regionRejected.Load(),
		spills:              globalIE64JITStats.spills.Load(),
		fpuSpills:           globalIE64JITStats.fpuSpills.Load(),
		directRAMProofs:     globalIE64JITStats.directRAMProofs.Load(),
		inlinedCalls:        globalIE64JITStats.inlinedCalls.Load(),
		ioBails:             globalIE64JITStats.ioBails.Load(),
		invalidations:       globalIE64JITStats.invalidations.Load(),
		helperResumes:       globalIE64JITStats.helperResumes.Load(),
		helperResumeCancels: globalIE64JITStats.helperResumeCancels.Load(),
	}
	for i := range snap.helperExits {
		snap.helperExits[i] = globalIE64JITStats.helperExits[i].Load()
	}
	for i := range snap.ioBailOpcodes {
		snap.ioBailOpcodes[i] = globalIE64JITStats.ioBailOpcodes[i].Load()
	}
	return snap
}

func (s ie64JITStatsSnapshot) Sub(base ie64JITStatsSnapshot) ie64JITStatsSnapshot {
	out := ie64JITStatsSnapshot{
		tier1Blocks:         s.tier1Blocks - base.tier1Blocks,
		regionCandidates:    s.regionCandidates - base.regionCandidates,
		regions:             s.regions - base.regions,
		regionRejected:      s.regionRejected - base.regionRejected,
		spills:              s.spills - base.spills,
		fpuSpills:           s.fpuSpills - base.fpuSpills,
		directRAMProofs:     s.directRAMProofs - base.directRAMProofs,
		inlinedCalls:        s.inlinedCalls - base.inlinedCalls,
		ioBails:             s.ioBails - base.ioBails,
		invalidations:       s.invalidations - base.invalidations,
		helperResumes:       s.helperResumes - base.helperResumes,
		helperResumeCancels: s.helperResumeCancels - base.helperResumeCancels,
	}
	for i := range out.helperExits {
		out.helperExits[i] = s.helperExits[i] - base.helperExits[i]
	}
	for i := range out.ioBailOpcodes {
		out.ioBailOpcodes[i] = s.ioBailOpcodes[i] - base.ioBailOpcodes[i]
	}
	return out
}

func (s ie64JITStatsSnapshot) Print() {}
