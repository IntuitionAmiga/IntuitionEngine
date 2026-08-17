//go:build !headless

// video_backend_ebiten.go - Ebiten video backend for IntuitionEngine

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
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/intuitionamiga/IntuitionEngine/internal/clipboard"
	"golang.org/x/image/font/basicfont"
)

func init() {
	compiledFeatures = append(compiledFeatures, "video:ebiten")
}

type EbitenOutput struct {
	running atomic.Bool
	// indexedUnsupported latches when the conversion shader fails on this
	// backend. It stops the compositor sending further indexed layers, so a
	// backend that cannot compile or run the shader degrades to the CPU
	// expansion permanently instead of failing every frame.
	indexedUnsupported atomic.Bool
	// statusBarImage caches the rendered overlay, rebuilt only when
	// statusBarKey changes. Touched from Draw only.
	statusBarImage *ebiten.Image
	statusBarKey   string
	// presentation is the reusable final image. Every selected route is
	// composed here before it is copied or filtered onto the display.
	presentation       *ebiten.Image
	compositionCapture *ebiten.Image
	// crtPresentationOverlay stages post-compositor host elements. Hardware
	// guest layers are filtered while scaling their native textures, whereas the
	// cursor and status bar must receive their own final-presentation CRT pass.
	crtPresentationOverlay *ebiten.Image
	crtMu                  sync.Mutex
	crtMode                crtPresentationMode
	presentationReset      atomic.Bool
	crtProfile             crtProfile
	crtState               crtFilterState
	crtFilter              *crtFilter
	firstFrameOnce         sync.Once
	window                 *ebiten.Image
	width                  int
	height                 int
	format                 PixelFormat
	fullscreen             bool
	lockFullscreen         bool
	scale                  int
	windowedW              int
	windowedH              int
	frameBuffer            []byte
	bufferMutex            sync.RWMutex
	frameCount             atomic.Uint64
	refreshRate            int
	vsyncChan              chan struct{}
	lifecycleMu            sync.Mutex
	done                   chan struct{}
	doneOnce               *sync.Once
	compositor             *VideoCompositor
	keyHandler             func(byte)
	scrollHandler          func(int)
	copyHandler            func()
	cutHandler             func()
	middleMouseHandler     func()
	wheelAccum             float64

	clipboardOnce sync.Once
	clipboardOK   bool
	showStatusBar bool

	hardResetHandler func()
	resetInProgress  atomic.Bool

	monitorOverlay   *MonitorOverlay
	luaOverlay       *LuaOverlay
	hostOverlay      *HostOverlay
	termMMIO         *TerminalMMIO
	gamepadPoll      func() // host gamepad poll, called once per frame; nil disables
	hideSystemCursor bool
	relativeMouse    relativeMouseCaptureState

	recorder           *VideoRecorder
	screenCaptureBuf   []byte
	presentationShot   *presentationScreenshotRequest
	compositionShot    *presentationScreenshotRequest
	presentationShotMu sync.Mutex
	hwFrameID          uint64
	hwPresentationW    int
	hwPresentationH    int
	hwHasContent       bool
	hwLayers           []ebitenHardwareLayer
	hwCopyShader       *ebiten.Shader
	// hwUploadCount counts hardware-layer WritePixels uploads, for the retained-
	// layer tests to prove an unchanged frame performs no upload.
	hwUploadCount atomic.Uint64

	// Software cursor overlay for guests that need host-side cursor rendering.
	// ROM desktops that draw into VRAM set noSoftwareCursor to avoid duplicates.
	cursorImage      *ebiten.Image
	noSoftwareCursor bool
}

type presentationScreenshotRequest struct {
	path string
	done chan error
}

type ebitenHardwareLayer struct {
	CompositorFrameLayer
	image *ebiten.Image
	// indices holds the staged palette indices for an indexed layer, and conv
	// the converter that expands them. Both are retained per slot so a steady
	// state neither allocates nor rebuilds textures.
	indices []byte
	palette [256]uint32
	conv    *clut8GPUConverter

	// Retained-layer state (Slice 8). uploadedSourceID/uploadedGen record what
	// the retained image currently holds, so an unchanged RGBA layer skips
	// WritePixels; geomKey caches the quad and uniform storage so geometry is
	// rebuilt only when the source or destination dimensions change.
	haveUpload       bool
	uploadedSourceID uint64
	uploadedGen      uint64
	geomValid        bool
	geomKey          ebitenLayerGeomKey
	cachedVertices   []ebiten.Vertex
	cachedOptions    *ebiten.DrawTrianglesShaderOptions
}

// ebitenLayerGeomKey is the geometry identity of a hardware layer's quad; the
// cached vertices and uniforms are valid while it is unchanged.
type ebitenLayerGeomKey struct {
	sw, sh, dx, dy, dw, dh int
	opaque                 bool
}

// retainedUploadSkippable reports whether the retained texture already holds
// this layer's exact content, so the whole-image WritePixels can be skipped.
func (l *ebitenHardwareLayer) retainedUploadSkippable(newImage, retained bool) bool {
	return retained && l.haveUpload && !newImage &&
		l.uploadedSourceID == l.SourceID &&
		l.uploadedGen == l.ContentGen
}

// geomReusable reports whether the cached quad and uniform storage are still
// valid for the given geometry identity.
func (l *ebitenHardwareLayer) geomReusable(retained bool, key ebitenLayerGeomKey) bool {
	return retained && l.geomValid && l.geomKey == key && l.cachedOptions != nil
}

// ebitenHWQuadIndices is the fixed two-triangle index list shared by every
// hardware layer quad. DrawTrianglesShader does not mutate it.
var ebitenHWQuadIndices = []uint16{0, 1, 2, 1, 3, 2}

// retainedLayersEnabled reports whether the hardware compositor retains unchanged
// layer textures and cached draw state. Default-on; IE_VIDEO_RETAINED_LAYERS=0
// restores the upload-and-rebuild-every-frame path. Read at use time.
func retainedLayersEnabled() bool {
	return os.Getenv("IE_VIDEO_RETAINED_LAYERS") != "0"
}

// AcceptsIndexedLayers reports that this backend can expand palette indices in
// a shader, so the compositor may hand it CLUT8 layers unexpanded. It follows
// the same switch as the rest of GPU conversion, so IE_VIDEO_GPU_CONVERT=0
// puts every layer back on the CPU expansion path.
func (eo *EbitenOutput) AcceptsIndexedLayers() bool {
	return videoGPUConvertRequested() && !eo.indexedUnsupported.Load()
}

const ebitenCompositorCopyShaderSrc = `//kage:unit pixels

package main

var SrcSize vec2
var RectSize vec2
var DestOrigin vec2
var Opaque float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	local := floor(dstPos.xy - imageDstOrigin() - DestOrigin)
	localX := local.x
	localY := local.y
	srcX := floor(localX * SrcSize.x / RectSize.x)
	srcY := floor(localY * SrcSize.y / RectSize.y)
	srcX = clamp(srcX, 0, SrcSize.x - 1)
	srcY = clamp(srcY, 0, SrcSize.y - 1)
	p := imageSrc0At(imageSrc0Origin() + vec2(srcX, srcY))
	// An opaque layer forces alpha here, exactly as the software kernel ORs
	// 0xFF000000 into every pixel it copies, so neither path needs the source
	// frame normalised beforehand. Such a layer never discards: it covers what
	// it draws over, and a fully zero pixel is opaque black on both paths.
	if Opaque != 0.0 {
		return vec4(p.r, p.g, p.b, 1.0)
	}
	if p.a == 0 && p.r == 0 && p.g == 0 && p.b == 0 {
		discard()
	}
	return p
}
`

func NewEbitenOutput() (VideoOutput, error) {
	eo := &EbitenOutput{
		width:         DefaultPresentationWidth,
		height:        DefaultPresentationHeight,
		format:        PixelFormatRGBA,
		scale:         1,
		windowedW:     DefaultPresentationWidth,
		windowedH:     DefaultPresentationHeight,
		frameBuffer:   make([]byte, DefaultPresentationWidth*DefaultPresentationHeight*4),
		refreshRate:   60,
		vsyncChan:     make(chan struct{}, 1),
		done:          make(chan struct{}),
		doneOnce:      &sync.Once{},
		showStatusBar: true,
		crtMode:       crtModeOff,
		crtProfile:    crtProfileGuestAdvanced,
	}
	// Browser build only: expose ieTypeText/ieKey so the demo page's text input
	// can drive the guest keyboard on touch devices. No-op on native.
	registerWasmInput(eo)
	return eo, nil
}

// crtIsRequested, setCRTRequested and toggleCRTRequested preserve IEScript's
// boolean CRT contract. F7 itself cycles the richer host presentation mode.
// Scripts run on a separate goroutine, so keep this control independent from
// the render thread's frame buffers.
func (eo *EbitenOutput) crtIsRequested() bool {
	eo.crtMu.Lock()
	defer eo.crtMu.Unlock()
	return eo.crtMode.enabled()
}

func (eo *EbitenOutput) setCRTRequested(enabled bool) {
	eo.crtMu.Lock()
	next := crtModeFromEnabled(enabled)
	changed := eo.crtMode != next
	eo.crtMode = next
	eo.crtMu.Unlock()
	if changed {
		eo.presentationReset.Store(true)
	}
}

func (eo *EbitenOutput) toggleCRTRequested() bool {
	eo.crtMu.Lock()
	next := crtModeOff
	if !eo.crtMode.enabled() {
		next = crtModeFlat
	}
	eo.crtMode = next
	eo.crtMu.Unlock()
	eo.presentationReset.Store(true)
	return next.enabled()
}

func (eo *EbitenOutput) crtModeRequested() string {
	return eo.crtPresentationMode().String()
}

func (eo *EbitenOutput) setCRTModeRequested(mode string) bool {
	next, ok := crtPresentationModeFromString(mode)
	if !ok {
		return false
	}
	eo.crtMu.Lock()
	changed := eo.crtMode != next
	eo.crtMode = next
	eo.crtMu.Unlock()
	if changed {
		eo.presentationReset.Store(true)
	}
	return true
}

func (eo *EbitenOutput) cycleCRTModeRequested() string {
	return eo.cycleCRTMode().String()
}

func (eo *EbitenOutput) cycleCRTMode() crtPresentationMode {
	eo.crtMu.Lock()
	eo.crtMode = eo.crtMode.next()
	next := eo.crtMode
	eo.crtMu.Unlock()
	eo.presentationReset.Store(true)
	return next
}

func (eo *EbitenOutput) crtPresentationMode() crtPresentationMode {
	eo.crtMu.Lock()
	defer eo.crtMu.Unlock()
	return eo.crtMode
}

func (eo *EbitenOutput) Start() error {
	if !eo.running.CompareAndSwap(false, true) {
		return nil
	}
	var err error
	defer func() {
		if err != nil {
			eo.running.Store(false)
			eo.lifecycleMu.Lock()
			done := eo.done
			once := eo.doneOnce
			eo.lifecycleMu.Unlock()
			closeVideoDoneOnce(done, once)
		}
	}()

	eo.lifecycleMu.Lock()
	done := make(chan struct{})
	once := &sync.Once{}
	eo.done = done
	eo.doneOnce = once
	eo.lifecycleMu.Unlock()

	eo.bufferMutex.RLock()
	windowedW := eo.windowedW
	windowedH := eo.windowedH
	fullscreen := eo.fullscreen
	hideSystemCursor := shouldHideSystemCursor(fullscreen, eo.hideSystemCursor)
	eo.bufferMutex.RUnlock()
	drainVSync(eo.vsyncChan)

	// macOS Cocoa requires NSApplication/NSWindow on the OS main thread, so
	// the Ebiten run loop must execute there. On !darwin (or headless darwin)
	// mainThreadCall is just `go fn()` — identical to the previous behaviour.
	mainThreadCall(func() {
		defer func() {
			eo.running.Store(false)
			closeVideoDoneOnce(done, once)
		}()
		ebiten.SetWindowSize(windowedW, windowedH)
		ebiten.SetWindowTitle("Intuition Engine (c) 2024 - 2026 Zayn Otley")
		ebiten.SetWindowResizable(true)
		ebiten.SetRunnableOnUnfocused(true)
		ebiten.SetVsyncEnabled(true)
		eo.applySystemCursorMode(hideSystemCursor)
		// Browsers only grant requestFullscreen inside a user gesture, so the
		// boot-time request is guaranteed to fail on js (the canvas fills the
		// page regardless). The runtime toggle still works there: it runs
		// inside a key event, which is a gesture.
		if fullscreen && runtime.GOOS != "js" {
			ebiten.SetFullscreen(true)
		}
		if err := ebiten.RunGame(eo); err != nil {
			fmt.Printf("Ebiten error: %v\n", err)
		}
	})

	return nil
}

func (eo *EbitenOutput) Stop() error {
	eo.running.Store(false)
	eo.lifecycleMu.Lock()
	done := eo.done
	once := eo.doneOnce
	eo.lifecycleMu.Unlock()
	closeVideoDoneOnce(done, once)
	return nil
}

func (eo *EbitenOutput) Close() error {
	return eo.Stop()
}

func (eo *EbitenOutput) Done() <-chan struct{} {
	eo.lifecycleMu.Lock()
	done := eo.done
	eo.lifecycleMu.Unlock()
	return done
}

func (eo *EbitenOutput) Clear(color uint32) error {
	eo.bufferMutex.Lock()
	for i := 0; i < len(eo.frameBuffer); i += 4 {
		eo.frameBuffer[i] = byte(color)
		eo.frameBuffer[i+1] = byte(color >> 8)
		eo.frameBuffer[i+2] = byte(color >> 16)
		eo.frameBuffer[i+3] = byte(color >> 24)
	}
	eo.bufferMutex.Unlock()
	return nil
}

func (eo *EbitenOutput) UpdateFrame(data []byte) error {
	eo.bufferMutex.Lock()
	if err := validateFrameSize(eo.width, eo.height, data); err != nil {
		eo.bufferMutex.Unlock()
		return err
	}
	copy(eo.frameBuffer, data)
	eo.clearHardwareCompositorLocked()
	eo.bufferMutex.Unlock()
	return nil
}

func (eo *EbitenOutput) UpdateHardwareCompositorFrame(update CompositorFrameUpdate) error {
	if update.PresentationWidth <= 0 || update.PresentationHeight <= 0 {
		return fmt.Errorf("invalid presentation dimensions %dx%d", update.PresentationWidth, update.PresentationHeight)
	}
	eo.bufferMutex.Lock()
	defer eo.bufferMutex.Unlock()
	if eo.hwCopyShader == nil {
		shader, err := ebiten.NewShader([]byte(ebitenCompositorCopyShaderSrc))
		if err != nil {
			return fmt.Errorf("compile compositor copy shader: %w", err)
		}
		eo.hwCopyShader = shader
	}
	if update.PresentationWidth != eo.width || update.PresentationHeight != eo.height {
		return fmt.Errorf("hardware frame dimensions %dx%d do not match output %dx%d", update.PresentationWidth, update.PresentationHeight, eo.width, eo.height)
	}
	if update.HasContent {
		for i, layer := range update.Layers {
			if err := validateHardwareLayer(update.PresentationWidth, update.PresentationHeight, layer); err != nil {
				return fmt.Errorf("layer %d: %w", i, err)
			}
		}
	}
	eo.hwFrameID = update.FrameID
	eo.hwPresentationW = update.PresentationWidth
	eo.hwPresentationH = update.PresentationHeight
	eo.hwHasContent = update.HasContent
	if !update.HasContent {
		for i := range eo.hwLayers {
			eo.releaseHardwareLayerLocked(i)
		}
		return nil
	}
	eo.resizeHardwareLayerSlotsLocked(len(update.Layers))
	for i, layer := range update.Layers {
		want := layer.SourceWidth * layer.SourceHeight * BYTES_PER_PIXEL
		oldBuf := eo.hwLayers[i].Buffer
		oldHadLease := eo.hwLayers[i].Lease != nil
		if layer.Lease != nil && !layer.Lease.Retain() {
			layer.Lease = nil
		}
		eo.releaseHardwareLayerLocked(i)
		if oldHadLease {
			oldBuf = nil
		}
		eo.hwLayers[i].CompositorFrameLayer = layer
		if layer.Indexed != nil {
			// The indices are the compositor's scratch and are overwritten on
			// the next frame, so they are staged like any other layer buffer.
			eo.hwLayers[i].indices = stageHardwareIndexBuffer(eo.hwLayers[i].indices, layer.Indexed.Indices)
			eo.hwLayers[i].palette = layer.Indexed.Palette
			eo.hwLayers[i].Buffer = nil
			continue
		}
		eo.hwLayers[i].indices = eo.hwLayers[i].indices[:0]
		if layer.Lease != nil {
			eo.hwLayers[i].Buffer = layer.Buffer[:want]
		} else {
			// An opaque layer has its alpha forced in the shader, so staging
			// only needs the bytes, not a pass to promote them.
			eo.hwLayers[i].Buffer = stageHardwareCompositorBuffer(oldBuf, layer.Buffer, want, layer.Opaque)
		}
	}
	for i := len(update.Layers); i < len(eo.hwLayers); i++ {
		eo.releaseHardwareLayerLocked(i)
	}
	return nil
}

// clut8ConvertLayer runs the layer's shader conversion. It is a variable so a
// test can make the conversion fail and exercise the CPU fallback below, which
// is otherwise only reachable on a backend that cannot run the shader.
var clut8ConvertLayer = func(layer *ebitenHardwareLayer) (*ebiten.Image, error) {
	return layer.conv.Convert(layer.indices, layer.SourceWidth, layer.SourceHeight, &layer.palette)
}

// expandIndexedLayerToRGBA expands a staged indexed layer into backend-owned
// RGBA storage, producing exactly what the CPU converter would have.
func expandIndexedLayerToRGBA(layer *ebitenHardwareLayer) []byte {
	pixels := layer.SourceWidth * layer.SourceHeight
	need := pixels * BYTES_PER_PIXEL
	dst := layer.Buffer
	if cap(dst) < need {
		dst = make([]byte, need)
	}
	dst = dst[:need]
	clut8ExpandSpanImpl(dst, layer.indices[:pixels], &layer.palette)
	return dst
}

// opaqueUniform maps the layer's opacity onto the shader uniform, which has no
// boolean type.
func opaqueUniform(opaque bool) float32 {
	if opaque {
		return 1
	}
	return 0
}

// stageHardwareIndexBuffer copies the layer's indices into backend-owned
// storage, reusing it across frames.
func stageHardwareIndexBuffer(dst, src []byte) []byte {
	if cap(dst) < len(src) {
		dst = make([]byte, len(src))
	}
	dst = dst[:len(src)]
	copy(dst, src)
	return dst
}

func stageHardwareCompositorBuffer(dst, src []byte, want int, opaque bool) []byte {
	if cap(dst) < want {
		dst = make([]byte, want)
	} else {
		dst = dst[:want]
	}
	copy(dst, src[:want])
	if opaque {
		return dst
	}
	for i := 0; i < want; i += BYTES_PER_PIXEL {
		if dst[i+3] == 0 && (dst[i] != 0 || dst[i+1] != 0 || dst[i+2] != 0) {
			dst[i+3] = 0xFF
		}
	}
	return dst
}

func validateHardwareLayer(dstW, dstH int, layer CompositorFrameLayer) error {
	if layer.SourceWidth <= 0 || layer.SourceHeight <= 0 || layer.DestWidth <= 0 || layer.DestHeight <= 0 {
		return fmt.Errorf("invalid dimensions")
	}
	if layer.DestX < 0 || layer.DestY < 0 || layer.DestWidth > dstW-layer.DestX || layer.DestHeight > dstH-layer.DestY {
		return fmt.Errorf("destination rect out of bounds")
	}
	if layer.Indexed != nil {
		// An indexed layer carries one byte per pixel and no RGBA buffer.
		if want := layer.SourceWidth * layer.SourceHeight; len(layer.Indexed.Indices) < want {
			return fmt.Errorf("index buffer too small: got %d, want at least %d", len(layer.Indexed.Indices), want)
		}
		return nil
	}
	want := layer.SourceWidth * layer.SourceHeight * BYTES_PER_PIXEL
	if len(layer.Buffer) < want {
		return fmt.Errorf("buffer too small: got %d, want at least %d", len(layer.Buffer), want)
	}
	return nil
}

func (eo *EbitenOutput) HardwareCompositorSnapshot(frameID uint64) ([]byte, bool) {
	return nil, false
}

func (eo *EbitenOutput) resizeHardwareLayerSlotsLocked(n int) {
	for len(eo.hwLayers) < n {
		eo.hwLayers = append(eo.hwLayers, ebitenHardwareLayer{})
	}
}

func (eo *EbitenOutput) clearHardwareCompositorLocked() {
	eo.hwFrameID = 0
	eo.hwPresentationW = 0
	eo.hwPresentationH = 0
	eo.hwHasContent = false
	for i := range eo.hwLayers {
		eo.releaseHardwareLayerLocked(i)
	}
}

func (eo *EbitenOutput) releaseHardwareLayerLocked(i int) {
	if i < 0 || i >= len(eo.hwLayers) {
		return
	}
	if lease := eo.hwLayers[i].Lease; lease != nil {
		lease.Release()
	}
	eo.hwLayers[i].CompositorFrameLayer = CompositorFrameLayer{}
	// Keep the backing array but drop the contents, so a released slot is
	// never mistaken for an indexed layer on a later frame.
	eo.hwLayers[i].indices = eo.hwLayers[i].indices[:0]
	// Invalidate the retained upload/geometry cache so a reused slot never
	// short-circuits an upload against a stale generation.
	eo.hwLayers[i].haveUpload = false
	eo.hwLayers[i].geomValid = false
}

func (eo *EbitenOutput) SetDisplayConfig(config DisplayConfig) error {
	eo.bufferMutex.Lock()
	defer eo.bufferMutex.Unlock()
	if config.LockFullscreen || eo.lockFullscreen {
		config.LockFullscreen = true
		config.Fullscreen = true
	}

	width := config.Width
	height := config.Height
	if width <= 0 {
		width = eo.width
	}
	if height <= 0 {
		height = eo.height
	}
	if width <= 0 {
		width = DefaultPresentationWidth
	}
	if height <= 0 {
		height = DefaultPresentationHeight
	}
	eo.width = width
	eo.height = height
	eo.format = config.PixelFormat
	eo.scale = ClampScale(config.Scale)
	newSize := eo.width * eo.height * 4

	if len(eo.frameBuffer) != newSize {
		eo.frameBuffer = make([]byte, newSize)
	}
	eo.clearHardwareCompositorLocked()

	eo.windowedW = eo.width * eo.scale
	eo.windowedH = eo.height * eo.scale
	eo.fullscreen = config.Fullscreen
	eo.lockFullscreen = config.LockFullscreen
	if eo.running.Load() {
		ebiten.SetFullscreen(eo.fullscreen)
		if !eo.fullscreen {
			ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
		}
		eo.applySystemCursorMode(shouldHideSystemCursor(eo.fullscreen, eo.hideSystemCursor))
	}
	if eo.window != nil {
		eo.window.Dispose()
		eo.window = nil
	}
	for i := range eo.hwLayers {
		if eo.hwLayers[i].image != nil {
			eo.hwLayers[i].image.Dispose()
			eo.hwLayers[i].image = nil
		}
	}
	return nil
}

func (eo *EbitenOutput) GetDisplayConfig() DisplayConfig {
	eo.bufferMutex.RLock()
	defer eo.bufferMutex.RUnlock()
	return DisplayConfig{
		Width:          eo.width,
		Height:         eo.height,
		Scale:          eo.scale,
		PixelFormat:    eo.format,
		RefreshRate:    eo.refreshRate,
		VSync:          true,
		Fullscreen:     eo.fullscreen,
		LockFullscreen: eo.lockFullscreen,
	}
}

func (eo *EbitenOutput) WaitForVSync() error {
	eo.lifecycleMu.Lock()
	done := eo.done
	eo.lifecycleMu.Unlock()
	select {
	case <-eo.vsyncChan:
		return nil
	case <-done:
		return fmt.Errorf("EbitenOutput: stopped")
	}
}

func (eo *EbitenOutput) GetFrameCount() uint64 {
	return eo.frameCount.Load()
}

func (eo *EbitenOutput) GetRefreshRate() int {
	return eo.refreshRate
}

func (eo *EbitenOutput) GetSnapshot() (FrameSnapshot, error) {
	eo.bufferMutex.RLock()
	defer eo.bufferMutex.RUnlock()

	snapshot := FrameSnapshot{
		Buffer:    make([]byte, len(eo.frameBuffer)),
		Width:     eo.width,
		Height:    eo.height,
		Format:    eo.format,
		Timestamp: time.Now(),
	}
	copy(snapshot.Buffer, eo.frameBuffer)
	return snapshot, nil
}

func (eo *EbitenOutput) IsStarted() bool {
	return eo.running.Load()
}

func (eo *EbitenOutput) SupportsPalette() bool {
	return false
}

func (eo *EbitenOutput) SupportsTextures() bool {
	return false
}

func (eo *EbitenOutput) SupportsSprites() bool {
	return false
}

func (eo *EbitenOutput) UpdateRegion(x, y, width, height int, pixels []byte) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("region dimensions out of bounds")
	}
	if x < 0 || y < 0 {
		return fmt.Errorf("region coordinates out of bounds")
	}
	eo.bufferMutex.Lock()
	defer eo.bufferMutex.Unlock()
	if x > eo.width || y > eo.height || width > eo.width-x || height > eo.height-y {
		return fmt.Errorf("region coordinates out of bounds")
	}
	want := width * height * 4
	if len(pixels) < want {
		return fmt.Errorf("region pixel buffer too small: got %d bytes, want at least %d", len(pixels), want)
	}

	for dy := range height {
		dstOffset := ((y+dy)*eo.width + x) * 4
		srcOffset := dy * width * 4
		copy(eo.frameBuffer[dstOffset:], pixels[srcOffset:srcOffset+width*4])
	}
	return nil
}

func (eo *EbitenOutput) Update() error {
	// Check if the window was closed using Ebiten's built-in detection
	if ebiten.IsWindowBeingClosed() {
		if activeCPU != nil {
			activeCPU.Stop()
		}
		return ebiten.Termination
	}

	// Normal update path when window is open
	if !eo.running.Load() {
		return ebiten.Termination
	}

	// F9: Machine Monitor toggle
	if inpututil.IsKeyJustPressed(ebiten.KeyF9) {
		if eo.monitorOverlay != nil {
			mon := eo.monitorOverlay.monitor
			if mon.IsActive() {
				mon.Deactivate()
			} else {
				mon.Activate()
			}
		}
	}
	// F8: Lua REPL toggle (monitor has priority when active)
	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		monitorActive := eo.monitorOverlay != nil && eo.monitorOverlay.monitor.IsActive()
		if !monitorActive && eo.luaOverlay != nil {
			eo.luaOverlay.Toggle()
		}
	}
	if decideEbitenF7Action(inpututil.IsKeyJustPressed(ebiten.KeyF7)) {
		eo.cycleCRTMode()
		eo.bufferMutex.RLock()
		compositor := eo.compositor
		eo.bufferMutex.RUnlock()
		if compositor != nil {
			compositor.RequestFullComposite()
		}
	}

	// F10: Hard reset - must be checked before the monitor input
	// intercept so reset works even when the monitor is active.
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
		if eo.resetInProgress.CompareAndSwap(false, true) {
			eo.bufferMutex.RLock()
			handler := eo.hardResetHandler
			eo.bufferMutex.RUnlock()
			if handler != nil {
				go func() {
					defer eo.resetInProgress.Store(false)
					handler()
				}()
			} else {
				eo.resetInProgress.Store(false)
			}
		}
	}

	eo.updateRelativeMouseBeforeOverlay()

	// Poll the gamepad every frame, before overlay routing can early-return.
	// The guest keeps reading gamepad MMIO while an overlay is open, so
	// releases, disconnects, and axis changes must not go stale.
	if eo.gamepadPoll != nil {
		eo.gamepadPoll()
	}

	// When monitor is active, route all input to the overlay
	if eo.monitorOverlay != nil && eo.monitorOverlay.monitor.IsActive() {
		eo.monitorOverlay.HandleInput()
		return nil
	}
	// Lua REPL has next priority after monitor.
	if eo.luaOverlay != nil && eo.luaOverlay.IsActive() {
		eo.luaOverlay.HandleInput()
		return nil
	}
	if eo.hostOverlay != nil && eo.hostOverlay.IsActive() {
		eo.hostOverlay.HandleInput()
		return nil
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		shiftPressed := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
		eo.bufferMutex.RLock()
		compositor := eo.compositor
		lockFullscreen := eo.lockFullscreen
		scaleToggleAvailable := compositor != nil && compositor.ActiveSourceNeedsScaleToggle()
		eo.bufferMutex.RUnlock()
		switch decideEbitenF11Action(true, shiftPressed, lockFullscreen, scaleToggleAvailable) {
		case ebitenF11ActionToggleScale:
			if compositor != nil {
				compositor.ToggleScaleModeIfNonNative()
			}
		case ebitenF11ActionToggleFullscreen:
			eo.bufferMutex.Lock()
			if !eo.lockFullscreen {
				eo.fullscreen = !eo.fullscreen
				ebiten.SetFullscreen(eo.fullscreen)
				if !eo.fullscreen {
					ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
				}
				eo.applySystemCursorMode(shouldHideSystemCursor(eo.fullscreen, eo.hideSystemCursor))
			}
			eo.bufferMutex.Unlock()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF12) {
		eo.bufferMutex.Lock()
		eo.showStatusBar = !eo.showStatusBar
		eo.bufferMutex.Unlock()
	}
	eo.handleKeyboardInput()
	eo.updateTerminalMMIOInput()
	return nil
}

// SetGamepadPoll installs the host gamepad poll invoked once per Update frame.
func (eo *EbitenOutput) SetGamepadPoll(poll func()) {
	eo.bufferMutex.Lock()
	eo.gamepadPoll = poll
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetMonitorOverlay(overlay *MonitorOverlay) {
	eo.bufferMutex.Lock()
	eo.monitorOverlay = overlay
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetLuaOverlay(overlay *LuaOverlay) {
	eo.bufferMutex.Lock()
	eo.luaOverlay = overlay
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetHostOverlay(overlay *HostOverlay) {
	eo.bufferMutex.Lock()
	eo.hostOverlay = overlay
	eo.bufferMutex.Unlock()
}

// AttachMonitor creates a MonitorOverlay and attaches it.
// Implements MonitorAttachable interface.
func (eo *EbitenOutput) AttachMonitor(monitor *MachineMonitor) {
	overlay := NewMonitorOverlay(monitor)
	eo.SetMonitorOverlay(overlay)
	if eo.luaOverlay == nil {
		eo.luaOverlay = NewLuaOverlay(nil)
	}
}

func (eo *EbitenOutput) SetScriptEngine(scriptEngine *ScriptEngine) {
	if eo.luaOverlay == nil {
		eo.luaOverlay = NewLuaOverlay(scriptEngine)
	} else {
		eo.luaOverlay.SetScriptEngine(scriptEngine)
	}
	if scriptEngine != nil {
		scriptEngine.SetLuaOverlay(eo.luaOverlay)
		eo.recorder = scriptEngine.recorder
	}
}

func (eo *EbitenOutput) SetTerminalMMIO(tm *TerminalMMIO) {
	eo.bufferMutex.Lock()
	eo.termMMIO = tm
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetVideoCompositor(compositor *VideoCompositor) {
	eo.bufferMutex.Lock()
	eo.compositor = compositor
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetHardResetHandler(fn func()) {
	eo.bufferMutex.Lock()
	eo.hardResetHandler = fn
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetKeyHandler(fn func(byte)) {
	eo.bufferMutex.Lock()
	eo.keyHandler = fn
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetScrollHandler(fn func(int)) {
	eo.bufferMutex.Lock()
	eo.scrollHandler = fn
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetCopyHandler(fn func()) {
	eo.bufferMutex.Lock()
	eo.copyHandler = fn
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetCutHandler(fn func()) {
	eo.bufferMutex.Lock()
	eo.cutHandler = fn
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) SetMiddleMouseHandler(fn func()) {
	eo.bufferMutex.Lock()
	eo.middleMouseHandler = fn
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) HideSystemCursor() {
	eo.bufferMutex.Lock()
	eo.hideSystemCursor = true
	running := eo.running.Load()
	noSoftwareCursor := eo.noSoftwareCursor
	if !noSoftwareCursor && eo.cursorImage == nil {
		eo.initSoftwareCursorLocked()
	}
	eo.bufferMutex.Unlock()
	if running {
		eo.applySystemCursorMode(true)
	}
}

func (eo *EbitenOutput) applySystemCursorMode(hidden bool) {
	if eo.relativeMouse.captured {
		ebiten.SetCursorMode(ebiten.CursorModeCaptured)
		return
	}
	if hidden {
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
		return
	}
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
}

func (eo *EbitenOutput) DisableSoftwareCursor() {
	eo.bufferMutex.Lock()
	defer eo.bufferMutex.Unlock()
	eo.noSoftwareCursor = true
}

// initSoftwareCursor creates a classic Amiga-style arrow cursor image.
func (eo *EbitenOutput) initSoftwareCursor() {
	eo.bufferMutex.Lock()
	defer eo.bufferMutex.Unlock()
	eo.initSoftwareCursorLocked()
}

func (eo *EbitenOutput) initSoftwareCursorLocked() {
	// 16x16 Amiga-style arrow cursor: 1=black outline, 2=white fill, 3=orange highlight
	const curW, curH = 16, 16
	cursor := [curH][curW]byte{
		{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 2, 2, 2, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0},
		{1, 2, 2, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 2, 1, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 0, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 0, 0, 0, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 1, 2, 2, 1, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	pixels := make([]byte, curW*curH*4)
	for y := range curH {
		for x := range curW {
			off := (y*curW + x) * 4
			switch cursor[y][x] {
			case 0: // transparent
				pixels[off+3] = 0
			case 1: // black outline
				pixels[off+0] = 0
				pixels[off+1] = 0
				pixels[off+2] = 0
				pixels[off+3] = 255
			case 2: // white fill
				pixels[off+0] = 255
				pixels[off+1] = 255
				pixels[off+2] = 255
				pixels[off+3] = 255
			}
		}
	}
	img := ebiten.NewImage(curW, curH)
	img.WritePixels(pixels)
	eo.cursorImage = img
}

func (eo *EbitenOutput) emitByte(b byte) {
	eo.bufferMutex.RLock()
	handler := eo.keyHandler
	eo.bufferMutex.RUnlock()
	if handler != nil {
		handler(b)
	}
}

func (eo *EbitenOutput) emitSeq(seq []byte) {
	for _, b := range seq {
		eo.emitByte(b)
	}
}

const (
	keyRepeatDelay    = 24 // ticks (~400ms at 60TPS)
	keyRepeatInterval = 2  // ticks (~33ms)
)

type ebitenF11Action int

const (
	ebitenF11ActionNone ebitenF11Action = iota
	ebitenF11ActionToggleScale
	ebitenF11ActionToggleFullscreen
)

func decideEbitenF11Action(f11JustPressed, shiftPressed, lockFullscreen, scaleToggleAvailable bool) ebitenF11Action {
	if !f11JustPressed {
		return ebitenF11ActionNone
	}
	if shiftPressed {
		if lockFullscreen {
			return ebitenF11ActionNone
		}
		return ebitenF11ActionToggleFullscreen
	}
	if scaleToggleAvailable {
		return ebitenF11ActionToggleScale
	}
	return ebitenF11ActionNone
}

// decideEbitenF7Action is kept separate from input forwarding: F7 toggles the
// host CRT filter and still reaches the guest through handleKeyboardInput.
func decideEbitenF7Action(f7JustPressed bool) bool {
	return f7JustPressed
}

var ebitenToSTScancode = map[ebiten.Key]uint8{
	ebiten.KeyEscape:       0x01,
	ebiten.Key1:            0x02,
	ebiten.Key2:            0x03,
	ebiten.Key3:            0x04,
	ebiten.Key4:            0x05,
	ebiten.Key5:            0x06,
	ebiten.Key6:            0x07,
	ebiten.Key7:            0x08,
	ebiten.Key8:            0x09,
	ebiten.Key9:            0x0A,
	ebiten.Key0:            0x0B,
	ebiten.KeyMinus:        0x0C,
	ebiten.KeyEqual:        0x0D,
	ebiten.KeyBackspace:    0x0E,
	ebiten.KeyTab:          0x0F,
	ebiten.KeyQ:            0x10,
	ebiten.KeyW:            0x11,
	ebiten.KeyE:            0x12,
	ebiten.KeyR:            0x13,
	ebiten.KeyT:            0x14,
	ebiten.KeyY:            0x15,
	ebiten.KeyU:            0x16,
	ebiten.KeyI:            0x17,
	ebiten.KeyO:            0x18,
	ebiten.KeyP:            0x19,
	ebiten.KeyBracketLeft:  0x1A,
	ebiten.KeyBracketRight: 0x1B,
	ebiten.KeyEnter:        0x1C,
	ebiten.KeyControlLeft:  0x1D,
	ebiten.KeyA:            0x1E,
	ebiten.KeyS:            0x1F,
	ebiten.KeyD:            0x20,
	ebiten.KeyF:            0x21,
	ebiten.KeyG:            0x22,
	ebiten.KeyH:            0x23,
	ebiten.KeyJ:            0x24,
	ebiten.KeyK:            0x25,
	ebiten.KeyL:            0x26,
	ebiten.KeySemicolon:    0x27,
	ebiten.KeyApostrophe:   0x28,
	ebiten.KeyBackquote:    0x29,
	ebiten.KeyShiftLeft:    0x2A,
	ebiten.KeyBackslash:    0x2B,
	ebiten.KeyZ:            0x2C,
	ebiten.KeyX:            0x2D,
	ebiten.KeyC:            0x2E,
	ebiten.KeyV:            0x2F,
	ebiten.KeyB:            0x30,
	ebiten.KeyN:            0x31,
	ebiten.KeyM:            0x32,
	ebiten.KeyComma:        0x33,
	ebiten.KeyPeriod:       0x34,
	ebiten.KeySlash:        0x35,
	ebiten.KeyShiftRight:   0x36,
	ebiten.KeySpace:        0x39,
	ebiten.KeyCapsLock:     0x3A,
	ebiten.KeyF1:           0x3B,
	ebiten.KeyF2:           0x3C,
	ebiten.KeyF3:           0x3D,
	ebiten.KeyF4:           0x3E,
	ebiten.KeyF5:           0x3F,
	ebiten.KeyF6:           0x40,
	ebiten.KeyF7:           0x41,
	ebiten.KeyF8:           0x42,
	ebiten.KeyF9:           0x43,
	ebiten.KeyF10:          0x44,
	ebiten.KeyF11:          0x57,
	ebiten.KeyF12:          0x58,
	ebiten.KeyArrowUp:      0x48,
	ebiten.KeyArrowLeft:    0x4B,
	ebiten.KeyArrowRight:   0x4D,
	ebiten.KeyArrowDown:    0x50,
}

// ebitenToAmigaRawkey maps Ebiten keys to Amiga rawkey codes.
// Used when TerminalMMIO.amigaScancodeMode is set (AROS boot).
var ebitenToAmigaRawkey = map[ebiten.Key]uint8{
	ebiten.KeyBackquote:    0x00,
	ebiten.Key1:            0x01,
	ebiten.Key2:            0x02,
	ebiten.Key3:            0x03,
	ebiten.Key4:            0x04,
	ebiten.Key5:            0x05,
	ebiten.Key6:            0x06,
	ebiten.Key7:            0x07,
	ebiten.Key8:            0x08,
	ebiten.Key9:            0x09,
	ebiten.Key0:            0x0A,
	ebiten.KeyMinus:        0x0B,
	ebiten.KeyEqual:        0x0C,
	ebiten.KeyBackslash:    0x0D,
	ebiten.KeyQ:            0x10,
	ebiten.KeyW:            0x11,
	ebiten.KeyE:            0x12,
	ebiten.KeyR:            0x13,
	ebiten.KeyT:            0x14,
	ebiten.KeyY:            0x15,
	ebiten.KeyU:            0x16,
	ebiten.KeyI:            0x17,
	ebiten.KeyO:            0x18,
	ebiten.KeyP:            0x19,
	ebiten.KeyBracketLeft:  0x1A,
	ebiten.KeyBracketRight: 0x1B,
	ebiten.KeyA:            0x20,
	ebiten.KeyS:            0x21,
	ebiten.KeyD:            0x22,
	ebiten.KeyF:            0x23,
	ebiten.KeyG:            0x24,
	ebiten.KeyH:            0x25,
	ebiten.KeyJ:            0x26,
	ebiten.KeyK:            0x27,
	ebiten.KeyL:            0x28,
	ebiten.KeySemicolon:    0x29,
	ebiten.KeyApostrophe:   0x2A,
	ebiten.KeyZ:            0x31,
	ebiten.KeyX:            0x32,
	ebiten.KeyC:            0x33,
	ebiten.KeyV:            0x34,
	ebiten.KeyB:            0x35,
	ebiten.KeyN:            0x36,
	ebiten.KeyM:            0x37,
	ebiten.KeyComma:        0x38,
	ebiten.KeyPeriod:       0x39,
	ebiten.KeySlash:        0x3A,
	ebiten.KeySpace:        0x40,
	ebiten.KeyBackspace:    0x41,
	ebiten.KeyTab:          0x42,
	ebiten.KeyEnter:        0x44,
	ebiten.KeyEscape:       0x45,
	ebiten.KeyDelete:       0x46,
	ebiten.KeyArrowUp:      0x4C,
	ebiten.KeyArrowDown:    0x4D,
	ebiten.KeyArrowRight:   0x4E,
	ebiten.KeyArrowLeft:    0x4F,
	ebiten.KeyF1:           0x50,
	ebiten.KeyF2:           0x51,
	ebiten.KeyF3:           0x52,
	ebiten.KeyF4:           0x53,
	ebiten.KeyF5:           0x54,
	ebiten.KeyF6:           0x55,
	ebiten.KeyF7:           0x56,
	ebiten.KeyF8:           0x57,
	ebiten.KeyF9:           0x58,
	ebiten.KeyF10:          0x59,
	ebiten.KeyShiftLeft:    0x60,
	ebiten.KeyShiftRight:   0x61,
	ebiten.KeyCapsLock:     0x62,
	ebiten.KeyControlLeft:  0x63,
	ebiten.KeyAltLeft:      0x64,
	ebiten.KeyAltRight:     0x65,
	ebiten.KeyMetaLeft:     0x66,
	ebiten.KeyMetaRight:    0x67,
}

func shouldRepeat(key ebiten.Key) bool {
	dur := inpututil.KeyPressDuration(key)
	if dur < keyRepeatDelay {
		return false
	}
	return (dur-keyRepeatDelay)%keyRepeatInterval == 0
}

func relativeMouseReleaseRequested() bool {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	alt := ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
	if !ctrl || !alt {
		return false
	}
	return inpututil.IsKeyJustPressed(ebiten.KeyControlLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyControlRight) ||
		inpututil.IsKeyJustPressed(ebiten.KeyAltLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyAltRight)
}

func (eo *EbitenOutput) applyRelativeMouseOutput(tm *TerminalMMIO, out relativeMouseCaptureOutput, fullscreen, hideSystemCursor bool) {
	if out.clearDeltas {
		tm.ClearMouseDeltas()
	}
	if out.addDX != 0 || out.addDY != 0 {
		tm.AddMouseDelta(out.addDX, out.addDY)
	}
	switch out.cursorAction {
	case relativeMouseCursorCapture:
		ebiten.SetCursorMode(ebiten.CursorModeCaptured)
	case relativeMouseCursorVisible:
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	case relativeMouseCursorRestorePolicy:
		eo.applySystemCursorMode(shouldHideSystemCursor(fullscreen, hideSystemCursor))
	}
}

func (eo *EbitenOutput) updateRelativeMouseBeforeOverlay() {
	eo.bufferMutex.RLock()
	tm := eo.termMMIO
	fullscreen := eo.fullscreen
	hideSystemCursor := eo.hideSystemCursor
	eo.bufferMutex.RUnlock()
	if tm == nil {
		return
	}
	relativeMode := tm.MouseRelativeMode()
	releaseRequested := relativeMouseReleaseRequested()
	if relativeMode && !releaseRequested {
		return
	}
	if !relativeMode && !eo.relativeMouse.active && !eo.relativeMouse.captured && !eo.relativeMouse.hostReleased {
		return
	}
	mx, my := ebiten.CursorPosition()
	out := eo.relativeMouse.Update(relativeMouseCaptureInput{
		guestRelative:    relativeMode,
		mouseOverride:    tm.mouseOverride.Load(),
		hostX:            mx,
		hostY:            my,
		releaseRequested: releaseRequested,
	})
	eo.applyRelativeMouseOutput(tm, out, fullscreen, hideSystemCursor)
}

func (eo *EbitenOutput) updateTerminalMMIOInput() {
	eo.bufferMutex.RLock()
	tm := eo.termMMIO
	compositor := eo.compositor
	width := eo.width
	height := eo.height
	fullscreen := eo.fullscreen
	hideSystemCursor := eo.hideSystemCursor
	eo.bufferMutex.RUnlock()
	if tm == nil {
		return
	}

	relativeMode := tm.MouseRelativeMode()
	mx, my := ebiten.CursorPosition()
	mouseOverride := tm.mouseOverride.Load()
	relativeOut := eo.relativeMouse.Update(relativeMouseCaptureInput{
		guestRelative:      relativeMode,
		mouseOverride:      mouseOverride,
		hostX:              mx,
		hostY:              my,
		releaseRequested:   relativeMouseReleaseRequested(),
		recaptureRequested: inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
	})
	eo.applyRelativeMouseOutput(tm, relativeOut, fullscreen, hideSystemCursor)

	if !mouseOverride {
		if !relativeMode {
			// Scale from display space to the guest cursor coordinate space.
			mx, my, width, height = mapPresentationMouseToGuest(mx, my, width, height, tm, compositor)

			newX := int32(max(0, min(mx, width-1)))
			newY := int32(max(0, min(my, height-1)))

			oldX := tm.mouseX.Swap(newX)
			oldY := tm.mouseY.Swap(newY)
			if oldX != newX || oldY != newY {
				tm.mouseChanged.Store(true)
			}
		}

		if relativeOut.suppressButtons {
			if oldButtons := tm.mouseButtons.Swap(0); oldButtons != 0 {
				tm.mouseChanged.Store(true)
			}
		} else {
			var buttons uint32
			if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
				buttons |= 1
			}
			if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
				buttons |= 2
			}
			if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
				buttons |= 4
			}

			oldButtons := tm.mouseButtons.Swap(buttons)
			if oldButtons != buttons {
				tm.mouseChanged.Store(true)
			}
		}
	}

	scancodeMap := ebitenToSTScancode
	if tm.amigaScancodeMode.Load() {
		scancodeMap = ebitenToAmigaRawkey
	}
	for ebitenKey, code := range scancodeMap {
		if inpututil.IsKeyJustPressed(ebitenKey) {
			tm.EnqueueScancode(code)
		}
		if inpututil.IsKeyJustReleased(ebitenKey) {
			tm.EnqueueScancode(code | 0x80)
		}
	}

	var mods uint32
	if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		mods |= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) {
		mods |= 2
	}
	if ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight) {
		mods |= 4
	}
	if ebiten.IsKeyPressed(ebiten.KeyCapsLock) {
		mods |= 8
	}
	tm.modifiers.Store(mods)
}

func (eo *EbitenOutput) handleKeyboardInput() {
	eo.bufferMutex.RLock()
	hasHandler := eo.keyHandler != nil
	eo.bufferMutex.RUnlock()
	if !hasHandler {
		return
	}

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)

	// Clipboard/selection: Ctrl+Shift+V/C/X (only when monitor is NOT active)
	monitorActive := eo.monitorOverlay != nil && eo.monitorOverlay.monitor.IsActive()
	if ctrl && shift && !monitorActive {
		if inpututil.IsKeyJustPressed(ebiten.KeyV) {
			eo.handleClipboardPaste()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyC) {
			eo.bufferMutex.RLock()
			handler := eo.copyHandler
			eo.bufferMutex.RUnlock()
			if handler != nil {
				handler()
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyX) {
			eo.bufferMutex.RLock()
			handler := eo.cutHandler
			eo.bufferMutex.RUnlock()
			if handler != nil {
				handler()
			}
		}
	}

	// Ctrl shortcuts (without shift): emit control bytes
	ctrlHandled := false
	if ctrl && !shift {
		type ctrlBind struct {
			key  ebiten.Key
			code byte
		}
		ctrlBinds := []ctrlBind{
			{ebiten.KeyA, 0x01}, // Home
			{ebiten.KeyE, 0x05}, // End
			{ebiten.KeyK, 0x0B}, // Kill to EOL
			{ebiten.KeyU, 0x15}, // Kill to BOL
			{ebiten.KeyL, 0x0C}, // Clear screen
		}
		for _, cb := range ctrlBinds {
			if inpututil.IsKeyJustPressed(cb.key) {
				eo.emitByte(cb.code)
				ctrlHandled = true
			}
		}
		// Ctrl+Arrow: emit CSI modifier sequences
		ctrlArrows := []struct {
			key ebiten.Key
			seq []byte
		}{
			{ebiten.KeyArrowLeft, []byte{0x1B, '[', '1', ';', '5', 'D'}},
			{ebiten.KeyArrowRight, []byte{0x1B, '[', '1', ';', '5', 'C'}},
			{ebiten.KeyArrowUp, []byte{0x1B, '[', '1', ';', '5', 'A'}},
			{ebiten.KeyArrowDown, []byte{0x1B, '[', '1', ';', '5', 'B'}},
		}
		for _, ca := range ctrlArrows {
			if inpututil.IsKeyJustPressed(ca.key) || shouldRepeat(ca.key) {
				eo.emitSeq(ca.seq)
				ctrlHandled = true
			}
		}
	}

	// Shift+Arrow/Home/End: emit CSI modifier sequences for selection.
	// When Ctrl is also held, only Home/End trigger selection (arrows stay as Ctrl+Arrow word-move).
	shiftHandled := false
	if shift && !monitorActive {
		shiftKeys := []struct {
			key ebiten.Key
			seq []byte
		}{
			{ebiten.KeyArrowLeft, []byte{0x1B, '[', '1', ';', '2', 'D'}},
			{ebiten.KeyArrowRight, []byte{0x1B, '[', '1', ';', '2', 'C'}},
			{ebiten.KeyArrowUp, []byte{0x1B, '[', '1', ';', '2', 'A'}},
			{ebiten.KeyArrowDown, []byte{0x1B, '[', '1', ';', '2', 'B'}},
			{ebiten.KeyHome, []byte{0x1B, '[', '1', ';', '2', 'H'}},
			{ebiten.KeyEnd, []byte{0x1B, '[', '1', ';', '2', 'F'}},
		}
		for _, sk := range shiftKeys {
			// When Ctrl is also held, only handle Home/End for selection
			if ctrl && sk.key != ebiten.KeyHome && sk.key != ebiten.KeyEnd {
				continue
			}
			if inpututil.IsKeyJustPressed(sk.key) || shouldRepeat(sk.key) {
				eo.emitSeq(sk.seq)
				shiftHandled = true
			}
		}
	}

	// Printable input path - skip when ctrl is held to avoid double emission.
	if !ctrl {
		for _, r := range ebiten.AppendInputChars(nil) {
			if b, ok := runeToInputByte(r); ok {
				eo.emitByte(b)
			}
		}
	} else {
		// Drain AppendInputChars to prevent stale buffer accumulation.
		ebiten.AppendInputChars(nil)
	}

	specialKeys := []ebiten.Key{
		ebiten.KeyEnter,
		ebiten.KeyNumpadEnter,
		ebiten.KeyBackspace,
		ebiten.KeyTab,
		ebiten.KeyEscape,
		ebiten.KeyArrowUp,
		ebiten.KeyArrowDown,
		ebiten.KeyArrowRight,
		ebiten.KeyArrowLeft,
		ebiten.KeyHome,
		ebiten.KeyEnd,
		ebiten.KeyDelete,
		ebiten.KeyPageUp,
		ebiten.KeyPageDown,
	}
	for _, key := range specialKeys {
		// Skip arrow keys when ctrl handled them as word-move/history
		if ctrlHandled && (key == ebiten.KeyArrowUp || key == ebiten.KeyArrowDown ||
			key == ebiten.KeyArrowLeft || key == ebiten.KeyArrowRight) {
			continue
		}
		// Skip selection keys when shift handled them
		if shiftHandled && (key == ebiten.KeyArrowLeft || key == ebiten.KeyArrowRight ||
			key == ebiten.KeyArrowUp || key == ebiten.KeyArrowDown ||
			key == ebiten.KeyHome || key == ebiten.KeyEnd) {
			continue
		}
		if inpututil.IsKeyJustPressed(key) || shouldRepeat(key) {
			if seq, ok := translateSpecialKey(key); ok {
				eo.emitSeq(seq)
			}
		}
	}

	// Middle mouse button paste
	if !monitorActive && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
		eo.bufferMutex.RLock()
		handler := eo.middleMouseHandler
		eo.bufferMutex.RUnlock()
		if handler != nil {
			handler()
		} else {
			eo.handleClipboardPaste()
		}
	}

	// Mouse wheel scrolling
	_, yoff := ebiten.Wheel()
	if yoff != 0 {
		eo.wheelAccum += yoff
		lines := int(eo.wheelAccum)
		if lines != 0 {
			eo.wheelAccum -= float64(lines)
			eo.bufferMutex.RLock()
			handler := eo.scrollHandler
			eo.bufferMutex.RUnlock()
			if handler != nil {
				handler(-lines)
			}
		}
	}
}

func runeToInputByte(r rune) (byte, bool) {
	if r <= 0 || r > 0xFF {
		return 0, false
	}
	return byte(r), true
}

func translateSpecialKey(key ebiten.Key) ([]byte, bool) {
	switch key {
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		return []byte{'\n'}, true
	case ebiten.KeyBackspace:
		return []byte{'\b'}, true
	case ebiten.KeyTab:
		return []byte{'\t'}, true
	case ebiten.KeyEscape:
		return []byte{0x1B}, true
	case ebiten.KeyArrowUp:
		return []byte{0x1B, '[', 'A'}, true
	case ebiten.KeyArrowDown:
		return []byte{0x1B, '[', 'B'}, true
	case ebiten.KeyArrowRight:
		return []byte{0x1B, '[', 'C'}, true
	case ebiten.KeyArrowLeft:
		return []byte{0x1B, '[', 'D'}, true
	case ebiten.KeyHome:
		return []byte{0x1B, '[', 'H'}, true
	case ebiten.KeyEnd:
		return []byte{0x1B, '[', 'F'}, true
	case ebiten.KeyDelete:
		return []byte{0x1B, '[', '3', '~'}, true
	case ebiten.KeyPageUp:
		return []byte{0x1B, '[', '5', '~'}, true
	case ebiten.KeyPageDown:
		return []byte{0x1B, '[', '6', '~'}, true
	default:
		return nil, false
	}
}

func (eo *EbitenOutput) handleClipboardPaste() {
	eo.clipboardOnce.Do(func() {
		eo.clipboardOK = clipboard.Init() == nil
	})
	if !eo.clipboardOK {
		return
	}
	data, _ := clipboard.ReadText()
	if len(data) == 0 {
		return
	}
	data = normalizePasteText(data)
	data = capPasteText(data, 4096)
	for _, b := range data {
		eo.emitByte(b)
	}
}

func (eo *EbitenOutput) Draw(screen *ebiten.Image) {
	// First rendered frame: let the host page know the screen is live. The
	// browser demo keeps its loading overlay up until this fires, because the
	// canvas element exists long before anything has been drawn to it.
	eo.firstFrameOnce.Do(hostSignalFirstFrame)
	crtMode := eo.crtPresentationMode()
	effectiveCRT := crtMode.enabled() && eo.ensureCRTFilter()
	hostSetCRTPresentationState(crtPresentationState(crtMode, effectiveCRT))
	curvedCRT := crtMode == crtModeCurved
	eo.resetPresentationTargetsAfterToggle()
	// F7 disables presentation entirely, so Guest-Advanced's finish pass does
	// not run to replace its history texture. Latch a reset for the next
	// enabled frame instead of allowing stale phosphor light to flash back in.
	if !effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced && eo.crtFilter != nil && eo.crtFilter.guest != nil {
		eo.crtFilter.guest.resetAfterglow()
	}
	presentation := eo.presentationImage(screen.Bounds().Dx(), screen.Bounds().Dy())
	// Routes are mutually exclusive. Clearing the retained staging image keeps
	// an overlay from inheriting guest, cursor, or status pixels from the
	// previous route.
	presentation.Clear()

	// Defer screen-capture recording: reads pixels after all rendering is done
	if eo.recorder != nil && eo.recorder.IsRecordingScreen() {
		sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
		need := sw * sh * 4
		if len(eo.screenCaptureBuf) < need {
			eo.screenCaptureBuf = make([]byte, need)
		}
		defer func() {
			screen.ReadPixels(eo.screenCaptureBuf[:need])
			eo.recorder.PushScreenFrame(eo.screenCaptureBuf[:need])
		}()
	}

	// When monitor is active, draw the overlay instead
	if eo.monitorOverlay != nil && eo.monitorOverlay.monitor.IsActive() {
		if effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced {
			eo.crtFilter.guest.setSourceMode("monitor-overlay")
		}
		eo.monitorOverlay.Draw(presentation)
		eo.completeCompositionScreenshot(presentation)
		eo.finishPresentation(screen, presentation, effectiveCRT, curvedCRT, defaultCRTPresentationGeometry())
		return
	}
	if eo.luaOverlay != nil && eo.luaOverlay.IsActive() {
		if effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced {
			eo.crtFilter.guest.setSourceMode("lua-overlay")
		}
		eo.luaOverlay.Draw(presentation)
		eo.completeCompositionScreenshot(presentation)
		eo.finishPresentation(screen, presentation, effectiveCRT, curvedCRT, defaultCRTPresentationGeometry())
		return
	}
	if eo.hostOverlay != nil && eo.hostOverlay.IsActive() {
		if effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced {
			eo.crtFilter.guest.setSourceMode("host-overlay")
		}
		eo.hostOverlay.Draw(presentation)
		eo.completeCompositionScreenshot(presentation)
		eo.finishPresentation(screen, presentation, effectiveCRT, curvedCRT, defaultCRTPresentationGeometry())
		return
	}

	eo.bufferMutex.Lock()
	usedHardware := eo.hwFrameID != 0
	if usedHardware {
		if effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced {
			eo.crtFilter.guest.setSourceMode(crtHardwareGuestModeKey(eo.hwLayers))
		}
		if effectiveCRT && eo.compositionScreenshotPending() {
			rawComposition := eo.compositionCaptureImage(screen.Bounds().Dx(), screen.Bounds().Dy())
			eo.drawHardwareCompositorLocked(rawComposition, false, nil)
			eo.completeCompositionScreenshot(rawComposition)
		}
		eo.drawHardwareCompositorLocked(presentation, effectiveCRT, eo.crtFilter)
	} else {
		if eo.window == nil {
			eo.window = ebiten.NewImage(eo.width, eo.height)
		}
		eo.window.WritePixels(eo.frameBuffer)
	}
	showStatusBar := eo.showStatusBar
	cursorImage := eo.cursorImage
	noSoftwareCursor := eo.noSoftwareCursor
	termMMIO := eo.termMMIO
	compositor := eo.compositor
	eo.bufferMutex.Unlock()
	if !usedHardware {
		if effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced && eo.compositionScreenshotPending() {
			rawComposition := eo.compositionCaptureImage(screen.Bounds().Dx(), screen.Bounds().Dy())
			rawComposition.Clear()
			rawComposition.DrawImage(eo.window, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
			eo.completeCompositionScreenshot(rawComposition)
		}
		if effectiveCRT && eo.crtProfile == crtProfileGuestAdvanced {
			eo.crtFilter.guest.setSourceMode(fmt.Sprintf("framebuffer:%dx%d", eo.width, eo.height))
			eo.crtFilter.guest.drawRaster(presentation, eo.window, guestAdvancedRasterUniforms(eo.width, eo.height, presentation.Bounds().Dx(), presentation.Bounds().Dy()), nil, nil)
		} else {
			presentation.DrawImage(eo.window, nil)
		}
	}
	// This is the diagnostic GPU compositor boundary: guest pixels are present,
	// while host cursor and status-bar pixels have not yet been composed.
	eo.completeCompositionScreenshot(presentation)
	postCompositor := presentation
	if effectiveCRT && (usedHardware || eo.crtProfile == crtProfileGuestAdvanced) {
		postCompositor = eo.crtPresentationOverlayImage(screen.Bounds().Dx(), screen.Bounds().Dy())
		postCompositor.Clear()
	}

	// Draw software cursor when the system cursor is hidden (EmuTOS mode).
	// AROS draws its own Intuition cursor in VRAM, so skip when noSoftwareCursor is set.
	if cursorImage != nil && termMMIO != nil && !noSoftwareCursor {
		mx := int(termMMIO.mouseX.Load())
		my := int(termMMIO.mouseY.Load())
		if compositor != nil {
			mx, my = compositor.MapNativePointToPresentation(mx, my)
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(mx), float64(my))
		postCompositor.DrawImage(cursorImage, op)
	}

	if showStatusBar {
		eo.drawRuntimeStatusBar(postCompositor, crtMode, effectiveCRT)
	}
	if postCompositor != presentation {
		eo.compositeCRTPresentationOverlay(presentation, postCompositor)
	}
	// Hardware layers have already received Zfast while sampling their native
	// textures. Do not run the composed guest through a second CRT pass.
	// Zfast is fully applied while compositing hardware layers. Guest-Advanced
	// needs a final viewport bloom/mask/deconvergence stage after those layers.
	eo.finishPresentation(screen, presentation, effectiveCRT && (!usedHardware || eo.crtProfile == crtProfileGuestAdvanced), curvedCRT, defaultCRTPresentationGeometry())
}

// ensureCRTFilter lazily compiles once. A backend that cannot compile or run
// Kage falls back for the rest of the session instead of attempting work every
// frame, and the current Draw already observes the false effective state.
func (eo *EbitenOutput) ensureCRTFilter() bool {
	if eo.crtProfile == crtProfileGuestAdvanced {
		return eo.initialiseGuestAdvancedCRTFilter()
	}
	return eo.initialiseCRTFilter([]byte(zfastCRTShaderSource))
}

func (eo *EbitenOutput) initialiseGuestAdvancedCRTFilter() bool {
	switch eo.crtState {
	case crtFilterAvailable:
		return true
	case crtFilterFailed:
		return false
	}
	eo.crtFilter = newGuestAdvancedCRTFilter()
	if eo.crtFilter.err != nil {
		eo.crtState = crtFilterFailed
		fmt.Printf("Ebiten: Guest-Advanced unavailable, using unfiltered presentation: %v\n", eo.crtFilter.err)
		return false
	}
	eo.crtState = crtFilterAvailable
	return true
}

// initialiseCRTFilter keeps source selection at the construction boundary so
// GPU fallback tests can use invalid Kage without changing process-wide state.
func (eo *EbitenOutput) initialiseCRTFilter(source []byte) bool {
	switch eo.crtState {
	case crtFilterAvailable:
		return true
	case crtFilterFailed:
		return false
	}
	eo.crtFilter = newCRTFilter(source)
	if eo.crtFilter.err != nil {
		eo.crtState = crtFilterFailed
		fmt.Printf("Ebiten: CRT filter unavailable, using unfiltered presentation: %v\n", eo.crtFilter.err)
		return false
	}
	eo.crtState = crtFilterAvailable
	return true
}

func (eo *EbitenOutput) presentationImage(width, height int) *ebiten.Image {
	if eo.presentation == nil || eo.presentation.Bounds().Dx() != width || eo.presentation.Bounds().Dy() != height {
		if eo.presentation != nil {
			eo.presentation.Deallocate()
		}
		eo.presentation = ebiten.NewImage(width, height)
	}
	return eo.presentation
}

// compositionCaptureImage owns the unfiltered GPU target used by
// rec.screenshot_composed. Hardware CRT applies a layer shader while writing
// presentation, so that target cannot provide a pre-CRT diagnostic image.
func (eo *EbitenOutput) compositionCaptureImage(width, height int) *ebiten.Image {
	if eo.compositionCapture == nil || eo.compositionCapture.Bounds().Dx() != width || eo.compositionCapture.Bounds().Dy() != height {
		if eo.compositionCapture != nil {
			eo.compositionCapture.Deallocate()
		}
		eo.compositionCapture = ebiten.NewImage(width, height)
	}
	return eo.compositionCapture
}

// resetPresentationTargetsAfterToggle makes the first frame following F7
// independent of GPU render targets from the preceding presentation mode.
// Hardware layers remain GPU-composited; their source textures are simply
// uploaded again on that first frame.
func (eo *EbitenOutput) resetPresentationTargetsAfterToggle() {
	if !eo.presentationReset.Swap(false) {
		return
	}
	if eo.presentation != nil {
		eo.presentation.Deallocate()
		eo.presentation = nil
	}
	if eo.compositionCapture != nil {
		eo.compositionCapture.Deallocate()
		eo.compositionCapture = nil
	}
	if eo.crtPresentationOverlay != nil {
		eo.crtPresentationOverlay.Deallocate()
		eo.crtPresentationOverlay = nil
	}
	if eo.crtFilter != nil && eo.crtFilter.guest != nil {
		eo.crtFilter.guest.disposeTargets()
		eo.crtFilter.guest.resetAfterglow()
	}
	eo.bufferMutex.Lock()
	for i := range eo.hwLayers {
		eo.hwLayers[i].haveUpload = false
		eo.hwLayers[i].geomValid = false
	}
	eo.bufferMutex.Unlock()
}

func (eo *EbitenOutput) crtPresentationOverlayImage(width, height int) *ebiten.Image {
	if eo.crtPresentationOverlay == nil || eo.crtPresentationOverlay.Bounds().Dx() != width || eo.crtPresentationOverlay.Bounds().Dy() != height {
		if eo.crtPresentationOverlay != nil {
			eo.crtPresentationOverlay.Deallocate()
		}
		eo.crtPresentationOverlay = ebiten.NewImage(width, height)
	}
	return eo.crtPresentationOverlay
}

// compositeCRTPresentationOverlay filters only the post-compositor host
// elements and source-over composites them onto native-source CRT guest layers.
// Applying the shader to presentation instead would filter the guest twice.
func (eo *EbitenOutput) compositeCRTPresentationOverlay(presentation, overlay *ebiten.Image) {
	if eo.crtProfile == crtProfileGuestAdvanced {
		eo.crtFilter.guest.drawRaster(presentation, overlay, guestAdvancedOverlayUniforms(overlay.Bounds().Dx(), overlay.Bounds().Dy()), nil, nil)
		return
	}
	op := &ebiten.DrawRectShaderOptions{}
	op.Images[0] = overlay
	op.Uniforms = map[string]any{
		"ScanlinePeriod": defaultCRTPresentationGeometry().scanlinePeriod,
		"ScanlineOrigin": defaultCRTPresentationGeometry().scanlineOrigin,
	}
	presentation.DrawRectShader(overlay.Bounds().Dx(), overlay.Bounds().Dy(), eo.crtFilter.shader, op)
}

func (eo *EbitenOutput) finishPresentation(screen, presentation *ebiten.Image, effectiveCRT, curvedCRT bool, geometry crtPresentationGeometry) {
	if effectiveCRT {
		if eo.crtProfile == crtProfileGuestAdvanced {
			eo.crtFilter.guest.finish(screen, presentation, curvedCRT)
		} else {
			op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
			op.Images[0] = presentation
			op.Uniforms = map[string]any{
				"ScanlinePeriod": geometry.scanlinePeriod,
				"ScanlineOrigin": geometry.scanlineOrigin,
			}
			screen.DrawRectShader(presentation.Bounds().Dx(), presentation.Bounds().Dy(), eo.crtFilter.shader, op)
		}
	} else {
		screen.DrawImage(presentation, nil)
	}
	eo.completePresentationScreenshot(screen)

	eo.frameCount.Add(1)
	select {
	case eo.vsyncChan <- struct{}{}:
	default:
	}
}

// TakeCompositionScreenshot captures the GPU-composed image immediately
// before CRT presentation. It distinguishes a compositor failure from a
// final-presentation failure without treating a CPU compositor screenshot as
// evidence of what the GPU actually drew.
func (eo *EbitenOutput) TakeCompositionScreenshot(path string) error {
	if path == "" {
		return fmt.Errorf("composition screenshot path is required")
	}
	req := &presentationScreenshotRequest{path: path, done: make(chan error, 1)}
	eo.presentationShotMu.Lock()
	if eo.compositionShot != nil {
		eo.presentationShotMu.Unlock()
		return fmt.Errorf("composition screenshot already pending")
	}
	eo.compositionShot = req
	eo.presentationShotMu.Unlock()
	return eo.waitForScreenshotRequest(req, func() {
		eo.presentationShotMu.Lock()
		if eo.compositionShot == req {
			eo.compositionShot = nil
		}
		eo.presentationShotMu.Unlock()
	})
}

func (eo *EbitenOutput) compositionScreenshotPending() bool {
	eo.presentationShotMu.Lock()
	pending := eo.compositionShot != nil
	eo.presentationShotMu.Unlock()
	return pending
}

// TakePresentationScreenshot captures the next final Ebiten frame. It is used
// by IEScript when an acceptance test needs the visible CRT presentation rather
// than the pre-presentation compositor frame.
func (eo *EbitenOutput) TakePresentationScreenshot(path string) error {
	if path == "" {
		return fmt.Errorf("presentation screenshot path is required")
	}
	req := &presentationScreenshotRequest{path: path, done: make(chan error, 1)}
	eo.presentationShotMu.Lock()
	if eo.presentationShot != nil {
		eo.presentationShotMu.Unlock()
		return fmt.Errorf("presentation screenshot already pending")
	}
	eo.presentationShot = req
	eo.presentationShotMu.Unlock()

	return eo.waitForScreenshotRequest(req, func() {
		eo.presentationShotMu.Lock()
		if eo.presentationShot == req {
			eo.presentationShot = nil
		}
		eo.presentationShotMu.Unlock()
	})
}

func (eo *EbitenOutput) waitForScreenshotRequest(req *presentationScreenshotRequest, cancel func()) error {
	select {
	case err := <-req.done:
		return err
	case <-time.After(5 * time.Second):
		cancel()
		return fmt.Errorf("timed out waiting for presentation frame")
	}
}

func (eo *EbitenOutput) completePresentationScreenshot(screen *ebiten.Image) {
	eo.presentationShotMu.Lock()
	req := eo.presentationShot
	eo.presentationShot = nil
	eo.presentationShotMu.Unlock()
	if req == nil {
		return
	}
	eo.completeScreenshot(req, screen)
}

func (eo *EbitenOutput) completeCompositionScreenshot(composition *ebiten.Image) {
	eo.presentationShotMu.Lock()
	req := eo.compositionShot
	eo.compositionShot = nil
	eo.presentationShotMu.Unlock()
	eo.completeScreenshot(req, composition)
}

func (eo *EbitenOutput) completeScreenshot(req *presentationScreenshotRequest, source *ebiten.Image) {
	if req == nil {
		return
	}
	w, h := source.Bounds().Dx(), source.Bounds().Dy()
	pixels := make([]byte, w*h*BYTES_PER_PIXEL)
	source.ReadPixels(pixels)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels)
	f, err := os.Create(req.path)
	if err == nil {
		err = png.Encode(f, img)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}
	req.done <- err
}

func (eo *EbitenOutput) drawHardwareCompositorLocked(screen *ebiten.Image, crtActive bool, filter *crtFilter) {
	screen.Clear()
	if !eo.hwHasContent || eo.hwCopyShader == nil {
		return
	}
	for i := range eo.hwLayers {
		layer := &eo.hwLayers[i]
		if layer.SourceWidth <= 0 || layer.SourceHeight <= 0 || layer.DestWidth <= 0 || layer.DestHeight <= 0 {
			continue
		}
		indexed := len(layer.indices) >= layer.SourceWidth*layer.SourceHeight
		if !indexed && len(layer.Buffer) == 0 {
			continue
		}

		// The source image for the draw below is either the layer's own RGBA
		// texture or, for an indexed layer, the shader-expanded output of its
		// converter. Everything after this point is identical either way.
		src := layer.image
		if indexed {
			if layer.conv == nil {
				layer.conv = &clut8GPUConverter{}
			}
			converted, err := clut8ConvertLayer(layer)
			if err != nil {
				// The shader cannot run on this backend. Expand on the CPU so
				// this frame is still correct, and latch the failure so the
				// compositor stops sending indices at all: the compositor
				// cannot fall back on our behalf, because the frame update it
				// already accepted reported success.
				if !eo.indexedUnsupported.Swap(true) {
					fmt.Printf("Ebiten: CLUT8 GPU conversion failed, falling back to CPU expansion: %v\n", err)
				}
				layer.Buffer = expandIndexedLayerToRGBA(layer)
				layer.indices = layer.indices[:0]
				indexed = false
			} else {
				src = converted
			}
		}
		retained := retainedLayersEnabled()
		if !indexed {
			newImage := layer.image == nil || layer.image.Bounds().Dx() != layer.SourceWidth || layer.image.Bounds().Dy() != layer.SourceHeight
			if newImage {
				if layer.image != nil {
					layer.image.Dispose()
				}
				layer.image = ebiten.NewImage(layer.SourceWidth, layer.SourceHeight)
				layer.haveUpload = false
			}
			// Skip the whole-image WritePixels when the retained texture already
			// holds this exact source's unchanged content. A source that does not
			// track a generation gets a per-frame ContentGen, so this never
			// short-circuits it.
			if !layer.retainedUploadSkippable(newImage, retained) {
				pixelBytes := layer.SourceWidth * layer.SourceHeight * BYTES_PER_PIXEL
				layer.image.WritePixels(layer.Buffer[:pixelBytes])
				layer.haveUpload = true
				layer.uploadedSourceID = layer.SourceID
				layer.uploadedGen = layer.ContentGen
				eo.hwUploadCount.Add(1)
			}
			src = layer.image
		}

		key := ebitenLayerGeomKey{
			sw: layer.SourceWidth, sh: layer.SourceHeight,
			dx: layer.DestX, dy: layer.DestY,
			dw: layer.DestWidth, dh: layer.DestHeight,
			opaque: layer.Opaque,
		}
		if !layer.geomReusable(retained, key) {
			x0 := float32(layer.DestX)
			y0 := float32(layer.DestY)
			x1 := float32(layer.DestX + layer.DestWidth)
			y1 := float32(layer.DestY + layer.DestHeight)
			sw := float32(layer.SourceWidth)
			sh := float32(layer.SourceHeight)
			layer.cachedVertices = []ebiten.Vertex{
				{DstX: x0, DstY: y0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				{DstX: x1, DstY: y0, SrcX: sw, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				{DstX: x0, DstY: y1, SrcX: 0, SrcY: sh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
				{DstX: x1, DstY: y1, SrcX: sw, SrcY: sh, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
			}
			layer.cachedOptions = &ebiten.DrawTrianglesShaderOptions{
				Blend: ebiten.BlendCopy,
				Uniforms: map[string]any{
					"SrcSize":  []float32{sw, sh},
					"RectSize": []float32{float32(layer.DestWidth), float32(layer.DestHeight)},
					"DestOrigin": []float32{
						float32(layer.DestX),
						float32(layer.DestY),
					},
					"Opaque": opaqueUniform(layer.Opaque),
				},
			}
			layer.geomKey = key
			layer.geomValid = true
		}
		op := layer.cachedOptions
		op.Images[0] = src
		shader := eo.hwCopyShader
		if crtActive && filter != nil && filter.shader != nil {
			shader = filter.shader
			op = &ebiten.DrawTrianglesShaderOptions{Blend: ebiten.BlendCopy, Uniforms: crtHardwareLayerUniforms(layer)}
			op.Images[0] = src
		} else if crtActive && filter != nil && filter.guest != nil {
			shader = filter.guest.raster
			op = &ebiten.DrawTrianglesShaderOptions{Blend: ebiten.BlendCopy, Uniforms: crtHardwareLayerUniforms(layer)}
			op.Images[0] = src
		}
		screen.DrawTrianglesShader(layer.cachedVertices, ebitenHWQuadIndices, shader, op)
	}
}

func (eo *EbitenOutput) Layout(_, _ int) (int, int) {
	eo.bufferMutex.RLock()
	defer eo.bufferMutex.RUnlock()
	return eo.width, eo.height
}

func playbackCPUFlags(s runtimeStatusSnapshot) (ie32 bool, ie64 bool, m68k bool, z80 bool, x86 bool, cpu65 bool) {
	if s.psgPlayer != nil && s.psgEngine != nil && s.psgEngine.IsPlaying() {
		_, cpuName, _ := s.psgPlayer.RenderPerf()
		switch cpuName {
		case "Z80":
			z80 = true
		case "68K":
			m68k = true
		case "6502":
			cpu65 = true
		case "IE32":
			ie32 = true
		case "IE64":
			ie64 = true
		case "X86":
			x86 = true
		}
	}
	if s.sidPlayer != nil && s.sidPlayer.IsPlaying() {
		_, cpuName, _ := s.sidPlayer.RenderPerf()
		if cpuName == "6502" {
			cpu65 = true
		}
	}
	if s.pokeyPlayer != nil && s.pokeyPlayer.IsPlaying() {
		_, cpuName, _ := s.pokeyPlayer.RenderPerf()
		if cpuName == "6502" {
			cpu65 = true
		}
	}
	if s.tedPlayer != nil && s.tedPlayer.IsPlaying() {
		_, cpuName, _ := s.tedPlayer.RenderPerf()
		if cpuName == "6502" {
			cpu65 = true
		}
	}
	return
}

type statusToken struct {
	name    string
	enabled bool
	colour  statusTokenColour
}

type statusTokenColour uint8

const (
	statusTokenColourDefault statusTokenColour = iota
	statusTokenColourBlue
)

const runtimeStatusBarAlpha = 40

// statusBarPixel is a lazily created 1x1 white image scaled and tinted to draw
// filled rectangles. Replaces the deprecated ebitenutil.DrawRect: dropping the
// ebitenutil import keeps net/http (and with it the TLS/x509 stack) out of the
// js/wasm binary.
var statusBarPixel *ebiten.Image

func drawFilledRect(screen *ebiten.Image, x, y, w, h float64, clr color.RGBA) {
	if statusBarPixel == nil {
		statusBarPixel = ebiten.NewImage(1, 1)
		statusBarPixel.Fill(color.White)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(w, h)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	screen.DrawImage(statusBarPixel, op)
}

func drawStatusLine(screen *ebiten.Image, x, baselineY int, label string, tokens []statusToken) {
	face := basicfont.Face7x13
	labelColor := color.RGBA{190, 190, 190, 255}

	text.Draw(screen, label, face, x, baselineY, labelColor)
	cursorX := x + text.BoundString(face, label).Dx() + 6
	drawStatusTokens(screen, cursorX, baselineY, tokens)
}

func drawStatusTokens(screen *ebiten.Image, x, baselineY int, tokens []statusToken) {
	face := basicfont.Face7x13
	offColor := color.RGBA{120, 120, 120, 255}
	onColor := color.RGBA{0, 220, 90, 255}
	blueColor := color.RGBA{80, 160, 255, 255}
	cursorX := x
	for _, token := range tokens {
		c := offColor
		if token.colour == statusTokenColourBlue {
			c = blueColor
		} else if token.enabled {
			c = onColor
		}
		text.Draw(screen, token.name, face, cursorX, baselineY, c)
		cursorX += text.BoundString(face, token.name).Dx() + 8
	}
}

func statusTokensWidth(tokens []statusToken) int {
	face := basicfont.Face7x13
	width := 0
	for i, token := range tokens {
		width += text.BoundString(face, token.name).Dx()
		if i != len(tokens)-1 {
			width += 8
		}
	}
	return width
}

func runtimeCPUStatusTokens(s runtimeStatusSnapshot) []statusToken {
	playIE32, playIE64, playM68K, playZ80, playX86, play6502 := playbackCPUFlags(s)

	ie32On := (s.selectedCPU == runtimeCPUIE32 && s.ie32 != nil && s.ie32.IsRunning()) || playIE32
	ie64On := (s.selectedCPU == runtimeCPUIE64 && s.ie64 != nil && s.ie64.IsRunning()) || playIE64
	m68kOn := (s.selectedCPU == runtimeCPUM68K && s.m68k != nil && s.m68k.IsRunning()) || playM68K
	z80On := (s.selectedCPU == runtimeCPUZ80 && s.z80 != nil && s.z80.IsRunning()) || playZ80
	x86On := (s.selectedCPU == runtimeCPUX86 && s.x86 != nil && s.x86.IsRunning()) || playX86
	cpu65On := (s.selectedCPU == runtimeCPU6502 && s.cpu65 != nil && s.cpu65.IsRunning()) || play6502

	// Coprocessor workers also light up their CPU indicators
	if cm := s.coprocManager; cm != nil {
		if cm.IsWorkerRunning(EXEC_TYPE_IE32) {
			ie32On = true
		}
		if cm.IsWorkerRunning(EXEC_TYPE_IE64) {
			ie64On = true
		}
		if cm.IsWorkerRunning(EXEC_TYPE_M68K) {
			m68kOn = true
		}
		if cm.IsWorkerRunning(EXEC_TYPE_Z80) {
			z80On = true
		}
		if cm.IsWorkerRunning(EXEC_TYPE_X86) {
			x86On = true
		}
		if cm.IsWorkerRunning(EXEC_TYPE_6502) {
			cpu65On = true
		}
	}
	iesOn := s.scriptEngine != nil && s.scriptEngine.IsRunning()

	return []statusToken{
		{name: "IE32 ", enabled: ie32On},
		{name: "|", enabled: false},
		{name: "Z80", enabled: z80On},
		{name: "|", enabled: false},
		{name: "X86", enabled: x86On},
		{name: "|", enabled: false},
		{name: "68K", enabled: m68kOn},
		{name: "|", enabled: false},
		{name: "IE64 ", enabled: ie64On},
		{name: "|", enabled: false},
		{name: "6502", enabled: cpu65On},
		{name: "|", enabled: false},
		{name: "IES", enabled: iesOn},
	}
}

func ebitenStatusLegendTokens(lockFullscreen, scaleToggleAvailable bool, scaleMode PresentationScaleMode, crtMode crtPresentationMode, effectiveCRT bool) []statusToken {
	crtToken := statusToken{name: "F7:CRT"}
	if effectiveCRT {
		crtToken.enabled = crtMode == crtModeFlat
		if crtMode == crtModeCurved {
			crtToken.colour = statusTokenColourBlue
		}
	}
	tokens := []statusToken{
		crtToken,
		{name: "F8:IE Script", enabled: false},
		{name: "F9:IE Monitor", enabled: false},
		{name: "F10:Reset", enabled: false},
	}
	if !lockFullscreen {
		tokens = append(tokens, statusToken{name: "Shift+F11:Fullscreen/Windowed", enabled: false})
	}
	if scaleToggleAvailable {
		tokens = append(tokens,
			statusToken{name: "F11:", enabled: false},
			statusToken{name: "fit", enabled: scaleMode == ScaleAspectFit},
			statusToken{name: "/", enabled: false},
			statusToken{name: "stretch", enabled: scaleMode == ScaleStretchFill},
		)
	}
	return append(tokens,
		statusToken{name: "F12:Status", enabled: false},
		statusToken{name: "Ctrl+Alt:Mouse", enabled: false},
	)
}

// drawRuntimeStatusBar draws the overlay from a cached image. Its text changes
// only when a device switches on or off, but redrawing it meant a text.Draw and
// a text.BoundString for every token on every frame, each hashing through the
// glyph and kerning caches. A browser profile of the rotozoomer put that at
// half the remaining map-lookup cost of the whole machine, for pixels that were
// identical to the previous frame. The bar is now rendered once into an
// offscreen image and re-rendered only when its content or the window geometry
// changes.
func (eo *EbitenOutput) drawRuntimeStatusBar(screen *ebiten.Image, crtMode crtPresentationMode, effectiveCRT bool) {
	s := runtimeStatus.snapshot()

	videoOn := s.video != nil && s.video.IsEnabled()
	vgaOn := s.vga != nil && s.vga.IsEnabled()
	ulaOn := s.ula != nil && s.ula.IsEnabled()
	tedVideoOn := s.tedVideo != nil && s.tedVideo.IsEnabled()
	anticOn := s.antic != nil && s.antic.IsEnabled()
	voodooOn := s.voodoo != nil && s.voodoo.IsEnabled()

	audioStatus := runtimeAudioStatusIndicators(s)
	audioTokens := make([]statusToken, 0, len(audioStatus))
	for _, indicator := range audioStatus {
		audioTokens = append(audioTokens, statusToken{name: indicator.name, enabled: indicator.enabled})
	}

	barHeight := 44
	if barHeight >= eo.height {
		return
	}
	y := eo.height - barHeight

	eo.bufferMutex.RLock()
	compositor := eo.compositor
	lockFullscreen := eo.lockFullscreen
	eo.bufferMutex.RUnlock()
	scaleToggleAvailable := false
	scaleMode := ScaleAspectFit
	if compositor != nil && compositor.ActiveSourceNeedsScaleToggle() {
		scaleToggleAvailable = true
		scaleMode = compositor.GetScaleMode()
	}

	cpuTokens := runtimeCPUStatusTokens(s)
	videoTokens := []statusToken{
		{name: "IEVID", enabled: videoOn},
		{name: "|", enabled: false},
		{name: "VGA", enabled: vgaOn},
		{name: "|", enabled: false},
		{name: "ULA", enabled: ulaOn},
		{name: "|", enabled: false},
		{name: "TED", enabled: tedVideoOn},
		{name: "|", enabled: false},
		{name: "ANTIC", enabled: anticOn},
		{name: "|", enabled: false},
		{name: "VOODOO", enabled: voodooOn},
	}
	legendTokens := ebitenStatusLegendTokens(lockFullscreen, scaleToggleAvailable, scaleMode, crtMode, effectiveCRT)

	key := statusBarCacheKey(eo.width, barHeight, cpuTokens, videoTokens, audioTokens, legendTokens)
	if eo.statusBarImage == nil || eo.statusBarKey != key ||
		eo.statusBarImage.Bounds().Dx() != eo.width || eo.statusBarImage.Bounds().Dy() != barHeight {
		if eo.statusBarImage != nil {
			eo.statusBarImage.Deallocate()
		}
		eo.statusBarImage = ebiten.NewImage(eo.width, barHeight)
		eo.statusBarKey = key
		bar := eo.statusBarImage
		drawFilledRect(bar, 0, 0, float64(eo.width), float64(barHeight), color.RGBA{0, 0, 0, runtimeStatusBarAlpha})
		drawStatusLine(bar, 6, 13, "CPU  ", cpuTokens)
		drawStatusLine(bar, 6, 26, "VIDEO", videoTokens)
		drawStatusLine(bar, 6, 39, "AUDIO", audioTokens)
		legendX := max(eo.width-statusTokensWidth(legendTokens)-6, 6)
		drawStatusTokens(bar, legendX, 39, legendTokens)
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, float64(y))
	screen.DrawImage(eo.statusBarImage, op)
}

// statusBarCacheKey summarises everything the bar draws, so the cached image is
// rebuilt exactly when what it would draw has changed.
func statusBarCacheKey(width, height int, groups ...[]statusToken) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d", width, height)
	for _, tokens := range groups {
		b.WriteByte(';')
		for _, token := range tokens {
			b.WriteString(token.name)
			b.WriteByte(byte('0' + token.colour))
			if token.enabled {
				b.WriteByte('+')
			} else {
				b.WriteByte('-')
			}
		}
	}
	return b.String()
}
