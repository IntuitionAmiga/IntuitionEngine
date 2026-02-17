package main

import (
	"encoding/binary"
	"testing"
)

// buildVTXFile constructs a valid VTX binary for testing.
// Fixed 16-byte header: magic(2) + stereo(1) + loop(2) + chipFreq(4) + playerFreq(1) + year(2) + dataSize(4).
func buildVTXFile(chipType string, stereo uint8, chipFreq uint32, playerFreq uint8, year uint16,
	title, author, from, tracker, comment string, frameData []byte) []byte {
	return buildVTXFileWithLoop(chipType, stereo, 0, chipFreq, playerFreq, year, title, author, from, tracker, comment, frameData)
}

func buildVTXFileWithLoop(chipType string, stereo uint8, loop uint16, chipFreq uint32, playerFreq uint8, year uint16,
	title, author, from, tracker, comment string, frameData []byte) []byte {

	// Fixed header: 2 + 1 + 2 + 4 + 1 + 2 + 4 = 16 bytes
	buf := make([]byte, 0, 256)
	buf = append(buf, chipType[0], chipType[1]) // magic
	buf = append(buf, stereo)                   // stereo type

	var lp [2]byte
	binary.LittleEndian.PutUint16(lp[:], loop)
	buf = append(buf, lp[:]...) // loop frame

	var freq [4]byte
	binary.LittleEndian.PutUint32(freq[:], chipFreq)
	buf = append(buf, freq[:]...)

	buf = append(buf, playerFreq) // player freq

	var yr [2]byte
	binary.LittleEndian.PutUint16(yr[:], year)
	buf = append(buf, yr[:]...)

	// Compress frameData
	compressed := testCompressLH5(frameData)

	var dsz [4]byte
	binary.LittleEndian.PutUint32(dsz[:], uint32(len(frameData)))
	buf = append(buf, dsz[:]...)

	// 5 null-terminated strings
	buf = append(buf, []byte(title)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(author)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(from)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(tracker)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(comment)...)
	buf = append(buf, 0)

	// LH5 compressed data
	buf = append(buf, compressed...)
	return buf
}

// makeInterleavedFrames creates interleaved YM3 register data.
// Returns regCount * frameCount bytes: all values for reg 0, then reg 1, etc.
func makeInterleavedFrames(frameCount, regCount int, fill func(reg, frame int) byte) []byte {
	data := make([]byte, regCount*frameCount)
	for reg := range regCount {
		for frame := range frameCount {
			data[reg*frameCount+frame] = fill(reg, frame)
		}
	}
	return data
}

func TestParseVTXHeader_AYChip(t *testing.T) {
	frames := makeInterleavedFrames(1, 14, func(reg, frame int) byte { return 0 })
	vtx := buildVTXFile("ay", 0, 1773400, 50, 1997, "Test", "Author", "", "", "", frames)

	ym, meta, err := ParseVTXData(vtx)
	if err != nil {
		t.Fatalf("ParseVTXData error: %v", err)
	}
	if meta.Title != "Test" {
		t.Errorf("title = %q, want %q", meta.Title, "Test")
	}
	if meta.Author != "Author" {
		t.Errorf("author = %q, want %q", meta.Author, "Author")
	}
	if meta.System != "ZX Spectrum" {
		t.Errorf("system = %q, want %q", meta.System, "ZX Spectrum")
	}
	if ym.ClockHz != 1773400 {
		t.Errorf("clockHz = %d, want 1773400", ym.ClockHz)
	}
	if ym.FrameRate != 50 {
		t.Errorf("frameRate = %d, want 50", ym.FrameRate)
	}
}

func TestParseVTXHeader_YMChip(t *testing.T) {
	frames := makeInterleavedFrames(1, 14, func(reg, frame int) byte { return 0 })
	vtx := buildVTXFile("ym", 0, 2000000, 50, 2001, "YM Test", "YM Author", "", "", "", frames)

	ym, meta, err := ParseVTXData(vtx)
	if err != nil {
		t.Fatalf("ParseVTXData error: %v", err)
	}
	if meta.System != "Atari ST" {
		t.Errorf("system = %q, want %q", meta.System, "Atari ST")
	}
	if ym.ClockHz != 2000000 {
		t.Errorf("clockHz = %d, want 2000000", ym.ClockHz)
	}
}

func TestParseVTXHeader_InvalidMagic(t *testing.T) {
	data := []byte("xx\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	_, _, err := ParseVTXData(data)
	if err == nil {
		t.Error("expected error for invalid magic")
	}
}

func TestParseVTXHeader_TooShort(t *testing.T) {
	data := []byte("ay\x00")
	_, _, err := ParseVTXData(data)
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestParseVTXHeader_AllStereoTypes(t *testing.T) {
	frames := makeInterleavedFrames(1, 14, func(reg, frame int) byte { return 0 })
	for stereo := uint8(0); stereo <= 6; stereo++ {
		vtx := buildVTXFile("ay", stereo, 1773400, 50, 2000, "", "", "", "", "", frames)
		_, _, err := ParseVTXData(vtx)
		if err != nil {
			t.Errorf("stereo=%d: unexpected error: %v", stereo, err)
		}
	}

	// Invalid stereo type (7+)
	vtx := buildVTXFile("ay", 7, 1773400, 50, 2000, "", "", "", "", "", frames)
	// Manually patch stereo byte since buildVTXFile doesn't validate
	vtx[2] = 7
	_, _, err := ParseVTXData(vtx)
	if err == nil {
		t.Error("expected error for stereo type 7")
	}
}

func TestParseVTXData_DecompressAndFrames(t *testing.T) {
	// 3 frames of 14 registers with known values
	frameCount := 3
	frameData := makeInterleavedFrames(frameCount, 14, func(reg, frame int) byte {
		return byte(reg*16 + frame)
	})

	vtx := buildVTXFile("ay", 0, 1773400, 50, 2000, "Test", "Auth", "", "", "", frameData)

	ym, _, err := ParseVTXData(vtx)
	if err != nil {
		t.Fatalf("ParseVTXData error: %v", err)
	}

	if len(ym.Frames) != frameCount {
		t.Fatalf("frame count = %d, want %d", len(ym.Frames), frameCount)
	}

	// Verify register values
	for frame := range frameCount {
		for reg := range 14 {
			expected := byte(reg*16 + frame)
			got := ym.Frames[frame][reg]
			if got != expected {
				t.Errorf("frame[%d][%d] = %d, want %d", frame, reg, got, expected)
			}
		}
	}
}

func TestParseVTXData_MetadataStrings(t *testing.T) {
	frames := makeInterleavedFrames(1, 14, func(reg, frame int) byte { return 0 })
	vtx := buildVTXFile("ay", 0, 1773400, 50, 2005,
		"My Song", "John Doe", "Demo Scene", "Vortex Tracker II", "A test comment",
		frames)

	_, meta, err := ParseVTXData(vtx)
	if err != nil {
		t.Fatalf("ParseVTXData error: %v", err)
	}
	if meta.Title != "My Song" {
		t.Errorf("title = %q, want %q", meta.Title, "My Song")
	}
	if meta.Author != "John Doe" {
		t.Errorf("author = %q, want %q", meta.Author, "John Doe")
	}
}

func TestParseVTXData_EmptyStrings(t *testing.T) {
	frames := makeInterleavedFrames(1, 14, func(reg, frame int) byte { return 0 })
	vtx := buildVTXFile("ay", 0, 1773400, 50, 2000, "", "", "", "", "", frames)

	_, meta, err := ParseVTXData(vtx)
	if err != nil {
		t.Fatalf("ParseVTXData error: %v", err)
	}
	if meta.Title != "" {
		t.Errorf("title = %q, want empty", meta.Title)
	}
	if meta.Author != "" {
		t.Errorf("author = %q, want empty", meta.Author)
	}
}

func TestParseVTXHeader_RealAliceVTX(t *testing.T) {
	// Real header bytes from alice.vtx: ym 01 00 00 58 0f 1b 00 32 d5 07 00 fc 00 00
	header := []byte{
		'y', 'm', // magic
		0x01,       // stereo=1 (ABC)
		0x00, 0x00, // loop=0
		0x58, 0x0f, 0x1b, 0x00, // chipFreq=1773400 (0x001B0F58) LE
		0x32,       // playerFreq=50
		0xd5, 0x07, // year=2005
		0x00, 0xfc, 0x00, 0x00, // dataSize=64512 (0x0000FC00) LE
	}
	// Add 5 null-terminated strings
	header = append(header, '"', 'A', 'L', 'I', 'C', 'E', '"', 0)      // title
	header = append(header, 'M', '.', 'F', 'A', 'R', 'M', 'E', 'R', 0) // author
	header = append(header, 0)                                         // from
	header = append(header, 0)                                         // tracker
	header = append(header, 0)                                         // comment

	h, _, err := parseVTXHeader(header)
	if err != nil {
		t.Fatalf("parseVTXHeader error: %v", err)
	}
	if h.ChipType != "ym" {
		t.Errorf("chipType = %q, want %q", h.ChipType, "ym")
	}
	if h.Stereo != 1 {
		t.Errorf("stereo = %d, want 1", h.Stereo)
	}
	if h.LoopFrame != 0 {
		t.Errorf("loopFrame = %d, want 0", h.LoopFrame)
	}
	if h.ChipFreqHz != 1773400 {
		t.Errorf("chipFreqHz = %d, want 1773400", h.ChipFreqHz)
	}
	if h.PlayerFreq != 50 {
		t.Errorf("playerFreq = %d, want 50", h.PlayerFreq)
	}
	if h.Year != 2005 {
		t.Errorf("year = %d, want 2005", h.Year)
	}
	if h.DataSize != 64512 {
		t.Errorf("dataSize = %d, want 64512", h.DataSize)
	}
	if h.Title != "\"ALICE\"" {
		t.Errorf("title = %q, want %q", h.Title, "\"ALICE\"")
	}
	if h.Author != "M.FARMER" {
		t.Errorf("author = %q, want %q", h.Author, "M.FARMER")
	}
}

func TestParseVTXHeader_LoopField(t *testing.T) {
	frames := makeInterleavedFrames(10, 14, func(reg, frame int) byte { return 0 })
	vtx := buildVTXFileWithLoop("ay", 0, 100, 1773400, 50, 2020, "Loop Test", "", "", "", "", frames)

	ym, _, err := ParseVTXData(vtx)
	if err != nil {
		t.Fatalf("ParseVTXData error: %v", err)
	}
	if ym.LoopFrame != 100 {
		t.Errorf("loopFrame = %d, want 100", ym.LoopFrame)
	}
}

func TestIsVTXData_Valid(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"ay mono", append([]byte("ay\x00"), make([]byte, 20)...), true},
		{"ym stereo ABC", append([]byte("ym\x01"), make([]byte, 20)...), true},
		{"invalid magic", append([]byte("xx\x00"), make([]byte, 20)...), false},
		{"stereo 7", append([]byte("ay\x07"), make([]byte, 20)...), false},
		{"too short", []byte("ay\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), false}, // 15 bytes, need 16
		{"really short", []byte("ay"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVTXData(tt.data); got != tt.valid {
				t.Errorf("isVTXData = %v, want %v", got, tt.valid)
			}
		})
	}
}
