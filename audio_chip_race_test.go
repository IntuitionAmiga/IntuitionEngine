package main

import (
	"sync"
	"testing"
	"time"
)

// TestSoundChip_ConcurrentWriteRead stresses the writer/reader race between
// HandleRegisterWrite (CPU thread) and GenerateSample (audio thread).
// The test itself has no assertions - the race detector is the oracle.
// Run with: go test -race -run TestSoundChip_ConcurrentWriteRead -count=1
func TestSoundChip_ConcurrentWriteRead(t *testing.T) {
	chip := newTestSoundChip()
	chip.enabled.Store(true)

	// Initialize reverb buffers (not done by newTestSoundChip)
	combDelays := []int{COMB_DELAY_1, COMB_DELAY_2, COMB_DELAY_3, COMB_DELAY_4}
	combDecays := []float32{COMB_DECAY_1, COMB_DECAY_2, COMB_DECAY_3, COMB_DECAY_4}
	for i := range chip.combFilters {
		chip.combFilters[i] = CombFilter{
			buffer: make([]float32, combDelays[i]),
			decay:  combDecays[i],
		}
	}
	allpassDelays := []int{ALLPASS_DELAY_1, ALLPASS_DELAY_2}
	for i := range chip.allpassBuf {
		chip.allpassBuf[i] = make([]float32, allpassDelays[i])
	}

	// Enable channel 0 with gate on so generateSample does real work
	chip.channels[0].enabled = true
	chip.channels[0].gate = true
	chip.channels[0].frequency = 440.0
	chip.channels[0].volume = 0.8
	chip.channels[0].envelopePhase = ENV_SUSTAIN
	chip.channels[0].envelopeLevel = 1.0

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Goroutine 1: CPU-side writer - hammers HandleRegisterWrite on channel 0
	wg.Go(func() {
		iter := uint32(0)
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Write frequency (float32 field via fixed-point)
			chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, (440+iter%200)*256)
			// Write volume (float32 field)
			chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, iter%256)
			// Write control - enabled + gate (bool fields + envelope state)
			chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 3) // enabled=1, gate=1
			// Write wave type (int field)
			chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_WAVE_TYPE, iter%5)
			// Write duty cycle (float32 field)
			chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_DUTY, 128)
			iter++
		}
	})

	// Goroutine 2: audio-side reader - calls GenerateSample in a loop
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			chip.GenerateSample()
		}
	})

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
