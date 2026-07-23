// video_gpu_convert.go - GPU-side compatibility-format conversion support.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
Compatibility formats reach the compositor as RGBA32 because the video chip
converts them on the CPU. GPU conversion moves that work into a fragment shader:
the indexed frame is uploaded as a single-channel texture, the palette as a
256x1 texture, and the shader does the lookup while it draws.

This file holds the parts of that scheme that are independent of any GPU: the
palette texture layout, the index arithmetic the shader performs, and the
selection policy. The CPU converter stays the canonical oracle, so the pure-Go
mirror here is what the differential tests compare against; a real GPU render
and readback is a separate gate, and until it exists the shader path stays
opt-in.

Selection: GPU conversion needs the Ebiten backend, which rules out headless
builds. IE_VIDEO_GPU_CONVERT=1 opts in on the builds that can run it. See
sdk/docs/architecture.md, "Build Profiles and Observable Runtime".
*/

package main

import (
	"encoding/binary"
	"os"
)

// gpuPaletteTextureWidth is the width of the palette texture. A 256x1 RGBA
// texture holds the whole CLUT8 palette, one entry per texel, so the shader
// reads it with a single fetch at (index+0.5)/256.
const gpuPaletteTextureWidth = 256

// videoGPUConvertRequested reports whether GPU conversion has been asked for.
// It is a request, not the decision: gpuConversionAvailable settles that.
func videoGPUConvertRequested() bool {
	return os.Getenv("IE_VIDEO_GPU_CONVERT") == "1"
}

// gpuConvertSelection is the resolved conversion path for a build.
type gpuConvertSelection int

const (
	gpuConvertCPU gpuConvertSelection = iota
	gpuConvertShader
)

func (s gpuConvertSelection) String() string {
	if s == gpuConvertShader {
		return "shader"
	}
	return "cpu"
}

// selectGPUConversion resolves the conversion path. Headless builds have no
// Ebiten backend, so they always convert on the CPU whatever the switch says.
func selectGPUConversion(requested, backendSupportsShaders bool) gpuConvertSelection {
	if requested && backendSupportsShaders {
		return gpuConvertShader
	}
	return gpuConvertCPU
}

// buildCLUT8PaletteTexture packs a CLUT8 palette into the 256x1 RGBA texture
// the shader samples. Entries are already pre-packed little-endian RGBA words,
// as the CPU expander writes them, so the texture is byte-identical to what the
// CPU converter would have produced for a run of those indices.
func buildCLUT8PaletteTexture(pal *[256]uint32, dst []byte) []byte {
	need := gpuPaletteTextureWidth * BYTES_PER_PIXEL
	if cap(dst) < need {
		dst = make([]byte, need)
	}
	dst = dst[:need]
	for i := range gpuPaletteTextureWidth {
		binary.LittleEndian.PutUint32(dst[i*BYTES_PER_PIXEL:], pal[i])
	}
	return dst
}

// clut8IndexTexel returns the texel coordinate the shader uses to read the
// palette for an index byte. Texel centres are sampled, so a sample at
// (index+0.5)/256 can only land in that index's texel whatever the rounding.
func clut8IndexTexel(index uint8) float32 {
	return (float32(index) + 0.5) / float32(gpuPaletteTextureWidth)
}

// clut8PaletteIndexForTexel inverts clut8IndexTexel, i.e. it is the shader's
// texel-to-index arithmetic. Together the two pin that the round trip through
// the palette texture is exact for all 256 indices.
func clut8PaletteIndexForTexel(u float32) uint8 {
	i := int(u * float32(gpuPaletteTextureWidth))
	if i < 0 {
		i = 0
	}
	if i > 255 {
		i = 255
	}
	return uint8(i)
}

// convertCLUT8SpanViaShaderMirror is the pure-Go mirror of the CLUT8 fragment
// shader: for each source index it computes the palette texel, samples the
// palette texture and writes the result. It exists so the shader's arithmetic
// can be held against the CPU converter without a GPU. It is NOT the production
// path and is never wired into the pixel loop.
func convertCLUT8SpanViaShaderMirror(dst, src []byte, paletteTexture []byte) {
	for i := range src {
		u := clut8IndexTexel(src[i])
		texel := int(clut8PaletteIndexForTexel(u)) * BYTES_PER_PIXEL
		copy(dst[i*BYTES_PER_PIXEL:(i+1)*BYTES_PER_PIXEL], paletteTexture[texel:texel+BYTES_PER_PIXEL])
	}
}

// clut8KageShaderSource is the Kage fragment shader for CLUT8 conversion.
// Image 0 is the indexed frame, one index per texel in the red channel; image 1
// is the 256x1 palette texture built by buildCLUT8PaletteTexture. The index is
// recovered by scaling the red channel back to 0..255 and rounding, which is
// exact for the 8-bit texture formats Ebiten uploads.
//
// The shader is compiled by the Ebiten backend only. It has no effect until the
// render-and-readback differential gate in the tranche plan is in place, so it
// is currently reachable only with IE_VIDEO_GPU_CONVERT=1.
const clut8KageShaderSource = `//kage:unit pixels
package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	index := floor(imageSrc0At(src).r*255.0 + 0.5)
	u := (index + 0.5) / 256.0
	return imageSrc1At(imageSrc1Origin() + vec2(u*imageSrc1Size().x, 0.5))
}
`
