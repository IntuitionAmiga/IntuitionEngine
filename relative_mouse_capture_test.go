package main

import "testing"

func TestRelativeMouseCaptureActivatesAndClearsOnlyWithoutOverride(t *testing.T) {
	var s relativeMouseCaptureState

	out := s.Update(relativeMouseCaptureInput{
		guestRelative: true,
		hostX:         100,
		hostY:         200,
	})

	if !s.active || !s.captured || s.hostReleased {
		t.Fatalf("state after activate = active:%v captured:%v released:%v", s.active, s.captured, s.hostReleased)
	}
	if !out.clearDeltas {
		t.Fatal("expected activation to clear stale deltas without override")
	}
	if out.cursorAction != relativeMouseCursorCapture {
		t.Fatalf("cursor action = %d, want capture", out.cursorAction)
	}

	s = relativeMouseCaptureState{}
	out = s.Update(relativeMouseCaptureInput{
		guestRelative: true,
		mouseOverride: true,
		hostX:         100,
		hostY:         200,
	})

	if out.clearDeltas {
		t.Fatal("activation with script override must preserve scripted deltas")
	}
	if out.cursorAction != relativeMouseCursorCapture {
		t.Fatalf("cursor action = %d, want capture", out.cursorAction)
	}
}

func TestRelativeMouseCaptureDeltasOnlyWhenCapturedAndNotOverride(t *testing.T) {
	var s relativeMouseCaptureState
	_ = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 10, hostY: 20})

	out := s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 15, hostY: 17})
	if out.addDX != 5 || out.addDY != -3 {
		t.Fatalf("delta = (%d,%d), want (5,-3)", out.addDX, out.addDY)
	}

	out = s.Update(relativeMouseCaptureInput{guestRelative: true, mouseOverride: true, hostX: 40, hostY: 60})
	if out.addDX != 0 || out.addDY != 0 {
		t.Fatalf("override delta = (%d,%d), want (0,0)", out.addDX, out.addDY)
	}

	out = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 42, hostY: 62})
	if out.addDX != 2 || out.addDY != 2 {
		t.Fatalf("post-override delta = (%d,%d), want (2,2)", out.addDX, out.addDY)
	}
}

func TestRelativeMouseCaptureHostReleaseAndClickRecapture(t *testing.T) {
	var s relativeMouseCaptureState
	_ = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 10, hostY: 20})

	out := s.Update(relativeMouseCaptureInput{
		guestRelative:    true,
		hostX:            12,
		hostY:            24,
		releaseRequested: true,
	})
	if s.captured || !s.hostReleased {
		t.Fatalf("state after release = captured:%v released:%v", s.captured, s.hostReleased)
	}
	if out.clearDeltas {
		t.Fatal("host release must not clear guest/script deltas")
	}
	if out.cursorAction != relativeMouseCursorVisible {
		t.Fatalf("cursor action = %d, want restore", out.cursorAction)
	}

	out = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 100, hostY: 200})
	if out.addDX != 0 || out.addDY != 0 {
		t.Fatalf("released delta = (%d,%d), want (0,0)", out.addDX, out.addDY)
	}

	out = s.Update(relativeMouseCaptureInput{
		guestRelative:      true,
		hostX:              130,
		hostY:              250,
		recaptureRequested: true,
	})
	if !s.captured || s.hostReleased {
		t.Fatalf("state after recapture = captured:%v released:%v", s.captured, s.hostReleased)
	}
	if out.addDX != 0 || out.addDY != 0 {
		t.Fatalf("recapture delta = (%d,%d), want (0,0)", out.addDX, out.addDY)
	}
	if !out.suppressButtons {
		t.Fatal("recapture click should be consumed for one frame")
	}
	if out.cursorAction != relativeMouseCursorCapture {
		t.Fatalf("cursor action = %d, want capture", out.cursorAction)
	}

	out = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 132, hostY: 247})
	if out.addDX != 2 || out.addDY != -3 {
		t.Fatalf("post-recapture delta = (%d,%d), want (2,-3)", out.addDX, out.addDY)
	}
}

func TestRelativeMouseCaptureDisableRestoresAndClears(t *testing.T) {
	var s relativeMouseCaptureState
	_ = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 10, hostY: 20})
	_ = s.Update(relativeMouseCaptureInput{guestRelative: true, hostX: 12, hostY: 24, releaseRequested: true})

	out := s.Update(relativeMouseCaptureInput{guestRelative: false, hostX: 12, hostY: 24})
	if s.active || s.captured || s.hostReleased {
		t.Fatalf("state after disable = active:%v captured:%v released:%v", s.active, s.captured, s.hostReleased)
	}
	if !out.clearDeltas {
		t.Fatal("guest disable should clear stale deltas")
	}
	if out.cursorAction != relativeMouseCursorRestorePolicy {
		t.Fatalf("cursor action = %d, want restore", out.cursorAction)
	}
}

func TestRelativeMouseCaptureReleaseOnActivation(t *testing.T) {
	var s relativeMouseCaptureState

	out := s.Update(relativeMouseCaptureInput{
		guestRelative:    true,
		hostX:            10,
		hostY:            20,
		releaseRequested: true,
	})
	if s.captured || !s.hostReleased {
		t.Fatalf("state after release-on-activate = captured:%v released:%v", s.captured, s.hostReleased)
	}
	if out.cursorAction != relativeMouseCursorVisible {
		t.Fatalf("cursor action = %d, want restore", out.cursorAction)
	}
}
