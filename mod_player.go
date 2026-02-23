package main

import (
	"sync"
)

// MODPlayer provides memory-mapped I/O control of MOD playback.
// Pattern follows AHXPlayer: async loading with generation counting.
type MODPlayer struct {
	engine *MODEngine

	// I/O register state
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

// NewMODPlayer creates a new MOD player bound to a SoundChip.
func NewMODPlayer(sound *SoundChip, sampleRate int) *MODPlayer {
	return &MODPlayer{
		engine: NewMODEngine(sound, sampleRate),
	}
}

// Load parses MOD data and starts playback.
func (p *MODPlayer) Load(data []byte) error {
	mod, err := ParseMOD(data)
	if err != nil {
		return err
	}
	p.engine.LoadMOD(mod)
	return nil
}

// Play starts playback.
func (p *MODPlayer) Play() {
	p.engine.SetPlaying(true)
}

// Stop stops playback.
func (p *MODPlayer) Stop() {
	p.mu.Lock()
	p.playGen++
	p.playBusy = false
	p.mu.Unlock()
	p.engine.SetPlaying(false)
}

// IsPlaying returns true if playing.
func (p *MODPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

// SetLoop enables/disables looping.
func (p *MODPlayer) SetLoop(loop bool) {
	p.engine.SetLoop(loop)
}

// AttachBus attaches the memory bus for reading MOD data from bus memory.
func (p *MODPlayer) AttachBus(bus Bus32) {
	p.bus = bus
}

// HandlePlayWrite handles writes to MOD_PLAY_* registers.
func (p *MODPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *modAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case MOD_PLAY_PTR:
		p.playPtrStaged = value
	case MOD_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 1)
	case MOD_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Word(p.playPtrStaged, value, 2)
	case MOD_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 3)
	case MOD_PLAY_LEN:
		p.playLenStaged = value
	case MOD_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 1)
	case MOD_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Word(p.playLenStaged, value, 2)
	case MOD_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 3)
	case MOD_PLAY_CTRL:
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
		startReq = &modAsyncStartRequest{
			gen:       p.playGen,
			data:      data,
			forceLoop: p.forceLoop,
		}
	case MOD_PLAY_CTRL + 1, MOD_PLAY_CTRL + 2, MOD_PLAY_CTRL + 3:
		// Ignore upper bytes of control register
	case MOD_FILTER_MODEL:
		p.engine.SetFilterModel(int(value))
	case MOD_FILTER_MODEL + 1, MOD_FILTER_MODEL + 2, MOD_FILTER_MODEL + 3:
		// Ignore upper bytes
	default:
		break
	}
	p.mu.Unlock()

	if stopPlayback {
		p.engine.SetPlaying(false)
	}
	if startReq != nil {
		go p.startAsync(*startReq)
	}
}

type modAsyncStartRequest struct {
	gen       uint64
	data      []byte
	forceLoop bool
}

func (p *MODPlayer) startAsync(req modAsyncStartRequest) {
	mod, err := ParseMOD(req.data)

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
	p.engine.LoadMOD(mod)
	p.engine.SetLoop(req.forceLoop)

	// Register as sample ticker
	if p.engine.sound != nil {
		p.engine.sound.SetSampleTicker(p.engine)
	}

	p.engine.SetPlaying(true)
}

// HandlePlayRead handles reads from MOD_PLAY_* registers.
func (p *MODPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case MOD_PLAY_PTR:
		return p.playPtrStaged
	case MOD_PLAY_PTR + 1:
		return readUint32Byte(p.playPtrStaged, 1)
	case MOD_PLAY_PTR + 2:
		return readUint32Byte(p.playPtrStaged, 2)
	case MOD_PLAY_PTR + 3:
		return readUint32Byte(p.playPtrStaged, 3)
	case MOD_PLAY_LEN:
		return p.playLenStaged
	case MOD_PLAY_LEN + 1:
		return readUint32Byte(p.playLenStaged, 1)
	case MOD_PLAY_LEN + 2:
		return readUint32Byte(p.playLenStaged, 2)
	case MOD_PLAY_LEN + 3:
		return readUint32Byte(p.playLenStaged, 3)
	case MOD_PLAY_CTRL:
		return p.playCtrlStatus()
	case MOD_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case MOD_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case MOD_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	case MOD_PLAY_STATUS:
		return p.playStatus()
	case MOD_PLAY_STATUS + 1:
		return readUint32Byte(p.playStatus(), 1)
	case MOD_PLAY_STATUS + 2:
		return readUint32Byte(p.playStatus(), 2)
	case MOD_PLAY_STATUS + 3:
		return readUint32Byte(p.playStatus(), 3)
	case MOD_FILTER_MODEL:
		return uint32(p.engine.filterModel)
	case MOD_POSITION:
		return uint32(p.engine.GetPosition())
	case MOD_POSITION + 1:
		return readUint32Byte(uint32(p.engine.GetPosition()), 1)
	case MOD_POSITION + 2:
		return readUint32Byte(uint32(p.engine.GetPosition()), 2)
	case MOD_POSITION + 3:
		return readUint32Byte(uint32(p.engine.GetPosition()), 3)
	default:
		return 0
	}
}

func (p *MODPlayer) playCtrlStatus() uint32 {
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

func (p *MODPlayer) playStatus() uint32 {
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
