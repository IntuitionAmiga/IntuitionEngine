// psg_mixer_test.go - Tests for PSG mixer behavior.

package main

import "testing"

func TestPSGNoiseMixerRoutesPerChannel(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)

	// Enable noise on channels A and B, disable on C. Tone stays enabled.
	engine.WriteRegister(7, 0x20)
	engine.WriteRegister(8, 0x0F)  // A
	engine.WriteRegister(9, 0x08)  // B
	engine.WriteRegister(10, 0x04) // C (noise disabled)

	if got := chip.channels[0].noiseMix; got != psgVolumeGain(0x0F, false) {
		t.Fatalf("channel A noiseMix=%.3f, want %.3f", got, psgVolumeGain(0x0F, false))
	}
	if got := chip.channels[1].noiseMix; got != psgVolumeGain(0x08, false) {
		t.Fatalf("channel B noiseMix=%.3f, want %.3f", got, psgVolumeGain(0x08, false))
	}
	if got := chip.channels[2].noiseMix; got != 0 {
		t.Fatalf("channel C noiseMix=%.3f, want 0", got)
	}
	if got := chip.channels[3].volume; got != 0 {
		t.Fatalf("global noise voice volume=%.3f, want 0", got)
	}
}

func TestPSGNoiseOnlyChannelRemainsAudible(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.WriteRegister(0, 0x20)
	engine.WriteRegister(1, 0x00)
	engine.WriteRegister(6, 0x01)
	engine.WriteRegister(7, 0x37) // A noise enabled, A tone and B/C tone+noise disabled.
	engine.WriteRegister(8, 0x0F)

	if chip.channels[0].volume != 0 {
		t.Fatalf("test setup tone volume=%f, want 0", chip.channels[0].volume)
	}
	if chip.channels[0].noiseMix == 0 {
		t.Fatalf("test setup noiseMix is zero")
	}

	var sum float64
	for range 64 {
		sum += absFloat32(chip.GenerateSample())
	}
	if sum == 0 {
		t.Fatalf("noise-only PSG channel was silent")
	}
}

func TestPSGNoiseMixClockFollowsNoisePeriod(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)
	engine.SetClockHz(PSG_CLOCK_ATARI_ST)
	engine.WriteRegister(0, 0x20)
	engine.WriteRegister(1, 0x00)
	engine.WriteRegister(6, 0x10)
	engine.WriteRegister(7, 0x37)
	engine.WriteRegister(8, 0x0F)

	want := float32(PSG_CLOCK_ATARI_ST) / (16.0 * 0x10)
	if got := chip.channels[0].noiseFrequency; got != want {
		t.Fatalf("noiseFrequency=%f, want %f", got, want)
	}

	engine.WriteRegister(6, 0x02)
	want = float32(PSG_CLOCK_ATARI_ST) / (16.0 * 0x02)
	if got := chip.channels[0].noiseFrequency; got != want {
		t.Fatalf("noiseFrequency after R6 change=%f, want %f", got, want)
	}
}

func absFloat32(v float32) float64 {
	if v < 0 {
		return float64(-v)
	}
	return float64(v)
}
