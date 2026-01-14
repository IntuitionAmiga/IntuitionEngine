// psg_test_helpers_test.go - Test helpers for PSG behavior.

package main

func newTestSoundChip() *SoundChip {
	chip := &SoundChip{
		filterLP:    DEFAULT_FILTER_LP,
		filterBP:    DEFAULT_FILTER_BP,
		filterHP:    DEFAULT_FILTER_HP,
		preDelayBuf: make([]float32, PRE_DELAY_MS*MS_TO_SAMPLES),
	}
	chip.sampleTicker.Store(&sampleTickerHolder{})

	waveTypes := []int{WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE}
	for i := 0; i < NUM_CHANNELS; i++ {
		chip.channels[i] = &Channel{
			waveType:          waveTypes[i],
			attackTime:        DEFAULT_ATTACK_TIME,
			decayTime:         DEFAULT_DECAY_TIME,
			sustainLevel:      DEFAULT_SUSTAIN,
			releaseTime:       DEFAULT_RELEASE_TIME,
			envelopePhase:     ENV_ATTACK,
			noiseSR:           NOISE_LFSR_SEED,
			dutyCycle:         DEFAULT_DUTY_CYCLE,
			phase:             MIN_PHASE,
			volume:            MIN_VOLUME,
			psgPlusGain:       1.0,
			psgPlusOversample: 1,
		}
	}

	return chip
}

func newTestPSGEngine(sampleRate int) (*PSGEngine, *SoundChip) {
	chip := newTestSoundChip()
	engine := NewPSGEngine(chip, sampleRate)
	return engine, chip
}
