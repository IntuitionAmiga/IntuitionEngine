package main

import (
	"math"
	"sync"
	"sync/atomic"
)

// MODEngine manages ProTracker MOD playback through the SoundChip.
// It implements the SampleTicker interface for per-sample callbacks.
//
// Playback state owned by the audio path (replayer, channel state, filters,
// currentSample, tickAccumQ, samplesPerTickQ) is mutated only while playback is
// stopped or under e.mu. Control-plane mutators use stop-and-swap so TickSample
// never observes partially replaced state.
type MODEngine struct {
	mu         sync.Mutex
	sound      *SoundChip
	sampleRate int
	channels   [modChannels]int // SoundChip channel indices
	replayer   *MODReplayer

	playing         atomic.Bool
	enabled         atomic.Bool
	currentSample   uint64
	samplesPerTick  int
	samplesPerTickQ uint64
	tickAccumQ      uint64
	loop            bool
	channelsInit    bool

	// Per-channel playback state
	chState [modChannels]modEngineChannel

	// Amiga filter emulation
	filterModel int // 0=none, 1=A500, 2=A1200
	filters     [modChannels]amigaFilter
	ledFilter   bool

	// Pre-computed filter coefficients
	rcAlpha             float32 // RC fixed output filter coefficient
	ledB0, ledB1, ledB2 float32 // LED biquad numerator
	ledA1, ledA2        float32 // LED biquad denominator

	config atomic.Pointer[engineConfig]
}

type engineConfig struct {
	filterModel int
	loop        bool
	ledFilter   bool
}

// modEngineChannel tracks per-channel sample playback state.
type modEngineChannel struct {
	phase    float64
	phaseInc float64
}

// amigaFilter holds per-channel Amiga filter state.
type amigaFilter struct {
	rcState float32 // RC fixed filter state
	ledX1   float32 // LED biquad input history
	ledX2   float32
	ledY1   float32 // LED biquad output history
	ledY2   float32
}

// NewMODEngine creates a new MOD engine bound to a SoundChip.
func NewMODEngine(sound *SoundChip, sampleRate int) *MODEngine {
	e := &MODEngine{
		sound:      sound,
		sampleRate: sampleRate,
		channels:   [modChannels]int{0, 1, 2, 3},
	}
	e.publishConfigLocked()
	return e
}

// LoadMOD loads a parsed MOD file and prepares for playback.
func (e *MODEngine) LoadMOD(mod *MODFile) {
	wasPlaying := e.playing.Swap(false)
	e.mu.Lock()

	e.replayer = NewMODReplayerWithSampleRate(mod, e.sampleRate)
	e.samplesPerTick = SamplesPerTick(e.sampleRate, modDefaultBPM)
	e.samplesPerTickQ = samplesPerTickQ(e.sampleRate, modDefaultBPM)
	e.tickAccumQ = e.samplesPerTickQ
	e.currentSample = 0
	e.enabled.Store(true)

	// Reset channel state
	for i := range modChannels {
		e.chState[i] = modEngineChannel{}
		e.filters[i] = amigaFilter{}
	}
	e.publishConfigLocked()
	e.mu.Unlock()
	if wasPlaying {
		e.playing.Store(true)
	}
}

// SetPlaying starts or stops MOD playback.
func (e *MODEngine) SetPlaying(playing bool) {
	if playing {
		e.playing.Store(true)
		return
	}
	e.playing.Store(false)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.silenceChannels()
}

// IsPlaying returns the current playback state.
func (e *MODEngine) IsPlaying() bool {
	return e.playing.Load()
}

// SetLoop enables or disables song looping.
func (e *MODEngine) SetLoop(loop bool) {
	wasPlaying := e.playing.Swap(false)
	e.mu.Lock()
	e.loop = loop
	e.publishConfigLocked()
	e.mu.Unlock()
	if wasPlaying {
		e.playing.Store(true)
	}
}

// SetFilterModel sets the Amiga output filter model.
// 0=none, 1=A500 (4.5kHz), 2=A1200 (28kHz)
func (e *MODEngine) SetFilterModel(model int) {
	wasPlaying := e.playing.Swap(false)
	e.mu.Lock()
	e.filterModel = model
	e.computeFilterCoefficients()
	e.publishConfigLocked()
	e.mu.Unlock()
	if wasPlaying {
		e.playing.Store(true)
	}
}

func (e *MODEngine) GetFilterModel() int {
	return e.configSnapshot().filterModel
}

// GetPosition returns the current song position.
func (e *MODEngine) GetPosition() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.replayer == nil {
		return 0
	}
	return e.replayer.position
}

// TickSample advances MOD playback by one audio sample.
// Called at 44.1kHz from SoundChip.ReadSample().
func (e *MODEngine) TickSample() {
	if !e.enabled.Load() || !e.playing.Load() {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.playing.Load() {
		return
	}
	if e.replayer == nil {
		return
	}

	e.tickAccumQ += 1 << 32
	if e.samplesPerTickQ > 0 && e.tickAccumQ >= e.samplesPerTickQ {
		e.tickAccumQ -= e.samplesPerTickQ
		e.replayer.ProcessTick()
		// Update samples per tick if BPM changed
		newSPT := SamplesPerTick(e.sampleRate, e.replayer.bpm)
		if newSPT != e.samplesPerTick {
			e.samplesPerTick = newSPT
		}
		e.samplesPerTickQ = samplesPerTickQ(e.sampleRate, e.replayer.bpm)
		// Sync LED filter state from replayer
		e.ledFilter = e.replayer.ledFilter
		e.publishConfigLocked()
	}

	e.currentSample++

	// Update DAC values for each channel
	e.ensureChannelsInitialized()
	e.updateDAC()

	// Handle song end
	if e.replayer.songEnd {
		if e.configSnapshot().loop {
			e.replayer.songEnd = false
			restart := e.replayer.mod.RestartPos
			if restart < 0 || restart >= e.replayer.mod.SongLength {
				restart = 0
			}
			e.replayer.position = restart
			e.replayer.row = 0
			e.replayer.tick = 0
		} else {
			e.playing.Store(false)
			e.silenceChannels()
		}
	}
}

func (e *MODEngine) configSnapshot() *engineConfig {
	if cfg := e.config.Load(); cfg != nil {
		return cfg
	}
	return &engineConfig{}
}

func (e *MODEngine) publishConfigLocked() {
	e.config.Store(&engineConfig{
		filterModel: e.filterModel,
		loop:        e.loop,
		ledFilter:   e.ledFilter,
	})
}

func samplesPerTickQ(sampleRate, bpm int) uint64 {
	if bpm <= 0 {
		return 0
	}
	return (uint64(sampleRate) * 5 << 32) / uint64(bpm*2)
}

// ensureChannelsInitialized configures SoundChip channels for DAC mode.
func (e *MODEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	for i := range modChannels {
		ch := e.channels[i]
		e.writeChannel(ch, FLEX_OFF_VOL, 255) // Full gain
		e.writeChannel(ch, FLEX_OFF_CTRL, 3)  // Enable + gate
		e.writeChannel(ch, FLEX_OFF_DAC, 0)   // Enter DAC mode, silence
	}

	e.channelsInit = true
}

// updateDAC reads sample data for each channel and writes to SoundChip DAC registers.
func (e *MODEngine) updateDAC() {
	if e.sound == nil {
		return
	}
	cfg := e.configSnapshot()

	sums := [modChannels]int{}
	counts := [modChannels]int{}
	active := [modChannels]bool{}

	for i := range e.replayer.channels {
		mc := &e.replayer.channels[i]
		out := i % modChannels
		counts[out]++

		if !mc.active || mc.sample == nil || len(mc.sample.Data) == 0 || mc.period == 0 {
			continue
		}

		// Read sample byte
		sampleByte, sampleActive := mc.ReadSample()
		if !sampleActive {
			continue
		}

		// Scale by ProTracker volume (0-64)
		scaledVol := clampVolume(mc.volume + mc.tremoloDelta)
		scaled := int(sampleByte) * scaledVol / 64

		// Apply Amiga filter if model is set
		if cfg.filterModel != 0 {
			scaledF := float32(scaled) / 128.0
			scaledF = e.applyAmigaFilter(out, scaledF)
			scaled = int(clampF32(scaledF*128.0, -128.0, 127.0))
		}
		sums[out] += scaled
		active[out] = true
	}

	for i := range modChannels {
		scaled := 0
		if active[i] && counts[i] > 0 {
			scaled = sums[i] / counts[i]
		}
		// Clamp to int8 range and write to DAC
		if scaled > 127 {
			scaled = 127
		} else if scaled < -128 {
			scaled = -128
		}
		e.writeChannel(e.channels[i], FLEX_OFF_DAC, uint32(byte(int8(scaled))))
	}
}

// applyAmigaFilter applies the selected Amiga filter chain to a sample.
func (e *MODEngine) applyAmigaFilter(ch int, sample float32) float32 {
	f := &e.filters[ch]

	// Fixed output filter (RC low-pass)
	if e.rcAlpha > 0 {
		f.rcState += e.rcAlpha * (sample - f.rcState)
		sample = f.rcState
	}

	// LED filter (2-pole Butterworth, toggled by E0x effect)
	if e.configSnapshot().ledFilter || e.ledFilter {
		out := e.ledB0*sample + e.ledB1*f.ledX1 + e.ledB2*f.ledX2 - e.ledA1*f.ledY1 - e.ledA2*f.ledY2
		f.ledX2 = f.ledX1
		f.ledX1 = sample
		f.ledY2 = f.ledY1
		f.ledY1 = out
		sample = out
	}

	return sample
}

// computeFilterCoefficients pre-computes filter coefficients for the selected model.
func (e *MODEngine) computeFilterCoefficients() {
	sr := float64(e.sampleRate)

	switch e.filterModel {
	case 1: // A500: fc ≈ 4500 Hz
		w := 2.0 * math.Pi * 4500.0 / sr
		e.rcAlpha = float32(w / (1.0 + w))
	case 2: // A1200: fc ≈ 28000 Hz
		w := 2.0 * math.Pi * 28000.0 / sr
		e.rcAlpha = float32(w / (1.0 + w))
	default:
		e.rcAlpha = 0
	}

	// LED filter: 2-pole Butterworth at ~3275 Hz
	fc := 3275.0
	w0 := 2.0 * math.Pi * fc / sr
	cosW0 := math.Cos(w0)
	sinW0 := math.Sin(w0)
	alpha := sinW0 / (2.0 * math.Sqrt2) // Q = 1/sqrt(2) for Butterworth

	a0 := 1.0 + alpha
	e.ledB0 = float32((1.0 - cosW0) / 2.0 / a0)
	e.ledB1 = float32((1.0 - cosW0) / a0)
	e.ledB2 = e.ledB0
	e.ledA1 = float32(-2.0 * cosW0 / a0)
	e.ledA2 = float32((1.0 - alpha) / a0)
}

// silenceChannels writes zero to all DAC channels.
func (e *MODEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for i := range modChannels {
		e.writeChannel(e.channels[i], FLEX_OFF_DAC, 0)
	}
}

// writeChannel writes a value to a SoundChip channel register.
func (e *MODEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	if addr, ok := flexAddrForChannel(ch, offset); ok {
		e.sound.HandleRegisterWrite(addr, value)
	}
}
