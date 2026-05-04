//go:build headless

package main

// ClipboardBridge is a headless stub — reads return CLIP_STATUS_EMPTY, writes are accepted silently.
type ClipboardBridge struct {
	bus       *MachineBus
	dataPtr   uint32
	dataLen   uint32
	status    uint32
	resultLen uint32
	format    uint32
}

func NewClipboardBridge(bus *MachineBus) *ClipboardBridge {
	return &ClipboardBridge{
		bus: bus,
	}
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
			// Headless: clipboard is always empty
			cb.resultLen = 0
			cb.status = CLIP_STATUS_EMPTY
		case CLIP_CMD_WRITE:
			// Headless: accept write silently
			cb.resultLen = cb.dataLen
			cb.status = CLIP_STATUS_READY
		}
	}
}

func (cb *ClipboardBridge) Close() {}
