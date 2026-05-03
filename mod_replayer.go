package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
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
	modMaxChannels     = 32
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

// MODPattern is a grid of rows x channels of notes.
type MODPattern struct {
	Notes [][]MODNote
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
	sample           *MODSample
	sampleNum        int
	period           uint16
	basePeriod       uint16
	targetPeriod     uint16
	portaSpeed       uint8
	volume           int
	phase            float64
	phaseInc         float64
	vibratoPos       int
	vibratoSpeed     uint8
	vibratoDepth     uint8
	tremoloPos       int
	tremoloSpeed     uint8
	tremoloDepth     uint8
	tremoloDelta     int
	vibWave          uint8
	sampleOffset     int
	looping          bool
	active           bool
	finetuneOverride int8

	pendingNoteDelay  int
	pendingNote       MODNote
	pendingNoteActive bool

	memVibrato      uint8
	memTremolo      uint8
	memVolSlide     uint8
	memPortaUp      uint8
	memPortaDown    uint8
	memSampleOffset uint8
	memArpeggio     uint8
	memTonePorta    uint8
	funkSpeed       uint8
	loopRow         int
	loopCount       int
	loopActive      bool
	loopCompleteRow int

	// Pattern delay / retriggering
	retrigCount int
	retrigRate  int
}

// MODReplayer manages song playback state.
type MODReplayer struct {
	mod                 *MODFile
	channels            []MODChannel
	sampleRate          int
	speed               int
	bpm                 int
	tick                int
	row                 int
	position            int
	patternDelay        int
	patternDelayApplied bool
	songEnd             bool
	ledFilter           bool

	// Deferred position/pattern changes (applied after row processing)
	posJumpPending     bool
	posJumpTarget      int
	patBreakPending    bool
	patBreakRow        int
	patternLoopPending bool
	patternLoopRow     int

	// Per-row note storage for effect processing on non-zero ticks
	rowNotes []MODNote
}

// Amiga period table for finetune 0 (octaves 1-3, C-1 to B-3)
var modPeriodTable = [36]uint16{
	856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453, // octave 1
	428, 404, 381, 360, 339, 320, 302, 285, 269, 254, 240, 226, // octave 2
	214, 202, 190, 180, 170, 160, 151, 143, 135, 127, 120, 113, // octave 3
}

var modFinetunePeriods = buildMODFinetunePeriods()

func buildMODFinetunePeriods() [16][60]uint16 {
	base := [12]uint16{856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453}
	var table [16][60]uint16
	for ftIdx := range 16 {
		ft := ftIdx
		if ft >= 8 {
			ft -= 16
		}
		factor := math.Pow(2, float64(ft)/(12.0*8.0))
		for note := range 60 {
			octave := note / 12
			semi := note % 12
			raw := float64(base[semi]) / math.Pow(2, float64(octave))
			period := int(math.Round(raw / factor))
			if period < 1 {
				period = 1
			}
			table[ftIdx][note] = uint16(period)
		}
	}
	return table
}

func finetuneIndex(ft int8) int {
	return int(uint8(ft) & 0x0F)
}

func periodForNote(noteIdx int, ft int8) uint16 {
	if noteIdx < 0 {
		noteIdx = 0
	} else if noteIdx >= len(modFinetunePeriods[0]) {
		noteIdx = len(modFinetunePeriods[0]) - 1
	}
	return modFinetunePeriods[finetuneIndex(ft)][noteIdx]
}

func findNoteIndex(period uint16, ft int8) int {
	row := modFinetunePeriods[finetuneIndex(ft)]
	bestIdx := 0
	bestDelta := math.MaxInt
	for i, p := range row {
		if p == period {
			return i
		}
		delta := modAbsInt(int(p) - int(period))
		if delta < bestDelta {
			bestDelta = delta
			bestIdx = i
		}
	}
	return bestIdx
}

func clampPeriod(v int) uint16 {
	if v < 113 {
		return 113
	}
	if v > 856 {
		return 856
	}
	return uint16(v)
}

func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 64 {
		return 64
	}
	return v
}

func modAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
	if mod.SongLength > modPatternTableLen {
		mod.SongLength = modPatternTableLen
	}
	mod.RestartPos = int(data[offset+1])
	offset += 2

	// Pattern sequence table (128 bytes)
	copy(mod.PatternTable[:], data[offset:offset+modPatternTableLen])
	offset += modPatternTableLen

	// Format ID (4 bytes)
	mod.FormatID = string(data[offset : offset+modFormatIDLen])
	offset += modFormatIDLen

	channels, ok := modChannelsForFormat(mod.FormatID)
	if !ok {
		return nil, fmt.Errorf("mod: unknown format ID %q", mod.FormatID)
	}
	mod.NumChannels = channels

	// Count patterns (highest pattern number in sequence + 1)
	mod.NumPatterns = 0
	for i := range mod.SongLength {
		if int(mod.PatternTable[i]) >= mod.NumPatterns {
			mod.NumPatterns = int(mod.PatternTable[i]) + 1
		}
	}

	// Parse pattern data
	rowBytes := mod.NumChannels * modNoteBytesPerCh
	patternBytes := modRowsPerPattern * rowBytes
	patternDataSize := mod.NumPatterns * patternBytes
	if offset+patternDataSize > len(data) {
		return nil, errors.New("mod: data too short for pattern data")
	}

	mod.Patterns = make([]MODPattern, mod.NumPatterns)
	for p := range mod.NumPatterns {
		mod.Patterns[p].Notes = make([][]MODNote, modRowsPerPattern)
		for row := range modRowsPerPattern {
			mod.Patterns[p].Notes[row] = make([]MODNote, mod.NumChannels)
			for ch := range mod.NumChannels {
				noteOff := offset + p*patternBytes + row*rowBytes + ch*modNoteBytesPerCh
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
	_, ok := modChannelsForFormat(id)
	return ok
}

func modChannelsForFormat(id string) (int, bool) {
	switch id {
	case "M.K.", "4CHN", "FLT4", "M!K!", "4CH\x00":
		return 4, true
	case "6CHN":
		return 6, true
	case "8CHN", "FLT8", "OCTA", "CD81", "OKTA":
		return 8, true
	}
	if len(id) == 4 && id[2] == 'C' && id[3] == 'H' && id[0] >= '0' && id[0] <= '9' && id[1] >= '0' && id[1] <= '9' {
		channels := int(id[0]-'0')*10 + int(id[1]-'0')
		if channels >= 1 && channels <= modMaxChannels {
			return channels, true
		}
	}
	return 0, false
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
	return NewMODReplayerWithSampleRate(mod, SAMPLE_RATE)
}

func NewMODReplayerWithSampleRate(mod *MODFile, sampleRate int) *MODReplayer {
	numChannels := mod.NumChannels
	if numChannels <= 0 {
		numChannels = modChannels
	}
	r := &MODReplayer{
		mod:        mod,
		channels:   make([]MODChannel, numChannels),
		rowNotes:   make([]MODNote, numChannels),
		sampleRate: sampleRate,
		speed:      modDefaultSpeed,
		bpm:        modDefaultBPM,
	}
	for i := range r.channels {
		r.channels[i].finetuneOverride = -128
		r.channels[i].loopCompleteRow = -1
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
		if r.patternLoopPending {
			r.row = r.patternLoopRow
			r.patternLoopPending = false
			r.patternDelayApplied = false
			return
		}

		// Apply deferred position/pattern changes
		if r.posJumpPending {
			r.position = r.posJumpTarget
			r.posJumpPending = false
			if !r.patBreakPending {
				r.row = 0
			}
			r.resetPatternLoopState()
			if r.position >= r.mod.SongLength {
				r.songEnd = true
			}
			if r.patBreakPending {
				r.row = r.patBreakRow
				r.patternDelayApplied = false
				r.patBreakPending = false
			}
			return
		}
		if r.patBreakPending {
			r.position++
			r.row = r.patBreakRow
			r.patternDelayApplied = false
			r.patBreakPending = false
			r.resetPatternLoopState()
			if r.position >= r.mod.SongLength {
				r.songEnd = true
			}
			return
		}

		r.row++
		r.patternDelayApplied = false
		if r.row >= modRowsPerPattern {
			r.row = 0
			r.position++
			r.resetPatternLoopState()
			if r.position >= r.mod.SongLength {
				r.songEnd = true
			}
		}
	}
}

func (r *MODReplayer) resetPatternLoopState() {
	for i := range r.channels {
		r.channels[i].loopRow = 0
		r.channels[i].loopCount = 0
		r.channels[i].loopActive = false
		r.channels[i].loopCompleteRow = -1
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

	for ch := range r.channels {
		note := r.mod.Patterns[patIdx].Notes[r.row][ch]
		r.rowNotes[ch] = note
		mc := &r.channels[ch]
		if note.Effect != 0x7 {
			mc.tremoloDelta = 0
		}

		if note.Effect == 0xE && note.EffParam>>4 == 0xD {
			delay := int(note.EffParam & 0x0F)
			if delay > 0 {
				mc.pendingNote = note
				mc.pendingNoteDelay = delay
				mc.pendingNoteActive = true
				continue
			}
		}

		r.applyNoteTrigger(ch, note)

		// Process tick-0 effects
		r.processTickZeroEffect(ch, note)
	}
}

func (r *MODReplayer) applyNoteTrigger(ch int, note MODNote) {
	mc := &r.channels[ch]

	if note.SampleNum > 0 && int(note.SampleNum) <= modNumSamples {
		mc.sampleNum = int(note.SampleNum)
		mc.sample = &r.mod.Samples[note.SampleNum-1]
		mc.volume = clampVolume(mc.sample.Volume)
	}

	if note.Period > 0 && note.Effect != 3 && note.Effect != 5 {
		noteIdx := findNoteIndex(note.Period, 0)
		ft := mc.effectiveFinetune()
		mc.basePeriod = periodForNote(noteIdx, ft)
		mc.period = mc.basePeriod
		startOffset := 0
		if note.Effect == 0x9 {
			startOffset = mc.sampleOffset
		}
		mc.phase = float64(startOffset)
		mc.active = true
		if mc.sample != nil {
			mc.looping = mc.sample.LoopLength > 2
		}
		mc.updatePhaseInc(r.sampleRate)
	}

	if note.Period > 0 && (note.Effect == 3 || note.Effect == 5) {
		noteIdx := findNoteIndex(note.Period, 0)
		mc.targetPeriod = periodForNote(noteIdx, mc.effectiveFinetune())
	}
}

func (mc *MODChannel) effectiveFinetune() int8 {
	if mc.finetuneOverride != -128 {
		return mc.finetuneOverride
	}
	if mc.sample == nil {
		return 0
	}
	return mc.sample.Finetune
}

// processTickZeroEffect handles effects that happen on tick 0.
func (r *MODReplayer) processTickZeroEffect(ch int, note MODNote) {
	mc := &r.channels[ch]

	switch note.Effect {
	case 0x3: // Tone portamento
		if note.EffParam != 0 {
			mc.portaSpeed = note.EffParam
			mc.memTonePorta = note.EffParam
		} else if mc.memTonePorta != 0 {
			mc.portaSpeed = mc.memTonePorta
		}
	case 0x4: // Vibrato
		if note.EffParam != 0 {
			mc.memVibrato = note.EffParam
		} else {
			note.EffParam = mc.memVibrato
		}
		if note.EffParam&0xF0 != 0 {
			mc.vibratoSpeed = (note.EffParam >> 4) & 0x0F
		}
		if note.EffParam&0x0F != 0 {
			mc.vibratoDepth = note.EffParam & 0x0F
		}
	case 0x7: // Tremolo
		if note.EffParam != 0 {
			mc.memTremolo = note.EffParam
		} else {
			note.EffParam = mc.memTremolo
		}
		if note.EffParam&0xF0 != 0 {
			mc.tremoloSpeed = (note.EffParam >> 4) & 0x0F
		}
		if note.EffParam&0x0F != 0 {
			mc.tremoloDepth = note.EffParam & 0x0F
		}
	case 0x9: // Sample offset
		if note.EffParam != 0 {
			mc.memSampleOffset = note.EffParam
		}
		if mc.memSampleOffset != 0 {
			mc.sampleOffset = int(mc.memSampleOffset) * 256
			mc.phase = float64(mc.sampleOffset)
			if mc.sample != nil && mc.sampleOffset >= len(mc.sample.Data) {
				mc.active = false
			}
		}
	case 0xB: // Position jump
		r.posJumpTarget = int(note.EffParam)
		r.posJumpPending = true
	case 0xC: // Set volume
		mc.volume = clampVolume(int(note.EffParam))
	case 0xD: // Pattern break
		r.patBreakRow = int(note.EffParam>>4)*10 + int(note.EffParam&0x0F)
		if r.patBreakRow >= modRowsPerPattern {
			r.patBreakRow = 0
		}
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
		mc.period = clampPeriod(int(mc.period) - int(y))
		mc.basePeriod = mc.period
		mc.updatePhaseInc(r.sampleRate)
	case 0x2: // E2x: Fine portamento down
		mc.period = clampPeriod(int(mc.period) + int(y))
		mc.basePeriod = mc.period
		mc.updatePhaseInc(r.sampleRate)
	case 0x5: // E5x: Set finetune
		mc.finetuneOverride = int8(y)
		if mc.finetuneOverride >= 8 {
			mc.finetuneOverride -= 16
		}
	case 0x6: // E6x: Pattern loop
		if y == 0 {
			if !mc.loopActive {
				mc.loopRow = r.row
				mc.loopCount = 0
				mc.loopCompleteRow = -1
			}
		} else {
			if mc.loopCompleteRow == r.row {
				return
			}
			if !mc.loopActive {
				mc.loopCount = int(y)
				mc.loopActive = true
			}
			if mc.loopCount > 0 {
				if mc.loopCount > 0 {
					r.patternLoopRow = mc.loopRow
					r.patternLoopPending = true
				} else {
				}
				mc.loopCount--
			} else {
				mc.loopActive = false
				mc.loopCompleteRow = r.row
			}
		}
	case 0x4: // E4x: Vibrato waveform
		mc.vibWave = y
	case 0x9: // E9x: Retrigger note
		mc.retrigRate = int(y)
		mc.retrigCount = 0
	case 0xA: // EAx: Fine volume slide up
		mc.volume = clampVolume(mc.volume + int(y))
	case 0xB: // EBx: Fine volume slide down
		mc.volume = clampVolume(mc.volume - int(y))
	case 0xC: // ECx: Note cut
		if y == 0 {
			mc.volume = 0
		}
	case 0xD: // EDx: Note delay
		// Handled on tick x
	case 0xE: // EEx: Pattern delay
		if !r.patternDelayApplied {
			r.patternDelay = int(y)
			r.patternDelayApplied = true
		}
	case 0xF: // EFx: Funk/InvertLoop memory-only stub
		mc.funkSpeed = y
	}
}

// processEffects handles per-tick effects (ticks > 0).
func (r *MODReplayer) processEffects() {
	for ch := range r.channels {
		note := r.rowNotes[ch]
		mc := &r.channels[ch]

		if mc.pendingNoteActive && r.tick == mc.pendingNoteDelay {
			r.applyNoteTrigger(ch, mc.pendingNote)
			mc.pendingNoteActive = false
		}

		switch note.Effect {
		case 0x0: // Arpeggio
			if note.EffParam != 0 {
				mc.memArpeggio = note.EffParam
			} else {
				note.EffParam = mc.memArpeggio
			}
			if note.EffParam != 0 {
				mc.applyArpeggio(note.EffParam, r.tick, r.sampleRate)
			}
		case 0x1: // Portamento up
			if note.EffParam != 0 {
				mc.memPortaUp = note.EffParam
			} else {
				note.EffParam = mc.memPortaUp
			}
			mc.period = clampPeriod(int(mc.period) - int(note.EffParam))
			mc.basePeriod = mc.period
			mc.updatePhaseInc(r.sampleRate)
		case 0x2: // Portamento down
			if note.EffParam != 0 {
				mc.memPortaDown = note.EffParam
			} else {
				note.EffParam = mc.memPortaDown
			}
			mc.period = clampPeriod(int(mc.period) + int(note.EffParam))
			mc.basePeriod = mc.period
			mc.updatePhaseInc(r.sampleRate)
		case 0x3: // Tone portamento
			mc.doTonePortamento(r.sampleRate)
		case 0x4: // Vibrato
			mc.doVibrato(r.sampleRate)
		case 0x5: // Tone portamento + volume slide
			mc.doTonePortamento(r.sampleRate)
			if note.EffParam != 0 {
				mc.memVolSlide = note.EffParam
			} else {
				note.EffParam = mc.memVolSlide
			}
			mc.doVolumeSlide(note.EffParam)
		case 0x6: // Vibrato + volume slide
			mc.doVibrato(r.sampleRate)
			if note.EffParam != 0 {
				mc.memVolSlide = note.EffParam
			} else {
				note.EffParam = mc.memVolSlide
			}
			mc.doVolumeSlide(note.EffParam)
		case 0x7: // Tremolo
			mc.doTremolo()
		case 0xA: // Volume slide
			if note.EffParam != 0 {
				mc.memVolSlide = note.EffParam
			} else {
				note.EffParam = mc.memVolSlide
			}
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
				if mc.pendingNoteActive && r.tick == int(y) {
					r.applyNoteTrigger(ch, mc.pendingNote)
					mc.pendingNoteActive = false
				}
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

func (mc *MODChannel) doTonePortamento(sampleRate int) {
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
	mc.basePeriod = mc.period
	mc.updatePhaseInc(sampleRate)
}

func (mc *MODChannel) doVibrato(sampleRate int) {
	delta := mc.waveformDelta(mc.vibWave, mc.vibratoPos, int(mc.vibratoDepth)*4)
	mc.period = clampPeriod(int(mc.basePeriod) + delta)
	mc.vibratoPos = (mc.vibratoPos + int(mc.vibratoSpeed)) & 63
	mc.updatePhaseInc(sampleRate)
}

func (mc *MODChannel) doTremolo() {
	mc.tremoloDelta = mc.waveformDelta(mc.vibWave, mc.tremoloPos, int(mc.tremoloDepth)*4)
	mc.tremoloDelta = clampVolume(mc.volume+mc.tremoloDelta) - mc.volume
	mc.tremoloPos = (mc.tremoloPos + int(mc.tremoloSpeed)) & 63
}

func (mc *MODChannel) waveformDelta(wave uint8, pos int, depth int) int {
	switch wave & 0x3 {
	case 1:
		if pos&32 == 0 {
			return depth
		}
		return -depth
	case 2:
		return depth - ((pos & 63) * depth / 32)
	default:
		delta := depth * vibratoTable[pos&31] / 255
		if pos&32 != 0 {
			return -delta
		}
		return delta
	}
}

func (mc *MODChannel) doVolumeSlide(param uint8) {
	up := int(param >> 4)
	down := int(param & 0x0F)
	mc.volume = clampVolume(mc.volume + up - down)
}

func (mc *MODChannel) applyArpeggio(param uint8, tick int, sampleRate int) {
	if mc.basePeriod == 0 {
		return
	}
	ft := mc.effectiveFinetune()
	baseIdx := findNoteIndex(mc.basePeriod, ft)
	switch tick % 3 {
	case 0:
		mc.period = mc.basePeriod
	case 1:
		mc.period = periodForNote(baseIdx+int(param>>4), ft)
	case 2:
		mc.period = periodForNote(baseIdx+int(param&0x0F), ft)
	}
	mc.updatePhaseInc(sampleRate)
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
		if mc.sample.LoopStart >= len(mc.sample.Data) || loopEnd > len(mc.sample.Data) {
			mc.active = false
			return 0, false
		}
		if pos >= loopEnd {
			loopLen := mc.sample.LoopLength
			pos = mc.sample.LoopStart + (pos-mc.sample.LoopStart)%loopLen
			mc.phase = float64(pos)
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
