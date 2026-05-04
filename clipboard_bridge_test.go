package main

import "testing"

func TestClipboardBounds_RejectsWrap(t *testing.T) {
	if clipboardBoundsOK(0xFFFFFFF0, 0x20, 0xFFFFFFFF) {
		t.Fatalf("clipboardBoundsOK accepted wrapping ptr+len")
	}
	if !clipboardBoundsOK(4, 8, 16) {
		t.Fatalf("clipboardBoundsOK rejected in-range span")
	}
}
