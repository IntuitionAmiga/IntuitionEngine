// ted_player_test.go - Tests for TED high-level player

package main

import (
	"testing"
)

func TestTEDPlayerCreation(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)
	if player == nil {
		t.Fatal("NewTEDPlayer returned nil")
	}
}

func TestTEDPlayerLoad(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	err := player.Load("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	meta := player.Metadata()
	if meta.Title == "" {
		t.Error("title should be set")
	}
	t.Logf("Loaded: %q by %q", meta.Title, meta.Author)
}

func TestTEDPlayerLoadData(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	// Test with minimal data (should handle gracefully)
	err := player.LoadData([]byte{0x01, 0x10, 0x60}) // Load at $1001, RTS
	if err != nil {
		t.Logf("LoadData with minimal data: %v", err)
	}
}

func TestTEDPlayerPlayStop(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	// Should not crash when no file loaded
	player.Play()
	player.Stop()

	if player.IsPlaying() {
		t.Error("should not be playing after Stop")
	}
}

func TestTEDPlayerMetadata(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	err := player.Load("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	meta := player.Metadata()
	t.Logf("Title: %q", meta.Title)
	t.Logf("Author: %q", meta.Author)
	t.Logf("Date: %q", meta.Date)
	t.Logf("Tool: %q", meta.Tool)

	if meta.Title == "" && meta.Author == "" {
		t.Error("expected at least title or author to be set")
	}
}

func TestTEDPlayerHandlePlayWrite(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	// Write pointer (should not crash)
	player.HandlePlayWrite(TED_PLAY_PTR, 0x100000)
	player.HandlePlayWrite(TED_PLAY_LEN, 1000)

	// Read back
	ptr := player.HandlePlayRead(TED_PLAY_PTR)
	if ptr != 0x100000 {
		t.Errorf("ptr = 0x%X, want 0x100000", ptr)
	}

	length := player.HandlePlayRead(TED_PLAY_LEN)
	if length != 1000 {
		t.Errorf("len = %d, want 1000", length)
	}
}

func TestTEDPlayerHandlePlayRead(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	// Read status (should not crash)
	status := player.HandlePlayRead(TED_PLAY_STATUS)
	t.Logf("Initial status: 0x%X", status)

	// Status should not have error bit set initially
	if status&0x02 != 0 {
		t.Error("error bit should not be set initially")
	}
}

func TestTEDPlayerDuration(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	err := player.Load("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	dur := player.DurationSeconds()
	t.Logf("Duration: %.2f seconds", dur)

	text := player.DurationText()
	t.Logf("Duration text: %s", text)
}

func TestTEDPlayerAttachBus(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player := NewTEDPlayer(engine)

	// Should not crash when bus is nil
	player.AttachBus(nil)

	// Test with no bus (should fail gracefully)
	player.HandlePlayWrite(TED_PLAY_PTR, 0x100000)
	player.HandlePlayWrite(TED_PLAY_LEN, 100)
	player.HandlePlayWrite(TED_PLAY_CTRL, 1) // Start playback

	// Should have error because no bus
	status := player.HandlePlayRead(TED_PLAY_STATUS)
	if status&0x02 == 0 {
		t.Error("error bit should be set when bus is nil")
	}
}
