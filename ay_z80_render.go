package main

import (
	"bytes"
	"fmt"
)

const ayZ80DefaultLoopFrames = 15000

const (
	ayZ80StartupFrames = 2
	ayZ80RenderChunk   = 50
	ayZ80BufferSeconds = 5
)

type ayZ80RenderStream struct {
	player     *ayZ80Player
	metadata   PSGMetadata
	clockHz    uint32
	frameRate  uint16
	loop       bool
	loopSample uint64
	framesLeft int
}

func isZXAYEMUL(data []byte) bool {
	return bytes.HasPrefix(data, []byte("ZXAYEMUL"))
}

func ayZ80SystemName(system byte) string {
	switch system {
	case ayZXSystemCPC:
		return "Amstrad CPC"
	case ayZXSystemMSX:
		return "MSX"
	default:
		return "ZX Spectrum"
	}
}

func renderAYZ80(data []byte, sampleRate int) (PSGMetadata, []PSGEvent, uint64, uint32, uint16, bool, uint64, uint64, uint64, error) {
	return renderAYZ80WithLimit(data, sampleRate, 0)
}

func newAYZ80RenderStream(data []byte, sampleRate int) (*ayZ80RenderStream, error) {
	file, err := ParseAYZ80Data(data)
	if err != nil {
		return nil, err
	}
	songIndex := int(file.Header.FirstSongIndex)
	if songIndex < 0 || songIndex >= len(file.Songs) {
		return nil, fmt.Errorf("ay z80 default song out of range")
	}
	song := file.Songs[songIndex]
	frameRate := uint16(50)
	var clockHz, z80Clock uint32
	switch song.Data.PlayerSystem {
	case ayZXSystemCPC:
		clockHz = PSG_CLOCK_CPC
		z80Clock = Z80_CLOCK_CPC
	case ayZXSystemMSX:
		clockHz = PSG_CLOCK_MSX
		z80Clock = Z80_CLOCK_MSX
	default:
		clockHz = PSG_CLOCK_ZX_SPECTRUM
		z80Clock = Z80_CLOCK_ZX_SPECTRUM
	}
	player, err := newAYZ80Player(file, songIndex, sampleRate, z80Clock, frameRate, nil)
	if err != nil {
		return nil, err
	}
	frames := int(song.Data.LengthFrames)
	loop := false
	if frames == 0 {
		frames = ayZ80DefaultLoopFrames
		loop = true
	}
	return &ayZ80RenderStream{
		player: player,
		metadata: PSGMetadata{
			Title:  song.Name,
			Author: file.Header.Author,
			System: ayZ80SystemName(song.Data.PlayerSystem),
		},
		clockHz:    clockHz,
		frameRate:  frameRate,
		loop:       loop,
		framesLeft: frames,
	}, nil
}

func (s *ayZ80RenderStream) render(frameCount int) ([]PSGEvent, uint64, bool) {
	frameCount = min(frameCount, s.framesLeft)
	events, total := s.player.RenderFrames(frameCount)
	s.framesLeft -= frameCount
	return events, total, s.framesLeft == 0
}

func renderAYZ80WithLimit(data []byte, sampleRate int, maxFrames int) (PSGMetadata, []PSGEvent, uint64, uint32, uint16, bool, uint64, uint64, uint64, error) {
	stream, err := newAYZ80RenderStream(data, sampleRate)
	if err != nil {
		return PSGMetadata{}, nil, 0, 0, 0, false, 0, 0, 0, err
	}
	if stream.loop && maxFrames > 0 && stream.framesLeft > maxFrames {
		stream.framesLeft = maxFrames
	}
	events, totalSamples, _ := stream.render(stream.framesLeft)
	return stream.metadata, events, totalSamples, stream.clockHz, stream.frameRate,
		stream.loop, stream.loopSample, stream.player.instructionCount, stream.player.cpuExecNanos, nil
}
