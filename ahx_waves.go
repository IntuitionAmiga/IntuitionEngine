// ahx_waves.go - AHX waveform generation
// Reference: AHX-Sources/AHX.cpp AHXWaves class

package main

// Period2Freq converts an AHX period to frequency
// The Paula clock is the NTSC colorburst frequency * 2
const AHXPeriod2FreqClock = 3579545.25

func AHXPeriod2Freq(period int) float64 {
	if period <= 0 {
		return 0
	}
	return AHXPeriod2FreqClock / float64(period)
}

// AHXVibratoTable is the 64-entry vibrato table (sine-like)
var AHXVibratoTable = [64]int{
	0, 24, 49, 74, 97, 120, 141, 161, 180, 197, 212, 224, 235, 244, 250, 253, 255,
	253, 250, 244, 235, 224, 212, 197, 180, 161, 141, 120, 97, 74, 49, 24,
	0, -24, -49, -74, -97, -120, -141, -161, -180, -197, -212, -224, -235, -244, -250, -253, -255,
	-253, -250, -244, -235, -224, -212, -197, -180, -161, -141, -120, -97, -74, -49, -24,
}

// AHXPeriodTable is the note-to-period lookup table (61 entries, notes 0-60)
var AHXPeriodTable = [61]int{
	0x0000, 0x0D60, 0x0CA0, 0x0BE8, 0x0B40, 0x0A98, 0x0A00, 0x0970,
	0x08E8, 0x0868, 0x07F0, 0x0780, 0x0714, 0x06B0, 0x0650, 0x05F4,
	0x05A0, 0x054C, 0x0500, 0x04B8, 0x0474, 0x0434, 0x03F8, 0x03C0,
	0x038A, 0x0358, 0x0328, 0x02FA, 0x02D0, 0x02A6, 0x0280, 0x025C,
	0x023A, 0x021A, 0x01FC, 0x01E0, 0x01C5, 0x01AC, 0x0194, 0x017D,
	0x0168, 0x0153, 0x0140, 0x012E, 0x011D, 0x010D, 0x00FE, 0x00F0,
	0x00E2, 0x00D6, 0x00CA, 0x00BE, 0x00B4, 0x00AA, 0x00A0, 0x0097,
	0x008F, 0x0087, 0x007F, 0x0078, 0x0071,
}

// AHXWaves holds all pre-generated waveform tables
type AHXWaves struct {
	// Base waveforms at different lengths
	Triangle04 [0x04]int8
	Triangle08 [0x08]int8
	Triangle10 [0x10]int8
	Triangle20 [0x20]int8
	Triangle40 [0x40]int8
	Triangle80 [0x80]int8

	Sawtooth04 [0x04]int8
	Sawtooth08 [0x08]int8
	Sawtooth10 [0x10]int8
	Sawtooth20 [0x20]int8
	Sawtooth40 [0x40]int8
	Sawtooth80 [0x80]int8

	Squares       [0x80 * 0x20]int8 // 32 duty cycles, 128 bytes each
	WhiteNoiseBig [0x280 * 3]int8   // Large noise buffer

	// Filtered versions (31 filter settings)
	LowPasses  []int8
	HighPasses []int8
}

// NewAHXWaves creates and initializes all waveform tables
func NewAHXWaves() *AHXWaves {
	w := &AHXWaves{}
	w.generate()
	return w
}

// generate creates all waveform tables
func (w *AHXWaves) generate() {
	// Generate base waveforms
	w.generateSawtooth(w.Sawtooth04[:], 0x04)
	w.generateSawtooth(w.Sawtooth08[:], 0x08)
	w.generateSawtooth(w.Sawtooth10[:], 0x10)
	w.generateSawtooth(w.Sawtooth20[:], 0x20)
	w.generateSawtooth(w.Sawtooth40[:], 0x40)
	w.generateSawtooth(w.Sawtooth80[:], 0x80)

	w.generateTriangle(w.Triangle04[:], 0x04)
	w.generateTriangle(w.Triangle08[:], 0x08)
	w.generateTriangle(w.Triangle10[:], 0x10)
	w.generateTriangle(w.Triangle20[:], 0x20)
	w.generateTriangle(w.Triangle40[:], 0x40)
	w.generateTriangle(w.Triangle80[:], 0x80)

	w.generateSquare(w.Squares[:])
	w.generateWhiteNoise(w.WhiteNoiseBig[:], 0x280*3)

	// Generate filtered waveforms
	w.generateFilterWaveforms()
}

// generateTriangle creates a triangle waveform
// From C++ reference: starts at 0, rises to 127, falls to 0, then to -128
func (w *AHXWaves) generateTriangle(buffer []int8, length int) {
	d2 := length
	d5 := d2 >> 2    // quarter length
	d1 := 128 / d5   // step size
	d4 := -(d2 >> 1) // negative half length
	pos := 0
	eax := 0

	// First quarter: 0 up to near 127
	for ecx := 0; ecx < d5; ecx++ {
		buffer[pos] = int8(eax)
		pos++
		eax += d1
	}

	// Peak at 127
	buffer[pos] = 0x7f
	pos++

	// Second quarter: down from 127 to 0
	if d5 != 1 {
		eax = 128
		for ecx := 0; ecx < d5-1; ecx++ {
			eax -= d1
			buffer[pos] = int8(eax)
			pos++
		}
	}

	// Second half: mirror and negate
	esi := pos + d4
	for ecx := 0; ecx < d5*2; ecx++ {
		val := buffer[esi]
		esi++
		if val == 0x7f {
			buffer[pos] = -128
		} else {
			buffer[pos] = -val
		}
		pos++
	}
}

// generateSawtooth creates a sawtooth waveform
// From C++ reference: linear ramp from -128 to ~127
func (w *AHXWaves) generateSawtooth(buffer []int8, length int) {
	step := 256 / (length - 1)
	val := -128
	for i := 0; i < length; i++ {
		buffer[i] = int8(val)
		val += step
	}
}

// generateSquare creates square waveforms with 32 different duty cycles
// From C++ reference: width 1 to 32, stored as 128-byte patterns
func (w *AHXWaves) generateSquare(buffer []int8) {
	pos := 0
	for width := 1; width <= 0x20; width++ {
		// Low portion: (0x40 - width) * 2 samples at -128
		lowCount := (0x40 - width) * 2
		for i := 0; i < lowCount; i++ {
			buffer[pos] = -128
			pos++
		}
		// High portion: width * 2 samples at 127
		highCount := width * 2
		for i := 0; i < highCount; i++ {
			buffer[pos] = 127
			pos++
		}
	}
}

// generateWhiteNoise creates white noise using a PRNG
// From C++ reference: uses specific PRNG algorithm
func (w *AHXWaves) generateWhiteNoise(buffer []int8, length int) {
	// Initial seed from C++ reference
	eax := uint32(0x41595321) // "AYS!"

	for i := 0; i < length; i++ {
		if eax&0x100 != 0 {
			// Check if ax portion is negative or positive
			ax := int16(eax & 0xFFFF)
			if ax < 0 {
				buffer[i] = -128
			} else {
				buffer[i] = 127
			}
		} else {
			buffer[i] = int8(eax & 0xFF)
		}

		// PRNG transformation from C++ assembly
		eax = (eax >> 5) | (eax << 27) // ror 5
		eax ^= 0x9A                    // xor al, 10011010b
		bx := uint16(eax & 0xFFFF)
		eax = (eax << 2) | (eax >> 30) // rol 2
		bx += uint16(eax & 0xFFFF)
		eax = (eax & 0xFFFF0000) | uint32(uint16(eax)^bx)
		eax = (eax >> 3) | (eax << 29) // ror 3
	}
}

// generateFilterWaveforms creates low-pass and high-pass filtered versions
// of all base waveforms at 31 different filter cutoff frequencies
func (w *AHXWaves) generateFilterWaveforms() {
	// Length table for each waveform type
	lengthTable := []int{
		3, 7, 0xf, 0x1f, 0x3f, 0x7f, // Triangle lengths - 1
		3, 7, 0xf, 0x1f, 0x3f, 0x7f, // Sawtooth lengths - 1
		0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, // Square patterns (32 total)
		0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f,
		0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f,
		0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f,
		(0x280 * 3) - 1, // Noise
	}

	// Calculate total size
	totalSize := 0
	for _, l := range lengthTable {
		totalSize += l + 1
	}
	totalSize *= 31 // 31 filter settings

	w.LowPasses = make([]int8, totalSize)
	w.HighPasses = make([]int8, totalSize)

	// Build source buffer: all base waveforms concatenated
	sourceSize := 0
	for _, l := range lengthTable {
		sourceSize += l + 1
	}
	source := make([]int8, sourceSize)

	pos := 0
	// Copy triangles
	copy(source[pos:], w.Triangle04[:])
	pos += 0x04
	copy(source[pos:], w.Triangle08[:])
	pos += 0x08
	copy(source[pos:], w.Triangle10[:])
	pos += 0x10
	copy(source[pos:], w.Triangle20[:])
	pos += 0x20
	copy(source[pos:], w.Triangle40[:])
	pos += 0x40
	copy(source[pos:], w.Triangle80[:])
	pos += 0x80
	// Copy sawtoothes
	copy(source[pos:], w.Sawtooth04[:])
	pos += 0x04
	copy(source[pos:], w.Sawtooth08[:])
	pos += 0x08
	copy(source[pos:], w.Sawtooth10[:])
	pos += 0x10
	copy(source[pos:], w.Sawtooth20[:])
	pos += 0x20
	copy(source[pos:], w.Sawtooth40[:])
	pos += 0x40
	copy(source[pos:], w.Sawtooth80[:])
	pos += 0x80
	// Copy squares
	copy(source[pos:], w.Squares[:])
	pos += len(w.Squares)
	// Copy noise
	copy(source[pos:], w.WhiteNoiseBig[:])

	// Generate filtered versions for 31 frequency settings
	lowPos := 0
	highPos := 0
	for temp, freq := 0, float32(8); temp < 31; temp, freq = temp+1, freq+3 {
		srcPos := 0
		for waveIdx := 0; waveIdx < len(lengthTable); waveIdx++ {
			waveLen := lengthTable[waveIdx]
			fre := freq * 1.25 / 100.0

			// Two-pass filter for stability
			var high, mid, low float32

			// First pass (warmup)
			for i := 0; i <= waveLen; i++ {
				high = float32(source[srcPos+i]) - mid - low
				high = clip(high)
				mid += high * fre
				mid = clip(mid)
				low += mid * fre
				low = clip(low)
			}

			// Second pass (output)
			for i := 0; i <= waveLen; i++ {
				high = float32(source[srcPos+i]) - mid - low
				high = clip(high)
				mid += high * fre
				mid = clip(mid)
				low += mid * fre
				low = clip(low)

				w.LowPasses[lowPos] = int8(low)
				lowPos++
				w.HighPasses[highPos] = int8(high)
				highPos++
			}

			srcPos += waveLen + 1
		}
	}
}

// clip clamps a value to the int8 range
func clip(x float32) float32 {
	if x > 127 {
		return 127
	}
	if x < -128 {
		return -128
	}
	return x
}
