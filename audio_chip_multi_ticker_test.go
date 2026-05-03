package main

import "testing"

type countTicker struct {
	count int
}

func (t *countTicker) TickSample() {
	t.count++
}

func TestSoundChip_RegisterSampleTicker_FiresOnce(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	ticker := &countTicker{}
	chip.RegisterSampleTicker("sn76489", ticker)

	chip.ReadSample()

	if ticker.count != 1 {
		t.Fatalf("ticker count: got %d, want 1", ticker.count)
	}
}

func TestSoundChip_RegisterSampleTicker_TwoEnginesBothFire(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	psg := &countTicker{}
	sn := &countTicker{}
	chip.RegisterSampleTicker("psg", psg)
	chip.RegisterSampleTicker("sn76489", sn)

	chip.ReadSample()

	if psg.count != 1 || sn.count != 1 {
		t.Fatalf("counts: psg=%d sn=%d, want both 1", psg.count, sn.count)
	}
}

func TestSoundChip_RegisterSampleTicker_KeyDedupes(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	first := &countTicker{}
	second := &countTicker{}
	chip.RegisterSampleTicker("sn76489", first)
	chip.RegisterSampleTicker("sn76489", second)

	chip.ReadSample()

	if first.count != 0 || second.count != 1 {
		t.Fatalf("counts: first=%d second=%d, want 0/1", first.count, second.count)
	}
}

func TestSoundChip_UnregisterSampleTicker(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	ticker := &countTicker{}
	chip.RegisterSampleTicker("sn76489", ticker)
	chip.UnregisterSampleTicker("sn76489")

	chip.ReadSample()

	if ticker.count != 0 {
		t.Fatalf("ticker count: got %d, want 0", ticker.count)
	}
}

func TestSoundChip_UnregisterSampleTickerIf_Match(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	ticker := &countTicker{}
	chip.RegisterSampleTicker("sn76489", ticker)

	if !chip.UnregisterSampleTickerIf("sn76489", ticker) {
		t.Fatalf("UnregisterSampleTickerIf returned false, want true")
	}
	if chip.HasSampleTicker("sn76489") {
		t.Fatalf("ticker still registered after matching unregister")
	}
}

func TestSoundChip_UnregisterSampleTickerIf_Mismatch(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	first := &countTicker{}
	second := &countTicker{}
	chip.RegisterSampleTicker("sn76489", first)

	if chip.UnregisterSampleTickerIf("sn76489", second) {
		t.Fatalf("UnregisterSampleTickerIf returned true for mismatched ticker")
	}
	chip.ReadSample()
	if first.count != 1 || second.count != 0 {
		t.Fatalf("counts: first=%d second=%d, want 1/0", first.count, second.count)
	}
}

func TestSoundChip_UnregisterSampleTickerIf_AbsentKey(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	if chip.UnregisterSampleTickerIf("missing", &countTicker{}) {
		t.Fatalf("UnregisterSampleTickerIf returned true for absent key")
	}
}

func TestSoundChip_SetSampleTicker_BackwardCompat(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	first := &countTicker{}
	second := &countTicker{}
	chip.SetSampleTicker(first)
	chip.SetSampleTicker(second)

	chip.ReadSample()

	if first.count != 0 || second.count != 1 {
		t.Fatalf("legacy counts: first=%d second=%d, want 0/1", first.count, second.count)
	}
}

func TestSoundChip_SNVoices_Zero_OnReset(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	chip.snVoices[0].enabled = true
	chip.snVoices[0].volume = 1
	chip.Reset()

	if chip.snVoices[0].enabled || chip.snVoices[0].volume != MIN_VOLUME {
		t.Fatalf("SN voice not reset: enabled=%v volume=%f", chip.snVoices[0].enabled, chip.snVoices[0].volume)
	}
}

func TestSoundChip_SNVoices_DontCollideWithFlex(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	chip.channels[0].volume = 0.25
	chip.snVoices[0].volume = 0.75

	if chip.channels[0].volume != 0.25 || chip.snVoices[0].volume != 0.75 {
		t.Fatalf("voices collided: flex=%f sn=%f", chip.channels[0].volume, chip.snVoices[0].volume)
	}
}

func TestSoundChip_Reset_PreservesSampleTickers(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatal(err)
	}
	ticker := &countTicker{}
	chip.RegisterSampleTicker("sn76489", ticker)
	chip.Reset()

	chip.ReadSample()

	if ticker.count != 1 {
		t.Fatalf("ticker did not survive Reset: got %d, want 1", ticker.count)
	}
}
