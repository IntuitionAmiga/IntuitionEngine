// video_compositor_test.go - Tests and benchmarks for video compositor

package main

import (
	"sync/atomic"
	"testing"
)

// BenchmarkFrameClear_Loop benchmarks the old loop-based frame clear
func BenchmarkFrameClear_Loop(b *testing.B) {
	// 640x480x4 = 1,228,800 bytes
	frame := make([]byte, 640*480*4)
	// Pre-fill with some data
	for i := range frame {
		frame[i] = 0xFF
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for j := range frame {
			frame[j] = 0
		}
	}
}

// BenchmarkFrameClear_Copy benchmarks the optimized copy-based frame clear
func BenchmarkFrameClear_Copy(b *testing.B) {
	// 640x480x4 = 1,228,800 bytes
	frameSize := 640 * 480 * 4
	frame := make([]byte, frameSize)
	zeroFrame := make([]byte, frameSize)
	// Pre-fill with some data
	for i := range frame {
		frame[i] = 0xFF
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copy(frame, zeroFrame)
	}
}

// BenchmarkFrameClear_Copy_1080p benchmarks copy for 1920x1080 resolution
func BenchmarkFrameClear_Copy_1080p(b *testing.B) {
	frameSize := 1920 * 1080 * 4
	frame := make([]byte, frameSize)
	zeroFrame := make([]byte, frameSize)
	for i := range frame {
		frame[i] = 0xFF
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copy(frame, zeroFrame)
	}
}

// mockScanlineSource implements both VideoSource and ScanlineAware for testing.
type mockScanlineSource struct {
	enabled   atomic.Bool
	layer     int
	w, h      int
	frame     []byte
	scanlines int // counts ProcessScanline calls per frame
}

func (m *mockScanlineSource) GetFrame() []byte          { return m.frame }
func (m *mockScanlineSource) IsEnabled() bool           { return m.enabled.Load() }
func (m *mockScanlineSource) GetLayer() int             { return m.layer }
func (m *mockScanlineSource) GetDimensions() (int, int) { return m.w, m.h }
func (m *mockScanlineSource) SignalVSync()              {}
func (m *mockScanlineSource) StartFrame()               { m.scanlines = 0 }
func (m *mockScanlineSource) ProcessScanline(y int)     { m.scanlines++ }
func (m *mockScanlineSource) FinishFrame() []byte       { return m.frame }

// TestCompositor_ScanlineAware_WithDisabledVoodoo verifies that when a Voodoo
// source exists but is disabled, compositeScanlineAware still works for the
// remaining ScanlineAware sources (VideoChip + VGA copper demos).
// Regression: if Voodoo starts enabled, the compositor falls back to full-frame
// rendering because Voodoo doesn't implement ScanlineAware.
func TestCompositor_ScanlineAware_WithDisabledVoodoo(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(640, 480)

	// Two ScanlineAware sources (simulate VideoChip + VGA)
	chip := &mockScanlineSource{layer: 0, w: 640, h: 480, frame: make([]byte, 640*480*4)}
	chip.enabled.Store(true)
	vga := &mockScanlineSource{layer: 10, w: 640, h: 480, frame: make([]byte, 640*480*4)}
	vga.enabled.Store(true)

	// Voodoo: registered but disabled (default state after NewVoodooEngine)
	voodoo, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine: %v", err)
	}
	defer voodoo.Destroy()

	// Voodoo must be disabled by default
	if voodoo.IsEnabled() {
		t.Fatal("Voodoo should be disabled by default")
	}

	comp.RegisterSource(chip)
	comp.RegisterSource(vga)
	comp.RegisterSource(voodoo)

	// Run one composite cycle
	comp.composite()

	// Scanline path should have been used — both mock sources should have
	// received ProcessScanline calls (480 scanlines each)
	if chip.scanlines != 480 {
		t.Errorf("chip: expected 480 ProcessScanline calls, got %d (scanline path not used)", chip.scanlines)
	}
	if vga.scanlines != 480 {
		t.Errorf("vga: expected 480 ProcessScanline calls, got %d (scanline path not used)", vga.scanlines)
	}
}
