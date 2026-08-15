//go:build linux && arm64 && goexperiment.simd

package main

import "simd/archsimd"

const simdF32Lanes = 4

func clampF32SpanSIMD(s []float32, min, max float32) {
	full := len(s) &^ (simdF32Lanes - 1)
	if full > 0 {
		lo := archsimd.BroadcastFloat32x4(min)
		hi := archsimd.BroadcastFloat32x4(max)
		for i := 0; i < full; i += simdF32Lanes {
			v := archsimd.LoadFloat32x4(s[i : i+simdF32Lanes])
			v = lo.IfElse(v.Less(lo), v)
			v = hi.IfElse(v.Greater(hi), v)
			v.Store(s[i : i+simdF32Lanes])
		}
	}
	if full < len(s) {
		clampF32SpanScalar(s[full:], min, max)
	}
}
