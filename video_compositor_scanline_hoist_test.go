package main

import "testing"

// newTwoLayerScanlineCompositor registers two ScanlineAware sources on distinct
// layers, which forces the multi-entry scanline path (the one whose per-scanline
// method binding was hoisted to per-frame).
func newTwoLayerScanlineCompositor(w, h int) (*VideoCompositor, *mockScanlineSource, *mockScanlineSource) {
	comp := NewVideoCompositor(nil)
	lo := &mockScanlineSource{layer: 0, w: w, h: h, frame: solidTestFrame(w, h, 0x11, 0x22, 0x33, 0xFF)}
	hi := &mockScanlineSource{layer: 1, w: w, h: h, frame: solidTestFrame(w, h, 0x44, 0x55, 0x66, 0xFF)}
	lo.enabled.Store(true)
	hi.enabled.Store(true)
	comp.RegisterSource(lo)
	comp.RegisterSource(hi)
	return comp, lo, hi
}

// TestCompositor_MultiScanlineProcessesEveryLine characterises the multi-entry
// scanline path: every source's ProcessScanline is called once per line, which
// the per-frame method binding must not change.
func TestCompositor_MultiScanlineProcessesEveryLine(t *testing.T) {
	const h = 8
	comp, lo, hi := newTwoLayerScanlineCompositor(4, h)

	_, hasContent, usedScanline := comp.collectScanlineAwareLayers(false)
	if !usedScanline || !hasContent {
		t.Fatalf("usedScanline=%v hasContent=%v, want true/true", usedScanline, hasContent)
	}
	if lo.scanlines != h || hi.scanlines != h {
		t.Fatalf("scanline counts lo=%d hi=%d, want %d each", lo.scanlines, hi.scanlines, h)
	}
}

// BenchmarkCompositeScanline_MultiLayer gates the per-frame method binding. The
// inner loop runs h*entries times, so a per-scanline rebind would show here as
// allocations that scale with height.
func BenchmarkCompositeScanline_MultiLayer(b *testing.B) {
	comp, _, _ := newTwoLayerScanlineCompositor(320, 240)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		comp.collectScanlineAwareLayers(false)
	}
}
