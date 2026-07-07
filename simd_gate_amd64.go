//go:build amd64 && goexperiment.simd

package main

import "simd/archsimd"

// simdHostSupported reports whether the host CPU provides the SIMD baseline the
// kernels require. AVX2 (256-bit integer and float ops, the Float32x8 / Uint32x8
// width the kernels are written against) is the baseline; older hosts route
// scalar. Queried at runtime, per the archsimd recommendation to gate on feature
// checks before using the corresponding vector operations.
func simdHostSupported() bool {
	return archsimd.X86.AVX2()
}

// init wires every ...Impl dispatch var to its SIMD variant when SIMD is both
// requested (IE_SIMD != "0") and supported by the host. Scalar impls remain the
// package-level defaults otherwise, so differential tests can always call the
// scalar leaf directly regardless of this assignment. Indirect call cost is paid
// once per strip/row, never per pixel.
func init() {
	if !simdRequested || !simdHostSupported() {
		return
	}
	simdKernelsActive = true
	// Per-kernel Impl vars are reassigned by the assignSIMDKernels helpers each
	// tagged kernel file registers; kept out of this init so kernel files stay
	// self-contained and land phase by phase.
	assignSIMDKernels()
}
