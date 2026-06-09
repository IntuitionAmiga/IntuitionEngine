package main

import "testing"

func TestMIDIEngine_NoteVolumePitchLoopPauseStopAndVoiceStealing(t *testing.T) {
	sound, err := NewSoundChip(AUDIO_BACKEND_NULL)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewMIDIEngine(sound, 44100)
	file := &MIDIFile{
		Division:        96,
		PatchTableName:  "RawlandMini",
		DurationSamples: 4096,
		Events: []MIDIEvent{
			{Kind: MIDIEventProgramChange, Program: 1},
			{Kind: MIDIEventControlChange, Controller: 7, Value: 100},
			{Kind: MIDIEventControlChange, Controller: 11, Value: 80},
			{Kind: MIDIEventPitchBend, Value: 8192 + 1024},
			{Kind: MIDIEventNoteOn, Note: 60, Velocity: 100},
			{Kind: MIDIEventNoteOff, Note: 60, SampleTime: 512},
		},
	}
	engine.LoadMIDI(file)
	engine.SetPlaying(true)
	if !sound.HasSampleTicker("midi") || !sound.HasSampleMixer("midi") {
		t.Fatal("MIDI engine did not register ticker and mixer")
	}
	for i := 0; i < 32; i++ {
		engine.TickSample()
	}
	if got := engine.ActiveVoiceCount(); got != 1 {
		t.Fatalf("active voices=%d, want 1", got)
	}
	engine.MixSample()
	if sample := engine.MixSample(); sample == 0 {
		t.Fatal("expected non-zero MIDI sample")
	}
	engine.SetPaused(true)
	pos := engine.PositionSamples()
	for i := 0; i < 16; i++ {
		engine.TickSample()
	}
	if engine.PositionSamples() != pos {
		t.Fatal("paused engine advanced")
	}
	engine.SetPaused(false)
	for i := 0; i < 600; i++ {
		engine.TickSample()
	}
	if got := engine.ActiveVoiceCount(); got != 0 {
		t.Fatalf("active voices after note-off=%d, want 0", got)
	}

	dense := &MIDIFile{DurationSamples: 2048}
	for i := 0; i < 12; i++ {
		dense.Events = append(dense.Events, MIDIEvent{Kind: MIDIEventNoteOn, Note: uint8(48 + i), Velocity: 90})
	}
	engine.LoadMIDI(dense)
	engine.SetPlaying(true)
	engine.TickSample()
	if got := engine.ActiveVoiceCount(); got != 10 {
		t.Fatalf("active voices after dense chord=%d, want 10", got)
	}
	if engine.HasActiveNote(48) || engine.HasActiveNote(49) {
		t.Fatal("deterministic voice stealing should remove oldest low-priority voices first")
	}
	engine.SetLoop(true)
	for i := 0; i < 2100; i++ {
		engine.TickSample()
	}
	if !engine.IsPlaying() {
		t.Fatal("looping engine stopped")
	}
	engine.SetPlaying(false)
	if sound.HasSampleTicker("midi") || sound.HasSampleMixer("midi") {
		t.Fatal("MIDI engine did not unregister ticker and mixer on stop")
	}
}

func TestMIDIMixer_DoesNotMutateManualSoundChipChannelState(t *testing.T) {
	sound, err := NewSoundChip(AUDIO_BACKEND_NULL)
	if err != nil {
		t.Fatal(err)
	}
	sound.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	sound.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
	sound.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 1)
	beforeFreq := sound.HandleRegisterRead(FLEX_CH0_BASE + FLEX_OFF_FREQ)
	beforeWave := sound.HandleRegisterRead(FLEX_CH0_BASE + FLEX_OFF_WAVE_TYPE)
	beforeEnable := sound.HandleRegisterRead(FLEX_CH0_BASE + FLEX_OFF_CTRL)

	engine := NewMIDIEngine(sound, 44100)
	engine.LoadMIDI(&MIDIFile{DurationSamples: 1000, Events: []MIDIEvent{{Kind: MIDIEventNoteOn, Note: 60, Velocity: 100}}})
	engine.SetPlaying(true)
	for i := 0; i < 32; i++ {
		sound.ReadSample()
	}
	engine.SetPlaying(false)

	if got := sound.HandleRegisterRead(FLEX_CH0_BASE + FLEX_OFF_FREQ); got != beforeFreq {
		t.Fatalf("FREQ_CH0 changed: %d want %d", got, beforeFreq)
	}
	if got := sound.HandleRegisterRead(FLEX_CH0_BASE + FLEX_OFF_WAVE_TYPE); got != beforeWave {
		t.Fatalf("WAVEFORM_CH0 changed: %d want %d", got, beforeWave)
	}
	if got := sound.HandleRegisterRead(FLEX_CH0_BASE + FLEX_OFF_CTRL); got != beforeEnable {
		t.Fatalf("ENABLE_CH0 changed: %d want %d", got, beforeEnable)
	}
}

func TestMIDIEngine_LoadResetsChannelState(t *testing.T) {
	engine := NewMIDIEngine(nil, SAMPLE_RATE)
	engine.LoadMIDI(&MIDIFile{
		DurationSamples: 64,
		Events: []MIDIEvent{
			{Kind: MIDIEventProgramChange, Channel: 0, Program: 33},
			{Kind: MIDIEventControlChange, Channel: 0, Controller: 7, Value: 0},
			{Kind: MIDIEventControlChange, Channel: 0, Controller: 11, Value: 0},
			{Kind: MIDIEventPitchBend, Channel: 0, Value: 4096},
			{Kind: MIDIEventNoteOn, Channel: 0, Note: 60, Velocity: 100},
		},
	})
	engine.SetPlaying(true)
	engine.TickSample()
	engine.SetPlaying(false)

	engine.LoadMIDI(&MIDIFile{
		DurationSamples: 64,
		Events:          []MIDIEvent{{Kind: MIDIEventNoteOn, Channel: 0, Note: 60, Velocity: 100}},
	})
	engine.SetPlaying(true)
	engine.TickSample()
	engine.MixSample()
	if sample := engine.MixSample(); sample == 0 {
		t.Fatal("new file inherited muted channel state")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.fileState.programs[0] != 0 || engine.fileState.chanVolume[0] != 127 || engine.fileState.expression[0] != 127 || engine.fileState.pitchBend[0] != 8192 {
		t.Fatalf("channel state leaked after load: program=%d vol=%d expr=%d bend=%d",
			engine.fileState.programs[0], engine.fileState.chanVolume[0], engine.fileState.expression[0], engine.fileState.pitchBend[0])
	}
}

func TestMIDIEngine_LiveChannelStateIsolatedFromFilePlayback(t *testing.T) {
	engine := NewMIDIEngine(nil, SAMPLE_RATE)
	engine.LoadMIDI(&MIDIFile{
		DurationSamples: 128,
		Events: []MIDIEvent{
			{Kind: MIDIEventProgramChange, Channel: 0, Program: 5},
			{Kind: MIDIEventNoteOn, Channel: 0, Note: 60, Velocity: 100},
			{Kind: MIDIEventNoteOn, Channel: 0, Note: 62, Velocity: 100, SampleTime: 1},
		},
	})
	engine.SetPlaying(true)
	engine.TickSample()

	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventProgramChange, Channel: 0, Program: 40})
	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventControlChange, Channel: 0, Controller: 7, Value: 12})
	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventControlChange, Channel: 0, Controller: 11, Value: 34})
	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventPitchBend, Channel: 0, Value: 4096})
	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventNoteOn, Channel: 0, Note: 67, Velocity: 100})
	engine.TickSample()

	engine.mu.Lock()
	defer engine.mu.Unlock()
	fileVoice := findMIDITestVoiceLocked(engine, 62, false)
	if fileVoice == nil {
		t.Fatal("later file note did not start")
	}
	if fileVoice.program != 5 {
		t.Fatalf("file note inherited live program: got %d want 5", fileVoice.program)
	}
	liveVoice := findMIDITestVoiceLocked(engine, 67, true)
	if liveVoice == nil {
		t.Fatal("live note did not start")
	}
	if liveVoice.program != 40 {
		t.Fatalf("live note ignored live program: got %d want 40", liveVoice.program)
	}
	if engine.fileState.chanVolume[0] != 127 || engine.fileState.expression[0] != 127 || engine.fileState.pitchBend[0] != 8192 {
		t.Fatalf("live controllers leaked into file state: vol=%d expr=%d bend=%d",
			engine.fileState.chanVolume[0], engine.fileState.expression[0], engine.fileState.pitchBend[0])
	}
	if engine.liveState.chanVolume[0] != 12 || engine.liveState.expression[0] != 34 || engine.liveState.pitchBend[0] != 4096 {
		t.Fatalf("live state not applied: vol=%d expr=%d bend=%d",
			engine.liveState.chanVolume[0], engine.liveState.expression[0], engine.liveState.pitchBend[0])
	}
}

func TestMIDIEngine_ZeroDurationFileStops(t *testing.T) {
	engine := NewMIDIEngine(nil, SAMPLE_RATE)
	engine.LoadMIDI(&MIDIFile{})
	engine.SetPlaying(true)
	engine.TickSample()
	if engine.IsPlaying() {
		t.Fatal("zero-duration MIDI file kept playing")
	}
}

func TestMIDIEngine_FileStopPreservesLiveVoices(t *testing.T) {
	engine := NewMIDIEngine(nil, SAMPLE_RATE)
	engine.LoadMIDI(&MIDIFile{
		DurationSamples: 256,
		Events: []MIDIEvent{
			{Kind: MIDIEventNoteOn, Note: 60, Velocity: 100},
		},
	})
	engine.SetPlaying(true)
	engine.TickSample()
	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventNoteOn, Note: 64, Velocity: 100})

	engine.SetPlaying(false)

	if engine.HasActiveNote(60) {
		t.Fatal("file-owned note survived file stop")
	}
	if !engine.HasActiveNote(64) {
		t.Fatal("live note was cut off by file stop")
	}
	engine.MixSample()
	if sample := engine.MixSample(); sample == 0 {
		t.Fatal("live note was silent after file stop")
	}
}

func TestMIDIEngine_FilePauseDoesNotMuteLiveVoices(t *testing.T) {
	engine := NewMIDIEngine(nil, SAMPLE_RATE)
	engine.LoadMIDI(&MIDIFile{
		DurationSamples: 256,
		Events: []MIDIEvent{
			{Kind: MIDIEventNoteOn, Note: 60, Velocity: 100},
		},
	})
	engine.SetPlaying(true)
	engine.TickSample()
	engine.SetPaused(true)
	engine.ApplyLiveEvent(MIDIEvent{Kind: MIDIEventNoteOn, Note: 64, Velocity: 100})

	for i := 0; i < 8; i++ {
		if sample := engine.MixSample(); sample != 0 {
			return
		}
	}
	t.Fatal("live note was silent while file playback was paused")
}

// Live MIDI takes precedence over the file player: when every voice is sounding
// and one must be stolen, a live voice is evicted only if no file voice is
// available, regardless of which source the incoming note comes from.
func TestMIDIEngine_LiveVoicePrecedenceOverFile(t *testing.T) {
	e := NewMIDIEngine(nil, SAMPLE_RATE)
	n := len(e.voices)
	fill := func(isLive func(i int) bool) {
		for i := 0; i < n; i++ {
			e.voices[i] = midiVoice{
				active:     true,
				releasing:  false,
				channel:    uint8(i),
				note:       uint8(40 + i),
				priority:   1,
				startOrder: int64(i),
				live:       isLive(i),
			}
		}
	}

	// One file voice among live voices: an incoming live note must steal that
	// file voice, never a live one.
	fileIdx := 4
	fill(func(i int) bool { return i != fileIdx })
	if got := e.selectVoiceLocked(0, 100, 100, true); got != fileIdx {
		t.Fatalf("live note stole voice %d, want the file voice %d", got, fileIdx)
	}

	// One live voice among file voices: even an incoming file note must protect
	// the live voice and steal a file voice instead.
	liveIdx := 7
	fill(func(i int) bool { return i == liveIdx })
	if got := e.selectVoiceLocked(0, 100, 100, false); got == liveIdx {
		t.Fatalf("file note evicted the protected live voice %d", liveIdx)
	}

	// All voices live: stealing is unavoidable but must still return a valid voice.
	fill(func(i int) bool { return true })
	if got := e.selectVoiceLocked(0, 100, 100, true); got < 0 || got >= n {
		t.Fatalf("all-live steal returned invalid index %d", got)
	}
}

func TestRawlandMini_MelodicPrograms35To81AreNotDrums(t *testing.T) {
	for program := uint8(35); program <= 81; program++ {
		patch := patchForNote(0, program, 60)
		if patch.percussion {
			t.Fatalf("melodic channel program %d returned percussion patch", program)
		}
		if patch.waveform == WAVE_NOISE {
			t.Fatalf("melodic channel program %d returned noise drum-like patch", program)
		}
	}
}

func TestRawlandMini_Channel9UsesDrumTableByNote(t *testing.T) {
	melodic := patchForNote(0, 40, 38)
	drum := patchForNote(9, 40, 38)
	if melodic.percussion {
		t.Fatal("melodic program 40 unexpectedly percussion")
	}
	if !drum.percussion {
		t.Fatal("channel 9 note 38 did not return percussion patch")
	}
	if drum == melodic {
		t.Fatal("channel 9 did not use a separate note-indexed drum patch")
	}
	if got, want := drum, rawlandMiniDrums[38]; got != want {
		t.Fatalf("channel 9 note patch = %#v, want drum table %#v", got, want)
	}
}

func TestRawlandMini_MelodicTableCoversAllProgramsWithVariety(t *testing.T) {
	signatures := map[rawlandMiniPatch]bool{}
	for program, patch := range rawlandMiniPatches {
		if patch.percussion {
			t.Fatalf("program %d is marked percussion", program)
		}
		if patch.volume <= 0 || patch.release <= 0 {
			t.Fatalf("program %d has unusable patch %#v", program, patch)
		}
		signatures[patch] = true
	}
	if len(signatures) < 24 {
		t.Fatalf("melodic patch table has %d unique patches, want at least 24", len(signatures))
	}
}

func TestRawlandMini_DrumTableHasDistinctFamilies(t *testing.T) {
	signatures := map[rawlandMiniPatch]bool{}
	for note := 35; note <= 81; note++ {
		patch := rawlandMiniDrums[note]
		if !patch.percussion {
			t.Fatalf("drum note %d is not marked percussion", note)
		}
		if patch.volume <= 0 || patch.release <= 0 {
			t.Fatalf("drum note %d has unusable patch %#v", note, patch)
		}
		signatures[patch] = true
	}
	if len(signatures) < 10 {
		t.Fatalf("drum table has %d unique patches for notes 35..81, want at least 10", len(signatures))
	}
	checkDifferent := func(a, b uint8, label string) {
		t.Helper()
		if rawlandMiniDrums[a] == rawlandMiniDrums[b] {
			t.Fatalf("%s notes %d and %d use identical patches", label, a, b)
		}
	}
	checkDifferent(36, 38, "kick/snare")
	checkDifferent(42, 46, "closed/open hat")
	checkDifferent(49, 56, "cymbal/cowbell")
}

func TestMIDIEngine_ADSREnvelopeUsesPatchFields(t *testing.T) {
	engine := NewMIDIEngine(nil, 1000)
	voice := midiVoice{
		active: true,
		patch: rawlandMiniPatch{
			attack:  0.010,
			decay:   0.020,
			sustain: 0.25,
			release: 0.030,
			volume:  1,
		},
	}

	voice.ageSamples = 0
	if got := engine.voiceEnvelopeLevelLocked(&voice); got != 0 {
		t.Fatalf("attack start envelope = %f, want 0", got)
	}
	voice.ageSamples = 5
	if got := engine.voiceEnvelopeLevelLocked(&voice); got < 0.45 || got > 0.55 {
		t.Fatalf("attack midpoint envelope = %f, want about 0.5", got)
	}
	voice.ageSamples = 10
	if got := engine.voiceEnvelopeLevelLocked(&voice); got != 1 {
		t.Fatalf("attack end envelope = %f, want 1", got)
	}
	voice.ageSamples = 20
	if got := engine.voiceEnvelopeLevelLocked(&voice); got < 0.60 || got > 0.70 {
		t.Fatalf("decay midpoint envelope = %f, want between peak and sustain", got)
	}
	voice.ageSamples = 40
	if got := engine.voiceEnvelopeLevelLocked(&voice); got != 0.25 {
		t.Fatalf("sustain envelope = %f, want 0.25", got)
	}
	voice.releasing = true
	voice.releaseStartLevel = 0.25
	voice.releasePos = 15
	if got := engine.voiceEnvelopeLevelLocked(&voice); got < 0.12 || got > 0.13 {
		t.Fatalf("release midpoint envelope = %f, want about 0.125", got)
	}
}

func findMIDITestVoiceLocked(engine *MIDIEngine, note uint8, live bool) *midiVoice {
	for i := range engine.voices {
		v := &engine.voices[i]
		if v.active && !v.releasing && v.note == note && v.live == live {
			return v
		}
	}
	return nil
}
