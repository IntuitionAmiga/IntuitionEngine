// audio_lut.go - Lookup tables for optimized audio synthesis

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "math"

// Lookup table sizes
const (
	sinLUTSize  = 8192           // 8192 entries for sine (~0.00077 radian resolution)
	sinLUTMask  = sinLUTSize - 1 // Mask for fast modulo
	tanhLUTSize = 4096           // 4096 entries for tanh
	tanhLUTMin  = float32(-4.0)  // Tanh LUT minimum input
	tanhLUTMax  = float32(4.0)   // Tanh LUT maximum input
)

// Precomputed scale factors
const (
	sinLUTScale  = float32(sinLUTSize) / (2 * math.Pi)                // phase to index
	tanhLUTScale = float32(tanhLUTSize-1) / (tanhLUTMax - tanhLUTMin) // input to index
)

// sinLUT contains precomputed sine values for phase [0, 2π)
// Index mapping: phase * sinLUTScale
var sinLUT [sinLUTSize]float32

// tanhLUT contains precomputed tanh values for input [-4, 4]
// Values outside this range are clamped to ±1
var tanhLUT [tanhLUTSize]float32

func init() {
	// Initialize sine lookup table
	for i := 0; i < sinLUTSize; i++ {
		phase := float64(i) * 2 * math.Pi / float64(sinLUTSize)
		sinLUT[i] = float32(math.Sin(phase))
	}

	// Initialize tanh lookup table
	for i := 0; i < tanhLUTSize; i++ {
		x := float64(tanhLUTMin) + float64(i)*float64(tanhLUTMax-tanhLUTMin)/float64(tanhLUTSize-1)
		tanhLUT[i] = float32(math.Tanh(x))
	}
}

// fastSin returns sin(phase) using lookup table with linear interpolation.
// Phase should be in radians [0, 2π). Values outside this range are wrapped.
//
//go:nosplit
func fastSin(phase float32) float32 {
	// Wrap phase to [0, 2π) range using optimized approach
	// First, handle common case of small positive values
	if phase < 0 {
		phase += TWO_PI
		if phase < 0 {
			// Very negative values need floor approach
			phase = phase - TWO_PI*float32(int(phase/TWO_PI)-1)
		}
	} else if phase >= TWO_PI {
		phase = phase - TWO_PI*float32(int(phase/TWO_PI))
	}

	// Convert phase to fractional index
	indexF := phase * sinLUTScale
	index := int(indexF)
	frac := indexF - float32(index)

	// Ensure index is in bounds
	index &= sinLUTMask
	nextIndex := (index + 1) & sinLUTMask

	// Linear interpolation between adjacent samples
	return sinLUT[index] + frac*(sinLUT[nextIndex]-sinLUT[index])
}

// fastTanh returns tanh(x) using lookup table with linear interpolation.
// Input is clamped to [-4, 4] range (tanh saturates quickly outside this).
//
//go:nosplit
func fastTanh(x float32) float32 {
	// Clamp to lookup table range
	if x <= tanhLUTMin {
		return -1.0
	}
	if x >= tanhLUTMax {
		return 1.0
	}

	// Convert input to fractional index
	indexF := (x - tanhLUTMin) * tanhLUTScale
	index := int(indexF)
	frac := indexF - float32(index)

	// Bounds check (shouldn't trigger after clamping, but be safe)
	if index < 0 {
		return tanhLUT[0]
	}
	if index >= tanhLUTSize-1 {
		return tanhLUT[tanhLUTSize-1]
	}

	// Linear interpolation
	return tanhLUT[index] + frac*(tanhLUT[index+1]-tanhLUT[index])
}

// polyBLEP32 applies polynomial band-limited step correction using float32.
// This is the float32 version of polyBLEP for consistent float32 audio pipeline.
// t is the normalized phase position (0.0-1.0)
// dt is the phase increment per sample (frequency/sampleRate)
//
//go:nosplit
func polyBLEP32(t, dt float32) float32 {
	if t < dt {
		// Leading edge correction
		t /= dt
		return t + t - t*t - 1.0
	} else if t > 1.0-dt {
		// Trailing edge correction
		t = (t - 1.0) / dt
		return t*t + t + t + 1.0
	}
	return 0.0
}
