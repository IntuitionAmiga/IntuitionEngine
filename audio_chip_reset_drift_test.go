package main

import "testing"

func TestSoundChipResetCoversSoundGapFields(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.channels[0].sweepInitialFreq = 1234
	chip.channels[0].filterCutoff = 0.5
	chip.channels[0].filterCutoffTarget = 0.75
	chip.channels[4].sweepInitialFreq = 5678
	chip.flexShadow[4*FLEX_CH_STRIDE] = 0xAA
	chip.audioFrozen.Store(true)

	chip.Reset()

	if chip.channels[0].sweepInitialFreq != 0 || chip.channels[4].sweepInitialFreq != 0 {
		t.Fatalf("Reset did not clear sweepInitialFreq")
	}
	if chip.channels[0].filterCutoff != 0 || chip.channels[0].filterCutoffTarget != 0 {
		t.Fatalf("Reset did not clear filter cold-start state")
	}
	if chip.flexShadow[4*FLEX_CH_STRIDE] != 0 {
		t.Fatalf("Reset did not clear expanded flexShadow")
	}
	if chip.audioFrozen.Load() {
		t.Fatalf("Reset did not thaw audio")
	}
}
