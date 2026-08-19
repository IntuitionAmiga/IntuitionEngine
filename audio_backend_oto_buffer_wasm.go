//go:build js

// audio_backend_oto_buffer_wasm.go - output device buffer sizing for the js/wasm oto backend.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"os"
	"strconv"
	"time"
)

// On js/wasm, single-threaded cooperative yielding parks the CPU goroutine for
// up to one display frame (16 ms by default). A 10 ms device buffer underruns
// on every yield cycle because the JS event loop cannot fire Oto's AudioWorklet
// refill callback while the CPU goroutine is executing. A 100 ms device buffer
// provides ample headroom (50 ms refill threshold vs 16 ms yield slice) to
// prevent WebAudio device underrun.
const otoWasmDefaultBufferDuration = 100 * time.Millisecond

func otoBufferDuration() time.Duration {
	if v := os.Getenv("IE_WASM_AUDIO_BUFFER_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 50 && n <= 1000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return otoWasmDefaultBufferDuration
}

// otoPlayerBufferSize returns the buffer size passed to op.player.SetBufferSize.
// The player buffer size must match the AudioWorklet device buffer (otoBufferDuration).
// If the player buffer is smaller than the Worklet's half-buffer threshold (50ms),
// Oto's AudioWorklet processor remains stuck in the waitRecv state, inserting 128-sample
// silence blocks whenever the buffer drops to 0. This time-stretches the audio, causing
// the playback tempo to sound noticeably slow. Matching the 100ms device buffer ensures
// the Worklet receives a full 100ms pre-fill and plays at 1.0x real-time speed.
func otoPlayerBufferSize(sampleRate int) int {
	bytesPerSample := 4
	d := otoBufferDuration()
	sz := int(int64(d) * int64(sampleRate) * int64(bytesPerSample) / int64(time.Second))
	return (sz / bytesPerSample) * bytesPerSample
}
