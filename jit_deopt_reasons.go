package main

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type DeoptReason uint8

const (
	DeoptNone DeoptReason = iota
	DeoptUnsupported
	DeoptHelper
	DeoptMMIO
	DeoptSMC
	DeoptInterrupt
	DeoptCachePressure
	DeoptDebug
	deoptReasonCount
)

var deoptReasonNames = [...]string{
	DeoptNone:          "none",
	DeoptUnsupported:   "unsupported",
	DeoptHelper:        "helper",
	DeoptMMIO:          "mmio",
	DeoptSMC:           "smc",
	DeoptInterrupt:     "interrupt",
	DeoptCachePressure: "cache_pressure",
	DeoptDebug:         "debug",
}

func (r DeoptReason) String() string {
	if r < deoptReasonCount {
		return deoptReasonNames[r]
	}
	return fmt.Sprintf("deopt_%d", uint8(r))
}

type DeoptExitFlags struct {
	Unsupported   bool
	NeedHelper    bool
	NeedIO        bool
	NeedInval     bool
	Interrupt     bool
	CachePressure bool
	Debug         bool
}

func ClassifyDeoptExit(flags DeoptExitFlags) DeoptReason {
	switch {
	case flags.NeedInval:
		return DeoptSMC
	case flags.NeedIO:
		return DeoptMMIO
	case flags.NeedHelper:
		return DeoptHelper
	case flags.Interrupt:
		return DeoptInterrupt
	case flags.CachePressure:
		return DeoptCachePressure
	case flags.Debug:
		return DeoptDebug
	case flags.Unsupported:
		return DeoptUnsupported
	default:
		return DeoptNone
	}
}

type DeoptStatsSnapshot struct {
	Counts [deoptReasonCount]uint64
	Total  uint64
}

type DeoptStats struct {
	counts [deoptReasonCount]atomic.Uint64
}

func (s *DeoptStats) Reset() {
	for i := range s.counts {
		s.counts[i].Store(0)
	}
}

func (s *DeoptStats) Add(reason DeoptReason) {
	if !perfAcctOn || reason == DeoptNone || reason >= deoptReasonCount {
		return
	}
	s.counts[reason].Add(1)
}

func (s *DeoptStats) Snapshot() DeoptStatsSnapshot {
	var snap DeoptStatsSnapshot
	for i := DeoptReason(1); i < deoptReasonCount; i++ {
		count := s.counts[i].Load()
		snap.Counts[i] = count
		snap.Total += count
	}
	return snap
}

func (s *DeoptStats) String() string {
	snap := s.Snapshot()
	if snap.Total == 0 {
		return "deopts total=0"
	}
	parts := []string{fmt.Sprintf("deopts total=%d", snap.Total)}
	for i := DeoptReason(1); i < deoptReasonCount; i++ {
		if count := snap.Counts[i]; count != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", i, count))
		}
	}
	return strings.Join(parts, " ")
}

func recordBlockDeopt(stats *DeoptStats, block *JITBlock, reason DeoptReason) {
	if reason == DeoptNone {
		return
	}
	if block != nil && block.dominantDeopt == DeoptNone {
		block.dominantDeopt = reason
	}
	if stats != nil {
		stats.Add(reason)
	}
}
