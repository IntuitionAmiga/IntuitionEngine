// plus_engine_test.go - Tests for Phase 2 PLUS engine quality improvements.

package main

import (
	"math"
	"testing"
)

// --- Infrastructure Tests ---

// TestPlusAllpassRoomNoCombPeaks verifies the allpass room diffuser does not
// produce periodic autocorrelation peaks (the signature of comb filter artifacts).
func TestPlusAllpassRoomNoCombPeaks(t *testing.T) {
	_, chip := newTestPSGEngine(SAMPLE_RATE)

	// Configure channel 0: 440Hz square wave with moderate volume
	chip.mu.Lock()
	ch := chip.channels[0]
	ch.frequency = 440
	ch.volume = 0.5
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SQUARE
	ch.envelopePhase = ENV_SUSTAIN
	ch.envelopeLevel = 1.0
	ch.sustainLevel = 1.0
	chip.mu.Unlock()

	chip.SetPSGPlusEnabled(true)

	// Generate enough samples to cover multiple room delay periods
	const numSamples = 2048
	samples := make([]float32, numSamples)
	for i := range numSamples {
		samples[i] = chip.GenerateSample()
	}

	// Compute autocorrelation at the room delay lag
	// A comb filter produces strong peaks at multiples of the delay length
	delay := PSG_PLUS_ROOM_DELAY
	if delay <= 0 || delay >= numSamples/2 {
		t.Skip("delay out of range for autocorrelation test")
	}

	// Compute normalised autocorrelation at the delay lag
	var sumXY, sumX2, sumY2 float64
	for i := delay; i < numSamples; i++ {
		x := float64(samples[i-delay])
		y := float64(samples[i])
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}
	denom := math.Sqrt(sumX2 * sumY2)
	if denom == 0 {
		t.Fatal("zero energy — no signal produced")
	}
	corr := sumXY / denom

	// With a comb filter the correlation at the delay lag is close to 1.0.
	// An allpass diffuser decorrelates, so expect significantly less.
	if corr > 0.85 {
		t.Errorf("autocorrelation at delay lag = %.4f (> 0.85), suggests comb-like metallic coloring", corr)
	}
}

// TestPlusBiquadAttenuatesHighFreq verifies the biquad lowpass coefficients
// attenuate frequencies above the cutoff.
func TestPlusBiquadAttenuatesHighFreq(t *testing.T) {
	// Compute biquad coefficients for 4x oversampled rate
	effectiveSR := float32(SAMPLE_RATE * 4)
	cutoff := effectiveSR * 0.45
	b0, b1, b2, a1, a2 := computePlusBiquadCoeffs(cutoff, effectiveSR)

	// Compute the biquad frequency response at a frequency above cutoff
	// H(z) = (b0 + b1*z^-1 + b2*z^-2) / (1 + a1*z^-1 + a2*z^-2)
	// At frequency f, z = e^(j*2*pi*f/fs)
	testFreq := effectiveSR * 0.49 // Just below Nyquist — should be heavily attenuated
	omega := 2.0 * math.Pi * float64(testFreq) / float64(effectiveSR)
	cosW := math.Cos(omega)
	sinW := math.Sin(omega)

	// Numerator: b0 + b1*cos(w) + b2*cos(2w), b1*sin(w) + b2*sin(2w)
	numRe := float64(b0) + float64(b1)*cosW + float64(b2)*(2*cosW*cosW-1)
	numIm := float64(b1)*sinW + float64(b2)*(2*sinW*cosW)

	// Denominator: 1 + a1*cos(w) + a2*cos(2w), a1*sin(w) + a2*sin(2w)
	denRe := 1.0 + float64(a1)*cosW + float64(a2)*(2*cosW*cosW-1)
	denIm := float64(a1)*sinW + float64(a2)*(2*sinW*cosW)

	numMag := math.Sqrt(numRe*numRe + numIm*numIm)
	denMag := math.Sqrt(denRe*denRe + denIm*denIm)

	gain := numMag / denMag
	gainDB := 20.0 * math.Log10(gain)

	// At 0.49*fs a Butterworth LPF with cutoff at 0.45*fs should attenuate at least -6dB
	if gainDB > -6.0 {
		t.Errorf("biquad gain at %.0f Hz = %.1f dB (expected < -6 dB), b=[%.6f %.6f %.6f] a=[%.6f %.6f]",
			testFreq, gainDB, b0, b1, b2, a1, a2)
	}
}

// TestPlusTransitionNoPop verifies that enabling PLUS mode doesn't produce a
// sudden large discontinuity in sample level (pop/click). The transition starts
// at transGain=1.0 (signal is already playing at full level) so there is no
// sudden drop to silence.
func TestPlusTransitionNoPop(t *testing.T) {
	_, chip := newTestPSGEngine(SAMPLE_RATE)

	// Use a low-frequency sine to get a smooth signal with small sample-to-sample deltas
	chip.mu.Lock()
	ch := chip.channels[0]
	ch.frequency = 100
	ch.volume = 0.5
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SINE
	ch.envelopePhase = ENV_SUSTAIN
	ch.envelopeLevel = 1.0
	ch.sustainLevel = 1.0
	chip.mu.Unlock()

	// Generate baseline samples and record the last one
	var prevSample float32
	for range 64 {
		prevSample = chip.GenerateSample()
	}

	// Enable PLUS — transGain starts at 1.0 so no fade from silence
	chip.SetPSGPlusEnabled(true)

	// The first PLUS sample should be close in magnitude to the last baseline sample
	firstPlusSample := chip.GenerateSample()
	delta := firstPlusSample - prevSample
	if delta < 0 {
		delta = -delta
	}

	// With a 100 Hz sine at 44100 SR, per-sample change is ~0.014.
	// The PLUS path (oversampling + biquad) may shift the value slightly but
	// should not produce a large discontinuity.
	if delta > 0.1 {
		t.Errorf("first PLUS sample delta = %.4f (> 0.1), possible pop: prev=%.4f first=%.4f",
			delta, prevSample, firstPlusSample)
	}
}

// --- SID+ Filter Passthrough ---

// TestSIDPlusPreservesFilter verifies that SID+ output with a lowpass filter
// active differs from the same signal with no filter (proving the filter is applied).
func TestSIDPlusPreservesFilter(t *testing.T) {
	chipFiltered := newTestSoundChip()
	chipUnfiltered := newTestSoundChip()

	// Set up identical square waves
	for _, c := range []*SoundChip{chipFiltered, chipUnfiltered} {
		c.mu.Lock()
		ch := c.channels[0]
		ch.frequency = 440
		ch.volume = 0.8
		ch.enabled = true
		ch.gate = true
		ch.waveType = WAVE_SQUARE
		ch.envelopePhase = ENV_SUSTAIN
		ch.envelopeLevel = 1.0
		ch.sustainLevel = 1.0
		c.mu.Unlock()
	}

	// Enable SID+ on both
	chipFiltered.SetSIDPlusEnabled(true)
	chipUnfiltered.SetSIDPlusEnabled(true)

	// Set lowpass filter on the filtered chip
	chipFiltered.mu.Lock()
	fch := chipFiltered.channels[0]
	fch.filterModeMask = 0x01 // Lowpass
	fch.filterCutoff = 0.3
	fch.filterCutoffTarget = 0.3
	fch.filterResonance = 0.5
	fch.filterResonanceTarget = 0.5
	fch.sidFilterMode = true
	chipFiltered.mu.Unlock()

	// Generate samples and compare
	const n = 256
	var filtered, unfiltered float32
	for range n {
		filtered = chipFiltered.GenerateSample()
		unfiltered = chipUnfiltered.GenerateSample()
	}

	if filtered == unfiltered {
		t.Fatal("SID+ with filter active should differ from SID+ without filter")
	}
}

// --- Per-Engine AffectsOutput Tests ---

func TestPOKEYPlusAffectsOutput(t *testing.T) {
	chipBase := newTestSoundChip()
	chipPlus := newTestSoundChip()

	for _, c := range []*SoundChip{chipBase, chipPlus} {
		c.mu.Lock()
		ch := c.channels[0]
		ch.frequency = 440
		ch.volume = 0.5
		ch.enabled = true
		ch.gate = true
		ch.waveType = WAVE_SQUARE
		ch.envelopePhase = ENV_SUSTAIN
		ch.envelopeLevel = 1.0
		ch.sustainLevel = 1.0
		c.mu.Unlock()
	}

	chipPlus.SetPOKEYPlusEnabled(true)

	var baseSample, plusSample float32
	for range 4 {
		baseSample = chipBase.GenerateSample()
		plusSample = chipPlus.GenerateSample()
	}
	if baseSample == plusSample {
		t.Fatal("expected POKEY+ sample to differ from baseline")
	}
}

func TestSIDPlusAffectsOutput(t *testing.T) {
	chipBase := newTestSoundChip()
	chipPlus := newTestSoundChip()

	for _, c := range []*SoundChip{chipBase, chipPlus} {
		c.mu.Lock()
		ch := c.channels[0]
		ch.frequency = 440
		ch.volume = 0.5
		ch.enabled = true
		ch.gate = true
		ch.waveType = WAVE_SQUARE
		ch.envelopePhase = ENV_SUSTAIN
		ch.envelopeLevel = 1.0
		ch.sustainLevel = 1.0
		c.mu.Unlock()
	}

	chipPlus.SetSIDPlusEnabled(true)

	var baseSample, plusSample float32
	for range 4 {
		baseSample = chipBase.GenerateSample()
		plusSample = chipPlus.GenerateSample()
	}
	if baseSample == plusSample {
		t.Fatal("expected SID+ sample to differ from baseline")
	}
}

func TestTEDPlusAffectsOutput(t *testing.T) {
	chipBase := newTestSoundChip()
	chipPlus := newTestSoundChip()

	for _, c := range []*SoundChip{chipBase, chipPlus} {
		c.mu.Lock()
		ch := c.channels[0]
		ch.frequency = 440
		ch.volume = 0.5
		ch.enabled = true
		ch.gate = true
		ch.waveType = WAVE_SQUARE
		ch.envelopePhase = ENV_SUSTAIN
		ch.envelopeLevel = 1.0
		ch.sustainLevel = 1.0
		c.mu.Unlock()
	}

	chipPlus.SetTEDPlusEnabled(true)

	var baseSample, plusSample float32
	for range 4 {
		baseSample = chipBase.GenerateSample()
		plusSample = chipPlus.GenerateSample()
	}
	if baseSample == plusSample {
		t.Fatal("expected TED+ sample to differ from baseline")
	}
}

func TestAHXPlusAffectsOutput(t *testing.T) {
	chipBase := newTestSoundChip()
	chipPlus := newTestSoundChip()

	for _, c := range []*SoundChip{chipBase, chipPlus} {
		c.mu.Lock()
		ch := c.channels[0]
		ch.frequency = 440
		ch.volume = 0.5
		ch.enabled = true
		ch.gate = true
		ch.waveType = WAVE_SQUARE
		ch.envelopePhase = ENV_SUSTAIN
		ch.envelopeLevel = 1.0
		ch.sustainLevel = 1.0
		c.mu.Unlock()
	}

	chipPlus.SetAHXPlusEnabled(true)

	var baseSample, plusSample float32
	for range 4 {
		baseSample = chipBase.GenerateSample()
		plusSample = chipPlus.GenerateSample()
	}
	if baseSample == plusSample {
		t.Fatal("expected AHX+ sample to differ from baseline")
	}
}
