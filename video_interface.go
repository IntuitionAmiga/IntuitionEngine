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
	VIDEO_BACKEND_OPENGL        // OpenGL backend using cgo
)

// NewVideoOutput creates a new video output instance using the specified backend
func NewVideoOutput(backend int) (VideoOutput, error) {
	switch backend {
	case VIDEO_BACKEND_EBITEN:
		return NewEbitenOutput()
	case VIDEO_BACKEND_OPENGL:
		return nil, &VideoError{
			Operation: "backend creation",
			Details:   "OpenGL backend not yet implemented",
		}
	}
	return nil, &VideoError{
		Operation: "backend creation",
		Details:   fmt.Sprintf("unknown backend type: %d", backend),
	}
}
