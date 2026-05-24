package main

import "testing"

func TestRuntimeAudioStatusIndicatorsIncludesMIDIAfterPaula(t *testing.T) {
	indicators := runtimeAudioStatusIndicators(runtimeStatusSnapshot{})
	names := make([]string, len(indicators))
	for i, indicator := range indicators {
		names[i] = indicator.name
	}

	paulaIndex := -1
	for i, name := range names {
		if name == "PAULA" {
			paulaIndex = i
			break
		}
	}
	if paulaIndex < 0 {
		t.Fatalf("audio status indicators missing PAULA: %v", names)
	}
	if paulaIndex+2 >= len(names) {
		t.Fatalf("audio status indicators truncated after PAULA: %v", names)
	}
	if names[paulaIndex+1] != "|" || names[paulaIndex+2] != "MIDI" {
		t.Fatalf("audio status indicators after PAULA = %q, %q; want |, MIDI", names[paulaIndex+1], names[paulaIndex+2])
	}
}

func TestRuntimeAudioStatusMIDIOnlyEnabledForMIDIPlayerPlayback(t *testing.T) {
	player := &MIDIPlayer{engine: &MIDIEngine{}}
	engineOnly := runtimeStatusSnapshot{midiEngine: player.engine}
	player.engine.SetPlaying(true)
	if indicatorEnabled(t, runtimeAudioStatusIndicators(engineOnly), "MIDI") {
		t.Fatal("MIDI indicator enabled for bare MIDI engine activity without MIDIPlayer playback")
	}
	player.engine.SetPlaying(false)

	snap := runtimeStatusSnapshot{
		midiEngine: player.engine,
		midiPlayer: player,
	}

	if indicatorEnabled(t, runtimeAudioStatusIndicators(snap), "MIDI") {
		t.Fatal("MIDI indicator enabled before MIDIPlayer playback")
	}

	player.Play()
	if !indicatorEnabled(t, runtimeAudioStatusIndicators(snap), "MIDI") {
		t.Fatal("MIDI indicator disabled while MIDIPlayer is playing")
	}

	player.Stop()
	if indicatorEnabled(t, runtimeAudioStatusIndicators(snap), "MIDI") {
		t.Fatal("MIDI indicator enabled after MIDIPlayer stopped")
	}
}

func indicatorEnabled(t *testing.T, indicators []runtimeStatusIndicator, name string) bool {
	t.Helper()
	for _, indicator := range indicators {
		if indicator.name == name {
			return indicator.enabled
		}
	}
	t.Fatalf("audio status indicators missing %s: %v", name, indicators)
	return false
}
