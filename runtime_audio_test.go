package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestRuntimeSoundChipFallsBackToSilentAudio(t *testing.T) {
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
