// ted_parser.go - Parser for TED/TMF music files (Commodore Plus/4)

/*
TED music files come in three main formats:

1. TMF format (cRTED standard):
   - PRG-style 2-byte load address header
   - BASIC loader with TEDMUSIC signature at file offset 17
   - Detection: BASIC line number < 4096 AND TEDMUSIC at offset 17
   - InitAddress/PlayAddress fields define entry points
   - RealTED mode when PlayAddress=0 (requires full raster-based emulation)
   - Metadata: title, author, tool, date, playtime per subtune

2. HVTC/Legacy format:
   - PRG-style 2-byte load address header
   - BASIC SYS loader program
   - "TEDMUSIC" signature found near end of file
   - Same header structure after signature

3. Raw PRG format:
   - Simple 2-byte load address header
   - Machine code program
   - No metadata (just load and execute)

TEDMUSIC header structure (relative to signature):
   Offset  Size  Description
   0       8     "TEDMUSIC" signature
   8       1     Null terminator
   9       2     Init address offset (little-endian)
   11      2     Play address (little-endian, 0 = RealTED mode)
   13      2     End address (little-endian)
   15      2     Reserved
   17      2     Number of subtunes (little-endian)
   19      1     FileFlags
   ...     ...   Additional header data
   48      32    Title (space-padded, Latin-1 for TMF)
   80      32    Author (space-padded)
   112     32    Date (space-padded)
   144     32    Tool/Player name (space-padded)
*/

package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
)

// TEDFormat represents the type of TED music file format
type TEDFormat int

const (
	TEDFormatRaw  TEDFormat = iota // Raw PRG file without TEDMUSIC header
	TEDFormatTMF                   // TMF format (cRTED standard, signature at offset 17)
	TEDFormatHVTC                  // HVTC/Legacy format (signature near end of file)
)

// String returns a human-readable format name
func (f TEDFormat) String() string {
	switch f {
	case TEDFormatTMF:
		return "TMF"
	case TEDFormatHVTC:
		return "HVTC"
	default:
		return "RAW"
	}
}

// TEDFile represents a parsed TED music file
type TEDFile struct {
	LoadAddr uint16 // Address to load program data
	InitAddr uint16 // Address to call for initialization
	PlayAddr uint16 // Address to call per frame for playback
	EndAddr  uint16 // End address of music data

	Title  string // Song title
	Author string // Author/composer
	Date   string // Release date
	Tool   string // Tool/player used to create

	NTSC         bool      // NTSC timing flag
	Subtunes     int       // Number of subtunes
	Data         []byte    // Program data (excluding load address header)
	ClockHz      uint32    // Clock frequency (PAL or NTSC)
	FrameRate    int       // Playback frame rate (50 or 60 Hz)
	FormatType   TEDFormat // Detected format type (TMF, HVTC, RAW)
	RealTEDMode  bool      // True when PlayAddr==0 (requires full raster emulation)
	FileFlags    byte      // FileFlags from header
	SubtuneTimes []float64 // Duration per subtune in seconds (if available)
}

// detectTEDFormat detects the format type of a TED music file
// Returns the format type and the position of the TEDMUSIC signature (or -1 if not found)
func detectTEDFormat(data []byte) (TEDFormat, int) {
	if len(data) < TMF_SIGNATURE_OFFSET+8 {
		return TEDFormatRaw, -1
	}

	sig := []byte("TEDMUSIC")

	// Check for TMF format: TEDMUSIC signature at offset 17
	// TMF format also has BASIC line number < 4096 in the first few bytes
	if len(data) >= TMF_SIGNATURE_OFFSET+len(sig) {
		if bytes.Equal(data[TMF_SIGNATURE_OFFSET:TMF_SIGNATURE_OFFSET+len(sig)], sig) {
			// Verify BASIC line number is < 4096 (indicates cRTED TMF format)
			// BASIC line number is at bytes 2-3 (after load address)
			if len(data) >= 4 {
				lineNum := uint16(data[2]) | (uint16(data[3]) << 8)
				if lineNum < TMF_MAX_BASIC_LINE {
					return TEDFormatTMF, TMF_SIGNATURE_OFFSET
				}
			}
			// Even without valid BASIC line, signature at offset 17 is TMF
			return TEDFormatTMF, TMF_SIGNATURE_OFFSET
		}
	}

	// Search for TEDMUSIC signature anywhere (HVTC/Legacy format)
	// Use bytes.Index for O(n) search instead of manual O(n*m) loop
	if idx := bytes.Index(data, sig); idx >= 0 {
		return TEDFormatHVTC, idx
	}

	// No signature found - raw PRG format
	return TEDFormatRaw, -1
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

	// Detect format and find signature position
	format, sigPos := detectTEDFormat(data)
	file.FormatType = format

	// Parse TEDMUSIC header if found
	if sigPos >= 0 {
		if format == TEDFormatTMF {
			parseTMFHeader(data, file)
		} else {
			parseTEDMUSICHeaderAt(data, sigPos, file)
		}
	}

	// Handle RealTED mode detection
	if file.PlayAddr == 0 && sigPos >= 0 {
		file.RealTEDMode = true
	}

	// For Plus/4 BASIC programs at $1001 without valid addresses from header,
	// use the SYS target as the entry point
	if file.LoadAddr == 0x1001 && (file.InitAddr == 0 || file.PlayAddr == 0) && !file.RealTEDMode {
		sysAddr := findSYSAddress(file.Data)
		if file.InitAddr == 0 {
			file.InitAddr = sysAddr
		}
		if file.PlayAddr == 0 {
			file.PlayAddr = sysAddr
		}
	} else if file.InitAddr == 0 && !file.RealTEDMode {
		// Non-BASIC file without header - use load address
		file.InitAddr = file.LoadAddr
		file.PlayAddr = file.LoadAddr
	}

	return file, nil
}

// findTEDMUSICSignature finds the "TEDMUSIC" signature in the data
// Deprecated: Use detectTEDFormat instead for proper format detection
func findTEDMUSICSignature(data []byte) int {
	_, sigPos := detectTEDFormat(data)
	return sigPos
}

// parseTMFHeader parses a TMF format file (cRTED standard with signature at offset 17)
func parseTMFHeader(data []byte, file *TEDFile) error {
	// TMF format has the signature at fixed offset 17
	return parseTEDMUSICHeaderAt(data, TMF_SIGNATURE_OFFSET, file)
}

// parseTEDMUSICHeaderAt parses the TEDMUSIC metadata header at the given signature position
// This is the unified parsing function used by both TMF and HVTC formats
func parseTEDMUSICHeaderAt(data []byte, sigPos int, file *TEDFile) error {
	// Header starts at signature position
	// Need at least signature + header fields + 4 x 32-byte strings
	minHeaderSize := 8 + 1 + TED_HDR_STRINGS + (4 * TED_STRING_SIZE)
	if sigPos+minHeaderSize > len(data) {
		// Try with smaller buffer - some files have truncated headers
		minHeaderSize = 8 + 1 + 20 // Just enough for core fields
		if sigPos+minHeaderSize > len(data) {
			return errors.New("header too small")
		}
	}

	pos := sigPos + 9 // Skip "TEDMUSIC\0"

	// Read header fields using defined constants
	// Bytes 9-10: Init offset (relative to music code base or absolute address)
	initOffset := uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	pos += 2

	// Bytes 11-12: Play address (0 = RealTED mode requiring full raster emulation)
	playAddr := uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	pos += 2

	// Bytes 13-14: End address
	if pos+2 <= len(data) {
		file.EndAddr = uint16(data[pos]) | (uint16(data[pos+1]) << 8)
	}
	pos += 2

	// Bytes 15-16: Reserved
	pos += 2

	// Bytes 17-18: Number of subtunes
	if pos+2 <= len(data) {
		file.Subtunes = int(data[pos]) | (int(data[pos+1]) << 8)
		if file.Subtunes < 1 {
			file.Subtunes = 1
		}
	}
	pos += 2

	// Byte 19: FileFlags
	if pos < len(data) {
		file.FileFlags = data[pos]
	}

	// Parse metadata strings
	// TMF format uses Latin-1 encoding, HVTC may use PETSCII
	stringStart := sigPos + TED_HDR_STRINGS
	if stringStart+(4*TED_STRING_SIZE) <= len(data) {
		isTMF := (file.FormatType == TEDFormatTMF)
		file.Title = parseMetadataString(data[stringStart:stringStart+TED_STRING_SIZE], isTMF)
		file.Author = parseMetadataString(data[stringStart+TED_STRING_SIZE:stringStart+(2*TED_STRING_SIZE)], isTMF)
		file.Date = parseMetadataString(data[stringStart+(2*TED_STRING_SIZE):stringStart+(3*TED_STRING_SIZE)], isTMF)
		file.Tool = parseMetadataString(data[stringStart+(3*TED_STRING_SIZE):stringStart+(4*TED_STRING_SIZE)], isTMF)
	} else if sigPos+39+(4*TED_STRING_SIZE) <= len(data) {
		// Alternative offset for older HVTC files
		stringStart = sigPos + 39
		isTMF := false // Old format likely uses PETSCII
		file.Title = parseMetadataString(data[stringStart:stringStart+TED_STRING_SIZE], isTMF)
		file.Author = parseMetadataString(data[stringStart+TED_STRING_SIZE:stringStart+(2*TED_STRING_SIZE)], isTMF)
		file.Date = parseMetadataString(data[stringStart+(2*TED_STRING_SIZE):stringStart+(3*TED_STRING_SIZE)], isTMF)
		file.Tool = parseMetadataString(data[stringStart+(3*TED_STRING_SIZE):stringStart+(4*TED_STRING_SIZE)], isTMF)
	}

	// Determine init and play addresses
	if playAddr == 0 {
		// RealTED mode - player uses raster-based timing, runs from InitAddr
		file.RealTEDMode = true
		if initOffset >= 0x1000 {
			file.InitAddr = initOffset
		} else if initOffset > 0 {
			file.InitAddr = file.LoadAddr + initOffset
		}
		// PlayAddr stays 0 for RealTED mode
		return nil
	}

	// Standard mode - resolve play address through JMP if present
	if playAddr >= 0x1000 {
		// Calculate file offset for the play address
		fileOffset := int(playAddr-file.LoadAddr) + 2 // +2 for load address header

		// Check if there's a JMP instruction at this location
		if fileOffset >= 0 && fileOffset+3 <= len(data) && data[fileOffset] == 0x4C {
			// JMP opcode found - extract the target address (little-endian)
			jmpTarget := uint16(data[fileOffset+1]) | (uint16(data[fileOffset+2]) << 8)
			file.PlayAddr = jmpTarget
			file.InitAddr = jmpTarget
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

// parseTEDMUSICHeader parses the TEDMUSIC metadata header (legacy wrapper)
// Deprecated: Use parseTEDMUSICHeaderAt instead
func parseTEDMUSICHeader(data []byte, sigPos int, file *TEDFile) error {
	return parseTEDMUSICHeaderAt(data, sigPos, file)
}

// parseMetadataString parses a metadata string with encoding conversion
// isTMF=true uses Latin-1 encoding, isTMF=false converts from PETSCII
func parseMetadataString(data []byte, isTMF bool) string {
	// Find first null byte using bytes.IndexByte
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		end = len(data)
	}

	if isTMF {
		// TMF format uses Latin-1 (ISO-8859-1) encoding
		// Convert Latin-1 to UTF-8 - pre-allocate exact size
		result := make([]rune, end)
		for i := 0; i < end; i++ {
			result[i] = rune(data[i])
		}
		return strings.TrimRight(string(result), " ")
	}

	// HVTC/Legacy format may use PETSCII
	// Convert PETSCII to ASCII/UTF-8 - pre-allocate buffer
	result := make([]byte, 0, end)
	for i := 0; i < end; i++ {
		b := data[i]
		switch {
		case b >= 0x41 && b <= 0x5A:
			// PETSCII uppercase A-Z -> lowercase a-z
			result = append(result, b+0x20)
		case b >= 0x61 && b <= 0x7A:
			// PETSCII lowercase a-z -> uppercase A-Z
			result = append(result, b-0x20)
		case b >= 0xC1 && b <= 0xDA:
			// PETSCII shifted uppercase -> regular uppercase
			result = append(result, b-0x80)
		case b >= 0x20 && b <= 0x7E:
			// Printable ASCII range - keep as-is
			result = append(result, b)
		case b == 0x0D:
			// Carriage return -> space
			result = append(result, ' ')
		case b >= 0xA0 && b <= 0xBF:
			// High characters to printable equivalents
			result = append(result, b-0x80)
		}
		// Skip other non-printable characters
	}
	return strings.TrimRight(string(result), " ")
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
