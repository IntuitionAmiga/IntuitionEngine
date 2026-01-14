// vgm_parser.go - VGM/VGZ parser for AY/YM register writes.

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type VGMFile struct {
	Events       []PSGEvent
	ClockHz      uint32
	TotalSamples uint64
	LoopSamples  uint64
	LoopSample   uint64
}

func ParseVGMFile(path string) (*VGMFile, error) {
	data, err := readVGMData(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 0x40 {
		return nil, fmt.Errorf("vgm too short")
	}
	if !bytes.Equal(data[0:4], []byte("Vgm ")) {
		return nil, fmt.Errorf("invalid vgm header")
	}

	version := binary.LittleEndian.Uint32(data[0x08:0x0C])
	_ = version

	totalSamples := binary.LittleEndian.Uint32(data[0x18:0x1C])
	loopSamples := binary.LittleEndian.Uint32(data[0x20:0x24])
	loopOffset := binary.LittleEndian.Uint32(data[0x1C:0x20])

	dataOffset := binary.LittleEndian.Uint32(data[0x34:0x38])
	dataStart := uint32(0x40)
	if dataOffset != 0 {
		dataStart = 0x34 + dataOffset
	}
	if int(dataStart) >= len(data) {
		return nil, fmt.Errorf("vgm data offset out of range")
	}

	clockHz := uint32(0)
	if len(data) >= 0x78 {
		clockHz = binary.LittleEndian.Uint32(data[0x74:0x78])
	}

	events := make([]PSGEvent, 0, 1024)
	samplePos := uint64(0)
	loopSample := uint64(0)
	loopStart := uint32(0)
	if loopOffset != 0 {
		loopStart = 0x1C + loopOffset
	}

	for i := int(dataStart); i < len(data); {
		if loopStart != 0 && loopSample == 0 && uint32(i) == loopStart {
			loopSample = samplePos
		}
		cmd := data[i]
		switch {
		case cmd == 0x66:
			i = len(data)
			continue
		case cmd == 0xA0:
			if i+2 >= len(data) {
				return nil, fmt.Errorf("vgm truncated AY write")
			}
			reg := data[i+1]
			val := data[i+2]
			events = append(events, PSGEvent{Sample: samplePos, Reg: reg, Value: val})
			i += 3
			continue
		case cmd == 0x61:
			if i+2 >= len(data) {
				return nil, fmt.Errorf("vgm truncated wait")
			}
			wait := binary.LittleEndian.Uint16(data[i+1 : i+3])
			samplePos += uint64(wait)
			i += 3
			continue
		case cmd == 0x50:
			if i+1 >= len(data) {
				return nil, fmt.Errorf("vgm truncated psg write")
			}
			i += 2
			continue
		case cmd == 0x62:
			samplePos += 735
			i++
			continue
		case cmd == 0x63:
			samplePos += 882
			i++
			continue
		case cmd >= 0x70 && cmd <= 0x7F:
			samplePos += uint64(cmd&0x0F) + 1
			i++
			continue
		case cmd == 0x67:
			if i+6 >= len(data) {
				return nil, fmt.Errorf("vgm truncated data block")
			}
			if data[i+1] != 0x66 {
				return nil, fmt.Errorf("vgm invalid data block")
			}
			blockLen := binary.LittleEndian.Uint32(data[i+3 : i+7])
			i += 7 + int(blockLen)
			continue
		default:
			return nil, fmt.Errorf("vgm unsupported command 0x%02X at 0x%X", cmd, i)
		}
	}

	if len(events) > 0 {
		last := events[len(events)-1].Sample + 1
		if uint64(totalSamples) > last {
			last = uint64(totalSamples)
		}
		totalSamples = uint32(last)
	}
	if loopSample == 0 && loopSamples > 0 && uint64(totalSamples) >= uint64(loopSamples) {
		loopSample = uint64(totalSamples) - uint64(loopSamples)
	}

	return &VGMFile{
		Events:       events,
		ClockHz:      clockHz,
		TotalSamples: uint64(totalSamples),
		LoopSamples:  uint64(loopSamples),
		LoopSample:   loopSample,
	}, nil
}

func readVGMData(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	header := make([]byte, 2)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if header[0] == 0x1F && header[1] == 0x8B {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		return io.ReadAll(gz)
	}

	return io.ReadAll(f)
}
