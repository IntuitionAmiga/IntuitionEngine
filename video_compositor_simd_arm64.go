//go:build linux && arm64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

const simdPixelLanes = 4

func pixelSliceU32(b []byte, pixels int) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), pixels)
}

func compositorOpaqueCopySpanSIMD(dst, src []byte) {
	n := min(len(src), len(dst))
	pixels := n / BYTES_PER_PIXEL
	full := pixels &^ (simdPixelLanes - 1)
	if full > 0 {
		du := pixelSliceU32(dst, pixels)
		su := pixelSliceU32(src, pixels)
		alpha := archsimd.BroadcastUint32x4(0xFF000000)
		for i := 0; i < full; i += simdPixelLanes {
			archsimd.LoadUint32x4(su[i : i+simdPixelLanes]).Or(alpha).Store(du[i : i+simdPixelLanes])
		}
	}
	if tail := full * BYTES_PER_PIXEL; tail < n {
		compositorOpaqueCopySpanScalar(dst[tail:n], src[tail:n])
	}
}

func compositorBlendSpanSIMD(dst, src []byte) {
	n := min(len(src), len(dst))
	pixels := n / BYTES_PER_PIXEL
	full := pixels &^ (simdPixelLanes - 1)
	if full > 0 {
		du := pixelSliceU32(dst, pixels)
		su := pixelSliceU32(src, pixels)
		aMask := archsimd.BroadcastUint32x4(0xFF000000)
		rgbMask := archsimd.BroadcastUint32x4(0x00FFFFFF)
		zero := archsimd.BroadcastUint32x4(0)
		for i := 0; i < full; i += simdPixelLanes {
			v := archsimd.LoadUint32x4(su[i : i+simdPixelLanes])
			d := archsimd.LoadUint32x4(du[i : i+simdPixelLanes])
			alphaNonZero := v.And(aMask).NotEqual(zero)
			rgbNonZero := v.And(rgbMask).NotEqual(zero)
			value := v.IfElse(alphaNonZero, v.Or(aMask))
			value.IfElse(alphaNonZero.Or(rgbNonZero), d).Store(du[i : i+simdPixelLanes])
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
		aMask := archsimd.BroadcastUint32x4(0xFF000000)
		rgbMask := archsimd.BroadcastUint32x4(0x00FFFFFF)
		zero := archsimd.BroadcastUint32x4(0)
		for i := 0; i < full; i += simdPixelLanes {
			v := archsimd.LoadUint32x4(pu[i : i+simdPixelLanes])
			promote := v.And(aMask).Equal(zero).And(v.And(rgbMask).NotEqual(zero))
			v.Or(aMask).IfElse(promote, v).Store(pu[i : i+simdPixelLanes])
		}
	}
	if tail := full * BYTES_PER_PIXEL; tail < len(pixels) {
		normaliseFrameLeaseSpanScalar(pixels[tail:])
	}
}
