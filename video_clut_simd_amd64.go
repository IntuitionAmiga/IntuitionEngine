//go:build amd64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

// clut8ExpandSpanSIMD gathers palette words scalar (archsimd 1.26 has no gather)
// into an 8-lane staging buffer, then wide-stores each vector. Byte-identical to
// the scalar leaf. Whether this beats the scalar/unrolled leaves is a benchmark
// question (the eight dependent LUT loads, not the store, bound the kernel); it
// is wired only if the stop rule is cleared.
func clut8ExpandSpanSIMD(dst, src []byte, pal *[256]uint32) {
	n := len(src)
	full := n &^ (simdPixelLanes - 1)
	if full > 0 {
		du := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), n)
		var lane [8]uint32
		for i := 0; i < full; i += simdPixelLanes {
			lane[0] = pal[src[i]]
			lane[1] = pal[src[i+1]]
			lane[2] = pal[src[i+2]]
			lane[3] = pal[src[i+3]]
			lane[4] = pal[src[i+4]]
			lane[5] = pal[src[i+5]]
			lane[6] = pal[src[i+6]]
			lane[7] = pal[src[i+7]]
			archsimd.LoadUint32x8(&lane).StoreSlice(du[i : i+simdPixelLanes])
		}
	}
	for i := full; i < n; i++ {
		*(*uint32)(unsafe.Pointer(&dst[i*4])) = pal[src[i]]
	}
}
