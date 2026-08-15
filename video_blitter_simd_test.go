//go:build goexperiment.simd && (amd64 || (linux && arm64))

package main

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

func TestSIMDFillUint32SpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for _, words := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 1023, 1024} {
		n := words * 4
		v := r.Uint32()
		a := make([]byte, n)
		b := make([]byte, n)
		// pre-dirty to prove full coverage
		for i := range a {
			a[i] = 0xAB
			b[i] = 0xAB
		}
		fillUint32LESpanScalar(a, v)
		fillUint32LESpanSIMD(b, v)
		if !bytes.Equal(a, b) {
			t.Fatalf("fill words=%d mismatch", words)
		}
		// spot-check LE encoding
		if words > 0 {
			if got := binary.LittleEndian.Uint32(b[:4]); got != v {
				t.Fatalf("fill LE encoding got %#08x want %#08x", got, v)
			}
		}
	}
}

func BenchmarkFillUint32Span(b *testing.B) {
	for _, name := range []struct {
		label string
		words int
	}{{"320px", 320}, {"1280px", 1280}} {
		dst := make([]byte, name.words*4)
		b.Run(name.label+"/scalar", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))
			for i := 0; i < b.N; i++ {
				fillUint32LESpanScalar(dst, 0xFF112233)
			}
		})
		b.Run(name.label+"/simd", func(b *testing.B) {
			b.SetBytes(int64(len(dst)))
			for i := 0; i < b.N; i++ {
				fillUint32LESpanSIMD(dst, 0xFF112233)
			}
		})
	}
}
