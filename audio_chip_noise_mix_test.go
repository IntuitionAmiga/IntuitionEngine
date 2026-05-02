package main

import (
	"math"
	"testing"
)

func TestSoundChipNoiseMixDefaultZeroPreservesSample(t *testing.T) {
	base := newTestSoundChip()
	mixed := newTestSoundChip()
	for _, chip := range []*SoundChip{base, mixed} {
		chip.channels[0].enabled = true
		chip.channels[0].waveType = WAVE_SQUARE
		chip.channels[0].frequency = 440
		chip.channels[0].volume = 0.5
		chip.channels[0].envelopeLevel = 1
		chip.channels[3].enabled = true
		chip.channels[3].waveType = WAVE_NOISE
		chip.channels[3].noiseMode = NOISE_MODE_PSG
		chip.channels[3].frequency = 500
		chip.channels[3].volume = 0.25
		chip.channels[3].envelopeLevel = 1
	}
	mixed.channels[0].noiseMix = 0

	for i := 0; i < 16; i++ {
		want := base.GenerateSample()
		got := mixed.GenerateSample()
		if got != want {
			t.Fatalf("sample %d with default noiseMix=%f, want %f", i, got, want)
		}
	}
}

func TestSoundChipNoiseMixAddsNoiseToNamedChannel(t *testing.T) {
	dry := newTestSoundChip()
	wet := newTestSoundChip()
	for _, chip := range []*SoundChip{dry, wet} {
		chip.channels[0].enabled = true
		chip.channels[0].waveType = WAVE_SQUARE
		chip.channels[0].frequency = 440
		chip.channels[0].volume = 0.5
		chip.channels[0].envelopeLevel = 1
		chip.channels[0].noiseMode = NOISE_MODE_PSG
		chip.channels[0].noiseSR = NOISE_LFSR_SEED
	}
	wet.channels[0].noiseMix = 1

	var diff float64
	for i := 0; i < 64; i++ {
		diff += math.Abs(float64(wet.GenerateSample() - dry.GenerateSample()))
	}
	if diff == 0 {
		t.Fatalf("noiseMix did not change channel output")
	}
}

func TestSoundChipNoiseMixAudibleWhenToneVolumeZero(t *testing.T) {
	chip := newTestSoundChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.waveType = WAVE_SQUARE
	ch.frequency = 440
	ch.noiseFrequency = 4000
	ch.volume = 0
	ch.envelopeLevel = 1
	ch.noiseMode = NOISE_MODE_PSG
	ch.noiseSR = NOISE_LFSR_SEED
	ch.noiseMix = 1

	var sum float64
	for range 64 {
		sum += math.Abs(float64(chip.GenerateSample()))
	}
	if sum == 0 {
		t.Fatalf("noiseMix was silent when tone volume was zero")
	}
}
