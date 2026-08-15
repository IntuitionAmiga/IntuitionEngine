//go:build linux && arm64 && goexperiment.simd

package main

func assignSIMDKernels() {
	compositorBlendSpanImpl = compositorBlendSpanSIMD
	compositorOpaqueCopySpanImpl = compositorOpaqueCopySpanSIMD
	normaliseFrameLeaseSpanImpl = normaliseFrameLeaseSpanSIMD
	fillUint32LESpanImpl = fillUint32LESpanSIMD
	voodooRasterizeRowsSIMDFn = rasterizeRowsSIMD
	clampF32SpanImpl = clampF32SpanSIMD
}
