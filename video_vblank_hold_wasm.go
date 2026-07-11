//go:build js && wasm

// video_vblank_hold_wasm.go - VBlank visibility window for the
// single-threaded js/wasm build. See the VIDEO_STATUS read in
// video_chip.go: the compositor sets and clears VBlank within one tick
// while the CPU goroutine is parked, so a polling guest needs the set
// state held readable for a fraction of the frame period.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// videoVBlankHoldNs keeps the VBlank status bit readable for 3 ms after
// each set edge, roughly the blanking fraction of a 60 Hz frame.
const videoVBlankHoldNs = 3_000_000
