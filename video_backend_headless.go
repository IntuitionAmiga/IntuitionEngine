//go:build headless

package main

import "sync/atomic"

type HeadlessVideoOutput struct {
	started     bool
	config      DisplayConfig
	frameCount  uint64
	refreshRate int
}

func NewEbitenOutput() (VideoOutput, error) {
	return &HeadlessVideoOutput{refreshRate: 60}, nil
}

func NewOpenGLOutput() (VideoOutput, error) {
	return &HeadlessVideoOutput{refreshRate: 60}, nil
}

func (h *HeadlessVideoOutput) Start() error {
	h.started = true
	return nil
}

func (h *HeadlessVideoOutput) Stop() error {
	h.started = false
	return nil
}

func (h *HeadlessVideoOutput) Close() error {
	h.started = false
	return nil
}

func (h *HeadlessVideoOutput) IsStarted() bool {
	return h.started
}

func (h *HeadlessVideoOutput) SetDisplayConfig(config DisplayConfig) error {
	h.config = config
	return nil
}

func (h *HeadlessVideoOutput) GetDisplayConfig() DisplayConfig {
	return h.config
}

func (h *HeadlessVideoOutput) UpdateFrame(buffer []byte) error {
	atomic.AddUint64(&h.frameCount, 1)
	return nil
}

func (h *HeadlessVideoOutput) WaitForVSync() error {
	return nil
}

func (h *HeadlessVideoOutput) GetFrameCount() uint64 {
	return atomic.LoadUint64(&h.frameCount)
}

func (h *HeadlessVideoOutput) GetRefreshRate() int {
	if h.refreshRate == 0 {
		return 60
	}
	return h.refreshRate
}
