package main

import "testing"

// CAMD bridge tests for the live-MIDI MMIO port.
//
// The AROS CAMD leaf driver (DEVS:Midi/ie) transmits by storing raw MIDI bytes
// to IE_MIDI_LIVE_DATA from the M68K CPU. These tests pin the M68K CPU Write8
// path to the mapped live-MIDI MMIO block: 0xF0BF4 is >= 0xA0000 so it bypasses
// the direct-RAM fast path and must route through the bus MMIO handlers into
// LiveMIDI.Feed. Bus-level parsing/running-status/reset behaviour is already
// covered by midi_live_test.go and is not duplicated here.

// TestM68KLiveMIDIPortSmoke drives a note-on, a running-status note-on, and a
// control reset through the M68K CPU byte path.
func TestM68KLiveMIDIPortSmoke(t *testing.T) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)

	cpu := NewM68KCPU(bus)

	// Note-on ch0 note 0x3C vel 0x64.
	for _, b := range []byte{0x90, 0x3C, 0x64} {
		cpu.Write8(IE_MIDI_LIVE_DATA, b)
	}
	if !eng.HasActiveNote(0x3C) {
		t.Fatalf("M68K data write did not drive engine: note 0x3C not active")
	}
	if !eng.LiveActive() {
		t.Fatalf("LiveActive should be true after note-on")
	}

	// Second note in running status: data bytes only, no status byte.
	for _, b := range []byte{0x40, 0x64} {
		cpu.Write8(IE_MIDI_LIVE_DATA, b)
	}
	if !eng.HasActiveNote(0x3C) || !eng.HasActiveNote(0x40) {
		t.Fatalf("running status note-on failed: 0x3C=%v 0x40=%v",
			eng.HasActiveNote(0x3C), eng.HasActiveNote(0x40))
	}

	// Control reset: all notes off, port goes inactive.
	cpu.Write8(IE_MIDI_LIVE_CTRL, MIDI_LIVE_CTRL_RESET)
	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("reset did not clear voices, got %d", eng.ActiveVoiceCount())
	}
	if eng.LiveActive() {
		t.Fatalf("reset should deactivate the live port")
	}
}

// TestM68KLiveMIDIPortNoShadow asserts the live-MIDI registers are pure MMIO:
// CPU writes to DATA/CTRL must not mirror into backing RAM. Guards the
// MapIONoShadow mapping in LiveMIDI.MapRegisters.
func TestM68KLiveMIDIPortNoShadow(t *testing.T) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)

	cpu := NewM68KCPU(bus)

	for _, b := range []byte{0x90, 0x3C, 0x64} {
		cpu.Write8(IE_MIDI_LIVE_DATA, b)
	}
	cpu.Write8(IE_MIDI_LIVE_CTRL, MIDI_LIVE_CTRL_RESET)

	for _, addr := range []uint32{IE_MIDI_LIVE_DATA, IE_MIDI_LIVE_CTRL} {
		if bus.memory[addr] != 0 {
			t.Fatalf("no-shadow violated: bus.memory[0x%08X] = 0x%02X, want 0",
				addr, bus.memory[addr])
		}
	}
}
