// jit_ie64_turbo_stub.go - non-AMD64 IE64 turbo policy stubs.

//go:build arm64 && (linux || windows || darwin)

package main

import "sync/atomic"

const (
	ie64JITTier1     = 0
	ie64JITTierTurbo = 1
)

func ie64TurboEnabled() bool { return false }

func ie64TurboStatsEnabled() bool { return false }

type ie64TurboStats struct {
	tier1Blocks     atomic.Uint64
	turboCandidates atomic.Uint64
	turboRegions    atomic.Uint64
	turboRejected   atomic.Uint64
	ioBails         atomic.Uint64
	invalidations   atomic.Uint64
}

var globalIE64TurboStats ie64TurboStats

type ie64TurboStatsSnapshot struct{}

func ie64TurboStatsLoad() ie64TurboStatsSnapshot { return ie64TurboStatsSnapshot{} }

func (s ie64TurboStatsSnapshot) Sub(base ie64TurboStatsSnapshot) ie64TurboStatsSnapshot {
	return ie64TurboStatsSnapshot{}
}

func (s ie64TurboStatsSnapshot) Print() {}
