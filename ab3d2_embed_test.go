//go:build embed_ab3d2

package main

import "testing"

func TestShouldAutostartAB3D2_EmbeddedBuildTrue(t *testing.T) {
	if !shouldAutostartAB3D2() {
		t.Fatal("AB3D2 autostart should be enabled with embed_ab3d2")
	}
}

func TestEmbeddedAB3D2DefaultBoot_OnlyForDefaultM68KLaunch(t *testing.T) {
	if !isEmbeddedAB3D2DefaultBoot(true, "") {
		t.Fatal("embedded default M68K launch should reset to AB3D2")
	}
	if isEmbeddedAB3D2DefaultBoot(false, "") {
		t.Fatal("non-M68K launch should not reset to AB3D2")
	}
	if isEmbeddedAB3D2DefaultBoot(true, "program.ie68") {
		t.Fatal("explicit M68K filename should reset using normal launch semantics")
	}
}

func TestShouldStartFullscreen_EmbeddedOverdriveBuildFlag(t *testing.T) {
	old := EmbeddedAB3D2StartFullscreen
	EmbeddedAB3D2StartFullscreen = "1"
	t.Cleanup(func() { EmbeddedAB3D2StartFullscreen = old })

	if !shouldStartFullscreen(false, true, "") {
		t.Fatal("embedded AB3D2 Overdrive build should start fullscreen")
	}
}
