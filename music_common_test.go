// music_common_test.go - Tests for shared music utilities

package main

import (
	"testing"
)

func TestWriteUint32Byte(t *testing.T) {
	tests := []struct {
		name      string
		current   uint32
		value     uint32
		byteIndex uint32
		expected  uint32
	}{
		{"write byte 0", 0x00000000, 0xAB, 0, 0x000000AB},
		{"write byte 1", 0x00000000, 0xCD, 1, 0x0000CD00},
		{"write byte 2", 0x00000000, 0xEF, 2, 0x00EF0000},
		{"write byte 3", 0x00000000, 0x12, 3, 0x12000000},
		{"overwrite byte 0", 0xFFFFFFFF, 0x00, 0, 0xFFFFFF00},
		{"overwrite byte 1", 0xFFFFFFFF, 0x00, 1, 0xFFFF00FF},
		{"overwrite byte 2", 0xFFFFFFFF, 0x00, 2, 0xFF00FFFF},
		{"overwrite byte 3", 0xFFFFFFFF, 0x00, 3, 0x00FFFFFF},
		{"preserve other bytes", 0x12345678, 0xAB, 1, 0x1234AB78},
		{"value masked to byte", 0x00000000, 0x1234, 0, 0x00000034},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := writeUint32Byte(tt.current, tt.value, tt.byteIndex)
			if result != tt.expected {
				t.Errorf("writeUint32Byte(0x%08X, 0x%X, %d) = 0x%08X, want 0x%08X",
					tt.current, tt.value, tt.byteIndex, result, tt.expected)
			}
		})
	}
}

func TestWriteUint32Word(t *testing.T) {
	tests := []struct {
		name      string
		current   uint32
		value     uint32
		byteIndex uint32
		expected  uint32
	}{
		{"write word at 0", 0x00000000, 0xABCD, 0, 0x0000ABCD},
		{"write word at 2", 0x00000000, 0x1234, 2, 0x12340000},
		{"write single byte", 0x00000000, 0x00AB, 0, 0x000000AB},
		{"overwrite word", 0xFFFFFFFF, 0x0000, 0, 0xFFFFFF00}, // Only writes byte 0 when value <= 0xFF
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := writeUint32Word(tt.current, tt.value, tt.byteIndex)
			if result != tt.expected {
				t.Errorf("writeUint32Word(0x%08X, 0x%X, %d) = 0x%08X, want 0x%08X",
					tt.current, tt.value, tt.byteIndex, result, tt.expected)
			}
		})
	}
}

func TestReadUint32Byte(t *testing.T) {
	tests := []struct {
		name      string
		value     uint32
		byteIndex uint32
		expected  uint32
	}{
		{"read byte 0", 0x12345678, 0, 0x78},
		{"read byte 1", 0x12345678, 1, 0x56},
		{"read byte 2", 0x12345678, 2, 0x34},
		{"read byte 3", 0x12345678, 3, 0x12},
		{"read all FF", 0xFFFFFFFF, 0, 0xFF},
		{"read all 00", 0x00000000, 0, 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := readUint32Byte(tt.value, tt.byteIndex)
			if result != tt.expected {
				t.Errorf("readUint32Byte(0x%08X, %d) = 0x%02X, want 0x%02X",
					tt.value, tt.byteIndex, result, tt.expected)
			}
		})
	}
}

func TestParseNullTerminatedString(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		offset         int
		expectedString string
		expectedOffset int
	}{
		{
			"simple string",
			[]byte("Hello\x00World"),
			0,
			"Hello",
			6,
		},
		{
			"string with offset",
			[]byte("Hello\x00World\x00"),
			6,
			"World",
			12,
		},
		{
			"empty string",
			[]byte("\x00"),
			0,
			"",
			1,
		},
		{
			"no null terminator",
			[]byte("Test"),
			0,
			"Test",
			4,
		},
		{
			"offset at end",
			[]byte("Test\x00"),
			5,
			"",
			5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, offset := parseNullTerminatedString(tt.data, tt.offset)
			if result != tt.expectedString {
				t.Errorf("parseNullTerminatedString() string = %q, want %q", result, tt.expectedString)
			}
			if offset != tt.expectedOffset {
				t.Errorf("parseNullTerminatedString() offset = %d, want %d", offset, tt.expectedOffset)
			}
		})
	}
}

func TestParsePaddedString(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			"null terminated",
			[]byte("Hello\x00\x00\x00"),
			"Hello",
		},
		{
			"space padded",
			[]byte("Hello   "),
			"Hello",
		},
		{
			"null and space padded",
			[]byte("Hello   \x00  "),
			"Hello",
		},
		{
			"empty with nulls",
			[]byte("\x00\x00\x00"),
			"",
		},
		{
			"empty with spaces",
			[]byte("   "),
			"",
		},
		{
			"no padding needed",
			[]byte("Hello"),
			"Hello",
		},
		{
			"internal spaces preserved",
			[]byte("Hello World\x00  "),
			"Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePaddedString(tt.data)
			if result != tt.expected {
				t.Errorf("parsePaddedString(%q) = %q, want %q", tt.data, result, tt.expected)
			}
		})
	}
}

func TestPlayerControlState_PlayStatus(t *testing.T) {
	tests := []struct {
		name          string
		busy          bool
		err           bool
		enginePlaying bool
		expected      uint32
	}{
		{"idle", false, false, false, 0},
		{"busy from state", true, false, false, 1},
		{"busy from engine", false, false, true, 1},
		{"error only", false, true, false, 2},
		{"busy and error", true, true, false, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PlayerControlState{
				PlayBusy: tt.busy,
				PlayErr:  tt.err,
			}
			result := s.PlayStatus(tt.enginePlaying)
			if result != tt.expected {
				t.Errorf("PlayStatus() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestPlayerControlState_HandlePtrWrite(t *testing.T) {
	s := &PlayerControlState{}

	// Write full value at offset 0
	s.HandlePtrWrite(0, 0x12345678)
	if s.PlayPtrStaged != 0x12345678 {
		t.Errorf("HandlePtrWrite(0) = 0x%08X, want 0x12345678", s.PlayPtrStaged)
	}

	// Write byte at offset 1
	s.PlayPtrStaged = 0
	s.HandlePtrWrite(1, 0xAB)
	if s.PlayPtrStaged != 0x0000AB00 {
		t.Errorf("HandlePtrWrite(1) = 0x%08X, want 0x0000AB00", s.PlayPtrStaged)
	}
}

func TestPlayerControlState_ReadPtrByte(t *testing.T) {
	s := &PlayerControlState{PlayPtrStaged: 0x12345678}

	if v := s.ReadPtrByte(0); v != 0x12345678 {
		t.Errorf("ReadPtrByte(0) = 0x%08X, want 0x12345678", v)
	}
	if v := s.ReadPtrByte(1); v != 0x56 {
		t.Errorf("ReadPtrByte(1) = 0x%02X, want 0x56", v)
	}
	if v := s.ReadPtrByte(2); v != 0x34 {
		t.Errorf("ReadPtrByte(2) = 0x%02X, want 0x34", v)
	}
	if v := s.ReadPtrByte(3); v != 0x12 {
		t.Errorf("ReadPtrByte(3) = 0x%02X, want 0x12", v)
	}
}
