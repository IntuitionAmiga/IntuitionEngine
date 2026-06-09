package main

import "testing"

// Phase 6 — generic live-MIDI MMIO port.
//
// These tests are CPU-agnostic: they drive the live port either directly or
// through the MachineBus MMIO handlers, never through any specific CPU core,
// proving the port is shared by every core and by BASIC.

// newLiveMIDITestEngine builds a MIDIEngine with no SoundChip, so live events
// mutate voice state deterministically without an audio clock running.
func newLiveMIDITestEngine() *MIDIEngine {
	return NewMIDIEngine(nil, SAMPLE_RATE)
}

// (1) byte-stream parser: running status, note-on/off, CC, program-change,
// pitch-bend, and a message split across multiple writes.
func TestLiveMIDI_ParserRunningStatus(t *testing.T) {
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)

	// Note-on ch0 note60 vel100, then a second note-on via running status
	// (no repeated 0x90 status byte) note64 vel90.
	for _, b := range []byte{0x90, 60, 100, 64, 90} {
		live.Feed(b)
	}
	if got := eng.ActiveVoiceCount(); got != 2 {
		t.Fatalf("running-status note-on: ActiveVoiceCount=%d want 2", got)
	}
	if !eng.HasActiveNote(60) || !eng.HasActiveNote(64) {
		t.Fatalf("running-status note-on: notes 60/64 not both active")
	}

	// Note-off via running status (0x80) for both notes.
	for _, b := range []byte{0x80, 60, 0, 64, 0} {
		live.Feed(b)
	}
	if got := eng.ActiveVoiceCount(); got != 0 {
		t.Fatalf("running-status note-off: ActiveVoiceCount=%d want 0", got)
	}
}

func TestLiveMIDI_ParserNoteOnZeroVelocityIsNoteOff(t *testing.T) {
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	for _, b := range []byte{0x90, 60, 100} {
		live.Feed(b)
	}
	if !eng.HasActiveNote(60) {
		t.Fatalf("note-on did not start note 60")
	}
	// Velocity-zero note-on is a note-off.
	for _, b := range []byte{0x90, 60, 0} {
		live.Feed(b)
	}
	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("velocity-zero note-on should release note")
	}
}

func TestLiveMIDI_ParserProgramAndControlChange(t *testing.T) {
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)

	// Program change ch3 -> program 40 (single data byte).
	live.Feed(0xC3)
	live.Feed(40)
	// Control change ch3 controller 7 (volume) -> 90.
	for _, b := range []byte{0xB3, 7, 90} {
		live.Feed(b)
	}
	// Pitch bend ch3 -> centre (single message, two data bytes).
	for _, b := range []byte{0xE3, 0x00, 0x40} {
		live.Feed(b)
	}
	// Now a note-on on ch3 must use program 40 and channel volume 90.
	for _, b := range []byte{0x93, 67, 100} {
		live.Feed(b)
	}
	if !eng.HasActiveNote(67) {
		t.Fatalf("note-on after prog/CC/bend did not start")
	}
}

// Message split across separate Feed calls (e.g. one MMIO write per byte)
// must reassemble correctly.
func TestLiveMIDI_ParserSplitAcrossWrites(t *testing.T) {
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.Feed(0x90)
	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("status byte alone should not start a note")
	}
	live.Feed(72)
	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("status+note without velocity should not start a note")
	}
	live.Feed(110)
	if !eng.HasActiveNote(72) {
		t.Fatalf("note 72 should be active after full message")
	}
}

// Realtime/system bytes must not corrupt running status.
func TestLiveMIDI_ParserIgnoresRealtimeAndSysex(t *testing.T) {
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.Feed(0x90)
	live.Feed(60)
	live.Feed(0xF8) // realtime clock mid-message: must be ignored
	live.Feed(100)
	if !eng.HasActiveNote(60) {
		t.Fatalf("realtime byte broke note assembly")
	}
	// SysEx block must be swallowed without emitting events.
	for _, b := range []byte{0xF0, 0x7E, 0x7F, 0x09, 0x01, 0xF7} {
		live.Feed(b)
	}
	if eng.ActiveVoiceCount() != 1 {
		t.Fatalf("sysex changed voice count: %d want 1", eng.ActiveVoiceCount())
	}
}

// (2) drive the MMIO registers and assert engine reacts.
func TestLiveMIDI_MMIONoteOnOff(t *testing.T) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)

	writeByte := func(b byte) { bus.Write8(IE_MIDI_LIVE_DATA, b) }

	for _, b := range []byte{0x90, 60, 100} {
		writeByte(b)
	}
	if !eng.HasActiveNote(60) {
		t.Fatalf("MMIO note-on did not start note 60")
	}
	if eng.LiveActive() != true {
		t.Fatalf("LiveActive should be true after a live note-on")
	}
	if got := bus.Read8(IE_MIDI_LIVE_STATUS) & MIDI_LIVE_STATUS_ACTIVE; got == 0 {
		t.Fatalf("STATUS active bit not set while live note sounding")
	}

	for _, b := range []byte{0x80, 60, 0} {
		writeByte(b)
	}
	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("MMIO note-off did not release note")
	}
}

func TestLiveMIDI_CtrlResetClearsVoices(t *testing.T) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)

	for _, b := range []byte{0x90, 60, 100, 64, 100} {
		bus.Write8(IE_MIDI_LIVE_DATA, b)
	}
	if eng.ActiveVoiceCount() != 2 {
		t.Fatalf("setup: want 2 active voices, got %d", eng.ActiveVoiceCount())
	}
	bus.Write8(IE_MIDI_LIVE_CTRL, MIDI_LIVE_CTRL_RESET)
	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("CTRL reset did not clear voices")
	}
	if eng.LiveActive() {
		t.Fatalf("CTRL reset should deactivate the live port")
	}
}

func TestLiveMIDI_BusResetClearsVoicesAndParserState(t *testing.T) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)

	for _, b := range []byte{0x90, 60, 100, 64} {
		bus.Write8(IE_MIDI_LIVE_DATA, b)
	}
	if !eng.HasActiveNote(60) {
		t.Fatalf("setup: note 60 should be active before reset")
	}
	if eng.HasActiveNote(64) {
		t.Fatalf("setup: partial note 64 message should not be active before reset")
	}

	bus.Reset()

	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("bus reset did not clear live voices, got %d", eng.ActiveVoiceCount())
	}
	if eng.LiveActive() {
		t.Fatalf("bus reset should deactivate the live port")
	}

	bus.Write8(IE_MIDI_LIVE_DATA, 100)
	if eng.HasActiveNote(64) {
		t.Fatalf("bus reset leaked the partial live-MIDI parser message")
	}
}

// (3) voice-arbitration: a live note sounds even when the file player is not
// playing, and a live reset does not require the file player.
func TestLiveMIDI_ArbitrationLiveIndependentOfFilePlayer(t *testing.T) {
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)

	// File player not playing.
	if eng.IsPlaying() {
		t.Fatalf("precondition: engine should not be file-playing")
	}
	for _, b := range []byte{0x90, 60, 100} {
		live.Feed(b)
	}
	// Live note active despite file player idle.
	if !eng.HasActiveNote(60) || !eng.LiveActive() {
		t.Fatalf("live note must sound independently of the file player")
	}
	// File player remains not playing; live owns the voices.
	if eng.IsPlaying() {
		t.Fatalf("live activity must not flip the file-player playing flag")
	}
}
