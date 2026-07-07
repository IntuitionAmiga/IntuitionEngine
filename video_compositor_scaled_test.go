package main

import (
	"encoding/binary"
	"math/rand"
	"testing"
)

// Reference models re-derive the original per-pixel scaled formulae so the
// hoisted-table / row-copy implementations can be proven byte-identical.

func refBlendScaled(final, src []byte, dstW, srcW, srcH int, rect scaleRect) {
	srcRowBytes := srcW * BYTES_PER_PIXEL
	dstRowBytes := dstW * BYTES_PER_PIXEL
	for dy := 0; dy < rect.h; dy++ {
		srcY := dy * srcH / rect.h
		srcRowOffset := srcY * srcRowBytes
		dstOffset := (rect.y+dy)*dstRowBytes + rect.x*BYTES_PER_PIXEL
		for dx := 0; dx < rect.w; dx++ {
			srcX := dx * srcW / rect.w
			sp := binary.LittleEndian.Uint32(src[srcRowOffset+srcX*BYTES_PER_PIXEL:])
			if pixel, ok := compositorOpaquePixel(sp); ok {
				binary.LittleEndian.PutUint32(final[dstOffset+dx*BYTES_PER_PIXEL:], pixel)
			}
		}
	}
}

func refCopyScaled(final, src []byte, dstW, srcW, srcH int, rect scaleRect) {
	srcRowBytes := srcW * BYTES_PER_PIXEL
	dstRowBytes := dstW * BYTES_PER_PIXEL
	for dy := 0; dy < rect.h; dy++ {
		srcY := dy * srcH / rect.h
		srcRowOffset := srcY * srcRowBytes
		dstOffset := (rect.y+dy)*dstRowBytes + rect.x*BYTES_PER_PIXEL
		for dx := 0; dx < rect.w; dx++ {
			srcX := dx * srcW / rect.w
			sp := binary.LittleEndian.Uint32(src[srcRowOffset+srcX*BYTES_PER_PIXEL:])
			binary.LittleEndian.PutUint32(final[dstOffset+dx*BYTES_PER_PIXEL:], sp|0xFF000000)
		}
	}
}

type scaledCase struct {
	srcW, srcH int
	dstW, dstH int
	rx, ry     int
	rw, rh     int
}

func scaledCases() []scaledCase {
	return []scaledCase{
		{64, 48, 320, 240, 0, 0, 320, 240},   // upscale, full
		{320, 240, 160, 120, 8, 4, 152, 116}, // downscale, offset rect
		{100, 100, 300, 200, 5, 7, 285, 190}, // non-integer + offset
		{320, 240, 320, 240, 0, 0, 320, 240}, // 1:1
		{17, 13, 200, 150, 3, 2, 191, 140},   // odd dims, offset
	}
}

func newScaledCompositor(dstW, dstH int) *VideoCompositor {
	return &VideoCompositor{
		frameWidth:  dstW,
		frameHeight: dstH,
		finalFrame:  make([]byte, dstW*dstH*BYTES_PER_PIXEL),
	}
}

func randScaledSrc(r *rand.Rand, w, h int) []byte {
	b := make([]byte, w*h*BYTES_PER_PIXEL)
	for i := 0; i+4 <= len(b); i += 4 {
		var v uint32
		switch r.Intn(3) {
		case 0:
			v = 0
		case 1:
			v = r.Uint32() & 0x00FFFFFF // zero alpha
		default:
			v = r.Uint32()
		}
		binary.LittleEndian.PutUint32(b[i:], v)
	}
	return b
}

func TestBlendFrameScaledMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i, tc := range scaledCases() {
		src := randScaledSrc(r, tc.srcW, tc.srcH)
		rect := scaleRect{x: tc.rx, y: tc.ry, w: tc.rw, h: tc.rh}

		// Identical random destination for both paths (blend is skip-write).
		dstInit := randScaledSrc(r, tc.dstW, tc.dstH)
		c := newScaledCompositor(tc.dstW, tc.dstH)
		copy(c.finalFrame, dstInit)
		c.blendFrameScaled(src, tc.srcW, tc.srcH, rect)

		want := make([]byte, len(dstInit))
		copy(want, dstInit)
		refBlendScaled(want, src, tc.dstW, tc.srcW, tc.srcH, rect)

		for j := range want {
			if c.finalFrame[j] != want[j] {
				t.Fatalf("case %d blend byte %d: got %#02x want %#02x", i, j, c.finalFrame[j], want[j])
			}
		}
	}
}

func TestCopyOpaqueFrameScaledMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i, tc := range scaledCases() {
		src := randScaledSrc(r, tc.srcW, tc.srcH)
		rect := scaleRect{x: tc.rx, y: tc.ry, w: tc.rw, h: tc.rh}
		c := newScaledCompositor(tc.dstW, tc.dstH)
		c.copyOpaqueFrameScaled(src, tc.srcW, tc.srcH, rect)

		want := make([]byte, tc.dstW*tc.dstH*BYTES_PER_PIXEL)
		refCopyScaled(want, src, tc.dstW, tc.srcW, tc.srcH, rect)

		for j := range want {
			if c.finalFrame[j] != want[j] {
				t.Fatalf("case %d copy byte %d: got %#02x want %#02x", i, j, c.finalFrame[j], want[j])
			}
		}
	}
}

func BenchmarkBlendFrameScaled(b *testing.B) {
	r := rand.New(rand.NewSource(3))
	src := randScaledSrc(r, 320, 240)
	c := newScaledCompositor(1280, 720)
	rect := scaleRect{x: 0, y: 0, w: 1280, h: 720}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.blendFrameScaled(src, 320, 240, rect)
	}
}

func BenchmarkCopyOpaqueFrameScaled(b *testing.B) {
	r := rand.New(rand.NewSource(4))
	src := randScaledSrc(r, 320, 240)
	c := newScaledCompositor(1280, 720)
	rect := scaleRect{x: 0, y: 0, w: 1280, h: 720}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.copyOpaqueFrameScaled(src, 320, 240, rect)
	}
}

// Reference (original per-pixel formula) benchmarks for before/after comparison.
func BenchmarkBlendFrameScaledRef(b *testing.B) {
	r := rand.New(rand.NewSource(3))
	src := randScaledSrc(r, 320, 240)
	final := make([]byte, 1280*720*BYTES_PER_PIXEL)
	rect := scaleRect{x: 0, y: 0, w: 1280, h: 720}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		refBlendScaled(final, src, 1280, 320, 240, rect)
	}
}

func BenchmarkCopyOpaqueFrameScaledRef(b *testing.B) {
	r := rand.New(rand.NewSource(4))
	src := randScaledSrc(r, 320, 240)
	final := make([]byte, 1280*720*BYTES_PER_PIXEL)
	rect := scaleRect{x: 0, y: 0, w: 1280, h: 720}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		refCopyScaled(final, src, 1280, 320, 240, rect)
	}
}
