package main

import "fmt"

type soundChipFactory func(int) (*SoundChip, error)

func newRuntimeSoundChip(factory soundChipFactory) (*SoundChip, error) {
	chip, err := factory(AUDIO_BACKEND_OTO)
	if err == nil {
		return chip, nil
	}

	fmt.Printf("Warning: failed to initialize OTO audio: %v\n", err)
	fmt.Println("Warning: continuing with silent audio output")

	chip, fallbackErr := factory(AUDIO_BACKEND_NULL)
	if fallbackErr != nil {
		return nil, fmt.Errorf("failed to initialize audio: %w; silent fallback failed: %v", err, fallbackErr)
	}
	return chip, nil
}
