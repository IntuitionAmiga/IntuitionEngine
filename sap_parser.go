package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SAPHeader represents the parsed SAP file header
type SAPHeader struct {
	Author    string    // AUTHOR tag
	Name      string    // NAME tag
	Date      string    // DATE tag
	Songs     int       // SONGS tag (default 1)
	DefSong   int       // DEFSONG tag (default 0)
	Stereo    bool      // STEREO tag
	NTSC      bool      // NTSC tag (default PAL)
	Type      byte      // TYPE tag: B, C, D, S, R
	FastPlay  int       // FASTPLAY tag (scanlines per frame)
	Init      uint16    // INIT tag (for TYPE B)
	Player    uint16    // PLAYER tag
	Music     uint16    // MUSIC tag (for TYPE C)
	Durations []float64 // TIME tags (seconds per subsong)
	LoopFlags []bool    // LOOP flags per subsong
}

// SAPBlock represents a binary data block in the SAP file
type SAPBlock struct {
	Start uint16 // Start address (inclusive)
	End   uint16 // End address (inclusive)
	Data  []byte // Binary data
}

// SAPFile represents a fully parsed SAP file
type SAPFile struct {
	Header SAPHeader
	Blocks []SAPBlock
}

// isSAPData checks if data starts with SAP file signature
func isSAPData(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	// SAP files start with "SAP" followed by CR/LF or just LF
	return bytes.HasPrefix(data, []byte("SAP\r\n")) || bytes.HasPrefix(data, []byte("SAP\n"))
}

// ParseSAPFile loads and parses a SAP file from disk
func ParseSAPFile(path string) (*SAPFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSAPData(data)
}

// ParseSAPData parses SAP data from a byte slice
func ParseSAPData(data []byte) (*SAPFile, error) {
	if !isSAPData(data) {
		return nil, errors.New("not a valid SAP file: missing SAP signature")
	}

	file := &SAPFile{
		Header: SAPHeader{
			Songs:    1,
			DefSong:  0,
			FastPlay: 0, // Will be set based on NTSC flag if not specified
		},
	}

	// Find the end of header (marked by 0xFF 0xFF)
	headerEnd := bytes.Index(data, []byte{0xFF, 0xFF})
	if headerEnd == -1 {
		return nil, errors.New("SAP file missing binary section marker (0xFF 0xFF)")
	}

	// Parse header lines
	headerData := data[:headerEnd]
	if err := parseHeader(headerData, &file.Header); err != nil {
		return nil, err
	}

	// Set default FASTPLAY based on NTSC flag if not specified
	if file.Header.FastPlay == 0 {
		if file.Header.NTSC {
			file.Header.FastPlay = 262 // NTSC default
		} else {
			file.Header.FastPlay = 312 // PAL default
		}
	}

	// Parse binary blocks
	binaryData := data[headerEnd:]
	blocks, err := parseBlocks(binaryData)
	if err != nil {
		return nil, err
	}
	file.Blocks = blocks

	// Validate required fields
	if err := validateHeader(&file.Header); err != nil {
		return nil, err
	}

	return file, nil
}

// parseHeader parses the text header portion of a SAP file
func parseHeader(data []byte, header *SAPHeader) error {
	// Process line by line without allocating entire slice
	pos := 0
	for pos < len(data) {
		// Find end of line
		lineEnd := pos
		for lineEnd < len(data) && data[lineEnd] != '\n' {
			lineEnd++
		}

		// Get line, trimming \r if present
		line := data[pos:lineEnd]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		// Trim leading/trailing spaces
		line = bytes.TrimSpace(line)

		// Skip empty lines and SAP marker
		if len(line) == 0 || bytes.Equal(line, []byte("SAP")) {
			pos = lineEnd + 1
			continue
		}

		// Find tag/value separator (first space)
		spaceIdx := bytes.IndexByte(line, ' ')
		var tag, value []byte
		if spaceIdx >= 0 {
			tag = line[:spaceIdx]
			value = bytes.TrimSpace(line[spaceIdx+1:])
		} else {
			tag = line
			value = nil
		}

		// Convert tag to string for switch (small allocation, unavoidable for now)
		tagStr := string(tag)
		valueStr := string(value)

		switch tagStr {
		case "AUTHOR":
			header.Author = parseQuotedString(valueStr)
		case "NAME":
			header.Name = parseQuotedString(valueStr)
		case "DATE":
			header.Date = parseQuotedString(valueStr)
		case "SONGS":
			n, _ := strconv.Atoi(valueStr)
			if n > 0 {
				header.Songs = n
			}
		case "DEFSONG":
			n, _ := strconv.Atoi(valueStr)
			header.DefSong = n
		case "STEREO":
			header.Stereo = true
		case "NTSC":
			header.NTSC = true
		case "TYPE":
			if len(value) > 0 {
				header.Type = value[0]
			}
		case "FASTPLAY":
			n, _ := strconv.Atoi(valueStr)
			if n > 0 {
				header.FastPlay = n
			}
		case "INIT":
			addr, _ := strconv.ParseUint(valueStr, 16, 16)
			header.Init = uint16(addr)
		case "PLAYER":
			addr, _ := strconv.ParseUint(valueStr, 16, 16)
			header.Player = uint16(addr)
		case "MUSIC":
			addr, _ := strconv.ParseUint(valueStr, 16, 16)
			header.Music = uint16(addr)
		case "TIME":
			duration, hasLoop := parseTime(valueStr)
			header.Durations = append(header.Durations, duration)
			header.LoopFlags = append(header.LoopFlags, hasLoop)
		}

		pos = lineEnd + 1
	}

	return nil
}

// parseQuotedString extracts content from a quoted string
func parseQuotedString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseTime parses a TIME value in format MM:SS.mmm [LOOP]
func parseTime(value string) (float64, bool) {
	hasLoop := false
	if strings.HasSuffix(value, " LOOP") {
		hasLoop = true
		value = strings.TrimSuffix(value, " LOOP")
	}
	value = strings.TrimSpace(value)

	var minutes, seconds float64
	var millis float64

	// Try MM:SS.mmm format
	if idx := strings.Index(value, ":"); idx > 0 {
		minutes, _ = strconv.ParseFloat(value[:idx], 64)
		secPart := value[idx+1:]

		if dotIdx := strings.Index(secPart, "."); dotIdx > 0 {
			seconds, _ = strconv.ParseFloat(secPart[:dotIdx], 64)
			millisStr := secPart[dotIdx+1:]
			// Normalize milliseconds (could be 1, 2, or 3 digits) without allocation
			switch len(millisStr) {
			case 0:
				millis = 0
			case 1:
				millis, _ = strconv.ParseFloat(millisStr, 64)
				millis *= 100 // "5" -> 500
			case 2:
				millis, _ = strconv.ParseFloat(millisStr, 64)
				millis *= 10 // "50" -> 500
			default:
				millis, _ = strconv.ParseFloat(millisStr[:3], 64)
			}
		} else {
			seconds, _ = strconv.ParseFloat(secPart, 64)
		}
	}

	return minutes*60 + seconds + millis/1000, hasLoop
}

// parseBlocks parses the binary data blocks from SAP file
func parseBlocks(data []byte) ([]SAPBlock, error) {
	var blocks []SAPBlock
	pos := 0

	for pos < len(data) {
		// Skip any 0xFF 0xFF markers
		for pos+1 < len(data) && data[pos] == 0xFF && data[pos+1] == 0xFF {
			pos += 2
		}

		if pos >= len(data) {
			break
		}

		// Need at least 4 bytes for start and end addresses
		if pos+3 >= len(data) {
			break
		}

		// Read start address (little-endian)
		start := uint16(data[pos]) | uint16(data[pos+1])<<8
		pos += 2

		// Read end address (little-endian)
		end := uint16(data[pos]) | uint16(data[pos+1])<<8
		pos += 2

		// Calculate data length
		if end < start {
			return nil, fmt.Errorf("invalid block: end address 0x%04X < start address 0x%04X", end, start)
		}
		length := int(end-start) + 1

		// Read data
		if pos+length > len(data) {
			// Truncated block at end of file
			length = len(data) - pos
		}

		blockData := make([]byte, length)
		copy(blockData, data[pos:pos+length])
		pos += length

		blocks = append(blocks, SAPBlock{
			Start: start,
			End:   end,
			Data:  blockData,
		})
	}

	return blocks, nil
}

// validateHeader checks that all required header fields are present
func validateHeader(header *SAPHeader) error {
	// TYPE is always required
	validTypes := []byte{'B', 'C', 'D', 'S', 'R'}
	typeValid := false
	for _, t := range validTypes {
		if header.Type == t {
			typeValid = true
			break
		}
	}
	if !typeValid {
		if header.Type == 0 {
			return errors.New("SAP file missing required TYPE tag")
		}
		return fmt.Errorf("SAP file has invalid TYPE: %c", header.Type)
	}

	// TYPE B requires INIT
	if header.Type == 'B' && header.Init == 0 && header.Player == 0 {
		return errors.New("SAP TYPE B requires INIT and PLAYER tags")
	}

	// TYPE C requires PLAYER and MUSIC
	if header.Type == 'C' && header.Player == 0 {
		return errors.New("SAP TYPE C requires PLAYER tag")
	}

	return nil
}
