//go:build amd64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

// SIMD variants of the compositor span kernels. Each is bit-exact with its
// scalar leaf: full 8-pixel vectors run through archsimd, and the sub-8-pixel
// tail defers to the scalar leaf so there is exactly one source of truth for
// edge behaviour. All loads/stores are unaligned; buffers are pixel-aligned
// (4-byte) which is sufficient for the 256-bit unaligned ops.

const simdPixelLanes = 8

// pixelSliceU32 reinterprets a pixel-aligned RGBA byte span as a uint32 slice.
// b must be non-empty and its base 4-byte aligned (guaranteed for RGBA pixel
// spans).
func pixelSliceU32(b []byte, pixels int) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), pixels)
}

func compositorOpaqueCopySpanSIMD(dst, src []byte) {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	pixels := n / BYTES_PER_PIXEL
	full := pixels &^ (simdPixelLanes - 1)
	if full > 0 {
		du := pixelSliceU32(dst, pixels)
		su := pixelSliceU32(src, pixels)
		alpha := archsimd.BroadcastUint32x8(0xFF000000)
		for i := 0; i < full; i += simdPixelLanes {
			v := archsimd.LoadUint32x8(su[i : i+simdPixelLanes])
			v.Or(alpha).Store(du[i : i+simdPixelLanes])
		}
	}
	if tail := full * BYTES_PER_PIXEL; tail < n {
		compositorOpaqueCopySpanScalar(dst[tail:n], src[tail:n])
	}
}

func compositorBlendSpanSIMD(dst, src []byte) {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	pixels := n / BYTES_PER_PIXEL
	full := pixels &^ (simdPixelLanes - 1)
	if full > 0 {
		du := pixelSliceU32(dst, pixels)
		su := pixelSliceU32(src, pixels)
		aMask := archsimd.BroadcastUint32x8(0xFF000000)
		rMask := archsimd.BroadcastUint32x8(0x00FFFFFF)
		zero := archsimd.BroadcastUint32x8(0)
		for i := 0; i < full; i += simdPixelLanes {
			v := archsimd.LoadUint32x8(su[i : i+simdPixelLanes])
			d := archsimd.LoadUint32x8(du[i : i+simdPixelLanes])
			alphaNonZero := v.And(aMask).NotEqual(zero)
			rgbNonZero := v.And(rMask).NotEqual(zero)
			promoted := v.Or(aMask)
			// alpha-set -> v, else promoted (only observed where rgb!=0).
			value := v.Merge(promoted, alphaNonZero)
			// write where not fully-zero; keep dst otherwise.
			writeMask := alphaNonZero.Or(rgbNonZero)
			value.Merge(d, writeMask).Store(du[i : i+simdPixelLanes])
		}
	}
	if tail := full * BYTES_PER_PIXEL; tail < n {
		compositorBlendSpanScalar(dst[tail:n], src[tail:n])
	}
}

func normaliseFrameLeaseSpanSIMD(pixels []byte) {
	npix := len(pixels) / BYTES_PER_PIXEL
	full := npix &^ (simdPixelLanes - 1)
	if full > 0 {
		pu := pixelSliceU32(pixels, npix)
		aMask := archsimd.BroadcastUint32x8(0xFF000000)
		rMask := archsimd.BroadcastUint32x8(0x00FFFFFF)
		zero := archsimd.BroadcastUint32x8(0)
		for i := 0; i < full; i += simdPixelLanes {
			v := archsimd.LoadUint32x8(pu[i : i+simdPixelLanes])
			alphaZero := v.And(aMask).Equal(zero)
			rgbNonZero := v.And(rMask).NotEqual(zero)
			promoted := v.Or(aMask)
			// promote only where alpha==0 && rgb!=0; every other lane keeps v
			// (fully-zero stays zero, alpha-set stays as-is).
			promoteMask := alphaZero.And(rgbNonZero)
			promoted.Merge(v, promoteMask).Store(pu[i : i+simdPixelLanes])
		}
	}
	if tail := full * BYTES_PER_PIXEL; tail < len(pixels) {
		normaliseFrameLeaseSpanScalar(pixels[tail:])
	}
}
