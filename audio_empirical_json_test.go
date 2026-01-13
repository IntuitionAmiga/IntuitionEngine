// audio_empirical_json_test.go - JSON test framework for audio unit tests

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
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

//go:build empiricaljson

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Register mappings for registers defined externally
const (
	// Filter registers
	FILTER_CTRL = FILTER_TYPE

	// Noise registers
	NOISE_TYPE = NOISE_MODE
)

// TestConfig represents a complete test configuration from JSON
type TestConfig struct {
	// Basic info
	Name        string `json:"name"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`

	// Test parameters
	Channel   string      `json:"channel"`             // "square", "triangle", "sine", "noise"
	Frequency interface{} `json:"frequency,omitempty"` // Can be string constant or numeric
	Volume    interface{} `json:"volume,omitempty"`
	DutyCycle interface{} `json:"dutyCycle,omitempty"`

	// Effect parameters
	Effect       string                 `json:"effect,omitempty"` // "pwm", "sweep", "sync", etc.
	EffectParams map[string]interface{} `json:"effectParams,omitempty"`

	// Register overwrites (for direct register configuration)
	Registers map[string]interface{} `json:"registers,omitempty"`

	// Test execution parameters
	PreReset  bool        `json:"preReset"`
	PostReset bool        `json:"postReset"`
	Analyser  string      `json:"analyser"`
	Duration  interface{} `json:"duration"`

	// Expected results
	Expected map[string]ExpectedValueJSON `json:"expected"`

	// Any custom parameters
	Params map[string]interface{} `json:"params,omitempty"`
}

// ExpectedValueJSON represents an expected test value from JSON
type ExpectedValueJSON struct {
	Value     interface{} `json:"value"`
	Tolerance interface{} `json:"tolerance"`
}

// ProcessedTestConfig contains resolved values from TestConfig
type ProcessedTestConfig struct {
	// Basic info
	Name        string
	Category    string
	Description string

	// Test parameters
	Channel   string
	Frequency uint32
	Volume    uint32
	DutyCycle uint32

	// Effect parameters
	Effect       string
	EffectParams map[string]uint32

	// Register configurations
	Registers map[uint32]uint32

	// Test execution parameters
	PreReset  bool
	PostReset bool
	Analyser  string
	Duration  time.Duration

	// Expected results
	Expected map[string]ExpectedMetric

	// Processed parameters
	Params map[string]interface{}
}

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

// AudioConstants organizes all audio-related constants into logical groups
var AudioConstants = struct {
	Frequency struct {
		A2, C4, E4, G4, A4, A5 uint32
	}
	Channel struct {
		Index struct {
			Square, Triangle, Sine, Noise int
		}
		Control struct {
			Enabled, Disabled, RegisterEnable uint32
		}
		Volume struct {
			Mid, Max uint32
		}
	}
	Envelope struct {
		Attack struct {
			Instant, Fast, Slow, VerySlow uint32
		}
		Decay struct {
			Fast, Medium, Slow, VerySlow uint32
		}
		Sustain struct {
			Low, Medium, High uint32
		}
		Release struct {
			Fast, Medium, Slow, VerySlow, ExtraSlow uint32
		}
	}
	Modulation struct {
		PWM struct {
			DepthNormal, EnableBit, RateSlow, RateMedium, RateNormal, RateFast uint32
		}
		Sweep struct {
			EnableBit, DirectionUp, DirectionDown uint32
			Period                                struct {
				Fast, Medium, Slow uint32
			}
			Shift struct {
				Narrow, Medium, Wide, VeryWide uint32
			}
		}
	}
	Filter struct {
		Type struct {
			None, Lowpass, Highpass, Bandpass uint32
		}
		Cutoff struct {
			Low, MidLow, Mid, MidHigh, High uint32
		}
		Resonance struct {
			Low, Medium, High, Max uint32
		}
	}
	DutyCycle struct {
		Half, Quarter uint32
		Min, Max      uint32
	}
	Amplitude struct {
		Normal      float64
		NormalPhase float64
	}
	Timing struct {
		CaptureShortMs, CaptureLongMs uint32
		PlaybackSeconds               float32
	}
	Analysis struct {
		Tolerance struct {
			Frequency, Amplitude, Duty, Default float64
			CrestFactor                         float64
		}
		Threshold struct {
			ZeroCrossing       float64
			MinSignalAmplitude float64
			MinPeakProminence  float64
			Decay              float64
			DbConversionFactor float64
		}
		Purity struct {
			MinSync, TestMinWave float64
		}
		Samples struct {
			SegmentCount, MinPeaksForAnalysis, MinAnalysisSamples, SmoothingWindowSize int
		}
	}
	PCM struct {
		MinValue, MaxValue      int
		BitsPerSample           uint32
		FormatSize, HeaderSize  int
		FormatPCM, MonoChannels int
		DataChunkSize           int
	}
	Noise struct {
		ModeWhite, ModePeriodic, ModeMetallic uint32
	}
}{
	Frequency: struct {
		A2, C4, E4, G4, A4, A5 uint32
	}{
		A2: 110,
		C4: 262,
		E4: 330,
		G4: 392,
		A4: 440,
		A5: 880,
	},
	Channel: struct {
		Index struct {
			Square, Triangle, Sine, Noise int
		}
		Control struct {
			Enabled, Disabled, RegisterEnable uint32
		}
		Volume struct {
			Mid, Max uint32
		}
	}{
		Index: struct {
			Square, Triangle, Sine, Noise int
		}{
			Square:   0,
			Triangle: 1,
			Sine:     2,
			Noise:    3,
		},
		Control: struct {
			Enabled, Disabled, RegisterEnable uint32
		}{
			Enabled:        1,
			Disabled:       0,
			RegisterEnable: 3,
		},
		Volume: struct {
			Mid, Max uint32
		}{
			Mid: 128,
			Max: 255,
		},
	},
	Envelope: struct {
		Attack struct {
			Instant, Fast, Slow, VerySlow uint32
		}
		Decay struct {
			Fast, Medium, Slow, VerySlow uint32
		}
		Sustain struct {
			Low, Medium, High uint32
		}
		Release struct {
			Fast, Medium, Slow, VerySlow, ExtraSlow uint32
		}
	}{
		Attack: struct {
			Instant, Fast, Slow, VerySlow uint32
		}{
			Instant:  1,
			Fast:     5,
			Slow:     50,
			VerySlow: 100,
		},
		Decay: struct {
			Fast, Medium, Slow, VerySlow uint32
		}{
			Fast:     20,
			Medium:   50,
			Slow:     100,
			VerySlow: 200,
		},
		Sustain: struct {
			Low, Medium, High uint32
		}{
			Low:    128,
			Medium: 180,
			High:   200,
		},
		Release: struct {
			Fast, Medium, Slow, VerySlow, ExtraSlow uint32
		}{
			Fast:      10,
			Medium:    20,
			Slow:      50,
			VerySlow:  100,
			ExtraSlow: 200,
		},
	},
	Modulation: struct {
		PWM struct {
			DepthNormal, EnableBit, RateSlow, RateMedium, RateNormal, RateFast uint32
		}
		Sweep struct {
			EnableBit, DirectionUp, DirectionDown uint32
			Period                                struct {
				Fast, Medium, Slow uint32
			}
			Shift struct {
				Narrow, Medium, Wide, VeryWide uint32
			}
		}
	}{
		PWM: struct {
			DepthNormal, EnableBit, RateSlow, RateMedium, RateNormal, RateFast uint32
		}{
			DepthNormal: 64,
			EnableBit:   0x80,
			RateSlow:    0x10,
			RateMedium:  0x30,
			RateNormal:  0x30,
			RateFast:    0x70,
		},
		Sweep: struct {
			EnableBit, DirectionUp, DirectionDown uint32
			Period                                struct {
				Fast, Medium, Slow uint32
			}
			Shift struct {
				Narrow, Medium, Wide, VeryWide uint32
			}
		}{
			EnableBit:     0x80,
			DirectionUp:   0x08,
			DirectionDown: 0x00,
			Period: struct {
				Fast, Medium, Slow uint32
			}{
				Fast:   0x01,
				Medium: 0x02,
				Slow:   0x03,
			},
			Shift: struct {
				Narrow, Medium, Wide, VeryWide uint32
			}{
				Narrow:   1,
				Medium:   2,
				Wide:     3,
				VeryWide: 4,
			},
		},
	},
	Filter: struct {
		Type struct {
			None, Lowpass, Highpass, Bandpass uint32
		}
		Cutoff struct {
			Low, MidLow, Mid, MidHigh, High uint32
		}
		Resonance struct {
			Low, Medium, High, Max uint32
		}
	}{
		Type: struct {
			None, Lowpass, Highpass, Bandpass uint32
		}{
			None:     0,
			Lowpass:  1,
			Highpass: 2,
			Bandpass: 3,
		},
		Cutoff: struct {
			Low, MidLow, Mid, MidHigh, High uint32
		}{
			Low:     32,
			MidLow:  64,
			Mid:     128,
			MidHigh: 192,
			High:    200,
		},
		Resonance: struct {
			Low, Medium, High, Max uint32
		}{
			Low:    64,
			Medium: 128,
			High:   192,
			Max:    255,
		},
	},
	DutyCycle: struct {
		Half, Quarter uint32
		Min, Max      uint32
	}{
		Half:    128,
		Quarter: 64,
		Min:     0,
		Max:     255,
	},
	Amplitude: struct {
		Normal      float64
		NormalPhase float64
	}{
		Normal:      1.0,
		NormalPhase: 0.0,
	},
	Timing: struct {
		CaptureShortMs, CaptureLongMs uint32
		PlaybackSeconds               float32
	}{
		CaptureShortMs:  100,
		CaptureLongMs:   300,
		PlaybackSeconds: 5.0,
	},
	Analysis: struct {
		Tolerance struct {
			Frequency, Amplitude, Duty, Default float64
			CrestFactor                         float64
		}
		Threshold struct {
			ZeroCrossing       float64
			MinSignalAmplitude float64
			MinPeakProminence  float64
			Decay              float64
			DbConversionFactor float64
		}
		Purity struct {
			MinSync, TestMinWave float64
		}
		Samples struct {
			SegmentCount, MinPeaksForAnalysis, MinAnalysisSamples, SmoothingWindowSize int
		}
	}{
		Tolerance: struct {
			Frequency, Amplitude, Duty, Default float64
			CrestFactor                         float64
		}{
			Frequency:   0.02,
			Amplitude:   0.02,
			Duty:        0.02,
			Default:     0.2,
			CrestFactor: 0.2,
		},
		Threshold: struct {
			ZeroCrossing       float64
			MinSignalAmplitude float64
			MinPeakProminence  float64
			Decay              float64
			DbConversionFactor float64
		}{
			ZeroCrossing:       0.0,
			MinSignalAmplitude: 1e-6,
			MinPeakProminence:  0.01,
			Decay:              -60.0,
			DbConversionFactor: 20.0,
		},
		Purity: struct {
			MinSync, TestMinWave float64
		}{
			MinSync:     0.15,
			TestMinWave: 0.95,
		},
		Samples: struct {
			SegmentCount, MinPeaksForAnalysis, MinAnalysisSamples, SmoothingWindowSize int
		}{
			SegmentCount:        10,
			MinPeaksForAnalysis: 5,
			MinAnalysisSamples:  3,
			SmoothingWindowSize: 5,
		},
	},
	PCM: struct {
		MinValue, MaxValue      int
		BitsPerSample           uint32
		FormatSize, HeaderSize  int
		FormatPCM, MonoChannels int
		DataChunkSize           int
	}{
		MinValue:      -32768,
		MaxValue:      32767,
		BitsPerSample: 16,
		FormatSize:    16,
		HeaderSize:    44,
		FormatPCM:     1,
		MonoChannels:  1,
		DataChunkSize: 8,
	},
	Noise: struct {
		ModeWhite, ModePeriodic, ModeMetallic uint32
	}{
		ModeWhite:    0,
		ModePeriodic: 1,
		ModeMetallic: 2,
	},
}

// Register mappings - Maps string names to register addresses
var registerMapping = map[string]uint32{
	"SQUARE_FREQ": SQUARE_FREQ,
	"SQUARE_VOL":  SQUARE_VOL,
	"SQUARE_DUTY": SQUARE_DUTY,
	"SQUARE_CTRL": SQUARE_CTRL,
	"TRI_FREQ":    TRI_FREQ,
	"TRI_VOL":     TRI_VOL,
	"TRI_CTRL":    TRI_CTRL,
	"SINE_FREQ":   SINE_FREQ,
	"SINE_VOL":    SINE_VOL,
	"SINE_CTRL":   SINE_CTRL,
	"NOISE_FREQ":  NOISE_FREQ,
	"NOISE_VOL":   NOISE_VOL,
	"NOISE_CTRL":  NOISE_CTRL,

	"FILTER_TYPE":         FILTER_TYPE,
	"FILTER_CTRL":         FILTER_TYPE, // Use FILTER_TYPE as the control register
	"FILTER_CUTOFF":       FILTER_CUTOFF,
	"FILTER_RESONANCE":    FILTER_RESONANCE,
	"OVERDRIVE_CTRL":      OVERDRIVE_CTRL,
	"SYNC_SOURCE_CH1":     SYNC_SOURCE_CH1,
	"RING_MOD_SOURCE_CH1": RING_MOD_SOURCE_CH1,
	"RING_MOD_SOURCE_CH2": RING_MOD_SOURCE_CH2,
	"AUDIO_CTRL":          AUDIO_CTRL,
	"SQUARE_PWM_CTRL":     SQUARE_PWM_CTRL,
	"SQUARE_SWEEP":        SQUARE_SWEEP,
	"TRI_SWEEP":           TRI_SWEEP,
	"SINE_SWEEP":          SINE_SWEEP,

	"SQUARE_ATK": SQUARE_ATK,
	"SQUARE_DEC": SQUARE_DEC,
	"SQUARE_SUS": SQUARE_SUS,
	"SQUARE_REL": SQUARE_REL,
	"TRI_ATK":    TRI_ATK,
	"TRI_DEC":    TRI_DEC,
	"TRI_SUS":    TRI_SUS,
	"TRI_REL":    TRI_REL,
	"SINE_ATK":   SINE_ATK,
	"SINE_DEC":   SINE_DEC,
	"SINE_SUS":   SINE_SUS,
	"SINE_REL":   SINE_REL,
	"NOISE_ATK":  NOISE_ATK,
	"NOISE_DEC":  NOISE_DEC,
	"NOISE_SUS":  NOISE_SUS,
	"NOISE_REL":  NOISE_REL,
}

// Time unit mappings
var timeUnitMapping = map[string]time.Duration{
	"MILLISECOND": time.Millisecond,
	"SECOND":      time.Second,
	"MS":          time.Millisecond,
	"S":           time.Second,
}

// Map to store custom analysers
var customAnalysers = map[string]func([]float32) map[string]float64{}

// constantMapping maps string constants to values
var constantMapping map[string]interface{}

// Initialize constantMapping from AudioConstants
func init() {
	constantMapping = map[string]interface{}{
		// Frequencies
		"A2_FREQUENCY": AudioConstants.Frequency.A2,
		"C4_FREQUENCY": AudioConstants.Frequency.C4,
		"E4_FREQUENCY": AudioConstants.Frequency.E4,
		"G4_FREQUENCY": AudioConstants.Frequency.G4,
		"A4_FREQUENCY": AudioConstants.Frequency.A4,
		"A5_FREQUENCY": AudioConstants.Frequency.A5,

		// Channel indexes
		"SQUARE_CHANNEL_IDX":   AudioConstants.Channel.Index.Square,
		"TRIANGLE_CHANNEL_IDX": AudioConstants.Channel.Index.Triangle,
		"SINE_CHANNEL_IDX":     AudioConstants.Channel.Index.Sine,
		"NOISE_CHANNEL_IDX":    AudioConstants.Channel.Index.Noise,

		// Binary control values
		"ENABLED":         AudioConstants.Channel.Control.Enabled,
		"DISABLED":        AudioConstants.Channel.Control.Disabled,
		"MID_VOLUME":      AudioConstants.Channel.Volume.Mid,
		"MAX_VOLUME":      AudioConstants.Channel.Volume.Max,
		"REGISTER_ENABLE": AudioConstants.Channel.Control.RegisterEnable,

		// Duty cycle parameters
		"HALF_DUTY_CYCLE":    AudioConstants.DutyCycle.Half,
		"QUARTER_DUTY_CYCLE": AudioConstants.DutyCycle.Quarter,
		"MIN_DUTY_CYCLE":     AudioConstants.DutyCycle.Min,
		"MAX_DUTY_CYCLE":     AudioConstants.DutyCycle.Max,
		"DUTY_CYCLE_NORMAL":  0.5, // 50% duty cycle

		// PWM parameters
		"PWM_DEPTH_NORMAL": AudioConstants.Modulation.PWM.DepthNormal,
		"PWM_ENABLE_BIT":   AudioConstants.Modulation.PWM.EnableBit,
		"PWM_RATE_SLOW":    AudioConstants.Modulation.PWM.RateSlow,
		"PWM_RATE_MEDIUM":  AudioConstants.Modulation.PWM.RateMedium,
		"PWM_RATE_NORMAL":  AudioConstants.Modulation.PWM.RateNormal,
		"PWM_RATE_FAST":    AudioConstants.Modulation.PWM.RateFast,

		// Sweep parameters
		"SWEEP_ENABLE_BIT":      AudioConstants.Modulation.Sweep.EnableBit,
		"SWEEP_DIRECTION_UP":    AudioConstants.Modulation.Sweep.DirectionUp,
		"SWEEP_DIRECTION_DOWN":  AudioConstants.Modulation.Sweep.DirectionDown,
		"SWEEP_PERIOD_FAST":     AudioConstants.Modulation.Sweep.Period.Fast,
		"SWEEP_PERIOD_MEDIUM":   AudioConstants.Modulation.Sweep.Period.Medium,
		"SWEEP_PERIOD_SLOW":     AudioConstants.Modulation.Sweep.Period.Slow,
		"SWEEP_SHIFT_NARROW":    AudioConstants.Modulation.Sweep.Shift.Narrow,
		"SWEEP_SHIFT_MEDIUM":    AudioConstants.Modulation.Sweep.Shift.Medium,
		"SWEEP_SHIFT_WIDE":      AudioConstants.Modulation.Sweep.Shift.Wide,
		"SWEEP_SHIFT_VERY_WIDE": AudioConstants.Modulation.Sweep.Shift.VeryWide,

		// Filter parameters
		"FILTER_TYPE_NONE":        AudioConstants.Filter.Type.None,
		"FILTER_TYPE_LOWPASS":     AudioConstants.Filter.Type.Lowpass,
		"FILTER_TYPE_HIGHPASS":    AudioConstants.Filter.Type.Highpass,
		"FILTER_TYPE_BANDPASS":    AudioConstants.Filter.Type.Bandpass,
		"FILTER_CUTOFF_LOW":       AudioConstants.Filter.Cutoff.Low,
		"FILTER_CUTOFF_MID_LOW":   AudioConstants.Filter.Cutoff.MidLow,
		"FILTER_CUTOFF_MID":       AudioConstants.Filter.Cutoff.Mid,
		"FILTER_CUTOFF_MID_HIGH":  AudioConstants.Filter.Cutoff.MidHigh,
		"FILTER_CUTOFF_HIGH":      AudioConstants.Filter.Cutoff.High,
		"FILTER_RESONANCE_LOW":    AudioConstants.Filter.Resonance.Low,
		"FILTER_RESONANCE_MEDIUM": AudioConstants.Filter.Resonance.Medium,
		"FILTER_RESONANCE_HIGH":   AudioConstants.Filter.Resonance.High,
		"FILTER_RESONANCE_MAX":    AudioConstants.Filter.Resonance.Max,

		// Envelope parameters
		"ATTACK_INSTANT_MS":     AudioConstants.Envelope.Attack.Instant,
		"ATTACK_FAST_MS":        AudioConstants.Envelope.Attack.Fast,
		"ATTACK_SLOW_MS":        AudioConstants.Envelope.Attack.Slow,
		"ATTACK_VERY_SLOW_MS":   AudioConstants.Envelope.Attack.VerySlow,
		"DECAY_FAST_MS":         AudioConstants.Envelope.Decay.Fast,
		"DECAY_MEDIUM_MS":       AudioConstants.Envelope.Decay.Medium,
		"DECAY_SLOW_MS":         AudioConstants.Envelope.Decay.Slow,
		"DECAY_VERY_SLOW_MS":    AudioConstants.Envelope.Decay.VerySlow,
		"SUSTAIN_LOW":           AudioConstants.Envelope.Sustain.Low,
		"SUSTAIN_MEDIUM":        AudioConstants.Envelope.Sustain.Medium,
		"SUSTAIN_HIGH":          AudioConstants.Envelope.Sustain.High,
		"HIGH_SUSTAIN":          AudioConstants.Envelope.Sustain.High,
		"RELEASE_FAST_MS":       AudioConstants.Envelope.Release.Fast,
		"RELEASE_MEDIUM_MS":     AudioConstants.Envelope.Release.Medium,
		"RELEASE_SLOW_MS":       AudioConstants.Envelope.Release.Slow,
		"RELEASE_VERY_SLOW_MS":  AudioConstants.Envelope.Release.VerySlow,
		"RELEASE_EXTRA_SLOW_MS": AudioConstants.Envelope.Release.ExtraSlow,

		// Noise modes
		"NOISE_MODE_WHITE":    AudioConstants.Noise.ModeWhite,
		"NOISE_MODE_PERIODIC": AudioConstants.Noise.ModePeriodic,
		"NOISE_MODE_METALLIC": AudioConstants.Noise.ModeMetallic,

		// Tolerance constants
		"FREQ_TOLERANCE":         AudioConstants.Analysis.Tolerance.Frequency,
		"AMPLITUDE_TOLERANCE":    AudioConstants.Analysis.Tolerance.Amplitude,
		"DUTY_TOLERANCE":         AudioConstants.Analysis.Tolerance.Duty,
		"DEFAULT_TOLERANCE":      AudioConstants.Analysis.Tolerance.Default,
		"CREST_FACTOR_TOLERANCE": AudioConstants.Analysis.Tolerance.CrestFactor,

		// Amplitude and normalization
		"AMPLITUDE_NORMAL": AudioConstants.Amplitude.Normal,

		// Capture durations
		"CAPTURE_SHORT_MS": AudioConstants.Timing.CaptureShortMs,
		"CAPTURE_LONG_MS":  AudioConstants.Timing.CaptureLongMs,

		"MIN_SYNC_PURITY":      AudioConstants.Analysis.Purity.MinSync,
		"TEST_MIN_WAVE_PURITY": AudioConstants.Analysis.Purity.TestMinWave,
	}
}

func captureAudio(t *testing.T, filename string) []float32 {
	// Get the samples for analysis
	numSamples := int(SAMPLE_RATE * float32(CAPTURE_SHORT_MS) / 1000)
	samples := make([]float32, numSamples)

	for i := range samples {
		samples[i] = chip.GenerateSample()
	}

	// For the WAV file, capture playback duration
	playbackSamples := int(SAMPLE_RATE * PLAYBACK_SECONDS)
	allSamples := make([]float32, playbackSamples)

	// First copy our analysis samples
	copy(allSamples, samples)

	// Then generate the rest
	for i := numSamples; i < playbackSamples; i++ {
		allSamples[i] = chip.GenerateSample()
	}

	// Write WAV file using the new utility
	err := WriteWAVFile(filename, allSamples, SAMPLE_RATE)
	if err != nil && t != nil {
		t.Fatalf("Failed to write WAV file: %v", err)
	}

	// Wait for post-capture time
	time.Sleep(POST_CAPTURE_WAIT_TIME)

	return samples
}

func standardTestSetup() {
	setupDisableAllChannels()
	setupResetAllPhases()
}

// LoadTestsFromFile loads test configurations from a JSON file
func LoadTestsFromFile(filename string) ([]TestConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	var tests []TestConfig
	if err := json.Unmarshal(data, &tests); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	return tests, nil
}

// resolveConstant converts a string expression to a numeric value
func resolveConstant(expr string) (float64, error) {
	// Check if it's a simple constant
	if val, exists := constantMapping[expr]; exists {
		return convertToFloat64(val)
	}

	// Check if it's an arithmetic expression with multiplication
	if strings.Contains(expr, "*") {
		parts := strings.Split(expr, "*")
		if len(parts) != 2 {
			return 0, fmt.Errorf("unsupported expression format: %s", expr)
		}

		leftStr, rightStr := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		left, err := resolveConstant(leftStr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve left part of expression: %v", err)
		}

		right, err := resolveConstant(rightStr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve right part of expression: %v", err)
		}

		return left * right, nil
	}

	// Check if it's an arithmetic expression with division
	if strings.Contains(expr, "/") {
		parts := strings.Split(expr, "/")
		if len(parts) != 2 {
			return 0, fmt.Errorf("unsupported expression format: %s", expr)
		}

		leftStr, rightStr := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		left, err := resolveConstant(leftStr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve left part of expression: %v", err)
		}

		right, err := resolveConstant(rightStr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve right part of expression: %v", err)
		}

		if right == 0 {
			return 0, fmt.Errorf("division by zero in expression: %s", expr)
		}

		return left / right, nil
	}

	// Try direct conversion
	val, err := strconv.ParseFloat(expr, 64)
	if err != nil {
		return 0, fmt.Errorf("unrecognized constant or invalid number: %s", expr)
	}
	return val, nil
}

// convertToFloat64 converts various types to float64
func convertToFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("unsupported type for conversion: %T", val)
	}
}

// resolveValue converts an interface value to a float64
func resolveValue(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		return resolveConstant(v)
	default:
		return 0, fmt.Errorf("unsupported value type: %T", val)
	}
}

// resolveDuration converts a duration value to time.Duration
func resolveDuration(val interface{}) (time.Duration, error) {
	switch v := val.(type) {
	case float64:
		return time.Duration(v) * time.Millisecond, nil
	case int:
		return time.Duration(v) * time.Millisecond, nil
	case string:
		// Special case for CAPTURE_LONG_MS * 2 * MILLISECOND
		if v == "CAPTURE_LONG_MS * 2 * MILLISECOND" {
			captureVal, exists := constantMapping["CAPTURE_LONG_MS"]
			if !exists {
				return 0, fmt.Errorf("unknown constant: CAPTURE_LONG_MS")
			}

			captureFloat, err := convertToFloat64(captureVal)
			if err != nil {
				return 0, err
			}

			return time.Duration(captureFloat*2) * time.Millisecond, nil
		}
		if strings.Contains(v, "*") {
			parts := strings.Split(v, "*")
			if len(parts) != 2 {
				return 0, fmt.Errorf("invalid duration format: %s", v)
			}

			valStr, unitStr := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			value, err := resolveConstant(valStr)
			if err != nil {
				return 0, fmt.Errorf("failed to resolve duration value: %v", err)
			}

			unit, exists := timeUnitMapping[unitStr]
			if !exists {
				return 0, fmt.Errorf("unknown time unit: %s", unitStr)
			}

			return time.Duration(value) * unit, nil
		}

		// Try direct conversion
		val, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("unrecognized duration format: %s", v)
		}
		return time.Duration(val) * time.Millisecond, nil
	default:
		return 0, fmt.Errorf("unsupported duration type: %T", val)
	}
}

// selectAndRunAnalyser selects and runs the appropriate analyser
func selectAndRunAnalyser(analyserName string, samples []float32, config ProcessedTestConfig) map[string]float64 {
	analyzer := NewAudioAnalyzer(samples)

	switch analyserName {
	case "basic":
		return analyzer.AnalyzeBasic()
	case "pwm":
		return analyzer.AnalyzePWM()
	case "sweep":
		isUpward := true
		if val, ok := config.EffectParams["direction"]; ok {
			isUpward = val == SWEEP_DIRECTION_UP
		}
		return analyzer.AnalyzeSweep(isUpward)
	case "sync":
		masterFreq := float64(config.EffectParams["masterFreq"])
		return analyzer.AnalyzeSync(masterFreq)
	case "envelope":
		return analyzer.AnalyzeEnvelope()
	case "waveformPurity":
		return analyzer.AnalyzeWaveformPurity()
	case "dynamicRange":
		return analyzer.AnalyzeDynamicRange()
	case "spectral":
		expectedFreq := float64(config.Frequency)
		if val, ok := config.EffectParams["expectedFreq"]; ok {
			expectedFreq = float64(val)
		}
		return analyzer.AnalyzeSpectral(expectedFreq)
	case "custom":
		if analyseFunc, ok := customAnalysers[config.Name]; ok {
			return analyseFunc(samples)
		}
		return analyzer.AnalyzeBasic()
	default:
		return analyzer.AnalyzeBasic()
	}
}

// runTestFromConfig executes a test using the processed configuration
func runTestFromConfig(t *testing.T, config ProcessedTestConfig) {
	t.Logf("►► RUNNING TEST: %s/%s", config.Category, config.Name)
	t.Logf("   Channel: %s, Frequency: %d Hz", config.Channel, config.Frequency)

	if config.Effect != "" {
		t.Logf("   Effect: %s", config.Effect)
		for k, v := range config.EffectParams {
			t.Logf("     - %s: %v", k, v)
		}
	}

	// Setup
	if config.PreReset {
		standardTestSetup()
	}

	// Configure registers using the builder pattern
	builder := NewRegisterBuilder()
	builder.ConfigureFromTestConfig(config)
	builder.Apply()

	// Capture audio
	filename := fmt.Sprintf("%s_%s.wav", config.Category, config.Name)
	t.Logf("   Capturing audio to: %s (duration: %v)", filename, config.Duration)
	samples := captureAudio(t, filename)

	// Analyse results
	t.Logf("   Analyzing with: %s analyser", config.Analyser)
	metrics := selectAndRunAnalyser(config.Analyser, samples, config)

	// Log metrics
	t.Logf("   Results:")
	sortedKeys := getSortedKeys(metrics)
	for _, key := range sortedKeys {
		t.Logf("     - %s: %.4f", key, metrics[key])
	}

	// Validate results
	validateResults(t, metrics, config.Expected)

	// Cleanup
	if config.PostReset {
		cleanupChannel(config.Channel)
	}

	t.Logf("►► TEST COMPLETE: %s/%s\n", config.Category, config.Name)
}

// Helper function to get sorted keys for consistent output
func getSortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// // validateResults compares metrics against expected values
func validateResults(t *testing.T, metrics map[string]float64, expected map[string]ExpectedMetric) {
	t.Logf("   Validating results:")

	// Track if any failures occurred
	failures := false

	// Get sorted keys for consistent output
	sortedKeys := make([]string, 0, len(expected))
	for key := range expected {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		exp := expected[key]
		actual, ok := metrics[key]
		if !ok {
			t.Errorf("     ✗ Metric %s not computed", key)
			failures = true
			continue
		}

		diff := math.Abs(actual - exp.Expected)
		withinTolerance := diff <= exp.Tolerance

		if withinTolerance {
			t.Logf("     ✓ %s: %.4f (expected: %.4f ±%.4f)",
				key, actual, exp.Expected, exp.Tolerance)
		} else {
			t.Errorf("     ✗ %s: %.4f (expected: %.4f ±%.4f), diff: %.4f",
				key, actual, exp.Expected, exp.Tolerance, diff)
			failures = true
		}
	}

	if !failures {
		t.Logf("     All validations passed!")
	}
}

func cleanupChannel(channelType string) {
	builder := NewRegisterBuilder()
	switch channelType {
	case "square":
		builder.Write(SQUARE_CTRL, AudioConstants.Channel.Control.Disabled)
	case "triangle":
		builder.Write(TRI_CTRL, AudioConstants.Channel.Control.Disabled)
	case "sine":
		builder.Write(SINE_CTRL, AudioConstants.Channel.Control.Disabled)
	case "noise":
		builder.Write(NOISE_CTRL, AudioConstants.Channel.Control.Disabled)
	}
	builder.Apply()
}

// TestFromJSON is the main test function to be called from your test suite
func TestFromJSON(t *testing.T) {
	// Find all test files
	files, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatalf("Failed to find test files: %v", err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			runTestsFromFile(t, file)
		})
	}
}

// runTestsFromFile loads and runs tests from a JSON file
func runTestsFromFile(t *testing.T, filename string) {
	tests, err := LoadTestsFromFile(filename)
	if err != nil {
		t.Fatalf("Failed to load tests: %v", err)
	}

	// Extract category from filename if not specified
	category := strings.TrimSuffix(filepath.Base(filename), ".json")

	for _, test := range tests {
		if test.Category == "" {
			test.Category = category
		}

		t.Run(test.Name, func(t *testing.T) {
			config, err := processConfig(test)
			if err != nil {
				t.Fatalf("Failed to process test config: %v", err)
			}

			runTestFromConfig(t, config)
		})
	}
}

func setupDisableAllChannels() {
	builder := NewRegisterBuilder()
	builder.Write(SQUARE_CTRL, AudioConstants.Channel.Control.Disabled)
	builder.Write(TRI_CTRL, AudioConstants.Channel.Control.Disabled)
	builder.Write(SINE_CTRL, AudioConstants.Channel.Control.Disabled)
	builder.Write(NOISE_CTRL, AudioConstants.Channel.Control.Disabled)
	builder.Apply()
}

func setupResetAllPhases() {
	for i := range chip.channels {
		chip.channels[i].phase = NORMAL_PHASE_VALUE
	}
}

func TestAudioFromJSON(t *testing.T) {
	TestFromJSON(t)
}

// WriteWAVFile writes audio samples to a WAV file in one operation
func WriteWAVFile(filename string, samples []float32, sampleRate uint32) error {
	// Create file
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create WAV file: %v", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Constants
	bytesPerSample := AudioConstants.PCM.BitsPerSample / 8
	dataChunkSize := uint32(len(samples)) * bytesPerSample * uint32(AudioConstants.PCM.MonoChannels)
	headerSize := uint32(AudioConstants.PCM.HeaderSize - AudioConstants.PCM.DataChunkSize)
	fileSize := headerSize + dataChunkSize

	// Write RIFF header
	if _, err := f.WriteString("RIFF"); err != nil {
		return fmt.Errorf("failed to write RIFF marker: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, fileSize); err != nil {
		return fmt.Errorf("failed to write file size: %v", err)
	}
	if _, err := f.WriteString("WAVE"); err != nil {
		return fmt.Errorf("failed to write WAVE marker: %v", err)
	}

	// Write format chunk
	if _, err := f.WriteString("fmt "); err != nil {
		return fmt.Errorf("failed to write fmt marker: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(AudioConstants.PCM.FormatSize)); err != nil {
		return fmt.Errorf("failed to write format size: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(AudioConstants.PCM.FormatPCM)); err != nil {
		return fmt.Errorf("failed to write format type: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(AudioConstants.PCM.MonoChannels)); err != nil {
		return fmt.Errorf("failed to write channels: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, sampleRate); err != nil {
		return fmt.Errorf("failed to write sample rate: %v", err)
	}

	byteRate := sampleRate * uint32(AudioConstants.PCM.MonoChannels) * bytesPerSample
	if err := binary.Write(f, binary.LittleEndian, byteRate); err != nil {
		return fmt.Errorf("failed to write byte rate: %v", err)
	}

	blockAlign := uint16(AudioConstants.PCM.MonoChannels) * uint16(bytesPerSample)
	if err := binary.Write(f, binary.LittleEndian, blockAlign); err != nil {
		return fmt.Errorf("failed to write block align: %v", err)
	}

	if err := binary.Write(f, binary.LittleEndian, uint16(AudioConstants.PCM.BitsPerSample)); err != nil {
		return fmt.Errorf("failed to write bits per sample: %v", err)
	}

	// Write data chunk
	if _, err := f.WriteString("data"); err != nil {
		return fmt.Errorf("failed to write data marker: %v", err)
	}
	if err := binary.Write(f, binary.LittleEndian, dataChunkSize); err != nil {
		return fmt.Errorf("failed to write data size: %v", err)
	}

	// Write sample data
	for _, sample := range samples {
		pcm := int16(math.Max(float64(AudioConstants.PCM.MinValue),
			math.Min(float64(AudioConstants.PCM.MaxValue),
				float64(sample)*float64(AudioConstants.PCM.MaxValue))))
		if err := binary.Write(f, binary.LittleEndian, pcm); err != nil {
			return fmt.Errorf("failed to write sample: %v", err)
		}
	}

	return nil
}

// SignalCharacteristics contains common signal analysis data

// findMinMax gets minimum and maximum values in a slice
func findMinMax(values []float64) (min, max float64) {
	if len(values) == 0 {
		return 0, 0
	}

	min, max = values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// findMinMaxFloat32 gets minimum and maximum values in a float32 slice
func findMinMaxFloat32(values []float32) (min, max float64) {
	if len(values) == 0 {
		return 0, 0
	}

	min, max = float64(values[0]), float64(values[0])
	for _, v := range values {
		val := float64(v)
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}
	return min, max
}

// calculateRMS calculates Root Mean Square of a signal
func calculateRMS(samples []float32) float64 {
	if len(samples) == 0 {
		return MIN_SIGNAL_AMPLITUDE
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// calculateMean calculates the mean value of a signal
func calculateMean(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s)
	}
	return sum / float64(len(samples))
}

// calculateVariance calculates variance of a slice of values
func calculateVariance(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum, sumSq float64
	for _, v := range values {
		sum += v
		sumSq += v * v
	}
	mean := sum / float64(len(values))
	return (sumSq / float64(len(values))) - (mean * mean)
}

// detectZeroCrossings finds all zero crossing points with linear interpolation
func detectZeroCrossings(samples []float32, risingOnly bool) []float64 {
	var crossings []float64

	if len(samples) < 2 {
		return crossings
	}

	prev := float64(samples[0])
	for i := 1; i < len(samples); i++ {
		cur := float64(samples[i])

		// Rising edge detection
		if prev <= ZERO_CROSSING_THRESHOLD && cur > ZERO_CROSSING_THRESHOLD {
			// Linear interpolation for precise crossing
			fraction := 0.0
			if cur-prev != 0 {
				fraction = -prev / (cur - prev)
			}
			crossing := float64(i-1) + fraction
			crossings = append(crossings, crossing)
		} else if !risingOnly && prev > ZERO_CROSSING_THRESHOLD && cur <= ZERO_CROSSING_THRESHOLD {
			// Falling edge detection (if needed)
			fraction := 0.0
			if cur-prev != 0 {
				fraction = -prev / (cur - prev)
			}
			crossing := float64(i-1) + fraction
			crossings = append(crossings, crossing)
		}

		prev = cur
	}

	return crossings
}

// detectPeaks finds local maxima and minima in a signal
func detectPeaks(samples []float32, minProminence float32) []float64 {
	var peaks []float64

	if len(samples) < 3 {
		return peaks
	}

	for i := 1; i < len(samples)-1; i++ {
		prev := samples[i-1]
		current := samples[i]
		next := samples[i+1]

		isMax := current > prev+minProminence && current > next+minProminence
		isMin := current < prev-minProminence && current < next-minProminence

		if isMax || isMin {
			peaks = append(peaks, float64(math.Abs(float64(current))))
		}
	}

	return peaks
}

// analyseSignalCharacteristics performs comprehensive signal analysis
func analyseSignalCharacteristics(samples []float32, sampleRate float64) SignalCharacteristics {
	// Initialize result
	result := SignalCharacteristics{
		SampleRate: sampleRate,
		Duration:   float64(len(samples)) / sampleRate,
	}

	// Find min/max values
	result.Min, result.Max = findMinMaxFloat32(samples)

	// Calculate mean and RMS
	result.Mean = calculateMean(samples)
	result.RMS = calculateRMS(samples)

	// Find zero crossings
	result.Crossings = detectZeroCrossings(samples, false)

	// Detect peaks
	minProminence := float32((result.Max - result.Min) * MIN_PEAK_PROMINENCE_FACTOR)
	result.Peaks = detectPeaks(samples, minProminence)

	return result
}

// calculateFrequency estimates fundamental frequency from zero crossings
func calculateFrequency(crossings []float64, sampleRate float64) float64 {
	if len(crossings) < MIN_ANALYSIS_SAMPLES {
		return 0
	}

	// Calculate average period between crossings
	periodSamples := (crossings[len(crossings)-1] - crossings[0]) / float64(len(crossings)-1)
	if periodSamples <= 0 {
		return 0
	}

	return sampleRate / periodSamples
}

// calculateDutyCycle estimates duty cycle from samples
func calculateDutyCycle(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}

	positiveCount := 0
	for _, s := range samples {
		if s > ZERO_CROSSING_THRESHOLD {
			positiveCount++
		}
	}

	return float64(positiveCount) / float64(len(samples))
}

// calculateHarmonicContent measures harmonic distortion in a signal
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
	if totalSamples < MIN_ANALYSIS_SAMPLES {
		return MIN_SIGNAL_AMPLITUDE
	}

	// Calculate RMS values
	rmsFundamental := math.Sqrt(fundamentalPower / float64(totalSamples))
	rmsHarmonic := math.Sqrt(harmonicPower / float64(totalSamples))

	// Handle edge cases and normalize
	if rmsFundamental < MIN_SIGNAL_AMPLITUDE {
		return AMPLITUDE_NORMAL // All noise/harmonics if no fundamental
	}

	return math.Min(rmsHarmonic/rmsFundamental, AMPLITUDE_NORMAL)
}

// measureHarmonics calculates odd or even harmonic content
func measureHarmonics(samples []float32, oddHarmonics bool) float64 {
	var sum float64

	for i := 2; i < len(samples)-2; i++ {
		if oddHarmonics {
			// Three-point analysis for odd harmonics
			prev := float64(samples[i-1])
			curr := float64(samples[i])
			next := float64(samples[i+1])

			// Detect odd symmetry
			oddComponent := math.Abs(next + prev - 2*curr)
			sum += oddComponent
		} else {
			// Three-point analysis for even harmonics
			prev := float64(samples[i-1])
			next := float64(samples[i+1])

			// Detect even symmetry
			evenComponent := math.Abs(next - prev)
			sum += evenComponent
		}
	}

	return sum / float64(max(1, len(samples)-4))
}

// measureOddHarmonics calculates odd harmonic content
func measureOddHarmonics(samples []float32) float64 {
	return measureHarmonics(samples, true)
}

// measureEvenHarmonics calculates even harmonic content
func measureEvenHarmonics(samples []float32) float64 {
	return measureHarmonics(samples, false)
}

// measureDecayTime measures time for signal to decay to threshold
func measureDecayTime(samples []float32) float64 {
	// Find peak amplitude
	_, peak := findMinMaxFloat32(samples)

	// Find time to reach -60dB
	threshold := float64(peak) * math.Pow(10, DECAY_THRESHOLD/DB_CONVERSION_FACTOR)
	for i, s := range samples {
		if math.Abs(float64(s)) < threshold {
			return float64(i) / SAMPLE_RATE
		}
	}

	return float64(len(samples)) / SAMPLE_RATE
}

// analyseWaveformUsingCharacteristics analyses signal using common characteristics

// analysePWMSegments analyses duty cycle variations in segments
func analysePWMSegments(samples []float32, numSegments int) []float64 {
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
		dutyValues = append(dutyValues, calculateDutyCycle(segment))
		start = end
	}

	return dutyValues
}

// findDutyCycleRange finds min and max duty cycle from segments
func findDutyCycleRange(dutyValues []float64) (float64, float64) {
	return findMinMax(dutyValues)
}

// analyseEnvelopeShape determines if envelope is linear or exponential
func analyseEnvelopeShape(samples []float32) string {
	// Simple shape analysis based on amplitude curve
	var deltas []float64
	for i := 1; i < len(samples); i++ {
		deltas = append(deltas, math.Abs(float64(samples[i]-samples[i-1])))
	}

	variance := calculateVariance(deltas)
	if variance > DEFAULT_TOLERANCE/2 {
		return "exponential"
	}
	return "linear"
}

// analyseSinePurity calculates sine wave purity
func analyseSinePurity(samples []float32) float64 {
	if len(samples) < MIN_ANALYSIS_SAMPLES {
		return 0.0
	}

	// Find maximum amplitude
	_, maxAmplitude := findMinMaxFloat32(samples)
	if maxAmplitude == 0 {
		return AMPLITUDE_NORMAL // Flat line is considered pure
	}

	minProminence := float32(maxAmplitude * MIN_PEAK_PROMINENCE_FACTOR)
	peaks := detectPeaks(samples, minProminence)

	if len(peaks) < MIN_PEAKS_FOR_ANALYSIS {
		return 0.0
	}

	// Calculate normalized standard deviation
	var sum, sumSquares float64
	for _, p := range peaks {
		sum += p
		sumSquares += p * p
	}

	mean := sum / float64(len(peaks))
	variance := (sumSquares / float64(len(peaks))) - (mean * mean)
	stdDev := math.Sqrt(variance)

	// Normalize purity score (0-1 scale)
	purity := AMPLITUDE_NORMAL - (stdDev / mean)
	if purity < 0 {
		return MIN_SIGNAL_AMPLITUDE
	}
	return math.Max(MIN_SIGNAL_AMPLITUDE, math.Min(AMPLITUDE_NORMAL, purity))
}

// analyseSinePhase calculates phase offset
func analyseSinePhase(samples []float32) float64 {
	crossings := detectZeroCrossings(samples, true)
	if len(crossings) > 0 {
		return crossings[0] / float64(len(samples))
	}
	return NORMAL_PHASE_VALUE
}

// AudioAnalyzer provides methods for analyzing audio signals

func NewAudioAnalyzer(samples []float32) *AudioAnalyzer {
	return &AudioAnalyzer{
		samples:    samples,
		sampleRate: float64(SAMPLE_RATE),
	}
}

// Core analysis methods
func (a *AudioAnalyzer) Frequency() float64 {
	crossings := detectZeroCrossings(a.samples, true)
	return calculateFrequency(crossings, a.sampleRate)
}

func (a *AudioAnalyzer) Amplitude() float64 {
	min, max := findMinMaxFloat32(a.samples)
	return (max - min) / 2
}

func (a *AudioAnalyzer) DutyCycle() float64 {
	return calculateDutyCycle(a.samples)
}

func (a *AudioAnalyzer) Characteristics() SignalCharacteristics {
	return analyseSignalCharacteristics(a.samples, a.sampleRate)
}

func (a *AudioAnalyzer) SinePurity() float64 {
	return analyseSinePurity(a.samples)
}

func (a *AudioAnalyzer) SinePhase() float64 {
	return analyseSinePhase(a.samples)
}

func (a *AudioAnalyzer) HarmonicContent() float64 {
	return calculateHarmonicContent(a.samples)
}

func (a *AudioAnalyzer) OddHarmonics() float64 {
	return measureOddHarmonics(a.samples)
}

func (a *AudioAnalyzer) EvenHarmonics() float64 {
	return measureEvenHarmonics(a.samples)
}

func (a *AudioAnalyzer) DecayTime() float64 {
	return measureDecayTime(a.samples)
}

func (a *AudioAnalyzer) EnvelopeShape() string {
	return analyseEnvelopeShape(a.samples)
}

func (a *AudioAnalyzer) PWMSegments(numSegments int) []float64 {
	return analysePWMSegments(a.samples, numSegments)
}

func (a *AudioAnalyzer) RMS() float64 {
	return calculateRMS(a.samples)
}

func (a *AudioAnalyzer) Mean() float64 {
	return calculateMean(a.samples)
}

// Analysis result methods
func (a *AudioAnalyzer) AnalyzeBasic() map[string]float64 {
	return map[string]float64{
		"frequency": a.Frequency(),
		"amplitude": a.Amplitude(),
		"dutyCycle": a.DutyCycle(),
	}
}

func (a *AudioAnalyzer) AnalyzePWM() map[string]float64 {
	segments := a.PWMSegments(SAMPLES_SEGMENT_COUNT)
	minDuty, maxDuty := findDutyCycleRange(segments)
	return map[string]float64{
		"pwmDepth": (maxDuty - minDuty) / 2,
	}
}

func (a *AudioAnalyzer) AnalyzeSweep(expectIncreasing bool) map[string]float64 {
	n := len(a.samples)
	segmentSize := n / SAMPLES_SEGMENT_COUNT / 2
	frontSamples := a.samples[:segmentSize]
	backSamples := a.samples[n-segmentSize:]

	frontAnalyzer := NewAudioAnalyzer(frontSamples)
	backAnalyzer := NewAudioAnalyzer(backSamples)

	frontFreq := frontAnalyzer.Frequency()
	backFreq := backAnalyzer.Frequency()

	var freqDiff float64
	if expectIncreasing {
		freqDiff = backFreq - frontFreq
	} else {
		freqDiff = frontFreq - backFreq
	}

	return map[string]float64{
		"frontFreq": frontFreq,
		"backFreq":  backFreq,
		"freqDiff":  freqDiff,
	}
}

func (a *AudioAnalyzer) AnalyzeSync(masterFreq float64) map[string]float64 {
	crossings := detectZeroCrossings(a.samples, true)

	var intervals []float64
	for i := 1; i < len(crossings); i++ {
		intervals = append(intervals, crossings[i]-crossings[i-1])
	}

	avgInterval := 0.0
	if len(intervals) > MIN_ANALYSIS_SAMPLES {
		var sum float64
		for _, interval := range intervals {
			sum += interval
		}
		avgInterval = sum / float64(len(intervals))
	}

	masterPeriod := a.sampleRate / masterFreq
	ratio := avgInterval / masterPeriod

	return map[string]float64{
		"syncRatio": ratio,
		"purity":    a.SinePurity(),
	}
}

func (a *AudioAnalyzer) AnalyzeEnvelope() map[string]float64 {
	shape := a.EnvelopeShape()
	decayTime := a.DecayTime()
	oddHarmonics := a.OddHarmonics()
	evenHarmonics := a.EvenHarmonics()

	shapeValue := 0.0
	if shape == "exponential" {
		shapeValue = 1.0
	}

	return map[string]float64{
		"decayTime":          decayTime,
		"oddHarmonics":       oddHarmonics,
		"evenHarmonics":      evenHarmonics,
		"shapeIsExponential": shapeValue,
	}
}

func (a *AudioAnalyzer) AnalyzeWaveformPurity() map[string]float64 {
	return map[string]float64{
		"purity":          a.SinePurity(),
		"harmonicContent": a.HarmonicContent(),
		"phase":           a.SinePhase(),
	}
}

func (a *AudioAnalyzer) AnalyzeDynamicRange() map[string]float64 {
	min, max := findMinMaxFloat32(a.samples)
	maxAmplitude := math.Max(math.Abs(min), math.Abs(max))
	rms := a.RMS()
	crestFactor := maxAmplitude / rms

	return map[string]float64{
		"maxAmplitude": maxAmplitude,
		"rms":          rms,
		"crestFactor":  crestFactor,
		"dynamicRange": math.Log10(crestFactor) * 20.0,
	}
}

func (a *AudioAnalyzer) AnalyzeSpectral(expectedFreq float64) map[string]float64 {
	detectedFreq := a.Frequency()
	freqAccuracy := math.Abs(detectedFreq-expectedFreq) / expectedFreq

	oddHarmonicStrength := a.OddHarmonics()
	evenHarmonicStrength := a.EvenHarmonics()
	harmonicRatio := oddHarmonicStrength / math.Max(evenHarmonicStrength, 0.0001)

	return map[string]float64{
		"detectedFrequency":    detectedFreq,
		"frequencyAccuracy":    freqAccuracy,
		"oddHarmonicStrength":  oddHarmonicStrength,
		"evenHarmonicStrength": evenHarmonicStrength,
		"harmonicRatio":        harmonicRatio,
	}
}

// Helper function to get value from map with default
func getValueOrDefault(params map[string]uint32, key string, defaultValue uint32) uint32 {
	if val, ok := params[key]; ok {
		return val
	}
	return defaultValue
}

// RegisterBuilder provides a fluent interface for building register configurations
type RegisterBuilder struct {
	writes []RegisterWrite
}

// NewRegisterBuilder creates a new RegisterBuilder
func NewRegisterBuilder() *RegisterBuilder {
	return &RegisterBuilder{writes: make([]RegisterWrite, 0)}
}

// Write adds a register write operation to the builder
func (b *RegisterBuilder) Write(register, value uint32) *RegisterBuilder {
	b.writes = append(b.writes, RegisterWrite{Register: register, Value: value})
	return b
}

// IfNonZero adds a register write only if the value is non-zero
func (b *RegisterBuilder) IfNonZero(register, value uint32) *RegisterBuilder {
	if value > 0 {
		b.Write(register, value)
	}
	return b
}

// Enable adds an enable operation for the given register
func (b *RegisterBuilder) Enable(register uint32) *RegisterBuilder {
	return b.Write(register, AudioConstants.Channel.Control.RegisterEnable)
}

// Apply executes all register write operations
func (b *RegisterBuilder) Apply() {
	for _, write := range b.writes {
		chip.HandleRegisterWrite(write.Register, write.Value)
	}
}

// createEmptyProcessedConfig creates an empty ProcessedTestConfig
func createEmptyProcessedConfig(config TestConfig) ProcessedTestConfig {
	return ProcessedTestConfig{
		Name:         config.Name,
		Category:     config.Category,
		Description:  config.Description,
		Channel:      config.Channel,
		Effect:       config.Effect,
		PreReset:     config.PreReset,
		PostReset:    config.PostReset,
		Analyser:     config.Analyser,
		Params:       config.Params,
		Registers:    make(map[uint32]uint32),
		EffectParams: make(map[string]uint32),
		Expected:     make(map[string]ExpectedMetric),
	}
}

// processNumericValues processes numeric fields in TestConfig
func processNumericValues(processed *ProcessedTestConfig, config TestConfig) error {
	if config.Frequency != nil {
		freq, err := resolveValue(config.Frequency)
		if err != nil {
			return fmt.Errorf("invalid frequency: %v", err)
		}
		processed.Frequency = uint32(freq)
	}

	if config.Volume != nil {
		vol, err := resolveValue(config.Volume)
		if err != nil {
			return fmt.Errorf("invalid volume: %v", err)
		}
		processed.Volume = uint32(vol)
	}

	if config.DutyCycle != nil {
		duty, err := resolveValue(config.DutyCycle)
		if err != nil {
			return fmt.Errorf("invalid duty cycle: %v", err)
		}
		processed.DutyCycle = uint32(duty)
	}

	return nil
}

// processEffectParams processes effect parameters in TestConfig
func processEffectParams(processed *ProcessedTestConfig, config TestConfig) error {
	for key, val := range config.EffectParams {
		resolved, err := resolveValue(val)
		if err != nil {
			return fmt.Errorf("invalid effect parameter %s: %v", key, err)
		}
		processed.EffectParams[key] = uint32(resolved)
	}
	return nil
}

// processRegisters processes register configurations in TestConfig
func processRegisters(processed *ProcessedTestConfig, config TestConfig) error {
	for regStr, val := range config.Registers {
		regAddr, exists := registerMapping[regStr]
		if !exists {
			return fmt.Errorf("unknown register: %s", regStr)
		}

		resolved, err := resolveValue(val)
		if err != nil {
			return fmt.Errorf("invalid register value for %s: %v", regStr, err)
		}
		processed.Registers[regAddr] = uint32(resolved)
	}
	return nil
}

// processDuration processes duration field in TestConfig
func processDuration(processed *ProcessedTestConfig, config TestConfig) error {
	if config.Duration != nil {
		duration, err := resolveDuration(config.Duration)
		if err != nil {
			return fmt.Errorf("invalid duration: %v", err)
		}
		processed.Duration = duration
	} else {
		processed.Duration = time.Duration(AudioConstants.Timing.CaptureShortMs) * time.Millisecond
	}
	return nil
}

// processExpectedValues processes expected values in TestConfig
func processExpectedValues(processed *ProcessedTestConfig, config TestConfig) error {
	for metric, exp := range config.Expected {
		val, err := resolveValue(exp.Value)
		if err != nil {
			return fmt.Errorf("invalid expected value for %s: %v", metric, err)
		}

		tol, err := resolveValue(exp.Tolerance)
		if err != nil {
			return fmt.Errorf("invalid tolerance for %s: %v", metric, err)
		}

		processed.Expected[metric] = ExpectedMetric{
			Expected:  val,
			Tolerance: tol,
		}
	}
	return nil
}

func processConfig(config TestConfig) (ProcessedTestConfig, error) {
	processed := createEmptyProcessedConfig(config)

	// Process all config components
	if err := processNumericValues(&processed, config); err != nil {
		return processed, err
	}

	if err := processEffectParams(&processed, config); err != nil {
		return processed, err
	}

	if err := processRegisters(&processed, config); err != nil {
		return processed, err
	}

	if err := processDuration(&processed, config); err != nil {
		return processed, err
	}

	if err := processExpectedValues(&processed, config); err != nil {
		return processed, err
	}

	return processed, nil
}

// Effect configuration methods

// ConfigurePWM configures PWM effect on square wave
func (b *RegisterBuilder) ConfigurePWM(depth, rate uint32, baseDuty uint32) *RegisterBuilder {
	if baseDuty == 0 {
		baseDuty = AudioConstants.DutyCycle.Half
	}
	b.Write(SQUARE_DUTY, (depth<<8)|baseDuty)
	b.Write(SQUARE_PWM_CTRL, AudioConstants.Modulation.PWM.EnableBit|rate)
	return b
}

// ConfigureSweep configures frequency sweep for specified channel
func (b *RegisterBuilder) ConfigureSweep(channel string, direction, period, shift uint32) *RegisterBuilder {
	var sweepReg uint32
	switch channel {
	case "square":
		sweepReg = SQUARE_SWEEP
	case "triangle":
		sweepReg = TRI_SWEEP
	case "sine":
		sweepReg = SINE_SWEEP
	default:
		return b
	}

	b.Write(sweepReg, AudioConstants.Modulation.Sweep.EnableBit|(period<<4)|direction|shift)
	return b
}

// ConfigureSync configures hard sync for specified channel
func (b *RegisterBuilder) ConfigureSync(channel string, sourceIdx uint32) *RegisterBuilder {
	var syncReg uint32
	switch channel {
	case "triangle":
		syncReg = SYNC_SOURCE_CH1
	case "sine":
		syncReg = SYNC_SOURCE_CH2
	default:
		return b
	}

	b.Write(syncReg, sourceIdx)
	return b
}

// ConfigureFilter configures audio filter
func (b *RegisterBuilder) ConfigureFilter(filterType, cutoff, resonance uint32) *RegisterBuilder {
	b.Write(FILTER_TYPE, filterType)
	b.Write(FILTER_CUTOFF, cutoff)
	b.Write(FILTER_RESONANCE, resonance)
	b.Write(FILTER_CTRL, AudioConstants.Channel.Control.Enabled)
	return b
}

// ConfigureEnvelope configures ADSR envelope for specified channel
func (b *RegisterBuilder) ConfigureEnvelope(channel string, attack, decay, sustain, release uint32) *RegisterBuilder {
	switch channel {
	case "square":
		b.Write(SQUARE_ATK, attack)
		b.Write(SQUARE_DEC, decay)
		b.Write(SQUARE_SUS, sustain)
		b.Write(SQUARE_REL, release)
	case "triangle":
		b.Write(TRI_ATK, attack)
		b.Write(TRI_DEC, decay)
		b.Write(TRI_SUS, sustain)
		b.Write(TRI_REL, release)
	case "sine":
		b.Write(SINE_ATK, attack)
		b.Write(SINE_DEC, decay)
		b.Write(SINE_SUS, sustain)
		b.Write(SINE_REL, release)
	case "noise":
		b.Write(NOISE_ATK, attack)
		b.Write(NOISE_DEC, decay)
		b.Write(NOISE_SUS, sustain)
		b.Write(NOISE_REL, release)
	}
	return b
}

// ConfigureOverdrive configures audio overdrive
func (b *RegisterBuilder) ConfigureOverdrive(value uint32) *RegisterBuilder {
	b.Write(OVERDRIVE_CTRL, value)
	return b
}

// Channel configuration methods

// ConfigureSquare configures square wave channel
func (b *RegisterBuilder) ConfigureSquare(freq, vol, duty uint32) *RegisterBuilder {
	if freq > 0 {
		b.Write(SQUARE_FREQ, freq)
	}
	if vol > 0 {
		b.Write(SQUARE_VOL, vol)
	}
	if duty > 0 {
		b.Write(SQUARE_DUTY, duty)
	}
	b.Enable(SQUARE_CTRL)
	return b
}

// ConfigureTriangle configures triangle wave channel
func (b *RegisterBuilder) ConfigureTriangle(freq, vol uint32) *RegisterBuilder {
	if freq > 0 {
		b.Write(TRI_FREQ, freq)
	}
	if vol > 0 {
		b.Write(TRI_VOL, vol)
	}
	b.Enable(TRI_CTRL)
	return b
}

// ConfigureSine configures sine wave channel
func (b *RegisterBuilder) ConfigureSine(freq, vol uint32) *RegisterBuilder {
	if freq > 0 {
		b.Write(SINE_FREQ, freq)
	}
	if vol > 0 {
		b.Write(SINE_VOL, vol)
	}
	b.Enable(SINE_CTRL)
	return b
}

// ConfigureNoise configures noise channel
func (b *RegisterBuilder) ConfigureNoise(freq, vol, noiseType uint32) *RegisterBuilder {
	if freq > 0 {
		b.Write(NOISE_FREQ, freq)
	}
	if vol > 0 {
		b.Write(NOISE_VOL, vol)
	}
	if noiseType > 0 {
		b.Write(NOISE_TYPE, noiseType)
	}
	b.Enable(NOISE_CTRL)
	return b
}

// ConfigureFromTestConfig configures audio based on ProcessedTestConfig
func (b *RegisterBuilder) ConfigureFromTestConfig(config ProcessedTestConfig) *RegisterBuilder {
	// Apply direct register writes first
	for reg, val := range config.Registers {
		b.Write(reg, val)
	}

	// Configure channel
	switch config.Channel {
	case "square":
		b.ConfigureSquare(config.Frequency, config.Volume, config.DutyCycle)
	case "triangle":
		b.ConfigureTriangle(config.Frequency, config.Volume)
	case "sine":
		b.ConfigureSine(config.Frequency, config.Volume)
	case "noise":
		noiseType := uint32(0)
		if val, ok := config.EffectParams["noiseType"]; ok {
			noiseType = val
		}
		b.ConfigureNoise(config.Frequency, config.Volume, noiseType)
	}

	// Configure effect
	if config.Effect != "" {
		switch config.Effect {
		case "pwm":
			pwmDepth := getValueOrDefault(config.EffectParams, "pwmDepth", AudioConstants.Modulation.PWM.DepthNormal)
			pwmRate := getValueOrDefault(config.EffectParams, "pwmRate", AudioConstants.Modulation.PWM.RateMedium)
			b.ConfigurePWM(pwmDepth, pwmRate, config.DutyCycle)

		case "sweep":
			direction := getValueOrDefault(config.EffectParams, "direction", AudioConstants.Modulation.Sweep.DirectionUp)
			period := getValueOrDefault(config.EffectParams, "period", AudioConstants.Modulation.Sweep.Period.Medium)
			shift := getValueOrDefault(config.EffectParams, "shift", AudioConstants.Modulation.Sweep.Shift.Medium)
			b.ConfigureSweep(config.Channel, direction, period, shift)

		case "sync":
			sourceIdx := getValueOrDefault(config.EffectParams, "sourceIndex", uint32(0))
			b.ConfigureSync(config.Channel, sourceIdx)

		case "filter":
			filterType := getValueOrDefault(config.EffectParams, "filterType", AudioConstants.Filter.Type.Lowpass)
			cutoff := getValueOrDefault(config.EffectParams, "cutoff", AudioConstants.Filter.Cutoff.Mid)
			resonance := getValueOrDefault(config.EffectParams, "resonance", AudioConstants.Filter.Resonance.Medium)
			b.ConfigureFilter(filterType, cutoff, resonance)

		case "envelope":
			attack := getValueOrDefault(config.EffectParams, "attack", AudioConstants.Envelope.Attack.Fast)
			decay := getValueOrDefault(config.EffectParams, "decay", AudioConstants.Envelope.Decay.Medium)
			sustain := getValueOrDefault(config.EffectParams, "sustain", AudioConstants.Envelope.Sustain.Medium)
			release := getValueOrDefault(config.EffectParams, "release", AudioConstants.Envelope.Release.Medium)
			b.ConfigureEnvelope(config.Channel, attack, decay, sustain, release)

		case "overdrive":
			value := getValueOrDefault(config.EffectParams, "value", AudioConstants.Channel.Volume.Mid)
			b.ConfigureOverdrive(value)
		}
	}

	return b
}
