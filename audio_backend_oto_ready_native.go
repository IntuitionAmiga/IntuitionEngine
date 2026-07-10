//go:build !headless && !js

// audio_backend_oto_ready_native.go - blocking wait for the audio device.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// otoAwaitReady blocks until the audio device is ready. Native backends
// signal readiness during context creation, so this returns near-instantly
// and keeps the historical guarantee that a constructed OtoPlayer is
// immediately usable.
func otoAwaitReady(ready chan struct{}) {
	<-ready
}
