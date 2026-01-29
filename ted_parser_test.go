// ted_parser_test.go - Tests for TED/TMF file parser

package main

import (
	"os"
	"testing"
)

func TestDetectTEDFormat_TMF(t *testing.T) {
	// Create a mock TMF file with TEDMUSIC at offset 17
	data := make([]byte, 200)
	data[0] = 0x01 // Load address low
	data[1] = 0x10 // Load address high ($1001)
	data[2] = 0x10 // BASIC line number low
	data[3] = 0x00 // BASIC line number high (< 4096)

	// Place TEDMUSIC signature at offset 17
	copy(data[TMF_SIGNATURE_OFFSET:], "TEDMUSIC")

	format, sigPos := detectTEDFormat(data)

	if format != TEDFormatTMF {
		t.Errorf("expected TEDFormatTMF, got %v", format)
	}
	if sigPos != TMF_SIGNATURE_OFFSET {
		t.Errorf("expected signature at %d, got %d", TMF_SIGNATURE_OFFSET, sigPos)
	}
}

func TestDetectTEDFormat_HVTC(t *testing.T) {
	// Create a mock HVTC file with TEDMUSIC near end
	data := make([]byte, 500)
	data[0] = 0x01 // Load address low
	data[1] = 0x10 // Load address high ($1001)

	// Place TEDMUSIC signature near the end (not at offset 17)
	copy(data[400:], "TEDMUSIC")

	format, sigPos := detectTEDFormat(data)

	if format != TEDFormatHVTC {
		t.Errorf("expected TEDFormatHVTC, got %v", format)
	}
	if sigPos != 400 {
		t.Errorf("expected signature at 400, got %d", sigPos)
	}
}

func TestDetectTEDFormat_Raw(t *testing.T) {
	// Create a mock raw PRG file without TEDMUSIC signature
	data := make([]byte, 100)
	data[0] = 0x01 // Load address low
	data[1] = 0x10 // Load address high ($1001)
	data[2] = 0x60 // RTS instruction

	format, sigPos := detectTEDFormat(data)

	if format != TEDFormatRaw {
		t.Errorf("expected TEDFormatRaw, got %v", format)
	}
	if sigPos != -1 {
		t.Errorf("expected no signature (-1), got %d", sigPos)
	}
}

func TestTEDFormatString(t *testing.T) {
	tests := []struct {
		format TEDFormat
		want   string
	}{
		{TEDFormatTMF, "TMF"},
		{TEDFormatHVTC, "HVTC"},
		{TEDFormatRaw, "RAW"},
	}

	for _, tt := range tests {
		got := tt.format.String()
		if got != tt.want {
			t.Errorf("TEDFormat(%d).String() = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestParseTEDFile_WithTMFHeader(t *testing.T) {
	// Create a mock TMF file with full header
	data := make([]byte, 300)
	data[0] = 0x01 // Load address low
	data[1] = 0x10 // Load address high ($1001)
	data[2] = 0x10 // BASIC line number low
	data[3] = 0x00 // BASIC line number high

	// Place TEDMUSIC signature at offset 17
	sigStart := TMF_SIGNATURE_OFFSET
	copy(data[sigStart:], "TEDMUSIC\x00")

	// Init offset (relative)
	data[sigStart+TED_HDR_INIT_LO] = 0x00
	data[sigStart+TED_HDR_INIT_HI] = 0x20 // $2000

	// Play address
	data[sigStart+TED_HDR_PLAY_LO] = 0x00
	data[sigStart+TED_HDR_PLAY_HI] = 0x20 // $2000

	// End address
	data[sigStart+TED_HDR_END_LO] = 0x00
	data[sigStart+TED_HDR_END_HI] = 0x30 // $3000

	// Subtunes
	data[sigStart+TED_HDR_SUBTUNES] = 3
	data[sigStart+TED_HDR_SUBTUNES+1] = 0

	// FileFlags
	data[sigStart+TED_HDR_FLAGS] = 0xFE

	// Metadata strings (at offset 48 from signature)
	stringStart := sigStart + TED_HDR_STRINGS
	copy(data[stringStart:], "Test Title                      ")
	copy(data[stringStart+32:], "Test Author                     ")
	copy(data[stringStart+64:], "2024-01-01                      ")
	copy(data[stringStart+96:], "Test Tool                       ")

	file, err := parseTEDFile(data)
	if err != nil {
		t.Fatalf("parseTEDFile failed: %v", err)
	}

	if file.FormatType != TEDFormatTMF {
		t.Errorf("FormatType = %v, want TEDFormatTMF", file.FormatType)
	}
	if file.LoadAddr != 0x1001 {
		t.Errorf("LoadAddr = $%04X, want $1001", file.LoadAddr)
	}
	if file.Subtunes != 3 {
		t.Errorf("Subtunes = %d, want 3", file.Subtunes)
	}
	if file.FileFlags != 0xFE {
		t.Errorf("FileFlags = $%02X, want $FE", file.FileFlags)
	}
	if file.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", file.Title, "Test Title")
	}
	if file.Author != "Test Author" {
		t.Errorf("Author = %q, want %q", file.Author, "Test Author")
	}
}

func TestParseTEDFile_RealTEDMode(t *testing.T) {
	// Create a mock TMF file with PlayAddr=0 (RealTED mode)
	data := make([]byte, 300)
	data[0] = 0x01 // Load address low
	data[1] = 0x10 // Load address high ($1001)
	data[2] = 0x10 // BASIC line number low
	data[3] = 0x00 // BASIC line number high

	// Place TEDMUSIC signature at offset 17
	sigStart := TMF_SIGNATURE_OFFSET
	copy(data[sigStart:], "TEDMUSIC\x00")

	// Init offset
	data[sigStart+TED_HDR_INIT_LO] = 0x00
	data[sigStart+TED_HDR_INIT_HI] = 0x20 // $2000

	// Play address = 0 (RealTED mode)
	data[sigStart+TED_HDR_PLAY_LO] = 0x00
	data[sigStart+TED_HDR_PLAY_HI] = 0x00

	file, err := parseTEDFile(data)
	if err != nil {
		t.Fatalf("parseTEDFile failed: %v", err)
	}

	if !file.RealTEDMode {
		t.Error("RealTEDMode should be true when PlayAddr=0")
	}
	if file.PlayAddr != 0 {
		t.Errorf("PlayAddr = $%04X, want $0000", file.PlayAddr)
	}
	if file.InitAddr != 0x2000 {
		t.Errorf("InitAddr = $%04X, want $2000", file.InitAddr)
	}
}

func TestParseMetadataString_Latin1(t *testing.T) {
	// Test Latin-1 encoding (TMF format)
	data := []byte("Test String\x00     padding")
	result := parseMetadataString(data, true)
	if result != "Test String" {
		t.Errorf("parseMetadataString(Latin1) = %q, want %q", result, "Test String")
	}
}

func TestParseMetadataString_PETSCII(t *testing.T) {
	// Test PETSCII encoding (HVTC format)
	// PETSCII uppercase A-Z (0x41-0x5A) should convert to lowercase
	data := []byte{0x41, 0x42, 0x43, 0x00} // "ABC" in PETSCII
	result := parseMetadataString(data, false)
	if result != "abc" {
		t.Errorf("parseMetadataString(PETSCII) = %q, want %q", result, "abc")
	}
}

func TestParseTEDFile_RealHVTCFile(t *testing.T) {
	// Test with a real HVTC file if available
	testFiles := []string{
		"/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted",
		"/home/zayn/Music/HVTC-ted/musicians/tobikomi/llama_polka.ted",
	}

	var data []byte
	var err error
	for _, path := range testFiles {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Skip("test file not found")
	}

	file, err := parseTEDFile(data)
	if err != nil {
		t.Fatalf("parseTEDFile failed: %v", err)
	}

	t.Logf("Format: %v", file.FormatType)
	t.Logf("LoadAddr: $%04X", file.LoadAddr)
	t.Logf("InitAddr: $%04X", file.InitAddr)
	t.Logf("PlayAddr: $%04X", file.PlayAddr)
	t.Logf("EndAddr: $%04X", file.EndAddr)
	t.Logf("RealTEDMode: %v", file.RealTEDMode)
	t.Logf("Subtunes: %d", file.Subtunes)
	t.Logf("Title: %q", file.Title)
	t.Logf("Author: %q", file.Author)
	t.Logf("Date: %q", file.Date)
	t.Logf("Tool: %q", file.Tool)

	// Basic sanity checks
	if file.LoadAddr == 0 {
		t.Error("LoadAddr should not be 0")
	}
	if file.FormatType == TEDFormatRaw && file.Title != "" {
		t.Error("Raw format should not have metadata")
	}
}

func TestSubtuneSelection(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	player, err := NewTED6502Player(engine, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("Failed to create player: %v", err)
	}

	// Create a mock file with 3 subtunes
	data := make([]byte, 300)
	data[0] = 0x01 // Load address low
	data[1] = 0x10 // Load address high ($1001)
	data[2] = 0x10 // BASIC line number
	data[3] = 0x00

	// Add minimal BASIC stub with SYS
	basicStart := 2
	data[basicStart] = 0x0E   // Next line low
	data[basicStart+1] = 0x10 // Next line high
	data[basicStart+2] = 0x0A // Line 10 low
	data[basicStart+3] = 0x00 // Line 10 high
	data[basicStart+4] = 0x9E // SYS token
	data[basicStart+5] = '4'
	data[basicStart+6] = '1'
	data[basicStart+7] = '0'
	data[basicStart+8] = '9'
	data[basicStart+9] = 0x00

	// At $100D (offset 12), add JMP to music code
	data[12] = 0x4C // JMP
	data[13] = 0x00
	data[14] = 0x20 // Target $2000

	// At $2000 (offset $FFF in file), add RTS
	offset := 0x2000 - 0x1001
	if offset < len(data) {
		data[offset] = 0x60 // RTS
	}

	// TEDMUSIC signature at offset 17
	sigStart := TMF_SIGNATURE_OFFSET
	copy(data[sigStart:], "TEDMUSIC\x00")
	data[sigStart+TED_HDR_SUBTUNES] = 3
	data[sigStart+TED_HDR_SUBTUNES+1] = 0

	err = player.LoadFromData(data)
	if err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}

	// Test subtune count
	if count := player.GetSubtuneCount(); count != 3 {
		t.Errorf("GetSubtuneCount() = %d, want 3", count)
	}

	// Test initial subtune
	if current := player.GetCurrentSubtune(); current != 0 {
		t.Errorf("GetCurrentSubtune() = %d, want 0", current)
	}

	// Test selecting subtune 2
	err = player.SelectSubtune(2)
	if err != nil {
		t.Fatalf("SelectSubtune(2) failed: %v", err)
	}
	if current := player.GetCurrentSubtune(); current != 2 {
		t.Errorf("GetCurrentSubtune() = %d, want 2", current)
	}

	// Test selecting invalid subtune
	err = player.SelectSubtune(5)
	if err == nil {
		t.Error("SelectSubtune(5) should fail for 3 subtunes")
	}
}
