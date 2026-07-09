//go:build amd64 && goexperiment.simd

package main

import (
	"bytes"
	"math/rand"
	"testing"
)

func makeCLUTPalette(r *rand.Rand) *[256]uint32 {
	var p [256]uint32
	for i := range p {
		p[i] = r.Uint32() | 0xFF000000
	}
	// pin palette edge values
	p[0] = 0xFF000000
	p[255] = 0xFFFFFFFF
	return &p
}

func TestCLUT8ExpandSpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(21))
	pal := makeCLUTPalette(r)
	for _, n := range []int{0, 1, 7, 8, 9, 255, 256, 1000} {
		src := make([]byte, n)
		for i := range src {
			src[i] = byte(r.Intn(256))
		}
		want := make([]byte, n*4)
		gotUnrolled := make([]byte, n*4)
		gotSIMD := make([]byte, n*4)
		clut8ExpandSpanScalar(want, src, pal)
		clut8ExpandSpanUnrolled(gotUnrolled, src, pal)
		clut8ExpandSpanSIMD(gotSIMD, src, pal)
		if !bytes.Equal(want, gotUnrolled) {
			t.Fatalf("unrolled n=%d mismatch", n)
		}
		if !bytes.Equal(want, gotSIMD) {
			t.Fatalf("simd n=%d mismatch", n)
		}
	}
}

func BenchmarkCLUT8Expand(b *testing.B) {
	r := rand.New(rand.NewSource(22))
	pal := makeCLUTPalette(r)
	for _, sz := range []struct {
		name   string
		pixels int
	}{{"320x240", 320 * 240}, {"640x480", 640 * 480}} {
		src := make([]byte, sz.pixels)
		for i := range src {
			src[i] = byte(r.Intn(256))
		}
		dst := make([]byte, sz.pixels*4)
		b.Run(sz.name+"/scalar", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))
			for i := 0; i < b.N; i++ {
				clut8ExpandSpanScalar(dst, src, pal)
			}
		})
		b.Run(sz.name+"/unrolled", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))
			for i := 0; i < b.N; i++ {
				clut8ExpandSpanUnrolled(dst, src, pal)
			}
		})
		b.Run(sz.name+"/simd", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))
			for i := 0; i < b.N; i++ {
				clut8ExpandSpanSIMD(dst, src, pal)
			}
		})
	}
}
