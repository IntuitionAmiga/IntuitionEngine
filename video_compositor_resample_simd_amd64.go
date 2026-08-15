//go:build amd64 && goexperiment.simd

package main

import (
	"simd/archsimd"
)

// compositorResampleRowSIMD gathers a destination row through one cross-lane
// permute per eight pixels. Blocks the plan could not vectorise, a colBase
// that is not block aligned, and the sub-block tail all defer to the scalar
// leaf, so edge behaviour has exactly one source of truth.
func compositorResampleRowSIMD(dst, srcRow []byte, p *resamplePlan, colBase, cols int) {
	if !p.vectorOK || colBase%resampleLanes != 0 || cols < resampleLanes {
		compositorResampleRowScalar(dst, srcRow, p, colBase, cols)
		return
	}
	firstBlock := colBase / resampleLanes
	blocks := cols / resampleLanes
	i := 0
	for b := range blocks {
		idx := firstBlock + b
		if idx >= len(p.blockBase) || p.blockBase[idx] < 0 {
			compositorResampleRowScalar(dst[i*BYTES_PER_PIXEL:], srcRow, p, colBase+i, resampleLanes)
			i += resampleLanes
			continue
		}
		base := int(p.blockBase[idx])
		src := pixelSliceU32(srcRow[base:], resampleLanes)
		perm := p.blockPerm[idx*resampleLanes : idx*resampleLanes+resampleLanes]
		v := archsimd.LoadUint32x8(src)
		indices := archsimd.LoadUint32x8(perm)
		out := v.Permute(indices)
		du := pixelSliceU32(dst[i*BYTES_PER_PIXEL:], resampleLanes)
		out.Store(du)
		i += resampleLanes
	}
	if i < cols {
		compositorResampleRowScalar(dst[i*BYTES_PER_PIXEL:], srcRow, p, colBase+i, cols-i)
	}
}
