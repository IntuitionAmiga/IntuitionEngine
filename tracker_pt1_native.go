// tracker_pt1_native.go - Native Go ProTracker 1 player.
// Ported from zxtune reference (vitamin-caig/zxtune, module/players/aym/protracker1.cpp
// + formats/chiptune/aym/protracker1.cpp).
//
// PT1 modules are rendered to AY register frames entirely in Go — no Z80 emulation needed.
// The frames feed into the existing loadFrames() → PSGEvents pipeline.

package main

import "fmt"

// PT1 frequency table — TABLE_PROTRACKER3_ST from zxtune (96 entries)
var pt1FreqTable = [96]uint16{
	0x0EF8, 0x0E10, 0x0D60, 0x0C80, 0x0BD8, 0x0B28, 0x0A88, 0x09F0, 0x0960, 0x08E0, 0x0858, 0x07E0,
	0x077C, 0x0708, 0x06B0, 0x0640, 0x05EC, 0x0594, 0x0544, 0x04F8, 0x04B0, 0x0470, 0x042C, 0x03FD,
	0x03BE, 0x0384, 0x0358, 0x0320, 0x02F6, 0x02CA, 0x02A2, 0x027C, 0x0258, 0x0238, 0x0216, 0x01F8,
	0x01DF, 0x01C2, 0x01AC, 0x0190, 0x017B, 0x0165, 0x0151, 0x013E, 0x012C, 0x011C, 0x010A, 0x00FC,
	0x00EF, 0x00E1, 0x00D6, 0x00C8, 0x00BD, 0x00B2, 0x00A8, 0x009F, 0x0096, 0x008E, 0x0085, 0x007E,
	0x0077, 0x0070, 0x006B, 0x0064, 0x005E, 0x0059, 0x0054, 0x004F, 0x004B, 0x0047, 0x0042, 0x003F,
	0x003B, 0x0038, 0x0035, 0x0032, 0x002F, 0x002C, 0x002A, 0x0027, 0x0025, 0x0023, 0x0021, 0x001F,
	0x001D, 0x001C, 0x001A, 0x0019, 0x0017, 0x0016, 0x0015, 0x0013, 0x0012, 0x0011, 0x0010, 0x000F,
}

type pt1SampleLine struct {
	level     uint8 // 0-15
	vibrato   int16 // signed 12-bit tone offset
	toneMask  bool  // true = tone disabled
	noiseMask bool  // true = noise disabled
	noise     uint8 // noise value 0-31
}

type pt1Sample struct {
	lines []pt1SampleLine
	loop  int
}

type pt1Ornament struct {
	lines []int8 // signed note offsets, always 64 entries
}

type pt1ChannelState struct {
	enabled     bool
	envelope    bool
	note        int
	sampleNum   int
	posInSample int
	ornamentNum int
	volume      int // 0-15
}

func pt1GetVolume(volume, level int) int {
	extra := 0
	if volume > 7 {
		extra = 1
	}
	return ((volume*17+extra)*level + 128) >> 8
}

// renderPT1Native parses a PT1 module and renders it to AY register frames.
// Returns (frames, loopFrame, error).
func renderPT1Native(data []byte) ([][]uint8, uint32, error) {
	if len(data) < 100 {
		return nil, 0, fmt.Errorf("pt1: data too short (%d bytes)", len(data))
	}

	// Parse header (RawHeader from zxtune)
	tempo := int(data[0])
	length := int(data[1])
	loopPos := int(data[2])
	if tempo < 2 {
		tempo = 3
	}

	// Sample offsets: 16 × uint16LE at offset 3
	sampleOffsets := make([]int, 16)
	for i := range 16 {
		sampleOffsets[i] = int(data[3+i*2]) | int(data[4+i*2])<<8
	}

	// Ornament offsets: 16 × uint16LE at offset 35
	ornamentOffsets := make([]int, 16)
	for i := range 16 {
		ornamentOffsets[i] = int(data[35+i*2]) | int(data[36+i*2])<<8
	}

	// Patterns offset: uint16LE at offset 67
	patternsOffset := int(data[67]) | int(data[68])<<8

	// Positions: starting at offset 99, until 0xFF
	var positions []int
	for i := 99; i < len(data) && data[i] != 0xFF; i++ {
		positions = append(positions, int(data[i]))
	}
	if length > len(positions) {
		length = len(positions)
	}
	if length == 0 {
		return nil, 0, fmt.Errorf("pt1: no positions")
	}
	if loopPos >= length {
		loopPos = 0
	}

	// Parse samples
	samples := make([]pt1Sample, 16)
	for i := range 16 {
		off := sampleOffsets[i]
		if off == 0 || off+2 > len(data) {
			samples[i] = pt1Sample{lines: []pt1SampleLine{{level: 0}}, loop: 0}
			continue
		}
		size := int(data[off])
		sloop := int(data[off+1])
		if size == 0 {
			samples[i] = pt1Sample{lines: []pt1SampleLine{{level: 0}}, loop: 0}
			continue
		}
		lines := make([]pt1SampleLine, size)
		for j := range size {
			// Use 8-bit index wrapping like zxtune: offset = uint8(idx * 3)
			lineOff := off + 2 + (j*3)&0xFF
			if lineOff+3 > len(data) {
				break
			}
			b0 := data[lineOff]   // HHHHaaaa
			b1 := data[lineOff+1] // NTsnnnnn
			b2 := data[lineOff+2] // LLLLLLLL

			lines[j].level = b0 & 0x0F
			vibratoHi := int16(b0&0xF0) << 4 // HHHH shifted to bits 11-8
			vibratoLo := int16(b2)           // LLLLLLLL
			val := vibratoHi | vibratoLo
			if b1&0x20 != 0 { // sign bit: 1 = positive
				lines[j].vibrato = val
			} else {
				lines[j].vibrato = -val
			}
			lines[j].noiseMask = b1&0x80 != 0
			lines[j].toneMask = b1&0x40 != 0
			lines[j].noise = b1 & 0x1F
		}
		if sloop > size {
			sloop = 0
		}
		samples[i] = pt1Sample{lines: lines, loop: sloop}
	}

	// Parse ornaments (64 × int8 each)
	ornaments := make([]pt1Ornament, 16)
	for i := range 16 {
		off := ornamentOffsets[i]
		lines := make([]int8, 64)
		if off != 0 && off+64 <= len(data) {
			for j := range 64 {
				lines[j] = int8(data[off+j])
			}
		}
		ornaments[i] = pt1Ornament{lines: lines}
	}

	// Render
	channels := [3]pt1ChannelState{
		{volume: 15},
		{volume: 15},
		{volume: 15},
	}

	speed := tempo
	var frames [][]uint8
	const maxFrames = 60 * 60 * 50 // 60 minutes max

	var envType, envToneLo, envToneHi uint8
	envWritten := false
	loopFrame := uint32(0)

	for posIdx := range length {
		if posIdx == loopPos {
			loopFrame = uint32(len(frames))
		}

		patIdx := positions[posIdx]
		patOff := patternsOffset + patIdx*6
		if patOff+6 > len(data) {
			break
		}
		chanOffsets := [3]int{
			int(data[patOff]) | int(data[patOff+1])<<8,
			int(data[patOff+2]) | int(data[patOff+3])<<8,
			int(data[patOff+4]) | int(data[patOff+5])<<8,
		}

		type chanParse struct {
			offset  int
			period  int
			counter int
		}
		cps := [3]chanParse{
			{offset: chanOffsets[0]},
			{offset: chanOffsets[1]},
			{offset: chanOffsets[2]},
		}

		for lineIdx := 0; lineIdx < 64 && len(frames) < maxFrames; lineIdx++ {
			// Check for pattern end: channel 0 sees 0xFF
			if cps[0].offset >= len(data) || data[cps[0].offset] == 0xFF {
				break
			}

			// Parse commands for channels with counter==0
			for ch := range 3 {
				if cps[ch].counter > 0 {
					cps[ch].counter--
					continue
				}

				for cps[ch].offset < len(data) {
					cmd := data[cps[ch].offset]
					cps[ch].offset++

					if cmd <= 0x5F {
						channels[ch].note = int(cmd)
						channels[ch].enabled = true
						channels[ch].posInSample = 0
						break
					} else if cmd <= 0x6F {
						channels[ch].sampleNum = int(cmd - 0x60)
					} else if cmd <= 0x7F {
						channels[ch].ornamentNum = int(cmd - 0x70)
					} else if cmd == 0x80 {
						channels[ch].enabled = false
						break
					} else if cmd == 0x81 {
						channels[ch].envelope = false
					} else if cmd <= 0x8F {
						envType = cmd - 0x81
						envWritten = true
						if cps[ch].offset+2 <= len(data) {
							envToneLo = data[cps[ch].offset]
							envToneHi = data[cps[ch].offset+1]
							cps[ch].offset += 2
						}
						channels[ch].envelope = true
					} else if cmd == 0x90 {
						break // empty, no note change
					} else if cmd <= 0xA0 {
						speed = max(int(cmd-0x91), 1)
					} else if cmd <= 0xB0 {
						channels[ch].volume = int(cmd - 0xA1)
					} else {
						cps[ch].period = int(cmd - 0xB1)
					}
				}
				cps[ch].counter = cps[ch].period
			}

			// Generate 'speed' frames for this row
			for tick := range speed {
				_ = tick
				frame := pt1SynthesizeFrame(&channels, samples, ornaments, envType, envToneLo, envToneHi, envWritten)
				frames = append(frames, frame)
				pt1AdvanceSamples(&channels, samples)
				if len(frames) >= maxFrames {
					break
				}
			}
		}
	}

	if len(frames) == 0 {
		return nil, 0, fmt.Errorf("pt1: no frames generated")
	}

	return frames, loopFrame, nil
}

func pt1SynthesizeFrame(channels *[3]pt1ChannelState, samples []pt1Sample, ornaments []pt1Ornament, envType, envToneLo, envToneHi uint8, envWritten bool) []uint8 {
	frame := make([]uint8, 14)
	frame[13] = 0xFF // sentinel: don't write R13 unless envelope command
	mixer := uint8(0)
	noise := uint8(0)

	for ch := range 3 {
		if !channels[ch].enabled {
			frame[8+ch] = 0
			mixer |= (1 << ch) | (1 << (ch + 3))
			continue
		}

		samIdx := channels[ch].sampleNum
		if samIdx >= len(samples) {
			samIdx = 0
		}
		sam := &samples[samIdx]
		pos := channels[ch].posInSample
		if pos >= len(sam.lines) {
			pos = sam.loop
			if pos >= len(sam.lines) {
				pos = 0
			}
		}
		line := sam.lines[pos]

		ornIdx := channels[ch].ornamentNum
		if ornIdx >= len(ornaments) {
			ornIdx = 0
		}
		orn := &ornaments[ornIdx]
		noteAddon := int8(0)
		ornPos := pos % len(orn.lines)
		noteAddon = orn.lines[ornPos]

		halfTone := min(max(channels[ch].note+int(noteAddon), 0), 95)

		tonePeriod := int(pt1FreqTable[halfTone]) + int(line.vibrato)
		// Quirk from zxtune: +1 when halftone == 46
		if halfTone == 46 {
			tonePeriod++
		}
		if tonePeriod < 0 {
			tonePeriod = 0
		}
		if tonePeriod > 0xFFF {
			tonePeriod = 0xFFF
		}

		frame[ch*2] = uint8(tonePeriod & 0xFF)
		frame[ch*2+1] = uint8((tonePeriod >> 8) & 0x0F)

		vol := pt1GetVolume(channels[ch].volume, int(line.level))
		if channels[ch].envelope {
			frame[8+ch] = uint8(vol) | 0x10
		} else {
			frame[8+ch] = uint8(vol)
		}

		if line.toneMask {
			mixer |= 1 << ch
		}
		if line.noiseMask {
			mixer |= 1 << (ch + 3)
		} else {
			noise = line.noise
		}
	}

	frame[6] = noise
	frame[7] = mixer
	frame[11] = envToneLo
	frame[12] = envToneHi
	if envWritten {
		frame[13] = envType
	}

	return frame
}

func pt1AdvanceSamples(channels *[3]pt1ChannelState, samples []pt1Sample) {
	for ch := range 3 {
		if !channels[ch].enabled {
			continue
		}
		samIdx := channels[ch].sampleNum
		if samIdx >= len(samples) {
			samIdx = 0
		}
		sam := &samples[samIdx]
		channels[ch].posInSample++
		if channels[ch].posInSample >= len(sam.lines) {
			channels[ch].posInSample = sam.loop
			if channels[ch].posInSample >= len(sam.lines) {
				channels[ch].posInSample = 0
			}
		}
	}
}
