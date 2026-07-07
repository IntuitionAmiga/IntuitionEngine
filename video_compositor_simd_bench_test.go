//go:build amd64 && goexperiment.simd

package main

import (
	"math/rand"
	"testing"
)

func benchFrameSizes() []struct {
	name   string
	pixels int
} {
	return []struct {
		name   string
		pixels int
	}{
		{"320x240", 320 * 240},
		{"1280x720", 1280 * 720},
	}
}

func benchSrcDst(pixels int) (src, dst []byte) {
	r := rand.New(rand.NewSource(42))
	return randPixelBytes(r, pixels*BYTES_PER_PIXEL), make([]byte, pixels*BYTES_PER_PIXEL)
}

func BenchmarkCompositorBlendSpan(b *testing.B) {
	for _, sz := range benchFrameSizes() {
		src, dst := benchSrcDst(sz.pixels)
		b.Run(sz.name+"/scalar", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			for i := 0; i < b.N; i++ {
				compositorBlendSpanScalar(dst, src)
			}
		})
		b.Run(sz.name+"/simd", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			for i := 0; i < b.N; i++ {
				compositorBlendSpanSIMD(dst, src)
			}
		})
	}
}

func BenchmarkCompositorOpaqueCopySpan(b *testing.B) {
	for _, sz := range benchFrameSizes() {
		src, dst := benchSrcDst(sz.pixels)
		b.Run(sz.name+"/scalar", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			for i := 0; i < b.N; i++ {
				compositorOpaqueCopySpanScalar(dst, src)
			}
		})
		b.Run(sz.name+"/simd", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			for i := 0; i < b.N; i++ {
				compositorOpaqueCopySpanSIMD(dst, src)
			}
		})
	}
}

func BenchmarkCompositorNormaliseSpan(b *testing.B) {
	for _, sz := range benchFrameSizes() {
		src, _ := benchSrcDst(sz.pixels)
		b.Run(sz.name+"/scalar", func(b *testing.B) {
			buf := append([]byte(nil), src...)
			b.SetBytes(int64(len(buf)))
			for i := 0; i < b.N; i++ {
				normaliseFrameLeaseSpanScalar(buf)
			}
		})
		b.Run(sz.name+"/simd", func(b *testing.B) {
			buf := append([]byte(nil), src...)
			b.SetBytes(int64(len(buf)))
			for i := 0; i < b.N; i++ {
				normaliseFrameLeaseSpanSIMD(buf)
			}
		})
	}
}
