// psg_player.go - Unified PSG playback controller.

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type PSGPlayer struct {
	engine     *PSGEngine
	bus        MemoryBus
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
}

func NewPSGPlayer(engine *PSGEngine) *PSGPlayer {
	return &PSGPlayer{
		engine: engine,
	}
}

func (p *PSGPlayer) AttachBus(bus MemoryBus) {
	p.bus = bus
}

func (p *PSGPlayer) Load(path string) error {
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
			meta, events, total, clockHz, frameRate, loop, loopSample, err := renderAYZ80(data, p.engine.sampleRate)
			if err != nil {
				return err
			}
			p.metadata = meta
			p.frameRate = frameRate
			p.clockHz = clockHz
			p.loop = loop
			p.loopSample = loopSample
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
	default:
		return fmt.Errorf("unsupported PSG file type: %s", ext)
	}
}

func (p *PSGPlayer) LoadData(data []byte) error {
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
		meta, events, total, clockHz, frameRate, loop, loopSample, err := renderAYZ80(data, p.engine.sampleRate)
		if err != nil {
			return err
		}
		p.metadata = meta
		p.frameRate = frameRate
		p.clockHz = clockHz
		p.loop = loop
		p.loopSample = loopSample
		p.engine.SetClockHz(clockHz)
		p.engine.SetEvents(events, total, loop, loopSample)
		return nil
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
	if p.engine == nil {
		return
	}
	p.engine.StopPlayback()
	p.playBusy = false
}

func (p *PSGPlayer) Metadata() PSGMetadata {
	return p.metadata
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

func (p *PSGPlayer) HandlePlayWrite(addr uint32, value uint32) {
	switch addr {
	case PSG_PLAY_PTR:
		p.playPtrStaged = value
	case PSG_PLAY_PTR + 1:
		p.playPtrStaged = writePSGUint32Byte(p.playPtrStaged, value, 1)
	case PSG_PLAY_PTR + 2:
		p.playPtrStaged = writePSGUint32Word(p.playPtrStaged, value, 2)
	case PSG_PLAY_PTR + 3:
		p.playPtrStaged = writePSGUint32Byte(p.playPtrStaged, value, 3)
	case PSG_PLAY_LEN:
		p.playLenStaged = value
	case PSG_PLAY_LEN + 1:
		p.playLenStaged = writePSGUint32Byte(p.playLenStaged, value, 1)
	case PSG_PLAY_LEN + 2:
		p.playLenStaged = writePSGUint32Word(p.playLenStaged, value, 2)
	case PSG_PLAY_LEN + 3:
		p.playLenStaged = writePSGUint32Byte(p.playLenStaged, value, 3)
	case PSG_PLAY_CTRL:
		if value&0x2 != 0 {
			p.Stop()
			p.playErr = false
			return
		}
		if value&0x1 == 0 {
			return
		}
		if p.playBusy {
			return
		}
		p.playPtr = p.playPtrStaged
		p.playLen = p.playLenStaged
		p.playErr = false
		if p.bus == nil {
			p.playErr = true
			return
		}
		if p.playLen == 0 {
			p.playErr = true
			return
		}
		p.playBusy = true
		// Read directly from bus memory to avoid deadlock (bus.Read8 would try to lock bus.mutex)
		mem := p.bus.GetMemory()
		if int(p.playPtr)+int(p.playLen) > len(mem) {
			p.playErr = true
			p.playBusy = false
			return
		}
		data := make([]byte, p.playLen)
		copy(data, mem[p.playPtr:p.playPtr+p.playLen])
		if err := p.LoadData(data); err != nil {
			p.playErr = true
			p.playBusy = false
			return
		}
		p.Play()
	default:
		return
	}
}

func (p *PSGPlayer) HandlePlayRead(addr uint32) uint32 {
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
		return readPSGUint32Byte(p.playPtrStaged, 1)
	case PSG_PLAY_PTR + 2:
		return readPSGUint32Byte(p.playPtrStaged, 2)
	case PSG_PLAY_PTR + 3:
		return readPSGUint32Byte(p.playPtrStaged, 3)
	case PSG_PLAY_LEN + 1:
		return readPSGUint32Byte(p.playLenStaged, 1)
	case PSG_PLAY_LEN + 2:
		return readPSGUint32Byte(p.playLenStaged, 2)
	case PSG_PLAY_LEN + 3:
		return readPSGUint32Byte(p.playLenStaged, 3)
	case PSG_PLAY_CTRL + 1:
		return readPSGUint32Byte(p.playCtrlStatus(), 1)
	case PSG_PLAY_CTRL + 2:
		return readPSGUint32Byte(p.playCtrlStatus(), 2)
	case PSG_PLAY_CTRL + 3:
		return readPSGUint32Byte(p.playCtrlStatus(), 3)
	case PSG_PLAY_STATUS + 1:
		return readPSGUint32Byte(p.playStatus(), 1)
	case PSG_PLAY_STATUS + 2:
		return readPSGUint32Byte(p.playStatus(), 2)
	case PSG_PLAY_STATUS + 3:
		return readPSGUint32Byte(p.playStatus(), 3)
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

func writePSGUint32Byte(current uint32, value uint32, byteIndex uint32) uint32 {
	shift := byteIndex * 8
	mask := uint32(0xFF) << shift
	return (current & ^mask) | ((value & 0xFF) << shift)
}

func writePSGUint32Word(current uint32, value uint32, byteIndex uint32) uint32 {
	current = writePSGUint32Byte(current, value, byteIndex)
	if value > 0xFF {
		current = writePSGUint32Byte(current, value>>8, byteIndex+1)
	}
	return current
}

func readPSGUint32Byte(value uint32, byteIndex uint32) uint32 {
	shift := byteIndex * 8
	return (value >> shift) & 0xFF
}
