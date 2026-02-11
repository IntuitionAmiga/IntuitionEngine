// psg_plus_test.go - Tests for PSG+ toggle and processing.

package main

import "testing"

func TestPSGPlusRegisterToggle(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)

	engine.HandlePSGPlusWrite(PSG_PLUS_CTRL, 1)
	if !engine.PSGPlusEnabled() {
		t.Fatalf("expected PSG+ enabled via register")
	}
	if got := engine.HandlePSGPlusRead(PSG_PLUS_CTRL); got != 1 {
		t.Fatalf("expected PSG+ readback 1, got %d", got)
	}

	engine.HandlePSGPlusWrite(PSG_PLUS_CTRL, 0)
	if engine.PSGPlusEnabled() {
		t.Fatalf("expected PSG+ disabled via register")
	}
}

func TestPSGPlusAffectsOutput(t *testing.T) {
	engineBase, chipBase := newTestPSGEngine(SAMPLE_RATE)
	enginePlus, chipPlus := newTestPSGEngine(SAMPLE_RATE)

	engineBase.WriteRegister(0, 0x20)
	engineBase.WriteRegister(1, 0x00)
	engineBase.WriteRegister(8, 0x08)
	engineBase.WriteRegister(7, 0x3E) // tone A enabled, noise disabled

	enginePlus.WriteRegister(0, 0x20)
	enginePlus.WriteRegister(1, 0x00)
	enginePlus.WriteRegister(8, 0x08)
	enginePlus.WriteRegister(7, 0x3E)
	enginePlus.SetPSGPlusEnabled(true)

	var baseSample float32
	var plusSample float32
	for range 4 {
		baseSample = chipBase.GenerateSample()
		plusSample = chipPlus.GenerateSample()
	}
	if baseSample == plusSample {
		t.Fatalf("expected PSG+ sample to differ from baseline")
	}
}

func TestPSGPlusDisabledMatchesBaseline(t *testing.T) {
	engineBase, chipBase := newTestPSGEngine(SAMPLE_RATE)
	engineOff, chipOff := newTestPSGEngine(SAMPLE_RATE)

	engineBase.WriteRegister(0, 0x10)
	engineBase.WriteRegister(1, 0x00)
	engineBase.WriteRegister(8, 0x07)
	engineBase.WriteRegister(7, 0x3E)

	engineOff.WriteRegister(0, 0x10)
	engineOff.WriteRegister(1, 0x00)
	engineOff.WriteRegister(8, 0x07)
	engineOff.WriteRegister(7, 0x3E)
	engineOff.SetPSGPlusEnabled(false)

	for i := range 4 {
		baseSample := chipBase.GenerateSample()
		offSample := chipOff.GenerateSample()
		if baseSample != offSample {
			t.Fatalf("sample %d mismatch: %.6f vs %.6f", i, baseSample, offSample)
		}
	}
}
