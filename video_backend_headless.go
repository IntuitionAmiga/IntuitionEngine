//go:build headless

package main

import (
	"sync"
	"sync/atomic"
)

func init() {
	compiledFeatures = append(compiledFeatures, "video:headless")
}

type HeadlessVideoOutput struct {
	started     atomic.Bool
	configMu    sync.RWMutex
	config      DisplayConfig
	frameCount  atomic.Uint64
	refreshRate int
}

func NewEbitenOutput() (VideoOutput, error) {
	return &HeadlessVideoOutput{
		config: DisplayConfig{
			Width:       DefaultScreenWidth,
			Height:      DefaultScreenHeight,
			PixelFormat: PixelFormatRGBA,
			RefreshRate: 60,
			Scale:       1,
		},
		refreshRate: 60,
	}, nil
}

func (h *HeadlessVideoOutput) Start() error {
	h.started.Store(true)
	return nil
}

func (h *HeadlessVideoOutput) Stop() error {
	h.started.Store(false)
	return nil
}

func (h *HeadlessVideoOutput) Close() error {
	h.started.Store(false)
	return nil
}

func (h *HeadlessVideoOutput) IsStarted() bool {
	return h.started.Load()
}

func (h *HeadlessVideoOutput) SetDisplayConfig(config DisplayConfig) error {
	h.configMu.Lock()
	defer h.configMu.Unlock()
	h.config = config
	return nil
}

func (h *HeadlessVideoOutput) GetDisplayConfig() DisplayConfig {
	h.configMu.RLock()
	defer h.configMu.RUnlock()
	return h.config
}

func (h *HeadlessVideoOutput) UpdateFrame(buffer []byte) error {
	h.configMu.RLock()
	width := h.config.Width
	height := h.config.Height
	h.configMu.RUnlock()
	if err := validateFrameSize(width, height, buffer); err != nil {
		return err
	}
	h.frameCount.Add(1)
	return nil
}

func (h *HeadlessVideoOutput) WaitForVSync() error {
	return nil
}

func (h *HeadlessVideoOutput) GetFrameCount() uint64 {
	return h.frameCount.Load()
}

func (h *HeadlessVideoOutput) GetRefreshRate() int {
	if h.refreshRate == 0 {
		return 60
	}
	return h.refreshRate
}
