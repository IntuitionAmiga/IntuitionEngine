// ahx_player.go - High-level AHX player interface

package main

import (
	"os"
)

// AHXPlayer provides a high-level interface for AHX playback
type AHXPlayer struct {
	engine *AHXEngine
}

// NewAHXPlayer creates a new AHX player
func NewAHXPlayer(sound *SoundChip, sampleRate int) *AHXPlayer {
	return &AHXPlayer{
		engine: NewAHXEngine(sound, sampleRate),
	}
}

// Load loads AHX data into the player
func (p *AHXPlayer) Load(data []byte) error {
	return p.engine.LoadData(data)
}

// LoadFile loads an AHX file from disk
func (p *AHXPlayer) LoadFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return p.Load(data)
}

// Play starts playback
func (p *AHXPlayer) Play() {
	p.engine.SetPlaying(true)
}

// PlaySubsong starts playback of a specific subsong
func (p *AHXPlayer) PlaySubsong(nr int) {
	p.engine.replayer.InitSubsong(nr)
	p.engine.SetPlaying(true)
}

// Stop stops playback
func (p *AHXPlayer) Stop() {
	p.engine.SetPlaying(false)
}

// IsPlaying returns true if playing
func (p *AHXPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

// SetLoop enables/disables looping
func (p *AHXPlayer) SetLoop(loop bool) {
	p.engine.SetLoop(loop)
}

// TickSample advances playback by one sample (call from audio callback)
func (p *AHXPlayer) TickSample() {
	p.engine.TickSample()
}

// GetSongName returns the name of the loaded song
func (p *AHXPlayer) GetSongName() string {
	return p.engine.GetSongName()
}

// AHXMetadata contains metadata about the loaded AHX file
type AHXMetadata struct {
	Name string
}

// Metadata returns metadata about the loaded AHX file
func (p *AHXPlayer) Metadata() AHXMetadata {
	return AHXMetadata{
		Name: p.engine.GetSongName(),
	}
}

// GetSubsongCount returns the number of subsongs
func (p *AHXPlayer) GetSubsongCount() int {
	if p.engine.replayer.Song != nil {
		return p.engine.replayer.Song.SubsongNr
	}
	return 0
}

// GetInstrumentCount returns the number of instruments
func (p *AHXPlayer) GetInstrumentCount() int {
	if p.engine.replayer.Song != nil {
		return p.engine.replayer.Song.InstrumentNr
	}
	return 0
}

// GetInstrumentName returns the name of an instrument
func (p *AHXPlayer) GetInstrumentName(nr int) string {
	if p.engine.replayer.Song != nil && nr > 0 && nr <= p.engine.replayer.Song.InstrumentNr {
		return p.engine.replayer.Song.Instruments[nr].Name
	}
	return ""
}

// GetPosition returns the current position and note
func (p *AHXPlayer) GetPosition() (posNr, noteNr int) {
	return p.engine.GetPosition()
}

// GetPlayingTime returns the playing time in ticks
func (p *AHXPlayer) GetPlayingTime() int {
	return p.engine.GetPlayingTime()
}

// Reset resets the player state
func (p *AHXPlayer) Reset() {
	p.engine.Reset()
}
