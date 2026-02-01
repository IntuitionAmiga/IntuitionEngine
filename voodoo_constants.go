// voodoo_constants.go - 3DFX Voodoo SST-1 Register Definitions

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
voodoo_constants.go - 3DFX Voodoo Graphics SST-1 Register Definitions

This file contains register addresses and bit field definitions for the Voodoo
SST-1 graphics chip emulation. Register offsets are sourced from MAME's
voodoo_regs.h for hardware accuracy.

The Voodoo uses a register-based programming model where 3D geometry is submitted
by writing vertex coordinates and attributes to sequential registers, then
triggering rasterization via the triangleCMD register.

Reference: MAME voodoo.cpp / voodoo_regs.h
*/

package main

// Voodoo memory map
const (
	VOODOO_BASE = 0xF4000 // Voodoo register base address
	VOODOO_END  = 0xF43FF // End of Voodoo register space
)

// Voodoo compositor layer (renders on top of VGA)
const VOODOO_LAYER = 20

// Voodoo display constants
const (
	VOODOO_DEFAULT_WIDTH  = 640
	VOODOO_DEFAULT_HEIGHT = 480
	VOODOO_MAX_WIDTH      = 800
	VOODOO_MAX_HEIGHT     = 600
)

// Status register (read-only)
const (
	VOODOO_STATUS = VOODOO_BASE + 0x000 // Status register
)

// Status register bits
const (
	VOODOO_STATUS_FBI_BUSY = 1 << 0     // FBI (framebuffer interface) busy
	VOODOO_STATUS_TMU_BUSY = 1 << 1     // TMU (texture mapping unit) busy
	VOODOO_STATUS_SST_BUSY = 1 << 2     // Overall chip busy
	VOODOO_STATUS_VRETRACE = 1 << 6     // Vertical retrace (VSync)
	VOODOO_STATUS_SWAPBUF  = 1 << 7     // Swap buffers pending
	VOODOO_STATUS_MEMFIFO  = 0xFF << 12 // Memory FIFO entries (8 bits)
	VOODOO_STATUS_PCIFIFO  = 0x1F << 20 // PCI FIFO entries (5 bits)
)

// Vertex coordinate registers (write-only, 12.4 fixed-point)
const (
	VOODOO_VERTEX_AX = VOODOO_BASE + 0x008 // Vertex A X coordinate
	VOODOO_VERTEX_AY = VOODOO_BASE + 0x00C // Vertex A Y coordinate
	VOODOO_VERTEX_BX = VOODOO_BASE + 0x010 // Vertex B X coordinate
	VOODOO_VERTEX_BY = VOODOO_BASE + 0x014 // Vertex B Y coordinate
	VOODOO_VERTEX_CX = VOODOO_BASE + 0x018 // Vertex C X coordinate
	VOODOO_VERTEX_CY = VOODOO_BASE + 0x01C // Vertex C Y coordinate
)

// Vertex attribute start values (write-only, various fixed-point formats)
const (
	VOODOO_START_R = VOODOO_BASE + 0x020 // Start red (12.12 fixed-point)
	VOODOO_START_G = VOODOO_BASE + 0x024 // Start green (12.12 fixed-point)
	VOODOO_START_B = VOODOO_BASE + 0x028 // Start blue (12.12 fixed-point)
	VOODOO_START_Z = VOODOO_BASE + 0x02C // Start Z (20.12 fixed-point)
	VOODOO_START_A = VOODOO_BASE + 0x030 // Start alpha (12.12 fixed-point)
	VOODOO_START_S = VOODOO_BASE + 0x034 // Start S texture coord (14.18 fixed-point)
	VOODOO_START_T = VOODOO_BASE + 0x038 // Start T texture coord (14.18 fixed-point)
	VOODOO_START_W = VOODOO_BASE + 0x03C // Start W (1/Z for perspective, 2.30 fixed-point)
)

// Delta values for Gouraud shading interpolation (write-only)
const (
	VOODOO_DRDX = VOODOO_BASE + 0x040 // dR/dX
	VOODOO_DGDX = VOODOO_BASE + 0x044 // dG/dX
	VOODOO_DBDX = VOODOO_BASE + 0x048 // dB/dX
	VOODOO_DZDX = VOODOO_BASE + 0x04C // dZ/dX
	VOODOO_DADX = VOODOO_BASE + 0x050 // dA/dX
	VOODOO_DSDX = VOODOO_BASE + 0x054 // dS/dX
	VOODOO_DTDX = VOODOO_BASE + 0x058 // dT/dX
	VOODOO_DWDX = VOODOO_BASE + 0x05C // dW/dX
	VOODOO_DRDY = VOODOO_BASE + 0x060 // dR/dY
	VOODOO_DGDY = VOODOO_BASE + 0x064 // dG/dY
	VOODOO_DBDY = VOODOO_BASE + 0x068 // dB/dY
	VOODOO_DZDY = VOODOO_BASE + 0x06C // dZ/dY
	VOODOO_DADY = VOODOO_BASE + 0x070 // dA/dY
	VOODOO_DSDY = VOODOO_BASE + 0x074 // dS/dY
	VOODOO_DTDY = VOODOO_BASE + 0x078 // dT/dY
	VOODOO_DWDY = VOODOO_BASE + 0x07C // dW/dY
)

// Command registers (write-only)
const (
	VOODOO_TRIANGLE_CMD    = VOODOO_BASE + 0x080 // Submit triangle for rasterization
	VOODOO_FTRIANGLECMD    = VOODOO_BASE + 0x084 // Fast triangle (strip mode)
	VOODOO_COLOR_SELECT    = VOODOO_BASE + 0x088 // Select vertex (0/1/2) for color writes (Gouraud)
	VOODOO_FBZCOLOR_PATH   = VOODOO_BASE + 0x104 // Framebuffer/color path config
	VOODOO_FOG_MODE        = VOODOO_BASE + 0x108 // Fog mode configuration
	VOODOO_ALPHA_MODE      = VOODOO_BASE + 0x10C // Alpha test/blend mode
	VOODOO_FBZ_MODE        = VOODOO_BASE + 0x110 // Framebuffer Z mode
	VOODOO_LFB_MODE        = VOODOO_BASE + 0x114 // Linear framebuffer mode
	VOODOO_CLIP_LEFT_RIGHT = VOODOO_BASE + 0x118 // Clip rectangle left/right
	VOODOO_CLIP_LOW_Y_HIGH = VOODOO_BASE + 0x11C // Clip rectangle top/bottom
	VOODOO_NOP_CMD         = VOODOO_BASE + 0x120 // No operation
	VOODOO_FAST_FILL_CMD   = VOODOO_BASE + 0x124 // Fast fill command
	VOODOO_SWAP_BUFFER_CMD = VOODOO_BASE + 0x128 // Swap front/back buffers
)

// Additional configuration registers
const (
	VOODOO_FOG_TABLE_BASE = VOODOO_BASE + 0x140 // Fog table (64 entries)
	VOODOO_FOG_COLOR      = VOODOO_BASE + 0x1C4 // Fog color (RGB)
	VOODOO_ZA_COLOR       = VOODOO_BASE + 0x1C8 // Z/A constant color
	VOODOO_CHROMA_KEY     = VOODOO_BASE + 0x1CC // Chroma key color
	VOODOO_CHROMA_RANGE   = VOODOO_BASE + 0x1D0 // Chroma key range
	VOODOO_STIPPLE        = VOODOO_BASE + 0x1D4 // Stipple pattern
	VOODOO_COLOR0         = VOODOO_BASE + 0x1D8 // Constant color 0
	VOODOO_COLOR1         = VOODOO_BASE + 0x1DC // Constant color 1
)

// Framebuffer configuration
const (
	VOODOO_FBI_INIT0   = VOODOO_BASE + 0x200 // FBI init register 0
	VOODOO_FBI_INIT1   = VOODOO_BASE + 0x204 // FBI init register 1
	VOODOO_FBI_INIT2   = VOODOO_BASE + 0x208 // FBI init register 2
	VOODOO_FBI_INIT3   = VOODOO_BASE + 0x20C // FBI init register 3
	VOODOO_FBI_INIT4   = VOODOO_BASE + 0x210 // FBI init register 4
	VOODOO_VIDEO_DIM   = VOODOO_BASE + 0x214 // Video dimensions (width<<16 | height)
	VOODOO_BACK_PORCH  = VOODOO_BASE + 0x218 // Back porch timing
	VOODOO_VIDEO_DIM_V = VOODOO_BASE + 0x21C // Vertical video dimensions
	VOODOO_H_SYNC      = VOODOO_BASE + 0x220 // Horizontal sync
	VOODOO_V_SYNC      = VOODOO_BASE + 0x224 // Vertical sync
	VOODOO_DAC_DATA    = VOODOO_BASE + 0x22C // DAC data register
)

// Texture mapping unit (TMU) registers
const (
	VOODOO_TEXTURE_MODE = VOODOO_BASE + 0x300 // Texture mode configuration
	VOODOO_TLOD         = VOODOO_BASE + 0x304 // Texture LOD control
	VOODOO_TDETAIL      = VOODOO_BASE + 0x308 // Texture detail control
	VOODOO_TEX_BASE0    = VOODOO_BASE + 0x30C // Texture base address (LOD 0)
	VOODOO_TEX_BASE1    = VOODOO_BASE + 0x310 // Texture base address (LOD 1)
	VOODOO_TEX_BASE2    = VOODOO_BASE + 0x314 // Texture base address (LOD 2)
	VOODOO_TEX_BASE3    = VOODOO_BASE + 0x318 // Texture base address (LOD 3)
	VOODOO_TEX_BASE4    = VOODOO_BASE + 0x31C // Texture base address (LOD 4)
	VOODOO_TEX_BASE5    = VOODOO_BASE + 0x320 // Texture base address (LOD 5)
	VOODOO_TEX_BASE6    = VOODOO_BASE + 0x324 // Texture base address (LOD 6)
	VOODOO_TEX_BASE7    = VOODOO_BASE + 0x328 // Texture base address (LOD 7)
	VOODOO_TEX_BASE8    = VOODOO_BASE + 0x32C // Texture base address (LOD 8)
	VOODOO_TEX_WIDTH    = VOODOO_BASE + 0x330 // Texture width for upload (IE extension)
	VOODOO_TEX_HEIGHT   = VOODOO_BASE + 0x334 // Texture height for upload (IE extension)
	VOODOO_TEX_UPLOAD   = VOODOO_BASE + 0x338 // Write to trigger texture upload (IE extension)
	VOODOO_PALETTE_BASE = VOODOO_BASE + 0x400 // Texture palette (256 entries)
)

// Texture memory base for texture uploads
const (
	VOODOO_TEXMEM_BASE = 0xF5000 // Texture memory base (separate from registers)
	VOODOO_TEXMEM_SIZE = 0x10000 // 64KB texture memory
)

// fbzMode bit fields
const (
	VOODOO_FBZ_CLIPPING     = 1 << 0      // Enable scissor clipping
	VOODOO_FBZ_CHROMAKEY    = 1 << 1      // Enable chroma keying
	VOODOO_FBZ_STIPPLE      = 1 << 2      // Enable stipple pattern
	VOODOO_FBZ_WBUFFER      = 1 << 3      // Use W buffer instead of Z
	VOODOO_FBZ_DEPTH_ENABLE = 1 << 4      // Enable depth buffer
	VOODOO_FBZ_DEPTH_FUNC   = 7 << 5      // Depth compare function (3 bits)
	VOODOO_FBZ_DITHER       = 1 << 8      // Enable dithering
	VOODOO_FBZ_RGB_WRITE    = 1 << 9      // Enable RGB buffer write
	VOODOO_FBZ_DEPTH_WRITE  = 1 << 10     // Enable depth buffer write
	VOODOO_FBZ_DITHER_2X2   = 1 << 11     // Use 2x2 dither (vs 4x4)
	VOODOO_FBZ_ALPHA_WRITE  = 1 << 12     // Enable alpha buffer write
	VOODOO_FBZ_DRAW_FRONT   = 1 << 14     // Draw to front buffer
	VOODOO_FBZ_DRAW_BACK    = 1 << 15     // Draw to back buffer
	VOODOO_FBZ_DEPTH_SOURCE = 1 << 16     // Depth source (0=iter, 1=float)
	VOODOO_FBZ_Y_ORIGIN     = 1 << 17     // Y origin (0=top, 1=bottom)
	VOODOO_FBZ_ALPHA_PLANES = 1 << 18     // Enable alpha planes
	VOODOO_FBZ_ALPHA_DITHER = 1 << 19     // Dither alpha
	VOODOO_FBZ_DEPTH_OFFSET = 0xFFF << 20 // Depth offset (12 bits)
)

// Depth compare functions (extracted from fbzMode bits 5-7)
const (
	VOODOO_DEPTH_NEVER        = 0
	VOODOO_DEPTH_LESS         = 1
	VOODOO_DEPTH_EQUAL        = 2
	VOODOO_DEPTH_LESSEQUAL    = 3
	VOODOO_DEPTH_GREATER      = 4
	VOODOO_DEPTH_NOTEQUAL     = 5
	VOODOO_DEPTH_GREATEREQUAL = 6
	VOODOO_DEPTH_ALWAYS       = 7
)

// alphaMode bit fields
const (
	VOODOO_ALPHA_TEST_EN   = 1 << 0     // Enable alpha test
	VOODOO_ALPHA_TEST_FUNC = 7 << 1     // Alpha test function (3 bits)
	VOODOO_ALPHA_BLEND_EN  = 1 << 4     // Enable alpha blending
	VOODOO_ALPHA_ANTIALIAS = 1 << 5     // Enable antialiasing
	VOODOO_ALPHA_SRC_RGB   = 0xF << 8   // Source RGB blend factor (4 bits)
	VOODOO_ALPHA_DST_RGB   = 0xF << 12  // Dest RGB blend factor (4 bits)
	VOODOO_ALPHA_SRC_A     = 0xF << 16  // Source alpha blend factor (4 bits)
	VOODOO_ALPHA_DST_A     = 0xF << 20  // Dest alpha blend factor (4 bits)
	VOODOO_ALPHA_REF       = 0xFF << 24 // Alpha reference value (8 bits)
)

// Alpha blend factors (for src/dst blend settings)
const (
	VOODOO_BLEND_ZERO      = 0  // 0
	VOODOO_BLEND_SRC_ALPHA = 1  // src.A
	VOODOO_BLEND_COLOR     = 2  // color (constant)
	VOODOO_BLEND_DST_ALPHA = 3  // dst.A
	VOODOO_BLEND_ONE       = 4  // 1
	VOODOO_BLEND_INV_SRC_A = 5  // 1 - src.A
	VOODOO_BLEND_INV_COLOR = 6  // 1 - color
	VOODOO_BLEND_INV_DST_A = 7  // 1 - dst.A
	VOODOO_BLEND_SATURATE  = 15 // min(src.A, 1-dst.A)
)

// Alpha test functions (same as depth functions)
const (
	VOODOO_ALPHA_NEVER        = 0
	VOODOO_ALPHA_LESS         = 1
	VOODOO_ALPHA_EQUAL        = 2
	VOODOO_ALPHA_LESSEQUAL    = 3
	VOODOO_ALPHA_GREATER      = 4
	VOODOO_ALPHA_NOTEQUAL     = 5
	VOODOO_ALPHA_GREATEREQUAL = 6
	VOODOO_ALPHA_ALWAYS       = 7
)

// textureMode bit fields
const (
	VOODOO_TEX_ENABLE      = 1 << 0   // Enable texturing
	VOODOO_TEX_MINIFY      = 7 << 1   // Minification filter (3 bits)
	VOODOO_TEX_MAGNIFY     = 1 << 4   // Magnification filter (0=point, 1=bilinear)
	VOODOO_TEX_CLAMP_S     = 1 << 5   // Clamp S coordinate
	VOODOO_TEX_CLAMP_T     = 1 << 6   // Clamp T coordinate
	VOODOO_TEX_FORMAT      = 0xF << 8 // Texture format (4 bits)
	VOODOO_TEX_CHROMA      = 1 << 12  // Texture chroma key enable
	VOODOO_TEX_TRILINEAR   = 1 << 13  // Trilinear filtering
	VOODOO_TEX_PERSPECTIVE = 1 << 14  // Perspective correction
	VOODOO_TEX_DETAIL      = 1 << 15  // Enable detail texturing
	VOODOO_TEX_SEQUENCE    = 1 << 16  // Sequence enable
)

// Texture formats
const (
	VOODOO_TEX_FMT_8BIT_PALETTE = 0  // 8-bit paletted
	VOODOO_TEX_FMT_YIQ          = 1  // YIQ compressed
	VOODOO_TEX_FMT_A8           = 2  // 8-bit alpha
	VOODOO_TEX_FMT_I8           = 3  // 8-bit intensity
	VOODOO_TEX_FMT_AI44         = 4  // 4-bit alpha + 4-bit intensity
	VOODOO_TEX_FMT_P8           = 5  // 8-bit palette (alternative)
	VOODOO_TEX_FMT_ARGB8332     = 6  // ARGB 8332
	VOODOO_TEX_FMT_AI88         = 7  // 8-bit alpha + 8-bit intensity
	VOODOO_TEX_FMT_ARGB1555     = 8  // ARGB 1555
	VOODOO_TEX_FMT_ARGB4444     = 9  // ARGB 4444
	VOODOO_TEX_FMT_ARGB8888     = 10 // ARGB 8888
)

// fbzColorPath bit fields (Phase 5: Color Combine)
// The fbzColorPath register controls how texture and iterated (vertex) colors are combined
const (
	VOODOO_FCP_RGB_SELECT_MASK  = 0x3      // Bits 0-1: RGB source select
	VOODOO_FCP_RGB_SELECT_SHIFT = 0        // Shift for RGB select
	VOODOO_FCP_A_SELECT_MASK    = 0x3 << 2 // Bits 2-3: Alpha source select
	VOODOO_FCP_A_SELECT_SHIFT   = 2        // Shift for alpha select
	VOODOO_FCP_CC_MSELECT_MASK  = 0x7 << 4 // Bits 4-6: Color combine mode select
	VOODOO_FCP_CC_MSELECT_SHIFT = 4        // Shift for CC mode
	VOODOO_FCP_TEXTURE_ENABLE   = 1 << 27  // Bit 27: Enable texture in color path
)

// Color source select values (for RGB_SELECT and A_SELECT)
const (
	VOODOO_CC_ITERATED = 0 // Use iterated (vertex) color
	VOODOO_CC_TEXTURE  = 1 // Use texture color
	VOODOO_CC_COLOR1   = 2 // Use constant color1
	VOODOO_CC_LFB      = 3 // Use linear framebuffer color
)

// Color combine function modes (for CC_MSELECT)
// These define how the two color sources are combined
const (
	VOODOO_CC_ZERO     = 0 // Output zero (black)
	VOODOO_CC_CSUB_CL  = 1 // cother - clocal (subtract)
	VOODOO_CC_ALOCAL   = 2 // clocal * alocal (modulate by local alpha)
	VOODOO_CC_AOTHER   = 3 // clocal * aother (modulate by other alpha)
	VOODOO_CC_CLOCAL   = 4 // clocal only (pass through)
	VOODOO_CC_ALOCAL_T = 5 // alocal * texture (alpha * texture)
	VOODOO_CC_CLOC_MUL = 6 // clocal * cother (multiply/modulate)
	VOODOO_CC_AOTHER_T = 7 // aother * texture
)

// Simplified color combine modes for common operations
// These are convenience values that combine select + mode bits
const (
	VOODOO_COMBINE_UNSET    = 0xFFFFFFFF                                                              // Not explicitly set (use defaults)
	VOODOO_COMBINE_ITERATED = 0                                                                       // Vertex color only (default when no texture)
	VOODOO_COMBINE_TEXTURE  = VOODOO_CC_TEXTURE                                                       // Texture color only
	VOODOO_COMBINE_MODULATE = VOODOO_CC_TEXTURE | (VOODOO_CC_CLOC_MUL << VOODOO_FCP_CC_MSELECT_SHIFT) // tex * vert
	VOODOO_COMBINE_ADD      = VOODOO_CC_TEXTURE | (0x08 << 4)                                         // tex + vert (extended)
	VOODOO_COMBINE_DECAL    = VOODOO_CC_TEXTURE | (VOODOO_CC_CLOCAL << VOODOO_FCP_CC_MSELECT_SHIFT)   // texture with vertex alpha
)

// Phase 6: fogMode bit fields
// The fogMode register controls depth-based fog blending
const (
	VOODOO_FOG_ENABLE      = 1 << 0 // Enable fog processing
	VOODOO_FOG_ADD         = 1 << 1 // Add fog color to output (vs. blend)
	VOODOO_FOG_MULT        = 1 << 2 // Multiply fog factor by alpha
	VOODOO_FOG_ZALPHA      = 1 << 3 // Use Z alpha for fog (vs. iterated)
	VOODOO_FOG_CONSTANT    = 1 << 4 // Use constant fog alpha
	VOODOO_FOG_DITHER      = 1 << 5 // Apply dithering to fog
	VOODOO_FOG_ZONES       = 1 << 6 // Enable fog zones (table-based fog)
	VOODOO_FOG_TABLE_SHIFT = 8      // Shift for fog table index
	VOODOO_FOG_TABLE_MASK  = 0x3F   // 6-bit fog table index mask
)

// Phase 6: Fog table constants
const (
	VOODOO_FOG_TABLE_SIZE   = 64 // Number of fog table entries
	VOODOO_FOG_TABLE_STRIDE = 4  // Bytes per fog table entry
	VOODOO_FOG_TABLE_END    = VOODOO_FOG_TABLE_BASE + (VOODOO_FOG_TABLE_SIZE * VOODOO_FOG_TABLE_STRIDE)
)

// Fixed-point format constants
const (
	VOODOO_FIXED_12_4_SHIFT  = 4  // Vertex coordinates (12.4)
	VOODOO_FIXED_12_12_SHIFT = 12 // Colors (12.12)
	VOODOO_FIXED_14_18_SHIFT = 18 // Texture coords (14.18)
	VOODOO_FIXED_20_12_SHIFT = 12 // Z coordinate (20.12)
	VOODOO_FIXED_2_30_SHIFT  = 30 // W coordinate (2.30)
)

// Clip register packing helpers
// clipLeftRight: bits 0-9 = right, bits 16-25 = left
// clipLowYHigh: bits 0-9 = bottom, bits 16-25 = top
const (
	VOODOO_CLIP_MASK = 0x3FF // 10-bit clip value mask
)

// swapbufferCMD bits
const (
	VOODOO_SWAP_VSYNC = 1 << 0 // Wait for VSync before swap
	VOODOO_SWAP_CLEAR = 1 << 1 // Clear buffer after swap
)

// fastfillCMD uses COLOR0 register for fill color

// Triangle batch limits
const (
	VOODOO_MAX_BATCH_TRIANGLES = 4096 // Maximum triangles per batch
	VOODOO_MAX_BATCH_VERTICES  = VOODOO_MAX_BATCH_TRIANGLES * 3
)

// Z80 Voodoo I/O ports (allows 8-bit CPUs to access 32-bit Voodoo registers)
// The Z80 cannot directly address Voodoo (0xF4000+) due to 16-bit address space.
// These ports provide an address/data interface with 32-bit accumulation:
//  1. Write register offset (from VOODOO_BASE) to ADDR_LO/HI ports
//  2. Write 4 data bytes to DATA0-DATA3 (little-endian)
//  3. Writing DATA3 triggers the 32-bit write to Voodoo
const (
	Z80_VOODOO_PORT_ADDR_LO   = 0xB0 // Voodoo register offset low byte
	Z80_VOODOO_PORT_ADDR_HI   = 0xB1 // Voodoo register offset high byte
	Z80_VOODOO_PORT_DATA0     = 0xB2 // Data byte 0 (bits 0-7)
	Z80_VOODOO_PORT_DATA1     = 0xB3 // Data byte 1 (bits 8-15)
	Z80_VOODOO_PORT_DATA2     = 0xB4 // Data byte 2 (bits 16-23)
	Z80_VOODOO_PORT_DATA3     = 0xB5 // Data byte 3 (bits 24-31) - triggers write
	Z80_VOODOO_PORT_TEXSRC_LO = 0xB6 // Texture source address low byte (Z80 RAM)
	Z80_VOODOO_PORT_TEXSRC_HI = 0xB7 // Texture source address high byte (Z80 RAM)
)
