package main

import (
	"fmt"
	"os"
)

// simdRequested reports whether the runtime SIMD kill switch permits SIMD
// kernels. SIMD is default-on; IE_SIMD=0 opts out. Read once at init, matching
// the IE_JIT_DISPATCH_CACHE convention (jit_dispatch_cache.go:7).
var simdRequested = os.Getenv("IE_SIMD") != "0"

// simdKernelsActive is set true by the amd64 && goexperiment.simd init() when
// SIMD is both requested and supported by the host CPU. On any other build it
// stays false and every ...Impl var keeps its scalar default. This is the single
// source of truth queried by SIMDStatus.
var simdKernelsActive bool

// SIMDStatus returns a human-readable description of the SIMD dispatch state for
// diagnostics. It never triggers SIMD execution.
func SIMDStatus() string {
	return fmt.Sprintf("SIMD: requested=%v hostSupported=%v active=%v",
		simdRequested, simdHostSupported(), simdKernelsActive)
}
