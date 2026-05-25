package main

import "testing"

func TestShouldStartFullscreen_ExplicitFlagWins(t *testing.T) {
	if !shouldStartFullscreen(true, false, "") {
		t.Fatal("explicit fullscreen flag should start fullscreen")
	}
}

func TestShouldStartFullscreen_ExplicitOverdriveM68KProgram(t *testing.T) {
	if !shouldStartFullscreen(false, true, "/tmp/ab3d2_ie68_overdrive.ie68") {
		t.Fatal("explicit Overdrive M68K image should start fullscreen")
	}
}

func TestShouldStartFullscreen_OverdriveNameRequiresM68K(t *testing.T) {
	if !shouldStartFullscreen(false, false, "/tmp/ab3d2_ie68_overdrive.ie68") {
		t.Fatal("all launches should start fullscreen by default")
	}
}

func TestShouldStartFullscreen_NonOverdriveM68KProgramStartsFullscreen(t *testing.T) {
	if !shouldStartFullscreen(false, true, "/tmp/ab3d2_ie68_redux_high.ie68") {
		t.Fatal("non-Overdrive M68K image should start fullscreen by default")
	}
}

func TestEmbeddedAB3D2StartFullscreenEnabledParsesBuildFlag(t *testing.T) {
	old := EmbeddedAB3D2StartFullscreen
	t.Cleanup(func() { EmbeddedAB3D2StartFullscreen = old })

	for _, value := range []string{"1", "true", "TRUE", "yes", "on"} {
		EmbeddedAB3D2StartFullscreen = value
		if !embeddedAB3D2StartFullscreenEnabled() {
			t.Fatalf("EmbeddedAB3D2StartFullscreen=%q should enable fullscreen", value)
		}
	}

	for _, value := range []string{"", "0", "false", "off", "overdrive"} {
		EmbeddedAB3D2StartFullscreen = value
		if embeddedAB3D2StartFullscreenEnabled() {
			t.Fatalf("EmbeddedAB3D2StartFullscreen=%q should not enable fullscreen", value)
		}
	}
}

func TestLiveImageModeEnabledParsesEnv(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv("IE_LIVE_IMAGE", value)
		if !liveImageModeEnabled() {
			t.Fatalf("IE_LIVE_IMAGE=%q should enable live image mode", value)
		}
	}

	for _, value := range []string{"", "0", "false", "off", "live"} {
		t.Setenv("IE_LIVE_IMAGE", value)
		if liveImageModeEnabled() {
			t.Fatalf("IE_LIVE_IMAGE=%q should not enable live image mode", value)
		}
	}
}
