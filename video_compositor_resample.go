// video_compositor_resample.go - horizontal resample gather for scaled layers.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
Scaled composition samples each destination column from a source column given
by dx*srcW/rectW. That mapping depends only on the two widths, so it is
computed once into a plan and reused for every row of every frame at that
scale.

The plan also carries a vector form. Whenever the destination is no narrower
than the source, the column index advances by at most one per destination
pixel, so any eight consecutive destination pixels read from a window of at
most eight consecutive source pixels. Those eight pixels can be loaded once
and rearranged with a single cross-lane permute, which is what the SIMD kernel
does. Narrowing scales, and blocks whose window would read past the end of the
source row, fall back to the scalar gather, so the result is identical either
way.
*/

package main

import "unsafe"

// resampleLanes is the number of destination pixels one vector block covers.
const resampleLanes = 8

// resamplePlan holds the per-column source offsets for one (srcW, rectW) pair.
type resamplePlan struct {
	srcW     int
	rectW    int
	srcXByte []int
	// blockBase is the byte offset of the eight-pixel source load for each
	// full block of destination pixels, or -1 when that block cannot be
	// served by a single load.
	blockBase []int32
	// blockPerm holds resampleLanes lane indices per block.
	blockPerm []uint32
	vectorOK  bool
}

// newResamplePlan builds the resample plan for scaling srcW source columns
// across rectW destination columns.
func newResamplePlan(srcW, rectW int) *resamplePlan {
	p := &resamplePlan{srcW: srcW, rectW: rectW}
	if srcW <= 0 || rectW <= 0 {
		return p
	}
	p.srcXByte = make([]int, rectW)
	for dx := range p.srcXByte {
		p.srcXByte[dx] = (dx * srcW / rectW) * BYTES_PER_PIXEL
	}
	p.buildVectorPlan()
	return p
}

func (p *resamplePlan) buildVectorPlan() {
	blocks := p.rectW / resampleLanes
	if blocks == 0 {
		return
	}
	base := make([]int32, blocks)
	perm := make([]uint32, blocks*resampleLanes)
	ok := false
	for b := range blocks {
		start := p.srcXByte[b*resampleLanes]
		usable := p.srcW >= resampleLanes
		// The load reads resampleLanes pixels from start, so it must stay
		// inside the source row. Near the right-hand edge the window is slid
		// back to the last full eight pixels, which only shifts the lane
		// indices.
		if usable {
			maxStart := (p.srcW - resampleLanes) * BYTES_PER_PIXEL
			if start > maxStart {
				start = maxStart
			}
			for k := range resampleLanes {
				delta := (p.srcXByte[b*resampleLanes+k] - start) / BYTES_PER_PIXEL
				if delta < 0 || delta >= resampleLanes {
					usable = false
					break
				}
				perm[b*resampleLanes+k] = uint32(delta)
			}
		}
		if usable {
			base[b] = int32(start)
			ok = true
		} else {
			base[b] = -1
		}
	}
	if ok {
		p.blockBase = base
		p.blockPerm = perm
		p.vectorOK = true
	}
}

// compositorResampleRowImpl defaults to the scalar leaf and is reassigned to
// the SIMD variant in assignSIMDKernels on supported hosts. Differential tests
// call compositorResampleRowScalar directly.
var compositorResampleRowImpl = compositorResampleRowScalar

// compositorResampleRowScalar gathers cols destination pixels from srcRow using
// the plan's column offsets. colBase is the first destination column, which is
// nonzero when only part of a row is being composed.
func compositorResampleRowScalar(dst, srcRow []byte, p *resamplePlan, colBase, cols int) {
	for i := range cols {
		*(*uint32)(unsafe.Pointer(&dst[i*BYTES_PER_PIXEL])) =
			*(*uint32)(unsafe.Pointer(&srcRow[p.srcXByte[colBase+i]]))
	}
}
