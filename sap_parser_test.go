package main

import (
	"math"
	"os"
	"strings"
	"testing"
)

// Test 1: Detect SAP file signature
func TestIsSAPData_ValidSignature(t *testing.T) {
	data := []byte("SAP\r\nAUTHOR \"Test\"\r\n")
	if !isSAPData(data) {
		t.Error("expected valid SAP signature to be detected")
	}
}

func TestIsSAPData_InvalidSignature(t *testing.T) {
	data := []byte("ZXAYEMUL") // AY file, not SAP
	if isSAPData(data) {
		t.Error("expected AY signature to not be detected as SAP")
	}
}

func TestIsSAPData_TooShort(t *testing.T) {
	data := []byte("SAP")
	if isSAPData(data) {
		t.Error("expected short data to not be detected as SAP")
	}
}

// Test 2: Parse minimal header
func TestParseSAPData_MinimalHeader(t *testing.T) {
	data := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1003\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'B' {
		t.Errorf("expected TYPE B, got %c", file.Header.Type)
	}
	if file.Header.Init != 0x1000 {
		t.Errorf("expected INIT 0x1000, got 0x%04X", file.Header.Init)
	}
	if file.Header.Player != 0x1003 {
		t.Errorf("expected PLAYER 0x1003, got 0x%04X", file.Header.Player)
	}
}

// Test 3: Parse all header tags
func TestParseSAPData_AllTags(t *testing.T) {
	data := []byte("SAP\r\n" +
		"AUTHOR \"Composer Name\"\r\n" +
		"NAME \"Song Title\"\r\n" +
		"DATE \"1986\"\r\n" +
		"SONGS 3\r\n" +
		"DEFSONG 1\r\n" +
		"STEREO\r\n" +
		"NTSC\r\n" +
		"TYPE B\r\n" +
		"FASTPLAY 156\r\n" +
		"INIT 31C2\r\n" +
		"PLAYER 31F1\r\n" +
		"TIME 00:13.47\r\n" +
		"TIME 00:03.45\r\n" +
		"TIME 00:07.06\r\n" +
		"\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Author != "Composer Name" {
		t.Errorf("expected author 'Composer Name', got '%s'", file.Header.Author)
	}
	if file.Header.Name != "Song Title" {
		t.Errorf("expected name 'Song Title', got '%s'", file.Header.Name)
	}
	if file.Header.Date != "1986" {
		t.Errorf("expected date '1986', got '%s'", file.Header.Date)
	}
	if file.Header.Songs != 3 {
		t.Errorf("expected 3 songs, got %d", file.Header.Songs)
	}
	if file.Header.DefSong != 1 {
		t.Errorf("expected default song 1, got %d", file.Header.DefSong)
	}
	if !file.Header.Stereo {
		t.Error("expected STEREO flag to be set")
	}
	if !file.Header.NTSC {
		t.Error("expected NTSC flag to be set")
	}
	if file.Header.Type != 'B' {
		t.Errorf("expected TYPE B, got %c", file.Header.Type)
	}
	if file.Header.FastPlay != 156 {
		t.Errorf("expected FASTPLAY 156, got %d", file.Header.FastPlay)
	}
	if file.Header.Init != 0x31C2 {
		t.Errorf("expected INIT 0x31C2, got 0x%04X", file.Header.Init)
	}
	if file.Header.Player != 0x31F1 {
		t.Errorf("expected PLAYER 0x31F1, got 0x%04X", file.Header.Player)
	}
	if len(file.Header.Durations) != 3 {
		t.Errorf("expected 3 durations, got %d", len(file.Header.Durations))
	}
}

// Test 4: Parse binary blocks
func TestParseSAPData_BinaryBlocks(t *testing.T) {
	header := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\n")
	binary := []byte{
		0xFF, 0xFF, // Block marker
		0x00, 0x10, // Start: $1000 (little-endian)
		0x03, 0x10, // End: $1003 (little-endian)
		0xA9, 0x00, 0x60, 0x00, // Data: LDA #0, RTS, BRK
	}
	data := append(header, binary...)

	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(file.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(file.Blocks))
	}
	if file.Blocks[0].Start != 0x1000 {
		t.Errorf("expected start 0x1000, got 0x%04X", file.Blocks[0].Start)
	}
	if file.Blocks[0].End != 0x1003 {
		t.Errorf("expected end 0x1003, got 0x%04X", file.Blocks[0].End)
	}
	expected := []byte{0xA9, 0x00, 0x60, 0x00}
	if len(file.Blocks[0].Data) != len(expected) {
		t.Errorf("expected data length %d, got %d", len(expected), len(file.Blocks[0].Data))
	}
	for i, b := range expected {
		if i < len(file.Blocks[0].Data) && file.Blocks[0].Data[i] != b {
			t.Errorf("data[%d]: expected 0x%02X, got 0x%02X", i, b, file.Blocks[0].Data[i])
		}
	}
}

// Test 5: Parse multiple binary blocks
func TestParseSAPData_MultipleBlocks(t *testing.T) {
	header := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 2000\r\n")
	binary := []byte{
		0xFF, 0xFF, 0x00, 0x10, 0x01, 0x10, 0xEA, 0xEA, // Block 1: $1000-$1001
		0xFF, 0xFF, 0x00, 0x20, 0x01, 0x20, 0x60, 0x60, // Block 2: $2000-$2001
	}
	data := append(header, binary...)

	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(file.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(file.Blocks))
	}
}

// Test 6: Parse TIME tag with milliseconds
func TestParseSAPData_TimeWithMillis(t *testing.T) {
	data := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\nTIME 01:23.456\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 83.456
	if len(file.Header.Durations) < 1 {
		t.Fatal("expected at least one duration")
	}
	if math.Abs(file.Header.Durations[0]-expected) > 0.001 {
		t.Errorf("expected duration %.3f, got %.3f", expected, file.Header.Durations[0])
	}
}

// Test 7: Parse TIME tag with LOOP marker
func TestParseSAPData_TimeWithLoop(t *testing.T) {
	data := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\nTIME 02:00 LOOP\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 120.0
	if len(file.Header.Durations) < 1 {
		t.Fatal("expected at least one duration")
	}
	if math.Abs(file.Header.Durations[0]-expected) > 0.001 {
		t.Errorf("expected duration %.3f, got %.3f", expected, file.Header.Durations[0])
	}
}

// Test 8: Error on missing required tags
func TestParseSAPData_MissingType(t *testing.T) {
	data := []byte("SAP\r\nINIT 1000\r\nPLAYER 1000\r\n\xFF\xFF")
	_, err := ParseSAPData(data)
	if err == nil {
		t.Fatal("expected error for missing TYPE")
	}
	if !strings.Contains(err.Error(), "TYPE") {
		t.Errorf("expected error to mention TYPE, got: %v", err)
	}
}

// Test 9: Error on invalid TYPE
func TestParseSAPData_InvalidType(t *testing.T) {
	data := []byte("SAP\r\nTYPE X\r\nINIT 1000\r\nPLAYER 1000\r\n\xFF\xFF")
	_, err := ParseSAPData(data)
	if err == nil {
		t.Fatal("expected error for invalid TYPE")
	}
}

// Test 10: Parse real SAP file
func TestParseSAPFile_RealFile(t *testing.T) {
	file, err := ParseSAPFile("/home/zayn/Music/asma/Games/Jumpman.sap")
	if os.IsNotExist(err) {
		t.Skip("Test SAP file not found")
	}
	if err != nil {
		t.Skipf("Error loading SAP file: %v", err)
	}
	if file.Header.Name == "" {
		t.Error("expected non-empty name")
	}
	if file.Header.Type != 'B' {
		t.Errorf("expected TYPE B, got %c", file.Header.Type)
	}
	if len(file.Blocks) == 0 {
		t.Error("expected at least one block")
	}
}

// Test 11: Parse TYPE C (player at fixed address)
func TestParseSAPData_TypeC(t *testing.T) {
	data := []byte("SAP\r\nTYPE C\r\nPLAYER 4000\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'C' {
		t.Errorf("expected TYPE C, got %c", file.Header.Type)
	}
	if file.Header.Player != 0x4000 {
		t.Errorf("expected PLAYER 0x4000, got 0x%04X", file.Header.Player)
	}
}

// Test 12: Parse MUSIC tag for TYPE C
func TestParseSAPData_TypeC_Music(t *testing.T) {
	data := []byte("SAP\r\nTYPE C\r\nPLAYER 4000\r\nMUSIC 2000\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Music != 0x2000 {
		t.Errorf("expected MUSIC 0x2000, got 0x%04X", file.Header.Music)
	}
}

// Test 13: Parse line endings (LF only, common in some files)
func TestParseSAPData_LFLineEndings(t *testing.T) {
	data := []byte("SAP\nTYPE B\nINIT 1000\nPLAYER 1000\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'B' {
		t.Errorf("expected TYPE B, got %c", file.Header.Type)
	}
}

// Test 14: Default FASTPLAY for PAL
func TestParseSAPData_DefaultFastPlayPAL(t *testing.T) {
	data := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.FastPlay != 312 {
		t.Errorf("expected default PAL FASTPLAY 312, got %d", file.Header.FastPlay)
	}
}

// Test 15: Default FASTPLAY for NTSC
func TestParseSAPData_DefaultFastPlayNTSC(t *testing.T) {
	data := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\nNTSC\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.FastPlay != 262 {
		t.Errorf("expected default NTSC FASTPLAY 262, got %d", file.Header.FastPlay)
	}
}

// Test 16: Parse TYPE D (digital, INIT only, no PLAYER required)
func TestParseSAPData_TypeD(t *testing.T) {
	data := []byte("SAP\r\nTYPE D\r\nINIT 2000\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'D' {
		t.Errorf("expected TYPE D, got %c", file.Header.Type)
	}
	if file.Header.Init != 0x2000 {
		t.Errorf("expected INIT 0x2000, got 0x%04X", file.Header.Init)
	}
	// Type D should NOT require PLAYER
	if file.Header.Player != 0 {
		t.Errorf("expected PLAYER 0, got 0x%04X", file.Header.Player)
	}
}

// Test 17: Parse TYPE S (SoftSynth)
func TestParseSAPData_TypeS(t *testing.T) {
	data := []byte("SAP\r\nTYPE S\r\nINIT 3000\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'S' {
		t.Errorf("expected TYPE S, got %c", file.Header.Type)
	}
	if file.Header.Init != 0x3000 {
		t.Errorf("expected INIT 0x3000, got 0x%04X", file.Header.Init)
	}
}

// Test 18: Parse TYPE R (raw POKEY register dump)
func TestParseSAPData_TypeR(t *testing.T) {
	data := []byte("SAP\r\nTYPE R\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'R' {
		t.Errorf("expected TYPE R, got %c", file.Header.Type)
	}
}

// Test 19: TYPE D with FASTPLAY override
func TestParseSAPData_TypeD_FastPlay(t *testing.T) {
	data := []byte("SAP\r\nTYPE D\r\nINIT 2000\r\nFASTPLAY 156\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.Header.Type != 'D' {
		t.Errorf("expected TYPE D, got %c", file.Header.Type)
	}
	if file.Header.FastPlay != 156 {
		t.Errorf("expected FASTPLAY 156, got %d", file.Header.FastPlay)
	}
}

// Test 20: STEREO with SONGS and DEFSONG
func TestParseSAPData_StereoMultiSong(t *testing.T) {
	data := []byte("SAP\r\nTYPE B\r\nINIT 1000\r\nPLAYER 1000\r\nSTEREO\r\nSONGS 5\r\nDEFSONG 2\r\n\xFF\xFF")
	file, err := ParseSAPData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !file.Header.Stereo {
		t.Error("expected STEREO flag to be set")
	}
	if file.Header.Songs != 5 {
		t.Errorf("expected 5 songs, got %d", file.Header.Songs)
	}
	if file.Header.DefSong != 2 {
		t.Errorf("expected default song 2, got %d", file.Header.DefSong)
	}
}

// Test helper to build test SAP data
func buildTestSAPData(name, author, date string) []byte {
	header := "SAP\r\n"
	if name != "" {
		header += "NAME \"" + name + "\"\r\n"
	}
	if author != "" {
		header += "AUTHOR \"" + author + "\"\r\n"
	}
	if date != "" {
		header += "DATE \"" + date + "\"\r\n"
	}
	header += "TYPE B\r\nINIT 1000\r\nPLAYER 1000\r\n"
	return append([]byte(header), 0xFF, 0xFF, 0x00, 0x10, 0x00, 0x10, 0x60)
}
