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
	done        chan struct{}
	keyHandler  func(byte)

	clipboardOnce sync.Once
	clipboardOK   bool
	showStatusBar bool

	hardResetHandler func()
	resetInProgress  atomic.Bool
}

func NewEbitenOutput() (VideoOutput, error) {
	return &EbitenOutput{
		width:         640,
		height:        480,
		format:        PixelFormatRGBA,
		scale:         1,
		windowedW:     640,
		windowedH:     480,
		frameBuffer:   make([]byte, 640*480*4),
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
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		eo.bufferMutex.Lock()
		eo.fullscreen = !eo.fullscreen
		ebiten.SetFullscreen(eo.fullscreen)
		if !eo.fullscreen {
			ebiten.SetWindowSize(eo.windowedW, eo.windowedH)
		}
		eo.bufferMutex.Unlock()
	}
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
	if inpututil.IsKeyJustPressed(ebiten.KeyF12) {
		eo.bufferMutex.Lock()
		eo.showStatusBar = !eo.showStatusBar
		eo.bufferMutex.Unlock()
	}
	eo.handleKeyboardInput()
	return nil
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
	legend := "F10 Reset  F11 Fullscreen  F12 Status Bar"
	legendScale := 1.0
	legendW := int(float64(text.BoundString(basicfont.Face7x13, legend).Dx()) * legendScale)
	legendX := max(eo.width-legendW-6, 6)
	legendOpts := &ebiten.DrawImageOptions{}
	legendOpts.GeoM.Scale(legendScale, legendScale)
	legendOpts.GeoM.Translate(float64(legendX), float64(y+39))
	legendOpts.ColorScale.ScaleWithColor(legendColor)
	text.DrawWithOptions(screen, legend, basicfont.Face7x13, legendOpts)
}
