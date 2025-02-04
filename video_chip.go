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

func (chip *VideoChip) scaleImageToMode(imgData []byte, width, height int, mode VideoMode) []byte {
	if width == mode.width && height == mode.height {
		return imgData
	}

	scaled := make([]byte, mode.totalSize)
	scaleX := float64(width) / float64(mode.width)

	// Calculate vertical centering offset
	yOffset := (mode.height - height) / 2

	for y := 0; y < mode.height; y++ {
		srcY := y - yOffset

		// Skip rows outside source image bounds
		if srcY < 0 || srcY >= height {
			continue
		}

		for x := 0; x < mode.width; x++ {
			srcX := int(float64(x) * scaleX)
			if srcX >= width {
				continue
			}

			srcIdx := (srcY*width + srcX) * 4
			dstIdx := (y*mode.width + x) * 4
			copy(scaled[dstIdx:dstIdx+4], imgData[srcIdx:srcIdx+4])
		}
	}

	return scaled
}

func (chip *VideoChip) Start() {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	chip.enabled = true
	if chip.output != nil {
		err := chip.output.Start()
		if err != nil {
			return
		}
	}
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
	regionIdx := regionY*chip.dirtyRowStride + regionX

	if region, exists := chip.dirtyRegions[regionIdx]; !exists || region.lastUpdated != chip.frameCounter {
		chip.dirtyRegions[regionIdx] = DirtyRegion{
			x:           regionX * DIRTY_REGION_SIZE,
			y:           regionY * DIRTY_REGION_SIZE,
			width:       DIRTY_REGION_SIZE,
			height:      DIRTY_REGION_SIZE,
			lastUpdated: chip.frameCounter,
		}
	}
}

func (chip *VideoChip) refreshLoop() {
	for {
		select {
		case <-chip.done:
			return

		default:
			if !chip.enabled {
				time.Sleep(time.Millisecond * 16)
				continue
			}

			chip.mutex.Lock()
			if chip.hasContent {
				if len(chip.dirtyRegions) > 0 {
					copy(chip.backBuffer, chip.frontBuffer)
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
			time.Sleep(time.Millisecond)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		// Handle VRAM reads
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			offset := addr - VRAM_START
			return binary.LittleEndian.Uint32(chip.frontBuffer[offset : offset+4])
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
		// Handle VRAM writes
		if addr >= VRAM_START && addr < VRAM_START+VRAM_SIZE {
			offset := addr - VRAM_START
			mode := VideoModes[chip.currentMode]

			// Convert RGBA value and update buffers
			r := byte(value >> 24)
			g := byte(value >> 16)
			b := byte(value >> 8)
			a := byte(value)
			screenValue := uint32(r) | uint32(g)<<8 | uint32(b)<<16 | uint32(a)<<24
			binary.LittleEndian.PutUint32(chip.frontBuffer[offset:offset+4], screenValue)

			// Mark region as dirty
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
