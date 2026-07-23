// video_gpu_convert_gate_headless.go - GPU gate stub for headless builds.

//go:build headless

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// runGPUGateIfRequested is a no-op on headless builds, which have no
// shader-capable backend to gate.
func runGPUGateIfRequested() {}
