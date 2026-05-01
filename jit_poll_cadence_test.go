//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestPollCadence_TickAndReset(t *testing.T) {
	p := NewPollCadence(100)
	if p.Tick(50) {
		t.Errorf("50 < 100: should not signal")
	}
	if !p.Tick(50) {
		t.Errorf("50+50 >= 100: should signal")
	}
	if p.Tick(50) {
		t.Errorf("counter reset: 50 < 100")
	}
	p.Reset()
	if p.Tick(50) {
		t.Errorf("after reset: 50 < 100")
	}
}

func TestPollCadence_DefaultThreshold(t *testing.T) {
	p := NewPollCadence(0)
	for i := 0; i < int(DefaultPollCadence-1); i++ {
		if p.Tick(1) {
			t.Fatalf("signalled at %d, before threshold", i+1)
		}
	}
	if !p.Tick(1) {
		t.Errorf("did not signal at threshold")
	}
}
