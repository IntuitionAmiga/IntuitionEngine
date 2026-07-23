// video_compositor_upload_test.go - upload planning and ownership tests.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"sync"
	"testing"
)

func TestUploadPlanner_OnlyDirtyRegions(t *testing.T) {
	t.Setenv("IE_VIDEO_PARTIAL_UPLOAD", "1")
	var p uploadPlanner

	regions := []FrameDirtyRect{
		{X: 8, Y: 8, Width: 16, Height: 16},
		{X: 100, Y: 40, Width: 8, Height: 8},
	}
	got := p.plan(320, 240, regions)
	if len(got) != 2 {
		t.Fatalf("planned %d regions, want 2", len(got))
	}
	for i := range got {
		if got[i] != regions[i] {
			t.Fatalf("region %d = %+v, want %+v", i, got[i], regions[i])
		}
	}

	// Pixels come from exactly the planned rectangle, packed tightly.
	frame := make([]byte, 320*240*BYTES_PER_PIXEL)
	for i := range frame {
		frame[i] = byte(i)
	}
	pixels := p.regionPixels(frame, got[1])
	if len(pixels) != 8*8*BYTES_PER_PIXEL {
		t.Fatalf("packed %d bytes, want %d", len(pixels), 8*8*BYTES_PER_PIXEL)
	}
	for y := range 8 {
		src := ((40+y)*320 + 100) * BYTES_PER_PIXEL
		row := pixels[y*8*BYTES_PER_PIXEL : (y+1)*8*BYTES_PER_PIXEL]
		if !bytes.Equal(row, frame[src:src+8*BYTES_PER_PIXEL]) {
			t.Fatalf("packed row %d does not match the frame", y)
		}
	}
}

func TestUploadPlanner_ClipsAndDropsRegions(t *testing.T) {
	t.Setenv("IE_VIDEO_PARTIAL_UPLOAD", "1")
	var p uploadPlanner
	p.plan(64, 64, []FrameDirtyRect{{X: 0, Y: 0, Width: 1, Height: 1}})

	got := p.plan(64, 64, []FrameDirtyRect{
		{X: -4, Y: -4, Width: 8, Height: 8},   // clipped to 4x4 at origin
		{X: 62, Y: 62, Width: 16, Height: 16}, // clipped to 2x2
		{X: 100, Y: 100, Width: 4, Height: 4}, // wholly outside, dropped
		{X: 0, Y: 0, Width: 2, Height: 2},     // inside the first, absorbed
	})
	want := []FrameDirtyRect{
		{X: 0, Y: 0, Width: 4, Height: 4},
		{X: 62, Y: 62, Width: 2, Height: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("planned %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("region %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestUploadPlanner_LargeCoverageFallsBackToWholeFrame(t *testing.T) {
	t.Setenv("IE_VIDEO_PARTIAL_UPLOAD", "1")
	var p uploadPlanner
	p.plan(64, 64, []FrameDirtyRect{{X: 0, Y: 0, Width: 1, Height: 1}})

	// 61% of the frame, over the limit.
	if got := p.plan(64, 64, []FrameDirtyRect{{X: 0, Y: 0, Width: 64, Height: 40}}); got != nil {
		t.Fatalf("planned %+v for a 62%% region, want a whole-frame upload", got)
	}
	// Just under it.
	if got := p.plan(64, 64, []FrameDirtyRect{{X: 0, Y: 0, Width: 64, Height: 32}}); len(got) != 1 {
		t.Fatalf("planned %+v for a 50%% region, want one region", got)
	}
}

func TestUploadPlanner_ResizeInvalidatesRetainedTexture(t *testing.T) {
	t.Setenv("IE_VIDEO_PARTIAL_UPLOAD", "1")
	var p uploadPlanner
	region := []FrameDirtyRect{{X: 0, Y: 0, Width: 4, Height: 4}}

	p.plan(64, 64, region)
	if got := p.plan(64, 64, region); len(got) != 1 {
		t.Fatalf("planned %+v at a stable size, want one region", got)
	}
	if got := p.plan(128, 64, region); got != nil {
		t.Fatalf("planned %+v straight after a resize, want a whole-frame upload", got)
	}
	if got := p.plan(128, 64, region); len(got) != 1 {
		t.Fatalf("planned %+v after the resize settled, want one region", got)
	}
}

func TestUploadPlanner_KillSwitchForcesWholeFrame(t *testing.T) {
	t.Setenv("IE_VIDEO_PARTIAL_UPLOAD", "0")
	var p uploadPlanner
	region := []FrameDirtyRect{{X: 0, Y: 0, Width: 4, Height: 4}}
	p.plan(64, 64, region)
	if got := p.plan(64, 64, region); got != nil {
		t.Fatalf("planned %+v with the kill switch set, want a whole-frame upload", got)
	}
}

func TestUploadPlanner_ZeroAllocsSteadyState(t *testing.T) {
	t.Setenv("IE_VIDEO_PARTIAL_UPLOAD", "1")
	var p uploadPlanner
	frame := make([]byte, 320*240*BYTES_PER_PIXEL)
	regions := []FrameDirtyRect{
		{X: 8, Y: 8, Width: 16, Height: 16},
		{X: 64, Y: 64, Width: 32, Height: 32},
	}
	// Warm the scratch buffers before measuring.
	for range 4 {
		for _, r := range p.plan(320, 240, regions) {
			p.regionPixels(frame, r)
		}
	}
	allocs := testing.AllocsPerRun(64, func() {
		for _, r := range p.plan(320, 240, regions) {
			p.regionPixels(frame, r)
		}
	})
	if allocs != 0 {
		t.Fatalf("steady-state upload planning allocated %.1f times per frame, want 0", allocs)
	}
}

func TestBlendFrame_ZeroAllocsSteadyState(t *testing.T) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(320, 240)
	comp.finalFrame = make([]byte, 320*240*BYTES_PER_PIXEL)

	// A scaled, blended layer exercises the resample plan cache and the row
	// scratch, which are the two per-frame allocations this path used to make.
	src := solidTestFrame(160, 120, 0x10, 0x20, 0x30, 0x00)
	layer := CompositorFrameLayer{
		SourceWidth:  160,
		SourceHeight: 120,
		DestX:        0,
		DestY:        0,
		DestWidth:    320,
		DestHeight:   240,
		Buffer:       src,
	}
	comp.blendLayer(layer)
	opaque := layer
	opaque.Opaque = true
	comp.blendLayer(opaque)

	allocs := testing.AllocsPerRun(32, func() {
		comp.blendLayer(layer)
		comp.blendLayer(opaque)
	})
	if allocs != 0 {
		t.Fatalf("steady-state scaled blend allocated %.1f times per frame, want 0", allocs)
	}
}

func TestFrameLease_DirtyMetadataPropagated(t *testing.T) {
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")
	out := newMockVideoOutput()
	if err := out.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	comp := NewVideoCompositor(out)
	comp.LockResolution(128, 128)
	src := newTileDirtySource(0, 128, 128, true, solidTestFrame(128, 128, 1, 2, 3, 0xFF))
	comp.RegisterSource(src)
	comp.composite()

	out.mu.Lock()
	out.updateCalls = 0
	out.regionCalls = 0
	out.mu.Unlock()

	src.rects = []FrameDirtyRect{{X: 16, Y: 16, Width: 8, Height: 8}}
	comp.composite()

	out.mu.Lock()
	defer out.mu.Unlock()
	if out.regionCalls != 1 || out.updateCalls != 0 {
		t.Fatalf("regionCalls=%d updateCalls=%d, want 1/0", out.regionCalls, out.updateCalls)
	}
	if out.lastRegionRect != (FrameDirtyRect{X: 16, Y: 16, Width: 8, Height: 8}) {
		t.Fatalf("region rect = %+v", out.lastRegionRect)
	}
}

func TestFrameLease_SlotNotReusedUntilAllOwnersRelease(t *testing.T) {
	ring := NewVideoFrameLeaseRing(2, 16)
	first, ok := ring.Acquire()
	if !ok {
		t.Fatal("first acquire failed")
	}
	if !first.Retain() {
		t.Fatal("retain failed")
	}
	second, ok := ring.Acquire()
	if !ok {
		t.Fatal("second acquire failed")
	}
	if _, ok := ring.Acquire(); ok {
		t.Fatal("ring handed out a third slot from a depth of two")
	}
	// One release of a twice-owned lease must not free the slot.
	first.Release()
	if _, ok := ring.Acquire(); ok {
		t.Fatal("slot was reused while an owner still held it")
	}
	first.Release()
	third, ok := ring.Acquire()
	if !ok {
		t.Fatal("slot was not reused after the last owner released it")
	}
	if third.Slot() != first.Slot() {
		t.Fatalf("reused slot = %d, want %d", third.Slot(), first.Slot())
	}
	second.Release()
	third.Release()
}

func TestCopySourcePath_NoIntermediateCopy(t *testing.T) {
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")
	comp := NewVideoCompositor(nil)
	comp.LockResolution(64, 64)
	src := &recordingCopySource{w: 64, h: 64}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	layers, ok := comp.collectCompositeLayers(true)
	if !ok || len(layers) != 1 {
		t.Fatalf("collect produced %d layers, hasContent=%v", len(layers), ok)
	}
	defer releaseFrameLayerLeases(layers)

	if layers[0].Lease == nil {
		t.Fatal("copy source layer has no lease")
	}
	// The source was handed compositor-owned storage and the layer buffer is
	// that same storage, so nothing was copied in between.
	if &layers[0].Buffer[0] != &src.handed[0] {
		t.Fatal("layer buffer is not the storage the source wrote into")
	}
	if &layers[0].Lease.Pixels()[0] != &src.handed[0] {
		t.Fatal("the storage handed to the source is not the lease's own pixels")
	}
}

// recordingCopySource records the exact storage the compositor hands it.
type recordingCopySource struct {
	mockOpaqueSource
	w, h   int
	handed []byte
	mu     sync.Mutex
}

func (m *recordingCopySource) GetDimensions() (int, int) { return m.w, m.h }
func (m *recordingCopySource) GetFrame() []byte          { return nil }
func (m *recordingCopySource) CopyFrameForCompositor(dst []byte) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range dst {
		dst[i] = byte(i)
	}
	m.handed = dst
	return dst, true
}
