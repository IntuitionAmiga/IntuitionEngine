package main

import "testing"

func TestValidateResolutionOverride_BothSet(t *testing.T) {
	w, h, ok := validateResolutionOverride(800, 600)
	if !ok {
		t.Fatal("expected override to be accepted")
	}
	if w != 800 || h != 600 {
		t.Fatalf("expected (800,600), got (%d,%d)", w, h)
	}
}

func TestValidateResolutionOverride_NeitherSet(t *testing.T) {
	w, h, ok := validateResolutionOverride(0, 0)
	if ok {
		t.Fatal("expected override to be disabled")
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", w, h)
	}
}

func TestValidateResolutionOverride_OnlyWidth(t *testing.T) {
	w, h, ok := validateResolutionOverride(800, 0)
	if ok {
		t.Fatal("expected partial override to be rejected")
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", w, h)
	}
}

func TestValidateResolutionOverride_OnlyHeight(t *testing.T) {
	w, h, ok := validateResolutionOverride(0, 600)
	if ok {
		t.Fatal("expected partial override to be rejected")
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0), got (%d,%d)", w, h)
	}
}
