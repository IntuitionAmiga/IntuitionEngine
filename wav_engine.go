package main

import (
	"math"
	"sync/atomic"
)

const wavTickerKey = "wav"

type wavState struct {
	left        []int16
	right       []int16
	phaseInc    float64
	loop        bool
	paused      bool
	channelBase int
	volumeL     uint8
	volumeR     uint8
	forceMono   bool
}

// WAVEngine streams parsed WAV frames through one or two SoundChip DAC channels.
type WAVEngine struct {
	sound      *SoundChip
	sampleRate int
	state      atomic.Pointer[wavState]
	playing    atomic.Bool
	enabled    atomic.Bool
	phaseBits  atomic.Uint64
}

// NewWAVEngine creates a new WAV engine bound to a SoundChip.
func NewWAVEngine(sound *SoundChip, sampleRate int) *WAVEngine {
	e := &WAVEngine{sound: sound, sampleRate: sampleRate}
	e.state.Store(&wavState{volumeL: 255, volumeR: 255, forceMono: true})
	return e
}

// LoadWAV loads a parsed WAV file and prepares for playback.
func (e *WAVEngine) LoadWAV(wav *WAVFile) {
	prev := e.snapshot()
	st := *prev
	st.left = append([]int16(nil), wav.LeftSamples...)
	st.right = append([]int16(nil), wav.RightSamples...)
	st.phaseInc = float64(wav.SampleRate) / float64(e.sampleRate)
	e.phaseBits.Store(math.Float64bits(0))
	e.enabled.Store(true)
	e.state.Store(&st)
}

// SetPlaying starts or stops WAV playback.
func (e *WAVEngine) SetPlaying(playing bool) {
	e.playing.Store(playing)
	if e.sound != nil {
		if playing {
			e.sound.RegisterSampleTicker(wavTickerKey, e)
		} else {
			e.sound.UnregisterSampleTicker(wavTickerKey)
			e.releaseChannels()
		}
	}
}

// IsPlaying returns the current playback state.
func (e *WAVEngine) IsPlaying() bool {
	return e.playing.Load()
}

// SetLoop enables or disables looping.
func (e *WAVEngine) SetLoop(loop bool) {
	e.updateState(func(st *wavState) { st.loop = loop })
}

func (e *WAVEngine) SetPaused(paused bool) {
	e.updateState(func(st *wavState) { st.paused = paused })
}

func (e *WAVEngine) IsPaused() bool {
	return e.snapshot().paused
}

func (e *WAVEngine) SetChannelBase(base int) {
	if base < 0 {
		base = 0
	}
	if base >= NUM_CHANNELS-1 {
		base = NUM_CHANNELS - 2
	}
	oldBase := e.snapshot().channelBase
	e.updateState(func(st *wavState) { st.channelBase = base })
	if oldBase != base && e.playing.Load() && e.sound != nil {
		e.sound.ReleaseDACMode(oldBase)
		e.sound.ReleaseDACMode(oldBase + 1)
	}
}

func (e *WAVEngine) ChannelBase() int {
	return e.snapshot().channelBase
}

func (e *WAVEngine) SetVolume(left, right uint8) {
	e.updateState(func(st *wavState) {
		st.volumeL = left
		st.volumeR = right
	})
}

func (e *WAVEngine) SetForceMono(force bool) {
	e.updateState(func(st *wavState) { st.forceMono = force })
}

func (e *WAVEngine) StereoActive() bool {
	st := e.snapshot()
	return !st.forceMono && len(st.right) > 0
}

// GetPosition returns the current source frame index.
func (e *WAVEngine) GetPosition() int {
	st := e.snapshot()
	if len(st.left) == 0 {
		return 0
	}
	pos := int(math.Floor(math.Float64frombits(e.phaseBits.Load())))
	if pos >= len(st.left) {
		if st.loop {
			return pos % len(st.left)
		}
		return len(st.left)
	}
	return pos
}

// Reset clears engine state.
func (e *WAVEngine) Reset() {
	e.SetPlaying(false)
	e.enabled.Store(false)
	e.phaseBits.Store(math.Float64bits(0))
	e.state.Store(&wavState{volumeL: 255, volumeR: 255, forceMono: true})
}

// TickSample advances WAV playback by one host sample.
func (e *WAVEngine) TickSample() {
	if !e.enabled.Load() || !e.playing.Load() {
		return
	}
	st := e.snapshot()
	if len(st.left) == 0 {
		e.SetPlaying(false)
		return
	}

	phase := math.Float64frombits(e.phaseBits.Load())
	pos := int(phase)
	for pos >= len(st.left) {
		if !st.loop {
			e.SetPlaying(false)
			return
		}
		phase -= float64(len(st.left))
		pos = int(phase)
	}

	left := st.left[pos]
	right := left
	if len(st.right) > pos {
		right = st.right[pos]
	}
	if st.forceMono {
		mono := int16((int(left) + int(right)) / 2)
		left, right = mono, mono
	}
	e.writeDAC(st.channelBase, left, st.volumeL)
	e.writeDAC(st.channelBase+1, right, st.volumeR)

	if !st.paused {
		phase += st.phaseInc
		e.phaseBits.Store(math.Float64bits(phase))
	}
}

func (e *WAVEngine) updateState(fn func(*wavState)) {
	prev := e.snapshot()
	next := *prev
	fn(&next)
	e.state.Store(&next)
}

func (e *WAVEngine) snapshot() *wavState {
	st := e.state.Load()
	if st == nil {
		return &wavState{volumeL: 255, volumeR: 255, forceMono: true}
	}
	return st
}

func (e *WAVEngine) writeDAC(ch int, sample int16, volume uint8) {
	if e.sound == nil || ch < 0 || ch >= NUM_CHANNELS {
		return
	}
	scaled := int(sample) * int(volume) / 255 / 256
	if scaled > 127 {
		scaled = 127
	} else if scaled < -128 {
		scaled = -128
	}
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+FLEX_OFF_VOL, 255)
	e.sound.HandleRegisterWrite(base+FLEX_OFF_CTRL, 3)
	e.sound.HandleRegisterWrite(base+FLEX_OFF_DAC, uint32(byte(int8(scaled))))
}

func (e *WAVEngine) releaseChannels() {
	if e.sound == nil {
		return
	}
	st := e.snapshot()
	e.sound.ReleaseDACMode(st.channelBase)
	e.sound.ReleaseDACMode(st.channelBase + 1)
}
