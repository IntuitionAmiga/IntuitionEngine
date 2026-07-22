// audio_backend_oto_buffer_test.go - pins the oto device buffer duration.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"testing"
	"time"
)

// TestOtoContextOptions_BufferSizeExplicit pins the measured device buffer
// duration. The previous value was the untyped constant 4, which is 4
// nanoseconds, truncates to zero bytes inside oto and silently selects the
// driver fallback buffer. A sub-millisecond value here is always a unit
// mistake, and values below 10 ms panicked the PulseAudio client during the
// measurement sweep.
func TestOtoContextOptions_BufferSizeExplicit(t *testing.T) {
	got := otoBufferDuration()
	if want := 10 * time.Millisecond; got != want {
		t.Fatalf("oto buffer duration = %v, want %v", got, want)
	}
	if got < time.Millisecond {
		t.Fatalf("oto buffer duration %v is sub-millisecond, which oto truncates to zero bytes", got)
	}
	// The duration must convert to a whole number of frames at the engine
	// sample rate, otherwise oto rounds the request down.
	frames := int64(got) * int64(SAMPLE_RATE) / int64(time.Second)
	if frames <= 0 {
		t.Fatalf("oto buffer duration %v yields %d frames at %d Hz", got, frames, SAMPLE_RATE)
	}
}
