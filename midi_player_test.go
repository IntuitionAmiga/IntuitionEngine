package main

import (
	"testing"
	"time"
)

func TestMIDIPlayer_MMIOStartPauseLoopVolumeTempoAndStop(t *testing.T) {
	sound, err := NewSoundChip(AUDIO_BACKEND_NULL)
	if err != nil {
		t.Fatal(err)
	}
	player := NewMIDIPlayer(sound, 44100)
	bus, err := NewMachineBusSized(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	player.AttachBus(bus)

	data := testSMF(0, 96, [][]byte{testMTrk(
		0x00, 0xFF, 0x51, 0x03, 0x07, 0xA1, 0x20,
		0x00, 0x90, 0x3C, 0x64,
		0x60, 0x80, 0x3C, 0x00,
		0x00, 0xFF, 0x2F, 0x00,
	)})
	copy(bus.GetMemory()[0x2000:], data)

	player.HandlePlayWrite(MIDI_PLAY_PTR, 0x2000)
	player.HandlePlayWrite(MIDI_PLAY_LEN, uint32(len(data)))
	player.HandlePlayWrite(MIDI_VOLUME, 128)
	player.HandlePlayWrite(MIDI_PLAY_CTRL, 1|4)
	if status := player.HandlePlayRead(MIDI_PLAY_STATUS); status&MIDI_STATUS_LOADING == 0 {
		t.Fatalf("status immediately after async start=%#x, want loading bit", status)
	}

	deadline := time.Now().Add(time.Second)
	for player.HandlePlayRead(MIDI_PLAY_STATUS)&MIDI_STATUS_LOADING != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if status := player.HandlePlayRead(MIDI_PLAY_STATUS); status&MIDI_STATUS_BUSY == 0 || status&MIDI_STATUS_ERROR != 0 || status&MIDI_STATUS_LOADING != 0 {
		t.Fatalf("status after start=%#x, want busy without error", status)
	}
	if got := player.HandlePlayRead(MIDI_VOLUME); got != 128 {
		t.Fatalf("volume=%d, want 128", got)
	}
	if got := player.HandlePlayRead(MIDI_TEMPO_BPM); got != 120 {
		t.Fatalf("tempo=%d, want 120", got)
	}
	player.HandlePlayWrite(MIDI_TEMPO_BPM, 250)
	if got := player.HandlePlayRead(MIDI_TEMPO_BPM); got != 120 {
		t.Fatalf("tempo write changed readback to %d", got)
	}
	player.HandlePlayWrite(MIDI_PLAY_CTRL, 8)
	if status := player.HandlePlayRead(MIDI_PLAY_STATUS); status&MIDI_STATUS_PAUSED == 0 {
		t.Fatalf("status after pause=%#x, want paused bit", status)
	}
	player.HandlePlayWrite(MIDI_PLAY_CTRL, 0)
	if status := player.HandlePlayRead(MIDI_PLAY_STATUS); status&MIDI_STATUS_PAUSED != 0 {
		t.Fatalf("status after resume=%#x, want paused cleared", status)
	}
	player.HandlePlayWrite(MIDI_PLAY_CTRL, 2)
	if status := player.HandlePlayRead(MIDI_PLAY_STATUS); status&MIDI_STATUS_BUSY != 0 {
		t.Fatalf("status after stop=%#x, want not busy", status)
	}
}

func TestMIDIPlayer_LoadingBitClearsAfterParseFailure(t *testing.T) {
	sound, err := NewSoundChip(AUDIO_BACKEND_NULL)
	if err != nil {
		t.Fatal(err)
	}
	player := NewMIDIPlayer(sound, 44100)
	bus, err := NewMachineBusSized(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	player.AttachBus(bus)

	copy(bus.GetMemory()[0x2000:], []byte("not midi"))
	player.HandlePlayWrite(MIDI_PLAY_PTR, 0x2000)
	player.HandlePlayWrite(MIDI_PLAY_LEN, 8)
	player.HandlePlayWrite(MIDI_PLAY_CTRL, 1)

	deadline := time.Now().Add(time.Second)
	for player.HandlePlayRead(MIDI_PLAY_STATUS)&MIDI_STATUS_LOADING != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := player.HandlePlayRead(MIDI_PLAY_STATUS)
	if status&MIDI_STATUS_LOADING != 0 {
		t.Fatalf("loading bit remained set after parse failure: status=%#x", status)
	}
	if status&MIDI_STATUS_ERROR == 0 {
		t.Fatalf("parse failure status=%#x, want error bit", status)
	}
}

func TestDetectMediaType_MIDIExtensions(t *testing.T) {
	for _, path := range []string{"song.mid", "song.midi", "doom.mus", "SONG.MID"} {
		if got := detectMediaType(path); got != MEDIA_TYPE_MIDI {
			t.Fatalf("detectMediaType(%q)=%d, want MEDIA_TYPE_MIDI", path, got)
		}
	}
	if MEDIA_TYPE_MIDI != 8 {
		t.Fatalf("MEDIA_TYPE_MIDI=%d, want 8", MEDIA_TYPE_MIDI)
	}
}
