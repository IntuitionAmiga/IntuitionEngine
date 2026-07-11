//go:build !js

// video_vblank_hold_other.go - native builds read VBlank exactly as the
// compositor publishes it; guest threads run in parallel with the tick and
// observe the real window, so no hold is needed.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

const videoVBlankHoldNs = 0
