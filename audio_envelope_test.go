// audio_envelope_test.go - Comprehensive test suite for the envelope system

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
License: GPLv3 or later
*/

package main

import (
	"strings"
	"testing"
	"time"
)

func TestEnvelope_ADSRAllChannels(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)

	t.Log("=== ENVELOPE SYSTEM COMPREHENSIVE DEMONSTRATION ===")
	t.Log("Demonstrating precision envelope control across all channels")

	channels := []struct {
		name     string
		ctrlReg  uint32
		freqReg  uint32
		volReg   uint32
		atkReg   uint32
		decReg   uint32
		susReg   uint32
		relReg   uint32
		baseFreq uint32
	}{
		{
			name:     "Square Wave",
			ctrlReg:  SQUARE_CTRL,
			freqReg:  SQUARE_FREQ,
			volReg:   SQUARE_VOL,
			atkReg:   SQUARE_ATK,
			decReg:   SQUARE_DEC,
			susReg:   SQUARE_SUS,
			relReg:   SQUARE_REL,
			baseFreq: 440,
		},
		{
			name:     "Triangle Wave",
			ctrlReg:  TRI_CTRL,
			freqReg:  TRI_FREQ,
			volReg:   TRI_VOL,
			atkReg:   TRI_ATK,
			decReg:   TRI_DEC,
			susReg:   TRI_SUS,
			relReg:   TRI_REL,
			baseFreq: 440,
		},
		{
			name:     "Sine Wave",
			ctrlReg:  SINE_CTRL,
			freqReg:  SINE_FREQ,
			volReg:   SINE_VOL,
			atkReg:   SINE_ATK,
			decReg:   SINE_DEC,
			susReg:   SINE_SUS,
			relReg:   SINE_REL,
			baseFreq: 440,
		},
		{
			name:     "Noise",
			ctrlReg:  NOISE_CTRL,
			freqReg:  NOISE_FREQ,
			volReg:   NOISE_VOL,
			atkReg:   NOISE_ATK,
			decReg:   NOISE_DEC,
			susReg:   NOISE_SUS,
			relReg:   NOISE_REL,
			baseFreq: 44100,
		},
	}

	envelopeTests := []struct {
		name string
		atk  uint32
		dec  uint32
		sus  uint32
		rel  uint32
		desc string
	}{
		{
			name: "Percussive Hit",
			atk:  10,
			dec:  100,
			sus:  0,
			rel:  200,
			desc: "sharp attack, no sustain",
		},
		{
			name: "Pad Sound",
			atk:  200,
			dec:  150,
			sus:  180,
			rel:  300,
			desc: "slow attack, high sustain",
		},
		{
			name: "Pluck",
			atk:  5,
			dec:  80,
			sus:  100,
			rel:  100,
			desc: "instant attack, natural decay",
		},
		{
			name: "Swell",
			atk:  400,
			dec:  0,
			sus:  255,
			rel:  400,
			desc: "gradual rise, full sustain",
		},
		{
			name: "Complex",
			atk:  150,
			dec:  100,
			sus:  128,
			rel:  250,
			desc: "balanced ADSR shape",
		},
	}

	for _, ch := range channels {
		t.Logf("\n=== %s ENVELOPE TESTS ===", strings.ToUpper(ch.name))
		chip.HandleRegisterWrite(ch.volReg, 200)
		chip.HandleRegisterWrite(ch.freqReg, ch.baseFreq)

		if ch.name == "Noise" {
			chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_WHITE)
		}

		for _, env := range envelopeTests {
			// Reset envelope registers before each test
			chip.HandleRegisterWrite(ch.atkReg, 0)
			chip.HandleRegisterWrite(ch.decReg, 0)
			chip.HandleRegisterWrite(ch.susReg, 0)
			chip.HandleRegisterWrite(ch.relReg, 0)

			t.Logf("\n[%s]", env.name)
			chip.HandleRegisterWrite(ch.atkReg, env.atk)
			chip.HandleRegisterWrite(ch.decReg, env.dec)
			chip.HandleRegisterWrite(ch.susReg, env.sus)
			chip.HandleRegisterWrite(ch.relReg, env.rel)

			chip.HandleRegisterWrite(ch.ctrlReg, 3)
			t.Logf("Attack %dms, Decay %dms, Sustain %d/255, Release %dms",
				env.atk, env.dec, env.sus, env.rel)
			t.Logf("Should hear: %s", env.desc)
			time.Sleep(1500 * time.Millisecond)

			chip.HandleRegisterWrite(ch.ctrlReg, 1)
			t.Log("Release phase - verify smooth fade")
			time.Sleep(500 * time.Millisecond)
		}

		t.Log("\n[Musical Example]")
		notes := []uint32{440, 494, 523, 587, 659, 587, 523, 494}
		for _, freq := range notes {
			chip.HandleRegisterWrite(ch.freqReg, freq)
			chip.HandleRegisterWrite(ch.ctrlReg, 3)
			time.Sleep(300 * time.Millisecond)
			chip.HandleRegisterWrite(ch.ctrlReg, 1)
			time.Sleep(100 * time.Millisecond)
		}

		// Clean up this channel before moving to next
		chip.HandleRegisterWrite(ch.ctrlReg, 0)
		chip.HandleRegisterWrite(ch.volReg, 0)
		chip.HandleRegisterWrite(ch.freqReg, 0)
		chip.HandleRegisterWrite(ch.atkReg, 0)
		chip.HandleRegisterWrite(ch.decReg, 0)
		chip.HandleRegisterWrite(ch.susReg, 0)
		chip.HandleRegisterWrite(ch.relReg, 0)

		if ch.name == "Noise" {
			chip.HandleRegisterWrite(NOISE_MODE, 0)
		}
	}

	// Final reset of audio control
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
