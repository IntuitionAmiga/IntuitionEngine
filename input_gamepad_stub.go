//go:build headless || js

// input_gamepad_stub.go - Non-Ebiten gamepad poll.
//
// Headless and js/wasm builds have no Ebiten gamepad source. Poll reports no
// pads. wasm can later feed the browser Gamepad API into the same GamepadMMIO
// struct via applySnapshot; the seam is deliberately clean.

package main

// Poll publishes an empty snapshot: no pads connected.
func (m *GamepadMMIO) Poll() {
	m.applySnapshot(GamepadSnapshot{})
}
