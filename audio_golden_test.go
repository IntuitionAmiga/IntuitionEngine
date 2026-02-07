// audio_golden_test.go - Golden output tests for audio quality regression

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

/*
Golden output tests capture expected audio output for regression testing.
These tests verify that optimizations do not change the audio quality.

The tests use deterministic configurations and check statistical properties
of the generated samples (RMS, peak, frequency content) rather than
exact bit-for-bit matching, since floating-point optimizations may
introduce minor numerical differences that are inaudible.
*/

package main

import (
	"math"
	"testing"
)

// createGoldenChip creates a SoundChip with deterministic initial state
func createGoldenChip() *SoundChip {
	chip := &SoundChip{
		filterLP:    DEFAULT_FILTER_LP,
		filterBP:    DEFAULT_FILTER_BP,
		filterHP:    DEFAULT_FILTER_HP,
		preDelayBuf: make([]float32, PRE_DELAY_MS*MS_TO_SAMPLES),
	}
	chip.enabled.Store(true)
	chip.sampleTicker.Store(&sampleTickerHolder{})

	// Initialize channels with deterministic state
	waveTypes := []int{WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE}
	for i := 0; i < NUM_CHANNELS; i++ {
		chip.channels[i] = &Channel{
			waveType:            waveTypes[i],
			attackTime:          DEFAULT_ATTACK_TIME,
			decayTime:           DEFAULT_DECAY_TIME,
			sustainLevel:        DEFAULT_SUSTAIN,
			releaseTime:         DEFAULT_RELEASE_TIME,
			envelopePhase:       ENV_SUSTAIN, // Start in sustain for consistent output
			envelopeLevel:       1.0,
			noiseSR:             NOISE_LFSR_SEED,
			dutyCycle:           DEFAULT_DUTY_CYCLE,
			phase:               0,
			volume:              0,
			psgPlusGain:         1.0,
			psgPlusOversample:   1,
			pokeyPlusGain:       1.0,
			pokeyPlusOversample: 1,
		}
	}

	// Initialize comb filters
	var combDelays = []int{COMB_DELAY_1, COMB_DELAY_2, COMB_DELAY_3, COMB_DELAY_4}
	var combDecays = []float32{COMB_DECAY_1, COMB_DECAY_2, COMB_DECAY_3, COMB_DECAY_4}

	for i := range chip.combFilters {
		chip.combFilters[i] = CombFilter{
			buffer: make([]float32, combDelays[i]),
			decay:  combDecays[i],
		}
	}

	// Initialize allpass filters
	var allpassDelays = []int{ALLPASS_DELAY_1, ALLPASS_DELAY_2}
	for i := range chip.allpassBuf {
		chip.allpassBuf[i] = make([]float32, allpassDelays[i])
	}

	return chip
}

// goldenStats captures statistical properties of audio output
type goldenStats struct {
	rms           float64 // Root mean square
	peak          float64 // Maximum absolute value
	dcOffset      float64 // Average (DC offset)
	zeroCrossings int     // Number of zero crossings
}

// computeStats calculates statistical properties of samples
func computeStats(samples []float32) goldenStats {
	if len(samples) == 0 {
		return goldenStats{}
	}

	var sum, sumSq float64
	var peak float64
	var crossings int
	var prevSign bool

	for i, s := range samples {
		v := float64(s)
		sum += v
		sumSq += v * v
		if math.Abs(v) > peak {
			peak = math.Abs(v)
		}

		// Count zero crossings
		currentSign := s >= 0
		if i > 0 && currentSign != prevSign {
			crossings++
		}
		prevSign = currentSign
	}

	n := float64(len(samples))
	return goldenStats{
		rms:           math.Sqrt(sumSq / n),
		peak:          peak,
		dcOffset:      sum / n,
		zeroCrossings: crossings,
	}
}

// TestGolden_SineWave verifies sine wave generation produces expected statistics
func TestGolden_SineWave(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SINE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.phase = 0

	// Generate exactly 1 period of 440Hz at 44100 sample rate
	// 44100 / 440 = 100.227... samples per period
	numSamples := 4410 // ~10 periods for stable stats

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Sine wave RMS should be peak / sqrt(2) = ~0.707
	expectedRMS := 0.707
	if math.Abs(stats.rms-expectedRMS) > 0.05 {
		t.Errorf("Sine RMS = %f, expected ~%f", stats.rms, expectedRMS)
	}

	// Peak should be close to 1.0
	if stats.peak < 0.95 || stats.peak > 1.05 {
		t.Errorf("Sine peak = %f, expected ~1.0", stats.peak)
	}

	// DC offset should be nearly 0
	if math.Abs(stats.dcOffset) > 0.01 {
		t.Errorf("Sine DC offset = %f, expected ~0", stats.dcOffset)
	}

	// 440Hz at 44100 sample rate = 100.227 samples/period
	// For 4410 samples (~44 periods), expect ~88 zero crossings
	expectedCrossings := 88
	if stats.zeroCrossings < expectedCrossings-10 || stats.zeroCrossings > expectedCrossings+10 {
		t.Errorf("Sine zero crossings = %d, expected ~%d", stats.zeroCrossings, expectedCrossings)
	}
}

// TestGolden_SquareWave verifies square wave generation with polyBLEP
func TestGolden_SquareWave(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SQUARE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.dutyCycle = 0.5
	ch.phase = 0

	numSamples := 4410

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Square wave with polyBLEP: RMS should be close to peak
	// (pure square has RMS = peak, polyBLEP slightly reduces this)
	if stats.rms < 0.85 || stats.rms > 1.05 {
		t.Errorf("Square RMS = %f, expected 0.85-1.0", stats.rms)
	}

	// DC offset should be nearly 0 for 50% duty cycle
	if math.Abs(stats.dcOffset) > 0.05 {
		t.Errorf("Square DC offset = %f, expected ~0", stats.dcOffset)
	}
}

// TestGolden_TriangleWave verifies triangle wave generation
func TestGolden_TriangleWave(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_TRIANGLE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.phase = 0

	numSamples := 4410

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Triangle wave RMS should be peak / sqrt(3) = ~0.577
	expectedRMS := 0.577
	if math.Abs(stats.rms-expectedRMS) > 0.1 {
		t.Errorf("Triangle RMS = %f, expected ~%f", stats.rms, expectedRMS)
	}

	// DC offset should be nearly 0
	if math.Abs(stats.dcOffset) > 0.01 {
		t.Errorf("Triangle DC offset = %f, expected ~0", stats.dcOffset)
	}
}

// TestGolden_Sawtooth verifies sawtooth wave generation with polyBLEP
func TestGolden_Sawtooth(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SAWTOOTH
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.phase = 0

	numSamples := 4410

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Sawtooth RMS should be peak / sqrt(3) = ~0.577
	expectedRMS := 0.577
	if math.Abs(stats.rms-expectedRMS) > 0.1 {
		t.Errorf("Sawtooth RMS = %f, expected ~%f", stats.rms, expectedRMS)
	}

	// DC offset should be nearly 0
	if math.Abs(stats.dcOffset) > 0.05 {
		t.Errorf("Sawtooth DC offset = %f, expected ~0", stats.dcOffset)
	}
}

// TestGolden_Noise verifies noise generation produces expected statistics
func TestGolden_Noise(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_NOISE
	ch.frequency = 44100.0 // Clock noise at sample rate
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.noiseSR = NOISE_LFSR_SEED
	ch.noiseMode = NOISE_MODE_WHITE

	numSamples := 44100

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Noise should have some non-zero RMS
	if stats.rms < 0.1 {
		t.Errorf("Noise RMS = %f, expected > 0.1", stats.rms)
	}

	// Noise should have many zero crossings (high frequency content)
	if stats.zeroCrossings < 1000 {
		t.Errorf("Noise zero crossings = %d, expected > 1000", stats.zeroCrossings)
	}
}

// TestGolden_MultiChannel verifies multi-channel mixing
func TestGolden_MultiChannel(t *testing.T) {
	chip := createGoldenChip()

	// Set up 3 channels with different frequencies (C major chord)
	for i := 0; i < 3; i++ {
		ch := chip.channels[i]
		ch.enabled = true
		ch.gate = true
		ch.waveType = WAVE_SINE
		ch.volume = 0.5
		ch.envelopeLevel = 1.0
		ch.envelopePhase = ENV_SUSTAIN
		ch.sustainLevel = 1.0
		ch.phase = 0
	}
	chip.channels[0].frequency = 261.63 // C4
	chip.channels[1].frequency = 329.63 // E4
	chip.channels[2].frequency = 392.0  // G4

	numSamples := 4410

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Mixed signal should have non-trivial RMS
	if stats.rms < 0.2 || stats.rms > 0.8 {
		t.Errorf("Multi-channel RMS = %f, expected 0.2-0.8", stats.rms)
	}

	// Peak should not clip
	if stats.peak > 1.0 {
		t.Errorf("Multi-channel peak = %f, should not exceed 1.0", stats.peak)
	}
}

// TestGolden_Filter verifies filter processing
func TestGolden_Filter(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SQUARE
	ch.frequency = 1000.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.dutyCycle = 0.5

	// Enable low-pass filter at moderate cutoff
	chip.filterType = 1 // Low-pass
	chip.filterCutoff = 0.3
	chip.filterResonance = 0.2

	numSamples := 4410

	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// Filtered square wave should have lower RMS than unfiltered
	// (harmonics are attenuated)
	if stats.rms > 0.9 {
		t.Errorf("Filtered square RMS = %f, expected < 0.9 (filter should attenuate)", stats.rms)
	}

	// Should still have non-zero output (filter may attenuate significantly)
	if stats.rms < 0.01 {
		t.Errorf("Filtered square RMS = %f, should be > 0.01", stats.rms)
	}
}

// TestGolden_Envelope verifies envelope generator
func TestGolden_Envelope(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SINE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.attackTime = 441   // 10ms attack
	ch.decayTime = 441    // 10ms decay
	ch.sustainLevel = 0.5 // 50% sustain
	ch.releaseTime = 441  // 10ms release
	ch.envelopePhase = ENV_ATTACK
	ch.envelopeLevel = 0
	ch.envelopeSample = 0

	// Generate attack phase samples
	attackSamples := make([]float32, 441)
	for i := range attackSamples {
		attackSamples[i] = chip.GenerateSample()
	}

	// During attack, RMS should increase
	firstQuarter := computeStats(attackSamples[:110])
	lastQuarter := computeStats(attackSamples[330:])

	if lastQuarter.rms <= firstQuarter.rms {
		t.Errorf("Attack envelope: last quarter RMS (%f) should be > first quarter (%f)",
			lastQuarter.rms, firstQuarter.rms)
	}
}

// TestGolden_Reverb verifies reverb effect
func TestGolden_Reverb(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SINE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0

	chip.reverbMix = 0.5

	// Generate samples with reverb
	numSamples := 4410
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	// Now mute the source and let reverb tail decay
	ch.enabled = false
	tailSamples := make([]float32, 4410)
	for i := range tailSamples {
		tailSamples[i] = chip.GenerateSample()
	}

	tailStats := computeStats(tailSamples)

	// Reverb tail should have some energy
	if tailStats.rms < 0.001 {
		t.Errorf("Reverb tail RMS = %f, expected > 0.001 (reverb should persist)", tailStats.rms)
	}
}

// TestGolden_Overdrive verifies overdrive effect
func TestGolden_Overdrive(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SINE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0

	// Generate clean samples first
	cleanSamples := make([]float32, 441)
	for i := range cleanSamples {
		cleanSamples[i] = chip.GenerateSample()
	}
	cleanStats := computeStats(cleanSamples)

	// Reset and add overdrive
	ch.phase = 0
	chip.overdriveLevel = 2.0

	driveSamples := make([]float32, 441)
	for i := range driveSamples {
		driveSamples[i] = chip.GenerateSample()
	}
	driveStats := computeStats(driveSamples)

	// Overdrive should increase RMS (saturation fills in valleys)
	if driveStats.rms <= cleanStats.rms {
		t.Logf("Warning: Overdrive RMS (%f) not greater than clean (%f)", driveStats.rms, cleanStats.rms)
	}

	// Output should still be within bounds
	if driveStats.peak > 1.0 {
		t.Errorf("Overdrive peak = %f, should not exceed 1.0", driveStats.peak)
	}
}

// TestGolden_PSGPlus verifies PSG+ enhanced mode
func TestGolden_PSGPlus(t *testing.T) {
	chip := createGoldenChip()
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SQUARE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0

	chip.SetPSGPlusEnabled(true)

	numSamples := 4410
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	stats := computeStats(samples)

	// PSG+ should produce non-zero output
	if stats.rms < 0.1 {
		t.Errorf("PSG+ RMS = %f, expected > 0.1", stats.rms)
	}

	// Output should not clip
	if stats.peak > 1.0 {
		t.Errorf("PSG+ peak = %f, should not exceed 1.0", stats.peak)
	}
}

// TestGolden_Determinism verifies that same input produces same output
func TestGolden_Determinism(t *testing.T) {
	// Run same configuration twice and verify identical output

	numSamples := 1000

	// First run
	chip1 := createGoldenChip()
	ch1 := chip1.channels[0]
	ch1.enabled = true
	ch1.gate = true
	ch1.waveType = WAVE_SINE
	ch1.frequency = 440.0
	ch1.volume = 1.0
	ch1.envelopeLevel = 1.0
	ch1.envelopePhase = ENV_SUSTAIN
	ch1.sustainLevel = 1.0

	samples1 := make([]float32, numSamples)
	for i := range samples1 {
		samples1[i] = chip1.GenerateSample()
	}

	// Second run with identical configuration
	chip2 := createGoldenChip()
	ch2 := chip2.channels[0]
	ch2.enabled = true
	ch2.gate = true
	ch2.waveType = WAVE_SINE
	ch2.frequency = 440.0
	ch2.volume = 1.0
	ch2.envelopeLevel = 1.0
	ch2.envelopePhase = ENV_SUSTAIN
	ch2.sustainLevel = 1.0

	samples2 := make([]float32, numSamples)
	for i := range samples2 {
		samples2[i] = chip2.GenerateSample()
	}

	// Samples should be identical
	for i := range samples1 {
		if samples1[i] != samples2[i] {
			t.Errorf("Determinism failed at sample %d: %f != %f", i, samples1[i], samples2[i])
			break
		}
	}
}
