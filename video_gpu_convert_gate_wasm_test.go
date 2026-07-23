// video_gpu_convert_gate_wasm_test.go - in-process gate runner for the browser.

//go:build js && wasm

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
The native runner re-executes the test binary so Ebiten can own the main OS
thread. In the browser there is no process to re-execute and no OS thread to
own: Ebiten drives itself from requestAnimationFrame on the single JS thread,
so the body runs in process and RunGame returns when the body terminates it.

This is what lets the same conversion gate run against WebGL, which is the only
way to know the shader works on the platform the demo actually ships to.
*/

package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func runGPUGate(t *testing.T, name string) {
	t.Helper()
	runGPUGateOutput(t, name)
}

// runGPUGateOutput runs the named body inside the browser's own game loop. It
// returns no captured output: the body writes to stdout, which the page shows.
func runGPUGateOutput(t *testing.T, name string) string {
	t.Helper()
	body, ok := gpuGateBodies[name]
	if !ok {
		t.Fatalf("no gate body registered under %q", name)
	}
	game := &gpuGateGame{body: body}
	if err := ebiten.RunGame(game); err != nil && err != ebiten.Termination {
		t.Skipf("could not start a graphics backend: %v", err)
	}
	if game.err != nil {
		t.Fatalf("gate %q failed: %v", name, game.err)
	}
	return ""
}
