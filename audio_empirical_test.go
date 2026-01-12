// audio_empirical_test.go

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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
	This file contains the empirical tests for the audio system.

	These tests are designed to verify the correctness of the audio synthesis by capturing samples
	and analyzing the waveforms, envelopes, and modulation effects etc to ensure that each hardware feature
	is functioning as expected.

*/

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Audio frequencies and musical parameters
const (
	// Standard musical frequencies
	A2_FREQUENCY uint32 = 110 // Low A
	C4_FREQUENCY uint32 = 262 // Middle C
	E4_FREQUENCY uint32 = 330 // E above middle C
	G4_FREQUENCY uint32 = 392 // G above middle C
	A4_FREQUENCY uint32 = 440 // Concert pitch A4
	A5_FREQUENCY uint32 = 880 // One octave above A4

	// Frequency range limits
	MIN_FREQUENCY uint32 = 20    // Minimum audible frequency
	MAX_FREQUENCY uint32 = 20000 // Maximum audible frequency

	// Test frequencies
	TEST_LOW_FREQ_HZ  = 100  // Test low frequency
	TEST_MID_FREQ_HZ  = 1000 // Test mid frequency
	TEST_HIGH_FREQ_HZ = 8000 // Test high frequency
)

// Channel configuration
const (
	// Channel indexes
	SQUARE_CHANNEL_IDX   = 0
	TRIANGLE_CHANNEL_IDX = 1
	SINE_CHANNEL_IDX     = 2
	NOISE_CHANNEL_IDX    = 3

	// Binary control values
	ENABLED  = 1
	DISABLED = 0

	// Register values
	REGISTER_ENABLE = 3 // Value to enable a channel (gate and enable bits)
)

// Envelope parameters
const (
	// Attack times
	ATTACK_INSTANT_MS   = 1   // Instant attack time
	FAST_ATTACK_MS      = 1   // Fast attack time in milliseconds
	ATTACK_FAST_MS      = 5   // Fast attack time
	ATTACK_SLOW_MS      = 50  // Slow attack time
	ATTACK_VERY_SLOW_MS = 100 // Very slow attack time

	// Decay times
	DECAY_FAST_MS      = 20  // Fast decay time
	MODERATE_DECAY_MS  = 50  // Moderate decay time in milliseconds
	DECAY_MEDIUM_MS    = 50  // Medium decay time
	DECAY_SLOW_MS      = 100 // Slow decay time
	DECAY_VERY_SLOW_MS = 200 // Very slow decay time

	// Sustain levels
	SUSTAIN_LOW    = 128 // 50%
	SUSTAIN_MEDIUM = 180 // ~70%
	HIGH_SUSTAIN   = 200 // High sustain level (out of 255)
	SUSTAIN_HIGH   = 200 // ~78%

	// Release times
	RELEASE_FAST_MS       = 10  // Fast release time
	SHORT_RELEASE_MS      = 20  // Short release time in milliseconds
	RELEASE_MEDIUM_MS     = 20  // Medium release time
	RELEASE_SLOW_MS       = 50  // Slow release time
	RELEASE_VERY_SLOW_MS  = 100 // Very slow release time
	RELEASE_EXTRA_SLOW_MS = 200 // Extra slow release time

	// Envelope limits
	ENVELOPE_MAX_VALUE uint32 = 255 // Maximum envelope value
	SUSTAIN_LEVEL_MAX  uint32 = 255 // Maximum sustain level
)

// Modulation parameters
const (
	// PWM parameters
	PWM_DEPTH_NORMAL = 64   // Standard PWM modulation depth
	PWM_ENABLE_BIT   = 0x80 // Bit that enables PWM

	// PWM rates
	PWM_RATE_SLOW   = 0x10 // Slow PWM rate
	PWM_RATE_MEDIUM = 0x30 // Medium PWM rate
	PWM_RATE_NORMAL = 0x30 // Standard PWM rate
	PWM_RATE_FAST   = 0x70 // Fast PWM rate

	// Sweep control
	SWEEP_ENABLE_BIT     = 0x80 // Bit that enables frequency sweep
	SWEEP_DIRECTION_UP   = 0x08 // Bit for upward sweep direction
	SWEEP_DIRECTION_DOWN = 0x00 // Value for downward sweep direction

	// Sweep periods
	SWEEP_PERIOD_FAST   = 0x01 // Fast sweep period
	SWEEP_PERIOD_MEDIUM = 0x02 // Medium sweep period
	SWEEP_PERIOD_SLOW   = 0x03 // Slow sweep period

	// Sweep ranges
	SWEEP_SHIFT_NARROW    = 1 // Narrow frequency sweep range
	SWEEP_SHIFT_MEDIUM    = 2 // Medium frequency sweep range
	SWEEP_SHIFT_WIDE      = 3 // Wide frequency sweep range
	SWEEP_SHIFT_VERY_WIDE = 4 // Very wide frequency sweep range

	// Modulation limits and defaults
	MODULATION_DEPTH_MAX = 255  // Maximum modulation depth
	MIN_MOD_RATE_HZ      = 0.1  // Minimum modulation rate (Hz)
	MAX_MOD_RATE_HZ      = 20.0 // Maximum modulation rate (Hz)
	MOD_DEPTH_NORMAL     = 0.5  // Standard modulation depth (0-1)
)

// Filter settings and parameters
const (
	// Filter types
	FILTER_TYPE_NONE     = 0 // No filtering
	FILTER_TYPE_LOWPASS  = 1 // Low-pass filter
	FILTER_TYPE_HIGHPASS = 2 // High-pass filter
	FILTER_TYPE_BANDPASS = 3 // Band-pass filter

	// Filter cutoff presets
	FILTER_CUTOFF_LOW      = 32  // ~12.5%
	FILTER_CUTOFF_MID_LOW  = 64  // 25%
	FILTER_CUTOFF_MID      = 128 // 50%
	FILTER_CUTOFF_MID_HIGH = 192 // 75%
	FILTER_CUTOFF_HIGH     = 200 // ~78%

	// Filter resonance presets
	FILTER_RESONANCE_LOW    = 64
	FILTER_RESONANCE_MEDIUM = 128
	FILTER_RESONANCE_HIGH   = 192
	FILTER_RESONANCE_MAX    = 255

	// Filter limits
	FILTER_CUTOFF_MAX = 255  // Maximum filter cutoff value
	FILTER_Q_MIN      = 0.1  // Minimum resonance Q factor
	FILTER_Q_MAX      = 10.0 // Maximum resonance Q factor

	// Filter frequency ranges
	LP_FILTER_MIN_CUTOFF = 20.0    // Minimum low-pass filter cutoff (Hz)
	LP_FILTER_MAX_CUTOFF = 20000.0 // Maximum low-pass filter cutoff (Hz)
)

// Duty cycle parameters
const (
	HALF_DUTY_CYCLE                = 128 // 50% duty cycle as 8-bit value
	DUTY_CYCLE_NORMAL              = 0.5 // 50% duty cycle
	MIN_DUTY_CYCLE          uint32 = 0   // Minimum duty cycle
	MAX_DUTY_CYCLE          uint32 = 255 // Maximum duty cycle
	TEST_DUTY_CYCLE_DEFAULT        = 0.5 // Default duty cycle for square waves
)

// Amplitude and normalization values
const (
	AMPLITUDE_NORMAL             = 1.0 // Full-scale normalized amplitude
	FULL_SCALE_AMPLITUDE float64 = 1.0 // Normalized full-scale amplitude
	NORMAL_PHASE_VALUE           = 0.0 // Expected phase value for unshifted signals
	PHASE_TOLERANCE              = 0.1 // Acceptable phase variation
)

// Timing and duration constants
const (
	// Capture durations
	CAPTURE_SHORT_MS = 100 // Standard capture duration (milliseconds)
	CAPTURE_LONG_MS  = 300 // Extended capture for modulation (milliseconds)

	// Setup and transition delays
	SETUP_DELAY_MS      = 100 // Delay after setup (milliseconds)
	GATE_OFF_DELAY_MS   = 50  // Delay after turning off gate (milliseconds)
	PHASE_TRANSITION_MS = 20  // Standard wait time between envelope phases (milliseconds)

	// Wait intervals
	WAIT_TIME_SHORT_MS     = 10              // Short wait interval (milliseconds)
	WAIT_TIME_MEDIUM_MS    = 50              // Medium wait interval (milliseconds)
	WAIT_TIME_LONG_MS      = 200             // Long wait interval (milliseconds)
	ONE_SECOND             = 1               // One second duration
	POST_CAPTURE_WAIT_TIME = 1 * time.Second // Wait time after capturing audio

	// Playback durations
	PLAYBACK_SHORT_SECONDS          = 1.0 // Short playback in seconds
	PLAYBACK_MEDIUM_SECONDS         = 2.0 // Medium playback in seconds
	PLAYBACK_SECONDS        float32 = 5.0 // Standard playback duration

	// Effect durations
	DRY_DURATION   = 0.1 // 100ms
	WET_DURATION   = 0.2 // 200ms
	DECAY_DURATION = 0.5 // 500ms

	// Time units
	MILLISECOND time.Duration = time.Millisecond
)

// Analysis thresholds and tolerances
const (
	// Tolerance values
	FREQ_TOLERANCE       = 0.02 // 2% tolerance for frequency measurements
	FREQUENCY_TOLERANCE  = 0.02 // 2% tolerance for frequency measurements
	AMPLITUDE_TOLERANCE  = 0.02 // 2% tolerance for amplitude measurements
	DUTY_TOLERANCE       = 0.02 // 2% tolerance for duty cycle measurements
	DUTY_CYCLE_TOLERANCE = 0.02 // 2% tolerance for duty cycle measurements
	DEFAULT_TOLERANCE    = 0.2  // Default tolerance for measurements

	// Signal thresholds and detection
	ZERO_CROSSING_THRESHOLD    = 0.0   // Threshold for zero crossing detection
	SIGNAL_THRESHOLD           = 1e-6  // Minimum signal threshold
	MIN_SIGNAL_AMPLITUDE       = 1e-6  // Minimum detectable signal
	MIN_SIGNAL_STRENGTH        = 1e-6  // Minimum detectable signal strength
	ZERO_DETECTION_THRESHOLD   = 0.001 // Threshold for detecting zero value
	PEAK_DETECTION_THRESHOLD   = 0.01  // Minimum threshold for peak detection
	MIN_PEAK_PROMINENCE_FACTOR = 0.01  // 1% of max amplitude
	CROSSING_THRESHOLD         = 0.01  // Threshold for crossings

	// Dynamic range and amplitude thresholds
	MIN_DYNAMIC_RANGE = 0.8 // Minimum expected dynamic range for noise
	MIN_ATTACK_AMP    = 0.9 // Minimum attack phase amplitude
	MAX_RELEASE_AMP   = 0.7 // Maximum release phase amplitude

	// Decay thresholds
	DECAY_THRESHOLD_DB   = -60.0 // Decay threshold in dB
	DECAY_THRESHOLD      = -60.0 // dB
	NOISE_FLOOR_DB       = -80.0 // Noise floor in dB
	DB_CONVERSION_FACTOR = 20.0  // Factor for dB conversion (20*log10)

	// Analysis parameters
	SAMPLES_SEGMENT_COUNT     = 10 // Number of segments for sample analysis
	MIN_PEAKS_FOR_ANALYSIS    = 5  // Minimum number of peaks required for analysis
	MIN_PEAKS_REQUIRED        = 5  // Minimum peaks needed for spectral analysis
	MIN_ANALYSIS_SAMPLES      = 3  // Minimum samples for analysis
	SMOOTHING_WINDOW_SIZE     = 5  // Window size for signal smoothing
	CROSS_CORRELATION_SAMPLES = 3  // Samples to use for cross-correlation

	// Frequency analysis
	MIN_ANALYSIS_SEGMENT_SIZE = 512  // Minimum samples for frequency analysis
	FFT_BINS_STANDARD         = 1024 // Standard FFT size
	MIN_FREQ_RESOLUTION_HZ    = 0.5  // Minimum frequency resolution

	// Audio processing thresholds
	MIN_SYNC_PURITY       = 0.15  // Minimum waveform purity for sync
	CORRELATION_THRESHOLD = 0.8   // Threshold for pattern detection
	MIN_METALLIC_DENSITY  = 0.001 // Minimum density for metallic noise
	MIN_PURITY_THRESHOLD  = 0.15  // Minimum threshold for signal purity
	MIN_RESONANCE_RATIO   = 0.7   // Minimum ratio for resonance detection
	MAX_CORRELATION       = 0.9   // Maximum allowed correlation for noise

	// Test thresholds
	TEST_MIN_WAVE_PURITY  = 0.95 // Minimum purity for pure waveforms
	TEST_MIN_FILTER_ATTEN = 0.5  // Minimum filter attenuation
	TEST_MAX_NOISE_CORR   = 0.1  // Maximum allowed noise correlation

	// Test parameters
	TEST_DURATION_DEFAULT_MS = 100   // Default test duration (ms)
	TEST_SAMPLE_RATE_NORMAL  = 44100 // Normal sample rate for testing
)

// PCM and audio format parameters
const (
	// PCM values
	PCM_MIN       = -32768 // Minimum 16-bit PCM value
	PCM_MAX       = 32767  // Maximum 16-bit PCM value
	MIN_PCM_VALUE = -32768 // Minimum 16-bit PCM value
	MAX_PCM_VALUE = 32767  // Maximum 16-bit PCM value

	// Audio parameters
	BITS_PER_SAMPLE uint32 = 16 // Bits per sample

	// WAV file parameters
	RIFF_CHUNK_SIZE_OFFSET = 4  // Offset of chunk size in RIFF header
	FORMAT_SIZE            = 16 // Size of format chunk
	PCM_FORMAT             = 1  // PCM format code
	MONO_CHANNELS          = 1  // Mono audio
	WAV_HEADER_SIZE        = 44 // Size of WAV header in bytes
	WAV_FORMAT_PCM         = 1  // PCM format code for WAV
	WAV_BITS_PER_SAMPLE    = 16 // Standard bit depth
	WAV_BYTES_PER_SAMPLE   = 2  // Bytes per sample (16 bits = 2 bytes)
	WAV_MONO_CHANNELS      = 1  // Mono audio
	WAV_STEREO_CHANNELS    = 2  // Stereo audio
	WAV_RIFF_CHUNK_SIZE    = 8  // Size of RIFF chunk header in bytes
	WAV_FMT_CHUNK_SIZE     = 8  // Size of fmt chunk header in bytes
	WAV_DATA_CHUNK_SIZE    = 8  // Size of data chunk header in bytes

	// Binary format
	ENDIAN_LITTLE = true // Use little endian
)

// Helper functions
func getMaxAmplitude(samples []float32) float64 {
	var max float32
	for _, s := range samples {
		if math.Abs(float64(s)) > float64(max) {
			max = float32(math.Abs(float64(s)))
		}
	}
	return float64(max)
}

func writeWAVHeader(f *os.File, numSamples int) {
	// Calculate correct sizes
	bytesPerSample := BITS_PER_SAMPLE / 8 // 16-bit samples
	dataChunkSize := uint32(numSamples) * bytesPerSample
	headerSize := uint32(12 + 8 + 16) // RIFF(8) + fmt(8+16) + data(8) = 36 bytes, size of header chunks
	fileSize := headerSize + dataChunkSize

	// RIFF header
	f.WriteString("RIFF")
	binary.Write(f, binary.LittleEndian, fileSize) // File size
	f.WriteString("WAVE")

	// fmt chunk
	f.WriteString("fmt ")
	binary.Write(f, binary.LittleEndian, uint32(FORMAT_SIZE))   // Chunk size
	binary.Write(f, binary.LittleEndian, uint16(PCM_FORMAT))    // PCM format
	binary.Write(f, binary.LittleEndian, uint16(MONO_CHANNELS)) // Mono
	binary.Write(f, binary.LittleEndian, uint32(SAMPLE_RATE))
	binary.Write(f, binary.LittleEndian, uint32(SAMPLE_RATE*bytesPerSample)) // Bytes/sec
	binary.Write(f, binary.LittleEndian, uint16(bytesPerSample))             // Block align
	binary.Write(f, binary.LittleEndian, uint16(BITS_PER_SAMPLE))            // Bits/sample

	// data chunk
	f.WriteString("data")
	binary.Write(f, binary.LittleEndian, dataChunkSize)
}

func captureAudio(t *testing.T, filename string) []float32 {
	// Get the samples for analysis
	numSamples := int(SAMPLE_RATE * float32(CAPTURE_SHORT_MS) / 1000)
	samples := make([]float32, numSamples)

	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	// For the WAV file, capture 5 seconds
	playbackSamples := int(SAMPLE_RATE * PLAYBACK_SECONDS)
	allSamples := make([]float32, playbackSamples)

	// First copy our analysis samples
	copy(allSamples, samples)

	// Then generate the rest
	for i := numSamples; i < playbackSamples; i++ {
		allSamples[i] = chip.GenerateSample()
	}

	// Write WAV file
	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("Failed to create WAV: %v", err)
	}
	defer f.Close()

	// Use the writeWAVHeader function
	writeWAVHeader(f, playbackSamples)

	// Write samples
	for _, sample := range allSamples {
		//pcm := int16(math.Max(PCM_MIN, math.Min(PCM_MAX, float64(sample*PCM_MAX))))
		pcm := int16(math.Max(float64(MIN_PCM_VALUE), math.Min(float64(MAX_PCM_VALUE), float64(sample)*float64(MAX_PCM_VALUE))))
		binary.Write(f, binary.LittleEndian, pcm)
	}

	// Wait 1 second to let audio chip continue generating samples.
	// The duration of audio heard depends on how long the sleep runs,
	// as the chip continues operating in real-time with its current register values.
	time.Sleep(1 * time.Second)

	return samples
}

// Analysis functions with constants applied
func analyzePWMSegments(samples []float32, numSegments int) []float64 {
	if numSegments <= 0 || len(samples) == 0 {
		return nil
	}

	if numSegments > len(samples) {
		numSegments = len(samples)
	}

	segmentLength := len(samples) / numSegments
	remainder := len(samples) % numSegments
	dutyValues := make([]float64, 0, numSegments)

	start := 0

	for i := 0; i < numSegments; i++ {
		end := start + segmentLength
		if i < remainder {
			end++
		}

		segment := samples[start:end]
		var timeHigh int

		for _, s := range segment {
			if float64(s) > SIGNAL_THRESHOLD {
				timeHigh++
			}
		}

		dutyValues = append(dutyValues, float64(timeHigh)/float64(len(segment)))
		start = end
	}

	return dutyValues
}

func findDutyCycleRange(dutyValues []float64) (float64, float64) {
	if len(dutyValues) == 0 {
		return 0, 0 // Or consider error handling
	}

	min, max := dutyValues[0], dutyValues[0]
	for _, d := range dutyValues {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return min, max
}

func analyseWaveform(t *testing.T, samples []float32) struct {
	frequency float64
	dutyCycle float64
	amplitude float64
} {
	sampleRate := float64(SAMPLE_RATE)
	totalSamples := len(samples)

	// Initialize amplitude tracking using float64 for precision.
	maxAmp := float64(samples[0])
	minAmp := float64(samples[0])

	// Use linear interpolation to determine more precise rising edge times.
	var risingTimes []float64
	prev := float64(samples[0])
	for i := 1; i < totalSamples; i++ {
		cur := float64(samples[i])
		// Update amplitude extrema.
		if cur > maxAmp {
			maxAmp = cur
		}
		if cur < minAmp {
			minAmp = cur
		}
		// Detect rising edge: transition from non-positive to positive.
		if prev <= ZERO_CROSSING_THRESHOLD && cur > ZERO_CROSSING_THRESHOLD {
			// Linear interpolation to estimate the crossing position.
			var fraction float64
			if cur-prev != 0 {
				fraction = -prev / (cur - prev)
			}
			crossing := float64(i-1) + fraction
			risingTimes = append(risingTimes, crossing)
		}
		prev = cur
	}

	// Compute frequency from the average period between rising edges.
	var frequency float64
	if len(risingTimes) > 1 {
		periodSamples := (risingTimes[len(risingTimes)-1] - risingTimes[0]) / float64(len(risingTimes)-1)
		frequency = sampleRate / periodSamples
	} else {
		frequency = 0
	}

	// Compute amplitude (assuming the wave oscillates between -A and A).
	amplitude := (maxAmp - minAmp) / 2

	// Compute duty cycle: fraction of samples greater than zero.
	positiveCount := 0
	for _, s := range samples {
		if s > ZERO_CROSSING_THRESHOLD {
			positiveCount++
		}
	}
	dutyCycle := float64(positiveCount) / float64(totalSamples)

	return struct {
		frequency float64
		dutyCycle float64
		amplitude float64
	}{
		frequency: frequency,
		dutyCycle: dutyCycle,
		amplitude: amplitude,
	}
}

func analyzeSinePurity(samples []float32) float64 {
	if len(samples) < 3 {
		return 0.0
	}

	// Find maximum amplitude
	maxAmplitude := float32(0)
	for _, s := range samples {
		if abs := float32(math.Abs(float64(s))); abs > maxAmplitude {
			maxAmplitude = abs
		}
	}
	if maxAmplitude == 0 {
		return AMPLITUDE_NORMAL // Flat line is considered pure
	}

	minProminence := maxAmplitude * MIN_PEAK_PROMINENCE_FACTOR
	var peaks []float32
	prev := samples[0]

	// Detect peaks with prominence
	for i := 1; i < len(samples)-1; i++ {
		current := samples[i]
		next := samples[i+1]

		isMax := current > prev+minProminence && current > next+minProminence
		isMin := current < prev-minProminence && current < next-minProminence

		if isMax || isMin {
			peaks = append(peaks, float32(math.Abs(float64(current))))
		}
		prev = current
	}

	if len(peaks) < MIN_PEAKS_FOR_ANALYSIS {
		return 0.0
	}

	// Calculate normalized standard deviation
	var sum, sumSquares float64
	for _, p := range peaks {
		sum += float64(p)
		sumSquares += float64(p) * float64(p)
	}

	mean := sum / float64(len(peaks))
	variance := (sumSquares / float64(len(peaks))) - (mean * mean)
	stdDev := math.Sqrt(variance)

	// Normalize purity score (0-1 scale)
	purity := AMPLITUDE_NORMAL - (stdDev / mean)
	if purity < 0 {
		return 0.0
	}
	return math.Max(0.0, math.Min(AMPLITUDE_NORMAL, purity))
}

func calculateHarmonicContent(samples []float32) float64 {
	if len(samples) < SMOOTHING_WINDOW_SIZE {
		return 0.0
	}

	// Calculate fundamental power and harmonic power simultaneously
	var fundamentalPower, harmonicPower float64
	halfWindow := SMOOTHING_WINDOW_SIZE / 2
	smoothed := make([]float64, len(samples))

	// First pass: noise-resistant smoothing
	for i := range samples {
		start := max(0, i-halfWindow)
		end := min(len(samples)-1, i+halfWindow)
		var sum float64
		for j := start; j <= end; j++ {
			sum += float64(samples[j])
		}
		smoothed[i] = sum / float64(end-start+1)
	}

	// Second pass: harmonic component extraction
	for i := halfWindow; i < len(samples)-halfWindow; i++ {
		// Fundamental component (smoothed signal)
		fund := smoothed[i]
		fundamentalPower += fund * fund

		// Harmonic component (remaining signal after smoothing)
		harmonic := float64(samples[i]) - fund
		harmonicPower += harmonic * harmonic
	}

	totalSamples := len(samples) - 2*halfWindow
	if totalSamples < 1 {
		return 0.0
	}

	// Calculate RMS values
	rmsFundamental := math.Sqrt(fundamentalPower / float64(totalSamples))
	rmsHarmonic := math.Sqrt(harmonicPower / float64(totalSamples))

	// Handle edge cases and normalize
	if rmsFundamental < MIN_SIGNAL_AMPLITUDE {
		return 1.0 // All noise/harmonics if no fundamental
	}

	return math.Min(rmsHarmonic/rmsFundamental, 1.0)
}

func analyzeSinePhase(samples []float32) float64 {
	// Find first positive zero crossing
	for i := 1; i < len(samples); i++ {
		if samples[i-1] <= 0 && samples[i] > 0 {
			return float64(i) / float64(len(samples))
		}
	}
	return 0
}

func analyzeEnvelopeShape(samples []float32) string {
	// Simple shape analysis based on amplitude curve
	var deltas []float64
	for i := 1; i < len(samples); i++ {
		deltas = append(deltas, math.Abs(float64(samples[i]-samples[i-1])))
	}

	variance := calculateVariance(deltas)
	if variance > 0.1 {
		return "exponential"
	}
	return "linear"
}

func calculateVariance(values []float64) float64 {
	var sum, sumSq float64
	for _, v := range values {
		sum += v
		sumSq += v * v
	}
	mean := sum / float64(len(values))
	return (sumSq / float64(len(values))) - (mean * mean)
}

func detectModulationFrequency(modulated, unmodulated []float32) float64 {
	// Calculate envelope of differences
	diffs := make([]float32, len(modulated))
	for i := range modulated {
		diffs[i] = modulated[i] - unmodulated[i]
	}

	// Find zero crossings
	crossings := 0
	prev := diffs[0]
	for i := 1; i < len(diffs); i++ {
		if (prev < 0 && diffs[i] >= 0) || (prev >= 0 && diffs[i] < 0) {
			crossings++
		}
		prev = diffs[i]
	}

	return float64(crossings) * SAMPLE_RATE / float64(len(diffs)*4)
}

func calculateModulationDepth(modulated, reference []float32) float64 {
	var maxDiff float64
	for i := range modulated {
		diff := math.Abs(float64(modulated[i] - reference[i]))
		maxDiff = math.Max(maxDiff, diff)
	}
	return maxDiff
}

func detectPeriod(samples []float32) int {
	// Use autocorrelation to find the first strong peak
	for lag := 1; lag < len(samples)/2; lag++ {
		var correlation float64
		for i := 0; i < len(samples)-lag; i++ {
			correlation += float64(samples[i] * samples[i+lag])
		}
		correlation /= float64(len(samples) - lag)

		if correlation > CORRELATION_THRESHOLD {
			return lag
		}
	}
	return 0
}

func findResonancePeak(samples []float32) float64 {
	// Find maximum amplitude in the signal
	var maxAmp float64
	for _, s := range samples {
		maxAmp = math.Max(maxAmp, math.Abs(float64(s)))
	}
	return maxAmp
}

func calculateRMS(samples []float32) float64 {
	if len(samples) == 0 {
		return 0.0 // Handle empty input to avoid division by zero
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s) // Convert before squaring for precision
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func measureOddHarmonics(samples []float32) float64 {
	var sum float64
	for i := 2; i < len(samples)-2; i++ {
		// Three-point analysis for odd harmonics
		prev := float64(samples[i-1])
		curr := float64(samples[i])
		next := float64(samples[i+1])

		// Detect odd symmetry
		oddComponent := math.Abs(next + prev - 2*curr)
		sum += oddComponent
	}
	return sum / float64(len(samples)-4)
}

func measureEvenHarmonics(samples []float32) float64 {
	var sum float64
	for i := 2; i < len(samples)-2; i++ {
		// Three-point analysis for even harmonics
		prev := float64(samples[i-1])
		next := float64(samples[i+1])

		// Detect even symmetry
		evenComponent := math.Abs(next - prev)
		sum += evenComponent
	}
	return sum / float64(len(samples)-4)
}

func measureDecayTime(samples []float32) float64 {

	// Find peak amplitude
	var peak float32
	for _, s := range samples {
		if math.Abs(float64(s)) > float64(peak) {
			peak = float32(math.Abs(float64(s)))
		}
	}

	// Find time to reach -60dB
	threshold := peak * float32(math.Pow(10, DECAY_THRESHOLD/20))
	for i, s := range samples {
		if math.Abs(float64(s)) < float64(threshold) {
			return float64(i) / SAMPLE_RATE
		}
	}
	return float64(len(samples)) / SAMPLE_RATE
}

//////////////////////////////////////////

// Store metrics between captures
var testMetrics sync.Map

// RegisterWrite describes a single write to a hardware register.
type RegisterWrite struct {
	Register uint32
	Value    uint32
}

// ExpectedMetric defines the expected value and tolerance for a given metric.
type ExpectedMetric struct {
	Expected  float64
	Tolerance float64
}

// AudioTestCase is the unified configuration for an audio test.
type AudioTestCase struct {
	// Name of the test case.
	Name string

	// Optional function to run before applying the configuration.
	PreSetup func()

	// List of register writes to configure the audio chip.
	Config []RegisterWrite

	// Optional additional setup (for example, resetting phases or configuring multiple channels).
	AdditionalSetup func()

	// Duration for capturing audio.
	CaptureDuration time.Duration

	// Filename to which audio will be written.
	Filename string

	// New field to override capture behavior when needed.
	CaptureFunc func(t *testing.T, filename string, duration time.Duration) []float32

	// Function to analyze the captured samples.
	// It should return a map where keys are metric names (e.g. "frequency", "amplitude", "dutyCycle").
	AnalyzeFunc func(samples []float32) map[string]float64

	// Expected metrics with tolerances.
	Expected map[string]ExpectedMetric

	// Optional custom validation function.
	Validate func(t *testing.T, metrics map[string]float64)

	// Optional cleanup function to run after the test.
	PostCleanup func()

	// New field for storing intermediate data
	Data map[string]interface{} // For storing any test-specific data
}

func defaultAnalyze(samples []float32) map[string]float64 {
	res := analyseWaveform(nil, samples)
	return map[string]float64{
		"frequency": res.frequency,
		"amplitude": res.amplitude,
		"dutyCycle": res.dutyCycle,
	}
}

func defaultValidate(t *testing.T, expected map[string]ExpectedMetric, actual map[string]float64) {
	for key, exp := range expected {
		act, ok := actual[key]
		if !ok {
			t.Errorf("Metric %s not computed", key)
			continue
		}
		if math.Abs(act-exp.Expected) > exp.Tolerance {
			t.Errorf("%s error: got %.2f, expected %.2f (tolerance %.2f)", key, act, exp.Expected, exp.Tolerance)
		}
	}
}

// runAudioTest is the unified test runner.
func runAudioTest(t *testing.T, tc AudioTestCase) {
	// Optional pre-setup.
	if tc.PreSetup != nil {
		tc.PreSetup()
	}

	// Apply register writes.
	for _, reg := range tc.Config {
		chip.HandleRegisterWrite(reg.Register, reg.Value)
	}

	// Additional setup if needed.
	if tc.AdditionalSetup != nil {
		tc.AdditionalSetup()
	}

	// Capture audio (using your existing captureAudio function).
	samples := captureAudio(t, tc.Filename)

	// Analyze captured samples.
	var metrics map[string]float64
	if tc.AnalyzeFunc != nil {
		metrics = tc.AnalyzeFunc(samples)
	} else {
		metrics = defaultAnalyze(samples)
	}

	// Validate results.
	if tc.Validate != nil {
		tc.Validate(t, metrics)
	} else {
		defaultValidate(t, tc.Expected, metrics)
	}

	// Post-test cleanup.
	if tc.PostCleanup != nil {
		tc.PostCleanup()
	}
}

//////////////////////////////////////////

// Square wave tests

var squareWaveBasicTests = []AudioTestCase{
	{
		Name: "C4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, DISABLED)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: C4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: HALF_DUTY_CYCLE},
			{Register: SQUARE_CTRL, Value: REGISTER_ENABLE},
		},
		CaptureDuration: CAPTURE_SHORT_MS * MILLISECOND,
		Filename:        "square_wave_basic_C4.wav",
		AnalyzeFunc:     nil, // Use defaultAnalyze
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(C4_FREQUENCY), Tolerance: float64(C4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			"dutyCycle": {Expected: DUTY_CYCLE_NORMAL, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil, // Use defaultValidate
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, DISABLED)
		},
	},
	{
		Name: "E4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "square_wave_basic_E4.wav",
		AnalyzeFunc:     nil,
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(E4_FREQUENCY), Tolerance: float64(E4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			"dutyCycle": {Expected: DUTY_CYCLE_NORMAL, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "G4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "square_wave_basic_G4.wav",
		AnalyzeFunc:     nil,
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(E4_FREQUENCY), Tolerance: float64(E4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			"dutyCycle": {Expected: DUTY_CYCLE_NORMAL, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "A4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "square_wave_basic_A4.wav",
		AnalyzeFunc:     nil,
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(E4_FREQUENCY), Tolerance: float64(E4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			"dutyCycle": {Expected: DUTY_CYCLE_NORMAL, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
}

func TestSquareWaveBasic(t *testing.T) {
	for _, tc := range squareWaveBasicTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var squareWaveDutyCycleTests = []AudioTestCase{
	{
		Name: "Eighth",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Set duty cycle to 32 (expected normalized duty cycle 32/256 = 0.125)
			{Register: SQUARE_DUTY, Value: MIN_DUTY_CYCLE + (MAX_DUTY_CYCLE-MIN_DUTY_CYCLE)/8}, // 1/8 duty cycle
			// Enable the square wave channel.
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "square_wave_duty_Eighth.wav",
		AnalyzeFunc:     nil, // Use defaultAnalyze (which calls analyseWaveform)
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			// Expected duty cycle normalized to 0.125
			"dutyCycle": {Expected: 0.125, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil, // Use default validation.
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "Quarter",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Set duty cycle to 64 (expected normalized duty cycle 64/256 = 0.25)
			{Register: SQUARE_DUTY, Value: 64},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "square_wave_duty_Quarter.wav",
		AnalyzeFunc:     nil,
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			// Expected duty cycle normalized to 0.25
			"dutyCycle": {Expected: 0.25, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "Three Quarter",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Set duty cycle to 192 (expected normalized duty cycle 192/256 = 0.75)
			{Register: SQUARE_DUTY, Value: 192},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "square_wave_duty_ThreeQuarter.wav",
		AnalyzeFunc:     nil,
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
			// Expected duty cycle normalized to 0.75
			"dutyCycle": {Expected: 0.75, Tolerance: DUTY_TOLERANCE},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
}

func TestSquareWaveDutyCycle(t *testing.T) {
	for _, tc := range squareWaveDutyCycleTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var squareWavePWMTests = []AudioTestCase{
	{
		Name: "Slow Shallow",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, DISABLED)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Compute pwmDuty as (pwmDepth << 8) | 128, here pwmDepth is 32
			{Register: SQUARE_DUTY, Value: (32 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: PWM_ENABLE_BIT | PWM_RATE_SLOW}, // pwmRate = 0x10
			{Register: SQUARE_CTRL, Value: REGISTER_ENABLE},
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_SlowShallow.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			segments := analyzePWMSegments(samples, 10)
			minDuty, maxDuty := findDutyCycleRange(segments)
			pwmDepth := (maxDuty - minDuty) / 2
			return map[string]float64{
				"pwmDepth": pwmDepth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"pwmDepth": {Expected: float64(32) / 512.0, Tolerance: 0.1},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, DISABLED)
		},
	},
	{
		Name: "Fast Deep",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, DISABLED)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: (192 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x70}, // pwmRate = 0x70
			{Register: SQUARE_CTRL, Value: REGISTER_ENABLE},
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_FastDeep.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			segments := analyzePWMSegments(samples, 10)
			minDuty, maxDuty := findDutyCycleRange(segments)
			pwmDepth := (maxDuty - minDuty) / 2
			return map[string]float64{
				"pwmDepth": pwmDepth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"pwmDepth": {Expected: float64(192) / 512.0, Tolerance: 0.1},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, DISABLED)
		},
	},
	{
		Name: "Mid Range",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: (128 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x30}, // pwmRate = 0x30
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_MidRange.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			segments := analyzePWMSegments(samples, 10)
			minDuty, maxDuty := findDutyCycleRange(segments)
			pwmDepth := (maxDuty - minDuty) / 2
			return map[string]float64{
				"pwmDepth": pwmDepth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"pwmDepth": {Expected: float64(128) / 512.0, Tolerance: 0.1},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "Low Freq",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A2_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: (128 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x20}, // pwmRate = 0x20
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_LowFreq.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			segments := analyzePWMSegments(samples, 10)
			minDuty, maxDuty := findDutyCycleRange(segments)
			pwmDepth := (maxDuty - minDuty) / 2
			return map[string]float64{
				"pwmDepth": pwmDepth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"pwmDepth": {Expected: float64(128) / 512.0, Tolerance: 0.1},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "High Freq",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: 1760},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: (64 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x40}, // pwmRate = 0x40
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_HighFreq.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			segments := analyzePWMSegments(samples, 10)
			minDuty, maxDuty := findDutyCycleRange(segments)
			pwmDepth := (maxDuty - minDuty) / 2
			return map[string]float64{
				"pwmDepth": pwmDepth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"pwmDepth": {Expected: float64(64) / 512.0, Tolerance: 0.1},
		},
		Validate: nil,
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
}

func TestSquareWavePWM(t *testing.T) {
	for _, tc := range squareWavePWMTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var squareWaveSweepTests = []AudioTestCase{
	{
		Name: "Up Slow",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			// Upward sweep: period=0x03, shift=2, upward true so set dirBit = 0x08.
			{Register: SQUARE_SWEEP, Value: SWEEP_ENABLE_BIT | (SWEEP_PERIOD_SLOW << SWEEP_PERIOD_SHIFT) | SWEEP_DIRECTION_UP | SWEEP_SHIFT_MEDIUM},
			{Register: SQUARE_CTRL, Value: 3},
		},
		// Capture for 500ms.
		CaptureDuration: 500 * time.Millisecond,
		Filename:        "sweep_UpSlow.wav",
		// Custom capture function that collects sampleCount samples in real time.
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)
			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  backResult.frequency - frontResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]
			// For upward sweep, final frequency must exceed the initial frequency by at least 50 Hz.
			if backFreq <= frontFreq {
				t.Errorf("Upward sweep failed: final frequency %.1f not greater than initial %.1f Hz", backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Upward sweep overall increase too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_SWEEP, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "Down Fast",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: 1760},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			// Downward sweep: period=0x01, shift=3, upward false (omit 0x08).
			{Register: SQUARE_SWEEP, Value: SWEEP_ENABLE_BIT | (SWEEP_PERIOD_SLOW << SWEEP_PERIOD_SHIFT) | SWEEP_DIRECTION_UP | SWEEP_SHIFT_MEDIUM},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: 300 * time.Millisecond,
		Filename:        "sweep_DownFast.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)
			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  frontResult.frequency - backResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]
			// For downward sweep, final frequency must be lower than the initial frequency by at least 50 Hz.
			if backFreq >= frontFreq {
				t.Errorf("Downward sweep failed: final frequency %.1f not lower than initial %.1f Hz", backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Downward sweep overall drop too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_SWEEP, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "Up Wide",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A2_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			// Upward sweep: period=0x02, shift=4, upward true.
			{Register: SQUARE_SWEEP, Value: 0x80 | (0x02 << 4) | 0x08 | 4},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: 400 * time.Millisecond,
		Filename:        "sweep_UpWide.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)
			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  backResult.frequency - frontResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]
			if backFreq <= frontFreq {
				t.Errorf("Upward sweep failed: final frequency %.1f not greater than initial %.1f Hz", backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Upward sweep overall increase too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_SWEEP, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
	{
		Name: "Down Narrow",
		PreSetup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A5_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			// Downward sweep: period=0x02, shift=1, upward false.
			{Register: SQUARE_SWEEP, Value: 0x80 | (0x02 << 4) | 1},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: 400 * time.Millisecond,
		Filename:        "sweep_DownNarrow.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)
			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  frontResult.frequency - backResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]
			if backFreq >= frontFreq {
				t.Errorf("Downward sweep failed: final frequency %.1f not lower than initial %.1f Hz", backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Downward sweep overall drop too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_SWEEP, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
		},
	},
}

func TestSquareWaveSweep(t *testing.T) {
	for _, tc := range squareWaveSweepTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var squareWaveSyncTests = []AudioTestCase{
	{
		Name: "Octave",
		PreSetup: func() {
			// Disable all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: E4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Configure slave oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Set up hard sync: assign channel 0 as the sync source for channel 1
			{Register: SYNC_SOURCE_CH1, Value: 0},

			// Enable both channels
			{Register: SQUARE_CTRL, Value: 3},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_Octave.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Calculate the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(E4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
	},
	{
		Name: "Fifth",
		PreSetup: func() {
			// Disable all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Configure slave oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: 660},
			{Register: TRI_VOL, Value: 255},

			// Set up hard sync: assign channel 0 as the sync source for channel 1
			{Register: SYNC_SOURCE_CH1, Value: 0},

			// Enable both channels
			{Register: SQUARE_CTRL, Value: 3},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_Fifth.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Calculate the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
	},
	{
		Name: "Third",
		PreSetup: func() {
			// Disable all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Configure slave oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: 550},
			{Register: TRI_VOL, Value: 255},

			// Set up hard sync: assign channel 0 as the sync source for channel 1
			{Register: SYNC_SOURCE_CH1, Value: 0},

			// Enable both channels
			{Register: SQUARE_CTRL, Value: 3},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_Third.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Calculate the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
	},
	{
		Name: "SubOctave",
		PreSetup: func() {
			// Disable all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: A5_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Configure slave oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Set up hard sync: assign channel 0 as the sync source for channel 1
			{Register: SYNC_SOURCE_CH1, Value: 0},

			// Enable both channels
			{Register: SQUARE_CTRL, Value: 3},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_SubOctave.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Calculate the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A5_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		},
	},
}

func TestSquareWaveSync(t *testing.T) {
	for _, tc := range squareWaveSyncTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var squareWavePWMOverdriveTests = []AudioTestCase{
	{
		Name: "Normal PWM",
		PreSetup: func() {
			// Disable square channel before configuration
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Base duty cycle of 50% with PWM depth of 128
			{Register: SQUARE_DUTY, Value: (128 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x30}, // pwmRate = 0x30
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_clean_Normal_PWM.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze clean duty cycle variation
			segments := analyzePWMSegments(samples, 10)
			cleanMinDuty, cleanMaxDuty := findDutyCycleRange(segments)
			cleanHarmonics := calculateHarmonicContent(samples)
			cleanRMS := calculateRMS(samples)

			return map[string]float64{
				"cleanMinDuty":    cleanMinDuty,
				"cleanMaxDuty":    cleanMaxDuty,
				"cleanHarmonics":  cleanHarmonics,
				"cleanRMS":        cleanRMS,
				"drivenMinDuty":   cleanMinDuty,   // Same as clean since no overdrive
				"drivenMaxDuty":   cleanMaxDuty,   // Same as clean since no overdrive
				"drivenHarmonics": cleanHarmonics, // Same as clean since no overdrive
				"drivenRMS":       cleanRMS,       // Same as clean since no overdrive
				"minDutyChange":   0.0,
				"maxDutyChange":   0.0,
				"harmonicRatio":   1.0,
				"rmsRatio":        1.0,
			}
		},
		Expected: map[string]ExpectedMetric{
			"minDutyChange": {Expected: 0.0, Tolerance: 0.1},
			"maxDutyChange": {Expected: 0.0, Tolerance: 0.1},
			"harmonicRatio": {Expected: 1.0, Tolerance: 0.1},
			"rmsRatio":      {Expected: 1.0, Tolerance: 0.1},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},
	{
		Name: "Light Overdrive",
		PreSetup: func() {
			// Disable square channel before configuration
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Base duty cycle of 50% with PWM depth of 128
			{Register: SQUARE_DUTY, Value: (128 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x30}, // pwmRate = 0x30
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture first sample without overdrive
			cleanSamples := captureAudio(nil, "square_pwm_clean_Light_Overdrive.wav")

			// Analyze clean duty cycle variation
			cleanSegments := analyzePWMSegments(cleanSamples, 10)
			cleanMinDuty, cleanMaxDuty := findDutyCycleRange(cleanSegments)
			cleanHarmonics := calculateHarmonicContent(cleanSamples)
			cleanRMS := calculateRMS(cleanSamples)

			// Store these values somewhere to be accessed by AnalyzeFunc
			// For example, you might have a global or package-level map
			testMetrics.Store("Light Overdrive", map[string]float64{
				"cleanMinDuty":   cleanMinDuty,
				"cleanMaxDuty":   cleanMaxDuty,
				"cleanHarmonics": cleanHarmonics,
				"cleanRMS":       cleanRMS,
			})

			// Now enable overdrive for the main test capture
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 128)
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_overdrive_Light_Overdrive.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze overdriven duty cycle variation
			drivenSegments := analyzePWMSegments(samples, 10)
			drivenMinDuty, drivenMaxDuty := findDutyCycleRange(drivenSegments)
			drivenHarmonics := calculateHarmonicContent(samples)
			drivenRMS := calculateRMS(samples)

			// Retrieve clean metrics from the additional setup
			cleanMetricsMap, _ := testMetrics.Load("Light Overdrive")
			cleanMap := cleanMetricsMap.(map[string]float64)

			// Calculate changes
			minDutyChange := drivenMinDuty - cleanMap["cleanMinDuty"]
			maxDutyChange := drivenMaxDuty - cleanMap["cleanMaxDuty"]
			harmonicRatio := drivenHarmonics / cleanMap["cleanHarmonics"]
			rmsRatio := drivenRMS / cleanMap["cleanRMS"]

			return map[string]float64{
				"cleanMinDuty":    cleanMap["cleanMinDuty"],
				"cleanMaxDuty":    cleanMap["cleanMaxDuty"],
				"cleanHarmonics":  cleanMap["cleanHarmonics"],
				"cleanRMS":        cleanMap["cleanRMS"],
				"drivenMinDuty":   drivenMinDuty,
				"drivenMaxDuty":   drivenMaxDuty,
				"drivenHarmonics": drivenHarmonics,
				"drivenRMS":       drivenRMS,
				"minDutyChange":   minDutyChange,
				"maxDutyChange":   maxDutyChange,
				"harmonicRatio":   harmonicRatio,
				"rmsRatio":        rmsRatio,
			}
		},
		Expected: map[string]ExpectedMetric{
			"minDutyChange": {Expected: 0.05, Tolerance: 0.1},
			"maxDutyChange": {Expected: -0.05, Tolerance: 0.1},
			"harmonicRatio": {Expected: 1.2, Tolerance: 0.2},
			"rmsRatio":      {Expected: 1.1, Tolerance: 0.2},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
			testMetrics.Delete("Light Overdrive")
		},
	},
	{
		Name: "Heavy Overdrive",
		PreSetup: func() {
			// Disable square channel before configuration
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Base duty cycle of 50% with PWM depth of 128
			{Register: SQUARE_DUTY, Value: (128 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x30}, // pwmRate = 0x30
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture first sample without overdrive
			cleanSamples := captureAudio(nil, "square_pwm_clean_Heavy_Overdrive.wav")

			// Analyze clean duty cycle variation
			cleanSegments := analyzePWMSegments(cleanSamples, 10)
			cleanMinDuty, cleanMaxDuty := findDutyCycleRange(cleanSegments)
			cleanHarmonics := calculateHarmonicContent(cleanSamples)
			cleanRMS := calculateRMS(cleanSamples)

			testMetrics.Store("Heavy Overdrive", map[string]float64{
				"cleanMinDuty":   cleanMinDuty,
				"cleanMaxDuty":   cleanMaxDuty,
				"cleanHarmonics": cleanHarmonics,
				"cleanRMS":       cleanRMS,
			})

			// Now enable overdrive for the main test capture
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_overdrive_Heavy_Overdrive.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze overdriven duty cycle variation
			drivenSegments := analyzePWMSegments(samples, 10)
			drivenMinDuty, drivenMaxDuty := findDutyCycleRange(drivenSegments)
			drivenHarmonics := calculateHarmonicContent(samples)
			drivenRMS := calculateRMS(samples)

			// Retrieve clean metrics
			cleanMetricsMap, _ := testMetrics.Load("Heavy Overdrive")
			cleanMap := cleanMetricsMap.(map[string]float64)

			// Calculate changes
			minDutyChange := drivenMinDuty - cleanMap["cleanMinDuty"]
			maxDutyChange := drivenMaxDuty - cleanMap["cleanMaxDuty"]
			harmonicRatio := drivenHarmonics / cleanMap["cleanHarmonics"]
			rmsRatio := drivenRMS / cleanMap["cleanRMS"]

			return map[string]float64{
				"minDutyChange": minDutyChange,
				"maxDutyChange": maxDutyChange,
				"harmonicRatio": harmonicRatio,
				"rmsRatio":      rmsRatio,
			}
		},
		Expected: map[string]ExpectedMetric{
			"minDutyChange": {Expected: 0.1, Tolerance: 0.1},
			"maxDutyChange": {Expected: -0.1, Tolerance: 0.1},
			"harmonicRatio": {Expected: 1.5, Tolerance: 0.2},
			"rmsRatio":      {Expected: 1.2, Tolerance: 0.2},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
			testMetrics.Delete("Heavy Overdrive")
		},
	},
	{
		Name: "Fast PWM Overdrive",
		PreSetup: func() {
			// Disable square channel before configuration
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Base duty cycle of 50% with PWM depth of 64
			{Register: SQUARE_DUTY, Value: (64 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x70}, // pwmRate = 0x70
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture first sample without overdrive
			cleanSamples := captureAudio(nil, "square_pwm_clean_Fast_PWM_Overdrive.wav")

			// Analyze clean duty cycle variation
			cleanSegments := analyzePWMSegments(cleanSamples, 10)
			cleanMinDuty, cleanMaxDuty := findDutyCycleRange(cleanSegments)
			cleanHarmonics := calculateHarmonicContent(cleanSamples)
			cleanRMS := calculateRMS(cleanSamples)

			testMetrics.Store("Fast PWM Overdrive", map[string]float64{
				"cleanMinDuty":   cleanMinDuty,
				"cleanMaxDuty":   cleanMaxDuty,
				"cleanHarmonics": cleanHarmonics,
				"cleanRMS":       cleanRMS,
			})

			// Now enable overdrive for the main test capture
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 192)
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_overdrive_Fast_PWM_Overdrive.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze overdriven duty cycle variation
			drivenSegments := analyzePWMSegments(samples, 10)
			drivenMinDuty, drivenMaxDuty := findDutyCycleRange(drivenSegments)
			drivenHarmonics := calculateHarmonicContent(samples)
			drivenRMS := calculateRMS(samples)

			// Retrieve clean metrics
			cleanMetricsMap, _ := testMetrics.Load("Fast PWM Overdrive")
			cleanMap := cleanMetricsMap.(map[string]float64)

			// Calculate changes
			minDutyChange := drivenMinDuty - cleanMap["cleanMinDuty"]
			maxDutyChange := drivenMaxDuty - cleanMap["cleanMaxDuty"]
			harmonicRatio := drivenHarmonics / cleanMap["cleanHarmonics"]
			rmsRatio := drivenRMS / cleanMap["cleanRMS"]

			return map[string]float64{
				"minDutyChange": minDutyChange,
				"maxDutyChange": maxDutyChange,
				"harmonicRatio": harmonicRatio,
				"rmsRatio":      rmsRatio,
			}
		},
		Expected: map[string]ExpectedMetric{
			"minDutyChange": {Expected: 0.08, Tolerance: 0.1},
			"maxDutyChange": {Expected: -0.08, Tolerance: 0.1},
			"harmonicRatio": {Expected: 1.3, Tolerance: 0.2},
			"rmsRatio":      {Expected: 1.15, Tolerance: 0.2},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
			testMetrics.Delete("Fast PWM Overdrive")
		},
	},
	{
		Name: "Deep PWM Overdrive",
		PreSetup: func() {
			// Disable square channel before configuration
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			// Base duty cycle of 50% with PWM depth of 192
			{Register: SQUARE_DUTY, Value: (192 << 8) | 128},
			{Register: SQUARE_PWM_CTRL, Value: 0x80 | 0x20}, // pwmRate = 0x20
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture first sample without overdrive
			cleanSamples := captureAudio(nil, "square_pwm_clean_Deep_PWM_Overdrive.wav")

			// Analyze clean duty cycle variation
			cleanSegments := analyzePWMSegments(cleanSamples, 10)
			cleanMinDuty, cleanMaxDuty := findDutyCycleRange(cleanSegments)
			cleanHarmonics := calculateHarmonicContent(cleanSamples)
			cleanRMS := calculateRMS(cleanSamples)

			testMetrics.Store("Deep PWM Overdrive", map[string]float64{
				"cleanMinDuty":   cleanMinDuty,
				"cleanMaxDuty":   cleanMaxDuty,
				"cleanHarmonics": cleanHarmonics,
				"cleanRMS":       cleanRMS,
			})

			// Now enable overdrive for the main test capture
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 192)
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "square_pwm_overdrive_Deep_PWM_Overdrive.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze overdriven duty cycle variation
			drivenSegments := analyzePWMSegments(samples, 10)
			drivenMinDuty, drivenMaxDuty := findDutyCycleRange(drivenSegments)
			drivenHarmonics := calculateHarmonicContent(samples)
			drivenRMS := calculateRMS(samples)

			// Retrieve clean metrics
			cleanMetricsMap, _ := testMetrics.Load("Deep PWM Overdrive")
			cleanMap := cleanMetricsMap.(map[string]float64)

			// Calculate changes
			minDutyChange := drivenMinDuty - cleanMap["cleanMinDuty"]
			maxDutyChange := drivenMaxDuty - cleanMap["cleanMaxDuty"]
			harmonicRatio := drivenHarmonics / cleanMap["cleanHarmonics"]
			rmsRatio := drivenRMS / cleanMap["cleanRMS"]

			return map[string]float64{
				"minDutyChange": minDutyChange,
				"maxDutyChange": maxDutyChange,
				"harmonicRatio": harmonicRatio,
				"rmsRatio":      rmsRatio,
			}
		},
		Expected: map[string]ExpectedMetric{
			"minDutyChange": {Expected: 0.1, Tolerance: 0.1},
			"maxDutyChange": {Expected: -0.1, Tolerance: 0.1},
			"harmonicRatio": {Expected: 1.4, Tolerance: 0.2},
			"rmsRatio":      {Expected: 1.2, Tolerance: 0.2},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
			testMetrics.Delete("Deep PWM Overdrive")
		},
	},
}

func TestSquareWavePWMOverdrive(t *testing.T) {
	for _, tc := range squareWavePWMOverdriveTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var neurobassTests = []AudioTestCase{
	{
		Name: "SubtectonicRipper",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 40)
			chip.HandleRegisterWrite(SINE_VOL, 255)

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float64(40)+2.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, 38)
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x03)

			// Enable oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_SubtectonicRipper.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_SubtectonicRipper.wav", i))
				filthySamples = append(filthySamples, segment...)
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_SubtectonicRipper.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_SubtectonicRipper.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("SubtectonicRipper: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},
	{
		Name: "WarningBassAttack",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 65)
			chip.HandleRegisterWrite(SINE_VOL, 255)

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, 68)
			chip.HandleRegisterWrite(TRI_VOL, 255)

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, 62)
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x05)

			// Enable oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_WarningBassAttack.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_WarningBassAttack.wav", i))
				filthySamples = append(filthySamples, segment...)
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_WarningBassAttack.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_WarningBassAttack.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("WarningBassAttack: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},
	{
		Name: "NeuralburnCrusher",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 55)
			chip.HandleRegisterWrite(SINE_VOL, 255)

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float64(55)+5.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, 50)
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x09)

			// Enable oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_NeuralburnCrusher.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_NeuralburnCrusher.wav", i))
				filthySamples = append(filthySamples, segment...)
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_NeuralburnCrusher.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_NeuralburnCrusher.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("NeuralburnCrusher: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},
	{
		Name: "InfrasonicDemolisher",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 30)
			chip.HandleRegisterWrite(SINE_VOL, 255)
			chip.HandleRegisterWrite(SINE_SWEEP, 0x34) // Downward, medium-fast

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float32(30)+7.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_SWEEP, 0x74) // Upward, medium-fast

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, 24)
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x02)

			// Add noise with extreme settings
			chip.HandleRegisterWrite(NOISE_FREQ, 30/3)
			chip.HandleRegisterWrite(NOISE_VOL, 200)

			// Enable all oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_InfrasonicDemolisher.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_InfrasonicDemolisher.wav", i))
				filthySamples = append(filthySamples, segment...)

				// Modulate between segments (extra features enabled)
				if i < 2 {
					newDetune1 := uint32(float32(30) + 7.0 + float32(i*3))
					newDetune2 := uint32(float32(30) - 5.5 - float32(i*4))
					if newDetune2 < 20 {
						newDetune2 = 20
					}
					chip.HandleRegisterWrite(TRI_FREQ, newDetune1)
					chip.HandleRegisterWrite(SQUARE_FREQ, newDetune2)
					// Shift LFO rate
					chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|(0x02+uint32(i*3)))
				}
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_InfrasonicDemolisher.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_InfrasonicDemolisher.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("InfrasonicDemolisher: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},
	{
		Name: "SkullCrusher",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 45)
			chip.HandleRegisterWrite(SINE_VOL, 255)
			chip.HandleRegisterWrite(SINE_SWEEP, 0x74) // Upward, medium-fast

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float32(45)+9.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_SWEEP, 0x34) // Downward, medium-fast

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, 37)
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x01)

			// Add noise with extreme settings
			chip.HandleRegisterWrite(NOISE_FREQ, 45/3)
			chip.HandleRegisterWrite(NOISE_VOL, 200)

			// Enable all oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_SkullCrusher.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_SkullCrusher.wav", i))
				filthySamples = append(filthySamples, segment...)

				// Modulate between segments (extra features enabled)
				if i < 2 {
					newDetune1 := uint32(float32(45) + 9.0 + float32(i*3))
					newDetune2 := uint32(float32(45) - 7.5 - float32(i*4))
					if newDetune2 < 20 {
						newDetune2 = 20
					}
					chip.HandleRegisterWrite(TRI_FREQ, newDetune1)
					chip.HandleRegisterWrite(SQUARE_FREQ, newDetune2)
					// Shift LFO rate
					chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|(0x01+uint32(i*3)))
				}
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_SkullCrusher.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_SkullCrusher.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			// Same analysis code as previous tests
			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("SkullCrusher: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},

	// SpeakerShredder
	{
		Name: "SpeakerShredder",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 80)
			chip.HandleRegisterWrite(SINE_VOL, 255)
			chip.HandleRegisterWrite(SINE_SWEEP, 0x34) // Downward, medium-fast

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float32(80)+12.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_SWEEP, 0x74) // Upward, medium-fast

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, uint32(float32(80)-9.0))
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x0F)

			// Add noise with extreme settings
			chip.HandleRegisterWrite(NOISE_FREQ, 80/3)
			chip.HandleRegisterWrite(NOISE_VOL, 200)

			// Enable all oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_SpeakerShredder.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_SpeakerShredder.wav", i))
				filthySamples = append(filthySamples, segment...)

				// Modulate between segments
				if i < 2 {
					newDetune1 := uint32(float32(80) + 12.0 + float32(i*3))
					newDetune2 := uint32(float32(80) - 9.0 - float32(i*4))
					if newDetune2 < 20 {
						newDetune2 = 20
					}
					chip.HandleRegisterWrite(TRI_FREQ, newDetune1)
					chip.HandleRegisterWrite(SQUARE_FREQ, newDetune2)
					// Shift LFO rate
					chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|(0x0F+uint32(i*3)))
				}
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_SpeakerShredder.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_SpeakerShredder.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Same analysis as previous test cases
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("SpeakerShredder: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},

	// HydrogenBombSub
	{
		Name: "HydrogenBombSub",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 25)
			chip.HandleRegisterWrite(SINE_VOL, 255)
			chip.HandleRegisterWrite(SINE_SWEEP, 0x34) // Downward, medium-fast

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float32(25)+3.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_SWEEP, 0x74) // Upward, medium-fast

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, 22)
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x08)

			// Add noise with extreme settings
			chip.HandleRegisterWrite(NOISE_FREQ, 25/3)
			chip.HandleRegisterWrite(NOISE_VOL, 200)

			// Enable all oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_HydrogenBombSub.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_HydrogenBombSub.wav", i))
				filthySamples = append(filthySamples, segment...)

				// Modulate between segments
				if i < 2 {
					newDetune1 := uint32(float32(25) + 3.0 + float32(i*3))
					newDetune2 := uint32(float32(25) - 2.5 - float32(i*4))
					if newDetune2 < 20 {
						newDetune2 = 20
					}
					chip.HandleRegisterWrite(TRI_FREQ, newDetune1)
					chip.HandleRegisterWrite(SQUARE_FREQ, newDetune2)
					// Shift LFO rate
					chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|(0x08+uint32(i*3)))
				}
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_HydrogenBombSub.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_HydrogenBombSub.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Same analysis as previous test cases
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("HydrogenBombSub: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},

	// BrainMelter
	{
		Name: "BrainMelter",
		PreSetup: func() {
			// Reset all channels
			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)

			// Enable audio
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// First oscillator - Sine at base frequency
			chip.HandleRegisterWrite(SINE_FREQ, 60)
			chip.HandleRegisterWrite(SINE_VOL, 255)
			chip.HandleRegisterWrite(SINE_SWEEP, 0x74) // Upward, medium-fast

			// Second oscillator - Triangle detuned up
			chip.HandleRegisterWrite(TRI_FREQ, uint32(float32(60)+15.0))
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_SWEEP, 0x34) // Downward, medium-fast

			// Third oscillator - Square detuned down
			chip.HandleRegisterWrite(SQUARE_FREQ, uint32(float32(60)-11.0))
			chip.HandleRegisterWrite(SQUARE_VOL, 255)
			chip.HandleRegisterWrite(SQUARE_DUTY, 32)
			chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x1F)

			// Add noise with extreme settings
			chip.HandleRegisterWrite(NOISE_FREQ, 60/3)
			chip.HandleRegisterWrite(NOISE_VOL, 200)

			// Enable all oscillators
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)

			// Capture reference clean sample
			_ = captureAudio(nil, "neurobass_clean_BrainMelter.wav")

			// Apply maximum overdrive
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 255)
		},
		Config: []RegisterWrite{}, // Config already applied in PreSetup
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			var filthySamples []float32

			for i := 0; i < 3; i++ {
				segment := captureAudio(nil, fmt.Sprintf("neurobass_segment%d_BrainMelter.wav", i))
				filthySamples = append(filthySamples, segment...)

				// Modulate between segments
				if i < 2 {
					newDetune1 := uint32(float32(60) + 15.0 + float32(i*3))
					newDetune2 := uint32(float32(60) - 11.0 - float32(i*4))
					if newDetune2 < 20 {
						newDetune2 = 20
					}
					chip.HandleRegisterWrite(TRI_FREQ, newDetune1)
					chip.HandleRegisterWrite(SQUARE_FREQ, newDetune2)
					// Shift LFO rate
					chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|(0x1F+uint32(i*3)))
				}
			}

			// Write combined file
			f, err := os.Create("neurobass_combined_BrainMelter.wav")
			if err != nil {
				t.Fatalf("Failed to create combined WAV: %v", err)
			}
			defer f.Close()
			writeWAVHeader(f, len(filthySamples))
			for _, sample := range filthySamples {
				pcm := int16(math.Max(-32768, math.Min(32767, float64(sample*32767))))
				binary.Write(f, binary.LittleEndian, pcm)
			}

			return filthySamples
		},
		CaptureDuration: CAPTURE_LONG_MS * time.Millisecond,
		Filename:        "neurobass_filthy_BrainMelter.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Same analysis as previous test cases
			filthyRMS := calculateRMS(samples)
			filthyHarmonics := calculateHarmonicContent(samples)
			evenHarmonics := measureEvenHarmonics(samples)
			oddHarmonics := measureOddHarmonics(samples)

			phaseCancellation := 0.0
			for i := 0; i < len(samples)-1; i++ {
				diff := math.Abs(float64(samples[i+1] - samples[i]))
				if diff > 0.5 {
					phaseCancellation += diff
				}
			}
			phaseCancellation /= float64(len(samples))

			subBassEnergy := 0.0
			for i := 1; i < len(samples)-1; i++ {
				if i > 2 && i < len(samples)-2 {
					avg := (samples[i-2] + samples[i-1] + samples[i] +
						samples[i+1] + samples[i+2]) / 5
					subBassEnergy += float64(avg * avg)
				}
			}
			subBassEnergy = math.Sqrt(subBassEnergy / float64(len(samples)))

			glitchCount := 0
			for i := 1; i < len(samples)-1; i++ {
				if math.Abs(float64(samples[i+1]-samples[i])) > 0.7 {
					glitchCount++
				}
			}
			glitchFactor := float64(glitchCount) / float64(len(samples))

			gritFactor := oddHarmonics / (evenHarmonics + 0.001) * phaseCancellation

			aggressionScore := (filthyRMS * 0.2) +
				(filthyHarmonics * 0.2) +
				(subBassEnergy * 0.2) +
				(gritFactor * 0.2) +
				(glitchFactor * 100.0 * 0.2)

			return map[string]float64{
				"aggressionScore": aggressionScore,
				"filthyRMS":       filthyRMS,
				"filthyHarmonics": filthyHarmonics,
				"subBassEnergy":   subBassEnergy,
				"gritFactor":      gritFactor,
				"glitchFactor":    glitchFactor,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("BrainMelter: FILTH SCORE=%.2f RMS=%.2f HARM=%.2f SUB=%.2f GRIT=%.2f GLITCH=%.2f",
				metrics["aggressionScore"], metrics["filthyRMS"], metrics["filthyHarmonics"],
				metrics["subBassEnergy"], metrics["gritFactor"], metrics["glitchFactor"])
		},
		PostCleanup: func() {
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
		},
	},
}

func TestNeurobassDestroyerReese(t *testing.T) {
	for _, tc := range neurobassTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

// Triangle wave tests
var triangleWaveBasicTests = []AudioTestCase{
	{
		Name: "A4",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, DISABLED)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: REGISTER_ENABLE},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_A4.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Get basic waveform analysis
			res := analyseWaveform(nil, samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, DISABLED)
		},
	},
	{
		Name: "C4",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: C4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_C4.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(C4_FREQUENCY), Tolerance: float64(C4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "E4",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: E4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_E4.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(E4_FREQUENCY), Tolerance: float64(E4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Low A2",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, DISABLED)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A2_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: REGISTER_ENABLE},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_Low_A2.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A2_FREQUENCY), Tolerance: float64(A2_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: AMPLITUDE_NORMAL, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, DISABLED)
		},
	},
	{
		Name: "High A5",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_High_A5.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A5_FREQUENCY), Tolerance: float64(A5_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Quarter Vol",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 64},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_Quarter_Vol.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 0.25, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Half Vol",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 128},
			{Register: TRI_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_basic_Half_Vol.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 0.5, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
}

func TestTriangleWaveBasic(t *testing.T) {
	for _, tc := range triangleWaveBasicTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var triangleWaveEnvelopeTests = []AudioTestCase{
	{
		Name: "Fast Attack Slow Release",
		PreSetup: func() {
			// Reset envelope parameters
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(TRI_FREQ, A4_FREQUENCY)
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_ATK, 1)
			chip.HandleRegisterWrite(TRI_DEC, 20)
			chip.HandleRegisterWrite(TRI_SUS, 200)
			chip.HandleRegisterWrite(TRI_REL, 100)

			// Start the envelope (key on)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
		},
		Config:          []RegisterWrite{}, // Configuration already done in PreSetup
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "tri_env_attack_Fast_Attack_Slow_Release.wav",
		AdditionalSetup: func() {
			// Wait for the decay phase and capture the sustain
			time.Sleep(time.Duration(20) * time.Millisecond)
			sustainSamples := captureAudio(nil, "tri_env_sustain_Fast_Attack_Slow_Release.wav")
			sustainAmp := getMaxAmplitude(sustainSamples)

			// Store the sustain amplitude for later validation
			testMetrics.Store("Fast Attack Slow Release.sustainAmp", sustainAmp)

			// Release the note
			chip.HandleRegisterWrite(TRI_CTRL, 1)
			time.Sleep(time.Duration(100) * time.Millisecond)
			releaseSamples := captureAudio(nil, "tri_env_release_Fast_Attack_Slow_Release.wav")
			releaseAmp := getMaxAmplitude(releaseSamples)

			// Store the release amplitude for later validation
			testMetrics.Store("Fast Attack Slow Release.releaseAmp", releaseAmp)
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze attack phase
			peakAmp := getMaxAmplitude(samples)

			// Retrieve stored metrics from other phases
			sustainAmp, _ := testMetrics.Load("Fast Attack Slow Release.sustainAmp")
			releaseAmp, _ := testMetrics.Load("Fast Attack Slow Release.releaseAmp")

			return map[string]float64{
				"peakAmp":    peakAmp,
				"sustainAmp": sustainAmp.(float64),
				"releaseAmp": releaseAmp.(float64),
			}
		},
		Expected: map[string]ExpectedMetric{
			"peakAmp":    {Expected: 0.99, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainAmp": {Expected: 0.78, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseAmp": {Expected: 0.77, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			testMetrics.Delete("Fast Attack Slow Release.sustainAmp")
			testMetrics.Delete("Fast Attack Slow Release.releaseAmp")
		},
	},
	{
		Name: "Slow Attack Fast Release",
		PreSetup: func() {
			// Reset envelope parameters
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(TRI_FREQ, A4_FREQUENCY)
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_ATK, 50)
			chip.HandleRegisterWrite(TRI_DEC, 20)
			chip.HandleRegisterWrite(TRI_SUS, 180)
			chip.HandleRegisterWrite(TRI_REL, 100)

			// Start the envelope (key on)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
		},
		Config:          []RegisterWrite{}, // Configuration already done in PreSetup
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "tri_env_attack_Slow_Attack_Fast_Release.wav",
		AdditionalSetup: func() {
			// Wait for the decay phase and capture the sustain
			time.Sleep(time.Duration(20) * time.Millisecond)
			sustainSamples := captureAudio(nil, "tri_env_sustain_Slow_Attack_Fast_Release.wav")
			sustainAmp := getMaxAmplitude(sustainSamples)

			// Store the sustain amplitude for later validation
			testMetrics.Store("Slow Attack Fast Release.sustainAmp", sustainAmp)

			// Release the note
			chip.HandleRegisterWrite(TRI_CTRL, 1)
			time.Sleep(time.Duration(100) * time.Millisecond)
			releaseSamples := captureAudio(nil, "tri_env_release_Slow_Attack_Fast_Release.wav")
			releaseAmp := getMaxAmplitude(releaseSamples)

			// Store the release amplitude for later validation
			testMetrics.Store("Slow Attack Fast Release.releaseAmp", releaseAmp)
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze attack phase
			peakAmp := getMaxAmplitude(samples)

			// Retrieve stored metrics from other phases
			sustainAmp, _ := testMetrics.Load("Slow Attack Fast Release.sustainAmp")
			releaseAmp, _ := testMetrics.Load("Slow Attack Fast Release.releaseAmp")

			return map[string]float64{
				"peakAmp":    peakAmp,
				"sustainAmp": sustainAmp.(float64),
				"releaseAmp": releaseAmp.(float64),
			}
		},
		Expected: map[string]ExpectedMetric{
			"peakAmp":    {Expected: 1.00, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainAmp": {Expected: 0.71, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseAmp": {Expected: 0.69, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			testMetrics.Delete("Slow Attack Fast Release.sustainAmp")
			testMetrics.Delete("Slow Attack Fast Release.releaseAmp")
		},
	},
	{
		Name: "Long Decay",
		PreSetup: func() {
			// Reset envelope parameters
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(TRI_FREQ, A4_FREQUENCY)
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_ATK, 5)
			chip.HandleRegisterWrite(TRI_DEC, 100)
			chip.HandleRegisterWrite(TRI_SUS, 128)
			chip.HandleRegisterWrite(TRI_REL, 200)

			// Start the envelope (key on)
			chip.HandleRegisterWrite(TRI_CTRL, 3)
		},
		Config:          []RegisterWrite{}, // Configuration already done in PreSetup
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "tri_env_attack_Long_Decay.wav",
		AdditionalSetup: func() {
			// Wait for the decay phase and capture the sustain
			time.Sleep(time.Duration(100) * time.Millisecond)
			sustainSamples := captureAudio(nil, "tri_env_sustain_Long_Decay.wav")
			sustainAmp := getMaxAmplitude(sustainSamples)

			// Store the sustain amplitude for later validation
			testMetrics.Store("Long Decay.sustainAmp", sustainAmp)

			// Release the note
			chip.HandleRegisterWrite(TRI_CTRL, 1)
			time.Sleep(time.Duration(200) * time.Millisecond)
			releaseSamples := captureAudio(nil, "tri_env_release_Long_Decay.wav")
			releaseAmp := getMaxAmplitude(releaseSamples)

			// Store the release amplitude for later validation
			testMetrics.Store("Long Decay.releaseAmp", releaseAmp)
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze attack phase
			peakAmp := getMaxAmplitude(samples)

			// Retrieve stored metrics from other phases
			sustainAmp, _ := testMetrics.Load("Long Decay.sustainAmp")
			releaseAmp, _ := testMetrics.Load("Long Decay.releaseAmp")

			return map[string]float64{
				"peakAmp":    peakAmp,
				"sustainAmp": sustainAmp.(float64),
				"releaseAmp": releaseAmp.(float64),
			}
		},
		Expected: map[string]ExpectedMetric{
			"peakAmp":    {Expected: 0.98, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainAmp": {Expected: 0.50, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseAmp": {Expected: 0.49, Tolerance: AMPLITUDE_TOLERANCE},
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			testMetrics.Delete("Long Decay.sustainAmp")
			testMetrics.Delete("Long Decay.releaseAmp")
		},
	},
}

func TestTriangleWaveEnvelope(t *testing.T) {
	for _, tc := range triangleWaveEnvelopeTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var triangleWaveSweepTests = []AudioTestCase{
	{
		Name: "Up Slow",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			// Configure sweep: upward (dirBit = 0x08), period = 0x03, shift = 2
			{Register: TRI_SWEEP, Value: 0x80 | (0x03 << 4) | 0x08 | 2},
			{Register: TRI_CTRL, Value: 3},
		},
		// Capture for 500ms
		CaptureDuration: 500 * time.Millisecond,
		Filename:        "triangle_sweep_UpSlow.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  backResult.frequency - frontResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Up Slow: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For upward sweep, final frequency must exceed the initial frequency by at least 50 Hz
			if backFreq <= frontFreq {
				t.Errorf("Upward sweep failed: final frequency %.1f not greater than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Upward sweep overall increase too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_SWEEP, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Down Fast",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: 1760},
			{Register: TRI_VOL, Value: 255},
			// Configure sweep: downward (dirBit = 0), period = 0x01, shift = 3
			{Register: TRI_SWEEP, Value: 0x80 | (0x01 << 4) | 3},
			{Register: TRI_CTRL, Value: 3},
		},
		// Capture for 300ms
		CaptureDuration: 300 * time.Millisecond,
		Filename:        "triangle_sweep_DownFast.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  frontResult.frequency - backResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Down Fast: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For downward sweep, final frequency must be lower than the initial frequency by at least 50 Hz
			if backFreq >= frontFreq {
				t.Errorf("Downward sweep failed: final frequency %.1f not lower than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Downward sweep overall drop too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_SWEEP, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Up Wide",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A2_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			// Configure sweep: upward (dirBit = 0x08), period = 0x02, shift = 4
			{Register: TRI_SWEEP, Value: 0x80 | (0x02 << 4) | 0x08 | 4},
			{Register: TRI_CTRL, Value: 3},
		},
		// Capture for 400ms
		CaptureDuration: 400 * time.Millisecond,
		Filename:        "triangle_sweep_UpWide.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  backResult.frequency - frontResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Up Wide: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For upward sweep, final frequency must exceed the initial frequency by at least 50 Hz
			if backFreq <= frontFreq {
				t.Errorf("Upward sweep failed: final frequency %.1f not greater than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Upward sweep overall increase too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_SWEEP, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Down Narrow",
		PreSetup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			// Configure sweep: downward (dirBit = 0), period = 0x02, shift = 1
			{Register: TRI_SWEEP, Value: 0x80 | (0x02 << 4) | 1},
			{Register: TRI_CTRL, Value: 3},
		},
		// Capture for 400ms
		CaptureDuration: 400 * time.Millisecond,
		Filename:        "triangle_sweep_DownNarrow.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  frontResult.frequency - backResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Down Narrow: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For downward sweep, final frequency must be lower than the initial frequency by at least 50 Hz
			if backFreq >= frontFreq {
				t.Errorf("Downward sweep failed: final frequency %.1f not lower than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Downward sweep overall drop too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_SWEEP, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
}

func TestTriangleWaveSweep(t *testing.T) {
	for _, tc := range triangleWaveSweepTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var triangleWaveSyncTests = []AudioTestCase{
	{
		Name: "Octave",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Configure slave oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: A5_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Set up hard sync: assign channel 1 as the sync source for channel 0
			{Register: SYNC_SOURCE_CH0, Value: 1},

			// Enable both channels
			{Register: TRI_CTRL, Value: 3},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_Octave.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Compute the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 0.5, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Octave sync ratio: %.3f (expected %.3f)", metrics["syncRatio"], 0.5)
			t.Logf("Octave waveform purity: %.2f", metrics["purity"])

			// Additional validation can be done here if needed beyond the standard Expected check
			if metrics["purity"] < 0.15 {
				t.Errorf("Sine wave distorted during sync in Octave: purity = %.2f, minimum = %.2f",
					metrics["purity"], 0.15)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0)
		},
	},
	{
		Name: "Fifth",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Configure slave oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: 660},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Set up hard sync: assign channel 1 as the sync source for channel 0
			{Register: SYNC_SOURCE_CH0, Value: 1},

			// Enable both channels
			{Register: TRI_CTRL, Value: 3},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_Fifth.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Compute the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Fifth sync ratio: %.3f (expected %.3f)", metrics["syncRatio"], 1.0)
			t.Logf("Fifth waveform purity: %.2f", metrics["purity"])

			if metrics["purity"] < 0.15 {
				t.Errorf("Sine wave distorted during sync in Fifth: purity = %.2f, minimum = %.2f",
					metrics["purity"], 0.15)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0)
		},
	},
	{
		Name: "Third",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Configure slave oscillator (square wave, channel 0)
			{Register: SQUARE_FREQ, Value: 550},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},

			// Set up hard sync: assign channel 1 as the sync source for channel 0
			{Register: SYNC_SOURCE_CH0, Value: 1},

			// Enable both channels
			{Register: TRI_CTRL, Value: 3},
			{Register: SQUARE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sync_Third.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Compute the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Third sync ratio: %.3f (expected %.3f)", metrics["syncRatio"], 1.0)
			t.Logf("Third waveform purity: %.2f", metrics["purity"])

			if metrics["purity"] < 0.15 {
				t.Errorf("Sine wave distorted during sync in Third: purity = %.2f, minimum = %.2f",
					metrics["purity"], 0.15)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH0, 0)
		},
	},
}

func TestTriangleWaveSync(t *testing.T) {
	for _, tc := range triangleWaveSyncTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var triangleWaveRingModulationTests = []AudioTestCase{
	{
		Name: "Octave Down",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure triangle wave (carrier)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Configure sine wave (modulator)
			{Register: SINE_FREQ, Value: 220},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Set ring modulation: use sine wave as the modulator
			{Register: RING_MOD_SOURCE_CH1, Value: 2},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_ringmod_Octave_Down.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: 220.2, Tolerance: 220.2 * FREQ_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Octave Down: freq=%.2f Hz amp=%.2f", metrics["frequency"], metrics["amplitude"])

			// Verify amplitude does not exceed the maximum
			if metrics["amplitude"] > 1.0 {
				t.Errorf("Octave Down: Amplitude too high: %.2f > %.2f", metrics["amplitude"], 1.0)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Octave Up",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure triangle wave (carrier)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Configure sine wave (modulator)
			{Register: SINE_FREQ, Value: A5_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Set ring modulation: use sine wave as the modulator
			{Register: RING_MOD_SOURCE_CH1, Value: 2},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_ringmod_Octave_Up.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A5_FREQUENCY), Tolerance: float64(A5_FREQUENCY) * FREQ_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Octave Up: freq=%.2f Hz amp=%.2f", metrics["frequency"], metrics["amplitude"])

			// Verify amplitude does not exceed the maximum
			if metrics["amplitude"] > 0.8 {
				t.Errorf("Octave Up: Amplitude too high: %.2f > %.2f", metrics["amplitude"], 0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Fifth",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
		},
		Config: []RegisterWrite{
			// Configure triangle wave (carrier)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Configure sine wave (modulator)
			{Register: SINE_FREQ, Value: 660},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Set ring modulation: use sine wave as the modulator
			{Register: RING_MOD_SOURCE_CH1, Value: 2},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "triangle_ringmod_Fifth.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: 660.4, Tolerance: 660.4 * FREQ_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Fifth: freq=%.2f Hz amp=%.2f", metrics["frequency"], metrics["amplitude"])

			// Verify amplitude does not exceed the maximum
			if metrics["amplitude"] > 1.0 {
				t.Errorf("Fifth: Amplitude too high: %.2f > %.2f", metrics["amplitude"], 1.0)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
}

func TestTriangleWaveRingModulation(t *testing.T) {
	for _, tc := range triangleWaveRingModulationTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

// Sine wave tests
var sineWaveBasicTests = []AudioTestCase{
	{
		Name: "A4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_A4.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Get basic waveform analysis
			res := analyseWaveform(nil, samples)

			// Additional sine-specific analysis
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.95, Tolerance: 0.0}, // Min threshold
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			// Standard validation for frequency and amplitude
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE}, "amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			// Special validation for purity (must be above threshold)
			purity := metrics["purity"]
			if purity < 0.95 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.95)
			}

			// Phase validation
			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("A4: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "C4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: C4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_C4.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(C4_FREQUENCY), Tolerance: float64(C4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.95, Tolerance: 0.0},
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: float64(C4_FREQUENCY), Tolerance: float64(C4_FREQUENCY) * FREQ_TOLERANCE},
				"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.95 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.95)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("C4: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "E4",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: E4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_E4.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(E4_FREQUENCY), Tolerance: float64(E4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.95, Tolerance: 0.0},
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: float64(E4_FREQUENCY), Tolerance: float64(E4_FREQUENCY) * FREQ_TOLERANCE},
				"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.95 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.95)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("E4: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Quarter Vol",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 64},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_QuarterVol.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE}, "amplitude": {Expected: 0.25, Tolerance: AMPLITUDE_TOLERANCE},
			"purity": {Expected: 0.95, Tolerance: 0.0},
			"phase":  {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE}, "amplitude": {Expected: 0.25, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.95 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.95)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("Quarter Vol: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Half Vol",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 128},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_HalfVol.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 0.5, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.95, Tolerance: 0.0},
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: float64(A4_FREQUENCY), Tolerance: float64(A4_FREQUENCY) * FREQ_TOLERANCE}, "amplitude": {Expected: 0.5, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.95 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.95)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("Half Vol: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Low A2",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A2_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_LowA2.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A2_FREQUENCY), Tolerance: float64(A2_FREQUENCY) * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.95, Tolerance: 0.0},
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: float64(A2_FREQUENCY), Tolerance: float64(A2_FREQUENCY) * FREQ_TOLERANCE},
				"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.95 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.95)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("Low A2: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "High A6",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: 1760},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_HighA6.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: 1760.0, Tolerance: 1760.0 * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.90, Tolerance: 0.0},
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: 1760.0, Tolerance: 1760.0 * FREQ_TOLERANCE},
				"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.90 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.90)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("High A6: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Very High",
		PreSetup: func() {
			// Disable all channels and reset phases.
			for _, ch := range []uint32{SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL} {
				chip.HandleRegisterWrite(ch, 0)
			}
			for i := range chip.channels {
				chip.channels[i].phase = 0
			}
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: 8000},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 0},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_basic_VeryHigh.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			res := analyseWaveform(nil, samples)
			purity := analyzeSinePurity(samples)
			phase := analyzeSinePhase(samples)

			return map[string]float64{
				"frequency": res.frequency,
				"amplitude": res.amplitude,
				"purity":    purity,
				"phase":     phase,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: 8000.0, Tolerance: 8000.0 * FREQ_TOLERANCE},
			"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"purity":    {Expected: 0.85, Tolerance: 0.0},
			"phase":     {Expected: 0.0, Tolerance: 0.1},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			defaultValidate(t, map[string]ExpectedMetric{
				"frequency": {Expected: 8000.0, Tolerance: 8000.0 * FREQ_TOLERANCE},
				"amplitude": {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)

			purity := metrics["purity"]
			if purity < 0.85 {
				t.Errorf("Purity below threshold: got %.2f, expected > %.2f", purity, 0.85)
			}

			phase := metrics["phase"]
			if math.Abs(phase-0.0) > 0.1 {
				t.Errorf("Phase error: got %.2f, expected %.2f", phase, 0.0)
			}

			t.Logf("Very High: freq=%.2f Hz amp=%.2f purity=%.2f phase=%.2f",
				metrics["frequency"], metrics["amplitude"], purity, phase)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
}

func TestSineWaveBasic(t *testing.T) {
	for _, tc := range sineWaveBasicTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

// This needs fixing it's totally borked
var sineEnvelopeTests = []AudioTestCase{
	{
		Name: "Fast Attack Slow Release",
		PreSetup: func() {
			// Reset and configure the sine channel
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 1},
			{Register: SINE_DEC, Value: 20},
			{Register: SINE_SUS, Value: 200},
			{Register: SINE_REL, Value: 100},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,     // Use standard duration
		Filename:        "sine_env_Fast_Attack_Slow_Release.wav", // Main filename
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			// Start the envelope (key on)
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			attackSamples := captureAudio(t, "sine_env_attack_Fast_Attack_Slow_Release.wav")

			// Wait for decay phase then capture sustain
			time.Sleep(20 * time.Millisecond)
			sustainSamples := captureAudio(t, "sine_env_sustain_Fast_Attack_Slow_Release.wav")

			// Release the note
			chip.HandleRegisterWrite(SINE_CTRL, 1)
			time.Sleep(100 * time.Millisecond)
			releaseSamples := captureAudio(t, "sine_env_release_Fast_Attack_Slow_Release.wav")

			// Store all amplitudes directly
			peakAmp := getMaxAmplitude(attackSamples)
			sustainAmp := getMaxAmplitude(sustainSamples)
			releaseAmp := getMaxAmplitude(releaseSamples)

			// We'll store these directly in the return value
			return []float32{float32(peakAmp), float32(sustainAmp), float32(releaseAmp)}
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Since we stored the calculated amplitudes directly, just extract them
			return map[string]float64{
				"peakAmp":    float64(samples[0]),
				"sustainAmp": float64(samples[1]),
				"releaseAmp": float64(samples[2]),
			}
		},
		Expected: map[string]ExpectedMetric{
			"peakAmp":    {Expected: 0.99, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainAmp": {Expected: 0.78, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseAmp": {Expected: 0.77, Tolerance: AMPLITUDE_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Fast Attack Slow Release: peak=%.2f sustain=%.2f release=%.2f",
				metrics["peakAmp"], metrics["sustainAmp"], metrics["releaseAmp"])

			defaultValidate(t, map[string]ExpectedMetric{
				"peakAmp":    {Expected: 0.99, Tolerance: AMPLITUDE_TOLERANCE},
				"sustainAmp": {Expected: 0.78, Tolerance: AMPLITUDE_TOLERANCE},
				"releaseAmp": {Expected: 0.77, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Data: make(map[string]interface{}),
	},
	{
		Name: "Slow Attack Fast Release",
		PreSetup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 50},
			{Register: SINE_DEC, Value: 20},
			{Register: SINE_SUS, Value: 180},
			{Register: SINE_REL, Value: 100},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_env_Slow_Attack_Fast_Release.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			// Use the filename passed to the function for the first capture
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			attackSamples := captureAudio(t, filename)

			// Use derived filenames for the other captures
			time.Sleep(20 * time.Millisecond)
			sustainSamples := captureAudio(t, strings.Replace(filename, "env", "env_sustain", 1))

			// Release the note
			chip.HandleRegisterWrite(SINE_CTRL, 1)
			time.Sleep(100 * time.Millisecond)
			releaseSamples := captureAudio(t, strings.Replace(filename, "env", "env_release", 1))

			// Store all amplitudes directly
			peakAmp := getMaxAmplitude(attackSamples)
			sustainAmp := getMaxAmplitude(sustainSamples)
			releaseAmp := getMaxAmplitude(releaseSamples)

			return []float32{float32(peakAmp), float32(sustainAmp), float32(releaseAmp)}
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			return map[string]float64{
				"peakAmp":    float64(samples[0]),
				"sustainAmp": float64(samples[1]),
				"releaseAmp": float64(samples[2]),
			}
		},
		Expected: map[string]ExpectedMetric{
			"peakAmp":    {Expected: 1.00, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainAmp": {Expected: 0.71, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseAmp": {Expected: 0.69, Tolerance: AMPLITUDE_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Slow Attack Fast Release: peak=%.2f sustain=%.2f release=%.2f",
				metrics["peakAmp"], metrics["sustainAmp"], metrics["releaseAmp"])

			defaultValidate(t, map[string]ExpectedMetric{
				"peakAmp":    {Expected: 1.00, Tolerance: AMPLITUDE_TOLERANCE},
				"sustainAmp": {Expected: 0.71, Tolerance: AMPLITUDE_TOLERANCE},
				"releaseAmp": {Expected: 0.69, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Data: make(map[string]interface{}),
	},
	{
		Name: "Long Decay",
		PreSetup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_ATK, Value: 5},
			{Register: SINE_DEC, Value: 100},
			{Register: SINE_SUS, Value: 128},
			{Register: SINE_REL, Value: 200},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_env_Long_Decay.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			// Use the filename passed to the function for the first capture
			chip.HandleRegisterWrite(SINE_CTRL, 3)
			attackSamples := captureAudio(t, filename)

			// Use derived filenames for the other captures
			time.Sleep(100 * time.Millisecond)
			sustainSamples := captureAudio(t, strings.Replace(filename, "env", "env_sustain", 1))

			// Release the note
			chip.HandleRegisterWrite(SINE_CTRL, 1)
			time.Sleep(200 * time.Millisecond)
			releaseSamples := captureAudio(t, strings.Replace(filename, "env", "env_release", 1))

			// Store all amplitudes directly
			peakAmp := getMaxAmplitude(attackSamples)
			sustainAmp := getMaxAmplitude(sustainSamples)
			releaseAmp := getMaxAmplitude(releaseSamples)

			return []float32{float32(peakAmp), float32(sustainAmp), float32(releaseAmp)}
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			return map[string]float64{
				"peakAmp":    float64(samples[0]),
				"sustainAmp": float64(samples[1]),
				"releaseAmp": float64(samples[2]),
			}
		},
		Expected: map[string]ExpectedMetric{
			"peakAmp":    {Expected: 0.98, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainAmp": {Expected: 0.50, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseAmp": {Expected: 0.49, Tolerance: AMPLITUDE_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Long Decay: peak=%.2f sustain=%.2f release=%.2f",
				metrics["peakAmp"], metrics["sustainAmp"], metrics["releaseAmp"])

			defaultValidate(t, map[string]ExpectedMetric{
				"peakAmp":    {Expected: 0.98, Tolerance: AMPLITUDE_TOLERANCE},
				"sustainAmp": {Expected: 0.50, Tolerance: AMPLITUDE_TOLERANCE},
				"releaseAmp": {Expected: 0.49, Tolerance: AMPLITUDE_TOLERANCE},
			}, metrics)
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Data: make(map[string]interface{}),
	},
}

func TestSineWaveEnvelope(t *testing.T) {
	for _, tc := range sineEnvelopeTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var sineWaveSweepTests = []AudioTestCase{
	{
		Name: "Up Slow",
		PreSetup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			// Configure sweep: upward (dirBit = 0x08), period = 0x03, shift = 2
			{Register: SINE_SWEEP, Value: 0x80 | (0x03 << 4) | 0x08 | 2},
			{Register: SINE_CTRL, Value: 3},
		},
		// Capture for 500ms
		CaptureDuration: 500 * time.Millisecond,
		Filename:        "sine_sweep_UpSlow.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  backResult.frequency - frontResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Up Slow: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For upward sweep, final frequency must exceed the initial frequency by at least 50 Hz
			if backFreq <= frontFreq {
				t.Errorf("Upward sweep failed: final frequency %.1f not greater than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Upward sweep overall increase too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_SWEEP, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Down Fast",
		PreSetup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: 1760},
			{Register: SINE_VOL, Value: 255},
			// Configure sweep: downward (dirBit = 0), period = 0x01, shift = 3
			{Register: SINE_SWEEP, Value: 0x80 | (0x01 << 4) | 3},
			{Register: SINE_CTRL, Value: 3},
		},
		// Capture for 300ms
		CaptureDuration: 300 * time.Millisecond,
		Filename:        "sine_sweep_DownFast.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  frontResult.frequency - backResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Down Fast: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For downward sweep, final frequency must be lower than the initial frequency by at least 50 Hz
			if backFreq >= frontFreq {
				t.Errorf("Downward sweep failed: final frequency %.1f not lower than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Downward sweep overall drop too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_SWEEP, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Up Wide",
		PreSetup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A2_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			// Configure sweep: upward (dirBit = 0x08), period = 0x02, shift = 4
			{Register: SINE_SWEEP, Value: 0x80 | (0x02 << 4) | 0x08 | 4},
			{Register: SINE_CTRL, Value: 3},
		},
		// Capture for 400ms
		CaptureDuration: 400 * time.Millisecond,
		Filename:        "sine_sweep_UpWide.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  backResult.frequency - frontResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Up Wide: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For upward sweep, final frequency must exceed the initial frequency by at least 50 Hz
			if backFreq <= frontFreq {
				t.Errorf("Upward sweep failed: final frequency %.1f not greater than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Upward sweep overall increase too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_SWEEP, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
	{
		Name: "Down Narrow",
		PreSetup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A5_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			// Configure sweep: downward (dirBit = 0), period = 0x02, shift = 1
			{Register: SINE_SWEEP, Value: 0x80 | (0x02 << 4) | 1},
			{Register: SINE_CTRL, Value: 3},
		},
		// Capture for 400ms
		CaptureDuration: 400 * time.Millisecond,
		Filename:        "sine_sweep_DownNarrow.wav",
		// Custom capture function that collects samples in real time
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			sampleCount := int(duration.Seconds() * float64(SAMPLE_RATE))
			samples := make([]float32, sampleCount)
			for i := 0; i < sampleCount; i++ {
				samples[i] = chip.GenerateSample()
				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
			}
			return samples
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			n := len(samples)
			frontSamples := samples[:n/5]
			backSamples := samples[n-n/5:]
			frontResult := analyseWaveform(nil, frontSamples)
			backResult := analyseWaveform(nil, backSamples)

			return map[string]float64{
				"frontFreq": frontResult.frequency,
				"backFreq":  backResult.frequency,
				"freqDiff":  frontResult.frequency - backResult.frequency,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			frontFreq := metrics["frontFreq"]
			backFreq := metrics["backFreq"]
			diff := metrics["freqDiff"]

			t.Logf("Down Narrow: front frequency = %.1f Hz, back frequency = %.1f Hz",
				frontFreq, backFreq)

			// For downward sweep, final frequency must be lower than the initial frequency by at least 50 Hz
			if backFreq >= frontFreq {
				t.Errorf("Downward sweep failed: final frequency %.1f not lower than initial %.1f Hz",
					backFreq, frontFreq)
			}
			if diff < 50 {
				t.Errorf("Downward sweep overall drop too small: %.1f Hz", diff)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_SWEEP, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
		},
	},
}

func TestSineWaveSweep(t *testing.T) {
	for _, tc := range sineWaveSweepTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var sineWaveSyncTests = []AudioTestCase{
	{
		Name: "Octave",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Configure slave oscillator (sine wave, channel 2)
			{Register: SINE_FREQ, Value: A5_FREQUENCY},
			{Register: SINE_VOL, Value: 255},

			// Set up hard sync: assign channel 1 as the sync source for channel 2
			{Register: SYNC_SOURCE_CH2, Value: 1},

			// Enable both channels
			{Register: TRI_CTRL, Value: 3},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_sync_Octave.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Compute the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 0.5, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Octave sync ratio: %.3f (expected %.3f)", metrics["syncRatio"], 0.5)
			t.Logf("Octave waveform purity: %.2f", metrics["purity"])

			// Additional validation can be done here if needed beyond the standard Expected check
			if metrics["purity"] < 0.15 {
				t.Errorf("Sine wave distorted during sync in Octave: purity = %.2f, minimum = %.2f",
					metrics["purity"], 0.15)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		},
	},
	{
		Name: "Fifth",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Configure slave oscillator (sine wave, channel 2)
			{Register: SINE_FREQ, Value: 660},
			{Register: SINE_VOL, Value: 255},

			// Set up hard sync: assign channel 1 as the sync source for channel 2
			{Register: SYNC_SOURCE_CH2, Value: 1},

			// Enable both channels
			{Register: TRI_CTRL, Value: 3},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_sync_Fifth.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Compute the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Fifth sync ratio: %.3f (expected %.3f)", metrics["syncRatio"], 1.0)
			t.Logf("Fifth waveform purity: %.2f", metrics["purity"])

			if metrics["purity"] < 0.15 {
				t.Errorf("Sine wave distorted during sync in Fifth: purity = %.2f, minimum = %.2f",
					metrics["purity"], 0.15)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		},
	},
	{
		Name: "Third",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		},
		Config: []RegisterWrite{
			// Configure master oscillator (triangle wave, channel 1)
			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},

			// Configure slave oscillator (sine wave, channel 2)
			{Register: SINE_FREQ, Value: 550},
			{Register: SINE_VOL, Value: 255},

			// Set up hard sync: assign channel 1 as the sync source for channel 2
			{Register: SYNC_SOURCE_CH2, Value: 1},

			// Enable both channels
			{Register: TRI_CTRL, Value: 3},
			{Register: SINE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_sync_Third.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Analyze zero crossings to compute the average interval
			var intervals []int
			prev := samples[0]
			lastCrossing := 0
			for i := 1; i < len(samples); i++ {
				if prev <= 0 && samples[i] > 0 {
					if lastCrossing > 0 {
						intervals = append(intervals, i-lastCrossing)
					}
					lastCrossing = i
				}
				prev = samples[i]
			}

			avgInterval := 0.0
			for _, interval := range intervals {
				avgInterval += float64(interval)
			}
			if len(intervals) > 0 {
				avgInterval /= float64(len(intervals))
			}

			// Compute the master period based on the master frequency
			masterPeriod := float64(SAMPLE_RATE) / float64(A4_FREQUENCY)
			ratio := avgInterval / masterPeriod

			// Calculate waveform purity
			purity := analyzeSinePurity(samples)

			return map[string]float64{
				"syncRatio": ratio,
				"purity":    purity,
			}
		},
		Expected: map[string]ExpectedMetric{
			"syncRatio": {Expected: 1.0, Tolerance: FREQ_TOLERANCE},
			"purity":    {Expected: 0.3, Tolerance: 0.15}, // MIN_SYNC_PURITY = 0.15
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Third sync ratio: %.3f (expected %.3f)", metrics["syncRatio"], 1.0)
			t.Logf("Third waveform purity: %.2f", metrics["purity"])

			if metrics["purity"] < 0.15 {
				t.Errorf("Sine wave distorted during sync in Third: purity = %.2f, minimum = %.2f",
					metrics["purity"], 0.15)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		},
	},
}

func TestSineWaveSync(t *testing.T) {
	for _, tc := range sineWaveSyncTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var sineWaveRingModulationTests = []AudioTestCase{
	{
		Name: "Octave Down",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		},
		Config: []RegisterWrite{
			// Configure sine wave (carrier)
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Configure triangle wave (modulator)
			{Register: TRI_FREQ, Value: 220},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Set ring modulation: use triangle wave as the modulator
			{Register: RING_MOD_SOURCE_CH2, Value: 1},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_ringmod_Octave_Down.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: 220.2, Tolerance: 220.2 * FREQ_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Octave Down: freq=%.2f Hz amp=%.2f", metrics["frequency"], metrics["amplitude"])

			// Verify amplitude does not exceed the maximum
			if metrics["amplitude"] > 1.0 {
				t.Errorf("Octave Down: Amplitude too high: %.2f > %.2f", metrics["amplitude"], 1.0)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Octave Up",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		},
		Config: []RegisterWrite{
			// Configure sine wave (carrier)
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Configure triangle wave (modulator)
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Set ring modulation: use triangle wave as the modulator
			{Register: RING_MOD_SOURCE_CH2, Value: 1},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_ringmod_Octave_Up.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: float64(A5_FREQUENCY), Tolerance: float64(A5_FREQUENCY) * FREQ_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Octave Up: freq=%.2f Hz amp=%.2f", metrics["frequency"], metrics["amplitude"])

			// Verify amplitude does not exceed the maximum
			if metrics["amplitude"] > 0.8 {
				t.Errorf("Octave Up: Amplitude too high: %.2f > %.2f", metrics["amplitude"], 0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
	{
		Name: "Fifth",
		PreSetup: func() {
			// Reset channels
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		},
		Config: []RegisterWrite{
			// Configure sine wave (carrier)
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Configure triangle wave (modulator)
			{Register: TRI_FREQ, Value: 660},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Set ring modulation: use triangle wave as the modulator
			{Register: RING_MOD_SOURCE_CH2, Value: 1},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "sine_ringmod_Fifth.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			result := analyseWaveform(nil, samples)
			return map[string]float64{
				"frequency": result.frequency,
				"amplitude": result.amplitude,
			}
		},
		Expected: map[string]ExpectedMetric{
			"frequency": {Expected: 660.4, Tolerance: 660.4 * FREQ_TOLERANCE},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			t.Logf("Fifth: freq=%.2f Hz amp=%.2f", metrics["frequency"], metrics["amplitude"])

			// Verify amplitude does not exceed the maximum
			if metrics["amplitude"] > 1.0 {
				t.Errorf("Fifth: Amplitude too high: %.2f > %.2f", metrics["amplitude"], 1.0)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
		},
	},
}

func TestSineWaveRingModulation(t *testing.T) {
	for _, tc := range sineWaveRingModulationTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

// Noise tests
var noiseBasicTests = []AudioTestCase{
	{
		Name: "White Noise Standard",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_WHITE},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_White_Noise_Standard.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min
			expectedRange := float64(255) / 255.0 * 2.0

			// Calculate maximum correlation to check for repetition
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var correlation float64
				for i := 0; i < len(samples)-lag; i++ {
					correlation += float64(samples[i] * samples[i+lag])
				}
				correlation /= float64(len(samples) - lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zerosCrossed := 0
			lastSample := samples[0]
			for i := 1; i < len(samples); i++ {
				if (lastSample < 0 && samples[i] >= 0) || (lastSample >= 0 && samples[i] < 0) {
					zerosCrossed++
				}
				lastSample = samples[i]
			}
			crossingRate := float64(zerosCrossed) / float64(len(samples))

			return map[string]float64{
				"dynamicRange":   dynamicRange,
				"maxCorrelation": maxCorrelation,
				"crossingRate":   crossingRate,
				"max":            max,
				"expectedRange":  expectedRange,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			max := metrics["max"]
			expectedRange := metrics["expectedRange"]

			t.Logf("White Noise Standard: range=%.2f, correlation=%.2f, crossings=%.2f",
				dynamicRange, maxCorrelation, crossingRate)

			// Check dynamic range
			minExpectedRange := 0.8 * expectedRange
			if dynamicRange < minExpectedRange {
				t.Errorf("White Noise Standard: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for excessive repetition
			if maxCorrelation > 0.9 {
				t.Errorf("White Noise Standard: Excessive repetition detected: correlation=%.2f, max=%.2f",
					maxCorrelation, 0.9)
			}

			// Check maximum amplitude
			expectedMax := float64(255) / 255.0
			if max > expectedMax+AMPLITUDE_TOLERANCE {
				t.Errorf("White Noise Standard: Output exceeds expected volume: got %.2f, expected <= %.2f",
					max, expectedMax)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Periodic Low Freq",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 100},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_PERIODIC},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_Periodic_Low_Freq.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min
			expectedRange := float64(255) / 255.0 * 2.0

			// Calculate maximum correlation to check for repetition
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var correlation float64
				for i := 0; i < len(samples)-lag; i++ {
					correlation += float64(samples[i] * samples[i+lag])
				}
				correlation /= float64(len(samples) - lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zerosCrossed := 0
			lastSample := samples[0]
			for i := 1; i < len(samples); i++ {
				if (lastSample < 0 && samples[i] >= 0) || (lastSample >= 0 && samples[i] < 0) {
					zerosCrossed++
				}
				lastSample = samples[i]
			}
			crossingRate := float64(zerosCrossed) / float64(len(samples))

			return map[string]float64{
				"dynamicRange":      dynamicRange,
				"maxCorrelation":    maxCorrelation,
				"crossingRate":      crossingRate,
				"max":               max,
				"expectedRange":     expectedRange,
				"expectedCrossRate": math.Min(float64(100)/SAMPLE_RATE, 0.5),
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			max := metrics["max"]
			expectedRange := metrics["expectedRange"]
			expectedCrossRate := metrics["expectedCrossRate"]

			t.Logf("Periodic Low Freq: range=%.2f, correlation=%.2f, crossings=%.2f",
				dynamicRange, maxCorrelation, crossingRate)

			// Check dynamic range
			minExpectedRange := 0.6 * expectedRange
			if dynamicRange < minExpectedRange {
				t.Errorf("Periodic Low Freq: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for excessive repetition - periodic noise can be more repetitive
			if maxCorrelation > 0.95 {
				t.Errorf("Periodic Low Freq: Excessive repetition detected: correlation=%.2f, max=%.2f",
					maxCorrelation, 0.95)
			}

			// Check zero-crossing rate
			crossingTolerance := 0.2
			if math.Abs(crossingRate-expectedCrossRate) > crossingTolerance {
				t.Errorf("Periodic Low Freq: Unexpected zero-crossing rate: got %.2f, expected %.2f ±%.2f",
					crossingRate, expectedCrossRate, crossingTolerance)
			}

			// Check maximum amplitude
			expectedMax := float64(255) / 255.0
			if max > expectedMax+AMPLITUDE_TOLERANCE {
				t.Errorf("Periodic Low Freq: Output exceeds expected volume: got %.2f, expected <= %.2f",
					max, expectedMax)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Metallic High Freq",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 4000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_METALLIC},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_Metallic_High_Freq.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min
			expectedRange := float64(255) / 255.0 * 2.0

			// Calculate maximum correlation to check for repetition
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var correlation float64
				for i := 0; i < len(samples)-lag; i++ {
					correlation += float64(samples[i] * samples[i+lag])
				}
				correlation /= float64(len(samples) - lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zerosCrossed := 0
			lastSample := samples[0]
			for i := 1; i < len(samples); i++ {
				if (lastSample < 0 && samples[i] >= 0) || (lastSample >= 0 && samples[i] < 0) {
					zerosCrossed++
				}
				lastSample = samples[i]
			}
			crossingRate := float64(zerosCrossed) / float64(len(samples))

			// Check for metallic character
			var spectralSum float64
			for i := 1; i < len(samples); i++ {
				spectralSum += math.Abs(float64(samples[i] - samples[i-1]))
			}
			spectralDensity := spectralSum / float64(len(samples))

			return map[string]float64{
				"dynamicRange":      dynamicRange,
				"maxCorrelation":    maxCorrelation,
				"crossingRate":      crossingRate,
				"max":               max,
				"expectedRange":     expectedRange,
				"expectedCrossRate": math.Min(float64(4000)/SAMPLE_RATE, 0.5),
				"spectralDensity":   spectralDensity,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			max := metrics["max"]
			expectedRange := metrics["expectedRange"]
			expectedCrossRate := metrics["expectedCrossRate"]
			spectralDensity := metrics["spectralDensity"]

			t.Logf("Metallic High Freq: range=%.2f, correlation=%.2f, crossings=%.2f, spectral=%.2f",
				dynamicRange, maxCorrelation, crossingRate, spectralDensity)

			// Check dynamic range
			minExpectedRange := 0.7 * expectedRange
			if dynamicRange < minExpectedRange {
				t.Errorf("Metallic High Freq: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for excessive repetition - metallic can be more repetitive than white noise
			if maxCorrelation > 0.5 {
				t.Errorf("Metallic High Freq: Excessive repetition detected: correlation=%.2f, max=%.2f",
					maxCorrelation, 0.5)
			}

			// Check zero-crossing rate
			crossingTolerance := 0.2
			if math.Abs(crossingRate-expectedCrossRate) > crossingTolerance {
				t.Errorf("Metallic High Freq: Unexpected zero-crossing rate: got %.2f, expected %.2f ±%.2f",
					crossingRate, expectedCrossRate, crossingTolerance)
			}

			// Check maximum amplitude
			expectedMax := float64(255) / 255.0
			if max > expectedMax+AMPLITUDE_TOLERANCE {
				t.Errorf("Metallic High Freq: Output exceeds expected volume: got %.2f, expected <= %.2f",
					max, expectedMax)
			}

			// Check metallic character
			if spectralDensity < MIN_METALLIC_DENSITY {
				t.Errorf("Metallic High Freq: Insufficient metallic character: density = %.2f",
					spectralDensity)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "White Noise Low Vol",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 64},
			{Register: NOISE_MODE, Value: NOISE_MODE_WHITE},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_White_Noise_Low_Vol.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min
			expectedRange := float64(64) / 255.0 * 2.0

			// Calculate maximum correlation to check for repetition
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var correlation float64
				for i := 0; i < len(samples)-lag; i++ {
					correlation += float64(samples[i] * samples[i+lag])
				}
				correlation /= float64(len(samples) - lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zerosCrossed := 0
			lastSample := samples[0]
			for i := 1; i < len(samples); i++ {
				if (lastSample < 0 && samples[i] >= 0) || (lastSample >= 0 && samples[i] < 0) {
					zerosCrossed++
				}
				lastSample = samples[i]
			}
			crossingRate := float64(zerosCrossed) / float64(len(samples))

			return map[string]float64{
				"dynamicRange":   dynamicRange,
				"maxCorrelation": maxCorrelation,
				"crossingRate":   crossingRate,
				"max":            max,
				"expectedRange":  expectedRange,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			max := metrics["max"]
			expectedRange := metrics["expectedRange"]

			t.Logf("White Noise Low Vol: range=%.2f, correlation=%.2f, crossings=%.2f",
				dynamicRange, maxCorrelation, crossingRate)

			// Check dynamic range
			minExpectedRange := 0.2 * expectedRange
			if dynamicRange < minExpectedRange {
				t.Errorf("White Noise Low Vol: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for excessive repetition
			if maxCorrelation > 0.2 {
				t.Errorf("White Noise Low Vol: Excessive repetition detected: correlation=%.2f, max=%.2f",
					maxCorrelation, 0.2)
			}

			// Check maximum amplitude
			expectedMax := float64(64) / 255.0
			if max > expectedMax+AMPLITUDE_TOLERANCE {
				t.Errorf("White Noise Low Vol: Output exceeds expected volume: got %.2f, expected <= %.2f",
					max, expectedMax)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
}

func TestNoiseBasic(t *testing.T) {
	for _, tc := range noiseBasicTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var noiseEnvelopeTests = []AudioTestCase{
	{
		Name: "Fast Attack White",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_WHITE},
			{Register: NOISE_ATK, Value: 1},
			{Register: NOISE_DEC, Value: 50},
			{Register: NOISE_SUS, Value: 200},
			{Register: NOISE_REL, Value: 20},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_env_attack_Fast_Attack_White.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			// Start the envelope (key on)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)
			attackSamples := captureAudio(t, filename)

			// Determine peak amplitude and time to peak
			var peakAmp float64
			var timeToPeak int
			for i, s := range attackSamples {
				amp := math.Abs(float64(s))
				if amp > peakAmp {
					peakAmp = amp
					timeToPeak = i
				}
			}

			// Analyze the shape of the envelope during attack
			attackShape := analyzeEnvelopeShape(attackSamples)

			// Capture sustain phase after waiting for decay duration
			time.Sleep(time.Duration(50) * time.Millisecond)
			sustainSamples := captureAudio(t, "noise_env_sustain_Fast_Attack_White.wav")
			var sustainSum float64
			for _, s := range sustainSamples {
				sustainSum += math.Abs(float64(s))
			}
			sustainLevel := sustainSum / float64(len(sustainSamples))

			// Release the note
			chip.HandleRegisterWrite(NOISE_CTRL, 1)
			time.Sleep(20 * time.Millisecond)
			releaseSamples := captureAudio(t, "noise_env_release_Fast_Attack_White.wav")

			// Determine release time (in samples) by finding when amplitude drops below 5% of peak
			var releaseTime int
			threshold := peakAmp * 0.05
			for i, s := range releaseSamples {
				if math.Abs(float64(s)) < threshold {
					releaseTime = i
					break
				}
			}

			// Store the results in a special format since we need multiple values
			// First element is time to peak, second is peak amplitude,
			// third is sustain level, fourth is release time, fifth is attack shape
			return []float32{
				float32(timeToPeak),
				float32(peakAmp),
				float32(sustainLevel),
				float32(releaseTime),
				float32(func() float64 {
					if attackShape == "linear" {
						return 1.0
					}
					return 2.0 // "exponential"
				}()),
			}
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Extract the values from our special format
			timeToPeak := float64(samples[0])
			peakAmp := float64(samples[1])
			sustainLevel := float64(samples[2])
			releaseTime := float64(samples[3])
			attackShape := func() string {
				if samples[4] == 1.0 {
					return "linear"
				}
				return "exponential"
			}()

			return map[string]float64{
				"timeToPeak":   timeToPeak,
				"peakAmp":      peakAmp,
				"sustainLevel": sustainLevel,
				"releaseTime":  releaseTime,
				"attackShape": func() float64 {
					if attackShape == "linear" {
						return 1.0
					}
					return 2.0
				}(),
			}
		},
		Expected: map[string]ExpectedMetric{
			"timeToPeak":   {Expected: 180, Tolerance: 180 * 0.2},
			"peakAmp":      {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainLevel": {Expected: 0.54, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseTime":  {Expected: 60, Tolerance: 60 * 0.2},
			"attackShape":  {Expected: 1.0, Tolerance: 0}, // 1.0 for linear
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			timeToPeak := metrics["timeToPeak"]
			peakAmp := metrics["peakAmp"]
			sustainLevel := metrics["sustainLevel"]
			releaseTime := metrics["releaseTime"]
			attackShape := func() string {
				if metrics["attackShape"] == 1.0 {
					return "linear"
				}
				return "exponential"
			}()

			t.Logf("Fast Attack White: attack=%dms peak=%.2f sustain=%.2f release=%dms shape=%s",
				int(timeToPeak)/44, peakAmp, sustainLevel, int(releaseTime)/44, attackShape)

			// Expected values
			expectedAttackTime := 180.0
			expectedPeakAmp := 1.0
			expectedSustainLevel := 0.54
			expectedReleaseTime := 60.0
			expectedShapeStr := "linear"

			// Check attack time
			attackTimeTolerance := expectedAttackTime * 0.2
			if math.Abs(timeToPeak-expectedAttackTime) > attackTimeTolerance {
				t.Errorf("Fast Attack White: Attack time error: got %d samples, expected %d ±%.0f",
					int(timeToPeak), int(expectedAttackTime), attackTimeTolerance)
			}

			// Check peak amplitude
			if math.Abs(peakAmp-expectedPeakAmp) > AMPLITUDE_TOLERANCE {
				t.Errorf("Fast Attack White: Peak amplitude error: got %.2f, expected %.2f",
					peakAmp, expectedPeakAmp)
			}

			// Check sustain level
			if math.Abs(sustainLevel-expectedSustainLevel) > AMPLITUDE_TOLERANCE {
				t.Errorf("Fast Attack White: Sustain level error: got %.2f, expected %.2f",
					sustainLevel, expectedSustainLevel)
			}

			// Check release time
			releaseTimeTolerance := expectedReleaseTime * 0.2
			if math.Abs(releaseTime-expectedReleaseTime) > releaseTimeTolerance {
				t.Errorf("Fast Attack White: Release time error: got %d samples, expected %d ±%.0f",
					int(releaseTime), int(expectedReleaseTime), releaseTimeTolerance)
			}

			// Check envelope shape
			if attackShape != expectedShapeStr {
				t.Errorf("Fast Attack White: Envelope shape error: got %s, expected %s",
					attackShape, expectedShapeStr)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Slow Attack Periodic",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: A4_FREQUENCY},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_PERIODIC},
			{Register: NOISE_ATK, Value: 100},
			{Register: NOISE_DEC, Value: 20},
			{Register: NOISE_SUS, Value: 180},
			{Register: NOISE_REL, Value: 10},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_env_attack_Slow_Attack_Periodic.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			// Start the envelope (key on)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)
			attackSamples := captureAudio(t, filename)

			// Determine peak amplitude and time to peak
			var peakAmp float64
			var timeToPeak int
			for i, s := range attackSamples {
				amp := math.Abs(float64(s))
				if amp > peakAmp {
					peakAmp = amp
					timeToPeak = i
				}
			}

			// Analyze the shape of the envelope during attack
			attackShape := analyzeEnvelopeShape(attackSamples)

			// Capture sustain phase after waiting for decay duration
			time.Sleep(time.Duration(20) * time.Millisecond)
			sustainSamples := captureAudio(t, "noise_env_sustain_Slow_Attack_Periodic.wav")
			var sustainSum float64
			for _, s := range sustainSamples {
				sustainSum += math.Abs(float64(s))
			}
			sustainLevel := sustainSum / float64(len(sustainSamples))

			// Release the note
			chip.HandleRegisterWrite(NOISE_CTRL, 1)
			time.Sleep(10 * time.Millisecond)
			releaseSamples := captureAudio(t, "noise_env_release_Slow_Attack_Periodic.wav")

			// Determine release time (in samples) by finding when amplitude drops below 5% of peak
			var releaseTime int
			threshold := peakAmp * 0.05
			for i, s := range releaseSamples {
				if math.Abs(float64(s)) < threshold {
					releaseTime = i
					break
				}
			}

			// Store the results in a special format
			return []float32{
				float32(timeToPeak),
				float32(peakAmp),
				float32(sustainLevel),
				float32(releaseTime),
				float32(func() float64 {
					if attackShape == "linear" {
						return 1.0
					}
					return 2.0 // "exponential"
				}()),
			}
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Extract the values from our special format
			timeToPeak := float64(samples[0])
			peakAmp := float64(samples[1])
			sustainLevel := float64(samples[2])
			releaseTime := float64(samples[3])
			attackShape := func() string {
				if samples[4] == 1.0 {
					return "linear"
				}
				return "exponential"
			}()

			return map[string]float64{
				"timeToPeak":   timeToPeak,
				"peakAmp":      peakAmp,
				"sustainLevel": sustainLevel,
				"releaseTime":  releaseTime,
				"attackShape": func() float64 {
					if attackShape == "linear" {
						return 1.0
					}
					return 2.0
				}(),
			}
		},
		Expected: map[string]ExpectedMetric{
			"timeToPeak":   {Expected: 4410, Tolerance: 4410 * 0.2},
			"peakAmp":      {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainLevel": {Expected: 0.6, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseTime":  {Expected: 210, Tolerance: 210 * 0.2},
			"attackShape":  {Expected: 1.0, Tolerance: 0}, // 1.0 for linear
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			timeToPeak := metrics["timeToPeak"]
			peakAmp := metrics["peakAmp"]
			sustainLevel := metrics["sustainLevel"]
			releaseTime := metrics["releaseTime"]
			attackShape := func() string {
				if metrics["attackShape"] == 1.0 {
					return "linear"
				}
				return "exponential"
			}()

			t.Logf("Slow Attack Periodic: attack=%dms peak=%.2f sustain=%.2f release=%dms shape=%s",
				int(timeToPeak)/44, peakAmp, sustainLevel, int(releaseTime)/44, attackShape)

			// Expected values
			expectedAttackTime := 4410.0
			expectedPeakAmp := 1.0
			expectedSustainLevel := 0.6
			expectedReleaseTime := 210.0
			expectedShapeStr := "linear"

			// Check attack time
			attackTimeTolerance := expectedAttackTime * 0.2
			if math.Abs(timeToPeak-expectedAttackTime) > attackTimeTolerance {
				t.Errorf("Slow Attack Periodic: Attack time error: got %d samples, expected %d ±%.0f",
					int(timeToPeak), int(expectedAttackTime), attackTimeTolerance)
			}

			// Check peak amplitude
			if math.Abs(peakAmp-expectedPeakAmp) > AMPLITUDE_TOLERANCE {
				t.Errorf("Slow Attack Periodic: Peak amplitude error: got %.2f, expected %.2f",
					peakAmp, expectedPeakAmp)
			}

			// Check sustain level
			if math.Abs(sustainLevel-expectedSustainLevel) > AMPLITUDE_TOLERANCE {
				t.Errorf("Slow Attack Periodic: Sustain level error: got %.2f, expected %.2f",
					sustainLevel, expectedSustainLevel)
			}

			// Check release time
			releaseTimeTolerance := expectedReleaseTime * 0.2
			if math.Abs(releaseTime-expectedReleaseTime) > releaseTimeTolerance {
				t.Errorf("Slow Attack Periodic: Release time error: got %d samples, expected %d ±%.0f",
					int(releaseTime), int(expectedReleaseTime), releaseTimeTolerance)
			}

			// Check envelope shape
			if attackShape != expectedShapeStr {
				t.Errorf("Slow Attack Periodic: Envelope shape error: got %s, expected %s",
					attackShape, expectedShapeStr)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Long Decay Metallic",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 2000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_METALLIC},
			{Register: NOISE_ATK, Value: 5},
			{Register: NOISE_DEC, Value: 200},
			{Register: NOISE_SUS, Value: 128},
			{Register: NOISE_REL, Value: 50},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_env_attack_Long_Decay_Metallic.wav",
		CaptureFunc: func(t *testing.T, filename string, duration time.Duration) []float32 {
			// Start the envelope (key on)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)
			attackSamples := captureAudio(t, filename)

			// Determine peak amplitude and time to peak
			var peakAmp float64
			var timeToPeak int
			for i, s := range attackSamples {
				amp := math.Abs(float64(s))
				if amp > peakAmp {
					peakAmp = amp
					timeToPeak = i
				}
			}

			// Analyze the shape of the envelope during attack
			attackShape := analyzeEnvelopeShape(attackSamples)

			// Capture sustain phase after waiting for decay duration
			time.Sleep(time.Duration(200) * time.Millisecond)
			sustainSamples := captureAudio(t, "noise_env_sustain_Long_Decay_Metallic.wav")
			var sustainSum float64
			for _, s := range sustainSamples {
				sustainSum += math.Abs(float64(s))
			}
			sustainLevel := sustainSum / float64(len(sustainSamples))

			// Release the note
			chip.HandleRegisterWrite(NOISE_CTRL, 1)
			time.Sleep(50 * time.Millisecond)
			releaseSamples := captureAudio(t, "noise_env_release_Long_Decay_Metallic.wav")

			// Determine release time (in samples) by finding when amplitude drops below 5% of peak
			var releaseTime int
			threshold := peakAmp * 0.05
			for i, s := range releaseSamples {
				if math.Abs(float64(s)) < threshold {
					releaseTime = i
					break
				}
			}

			// Store the results in a special format
			return []float32{
				float32(timeToPeak),
				float32(peakAmp),
				float32(sustainLevel),
				float32(releaseTime),
				float32(func() float64 {
					if attackShape == "linear" {
						return 1.0
					}
					return 2.0 // "exponential"
				}()),
			}
		},
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Extract the values from our special format
			timeToPeak := float64(samples[0])
			peakAmp := float64(samples[1])
			sustainLevel := float64(samples[2])
			releaseTime := float64(samples[3])
			attackShape := func() string {
				if samples[4] == 1.0 {
					return "linear"
				}
				return "exponential"
			}()

			return map[string]float64{
				"timeToPeak":   timeToPeak,
				"peakAmp":      peakAmp,
				"sustainLevel": sustainLevel,
				"releaseTime":  releaseTime,
				"attackShape": func() float64 {
					if attackShape == "linear" {
						return 1.0
					}
					return 2.0
				}(),
			}
		},
		Expected: map[string]ExpectedMetric{
			"timeToPeak":   {Expected: 260, Tolerance: 260 * 0.2},
			"peakAmp":      {Expected: 1.0, Tolerance: AMPLITUDE_TOLERANCE},
			"sustainLevel": {Expected: 0.27, Tolerance: AMPLITUDE_TOLERANCE},
			"releaseTime":  {Expected: 25, Tolerance: 25 * 0.2},
			"attackShape":  {Expected: 1.0, Tolerance: 0}, // 1.0 for linear
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			timeToPeak := metrics["timeToPeak"]
			peakAmp := metrics["peakAmp"]
			sustainLevel := metrics["sustainLevel"]
			releaseTime := metrics["releaseTime"]
			attackShape := func() string {
				if metrics["attackShape"] == 1.0 {
					return "linear"
				}
				return "exponential"
			}()

			t.Logf("Long Decay Metallic: attack=%dms peak=%.2f sustain=%.2f release=%dms shape=%s",
				int(timeToPeak)/44, peakAmp, sustainLevel, int(releaseTime)/44, attackShape)

			// Expected values
			expectedAttackTime := 260.0
			expectedPeakAmp := 1.0
			expectedSustainLevel := 0.27
			expectedReleaseTime := 25.0
			expectedShapeStr := "linear"

			// Check attack time
			attackTimeTolerance := expectedAttackTime * 0.2
			if math.Abs(timeToPeak-expectedAttackTime) > attackTimeTolerance {
				t.Errorf("Long Decay Metallic: Attack time error: got %d samples, expected %d ±%.0f",
					int(timeToPeak), int(expectedAttackTime), attackTimeTolerance)
			}

			// Check peak amplitude
			if math.Abs(peakAmp-expectedPeakAmp) > AMPLITUDE_TOLERANCE {
				t.Errorf("Long Decay Metallic: Peak amplitude error: got %.2f, expected %.2f",
					peakAmp, expectedPeakAmp)
			}

			// Check sustain level
			if math.Abs(sustainLevel-expectedSustainLevel) > AMPLITUDE_TOLERANCE {
				t.Errorf("Long Decay Metallic: Sustain level error: got %.2f, expected %.2f",
					sustainLevel, expectedSustainLevel)
			}

			// Check release time
			releaseTimeTolerance := expectedReleaseTime * 0.2
			if math.Abs(releaseTime-expectedReleaseTime) > releaseTimeTolerance {
				t.Errorf("Long Decay Metallic: Release time error: got %d samples, expected %d ±%.0f",
					int(releaseTime), int(expectedReleaseTime), releaseTimeTolerance)
			}

			// Check envelope shape
			if attackShape != expectedShapeStr {
				t.Errorf("Long Decay Metallic: Envelope shape error: got %s, expected %s",
					attackShape, expectedShapeStr)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
}

func TestNoiseEnvelope(t *testing.T) {
	for _, tc := range noiseEnvelopeTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var noiseModesTests = []AudioTestCase{
	{
		Name: "White Noise",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_WHITE},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_mode_White_Noise.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min

			// Calculate maximum autocorrelation
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var sum float64
				for i := 0; i < len(samples)-lag; i++ {
					sum += float64(samples[i] * samples[i+lag])
				}
				correlation := sum / float64(len(samples)-lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zeroCrossings := 0
			prev := samples[0]
			for i := 1; i < len(samples); i++ {
				if (prev < 0 && samples[i] >= 0) || (prev >= 0 && samples[i] < 0) {
					zeroCrossings++
				}
				prev = samples[i]
			}
			crossingRate := float64(zeroCrossings) / float64(len(samples))

			// Detect period (if any)
			period := detectPeriod(samples)

			return map[string]float64{
				"dynamicRange":   dynamicRange,
				"maxCorrelation": maxCorrelation,
				"crossingRate":   crossingRate,
				"period":         float64(period),
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			period := metrics["period"]

			t.Logf("White Noise: range=%.2f corr=%.2f cross=%.2f period=%d",
				dynamicRange, maxCorrelation, crossingRate, int(period))

			// Check dynamic range
			minExpectedRange := 0.8
			if dynamicRange < minExpectedRange {
				t.Errorf("White Noise: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for excessive repetition
			maxExpectedCorrelation := 0.9
			if maxCorrelation > maxExpectedCorrelation {
				t.Errorf("White Noise: Excessive correlation: got %.2f, expected <= %.2f",
					maxCorrelation, maxExpectedCorrelation)
			}

			// Check zero-crossing rate
			minExpectedCrossings := 0.0
			maxExpectedCrossings := 0.1
			if crossingRate < minExpectedCrossings || crossingRate > maxExpectedCrossings {
				t.Errorf("White Noise: Zero-crossing rate out of range: got %.2f, expected %.2f-%.2f",
					crossingRate, minExpectedCrossings, maxExpectedCrossings)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Periodic Low",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: A4_FREQUENCY},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_PERIODIC},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_mode_Periodic_Low.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min

			// Calculate maximum autocorrelation
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var sum float64
				for i := 0; i < len(samples)-lag; i++ {
					sum += float64(samples[i] * samples[i+lag])
				}
				correlation := sum / float64(len(samples)-lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zeroCrossings := 0
			prev := samples[0]
			for i := 1; i < len(samples); i++ {
				if (prev < 0 && samples[i] >= 0) || (prev >= 0 && samples[i] < 0) {
					zeroCrossings++
				}
				prev = samples[i]
			}
			crossingRate := float64(zeroCrossings) / float64(len(samples))

			// Detect period (if any)
			period := detectPeriod(samples)

			return map[string]float64{
				"dynamicRange":   dynamicRange,
				"maxCorrelation": maxCorrelation,
				"crossingRate":   crossingRate,
				"period":         float64(period),
				"expectedRate":   math.Min(float64(A4_FREQUENCY)/SAMPLE_RATE, 0.5),
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			period := metrics["period"]
			expectedRate := metrics["expectedRate"]

			t.Logf("Periodic Low: range=%.2f corr=%.2f cross=%.2f period=%d",
				dynamicRange, maxCorrelation, crossingRate, int(period))

			// Check dynamic range
			minExpectedRange := 0.6
			if dynamicRange < minExpectedRange {
				t.Errorf("Periodic Low: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for expected periodicity
			maxExpectedCorrelation := 0.95
			if maxCorrelation > maxExpectedCorrelation {
				t.Errorf("Periodic Low: Excessive correlation: got %.2f, expected <= %.2f",
					maxCorrelation, maxExpectedCorrelation)
			}

			// Check zero-crossing rate
			minExpectedCrossings := 0.0
			maxExpectedCrossings := 0.1
			if crossingRate < minExpectedCrossings || crossingRate > maxExpectedCrossings {
				t.Errorf("Periodic Low: Zero-crossing rate out of range: got %.2f, expected %.2f-%.2f",
					crossingRate, minExpectedCrossings, maxExpectedCrossings)
			}

			// Cross-check with expected frequency rate
			crossingTolerance := 0.2
			if math.Abs(crossingRate-expectedRate) > crossingTolerance {
				t.Errorf("Periodic Low: Unexpected zero-crossing rate: got %.2f, expected %.2f ±%.2f",
					crossingRate, expectedRate, crossingTolerance)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Periodic High",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 2000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_PERIODIC},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_mode_Periodic_High.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min

			// Calculate maximum autocorrelation
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var sum float64
				for i := 0; i < len(samples)-lag; i++ {
					sum += float64(samples[i] * samples[i+lag])
				}
				correlation := sum / float64(len(samples)-lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zeroCrossings := 0
			prev := samples[0]
			for i := 1; i < len(samples); i++ {
				if (prev < 0 && samples[i] >= 0) || (prev >= 0 && samples[i] < 0) {
					zeroCrossings++
				}
				prev = samples[i]
			}
			crossingRate := float64(zeroCrossings) / float64(len(samples))

			// Detect period (if any)
			period := detectPeriod(samples)

			return map[string]float64{
				"dynamicRange":   dynamicRange,
				"maxCorrelation": maxCorrelation,
				"crossingRate":   crossingRate,
				"period":         float64(period),
				"expectedRate":   math.Min(float64(2000)/SAMPLE_RATE, 0.5),
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			period := metrics["period"]
			expectedRate := metrics["expectedRate"]

			t.Logf("Periodic High: range=%.2f corr=%.2f cross=%.2f period=%d",
				dynamicRange, maxCorrelation, crossingRate, int(period))

			// Check dynamic range
			minExpectedRange := 0.6
			if dynamicRange < minExpectedRange {
				t.Errorf("Periodic High: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for expected periodicity
			maxExpectedCorrelation := 0.9
			if maxCorrelation > maxExpectedCorrelation {
				t.Errorf("Periodic High: Excessive correlation: got %.2f, expected <= %.2f",
					maxCorrelation, maxExpectedCorrelation)
			}

			// Check zero-crossing rate
			minExpectedCrossings := 0.0
			maxExpectedCrossings := 0.1
			if crossingRate < minExpectedCrossings || crossingRate > maxExpectedCrossings {
				t.Errorf("Periodic High: Zero-crossing rate out of range: got %.2f, expected %.2f-%.2f",
					crossingRate, minExpectedCrossings, maxExpectedCrossings)
			}

			// Cross-check with expected frequency rate
			crossingTolerance := 0.2
			if math.Abs(crossingRate-expectedRate) > crossingTolerance {
				t.Errorf("Periodic High: Unexpected zero-crossing rate: got %.2f, expected %.2f ±%.2f",
					crossingRate, expectedRate, crossingTolerance)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Metallic Low",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: A4_FREQUENCY},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_METALLIC},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_mode_Metallic_Low.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min

			// Calculate maximum autocorrelation
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var sum float64
				for i := 0; i < len(samples)-lag; i++ {
					sum += float64(samples[i] * samples[i+lag])
				}
				correlation := sum / float64(len(samples)-lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zeroCrossings := 0
			prev := samples[0]
			for i := 1; i < len(samples); i++ {
				if (prev < 0 && samples[i] >= 0) || (prev >= 0 && samples[i] < 0) {
					zeroCrossings++
				}
				prev = samples[i]
			}
			crossingRate := float64(zeroCrossings) / float64(len(samples))

			// Detect period (if any)
			period := detectPeriod(samples)

			// Additional analysis for metallic character
			var spectralSum float64
			for i := 1; i < len(samples); i++ {
				spectralSum += math.Abs(float64(samples[i] - samples[i-1]))
			}
			spectralDensity := spectralSum / float64(len(samples))

			return map[string]float64{
				"dynamicRange":    dynamicRange,
				"maxCorrelation":  maxCorrelation,
				"crossingRate":    crossingRate,
				"period":          float64(period),
				"expectedRate":    math.Min(float64(A4_FREQUENCY)/SAMPLE_RATE, 0.5),
				"spectralDensity": spectralDensity,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			period := metrics["period"]
			spectralDensity := metrics["spectralDensity"]

			t.Logf("Metallic Low: range=%.2f corr=%.2f cross=%.2f period=%d density=%.3f",
				dynamicRange, maxCorrelation, crossingRate, int(period), spectralDensity)

			// Check dynamic range
			minExpectedRange := 0.7
			if dynamicRange < minExpectedRange {
				t.Errorf("Metallic Low: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for expected periodicity
			maxExpectedCorrelation := 0.9
			if maxCorrelation > maxExpectedCorrelation {
				t.Errorf("Metallic Low: Excessive correlation: got %.2f, expected <= %.2f",
					maxCorrelation, maxExpectedCorrelation)
			}

			// Check zero-crossing rate
			minExpectedCrossings := 0.0
			maxExpectedCrossings := 0.1
			if crossingRate < minExpectedCrossings || crossingRate > maxExpectedCrossings {
				t.Errorf("Metallic Low: Zero-crossing rate out of range: got %.2f, expected %.2f-%.2f",
					crossingRate, minExpectedCrossings, maxExpectedCrossings)
			}

			// Check metallic character
			if spectralDensity < MIN_METALLIC_DENSITY {
				t.Errorf("Metallic Low: Insufficient metallic character: density = %.3f",
					spectralDensity)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
	{
		Name: "Metallic High",
		PreSetup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 2000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_MODE, Value: NOISE_MODE_METALLIC},
			{Register: NOISE_CTRL, Value: 3},
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "noise_mode_Metallic_High.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Calculate dynamic range
			var min, max float64 = 1.0, -1.0
			for _, s := range samples {
				v := float64(s)
				min = math.Min(min, v)
				max = math.Max(max, v)
			}
			dynamicRange := max - min

			// Calculate maximum autocorrelation
			var maxCorrelation float64
			for lag := 1; lag < len(samples)/4; lag++ {
				var sum float64
				for i := 0; i < len(samples)-lag; i++ {
					sum += float64(samples[i] * samples[i+lag])
				}
				correlation := sum / float64(len(samples)-lag)
				maxCorrelation = math.Max(maxCorrelation, math.Abs(correlation))
			}

			// Calculate zero-crossing rate
			zeroCrossings := 0
			prev := samples[0]
			for i := 1; i < len(samples); i++ {
				if (prev < 0 && samples[i] >= 0) || (prev >= 0 && samples[i] < 0) {
					zeroCrossings++
				}
				prev = samples[i]
			}
			crossingRate := float64(zeroCrossings) / float64(len(samples))

			// Detect period (if any)
			period := detectPeriod(samples)

			// Additional analysis for metallic character
			var spectralSum float64
			for i := 1; i < len(samples); i++ {
				spectralSum += math.Abs(float64(samples[i] - samples[i-1]))
			}
			spectralDensity := spectralSum / float64(len(samples))

			return map[string]float64{
				"dynamicRange":    dynamicRange,
				"maxCorrelation":  maxCorrelation,
				"crossingRate":    crossingRate,
				"period":          float64(period),
				"expectedRate":    math.Min(float64(2000)/SAMPLE_RATE, 0.5),
				"spectralDensity": spectralDensity,
			}
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			dynamicRange := metrics["dynamicRange"]
			maxCorrelation := metrics["maxCorrelation"]
			crossingRate := metrics["crossingRate"]
			period := metrics["period"]
			spectralDensity := metrics["spectralDensity"]

			t.Logf("Metallic High: range=%.2f corr=%.2f cross=%.2f period=%d density=%.3f",
				dynamicRange, maxCorrelation, crossingRate, int(period), spectralDensity)

			// Check dynamic range
			minExpectedRange := 0.7
			if dynamicRange < minExpectedRange {
				t.Errorf("Metallic High: Insufficient dynamic range: got %.2f, expected >= %.2f",
					dynamicRange, minExpectedRange)
			}

			// Check for expected periodicity
			maxExpectedCorrelation := 0.9
			if maxCorrelation > maxExpectedCorrelation {
				t.Errorf("Metallic High: Excessive correlation: got %.2f, expected <= %.2f",
					maxCorrelation, maxExpectedCorrelation)
			}

			// Check zero-crossing rate
			minExpectedCrossings := 0.0
			maxExpectedCrossings := 0.1
			if crossingRate < minExpectedCrossings || crossingRate > maxExpectedCrossings {
				t.Errorf("Metallic High: Zero-crossing rate out of range: got %.2f, expected %.2f-%.2f",
					crossingRate, minExpectedCrossings, maxExpectedCrossings)
			}

			// Check metallic character
			if spectralDensity < MIN_METALLIC_DENSITY {
				t.Errorf("Metallic High: Insufficient metallic character: density = %.3f",
					spectralDensity)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
	},
}

func TestNoiseModes(t *testing.T) {
	for _, tc := range noiseModesTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

// Global Effects and Processing Tests
type channelConfig struct {
	ctrl uint32
	freq uint32
	vol  uint32
	duty uint32 // Only used for square waves.
}

var globalFilterLowPassTests = []AudioTestCase{
	{
		Name: "Square Wave Basic",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "lp_unfiltered_Square_Wave_Basic.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Square Wave Basic.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, FILTER_TYPE_LOWPASS)
			chip.HandleRegisterWrite(FILTER_CUTOFF, FILTER_CUTOFF_MID_LOW)
			chip.HandleRegisterWrite(FILTER_RESONANCE, FILTER_RESONANCE_MEDIUM)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "lp_filtered_Square_Wave_Basic.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Square Wave Basic.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Analyze waveforms
			unfilteredResult := analyseWaveform(nil, unfiltered)
			filteredResult := analyseWaveform(nil, samples)

			// Calculate energy in each signal
			var unfilteredEnergy, filteredEnergy float64
			for i := range unfiltered {
				unfilteredEnergy += float64(unfiltered[i] * unfiltered[i])
				filteredEnergy += float64(samples[i] * samples[i])
			}
			attenuation := 1.0 - (filteredEnergy / unfilteredEnergy)

			// Check for resonance peak
			resonancePeak := 0.0
			if filteredResult.amplitude > 0 {
				resonancePeak = findResonancePeak(samples)
			}

			cutoffFreq := float64(64) / 255.0 * MAX_FILTER_FREQ

			return map[string]float64{
				"attenuation":      attenuation,
				"unfilteredAmp":    unfilteredResult.amplitude,
				"filteredAmp":      filteredResult.amplitude,
				"unfilteredFreq":   unfilteredResult.frequency,
				"filteredFreq":     filteredResult.frequency,
				"freqRatio":        filteredResult.frequency / unfilteredResult.frequency,
				"resonancePeak":    resonancePeak,
				"cutoffFreq":       cutoffFreq,
				"unfilteredEnergy": unfilteredEnergy,
				"filteredEnergy":   filteredEnergy,
			}
		},
		Expected: map[string]ExpectedMetric{
			"attenuation": {Expected: 0.6, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			attenuation := metrics["attenuation"]
			unfilteredAmp := metrics["unfilteredAmp"]
			filteredAmp := metrics["filteredAmp"]
			freqRatio := metrics["freqRatio"]
			resonancePeak := metrics["resonancePeak"]
			cutoffFreq := metrics["cutoffFreq"]

			t.Logf("Square Wave Basic: attenuation=%.2f freq_ratio=%.2f resonance=%.2f cutoff=%.1fHz",
				attenuation, freqRatio, resonancePeak, cutoffFreq)

			// Validate expected attenuation
			expectedAtten := 0.6
			if math.Abs(attenuation-expectedAtten) > 0.2 {
				t.Errorf("Square Wave Basic: Incorrect attenuation: got %.2f, expected %.2f",
					attenuation, expectedAtten)
			}

			// Ensure the filter does not boost amplitude
			if filteredAmp > unfilteredAmp {
				t.Errorf("Square Wave Basic: Filter increased amplitude: %.2f > %.2f",
					filteredAmp, unfilteredAmp)
			}

			// Verify resonance behavior
			if resonancePeak > 1.2 {
				t.Logf("Square Wave Basic: Resonance peak detected: %.2f", resonancePeak)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Square Wave Basic.unfiltered")
		},
	},
	{
		Name: "Triangle High Frequency",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: 1760},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "lp_unfiltered_Triangle_High_Frequency.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Triangle High Frequency.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 1) // Low-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 32)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 192)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "lp_filtered_Triangle_High_Frequency.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Triangle High Frequency.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Analyze waveforms
			unfilteredResult := analyseWaveform(nil, unfiltered)
			filteredResult := analyseWaveform(nil, samples)

			// Calculate energy in each signal
			var unfilteredEnergy, filteredEnergy float64
			for i := range unfiltered {
				unfilteredEnergy += float64(unfiltered[i] * unfiltered[i])
				filteredEnergy += float64(samples[i] * samples[i])
			}
			attenuation := 1.0 - (filteredEnergy / unfilteredEnergy)

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			cutoffFreq := float64(32) / 255.0 * MAX_FILTER_FREQ

			return map[string]float64{
				"attenuation":      attenuation,
				"unfilteredAmp":    unfilteredResult.amplitude,
				"filteredAmp":      filteredResult.amplitude,
				"unfilteredFreq":   unfilteredResult.frequency,
				"filteredFreq":     filteredResult.frequency,
				"freqRatio":        filteredResult.frequency / unfilteredResult.frequency,
				"resonancePeak":    resonancePeak,
				"cutoffFreq":       cutoffFreq,
				"unfilteredEnergy": unfilteredEnergy,
				"filteredEnergy":   filteredEnergy,
			}
		},
		Expected: map[string]ExpectedMetric{
			"attenuation": {Expected: 0.8, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			attenuation := metrics["attenuation"]
			unfilteredAmp := metrics["unfilteredAmp"]
			filteredAmp := metrics["filteredAmp"]
			freqRatio := metrics["freqRatio"]
			resonancePeak := metrics["resonancePeak"]
			cutoffFreq := metrics["cutoffFreq"]

			t.Logf("Triangle High Frequency: attenuation=%.2f freq_ratio=%.2f resonance=%.2f cutoff=%.1fHz",
				attenuation, freqRatio, resonancePeak, cutoffFreq)

			// Validate expected attenuation
			expectedAtten := 0.8
			if math.Abs(attenuation-expectedAtten) > 0.2 {
				t.Errorf("Triangle High Frequency: Incorrect attenuation: got %.2f, expected %.2f",
					attenuation, expectedAtten)
			}

			// Ensure the filter does not boost amplitude
			if filteredAmp > unfilteredAmp {
				t.Errorf("Triangle High Frequency: Filter increased amplitude: %.2f > %.2f",
					filteredAmp, unfilteredAmp)
			}

			// Verify resonance behavior - with high resonance setting, should see a peak
			if resonancePeak < 1.2 {
				t.Errorf("Triangle High Frequency: Insufficient resonance peak: %.2f", resonancePeak)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Triangle High Frequency.unfiltered")
		},
	},
	{
		Name: "Sine Wave Verification",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "lp_unfiltered_Sine_Wave_Verification.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Sine Wave Verification.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 1) // Low-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 128)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 64)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "lp_filtered_Sine_Wave_Verification.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Sine Wave Verification.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Analyze waveforms
			unfilteredResult := analyseWaveform(nil, unfiltered)
			filteredResult := analyseWaveform(nil, samples)

			// Calculate energy in each signal
			var unfilteredEnergy, filteredEnergy float64
			for i := range unfiltered {
				unfilteredEnergy += float64(unfiltered[i] * unfiltered[i])
				filteredEnergy += float64(samples[i] * samples[i])
			}
			attenuation := 1.0 - (filteredEnergy / unfilteredEnergy)

			cutoffFreq := float64(128) / 255.0 * MAX_FILTER_FREQ

			return map[string]float64{
				"attenuation":      attenuation,
				"unfilteredAmp":    unfilteredResult.amplitude,
				"filteredAmp":      filteredResult.amplitude,
				"unfilteredFreq":   unfilteredResult.frequency,
				"filteredFreq":     filteredResult.frequency,
				"freqRatio":        filteredResult.frequency / unfilteredResult.frequency,
				"cutoffFreq":       cutoffFreq,
				"unfilteredEnergy": unfilteredEnergy,
				"filteredEnergy":   filteredEnergy,
			}
		},
		Expected: map[string]ExpectedMetric{
			"attenuation": {Expected: 0.3, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			attenuation := metrics["attenuation"]
			unfilteredAmp := metrics["unfilteredAmp"]
			filteredAmp := metrics["filteredAmp"]
			freqRatio := metrics["freqRatio"]
			cutoffFreq := metrics["cutoffFreq"]

			t.Logf("Sine Wave Verification: attenuation=%.2f freq_ratio=%.2f cutoff=%.1fHz",
				attenuation, freqRatio, cutoffFreq)

			// Validate expected attenuation
			expectedAtten := 0.3
			if math.Abs(attenuation-expectedAtten) > 0.2 {
				t.Errorf("Sine Wave Verification: Incorrect attenuation: got %.2f, expected %.2f",
					attenuation, expectedAtten)
			}

			// Ensure the filter does not boost amplitude
			if filteredAmp > unfilteredAmp {
				t.Errorf("Sine Wave Verification: Filter increased amplitude: %.2f > %.2f",
					filteredAmp, unfilteredAmp)
			}

			// For sine waves, the frequency should be 440Hz, which is below the cutoff
			// So there should not be excessive attenuation
			if filteredAmp < 0.5*unfilteredAmp {
				t.Errorf("Sine Wave Verification: Excessive attenuation below cutoff frequency")
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Sine Wave Verification.unfiltered")
		},
	},
	{
		Name: "Noise Filtering",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "lp_unfiltered_Noise_Filtering.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Noise Filtering.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 1) // Low-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 32)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 255)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "lp_filtered_Noise_Filtering.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Noise Filtering.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Calculate RMS levels for both signals
			var unfilteredRMS, filteredRMS float64
			for i := range unfiltered {
				unfilteredRMS += float64(unfiltered[i] * unfiltered[i])
				filteredRMS += float64(samples[i] * samples[i])
			}
			unfilteredRMS = math.Sqrt(unfilteredRMS / float64(len(unfiltered)))
			filteredRMS = math.Sqrt(filteredRMS / float64(len(samples)))

			attenuation := 1.0 - (filteredRMS / unfilteredRMS)

			// Check for resonance peak with max resonance
			resonancePeak := findResonancePeak(samples)

			cutoffFreq := float64(32) / 255.0 * MAX_FILTER_FREQ

			return map[string]float64{
				"attenuation":   attenuation,
				"unfilteredRMS": unfilteredRMS,
				"filteredRMS":   filteredRMS,
				"resonancePeak": resonancePeak,
				"cutoffFreq":    cutoffFreq,
			}
		},
		Expected: map[string]ExpectedMetric{
			"attenuation": {Expected: 0.7, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			attenuation := metrics["attenuation"]
			unfilteredRMS := metrics["unfilteredRMS"]
			filteredRMS := metrics["filteredRMS"]
			resonancePeak := metrics["resonancePeak"]
			cutoffFreq := metrics["cutoffFreq"]

			t.Logf("Noise Filtering: attenuation=%.2f unfilteredRMS=%.2f filteredRMS=%.2f resonance=%.2f cutoff=%.1fHz",
				attenuation, unfilteredRMS, filteredRMS, resonancePeak, cutoffFreq)

			// Validate expected attenuation
			expectedAtten := 0.7
			if math.Abs(attenuation-expectedAtten) > 0.2 {
				t.Errorf("Noise Filtering: Incorrect attenuation: got %.2f, expected %.2f",
					attenuation, expectedAtten)
			}

			// Ensure the filter does not boost amplitude
			if filteredRMS > unfilteredRMS {
				t.Errorf("Noise Filtering: Filter increased RMS level: %.2f > %.2f",
					filteredRMS, unfilteredRMS)
			}

			// Verify resonance behavior - with max resonance, should see a significant peak
			expectedResonancePeak := 1.0 + float64(255)/255.0
			if resonancePeak < expectedResonancePeak*0.8 {
				t.Errorf("Noise Filtering: Insufficient resonance peak: %.2f, expected >= %.2f",
					resonancePeak, expectedResonancePeak*0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Noise Filtering.unfiltered")
		},
	},
	{
		Name: "All Channels Combined",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},

			{Register: TRI_FREQ, Value: 660},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			{Register: SINE_FREQ, Value: A5_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "lp_unfiltered_All_Channels_Combined.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("All Channels Combined.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 1) // Low-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 64)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "lp_filtered_All_Channels_Combined.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("All Channels Combined.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Calculate energy in each signal
			var unfilteredEnergy, filteredEnergy float64
			for i := range unfiltered {
				unfilteredEnergy += float64(unfiltered[i] * unfiltered[i])
				filteredEnergy += float64(samples[i] * samples[i])
			}
			attenuation := 1.0 - (filteredEnergy / unfilteredEnergy)

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			cutoffFreq := float64(64) / 255.0 * MAX_FILTER_FREQ

			return map[string]float64{
				"attenuation":      attenuation,
				"resonancePeak":    resonancePeak,
				"cutoffFreq":       cutoffFreq,
				"unfilteredEnergy": unfilteredEnergy,
				"filteredEnergy":   filteredEnergy,
			}
		},
		Expected: map[string]ExpectedMetric{
			"attenuation": {Expected: 0.65, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			attenuation := metrics["attenuation"]
			resonancePeak := metrics["resonancePeak"]
			cutoffFreq := metrics["cutoffFreq"]

			t.Logf("All Channels Combined: attenuation=%.2f resonance=%.2f cutoff=%.1fHz",
				attenuation, resonancePeak, cutoffFreq)

			// Validate expected attenuation
			expectedAtten := 0.65
			if math.Abs(attenuation-expectedAtten) > 0.2 {
				t.Errorf("All Channels Combined: Incorrect attenuation: got %.2f, expected %.2f",
					attenuation, expectedAtten)
			}

			// Verify energy reduction
			if metrics["filteredEnergy"] > metrics["unfilteredEnergy"] {
				t.Errorf("All Channels Combined: Filter increased energy: %.2f > %.2f",
					metrics["filteredEnergy"], metrics["unfilteredEnergy"])
			}

			// Verify filter behavior for multiple channels maintains characteristics
			expectedResonancePeak := 1.0 + float64(128)/255.0
			if resonancePeak < expectedResonancePeak*0.7 {
				t.Errorf("All Channels Combined: Insufficient resonance peak: %.2f, expected >= %.2f",
					resonancePeak, expectedResonancePeak*0.7)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("All Channels Combined.unfiltered")
		},
	},
}

func TestGlobalFilterLowPass(t *testing.T) {
	for _, tc := range globalFilterLowPassTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var globalFilterHighPassTests = []AudioTestCase{
	{
		Name: "Square Low Freq",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A2_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "hp_unfiltered_Square_Low_Freq.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Square Low Freq.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 2) // High-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 200)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "hp_filtered_Square_Low_Freq.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Square Low Freq.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Define cutoffBin: index below which frequencies are considered low
			cutoffFreq := float64(200) / 255.0 * MAX_FILTER_FREQ
			cutoffBin := int(cutoffFreq / (SAMPLE_RATE / 2) * float64(len(samples)) / 2)

			// Calculate spectral energy in low and high frequency bands
			var unfilteredLowEnergy, unfilteredHighEnergy float64
			var filteredLowEnergy, filteredHighEnergy float64

			for i := 0; i < len(samples); i++ {
				if i < cutoffBin {
					unfilteredLowEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredLowEnergy += float64(samples[i] * samples[i])
				} else {
					unfilteredHighEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredHighEnergy += float64(samples[i] * samples[i])
				}
			}

			// Calculate attenuation metrics
			lowFreqAttenuation := 1.0 - (filteredLowEnergy / unfilteredLowEnergy)
			highFreqAmplitude := filteredHighEnergy / unfilteredHighEnergy

			// Analyze waveforms
			unfilteredResult := analyseWaveform(nil, unfiltered)
			filteredResult := analyseWaveform(nil, samples)

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"lowFreqAttenuation": lowFreqAttenuation,
				"highFreqAmplitude":  highFreqAmplitude,
				"unfilteredAmp":      unfilteredResult.amplitude,
				"filteredAmp":        filteredResult.amplitude,
				"unfilteredFreq":     unfilteredResult.frequency,
				"filteredFreq":       filteredResult.frequency,
				"freqRatio":          filteredResult.frequency / unfilteredResult.frequency,
				"cutoffFreq":         cutoffFreq,
				"resonancePeak":      resonancePeak,
			}
		},
		Expected: map[string]ExpectedMetric{
			"lowFreqAttenuation": {Expected: 0.7, Tolerance: 0.2},
			"highFreqAmplitude":  {Expected: 0.6, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			lowFreqAttenuation := metrics["lowFreqAttenuation"]
			highFreqAmplitude := metrics["highFreqAmplitude"]
			cutoffFreq := metrics["cutoffFreq"]
			resonancePeak := metrics["resonancePeak"]
			inputFreq := metrics["unfilteredFreq"]

			t.Logf("Square Low Freq: low_atten=%.2f high_amp=%.2f freq=%.1f cutoff=%.1f resonance=%.2f",
				lowFreqAttenuation, highFreqAmplitude, inputFreq, cutoffFreq, resonancePeak)

			// Validate expected low frequency attenuation
			expectedAtten := 0.7
			if math.Abs(lowFreqAttenuation-expectedAtten) > 0.2 {
				t.Errorf("Square Low Freq: Incorrect low frequency attenuation: got %.2f, expected %.2f",
					lowFreqAttenuation, expectedAtten)
			}

			// Validate high frequency preservation
			minHighFreqAmp := 0.6
			if highFreqAmplitude < minHighFreqAmp {
				t.Errorf("Square Low Freq: Excessive high frequency attenuation: amp=%.2f, min=%.2f",
					highFreqAmplitude, minHighFreqAmp)
			}

			// Verify resonance behavior
			expectedPeak := 1.0 + float64(128)/255.0
			if resonancePeak < expectedPeak*0.8 {
				t.Errorf("Square Low Freq: Insufficient resonance peak: %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Square Low Freq.unfiltered")
		},
	},
	{
		Name: "Triangle Mixed",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "hp_unfiltered_Triangle_Mixed.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Triangle Mixed.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 2) // High-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 100)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 192)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "hp_filtered_Triangle_Mixed.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Triangle Mixed.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Define cutoffBin: index below which frequencies are considered low
			cutoffFreq := float64(100) / 255.0 * MAX_FILTER_FREQ
			cutoffBin := int(cutoffFreq / (SAMPLE_RATE / 2) * float64(len(samples)) / 2)

			// Calculate spectral energy in low and high frequency bands
			var unfilteredLowEnergy, unfilteredHighEnergy float64
			var filteredLowEnergy, filteredHighEnergy float64

			for i := 0; i < len(samples); i++ {
				if i < cutoffBin {
					unfilteredLowEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredLowEnergy += float64(samples[i] * samples[i])
				} else {
					unfilteredHighEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredHighEnergy += float64(samples[i] * samples[i])
				}
			}

			// Calculate attenuation metrics
			lowFreqAttenuation := 1.0 - (filteredLowEnergy / unfilteredLowEnergy)
			highFreqAmplitude := filteredHighEnergy / unfilteredHighEnergy

			// Analyze waveforms
			unfilteredResult := analyseWaveform(nil, unfiltered)
			filteredResult := analyseWaveform(nil, samples)

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"lowFreqAttenuation": lowFreqAttenuation,
				"highFreqAmplitude":  highFreqAmplitude,
				"unfilteredAmp":      unfilteredResult.amplitude,
				"filteredAmp":        filteredResult.amplitude,
				"unfilteredFreq":     unfilteredResult.frequency,
				"filteredFreq":       filteredResult.frequency,
				"freqRatio":          filteredResult.frequency / unfilteredResult.frequency,
				"cutoffFreq":         cutoffFreq,
				"resonancePeak":      resonancePeak,
			}
		},
		Expected: map[string]ExpectedMetric{
			"lowFreqAttenuation": {Expected: 0.3, Tolerance: 0.2},
			"highFreqAmplitude":  {Expected: 0.8, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			lowFreqAttenuation := metrics["lowFreqAttenuation"]
			highFreqAmplitude := metrics["highFreqAmplitude"]
			cutoffFreq := metrics["cutoffFreq"]
			resonancePeak := metrics["resonancePeak"]
			inputFreq := metrics["unfilteredFreq"]

			t.Logf("Triangle Mixed: low_atten=%.2f high_amp=%.2f freq=%.1f cutoff=%.1f resonance=%.2f",
				lowFreqAttenuation, highFreqAmplitude, inputFreq, cutoffFreq, resonancePeak)

			// Validate expected low frequency attenuation
			expectedAtten := 0.3
			if math.Abs(lowFreqAttenuation-expectedAtten) > 0.2 {
				t.Errorf("Triangle Mixed: Incorrect low frequency attenuation: got %.2f, expected %.2f",
					lowFreqAttenuation, expectedAtten)
			}

			// Validate high frequency preservation
			minHighFreqAmp := 0.8
			if highFreqAmplitude < minHighFreqAmp {
				t.Errorf("Triangle Mixed: Excessive high frequency attenuation: amp=%.2f, min=%.2f",
					highFreqAmplitude, minHighFreqAmp)
			}

			// Verify resonance behavior
			expectedPeak := 1.0 + float64(192)/255.0
			if resonancePeak < expectedPeak*0.8 {
				t.Errorf("Triangle Mixed: Insufficient resonance peak: %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Triangle Mixed.unfiltered")
		},
	},
	{
		Name: "Sine Low to High",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: 2000},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "hp_unfiltered_Sine_Low_to_High.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Sine Low to High.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 2) // High-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 150)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 64)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "hp_filtered_Sine_Low_to_High.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Sine Low to High.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Define cutoffBin: index below which frequencies are considered low
			cutoffFreq := float64(150) / 255.0 * MAX_FILTER_FREQ
			cutoffBin := int(cutoffFreq / (SAMPLE_RATE / 2) * float64(len(samples)) / 2)

			// Calculate spectral energy in low and high frequency bands
			var unfilteredLowEnergy, unfilteredHighEnergy float64
			var filteredLowEnergy, filteredHighEnergy float64

			for i := 0; i < len(samples); i++ {
				if i < cutoffBin {
					unfilteredLowEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredLowEnergy += float64(samples[i] * samples[i])
				} else {
					unfilteredHighEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredHighEnergy += float64(samples[i] * samples[i])
				}
			}

			// Calculate attenuation metrics
			lowFreqAttenuation := 1.0 - (filteredLowEnergy / unfilteredLowEnergy)
			highFreqAmplitude := filteredHighEnergy / unfilteredHighEnergy

			// Analyze waveforms
			unfilteredResult := analyseWaveform(nil, unfiltered)
			filteredResult := analyseWaveform(nil, samples)

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"lowFreqAttenuation": lowFreqAttenuation,
				"highFreqAmplitude":  highFreqAmplitude,
				"unfilteredAmp":      unfilteredResult.amplitude,
				"filteredAmp":        filteredResult.amplitude,
				"unfilteredFreq":     unfilteredResult.frequency,
				"filteredFreq":       filteredResult.frequency,
				"freqRatio":          filteredResult.frequency / unfilteredResult.frequency,
				"cutoffFreq":         cutoffFreq,
				"resonancePeak":      resonancePeak,
			}
		},
		Expected: map[string]ExpectedMetric{
			"lowFreqAttenuation": {Expected: 0.2, Tolerance: 0.2},
			"highFreqAmplitude":  {Expected: 0.9, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			lowFreqAttenuation := metrics["lowFreqAttenuation"]
			highFreqAmplitude := metrics["highFreqAmplitude"]
			cutoffFreq := metrics["cutoffFreq"]
			resonancePeak := metrics["resonancePeak"]
			inputFreq := metrics["unfilteredFreq"]

			t.Logf("Sine Low to High: low_atten=%.2f high_amp=%.2f freq=%.1f cutoff=%.1f resonance=%.2f",
				lowFreqAttenuation, highFreqAmplitude, inputFreq, cutoffFreq, resonancePeak)

			// Validate expected low frequency attenuation
			expectedAtten := 0.2
			if math.Abs(lowFreqAttenuation-expectedAtten) > 0.2 {
				t.Errorf("Sine Low to High: Incorrect low frequency attenuation: got %.2f, expected %.2f",
					lowFreqAttenuation, expectedAtten)
			}

			// Validate high frequency preservation
			minHighFreqAmp := 0.9
			if highFreqAmplitude < minHighFreqAmp {
				t.Errorf("Sine Low to High: Excessive high frequency attenuation: amp=%.2f, min=%.2f",
					highFreqAmplitude, minHighFreqAmp)
			}

			// For sine wave, the frequency is well above cutoff, so there should be minimal attenuation
			if inputFreq > cutoffFreq*2 && metrics["filteredAmp"] < 0.7*metrics["unfilteredAmp"] {
				t.Errorf("Sine Low to High: Excessive attenuation above cutoff for %.1f Hz", inputFreq)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Sine Low to High.unfiltered")
		},
	},
	{
		Name: "Noise Filter",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 500},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "hp_unfiltered_Noise_Filter.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Noise Filter.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 2) // High-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 180)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 255)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "hp_filtered_Noise_Filter.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Noise Filter.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Define cutoffBin: index below which frequencies are considered low
			cutoffFreq := float64(180) / 255.0 * MAX_FILTER_FREQ
			cutoffBin := int(cutoffFreq / (SAMPLE_RATE / 2) * float64(len(samples)) / 2)

			// Calculate spectral energy in low and high frequency bands
			var unfilteredLowEnergy, unfilteredHighEnergy float64
			var filteredLowEnergy, filteredHighEnergy float64

			for i := 0; i < len(samples); i++ {
				if i < cutoffBin {
					unfilteredLowEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredLowEnergy += float64(samples[i] * samples[i])
				} else {
					unfilteredHighEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredHighEnergy += float64(samples[i] * samples[i])
				}
			}

			// Calculate attenuation metrics
			lowFreqAttenuation := 1.0 - (filteredLowEnergy / unfilteredLowEnergy)
			highFreqAmplitude := filteredHighEnergy / unfilteredHighEnergy

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"lowFreqAttenuation": lowFreqAttenuation,
				"highFreqAmplitude":  highFreqAmplitude,
				"cutoffFreq":         cutoffFreq,
				"resonancePeak":      resonancePeak,
			}
		},
		Expected: map[string]ExpectedMetric{
			"lowFreqAttenuation": {Expected: 0.4, Tolerance: 0.2},
			"highFreqAmplitude":  {Expected: 0.7, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			lowFreqAttenuation := metrics["lowFreqAttenuation"]
			highFreqAmplitude := metrics["highFreqAmplitude"]
			cutoffFreq := metrics["cutoffFreq"]
			resonancePeak := metrics["resonancePeak"]

			t.Logf("Noise Filter: low_atten=%.2f high_amp=%.2f cutoff=%.1f resonance=%.2f",
				lowFreqAttenuation, highFreqAmplitude, cutoffFreq, resonancePeak)

			// Validate expected low frequency attenuation
			expectedAtten := 0.4
			if math.Abs(lowFreqAttenuation-expectedAtten) > 0.2 {
				t.Errorf("Noise Filter: Incorrect low frequency attenuation: got %.2f, expected %.2f",
					lowFreqAttenuation, expectedAtten)
			}

			// Validate high frequency preservation
			minHighFreqAmp := 0.7
			if highFreqAmplitude < minHighFreqAmp {
				t.Errorf("Noise Filter: Excessive high frequency attenuation: amp=%.2f, min=%.2f",
					highFreqAmplitude, minHighFreqAmp)
			}

			// Verify resonance behavior - with max resonance, should see a significant peak
			expectedResonancePeak := 1.0 + float64(255)/255.0
			if resonancePeak < expectedResonancePeak*0.8 {
				t.Errorf("Noise Filter: Insufficient resonance peak: %.2f, expected >= %.2f",
					resonancePeak, expectedResonancePeak*0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Noise Filter.unfiltered")
		},
	},
	{
		Name: "All Channels",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A2_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},

			{Register: TRI_FREQ, Value: A4_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			{Register: SINE_FREQ, Value: 1760},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "hp_unfiltered_All_Channels.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("All Channels.unfiltered", unfiltered)

			// Configure and enable the filter
			chip.HandleRegisterWrite(FILTER_TYPE, 2) // High-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 150)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "hp_filtered_All_Channels.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("All Channels.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Define cutoffBin: index below which frequencies are considered low
			cutoffFreq := float64(150) / 255.0 * MAX_FILTER_FREQ
			cutoffBin := int(cutoffFreq / (SAMPLE_RATE / 2) * float64(len(samples)) / 2)

			// Calculate spectral energy in low and high frequency bands
			var unfilteredLowEnergy, unfilteredHighEnergy float64
			var filteredLowEnergy, filteredHighEnergy float64

			for i := 0; i < len(samples); i++ {
				if i < cutoffBin {
					unfilteredLowEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredLowEnergy += float64(samples[i] * samples[i])
				} else {
					unfilteredHighEnergy += float64(unfiltered[i] * unfiltered[i])
					filteredHighEnergy += float64(samples[i] * samples[i])
				}
			}

			// Calculate attenuation metrics
			lowFreqAttenuation := 1.0 - (filteredLowEnergy / unfilteredLowEnergy)
			highFreqAmplitude := filteredHighEnergy / unfilteredHighEnergy

			// Check for resonance peak
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"lowFreqAttenuation": lowFreqAttenuation,
				"highFreqAmplitude":  highFreqAmplitude,
				"cutoffFreq":         cutoffFreq,
				"resonancePeak":      resonancePeak,
			}
		},
		Expected: map[string]ExpectedMetric{
			"lowFreqAttenuation": {Expected: 0.5, Tolerance: 0.2},
			"highFreqAmplitude":  {Expected: 0.7, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			lowFreqAttenuation := metrics["lowFreqAttenuation"]
			highFreqAmplitude := metrics["highFreqAmplitude"]
			cutoffFreq := metrics["cutoffFreq"]
			resonancePeak := metrics["resonancePeak"]

			t.Logf("All Channels: low_atten=%.2f high_amp=%.2f cutoff=%.1f resonance=%.2f",
				lowFreqAttenuation, highFreqAmplitude, cutoffFreq, resonancePeak)

			// Validate expected low frequency attenuation
			expectedAtten := 0.5
			if math.Abs(lowFreqAttenuation-expectedAtten) > 0.2 {
				t.Errorf("All Channels: Incorrect low frequency attenuation: got %.2f, expected %.2f",
					lowFreqAttenuation, expectedAtten)
			}

			// Validate high frequency preservation
			minHighFreqAmp := 0.7
			if highFreqAmplitude < minHighFreqAmp {
				t.Errorf("All Channels: Excessive high frequency attenuation: amp=%.2f, min=%.2f",
					highFreqAmplitude, minHighFreqAmp)
			}

			// Verify resonance behavior
			expectedResonancePeak := 1.0 + float64(128)/255.0
			if resonancePeak < expectedResonancePeak*0.8 {
				t.Errorf("All Channels: Insufficient resonance peak: %.2f, expected >= %.2f",
					resonancePeak, expectedResonancePeak*0.8)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("All Channels.unfiltered")
		},
	},
}

func TestGlobalFilterHighPass(t *testing.T) {
	for _, tc := range globalFilterHighPassTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var globalFilterBandPassTests = []AudioTestCase{
	{
		Name: "Square Band",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "bp_unfiltered_Square_Band.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Square Band.unfiltered", unfiltered)

			// Configure band-pass filter
			chip.HandleRegisterWrite(FILTER_TYPE, 3) // Band-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 128)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 200)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "bp_filtered_Square_Band.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Square Band.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Compute the filter's center frequency in Hz
			centerFreq := float64(128) / 255.0 * MAX_FILTER_FREQ
			// Approximate bandwidth: lower resonance yields wider bandwidth
			bandwidth := centerFreq / float64(200/64.0)

			var passbandEnergy, rejectbandEnergy float64
			var unfilteredPassbandEnergy, unfilteredRejectbandEnergy float64

			// Iterate over frequency bins
			for i := 0; i < len(samples); i++ {
				freq := float64(i) / float64(len(samples)) * SAMPLE_RATE / 2
				inPassband := math.Abs(freq-centerFreq) < bandwidth/2
				if inPassband {
					passbandEnergy += float64(samples[i] * samples[i])
					unfilteredPassbandEnergy += float64(unfiltered[i] * unfiltered[i])
				} else {
					rejectbandEnergy += float64(samples[i] * samples[i])
					unfilteredRejectbandEnergy += float64(unfiltered[i] * unfiltered[i])
				}
			}

			passbandRatio := math.Sqrt(passbandEnergy / unfilteredPassbandEnergy)
			rejectbandRatio := math.Sqrt(rejectbandEnergy / unfilteredRejectbandEnergy)
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"passbandRatio":   passbandRatio,
				"rejectbandRatio": rejectbandRatio,
				"resonancePeak":   resonancePeak,
				"centerFreq":      centerFreq,
				"bandwidth":       bandwidth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"passbandRatio":   {Expected: 0.8, Tolerance: 0.2},
			"rejectbandRatio": {Expected: 0.3, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			passbandRatio := metrics["passbandRatio"]
			rejectbandRatio := metrics["rejectbandRatio"]
			resonancePeak := metrics["resonancePeak"]
			centerFreq := metrics["centerFreq"]
			bandwidth := metrics["bandwidth"]

			t.Logf("Square Band: pass_ratio=%.2f, reject_ratio=%.2f, bandwidth=%.1f Hz",
				passbandRatio, rejectbandRatio, bandwidth)

			// Verify that filter passes frequencies in the passband
			if passbandRatio < 0.8 {
				t.Errorf("Square Band: Insufficient passband amplitude: got %.2f, expected >= %.2f",
					passbandRatio, 0.8)
			}

			// Verify that filter attenuates frequencies outside the passband
			if rejectbandRatio > 0.3 {
				t.Errorf("Square Band: Insufficient stopband attenuation: got %.2f, expected <= %.2f",
					rejectbandRatio, 0.3)
			}

			// Verify resonance behavior
			expectedPeak := 1.0 + float64(200)/255.0
			if resonancePeak < expectedPeak*0.7 {
				t.Errorf("Square Band: Insufficient resonance peak: got %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.7)
			}

			// Check that square wave frequency is properly filtered
			if passbandRatio < 0.5 && float64(A4_FREQUENCY) > centerFreq-bandwidth/2 && float64(A4_FREQUENCY) < centerFreq+bandwidth/2 {
				t.Errorf("Square Band: Excessive attenuation in passband for 440.0 Hz")
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Square Band.unfiltered")
		},
	},
	{
		Name: "Triangle Sweep",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "bp_unfiltered_Triangle_Sweep.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Triangle Sweep.unfiltered", unfiltered)

			// Configure band-pass filter
			chip.HandleRegisterWrite(FILTER_TYPE, 3) // Band-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 64)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 255)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "bp_filtered_Triangle_Sweep.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Triangle Sweep.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Compute the filter's center frequency in Hz
			centerFreq := float64(64) / 255.0 * MAX_FILTER_FREQ
			// Approximate bandwidth: lower resonance yields wider bandwidth
			bandwidth := centerFreq / float64(255/64.0)

			var passbandEnergy, rejectbandEnergy float64
			var unfilteredPassbandEnergy, unfilteredRejectbandEnergy float64

			// Iterate over frequency bins
			for i := 0; i < len(samples); i++ {
				freq := float64(i) / float64(len(samples)) * SAMPLE_RATE / 2
				inPassband := math.Abs(freq-centerFreq) < bandwidth/2
				if inPassband {
					passbandEnergy += float64(samples[i] * samples[i])
					unfilteredPassbandEnergy += float64(unfiltered[i] * unfiltered[i])
				} else {
					rejectbandEnergy += float64(samples[i] * samples[i])
					unfilteredRejectbandEnergy += float64(unfiltered[i] * unfiltered[i])
				}
			}

			passbandRatio := math.Sqrt(passbandEnergy / unfilteredPassbandEnergy)
			rejectbandRatio := math.Sqrt(rejectbandEnergy / unfilteredRejectbandEnergy)
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"passbandRatio":   passbandRatio,
				"rejectbandRatio": rejectbandRatio,
				"resonancePeak":   resonancePeak,
				"centerFreq":      centerFreq,
				"bandwidth":       bandwidth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"passbandRatio":   {Expected: 0.7, Tolerance: 0.2},
			"rejectbandRatio": {Expected: 0.2, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			passbandRatio := metrics["passbandRatio"]
			rejectbandRatio := metrics["rejectbandRatio"]
			resonancePeak := metrics["resonancePeak"]
			bandwidth := metrics["bandwidth"]

			t.Logf("Triangle Sweep: pass_ratio=%.2f, reject_ratio=%.2f, bandwidth=%.1f Hz",
				passbandRatio, rejectbandRatio, bandwidth)

			// Verify passband
			if passbandRatio < 0.7 {
				t.Errorf("Triangle Sweep: Insufficient passband amplitude: got %.2f, expected >= %.2f",
					passbandRatio, 0.7)
			}

			// Verify stopband
			if rejectbandRatio > 0.2 {
				t.Errorf("Triangle Sweep: Insufficient stopband attenuation: got %.2f, expected <= %.2f",
					rejectbandRatio, 0.2)
			}

			// Verify resonance behavior with high resonance setting
			expectedPeak := 1.0 + float64(255)/255.0
			if resonancePeak < expectedPeak*0.7 {
				t.Errorf("Triangle Sweep: Insufficient resonance peak: got %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.7)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Triangle Sweep.unfiltered")
		},
	},
	{
		Name: "Sine Harmonics",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SINE_FREQ, Value: 1760},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "bp_unfiltered_Sine_Harmonics.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Sine Harmonics.unfiltered", unfiltered)

			// Configure band-pass filter
			chip.HandleRegisterWrite(FILTER_TYPE, 3) // Band-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 192)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 150)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "bp_filtered_Sine_Harmonics.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Sine Harmonics.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Compute the filter's center frequency in Hz
			centerFreq := float64(192) / 255.0 * MAX_FILTER_FREQ
			// Approximate bandwidth: lower resonance yields wider bandwidth
			bandwidth := centerFreq / float64(150/64.0)

			var passbandEnergy, rejectbandEnergy float64
			var unfilteredPassbandEnergy, unfilteredRejectbandEnergy float64

			// Iterate over frequency bins
			for i := 0; i < len(samples); i++ {
				freq := float64(i) / float64(len(samples)) * SAMPLE_RATE / 2
				inPassband := math.Abs(freq-centerFreq) < bandwidth/2
				if inPassband {
					passbandEnergy += float64(samples[i] * samples[i])
					unfilteredPassbandEnergy += float64(unfiltered[i] * unfiltered[i])
				} else {
					rejectbandEnergy += float64(samples[i] * samples[i])
					unfilteredRejectbandEnergy += float64(unfiltered[i] * unfiltered[i])
				}
			}

			passbandRatio := math.Sqrt(passbandEnergy / unfilteredPassbandEnergy)
			rejectbandRatio := math.Sqrt(rejectbandEnergy / unfilteredRejectbandEnergy)
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"passbandRatio":   passbandRatio,
				"rejectbandRatio": rejectbandRatio,
				"resonancePeak":   resonancePeak,
				"centerFreq":      centerFreq,
				"bandwidth":       bandwidth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"passbandRatio":   {Expected: 0.9, Tolerance: 0.2},
			"rejectbandRatio": {Expected: 0.4, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			passbandRatio := metrics["passbandRatio"]
			rejectbandRatio := metrics["rejectbandRatio"]
			resonancePeak := metrics["resonancePeak"]
			centerFreq := metrics["centerFreq"]
			bandwidth := metrics["bandwidth"]

			t.Logf("Sine Harmonics: pass_ratio=%.2f, reject_ratio=%.2f, bandwidth=%.1f Hz",
				passbandRatio, rejectbandRatio, bandwidth)

			// Verify passband
			if passbandRatio < 0.9 {
				t.Errorf("Sine Harmonics: Insufficient passband amplitude: got %.2f, expected >= %.2f",
					passbandRatio, 0.9)
			}

			// Verify stopband
			if rejectbandRatio > 0.4 {
				t.Errorf("Sine Harmonics: Insufficient stopband attenuation: got %.2f, expected <= %.2f",
					rejectbandRatio, 0.4)
			}

			// Verify resonance behavior
			expectedPeak := 1.0 + float64(150)/255.0
			if resonancePeak < expectedPeak*0.7 {
				t.Errorf("Sine Harmonics: Insufficient resonance peak: got %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.7)
			}

			// Pure sine should pass with minimal attenuation
			if 1760.0 > centerFreq-bandwidth/2 && 1760.0 < centerFreq+bandwidth/2 && passbandRatio < 0.5 {
				t.Errorf("Sine Harmonics: Excessive attenuation in passband for 1760.0 Hz")
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Sine Harmonics.unfiltered")
		},
	},
	{
		Name: "Noise Band",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: NOISE_FREQ, Value: 2000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "bp_unfiltered_Noise_Band.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("Noise Band.unfiltered", unfiltered)

			// Configure band-pass filter
			chip.HandleRegisterWrite(FILTER_TYPE, 3) // Band-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 100)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 180)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "bp_filtered_Noise_Band.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("Noise Band.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Compute the filter's center frequency in Hz
			centerFreq := float64(100) / 255.0 * MAX_FILTER_FREQ
			// Approximate bandwidth: lower resonance yields wider bandwidth
			bandwidth := centerFreq / float64(180/64.0)

			var passbandEnergy, rejectbandEnergy float64
			var unfilteredPassbandEnergy, unfilteredRejectbandEnergy float64

			// Iterate over frequency bins
			for i := 0; i < len(samples); i++ {
				freq := float64(i) / float64(len(samples)) * SAMPLE_RATE / 2
				inPassband := math.Abs(freq-centerFreq) < bandwidth/2
				if inPassband {
					passbandEnergy += float64(samples[i] * samples[i])
					unfilteredPassbandEnergy += float64(unfiltered[i] * unfiltered[i])
				} else {
					rejectbandEnergy += float64(samples[i] * samples[i])
					unfilteredRejectbandEnergy += float64(unfiltered[i] * unfiltered[i])
				}
			}

			passbandRatio := math.Sqrt(passbandEnergy / unfilteredPassbandEnergy)
			rejectbandRatio := math.Sqrt(rejectbandEnergy / unfilteredRejectbandEnergy)
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"passbandRatio":   passbandRatio,
				"rejectbandRatio": rejectbandRatio,
				"resonancePeak":   resonancePeak,
				"centerFreq":      centerFreq,
				"bandwidth":       bandwidth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"passbandRatio":   {Expected: 0.6, Tolerance: 0.2},
			"rejectbandRatio": {Expected: 0.3, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			passbandRatio := metrics["passbandRatio"]
			rejectbandRatio := metrics["rejectbandRatio"]
			resonancePeak := metrics["resonancePeak"]
			bandwidth := metrics["bandwidth"]

			t.Logf("Noise Band: pass_ratio=%.2f, reject_ratio=%.2f, bandwidth=%.1f Hz",
				passbandRatio, rejectbandRatio, bandwidth)

			// Verify passband
			if passbandRatio < 0.6 {
				t.Errorf("Noise Band: Insufficient passband amplitude: got %.2f, expected >= %.2f",
					passbandRatio, 0.6)
			}

			// Verify stopband
			if rejectbandRatio > 0.3 {
				t.Errorf("Noise Band: Insufficient stopband attenuation: got %.2f, expected <= %.2f",
					rejectbandRatio, 0.3)
			}

			// Verify resonance behavior
			expectedPeak := 1.0 + float64(180)/255.0
			if resonancePeak < expectedPeak*0.7 {
				t.Errorf("Noise Band: Insufficient resonance peak: got %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.7)
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("Noise Band.unfiltered")
		},
	},
	{
		Name: "All Channels Combined",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Disable all channels initially
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},

			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			{Register: SINE_FREQ, Value: 1760},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unfiltered output first
			unfiltered := captureAudio(nil, "bp_unfiltered_All_Channels_Combined.wav")

			// Store unfiltered samples for later comparison
			testMetrics.Store("All Channels Combined.unfiltered", unfiltered)

			// Configure band-pass filter
			chip.HandleRegisterWrite(FILTER_TYPE, 3) // Band-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 128)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 200)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "bp_filtered_All_Channels_Combined.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unfiltered samples
			unfilteredInterface, _ := testMetrics.Load("All Channels Combined.unfiltered")
			unfiltered := unfilteredInterface.([]float32)

			// Compute the filter's center frequency in Hz
			centerFreq := float64(128) / 255.0 * MAX_FILTER_FREQ
			// Approximate bandwidth: lower resonance yields wider bandwidth
			bandwidth := centerFreq / float64(200/64.0)

			var passbandEnergy, rejectbandEnergy float64
			var unfilteredPassbandEnergy, unfilteredRejectbandEnergy float64

			// Iterate over frequency bins
			for i := 0; i < len(samples); i++ {
				freq := float64(i) / float64(len(samples)) * SAMPLE_RATE / 2
				inPassband := math.Abs(freq-centerFreq) < bandwidth/2
				if inPassband {
					passbandEnergy += float64(samples[i] * samples[i])
					unfilteredPassbandEnergy += float64(unfiltered[i] * unfiltered[i])
				} else {
					rejectbandEnergy += float64(samples[i] * samples[i])
					unfilteredRejectbandEnergy += float64(unfiltered[i] * unfiltered[i])
				}
			}

			passbandRatio := math.Sqrt(passbandEnergy / unfilteredPassbandEnergy)
			rejectbandRatio := math.Sqrt(rejectbandEnergy / unfilteredRejectbandEnergy)
			resonancePeak := findResonancePeak(samples)

			return map[string]float64{
				"passbandRatio":   passbandRatio,
				"rejectbandRatio": rejectbandRatio,
				"resonancePeak":   resonancePeak,
				"centerFreq":      centerFreq,
				"bandwidth":       bandwidth,
			}
		},
		Expected: map[string]ExpectedMetric{
			"passbandRatio":   {Expected: 0.7, Tolerance: 0.2},
			"rejectbandRatio": {Expected: 0.3, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			passbandRatio := metrics["passbandRatio"]
			rejectbandRatio := metrics["rejectbandRatio"]
			resonancePeak := metrics["resonancePeak"]
			centerFreq := metrics["centerFreq"]
			bandwidth := metrics["bandwidth"]

			t.Logf("All Channels Combined: pass_ratio=%.2f, reject_ratio=%.2f, bandwidth=%.1f Hz",
				passbandRatio, rejectbandRatio, bandwidth)

			// Verify passband
			if passbandRatio < 0.7 {
				t.Errorf("All Channels Combined: Insufficient passband amplitude: got %.2f, expected >= %.2f",
					passbandRatio, 0.7)
			}

			// Verify stopband
			if rejectbandRatio > 0.3 {
				t.Errorf("All Channels Combined: Insufficient stopband attenuation: got %.2f, expected <= %.2f",
					rejectbandRatio, 0.3)
			}

			// Verify resonance behavior
			expectedPeak := 1.0 + float64(200)/255.0
			if resonancePeak < expectedPeak*0.7 {
				t.Errorf("All Channels Combined: Insufficient resonance peak: got %.2f, expected >= %.2f",
					resonancePeak, expectedPeak*0.7)
			}

			// Combined output should maintain filter characteristics
			if passbandRatio < 0.5 && float64(A4_FREQUENCY) > centerFreq-bandwidth/2 && float64(A4_FREQUENCY) < centerFreq+bandwidth/2 {
				t.Errorf("All Channels Combined: Excessive attenuation in passband for 440.0 Hz")
			}
		},
		PostCleanup: func() {
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			testMetrics.Delete("All Channels Combined.unfiltered")
		},
	},
}

func TestGlobalFilterBandPass(t *testing.T) {
	for _, tc := range globalFilterBandPassTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

var globalFilterModulationTests = []AudioTestCase{
	{
		Name: "Square by Sine",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Reset all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			// Configure input channel (Square)
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},

			// Configure modulation source (Sine)
			{Register: SINE_FREQ, Value: 2},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unmodulated filter response
			chip.HandleRegisterWrite(FILTER_TYPE, 1)
			chip.HandleRegisterWrite(FILTER_CUTOFF, 128)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
			unmodulated := captureAudio(nil, "filter_mod_unmod_Square_by_Sine.wav")
			testMetrics.Store("Square by Sine.unmodulated", unmodulated)

			// Enable modulation
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2) // Sine = source 2
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 200)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "filter_mod_Square_by_Sine.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unmodulated samples
			unmodulatedInterface, _ := testMetrics.Load("Square by Sine.unmodulated")
			unmodulated := unmodulatedInterface.([]float32)

			// Analyze modulation depth
			var maxDiff, avgFreqVar float64
			prevUnmod := unmodulated[0]
			prevMod := samples[0]
			for i := 1; i < len(samples); i++ {
				unmodDiff := math.Abs(float64(unmodulated[i] - prevUnmod))
				modDiff := math.Abs(float64(samples[i] - prevMod))
				diff := math.Abs(modDiff - unmodDiff)
				maxDiff = math.Max(maxDiff, diff)
				avgFreqVar += diff
				prevUnmod = unmodulated[i]
				prevMod = samples[i]
			}
			avgFreqVar /= float64(len(samples))

			// Calculate modulation index
			modIndex := maxDiff / float64(200) * 255.0
			modFreq := detectModulationFrequency(samples, unmodulated)

			return map[string]float64{
				"modIndex":   modIndex,
				"avgFreqVar": avgFreqVar,
				"maxDiff":    maxDiff,
				"modFreq":    modFreq,
			}
		},
		Expected: map[string]ExpectedMetric{
			"modIndex": {Expected: 0.6, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			modIndex := metrics["modIndex"]
			avgFreqVar := metrics["avgFreqVar"]
			maxDiff := metrics["maxDiff"]
			modFreq := metrics["modFreq"]

			t.Logf("Square by Sine: mod_index=%.2f avg_var=%.3f max_diff=%.2f mod_freq=%.2f",
				modIndex, avgFreqVar, maxDiff, modFreq)

			// Verify modulation depth
			if math.Abs(modIndex-0.6) > 0.2 {
				t.Errorf("Square by Sine: Incorrect modulation depth: got %.2f, expected %.2f ±0.2",
					modIndex, 0.6)
			}

			// Verify frequency tracking
			expectedFreq := 2.0
			if math.Abs(modFreq-expectedFreq)/expectedFreq > 0.2 {
				t.Errorf("Square by Sine: Incorrect modulation frequency: got %.1f Hz, expected %.1f Hz",
					modFreq, expectedFreq)
			}
		},
		PostCleanup: func() {
			// Cleanup
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
			testMetrics.Delete("Square by Sine.unmodulated")
		},
	},
	{
		Name: "Triangle by Square",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Reset all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			// Configure input channel (Triangle)
			{Register: TRI_FREQ, Value: A5_FREQUENCY},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},

			// Configure modulation source (Square)
			{Register: SQUARE_FREQ, Value: 4},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 64},
			{Register: SQUARE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unmodulated filter response
			chip.HandleRegisterWrite(FILTER_TYPE, 2) // High-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 64)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
			unmodulated := captureAudio(nil, "filter_mod_unmod_Triangle_by_Square.wav")
			testMetrics.Store("Triangle by Square.unmodulated", unmodulated)

			// Enable modulation
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0) // Square = source 0
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 255)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "filter_mod_Triangle_by_Square.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unmodulated samples
			unmodulatedInterface, _ := testMetrics.Load("Triangle by Square.unmodulated")
			unmodulated := unmodulatedInterface.([]float32)

			// Analyze modulation depth
			var maxDiff, avgFreqVar float64
			prevUnmod := unmodulated[0]
			prevMod := samples[0]
			for i := 1; i < len(samples); i++ {
				unmodDiff := math.Abs(float64(unmodulated[i] - prevUnmod))
				modDiff := math.Abs(float64(samples[i] - prevMod))
				diff := math.Abs(modDiff - unmodDiff)
				maxDiff = math.Max(maxDiff, diff)
				avgFreqVar += diff
				prevUnmod = unmodulated[i]
				prevMod = samples[i]
			}
			avgFreqVar /= float64(len(samples))

			// Calculate modulation index
			modIndex := maxDiff / float64(255) * 255.0
			modFreq := detectModulationFrequency(samples, unmodulated)

			return map[string]float64{
				"modIndex":   modIndex,
				"avgFreqVar": avgFreqVar,
				"maxDiff":    maxDiff,
				"modFreq":    modFreq,
			}
		},
		Expected: map[string]ExpectedMetric{
			"modIndex": {Expected: 0.8, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			modIndex := metrics["modIndex"]
			avgFreqVar := metrics["avgFreqVar"]
			maxDiff := metrics["maxDiff"]
			modFreq := metrics["modFreq"]

			t.Logf("Triangle by Square: mod_index=%.2f avg_var=%.3f max_diff=%.2f mod_freq=%.2f",
				modIndex, avgFreqVar, maxDiff, modFreq)

			// Verify modulation depth
			if math.Abs(modIndex-0.8) > 0.2 {
				t.Errorf("Triangle by Square: Incorrect modulation depth: got %.2f, expected %.2f ±0.2",
					modIndex, 0.8)
			}

			// Verify frequency tracking
			expectedFreq := 4.0
			if math.Abs(modFreq-expectedFreq)/expectedFreq > 0.2 {
				t.Errorf("Triangle by Square: Incorrect modulation frequency: got %.1f Hz, expected %.1f Hz",
					modFreq, expectedFreq)
			}
		},
		PostCleanup: func() {
			// Cleanup
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
			testMetrics.Delete("Triangle by Square.unmodulated")
		},
	},
	{
		Name: "Sine by Triangle",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Reset all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			// Configure input channel (Sine)
			{Register: SINE_FREQ, Value: A4_FREQUENCY},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},

			// Configure modulation source (Triangle)
			{Register: TRI_FREQ, Value: 3},
			{Register: TRI_VOL, Value: 255},
			{Register: TRI_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unmodulated filter response
			chip.HandleRegisterWrite(FILTER_TYPE, 3) // Band-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 192)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
			unmodulated := captureAudio(nil, "filter_mod_unmod_Sine_by_Triangle.wav")
			testMetrics.Store("Sine by Triangle.unmodulated", unmodulated)

			// Enable modulation
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 1) // Triangle = source 1
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 128)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "filter_mod_Sine_by_Triangle.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unmodulated samples
			unmodulatedInterface, _ := testMetrics.Load("Sine by Triangle.unmodulated")
			unmodulated := unmodulatedInterface.([]float32)

			// Analyze modulation depth
			var maxDiff, avgFreqVar float64
			prevUnmod := unmodulated[0]
			prevMod := samples[0]
			for i := 1; i < len(samples); i++ {
				unmodDiff := math.Abs(float64(unmodulated[i] - prevUnmod))
				modDiff := math.Abs(float64(samples[i] - prevMod))
				diff := math.Abs(modDiff - unmodDiff)
				maxDiff = math.Max(maxDiff, diff)
				avgFreqVar += diff
				prevUnmod = unmodulated[i]
				prevMod = samples[i]
			}
			avgFreqVar /= float64(len(samples))

			// Calculate modulation index
			modIndex := maxDiff / float64(128) * 255.0
			modFreq := detectModulationFrequency(samples, unmodulated)

			return map[string]float64{
				"modIndex":   modIndex,
				"avgFreqVar": avgFreqVar,
				"maxDiff":    maxDiff,
				"modFreq":    modFreq,
			}
		},
		Expected: map[string]ExpectedMetric{
			"modIndex": {Expected: 0.4, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			modIndex := metrics["modIndex"]
			avgFreqVar := metrics["avgFreqVar"]
			maxDiff := metrics["maxDiff"]
			modFreq := metrics["modFreq"]

			t.Logf("Sine by Triangle: mod_index=%.2f avg_var=%.3f max_diff=%.2f mod_freq=%.2f",
				modIndex, avgFreqVar, maxDiff, modFreq)

			// Verify modulation depth
			if math.Abs(modIndex-0.4) > 0.2 {
				t.Errorf("Sine by Triangle: Incorrect modulation depth: got %.2f, expected %.2f ±0.2",
					modIndex, 0.4)
			}

			// Verify frequency tracking
			expectedFreq := 3.0
			if math.Abs(modFreq-expectedFreq)/expectedFreq > 0.2 {
				t.Errorf("Sine by Triangle: Incorrect modulation frequency: got %.1f Hz, expected %.1f Hz",
					modFreq, expectedFreq)
			}
		},
		PostCleanup: func() {
			// Cleanup
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
			testMetrics.Delete("Sine by Triangle.unmodulated")
		},
	},
	{
		Name: "Noise by Sine",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Reset all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			// Configure input channel (Noise)
			{Register: NOISE_FREQ, Value: 1000},
			{Register: NOISE_VOL, Value: 255},
			{Register: NOISE_CTRL, Value: 3},

			// Configure modulation source (Sine)
			{Register: SINE_FREQ, Value: 5},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unmodulated filter response
			chip.HandleRegisterWrite(FILTER_TYPE, 1) // Low-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 100)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
			unmodulated := captureAudio(nil, "filter_mod_unmod_Noise_by_Sine.wav")
			testMetrics.Store("Noise by Sine.unmodulated", unmodulated)

			// Enable modulation
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2) // Sine = source 2
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 180)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "filter_mod_Noise_by_Sine.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unmodulated samples
			unmodulatedInterface, _ := testMetrics.Load("Noise by Sine.unmodulated")
			unmodulated := unmodulatedInterface.([]float32)

			// Analyze modulation depth
			var maxDiff, avgFreqVar float64
			prevUnmod := unmodulated[0]
			prevMod := samples[0]
			for i := 1; i < len(samples); i++ {
				unmodDiff := math.Abs(float64(unmodulated[i] - prevUnmod))
				modDiff := math.Abs(float64(samples[i] - prevMod))
				diff := math.Abs(modDiff - unmodDiff)
				maxDiff = math.Max(maxDiff, diff)
				avgFreqVar += diff
				prevUnmod = unmodulated[i]
				prevMod = samples[i]
			}
			avgFreqVar /= float64(len(samples))

			// Calculate modulation index
			modIndex := maxDiff / float64(180) * 255.0
			modFreq := detectModulationFrequency(samples, unmodulated)

			return map[string]float64{
				"modIndex":   modIndex,
				"avgFreqVar": avgFreqVar,
				"maxDiff":    maxDiff,
				"modFreq":    modFreq,
			}
		},
		Expected: map[string]ExpectedMetric{
			"modIndex": {Expected: 0.5, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			modIndex := metrics["modIndex"]
			avgFreqVar := metrics["avgFreqVar"]
			maxDiff := metrics["maxDiff"]
			modFreq := metrics["modFreq"]

			t.Logf("Noise by Sine: mod_index=%.2f avg_var=%.3f max_diff=%.2f mod_freq=%.2f",
				modIndex, avgFreqVar, maxDiff, modFreq)

			// Verify modulation depth
			if math.Abs(modIndex-0.5) > 0.2 {
				t.Errorf("Noise by Sine: Incorrect modulation depth: got %.2f, expected %.2f ±0.2",
					modIndex, 0.5)
			}

			// Verify frequency tracking
			expectedFreq := 5.0
			if math.Abs(modFreq-expectedFreq)/expectedFreq > 0.2 {
				t.Errorf("Noise by Sine: Incorrect modulation frequency: got %.1f Hz, expected %.1f Hz",
					modFreq, expectedFreq)
			}
		},
		PostCleanup: func() {
			// Cleanup
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
			testMetrics.Delete("Noise by Sine.unmodulated")
		},
	},
	{
		Name: "All Channels with Sine Mod",
		PreSetup: func() {
			// Enable global audio processing
			chip.HandleRegisterWrite(AUDIO_CTRL, 1)

			// Reset all channels
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
		},
		Config: []RegisterWrite{
			// Configure input channel (Square)
			{Register: SQUARE_FREQ, Value: A4_FREQUENCY},
			{Register: SQUARE_VOL, Value: 255},
			{Register: SQUARE_DUTY, Value: 128},
			{Register: SQUARE_CTRL, Value: 3},

			// Configure modulation source (Sine)
			{Register: SINE_FREQ, Value: 3},
			{Register: SINE_VOL, Value: 255},
			{Register: SINE_CTRL, Value: 3},
		},
		AdditionalSetup: func() {
			// Capture unmodulated filter response
			chip.HandleRegisterWrite(FILTER_TYPE, 1) // Low-pass
			chip.HandleRegisterWrite(FILTER_CUTOFF, 128)
			chip.HandleRegisterWrite(FILTER_RESONANCE, 128)
			unmodulated := captureAudio(nil, "filter_mod_unmod_All_Channels_with_Sine_Mod.wav")
			testMetrics.Store("All Channels with Sine Mod.unmodulated", unmodulated)

			// Enable modulation
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2) // Sine = source 2
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 200)
		},
		CaptureDuration: CAPTURE_SHORT_MS * time.Millisecond,
		Filename:        "filter_mod_All_Channels_with_Sine_Mod.wav",
		AnalyzeFunc: func(samples []float32) map[string]float64 {
			// Retrieve unmodulated samples
			unmodulatedInterface, _ := testMetrics.Load("All Channels with Sine Mod.unmodulated")
			unmodulated := unmodulatedInterface.([]float32)

			// Analyze modulation depth
			var maxDiff, avgFreqVar float64
			prevUnmod := unmodulated[0]
			prevMod := samples[0]
			for i := 1; i < len(samples); i++ {
				unmodDiff := math.Abs(float64(unmodulated[i] - prevUnmod))
				modDiff := math.Abs(float64(samples[i] - prevMod))
				diff := math.Abs(modDiff - unmodDiff)
				maxDiff = math.Max(maxDiff, diff)
				avgFreqVar += diff
				prevUnmod = unmodulated[i]
				prevMod = samples[i]
			}
			avgFreqVar /= float64(len(samples))

			// Calculate modulation index
			modIndex := maxDiff / float64(200) * 255.0
			modFreq := detectModulationFrequency(samples, unmodulated)

			// Special test case: enable all channels
			chip.HandleRegisterWrite(TRI_FREQ, 660)
			chip.HandleRegisterWrite(TRI_VOL, 255)
			chip.HandleRegisterWrite(TRI_CTRL, 3)

			chip.HandleRegisterWrite(NOISE_FREQ, 1000)
			chip.HandleRegisterWrite(NOISE_VOL, 255)
			chip.HandleRegisterWrite(NOISE_CTRL, 3)

			allChannels := captureAudio(nil, "filter_mod_all.wav")
			modDepthAll := calculateModulationDepth(allChannels, unmodulated)

			return map[string]float64{
				"modIndex":    modIndex,
				"avgFreqVar":  avgFreqVar,
				"maxDiff":     maxDiff,
				"modFreq":     modFreq,
				"modDepthAll": modDepthAll,
			}
		},
		Expected: map[string]ExpectedMetric{
			"modIndex": {Expected: 0.6, Tolerance: 0.2},
		},
		Validate: func(t *testing.T, metrics map[string]float64) {
			modIndex := metrics["modIndex"]
			avgFreqVar := metrics["avgFreqVar"]
			maxDiff := metrics["maxDiff"]
			modFreq := metrics["modFreq"]
			modDepthAll := metrics["modDepthAll"]

			t.Logf("All Channels with Sine Mod: mod_index=%.2f avg_var=%.3f max_diff=%.2f mod_freq=%.2f all_mod=%.2f",
				modIndex, avgFreqVar, maxDiff, modFreq, modDepthAll)

			// Verify modulation depth
			if math.Abs(modIndex-0.6) > 0.2 {
				t.Errorf("All Channels with Sine Mod: Incorrect modulation depth: got %.2f, expected %.2f ±0.2",
					modIndex, 0.6)
			}

			// Verify frequency tracking
			expectedFreq := 3.0
			if math.Abs(modFreq-expectedFreq)/expectedFreq > 0.2 {
				t.Errorf("All Channels with Sine Mod: Incorrect modulation frequency: got %.1f Hz, expected %.1f Hz",
					modFreq, expectedFreq)
			}

			// Check if modulation with all channels is still effective
			if modDepthAll < modIndex*0.7 {
				t.Errorf("Modulation depth decreased with all channels: %.2f vs %.2f",
					modDepthAll, modIndex)
			}
		},
		PostCleanup: func() {
			// Cleanup
			chip.HandleRegisterWrite(SQUARE_CTRL, 0)
			chip.HandleRegisterWrite(TRI_CTRL, 0)
			chip.HandleRegisterWrite(SINE_CTRL, 0)
			chip.HandleRegisterWrite(NOISE_CTRL, 0)
			chip.HandleRegisterWrite(FILTER_TYPE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
			chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
			testMetrics.Delete("All Channels with Sine Mod.unmodulated")
		},
	},
}

func TestGlobalFilterModulation(t *testing.T) {
	for _, tc := range globalFilterModulationTests {
		t.Run(tc.Name, func(t *testing.T) {
			runAudioTest(t, tc)
		})
	}
}

//type GlobalReverbTestCase struct {
//	name         string
//	enabledChan  []bool   // One boolean per channel (order: Square, Triangle, Sine, Noise)
//	mix          uint32   // Reverb mix (0-255)
//	decay        uint32   // Reverb decay (0-255)
//	frequencies  []uint32 // Frequencies for each channel (if enabled)
//	volumes      []uint32 // Volumes for each channel (if enabled)
//	minDecayTime float64  // Minimum acceptable decay time (in seconds)
//	minWetLevel  float64  // Minimum acceptable RMS level for the wet signal
//	maxDryLevel  float64  // Maximum acceptable RMS level for the decay (dry) portion
//}
//
//var globalReverbTests = []GlobalReverbTestCase{
//	{
//		name:         "Square Wave Reverb",
//		enabledChan:  []bool{true, false, false, false},
//		mix:          192,
//		decay:        200,
//		frequencies:  []uint32{A4_FREQUENCY, 0, 0, 0},
//		volumes:      []uint32{128, 0, 0, 0},
//		minDecayTime: 0.015,
//		minWetLevel:  0.1,
//		maxDryLevel:  0.6,
//	},
//	{
//		name:         "Triangle Wave Reverb",
//		enabledChan:  []bool{false, true, false, false},
//		mix:          128,
//		decay:        150,
//		frequencies:  []uint32{0, A4_FREQUENCY, 0, 0},
//		volumes:      []uint32{0, 128, 0, 0},
//		minDecayTime: 0.025,
//		minWetLevel:  0.1,
//		maxDryLevel:  0.3,
//	},
//	{
//		name:         "Sine Wave Reverb",
//		enabledChan:  []bool{false, false, true, false},
//		mix:          64,
//		decay:        100,
//		frequencies:  []uint32{0, 0, A4_FREQUENCY, 0},
//		volumes:      []uint32{0, 0, 128, 0},
//		minDecayTime: 0.003,
//		minWetLevel:  0.05,
//		maxDryLevel:  0.2,
//	},
//	{
//		name:         "Noise Reverb",
//		enabledChan:  []bool{false, false, false, true},
//		mix:          255,
//		decay:        255,
//		frequencies:  []uint32{0, 0, 0, 1000},
//		volumes:      []uint32{0, 0, 0, 96},
//		minDecayTime: 0.015,
//		minWetLevel:  0.2,
//		maxDryLevel:  0.7,
//	},
//	{
//		name:         "All Channels Reverb",
//		enabledChan:  []bool{true, true, true, true},
//		mix:          128,
//		decay:        180,
//		frequencies:  []uint32{A4_FREQUENCY, 554, 659, 500},
//		volumes:      []uint32{96, 96, 96, 48},
//		minDecayTime: 0.003,
//		minWetLevel:  0.15,
//		maxDryLevel:  0.3,
//	},
//}
//
//func TestGlobalReverb(t *testing.T) {
//	genericTestRunner(t, globalReverbTests,
//		func(tc GlobalReverbTestCase) string { return tc.name },
//		func(t *testing.T, tc GlobalReverbTestCase, _ int) {
//			// Reset chip state.
//			chip.HandleRegisterWrite(AUDIO_CTRL, 0)
//			time.Sleep(100 * time.Millisecond)
//
//			// Configure reverb parameters.
//			chip.HandleRegisterWrite(REVERB_MIX, tc.mix)
//			chip.HandleRegisterWrite(REVERB_DECAY, tc.decay)
//			chip.HandleRegisterWrite(AUDIO_CTRL, 1)
//
//			// Configure channels.
//			for i := 0; i < NUM_CHANNELS; i++ {
//				if tc.enabledChan[i] {
//					var ctrl, freq, vol uint32
//					switch i {
//					case 0:
//						ctrl, freq, vol = SQUARE_CTRL, SQUARE_FREQ, SQUARE_VOL
//					case 1:
//						ctrl, freq, vol = TRI_CTRL, TRI_FREQ, TRI_VOL
//					case 2:
//						ctrl, freq, vol = SINE_CTRL, SINE_FREQ, SINE_VOL
//					case 3:
//						ctrl, freq, vol = NOISE_CTRL, NOISE_FREQ, NOISE_VOL
//					}
//					chip.HandleRegisterWrite(freq, tc.frequencies[i])
//					chip.HandleRegisterWrite(vol, tc.volumes[i])
//					chip.HandleRegisterWrite(ctrl, 3)
//				}
//			}
//
//			// Prepare a WAV file to record the full reverb response.
//			filename := fmt.Sprintf("reverb_%s.wav", strings.ReplaceAll(tc.name, " ", "_"))
//			f, err := os.Create(filename)
//			if err != nil {
//				t.Fatalf("Failed to create WAV: %v", err)
//			}
//			defer f.Close()
//
//			// Calculate total samples.
//			drySamples := int(DRY_DURATION * SAMPLE_RATE)
//			wetSamples := int(WET_DURATION * SAMPLE_RATE)
//			decaySamples := int(DECAY_DURATION * SAMPLE_RATE)
//			totalSamples := drySamples + wetSamples + decaySamples
//
//			// Write WAV header.
//			writeWAVHeader(f, totalSamples)
//
//			// Capture and write initial dry signal.
//			dry := make([]float32, drySamples)
//			for i := range dry {
//				sample := chip.GenerateSample()
//				dry[i] = sample
//				binary.Write(f, binary.LittleEndian,
//					int16(math.Max(-32768, math.Min(32767, float64(sample)*32767))))
//				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
//			}
//
//			// Capture and write wet signal.
//			wet := make([]float32, wetSamples)
//			for i := range wet {
//				sample := chip.GenerateSample()
//				wet[i] = sample
//				binary.Write(f, binary.LittleEndian,
//					int16(math.Max(-32768, math.Min(32767, float64(sample)*32767))))
//				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
//			}
//
//			// Gate off all channels.
//			for i := 0; i < NUM_CHANNELS; i++ {
//				if tc.enabledChan[i] {
//					var ctrl uint32
//					switch i {
//					case 0:
//						ctrl = SQUARE_CTRL
//					case 1:
//						ctrl = TRI_CTRL
//					case 2:
//						ctrl = SINE_CTRL
//					case 3:
//						ctrl = NOISE_CTRL
//					}
//					chip.HandleRegisterWrite(ctrl, 0)
//				}
//			}
//
//			// Capture and write decay signal.
//			decay := make([]float32, decaySamples)
//			for i := range decay {
//				sample := chip.GenerateSample()
//				decay[i] = sample
//				binary.Write(f, binary.LittleEndian,
//					int16(math.Max(-32768, math.Min(32767, float64(sample)*32767))))
//				time.Sleep(time.Second / time.Duration(SAMPLE_RATE))
//			}
//
//			// Analysis.
//			dryRMS := calculateRMS(dry)
//			wetRMS := calculateRMS(wet)
//			decayRMS := calculateRMS(decay)
//			decayTime := measureDecayTime(decay)
//
//			t.Logf("%s: dry=%.3f wet=%.3f decay=%.3f time=%.3fs",
//				tc.name, dryRMS, wetRMS, decayRMS, decayTime)
//
//			if decayTime < tc.minDecayTime {
//				t.Errorf("%s: Decay time too short: got %.3fs, want >= %.3fs",
//					tc.name, decayTime, tc.minDecayTime)
//			}
//
//			if wetRMS < tc.minWetLevel {
//				t.Errorf("%s: Wet signal too quiet: got %.3f, want >= %.3f",
//					tc.name, wetRMS, tc.minWetLevel)
//			}
//
//			if decayRMS > tc.maxDryLevel {
//				t.Errorf("%s: Decay level too high: got %.3f, want <= %.3f",
//					tc.name, decayRMS, tc.maxDryLevel)
//			}
//
//			actualMix := wetRMS / (dryRMS + wetRMS)
//			expectedMix := float64(tc.mix) / 255.0
//			mixTolerance := 0.3
//			if math.Abs(actualMix-expectedMix) > mixTolerance {
//				t.Errorf("%s: Wet/dry mix incorrect: got %.2f, want %.2f ±%.2f",
//					tc.name, actualMix, expectedMix, mixTolerance)
//			}
//		},
//	)
//}
