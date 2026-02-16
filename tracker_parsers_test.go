package main

import (
	"testing"
)

func TestParsePT3Header(t *testing.T) {
	// Build a minimal PT3 header
	data := make([]byte, 0x100)
	copy(data[0:13], "ProTracker 3.")
	data[0x0D] = '5' // version
	copy(data[0x1E:0x3E], "Test Song Title\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	copy(data[0x42:0x62], "Test Author\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	data[0x63] = 3  // speed
	data[0x64] = 10 // positions
	data[0x65] = 2  // loop position

	info, err := parsePT3Header(data)
	if err != nil {
		t.Fatalf("parsePT3Header error: %v", err)
	}
	if info.format != "PT3" {
		t.Errorf("format = %q, want PT3", info.format)
	}
	if info.title != "Test Song Title" {
		t.Errorf("title = %q, want %q", info.title, "Test Song Title")
	}
	if info.author != "Test Author" {
		t.Errorf("author = %q, want %q", info.author, "Test Author")
	}
	if info.speed != 3 {
		t.Errorf("speed = %d, want 3", info.speed)
	}
	if info.positions != 10 {
		t.Errorf("positions = %d, want 10", info.positions)
	}
	if info.loopPos != 2 {
		t.Errorf("loopPos = %d, want 2", info.loopPos)
	}
	// frameCount = 10 * 64 * 3 = 1920
	if info.frameCount != 1920 {
		t.Errorf("frameCount = %d, want 1920", info.frameCount)
	}
}

func TestParsePT3Header_TooShort(t *testing.T) {
	data := make([]byte, 0x50)
	_, err := parsePT3Header(data)
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestParsePT3Header_EstimateFrames(t *testing.T) {
	data := make([]byte, 0x100)
	copy(data[0:13], "ProTracker 3.")
	data[0x63] = 5  // speed
	data[0x64] = 20 // positions
	data[0x65] = 0  // no loop

	info, err := parsePT3Header(data)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// frameCount = 20 * 64 * 5 = 6400
	if info.frameCount != 6400 {
		t.Errorf("frameCount = %d, want 6400", info.frameCount)
	}
}

func TestParseSTCHeader(t *testing.T) {
	data := make([]byte, 0x40)
	data[0x00] = 4 // speed
	data[0x01] = 8 // positions

	info, err := parseSTCHeader(data)
	if err != nil {
		t.Fatalf("parseSTCHeader error: %v", err)
	}
	if info.format != "STC" {
		t.Errorf("format = %q, want STC", info.format)
	}
	if info.speed != 4 {
		t.Errorf("speed = %d, want 4", info.speed)
	}
	if info.positions != 8 {
		t.Errorf("positions = %d, want 8", info.positions)
	}
	// frameCount = 8 * 64 * 4 = 2048
	if info.frameCount != 2048 {
		t.Errorf("frameCount = %d, want 2048", info.frameCount)
	}
}

func TestParseSTCHeader_TooShort(t *testing.T) {
	_, err := parseSTCHeader([]byte{0x01, 0x02})
	if err == nil {
		t.Error("expected error for short data")
	}
}

func TestParsePT2Header(t *testing.T) {
	data := make([]byte, 0x42)
	copy(data[:0x1E], "PT2 Test Song\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	data[0x1E] = 12 // positions
	data[0x1F] = 4  // loop pos

	info, err := parsePT2Header(data)
	if err != nil {
		t.Fatalf("parsePT2Header error: %v", err)
	}
	if info.format != "PT2" {
		t.Errorf("format = %q, want PT2", info.format)
	}
	if info.title != "PT2 Test Song" {
		t.Errorf("title = %q, want %q", info.title, "PT2 Test Song")
	}
	if info.positions != 12 {
		t.Errorf("positions = %d, want 12", info.positions)
	}
}

func TestParsePT1Header(t *testing.T) {
	data := make([]byte, 0x22)
	data[0x00] = 6 // positions

	info, err := parsePT1Header(data)
	if err != nil {
		t.Fatalf("parsePT1Header error: %v", err)
	}
	if info.format != "PT1" {
		t.Errorf("format = %q, want PT1", info.format)
	}
	if info.positions != 6 {
		t.Errorf("positions = %d, want 6", info.positions)
	}
}

func TestParseSQTHeader(t *testing.T) {
	data := make([]byte, 0x40)
	data[0x00] = 9 // positions (0-based, actual = 10)
	data[0x01] = 3 // loop pos
	data[0x02] = 5 // speed

	info, err := parseSQTHeader(data)
	if err != nil {
		t.Fatalf("parseSQTHeader error: %v", err)
	}
	if info.format != "SQT" {
		t.Errorf("format = %q, want SQT", info.format)
	}
	if info.positions != 10 {
		t.Errorf("positions = %d, want 10", info.positions)
	}
	if info.speed != 5 {
		t.Errorf("speed = %d, want 5", info.speed)
	}
}

func TestParseASCHeader(t *testing.T) {
	data := make([]byte, 0x30)
	data[0x00] = 0 // version 0
	data[0x01] = 4 // speed
	data[0x02] = 7 // positions
	data[0x03] = 1 // loop pos
	copy(data[0x04:0x24], "ASC Test Song\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")

	info, err := parseASCHeader(data)
	if err != nil {
		t.Fatalf("parseASCHeader error: %v", err)
	}
	if info.format != "ASC" {
		t.Errorf("format = %q, want ASC", info.format)
	}
	if info.title != "ASC Test Song" {
		t.Errorf("title = %q, want %q", info.title, "ASC Test Song")
	}
	if info.speed != 4 {
		t.Errorf("speed = %d, want 4", info.speed)
	}
	if info.positions != 7 {
		t.Errorf("positions = %d, want 7", info.positions)
	}
}

func TestParseFTCHeader(t *testing.T) {
	data := make([]byte, 0x20)
	copy(data[0:4], "FTC!")
	data[0x04] = 15 // positions
	data[0x05] = 5  // loop pos
	data[0x06] = 3  // speed

	info, err := parseFTCHeader(data)
	if err != nil {
		t.Fatalf("parseFTCHeader error: %v", err)
	}
	if info.format != "FTC" {
		t.Errorf("format = %q, want FTC", info.format)
	}
	if info.positions != 15 {
		t.Errorf("positions = %d, want 15", info.positions)
	}
	if info.speed != 3 {
		t.Errorf("speed = %d, want 3", info.speed)
	}
}

func TestParseTrackerModule_AllFormats(t *testing.T) {
	formats := []struct {
		ext     string
		minSize int
	}{
		{".pt3", 0x100},
		{".pt2", 0x42},
		{".pt1", 0x22},
		{".stc", 0x40},
		{".sqt", 0x40},
		{".asc", 0x30},
		{".ftc", 0x20},
	}
	for _, f := range formats {
		data := make([]byte, f.minSize)
		if f.ext == ".pt3" {
			copy(data[0:13], "ProTracker 3.")
			data[0x63] = 3
			data[0x64] = 1
		} else if f.ext == ".ftc" {
			copy(data[0:4], "FTC!")
			data[0x04] = 1
			data[0x06] = 3
		} else if f.ext == ".stc" {
			data[0x00] = 3
			data[0x01] = 1
		} else if f.ext == ".sqt" {
			data[0x02] = 3
		} else if f.ext == ".asc" {
			data[0x01] = 3
			data[0x02] = 1
		} else if f.ext == ".pt2" {
			data[0x1E] = 1
		} else if f.ext == ".pt1" {
			data[0x00] = 1
		}

		info, err := parseTrackerModule(f.ext, data)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", f.ext, err)
			continue
		}
		if info.frameCount <= 0 {
			t.Errorf("%s: frameCount = %d, want > 0", f.ext, info.frameCount)
		}
	}
}

func TestIsTrackerFormat(t *testing.T) {
	for _, ext := range []string{".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc"} {
		if !isTrackerFormat(ext) {
			t.Errorf("isTrackerFormat(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{".ym", ".ay", ".vgm", ".mp3", ".wav"} {
		if isTrackerFormat(ext) {
			t.Errorf("isTrackerFormat(%q) = true, want false", ext)
		}
	}
}
