// ahx_benchmark_test.go - Benchmarks for AHX engine and replayer hot paths

package main

import (
	"testing"
)

// BenchmarkAHXEngine_TickSample benchmarks the per-sample tick function
// This is called 48,000 times per second at 48kHz sample rate
func BenchmarkAHXEngine_TickSample(b *testing.B) {
	engine := NewAHXEngine(chip, 48000)

	// Load minimal valid AHX data
	data := createMinimalAHXData()
	if err := engine.LoadData(data); err != nil {
		b.Fatalf("LoadData failed: %v", err)
	}
	engine.SetPlaying(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.TickSample()
	}
}

// BenchmarkAHXReplayer_PlayIRQ benchmarks the per-tick IRQ function
// This is called 50-200 times per second depending on speed multiplier
func BenchmarkAHXReplayer_PlayIRQ(b *testing.B) {
	replayer := NewAHXReplayer()

	// Create a song with actual content to process
	song := createTestSong()
	replayer.InitSong(song)
	replayer.InitSubsong(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		replayer.PlayIRQ()
		// Reset to avoid song end
		if replayer.SongEndReached {
			replayer.InitSubsong(0)
		}
	}
}

// BenchmarkAHXReplayer_ProcessFrame benchmarks per-tick effect processing
// This is called 200-800 times per second (4 voices × 50-200 Hz)
func BenchmarkAHXReplayer_ProcessFrame(b *testing.B) {
	replayer := NewAHXReplayer()

	song := createTestSong()
	replayer.InitSong(song)
	replayer.InitSubsong(0)

	// Set up a voice with active effects
	voice := &replayer.Voices[0]
	voice.VibratoDepth = 4
	voice.VibratoSpeed = 8
	voice.PeriodSlideOn = true
	voice.PeriodSlideSpeed = 8
	voice.FilterOn = 1
	voice.FilterSpeed = 3
	voice.FilterLowerLimit = 4
	voice.FilterUpperLimit = 60
	voice.SquareOn = 1
	voice.SquareLowerLimit = 8
	voice.SquareUpperLimit = 24
	voice.Instrument = &song.Instruments[1]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		replayer.ProcessFrame(0)
	}
}

// BenchmarkAHXReplayer_SetAudio benchmarks voice output update
// This is called per voice per tick (4 × 50-200 = 200-800/sec)
func BenchmarkAHXReplayer_SetAudio(b *testing.B) {
	replayer := NewAHXReplayer()

	song := createTestSong()
	replayer.InitSong(song)
	replayer.InitSubsong(0)

	// Set up voice with waveform update
	voice := &replayer.Voices[0]
	voice.TrackOn = true
	voice.AudioVolume = 32
	voice.AudioPeriod = 284
	voice.PlantPeriod = true
	voice.NewWaveform = 1
	voice.WaveLength = 3
	voice.Waveform = 0
	voice.AudioSource = replayer.WaveformTab[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		voice.NewWaveform = 1
		voice.PlantPeriod = true
		replayer.SetAudio(0)
	}
}

// BenchmarkParseAHX benchmarks AHX file parsing
func BenchmarkParseAHX(b *testing.B) {
	data := createMinimalAHXData()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseAHX(data)
		if err != nil {
			b.Fatalf("ParseAHX failed: %v", err)
		}
	}
}

// BenchmarkAHXWaveforms_Generate benchmarks waveform table generation
func BenchmarkAHXWaveforms_Generate(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewAHXWaves()
	}
}

// BenchmarkAHXEngine_UpdateChannels benchmarks channel update (called per tick)
func BenchmarkAHXEngine_UpdateChannels(b *testing.B) {
	engine := NewAHXEngine(chip, 48000)

	data := createMinimalAHXData()
	if err := engine.LoadData(data); err != nil {
		b.Fatalf("LoadData failed: %v", err)
	}
	engine.SetPlaying(true)
	engine.ensureChannelsInitialized()

	// Set up voices with data
	for i := range 4 {
		engine.replayer.Voices[i].VoiceVolume = 32
		engine.replayer.Voices[i].VoicePeriod = 284
		engine.replayer.Voices[i].WaveLength = 3
		engine.replayer.Voices[i].FilterPos = 32
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.updateChannels()
	}
}

// BenchmarkAHXReplayer_ProcessStep benchmarks new row processing
func BenchmarkAHXReplayer_ProcessStep(b *testing.B) {
	replayer := NewAHXReplayer()

	song := createTestSongWithTracks()
	replayer.InitSong(song)
	replayer.InitSubsong(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		replayer.ProcessStep(0)
		// Cycle through rows
		replayer.NoteNr = (replayer.NoteNr + 1) % song.TrackLength
	}
}

// createMinimalAHXData creates valid minimal AHX data for benchmarking
func createMinimalAHXData() []byte {
	return []byte{
		'T', 'H', 'X', 0x01, // Magic + revision 1
		0x00, 0x30, // Name offset
		0xA0, 0x01, // Speed multiplier 2, 1 position
		0x00, 0x00, // Restart = 0
		0x04, // TrackLength = 4
		0x00, // TrackNr = 0
		0x01, // InstrumentNr = 1
		0x00, // SubsongNr = 0
		// Position 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Track 0 rows (4 × 3 bytes)
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		0x00, 0x00, 0x00,
		// Instrument 1 (22 bytes)
		0x40,                         // Volume = 64
		0x18,                         // WaveLength = 3, FilterSpeed = 3
		0x04, 0x40, 0x08, 0x20, 0x10, // Envelope: A=4/64, D=8/32, S=16
		0x08, 0x00, // R=8/0
		0x00, 0x00, 0x00, // Unused
		0x10,       // FilterLowerLimit
		0x00,       // VibratoDelay
		0x44,       // HardCut + VibratoDepth
		0x08,       // VibratoSpeed
		0x10, 0x30, // SquareLower/Upper
		0x02,                          // SquareSpeed
		0x30,                          // FilterUpperLimit
		0x06,                          // PList speed
		0x00,                          // PList length
		'B', 'e', 'n', 'c', 'h', 0x00, // Song name
	}
}

// createTestSong creates a test song with instruments for benchmarking
func createTestSong() *AHXFile {
	return &AHXFile{
		Revision:        1,
		SpeedMultiplier: 2,
		PositionNr:      2,
		TrackLength:     8,
		TrackNr:         1,
		InstrumentNr:    1,
		Restart:         0,
		Positions: []AHXPosition{
			{Track: [4]int{0, 0, 0, 0}},
			{Track: [4]int{1, 1, 0, 0}},
		},
		Tracks: [][]AHXStep{
			make([]AHXStep, 8), // Track 0
			make([]AHXStep, 8), // Track 1
		},
		Instruments: []AHXInstrument{
			{}, // Instrument 0 (unused)
			{
				Volume:           64,
				WaveLength:       3,
				FilterLowerLimit: 8,
				FilterUpperLimit: 56,
				FilterSpeed:      4,
				SquareLowerLimit: 8,
				SquareUpperLimit: 48,
				SquareSpeed:      2,
				VibratoDelay:     4,
				VibratoDepth:     4,
				VibratoSpeed:     8,
				Envelope: AHXEnvelope{
					AFrames: 4,
					AVolume: 64,
					DFrames: 8,
					DVolume: 32,
					SFrames: 16,
					RFrames: 8,
					RVolume: 0,
				},
				PList: AHXPList{
					Speed:   6,
					Length:  0,
					Entries: []AHXPListEntry{},
				},
			},
		},
	}
}

// createTestSongWithTracks creates a test song with populated track data
func createTestSongWithTracks() *AHXFile {
	song := createTestSong()

	// Populate track 0 with some notes
	for i := 0; i < len(song.Tracks[0]); i++ {
		song.Tracks[0][i] = AHXStep{
			Note:       24 + (i % 12),
			Instrument: 1,
			FX:         0,
			FXParam:    0,
		}
	}

	// Populate track 1 with notes and effects
	for i := 0; i < len(song.Tracks[1]); i++ {
		song.Tracks[1][i] = AHXStep{
			Note:       36 + (i % 12),
			Instrument: 1,
			FX:         i % 8,          // Vary effects
			FXParam:    (i * 16) % 256, // Vary params
		}
	}

	return song
}
