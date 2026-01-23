// ted_6502_player_test.go - Tests for TED 6502 music player

package main

import (
	"os"
	"testing"
)

func TestTED6502PlayerCreation(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}
	if player == nil {
		t.Fatal("player is nil")
	}
	if player.clockHz != TED_CLOCK_PAL {
		t.Errorf("clockHz = %d, want %d", player.clockHz, TED_CLOCK_PAL)
	}
	if player.frameRate != 50 {
		t.Errorf("frameRate = %d, want 50", player.frameRate)
	}
}

func TestTED6502PlayerLoadFile(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}

	data, err := os.ReadFile("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	err = player.LoadFromData(data)
	if err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}

	meta := player.GetMetadata()
	if meta.Title == "" {
		t.Error("title should be set")
	}
	t.Logf("Loaded: %q by %q", meta.Title, meta.Author)
}

func TestTED6502PlayerRenderFrame(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}

	data, err := os.ReadFile("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	err = player.LoadFromData(data)
	if err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}

	// Render a few frames
	var totalEvents int
	for i := 0; i < 10; i++ {
		events, err := player.RenderFrame()
		if err != nil {
			t.Fatalf("RenderFrame failed: %v", err)
		}
		totalEvents += len(events)
	}

	t.Logf("Total events from 10 frames: %d", totalEvents)
}

func TestTED6502PlayerGetClockHz(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	// Default is PAL
	if player.GetClockHz() != TED_CLOCK_PAL {
		t.Errorf("GetClockHz = %d, want %d", player.GetClockHz(), TED_CLOCK_PAL)
	}
}

func TestTED6502PlayerGetFrameRate(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	// Default is 50 Hz (PAL)
	if player.GetFrameRate() != 50 {
		t.Errorf("GetFrameRate = %d, want 50", player.GetFrameRate())
	}
}

func TestTED6502PlayerReset(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	data, err := os.ReadFile("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	player.LoadFromData(data)
	player.RenderFrame() // Advance state

	player.Reset()

	// After reset, should be able to render again
	_, err = player.RenderFrame()
	if err != nil {
		t.Errorf("RenderFrame after reset failed: %v", err)
	}
}

func TestTED6502PlayerCyclesPerFrame(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	// PAL: 886724 Hz / 50 fps = 17734 cycles per frame
	expected := uint64(TED_CLOCK_PAL / 50)
	if player.cyclesPerFrame != expected {
		t.Errorf("cyclesPerFrame = %d, want %d", player.cyclesPerFrame, expected)
	}
}
