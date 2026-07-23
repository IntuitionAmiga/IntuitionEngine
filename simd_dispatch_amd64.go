//go:build amd64 && goexperiment.simd

package main

// assignSIMDKernels points each ...Impl dispatch var at its SIMD variant. Called
// once from simd_gate_amd64.go's init() only when SIMD is requested and the host
// is supported. Each phase of the SIMD acceleration work appends its kernel
// registration here; Phase 0 registers nothing.
func assignSIMDKernels() {
	// Phase 2: compositor. Opaque copy is bandwidth-bound; it is wired only if
	// its benchstat win clears the 5% stop rule (see simd_evidence notes). Blend
	// and lease normalise do per-pixel classification and win clearly.
	compositorBlendSpanImpl = compositorBlendSpanSIMD
	compositorOpaqueCopySpanImpl = compositorOpaqueCopySpanSIMD
	normaliseFrameLeaseSpanImpl = normaliseFrameLeaseSpanSIMD
	// Scaled composition: the horizontal resample gather becomes one
	// cross-lane permute per eight destination pixels where the plan allows it.
	compositorResampleRowImpl = compositorResampleRowSIMD

	// Phase 3: blitter. Fill vectorises cleanly. Colour-expand stays scalar
	// (colorExpandRowImpl unset here) per the stop rule; the scalar fast path is
	// its win.
	fillUint32LESpanImpl = fillUint32LESpanSIMD

	// Phase 4: Voodoo untextured spans. Bit-exact for the eligible setup class;
	// the scalar rasteriser stays the conformance reference for everything else.
	voodooRasterizeRowsSIMDFn = rasterizeRowsSIMD
}
