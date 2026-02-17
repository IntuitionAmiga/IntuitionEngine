package main

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestPSGPlayer(t *testing.T) *PSGPlayer {
	t.Helper()
	engine := NewPSGEngine(nil, 44100)
	return NewPSGPlayer(engine)
}

func TestPSGPlayer_LoadVTX(t *testing.T) {
	// Build a minimal VTX file
	vtxData := buildTestVTXFile(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.vtx")
	if err := os.WriteFile(path, vtxData, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load VTX error: %v", err)
	}
	if player.metadata.System == "" {
		t.Error("metadata.System is empty")
	}
}

func TestPSGPlayer_LoadDataVTX(t *testing.T) {
	vtxData := buildTestVTXFile(t)

	player := newTestPSGPlayer(t)
	if err := player.LoadData(vtxData); err != nil {
		t.Fatalf("LoadData VTX error: %v", err)
	}
}

func TestPSGPlayer_LoadPT3(t *testing.T) {
	module := make([]byte, 0x100)
	copy(module[0:13], "ProTracker 3.")
	module[0x0D] = '5'
	module[0x63] = 3
	module[0x64] = 1
	module[0x65] = 0

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pt3")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load PT3 error: %v", err)
	}
	if player.metadata.Title != "" {
		// Title may be empty for this minimal module, that's OK
	}
}

func TestPSGPlayer_LoadSTC(t *testing.T) {
	module := make([]byte, 0x40)
	module[0x00] = 3
	module[0x01] = 1

	dir := t.TempDir()
	path := filepath.Join(dir, "test.stc")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load STC error: %v", err)
	}
}

func TestPSGPlayer_LoadPT2(t *testing.T) {
	module := make([]byte, 0x42)
	module[0x1E] = 1

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pt2")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load PT2 error: %v", err)
	}
}

func TestPSGPlayer_LoadPT1(t *testing.T) {
	// Build a minimal valid PT1 module for native parser
	// Header: tempo(1), length(1), loop(1), sampleOffsets[16]×u16LE(32), ornamentOffsets[16]×u16LE(32),
	//         patternsOffset(u16LE)(2), name[30], positions...0xFF
	module := make([]byte, 0x200)
	module[0] = 3 // tempo
	module[1] = 1 // length
	module[2] = 0 // loop
	// Patterns offset at bytes 67-68: point after positions
	patternsOff := 101 // after positions (99 + 1 position + 0xFF marker)
	module[67] = byte(patternsOff)
	module[68] = byte(patternsOff >> 8)
	// Position 0 at offset 99
	module[99] = 0
	module[100] = 0xFF
	// Pattern 0: 3 channel offsets (u16LE each)
	chanDataOff := patternsOff + 6 // after pattern header
	module[patternsOff] = byte(chanDataOff)
	module[patternsOff+1] = byte(chanDataOff >> 8)
	module[patternsOff+2] = byte(chanDataOff + 2)
	module[patternsOff+3] = byte((chanDataOff + 2) >> 8)
	module[patternsOff+4] = byte(chanDataOff + 4)
	module[patternsOff+5] = byte((chanDataOff + 4) >> 8)
	// Channel data: 0x90 (empty/no note) for each channel, then 0xFF to end
	module[chanDataOff] = 0x90   // ch A: empty
	module[chanDataOff+1] = 0xFF // end marker (for ch A at start of next line check)
	module[chanDataOff+2] = 0x90 // ch B: empty
	module[chanDataOff+3] = 0x90 // padding
	module[chanDataOff+4] = 0x90 // ch C: empty
	module[chanDataOff+5] = 0x90 // padding

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pt1")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load PT1 error: %v", err)
	}
}

func TestPSGPlayer_LoadSQT(t *testing.T) {
	module := make([]byte, 0x40)
	module[0x02] = 3

	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqt")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load SQT error: %v", err)
	}
}

func TestPSGPlayer_LoadASC(t *testing.T) {
	// Build a minimal valid ASC v0 module for native parser
	// v0 header: tempo(1), patternsOff(u16LE), samplesOff(u16LE), ornamentsOff(u16LE), length(1), positions[]
	module := make([]byte, 0x200)
	module[0] = 5    // tempo
	module[1] = 0x40 // patterns offset lo
	module[2] = 0x00 // patterns offset hi
	module[3] = 0x80 // samples offset lo
	module[4] = 0x00 // samples offset hi
	module[5] = 0xC0 // ornaments offset lo
	module[6] = 0x00 // ornaments offset hi
	module[7] = 1    // length
	module[8] = 0    // position 0

	// Pattern 0 at offset 0x40: 3 channel offsets
	chanBase := 0x46
	module[0x40] = byte(chanBase)
	module[0x41] = byte(chanBase >> 8)
	module[0x42] = byte(chanBase + 1)
	module[0x43] = byte((chanBase + 1) >> 8)
	module[0x44] = byte(chanBase + 2)
	module[0x45] = byte((chanBase + 2) >> 8)
	// Channel data: 0x5F (rest) for each
	module[chanBase] = 0x5F   // rest
	module[chanBase+1] = 0x5F // rest
	module[chanBase+2] = 0x5F // rest
	// Pattern end marker
	module[chanBase+3] = 0xFF

	dir := t.TempDir()
	path := filepath.Join(dir, "test.asc")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load ASC error: %v", err)
	}
}

func TestPSGPlayer_LoadFTC(t *testing.T) {
	// Build a minimal valid FTC module for native parser (212-byte header + positions)
	module := make([]byte, 0x200)
	copy(module[0:8], "Module: ")
	module[69] = 5    // tempo
	module[70] = 0    // loop
	module[75] = 0xE0 // patterns offset lo
	module[76] = 0x00 // patterns offset hi
	// Position at offset 212
	module[212] = 0    // pattern 0
	module[213] = 0    // transposition 0
	module[214] = 0xFF // end marker

	// Pattern 0 at offset 0xE0: 3 channel offsets
	chanBase := 0xE6
	module[0xE0] = byte(chanBase)
	module[0xE1] = byte(chanBase >> 8)
	module[0xE2] = byte(chanBase + 1)
	module[0xE3] = byte((chanBase + 1) >> 8)
	module[0xE4] = byte(chanBase + 2)
	module[0xE5] = byte((chanBase + 2) >> 8)
	// Channel data: 0x30 (rest) for each
	module[chanBase] = 0x30   // rest
	module[chanBase+1] = 0x30 // rest
	module[chanBase+2] = 0x30 // rest
	module[chanBase+3] = 0xFF // end

	dir := t.TempDir()
	path := filepath.Join(dir, "test.ftc")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load FTC error: %v", err)
	}
}

func TestIsPSGExtension_NewFormats(t *testing.T) {
	for _, ext := range []string{".vtx", ".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc"} {
		if !isPSGExtension("song" + ext) {
			t.Errorf("isPSGExtension(song%s) = false, want true", ext)
		}
	}
}

func TestDetectMediaType_NewFormats(t *testing.T) {
	for _, ext := range []string{".vtx", ".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc", ".vgm", ".vgz", ".snd"} {
		typ := detectMediaType("song" + ext)
		if typ != MEDIA_TYPE_PSG {
			t.Errorf("detectMediaType(song%s) = %d, want MEDIA_TYPE_PSG (%d)", ext, typ, MEDIA_TYPE_PSG)
		}
	}
}

func TestPSGPlayer_LoadRealPT3(t *testing.T) {
	path := "testdata/music/test_pt3.pt3"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("real PT3 file not available")
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load real PT3 error: %v", err)
	}
	if player.metadata.Title == "" {
		t.Log("note: real PT3 title is empty (may be normal)")
	}
}

func TestPSGPlayer_LoadRealVTX(t *testing.T) {
	path := "testdata/music/test_vtx.vtx"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("real VTX file not available")
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load real VTX error: %v", err)
	}
	if player.metadata.System == "" {
		t.Error("metadata.System is empty for real VTX")
	}
}

func TestPSGPlayer_LoadRealSTC(t *testing.T) {
	path := "testdata/music/test_stc.stc"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("real STC file not available")
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load real STC error: %v", err)
	}
}

func TestPSGPlayer_LoadRealSQT(t *testing.T) {
	path := "testdata/music/test_sqt.sqt"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("real SQT file not available")
	}

	player := newTestPSGPlayer(t)
	if err := player.Load(path); err != nil {
		t.Fatalf("Load real SQT error: %v", err)
	}
}

// buildTestVTXFile creates a minimal valid VTX file for testing.
func buildTestVTXFile(t *testing.T) []byte {
	t.Helper()

	// Create 3 frames of 14 registers, interleaved (YM3 format)
	numFrames := 3
	numRegs := 14
	interleaved := make([]byte, numFrames*numRegs)
	for reg := range numRegs {
		for frame := range numFrames {
			interleaved[reg*numFrames+frame] = byte(reg + frame)
		}
	}

	return buildVTXFile("ay", 0, 1773400, 50, 0, "Test", "", "", "", "", interleaved)
}
