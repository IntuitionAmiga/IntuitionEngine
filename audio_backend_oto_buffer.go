// audio_backend_oto_buffer.go - output device buffer sizing for the oto backend.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "time"

// otoDefaultBufferDuration is the size of the underlying audio device buffer,
// expressed as a duration of audio. oto converts it to bytes with
// bufferSizeInBytes = duration * sampleRate * channels * 4, so any value small
// enough to truncate to zero bytes is treated by the drivers as "no buffer
// size requested": PulseAudio then asks for 100 ms of latency and ALSA falls
// back to a 1024 frame period. The historical value was the untyped constant
// 4, which is 4 nanoseconds and therefore always took that fallback path.
//
// The value was selected by sweeping the device read cadence over a WAV
// playback workload at 0 (driver default), 4 ms, 8 ms, 10 ms and 20 ms and
// counting reads that started after the previously supplied audio had already
// been consumed. 10 ms, 8 ms and 20 ms produced no starved reads over three
// runs each; the driver default produced starved reads; 4 ms panicked
// deterministically inside the PulseAudio client because the requested
// latency falls below the negotiated fragment size. 10 ms is the smallest
// duration kept, leaving margin above the value that fails.
//
// Do not reduce this below 10 ms without repeating the sweep on both the
// PulseAudio and ALSA backends.
const otoDefaultBufferDuration = 10 * time.Millisecond

// otoBufferDuration returns the device buffer duration requested at context
// creation. It exists so the value can be asserted from headless builds,
// where the oto backend itself is not compiled in.
func otoBufferDuration() time.Duration {
	return otoDefaultBufferDuration
}
