// psg_clock_test.go - Tests for PSG clock handling.

package main

import "testing"

func TestPSGClockAffectsToneFrequency(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.SetClockHz(PSG_CLOCK_ATARI_ST)

	engine.WriteRegister(0, 0x01)
	engine.WriteRegister(1, 0x00)

	want := float32(PSG_CLOCK_ATARI_ST) / 16.0
	got := chip.channels[0].frequency
	if got != want {
		t.Fatalf("tone freq = %.2f, want %.2f", got, want)
	}
}

func TestPSGClockUpdatesEnvelopePeriod(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.WriteRegister(11, 0x01)
	engine.WriteRegister(12, 0x00)

	engine.SetClockHz(PSG_CLOCK_ATARI_ST)
	atari := engine.envPeriodSamples
	engine.SetClockHz(PSG_CLOCK_ZX_SPECTRUM)
	zx := engine.envPeriodSamples

	if atari == zx {
		t.Fatalf("expected different envelope periods, got %.3f and %.3f", atari, zx)
	}
}
