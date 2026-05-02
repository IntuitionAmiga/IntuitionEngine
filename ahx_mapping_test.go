// ahx_mapping_test.go - AHX voice state to SoundChip mapping tests.

package main

import "testing"

func newLoadedMappingEngine(t *testing.T) (*AHXEngine, *SoundChip) {
	t.Helper()
	chip := newTestSoundChip()
	engine := NewAHXEngine(chip, 44100)
	song := &AHXFile{
		Revision:        0,
		SpeedMultiplier: 1,
		PositionNr:      1,
		TrackLength:     1,
		Positions:       []AHXPosition{{Track: [4]int{0, 0, 0, 0}}},
		Tracks:          [][]AHXStep{{{}}},
		Instruments:     []AHXInstrument{{}},
	}
	if err := engine.LoadSong(song, 0); err != nil {
		t.Fatalf("LoadSong failed: %v", err)
	}
	return engine, chip
}

func TestAHX_FilterFixed_MapsToIECutoff(t *testing.T) {
	engine, chip := newLoadedMappingEngine(t)
	v := &engine.replayer.Voices[0]
	v.Init()
	v.FilterPos = 16
	v.VoiceVolume = 64
	engine.updateChannels()

	ch := chip.channels[0]
	if ch.filterModeMask != 1 {
		t.Fatalf("filterModeMask = %d, want lowpass enabled", ch.filterModeMask)
	}
	if ch.filterCutoffTarget <= 0 || ch.filterCutoffTarget >= 1 {
		t.Fatalf("filterCutoffTarget = %.3f, want normalized lowpass cutoff", ch.filterCutoffTarget)
	}
}

func TestAHX_FilterModulation_MapsToIECutoffSweep(t *testing.T) {
	engine, chip := newLoadedMappingEngine(t)
	v := &engine.replayer.Voices[0]
	v.Init()
	v.FilterPos = 8
	engine.updateChannels()
	first := chip.channels[0].filterCutoffTarget
	v.FilterPos = 24
	engine.updateChannels()
	second := chip.channels[0].filterCutoffTarget
	if first == second {
		t.Fatalf("filter cutoff did not change across FilterPos sweep: %.3f", first)
	}
}

func TestAHX_SquareModulation_MapsToPWMDuty(t *testing.T) {
	engine, chip := newLoadedMappingEngine(t)
	engine.SetAHXPlusEnabled(true)
	v := &engine.replayer.Voices[0]
	v.Init()
	v.Waveform = 2
	v.SquarePos = 16
	engine.updateChannels()
	if got := chip.channels[0].dutyCycle; got != 0.25 {
		t.Fatalf("dutyCycle = %.3f, want 0.25", got)
	}
}

func TestAHX_VibratoAndPortamento_MapToFreqChange(t *testing.T) {
	engine, chip := newLoadedMappingEngine(t)
	v := &engine.replayer.Voices[0]
	v.Init()
	v.WaveLength = 2
	v.VoicePeriod = AHXPeriodTable[24]
	engine.updateChannels()
	first := chip.channels[0].frequency
	v.VoicePeriod = AHXPeriodTable[24] + 32
	engine.updateChannels()
	second := chip.channels[0].frequency
	if first == second {
		t.Fatalf("frequency did not change when voice period changed: %.3f", first)
	}
}

func TestAHX_VolumeSlide_MapsToIEVolume(t *testing.T) {
	engine, chip := newLoadedMappingEngine(t)
	v := &engine.replayer.Voices[0]
	v.Init()
	v.VoiceVolume = 16
	engine.updateChannels()
	first := chip.channels[0].volume
	v.VoiceVolume = 48
	engine.updateChannels()
	second := chip.channels[0].volume
	if second <= first {
		t.Fatalf("volume did not increase: first %.3f second %.3f", first, second)
	}
}

func TestAHX_NoiseChannel_MapsToWaveNoise(t *testing.T) {
	engine, chip := newLoadedMappingEngine(t)
	v := &engine.replayer.Voices[0]
	v.Init()
	v.Waveform = 3
	engine.updateChannels()
	if got := chip.channels[0].waveType; got != WAVE_NOISE {
		t.Fatalf("waveType = %d, want WAVE_NOISE", got)
	}
}

func TestAHX_Subsong_Switch(t *testing.T) {
	song := &AHXFile{
		Revision:        0,
		SpeedMultiplier: 1,
		PositionNr:      3,
		TrackLength:     1,
		SubsongNr:       2,
		Subsongs:        []int{1, 2},
		Positions:       []AHXPosition{{}, {}, {}},
		Tracks:          [][]AHXStep{{{}}},
		Instruments:     []AHXInstrument{{}},
	}
	r := NewAHXReplayer()
	r.InitSong(song)
	if err := r.InitSubsong(2); err != nil {
		t.Fatalf("InitSubsong failed: %v", err)
	}
	if r.PosNr != 2 {
		t.Fatalf("PosNr = %d, want subsong 2 start position 2", r.PosNr)
	}
}
