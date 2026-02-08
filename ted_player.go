// ted_player.go - High-level TED music player for .ted file playback

/*
TEDPlayer provides a high-level interface for playing TED music files.
It handles file loading, metadata extraction, and memory-mapped playback control.

Usage:
  player := NewTEDPlayer(engine)
  player.Load("music.ted")
  player.Play()

Memory-mapped registers allow control from running programs:
  TED_PLAY_PTR    - Pointer to TED data in memory
  TED_PLAY_LEN    - Length of TED data
  TED_PLAY_CTRL   - Control: bit 0=start, bit 1=stop, bit 2=loop
  TED_PLAY_STATUS - Status: bit 0=busy, bit 1=error
*/

package main

import (
	"fmt"
	"os"
	"sync"
)

const (
	tedDefaultLoopFrames = 3000  // ~60 seconds at 50Hz (enough for song patterns)
	tedMaxFrames         = 30000 // ~10 minutes at 50Hz
)

// TEDPlayerMetadata contains metadata from a TED file
type TEDPlayerMetadata struct {
	Title  string
	Author string
	Date   string
	Tool   string
}

// TEDPlayer provides high-level TED music playback
type TEDPlayer struct {
	engine   *TEDEngine
	bus      Bus32
	metadata TEDPlayerMetadata
	clockHz  uint32
	loop     bool

	// Playback control state
	playPtrStaged uint32
	playLenStaged uint32
	playPtr       uint32
	playLen       uint32
	playBusy      bool
	playErr       bool
	forceLoop     bool
	playGen       uint64

	mu sync.Mutex

	renderInstructions uint64
	renderCPU          string
	renderExecNanos    uint64
}

// NewTEDPlayer creates a new TED player
func NewTEDPlayer(engine *TEDEngine) *TEDPlayer {
	return &TEDPlayer{engine: engine}
}

// Load loads a TED file from disk
func (p *TEDPlayer) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read TED file: %w", err)
	}
	return p.LoadData(data)
}

// LoadData loads a TED file from raw data
func (p *TEDPlayer) LoadData(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.engine != nil {
		p.engine.StopPlayback()
	}

	meta, events, totalSamples, clockHz, loop, loopSample, instrCount, execNanos, err := renderTEDWithLimit(data, p.engine.sampleRate, 0)
	if err != nil {
		return err
	}

	p.metadata = meta
	p.clockHz = clockHz
	p.loop = loop
	p.renderInstructions = instrCount
	p.renderCPU = "6502"
	p.renderExecNanos = execNanos

	if p.engine != nil {
		p.engine.SetClockHz(clockHz)
		p.engine.SetEvents(events, totalSamples, loop, loopSample)
	}

	return nil
}

// renderTEDWithLimit renders a TED file to events
func renderTEDWithLimit(data []byte, sampleRate int, maxFrames int) (TEDPlayerMetadata, []TEDEvent, uint64, uint32, bool, uint64, uint64, uint64, error) {
	file, err := parseTEDFile(data)
	if err != nil {
		return TEDPlayerMetadata{}, nil, 0, 0, false, 0, 0, 0, fmt.Errorf("parse TED: %w", err)
	}

	player, err := NewTED6502Player(nil, sampleRate)
	if err != nil {
		return TEDPlayerMetadata{}, nil, 0, 0, false, 0, 0, 0, fmt.Errorf("create player: %w", err)
	}

	if err := player.LoadFromData(data); err != nil {
		return TEDPlayerMetadata{}, nil, 0, 0, false, 0, 0, 0, fmt.Errorf("load data: %w", err)
	}

	frameCount := tedDefaultLoopFrames
	loop := true
	loopSample := uint64(0)

	if maxFrames > 0 && frameCount > maxFrames {
		frameCount = maxFrames
	}
	if frameCount > tedMaxFrames {
		frameCount = tedMaxFrames
	}

	// Render frames
	var allEvents []TEDEvent
	for i := 0; i < frameCount; i++ {
		events, err := player.RenderFrame()
		if err != nil {
			break
		}
		allEvents = append(allEvents, events...)
	}

	totalSamples := player.GetTotalSamples()

	meta := TEDPlayerMetadata{
		Title:  file.Title,
		Author: file.Author,
		Date:   file.Date,
		Tool:   file.Tool,
	}

	clockHz := player.GetClockHz()

	return meta, allEvents, totalSamples, clockHz, loop, loopSample, player.instructionCount, player.cpuExecNanos, nil
}

// Play starts playback
func (p *TEDPlayer) Play() {
	if p.engine != nil {
		p.engine.SetPlaying(true)
	}
}

// Stop stops playback
func (p *TEDPlayer) Stop() {
	p.mu.Lock()
	p.playGen++
	p.playBusy = false
	p.mu.Unlock()
	if p.engine != nil {
		p.engine.StopPlayback()
	}
}

// IsPlaying returns true if playback is active
func (p *TEDPlayer) IsPlaying() bool {
	if p.engine != nil {
		return p.engine.IsPlaying()
	}
	return false
}

// Metadata returns the loaded file's metadata
func (p *TEDPlayer) Metadata() TEDPlayerMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metadata
}

// DurationSeconds returns the duration in seconds
func (p *TEDPlayer) RenderPerf() (uint64, string, uint64) {
	return p.renderInstructions, p.renderCPU, p.renderExecNanos
}

func (p *TEDPlayer) DurationSeconds() float64 {
	if p.engine == nil {
		return 0
	}
	p.engine.mutex.Lock()
	defer p.engine.mutex.Unlock()
	if p.engine.totalSamples == 0 {
		return 0
	}
	return float64(p.engine.totalSamples) / float64(p.engine.sampleRate)
}

// DurationText returns a formatted duration string
// TED files loop forever so we don't display a duration
func (p *TEDPlayer) DurationText() string {
	// TED files don't have duration metadata and loop indefinitely
	// Return empty string to avoid showing misleading buffer-based duration
	return ""
}

// AttachBus attaches a memory bus for reading embedded TED data
func (p *TEDPlayer) AttachBus(bus Bus32) {
	p.bus = bus
}

// HandlePlayWrite handles writes to TED_PLAY_* registers
func (p *TEDPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *tedAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case TED_PLAY_PTR:
		p.playPtrStaged = value
	case TED_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 1)
	case TED_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Word(p.playPtrStaged, value, 2)
	case TED_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 3)
	case TED_PLAY_LEN:
		p.playLenStaged = value
	case TED_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 1)
	case TED_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Word(p.playLenStaged, value, 2)
	case TED_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 3)
	case TED_PLAY_CTRL:
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
		// Read directly from bus memory
		mem := p.bus.GetMemory()
		if int(p.playPtr)+int(p.playLen) > len(mem) {
			p.playErr = true
			break
		}
		data := make([]byte, p.playLen)
		copy(data, mem[p.playPtr:p.playPtr+p.playLen])
		p.playBusy = true
		p.playGen++
		startReq = &tedAsyncStartRequest{
			gen:       p.playGen,
			data:      data,
			forceLoop: p.forceLoop,
		}
	default:
		break
	}
	p.mu.Unlock()

	if stopPlayback && p.engine != nil {
		p.engine.StopPlayback()
	}
	if startReq != nil {
		go p.startAsync(*startReq)
	}
}

type tedAsyncStartRequest struct {
	gen       uint64
	data      []byte
	forceLoop bool
}

func (p *TEDPlayer) startAsync(req tedAsyncStartRequest) {
	meta, events, totalSamples, clockHz, loop, loopSample, instrCount, execNanos, err := renderTEDWithLimit(
		req.data, p.engine.sampleRate, 0,
	)

	p.mu.Lock()
	defer p.mu.Unlock()

	if req.gen != p.playGen {
		return
	}

	if err != nil {
		p.playErr = true
		p.playBusy = false
		return
	}

	p.metadata = meta
	p.clockHz = clockHz
	p.loop = loop
	p.renderInstructions = instrCount
	p.renderCPU = "6502"
	p.renderExecNanos = execNanos

	if p.engine != nil {
		p.engine.StopPlayback()
		p.engine.SetClockHz(clockHz)
		p.engine.SetEvents(events, totalSamples, loop, loopSample)
		if req.forceLoop {
			p.engine.SetForceLoop(true)
		}
		if p.engine.sound != nil {
			p.engine.sound.SetSampleTicker(p.engine)
		}
		p.engine.SetPlaying(true)
	}
}

// HandlePlayRead handles reads from TED_PLAY_* registers
func (p *TEDPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case TED_PLAY_PTR:
		return p.playPtrStaged
	case TED_PLAY_LEN:
		return p.playLenStaged
	case TED_PLAY_CTRL:
		return p.playCtrlStatus()
	case TED_PLAY_STATUS:
		return p.playStatus()
	case TED_PLAY_PTR + 1:
		return readUint32Byte(p.playPtrStaged, 1)
	case TED_PLAY_PTR + 2:
		return readUint32Byte(p.playPtrStaged, 2)
	case TED_PLAY_PTR + 3:
		return readUint32Byte(p.playPtrStaged, 3)
	case TED_PLAY_LEN + 1:
		return readUint32Byte(p.playLenStaged, 1)
	case TED_PLAY_LEN + 2:
		return readUint32Byte(p.playLenStaged, 2)
	case TED_PLAY_LEN + 3:
		return readUint32Byte(p.playLenStaged, 3)
	case TED_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case TED_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case TED_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	default:
		return 0
	}
}

func (p *TEDPlayer) playCtrlStatus() uint32 {
	ctrl := uint32(0)
	busy := p.playBusy
	if p.engine != nil && p.engine.IsPlaying() {
		busy = true
	} else if !busy {
		p.playBusy = false
	}
	if busy {
		ctrl |= 1
	}
	if p.playErr {
		ctrl |= 2
	}
	return ctrl
}

func (p *TEDPlayer) playStatus() uint32 {
	return p.playCtrlStatus()
}
