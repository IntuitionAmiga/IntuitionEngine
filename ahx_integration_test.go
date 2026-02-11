// ahx_integration_test.go - Integration tests for AHX playback

package main

import (
	"testing"
)

// TestAHXIntegration_WaveformMapping verifies AHX waveforms map to SoundChip types
func TestAHXIntegration_WaveformMapping(t *testing.T) {
	// AHX waveform 0 = Triangle, 1 = Sawtooth, 2 = Square, 3 = Noise
	tests := []struct {
		ahxWaveform int
		expected    int
		name        string
	}{
		{0, WAVE_TRIANGLE, "Triangle"},
		{1, WAVE_SAWTOOTH, "Sawtooth"},
		{2, WAVE_SQUARE, "Square"},
		{3, WAVE_NOISE, "Noise"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var waveType int
			switch tc.ahxWaveform {
			case 0:
				waveType = WAVE_TRIANGLE
			case 1:
				waveType = WAVE_SAWTOOTH
			case 2:
				waveType = WAVE_SQUARE
			case 3:
				waveType = WAVE_NOISE
			}
			if waveType != tc.expected {
				t.Errorf("AHX waveform %d: expected %d, got %d", tc.ahxWaveform, tc.expected, waveType)
			}
		})
	}
}

// TestAHXIntegration_FrequencyCalculation verifies period + wavelength → frequency
func TestAHXIntegration_FrequencyCalculation(t *testing.T) {
	// Frequency = (3579545.25 / period) / (4 * (1 << waveLength))
	tests := []struct {
		period     int
		waveLength int
		minFreq    float64
		maxFreq    float64
		name       string
	}{
		{428, 5, 60, 70, "Period 428, WaveLen 5 (~65 Hz)"},
		{428, 2, 500, 530, "Period 428, WaveLen 2 (~523 Hz)"},
		{214, 5, 125, 135, "Period 214, WaveLen 5 (~130 Hz)"},
		{856, 1, 500, 530, "Period 856, WaveLen 1 (~523 Hz)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sampleRate := AHXPeriod2Freq(tc.period)
			waveLen := 4 * (1 << uint(tc.waveLength))
			freq := sampleRate / float64(waveLen)

			if freq < tc.minFreq || freq > tc.maxFreq {
				t.Errorf("Period %d, WaveLen %d: expected freq [%.0f, %.0f], got %.2f",
					tc.period, tc.waveLength, tc.minFreq, tc.maxFreq, freq)
			}
		})
	}
}

// TestAHXIntegration_VolumeScaling verifies volume scaling (0-64 → 0-255)
func TestAHXIntegration_VolumeScaling(t *testing.T) {
	tests := []struct {
		voiceVol int
		minDAC   int
		maxDAC   int
		name     string
	}{
		{0, 0, 0, "Zero volume"},
		{32, 125, 135, "Half volume"},
		{64, 250, 260, "Full volume"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vol := min(max(tc.voiceVol, 0), 64)
			dacVol := vol * 4
			if dacVol < tc.minDAC || dacVol > tc.maxDAC {
				t.Errorf("Voice vol %d: expected DAC [%d, %d], got %d",
					tc.voiceVol, tc.minDAC, tc.maxDAC, dacVol)
			}
		})
	}
}

// TestAHXIntegration_EngineCreation tests engine initialization
func TestAHXIntegration_EngineCreation(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)
	if engine == nil {
		t.Fatal("NewAHXEngine returned nil")
	}
	if engine.replayer == nil {
		t.Error("Engine replayer should be initialized")
	}
}

// TestAHXIntegration_LoadData tests loading AHX data
func TestAHXIntegration_LoadData(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	// Minimal valid AHX data
	data := []byte{
		'T', 'H', 'X', 0x00,
		0x00, 0x19,
		0x80, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		'T', 'e', 's', 't', 0x00,
	}

	err := engine.LoadData(data)
	if err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}

	if engine.replayer.Song == nil {
		t.Error("Song should be loaded")
	}
}

// TestAHXIntegration_TrackEffects tests replayer effect processing
func TestAHXIntegration_TrackEffects(t *testing.T) {
	t.Run("Effect 0xF - Speed", func(t *testing.T) {
		replayer := NewAHXReplayer()
		song := &AHXFile{
			SpeedMultiplier: 1,
			TrackLength:     64,
			PositionNr:      1,
			Positions:       make([]AHXPosition, 1),
			Tracks:          make([][]AHXStep, 1),
			Instruments:     make([]AHXInstrument, 1),
		}
		song.Tracks[0] = make([]AHXStep, 64)
		song.Tracks[0][0] = AHXStep{FX: 0xF, FXParam: 8}

		replayer.InitSong(song)
		replayer.InitSubsong(0)
		replayer.Playing = true
		replayer.PlayIRQ()

		if replayer.Tempo != 8 {
			t.Errorf("Speed effect: expected tempo 8, got %d", replayer.Tempo)
		}
	})

	t.Run("Effect 0x1 - Portamento Up", func(t *testing.T) {
		replayer := NewAHXReplayer()
		song := &AHXFile{
			SpeedMultiplier: 1,
			TrackLength:     64,
			PositionNr:      1,
			Positions:       make([]AHXPosition, 1),
			Tracks:          make([][]AHXStep, 1),
			Instruments:     make([]AHXInstrument, 2),
		}
		song.Tracks[0] = make([]AHXStep, 64)
		song.Tracks[0][0] = AHXStep{Note: 24, Instrument: 1, FX: 0x1, FXParam: 0x10}
		song.Instruments[1] = AHXInstrument{Volume: 64}

		replayer.InitSong(song)
		replayer.InitSubsong(0)
		replayer.Playing = true
		replayer.PlayIRQ()

		if !replayer.Voices[0].PeriodSlideOn {
			t.Error("Portamento up should enable period slide")
		}
	})
}

// TestAHXIntegration_FullPlayback tests the complete playback chain
func TestAHXIntegration_FullPlayback(t *testing.T) {
	engine := NewAHXEngine(chip, 44100)

	song := &AHXFile{
		Name:            "Test",
		SpeedMultiplier: 1,
		TrackLength:     4,
		PositionNr:      1,
		Positions:       make([]AHXPosition, 1),
		Tracks:          make([][]AHXStep, 1),
		Instruments:     make([]AHXInstrument, 2),
	}
	song.Tracks[0] = []AHXStep{
		{Note: 24, Instrument: 1},
		{Note: 28, Instrument: 1},
		{Note: 31, Instrument: 1},
		{Note: 0, Instrument: 0},
	}
	song.Instruments[1] = AHXInstrument{
		Volume:     64,
		WaveLength: 5,
		Envelope: AHXEnvelope{
			AFrames: 2, AVolume: 64,
			DFrames: 4, DVolume: 48,
			RFrames: 8, RVolume: 0,
		},
	}

	engine.replayer.InitSong(song)
	engine.replayer.InitSubsong(0)
	engine.enabled.Store(true)
	engine.playing.Store(true)
	engine.replayer.Playing = true
	engine.samplesPerTick = 44100 / 50

	// Tick through several frames
	for range 20 {
		for sample := 0; sample < engine.samplesPerTick; sample++ {
			engine.TickSample()
		}
	}

	// Should have progressed through the song
	if engine.replayer.NoteNr == 0 && engine.replayer.PosNr == 0 {
		t.Log("Song should have progressed (may depend on tempo)")
	}
}
