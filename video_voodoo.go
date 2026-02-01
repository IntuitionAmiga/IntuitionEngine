// video_voodoo.go - 3DFX Voodoo SST-1 Graphics Emulation (Vulkan-Accelerated)

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
video_voodoo.go - 3DFX Voodoo Graphics SST-1 Emulation

This module implements 3DFX Voodoo SST-1 graphics chip emulation using a
High-Level Emulation (HLE) approach. Instead of software rasterization,
register writes are translated to Vulkan draw calls for GPU-accelerated
3D rendering.

Architecture:
- VoodooEngine: Register interface, command buffering, state machine
- VulkanBackend: Device/pipeline management, actual GPU rendering

Programming Model:
1. ASM code writes vertex coordinates to VERTEX_* registers (12.4 fixed-point)
2. ASM code writes colors to START_R/G/B/A registers (12.12 fixed-point)
3. Writing to TRIANGLE_CMD adds a triangle to the batch
4. Writing to SWAP_BUFFER_CMD flushes the batch to GPU and presents

Features:
- Full Voodoo register compatibility
- Gouraud shaded triangles
- Z-buffering with configurable depth function
- Alpha blending
- Scissor clipping
- Texture mapping (planned)

Reference: MAME voodoo.cpp for register-level compatibility
*/

package main

import (
	"sync"
)

// VoodooVertex represents a single vertex with all attributes
type VoodooVertex struct {
	X, Y, Z    float32 // Position (from 12.4 fixed-point)
	R, G, B, A float32 // Color (from 12.12 fixed-point, 0.0-1.0 range)
	S, T, W    float32 // Texture coords (from 14.18) and W (perspective)
}

// VoodooTriangle represents a triangle with three vertices
type VoodooTriangle struct {
	Vertices [3]VoodooVertex
}

// VoodooEngine implements 3DFX Voodoo SST-1 graphics emulation
type VoodooEngine struct {
	mutex sync.RWMutex
	bus   *SystemBus

	// Rendering backend (Vulkan or software fallback)
	backend VoodooBackend

	// Display configuration
	width, height int
	layer         int
	enabled       bool

	// Shadow registers (CPU-written values)
	regs [256]uint32

	// Current vertex being assembled (cycling 0, 1, 2)
	currentVertex VoodooVertex
	vertexIndex   int
	vertices      [3]VoodooVertex // Triangle vertices A, B, C

	// Per-vertex colors for Gouraud shading
	vertexColors       [3]VoodooVertex // Per-vertex attributes (R,G,B,A,Z,S,T,W)
	currentColorTarget int             // Which vertex (0,1,2) receives color writes
	gouraudEnabled     bool            // True if COLOR_SELECT was used this triangle

	// Triangle batch (flushed on SWAP_BUFFER_CMD)
	triangleBatch []VoodooTriangle

	// Pipeline state
	fbzMode       uint32
	alphaMode     uint32
	fbzColorPath  uint32
	textureMode   uint32
	fogMode       uint32
	pipelineDirty bool

	// Clipping rectangle
	clipLeft, clipRight int
	clipTop, clipBottom int

	// Colors
	color0    uint32 // Constant color 0 (for fast fill)
	color1    uint32 // Constant color 1
	fogColor  uint32 // Fog color
	zaColor   uint32 // Z/A constant
	chromaKey uint32 // Chroma key color

	// Status
	busy        bool
	vretrace    bool
	swapPending bool

	// Framebuffer for GetFrame() (compositor output)
	frameBuffer []byte
}

// VoodooBackend interface for rendering backends
type VoodooBackend interface {
	// Initialize the backend
	Init(width, height int) error

	// Pipeline state
	UpdatePipelineState(fbzMode, alphaMode uint32) error
	SetScissor(left, top, right, bottom int)
	SetChromaKey(chromaKey uint32) // Phase 3: Chroma key support

	// Phase 4: Texture mapping
	SetTextureData(width, height int, data []byte, format int)
	SetTextureEnabled(enabled bool)
	SetTextureWrapMode(clampS, clampT bool)

	// Phase 5: Color combine (fbzColorPath)
	SetColorPath(fbzColorPath uint32)

	// Rendering operations
	FlushTriangles(triangles []VoodooTriangle)
	ClearFramebuffer(color uint32)
	SwapBuffers(waitVSync bool)

	// Frame retrieval
	GetFrame() []byte

	// Cleanup
	Destroy()
}

// NewVoodooEngine creates a new Voodoo graphics engine
func NewVoodooEngine(bus *SystemBus) (*VoodooEngine, error) {
	v := &VoodooEngine{
		bus:           bus,
		width:         VOODOO_DEFAULT_WIDTH,
		height:        VOODOO_DEFAULT_HEIGHT,
		layer:         VOODOO_LAYER,
		enabled:       true,
		triangleBatch: make([]VoodooTriangle, 0, VOODOO_MAX_BATCH_TRIANGLES),
		frameBuffer:   make([]byte, VOODOO_DEFAULT_WIDTH*VOODOO_DEFAULT_HEIGHT*4),
		clipRight:     VOODOO_DEFAULT_WIDTH,
		clipBottom:    VOODOO_DEFAULT_HEIGHT,
	}

	// Initialize with Vulkan backend (falls back to software internally if Vulkan unavailable)
	vulkanBackend, err := NewVulkanBackend()
	if err != nil {
		return nil, err
	}
	if err := vulkanBackend.Init(v.width, v.height); err != nil {
		return nil, err
	}
	v.backend = vulkanBackend

	// Initialize default state
	v.initDefaultState()

	return v, nil
}

// initDefaultState sets up initial register values
func (v *VoodooEngine) initDefaultState() {
	// Default fbzMode: depth test enabled, depth write enabled, RGB write enabled
	v.fbzMode = VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_LESS << 5) // depth function = LESS
	v.regs[(VOODOO_FBZ_MODE-VOODOO_BASE)/4] = v.fbzMode

	// Default alphaMode: blending disabled, alpha test disabled
	v.alphaMode = 0
	v.regs[(VOODOO_ALPHA_MODE-VOODOO_BASE)/4] = v.alphaMode

	// Default clip rectangle: full screen
	v.clipLeft = 0
	v.clipTop = 0
	v.clipRight = v.width
	v.clipBottom = v.height
	v.regs[(VOODOO_CLIP_LEFT_RIGHT-VOODOO_BASE)/4] = uint32(v.clipRight) | (uint32(v.clipLeft) << 16)
	v.regs[(VOODOO_CLIP_LOW_Y_HIGH-VOODOO_BASE)/4] = uint32(v.clipBottom) | (uint32(v.clipTop) << 16)

	// Default video dimensions
	v.regs[(VOODOO_VIDEO_DIM-VOODOO_BASE)/4] = uint32(v.width)<<16 | uint32(v.height)

	// Default colors
	v.color0 = 0x00000000 // Black
	v.color1 = 0xFFFFFFFF // White
}

// HandleRead handles register reads from the CPU
func (v *VoodooEngine) HandleRead(addr uint32) uint32 {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	switch addr {
	case VOODOO_STATUS:
		return v.getStatus()
	default:
		// Return shadow register value
		regIndex := (addr - VOODOO_BASE) / 4
		if regIndex < 256 {
			return v.regs[regIndex]
		}
	}
	return 0
}

// HandleWrite handles register writes from the CPU
func (v *VoodooEngine) HandleWrite(addr uint32, value uint32) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	// Store in shadow register
	regIndex := (addr - VOODOO_BASE) / 4
	if regIndex < 256 {
		v.regs[regIndex] = value
	}

	// Process the write
	switch addr {
	// Vertex coordinates (12.4 fixed-point)
	case VOODOO_VERTEX_AX:
		v.vertices[0].X = fixed12_4ToFloat(value)
	case VOODOO_VERTEX_AY:
		v.vertices[0].Y = fixed12_4ToFloat(value)
	case VOODOO_VERTEX_BX:
		v.vertices[1].X = fixed12_4ToFloat(value)
	case VOODOO_VERTEX_BY:
		v.vertices[1].Y = fixed12_4ToFloat(value)
	case VOODOO_VERTEX_CX:
		v.vertices[2].X = fixed12_4ToFloat(value)
	case VOODOO_VERTEX_CY:
		v.vertices[2].Y = fixed12_4ToFloat(value)

	// Vertex color select for Gouraud shading
	case VOODOO_COLOR_SELECT:
		target := int(value & 0x03)
		if target < 3 {
			v.currentColorTarget = target
			v.gouraudEnabled = true
		}

	// Vertex colors (12.12 fixed-point)
	// When gouraudEnabled, writes go to vertexColors[currentColorTarget]
	// Otherwise, writes go to currentVertex (flat shading compatibility)
	case VOODOO_START_R:
		val := fixed12_12ToFloat(value)
		v.currentVertex.R = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].R = val
		}
	case VOODOO_START_G:
		val := fixed12_12ToFloat(value)
		v.currentVertex.G = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].G = val
		}
	case VOODOO_START_B:
		val := fixed12_12ToFloat(value)
		v.currentVertex.B = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].B = val
		}
	case VOODOO_START_A:
		val := fixed12_12ToFloat(value)
		v.currentVertex.A = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].A = val
		}
	case VOODOO_START_Z:
		val := fixed20_12ToFloat(value)
		v.currentVertex.Z = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].Z = val
		}

	// Texture coordinates (14.18 fixed-point)
	case VOODOO_START_S:
		val := fixed14_18ToFloat(value)
		v.currentVertex.S = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].S = val
		}
	case VOODOO_START_T:
		val := fixed14_18ToFloat(value)
		v.currentVertex.T = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].T = val
		}
	case VOODOO_START_W:
		val := fixed2_30ToFloat(value)
		v.currentVertex.W = val
		if v.gouraudEnabled {
			v.vertexColors[v.currentColorTarget].W = val
		}

	// Mode registers
	case VOODOO_FBZ_MODE:
		if v.fbzMode != value {
			v.fbzMode = value
			v.pipelineDirty = true
		}
	case VOODOO_ALPHA_MODE:
		if v.alphaMode != value {
			v.alphaMode = value
			v.pipelineDirty = true
		}
	case VOODOO_FBZCOLOR_PATH:
		v.fbzColorPath = value
	case VOODOO_TEXTURE_MODE:
		v.textureMode = value
		// Phase 4: Update backend texture state
		if v.backend != nil {
			enabled := (value & VOODOO_TEX_ENABLE) != 0
			clampS := (value & VOODOO_TEX_CLAMP_S) != 0
			clampT := (value & VOODOO_TEX_CLAMP_T) != 0
			v.backend.SetTextureEnabled(enabled)
			v.backend.SetTextureWrapMode(clampS, clampT)
		}
	case VOODOO_FOG_MODE:
		v.fogMode = value

	// Clipping
	case VOODOO_CLIP_LEFT_RIGHT:
		v.clipRight = int(value & VOODOO_CLIP_MASK)
		v.clipLeft = int((value >> 16) & VOODOO_CLIP_MASK)
		if v.backend != nil {
			v.backend.SetScissor(v.clipLeft, v.clipTop, v.clipRight, v.clipBottom)
		}
	case VOODOO_CLIP_LOW_Y_HIGH:
		v.clipBottom = int(value & VOODOO_CLIP_MASK)
		v.clipTop = int((value >> 16) & VOODOO_CLIP_MASK)
		if v.backend != nil {
			v.backend.SetScissor(v.clipLeft, v.clipTop, v.clipRight, v.clipBottom)
		}

	// Colors
	case VOODOO_COLOR0:
		v.color0 = value
	case VOODOO_COLOR1:
		v.color1 = value
	case VOODOO_FOG_COLOR:
		v.fogColor = value
	case VOODOO_ZA_COLOR:
		v.zaColor = value
	case VOODOO_CHROMA_KEY:
		v.chromaKey = value
		if v.backend != nil {
			v.backend.SetChromaKey(value)
		}

	// Video dimensions
	case VOODOO_VIDEO_DIM:
		newWidth := int((value >> 16) & 0xFFFF)
		newHeight := int(value & 0xFFFF)
		if newWidth > 0 && newHeight > 0 && newWidth <= VOODOO_MAX_WIDTH && newHeight <= VOODOO_MAX_HEIGHT {
			v.width = newWidth
			v.height = newHeight
			v.frameBuffer = make([]byte, v.width*v.height*4)
			if v.backend != nil {
				v.backend.Init(v.width, v.height)
			}
		}

	// Command registers
	case VOODOO_TRIANGLE_CMD:
		v.executeTriangleCmd()
	case VOODOO_FAST_FILL_CMD:
		v.executeFastFillCmd()
	case VOODOO_SWAP_BUFFER_CMD:
		v.executeSwapBufferCmd(value)
	case VOODOO_NOP_CMD:
		// No operation
	}
}

// executeTriangleCmd adds the current triangle to the batch
func (v *VoodooEngine) executeTriangleCmd() {
	// Apply vertex attributes based on shading mode
	if v.gouraudEnabled {
		// Gouraud shading: use per-vertex colors from vertexColors[]
		for i := 0; i < 3; i++ {
			v.vertices[i].R = v.vertexColors[i].R
			v.vertices[i].G = v.vertexColors[i].G
			v.vertices[i].B = v.vertexColors[i].B
			v.vertices[i].A = v.vertexColors[i].A
			v.vertices[i].Z = v.vertexColors[i].Z
			v.vertices[i].S = v.vertexColors[i].S
			v.vertices[i].T = v.vertexColors[i].T
			v.vertices[i].W = v.vertexColors[i].W
		}
	} else {
		// Flat shading: all vertices get the same color from currentVertex
		for i := 0; i < 3; i++ {
			v.vertices[i].R = v.currentVertex.R
			v.vertices[i].G = v.currentVertex.G
			v.vertices[i].B = v.currentVertex.B
			v.vertices[i].A = v.currentVertex.A
			v.vertices[i].Z = v.currentVertex.Z
			v.vertices[i].S = v.currentVertex.S
			v.vertices[i].T = v.currentVertex.T
			v.vertices[i].W = v.currentVertex.W
		}
	}

	// Add triangle to batch
	if len(v.triangleBatch) < VOODOO_MAX_BATCH_TRIANGLES {
		tri := VoodooTriangle{
			Vertices: [3]VoodooVertex{v.vertices[0], v.vertices[1], v.vertices[2]},
		}
		v.triangleBatch = append(v.triangleBatch, tri)
	}

	// Reset Gouraud state for next triangle (can be re-enabled with COLOR_SELECT)
	v.gouraudEnabled = false
}

// executeFastFillCmd clears the framebuffer with color0
func (v *VoodooEngine) executeFastFillCmd() {
	if v.backend != nil {
		// Update pipeline state if needed
		if v.pipelineDirty {
			v.backend.UpdatePipelineState(v.fbzMode, v.alphaMode)
			v.pipelineDirty = false
		}
		v.backend.ClearFramebuffer(v.color0)
	}
}

// executeSwapBufferCmd flushes triangles and swaps buffers
func (v *VoodooEngine) executeSwapBufferCmd(value uint32) {
	if v.backend != nil {
		// Update pipeline state if needed
		if v.pipelineDirty {
			v.backend.UpdatePipelineState(v.fbzMode, v.alphaMode)
			v.pipelineDirty = false
		}

		// Always flush triangles (even if empty, this triggers the clear)
		v.backend.FlushTriangles(v.triangleBatch)
		v.triangleBatch = v.triangleBatch[:0] // Clear batch

		// Swap buffers
		waitVSync := (value & VOODOO_SWAP_VSYNC) != 0
		v.backend.SwapBuffers(waitVSync)

		// Copy rendered frame for compositor
		frame := v.backend.GetFrame()
		if frame != nil && len(frame) == len(v.frameBuffer) {
			copy(v.frameBuffer, frame)
		}
	}

	v.swapPending = false
}

// getStatus builds the status register value
func (v *VoodooEngine) getStatus() uint32 {
	var status uint32

	if v.busy {
		status |= VOODOO_STATUS_FBI_BUSY | VOODOO_STATUS_SST_BUSY
	}
	if v.vretrace {
		status |= VOODOO_STATUS_VRETRACE
	}
	if v.swapPending {
		status |= VOODOO_STATUS_SWAPBUF
	}

	// Report some FIFO entries available
	status |= (0x3F << 12) // memfifo
	status |= (0x1F << 20) // pcififo

	return status
}

// Fixed-point conversion functions

// fixed12_4ToFloat converts 12.4 fixed-point to float32 (vertex coords)
func fixed12_4ToFloat(value uint32) float32 {
	// Sign-extend from 16 bits
	signed := int32(int16(value & 0xFFFF))
	return float32(signed) / float32(1<<VOODOO_FIXED_12_4_SHIFT)
}

// fixed12_12ToFloat converts 12.12 fixed-point to float32 (colors)
// Result is in 0.0-1.0 range for colors (assuming max input is 255.0)
func fixed12_12ToFloat(value uint32) float32 {
	signed := int32(value)
	f := float32(signed) / float32(1<<VOODOO_FIXED_12_12_SHIFT)
	// Clamp to 0.0-1.0 range for colors
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// fixed20_12ToFloat converts 20.12 fixed-point to float32 (Z coordinate)
func fixed20_12ToFloat(value uint32) float32 {
	signed := int32(value)
	return float32(signed) / float32(1<<VOODOO_FIXED_20_12_SHIFT)
}

// fixed14_18ToFloat converts 14.18 fixed-point to float32 (texture coords)
func fixed14_18ToFloat(value uint32) float32 {
	signed := int32(value)
	return float32(signed) / float32(1<<VOODOO_FIXED_14_18_SHIFT)
}

// fixed2_30ToFloat converts 2.30 fixed-point to float32 (W coordinate)
func fixed2_30ToFloat(value uint32) float32 {
	signed := int32(value)
	return float32(signed) / float32(1<<VOODOO_FIXED_2_30_SHIFT)
}

// VideoSource interface implementation

// GetFrame returns the current rendered frame for the compositor
func (v *VoodooEngine) GetFrame() []byte {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	if !v.enabled {
		return nil
	}
	return v.frameBuffer
}

// IsEnabled returns whether the Voodoo is active
func (v *VoodooEngine) IsEnabled() bool {
	v.mutex.RLock()
	defer v.mutex.RUnlock()
	return v.enabled
}

// GetLayer returns the compositor layer for the Voodoo
func (v *VoodooEngine) GetLayer() int {
	return v.layer
}

// GetDimensions returns the current framebuffer dimensions
func (v *VoodooEngine) GetDimensions() (int, int) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()
	return v.width, v.height
}

// SignalVSync signals vertical retrace to the Voodoo
func (v *VoodooEngine) SignalVSync() {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	v.vretrace = true
}

// SetEnabled enables or disables the Voodoo
func (v *VoodooEngine) SetEnabled(enabled bool) {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	v.enabled = enabled
}

// SetBackend sets the rendering backend (Vulkan or software)
func (v *VoodooEngine) SetBackend(backend VoodooBackend) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	// Destroy old backend
	if v.backend != nil {
		v.backend.Destroy()
	}

	v.backend = backend
	if v.backend != nil {
		return v.backend.Init(v.width, v.height)
	}
	return nil
}

// GetTriangleBatchCount returns the number of triangles in the current batch
func (v *VoodooEngine) GetTriangleBatchCount() int {
	v.mutex.RLock()
	defer v.mutex.RUnlock()
	return len(v.triangleBatch)
}

// SetTextureData uploads texture data to the backend
// Phase 4: Texture mapping support
func (v *VoodooEngine) SetTextureData(width, height int, data []byte) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if v.backend != nil {
		// Get format from textureMode register
		format := int((v.textureMode >> 8) & 0xF)
		v.backend.SetTextureData(width, height, data, format)
	}
}

// Destroy cleans up resources
func (v *VoodooEngine) Destroy() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if v.backend != nil {
		v.backend.Destroy()
		v.backend = nil
	}
}
