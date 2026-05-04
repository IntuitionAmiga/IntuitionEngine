package main

import (
	"bytes"
	"os"
	"testing"
)

func TestEmuTOSDocSync_MentionsXBIOSShim(t *testing.T) {
	data, err := os.ReadFile("sdk/docs/ie_emutos.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte("XBIOS minimal shim (TRAP #14)"),
		[]byte("unsupported XBIOS calls return -1"),
		[]byte("IOREC keyboard pump"),
		[]byte("drop-on-full"),
	} {
		if !bytes.Contains(data, want) {
			t.Fatalf("sdk/docs/ie_emutos.md missing %q", want)
		}
	}
}
