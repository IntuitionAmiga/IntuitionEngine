package main

import "testing"

func lfsrPeriod(t *testing.T, mode int, seed uint32, limit int) int {
	t.Helper()
	sr := seed
	for i := 1; i <= limit; i++ {
		sr = stepNoiseLFSR(mode, sr)
		if sr == seed {
			return i
		}
	}
	t.Fatalf("mode %d did not repeat seed within %d steps", mode, limit)
	return 0
}

func TestSN15_LFSRPeriod(t *testing.T) {
	if got := lfsrPeriod(t, NOISE_MODE_SN15_WHITE, SN15_NOISE_LFSR_MASK, SN15_NOISE_LFSR_MASK); got != 32767 {
		t.Fatalf("SN15 white period: got %d, want 32767", got)
	}
}

func TestSN16_LFSRPeriod(t *testing.T) {
	if got := lfsrPeriod(t, NOISE_MODE_SN16_WHITE, SN16_NOISE_LFSR_MASK, SN16_NOISE_LFSR_MASK); got != 65535 {
		t.Fatalf("SN16 white period: got %d, want 65535", got)
	}
}

func TestSN15_PeriodicMode(t *testing.T) {
	if got := lfsrPeriod(t, NOISE_MODE_SN15_PERIODIC, 1, 15); got != 15 {
		t.Fatalf("SN15 periodic period: got %d, want 15", got)
	}
}

func TestSN16_PeriodicMode(t *testing.T) {
	if got := lfsrPeriod(t, NOISE_MODE_SN16_PERIODIC, 1, 16); got != 16 {
		t.Fatalf("SN16 periodic period: got %d, want 16", got)
	}
}

func TestSN_TapsMatchDatasheet(t *testing.T) {
	sn15 := stepNoiseLFSR(NOISE_MODE_SN15_WHITE, 0x0001)
	if sn15 != 0x4000 {
		t.Fatalf("SN15 taps: got 0x%04X, want 0x4000", sn15)
	}
	sn16 := stepNoiseLFSR(NOISE_MODE_SN16_WHITE, 0x0001)
	if sn16 != 0x8000 {
		t.Fatalf("SN16 taps: got 0x%04X, want 0x8000", sn16)
	}
}
