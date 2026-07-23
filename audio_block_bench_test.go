// audio_block_bench_test.go - block audio benchmarks.
//
// (c) 2024 - 2026 Zayn Otley. GPLv3 or later.

package main

import (
	"math/rand"
	"testing"
)

// benchBlockChip builds a chip playing one engine's worth of event material,
// which is the shape the block graph is meant to cover: a burst of register
// writes every frame with a long quiet gap between bursts.
func benchBlockChip(kind string) *SoundChip {
	rng := rand.New(rand.NewSource(1))
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	const frames = 200
	switch kind {
	case "PSG":
		e := NewPSGEngine(chip, SAMPLE_RATE)
		e.SetEvents(randomPSGEvents(rng, frames, 441), 0, true, 0)
		e.SetPlaying(true)
	case "SID":
		e := NewSIDEngine(chip, SAMPLE_RATE)
		e.SetEvents(randomSIDEvents(rng, frames, 441), 0, true, 0)
		e.SetPlaying(true)
		chip.RegisterSampleTicker("sid", e)
	case "TED":
		e := NewTEDEngine(chip, SAMPLE_RATE)
		e.SetEvents(randomTEDEvents(rng, frames, 441), 0, true, 0)
		e.SetPlaying(true)
		chip.RegisterSampleTicker("ted", e)
	case "POKEY":
		e := NewPOKEYEngine(chip, SAMPLE_RATE)
		e.SetEvents(randomPOKEYEvents(rng, frames, 441), 0, true, 0)
		e.SetPlaying(true)
		chip.RegisterSampleTicker("pokey", e)
	}
	return chip
}

// BenchmarkReadSamplesBlock_AllChips measures a realistic ReadSamples pull for
// each event engine, with the block graph as configured. Compare against
// IE_AUDIO_BLOCK=0 for the per-sample path.
func BenchmarkReadSamplesBlock_AllChips(b *testing.B) {
	for _, kind := range []string{"PSG", "SID", "TED", "POKEY"} {
		b.Run(kind, func(b *testing.B) {
			chip := benchBlockChip(kind)
			out := make([]float32, 1024)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				chip.ReadSamples(out)
			}
		})
	}
}

// BenchmarkAudioRegisterWrite_UnderRender measures what the event ring exists
// for: the cost of a guest register write while the renderer is holding chip.mu
// to mix a segment. The "ring" case publishes and returns; the "mutex" case
// waits for the renderer.
func BenchmarkAudioRegisterWrite_UnderRender(b *testing.B) {
	for _, useRing := range []bool{false, true} {
		name := "mutex"
		if useRing {
			name = "ring"
		}
		b.Run(name, func(b *testing.B) {
			chip := newTestSoundChip()
			configureBlockReadChip(chip)
			if useRing {
				chip.eventRing = newAudioEventRing()
			}

			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				buf := make([]float32, 1024)
				for {
					select {
					case <-stop:
						return
					default:
					}
					chip.ReadSamples(buf)
				}
			}()

			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				chip.HandleRegisterWriteFromBus(FLEX_CH0_BASE+FLEX_OFF_VOL, uint32(i%256))
			}
			b.StopTimer()
			close(stop)
			<-done
		})
	}
}
