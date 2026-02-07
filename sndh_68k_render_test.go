// sndh_68k_render_test.go - Integration tests for SNDH rendering.

//go:build headless

package main

import (
	"os"
	"testing"
)

func TestRenderSNDH_Goldrunner(t *testing.T) {
	sndhPath := "/home/zayn/Music/sndh_lf/Hubbard_Rob/Goldrunner.sndh"
	if _, err := os.Stat(sndhPath); os.IsNotExist(err) {
		t.Skip("Goldrunner.sndh not found")
	}

	data, err := os.ReadFile(sndhPath)
	if err != nil {
		t.Fatalf("Failed to read SNDH file: %v", err)
	}

	// Verify detection
	if !isSNDHData(data) {
		t.Fatal("isSNDHData should return true for Goldrunner.sndh")
	}

	// Test rendering
	sampleRate := 44100
	meta, events, totalSamples, clockHz, frameRate, loop, loopSample, _, _, err := renderSNDH(data, sampleRate)
	if err != nil {
		t.Fatalf("renderSNDH failed: %v", err)
	}

	// Verify metadata
	if meta.Title != "Gold Runner" {
		t.Errorf("Expected title 'Gold Runner', got %q", meta.Title)
	}
	if meta.Author != "Rob Hubbard" {
		t.Errorf("Expected author 'Rob Hubbard', got %q", meta.Author)
	}
	if meta.System != "Atari ST" {
		t.Errorf("Expected system 'Atari ST', got %q", meta.System)
	}

	// Verify frame rate
	if frameRate != 50 {
		t.Errorf("Expected frame rate 50, got %d", frameRate)
	}

	// Verify clock
	if clockHz != PSG_CLOCK_ATARI_ST {
		t.Errorf("Expected clock %d, got %d", PSG_CLOCK_ATARI_ST, clockHz)
	}

	// Verify we got some events
	if len(events) == 0 {
		t.Error("Expected some PSG events, got 0")
	}

	// Verify totalSamples is reasonable
	if totalSamples == 0 {
		t.Error("Expected non-zero totalSamples")
	}

	t.Logf("SNDH render test passed:")
	t.Logf("  Title: %s", meta.Title)
	t.Logf("  Author: %s", meta.Author)
	t.Logf("  Events: %d", len(events))
	t.Logf("  Total samples: %d (%.2f seconds)", totalSamples, float64(totalSamples)/float64(sampleRate))
	t.Logf("  Loop: %v, Loop sample: %d", loop, loopSample)

	// Log first few events
	if len(events) > 10 {
		t.Log("  First 10 events:")
		for i := 0; i < 10; i++ {
			t.Logf("    [%d] Sample=%d Reg=%d Value=0x%02X", i, events[i].Sample, events[i].Reg, events[i].Value)
		}
	}

	// Log events around frame 115 (should have volume changes)
	if len(events) > 50 {
		t.Log("  Events 20-40:")
		for i := 20; i < 40 && i < len(events); i++ {
			t.Logf("    [%d] Sample=%d Reg=%d Value=0x%02X", i, events[i].Sample, events[i].Reg, events[i].Value)
		}
	}
}

func TestPSGPlayer_LoadSNDH(t *testing.T) {
	sndhPath := "/home/zayn/Music/sndh_lf/Hubbard_Rob/Goldrunner.sndh"
	if _, err := os.Stat(sndhPath); os.IsNotExist(err) {
		t.Skip("Goldrunner.sndh not found")
	}

	// Create sound chip, engine and player
	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("Failed to create SoundChip: %v", err)
	}
	engine := NewPSGEngine(sound, 44100)
	player := NewPSGPlayer(engine)

	// Load SNDH file
	err = player.Load(sndhPath)
	if err != nil {
		t.Fatalf("PSGPlayer.Load failed: %v", err)
	}

	// Verify metadata loaded
	meta := player.Metadata()
	if meta.Title != "Gold Runner" {
		t.Errorf("Expected title 'Gold Runner', got %q", meta.Title)
	}

	// Verify duration
	duration := player.DurationSeconds()
	if duration <= 0 {
		t.Error("Expected positive duration")
	}

	t.Logf("PSGPlayer load test passed:")
	t.Logf("  Title: %s", meta.Title)
	t.Logf("  Author: %s", meta.Author)
	t.Logf("  Duration: %s (%.2f seconds)", player.DurationText(), duration)
}
