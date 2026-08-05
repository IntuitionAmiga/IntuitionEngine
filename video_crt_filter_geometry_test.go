//go:build !headless

package main

import "testing"

func TestCRTHardwareLayerUniformsRetainGuestAndPresentationGeometry(t *testing.T) {
	layer := ebitenHardwareLayer{CompositorFrameLayer: CompositorFrameLayer{
		SourceWidth: 320, SourceHeight: 200,
		DestX: 160, DestY: 40, DestWidth: 1600, DestHeight: 1000,
	}}
	got := crtHardwareLayerUniforms(&layer)
	for name, want := range map[string][]float32{
		"SourceSize": {320, 200},
		"DestSize":   {1600, 1000},
		"DestOrigin": {160, 40},
	} {
		value, ok := got[name].([]float32)
		if !ok || len(value) != 2 || value[0] != want[0] || value[1] != want[1] {
			t.Fatalf("%s = %#v, want %v", name, got[name], want)
		}
	}
}
