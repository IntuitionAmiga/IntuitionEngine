// video_compositor_skip_test.go - Slice 1: timing/pixel split on the composite skip

package main

import (
	"sync/atomic"
	"testing"
)

// genTickSource is a production-shaped source that tracks a frame generation
// (FrameGenerationSource) and counts TickFrame calls (FrameTicker). Holding the
// generation stable makes an unchanged tick skip-eligible.
type genTickSource struct {
	mockOpaqueSource
	gen   atomic.Uint64
	ticks atomic.Int32
}

func (s *genTickSource) FrameGeneration() uint64 { return s.gen.Load() }
func (s *genTickSource) TickFrame()              { s.ticks.Add(1) }

func newGenTickSource() *genTickSource {
	s := &genTickSource{
		mockOpaqueSource: mockOpaqueSource{
			layer: 0,
			w:     2,
			h:     2,
			frame: solidTestFrame(2, 2, 0xAA, 0xBB, 0xCC, 0xFF),
		},
	}
	s.enabled.Store(true)
	s.gen.Store(1)
	return s
}

// TestCompositeSkip_TimingFiresPixelsDoNot proves that on a skipped tick the
// unconditional timing callback fires and frame metadata advances, while the
// pixel path (UpdateFrame/UpdateRegion) and the pixel-consumer callback do not.
func TestCompositeSkip_TimingFiresPixelsDoNot(t *testing.T) {
	t.Setenv("IE_VIDEO_COMPOSITE_SKIP", "1")
	out := newMockVideoOutput()
	if err := out.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	comp := NewVideoCompositor(out)
	comp.frameWidth = 2
	comp.frameHeight = 2

	var timing, pixels atomic.Int32
	comp.SetFrameTimingCallback(func() { timing.Add(1) })
	comp.SetFrameCallback(func() { pixels.Add(1) })

	src := newGenTickSource()
	comp.RegisterSource(src)

	// Prime: the first composite (frameCounter 0) records nothing, the second
	// records the generation baseline. Both materialise. The skip becomes
	// eligible only once the generation has been observed unchanged.
	comp.composite()
	comp.composite()
	if timing.Load() != 2 || pixels.Load() != 2 {
		t.Fatalf("after priming timing=%d pixels=%d, want 2/2", timing.Load(), pixels.Load())
	}
	firstFrame := comp.frameCounter
	baseTicks := src.ticks.Load()
	out.updateCalls = 0
	out.regionCalls = 0

	// Generation unchanged: this tick must skip pixel work.
	comp.composite()

	if out.updateCalls != 0 || out.regionCalls != 0 {
		t.Fatalf("skip tick did pixel work: updateCalls=%d regionCalls=%d, want 0/0", out.updateCalls, out.regionCalls)
	}
	if comp.frameCounter != firstFrame+1 {
		t.Fatalf("frameCounter = %d, want %d (logical frame must advance on skip)", comp.frameCounter, firstFrame+1)
	}
	if timing.Load() != 3 {
		t.Fatalf("timing callback count = %d, want 3 (fires on skip)", timing.Load())
	}
	if pixels.Load() != 2 {
		t.Fatalf("pixel callback count = %d, want 2 (must NOT fire on skip)", pixels.Load())
	}
	if src.ticks.Load() != baseTicks+1 {
		t.Fatalf("TickFrame calls = %d, want %d (VBlank edges still run on skip)", src.ticks.Load(), baseTicks+1)
	}
}

// TestCompositeSkip_KillSwitchForcesComposite proves IE_VIDEO_COMPOSITE_SKIP=0
// disables the skip so every unchanged tick still composites pixels.
func TestCompositeSkip_KillSwitchForcesComposite(t *testing.T) {
	t.Setenv("IE_VIDEO_COMPOSITE_SKIP", "0")
	out := newMockVideoOutput()
	if err := out.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	comp := NewVideoCompositor(out)
	comp.frameWidth = 2
	comp.frameHeight = 2
	comp.SetFrameTimingCallback(func() {})

	src := newGenTickSource()
	comp.RegisterSource(src)

	comp.composite()
	out.updateCalls = 0
	out.regionCalls = 0

	// Generation unchanged, but the kill switch forces a full composite.
	comp.composite()
	if out.updateCalls == 0 && out.regionCalls == 0 {
		t.Fatalf("IE_VIDEO_COMPOSITE_SKIP=0 still skipped: updateCalls=%d regionCalls=%d", out.updateCalls, out.regionCalls)
	}
}

// TestCompositeSkip_ActivePixelConsumerDisablesSkip proves that a live pixel
// consumer (recorder/capture) forces materialisation even with a stable
// generation, so recording never starves.
func TestCompositeSkip_ActivePixelConsumerDisablesSkip(t *testing.T) {
	t.Setenv("IE_VIDEO_COMPOSITE_SKIP", "1")
	out := newMockVideoOutput()
	if err := out.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	comp := NewVideoCompositor(out)
	comp.frameWidth = 2
	comp.frameHeight = 2
	comp.SetFrameTimingCallback(func() {})

	var pixels atomic.Int32
	comp.SetFrameCallback(func() { pixels.Add(1) })
	var consuming atomic.Bool
	comp.SetPixelConsumerActiveFunc(func() bool { return consuming.Load() })

	src := newGenTickSource()
	comp.RegisterSource(src)

	comp.composite()
	out.updateCalls = 0
	out.regionCalls = 0
	basePixels := pixels.Load()

	// Consumer active: unchanged generation must NOT skip.
	consuming.Store(true)
	comp.composite()
	if out.updateCalls == 0 && out.regionCalls == 0 {
		t.Fatalf("active pixel consumer was skipped: updateCalls=%d regionCalls=%d", out.updateCalls, out.regionCalls)
	}
	if pixels.Load() != basePixels+1 {
		t.Fatalf("pixel callback count = %d, want %d while consuming", pixels.Load(), basePixels+1)
	}
}
