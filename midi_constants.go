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

const RawlandMiniPatchTableName = "RawlandMini"
