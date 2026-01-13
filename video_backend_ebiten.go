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
	"sync"
	"time"
)

type EbitenOutput struct {
	running     bool
	window      *ebiten.Image
	width       int
	height      int
	format      PixelFormat
	frameBuffer []byte
	bufferMutex sync.RWMutex
	frameCount  uint64
	refreshRate int
	vsyncChan   chan struct{}
}

func NewEbitenOutput() (VideoOutput, error) {
	return &EbitenOutput{
		width:       640,
		height:      480,
		format:      PixelFormatRGBA,
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
	ebiten.SetWindowSize(eo.width, eo.height)
	ebiten.SetWindowTitle("Intuition Engine (c) 2024 - 2026 Zayn Otley")
	ebiten.SetWindowResizable(false)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetVsyncEnabled(true)

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

	eo.width = config.Width
	eo.height = config.Height
	eo.format = config.PixelFormat
	newSize := config.Width * config.Height * 4

	if len(eo.frameBuffer) != newSize {
		eo.frameBuffer = make([]byte, newSize)
	}

	ebiten.SetWindowSize(config.Width*config.Scale, config.Height*config.Scale)
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
		PixelFormat: eo.format,
		RefreshRate: eo.refreshRate,
		VSync:       true,
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
	return nil
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
