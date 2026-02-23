package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ProTracker .mod file replayer
//
// Supports standard 4-channel ProTracker modules with format IDs:
// "M.K.", "4CHN", "FLT4", "M!K!", "4CH\0"
//
// Signal flow: MOD pattern data → period/sample/effect processing → PCM sample output

// MOD format constants
const (
	modSongNameLen     = 20
	modSampleDescLen   = 30
	modSampleNameLen   = 22
	modNumSamples      = 31
	modPatternTableLen = 128
	modFormatIDLen     = 4
	modHeaderLen       = modSongNameLen + (modSampleDescLen * modNumSamples) + 1 + 1 + modPatternTableLen + modFormatIDLen // 1084
	modRowsPerPattern  = 64
	modChannels        = 4
	modNoteBytesPerCh  = 4
	modRowBytes        = modChannels * modNoteBytesPerCh // 16
	modPatternBytes    = modRowsPerPattern * modRowBytes // 1024

	modDefaultSpeed = 6
	modDefaultBPM   = 125

	// PAL clock rate for Amiga period → frequency conversion
	modPALClock = 3546895.0
)

// MODSample describes a single sample instrument in a MOD file.
type MODSample struct {
	Name       string
	Length     int // in bytes (not words)
	Finetune   int8
	Volume     int // 0-64
	LoopStart  int // in bytes
	LoopLength int // in bytes
	Data       []int8
}

// MODNote represents a single note event in a pattern.
type MODNote struct {
	SampleNum uint8
	Period    uint16
	Effect    uint8
	EffParam  uint8
}

// MODPattern is a grid of 64 rows x 4 channels of notes.
type MODPattern struct {
	Notes [modRowsPerPattern][modChannels]MODNote
}

// MODFile holds a parsed ProTracker module.
type MODFile struct {
	SongName     string
	Samples      [modNumSamples]MODSample
	SongLength   int
	RestartPos   int
	PatternTable [modPatternTableLen]uint8
	FormatID     string
	Patterns     []MODPattern
	NumPatterns  int
	NumChannels  int
}

// MODChannel holds per-channel playback state for the replayer.
type MODChannel struct {
	sample       *MODSample
	sampleNum    int
	period       uint16
	targetPeriod uint16
	portaSpeed   uint8
	volume       int
	phase        float64
	phaseInc     float64
	vibratoPos   int
	vibratoSpeed uint8
	vibratoDepth uint8
	tremoloPos   int
	tremoloSpeed uint8
	tremoloDepth uint8
	sampleOffset int
	looping      bool
	active       bool

	// Pattern delay / retriggering
	retrigCount int
	retrigRate  int
}

// MODReplayer manages song playback state.
type MODReplayer struct {
	mod          *MODFile
	channels     [modChannels]MODChannel
	speed        int
	bpm          int
	tick         int
	row          int
	position     int
	patternDelay int
	songEnd      bool
	ledFilter    bool

	// Deferred position/pattern changes (applied after row processing)
	posJumpPending  bool
	posJumpTarget   int
	patBreakPending bool
	patBreakRow     int

	// Per-row note storage for effect processing on non-zero ticks
	rowNotes [modChannels]MODNote
}

// Amiga period table for finetune 0 (octaves 1-3, C-1 to B-3)
var modPeriodTable = [36]uint16{
	856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453, // octave 1
	428, 404, 381, 360, 339, 320, 302, 285, 269, 254, 240, 226, // octave 2
	214, 202, 190, 180, 170, 160, 151, 143, 135, 127, 120, 113, // octave 3
}

// ParseMOD parses a ProTracker .mod file from raw bytes.
func ParseMOD(data []byte) (*MODFile, error) {
	if len(data) < modHeaderLen {
		return nil, errors.New("mod: data too short for header")
	}

	mod := &MODFile{}

	// Song name (20 bytes)
	mod.SongName = trimNullString(data[:modSongNameLen])

	// Parse 31 sample descriptors
	offset := modSongNameLen
	for i := range modNumSamples {
		s := &mod.Samples[i]
		desc := data[offset : offset+modSampleDescLen]
		s.Name = trimNullString(desc[:modSampleNameLen])
		s.Length = int(binary.BigEndian.Uint16(desc[22:24])) * 2 // words → bytes
		s.Finetune = int8(desc[24] & 0x0F)
		if s.Finetune >= 8 {
			s.Finetune -= 16 // signed nibble
		}
		s.Volume = int(desc[25])
		if s.Volume > 64 {
			s.Volume = 64
		}
		s.LoopStart = int(binary.BigEndian.Uint16(desc[26:28])) * 2
		s.LoopLength = int(binary.BigEndian.Uint16(desc[28:30])) * 2
		offset += modSampleDescLen
	}

	// Song length and restart position
	mod.SongLength = int(data[offset])
	mod.RestartPos = int(data[offset+1])
	offset += 2

	// Pattern sequence table (128 bytes)
	copy(mod.PatternTable[:], data[offset:offset+modPatternTableLen])
	offset += modPatternTableLen

	// Format ID (4 bytes)
	mod.FormatID = string(data[offset : offset+modFormatIDLen])
	offset += modFormatIDLen

	// Validate format ID
	if !isValidMODFormat(mod.FormatID) {
		return nil, fmt.Errorf("mod: unknown format ID %q", mod.FormatID)
	}
	mod.NumChannels = modChannels

	// Count patterns (highest pattern number in sequence + 1)
	mod.NumPatterns = 0
	for i := range mod.SongLength {
		if int(mod.PatternTable[i]) >= mod.NumPatterns {
			mod.NumPatterns = int(mod.PatternTable[i]) + 1
		}
	}

	// Parse pattern data
	patternDataSize := mod.NumPatterns * modPatternBytes
	if offset+patternDataSize > len(data) {
		return nil, errors.New("mod: data too short for pattern data")
	}

	mod.Patterns = make([]MODPattern, mod.NumPatterns)
	for p := range mod.NumPatterns {
		for row := range modRowsPerPattern {
			for ch := range modChannels {
				noteOff := offset + p*modPatternBytes + row*modRowBytes + ch*modNoteBytesPerCh
				b := data[noteOff : noteOff+4]

				note := &mod.Patterns[p].Notes[row][ch]
				note.SampleNum = (b[0] & 0xF0) | ((b[2] >> 4) & 0x0F)
				note.Period = uint16(b[0]&0x0F)<<8 | uint16(b[1])
				note.Effect = b[2] & 0x0F
				note.EffParam = b[3]
			}
		}
	}
	offset += patternDataSize

	// Extract sample data
	for i := range modNumSamples {
		s := &mod.Samples[i]
		if s.Length == 0 {
			continue
		}
		end := offset + s.Length
		if end > len(data) {
			// Truncate to available data
			end = len(data)
			s.Length = end - offset
		}
		s.Data = make([]int8, s.Length)
		for j := range s.Length {
			s.Data[j] = int8(data[offset+j])
		}
		offset += s.Length
	}

	return mod, nil
}

func isValidMODFormat(id string) bool {
	switch id {
	case "M.K.", "4CHN", "FLT4", "M!K!", "4CH\x00":
		return true
	}
	return false
}

func trimNullString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// NewMODReplayer creates a replayer for the given parsed MOD file.
func NewMODReplayer(mod *MODFile) *MODReplayer {
	r := &MODReplayer{
		mod:   mod,
		speed: modDefaultSpeed,
		bpm:   modDefaultBPM,
	}
	return r
}

// ProcessTick advances the replayer by one tick.
// Called at tick boundaries (samplesPerTick intervals).
func (r *MODReplayer) ProcessTick() {
	if r.songEnd {
		return
	}

	if r.tick == 0 {
		r.processRow()
	} else {
		r.processEffects()
	}

	r.tick++
	if r.tick >= r.speed {
		r.tick = 0
		if r.patternDelay > 0 {
			r.patternDelay--
			return
		}

		// Apply deferred position/pattern changes
		if r.posJumpPending {
			r.position = r.posJumpTarget - 1
			r.posJumpPending = false
			if !r.patBreakPending {
				r.row = 0
			}
			if r.position >= r.mod.SongLength {
				r.songEnd = true
			}
			if r.patBreakPending {
				r.row = r.patBreakRow
				r.patBreakPending = false
			}
			return
		}
		if r.patBreakPending {
			r.position++
			r.row = r.patBreakRow
			r.patBreakPending = false
			if r.position >= r.mod.SongLength {
				r.songEnd = true
			}
			return
		}

		r.row++
		if r.row >= modRowsPerPattern {
			r.row = 0
			r.position++
			if r.position >= r.mod.SongLength {
				r.songEnd = true
			}
		}
	}
}

// processRow handles tick 0: load new note data and process tick-0 effects.
func (r *MODReplayer) processRow() {
	if r.position >= r.mod.SongLength {
		r.songEnd = true
		return
	}
	patIdx := int(r.mod.PatternTable[r.position])
	if patIdx >= len(r.mod.Patterns) {
		r.songEnd = true
		return
	}

	for ch := range modChannels {
		note := r.mod.Patterns[patIdx].Notes[r.row][ch]
		r.rowNotes[ch] = note
		mc := &r.channels[ch]

		// Sample number change
		if note.SampleNum > 0 && int(note.SampleNum) <= modNumSamples {
			mc.sampleNum = int(note.SampleNum)
			mc.sample = &r.mod.Samples[note.SampleNum-1]
			mc.volume = mc.sample.Volume
		}

		// Period (note) trigger — but not for tone portamento (effect 3/5)
		if note.Period > 0 && note.Effect != 3 && note.Effect != 5 {
			mc.period = note.Period
			mc.phase = 0
			mc.active = true
			if mc.sample != nil {
				mc.looping = mc.sample.LoopLength > 2
			}
			mc.updatePhaseInc(SAMPLE_RATE)
		}

		// Tone portamento target
		if note.Period > 0 && (note.Effect == 3 || note.Effect == 5) {
			mc.targetPeriod = note.Period
		}

		// Process tick-0 effects
		r.processTickZeroEffect(ch, note)
	}
}

// processTickZeroEffect handles effects that happen on tick 0.
func (r *MODReplayer) processTickZeroEffect(ch int, note MODNote) {
	mc := &r.channels[ch]

	switch note.Effect {
	case 0x3: // Tone portamento
		if note.EffParam != 0 {
			mc.portaSpeed = note.EffParam
		}
	case 0x4: // Vibrato
		if note.EffParam&0xF0 != 0 {
			mc.vibratoSpeed = (note.EffParam >> 4) & 0x0F
		}
		if note.EffParam&0x0F != 0 {
			mc.vibratoDepth = note.EffParam & 0x0F
		}
	case 0x7: // Tremolo
		if note.EffParam&0xF0 != 0 {
			mc.tremoloSpeed = (note.EffParam >> 4) & 0x0F
		}
		if note.EffParam&0x0F != 0 {
			mc.tremoloDepth = note.EffParam & 0x0F
		}
	case 0x9: // Sample offset
		if note.EffParam != 0 {
			mc.sampleOffset = int(note.EffParam) * 256
			mc.phase = float64(mc.sampleOffset)
		}
	case 0xB: // Position jump
		r.posJumpTarget = int(note.EffParam) + 1 // +1 so 0 is distinguishable from "no jump"
		r.posJumpPending = true
	case 0xC: // Set volume
		mc.volume = min(int(note.EffParam), 64)
	case 0xD: // Pattern break
		r.patBreakRow = int(note.EffParam>>4)*10 + int(note.EffParam&0x0F)
		r.patBreakPending = true
	case 0xE: // Extended effects
		r.processExtendedEffect(ch, note.EffParam)
	case 0xF: // Set speed / BPM
		if note.EffParam == 0 {
			// speed 0 is ignored
		} else if note.EffParam < 32 {
			r.speed = int(note.EffParam)
		} else {
			r.bpm = int(note.EffParam)
		}
	}
}

// processExtendedEffect handles Exy effects (tick 0 only for most).
func (r *MODReplayer) processExtendedEffect(ch int, param uint8) {
	mc := &r.channels[ch]
	x := param >> 4
	y := param & 0x0F

	switch x {
	case 0x0: // E0x: LED filter
		r.ledFilter = y == 0
	case 0x1: // E1x: Fine portamento up
		if mc.period > uint16(y) {
			mc.period -= uint16(y)
		}
		mc.updatePhaseInc(SAMPLE_RATE)
	case 0x2: // E2x: Fine portamento down
		mc.period += uint16(y)
		mc.updatePhaseInc(SAMPLE_RATE)
	case 0x5: // E5x: Set finetune
		if mc.sample != nil {
			mc.sample.Finetune = int8(y)
			if mc.sample.Finetune >= 8 {
				mc.sample.Finetune -= 16
			}
		}
	case 0x6: // E6x: Pattern loop
		// Not implemented (complex state management)
	case 0x9: // E9x: Retrigger note
		mc.retrigRate = int(y)
		mc.retrigCount = 0
	case 0xA: // EAx: Fine volume slide up
		mc.volume = min(mc.volume+int(y), 64)
	case 0xB: // EBx: Fine volume slide down
		mc.volume = max(mc.volume-int(y), 0)
	case 0xC: // ECx: Note cut
		// Handled on tick x
	case 0xD: // EDx: Note delay
		// Handled on tick x
	case 0xE: // EEx: Pattern delay
		r.patternDelay = int(y)
	}
}

// processEffects handles per-tick effects (ticks > 0).
func (r *MODReplayer) processEffects() {
	for ch := range modChannels {
		note := r.rowNotes[ch]
		mc := &r.channels[ch]

		switch note.Effect {
		case 0x0: // Arpeggio
			if note.EffParam != 0 {
				mc.applyArpeggio(note.EffParam, r.tick)
			}
		case 0x1: // Portamento up
			if mc.period > uint16(note.EffParam) {
				mc.period -= uint16(note.EffParam)
			} else {
				mc.period = 113 // minimum period
			}
			mc.updatePhaseInc(SAMPLE_RATE)
		case 0x2: // Portamento down
			mc.period += uint16(note.EffParam)
			if mc.period > 856 {
				mc.period = 856 // maximum period
			}
			mc.updatePhaseInc(SAMPLE_RATE)
		case 0x3: // Tone portamento
			mc.doTonePortamento()
		case 0x4: // Vibrato
			mc.doVibrato()
		case 0x5: // Tone portamento + volume slide
			mc.doTonePortamento()
			mc.doVolumeSlide(note.EffParam)
		case 0x6: // Vibrato + volume slide
			mc.doVibrato()
			mc.doVolumeSlide(note.EffParam)
		case 0xA: // Volume slide
			mc.doVolumeSlide(note.EffParam)
		case 0xE: // Extended effects on non-zero ticks
			x := note.EffParam >> 4
			y := note.EffParam & 0x0F
			switch x {
			case 0x9: // E9x: Retrigger
				if mc.retrigRate > 0 {
					mc.retrigCount++
					if mc.retrigCount >= mc.retrigRate {
						mc.retrigCount = 0
						mc.phase = 0
					}
				}
			case 0xC: // ECx: Note cut on tick x
				if r.tick == int(y) {
					mc.volume = 0
				}
			case 0xD: // EDx: Note delay
				// Note triggered on tick x (simplified)
			}
		}
	}
}

func (mc *MODChannel) updatePhaseInc(sampleRate int) {
	if mc.period == 0 {
		mc.phaseInc = 0
		return
	}
	mc.phaseInc = modPALClock / (float64(mc.period) * float64(sampleRate))
}

func (mc *MODChannel) doTonePortamento() {
	if mc.targetPeriod == 0 || mc.portaSpeed == 0 {
		return
	}
	if mc.period < mc.targetPeriod {
		mc.period += uint16(mc.portaSpeed)
		if mc.period > mc.targetPeriod {
			mc.period = mc.targetPeriod
		}
	} else if mc.period > mc.targetPeriod {
		if mc.period > uint16(mc.portaSpeed) {
			mc.period -= uint16(mc.portaSpeed)
		}
		if mc.period < mc.targetPeriod {
			mc.period = mc.targetPeriod
		}
	}
	mc.updatePhaseInc(SAMPLE_RATE)
}

func (mc *MODChannel) doVibrato() {
	// Sine vibrato
	delta := int(mc.vibratoDepth) * vibratoTable[mc.vibratoPos&31] / 128
	if mc.vibratoPos >= 32 {
		mc.period = uint16(max(int(mc.period)-delta, 113))
	} else {
		mc.period = uint16(min(int(mc.period)+delta, 856))
	}
	mc.vibratoPos = (mc.vibratoPos + int(mc.vibratoSpeed)) & 63
	mc.updatePhaseInc(SAMPLE_RATE)
}

func (mc *MODChannel) doVolumeSlide(param uint8) {
	up := int(param >> 4)
	down := int(param & 0x0F)
	mc.volume = max(min(mc.volume+up-down, 64), 0)
}

func (mc *MODChannel) applyArpeggio(param uint8, tick int) {
	if mc.period == 0 {
		return
	}
	switch tick % 3 {
	case 0:
		// base period (no change)
	case 1:
		mc.period = arpeggioPeriod(mc.period, int(param>>4))
	case 2:
		mc.period = arpeggioPeriod(mc.period, int(param&0x0F))
	}
	mc.updatePhaseInc(SAMPLE_RATE)
}

// arpeggioPeriod looks up the period shifted by semitones.
func arpeggioPeriod(basePeriod uint16, semitones int) uint16 {
	// Find the base period in the table and shift
	for i, p := range modPeriodTable {
		if basePeriod >= p {
			idx := i + semitones
			if idx < len(modPeriodTable) {
				return modPeriodTable[idx]
			}
			return modPeriodTable[len(modPeriodTable)-1]
		}
	}
	return basePeriod
}

// Sine table for vibrato/tremolo (0-31 half-wave, 32-63 negative half)
var vibratoTable = [32]int{
	0, 24, 49, 74, 97, 120, 141, 161,
	180, 197, 212, 224, 235, 244, 250, 253,
	255, 253, 250, 244, 235, 224, 212, 197,
	180, 161, 141, 120, 97, 74, 49, 24,
}

// ReadSample reads the current sample byte for a channel, advancing the phase.
// Returns a signed sample value (-128..+127) and whether the channel is still active.
func (mc *MODChannel) ReadSample() (int8, bool) {
	if !mc.active || mc.sample == nil || len(mc.sample.Data) == 0 {
		return 0, false
	}

	pos := int(mc.phase)

	// Handle looping
	if mc.looping && mc.sample.LoopLength > 2 {
		loopEnd := mc.sample.LoopStart + mc.sample.LoopLength
		for pos >= loopEnd {
			pos -= mc.sample.LoopLength
			mc.phase -= float64(mc.sample.LoopLength)
		}
	} else if pos >= len(mc.sample.Data) {
		mc.active = false
		return 0, false
	}

	if pos < 0 {
		pos = 0
	}
	if pos >= len(mc.sample.Data) {
		if mc.looping && mc.sample.LoopLength > 2 {
			pos = mc.sample.LoopStart
			mc.phase = float64(pos)
		} else {
			mc.active = false
			return 0, false
		}
	}

	val := mc.sample.Data[pos]

	mc.phase += mc.phaseInc
	return val, true
}

// SamplesPerTick computes the number of audio samples per tracker tick.
func SamplesPerTick(sampleRate, bpm int) int {
	return sampleRate * 5 / (bpm * 2)
}
