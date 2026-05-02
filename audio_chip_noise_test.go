package main

import "testing"

func TestNoiseModeTED8BitLFSR(t *testing.T) {
	ch := &Channel{
		waveType:  WAVE_NOISE,
		frequency: SAMPLE_RATE,
		noiseMode: NOISE_MODE_TED_8BIT,
		noiseSR:   0xFF,
	}
	expectedSR := uint32(0xFF)

	for i := 0; i < 8; i++ {
		// TED polynomial x^8+x^4+x^3+x^2+1.
		newBit := ((expectedSR >> 0) ^ (expectedSR >> 2) ^ (expectedSR >> 3) ^ (expectedSR >> 4) ^ (expectedSR >> 7)) & 1
		expectedSR = ((expectedSR << 1) | newBit) & 0xFF

		ch.generateWaveSample(float32(SAMPLE_RATE), 1.0/float32(SAMPLE_RATE))
		if ch.noiseSR != expectedSR {
			t.Fatalf("step %d: noiseSR = 0x%02X, want 0x%02X", i+1, ch.noiseSR, expectedSR)
		}
	}
}

func TestNoiseModeWriteAcceptsTED8Bit(t *testing.T) {
	chip, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("NewSoundChip: %v", err)
	}

	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_NOISEMODE, NOISE_MODE_TED_8BIT)
	if chip.channels[0].noiseMode != NOISE_MODE_TED_8BIT {
		t.Fatalf("noiseMode = %d, want %d", chip.channels[0].noiseMode, NOISE_MODE_TED_8BIT)
	}
}
