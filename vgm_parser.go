// vgm_parser.go - VGM/VGZ parser for PSG register writes.
//
// Supported chips (events extracted as PSGEvents):
//   - AY-3-8910 / YM2149 (cmd 0xA0) — direct register mapping
//   - SN76489 / SN76496 (cmd 0x50) — converted to AY-equivalent register writes
//
// Ignored chips (commands skipped gracefully):
//   - SN76489 GG stereo (cmd 0x4F)
//   - YM2413 (cmd 0x51), YM2612 (cmd 0x52-0x53), YM2151 (cmd 0x54)
//   - YM2203 (cmd 0x55), YM2608 (cmd 0x56-0x57), YM2610 (cmd 0x58-0x59)
//   - YM3812 (cmd 0x5A), YM3526 (cmd 0x5B), Y8950 (cmd 0x5C)
//   - YMF262 (cmd 0x5D-0x5E), YMZ280B (cmd 0x5F)
//   - Sega PCM (cmd 0xC0+), DAC stream (cmd 0x90-0x95)
//   - PCM RAM writes (cmd 0x68), data blocks (cmd 0x67)
//   - All seek/meta commands (cmd 0xE0-0xFF)
//
// Rejected: None — unknown commands are skipped with 1-byte advancement.

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
	SNClockHz    uint32 // SN76489 clock from header (0 if not present)
	TotalSamples uint64
	LoopSamples  uint64
	LoopSample   uint64
}

// sn76489State tracks the latch register state for SN76489 decoding.
// The SN76489 uses a single write port with latch/data byte protocol:
//   - Latch byte (bit 7=1): sets channel, type (tone/attenuation), and low data bits
//   - Data byte (bit 7=0): writes additional data bits to the latched register
type sn76489State struct {
	latchedCh   uint8     // Currently latched channel (0-3)
	latchedType uint8     // 0=tone, 1=attenuation
	toneRegs    [3]uint16 // 10-bit tone dividers for channels 0-2
	attenRegs   [4]uint8  // 4-bit attenuation for channels 0-3 (0=max, 15=off)
	noiseReg    uint8     // Noise control register (3 bits)
	snClockHz   uint32    // SN76489 chip clock frequency
	ayClockHz   uint32    // Target AY clock for divider conversion
}

// sn76489Decode processes a single SN76489 write byte and returns any
// resulting PSGEvents (AY register equivalents). Returns nil if no events needed.
func (s *sn76489State) decode(val byte, samplePos uint64) []PSGEvent {
	if val&0x80 != 0 {
		// Latch byte: bit 7=1
		// Bits 6-5: channel (0-3)
		// Bit 4: 0=tone, 1=attenuation
		// Bits 3-0: data (low 4 bits)
		s.latchedCh = (val >> 5) & 0x03
		s.latchedType = (val >> 4) & 0x01
		lowBits := val & 0x0F

		if s.latchedType == 1 {
			// Attenuation latch: full 4-bit value
			s.attenRegs[s.latchedCh] = lowBits
			return s.emitAttenuation(s.latchedCh, samplePos)
		}

		if s.latchedCh == 3 {
			// Noise register: 3-bit value in low bits
			s.noiseReg = lowBits & 0x07
			return s.emitNoise(samplePos)
		}

		// Tone latch: low 4 bits of 10-bit divider
		s.toneRegs[s.latchedCh] = (s.toneRegs[s.latchedCh] & 0x3F0) | uint16(lowBits)
		events := s.emitTone(s.latchedCh, samplePos)
		if s.latchedCh == 2 && s.noiseReg&0x03 == 3 {
			events = append(events, PSGEvent{Sample: samplePos, Reg: 6, Value: s.noiseFromTone2()})
		}
		return events
	}

	// Data byte: bit 7=0, bits 5-0 are data
	dataBits := val & 0x3F

	if s.latchedType == 1 {
		// Attenuation data byte: low 4 bits
		s.attenRegs[s.latchedCh] = dataBits & 0x0F
		return s.emitAttenuation(s.latchedCh, samplePos)
	}

	if s.latchedCh == 3 {
		// Real SN76489 ignores data bytes for the noise register
		return nil
	}

	// Tone data byte: high 6 bits of 10-bit divider
	s.toneRegs[s.latchedCh] = (s.toneRegs[s.latchedCh] & 0x0F) | (uint16(dataBits) << 4)
	events := s.emitTone(s.latchedCh, samplePos)
	if s.latchedCh == 2 && s.noiseReg&0x03 == 3 {
		events = append(events, PSGEvent{Sample: samplePos, Reg: 6, Value: s.noiseFromTone2()})
	}
	return events
}

// emitTone converts an SN76489 tone divider to AY frequency register writes.
// SN76489: Freq = snClock / (32 * N), AY: Freq = ayClock / (16 * N_ay)
// For same output: N_ay = N_sn * ayClock / (snClock * 2)
func (s *sn76489State) emitTone(ch uint8, samplePos uint64) []PSGEvent {
	if ch > 2 {
		return nil
	}
	divider := s.toneRegs[ch]
	if divider == 0 {
		divider = 1
	}

	// Convert SN76489 divider to AY divider
	var ayDiv uint16
	if s.snClockHz > 0 && s.ayClockHz > 0 {
		// N_ay = N_sn * ayClock / (snClock * 2)
		ayDiv = uint16(uint32(divider) * s.ayClockHz / (s.snClockHz * 2))
	} else {
		// Fallback: approximate 1:2 ratio (SN76489 divides by 32, AY by 16)
		ayDiv = divider / 2
	}
	if ayDiv == 0 {
		ayDiv = 1
	}
	if ayDiv > 0xFFF {
		ayDiv = 0xFFF
	}

	// AY registers: channel A = regs 0/1, B = regs 2/3, C = regs 4/5
	regLo := ch * 2
	regHi := ch*2 + 1
	return []PSGEvent{
		{Sample: samplePos, Reg: regLo, Value: uint8(ayDiv & 0xFF)},
		{Sample: samplePos, Reg: regHi, Value: uint8((ayDiv >> 8) & 0x0F)},
	}
}

// emitAttenuation converts SN76489 attenuation to AY volume.
// SN76489: 0=max volume, 15=silence. AY: 0=silence, 15=max volume.
func (s *sn76489State) emitAttenuation(ch uint8, samplePos uint64) []PSGEvent {
	if ch > 3 {
		return nil
	}
	if ch == 3 {
		// Noise channel attenuation: no direct AY equivalent for noise volume.
		// Map to mixer enable/disable: if attenuation=15 (off), disable noise.
		return s.emitMixer(samplePos)
	}
	// AY volume registers: A=8, B=9, C=10
	ayVol := 15 - s.attenRegs[ch]
	return []PSGEvent{
		{Sample: samplePos, Reg: 8 + ch, Value: ayVol},
	}
}

// noiseFromTone2 computes the AY noise period from channel 2's tone divider.
// SN76489 noise rate 3 uses channel 2's tone output as the noise clock.
// SN76489: Freq = snClock / (32 * N_sn), AY noise: Freq = ayClock / (16 * N_noise)
// For equal frequency: N_noise = N_sn * ayClock / (snClock * 2)
func (s *sn76489State) noiseFromTone2() uint8 {
	divider := s.toneRegs[2]
	if divider == 0 {
		return 1
	}
	if s.snClockHz > 0 && s.ayClockHz > 0 {
		np := uint32(divider) * s.ayClockHz / (s.snClockHz * 2)
		if np == 0 {
			return 1
		}
		if np > 31 {
			return 31
		}
		return uint8(np)
	}
	// Fallback: approximate 1:2 ratio
	np := divider / 2
	if np == 0 {
		return 1
	}
	if np > 31 {
		return 31
	}
	return uint8(np)
}

// emitNoise converts SN76489 noise control to AY noise register + mixer.
// SN76489 noise: bits 1-0 = shift rate, bit 2 = white(1)/periodic(0)
// AY noise period register = 6 (5-bit), mixer register = 7
func (s *sn76489State) emitNoise(samplePos uint64) []PSGEvent {
	// SN76489 noise shift rates:
	//   0 = clock/512, 1 = clock/1024, 2 = clock/2048, 3 = channel 2 output
	// Convert to AY noise period (5-bit, 0-31):
	//   AY noise freq = ayClock / (16 * N_noise)
	var noisePeriod uint8
	shiftRate := s.noiseReg & 0x03
	switch shiftRate {
	case 0:
		noisePeriod = 4 // Fastest noise
	case 1:
		noisePeriod = 8
	case 2:
		noisePeriod = 16
	case 3:
		// Track channel 2's tone frequency
		noisePeriod = s.noiseFromTone2()
	}

	events := []PSGEvent{
		{Sample: samplePos, Reg: 6, Value: noisePeriod},
	}
	events = append(events, s.emitMixer(samplePos)...)
	return events
}

// emitMixer generates the AY mixer register (reg 7) based on current SN76489 state.
// The mixer controls which channels have tone and/or noise enabled.
func (s *sn76489State) emitMixer(samplePos uint64) []PSGEvent {
	// AY register 7 (mixer): bits 0-2 = tone disable (A/B/C), bits 3-5 = noise disable (A/B/C)
	// Active low: 0 = enabled, 1 = disabled
	mixer := uint8(0x38) // Start with all noise disabled

	// Enable tone for channels with non-zero divider and non-silent attenuation
	for ch := range 3 {
		if s.attenRegs[ch] >= 15 {
			mixer |= 1 << ch // Disable tone for silent channels
		}
	}

	// Enable noise on channel C if noise attenuation < 15
	if s.attenRegs[3] < 15 {
		mixer &^= 0x20 // Enable noise on channel C (bit 5 = 0)
	}

	return []PSGEvent{
		{Sample: samplePos, Reg: 7, Value: mixer},
	}
}

func ParseVGMFile(path string) (*VGMFile, error) {
	data, err := readVGMData(path)
	if err != nil {
		return nil, err
	}
	return ParseVGMData(data)
}

func ParseVGMData(data []byte) (*VGMFile, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("vgm too short")
	}
	if data[0] == 0x1F && data[1] == 0x8B {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		data, err = io.ReadAll(gz)
		if err != nil {
			return nil, err
		}
	}
	if len(data) < 0x40 {
		return nil, fmt.Errorf("vgm too short")
	}
	if !bytes.Equal(data[0:4], []byte("Vgm ")) {
		return nil, fmt.Errorf("invalid vgm header")
	}

	version := binary.LittleEndian.Uint32(data[0x08:0x0C])
	_ = version

	totalSamples := uint64(binary.LittleEndian.Uint32(data[0x18:0x1C]))
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

	// Read chip clocks from VGM header
	snClockHz := uint32(0)
	if len(data) >= 0x10 {
		snClockHz = binary.LittleEndian.Uint32(data[0x0C:0x10])
	}
	clockHz := uint32(0) // AY-3-8910 / YM2149 clock
	if len(data) >= 0x78 {
		clockHz = binary.LittleEndian.Uint32(data[0x74:0x78])
	}

	// Initialize SN76489 state machine for latch/data decoding
	snState := sn76489State{
		snClockHz: snClockHz,
		ayClockHz: clockHz,
		attenRegs: [4]uint8{15, 15, 15, 15}, // All channels start silent
	}
	// Default AY clock for SN76489-only VGMs
	if snState.ayClockHz == 0 {
		snState.ayClockHz = PSG_CLOCK_MSX
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
			snEvents := snState.decode(data[i+1], samplePos)
			events = append(events, snEvents...)
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
		case cmd == 0x68:
			// PCM RAM write: 12 bytes total
			if i+12 > len(data) {
				return nil, fmt.Errorf("vgm truncated PCM RAM write at offset %d", i)
			}
			i += 12
			continue
		case cmd >= 0x80 && cmd <= 0x8F:
			// YM2612 port 0 address 2A write + wait: 1 byte (no operand)
			samplePos += uint64(cmd & 0x0F)
			i++
			continue
		case cmd == 0x90 || cmd == 0x91 || cmd == 0x95:
			// DAC stream setup/set data/start fast: 5 bytes total
			if i+5 > len(data) {
				return nil, fmt.Errorf("vgm truncated DAC stream command at offset %d", i)
			}
			i += 5
			continue
		case cmd == 0x92:
			// DAC stream set frequency: 6 bytes total
			if i+6 > len(data) {
				return nil, fmt.Errorf("vgm truncated DAC stream frequency at offset %d", i)
			}
			i += 6
			continue
		case cmd == 0x93:
			// DAC stream start: 11 bytes total
			if i+11 > len(data) {
				return nil, fmt.Errorf("vgm truncated DAC stream start at offset %d", i)
			}
			i += 11
			continue
		case cmd == 0x94:
			// DAC stream stop: 2 bytes total
			if i+2 > len(data) {
				return nil, fmt.Errorf("vgm truncated DAC stream stop at offset %d", i)
			}
			i += 2
			continue
		case cmd >= 0x30 && cmd <= 0x3F:
			// One-operand commands (reserved/misc): 2 bytes total
			if i+2 > len(data) {
				return nil, fmt.Errorf("vgm truncated command 0x%02X at offset %d", cmd, i)
			}
			i += 2
			continue
		case cmd >= 0x41 && cmd <= 0x4E:
			// Two-operand commands (misc chip writes): 3 bytes total
			if i+3 > len(data) {
				return nil, fmt.Errorf("vgm truncated command 0x%02X at offset %d", cmd, i)
			}
			i += 3
			continue
		case cmd == 0x4F:
			// Game Gear PSG stereo: 2 bytes total
			if i+2 > len(data) {
				return nil, fmt.Errorf("vgm truncated GG PSG stereo at offset %d", i)
			}
			i += 2
			continue
		case cmd >= 0x51 && cmd <= 0x5F:
			// Two-operand chip writes (YM2413, YM2612, etc.): 3 bytes total
			if i+3 > len(data) {
				return nil, fmt.Errorf("vgm truncated command 0x%02X at offset %d", cmd, i)
			}
			i += 3
			continue
		case cmd >= 0xA1 && cmd <= 0xBF:
			// Two-operand chip writes: 3 bytes total
			if i+3 > len(data) {
				return nil, fmt.Errorf("vgm truncated command 0x%02X at offset %d", cmd, i)
			}
			i += 3
			continue
		case cmd >= 0xC0 && cmd <= 0xDF:
			// Three-operand chip writes: 4 bytes total
			if i+4 > len(data) {
				return nil, fmt.Errorf("vgm truncated command 0x%02X at offset %d", cmd, i)
			}
			i += 4
			continue
		case cmd >= 0xE0 && cmd <= 0xFF:
			// Four-operand commands (seek, etc.): 5 bytes total
			if i+5 > len(data) {
				return nil, fmt.Errorf("vgm truncated command 0x%02X at offset %d", cmd, i)
			}
			i += 5
			continue
		default:
			// Unknown command — skip 1 byte and hope for the best
			i++
			continue
		}
	}

	if len(events) > 0 {
		last := max(totalSamples, events[len(events)-1].Sample+1)
		totalSamples = last
	}
	if loopSample == 0 && loopSamples > 0 && totalSamples >= uint64(loopSamples) {
		loopSample = totalSamples - uint64(loopSamples)
	}

	// Use SN76489 clock as primary clock if no AY clock present
	reportClockHz := clockHz
	if reportClockHz == 0 {
		reportClockHz = snClockHz
	}

	return &VGMFile{
		Events:       events,
		ClockHz:      reportClockHz,
		SNClockHz:    snClockHz,
		TotalSamples: totalSamples,
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
