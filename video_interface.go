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
	// LockFullscreen forces fullscreen on and prevents runtime fullscreen toggles.
	LockFullscreen bool
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
	// UpdateFrame takes raw RGBA pixels only. The buffer length must equal
	// width*height*4 bytes for the current display config.
	UpdateFrame(buffer []byte) error

	// Timing and synchronization
	WaitForVSync() error
	GetFrameCount() uint64
	GetRefreshRate() int
}

type FrameDirtyRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

type RegionUpdatingOutput interface {
	UpdateRegion(x, y, width, height int, pixels []byte) error
}

type DirtyFrameSource interface {
	TakeDirtyRects() []FrameDirtyRect
}

type OpaqueFrameSource interface {
	IsOpaqueFrame() bool
}

// CompositorFrameCopySource lets the compositor request a stable frame copy
// directly into caller-provided storage. This avoids sources first copying into
// an internal snapshot only for the compositor to copy that snapshot again into
// a presentation lease.
type CompositorFrameCopySource interface {
	CopyFrameForCompositor(dst []byte) ([]byte, bool)
}

// FrameGenerationSource reports a counter that advances exactly when the
// source publishes new frame content. The compositor skips the
// collect/copy/blend/upload work for a tick when every enabled source
// implements this and no generation has advanced since the last
// composite; a source that cannot track this must not implement the
// interface (its presence disables the skip).
type FrameGenerationSource interface {
	FrameGeneration() uint64
}

// CompositorFrameLayer is one ordered native-resolution source layer in a
// complete compositor frame. The buffer is RGBA and must contain at least
// SourceWidth*SourceHeight*4 bytes.
type CompositorFrameLayer struct {
	SourceID     uint64
	SourceWidth  int
	SourceHeight int
	DestX        int
	DestY        int
	DestWidth    int
	DestHeight   int
	Buffer       []byte
	Lease        *VideoFrameLease
	Opaque       bool
	DirtyRects   []FrameDirtyRect
	// Indexed carries the layer as palette indices instead of RGBA. It is set
	// only when the frame is going to a backend that converts on the GPU, and
	// Buffer is then nil until something on the CPU side asks for pixels.
	Indexed *IndexedLayerData
}

// IndexedLayerData is a CLUT8 layer before palette expansion: one index byte
// per pixel plus the palette they index. The palette is a value, not a
// reference, because the source keeps mutating its own copy.
type IndexedLayerData struct {
	Indices []byte
	Palette [256]uint32
}

// ExpandInto writes the RGBA expansion of the layer into dst, which must hold
// at least four bytes per index. This is the CPU fallback, and it produces
// exactly what the CPU converter would have produced for the same frame.
func (d *IndexedLayerData) ExpandInto(dst []byte) bool {
	if d == nil || len(dst) < len(d.Indices)*BYTES_PER_PIXEL {
		return false
	}
	clut8ExpandSpanImpl(dst, d.Indices, &d.Palette)
	return true
}

// IndexedFrameSource is an optional source extension: a source that holds its
// frame as palette indices can hand those over instead of an expanded RGBA
// frame, letting the backend do the expansion on the GPU. Sources must copy,
// not alias, since the compositor reads the data after the call returns.
type IndexedFrameSource interface {
	// IndexedFrameForCompositor fills dst with one index byte per pixel and
	// returns the palette to expand them through. It reports false whenever
	// the source is not currently in an indexed mode.
	IndexedFrameForCompositor(dst []byte) (pal [256]uint32, ok bool)
}

// IndexedLayerOutput is an optional backend extension for outputs that can
// expand palette indices themselves, i.e. that have a shader to do it with.
type IndexedLayerOutput interface {
	AcceptsIndexedLayers() bool
}

// CompositorFrameUpdate describes a complete compositor frame for outputs that
// can perform final scaling/composition themselves.
type CompositorFrameUpdate struct {
	FrameID            uint64
	PresentationWidth  int
	PresentationHeight int
	HasContent         bool
	Layers             []CompositorFrameLayer
}

// HardwareCompositingOutput is an optional extension implemented by display
// backends that can perform compositor scaling/layering outside the CPU path.
type HardwareCompositingOutput interface {
	UpdateHardwareCompositorFrame(update CompositorFrameUpdate) error
	HardwareCompositorSnapshot(frameID uint64) ([]byte, bool)
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

// FrameTicker is an optional chip-clock hook invoked once per compositor frame,
// even when a source is disabled and produces no visible frame.
type FrameTicker interface {
	TickFrame()
}

// KeyboardInput is implemented by video outputs that can forward keyboard bytes.
type KeyboardInput interface {
	SetKeyHandler(func(byte))
}

// ScrollInput is implemented by video outputs that can forward scroll events.
type ScrollInput interface {
	SetScrollHandler(func(int))
}

// ClipboardInput is implemented by video outputs that support copy/cut/paste handlers.
type ClipboardInput interface {
	SetCopyHandler(func())
	SetCutHandler(func())
	SetMiddleMouseHandler(func())
}

// SystemCursorHider is implemented by video outputs that can hide the OS cursor.
type SystemCursorHider interface {
	HideSystemCursor()
}

// SoftwareCursorDisabler disables the emulator's software cursor overlay.
// Used by AROS which draws its own Intuition cursor in VRAM.
type SoftwareCursorDisabler interface {
	DisableSoftwareCursor()
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

// ScanlineBatchAware is an optional acceleration for sources whose scanline
// state can be advanced over a contiguous range without interleaving another
// source between scanlines.
type ScanlineBatchAware interface {
	ProcessScanlineRange(y0, y1 int)
}

// ScanlineCompositingSource optionally narrows ScanlineAware sources to the
// frames where compositor-owned scanline timing is required.
type ScanlineCompositingSource interface {
	NeedsScanlineCompositing() bool
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
