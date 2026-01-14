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
	metadata   PSGMetadata
	frameRate  uint16
	clockHz    uint32
	loopSample uint64
	loop       bool
}

func NewPSGPlayer(engine *PSGEngine) *PSGPlayer {
	return &PSGPlayer{
		engine: engine,
	}
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
