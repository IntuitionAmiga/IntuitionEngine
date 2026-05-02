package main

import "testing"

func newSubsongProbeSIDFile(songs uint16) *SIDFile {
	return &SIDFile{
		Header: SIDHeader{
			MagicID:     "PSID",
			Version:     2,
			DataOffset:  0x7C,
			LoadAddress: 0x1000,
			InitAddress: 0x1000,
			PlayAddress: 0x1006,
			Songs:       songs,
			StartSong:   1,
		},
		Data: []byte{
			0x8D, 0x00, 0x20, // STA $2000
			0x60, // RTS
			0xEA, 0xEA,
			0x60, // PLAY RTS
		},
	}
}

func TestSID6502PlayerInitPassesZeroBasedSubsongInA(t *testing.T) {
	tests := []struct {
		subsong int
		wantA   byte
	}{
		{subsong: 1, wantA: 0},
		{subsong: 2, wantA: 1},
	}

	for _, tt := range tests {
		player, err := newSID6502Player(newSubsongProbeSIDFile(2), tt.subsong, 44100)
		if err != nil {
			t.Fatalf("newSID6502Player(subsong=%d): %v", tt.subsong, err)
		}
		if got := player.bus.ram[0x2000]; got != tt.wantA {
			t.Fatalf("subsong %d INIT A stored 0x%02X, want 0x%02X", tt.subsong, got, tt.wantA)
		}
	}
}

func TestSID6502PlayerUsesCIA1TimerALatchForCyclesPerTick(t *testing.T) {
	file := &SIDFile{
		Header: SIDHeader{
			MagicID:     "PSID",
			Version:     2,
			DataOffset:  0x7C,
			LoadAddress: 0x1000,
			InitAddress: 0x1000,
			PlayAddress: 0x100D,
			Songs:       1,
			StartSong:   1,
		},
		Data: []byte{
			0xA9, 0x42, // LDA #$42
			0x8D, 0x04, 0xDC, // STA $DC04
			0xA9, 0x10, // LDA #$10
			0x8D, 0x05, 0xDC, // STA $DC05
			0x60,       // RTS
			0xEA, 0xEA, // padding
			0x60, // PLAY RTS
		},
	}

	player, err := newSID6502Player(file, 1, 44100)
	if err != nil {
		t.Fatalf("newSID6502Player: %v", err)
	}
	if got := player.bus.CIATimerALatch(); got != 0x1042 {
		t.Fatalf("CIA timer A latch=0x%04X, want 0x1042", got)
	}
	if player.cyclesPerTick != 0x1042 {
		t.Fatalf("cyclesPerTick=%d, want %d", player.cyclesPerTick, 0x1042)
	}
	wantHz := float64(SID_CLOCK_PAL) / float64(0x1042)
	if got := player.TickHz(); got < wantHz-0.01 || got > wantHz+0.01 {
		t.Fatalf("TickHz()=%f, want %f", got, wantHz)
	}
}

func TestSID6502PlayerRenderFramesPreservesInitEventChip(t *testing.T) {
	file := &SIDFile{
		Header: SIDHeader{
			MagicID:     "PSID",
			Version:     3,
			DataOffset:  0x7C,
			LoadAddress: 0x1000,
			InitAddress: 0x1000,
			PlayAddress: 0x1008,
			Songs:       1,
			StartSong:   1,
			Sid2Addr:    0xD420,
		},
		Data: []byte{
			0xA9, 0x7E, // LDA #$7E
			0x8D, 0x20, 0xD4, // STA $D420
			0x60, // RTS
			0xEA, 0xEA,
			0x60, // PLAY RTS
		},
	}

	player, err := newSID6502Player(file, 1, 44100)
	if err != nil {
		t.Fatalf("newSID6502Player: %v", err)
	}
	events, _ := player.RenderFrames(0)
	if len(events) != 1 {
		t.Fatalf("events=%d, want 1", len(events))
	}
	if events[0].Chip != 1 {
		t.Fatalf("init event Chip=%d, want 1", events[0].Chip)
	}
}

func TestSID6502PlayerRenderFramesPreservesFrameEventChip(t *testing.T) {
	file := &SIDFile{
		Header: SIDHeader{
			MagicID:     "PSID",
			Version:     3,
			DataOffset:  0x7C,
			LoadAddress: 0x1000,
			PlayAddress: 0x1000,
			Songs:       1,
			StartSong:   1,
			Sid2Addr:    0xD420,
		},
		Data: []byte{
			0xA9, 0x5A, // LDA #$5A
			0x8D, 0x20, 0xD4, // STA $D420
			0x60, // RTS
		},
	}

	player, err := newSID6502Player(file, 1, 44100)
	if err != nil {
		t.Fatalf("newSID6502Player: %v", err)
	}
	events, _ := player.RenderFrames(1)
	if len(events) != 1 {
		t.Fatalf("events=%d, want 1", len(events))
	}
	if events[0].Chip != 1 {
		t.Fatalf("frame event Chip=%d, want 1", events[0].Chip)
	}
}
