//go:build !headless && !js

// input_gamepad_ebiten.go - Ebiten host polling for the gamepad MMIO block.
//
// Ebiten resolves vendor differences via its built-in standard-layout mapping
// database. We read the standard button/axis layout and publish a canonical
// snapshot into GamepadMMIO. Non-standard controllers (no standard layout) are
// reported as disconnected rather than guessed.

package main

import "github.com/hajimehoshi/ebiten/v2"

// canonButtonMap maps each canonical JOY_BIT index to its Ebiten standard
// gamepad button.
var canonButtonMap = [GAMEPAD_BUTTON_COUNT]ebiten.StandardGamepadButton{
	JOY_BIT_UP:     ebiten.StandardGamepadButtonLeftTop,
	JOY_BIT_DOWN:   ebiten.StandardGamepadButtonLeftBottom,
	JOY_BIT_LEFT:   ebiten.StandardGamepadButtonLeftLeft,
	JOY_BIT_RIGHT:  ebiten.StandardGamepadButtonLeftRight,
	JOY_BIT_A:      ebiten.StandardGamepadButtonRightBottom,
	JOY_BIT_B:      ebiten.StandardGamepadButtonRightRight,
	JOY_BIT_X:      ebiten.StandardGamepadButtonRightLeft,
	JOY_BIT_Y:      ebiten.StandardGamepadButtonRightTop,
	JOY_BIT_LB:     ebiten.StandardGamepadButtonFrontTopLeft,
	JOY_BIT_RB:     ebiten.StandardGamepadButtonFrontTopRight,
	JOY_BIT_LT:     ebiten.StandardGamepadButtonFrontBottomLeft,
	JOY_BIT_RT:     ebiten.StandardGamepadButtonFrontBottomRight,
	JOY_BIT_SELECT: ebiten.StandardGamepadButtonCenterLeft,
	JOY_BIT_START:  ebiten.StandardGamepadButtonCenterRight,
	JOY_BIT_L3:     ebiten.StandardGamepadButtonLeftStick,
	JOY_BIT_R3:     ebiten.StandardGamepadButtonRightStick,
	JOY_BIT_HOME:   ebiten.StandardGamepadButtonCenterCenter,
}

// Poll reads the current Ebiten gamepad state and publishes a canonical
// snapshot. It is called once per frame from the Ebiten Update loop.
func (m *GamepadMMIO) Poll() {
	var snap GamepadSnapshot
	ids := ebiten.AppendGamepadIDs(nil)
	slot := 0
	for _, id := range ids {
		if slot >= GAMEPAD_MAX_PADS {
			break
		}
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue
		}
		p := &snap.Pads[slot]
		p.Connected = true
		for bit := 0; bit < GAMEPAD_BUTTON_COUNT; bit++ {
			p.Buttons[bit] = ebiten.IsStandardGamepadButtonPressed(id, canonButtonMap[bit])
		}
		p.LX = ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
		p.LY = ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
		p.RX = ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisRightStickHorizontal)
		p.RY = ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisRightStickVertical)
		slot++
	}
	m.applySnapshot(snap)
}
