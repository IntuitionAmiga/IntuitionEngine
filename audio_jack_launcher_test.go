//go:build linux && cgo && jack

package main

import "testing"

func TestJackALSADevice(t *testing.T) {
	t.Setenv("IE_JACK_ALSA_CARD", "2")
	if got := jackALSADevice(""); got != "hw:2" {
		t.Fatalf("card-derived JACK device = %q, want hw:2", got)
	}
	if got := jackALSADevice(" hw:USB "); got != "hw:USB" {
		t.Fatalf("explicit JACK device = %q, want hw:USB", got)
	}
	t.Setenv("IE_JACK_ALSA_CARD", "")
	if got := jackALSADevice(""); got != "hw:0" {
		t.Fatalf("default JACK device = %q, want hw:0", got)
	}
}
