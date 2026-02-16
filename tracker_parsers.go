// tracker_parsers.go - Header parsers for ZX Spectrum tracker formats.
//
// Each parser extracts metadata and estimates song length from the module header.
// The actual music rendering is done by Z80 player routines via renderTrackerZ80().
//
// Supported formats:
//   PT3 - ProTracker 3.x (Vortex Tracker II)
//   PT2 - ProTracker 2
//   PT1 - ProTracker 1
//   STC - Sound Tracker Compiled
//   SQT - SQ-Tracker
//   ASC - ASC Sound Master
//   FTC - Fast Tracker (ZX)

package main

import "fmt"

// trackerModuleInfo holds metadata extracted from a tracker module header.
type trackerModuleInfo struct {
	format     string // "PT3", "PT2", "PT1", "STC", "SQT", "ASC", "FTC"
	title      string
	author     string
	positions  int // Number of positions in song
	loopPos    int // Loop position
	speed      int // Initial tempo (ticks per row)
	frameCount int // Estimated total frames
}

const pt3DefaultSpeed = 3
const pt3RowsPerPattern = 64

// parsePT3Header parses a ProTracker 3 module header.
// PT3 header layout:
//
//	0x00-0x0C: "ProTracker 3." signature (13 bytes)
//	0x0D:      Version digit (e.g., '5' for 3.5)
//	0x1E-0x3D: Song title (32 bytes, space-padded)
//	0x42-0x61: Author name (32 bytes, space-padded)
//	0x63:      Initial speed (tempo)
//	0x64:      Number of positions
//	0x65:      Loop position
//	0x66+:     Position list
func parsePT3Header(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x67 {
		return trackerModuleInfo{}, fmt.Errorf("pt3: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "PT3"}

	// Check signature
	sig := string(data[:13])
	if sig != "ProTracker 3." {
		// Some PT3 files lack the full signature but are still valid
		// Check for the "PT3!" or "VT2!" magic at known positions
		if len(data) > 100 && string(data[0x62:0x66]) == "PT3!" {
			// Alternative header layout
		}
	}

	info.title = parsePaddedString(data[0x1E:0x3E])
	if len(data) >= 0x62 {
		info.author = parsePaddedString(data[0x42:0x62])
	}

	info.speed = int(data[0x63])
	if info.speed == 0 {
		info.speed = pt3DefaultSpeed
	}
	info.positions = int(data[0x64])
	info.loopPos = int(data[0x65])

	// Estimate total frames: positions × rows_per_pattern × speed
	info.frameCount = info.positions * pt3RowsPerPattern * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000 // Default ~5 min
	}

	return info, nil
}

// parsePT2Header parses a ProTracker 2 module header.
// PT2 header layout:
//
//	0x00-0x1D: Song title (30 bytes, space-padded)
//	0x1E:      Number of positions
//	0x1F:      Loop position
//	0x20-0x3F: Position list (up to 32)
//	0x40-0x41: Pattern data offsets table pointer
func parsePT2Header(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x20 {
		return trackerModuleInfo{}, fmt.Errorf("pt2: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "PT2"}
	info.title = parsePaddedString(data[:0x1E])
	info.positions = int(data[0x1E])
	info.loopPos = int(data[0x1F])
	info.speed = 6 // PT2 default speed

	info.frameCount = info.positions * 64 * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000
	}

	return info, nil
}

// parsePT1Header parses a ProTracker 1 module header.
// PT1 has a simpler structure than PT2/PT3.
//
//	0x00:      Number of positions
//	0x01-0x20: Position list
func parsePT1Header(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x02 {
		return trackerModuleInfo{}, fmt.Errorf("pt1: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "PT1"}
	info.positions = int(data[0x00])
	info.speed = 6

	info.frameCount = info.positions * 64 * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000
	}

	return info, nil
}

// parseSTCHeader parses a Sound Tracker Compiled module header.
// STC header layout:
//
//	0x00:      Initial speed (delay)
//	0x01:      Number of positions
//	0x02+:     Position list (3 bytes per position: pattern A, B, C)
func parseSTCHeader(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x03 {
		return trackerModuleInfo{}, fmt.Errorf("stc: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "STC"}
	info.speed = int(data[0x00])
	if info.speed == 0 {
		info.speed = 1
	}
	info.positions = int(data[0x01])

	// STC: 64 rows per pattern, each row takes speed frames
	info.frameCount = info.positions * 64 * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000
	}

	return info, nil
}

// parseSQTHeader parses an SQ-Tracker module header.
// SQT header layout:
//
//	0x00:      Number of positions (0-based count)
//	0x01:      Loop position
//	0x02:      Initial speed
//	0x03+:     Position list
func parseSQTHeader(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x04 {
		return trackerModuleInfo{}, fmt.Errorf("sqt: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "SQT"}
	info.positions = int(data[0x00]) + 1
	info.loopPos = int(data[0x01])
	info.speed = int(data[0x02])
	if info.speed == 0 {
		info.speed = 3
	}

	info.frameCount = info.positions * 64 * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000
	}

	return info, nil
}

// parseASCHeader parses an ASC Sound Master module header.
// ASC has two versions (0 and 1) with slightly different layouts.
//
//	0x00:      Version byte (0 or 1)
//	0x01:      Initial speed
//	0x02:      Number of positions
//	0x03:      Loop position
//	0x04-0x23: Song title (32 bytes, padded)
func parseASCHeader(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x05 {
		return trackerModuleInfo{}, fmt.Errorf("asc: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "ASC"}
	version := data[0x00]
	if version > 1 {
		// Not necessarily invalid, but unexpected
	}
	info.speed = int(data[0x01])
	if info.speed == 0 {
		info.speed = 3
	}
	info.positions = int(data[0x02])
	info.loopPos = int(data[0x03])

	if len(data) >= 0x24 {
		info.title = parsePaddedString(data[0x04:0x24])
	}

	info.frameCount = info.positions * 64 * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000
	}

	return info, nil
}

// parseFTCHeader parses a Fast Tracker (ZX) module header.
// FTC header layout:
//
//	0x00-0x03: "FTC!" magic
//	0x04:      Number of positions
//	0x05:      Loop position
//	0x06:      Initial speed
//	0x07+:     Position list
func parseFTCHeader(data []byte) (trackerModuleInfo, error) {
	if len(data) < 0x08 {
		return trackerModuleInfo{}, fmt.Errorf("ftc: data too short (%d bytes)", len(data))
	}

	info := trackerModuleInfo{format: "FTC"}
	info.positions = int(data[0x04])
	info.loopPos = int(data[0x05])
	info.speed = int(data[0x06])
	if info.speed == 0 {
		info.speed = 3
	}

	info.frameCount = info.positions * 64 * info.speed
	if info.frameCount == 0 {
		info.frameCount = 15000
	}

	return info, nil
}

// parseTrackerModule dispatches to the appropriate header parser based on format extension.
func parseTrackerModule(ext string, data []byte) (trackerModuleInfo, error) {
	switch ext {
	case ".pt3":
		return parsePT3Header(data)
	case ".pt2":
		return parsePT2Header(data)
	case ".pt1":
		return parsePT1Header(data)
	case ".stc":
		return parseSTCHeader(data)
	case ".sqt":
		return parseSQTHeader(data)
	case ".asc":
		return parseASCHeader(data)
	case ".ftc":
		return parseFTCHeader(data)
	default:
		return trackerModuleInfo{}, fmt.Errorf("unknown tracker format: %s", ext)
	}
}

// isTrackerFormat returns true if the file extension is a supported tracker format.
func isTrackerFormat(ext string) bool {
	switch ext {
	case ".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc":
		return true
	}
	return false
}
