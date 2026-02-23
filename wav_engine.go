package main

import (
	"sync"
	"sync/atomic"
)

// WAVEngine streams pre-parsed float32 samples through a SoundChip DAC channel.
// It implements the SampleTicker interface for per-sample callbacks at 44.1kHz.
type WAVEngine struct {
	mu          sync.Mutex
	sound       *SoundChip
	sampleRate  int
	playing     atomic.Bool
	enabled     atomic.Bool
	samples     []float32
	sourceRate  uint32
	phase       float64
	phaseInc    float64
	loop        bool
	channelInit bool
}

// NewWAVEngine creates a new WAV engine bound to a SoundChip.
func NewWAVEngine(sound *SoundChip, sampleRate int) *WAVEngine {
	return &WAVEngine{
		sound:      sound,
		sampleRate: sampleRate,
	}
}

// LoadWAV loads a parsed WAV file and prepares for playback.
func (e *WAVEngine) LoadWAV(wav *WAVFile) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.samples = wav.Samples
	e.sourceRate = wav.SampleRate
	e.phase = 0
	e.phaseInc = float64(wav.SampleRate) / float64(e.sampleRate)
	e.enabled.Store(true)
}

// SetPlaying starts or stops WAV playback.
func (e *WAVEngine) SetPlaying(playing bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.playing.Store(playing)
	if !playing {
		e.silenceChannel()
	}
}

// IsPlaying returns the current playback state.
func (e *WAVEngine) IsPlaying() bool {
	return e.playing.Load()
}

// SetLoop enables or disables looping.
func (e *WAVEngine) SetLoop(loop bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loop = loop
}

// GetPosition returns the current playback position in samples.
func (e *WAVEngine) GetPosition() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return int(e.phase)
}

// Reset clears engine state.
func (e *WAVEngine) Reset() {
	e.playing.Store(false)
	e.enabled.Store(false)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.samples = nil
	e.phase = 0
	e.phaseInc = 0
	e.loop = false
	e.channelInit = false
	e.silenceChannel()
}

// TickSample advances WAV playback by one audio sample.
// Called at 44.1kHz from SoundChip.ReadSample().
func (e *WAVEngine) TickSample() {
	if !e.enabled.Load() || !e.playing.Load() {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.samples) == 0 {
		return
	}

	e.ensureChannelInitialized()

	// Get current position
	pos := int(e.phase)
	if pos >= len(e.samples) {
		if e.loop {
			e.phase = 0
			pos = 0
		} else {
			e.playing.Store(false)
			e.silenceChannel()
			return
		}
	}

	// Linear interpolation between adjacent samples
	frac := float32(e.phase - float64(pos))
	sample := e.samples[pos]
	if pos+1 < len(e.samples) {
		sample += frac * (e.samples[pos+1] - sample)
	}

	// Scale to int8 range and write to DAC
	scaled := int(sample * 127.0)
	if scaled > 127 {
		scaled = 127
	} else if scaled < -128 {
		scaled = -128
	}
	e.writeChannel(0, FLEX_OFF_DAC, uint32(byte(int8(scaled))))

	// Advance phase
	e.phase += e.phaseInc
}

// ensureChannelInitialized configures SoundChip channel 0 for DAC mode.
func (e *WAVEngine) ensureChannelInitialized() {
	if e.channelInit || e.sound == nil {
		return
	}

	e.writeChannel(0, FLEX_OFF_VOL, 255) // Full gain
	e.writeChannel(0, FLEX_OFF_CTRL, 3)  // Enable + gate
	e.writeChannel(0, FLEX_OFF_DAC, 0)   // Enter DAC mode, silence

	e.channelInit = true
}

// silenceChannel writes zero to the DAC channel.
func (e *WAVEngine) silenceChannel() {
	if e.sound == nil {
		return
	}
	e.writeChannel(0, FLEX_OFF_DAC, 0)
}

// writeChannel writes a value to a SoundChip channel register.
func (e *WAVEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}
