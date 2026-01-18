// sndh_68k_parser.go - SNDH file format parser for Atari ST YM2149 music.
//
// SNDH is a container format for Atari ST music containing:
// - 68000 assembly replay routines (INIT, EXIT, PLAY)
// - Metadata tags (title, composer, year, etc.)
// - Music data
//
// Format reference: https://sndh.atari.org/fileformat.php

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// SNDHHeader contains parsed metadata from an SNDH file
type SNDHHeader struct {
	Title        string
	Composer     string
	Ripper       string
	Converter    string
	Year         string
	SubSongCount int
	DefaultSong  int
	TimerType    string   // "A", "B", "C", "D", or "V" (VBL)
	TimerFreq    int      // Frequency in Hz (50 for PAL VBL)
	Durations    []int    // Per-subsong duration in seconds (0 = loops)
	Flags        []string // Hardware flags
}

// SNDHFile represents a parsed SNDH file
type SNDHFile struct {
	Header     SNDHHeader
	Data       []byte // Raw unpacked data (includes 68K code)
	InitOffset int    // Offset to INIT routine
	ExitOffset int    // Offset to EXIT routine
	PlayOffset int    // Offset to PLAY routine
	CodeOffset int    // Offset where 68K code/data begins (after HDNS)
}

// isSNDH checks if data contains an SNDH file (possibly ICE-packed)
func isSNDH(data []byte) bool {
	// Check for ICE-packed SNDH
	if isICE(data) {
		return true // Assume ICE-packed files in SNDH context are SNDH
	}

	// Check for raw SNDH - look for SNDH magic within first 16 bytes
	// (it's at offset 12 after the BRA instructions)
	if len(data) < 16 {
		return false
	}

	// Look for "SNDH" magic
	for i := 0; i <= 12 && i+4 <= len(data); i++ {
		if bytes.Equal(data[i:i+4], []byte("SNDH")) {
			return true
		}
	}

	return false
}

// findSNDHMagic locates the "SNDH" magic in the data
func findSNDHMagic(data []byte) int {
	for i := 0; i < len(data)-4; i++ {
		if bytes.Equal(data[i:i+4], []byte("SNDH")) {
			return i
		}
	}
	return -1
}

// ParseSNDHData parses SNDH data (handles ICE decompression if needed)
func ParseSNDHData(data []byte) (*SNDHFile, error) {
	// Decompress if ICE-packed
	if isICE(data) {
		unpacked, err := UnpackICE(data)
		if err != nil {
			return nil, fmt.Errorf("failed to unpack ICE: %w", err)
		}
		data = unpacked
	}

	// Find SNDH magic
	sndhOffset := findSNDHMagic(data)
	if sndhOffset < 0 {
		return nil, fmt.Errorf("SNDH magic not found")
	}

	file := &SNDHFile{
		Data: data,
	}

	// Parse branch instructions before SNDH magic
	// Standard layout: BRA.W to INIT, EXIT, PLAY at offsets 0, 4, 8
	if sndhOffset >= 12 {
		file.InitOffset = parseBranchTarget(data, 0)
		file.ExitOffset = parseBranchTarget(data, 4)
		file.PlayOffset = parseBranchTarget(data, 8)
	}

	// Parse tags starting after SNDH magic
	tagOffset := sndhOffset + 4

	// Default values
	file.Header.SubSongCount = 1
	file.Header.DefaultSong = 1
	file.Header.TimerType = "C"
	file.Header.TimerFreq = 50

	// Parse tags until HDNS
	for tagOffset < len(data)-4 {
		// Skip null bytes (padding between tags)
		for tagOffset < len(data) && data[tagOffset] == 0 {
			tagOffset++
		}
		if tagOffset >= len(data)-4 {
			break
		}

		// Check for HDNS (header end)
		if bytes.Equal(data[tagOffset:tagOffset+4], []byte("HDNS")) {
			// HDNS must be word-aligned, code starts after
			file.CodeOffset = tagOffset + 4
			if file.CodeOffset%2 != 0 {
				file.CodeOffset++
			}
			break
		}

		tag := string(data[tagOffset : tagOffset+4])
		tagOffset += 4

		switch tag {
		case "TITL":
			file.Header.Title, tagOffset = readNullString(data, tagOffset)
		case "COMM":
			file.Header.Composer, tagOffset = readNullString(data, tagOffset)
		case "RIPP":
			file.Header.Ripper, tagOffset = readNullString(data, tagOffset)
		case "CONV":
			file.Header.Converter, tagOffset = readNullString(data, tagOffset)
		case "YEAR":
			file.Header.Year, tagOffset = readNullString(data, tagOffset)
		default:
			// Handle special tags
			if strings.HasPrefix(tag, "##") {
				// Number of subsongs: ##nn
				numStr := tag[2:]
				if n, err := strconv.Atoi(numStr); err == nil && n > 0 {
					file.Header.SubSongCount = n
				}
			} else if strings.HasPrefix(tag, "!#") {
				// Default subsong: !#nn
				numStr := tag[2:]
				if n, err := strconv.Atoi(numStr); err == nil && n > 0 {
					file.Header.DefaultSong = n
				}
			} else if len(tag) >= 2 && tag[0] == 'T' && (tag[1] == 'A' || tag[1] == 'B' || tag[1] == 'C' || tag[1] == 'D') {
				// Timer: TAnnn, TBnnn, TCnnn, TDnnn
				file.Header.TimerType = string(tag[1])
				if freq, err := strconv.Atoi(tag[2:]); err == nil && freq > 0 {
					file.Header.TimerFreq = freq
				}
			} else if strings.HasPrefix(tag, "!V") {
				// VBL: !Vnn
				file.Header.TimerType = "V"
				if freq, err := strconv.Atoi(tag[2:]); err == nil && freq > 0 {
					file.Header.TimerFreq = freq
				}
			} else if tag == "TIME" {
				// Duration words for each subsong
				file.Header.Durations = make([]int, file.Header.SubSongCount)
				for i := 0; i < file.Header.SubSongCount && tagOffset+2 <= len(data); i++ {
					dur := int(binary.BigEndian.Uint16(data[tagOffset : tagOffset+2]))
					file.Header.Durations[i] = dur
					tagOffset += 2
				}
			} else if tag == "FLAG" {
				// Hardware flags - read null-terminated strings
				flagStr, newOffset := readNullString(data, tagOffset)
				if flagStr != "" {
					file.Header.Flags = append(file.Header.Flags, flagStr)
				}
				tagOffset = newOffset
			} else {
				// Unknown tag - try to skip past it
				// Look for next valid tag or HDNS
				_, tagOffset = skipUnknownTag(data, tagOffset)
			}
		}
	}

	// Validate offsets
	if file.InitOffset == 0 && sndhOffset >= 4 {
		// If no valid branch, assume code starts at offset 0
		file.InitOffset = 0
	}
	if file.PlayOffset == 0 && file.InitOffset > 0 {
		// Default PLAY to INIT + 8 if not specified
		file.PlayOffset = file.InitOffset + 8
	}

	return file, nil
}

// parseBranchTarget parses a 68000 BRA.W instruction and returns the target address
func parseBranchTarget(data []byte, offset int) int {
	if offset+4 > len(data) {
		return 0
	}

	opcode := binary.BigEndian.Uint16(data[offset : offset+2])

	// BRA.W = 0x6000 + 16-bit displacement
	if opcode&0xFF00 == 0x6000 {
		if opcode&0xFF == 0x00 {
			// BRA.W - 16-bit displacement follows
			disp := int16(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
			return offset + 2 + int(disp)
		} else {
			// BRA.B - 8-bit displacement in opcode
			disp := int8(opcode & 0xFF)
			return offset + 2 + int(disp)
		}
	}

	return 0
}

// readNullString reads a null-terminated string from data
func readNullString(data []byte, offset int) (string, int) {
	start := offset
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	if offset < len(data) {
		offset++ // Skip null terminator
	}
	return string(data[start : offset-1]), offset
}

// skipUnknownTag attempts to skip an unknown tag
func skipUnknownTag(data []byte, offset int) (string, int) {
	// Try to read as null-terminated string first
	start := offset
	for offset < len(data) && data[offset] != 0 && offset-start < 256 {
		offset++
	}
	if offset < len(data) && data[offset] == 0 {
		offset++
		return string(data[start : offset-1]), offset
	}

	// If no null found, just advance by 1 and let caller retry
	return "", start + 1
}

// GetDefaultDuration returns the duration for the default subsong in seconds
// Returns 0 if the song loops infinitely or duration is unknown
func (f *SNDHFile) GetDefaultDuration() int {
	if len(f.Header.Durations) == 0 {
		return 0
	}
	idx := f.Header.DefaultSong - 1
	if idx < 0 || idx >= len(f.Header.Durations) {
		idx = 0
	}
	return f.Header.Durations[idx]
}

// GetSubSongDuration returns the duration for a specific subsong (1-based index)
func (f *SNDHFile) GetSubSongDuration(subsong int) int {
	if len(f.Header.Durations) == 0 {
		return 0
	}
	idx := subsong - 1
	if idx < 0 || idx >= len(f.Header.Durations) {
		return 0
	}
	return f.Header.Durations[idx]
}
