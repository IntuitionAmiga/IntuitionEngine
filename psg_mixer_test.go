// psg_mixer_test.go - Tests for PSG mixer behavior.

package main

import "testing"

func TestPSGNoiseMixerSum(t *testing.T) {
	engine, chip := newTestPSGEngine(SAMPLE_RATE)

	// Enable noise on channels A and B, disable on C. Tone stays enabled.
	engine.WriteRegister(7, 0x20)
	engine.WriteRegister(8, 0x0F)  // A
	engine.WriteRegister(9, 0x08)  // B
	engine.WriteRegister(10, 0x04) // C (noise disabled)

	noiseVol := chip.channels[3].volume
	sum := float32(psgVolumeToDAC(0x0F, false)) + float32(psgVolumeToDAC(0x08, false))
	want := sum / NORMALISE_8BIT
	if want > 1.0 {
		want = 1.0
	}
	if noiseVol != want {
		t.Fatalf("noise volume=%.3f, want %.3f", noiseVol, want)
	}
}
