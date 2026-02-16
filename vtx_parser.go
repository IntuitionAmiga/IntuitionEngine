// vtx_parser.go - VTX file format parser (Vortex Tracker / AY Emulator).
//
// VTX is a metadata wrapper around LH5-compressed YM register data.
// Created by Sergey Bulba for the AY Emulator program.
//
// Header layout (little-endian):
//   0x00  2  Magic: "ay" (AY-3-8910) or "ym" (YM2149)
//   0x02  1  Stereo type (0=MONO, 1=ABC, 2=ACB, 3=BAC, 4=BCA, 5=CAB, 6=CBA)
//   0x03  2  Chip frequency (uint16 LE) — in Hz; may need scaling
//   0x05  1  Player/interrupt frequency (50 or 60 Hz)
//   0x06  2  Year (uint16 LE)
//   0x08  4  Uncompressed data size (uint32 LE)
//   0x0C  ...  5 null-terminated strings: title, author, from, tracker, comment
//   ...   ...  LH5-compressed YM register data (interleaved)

package main

import (
	"encoding/binary"
	"fmt"
)

const vtxMinHeaderSize = 12 // Fixed header before variable strings

// VTXStereo represents the stereo layout of a VTX file.
type VTXStereo uint8

const (
	VTXStereoMono VTXStereo = 0
	VTXStereoABC  VTXStereo = 1
	VTXStereoACB  VTXStereo = 2
	VTXStereoBAC  VTXStereo = 3
	VTXStereoBCA  VTXStereo = 4
	VTXStereoCAB  VTXStereo = 5
	VTXStereoCBA  VTXStereo = 6
)

// VTXHeader contains the parsed VTX file header.
type VTXHeader struct {
	ChipType   string    // "ay" or "ym"
	Stereo     VTXStereo // Stereo layout (0-6)
	ChipFreqHz uint32    // Chip frequency in Hz
	PlayerFreq uint8     // Interrupt/player frequency (50 or 60)
	Year       uint16    // Year of creation
	DataSize   uint32    // Uncompressed data size in bytes
	Title      string
	Author     string
	From       string // Program/source name
	Tracker    string // Tracker used
	Comment    string
}

// isVTXData detects VTX format by checking the magic bytes and stereo field.
func isVTXData(data []byte) bool {
	if len(data) < vtxMinHeaderSize {
		return false
	}
	magic := string(data[:2])
	if magic != "ay" && magic != "ym" {
		return false
	}
	stereo := data[2]
	return stereo <= 6
}

// ParseVTXData parses a VTX file and returns a YMFile (for loadFrames) and metadata.
func ParseVTXData(data []byte) (*YMFile, PSGMetadata, error) {
	header, compressedData, err := parseVTXHeader(data)
	if err != nil {
		return nil, PSGMetadata{}, err
	}

	if len(compressedData) == 0 {
		return nil, PSGMetadata{}, fmt.Errorf("vtx: no compressed data")
	}

	decompressed, err := decompressLH5(compressedData, int(header.DataSize))
	if err != nil {
		return nil, PSGMetadata{}, fmt.Errorf("vtx: decompression failed: %w", err)
	}

	// Decompressed data is interleaved YM3 format (register-major order)
	ym, err := parseYMLegacy(decompressed, 0, "YM3!")
	if err != nil {
		return nil, PSGMetadata{}, fmt.Errorf("vtx: frame parsing failed: %w", err)
	}

	// Override defaults with VTX header values
	ym.ClockHz = header.ChipFreqHz
	if header.PlayerFreq > 0 {
		ym.FrameRate = uint16(header.PlayerFreq)
	}
	ym.Title = header.Title
	ym.Author = header.Author

	meta := PSGMetadata{
		Title:  header.Title,
		Author: header.Author,
		System: vtxSystemName(header.ChipType),
	}

	return ym, meta, nil
}

// parseVTXHeader parses the VTX header and returns the header info
// plus a slice pointing to the compressed data region.
func parseVTXHeader(data []byte) (VTXHeader, []byte, error) {
	if len(data) < vtxMinHeaderSize {
		return VTXHeader{}, nil, fmt.Errorf("vtx: data too short (%d bytes)", len(data))
	}

	magic := string(data[:2])
	if magic != "ay" && magic != "ym" {
		return VTXHeader{}, nil, fmt.Errorf("vtx: invalid magic %q", magic)
	}

	stereo := data[2]
	if stereo > 6 {
		return VTXHeader{}, nil, fmt.Errorf("vtx: invalid stereo type %d", stereo)
	}

	h := VTXHeader{
		ChipType: magic,
		Stereo:   VTXStereo(stereo),
	}

	// Detect header variant: check if byte at offset 5 or 7 is a valid player freq (50 or 60)
	// This distinguishes uint16 vs uint32 chip frequency field.
	off := 3
	if len(data) >= 14 && (data[7] == 50 || data[7] == 60) {
		// uint32 chip frequency variant (14-byte fixed header)
		h.ChipFreqHz = binary.LittleEndian.Uint32(data[off:])
		off = 7
		h.PlayerFreq = data[off]
		off++
		h.Year = binary.LittleEndian.Uint16(data[off:])
		off += 2
		h.DataSize = binary.LittleEndian.Uint32(data[off:])
		off += 4
	} else {
		// uint16 chip frequency variant (12-byte fixed header)
		rawFreq := binary.LittleEndian.Uint16(data[off:])
		off += 2
		h.PlayerFreq = data[off]
		off++
		h.Year = binary.LittleEndian.Uint16(data[off:])
		off += 2
		h.DataSize = binary.LittleEndian.Uint32(data[off:])
		off += 4

		// Scale frequency: if < 10000, likely in kHz units
		h.ChipFreqHz = uint32(rawFreq)
		if rawFreq > 0 && rawFreq < 10000 {
			h.ChipFreqHz = uint32(rawFreq) * 1000
		}
	}

	// Apply defaults
	if h.ChipFreqHz == 0 {
		if h.ChipType == "ym" {
			h.ChipFreqHz = PSG_CLOCK_ATARI_ST
		} else {
			h.ChipFreqHz = PSG_CLOCK_ZX_SPECTRUM
		}
	}
	if h.PlayerFreq == 0 {
		h.PlayerFreq = 50
	}

	// Read 5 null-terminated strings
	h.Title, off = parseNullTerminatedString(data, off)
	h.Author, off = parseNullTerminatedString(data, off)
	h.From, off = parseNullTerminatedString(data, off)
	h.Tracker, off = parseNullTerminatedString(data, off)
	h.Comment, off = parseNullTerminatedString(data, off)

	if off > len(data) {
		return VTXHeader{}, nil, fmt.Errorf("vtx: header extends beyond data")
	}

	return h, data[off:], nil
}

func vtxSystemName(chipType string) string {
	if chipType == "ym" {
		return "Atari ST"
	}
	return "ZX Spectrum"
}
