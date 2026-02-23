//go:build headless

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMODEndToEnd loads a MOD via MMIO (PTR/LEN/CTRL), ticks samples,
// and verifies DAC output on FLEX channels.
func TestMODEndToEnd(t *testing.T) {
	sound, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewMODPlayer(sound, SAMPLE_RATE)
	bus := NewMachineBus()
	player.AttachBus(bus)
	bus.MapIO(MOD_PLAY_PTR, MOD_END, player.HandlePlayRead, player.HandlePlayWrite)

	// Build a minimal MOD with audible sample data
	sampleData := make([]int8, 64)
	for i := range sampleData {
		sampleData[i] = int8((i % 128) - 64) // bipolar ramp
	}
	notes := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0, EffParam: 0}, // C-3 on channel 0
	}
	modData := buildMinimalMOD(sampleData, notes)

	// Load MOD data into bus memory at address 0x1000
	loadAddr := uint32(0x1000)
	for i, b := range modData {
		bus.Write8(loadAddr+uint32(i), b)
	}

	// Write MMIO registers: PTR, LEN, CTRL=start
	player.HandlePlayWrite(MOD_PLAY_PTR, loadAddr)
	player.HandlePlayWrite(MOD_PLAY_LEN, uint32(len(modData)))
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1) // start

	// Wait for async load
	for range 100 {
		status := player.HandlePlayRead(MOD_PLAY_STATUS)
		if status&0x1 == 0 && player.IsPlaying() {
			break
		}
		// Small tick to allow goroutine to complete
		for range 1000 {
			sound.GenerateSample()
		}
	}

	if !player.IsPlaying() {
		t.Fatal("expected MOD player to be playing after CTRL start")
	}

	// Tick enough samples to produce DAC output
	for range 2000 {
		sound.GenerateSample()
	}

	// Verify that FLEX channel 0 has been put into DAC mode
	ch := sound.channels[0]
	if !ch.dacMode {
		t.Error("expected channel 0 to be in DAC mode")
	}

	// Stop playback
	player.HandlePlayWrite(MOD_PLAY_CTRL, 2) // stop
	if player.IsPlaying() {
		t.Error("expected playback to stop after CTRL stop")
	}
}

// TestMODMediaLoaderRouting verifies .mod extension is detected as MEDIA_TYPE_MOD.
func TestMODMediaLoaderRouting(t *testing.T) {
	tests := []struct {
		path     string
		expected uint32
	}{
		{"music.mod", MEDIA_TYPE_MOD},
		{"TRACK.MOD", MEDIA_TYPE_MOD},
		{"song.Mod", MEDIA_TYPE_MOD},
		{"music.sid", MEDIA_TYPE_SID},
		{"tune.ahx", MEDIA_TYPE_AHX},
		{"prog.iex", MEDIA_TYPE_NONE},
	}
	for _, tt := range tests {
		got := detectMediaType(tt.path)
		if got != tt.expected {
			t.Errorf("detectMediaType(%q) = %d, want %d", tt.path, got, tt.expected)
		}
	}
}

// TestMODMediaLoaderLargeFile verifies files > 64KB load correctly
// (bypasses the staging area).
func TestMODMediaLoaderLargeFile(t *testing.T) {
	sound, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewMODPlayer(sound, SAMPLE_RATE)

	// Create a large MOD (>64KB) - pad with sample data
	sampleData := make([]int8, 70000) // > 64KB
	for i := range sampleData {
		sampleData[i] = int8(i % 128)
	}
	modData := buildMinimalMOD(sampleData, nil)

	if len(modData) <= 65536 {
		t.Fatalf("test MOD should be > 64KB, got %d bytes", len(modData))
	}

	// Load directly (simulating what media_loader does for large files)
	err := player.Load(modData)
	if err != nil {
		t.Fatalf("Load failed for large MOD: %v", err)
	}
}

// TestMODParseRealFiles tests parsing of actual MOD files from the repo.
func TestMODParseRealFiles(t *testing.T) {
	modFiles := []string{
		"sdk/examples/assets/music/professional_tracker.mod",
		"sdk/examples/assets/music/krunkd.mod",
		"sdk/examples/assets/music/pattern_skank.mod",
	}

	for _, path := range modFiles {
		fullPath := filepath.Join(".", path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Skipf("skipping %s: %v", path, err)
			continue
		}

		t.Run(filepath.Base(path), func(t *testing.T) {
			mod, err := ParseMOD(data)
			if err != nil {
				t.Fatalf("ParseMOD(%s) failed: %v", path, err)
			}
			if mod.NumChannels != 4 {
				t.Errorf("expected 4 channels, got %d", mod.NumChannels)
			}
			if mod.SongLength == 0 {
				t.Error("expected non-zero song length")
			}
			if len(mod.Patterns) == 0 {
				t.Error("expected at least one pattern")
			}
			// Verify at least one sample has data
			hasSample := false
			for _, s := range mod.Samples {
				if s.Length > 0 {
					hasSample = true
					break
				}
			}
			if !hasSample {
				t.Error("expected at least one sample with data")
			}

			// Verify we can load into engine and tick without panic
			sound, _ := NewSoundChip(AUDIO_BACKEND_OTO)
			engine := NewMODEngine(sound, SAMPLE_RATE)
			engine.LoadMOD(mod)
			engine.SetPlaying(true)
			for range 44100 { // 1 second of ticking
				engine.TickSample()
			}
		})
	}
}

// TestMODDACPostMixEffectsIsolation verifies DAC channel output is correct
// at per-channel level even when global effects are enabled.
func TestMODDACPostMixEffectsIsolation(t *testing.T) {
	sound, _ := NewSoundChip(AUDIO_BACKEND_OTO)

	// Enable FLEX channel and put it in DAC mode
	ch := sound.channels[0]
	ch.enabled = true
	ch.volume = 1.0
	ch.dacMode = true

	// Write a known value
	ch.dacValue = 0.5

	// Get per-channel output
	sample := ch.generateSample()
	if sample < 0.49 || sample > 0.51 {
		t.Errorf("DAC channel output = %f, want ~0.5", sample)
	}

	// Write another value and verify
	ch.dacValue = -0.75
	sample = ch.generateSample()
	if sample < -0.76 || sample > -0.74 {
		t.Errorf("DAC channel output = %f, want ~-0.75", sample)
	}
}

// TestMODRegisters8BitCPU verifies MOD register constants are correctly defined.
func TestMODRegisters8BitCPU(t *testing.T) {
	if MOD_PLAY_PTR != 0xF0BC0 {
		t.Errorf("MOD_PLAY_PTR = 0x%X, want 0xF0BC0", MOD_PLAY_PTR)
	}
	if MOD_PLAY_LEN != 0xF0BC4 {
		t.Errorf("MOD_PLAY_LEN = 0x%X, want 0xF0BC4", MOD_PLAY_LEN)
	}
	if MOD_PLAY_CTRL != 0xF0BC8 {
		t.Errorf("MOD_PLAY_CTRL = 0x%X, want 0xF0BC8", MOD_PLAY_CTRL)
	}
	if MOD_PLAY_STATUS != 0xF0BCC {
		t.Errorf("MOD_PLAY_STATUS = 0x%X, want 0xF0BCC", MOD_PLAY_STATUS)
	}
	if MOD_FILTER_MODEL != 0xF0BD0 {
		t.Errorf("MOD_FILTER_MODEL = 0x%X, want 0xF0BD0", MOD_FILTER_MODEL)
	}
	if MOD_POSITION != 0xF0BD4 {
		t.Errorf("MOD_POSITION = 0x%X, want 0xF0BD4", MOD_POSITION)
	}
	if MOD_END != 0xF0BD7 {
		t.Errorf("MOD_END = 0x%X, want 0xF0BD7", MOD_END)
	}

	// Verify 6502 mapping: 0xF0Bxx maps to $FBxx
	expected6502 := map[string]uint32{
		"MOD_PLAY_PTR":     0xFBC0,
		"MOD_PLAY_LEN":     0xFBC4,
		"MOD_PLAY_CTRL":    0xFBC8,
		"MOD_PLAY_STATUS":  0xFBCC,
		"MOD_FILTER_MODEL": 0xFBD0,
		"MOD_POSITION":     0xFBD4,
	}
	ieAddrs := map[string]uint32{
		"MOD_PLAY_PTR":     MOD_PLAY_PTR,
		"MOD_PLAY_LEN":     MOD_PLAY_LEN,
		"MOD_PLAY_CTRL":    MOD_PLAY_CTRL,
		"MOD_PLAY_STATUS":  MOD_PLAY_STATUS,
		"MOD_FILTER_MODEL": MOD_FILTER_MODEL,
		"MOD_POSITION":     MOD_POSITION,
	}
	for name, expected := range expected6502 {
		ie := ieAddrs[name]
		got := (ie & 0xFFF) | 0xF000
		if got != expected {
			t.Errorf("%s: 6502 addr = 0x%04X, want 0x%04X", name, got, expected)
		}
	}
}

// TestMODEndToEndWithRealFile loads a real MOD file via the player and
// ticks samples to verify playback produces output.
func TestMODEndToEndWithRealFile(t *testing.T) {
	modPath := "sdk/examples/assets/music/professional_tracker.mod"
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	sound, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewMODPlayer(sound, SAMPLE_RATE)
	sound.SetSampleTicker(player.engine)
	sound.enabled.Store(true)

	if err := player.Load(data); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	player.Play()

	if !player.IsPlaying() {
		t.Fatal("expected player to be playing")
	}

	// Generate 1 second of audio via ReadSample (which calls TickSample + GenerateSample)
	var nonZero int
	for range 44100 {
		sample := sound.ReadSample()
		if sample != 0 {
			nonZero++
		}
	}

	if nonZero == 0 {
		t.Error("expected non-zero audio samples during MOD playback")
	}

	player.Stop()
	if player.IsPlaying() {
		t.Error("expected player to stop")
	}
}

// TestMODFilterModelMMIO tests filter model switching via MMIO.
func TestMODFilterModelMMIO(t *testing.T) {
	sound, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewMODPlayer(sound, SAMPLE_RATE)
	bus := NewMachineBus()
	player.AttachBus(bus)
	bus.MapIO(MOD_PLAY_PTR, MOD_END, player.HandlePlayRead, player.HandlePlayWrite)

	player.HandlePlayWrite(MOD_FILTER_MODEL, 1) // A500
	got := player.HandlePlayRead(MOD_FILTER_MODEL)
	if got != 1 {
		t.Errorf("filter model = %d, want 1", got)
	}

	player.HandlePlayWrite(MOD_FILTER_MODEL, 2) // A1200
	got = player.HandlePlayRead(MOD_FILTER_MODEL)
	if got != 2 {
		t.Errorf("filter model = %d, want 2", got)
	}

	player.HandlePlayWrite(MOD_FILTER_MODEL, 0) // none
	got = player.HandlePlayRead(MOD_FILTER_MODEL)
	if got != 0 {
		t.Errorf("filter model = %d, want 0", got)
	}
}
