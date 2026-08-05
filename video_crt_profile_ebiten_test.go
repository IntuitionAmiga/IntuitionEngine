//go:build !headless

package main

import "testing"

func TestCRTProfileGuestAdvancedIsDefault(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	if got := eo.crtProfile; got != crtProfileGuestAdvanced {
		t.Fatalf("default CRT profile = %v, want Guest-Advanced", got)
	}
	if !eo.crtRequested {
		t.Fatal("Guest-Advanced must be enabled by default")
	}
}

func TestCRTProfilesKeepZfastAsFallbackOption(t *testing.T) {
	if got := crtProfileGuestAdvanced.String(); got != "Guest-Advanced" {
		t.Fatalf("Guest-Advanced label = %q", got)
	}
	if got := crtProfileZfast.String(); got != "Zfast" {
		t.Fatalf("Zfast label = %q", got)
	}
}

func TestGuestAdvancedUsesUpstreamDefaultPersistence(t *testing.T) {
	if guestAdvancedPersistence != 0.32 {
		t.Fatalf("Guest-Advanced persistence = %v, want upstream PR/PG/PB default 0.32", guestAdvancedPersistence)
	}
}

func TestGuestAdvancedPassGraphHasScreenOnlyStages(t *testing.T) {
	stages := []struct {
		name   string
		source string
	}{
		{"native raster", guestAdvancedRasterShaderSource},
		{"afterglow", guestAdvancedAfterglowShaderSource},
		{"linearise", guestAdvancedLinearizeShaderSource},
		{"horizontal glow", guestAdvancedHorizontalBlurShaderSource},
		{"vertical bloom", guestAdvancedVerticalBlurShaderSource},
		{"mask and deconvergence", guestAdvancedFinalShaderSource},
	}
	for _, stage := range stages {
		if stage.source == "" {
			t.Fatalf("Guest-Advanced %s stage is absent", stage.name)
		}
	}
}

func TestGuestAdvancedRasterUniformsRetainNativeAndDestinationSizes(t *testing.T) {
	uniforms := guestAdvancedRasterUniforms(320, 200, 1920, 1080)
	if got := uniforms["SourceSize"]; !equalFloat32Slice(got, []float32{320, 200}) {
		t.Fatalf("SourceSize = %v, want native 320x200", got)
	}
	if got := uniforms["DestSize"]; !equalFloat32Slice(got, []float32{1920, 1080}) {
		t.Fatalf("DestSize = %v, want 1920x1080", got)
	}
}

func equalFloat32Slice(got any, want []float32) bool {
	actual, ok := got.([]float32)
	if !ok || len(actual) != len(want) {
		return false
	}
	for i := range want {
		if actual[i] != want[i] {
			return false
		}
	}
	return true
}
