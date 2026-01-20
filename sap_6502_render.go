// sap_6502_render.go - Entry point for SAP rendering to POKEY events.
//
// This provides a unified interface for SAP playback using the 6502 CPU
// and POKEY chip emulation. POKEY is NOT PSG - they are different chips.

package main

import (
	"fmt"
)

const (
	// Default frame count when duration is unknown
	sapDefaultFrames = 15000

	// Maximum frame count to prevent runaway rendering
	sapMaxFrames = 600000
)

// SAPMetadata contains metadata from a SAP file
type SAPMetadata struct {
	Title  string
	Author string
	Date   string
	Songs  int
	Stereo bool
	NTSC   bool
}

// SAPRenderResult contains the results of rendering a SAP file
type SAPRenderResult struct {
	Metadata     SAPMetadata
	Events       []SAPPOKEYEvent
	TotalSamples uint64
	ClockHz      uint32
	FrameRate    uint16
	Loop         bool
	LoopSample   uint64
	Stereo       bool
}

// renderSAP renders SAP data to POKEY events
func renderSAP(data []byte, sampleRate int) (SAPMetadata, []SAPPOKEYEvent, uint64, uint32, uint16, bool, uint64, error) {
	return renderSAPWithLimit(data, sampleRate, 0, 0)
}

// renderSAPWithLimit renders SAP data with optional frame limit and subsong selection
func renderSAPWithLimit(data []byte, sampleRate int, maxFrames int, subsong int) (SAPMetadata, []SAPPOKEYEvent, uint64, uint32, uint16, bool, uint64, error) {
	// Parse SAP file
	file, err := ParseSAPData(data)
	if err != nil {
		return SAPMetadata{}, nil, 0, 0, 0, false, 0, fmt.Errorf("parse SAP: %w", err)
	}

	// Validate subsong
	if subsong < 0 || subsong >= file.Header.Songs {
		subsong = file.Header.DefSong
	}

	// Create player
	player, err := newSAP6502Player(file, subsong, sampleRate)
	if err != nil {
		return SAPMetadata{}, nil, 0, 0, 0, false, 0, fmt.Errorf("create SAP player: %w", err)
	}

	// Build metadata
	meta := SAPMetadata{
		Title:  file.Header.Name,
		Author: file.Header.Author,
		Date:   file.Header.Date,
		Songs:  file.Header.Songs,
		Stereo: file.Header.Stereo,
		NTSC:   file.Header.NTSC,
	}

	// Calculate frame rate
	// Frame rate = clockHz / (scanlinesPerFrame * cyclesPerScanline)
	clockHz := player.GetClockHz()
	cyclesPerFrame := player.scanlinesPerFrame * atariCyclesPerScanline
	frameRateFloat := float64(clockHz) / float64(cyclesPerFrame)
	frameRate := uint16(frameRateFloat + 0.5)

	// Determine frame count
	frameCount := maxFrames
	loop := false
	loopSample := uint64(0)

	if frameCount <= 0 {
		// Check for duration from TIME tag
		if subsong < len(file.Header.Durations) && file.Header.Durations[subsong] > 0 {
			duration := file.Header.Durations[subsong]
			frameCount = int(duration * frameRateFloat)

			// Check for loop flag
			if subsong < len(file.Header.LoopFlags) && file.Header.LoopFlags[subsong] {
				loop = true
			}
		} else {
			// Default duration
			frameCount = sapDefaultFrames
			loop = true // Assume loops when duration unknown
		}
	}

	// Cap frame count
	if frameCount > sapMaxFrames {
		frameCount = sapMaxFrames
	}

	// Render frames - returns native POKEY events
	pokeyEvents, totalSamples := player.RenderFrames(frameCount)

	// Calculate loop sample if looping
	if loop && frameCount > 0 {
		loopSample = 0
	}

	return meta, pokeyEvents, totalSamples, clockHz, frameRate, loop, loopSample, nil
}
