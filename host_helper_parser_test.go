package main

import (
	"reflect"
	"testing"
)

func TestParseHostWiFiScanOutput(t *testing.T) {
	input := "" +
		"Home Network\t86\twpa2-psk\n" +
		"\t70\topen\n" +
		" Office \t67\twpa2-psk\n" +
		"Line%0ABreak\t61\twpa2-psk\n" +
		"Office AP\t42\twpa3-sae\n" +
		"Bad Signal\tnope\twpa2-psk\n" +
		"Coffee Shop\t100\topen\n" +
		"Trailing Tabs\t64\twpa2-psk\tignored\n" +
		"Tabbed%09SSID\t55\twpa2-psk\n" +
		"Percent%25SSID\t54\twpa2-psk\n" +
		"Legacy\t0\twep\n"

	want := []HostWiFiNetwork{
		{SSID: "Home Network", Signal: 86, Security: "wpa2-psk"},
		{SSID: " Office ", Signal: 67, Security: "wpa2-psk"},
		{SSID: "Line\nBreak", Signal: 61, Security: "wpa2-psk"},
		{SSID: "Office AP", Signal: 42, Security: "wpa3-sae"},
		{SSID: "Coffee Shop", Signal: 100, Security: "open"},
		{SSID: "Trailing Tabs", Signal: 64, Security: "wpa2-psk ignored"},
		{SSID: "Tabbed\tSSID", Signal: 55, Security: "wpa2-psk"},
		{SSID: "Percent%SSID", Signal: 54, Security: "wpa2-psk"},
		{SSID: "Legacy", Signal: 0, Security: "wep"},
	}

	got := ParseHostWiFiScanOutput(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseHostWiFiScanOutput() = %#v, want %#v", got, want)
	}
}

func TestFormatHostWiFiScanLineEscapesSSIDSeparators(t *testing.T) {
	network := HostWiFiNetwork{
		SSID:     " Shop\tFront\n",
		Signal:   73,
		Security: "wpa2\tpsk\nsae\rtransition",
	}

	got := FormatHostWiFiScanLine(network)
	want := " Shop%09Front%0A\t73\twpa2 psk sae transition\n"
	if got != want {
		t.Fatalf("FormatHostWiFiScanLine() = %q, want %q", got, want)
	}
}

func TestHostWiFiScanFormatRoundTripNormalizesSecurityLineBreaks(t *testing.T) {
	network := HostWiFiNetwork{
		SSID:     "Office AP",
		Signal:   88,
		Security: "wpa2\nwpa3\rsae",
	}

	got := ParseHostWiFiScanOutput(FormatHostWiFiScanLine(network))
	want := HostWiFiNetwork{
		SSID:     "Office AP",
		Signal:   88,
		Security: "wpa2 wpa3 sae",
	}
	if len(got) != 1 {
		t.Fatalf("round trip produced %d networks, want 1: %#v", len(got), got)
	}
	if got[0] != want {
		t.Fatalf("round trip = %#v, want %#v", got[0], want)
	}
}

func TestHostWiFiScanFormatRoundTripPreservesSSID(t *testing.T) {
	want := HostWiFiNetwork{
		SSID:     " Office\tAP\n",
		Signal:   73,
		Security: "wpa2-psk",
	}

	got := ParseHostWiFiScanOutput(FormatHostWiFiScanLine(want))
	if len(got) != 1 {
		t.Fatalf("round trip produced %d networks, want 1: %#v", len(got), got)
	}
	if got[0] != want {
		t.Fatalf("round trip = %#v, want %#v", got[0], want)
	}
}
