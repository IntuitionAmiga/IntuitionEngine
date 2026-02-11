//go:build headless

package main

import "testing"

func TestHeadlessOutput_SetDisplayConfig_StoresFullscreen(t *testing.T) {
	out := &HeadlessVideoOutput{}
	cfg := DisplayConfig{
		Width:      640,
		Height:     480,
		Scale:      2,
		Fullscreen: true,
	}
	if err := out.SetDisplayConfig(cfg); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
	}
	got := out.GetDisplayConfig()
	if !got.Fullscreen {
		t.Fatal("expected Fullscreen=true")
	}
}

func TestHeadlessOutput_DisplayConfig_ScaleAndFullscreen(t *testing.T) {
	out := &HeadlessVideoOutput{}
	cfg := DisplayConfig{
		Width:      320,
		Height:     240,
		Scale:      2,
		Fullscreen: true,
	}
	if err := out.SetDisplayConfig(cfg); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
	}
	got := out.GetDisplayConfig()
	if got.Scale != 2 || !got.Fullscreen {
		t.Fatalf("expected Scale=2, Fullscreen=true; got Scale=%d, Fullscreen=%v", got.Scale, got.Fullscreen)
	}
}
