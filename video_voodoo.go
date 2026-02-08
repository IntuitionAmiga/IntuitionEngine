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
	"sync/atomic"
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
	mu  sync.Mutex
	bus *MachineBus

	// Rendering backend (Vulkan or software fallback)
	backend VoodooBackend

	// Display configuration — lock-free for compositor reads
	width   atomic.Int32
	height  atomic.Int32
	layer   int
	enabled atomic.Bool

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
	vretrace    atomic.Bool
	swapPending bool

	// Triple-buffered frame output for lock-free GetFrame()
	// Protocol: producer owns writeIdx, consumer owns readIdx (via readingIdx),
	// sharedIdx holds the buffer in transit. Both sides use Swap to exchange.
	frameBufs  [3][]byte    // Pre-allocated framebuffers
	sharedIdx  atomic.Int32 // Buffer in shared slot (exchanged via Swap)
	readingIdx atomic.Int32 // Consumer's currently-owned buffer index
	writeIdx   int          // Producer's write buffer (not shared)

	// Texture memory for uploads
	textureMemory []byte
	textureWidth  int
	textureHeight int
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

	// Phase 6: Fog and dithering
	SetFogState(fogMode, fogColor uint32)

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
func NewVoodooEngine(bus *MachineBus) (*VoodooEngine, error) {
	v := &VoodooEngine{
		bus:           bus,
		layer:         VOODOO_LAYER,
		triangleBatch: make([]VoodooTriangle, 0, VOODOO_MAX_BATCH_TRIANGLES),
		textureMemory: make([]byte, VOODOO_TEXMEM_SIZE),
		clipRight:     VOODOO_DEFAULT_WIDTH,
		clipBottom:    VOODOO_DEFAULT_HEIGHT,
	}
	v.width.Store(int32(VOODOO_DEFAULT_WIDTH))
	v.height.Store(int32(VOODOO_DEFAULT_HEIGHT))
	// enabled defaults to false (atomic.Bool zero value) — programs enable via VOODOO_ENABLE write

	// Initialize triple-buffer: producer owns buf 0, shared holds buf 1,
	// consumer owns buf 2. All buffers start zeroed (black frame).
	bufSize := VOODOO_DEFAULT_WIDTH * VOODOO_DEFAULT_HEIGHT * 4
	for i := range v.frameBufs {
		v.frameBufs[i] = make([]byte, bufSize)
	}
	v.writeIdx = 0        // Producer starts writing to buffer 0
	v.sharedIdx.Store(1)  // Buffer 1 in shared slot
	v.readingIdx.Store(2) // Consumer starts with buffer 2

	// Initialize with Vulkan backend (falls back to software internally if Vulkan unavailable)
	vulkanBackend, err := NewVulkanBackend()
	if err != nil {
		return nil, err
	}
	if err := vulkanBackend.Init(int(v.width.Load()), int(v.height.Load())); err != nil {
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
	w := int(v.width.Load())
	h := int(v.height.Load())
	v.clipLeft = 0
	v.clipTop = 0
	v.clipRight = w
	v.clipBottom = h
	v.regs[(VOODOO_CLIP_LEFT_RIGHT-VOODOO_BASE)/4] = uint32(v.clipRight) | (uint32(v.clipLeft) << 16)
	v.regs[(VOODOO_CLIP_LOW_Y_HIGH-VOODOO_BASE)/4] = uint32(v.clipBottom) | (uint32(v.clipTop) << 16)

	// Default video dimensions
	v.regs[(VOODOO_VIDEO_DIM-VOODOO_BASE)/4] = uint32(w)<<16 | uint32(h)

	// Default colors
	v.color0 = 0x00000000 // Black
	v.color1 = 0xFFFFFFFF // White
}

// HandleRead handles register reads from the CPU
func (v *VoodooEngine) HandleRead(addr uint32) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()

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
	v.mu.Lock()
	defer v.mu.Unlock()

	// Handle texture memory writes (separate address range)
	if addr >= VOODOO_TEXMEM_BASE && addr < VOODOO_TEXMEM_BASE+VOODOO_TEXMEM_SIZE {
		offset := addr - VOODOO_TEXMEM_BASE
		// Bounds check for 4-byte write
		if offset+4 <= VOODOO_TEXMEM_SIZE {
			// Store as little-endian
			v.textureMemory[offset] = byte(value)
			v.textureMemory[offset+1] = byte(value >> 8)
			v.textureMemory[offset+2] = byte(value >> 16)
			v.textureMemory[offset+3] = byte(value >> 24)
		}
		return
	}

	// Store in shadow register
	regIndex := (addr - VOODOO_BASE) / 4
	if regIndex < 256 {
		v.regs[regIndex] = value
	}

	// Process the write
	switch addr {
	// Enable/disable the Voodoo engine
	case VOODOO_ENABLE:
		v.enabled.Store(value != 0)

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
	case VOODOO_TEX_WIDTH:
		v.textureWidth = int(value)
	case VOODOO_TEX_HEIGHT:
		v.textureHeight = int(value)
	case VOODOO_TEX_UPLOAD:
		// Trigger texture upload to backend
		if v.textureWidth > 0 && v.textureHeight > 0 && v.backend != nil {
			size := v.textureWidth * v.textureHeight * 4
			if size <= len(v.textureMemory) {
				format := int((v.textureMode >> 8) & 0xF)
				v.backend.SetTextureData(v.textureWidth, v.textureHeight,
					v.textureMemory[:size], format)
			}
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
			v.width.Store(int32(newWidth))
			v.height.Store(int32(newHeight))
			// Reallocate triple-buffer for new dimensions
			bufSize := newWidth * newHeight * 4
			for i := range v.frameBufs {
				v.frameBufs[i] = make([]byte, bufSize)
			}
			v.writeIdx = 0        // Producer takes buffer 0
			v.sharedIdx.Store(1)  // Buffer 1 in shared slot
			v.readingIdx.Store(2) // Consumer takes buffer 2
			if v.backend != nil {
				v.backend.Init(newWidth, newHeight)
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

		// Copy rendered frame to write buffer for triple-buffer publish
		frame := v.backend.GetFrame()
		if frame != nil && len(frame) == len(v.frameBufs[v.writeIdx]) {
			copy(v.frameBufs[v.writeIdx], frame)
		}

		// Publish completed frame via triple-buffer:
		// Swap our write buffer into the shared slot, get back the old shared buffer.
		// The old shared buffer is now ours to write to next frame.
		v.writeIdx = int(v.sharedIdx.Swap(int32(v.writeIdx)))
	}

	v.swapPending = false
}

// getStatus builds the status register value
func (v *VoodooEngine) getStatus() uint32 {
	var status uint32

	if v.busy {
		status |= VOODOO_STATUS_FBI_BUSY | VOODOO_STATUS_SST_BUSY
	}
	if v.vretrace.Load() {
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
// Pre-computed inverse constants for multiplication (faster than division)
const (
	inv12_4  = float32(1.0) / float32(1<<VOODOO_FIXED_12_4_SHIFT)  // 1/16 = 0.0625
	inv12_12 = float32(1.0) / float32(1<<VOODOO_FIXED_12_12_SHIFT) // 1/4096
	inv20_12 = float32(1.0) / float32(1<<VOODOO_FIXED_20_12_SHIFT) // 1/4096
	inv14_18 = float32(1.0) / float32(1<<VOODOO_FIXED_14_18_SHIFT) // 1/262144
	inv2_30  = float32(1.0) / float32(1<<VOODOO_FIXED_2_30_SHIFT)  // 1/1073741824
)

// fixed12_4ToFloat converts 12.4 fixed-point to float32 (vertex coords)
func fixed12_4ToFloat(value uint32) float32 {
	// Sign-extend from 16 bits
	signed := int32(int16(value & 0xFFFF))
	return float32(signed) * inv12_4
}

// fixed12_12ToFloat converts 12.12 fixed-point to float32 (colors)
// Result is in 0.0-1.0 range for colors (assuming max input is 255.0)
func fixed12_12ToFloat(value uint32) float32 {
	signed := int32(value)
	f := float32(signed) * inv12_12
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
	return float32(signed) * inv20_12
}

// fixed14_18ToFloat converts 14.18 fixed-point to float32 (texture coords)
func fixed14_18ToFloat(value uint32) float32 {
	signed := int32(value)
	return float32(signed) * inv14_18
}

// fixed2_30ToFloat converts 2.30 fixed-point to float32 (W coordinate)
func fixed2_30ToFloat(value uint32) float32 {
	signed := int32(value)
	return float32(signed) * inv2_30
}

// VideoSource interface implementation

// GetFrame returns the current rendered frame for the compositor (lock-free triple-buffer read)
func (v *VoodooEngine) GetFrame() []byte {
	if !v.enabled.Load() {
		return nil
	}
	// Swap our read buffer into the shared slot, get back the latest frame.
	// This is lock-free: single atomic Swap ensures no tearing.
	oldRead := v.readingIdx.Load()
	newRead := v.sharedIdx.Swap(oldRead)
	v.readingIdx.Store(newRead)
	return v.frameBufs[newRead]
}

// IsEnabled returns whether the Voodoo is active (lock-free)
func (v *VoodooEngine) IsEnabled() bool {
	return v.enabled.Load()
}

// GetLayer returns the compositor layer for the Voodoo
func (v *VoodooEngine) GetLayer() int {
	return v.layer
}

// GetDimensions returns the current framebuffer dimensions (lock-free)
func (v *VoodooEngine) GetDimensions() (int, int) {
	return int(v.width.Load()), int(v.height.Load())
}

// SignalVSync signals vertical retrace to the Voodoo (lock-free)
func (v *VoodooEngine) SignalVSync() {
	v.vretrace.Store(true)
}

// SetEnabled enables or disables the Voodoo (lock-free)
func (v *VoodooEngine) SetEnabled(enabled bool) {
	v.enabled.Store(enabled)
}

// SetBackend sets the rendering backend (Vulkan or software)
func (v *VoodooEngine) SetBackend(backend VoodooBackend) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Destroy old backend
	if v.backend != nil {
		v.backend.Destroy()
	}

	v.backend = backend
	if v.backend != nil {
		return v.backend.Init(int(v.width.Load()), int(v.height.Load()))
	}
	return nil
}

// GetTriangleBatchCount returns the number of triangles in the current batch
func (v *VoodooEngine) GetTriangleBatchCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.triangleBatch)
}

// SetTextureData uploads texture data to the backend
// Phase 4: Texture mapping support
func (v *VoodooEngine) SetTextureData(width, height int, data []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.backend != nil {
		// Get format from textureMode register
		format := int((v.textureMode >> 8) & 0xF)
		v.backend.SetTextureData(width, height, data, format)
	}
}

// Destroy cleans up resources
func (v *VoodooEngine) Destroy() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.backend != nil {
		v.backend.Destroy()
		v.backend = nil
	}
}
