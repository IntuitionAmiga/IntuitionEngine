//go:build !js && !headless

// wasm_input_bridge_stub.go - off the browser build the text-input bridge is a
// no-op: native keyboard input goes straight through Ebiten from the OS.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

func registerWasmInput(_ *EbitenOutput) {}
