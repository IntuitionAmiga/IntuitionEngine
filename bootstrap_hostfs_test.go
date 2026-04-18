package main

import "testing"

func TestBootstrapHostFSSpecialFileRegistrationClonesBytes(t *testing.T) {
	dev := NewBootstrapHostFSDevice(nil, "")
	orig := []byte{0x7f, 'E', 'L', 'F'}

	dev.SetSpecialFile("IOSSYS/Tools/Shell", orig)
	orig[0] = 0

	got, ok := dev.specialFile("IOSSYS/Tools/Shell")
	if !ok {
		t.Fatal("special file not registered")
	}
	if len(got) != 4 || got[0] != 0x7f || got[1] != 'E' || got[2] != 'L' || got[3] != 'F' {
		t.Fatalf("special file bytes mutated with source slice: %v", got)
	}
}
