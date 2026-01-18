// psg_clock_test.go - Tests for PSG clock handling.

package main

import "testing"

func TestPSGClockAffectsToneFrequency(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.SetClockHz(PSG_CLOCK_ATARI_ST)

	// Use period 100 (0x64) to get an audible frequency
	// Period 1 would give 125kHz which is ultrasonic and correctly muted
	engine.WriteRegister(0, 0x64) // Low byte of period
	engine.WriteRegister(1, 0x00) // High byte of period

	// freq = clock / (16 * period) = 2,000,000 / (16 * 100) = 1250 Hz
	want := float32(PSG_CLOCK_ATARI_ST) / 16.0 / 100.0
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
