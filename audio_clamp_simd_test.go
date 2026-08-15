//go:build goexperiment.simd && (amd64 || (linux && arm64))

package main

import (
	"math"
	"math/rand"
	"testing"
)

func TestClampF32SpanSIMDMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(127))
	lengths := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 256, 1024}
	edges := []float32{
		float32(math.NaN()),
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		0,
		math.Float32frombits(1),
		-math.Float32frombits(1),
		math.MaxFloat32,
		-math.MaxFloat32,
	}
	for _, n := range lengths {
		storage := make([]float32, n+1)
		base := storage[1:]
		for i := range base {
			if i < len(edges) {
				base[i] = edges[i]
			} else {
				base[i] = r.Float32()*4 - 2
			}
		}
		scalar := append([]float32(nil), base...)
		vector := append([]float32(nil), base...)
		clampF32SpanScalar(scalar, -1, 1)
		clampF32SpanSIMD(vector, -1, 1)
		for i := range scalar {
			if math.Float32bits(scalar[i]) != math.Float32bits(vector[i]) {
				t.Fatalf("length %d lane %d differs", n, i)
			}
		}
	}
}
