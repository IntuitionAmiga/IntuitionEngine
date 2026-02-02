// ym_parser.go - YM file parser for AY/YM register frames.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

type YMFile struct {
	Frames      [][]uint8
	FrameRate   uint16
	ClockHz     uint32
	LoopFrame   uint32
	Title       string
	Author      string
	Comments    string
	Interleaved bool
}

const ymFrameRegisters = 16
const ymLegacyRegisters = 14

func ParseYMFile(path string) (*YMFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ym, err := parseYMData(data)
	if err == nil {
		return ym, nil
	}

	decompressed, decErr := DecompressLHAFile(path)
	if decErr != nil {
		return nil, err
	}

	ym, err = parseYMData(decompressed)
	if err != nil {
		return nil, err
	}
	return ym, nil
}

// ymPsgDebugEnabled caches the PSG_DEBUG environment variable at init time
var ymPsgDebugEnabled = func() bool {
	value := strings.ToLower(os.Getenv("PSG_DEBUG"))
	return value == "1" || value == "true" || value == "yes"
}()

func psgDebugEnabledYM() bool {
	return ymPsgDebugEnabled
}

func parseYMData(data []byte) (*YMFile, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("ym too short")
	}

	id := string(data[:4])
	if id != "YM5!" && id != "YM6!" {
		return nil, fmt.Errorf("unsupported ym version: %s", id)
	}
	if string(data[4:12]) != "LeOnArD!" {
		return nil, fmt.Errorf("invalid ym signature")
	}

	off := 12
	readU32 := func() (uint32, error) {
		if off+4 > len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		val := binary.BigEndian.Uint32(data[off : off+4])
		off += 4
		return val, nil
	}
	readU16 := func() (uint16, error) {
		if off+2 > len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		val := binary.BigEndian.Uint16(data[off : off+2])
		off += 2
		return val, nil
	}

	nbFrames, err := readU32()
	if err != nil {
		return nil, err
	}
	songAttrs, err := readU32()
	if err != nil {
		return nil, err
	}
	numDrums, err := readU16()
	if err != nil {
		return nil, err
	}
	clock, err := readU32()
	if err != nil {
		return nil, err
	}
	frameRate, err := readU16()
	if err != nil {
		return nil, err
	}
	loopFrame, err := readU32()
	if err != nil {
		return nil, err
	}
	addData, err := readU16()
	if err != nil {
		return nil, err
	}

	if off+int(addData) > len(data) {
		return nil, io.ErrUnexpectedEOF
	}
	off += int(addData)

	readString := func() (string, error) {
		start := off
		for off < len(data) && data[off] != 0 {
			off++
		}
		if off > len(data) {
			return "", io.ErrUnexpectedEOF
		}
		s := string(data[start:off])
		if off < len(data) {
			off++
		}
		return s, nil
	}

	title, _ := readString()
	author, _ := readString()
	comments, _ := readString()

	if numDrums > 0 {
		for i := 0; i < int(numDrums); i++ {
			sz, err := readU32()
			if err != nil {
				return nil, err
			}
			if off+int(sz) > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			off += int(sz)
		}
	}

	if psgDebugEnabledYM() {
		dumpLen := 64
		if len(data) < dumpLen {
			dumpLen = len(data)
		}
		fmt.Printf("YM debug header bytes: % X\n", data[:dumpLen])
		fmt.Printf("YM debug: frames=%d attrs=0x%X drums=%d clock=%d rate=%d loop=%d add=%d title=%q author=%q\n",
			nbFrames, songAttrs, numDrums, clock, frameRate, loopFrame, addData, title, author)
	}

	interleaved := (songAttrs & 0x01) != 0

	frameCount := int(nbFrames)
	if frameCount < 0 {
		return nil, fmt.Errorf("invalid frame count")
	}

	frameDataStart := off
	remaining := data[frameDataStart:]
	expected16 := frameCount * ymFrameRegisters
	expected14 := frameCount * ymLegacyRegisters
	frameRegCount := ymFrameRegisters
	expected := expected16

	if len(remaining) < expected16 {
		if len(remaining) >= expected14 {
			frameRegCount = ymLegacyRegisters
			expected = expected14
		} else {
			return nil, fmt.Errorf("ym frame data too short")
		}
	}

	// Allocate single contiguous buffer for all frames
	buffer := make([]uint8, frameCount*PSG_REG_COUNT)
	frames := make([][]uint8, frameCount)
	for i := 0; i < frameCount; i++ {
		start := i * PSG_REG_COUNT
		frames[i] = buffer[start : start+PSG_REG_COUNT : start+PSG_REG_COUNT]
	}

	if interleaved {
		for reg := 0; reg < frameRegCount && reg < PSG_REG_COUNT; reg++ {
			base := reg * frameCount
			for frame := 0; frame < frameCount; frame++ {
				frames[frame][reg] = remaining[base+frame]
			}
		}
	} else {
		for frame := 0; frame < frameCount; frame++ {
			start := frame * frameRegCount
			copy(frames[frame], remaining[start:start+PSG_REG_COUNT])
		}
	}

	off = frameDataStart + expected

	return &YMFile{
		Frames:      frames,
		FrameRate:   frameRate,
		ClockHz:     clock,
		LoopFrame:   loopFrame,
		Title:       title,
		Author:      author,
		Comments:    comments,
		Interleaved: interleaved,
	}, nil
}
