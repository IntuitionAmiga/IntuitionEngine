// ahx_replayer.go - AHX replayer (tick-by-tick playback)
// Reference: AHX-Sources/AHX.cpp

package main

// Pre-computed constants for AHX waveform processing
const (
	// filterBlockSize is the stride between filter positions in the waveform buffer
	// Formula: 0xfc + 0xfc + 0x80*0x1f + 0x80 + 0x280*3 = 252 + 252 + 3968 + 128 + 1920 = 6520
	ahxFilterBlockSize = 0xfc + 0xfc + 0x80*0x1f + 0x80 + 0x280*3
)

// ahxWaveLengthOffsets maps WaveLength (0-5) to cumulative offsets within a filter block
// These are the byte offsets for triangle/sawtooth waveforms of different lengths:
// WaveLength 0 = 4 bytes, 1 = 8 bytes, 2 = 16 bytes, 3 = 32 bytes, 4 = 64 bytes, 5 = 128 bytes
var ahxWaveLengthOffsets = [6]int{
	0x00,                             // WaveLength 0: offset 0
	0x04,                             // WaveLength 1: offset 4
	0x04 + 0x08,                      // WaveLength 2: offset 12
	0x04 + 0x08 + 0x10,               // WaveLength 3: offset 28
	0x04 + 0x08 + 0x10 + 0x20,        // WaveLength 4: offset 60
	0x04 + 0x08 + 0x10 + 0x20 + 0x40, // WaveLength 5: offset 124
}

// AHXVoice represents the state of a single AHX voice
type AHXVoice struct {
	// Output (read by mixer)
	VoiceVolume int
	VoicePeriod int
	VoiceBuffer [0x281]int8 // Extra byte for interpolation

	// Track state
	Track, Transpose         int
	NextTrack, NextTranspose int
	TrackOn                  bool
	TrackMasterVolume        int

	// Envelope
	ADSRVolume int // Fixed point 8:8
	ADSR       AHXVoiceEnvelope
	Instrument *AHXInstrument

	// Period/Note
	InstrPeriod, TrackPeriod, VibratoPeriod int
	FixedNote                               int
	PlantPeriod                             bool

	// Volume
	NoteMaxVolume, PerfSubVolume   int
	VolumeSlideUp, VolumeSlideDown int

	// Hard cut
	HardCut, HardCutRelease, HardCutReleaseF int

	// Portamento
	PeriodSlideSpeed, PeriodSlidePeriod, PeriodSlideLimit int
	PeriodSlideOn, PeriodSlideWithLimit                   bool
	PeriodPerfSlideSpeed, PeriodPerfSlidePeriod           int
	PeriodPerfSlideOn                                     bool

	// Vibrato
	VibratoDelay, VibratoCurrent, VibratoDepth, VibratoSpeed int

	// Square modulation
	SquareOn, SquareInit, SquareWait              int
	SquareLowerLimit, SquareUpperLimit, SquarePos int
	SquareSign, SquareSlidingIn, SquareReverse    int
	IgnoreSquare, PlantSquare                     int
	SquareTempBuffer                              [0x80]int8

	// Filter modulation
	FilterOn, FilterInit, FilterWait                       int
	FilterLowerLimit, FilterUpperLimit, FilterPos          int
	FilterSign, FilterSpeed, FilterSlidingIn, IgnoreFilter int

	// Playlist
	PerfCurrent, PerfSpeed, PerfWait int
	PerfList                         *AHXPList

	// Note delay/cut
	NoteDelayWait, NoteDelayOn int
	NoteCutWait, NoteCutOn     int

	// Waveform
	WaveLength, Waveform, NewWaveform int
	AudioSource                       []int8
	AudioPeriod, AudioVolume          int
}

// AHXVoiceEnvelope holds the calculated envelope deltas
type AHXVoiceEnvelope struct {
	AFrames, AVolume int // Attack frames and delta
	DFrames, DVolume int // Decay frames and delta
	SFrames          int // Sustain frames
	RFrames, RVolume int // Release frames and delta
}

// Init initializes a voice to default state
func (v *AHXVoice) Init() {
	*v = AHXVoice{
		TrackOn:           true,
		TrackMasterVolume: 0x40,
	}
}

// CalcADSR calculates ADSR deltas from the instrument envelope
func (v *AHXVoice) CalcADSR() {
	if v.Instrument == nil {
		return
	}
	env := &v.Instrument.Envelope

	v.ADSR.AFrames = env.AFrames
	if v.ADSR.AFrames > 0 {
		v.ADSR.AVolume = env.AVolume * 256 / v.ADSR.AFrames
	}

	v.ADSR.DFrames = env.DFrames
	if v.ADSR.DFrames > 0 {
		v.ADSR.DVolume = (env.DVolume - env.AVolume) * 256 / v.ADSR.DFrames
	}

	v.ADSR.SFrames = env.SFrames

	v.ADSR.RFrames = env.RFrames
	if v.ADSR.RFrames > 0 {
		v.ADSR.RVolume = (env.RVolume - env.DVolume) * 256 / v.ADSR.RFrames
	}
}

// AHXReplayer is the main replayer engine
type AHXReplayer struct {
	Song   *AHXFile
	Waves  *AHXWaves
	Voices [4]AHXVoice

	// Position
	PosNr, NoteNr        int
	PosJump, PosJumpNote int
	PatternBreak         bool
	GetNewPosition       bool
	SongEndReached       bool

	// Timing
	Tempo, StepWaitFrames int
	PlayingTime           int
	MainVolume            int
	Playing               bool

	WaveformTab [4][]int8
	WNRandom    int
}

// NewAHXReplayer creates a new replayer with initialized waves
func NewAHXReplayer() *AHXReplayer {
	r := &AHXReplayer{
		Waves: NewAHXWaves(),
	}
	// Initialize waveform table pointers
	r.WaveformTab[0] = r.Waves.Triangle04[:]
	r.WaveformTab[1] = r.Waves.Sawtooth04[:]
	r.WaveformTab[3] = r.Waves.WhiteNoiseBig[:]
	return r
}

// InitSong initializes the replayer with a song
func (r *AHXReplayer) InitSong(song *AHXFile) {
	r.Song = song
}

// InitSubsong initializes a specific subsong (0 = main song)
func (r *AHXReplayer) InitSubsong(nr int) error {
	if r.Song == nil {
		return nil
	}

	if nr > r.Song.SubsongNr {
		return nil
	}

	if nr == 0 {
		r.PosNr = 0
	} else {
		r.PosNr = r.Song.Subsongs[nr-1]
	}

	r.PosJump = 0
	r.PatternBreak = false
	r.MainVolume = 0x40
	r.Playing = true
	r.NoteNr = 0
	r.PosJumpNote = 0
	r.Tempo = 6
	r.StepWaitFrames = 0
	r.GetNewPosition = true
	r.SongEndReached = false
	r.PlayingTime = 0

	for v := range 4 {
		r.Voices[v].Init()
	}

	return nil
}

// PlayIRQ is called once per tick (50-200 Hz depending on speed multiplier)
func (r *AHXReplayer) PlayIRQ() {
	if r.Song == nil || !r.Playing {
		return
	}

	if r.StepWaitFrames <= 0 {
		if r.GetNewPosition {
			nextPos := r.PosNr + 1
			if nextPos >= r.Song.PositionNr {
				nextPos = 0
			}
			for i := range 4 {
				r.Voices[i].Track = r.Song.Positions[r.PosNr].Track[i]
				r.Voices[i].Transpose = int(r.Song.Positions[r.PosNr].Transpose[i])
				r.Voices[i].NextTrack = r.Song.Positions[nextPos].Track[i]
				r.Voices[i].NextTranspose = int(r.Song.Positions[nextPos].Transpose[i])
			}
			r.GetNewPosition = false
		}
		for i := range 4 {
			r.ProcessStep(i)
		}
		r.StepWaitFrames = r.Tempo
	}

	// Process frame effects
	for i := range 4 {
		r.ProcessFrame(i)
	}
	r.PlayingTime++

	if r.Tempo > 0 {
		r.StepWaitFrames--
		if r.StepWaitFrames <= 0 {
			if !r.PatternBreak {
				r.NoteNr++
				if r.NoteNr >= r.Song.TrackLength {
					r.PosJump = r.PosNr + 1
					r.PosJumpNote = 0
					r.PatternBreak = true
				}
			}
			if r.PatternBreak {
				r.PatternBreak = false
				r.NoteNr = r.PosJumpNote
				r.PosJumpNote = 0
				r.PosNr = r.PosJump
				r.PosJump = 0
				if r.PosNr >= r.Song.PositionNr {
					r.SongEndReached = true
					r.PosNr = r.Song.Restart
				}
				r.GetNewPosition = true
			}
		}
	}

	// Set audio output for all voices
	for a := range 4 {
		r.SetAudio(a)
	}
}

// ProcessStep handles a new row for a voice
func (r *AHXReplayer) ProcessStep(v int) {
	voice := &r.Voices[v]
	if !voice.TrackOn {
		return
	}

	voice.VolumeSlideUp = 0
	voice.VolumeSlideDown = 0

	track := voice.Track
	if track >= len(r.Song.Tracks) {
		return
	}
	if r.NoteNr >= len(r.Song.Tracks[track]) {
		return
	}

	step := &r.Song.Tracks[track][r.NoteNr]
	note := step.Note
	instrument := step.Instrument
	fx := step.FX
	fxParam := step.FXParam

	// Process effects that happen before instrument setup
	switch fx {
	case 0x0: // Position Jump HI
		if (fxParam&0xF) > 0 && (fxParam&0xF) <= 9 {
			r.PosJump = fxParam & 0xF
		}
	case 0x5, 0xA: // Volume Slide (+ Tone Portamento)
		voice.VolumeSlideDown = fxParam & 0x0F
		voice.VolumeSlideUp = fxParam >> 4
	case 0xB: // Position Jump
		r.PosJump = r.PosJump*100 + (fxParam & 0x0F) + (fxParam>>4)*10
		r.PatternBreak = true
	case 0xD: // Pattern Break
		r.PosJump = r.PosNr + 1
		r.PosJumpNote = (fxParam & 0x0F) + (fxParam>>4)*10
		if r.PosJumpNote > r.Song.TrackLength {
			r.PosJumpNote = 0
		}
		r.PatternBreak = true
	case 0xE: // Enhanced commands
		switch fxParam >> 4 {
		case 0xC: // Note Cut
			if (fxParam & 0x0F) < r.Tempo {
				voice.NoteCutWait = fxParam & 0x0F
				if voice.NoteCutWait != 0 {
					voice.NoteCutOn = 1
					voice.HardCutRelease = 0
				}
			}
		case 0xD: // Note Delay
			if voice.NoteDelayOn != 0 {
				voice.NoteDelayOn = 0
			} else {
				if (fxParam & 0x0F) < r.Tempo {
					voice.NoteDelayWait = fxParam & 0x0F
					if voice.NoteDelayWait != 0 {
						voice.NoteDelayOn = 1
						return
					}
				}
			}
		}
	case 0xF: // Speed
		if fxParam == 0 {
			// Speed 0 = halt/end song (used by composers to signal end)
			r.SongEndReached = true
		} else {
			r.Tempo = fxParam
		}
	}

	if instrument != 0 && instrument <= r.Song.InstrumentNr {
		voice.PerfSubVolume = 0x40
		voice.PeriodSlideSpeed = 0
		voice.PeriodSlidePeriod = 0
		voice.PeriodSlideLimit = 0
		voice.ADSRVolume = 0
		voice.Instrument = &r.Song.Instruments[instrument]
		voice.CalcADSR()

		// Init on instrument
		voice.WaveLength = voice.Instrument.WaveLength
		voice.NoteMaxVolume = voice.Instrument.Volume

		// Init vibrato
		voice.VibratoCurrent = 0
		voice.VibratoDelay = voice.Instrument.VibratoDelay
		voice.VibratoDepth = voice.Instrument.VibratoDepth
		voice.VibratoSpeed = voice.Instrument.VibratoSpeed
		voice.VibratoPeriod = 0

		// Init hard cut
		voice.HardCutRelease = voice.Instrument.HardCutRelease
		voice.HardCut = voice.Instrument.HardCutReleaseFrames

		// Init square
		voice.IgnoreSquare = 0
		voice.SquareSlidingIn = 0
		voice.SquareWait = 0
		voice.SquareOn = 0
		squareLower := voice.Instrument.SquareLowerLimit >> (5 - voice.WaveLength)
		squareUpper := voice.Instrument.SquareUpperLimit >> (5 - voice.WaveLength)
		if squareUpper < squareLower {
			squareLower, squareUpper = squareUpper, squareLower
		}
		voice.SquareUpperLimit = squareUpper
		voice.SquareLowerLimit = squareLower

		// Init filter
		voice.IgnoreFilter = 0
		voice.FilterWait = 0
		voice.FilterOn = 0
		voice.FilterSlidingIn = 0
		d6 := voice.Instrument.FilterSpeed
		d3 := voice.Instrument.FilterLowerLimit
		d4 := voice.Instrument.FilterUpperLimit
		if d3&0x80 != 0 {
			d6 |= 0x20
		}
		if d4&0x80 != 0 {
			d6 |= 0x40
		}
		voice.FilterSpeed = d6
		d3 &= ^0x80
		d4 &= ^0x80
		if d3 > d4 {
			d3, d4 = d4, d3
		}
		voice.FilterUpperLimit = d4
		voice.FilterLowerLimit = d3
		voice.FilterPos = 32

		// Init PerfList
		voice.PerfWait = 0
		voice.PerfCurrent = 0
		voice.PerfSpeed = voice.Instrument.PList.Speed
		voice.PerfList = &voice.Instrument.PList
	}

	// No instrument
	voice.PeriodSlideOn = false

	switch fx {
	case 0x4: // Override filter
		// Handled in PList
	case 0x9: // Set Square Offset
		voice.SquarePos = fxParam >> (5 - voice.WaveLength)
		voice.PlantSquare = 1
		voice.IgnoreSquare = 1
	case 0x5, 0x3: // Tone Portamento (+ Volume Slide)
		if fxParam != 0 {
			voice.PeriodSlideSpeed = fxParam
		}
		if note != 0 {
			newPeriod := AHXPeriodTable[note]
			oldPeriod := AHXPeriodTable[voice.TrackPeriod]
			diff := oldPeriod - newPeriod
			newLimit := diff + voice.PeriodSlidePeriod
			if newLimit != 0 {
				voice.PeriodSlideLimit = -diff
			}
		}
		voice.PeriodSlideOn = true
		voice.PeriodSlideWithLimit = true
		note = 0 // Don't trigger note
	}

	// Trigger note
	if note != 0 {
		voice.TrackPeriod = note
		voice.PlantPeriod = true
	}

	// Post-note effects
	switch fx {
	case 0x1: // Portamento up (period slide down)
		voice.PeriodSlideSpeed = -fxParam
		voice.PeriodSlideOn = true
		voice.PeriodSlideWithLimit = false
	case 0x2: // Portamento down (period slide up)
		voice.PeriodSlideSpeed = fxParam
		voice.PeriodSlideOn = true
		voice.PeriodSlideWithLimit = false
	case 0xC: // Volume
		if fxParam <= 0x40 {
			voice.NoteMaxVolume = fxParam
		} else {
			fxParam -= 0x50
			if fxParam <= 0x40 {
				for i := range 4 {
					r.Voices[i].TrackMasterVolume = fxParam
				}
			} else {
				fxParam -= 0xA0 - 0x50
				if fxParam <= 0x40 {
					voice.TrackMasterVolume = fxParam
				}
			}
		}
	case 0xE: // Enhanced commands
		switch fxParam >> 4 {
		case 0x1: // Fine slide up
			voice.PeriodSlidePeriod = -(fxParam & 0x0F)
			voice.PlantPeriod = true
		case 0x2: // Fine slide down
			voice.PeriodSlidePeriod = fxParam & 0x0F
			voice.PlantPeriod = true
		case 0x4: // Vibrato control
			voice.VibratoDepth = fxParam & 0x0F
		case 0xA: // Fine volume up
			voice.NoteMaxVolume += fxParam & 0x0F
			if voice.NoteMaxVolume > 0x40 {
				voice.NoteMaxVolume = 0x40
			}
		case 0xB: // Fine volume down
			voice.NoteMaxVolume -= fxParam & 0x0F
			if voice.NoteMaxVolume < 0 {
				voice.NoteMaxVolume = 0
			}
		}
	}
}

// ProcessFrame handles per-tick effects for a voice
func (r *AHXReplayer) ProcessFrame(v int) {
	voice := &r.Voices[v]
	if !voice.TrackOn {
		return
	}

	// Note delay
	if voice.NoteDelayOn != 0 {
		if voice.NoteDelayWait <= 0 {
			r.ProcessStep(v)
		} else {
			voice.NoteDelayWait--
		}
	}

	// Hard cut
	if voice.HardCut != 0 {
		var nextInst int
		if r.NoteNr+1 < r.Song.TrackLength {
			track := voice.Track
			if track < len(r.Song.Tracks) {
				nextInst = r.Song.Tracks[track][r.NoteNr+1].Instrument
			}
		} else {
			track := voice.NextTrack
			if track < len(r.Song.Tracks) {
				nextInst = r.Song.Tracks[track][0].Instrument
			}
		}
		if nextInst != 0 {
			d1 := max(r.Tempo-voice.HardCut, 0)
			if voice.NoteCutOn == 0 {
				voice.NoteCutOn = 1
				voice.NoteCutWait = d1
				voice.HardCutReleaseF = -(d1 - r.Tempo)
			} else {
				voice.HardCut = 0
			}
		}
	}

	// Note cut
	if voice.NoteCutOn != 0 {
		if voice.NoteCutWait <= 0 {
			voice.NoteCutOn = 0
			if voice.HardCutRelease != 0 {
				voice.ADSR.RVolume = -(voice.ADSRVolume - (voice.Instrument.Envelope.RVolume << 8)) / voice.HardCutReleaseF
				voice.ADSR.RFrames = voice.HardCutReleaseF
				voice.ADSR.AFrames = 0
				voice.ADSR.DFrames = 0
				voice.ADSR.SFrames = 0
			} else {
				voice.NoteMaxVolume = 0
			}
		} else {
			voice.NoteCutWait--
		}
	}

	// ADSR envelope
	if voice.ADSR.AFrames > 0 {
		voice.ADSRVolume += voice.ADSR.AVolume
		voice.ADSR.AFrames--
		if voice.ADSR.AFrames <= 0 && voice.Instrument != nil {
			voice.ADSRVolume = voice.Instrument.Envelope.AVolume << 8
		}
	} else if voice.ADSR.DFrames > 0 {
		voice.ADSRVolume += voice.ADSR.DVolume
		voice.ADSR.DFrames--
		if voice.ADSR.DFrames <= 0 && voice.Instrument != nil {
			voice.ADSRVolume = voice.Instrument.Envelope.DVolume << 8
		}
	} else if voice.ADSR.SFrames > 0 {
		voice.ADSR.SFrames--
	} else if voice.ADSR.RFrames > 0 {
		voice.ADSRVolume += voice.ADSR.RVolume
		voice.ADSR.RFrames--
		if voice.ADSR.RFrames <= 0 && voice.Instrument != nil {
			voice.ADSRVolume = voice.Instrument.Envelope.RVolume << 8
		}
	}

	// Volume slide
	voice.NoteMaxVolume = min(max(voice.NoteMaxVolume+voice.VolumeSlideUp-voice.VolumeSlideDown, 0), 0x40)

	// Portamento
	if voice.PeriodSlideOn {
		if voice.PeriodSlideWithLimit {
			d0 := voice.PeriodSlidePeriod - voice.PeriodSlideLimit
			d2 := voice.PeriodSlideSpeed
			if d0 > 0 {
				d2 = -d2
			}
			if d0 != 0 {
				d3 := (d0 + d2) ^ d0
				if d3 >= 0 {
					d0 = voice.PeriodSlidePeriod + d2
				} else {
					d0 = voice.PeriodSlideLimit
				}
				voice.PeriodSlidePeriod = d0
				voice.PlantPeriod = true
			}
		} else {
			voice.PeriodSlidePeriod += voice.PeriodSlideSpeed
			voice.PlantPeriod = true
		}
	}

	// Vibrato
	if voice.VibratoDepth > 0 {
		if voice.VibratoDelay <= 0 {
			voice.VibratoPeriod = (AHXVibratoTable[voice.VibratoCurrent] * voice.VibratoDepth) >> 7
			voice.PlantPeriod = true
			voice.VibratoCurrent = (voice.VibratoCurrent + voice.VibratoSpeed) & 0x3F
		} else {
			voice.VibratoDelay--
		}
	}

	// Performance list (PList)
	if voice.Instrument != nil && voice.PerfCurrent < voice.Instrument.PList.Length {
		voice.PerfWait--
		if voice.PerfWait <= 0 {
			cur := voice.PerfCurrent
			voice.PerfCurrent++
			voice.PerfWait = voice.PerfSpeed

			entry := &voice.PerfList.Entries[cur]
			if entry.Waveform != 0 {
				voice.Waveform = entry.Waveform - 1
				voice.NewWaveform = 1
				voice.PeriodPerfSlideSpeed = 0
				voice.PeriodPerfSlidePeriod = 0
			}

			voice.PeriodPerfSlideOn = false
			for i := range 2 {
				r.PListCommandParse(v, entry.FX[i], entry.FXParam[i])
			}

			// Get note
			if entry.Note != 0 {
				voice.InstrPeriod = entry.Note
				voice.PlantPeriod = true
				voice.FixedNote = entry.Fixed
			}
		}
	} else {
		if voice.PerfWait > 0 {
			voice.PerfWait--
		} else {
			voice.PeriodPerfSlideSpeed = 0
		}
	}

	// Performance portamento
	if voice.PeriodPerfSlideOn {
		voice.PeriodPerfSlidePeriod -= voice.PeriodPerfSlideSpeed
		if voice.PeriodPerfSlidePeriod != 0 {
			voice.PlantPeriod = true
		}
	}

	// Square modulation
	if voice.Waveform == 3-1 && voice.SquareOn != 0 {
		voice.SquareWait--
		if voice.SquareWait <= 0 {
			d1 := voice.SquareLowerLimit
			d2 := voice.SquareUpperLimit
			d3 := voice.SquarePos

			if voice.SquareInit != 0 {
				voice.SquareInit = 0
				if d3 <= d1 {
					voice.SquareSlidingIn = 1
					voice.SquareSign = 1
				} else if d3 >= d2 {
					voice.SquareSlidingIn = 1
					voice.SquareSign = -1
				}
			}

			if d1 == d3 || d2 == d3 {
				if voice.SquareSlidingIn != 0 {
					voice.SquareSlidingIn = 0
				} else {
					voice.SquareSign = -voice.SquareSign
				}
			}
			d3 += voice.SquareSign
			voice.SquarePos = d3
			voice.PlantSquare = 1
			if voice.Instrument != nil {
				voice.SquareWait = voice.Instrument.SquareSpeed
			}
		}
	}

	// Filter modulation
	if voice.FilterOn != 0 {
		voice.FilterWait--
		if voice.FilterWait <= 0 {
			d1 := voice.FilterLowerLimit
			d2 := voice.FilterUpperLimit
			d3 := voice.FilterPos

			if voice.FilterInit != 0 {
				voice.FilterInit = 0
				if d3 <= d1 {
					voice.FilterSlidingIn = 1
					voice.FilterSign = 1
				} else if d3 >= d2 {
					voice.FilterSlidingIn = 1
					voice.FilterSign = -1
				}
			}

			fMax := 1
			if voice.FilterSpeed < 3 {
				fMax = 5 - voice.FilterSpeed
			}
			for i := 0; i < fMax; i++ {
				if d1 == d3 || d2 == d3 {
					if voice.FilterSlidingIn != 0 {
						voice.FilterSlidingIn = 0
					} else {
						voice.FilterSign = -voice.FilterSign
					}
				}
				d3 += voice.FilterSign
			}
			voice.FilterPos = d3
			voice.NewWaveform = 1
			voice.FilterWait = max(voice.FilterSpeed-3, 1)
		}
	}

	// Calculate square waveform
	if voice.Waveform == 3-1 || voice.PlantSquare != 0 {
		// Get base square from filter position
		squareOffset := (voice.FilterPos - 0x20) * ahxFilterBlockSize
		x := voice.SquarePos << (5 - voice.WaveLength)
		if x > 0x20 {
			x = 0x40 - x
			voice.SquareReverse = 1
		}
		if x > 0 {
			x--
			squareOffset += x << 7
		}
		delta := 32 >> voice.WaveLength
		if squareOffset >= 0 && squareOffset < len(r.Waves.Squares) {
			r.WaveformTab[2] = voice.SquareTempBuffer[:]
			for i := 0; i < (1<<voice.WaveLength)*4; i++ {
				srcIdx := squareOffset + i*delta
				if srcIdx >= 0 && srcIdx < len(r.Waves.Squares) {
					voice.SquareTempBuffer[i] = r.Waves.Squares[srcIdx]
				}
			}
		}
		voice.NewWaveform = 1
		voice.Waveform = 3 - 1
		voice.PlantSquare = 0
	}

	if voice.Waveform == 4-1 {
		voice.NewWaveform = 1
	}

	// Update audio source
	if voice.NewWaveform != 0 {
		audioSource := r.WaveformTab[voice.Waveform]
		if voice.Waveform != 3-1 {
			// Apply filter offset
			filterOffset := (voice.FilterPos - 0x20) * ahxFilterBlockSize
			if filterOffset >= 0 && filterOffset < len(r.Waves.LowPasses) {
				if voice.Waveform < 3-1 {
					// Triangle or sawtooth - get from lowpass buffer
					if voice.WaveLength < len(ahxWaveLengthOffsets) {
						filterOffset += ahxWaveLengthOffsets[voice.WaveLength]
					}
				}
			}
		}
		if voice.Waveform == 4-1 {
			// Noise - add random offset
			audioSource = r.Waves.WhiteNoiseBig[:]
			offset := (r.WNRandom & (2*0x280 - 1)) & ^1
			if offset < len(audioSource) {
				audioSource = audioSource[offset:]
			}
			r.WNRandom += 2239384
			r.WNRandom = ((((r.WNRandom >> 8) | (r.WNRandom << 24)) + 782323) ^ 75) - 6735
		}
		voice.AudioSource = audioSource
	}

	// Calculate final period
	voice.AudioPeriod = voice.InstrPeriod
	if voice.FixedNote == 0 {
		voice.AudioPeriod += voice.Transpose + voice.TrackPeriod - 1
	}
	if voice.AudioPeriod > 5*12 {
		voice.AudioPeriod = 5 * 12
	}
	if voice.AudioPeriod < 0 {
		voice.AudioPeriod = 0
	}
	voice.AudioPeriod = AHXPeriodTable[voice.AudioPeriod]
	if voice.FixedNote == 0 {
		voice.AudioPeriod += voice.PeriodSlidePeriod
	}
	voice.AudioPeriod += voice.PeriodPerfSlidePeriod + voice.VibratoPeriod
	if voice.AudioPeriod > 0x0D60 {
		voice.AudioPeriod = 0x0D60
	}
	if voice.AudioPeriod < 0x0071 {
		voice.AudioPeriod = 0x0071
	}

	// Calculate final volume
	// (ADSRVolume>>8) * NoteMaxVolume / 64 * PerfSubVolume / 64 * TrackMasterVolume / 64 * MainVolume / 64
	vol := voice.ADSRVolume >> 8
	vol = (vol * voice.NoteMaxVolume) >> 6
	vol = (vol * voice.PerfSubVolume) >> 6
	vol = (vol * voice.TrackMasterVolume) >> 6
	vol = (vol * r.MainVolume) >> 6
	voice.AudioVolume = vol
}

// SetAudio updates the voice output (period and volume)
func (r *AHXReplayer) SetAudio(v int) {
	voice := &r.Voices[v]
	if !voice.TrackOn {
		voice.VoiceVolume = 0
		return
	}

	voice.VoiceVolume = voice.AudioVolume
	if voice.PlantPeriod {
		voice.PlantPeriod = false
		voice.VoicePeriod = voice.AudioPeriod
	}
	if voice.NewWaveform != 0 {
		// Copy waveform to voice buffer
		if voice.Waveform == 4-1 {
			// Noise
			if len(voice.AudioSource) >= 0x280 {
				copy(voice.VoiceBuffer[:0x280], voice.AudioSource[:0x280])
			}
		} else {
			// Other waveforms - loop to fill buffer
			waveLen := 4 * (1 << voice.WaveLength)
			waveLoops := (1 << (5 - voice.WaveLength)) * 5
			for i := 0; i < waveLoops && len(voice.AudioSource) >= waveLen; i++ {
				copy(voice.VoiceBuffer[i*waveLen:], voice.AudioSource[:waveLen])
			}
		}
		voice.VoiceBuffer[0x280] = voice.VoiceBuffer[0]
		voice.NewWaveform = 0
	}
}

// PListCommandParse handles playlist commands
func (r *AHXReplayer) PListCommandParse(v, fx, fxParam int) {
	voice := &r.Voices[v]

	switch fx {
	case 0: // Set filter (AHX1 only)
		if r.Song.Revision > 0 && fxParam != 0 {
			if voice.IgnoreFilter != 0 {
				voice.FilterPos = voice.IgnoreFilter
				voice.IgnoreFilter = 0
			} else {
				voice.FilterPos = fxParam
			}
			voice.NewWaveform = 1
		}
	case 1: // Slide up
		voice.PeriodPerfSlideSpeed = fxParam
		voice.PeriodPerfSlideOn = true
	case 2: // Slide down
		voice.PeriodPerfSlideSpeed = -fxParam
		voice.PeriodPerfSlideOn = true
	case 3: // Init square modulation
		if voice.IgnoreSquare == 0 {
			voice.SquarePos = fxParam >> (5 - voice.WaveLength)
		} else {
			voice.IgnoreSquare = 0
		}
	case 4: // Start/stop modulation
		if r.Song.Revision == 0 || fxParam == 0 {
			if voice.SquareOn != 0 {
				voice.SquareOn = 0
			} else {
				voice.SquareOn = 1
				voice.SquareInit = 1
			}
			voice.SquareSign = 1
		} else {
			if fxParam&0x0F != 0 {
				if voice.SquareOn != 0 {
					voice.SquareOn = 0
				} else {
					voice.SquareOn = 1
					voice.SquareInit = 1
				}
				voice.SquareSign = 1
				if (fxParam & 0x0F) == 0x0F {
					voice.SquareSign = -1
				}
			}
			if fxParam&0xF0 != 0 {
				if voice.FilterOn != 0 {
					voice.FilterOn = 0
				} else {
					voice.FilterOn = 1
					voice.FilterInit = 1
				}
				voice.FilterSign = 1
				if (fxParam & 0xF0) == 0xF0 {
					voice.FilterSign = -1
				}
			}
		}
	case 5: // Jump to step
		voice.PerfCurrent = fxParam
	case 6: // Set volume
		if fxParam > 0x40 {
			fxParam -= 0x50
			if fxParam >= 0 {
				if fxParam <= 0x40 {
					voice.PerfSubVolume = fxParam
				} else {
					fxParam -= 0xA0 - 0x50
					if fxParam >= 0 && fxParam <= 0x40 {
						voice.TrackMasterVolume = fxParam
					}
				}
			}
		} else {
			voice.NoteMaxVolume = fxParam
		}
	case 7: // Set speed
		voice.PerfSpeed = fxParam
		voice.PerfWait = fxParam
	}
}
