package main

import "testing"

func TestSoundChip_RetriggerChannel_API(t *testing.T) {
	chip := newTestSoundChip()
	chip.channels[0].phase = 1.5
	chip.channels[0].noisePhase = 0.75

	chip.RetriggerChannel(0)

	if chip.channels[0].phase != 0 || chip.channels[0].noisePhase != 0 {
		t.Fatalf("RetriggerChannel left phase %.4f noisePhase %.4f, want zeroes",
			chip.channels[0].phase, chip.channels[0].noisePhase)
	}
}

func TestSoundChip_PhaseReset_MMIO(t *testing.T) {
	chip := newTestSoundChip()
	chip.channels[0].phase = 1.5
	addr := uint32(FLEX_CH0_BASE + FLEX_OFF_PHASE_RESET)

	chip.HandleRegisterWrite(addr, 1)

	if got := chip.channels[0].phase; got != 0 {
		t.Fatalf("phase reset MMIO left phase %.4f, want 0", got)
	}
	if got := chip.HandleRegisterRead(addr); got != 0 {
		t.Fatalf("phase reset MMIO read got %d, want 0", got)
	}
}
