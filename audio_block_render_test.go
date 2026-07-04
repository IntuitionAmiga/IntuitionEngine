// audio_block_render_test.go - block ticker parity tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import "testing"

func TestTickBlock_BitIdenticalToPerSample_PSG(t *testing.T) {
	events := []PSGEvent{
		{Sample: 0, Reg: 8, Value: 0x0F},
		{Sample: 2, Reg: 11, Value: 0x01},
		{Sample: 2, Reg: 12, Value: 0x00},
		{Sample: 2, Reg: 13, Value: 0x09},
		{Sample: 5, Reg: 8, Value: 0x00},
	}
	perSample := NewPSGEngine(nil, 100)
	block := NewPSGEngine(nil, 100)
	perSample.SetEvents(events, 12, false, 0)
	block.SetEvents(events, 12, false, 0)

	for range 9 {
		perSample.TickSample()
	}
	block.TickBlock(9)

	if perSample.regs != block.regs {
		t.Fatalf("registers differ after TickBlock: per-sample=%v block=%v", perSample.regs, block.regs)
	}
	if perSample.eventIndex != block.eventIndex ||
		perSample.currentSample != block.currentSample ||
		perSample.playing != block.playing ||
		perSample.envLevel != block.envLevel ||
		perSample.envDirection != block.envDirection ||
		perSample.envContinue != block.envContinue ||
		perSample.envAlternate != block.envAlternate ||
		perSample.envAttack != block.envAttack ||
		perSample.envHoldActive != block.envHoldActive {
		t.Fatalf("PSG block state differs: per-sample=%+v block=%+v", perSample, block)
	}
}

func TestTickBlock_BitIdenticalToPerSample_SID(t *testing.T) {
	events := []SIDEvent{
		{Sample: 0, Reg: 0x00, Value: 0x34, Chip: 0},
		{Sample: 0, Reg: 0x01, Value: 0x12, Chip: 0},
		{Sample: 1, Reg: 0x04, Value: SID_CTRL_TRIANGLE | SID_CTRL_GATE, Chip: 0},
		{Sample: 4, Reg: 0x04, Value: SID_CTRL_TRIANGLE, Chip: 0},
	}
	perSample := NewSIDEngine(nil, 100)
	block := NewSIDEngine(nil, 100)
	perSample.SetEvents(events, 8, false, 0)
	block.SetEvents(events, 8, false, 0)
	perSample.SetPlaying(true)
	block.SetPlaying(true)

	for range 6 {
		perSample.TickSample()
	}
	block.TickBlock(6)

	if perSample.regs != block.regs {
		t.Fatalf("registers differ after TickBlock: per-sample=%v block=%v", perSample.regs, block.regs)
	}
	if perSample.eventIndex != block.eventIndex ||
		perSample.currentSample != block.currentSample ||
		perSample.playing != block.playing ||
		perSample.playingActive.Load() != block.playingActive.Load() ||
		perSample.enabled.Load() != block.enabled.Load() ||
		perSample.sidPlusEnabled != block.sidPlusEnabled {
		t.Fatalf("SID block state differs: per-sample=%+v block=%+v", perSample, block)
	}
}
