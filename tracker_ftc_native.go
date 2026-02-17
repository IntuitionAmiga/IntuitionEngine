// tracker_ftc_native.go - Native Go Fast Tracker (ZX) player.
// Ported from zxtune reference (vitamin-caig/zxtune, module/players/aym/fasttracker.cpp
// + formats/chiptune/aym/fasttracker.cpp).
//
// FTC modules are rendered to AY register frames entirely in Go — no Z80 emulation needed.

package main

import "fmt"

// FTC frequency tables from zxtune

var ftcTableSoundTracker = [96]uint16{
	0x0EF8, 0x0E10, 0x0D60, 0x0C80, 0x0BD8, 0x0B28, 0x0A88, 0x09F0, 0x0960, 0x08E0, 0x0858, 0x07E0,
	0x077C, 0x0708, 0x06B0, 0x0640, 0x05EC, 0x0594, 0x0544, 0x04F8, 0x04B0, 0x0470, 0x042C, 0x03F0,
	0x03BE, 0x0384, 0x0358, 0x0320, 0x02F6, 0x02CA, 0x02A2, 0x027C, 0x0258, 0x0238, 0x0216, 0x01F8,
	0x01DF, 0x01C2, 0x01AC, 0x0190, 0x017B, 0x0165, 0x0151, 0x013E, 0x012C, 0x011C, 0x010B, 0x00FC,
	0x00EF, 0x00E1, 0x00D6, 0x00C8, 0x00BD, 0x00B2, 0x00A8, 0x009F, 0x0096, 0x008E, 0x0085, 0x007E,
	0x0077, 0x0070, 0x006B, 0x0064, 0x005E, 0x0059, 0x0054, 0x004F, 0x004B, 0x0047, 0x0042, 0x003F,
	0x003B, 0x0038, 0x0035, 0x0032, 0x002F, 0x002C, 0x002A, 0x0027, 0x0025, 0x0023, 0x0021, 0x001F,
	0x001D, 0x001C, 0x001A, 0x0019, 0x0017, 0x0016, 0x0015, 0x0013, 0x0012, 0x0011, 0x0010, 0x000F,
}

var ftcTableFastTracker = [96]uint16{
	0x0C22, 0x0B73, 0x0ACF, 0x0A33, 0x09A1, 0x0917, 0x0894, 0x0819, 0x07A4, 0x0737, 0x06CF, 0x066D,
	0x0611, 0x05BA, 0x0567, 0x051A, 0x04D0, 0x048B, 0x044A, 0x040C, 0x03D2, 0x039B, 0x0367, 0x0337,
	0x0308, 0x02DD, 0x02B4, 0x028D, 0x0268, 0x0246, 0x0225, 0x0206, 0x01E9, 0x01CE, 0x01B4, 0x019B,
	0x0184, 0x016E, 0x015A, 0x0146, 0x0134, 0x0123, 0x0113, 0x0103, 0x00F5, 0x00E7, 0x00DA, 0x00CE,
	0x00C2, 0x00B7, 0x00AD, 0x00A3, 0x009A, 0x0091, 0x0089, 0x0082, 0x007A, 0x0074, 0x006D, 0x0067,
	0x0061, 0x005C, 0x0056, 0x0052, 0x004D, 0x0049, 0x0045, 0x0041, 0x003D, 0x003A, 0x0036, 0x0033,
	0x0031, 0x002E, 0x002B, 0x0029, 0x0027, 0x0024, 0x0022, 0x0020, 0x001F, 0x001D, 0x001B, 0x001A,
	0x0018, 0x0017, 0x0016, 0x0014, 0x0013, 0x0012, 0x0011, 0x0010, 0x000F, 0x000E, 0x000D, 0x000D,
}

var ftcTableProTracker2 = [96]uint16{
	0x0D10, 0x0C55, 0x0BA4, 0x0AFC, 0x0A5F, 0x09CA, 0x093D, 0x08B8, 0x083B, 0x07C5, 0x0755, 0x06EC,
	0x0688, 0x062A, 0x05D2, 0x057E, 0x052F, 0x04E5, 0x049E, 0x045C, 0x041D, 0x03E2, 0x03AB, 0x0376,
	0x0344, 0x0315, 0x02E9, 0x02BF, 0x0298, 0x0272, 0x024F, 0x022E, 0x020F, 0x01F1, 0x01D5, 0x01BB,
	0x01A2, 0x018B, 0x0174, 0x0160, 0x014C, 0x0139, 0x0128, 0x0117, 0x0107, 0x00F9, 0x00EB, 0x00DD,
	0x00D1, 0x00C5, 0x00BA, 0x00B0, 0x00A6, 0x009D, 0x0094, 0x008C, 0x0084, 0x007C, 0x0075, 0x006F,
	0x0069, 0x0063, 0x005D, 0x0058, 0x0053, 0x004E, 0x004A, 0x0046, 0x0042, 0x003E, 0x003B, 0x0037,
	0x0034, 0x0031, 0x002F, 0x002C, 0x0029, 0x0027, 0x0025, 0x0023, 0x0021, 0x001F, 0x001D, 0x001C,
	0x001A, 0x0019, 0x0017, 0x0016, 0x0015, 0x0014, 0x0012, 0x0011, 0x0011, 0x0010, 0x000F, 0x000E,
}

// --- FTC data types ---

type ftcSampleLine struct {
	noise              uint8
	accumulateNoise    bool
	noiseMask          bool
	tone               int
	accumulateTone     bool
	toneMask           bool
	level              int
	volSlide           int // 0, +1, -1
	accumulateEnvelope bool
	enableEnvelope     bool
	envelopeAddon      int8
}

type ftcSample struct {
	lines     []ftcSampleLine
	size      int
	loop      int
	loopLimit int
}

type ftcOrnamentLine struct {
	noiseAddon     int
	keepNoiseAddon bool
	noteAddon      int
	keepNoteAddon  bool
}

type ftcOrnament struct {
	lines     []ftcOrnamentLine
	size      int
	loop      int
	loopLimit int
}

type ftcAccumulator struct {
	value int
}

func (a *ftcAccumulator) update(delta int, accumulate bool) int {
	res := a.value + delta
	if accumulate {
		a.value = res
	}
	return res
}

func (a *ftcAccumulator) reset() { a.value = 0 }

type ftcToneSlider struct {
	sliding   int
	glissade  int
	direction int
}

func (ts *ftcToneSlider) update() int {
	ts.sliding += ts.glissade
	if (ts.direction > 0 && ts.sliding >= 0) || (ts.direction < 0 && ts.sliding < 0) {
		ts.sliding = 0
		ts.glissade = 0
	}
	return ts.sliding
}

func (ts *ftcToneSlider) reset() {
	ts.sliding = 0
	ts.glissade = 0
	ts.direction = 0
}

type ftcChannelState struct {
	note             int
	envelope         int
	envelopeEnabled  bool
	volume           int // 0-15
	volumeSlide      int
	noise            int
	sampleIdx        int
	posInSample      int
	ornamentIdx      int
	posInOrnament    int
	noteAccum        ftcAccumulator
	toneAccum        ftcAccumulator
	noiseAccum       ftcAccumulator
	sampleNoiseAccum ftcAccumulator
	envelopeAccum    ftcAccumulator
	toneAddon        int
	toneSlide        ftcToneSlider
	sampleDisabled   bool // sample iterator at end (rest state)
}

// renderFTCNative parses an FTC module and renders it to AY register frames.
func renderFTCNative(data []byte) ([][]uint8, uint32, error) {
	if len(data) < 212 {
		return nil, 0, fmt.Errorf("ftc: data too short (%d bytes)", len(data))
	}

	// Parse header (RawHeader from zxtune = 212 bytes)
	// RawId: 69 bytes (id[8] + title[42] + noteTableSign + editor[18])
	noteTableSign := data[50] // byte at offset 50 within header
	tempo := int(data[69])
	loopPos := int(data[70])

	// PatternsOffset: uint16LE at offset 75
	patternsOffset := int(data[75]) | int(data[76])<<8

	// SamplesOffsets: 32 × uint16LE at offset 82
	sampleOffsets := make([]int, 32)
	for i := range 32 {
		sampleOffsets[i] = int(data[82+i*2]) | int(data[83+i*2])<<8
	}

	// OrnamentsOffsets: 33 × uint16LE at offset 146
	ornamentOffsets := make([]int, 33)
	for i := range 33 {
		ornamentOffsets[i] = int(data[146+i*2]) | int(data[147+i*2])<<8
	}

	// Select frequency table
	var freqTable *[96]uint16
	switch noteTableSign {
	case 0x01: // SOUNDTRACKER
		freqTable = &ftcTableSoundTracker
	case 0x02: // FASTTRACKER
		freqTable = &ftcTableFastTracker
	default: // PROTRACKER2 (';' = 0x3B or other)
		freqTable = &ftcTableProTracker2
	}

	if tempo < 3 {
		tempo = 3
	}

	// Positions: start after header (offset 212), each 2 bytes (patternIndex, transposition)
	type ftcPosition struct {
		patternIndex  int
		transposition int
	}

	var positions []ftcPosition
	posOff := 212
	for posOff+1 < len(data) {
		patIdx := data[posOff]
		if patIdx == 0xFF {
			break
		}
		trans := int(int8(data[posOff+1]))
		positions = append(positions, ftcPosition{patternIndex: int(patIdx), transposition: trans})
		posOff += 2
	}

	if len(positions) == 0 {
		return nil, 0, fmt.Errorf("ftc: no positions")
	}
	if loopPos >= len(positions) {
		loopPos = 0
	}

	// Parse samples
	samples := make([]ftcSample, 32)
	for i := range 32 {
		samples[i] = ftcParseSample(data, sampleOffsets[i])
	}

	// Parse ornaments
	ornaments := make([]ftcOrnament, 33)
	for i := range 33 {
		ornaments[i] = ftcParseOrnament(data, ornamentOffsets[i])
	}

	// Render
	channels := [3]ftcChannelState{}
	for ch := range 3 {
		channels[ch].volume = 15
		channels[ch].sampleDisabled = true
	}

	speed := tempo
	var frames [][]uint8
	const maxFrames = 60 * 60 * 50
	loopFrame := uint32(0)

	transposition := 0

	for posIdx, pos := range positions {
		if posIdx == loopPos {
			loopFrame = uint32(len(frames))
		}

		transposition = pos.transposition

		// Pattern offsets table at patternsOffset
		patOff := patternsOffset + pos.patternIndex*6
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
			if cps[0].offset >= len(data) || data[cps[0].offset] == 0xFF {
				break
			}

			for ch := range 3 {
				if cps[ch].counter > 0 {
					cps[ch].counter--
					continue
				}

				dst := &channels[ch]
				noteSet := false

				for cps[ch].offset < len(data) {
					cmd := data[cps[ch].offset]
					cps[ch].offset++

					if cmd <= 0x1F {
						// Sample
						dst.sampleIdx = int(cmd)
					} else if cmd <= 0x2F {
						// Volume
						dst.volume = int(cmd - 0x20)
					} else if cmd == 0x30 {
						// Rest
						dst.sampleDisabled = true
						dst.sampleNoiseAccum.reset()
						dst.volumeSlide = 0
						dst.noiseAccum.reset()
						dst.noteAccum.reset()
						dst.posInOrnament = 0 // reset ornament
						dst.toneAccum.reset()
						dst.envelopeAccum.reset()
						dst.toneSlide.reset()
						break
					} else if cmd <= 0x3E {
						// Envelope: type = cmd - 0x30
						envType := cmd - 0x30
						if cps[ch].offset+1 < len(data) {
							envTone := int(data[cps[ch].offset]) | int(data[cps[ch].offset+1])<<8
							cps[ch].offset += 2
							dst.envelope = envTone
							_ = envType // will be written to frame
						}
						dst.envelopeEnabled = true
					} else if cmd == 0x3F {
						// No envelope
						dst.envelopeEnabled = false
					} else if cmd <= 0x5F {
						// Period + break: period = cmd - 0x40
						cps[ch].period = int(cmd - 0x40)
						break
					} else if cmd <= 0xCB {
						// Note: note = cmd - 0x60
						note := int(cmd-0x60) + transposition
						dst.note = note

						// Reset state for new note
						dst.sampleDisabled = false
						dst.posInSample = 0
						dst.sampleNoiseAccum.reset()
						dst.volumeSlide = 0
						dst.noiseAccum.reset()
						dst.noteAccum.reset()
						dst.posInOrnament = 0
						dst.toneAccum.reset()
						dst.envelopeAccum.reset()
						dst.toneSlide.reset()
						noteSet = true
					} else if cmd <= 0xEC {
						// Ornament: ornament = cmd - 0xCC
						dst.ornamentIdx = int(cmd - 0xCC)
						dst.posInOrnament = 0
						dst.noiseAccum.reset()
						dst.noteAccum.reset()
					} else if cmd == 0xED {
						// Slide: u16LE step
						if cps[ch].offset+1 < len(data) {
							step := int(data[cps[ch].offset]) | int(data[cps[ch].offset+1])<<8
							cps[ch].offset += 2
							dst.toneSlide.glissade = int(int16(uint16(step)))
						}
					} else if cmd == 0xEE {
						// Note slide: u8 step
						if cps[ch].offset < len(data) {
							step := int(data[cps[ch].offset])
							cps[ch].offset++
							if noteSet {
								// Compute slide direction
								targetNote := dst.note
								slideDiff := ftcGetSlidingDifference(freqTable, targetNote, dst.note)
								gliss := step
								dir := 1
								if slideDiff >= 0 {
									gliss = -step
									dir = -1
								}
								dst.toneSlide.sliding = slideDiff
								dst.toneSlide.glissade = gliss
								dst.toneSlide.direction = dir
							}
						}
					} else if cmd == 0xEF {
						// Noise
						if cps[ch].offset < len(data) {
							dst.noise = int(data[cps[ch].offset])
							cps[ch].offset++
						}
					} else if cmd >= 0xF0 {
						// Tempo
						if cps[ch].offset < len(data) {
							speed = int(data[cps[ch].offset])
							cps[ch].offset++
							if speed < 1 {
								speed = 1
							}
						}
					}
				}
				cps[ch].counter = cps[ch].period
			}

			// Generate 'speed' frames for this row
			for tick := range speed {
				_ = tick
				frame := ftcSynthesizeFrame(&channels, samples, ornaments, freqTable)
				frames = append(frames, frame)
				if len(frames) >= maxFrames {
					break
				}
			}
		}
	}

	if len(frames) == 0 {
		return nil, 0, fmt.Errorf("ftc: no frames generated")
	}
	return frames, loopFrame, nil
}

func ftcParseSample(data []byte, off int) ftcSample {
	stub := ftcSample{
		lines:     []ftcSampleLine{{}},
		size:      1,
		loop:      0,
		loopLimit: 0,
	}
	if off == 0 || off+3 > len(data) {
		return stub
	}

	// RawObject header: Size(1), Loop(1), LoopLimit(1)
	objSize := int(data[off]) + 1
	objLoop := int(data[off+1])
	objLoopLimit := int(data[off+2]) + 1

	lines := make([]ftcSampleLine, 0, objSize)
	for i := range objSize {
		lineOff := off + 3 + i*5
		if lineOff+5 > len(data) {
			break
		}
		b0 := data[lineOff]   // KMxnnnnn
		b1 := data[lineOff+1] // ToneLo
		b2 := data[lineOff+2] // KMxxtttt
		b3 := data[lineOff+3] // KMvvaaaa
		b4 := data[lineOff+4] // EnvelopeAddon (int8)

		var volSlide int
		if b3&0x20 != 0 {
			if b3&0x10 != 0 {
				volSlide = -1
			} else {
				volSlide = 1
			}
		}

		lines = append(lines, ftcSampleLine{
			noise:              b0 & 0x1F,
			accumulateNoise:    b0&0x80 != 0,
			noiseMask:          b0&0x40 != 0,
			tone:               int(b2&0x0F)*256 + int(b1),
			accumulateTone:     b2&0x80 != 0,
			toneMask:           b2&0x40 != 0,
			level:              int(b3 & 0x0F),
			volSlide:           volSlide,
			accumulateEnvelope: b3&0x80 != 0,
			enableEnvelope:     b3&0x40 != 0,
			envelopeAddon:      int8(b4),
		})
	}

	if len(lines) == 0 {
		return stub
	}
	if objLoop >= len(lines) {
		objLoop = 0
	}
	if objLoopLimit > len(lines) {
		objLoopLimit = len(lines)
	}

	return ftcSample{
		lines:     lines,
		size:      len(lines),
		loop:      objLoop,
		loopLimit: objLoopLimit,
	}
}

func ftcParseOrnament(data []byte, off int) ftcOrnament {
	stub := ftcOrnament{
		lines:     []ftcOrnamentLine{{}},
		size:      1,
		loop:      0,
		loopLimit: 0,
	}
	if off == 0 || off+3 > len(data) {
		return stub
	}

	objSize := int(data[off]) + 1
	objLoop := int(data[off+1])
	objLoopLimit := int(data[off+2]) + 1

	lines := make([]ftcOrnamentLine, 0, objSize)
	for i := range objSize {
		lineOff := off + 3 + i*2
		if lineOff+2 > len(data) {
			break
		}
		b0 := data[lineOff]   // nNxooooo
		b1 := data[lineOff+1] // NoteAddon (int8)

		lines = append(lines, ftcOrnamentLine{
			noiseAddon:     int(b0 & 0x1F),
			keepNoiseAddon: b0&0x80 != 0,
			noteAddon:      int(int8(b1)),
			keepNoteAddon:  b0&0x40 != 0,
		})
	}

	if len(lines) == 0 {
		return stub
	}
	if objLoop >= len(lines) {
		objLoop = 0
	}
	if objLoopLimit > len(lines) {
		objLoopLimit = len(lines)
	}

	return ftcOrnament{
		lines:     lines,
		size:      len(lines),
		loop:      objLoop,
		loopLimit: objLoopLimit,
	}
}

func ftcSynthesizeFrame(channels *[3]ftcChannelState, samples []ftcSample, ornaments []ftcOrnament, freqTable *[96]uint16) []uint8 {
	frame := make([]uint8, 14)
	frame[13] = 0xFF // sentinel: don't write R13 unless envelope command
	mixer := uint8(0)
	noise := uint8(0)

	for ch := range 3 {
		dst := &channels[ch]

		// Get ornament line
		ornIdx := dst.ornamentIdx
		if ornIdx >= len(ornaments) {
			ornIdx = 0
		}
		orn := &ornaments[ornIdx]

		opos := dst.posInOrnament
		if opos >= len(orn.lines) {
			opos = orn.loop
			if opos >= len(orn.lines) {
				opos = 0
			}
		}
		oline := orn.lines[opos]

		noteAddon := dst.noteAccum.update(oline.noteAddon, oline.keepNoteAddon)
		noiseAddon := dst.noiseAccum.update(oline.noiseAddon, oline.keepNoiseAddon)

		// Advance ornament
		dst.posInOrnament++
		if dst.posInOrnament >= orn.loopLimit {
			dst.posInOrnament = orn.loop
		}

		// Get sample line
		samIdx := dst.sampleIdx
		if samIdx >= len(samples) {
			samIdx = 0
		}
		sam := &samples[samIdx]

		spos := dst.posInSample
		if spos >= len(sam.lines) {
			spos = sam.loop
			if spos >= len(sam.lines) {
				spos = 0
			}
		}

		if dst.sampleDisabled || spos >= len(sam.lines) {
			// Channel disabled or sample exhausted
			frame[8+ch] = 0
			mixer |= (1 << ch) | (1 << (ch + 3))
			continue
		}

		sline := sam.lines[spos]

		// Noise
		sampleNoiseAddon := dst.sampleNoiseAccum.update(int(sline.noise), sline.accumulateNoise)
		if sline.noiseMask {
			mixer |= 1 << (ch + 3)
		} else {
			noise = uint8((dst.noise + noiseAddon + sampleNoiseAddon) & 0x1F)
		}

		// Tone
		dst.toneAddon = dst.toneAccum.update(sline.tone, sline.accumulateTone)
		if sline.toneMask {
			mixer |= 1 << ch
		}

		// Level
		dst.volumeSlide += sline.volSlide
		level := min(max(sline.level+dst.volumeSlide, 0), 15)
		vol := ftcGetVolume(dst.volume, level)
		frame[8+ch] = uint8(vol)

		// Envelope
		envAddon := dst.envelopeAccum.update(int(sline.envelopeAddon), sline.accumulateEnvelope)
		if sline.enableEnvelope && dst.envelopeEnabled {
			frame[8+ch] |= 0x10
			envTone := max(dst.envelope-envAddon, 0)
			frame[11] = uint8(envTone & 0xFF)
			frame[12] = uint8((envTone >> 8) & 0xFF)
		}

		// Tone period
		halfTone := min(max(dst.note+noteAddon, 0), 95)
		tonePeriod := min(max(int(freqTable[halfTone])+dst.toneAddon+dst.toneSlide.update(), 0), 0xFFF)
		frame[ch*2] = uint8(tonePeriod & 0xFF)
		frame[ch*2+1] = uint8((tonePeriod >> 8) & 0x0F)

		// Advance sample
		dst.posInSample++
		if dst.posInSample >= sam.loopLimit {
			dst.posInSample = sam.loop
		}
	}

	frame[6] = noise
	frame[7] = mixer

	return frame
}

func ftcGetVolume(volume, level int) int {
	extra := 0
	if volume > 7 {
		extra = 1
	}
	return ((volume*17+extra)*level + 128) >> 8
}

func ftcGetSlidingDifference(freqTable *[96]uint16, note1, note2 int) int {
	if note1 < 0 {
		note1 = 0
	}
	if note1 > 95 {
		note1 = 95
	}
	if note2 < 0 {
		note2 = 0
	}
	if note2 > 95 {
		note2 = 95
	}
	return int(freqTable[note1]) - int(freqTable[note2])
}
