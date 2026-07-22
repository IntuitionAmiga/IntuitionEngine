//go:build !headless

// audio_backend_oto_options_test.go - oto context option construction.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"testing"

	"github.com/ebitengine/oto/v3"
)

// TestOtoContextOptions_CarriesBufferDuration checks that the constructed
// options carry the pinned buffer duration and the mono float32 format the
// player writes.
func TestOtoContextOptions_CarriesBufferDuration(t *testing.T) {
	opts := otoContextOptions(SAMPLE_RATE)
	if opts.BufferSize != otoBufferDuration() {
		t.Fatalf("BufferSize = %v, want %v", opts.BufferSize, otoBufferDuration())
	}
	if opts.SampleRate != SAMPLE_RATE {
		t.Fatalf("SampleRate = %d, want %d", opts.SampleRate, SAMPLE_RATE)
	}
	if opts.ChannelCount != 1 {
		t.Fatalf("ChannelCount = %d, want 1", opts.ChannelCount)
	}
	if opts.Format != oto.FormatFloat32LE {
		t.Fatalf("Format = %v, want FormatFloat32LE", opts.Format)
	}
}
