// ahx_waves.go - AHX base waveform generation tests.

package main

// AHXWaves holds base waveform tables retained for AHX table validation.
// Runtime AHX playback maps tracker state to native SoundChip waveforms.
type AHXWaves struct {
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
	for range d5 {
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
	for i := range length {
		buffer[i] = int8(val)
		val += step
	}
}
