// video_compositor_tiles.go - tile-based retained software composition.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
Tile-based retained composition composes only the tiles whose pixels can
differ from what the destination buffer already holds.

The software output path hands each frame to a lease ring slot, and a slot's
pixels survive until that slot is handed out again. A slot therefore still
holds a complete, correct composite of some earlier frame. Composing a new
frame into it only has to repaint the tiles that have been dirtied since that
earlier frame; every other tile is already correct and is retained untouched.

The per-frame dirty tile sets are kept in a short history so the union over
the frames between the slot's contents and the frame being composed can be
computed exactly. A frame whose dirty region is unknown (any layer without
dirty rectangles, a scanline-composited frame, a forced full frame, or a
layer set that has changed shape) marks every tile, which degrades cleanly to
the full-frame composite.

Every tile is composed with the same per-destination-pixel rules as the
full-frame path, so the output is bit-identical.
*/

package main

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

// compositorTileSize is the edge length in pixels of a composition tile.
const compositorTileSize = 64

// compositorTileParallelThreshold is the number of tiles needing composition
// below which the work is done on the calling goroutine. Below it the worker
// hand-off costs more than the pixels saved.
const compositorTileParallelThreshold = 12

// videoTileCompositeEnabled reports whether tile-based retained composition is
// active. See sdk/docs/architecture.md, "Build Profiles and Observable
// Runtime".
func videoTileCompositeEnabled() bool {
	return os.Getenv("IE_VIDEO_TILE_COMPOSITE") != "0"
}

// tileDirtyEntry records which tiles a composed frame repainted.
type tileDirtyEntry struct {
	frame uint64
	bits  []uint64
}

// tileCompositeStats counts tile work for tests and diagnostics.
type tileCompositeStats struct {
	Frames     uint64
	Composed   uint64
	Retained   uint64
	FullFrames uint64
}

// tileLayerPlan is the per-layer geometry a tile composition needs, computed
// once per frame rather than once per tile.
type tileLayerPlan struct {
	layer    CompositorFrameLayer
	rect     scaleRect
	oneToOne bool
	plan     *resamplePlan
}

// tileWorkspace holds the per-goroutine scratch used while composing a tile.
type tileWorkspace struct {
	rowBuf []byte
}

// tileBitsScratch returns a zeroed bitmap of words words, reusing the backing
// array so a steady state composes without allocating.
func tileBitsScratch(buf *[]uint64, words int) []uint64 {
	if cap(*buf) < words {
		*buf = make([]uint64, words)
	}
	out := (*buf)[:words]
	for i := range out {
		out[i] = 0
	}
	return out
}

func tileBitsLen(count int) int {
	return (count + 63) / 64
}

func tileBitSet(bits []uint64, i int) {
	bits[i>>6] |= 1 << (uint(i) & 63)
}

func tileBitGet(bits []uint64, i int) bool {
	return bits[i>>6]&(1<<(uint(i)&63)) != 0
}

func tileBitsOr(dst, src []uint64) {
	for i := range dst {
		if i < len(src) {
			dst[i] |= src[i]
		}
	}
}

func tileBitsSetAll(bits []uint64, count int) {
	for i := range bits {
		bits[i] = ^uint64(0)
	}
	if rem := count & 63; rem != 0 && len(bits) > 0 {
		bits[len(bits)-1] = (uint64(1) << uint(rem)) - 1
	}
}

func tileBitsCount(bits []uint64) int {
	n := 0
	for _, w := range bits {
		for w != 0 {
			w &= w - 1
			n++
		}
	}
	return n
}

// TileCompositeStats returns a snapshot of the tile composition counters.
func (c *VideoCompositor) TileCompositeStats() tileCompositeStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tileStats
}

// invalidateTileStateLocked discards all retained tile knowledge. It is called
// whenever the lease slots stop holding what the history says they hold, which
// means a resolution change or a rebuilt lease ring.
func (c *VideoCompositor) invalidateTileStateLocked() {
	for i := range c.tileSlotFrame {
		c.tileSlotFrame[i] = 0
	}
	c.tileHistory = c.tileHistory[:0]
}

// tileLayerSignature summarises the structural shape of a layer set. Buffer
// contents are deliberately excluded: only geometry, ordering and blend mode
// decide whether a retained tile is still valid.
func tileLayerSignature(layers []CompositorFrameLayer) uint64 {
	h := uint64(1469598103934665603)
	mix := func(v uint64) {
		h ^= v
		h *= 1099511628211
	}
	mix(uint64(len(layers)))
	for i := range layers {
		l := &layers[i]
		mix(l.SourceID)
		mix(uint64(uint32(l.SourceWidth)))
		mix(uint64(uint32(l.SourceHeight)))
		mix(uint64(uint32(l.DestX)))
		mix(uint64(uint32(l.DestY)))
		mix(uint64(uint32(l.DestWidth)))
		mix(uint64(uint32(l.DestHeight)))
		if l.Opaque {
			mix(1)
		} else {
			mix(2)
		}
	}
	return h
}

// tileHistoryBits returns the dirty tile bitmap recorded for a frame.
func (c *VideoCompositor) tileHistoryBits(frame uint64) ([]uint64, bool) {
	for i := range c.tileHistory {
		if c.tileHistory[i].frame == frame {
			return c.tileHistory[i].bits, true
		}
	}
	return nil, false
}

// recordTileHistoryLocked stores this frame's dirty tile bitmap, replacing an
// existing entry for the same frame so a repeated composition of one frame
// (the hardware compositor fallback path) does not duplicate it.
func (c *VideoCompositor) recordTileHistoryLocked(frame uint64, bits []uint64) {
	for i := range c.tileHistory {
		if c.tileHistory[i].frame == frame {
			c.tileHistory[i].bits = appendReuse(c.tileHistory[i].bits, bits)
			return
		}
	}
	// Keep a little more history than the lease ring is deep, so a slot's
	// contents can always be aged forward while it is still in the ring.
	const tileHistoryDepth = 8
	if len(c.tileHistory) >= tileHistoryDepth {
		copy(c.tileHistory, c.tileHistory[1:])
		c.tileHistory = c.tileHistory[:len(c.tileHistory)-1]
	}
	// Reuse the evicted entry's backing array rather than allocating a new
	// bitmap every frame.
	if len(c.tileHistory) < cap(c.tileHistory) {
		c.tileHistory = c.tileHistory[:len(c.tileHistory)+1]
		last := &c.tileHistory[len(c.tileHistory)-1]
		last.frame = frame
		last.bits = appendReuse(last.bits, bits)
		return
	}
	c.tileHistory = append(c.tileHistory, tileDirtyEntry{frame: frame, bits: append([]uint64(nil), bits...)})
}

// appendReuse copies src into dst, reusing dst's array when it is large
// enough.
func appendReuse(dst, src []uint64) []uint64 {
	if cap(dst) < len(src) {
		dst = make([]uint64, len(src))
	}
	dst = dst[:len(src)]
	copy(dst, src)
	return dst
}

// dirtyRectsToTiles marks every tile a destination-space rectangle touches.
func dirtyRectsToTiles(bits []uint64, rects []FrameDirtyRect, frameW, frameH, gridW int) {
	for _, r := range rects {
		x0, y0 := r.X, r.Y
		x1, y1 := r.X+r.Width, r.Y+r.Height
		if x0 < 0 {
			x0 = 0
		}
		if y0 < 0 {
			y0 = 0
		}
		if x1 > frameW {
			x1 = frameW
		}
		if y1 > frameH {
			y1 = frameH
		}
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		for ty := y0 / compositorTileSize; ty <= (y1-1)/compositorTileSize; ty++ {
			for tx := x0 / compositorTileSize; tx <= (x1-1)/compositorTileSize; tx++ {
				tileBitSet(bits, ty*gridW+tx)
			}
		}
	}
}

// tileDirtyRects returns the destination-space rectangles the layer set has
// declared changed this frame, and whether that set is a complete description
// of the change. A layer without dirty rectangles has published no bound on
// what it changed, so the whole frame must be treated as dirty.
//
// This deliberately does not reuse collectOutputDirtyRectsLocked, whose nil
// return conflates "nothing changed" with "change extent unknown". Retained
// tiles need those two cases kept apart.
func tileDirtyRects(scratch []FrameDirtyRect, layers []CompositorFrameLayer) ([]FrameDirtyRect, bool) {
	rects := scratch[:0]
	for i := range layers {
		if layers[i].DirtyRects == nil {
			return nil, false
		}
		for _, rect := range layers[i].DirtyRects {
			scaled := scaleLayerDirtyRect(layers[i], rect)
			if scaled.Width > 0 && scaled.Height > 0 {
				rects = append(rects, scaled)
			}
		}
	}
	return rects, true
}

// tryRenderLayersTiledLocked composes layers into the current frame buffer,
// repainting only the tiles that can differ from what the buffer already
// holds. It reports false when the caller must fall back to the full-frame
// composite, having written nothing.
func (c *VideoCompositor) tryRenderLayersTiledLocked(layers []CompositorFrameLayer, slot int, frameID uint64) bool {
	if !videoTileCompositeEnabled() || slot < 0 || frameID == 0 {
		return false
	}
	w, h := c.frameWidth, c.frameHeight
	if w <= 0 || h <= 0 || len(c.finalFrame) != w*h*BYTES_PER_PIXEL {
		return false
	}

	gridW := (w + compositorTileSize - 1) / compositorTileSize
	gridH := (h + compositorTileSize - 1) / compositorTileSize
	tileCount := gridW * gridH
	if tileCount <= 0 {
		return false
	}

	if slot >= len(c.tileSlotFrame) {
		grown := make([]uint64, slot+1)
		copy(grown, c.tileSlotFrame)
		c.tileSlotFrame = grown
	}

	sig := tileLayerSignature(layers)
	if sig != c.tileLayerSig || c.tileGridW != gridW || c.tileGridH != gridH {
		c.invalidateTileStateLocked()
		c.tileLayerSig = sig
		c.tileGridW = gridW
		c.tileGridH = gridH
	}

	tileRects, known := tileDirtyRects(c.tileRectScratch, layers)
	c.tileRectScratch = tileRects[:0]

	words := tileBitsLen(tileCount)
	current := tileBitsScratch(&c.tileCurBits, words)
	fullFrame := c.forceFullFrame || !known || c.scanlineCompositingActiveLocked()
	if fullFrame {
		tileBitsSetAll(current, tileCount)
	} else {
		dirtyRectsToTiles(current, tileRects, w, h, gridW)
	}

	need := tileBitsScratch(&c.tileNeedBits, words)
	copy(need, current)
	base := c.tileSlotFrame[slot]
	if base == 0 || base >= frameID {
		tileBitsSetAll(need, tileCount)
	} else {
		for f := base + 1; f < frameID; f++ {
			bits, ok := c.tileHistoryBits(f)
			if !ok {
				tileBitsSetAll(need, tileCount)
				break
			}
			tileBitsOr(need, bits)
		}
	}

	plans := c.buildTilePlansLocked(layers)
	needCount := tileBitsCount(need)
	c.composeTilesLocked(plans, need, tileCount, gridW, needCount)

	c.tileSlotFrame[slot] = frameID
	c.recordTileHistoryLocked(frameID, current)
	c.softwareDirtyRects = c.collectOutputDirtyRectsLocked(layers)

	c.tileStats.Frames++
	c.tileStats.Composed += uint64(needCount)
	c.tileStats.Retained += uint64(tileCount - needCount)
	if needCount == tileCount {
		c.tileStats.FullFrames++
	}
	return true
}

// scanlineCompositingActiveLocked reports whether any enabled source is being
// composited scanline by scanline this frame. Those effects sample state that
// the dirty rectangles do not describe, so they force a full frame.
func (c *VideoCompositor) scanlineCompositingActiveLocked() bool {
	for i := range c.sources {
		source := c.sources[i].source
		if source == nil || !source.IsEnabled() {
			continue
		}
		if _, ok := source.(ScanlineAware); !ok {
			continue
		}
		if selector, ok := source.(ScanlineCompositingSource); ok {
			if selector.NeedsScanlineCompositing() {
				return true
			}
			continue
		}
		return true
	}
	return false
}

func (c *VideoCompositor) buildTilePlansLocked(layers []CompositorFrameLayer) []tileLayerPlan {
	plans := c.tilePlans[:0]
	for i := range layers {
		layer := layers[i]
		if layer.SourceWidth <= 0 || layer.SourceHeight <= 0 ||
			len(layer.Buffer) < layer.SourceWidth*layer.SourceHeight*BYTES_PER_PIXEL {
			continue
		}
		rect := scaleRect{x: layer.DestX, y: layer.DestY, w: layer.DestWidth, h: layer.DestHeight}
		if rect.w <= 0 || rect.h <= 0 {
			continue
		}
		plan := tileLayerPlan{layer: layer, rect: rect}
		plan.oneToOne = rect.x == 0 && rect.y == 0 && rect.w == c.frameWidth && rect.h == c.frameHeight &&
			layer.SourceWidth == c.frameWidth && layer.SourceHeight == c.frameHeight
		if !plan.oneToOne {
			plan.plan = c.cachedResamplePlanLocked(layer.SourceWidth, rect.w)
		}
		plans = append(plans, plan)
	}
	c.tilePlans = plans
	return plans
}

// composeTilesLocked repaints each marked tile. Tiles own disjoint destination
// pixels and read only immutable layer buffers, so they may be composed
// concurrently.
func (c *VideoCompositor) composeTilesLocked(plans []tileLayerPlan, need []uint64, tileCount, gridW, needCount int) {
	if needCount == 0 {
		return
	}
	workers := 1
	if needCount >= compositorTileParallelThreshold {
		workers = runtime.GOMAXPROCS(0)
		if workers > needCount {
			workers = needCount
		}
		if workers < 1 {
			workers = 1
		}
	}
	if workers == 1 {
		if c.tileSerialWS.rowBuf == nil {
			c.tileSerialWS.rowBuf = make([]byte, compositorTileSize*BYTES_PER_PIXEL)
		}
		for i := 0; i < tileCount; i++ {
			if tileBitGet(need, i) {
				c.composeTile(plans, i, gridW, &c.tileSerialWS)
			}
		}
		return
	}

	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ws := tileWorkspace{rowBuf: make([]byte, compositorTileSize*BYTES_PER_PIXEL)}
			for {
				i := int(next.Add(1)) - 1
				if i >= tileCount {
					return
				}
				if tileBitGet(need, i) {
					c.composeTile(plans, i, gridW, &ws)
				}
			}
		}()
	}
	wg.Wait()
}

// composeTile clears one tile and blends every layer into it, using exactly
// the per-destination-pixel rules of the full-frame path.
func (c *VideoCompositor) composeTile(plans []tileLayerPlan, index, gridW int, ws *tileWorkspace) {
	tx := index % gridW
	ty := index / gridW
	clip := scaleRect{
		x: tx * compositorTileSize,
		y: ty * compositorTileSize,
		w: compositorTileSize,
		h: compositorTileSize,
	}
	if clip.x+clip.w > c.frameWidth {
		clip.w = c.frameWidth - clip.x
	}
	if clip.y+clip.h > c.frameHeight {
		clip.h = c.frameHeight - clip.y
	}
	if clip.w <= 0 || clip.h <= 0 {
		return
	}

	rowBytes := c.frameWidth * BYTES_PER_PIXEL
	spanBytes := clip.w * BYTES_PER_PIXEL
	for y := clip.y; y < clip.y+clip.h; y++ {
		off := y*rowBytes + clip.x*BYTES_PER_PIXEL
		row := c.finalFrame[off : off+spanBytes]
		for i := range row {
			row[i] = 0
		}
	}
	for i := range plans {
		c.blendPlanRegion(&plans[i], clip, ws)
	}
}

// blendPlanRegion blends one layer into the destination pixels inside clip.
// Both the 1:1 and the scaled paths are pure per-destination-pixel functions
// of the layer, so restricting them to a rectangle cannot change the result.
func (c *VideoCompositor) blendPlanRegion(plan *tileLayerPlan, clip scaleRect, ws *tileWorkspace) {
	rect := plan.rect
	x0 := max(rect.x, clip.x)
	y0 := max(rect.y, clip.y)
	x1 := min(rect.x+rect.w, clip.x+clip.w)
	y1 := min(rect.y+rect.h, clip.y+clip.h)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	layer := &plan.layer
	dstRowBytes := c.frameWidth * BYTES_PER_PIXEL
	spanBytes := (x1 - x0) * BYTES_PER_PIXEL

	if plan.oneToOne {
		for y := y0; y < y1; y++ {
			off := y*dstRowBytes + x0*BYTES_PER_PIXEL
			if layer.Opaque {
				compositorOpaqueCopySpanImpl(c.finalFrame[off:off+spanBytes], layer.Buffer[off:off+spanBytes])
			} else {
				compositorBlendSpanImpl(c.finalFrame[off:off+spanBytes], layer.Buffer[off:off+spanBytes])
			}
		}
		return
	}

	cols := x1 - x0
	if cap(ws.rowBuf) < spanBytes {
		ws.rowBuf = make([]byte, spanBytes)
	}
	rowBuf := ws.rowBuf[:spanBytes]
	srcRowBytes := layer.SourceWidth * BYTES_PER_PIXEL
	colBase := x0 - rect.x
	for dyAbs := y0; dyAbs < y1; dyAbs++ {
		srcY := (dyAbs - rect.y) * layer.SourceHeight / rect.h
		srcRowOffset := srcY * srcRowBytes
		compositorResampleRowImpl(rowBuf, layer.Buffer[srcRowOffset:], plan.plan, colBase, cols)
		off := dyAbs*dstRowBytes + x0*BYTES_PER_PIXEL
		if layer.Opaque {
			compositorOpaqueCopySpanImpl(c.finalFrame[off:off+spanBytes], rowBuf)
		} else {
			compositorBlendSpanImpl(c.finalFrame[off:off+spanBytes], rowBuf)
		}
	}
}
