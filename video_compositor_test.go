// video_compositor_test.go - Tests and benchmarks for video compositor

package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

	// Scanline path should have been used - both mock sources should have
	// received ProcessScanline calls (480 scanlines each)
	if chip.scanlines != 480 {
		t.Errorf("chip: expected 480 ProcessScanline calls, got %d (scanline path not used)", chip.scanlines)
	}
	if vga.scanlines != 480 {
		t.Errorf("vga: expected 480 ProcessScanline calls, got %d (scanline path not used)", vga.scanlines)
	}
}

type mockVideoOutput struct {
	mu        sync.Mutex
	started   bool
	config    DisplayConfig
	setCalls  int
	updateErr error
	setErr    error
}

func newMockVideoOutput() *mockVideoOutput {
	return &mockVideoOutput{
		config: DisplayConfig{
			Width:       640,
			Height:      480,
			Scale:       1,
			PixelFormat: PixelFormatRGBA,
			RefreshRate: 60,
			VSync:       true,
		},
	}
}

func (m *mockVideoOutput) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	return nil
}

func (m *mockVideoOutput) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = false
	return nil
}

func (m *mockVideoOutput) Close() error { return m.Stop() }

func (m *mockVideoOutput) IsStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockVideoOutput) SetDisplayConfig(config DisplayConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalls++
	m.config = config
	return m.setErr
}

func (m *mockVideoOutput) GetDisplayConfig() DisplayConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

func (m *mockVideoOutput) UpdateFrame(buffer []byte) error { return m.updateErr }
func (m *mockVideoOutput) WaitForVSync() error             { return nil }
func (m *mockVideoOutput) GetFrameCount() uint64           { return 0 }
func (m *mockVideoOutput) GetRefreshRate() int             { return 60 }

func TestCompositor_SetDimensions_UpdatesFrameSize(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(800, 600)
	if comp.frameWidth != 800 || comp.frameHeight != 600 {
		t.Fatalf("expected 800x600, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	if len(comp.finalFrame) != 800*600*4 {
		t.Fatalf("expected finalFrame len %d, got %d", 800*600*4, len(comp.finalFrame))
	}
}

func TestCompositor_NotifyResolutionChange_AppliesOnComposite(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.NotifyResolutionChange(800, 600)
	if comp.frameWidth != 640 {
		t.Fatalf("expected width unchanged before composite, got %d", comp.frameWidth)
	}
	comp.composite()
	if comp.frameWidth != 800 || comp.frameHeight != 600 {
		t.Fatalf("expected 800x600, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	cfg := out.GetDisplayConfig()
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("expected output config 800x600, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestCompositor_NotifyResolutionChange_LastWriterWins(t *testing.T) {
	comp := NewVideoCompositor(newMockVideoOutput())
	comp.NotifyResolutionChange(800, 600)
	comp.NotifyResolutionChange(1024, 768)
	comp.composite()
	if comp.frameWidth != 1024 || comp.frameHeight != 768 {
		t.Fatalf("expected 1024x768, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
}

func TestCompositor_LockResolution_IgnoresNotifications(t *testing.T) {
	comp := NewVideoCompositor(newMockVideoOutput())
	comp.LockResolution(320, 240)
	comp.NotifyResolutionChange(800, 600)
	comp.composite()
	if comp.frameWidth != 320 || comp.frameHeight != 240 {
		t.Fatalf("expected locked 320x240, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
}

func TestCompositor_LockResolution_PropagatesConfig_Started(t *testing.T) {
	out := newMockVideoOutput()
	_ = out.Start()
	comp := NewVideoCompositor(out)
	comp.LockResolution(800, 600)
	cfg := out.GetDisplayConfig()
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("expected output config 800x600, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestCompositor_LockResolution_PropagatesConfig_PreStart(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(800, 600)
	cfg := out.GetDisplayConfig()
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("expected output config 800x600, got %dx%d", cfg.Width, cfg.Height)
	}
	if comp.frameWidth != 800 || comp.frameHeight != 600 {
		t.Fatalf("expected compositor 800x600, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	_ = out.Start()
	comp.composite()
	if len(comp.finalFrame) != 800*600*4 {
		t.Fatalf("expected finalFrame len %d, got %d", 800*600*4, len(comp.finalFrame))
	}
}

func TestCompositor_ApplyResolution_NoDuplicateUpdate(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.NotifyResolutionChange(640, 480)
	comp.composite()
	if out.setCalls != 0 {
		t.Fatalf("expected no SetDisplayConfig calls, got %d", out.setCalls)
	}
}

func TestCompositor_ApplyResolution_OutputError_ContinuesGracefully(t *testing.T) {
	out := newMockVideoOutput()
	out.setErr = errors.New("set config failed")
	comp := NewVideoCompositor(out)
	comp.NotifyResolutionChange(800, 600)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("composite panicked: %v", r)
			}
		}()
		comp.composite()
		comp.composite()
	}()
	if comp.frameWidth != 800 || comp.frameHeight != 600 {
		t.Fatalf("expected compositor 800x600 after error, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
}

func TestDisplayConfig_FullscreenDefaultFalse(t *testing.T) {
	var config DisplayConfig
	if config.Fullscreen {
		t.Fatal("expected zero-value Fullscreen to be false")
	}
}

func TestClampScale(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: 0, want: 1},
		{in: -1, want: 1},
		{in: 1, want: 1},
		{in: 2, want: 2},
		{in: 4, want: 4},
		{in: 5, want: 4},
		{in: 999, want: 4},
	}
	for _, tc := range cases {
		if got := ClampScale(tc.in); got != tc.want {
			t.Fatalf("ClampScale(%d): want %d, got %d", tc.in, tc.want, got)
		}
	}
}

func newTestVideoChip(out VideoOutput) *VideoChip {
	chip := &VideoChip{
		output:      out,
		currentMode: MODE_640x480,
		layer:       VIDEOCHIP_LAYER,
		vsyncChan:   make(chan struct{}),
		done:        make(chan struct{}),
		prevVRAM:    make([]byte, VRAM_SIZE),
	}
	mode := VideoModes[chip.currentMode]
	chip.frontBuffer = make([]byte, mode.totalSize)
	chip.backBuffer = make([]byte, mode.totalSize)
	chip.initialiseDirtyGrid(mode)
	return chip
}

func TestVideoChip_ModeChange_FiresCallback(t *testing.T) {
	chip := newTestVideoChip(newMockVideoOutput())
	var gotW, gotH int
	chip.SetResolutionChangeCallback(func(w, h int) {
		gotW, gotH = w, h
	})
	chip.HandleWrite(VIDEO_MODE, MODE_800x600)
	if gotW != 800 || gotH != 600 {
		t.Fatalf("expected callback 800x600, got %dx%d", gotW, gotH)
	}
	if len(chip.frontBuffer) != 800*600*4 {
		t.Fatalf("expected frontBuffer len %d, got %d", 800*600*4, len(chip.frontBuffer))
	}
}

func TestVideoChip_ModeChange_NilCallback_FallsBackToDirectOutput(t *testing.T) {
	out := newMockVideoOutput()
	chip := newTestVideoChip(out)
	chip.HandleWrite(VIDEO_MODE, MODE_800x600)
	cfg := out.GetDisplayConfig()
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("expected output config 800x600, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestVideoChip_CtrlEnable_FiresCallback(t *testing.T) {
	chip := newTestVideoChip(newMockVideoOutput())
	var gotW, gotH int
	chip.SetResolutionChangeCallback(func(w, h int) {
		gotW, gotH = w, h
	})
	chip.HandleWrite(VIDEO_CTRL, 1)
	if gotW != 640 || gotH != 480 {
		t.Fatalf("expected callback 640x480, got %dx%d", gotW, gotH)
	}
}

func TestVideoChip_CtrlEnable_NilCallback_FallsBackToStartOutput(t *testing.T) {
	out := newMockVideoOutput()
	chip := newTestVideoChip(out)
	chip.HandleWrite(VIDEO_CTRL, 1)
	if !out.IsStarted() {
		t.Fatal("expected output to be started")
	}
}

func TestVideoChip_ModeChange_WithCompositor_EndToEnd(t *testing.T) {
	out := newMockVideoOutput()
	chip := newTestVideoChip(out)
	comp := NewVideoCompositor(out)
	chip.SetResolutionChangeCallback(func(w, h int) {
		comp.NotifyResolutionChange(w, h)
	})
	chip.HandleWrite(VIDEO_MODE, MODE_800x600)
	comp.composite()
	if comp.frameWidth != 800 || comp.frameHeight != 600 {
		t.Fatalf("expected compositor 800x600, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
}

func TestVoodoo_DimensionChange_FiresCallback(t *testing.T) {
	v := &VoodooEngine{}
	v.width.Store(VOODOO_DEFAULT_WIDTH)
	v.height.Store(VOODOO_DEFAULT_HEIGHT)
	var gotW, gotH int
	v.SetResolutionChangeCallback(func(w, h int) {
		gotW, gotH = w, h
	})
	v.HandleWrite(VOODOO_VIDEO_DIM, (320<<16)|200)
	if gotW != 320 || gotH != 200 {
		t.Fatalf("expected callback 320x200, got %dx%d", gotW, gotH)
	}
}

func TestVoodoo_DimensionChange_NilCallback_ExistingBehavior(t *testing.T) {
	v := &VoodooEngine{}
	v.width.Store(VOODOO_DEFAULT_WIDTH)
	v.height.Store(VOODOO_DEFAULT_HEIGHT)
	v.HandleWrite(VOODOO_VIDEO_DIM, (320<<16)|200)
	if int(v.width.Load()) != 320 || int(v.height.Load()) != 200 {
		t.Fatalf("expected dimensions 320x200, got %dx%d", v.width.Load(), v.height.Load())
	}
}

func TestCompositor_FullIntegration_ModeChangePropagatesToOutput(t *testing.T) {
	out := newMockVideoOutput()
	chip := newTestVideoChip(out)
	comp := NewVideoCompositor(out)
	comp.RegisterSource(chip)
	chip.SetResolutionChangeCallback(func(w, h int) {
		comp.NotifyResolutionChange(w, h)
	})

	if err := comp.Start(); err != nil {
		t.Fatalf("compositor start: %v", err)
	}
	defer comp.Stop()

	// Trigger mode change while compositor refresh loop is running
	chip.HandleWrite(VIDEO_MODE, MODE_1024x768)

	// Poll with timeout for the compositor to pick up the change
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		cfg := out.GetDisplayConfig()
		if cfg.Width == 1024 && cfg.Height == 768 {
			return // success
		}
		time.Sleep(5 * time.Millisecond)
	}
	cfg := out.GetDisplayConfig()
	t.Fatalf("expected output config 1024x768 within 200ms, got %dx%d", cfg.Width, cfg.Height)
}
