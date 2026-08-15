//go:build linux && arm64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

func fillUint32LESpanSIMD(dst []byte, v uint32) {
	words := len(dst) / 4
	full := words &^ (simdPixelLanes - 1)
	if full > 0 {
		du := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), words)
		value := archsimd.BroadcastUint32x4(v)
		for i := 0; i < full; i += simdPixelLanes {
			value.Store(du[i : i+simdPixelLanes])
		}
	}
	if tail := full * 4; tail < len(dst) {
		fillUint32LESpanScalar(dst[tail:], v)
	}
}
