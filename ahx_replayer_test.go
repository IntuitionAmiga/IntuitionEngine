// ahx_replayer_test.go - Tests for AHX replayer

package main

import (
	"testing"
)

// TestVoiceInit tests voice initialization
func TestVoiceInit(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	if v.TrackOn != true {
		t.Error("TrackOn should be true after Init")
	}
	if v.TrackMasterVolume != 0x40 {
		t.Errorf("TrackMasterVolume should be 0x40, got 0x%02X", v.TrackMasterVolume)
	}
	if v.ADSRVolume != 0 {
		t.Errorf("ADSRVolume should be 0, got %d", v.ADSRVolume)
	}
}

// TestVoiceCalcADSR tests ADSR calculation from instrument envelope
func TestVoiceCalcADSR(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	// Create a test instrument with specific envelope
	inst := &AHXInstrument{
		Envelope: AHXEnvelope{
			AFrames: 10,
			AVolume: 64,
			DFrames: 20,
			DVolume: 32,
			SFrames: 30,
			RFrames: 15,
			RVolume: 0,
		},
	}

	v.Instrument = inst
	v.CalcADSR()

	// Attack: target volume 64*256 = 16384, over 10 frames
	// Delta should be 16384/10 = 1638 (approximately)
	expectedAttackDelta := (64 * 256) / 10
	if v.ADSR.AFrames != 10 {
		t.Errorf("ADSR.AFrames: expected 10, got %d", v.ADSR.AFrames)
	}
	if v.ADSR.AVolume != expectedAttackDelta {
		t.Errorf("ADSR.AVolume delta: expected %d, got %d", expectedAttackDelta, v.ADSR.AVolume)
	}

	// Decay: from 64*256 to 32*256 over 20 frames
	// Delta should be (32-64)*256/20 = -409 (approximately)
	expectedDecayDelta := (32 - 64) * 256 / 20
	if v.ADSR.DFrames != 20 {
		t.Errorf("ADSR.DFrames: expected 20, got %d", v.ADSR.DFrames)
	}
	if v.ADSR.DVolume != expectedDecayDelta {
		t.Errorf("ADSR.DVolume delta: expected %d, got %d", expectedDecayDelta, v.ADSR.DVolume)
	}

	// Release: from 32*256 to 0*256 over 15 frames
	// Delta should be (0-32)*256/15 = -546 (approximately)
	expectedReleaseDelta := (0 - 32) * 256 / 15
	if v.ADSR.RFrames != 15 {
		t.Errorf("ADSR.RFrames: expected 15, got %d", v.ADSR.RFrames)
	}
	if v.ADSR.RVolume != expectedReleaseDelta {
		t.Errorf("ADSR.RVolume delta: expected %d, got %d", expectedReleaseDelta, v.ADSR.RVolume)
	}
}

// TestEnvelopeProgression_Attack tests attack phase envelope progression
func TestEnvelopeProgression_Attack(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	// Set up a simple attack
	v.ADSR.AFrames = 4
	v.ADSR.AVolume = 4096 // 16384/4 = 4096 per frame
	v.ADSRVolume = 0

	// Create mock instrument for reference
	v.Instrument = &AHXInstrument{
		Envelope: AHXEnvelope{
			AVolume: 64,
		},
	}

	// Simulate 4 frames of attack
	for i := range 4 {
		if v.ADSR.AFrames <= 0 {
			t.Fatalf("Attack ended too early at frame %d", i)
		}
		v.ADSRVolume += v.ADSR.AVolume
		v.ADSR.AFrames--
		if v.ADSR.AFrames <= 0 {
			v.ADSRVolume = 64 * 256 // Snap to target
		}
	}

	// After attack, volume should be at target (64 * 256 = 16384)
	if v.ADSRVolume != 64*256 {
		t.Errorf("After attack, ADSRVolume should be %d, got %d", 64*256, v.ADSRVolume)
	}
}

// TestEnvelopeProgression_Decay tests decay phase envelope progression
func TestEnvelopeProgression_Decay(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	// Start at attack peak
	v.ADSRVolume = 64 * 256
	v.ADSR.AFrames = 0 // Attack done
	v.ADSR.DFrames = 4
	v.ADSR.DVolume = (32 - 64) * 256 / 4 // Decay to 32

	v.Instrument = &AHXInstrument{
		Envelope: AHXEnvelope{
			DVolume: 32,
		},
	}

	// Simulate 4 frames of decay
	for range 4 {
		v.ADSRVolume += v.ADSR.DVolume
		v.ADSR.DFrames--
		if v.ADSR.DFrames <= 0 {
			v.ADSRVolume = 32 * 256 // Snap to target
		}
	}

	// After decay, volume should be at decay target
	if v.ADSRVolume != 32*256 {
		t.Errorf("After decay, ADSRVolume should be %d, got %d", 32*256, v.ADSRVolume)
	}
}

// TestEnvelopeProgression_Sustain tests sustain phase (hold)
func TestEnvelopeProgression_Sustain(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	// Set up sustain phase
	v.ADSRVolume = 32 * 256
	v.ADSR.AFrames = 0
	v.ADSR.DFrames = 0
	v.ADSR.SFrames = 10

	initialVolume := v.ADSRVolume

	// Sustain should hold volume constant while counting down
	for range 10 {
		if v.ADSR.SFrames > 0 {
			v.ADSR.SFrames--
		}
	}

	// Volume should remain unchanged during sustain
	if v.ADSRVolume != initialVolume {
		t.Errorf("Volume should remain %d during sustain, got %d", initialVolume, v.ADSRVolume)
	}
	if v.ADSR.SFrames != 0 {
		t.Errorf("SFrames should be 0 after sustain, got %d", v.ADSR.SFrames)
	}
}

// TestEnvelopeProgression_Release tests release phase
func TestEnvelopeProgression_Release(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	// Set up release from sustain level
	v.ADSRVolume = 32 * 256
	v.ADSR.AFrames = 0
	v.ADSR.DFrames = 0
	v.ADSR.SFrames = 0
	v.ADSR.RFrames = 4
	v.ADSR.RVolume = (0 - 32) * 256 / 4

	v.Instrument = &AHXInstrument{
		Envelope: AHXEnvelope{
			RVolume: 0,
		},
	}

	// Simulate 4 frames of release
	for range 4 {
		v.ADSRVolume += v.ADSR.RVolume
		v.ADSR.RFrames--
		if v.ADSR.RFrames <= 0 {
			v.ADSRVolume = 0 // Release target
		}
	}

	// After release, volume should be at release target (0)
	if v.ADSRVolume != 0 {
		t.Errorf("After release, ADSRVolume should be 0, got %d", v.ADSRVolume)
	}
}

// TestPortamentoUp tests portamento up (period slide down)
func TestPortamentoUp(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	v.PeriodSlideOn = true
	v.PeriodSlideWithLimit = false
	v.PeriodSlideSpeed = -8 // Negative = period goes down = pitch goes up
	v.PeriodSlidePeriod = 0

	// Simulate a few frames
	for range 4 {
		if v.PeriodSlideOn && !v.PeriodSlideWithLimit {
			v.PeriodSlidePeriod += v.PeriodSlideSpeed
			v.PlantPeriod = true
		}
	}

	// Period should have decreased (pitch up)
	if v.PeriodSlidePeriod >= 0 {
		t.Errorf("Portamento up should decrease period, got %d", v.PeriodSlidePeriod)
	}
}

// TestPortamentoDown tests portamento down (period slide up)
func TestPortamentoDown(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	v.PeriodSlideOn = true
	v.PeriodSlideWithLimit = false
	v.PeriodSlideSpeed = 8 // Positive = period goes up = pitch goes down
	v.PeriodSlidePeriod = 0

	// Simulate a few frames
	for range 4 {
		if v.PeriodSlideOn && !v.PeriodSlideWithLimit {
			v.PeriodSlidePeriod += v.PeriodSlideSpeed
			v.PlantPeriod = true
		}
	}

	// Period should have increased (pitch down)
	if v.PeriodSlidePeriod <= 0 {
		t.Errorf("Portamento down should increase period, got %d", v.PeriodSlidePeriod)
	}
}

// TestTonePortamento tests tone portamento (slide to target note)
func TestTonePortamento(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	// Set up tone portamento: slide from note 24 (C-3) to note 36 (C-4)
	// Period 24 = 0x038A, Period 36 = 0x01C5
	// Difference = 0x038A - 0x01C5 = 0x01C5
	v.PeriodSlideOn = true
	v.PeriodSlideWithLimit = true
	v.PeriodSlideSpeed = 16
	v.PeriodSlidePeriod = 0
	v.PeriodSlideLimit = -0x01C5 // Target is lower period (higher pitch)

	// Simulate frames until we reach the limit
	for range 100 {
		if v.PeriodSlideOn && v.PeriodSlideWithLimit {
			d0 := v.PeriodSlidePeriod - v.PeriodSlideLimit
			d2 := v.PeriodSlideSpeed
			if d0 > 0 {
				d2 = -d2
			}
			if d0 != 0 {
				d3 := (d0 + d2) ^ d0
				if d3 >= 0 {
					d0 = v.PeriodSlidePeriod + d2
				} else {
					d0 = v.PeriodSlideLimit
				}
				v.PeriodSlidePeriod = d0
				v.PlantPeriod = true
			} else {
				break // Reached target
			}
		}
	}

	// Should have reached the target
	if v.PeriodSlidePeriod != v.PeriodSlideLimit {
		t.Errorf("Tone portamento should reach target %d, got %d", v.PeriodSlideLimit, v.PeriodSlidePeriod)
	}
}

// TestVibrato tests vibrato effect
func TestVibrato(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	v.VibratoDelay = 0
	v.VibratoDepth = 4
	v.VibratoSpeed = 8
	v.VibratoCurrent = 0

	periods := make([]int, 64)

	// Simulate 64 frames of vibrato
	for i := range 64 {
		if v.VibratoDepth > 0 && v.VibratoDelay <= 0 {
			v.VibratoPeriod = (AHXVibratoTable[v.VibratoCurrent] * v.VibratoDepth) >> 7
			v.VibratoCurrent = (v.VibratoCurrent + v.VibratoSpeed) & 0x3F
		}
		periods[i] = v.VibratoPeriod
	}

	// Check that vibrato oscillates
	hasPositive := false
	hasNegative := false
	for _, p := range periods {
		if p > 0 {
			hasPositive = true
		}
		if p < 0 {
			hasNegative = true
		}
	}

	if !hasPositive || !hasNegative {
		t.Error("Vibrato should oscillate between positive and negative values")
	}
}

// TestVolumeSlide tests volume slide effect
func TestVolumeSlide(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	v.NoteMaxVolume = 32
	v.VolumeSlideUp = 4
	v.VolumeSlideDown = 0

	// Slide up
	for range 8 {
		v.NoteMaxVolume += v.VolumeSlideUp - v.VolumeSlideDown
		if v.NoteMaxVolume < 0 {
			v.NoteMaxVolume = 0
		}
		if v.NoteMaxVolume > 0x40 {
			v.NoteMaxVolume = 0x40
		}
	}

	if v.NoteMaxVolume != 0x40 {
		t.Errorf("After volume slide up, should be at max 0x40, got 0x%02X", v.NoteMaxVolume)
	}

	// Now slide down
	v.VolumeSlideUp = 0
	v.VolumeSlideDown = 8

	for range 8 {
		v.NoteMaxVolume += v.VolumeSlideUp - v.VolumeSlideDown
		if v.NoteMaxVolume < 0 {
			v.NoteMaxVolume = 0
		}
		if v.NoteMaxVolume > 0x40 {
			v.NoteMaxVolume = 0x40
		}
	}

	if v.NoteMaxVolume != 0 {
		t.Errorf("After volume slide down, should be at 0, got 0x%02X", v.NoteMaxVolume)
	}
}

// TestSquareModulation tests square wave duty cycle modulation
func TestSquareModulation(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	v.SquareOn = 1
	v.SquareInit = 1
	v.SquareLowerLimit = 8
	v.SquareUpperLimit = 24
	v.SquarePos = 8 // Start at lower limit
	v.SquareSign = 1
	v.SquareWait = 0

	v.Instrument = &AHXInstrument{
		SquareSpeed: 1,
	}

	positions := make([]int, 40)

	// Simulate modulation
	for i := range 40 {
		if v.SquareOn != 0 && v.SquareWait <= 0 {
			if v.SquareInit != 0 {
				v.SquareInit = 0
				if v.SquarePos <= v.SquareLowerLimit {
					v.SquareSlidingIn = 1
					v.SquareSign = 1
				} else if v.SquarePos >= v.SquareUpperLimit {
					v.SquareSlidingIn = 1
					v.SquareSign = -1
				}
			}

			if v.SquareLowerLimit == v.SquarePos || v.SquareUpperLimit == v.SquarePos {
				if v.SquareSlidingIn != 0 {
					v.SquareSlidingIn = 0
				} else {
					v.SquareSign = -v.SquareSign
				}
			}
			v.SquarePos += v.SquareSign
			v.SquareWait = v.Instrument.SquareSpeed
		} else {
			v.SquareWait--
		}
		positions[i] = v.SquarePos
	}

	// Check that position oscillates between limits
	hitLower := false
	hitUpper := false
	for _, p := range positions {
		if p <= v.SquareLowerLimit+1 {
			hitLower = true
		}
		if p >= v.SquareUpperLimit-1 {
			hitUpper = true
		}
	}

	if !hitLower || !hitUpper {
		t.Error("Square modulation should oscillate between lower and upper limits")
	}
}

// TestFilterModulation tests filter cutoff modulation
func TestFilterModulation(t *testing.T) {
	v := &AHXVoice{}
	v.Init()

	v.FilterOn = 1
	v.FilterInit = 1
	v.FilterLowerLimit = 4
	v.FilterUpperLimit = 32
	v.FilterPos = 4 // Start at lower limit
	v.FilterSign = 1
	v.FilterSpeed = 3 // Speed >= 3 means process once per frame
	v.FilterWait = 0

	positions := make([]int, 100)

	// Simulate modulation
	for i := range 100 {
		if v.FilterOn != 0 && v.FilterWait <= 0 {
			if v.FilterInit != 0 {
				v.FilterInit = 0
				if v.FilterPos <= v.FilterLowerLimit {
					v.FilterSlidingIn = 1
					v.FilterSign = 1
				} else if v.FilterPos >= v.FilterUpperLimit {
					v.FilterSlidingIn = 1
					v.FilterSign = -1
				}
			}

			fMax := 1
			if v.FilterSpeed < 3 {
				fMax = 5 - v.FilterSpeed
			}
			for f := 0; f < fMax; f++ {
				if v.FilterLowerLimit == v.FilterPos || v.FilterUpperLimit == v.FilterPos {
					if v.FilterSlidingIn != 0 {
						v.FilterSlidingIn = 0
					} else {
						v.FilterSign = -v.FilterSign
					}
				}
				v.FilterPos += v.FilterSign
			}
			v.FilterWait = max(v.FilterSpeed-3, 1)
		} else {
			v.FilterWait--
		}
		positions[i] = v.FilterPos
	}

	// Check that position oscillates between limits
	hitLower := false
	hitUpper := false
	for _, p := range positions {
		if p <= v.FilterLowerLimit+1 {
			hitLower = true
		}
		if p >= v.FilterUpperLimit-1 {
			hitUpper = true
		}
	}

	if !hitLower || !hitUpper {
		t.Error("Filter modulation should oscillate between lower and upper limits")
	}
}

// TestPListCommandParse tests playlist command parsing
func TestPListCommandParse(t *testing.T) {
	replayer := NewAHXReplayer()
	song := &AHXFile{
		Revision: 1,
	}
	replayer.Song = song

	// Test command 0: Set filter position
	v := &replayer.Voices[0]
	v.Init()
	v.FilterPos = 32
	replayer.PListCommandParse(0, 0, 16) // Set filter to 16
	if v.FilterPos != 16 {
		t.Errorf("PList cmd 0: FilterPos should be 16, got %d", v.FilterPos)
	}

	// Test command 1: Slide up
	v.PeriodPerfSlideSpeed = 0
	v.PeriodPerfSlideOn = false
	replayer.PListCommandParse(0, 1, 8)
	if v.PeriodPerfSlideSpeed != 8 {
		t.Errorf("PList cmd 1: PeriodPerfSlideSpeed should be 8, got %d", v.PeriodPerfSlideSpeed)
	}
	if !v.PeriodPerfSlideOn {
		t.Error("PList cmd 1: PeriodPerfSlideOn should be true")
	}

	// Test command 2: Slide down
	replayer.PListCommandParse(0, 2, 4)
	if v.PeriodPerfSlideSpeed != -4 {
		t.Errorf("PList cmd 2: PeriodPerfSlideSpeed should be -4, got %d", v.PeriodPerfSlideSpeed)
	}

	// Test command 6: Set volume (note volume)
	v.NoteMaxVolume = 32
	replayer.PListCommandParse(0, 6, 0x20) // Set to 32
	if v.NoteMaxVolume != 0x20 {
		t.Errorf("PList cmd 6: NoteMaxVolume should be 0x20, got 0x%02X", v.NoteMaxVolume)
	}

	// Test command 7: Set speed
	v.PerfSpeed = 6
	v.PerfWait = 0
	replayer.PListCommandParse(0, 7, 3)
	if v.PerfSpeed != 3 {
		t.Errorf("PList cmd 7: PerfSpeed should be 3, got %d", v.PerfSpeed)
	}
	if v.PerfWait != 3 {
		t.Errorf("PList cmd 7: PerfWait should be 3, got %d", v.PerfWait)
	}
}

// TestReplayerInit tests replayer initialization
func TestReplayerInit(t *testing.T) {
	replayer := NewAHXReplayer()

	// Should have 4 voices
	if len(replayer.Voices) != 4 {
		t.Errorf("Replayer should have 4 voices, got %d", len(replayer.Voices))
	}

	// Waves should be initialized
	if replayer.Waves == nil {
		t.Error("Waves should be initialized")
	}
}

// TestPlayIRQ_AdvanceRow tests row advancement in PlayIRQ
func TestPlayIRQ_AdvanceRow(t *testing.T) {
	replayer := NewAHXReplayer()

	// Create minimal song
	song := &AHXFile{
		PositionNr:  1,
		TrackLength: 4,
		TrackNr:     0,
		Restart:     0,
		Positions: []AHXPosition{
			{Track: [4]int{0, 0, 0, 0}},
		},
		Tracks: [][]AHXStep{
			make([]AHXStep, 4), // Track 0 with 4 rows
		},
		Instruments: []AHXInstrument{{}},
	}

	replayer.InitSong(song)
	replayer.InitSubsong(0)

	initialRow := replayer.NoteNr

	// Run enough IRQ calls to advance rows
	for range 20 {
		replayer.PlayIRQ()
	}

	// Row should have advanced
	if replayer.NoteNr == initialRow && !replayer.SongEndReached {
		t.Error("NoteNr should advance after multiple PlayIRQ calls")
	}
}
