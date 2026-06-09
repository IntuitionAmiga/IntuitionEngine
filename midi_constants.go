package main

// MIDI Player registers (memory-mapped at 0xF0BA0-0xF0BBF).
const (
	MIDI_PLAY_PTR    = 0xF0BA0
	MIDI_PLAY_LEN    = 0xF0BA4
	MIDI_PLAY_CTRL   = 0xF0BA8
	MIDI_PLAY_STATUS = 0xF0BAC
	MIDI_POSITION    = 0xF0BB0
	MIDI_VOLUME      = 0xF0BB4
	MIDI_TEMPO_BPM   = 0xF0BB8
	MIDI_END         = 0xF0BBF
)

const (
	MIDI_STATUS_BUSY    = 0x01
	MIDI_STATUS_ERROR   = 0x02
	MIDI_STATUS_PAUSED  = 0x04
	MIDI_STATUS_LOADING = 0x08
)

// Generic live-MIDI port (raw MIDI byte stream into the shared MIDIEngine).
// CPU-agnostic: usable by every core and by BASIC, distinct from the file
// player above. Mapped in the gap between the WAV block (ends 0xF0BF3) and the
// PSG block (starts 0xF0C00).
const (
	IE_MIDI_LIVE_DATA   = 0xF0BF4 // W: raw MIDI byte (running-status stream); R: 0
	IE_MIDI_LIVE_STATUS = 0xF0BF5 // R: bit 0 = live port active (notes sounding)
	IE_MIDI_LIVE_CTRL   = 0xF0BF6 // W: bit 0 = reset (all notes off + deactivate)
	IE_MIDI_LIVE_END    = 0xF0BF6
)

const (
	MIDI_LIVE_STATUS_ACTIVE = 0x01 // IE_MIDI_LIVE_STATUS bit 0
	MIDI_LIVE_CTRL_RESET    = 0x01 // IE_MIDI_LIVE_CTRL bit 0: all notes off + reset
)

const RawlandMiniPatchTableName = "RawlandMini"
