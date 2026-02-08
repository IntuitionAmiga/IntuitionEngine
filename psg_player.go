// psg_player.go - Unified PSG playback controller.

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type PSGPlayer struct {
	engine     *PSGEngine
	bus        Bus32
	metadata   PSGMetadata
	frameRate  uint16
	clockHz    uint32
	loopSample uint64
	loop       bool

	playPtrStaged uint32
	playLenStaged uint32
	playPtr       uint32
	playLen       uint32
	playBusy      bool
	playErr       bool
	forceLoop     bool // When true, loop from start even if file has no loop point
	playGen       uint64

	mu sync.Mutex

	renderInstructions uint64
	renderCPU          string
	renderExecNanos    uint64
}

func NewPSGPlayer(engine *PSGEngine) *PSGPlayer {
	return &PSGPlayer{
		engine: engine,
	}
}

func (p *PSGPlayer) AttachBus(bus Bus32) {
	p.bus = bus
}

func (p *PSGPlayer) Load(path string) error {
	p.renderInstructions = 0
	p.renderCPU = ""
	p.renderExecNanos = 0
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ym":
		file, err := ParseYMFile(path)
		if err != nil {
			return err
		}
		p.metadata = PSGMetadata{Title: file.Title, Author: file.Author, System: "Atari ST"}
		p.frameRate = file.FrameRate
		p.clockHz = file.ClockHz
		return p.loadFrames(file.Frames, file.FrameRate, file.ClockHz, file.LoopFrame)
	case ".ay":
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isZXAYEMUL(data) {
			if p.engine == nil {
				return fmt.Errorf("psg engine not configured")
			}
			meta, events, total, clockHz, frameRate, loop, loopSample, instrCount, execNanos, err := renderAYZ80(data, p.engine.sampleRate)
			if err != nil {
				return err
			}
			p.metadata = meta
			p.frameRate = frameRate
			p.clockHz = clockHz
			p.loop = loop
			p.loopSample = loopSample
			p.renderInstructions = instrCount
			p.renderCPU = "Z80"
			p.renderExecNanos = execNanos
			p.engine.SetClockHz(clockHz)
			p.engine.SetEvents(events, total, loop, loopSample)
			return nil
		}
		file, err := ParseAYFile(path)
		if err != nil {
			return err
		}
		p.metadata = PSGMetadata{Title: file.Title, Author: file.Author, System: "ZX Spectrum"}
		p.frameRate = file.FrameRate
		p.clockHz = file.ClockHz
		return p.loadFrames(file.Frames, file.FrameRate, file.ClockHz, 0)
	case ".vgm", ".vgz":
		file, err := ParseVGMFile(path)
		if err != nil {
			return err
		}
		if file.ClockHz == 0 {
			file.ClockHz = PSG_CLOCK_MSX
		}
		p.metadata = PSGMetadata{Title: "", Author: "", System: "VGM"}
		p.frameRate = 0
		p.clockHz = file.ClockHz
		p.loop = file.LoopSample > 0
		p.loopSample = file.LoopSample
		p.engine.SetClockHz(file.ClockHz)
		p.engine.SetEvents(file.Events, file.TotalSamples, p.loop, p.loopSample)
		return nil
	case ".snd", ".sndh":
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return p.loadSNDH(data)
	default:
		return fmt.Errorf("unsupported PSG file type: %s", ext)
	}
}

func (p *PSGPlayer) LoadData(data []byte) error {
	p.renderInstructions = 0
	p.renderCPU = ""
	p.renderExecNanos = 0
	if len(data) == 0 {
		return fmt.Errorf("psg data empty")
	}
	if len(data) >= 4 && string(data[:4]) == "Vgm " {
		file, err := ParseVGMData(data)
		if err != nil {
			return err
		}
		if file.ClockHz == 0 {
			file.ClockHz = PSG_CLOCK_MSX
		}
		p.metadata = PSGMetadata{Title: "", Author: "", System: "VGM"}
		p.frameRate = 0
		p.clockHz = file.ClockHz
		p.loop = file.LoopSample > 0
		p.loopSample = file.LoopSample
		p.engine.SetClockHz(file.ClockHz)
		p.engine.SetEvents(file.Events, file.TotalSamples, p.loop, p.loopSample)
		return nil
	}
	if len(data) >= 2 && data[0] == 0x1F && data[1] == 0x8B {
		file, err := ParseVGMData(data)
		if err != nil {
			return err
		}
		if file.ClockHz == 0 {
			file.ClockHz = PSG_CLOCK_MSX
		}
		p.metadata = PSGMetadata{Title: "", Author: "", System: "VGM"}
		p.frameRate = 0
		p.clockHz = file.ClockHz
		p.loop = file.LoopSample > 0
		p.loopSample = file.LoopSample
		p.engine.SetClockHz(file.ClockHz)
		p.engine.SetEvents(file.Events, file.TotalSamples, p.loop, p.loopSample)
		return nil
	}
	if len(data) >= 4 && (string(data[:4]) == "YM5!" || string(data[:4]) == "YM6!") {
		file, err := parseYMData(data)
		if err != nil {
			return err
		}
		p.metadata = PSGMetadata{Title: file.Title, Author: file.Author, System: "Atari ST"}
		p.frameRate = file.FrameRate
		p.clockHz = file.ClockHz
		return p.loadFrames(file.Frames, file.FrameRate, file.ClockHz, file.LoopFrame)
	}
	if isZXAYEMUL(data) {
		if p.engine == nil {
			return fmt.Errorf("psg engine not configured")
		}
		meta, events, total, clockHz, frameRate, loop, loopSample, instrCount, execNanos, err := renderAYZ80(data, p.engine.sampleRate)
		if err != nil {
			return err
		}
		p.metadata = meta
		p.frameRate = frameRate
		p.clockHz = clockHz
		p.loop = loop
		p.loopSample = loopSample
		p.renderInstructions = instrCount
		p.renderCPU = "Z80"
		p.renderExecNanos = execNanos
		p.engine.SetClockHz(clockHz)
		p.engine.SetEvents(events, total, loop, loopSample)
		return nil
	}
	if isSNDHData(data) {
		return p.loadSNDH(data)
	}
	file, err := ParseAYData(data)
	if err != nil {
		return err
	}
	p.metadata = PSGMetadata{Title: file.Title, Author: file.Author, System: "ZX Spectrum"}
	p.frameRate = file.FrameRate
	p.clockHz = file.ClockHz
	return p.loadFrames(file.Frames, file.FrameRate, file.ClockHz, 0)
}

func (p *PSGPlayer) Play() {
	if p.engine == nil {
		return
	}
	p.engine.SetPlaying(true)
}

func (p *PSGPlayer) Stop() {
	p.mu.Lock()
	p.playGen++
	p.playBusy = false
	p.mu.Unlock()

	if p.engine == nil {
		return
	}
	p.engine.StopPlayback()
}

func (p *PSGPlayer) Metadata() PSGMetadata {
	return p.metadata
}

func (p *PSGPlayer) loadSNDH(data []byte) error {
	if p.engine == nil {
		return fmt.Errorf("psg engine not configured")
	}
	meta, events, total, clockHz, frameRate, loop, loopSample, instrCount, execNanos, err := renderSNDH(data, p.engine.sampleRate)
	if err != nil {
		return err
	}
	p.metadata = meta
	p.frameRate = frameRate
	p.clockHz = clockHz
	p.loop = loop
	p.loopSample = loopSample
	p.renderInstructions = instrCount
	p.renderCPU = "68K"
	p.renderExecNanos = execNanos
	p.engine.SetClockHz(clockHz)
	p.engine.SetEvents(events, total, loop, loopSample)
	return nil
}

func (p *PSGPlayer) loadFrames(frames [][]uint8, frameRate uint16, clockHz uint32, loopFrame uint32) error {
	if frameRate == 0 {
		return fmt.Errorf("invalid frame rate")
	}
	if p.engine == nil {
		return fmt.Errorf("psg engine not configured")
	}

	if len(frames) > 0 && psgDebugEnabled() {
		first := frames[0]
		fmt.Printf("PSG debug: R0=%02X R1=%02X R2=%02X R3=%02X R4=%02X R5=%02X R6=%02X R7=%02X R8=%02X R9=%02X R10=%02X R11=%02X R12=%02X R13=%02X\n",
			first[0], first[1], first[2], first[3], first[4], first[5], first[6], first[7], first[8], first[9], first[10], first[11], first[12], first[13])
	}

	samplesPerFrameNum := uint64(p.engine.sampleRate)
	samplesPerFrameDen := uint64(frameRate)
	acc := uint64(0)
	samplePos := uint64(0)

	events := make([]PSGEvent, 0, len(frames)*PSG_REG_COUNT)
	loopSample := uint64(0)
	for frameIndex, frame := range frames {
		if uint32(frameIndex) == loopFrame {
			loopSample = samplePos
		}
		for reg := 0; reg < PSG_REG_COUNT; reg++ {
			events = append(events, PSGEvent{
				Sample: samplePos,
				Reg:    uint8(reg),
				Value:  frame[reg],
			})
		}
		acc += samplesPerFrameNum
		step := acc / samplesPerFrameDen
		samplePos += step
		acc -= step * samplesPerFrameDen
	}

	p.loop = loopFrame > 0 && loopFrame < uint32(len(frames))
	p.loopSample = loopSample
	p.engine.SetClockHz(clockHz)
	p.engine.SetEvents(events, samplePos, p.loop, p.loopSample)
	return nil
}

func (p *PSGPlayer) RenderPerf() (uint64, string, uint64) {
	return p.renderInstructions, p.renderCPU, p.renderExecNanos
}

func (p *PSGPlayer) DurationSeconds() float64 {
	if p.engine == nil || p.engine.totalSamples == 0 {
		return 0
	}
	return float64(p.engine.totalSamples) / float64(p.engine.sampleRate)
}

func (p *PSGPlayer) DurationText() string {
	secs := p.DurationSeconds()
	if secs <= 0 {
		return ""
	}
	mins := int(secs) / 60
	rem := int(math.Round(secs)) % 60
	return fmt.Sprintf("%d:%02d", mins, rem)
}

func psgDebugEnabled() bool {
	value := strings.ToLower(os.Getenv("PSG_DEBUG"))
	return value == "1" || value == "true" || value == "yes"
}

type psgRenderResult struct {
	metadata           PSGMetadata
	frameRate          uint16
	clockHz            uint32
	loop               bool
	loopSample         uint64
	events             []PSGEvent
	totalSamples       uint64
	renderInstructions uint64
	renderCPU          string
	renderExecNanos    uint64
}

func buildPSGEventsFromFrames(frames [][]uint8, frameRate uint16, sampleRate int, loopFrame uint32) ([]PSGEvent, uint64, bool, uint64, error) {
	if frameRate == 0 {
		return nil, 0, false, 0, fmt.Errorf("invalid frame rate")
	}
	samplesPerFrameNum := uint64(sampleRate)
	samplesPerFrameDen := uint64(frameRate)
	acc := uint64(0)
	samplePos := uint64(0)

	events := make([]PSGEvent, 0, len(frames)*PSG_REG_COUNT)
	loopSample := uint64(0)
	for frameIndex, frame := range frames {
		if uint32(frameIndex) == loopFrame {
			loopSample = samplePos
		}
		for reg := 0; reg < PSG_REG_COUNT; reg++ {
			events = append(events, PSGEvent{
				Sample: samplePos,
				Reg:    uint8(reg),
				Value:  frame[reg],
			})
		}
		acc += samplesPerFrameNum
		step := acc / samplesPerFrameDen
		samplePos += step
		acc -= step * samplesPerFrameDen
	}

	loop := loopFrame > 0 && loopFrame < uint32(len(frames))
	return events, samplePos, loop, loopSample, nil
}

func renderPSGData(data []byte, sampleRate int) (psgRenderResult, error) {
	var res psgRenderResult

	if len(data) == 0 {
		return res, fmt.Errorf("psg data empty")
	}
	if len(data) >= 4 && string(data[:4]) == "Vgm " {
		file, err := ParseVGMData(data)
		if err != nil {
			return res, err
		}
		if file.ClockHz == 0 {
			file.ClockHz = PSG_CLOCK_MSX
		}
		res.metadata = PSGMetadata{Title: "", Author: "", System: "VGM"}
		res.clockHz = file.ClockHz
		res.loop = file.LoopSample > 0
		res.loopSample = file.LoopSample
		res.events = file.Events
		res.totalSamples = file.TotalSamples
		return res, nil
	}
	if len(data) >= 2 && data[0] == 0x1F && data[1] == 0x8B {
		file, err := ParseVGMData(data)
		if err != nil {
			return res, err
		}
		if file.ClockHz == 0 {
			file.ClockHz = PSG_CLOCK_MSX
		}
		res.metadata = PSGMetadata{Title: "", Author: "", System: "VGM"}
		res.clockHz = file.ClockHz
		res.loop = file.LoopSample > 0
		res.loopSample = file.LoopSample
		res.events = file.Events
		res.totalSamples = file.TotalSamples
		return res, nil
	}
	if len(data) >= 4 && (string(data[:4]) == "YM5!" || string(data[:4]) == "YM6!") {
		file, err := parseYMData(data)
		if err != nil {
			return res, err
		}
		events, total, loop, loopSample, err := buildPSGEventsFromFrames(file.Frames, file.FrameRate, sampleRate, file.LoopFrame)
		if err != nil {
			return res, err
		}
		res.metadata = PSGMetadata{Title: file.Title, Author: file.Author, System: "Atari ST"}
		res.frameRate = file.FrameRate
		res.clockHz = file.ClockHz
		res.loop = loop
		res.loopSample = loopSample
		res.events = events
		res.totalSamples = total
		return res, nil
	}
	if isZXAYEMUL(data) {
		meta, events, total, clockHz, frameRate, loop, loopSample, instrCount, execNanos, err := renderAYZ80(data, sampleRate)
		if err != nil {
			return res, err
		}
		res.metadata = meta
		res.frameRate = frameRate
		res.clockHz = clockHz
		res.loop = loop
		res.loopSample = loopSample
		res.events = events
		res.totalSamples = total
		res.renderInstructions = instrCount
		res.renderCPU = "Z80"
		res.renderExecNanos = execNanos
		return res, nil
	}
	if isSNDHData(data) {
		meta, events, total, clockHz, frameRate, loop, loopSample, instrCount, execNanos, err := renderSNDH(data, sampleRate)
		if err != nil {
			return res, err
		}
		res.metadata = meta
		res.frameRate = frameRate
		res.clockHz = clockHz
		res.loop = loop
		res.loopSample = loopSample
		res.events = events
		res.totalSamples = total
		res.renderInstructions = instrCount
		res.renderCPU = "68K"
		res.renderExecNanos = execNanos
		return res, nil
	}

	file, err := ParseAYData(data)
	if err != nil {
		return res, err
	}
	events, total, loop, loopSample, err := buildPSGEventsFromFrames(file.Frames, file.FrameRate, sampleRate, 0)
	if err != nil {
		return res, err
	}
	res.metadata = PSGMetadata{Title: file.Title, Author: file.Author, System: "ZX Spectrum"}
	res.frameRate = file.FrameRate
	res.clockHz = file.ClockHz
	res.loop = loop
	res.loopSample = loopSample
	res.events = events
	res.totalSamples = total
	return res, nil
}

func (p *PSGPlayer) applyRenderResult(res psgRenderResult) {
	p.metadata = res.metadata
	p.frameRate = res.frameRate
	p.clockHz = res.clockHz
	p.loop = res.loop
	p.loopSample = res.loopSample
	p.renderInstructions = res.renderInstructions
	p.renderCPU = res.renderCPU
	p.renderExecNanos = res.renderExecNanos
	p.engine.SetClockHz(res.clockHz)
	p.engine.SetEvents(res.events, res.totalSamples, res.loop, res.loopSample)
}

type psgAsyncStartRequest struct {
	gen       uint64
	data      []byte
	forceLoop bool
}

func (p *PSGPlayer) startAsync(req psgAsyncStartRequest) {
	res, err := renderPSGData(req.data, p.engine.sampleRate)

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

	p.engine.StopPlayback()
	p.applyRenderResult(res)
	if req.forceLoop {
		p.engine.SetForceLoop(true)
	}
	if p.engine.sound != nil {
		p.engine.sound.SetSampleTicker(p.engine)
	}
	p.engine.SetPlaying(true)
}

func (p *PSGPlayer) HandlePlayWrite(addr uint32, value uint32) {
	var stopPlayback bool
	var startReq *psgAsyncStartRequest

	p.mu.Lock()
	switch addr {
	case PSG_PLAY_PTR:
		p.playPtrStaged = value
	case PSG_PLAY_PTR + 1:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 1)
	case PSG_PLAY_PTR + 2:
		p.playPtrStaged = writeUint32Word(p.playPtrStaged, value, 2)
	case PSG_PLAY_PTR + 3:
		p.playPtrStaged = writeUint32Byte(p.playPtrStaged, value, 3)
	case PSG_PLAY_LEN:
		p.playLenStaged = value
	case PSG_PLAY_LEN + 1:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 1)
	case PSG_PLAY_LEN + 2:
		p.playLenStaged = writeUint32Word(p.playLenStaged, value, 2)
	case PSG_PLAY_LEN + 3:
		p.playLenStaged = writeUint32Byte(p.playLenStaged, value, 3)
	case PSG_PLAY_CTRL:
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
		p.forceLoop = (value & 0x4) != 0 // bit 2 = enable looping
		p.playErr = false
		if p.bus == nil {
			p.playErr = true
			break
		}
		if p.playLen == 0 {
			p.playErr = true
			break
		}
		// Read directly from bus memory to avoid deadlock (bus.Read8 would try to lock bus.mutex)
		mem := p.bus.GetMemory()
		if int(p.playPtr)+int(p.playLen) > len(mem) {
			p.playErr = true
			break
		}
		data := make([]byte, p.playLen)
		copy(data, mem[p.playPtr:p.playPtr+p.playLen])
		p.playBusy = true
		p.playGen++
		startReq = &psgAsyncStartRequest{
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

func (p *PSGPlayer) HandlePlayRead(addr uint32) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch addr {
	case PSG_PLAY_PTR:
		return p.playPtrStaged
	case PSG_PLAY_LEN:
		return p.playLenStaged
	case PSG_PLAY_CTRL:
		return p.playCtrlStatus()
	case PSG_PLAY_STATUS:
		return p.playStatus()
	case PSG_PLAY_PTR + 1:
		return readUint32Byte(p.playPtrStaged, 1)
	case PSG_PLAY_PTR + 2:
		return readUint32Byte(p.playPtrStaged, 2)
	case PSG_PLAY_PTR + 3:
		return readUint32Byte(p.playPtrStaged, 3)
	case PSG_PLAY_LEN + 1:
		return readUint32Byte(p.playLenStaged, 1)
	case PSG_PLAY_LEN + 2:
		return readUint32Byte(p.playLenStaged, 2)
	case PSG_PLAY_LEN + 3:
		return readUint32Byte(p.playLenStaged, 3)
	case PSG_PLAY_CTRL + 1:
		return readUint32Byte(p.playCtrlStatus(), 1)
	case PSG_PLAY_CTRL + 2:
		return readUint32Byte(p.playCtrlStatus(), 2)
	case PSG_PLAY_CTRL + 3:
		return readUint32Byte(p.playCtrlStatus(), 3)
	case PSG_PLAY_STATUS + 1:
		return readUint32Byte(p.playStatus(), 1)
	case PSG_PLAY_STATUS + 2:
		return readUint32Byte(p.playStatus(), 2)
	case PSG_PLAY_STATUS + 3:
		return readUint32Byte(p.playStatus(), 3)
	default:
		return 0
	}
}

func (p *PSGPlayer) playCtrlStatus() uint32 {
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

func (p *PSGPlayer) playStatus() uint32 {
	return p.playCtrlStatus()
}
