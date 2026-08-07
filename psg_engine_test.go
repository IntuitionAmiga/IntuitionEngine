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
	for range steps {
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

func TestPSGEngineProgressiveLateEventIsNotLost(t *testing.T) {
	engine := NewPSGEngine(nil, 44100)
	engine.SetEvents(nil, 0, false, 0)
	engine.TickBlock(8)
	engine.AppendEvents([]PSGEvent{{Sample: 4, Reg: 7, Value: 0x3e}}, 16, true, false, 0)
	engine.TickSample()
	if got := engine.HandleRead(PSG_BASE + 7); got != 0x3e {
		t.Fatalf("late progressive event = %#x, want %#x", got, 0x3e)
	}
}

func TestPSGEngineProgressivePlaybackStopsAtRenderedFrontier(t *testing.T) {
	engine := NewPSGEngine(nil, 100)
	engine.SetEvents([]PSGEvent{{Sample: 0, Reg: 8, Value: 1}}, 0, false, 0)
	engine.AppendEvents(nil, 2, false, false, 0)

	engine.TickBlock(8)
	if got := engine.playbackSample(); got != 2 {
		t.Fatalf("playback sample after underrun = %d, want rendered frontier 2", got)
	}

	engine.AppendEvents([]PSGEvent{{Sample: 3, Reg: 8, Value: 9}}, 5, false, false, 0)
	engine.TickSample()
	if got := engine.HandleRead(PSG_BASE + 8); got != 1 {
		t.Fatalf("future event applied before sample 3: got %d", got)
	}
	engine.TickSample()
	if got := engine.HandleRead(PSG_BASE + 8); got != 9 {
		t.Fatalf("resumed event at sample 3 = %d, want 9", got)
	}
}

func TestPSGFrameTimingFractional(t *testing.T) {
	engine, _ := newTestPSGEngine(100)
	player := NewPSGPlayer(engine)

	frames := [][]uint8{
		make([]uint8, PSG_REG_COUNT),
		make([]uint8, PSG_REG_COUNT),
		make([]uint8, PSG_REG_COUNT),
		make([]uint8, PSG_REG_COUNT),
	}
	if err := player.loadFrames(frames, 60, PSG_CLOCK_ATARI_ST, 0); err != nil {
		t.Fatalf("loadFrames failed: %v", err)
	}

	if len(engine.events) < PSG_REG_COUNT*len(frames) {
		t.Fatalf("expected %d events, got %d", PSG_REG_COUNT*len(frames), len(engine.events))
	}

	wantSamples := []uint64{0, 1, 3, 5}
	for i, want := range wantSamples {
		ev := engine.events[i*PSG_REG_COUNT]
		if ev.Sample != want {
			t.Fatalf("frame %d sample = %d, want %d", i, ev.Sample, want)
		}
	}
}

func TestPSGEventOrderingSameSample(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	engine.SetEvents([]PSGEvent{
		{Sample: 0, Reg: 8, Value: 0x01},
		{Sample: 0, Reg: 8, Value: 0x02},
	}, 1, false, 0)

	engine.TickSample()
	if engine.regs[8] != 0x02 {
		t.Fatalf("expected last write to win, got 0x%02X", engine.regs[8])
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
