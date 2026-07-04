package main

import (
	"os"
	"strings"
)

const defaultAudioBlockSegmentMax = 64

type PerfTuningKnobs struct {
	PromoteAtExecCount   uint32
	IOBailMaxNumerator   uint32
	IOBailMaxDenominator uint32
	RegionMinBlocks      uint32
	PollCap              uint32
	AudioChunk           uint32
	FrameLeaseRingDepth  uint32
}

var perfTuningBackendDefaults = map[string]PerfTuningKnobs{
	"ie64": {
		PromoteAtExecCount:   64,
		IOBailMaxNumerator:   1,
		IOBailMaxDenominator: 4,
		RegionMinBlocks:      2,
		PollCap:              64,
		AudioChunk:           defaultAudioBlockSegmentMax,
		FrameLeaseRingDepth:  3,
	},
	"m68k": {
		PromoteAtExecCount:   64,
		IOBailMaxNumerator:   1,
		IOBailMaxDenominator: 4,
		RegionMinBlocks:      2,
		PollCap:              64,
		AudioChunk:           defaultAudioBlockSegmentMax,
		FrameLeaseRingDepth:  3,
	},
	"x86": {
		PromoteAtExecCount:   64,
		IOBailMaxNumerator:   1,
		IOBailMaxDenominator: 4,
		RegionMinBlocks:      3,
		PollCap:              64,
		AudioChunk:           defaultAudioBlockSegmentMax,
		FrameLeaseRingDepth:  3,
	},
	"p65": {
		PromoteAtExecCount:   32,
		IOBailMaxNumerator:   1,
		IOBailMaxDenominator: 4,
		RegionMinBlocks:      2,
		PollCap:              64,
		AudioChunk:           defaultAudioBlockSegmentMax,
		FrameLeaseRingDepth:  3,
	},
	"audio": {
		PromoteAtExecCount:   DefaultTierThresholds.PromoteAtExecCount,
		IOBailMaxNumerator:   1,
		IOBailMaxDenominator: 4,
		RegionMinBlocks:      DefaultTierThresholds.RegionMinBlocks,
		PollCap:              64,
		AudioChunk:           defaultAudioBlockSegmentMax,
		FrameLeaseRingDepth:  3,
	},
}

func perfTuningProfileName() string {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("IE_PERF_PROFILE")))
	if name == "" {
		return "default"
	}
	return name
}

func perfTuningProfileForBackend(backend string) PerfTuningKnobs {
	base, ok := perfTuningBackendDefaults[backend]
	if !ok {
		base = PerfTuningKnobs{
			PromoteAtExecCount:   DefaultTierThresholds.PromoteAtExecCount,
			IOBailMaxNumerator:   DefaultTierThresholds.IOBailMaxNumerator,
			IOBailMaxDenominator: DefaultTierThresholds.IOBailMaxDenominator,
			RegionMinBlocks:      DefaultTierThresholds.RegionMinBlocks,
			PollCap:              64,
			AudioChunk:           defaultAudioBlockSegmentMax,
			FrameLeaseRingDepth:  3,
		}
	}
	switch perfTuningProfileName() {
	case "latency":
		if base.PromoteAtExecCount > 16 {
			base.PromoteAtExecCount /= 2
		}
		if base.AudioChunk > 16 {
			base.AudioChunk /= 2
		}
	case "throughput":
		base.PromoteAtExecCount *= 2
		base.AudioChunk *= 2
	case "off", "default":
	default:
	}
	return base
}

func perfTuningAudioChunk() int {
	chunk := perfTuningProfileForBackend("audio").AudioChunk
	if chunk < 1 {
		return defaultAudioBlockSegmentMax
	}
	if chunk > 4096 {
		return 4096
	}
	return int(chunk)
}

func applyPerfTuningProfileToTierController(backend string, c *TierController) *TierController {
	if c == nil {
		return nil
	}
	knobs := perfTuningProfileForBackend(backend)
	c.Thresholds.PromoteAtExecCount = knobs.PromoteAtExecCount
	c.Thresholds.IOBailMaxNumerator = knobs.IOBailMaxNumerator
	c.Thresholds.IOBailMaxDenominator = knobs.IOBailMaxDenominator
	c.Thresholds.RegionMinBlocks = knobs.RegionMinBlocks
	return c
}
