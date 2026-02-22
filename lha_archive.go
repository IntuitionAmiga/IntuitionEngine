// lha_archive.go - Pure Go LHA archive parser and decompressor.
// Supports Level 0, 1, and 2 headers with -lh0- through -lh7- methods.

package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const lhaMaxSize = 64 << 20 // 64 MB maximum allocation guard

type lhaHeader struct {
	method         string
	compressedSize int
	originalSize   int
	headerSize     int
}

func parseLHAHeader(data []byte) (lhaHeader, error) {
	if len(data) < 21 {
		return lhaHeader{}, fmt.Errorf("lha: data too short for header (%d bytes)", len(data))
	}

	method := string(data[2:7])
	if len(method) != 5 || method[0] != '-' || method[4] != '-' {
		return lhaHeader{}, fmt.Errorf("lha: invalid method signature %q", method)
	}

	level := data[20]
	switch level {
	case 0:
		return parseLHALevel0(data, method)
	case 1:
		return parseLHALevel1(data, method)
	case 2:
		return parseLHALevel2(data, method)
	default:
		return lhaHeader{}, fmt.Errorf("lha: unsupported header level %d", level)
	}
}

func parseLHALevel0(data []byte, method string) (lhaHeader, error) {
	totalHeader := int(data[0]) + 2
	if len(data) < totalHeader {
		return lhaHeader{}, fmt.Errorf("lha: level 0 header truncated (need %d, have %d)", totalHeader, len(data))
	}
	if totalHeader < 22 {
		return lhaHeader{}, fmt.Errorf("lha: level 0 header too small (%d)", totalHeader)
	}

	compU32 := binary.LittleEndian.Uint32(data[7:11])
	origU32 := binary.LittleEndian.Uint32(data[11:15])

	if compU32 > lhaMaxSize {
		return lhaHeader{}, fmt.Errorf("lha: compressed size %d exceeds maximum", compU32)
	}
	if origU32 > lhaMaxSize {
		return lhaHeader{}, fmt.Errorf("lha: original size %d exceeds maximum", origU32)
	}

	return lhaHeader{
		method:         method,
		compressedSize: int(compU32),
		originalSize:   int(origU32),
		headerSize:     totalHeader,
	}, nil
}

func parseLHALevel1(data []byte, method string) (lhaHeader, error) {
	baseHeader := int(data[0]) + 2
	if len(data) < baseHeader {
		return lhaHeader{}, fmt.Errorf("lha: level 1 header truncated (need %d, have %d)", baseHeader, len(data))
	}
	if baseHeader < 27 {
		return lhaHeader{}, fmt.Errorf("lha: level 1 header too small (%d)", baseHeader)
	}

	compU32 := binary.LittleEndian.Uint32(data[7:11])
	origU32 := binary.LittleEndian.Uint32(data[11:15])

	if compU32 > lhaMaxSize {
		return lhaHeader{}, fmt.Errorf("lha: compressed size %d exceeds maximum", compU32)
	}
	if origU32 > lhaMaxSize {
		return lhaHeader{}, fmt.Errorf("lha: original size %d exceeds maximum", origU32)
	}

	compressedSize := int(compU32)

	// Walk extended header chain
	offset := baseHeader
	extTotal := 0
	for range 256 {
		if offset+2 > len(data) {
			return lhaHeader{}, fmt.Errorf("lha: level 1 extended header chain truncated at offset %d", offset)
		}
		nextSize := int(binary.LittleEndian.Uint16(data[offset:]))
		if nextSize == 0 {
			extTotal += 2 // terminator counts toward chain size
			offset += 2
			break
		}
		if offset+nextSize > len(data) {
			return lhaHeader{}, fmt.Errorf("lha: level 1 extended header overflows data at offset %d (size %d)", offset, nextSize)
		}
		extTotal += nextSize
		offset += nextSize
	}

	actualCompressed := compressedSize - extTotal
	if actualCompressed < 0 {
		return lhaHeader{}, fmt.Errorf("lha: level 1 extended headers (%d bytes) exceed compressed size (%d)", extTotal, compressedSize)
	}

	return lhaHeader{
		method:         method,
		compressedSize: actualCompressed,
		originalSize:   int(origU32),
		headerSize:     offset,
	}, nil
}

func parseLHALevel2(data []byte, method string) (lhaHeader, error) {
	if len(data) < 26 {
		return lhaHeader{}, fmt.Errorf("lha: level 2 header too short")
	}

	totalHeader := int(binary.LittleEndian.Uint16(data[0:2]))
	if len(data) < totalHeader {
		return lhaHeader{}, fmt.Errorf("lha: level 2 header truncated (need %d, have %d)", totalHeader, len(data))
	}

	compU32 := binary.LittleEndian.Uint32(data[7:11])
	origU32 := binary.LittleEndian.Uint32(data[11:15])

	if compU32 > lhaMaxSize {
		return lhaHeader{}, fmt.Errorf("lha: compressed size %d exceeds maximum", compU32)
	}
	if origU32 > lhaMaxSize {
		return lhaHeader{}, fmt.Errorf("lha: original size %d exceeds maximum", origU32)
	}

	return lhaHeader{
		method:         method,
		compressedSize: int(compU32),
		originalSize:   int(origU32),
		headerSize:     totalHeader,
	}, nil
}

func extractFirstFile(data []byte) ([]byte, error) {
	hdr, err := parseLHAHeader(data)
	if err != nil {
		return nil, err
	}

	if hdr.compressedSize < 0 || hdr.headerSize < 0 {
		return nil, fmt.Errorf("lha: invalid header values")
	}
	if hdr.compressedSize > len(data)-hdr.headerSize {
		return nil, fmt.Errorf("lha: compressed data truncated (need %d, have %d after header)", hdr.compressedSize, len(data)-hdr.headerSize)
	}

	payload := data[hdr.headerSize : hdr.headerSize+hdr.compressedSize]

	switch hdr.method {
	case "-lh0-":
		if len(payload) != hdr.originalSize {
			return nil, fmt.Errorf("lha: -lh0- size mismatch (compressed %d != original %d)", len(payload), hdr.originalSize)
		}
		out := make([]byte, hdr.originalSize)
		copy(out, payload)
		return out, nil
	case "-lh1-":
		return decompressLH1(payload, hdr.originalSize)
	case "-lh4-":
		return decompressLH(payload, hdr.originalSize, 12)
	case "-lh5-":
		return decompressLH(payload, hdr.originalSize, 13)
	case "-lh6-":
		return decompressLH(payload, hdr.originalSize, 15)
	case "-lh7-":
		return decompressLH(payload, hdr.originalSize, 16)
	default:
		return nil, fmt.Errorf("lha: unsupported compression method %s", hdr.method)
	}
}

func DecompressLHAFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lha: %w", err)
	}
	return extractFirstFile(data)
}

func DecompressLHAData(data []byte) ([]byte, error) {
	return extractFirstFile(data)
}

func init() {
	compiledFeatures = append(compiledFeatures, "lha:native")
}
