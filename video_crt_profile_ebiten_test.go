//go:build !headless

package main

import (
	"strings"
	"testing"
)

func TestGuestAdvancedConvexWarpGeometry(t *testing.T) {
	centreX, centreY := guestAdvancedWarpUV(0.5, 0.5, guestAdvancedCurvatureX, guestAdvancedCurvatureY, guestAdvancedCurvatureShape)
	if centreX != 0.5 || centreY != 0.5 {
		t.Fatalf("convex warp moved screen centre to (%v, %v)", centreX, centreY)
	}

	cornerX, cornerY := guestAdvancedWarpUV(0, 0, guestAdvancedCurvatureX, guestAdvancedCurvatureY, guestAdvancedCurvatureShape)
	if cornerX >= 0 || cornerY >= 0 {
		t.Fatalf("convex warp did not move top-left corner outside the CRT face: (%v, %v)", cornerX, cornerY)
	}

	leftX, leftY := guestAdvancedWarpUV(0.25, 0.25, guestAdvancedCurvatureX, guestAdvancedCurvatureY, guestAdvancedCurvatureShape)
	rightX, rightY := guestAdvancedWarpUV(0.75, 0.75, guestAdvancedCurvatureX, guestAdvancedCurvatureY, guestAdvancedCurvatureShape)
	if leftX >= 0.25 || leftY >= 0.25 || rightX <= 0.75 || rightY <= 0.75 {
		t.Fatalf("convex warp lacks symmetric outward curvature: left=(%v,%v) right=(%v,%v)", leftX, leftY, rightX, rightY)
	}

	plainX, plainY := guestAdvancedWarpUV(0.25, 0.75, guestAdvancedCurvatureX, guestAdvancedCurvatureY, 0)
	if plainX != 0.25 || plainY != 0.75 {
		t.Fatalf("zero curvature shape changed coordinates to (%v, %v)", plainX, plainY)
	}
}

func TestGuestAdvancedCurvatureSelection(t *testing.T) {
	if x, y, shape := guestAdvancedCurvature(false); x != 0 || y != 0 || shape != 0 {
		t.Fatalf("flat CRT curvature = (%v, %v, %v), want zero", x, y, shape)
	}
	if x, y, shape := guestAdvancedCurvature(true); x != guestAdvancedCurvatureX || y != guestAdvancedCurvatureY || shape != guestAdvancedCurvatureShape {
		t.Fatalf("curved CRT curvature = (%v, %v, %v), want (%v, %v, %v)", x, y, shape, guestAdvancedCurvatureX, guestAdvancedCurvatureY, guestAdvancedCurvatureShape)
	}
}

func TestGuestAdvancedFinalWarpSamplesGaussianGlow(t *testing.T) {
	if !strings.Contains(guestAdvancedFinalShaderSource, "p2 := imageSrc2Origin()+uv*ScreenSize") {
		t.Fatal("final CRT pass does not derive Gaussian-glow coordinates from the convex warp")
	}
	if strings.Contains(guestAdvancedFinalShaderSource, "imageSrc2Origin()+srcPos") {
		t.Fatal("final CRT pass still samples Gaussian glow at the unwarped coordinate")
	}
}

func TestCRTProfileGuestAdvancedIsDefault(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	if got := eo.crtProfile; got != crtProfileGuestAdvanced {
		t.Fatalf("default CRT profile = %v, want Guest-Advanced", got)
	}
	if eo.crtMode != crtModeOff {
		t.Fatal("CRT presentation must be off by default")
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
		{"afterglow preparation", guestAdvancedPreAfterglowShaderSource},
		{"average luminance", guestAdvancedAverageLuminanceShaderSource},
		{"linearise", guestAdvancedLinearizeShaderSource},
		{"horizontal Gaussian glow", guestAdvancedGaussianHorizontalShaderSource},
		{"vertical Gaussian glow", guestAdvancedGaussianVerticalShaderSource},
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
