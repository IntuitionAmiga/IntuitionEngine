//go:build js && !headless

// video_first_frame_wasm.go - first rendered frame signal for the browser.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "syscall/js"

// hostSignalFirstFrame tells the hosting page that the first frame has been
// rendered. The demo page keeps its loading overlay up until this fires: the
// Ebiten canvas element is created during package initialisation, long before
// RunGame draws anything, so canvas existence alone is not "screen ready".
// Sets a global flag (for pages that start polling late) and dispatches an
// event (for pages that listen).
func hostSignalFirstFrame() {
	global := js.Global()
	global.Set("ieFirstFrame", js.ValueOf(true))
	document := global.Get("document")
	if document.Truthy() {
		event := global.Get("Event").New("ie-first-frame")
		document.Call("dispatchEvent", event)
	}
}

// hostSetCRTPresentationState publishes the post-initialisation CRT state for
// browser automation. It is set from Draw after shader initialisation, so a
// requested mode that fell back to an unfiltered frame is observable.
func hostSetCRTPresentationState(state string) {
	js.Global().Set("ieCRTPresentationState", js.ValueOf(state))
}
