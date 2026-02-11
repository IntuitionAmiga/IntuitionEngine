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
- Multiple resolution modes (640x480, 800x600, 1024x768)
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
	VIDEO_REG_OFFSET_CTRL           = 0x000
	VIDEO_REG_OFFSET_MODE           = 0x004
	VIDEO_REG_OFFSET_STATUS         = 0x008
	VIDEO_REG_OFFSET_COPPER_CTRL    = 0x00C
	VIDEO_REG_OFFSET_COPPER_PTR     = 0x010
	VIDEO_REG_OFFSET_COPPER_PC      = 0x014
	VIDEO_REG_OFFSET_COPPER_STATUS  = 0x018
	VIDEO_REG_OFFSET_BLT_CTRL       = 0x01C
	VIDEO_REG_OFFSET_BLT_OP         = 0x020
	VIDEO_REG_OFFSET_BLT_SRC        = 0x024
	VIDEO_REG_OFFSET_BLT_DST        = 0x028
	VIDEO_REG_OFFSET_BLT_WIDTH      = 0x02C
	VIDEO_REG_OFFSET_BLT_HEIGHT     = 0x030
	VIDEO_REG_OFFSET_BLT_SRC_STRIDE = 0x034
	VIDEO_REG_OFFSET_BLT_DST_STRIDE = 0x038
	VIDEO_REG_OFFSET_BLT_COLOR      = 0x03C
	VIDEO_REG_OFFSET_BLT_MASK       = 0x040
	VIDEO_REG_OFFSET_BLT_STATUS     = 0x044
	VIDEO_REG_OFFSET_RASTER_Y       = 0x048
	VIDEO_REG_OFFSET_RASTER_HEIGHT  = 0x04C
	VIDEO_REG_OFFSET_RASTER_COLOR   = 0x050
	VIDEO_REG_OFFSET_RASTER_CTRL    = 0x054
	VIDEO_CTRL                      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_CTRL
	VIDEO_MODE                      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_MODE
	VIDEO_STATUS                    = VIDEO_REG_BASE + VIDEO_REG_OFFSET_STATUS
	COPPER_CTRL                     = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_CTRL
	COPPER_PTR                      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_PTR
	COPPER_PC                       = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_PC
	COPPER_STATUS                   = VIDEO_REG_BASE + VIDEO_REG_OFFSET_COPPER_STATUS
	BLT_CTRL                        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_CTRL
	BLT_OP                          = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_OP
	BLT_SRC                         = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_SRC
	BLT_DST                         = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_DST
	BLT_WIDTH                       = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_WIDTH
	BLT_HEIGHT                      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_HEIGHT
	BLT_SRC_STRIDE                  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_SRC_STRIDE
	BLT_DST_STRIDE                  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_DST_STRIDE
	BLT_COLOR                       = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_COLOR
	BLT_MASK                        = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_MASK
	BLT_STATUS                      = VIDEO_REG_BASE + VIDEO_REG_OFFSET_BLT_STATUS
	VIDEO_RASTER_Y                  = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_Y
	VIDEO_RASTER_HEIGHT             = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_HEIGHT
	VIDEO_RASTER_COLOR              = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_COLOR
	VIDEO_RASTER_CTRL               = VIDEO_REG_BASE + VIDEO_REG_OFFSET_RASTER_CTRL
	VIDEO_REG_END                   = VIDEO_RASTER_CTRL + 3

	VRAM_START_MB = 1 // VRAM starts at 1MB offset
	VRAM_SIZE_MB  = 4 // 4MB of video memory
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
	bltCtrlStart = 1 << 0
	bltCtrlBusy  = 1 << 1
	bltCtrlIRQ   = 1 << 2
)

const (
	bltOpCopy = iota
	bltOpFill
	bltOpLine
	bltOpMaskedCopy
	bltOpAlphaCopy // Copy only pixels with alpha > 0 (for transparency)
)

const (
	bltStatusErr = 1 << 0
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

	RESOLUTION_640x480_WIDTH   = 640
	RESOLUTION_640x480_HEIGHT  = 480
	RESOLUTION_800x600_WIDTH   = 800
	RESOLUTION_800x600_HEIGHT  = 600
	RESOLUTION_1024x768_WIDTH  = 1024
	RESOLUTION_1024x768_HEIGHT = 768
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
}

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
	enabled         atomic.Bool // Lock-free enable status
	hasContent      atomic.Bool // Lock-free content flag
	inVBlank        atomic.Bool // Lock-free VBlank status for CPU polling
	resetting       bool        // 1 byte - still needs mutex for multi-field operations
	directMode      atomic.Bool // Lock-free direct VRAM mode flag
	fullScreenDirty atomic.Bool // Lock-free full-screen dirty flag

	// Synchronization (Cache Line 1)
	mu sync.Mutex // 8 bytes - Keep mutex at cache line boundary

	// Display interface (Cache Line 1-2)
	output             VideoOutput    // 8 bytes - Interface pointer
	onResolutionChange func(w, h int) // Optional resolution callback for compositor integration
	layer              int            // Z-order for compositor

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
	bltIrqEnabled   bool
	bltBusy         bool
	bltOpStaged     uint32
	bltSrcStaged    uint32
	bltDstStaged    uint32
	bltWidthStaged  uint32
	bltHeightStaged uint32
	bltSrcStride    uint32
	bltDstStride    uint32
	bltColorStaged  uint32
	bltMaskStaged   uint32
	bltOp           uint32
	bltSrc          uint32
	bltDst          uint32
	bltWidth        uint32
	bltHeight       uint32
	bltSrcStrideRun uint32
	bltDstStrideRun uint32
	bltColor        uint32
	bltMask         uint32
	bltPending      bool
	bltErr          bool

	rasterY      uint32
	rasterHeight uint32
	rasterColor  uint32
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
		output:       output,
		currentMode:  MODE_640x480, // Default video mode
		layer:        VIDEOCHIP_LAYER,
		vsyncChan:    make(chan struct{}),
		done:         make(chan struct{}),
		frameCounter: 0,
		prevVRAM:     make([]byte, VRAM_SIZE),
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

// SetBigEndianMode configures the video chip to read memory in big-endian format.
// This is required for M68K programs where data (copper lists, etc.) is stored
// in big-endian byte order.
func (chip *VideoChip) SetBigEndianMode(enabled bool) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.bigEndianMode = enabled
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
	if chip.output != nil {
		return chip.output.Stop()
	}
	return nil
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
						for bitIdx := 0; bitIdx < DIRTY_BITS_PER_WORD; bitIdx++ {
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
}

func (chip *VideoChip) executeBlitterLocked(mode VideoMode) {
	switch chip.bltOp {
	case bltOpFill:
		chip.blitFillLocked(mode)
	case bltOpLine:
		chip.blitLineLocked(mode)
	case bltOpMaskedCopy:
		chip.blitMaskedCopyLocked(mode)
	case bltOpAlphaCopy:
		chip.blitAlphaCopyLocked(mode)
	default:
		chip.blitCopyLocked(mode)
	}
}

func (chip *VideoChip) blitFillLocked(mode VideoMode) {
	width := int(chip.bltWidth)
	height := int(chip.bltHeight)
	if width <= 0 || height <= 0 {
		return
	}
	stride := chip.bltDstStrideRun
	if stride == 0 {
		stride = chip.defaultStride(chip.bltDst, width, mode)
	}

	rowAddr := chip.bltDst
	for y := 0; y < height; y++ {
		addr := rowAddr
		for x := 0; x < width; x++ {
			chip.blitWritePixelLocked(addr, chip.bltColor, mode)
			addr += BYTES_PER_PIXEL
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
	for y := 0; y < height; y++ {
		srcAddr := srcRow
		dstAddr := dstRow
		for x := 0; x < width; x++ {
			value := chip.blitReadPixelLocked(srcAddr)
			chip.blitWritePixelLocked(dstAddr, value, mode)
			srcAddr += BYTES_PER_PIXEL
			dstAddr += BYTES_PER_PIXEL
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
	for y := 0; y < height; y++ {
		srcAddr := srcRow
		dstAddr := dstRow
		maskAddr := maskRow
		for x := 0; x < width; x++ {
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
	for y := 0; y < height; y++ {
		srcAddr := srcRow
		dstAddr := dstRow
		for x := 0; x < width; x++ {
			value := chip.blitReadPixelLocked(srcAddr)
			// Only copy if alpha (lowest byte in our BGRA format) is non-zero
			alpha := value & 0xFF
			if alpha > 0 {
				chip.blitWritePixelLocked(dstAddr, value, mode)
			}
			srcAddr += BYTES_PER_PIXEL
			dstAddr += BYTES_PER_PIXEL
		}
		srcRow += srcStride
		dstRow += dstStride
	}
}

func (chip *VideoChip) blitLineLocked(mode VideoMode) {
	x0 := int(uint16(chip.bltSrc & 0xFFFF))
	y0 := int(uint16((chip.bltSrc >> 16) & 0xFFFF))
	x1 := int(uint16(chip.bltDst & 0xFFFF))
	y1 := int(uint16((chip.bltDst >> 16) & 0xFFFF))

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
		if x0 >= 0 && x0 < mode.width && y0 >= 0 && y0 < mode.height {
			addr := VRAM_START + uint32((y0*mode.width+x0)*BYTES_PER_PIXEL)
			chip.blitWritePixelLocked(addr, chip.bltColor, mode)
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
		offset := addr - BUFFER_OFFSET
		// Read from frontBuffer if within display area
		if offset+BYTES_PER_PIXEL <= uint32(len(chip.frontBuffer)) && offset%BYTES_PER_PIXEL == BUFFER_REMAINDER {
			// Direct pointer access for performance
			return *(*uint32)(unsafe.Pointer(&chip.frontBuffer[offset]))
		}
		// For VRAM addresses beyond frontBuffer, fall back to busMemory
		// This enables double-buffering by rendering to VRAM offset > one frame
		if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
			if chip.bigEndianMode {
				return binary.BigEndian.Uint32(chip.busMemory[addr : addr+4])
			}
			return *(*uint32)(unsafe.Pointer(&chip.busMemory[addr]))
		}
		return 0
	}
	// Read directly from cached bus memory to avoid mutex deadlock
	// Use big-endian byte order when bigEndianMode is set (for M68K programs)
	if chip.busMemory != nil && addr+4 <= uint32(len(chip.busMemory)) {
		if chip.bigEndianMode {
			return binary.BigEndian.Uint32(chip.busMemory[addr : addr+4])
		}
		return *(*uint32)(unsafe.Pointer(&chip.busMemory[addr]))
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
		offset := addr - BUFFER_OFFSET
		if offset+BYTES_PER_PIXEL > uint32(len(chip.frontBuffer)) || offset%BYTES_PER_PIXEL != BUFFER_REMAINDER {
			chip.bltErr = true
			return
		}
		// Direct pointer access for performance
		*(*uint32)(unsafe.Pointer(&chip.frontBuffer[offset])) = value
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
		// Calculate VBlank based on position within current frame
		// Use 50% threshold to give M68K a wider window to catch VBlank
		// - Active display (VBlank=false): first 50% of frame (~8.3ms)
		// - VBlank period (VBlank=true): last 50% of frame (~8.3ms)
		// This makes VBlank detection more reliable for M68K polling.
		// Optimized: direct int64 comparison avoids time.Unix() allocation
		frameStart := chip.lastFrameStart.Load()
		now := time.Now().UnixNano()
		vblankThresholdNs := int64(REFRESH_INTERVAL / 2)
		if now-frameStart >= vblankThresholdNs {
			status |= 2 // bit 1: in VBlank
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
		return 0
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
	default:
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			offset := addr - ADDR_OFFSET
			if offset+PIXEL_ALIGNMENT > uint32(len(chip.frontBuffer)) || (offset&PIXEL_ALIGN_MASK) != DEFAULT_RETURN {
				return DEFAULT_RETURN
			}
			return binary.LittleEndian.Uint32(chip.frontBuffer[offset:])
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
		ctrl |= bltCtrlIRQ
	}
	return ctrl
}

func (chip *VideoChip) blitterStatusLocked() uint32 {
	status := uint32(0)
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

func (chip *VideoChip) handleWriteLocked(addr uint32, value uint32) {
	switch addr {
	case VIDEO_CTRL:
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
		if value&rasterCtrlStart != 0 {
			chip.drawRasterBandLocked()
		}
	default:
		if chip.handleBlitterWriteLocked(addr, value) {
			return
		}
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			offset := addr - BUFFER_OFFSET
			// Write to frontBuffer if within display area
			if offset+BYTES_PER_PIXEL <= uint32(len(chip.frontBuffer)) && offset%BYTES_PER_PIXEL == BUFFER_REMAINDER {
				mode := VideoModes[chip.currentMode]
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
		if value&bltCtrlIRQ != 0 {
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
		chip.bltOp = chip.bltOpStaged
		chip.bltSrc = chip.bltSrcStaged
		chip.bltDst = chip.bltDstStaged
		chip.bltWidth = chip.bltWidthStaged
		chip.bltHeight = chip.bltHeightStaged
		chip.bltSrcStrideRun = chip.bltSrcStride
		chip.bltDstStrideRun = chip.bltDstStride
		chip.bltColor = chip.bltColorStaged
		chip.bltMask = chip.bltMaskStaged
		chip.bltPending = true
		// Run blitter immediately (synchronous) so CPU doesn't wait for next frame
		mode := VideoModes[chip.currentMode]
		chip.runBlitterLocked(mode)
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
	endY := startY + height
	if endY > mode.height {
		endY = mode.height
	}

	for y := startY; y < endY; y++ {
		rowOffset := uint32(y * mode.bytesPerRow)
		for x := 0; x < mode.width; x++ {
			offset := rowOffset + uint32(x*BYTES_PER_PIXEL)
			if offset+BYTES_PER_PIXEL > uint32(len(chip.frontBuffer)) {
				break
			}
			binary.LittleEndian.PutUint32(chip.frontBuffer[offset:], chip.rasterColor)
		}
		chip.markRegionDirty(0, y)
	}
	if !chip.resetting && !chip.hasContent.Load() {
		chip.hasContent.Store(true)
	}
}

// EnableDirectMode enables direct VRAM access mode and returns the framebuffer.
// In this mode, dirty region tracking is bypassed and the entire screen is
// refreshed each frame. This is optimal for fullscreen effects like plasma,
// fire, etc. where every pixel changes every frame.
//
// The returned buffer can be written to directly without mutex locks.
// Call MarkFullScreenDirty() after writing a frame to trigger refresh.
func (chip *VideoChip) EnableDirectMode() []byte {
	chip.mu.Lock()
	defer chip.mu.Unlock()

	chip.directMode.Store(true)
	chip.hasContent.Store(true)

	// Ensure buffer is allocated
	if chip.frontBuffer == nil {
		mode := VideoModes[chip.currentMode]
		chip.frontBuffer = make([]byte, mode.totalSize)
	}

	return chip.frontBuffer
}

// DisableDirectMode returns to normal dirty-region-tracked VRAM mode.
func (chip *VideoChip) DisableDirectMode() {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.directMode.Store(false)
}

// MarkFullScreenDirty signals that the entire framebuffer has been updated.
// Use this after writing a frame in direct mode to trigger display refresh.
func (chip *VideoChip) MarkFullScreenDirty() {
	chip.fullScreenDirty.Store(true)
}

// IsDirectMode returns true if direct VRAM mode is enabled.
func (chip *VideoChip) IsDirectMode() bool {
	return chip.directMode.Load()
}

// GetFrontBuffer returns a direct reference to the front buffer for reading.
// This is useful for tests and debugging.
func (chip *VideoChip) GetFrontBuffer() []byte {
	return chip.frontBuffer
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
	if !chip.enabled.Load() {
		return nil
	}
	// Return splash screen if no content has been written
	if !chip.hasContent.Load() {
		return chip.splashBuffer
	}
	return chip.frontBuffer
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

// SignalVSync implements VideoSource - called by compositor after frame sent
func (chip *VideoChip) SignalVSync() {
	chip.inVBlank.Store(true)
}

// -----------------------------------------------------------------------------
// ScanlineAware Interface Implementation
// -----------------------------------------------------------------------------

// StartFrame implements ScanlineAware - prepares for per-scanline copper execution
func (chip *VideoChip) StartFrame() {
	chip.mu.Lock()
	defer chip.mu.Unlock()

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

	chip.copperManagedByCompositor = false // Release copper management back to refreshLoop

	// Return the current front buffer
	return chip.frontBuffer
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
