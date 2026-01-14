package main

import (
	"bytes"
	"fmt"
)

const ayZ80DefaultLoopFrames = 15000

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

func renderAYZ80(data []byte, sampleRate int) (PSGMetadata, []PSGEvent, uint64, uint32, uint16, bool, uint64, error) {
	return renderAYZ80WithLimit(data, sampleRate, 0)
}

func renderAYZ80WithLimit(data []byte, sampleRate int, maxFrames int) (PSGMetadata, []PSGEvent, uint64, uint32, uint16, bool, uint64, error) {
	file, err := ParseAYZ80Data(data)
	if err != nil {
		return PSGMetadata{}, nil, 0, 0, 0, false, 0, err
	}
	songIndex := int(file.Header.FirstSongIndex)
	if songIndex < 0 || songIndex >= len(file.Songs) {
		return PSGMetadata{}, nil, 0, 0, 0, false, 0, fmt.Errorf("ay z80 default song out of range")
	}
	song := file.Songs[songIndex]
	frameRate := uint16(50)
	clockHz := uint32(PSG_CLOCK_ZX_SPECTRUM)
	z80Clock := uint32(Z80_CLOCK_ZX_SPECTRUM)

	player, err := newAYZ80Player(file, songIndex, sampleRate, z80Clock, frameRate, nil)
	if err != nil {
		return PSGMetadata{}, nil, 0, 0, 0, false, 0, err
	}

	frameCount := int(song.Data.LengthFrames)
	loop := false
	loopSample := uint64(0)
	if frameCount == 0 {
		frameCount = ayZ80DefaultLoopFrames
		if maxFrames > 0 && frameCount > maxFrames {
			frameCount = maxFrames
		}
		loop = true
	}
	events, totalSamples := player.RenderFrames(frameCount)

	meta := PSGMetadata{
		Title:  song.Name,
		Author: file.Header.Author,
		System: ayZ80SystemName(song.Data.PlayerSystem),
	}
	return meta, events, totalSamples, clockHz, frameRate, loop, loopSample, nil
}
