//go:build !amd64 || !goexperiment.simd

package main

// simdHostSupported reports whether the host can execute the SIMD kernels. On
// non-amd64 targets, or when the goexperiment.simd build tag is absent, there
// are no SIMD kernels and this is always false. Keeps CGO_ENABLED=0 cross builds
// (build-purego-novulkan-vm-binary, Makefile) compiling with scalar-only paths.
func simdHostSupported() bool { return false }
