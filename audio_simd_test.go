//go:build amd64 && goexperiment.simd

package main

import (
	"math"
	"math/rand"
	"testing"
)

func f32bits(s []float32) []uint32 {
	out := make([]uint32, len(s))
	for i, v := range s {
		out[i] = math.Float32bits(v)
	}
	return out
}

func equalF32Bits(t *testing.T, a, b []float32, what string) {
	t.Helper()
	ba, bb := f32bits(a), f32bits(b)
	for i := range ba {
		if ba[i] != bb[i] {
			t.Fatalf("%s lane %d: scalar %#08x simd %#08x", what, i, ba[i], bb[i])
		}
	}
}

// audioEdgeSamples includes NaN, +/-Inf, denormals, zeros and out-of-range
// values to exercise the compare-blend clamp and the multiply.
func audioEdgeSamples(r *rand.Rand, n int) []float32 {
	pool := []float32{
		float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)),
		0, -0, 1, -1, 2, -2, math.SmallestNonzeroFloat32, -math.SmallestNonzeroFloat32,
		math.MaxFloat32, -math.MaxFloat32,
	}
	s := make([]float32, n)
	for i := range s {
		if r.Intn(3) == 0 {
			s[i] = pool[r.Intn(len(pool))]
		} else {
			s[i] = (r.Float32()*4 - 2)
		}
	}
	return s
}

func TestScaleF32SpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(31))
	for _, n := range []int{0, 1, 7, 8, 9, 256, 1024} {
		base := audioEdgeSamples(r, n)
		for _, gain := range []float32{0.5, 1.0, 2.0, -1.5, 0.0} {
			a := append([]float32(nil), base...)
			b := append([]float32(nil), base...)
			scaleF32SpanScalar(a, gain)
			scaleF32SpanSIMD(b, gain)
			equalF32Bits(t, a, b, "scale")
		}
	}
}

func TestClampF32SpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(32))
	for _, n := range []int{0, 1, 7, 8, 9, 256, 1024} {
		base := audioEdgeSamples(r, n)
		for _, rng := range []struct{ min, max float32 }{
			{-1, 1}, {-0.5, 0.5}, {0, 2},
		} {
			a := append([]float32(nil), base...)
			b := append([]float32(nil), base...)
			clampF32SpanScalar(a, rng.min, rng.max)
			clampF32SpanSIMD(b, rng.min, rng.max)
			equalF32Bits(t, a, b, "clamp")
		}
	}
}

func BenchmarkMasterGainSpan(b *testing.B) {
	r := rand.New(rand.NewSource(33))
	for _, n := range []int{256, 1024} {
		src := audioEdgeSamples(r, n)
		b.Run("scale/"+itoaBench(n)+"/scalar", func(b *testing.B) {
			buf := append([]float32(nil), src...)
			for i := 0; i < b.N; i++ {
				scaleF32SpanScalar(buf, 0.75)
			}
		})
		b.Run("scale/"+itoaBench(n)+"/simd", func(b *testing.B) {
			buf := append([]float32(nil), src...)
			for i := 0; i < b.N; i++ {
				scaleF32SpanSIMD(buf, 0.75)
			}
		})
		b.Run("clamp/"+itoaBench(n)+"/scalar", func(b *testing.B) {
			buf := append([]float32(nil), src...)
			for i := 0; i < b.N; i++ {
				clampF32SpanScalar(buf, -1, 1)
			}
		})
		b.Run("clamp/"+itoaBench(n)+"/simd", func(b *testing.B) {
			buf := append([]float32(nil), src...)
			for i := 0; i < b.N; i++ {
				clampF32SpanSIMD(buf, -1, 1)
			}
		})
	}
}

func itoaBench(n int) string {
	if n == 256 {
		return "256"
	}
	return "1024"
}
