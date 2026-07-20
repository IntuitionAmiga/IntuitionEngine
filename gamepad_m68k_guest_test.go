//go:build headless

// gamepad_m68k_guest_test.go - Native M68K guest access to the gamepad block.
//
// USB Gamepad Input plan, steps 5a/5b. In this project EmuTOS and AROS are
// IE-native M68K builds: they consume IE MMIO directly (EmuTOS reads scancodes
// from SCAN_CODE $F0740, AROS reads the terminal MMIO in Amiga rawkey mode)
// rather than emulated Atari IKBD or Amiga CIA hardware, none of which this
// build provides. M68K is a flat-addressing architecture, so the gamepad block
// is reachable directly at its canonical bus address with no per-guest Go
// adapter, exactly as ie68.inc declares (GAMEPAD_REGION_BASE equ $F25C0).
//
// This is the plan's 5a/5b acceptance in the form that matches the architecture:
// read through the M68K CPU at the address the guest actually uses and assert a
// seeded snapshot yields the expected register bytes. The Atari-joystick and
// lowlevel.library/ReadJoyPort upstream-API adapters remain deferred: they are
// guest-side driver work in the external EmuTOS and AROS-deadw00d source trees,
// not a Go shim, and this build emulates no native joystick/CIA hardware to hang
// one on. See the plan's decisions-folded-in notes.
package main

import "testing"

func TestGamepad_M68KGuestReadsCanonicalBlock(t *testing.T) {
	bus := NewMachineBus()
	gp := RegisterGamepadMMIO(bus)
	cpu := NewM68KCPU(bus)

	// Pad 0 connected: A and Home pressed, left stick full right (+1) and full
	// up in Ebiten-native terms (-1 on Y). Pad 1 also connected but idle, to
	// prove the connected mask and count.
	var snap GamepadSnapshot
	snap.Pads[0].Connected = true
	snap.Pads[0].Buttons[JOY_BIT_A] = true
	snap.Pads[0].Buttons[JOY_BIT_HOME] = true
	snap.Pads[0].LX = 1.0
	snap.Pads[0].LY = -1.0
	snap.Pads[1].Connected = true
	gp.applySnapshot(snap)

	// STATUS: pads 0 and 1 connected (mask 0b11), count 2.
	wantStatus := uint32(0b11) | (2 << 8)
	if got := cpu.Read32(GAMEPAD_STATUS); got != wantStatus {
		t.Errorf("STATUS via M68K Read32 = %#x, want %#x", got, wantStatus)
	}

	// Pad 0 BUTTONS word: A (bit 4) and Home (bit 16).
	wantButtons := uint32(1<<JOY_BIT_A | 1<<JOY_BIT_HOME)
	if got := cpu.Read32(GAMEPAD_PAD0_BASE + GAMEPAD_BUTTONS_OFF); got != wantButtons {
		t.Errorf("PAD0 BUTTONS via M68K Read32 = %#x, want %#x", got, wantButtons)
	}

	// Pad 0 left axis word: X = +32767 (0x7FFF) low half, Y = -32767 (0x8001)
	// high half. Matches packAxisPair, independent of CPU.
	wantAxis := packAxisPair(1.0, -1.0)
	if got := cpu.Read32(GAMEPAD_PAD0_BASE + GAMEPAD_AXIS_LXY_OFF); got != wantAxis {
		t.Errorf("PAD0 AXIS_LXY via M68K Read32 = %#x, want %#x", got, wantAxis)
	}

	// Byte-lane: Home is bit 16, i.e. byte 2 of the BUTTONS word. A small guest
	// reading a single byte lane must see it, proving the byte-lane ABI holds
	// through the M68K byte read path too.
	if got := cpu.Read8(GAMEPAD_PAD0_BASE + GAMEPAD_BUTTONS_OFF + 2); got&0x01 == 0 {
		t.Errorf("PAD0 BUTTONS byte lane 2 via M68K Read8 = %#x, want bit0 set (Home)", got)
	}

	// Disconnected pad 2 reads as zero (no stale state).
	if got := cpu.Read32(GAMEPAD_PAD0_BASE + 2*GAMEPAD_PAD_STRIDE + GAMEPAD_BUTTONS_OFF); got != 0 {
		t.Errorf("disconnected PAD2 BUTTONS via M68K Read32 = %#x, want 0", got)
	}
}
