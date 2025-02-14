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
License: GPLv3 or later
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
	"sync"
	"time"
)

//go:embed splash.png
var splashData embed.FS

func GetSplashImageData() ([]byte, error) {
	return splashData.ReadFile("splash.png")
}

const (
	VIDEO_CTRL   = 0xF000
	VIDEO_MODE   = 0xF004
	VIDEO_STATUS = 0xF008

	MODE_640x480  = 0x00
	MODE_800x600  = 0x01
	MODE_1024x768 = 0x02

	VRAM_START = 0x100000
	VRAM_SIZE  = 0x400000

	DIRTY_REGION_SIZE = 32
)

type VideoMode struct {
	width       int
	height      int
	bytesPerRow int
	totalSize   int
}

var VideoModes = map[uint32]VideoMode{
	MODE_640x480: {
		width:       640,
		height:      480,
		bytesPerRow: 640 * 4,
		totalSize:   640 * 480 * 4,
	},
	MODE_800x600: {
		width:       800,
		height:      600,
		bytesPerRow: 800 * 4,
		totalSize:   800 * 600 * 4,
	},
	MODE_1024x768: {
		width:       1024,
		height:      768,
		bytesPerRow: 1024 * 4,
		totalSize:   1024 * 768 * 4,
	},
}

type DirtyRegion struct {
	x, y, width, height int
	lastUpdated         uint64
}

type VideoChip struct {
	mutex          sync.RWMutex
	output         VideoOutput
	enabled        bool
	currentMode    uint32
	frontBuffer    []byte // Main display buffer
	backBuffer     []byte // Double buffer for smooth updates
	splashBuffer   []byte // Decoded RGBA splash image data
	vsyncChan      chan struct{}
	done           chan struct{}
	dirtyRegions   map[int]DirtyRegion
	frameCounter   uint64
	dirtyRowStride int
	dirtyColStride int
	hasContent     bool
	resetting      bool
	prevVRAM       []byte // Store previous video memory state
}

func NewVideoChip(backend int) (*VideoChip, error) {
	output, err := NewVideoOutput(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to create video output: %w", err)
	}

	chip := &VideoChip{
		output:       output,
		currentMode:  MODE_640x480,
		vsyncChan:    make(chan struct{}),
		done:         make(chan struct{}),
		dirtyRegions: make(map[int]DirtyRegion),
		hasContent:   false,
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
			draw.Draw(rgbaImg, bounds, img, bounds.Min, draw.Src)

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
	scaled := make([]byte, mode.totalSize)
	scaleX := float64(srcWidth) / float64(mode.width)
	yOffset := (mode.height - srcHeight) / 2

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
			x1, y1 := min(x0+1, srcWidth-1), min(y0+1, srcHeight-1)
			fx, fy := srcX-float64(x0), srcY-float64(y0)

			// Sample four corners
			dstIdx := (y*mode.width + x) * 4

			// Top-left
			idx00 := (y0*srcWidth + x0) * 4
			r00, g00, b00, a00 := imgData[idx00], imgData[idx00+1], imgData[idx00+2], imgData[idx00+3]

			// Top-right
			idx10 := (y0*srcWidth + x1) * 4
			r10, g10, b10, a10 := imgData[idx10], imgData[idx10+1], imgData[idx10+2], imgData[idx10+3]

			// Bottom-left
			idx01 := (y1*srcWidth + x0) * 4
			r01, g01, b01, a01 := imgData[idx01], imgData[idx01+1], imgData[idx01+2], imgData[idx01+3]

			// Bottom-right
			idx11 := (y1*srcWidth + x1) * 4
			r11, g11, b11, a11 := imgData[idx11], imgData[idx11+1], imgData[idx11+2], imgData[idx11+3]

			// Bilinear interpolation
			scaled[dstIdx] = byte(float64(r00)*(1-fx)*(1-fy) + float64(r10)*fx*(1-fy) +
				float64(r01)*(1-fx)*fy + float64(r11)*fx*fy)
			scaled[dstIdx+1] = byte(float64(g00)*(1-fx)*(1-fy) + float64(g10)*fx*(1-fy) +
				float64(g01)*(1-fx)*fy + float64(g11)*fx*fy)
			scaled[dstIdx+2] = byte(float64(b00)*(1-fx)*(1-fy) + float64(b10)*fx*(1-fy) +
				float64(b01)*(1-fx)*fy + float64(b11)*fx*fy)
			scaled[dstIdx+3] = byte(float64(a00)*(1-fx)*(1-fy) + float64(a10)*fx*(1-fy) +
				float64(a01)*(1-fx)*fy + float64(a11)*fx*fy)
		}
	}
	return scaled
}

func (chip *VideoChip) Start() error {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()
	chip.enabled = true
	if chip.output != nil {
		return chip.output.Start()
	}
	return nil
}

func (chip *VideoChip) Stop() {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	chip.enabled = false
	if chip.output != nil {
		err := chip.output.Stop()
		if err != nil {
			return
		}
	}
}

func (chip *VideoChip) initializeDirtyGrid(mode VideoMode) {
	chip.dirtyRowStride = (mode.width + DIRTY_REGION_SIZE - 1) / DIRTY_REGION_SIZE
	chip.dirtyColStride = (mode.height + DIRTY_REGION_SIZE - 1) / DIRTY_REGION_SIZE
	chip.dirtyRegions = make(map[int]DirtyRegion)
}

func (chip *VideoChip) markRegionDirty(x, y int) {
	regionX := x / DIRTY_REGION_SIZE
	regionY := y / DIRTY_REGION_SIZE
	regionKey := (regionY << 16) | regionX // Unique key using bit shifting

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
	ticker := time.NewTicker(time.Second / 60)
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
				if len(chip.dirtyRegions) > 0 {
					mode := VideoModes[chip.currentMode]
					// Only copy dirty regions
					for _, region := range chip.dirtyRegions {
						for y := 0; y < region.height; y++ {
							srcOffset := ((region.y + y) * mode.bytesPerRow) + (region.x * 4)
							copyLen := region.width * 4
							if srcOffset+copyLen <= len(chip.frontBuffer) {
								copy(chip.backBuffer[srcOffset:srcOffset+copyLen],
									chip.frontBuffer[srcOffset:srcOffset+copyLen])
							}
						}
					}
					chip.dirtyRegions = make(map[int]DirtyRegion)
					chip.frontBuffer, chip.backBuffer = chip.backBuffer, chip.frontBuffer
					chip.frameCounter++
				}

				err := chip.output.UpdateFrame(chip.frontBuffer)
				if err != nil {
					fmt.Printf("Error updating frame: %v\n", err)
				}
			} else if chip.splashBuffer != nil {
				err := chip.output.UpdateFrame(chip.splashBuffer)
				if err != nil {
					fmt.Printf("Error updating splash: %v\n", err)
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
			offset := addr - VRAM_START
			if offset+4 > uint32(len(chip.frontBuffer)) || offset%4 != 0 {
				return 0
			}
			return binary.LittleEndian.Uint32(chip.frontBuffer[offset:])
		}
	}
	return 0
}

func (chip *VideoChip) HandleWrite(addr uint32, value uint32) {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	switch addr {
	case VIDEO_CTRL:
		wasEnabled := chip.enabled
		chip.enabled = value != 0
		if !wasEnabled && chip.enabled {
			mode := VideoModes[chip.currentMode]
			config := DisplayConfig{
				Width:       mode.width,
				Height:      mode.height,
				Scale:       1,
				PixelFormat: PixelFormatRGBA,
				VSync:       true,
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
				Scale:       1,
				PixelFormat: PixelFormatRGBA,
				VSync:       true,
			}
			err := chip.output.SetDisplayConfig(config)
			if err != nil {
				return
			}
			chip.initializeDirtyGrid(mode)
		}
	default:
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			offset := addr - VRAM_START
			if offset+4 > uint32(len(chip.frontBuffer)) || offset%4 != 0 {
				return
			}
			mode := VideoModes[chip.currentMode]
			binary.LittleEndian.PutUint32(chip.frontBuffer[offset:], value)

			startPixel := offset / 4
			startX := int(startPixel) % mode.width
			startY := int(startPixel) / mode.width
			chip.markRegionDirty(startX, startY)

			if !chip.resetting && !chip.hasContent {
				chip.hasContent = true
			}
		}
	}
}
