package main

import (
	"testing"
	"time"
)

type readOnStartOutput struct {
	chip *SoundChip
}

func (o *readOnStartOutput) Start() {
	o.chip.ReadSample()
}

func (o *readOnStartOutput) Stop()           {}
func (o *readOnStartOutput) Close()          {}
func (o *readOnStartOutput) IsStarted() bool { return true }

func TestSoundChipStartDoesNotHoldMutexAcrossOutputStart(t *testing.T) {
	chip := &SoundChip{
		output:           nil,
		sampleRateRecip:  1.0 / float32(SAMPLE_RATE),
		masterGainLinear: 1.0,
		sampleTickers:    make(map[string]SampleTicker),
	}
	chip.sampleTicker.Store(&sampleTickerListHolder{})
	chip.sampleTap.Store(&sampleTapHolder{})
	for i := range NUM_CHANNELS {
		chip.channels[i] = &Channel{}
	}
	chip.output = &readOnStartOutput{chip: chip}

	done := make(chan struct{})
	go func() {
		chip.Start()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("SoundChip.Start deadlocked when output.Start read from SoundChip")
	}
}
