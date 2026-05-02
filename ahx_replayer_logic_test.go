// ahx_replayer_logic_test.go - AHX replayer correctness tests.

package main

import "testing"

func TestAHX_F00_Pauses(t *testing.T) {
	r := newTestAHXReplayer(t)
	r.Song.Tracks[0][0] = AHXStep{FX: 0xF, FXParam: 0}
	r.PlayIRQ()
	if r.Tempo != 0 {
		t.Fatalf("Tempo = %d, want paused tempo 0", r.Tempo)
	}
	if r.SongEndReached {
		t.Fatal("F00 should not mark song end")
	}
}

func TestAHX_PListFX0_RevisionGate(t *testing.T) {
	for _, tc := range []struct {
		revision int
		want     int
	}{
		{revision: 0, want: 16},
		{revision: 1, want: 32},
	} {
		r := NewAHXReplayer()
		r.Song = &AHXFile{Revision: tc.revision}
		r.Voices[0].Init()
		r.Voices[0].FilterPos = 32
		r.PListCommandParse(0, 0, 16)
		if got := r.Voices[0].FilterPos; got != tc.want {
			t.Fatalf("revision %d FilterPos = %d, want %d", tc.revision, got, tc.want)
		}
	}
}

func TestAHX_TonePorta_LimitComparison(t *testing.T) {
	r := newTestAHXReplayer(t)
	v := &r.Voices[0]
	v.TrackPeriod = 24
	r.Song.Tracks[0][0] = AHXStep{Note: 24, FX: 3, FXParam: 4}
	r.ProcessStep(0)
	if !v.PeriodSlideWithLimit {
		t.Fatal("tone portamento should enable limit mode")
	}
	if v.PeriodSlideLimit != 0 {
		t.Fatalf("PeriodSlideLimit = %d, want 0 when target already matches current note", v.PeriodSlideLimit)
	}
}

func TestAHX_TonePorta_DifferentTargetSlides(t *testing.T) {
	r := newTestAHXReplayer(t)
	v := &r.Voices[0]
	v.TrackPeriod = 24
	r.Song.Tracks[0][0] = AHXStep{Note: 30, FX: 3, FXParam: 4}
	r.ProcessStep(0)
	if v.PeriodSlideLimit == 0 {
		t.Fatal("PeriodSlideLimit should be set when tone portamento targets a different note")
	}
	before := v.PeriodSlidePeriod
	r.ProcessFrame(0)
	if v.PeriodSlidePeriod == before {
		t.Fatalf("PeriodSlidePeriod did not advance toward target: before=%d after=%d limit=%d", before, v.PeriodSlidePeriod, v.PeriodSlideLimit)
	}
}

func TestAHX_PatternBreak_ClampGE(t *testing.T) {
	r := newTestAHXReplayer(t)
	r.Song.TrackLength = 4
	r.Song.Tracks[0] = make([]AHXStep, 4)
	r.Song.Tracks[0][0] = AHXStep{FX: 0xD, FXParam: 0x04}
	r.ProcessStep(0)
	if r.PosJumpNote != 0 {
		t.Fatalf("PosJumpNote = %d, want clamp to 0 for row == TrackLength", r.PosJumpNote)
	}
}

func TestAHX_CxxVolumeRange(t *testing.T) {
	r := newTestAHXReplayer(t)
	v := &r.Voices[0]
	v.TrackMasterVolume = 64
	r.Song.Tracks[0][0] = AHXStep{FX: 0xC, FXParam: 0x91}
	r.ProcessStep(0)
	if v.TrackMasterVolume != 64 {
		t.Fatalf("TrackMasterVolume = %d, want unchanged for invalid Cxx range", v.TrackMasterVolume)
	}
}

func TestAHX_TickFractionalAccumulator(t *testing.T) {
	song := &AHXFile{
		Revision:        1,
		SpeedMultiplier: 4,
		PositionNr:      1,
		TrackLength:     64,
		TrackNr:         0,
		Restart:         0,
		Positions:       []AHXPosition{{Track: [4]int{0, 0, 0, 0}}},
		Tracks:          [][]AHXStep{make([]AHXStep, 64)},
		Instruments:     []AHXInstrument{{}},
	}
	engine := NewAHXEngine(newTestSoundChip(), 44100)
	if err := engine.LoadSong(song, 0); err != nil {
		t.Fatalf("LoadSong failed: %v", err)
	}
	engine.SetPlaying(true)
	for range 22050 {
		engine.TickSample()
	}
	if got := engine.replayer.PlayingTime; got != 100 {
		t.Fatalf("PlayingTime = %d ticks, want 100", got)
	}
}
