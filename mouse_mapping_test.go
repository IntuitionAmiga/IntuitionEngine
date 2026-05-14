package main

import "testing"

func TestMapPresentationMouseToGuest_UsesTerminalNativeAspectFitRect(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	tm := NewTerminalMMIO()
	tm.SetMouseNativeResolution(640, 480)

	x, y, w, h := mapPresentationMouseToGuest(240, 0, 1920, 1080, tm, comp)
	if x != 0 || y != 0 || w != 640 || h != 480 {
		t.Fatalf("visible top-left maps to (%d,%d) %dx%d, want (0,0) 640x480", x, y, w, h)
	}

	x, y, _, _ = mapPresentationMouseToGuest(239, 540, 1920, 1080, tm, comp)
	if x != 0 || y != 240 {
		t.Fatalf("pillarbox maps to (%d,%d), want clamped left edge (0,240)", x, y)
	}
}

func TestMapPresentationMouseToGuest_AROSPresentationTargetReachesBottomRight(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{layer: 0, w: 960, h: 540, frame: solidTestFrame(960, 540, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	tm := NewTerminalMMIO()
	tm.LockMouseNativeResolution(1920, 1080)

	x, y, w, h := mapPresentationMouseToGuest(1919, 1079, 1920, 1080, tm, comp)
	if x != 1919 || y != 1079 || w != 1920 || h != 1080 {
		t.Fatalf("AROS target maps to (%d,%d) %dx%d, want (1919,1079) 1920x1080", x, y, w, h)
	}
}
