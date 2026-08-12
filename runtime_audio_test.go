package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestRuntimeAudioCleanupIsIdempotent(t *testing.T) {
	defer runRuntimeAudioCleanup()
	var calls int
	registerRuntimeAudioCleanup(func() { calls++ })
	runRuntimeAudioCleanup()
	runRuntimeAudioCleanup()
	if calls != 1 {
		t.Fatalf("runtime audio cleanup calls = %d, want 1", calls)
	}
}

func TestNewRuntimeSoundChipSelectsConfiguredBackendOrder(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        []int
	}{
		{"default", "", []int{AUDIO_BACKEND_OTO}},
		{"oto fallback", "oto", []int{AUDIO_BACKEND_OTO}},
		{"jack", "jack", []int{AUDIO_BACKEND_JACK}},
		{"null", "null", []int{AUDIO_BACKEND_NULL}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IE_AUDIO_BACKEND", tc.value)
			var got []int
			_, err := newRuntimeSoundChip(func(backend int) (*SoundChip, error) { got = append(got, backend); return &SoundChip{}, nil })
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("attempts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewRuntimeSoundChipRejectsUnknownConfiguredBackend(t *testing.T) {
	t.Setenv("IE_AUDIO_BACKEND", "alsa")
	called := false
	_, err := newRuntimeSoundChip(func(int) (*SoundChip, error) { called = true; return nil, nil })
	if err == nil || called {
		t.Fatalf("unknown backend err=%v called=%v", err, called)
	}
}

func TestRuntimeSoundChipFallsBackToSilentAudio(t *testing.T) {
	t.Setenv("IE_AUDIO_BACKEND", "")
	var calls []int

	chip, err := newRuntimeSoundChip(func(backend int) (*SoundChip, error) {
		calls = append(calls, backend)
		if backend == AUDIO_BACKEND_OTO {
			return nil, errors.New("oto unavailable")
		}
		if backend == AUDIO_BACKEND_NULL {
			return &SoundChip{}, nil
		}
		t.Fatalf("unexpected backend %d", backend)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("newRuntimeSoundChip returned error: %v", err)
	}
	if chip == nil {
		t.Fatal("newRuntimeSoundChip returned nil chip")
	}

	want := []int{AUDIO_BACKEND_OTO, AUDIO_BACKEND_NULL}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("backend calls=%v, want %v", calls, want)
	}
}

func TestRuntimeSoundChipReturnsPrimaryAudio(t *testing.T) {
	t.Setenv("IE_AUDIO_BACKEND", "")
	wantChip := &SoundChip{}
	var calls []int

	chip, err := newRuntimeSoundChip(func(backend int) (*SoundChip, error) {
		calls = append(calls, backend)
		if backend != AUDIO_BACKEND_OTO {
			t.Fatalf("unexpected fallback backend %d", backend)
		}
		return wantChip, nil
	})
	if err != nil {
		t.Fatalf("newRuntimeSoundChip returned error: %v", err)
	}
	if chip != wantChip {
		t.Fatal("newRuntimeSoundChip did not return the primary audio chip")
	}

	want := []int{AUDIO_BACKEND_OTO}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("backend calls=%v, want %v", calls, want)
	}
}

func TestNullAudioOutputLifecycle(t *testing.T) {
	out, err := NewAudioOutput(AUDIO_BACKEND_NULL, SAMPLE_RATE, nil)
	if err != nil {
		t.Fatalf("NewAudioOutput(AUDIO_BACKEND_NULL) returned error: %v", err)
	}

	out.Start()
	if !out.IsStarted() {
		t.Fatal("null audio output did not report started")
	}

	out.Stop()
	if out.IsStarted() {
		t.Fatal("null audio output remained started after Stop")
	}

	out.Start()
	out.Close()
	if out.IsStarted() {
		t.Fatal("null audio output remained started after Close")
	}
}

func TestJACKAudioOutputFailsClearlyWhenNotCompiled(t *testing.T) {
	_, err := NewAudioOutput(AUDIO_BACKEND_JACK, SAMPLE_RATE, nil)
	if err == nil {
		t.Fatal("JACK constructor succeeded without the jack build tag")
	}
}
