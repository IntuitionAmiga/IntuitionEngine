// psg_test_helpers_test.go - Test helpers for PSG behavior.

package main

func newTestSoundChip() *SoundChip {
	chip := &SoundChip{
		filterLP:    DEFAULT_FILTER_LP,
		filterBP:    DEFAULT_FILTER_BP,
		filterHP:    DEFAULT_FILTER_HP,
		preDelayBuf: make([]float32, PRE_DELAY_MS*MS_TO_SAMPLES),
	}
	chip.enabled.Store(true)
	chip.sampleTicker.Store(&sampleTickerHolder{})

	// Initialize reverb buffers (required by GenerateSample → applyReverb)
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

	waveTypes := []int{
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE,
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE,
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE,
	}
	for i := range NUM_CHANNELS {
		chip.channels[i] = &Channel{
			waveType:            waveTypes[i],
			attackTime:          DEFAULT_ATTACK_TIME,
			decayTime:           DEFAULT_DECAY_TIME,
			sustainLevel:        DEFAULT_SUSTAIN,
			releaseTime:         DEFAULT_RELEASE_TIME,
			envelopePhase:       ENV_ATTACK,
			noiseSR:             NOISE_LFSR_SEED,
			dutyCycle:           DEFAULT_DUTY_CYCLE,
			phase:               MIN_PHASE,
			volume:              MIN_VOLUME,
			psgPlusGain:         1.0,
			psgPlusOversample:   1,
			pokeyPlusGain:       1.0,
			pokeyPlusOversample: 1,
			sidPlusGain:         1.0,
			sidPlusOversample:   1,
		}
	}

	return chip
}

func newTestPSGEngine(sampleRate int) (*PSGEngine, *SoundChip) {
	chip := newTestSoundChip()
	engine := NewPSGEngine(chip, sampleRate)
	return engine, chip
}
