// voodoo_software.go - Software Rasterizer Backend for Voodoo Graphics

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine

License: GPLv3 or later
*/

/*
voodoo_software.go - Software Rasterizer Backend for Voodoo Graphics

Provides a pure-Go software rasterizer that implements the VoodooBackend interface:
- Barycentric triangle rasterization
- Z-buffering with all 8 compare functions
- Gouraud shading
- Scissor clipping
- Alpha blending
- Texture mapping with color combine
- Fog and dithering
*/

package main

import (
	"math"
	"sync"
	"unsafe"
)

// =============================================================================
// Software Rasterizer Backend
// =============================================================================

// VoodooSoftwareBackend implements software rasterization as a fallback
type VoodooSoftwareBackend struct {
	mutex sync.RWMutex

	// Framebuffer
	width, height int
	colorBuffer   []byte    // RGBA
	depthBuffer   []float32 // Z values

	// State
	fbzMode      uint32
	alphaMode    uint32
	chromaKey    uint32 // Chroma key color (RGB packed)
	fbzColorPath uint32 // Color combine mode
	colorPathSet bool   // Track if color path was explicitly set

	// Cached pipeline state (parsed from registers)
	pipelineKey PipelineKey

	// Scissor rectangle
	scissorLeft, scissorTop     int
	scissorRight, scissorBottom int

	// Double buffering
	frontBuffer []byte
	backBuffer  []byte
	isBackBuf   bool

	// Texture mapping
	textureData    []byte // RGBA texture data
	textureWidth   int
	textureHeight  int
	textureFormat  int
	textureEnabled bool
	textureClampS  bool
	textureClampT  bool

	// Fog state
	fogMode  uint32
	fogColor uint32
}

// NewVoodooSoftwareBackend creates a new software rasterizer backend
func NewVoodooSoftwareBackend() *VoodooSoftwareBackend {
	return &VoodooSoftwareBackend{}
}

// Init initializes the software backend with given dimensions
func (b *VoodooSoftwareBackend) Init(width, height int) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.width = width
	b.height = height

	// Allocate buffers
	pixelCount := width * height
	b.colorBuffer = make([]byte, pixelCount*4)
	b.depthBuffer = make([]float32, pixelCount)
	b.frontBuffer = make([]byte, pixelCount*4)
	b.backBuffer = make([]byte, pixelCount*4)

	// Initialize depth buffer to max depth
	for i := range b.depthBuffer {
		b.depthBuffer[i] = math.MaxFloat32
	}

	// Default scissor to full screen
	b.scissorLeft = 0
	b.scissorTop = 0
	b.scissorRight = width
	b.scissorBottom = height

	// Default state
	b.fbzMode = VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_LESS << 5)

	return nil
}

// UpdatePipelineState updates the rendering state
func (b *VoodooSoftwareBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.fbzMode = fbzMode
	b.alphaMode = alphaMode
	b.pipelineKey = PipelineKeyFromRegisters(fbzMode, alphaMode)
	return nil
}

// SetScissor sets the scissor rectangle
func (b *VoodooSoftwareBackend) SetScissor(left, top, right, bottom int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.scissorLeft = max(0, left)
	b.scissorTop = max(0, top)
	b.scissorRight = min(b.width, right)
	b.scissorBottom = min(b.height, bottom)
}

// SetChromaKey sets the chroma key color for transparency keying
func (b *VoodooSoftwareBackend) SetChromaKey(chromaKey uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.chromaKey = chromaKey
}

// SetTextureData uploads texture data for texture mapping
func (b *VoodooSoftwareBackend) SetTextureData(width, height int, data []byte, format int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.textureWidth = width
	b.textureHeight = height
	b.textureFormat = format

	// Copy texture data (assuming ARGB8888 format for now)
	b.textureData = make([]byte, len(data))
	copy(b.textureData, data)
}

// SetTextureEnabled enables or disables texture mapping
func (b *VoodooSoftwareBackend) SetTextureEnabled(enabled bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.textureEnabled = enabled
}

// SetTextureWrapMode sets texture coordinate wrap/clamp mode
func (b *VoodooSoftwareBackend) SetTextureWrapMode(clampS, clampT bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.textureClampS = clampS
	b.textureClampT = clampT
}

// SetColorPath sets the color combine mode from fbzColorPath register
func (b *VoodooSoftwareBackend) SetColorPath(fbzColorPath uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.fbzColorPath = fbzColorPath
	b.colorPathSet = true
}

// SetFogState sets the fog mode and color
func (b *VoodooSoftwareBackend) SetFogState(fogMode, fogColor uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.fogMode = fogMode
	b.fogColor = fogColor
}

// sampleTexture samples the texture at given UV coordinates
func (b *VoodooSoftwareBackend) sampleTexture(s, t float32) (r, g, bVal, a float32) {
	if b.textureData == nil || b.textureWidth == 0 || b.textureHeight == 0 {
		return 1.0, 1.0, 1.0, 1.0 // White if no texture
	}

	// Apply wrap/clamp mode
	if b.textureClampS {
		s = clampf(s, 0, 1)
	} else {
		// Wrap (repeat) mode - use fmod
		s = s - float32(math.Floor(float64(s)))
		if s < 0 {
			s += 1.0
		}
	}

	if b.textureClampT {
		t = clampf(t, 0, 1)
	} else {
		// Wrap (repeat) mode
		t = t - float32(math.Floor(float64(t)))
		if t < 0 {
			t += 1.0
		}
	}

	// Point sampling (nearest neighbor)
	texX := int(s * float32(b.textureWidth))
	texY := int(t * float32(b.textureHeight))

	// Clamp to texture bounds
	if texX >= b.textureWidth {
		texX = b.textureWidth - 1
	}
	if texY >= b.textureHeight {
		texY = b.textureHeight - 1
	}
	if texX < 0 {
		texX = 0
	}
	if texY < 0 {
		texY = 0
	}

	// Sample texel (assuming RGBA format)
	idx := (texY*b.textureWidth + texX) * 4
	if idx+3 < len(b.textureData) {
		r = float32(b.textureData[idx+0]) / 255.0
		g = float32(b.textureData[idx+1]) / 255.0
		bVal = float32(b.textureData[idx+2]) / 255.0
		a = float32(b.textureData[idx+3]) / 255.0
	} else {
		r, g, bVal, a = 1.0, 1.0, 1.0, 1.0
	}

	return r, g, bVal, a
}

// combineColors combines vertex and texture colors based on fbzColorPath register
func (b *VoodooSoftwareBackend) combineColors(vertR, vertG, vertB, vertA, texR, texG, texB, texA float32) (r, g, bVal, a float32) {
	// Default to modulate for backward compatibility (if color path was never explicitly set)
	if !b.colorPathSet {
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}

	// Handle special convenience modes first (these have specific bit patterns)
	switch b.fbzColorPath {
	case VOODOO_COMBINE_ADD:
		return vertR + texR, vertG + texG, vertB + texB, vertA + texA
	case VOODOO_COMBINE_MODULATE:
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}

	// Extract color combine mode from fbzColorPath
	rgbSelect := b.fbzColorPath & VOODOO_FCP_RGB_SELECT_MASK

	switch rgbSelect {
	case VOODOO_CC_ITERATED:
		return vertR, vertG, vertB, vertA
	case VOODOO_CC_TEXTURE:
		return texR, texG, texB, texA
	default:
		ccMode := (b.fbzColorPath >> VOODOO_FCP_CC_MSELECT_SHIFT) & 0x7
		switch ccMode {
		case VOODOO_CC_ZERO:
			return 0, 0, 0, 0
		case VOODOO_CC_CSUB_CL:
			return texR - vertR, texG - vertG, texB - vertB, texA - vertA
		case VOODOO_CC_CLOCAL:
			return vertR, vertG, vertB, vertA
		case VOODOO_CC_CLOC_MUL:
			return vertR * texR, vertG * texG, vertB * texB, vertA * texA
		default:
			return vertR * texR, vertG * texG, vertB * texB, vertA * texA
		}
	}
}

// FlushTriangles rasterizes all triangles in the batch
func (b *VoodooSoftwareBackend) FlushTriangles(triangles []VoodooTriangle) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, tri := range triangles {
		b.rasterizeTriangle(&tri)
	}
}

// ClearFramebuffer clears the color and depth buffers
func (b *VoodooSoftwareBackend) ClearFramebuffer(color uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Extract RGBA from packed color (assuming ARGB format)
	r := byte((color >> 16) & 0xFF)
	g := byte((color >> 8) & 0xFF)
	bVal := byte(color & 0xFF)
	a := byte((color >> 24) & 0xFF)
	if a == 0 {
		a = 255 // Default to opaque
	}

	// Clear color buffer
	for i := 0; i < len(b.colorBuffer); i += 4 {
		b.colorBuffer[i+0] = r
		b.colorBuffer[i+1] = g
		b.colorBuffer[i+2] = bVal
		b.colorBuffer[i+3] = a
	}

	// Clear depth buffer based on depth function
	depthFunc := b.pipelineKey.DepthCompareOp
	var depthClearValue float32
	switch depthFunc {
	case VOODOO_DEPTH_GREATER, VOODOO_DEPTH_GREATEREQUAL:
		depthClearValue = 0.0
	default:
		depthClearValue = math.MaxFloat32
	}

	for i := range b.depthBuffer {
		b.depthBuffer[i] = depthClearValue
	}
}

// SwapBuffers swaps front and back buffers
func (b *VoodooSoftwareBackend) SwapBuffers(waitVSync bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Copy color buffer to front buffer
	copy(b.frontBuffer, b.colorBuffer)
}

// GetFrame returns the current front buffer
func (b *VoodooSoftwareBackend) GetFrame() []byte {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.frontBuffer
}

// Destroy cleans up resources
func (b *VoodooSoftwareBackend) Destroy() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.colorBuffer = nil
	b.depthBuffer = nil
	b.frontBuffer = nil
	b.backBuffer = nil
}

// rasterizeTriangle performs software triangle rasterization
func (b *VoodooSoftwareBackend) rasterizeTriangle(tri *VoodooTriangle) {
	v0 := &tri.Vertices[0]
	v1 := &tri.Vertices[1]
	v2 := &tri.Vertices[2]

	// Check if clipping is enabled
	enableClipping := (b.fbzMode & VOODOO_FBZ_CLIPPING) != 0

	// Compute bounding box
	minX := int(math.Floor(float64(min3f(v0.X, v1.X, v2.X))))
	maxX := int(math.Ceil(float64(max3f(v0.X, v1.X, v2.X))))
	minY := int(math.Floor(float64(min3f(v0.Y, v1.Y, v2.Y))))
	maxY := int(math.Ceil(float64(max3f(v0.Y, v1.Y, v2.Y))))

	// Clip to screen bounds
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > b.width {
		maxX = b.width
	}
	if maxY > b.height {
		maxY = b.height
	}

	// Clip to scissor rectangle if enabled
	if enableClipping {
		if minX < b.scissorLeft {
			minX = b.scissorLeft
		}
		if minY < b.scissorTop {
			minY = b.scissorTop
		}
		if maxX > b.scissorRight {
			maxX = b.scissorRight
		}
		if maxY > b.scissorBottom {
			maxY = b.scissorBottom
		}
	}

	// Compute triangle area (2x for efficiency)
	area := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, v2.X, v2.Y)
	if area == 0 {
		return // Degenerate triangle
	}

	// Handle backface culling (if area is negative, triangle is back-facing)
	if area < 0 {
		// Swap vertices to make it front-facing
		v0, v2 = v2, v0
		area = -area
	}

	invArea := 1.0 / area

	// Check depth test settings
	depthEnable := (b.fbzMode & VOODOO_FBZ_DEPTH_ENABLE) != 0
	depthWrite := (b.fbzMode & VOODOO_FBZ_DEPTH_WRITE) != 0
	rgbWrite := (b.fbzMode & VOODOO_FBZ_RGB_WRITE) != 0
	depthFunc := int((b.fbzMode >> 5) & 0x7)

	// Check alpha test settings
	alphaTestEnable := (b.alphaMode & VOODOO_ALPHA_TEST_EN) != 0
	alphaTestFunc := int((b.alphaMode >> 1) & 0x7)
	alphaTestRef := float32((b.alphaMode>>24)&0xFF) / 255.0

	// Check chroma key settings
	chromaKeyEnable := (b.fbzMode & VOODOO_FBZ_CHROMAKEY) != 0

	// Check alpha blending
	alphaBlendEnable := (b.alphaMode & VOODOO_ALPHA_BLEND_EN) != 0

	// Check fog settings
	fogEnable := (b.fogMode & VOODOO_FOG_ENABLE) != 0
	var fogR, fogG, fogB float32
	if fogEnable {
		fogR = float32((b.fogColor>>16)&0xFF) / 255.0
		fogG = float32((b.fogColor>>8)&0xFF) / 255.0
		fogB = float32(b.fogColor&0xFF) / 255.0
	}

	// Check dithering settings
	ditherEnable := (b.fbzMode & VOODOO_FBZ_DITHER) != 0
	dither2x2 := (b.fbzMode & VOODOO_FBZ_DITHER_2X2) != 0

	// Rasterize - optimized with row base precomputation
	for y := minY; y < maxY; y++ {
		rowBase := y * b.width
		py := float32(y) + 0.5

		for x := minX; x < maxX; x++ {
			px := float32(x) + 0.5

			// Compute barycentric coordinates
			w0 := edgeFunction(v1.X, v1.Y, v2.X, v2.Y, px, py)
			w1 := edgeFunction(v2.X, v2.Y, v0.X, v0.Y, px, py)
			w2 := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, px, py)

			// Check if pixel is inside triangle
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				// Normalize barycentric coordinates
				w0 *= invArea
				w1 *= invArea
				w2 *= invArea

				// Interpolate Z
				z := w0*v0.Z + w1*v1.Z + w2*v2.Z

				// Depth test
				pixelIndex := rowBase + x
				if depthEnable {
					oldZ := b.depthBuffer[pixelIndex]
					if !b.depthTest(z, oldZ, depthFunc) {
						continue
					}
				}

				// Interpolate color (Gouraud shading)
				r := w0*v0.R + w1*v1.R + w2*v2.R
				g := w0*v0.G + w1*v1.G + w2*v2.G
				bVal := w0*v0.B + w1*v1.B + w2*v2.B
				a := w0*v0.A + w1*v1.A + w2*v2.A

				// Texture mapping with color combine
				if b.textureEnabled && b.textureData != nil {
					s := w0*v0.S + w1*v1.S + w2*v2.S
					t := w0*v0.T + w1*v1.T + w2*v2.T

					texR, texG, texB, texA := b.sampleTexture(s, t)
					r, g, bVal, a = b.combineColors(r, g, bVal, a, texR, texG, texB, texA)
				}

				// Clamp colors
				r = clampf(r, 0, 1)
				g = clampf(g, 0, 1)
				bVal = clampf(bVal, 0, 1)
				a = clampf(a, 0, 1)

				// Alpha test (discard if fails)
				if alphaTestEnable {
					if !b.alphaTest(a, alphaTestRef, alphaTestFunc) {
						continue
					}
				}

				// Chroma key test (discard if matches key color)
				if chromaKeyEnable {
					if b.chromaKeyTest(r, g, bVal) {
						continue
					}
				}

				// Fog blending
				if fogEnable {
					fogFactor := clampf(z, 0, 1)
					r = r*(1-fogFactor) + fogR*fogFactor
					g = g*(1-fogFactor) + fogG*fogFactor
					bVal = bVal*(1-fogFactor) + fogB*fogFactor
					r = clampf(r, 0, 1)
					g = clampf(g, 0, 1)
					bVal = clampf(bVal, 0, 1)
				}

				// Dithering
				if ditherEnable {
					threshold := b.getDitherThreshold(x, y, dither2x2)
					r = b.applyDither(r, threshold)
					g = b.applyDither(g, threshold)
					bVal = b.applyDither(bVal, threshold)
				}

				// Write pixel
				if rgbWrite {
					bufIdx := pixelIndex * 4
					if alphaBlendEnable {
						srcR, srcG, srcB, srcA := r, g, bVal, a
						const inv255 = float32(1.0 / 255.0)
						dstR := float32(b.colorBuffer[bufIdx+0]) * inv255
						dstG := float32(b.colorBuffer[bufIdx+1]) * inv255
						dstB := float32(b.colorBuffer[bufIdx+2]) * inv255
						dstA := float32(b.colorBuffer[bufIdx+3]) * inv255

						srcFactor := b.getBlendFactor(b.pipelineKey.SrcBlendFactor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)
						dstFactor := b.getBlendFactor(b.pipelineKey.DstBlendFactor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)

						outR := clampf(srcR*srcFactor+dstR*dstFactor, 0, 1)
						outG := clampf(srcG*srcFactor+dstG*dstFactor, 0, 1)
						outB := clampf(srcB*srcFactor+dstB*dstFactor, 0, 1)
						outA := clampf(srcA*srcFactor+dstA*dstFactor, 0, 1)

						packed := uint32(outR*255) | uint32(outG*255)<<8 | uint32(outB*255)<<16 | uint32(outA*255)<<24
						*(*uint32)(unsafe.Pointer(&b.colorBuffer[bufIdx])) = packed
					} else {
						packed := uint32(r*255) | uint32(g*255)<<8 | uint32(bVal*255)<<16 | uint32(a*255)<<24
						*(*uint32)(unsafe.Pointer(&b.colorBuffer[bufIdx])) = packed
					}
				}

				// Write depth
				if depthEnable && depthWrite {
					b.depthBuffer[pixelIndex] = z
				}
			}
		}
	}
}

// depthTest performs depth comparison
func (b *VoodooSoftwareBackend) depthTest(newZ, oldZ float32, depthFunc int) bool {
	switch depthFunc {
	case VOODOO_DEPTH_NEVER:
		return false
	case VOODOO_DEPTH_LESS:
		return newZ < oldZ
	case VOODOO_DEPTH_EQUAL:
		return newZ == oldZ
	case VOODOO_DEPTH_LESSEQUAL:
		return newZ <= oldZ
	case VOODOO_DEPTH_GREATER:
		return newZ > oldZ
	case VOODOO_DEPTH_NOTEQUAL:
		return newZ != oldZ
	case VOODOO_DEPTH_GREATEREQUAL:
		return newZ >= oldZ
	case VOODOO_DEPTH_ALWAYS:
		return true
	}
	return true
}

// alphaTest performs alpha comparison (same functions as depth test)
func (b *VoodooSoftwareBackend) alphaTest(alphaValue, alphaRef float32, alphaFunc int) bool {
	switch alphaFunc {
	case VOODOO_ALPHA_NEVER:
		return false
	case VOODOO_ALPHA_LESS:
		return alphaValue < alphaRef
	case VOODOO_ALPHA_EQUAL:
		return alphaValue == alphaRef
	case VOODOO_ALPHA_LESSEQUAL:
		return alphaValue <= alphaRef
	case VOODOO_ALPHA_GREATER:
		return alphaValue > alphaRef
	case VOODOO_ALPHA_NOTEQUAL:
		return alphaValue != alphaRef
	case VOODOO_ALPHA_GREATEREQUAL:
		return alphaValue >= alphaRef
	case VOODOO_ALPHA_ALWAYS:
		return true
	}
	return true
}

// abs32 returns absolute value of float32 without float64 conversion
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// chromaKeyTest checks if a color matches the chroma key (returns true if should discard)
func (b *VoodooSoftwareBackend) chromaKeyTest(r, g, bVal float32) bool {
	const inv255 = float32(1.0 / 255.0)
	keyR := float32((b.chromaKey>>16)&0xFF) * inv255
	keyG := float32((b.chromaKey>>8)&0xFF) * inv255
	keyB := float32(b.chromaKey&0xFF) * inv255

	const tolerance = inv255

	rMatch := abs32(r-keyR) <= tolerance
	gMatch := abs32(g-keyG) <= tolerance
	bMatch := abs32(bVal-keyB) <= tolerance

	return rMatch && gMatch && bMatch
}

// bayer4x4Flat is a flattened 4x4 Bayer ordered dither matrix (normalized 0.0-1.0)
var bayer4x4Flat = [16]float32{
	0.0 / 16.0, 8.0 / 16.0, 2.0 / 16.0, 10.0 / 16.0,
	12.0 / 16.0, 4.0 / 16.0, 14.0 / 16.0, 6.0 / 16.0,
	3.0 / 16.0, 11.0 / 16.0, 1.0 / 16.0, 9.0 / 16.0,
	15.0 / 16.0, 7.0 / 16.0, 13.0 / 16.0, 5.0 / 16.0,
}

// bayer2x2Flat is a flattened 2x2 Bayer ordered dither matrix
var bayer2x2Flat = [4]float32{
	0.0 / 4.0, 2.0 / 4.0,
	3.0 / 4.0, 1.0 / 4.0,
}

// getDitherThreshold returns the dither threshold for a given pixel position
func (b *VoodooSoftwareBackend) getDitherThreshold(x, y int, use2x2 bool) float32 {
	if use2x2 {
		return bayer2x2Flat[(y&1)<<1|(x&1)]
	}
	return bayer4x4Flat[(y&3)<<2|(x&3)]
}

// applyDither applies ordered dithering to a color value
func (b *VoodooSoftwareBackend) applyDither(value, threshold float32) float32 {
	colorLevel := value * 255.0
	ditherOffset := threshold - 0.5
	colorLevel += ditherOffset

	quantized := float32(int(colorLevel+0.5)) / 255.0
	return clampf(quantized, 0, 1)
}

// inv3 is precomputed 1/3 to avoid division in getBlendFactor
const inv3 = float32(1.0 / 3.0)

// getBlendFactor calculates the blend factor value based on Voodoo blend mode
func (b *VoodooSoftwareBackend) getBlendFactor(factor int, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA float32) float32 {
	switch factor {
	case VOODOO_BLEND_ZERO:
		return 0.0
	case VOODOO_BLEND_SRC_ALPHA:
		return srcA
	case VOODOO_BLEND_COLOR:
		return (srcR + srcG + srcB) * inv3
	case VOODOO_BLEND_DST_ALPHA:
		return dstA
	case VOODOO_BLEND_ONE:
		return 1.0
	case VOODOO_BLEND_INV_SRC_A:
		return 1.0 - srcA
	case VOODOO_BLEND_INV_COLOR:
		return 1.0 - (srcR+srcG+srcB)*inv3
	case VOODOO_BLEND_INV_DST_A:
		return 1.0 - dstA
	case VOODOO_BLEND_SATURATE:
		invDstA := 1.0 - dstA
		if srcA < invDstA {
			return srcA
		}
		return invDstA
	}
	return 1.0
}

// edgeFunction computes the signed area of a parallelogram
func edgeFunction(ax, ay, bx, by, cx, cy float32) float32 {
	return (cx-ax)*(by-ay) - (cy-ay)*(bx-ax)
}

// Helper functions
func min3f(a, b, c float32) float32 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func max3f(a, b, c float32) float32 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}

func clampf(v, minVal, maxVal float32) float32 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}
