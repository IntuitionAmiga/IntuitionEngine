//go:build headless

// input_gamepad_stub.go - Non-Ebiten gamepad poll.
//
// Only the headless build has no windowing backend and therefore no gamepad
// source, so Poll reports no pads. The native desktop and js/wasm browser
// builds both use the Ebiten poll (input_gamepad_ebiten.go); on js Ebiten is
// backed by the browser Gamepad API.

package main

// Poll publishes an empty snapshot: no pads connected.
func (m *GamepadMMIO) Poll() {
	m.applySnapshot(GamepadSnapshot{})
}
