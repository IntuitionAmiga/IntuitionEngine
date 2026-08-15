//go:build !goexperiment.simd || (!amd64 && (!linux || !arm64))

package main

// simdHostSupported reports whether the host can execute the SIMD kernels. On
// unsupported targets, or when the goexperiment.simd build tag is absent,
// there are no SIMD kernels and this is always false. This keeps portable
// cross-builds compiling with scalar-only paths.
func simdHostSupported() bool { return false }
