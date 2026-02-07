// sndh_68k_render.go - Entry point for SNDH rendering to PSGEvents.
//
// This provides a unified interface for SNDH playback that matches
// the existing AY/Z80 render pattern used by the PSG player.

package main

import (
	"bytes"
	"fmt"
)

const (
	// Default loop frame count when duration is unknown or loops infinitely
	sndhDefaultLoopFrames = 15000

	// Maximum frame count to prevent runaway rendering
	sndhMaxFrames = 600000
)

// isSNDHData checks if data is SNDH format (possibly ICE-packed)
func isSNDHData(data []byte) bool {
	// Check for ICE-packed data
	if isICE(data) {
		return true
	}

	// Check for raw SNDH magic
	if len(data) < 16 {
		return false
	}

	// Look for SNDH magic in first 16 bytes
	for i := 0; i <= 12 && i+4 <= len(data); i++ {
		if bytes.Equal(data[i:i+4], []byte("SNDH")) {
			return true
		}
	}

	return false
}

// renderSNDH renders SNDH data to PSGEvents
// Returns: metadata, events, totalSamples, clockHz, frameRate, loop, loopSample, error
func renderSNDH(data []byte, sampleRate int) (PSGMetadata, []PSGEvent, uint64, uint32, uint16, bool, uint64, uint64, uint64, error) {
	return renderSNDHWithLimit(data, sampleRate, 0, 1)
}

// renderSNDHWithLimit renders SNDH data with optional frame limit and subsong selection
func renderSNDHWithLimit(data []byte, sampleRate int, maxFrames int, subsong int) (PSGMetadata, []PSGEvent, uint64, uint32, uint16, bool, uint64, uint64, uint64, error) {
	// Parse SNDH file
	file, err := ParseSNDHData(data)
	if err != nil {
		return PSGMetadata{}, nil, 0, 0, 0, false, 0, 0, 0, fmt.Errorf("parse SNDH: %w", err)
	}

	// Validate subsong
	if subsong < 1 || subsong > file.Header.SubSongCount {
		subsong = file.Header.DefaultSong
	}

	// Create player
	player, err := newSNDH68KPlayer(file, subsong, sampleRate)
	if err != nil {
		return PSGMetadata{}, nil, 0, 0, 0, false, 0, 0, 0, fmt.Errorf("create player: %w", err)
	}

	// Determine frame count
	frameRate := player.frameRate
	duration := file.GetSubSongDuration(subsong)

	frameCount := 0
	loop := false
	loopSample := uint64(0)

	if duration > 0 {
		// Known duration
		frameCount = duration * int(frameRate)
	} else {
		// Unknown duration or loops infinitely
		frameCount = sndhDefaultLoopFrames
		loop = true
	}

	// Apply max frames limit
	if maxFrames > 0 && frameCount > maxFrames {
		frameCount = maxFrames
	}
	if frameCount > sndhMaxFrames {
		frameCount = sndhMaxFrames
	}

	// Render frames
	events, totalSamples := player.RenderFrames(frameCount)

	// Build metadata
	meta := PSGMetadata{
		Title:  file.Header.Title,
		Author: file.Header.Composer,
		System: "Atari ST",
	}

	clockHz := uint32(PSG_CLOCK_ATARI_ST)

	return meta, events, totalSamples, clockHz, frameRate, loop, loopSample, player.instructionCount, player.cpuExecNanos, nil
}

// SNDHSystemName returns the system name for SNDH files
func SNDHSystemName() string {
	return "Atari ST"
}
