// psg_noise_test.go - Tests for PSG noise LFSR behavior.

package main

import "testing"

func psgNoiseStep(sr uint32) uint32 {
	newBit := ((sr >> 0) ^ (sr >> 3)) & 1
	return ((sr << 1) | newBit) & PSG_NOISE_LFSR_MASK
}

func TestPSGNoiseLFSRSequence(t *testing.T) {
	ch := &Channel{
		waveType:  WAVE_NOISE,
		frequency: float32(SAMPLE_RATE),
		noiseMode: NOISE_MODE_PSG,
		noiseSR:   PSG_NOISE_LFSR_SEED,
	}

	sampleRate := float32(SAMPLE_RATE)
	const steps = 8
	got := make([]uint32, 0, steps)
	want := make([]uint32, 0, steps)

	sr := PSG_NOISE_LFSR_SEED
	for i := 0; i < steps; i++ {
		sr = psgNoiseStep(sr)
		want = append(want, sr&1)
		ch.generateWaveSample(sampleRate)
		got = append(got, ch.noiseSR&1)
	}

	for i := 0; i < steps; i++ {
		if got[i] != want[i] {
			t.Fatalf("step %d noise bit=%d, want %d", i, got[i], want[i])
		}
	}
}
