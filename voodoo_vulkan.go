//go:build !headless

// voodoo_vulkan.go - Vulkan Backend for Voodoo Graphics

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
voodoo_vulkan.go - Vulkan Rendering Backend for Voodoo Graphics

This file provides both the Vulkan backend implementation for hardware-accelerated
3D rendering and a software rasterizer fallback.

Vulkan Backend:
- Offscreen rendering (no window/swapchain needed)
- Dynamic vertex buffer for triangle batches
- Configurable depth test and alpha blending via pipeline recreation
- Frame readback via staging buffer for compositor integration

Software Backend:
- Barycentric triangle rasterization
- Z-buffering with all 8 compare functions
- Gouraud shading
- Scissor clipping
- Alpha blending
*/

package main

import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	vk "github.com/goki/vulkan"
)

// =============================================================================
// Pipeline Key and Caching (Phase 2: Dynamic Pipeline State)
// =============================================================================

// PipelineKey uniquely identifies a pipeline configuration for caching
type PipelineKey struct {
	DepthTestEnable  bool
	DepthWriteEnable bool
	DepthCompareOp   int // Voodoo depth function (0-7)
	BlendEnable      bool
	SrcBlendFactor   int // Voodoo source blend factor
	DstBlendFactor   int // Voodoo destination blend factor
}

// VoodooPushConstants contains per-draw state passed to shaders via push constants
// Phase 3: Alpha Test & Chroma Key support
// Phase 4: Texture mode support
// Phase 5: Color combine (fbzColorPath) support
// Phase 6: Fog support
type VoodooPushConstants struct {
	FbzMode      uint32 // Framebuffer Z mode (contains chroma key enable flag, dither enable)
	AlphaMode    uint32 // Alpha test mode (enable, function, reference value)
	ChromaKey    uint32 // Chroma key color (RGB packed)
	TextureMode  uint32 // Phase 4: bit 0 = texture enable
	FbzColorPath uint32 // Phase 5: Color combine mode
	FogMode      uint32 // Phase 6: Fog mode (bit 0 = enable)
	FogColor     uint32 // Phase 6: Fog color (RGB packed)
}

// PipelineKeyFromRegisters creates a PipelineKey from fbzMode and alphaMode registers
func PipelineKeyFromRegisters(fbzMode, alphaMode uint32) PipelineKey {
	key := PipelineKey{
		DepthTestEnable:  (fbzMode & VOODOO_FBZ_DEPTH_ENABLE) != 0,
		DepthWriteEnable: (fbzMode & VOODOO_FBZ_DEPTH_WRITE) != 0,
		DepthCompareOp:   int((fbzMode >> 5) & 0x7),
		BlendEnable:      (alphaMode & VOODOO_ALPHA_BLEND_EN) != 0,
		SrcBlendFactor:   VOODOO_BLEND_ONE,  // Default
		DstBlendFactor:   VOODOO_BLEND_ZERO, // Default
	}

	if key.BlendEnable {
		key.SrcBlendFactor = int((alphaMode >> 8) & 0xF)
		key.DstBlendFactor = int((alphaMode >> 12) & 0xF)
	}

	return key
}

// VoodooDepthFuncToVulkan maps Voodoo depth function to Vulkan VkCompareOp
// Voodoo: 0=NEVER, 1=LESS, 2=EQUAL, 3=LESSEQUAL, 4=GREATER, 5=NOTEQUAL, 6=GREATEREQUAL, 7=ALWAYS
// Vulkan: 0=NEVER, 1=LESS, 2=EQUAL, 3=LESS_OR_EQUAL, 4=GREATER, 5=NOT_EQUAL, 6=GREATER_OR_EQUAL, 7=ALWAYS
// The mappings are identical!
func VoodooDepthFuncToVulkan(voodooFunc int) int {
	// Direct mapping - Voodoo and Vulkan use the same ordering
	if voodooFunc < 0 || voodooFunc > 7 {
		return 7 // ALWAYS as fallback
	}
	return voodooFunc
}

// VoodooBlendFactorToVulkan maps Voodoo blend factor to Vulkan VkBlendFactor
// Voodoo blend factors:
//
//	0 = ZERO
//	1 = SRC_ALPHA
//	2 = COLOR (constant)
//	3 = DST_ALPHA
//	4 = ONE
//	5 = INV_SRC_ALPHA (1-srcA)
//	6 = INV_COLOR
//	7 = INV_DST_ALPHA (1-dstA)
//	15 = SATURATE
//
// Vulkan VkBlendFactor:
//
//	0 = ZERO
//	1 = ONE
//	2 = SRC_COLOR
//	3 = ONE_MINUS_SRC_COLOR
//	4 = DST_COLOR
//	5 = ONE_MINUS_DST_COLOR
//	6 = SRC_ALPHA
//	7 = ONE_MINUS_SRC_ALPHA
//	8 = DST_ALPHA
//	9 = ONE_MINUS_DST_ALPHA
//	10 = CONSTANT_COLOR
//	11 = ONE_MINUS_CONSTANT_COLOR
//	12 = CONSTANT_ALPHA
//	13 = ONE_MINUS_CONSTANT_ALPHA
//	14 = SRC_ALPHA_SATURATE
func VoodooBlendFactorToVulkan(voodooFactor int) int {
	switch voodooFactor {
	case VOODOO_BLEND_ZERO: // 0
		return 0 // VK_BLEND_FACTOR_ZERO
	case VOODOO_BLEND_SRC_ALPHA: // 1
		return 6 // VK_BLEND_FACTOR_SRC_ALPHA
	case VOODOO_BLEND_COLOR: // 2
		return 10 // VK_BLEND_FACTOR_CONSTANT_COLOR
	case VOODOO_BLEND_DST_ALPHA: // 3
		return 8 // VK_BLEND_FACTOR_DST_ALPHA
	case VOODOO_BLEND_ONE: // 4
		return 1 // VK_BLEND_FACTOR_ONE
	case VOODOO_BLEND_INV_SRC_A: // 5
		return 7 // VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA
	case VOODOO_BLEND_INV_COLOR: // 6
		return 11 // VK_BLEND_FACTOR_ONE_MINUS_CONSTANT_COLOR
	case VOODOO_BLEND_INV_DST_A: // 7
		return 9 // VK_BLEND_FACTOR_ONE_MINUS_DST_ALPHA
	case VOODOO_BLEND_SATURATE: // 15
		return 14 // VK_BLEND_FACTOR_SRC_ALPHA_SATURATE
	default:
		return 1 // VK_BLEND_FACTOR_ONE as fallback
	}
}

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
	chromaKey    uint32 // Phase 3: Chroma key color (RGB packed)
	fbzColorPath uint32 // Phase 5: Color combine mode
	colorPathSet bool   // Phase 5: Track if color path was explicitly set

	// Cached pipeline state (parsed from registers)
	pipelineKey PipelineKey

	// Scissor rectangle
	scissorLeft, scissorTop     int
	scissorRight, scissorBottom int

	// Double buffering
	frontBuffer []byte
	backBuffer  []byte
	isBackBuf   bool

	// Phase 4: Texture mapping
	textureData    []byte // RGBA texture data
	textureWidth   int
	textureHeight  int
	textureFormat  int
	textureEnabled bool
	textureClampS  bool
	textureClampT  bool

	// Phase 6: Fog state
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
// Phase 3: Chroma Key support
func (b *VoodooSoftwareBackend) SetChromaKey(chromaKey uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.chromaKey = chromaKey
}

// SetTextureData uploads texture data for texture mapping
// Phase 4: Texture Mapping support
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
// Phase 4: Texture Mapping support
func (b *VoodooSoftwareBackend) SetTextureEnabled(enabled bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.textureEnabled = enabled
}

// SetTextureWrapMode sets texture coordinate wrap/clamp mode
// Phase 4: Texture Mapping support
func (b *VoodooSoftwareBackend) SetTextureWrapMode(clampS, clampT bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.textureClampS = clampS
	b.textureClampT = clampT
}

// SetColorPath sets the color combine mode from fbzColorPath register
// Phase 5: Color Combine support
func (b *VoodooSoftwareBackend) SetColorPath(fbzColorPath uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.fbzColorPath = fbzColorPath
	b.colorPathSet = true // Mark that color path was explicitly set
}

// SetFogState sets the fog mode and color
// Phase 6: Fog support
func (b *VoodooSoftwareBackend) SetFogState(fogMode, fogColor uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.fogMode = fogMode
	b.fogColor = fogColor
}

// sampleTexture samples the texture at given UV coordinates
// Phase 4: Texture Mapping support
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
// Phase 5: Color Combine support
func (b *VoodooSoftwareBackend) combineColors(vertR, vertG, vertB, vertA, texR, texG, texB, texA float32) (r, g, bVal, a float32) {
	// Default to modulate for backward compatibility (if color path was never explicitly set)
	if !b.colorPathSet {
		// Default behavior: modulate (tex * vert) for backward compatibility
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}

	// Handle special convenience modes first (these have specific bit patterns)
	switch b.fbzColorPath {
	case VOODOO_COMBINE_ADD:
		// ADD mode: tex + vert clamped
		return vertR + texR, vertG + texG, vertB + texB, vertA + texA
	case VOODOO_COMBINE_MODULATE:
		// Explicit MODULATE mode
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}

	// Extract color combine mode from fbzColorPath
	rgbSelect := b.fbzColorPath & VOODOO_FCP_RGB_SELECT_MASK

	switch rgbSelect {
	case VOODOO_CC_ITERATED:
		// Use vertex color only (ignore texture)
		return vertR, vertG, vertB, vertA
	case VOODOO_CC_TEXTURE:
		// Use texture color only (ignore vertex)
		return texR, texG, texB, texA
	default:
		// Apply combine mode based on CC_MSELECT bits
		ccMode := (b.fbzColorPath >> VOODOO_FCP_CC_MSELECT_SHIFT) & 0x7
		switch ccMode {
		case VOODOO_CC_ZERO:
			// Output zero (black)
			return 0, 0, 0, 0
		case VOODOO_CC_CSUB_CL:
			// cother - clocal (subtract)
			return texR - vertR, texG - vertG, texB - vertB, texA - vertA
		case VOODOO_CC_CLOCAL:
			// clocal only (pass through vertex)
			return vertR, vertG, vertB, vertA
		case VOODOO_CC_CLOC_MUL:
			// clocal * cother (modulate/multiply)
			return vertR * texR, vertG * texG, vertB * texB, vertA * texA
		default:
			// Default to modulate
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
	// For LESS/LESSEQUAL: clear to max (so any fragment closer passes)
	// For GREATER/GREATEREQUAL: clear to 0 (so any fragment farther passes)
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

	// Phase 3: Check alpha test settings
	alphaTestEnable := (b.alphaMode & VOODOO_ALPHA_TEST_EN) != 0
	alphaTestFunc := int((b.alphaMode >> 1) & 0x7)
	alphaTestRef := float32((b.alphaMode>>24)&0xFF) / 255.0

	// Phase 3: Check chroma key settings
	chromaKeyEnable := (b.fbzMode & VOODOO_FBZ_CHROMAKEY) != 0

	// Check alpha blending
	alphaBlendEnable := (b.alphaMode & VOODOO_ALPHA_BLEND_EN) != 0

	// Phase 6: Check fog settings
	fogEnable := (b.fogMode & VOODOO_FOG_ENABLE) != 0
	var fogR, fogG, fogB float32
	if fogEnable {
		fogR = float32((b.fogColor>>16)&0xFF) / 255.0
		fogG = float32((b.fogColor>>8)&0xFF) / 255.0
		fogB = float32(b.fogColor&0xFF) / 255.0
	}

	// Phase 6: Check dithering settings
	ditherEnable := (b.fbzMode & VOODOO_FBZ_DITHER) != 0
	dither2x2 := (b.fbzMode & VOODOO_FBZ_DITHER_2X2) != 0

	// Rasterize - optimized with row base precomputation
	for y := minY; y < maxY; y++ {
		// Pre-compute row base offset (avoids y*width multiplication per pixel)
		rowBase := y * b.width
		py := float32(y) + 0.5

		for x := minX; x < maxX; x++ {
			// Sample at pixel center
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

				// Depth test - use precomputed row base
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

				// Phase 4/5: Texture mapping with color combine
				if b.textureEnabled && b.textureData != nil {
					// Interpolate texture coordinates
					s := w0*v0.S + w1*v1.S + w2*v2.S
					t := w0*v0.T + w1*v1.T + w2*v2.T

					// Sample texture
					texR, texG, texB, texA := b.sampleTexture(s, t)

					// Phase 5: Apply color combine mode from fbzColorPath
					r, g, bVal, a = b.combineColors(r, g, bVal, a, texR, texG, texB, texA)
				}

				// Clamp colors
				r = clampf(r, 0, 1)
				g = clampf(g, 0, 1)
				bVal = clampf(bVal, 0, 1)
				a = clampf(a, 0, 1)

				// Phase 3: Alpha test (discard if fails)
				if alphaTestEnable {
					if !b.alphaTest(a, alphaTestRef, alphaTestFunc) {
						continue // Discard this fragment
					}
				}

				// Phase 3: Chroma key test (discard if matches key color)
				if chromaKeyEnable {
					if b.chromaKeyTest(r, g, bVal) {
						continue // Discard this fragment
					}
				}

				// Phase 6: Apply fog blending
				// Fog blends the fragment color with fog color based on depth
				// fogFactor = depth (0.0 = no fog, 1.0 = full fog)
				if fogEnable {
					fogFactor := clampf(z, 0, 1) // Linear fog based on depth
					r = r*(1-fogFactor) + fogR*fogFactor
					g = g*(1-fogFactor) + fogG*fogFactor
					bVal = bVal*(1-fogFactor) + fogB*fogFactor
					// Re-clamp after fog blending
					r = clampf(r, 0, 1)
					g = clampf(g, 0, 1)
					bVal = clampf(bVal, 0, 1)
				}

				// Phase 6: Apply dithering
				// 4x4 Bayer matrix dithering (or 2x2 if DITHER_2X2 is set)
				if ditherEnable {
					threshold := b.getDitherThreshold(x, y, dither2x2)
					// Apply dither threshold to each color channel
					r = b.applyDither(r, threshold)
					g = b.applyDither(g, threshold)
					bVal = b.applyDither(bVal, threshold)
				}

				// Write pixel
				if rgbWrite {
					bufIdx := pixelIndex * 4
					if alphaBlendEnable {
						// Configurable blending using blend factors
						srcR, srcG, srcB, srcA := r, g, bVal, a
						const inv255 = float32(1.0 / 255.0)
						dstR := float32(b.colorBuffer[bufIdx+0]) * inv255
						dstG := float32(b.colorBuffer[bufIdx+1]) * inv255
						dstB := float32(b.colorBuffer[bufIdx+2]) * inv255
						dstA := float32(b.colorBuffer[bufIdx+3]) * inv255

						// Get blend factors
						srcFactor := b.getBlendFactor(b.pipelineKey.SrcBlendFactor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)
						dstFactor := b.getBlendFactor(b.pipelineKey.DstBlendFactor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)

						// Apply blend equation: result = src * srcFactor + dst * dstFactor
						outR := clampf(srcR*srcFactor+dstR*dstFactor, 0, 1)
						outG := clampf(srcG*srcFactor+dstG*dstFactor, 0, 1)
						outB := clampf(srcB*srcFactor+dstB*dstFactor, 0, 1)
						outA := clampf(srcA*srcFactor+dstA*dstFactor, 0, 1)

						// Write blended pixel as packed uint32 (RGBA)
						packed := uint32(outR*255) | uint32(outG*255)<<8 | uint32(outB*255)<<16 | uint32(outA*255)<<24
						*(*uint32)(unsafe.Pointer(&b.colorBuffer[bufIdx])) = packed
					} else {
						// Write non-blended pixel as packed uint32 (RGBA) - single memory write
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
// Phase 3: Alpha Test support
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
// Phase 3: Chroma Key support
// Optimized: uses pure float32 arithmetic instead of math.Abs with float64 conversion
func (b *VoodooSoftwareBackend) chromaKeyTest(r, g, bVal float32) bool {
	// Extract RGB from chroma key (packed as 0x00RRGGBB)
	const inv255 = float32(1.0 / 255.0)
	keyR := float32((b.chromaKey>>16)&0xFF) * inv255
	keyG := float32((b.chromaKey>>8)&0xFF) * inv255
	keyB := float32(b.chromaKey&0xFF) * inv255

	// Use a small tolerance for floating point comparison
	const tolerance = inv255

	// Check if color matches chroma key within tolerance (pure float32)
	rMatch := abs32(r-keyR) <= tolerance
	gMatch := abs32(g-keyG) <= tolerance
	bMatch := abs32(bVal-keyB) <= tolerance

	return rMatch && gMatch && bMatch
}

// Phase 6: Dithering support

// bayer4x4Flat is a flattened 4x4 Bayer ordered dither matrix (normalized 0.0-1.0)
// Indexed as: bayer4x4Flat[(y&3)<<2 | (x&3)] for O(1) lookup with bitmask
var bayer4x4Flat = [16]float32{
	0.0 / 16.0, 8.0 / 16.0, 2.0 / 16.0, 10.0 / 16.0, // y=0
	12.0 / 16.0, 4.0 / 16.0, 14.0 / 16.0, 6.0 / 16.0, // y=1
	3.0 / 16.0, 11.0 / 16.0, 1.0 / 16.0, 9.0 / 16.0, // y=2
	15.0 / 16.0, 7.0 / 16.0, 13.0 / 16.0, 5.0 / 16.0, // y=3
}

// bayer2x2Flat is a flattened 2x2 Bayer ordered dither matrix
// Indexed as: bayer2x2Flat[(y&1)<<1 | (x&1)]
var bayer2x2Flat = [4]float32{
	0.0 / 4.0, 2.0 / 4.0, // y=0
	3.0 / 4.0, 1.0 / 4.0, // y=1
}

// getDitherThreshold returns the dither threshold for a given pixel position
// Optimized: uses flattened arrays with bitmask indexing instead of modulo
func (b *VoodooSoftwareBackend) getDitherThreshold(x, y int, use2x2 bool) float32 {
	if use2x2 {
		return bayer2x2Flat[(y&1)<<1|(x&1)]
	}
	return bayer4x4Flat[(y&3)<<2|(x&3)]
}

// applyDither applies ordered dithering to a color value
// This helps reduce banding when quantizing to 8-bit color
func (b *VoodooSoftwareBackend) applyDither(value, threshold float32) float32 {
	// Ordered dithering works by adding a threshold offset before quantization
	// The threshold is normalized to the 0-1 range from the Bayer matrix
	// We scale it to about 1 color level (1/256) for visible effect
	// The pattern creates perception of intermediate colors through spatial mixing

	// Convert to 8-bit range, add dither offset, then back to 0-1
	colorLevel := value * 255.0
	ditherOffset := threshold - 0.5 // Range -0.5 to +0.5
	colorLevel += ditherOffset

	// Quantize to integer and back
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
		// Use precomputed 1/3 instead of division
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
		// min(srcA, 1-dstA)
		invDstA := 1.0 - dstA
		if srcA < invDstA {
			return srcA
		}
		return invDstA
	}
	return 1.0 // Default to ONE
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

// =============================================================================
// Vulkan Backend Implementation
// =============================================================================

// VulkanVertex is the vertex format for the Vulkan pipeline
// Phase 4: Added texture coordinates
type VulkanVertex struct {
	Position [3]float32 // X, Y, Z
	Color    [4]float32 // R, G, B, A
	TexCoord [2]float32 // S, T (texture coordinates)
}

// VulkanBackend implements hardware-accelerated rendering using Vulkan
type VulkanBackend struct {
	mutex sync.RWMutex

	// Vulkan instance and device
	instance       vk.Instance
	physicalDevice vk.PhysicalDevice
	device         vk.Device
	graphicsQueue  vk.Queue
	queueFamily    uint32

	// Offscreen rendering resources
	width, height    int
	colorImage       vk.Image
	colorImageMemory vk.DeviceMemory
	colorImageView   vk.ImageView
	depthImage       vk.Image
	depthImageMemory vk.DeviceMemory
	depthImageView   vk.ImageView

	// Render pass and framebuffer
	renderPass  vk.RenderPass
	framebuffer vk.Framebuffer

	// Pipeline management
	pipelineLayout     vk.PipelineLayout
	pipeline           vk.Pipeline // Default/current pipeline
	vkPipelineCache    vk.PipelineCache
	pipelineVariants   map[PipelineKey]vk.Pipeline // Pipeline cache for different states
	currentPipelineKey PipelineKey                 // Currently bound pipeline key

	// Vertex buffer (dynamic)
	vertexBuffer       vk.Buffer
	vertexBufferMemory vk.DeviceMemory
	vertexBufferSize   vk.DeviceSize

	// Staging buffer for readback
	stagingBuffer       vk.Buffer
	stagingBufferMemory vk.DeviceMemory

	// Command pool and buffer
	commandPool   vk.CommandPool
	commandBuffer vk.CommandBuffer

	// Synchronization
	fence vk.Fence

	// Current state
	fbzMode      uint32
	alphaMode    uint32
	chromaKey    uint32 // Phase 3: Chroma key color
	fbzColorPath uint32 // Phase 5: Color combine mode
	colorPathSet bool   // Phase 5: Track if color path was explicitly set
	fogMode      uint32 // Phase 6: Fog mode
	fogColor     uint32 // Phase 6: Fog color
	scissor      vk.Rect2D

	// Clear color (set by ClearFramebuffer, used by FlushTriangles)
	clearColor [4]float32
	needsClear bool

	// Depth clear value (depends on depth function)
	depthClearValue float32

	// Output frame for compositor
	outputFrame []byte

	// Shader modules
	vertShaderModule vk.ShaderModule
	fragShaderModule vk.ShaderModule

	// Initialization state
	initialized bool

	// Software fallback (used if Vulkan init fails)
	software *VoodooSoftwareBackend

	// Phase 4: Texture resources
	textureImage       vk.Image
	textureImageMemory vk.DeviceMemory
	textureImageView   vk.ImageView
	textureSampler     vk.Sampler
	textureWidth       int
	textureHeight      int
	textureEnabled     bool
	textureClampS      bool
	textureClampT      bool

	// Descriptor set for texture
	descriptorPool      vk.DescriptorPool
	descriptorSetLayout vk.DescriptorSetLayout
	descriptorSet       vk.DescriptorSet
	textureStaging      vk.Buffer
	textureStagingMem   vk.DeviceMemory
}

// Global Vulkan initialization flag
var vulkanInitialized bool
var vulkanInitMutex sync.Mutex

// NewVulkanBackend creates a new Vulkan backend
func NewVulkanBackend() (*VulkanBackend, error) {
	vb := &VulkanBackend{
		software:         NewVoodooSoftwareBackend(),
		pipelineVariants: make(map[PipelineKey]vk.Pipeline),
		depthClearValue:  1.0,                  // Default depth clear for LESS comparison
		fbzColorPath:     VOODOO_COMBINE_UNSET, // Not set = use defaults
	}
	return vb, nil
}

// Init initializes the Vulkan backend
func (vb *VulkanBackend) Init(width, height int) error {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.width = width
	vb.height = height
	vb.outputFrame = make([]byte, width*height*4)

	// Initialize software backend as fallback
	if err := vb.software.Init(width, height); err != nil {
		return err
	}

	// Try to initialize Vulkan
	if err := vb.initVulkan(); err != nil {
		// Vulkan initialization failed, will use software fallback
		fmt.Printf("Vulkan initialization failed, using software backend: %v\n", err)
		vb.initialized = false
		return nil
	}

	vb.initialized = true
	return nil
}

// initVulkan performs full Vulkan initialization
func (vb *VulkanBackend) initVulkan() error {
	vulkanInitMutex.Lock()
	defer vulkanInitMutex.Unlock()

	// Initialize Vulkan loader
	if !vulkanInitialized {
		// First, set up the proc address loader to find the Vulkan library
		if err := vk.SetDefaultGetInstanceProcAddr(); err != nil {
			return fmt.Errorf("failed to load Vulkan library: %w", err)
		}
		if err := vk.Init(); err != nil {
			return fmt.Errorf("failed to initialize Vulkan loader: %w", err)
		}
		vulkanInitialized = true
	}

	// Create instance
	if err := vb.createInstance(); err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	// Select physical device
	if err := vb.selectPhysicalDevice(); err != nil {
		vb.destroyInstance()
		return fmt.Errorf("failed to select physical device: %w", err)
	}

	// Create logical device
	if err := vb.createDevice(); err != nil {
		vb.destroyInstance()
		return fmt.Errorf("failed to create device: %w", err)
	}

	// Create command pool
	if err := vb.createCommandPool(); err != nil {
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create command pool: %w", err)
	}

	// Create offscreen images
	if err := vb.createOffscreenImages(); err != nil {
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create offscreen images: %w", err)
	}

	// Create render pass
	if err := vb.createRenderPass(); err != nil {
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create render pass: %w", err)
	}

	// Create framebuffer
	if err := vb.createFramebuffer(); err != nil {
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create framebuffer: %w", err)
	}

	// Phase 4: Create descriptor set layout for textures
	if err := vb.createDescriptorSetLayout(); err != nil {
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create descriptor set layout: %w", err)
	}

	// Phase 4: Create descriptor pool
	if err := vb.createDescriptorPool(); err != nil {
		vb.destroyTextureResources()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create descriptor pool: %w", err)
	}

	// Phase 4: Create texture sampler
	if err := vb.createTextureSampler(); err != nil {
		vb.destroyTextureResources()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create texture sampler: %w", err)
	}

	// Phase 4: Create default texture (1x1 white)
	if err := vb.createDefaultTexture(); err != nil {
		vb.destroyTextureResources()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create default texture: %w", err)
	}

	// Phase 4: Allocate descriptor set
	if err := vb.allocateDescriptorSet(); err != nil {
		vb.destroyTextureResources()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to allocate descriptor set: %w", err)
	}

	// Create pipeline
	if err := vb.createPipeline(); err != nil {
		vb.destroyTextureResources()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Create vertex buffer
	if err := vb.createVertexBuffer(); err != nil {
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create vertex buffer: %w", err)
	}

	// Create staging buffer for readback
	if err := vb.createStagingBuffer(); err != nil {
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create staging buffer: %w", err)
	}

	// Create command buffer
	if err := vb.createCommandBuffer(); err != nil {
		vb.destroyStagingBuffer()
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create command buffer: %w", err)
	}

	// Create fence
	if err := vb.createFence(); err != nil {
		vb.destroyStagingBuffer()
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create fence: %w", err)
	}

	// Set default scissor
	vb.scissor = vk.Rect2D{
		Offset: vk.Offset2D{X: 0, Y: 0},
		Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)},
	}

	return nil
}

// createInstance creates the Vulkan instance
func (vb *VulkanBackend) createInstance() error {
	appInfo := vk.ApplicationInfo{
		SType:              vk.StructureTypeApplicationInfo,
		PApplicationName:   safeString("IntuitionEngine Voodoo"),
		ApplicationVersion: vk.MakeVersion(1, 0, 0),
		PEngineName:        safeString("Voodoo HLE"),
		EngineVersion:      vk.MakeVersion(1, 0, 0),
		ApiVersion:         vk.MakeVersion(1, 1, 0),
	}

	createInfo := vk.InstanceCreateInfo{
		SType:            vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo: &appInfo,
	}

	var instance vk.Instance
	if res := vk.CreateInstance(&createInfo, nil, &instance); res != vk.Success {
		return fmt.Errorf("vkCreateInstance failed: %d", res)
	}

	vb.instance = instance
	vk.InitInstance(instance)
	return nil
}

// selectPhysicalDevice selects a suitable GPU
func (vb *VulkanBackend) selectPhysicalDevice() error {
	var deviceCount uint32
	vk.EnumeratePhysicalDevices(vb.instance, &deviceCount, nil)
	if deviceCount == 0 {
		return fmt.Errorf("no Vulkan-capable GPUs found")
	}

	devices := make([]vk.PhysicalDevice, deviceCount)
	vk.EnumeratePhysicalDevices(vb.instance, &deviceCount, devices)

	// Find a device with a graphics queue
	for _, device := range devices {
		var queueFamilyCount uint32
		vk.GetPhysicalDeviceQueueFamilyProperties(device, &queueFamilyCount, nil)
		queueFamilies := make([]vk.QueueFamilyProperties, queueFamilyCount)
		vk.GetPhysicalDeviceQueueFamilyProperties(device, &queueFamilyCount, queueFamilies)

		for i, qf := range queueFamilies {
			qf.Deref()
			if qf.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 {
				vb.physicalDevice = device
				vb.queueFamily = uint32(i)
				return nil
			}
		}
	}

	return fmt.Errorf("no suitable GPU with graphics queue found")
}

// createDevice creates the logical device
func (vb *VulkanBackend) createDevice() error {
	queuePriority := float32(1.0)
	queueCreateInfo := vk.DeviceQueueCreateInfo{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: vb.queueFamily,
		QueueCount:       1,
		PQueuePriorities: []float32{queuePriority},
	}

	deviceCreateInfo := vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos:    []vk.DeviceQueueCreateInfo{queueCreateInfo},
	}

	var device vk.Device
	if res := vk.CreateDevice(vb.physicalDevice, &deviceCreateInfo, nil, &device); res != vk.Success {
		return fmt.Errorf("vkCreateDevice failed: %d", res)
	}

	vb.device = device

	var queue vk.Queue
	vk.GetDeviceQueue(device, vb.queueFamily, 0, &queue)
	vb.graphicsQueue = queue

	return nil
}

// createCommandPool creates the command pool
func (vb *VulkanBackend) createCommandPool() error {
	poolInfo := vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		QueueFamilyIndex: vb.queueFamily,
		Flags:            vk.CommandPoolCreateFlags(vk.CommandPoolCreateResetCommandBufferBit),
	}

	var pool vk.CommandPool
	if res := vk.CreateCommandPool(vb.device, &poolInfo, nil, &pool); res != vk.Success {
		return fmt.Errorf("vkCreateCommandPool failed: %d", res)
	}

	vb.commandPool = pool
	return nil
}

// createOffscreenImages creates the color and depth images for offscreen rendering
func (vb *VulkanBackend) createOffscreenImages() error {
	// Color image
	colorImageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    vk.FormatR8g8b8a8Unorm,
		Extent: vk.Extent3D{
			Width:  uint32(vb.width),
			Height: uint32(vb.height),
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1Bit,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit | vk.ImageUsageTransferSrcBit),
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var colorImage vk.Image
	if res := vk.CreateImage(vb.device, &colorImageInfo, nil, &colorImage); res != vk.Success {
		return fmt.Errorf("vkCreateImage (color) failed: %d", res)
	}
	vb.colorImage = colorImage

	// Allocate memory for color image
	var memReqs vk.MemoryRequirements
	vk.GetImageMemoryRequirements(vb.device, colorImage, &memReqs)
	memReqs.Deref()

	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		return err
	}

	colorAllocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var colorMem vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &colorAllocInfo, nil, &colorMem); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (color) failed: %d", res)
	}
	vb.colorImageMemory = colorMem

	vk.BindImageMemory(vb.device, colorImage, colorMem, 0)

	// Color image view
	colorViewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    colorImage,
		ViewType: vk.ImageViewType2d,
		Format:   vk.FormatR8g8b8a8Unorm,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var colorView vk.ImageView
	if res := vk.CreateImageView(vb.device, &colorViewInfo, nil, &colorView); res != vk.Success {
		return fmt.Errorf("vkCreateImageView (color) failed: %d", res)
	}
	vb.colorImageView = colorView

	// Depth image
	depthFormat := vk.FormatD32Sfloat
	depthImageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    depthFormat,
		Extent: vk.Extent3D{
			Width:  uint32(vb.width),
			Height: uint32(vb.height),
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1Bit,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageDepthStencilAttachmentBit),
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var depthImage vk.Image
	if res := vk.CreateImage(vb.device, &depthImageInfo, nil, &depthImage); res != vk.Success {
		return fmt.Errorf("vkCreateImage (depth) failed: %d", res)
	}
	vb.depthImage = depthImage

	// Allocate memory for depth image
	vk.GetImageMemoryRequirements(vb.device, depthImage, &memReqs)
	memReqs.Deref()

	memTypeIndex, err = vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		return err
	}

	depthAllocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var depthMem vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &depthAllocInfo, nil, &depthMem); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (depth) failed: %d", res)
	}
	vb.depthImageMemory = depthMem

	vk.BindImageMemory(vb.device, depthImage, depthMem, 0)

	// Depth image view
	depthViewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    depthImage,
		ViewType: vk.ImageViewType2d,
		Format:   depthFormat,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectDepthBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var depthView vk.ImageView
	if res := vk.CreateImageView(vb.device, &depthViewInfo, nil, &depthView); res != vk.Success {
		return fmt.Errorf("vkCreateImageView (depth) failed: %d", res)
	}
	vb.depthImageView = depthView

	return nil
}

// createRenderPass creates the render pass
func (vb *VulkanBackend) createRenderPass() error {
	colorAttachment := vk.AttachmentDescription{
		Format:         vk.FormatR8g8b8a8Unorm,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutTransferSrcOptimal,
	}

	depthAttachment := vk.AttachmentDescription{
		Format:         vk.FormatD32Sfloat,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpDontCare,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutDepthStencilAttachmentOptimal,
	}

	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}

	depthRef := vk.AttachmentReference{
		Attachment: 1,
		Layout:     vk.ImageLayoutDepthStencilAttachmentOptimal,
	}

	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    1,
		PColorAttachments:       []vk.AttachmentReference{colorRef},
		PDepthStencilAttachment: &depthRef,
	}

	renderPassInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 2,
		PAttachments:    []vk.AttachmentDescription{colorAttachment, depthAttachment},
		SubpassCount:    1,
		PSubpasses:      []vk.SubpassDescription{subpass},
	}

	var renderPass vk.RenderPass
	if res := vk.CreateRenderPass(vb.device, &renderPassInfo, nil, &renderPass); res != vk.Success {
		return fmt.Errorf("vkCreateRenderPass failed: %d", res)
	}

	vb.renderPass = renderPass
	return nil
}

// createFramebuffer creates the framebuffer
func (vb *VulkanBackend) createFramebuffer() error {
	attachments := []vk.ImageView{vb.colorImageView, vb.depthImageView}

	fbInfo := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      vb.renderPass,
		AttachmentCount: uint32(len(attachments)),
		PAttachments:    attachments,
		Width:           uint32(vb.width),
		Height:          uint32(vb.height),
		Layers:          1,
	}

	var framebuffer vk.Framebuffer
	if res := vk.CreateFramebuffer(vb.device, &fbInfo, nil, &framebuffer); res != vk.Success {
		return fmt.Errorf("vkCreateFramebuffer failed: %d", res)
	}

	vb.framebuffer = framebuffer
	return nil
}

// createPipeline creates the graphics pipeline and initializes the pipeline layout
func (vb *VulkanBackend) createPipeline() error {
	// Create shader modules from embedded SPIR-V
	vertModule, err := vb.createShaderModule(VoodooVertexShaderSPIRV)
	if err != nil {
		return fmt.Errorf("failed to create vertex shader module: %w", err)
	}
	vb.vertShaderModule = vertModule

	fragModule, err := vb.createShaderModule(VoodooFragmentShaderSPIRV)
	if err != nil {
		vk.DestroyShaderModule(vb.device, vertModule, nil)
		return fmt.Errorf("failed to create fragment shader module: %w", err)
	}
	vb.fragShaderModule = fragModule

	// Pipeline layout (shared by all variants)
	// Phase 3: Add push constant range for alpha test and chroma key
	pushConstantRange := vk.PushConstantRange{
		StageFlags: vk.ShaderStageFlags(vk.ShaderStageFragmentBit),
		Offset:     0,
		Size:       28, // VoodooPushConstants: 7 x uint32 = 28 bytes (Phase 6: added FogMode, FogColor)
	}

	// Phase 4: Include descriptor set layout for texture sampling
	layoutInfo := vk.PipelineLayoutCreateInfo{
		SType:                  vk.StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount:         1,
		PSetLayouts:            []vk.DescriptorSetLayout{vb.descriptorSetLayout},
		PushConstantRangeCount: 1,
		PPushConstantRanges:    []vk.PushConstantRange{pushConstantRange},
	}

	var pipelineLayout vk.PipelineLayout
	if res := vk.CreatePipelineLayout(vb.device, &layoutInfo, nil, &pipelineLayout); res != vk.Success {
		return fmt.Errorf("vkCreatePipelineLayout failed: %d", res)
	}
	vb.pipelineLayout = pipelineLayout

	// Create the default pipeline (depth LESS, no blending)
	defaultKey := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   VOODOO_DEPTH_LESS,
		BlendEnable:      false,
		SrcBlendFactor:   VOODOO_BLEND_ONE,
		DstBlendFactor:   VOODOO_BLEND_ZERO,
	}

	pipeline, err := vb.createPipelineVariant(defaultKey)
	if err != nil {
		return err
	}

	vb.pipeline = pipeline
	vb.currentPipelineKey = defaultKey
	vb.pipelineVariants[defaultKey] = pipeline

	return nil
}

// createPipelineVariant creates a graphics pipeline with specific depth/blend settings
func (vb *VulkanBackend) createPipelineVariant(key PipelineKey) (vk.Pipeline, error) {
	// Shader stages
	vertStage := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageVertexBit,
		Module: vb.vertShaderModule,
		PName:  safeString("main"),
	}

	fragStage := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageFragmentBit,
		Module: vb.fragShaderModule,
		PName:  safeString("main"),
	}

	shaderStages := []vk.PipelineShaderStageCreateInfo{vertStage, fragStage}

	// Vertex input
	bindingDesc := vk.VertexInputBindingDescription{
		Binding:   0,
		Stride:    uint32(unsafe.Sizeof(VulkanVertex{})),
		InputRate: vk.VertexInputRateVertex,
	}

	attrDescs := []vk.VertexInputAttributeDescription{
		{
			Location: 0,
			Binding:  0,
			Format:   vk.FormatR32g32b32Sfloat,
			Offset:   0,
		},
		{
			Location: 1,
			Binding:  0,
			Format:   vk.FormatR32g32b32a32Sfloat,
			Offset:   uint32(unsafe.Offsetof(VulkanVertex{}.Color)),
		},
		{
			// Phase 4: Texture coordinates
			Location: 2,
			Binding:  0,
			Format:   vk.FormatR32g32Sfloat,
			Offset:   uint32(unsafe.Offsetof(VulkanVertex{}.TexCoord)),
		},
	}

	vertexInputInfo := vk.PipelineVertexInputStateCreateInfo{
		SType:                           vk.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		PVertexBindingDescriptions:      []vk.VertexInputBindingDescription{bindingDesc},
		VertexAttributeDescriptionCount: uint32(len(attrDescs)),
		PVertexAttributeDescriptions:    attrDescs,
	}

	// Input assembly
	inputAssembly := vk.PipelineInputAssemblyStateCreateInfo{
		SType:                  vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               vk.PrimitiveTopologyTriangleList,
		PrimitiveRestartEnable: vk.False,
	}

	// Viewport and scissor (scissor is dynamic)
	viewport := vk.Viewport{
		X:        0,
		Y:        0,
		Width:    float32(vb.width),
		Height:   float32(vb.height),
		MinDepth: 0,
		MaxDepth: 1,
	}

	scissor := vk.Rect2D{
		Offset: vk.Offset2D{X: 0, Y: 0},
		Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)},
	}

	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		PViewports:    []vk.Viewport{viewport},
		ScissorCount:  1,
		PScissors:     []vk.Rect2D{scissor},
	}

	// Rasterization
	rasterizer := vk.PipelineRasterizationStateCreateInfo{
		SType:                   vk.StructureTypePipelineRasterizationStateCreateInfo,
		DepthClampEnable:        vk.False,
		RasterizerDiscardEnable: vk.False,
		PolygonMode:             vk.PolygonModeFill,
		CullMode:                vk.CullModeFlags(vk.CullModeNone),
		FrontFace:               vk.FrontFaceCounterClockwise,
		DepthBiasEnable:         vk.False,
		LineWidth:               1.0,
	}

	// Multisampling
	multisampling := vk.PipelineMultisampleStateCreateInfo{
		SType:                 vk.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples:  vk.SampleCount1Bit,
		SampleShadingEnable:   vk.False,
		MinSampleShading:      1.0,
		AlphaToCoverageEnable: vk.False,
		AlphaToOneEnable:      vk.False,
	}

	// Depth/stencil - configured based on PipelineKey
	var depthTestEnable vk.Bool32 = vk.False
	if key.DepthTestEnable {
		depthTestEnable = vk.True
	}
	var depthWriteEnable vk.Bool32 = vk.False
	if key.DepthWriteEnable {
		depthWriteEnable = vk.True
	}

	depthStencil := vk.PipelineDepthStencilStateCreateInfo{
		SType:                 vk.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable:       depthTestEnable,
		DepthWriteEnable:      depthWriteEnable,
		DepthCompareOp:        vk.CompareOp(VoodooDepthFuncToVulkan(key.DepthCompareOp)),
		DepthBoundsTestEnable: vk.False,
		StencilTestEnable:     vk.False,
	}

	// Color blending - configured based on PipelineKey
	var blendEnable vk.Bool32 = vk.False
	if key.BlendEnable {
		blendEnable = vk.True
	}

	colorBlendAttachment := vk.PipelineColorBlendAttachmentState{
		BlendEnable:         blendEnable,
		SrcColorBlendFactor: vk.BlendFactor(VoodooBlendFactorToVulkan(key.SrcBlendFactor)),
		DstColorBlendFactor: vk.BlendFactor(VoodooBlendFactorToVulkan(key.DstBlendFactor)),
		ColorBlendOp:        vk.BlendOpAdd,
		SrcAlphaBlendFactor: vk.BlendFactor(VoodooBlendFactorToVulkan(key.SrcBlendFactor)),
		DstAlphaBlendFactor: vk.BlendFactor(VoodooBlendFactorToVulkan(key.DstBlendFactor)),
		AlphaBlendOp:        vk.BlendOpAdd,
		ColorWriteMask:      vk.ColorComponentFlags(vk.ColorComponentRBit | vk.ColorComponentGBit | vk.ColorComponentBBit | vk.ColorComponentABit),
	}

	colorBlending := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		LogicOpEnable:   vk.False,
		AttachmentCount: 1,
		PAttachments:    []vk.PipelineColorBlendAttachmentState{colorBlendAttachment},
	}

	// Dynamic state
	dynamicStates := []vk.DynamicState{vk.DynamicStateScissor}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    dynamicStates,
	}

	// Create pipeline
	pipelineInfo := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          uint32(len(shaderStages)),
		PStages:             shaderStages,
		PVertexInputState:   &vertexInputInfo,
		PInputAssemblyState: &inputAssembly,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterizer,
		PMultisampleState:   &multisampling,
		PDepthStencilState:  &depthStencil,
		PColorBlendState:    &colorBlending,
		PDynamicState:       &dynamicState,
		Layout:              vb.pipelineLayout,
		RenderPass:          vb.renderPass,
		Subpass:             0,
	}

	pipelines := make([]vk.Pipeline, 1)
	if res := vk.CreateGraphicsPipelines(vb.device, vk.PipelineCache(vk.NullHandle), 1, []vk.GraphicsPipelineCreateInfo{pipelineInfo}, nil, pipelines); res != vk.Success {
		return vk.NullPipeline, fmt.Errorf("vkCreateGraphicsPipelines failed: %d", res)
	}

	return pipelines[0], nil
}

// getOrCreatePipeline returns a pipeline for the given key, creating it if necessary
func (vb *VulkanBackend) getOrCreatePipeline(key PipelineKey) (vk.Pipeline, error) {
	// Check cache first
	if pipeline, exists := vb.pipelineVariants[key]; exists {
		return pipeline, nil
	}

	// Create new pipeline variant
	pipeline, err := vb.createPipelineVariant(key)
	if err != nil {
		return vk.NullPipeline, err
	}

	// Store in cache
	vb.pipelineVariants[key] = pipeline
	return pipeline, nil
}

// createShaderModule creates a shader module from SPIR-V bytecode
func (vb *VulkanBackend) createShaderModule(code []byte) (vk.ShaderModule, error) {
	createInfo := vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uint64(len(code)),
		PCode:    sliceUint32(code),
	}

	var shaderModule vk.ShaderModule
	if res := vk.CreateShaderModule(vb.device, &createInfo, nil, &shaderModule); res != vk.Success {
		return vk.NullShaderModule, fmt.Errorf("vkCreateShaderModule failed: %d", res)
	}

	return shaderModule, nil
}

// createVertexBuffer creates the dynamic vertex buffer
func (vb *VulkanBackend) createVertexBuffer() error {
	// Size for maximum triangles
	vb.vertexBufferSize = vk.DeviceSize(VOODOO_MAX_BATCH_VERTICES * int(unsafe.Sizeof(VulkanVertex{})))

	bufferInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        vb.vertexBufferSize,
		Usage:       vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var buffer vk.Buffer
	if res := vk.CreateBuffer(vb.device, &bufferInfo, nil, &buffer); res != vk.Success {
		return fmt.Errorf("vkCreateBuffer (vertex) failed: %d", res)
	}
	vb.vertexBuffer = buffer

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(vb.device, buffer, &memReqs)
	memReqs.Deref()

	// Find host-visible memory type
	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		return err
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &allocInfo, nil, &memory); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (vertex) failed: %d", res)
	}
	vb.vertexBufferMemory = memory

	vk.BindBufferMemory(vb.device, buffer, memory, 0)
	return nil
}

// createStagingBuffer creates a buffer for reading back the framebuffer
func (vb *VulkanBackend) createStagingBuffer() error {
	bufferSize := vk.DeviceSize(vb.width * vb.height * 4)

	bufferInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        bufferSize,
		Usage:       vk.BufferUsageFlags(vk.BufferUsageTransferDstBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var buffer vk.Buffer
	if res := vk.CreateBuffer(vb.device, &bufferInfo, nil, &buffer); res != vk.Success {
		return fmt.Errorf("vkCreateBuffer (staging) failed: %d", res)
	}
	vb.stagingBuffer = buffer

	var memReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(vb.device, buffer, &memReqs)
	memReqs.Deref()

	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		return err
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &allocInfo, nil, &memory); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (staging) failed: %d", res)
	}
	vb.stagingBufferMemory = memory

	vk.BindBufferMemory(vb.device, buffer, memory, 0)
	return nil
}

// createCommandBuffer allocates a command buffer
func (vb *VulkanBackend) createCommandBuffer() error {
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        vb.commandPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}

	cmdBuffers := make([]vk.CommandBuffer, 1)
	if res := vk.AllocateCommandBuffers(vb.device, &allocInfo, cmdBuffers); res != vk.Success {
		return fmt.Errorf("vkAllocateCommandBuffers failed: %d", res)
	}

	vb.commandBuffer = cmdBuffers[0]
	return nil
}

// createFence creates a fence for synchronization
func (vb *VulkanBackend) createFence() error {
	fenceInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: vk.FenceCreateFlags(vk.FenceCreateSignaledBit),
	}

	var fence vk.Fence
	if res := vk.CreateFence(vb.device, &fenceInfo, nil, &fence); res != vk.Success {
		return fmt.Errorf("vkCreateFence failed: %d", res)
	}

	vb.fence = fence
	return nil
}

// =============================================================================
// Phase 4: Texture Resources
// =============================================================================

// createDescriptorSetLayout creates the descriptor set layout for texture sampling
func (vb *VulkanBackend) createDescriptorSetLayout() error {
	// Binding 0: Combined image sampler for texture
	binding := vk.DescriptorSetLayoutBinding{
		Binding:            0,
		DescriptorType:     vk.DescriptorTypeCombinedImageSampler,
		DescriptorCount:    1,
		StageFlags:         vk.ShaderStageFlags(vk.ShaderStageFragmentBit),
		PImmutableSamplers: nil,
	}

	layoutInfo := vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: 1,
		PBindings:    []vk.DescriptorSetLayoutBinding{binding},
	}

	var layout vk.DescriptorSetLayout
	if res := vk.CreateDescriptorSetLayout(vb.device, &layoutInfo, nil, &layout); res != vk.Success {
		return fmt.Errorf("vkCreateDescriptorSetLayout failed: %d", res)
	}

	vb.descriptorSetLayout = layout
	return nil
}

// createDescriptorPool creates the descriptor pool for texture descriptors
func (vb *VulkanBackend) createDescriptorPool() error {
	poolSize := vk.DescriptorPoolSize{
		Type:            vk.DescriptorTypeCombinedImageSampler,
		DescriptorCount: 1,
	}

	poolInfo := vk.DescriptorPoolCreateInfo{
		SType:         vk.StructureTypeDescriptorPoolCreateInfo,
		MaxSets:       1,
		PoolSizeCount: 1,
		PPoolSizes:    []vk.DescriptorPoolSize{poolSize},
	}

	var pool vk.DescriptorPool
	if res := vk.CreateDescriptorPool(vb.device, &poolInfo, nil, &pool); res != vk.Success {
		return fmt.Errorf("vkCreateDescriptorPool failed: %d", res)
	}

	vb.descriptorPool = pool
	return nil
}

// createTextureSampler creates the texture sampler
func (vb *VulkanBackend) createTextureSampler() error {
	samplerInfo := vk.SamplerCreateInfo{
		SType:                   vk.StructureTypeSamplerCreateInfo,
		MagFilter:               vk.FilterNearest, // Point sampling for now
		MinFilter:               vk.FilterNearest,
		AddressModeU:            vk.SamplerAddressModeRepeat,
		AddressModeV:            vk.SamplerAddressModeRepeat,
		AddressModeW:            vk.SamplerAddressModeRepeat,
		AnisotropyEnable:        vk.False,
		MaxAnisotropy:           1.0,
		BorderColor:             vk.BorderColorFloatOpaqueBlack,
		UnnormalizedCoordinates: vk.False,
		CompareEnable:           vk.False,
		MipmapMode:              vk.SamplerMipmapModeNearest,
		MipLodBias:              0.0,
		MinLod:                  0.0,
		MaxLod:                  0.0,
	}

	var sampler vk.Sampler
	if res := vk.CreateSampler(vb.device, &samplerInfo, nil, &sampler); res != vk.Success {
		return fmt.Errorf("vkCreateSampler failed: %d", res)
	}

	vb.textureSampler = sampler
	return nil
}

// createDefaultTexture creates a 1x1 white texture as default
func (vb *VulkanBackend) createDefaultTexture() error {
	vb.textureWidth = 1
	vb.textureHeight = 1

	// Create the texture image
	imageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    vk.FormatR8g8b8a8Unorm,
		Extent: vk.Extent3D{
			Width:  1,
			Height: 1,
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1Bit,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageTransferDstBit | vk.ImageUsageSampledBit),
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var image vk.Image
	if res := vk.CreateImage(vb.device, &imageInfo, nil, &image); res != vk.Success {
		return fmt.Errorf("vkCreateImage failed: %d", res)
	}
	vb.textureImage = image

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	vk.GetImageMemoryRequirements(vb.device, image, &memReqs)
	memReqs.Deref()

	// Allocate memory
	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		vk.DestroyImage(vb.device, image, nil)
		return fmt.Errorf("failed to find memory type for texture: %w", err)
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &allocInfo, nil, &memory); res != vk.Success {
		vk.DestroyImage(vb.device, image, nil)
		return fmt.Errorf("vkAllocateMemory failed: %d", res)
	}
	vb.textureImageMemory = memory

	vk.BindImageMemory(vb.device, image, memory, 0)

	// Create image view
	viewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    image,
		ViewType: vk.ImageViewType2d,
		Format:   vk.FormatR8g8b8a8Unorm,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var imageView vk.ImageView
	if res := vk.CreateImageView(vb.device, &viewInfo, nil, &imageView); res != vk.Success {
		vk.FreeMemory(vb.device, memory, nil)
		vk.DestroyImage(vb.device, image, nil)
		return fmt.Errorf("vkCreateImageView failed: %d", res)
	}
	vb.textureImageView = imageView

	// Create staging buffer for texture upload
	stagingSize := vk.DeviceSize(4) // 1x1 RGBA
	stagingInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        stagingSize,
		Usage:       vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var stagingBuffer vk.Buffer
	if res := vk.CreateBuffer(vb.device, &stagingInfo, nil, &stagingBuffer); res != vk.Success {
		vk.DestroyImageView(vb.device, imageView, nil)
		vk.FreeMemory(vb.device, memory, nil)
		vk.DestroyImage(vb.device, image, nil)
		return fmt.Errorf("vkCreateBuffer (staging) failed: %d", res)
	}
	vb.textureStaging = stagingBuffer

	var stagingMemReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(vb.device, stagingBuffer, &stagingMemReqs)
	stagingMemReqs.Deref()

	stagingMemType, err := vb.findMemoryType(stagingMemReqs.MemoryTypeBits,
		vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		vk.DestroyBuffer(vb.device, stagingBuffer, nil)
		vk.DestroyImageView(vb.device, imageView, nil)
		vk.FreeMemory(vb.device, memory, nil)
		vk.DestroyImage(vb.device, image, nil)
		return fmt.Errorf("failed to find staging memory type: %w", err)
	}

	stagingAllocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  stagingMemReqs.Size,
		MemoryTypeIndex: stagingMemType,
	}

	var stagingMem vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &stagingAllocInfo, nil, &stagingMem); res != vk.Success {
		vk.DestroyBuffer(vb.device, stagingBuffer, nil)
		vk.DestroyImageView(vb.device, imageView, nil)
		vk.FreeMemory(vb.device, memory, nil)
		vk.DestroyImage(vb.device, image, nil)
		return fmt.Errorf("vkAllocateMemory (staging) failed: %d", res)
	}
	vb.textureStagingMem = stagingMem

	vk.BindBufferMemory(vb.device, stagingBuffer, stagingMem, 0)

	// Upload white pixel to staging buffer
	var data unsafe.Pointer
	vk.MapMemory(vb.device, stagingMem, 0, stagingSize, 0, &data)
	whitePixel := []byte{255, 255, 255, 255}
	copy((*[4]byte)(data)[:], whitePixel)
	vk.UnmapMemory(vb.device, stagingMem)

	// Copy staging buffer to image
	if err := vb.copyBufferToImage(stagingBuffer, image, 1, 1); err != nil {
		return fmt.Errorf("failed to copy buffer to image: %w", err)
	}

	return nil
}

// destroyTextureImage destroys just the texture image resources (not sampler or descriptor set)
func (vb *VulkanBackend) destroyTextureImage() {
	if vb.textureImageView != vk.NullImageView {
		vk.DestroyImageView(vb.device, vb.textureImageView, nil)
		vb.textureImageView = vk.NullImageView
	}
	if vb.textureImage != vk.NullImage {
		vk.DestroyImage(vb.device, vb.textureImage, nil)
		vb.textureImage = vk.NullImage
	}
	if vb.textureImageMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.textureImageMemory, nil)
		vb.textureImageMemory = vk.NullDeviceMemory
	}
	if vb.textureStaging != vk.NullBuffer {
		vk.DestroyBuffer(vb.device, vb.textureStaging, nil)
		vb.textureStaging = vk.NullBuffer
	}
	if vb.textureStagingMem != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.textureStagingMem, nil)
		vb.textureStagingMem = vk.NullDeviceMemory
	}
}

// createTextureImage creates texture image resources for the given dimensions
func (vb *VulkanBackend) createTextureImage(width, height int) error {
	// Create the texture image
	imageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    vk.FormatR8g8b8a8Unorm,
		Extent: vk.Extent3D{
			Width:  uint32(width),
			Height: uint32(height),
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1Bit,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageTransferDstBit | vk.ImageUsageSampledBit),
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var image vk.Image
	if res := vk.CreateImage(vb.device, &imageInfo, nil, &image); res != vk.Success {
		return fmt.Errorf("vkCreateImage failed: %d", res)
	}
	vb.textureImage = image

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	vk.GetImageMemoryRequirements(vb.device, image, &memReqs)
	memReqs.Deref()

	// Allocate memory
	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		vk.DestroyImage(vb.device, image, nil)
		vb.textureImage = vk.NullImage
		return fmt.Errorf("failed to find memory type for texture: %w", err)
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &allocInfo, nil, &memory); res != vk.Success {
		vk.DestroyImage(vb.device, image, nil)
		vb.textureImage = vk.NullImage
		return fmt.Errorf("vkAllocateMemory failed: %d", res)
	}
	vb.textureImageMemory = memory

	vk.BindImageMemory(vb.device, image, memory, 0)

	// Create image view
	viewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    image,
		ViewType: vk.ImageViewType2d,
		Format:   vk.FormatR8g8b8a8Unorm,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var imageView vk.ImageView
	if res := vk.CreateImageView(vb.device, &viewInfo, nil, &imageView); res != vk.Success {
		vb.destroyTextureImage()
		return fmt.Errorf("vkCreateImageView failed: %d", res)
	}
	vb.textureImageView = imageView

	// Create staging buffer for texture upload
	stagingSize := vk.DeviceSize(width * height * 4)
	stagingInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        stagingSize,
		Usage:       vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var stagingBuffer vk.Buffer
	if res := vk.CreateBuffer(vb.device, &stagingInfo, nil, &stagingBuffer); res != vk.Success {
		vb.destroyTextureImage()
		return fmt.Errorf("vkCreateBuffer (staging) failed: %d", res)
	}
	vb.textureStaging = stagingBuffer

	var stagingMemReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(vb.device, stagingBuffer, &stagingMemReqs)
	stagingMemReqs.Deref()

	stagingMemType, err := vb.findMemoryType(stagingMemReqs.MemoryTypeBits,
		vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		vb.destroyTextureImage()
		return fmt.Errorf("failed to find staging memory type: %w", err)
	}

	stagingAllocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  stagingMemReqs.Size,
		MemoryTypeIndex: stagingMemType,
	}

	var stagingMem vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &stagingAllocInfo, nil, &stagingMem); res != vk.Success {
		vb.destroyTextureImage()
		return fmt.Errorf("vkAllocateMemory (staging) failed: %d", res)
	}
	vb.textureStagingMem = stagingMem

	vk.BindBufferMemory(vb.device, stagingBuffer, stagingMem, 0)

	return nil
}

// uploadTextureData uploads texture data to the GPU via staging buffer
func (vb *VulkanBackend) uploadTextureData(data []byte, width, height int) error {
	if vb.textureStaging == vk.NullBuffer {
		return fmt.Errorf("staging buffer not created")
	}

	stagingSize := vk.DeviceSize(width * height * 4)

	// Upload data to staging buffer
	var ptr unsafe.Pointer
	vk.MapMemory(vb.device, vb.textureStagingMem, 0, stagingSize, 0, &ptr)
	copy((*[1 << 30]byte)(ptr)[:len(data)], data)
	vk.UnmapMemory(vb.device, vb.textureStagingMem)

	// Copy staging buffer to image
	if err := vb.copyBufferToImage(vb.textureStaging, vb.textureImage, width, height); err != nil {
		return fmt.Errorf("failed to copy buffer to image: %w", err)
	}

	return nil
}

// copyBufferToImage copies a buffer to an image with proper layout transitions
func (vb *VulkanBackend) copyBufferToImage(buffer vk.Buffer, image vk.Image, width, height int) error {
	// Use a one-time command buffer
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        vb.commandPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}

	cmdBuffers := make([]vk.CommandBuffer, 1)
	if res := vk.AllocateCommandBuffers(vb.device, &allocInfo, cmdBuffers); res != vk.Success {
		return fmt.Errorf("vkAllocateCommandBuffers failed: %d", res)
	}
	cmdBuffer := cmdBuffers[0]

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	vk.BeginCommandBuffer(cmdBuffer, &beginInfo)

	// Transition image layout from undefined to transfer dst
	barrier := vk.ImageMemoryBarrier{
		SType:               vk.StructureTypeImageMemoryBarrier,
		OldLayout:           vk.ImageLayoutUndefined,
		NewLayout:           vk.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image:               image,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
		SrcAccessMask: 0,
		DstAccessMask: vk.AccessFlags(vk.AccessTransferWriteBit),
	}

	vk.CmdPipelineBarrier(cmdBuffer,
		vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit),
		vk.PipelineStageFlags(vk.PipelineStageTransferBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{barrier})

	// Copy buffer to image
	region := vk.BufferImageCopy{
		BufferOffset:      0,
		BufferRowLength:   0,
		BufferImageHeight: 0,
		ImageSubresource: vk.ImageSubresourceLayers{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			MipLevel:       0,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
		ImageOffset: vk.Offset3D{X: 0, Y: 0, Z: 0},
		ImageExtent: vk.Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
	}

	vk.CmdCopyBufferToImage(cmdBuffer, buffer, image,
		vk.ImageLayoutTransferDstOptimal, 1, []vk.BufferImageCopy{region})

	// Transition image layout to shader read optimal
	barrier.OldLayout = vk.ImageLayoutTransferDstOptimal
	barrier.NewLayout = vk.ImageLayoutShaderReadOnlyOptimal
	barrier.SrcAccessMask = vk.AccessFlags(vk.AccessTransferWriteBit)
	barrier.DstAccessMask = vk.AccessFlags(vk.AccessShaderReadBit)

	vk.CmdPipelineBarrier(cmdBuffer,
		vk.PipelineStageFlags(vk.PipelineStageTransferBit),
		vk.PipelineStageFlags(vk.PipelineStageFragmentShaderBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{barrier})

	vk.EndCommandBuffer(cmdBuffer)

	// Submit and wait
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vk.CommandBuffer{cmdBuffer},
	}

	vk.QueueSubmit(vb.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vk.NullFence)
	vk.QueueWaitIdle(vb.graphicsQueue)

	vk.FreeCommandBuffers(vb.device, vb.commandPool, 1, cmdBuffers)

	return nil
}

// allocateDescriptorSet allocates and updates the descriptor set
func (vb *VulkanBackend) allocateDescriptorSet() error {
	allocInfo := vk.DescriptorSetAllocateInfo{
		SType:              vk.StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     vb.descriptorPool,
		DescriptorSetCount: 1,
		PSetLayouts:        []vk.DescriptorSetLayout{vb.descriptorSetLayout},
	}

	var descriptorSet vk.DescriptorSet
	if res := vk.AllocateDescriptorSets(vb.device, &allocInfo, &descriptorSet); res != vk.Success {
		return fmt.Errorf("vkAllocateDescriptorSets failed: %d", res)
	}
	vb.descriptorSet = descriptorSet

	// Update descriptor set with the default texture
	vb.updateDescriptorSet()

	return nil
}

// updateDescriptorSet updates the descriptor set with the current texture
func (vb *VulkanBackend) updateDescriptorSet() {
	imageInfo := vk.DescriptorImageInfo{
		Sampler:     vb.textureSampler,
		ImageView:   vb.textureImageView,
		ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
	}

	writeDescriptor := vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          vb.descriptorSet,
		DstBinding:      0,
		DstArrayElement: 0,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeCombinedImageSampler,
		PImageInfo:      []vk.DescriptorImageInfo{imageInfo},
	}

	vk.UpdateDescriptorSets(vb.device, 1, []vk.WriteDescriptorSet{writeDescriptor}, 0, nil)
}

// destroyTextureResources cleans up texture-related Vulkan resources
func (vb *VulkanBackend) destroyTextureResources() {
	if vb.descriptorPool != vk.NullDescriptorPool {
		vk.DestroyDescriptorPool(vb.device, vb.descriptorPool, nil)
		vb.descriptorPool = vk.NullDescriptorPool
	}
	if vb.descriptorSetLayout != vk.NullDescriptorSetLayout {
		vk.DestroyDescriptorSetLayout(vb.device, vb.descriptorSetLayout, nil)
		vb.descriptorSetLayout = vk.NullDescriptorSetLayout
	}
	if vb.textureSampler != vk.NullSampler {
		vk.DestroySampler(vb.device, vb.textureSampler, nil)
		vb.textureSampler = vk.NullSampler
	}
	if vb.textureImageView != vk.NullImageView {
		vk.DestroyImageView(vb.device, vb.textureImageView, nil)
		vb.textureImageView = vk.NullImageView
	}
	if vb.textureImage != vk.NullImage {
		vk.DestroyImage(vb.device, vb.textureImage, nil)
		vb.textureImage = vk.NullImage
	}
	if vb.textureImageMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.textureImageMemory, nil)
		vb.textureImageMemory = vk.NullDeviceMemory
	}
	if vb.textureStaging != vk.NullBuffer {
		vk.DestroyBuffer(vb.device, vb.textureStaging, nil)
		vb.textureStaging = vk.NullBuffer
	}
	if vb.textureStagingMem != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.textureStagingMem, nil)
		vb.textureStagingMem = vk.NullDeviceMemory
	}
}

// findMemoryType finds a suitable memory type
func (vb *VulkanBackend) findMemoryType(typeFilter uint32, properties vk.MemoryPropertyFlags) (uint32, error) {
	var memProps vk.PhysicalDeviceMemoryProperties
	vk.GetPhysicalDeviceMemoryProperties(vb.physicalDevice, &memProps)
	memProps.Deref()

	for i := uint32(0); i < memProps.MemoryTypeCount; i++ {
		memProps.MemoryTypes[i].Deref()
		if (typeFilter&(1<<i)) != 0 && (memProps.MemoryTypes[i].PropertyFlags&properties) == properties {
			return i, nil
		}
	}

	return 0, fmt.Errorf("failed to find suitable memory type")
}

// UpdatePipelineState updates the pipeline state (may require recreation)
func (vb *VulkanBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.fbzMode = fbzMode
	vb.alphaMode = alphaMode

	// Update software backend too
	vb.software.UpdatePipelineState(fbzMode, alphaMode)

	if !vb.initialized {
		return nil
	}

	// Create pipeline key from register values
	key := PipelineKeyFromRegisters(fbzMode, alphaMode)

	// Get or create the pipeline for this state
	pipeline, err := vb.getOrCreatePipeline(key)
	if err != nil {
		return err
	}

	vb.pipeline = pipeline
	vb.currentPipelineKey = key

	// Set depth clear value based on depth function
	switch key.DepthCompareOp {
	case VOODOO_DEPTH_GREATER, VOODOO_DEPTH_GREATEREQUAL:
		vb.depthClearValue = 0.0
	default:
		vb.depthClearValue = 1.0
	}

	return nil
}

// SetScissor sets the scissor rectangle
func (vb *VulkanBackend) SetScissor(left, top, right, bottom int) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.scissor = vk.Rect2D{
		Offset: vk.Offset2D{X: int32(left), Y: int32(top)},
		Extent: vk.Extent2D{Width: uint32(right - left), Height: uint32(bottom - top)},
	}

	vb.software.SetScissor(left, top, right, bottom)
}

// SetChromaKey sets the chroma key color for transparency keying
// Phase 3: Chroma Key support
func (vb *VulkanBackend) SetChromaKey(chromaKey uint32) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.chromaKey = chromaKey
	vb.software.SetChromaKey(chromaKey)
}

// SetTextureData uploads texture data for texture mapping
// Phase 4: Texture Mapping support
func (vb *VulkanBackend) SetTextureData(width, height int, data []byte, format int) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	// Update software backend
	vb.software.SetTextureData(width, height, data, format)

	// Upload to Vulkan if initialized
	if !vb.initialized {
		return
	}

	// Check if we need to recreate texture resources (size changed)
	if width != vb.textureWidth || height != vb.textureHeight {
		vb.destroyTextureImage()
		if err := vb.createTextureImage(width, height); err != nil {
			return
		}
	}

	vb.textureWidth = width
	vb.textureHeight = height

	// Upload texture data via staging buffer
	if err := vb.uploadTextureData(data, width, height); err != nil {
		return
	}

	// Update descriptor set with new texture
	vb.updateDescriptorSet()
}

// SetTextureEnabled enables or disables texture mapping
// Phase 4: Texture Mapping support
func (vb *VulkanBackend) SetTextureEnabled(enabled bool) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.textureEnabled = enabled
	vb.software.SetTextureEnabled(enabled)
}

// SetTextureWrapMode sets texture coordinate wrap/clamp mode
// Phase 4: Texture Mapping support
func (vb *VulkanBackend) SetTextureWrapMode(clampS, clampT bool) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.textureClampS = clampS
	vb.textureClampT = clampT
	vb.software.SetTextureWrapMode(clampS, clampT)
}

// SetColorPath sets the color combine mode from fbzColorPath register
// Phase 5: Color Combine support
func (vb *VulkanBackend) SetColorPath(fbzColorPath uint32) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.fbzColorPath = fbzColorPath
	vb.colorPathSet = true
	vb.software.SetColorPath(fbzColorPath)
}

// SetFogState sets the fog mode and color
// Phase 6: Fog support
func (vb *VulkanBackend) SetFogState(fogMode, fogColor uint32) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.fogMode = fogMode
	vb.fogColor = fogColor
	vb.software.SetFogState(fogMode, fogColor)
}

// FlushTriangles renders all triangles
func (vb *VulkanBackend) FlushTriangles(triangles []VoodooTriangle) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	// Update software backend for fallback when Vulkan not initialized
	if !vb.initialized {
		vb.software.FlushTriangles(triangles)
	}

	// Phase 6: GPU shaders now handle fog and dithering natively

	if !vb.initialized {
		return
	}

	// If no triangles, still need to render an empty frame with clear color
	if len(triangles) == 0 {
		vb.renderEmptyFrame()
		return
	}

	// Convert triangles to Vulkan vertices
	vertices := make([]VulkanVertex, 0, len(triangles)*3)
	for _, tri := range triangles {
		for _, v := range tri.Vertices {
			// Convert from Voodoo screen coords to NDC
			ndcX := (v.X/float32(vb.width))*2.0 - 1.0
			ndcY := (v.Y/float32(vb.height))*2.0 - 1.0

			// Normalize Z to Vulkan depth range [0, 1]
			// If Z is already in 0-1 range, use directly (modern usage)
			// Otherwise, divide by 65536 for Voodoo-style depth buffer values
			var ndcZ float32
			if v.Z >= 0 && v.Z <= 1.0 {
				ndcZ = v.Z // Already normalized
			} else if v.Z > 1.0 {
				ndcZ = v.Z / 65536.0 // Voodoo-style large Z values
				if ndcZ > 1 {
					ndcZ = 1
				}
			} else {
				ndcZ = 0 // Negative Z clamps to 0
			}

			vertices = append(vertices, VulkanVertex{
				Position: [3]float32{ndcX, ndcY, ndcZ},
				Color:    [4]float32{v.R, v.G, v.B, v.A},
				TexCoord: [2]float32{v.S, v.T}, // Phase 4: Texture coordinates
			})
		}
	}

	// Upload vertices to buffer
	var data unsafe.Pointer
	vk.MapMemory(vb.device, vb.vertexBufferMemory, 0, vk.DeviceSize(len(vertices)*int(unsafe.Sizeof(VulkanVertex{}))), 0, &data)
	vk.Memcopy(data, sliceToBytes(vertices))
	vk.UnmapMemory(vb.device, vb.vertexBufferMemory)

	// Wait for previous frame
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))
	vk.ResetFences(vb.device, 1, []vk.Fence{vb.fence})

	// Record command buffer
	vk.ResetCommandBuffer(vb.commandBuffer, 0)

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	vk.BeginCommandBuffer(vb.commandBuffer, &beginInfo)

	// Begin render pass with stored clear color and depth clear value
	clearValues := []vk.ClearValue{
		vk.NewClearValue([]float32{vb.clearColor[0], vb.clearColor[1], vb.clearColor[2], vb.clearColor[3]}),
		vk.NewClearDepthStencil(vb.depthClearValue, 0),
	}

	renderPassBegin := vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass:      vb.renderPass,
		Framebuffer:     vb.framebuffer,
		RenderArea:      vk.Rect2D{Offset: vk.Offset2D{X: 0, Y: 0}, Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)}},
		ClearValueCount: uint32(len(clearValues)),
		PClearValues:    clearValues,
	}

	vk.CmdBeginRenderPass(vb.commandBuffer, &renderPassBegin, vk.SubpassContentsInline)
	vk.CmdBindPipeline(vb.commandBuffer, vk.PipelineBindPointGraphics, vb.pipeline)

	// Push constants for alpha test, chroma key, texture mode, and color path (Phase 3-5)
	var texMode uint32
	if vb.textureEnabled {
		texMode = 1
	}
	// Phase 5: Set bit 1 of texMode to indicate colorPathSet
	if vb.colorPathSet {
		texMode |= 2
	}
	pushConstants := VoodooPushConstants{
		FbzMode:      vb.fbzMode,
		AlphaMode:    vb.alphaMode,
		ChromaKey:    vb.chromaKey,
		TextureMode:  texMode,
		FbzColorPath: vb.fbzColorPath,
		FogMode:      vb.fogMode,  // Phase 6: GPU fog
		FogColor:     vb.fogColor, // Phase 6: GPU fog
	}
	vk.CmdPushConstants(vb.commandBuffer, vb.pipelineLayout,
		vk.ShaderStageFlags(vk.ShaderStageFragmentBit), 0, 28,
		unsafe.Pointer(&pushConstants))

	// Bind descriptor set for texture sampling (Phase 4)
	if vb.descriptorSet != vk.NullDescriptorSet {
		vk.CmdBindDescriptorSets(vb.commandBuffer, vk.PipelineBindPointGraphics,
			vb.pipelineLayout, 0, 1, []vk.DescriptorSet{vb.descriptorSet}, 0, nil)
	}

	// Set dynamic scissor
	vk.CmdSetScissor(vb.commandBuffer, 0, 1, []vk.Rect2D{vb.scissor})

	// Bind vertex buffer and draw
	offsets := []vk.DeviceSize{0}
	vk.CmdBindVertexBuffers(vb.commandBuffer, 0, 1, []vk.Buffer{vb.vertexBuffer}, offsets)
	vk.CmdDraw(vb.commandBuffer, uint32(len(vertices)), 1, 0, 0)

	vk.CmdEndRenderPass(vb.commandBuffer)
	vk.EndCommandBuffer(vb.commandBuffer)

	// Submit
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vk.CommandBuffer{vb.commandBuffer},
	}

	vk.QueueSubmit(vb.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vb.fence)
}

// renderEmptyFrame renders a frame with just the clear color (no triangles)
func (vb *VulkanBackend) renderEmptyFrame() {
	// Wait for previous frame
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))
	vk.ResetFences(vb.device, 1, []vk.Fence{vb.fence})

	// Record command buffer
	vk.ResetCommandBuffer(vb.commandBuffer, 0)

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	vk.BeginCommandBuffer(vb.commandBuffer, &beginInfo)

	// Begin render pass with clear values (this will clear the buffer)
	clearValues := []vk.ClearValue{
		vk.NewClearValue([]float32{vb.clearColor[0], vb.clearColor[1], vb.clearColor[2], vb.clearColor[3]}),
		vk.NewClearDepthStencil(vb.depthClearValue, 0),
	}

	renderPassBegin := vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass:      vb.renderPass,
		Framebuffer:     vb.framebuffer,
		RenderArea:      vk.Rect2D{Offset: vk.Offset2D{X: 0, Y: 0}, Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)}},
		ClearValueCount: uint32(len(clearValues)),
		PClearValues:    clearValues,
	}

	vk.CmdBeginRenderPass(vb.commandBuffer, &renderPassBegin, vk.SubpassContentsInline)
	// No draw calls - just clear
	vk.CmdEndRenderPass(vb.commandBuffer)
	vk.EndCommandBuffer(vb.commandBuffer)

	// Submit
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vk.CommandBuffer{vb.commandBuffer},
	}

	vk.QueueSubmit(vb.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vb.fence)
}

// ClearFramebuffer clears the framebuffer
func (vb *VulkanBackend) ClearFramebuffer(color uint32) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.software.ClearFramebuffer(color)

	// Store clear color for Vulkan (ARGB format to RGBA floats)
	vb.clearColor[0] = float32((color>>16)&0xFF) / 255.0 // R
	vb.clearColor[1] = float32((color>>8)&0xFF) / 255.0  // G
	vb.clearColor[2] = float32(color&0xFF) / 255.0       // B
	vb.clearColor[3] = 1.0                               // A (opaque)
	vb.needsClear = true
}

// SwapBuffers presents the frame
func (vb *VulkanBackend) SwapBuffers(waitVSync bool) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	// Phase 6: GPU shaders now handle fog and dithering natively

	if !vb.initialized {
		vb.software.SwapBuffers(waitVSync)
		return
	}

	// Wait for rendering to complete
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))

	// Read back framebuffer
	vb.readbackFramebuffer()

	vb.software.SwapBuffers(waitVSync)
}

// readbackFramebuffer copies the rendered image to CPU memory
func (vb *VulkanBackend) readbackFramebuffer() {
	vk.ResetFences(vb.device, 1, []vk.Fence{vb.fence})
	vk.ResetCommandBuffer(vb.commandBuffer, 0)

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	vk.BeginCommandBuffer(vb.commandBuffer, &beginInfo)

	// Copy image to staging buffer
	region := vk.BufferImageCopy{
		BufferOffset:      0,
		BufferRowLength:   0,
		BufferImageHeight: 0,
		ImageSubresource: vk.ImageSubresourceLayers{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			MipLevel:       0,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
		ImageOffset: vk.Offset3D{X: 0, Y: 0, Z: 0},
		ImageExtent: vk.Extent3D{Width: uint32(vb.width), Height: uint32(vb.height), Depth: 1},
	}

	vk.CmdCopyImageToBuffer(vb.commandBuffer, vb.colorImage, vk.ImageLayoutTransferSrcOptimal, vb.stagingBuffer, 1, []vk.BufferImageCopy{region})

	vk.EndCommandBuffer(vb.commandBuffer)

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vk.CommandBuffer{vb.commandBuffer},
	}

	vk.QueueSubmit(vb.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vb.fence)
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))

	// Map staging buffer and copy to output
	var data unsafe.Pointer
	vk.MapMemory(vb.device, vb.stagingBufferMemory, 0, vk.DeviceSize(len(vb.outputFrame)), 0, &data)
	copy(vb.outputFrame, (*[1 << 30]byte)(data)[:len(vb.outputFrame)])
	vk.UnmapMemory(vb.device, vb.stagingBufferMemory)
}

// GetFrame returns the rendered frame
func (vb *VulkanBackend) GetFrame() []byte {
	vb.mutex.RLock()
	defer vb.mutex.RUnlock()

	// Phase 6: GPU shaders now handle fog and dithering natively

	if vb.initialized {
		return vb.outputFrame
	}
	return vb.software.GetFrame()
}

// Destroy cleans up all resources
func (vb *VulkanBackend) Destroy() {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	if vb.initialized {
		vk.DeviceWaitIdle(vb.device)

		vb.destroyFence()
		vb.destroyStagingBuffer()
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyTextureResources() // Phase 4: Clean up texture resources
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
	}

	if vb.software != nil {
		vb.software.Destroy()
	}
}

// Cleanup helpers
func (vb *VulkanBackend) destroyInstance() {
	if vb.instance != nil {
		vk.DestroyInstance(vb.instance, nil)
		vb.instance = nil
	}
}

func (vb *VulkanBackend) destroyDevice() {
	if vb.device != nil {
		vk.DestroyDevice(vb.device, nil)
		vb.device = nil
	}
}

func (vb *VulkanBackend) destroyCommandPool() {
	if vb.commandPool != vk.NullCommandPool {
		vk.DestroyCommandPool(vb.device, vb.commandPool, nil)
		vb.commandPool = vk.NullCommandPool
	}
}

func (vb *VulkanBackend) destroyOffscreenImages() {
	if vb.colorImageView != vk.NullImageView {
		vk.DestroyImageView(vb.device, vb.colorImageView, nil)
	}
	if vb.colorImage != vk.NullImage {
		vk.DestroyImage(vb.device, vb.colorImage, nil)
	}
	if vb.colorImageMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.colorImageMemory, nil)
	}
	if vb.depthImageView != vk.NullImageView {
		vk.DestroyImageView(vb.device, vb.depthImageView, nil)
	}
	if vb.depthImage != vk.NullImage {
		vk.DestroyImage(vb.device, vb.depthImage, nil)
	}
	if vb.depthImageMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.depthImageMemory, nil)
	}
}

func (vb *VulkanBackend) destroyRenderPass() {
	if vb.renderPass != vk.NullRenderPass {
		vk.DestroyRenderPass(vb.device, vb.renderPass, nil)
		vb.renderPass = vk.NullRenderPass
	}
}

func (vb *VulkanBackend) destroyFramebuffer() {
	if vb.framebuffer != vk.NullFramebuffer {
		vk.DestroyFramebuffer(vb.device, vb.framebuffer, nil)
		vb.framebuffer = vk.NullFramebuffer
	}
}

func (vb *VulkanBackend) destroyPipeline() {
	// Destroy all pipeline variants in the cache
	for key, pipeline := range vb.pipelineVariants {
		if pipeline != vk.NullPipeline {
			vk.DestroyPipeline(vb.device, pipeline, nil)
		}
		delete(vb.pipelineVariants, key)
	}

	// Clear the current pipeline reference (it was in the cache)
	vb.pipeline = vk.NullPipeline

	if vb.pipelineLayout != vk.NullPipelineLayout {
		vk.DestroyPipelineLayout(vb.device, vb.pipelineLayout, nil)
		vb.pipelineLayout = vk.NullPipelineLayout
	}
	if vb.vertShaderModule != vk.NullShaderModule {
		vk.DestroyShaderModule(vb.device, vb.vertShaderModule, nil)
		vb.vertShaderModule = vk.NullShaderModule
	}
	if vb.fragShaderModule != vk.NullShaderModule {
		vk.DestroyShaderModule(vb.device, vb.fragShaderModule, nil)
		vb.fragShaderModule = vk.NullShaderModule
	}
}

func (vb *VulkanBackend) destroyVertexBuffer() {
	if vb.vertexBuffer != vk.NullBuffer {
		vk.DestroyBuffer(vb.device, vb.vertexBuffer, nil)
	}
	if vb.vertexBufferMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.vertexBufferMemory, nil)
	}
}

func (vb *VulkanBackend) destroyStagingBuffer() {
	if vb.stagingBuffer != vk.NullBuffer {
		vk.DestroyBuffer(vb.device, vb.stagingBuffer, nil)
	}
	if vb.stagingBufferMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.stagingBufferMemory, nil)
	}
}

func (vb *VulkanBackend) destroyFence() {
	if vb.fence != vk.NullFence {
		vk.DestroyFence(vb.device, vb.fence, nil)
		vb.fence = vk.NullFence
	}
}

// Helper functions for Vulkan

// safeString creates a null-terminated string for Vulkan
func safeString(s string) string {
	return s + "\x00"
}

// sliceUint32 converts a byte slice to a uint32 slice for SPIR-V
func sliceUint32(data []byte) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&data[0])), len(data)/4)
}

// sliceToBytes converts a vertex slice to bytes
func sliceToBytes(vertices []VulkanVertex) []byte {
	size := len(vertices) * int(unsafe.Sizeof(VulkanVertex{}))
	return unsafe.Slice((*byte)(unsafe.Pointer(&vertices[0])), size)
}
