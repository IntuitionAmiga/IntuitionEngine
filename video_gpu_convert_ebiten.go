// video_gpu_convert_ebiten.go - CLUT8 conversion on the GPU via a Kage shader.

//go:build !headless

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
The CPU converter expands one index byte into a four byte RGBA word before the
frame is uploaded, so a CLUT8 frame costs a full RGBA write on the CPU and a
full RGBA upload. Converting on the GPU moves the palette lookup into a fragment
shader and leaves only the index bytes to upload.

Ebiten has no single-channel texture, so four consecutive indices are packed
into the four channels of one RGBA texel and the index texture is a quarter of
the frame width, rounded up. The shader recovers the index by selecting the
channel from the destination x coordinate, scaling the channel back to 0..255
and rounding. The palette is a separate 256 by 1 texture, uploaded only when the
palette actually changes, which is the case that partial upload was for.

The CPU converter remains the canonical oracle. This path is selected only with
IE_VIDEO_GPU_CONVERT=1 and only where a shader-capable backend exists, and it is
gated by a render and readback differential against the CPU converter rather
than by the pure-Go mirror alone, because only a real render exercises the
compiled shader, the texture formats, the bindings and the coordinate rules.
*/

package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

// clut8IndicesPerTexel is how many index bytes one RGBA texel of the index
// texture carries.
const clut8IndicesPerTexel = 4

// clut8ConvertShaderSrc converts packed indices through a palette. Both live in
// one source image because Ebiten requires every source image bound to a shader
// draw to have the same size.
//
// The texture is exactly as wide as the packed index rows need. The palette
// occupies whole rows above them, wrapping across the width, so a narrow frame
// does not pay for a 256 texel wide texture it never reads. That matters
// because Ebiten cannot write part of an image: WritePixels replaces the whole
// texture, so every byte in it is uploaded every frame, and the layout is what
// keeps that payload down to the index bytes plus 256 palette texels.
//
// The channel scale back to an index has to round rather than truncate: the
// value has made a round trip through an 8 bit texture and a float, so the
// nearest integer is the index that was written and a truncation would pick its
// neighbour whenever the float lands a hair below.
const clut8ConvertShaderSrc = `//kage:unit pixels

package main

var DestOrigin vec2
var TexelsPerRow float
var PaletteRows float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	localX := floor(dstPos.x - DestOrigin.x)
	localY := floor(dstPos.y - DestOrigin.y)

	texelX := floor(localX / 4.0)
	channel := localX - texelX*4.0

	origin := imageSrc0Origin()
	packed := imageSrc0At(origin + vec2(texelX, localY+PaletteRows))
	value := packed.r
	if channel == 1.0 {
		value = packed.g
	} else if channel == 2.0 {
		value = packed.b
	} else if channel == 3.0 {
		value = packed.a
	}

	index := floor(value*255.0 + 0.5)
	palRow := floor(index / TexelsPerRow)
	palCol := index - palRow*TexelsPerRow
	return imageSrc0At(origin + vec2(palCol, palRow))
}
`

// clut8GPUConverter owns the textures and the shader for GPU CLUT8 conversion.
// Textures are retained across frames and only rebuilt when the geometry
// changes, so a steady state uploads the index bytes and nothing else.
type clut8GPUConverter struct {
	shader *ebiten.Shader
	// srcIm holds the palette in row 0 and the packed indices from row 1 on.
	// One image, because Ebiten requires every source image bound to a shader
	// draw to be the same size.
	srcIm *ebiten.Image
	out   *ebiten.Image

	width, height int
	texelsPerRow  int
	paletteRows   int

	srcBuf  []byte
	palBuf  []byte
	palSet  bool
	lastPal [256]uint32
}

// Convert uploads the indexed frame and palette as needed, runs the shader and
// returns the converted image, which the converter owns and reuses.
func (c *clut8GPUConverter) Convert(indices []byte, width, height int, pal *[256]uint32) (*ebiten.Image, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid frame dimensions %dx%d", width, height)
	}
	if len(indices) < width*height {
		return nil, fmt.Errorf("indexed frame holds %d bytes, need %d", len(indices), width*height)
	}
	if c.shader == nil {
		shader, err := ebiten.NewShader([]byte(clut8ConvertShaderSrc))
		if err != nil {
			return nil, fmt.Errorf("compile CLUT8 conversion shader: %w", err)
		}
		c.shader = shader
	}
	if width != c.width || height != c.height {
		c.resize(width, height)
	}

	if !c.palSet || c.lastPal != *pal {
		c.palBuf = buildCLUT8PaletteTexture(pal, c.palBuf)
		copy(c.srcBuf[:len(c.palBuf)], c.palBuf)
		c.lastPal = *pal
		c.palSet = true
	}
	c.packIndices(indices)
	c.srcIm.WritePixels(c.srcBuf)

	w := float32(c.width)
	h := float32(c.height)
	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: w, DstY: 0, SrcX: w, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 0, DstY: h, SrcX: 0, SrcY: h, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: w, DstY: h, SrcX: w, SrcY: h, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	op := &ebiten.DrawTrianglesShaderOptions{
		Blend: ebiten.BlendCopy,
		Uniforms: map[string]any{
			// The destination lives in an atlas, so its origin is not
			// necessarily zero and the shader works in local coordinates.
			"DestOrigin":   []float32{float32(c.out.Bounds().Min.X), float32(c.out.Bounds().Min.Y)},
			"TexelsPerRow": float32(c.texelsPerRow),
			"PaletteRows":  float32(c.paletteRows),
		},
	}
	op.Images[0] = c.srcIm
	c.out.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, c.shader, op)
	return c.out, nil
}

// resize rebuilds the retained textures for a new frame geometry.
func (c *clut8GPUConverter) resize(width, height int) {
	c.dispose()
	c.width, c.height = width, height
	c.texelsPerRow = (width + clut8IndicesPerTexel - 1) / clut8IndicesPerTexel
	c.paletteRows = (gpuPaletteTextureWidth + c.texelsPerRow - 1) / c.texelsPerRow
	c.srcIm = ebiten.NewImage(c.texelsPerRow, height+c.paletteRows)
	c.out = ebiten.NewImage(width, height)
	c.srcBuf = make([]byte, c.texelsPerRow*(height+c.paletteRows)*BYTES_PER_PIXEL)
	c.palSet = false
}

// packIndices lays the indexed frame out as RGBA texels, four indices per texel.
// A row whose width is not a multiple of four leaves the trailing channels of
// its last texel zero; the shader never reads them.
func (c *clut8GPUConverter) packIndices(indices []byte) {
	rowBytes := c.texelsPerRow * BYTES_PER_PIXEL
	for y := range c.height {
		src := indices[y*c.width : (y+1)*c.width]
		// The palette occupies the rows above, so the index rows start there.
		dst := c.srcBuf[(y+c.paletteRows)*rowBytes : (y+c.paletteRows+1)*rowBytes]
		copy(dst, src)
		for i := len(src); i < len(dst); i++ {
			dst[i] = 0
		}
	}
}

// dispose releases the retained textures.
func (c *clut8GPUConverter) dispose() {
	if c.srcIm != nil {
		c.srcIm.Deallocate()
		c.srcIm = nil
	}
	if c.out != nil {
		c.out.Deallocate()
		c.out = nil
	}
}
