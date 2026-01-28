// ahx_engine_test.go - Tests for AHX engine and player

package main

import (
	"testing"
)

// TestAHXEngine_Creation tests engine creation
func TestAHXEngine_Creation(t *testing.T) {
	// Use global sound chip from common_setup_test.go
	engine := NewAHXEngine(chip, 44100)

	if engine == nil {
		t.Fatal("NewAHXEngine returned nil")
	}
	if engine.replayer == nil {
		t.Error("Engine replayer should be initialized")
	}
	if engine.sampleRate != 44100 {
		t.Errorf("Sample rate should be 44100, got %d", engine.sampleRate)
	}
}

// TestAHXEngine_LoadData tests loading AHX data
func TestAHXEngine_LoadData(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Create minimal valid AHX data
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 'e', 's', 't', 0x00,
	}

	err := engine.LoadData(data)
	if err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}

	if engine.replayer.Song == nil {
		t.Error("Song should be loaded after LoadData")
	}
	if engine.replayer.Song.Name != "Test" {
		t.Errorf("Song name should be 'Test', got '%s'", engine.replayer.Song.Name)
	}
}

// TestAHXEngine_PlayStop tests play/stop functionality
func TestAHXEngine_PlayStop(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Load data first
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 0x00,
	}
	engine.LoadData(data)

	// Start playing
	engine.SetPlaying(true)
	if !engine.IsPlaying() {
		t.Error("Should be playing after SetPlaying(true)")
	}

	// Stop playing
	engine.SetPlaying(false)
	if engine.IsPlaying() {
		t.Error("Should not be playing after SetPlaying(false)")
	}
}

// TestAHXEngine_TickSample tests sample tick processing
func TestAHXEngine_TickSample(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Load and start playing
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x04, 0x00, 0x00, 0x00, // TrackLength=4
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 0x00,
	}
	engine.LoadData(data)
	engine.SetPlaying(true)

	// Tick some samples
	for i := 0; i < 1000; i++ {
		engine.TickSample()
	}

	// Should have advanced
	if engine.currentSample != 1000 {
		t.Errorf("Current sample should be 1000, got %d", engine.currentSample)
	}
}

// TestAHXEngine_SpeedMultiplier tests different speed multipliers
func TestAHXEngine_SpeedMultiplier(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Test with 100Hz song (speed multiplier 2)
	data := []byte{
		'T', 'H', 'X', 0x01, // AHX1 for speed multiplier
		0x00, 0x19,
		0xA0, 0x01, // Speed multiplier = 2 (bits 6-5 = 01)
		0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 0x00,
	}
	engine.LoadData(data)

	if engine.replayer.Song.SpeedMultiplier != 2 {
		t.Errorf("Speed multiplier should be 2, got %d", engine.replayer.Song.SpeedMultiplier)
	}
}

// TestAHXPlayer_Load tests player loading
func TestAHXPlayer_Load(t *testing.T) {
	player := NewAHXPlayer(chip, 44100)

	// Create minimal valid AHX data
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'S', 'o', 'n', 'g', 0x00,
	}

	err := player.Load(data)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
}

// TestAHXPlayer_PlayStop tests player play/stop
func TestAHXPlayer_PlayStop(t *testing.T) {
	player := NewAHXPlayer(chip, 44100)

	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 0x00,
	}
	player.Load(data)

	player.Play()
	if !player.IsPlaying() {
		t.Error("Should be playing after Play()")
	}

	player.Stop()
	if player.IsPlaying() {
		t.Error("Should not be playing after Stop()")
	}
}

// TestAHXPlayer_Subsong tests subsong selection
func TestAHXPlayer_Subsong(t *testing.T) {
	player := NewAHXPlayer(chip, 44100)

	// Create AHX data with 2 subsongs
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x20, // Name offset
		0x80, 0x02, // PositionNr = 2
		0x00, 0x01, // Restart = 1
		0x01, // TrackLength = 1
		0x00, // TrackNr = 0
		0x00, // InstrumentNr = 0
		0x02, // SubsongNr = 2
		// Subsong list
		0x00, 0x00, // Subsong 0 starts at position 0
		0x00, 0x01, // Subsong 1 starts at position 1
		// Position 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Position 1
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data
		0x00, 0x00, 0x00,
		// Song name
		'S', 'u', 'b', 0x00,
	}
	player.Load(data)

	// Play subsong 0 (main)
	player.PlaySubsong(0)
	if player.engine.replayer.PosNr != 0 {
		t.Errorf("Subsong 0 should start at position 0, got %d", player.engine.replayer.PosNr)
	}

	// Play subsong 1
	player.PlaySubsong(1)
	if player.engine.replayer.PosNr != 0 {
		t.Errorf("Subsong 1 should start at position 0 (from subsong list), got %d", player.engine.replayer.PosNr)
	}
}

// TestAHXEngine_ChannelMapping tests that AHX uses 4 dedicated channels
func TestAHXEngine_ChannelMapping(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Verify 4 channels are mapped
	if len(engine.channels) != 4 {
		t.Errorf("Should have 4 channels, got %d", len(engine.channels))
	}

	// Channels should be consecutive (0-3)
	for i := 0; i < 4; i++ {
		expected := i
		if engine.channels[i] != expected {
			t.Errorf("Channel %d should map to %d, got %d", i, expected, engine.channels[i])
		}
	}
}
