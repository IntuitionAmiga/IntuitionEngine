package main

import (
	"os"
	"testing"
)

// Test 1: renderSAP returns valid metadata
func TestRenderSAP_Metadata(t *testing.T) {
	data := buildTestSAPData("Test Song", "Test Author", "1986")
	meta, _, _, _, _, _, _, err := renderSAP(data, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Title != "Test Song" {
		t.Errorf("expected title 'Test Song', got '%s'", meta.Title)
	}
	if meta.Author != "Test Author" {
		t.Errorf("expected author 'Test Author', got '%s'", meta.Author)
	}
}

// Test 2: renderSAP returns POKEY events
func TestRenderSAP_Events(t *testing.T) {
	data := buildTestSAPWithPOKEYWrites()
	_, events, _, _, _, _, _, err := renderSAP(data, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected at least one POKEY event")
	}
}

// Test 3: renderSAP returns correct POKEY clock
func TestRenderSAP_ClockHz(t *testing.T) {
	data := buildTestSAPDataPAL()
	_, _, _, clockHz, _, _, _, _ := renderSAP(data, 44100)
	if clockHz != POKEY_CLOCK_PAL {
		t.Errorf("expected POKEY clock %d, got %d", POKEY_CLOCK_PAL, clockHz)
	}
}

// Test 4: renderSAP with frame limit
func TestRenderSAPWithLimit(t *testing.T) {
	data := buildTestSAPWithPOKEYWrites()
	_, events1, _, _, _, _, _, _ := renderSAPWithLimit(data, 44100, 10, 0)
	_, events2, _, _, _, _, _, _ := renderSAPWithLimit(data, 44100, 100, 0)
	if len(events1) >= len(events2) {
		t.Errorf("expected fewer events with smaller limit: %d vs %d", len(events1), len(events2))
	}
}

// Test 5: renderSAP with real file
func TestRenderSAP_RealFile(t *testing.T) {
	data, err := os.ReadFile("/home/zayn/Music/asma/Games/Jumpman.sap")
	if os.IsNotExist(err) {
		t.Skip("Test SAP file not found")
	}
	if err != nil {
		t.Skipf("Error loading SAP file: %v", err)
	}
	meta, events, total, clockHz, _, _, _, err := renderSAP(data, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Title == "" {
		t.Error("expected non-empty title")
	}
	if len(events) == 0 {
		t.Error("expected at least one POKEY event")
	}
	if total == 0 {
		t.Error("expected non-zero total samples")
	}
	if clockHz != POKEY_CLOCK_PAL {
		t.Errorf("expected PAL clock, got %d", clockHz)
	}
}

// Test 6: Frame rate calculation
func TestRenderSAP_FrameRate(t *testing.T) {
	data := buildTestSAPDataPAL()
	_, _, _, _, frameRate, _, _, _ := renderSAP(data, 44100)
	// PAL rate is approximately 50 Hz
	if frameRate < 49 || frameRate > 51 {
		t.Errorf("expected frame rate ~50, got %d", frameRate)
	}
}

// Test 7: POKEY events have proper register values
func TestRenderSAP_POKEYEventFormat(t *testing.T) {
	data := buildTestSAPWithPOKEYWrites()
	// Use limited frames
	_, events, total, _, _, _, _, _ := renderSAPWithLimit(data, 44100, 50, 0)
	// Events should have valid sample positions within total
	for _, e := range events {
		if e.Sample > total {
			t.Errorf("sample position %d exceeds total %d", e.Sample, total)
		}
		// POKEY has registers 0-9
		if e.Reg > 9 {
			t.Errorf("POKEY register %d is invalid (max 9)", e.Reg)
		}
	}
}

// Test 8: Stereo detection
func TestRenderSAP_StereoMetadata(t *testing.T) {
	// Build stereo SAP data
	header := "SAP\r\nNAME \"Stereo Test\"\r\nAUTHOR \"Author\"\r\nSTEREO\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\n"
	binary := []byte{0xFF, 0xFF, 0x00, 0x10, 0x00, 0x10, 0x60}
	data := append([]byte(header), binary...)

	meta, _, _, _, _, _, _, err := renderSAP(data, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.Stereo {
		t.Error("expected stereo flag to be set")
	}
}

// Helper functions for tests

func buildTestSAPDataPAL() []byte {
	return buildTestSAPData("Test", "Author", "1986")
}

func buildTestSAPWithPOKEYWrites() []byte {
	// SAP file with PLAYER routine that writes to POKEY
	header := "SAP\r\nNAME \"Test\"\r\nAUTHOR \"Author\"\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1010\r\n"
	binary := []byte{
		0xFF, 0xFF,
		// Block 1: INIT at $1000 - just RTS
		0x00, 0x10, 0x00, 0x10,
		0x60, // RTS
		// Block 2: PLAYER at $1010 - write to POKEY then RTS
		0xFF, 0xFF,
		0x10, 0x10, 0x1A, 0x10,
		0xA9, 0x50, // LDA #$50
		0x8D, 0x00, 0xD2, // STA $D200 (AUDF1)
		0xA9, 0xAF, // LDA #$AF
		0x8D, 0x01, 0xD2, // STA $D201 (AUDC1)
		0x60, // RTS
	}
	return append([]byte(header), binary...)
}
