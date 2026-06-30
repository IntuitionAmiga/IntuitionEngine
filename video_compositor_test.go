// video_compositor_test.go - Tests and benchmarks for video compositor

package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompositorOpaquePixelTreatsRGBWithZeroAlphaAsOpaque(t *testing.T) {
	got, ok := compositorOpaquePixel(0x00332211)
	if !ok {
		t.Fatal("RGB pixel with zero alpha was treated as transparent")
	}
	if got != 0xFF332211 {
		t.Fatalf("opaque pixel=0x%08X, want 0xFF332211", got)
	}

	if _, ok := compositorOpaquePixel(0x00000000); ok {
		t.Fatal("black zero-alpha pixel should remain transparent")
	}
}

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
	vsyncs    atomic.Int32
}

func (m *mockScanlineSource) GetFrame() []byte          { return m.frame }
func (m *mockScanlineSource) IsEnabled() bool           { return m.enabled.Load() }
func (m *mockScanlineSource) GetLayer() int             { return m.layer }
func (m *mockScanlineSource) GetDimensions() (int, int) { return m.w, m.h }
func (m *mockScanlineSource) SignalVSync()              { m.vsyncs.Add(1) }
func (m *mockScanlineSource) StartFrame()               { m.scanlines = 0 }
func (m *mockScanlineSource) ProcessScanline(y int)     { m.scanlines++ }
func (m *mockScanlineSource) FinishFrame() []byte       { return m.frame }

type mockOpaqueSource struct {
	enabled atomic.Bool
	layer   int
	w, h    int
	frame   []byte
	vsyncs  atomic.Int32
	panicOn string
}

func (m *mockOpaqueSource) GetFrame() []byte {
	if m.panicOn == "GetFrame" {
		panic("mock getframe panic")
	}
	return m.frame
}
func (m *mockOpaqueSource) IsEnabled() bool           { return m.enabled.Load() }
func (m *mockOpaqueSource) GetLayer() int             { return m.layer }
func (m *mockOpaqueSource) GetDimensions() (int, int) { return m.w, m.h }
func (m *mockOpaqueSource) SignalVSync() {
	if m.panicOn == "SignalVSync" {
		panic("mock vsync panic")
	}
	m.vsyncs.Add(1)
}

type managedScanlineSource struct {
	mockScanlineSource
	managedFalse atomic.Int32
	panicY       int
	ys           []int
}

func (m *managedScanlineSource) SetCompositorManaged(managed bool) {
	if !managed {
		m.managedFalse.Add(1)
	}
}
func (m *managedScanlineSource) WaitRenderIdle() {}
func (m *managedScanlineSource) ProcessScanline(y int) {
	m.ys = append(m.ys, y)
	if y == m.panicY {
		panic("mock scanline panic")
	}
	m.mockScanlineSource.ProcessScanline(y)
}

type selectableScanlineSource struct {
	mockScanlineSource
	needs bool
}

func (m *selectableScanlineSource) NeedsScanlineCompositing() bool {
	return m.needs
}

func solidTestFrame(w, h int, r, g, b, a byte) []byte {
	frame := make([]byte, w*h*BYTES_PER_PIXEL)
	for i := 0; i < len(frame); i += BYTES_PER_PIXEL {
		frame[i] = r
		frame[i+1] = g
		frame[i+2] = b
		frame[i+3] = a
	}
	return frame
}

func testPixel(frame []byte, x, y, w int) [4]byte {
	i := (y*w + x) * BYTES_PER_PIXEL
	return [4]byte{frame[i], frame[i+1], frame[i+2], frame[i+3]}
}

func setTestPixel(frame []byte, x, y, w int, r, g, b, a byte) {
	i := (y*w + x) * BYTES_PER_PIXEL
	frame[i] = r
	frame[i+1] = g
	frame[i+2] = b
	frame[i+3] = a
}

type mockTickSource struct {
	mockScanlineSource
	ticks atomic.Int32
}

func (m *mockTickSource) TickFrame() {
	m.ticks.Add(1)
}

func TestCompositorTickFrameFiresWhenSourceDisabled(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(16, 16)

	source := &mockTickSource{}
	source.enabled.Store(false)
	source.w = 16
	source.h = 16
	source.frame = make([]byte, 16*16*4)
	comp.RegisterSource(source)

	comp.composite()

	if got := source.ticks.Load(); got != 1 {
		t.Fatalf("ticks = %d, want 1", got)
	}
}

func TestCompositor_FullFrame_RespectsLayerOrder(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(2, 1)

	top := &mockOpaqueSource{layer: 10, w: 2, h: 1, frame: solidTestFrame(2, 1, 0xAA, 0, 0, 0xFF)}
	top.enabled.Store(true)
	bottom := &mockOpaqueSource{layer: 0, w: 2, h: 1, frame: solidTestFrame(2, 1, 0, 0xBB, 0, 0xFF)}
	bottom.enabled.Store(true)
	comp.RegisterSource(top)
	comp.RegisterSource(bottom)

	comp.composite()

	if got := testPixel(comp.finalFrame, 0, 0, 2); got != [4]byte{0xAA, 0, 0, 0xFF} {
		t.Fatalf("top layer pixel = %v", got)
	}
}

func TestCompositor_Unregister_RemovesSource(t *testing.T) {
	comp := NewVideoCompositor(nil)
	src := &mockOpaqueSource{layer: 0, w: 1, h: 1, frame: solidTestFrame(1, 1, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	id := comp.RegisterSourceWithID(src)
	if !comp.UnregisterSource(id) {
		t.Fatal("expected source to unregister")
	}
	if comp.UnregisterSource(id) {
		t.Fatal("second unregister unexpectedly succeeded")
	}
	comp.SetDimensions(1, 1)
	comp.composite()
	if got := testPixel(comp.finalFrame, 0, 0, 1); got != [4]byte{} {
		t.Fatalf("unregistered source still composed: %v", got)
	}
	if len(comp.sources) != 0 {
		t.Fatalf("sources len = %d, want 0", len(comp.sources))
	}
}

func TestCompositor_RegisterSource_StableOrder(t *testing.T) {
	comp := NewVideoCompositor(nil)
	a := &mockOpaqueSource{layer: 20}
	b := &mockOpaqueSource{layer: 10}
	csrc := &mockOpaqueSource{layer: 10}
	d := &mockOpaqueSource{layer: -1}
	comp.RegisterSource(a)
	comp.RegisterSource(b)
	comp.RegisterSource(csrc)
	comp.RegisterSource(d)
	got := []VideoSource{comp.sources[0].source, comp.sources[1].source, comp.sources[2].source, comp.sources[3].source}
	want := []VideoSource{d, b, csrc, a}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("source order[%d] = %T/%p, want %T/%p", i, got[i], got[i], want[i], want[i])
		}
	}
}

func TestCompositor_MixedScanline_OpaqueBetween(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(1, 1)
	chip := &mockScanlineSource{layer: 0, w: 1, h: 1, frame: solidTestFrame(1, 1, 0x10, 0, 0, 0xFF)}
	chip.enabled.Store(true)
	opaque := &mockOpaqueSource{layer: 5, w: 1, h: 1, frame: solidTestFrame(1, 1, 0, 0x20, 0, 0xFF)}
	opaque.enabled.Store(true)
	vga := &mockScanlineSource{layer: 10, w: 1, h: 1, frame: solidTestFrame(1, 1, 0, 0, 0x30, 0x80)}
	vga.enabled.Store(true)
	comp.RegisterSource(vga)
	comp.RegisterSource(opaque)
	comp.RegisterSource(chip)

	comp.composite()

	if got := testPixel(comp.finalFrame, 0, 0, 1); got != [4]byte{0, 0, 0x30, 0x80} {
		t.Fatalf("mixed layer pixel = %v", got)
	}
	if chip.scanlines != 1 || vga.scanlines != 1 {
		t.Fatalf("scanline calls chip=%d vga=%d", chip.scanlines, vga.scanlines)
	}
}

func TestCompositor_MixedScanline_OpaqueBelowTransparentShowsThrough(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(2, 1)
	bottom := &mockOpaqueSource{layer: -5, w: 2, h: 1, frame: solidTestFrame(2, 1, 7, 8, 9, 0xFF)}
	bottom.enabled.Store(true)
	scan := solidTestFrame(2, 1, 1, 2, 3, 0xFF)
	setTestPixel(scan, 1, 0, 2, 0, 0, 0, 0)
	chip := &mockScanlineSource{layer: 0, w: 2, h: 1, frame: scan}
	chip.enabled.Store(true)
	comp.RegisterSource(chip)
	comp.RegisterSource(bottom)

	comp.composite()

	if got := testPixel(comp.finalFrame, 0, 0, 2); got != [4]byte{1, 2, 3, 0xFF} {
		t.Fatalf("opaque scanline pixel = %v", got)
	}
	if got := testPixel(comp.finalFrame, 1, 0, 2); got != [4]byte{7, 8, 9, 0xFF} {
		t.Fatalf("transparent scanline pixel = %v", got)
	}
}

func TestCompositor_PanicInProcessScanline_ReleasesManaged(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(1, 2)
	src := &managedScanlineSource{panicY: 1}
	src.layer, src.w, src.h = 0, 1, 2
	src.frame = solidTestFrame(1, 2, 1, 2, 3, 0xFF)
	src.enabled.Store(true)
	comp.RegisterSource(src)

	comp.composite()

	if src.managedFalse.Load() == 0 {
		t.Fatal("managed source was not released after panic")
	}
}

func TestCompositor_ScanlineProcess_PastSourceHeight(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(1, 3)
	small := &managedScanlineSource{panicY: -1}
	small.layer, small.w, small.h = 0, 1, 1
	small.frame = solidTestFrame(1, 1, 1, 2, 3, 0xFF)
	small.enabled.Store(true)
	tall := &mockScanlineSource{layer: 1, w: 1, h: 3, frame: solidTestFrame(1, 3, 4, 5, 6, 0xFF)}
	tall.enabled.Store(true)
	comp.RegisterSource(small)
	comp.RegisterSource(tall)

	comp.composite()

	if got, want := small.ys, []int{0, 0, 0}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("small source y calls = %v, want %v", got, want)
	}
}

func TestCompositor_AllDisabled_PushesClearedFrameOnce(t *testing.T) {
	out := newMockVideoOutput()
	_ = out.Start()
	comp := NewVideoCompositor(out)
	comp.SetDimensions(1, 1)
	src := &mockOpaqueSource{layer: 0, w: 1, h: 1, frame: solidTestFrame(1, 1, 9, 9, 9, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	comp.composite()
	src.enabled.Store(false)
	comp.composite()
	comp.composite()

	if out.updateCalls != 2 {
		t.Fatalf("update calls = %d, want 2", out.updateCalls)
	}
	if got := testPixel(out.lastFrame, 0, 0, 1); got != [4]byte{} {
		t.Fatalf("cleared frame pixel = %v", got)
	}
}

func TestCompositor_SignalVSync_FiresEvenOnNilFrame(t *testing.T) {
	comp := NewVideoCompositor(nil)
	src := &mockOpaqueSource{layer: 0, w: 1, h: 1}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	comp.composite()
	if src.vsyncs.Load() != 1 {
		t.Fatalf("vsyncs = %d, want 1", src.vsyncs.Load())
	}
}

func TestCompositor_FrameCallback_ExactlyOncePerComposite(t *testing.T) {
	comp := NewVideoCompositor(nil)
	var calls atomic.Int32
	comp.SetFrameCallback(func() { calls.Add(1) })
	comp.composite()
	comp.composite()
	if calls.Load() != 2 {
		t.Fatalf("callback calls = %d, want 2", calls.Load())
	}
}

func TestCompositor_OutputUpdate_ReentrantNoDeadlock(t *testing.T) {
	out := newMockVideoOutput()
	_ = out.Start()
	comp := NewVideoCompositor(out)
	comp.SetDimensions(1, 1)
	src := &mockOpaqueSource{layer: 0, w: 1, h: 1, frame: solidTestFrame(1, 1, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	done := make(chan struct{})
	out.updateCallback = func() {
		_ = comp.GetCurrentFrame()
		close(done)
	}
	comp.composite()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reentrant UpdateFrame callback deadlocked")
	}
}

func TestCompositor_SetDisplayConfig_ReentrantNoDeadlock(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	done := make(chan struct{})
	out.setCallback = func() {
		_ = comp.GetCurrentFrame()
		close(done)
	}
	comp.SetDimensions(2, 2)
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reentrant SetDisplayConfig callback deadlocked")
	}
}

func TestCompositor_SetDimensions_HonorsLock(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(3, 2)
	comp.SetDimensions(1, 1)
	if w, h := comp.GetDimensions(); w != 3 || h != 2 {
		t.Fatalf("dimensions = %dx%d, want 3x2", w, h)
	}
}

func TestCompositor_AlphaMask_OpaqueWins_ZeroAlphaColorPromoted(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(2, 1)
	bottom := &mockOpaqueSource{layer: 0, w: 2, h: 1, frame: solidTestFrame(2, 1, 1, 2, 3, 0xFF)}
	bottom.enabled.Store(true)
	topFrame := solidTestFrame(2, 1, 9, 8, 7, 0xFF)
	setTestPixel(topFrame, 1, 0, 2, 4, 5, 6, 0)
	top := &mockOpaqueSource{layer: 1, w: 2, h: 1, frame: topFrame}
	top.enabled.Store(true)
	comp.RegisterSource(bottom)
	comp.RegisterSource(top)
	comp.composite()
	if got := testPixel(comp.finalFrame, 0, 0, 2); got != [4]byte{9, 8, 7, 0xFF} {
		t.Fatalf("opaque pixel = %v", got)
	}
	if got := testPixel(comp.finalFrame, 1, 0, 2); got != [4]byte{4, 5, 6, 0xFF} {
		t.Fatalf("zero-alpha color pixel = %v", got)
	}
}

func TestCompositor_AlphaMask_PartialAlphaTreatedAsOpaque(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(1, 1)
	bottom := &mockOpaqueSource{layer: 0, w: 1, h: 1, frame: solidTestFrame(1, 1, 1, 2, 3, 0xFF)}
	bottom.enabled.Store(true)
	top := &mockOpaqueSource{layer: 1, w: 1, h: 1, frame: solidTestFrame(1, 1, 9, 8, 7, 0x01)}
	top.enabled.Store(true)
	comp.RegisterSource(bottom)
	comp.RegisterSource(top)
	comp.composite()
	if got := testPixel(comp.finalFrame, 0, 0, 1); got != [4]byte{9, 8, 7, 0x01} {
		t.Fatalf("partial alpha pixel = %v", got)
	}
}

func TestCompositor_Close_ReleasesResources(t *testing.T) {
	comp := NewVideoCompositor(nil)
	src := &mockOpaqueSource{}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	if err := comp.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := comp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(comp.sources) != 0 {
		t.Fatalf("sources len after close = %d", len(comp.sources))
	}
	if err := comp.Start(); err == nil {
		t.Fatal("Start after Close unexpectedly succeeded")
	}
}

func TestCompositor_PanickingSource_DoesNotKillLoop(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(1, 1)
	bad := &mockOpaqueSource{layer: 0, w: 1, h: 1, panicOn: "GetFrame"}
	bad.enabled.Store(true)
	good := &mockOpaqueSource{layer: 1, w: 1, h: 1, frame: solidTestFrame(1, 1, 7, 8, 9, 0xFF)}
	good.enabled.Store(true)
	comp.RegisterSource(bad)
	comp.RegisterSource(good)
	comp.composite()
	if got := testPixel(comp.finalFrame, 0, 0, 1); got != [4]byte{7, 8, 9, 0xFF} {
		t.Fatalf("good source did not compose after panic: %v", got)
	}
}

func TestCompositor_StopStart_RaceFree(t *testing.T) {
	comp := NewVideoCompositor(nil)
	for range 20 {
		if err := comp.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		comp.Stop()
	}
}

func TestCompositor_StopWaitsWhenAlreadyStopping(t *testing.T) {
	comp := NewVideoCompositor(nil)
	loopDone := make(chan struct{})
	comp.mu.Lock()
	comp.state = compositorStopping
	comp.loopDone = loopDone
	comp.mu.Unlock()

	stopped := make(chan struct{})
	go func() {
		comp.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned before in-progress stop completed")
	case <-time.After(25 * time.Millisecond):
	}

	close(loopDone)
	select {
	case <-stopped:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Stop did not return after loopDone closed")
	}
}

func TestCompositor_GetFrameSnapshot_IncludesFrameCounter(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.composite()
	_, n1, ts1 := comp.GetFrameSnapshot()
	comp.composite()
	_, n2, ts2 := comp.GetFrameSnapshot()
	if n2 <= n1 {
		t.Fatalf("frame counter did not increase: %d -> %d", n1, n2)
	}
	if !ts2.After(ts1) && !ts2.Equal(ts1) {
		t.Fatalf("timestamp regressed: %v -> %v", ts1, ts2)
	}
}

func TestCompositor_TickRate_Is60(t *testing.T) {
	comp := NewVideoCompositor(nil)
	if got := comp.GetTickRate(); got != 60 {
		t.Fatalf("tick rate = %d, want 60", got)
	}
}

func TestCompositor_OutputRate_FollowsBackend(t *testing.T) {
	out := newMockVideoOutput()
	out.refreshRate = 75
	comp := NewVideoCompositor(out)
	if got := comp.GetRefreshRate(); got != 75 {
		t.Fatalf("refresh rate = %d, want 75", got)
	}
}

func TestCompositor_ScanlineSelectorFalseUsesFullFrame(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(2, 1)
	source := &selectableScanlineSource{
		mockScanlineSource: mockScanlineSource{
			layer: 0,
			w:     2,
			h:     1,
			frame: solidTestFrame(2, 1, 1, 2, 3, 255),
		},
		needs: false,
	}
	source.enabled.Store(true)
	comp.RegisterSource(source)

	comp.composite()

	if source.scanlines != 0 {
		t.Fatalf("scanline calls = %d, want 0", source.scanlines)
	}
	if source.vsyncs.Load() != 1 {
		t.Fatalf("vsync calls = %d, want 1", source.vsyncs.Load())
	}
	if got := testPixel(comp.finalFrame, 0, 0, 2); got != [4]byte{1, 2, 3, 255} {
		t.Fatalf("full-frame pixel = %v", got)
	}
}

func TestCompositor_ScanlineSelectorTrueUsesScanlinePath(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(2, 3)
	source := &selectableScanlineSource{
		mockScanlineSource: mockScanlineSource{
			layer: 0,
			w:     2,
			h:     3,
			frame: solidTestFrame(2, 3, 4, 5, 6, 255),
		},
		needs: true,
	}
	source.enabled.Store(true)
	comp.RegisterSource(source)

	comp.composite()

	if source.scanlines != 3 {
		t.Fatalf("scanline calls = %d, want 3", source.scanlines)
	}
}

func TestCompositor_LegacyScanlineSourceStillUsesScanlinePath(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(2, 3)
	source := &mockScanlineSource{
		layer: 0,
		w:     2,
		h:     3,
		frame: solidTestFrame(2, 3, 7, 8, 9, 255),
	}
	source.enabled.Store(true)
	comp.RegisterSource(source)

	comp.composite()

	if source.scanlines != 3 {
		t.Fatalf("scanline calls = %d, want 3", source.scanlines)
	}
}

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

func TestCompositor_EnabledVoodooScalesIntoLockedVideoChipFrame(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int
	}{
		{name: "640x480", w: 640, h: 480},
		{name: "800x600", w: 800, h: 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := NewVideoCompositor(nil)
			comp.LockResolution(1920, 1080)

			chip := &mockScanlineSource{
				layer: 0,
				w:     1920,
				h:     1080,
				frame: solidTestFrame(1920, 1080, 0x10, 0x20, 0x30, 0xFF),
			}
			chip.enabled.Store(true)
			voodoo := &mockOpaqueSource{
				layer: 20,
				w:     tc.w,
				h:     tc.h,
				frame: solidTestFrame(tc.w, tc.h, 0xA0, 0x40, 0x10, 0xFF),
			}
			voodoo.enabled.Store(true)

			comp.RegisterSource(chip)
			comp.RegisterSource(voodoo)
			comp.composite()

			if gotW, gotH := comp.GetDimensions(); gotW != 1920 || gotH != 1080 {
				t.Fatalf("compositor dimensions = %dx%d, want 1920x1080", gotW, gotH)
			}
			if got := testPixel(comp.finalFrame, 960, 540, 1920); got != [4]byte{0xA0, 0x40, 0x10, 0xFF} {
				t.Fatalf("scaled Voodoo pixel = %v, want [160 64 16 255]", got)
			}
		})
	}
}

func TestCompositor_NativeSourceDimensionsPreferVideoChipBelowVoodoo(t *testing.T) {
	comp := NewVideoCompositor(nil)
	chip := &mockScanlineSource{layer: 0, w: 1920, h: 1080, frame: solidTestFrame(1920, 1080, 1, 2, 3, 255)}
	chip.enabled.Store(true)
	voodoo := &mockOpaqueSource{layer: 20, w: 640, h: 480, frame: solidTestFrame(640, 480, 4, 5, 6, 255)}
	voodoo.enabled.Store(true)

	comp.RegisterSource(voodoo)
	comp.RegisterSource(chip)

	if gotW, gotH := comp.GetNativeSourceDimensions(); gotW != 1920 || gotH != 1080 {
		t.Fatalf("native dimensions = %dx%d, want VideoChip 1920x1080", gotW, gotH)
	}
}

type mockVideoOutput struct {
	mu             sync.Mutex
	started        bool
	config         DisplayConfig
	setCalls       int
	updateCalls    int
	lastFrame      []byte
	updateErr      error
	setErr         error
	updateCallback func()
	setCallback    func()
	refreshRate    int
}

type mockHardwareVideoOutput struct {
	*mockVideoOutput
	hwUpdates []CompositorFrameUpdate
	hwErr     error
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
	m.setCalls++
	m.config = config
	err := m.setErr
	cb := m.setCallback
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
	return err
}

func (m *mockVideoOutput) GetDisplayConfig() DisplayConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

func (m *mockVideoOutput) UpdateFrame(buffer []byte) error {
	m.mu.Lock()
	m.updateCalls++
	m.lastFrame = append(m.lastFrame[:0], buffer...)
	err := m.updateErr
	cb := m.updateCallback
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
	return err
}
func (m *mockVideoOutput) WaitForVSync() error   { return nil }
func (m *mockVideoOutput) GetFrameCount() uint64 { return 0 }
func (m *mockVideoOutput) GetRefreshRate() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.refreshRate != 0 {
		return m.refreshRate
	}
	return 60
}

func newMockHardwareVideoOutput() *mockHardwareVideoOutput {
	out := &mockHardwareVideoOutput{mockVideoOutput: newMockVideoOutput()}
	_ = out.Start()
	return out
}

func (m *mockHardwareVideoOutput) UpdateHardwareCompositorFrame(update CompositorFrameUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hwErr != nil {
		return m.hwErr
	}
	cloned := update
	cloned.Layers = cloneCompositorLayers(update.Layers)
	m.hwUpdates = append(m.hwUpdates, cloned)
	return nil
}

func (m *mockHardwareVideoOutput) HardwareCompositorSnapshot(frameID uint64) ([]byte, bool) {
	return nil, false
}

func (m *mockHardwareVideoOutput) hardwareUpdateCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.hwUpdates)
}

func (m *mockHardwareVideoOutput) lastHardwareUpdate() CompositorFrameUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hwUpdates[len(m.hwUpdates)-1]
}

func TestCompositor_SetDimensions_UpdatesFrameSize(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.SetDimensions(1024, 768)
	if comp.frameWidth != 1024 || comp.frameHeight != 768 {
		t.Fatalf("expected 1024x768, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	if len(comp.finalFrame) != 1024*768*4 {
		t.Fatalf("expected finalFrame len %d, got %d", 1024*768*4, len(comp.finalFrame))
	}
}

func TestCompositor_NotifyResolutionChange_AppliesOnComposite(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.NotifyResolutionChange(1024, 768)
	if comp.frameWidth != DefaultPresentationWidth {
		t.Fatalf("expected width unchanged before composite, got %d", comp.frameWidth)
	}
	comp.composite()
	if comp.frameWidth != 1024 || comp.frameHeight != 768 {
		t.Fatalf("expected 1024x768, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	cfg := out.GetDisplayConfig()
	if cfg.Width != 1024 || cfg.Height != 768 {
		t.Fatalf("expected output config 1024x768, got %dx%d", cfg.Width, cfg.Height)
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

func TestCompositor_UnlockResolution_AppliesPendingNotification(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(DefaultPresentationWidth, DefaultPresentationHeight)
	comp.NotifyResolutionChange(640, 480)
	comp.composite()
	if comp.frameWidth != DefaultPresentationWidth || comp.frameHeight != DefaultPresentationHeight {
		t.Fatalf("expected locked default resolution, got %dx%d", comp.frameWidth, comp.frameHeight)
	}

	comp.UnlockResolution()
	comp.composite()
	if comp.frameWidth != 640 || comp.frameHeight != 480 {
		t.Fatalf("expected pending 640x480 after unlock, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	cfg := out.GetDisplayConfig()
	if cfg.Width != 640 || cfg.Height != 480 {
		t.Fatalf("expected output config 640x480, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestCompositor_LockResolution_PropagatesConfig_Started(t *testing.T) {
	out := newMockVideoOutput()
	_ = out.Start()
	comp := NewVideoCompositor(out)
	comp.LockResolution(1024, 768)
	cfg := out.GetDisplayConfig()
	if cfg.Width != 1024 || cfg.Height != 768 {
		t.Fatalf("expected output config 1024x768, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestCompositor_LockResolution_PropagatesConfig_PreStart(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(1024, 768)
	cfg := out.GetDisplayConfig()
	if cfg.Width != 1024 || cfg.Height != 768 {
		t.Fatalf("expected output config 1024x768, got %dx%d", cfg.Width, cfg.Height)
	}
	if comp.frameWidth != 1024 || comp.frameHeight != 768 {
		t.Fatalf("expected compositor 1024x768, got %dx%d", comp.frameWidth, comp.frameHeight)
	}
	_ = out.Start()
	comp.composite()
	if len(comp.finalFrame) != 1024*768*4 {
		t.Fatalf("expected finalFrame len %d, got %d", 1024*768*4, len(comp.finalFrame))
	}
}

func TestCompositor_ApplyResolution_NoDuplicateUpdate(t *testing.T) {
	out := newMockVideoOutput()
	comp := NewVideoCompositor(out)
	comp.NotifyResolutionChange(DefaultPresentationWidth, DefaultPresentationHeight)
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

func TestCompositor_HardwarePath_Default960To1080AvoidsSoftwareScale(t *testing.T) {
	t.Setenv("IE_DISABLE_GPU_COMPOSITOR", "")
	out := newMockHardwareVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{
		layer: 0,
		w:     960,
		h:     540,
		frame: solidTestFrame(960, 540, 0x11, 0x22, 0x33, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	var softwareScaleCalls atomic.Int32
	oldScaleHook := compositorSoftwareScaleHook
	oldPresentationHook := compositorSoftwarePresentationHook
	compositorSoftwareScaleHook = func() { softwareScaleCalls.Add(1) }
	compositorSoftwarePresentationHook = func() { t.Fatal("software presentation path used on hardware happy path") }
	defer func() {
		compositorSoftwareScaleHook = oldScaleHook
		compositorSoftwarePresentationHook = oldPresentationHook
	}()

	comp.composite()

	if got := out.hardwareUpdateCount(); got != 1 {
		t.Fatalf("hardware updates = %d, want 1", got)
	}
	if out.updateCalls != 0 {
		t.Fatalf("software UpdateFrame calls = %d, want 0", out.updateCalls)
	}
	if got := softwareScaleCalls.Load(); got != 0 {
		t.Fatalf("software scale calls = %d, want 0", got)
	}
	update := out.lastHardwareUpdate()
	if update.PresentationWidth != 1920 || update.PresentationHeight != 1080 || !update.HasContent {
		t.Fatalf("update metadata = %dx%d content=%v", update.PresentationWidth, update.PresentationHeight, update.HasContent)
	}
	if len(update.Layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(update.Layers))
	}
	layer := update.Layers[0]
	if layer.SourceWidth != 960 || layer.SourceHeight != 540 || layer.DestX != 0 || layer.DestY != 0 || layer.DestWidth != 1920 || layer.DestHeight != 1080 {
		t.Fatalf("layer = src %dx%d dst (%d,%d) %dx%d", layer.SourceWidth, layer.SourceHeight, layer.DestX, layer.DestY, layer.DestWidth, layer.DestHeight)
	}
}

func TestCompositor_HardwarePath_AspectFitRect(t *testing.T) {
	t.Setenv("IE_DISABLE_GPU_COMPOSITOR", "")
	out := newMockHardwareVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(1920, 1080)
	comp.SetScaleMode(ScaleAspectFit)
	src := &mockOpaqueSource{
		layer: 0,
		w:     640,
		h:     480,
		frame: solidTestFrame(640, 480, 0x20, 0x40, 0x60, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	comp.composite()

	layer := out.lastHardwareUpdate().Layers[0]
	if layer.DestX != 240 || layer.DestY != 0 || layer.DestWidth != 1440 || layer.DestHeight != 1080 {
		t.Fatalf("aspect-fit rect = (%d,%d) %dx%d, want (240,0) 1440x1080", layer.DestX, layer.DestY, layer.DestWidth, layer.DestHeight)
	}
}

func TestCompositor_HardwareFailureFallsBackAndDisablesHardware(t *testing.T) {
	t.Setenv("IE_DISABLE_GPU_COMPOSITOR", "")
	out := newMockHardwareVideoOutput()
	out.hwErr = errors.New("gpu failed")
	comp := NewVideoCompositor(out)
	comp.LockResolution(4, 2)
	src := &mockOpaqueSource{layer: 0, w: 2, h: 1, frame: solidTestFrame(2, 1, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	comp.composite()
	if got := out.hardwareUpdateCount(); got != 0 {
		t.Fatalf("successful hardware updates = %d, want 0", got)
	}
	if out.updateCalls != 1 {
		t.Fatalf("software updates after failure = %d, want 1", out.updateCalls)
	}
	out.hwErr = nil
	comp.composite()
	if got := out.hardwareUpdateCount(); got != 0 {
		t.Fatalf("hardware retried after sticky disable: %d", got)
	}
	if out.updateCalls != 2 {
		t.Fatalf("software updates after sticky disable = %d, want 2", out.updateCalls)
	}
}

func TestCompositor_HardwareSnapshotLazyCache(t *testing.T) {
	t.Setenv("IE_DISABLE_GPU_COMPOSITOR", "")
	out := newMockHardwareVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(4, 2)
	src := &mockOpaqueSource{layer: 0, w: 2, h: 1, frame: solidTestFrame(2, 1, 9, 8, 7, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	comp.composite()

	var softwarePresentationCalls atomic.Int32
	oldHook := compositorSoftwarePresentationHook
	compositorSoftwarePresentationHook = func() { softwarePresentationCalls.Add(1) }
	defer func() { compositorSoftwarePresentationHook = oldHook }()

	first := comp.GetCurrentFrame()
	second := comp.GetCurrentFrame()

	if got := softwarePresentationCalls.Load(); got != 1 {
		t.Fatalf("snapshot software renders = %d, want 1", got)
	}
	if len(first) != 4*2*4 || len(second) != len(first) {
		t.Fatalf("snapshot sizes first=%d second=%d", len(first), len(second))
	}
	if got := testPixel(first, 3, 1, 4); got != [4]byte{9, 8, 7, 0xFF} {
		t.Fatalf("snapshot bottom-right = %v", got)
	}
}

func TestDisplayConfig_FullscreenDefaultFalse(t *testing.T) {
	var config DisplayConfig
	if config.Fullscreen {
		t.Fatal("expected zero-value Fullscreen to be false")
	}
}

func TestDefaultVideoAndPresentationModes(t *testing.T) {
	mode := VideoModes[DEFAULT_VIDEO_MODE]
	if DEFAULT_VIDEO_MODE != MODE_960x540 {
		t.Fatalf("DEFAULT_VIDEO_MODE = 0x%X, want MODE_960x540", DEFAULT_VIDEO_MODE)
	}
	if mode.width != 960 || mode.height != 540 {
		t.Fatalf("default native mode = %dx%d, want 960x540", mode.width, mode.height)
	}
	if DefaultPresentationWidth != 1920 || DefaultPresentationHeight != 1080 {
		t.Fatalf("default presentation = %dx%d, want 1920x1080", DefaultPresentationWidth, DefaultPresentationHeight)
	}
}

func TestCompositor_AspectFit_BarsFourByThree(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	comp.SetScaleMode(ScaleAspectFit)
	src := &mockOpaqueSource{
		layer: 0,
		w:     640,
		h:     480,
		frame: solidTestFrame(640, 480, 0x20, 0x40, 0x60, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	comp.composite()

	if got := testPixel(comp.finalFrame, 239, 540, 1920); got != [4]byte{} {
		t.Fatalf("left bar pixel = %v, want transparent black", got)
	}
	if got := testPixel(comp.finalFrame, 240, 540, 1920); got != [4]byte{0x20, 0x40, 0x60, 0xFF} {
		t.Fatalf("first fit pixel = %v, want source color", got)
	}
}

func TestCompositor_StretchFill_FillsFourByThree(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	comp.SetScaleMode(ScaleStretchFill)
	src := &mockOpaqueSource{
		layer: 0,
		w:     640,
		h:     480,
		frame: solidTestFrame(640, 480, 0x20, 0x40, 0x60, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	comp.composite()

	if got := testPixel(comp.finalFrame, 0, 540, 1920); got != [4]byte{0x20, 0x40, 0x60, 0xFF} {
		t.Fatalf("stretched edge pixel = %v, want source color", got)
	}
}

func TestCompositor_960x540Fills1080p(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{
		layer: 0,
		w:     960,
		h:     540,
		frame: solidTestFrame(960, 540, 0x11, 0x22, 0x33, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	comp.composite()

	if got := testPixel(comp.finalFrame, 0, 0, 1920); got != [4]byte{0x11, 0x22, 0x33, 0xFF} {
		t.Fatalf("top-left pixel = %v, want source color", got)
	}
	if got := testPixel(comp.finalFrame, 1919, 1079, 1920); got != [4]byte{0x11, 0x22, 0x33, 0xFF} {
		t.Fatalf("bottom-right pixel = %v, want source color", got)
	}
}

func TestCompositor_ScaleToggleOnlyForNon16x9Sources(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{layer: 0, w: 960, h: 540, frame: solidTestFrame(960, 540, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	if comp.ActiveSourceNeedsScaleToggle() {
		t.Fatal("16:9 source should not need scale toggle")
	}
	if comp.ToggleScaleModeIfNonNative() {
		t.Fatal("16:9 source should not toggle scale mode")
	}
	if got := comp.GetScaleMode(); got != ScaleStretchFill {
		t.Fatalf("scale mode changed for 16:9 source: %v", got)
	}

	src.w, src.h = 640, 480
	src.frame = solidTestFrame(640, 480, 1, 2, 3, 0xFF)
	if !comp.ActiveSourceNeedsScaleToggle() {
		t.Fatal("4:3 source should need scale toggle")
	}
	if got := comp.GetScaleMode(); got != ScaleStretchFill {
		t.Fatalf("default scale mode = %v, want stretch fill", got)
	}
	if !comp.ToggleScaleModeIfNonNative() {
		t.Fatal("4:3 source should toggle scale mode")
	}
	if got := comp.GetScaleMode(); got != ScaleAspectFit {
		t.Fatalf("scale mode = %v, want aspect fit", got)
	}
}

func TestCompositor_MapPresentationPointToNative_AspectFitPillarbox(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	comp.SetScaleMode(ScaleAspectFit)
	src := &mockOpaqueSource{layer: 0, w: 640, h: 480, frame: solidTestFrame(640, 480, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	x, y, w, h := comp.MapPresentationPointToNative(240, 0)
	if x != 0 || y != 0 || w != 640 || h != 480 {
		t.Fatalf("visible top-left maps to (%d,%d) %dx%d, want (0,0) 640x480", x, y, w, h)
	}
	x, y, _, _ = comp.MapPresentationPointToNative(960, 540)
	if x != 320 || y != 240 {
		t.Fatalf("presentation center maps to (%d,%d), want (320,240)", x, y)
	}
	x, y, _, _ = comp.MapPresentationPointToNative(239, 540)
	if x != 0 || y != 240 {
		t.Fatalf("left pillarbox maps to (%d,%d), want clamped left edge (0,240)", x, y)
	}
	x, y, _, _ = comp.MapPresentationPointToNative(1680, 1079)
	if x != 639 || y != 479 {
		t.Fatalf("visible bottom-right maps to (%d,%d), want (639,479)", x, y)
	}
}

func TestCompositor_MapNativePointToPresentation_AspectFitPillarbox(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	comp.SetScaleMode(ScaleAspectFit)
	src := &mockOpaqueSource{layer: 0, w: 640, h: 480, frame: solidTestFrame(640, 480, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	x, y := comp.MapNativePointToPresentation(0, 0)
	if x != 240 || y != 0 {
		t.Fatalf("native top-left maps to (%d,%d), want presentation visible top-left (240,0)", x, y)
	}
	x, y = comp.MapNativePointToPresentation(320, 240)
	if x != 960 || y != 540 {
		t.Fatalf("native center maps to (%d,%d), want presentation center (960,540)", x, y)
	}
	x, y = comp.MapNativePointToPresentation(639, 479)
	if x != 1677 || y != 1077 {
		t.Fatalf("native bottom-right maps to (%d,%d), want last scaled pixel near (1677,1077)", x, y)
	}
}

func TestCompositor_MapNativePointToPresentation_960x540Doubles(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{layer: 0, w: 960, h: 540, frame: solidTestFrame(960, 540, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	x, y := comp.MapNativePointToPresentation(480, 270)
	if x != 960 || y != 540 {
		t.Fatalf("native 960x540 center maps to (%d,%d), want (960,540)", x, y)
	}
}

func TestCompositor_MapPresentationPointToNativeForSource_IgnoresActiveSource(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{layer: 0, w: 960, h: 540, frame: solidTestFrame(960, 540, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	x, y, w, h := comp.MapPresentationPointToNativeForSource(1919, 1079, 1920, 1080)
	if x != 1919 || y != 1079 || w != 1920 || h != 1080 {
		t.Fatalf("presentation bottom-right maps to (%d,%d) %dx%d, want (1919,1079) 1920x1080", x, y, w, h)
	}
}

func TestCompositor_MapNativePointToPresentationForSource_IgnoresActiveSource(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{layer: 0, w: 960, h: 540, frame: solidTestFrame(960, 540, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	x, y := comp.MapNativePointToPresentationForSource(960, 540, 1920, 1080)
	if x != 960 || y != 540 {
		t.Fatalf("native center maps to (%d,%d), want presentation center (960,540)", x, y)
	}
}

func TestCompositor_MapPresentationPointToNative_StretchFill(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	comp.SetScaleMode(ScaleStretchFill)
	src := &mockOpaqueSource{layer: 0, w: 640, h: 480, frame: solidTestFrame(640, 480, 1, 2, 3, 0xFF)}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	x, y, w, h := comp.MapPresentationPointToNative(960, 540)
	if x != 320 || y != 240 || w != 640 || h != 480 {
		t.Fatalf("stretch center maps to (%d,%d) %dx%d, want (320,240) 640x480", x, y, w, h)
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

func BenchmarkCompositorSoftwareScaled960x540To1080p(b *testing.B) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{
		layer: 0,
		w:     960,
		h:     540,
		frame: solidTestFrame(960, 540, 0x11, 0x22, 0x33, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	b.SetBytes(1920 * 1080 * BYTES_PER_PIXEL)
	b.ResetTimer()
	for range b.N {
		comp.composite()
	}
}

func BenchmarkCompositorHardwareLayerBuild960x540To1080p(b *testing.B) {
	b.Setenv("IE_DISABLE_GPU_COMPOSITOR", "")
	out := newMockHardwareVideoOutput()
	comp := NewVideoCompositor(out)
	comp.LockResolution(1920, 1080)
	src := &mockOpaqueSource{
		layer: 0,
		w:     960,
		h:     540,
		frame: solidTestFrame(960, 540, 0x11, 0x22, 0x33, 0xFF),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)
	b.SetBytes(960 * 540 * BYTES_PER_PIXEL)
	b.ResetTimer()
	for range b.N {
		comp.composite()
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

func TestVideoChip_Reset_FiresDefaultResolutionCallback(t *testing.T) {
	chip := newTestVideoChip(newMockVideoOutput())
	chip.HandleWrite(VIDEO_MODE, MODE_320x200)

	var gotW, gotH int
	chip.SetResolutionChangeCallback(func(w, h int) {
		gotW, gotH = w, h
	})
	chip.Reset()

	mode := VideoModes[DEFAULT_VIDEO_MODE]
	if gotW != mode.width || gotH != mode.height {
		t.Fatalf("expected reset callback %dx%d, got %dx%d", mode.width, mode.height, gotW, gotH)
	}
}

func TestVideoChip_Reset_WithCompositor_RestoresDefaultResolution(t *testing.T) {
	out := newMockVideoOutput()
	chip := newTestVideoChip(out)
	comp := NewVideoCompositor(out)
	chip.SetResolutionChangeCallback(func(w, h int) {
		comp.NotifyResolutionChange(w, h)
	})

	chip.HandleWrite(VIDEO_MODE, MODE_320x200)
	comp.composite()
	if comp.frameWidth != 320 || comp.frameHeight != 200 {
		t.Fatalf("expected compositor 320x200 before reset, got %dx%d", comp.frameWidth, comp.frameHeight)
	}

	chip.Reset()
	comp.composite()

	mode := VideoModes[DEFAULT_VIDEO_MODE]
	if comp.frameWidth != mode.width || comp.frameHeight != mode.height {
		t.Fatalf("expected compositor default %dx%d after reset, got %dx%d", mode.width, mode.height, comp.frameWidth, comp.frameHeight)
	}
	cfg := out.GetDisplayConfig()
	if cfg.Width != mode.width || cfg.Height != mode.height {
		t.Fatalf("expected output config default %dx%d after reset, got %dx%d", mode.width, mode.height, cfg.Width, cfg.Height)
	}
}

func TestVideoChip_Reset_NilCallback_RestoresOutputConfig(t *testing.T) {
	out := newMockVideoOutput()
	chip := newTestVideoChip(out)
	chip.HandleWrite(VIDEO_MODE, MODE_320x200)
	chip.SetResolutionChangeCallback(nil)

	chip.Reset()

	mode := VideoModes[DEFAULT_VIDEO_MODE]
	cfg := out.GetDisplayConfig()
	if cfg.Width != mode.width || cfg.Height != mode.height {
		t.Fatalf("expected output config default %dx%d after reset, got %dx%d", mode.width, mode.height, cfg.Width, cfg.Height)
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
