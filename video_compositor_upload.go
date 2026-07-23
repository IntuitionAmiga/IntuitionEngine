// video_compositor_upload.go - dirty region upload planning.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
Upload planning decides which parts of a composed frame are handed to the
output backend.

The planner is deliberately free of any backend type so it can be tested
headless. It clips the dirty rectangles the compositor produced to the frame,
drops empty ones, discards rectangles already covered by another, and gives up
on regional upload entirely once the regions cover most of the frame or there
are too many of them, because a single whole-frame upload is then cheaper than
many small ones. Giving up changes only how the pixels are delivered, never
which pixels they are.

Region pixels are packed into a scratch buffer owned by the planner, so a
steady state of the same regions frame after frame allocates nothing. The
scratch is discarded whenever the frame dimensions change, which is also what
invalidates any retained texture on the backend side.
*/

package main

import "os"

// uploadRegionCountLimit is the largest number of separate regions worth
// uploading before a whole-frame upload wins.
const uploadRegionCountLimit = 16

// uploadRegionAreaLimitPercent is the share of the frame above which regional
// upload is abandoned in favour of a whole-frame upload.
const uploadRegionAreaLimitPercent = 60

// videoPartialUploadEnabled reports whether regional uploads are used at all.
// See sdk/docs/architecture.md, "Build Profiles and Observable Runtime".
func videoPartialUploadEnabled() bool {
	return os.Getenv("IE_VIDEO_PARTIAL_UPLOAD") != "0"
}

// uploadPlanner turns a compositor dirty rectangle set into the regions to
// upload, and packs their pixels without allocating in the steady state.
type uploadPlanner struct {
	frameW  int
	frameH  int
	planned []FrameDirtyRect
	scratch []byte
}

// invalidate drops everything the planner has retained. Called when the frame
// geometry changes, since neither the planned regions nor any texture the
// backend retained still describe the frame.
func (p *uploadPlanner) invalidate() {
	p.frameW = 0
	p.frameH = 0
	p.planned = p.planned[:0]
	p.scratch = p.scratch[:0]
}

// plan returns the regions to upload for this frame, or nil to mean "upload
// the whole frame". The returned slice is owned by the planner and is valid
// until the next call.
func (p *uploadPlanner) plan(frameW, frameH int, regions []FrameDirtyRect) []FrameDirtyRect {
	if frameW <= 0 || frameH <= 0 {
		return nil
	}
	if frameW != p.frameW || frameH != p.frameH {
		known := p.frameW != 0 && p.frameH != 0
		p.invalidate()
		p.frameW = frameW
		p.frameH = frameH
		if known {
			// The geometry changed, so whatever the backend retained no
			// longer describes the frame and this one goes up whole.
			return nil
		}
	}
	if !videoPartialUploadEnabled() || len(regions) == 0 {
		return nil
	}

	p.planned = p.planned[:0]
	area := 0
	for _, r := range regions {
		clipped, ok := clipFrameDirtyRect(r, frameW, frameH)
		if !ok {
			continue
		}
		if p.absorb(clipped) {
			continue
		}
		p.planned = append(p.planned, clipped)
		area += clipped.Width * clipped.Height
		if len(p.planned) > uploadRegionCountLimit {
			return nil
		}
	}
	if len(p.planned) == 0 {
		return nil
	}
	if area*100 >= frameW*frameH*uploadRegionAreaLimitPercent {
		return nil
	}
	return p.planned
}

// absorb reports whether rect is already covered by a planned region, and
// grows a planned region to cover rect when that region already contains it.
func (p *uploadPlanner) absorb(rect FrameDirtyRect) bool {
	for i := range p.planned {
		if frameDirtyRectContains(p.planned[i], rect) {
			return true
		}
	}
	return false
}

// regionPixels packs a region's pixels into the planner's scratch buffer.
func (p *uploadPlanner) regionPixels(frame []byte, rect FrameDirtyRect) []byte {
	rowBytes := rect.Width * BYTES_PER_PIXEL
	need := rowBytes * rect.Height
	if need <= 0 {
		return nil
	}
	if cap(p.scratch) < need {
		p.scratch = make([]byte, need)
	}
	out := p.scratch[:need]
	dst := 0
	for y := range rect.Height {
		src := ((rect.Y+y)*p.frameW + rect.X) * BYTES_PER_PIXEL
		if src+rowBytes > len(frame) {
			return nil
		}
		copy(out[dst:dst+rowBytes], frame[src:src+rowBytes])
		dst += rowBytes
	}
	return out
}

func clipFrameDirtyRect(r FrameDirtyRect, frameW, frameH int) (FrameDirtyRect, bool) {
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
		return FrameDirtyRect{}, false
	}
	return FrameDirtyRect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}, true
}

func frameDirtyRectContains(outer, inner FrameDirtyRect) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.Width <= outer.X+outer.Width &&
		inner.Y+inner.Height <= outer.Y+outer.Height
}
