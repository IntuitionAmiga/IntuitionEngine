package main

import (
	"encoding/binary"
	"testing"
)

// Characterisation of the compositor span pixel classes before the scalar-leaf
// extraction. Three classes drive every kernel:
//   - fully-zero  0x00000000 : blend skips (dst preserved), opaque-copy writes
//     0xFF000000, normalise leaves unchanged.
//   - zero-alpha  0x00RRGGBB : promoted to 0xFFRRGGBB by all three.
//   - alpha-set   0xAARRGGBB : blend/normalise keep as-is, opaque-copy forces
//     the alpha byte to 0xFF.

func putPixels(vals ...uint32) []byte {
	b := make([]byte, len(vals)*BYTES_PER_PIXEL)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(b[i*BYTES_PER_PIXEL:], v)
	}
	return b
}

func getPixels(b []byte) []uint32 {
	out := make([]uint32, len(b)/BYTES_PER_PIXEL)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(b[i*BYTES_PER_PIXEL:])
	}
	return out
}

func assertPixels(t *testing.T, got []byte, want ...uint32) {
	t.Helper()
	g := getPixels(got)
	if len(g) != len(want) {
		t.Fatalf("pixel count %d, want %d", len(g), len(want))
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("pixel %d = %#08x, want %#08x", i, g[i], want[i])
		}
	}
}

func TestCompositorBlendStripCharacterisation(t *testing.T) {
	const sentinel = 0xDEADBEEF
	src := putPixels(0x00000000, 0x00123456, 0x80112233, 0xFFAABBCC)
	c := &VideoCompositor{finalFrame: putPixels(sentinel, sentinel, sentinel, sentinel)}
	c.blendStrip(src, 4, 0, 1)
	// zero pixel: skip (sentinel kept); zero-alpha: promoted; alpha-set: kept.
	assertPixels(t, c.finalFrame, sentinel, 0xFF123456, 0x80112233, 0xFFAABBCC)
}

func TestCompositorOpaqueCopyCharacterisation(t *testing.T) {
	const sentinel = 0xDEADBEEF
	src := putPixels(0x00000000, 0x00123456, 0x80112233, 0xFFAABBCC)
	c := &VideoCompositor{finalFrame: putPixels(sentinel, sentinel, sentinel, sentinel)}
	c.copyOpaqueFrame1to1(src, 4, 1)
	// opaque copy always writes src|0xFF000000, never skips.
	assertPixels(t, c.finalFrame, 0xFF000000, 0xFF123456, 0xFF112233, 0xFFAABBCC)
}

func TestNormaliseFrameLeaseCharacterisation(t *testing.T) {
	pix := putPixels(0x00000000, 0x00123456, 0x80112233, 0xFFAABBCC)
	normaliseFrameLeaseAlphaRGBA(pix)
	// write-only-when-changed: only the zero-alpha nonzero-rgb pixel rewrites.
	assertPixels(t, pix, 0x00000000, 0xFF123456, 0x80112233, 0xFFAABBCC)
}
