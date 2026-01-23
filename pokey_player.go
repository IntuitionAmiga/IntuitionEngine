// pokey_player.go - POKEY/SAP file playback system
//
// Provides playback of Atari 8-bit SAP music files using the 6502 CPU
// and POKEY chip emulation. Similar to PSGPlayer but for Atari sound.

package main

import (
	"fmt"
	"os"
	"sync"
)

// POKEYPlayer handles SAP file playback
type POKEYPlayer struct {
	engine   *POKEYEngine
	metadata SAPMetadata

	mutex sync.RWMutex
}

// NewPOKEYPlayer creates a new POKEY player
func NewPOKEYPlayer(engine *POKEYEngine) *POKEYPlayer {
	return &POKEYPlayer{
		engine: engine,
	}
}

// Load loads a SAP file from disk
func (p *POKEYPlayer) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read SAP file: %w", err)
	}
	return p.LoadData(data)
}

// LoadData loads SAP data from memory
func (p *POKEYPlayer) LoadData(data []byte) error {
	return p.LoadDataWithSubsong(data, 0)
}

// LoadDataWithSubsong loads SAP data with a specific subsong
func (p *POKEYPlayer) LoadDataWithSubsong(data []byte, subsong int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Stop any current playback
	p.engine.StopPlayback()

	// Render SAP to POKEY events
	meta, events, totalSamples, clockHz, _, loop, loopSample, err := renderSAPWithLimit(data, SAMPLE_RATE, 0, subsong)
	if err != nil {
		return err
	}

	p.metadata = meta

	// Set POKEY clock and events
	p.engine.SetClockHz(clockHz)
	p.engine.SetEvents(events, totalSamples, loop, loopSample)

	return nil
}

// Play starts playback
func (p *POKEYPlayer) Play() {
	p.engine.SetPlaying(true)
}

// Stop stops playback
func (p *POKEYPlayer) Stop() {
	p.engine.StopPlayback()
}

// IsPlaying returns true if playback is active
func (p *POKEYPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

// Metadata returns the SAP file metadata
func (p *POKEYPlayer) Metadata() SAPMetadata {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.metadata
}

// DurationSeconds returns the duration in seconds
func (p *POKEYPlayer) DurationSeconds() float64 {
	p.engine.mutex.Lock()
	defer p.engine.mutex.Unlock()
	if p.engine.totalSamples == 0 {
		return 0
	}
	return float64(p.engine.totalSamples) / float64(SAMPLE_RATE)
}

// DurationText returns formatted duration string
func (p *POKEYPlayer) DurationText() string {
	dur := p.DurationSeconds()
	if dur <= 0 {
		return ""
	}
	minutes := int(dur) / 60
	seconds := int(dur) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
