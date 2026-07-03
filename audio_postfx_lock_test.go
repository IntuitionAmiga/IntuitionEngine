package main

import (
	"math"
	"testing"
	"time"
)

func TestApplyPostFXCallableWithoutChipMutex(t *testing.T) {
	chip := newTestSoundChip()
	chip.reverbMix = 0.25
	chip.masterGainLinear = 1.0
	chip.masterCompEnvelope = 1.0

	snap := soundPostFXSnapshot{
		reverbMix: 0.25,
		reverbDecays: [NUM_COMB_FILTERS]float32{
			COMB_DECAY_1,
			COMB_DECAY_2,
			COMB_DECAY_3,
			COMB_DECAY_4,
		},
		master: chip.snapshotMasterNormalizerConfigUnlocked(),
	}

	chip.mu.Lock()
	defer chip.mu.Unlock()

	done := make(chan struct{})
	go func() {
		_ = chip.applyPostFX(0.25, snap)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("post-FX path blocked on chip.mu")
	}
}

func TestHandleRegisterWritesMatchesSequentialWrites(t *testing.T) {
	writes := []AudioRegisterWrite{
		{Addr: AUDIO_CTRL, Value: 1},
		{Addr: FLEX_CH0_BASE + FLEX_OFF_FREQ, Value: 440 * 256},
		{Addr: FLEX_CH0_BASE + FLEX_OFF_VOL, Value: 220},
		{Addr: FLEX_CH0_BASE + FLEX_OFF_CTRL, Value: 3},
		{Addr: FILTER_CUTOFF, Value: 128},
		{Addr: FILTER_RESONANCE, Value: 48},
		{Addr: FILTER_TYPE, Value: 1},
		{Addr: REVERB_MIX, Value: 32},
		{Addr: REVERB_DECAY, Value: 80},
	}

	sequential := newTestSoundChip()
	batched := newTestSoundChip()

	for _, write := range writes {
		sequential.HandleRegisterWrite(write.Addr, write.Value)
	}
	batched.HandleRegisterWrites(writes)

	for i := range 2048 {
		want := sequential.GenerateSample()
		got := batched.GenerateSample()
		if math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("sample %d: batch=%08x sequential=%08x", i, math.Float32bits(got), math.Float32bits(want))
		}
	}
}
