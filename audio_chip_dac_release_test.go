//go:build headless

package main

import "testing"

func TestReleaseDACModeClearsDACAndGate(t *testing.T) {
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 3)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_DAC, uint32(byte(int8(42))))

	chip.ReleaseDACMode(0)

	ch := chip.channels[0]
	if ch.dacMode || ch.gate || ch.enabled || ch.dacValue != 0 {
		t.Fatalf("ReleaseDACMode left channel dirty: dac=%v gate=%v enabled=%v value=%f", ch.dacMode, ch.gate, ch.enabled, ch.dacValue)
	}
}

func TestIsChannelInDACReportsState(t *testing.T) {
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	if chip.IsChannelInDAC(0) {
		t.Fatal("channel unexpectedly starts in DAC")
	}
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_DAC, 1)
	if !chip.IsChannelInDAC(0) {
		t.Fatal("channel should report DAC after DAC write")
	}
	chip.ReleaseDACMode(0)
	if chip.IsChannelInDAC(0) {
		t.Fatal("channel should leave DAC after release")
	}
}

func TestHasSampleTickerReportsRegistration(t *testing.T) {
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	engine := NewWAVEngine(chip, SAMPLE_RATE)
	if chip.HasSampleTicker("someKey") {
		t.Fatal("ticker unexpectedly registered")
	}
	chip.RegisterSampleTicker("someKey", engine)
	if !chip.HasSampleTicker("someKey") {
		t.Fatal("ticker registration not reported")
	}
	chip.UnregisterSampleTicker("someKey")
	if chip.HasSampleTicker("someKey") {
		t.Fatal("ticker unregister not reported")
	}
}
