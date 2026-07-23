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
	"runtime"
	"sync"
	"unsafe"
)

// =============================================================================
// Software Rasterizer Backend
// =============================================================================

// VoodooSoftwareBackend implements software rasterization as a fallback
type VoodooSoftwareBackend struct {
	// mutex guards the live register state (the Set* fields below).
	// Setters take only this lock, so guest-forwarded state writes never
	// wait for an in-flight raster.
	mutex sync.RWMutex

	// fbMu guards the framebuffers (color/depth/front/back and their
	// dimensions). Rasterisation runs entirely under fbMu with state
	// carried in per-flush snapshots, so a full-frame raster blocks only
	// other framebuffer operations, never state writes.
	fbMu sync.Mutex

	// tileBinner is the reused binning state for the tiled raster path. Only
	// ever touched under fbMu. nil until the first tiled flush.
	tileBinner *voodooTileBinner

	// Framebuffer
	width, height int
	colorBuffer   []byte    // RGBA
	depthBuffer   []float32 // Z values

	// State
	fbzMode      uint32
	alphaMode    uint32
	chromaKey    uint32 // Chroma key color (RGB packed)
	chromaRange  uint32 // Chroma max color when non-zero; chromaKey is min
	fbzColorPath uint32 // Color combine mode
	colorPathSet bool   // Track if color path was explicitly set
	stipple      uint32
	lfbMode      uint32
	tlod         uint32
	texBase      [9]uint32
	slopes       VoodooSlopes
	slopesValid  bool
	fogTable     [VOODOO_FOG_TABLE_SIZE]uint32
	palette      [VOODOO_PALETTE_SIZE]uint32

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
	textureMode    uint32
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
	b.fbMu.Lock()
	defer b.fbMu.Unlock()

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

func (b *VoodooSoftwareBackend) Resize(width, height int) error {
	return b.Init(width, height)
}

func (b *VoodooSoftwareBackend) Reset() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.fbMu.Lock()
	defer b.fbMu.Unlock()

	for i := range b.colorBuffer {
		b.colorBuffer[i] = 0
	}
	for i := range b.frontBuffer {
		b.frontBuffer[i] = 0
	}
	for i := range b.backBuffer {
		b.backBuffer[i] = 0
	}
	for i := range b.depthBuffer {
		b.depthBuffer[i] = math.MaxFloat32
	}

	b.fbzMode = VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_LESS << 5)
	b.alphaMode = 0
	b.chromaKey = 0
	b.chromaRange = 0
	b.fbzColorPath = 0
	b.colorPathSet = false
	b.stipple = 0
	b.lfbMode = 0
	b.tlod = 0
	b.texBase = [9]uint32{}
	b.slopes = VoodooSlopes{}
	b.slopesValid = false
	b.fogTable = [VOODOO_FOG_TABLE_SIZE]uint32{}
	b.palette = [VOODOO_PALETTE_SIZE]uint32{}
	b.pipelineKey = PipelineKeyFromRegisters(b.fbzMode, b.alphaMode)
	b.scissorLeft = 0
	b.scissorTop = 0
	b.scissorRight = b.width
	b.scissorBottom = b.height
	b.isBackBuf = false
	b.textureData = nil
	b.textureWidth = 0
	b.textureHeight = 0
	b.textureFormat = 0
	b.textureMode = 0
	b.textureEnabled = false
	b.textureClampS = false
	b.textureClampT = false
	b.fogMode = 0
	b.fogColor = 0
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

func (b *VoodooSoftwareBackend) SetChromaRange(chromaRange uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.chromaRange = chromaRange
}

func (b *VoodooSoftwareBackend) SetStipple(stipple uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.stipple = stipple
}

func (b *VoodooSoftwareBackend) SetLFBMode(lfbMode uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.lfbMode = lfbMode
}

func (b *VoodooSoftwareBackend) SetTexBase(level int, addr uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if level >= 0 && level < len(b.texBase) {
		b.texBase[level] = addr
	}
}

func (b *VoodooSoftwareBackend) SetTLOD(tlod uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.tlod = tlod
}

func (b *VoodooSoftwareBackend) SetSlopes(slopes VoodooSlopes, valid bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.slopes = slopes
	b.slopesValid = valid
}

func (b *VoodooSoftwareBackend) SetFogTableEntry(index int, value uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if index >= 0 && index < len(b.fogTable) {
		b.fogTable[index] = value
	}
}

func (b *VoodooSoftwareBackend) SetPaletteEntry(index int, value uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if index >= 0 && index < len(b.palette) {
		b.palette[index] = value
	}
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

func (b *VoodooSoftwareBackend) SetTextureMode(textureMode uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.textureMode = textureMode
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
	return sampleVoodooTexel(b.textureData, b.textureWidth, b.textureHeight,
		b.textureClampS, b.textureClampT, s, t)
}

// sampleVoodooTexel is the sampling core with the texture parameters
// passed in, so the rasteriser can hoist the field loads per triangle.
func sampleVoodooTexel(data []byte, width, height int, clampS, clampT bool, s, t float32) (r, g, bVal, a float32) {
	if data == nil || width == 0 || height == 0 {
		return 1.0, 1.0, 1.0, 1.0 // White if no texture
	}

	// Apply wrap/clamp mode
	if clampS {
		s = clampf(s, 0, 1)
	} else {
		// Wrap (repeat) mode - use fmod
		s = s - float32(math.Floor(float64(s)))
		if s < 0 {
			s += 1.0
		}
	}

	if clampT {
		t = clampf(t, 0, 1)
	} else {
		// Wrap (repeat) mode
		t = t - float32(math.Floor(float64(t)))
		if t < 0 {
			t += 1.0
		}
	}

	// Point sampling (nearest neighbor)
	texX := int(s * float32(width))
	texY := int(t * float32(height))

	// Clamp to texture bounds
	if texX >= width {
		texX = width - 1
	}
	if texY >= height {
		texY = height - 1
	}
	if texX < 0 {
		texX = 0
	}
	if texY < 0 {
		texY = 0
	}

	// Sample texel (assuming RGBA format)
	idx := (texY*width + texX) * 4
	if idx+3 < len(data) {
		r = float32(data[idx+0]) / 255.0
		g = float32(data[idx+1]) / 255.0
		bVal = float32(data[idx+2]) / 255.0
		a = float32(data[idx+3]) / 255.0
	} else {
		r, g, bVal, a = 1.0, 1.0, 1.0, 1.0
	}

	return r, g, bVal, a
}

// combineColors combines vertex and texture colors based on fbzColorPath register
func (b *VoodooSoftwareBackend) combineColors(vertR, vertG, vertB, vertA, texR, texG, texB, texA float32) (r, g, bVal, a float32) {
	return combineVoodooColors(b.fbzColorPath, b.colorPathSet, vertR, vertG, vertB, vertA, texR, texG, texB, texA)
}

// combineVoodooColors is the colour-combine core with the colour-path
// state passed in, so the rasteriser can hoist the field loads per
// triangle.
func combineVoodooColors(fbzColorPath uint32, colorPathSet bool, vertR, vertG, vertB, vertA, texR, texG, texB, texA float32) (r, g, bVal, a float32) {
	// Default to modulate for backward compatibility (if color path was never explicitly set)
	if !colorPathSet {
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}

	// Handle special convenience modes first (these have specific bit patterns)
	switch fbzColorPath {
	case VOODOO_COMBINE_ADD:
		return vertR + texR, vertG + texG, vertB + texB, vertA + texA
	case VOODOO_COMBINE_MODULATE:
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}

	// Extract color combine mode from fbzColorPath
	rgbSelect := fbzColorPath & VOODOO_FCP_RGB_SELECT_MASK
	ccMode := (fbzColorPath >> VOODOO_FCP_CC_MSELECT_SHIFT) & 0x7

	switch rgbSelect {
	case VOODOO_CC_ITERATED:
		return vertR, vertG, vertB, vertA
	case VOODOO_CC_TEXTURE:
		return texR, texG, texB, texA
	}

	switch ccMode {
	case VOODOO_CC_ZERO:
		return 0, 0, 0, 0
	case VOODOO_CC_CSUB_CL:
		return texR - vertR, texG - vertG, texB - vertB, texA - vertA
	case VOODOO_CC_ALOCAL:
		return vertR * vertA, vertG * vertA, vertB * vertA, vertA * vertA
	case VOODOO_CC_AOTHER:
		return vertR * texA, vertG * texA, vertB * texA, vertA * texA
	case VOODOO_CC_CLOCAL:
		return vertR, vertG, vertB, vertA
	case VOODOO_CC_ALOCAL_T:
		return texR * vertA, texG * vertA, texB * vertA, texA * vertA
	case VOODOO_CC_CLOC_MUL:
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	case VOODOO_CC_AOTHER_T:
		return texR * texA, texG * texA, texB * texA, texA * texA
	default:
		return vertR * texR, vertG * texG, vertB * texB, vertA * texA
	}
}

// FlushTriangles rasterizes all triangles in the batch
func (b *VoodooSoftwareBackend) FlushTriangles(triangles []VoodooTriangle) {
	// Each triangle rasterises under the state bound at its
	// triangleCMD write (hardware-accurate binding); consecutive
	// triangles share snapshots, so the working state is rebuilt only
	// on group boundaries. Triangles without a snapshot (nil State)
	// use the live register state captured on entry.
	//
	// State is carried in a per-flush snapshot rather than installed
	// into the backend fields, so the raster holds only the
	// framebuffer lock and guest-forwarded state writes proceed
	// concurrently with the pixel loop.
	b.mutex.RLock()
	live := b.captureLiveStateLocked()
	b.mutex.RUnlock()

	b.fbMu.Lock()
	defer b.fbMu.Unlock()

	if voodooTileRasterEnabled() {
		b.flushTrianglesTiled(triangles, live)
		return
	}

	var applied *VoodooRasterState
	state := live
	for i := range triangles {
		if st := triangles[i].State; st != nil && st != applied {
			state = softwareLiveStateFromSnapshot(st)
			applied = st
		}
		b.rasterizeTriangle(&triangles[i], &state)
	}
}

// softwareLiveState preserves the raster-affecting backend fields
// across a stamped-triangle flush. The caller holds b.mutex.
type softwareLiveState struct {
	fbzMode, alphaMode           uint32
	pipelineKey                  PipelineKey
	fbzColorPath                 uint32
	colorPathSet                 bool
	textureMode                  uint32
	textureEnabled               bool
	textureClampS, textureClampT bool
	fogMode, fogColor            uint32
	chromaKey, chromaRange       uint32
	stipple                      uint32
	scissorLeft, scissorTop      int
	scissorRight, scissorBottom  int
	slopes                       VoodooSlopes
	slopesValid                  bool
	textureData                  []byte
	textureWidth, textureHeight  int
	textureFormat                int
}

func (b *VoodooSoftwareBackend) captureLiveStateLocked() softwareLiveState {
	return softwareLiveState{
		fbzMode: b.fbzMode, alphaMode: b.alphaMode,
		pipelineKey:  b.pipelineKey,
		fbzColorPath: b.fbzColorPath, colorPathSet: b.colorPathSet,
		textureMode: b.textureMode, textureEnabled: b.textureEnabled,
		textureClampS: b.textureClampS, textureClampT: b.textureClampT,
		fogMode: b.fogMode, fogColor: b.fogColor,
		chromaKey: b.chromaKey, chromaRange: b.chromaRange,
		stipple:     b.stipple,
		scissorLeft: b.scissorLeft, scissorTop: b.scissorTop,
		scissorRight: b.scissorRight, scissorBottom: b.scissorBottom,
		slopes: b.slopes, slopesValid: b.slopesValid,
		textureData: b.textureData, textureWidth: b.textureWidth,
		textureHeight: b.textureHeight, textureFormat: b.textureFormat,
	}
}

// softwareLiveStateFromSnapshot builds the raster working state from a
// raster-state snapshot. Field derivations mirror the individual
// backend setters. Snapshot textures are immutable, so Data is
// referenced without copying.
func softwareLiveStateFromSnapshot(st *VoodooRasterState) softwareLiveState {
	s := softwareLiveState{
		fbzMode:        st.FbzMode,
		alphaMode:      st.AlphaMode,
		pipelineKey:    PipelineKeyFromRegisters(st.FbzMode, st.AlphaMode),
		fbzColorPath:   st.FbzColorPath,
		colorPathSet:   st.ColorPathWritten,
		textureMode:    st.TextureMode,
		textureEnabled: st.TextureMode&1 != 0,
		textureClampS:  st.TextureMode&(1<<5) != 0,
		textureClampT:  st.TextureMode&(1<<6) != 0,
		fogMode:        st.FogMode,
		fogColor:       st.FogColor,
		chromaKey:      st.ChromaKey, chromaRange: st.ChromaRange,
		stipple:     st.Stipple,
		scissorLeft: st.ClipLeft, scissorRight: st.ClipRight,
		scissorTop: st.ClipTop, scissorBottom: st.ClipBottom,
		slopes: st.Slopes, slopesValid: st.SlopesValid,
	}
	if st.Texture != nil {
		s.textureData = st.Texture.Data
		s.textureWidth = st.Texture.Width
		s.textureHeight = st.Texture.Height
		s.textureFormat = st.Texture.Format
	}
	return s
}

// ClearFramebuffer clears the color and depth buffers
func (b *VoodooSoftwareBackend) ClearFramebuffer(color uint32) {
	// Clear depth based on the current depth function
	b.mutex.RLock()
	depthCompareOp := b.pipelineKey.DepthCompareOp
	b.mutex.RUnlock()
	var depthClearValue float32
	switch depthCompareOp {
	case VOODOO_DEPTH_GREATER, VOODOO_DEPTH_GREATEREQUAL:
		depthClearValue = 0.0
	default:
		depthClearValue = math.MaxFloat32
	}

	b.fbMu.Lock()
	defer b.fbMu.Unlock()
	b.clearLocked(color, depthClearValue)
}

// ClearFramebufferWithDepth clears with an explicit depth-clear value.
// Used when replaying a recorded GPU flush: the depth mode current at
// that flush may differ from the mode current now.
func (b *VoodooSoftwareBackend) ClearFramebufferWithDepth(color uint32, depthClearValue float32) {
	b.fbMu.Lock()
	defer b.fbMu.Unlock()
	b.clearLocked(color, depthClearValue)
}

func (b *VoodooSoftwareBackend) clearLocked(color uint32, depthClearValue float32) {
	// Extract RGBA from packed color (assuming ARGB format)
	r := byte((color >> 16) & 0xFF)
	g := byte((color >> 8) & 0xFF)
	bVal := byte(color & 0xFF)
	a := byte((color >> 24) & 0xFF)
	if a == 0 {
		a = 255 // Default to opaque
	}

	// Clear color buffer (packed dword fill; same little-endian byte
	// layout the per-pixel writes use)
	if len(b.colorBuffer) >= 4 {
		packed := uint32(r) | uint32(g)<<8 | uint32(bVal)<<16 | uint32(a)<<24
		buf := unsafe.Slice((*uint32)(unsafe.Pointer(&b.colorBuffer[0])), len(b.colorBuffer)/4)
		for i := range buf {
			buf[i] = packed
		}
	}

	for i := range b.depthBuffer {
		b.depthBuffer[i] = depthClearValue
	}
}

// SwapBuffers swaps front and back buffers
func (b *VoodooSoftwareBackend) SwapBuffers(waitVSync bool) {
	b.fbMu.Lock()
	defer b.fbMu.Unlock()

	// Copy color buffer to front buffer
	copy(b.frontBuffer, b.colorBuffer)
}

// GetFrame returns the current front buffer
func (b *VoodooSoftwareBackend) GetFrame() []byte {
	b.fbMu.Lock()
	defer b.fbMu.Unlock()

	return b.frontBuffer
}

// Destroy cleans up resources
func (b *VoodooSoftwareBackend) Destroy() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.fbMu.Lock()
	defer b.fbMu.Unlock()

	b.colorBuffer = nil
	b.depthBuffer = nil
	b.frontBuffer = nil
	b.backBuffer = nil
}

// voodooTriangleSetup carries the per-triangle constants used by the
// row rasteriser. Everything here is derived once per triangle so the
// per-pixel loop performs no repeated flag parsing, fixed-point
// conversion, or draw-target construction. All expressions preserve the
// exact floating-point operation trees of the previous per-pixel code:
// this backend is the conformance reference, so optimisations must be
// bit-exact.
type voodooTriangleSetup struct {
	v0, v1, v2 *VoodooVertex
	invArea    float32

	// Edge equations: wN(px,py) = (px-aN.X)*eN - (py-aN.Y)*fN, the same
	// operation tree as edgeFunction, so hoisting the (py-aN.Y)*fN term
	// per row is bit-exact.
	e0, f0, e1, f1, e2, f2 float32

	minX, maxX int

	depthEnable, depthWrite, rgbWrite bool
	depthFunc                         int
	alphaTestEnable                   bool
	alphaTestFunc                     int
	alphaTestRef                      float32
	chromaKeyEnable                   bool
	chromaKey, chromaRange            uint32
	alphaBlendEnable                  bool
	srcBlendFactor, dstBlendFactor    int
	fogEnable                         bool
	fogR, fogG, fogB                  float32
	ditherEnable, dither2x2           bool
	stippleEnable                     bool
	stipple                           uint32
	yFlip                             bool
	forceOpaqueAlpha                  bool
	targets                           [][]byte
	texActive                         bool
	texPerspective                    bool
	texData                           []byte
	texWidth, texHeight               int
	texClampS, texClampT              bool
	fbzColorPath                      uint32
	colorPathSet                      bool

	// Slope-register interpolation deltas, converted from fixed point
	// once per triangle (identical values to the per-pixel conversions
	// they replace).
	slopesValid                        bool
	drdx, drdy, dgdx, dgdy, dbdx, dbdy float32
	dadx, dady                         float32
	dzdx, dzdy                         float32
	dsdx, dsdy, dtdx, dtdy             float32
}

// rasterizeTriangle performs software triangle rasterization. Raster
// state comes from the caller's snapshot; the backend contributes only
// the framebuffers (the caller holds fbMu).
func (b *VoodooSoftwareBackend) rasterizeTriangle(tri *VoodooTriangle, st *softwareLiveState) {
	setup, minY, maxY, ok := b.buildTriangleSetup(tri, st)
	if !ok {
		return
	}
	b.rasterizeSetupBanded(&setup, minY, maxY)
}

// buildTriangleSetup derives the read-only per-triangle raster setup and its
// row range from a triangle and the state bound at its submission. It reports
// false for triangles that cover nothing: degenerate area, or a bounding box
// emptied by the screen bounds or the scissor rectangle. The setup it returns
// is self-contained, so it can be rasterised now, or queued and rasterised
// later by a tile worker, without consulting backend state again.
func (b *VoodooSoftwareBackend) buildTriangleSetup(tri *VoodooTriangle, st *softwareLiveState) (voodooTriangleSetup, int, int, bool) {
	v0 := &tri.Vertices[0]
	v1 := &tri.Vertices[1]
	v2 := &tri.Vertices[2]

	// Check if clipping is enabled
	enableClipping := (st.fbzMode & VOODOO_FBZ_CLIPPING) != 0

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
		if minX < st.scissorLeft {
			minX = st.scissorLeft
		}
		if minY < st.scissorTop {
			minY = st.scissorTop
		}
		if maxX > st.scissorRight {
			maxX = st.scissorRight
		}
		if maxY > st.scissorBottom {
			maxY = st.scissorBottom
		}
	}

	// Compute triangle area (2x for efficiency)
	area := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, v2.X, v2.Y)
	if area == 0 {
		return voodooTriangleSetup{}, 0, 0, false // Degenerate triangle
	}

	// Handle backface culling (if area is negative, triangle is back-facing)
	if area < 0 {
		// Swap vertices to make it front-facing
		v0, v2 = v2, v0
		area = -area
	}

	setup := voodooTriangleSetup{
		v0: v0, v1: v1, v2: v2,
		invArea: 1.0 / area,
		e0:      v2.Y - v1.Y, f0: v2.X - v1.X,
		e1: v0.Y - v2.Y, f1: v0.X - v2.X,
		e2: v1.Y - v0.Y, f2: v1.X - v0.X,
		minX: minX, maxX: maxX,

		depthEnable:      (st.fbzMode & VOODOO_FBZ_DEPTH_ENABLE) != 0,
		depthWrite:       (st.fbzMode & VOODOO_FBZ_DEPTH_WRITE) != 0,
		rgbWrite:         (st.fbzMode & VOODOO_FBZ_RGB_WRITE) != 0,
		depthFunc:        int((st.fbzMode >> 5) & 0x7),
		alphaTestEnable:  (st.alphaMode & VOODOO_ALPHA_TEST_EN) != 0,
		alphaTestFunc:    int((st.alphaMode >> 1) & 0x7),
		alphaTestRef:     float32((st.alphaMode>>24)&0xFF) / 255.0,
		chromaKeyEnable:  (st.fbzMode & VOODOO_FBZ_CHROMAKEY) != 0,
		chromaKey:        st.chromaKey,
		chromaRange:      st.chromaRange,
		alphaBlendEnable: (st.alphaMode & VOODOO_ALPHA_BLEND_EN) != 0,
		srcBlendFactor:   st.pipelineKey.SrcBlendFactor,
		dstBlendFactor:   st.pipelineKey.DstBlendFactor,
		fogEnable:        (st.fogMode & VOODOO_FOG_ENABLE) != 0,
		ditherEnable:     (st.fbzMode & VOODOO_FBZ_DITHER) != 0,
		dither2x2:        (st.fbzMode & VOODOO_FBZ_DITHER_2X2) != 0,
		stippleEnable:    (st.fbzMode & VOODOO_FBZ_STIPPLE) != 0,
		stipple:          st.stipple,
		yFlip:            st.fbzMode&VOODOO_FBZ_Y_ORIGIN != 0,
		forceOpaqueAlpha: st.fbzMode&VOODOO_FBZ_ALPHA_PLANES == 0,
		targets:          b.drawTargetsFor(st.fbzMode),
		texActive:        st.textureEnabled && st.textureData != nil,
		texPerspective:   st.textureMode&VOODOO_TEX_PERSPECTIVE != 0,
		texData:          st.textureData,
		texWidth:         st.textureWidth,
		texHeight:        st.textureHeight,
		texClampS:        st.textureClampS,
		texClampT:        st.textureClampT,
		fbzColorPath:     st.fbzColorPath,
		colorPathSet:     st.colorPathSet,
		slopesValid:      st.slopesValid,
	}
	if setup.fogEnable {
		setup.fogR = float32((st.fogColor>>16)&0xFF) / 255.0
		setup.fogG = float32((st.fogColor>>8)&0xFF) / 255.0
		setup.fogB = float32(st.fogColor&0xFF) / 255.0
	}
	if setup.slopesValid {
		setup.drdx = fixed12_12ToFloat(st.slopes.DRDX)
		setup.drdy = fixed12_12ToFloat(st.slopes.DRDY)
		setup.dgdx = fixed12_12ToFloat(st.slopes.DGDX)
		setup.dgdy = fixed12_12ToFloat(st.slopes.DGDY)
		setup.dbdx = fixed12_12ToFloat(st.slopes.DBDX)
		setup.dbdy = fixed12_12ToFloat(st.slopes.DBDY)
		setup.dadx = fixed12_12ToFloat(st.slopes.DADX)
		setup.dady = fixed12_12ToFloat(st.slopes.DADY)
		setup.dzdx = fixed20_12ToFloat(st.slopes.DZDX)
		setup.dzdy = fixed20_12ToFloat(st.slopes.DZDY)
		setup.dsdx = fixed14_18ToFloat(st.slopes.DSDX)
		setup.dsdy = fixed14_18ToFloat(st.slopes.DSDY)
		setup.dtdx = fixed14_18ToFloat(st.slopes.DTDX)
		setup.dtdy = fixed14_18ToFloat(st.slopes.DTDY)
	}

	return setup, minY, maxY, true
}

// rasterizeSetupBanded rasterises one prepared setup over [minY, maxY).
func (b *VoodooSoftwareBackend) rasterizeSetupBanded(setup *voodooTriangleSetup, minY, maxY int) {
	// Parallelise large triangles across row bands. Bands write disjoint
	// rows (also under Y-flip), each pixel's result is a deterministic
	// function of its coordinates, and the setup is read-only, so the
	// output is bit-identical to the sequential path.
	rows := maxY - minY
	const bandMinRows = 64
	if rows >= bandMinRows && setup.maxX-setup.minX >= 64 {
		if workers := min(runtime.NumCPU(), rows/(bandMinRows/2)); workers > 1 {
			step := (rows + workers - 1) / workers
			var wg sync.WaitGroup
			for y := minY; y < maxY; y += step {
				y0, y1 := y, min(y+step, maxY)
				wg.Add(1)
				go func() {
					defer wg.Done()
					b.rasterizeRows(setup, y0, y1)
				}()
			}
			wg.Wait()
			return
		}
	}
	b.rasterizeRows(setup, minY, maxY)
}

// rasterizeRows rasterises the triangle rows [minY, maxY) using the
// per-triangle setup. Per-pixel results are bit-identical to the
// previous single-loop implementation.
// voodooRasterizeRowsSIMDFn is set by assignSIMDKernels on supported hosts to
// the bit-exact SIMD row rasteriser. nil on every other build, so rasterizeRows
// runs the scalar reference. The scalar path stays the canonical conformance
// reference; SIMD is purely additive and only for eligible setups.
var voodooRasterizeRowsSIMDFn func(b *VoodooSoftwareBackend, s *voodooTriangleSetup, minY, maxY int)

// voodooSetupSIMDEligible reports whether a setup is SIMD-eligible: slope-
// register affine interpolation and RGB write. Texture, alpha test, chroma key,
// dither, stipple and alpha blending were each added feature by feature, each
// behind its own bit-exact differential gate (texture, the quantising stages and
// blending run as scalar hybrids inside the lane loop). Blending reads each
// target's own destination pixel and multiplies through per-target factors, so
// only its arithmetic stays scalar; the interpolation ahead of it, which is the
// bulk of the work, is vectorised like any other setup.
func voodooSetupSIMDEligible(s *voodooTriangleSetup) bool {
	return s.slopesValid && s.rgbWrite
}

func (b *VoodooSoftwareBackend) rasterizeRows(s *voodooTriangleSetup, minY, maxY int) {
	if voodooRasterizeRowsSIMDFn != nil && voodooSetupSIMDEligible(s) {
		voodooRasterizeRowsSIMDFn(b, s, minY, maxY)
		return
	}
	v0, v1, v2 := s.v0, s.v1, s.v2

	for y := minY; y < maxY; y++ {
		py := float32(y) + 0.5

		// Row-constant halves of the edge functions (bit-exact hoist of
		// the (py-aN.Y)*fN products).
		t0 := (py - v1.Y) * s.f0
		t1 := (py - v2.Y) * s.f1
		t2 := (py - v0.Y) * s.f2

		dstY := y
		if s.yFlip {
			dstY = b.height - 1 - y
		}
		rowBase := dstY * b.width

		inside := false
		for x := s.minX; x < s.maxX; x++ {
			px := float32(x) + 0.5

			// Compute barycentric coordinates
			w0 := (px-v1.X)*s.e0 - t0
			w1 := (px-v2.X)*s.e1 - t1
			w2 := (px-v0.X)*s.e2 - t2

			// Check if pixel is inside triangle. The per-row span is
			// contiguous (each edge test flips at most once as px
			// increases), so leaving the span ends the row.
			if !(w0 >= 0 && w1 >= 0 && w2 >= 0) {
				if inside {
					break
				}
				continue
			}
			inside = true

			if s.stippleEnable && !stippleAllowsVoodooPixel(s.stipple, x, y) {
				continue
			}

			// Normalize barycentric coordinates
			w0 *= s.invArea
			w1 *= s.invArea
			w2 *= s.invArea

			var r, g, bVal, a, z, sTex, tTex float32
			if !s.slopesValid {
				r = w0*v0.R + w1*v1.R + w2*v2.R
				g = w0*v0.G + w1*v1.G + w2*v2.G
				bVal = w0*v0.B + w1*v1.B + w2*v2.B
				a = w0*v0.A + w1*v1.A + w2*v2.A
				z = w0*v0.Z + w1*v1.Z + w2*v2.Z
				if s.texActive {
					if !s.texPerspective {
						sTex = w0*v0.S + w1*v1.S + w2*v2.S
						tTex = w0*v0.T + w1*v1.T + w2*v2.T
					} else if denom := w0*v0.W + w1*v1.W + w2*v2.W; denom != 0 {
						sTex = (w0*v0.S*v0.W + w1*v1.S*v1.W + w2*v2.S*v2.W) / denom
						tTex = (w0*v0.T*v0.W + w1*v1.T*v1.W + w2*v2.T*v2.W) / denom
					}
				}
			} else {
				dx := px - v0.X
				dy := py - v0.Y
				r = v0.R + dx*s.drdx + dy*s.drdy
				g = v0.G + dx*s.dgdx + dy*s.dgdy
				bVal = v0.B + dx*s.dbdx + dy*s.dbdy
				a = v0.A + dx*s.dadx + dy*s.dady
				z = v0.Z + dx*s.dzdx + dy*s.dzdy
				sTex = v0.S + dx*s.dsdx + dy*s.dsdy
				tTex = v0.T + dx*s.dtdx + dy*s.dtdy
			}

			// Depth test
			pixelIndex := rowBase + x
			if s.depthEnable {
				if !b.depthTest(z, b.depthBuffer[pixelIndex], s.depthFunc) {
					continue
				}
			}

			// Texture mapping with color combine
			if s.texActive {
				texR, texG, texB, texA := sampleVoodooTexel(s.texData, s.texWidth, s.texHeight, s.texClampS, s.texClampT, sTex, tTex)
				r, g, bVal, a = combineVoodooColors(s.fbzColorPath, s.colorPathSet, r, g, bVal, a, texR, texG, texB, texA)
			}

			// Clamp colors
			r = clampf(r, 0, 1)
			g = clampf(g, 0, 1)
			bVal = clampf(bVal, 0, 1)
			a = clampf(a, 0, 1)

			// Alpha test (discard if fails)
			if s.alphaTestEnable && !b.alphaTest(a, s.alphaTestRef, s.alphaTestFunc) {
				continue
			}

			// Chroma key test (discard if matches key color)
			if s.chromaKeyEnable && voodooChromaTest(s.chromaKey, s.chromaRange, r, g, bVal) {
				continue
			}

			// Fog blending
			if s.fogEnable {
				fogFactor := clampf(z, 0, 1)
				r = clampf(r*(1-fogFactor)+s.fogR*fogFactor, 0, 1)
				g = clampf(g*(1-fogFactor)+s.fogG*fogFactor, 0, 1)
				bVal = clampf(bVal*(1-fogFactor)+s.fogB*fogFactor, 0, 1)
			}

			// Dithering
			if s.ditherEnable {
				threshold := b.getDitherThreshold(x, y, s.dither2x2)
				r = b.applyDither(r, threshold)
				g = b.applyDither(g, threshold)
				bVal = b.applyDither(bVal, threshold)
			}

			// Write pixel
			if s.rgbWrite {
				bufIdx := pixelIndex * 4
				if s.forceOpaqueAlpha {
					a = 1.0
				}
				if s.alphaBlendEnable {
					for _, target := range s.targets {
						packed := blendVoodooPixel(b, s.srcBlendFactor, s.dstBlendFactor, target, bufIdx, r, g, bVal, a)
						*(*uint32)(unsafe.Pointer(&target[bufIdx])) = packed
					}
				} else {
					packed := uint32(r*255) | uint32(g*255)<<8 | uint32(bVal*255)<<16 | uint32(a*255)<<24
					for _, target := range s.targets {
						*(*uint32)(unsafe.Pointer(&target[bufIdx])) = packed
					}
				}
			}

			// Write depth
			if s.depthEnable && s.depthWrite {
				b.depthBuffer[pixelIndex] = z
			}
		}
	}
}

// interpolateTextureCoords is the reference S/T interpolation used by
// conformance tests; rasterizeRows inlines the same expressions with
// per-triangle hoisted flags.
func (b *VoodooSoftwareBackend) interpolateTextureCoords(w0, w1, w2 float32, v0, v1, v2 *VoodooVertex) (float32, float32) {
	if b.textureMode&VOODOO_TEX_PERSPECTIVE == 0 {
		return w0*v0.S + w1*v1.S + w2*v2.S,
			w0*v0.T + w1*v1.T + w2*v2.T
	}

	denom := w0*v0.W + w1*v1.W + w2*v2.W
	if denom == 0 {
		return 0, 0
	}
	s := (w0*v0.S*v0.W + w1*v1.S*v1.W + w2*v2.S*v2.W) / denom
	t := (w0*v0.T*v0.W + w1*v1.T*v1.W + w2*v2.T*v2.W) / denom
	return s, t
}

// drawTargetsFor resolves the colour buffers a raster writes, from the
// flush snapshot's fbzMode. The caller holds fbMu.
func (b *VoodooSoftwareBackend) drawTargetsFor(fbzMode uint32) [][]byte {
	drawFront := fbzMode&VOODOO_FBZ_DRAW_FRONT != 0
	drawBack := fbzMode&VOODOO_FBZ_DRAW_BACK != 0
	switch {
	case drawFront && drawBack:
		return [][]byte{b.colorBuffer, b.frontBuffer}
	case drawFront:
		return [][]byte{b.frontBuffer}
	default:
		return [][]byte{b.colorBuffer}
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

// chromaKeyTest checks the live chroma state; the raster loop uses the
// parameterised voodooChromaTest with per-flush snapshot state.
func (b *VoodooSoftwareBackend) chromaKeyTest(r, g, bVal float32) bool {
	return voodooChromaTest(b.chromaKey, b.chromaRange, r, g, bVal)
}

// voodooChromaTest checks if a color matches the chroma key (returns true if should discard)
func voodooChromaTest(chromaKey, chromaRange uint32, r, g, bVal float32) bool {
	if chromaRange != 0 {
		r8 := int(clampf(r, 0, 1)*255 + 0.5)
		g8 := int(clampf(g, 0, 1)*255 + 0.5)
		b8 := int(clampf(bVal, 0, 1)*255 + 0.5)

		minR := int((chromaKey >> 16) & 0xFF)
		minG := int((chromaKey >> 8) & 0xFF)
		minB := int(chromaKey & 0xFF)
		maxR := int((chromaRange >> 16) & 0xFF)
		maxG := int((chromaRange >> 8) & 0xFF)
		maxB := int(chromaRange & 0xFF)
		return r8 >= minR && r8 <= maxR &&
			g8 >= minG && g8 <= maxG &&
			b8 >= minB && b8 <= maxB
	}

	const inv255 = float32(1.0 / 255.0)
	keyR := float32((chromaKey>>16)&0xFF) * inv255
	keyG := float32((chromaKey>>8)&0xFF) * inv255
	keyB := float32(chromaKey&0xFF) * inv255

	const tolerance = inv255

	rMatch := abs32(r-keyR) <= tolerance
	gMatch := abs32(g-keyG) <= tolerance
	bMatch := abs32(bVal-keyB) <= tolerance

	return rMatch && gMatch && bMatch
}

func stippleAllowsVoodooPixel(stipple uint32, x, y int) bool {
	if stipple == 0 {
		return true
	}
	bit := uint((y&3)*8 + (x & 7))
	return (stipple & (1 << bit)) != 0
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
// blendVoodooPixel applies the alpha blend for one pixel of one target and
// returns the packed result. Both the scalar rasteriser and the SIMD kernel's
// blend stage call it, which is what makes them bit-identical: the expression
// `src*srcFactor + dst*dstFactor` is fused into an FMA by gc, and the fusion
// gc chooses depends on the function the expression sits in, so two copies of
// the same source text drift by an ulp. One body compiled once cannot. It is
// marked noinline for the same reason, since inlining would recreate the two
// separate contexts at the call sites.
func blendVoodooPixel(b *VoodooSoftwareBackend, srcBlendFactor, dstBlendFactor int, target []byte, bufIdx int, srcR, srcG, srcB, srcA float32) uint32 {
	const inv255 = float32(1.0 / 255.0)
	dstR := float32(target[bufIdx+0]) * inv255
	dstG := float32(target[bufIdx+1]) * inv255
	dstB := float32(target[bufIdx+2]) * inv255
	dstA := float32(target[bufIdx+3]) * inv255

	srcFactor := b.getBlendFactor(srcBlendFactor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)
	dstFactor := b.getBlendFactor(dstBlendFactor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)

	outR := clampf(srcR*srcFactor+dstR*dstFactor, 0, 1)
	outG := clampf(srcG*srcFactor+dstG*dstFactor, 0, 1)
	outB := clampf(srcB*srcFactor+dstB*dstFactor, 0, 1)
	outA := clampf(srcA*srcFactor+dstA*dstFactor, 0, 1)

	return uint32(outR*255) | uint32(outG*255)<<8 | uint32(outB*255)<<16 | uint32(outA*255)<<24
}

// getBlendFactor is kept as a method for the existing callers and tests; the
// selection itself never touches backend state, so it lives in a free function
// the blend body can call without a receiver.
func (b *VoodooSoftwareBackend) getBlendFactor(factor int, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA float32) float32 {
	return voodooBlendFactor(factor, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)
}

func voodooBlendFactor(factor int, srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA float32) float32 {
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
