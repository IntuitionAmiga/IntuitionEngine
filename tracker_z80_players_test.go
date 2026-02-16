package main

import "testing"

func TestPT3PlayerBinary_NotEmpty(t *testing.T) {
	if len(pt3PlayerBinary) == 0 {
		t.Fatal("pt3PlayerBinary is empty")
	}
	if len(pt3PlayerBinary) < 500 {
		t.Errorf("pt3PlayerBinary too small: %d bytes", len(pt3PlayerBinary))
	}
	if len(pt3PlayerBinary) > 8192 {
		t.Errorf("pt3PlayerBinary unexpectedly large: %d bytes", len(pt3PlayerBinary))
	}
}

func TestPT3PlayerBinary_ValidZ80(t *testing.T) {
	// First 3 bytes should be JP xxxx (0xC3 = JP)
	if pt3PlayerBinary[0] != 0xC3 {
		t.Errorf("first byte = 0x%02X, want 0xC3 (JP)", pt3PlayerBinary[0])
	}
	// PLAY entry at offset 3 should also be JP
	if pt3PlayerBinary[3] != 0xC3 {
		t.Errorf("byte[3] = 0x%02X, want 0xC3 (JP)", pt3PlayerBinary[3])
	}
}

func TestSTCPlayerBinary_NotEmpty(t *testing.T) {
	if len(stcPlayerBinary) == 0 {
		t.Fatal("stcPlayerBinary is empty")
	}
	if len(stcPlayerBinary) < 100 {
		t.Errorf("stcPlayerBinary too small: %d bytes", len(stcPlayerBinary))
	}
}

func TestSTCPlayerBinary_ValidZ80(t *testing.T) {
	if stcPlayerBinary[0] != 0xC3 {
		t.Errorf("first byte = 0x%02X, want 0xC3 (JP)", stcPlayerBinary[0])
	}
	if stcPlayerBinary[3] != 0xC3 {
		t.Errorf("byte[3] = 0x%02X, want 0xC3 (JP)", stcPlayerBinary[3])
	}
}

func TestGenericPlayerBinary_NotEmpty(t *testing.T) {
	if len(genericPlayerBinary) == 0 {
		t.Fatal("genericPlayerBinary is empty")
	}
}

func TestGenericPlayerBinary_ValidZ80(t *testing.T) {
	if genericPlayerBinary[0] != 0xC3 {
		t.Errorf("first byte = 0x%02X, want 0xC3 (JP)", genericPlayerBinary[0])
	}
	if genericPlayerBinary[3] != 0xC3 {
		t.Errorf("byte[3] = 0x%02X, want 0xC3 (JP)", genericPlayerBinary[3])
	}
}

func TestTrackerConfigs_AllFormats(t *testing.T) {
	formats := []string{".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc"}
	for _, ext := range formats {
		config, ok := trackerFormatConfigByExt(ext)
		if !ok {
			t.Errorf("trackerFormatConfigByExt(%q) returned false", ext)
			continue
		}
		if config.playerBinary == nil || len(config.playerBinary) == 0 {
			t.Errorf("%s: playerBinary is empty", ext)
		}
		if config.playerBase != 0xC000 {
			t.Errorf("%s: playerBase = 0x%04X, want 0xC000", ext, config.playerBase)
		}
		if config.initEntry != 0xC000 {
			t.Errorf("%s: initEntry = 0x%04X, want 0xC000", ext, config.initEntry)
		}
		if config.playEntry != 0xC003 {
			t.Errorf("%s: playEntry = 0x%04X, want 0xC003", ext, config.playEntry)
		}
		if config.frameRate != 50 {
			t.Errorf("%s: frameRate = %d, want 50", ext, config.frameRate)
		}
	}
}

func TestTrackerConfigByExt_Unknown(t *testing.T) {
	_, ok := trackerFormatConfigByExt(".xyz")
	if ok {
		t.Error("expected false for unknown extension")
	}
}

func TestPT3Config_EntryPoints(t *testing.T) {
	config := pt3FormatConfig()
	// Verify entry points fall within player binary range
	initOff := int(config.initEntry - config.playerBase)
	playOff := int(config.playEntry - config.playerBase)
	if initOff < 0 || initOff >= len(config.playerBinary) {
		t.Errorf("initEntry offset %d out of range [0, %d)", initOff, len(config.playerBinary))
	}
	if playOff < 0 || playOff >= len(config.playerBinary) {
		t.Errorf("playEntry offset %d out of range [0, %d)", playOff, len(config.playerBinary))
	}
}

func TestPT3RenderWithModule(t *testing.T) {
	// Build a minimal PT3 module header
	module := make([]byte, 0x100)
	copy(module[0:13], "ProTracker 3.")
	module[0x0D] = '5'
	module[0x63] = 3 // speed
	module[0x64] = 1 // 1 position
	module[0x65] = 0 // loop pos

	config := pt3FormatConfig()
	_, events, totalSamples, err := renderTrackerZ80(config, module, 44100, 100)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected events from PT3 render, got 0")
	}
	if totalSamples == 0 {
		t.Error("totalSamples = 0")
	}
}

func TestSTCRenderWithModule(t *testing.T) {
	// Build a minimal STC module: speed=3, positions=1, pattern data
	module := make([]byte, 0x40)
	module[0x00] = 3 // speed
	module[0x01] = 1 // 1 position
	// Position 0: pattern bytes for 3 channels at offset 2
	module[0x02] = 0x24 // note 36 (C-4) channel A
	module[0x03] = 0x00 // empty channel B
	module[0x04] = 0x00 // empty channel C

	config := stcFormatConfig()
	_, events, totalSamples, err := renderTrackerZ80(config, module, 44100, 100)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected events from STC render, got 0")
	}
	if totalSamples == 0 {
		t.Error("totalSamples = 0")
	}
}

func TestGenericRenderWithModule(t *testing.T) {
	module := make([]byte, 0x20)
	module[0x00] = 3 // speed
	module[0x01] = 1 // 1 position

	config := pt2FormatConfig() // uses generic player
	_, events, totalSamples, err := renderTrackerZ80(config, module, 44100, 100)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected events from generic render, got 0")
	}
	if totalSamples == 0 {
		t.Error("totalSamples = 0")
	}
}
