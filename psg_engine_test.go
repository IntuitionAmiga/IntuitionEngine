// psg_engine_test.go - Tests for PSG engine scheduling and envelope logic.

package main

import "testing"

func TestPSGEngineEnvelopeAdvance(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)

	// Set envelope period to 1 for fast stepping.
	engine.WriteRegister(11, 0x01)
	engine.WriteRegister(12, 0x00)
	engine.WriteRegister(13, 0x09) // continue + attack

	initial := engine.envLevel
	steps := int(engine.envPeriodSamples) + 2
	for i := 0; i < steps; i++ {
		engine.TickSample()
	}

	if engine.envLevel == initial {
		t.Fatalf("expected envelope to advance, still at %d", engine.envLevel)
	}
}

func TestPSGEngineEventsApply(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetEvents([]PSGEvent{
		{Sample: 0, Reg: 8, Value: 0x0F},
		{Sample: 2, Reg: 8, Value: 0x00},
	}, 3, false, 0)

	if engine.regs[8] != 0x00 {
		t.Fatalf("expected initial register 8 to be 0, got %d", engine.regs[8])
	}

	engine.TickSample()
	if engine.regs[8] != 0x0F {
		t.Fatalf("expected register 8 to be 0x0F at sample 0, got %d", engine.regs[8])
	}

	engine.TickSample()
	engine.TickSample()
	if engine.regs[8] != 0x00 {
		t.Fatalf("expected register 8 to be 0x00 at sample 2, got %d", engine.regs[8])
	}
}

func TestIsPSGExtension(t *testing.T) {
	cases := map[string]bool{
		"track.ym":  true,
		"track.ay":  true,
		"track.vgm": true,
		"track.vgz": true,
		"track.iex": false,
	}
	for path, want := range cases {
		if got := isPSGExtension(path); got != want {
			t.Fatalf("isPSGExtension(%q) = %v, want %v", path, got, want)
		}
	}
}
