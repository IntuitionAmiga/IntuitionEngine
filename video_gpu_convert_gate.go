// video_gpu_convert_gate.go - render and readback gate entry point for GPU conversion.

//go:build !headless

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
Anything that renders through a real backend and reads the pixels back needs
Ebiten to own the main OS thread. A test function never runs there: creating a
texture from one segfaults inside the OpenGL driver, and reading pixels back
outside a running game panics with "ReadPixels cannot be called before the game
starts". That applies to the GPU conversion differential and equally to the
hardware compositor tests, which is why those stopped working when Ebitengine
moved to v2.10.

So such a check runs as its own process. It registers a named body, the test
re-executes the test binary with IE_GPU_GATE_BODY set to that name, TestMain
calls runGPUGateIfRequested before it sets any test state up, and the body owns
the main thread for the life of that process. The result comes back as an exit
status plus a message on stderr.

This file is compiled into the normal build as well, where the environment
variable is never set and the function returns immediately.
*/

package main

import (
	"fmt"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// gpuGateEnv names the gate body to run in this process. A body registers
// itself in gpuGateBodies, and the parent re-executes the binary with this
// variable set to the body's name.
const gpuGateEnv = "IE_GPU_GATE_BODY"

// gpuGateBodies holds the registered gate bodies. Tests populate it from their
// package initialisers, which run before TestMain.
var gpuGateBodies = map[string]func() error{}

// runGPUGateIfRequested runs the GPU conversion gate and exits, when asked. It
// returns immediately otherwise, so it is safe to call unconditionally.
func runGPUGateIfRequested() {
	name := os.Getenv(gpuGateEnv)
	if name == "" {
		return
	}
	body, ok := gpuGateBodies[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "gpu gate: no body registered under %q\n", name)
		os.Exit(4)
	}
	game := &gpuGateGame{body: body}
	ebiten.SetWindowSize(64, 64)
	ebiten.SetWindowTitle("IE GPU conversion gate")
	if err := ebiten.RunGame(game); err != nil && err != ebiten.Termination {
		fmt.Fprintf(os.Stderr, "gpu gate: could not start a graphics backend: %v\n", err)
		os.Exit(3)
	}
	if game.err != nil {
		fmt.Fprintf(os.Stderr, "gpu gate: %v\n", game.err)
		os.Exit(2)
	}
	os.Exit(0)
}

// gpuGateGame runs one body inside a real game loop and then terminates.
type gpuGateGame struct {
	body func() error
	err  error
	done bool
}

func (g *gpuGateGame) Update() error {
	if !g.done {
		g.err = g.body()
		g.done = true
	}
	return ebiten.Termination
}

func (g *gpuGateGame) Draw(*ebiten.Image)         {}
func (g *gpuGateGame) Layout(_, _ int) (int, int) { return 64, 64 }
