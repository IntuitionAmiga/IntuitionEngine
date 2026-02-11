// audio_benchmark_test.go - Performance benchmarks for audio subsystem

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

import (
	"math"
	"testing"
)

// createBenchmarkChip creates a SoundChip configured for benchmarking
// without an audio backend (backend = 0 for null/headless)
func createBenchmarkChip(t testing.TB) *SoundChip {
	chip := &SoundChip{
		filterLP:    DEFAULT_FILTER_LP,
		filterBP:    DEFAULT_FILTER_BP,
		filterHP:    DEFAULT_FILTER_HP,
		preDelayBuf: make([]float32, PRE_DELAY_MS*MS_TO_SAMPLES),
	}
	chip.enabled.Store(true)
	chip.sampleTicker.Store(&sampleTickerHolder{})

	// Initialize channels
	waveTypes := []int{WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE}
	for i := range NUM_CHANNELS {
		chip.channels[i] = &Channel{
			waveType:            waveTypes[i],
			attackTime:          DEFAULT_ATTACK_TIME,
			decayTime:           DEFAULT_DECAY_TIME,
			sustainLevel:        DEFAULT_SUSTAIN,
			releaseTime:         DEFAULT_RELEASE_TIME,
			envelopePhase:       ENV_ATTACK,
			noiseSR:             NOISE_LFSR_SEED,
			dutyCycle:           DEFAULT_DUTY_CYCLE,
			phase:               MIN_PHASE,
			volume:              MIN_VOLUME,
			psgPlusGain:         1.0,
			psgPlusOversample:   1,
			pokeyPlusGain:       1.0,
			pokeyPlusOversample: 1,
		}
	}

	// Initialize comb filters for reverb
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

// setupBenchmarkChannel configures a single channel for benchmarking
func setupBenchmarkChannel(chip *SoundChip, chIdx int, waveType int, freq float32) {
	ch := chip.channels[chIdx]
	ch.enabled = true
	ch.gate = true
	ch.waveType = waveType
	ch.frequency = freq
	ch.volume = 0.8
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
}

// BenchmarkSoundChip_GenerateSample benchmarks basic sample generation
// with a single sine channel (simplest case)
func BenchmarkSoundChip_GenerateSample(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SINE, 440.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_Square benchmarks square wave generation
// which includes polyBLEP anti-aliasing
func BenchmarkSoundChip_GenerateSample_Square(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 440.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_Sawtooth benchmarks sawtooth generation
// which includes polyBLEP anti-aliasing
func BenchmarkSoundChip_GenerateSample_Sawtooth(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SAWTOOTH, 440.0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_AllChannels benchmarks with all 4 channels active
func BenchmarkSoundChip_GenerateSample_AllChannels(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 261.63)   // C4
	setupBenchmarkChannel(chip, 1, WAVE_TRIANGLE, 329.63) // E4
	setupBenchmarkChannel(chip, 2, WAVE_SINE, 392.0)      // G4
	setupBenchmarkChannel(chip, 3, WAVE_NOISE, 1000.0)    // Noise

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_WithFilter benchmarks with global filter enabled
func BenchmarkSoundChip_GenerateSample_WithFilter(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 440.0)
	chip.filterType = 1 // Low-pass
	chip.filterCutoff = 0.5
	chip.filterResonance = 0.3

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_WithReverb benchmarks with reverb enabled
func BenchmarkSoundChip_GenerateSample_WithReverb(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SINE, 440.0)
	chip.reverbMix = 0.3

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_WithOverdrive benchmarks with overdrive enabled
func BenchmarkSoundChip_GenerateSample_WithOverdrive(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 440.0)
	chip.overdriveLevel = 2.0

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_AllEffects benchmarks with all effects enabled
func BenchmarkSoundChip_GenerateSample_AllEffects(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 440.0)
	setupBenchmarkChannel(chip, 1, WAVE_SINE, 880.0)
	chip.filterType = 1
	chip.filterCutoff = 0.5
	chip.filterResonance = 0.3
	chip.overdriveLevel = 1.5
	chip.reverbMix = 0.2

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_PSGPlus benchmarks PSG+ enhanced mode
func BenchmarkSoundChip_GenerateSample_PSGPlus(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 440.0)
	chip.SetPSGPlusEnabled(true)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkSoundChip_GenerateSample_PWM benchmarks square wave with PWM modulation
func BenchmarkSoundChip_GenerateSample_PWM(b *testing.B) {
	chip := createBenchmarkChip(b)
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.waveType = WAVE_SQUARE
	ch.frequency = 440.0
	ch.volume = 0.8
	ch.envelopeLevel = 1.0
	ch.envelopePhase = ENV_SUSTAIN
	ch.sustainLevel = 1.0
	ch.pwmEnabled = true
	ch.pwmRate = 5.0
	ch.pwmDepth = 0.3

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.GenerateSample()
	}
}

// BenchmarkChannel_GenerateSample_Sine benchmarks sine waveform generation
func BenchmarkChannel_GenerateSample_Sine(b *testing.B) {
	chip := createBenchmarkChip(b)
	ch := chip.channels[0]
	ch.enabled = true
	ch.waveType = WAVE_SINE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ch.generateWaveSample(SAMPLE_RATE, 1.0/SAMPLE_RATE)
	}
}

// BenchmarkChannel_GenerateSample_Square benchmarks square waveform with polyBLEP
func BenchmarkChannel_GenerateSample_Square(b *testing.B) {
	chip := createBenchmarkChip(b)
	ch := chip.channels[0]
	ch.enabled = true
	ch.waveType = WAVE_SQUARE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.dutyCycle = 0.5

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ch.generateWaveSample(SAMPLE_RATE, 1.0/SAMPLE_RATE)
	}
}

// BenchmarkChannel_GenerateSample_Triangle benchmarks triangle waveform generation
func BenchmarkChannel_GenerateSample_Triangle(b *testing.B) {
	chip := createBenchmarkChip(b)
	ch := chip.channels[0]
	ch.enabled = true
	ch.waveType = WAVE_TRIANGLE
	ch.frequency = 440.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ch.generateWaveSample(SAMPLE_RATE, 1.0/SAMPLE_RATE)
	}
}

// BenchmarkChannel_GenerateSample_Noise benchmarks noise generation with LFSR
func BenchmarkChannel_GenerateSample_Noise(b *testing.B) {
	chip := createBenchmarkChip(b)
	ch := chip.channels[0]
	ch.enabled = true
	ch.waveType = WAVE_NOISE
	ch.frequency = 1000.0
	ch.volume = 1.0
	ch.envelopeLevel = 1.0
	ch.noiseSR = NOISE_LFSR_SEED

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ch.generateWaveSample(SAMPLE_RATE, 1.0/SAMPLE_RATE)
	}
}

// BenchmarkMathSin benchmarks math.Sin for baseline comparison
func BenchmarkMathSin(b *testing.B) {
	phase := float64(0)
	phaseInc := TWO_PI * 440.0 / SAMPLE_RATE

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = math.Sin(phase)
		phase += phaseInc
		if phase >= TWO_PI {
			phase -= TWO_PI
		}
	}
}

// BenchmarkMathTanh benchmarks math.Tanh for baseline comparison
func BenchmarkMathTanh(b *testing.B) {
	sample := float64(0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = math.Tanh(sample * 1.18)
		sample = -sample // Alternate to prevent optimization
	}
}

// BenchmarkPolyBLEP benchmarks the polyBLEP anti-aliasing function
func BenchmarkPolyBLEP(b *testing.B) {
	t := 0.001 // Near edge
	dt := 440.0 / SAMPLE_RATE

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = polyBLEP(t, dt)
	}
}

// BenchmarkCalculateFilterCutoff benchmarks filter cutoff calculation
func BenchmarkCalculateFilterCutoff(b *testing.B) {
	cutoff := float32(0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = calculateFilterCutoff(cutoff)
	}
}

// BenchmarkFastExp benchmarks the fastExp approximation
func BenchmarkFastExp(b *testing.B) {
	x := float32(0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = fastExp(x)
	}
}

// BenchmarkMathExp benchmarks math.Exp for comparison
func BenchmarkMathExp(b *testing.B) {
	x := float64(0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = math.Exp(x)
	}
}

// BenchmarkFastSin benchmarks the LUT-based sine function
func BenchmarkFastSin(b *testing.B) {
	phase := float32(0)
	phaseInc := float32(TWO_PI * 440.0 / SAMPLE_RATE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = fastSin(phase)
		phase += phaseInc
		if phase >= TWO_PI {
			phase -= TWO_PI
		}
	}
}

// BenchmarkFastTanh benchmarks the LUT-based tanh function
func BenchmarkFastTanh(b *testing.B) {
	sample := float32(0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = fastTanh(sample * 1.18)
		sample = -sample // Alternate to prevent optimization
	}
}

// BenchmarkPolyBLEP32 benchmarks the float32 polyBLEP function
func BenchmarkPolyBLEP32(b *testing.B) {
	t := float32(0.001) // Near edge
	dt := float32(440.0 / SAMPLE_RATE)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = polyBLEP32(t, dt)
	}
}

// BenchmarkApplyReverb benchmarks the reverb effect processing
func BenchmarkApplyReverb(b *testing.B) {
	chip := createBenchmarkChip(b)
	sample := float32(0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.applyReverb(sample)
	}
}

// BenchmarkUpdateEnvelope benchmarks the envelope generator
func BenchmarkUpdateEnvelope(b *testing.B) {
	chip := createBenchmarkChip(b)
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.attackTime = 4410 // 100ms
	ch.decayTime = 8820  // 200ms
	ch.sustainLevel = 0.7
	ch.releaseTime = 4410
	ch.envelopePhase = ENV_ATTACK
	ch.envelopeLevel = 0

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ch.updateEnvelope()
		// Reset periodically to keep envelope cycling
		if i%10000 == 0 {
			ch.envelopePhase = ENV_ATTACK
			ch.envelopeLevel = 0
			ch.envelopeSample = 0
		}
	}
}

// BenchmarkClampF32 benchmarks the clamping function
func BenchmarkClampF32(b *testing.B) {
	values := []float32{-1.5, -0.5, 0.0, 0.5, 1.5}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		v := values[i%len(values)]
		_ = clampF32(v, MIN_SAMPLE, MAX_SAMPLE)
	}
}

// BenchmarkFlushDenormal benchmarks the denormal flushing function
func BenchmarkFlushDenormal(b *testing.B) {
	values := []float32{1e-20, 0.5, 1e-16, -0.5, 1e-14}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		v := values[i%len(values)]
		_ = flushDenormal(v)
	}
}

// BenchmarkGenerateSample_1Second generates 1 second of audio for throughput measurement
func BenchmarkGenerateSample_1Second(b *testing.B) {
	chip := createBenchmarkChip(b)
	setupBenchmarkChannel(chip, 0, WAVE_SQUARE, 440.0)
	setupBenchmarkChannel(chip, 1, WAVE_SINE, 880.0)

	samples := SAMPLE_RATE // 1 second worth

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for range samples {
			_ = chip.GenerateSample()
		}
	}

	b.ReportMetric(float64(samples*b.N)/b.Elapsed().Seconds(), "samples/sec")
}

// BenchmarkPSG_VolumeGain benchmarks PSG volume gain lookup table
func BenchmarkPSG_VolumeGain(b *testing.B) {
	levels := []uint8{0, 4, 8, 12, 15}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = psgVolumeGain(level, false)
	}
}

// BenchmarkPSG_VolumeGain_Plus benchmarks PSG+ logarithmic volume curve
func BenchmarkPSG_VolumeGain_Plus(b *testing.B) {
	levels := []uint8{0, 4, 8, 12, 15}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = psgVolumeGain(level, true)
	}
}

// BenchmarkSID_VolumeGain benchmarks SID volume gain lookup table
func BenchmarkSID_VolumeGain(b *testing.B) {
	levels := []uint8{0, 4, 8, 12, 15}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = sidVolumeGain(level, false)
	}
}

// BenchmarkSID_VolumeGain_Plus benchmarks SID+ logarithmic volume curve
func BenchmarkSID_VolumeGain_Plus(b *testing.B) {
	levels := []uint8{0, 4, 8, 12, 15}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = sidVolumeGain(level, true)
	}
}

// BenchmarkTED_VolumeGain benchmarks TED volume gain lookup table
func BenchmarkTED_VolumeGain(b *testing.B) {
	levels := []uint8{0, 2, 4, 6, 8}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = tedVolumeGain(level, false)
	}
}

// BenchmarkTED_VolumeGain_Plus benchmarks TED+ logarithmic volume curve
func BenchmarkTED_VolumeGain_Plus(b *testing.B) {
	levels := []uint8{0, 2, 4, 6, 8}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = tedVolumeGain(level, true)
	}
}

// BenchmarkPOKEY_VolumeGain benchmarks POKEY volume gain lookup table
func BenchmarkPOKEY_VolumeGain(b *testing.B) {
	levels := []uint8{0, 4, 8, 12, 15}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = pokeyVolumeGain(level, false)
	}
}

// BenchmarkPOKEY_VolumeGain_Plus benchmarks POKEY+ logarithmic volume curve
func BenchmarkPOKEY_VolumeGain_Plus(b *testing.B) {
	levels := []uint8{0, 4, 8, 12, 15}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		level := levels[i%len(levels)]
		_ = pokeyVolumeGain(level, true)
	}
}
