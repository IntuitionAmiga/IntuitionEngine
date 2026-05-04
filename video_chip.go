// video_chip.go - Custom video chip for Intuition Engine

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
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
video_chip.go - Graphics Display Chip for the Intuition Engine

This module implements a complete video display system with:
- Multiple resolution modes (640x480, 800x600, 1024x768, 1280x960)
- Double-buffered RGBA framebuffer
- Dirty region tracking for efficient updates
- Splash screen support with bilinear scaling
- Memory-mapped register interface
- Hardware synchronisation support

Signal Flow:
1. Memory-mapped register writes.
2. Dirty region tracking.
3. Double buffer management.
4. Frame synchronisation.
5. Display output.

Thread Safety:
All parameter updates are protected by a mutex, allowing real-time control from external threads while video processing continues.
*/

package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"math"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// VideoChip layer constant for compositor
const VIDEOCHIP_LAYER = 0 // VideoChip renders as background

// ------------------------------------------------------------------------------
// Memory and Address Constants
// ------------------------------------------------------------------------------
const (
	BYTES_PER_KB = 1024                        // Size of a kilobyte in bytes
	BYTES_PER_MB = BYTES_PER_KB * BYTES_PER_KB // Size of a megabyte in bytes

	VIDEO_REG_BASE = 0xF0000 // Base address for memory-mapped registers
	// Register offsets for control, mode, and status
	VIDEO_REG_OFFSET_CTRL             = 0x000
	VIDEO_REG_OFFSET_MODE             = 0x004
	VIDEO_REG_OFFSET_STATUS           = 0x008
	VIDEO_REG_OFFSET_COPPER_CTRL      = 0x00C
	VIDEO_REG_OFFSET_COPPER_PTR       = 0x010
	VIDEO_REG_OFFSET_COPPER_PC        = 0x014
	VIDEO_REG_OFFSET_COPPER_STATUS    = 0x018
	VIDEO_REG_OFFSET_BLT_CTRL         = 0x01C
	VIDEO_REG_OFFSET_BLT_OP           = 0x020
	VIDEO_REG_OFFSET_BLT_SRC          = 0x024
	VIDEO_REG_OFFSET_BLT_DST          = 0x028
	VIDEO_REG_OFFSET_BLT_WIDTH        = 0x02C
	VIDEO_REG_OFFSET_BLT_HEIGHT       = 0x030
	VIDEO_REG_OFFSET_BLT_SRC_STRIDE   = 0x034
	VIDEO_REG_OFFSET_BLT_DST_STRIDE   = 0x038
	VIDEO_REG_OFFSET_BLT_COLOR        = 0x03C
	VIDEO_REG_OFFSET_BLT_MASK         = 0x040
	VIDEO_REG_OFFSET_BLT_STATUS       = 0x044
	VIDEO_REG_OFFSET_RASTER_Y         = 0x048
	VIDEO_REG_OFFSET_RASTER_HEIGHT    = 0x04C
	VIDEO_REG_OFFSET_RASTER_COLOR     = 0x050
	VIDEO_REG_OFFSET_RASTER_CTRL      = 0x054
	VIDEO_REG_OFFSET_BLT_MODE7_U0     = 0x058
	VIDEO_REG_OFFSET_BLT_MODE7_V0     = 0x05C
	VIDEO_REG_OFFSET_BLT_MODE7_DU_COL = 0x060
	VIDEO_REG_OFFSET_BLT_MODE7_DV_COL = 0x064
	VIDEO_REG_OFFSET_BLT_MODE7_DU_ROW = 0x068
	VIDEO_REG_OFFSET_BLT_MODE7_DV_ROW = 0x06C
	VIDEO_REG_OFFSET_BLT_MODE7_TEX_W  = 0x070
	VIDEO_REG_OFFSET_BLT_MODE7_TEX_H  = 0x074

	// CLUT8 palette mode registers
	VIDEO_REG_OFFSET_PAL_INDEX  = 0x078
	VIDEO_REG_OFFSET_PAL_DATA   = 0x07C
	VIDEO_REG_OFFSET_COLOR_MODE = 0x080
	VIDEO_REG_OFFSET_FB_BASE    = 0x084
	VIDEO_REG_OFFSET_PAL_TABLE  = 0x088
	VIDEO_REG_OFFSET_PAL_END    = 0x487

	// Extended blitter registers (BPP mode, draw modes, color expansion)
	VIDEO_REG_OFFSET_BLT_FLAGS     = 0x488
	VIDEO_REG_OFFSET_BLT_FG        = 0x48C
	VIDEO_REG_OFFSET_BLT_BG        = 0x490
	VIDEO_REG_OFFSET_BLT_MASK_MOD  = 0x494
	VIDEO_REG_OFFSET_BLT_MASK_SRCX = 0x498

	VIDEO_CTRL          = VIDEO_REG_BASE + VIDEO_REG_OFFSET_CTRL
	VIDEO_MODE          = VIDEO_REG_BASE + VIDEO_REG_OFFSET_MODE
	VIDEO_STATUS        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_STATUS
	COPPER_CTRL         = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_CTRL
	COPPER_PTR          = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_PTR
	COPPER_PC           = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_PC
	COPPER_STATUS       = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_STATUS
	BLT_CTRL            = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_CTRL
	BLT_OP              = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_OP
	BLT_SRC             = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_SRC
	BLT_DST             = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_DST
	BLT_WIDTH           = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_WIDTH
	BLT_HEIGHT          = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_HEIGHT
	BLT_SRC_STRIDE      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_SRC_STRIDE
	BLT_DST_STRIDE      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_DST_STRIDE
	BLT_COLOR           = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_COLOR
	BLT_MASK            = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MASK
	BLT_STATUS          = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_STATUS
	VIDEO_RASTER_Y      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_Y
	VIDEO_RASTER_HEIGHT = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_HEIGHT
	VIDEO_RASTER_COLOR  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_COLOR
	VIDEO_RASTER_CTRL   = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_CTRL
	BLT_MODE7_U0        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_U0
	BLT_MODE7_V0        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_V0
	BLT_MODE7_DU_COL    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_DU_COL
	BLT_MODE7_DV_COL    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_DV_COL
	BLT_MODE7_DU_ROW    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_DU_ROW
	BLT_MODE7_DV_ROW    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_DV_ROW
	BLT_MODE7_TEX_W     = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_TEX_W
	BLT_MODE7_TEX_H     = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MODE7_TEX_H

	// CLUT8 palette mode addresses
	VIDEO_PAL_INDEX  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_PAL_INDEX
	VIDEO_PAL_DATA   = VIDEO_REG_BASE + VIDEO_REG_OFFSET_PAL_DATA
	VIDEO_COLOR_MODE = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COLOR_MODE
	VIDEO_FB_BASE    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_FB_BASE
	VIDEO_PAL_TABLE  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_PAL_TABLE
	VIDEO_PAL_END    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_PAL_END

	// Extended blitter register addresses
	BLT_FLAGS     = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_FLAGS
	BLT_FG        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_FG
	BLT_BG        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_BG
	BLT_MASK_MOD  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MASK_MOD
	BLT_MASK_SRCX = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MASK_SRCX

	VIDEO_REG_END = BLT_MASK_SRCX + 3 // 0xF049B

	VRAM_START_MB = 1 // VRAM starts at 1MB offset
	VRAM_SIZE_MB  = 5 // 5MB of video memory
	VRAM_START    = VRAM_START_MB * BYTES_PER_MB
	VRAM_SIZE     = VRAM_SIZE_MB * BYTES_PER_MB
)

const (
	copperCtrlEnable = 1 << 0
	copperCtrlReset  = 1 << 1
)

const (
	copperStatusRunning = 1 << 0
	copperStatusWaiting = 1 << 1
	copperStatusHalted  = 1 << 2
)

const (
	copperOpcodeWait    = 0
	copperOpcodeMove    = 1
	copperOpcodeSetBase = 2
	copperOpcodeEnd     = 3
)

const (
	copperSetBaseMask = 0x00FFFFFF // 24-bit mask for base address (>> 2)
)

const (
	copperOpcodeShift = 30
	copperYShift      = 12
	copperCoordMask   = 0x0FFF
	copperRegShift    = 16   // Register index is in bits 16-23
	copperRegMask     = 0xFF // 8-bit register index
)

const (
	bltCtrlStart     = 1 << 0
	bltCtrlBusy      = 1 << 1
	bltCtrlIRQEnable = 1 << 2
	bltCtrlIRQ       = bltCtrlIRQEnable
)

const (
	bltOpCopy = iota
	bltOpFill
	bltOpLine
	bltOpMaskedCopy
	bltOpAlphaCopy // Copy only pixels with alpha > 0 (for transparency)
	bltOpMode7     // Affine texture mapping
	bltOpColorExpand
)

// BLT_FLAGS bit definitions
const (
	bltFlagsBPPMask       = 0x03 // Bits 0-1: BPP (0=RGBA32 4bpp, 1=CLUT8 1bpp)
	bltFlagsBPP_RGBA32    = 0x00
	bltFlagsBPP_CLUT8     = 0x01
	bltFlagsDrawModeMask  = 0xF0 // Bits 4-7: draw mode (16 raster ops)
	bltFlagsDrawModeShift = 4
	bltFlagsJAM1          = 1 << 8  // Bit 8: JAM1 mode (template: skip BG pixels)
	bltFlagsInvertTmpl    = 1 << 9  // Bit 9: invert template bits before processing
	bltFlagsInvertMode    = 1 << 10 // Bit 10: XOR dst with all-ones for set template bits
)

const (
	bltStatusErr        = 1 << 0
	bltStatusDone       = 1 << 1
	bltStatusIRQPending = 1 << 2
)

const (
	rasterCtrlStart = 1 << 0
)

// ------------------------------------------------------------------------------
// Video Mode Constants
// ------------------------------------------------------------------------------
const (
	MODE_640x480  = 0x00
	MODE_800x600  = 0x01
	MODE_1024x768 = 0x02
	MODE_1280x960 = 0x03

	RESOLUTION_640x480_WIDTH   = 640
	RESOLUTION_640x480_HEIGHT  = 480
	RESOLUTION_800x600_WIDTH   = 800
	RESOLUTION_800x600_HEIGHT  = 600
	RESOLUTION_1024x768_WIDTH  = 1024
	RESOLUTION_1024x768_HEIGHT = 768
	RESOLUTION_1280x960_WIDTH  = 1280
	RESOLUTION_1280x960_HEIGHT = 960

	// DEFAULT_VIDEO_MODE is the single source of truth for the default resolution.
	// Change this one constant to change the default everywhere (VideoChip, compositor,
	// Ebiten window, overlays). Valid values: MODE_640x480 .. MODE_1280x960.
	DEFAULT_VIDEO_MODE = MODE_800x600
)

// ------------------------------------------------------------------------------
// Pixel/Colour Constants
// ------------------------------------------------------------------------------
const (
	BYTES_PER_PIXEL  = 4 // RGBA format
	PIXEL_ALIGNMENT  = BYTES_PER_PIXEL
	PIXEL_ALIGN_MASK = PIXEL_ALIGNMENT - 1

	COLOR_MIN = 0
	COLOR_MAX = math.MaxUint8

	RGBA_R = 0
	RGBA_G = 1
	RGBA_B = 2
	RGBA_A = 3
)

// ------------------------------------------------------------------------------
// Dirty Region Tracking Constants
// ------------------------------------------------------------------------------
const (
	DIRTY_REGION_SIZE = 32 // Pixel dimensions per region block
	DIRTY_REGION_MIN  = 0
	REGION_ADJUSTMENT = 1

	REGION_COORDINATE_BITS = 16 // Bits allocated for X/Y in region keys
	REGION_Y_SHIFT         = REGION_COORDINATE_BITS
	REGION_MASK            = (1 << REGION_COORDINATE_BITS) - 1
	REGION_MAX_COORDINATE  = REGION_MASK
	INVALID_REGION         = -1
)

// ------------------------------------------------------------------------------
// Lock-Free Dirty Tracking Constants
// ------------------------------------------------------------------------------
const (
	// Atomic dirty grid: 16x16 tiles = 256 bits = 4 uint64s
	DIRTY_GRID_COLS     = 16 // Number of tile columns
	DIRTY_GRID_ROWS     = 16 // Number of tile rows
	DIRTY_GRID_SIZE     = 4  // Number of atomic.Uint64 needed (256 bits / 64)
	DIRTY_BITS_PER_WORD = 64 // Bits per atomic.Uint64
)

// ------------------------------------------------------------------------------
// Timing/Refresh Constants
// ------------------------------------------------------------------------------
const (
	REFRESH_RATE_HZ  = 60
	REFRESH_INTERVAL = time.Second / REFRESH_RATE_HZ
)

// ------------------------------------------------------------------------------
// Buffer/Channel Constants
// ------------------------------------------------------------------------------
const (
	BUFFER_OFFSET    = VRAM_START // Offset for VRAM access in HandleWrite
	BUFFER_REMAINDER = 0          // Required alignment remainder for pixel writes
)

// ------------------------------------------------------------------------------
// Control States/Flag Constants
// ------------------------------------------------------------------------------
const (
	CTRL_DISABLE_FLAG = 0 // Writing 0 to CTRL enables video
	ENABLED_STATE     = true
	DISABLED_STATE    = false
	VSYNC_ON          = true // Enable vertical synchronization in display config
)

// ------------------------------------------------------------------------------
// Initial State Constants
// ------------------------------------------------------------------------------
const (
	INITIAL_HAS_CONTENT = false
	INITIAL_MAP_SIZE    = 0
)

// ------------------------------------------------------------------------------
// Image Processing Constants
// ------------------------------------------------------------------------------
const (
	DRAW_SOURCE_OFFSET    = 0 // bounds.Min in draw.Draw
	DRAW_MODE_SOURCE      = draw.Src
	CENTER_OFFSET_DIVISOR = 2
	NEXT_PIXEL_OFFSET     = 1
)

// ------------------------------------------------------------------------------
// Error Message Constants
// ------------------------------------------------------------------------------
const (
	ERROR_FRAME_MSG  = "Error updating frame: %v\n"  // Shown when frame rendering fails
	ERROR_SPLASH_MSG = "Error updating splash: %v\n" // Shown when splash image fails to load
)

// ------------------------------------------------------------------------------
// Miscellaneous Constants
// ------------------------------------------------------------------------------
const (
	DEFAULT_DISPLAY_SCALE = 1
	DEFAULT_RETURN        = 0
	ADDR_OFFSET           = VRAM_START
	FRAME_INCREMENT       = 1
)

// ------------------------------------------------------------------------------
// Video Mode Configuration
// ------------------------------------------------------------------------------
var VideoModes = map[uint32]VideoMode{
	MODE_640x480: {
		width:       RESOLUTION_640x480_WIDTH,
		height:      RESOLUTION_640x480_HEIGHT,
		bytesPerRow: RESOLUTION_640x480_WIDTH * BYTES_PER_PIXEL,
		totalSize:   RESOLUTION_640x480_WIDTH * RESOLUTION_640x480_HEIGHT * BYTES_PER_PIXEL,
	},
	MODE_800x600: {
		width:       RESOLUTION_800x600_WIDTH,
		height:      RESOLUTION_800x600_HEIGHT,
		bytesPerRow: RESOLUTION_800x600_WIDTH * BYTES_PER_PIXEL,
		totalSize:   RESOLUTION_800x600_WIDTH * RESOLUTION_800x600_HEIGHT * BYTES_PER_PIXEL,
	},
	MODE_1024x768: {
		width:       RESOLUTION_1024x768_WIDTH,
		height:      RESOLUTION_1024x768_HEIGHT,
		bytesPerRow: RESOLUTION_1024x768_WIDTH * BYTES_PER_PIXEL,
		totalSize:   RESOLUTION_1024x768_WIDTH * RESOLUTION_1024x768_HEIGHT * BYTES_PER_PIXEL,
	},
	MODE_1280x960: {
		width:       RESOLUTION_1280x960_WIDTH,
		height:      RESOLUTION_1280x960_HEIGHT,
		bytesPerRow: RESOLUTION_1280x960_WIDTH * BYTES_PER_PIXEL,
		totalSize:   RESOLUTION_1280x960_WIDTH * RESOLUTION_1280x960_HEIGHT * BYTES_PER_PIXEL,
	},
}

// Derived from DEFAULT_VIDEO_MODE — do not edit these directly.
var (
	defaultMode         = VideoModes[DEFAULT_VIDEO_MODE]
	DefaultScreenWidth  = defaultMode.width
	DefaultScreenHeight = defaultMode.height
	DefaultOverlayCols  = DefaultScreenWidth / 8   // 8px glyph width
	DefaultOverlayRows  = DefaultScreenHeight / 16 // 16px glyph height
)

//go:embed splash.png
var splashData embed.FS

// VideoChip represents an Intuition Engine video chip with memory-mapped registers
type VideoChip struct {
	/*
	   Optimised Memory Layout Analysis (64-bit system):

	   Cache Line 0 (64 bytes):
	   - frameCounter   : offset 0,  size 8  - Hot path counter
	   - currentMode    : offset 8,  size 4  - Frequently accessed
	   - dirtyRowStride : offset 12, size 4  - Used in dirty region calc
	   - dirtyColStride : offset 16, size 4  - Used in dirty region calc
	   - Status flags   : offset 20, size 4  - Packed bools with padding

	   Cache Line 1 (64 bytes):
	   - mutex          : offset 24, size 8  - Aligned for atomic ops
	   - output         : offset 32, size 8  - Display interface

	   Cache Line 2 (64 bytes):
	   - vsyncChan      : offset 40, size 8  - Sync channels
	   - done           : offset 48, size 8  - Sync channels

	   Cache Line 3 (64 bytes):
	   - dirtyRegions   : offset 56, size 8  - Map pointer

	   Cache Lines 4+ (remaining):
	   - Buffer slices (to be converted to fixed arrays)

	   Benefits:
	   1. Hot path fields grouped in first cache line
	   2. Related fields kept together
	   3. Proper alignment for atomic operations
	   4. Packed boolean flags to reduce padding
	   5. Explicit padding for clarity
	   6. Mutex aligned to cache line for better lock performance
	   7. Changed int to int32 for tighter packing where full int64 not needed
	*/

	// Hot path: frequently accessed during refresh/render (Cache Line 0)
	frameCounter   uint64 // 8 bytes - Used every frame
	currentMode    uint32 // 4 bytes - Checked every pixel operation
	dirtyRowStride int32  // 4 bytes - Changed from int to int32 for alignment
	dirtyColStride int32  // 4 bytes - Changed from int to int32 for alignment

	// Status flags - atomic for lock-free access (part of Cache Line 0)
	enabled      atomic.Bool // Lock-free enable status
	hasContent   atomic.Bool // Lock-free content flag
	inVBlank     atomic.Bool // Lock-free VBlank status for CPU polling
	everSignaled atomic.Bool // true once compositor-driven VBlank is active
	stopped      atomic.Bool // Stop is final; stopped chips cannot be restarted
	resetting    bool        // 1 byte - still needs mutex for multi-field operations

	// Synchronization (Cache Line 1)
	mu       sync.Mutex // 8 bytes - Keep mutex at cache line boundary
	stopOnce sync.Once

	// Display interface (Cache Line 1-2)
	output             VideoOutput    // 8 bytes - Interface pointer
	onResolutionChange func(w, h int) // Optional resolution callback for compositor integration
	layer              int            // Z-order for compositor
	intSink            InterruptSink

	// Communication channels (Cache Line 2)
	vsyncChan chan struct{} // 8 bytes
	done      chan struct{} // 8 bytes

	// Lock-free dirty tracking (Cache Line 3)
	// Atomic bitmap: 256 tiles (16x16 grid), 4 uint64s
	dirtyBitmap [DIRTY_GRID_SIZE]atomic.Uint64 // 32 bytes - lock-free dirty bits
	tileWidth   int32                          // 4 bytes - tile width in pixels (mode-dependent)
	tileHeight  int32                          // 4 bytes - tile height in pixels (mode-dependent)

	// Fixed-size buffers (Cache Lines 4+)
	// Note: These will be converted to fixed arrays in next iteration
	frontBuffer  []byte // 24 bytes
	backBuffer   []byte // 24 bytes
	splashBuffer []byte // 24 bytes
	prevVRAM     []byte // 24 bytes

	// Copper state
	bus                       Bus32
	busMemory                 []byte       // Cached reference to bus memory for lock-free reads
	bigEndianMode             bool         // Read memory as big-endian (for M68K programs)
	directVRAM                []byte       // When set, GetFrame returns this instead of frontBuffer
	lastFrameStart            atomic.Int64 // Unix nanoseconds when current frame started
	copperEnabled             bool
	copperPtrStaged           uint32
	copperPtr                 uint32
	copperPC                  uint32
	copperWaiting             bool
	copperHalted              bool
	copperWaitX               uint16
	copperWaitY               uint16
	copperRasterX             uint16
	copperRasterY             uint16
	copperIOBase              uint32 // Base address for MOVE operations (default VIDEO_REG_BASE)
	copperManagedByCompositor bool   // true when compositor handles copper per-scanline

	// Blitter state
	bltIrqEnabled       bool
	bltBusy             bool
	bltOpStaged         uint32
	bltSrcStaged        uint32
	bltDstStaged        uint32
	bltWidthStaged      uint32
	bltHeightStaged     uint32
	bltSrcStride        uint32
	bltDstStride        uint32
	bltColorStaged      uint32
	bltMaskStaged       uint32
	bltMode7U0Staged    uint32
	bltMode7V0Staged    uint32
	bltMode7DuColStaged uint32
	bltMode7DvColStaged uint32
	bltMode7DuRowStaged uint32
	bltMode7DvRowStaged uint32
	bltMode7TexWStaged  uint32
	bltMode7TexHStaged  uint32
	bltFlagsStaged      uint32
	bltFGStaged         uint32
	bltBGStaged         uint32
	bltMaskModStaged    uint32
	bltMaskSrcXStaged   uint32

	bltOp           uint32
	bltSrc          uint32
	bltDst          uint32
	bltWidth        uint32
	bltHeight       uint32
	bltSrcStrideRun uint32
	bltDstStrideRun uint32
	bltColor        uint32
	bltMask         uint32
	bltMode7U0      uint32
	bltMode7V0      uint32
	bltMode7DuCol   uint32
	bltMode7DvCol   uint32
	bltMode7DuRow   uint32
	bltMode7DvRow   uint32
	bltMode7TexW    uint32
	bltMode7TexH    uint32
	bltFlags        uint32
	bltFG           uint32
	bltBG           uint32
	bltMaskMod      uint32
	bltMaskSrcX     uint32
	blitterEnabled  bool

	bltPending bool
	bltErr     bool
	bltDone    bool
	bltIrqPend bool

	rasterY      uint32
	rasterHeight uint32
	rasterColor  uint32
	rasterCtrl   uint32

	// CLUT8 palette mode state
	clutMode      atomic.Bool // true = CLUT8 indexed mode, false = RGBA32 direct
	clutPalette   [256]uint32 // Pre-packed LE RGBA values for fast lookup
	clutPaletteHW [256]uint32 // Raw 0x00RRGGBB values as written by guest
	palIndex      uint32      // Auto-incrementing palette write index
	clutFrame     []byte      // Conversion buffer (width*height*4 RGBA32)
	fbBase        uint32      // Framebuffer base address in bus memory
	clutWarnOnce  sync.Once   // Rate-limit out-of-range warnings
	clutWarnFrame uint64      // Last frame that emitted an out-of-range CLUT warning
	clutWarned    bool
}

// VideoMode defines resolution and buffer parameters for a display mode
type VideoMode struct {
	/*
	   Memory layout (64-bit system):
	   Field       Offset Size  Cache Line
	   ----------------------------------------
	   width       0      8    Line 0
	   bytesPerRow 8      8    Line 0
	   height      16     8    Line 0
	   totalSize   24     8    Line 0
	   Total size: 32 bytes (fits in half a cache line)

	   Benefits:
	   1. Related fields accessed together are adjacent in memory
	   2. No padding needed - maintains 8-byte alignment
	   3. Entire struct fits in half a cache line
	   4. Preserves all original fields and types exactly
	   5. Groups fields by usage patterns in the code:
	      - width/bytesPerRow: Used together for pixel addressing
	      - height/totalSize: Used together for buffer management
	*/

	// Group width and bytesPerRow together since they're frequently accessed
	// together for pixel address calculations
	width       int // Horizontal resolution in pixels
	bytesPerRow int // Bytes per row (width * BYTES_PER_PIXEL)

	// Group height and totalSize together since they're often used
	// together for buffer allocation and bounds checking
	height    int // Vertical resolution in pixels
	totalSize int // Total buffer size (width * height * BYTES_PER_PIXEL)
}

// DirtyRegion tracks a modified area of the screen for partial updates
type DirtyRegion struct {
	/*
	   Memory layout (64-bit system):
	   Field         Offset Size  Cache Line
	   ------------------------------------------
	   lastUpdated   0      8    Line 0 - Hot path
	   x            8      8    Line 0 - Coordinates
	   y            16     8    Line 0 - Coordinates
	   width        24     8    Line 0 - Dimensions
	   height       32     8    Line 0 - Dimensions
	   Total: 40 bytes (fits in single cache line)
	*/

	// Most frequently accessed field
	lastUpdated uint64 // Frame counter when last updated

	// Coordinate fields grouped together
	x int // Top-left X coordinate
	y int // Top-left Y coordinate

	// Dimension fields grouped together
	width  int // Region width in pixels
	height int // Region height in pixels
}

func NewVideoChip(backend int) (*VideoChip, error) {
	/*
		NewVideoChip creates and initialises a new VideoChip instance.

		It performs the following tasks:
		1. Initialises the video output using the supplied backend.
		2. Sets the default video mode and allocates the front/back buffers.
		3. Loads and decodes the splash image (if available), converting it to an RGBA buffer.
		4. Scales the splash image to the current video mode.
		5. Initialises the dirty region grid for efficient partial updates.
		6. Spawns a goroutine to run the refresh loop at the configured refresh rate.

		Parameters:
		 - backend: Identifier for the video backend.

		Returns:
		 - *VideoChip: Pointer to the new VideoChip instance.
		 - error: Non-nil if an error occurs during initialisation.

		Thread Safety:
		State modifications are protected by a mutex where appropriate.
	*/

	output, err := NewVideoOutput(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to create video output: %w", err)
	}

	chip := &VideoChip{
		output:         output,
		currentMode:    DEFAULT_VIDEO_MODE,
		layer:          VIDEOCHIP_LAYER,
		vsyncChan:      make(chan struct{}),
		done:           make(chan struct{}),
		frameCounter:   0,
		prevVRAM:       make([]byte, VRAM_SIZE),
		blitterEnabled: true,
	}
	// Atomic fields default to false - no explicit init needed

	mode := VideoModes[chip.currentMode]
	chip.frontBuffer = make([]byte, mode.totalSize)
	chip.backBuffer = make([]byte, mode.totalSize)

	// Load and decode splash image to RGBA
	splashPNG, err := GetSplashImageData()
	if err == nil {
		img, _, err := image.Decode(bytes.NewReader(splashPNG))
		if err == nil {
			// Convert image to RGBA format
			bounds := img.Bounds()
			rgbaImg := image.NewRGBA(bounds)
			draw.Draw(rgbaImg, bounds, img, image.Point{DRAW_SOURCE_OFFSET, DRAW_SOURCE_OFFSET}, DRAW_MODE_SOURCE)

			// Store the raw RGBA pixels
			chip.splashBuffer = make([]byte, len(rgbaImg.Pix))
			copy(chip.splashBuffer, rgbaImg.Pix)

			// Scale the splash image to the current video mode
			chip.splashBuffer = chip.scaleImageToMode(chip.splashBuffer,
				bounds.Dx(), bounds.Dy(), mode)
		}
	}

	chip.initialiseDirtyGrid(mode)
	go chip.refreshLoop()

	return chip, nil
}

func (chip *VideoChip) AttachBus(bus Bus32) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.bus = bus
	chip.busMemory = bus.GetMemory() // Cache for lock-free reads
}

// SetBusMemory sets a direct reference to bus memory, used when VRAM I/O mapping
// has been removed (e.g., EmuTOS mode where the VRAM address range is normal RAM).
func (chip *VideoChip) SetBusMemory(mem []byte) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.busMemory = mem
}

// SetBigEndianMode configures the video chip to read memory in big-endian format.
// This is required for M68K programs where data (copper lists, etc.) is stored
// in big-endian byte order.
func (chip *VideoChip) SetBigEndianMode(enabled bool) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.bigEndianMode = enabled
}

// SetDirectVRAM configures the video chip to return a slice of bus memory
// for GetFrame() instead of its internal frontBuffer. This is used when VRAM
// I/O is unmapped (e.g. EmuTOS mode) so the CPU writes directly to bus memory
// and the VideoChip reads from the same location.
func (chip *VideoChip) SetDirectVRAM(slice []byte) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.directVRAM = slice
}

// setPaletteEntry unpacks a 0x00RRGGBB hardware value into a pre-packed
// little-endian RGBA uint32 for fast CLUT8→RGBA32 conversion.
func (chip *VideoChip) setPaletteEntry(index uint8, hwVal uint32) {
	chip.clutPaletteHW[index] = hwVal
	r := uint8((hwVal >> 16) & 0xFF)
	g := uint8((hwVal >> 8) & 0xFF)
	b := uint8(hwVal & 0xFF)
	// Pack as LE RGBA: R at byte 0, G at byte 1, B at byte 2, A at byte 3
	chip.clutPalette[index] = uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xFF000000
}

func (chip *VideoChip) SetPaletteEntry(index uint8, hwVal uint32) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.setPaletteEntry(index, hwVal)
}

// convertCLUT8Frame reads indexed pixels from bus memory at fbBase and
// converts them to RGBA32 in clutFrame using the palette lookup table.
func (chip *VideoChip) convertCLUT8Frame() {
	mode := VideoModes[chip.currentMode]
	pixelCount := uint32(mode.width * mode.height)
	frameSize := int(pixelCount) * BYTES_PER_PIXEL

	// Ensure clutFrame is allocated
	if len(chip.clutFrame) != frameSize {
		chip.clutFrame = make([]byte, frameSize)
	}

	fb := chip.fbBase
	if chip.busMemory == nil || fb+pixelCount > uint32(len(chip.busMemory)) {
		chip.bltErr = true
		if !chip.clutWarned || chip.frameCounter-chip.clutWarnFrame >= 60 {
			chip.clutWarned = true
			chip.clutWarnFrame = chip.frameCounter
			fmt.Printf("CLUT8: fbBase 0x%X out of range (need %d bytes, bus has %d)\n",
				fb, fb+pixelCount, len(chip.busMemory))
		}
		clear(chip.clutFrame)
		return
	}

	src := chip.busMemory[fb : fb+pixelCount]
	dst := chip.clutFrame
	for i := uint32(0); i < pixelCount; i++ {
		rgba := chip.clutPalette[src[i]]
		off := i * BYTES_PER_PIXEL
		*(*uint32)(unsafe.Pointer(&dst[off])) = rgba
	}
}

// clutGetFrame returns the appropriate frame for CLUT8 or RGBA32 direct modes.
// Caller must hold chip.mu.
func (chip *VideoChip) clutGetFrame() []byte {
	if chip.clutMode.Load() {
		chip.convertCLUT8Frame()
		return chip.clutFrame
	}
	if chip.fbBase != 0 {
		mode := VideoModes[chip.currentMode]
		frameSize := uint32(mode.width * mode.height * BYTES_PER_PIXEL)
		fb := chip.fbBase
		if chip.busMemory != nil && fb+frameSize <= uint32(len(chip.busMemory)) {
			return chip.busMemory[fb : fb+frameSize]
		}
	}
	return chip.directVRAM
}

func (chip *VideoChip) SetBlitterEnabled(enabled bool) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.blitterEnabled = enabled
}

func (chip *VideoChip) SetResolutionChangeCallback(cb func(w, h int)) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.onResolutionChange = cb
}

// MarkHasContent signals that the framebuffer contains displayable content.
// This should be called by external renderers (like VGA) after writing to the buffer.
func (chip *VideoChip) MarkHasContent() {
	chip.hasContent.Store(true)
}

func (chip *VideoChip) scaleImageToMode(imgData []byte, srcWidth, srcHeight int, mode VideoMode) []byte {
	/*
	   scaleImageToMode resizes an image from its source dimensions to fit the target video mode using bilinear interpolation.

	   Parameters:
	    - imgData: Source image pixels in RGBA format.
	    - srcWidth, srcHeight: Dimensions of the source image.
	    - mode: Target video mode configuration (defines width, height and totalSize).

	   Returns:
	    - []byte: Rescaled image pixel data, conforming to mode.totalSize.
	*/

	scaled := make([]byte, mode.totalSize)
	scaleX := float64(srcWidth) / float64(mode.width)
	yOffset := (mode.height - srcHeight) / CENTER_OFFSET_DIVISOR // Center the image

	for y := 0; y < mode.height; y++ {
		srcY := float64(y - yOffset)
		if srcY < 0 || srcY >= float64(srcHeight-1) {
			continue
		}

		for x := 0; x < mode.width; x++ {
			srcX := float64(x) * scaleX
			if srcX >= float64(srcWidth-1) {
				continue
			}

			// Get surrounding pixels
			x0, y0 := int(srcX), int(srcY)
			x1 := min(x0+NEXT_PIXEL_OFFSET, srcWidth-1)
			y1 := min(y0+NEXT_PIXEL_OFFSET, srcHeight-1)
			fx, fy := srcX-float64(x0), srcY-float64(y0)

			// Sample four corners
			dstIdx := (y*mode.width + x) * BYTES_PER_PIXEL

			// Top-left
			idx00 := (y0*srcWidth + x0) * BYTES_PER_PIXEL
			r00 := imgData[idx00+RGBA_R]
			g00 := imgData[idx00+RGBA_G]
			b00 := imgData[idx00+RGBA_B]
			a00 := imgData[idx00+RGBA_A]

			// Top-right
			idx10 := (y0*srcWidth + x1) * BYTES_PER_PIXEL
			r10 := imgData[idx10+RGBA_R]
			g10 := imgData[idx10+RGBA_G]
			b10 := imgData[idx10+RGBA_B]
			a10 := imgData[idx10+RGBA_A]

			// Bottom-left
			idx01 := (y1*srcWidth + x0) * BYTES_PER_PIXEL
			r01 := imgData[idx01+RGBA_R]
			g01 := imgData[idx01+RGBA_G]
			b01 := imgData[idx01+RGBA_B]
			a01 := imgData[idx01+RGBA_A]

			// Bottom-right
			idx11 := (y1*srcWidth + x1) * BYTES_PER_PIXEL
			r11 := imgData[idx11+RGBA_R]
			g11 := imgData[idx11+RGBA_G]
			b11 := imgData[idx11+RGBA_B]
			a11 := imgData[idx11+RGBA_A]

			// Bilinear interpolation
			scaled[dstIdx] = byte(math.Max(float64(COLOR_MIN), math.Min(float64(COLOR_MAX),
				float64(r00)*(1-fx)*(1-fy)+float64(r10)*fx*(1-fy)+
					float64(r01)*(1-fx)*fy+float64(r11)*fx*fy)))
			scaled[dstIdx+1] = byte(math.Max(float64(COLOR_MIN), math.Min(float64(COLOR_MAX),
				float64(g00)*(1-fx)*(1-fy)+float64(g10)*fx*(1-fy)+
					float64(g01)*(1-fx)*fy+float64(g11)*fx*fy)))
			scaled[dstIdx+2] = byte(math.Max(float64(COLOR_MIN), math.Min(float64(COLOR_MAX),
				float64(b00)*(1-fx)*(1-fy)+float64(b10)*fx*(1-fy)+
					float64(b01)*(1-fx)*fy+float64(b11)*fx*fy)))
			scaled[dstIdx+3] = byte(math.Max(float64(COLOR_MIN), math.Min(float64(COLOR_MAX),
				float64(a00)*(1-fx)*(1-fy)+float64(a10)*fx*(1-fy)+
					float64(a01)*(1-fx)*fy+float64(a11)*fx*fy)))
		}
	}
	return scaled
}

func (chip *VideoChip) Start() error {
	/*
		Start enables the VideoChip and initiates the display output.

		This method sets the chip's enabled state to true and, if a video output interface is available,
		it calls the Start method on that interface. The operation is performed under a mutex lock to ensure
		thread safety during the state update.

		Returns:
		  - error: Any error encountered when starting the video output, or nil if the operation succeeds.
	*/
	chip.mu.Lock()
	defer chip.mu.Unlock()
	if chip.stopped.Load() {
		return nil
	}
	chip.enabled.Store(true)
	if chip.output != nil {
		return chip.output.Start()
	}
	return nil
}

func (chip *VideoChip) Stop() error {
	/*
		Stop disables the VideoChip and halts the display output.

		This method sets the chip's enabled state to false and, if a video output interface is available,
		it calls the Stop method on that interface. The operation is performed under a mutex lock to ensure
		thread safety during the state update.

		Returns:
		  - error: Any error encountered when stopping the video output, or nil if the operation succeeds.
	*/
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.enabled.Store(false)
	chip.stopOnce.Do(func() {
		chip.stopped.Store(true)
		close(chip.done)
	})
	if chip.output != nil {
		return chip.output.Stop()
	}
	return nil
}

func (chip *VideoChip) SetInterruptSink(sink InterruptSink) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.intSink = sink
}

func (chip *VideoChip) initialiseDirtyGrid(mode VideoMode) {
	/*
		initialiseDirtyGrid configures the dirty region grid based on the current video mode.

		This function initialises the lock-free atomic dirty bitmap with tile dimensions
		calculated from the current video mode to cover a 16x16 grid.

		Parameters:
		  - mode: The current VideoMode configuration, providing the resolution (width and height).

		Notes:
		  - Lock-free bitmap uses 16x16 tiles with mode-dependent pixel dimensions.
	*/

	// Lock-free atomic bitmap: calculate tile dimensions for 16x16 grid
	chip.tileWidth = int32((mode.width + DIRTY_GRID_COLS - 1) / DIRTY_GRID_COLS)
	chip.tileHeight = int32((mode.height + DIRTY_GRID_ROWS - 1) / DIRTY_GRID_ROWS)

	// Clear atomic dirty bitmap
	for i := range chip.dirtyBitmap {
		chip.dirtyBitmap[i].Store(0)
	}
}

// markTileDirtyAtomic marks a tile as dirty using lock-free atomic operations.
// This is the fast path for dirty tracking, using a CAS loop to set a bit
// in the atomic bitmap without requiring any mutex locks.
//
// Parameters:
//   - x: The x-coordinate (in pixels) that triggered the dirty tile.
//   - y: The y-coordinate (in pixels) that triggered the dirty tile.
func (chip *VideoChip) markTileDirtyAtomic(x, y int) {
	// Calculate tile indices (16x16 grid)
	tileX := x / int(chip.tileWidth)
	tileY := y / int(chip.tileHeight)

	// Clamp to valid range
	if tileX < 0 || tileX >= DIRTY_GRID_COLS || tileY < 0 || tileY >= DIRTY_GRID_ROWS {
		return
	}

	// Calculate bit position: tileY * 16 + tileX = 0-255
	bitIndex := tileY*DIRTY_GRID_COLS + tileX

	// Determine which uint64 and which bit within it
	wordIndex := bitIndex / DIRTY_BITS_PER_WORD
	bitOffset := uint(bitIndex % DIRTY_BITS_PER_WORD)

	// Lock-free CAS loop to set the bit
	for {
		old := chip.dirtyBitmap[wordIndex].Load()
		new := old | (1 << bitOffset)
		if old == new || chip.dirtyBitmap[wordIndex].CompareAndSwap(old, new) {
			break
		}
	}
}

// hasDirtyTiles returns true if any tiles are marked dirty in the atomic bitmap.
func (chip *VideoChip) hasDirtyTiles() bool {
	for i := range chip.dirtyBitmap {
		if chip.dirtyBitmap[i].Load() != 0 {
			return true
		}
	}
	return false
}

// clearDirtyBitmap atomically clears all dirty bits and returns the previous state.
// Returns an array of the dirty bitmap values before clearing.
func (chip *VideoChip) clearDirtyBitmap() [DIRTY_GRID_SIZE]uint64 {
	var snapshot [DIRTY_GRID_SIZE]uint64
	for i := range chip.dirtyBitmap {
		snapshot[i] = chip.dirtyBitmap[i].Swap(0)
	}
	return snapshot
}

func (chip *VideoChip) markRegionDirty(x, y int) {
	/*
	   markRegionDirty identifies and marks a pixel region as modified.

	   This function uses lock-free atomic operations to mark the tile containing
	   the given pixel coordinate (x, y) as dirty.

	   Parameters:
	     - x: The x-coordinate (in pixels) that triggered the dirty region.
	     - y: The y-coordinate (in pixels) that triggered the dirty region.
	*/

	// Lock-free atomic bitmap update
	chip.markTileDirtyAtomic(x, y)
}

// markRectDirty marks all tiles covered by a rectangle as dirty in one pass.
func (chip *VideoChip) markRectDirty(x, y, w, h int) {
	tw := int(chip.tileWidth)
	th := int(chip.tileHeight)
	if tw <= 0 || th <= 0 {
		return
	}
	tileX0 := x / tw
	tileY0 := y / th
	tileX1 := (x + w - 1) / tw
	tileY1 := (y + h - 1) / th
	if tileX0 < 0 {
		tileX0 = 0
	}
	if tileY0 < 0 {
		tileY0 = 0
	}
	if tileX1 >= DIRTY_GRID_COLS {
		tileX1 = DIRTY_GRID_COLS - 1
	}
	if tileY1 >= DIRTY_GRID_ROWS {
		tileY1 = DIRTY_GRID_ROWS - 1
	}
	for ty := tileY0; ty <= tileY1; ty++ {
		for tx := tileX0; tx <= tileX1; tx++ {
			bitIndex := ty*DIRTY_GRID_COLS + tx
			wordIndex := bitIndex / DIRTY_BITS_PER_WORD
			bitOffset := uint(bitIndex % DIRTY_BITS_PER_WORD)
			for {
				old := chip.dirtyBitmap[wordIndex].Load()
				newVal := old | (1 << bitOffset)
				if old == newVal || chip.dirtyBitmap[wordIndex].CompareAndSwap(old, newVal) {
					break
				}
			}
		}
	}
}

func (chip *VideoChip) refreshLoop() {
	/*
	   refreshLoop handles periodic display updates at the configured refresh rate.

	   Every tick it:
	   1. Checks if the video output is enabled.
	   2. Uses lock-free atomic check to see if any tiles are dirty.
	   3. If content is present, copies dirty tiles from front buffer to back buffer.
	   4. Swaps the front and back buffers and increments the frame counter.
	   5. Sends the updated frame to the display output.
	   6. If no content exists but a splash image is available, it displays the splash image.

	   Thread Safety:
	   Uses lock-free atomic bitmap for dirty checking. Mutex is only acquired
	   when buffer operations are needed.
	*/

	ticker := time.NewTicker(REFRESH_INTERVAL)
	defer ticker.Stop()

	// VBlank timing: using 50% window for reliable M68K polling detection
	// Active display is first 50% of frame, VBlank is last 50%
	vblankStartDelay := REFRESH_INTERVAL / 2 // VBlank starts after 50% of frame
	_ = vblankStartDelay                     // Reserved for future use

	// Frame counter for VBlank race prevention
	var currentFrame uint64

	// Initialize lastFrameStart so M68K polling works before first ticker fires
	chip.lastFrameStart.Store(time.Now().UnixNano())

	for {
		select {
		case <-chip.done:
			return
		case <-ticker.C:
			// Start of new frame - record start time for VBlank calculation
			currentFrame++
			chip.lastFrameStart.Store(time.Now().UnixNano())
			chip.inVBlank.Store(false) // Legacy support

			if !chip.enabled.Load() {
				continue
			}

			// Minimize mutex hold time: do state updates under lock, then release before slow I/O
			chip.mu.Lock()
			mode := VideoModes[chip.currentMode]
			// Only run copper if compositor isn't managing it per-scanline
			if !chip.copperManagedByCompositor {
				chip.advanceCopperFrameLocked(mode)
			}
			chip.runBlitterLocked(mode)

			// Lock-free check: skip mutex if no dirty tiles
			hasDirty := chip.hasDirtyTiles()
			hasContent := chip.hasContent.Load()
			var frameToSend []byte

			if hasContent {
				if hasDirty {
					tileW := int(chip.tileWidth)
					tileH := int(chip.tileHeight)

					// Atomically get and clear dirty bitmap
					dirtySnapshot := chip.clearDirtyBitmap()

					// Process dirty tiles from atomic bitmap
					for wordIdx, bits := range dirtySnapshot {
						if bits == 0 {
							continue
						}
						for bitIdx := range DIRTY_BITS_PER_WORD {
							if bits&(1<<uint(bitIdx)) == 0 {
								continue
							}
							// Calculate tile coordinates
							tileIndex := wordIdx*DIRTY_BITS_PER_WORD + bitIdx
							tileX := tileIndex % DIRTY_GRID_COLS
							tileY := tileIndex / DIRTY_GRID_COLS

							// Calculate pixel coordinates
							startX := tileX * tileW
							startY := tileY * tileH
							endX := startX + tileW
							endY := startY + tileH

							// Clamp to screen bounds
							if endX > mode.width {
								endX = mode.width
							}
							if endY > mode.height {
								endY = mode.height
							}

							// Copy tile from front to back buffer
							// Pre-compute copyLen (constant for this tile)
							copyLen := (endX - startX) * BYTES_PER_PIXEL
							// Pre-compute initial offset and stride
							srcOffset := (startY * mode.bytesPerRow) + (startX * BYTES_PER_PIXEL)
							// Validate bounds once before the loop
							maxOffset := srcOffset + (endY-startY-1)*mode.bytesPerRow + copyLen
							if maxOffset <= len(chip.frontBuffer) {
								for y := startY; y < endY; y++ {
									copy(chip.backBuffer[srcOffset:srcOffset+copyLen],
										chip.frontBuffer[srcOffset:srcOffset+copyLen])
									srcOffset += mode.bytesPerRow
								}
							}
						}
					}

					chip.frontBuffer, chip.backBuffer = chip.backBuffer, chip.frontBuffer
					chip.frameCounter += FRAME_INCREMENT
				}
				frameToSend = chip.frontBuffer
			} else if chip.splashBuffer != nil {
				frameToSend = chip.splashBuffer
			}
			chip.mu.Unlock()

			// Note: Frame output is handled by the compositor, which calls GetFrame()
			// VideoChip no longer sends directly to output
			_ = frameToSend // Buffer management still happens, compositor reads via GetFrame()
		}
	}
}

func (chip *VideoChip) runBlitterLocked(mode VideoMode) {
	if !chip.bltPending || chip.bus == nil {
		return
	}

	chip.bltPending = false
	chip.executeBlitterLocked(mode)
	chip.bltBusy = false
	chip.bltDone = true
	if chip.bltIrqEnabled {
		chip.bltIrqPend = true
		if chip.intSink != nil {
			chip.intSink.Pulse(IntMaskBlitter)
		}
	}
}

func (chip *VideoChip) executeBlitterLocked(mode VideoMode) {
	switch chip.bltOp {
	case bltOpCopy:
		chip.blitCopyLocked(mode)
	case bltOpFill:
		chip.blitFillLocked(mode)
	case bltOpLine:
		chip.blitLineLocked(mode)
	case bltOpMaskedCopy:
		chip.blitMaskedCopyLocked(mode)
	case bltOpAlphaCopy:
		chip.blitAlphaCopyLocked(mode)
	case bltOpMode7:
		chip.blitMode7Locked(mode)
	case bltOpColorExpand:
		chip.blitColorExpandLocked(mode)
	default:
		chip.bltErr = true
	}
}

// applyDrawMode applies one of 16 standard raster ops to src and dst pixels.
func applyDrawMode(src, dst uint32, mode int) uint32 {
	switch mode {
	case 0x00: // Clear
		return 0
	case 0x01: // And
		return src & dst
	case 0x02: // AndReverse
		return src & ^dst
	case 0x03: // Copy
		return src
	case 0x04: // AndInverted
		return ^src & dst
	case 0x05: // NoOp
		return dst
	case 0x06: // Xor
		return src ^ dst
	case 0x07: // Or
		return src | dst
	case 0x08: // Nor
		return ^(src | dst)
	case 0x09: // Equiv
		return ^(src ^ dst)
	case 0x0A: // Invert
		return ^dst
	case 0x0B: // OrReverse
		return src | ^dst
	case 0x0C: // CopyInverted
		return ^src
	case 0x0D: // OrInverted
		return ^src | dst
	case 0x0E: // Nand
		return ^(src & dst)
	case 0x0F: // Set
		return 0xFFFFFFFF
	default:
		return src
	}
}

// bppFromFlags returns bytes per pixel from BLT_FLAGS (1 for CLUT8, 4 for RGBA32).
func bppFromFlags(flags uint32) int {
	if flags&bltFlagsBPPMask == bltFlagsBPP_CLUT8 {
		return 1
	}
	return 4
}

// drawModeFromFlags extracts the draw mode (0-15) from BLT_FLAGS.
// When BLT_FLAGS is 0 (default/legacy), returns Copy mode (0x03) for backward compatibility.
func drawModeFromFlags(flags uint32) int {
	if flags == 0 {
		return 0x03
	}
	return int((flags & bltFlagsDrawModeMask) >> bltFlagsDrawModeShift)
}

func (chip *VideoChip) blitFillLocked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}

	bpp := bppFromFlags(chip.bltFlags)
	drawMode := drawModeFromFlags(chip.bltFlags)
	stride := chip.bltDstStrideRun
	if stride == 0 {
		if bpp == 1 {
			stride = chip.defaultStrideBPP(chip.bltDst, width, 1, mode)
		} else {
			stride = chip.defaultStride(chip.bltDst, width, mode)
		}
	}
	color := chip.bltColor

	// Non-copy draw mode: per-pixel read-modify-write
	if drawMode != 0x03 {
		rowAddr := chip.bltDst
		for range height {
			addr := rowAddr
			for range width {
				if bpp == 1 {
					dst := uint32(chip.blitRead8Locked(addr))
					result := applyDrawMode(color&0xFF, dst, drawMode)
					chip.blitWrite8Locked(addr, uint8(result), mode)
					addr++
				} else {
					dst := chip.blitReadPixelLocked(addr)
					result := applyDrawMode(color, dst, drawMode)
					chip.blitWritePixelLocked(addr, result, mode)
					addr += BYTES_PER_PIXEL
				}
			}
			rowAddr += stride
		}
		return
	}

	// Copy draw mode: bulk fill
	dst := chip.bltDst
	if bpp == 1 {
		chip.blitBulkFill8Locked(dst, width, height, stride, uint8(color), mode)
	} else {
		chip.blitBulkFill32Locked(dst, width, height, stride, color, mode)
	}
}

// blitBulkFill32Locked fills a rectangle with a 32-bit color using bulk operations.
func (chip *VideoChip) blitBulkFill32Locked(dst uint32, width, height int, stride, color uint32, mode VideoMode) {
	rowBytes := uint32(width * BYTES_PER_PIXEL)

	if chip.directVRAM != nil && dst >= VRAM_START && dst < VRAM_START+VRAM_SIZE {
		// directVRAM path: write to busMemory
		if chip.busMemory == nil {
			return
		}
		endAddr := uint64(dst) + uint64(stride)*uint64(height-1) + uint64(rowBytes)
		if endAddr > uint64(len(chip.busMemory)) {
			return
		}
		// Fill first row
		addr := dst
		for range width {
			binary.LittleEndian.PutUint32(chip.busMemory[addr:addr+4], color)
			addr += BYTES_PER_PIXEL
		}
		// Copy first row to remaining rows
		firstRow := chip.busMemory[dst : dst+rowBytes]
		rowAddr := dst + stride
		for y := 1; y < height; y++ {
			copy(chip.busMemory[rowAddr:rowAddr+rowBytes], firstRow)
			rowAddr += stride
		}
		if !chip.resetting && !chip.hasContent.Load() {
			chip.hasContent.Store(true)
		}
		return
	}

	if dst >= VRAM_START && dst < VRAM_START+VRAM_SIZE {
		// frontBuffer path
		offset := dst - BUFFER_OFFSET
		bufLen := uint64(len(chip.frontBuffer))
		lastOffset := uint64(offset) + uint64(stride)*uint64(height-1) + uint64(rowBytes)
		if lastOffset > bufLen || offset%BYTES_PER_PIXEL != BUFFER_REMAINDER {
			chip.bltErr = true
			return
		}
		// Fill first row
		addr := offset
		for range width {
			binary.LittleEndian.PutUint32(chip.frontBuffer[addr:], color)
			addr += BYTES_PER_PIXEL
		}
		// Copy first row to remaining rows
		firstRow := chip.frontBuffer[offset : offset+rowBytes]
		rowOff := offset + stride
		for y := 1; y < height; y++ {
			copy(chip.frontBuffer[rowOff:rowOff+rowBytes], firstRow)
			rowOff += stride
		}
		// Mark dirty rectangle
		startPixel := offset / BYTES_PER_PIXEL
		startX := int(startPixel) % mode.width
		startY := int(startPixel) / mode.width
		chip.markRectDirty(startX, startY, width, height)
		if !chip.resetting && !chip.hasContent.Load() {
			chip.hasContent.Store(true)
		}
		return
	}

	// Non-VRAM: per-pixel bus writes
	rowAddr := dst
	for range height {
		addr := rowAddr
		for range width {
			chip.bus.Write32(addr, color)
			addr += BYTES_PER_PIXEL
		}
		rowAddr += stride
	}
}

// blitBulkFill8Locked fills a rectangle with an 8-bit color using bulk operations.
func (chip *VideoChip) blitBulkFill8Locked(dst uint32, width, height int, stride uint32, color uint8, mode VideoMode) {
	rowBytes := uint32(width)

	if chip.directVRAM != nil && dst >= VRAM_START && dst < VRAM_START+VRAM_SIZE {
		if chip.busMemory == nil {
			return
		}
		endAddr := dst + stride*uint32(height-1) + rowBytes
		if endAddr > uint32(len(chip.busMemory)) {
			return
		}
		rowAddr := dst
		for range height {
			for i := range width {
				chip.busMemory[rowAddr+uint32(i)] = color
			}
			rowAddr += stride
		}
		if !chip.resetting && !chip.hasContent.Load() {
			chip.hasContent.Store(true)
		}
		return
	}

	if dst >= VRAM_START && dst < VRAM_START+VRAM_SIZE {
		offset := dst - BUFFER_OFFSET
		bufLen := uint32(len(chip.frontBuffer))
		lastOffset := offset + stride*uint32(height-1) + rowBytes
		if lastOffset > bufLen {
			chip.bltErr = true
			return
		}
		rowOff := offset
		for range height {
			for i := range width {
				chip.frontBuffer[rowOff+uint32(i)] = color
			}
			rowOff += stride
		}
		// Dirty tracking uses pixel coords; CLUT8 pixels are 1 byte each
		startX := int(offset) % mode.width
		startY := int(offset) / mode.width
		chip.markRectDirty(startX, startY, width, height)
		if !chip.resetting && !chip.hasContent.Load() {
			chip.hasContent.Store(true)
		}
		return
	}

	// Non-VRAM: per-byte bus writes
	rowAddr := dst
	for range height {
		addr := rowAddr
		for range width {
			chip.bus.Write8(addr, color)
			addr++
		}
		rowAddr += stride
	}
}

func (chip *VideoChip) blitCopyLocked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}

	bpp := bppFromFlags(chip.bltFlags)
	drawMode := drawModeFromFlags(chip.bltFlags)
	bytesPerPx := uint32(bpp)

	srcStride := chip.bltSrcStrideRun
	if srcStride == 0 {
		if bpp == 1 {
			srcStride = chip.defaultStrideBPP(chip.bltSrc, width, 1, mode)
		} else {
			srcStride = chip.defaultStride(chip.bltSrc, width, mode)
		}
	}
	dstStride := chip.bltDstStrideRun
	if dstStride == 0 {
		if bpp == 1 {
			dstStride = chip.defaultStrideBPP(chip.bltDst, width, 1, mode)
		} else {
			dstStride = chip.defaultStride(chip.bltDst, width, mode)
		}
	}

	// Non-copy draw mode: per-pixel read-modify-write
	if drawMode != 0x03 {
		srcRow := chip.bltSrc
		dstRow := chip.bltDst
		for range height {
			srcAddr := srcRow
			dstAddr := dstRow
			for range width {
				if bpp == 1 {
					src := uint32(chip.blitRead8Locked(srcAddr))
					dst := uint32(chip.blitRead8Locked(dstAddr))
					result := applyDrawMode(src, dst, drawMode)
					chip.blitWrite8Locked(dstAddr, uint8(result), mode)
				} else {
					src := chip.blitReadPixelLocked(srcAddr)
					dst := chip.blitReadPixelLocked(dstAddr)
					result := applyDrawMode(src, dst, drawMode)
					chip.blitWritePixelLocked(dstAddr, result, mode)
				}
				srcAddr += bytesPerPx
				dstAddr += bytesPerPx
			}
			srcRow += srcStride
			dstRow += dstStride
		}
		return
	}

	// Copy draw mode: try bulk path for VRAM
	src := chip.bltSrc
	dst := chip.bltDst
	rowBytes := uint32(width) * bytesPerPx

	// Both in directVRAM busMemory
	if chip.directVRAM != nil && chip.busMemory != nil &&
		src >= VRAM_START && src < VRAM_START+VRAM_SIZE &&
		dst >= VRAM_START && dst < VRAM_START+VRAM_SIZE {
		srcEnd := uint64(src) + uint64(srcStride)*uint64(height-1) + uint64(rowBytes)
		dstEnd := uint64(dst) + uint64(dstStride)*uint64(height-1) + uint64(rowBytes)
		if srcEnd <= uint64(len(chip.busMemory)) && dstEnd <= uint64(len(chip.busMemory)) {
			// Overlap detection: copy forward or backward
			if dst > src && dst < src+srcStride*uint32(height) {
				// Backward (bottom to top)
				srcRow := src + srcStride*uint32(height-1)
				dstRow := dst + dstStride*uint32(height-1)
				for y := height - 1; y >= 0; y-- {
					copy(chip.busMemory[dstRow:dstRow+rowBytes], chip.busMemory[srcRow:srcRow+rowBytes])
					srcRow -= srcStride
					dstRow -= dstStride
				}
			} else {
				// Forward (top to bottom)
				srcRow := src
				dstRow := dst
				for range height {
					copy(chip.busMemory[dstRow:dstRow+rowBytes], chip.busMemory[srcRow:srcRow+rowBytes])
					srcRow += srcStride
					dstRow += dstStride
				}
			}
			if !chip.resetting && !chip.hasContent.Load() {
				chip.hasContent.Store(true)
			}
			return
		}
	}

	// Both in frontBuffer (non-directVRAM VRAM)
	if chip.directVRAM == nil &&
		src >= VRAM_START && src < VRAM_START+VRAM_SIZE &&
		dst >= VRAM_START && dst < VRAM_START+VRAM_SIZE {
		srcOff := src - BUFFER_OFFSET
		dstOff := dst - BUFFER_OFFSET
		bufLen := uint64(len(chip.frontBuffer))
		srcEnd := uint64(srcOff) + uint64(srcStride)*uint64(height-1) + uint64(rowBytes)
		dstEnd := uint64(dstOff) + uint64(dstStride)*uint64(height-1) + uint64(rowBytes)
		if srcEnd <= bufLen && dstEnd <= bufLen {
			if dstOff > srcOff && dstOff < srcOff+srcStride*uint32(height) {
				srcRow := srcOff + srcStride*uint32(height-1)
				dstRow := dstOff + dstStride*uint32(height-1)
				for y := height - 1; y >= 0; y-- {
					copy(chip.frontBuffer[dstRow:dstRow+rowBytes], chip.frontBuffer[srcRow:srcRow+rowBytes])
					srcRow -= srcStride
					dstRow -= dstStride
				}
			} else {
				srcRow := srcOff
				dstRow := dstOff
				for range height {
					copy(chip.frontBuffer[dstRow:dstRow+rowBytes], chip.frontBuffer[srcRow:srcRow+rowBytes])
					srcRow += srcStride
					dstRow += dstStride
				}
			}
			startPixel := dstOff / bytesPerPx
			startX := int(startPixel) % mode.width
			startY := int(startPixel) / mode.width
			chip.markRectDirty(startX, startY, width, height)
			if !chip.resetting && !chip.hasContent.Load() {
				chip.hasContent.Store(true)
			}
			return
		}
	}

	// Fallback: per-pixel copy
	srcRow := chip.bltSrc
	dstRow := chip.bltDst
	for range height {
		srcAddr := srcRow
		dstAddr := dstRow
		for range width {
			if bpp == 1 {
				v := chip.blitRead8Locked(srcAddr)
				chip.blitWrite8Locked(dstAddr, v, mode)
			} else {
				value := chip.blitReadPixelLocked(srcAddr)
				chip.blitWritePixelLocked(dstAddr, value, mode)
			}
			srcAddr += bytesPerPx
			dstAddr += bytesPerPx
		}
		srcRow += srcStride
		dstRow += dstStride
	}
}

func (chip *VideoChip) blitMaskedCopyLocked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}
	srcStride := chip.bltSrcStrideRun
	if srcStride == 0 {
		srcStride = chip.defaultStride(chip.bltSrc, width, mode)
	}
	dstStride := chip.bltDstStrideRun
	if dstStride == 0 {
		dstStride = chip.defaultStride(chip.bltDst, width, mode)
	}
	maskStride := uint32((width + 7) / 8)

	srcRow := chip.bltSrc
	dstRow := chip.bltDst
	maskRow := chip.bltMask
	for range height {
		srcAddr := srcRow
		dstAddr := dstRow
		maskAddr := maskRow
		for x := range width {
			maskByte := chip.busRead8Locked(maskAddr + uint32(x/8))
			if (maskByte>>uint(x%8))&1 == 0 {
				srcAddr += BYTES_PER_PIXEL
				dstAddr += BYTES_PER_PIXEL
				continue
			}
			value := chip.blitReadPixelLocked(srcAddr)
			chip.blitWritePixelLocked(dstAddr, value, mode)
			srcAddr += BYTES_PER_PIXEL
			dstAddr += BYTES_PER_PIXEL
		}
		srcRow += srcStride
		dstRow += dstStride
		maskRow += maskStride
	}
}

func (chip *VideoChip) blitAlphaCopyLocked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}
	srcStride := chip.bltSrcStrideRun
	if srcStride == 0 {
		srcStride = chip.defaultStride(chip.bltSrc, width, mode)
	}
	dstStride := chip.bltDstStrideRun
	if dstStride == 0 {
		dstStride = chip.defaultStride(chip.bltDst, width, mode)
	}

	srcRow := chip.bltSrc
	dstRow := chip.bltDst
	for range height {
		srcAddr := srcRow
		dstAddr := dstRow
		for range width {
			value := chip.blitReadPixelLocked(srcAddr)
			alpha := (value >> 24) & 0xFF
			if alpha == 0 {
				srcAddr += BYTES_PER_PIXEL
				dstAddr += BYTES_PER_PIXEL
				continue
			}
			if alpha == 0xFF {
				chip.blitWritePixelLocked(dstAddr, value, mode)
			} else {
				dst := chip.blitReadPixelLocked(dstAddr)
				inv := 255 - alpha
				sr := value & 0xFF
				sg := (value >> 8) & 0xFF
				sb := (value >> 16) & 0xFF
				dr := dst & 0xFF
				dg := (dst >> 8) & 0xFF
				db := (dst >> 16) & 0xFF
				da := (dst >> 24) & 0xFF
				outR := (sr*alpha + dr*inv) / 255
				outG := (sg*alpha + dg*inv) / 255
				outB := (sb*alpha + db*inv) / 255
				outA := alpha + (da*inv)/255
				chip.blitWritePixelLocked(dstAddr, outR|(outG<<8)|(outB<<16)|(outA<<24), mode)
			}
			srcAddr += BYTES_PER_PIXEL
			dstAddr += BYTES_PER_PIXEL
		}
		srcRow += srcStride
		dstRow += dstStride
	}
}

func (chip *VideoChip) blitMode7Locked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}

	// Validate masks are power of 2 minus 1
	if !isValidMask(chip.bltMode7TexW) || !isValidMask(chip.bltMode7TexH) {
		chip.bltErr = true
		return
	}

	texMaskU := int32(chip.bltMode7TexW)
	texMaskV := int32(chip.bltMode7TexH)

	srcStride := chip.bltSrcStrideRun
	if srcStride == 0 {
		srcStride = uint32(texMaskU+1) * 4
	}
	dstStride := chip.bltDstStrideRun
	if dstStride == 0 {
		dstStride = chip.defaultStride(chip.bltDst, width, mode)
	}

	// Fixed point coordinates (16.16)
	rowU := int32(chip.bltMode7U0)
	rowV := int32(chip.bltMode7V0)
	duCol := int32(chip.bltMode7DuCol)
	dvCol := int32(chip.bltMode7DvCol)
	duRow := int32(chip.bltMode7DuRow)
	dvRow := int32(chip.bltMode7DvRow)

	dstRow := chip.bltDst

	for range height {
		u := rowU
		v := rowV
		dstAddr := dstRow

		for range width {
			uInt := (u >> 16) & texMaskU
			vInt := (v >> 16) & texMaskV

			texOff := uint64(uint32(vInt))*uint64(srcStride) + uint64(uint32(uInt))*BYTES_PER_PIXEL
			texAddr64 := uint64(chip.bltSrc) + texOff
			if texAddr64+BYTES_PER_PIXEL > math.MaxUint32 {
				chip.bltErr = true
				return
			}
			texAddr := uint32(texAddr64)
			if chip.busMemory != nil && texAddr64+BYTES_PER_PIXEL > uint64(len(chip.busMemory)) {
				chip.bltErr = true
				return
			}
			texel := chip.blitReadPixelLocked(texAddr)

			// Write destination
			chip.blitWritePixelLocked(dstAddr, texel, mode)

			dstAddr += BYTES_PER_PIXEL
			u += duCol
			v += dvCol
		}

		rowU += duRow
		rowV += dvRow
		dstRow += dstStride
	}
}

func isValidMask(mask uint32) bool {
	// Check if mask is 2^n - 1
	// (mask + 1) should be power of 2
	// power of 2 means only one bit set
	v := mask + 1
	return v > 0 && (v&(v-1)) == 0
}

func (chip *VideoChip) blitLineLocked(mode VideoMode) {
	flags := chip.bltFlags
	bpp := bppFromFlags(flags)
	drawMode := drawModeFromFlags(flags)

	var base uint32
	var stride int
	var clipW, clipH int
	x0 := int(uint16(chip.bltSrc & 0xFFFF))
	y0 := int(uint16((chip.bltSrc >> 16) & 0xFFFF))
	var x1, y1 int

	if flags != 0 {
		// Extended mode: BLT_DST = framebuffer base, BLT_WIDTH = packed endpoint,
		// BLT_DST_STRIDE = row stride.
		base = chip.bltDst
		x1 = int(uint16(chip.bltWidth & 0xFFFF))
		y1 = int(uint16((chip.bltWidth >> 16) & 0xFFFF))
		if chip.bltDstStrideRun != 0 {
			stride = int(chip.bltDstStrideRun)
		} else {
			stride = int(chip.defaultStrideBPP(base, mode.width, bpp, mode))
		}
		// No viewport clipping in extended mode — the caller must provide
		// pre-clipped coordinates. Pixel writes are still bounds-checked
		// by blitWritePixelLocked/blitWrite8Locked against VRAM/buffer limits.
		clipW = 0x7FFF
		clipH = 0x7FFF
	} else {
		// Legacy mode: BLT_DST = packed endpoint, base = VRAM_START.
		x1 = int(uint16(chip.bltDst & 0xFFFF))
		y1 = int(uint16((chip.bltDst >> 16) & 0xFFFF))
		base = VRAM_START
		stride = mode.width * bpp
		clipW = mode.width
		clipH = mode.height
	}

	dx := absInt(x1 - x0)
	dy := absInt(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	for {
		if x0 >= 0 && x0 < clipW && y0 >= 0 && y0 < clipH {
			addr := base + uint32(y0*stride+x0*bpp)
			color := chip.bltColor
			if bpp == 1 {
				if drawMode != 0x03 {
					dst := uint32(chip.blitRead8Locked(addr))
					color = applyDrawMode(color&0xFF, dst, drawMode)
				}
				chip.blitWrite8Locked(addr, uint8(color&0xFF), mode)
			} else {
				if drawMode != 0x03 {
					dst := chip.blitReadPixelLocked(addr)
					color = applyDrawMode(color, dst, drawMode)
				}
				chip.blitWritePixelLocked(addr, color, mode)
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func (chip *VideoChip) blitReadPixelLocked(addr uint32) uint32 {
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		// directVRAM mode: read from busMemory (what GetFrame returns).
		// VRAM byte order is always LE, matching the frontBuffer convention.
		if chip.directVRAM != nil {
			if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
				return binary.LittleEndian.Uint32(chip.busMemory[addr : addr+4])
			}
			return 0
		}
		offset := addr - BUFFER_OFFSET
		// Read from frontBuffer if within display area
		if offset+BYTES_PER_PIXEL <= uint32(len(chip.frontBuffer)) && offset%BYTES_PER_PIXEL == BUFFER_REMAINDER {
			// VRAM pixels are always stored as little-endian RGBA bytes.
			return binary.LittleEndian.Uint32(chip.frontBuffer[offset : offset+4])
		}
		// For VRAM addresses beyond frontBuffer, fall back to busMemory
		// This enables double-buffering by rendering to VRAM offset > one frame
		if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
			return binary.LittleEndian.Uint32(chip.busMemory[addr : addr+4])
		}
		return 0
	}
	if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
		// Non-VRAM pixel sources are raw RGBA bytes, not CPU-endian words.
		// Keep them little-endian so incbin textures render identically on
		// big-endian CPU profiles such as M68K.
		return binary.LittleEndian.Uint32(chip.busMemory[addr : addr+4])
	}
	return 0
}

// busRead32Locked reads a 32-bit value directly from cached bus memory without mutex.
// This avoids deadlock when the video chip holds its mutex and needs to read from memory.
// Uses big-endian byte order when bigEndianMode is set (for M68K programs).
func (chip *VideoChip) busRead32Locked(addr uint32) uint32 {
	if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
		if chip.bigEndianMode {
			return binary.BigEndian.Uint32(chip.busMemory[addr : addr+4])
		}
		return binary.LittleEndian.Uint32(chip.busMemory[addr : addr+4])
	}
	return 0
}

// busRead8Locked reads an 8-bit value directly from cached bus memory without mutex.
func (chip *VideoChip) busRead8Locked(addr uint32) uint8 {
	if chip.busMemory != nil && addr < uint32(len(chip.busMemory)) {
		return chip.busMemory[addr]
	}
	return 0
}

func (chip *VideoChip) blitWritePixelLocked(addr uint32, value uint32, mode VideoMode) {
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		// directVRAM mode: write to busMemory (what GetFrame returns).
		// VRAM byte order is always LE, matching the frontBuffer convention.
		if chip.directVRAM != nil {
			if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
				binary.LittleEndian.PutUint32(chip.busMemory[addr:addr+4], value)
			}
			if !chip.resetting && !chip.hasContent.Load() {
				chip.hasContent.Store(true)
			}
			return
		}
		offset := addr - BUFFER_OFFSET
		if offset+BYTES_PER_PIXEL > uint32(len(chip.frontBuffer)) || offset%BYTES_PER_PIXEL != BUFFER_REMAINDER {
			chip.bltErr = true
			return
		}
		// VRAM pixels are always stored as little-endian RGBA bytes.
		binary.LittleEndian.PutUint32(chip.frontBuffer[offset:], value)
		startPixel := offset / BYTES_PER_PIXEL
		startX := int(startPixel) % mode.width
		startY := int(startPixel) / mode.width
		chip.markRegionDirty(startX, startY)
		if !chip.resetting && !chip.hasContent.Load() {
			chip.hasContent.Store(true)
		}
		return
	}
	chip.bus.Write32(addr, value)
}

func (chip *VideoChip) defaultStride(addr uint32, width int, mode VideoMode) uint32 {
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		return uint32(mode.bytesPerRow)
	}
	return uint32(width * BYTES_PER_PIXEL)
}

func (chip *VideoChip) defaultStrideBPP(addr uint32, width, bpp int, mode VideoMode) uint32 {
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		if bpp == 1 {
			return uint32(mode.width)
		}
		return uint32(mode.bytesPerRow)
	}
	return uint32(width * bpp)
}

// blitRead8Locked reads a single byte from bus memory for CLUT8 operations.
func (chip *VideoChip) blitRead8Locked(addr uint32) uint8 {
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		if chip.directVRAM != nil {
			if chip.busMemory != nil && addr < uint32(len(chip.busMemory)) {
				return chip.busMemory[addr]
			}
			return 0
		}
		offset := addr - BUFFER_OFFSET
		if offset < uint32(len(chip.frontBuffer)) {
			return chip.frontBuffer[offset]
		}
		if chip.busMemory != nil && addr < uint32(len(chip.busMemory)) {
			return chip.busMemory[addr]
		}
		return 0
	}
	if chip.busMemory != nil && addr < uint32(len(chip.busMemory)) {
		return chip.busMemory[addr]
	}
	return 0
}

// blitWrite8Locked writes a single byte for CLUT8 operations.
func (chip *VideoChip) blitWrite8Locked(addr uint32, value uint8, mode VideoMode) {
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		if chip.directVRAM != nil {
			if chip.busMemory != nil && addr < uint32(len(chip.busMemory)) {
				chip.busMemory[addr] = value
			}
			if !chip.resetting && !chip.hasContent.Load() {
				chip.hasContent.Store(true)
			}
			return
		}
		offset := addr - BUFFER_OFFSET
		if offset >= uint32(len(chip.frontBuffer)) {
			chip.bltErr = true
			return
		}
		chip.frontBuffer[offset] = value
		// Dirty tracking: treat CLUT8 pixel coords as byte offset / mode.width
		px := int(offset) % mode.width
		py := int(offset) / mode.width
		chip.markRegionDirty(px, py)
		if !chip.resetting && !chip.hasContent.Load() {
			chip.hasContent.Store(true)
		}
		return
	}
	chip.bus.Write8(addr, value)
}

// blitColorExpandLocked expands a 1-bit template to colored pixels.
func (chip *VideoChip) blitColorExpandLocked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}

	bpp := bppFromFlags(chip.bltFlags)
	bytesPerPx := uint32(bpp)
	jam1 := chip.bltFlags&bltFlagsJAM1 != 0
	invertTmpl := chip.bltFlags&bltFlagsInvertTmpl != 0
	invertMode := chip.bltFlags&bltFlagsInvertMode != 0
	fg := chip.bltFG
	bg := chip.bltBG
	maskAddr := chip.bltMask
	maskMod := chip.bltMaskMod
	maskSrcX := int(chip.bltMaskSrcX)

	dstStride := chip.bltDstStrideRun
	if dstStride == 0 {
		if bpp == 1 {
			dstStride = chip.defaultStrideBPP(chip.bltDst, width, 1, mode)
		} else {
			dstStride = chip.defaultStride(chip.bltDst, width, mode)
		}
	}

	dstRow := chip.bltDst
	for y := range height {
		_ = y
		dstAddr := dstRow
		for x := range width {
			// Read template bit (MSB-first: bit 7 of first byte = leftmost pixel)
			bitX := maskSrcX + x
			byteIdx := uint32(bitX / 8)
			bitIdx := uint(7 - (bitX % 8)) // MSB-first
			tmplByte := chip.busRead8Locked(maskAddr + byteIdx)
			bit := (tmplByte >> bitIdx) & 1
			if invertTmpl {
				bit ^= 1
			}

			if invertMode {
				// Invert mode: XOR destination with all-ones for set bits
				if bit == 1 {
					if bpp == 1 {
						dst := uint32(chip.blitRead8Locked(dstAddr))
						chip.blitWrite8Locked(dstAddr, uint8(dst^0xFF), mode)
					} else {
						dst := chip.blitReadPixelLocked(dstAddr)
						chip.blitWritePixelLocked(dstAddr, dst^0xFFFFFFFF, mode)
					}
				}
				// bit == 0: leave destination unchanged
			} else if bit == 1 {
				// Set bit: write foreground
				if bpp == 1 {
					chip.blitWrite8Locked(dstAddr, uint8(fg), mode)
				} else {
					chip.blitWritePixelLocked(dstAddr, fg, mode)
				}
			} else if !jam1 {
				// Clear bit in JAM2 mode: write background
				if bpp == 1 {
					chip.blitWrite8Locked(dstAddr, uint8(bg), mode)
				} else {
					chip.blitWritePixelLocked(dstAddr, bg, mode)
				}
			}
			// JAM1 + bit==0: leave destination unchanged

			dstAddr += bytesPerPx
		}
		maskAddr += maskMod
		dstRow += dstStride
	}
}

func (chip *VideoChip) advanceCopperFrameLocked(mode VideoMode) {
	if !chip.copperEnabled || chip.bus == nil {
		return
	}

	chip.copperStartFrameLocked()

	for y := 0; y < mode.height; y++ {
		chip.copperAdvanceRasterLocked(y, 0)
		if chip.copperHalted {
			return
		}
		if chip.copperWaiting && chip.copperWaitY == uint16(y) && chip.copperWaitX < uint16(mode.width) {
			chip.copperAdvanceRasterLocked(y, int(chip.copperWaitX))
			if chip.copperHalted {
				return
			}
		}
	}
}

func (chip *VideoChip) copperStartFrameLocked() {
	chip.copperPC = chip.copperPtr
	chip.copperWaiting = false
	chip.copperHalted = false
	chip.copperRasterX = 0
	chip.copperRasterY = 0
	chip.copperIOBase = VIDEO_REG_BASE // Reset to default each frame
}

func (chip *VideoChip) copperAdvanceRasterLocked(y, x int) {
	if chip.copperHalted {
		return
	}

	chip.copperRasterY = uint16(y)
	chip.copperRasterX = uint16(x)

	if chip.copperWaiting {
		if !chip.copperWaitSatisfied() {
			return
		}
		chip.copperWaiting = false
	}

	chip.copperRunLocked()
}

func (chip *VideoChip) copperWaitSatisfied() bool {
	if chip.copperRasterY > chip.copperWaitY {
		return true
	}
	if chip.copperRasterY < chip.copperWaitY {
		return false
	}
	return chip.copperRasterX >= chip.copperWaitX
}

func (chip *VideoChip) copperRunLocked() {
	const maxOps = 0x10000
	ops := 0

	for !chip.copperWaiting && !chip.copperHalted && ops < maxOps {
		ops++
		word := chip.busRead32Locked(chip.copperPC)
		opcode := word >> copperOpcodeShift

		switch opcode {
		case copperOpcodeWait:
			waitY := uint16((word >> copperYShift) & copperCoordMask)
			waitX := uint16(word & copperCoordMask)
			chip.copperPC += 4
			chip.copperWaitY = waitY
			chip.copperWaitX = waitX
			if chip.copperWaitSatisfied() {
				continue
			}
			chip.copperWaiting = true
			return
		case copperOpcodeMove:
			regIndex := (word >> copperRegShift) & copperRegMask
			value := chip.busRead32Locked(chip.copperPC + 4)
			chip.copperPC += 8
			regAddr := chip.copperIOBase + (regIndex * 4)
			// Route to appropriate device based on address
			if regAddr >= VIDEO_REG_BASE && regAddr <= VIDEO_REG_END {
				chip.handleWriteLocked(regAddr, value)
			} else if chip.bus != nil {
				chip.bus.Write32(regAddr, value)
			}
		case copperOpcodeSetBase:
			// Base address encoded as (addr >> 2) in bits 0-23
			baseShifted := word & copperSetBaseMask
			chip.copperIOBase = baseShifted << 2
			chip.copperPC += 4
		case copperOpcodeEnd:
			chip.copperPC += 4
			chip.copperHalted = true
		default:
			chip.copperHalted = true
		}
	}
}

func (chip *VideoChip) RunCopperFrameForTest() {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	mode := VideoModes[chip.currentMode]
	chip.advanceCopperFrameLocked(mode)
}

func (chip *VideoChip) StepCopperRasterForTest(y, x int) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.copperAdvanceRasterLocked(y, x)
}

func (chip *VideoChip) RunBlitterForTest() {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	mode := VideoModes[chip.currentMode]
	chip.runBlitterLocked(mode)
}

func (chip *VideoChip) HandleRead(addr uint32) uint32 {
	/*
		HandleRead processes a read request from the memory-mapped register interface.

		Parameters:
		 - addr: The memory address to read from.

		Returns:
		 - uint32: The value found at the specified address (register or VRAM).

		The function performs boundary and alignment checks to ensure valid memory access.
		Thread Safety:
		A read-lock is acquired during the operation.
		VIDEO_STATUS is read lock-free to allow VBlank polling during refresh.
	*/

	// Lock-free read for VIDEO_STATUS to allow VBlank polling without blocking
	// during the refresh loop (which holds the write lock)
	if addr == VIDEO_STATUS {
		status := uint32(0)
		if chip.hasContent.Load() {
			status |= 1 // bit 0: has content
		}
		if chip.inVBlank.Load() {
			status |= 2 // bit 1: in VBlank
		} else if !chip.everSignaled.Load() {
			frameStart := chip.lastFrameStart.Load()
			now := time.Now().UnixNano()
			vblankThresholdNs := int64(REFRESH_INTERVAL / 2)
			if now-frameStart >= vblankThresholdNs {
				status |= 2 // bit 1: in VBlank
			}
		}

		return status
	}

	chip.mu.Lock()
	defer chip.mu.Unlock()

	switch addr {
	case VIDEO_CTRL:
		return btou32(chip.enabled.Load())
	case VIDEO_MODE:
		return chip.currentMode
	case COPPER_PC:
		return chip.copperPC
	case COPPER_STATUS:
		return chip.copperStatusLocked()
	case BLT_CTRL:
		return chip.blitterCtrlValueLocked()
	case BLT_OP:
		return chip.bltOpStaged
	case BLT_SRC:
		return chip.bltSrcStaged
	case BLT_DST:
		return chip.bltDstStaged
	case BLT_WIDTH:
		return chip.bltWidthStaged
	case BLT_HEIGHT:
		return chip.bltHeightStaged
	case BLT_SRC_STRIDE:
		return chip.bltSrcStride
	case BLT_DST_STRIDE:
		return chip.bltDstStride
	case BLT_COLOR:
		return chip.bltColorStaged
	case BLT_MASK:
		return chip.bltMaskStaged
	case BLT_STATUS:
		return chip.blitterStatusLocked()
	case VIDEO_RASTER_Y:
		return chip.rasterY
	case VIDEO_RASTER_HEIGHT:
		return chip.rasterHeight
	case VIDEO_RASTER_COLOR:
		return chip.rasterColor
	case VIDEO_RASTER_CTRL:
		return chip.rasterCtrl
	case BLT_MODE7_U0:
		return chip.bltMode7U0Staged
	case BLT_MODE7_V0:
		return chip.bltMode7V0Staged
	case BLT_MODE7_DU_COL:
		return chip.bltMode7DuColStaged
	case BLT_MODE7_DV_COL:
		return chip.bltMode7DvColStaged
	case BLT_MODE7_DU_ROW:
		return chip.bltMode7DuRowStaged
	case BLT_MODE7_DV_ROW:
		return chip.bltMode7DvRowStaged
	case BLT_MODE7_TEX_W:
		return chip.bltMode7TexWStaged
	case BLT_MODE7_TEX_H:
		return chip.bltMode7TexHStaged
	case BLT_FLAGS:
		return chip.bltFlagsStaged
	case BLT_FG:
		return chip.bltFGStaged
	case BLT_BG:
		return chip.bltBGStaged
	case BLT_MASK_MOD:
		return chip.bltMaskModStaged
	case BLT_MASK_SRCX:
		return chip.bltMaskSrcXStaged
	case COPPER_PC + 1:
		return readUint32Byte(chip.copperPC, 1)
	case COPPER_PC + 2:
		return readUint32Byte(chip.copperPC, 2)
	case COPPER_PC + 3:
		return readUint32Byte(chip.copperPC, 3)
	case COPPER_STATUS + 1:
		return readUint32Byte(chip.copperStatusLocked(), 1)
	case COPPER_STATUS + 2:
		return readUint32Byte(chip.copperStatusLocked(), 2)
	case COPPER_STATUS + 3:
		return readUint32Byte(chip.copperStatusLocked(), 3)
	case BLT_CTRL + 1:
		return readUint32Byte(chip.blitterCtrlValueLocked(), 1)
	case BLT_CTRL + 2:
		return readUint32Byte(chip.blitterCtrlValueLocked(), 2)
	case BLT_CTRL + 3:
		return readUint32Byte(chip.blitterCtrlValueLocked(), 3)
	case BLT_OP + 1:
		return readUint32Byte(chip.bltOpStaged, 1)
	case BLT_OP + 2:
		return readUint32Byte(chip.bltOpStaged, 2)
	case BLT_OP + 3:
		return readUint32Byte(chip.bltOpStaged, 3)
	case BLT_SRC + 1:
		return readUint32Byte(chip.bltSrcStaged, 1)
	case BLT_SRC + 2:
		return readUint32Byte(chip.bltSrcStaged, 2)
	case BLT_SRC + 3:
		return readUint32Byte(chip.bltSrcStaged, 3)
	case BLT_DST + 1:
		return readUint32Byte(chip.bltDstStaged, 1)
	case BLT_DST + 2:
		return readUint32Byte(chip.bltDstStaged, 2)
	case BLT_DST + 3:
		return readUint32Byte(chip.bltDstStaged, 3)
	case BLT_WIDTH + 1:
		return readUint32Byte(chip.bltWidthStaged, 1)
	case BLT_WIDTH + 2:
		return readUint32Byte(chip.bltWidthStaged, 2)
	case BLT_WIDTH + 3:
		return readUint32Byte(chip.bltWidthStaged, 3)
	case BLT_HEIGHT + 1:
		return readUint32Byte(chip.bltHeightStaged, 1)
	case BLT_HEIGHT + 2:
		return readUint32Byte(chip.bltHeightStaged, 2)
	case BLT_HEIGHT + 3:
		return readUint32Byte(chip.bltHeightStaged, 3)
	case BLT_SRC_STRIDE + 1:
		return readUint32Byte(chip.bltSrcStride, 1)
	case BLT_SRC_STRIDE + 2:
		return readUint32Byte(chip.bltSrcStride, 2)
	case BLT_SRC_STRIDE + 3:
		return readUint32Byte(chip.bltSrcStride, 3)
	case BLT_DST_STRIDE + 1:
		return readUint32Byte(chip.bltDstStride, 1)
	case BLT_DST_STRIDE + 2:
		return readUint32Byte(chip.bltDstStride, 2)
	case BLT_DST_STRIDE + 3:
		return readUint32Byte(chip.bltDstStride, 3)
	case BLT_COLOR + 1:
		return readUint32Byte(chip.bltColorStaged, 1)
	case BLT_COLOR + 2:
		return readUint32Byte(chip.bltColorStaged, 2)
	case BLT_COLOR + 3:
		return readUint32Byte(chip.bltColorStaged, 3)
	case BLT_MASK + 1:
		return readUint32Byte(chip.bltMaskStaged, 1)
	case BLT_MASK + 2:
		return readUint32Byte(chip.bltMaskStaged, 2)
	case BLT_MASK + 3:
		return readUint32Byte(chip.bltMaskStaged, 3)
	case BLT_STATUS + 1:
		return readUint32Byte(chip.blitterStatusLocked(), 1)
	case BLT_STATUS + 2:
		return readUint32Byte(chip.blitterStatusLocked(), 2)
	case BLT_STATUS + 3:
		return readUint32Byte(chip.blitterStatusLocked(), 3)
	case VIDEO_RASTER_Y + 1:
		return readUint32Byte(chip.rasterY, 1)
	case VIDEO_RASTER_Y + 2:
		return readUint32Byte(chip.rasterY, 2)
	case VIDEO_RASTER_Y + 3:
		return readUint32Byte(chip.rasterY, 3)
	case VIDEO_RASTER_HEIGHT + 1:
		return readUint32Byte(chip.rasterHeight, 1)
	case VIDEO_RASTER_HEIGHT + 2:
		return readUint32Byte(chip.rasterHeight, 2)
	case VIDEO_RASTER_HEIGHT + 3:
		return readUint32Byte(chip.rasterHeight, 3)
	case VIDEO_RASTER_COLOR + 1:
		return readUint32Byte(chip.rasterColor, 1)
	case VIDEO_RASTER_COLOR + 2:
		return readUint32Byte(chip.rasterColor, 2)
	case VIDEO_RASTER_COLOR + 3:
		return readUint32Byte(chip.rasterColor, 3)
	case BLT_MODE7_U0 + 1:
		return readUint32Byte(chip.bltMode7U0Staged, 1)
	case BLT_MODE7_U0 + 2:
		return readUint32Byte(chip.bltMode7U0Staged, 2)
	case BLT_MODE7_U0 + 3:
		return readUint32Byte(chip.bltMode7U0Staged, 3)
	case BLT_MODE7_V0 + 1:
		return readUint32Byte(chip.bltMode7V0Staged, 1)
	case BLT_MODE7_V0 + 2:
		return readUint32Byte(chip.bltMode7V0Staged, 2)
	case BLT_MODE7_V0 + 3:
		return readUint32Byte(chip.bltMode7V0Staged, 3)
	case BLT_MODE7_DU_COL + 1:
		return readUint32Byte(chip.bltMode7DuColStaged, 1)
	case BLT_MODE7_DU_COL + 2:
		return readUint32Byte(chip.bltMode7DuColStaged, 2)
	case BLT_MODE7_DU_COL + 3:
		return readUint32Byte(chip.bltMode7DuColStaged, 3)
	case BLT_MODE7_DV_COL + 1:
		return readUint32Byte(chip.bltMode7DvColStaged, 1)
	case BLT_MODE7_DV_COL + 2:
		return readUint32Byte(chip.bltMode7DvColStaged, 2)
	case BLT_MODE7_DV_COL + 3:
		return readUint32Byte(chip.bltMode7DvColStaged, 3)
	case BLT_MODE7_DU_ROW + 1:
		return readUint32Byte(chip.bltMode7DuRowStaged, 1)
	case BLT_MODE7_DU_ROW + 2:
		return readUint32Byte(chip.bltMode7DuRowStaged, 2)
	case BLT_MODE7_DU_ROW + 3:
		return readUint32Byte(chip.bltMode7DuRowStaged, 3)
	case BLT_MODE7_DV_ROW + 1:
		return readUint32Byte(chip.bltMode7DvRowStaged, 1)
	case BLT_MODE7_DV_ROW + 2:
		return readUint32Byte(chip.bltMode7DvRowStaged, 2)
	case BLT_MODE7_DV_ROW + 3:
		return readUint32Byte(chip.bltMode7DvRowStaged, 3)
	case BLT_MODE7_TEX_W + 1:
		return readUint32Byte(chip.bltMode7TexWStaged, 1)
	case BLT_MODE7_TEX_W + 2:
		return readUint32Byte(chip.bltMode7TexWStaged, 2)
	case BLT_MODE7_TEX_W + 3:
		return readUint32Byte(chip.bltMode7TexWStaged, 3)
	case BLT_MODE7_TEX_H + 1:
		return readUint32Byte(chip.bltMode7TexHStaged, 1)
	case BLT_MODE7_TEX_H + 2:
		return readUint32Byte(chip.bltMode7TexHStaged, 2)
	case BLT_MODE7_TEX_H + 3:
		return readUint32Byte(chip.bltMode7TexHStaged, 3)
	case BLT_FLAGS + 1:
		return readUint32Byte(chip.bltFlagsStaged, 1)
	case BLT_FLAGS + 2:
		return readUint32Byte(chip.bltFlagsStaged, 2)
	case BLT_FLAGS + 3:
		return readUint32Byte(chip.bltFlagsStaged, 3)
	case BLT_FG + 1:
		return readUint32Byte(chip.bltFGStaged, 1)
	case BLT_FG + 2:
		return readUint32Byte(chip.bltFGStaged, 2)
	case BLT_FG + 3:
		return readUint32Byte(chip.bltFGStaged, 3)
	case BLT_BG + 1:
		return readUint32Byte(chip.bltBGStaged, 1)
	case BLT_BG + 2:
		return readUint32Byte(chip.bltBGStaged, 2)
	case BLT_BG + 3:
		return readUint32Byte(chip.bltBGStaged, 3)
	case BLT_MASK_MOD + 1:
		return readUint32Byte(chip.bltMaskModStaged, 1)
	case BLT_MASK_MOD + 2:
		return readUint32Byte(chip.bltMaskModStaged, 2)
	case BLT_MASK_MOD + 3:
		return readUint32Byte(chip.bltMaskModStaged, 3)
	case BLT_MASK_SRCX + 1:
		return readUint32Byte(chip.bltMaskSrcXStaged, 1)
	case BLT_MASK_SRCX + 2:
		return readUint32Byte(chip.bltMaskSrcXStaged, 2)
	case BLT_MASK_SRCX + 3:
		return readUint32Byte(chip.bltMaskSrcXStaged, 3)
	case VIDEO_PAL_INDEX:
		return chip.palIndex
	case VIDEO_COLOR_MODE:
		return btou32(chip.clutMode.Load())
	case VIDEO_FB_BASE:
		return chip.fbBase
	default:
		// Handle palette table reads (0xF0088-0xF0487)
		if addr >= VIDEO_PAL_TABLE && addr <= VIDEO_PAL_END {
			entryOffset := addr - VIDEO_PAL_TABLE
			if entryOffset%4 == 0 {
				idx := entryOffset / 4
				if idx < 256 {
					return chip.clutPaletteHW[idx]
				}
			}
			return 0
		}
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			// directVRAM mode: always read from busMemory (CPU owns VRAM)
			if chip.directVRAM != nil {
				if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
					return binary.LittleEndian.Uint32(chip.busMemory[addr : addr+4])
				}
				return DEFAULT_RETURN
			}
			offset := addr - ADDR_OFFSET
			if offset+PIXEL_ALIGNMENT <= uint32(len(chip.frontBuffer)) && (offset&PIXEL_ALIGN_MASK) == DEFAULT_RETURN {
				return binary.LittleEndian.Uint32(chip.frontBuffer[offset:])
			}
			if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
				return binary.LittleEndian.Uint32(chip.busMemory[addr : addr+4])
			}
			return DEFAULT_RETURN
		}
	}
	return DEFAULT_RETURN
}

func (chip *VideoChip) copperStatusLocked() uint32 {
	status := uint32(0)
	if chip.copperEnabled && !chip.copperHalted {
		status |= copperStatusRunning
	}
	if chip.copperWaiting {
		status |= copperStatusWaiting
	}
	if chip.copperHalted {
		status |= copperStatusHalted
	}
	return status
}

func (chip *VideoChip) blitterCtrlValueLocked() uint32 {
	ctrl := uint32(0)
	if chip.bltBusy {
		ctrl |= bltCtrlBusy
	}
	if chip.bltIrqEnabled {
		ctrl |= bltCtrlIRQEnable
	}
	return ctrl
}

func (chip *VideoChip) blitterStatusLocked() uint32 {
	status := uint32(0)
	if chip.bltDone {
		status |= bltStatusDone
	}
	if chip.bltIrqPend {
		status |= bltStatusIRQPending
	}
	if chip.bltErr {
		status |= bltStatusErr
	}
	return status
}

func (chip *VideoChip) HandleWrite(addr uint32, value uint32) {
	/*
		HandleWrite processes a write request to the memory-mapped register interface.

		Parameters:
		 - addr: The memory address to write to.
		 - value: The value to be written.

		Depending on the address, this function updates control registers, video mode,
		or writes to VRAM. It performs alignment and bounds checks before writing.
		Thread Safety:
		A full mutex lock is acquired during the write operation.
	*/
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.handleWriteLocked(addr, value)
}

// HandleWrite8 handles byte-wise writes into video MMIO/VRAM.
// This is required for big-endian CPUs (e.g. M68K) that emit 8-bit writes
// for 16/32-bit store operations.
func (chip *VideoChip) HandleWrite8(addr uint32, value uint8) {
	chip.mu.Lock()
	defer chip.mu.Unlock()

	// Byte writes to VRAM must update the framebuffer directly.
	if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
		// directVRAM mode: bus.memory is the source of truth, no frontBuffer update
		if chip.directVRAM != nil {
			return
		}
		offset := addr - BUFFER_OFFSET
		if offset < uint32(len(chip.frontBuffer)) {
			chip.frontBuffer[offset] = value
			mode := VideoModes[chip.currentMode]
			pixelIndex := int(offset) / BYTES_PER_PIXEL
			x := pixelIndex % mode.width
			y := pixelIndex / mode.width
			chip.markRegionDirty(x, y)
			if !chip.resetting && !chip.hasContent.Load() {
				chip.hasContent.Store(true)
			}
		}
		return
	}

	// For register writes (including palette registers), reconstruct the
	// full 32-bit register value from mirrored bus bytes, applying CPU byte order.
	if addr >= VIDEO_CTRL && addr <= VIDEO_REG_END {
		base := addr &^ 0x3
		var raw [4]byte
		if chip.busMemory != nil && base+3 < uint32(len(chip.busMemory)) {
			copy(raw[:], chip.busMemory[base:base+4])
		}
		raw[addr-base] = value

		var assembled uint32
		if chip.bigEndianMode {
			assembled = binary.BigEndian.Uint32(raw[:])
		} else {
			assembled = binary.LittleEndian.Uint32(raw[:])
		}
		chip.handleWriteLocked(base, assembled)
	}
}

func (chip *VideoChip) handleWriteLocked(addr uint32, value uint32) {
	switch addr {
	case VIDEO_CTRL:
		if chip.stopped.Load() {
			return
		}
		wasEnabled := chip.enabled.Load()
		chip.enabled.Store(value != CTRL_DISABLE_FLAG)
		if !wasEnabled && chip.enabled.Load() {
			mode := VideoModes[chip.currentMode]
			if chip.onResolutionChange != nil {
				chip.onResolutionChange(mode.width, mode.height)
			} else {
				config := DisplayConfig{
					Width:       mode.width,
					Height:      mode.height,
					Scale:       DEFAULT_DISPLAY_SCALE,
					PixelFormat: PixelFormatRGBA,
					VSync:       VSYNC_ON,
				}
				if err := chip.output.SetDisplayConfig(config); err != nil {
					return
				}
				if err := chip.output.Start(); err != nil {
					return
				}
			}
		}
	case VIDEO_MODE:
		if mode, ok := VideoModes[value]; ok {
			chip.currentMode = value
			if len(chip.frontBuffer) != mode.totalSize {
				chip.frontBuffer = make([]byte, mode.totalSize)
				chip.backBuffer = make([]byte, mode.totalSize)
			}
			chip.initialiseDirtyGrid(mode)
			if chip.onResolutionChange != nil {
				chip.onResolutionChange(mode.width, mode.height)
			} else {
				config := DisplayConfig{
					Width:       mode.width,
					Height:      mode.height,
					Scale:       DEFAULT_DISPLAY_SCALE,
					PixelFormat: PixelFormatRGBA,
					VSync:       VSYNC_ON,
				}
				if err := chip.output.SetDisplayConfig(config); err != nil {
					return
				}
			}
		}
	case COPPER_CTRL:
		prevEnabled := chip.copperEnabled
		enable := value&copperCtrlEnable != 0
		reset := value&copperCtrlReset != 0
		if reset || (enable && !prevEnabled) {
			chip.copperPtr = chip.copperPtrStaged
			chip.copperPC = chip.copperPtr
			chip.copperWaiting = false
			chip.copperHalted = false
		}
		chip.copperEnabled = enable
		if !enable {
			chip.copperWaiting = false
		}
	case COPPER_PTR:
		chip.copperPtrStaged = value
	case COPPER_PTR + 1:
		chip.copperPtrStaged = writeUint32Byte(chip.copperPtrStaged, value, 1)
	case COPPER_PTR + 2:
		if value > 0xFF {
			chip.copperPtrStaged = writeUint32Byte(chip.copperPtrStaged, value, 2)
			chip.copperPtrStaged = writeUint32Byte(chip.copperPtrStaged, value>>8, 3)
		} else {
			chip.copperPtrStaged = writeUint32Byte(chip.copperPtrStaged, value, 2)
		}
	case COPPER_PTR + 3:
		chip.copperPtrStaged = writeUint32Byte(chip.copperPtrStaged, value, 3)
	case VIDEO_RASTER_Y:
		chip.rasterY = value
	case VIDEO_RASTER_Y + 1:
		chip.rasterY = writeUint32Byte(chip.rasterY, value, 1)
	case VIDEO_RASTER_Y + 2:
		chip.rasterY = writeUint32Word(chip.rasterY, value, 2)
	case VIDEO_RASTER_Y + 3:
		chip.rasterY = writeUint32Byte(chip.rasterY, value, 3)
	case VIDEO_RASTER_HEIGHT:
		chip.rasterHeight = value
	case VIDEO_RASTER_HEIGHT + 1:
		chip.rasterHeight = writeUint32Byte(chip.rasterHeight, value, 1)
	case VIDEO_RASTER_HEIGHT + 2:
		chip.rasterHeight = writeUint32Word(chip.rasterHeight, value, 2)
	case VIDEO_RASTER_HEIGHT + 3:
		chip.rasterHeight = writeUint32Byte(chip.rasterHeight, value, 3)
	case VIDEO_RASTER_COLOR:
		chip.rasterColor = value
	case VIDEO_RASTER_COLOR + 1:
		chip.rasterColor = writeUint32Byte(chip.rasterColor, value, 1)
	case VIDEO_RASTER_COLOR + 2:
		chip.rasterColor = writeUint32Word(chip.rasterColor, value, 2)
	case VIDEO_RASTER_COLOR + 3:
		chip.rasterColor = writeUint32Byte(chip.rasterColor, value, 3)
	case VIDEO_RASTER_CTRL:
		chip.rasterCtrl = value
		if value&rasterCtrlStart != 0 {
			chip.drawRasterBandLocked()
		}
	case VIDEO_PAL_INDEX:
		chip.palIndex = value & 0xFF
	case VIDEO_PAL_DATA:
		idx := uint8(chip.palIndex & 0xFF)
		chip.setPaletteEntry(idx, value)
		chip.palIndex = (chip.palIndex + 1) & 0xFF
	case VIDEO_COLOR_MODE:
		chip.clutMode.Store(value != 0)
		if value != 0 {
			// Lazily allocate CLUT8 conversion buffer
			mode := VideoModes[chip.currentMode]
			frameSize := mode.width * mode.height * BYTES_PER_PIXEL
			if len(chip.clutFrame) != frameSize {
				chip.clutFrame = make([]byte, frameSize)
			}
		}
	case VIDEO_FB_BASE:
		chip.fbBase = value
	default:
		// Handle palette table direct writes (0xF0088-0xF0487)
		if addr >= VIDEO_PAL_TABLE && addr <= VIDEO_PAL_END {
			entryOffset := addr - VIDEO_PAL_TABLE
			if entryOffset%4 == 0 {
				idx := uint8(entryOffset / 4)
				chip.setPaletteEntry(idx, value)
				// Mirror to busMemory for byte-write accumulation in HandleWrite8.
				// Use the appropriate byte order so HandleWrite8 can reconstruct correctly.
				if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
					if chip.bigEndianMode {
						binary.BigEndian.PutUint32(chip.busMemory[addr:addr+4], value)
					} else {
						binary.LittleEndian.PutUint32(chip.busMemory[addr:addr+4], value)
					}
				}
				return
			}
		}
		if chip.handleBlitterWriteLocked(addr, value) {
			return
		}
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			// directVRAM mode: bus.memory is the source of truth, let bus handle writes
			if chip.directVRAM != nil {
				if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
					binary.LittleEndian.PutUint32(chip.busMemory[addr:addr+4], value)
				}
				return
			}
			offset := addr - BUFFER_OFFSET
			// Write to frontBuffer if within display area
			if offset+BYTES_PER_PIXEL <= uint32(len(chip.frontBuffer)) && offset%BYTES_PER_PIXEL == BUFFER_REMAINDER {
				mode := VideoModes[chip.currentMode]
				// VRAM pixels are always stored as little-endian RGBA bytes.
				binary.LittleEndian.PutUint32(chip.frontBuffer[offset:], value)

				startPixel := offset / BYTES_PER_PIXEL
				startX := int(startPixel) % mode.width
				startY := int(startPixel) / mode.width
				chip.markRegionDirty(startX, startY)

				if !chip.resetting && !chip.hasContent.Load() {
					chip.hasContent.Store(true)
				}
			} else if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
				// For VRAM addresses beyond frontBuffer, write directly to bus memory
				// This enables double-buffering by rendering to VRAM offset > one frame
				binary.LittleEndian.PutUint32(chip.busMemory[addr:addr+4], value)
			}
		}
	}
}

func (chip *VideoChip) handleBlitterWriteLocked(addr uint32, value uint32) bool {
	switch addr {
	case BLT_CTRL:
		if !chip.blitterEnabled {
			return true
		}
		if value&bltCtrlIRQEnable != 0 {
			chip.bltIrqEnabled = true
		} else {
			chip.bltIrqEnabled = false
		}
		if value&bltCtrlStart == 0 {
			return true
		}
		if chip.bltBusy {
			return true
		}
		chip.bltBusy = true
		chip.bltErr = false
		chip.bltDone = false
		chip.bltOp = chip.bltOpStaged
		chip.bltSrc = chip.bltSrcStaged
		chip.bltDst = chip.bltDstStaged
		chip.bltWidth = chip.bltWidthStaged
		chip.bltHeight = chip.bltHeightStaged
		chip.bltSrcStrideRun = chip.bltSrcStride
		chip.bltDstStrideRun = chip.bltDstStride
		chip.bltColor = chip.bltColorStaged
		chip.bltMask = chip.bltMaskStaged
		chip.bltMode7U0 = chip.bltMode7U0Staged
		chip.bltMode7V0 = chip.bltMode7V0Staged
		chip.bltMode7DuCol = chip.bltMode7DuColStaged
		chip.bltMode7DvCol = chip.bltMode7DvColStaged
		chip.bltMode7DuRow = chip.bltMode7DuRowStaged
		chip.bltMode7DvRow = chip.bltMode7DvRowStaged
		chip.bltMode7TexW = chip.bltMode7TexWStaged
		chip.bltMode7TexH = chip.bltMode7TexHStaged
		chip.bltFlags = chip.bltFlagsStaged
		chip.bltFG = chip.bltFGStaged
		chip.bltBG = chip.bltBGStaged
		chip.bltMaskMod = chip.bltMaskModStaged
		chip.bltMaskSrcX = chip.bltMaskSrcXStaged
		chip.bltPending = true
		// Run blitter immediately (synchronous) so CPU doesn't wait for next frame
		mode := VideoModes[chip.currentMode]
		chip.runBlitterLocked(mode)
		return true
	case BLT_STATUS:
		if value&bltStatusIRQPending != 0 {
			chip.bltIrqPend = false
		}
		return true
	case BLT_OP:
		chip.bltOpStaged = value
		return true
	case BLT_OP + 1:
		chip.bltOpStaged = writeUint32Byte(chip.bltOpStaged, value, 1)
		return true
	case BLT_OP + 2:
		chip.bltOpStaged = writeUint32Word(chip.bltOpStaged, value, 2)
		return true
	case BLT_OP + 3:
		chip.bltOpStaged = writeUint32Byte(chip.bltOpStaged, value, 3)
		return true
	case BLT_SRC:
		chip.bltSrcStaged = value
		return true
	case BLT_SRC + 1:
		chip.bltSrcStaged = writeUint32Byte(chip.bltSrcStaged, value, 1)
		return true
	case BLT_SRC + 2:
		chip.bltSrcStaged = writeUint32Word(chip.bltSrcStaged, value, 2)
		return true
	case BLT_SRC + 3:
		chip.bltSrcStaged = writeUint32Byte(chip.bltSrcStaged, value, 3)
		return true
	case BLT_DST:
		chip.bltDstStaged = value
		return true
	case BLT_DST + 1:
		chip.bltDstStaged = writeUint32Byte(chip.bltDstStaged, value, 1)
		return true
	case BLT_DST + 2:
		chip.bltDstStaged = writeUint32Word(chip.bltDstStaged, value, 2)
		return true
	case BLT_DST + 3:
		chip.bltDstStaged = writeUint32Byte(chip.bltDstStaged, value, 3)
		return true
	case BLT_WIDTH:
		chip.bltWidthStaged = value
		return true
	case BLT_WIDTH + 1:
		chip.bltWidthStaged = writeUint32Byte(chip.bltWidthStaged, value, 1)
		return true
	case BLT_WIDTH + 2:
		chip.bltWidthStaged = writeUint32Word(chip.bltWidthStaged, value, 2)
		return true
	case BLT_WIDTH + 3:
		chip.bltWidthStaged = writeUint32Byte(chip.bltWidthStaged, value, 3)
		return true
	case BLT_HEIGHT:
		chip.bltHeightStaged = value
		return true
	case BLT_HEIGHT + 1:
		chip.bltHeightStaged = writeUint32Byte(chip.bltHeightStaged, value, 1)
		return true
	case BLT_HEIGHT + 2:
		chip.bltHeightStaged = writeUint32Word(chip.bltHeightStaged, value, 2)
		return true
	case BLT_HEIGHT + 3:
		chip.bltHeightStaged = writeUint32Byte(chip.bltHeightStaged, value, 3)
		return true
	case BLT_SRC_STRIDE:
		chip.bltSrcStride = value
		return true
	case BLT_SRC_STRIDE + 1:
		chip.bltSrcStride = writeUint32Byte(chip.bltSrcStride, value, 1)
		return true
	case BLT_SRC_STRIDE + 2:
		chip.bltSrcStride = writeUint32Word(chip.bltSrcStride, value, 2)
		return true
	case BLT_SRC_STRIDE + 3:
		chip.bltSrcStride = writeUint32Byte(chip.bltSrcStride, value, 3)
		return true
	case BLT_DST_STRIDE:
		chip.bltDstStride = value
		return true
	case BLT_DST_STRIDE + 1:
		chip.bltDstStride = writeUint32Byte(chip.bltDstStride, value, 1)
		return true
	case BLT_DST_STRIDE + 2:
		chip.bltDstStride = writeUint32Word(chip.bltDstStride, value, 2)
		return true
	case BLT_DST_STRIDE + 3:
		chip.bltDstStride = writeUint32Byte(chip.bltDstStride, value, 3)
		return true
	case BLT_COLOR:
		chip.bltColorStaged = value
		return true
	case BLT_COLOR + 1:
		chip.bltColorStaged = writeUint32Byte(chip.bltColorStaged, value, 1)
		return true
	case BLT_COLOR + 2:
		chip.bltColorStaged = writeUint32Word(chip.bltColorStaged, value, 2)
		return true
	case BLT_COLOR + 3:
		chip.bltColorStaged = writeUint32Byte(chip.bltColorStaged, value, 3)
		return true
	case BLT_MASK:
		chip.bltMaskStaged = value
		return true
	case BLT_MASK + 1:
		chip.bltMaskStaged = writeUint32Byte(chip.bltMaskStaged, value, 1)
		return true
	case BLT_MASK + 2:
		chip.bltMaskStaged = writeUint32Word(chip.bltMaskStaged, value, 2)
		return true
	case BLT_MASK + 3:
		chip.bltMaskStaged = writeUint32Byte(chip.bltMaskStaged, value, 3)
		return true
	case BLT_MODE7_U0:
		chip.bltMode7U0Staged = value
		return true
	case BLT_MODE7_U0 + 1:
		chip.bltMode7U0Staged = writeUint32Byte(chip.bltMode7U0Staged, value, 1)
		return true
	case BLT_MODE7_U0 + 2:
		chip.bltMode7U0Staged = writeUint32Word(chip.bltMode7U0Staged, value, 2)
		return true
	case BLT_MODE7_U0 + 3:
		chip.bltMode7U0Staged = writeUint32Byte(chip.bltMode7U0Staged, value, 3)
		return true
	case BLT_MODE7_V0:
		chip.bltMode7V0Staged = value
		return true
	case BLT_MODE7_V0 + 1:
		chip.bltMode7V0Staged = writeUint32Byte(chip.bltMode7V0Staged, value, 1)
		return true
	case BLT_MODE7_V0 + 2:
		chip.bltMode7V0Staged = writeUint32Word(chip.bltMode7V0Staged, value, 2)
		return true
	case BLT_MODE7_V0 + 3:
		chip.bltMode7V0Staged = writeUint32Byte(chip.bltMode7V0Staged, value, 3)
		return true
	case BLT_MODE7_DU_COL:
		chip.bltMode7DuColStaged = value
		return true
	case BLT_MODE7_DU_COL + 1:
		chip.bltMode7DuColStaged = writeUint32Byte(chip.bltMode7DuColStaged, value, 1)
		return true
	case BLT_MODE7_DU_COL + 2:
		chip.bltMode7DuColStaged = writeUint32Word(chip.bltMode7DuColStaged, value, 2)
		return true
	case BLT_MODE7_DU_COL + 3:
		chip.bltMode7DuColStaged = writeUint32Byte(chip.bltMode7DuColStaged, value, 3)
		return true
	case BLT_MODE7_DV_COL:
		chip.bltMode7DvColStaged = value
		return true
	case BLT_MODE7_DV_COL + 1:
		chip.bltMode7DvColStaged = writeUint32Byte(chip.bltMode7DvColStaged, value, 1)
		return true
	case BLT_MODE7_DV_COL + 2:
		chip.bltMode7DvColStaged = writeUint32Word(chip.bltMode7DvColStaged, value, 2)
		return true
	case BLT_MODE7_DV_COL + 3:
		chip.bltMode7DvColStaged = writeUint32Byte(chip.bltMode7DvColStaged, value, 3)
		return true
	case BLT_MODE7_DU_ROW:
		chip.bltMode7DuRowStaged = value
		return true
	case BLT_MODE7_DU_ROW + 1:
		chip.bltMode7DuRowStaged = writeUint32Byte(chip.bltMode7DuRowStaged, value, 1)
		return true
	case BLT_MODE7_DU_ROW + 2:
		chip.bltMode7DuRowStaged = writeUint32Word(chip.bltMode7DuRowStaged, value, 2)
		return true
	case BLT_MODE7_DU_ROW + 3:
		chip.bltMode7DuRowStaged = writeUint32Byte(chip.bltMode7DuRowStaged, value, 3)
		return true
	case BLT_MODE7_DV_ROW:
		chip.bltMode7DvRowStaged = value
		return true
	case BLT_MODE7_DV_ROW + 1:
		chip.bltMode7DvRowStaged = writeUint32Byte(chip.bltMode7DvRowStaged, value, 1)
		return true
	case BLT_MODE7_DV_ROW + 2:
		chip.bltMode7DvRowStaged = writeUint32Word(chip.bltMode7DvRowStaged, value, 2)
		return true
	case BLT_MODE7_DV_ROW + 3:
		chip.bltMode7DvRowStaged = writeUint32Byte(chip.bltMode7DvRowStaged, value, 3)
		return true
	case BLT_MODE7_TEX_W:
		chip.bltMode7TexWStaged = value
		return true
	case BLT_MODE7_TEX_W + 1:
		chip.bltMode7TexWStaged = writeUint32Byte(chip.bltMode7TexWStaged, value, 1)
		return true
	case BLT_MODE7_TEX_W + 2:
		chip.bltMode7TexWStaged = writeUint32Word(chip.bltMode7TexWStaged, value, 2)
		return true
	case BLT_MODE7_TEX_W + 3:
		chip.bltMode7TexWStaged = writeUint32Byte(chip.bltMode7TexWStaged, value, 3)
		return true
	case BLT_MODE7_TEX_H:
		chip.bltMode7TexHStaged = value
		return true
	case BLT_MODE7_TEX_H + 1:
		chip.bltMode7TexHStaged = writeUint32Byte(chip.bltMode7TexHStaged, value, 1)
		return true
	case BLT_MODE7_TEX_H + 2:
		chip.bltMode7TexHStaged = writeUint32Word(chip.bltMode7TexHStaged, value, 2)
		return true
	case BLT_MODE7_TEX_H + 3:
		chip.bltMode7TexHStaged = writeUint32Byte(chip.bltMode7TexHStaged, value, 3)
		return true
	case BLT_FLAGS:
		chip.bltFlagsStaged = value
		return true
	case BLT_FLAGS + 1:
		chip.bltFlagsStaged = writeUint32Byte(chip.bltFlagsStaged, value, 1)
		return true
	case BLT_FLAGS + 2:
		chip.bltFlagsStaged = writeUint32Word(chip.bltFlagsStaged, value, 2)
		return true
	case BLT_FLAGS + 3:
		chip.bltFlagsStaged = writeUint32Byte(chip.bltFlagsStaged, value, 3)
		return true
	case BLT_FG:
		chip.bltFGStaged = value
		return true
	case BLT_FG + 1:
		chip.bltFGStaged = writeUint32Byte(chip.bltFGStaged, value, 1)
		return true
	case BLT_FG + 2:
		chip.bltFGStaged = writeUint32Word(chip.bltFGStaged, value, 2)
		return true
	case BLT_FG + 3:
		chip.bltFGStaged = writeUint32Byte(chip.bltFGStaged, value, 3)
		return true
	case BLT_BG:
		chip.bltBGStaged = value
		return true
	case BLT_BG + 1:
		chip.bltBGStaged = writeUint32Byte(chip.bltBGStaged, value, 1)
		return true
	case BLT_BG + 2:
		chip.bltBGStaged = writeUint32Word(chip.bltBGStaged, value, 2)
		return true
	case BLT_BG + 3:
		chip.bltBGStaged = writeUint32Byte(chip.bltBGStaged, value, 3)
		return true
	case BLT_MASK_MOD:
		chip.bltMaskModStaged = value
		return true
	case BLT_MASK_MOD + 1:
		chip.bltMaskModStaged = writeUint32Byte(chip.bltMaskModStaged, value, 1)
		return true
	case BLT_MASK_MOD + 2:
		chip.bltMaskModStaged = writeUint32Word(chip.bltMaskModStaged, value, 2)
		return true
	case BLT_MASK_MOD + 3:
		chip.bltMaskModStaged = writeUint32Byte(chip.bltMaskModStaged, value, 3)
		return true
	case BLT_MASK_SRCX:
		chip.bltMaskSrcXStaged = value
		return true
	case BLT_MASK_SRCX + 1:
		chip.bltMaskSrcXStaged = writeUint32Byte(chip.bltMaskSrcXStaged, value, 1)
		return true
	case BLT_MASK_SRCX + 2:
		chip.bltMaskSrcXStaged = writeUint32Word(chip.bltMaskSrcXStaged, value, 2)
		return true
	case BLT_MASK_SRCX + 3:
		chip.bltMaskSrcXStaged = writeUint32Byte(chip.bltMaskSrcXStaged, value, 3)
		return true
	default:
		return false
	}
}

func (chip *VideoChip) drawRasterBandLocked() {
	mode := VideoModes[chip.currentMode]
	startY := int(chip.rasterY)
	height := int(chip.rasterHeight)
	if height <= 0 {
		height = 1
	}
	if startY < 0 || startY >= mode.height {
		return
	}
	endY := min(startY+height, mode.height)

	for y := startY; y < endY; y++ {
		if chip.clutMode.Load() {
			rowOffset := chip.fbBase + uint32(y*mode.width)
			for x := 0; x < mode.width; x++ {
				addr := rowOffset + uint32(x)
				if chip.busMemory != nil && addr < uint32(len(chip.busMemory)) {
					chip.busMemory[addr] = uint8(chip.rasterColor)
					continue
				}
				if chip.directVRAM != nil {
					off := addr - VRAM_START
					if off < uint32(len(chip.directVRAM)) {
						chip.directVRAM[off] = uint8(chip.rasterColor)
					}
				}
			}
		} else if chip.directVRAM != nil {
			rowOffset := uint32(y * mode.bytesPerRow)
			for x := 0; x < mode.width; x++ {
				offset := rowOffset + uint32(x*BYTES_PER_PIXEL)
				if offset+BYTES_PER_PIXEL > uint32(len(chip.directVRAM)) {
					break
				}
				binary.LittleEndian.PutUint32(chip.directVRAM[offset:], chip.rasterColor)
			}
		} else {
			rowOffset := uint32(y * mode.bytesPerRow)
			for x := 0; x < mode.width; x++ {
				offset := rowOffset + uint32(x*BYTES_PER_PIXEL)
				if offset+BYTES_PER_PIXEL > uint32(len(chip.frontBuffer)) {
					break
				}
				binary.LittleEndian.PutUint32(chip.frontBuffer[offset:], chip.rasterColor)
			}
		}
		chip.markRegionDirty(0, y)
	}
	if !chip.resetting && !chip.hasContent.Load() {
		chip.hasContent.Store(true)
	}
}

// GetFrontBuffer returns a direct reference to the front buffer for reading.
// This is useful for tests and debugging.
func (chip *VideoChip) GetFrontBuffer() []byte {
	return chip.frontBuffer
}

// RenderToFrontBuffer executes fn while holding chip.mu and providing front buffer access.
// The provided stride is bytes per row for the active mode.
func (chip *VideoChip) RenderToFrontBuffer(fn func(fb []byte, stride int)) {
	chip.mu.Lock()
	mode := VideoModes[chip.currentMode]
	fn(chip.frontBuffer, mode.bytesPerRow)
	chip.hasContent.Store(true)
	chip.mu.Unlock()
}

// MarkRectDirty marks all dirty tiles overlapped by the provided pixel rectangle.
func (chip *VideoChip) MarkRectDirty(x, y, w, h int) {
	tileW := int(chip.tileWidth)
	tileH := int(chip.tileHeight)
	if tileW <= 0 || tileH <= 0 || w <= 0 || h <= 0 {
		return
	}

	startTileX := x / tileW
	startTileY := y / tileH
	endTileX := (x + w - 1) / tileW
	endTileY := (y + h - 1) / tileH
	for ty := startTileY; ty <= endTileY; ty++ {
		for tx := startTileX; tx <= endTileX; tx++ {
			chip.markTileDirtyAtomic(tx*tileW, ty*tileH)
		}
	}
}

// GetOutput returns the video output interface for sharing with other video devices.
func (chip *VideoChip) GetOutput() VideoOutput {
	return chip.output
}

// -----------------------------------------------------------------------------
// VideoSource Interface Implementation
// -----------------------------------------------------------------------------

// GetFrame implements VideoSource - returns the current rendered frame
// Called by compositor each frame to collect video output
func (chip *VideoChip) GetFrame() []byte {
	chip.inVBlank.Store(false)
	if !chip.enabled.Load() {
		return nil
	}
	chip.mu.Lock()
	defer chip.mu.Unlock()

	var frame []byte
	if chip.clutMode.Load() || chip.directVRAM != nil || chip.fbBase != 0 {
		frame = chip.clutGetFrame()
	} else if !chip.hasContent.Load() {
		frame = chip.splashBuffer
	} else {
		frame = chip.frontBuffer
	}
	if frame == nil {
		return nil
	}
	out := make([]byte, len(frame))
	copy(out, frame)
	return out
}

// IsEnabled implements VideoSource - returns whether VideoChip is enabled
func (chip *VideoChip) IsEnabled() bool {
	return chip.enabled.Load()
}

// GetLayer implements VideoSource - returns Z-order for compositing
func (chip *VideoChip) GetLayer() int {
	return chip.layer
}

// GetDimensions implements VideoSource - returns frame dimensions
func (chip *VideoChip) GetDimensions() (int, int) {
	mode := VideoModes[chip.currentMode]
	return mode.width, mode.height
}

// TickFrame advances chip-clock state once per compositor frame.
func (chip *VideoChip) TickFrame() {
	chip.inVBlank.Store(true)
}

// SignalVSync implements VideoSource - called by compositor after frame sent.
func (chip *VideoChip) SignalVSync() {
	chip.everSignaled.Store(true)
	chip.inVBlank.Store(true)
}

// NeedsScanlineCompositing reports whether the compositor must own per-scanline
// timing for this frame. Normal framebuffer/blitter/raster rendering is already
// materialized in buffers and can use full-frame compositing.
func (chip *VideoChip) NeedsScanlineCompositing() bool {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	return chip.copperEnabled && chip.bus != nil
}

// -----------------------------------------------------------------------------
// ScanlineAware Interface Implementation
// -----------------------------------------------------------------------------

// StartFrame implements ScanlineAware - prepares for per-scanline copper execution
func (chip *VideoChip) StartFrame() {
	chip.mu.Lock()
	defer chip.mu.Unlock()

	chip.inVBlank.Store(false)
	chip.copperManagedByCompositor = true // Signal that compositor is managing copper

	if chip.copperEnabled && chip.bus != nil {
		chip.copperStartFrameLocked()
	}
}

// ProcessScanline implements ScanlineAware - advances copper to the given scanline
func (chip *VideoChip) ProcessScanline(y int) {
	chip.mu.Lock()
	defer chip.mu.Unlock()

	if !chip.copperEnabled || chip.bus == nil {
		return
	}

	mode := VideoModes[chip.currentMode]
	if y >= mode.height {
		return
	}

	// Advance copper to this scanline
	chip.copperAdvanceRasterLocked(y, 0)
	if chip.copperHalted {
		return
	}
	if chip.copperWaiting && chip.copperWaitY == uint16(y) && chip.copperWaitX < uint16(mode.width) {
		chip.copperAdvanceRasterLocked(y, int(chip.copperWaitX))
	}
}

// FinishFrame implements ScanlineAware - returns the rendered frame
func (chip *VideoChip) FinishFrame() []byte {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.inVBlank.Store(false)

	chip.copperManagedByCompositor = false // Release copper management back to refreshLoop

	var frame []byte
	if chip.clutMode.Load() || chip.directVRAM != nil || chip.fbBase != 0 {
		frame = chip.clutGetFrame()
	} else {
		frame = chip.frontBuffer
	}
	if frame == nil {
		return nil
	}
	out := make([]byte, len(frame))
	copy(out, frame)
	return out
}

func GetSplashImageData() ([]byte, error) {
	/*
		GetSplashImageData retrieves the splash image data from the embedded file system.

		 It reads the "splash.png" file and returns its contents as a byte slice.

		 Returns:
		   []byte - The raw PNG file data.
		   error  - An error if the file cannot be read.
	*/
	return splashData.ReadFile("splash.png")
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func makeRegionKey(x, y int) int {
	/* makeRegionKey computes a unique key for a given dirty region based on its (x, y) coordinates.

	This key is used for tracking modified screen regions. The x-coordinate is masked and the y-coordinate
	is shifted before being combined. If either coordinate is out of the acceptable bounds (0 to REGION_MAX_COORDINATE),
	the function returns INVALID_REGION.

	Parameters:
	  x - The x-coordinate (in region units).
	  y - The y-coordinate (in region units).

	Returns:
	  int - A unique key representing the dirty region, or INVALID_REGION if the coordinates are invalid.
	*/

	if x < 0 || x > REGION_MAX_COORDINATE || y < 0 || y > REGION_MAX_COORDINATE {
		return INVALID_REGION
	}
	return (y << REGION_Y_SHIFT) | (x & REGION_MASK)
}
