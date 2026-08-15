//go:build linux && arm64 && goexperiment.simd

package main

func simdHostSupported() bool { return true }

func init() {
	if !simdRequested {
		return
	}
	simdKernelsActive = true
	assignSIMDKernels()
}
