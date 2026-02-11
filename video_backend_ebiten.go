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
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.design/x/clipboard"
	"sync"
	"time"
)

type EbitenOutput struct {
	running     bool
	window      *ebiten.Image
	width       int
	height      int
	format      PixelFormat
	fullscreen  bool
	scale       int
	windowedW   int
	windowedH   int
	frameBuffer []byte
	bufferMutex sync.RWMutex
	frameCount  uint64
	refreshRate int
	vsyncChan   chan struct{}
	keyHandler  func(byte)

	clipboardOnce sync.Once
	clipboardOK   bool
}

func NewEbitenOutput() (VideoOutput, error) {
	return &EbitenOutput{
		width:       640,
		height:      480,
		format:      PixelFormatRGBA,
		scale:       1,
		windowedW:   640,
		windowedH:   480,
		frameBuffer: make([]byte, 640*480*4),
		refreshRate: 60,
		vsyncChan:   make(chan struct{}, 1),
	}, nil
}

func (eo *EbitenOutput) Start() error {
	if eo.running {
		return nil
	}
	eo.running = true
	ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
	ebiten.SetWindowTitle("Intuition Engine (c) 2024 - 2026 Zayn Otley")
	ebiten.SetWindowResizable(true)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetVsyncEnabled(true)
	if eo.fullscreen {
		ebiten.SetFullscreen(true)
	}

	go func() {
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
		width = 640
	}
	if height <= 0 {
		height = 480
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
	for dy := 0; dy < height; dy++ {
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
		if activeFrontend != nil && activeFrontend.cpu != nil {
			activeFrontend.cpu.Reset()
		}
		return ebiten.Termination
	}

	// Normal update path when window is open
	if !eo.running {
		return ebiten.Termination
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
	eo.handleKeyboardInput()
	return nil
}

func (eo *EbitenOutput) SetKeyHandler(fn func(byte)) {
	eo.bufferMutex.Lock()
	eo.keyHandler = fn
	eo.bufferMutex.Unlock()
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

func (eo *EbitenOutput) handleKeyboardInput() {
	eo.bufferMutex.RLock()
	hasHandler := eo.keyHandler != nil
	eo.bufferMutex.RUnlock()
	if !hasHandler {
		return
	}

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)

	// Clipboard paste: Ctrl+Shift+V
	if ctrl && shift && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		eo.handleClipboardPaste()
	}
	// Ctrl+Shift+C intentionally reserved for future copy/selection support.

	// Printable input path.
	for _, r := range ebiten.AppendInputChars(nil) {
		if b, ok := runeToInputByte(r); ok {
			eo.emitByte(b)
		}
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
	}
	for _, key := range specialKeys {
		if inpututil.IsKeyJustPressed(key) {
			if seq, ok := translateSpecialKey(key); ok {
				eo.emitSeq(seq)
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
	default:
		return nil, false
	}
}

func normalizePasteText(raw []byte) []byte {
	norm := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\r' {
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			norm = append(norm, '\n')
			continue
		}
		norm = append(norm, raw[i])
	}
	return norm
}

func capPasteText(raw []byte, max int) []byte {
	if len(raw) <= max {
		return raw
	}
	return raw[:max]
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
	if eo.window == nil {
		eo.window = ebiten.NewImage(eo.width, eo.height)
	}

	eo.bufferMutex.RLock()
	eo.window.WritePixels(eo.frameBuffer)
	eo.bufferMutex.RUnlock()
	screen.DrawImage(eo.window, nil)

	eo.frameCount++
	select {
	case eo.vsyncChan <- struct{}{}:
	default:
	}
}

func (eo *EbitenOutput) Layout(_, _ int) (int, int) {
	return eo.width, eo.height
}
