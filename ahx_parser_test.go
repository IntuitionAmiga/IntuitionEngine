// ahx_parser_test.go - Tests for AHX module parsing

package main

import (
	"testing"
)

// TestParseAHXHeader_AHX0 tests parsing a valid AHX0 (THX\0) header
func TestParseAHXHeader_AHX0(t *testing.T) {
	// Minimal AHX0 header: THX\0 + offset + flags/len + restart + tracklen + trackNr + smpNr + ssNr
	// Bytes 0-3: THX\0
	// Bytes 4-5: Name offset (big-endian)
	// Bytes 6-7: Flags(7-4) + PositionNr(11-0)
	// Bytes 8-9: Restart
	// Byte 10: TrackLength
	// Byte 11: TrackNr
	// Byte 12: InstrumentNr
	// Byte 13: SubsongNr
	// Then: positions, tracks, instruments, names

	// Calculate offsets:
	// Header: 14 bytes
	// Subsongs: 0
	// Positions: 1 * 8 = 8 bytes
	// Tracks: 1 track (track 0 in file) * 1 row * 3 = 3 bytes
	// Name offset = 14 + 8 + 3 = 25 = 0x19
	data := []byte{
		'T', 'H', 'X', 0x00, // Magic: AHX0
		0x00, 0x19, // Name offset = 25
		0x00, 0x01, // bit 7=0 (track 0 in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, // TrackLength = 1
		0x00, // TrackNr = 0 (no additional tracks)
		0x00, // InstrumentNr = 0
		0x00, // SubsongNr = 0
		// Position list: 1 entry = 8 bytes
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (1 row = 3 bytes, bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Song name (null-terminated)
		'T', 'e', 's', 't', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	if song.Revision != 0 {
		t.Errorf("Expected Revision 0, got %d", song.Revision)
	}
	if song.Name != "Test" {
		t.Errorf("Expected Name 'Test', got '%s'", song.Name)
	}
	if song.PositionNr != 1 {
		t.Errorf("Expected PositionNr 1, got %d", song.PositionNr)
	}
	if song.TrackLength != 1 {
		t.Errorf("Expected TrackLength 1, got %d", song.TrackLength)
	}
}

// TestParseAHXHeader_AHX1 tests parsing a valid AHX1 (THX\1) header
func TestParseAHXHeader_AHX1(t *testing.T) {
	// Name offset = 14 + 8 + 3 = 25 = 0x19
	data := []byte{
		'T', 'H', 'X', 0x01, // Magic: AHX1
		0x00, 0x19, // Name offset = 25
		0x00, 0x01, // bit 7=0 (track 0 in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, // TrackLength = 1
		0x00, // TrackNr = 0
		0x00, // InstrumentNr = 0
		0x00, // SubsongNr = 0
		// Position list: 1 entry = 8 bytes
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Song name
		'A', 'H', 'X', '1', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	if song.Revision != 1 {
		t.Errorf("Expected Revision 1, got %d", song.Revision)
	}
}

// TestParseAHXHeader_InvalidMagic tests rejection of invalid magic bytes
func TestParseAHXHeader_InvalidMagic(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"Wrong magic", []byte{'M', 'O', 'D', 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}},
		{"THX with revision 2", []byte{'T', 'H', 'X', 0x02, 0x00, 0x0E, 0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}},
		{"Too short", []byte{'T', 'H', 'X', 0x00}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAHX(tc.data)
			if err == nil {
				t.Error("Expected error for invalid magic, got nil")
			}
		})
	}
}

// TestParseAHXHeader_SpeedMultiplier tests the speed multiplier extraction from byte 6
func TestParseAHXHeader_SpeedMultiplier(t *testing.T) {
	// Speed multiplier is in bits 6-5 of byte 6 (3-bit field after mask & 7)
	// SPD=0 -> 50Hz (multiplier 1)
	// SPD=1 -> 100Hz (multiplier 2)
	// SPD=2 -> 150Hz (multiplier 3)
	// SPD=3 -> 200Hz (multiplier 4)

	testCases := []struct {
		name      string
		byte6     byte
		wantSpeed int
	}{
		{"50Hz", 0x80, 1},  // bits 6-5 = 00, track0 saved
		{"100Hz", 0xA0, 2}, // bits 6-5 = 01
		{"150Hz", 0xC0, 3}, // bits 6-5 = 10
		{"200Hz", 0xE0, 4}, // bits 6-5 = 11
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Name offset = 14 + 8 + 3 = 25 = 0x19
			data := []byte{
				'T', 'H', 'X', 0x01, // AHX1 required for speed multiplier
				0x00, 0x19,
				tc.byte6, 0x01, // Flags + PositionNr
				0x00, 0x00,
				0x01, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00,
				'T', 0x00,
			}

			song, err := ParseAHX(data)
			if err != nil {
				t.Fatalf("ParseAHX failed: %v", err)
			}

			if song.SpeedMultiplier != tc.wantSpeed {
				t.Errorf("Expected SpeedMultiplier %d, got %d", tc.wantSpeed, song.SpeedMultiplier)
			}
		})
	}
}

// TestParseAHXHeader_TrackZeroFlag tests the track 0 saved flag
func TestParseAHXHeader_TrackZeroFlag(t *testing.T) {
	// Bit 7 of byte 6 (per C++ reference behavior):
	// 0 = track 0 IS saved in file (read it)
	// 1 = track 0 NOT saved in file (skip, use zeros)
	// Note: The spec text says the opposite, but C++ code behavior is authoritative

	// Track 0 saved in file (bit 7 = 0)
	dataSaved := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x11, // Name offset = 17
		0x00, 0x01, // bit 7 = 0 (track 0 IS in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, 0x00, 0x00, 0x00, // TrackLength=1, TrackNr=0, InstrumentNr=0, SubsongNr=0
		// Position (8 bytes)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (1 row * 3 bytes) - saved in file
		0x00, 0x00, 0x00,
		'T', 0x00,
	}

	song, err := ParseAHX(dataSaved)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}
	if len(song.Tracks) < 1 || len(song.Tracks[0]) != 1 {
		t.Errorf("Expected track 0 to be present with 1 row")
	}

	// Track 0 not saved in file (bit 7 = 1) - track 0 should be auto-generated as empty
	dataNotSaved := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x0E, // Name offset = 14
		0x80, 0x01, // bit 7 = 1 (track 0 NOT in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, 0x00, 0x00, 0x00, // TrackLength=1, TrackNr=0, InstrumentNr=0, SubsongNr=0
		// Position (8 bytes)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// No track 0 data in file (skipped)
		'T', 0x00,
	}

	song2, err := ParseAHX(dataNotSaved)
	if err != nil {
		t.Fatalf("ParseAHX failed for track0 not saved: %v", err)
	}
	// Track 0 should still exist in memory, but be empty
	if len(song2.Tracks) < 1 {
		t.Error("Expected track 0 to be auto-generated")
	}
	// All steps in track 0 should be zeroed
	for i, step := range song2.Tracks[0] {
		if step.Note != 0 || step.Instrument != 0 || step.FX != 0 || step.FXParam != 0 {
			t.Errorf("Track 0 step %d should be empty, got %+v", i, step)
		}
	}
}

// TestParseAHXPositions tests position list parsing
func TestParseAHXPositions(t *testing.T) {
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x22, // Name offset
		0x00, 0x02, // bit 7=0 (track 0 in file), PositionNr = 2
		0x00, 0x00, // Restart = 0
		0x01, // TrackLength = 1
		0x01, // TrackNr = 1 (track 0 + track 1)
		0x00, // InstrumentNr = 0
		0x00, // SubsongNr = 0
		// Position 0: tracks [0,1,0,1], transposes [0,12,-12,5]
		0x00, 0x00, // track 0, transpose 0
		0x01, 0x0C, // track 1, transpose 12
		0x00, 0xF4, // track 0, transpose -12 (0xF4 = -12 signed)
		0x01, 0x05, // track 1, transpose 5
		// Position 1: all track 0, all transpose 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (1 row, bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Track 1 data (1 row)
		0x00, 0x00, 0x00,
		// Song name
		'P', 'o', 's', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	if len(song.Positions) != 2 {
		t.Fatalf("Expected 2 positions, got %d", len(song.Positions))
	}

	// Check position 0
	pos := song.Positions[0]
	expectedTracks := [4]int{0, 1, 0, 1}
	expectedTranspose := [4]int8{0, 12, -12, 5}

	for i := range 4 {
		if pos.Track[i] != expectedTracks[i] {
			t.Errorf("Position 0 track[%d]: expected %d, got %d", i, expectedTracks[i], pos.Track[i])
		}
		if pos.Transpose[i] != expectedTranspose[i] {
			t.Errorf("Position 0 transpose[%d]: expected %d, got %d", i, expectedTranspose[i], pos.Transpose[i])
		}
	}
}

// TestParseAHXTracks_UnpackRow tests the 3-byte track row unpacking
func TestParseAHXTracks_UnpackRow(t *testing.T) {
	// Track row format (24 bits / 3 bytes):
	// bits 23-18 (6 bits): Note (0-60)
	// bits 17-12 (6 bits): Instrument (0-63)
	// bits 11-8  (4 bits): FX command (0-15)
	// bits 7-0   (8 bits): FX parameter (0-255)

	// Track row format (24 bits):
	// byte 0 bits 7-2 = note (6 bits)
	// byte 0 bits 1-0 = instrument high (2 bits)
	// byte 1 bits 7-4 = instrument low (4 bits)
	// byte 1 bits 3-0 = fx (4 bits)
	// byte 2 = fx param (8 bits)
	testCases := []struct {
		name     string
		bytes    [3]byte
		wantNote int
		wantInst int
		wantFX   int
		wantFXP  int
	}{
		{
			name:     "C-1, inst 1, no fx",
			bytes:    [3]byte{0x04, 0x10, 0x00}, // note=1, inst=1, fx=0, param=0
			wantNote: 1,
			wantInst: 1,
			wantFX:   0,
			wantFXP:  0,
		},
		{
			name: "A-4 (note 46), inst 63, fx F, param C0",
			// note=46 -> 46<<2 = 0xB8, inst=63 -> (3<<0)|(15<<4) -> byte0&3=3, byte1>>4=15
			// So byte0 = 0xB8 | 0x03 = 0xBB, byte1 = 0xFF (inst_low=15, fx=15)
			bytes:    [3]byte{0xBB, 0xFF, 0xC0},
			wantNote: 46,
			wantInst: 63,
			wantFX:   15,
			wantFXP:  192,
		},
		{
			name: "B-5 (note 60), inst 8, fx 5, param 20",
			// note=60 -> 60<<2 = 0xF0, inst=8 -> (0<<0)|(8<<4) -> byte0&3=0, byte1>>4=8
			// byte0 = 0xF0, byte1 = 0x85 (inst_low=8, fx=5)
			bytes:    [3]byte{0xF0, 0x85, 0x20},
			wantNote: 60,
			wantInst: 8,
			wantFX:   5,
			wantFXP:  0x20,
		},
		{
			name:     "Empty row",
			bytes:    [3]byte{0x00, 0x00, 0x00},
			wantNote: 0,
			wantInst: 0,
			wantFX:   0,
			wantFXP:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			step := unpackAHXStep(tc.bytes[:])

			if step.Note != tc.wantNote {
				t.Errorf("Note: expected %d, got %d", tc.wantNote, step.Note)
			}
			if step.Instrument != tc.wantInst {
				t.Errorf("Instrument: expected %d, got %d", tc.wantInst, step.Instrument)
			}
			if step.FX != tc.wantFX {
				t.Errorf("FX: expected %d, got %d", tc.wantFX, step.FX)
			}
			if step.FXParam != tc.wantFXP {
				t.Errorf("FXParam: expected %d, got %d", tc.wantFXP, step.FXParam)
			}
		})
	}
}

// TestParseAHXInstrument_Envelope tests instrument envelope parsing
func TestParseAHXInstrument_Envelope(t *testing.T) {
	// Instrument format (22 bytes + playlist):
	// byte 0: Volume (0-64)
	// byte 1: bits 7-3 = filter speed bits 4-0, bits 2-0 = wavelength (0-5)
	// byte 2: attack frames
	// byte 3: attack volume
	// byte 4: decay frames
	// byte 5: decay volume
	// byte 6: sustain frames
	// byte 7: release frames
	// byte 8: release volume
	// ... more fields
	// byte 20: playlist speed
	// byte 21: playlist length

	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x32, // Name offset
		0x00, 0x01, // bit 7=0 (track 0 in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, // TrackLength = 1
		0x00, // TrackNr = 0
		0x01, // InstrumentNr = 1
		0x00, // SubsongNr = 0
		// Position
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Instrument 1 (22 bytes + playlist)
		0x40,             // Volume = 64
		0x03,             // FilterSpeed bits + WaveLength = 3 (0x20)
		0x10,             // Attack frames = 16
		0x40,             // Attack volume = 64
		0x20,             // Decay frames = 32
		0x30,             // Decay volume = 48
		0x40,             // Sustain frames = 64
		0x08,             // Release frames = 8
		0x00,             // Release volume = 0
		0x00, 0x00, 0x00, // Unused
		0x00, // FilterLowerLimit
		0x00, // VibratoDelay
		0x00, // HardCut + VibratoDepth
		0x00, // VibratoSpeed
		0x00, // SquareLowerLimit
		0x3F, // SquareUpperLimit = 63
		0x00, // SquareSpeed
		0x00, // FilterUpperLimit
		0x06, // Playlist speed = 6
		0x00, // Playlist length = 0
		// Song name
		'S', 'o', 'n', 'g', 0x00,
		// Instrument 1 name
		'I', 'n', 's', 't', '1', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	if len(song.Instruments) < 2 {
		t.Fatalf("Expected at least 2 instruments (0 + 1), got %d", len(song.Instruments))
	}

	inst := song.Instruments[1]
	if inst.Volume != 64 {
		t.Errorf("Volume: expected 64, got %d", inst.Volume)
	}
	if inst.WaveLength != 3 {
		t.Errorf("WaveLength: expected 3, got %d", inst.WaveLength)
	}
	if inst.Envelope.AFrames != 16 {
		t.Errorf("Attack frames: expected 16, got %d", inst.Envelope.AFrames)
	}
	if inst.Envelope.AVolume != 64 {
		t.Errorf("Attack volume: expected 64, got %d", inst.Envelope.AVolume)
	}
	if inst.Envelope.DFrames != 32 {
		t.Errorf("Decay frames: expected 32, got %d", inst.Envelope.DFrames)
	}
	if inst.Envelope.DVolume != 48 {
		t.Errorf("Decay volume: expected 48, got %d", inst.Envelope.DVolume)
	}
	if inst.Envelope.SFrames != 64 {
		t.Errorf("Sustain frames: expected 64, got %d", inst.Envelope.SFrames)
	}
	if inst.Envelope.RFrames != 8 {
		t.Errorf("Release frames: expected 8, got %d", inst.Envelope.RFrames)
	}
	if inst.Envelope.RVolume != 0 {
		t.Errorf("Release volume: expected 0, got %d", inst.Envelope.RVolume)
	}
	if inst.Name != "Inst1" {
		t.Errorf("Instrument name: expected 'Inst1', got '%s'", inst.Name)
	}
}

// TestParseAHXInstrument_Playlist tests instrument playlist parsing
func TestParseAHXInstrument_Playlist(t *testing.T) {
	// Playlist entry format (4 bytes / 32 bits):
	// bits 31-29: FX2 command (0-7)
	// bits 28-26: FX1 command (0-7)
	// bits 25-23: Waveform (0-7)
	// bit 22: Fixed note flag
	// bits 21-16: Note (0-60)
	// bits 15-8: FX1 parameter
	// bits 7-0: FX2 parameter

	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x3A, // Name offset
		0x00, 0x01, // bit 7=0 (track 0 in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, 0x00, 0x01, 0x00, // TrackLength=1, TrackNr=0, InstrumentNr=1, SubsongNr=0
		// Position
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Instrument 1
		0x40, 0x03, 0x10, 0x40, 0x20, 0x30, 0x40, 0x08, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3F, 0x00, 0x00,
		0x06, // Playlist speed = 6
		0x02, // Playlist length = 2
		// Playlist entry 0: FX2=1, FX1=2, Waveform=3, Fixed=1, Note=24, FX1P=0x10, FX2P=0x20
		// bits: 001 010 011 1 011000 = 0x29 0xD8
		// Then FX1P, FX2P
		0x29, 0xD8, 0x10, 0x20,
		// Playlist entry 1: FX2=0, FX1=0, Waveform=1, Fixed=0, Note=36, FX1P=0, FX2P=0
		// bits: 000 000 001 0 100100 = 0x00 0xA4
		0x00, 0xA4, 0x00, 0x00,
		// Song name
		'P', 'L', 0x00,
		// Instrument 1 name
		'I', '1', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	inst := song.Instruments[1]
	if inst.PList.Speed != 6 {
		t.Errorf("PList speed: expected 6, got %d", inst.PList.Speed)
	}
	if inst.PList.Length != 2 {
		t.Errorf("PList length: expected 2, got %d", inst.PList.Length)
	}
	if len(inst.PList.Entries) != 2 {
		t.Fatalf("Expected 2 playlist entries, got %d", len(inst.PList.Entries))
	}

	// Check entry 0
	e0 := inst.PList.Entries[0]
	if e0.FX[1] != 1 {
		t.Errorf("Entry 0 FX2: expected 1, got %d", e0.FX[1])
	}
	if e0.FX[0] != 2 {
		t.Errorf("Entry 0 FX1: expected 2, got %d", e0.FX[0])
	}
	if e0.Waveform != 3 {
		t.Errorf("Entry 0 Waveform: expected 3, got %d", e0.Waveform)
	}
	if e0.Fixed != 1 {
		t.Errorf("Entry 0 Fixed: expected 1, got %d", e0.Fixed)
	}
	if e0.Note != 24 {
		t.Errorf("Entry 0 Note: expected 24, got %d", e0.Note)
	}
	if e0.FXParam[0] != 0x10 {
		t.Errorf("Entry 0 FX1Param: expected 0x10, got 0x%02X", e0.FXParam[0])
	}
	if e0.FXParam[1] != 0x20 {
		t.Errorf("Entry 0 FX2Param: expected 0x20, got 0x%02X", e0.FXParam[1])
	}

	// Check entry 1
	e1 := inst.PList.Entries[1]
	if e1.Waveform != 1 {
		t.Errorf("Entry 1 Waveform: expected 1, got %d", e1.Waveform)
	}
	if e1.Fixed != 0 {
		t.Errorf("Entry 1 Fixed: expected 0, got %d", e1.Fixed)
	}
	if e1.Note != 36 {
		t.Errorf("Entry 1 Note: expected 36, got %d", e1.Note)
	}
}

// TestParseAHXNames tests song and instrument name parsing
func TestParseAHXNames(t *testing.T) {
	// Calculate offset:
	// Header: 14 bytes
	// Positions: 1 * 8 = 8 bytes
	// Tracks: 1 * 1 * 3 = 3 bytes
	// Instrument 1: 22 bytes
	// Name offset = 14 + 8 + 3 + 22 = 47 = 0x2F
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x2F, // Name offset = 47
		0x00, 0x01, // bit 7=0 (track 0 in file), PositionNr = 1
		0x00, 0x00, // Restart = 0
		0x01, 0x00, 0x01, 0x00, // TrackLength=1, TrackNr=0, InstrumentNr=1, SubsongNr=0
		// Position
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Instrument 1 (22 bytes, no playlist)
		0x40, 0x03, 0x10, 0x40, 0x20, 0x30, 0x40, 0x08, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3F, 0x00, 0x00,
		0x06, 0x00, // Playlist speed = 6, length = 0
		// Song name at offset 0x2F
		'M', 'y', ' ', 'S', 'o', 'n', 'g', 0x00,
		// Instrument 1 name
		'L', 'e', 'a', 'd', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	if song.Name != "My Song" {
		t.Errorf("Song name: expected 'My Song', got '%s'", song.Name)
	}
	if song.Instruments[1].Name != "Lead" {
		t.Errorf("Instrument 1 name: expected 'Lead', got '%s'", song.Instruments[1].Name)
	}
}

// TestParseAHXSubsongs tests subsong list parsing
func TestParseAHXSubsongs(t *testing.T) {
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x18, // Name offset
		0x00, 0x02, // bit 7=0 (track 0 in file), PositionNr = 2
		0x00, 0x01, // Restart = 1
		0x01, // TrackLength = 1
		0x00, // TrackNr = 0
		0x00, // InstrumentNr = 0
		0x02, // SubsongNr = 2
		// Subsong list (2 entries, 2 bytes each)
		0x00, 0x00, // Subsong 0 starts at position 0
		0x00, 0x01, // Subsong 1 starts at position 1
		// Position 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Position 1
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 data (bit 7=0 means track 0 IS in file)
		0x00, 0x00, 0x00,
		// Song name
		'S', 'u', 'b', 0x00,
	}

	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}

	if song.SubsongNr != 2 {
		t.Errorf("SubsongNr: expected 2, got %d", song.SubsongNr)
	}
	if len(song.Subsongs) != 2 {
		t.Fatalf("Expected 2 subsongs, got %d", len(song.Subsongs))
	}
	if song.Subsongs[0] != 0 {
		t.Errorf("Subsong 0: expected 0, got %d", song.Subsongs[0])
	}
	if song.Subsongs[1] != 1 {
		t.Errorf("Subsong 1: expected 1, got %d", song.Subsongs[1])
	}
	if song.Restart != 1 {
		t.Errorf("Restart: expected 1, got %d", song.Restart)
	}
}
