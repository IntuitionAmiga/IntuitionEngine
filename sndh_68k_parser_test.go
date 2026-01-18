//go:build headless

package main

import (
	"os"
	"testing"
)

func TestParseSNDHData_Goldrunner(t *testing.T) {
	data, err := os.ReadFile("/home/zayn/Music/sndh_lf/Hubbard_Rob/Goldrunner.sndh")
	if err != nil {
		t.Skipf("Test file not available: %v", err)
	}

	if !isSNDH(data) {
		t.Fatal("Expected SNDH data")
	}

	file, err := ParseSNDHData(data)
	if err != nil {
		t.Fatalf("ParseSNDHData failed: %v", err)
	}

	t.Logf("Title: %q", file.Header.Title)
	t.Logf("Composer: %q", file.Header.Composer)
	t.Logf("Ripper: %q", file.Header.Ripper)
	t.Logf("Converter: %q", file.Header.Converter)
	t.Logf("Year: %q", file.Header.Year)
	t.Logf("SubSongs: %d", file.Header.SubSongCount)
	t.Logf("DefaultSong: %d", file.Header.DefaultSong)
	t.Logf("Timer: %s @ %d Hz", file.Header.TimerType, file.Header.TimerFreq)
	t.Logf("Durations: %v", file.Header.Durations)
	t.Logf("InitOffset: 0x%X", file.InitOffset)
	t.Logf("ExitOffset: 0x%X", file.ExitOffset)
	t.Logf("PlayOffset: 0x%X", file.PlayOffset)
	t.Logf("CodeOffset: 0x%X", file.CodeOffset)
	t.Logf("DataLength: %d bytes", len(file.Data))

	// Verify expected values for Goldrunner
	if file.Header.Title != "Gold Runner" {
		t.Errorf("Title = %q, want %q", file.Header.Title, "Gold Runner")
	}
	if file.Header.Composer != "Rob Hubbard" {
		t.Errorf("Composer = %q, want %q", file.Header.Composer, "Rob Hubbard")
	}
	if file.Header.Year != "1987" {
		t.Errorf("Year = %q, want %q", file.Header.Year, "1987")
	}
	if file.Header.TimerFreq != 50 {
		t.Errorf("TimerFreq = %d, want 50", file.Header.TimerFreq)
	}
}

func TestParseSNDHData_BranchInstructions(t *testing.T) {
	data, err := os.ReadFile("/home/zayn/Music/sndh_lf/Hubbard_Rob/Goldrunner.sndh")
	if err != nil {
		t.Skipf("Test file not available: %v", err)
	}

	file, err := ParseSNDHData(data)
	if err != nil {
		t.Fatalf("ParseSNDHData failed: %v", err)
	}

	// Verify branch targets are within data range
	if file.InitOffset < 0 || file.InitOffset >= len(file.Data) {
		t.Errorf("InitOffset %d out of range [0, %d)", file.InitOffset, len(file.Data))
	}
	if file.PlayOffset < 0 || file.PlayOffset >= len(file.Data) {
		t.Errorf("PlayOffset %d out of range [0, %d)", file.PlayOffset, len(file.Data))
	}

	// Log first few bytes at each entry point for debugging
	if file.InitOffset > 0 && file.InitOffset+8 <= len(file.Data) {
		t.Logf("INIT code at 0x%X: %02X %02X %02X %02X %02X %02X %02X %02X",
			file.InitOffset,
			file.Data[file.InitOffset], file.Data[file.InitOffset+1],
			file.Data[file.InitOffset+2], file.Data[file.InitOffset+3],
			file.Data[file.InitOffset+4], file.Data[file.InitOffset+5],
			file.Data[file.InitOffset+6], file.Data[file.InitOffset+7])
	}
	if file.PlayOffset > 0 && file.PlayOffset+8 <= len(file.Data) {
		t.Logf("PLAY code at 0x%X: %02X %02X %02X %02X %02X %02X %02X %02X",
			file.PlayOffset,
			file.Data[file.PlayOffset], file.Data[file.PlayOffset+1],
			file.Data[file.PlayOffset+2], file.Data[file.PlayOffset+3],
			file.Data[file.PlayOffset+4], file.Data[file.PlayOffset+5],
			file.Data[file.PlayOffset+6], file.Data[file.PlayOffset+7])
	}
}

func TestIsSNDH(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid SNDH at offset 12",
			data: []byte{
				0x60, 0x00, 0x00, 0x10, // BRA.W to INIT
				0x60, 0x00, 0x00, 0x20, // BRA.W to EXIT
				0x60, 0x00, 0x00, 0x30, // BRA.W to PLAY
				'S', 'N', 'D', 'H', // Magic
			},
			want: true,
		},
		{
			name: "ICE packed",
			data: []byte{'I', 'C', 'E', '!', 0, 0, 0, 100, 0, 0, 1, 0},
			want: true,
		},
		{
			name: "not SNDH",
			data: []byte{'Z', 'X', 'A', 'Y', 'E', 'M', 'U', 'L'},
			want: false,
		},
		{
			name: "too short",
			data: []byte{'S', 'N', 'D'},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSNDH(tt.data); got != tt.want {
				t.Errorf("isSNDH() = %v, want %v", got, tt.want)
			}
		})
	}
}
