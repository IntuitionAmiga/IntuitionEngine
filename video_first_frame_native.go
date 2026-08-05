//go:build !js && !headless

// video_first_frame_native.go - no-op first-frame signal outside the browser.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// hostSignalFirstFrame is only meaningful on js/wasm, where the hosting page
// holds a loading overlay until the first frame renders. Native windows are
// their own evidence of a live screen.
func hostSignalFirstFrame() {}

func hostSetCRTPresentationState(string) {}
