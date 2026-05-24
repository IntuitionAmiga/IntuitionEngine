package main

import "testing"

func TestMediaLoaderStopPlayersOnlyStopsAllMusicPlayers(t *testing.T) {
	sound, err := NewSoundChip(AUDIO_BACKEND_NULL)
	if err != nil {
		t.Fatalf("NewSoundChip failed: %v", err)
	}
	t.Cleanup(sound.Stop)

	psgPlayer := NewPSGPlayer(NewPSGEngine(sound, SAMPLE_RATE))
	sidPlayer := NewSIDPlayer(NewSIDEngine(sound, SAMPLE_RATE))
	tedPlayer := NewTEDPlayer(NewTEDEngine(sound, SAMPLE_RATE))
	pokeyPlayer := NewPOKEYPlayer(NewPOKEYEngine(sound, SAMPLE_RATE))
	ahxPlayer := NewAHXPlayer(sound, SAMPLE_RATE)
	modPlayer := NewMODPlayer(sound, SAMPLE_RATE)
	wavPlayer := NewWAVPlayer(sound, SAMPLE_RATE)
	midiPlayer := NewMIDIPlayer(sound, SAMPLE_RATE)
	loader := NewMediaLoader(nil, sound, "", psgPlayer, sidPlayer, tedPlayer, ahxPlayer, pokeyPlayer, modPlayer, wavPlayer, midiPlayer)

	psgPlayer.engine.SetPlaying(true)
	sidPlayer.engine.SetPlaying(true)
	tedPlayer.engine.SetPlaying(true)
	pokeyPlayer.engine.SetPlaying(true)
	ahxPlayer.Play()
	modPlayer.Play()
	wavPlayer.Play()
	midiPlayer.Play()
	if !midiPlayer.IsPlaying() || !sound.HasSampleTicker("midi") || !sound.HasSampleMixer("midi") {
		t.Fatal("test setup failed to start MIDI playback")
	}

	loader.stopPlayersOnly()

	if psgPlayer.engine.IsPlaying() {
		t.Fatal("PSG player still playing after stopPlayersOnly")
	}
	if sidPlayer.IsPlaying() {
		t.Fatal("SID player still playing after stopPlayersOnly")
	}
	if tedPlayer.IsPlaying() {
		t.Fatal("TED player still playing after stopPlayersOnly")
	}
	if pokeyPlayer.IsPlaying() {
		t.Fatal("POKEY player still playing after stopPlayersOnly")
	}
	if ahxPlayer.IsPlaying() {
		t.Fatal("AHX player still playing after stopPlayersOnly")
	}
	if modPlayer.IsPlaying() {
		t.Fatal("MOD player still playing after stopPlayersOnly")
	}
	if wavPlayer.IsPlaying() || sound.HasSampleTicker(wavTickerKey) {
		t.Fatal("WAV player still playing after stopPlayersOnly")
	}
	if midiPlayer.IsPlaying() || sound.HasSampleTicker("midi") || sound.HasSampleMixer("midi") {
		t.Fatal("MIDI player still playing after stopPlayersOnly")
	}
}
