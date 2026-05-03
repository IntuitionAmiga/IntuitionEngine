// pokey_player.go - POKEY/SAP file playback system
//
// Provides playback of Atari 8-bit SAP music files using the 6502 CPU
// and POKEY chip emulation. Similar to PSGPlayer but for Atari sound.

package main

import (
	"fmt"
	"os"
	"sync"
)

// POKEYPlayer handles SAP file playback
type POKEYPlayer struct {
	engine   *POKEYEngine
	right    *POKEYEngine
	bus      Bus32
	metadata SAPMetadata

	// Playback control state (for CPU-triggered playback)
	playPtrStaged uint32
	playLenStaged uint32
	playPtr       uint32
	playLen       uint32
	playBusy      bool
	playErr       bool
	forceLoop     bool
	subsong       uint8
	playGen       uint64

	mu sync.Mutex

	renderInstructions uint64
	renderCPU          string
	renderExecNanos    uint64
}

// NewPOKEYPlayer creates a new POKEY player
func NewPOKEYPlayer(engine *POKEYEngine) *POKEYPlayer {
	return &POKEYPlayer{
		engine: engine,
	}
}

// Load loads a SAP file from disk
func (p *POKEYPlayer) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read SAP file: %w", err)
	}
	return p.LoadData(data)
}

// LoadData loads SAP data from memory
func (p *POKEYPlayer) LoadData(data []byte) error {
	return p.LoadDataWithSubsong(data, 0)
}

// LoadDataWithSubsong loads SAP data with a specific subsong
func (p *POKEYPlayer) LoadDataWithSubsong(data []byte, subsong int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop any current playback
	p.engine.StopPlayback()

	// Render SAP to POKEY events
	meta, events, totalSamples, clockHz, _, loop, loopSample, instrCount, execNanos, err := renderSAPWithLimit(data, SAMPLE_RATE, 0, subsong)
	if err != nil {
		return err
	}

	p.metadata = meta
	p.renderInstructions = instrCount
	p.renderCPU = "6502"
	p.renderExecNanos = execNanos
	p.configureStereo(meta.Stereo)

	// Set POKEY clock and events
	p.engine.SetClockHz(clockHz)
	p.syncStereoClock()
	p.engine.SetEvents(events, totalSamples, loop, loopSample)

	return nil
}

// Play starts playback
func (p *POKEYPlayer) Play() {
	p.engine.SetPlaying(true)
}

// Stop stops playback
func (p *POKEYPlayer) Stop() {
	p.mu.Lock()
	p.playGen++
	p.playBusy = false
	p.mu.Unlock()
	p.engine.StopPlayback()
	p.releaseStereo()
}

// IsPlaying returns true if playback is active
func (p *POKEYPlayer) IsPlaying() bool {
	return p.engine.IsPlaying()
}

// Metadata returns the SAP file metadata
func (p *POKEYPlayer) Metadata() SAPMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metadata
}

// DurationSeconds returns the duration in seconds
func (p *POKEYPlayer) RenderPerf() (uint64, string, uint64) {
	return p.renderInstructions, p.renderCPU, p.renderExecNanos
}

func (p *POKEYPlayer) DurationSeconds() float64 {
	p.engine.mutex.Lock()
	defer p.engine.mutex.Unlock()
	if p.engine.totalSamples == 0 {
		return 0
	}
	return float64(p.engine.totalSamples) / float64(SAMPLE_RATE)
}

// DurationText returns formatted duration string
func (p *POKEYPlayer) DurationText() string {
	dur := p.DurationSeconds()
	if dur <= 0 {
		return ""
	}
	minutes := int(dur) / 60
	seconds := int(dur) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// AttachBus attaches a memory bus for reading embedded SAP data
func (p *POKEYPlayer) AttachBus(bus Bus32) {
	p.bus = bus
}

// HandlePlayWrite handles writes to SAP_PLAY_* registers
func (p *POKEYPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *pokeyAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case SAP_PLAY_PTR:
		p.playPtrStaged = value
	case SAP_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 1)
	case SAP_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Word(p.playPtrStaged, value, 2)
	case SAP_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 3)
	case SAP_PLAY_LEN:
		p.playLenStaged = value
	case SAP_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 1)
	case SAP_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Word(p.playLenStaged, value, 2)
	case SAP_SUBSONG:
		p.subsong = uint8(value)
	case SAP_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 3)
	case SAP_PLAY_CTRL:
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
		data := make([]byte, p.playLen)
		if err := ReadGuestBytes(p.bus, p.playPtr, 0, data); err != nil {
			p.playErr = true
			break
		}
		p.playBusy = true
		p.playGen++
		startReq = &pokeyAsyncStartRequest{
			gen:       p.playGen,
			data:      data,
			forceLoop: p.forceLoop,
			subsong:   int(p.subsong),
		}
	default:
		break
	}
	p.mu.Unlock()

	if stopPlayback {
		p.engine.StopPlayback()
	}
	if startReq != nil {
		go p.startAsync(*startReq)
	}
}

type pokeyAsyncStartRequest struct {
	gen       uint64
	data      []byte
	forceLoop bool
	subsong   int
}

func (p *POKEYPlayer) startAsync(req pokeyAsyncStartRequest) {
	meta, events, totalSamples, clockHz, _, loop, loopSample, instrCount, execNanos, err := renderSAPWithLimit(
		req.data, SAMPLE_RATE, 0, req.subsong,
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
	p.renderInstructions = instrCount
	p.renderCPU = "6502"
	p.renderExecNanos = execNanos
	p.configureStereo(meta.Stereo)

	p.engine.StopPlayback()
	p.engine.SetClockHz(clockHz)
	p.syncStereoClock()
	p.engine.SetEvents(events, totalSamples, loop, loopSample)
	if req.forceLoop {
		p.engine.SetForceLoop(true)
	}
	if p.engine.sound != nil {
		p.engine.sound.SetSampleTicker(p.engine)
	}
	p.engine.SetPlaying(true)
}

func (p *POKEYPlayer) configureStereo(stereo bool) {
	if p.engine == nil {
		return
	}
	if !stereo {
		p.releaseStereo()
		return
	}
	if p.right == nil {
		p.right = NewPOKEYEngineMulti(p.engine.sound, p.engine.sampleRate, 4)
	}
	if p.engine.right == nil {
		p.engine.setRight(p.right)
	}
}

func (p *POKEYPlayer) syncStereoClock() {
	if p.engine == nil || p.right == nil {
		return
	}
	p.right.SetClockHz(p.engine.clockHz)
}

func (p *POKEYPlayer) releaseStereo() {
	if p.engine != nil {
		p.engine.right = nil
	}
	if p.right != nil {
		p.right.Reset()
	}
	p.restoreSharedStereoBank()
}

func (p *POKEYPlayer) restoreSharedStereoBank() {
	if p.engine == nil || p.engine.sound == nil {
		return
	}
	sound := p.engine.sound
	sound.mu.Lock()
	defer sound.mu.Unlock()
	waveTypes := [NUM_CHANNELS]int{
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE,
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE,
		WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE,
	}
	for ch := 4; ch < 8 && ch < NUM_CHANNELS; ch++ {
		c := sound.channels[ch]
		if c == nil {
			continue
		}
		c.waveType = waveTypes[ch]
		c.frequency = 0
		c.volume = MIN_VOLUME
		c.phase = MIN_PHASE
		c.noisePhase = 0
		c.noiseSR = NOISE_LFSR_SEED
		c.enabled = false
		c.gate = false
		c.dutyCycle = DEFAULT_DUTY_CYCLE
		c.pokeyPlusEnabled = false
		c.pokeyPlusOversample = 1
		c.pokeyPlusGain = 1.0
		c.filterModeMask = 0
		c.filterType = 0
	}
}

func (p *POKEYPlayer) HandlePlayWrite8(addr uint32, value uint8) {
	p.mu.Lock()

	switch addr {
	case SAP_PLAY_PTR:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, uint32(value), 0)
	case SAP_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, uint32(value), 1)
	case SAP_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, uint32(value), 2)
	case SAP_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, uint32(value), 3)
	case SAP_PLAY_LEN:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, uint32(value), 0)
	case SAP_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, uint32(value), 1)
	case SAP_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, uint32(value), 2)
	case SAP_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, uint32(value), 3)
	case SAP_SUBSONG:
		p.subsong = value
	default:
		p.mu.Unlock()
		p.HandlePlayWrite(addr, uint32(value))
		return
	}
	p.mu.Unlock()
}

// HandlePlayRead handles reads from SAP_PLAY_* registers
func (p *POKEYPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case SAP_PLAY_PTR:
		return p.playPtrStaged
	case SAP_PLAY_LEN:
		return p.playLenStaged
	case SAP_PLAY_CTRL:
		return p.playCtrlStatus()
	case SAP_PLAY_STATUS:
		return p.playStatus()
	case SAP_SUBSONG:
		return uint32(p.subsong)
	case SAP_PLAY_PTR + 1:
		return readUint32Byte(p.playPtrStaged, 1)
	case SAP_PLAY_PTR + 2:
		return readUint32Byte(p.playPtrStaged, 2)
	case SAP_PLAY_PTR + 3:
		return readUint32Byte(p.playPtrStaged, 3)
	case SAP_PLAY_LEN + 1:
		return readUint32Byte(p.playLenStaged, 1)
	case SAP_PLAY_LEN + 2:
		return readUint32Byte(p.playLenStaged, 2)
	case SAP_PLAY_LEN + 3:
		return readUint32Byte(p.playLenStaged, 3)
	case SAP_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case SAP_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case SAP_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	default:
		return 0
	}
}

func (p *POKEYPlayer) playCtrlStatus() uint32 {
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

func (p *POKEYPlayer) playStatus() uint32 {
	return p.playCtrlStatus()
}
