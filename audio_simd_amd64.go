//go:build amd64 && goexperiment.simd

package main

import "simd/archsimd"

const simdF32Lanes = 8

// scaleF32SpanSIMD multiplies s by gain in place, 8 lanes at a time. VMULPS has
// the same IEEE rounding as the scalar multiply and no FMA is used, so it is
// bit-identical to scaleF32SpanScalar. Sub-8 tail defers to the scalar leaf.
func scaleF32SpanSIMD(s []float32, gain float32) {
	full := len(s) &^ (simdF32Lanes - 1)
	if full > 0 {
		g := archsimd.BroadcastFloat32x8(gain)
		for i := 0; i < full; i += simdF32Lanes {
			archsimd.LoadFloat32x8Slice(s[i : i+simdF32Lanes]).Mul(g).StoreSlice(s[i : i+simdF32Lanes])
		}
	}
	if full < len(s) {
		scaleF32SpanScalar(s[full:], gain)
	}
}

// clampF32SpanSIMD clamps s to [min, max] in place via compare-and-blend, in the
// same order as the scalar leaf (lo clamp first, then hi), so NaN lanes fall
// through both compares unchanged. VMINPS/VMAXPS are deliberately avoided: their
// NaN handling would diverge from clampF32.
func clampF32SpanSIMD(s []float32, min, max float32) {
	full := len(s) &^ (simdF32Lanes - 1)
	if full > 0 {
		lo := archsimd.BroadcastFloat32x8(min)
		hi := archsimd.BroadcastFloat32x8(max)
		for i := 0; i < full; i += simdF32Lanes {
			v := archsimd.LoadFloat32x8Slice(s[i : i+simdF32Lanes])
			// where v < min -> min, else v
			v = lo.Merge(v, v.Less(lo))
			// where v > max -> max, else v
			v = hi.Merge(v, v.Greater(hi))
			v.StoreSlice(s[i : i+simdF32Lanes])
		}
	}
	if full < len(s) {
		clampF32SpanScalar(s[full:], min, max)
	}
}
