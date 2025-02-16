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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
video_chip.go - Graphics Display Engine for the Intuition Platform

This module implements a complete video display system with:
- Multiple resolution modes (640x480, 800x600, 1024x768)
- Double-buffered RGBA framebuffer
- Dirty region tracking for efficient updates
- Splash screen support with bilinear scaling
- Memory-mapped register interface
- Hardware synchronization support

The architecture follows classic video chip design while adding
modern features like RGBA color and dirty region optimization.
Frame updates happen at 60Hz with all processing done in 32-bit
color space for maximum visual quality.

Signal flow:
1. Memory-mapped register writes
2. Dirty region tracking
3. Double buffer management
4. Frame synchronization
5. Display output

Thread Safety:
All parameter updates are protected by a mutex, allowing real-time
control from external threads while video processing continues.
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
	"time"
)

// -------------------- Memory and Address Constants --------------------
const (
	BYTES_PER_KB = 1024                        // Size of a kilobyte in bytes
	BYTES_PER_MB = BYTES_PER_KB * BYTES_PER_KB // Size of a megabyte in bytes

	VIDEO_REG_BASE = 0xF000 // Base address for memory-mapped registers
	// Register offsets for control, mode, and status
	VIDEO_REG_OFFSET_CTRL   = 0x000
	VIDEO_REG_OFFSET_MODE   = 0x004
	VIDEO_REG_OFFSET_STATUS = 0x008
	VIDEO_CTRL              = VIDEO_REG_BASE + VIDEO_REG_OFFSET_CTRL
	VIDEO_MODE              = VIDEO_REG_BASE + VIDEO_REG_OFFSET_MODE
	VIDEO_STATUS            = VIDEO_REG_BASE + VIDEO_REG_OFFSET_STATUS

	VRAM_START_MB = 1 // VRAM starts at 1MB offset
	VRAM_SIZE_MB  = 4 // 4MB of video memory
	VRAM_START    = VRAM_START_MB * BYTES_PER_MB
	VRAM_SIZE     = VRAM_SIZE_MB * BYTES_PER_MB
)

// ----------------------- Video Mode Constants -------------------------
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

// --------------------- Pixel/Color Constants --------------------------
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

// -------------------- Dirty Region Tracking ---------------------------
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

// --------------------- Timing/Refresh Constants -----------------------
const (
	REFRESH_RATE_HZ  = 60
	REFRESH_INTERVAL = time.Second / REFRESH_RATE_HZ
)

// -------------------- Buffer/Channel Constants ------------------------
const (
	BUFFER_OFFSET    = VRAM_START // Offset for VRAM access in HandleWrite
	BUFFER_REMAINDER = 0          // Required alignment remainder for pixel writes
)

// ------------------- Control States/Flags -----------------------------
const (
	CTRL_DISABLE_FLAG = 0 // Writing 0 to CTRL enables video
	ENABLED_STATE     = true
	DISABLED_STATE    = false
	VSYNC_ON          = true // Enable vertical synchronization in display config
)

// -------------------- Initial State Constants -------------------------
const (
	INITIAL_HAS_CONTENT = false
	INITIAL_MAP_SIZE    = 0
)

// --------------------- Image Processing -------------------------------
const (
	DRAW_SOURCE_OFFSET    = 0 // bounds.Min in draw.Draw
	DRAW_MODE_SOURCE      = draw.Src
	CENTER_OFFSET_DIVISOR = 2
	NEXT_PIXEL_OFFSET     = 1
)

// ----------------------- Error Messages -------------------------------
const (
	ERROR_FRAME_MSG  = "Error updating frame: %v\n"  // Shown when frame rendering fails
	ERROR_SPLASH_MSG = "Error updating splash: %v\n" // Shown when splash image fails to load
)

// ---------------------- Miscellaneous --------------------------------
const (
	DEFAULT_DISPLAY_SCALE = 1
	DEFAULT_RETURN        = 0
	ADDR_OFFSET           = VRAM_START
	FRAME_INCREMENT       = 1
)

// --------------------- Video Mode Configuration ----------------------
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
	   Optimized Memory Layout Analysis (64-bit system):

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

	// Status flags packed together (part of Cache Line 0)
	enabled    bool // 1 byte
	hasContent bool // 1 byte
	resetting  bool // 1 byte
	_padding   byte // 1 byte - Explicit padding for alignment

	// Synchronization (Cache Line 1)
	mutex sync.RWMutex // 8 bytes - Keep mutex at cache line boundary

	// Display interface (Cache Line 1-2)
	output VideoOutput // 8 bytes - Interface pointer

	// Communication channels (Cache Line 2)
	vsyncChan chan struct{} // 8 bytes
	done      chan struct{} // 8 bytes

	// Dirty region tracking (Cache Line 3)
	dirtyRegions map[int]DirtyRegion // 8 bytes

	// Fixed-size buffers (Cache Lines 4+)
	// Note: These will be converted to fixed arrays in next iteration
	frontBuffer  []byte // 24 bytes
	backBuffer   []byte // 24 bytes
	splashBuffer []byte // 24 bytes
	prevVRAM     []byte // 24 bytes
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
	output, err := NewVideoOutput(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to create video output: %w", err)
	}

	chip := &VideoChip{
		output:       output,
		currentMode:  MODE_640x480, // Using renamed constant
		vsyncChan:    make(chan struct{}),
		done:         make(chan struct{}),
		dirtyRegions: make(map[int]DirtyRegion),
		hasContent:   INITIAL_HAS_CONTENT,
		frameCounter: 0,
		prevVRAM:     make([]byte, VRAM_SIZE),
	}

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

			// Scale the splash image if needed
			chip.splashBuffer = chip.scaleImageToMode(chip.splashBuffer,
				bounds.Dx(), bounds.Dy(), mode)
		}
	}

	chip.initializeDirtyGrid(mode)
	go chip.refreshLoop()

	return chip, nil
}

func (chip *VideoChip) scaleImageToMode(imgData []byte, srcWidth, srcHeight int, mode VideoMode) []byte {
	// scaleImageToMode resizes an image to fit the current video mode using bilinear interpolation.
	// - imgData: Source image pixels in RGBA format
	// - srcWidth/srcHeight: Dimensions of the source image
	// - mode: Target video mode configuration
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
	chip.mutex.Lock()
	defer chip.mutex.Unlock()
	chip.enabled = ENABLED_STATE
	if chip.output != nil {
		return chip.output.Start()
	}
	return nil
}

func (chip *VideoChip) Stop() error {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()
	chip.enabled = DISABLED_STATE
	if chip.output != nil {
		return chip.output.Stop()
	}
	return nil
}

func (chip *VideoChip) initializeDirtyGrid(mode VideoMode) {
	chip.dirtyRowStride = int32((mode.width + DIRTY_REGION_SIZE - REGION_ADJUSTMENT) / DIRTY_REGION_SIZE)
	chip.dirtyColStride = int32((mode.height + DIRTY_REGION_SIZE - REGION_ADJUSTMENT) / DIRTY_REGION_SIZE)
	chip.dirtyRegions = make(map[int]DirtyRegion)
}

func (chip *VideoChip) markRegionDirty(x, y int) {
	// markRegionDirty marks a 32x32 pixel region as modified.
	// - x/y: Pixel coordinates that triggered the dirty region
	regionX := x / DIRTY_REGION_SIZE
	regionY := y / DIRTY_REGION_SIZE

	regionKey := makeRegionKey(regionX, regionY)
	if regionKey < INVALID_REGION {
		return
	}

	if region, exists := chip.dirtyRegions[regionKey]; !exists || region.lastUpdated != chip.frameCounter {
		chip.dirtyRegions[regionKey] = DirtyRegion{
			x:           regionX * DIRTY_REGION_SIZE,
			y:           regionY * DIRTY_REGION_SIZE,
			width:       DIRTY_REGION_SIZE,
			height:      DIRTY_REGION_SIZE,
			lastUpdated: chip.frameCounter,
		}
	}
}

func (chip *VideoChip) refreshLoop() {
	// refreshLoop handles periodic screen updates at the refresh rate.
	// - Copies dirty regions from front to back buffer
	// - Swaps buffers and sends the updated frame to the output
	ticker := time.NewTicker(REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-chip.done:
			return
		case <-ticker.C:
			if !chip.enabled {
				continue
			}

			chip.mutex.Lock()
			if chip.hasContent {
				if len(chip.dirtyRegions) > INITIAL_MAP_SIZE {
					mode := VideoModes[chip.currentMode]
					// Only copy dirty regions
					for _, region := range chip.dirtyRegions {
						for y := DIRTY_REGION_MIN; y < region.height; y++ {
							srcOffset := ((region.y + y) * mode.bytesPerRow) + (region.x * BYTES_PER_PIXEL)
							copyLen := region.width * BYTES_PER_PIXEL
							if srcOffset+copyLen <= len(chip.frontBuffer) {
								copy(chip.backBuffer[srcOffset:srcOffset+copyLen],
									chip.frontBuffer[srcOffset:srcOffset+copyLen])
							}
						}
					}
					chip.dirtyRegions = make(map[int]DirtyRegion)
					chip.frontBuffer, chip.backBuffer = chip.backBuffer, chip.frontBuffer
					chip.frameCounter += FRAME_INCREMENT
				}

				err := chip.output.UpdateFrame(chip.frontBuffer)
				if err != nil {
					fmt.Printf(ERROR_FRAME_MSG, err)
				}
			} else if chip.splashBuffer != nil {
				err := chip.output.UpdateFrame(chip.splashBuffer)
				if err != nil {
					fmt.Printf(ERROR_SPLASH_MSG, err)
				}
			}
			chip.mutex.Unlock()
		}
	}
}

func (chip *VideoChip) HandleRead(addr uint32) uint32 {
	chip.mutex.RLock()
	defer chip.mutex.RUnlock()

	switch addr {
	case VIDEO_CTRL:
		return btou32(chip.enabled)
	case VIDEO_MODE:
		return chip.currentMode
	case VIDEO_STATUS:
		return btou32(chip.hasContent)
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

func (chip *VideoChip) HandleWrite(addr uint32, value uint32) {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	switch addr {
	case VIDEO_CTRL:
		wasEnabled := chip.enabled
		chip.enabled = value != CTRL_DISABLE_FLAG
		if !wasEnabled && chip.enabled {
			mode := VideoModes[chip.currentMode]
			config := DisplayConfig{
				Width:       mode.width,
				Height:      mode.height,
				Scale:       DEFAULT_DISPLAY_SCALE,
				PixelFormat: PixelFormatRGBA,
				VSync:       VSYNC_ON,
			}
			err := chip.output.SetDisplayConfig(config)
			if err != nil {
				return
			}
			err = chip.output.Start()
			if err != nil {
				return
			}
		}

	case VIDEO_MODE:
		if mode, ok := VideoModes[value]; ok {
			chip.currentMode = value
			if len(chip.frontBuffer) != mode.totalSize {
				chip.frontBuffer = make([]byte, mode.totalSize)
				chip.backBuffer = make([]byte, mode.totalSize)
			}
			config := DisplayConfig{
				Width:       mode.width,
				Height:      mode.height,
				Scale:       DEFAULT_DISPLAY_SCALE,
				PixelFormat: PixelFormatRGBA,
				VSync:       VSYNC_ON,
			}
			err := chip.output.SetDisplayConfig(config)
			if err != nil {
				return
			}
			chip.initializeDirtyGrid(mode)
		}
	default:
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			offset := addr - BUFFER_OFFSET
			if offset+BYTES_PER_PIXEL > uint32(len(chip.frontBuffer)) || offset%BYTES_PER_PIXEL != BUFFER_REMAINDER {
				return
			}
			mode := VideoModes[chip.currentMode]
			binary.LittleEndian.PutUint32(chip.frontBuffer[offset:], value)

			startPixel := offset / BYTES_PER_PIXEL
			startX := int(startPixel) % mode.width
			startY := int(startPixel) / mode.width
			chip.markRegionDirty(startX, startY)

			if !chip.resetting && !chip.hasContent {
				chip.hasContent = ENABLED_STATE
			}
		}
	}
}

func GetSplashImageData() ([]byte, error) {
	return splashData.ReadFile("splash.png")
}

func makeRegionKey(x, y int) int {
	if x < 0 || x > REGION_MAX_COORDINATE || y < 0 || y > REGION_MAX_COORDINATE {
		return INVALID_REGION
	}
	return (y << REGION_Y_SHIFT) | (x & REGION_MASK)
}
