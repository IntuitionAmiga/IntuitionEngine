//go:build linux && arm64 && goexperiment.simd

package main

import (
	"reflect"
	"testing"
)

func sameFunction(a, b any) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func TestSIMDARM64Dispatch(t *testing.T) {
	if !simdRequested {
		t.Skip("IE_SIMD=0 selects scalar dispatch")
	}
	if !simdKernelsActive {
		t.Fatal("Linux ARM64 SIMD is not active")
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"audio clamp", clampF32SpanImpl, clampF32SpanSIMD},
		{"blitter fill", fillUint32LESpanImpl, fillUint32LESpanSIMD},
		{"opaque copy", compositorOpaqueCopySpanImpl, compositorOpaqueCopySpanSIMD},
		{"frame normalisation", normaliseFrameLeaseSpanImpl, normaliseFrameLeaseSpanSIMD},
		{"compositor blend", compositorBlendSpanImpl, compositorBlendSpanSIMD},
		{"Voodoo spans", voodooRasterizeRowsSIMDFn, rasterizeRowsSIMD},
		{"scalar resampling", compositorResampleRowImpl, compositorResampleRowScalar},
	}
	for _, check := range checks {
		if !sameFunction(check.got, check.want) {
			t.Errorf("%s dispatch is incorrect", check.name)
		}
	}
}

func TestSIMDARM64KillSwitchUsesScalarDispatch(t *testing.T) {
	if simdRequested {
		t.Skip("set IE_SIMD=0 to exercise the kill switch")
	}
	if simdKernelsActive {
		t.Fatal("Linux ARM64 SIMD is active despite IE_SIMD=0")
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"audio clamp", clampF32SpanImpl, clampF32SpanScalar},
		{"blitter fill", fillUint32LESpanImpl, fillUint32LESpanScalar},
		{"opaque copy", compositorOpaqueCopySpanImpl, compositorOpaqueCopySpanScalar},
		{"frame normalisation", normaliseFrameLeaseSpanImpl, normaliseFrameLeaseSpanScalar},
		{"compositor blend", compositorBlendSpanImpl, compositorBlendSpanScalar},
		{"scalar resampling", compositorResampleRowImpl, compositorResampleRowScalar},
	}
	for _, check := range checks {
		if !sameFunction(check.got, check.want) {
			t.Errorf("%s did not remain scalar", check.name)
		}
	}
	if voodooRasterizeRowsSIMDFn != nil {
		t.Error("Voodoo SIMD dispatch is set despite IE_SIMD=0")
	}
}
