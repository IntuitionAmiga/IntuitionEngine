package main

import (
	"testing"
)

// Test 1: Create player from parsed file
func TestSAP6502Player_New(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{
			Type:     'B',
			Init:     0x1000,
			Player:   0x1003,
			FastPlay: 312,
		},
		Blocks: []SAPBlock{
			{Start: 0x1000, End: 0x1005, Data: []byte{0xA9, 0x00, 0x60, 0xA9, 0x01, 0x60}},
		},
	}
	player, err := newSAP6502Player(file, 0, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if player == nil {
		t.Fatal("expected non-nil player")
	}
}

// Test 2: PAL timing calculation
func TestSAP6502Player_PALTiming(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 312},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	if player.clockHz != POKEY_CLOCK_PAL {
		t.Errorf("expected clock %d, got %d", POKEY_CLOCK_PAL, player.clockHz)
	}
	if player.scanlinesPerFrame != 312 {
		t.Errorf("expected 312 scanlines, got %d", player.scanlinesPerFrame)
	}
}

// Test 3: NTSC timing calculation
func TestSAP6502Player_NTSCTiming(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 262, NTSC: true},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	if player.clockHz != POKEY_CLOCK_NTSC {
		t.Errorf("expected clock %d, got %d", POKEY_CLOCK_NTSC, player.clockHz)
	}
	if player.scanlinesPerFrame != 262 {
		t.Errorf("expected 262 scanlines, got %d", player.scanlinesPerFrame)
	}
}

// Test 4: TYPE B init routine runs
func TestSAP6502Player_TypeB_Init(t *testing.T) {
	// INIT at $1000: LDA #$42, STA $00, RTS
	// Should store $42 in zero page location $00
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1010, FastPlay: 312},
		Blocks: []SAPBlock{
			{Start: 0x1000, End: 0x1004, Data: []byte{
				0xA9, 0x42, // LDA #$42
				0x85, 0x00, // STA $00
				0x60, // RTS
			}},
			{Start: 0x1010, End: 0x1010, Data: []byte{0x60}}, // PLAYER: RTS
		},
	}
	player, err := newSAP6502Player(file, 0, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if player.bus.Read(0x00) != 0x42 {
		t.Errorf("expected $00 = 0x42 after init, got 0x%02X", player.bus.Read(0x00))
	}
}

// Test 5: TYPE B subsong passed in A register
func TestSAP6502Player_TypeB_Subsong(t *testing.T) {
	// INIT: STA $00, RTS - stores A register to $00
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1010, FastPlay: 312, Songs: 3},
		Blocks: []SAPBlock{
			{Start: 0x1000, End: 0x1002, Data: []byte{0x85, 0x00, 0x60}}, // STA $00, RTS
			{Start: 0x1010, End: 0x1010, Data: []byte{0x60}},
		},
	}
	player, _ := newSAP6502Player(file, 2, 44100) // Subsong 2
	if player.bus.Read(0x00) != 0x02 {
		t.Errorf("expected $00 = 0x02 (subsong), got 0x%02X", player.bus.Read(0x00))
	}
}

// Test 6: PLAYER routine generates POKEY events
func TestSAP6502Player_TypeB_POKEYEvents(t *testing.T) {
	// PLAYER: write to POKEY registers, RTS
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1010, FastPlay: 312},
		Blocks: []SAPBlock{
			{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}, // INIT: RTS
			{Start: 0x1010, End: 0x1019, Data: []byte{
				0xA9, 0x50, // LDA #$50
				0x8D, 0x00, 0xD2, // STA $D200 (AUDF1)
				0xA9, 0xAF, // LDA #$AF
				0x8D, 0x01, 0xD2, // STA $D201 (AUDC1)
				0x60, // RTS
			}},
		},
	}
	player, err := newSAP6502Player(file, 0, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events, _ := player.RenderFrames(1)

	// Should have captured 2 POKEY writes
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
}

// Test 7: Multiple frames accumulate events
func TestSAP6502Player_MultipleFrames(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1010, FastPlay: 312},
		Blocks: []SAPBlock{
			{Start: 0x1000, End: 0x1000, Data: []byte{0x60}},
			{Start: 0x1010, End: 0x1015, Data: []byte{
				0xA9, 0x50, // LDA #$50
				0x8D, 0x00, 0xD2, // STA $D200 (AUDF1)
				0x60, // RTS
			}},
		},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	events, totalSamples := player.RenderFrames(50)

	// 50 frames * 1 write per frame = 50 events minimum
	if len(events) < 50 {
		t.Errorf("expected at least 50 events, got %d", len(events))
	}
	if totalSamples == 0 {
		t.Error("expected non-zero total samples")
	}
}

// Test 8: Cycle-to-sample conversion
func TestSAP6502Player_CycleToSample(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1010, FastPlay: 312},
		Blocks: []SAPBlock{
			{Start: 0x1000, End: 0x1000, Data: []byte{0x60}},
			{Start: 0x1010, End: 0x1015, Data: []byte{
				0xA9, 0x50, // LDA #$50
				0x8D, 0x00, 0xD2, // STA $D200
				0x60, // RTS
			}},
		},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	events, _ := player.RenderFrames(1)

	// Events should have sample timestamps
	for _, e := range events {
		// Sample should be within reasonable range for one frame
		if e.Sample > uint64(44100) {
			t.Errorf("sample timestamp %d seems too large for one frame", e.Sample)
		}
	}
}

// Test 9: Frame rate calculation
func TestSAP6502Player_FrameRate(t *testing.T) {
	// PAL: 1773447 Hz / (312 scanlines * 114 cycles) = ~49.86 Hz
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 312},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)

	// Should be approximately 50 Hz for PAL
	frameRate := float64(player.clockHz) / float64(player.scanlinesPerFrame*114)
	if frameRate < 49.0 || frameRate > 51.0 {
		t.Errorf("expected frame rate ~50 Hz, got %.2f", frameRate)
	}
}

// Test 10: Custom FASTPLAY rate
func TestSAP6502Player_CustomFastPlay(t *testing.T) {
	// FASTPLAY 156 = double speed (~100 Hz)
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 156},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	if player.scanlinesPerFrame != 156 {
		t.Errorf("expected 156 scanlines, got %d", player.scanlinesPerFrame)
	}
}

// Test 11: Samples per frame calculation
func TestSAP6502Player_SamplesPerFrame(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 312},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)

	// At ~50 Hz, each frame should have ~882 samples (44100 / 50)
	samplesPerFrame := player.getSamplesPerFrame()
	if samplesPerFrame < 800 || samplesPerFrame > 950 {
		t.Errorf("expected ~882 samples per frame, got %d", samplesPerFrame)
	}
}

// Test 12: Clock Hz getter
func TestSAP6502Player_GetClockHz(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 312},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	if player.GetClockHz() != POKEY_CLOCK_PAL {
		t.Errorf("expected %d, got %d", POKEY_CLOCK_PAL, player.GetClockHz())
	}
}

// Test 13: Stereo flag detection
func TestSAP6502Player_StereoDetection(t *testing.T) {
	file := &SAPFile{
		Header: SAPHeader{Type: 'B', Init: 0x1000, Player: 0x1000, FastPlay: 312, Stereo: true},
		Blocks: []SAPBlock{{Start: 0x1000, End: 0x1000, Data: []byte{0x60}}},
	}
	player, _ := newSAP6502Player(file, 0, 44100)
	if !player.IsStereo() {
		t.Error("expected stereo to be true")
	}
}
