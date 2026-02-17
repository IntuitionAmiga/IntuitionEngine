package main

import (
	"os"
	"testing"
)

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

func TestSQTPlayerBinary_NotEmpty(t *testing.T) {
	if len(sqtPlayerBinary) == 0 {
		t.Fatal("sqtPlayerBinary is empty")
	}
	if len(sqtPlayerBinary) < 100 {
		t.Errorf("sqtPlayerBinary too small: %d bytes", len(sqtPlayerBinary))
	}
}

func TestSQTPlayerBinary_ValidZ80(t *testing.T) {
	if sqtPlayerBinary[0] != 0xC3 {
		t.Errorf("first byte = 0x%02X, want 0xC3 (JP)", sqtPlayerBinary[0])
	}
	if sqtPlayerBinary[3] != 0xC3 {
		t.Errorf("byte[3] = 0x%02X, want 0xC3 (JP)", sqtPlayerBinary[3])
	}
}

func TestTrackerConfigs_Z80Formats(t *testing.T) {
	// Only PT3, PT2, STC, SQT use Z80 emulation now
	formats := []string{".pt3", ".pt2", ".stc", ".sqt"}
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

func TestTrackerConfigByExt_NativeFormats(t *testing.T) {
	// PT1, ASC, FTC now use native Go players — not in trackerFormatConfigByExt
	for _, ext := range []string{".pt1", ".asc", ".ftc"} {
		_, ok := trackerFormatConfigByExt(ext)
		if ok {
			t.Errorf("trackerFormatConfigByExt(%q) should return false (native player)", ext)
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
	module := make([]byte, 0x40)
	module[0x00] = 3 // speed
	module[0x01] = 1 // 1 position
	module[0x02] = 0x24
	module[0x03] = 0x00
	module[0x04] = 0x00

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

func TestPT1NativeRender(t *testing.T) {
	// Build a minimal PT1 module with valid pattern data
	module := make([]byte, 0x200)
	module[0] = 3 // tempo
	module[1] = 1 // length
	module[2] = 0 // loop
	// Patterns offset at bytes 67-68
	patternsOff := 101 // right after positions
	module[67] = byte(patternsOff)
	module[68] = byte(patternsOff >> 8)
	// Position at offset 99
	module[99] = 0
	module[100] = 0xFF
	// Pattern 0: 3 channel offsets
	chanDataOff := patternsOff + 6
	module[patternsOff] = byte(chanDataOff)
	module[patternsOff+1] = byte(chanDataOff >> 8)
	module[patternsOff+2] = byte(chanDataOff + 2)
	module[patternsOff+3] = byte((chanDataOff + 2) >> 8)
	module[patternsOff+4] = byte(chanDataOff + 4)
	module[patternsOff+5] = byte((chanDataOff + 4) >> 8)
	// Channel data: 0x90 (empty/break) for each, 0xFF for end
	module[chanDataOff] = 0x90
	module[chanDataOff+1] = 0xFF // end marker
	module[chanDataOff+2] = 0x90
	module[chanDataOff+3] = 0x90
	module[chanDataOff+4] = 0x90
	module[chanDataOff+5] = 0x90

	frames, _, err := renderPT1Native(module)
	if err != nil {
		t.Fatalf("renderPT1Native error: %v", err)
	}
	if len(frames) == 0 {
		t.Error("expected frames from PT1 render, got 0")
	}
}

func TestASCNativeRender(t *testing.T) {
	// Build a minimal ASC v0 module
	module := make([]byte, 0x100)
	module[0] = 5    // tempo
	module[1] = 0x40 // patterns offset (lo)
	module[2] = 0x00 // patterns offset (hi)
	module[3] = 0x80 // samples offset (lo)
	module[4] = 0x00 // samples offset (hi)
	module[5] = 0xA0 // ornaments offset (lo)
	module[6] = 0x00 // ornaments offset (hi)
	module[7] = 1    // length
	module[8] = 0    // position 0

	frames, _, err := renderASCNative(module)
	if err != nil {
		t.Fatalf("renderASCNative error: %v", err)
	}
	if len(frames) == 0 {
		t.Error("expected frames from ASC render, got 0")
	}
}

func TestFTCNativeRender(t *testing.T) {
	// Build a minimal FTC module (212-byte header + positions)
	module := make([]byte, 0x200)
	copy(module[0:8], "Module: ")
	module[69] = 5    // tempo
	module[70] = 0    // loop
	module[75] = 0xE0 // patterns offset (lo)
	module[76] = 0x00 // patterns offset (hi)
	// Position at offset 212
	module[212] = 0    // pattern 0
	module[213] = 0    // transposition 0
	module[214] = 0xFF // end marker

	frames, _, err := renderFTCNative(module)
	if err != nil {
		t.Fatalf("renderFTCNative error: %v", err)
	}
	if len(frames) == 0 {
		t.Error("expected frames from FTC render, got 0")
	}
}

func TestRealPT3File(t *testing.T) {
	path := "testdata/music/test_pt3.pt3"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("test PT3 file not available: %v", err)
	}

	config := pt3FormatConfig()
	_, events, totalSamples, err := renderTrackerZ80(config, data, 44100, 500)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if len(events) < 100 {
		t.Errorf("expected >100 events from real PT3, got %d", len(events))
	}
	if totalSamples == 0 {
		t.Error("totalSamples = 0")
	}
	hasNonZeroVolume := false
	for _, ev := range events {
		if (ev.Reg == 8 || ev.Reg == 9 || ev.Reg == 10) && ev.Value > 0 {
			hasNonZeroVolume = true
			break
		}
	}
	if !hasNonZeroVolume {
		t.Error("no non-zero volume register writes found — player may not be producing sound")
	}
}

func TestRealSTCFile(t *testing.T) {
	path := "testdata/music/test_stc.stc"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("test STC file not available: %v", err)
	}

	config := stcFormatConfig()
	_, events, totalSamples, err := renderTrackerZ80(config, data, 44100, 500)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if len(events) < 100 {
		t.Errorf("expected >100 events from real STC, got %d", len(events))
	}
	if totalSamples == 0 {
		t.Error("totalSamples = 0")
	}
	hasNonZeroVolume := false
	for _, ev := range events {
		if (ev.Reg == 8 || ev.Reg == 9 || ev.Reg == 10) && ev.Value > 0 {
			hasNonZeroVolume = true
			break
		}
	}
	if !hasNonZeroVolume {
		t.Error("no non-zero volume register writes found — player may not be producing sound")
	}
}

func TestRealSQTFile(t *testing.T) {
	path := "testdata/music/test_sqt.sqt"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("test SQT file not available: %v", err)
	}

	config := sqtFormatConfig()
	_, events, totalSamples, err := renderTrackerZ80(config, data, 44100, 500)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if len(events) < 100 {
		t.Errorf("expected >100 events from real SQT, got %d", len(events))
	}
	if totalSamples == 0 {
		t.Error("totalSamples = 0")
	}
	hasNonZeroVolume := false
	for _, ev := range events {
		if (ev.Reg == 8 || ev.Reg == 9 || ev.Reg == 10) && ev.Value > 0 {
			hasNonZeroVolume = true
			break
		}
	}
	if !hasNonZeroVolume {
		t.Error("no non-zero volume register writes found — player may not be producing sound")
	}
}
