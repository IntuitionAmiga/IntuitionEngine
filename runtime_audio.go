package main

import (
	"fmt"
	"os"
)

type soundChipFactory func(int) (*SoundChip, error)

func newRuntimeSoundChip(factory soundChipFactory) (*SoundChip, error) {
	backend := os.Getenv("IE_AUDIO_BACKEND")
	var attempts []int
	switch backend {
	case "", "oto":
		attempts = []int{AUDIO_BACKEND_OTO, AUDIO_BACKEND_NULL}
	case "jack":
		attempts = []int{AUDIO_BACKEND_JACK, AUDIO_BACKEND_OTO, AUDIO_BACKEND_NULL}
	case "null":
		attempts = []int{AUDIO_BACKEND_NULL}
	default:
		return nil, fmt.Errorf("invalid IE_AUDIO_BACKEND %q: expected oto, jack or null", backend)
	}

	var firstErr error
	for _, candidate := range attempts {
		chip, err := factory(candidate)
		if err == nil {
			return chip, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if candidate != AUDIO_BACKEND_NULL {
			fmt.Printf("Warning: failed to initialize audio backend %d: %v\n", candidate, err)
		}
	}
	return nil, fmt.Errorf("failed to initialize audio: %w; silent fallback failed", firstErr)
}
