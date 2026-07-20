//go:build !headless && !js

// input_gamepad_ebiten_test.go - Gamepad poll injection into Update.
//
// USB Gamepad Input plan, step 2 (RED phase). Verifies the injected
// gamepadPoll runs once per Update frame and its snapshot is visible through a
// bus read.

package main

import "testing"

func TestGamepadPoll_InvokedOncePerFrame(t *testing.T) {
	bus := NewMachineBus()
	dev := RegisterGamepadMMIO(bus)

	eo := &EbitenOutput{}
	eo.running.Store(true)

	calls := 0
	eo.SetGamepadPoll(func() {
		calls++
		var snap GamepadSnapshot
		snap.Pads[0].Connected = true
		snap.Pads[0].Buttons[JOY_BIT_A] = true
		dev.applySnapshot(snap)
	})

	if err := eo.Update(); err != nil {
		t.Fatalf("Update returned %v", err)
	}
	if calls != 1 {
		t.Fatalf("poll called %d times, want 1", calls)
	}

	if got := bus.Read32(GAMEPAD_STATUS) & 0xF; got != 1 {
		t.Fatalf("status connected = %#x, want 1 after frame", got)
	}
	if got := bus.Read32(GAMEPAD_PAD0_BASE + GAMEPAD_BUTTONS_OFF); got != 1<<JOY_BIT_A {
		t.Fatalf("pad0 buttons = %#x, want %#x", got, 1<<JOY_BIT_A)
	}
}

// The guest keeps reading gamepad MMIO while an overlay is open, so the poll
// must run even when an active overlay early-returns from Update.
func TestGamepadPoll_RunsWhileOverlayActive(t *testing.T) {
	bus := NewMachineBus()
	eo := &EbitenOutput{}
	eo.running.Store(true)

	monitor := NewMachineMonitor(bus)
	eo.SetMonitorOverlay(NewMonitorOverlay(monitor))
	monitor.Activate()
	if !monitor.IsActive() {
		t.Fatal("monitor should be active")
	}

	calls := 0
	eo.SetGamepadPoll(func() { calls++ })

	if err := eo.Update(); err != nil {
		t.Fatalf("Update returned %v", err)
	}
	if calls != 1 {
		t.Fatalf("poll called %d times with overlay active, want 1", calls)
	}
}
