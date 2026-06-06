package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	sidDefaultLoopFrames = 15000
	sidMaxFrames         = 600000
)

type SIDMetadata struct {
	Title    string
	Author   string
	Released string
}

type SIDPlayer struct {
	engine   *SIDEngine
	metadata SIDMetadata
	clockHz  uint32
	loop     bool

	PlayerControlState
	startOnce sync.Once
	startCh   chan sidAsyncStartRequest

	mu sync.Mutex

	renderInstructions uint64
	renderCPU          string
	renderExecNanos    uint64
}

func NewSIDPlayer(engine *SIDEngine) *SIDPlayer {
	return &SIDPlayer{engine: engine}
}

func (p *SIDPlayer) Load(path string) error {
	return p.LoadWithOptions(path, 0, false, false)
}

func (p *SIDPlayer) LoadWithOptions(path string, subsong int, forcePAL bool, forceNTSC bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read SID file: %w", err)
	}
	return p.LoadDataWithOptions(data, subsong, forcePAL, forceNTSC)
}

func (p *SIDPlayer) LoadData(data []byte) error {
	return p.LoadDataWithOptions(data, 0, false, false)
}

func (p *SIDPlayer) LoadDataWithOptions(data []byte, subsong int, forcePAL bool, forceNTSC bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.engine.StopPlayback()

	meta, events, totalSamples, clockHz, _, loop, loopSample, instrCount, execNanos, err := renderSIDWithLimit(data, p.engine.sampleRate, 0, subsong, forcePAL, forceNTSC)
	if err != nil {
		return err
	}

	p.metadata = meta
	p.clockHz = clockHz
	p.loop = loop
	p.renderInstructions = instrCount
	p.renderCPU = "6502"
	p.renderExecNanos = execNanos

	// Set up multi-SID engines and apply chip models from header flags
	if p.engine.sound != nil {
		sidFile, parseErr := ParseSIDData(data)
		if parseErr == nil {
			// Apply primary SID model from flags bits 4-5
			p.engine.SetModel(sidHeaderModel(sidFile.Header.Flags, 4))

			if sidFile.Header.Sid2Addr != 0 && p.engine.sid2 == nil {
				p.engine.sid2 = NewSIDEngineMulti(p.engine.sound, p.engine.sampleRate, 4, SID2_BASE, SID2_END)
				p.engine.sid2.SetClockHz(clockHz)
				p.engine.sid2.SetModel(sidHeaderModel(sidFile.Header.Flags, 6))
			}
			if sidFile.Header.Sid3Addr != 0 && p.engine.sid3 == nil {
				p.engine.sid3 = NewSIDEngineMulti(p.engine.sound, p.engine.sampleRate, 7, SID3_BASE, SID3_END)
				p.engine.sid3.SetClockHz(clockHz)
				p.engine.sid3.SetModel(sidHeaderModel(sidFile.Header.Flags, 8))
			}
		}
	}

	p.engine.SetClockHz(clockHz)
	p.engine.SetEvents(events, totalSamples, loop, loopSample)

	return nil
}

func (p *SIDPlayer) Play() {
	if p.engine.sound != nil {
		p.engine.sound.SetSampleTicker(p.engine)
	}
	p.engine.SetPlaying(true)
}

func (p *SIDPlayer) Stop() {
	p.mu.Lock()
	p.StopPlaybackRequest()
	p.mu.Unlock()
	p.engine.StopPlayback()
}

func (p *SIDPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

func (p *SIDPlayer) Metadata() SIDMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metadata
}

func (p *SIDPlayer) RenderPerf() (uint64, string, uint64) {
	return p.renderInstructions, p.renderCPU, p.renderExecNanos
}

func (p *SIDPlayer) DurationSeconds() float64 {
	p.engine.mutex.Lock()
	defer p.engine.mutex.Unlock()
	if p.engine.totalSamples == 0 {
		return 0
	}
	return float64(p.engine.totalSamples) / float64(p.engine.sampleRate)
}

func (p *SIDPlayer) DurationText() string {
	dur := p.DurationSeconds()
	if dur <= 0 {
		return ""
	}
	minutes := int(dur) / 60
	seconds := int(dur) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func renderSIDWithLimit(data []byte, sampleRate int, maxFrames int, subsong int, forcePAL bool, forceNTSC bool) (SIDMetadata, []SIDEvent, uint64, uint32, uint16, bool, uint64, uint64, uint64, error) {
	file, err := ParseSIDData(data)
	if err != nil {
		return SIDMetadata{}, nil, 0, 0, 0, false, 0, 0, 0, fmt.Errorf("parse SID: %w", err)
	}

	if forcePAL && forceNTSC {
		return SIDMetadata{}, nil, 0, 0, 0, false, 0, 0, 0, fmt.Errorf("cannot force both PAL and NTSC")
	}
	if forcePAL {
		file.Header.Flags = (file.Header.Flags &^ 0x03) | 0x01
	}
	if forceNTSC {
		file.Header.Flags = (file.Header.Flags &^ 0x03) | 0x02
	}

	if subsong < 1 || subsong > int(file.Header.Songs) {
		subsong = int(file.Header.StartSong)
	}
	if subsong < 1 {
		subsong = 1
	}

	player, err := newSID6502Player(file, subsong, sampleRate)
	if err != nil {
		return SIDMetadata{}, nil, 0, 0, 0, false, 0, 0, 0, fmt.Errorf("create player: %w", err)
	}

	frameRate := uint16(math.Round(player.TickHz()))

	frameCount := sidDefaultLoopFrames
	loop := true
	loopSample := uint64(0)

	if maxFrames > 0 && frameCount > maxFrames {
		frameCount = maxFrames
	}
	if frameCount > sidMaxFrames {
		frameCount = sidMaxFrames
	}

	events, totalSamples := player.RenderFrames(frameCount)

	meta := SIDMetadata{
		Title:    file.Header.Name,
		Author:   file.Header.Author,
		Released: file.Header.Released,
	}

	clockHz := player.clockHz

	return meta, events, totalSamples, clockHz, uint16(frameRate), loop, loopSample, player.instructionCount, player.cpuExecNanos, nil
}

func isSIDExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sid":
		return true
	default:
		return false
	}
}

// AttachBus attaches the memory bus for reading embedded SID data
func (p *SIDPlayer) AttachBus(bus Bus32) {
	p.Bus = bus
}

// HandlePlayWrite handles writes to SID_PLAY_* registers
func (p *SIDPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *sidAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case SID_PLAY_PTR:
		p.PlayPtrStaged = value
	case SID_PLAY_PTR + 1:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, value, 1)
	case SID_PLAY_PTR + 2:
		p.PlayPtrStaged = writeUint32Word(p.PlayPtrStaged, value, 2)
	case SID_PLAY_PTR + 3:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, value, 3)
	case SID_PLAY_LEN:
		p.PlayLenStaged = value
	case SID_PLAY_LEN + 1:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, value, 1)
	case SID_PLAY_LEN + 2:
		p.PlayLenStaged = writeUint32Word(p.PlayLenStaged, value, 2)
	case SID_PLAY_LEN + 3:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, value, 3)
	case SID_SUBSONG:
		p.Subsong = uint8(value)
	case SID_PLAY_CTRL:
		if value&0x2 != 0 {
			p.PlayGen++
			p.PlayBusy = false
			p.PlayErr = false
			stopPlayback = true
			break
		}
		if value&0x1 == 0 {
			break
		}
		if p.PlayBusy {
			break
		}
		p.PlayPtr = p.PlayPtrStaged
		p.PlayLen = p.PlayLenStaged
		p.ForceLoop = (value & 0x4) != 0
		p.PlayErr = false
		if p.Bus == nil {
			p.PlayErr = true
			break
		}
		if p.PlayLen == 0 {
			p.PlayErr = true
			break
		}
		data := make([]byte, p.PlayLen)
		if err := ReadGuestBytes(p.Bus, p.PlayPtr, 0, data); err != nil {
			p.PlayErr = true
			break
		}
		subsong := int(p.Subsong)
		p.PlayBusy = true
		p.PlayGen++
		startReq = &sidAsyncStartRequest{
			gen:       p.PlayGen,
			data:      data,
			subsong:   subsong,
			forceLoop: p.ForceLoop,
		}
	default:
		break
	}
	p.mu.Unlock()

	if stopPlayback {
		p.engine.StopPlayback()
	}
	if startReq != nil {
		p.enqueueStart(*startReq)
	}
}

type sidAsyncStartRequest struct {
	gen       uint64
	data      []byte
	subsong   int
	forceLoop bool
}

func (p *SIDPlayer) enqueueStart(req sidAsyncStartRequest) {
	p.startOnce.Do(func() {
		p.startCh = make(chan sidAsyncStartRequest, 4)
		go func() {
			for queued := range p.startCh {
				p.startAsync(queued)
			}
		}()
	})
	select {
	case p.startCh <- req:
	default:
		p.mu.Lock()
		p.PlayErr = true
		p.PlayBusy = false
		p.mu.Unlock()
	}
}

func (p *SIDPlayer) startAsync(req sidAsyncStartRequest) {
	meta, events, totalSamples, clockHz, _, loop, loopSample, instrCount, execNanos, err := renderSIDWithLimit(
		req.data, p.engine.sampleRate, 0, req.subsong, false, false,
	)

	p.mu.Lock()
	defer p.mu.Unlock()

	if req.gen != p.PlayGen {
		return
	}

	if err != nil {
		fmt.Printf("SID PLAY error: %v\n", err)
		p.PlayErr = true
		p.PlayBusy = false
		return
	}
	if len(events) == 0 || totalSamples == 0 {
		fmt.Printf("SID PLAY warning: rendered empty stream (events=%d, samples=%d)\n", len(events), totalSamples)
	}

	p.metadata = meta
	p.clockHz = clockHz
	p.loop = loop
	p.renderInstructions = instrCount
	p.renderCPU = "6502"
	p.renderExecNanos = execNanos

	p.engine.StopPlayback()
	p.engine.SetClockHz(clockHz)
	p.engine.SetEvents(events, totalSamples, loop, loopSample)
	if req.forceLoop {
		p.engine.SetForceLoop(true)
	}
	p.Play()
}

// HandlePlayRead handles reads from SID_PLAY_* registers
func (p *SIDPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case SID_PLAY_PTR:
		return p.PlayPtrStaged
	case SID_PLAY_LEN:
		return p.PlayLenStaged
	case SID_PLAY_CTRL:
		return p.playCtrlStatus()
	case SID_PLAY_STATUS:
		return p.playStatus()
	case SID_SUBSONG:
		return uint32(p.Subsong)
	case SID_PLAY_PTR + 1:
		return readUint32Byte(p.PlayPtrStaged, 1)
	case SID_PLAY_PTR + 2:
		return readUint32Byte(p.PlayPtrStaged, 2)
	case SID_PLAY_PTR + 3:
		return readUint32Byte(p.PlayPtrStaged, 3)
	case SID_PLAY_LEN + 1:
		return readUint32Byte(p.PlayLenStaged, 1)
	case SID_PLAY_LEN + 2:
		return readUint32Byte(p.PlayLenStaged, 2)
	case SID_PLAY_LEN + 3:
		return readUint32Byte(p.PlayLenStaged, 3)
	case SID_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case SID_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case SID_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	default:
		return 0
	}
}

func (p *SIDPlayer) playCtrlStatus() uint32 {
	ctrl := uint32(0)
	busy := p.PlayBusy
	if p.engine != nil && p.engine.IsPlaying() {
		busy = true
	} else if !busy {
		p.PlayBusy = false
	}
	if busy {
		ctrl |= 1
	}
	if p.PlayErr {
		ctrl |= 2
	}
	return ctrl
}

func (p *SIDPlayer) playStatus() uint32 {
	return p.playCtrlStatus()
}
