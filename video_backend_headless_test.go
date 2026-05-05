//go:build headless

package main

import (
	"sync"
	"testing"
)

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

func TestHeadlessOutput_UpdateFrame_RejectsWrongSize(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	h := out.(*HeadlessVideoOutput)
	cfg := h.GetDisplayConfig()
	want := cfg.Width * cfg.Height * 4
	if err := h.UpdateFrame(make([]byte, want)); err != nil {
		t.Fatalf("valid frame rejected: %v", err)
	}
	if err := h.UpdateFrame(make([]byte, want-1)); err == nil {
		t.Fatal("short frame was accepted")
	}
	if err := h.UpdateFrame(make([]byte, want+1)); err == nil {
		t.Fatal("long frame was accepted")
	}
}

func TestHeadlessOutput_ParallelLifecycle(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	h := out.(*HeadlessVideoOutput)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Start(); err != nil {
				t.Errorf("Start returned error: %v", err)
			}
			_ = h.IsStarted()
			if err := h.Stop(); err != nil {
				t.Errorf("Stop returned error: %v", err)
			}
		}()
	}
	wg.Wait()
}
