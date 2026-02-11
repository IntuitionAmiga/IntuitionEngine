// ay_parser.go - AY raw frame parser (limited scope).

package main

import (
	"bytes"
	"fmt"
	"os"
)

type AYFile struct {
	Frames    [][]uint8
	FrameRate uint16
	ClockHz   uint32
	Title     string
	Author    string
}

func ParseAYFile(path string) (*AYFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseAYData(data)
}

func ParseAYData(data []byte) (*AYFile, error) {
	if bytes.HasPrefix(data, []byte("ZXAYEMUL")) {
		return nil, fmt.Errorf("ay file uses Z80 player code; raw frames required")
	}

	if len(data)%PSG_REG_COUNT != 0 {
		return nil, fmt.Errorf("ay raw frame data must be multiple of %d bytes", PSG_REG_COUNT)
	}

	frameCount := len(data) / PSG_REG_COUNT

	// Allocate single contiguous buffer for all frames
	buffer := make([]uint8, len(data))
	copy(buffer, data)

	// Create slice headers pointing into the contiguous buffer
	frames := make([][]uint8, frameCount)
	for i := range frameCount {
		start := i * PSG_REG_COUNT
		frames[i] = buffer[start : start+PSG_REG_COUNT : start+PSG_REG_COUNT]
	}

	return &AYFile{
		Frames:    frames,
		FrameRate: 50,
		ClockHz:   PSG_CLOCK_ZX_SPECTRUM,
		Title:     "",
		Author:    "",
	}, nil
}
