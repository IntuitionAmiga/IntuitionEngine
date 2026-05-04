//go:build !headless

package main

import (
	"sync"

	"github.com/intuitionamiga/IntuitionEngine/internal/clipboard"
)

// ClipboardBridge provides MMIO-based clipboard exchange between host and guest.
// Guest AROS applications read/write clipboard data via MMIO registers.
type ClipboardBridge struct {
	bus       *MachineBus
	dataPtr   uint32
	dataLen   uint32
	status    uint32
	resultLen uint32
	format    uint32
	initOnce  sync.Once
	initOK    bool
}

func NewClipboardBridge(bus *MachineBus) *ClipboardBridge {
	return &ClipboardBridge{
		bus: bus,
	}
}

func (cb *ClipboardBridge) ensureInit() bool {
	cb.initOnce.Do(func() {
		cb.initOK = clipboard.Init() == nil
	})
	return cb.initOK
}

func (cb *ClipboardBridge) HandleRead(addr uint32) uint32 {
	switch addr {
	case CLIP_DATA_PTR:
		return cb.dataPtr
	case CLIP_DATA_LEN:
		return cb.dataLen
	case CLIP_STATUS:
		return cb.status
	case CLIP_RESULT_LEN:
		return cb.resultLen
	case CLIP_FORMAT:
		return cb.format
	}
	return 0
}

func (cb *ClipboardBridge) HandleWrite(addr uint32, val uint32) {
	switch addr {
	case CLIP_DATA_PTR:
		cb.dataPtr = val
	case CLIP_DATA_LEN:
		cb.dataLen = val
	case CLIP_FORMAT:
		cb.format = val
	case CLIP_CTRL:
		switch val {
		case CLIP_CMD_READ:
			cb.doRead()
		case CLIP_CMD_WRITE:
			cb.doWrite()
		}
	}
}

// doRead reads from the host clipboard and writes data into guest RAM.
func (cb *ClipboardBridge) doRead() {
	cb.status = CLIP_STATUS_BUSY
	cb.resultLen = 0

	if !cb.ensureInit() {
		cb.status = CLIP_STATUS_ERROR
		return
	}

	data, _ := clipboard.ReadText()
	if len(data) == 0 {
		cb.status = CLIP_STATUS_EMPTY
		return
	}

	ptr := cb.dataPtr
	maxLen := cb.dataLen
	copyLen := uint32(len(data))
	if copyLen > maxLen {
		copyLen = maxLen
	}

	if cb.bus != nil && clipboardBoundsOK(ptr, copyLen, uint32(len(cb.bus.memory))) {
		copy(cb.bus.memory[ptr:ptr+copyLen], data[:copyLen])
	} else {
		cb.status = CLIP_STATUS_ERROR
		return
	}

	cb.resultLen = copyLen
	cb.status = CLIP_STATUS_READY
}

// doWrite reads data from guest RAM and writes it to the host clipboard.
func (cb *ClipboardBridge) doWrite() {
	cb.status = CLIP_STATUS_BUSY
	cb.resultLen = 0

	if !cb.ensureInit() {
		cb.status = CLIP_STATUS_ERROR
		return
	}

	ptr := cb.dataPtr
	dataLen := cb.dataLen

	if dataLen == 0 {
		cb.status = CLIP_STATUS_READY
		return
	}

	if cb.bus == nil || !clipboardBoundsOK(ptr, dataLen, uint32(len(cb.bus.memory))) {
		cb.status = CLIP_STATUS_ERROR
		return
	}

	data := make([]byte, dataLen)
	copy(data, cb.bus.memory[ptr:ptr+dataLen])

	_ = clipboard.WriteText(data)

	cb.resultLen = dataLen
	cb.status = CLIP_STATUS_READY
}

func (cb *ClipboardBridge) Close() {}
