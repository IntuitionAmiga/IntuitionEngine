// video_gpu_convert_test.go - GPU conversion layout, mirror and selection tests.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

func testCLUT8Palette(seed int64) *[256]uint32 {
	r := rand.New(rand.NewSource(seed))
	var pal [256]uint32
	for i := range pal {
		pal[i] = r.Uint32()
	}
	return &pal
}

func TestCLUT8PaletteTextureLayout(t *testing.T) {
	pal := testCLUT8Palette(17)
	tex := buildCLUT8PaletteTexture(pal, nil)
	if len(tex) != gpuPaletteTextureWidth*BYTES_PER_PIXEL {
		t.Fatalf("palette texture is %d bytes, want %d", len(tex), gpuPaletteTextureWidth*BYTES_PER_PIXEL)
	}
	for i := range gpuPaletteTextureWidth {
		got := binary.LittleEndian.Uint32(tex[i*BYTES_PER_PIXEL:])
		if got != pal[i] {
			t.Fatalf("texel %d = %#08x, want %#08x", i, got, pal[i])
		}
	}

	// Rebuilding into the same buffer reuses it rather than allocating.
	again := buildCLUT8PaletteTexture(pal, tex)
	if &again[0] != &tex[0] {
		t.Fatal("palette texture rebuild did not reuse the caller's buffer")
	}

	// Every index round-trips through the texel coordinate exactly, so no
	// index can ever sample a neighbouring entry.
	for i := range 256 {
		if got := clut8PaletteIndexForTexel(clut8IndexTexel(uint8(i))); got != uint8(i) {
			t.Fatalf("index %d round-tripped to %d", i, got)
		}
	}
}

// TestCLUT8ShaderIndexComputationMatchesCPU holds the shader's index arithmetic
// against the CPU expander, which is the canonical oracle for the format.
func TestCLUT8ShaderIndexComputationMatchesCPU(t *testing.T) {
	pal := testCLUT8Palette(4242)
	tex := buildCLUT8PaletteTexture(pal, nil)

	r := rand.New(rand.NewSource(99))
	for _, n := range []int{1, 7, 8, 255, 256, 1024, 320 * 200} {
		src := make([]byte, n)
		for i := range src {
			src[i] = byte(r.Intn(256))
		}
		// Also cover every index exactly once at the front of a long span.
		if n >= 256 {
			for i := range 256 {
				src[i] = byte(i)
			}
		}
		want := make([]byte, n*BYTES_PER_PIXEL)
		got := make([]byte, n*BYTES_PER_PIXEL)
		clut8ExpandSpanScalar(want, src, pal)
		convertCLUT8SpanViaShaderMirror(got, src, tex)
		if !bytes.Equal(want, got) {
			t.Fatalf("n=%d: the shader mirror differs from the CPU converter", n)
		}
	}
}

func TestGPUConversionFallback_HeadlessSelectsCPUPath(t *testing.T) {
	// Headless has no shader-capable backend, so the switch cannot select it.
	if got := selectGPUConversion(true, false); got != gpuConvertCPU {
		t.Fatalf("headless selection = %v, want cpu", got)
	}
	if got := selectGPUConversion(false, true); got != gpuConvertCPU {
		t.Fatalf("selection without the switch = %v, want cpu", got)
	}
	if got := selectGPUConversion(true, true); got != gpuConvertShader {
		t.Fatalf("selection with the switch on a shader backend = %v, want shader", got)
	}
}

func TestVideoGPUConvertSwitchIsOptIn(t *testing.T) {
	t.Setenv("IE_VIDEO_GPU_CONVERT", "")
	if videoGPUConvertRequested() {
		t.Fatal("GPU conversion requested with the switch unset")
	}
	t.Setenv("IE_VIDEO_GPU_CONVERT", "0")
	if videoGPUConvertRequested() {
		t.Fatal("GPU conversion requested with the switch off")
	}
	t.Setenv("IE_VIDEO_GPU_CONVERT", "1")
	if !videoGPUConvertRequested() {
		t.Fatal("GPU conversion not requested with the switch on")
	}
}
