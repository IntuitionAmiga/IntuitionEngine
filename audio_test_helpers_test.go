package main

import "testing"

type audioFixture struct {
	chip *SoundChip
	mem  []byte
}

func newAudioFixture(t *testing.T) audioFixture {
	t.Helper()
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}
	t.Cleanup(chip.Stop)
	mem := make([]byte, 0x100000)
	chip.AttachBusMemory(mem)
	chip.enabled.Store(true)
	return audioFixture{chip: chip, mem: mem}
}

func captureChannelOutput(chip *SoundChip, chIndex, samples int) []float32 {
	out := make([]float32, 0, samples)
	chip.SetSampleTap(func(sample float32) {
		out = append(out, sample)
	})
	defer chip.ClearSampleTap()
	for range samples {
		_ = chip.ReadSample()
	}
	return out
}

func writeMMIO(chip *SoundChip, addr uint32, value uint32) {
	chip.HandleRegisterWrite(addr, value)
}
