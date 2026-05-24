package main

import (
	"math"
	"sync"
)

const midiMaxVoices = 10

type rawlandMiniPatch struct {
	waveform   int
	attack     float32
	decay      float32
	sustain    float32
	release    float32
	volume     float32
	percussion bool
}

// RawlandMini is an IE-native compact table derived from project-owned
// IntuitionSubtractor/Subsynth patch data and licensed with Intuition Engine
// under GPLv3-or-later. It is intentionally fixed and reduced for v1.
var rawlandMiniPatches = [128]rawlandMiniPatch{}
var rawlandMiniDrums = [128]rawlandMiniPatch{}

func init() {
	initRawlandMiniMelodic()
	initRawlandMiniDrums()
}

func initRawlandMiniMelodic() {
	for program := range rawlandMiniPatches {
		rawlandMiniPatches[program] = melodicPatchForProgram(program)
	}
}

func melodicPatchForProgram(program int) rawlandMiniPatch {
	var p rawlandMiniPatch
	family := program / 8
	variant := float32(program%8) / 7.0
	switch family {
	case 0: // Piano
		p = rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.001 + 0.002*variant, decay: 0.18 - 0.05*variant, sustain: 0.12 + 0.08*variant, release: 0.08 + 0.03*variant, volume: 0.24}
	case 1: // Chromatic percussion
		p = rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001, decay: 0.10 + 0.05*variant, sustain: 0.08, release: 0.06 + 0.05*variant, volume: 0.21}
	case 2: // Organ
		p = rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.006 + 0.006*variant, decay: 0.03, sustain: 0.86 - 0.10*variant, release: 0.10 + 0.05*variant, volume: 0.19}
	case 3: // Guitar
		p = rawlandMiniPatch{waveform: WAVE_SAWTOOTH, attack: 0.002, decay: 0.16 + 0.08*variant, sustain: 0.28 + 0.12*variant, release: 0.08 + 0.05*variant, volume: 0.22}
	case 4: // Bass
		p = rawlandMiniPatch{waveform: WAVE_SAWTOOTH, attack: 0.001 + 0.002*variant, decay: 0.08 + 0.04*variant, sustain: 0.62 + 0.12*variant, release: 0.07 + 0.04*variant, volume: 0.27}
	case 5: // Strings
		p = rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.025 + 0.030*variant, decay: 0.15, sustain: 0.72 + 0.10*variant, release: 0.22 + 0.15*variant, volume: 0.20}
	case 6: // Ensemble
		p = rawlandMiniPatch{waveform: WAVE_SAWTOOTH, attack: 0.020 + 0.025*variant, decay: 0.12, sustain: 0.70 + 0.12*variant, release: 0.20 + 0.12*variant, volume: 0.19}
	case 7: // Brass
		p = rawlandMiniPatch{waveform: WAVE_SAWTOOTH, attack: 0.010 + 0.020*variant, decay: 0.10, sustain: 0.78, release: 0.12 + 0.08*variant, volume: 0.23}
	case 8: // Reed
		p = rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.012 + 0.010*variant, decay: 0.08, sustain: 0.70 + 0.08*variant, release: 0.10 + 0.06*variant, volume: 0.20}
	case 9: // Pipe
		p = rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.006 + 0.012*variant, decay: 0.06, sustain: 0.76 + 0.08*variant, release: 0.12 + 0.10*variant, volume: 0.18}
	case 10: // Synth lead
		p = rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001 + 0.008*variant, decay: 0.06 + 0.04*variant, sustain: 0.60 + 0.20*variant, release: 0.08 + 0.08*variant, volume: 0.23}
	case 11: // Synth pad
		p = rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.050 + 0.080*variant, decay: 0.20, sustain: 0.68 + 0.12*variant, release: 0.35 + 0.25*variant, volume: 0.17}
	case 12: // Synth effects
		p = rawlandMiniPatch{waveform: WAVE_SAWTOOTH, attack: 0.010 + 0.050*variant, decay: 0.20 + 0.10*variant, sustain: 0.40 + 0.20*variant, release: 0.25 + 0.20*variant, volume: 0.19}
	case 13: // Ethnic
		p = rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.003 + 0.010*variant, decay: 0.12 + 0.08*variant, sustain: 0.35 + 0.20*variant, release: 0.10 + 0.08*variant, volume: 0.21}
	case 14: // Percussive melodic
		p = rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001, decay: 0.08 + 0.06*variant, sustain: 0.18 + 0.10*variant, release: 0.05 + 0.06*variant, volume: 0.22}
	default: // Sound effects, still melodic table patches
		p = rawlandMiniPatch{waveform: WAVE_SAWTOOTH, attack: 0.002 + 0.020*variant, decay: 0.10 + 0.12*variant, sustain: 0.25 + 0.25*variant, release: 0.10 + 0.18*variant, volume: 0.20}
	}
	switch program % 4 {
	case 1:
		p.volume *= 0.92
	case 2:
		p.release *= 1.18
	case 3:
		p.decay *= 0.82
	}
	return p
}

func initRawlandMiniDrums() {
	for note := range rawlandMiniDrums {
		rawlandMiniDrums[note] = rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.001, decay: 0.035, sustain: 0, release: 0.040, volume: 0.18, percussion: true}
	}
	for note := 35; note <= 81; note++ {
		rawlandMiniDrums[note] = drumPatchForNote(note)
	}
}

func drumPatchForNote(note int) rawlandMiniPatch {
	switch note {
	case 35, 36: // Acoustic/electric kick
		return rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.001, decay: 0.060, sustain: 0, release: 0.080, volume: 0.34, percussion: true}
	case 37, 38, 39, 40: // Side stick, snares, clap
		return rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.001, decay: 0.045 + 0.008*float32(note-37), sustain: 0, release: 0.055 + 0.006*float32(note-37), volume: 0.27, percussion: true}
	case 41, 43, 45, 47, 48, 50: // Toms
		return rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001, decay: 0.070, sustain: 0, release: 0.095, volume: 0.25, percussion: true}
	case 42, 44: // Closed/pedal hat
		return rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.001, decay: 0.018, sustain: 0, release: 0.020, volume: 0.18, percussion: true}
	case 46: // Open hat
		return rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.001, decay: 0.070, sustain: 0, release: 0.110, volume: 0.17, percussion: true}
	case 49, 52, 55, 57, 59: // Cymbals
		return rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.002, decay: 0.120 + 0.020*float32(note%3), sustain: 0, release: 0.220 + 0.030*float32(note%3), volume: 0.16, percussion: true}
	case 51, 53: // Ride bells
		return rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001, decay: 0.090, sustain: 0.05, release: 0.180, volume: 0.15, percussion: true}
	case 54, 56, 58, 60: // Tambourine/cowbell/vibraslap/bongo high
		return rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001, decay: 0.035 + 0.010*float32(note%4), sustain: 0, release: 0.050 + 0.012*float32(note%4), volume: 0.19, percussion: true}
	case 61, 62, 63, 64, 67, 68: // Congas/bongos/agogo
		return rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.001, decay: 0.055 + 0.010*float32(note%2), sustain: 0, release: 0.080 + 0.015*float32(note%2), volume: 0.21, percussion: true}
	case 65, 66, 69, 70, 73, 74, 78, 79: // Timbales, cabasa, maracas, cuica, guiro
		return rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.001, decay: 0.026 + 0.007*float32(note%5), sustain: 0, release: 0.035 + 0.008*float32(note%5), volume: 0.16, percussion: true}
	case 71, 72, 75, 76, 77: // Whistle/claves/woodblock
		return rawlandMiniPatch{waveform: WAVE_SQUARE, attack: 0.001, decay: 0.030 + 0.009*float32(note%3), sustain: 0, release: 0.040 + 0.010*float32(note%3), volume: 0.18, percussion: true}
	case 80, 81: // Triangle
		return rawlandMiniPatch{waveform: WAVE_TRIANGLE, attack: 0.001, decay: 0.120, sustain: 0.03, release: 0.260, volume: 0.14, percussion: true}
	default:
		return rawlandMiniPatch{waveform: WAVE_NOISE, attack: 0.001, decay: 0.035, sustain: 0, release: 0.040, volume: 0.18, percussion: true}
	}
}

func patchForNote(ch, program, note uint8) rawlandMiniPatch {
	if ch == 9 {
		return rawlandMiniDrums[note]
	}
	return rawlandMiniPatches[program]
}

type midiVoice struct {
	active            bool
	releasing         bool
	channel           uint8
	note              uint8
	velocity          uint8
	program           uint8
	priority          int
	startOrder        int64
	phase             float32
	freq              float32
	level             float32
	ageSamples        int
	releasePos        int
	releaseStartLevel float32
	patch             rawlandMiniPatch
}

type MIDIEngine struct {
	sound      *SoundChip
	sampleRate int

	mu         sync.Mutex
	file       *MIDIFile
	eventIndex int
	position   int64
	playing    bool
	paused     bool
	loop       bool
	volume     uint8
	programs   [16]uint8
	chanVolume [16]uint8
	expression [16]uint8
	pitchBend  [16]int
	voices     [midiMaxVoices]midiVoice
	order      int64
	noiseState uint32
	currentBPM int
}

func NewMIDIEngine(sound *SoundChip, sampleRate int) *MIDIEngine {
	e := &MIDIEngine{sound: sound, sampleRate: sampleRate, volume: 255, noiseState: 1, currentBPM: 120}
	e.resetChannelStateLocked()
	return e
}

func (e *MIDIEngine) LoadMIDI(file *MIDIFile) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.file = file
	e.eventIndex = 0
	e.position = 0
	e.clearVoicesLocked()
	e.resetChannelStateLocked()
	e.currentBPM = 120
	if file != nil {
		e.currentBPM = file.TempoBPMAtSample(0)
	}
}

func (e *MIDIEngine) SetPlaying(playing bool) {
	e.mu.Lock()
	e.playing = playing
	if !playing {
		e.clearVoicesLocked()
	}
	e.mu.Unlock()
	if e.sound != nil {
		if playing {
			e.sound.RegisterSampleTicker("midi", e)
			e.sound.RegisterSampleMixer("midi", e)
		} else {
			e.sound.UnregisterSampleTicker("midi")
			e.sound.UnregisterSampleMixer("midi")
		}
	}
}

func (e *MIDIEngine) IsPlaying() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playing
}

func (e *MIDIEngine) SetPaused(paused bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.paused = paused
}

func (e *MIDIEngine) SetLoop(loop bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loop = loop
}

func (e *MIDIEngine) SetVolume(volume uint8) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.volume = volume
}

func (e *MIDIEngine) PositionSamples() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.position
}

func (e *MIDIEngine) CurrentTempoBPM() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.currentBPM
}

func (e *MIDIEngine) ActiveVoiceCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for i := range e.voices {
		if e.voices[i].active && !e.voices[i].releasing {
			n++
		}
	}
	return n
}

func (e *MIDIEngine) HasActiveNote(note uint8) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.voices {
		v := &e.voices[i]
		if v.active && !v.releasing && v.note == note {
			return true
		}
	}
	return false
}

func (e *MIDIEngine) TickSample() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.playing || e.paused || e.file == nil {
		return
	}
	for e.eventIndex < len(e.file.Events) && e.file.Events[e.eventIndex].SampleTime <= e.position {
		e.applyEventLocked(e.file.Events[e.eventIndex])
		e.eventIndex++
	}
	e.currentBPM = e.file.TempoBPMAtSample(e.position)
	e.position++
	if e.playbackCompleteLocked() {
		if e.loop {
			e.position = 0
			e.eventIndex = 0
			e.clearVoicesLocked()
		} else {
			e.playing = false
			e.clearVoicesLocked()
			if e.sound != nil {
				go func() {
					e.sound.UnregisterSampleTicker("midi")
					e.sound.UnregisterSampleMixer("midi")
				}()
			}
		}
	}
}

func (e *MIDIEngine) playbackCompleteLocked() bool {
	if e.file == nil {
		return true
	}
	if e.file.DurationSamples > 0 {
		return e.position > e.file.DurationSamples
	}
	return e.eventIndex >= len(e.file.Events)
}

func (e *MIDIEngine) MixSample() float32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.playing || e.paused {
		return 0
	}
	var sum float32
	active := 0
	for i := range e.voices {
		v := &e.voices[i]
		if !v.active {
			continue
		}
		s := e.voiceSampleLocked(v)
		sum += s
		active++
	}
	if active == 0 {
		return 0
	}
	return clampF32(sum/float32(midiMaxVoices), -0.8, 0.8)
}

func (e *MIDIEngine) applyEventLocked(ev MIDIEvent) {
	ch := ev.Channel & 0x0F
	switch ev.Kind {
	case MIDIEventProgramChange:
		e.programs[ch] = ev.Program
	case MIDIEventControlChange:
		if ev.Controller == 7 {
			e.chanVolume[ch] = uint8(clampInt(ev.Value, 0, 127))
		} else if ev.Controller == 11 {
			e.expression[ch] = uint8(clampInt(ev.Value, 0, 127))
		}
	case MIDIEventPitchBend:
		e.pitchBend[ch] = clampInt(ev.Value, 0, 16383)
	case MIDIEventNoteOn:
		if ev.Velocity == 0 {
			e.releaseNoteLocked(ch, ev.Note)
		} else {
			e.startNoteLocked(ch, ev.Note, ev.Velocity)
		}
	case MIDIEventNoteOff:
		e.releaseNoteLocked(ch, ev.Note)
	}
}

func (e *MIDIEngine) startNoteLocked(ch, note, velocity uint8) {
	idx := e.selectVoiceLocked(ch, note, velocity)
	prog := e.programs[ch]
	patch := patchForNote(ch, prog, note)
	e.order++
	pbSemis := (float32(e.pitchBend[ch]) - 8192.0) / 8192.0 * 2.0
	freq := float32(440.0 * math.Pow(2, (float64(note)-69.0+float64(pbSemis))/12.0))
	e.voices[idx] = midiVoice{
		active:     true,
		channel:    ch,
		note:       note,
		velocity:   velocity,
		program:    prog,
		priority:   midiPriority(ch, note, velocity),
		startOrder: e.order,
		freq:       freq,
		patch:      patch,
	}
}

func (e *MIDIEngine) releaseNoteLocked(ch, note uint8) {
	for i := range e.voices {
		v := &e.voices[i]
		if v.active && !v.releasing && v.channel == ch && v.note == note {
			v.releaseStartLevel = e.voiceEnvelopeLevelLocked(v)
			v.releasing = true
			v.releasePos = 0
		}
	}
}

func (e *MIDIEngine) selectVoiceLocked(ch, note, velocity uint8) int {
	for i := range e.voices {
		v := &e.voices[i]
		if v.active && v.releasing && v.channel == ch && v.note == note {
			return i
		}
	}
	for i := range e.voices {
		if !e.voices[i].active {
			return i
		}
	}
	for i := range e.voices {
		if e.voices[i].releasing {
			return i
		}
	}
	newPrio := midiPriority(ch, note, velocity)
	best := 0
	for i := 1; i < len(e.voices); i++ {
		a := e.voices[i]
		b := e.voices[best]
		if a.priority < b.priority || (a.priority == b.priority && a.startOrder < b.startOrder) {
			best = i
		}
	}
	_ = newPrio
	return best
}

func midiPriority(ch, note, velocity uint8) int {
	p := 1
	if velocity >= 100 {
		p++
	}
	if note <= 35 {
		p++
	}
	if ch == 9 {
		p += 2
	}
	return p
}

func (e *MIDIEngine) voiceSampleLocked(v *midiVoice) float32 {
	vol := float32(v.velocity) / 127.0
	vol *= float32(e.chanVolume[v.channel]) / 127.0
	vol *= float32(e.expression[v.channel]) / 127.0
	vol *= float32(e.volume) / 255.0
	vol *= v.patch.volume
	envLevel := e.voiceEnvelopeLevelLocked(v)
	if v.releasing {
		releaseSamples := max(1, int(v.patch.release*float32(e.sampleRate)))
		v.releasePos++
		if v.releasePos >= releaseSamples {
			*v = midiVoice{}
			return 0
		}
	}
	vol *= envLevel
	inc := v.freq / float32(e.sampleRate)
	v.phase += inc
	v.ageSamples++
	for v.phase >= 1 {
		v.phase -= 1
	}
	switch v.patch.waveform {
	case WAVE_TRIANGLE:
		return (4*float32(math.Abs(float64(v.phase-0.5))) - 1) * vol
	case WAVE_SAWTOOTH:
		return (2*v.phase - 1) * vol
	case WAVE_NOISE:
		e.noiseState = stepNoiseLFSR(NOISE_MODE_WHITE, e.noiseState)
		return (float32(e.noiseState&1)*2 - 1) * vol
	default:
		if v.phase < 0.5 {
			return vol
		}
		return -vol
	}
}

func (e *MIDIEngine) voiceEnvelopeLevelLocked(v *midiVoice) float32 {
	if v.patch.volume <= 0 {
		return 0
	}
	if v.releasing {
		releaseSamples := max(1, int(v.patch.release*float32(e.sampleRate)))
		level := v.releaseStartLevel * (1 - float32(v.releasePos)/float32(releaseSamples))
		return clampF32(level, 0, 1)
	}
	attackSamples := max(1, int(v.patch.attack*float32(e.sampleRate)))
	decaySamples := max(1, int(v.patch.decay*float32(e.sampleRate)))
	if v.ageSamples < attackSamples {
		return clampF32(float32(v.ageSamples)/float32(attackSamples), 0, 1)
	}
	decayPos := v.ageSamples - attackSamples
	if decayPos < decaySamples {
		t := float32(decayPos) / float32(decaySamples)
		return clampF32(1-(1-v.patch.sustain)*t, 0, 1)
	}
	return clampF32(v.patch.sustain, 0, 1)
}

func (e *MIDIEngine) clearVoicesLocked() {
	for i := range e.voices {
		e.voices[i] = midiVoice{}
	}
}

func (e *MIDIEngine) resetChannelStateLocked() {
	for i := range e.programs {
		e.programs[i] = 0
		e.chanVolume[i] = 127
		e.expression[i] = 127
		e.pitchBend[i] = 8192
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
