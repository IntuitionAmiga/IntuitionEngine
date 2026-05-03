//go:build headless

package main

import (
	"math"
	"testing"
)

func newTestWAVEngine(t *testing.T) (*WAVEngine, *SoundChip) {
	t.Helper()
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	engine := NewWAVEngine(chip, SAMPLE_RATE)
	return engine, chip
}

func buildTestWAVFile(samples []int16, sampleRate uint32) *WAVFile {
	return &WAVFile{
		SampleRate:    sampleRate,
		NumChannels:   1,
		BitsPerSample: 16,
		LeftSamples:   samples,
		RightSamples:  append([]int16(nil), samples...),
	}
}

func TestWAVEngineChannelInit(t *testing.T) {
	engine, chip := newTestWAVEngine(t)

	samples := make([]int16, 64)
	wav := buildTestWAVFile(samples, 44100)
	engine.LoadWAV(wav)
	engine.SetPlaying(true)

	// First TickSample should initialize channel 0
	engine.TickSample()

	ch := chip.channels[0]
	if !ch.dacMode {
		t.Error("expected channel 0 in DAC mode")
	}
	if !ch.enabled {
		t.Error("expected channel 0 enabled")
	}
	if ch.volume != 1.0 {
		t.Errorf("expected volume=1.0, got %f", ch.volume)
	}
}

func TestWAVEngineTickSampleWritesDAC(t *testing.T) {
	engine, chip := newTestWAVEngine(t)

	// Create samples with known value 0.5
	samples := make([]int16, 64)
	for i := range samples {
		samples[i] = 16384
	}
	wav := buildTestWAVFile(samples, 44100)
	engine.LoadWAV(wav)
	engine.SetPlaying(true)

	// Tick to produce output
	for range 5 {
		engine.TickSample()
	}

	ch := chip.channels[0]
	if !ch.dacMode {
		t.Fatal("expected channel 0 in DAC mode")
	}
	// 0.5 * 127 = 63.5 → int8(63) → normalized: 63/128 ≈ 0.492
	sample := ch.generateSample()
	if math.Abs(float64(sample)-0.492) > 0.05 {
		t.Errorf("expected sample ≈ 0.49, got %f", sample)
	}
}

func TestWAVEngineSampleRateConversion(t *testing.T) {
	engine, _ := newTestWAVEngine(t)

	// 22050 Hz source at 44100 Hz output → phaseInc = 0.5
	samples := make([]int16, 100)
	for i := range samples {
		samples[i] = int16(i)
	}
	wav := buildTestWAVFile(samples, 22050)
	engine.LoadWAV(wav)

	phaseInc := engine.snapshot().phaseInc

	expected := 22050.0 / 44100.0
	if math.Abs(phaseInc-expected) > 0.001 {
		t.Errorf("expected phaseInc=%f, got %f", expected, phaseInc)
	}

	// At phaseInc=0.5, after 4 ticks we should be at phase=2.0 (sample index 2)
	engine.SetPlaying(true)
	for range 4 {
		engine.TickSample()
	}

	if got := engine.GetPosition(); got != 2 {
		t.Errorf("expected source position 2 after 4 ticks at phaseInc=0.5, got %d", got)
	}
}

func TestWAVEngineLoop(t *testing.T) {
	engine, _ := newTestWAVEngine(t)

	// Short sample with looping
	samples := make([]int16, 10)
	for i := range samples {
		samples[i] = 16384
	}
	wav := buildTestWAVFile(samples, 44100)
	engine.LoadWAV(wav)
	engine.SetLoop(true)
	engine.SetPlaying(true)

	// Tick well past the end
	for range 100 {
		engine.TickSample()
	}

	if !engine.IsPlaying() {
		t.Error("expected engine to still be playing (looping)")
	}
}

func TestWAVEngineStop(t *testing.T) {
	engine, _ := newTestWAVEngine(t)

	// Short sample without looping
	samples := make([]int16, 10)
	for i := range samples {
		samples[i] = 16384
	}
	wav := buildTestWAVFile(samples, 44100)
	engine.LoadWAV(wav)
	engine.SetLoop(false)
	engine.SetPlaying(true)

	// Tick past the end
	for range 20 {
		engine.TickSample()
	}

	if engine.IsPlaying() {
		t.Error("expected engine to stop at end of samples")
	}
}

func TestWAVEngineSilenceOnStop(t *testing.T) {
	engine, chip := newTestWAVEngine(t)

	samples := make([]int16, 64)
	for i := range samples {
		samples[i] = 26000
	}
	wav := buildTestWAVFile(samples, 44100)
	engine.LoadWAV(wav)
	engine.SetPlaying(true)

	// Tick to produce output
	for range 5 {
		engine.TickSample()
	}

	// Stop playback
	engine.SetPlaying(false)

	// DAC mode should be released.
	ch := chip.channels[0]
	if ch.dacMode {
		t.Error("expected DAC mode to be released after stop")
	}
}
