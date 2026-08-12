//go:build !linux || !cgo || !jack

package main

import "fmt"

// NewJackAudioOutput preserves all non-JACK build contracts. A JACK request is
// explicit, so it fails clearly and lets runtime selection use its Oto fallback.
func NewJackAudioOutput(sampleRate int, chip *SoundChip) (AudioOutput, error) {
	return nil, fmt.Errorf("JACK audio backend is unavailable in this build; rebuild for linux with cgo and -tags jack")
}
