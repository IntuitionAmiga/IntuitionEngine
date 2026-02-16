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
	module := make([]byte, 0x22)
	module[0x00] = 1

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
	module := make([]byte, 0x30)
	module[0x01] = 3
	module[0x02] = 1

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
	module := make([]byte, 0x20)
	copy(module[0:4], "FTC!")
	module[0x04] = 1
	module[0x06] = 3

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
	compressed := testCompressLH5(interleaved)

	// Build VTX header (uint16 chipFreq variant, 12-byte header)
	header := make([]byte, 0, 64)
	header = append(header, 'a', 'y')   // chip ID
	header = append(header, 0)          // stereo: mono
	header = append(header, 0xF4, 0x06) // chipFreq: 1780 (×1000 = 1780000 Hz) LE uint16
	header = append(header, 50)         // player freq
	header = append(header, 0, 0)       // year: 0
	origSize := uint32(len(interleaved))
	header = append(header, byte(origSize), byte(origSize>>8), byte(origSize>>16), byte(origSize>>24))
	// Null-terminated strings: title, author, from, tracker, comment
	header = append(header, 'T', 'e', 's', 't', 0)
	header = append(header, 0) // author
	header = append(header, 0) // from
	header = append(header, 0) // tracker
	header = append(header, 0) // comment
	header = append(header, compressed...)

	return header
}
