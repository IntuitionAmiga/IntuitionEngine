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

// Atari ST/Falcon MIDI port: MC6850 ACIA (EmuTOS-only bridge into LiveMIDI).
//
// EmuTOS observes this as $FFFC04/$FFFC06, usually sign-extended by IE M68K to
// 0xFFFFFC04/06. MachineBus normalizes those high addresses to the low-16 alias
// before handler lookup, so the low-16 constants are the bus-canonical mapping
// keys. The 24-bit forms are the Atari ST hardware I/O addresses, which do not
// pass through sign-extension normalization and must be mapped explicitly.
const (
	ATARI_MIDI_ACIA_CTRL = 0x0000FC04 // RS=0: write control, read status
	ATARI_MIDI_ACIA_DATA = 0x0000FC06 // RS=1: write TX data (MIDI byte), read RX

	ATARI_MIDI_ACIA_CTRL_SIGNEXT = 0xFFFFFC04 // guest-visible sign-extended address
	ATARI_MIDI_ACIA_DATA_SIGNEXT = 0xFFFFFC06 // guest-visible sign-extended address

	ATARI_MIDI_ACIA_CTRL_24BIT = 0x00FFFC04 // 24-bit Atari hardware alias
	ATARI_MIDI_ACIA_DATA_24BIT = 0x00FFFC06 // 24-bit Atari hardware alias
)

const (
	ACIA_SR_RDRF = 0x01 // status bit0: receive data register full
	ACIA_SR_TDRE = 0x02 // status bit1: transmit data register empty (ready)

	ACIA_CTRL_MASTER_RESET = 0x03 // CR1:CR0 == 11 resets MC6850 state
)

const RawlandMiniPatchTableName = "RawlandMini"
