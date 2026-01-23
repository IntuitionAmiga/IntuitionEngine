// ted_parser.go - Parser for TED/TMF music files (Commodore Plus/4)

/*
TED music files come in two main formats:

1. TMF format (.ted files from HVTC collection):
   - PRG-style 2-byte load address header
   - BASIC SYS loader program
   - Embedded 6502 music code
   - "TEDMUSIC" signature near end with metadata header

2. Raw PRG format:
   - Simple 2-byte load address header
   - Machine code program
   - No metadata (just load and execute)

TEDMUSIC header structure (found near end of file):
   Offset  Size  Description
   0       8     "TEDMUSIC" signature
   8       1     Null terminator
   9       2     Init address offset (little-endian)
   11      2     Play address (little-endian)
   13      2     End address or play repeat (little-endian)
   15      2     Reserved
   17      2     Number of subtunes (little-endian)
   19      2     Flags (e.g., "FE")
   ...     ...   More header data
   39      32    Title (space-padded)
   71      32    Author (space-padded)
   103     32    Date (space-padded)
   135     32    Tool/Player name (space-padded)
*/

package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
)

// TEDFile represents a parsed TED music file
type TEDFile struct {
	LoadAddr uint16 // Address to load program data
	InitAddr uint16 // Address to call for initialization
	PlayAddr uint16 // Address to call per frame for playback

	Title  string // Song title
	Author string // Author/composer
	Date   string // Release date
	Tool   string // Tool/player used to create

	NTSC      bool   // NTSC timing flag
	Subtunes  int    // Number of subtunes
	Data      []byte // Program data (excluding load address header)
	ClockHz   uint32 // Clock frequency (PAL or NTSC)
	FrameRate int    // Playback frame rate (50 or 60 Hz)
}

// parseTEDFile parses a TED/TMF music file
func parseTEDFile(data []byte) (*TEDFile, error) {
	if len(data) < 3 {
		return nil, errors.New("file too small")
	}

	file := &TEDFile{
		ClockHz:   TED_CLOCK_PAL,
		FrameRate: 50,
		NTSC:      false,
		Subtunes:  1,
	}

	// First two bytes are the load address (little-endian PRG format)
	file.LoadAddr = uint16(data[0]) | (uint16(data[1]) << 8)
	file.Data = data[2:]

	// Look for TEDMUSIC signature for metadata
	sigPos := findTEDMUSICSignature(data)
	if sigPos >= 0 {
		// Parse the TEDMUSIC header for metadata only
		parseTEDMUSICHeader(data, sigPos, file)
	}

	// For Plus/4 BASIC programs at $1001, always use the SYS target
	// as the entry point. This handles both packed (unpacker at SYS)
	// and unpacked (direct music code via JMP) files correctly.
	if file.LoadAddr == 0x1001 {
		sysAddr := findSYSAddress(file.Data)
		file.InitAddr = sysAddr
		file.PlayAddr = sysAddr
	} else if file.InitAddr == 0 {
		// Non-BASIC file without header - use load address
		file.InitAddr = file.LoadAddr
		file.PlayAddr = file.LoadAddr
	}

	return file, nil
}

// findTEDMUSICSignature finds the "TEDMUSIC" signature in the data
func findTEDMUSICSignature(data []byte) int {
	sig := []byte("TEDMUSIC")
	for i := 0; i <= len(data)-len(sig); i++ {
		if bytes.Equal(data[i:i+len(sig)], sig) {
			return i
		}
	}
	return -1
}

// parseTEDMUSICHeader parses the TEDMUSIC metadata header
func parseTEDMUSICHeader(data []byte, sigPos int, file *TEDFile) error {
	// Header starts at signature position
	// Need at least signature + header fields + 4 x 32-byte strings
	minHeaderSize := 8 + 1 + 30 + 128 // sig + null + header + strings
	if sigPos+minHeaderSize > len(data) {
		return errors.New("header too small")
	}

	pos := sigPos + 9 // Skip "TEDMUSIC\0"

	// Read header fields (offsets based on analysis of real files)
	// Byte 9-10: Init offset (relative to actual music code base)
	initOffset := uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	pos += 2

	// Byte 11-12: Play address (points to BASIC stub JMP)
	playAddr := uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	pos += 2

	// Byte 13-14: End address or duplicate
	pos += 2 // Skip

	// Byte 15-16: Reserved
	pos += 2 // Skip

	// Byte 17-18: Number of subtunes
	if pos+2 <= len(data) {
		file.Subtunes = int(data[pos]) | (int(data[pos+1]) << 8)
		if file.Subtunes < 1 {
			file.Subtunes = 1
		}
	}
	pos += 2

	// Skip remaining header bytes to reach metadata strings
	// The strings start at a fixed offset from the signature
	// Based on analysis: strings start at sig+48 for the title
	stringStart := sigPos + 48 // Adjusted based on hex dump analysis
	if stringStart+128 > len(data) {
		stringStart = sigPos + 39 // Alternative offset
	}

	// Parse 32-byte strings
	if stringStart+128 <= len(data) {
		file.Title = trimString(data[stringStart : stringStart+32])
		file.Author = trimString(data[stringStart+32 : stringStart+64])
		file.Date = trimString(data[stringStart+64 : stringStart+96])
		file.Tool = trimString(data[stringStart+96 : stringStart+128])
	}

	// The play address points to a BASIC stub that typically contains a JMP
	// to the actual music code. We need to follow this JMP to find the real address.
	if playAddr >= 0x1000 {
		// Calculate file offset for the play address
		// playAddr is the memory address, data starts at file.LoadAddr
		fileOffset := int(playAddr-file.LoadAddr) + 2 // +2 for load address header

		// Check if there's a JMP instruction at this location
		if fileOffset >= 0 && fileOffset+3 <= len(data) && data[fileOffset] == 0x4C {
			// JMP opcode found - extract the target address (little-endian)
			jmpTarget := uint16(data[fileOffset+1]) | (uint16(data[fileOffset+2]) << 8)
			file.PlayAddr = jmpTarget
			// Init is typically at the same address (play with A=0) or at play + initOffset
			if initOffset > 0 && initOffset < 0x100 {
				// Small offset - probably relative to the JMP target
				file.InitAddr = jmpTarget
			} else {
				file.InitAddr = jmpTarget
			}
		} else {
			// No JMP found, use addresses as-is
			file.PlayAddr = playAddr
			if initOffset > 0 && initOffset < 0x1000 {
				file.InitAddr = file.LoadAddr + initOffset
			} else if initOffset >= 0x1000 {
				file.InitAddr = initOffset
			} else {
				file.InitAddr = playAddr
			}
		}
	} else {
		file.PlayAddr = file.LoadAddr
		file.InitAddr = file.LoadAddr
	}

	return nil
}

// trimString removes trailing spaces and null bytes from a string
func trimString(data []byte) string {
	// Find first null byte
	end := len(data)
	for i, b := range data {
		if b == 0 {
			end = i
			break
		}
	}
	return strings.TrimRight(string(data[:end]), " ")
}

// findSYSAddress scans BASIC program for SYS command and extracts address
// It also follows JMP instructions to find the actual code address
func findSYSAddress(data []byte) uint16 {
	// BASIC SYS token is 0x9E
	// Format: SYS followed by ASCII digits for the address
	sysToken := byte(0x9E)
	for i := 0; i < len(data)-4; i++ {
		if data[i] == sysToken {
			// Read following digits
			addr := uint16(0)
			for j := i + 1; j < len(data) && j < i+6; j++ {
				if data[j] >= '0' && data[j] <= '9' {
					addr = addr*10 + uint16(data[j]-'0')
				} else if data[j] != ' ' {
					break
				}
			}
			if addr > 0 {
				// Found SYS address - now check if it's a JMP instruction
				// Calculate file offset (data is file content after load address)
				// Load address for Plus/4 BASIC is $1001
				loadAddr := uint16(0x1001)
				fileOffset := int(addr - loadAddr)
				if fileOffset >= 0 && fileOffset+3 <= len(data) && data[fileOffset] == 0x4C {
					// JMP instruction - follow it
					jmpTarget := uint16(data[fileOffset+1]) | (uint16(data[fileOffset+2]) << 8)
					return jmpTarget
				}
				return addr
			}
		}
	}
	// Default to common Plus/4 addresses
	return 0x100D
}

// isTEDExtension returns true if the file path has a TED-related extension
func isTEDExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ted", ".prg":
		return true
	default:
		return false
	}
}
