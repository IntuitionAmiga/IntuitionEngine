//go:build !headless

// video_backend_ebiten_retained_test.go - Slice 8: retained-layer decision logic

package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestEbitenRetainedLayer_UploadSkipDecision proves the WritePixels-skip logic:
// a retained layer skips upload only when the retained image already holds the
// same source's identical generation, and the kill switch forces upload.
func TestEbitenRetainedLayer_UploadSkipDecision(t *testing.T) {
	base := func() *ebitenHardwareLayer {
		l := &ebitenHardwareLayer{haveUpload: true}
		l.SourceID = 7
		l.ContentGen = 42
		l.uploadedSourceID = 7
		l.uploadedGen = 42
		return l
	}

	cases := []struct {
		name     string
		mutate   func(l *ebitenHardwareLayer)
		newImage bool
		retained bool
		want     bool
	}{
		{"identical retained", nil, false, true, true},
		{"kill switch off", nil, false, false, false},
		{"new image", nil, true, true, false},
		{"never uploaded", func(l *ebitenHardwareLayer) { l.haveUpload = false }, false, true, false},
		{"generation changed", func(l *ebitenHardwareLayer) { l.ContentGen = 43 }, false, true, false},
		{"source changed", func(l *ebitenHardwareLayer) { l.SourceID = 8 }, false, true, false},
	}
	for _, tc := range cases {
		l := base()
		if tc.mutate != nil {
			tc.mutate(l)
		}
		if got := l.retainedUploadSkippable(tc.newImage, tc.retained); got != tc.want {
			t.Errorf("%s: retainedUploadSkippable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestEbitenRetainedLayer_GeomReuseDecision proves cached geometry is reused
// only when valid, keyed and matching, and never when the kill switch is off.
func TestEbitenRetainedLayer_GeomReuseDecision(t *testing.T) {
	key := ebitenLayerGeomKey{sw: 2, sh: 2, dw: 4, dh: 4}
	l := &ebitenHardwareLayer{
		geomValid:     true,
		geomKey:       key,
		cachedOptions: &ebiten.DrawTrianglesShaderOptions{},
	}
	if !l.geomReusable(true, key) {
		t.Fatal("matching key should reuse geometry")
	}
	if l.geomReusable(false, key) {
		t.Fatal("kill switch off must rebuild geometry")
	}
	if l.geomReusable(true, ebitenLayerGeomKey{sw: 3, sh: 2, dw: 4, dh: 4}) {
		t.Fatal("changed key must rebuild geometry")
	}
	l.geomValid = false
	if l.geomReusable(true, key) {
		t.Fatal("invalid cache must rebuild geometry")
	}
}
