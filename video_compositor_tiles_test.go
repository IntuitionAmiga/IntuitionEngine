// video_compositor_tiles_test.go - tile-based retained composition tests.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"math/rand"
	"testing"
)

// tileDirtySource publishes an explicit dirty rectangle set every frame,
// including the empty set, which is what lets the compositor retain tiles.
type tileDirtySource struct {
	mockOpaqueSource
	rects  []FrameDirtyRect
	opaque bool
}

func (m *tileDirtySource) TakeDirtyRects() []FrameDirtyRect {
	rects := m.rects
	m.rects = []FrameDirtyRect{}
	if rects == nil {
		return []FrameDirtyRect{}
	}
	return rects
}

func (m *tileDirtySource) IsOpaqueFrame() bool { return m.opaque }

func newTileDirtySource(layer, w, h int, opaque bool, frame []byte) *tileDirtySource {
	src := &tileDirtySource{
		mockOpaqueSource: mockOpaqueSource{layer: layer, w: w, h: h, frame: frame},
		rects:            []FrameDirtyRect{{X: 0, Y: 0, Width: w, Height: h}},
		opaque:           opaque,
	}
	src.enabled.Store(true)
	return src
}

func TestCompositeTiles_UnchangedTilesRetained(t *testing.T) {
	t.Setenv("IE_VIDEO_TILE_COMPOSITE", "1")
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")

	comp := NewVideoCompositor(nil)
	comp.LockResolution(256, 128)
	src := newTileDirtySource(0, 256, 128, true, solidTestFrame(256, 128, 0x10, 0x20, 0x30, 0xFF))
	comp.RegisterSource(src)

	// Enough frames to age every lease slot past the initial full frames.
	for range 8 {
		comp.composite()
	}
	before := comp.TileCompositeStats()
	comp.composite()
	after := comp.TileCompositeStats()

	if after.Composed != before.Composed {
		t.Fatalf("static frame composed %d tiles, want 0", after.Composed-before.Composed)
	}
	if after.Retained == before.Retained {
		t.Fatal("static frame retained no tiles")
	}
}

func TestCompositeTiles_DirtySourceMarksOnlyCoveredTiles(t *testing.T) {
	t.Setenv("IE_VIDEO_TILE_COMPOSITE", "1")
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")

	const w, h = 256, 128
	comp := NewVideoCompositor(nil)
	comp.LockResolution(w, h)
	src := newTileDirtySource(0, w, h, true, solidTestFrame(w, h, 0x10, 0x20, 0x30, 0xFF))
	comp.RegisterSource(src)

	for range 8 {
		comp.composite()
	}

	// One rectangle wholly inside the first tile, repeated so that the union
	// over the frames since each slot was last written stays inside it too.
	gridW := (w + compositorTileSize - 1) / compositorTileSize
	gridH := (h + compositorTileSize - 1) / compositorTileSize
	total := uint64(gridW * gridH)
	for range 4 {
		src.rects = []FrameDirtyRect{{X: 1, Y: 1, Width: 8, Height: 8}}
		comp.composite()
	}
	before := comp.TileCompositeStats()
	src.rects = []FrameDirtyRect{{X: 1, Y: 1, Width: 8, Height: 8}}
	comp.composite()
	after := comp.TileCompositeStats()

	composed := after.Composed - before.Composed
	if composed != 1 {
		t.Fatalf("small dirty rectangle composed %d tiles, want 1 of %d", composed, total)
	}
}

func TestCompositeTiles_ScanlineEffectForcesFullFrame(t *testing.T) {
	t.Setenv("IE_VIDEO_TILE_COMPOSITE", "1")
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")

	const w, h = 128, 128
	comp := NewVideoCompositor(nil)
	comp.LockResolution(w, h)
	src := &scanlineTileSource{
		tileDirtySource: *newTileDirtySource(0, w, h, true, solidTestFrame(w, h, 0x40, 0x50, 0x60, 0xFF)),
	}
	src.enabled.Store(true)
	comp.RegisterSource(src)

	for range 6 {
		comp.composite()
	}
	before := comp.TileCompositeStats()
	comp.composite()
	after := comp.TileCompositeStats()

	if after.FullFrames == before.FullFrames {
		t.Fatal("scanline-composited frame did not force a full frame")
	}
	if after.Retained != before.Retained {
		t.Fatalf("scanline-composited frame retained %d tiles, want 0", after.Retained-before.Retained)
	}
}

// scanlineTileSource is a dirty-rect source that also demands scanline
// compositing, which must defeat tile retention.
type scanlineTileSource struct {
	tileDirtySource
}

func (m *scanlineTileSource) StartFrame()                    {}
func (m *scanlineTileSource) ProcessScanline(y int)          {}
func (m *scanlineTileSource) FinishFrame() []byte            { return m.frame }
func (m *scanlineTileSource) NeedsScanlineCompositing() bool { return true }

// compositeTileScenario drives a randomised layer stack through a compositor
// and returns the sequence of output frames.
func compositeTileScenario(t *testing.T, seed int64, frames int, w, h int) [][]byte {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))

	comp := NewVideoCompositor(nil)
	comp.LockResolution(w, h)

	// A mix of 1:1 and scaled layers, opaque and blended, so every kernel in
	// the tile path is exercised.
	specs := []struct {
		w, h   int
		opaque bool
	}{
		{w, h, true},
		{w / 2, h / 2, false},
		{w, h, false},
	}
	sources := make([]*tileDirtySource, 0, len(specs))
	for i, spec := range specs {
		frame := make([]byte, spec.w*spec.h*BYTES_PER_PIXEL)
		for j := range frame {
			frame[j] = byte(rng.Intn(256))
		}
		src := newTileDirtySource(i, spec.w, spec.h, spec.opaque, frame)
		comp.RegisterSource(src)
		sources = append(sources, src)
	}

	out := make([][]byte, 0, frames)
	for f := range frames {
		for _, src := range sources {
			// Mutate a random patch and publish the matching dirty rect.
			rw := 1 + rng.Intn(src.w)
			rh := 1 + rng.Intn(src.h)
			rx := rng.Intn(src.w - rw + 1)
			ry := rng.Intn(src.h - rh + 1)
			for y := ry; y < ry+rh; y++ {
				base := (y*src.w + rx) * BYTES_PER_PIXEL
				for x := 0; x < rw*BYTES_PER_PIXEL; x++ {
					src.frame[base+x] = byte(rng.Intn(256))
				}
			}
			if f == 0 {
				src.rects = []FrameDirtyRect{{X: 0, Y: 0, Width: src.w, Height: src.h}}
			} else {
				src.rects = []FrameDirtyRect{{X: rx, Y: ry, Width: rw, Height: rh}}
			}
		}
		comp.composite()
		snap, _, _ := comp.GetFrameSnapshot()
		out = append(out, snap)
	}
	return out
}

func TestCompositeTiles_OutputBitIdenticalToFullFrame(t *testing.T) {
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")
	for _, seed := range []int64{1, 2, 3, 4, 5} {
		t.Setenv("IE_VIDEO_TILE_COMPOSITE", "0")
		want := compositeTileScenario(t, seed, 12, 192, 128)
		t.Setenv("IE_VIDEO_TILE_COMPOSITE", "1")
		got := compositeTileScenario(t, seed, 12, 192, 128)
		for i := range want {
			if !bytes.Equal(want[i], got[i]) {
				t.Fatalf("seed %d frame %d: tiled output differs from full-frame output", seed, i)
			}
		}
	}
}

func TestCompositeTiles_ParallelLargeDirtySetMatchesSerial(t *testing.T) {
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")
	t.Setenv("IE_VIDEO_TILE_COMPOSITE", "1")

	// 640x480 is 80 tiles, comfortably over the parallel threshold, so the
	// full-dirty frames below take the worker path.
	got := compositeTileScenario(t, 99, 6, 640, 480)

	t.Setenv("IE_VIDEO_TILE_COMPOSITE", "0")
	want := compositeTileScenario(t, 99, 6, 640, 480)

	for i := range want {
		if !bytes.Equal(want[i], got[i]) {
			t.Fatalf("frame %d: parallel tile output differs from serial full-frame output", i)
		}
	}
}

func TestCompositeTiles_KillSwitchRestoresFullFrameComposite(t *testing.T) {
	t.Setenv("IE_VIDEO_FRAME_LEASES", "1")
	t.Setenv("IE_VIDEO_TILE_COMPOSITE", "0")

	comp := NewVideoCompositor(nil)
	comp.LockResolution(128, 128)
	src := newTileDirtySource(0, 128, 128, true, solidTestFrame(128, 128, 1, 2, 3, 0xFF))
	comp.RegisterSource(src)
	for range 4 {
		comp.composite()
	}
	if stats := comp.TileCompositeStats(); stats.Frames != 0 {
		t.Fatalf("tile path ran %d frames with the kill switch set", stats.Frames)
	}
}

func BenchmarkComposite_SmallDirty(b *testing.B) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(640, 480)
	src := newTileDirtySource(0, 640, 480, true, solidTestFrame(640, 480, 0x22, 0x44, 0x66, 0xFF))
	comp.RegisterSource(src)
	comp.composite()
	b.SetBytes(640 * 480 * BYTES_PER_PIXEL)
	b.ResetTimer()
	for range b.N {
		src.rects = []FrameDirtyRect{{X: 32, Y: 32, Width: 48, Height: 48}}
		comp.composite()
	}
}

func BenchmarkComposite_NoDirty(b *testing.B) {
	comp := NewVideoCompositor(nil)
	comp.LockResolution(640, 480)
	src := newTileDirtySource(0, 640, 480, true, solidTestFrame(640, 480, 0x22, 0x44, 0x66, 0xFF))
	comp.RegisterSource(src)
	comp.composite()
	b.SetBytes(640 * 480 * BYTES_PER_PIXEL)
	b.ResetTimer()
	for range b.N {
		comp.composite()
	}
}
