// ahx_parser_hardening_test.go - AHX malformed input hardening tests.

package main

import "testing"

func TestAHX_TestHelpersBuildParsableModule(t *testing.T) {
	data := buildAHXModule(ahxModuleOptions{})
	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}
	if song.Name != "Test" {
		t.Fatalf("song name = %q, want Test", song.Name)
	}
}

func TestAHX_MalformedWaveLength_NoPanic(t *testing.T) {
	for _, wl := range []int{6, 7} {
		inst := AHXInstrument{Volume: 64, WaveLength: wl}
		data := buildAHXModule(ahxModuleOptions{
			TrackLength:  1,
			TrackNr:      0,
			InstrumentNr: 1,
			Tracks:       [][]AHXStep{{{Note: 24, Instrument: 1}}},
			Instruments:  []AHXInstrument{{}, inst},
		})
		song, err := ParseAHX(data)
		if err != nil {
			t.Fatalf("WaveLength %d should be clamped, got parse error: %v", wl, err)
		}
		if got := song.Instruments[1].WaveLength; got != 5 {
			t.Fatalf("WaveLength %d parsed as %d, want clamp to 5", wl, got)
		}
		r := NewAHXReplayer()
		r.InitSong(song)
		if err := r.InitSubsong(0); err != nil {
			t.Fatalf("InitSubsong failed: %v", err)
		}
		runAHXFrames(r, 2)
	}
}

func TestAHX_MalformedPListWaveform_NoPanic(t *testing.T) {
	for _, waveform := range []int{5, 6, 7} {
		inst := AHXInstrument{
			Volume:     64,
			WaveLength: 2,
			PList: AHXPList{
				Speed:   1,
				Length:  1,
				Entries: []AHXPListEntry{{Waveform: waveform, Note: 24}},
			},
		}
		data := buildAHXModule(ahxModuleOptions{
			TrackLength:  1,
			TrackNr:      0,
			InstrumentNr: 1,
			Tracks:       [][]AHXStep{{{Note: 24, Instrument: 1}}},
			Instruments:  []AHXInstrument{{}, inst},
		})
		song, err := ParseAHX(data)
		if err != nil {
			t.Fatalf("PList waveform %d should be clamped, got parse error: %v", waveform, err)
		}
		if got := song.Instruments[1].PList.Entries[0].Waveform; got != 4 {
			t.Fatalf("PList waveform %d parsed as %d, want clamp to 4", waveform, got)
		}
		r := NewAHXReplayer()
		r.InitSong(song)
		if err := r.InitSubsong(0); err != nil {
			t.Fatalf("InitSubsong failed: %v", err)
		}
		runAHXFrames(r, 3)
	}
}

func TestAHX_SubsongStartPositionOOB_RejectParse(t *testing.T) {
	data := buildAHXModule(ahxModuleOptions{
		PositionNr: 2,
		Subsongs:   []int{2},
	})
	if _, err := ParseAHX(data); err == nil {
		t.Fatal("ParseAHX should reject subsong start position outside position list")
	}
}

func TestAHX_InvalidSubsongIndex_NoPanic(t *testing.T) {
	r := newTestAHXReplayer(t)
	if err := r.InitSubsong(1); err == nil {
		t.Fatal("InitSubsong should reject subsong index above SubsongNr")
	}
	if err := r.InitSubsong(-1); err == nil {
		t.Fatal("InitSubsong should reject negative subsong index")
	}
}

func TestAHX_RestartPositionOOB_Clamped(t *testing.T) {
	data := buildAHXModule(ahxModuleOptions{
		PositionNr: 2,
		Restart:    9,
	})
	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}
	if song.Restart != 1 {
		t.Fatalf("Restart = %d, want clamp to last position 1", song.Restart)
	}
}

func TestAHX_TonePortaNoteOOB_NoPanic(t *testing.T) {
	data := buildAHXModule(ahxModuleOptions{
		TrackLength:  2,
		InstrumentNr: 1,
		Tracks: [][]AHXStep{{
			{Note: 24, Instrument: 1},
			{Note: 63, FX: 3, FXParam: 4},
		}},
		Instruments: []AHXInstrument{{}, {Volume: 64, WaveLength: 2}},
	})
	song, err := ParseAHX(data)
	if err != nil {
		t.Fatalf("ParseAHX failed: %v", err)
	}
	if got := song.Tracks[0][1].Note; got != 60 {
		t.Fatalf("tone-porta note parsed as %d, want clamp to 60", got)
	}
	r := NewAHXReplayer()
	r.InitSong(song)
	if err := r.InitSubsong(0); err != nil {
		t.Fatalf("InitSubsong failed: %v", err)
	}
	runAHXFrames(r, 8)
}

func TestAHX_HardCutReleaseFZero_NoDivZero(t *testing.T) {
	r := newTestAHXReplayer(t)
	v := &r.Voices[0]
	v.Init()
	v.Instrument = &AHXInstrument{
		HardCutRelease:       1,
		HardCutReleaseFrames: 0,
		Envelope:             AHXEnvelope{RVolume: 0},
	}
	v.HardCutRelease = 1
	v.HardCutReleaseF = 0
	v.NoteCutOn = 1
	v.NoteCutWait = 0
	v.ADSRVolume = 64 << 8
	r.ProcessFrame(0)
}
