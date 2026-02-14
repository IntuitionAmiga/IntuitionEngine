// video_interface.go - Video chip interface for Intuition Engine

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

package main

import (
	"fmt"
	"time"
)

// VideoError provides detailed error context for video operations
type VideoError struct {
	Operation string // What operation was being attempted
	Details   string // Additional error context
	Err       error  // Underlying error if any
}

func (e *VideoError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("video %s failed: %s: %v", e.Operation, e.Details, e.Err)
	}
	return fmt.Sprintf("video %s failed: %s", e.Operation, e.Details)
}

// FrameSnapshot encapsulates the data needed to represent a complete frame
type FrameSnapshot struct {
	Buffer    []byte   // Raw frame buffer data
	Palette   []uint32 // Color palette if applicable
	Width     int      // Frame width in pixels
	Height    int      // Frame height in pixels
	Format    PixelFormat
	Timestamp time.Time // When the snapshot was taken
}

// DisplayConfig contains hardware-independent configuration
type DisplayConfig struct {
	Width       int
	Height      int
	Scale       int // Integer scaling factor for output
	RefreshRate int // Target refresh rate in Hz
	PixelFormat PixelFormat
	VSync       bool // Whether to sync frame updates to display refresh
	Fullscreen  bool
}

func ClampScale(s int) int {
	if s < 1 {
		return 1
	}
	if s > 4 {
		return 4
	}
	return s
}

// VideoOutput defines the minimal interface that backends must implement
type VideoOutput interface {
	// Lifecycle management
	Start() error
	Stop() error
	Close() error
	IsStarted() bool

	// Core display operations - kept minimal
	SetDisplayConfig(config DisplayConfig) error
	GetDisplayConfig() DisplayConfig
	UpdateFrame(buffer []byte) error // Takes raw RGBA pixels only

	// Timing and synchronization
	WaitForVSync() error
	GetFrameCount() uint64
	GetRefreshRate() int
}

type PixelFormat int

const (
	PixelFormatRGBA PixelFormat = iota
	PixelFormatRGB565
	PixelFormatPaletted
)

// VideoSource represents a video device that can provide frames to the compositor.
// Both VideoChip and VideoVGA implement this interface.
type VideoSource interface {
	GetFrame() []byte          // Returns current rendered frame (nil if disabled)
	IsEnabled() bool           // Whether this source is active
	GetLayer() int             // Z-order for compositing (higher = on top)
	GetDimensions() (w, h int) // Returns the frame dimensions
	SignalVSync()              // Called by compositor after frame sent
}

// KeyboardInput is implemented by video outputs that can forward keyboard bytes.
type KeyboardInput interface {
	SetKeyHandler(func(byte))
}

// ScanlineAware is implemented by video sources that support per-scanline rendering.
// This enables copper-style raster effects where register changes take effect
// at specific scanline positions.
type ScanlineAware interface {
	// StartFrame prepares for per-scanline rendering
	StartFrame()
	// ProcessScanline advances internal state to the given scanline
	// For VideoChip: runs copper until it waits past this scanline
	// For VGA: renders this scanline with current register state
	ProcessScanline(y int)
	// FinishFrame completes the frame and returns the rendered result
	FinishFrame() []byte
}

// CompositorManageable is implemented by video sources with independent render
// goroutines. The compositor sets the flag during scanline-aware rendering to
// prevent the render goroutine from racing with the compositor's scanline path.
//
// Protocol: compositor calls SetCompositorManaged(true), then WaitRenderIdle()
// to ensure any in-flight render tick has finished before scanline rendering.
type CompositorManageable interface {
	SetCompositorManaged(managed bool)
	WaitRenderIdle()
}

// HardResettable is implemented by video outputs that support F10 hard reset.
type HardResettable interface {
	SetHardResetHandler(func())
}

// Optional interfaces for enhanced functionality
type PaletteCapable interface {
	UpdatePalette(colors []uint32) error
	GetPalette() []uint32
	SetPaletteEntry(index int, color uint32) error
}

type TextureCapable interface {
	CreateTexture(width, height int, format PixelFormat) (int, error)
	UpdateTexture(id int, data []byte) error
	DeleteTexture(id int) error
	GetTextureCount() int
}

type SpriteCapable interface {
	UpdateSprites(data []byte) error
	EnableSprites(enable bool)
	GetSpriteCount() int
	SetSpritePosition(index int, x, y int) error
}

// Predefined video backend types
const (
	VIDEO_BACKEND_EBITEN = iota // Pure Go Ebiten backend
)

// NewVideoOutput creates a new video output instance using the specified backend
func NewVideoOutput(backend int) (VideoOutput, error) {
	switch backend {
	case VIDEO_BACKEND_EBITEN:
		return NewEbitenOutput()
	}
	return nil, &VideoError{
		Operation: "backend creation",
		Details:   fmt.Sprintf("unknown backend type: %d", backend),
	}
}
