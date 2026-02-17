// tracker_asc_native.go - Native Go ASC Sound Master player.
// Ported from zxtune reference (vitamin-caig/zxtune, module/players/aym/ascsoundmaster.cpp
// + formats/chiptune/aym/ascsoundmaster.cpp).
//
// ASC modules have two format versions (v0 and v1) with slightly different header layouts.
// Rendered entirely in Go — no Z80 emulation needed.

package main

import "fmt"

// ASC frequency table — TABLE_ASM from zxtune (96 entries)
var ascFreqTable = [96]uint16{
	0x0EDC, 0x0E07, 0x0D3E, 0x0C80, 0x0BCC, 0x0B22, 0x0A82, 0x09EC, 0x095C, 0x08D6, 0x0858, 0x07E0,
	0x076E, 0x0704, 0x069F, 0x0640, 0x05E6, 0x0591, 0x0541, 0x04F6, 0x04AE, 0x046B, 0x042C, 0x03F0,
	0x03B7, 0x0382, 0x034F, 0x0320, 0x02F3, 0x02C8, 0x02A1, 0x027B, 0x0257, 0x0236, 0x0216, 0x01F8,
	0x01DC, 0x01C1, 0x01A8, 0x0190, 0x0179, 0x0164, 0x0150, 0x013D, 0x012C, 0x011B, 0x010B, 0x00FC,
	0x00EE, 0x00E0, 0x00D4, 0x00C8, 0x00BD, 0x00B2, 0x00A8, 0x009F, 0x0096, 0x008D, 0x0085, 0x007E,
	0x0077, 0x0070, 0x006A, 0x0064, 0x005E, 0x0059, 0x0054, 0x0050, 0x004B, 0x0047, 0x0043, 0x003F,
	0x003C, 0x0038, 0x0035, 0x0032, 0x002F, 0x002D, 0x002A, 0x0028, 0x0026, 0x0024, 0x0022, 0x0020,
	0x001E, 0x001C, 0x001A, 0x0019, 0x0017, 0x0016, 0x0015, 0x0014, 0x0013, 0x0012, 0x0011, 0x0010,
}

// --- ASC data types ---

type ascSampleLine struct {
	loopBegin      bool
	loopEnd        bool
	finished       bool
	adding         int  // signed 5-bit
	toneDeviation  int8 // signed 8-bit
	level          int  // 0-15
	noiseMask      bool
	enableEnvelope bool // command == 1
	volSlide       int  // -1, 0, or +1 (command 2/3)
	toneMask       bool
}

type ascSample struct {
	lines     []ascSampleLine
	loop      int // loop begin index
	loopLimit int // loop end index
	size      int
}

type ascOrnamentLine struct {
	loopBegin   bool
	loopEnd     bool
	finished    bool
	noiseOffset int  // signed 5-bit
	noteOffset  int8 // signed 8-bit
}

type ascOrnament struct {
	lines     []ascOrnamentLine
	loop      int
	loopLimit int
	size      int
}

type ascChannelState struct {
	enabled            bool
	envelope           bool
	breakSample        bool
	volume             int // 0-15
	volumeAddon        int
	volSlideDelay      int
	volSlideAddon      int
	volSlideCounter    int
	baseNoise          int
	currentNoise       int
	note               int
	noteAddon          int
	sampleNum          int
	currentSampleNum   int
	posInSample        int
	ornamentNum        int
	currentOrnamentNum int
	posInOrnament      int
	toneDeviation      int
	slidingSteps       int // may be negative (infinite)
	sliding            int
	slidingTargetNote  int // -1 = no target (LIMITER)
	glissade           int
}

// renderASCNative parses an ASC module and renders it to AY register frames.
func renderASCNative(data []byte) ([][]uint8, uint32, error) {
	if len(data) < 10 {
		return nil, 0, fmt.Errorf("asc: data too short (%d bytes)", len(data))
	}

	// Detect version: try v1 first, fall back to v0
	var tempo, loopPos, length int
	var patternsOff, samplesOff, ornamentsOff int
	var positionsStart int
	ver := 1

	// v1: Tempo(1), Loop(1), PatternsOffset(u16LE), SamplesOffset(u16LE), OrnamentsOffset(u16LE), Length(1), Positions[Length]
	// v0: Tempo(1), PatternsOffset(u16LE), SamplesOffset(u16LE), OrnamentsOffset(u16LE), Length(1), Positions[Length]
	tempo1 := int(data[0])
	// Try v1 first
	if tempo1 >= 3 && tempo1 <= 50 {
		loop1 := int(data[1])
		pat1 := int(data[2]) | int(data[3])<<8
		sam1 := int(data[4]) | int(data[5])<<8
		orn1 := int(data[6]) | int(data[7])<<8
		len1 := int(data[8])

		// Validate v1: offsets must be ordered (pat <= sam <= orn) per reference,
		// patterns must start after header+positions, and position values < 32.
		v1Valid := loop1 <= 99 && len1 >= 1 && len1 <= 100 &&
			pat1 >= 9+len1 && pat1 < len(data) &&
			sam1 >= pat1 && sam1 < len(data) &&
			orn1 >= sam1 && orn1 < len(data) &&
			9+len1 <= len(data)
		if v1Valid {
			// Validate position values < 32 (MAX_PATTERNS_COUNT)
			for i := range len1 {
				if data[9+i] >= 32 {
					v1Valid = false
					break
				}
			}
		}
		if v1Valid {
			tempo = tempo1
			loopPos = loop1
			patternsOff = pat1
			samplesOff = sam1
			ornamentsOff = orn1
			length = len1
			positionsStart = 9
		} else {
			ver = 0
		}
	} else {
		ver = 0
	}

	if ver == 0 {
		// v0: Tempo(1), PatternsOffset(u16LE), SamplesOffset(u16LE), OrnamentsOffset(u16LE), Length(1), Positions[Length]
		tempo = int(data[0])
		patternsOff = int(data[1]) | int(data[2])<<8
		samplesOff = int(data[3]) | int(data[4])<<8
		ornamentsOff = int(data[5]) | int(data[6])<<8
		length = int(data[7])
		positionsStart = 8
		loopPos = 0 // v0 has no explicit loop
	}

	if tempo < 3 {
		tempo = 3
	}
	if length == 0 || positionsStart+length > len(data) {
		return nil, 0, fmt.Errorf("asc: invalid length %d", length)
	}

	// Read positions
	positions := make([]int, length)
	for i := range length {
		positions[i] = int(data[positionsStart+i])
	}
	if loopPos >= length {
		loopPos = 0
	}

	// Parse samples (32 entries, offsets from samplesOff)
	samples := make([]ascSample, 32)
	if samplesOff+64 <= len(data) {
		for i := range 32 {
			soff := int(data[samplesOff+i*2]) | int(data[samplesOff+i*2+1])<<8
			samples[i] = ascParseSample(data, soff)
		}
	}

	// Parse ornaments (32 entries, offsets from ornamentsOff)
	ornaments := make([]ascOrnament, 32)
	if ornamentsOff+64 <= len(data) {
		for i := range 32 {
			ooff := int(data[ornamentsOff+i*2]) | int(data[ornamentsOff+i*2+1])<<8
			ornaments[i] = ascParseOrnament(data, ooff)
		}
	}

	// Render
	channels := [3]ascChannelState{}
	for ch := range 3 {
		channels[ch].volume = 15
		channels[ch].slidingTargetNote = -1 // LIMITER
	}

	speed := tempo
	var frames [][]uint8
	const maxFrames = 60 * 60 * 50

	envTone := 0
	envType := -1 // -1 = not set
	envTypeWritePending := false
	loopFrame := uint32(0)

	for posIdx := range length {
		if posIdx == loopPos {
			loopFrame = uint32(len(frames))
		}

		patIdx := positions[posIdx]
		patOff := patternsOff + patIdx*6
		if patOff+6 > len(data) {
			break
		}
		chanOffsets := [3]int{
			int(data[patOff]) | int(data[patOff+1])<<8,
			int(data[patOff+2]) | int(data[patOff+3])<<8,
			int(data[patOff+4]) | int(data[patOff+5])<<8,
		}

		// Reset base noise at pattern start
		for ch := range 3 {
			channels[ch].baseNoise = 0
		}

		type chanParse struct {
			offset   int
			period   int
			counter  int
			envelope bool // envelope mode: read extra byte after notes
		}
		cps := [3]chanParse{
			{offset: chanOffsets[0]},
			{offset: chanOffsets[1]},
			{offset: chanOffsets[2]},
		}

		for lineIdx := 0; lineIdx < 64 && len(frames) < maxFrames; lineIdx++ {
			// Check for pattern end (channel 0 sees 0xFF, or any channel offset out of range)
			if cps[0].offset >= len(data) || data[cps[0].offset] == 0xFF {
				break
			}

			for ch := range 3 {
				if cps[ch].counter > 0 {
					cps[ch].counter--
					continue
				}

				dst := &channels[ch]
				dst.volSlideCounter = 0
				dst.slidingSteps = 0
				contSample := false
				contOrnament := false
				reloadNote := false

				// Track pending slide for SLIDE_NOTE conversion
				pendingSlide := false
				pendingSlideSteps := 0
				pendingSlideUseTone := false
				pendingSlideUsedAsNote := false

				for cps[ch].offset < len(data) {
					cmd := data[cps[ch].offset]
					cps[ch].offset++

					if cmd <= 0x55 {
						// Note
						if !dst.breakSample {
							dst.enabled = true
						}

						if pendingSlide {
							// SLIDE_NOTE: slide from current note to this note
							targetNote := int(cmd)
							dst.slidingSteps = pendingSlideSteps
							dst.slidingTargetNote = targetNote
							fromIdx := ascClampNote(dst.note)
							toIdx := ascClampNote(targetNote)
							absoluteSliding := int(ascFreqTable[toIdx]) - int(ascFreqTable[fromIdx])
							newSliding := absoluteSliding
							if pendingSlideUseTone {
								newSliding -= dst.sliding / 16
							}
							if pendingSlideSteps != 0 {
								dst.glissade = 16 * newSliding / pendingSlideSteps
							} else {
								dst.glissade = 16 * newSliding
							}
							pendingSlideUsedAsNote = true
							// Don't change dst.note — sliding handles the transition
						} else {
							dst.note = int(cmd)
						}
						reloadNote = true

						// In envelope mode, read extra byte as envelope tone
						if cps[ch].envelope && cps[ch].offset < len(data) {
							envTone = int(data[cps[ch].offset])
							cps[ch].offset++
						}
						break

					} else if cmd <= 0x5D {
						// Stop (0x56-0x5D): break without changing anything
						break
					} else if cmd == 0x5E {
						dst.breakSample = true
						break
					} else if cmd == 0x5F {
						dst.enabled = false
						break
					} else if cmd <= 0x9F {
						// Period: cmd - 0x60
						cps[ch].period = int(cmd - 0x60)
					} else if cmd <= 0xBF {
						// Sample: cmd - 0xA0
						dst.sampleNum = int(cmd - 0xA0)
					} else if cmd <= 0xDF {
						// Ornament: cmd - 0xC0
						dst.ornamentNum = int(cmd - 0xC0)
					} else if cmd == 0xE0 {
						// Envelope + volume 15
						dst.envelope = true
						dst.volume = 15
						cps[ch].envelope = true
					} else if cmd <= 0xEF {
						// Volume + no envelope: 0xE1=vol 1, 0xEF=vol 15
						dst.volume = int(cmd - 0xE0)
						dst.envelope = false
						cps[ch].envelope = false
					} else if cmd == 0xF0 {
						// Noise
						if cps[ch].offset < len(data) {
							dst.baseNoise = int(data[cps[ch].offset])
							cps[ch].offset++
						}
					} else if cmd >= 0xF1 && cmd <= 0xF3 {
						// Continue: f1=sample, f2=ornament, f3=both
						if cmd == 0xF1 || cmd == 0xF3 {
							contSample = true
						}
						if cmd == 0xF2 || cmd == 0xF3 {
							contOrnament = true
						}
					} else if cmd == 0xF4 {
						// Tempo change
						if cps[ch].offset < len(data) {
							speed = int(data[cps[ch].offset])
							cps[ch].offset++
							if speed < 1 {
								speed = 1
							}
						}
					} else if cmd == 0xF5 || cmd == 0xF6 {
						// Glissade: 0xF5 = negative (×-16), 0xF6 = positive (×16)
						if cps[ch].offset < len(data) {
							g := int(data[cps[ch].offset]) * 16
							cps[ch].offset++
							if cmd == 0xF5 {
								g = -g
							}
							dst.glissade = g
							dst.slidingSteps = -1 // infinite
						}
					} else if cmd == 0xF7 || cmd == 0xF9 {
						// Slide: 0xF7 = continue sample + use tone sliding, 0xF9 = no continue
						if cps[ch].offset < len(data) {
							pendingSlideSteps = int(int8(data[cps[ch].offset]))
							cps[ch].offset++
							pendingSlide = true
							pendingSlideUseTone = (cmd == 0xF7)
							if cmd == 0xF7 {
								contSample = true
							}
						}
					} else if (cmd & 0xF9) == 0xF8 {
						// Envelope type: 0xF8, 0xFA, 0xFC, 0xFE
						// Type comes from cmd & 0xF, no extra bytes
						envType = int(cmd & 0x0F)
						envTypeWritePending = true
					} else if cmd == 0xFB {
						// Amplitude slide: 1 byte packed (bits 0-4=period, bit 5=direction)
						if cps[ch].offset < len(data) {
							step := data[cps[ch].offset]
							cps[ch].offset++
							dst.volSlideDelay = int(step & 31)
							if step&32 != 0 {
								dst.volSlideAddon = -1
							} else {
								dst.volSlideAddon = 1
							}
							dst.volSlideCounter = dst.volSlideDelay
						}
					}
				}

				// If slide was pending but no note came, apply as pure SLIDE
				if pendingSlide && !pendingSlideUsedAsNote {
					dst.slidingSteps = pendingSlideSteps
					newSliding := (dst.sliding | 0xF) ^ 0xF
					if pendingSlideSteps != 0 {
						dst.glissade = -newSliding / pendingSlideSteps
					} else {
						dst.glissade = -newSliding
					}
					dst.sliding = dst.glissade * pendingSlideSteps
				}

				if reloadNote {
					dst.currentNoise = dst.baseNoise
					if dst.slidingSteps <= 0 {
						dst.sliding = 0
					}
					if !contSample {
						dst.currentSampleNum = dst.sampleNum
						dst.posInSample = 0
						dst.volumeAddon = 0
						dst.toneDeviation = 0
						dst.breakSample = false
					}
					if !contOrnament {
						dst.currentOrnamentNum = dst.ornamentNum
						dst.posInOrnament = 0
						dst.noteAddon = 0
					}
				}

				cps[ch].counter = cps[ch].period
			}

			// Generate 'speed' frames for this row
			for tick := range speed {
				_ = tick
				frame := ascSynthesizeFrame(&channels, samples, ornaments, &envTone,
					envType, envTypeWritePending)
				envTypeWritePending = false // only write R13 on first tick of line
				frames = append(frames, frame)
				if len(frames) >= maxFrames {
					break
				}
			}
		}
	}

	if len(frames) == 0 {
		return nil, 0, fmt.Errorf("asc: no frames generated")
	}
	return frames, loopFrame, nil
}

func ascParseSample(data []byte, off int) ascSample {
	stub := ascSample{
		lines:     []ascSampleLine{{level: 0, noiseMask: true, toneMask: true}},
		loop:      0,
		loopLimit: 0,
		size:      1,
	}
	if off == 0 || off+3 > len(data) {
		return stub
	}

	// Parse sample lines. Each line is 3 bytes: BEFaaaaa, TTTTTTTT, LLLLnCCt
	// CC (command): 0=empty, 1=envelope, 2=decVolAdd, 3=incVolAdd
	var lines []ascSampleLine
	loopBegin := -1
	loopEnd := -1

	for i := range 150 { // max sample size
		lineOff := off + i*3
		if lineOff+3 > len(data) {
			break
		}
		b0 := data[lineOff]
		b1 := data[lineOff+1]
		b2 := data[lineOff+2]

		command := int((b2 & 0x06) >> 1)
		line := ascSampleLine{
			loopBegin:     b0&0x80 != 0,
			loopEnd:       b0&0x40 != 0,
			finished:      b0&0x20 != 0,
			adding:        int(int8(b0<<3) >> 3), // sign-extend 5-bit
			toneDeviation: int8(b1),
			level:         int(b2 >> 4),
			noiseMask:     b2&0x08 != 0,
			toneMask:      b2&0x01 != 0,
		}
		// Decode command into separate fields (mutually exclusive in format)
		switch command {
		case 1: // ENVELOPE
			line.enableEnvelope = true
		case 2: // DECVOLADD
			line.volSlide = -1
		case 3: // INCVOLADD
			line.volSlide = 1
		}

		if line.loopBegin {
			loopBegin = len(lines)
		}
		if line.loopEnd {
			loopEnd = len(lines)
		}

		lines = append(lines, line)

		if line.finished {
			break
		}
	}

	if len(lines) == 0 {
		return stub
	}

	sloop := max(loopBegin, 0)
	sloopLimit := len(lines) - 1
	if loopEnd >= 0 {
		sloopLimit = loopEnd
	}

	return ascSample{
		lines:     lines,
		loop:      sloop,
		loopLimit: sloopLimit,
		size:      len(lines),
	}
}

func ascParseOrnament(data []byte, off int) ascOrnament {
	stub := ascOrnament{
		lines:     []ascOrnamentLine{{}},
		loop:      0,
		loopLimit: 0,
		size:      1,
	}
	if off == 0 || off+2 > len(data) {
		return stub
	}

	var lines []ascOrnamentLine
	loopBegin := -1
	loopEnd := -1

	for i := range 30 { // max ornament size
		lineOff := off + i*2
		if lineOff+2 > len(data) {
			break
		}
		b0 := data[lineOff]
		b1 := data[lineOff+1]

		line := ascOrnamentLine{
			loopBegin:   b0&0x80 != 0,
			loopEnd:     b0&0x40 != 0,
			finished:    b0&0x20 != 0,
			noiseOffset: int(int8(b0<<3) >> 3), // sign-extend 5-bit
			noteOffset:  int8(b1),
		}

		if line.loopBegin {
			loopBegin = len(lines)
		}
		if line.loopEnd {
			loopEnd = len(lines)
		}

		lines = append(lines, line)

		if line.finished {
			break
		}
	}

	if len(lines) == 0 {
		return stub
	}

	oloop := max(loopBegin, 0)
	oloopLimit := len(lines) - 1
	if loopEnd >= 0 {
		oloopLimit = loopEnd
	}

	return ascOrnament{
		lines:     lines,
		loop:      oloop,
		loopLimit: oloopLimit,
		size:      len(lines),
	}
}

func ascSynthesizeFrame(channels *[3]ascChannelState, samples []ascSample, ornaments []ascOrnament,
	envTone *int, envType int, envTypeWrite bool) []uint8 {
	frame := make([]uint8, 14)
	frame[13] = 0xFF // sentinel: don't write R13 unless envelope command
	mixer := uint8(0)
	noise := uint8(0)

	for ch := range 3 {
		dst := &channels[ch]

		if !dst.enabled {
			frame[8+ch] = 0
			mixer |= (1 << ch) | (1 << (ch + 3))
			continue
		}

		samIdx := dst.currentSampleNum
		if samIdx >= len(samples) {
			samIdx = 0
		}
		sam := &samples[samIdx]

		ornIdx := dst.currentOrnamentNum
		if ornIdx >= len(ornaments) {
			ornIdx = 0
		}
		orn := &ornaments[ornIdx]

		// Get current sample line
		spos := dst.posInSample
		if spos >= len(sam.lines) {
			spos = sam.loop
			if spos >= len(sam.lines) {
				spos = 0
			}
		}
		sline := sam.lines[spos]

		// Get current ornament line
		opos := dst.posInOrnament
		if opos >= len(orn.lines) {
			opos = orn.loop
			if opos >= len(orn.lines) {
				opos = 0
			}
		}
		oline := orn.lines[opos]

		// Volume addon processing
		if dst.volSlideCounter >= 2 {
			dst.volSlideCounter--
		} else if dst.volSlideCounter == 1 {
			dst.volumeAddon += dst.volSlideAddon
			dst.volSlideCounter = dst.volSlideDelay
		}
		dst.volumeAddon += sline.volSlide
		if dst.volumeAddon < -15 {
			dst.volumeAddon = -15
		}
		if dst.volumeAddon > 15 {
			dst.volumeAddon = 15
		}

		// Tone calculation
		dst.toneDeviation += int(sline.toneDeviation)
		dst.noteAddon += int(oline.noteOffset)
		halfTone := min(max(int(dst.note)+int(dst.noteAddon), 0), 0x55)

		toneAddon := dst.toneDeviation + dst.sliding/16
		tonePeriod := min(max(int(ascFreqTable[halfTone])+toneAddon, 0), 0xFFF)

		frame[ch*2] = uint8(tonePeriod & 0xFF)
		frame[ch*2+1] = uint8((tonePeriod >> 8) & 0x0F)

		// Volume: (Volume+1) * clamp(VolumeAddon+Level, 0, 15) / 16
		level := min(max(dst.volumeAddon+sline.level, 0), 15)
		vol := (dst.volume + 1) * level / 16

		// Envelope: both channel state and sample line must agree
		if dst.envelope && sline.enableEnvelope {
			frame[8+ch] = uint8(vol) | 0x10
		} else {
			frame[8+ch] = uint8(vol)
		}

		// Noise
		dst.currentNoise += int(oline.noiseOffset)

		// Mixer
		if sline.toneMask {
			mixer |= 1 << ch
		}
		if sline.noiseMask && sline.enableEnvelope {
			// Noise mask + envelope: adding goes to envelope tone
			*envTone += sline.adding
		} else {
			dst.currentNoise += sline.adding
		}

		if !sline.noiseMask {
			noise = uint8((dst.currentNoise + dst.sliding/256) & 0x1F)
		} else {
			mixer |= 1 << (ch + 3)
		}

		// Sliding
		if dst.slidingSteps != 0 {
			if dst.slidingSteps > 0 {
				dst.slidingSteps--
				if dst.slidingSteps == 0 && dst.slidingTargetNote >= 0 {
					dst.note = dst.slidingTargetNote
					dst.slidingTargetNote = -1
					dst.sliding = 0
					dst.glissade = 0
				}
			}
			dst.sliding += dst.glissade
		}

		// Advance sample position
		dst.posInSample++
		if dst.posInSample > sam.loopLimit {
			if !dst.breakSample {
				dst.posInSample = sam.loop
			} else if dst.posInSample >= sam.size {
				dst.enabled = false
			}
		}

		// Advance ornament position
		dst.posInOrnament++
		if dst.posInOrnament > orn.loopLimit {
			dst.posInOrnament = orn.loop
		}
	}

	frame[6] = noise
	frame[7] = mixer
	// Always write envelope tone (R11/R12) — no side effects
	frame[11] = uint8(*envTone & 0xFF)
	frame[12] = uint8((*envTone >> 8) & 0xFF)
	// Write envelope type (R13) only when command sets it
	if envTypeWrite && envType >= 0 {
		frame[13] = uint8(envType)
	}

	return frame
}

func ascClampNote(note int) int {
	if note < 0 {
		return 0
	}
	if note > 95 {
		return 95
	}
	return note
}
