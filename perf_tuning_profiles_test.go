package main

import "testing"

func TestPerfProfiles_DefaultMatchesCurrentConstants(t *testing.T) {
	t.Setenv("IE_PERF_PROFILE", "")

	tests := []struct {
		backend string
		promote uint32
		region  uint32
	}{
		{"ie64", 64, 2},
		{"m68k", 64, 2},
		{"x86", 64, 3},
		{"p65", 32, 2},
	}
	for _, tc := range tests {
		knobs := perfTuningProfileForBackend(tc.backend)
		if knobs.PromoteAtExecCount != tc.promote {
			t.Fatalf("%s PromoteAtExecCount = %d, want %d", tc.backend, knobs.PromoteAtExecCount, tc.promote)
		}
		if knobs.RegionMinBlocks != tc.region {
			t.Fatalf("%s RegionMinBlocks = %d, want %d", tc.backend, knobs.RegionMinBlocks, tc.region)
		}
		if knobs.IOBailMaxNumerator != 1 || knobs.IOBailMaxDenominator != 4 {
			t.Fatalf("%s I/O bail ratio = %d/%d, want 1/4",
				tc.backend, knobs.IOBailMaxNumerator, knobs.IOBailMaxDenominator)
		}
	}
}

func TestPerfProfiles_AllValuesInValidatedRanges(t *testing.T) {
	for _, profile := range []string{"default", "latency", "throughput", "unknown"} {
		t.Run(profile, func(t *testing.T) {
			t.Setenv("IE_PERF_PROFILE", profile)
			for _, backend := range []string{"ie64", "m68k", "x86", "p65", "z80", "audio"} {
				knobs := perfTuningProfileForBackend(backend)
				if knobs.PromoteAtExecCount == 0 || knobs.PromoteAtExecCount > 4096 {
					t.Fatalf("%s/%s PromoteAtExecCount = %d", profile, backend, knobs.PromoteAtExecCount)
				}
				if knobs.IOBailMaxNumerator == 0 || knobs.IOBailMaxDenominator == 0 ||
					knobs.IOBailMaxNumerator >= knobs.IOBailMaxDenominator {
					t.Fatalf("%s/%s invalid I/O bail ratio %d/%d",
						profile, backend, knobs.IOBailMaxNumerator, knobs.IOBailMaxDenominator)
				}
				if knobs.RegionMinBlocks < 2 || knobs.RegionMinBlocks > 16 {
					t.Fatalf("%s/%s RegionMinBlocks = %d", profile, backend, knobs.RegionMinBlocks)
				}
				if knobs.AudioChunk == 0 || knobs.AudioChunk > 4096 {
					t.Fatalf("%s/%s AudioChunk = %d", profile, backend, knobs.AudioChunk)
				}
				if knobs.FrameLeaseRingDepth < 2 || knobs.FrameLeaseRingDepth > 8 {
					t.Fatalf("%s/%s FrameLeaseRingDepth = %d", profile, backend, knobs.FrameLeaseRingDepth)
				}
			}
		})
	}
}

func TestPerfProfiles_AudioChunkProfiles(t *testing.T) {
	tests := []struct {
		profile string
		want    uint32
	}{
		{"default", defaultAudioBlockSegmentMax},
		{"latency", defaultAudioBlockSegmentMax / 2},
		{"throughput", defaultAudioBlockSegmentMax * 2},
	}
	for _, tc := range tests {
		t.Run(tc.profile, func(t *testing.T) {
			t.Setenv("IE_PERF_PROFILE", tc.profile)
			if got := perfTuningProfileForBackend("audio").AudioChunk; got != tc.want {
				t.Fatalf("AudioChunk = %d, want %d", got, tc.want)
			}
		})
	}
}
