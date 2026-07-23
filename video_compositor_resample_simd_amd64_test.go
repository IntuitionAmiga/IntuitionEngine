//go:build amd64 && goexperiment.simd

package main

import (
	"bytes"
	"math/rand"
	"testing"
)

// TestResampleGatherSIMDMatchesScalarBitExact calls the SIMD kernel directly
// rather than through the dispatch variable, so the comparison holds whether
// or not the kernel is currently wired in assignSIMDKernels.
func TestResampleGatherSIMDMatchesScalarBitExact(t *testing.T) {
	rng := rand.New(rand.NewSource(31))
	widths := []int{1, 2, 7, 8, 9, 16, 17, 63, 64, 65, 128, 320, 321, 640, 800, 1024, 1920}
	for _, srcW := range widths {
		for _, rectW := range widths {
			plan := newResamplePlan(srcW, rectW)
			srcRow := resampleTestRow(srcW, rng)
			spans := [][2]int{{0, rectW}}
			for base := 0; base < rectW; base += compositorTileSize {
				spans = append(spans, [2]int{base, min(compositorTileSize, rectW-base)})
			}
			// A deliberately unaligned window, which must defer to scalar.
			if rectW > 3 {
				spans = append(spans, [2]int{1, rectW - 1})
			}
			for _, span := range spans {
				colBase, cols := span[0], span[1]
				got := make([]byte, cols*BYTES_PER_PIXEL)
				want := make([]byte, cols*BYTES_PER_PIXEL)
				compositorResampleRowSIMD(got, srcRow, plan, colBase, cols)
				compositorResampleRowScalar(want, srcRow, plan, colBase, cols)
				if !bytes.Equal(got, want) {
					t.Fatalf("srcW=%d rectW=%d colBase=%d cols=%d: SIMD gather differs from the scalar leaf",
						srcW, rectW, colBase, cols)
				}
			}
		}
	}
}
