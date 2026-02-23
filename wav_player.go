package main

import (
	"sync"
)

// WAVPlayer provides memory-mapped I/O control of WAV playback.
// Pattern follows MODPlayer: async loading with generation counting.
type WAVPlayer struct {
	engine *WAVEngine

	bus           Bus32
	playPtrStaged uint32
	playLenStaged uint32
	playPtr       uint32
	playLen       uint32
	playBusy      bool
	playErr       bool
	forceLoop     bool
	playGen       uint64

	mu sync.Mutex
}

// NewWAVPlayer creates a new WAV player bound to a SoundChip.
func NewWAVPlayer(sound *SoundChip, sampleRate int) *WAVPlayer {
	return &WAVPlayer{
		engine: NewWAVEngine(sound, sampleRate),
	}
}

// Load parses WAV data and starts playback.
func (p *WAVPlayer) Load(data []byte) error {
	wav, err := ParseWAV(data)
	if err != nil {
		return err
	}
	p.engine.LoadWAV(wav)
	return nil
}

// Play starts playback.
func (p *WAVPlayer) Play() {
	p.engine.SetPlaying(true)
}

// Stop stops playback.
func (p *WAVPlayer) Stop() {
	p.mu.Lock()
	p.playGen++
	p.playBusy = false
	p.mu.Unlock()
	p.engine.SetPlaying(false)
}

// IsPlaying returns true if playing.
func (p *WAVPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

// SetLoop enables/disables looping.
func (p *WAVPlayer) SetLoop(loop bool) {
	p.engine.SetLoop(loop)
}

// AttachBus attaches the memory bus for reading WAV data from bus memory.
func (p *WAVPlayer) AttachBus(bus Bus32) {
	p.bus = bus
}

// Reset clears player state.
func (p *WAVPlayer) Reset() {
	p.Stop()
	p.mu.Lock()
	p.playPtrStaged = 0
	p.playLenStaged = 0
	p.playPtr = 0
	p.playLen = 0
	p.playBusy = false
	p.playErr = false
	p.forceLoop = false
	p.mu.Unlock()
	p.engine.Reset()
}

// HandlePlayWrite handles writes to WAV_PLAY_* registers.
func (p *WAVPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *wavAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case WAV_PLAY_PTR:
		p.playPtrStaged = value
	case WAV_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 1)
	case WAV_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Word(p.playPtrStaged, value, 2)
	case WAV_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 3)
	case WAV_PLAY_LEN:
		p.playLenStaged = value
	case WAV_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 1)
	case WAV_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Word(p.playLenStaged, value, 2)
	case WAV_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 3)
	case WAV_PLAY_CTRL:
		if value&0x2 != 0 {
			p.playGen++
			p.playBusy = false
			p.playErr = false
			stopPlayback = true
			break
		}
		if value&0x1 == 0 {
			break
		}
		if p.playBusy {
			break
		}
		p.playPtr = p.playPtrStaged
		p.playLen = p.playLenStaged
		p.forceLoop = (value & 0x4) != 0
		p.playErr = false
		if p.bus == nil {
			p.playErr = true
			break
		}
		if p.playLen == 0 {
			p.playErr = true
			break
		}
		mem := p.bus.GetMemory()
		if int(p.playPtr)+int(p.playLen) > len(mem) {
			p.playErr = true
			break
		}
		data := make([]byte, p.playLen)
		copy(data, mem[p.playPtr:p.playPtr+p.playLen])
		p.playBusy = true
		p.playGen++
		startReq = &wavAsyncStartRequest{
			gen:       p.playGen,
			data:      data,
			forceLoop: p.forceLoop,
		}
	case WAV_PLAY_CTRL + 1, WAV_PLAY_CTRL + 2, WAV_PLAY_CTRL + 3:
		// Ignore upper bytes of control register
	}
	p.mu.Unlock()

	if stopPlayback {
		p.engine.SetPlaying(false)
	}
	if startReq != nil {
		go p.startAsync(*startReq)
	}
}

type wavAsyncStartRequest struct {
	gen       uint64
	data      []byte
	forceLoop bool
}

func (p *WAVPlayer) startAsync(req wavAsyncStartRequest) {
	wav, err := ParseWAV(req.data)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Generation guard: ignore if CPU issued stop/new-start while parsing
	if req.gen != p.playGen {
		return
	}
	if err != nil {
		p.playErr = true
		p.playBusy = false
		return
	}

	p.engine.SetPlaying(false)
	p.engine.LoadWAV(wav)
	p.engine.SetLoop(req.forceLoop)

	// Register as sample ticker
	if p.engine.sound != nil {
		p.engine.sound.SetSampleTicker(p.engine)
	}

	p.engine.SetPlaying(true)
}

// HandlePlayRead handles reads from WAV_PLAY_* registers.
func (p *WAVPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case WAV_PLAY_PTR:
		return p.playPtrStaged
	case WAV_PLAY_PTR + 1:
		return readUint32Byte(p.playPtrStaged, 1)
	case WAV_PLAY_PTR + 2:
		return readUint32Byte(p.playPtrStaged, 2)
	case WAV_PLAY_PTR + 3:
		return readUint32Byte(p.playPtrStaged, 3)
	case WAV_PLAY_LEN:
		return p.playLenStaged
	case WAV_PLAY_LEN + 1:
		return readUint32Byte(p.playLenStaged, 1)
	case WAV_PLAY_LEN + 2:
		return readUint32Byte(p.playLenStaged, 2)
	case WAV_PLAY_LEN + 3:
		return readUint32Byte(p.playLenStaged, 3)
	case WAV_PLAY_CTRL:
		return p.playCtrlStatus()
	case WAV_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case WAV_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case WAV_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	case WAV_PLAY_STATUS:
		return p.playStatus()
	case WAV_PLAY_STATUS + 1:
		return readUint32Byte(p.playStatus(), 1)
	case WAV_PLAY_STATUS + 2:
		return readUint32Byte(p.playStatus(), 2)
	case WAV_PLAY_STATUS + 3:
		return readUint32Byte(p.playStatus(), 3)
	case WAV_POSITION:
		return uint32(p.engine.GetPosition())
	case WAV_POSITION + 1:
		return readUint32Byte(uint32(p.engine.GetPosition()), 1)
	case WAV_POSITION + 2:
		return readUint32Byte(uint32(p.engine.GetPosition()), 2)
	case WAV_POSITION + 3:
		return readUint32Byte(uint32(p.engine.GetPosition()), 3)
	default:
		return 0
	}
}

func (p *WAVPlayer) playCtrlStatus() uint32 {
	ctrl := uint32(0)
	busy := p.playBusy
	if busy && !p.IsPlaying() {
		p.playBusy = false
		busy = false
	}
	if busy {
		ctrl |= 0x1
	}
	if p.forceLoop {
		ctrl |= 0x4
	}
	return ctrl
}

func (p *WAVPlayer) playStatus() uint32 {
	status := uint32(0)
	busy := p.playBusy
	if busy && !p.IsPlaying() {
		p.playBusy = false
		busy = false
	}
	if busy {
		status |= 0x1
	}
	if p.playErr {
		status |= 0x2
	}
	return status
}
