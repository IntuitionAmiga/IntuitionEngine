package main

import "sync"

// WAVPlayer provides memory-mapped I/O control of WAV playback.
type WAVPlayer struct {
	PlayerControlState

	engine  *WAVEngine
	playGen uint64

	channelBase uint8
	volumeL     uint8
	volumeR     uint8
	flags       uint8
	paused      bool

	inflight bool
	pending  *wavAsyncStartRequest
	mu       sync.Mutex
}

// NewWAVPlayer creates a new WAV player bound to a SoundChip.
func NewWAVPlayer(sound *SoundChip, sampleRate int) *WAVPlayer {
	p := &WAVPlayer{
		engine:  NewWAVEngine(sound, sampleRate),
		volumeL: 255,
		volumeR: 255,
		flags:   1,
	}
	p.applyEngineConfig()
	return p
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

func (p *WAVPlayer) Play() { p.engine.SetPlaying(true) }

func (p *WAVPlayer) Stop() {
	p.mu.Lock()
	p.playGen++
	p.PlayBusy = false
	p.pending = nil
	p.mu.Unlock()
	p.engine.SetPlaying(false)
}

func (p *WAVPlayer) IsPlaying() bool { return p.engine.IsPlaying() }

func (p *WAVPlayer) SetLoop(loop bool) { p.engine.SetLoop(loop) }

func (p *WAVPlayer) AttachBus(bus Bus32) { p.Bus = bus }

func (p *WAVPlayer) Reset() {
	p.Stop()
	p.mu.Lock()
	p.PlayerControlState = PlayerControlState{Bus: p.Bus}
	p.channelBase = 0
	p.volumeL = 255
	p.volumeR = 255
	p.flags = 1
	p.paused = false
	p.mu.Unlock()
	p.engine.Reset()
	p.applyEngineConfig()
}

// HandlePlayWrite handles writes to WAV_PLAY_* registers.
func (p *WAVPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var enqueue *wavAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case WAV_PLAY_PTR:
		p.HandlePtrWrite(0, value)
	case WAV_PLAY_PTR + 1:
		p.HandlePtrWrite(1, value)
	case WAV_PLAY_PTR + 2:
		p.HandlePtrWordWrite(2, value)
	case WAV_PLAY_PTR + 3:
		p.HandlePtrWrite(3, value)
	case WAV_PLAY_LEN:
		p.HandleLenWrite(0, value)
	case WAV_PLAY_LEN + 1:
		p.HandleLenWrite(1, value)
	case WAV_PLAY_LEN + 2:
		p.HandleLenWordWrite(2, value)
	case WAV_PLAY_LEN + 3:
		p.HandleLenWrite(3, value)
	case WAV_PLAY_PTR_HI:
		p.PlayPtrHigh = value
	case WAV_PLAY_PTR_HI + 1:
		p.PlayPtrHigh = writeUint32Byte(p.PlayPtrHigh, value, 1)
	case WAV_PLAY_PTR_HI + 2:
		p.PlayPtrHigh = writeUint32Word(p.PlayPtrHigh, value, 2)
	case WAV_PLAY_PTR_HI + 3:
		p.PlayPtrHigh = writeUint32Byte(p.PlayPtrHigh, value, 3)
	case WAV_CHANNEL_BASE:
		p.channelBase = uint8(value)
		p.applyEngineConfigLocked()
	case WAV_VOLUME_L:
		p.volumeL = uint8(value)
		p.applyEngineConfigLocked()
	case WAV_VOLUME_R:
		p.volumeR = uint8(value)
		p.applyEngineConfigLocked()
	case WAV_FLAGS:
		p.flags = uint8(value)
		p.applyEngineConfigLocked()
	case WAV_PLAY_CTRL:
		if value&0x2 != 0 {
			p.playGen++
			p.PlayBusy = false
			p.pending = nil
			stopPlayback = true
			break
		}
		if value&0x8 != 0 {
			p.paused = true
			p.engine.SetPaused(true)
		} else {
			p.paused = false
			p.engine.SetPaused(false)
		}
		if value&0x10 != 0 {
			p.ForceLoop = value&0x4 != 0
			p.engine.SetLoop(p.ForceLoop)
			break
		}
		if value&0x1 == 0 {
			break
		}
		p.playGen++
		if errText := p.PreparePlay64(p.PlayPtrHigh, value&0x4 != 0); errText != "" {
			break
		}
		data, err := p.ReadDataFromBus()
		if err != nil {
			p.SetError()
			break
		}
		p.PlayBusy = true
		p.ClearError()
		enqueue = &wavAsyncStartRequest{
			gen:         p.playGen,
			data:        data,
			forceLoop:   p.ForceLoop,
			channelBase: p.channelBase,
			volumeL:     p.volumeL,
			volumeR:     p.volumeR,
			flags:       p.flags,
		}
	case WAV_PLAY_CTRL + 1, WAV_PLAY_CTRL + 2, WAV_PLAY_CTRL + 3:
	}
	p.mu.Unlock()

	if stopPlayback {
		p.engine.SetPlaying(false)
	}
	if enqueue != nil {
		p.enqueueStart(*enqueue)
	}
}

type wavAsyncStartRequest struct {
	gen         uint64
	data        []byte
	forceLoop   bool
	channelBase uint8
	volumeL     uint8
	volumeR     uint8
	flags       uint8
}

func (p *WAVPlayer) enqueueStart(req wavAsyncStartRequest) {
	p.mu.Lock()
	if p.inflight {
		p.pending = &req
		p.mu.Unlock()
		return
	}
	p.inflight = true
	p.mu.Unlock()
	go p.startAsync(req)
}

func (p *WAVPlayer) startAsync(req wavAsyncStartRequest) {
	for {
		wav, err := ParseWAV(req.data)
		p.mu.Lock()
		if req.gen == p.playGen {
			if err != nil {
				p.SetError()
			} else {
				p.engine.SetPlaying(false)
				p.engine.LoadWAV(wav)
				p.engine.SetLoop(req.forceLoop)
				p.engine.SetChannelBase(int(req.channelBase))
				p.engine.SetVolume(req.volumeL, req.volumeR)
				p.engine.SetForceMono(req.flags&1 != 0)
				p.engine.SetPaused(p.paused)
				p.engine.SetPlaying(true)
				p.PlayBusy = false
			}
		}
		if p.pending == nil {
			p.inflight = false
			p.mu.Unlock()
			return
		}
		req = *p.pending
		p.pending = nil
		p.mu.Unlock()
	}
}

// HandlePlayRead handles reads from WAV_PLAY_* registers.
func (p *WAVPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case WAV_PLAY_PTR, WAV_PLAY_PTR + 1, WAV_PLAY_PTR + 2, WAV_PLAY_PTR + 3:
		return p.ReadPtrByte(addr - WAV_PLAY_PTR)
	case WAV_PLAY_LEN, WAV_PLAY_LEN + 1, WAV_PLAY_LEN + 2, WAV_PLAY_LEN + 3:
		return p.ReadLenByte(addr - WAV_PLAY_LEN)
	case WAV_PLAY_PTR_HI:
		return p.PlayPtrHigh
	case WAV_PLAY_PTR_HI + 1:
		return readUint32Byte(p.PlayPtrHigh, 1)
	case WAV_PLAY_PTR_HI + 2:
		return readUint32Byte(p.PlayPtrHigh, 2)
	case WAV_PLAY_PTR_HI + 3:
		return readUint32Byte(p.PlayPtrHigh, 3)
	case WAV_PLAY_CTRL, WAV_PLAY_CTRL + 1, WAV_PLAY_CTRL + 2, WAV_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), addr-WAV_PLAY_CTRL)
	case WAV_PLAY_STATUS, WAV_PLAY_STATUS + 1, WAV_PLAY_STATUS + 2, WAV_PLAY_STATUS + 3:
		return readUint32Byte(p.playStatus(), addr-WAV_PLAY_STATUS)
	case WAV_POSITION, WAV_POSITION + 1, WAV_POSITION + 2, WAV_POSITION + 3:
		return readUint32Byte(uint32(p.engine.GetPosition()), addr-WAV_POSITION)
	case WAV_CHANNEL_BASE:
		return uint32(p.channelBase)
	case WAV_VOLUME_L:
		return uint32(p.volumeL)
	case WAV_VOLUME_R:
		return uint32(p.volumeR)
	case WAV_FLAGS:
		return uint32(p.flags)
	default:
		return 0
	}
}

func (p *WAVPlayer) playCtrlStatus() uint32 {
	var ctrl uint32
	if p.PlayBusy || p.engine.IsPlaying() {
		ctrl |= 1
	}
	if p.ForceLoop {
		ctrl |= 4
	}
	if p.paused {
		ctrl |= 8
	}
	return ctrl
}

func (p *WAVPlayer) playStatus() uint32 {
	var status uint32
	if p.PlayBusy || p.engine.IsPlaying() {
		status |= 1
	}
	if p.PlayErr {
		status |= 2
	}
	if p.paused {
		status |= 4
	}
	if p.engine.StereoActive() {
		status |= 8
	}
	return status
}

func (p *WAVPlayer) applyEngineConfig() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.applyEngineConfigLocked()
}

func (p *WAVPlayer) applyEngineConfigLocked() {
	p.engine.SetChannelBase(int(p.channelBase))
	p.engine.SetVolume(p.volumeL, p.volumeR)
	p.engine.SetForceMono(p.flags&1 != 0)
}
