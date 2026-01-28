// ahx_engine.go - AHX audio engine for SoundChip integration

package main

import (
	"sync"
)

// AHXEngine manages AHX playback through the SoundChip
type AHXEngine struct {
	mutex      sync.Mutex
	sound      *SoundChip
	sampleRate int
	channels   [4]int
	replayer   *AHXReplayer

	playing        bool
	currentSample  uint64
	loop           bool
	samplesPerTick int

	enabled        bool
	channelsInit   bool
	ahxPlusEnabled bool
}

// NewAHXEngine creates a new AHX engine
func NewAHXEngine(sound *SoundChip, sampleRate int) *AHXEngine {
	return &AHXEngine{
		sound:      sound,
		sampleRate: sampleRate,
		replayer:   NewAHXReplayer(),
		channels:   [4]int{0, 1, 2, 3},
	}
}

// LoadData parses and loads AHX data
func (e *AHXEngine) LoadData(data []byte) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	song, err := ParseAHX(data)
	if err != nil {
		return err
	}

	e.replayer.InitSong(song)
	e.replayer.InitSubsong(0)

	baseHz := 50 * song.SpeedMultiplier
	e.samplesPerTick = e.sampleRate / baseHz

	e.enabled = true
	e.currentSample = 0

	return nil
}

// SetPlaying starts or stops playback
func (e *AHXEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = playing
	e.replayer.Playing = playing
	if !playing {
		e.silenceChannels()
	}
}

// IsPlaying returns current playback state
func (e *AHXEngine) IsPlaying() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.playing
}

// SetLoop enables/disables looping
func (e *AHXEngine) SetLoop(loop bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.loop = loop
}

// TickSample advances playback by one sample
func (e *AHXEngine) TickSample() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.enabled || !e.playing {
		return
	}

	if e.samplesPerTick > 0 && e.currentSample%uint64(e.samplesPerTick) == 0 {
		e.replayer.PlayIRQ()
		e.updateChannels()

		// Debug
		tick := e.currentSample / uint64(e.samplesPerTick)
		if tick%50 == 0 {
			println("AHX tick", tick, "pos:", e.replayer.PosNr, "row:", e.replayer.NoteNr)
			for i := 0; i < 4; i++ {
				v := &e.replayer.Voices[i]
				if v.VoiceVolume > 0 {
					waveLen := 4 * (1 << uint(v.WaveLength))
					freq := 0.0
					if v.VoicePeriod > 0 {
						freq = AHXPeriod2Freq(v.VoicePeriod) / float64(waveLen)
					}
					println("  ch", i, ": vol=", v.VoiceVolume, "freq=", int(freq), "Hz wave=", v.Waveform, "flt=", v.FilterPos)
				}
			}
		}
	}

	e.currentSample++

	if e.replayer.SongEndReached {
		if e.loop {
			e.replayer.SongEndReached = false
		} else {
			e.playing = false
			e.silenceChannels()
		}
	}
}

// ensureChannelsInitialized sets up SoundChip channels
func (e *AHXEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	for i := 0; i < 4; i++ {
		ch := e.channels[i]
		// Simple setup - no envelope, just direct volume control
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
		e.writeChannel(ch, FLEX_OFF_DUTY, 0x40)
		e.writeChannel(ch, FLEX_OFF_ATK, 0)
		e.writeChannel(ch, FLEX_OFF_DEC, 0)
		e.writeChannel(ch, FLEX_OFF_SUS, 255)
		e.writeChannel(ch, FLEX_OFF_REL, 0)
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
		e.writeChannel(ch, FLEX_OFF_CTRL, 3) // Enable + gate
		e.sound.SetChannelEnvelopeMode(ch, false)
	}

	e.channelsInit = true
}

// updateChannels transfers voice state to SoundChip
func (e *AHXEngine) updateChannels() {
	if e.sound == nil {
		return
	}

	e.ensureChannelsInitialized()

	for i := 0; i < 4; i++ {
		voice := &e.replayer.Voices[i]
		ch := e.channels[i]

		// Volume (0-64 -> 0-255)
		vol := voice.VoiceVolume
		if vol < 0 {
			vol = 0
		}
		if vol > 64 {
			vol = 64
		}
		e.writeChannel(ch, FLEX_OFF_VOL, uint32(vol*4))

		// Frequency from period - must account for waveform length!
		// AHX period = sample playback rate, actual freq = rate / waveform_samples
		if voice.VoicePeriod > 0 {
			sampleRate := AHXPeriod2Freq(voice.VoicePeriod)
			// Waveform buffer size = 4 * (1 << WaveLength)
			waveLen := 4 * (1 << uint(voice.WaveLength))
			freq := sampleRate / float64(waveLen)
			if freq > 20000 {
				freq = 20000
			}
			if freq < 20 {
				freq = 20
			}
			e.writeChannel(ch, FLEX_OFF_FREQ, uint32(freq*256))
		}

		// Waveform
		var waveType int
		switch voice.Waveform {
		case 0:
			waveType = WAVE_TRIANGLE
		case 1:
			waveType = WAVE_SAWTOOTH
		case 2:
			waveType = WAVE_SQUARE
		case 3:
			waveType = WAVE_NOISE
		default:
			waveType = WAVE_SQUARE
		}
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, uint32(waveType))

		// AHX+ hardware PWM: map SquarePos to duty cycle for square waves
		if e.ahxPlusEnabled && voice.Waveform == 2 {
			// SquarePos range: 0-63, mirror for symmetric sweep
			squarePos := voice.SquarePos
			if squarePos > 0x20 {
				squarePos = 0x40 - squarePos
			}
			// Map to duty cycle range 0x08-0x80 (narrow to 50%)
			duty := squarePos * 4
			if duty < 0x08 {
				duty = 0x08
			}
			if duty > 0x80 {
				duty = 0x80
			}
			e.writeChannel(ch, FLEX_OFF_DUTY, uint32(duty))
		}

		// Keep gate on
		e.writeChannel(ch, FLEX_OFF_CTRL, 3)

		// Filter modulation - map FilterPos to lowpass filter
		// FilterPos range: 0-63, neutral at 32
		// Lower values = more filtered (darker sound)
		filterPos := voice.FilterPos
		if filterPos < 32 {
			// Enable lowpass filter, cutoff proportional to FilterPos
			// FilterPos 0 = very dark (low cutoff), 31 = almost neutral
			cutoff := float32(filterPos) / 32.0 // 0.0 to ~0.97
			// Add minimum cutoff to avoid completely muting
			cutoff = 0.1 + cutoff*0.85                   // Range: 0.1 to 0.95
			e.sound.SetChannelFilter(ch, 1, cutoff, 0.3) // 1 = lowpass, mild resonance
		} else {
			// FilterPos >= 32: disable filter (neutral or bright)
			e.sound.SetChannelFilter(ch, 0, 1.0, 0) // Filter off
		}
	}
}

// writeChannel writes a value to a SoundChip channel register
func (e *AHXEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

// silenceChannels mutes all AHX channels
func (e *AHXEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for i := 0; i < 4; i++ {
		e.writeChannel(e.channels[i], FLEX_OFF_VOL, 0)
	}
}

// Reset resets the engine state
func (e *AHXEngine) Reset() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = false
	e.enabled = false
	e.currentSample = 0
	e.silenceChannels()
}

// SetAHXPlusEnabled enables/disables AHX+ enhanced mode
func (e *AHXEngine) SetAHXPlusEnabled(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.ahxPlusEnabled = enabled
	if e.sound != nil {
		e.sound.SetAHXPlusEnabled(enabled)
	}
}

// AHXPlusEnabled returns whether AHX+ mode is enabled
func (e *AHXEngine) AHXPlusEnabled() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.ahxPlusEnabled
}

// GetSongName returns the loaded song name
func (e *AHXEngine) GetSongName() string {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.replayer.Song != nil {
		return e.replayer.Song.Name
	}
	return ""
}

// GetPlayingTime returns the current playing time in ticks
func (e *AHXEngine) GetPlayingTime() int {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.replayer.PlayingTime
}

// GetPosition returns the current position in the song
func (e *AHXEngine) GetPosition() (posNr, noteNr int) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.replayer.PosNr, e.replayer.NoteNr
}
