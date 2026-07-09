//go:build amd64 && goexperiment.simd

package main

import (
	"bytes"
	"math/rand"
	"testing"
)

// Differential tests: the SIMD span kernels must be byte-identical to their
// scalar leaves for every length, pixel class, and buffer base. These call the
// scalar and SIMD leaves directly, never the ...Impl dispatch var.

var simdSpanByteLengths = []int{0, 4, 8, 28, 32, 4092, 4096}

// pixelClassPool spans the three compositor pixel classes plus mixed alpha.
var pixelClassPool = []uint32{
	0x00000000, // fully zero
	0x00123456, // zero alpha, nonzero rgb
	0x00FFFFFF, // zero alpha, max rgb
	0x01000000, // min nonzero alpha, zero rgb
	0x80112233, // mid alpha
	0xFFAABBCC, // full alpha
	0xFF000000, // full alpha, zero rgb
}

func randPixelBytes(r *rand.Rand, nBytes int) []byte {
	b := make([]byte, nBytes)
	for i := 0; i+BYTES_PER_PIXEL <= nBytes; i += BYTES_PER_PIXEL {
		var v uint32
		if r.Intn(2) == 0 {
			v = pixelClassPool[r.Intn(len(pixelClassPool))]
		} else {
			v = r.Uint32()
		}
		b[i] = byte(v)
		b[i+1] = byte(v >> 8)
		b[i+2] = byte(v >> 16)
		b[i+3] = byte(v >> 24)
	}
	return b
}

func TestSIMDCompositorBlendSpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range simdSpanByteLengths {
		src := randPixelBytes(r, n)
		dstBase := randPixelBytes(r, n)
		dScalar := append([]byte(nil), dstBase...)
		dSIMD := append([]byte(nil), dstBase...)
		compositorBlendSpanScalar(dScalar, src)
		compositorBlendSpanSIMD(dSIMD, src)
		if !bytes.Equal(dScalar, dSIMD) {
			t.Fatalf("blend len=%d mismatch", n)
		}
	}
}

func TestSIMDCompositorOpaqueCopySpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for _, n := range simdSpanByteLengths {
		src := randPixelBytes(r, n)
		dScalar := make([]byte, n)
		dSIMD := make([]byte, n)
		compositorOpaqueCopySpanScalar(dScalar, src)
		compositorOpaqueCopySpanSIMD(dSIMD, src)
		if !bytes.Equal(dScalar, dSIMD) {
			t.Fatalf("opaque copy len=%d mismatch", n)
		}
	}
}

func TestSIMDNormaliseFrameLeaseSpanMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, n := range simdSpanByteLengths {
		base := randPixelBytes(r, n)
		pScalar := append([]byte(nil), base...)
		pSIMD := append([]byte(nil), base...)
		normaliseFrameLeaseSpanScalar(pScalar)
		normaliseFrameLeaseSpanSIMD(pSIMD)
		if !bytes.Equal(pScalar, pSIMD) {
			t.Fatalf("normalise len=%d mismatch", n)
		}
	}
}

// Misaligned base: offset the whole buffer by one pixel so the vector base is
// not 32-byte aligned, exercising the unaligned load/store path.
func TestSIMDCompositorMisalignedBase(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	const pixels = 1024
	raw := randPixelBytes(r, (pixels+1)*BYTES_PER_PIXEL)
	src := raw[BYTES_PER_PIXEL:]
	dstBase := randPixelBytes(r, pixels*BYTES_PER_PIXEL)
	dScalar := append([]byte(nil), dstBase...)
	dSIMD := append([]byte(nil), dstBase...)
	compositorBlendSpanScalar(dScalar, src)
	compositorBlendSpanSIMD(dSIMD, src)
	if !bytes.Equal(dScalar, dSIMD) {
		t.Fatal("blend misaligned base mismatch")
	}
}

func TestSIMDCompositorRandomBuffers(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for iter := 0; iter < 1000; iter++ {
		pixels := r.Intn(300)
		n := pixels * BYTES_PER_PIXEL
		src := randPixelBytes(r, n)
		dstBase := randPixelBytes(r, n)

		bScalar := append([]byte(nil), dstBase...)
		bSIMD := append([]byte(nil), dstBase...)
		compositorBlendSpanScalar(bScalar, src)
		compositorBlendSpanSIMD(bSIMD, src)
		if !bytes.Equal(bScalar, bSIMD) {
			t.Fatalf("iter %d blend mismatch pixels=%d", iter, pixels)
		}

		cScalar := make([]byte, n)
		cSIMD := make([]byte, n)
		compositorOpaqueCopySpanScalar(cScalar, src)
		compositorOpaqueCopySpanSIMD(cSIMD, src)
		if !bytes.Equal(cScalar, cSIMD) {
			t.Fatalf("iter %d copy mismatch pixels=%d", iter, pixels)
		}

		nScalar := append([]byte(nil), src...)
		nSIMD := append([]byte(nil), src...)
		normaliseFrameLeaseSpanScalar(nScalar)
		normaliseFrameLeaseSpanSIMD(nSIMD)
		if !bytes.Equal(nScalar, nSIMD) {
			t.Fatalf("iter %d normalise mismatch pixels=%d", iter, pixels)
		}
	}
}
