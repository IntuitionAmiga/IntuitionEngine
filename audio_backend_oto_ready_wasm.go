//go:build !headless && js

// audio_backend_oto_ready_wasm.go - non-blocking audio readiness on js/wasm.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// otoAwaitReady must not block on js/wasm: the browser keeps the AudioContext
// suspended until a user gesture, so the ready channel stays open on any page
// load that lacks one (a reload, a typed URL) and waiting would stall machine
// boot forever. Oto resumes the context on the first keypress or click and
// buffers writes until then, so returning early only means silence, never
// lost state.
func otoAwaitReady(ready chan struct{}) {
	go func() { <-ready }()
}
