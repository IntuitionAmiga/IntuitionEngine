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
	metadata SAPMetadata

	PlayerControlState

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
	p.StopPlaybackRequest()
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
	p.Bus = bus
}

// HandlePlayWrite handles writes to SAP_PLAY_* registers
func (p *POKEYPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *pokeyAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case SAP_PLAY_PTR:
		p.PlayPtrStaged = value
	case SAP_PLAY_PTR + 1:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, value, 1)
	case SAP_PLAY_PTR + 2:
		p.PlayPtrStaged = writeUint32Word(p.PlayPtrStaged, value, 2)
	case SAP_PLAY_PTR + 3:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, value, 3)
	case SAP_PLAY_LEN:
		p.PlayLenStaged = value
	case SAP_PLAY_LEN + 1:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, value, 1)
	case SAP_PLAY_LEN + 2:
		p.PlayLenStaged = writeUint32Word(p.PlayLenStaged, value, 2)
	case SAP_SUBSONG:
		p.Subsong = uint8(value)
	case SAP_PLAY_LEN + 3:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, value, 3)
	case SAP_PLAY_CTRL:
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
		// Read directly from bus memory
		data := make([]byte, p.PlayLen)
		if err := ReadGuestBytes(p.Bus, p.PlayPtr, 0, data); err != nil {
			p.PlayErr = true
			break
		}
		p.PlayBusy = true
		p.PlayGen++
		startReq = &pokeyAsyncStartRequest{
			gen:       p.PlayGen,
			data:      data,
			forceLoop: p.ForceLoop,
			subsong:   int(p.Subsong),
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

	if req.gen != p.PlayGen {
		return
	}

	if err != nil {
		p.PlayErr = true
		p.PlayBusy = false
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
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, uint32(value), 0)
	case SAP_PLAY_PTR + 1:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, uint32(value), 1)
	case SAP_PLAY_PTR + 2:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, uint32(value), 2)
	case SAP_PLAY_PTR + 3:
		p.PlayPtrStaged = writeUint32Byte(p.PlayPtrStaged, uint32(value), 3)
	case SAP_PLAY_LEN:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, uint32(value), 0)
	case SAP_PLAY_LEN + 1:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, uint32(value), 1)
	case SAP_PLAY_LEN + 2:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, uint32(value), 2)
	case SAP_PLAY_LEN + 3:
		p.PlayLenStaged = writeUint32Byte(p.PlayLenStaged, uint32(value), 3)
	case SAP_SUBSONG:
		p.Subsong = value
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
		return p.PlayPtrStaged
	case SAP_PLAY_LEN:
		return p.PlayLenStaged
	case SAP_PLAY_CTRL:
		return p.playCtrlStatus()
	case SAP_PLAY_STATUS:
		return p.playStatus()
	case SAP_SUBSONG:
		return uint32(p.Subsong)
	case SAP_PLAY_PTR + 1:
		return readUint32Byte(p.PlayPtrStaged, 1)
	case SAP_PLAY_PTR + 2:
		return readUint32Byte(p.PlayPtrStaged, 2)
	case SAP_PLAY_PTR + 3:
		return readUint32Byte(p.PlayPtrStaged, 3)
	case SAP_PLAY_LEN + 1:
		return readUint32Byte(p.PlayLenStaged, 1)
	case SAP_PLAY_LEN + 2:
		return readUint32Byte(p.PlayLenStaged, 2)
	case SAP_PLAY_LEN + 3:
		return readUint32Byte(p.PlayLenStaged, 3)
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

func (p *POKEYPlayer) playStatus() uint32 {
	return p.playCtrlStatus()
}
