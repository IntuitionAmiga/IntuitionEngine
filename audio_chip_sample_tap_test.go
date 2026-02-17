package main

import "testing"

func TestSoundChip_SampleTap(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip failed: %v", err)
	}
	t.Cleanup(chip.Stop)

	var tapped []float32
	chip.SetSampleTap(func(v float32) {
		tapped = append(tapped, v)
	})

	const n = 16
	got := make([]float32, 0, n)
	for range n {
		got = append(got, chip.ReadSample())
	}

	if len(tapped) != n {
		t.Fatalf("tap count=%d, want %d", len(tapped), n)
	}
	for i := range n {
		if tapped[i] != got[i] {
			t.Fatalf("tap[%d]=%f, sample[%d]=%f", i, tapped[i], i, got[i])
		}
	}

	chip.ClearSampleTap()
	tapped = tapped[:0]
	_ = chip.ReadSample()
	if len(tapped) != 0 {
		t.Fatalf("tap called after ClearSampleTap")
	}
}
