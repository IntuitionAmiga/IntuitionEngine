//go:build !embed_ab3d2

package main

import "testing"

func TestShouldAutostartAB3D2_DefaultBuildFalse(t *testing.T) {
	if shouldAutostartAB3D2() {
		t.Fatal("AB3D2 autostart should be disabled without embed_ab3d2")
	}
}

func TestEmbeddedAB3D2DefaultBoot_DefaultBuildFalse(t *testing.T) {
	if isEmbeddedAB3D2DefaultBoot(true, "") {
		t.Fatal("default build must not treat F10 as embedded AB3D2 default boot")
	}
}
