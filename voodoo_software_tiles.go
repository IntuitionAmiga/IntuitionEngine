// voodoo_software_tiles.go - tile binning and worker pool for the software rasteriser.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
Tile binning splits a triangle flush over the framebuffer rather than over the
rows of one triangle. The existing row-band path parallelises a single large
triangle and does nothing for a flush of many small ones, which is the common
shape for real scenes.

The binner walks the flush once in submission order, builds each triangle's
read-only setup, and records its index in every tile its bounding box touches.
Workers then take whole tiles. A tile owns its pixels exclusively, so no two
workers ever touch the same colour or depth word, and each worker replays its
tile's list in submission order. Depth-equal, blended, fogged, chroma-keyed and
overlapping translucent primitives therefore land in the same order they would
have serially, and the framebuffer is bit-identical to the serial path.

Tiles are indexed in raster y, before the Y-flip. The flip is a bijection on
rows, so disjoint raster rows stay disjoint destination rows either way.

Clipping a setup to a tile is safe because every pixel result is a function of
its own coordinates, the setup and (for depth and blending) that pixel's own
prior contents. Narrowing the loop bounds cannot change a pixel that remains in
range, and the per-row early exit only ends a row once the span has been left.

Switches: IE_VOODOO_TILE_RASTER=0 restores the per-triangle path, and
IE_VOODOO_WORKERS overrides the worker count, with 1 meaning serial tile replay.
See sdk/docs/architecture.md, "Build Profiles and Observable Runtime".
*/

package main

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
)

// voodooTileW and voodooTileH are the binning grid dimensions in pixels. 64x64
// keeps a tile's colour and depth working set inside L2 while staying coarse
// enough that binning cost stays well below raster cost.
const (
	voodooTileW = 64
	voodooTileH = 64
)

// voodooTileRasterEnabled reports whether the tiled rasteriser is selected.
// It is the default; IE_VOODOO_TILE_RASTER=0 restores the per-triangle path.
func voodooTileRasterEnabled() bool {
	return os.Getenv("IE_VOODOO_TILE_RASTER") != "0"
}

// voodooRasterWorkers returns the worker count for tile replay. IE_VOODOO_WORKERS
// overrides the default; 1 replays every tile on the calling goroutine.
func voodooRasterWorkers() int {
	if v := os.Getenv("IE_VOODOO_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			return n
		}
	}
	return max(1, runtime.NumCPU())
}

// voodooTileBinner holds the per-flush binning state. It is owned by one
// backend and reused across flushes, so a steady state of similar scenes
// allocates nothing.
type voodooTileBinner struct {
	gridW, gridH int
	frameW       int
	frameH       int
	setups       []voodooTriangleSetup
	rows         []voodooSetupRows
	bins         [][]int32
	nonEmpty     []int32
	cursor       atomic.Int64
}

// voodooSetupRows is the row range of a prepared setup.
type voodooSetupRows struct{ minY, maxY int }

// reset prepares the binner for a flush over a frame of the given size.
func (t *voodooTileBinner) reset(frameW, frameH int) {
	gridW := (frameW + voodooTileW - 1) / voodooTileW
	gridH := (frameH + voodooTileH - 1) / voodooTileH
	if gridW != t.gridW || gridH != t.gridH {
		t.gridW, t.gridH = gridW, gridH
		t.bins = make([][]int32, gridW*gridH)
	}
	t.frameW, t.frameH = frameW, frameH
	for i := range t.bins {
		t.bins[i] = t.bins[i][:0]
	}
	t.setups = t.setups[:0]
	t.rows = t.rows[:0]
	t.nonEmpty = t.nonEmpty[:0]
}

// add records a prepared setup in every tile its bounding box touches.
func (t *voodooTileBinner) add(setup *voodooTriangleSetup, minY, maxY int) {
	if setup.minX >= setup.maxX || minY >= maxY {
		return
	}
	idx := int32(len(t.setups))
	t.setups = append(t.setups, *setup)
	t.rows = append(t.rows, voodooSetupRows{minY: minY, maxY: maxY})

	tx0 := max(0, setup.minX/voodooTileW)
	tx1 := min(t.gridW-1, (setup.maxX-1)/voodooTileW)
	ty0 := max(0, minY/voodooTileH)
	ty1 := min(t.gridH-1, (maxY-1)/voodooTileH)
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			bin := ty*t.gridW + tx
			if len(t.bins[bin]) == 0 {
				t.nonEmpty = append(t.nonEmpty, int32(bin))
			}
			t.bins[bin] = append(t.bins[bin], idx)
		}
	}
}

// replay rasterises every non-empty tile, each tile's list in submission order.
func (t *voodooTileBinner) replay(b *VoodooSoftwareBackend, workers int) {
	if len(t.nonEmpty) == 0 {
		return
	}
	if workers > len(t.nonEmpty) {
		workers = len(t.nonEmpty)
	}
	if workers <= 1 {
		for _, bin := range t.nonEmpty {
			t.replayTile(b, int(bin))
		}
		return
	}
	t.cursor.Store(0)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(t.cursor.Add(1)) - 1
				if i >= len(t.nonEmpty) {
					return
				}
				t.replayTile(b, int(t.nonEmpty[i]))
			}
		}()
	}
	wg.Wait()
}

// replayTile rasterises one tile's binned setups, clipped to the tile.
func (t *voodooTileBinner) replayTile(b *VoodooSoftwareBackend, bin int) {
	tx := (bin % t.gridW) * voodooTileW
	ty := (bin / t.gridW) * voodooTileH
	x0, x1 := tx, min(tx+voodooTileW, t.frameW)
	y0, y1 := ty, min(ty+voodooTileH, t.frameH)

	for _, idx := range t.bins[bin] {
		local := t.setups[idx]
		rows := t.rows[idx]
		local.minX = max(local.minX, x0)
		local.maxX = min(local.maxX, x1)
		if local.minX >= local.maxX {
			continue
		}
		ry0 := max(rows.minY, y0)
		ry1 := min(rows.maxY, y1)
		if ry0 >= ry1 {
			continue
		}
		b.rasterizeRows(&local, ry0, ry1)
	}
}

// flushTrianglesTiled bins a whole flush and replays it tile by tile. The
// caller holds fbMu, so the flush is complete when this returns and WaitSwapIdle
// keeps meaning that all raster work has finished.
func (b *VoodooSoftwareBackend) flushTrianglesTiled(triangles []VoodooTriangle, live softwareLiveState) {
	if b.tileBinner == nil {
		b.tileBinner = &voodooTileBinner{}
	}
	t := b.tileBinner
	t.reset(b.width, b.height)

	var applied *VoodooRasterState
	state := live
	for i := range triangles {
		if st := triangles[i].State; st != nil && st != applied {
			state = softwareLiveStateFromSnapshot(st)
			applied = st
		}
		setup, minY, maxY, ok := b.buildTriangleSetup(&triangles[i], &state)
		if !ok {
			continue
		}
		t.add(&setup, minY, maxY)
	}
	t.replay(b, voodooRasterWorkers())
}
