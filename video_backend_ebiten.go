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
	"image/color"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.design/x/clipboard"
	"golang.org/x/image/font/basicfont"
)

func init() {
	compiledFeatures = append(compiledFeatures, "video:ebiten")
}

type EbitenOutput struct {
	running            bool
	window             *ebiten.Image
	width              int
	height             int
	format             PixelFormat
	fullscreen         bool
	scale              int
	windowedW          int
	windowedH          int
	frameBuffer        []byte
	bufferMutex        sync.RWMutex
	frameCount         uint64
	refreshRate        int
	vsyncChan          chan struct{}
	done               chan struct{}
	keyHandler         func(byte)
	scrollHandler      func(int)
	copyHandler        func()
	cutHandler         func()
	middleMouseHandler func()
	wheelAccum         float64

	clipboardOnce sync.Once
	clipboardOK   bool
	showStatusBar bool

	hardResetHandler func()
	resetInProgress  atomic.Bool

	monitorOverlay   *MonitorOverlay
	luaOverlay       *LuaOverlay
	termMMIO         *TerminalMMIO
	hideSystemCursor bool
}

func NewEbitenOutput() (VideoOutput, error) {
	return &EbitenOutput{
		width:         DefaultScreenWidth,
		height:        DefaultScreenHeight,
		format:        PixelFormatRGBA,
		scale:         1,
		windowedW:     DefaultScreenWidth,
		windowedH:     DefaultScreenHeight,
		frameBuffer:   make([]byte, DefaultScreenWidth*DefaultScreenHeight*4),
		refreshRate:   60,
		vsyncChan:     make(chan struct{}, 1),
		done:          make(chan struct{}),
		showStatusBar: true,
	}, nil
}

func (eo *EbitenOutput) Start() error {
	if eo.running {
		return nil
	}
	eo.bufferMutex.Lock()
	eo.done = make(chan struct{})
	eo.bufferMutex.Unlock()
	eo.running = true
	ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
	ebiten.SetWindowTitle("Intuition Engine (c) 2024 - 2026 Zayn Otley")
	ebiten.SetWindowResizable(true)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetVsyncEnabled(true)
	if eo.hideSystemCursor {
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
	}
	if eo.fullscreen {
		ebiten.SetFullscreen(true)
	}

	go func() {
		defer func() {
			eo.running = false
			eo.bufferMutex.RLock()
			done := eo.done
			eo.bufferMutex.RUnlock()
			select {
			case <-done:
			default:
				close(done)
			}
		}()
		if err := ebiten.RunGame(eo); err != nil {
			fmt.Printf("Ebiten error: %v\n", err)
		}
	}()

	// Wait for first Draw call to ensure Ebiten is ready
	<-eo.vsyncChan
	return nil
}

func (eo *EbitenOutput) Stop() error {
	eo.running = false
	return nil
}

func (eo *EbitenOutput) Close() error {
	return eo.Stop()
}

func (eo *EbitenOutput) Done() <-chan struct{} {
	eo.bufferMutex.RLock()
	done := eo.done
	eo.bufferMutex.RUnlock()
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
	copy(eo.frameBuffer, data)
	eo.bufferMutex.Unlock()
	return nil
}

func (eo *EbitenOutput) SetDisplayConfig(config DisplayConfig) error {
	eo.bufferMutex.Lock()
	defer eo.bufferMutex.Unlock()

	width := config.Width
	height := config.Height
	if width <= 0 {
		width = eo.width
	}
	if height <= 0 {
		height = eo.height
	}
	if width <= 0 {
		width = DefaultScreenWidth
	}
	if height <= 0 {
		height = DefaultScreenHeight
	}
	eo.width = width
	eo.height = height
	eo.format = config.PixelFormat
	eo.scale = ClampScale(config.Scale)
	newSize := eo.width * eo.height * 4

	if len(eo.frameBuffer) != newSize {
		eo.frameBuffer = make([]byte, newSize)
	}

	eo.windowedW = eo.width * eo.scale
	eo.windowedH = eo.height * eo.scale
	eo.fullscreen = config.Fullscreen
	ebiten.SetFullscreen(eo.fullscreen)
	if !eo.fullscreen {
		ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
	}
	if eo.window != nil {
		eo.window.Dispose()
		eo.window = nil
	}
	return nil
}

func (eo *EbitenOutput) GetDisplayConfig() DisplayConfig {
	return DisplayConfig{
		Width:       eo.width,
		Height:      eo.height,
		Scale:       eo.scale,
		PixelFormat: eo.format,
		RefreshRate: eo.refreshRate,
		VSync:       true,
		Fullscreen:  eo.fullscreen,
	}
}

func (eo *EbitenOutput) WaitForVSync() error {
	<-eo.vsyncChan
	// print current FPS to console
	fmt.Printf("FPS: %0.2f\n", ebiten.CurrentFPS())
	return nil
}

func (eo *EbitenOutput) GetFrameCount() uint64 {
	return eo.frameCount
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
	return eo.running
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
	if x < 0 || y < 0 || x+width > eo.width || y+height > eo.height {
		return fmt.Errorf("region coordinates out of bounds")
	}

	eo.bufferMutex.Lock()
	for dy := range height {
		dstOffset := ((y+dy)*eo.width + x) * 4
		srcOffset := dy * width * 4
		copy(eo.frameBuffer[dstOffset:], pixels[srcOffset:srcOffset+width*4])
	}
	eo.bufferMutex.Unlock()
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
	if !eo.running {
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

	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		eo.bufferMutex.Lock()
		eo.fullscreen = !eo.fullscreen
		ebiten.SetFullscreen(eo.fullscreen)
		if !eo.fullscreen {
			ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
		}
		eo.bufferMutex.Unlock()
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
		return
	}
	eo.luaOverlay.SetScriptEngine(scriptEngine)
}

func (eo *EbitenOutput) SetTerminalMMIO(tm *TerminalMMIO) {
	eo.bufferMutex.Lock()
	eo.termMMIO = tm
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
	eo.hideSystemCursor = true
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

func shouldRepeat(key ebiten.Key) bool {
	dur := inpututil.KeyPressDuration(key)
	if dur < keyRepeatDelay {
		return false
	}
	return (dur-keyRepeatDelay)%keyRepeatInterval == 0
}

func (eo *EbitenOutput) updateTerminalMMIOInput() {
	eo.bufferMutex.RLock()
	tm := eo.termMMIO
	width := eo.width
	height := eo.height
	eo.bufferMutex.RUnlock()
	if tm == nil {
		return
	}

	mx, my := ebiten.CursorPosition()
	newX := int32(max(0, min(mx, width-1)))
	newY := int32(max(0, min(my, height-1)))

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

	oldX := tm.mouseX.Swap(newX)
	oldY := tm.mouseY.Swap(newY)
	oldButtons := tm.mouseButtons.Swap(buttons)
	if oldX != newX || oldY != newY || oldButtons != buttons {
		tm.mouseChanged.Store(true)
	}

	for ebitenKey, stScancode := range ebitenToSTScancode {
		if inpututil.IsKeyJustPressed(ebitenKey) {
			tm.EnqueueScancode(stScancode)
		}
		if inpututil.IsKeyJustReleased(ebitenKey) {
			tm.EnqueueScancode(stScancode | 0x80)
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
	data := clipboard.Read(clipboard.FmtText)
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
	// When monitor is active, draw the overlay instead
	if eo.monitorOverlay != nil && eo.monitorOverlay.monitor.IsActive() {
		eo.monitorOverlay.Draw(screen)
		eo.frameCount++
		select {
		case eo.vsyncChan <- struct{}{}:
		default:
		}
		return
	}
	if eo.luaOverlay != nil && eo.luaOverlay.IsActive() {
		eo.luaOverlay.Draw(screen)
		eo.frameCount++
		select {
		case eo.vsyncChan <- struct{}{}:
		default:
		}
		return
	}

	if eo.window == nil {
		eo.window = ebiten.NewImage(eo.width, eo.height)
	}

	eo.bufferMutex.RLock()
	eo.window.WritePixels(eo.frameBuffer)
	showStatusBar := eo.showStatusBar
	eo.bufferMutex.RUnlock()
	screen.DrawImage(eo.window, nil)
	if showStatusBar {
		eo.drawRuntimeStatusBar(screen)
	}

	eo.frameCount++
	select {
	case eo.vsyncChan <- struct{}{}:
	default:
	}
}

func (eo *EbitenOutput) Layout(_, _ int) (int, int) {
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
}

func drawStatusLine(screen *ebiten.Image, x, baselineY int, label string, tokens []statusToken) {
	face := basicfont.Face7x13
	labelColor := color.RGBA{190, 190, 190, 255}
	offColor := color.RGBA{120, 120, 120, 255}
	onColor := color.RGBA{0, 220, 90, 255}

	text.Draw(screen, label, face, x, baselineY, labelColor)
	cursorX := x + text.BoundString(face, label).Dx() + 6

	for _, token := range tokens {
		c := offColor
		if token.enabled {
			c = onColor
		}
		text.Draw(screen, token.name, face, cursorX, baselineY, c)
		cursorX += text.BoundString(face, token.name).Dx() + 8
	}
}

func (eo *EbitenOutput) drawRuntimeStatusBar(screen *ebiten.Image) {
	s := runtimeStatus.snapshot()
	playIE32, playIE64, playM68K, playZ80, playX86, play6502 := playbackCPUFlags(s)

	ie32On := (s.selectedCPU == runtimeCPUIE32 && s.ie32 != nil && s.ie32.IsRunning()) || playIE32
	ie64On := (s.selectedCPU == runtimeCPUIE64 && s.ie64 != nil && s.ie64.IsRunning()) || playIE64
	m68kOn := (s.selectedCPU == runtimeCPUM68K && s.m68k != nil && s.m68k.IsRunning()) || playM68K
	z80On := (s.selectedCPU == runtimeCPUZ80 && s.z80 != nil && s.z80.IsRunning()) || playZ80
	x86On := (s.selectedCPU == runtimeCPUX86 && s.x86 != nil && s.x86.IsRunning()) || playX86
	cpu65On := (s.selectedCPU == runtimeCPU6502 && s.cpu65 != nil && s.cpu65.IsRunning()) || play6502

	videoOn := s.video != nil && s.video.IsEnabled()
	vgaOn := s.vga != nil && s.vga.IsEnabled()
	ulaOn := s.ula != nil && s.ula.IsEnabled()
	tedVideoOn := s.tedVideo != nil && s.tedVideo.IsEnabled()
	anticOn := s.antic != nil && s.antic.IsEnabled()
	voodooOn := s.voodoo != nil && s.voodoo.IsEnabled()

	soundOn := s.sound != nil && s.sound.IsEnabled()
	psgOn := s.psgEngine != nil && s.psgEngine.IsPlaying()
	sidOn := s.sidEngine != nil && s.sidEngine.IsPlaying()
	pokeyOn := s.pokey != nil && s.pokey.IsPlaying()
	tedOn := s.tedEngine != nil && s.tedEngine.IsPlaying()
	ahxOn := s.ahxEngine != nil && s.ahxEngine.IsPlaying()

	barHeight := 44
	if barHeight >= eo.height {
		return
	}
	y := eo.height - barHeight
	ebitenutil.DrawRect(screen, 0, float64(y), float64(eo.width), float64(barHeight), color.RGBA{0, 0, 0, 180})

	drawStatusLine(screen, 6, y+13, "CPU  ", []statusToken{
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
	})
	drawStatusLine(screen, 6, y+26, "VIDEO", []statusToken{
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
	})
	drawStatusLine(screen, 6, y+39, "AUDIO", []statusToken{
		{name: "IESND", enabled: soundOn},
		{name: "|", enabled: false},
		{name: "PSG", enabled: psgOn},
		{name: "|", enabled: false},
		{name: "TED", enabled: tedOn},
		{name: "|", enabled: false},
		{name: "SID", enabled: sidOn},
		{name: "|", enabled: false},
		{name: "POKEY", enabled: pokeyOn},
		{name: "|", enabled: false},
		{name: "AHX", enabled: ahxOn},
	})

	legendColor := color.RGBA{160, 160, 160, 255}
	legend := "F8:Lua F9:Debug F10:Reset F11:FS/Win F12:Status"
	legendScale := 1.0
	legendW := int(float64(text.BoundString(basicfont.Face7x13, legend).Dx()) * legendScale)
	legendX := max(eo.width-legendW-6, 6)
	legendOpts := &ebiten.DrawImageOptions{}
	legendOpts.GeoM.Scale(legendScale, legendScale)
	legendOpts.GeoM.Translate(float64(legendX), float64(y+39))
	legendOpts.ColorScale.ScaleWithColor(legendColor)
	text.DrawWithOptions(screen, legend, basicfont.Face7x13, legendOpts)
}
