//go:build !headless

package main

import (
	"testing"
	"time"
)

func TestEbitenOutput_UpdateFrame_RejectsWrongSize(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	want := eo.width * eo.height * 4
	if err := eo.UpdateFrame(make([]byte, want)); err != nil {
		t.Fatalf("valid frame rejected: %v", err)
	}
	if err := eo.UpdateFrame(make([]byte, want-1)); err == nil {
		t.Fatal("short frame was accepted")
	}
	if err := eo.UpdateFrame(make([]byte, want+1)); err == nil {
		t.Fatal("long frame was accepted")
	}
}

func TestEbitenOutput_UpdateRegion_RejectsShortPixels(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	if err := eo.UpdateRegion(0, 0, 2, 2, make([]byte, 2*2*4-1)); err == nil {
		t.Fatal("short region pixels were accepted")
	}
}

func TestEbitenOutput_WaitForVSync_AfterStop_DoesNotBlock(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	if err := eo.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	errc := make(chan error, 1)
	go func() {
		errc <- eo.WaitForVSync()
	}()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("WaitForVSync returned nil after Stop")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("WaitForVSync blocked after Stop")
	}
}
