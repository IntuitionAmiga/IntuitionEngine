// advanced_channel_test.go - Advanced single-channel features

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

func TestSineWave_HarmonicPurity(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SINE_VOL, 255)
	chip.HandleRegisterWrite(SINE_CTRL, 0)

	t.Log("=== SINE WAVE HARMONIC PURITY TEST ===")
	t.Log("Demonstrating mathematically perfect sine wave generation")

	// 1. Amplitude Distortion Testing
	t.Log("\n[1] AMPLITUDE DISTORTION IMMUNITY")
	t.Log("Verifying perfect amplitude scaling with zero harmonic generation")
	ampTests := []struct {
		vol    uint32
		freq   uint32
		desc   string
		effect string
	}{
		{255, 440, "Maximum Amplitude Reference",
			"concert A fundamental with theoretically perfect sine shape"},
		{192, 880, "75% Amplitude Test",
			"pure A5 reproduction demonstrating linear scaling"},
		{128, 1760, "50% Level Verification",
			"A6 with zero harmonic content demonstrating perfect division"},
		{64, 3520, "25% Amplitude High Range",
			"ultra-high A7 showing clean reproduction at frequency extremes"},
		{32, 440, "12.5% Low Level Test",
			"minimal amplitude A4 verifying clean scaling at quiet levels"},
	}

	for _, test := range ampTests {
		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Configuration:")
		t.Logf("- Frequency: %dHz", test.freq)
		t.Logf("- Volume: %d/255 (%.1f%%)", test.vol, float32(test.vol)/255.0*100)
		t.Logf("Character: %s", test.effect)

		chip.HandleRegisterWrite(SINE_VOL, test.vol)
		chip.HandleRegisterWrite(SINE_FREQ, test.freq)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Spectral Purity Test
	t.Log("\n[2] SPECTRAL PURITY ACROSS FREQUENCY RANGE")
	t.Log("Verifying zero harmonic content across full frequency spectrum")
	spectralTests := []struct {
		freq   uint32
		desc   string
		effect string
	}{
		{28, "Sub-bass Fundamental Test",
			"pure 28Hz tone with zero upper harmonic content"},
		{110, "Bass Register Verification",
			"A2 demonstrating perfect low frequency reproduction"},
		{440, "Mid-range Reference",
			"A4 concert pitch showing ideal sine characteristics"},
		{1760, "High Register Test",
			"A6 demonstrating alias-free high frequency output"},
		{7040, "Ultra-high Stability Test",
			"maintaining perfect sine character at frequency extremes"},
	}

	chip.HandleRegisterWrite(SINE_VOL, 200)
	for _, test := range spectralTests {
		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Frequency: %dHz", test.freq)
		t.Logf("Character: %s", test.effect)

		chip.HandleRegisterWrite(SINE_FREQ, test.freq)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Interference Testing
	t.Log("\n[3] MULTI-CHANNEL INTERFERENCE IMMUNITY")
	t.Log("Verifying sine wave purity during multi-channel operation")

	chip.HandleRegisterWrite(SINE_FREQ, 440)
	chip.HandleRegisterWrite(SQUARE_FREQ, 440)
	chip.HandleRegisterWrite(TRI_FREQ, 440)

	chip.HandleRegisterWrite(SQUARE_VOL, 128)
	chip.HandleRegisterWrite(TRI_VOL, 128)
	chip.HandleRegisterWrite(SINE_VOL, 200)

	t.Log("\n=== Channel Interaction Test ===")
	t.Log("Phase 1: Pure Sine Reference")
	chip.HandleRegisterWrite(SINE_CTRL, 3)
	time.Sleep(1000 * time.Millisecond)

	t.Log("Phase 2: Adding Square Wave")
	t.Log("Verifying sine purity with harmonic-rich square present")
	chip.HandleRegisterWrite(SQUARE_CTRL, 3)
	time.Sleep(1000 * time.Millisecond)

	t.Log("Phase 3: Full Channel Stack")
	t.Log("Testing complete channel isolation with all waveforms")
	chip.HandleRegisterWrite(TRI_CTRL, 3)
	time.Sleep(2000 * time.Millisecond)

	// Complete cleanup
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(TRI_CTRL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(TRI_VOL, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(TRI_FREQ, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
func TestNoise_Shaping(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	time.Sleep(50 * time.Millisecond)
	chip.HandleRegisterWrite(NOISE_VOL, 200)
	time.Sleep(50 * time.Millisecond)

	t.Log("=== NOISE SHAPING AND FILTERING TEST ===")
	t.Log("Exploring noise generation with spectral filtering and modulation")

	// 1. Filter Response Testing
	filterTests := []struct {
		ftype  uint32
		cutoff uint32
		res    uint32
		desc   string
		effect string
	}{
		{1, 64, 220, "Low-pass Resonant Bass",
			"massive sub-bass emphasis with powerful resonant peak at cutoff"},
		{1, 128, 200, "Warm Mid-frequency Filter",
			"smooth filtered spectrum with natural high-end rolloff"},
		{2, 192, 200, "Bright High-pass Character",
			"crisp upper frequency emphasis with controlled resonance"},
		{3, 128, 240, "Focused Band-pass Sweep",
			"precise frequency isolation with aggressive resonant peak"},
	}

	// Configure noise source first
	chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_WHITE)
	time.Sleep(50 * time.Millisecond)
	chip.HandleRegisterWrite(NOISE_FREQ, 8000)
	time.Sleep(50 * time.Millisecond)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 3)
	time.Sleep(50 * time.Millisecond)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 255)
	time.Sleep(50 * time.Millisecond)

	for _, f := range filterTests {
		// Reset filter state between tests
		chip.HandleRegisterWrite(FILTER_TYPE, 0)
		chip.HandleRegisterWrite(FILTER_CUTOFF, 0)
		chip.HandleRegisterWrite(FILTER_RESONANCE, 0)
		time.Sleep(100 * time.Millisecond)

		t.Logf("\n=== %s ===", f.desc)
		t.Logf("Configuration:")
		t.Logf("- Filter Type: %d", f.ftype)
		t.Logf("- Cutoff: %d/255", f.cutoff)
		t.Logf("- Resonance: %d/255", f.res)
		t.Logf("Character: %s", f.effect)

		chip.HandleRegisterWrite(FILTER_TYPE, f.ftype)
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(FILTER_CUTOFF, f.cutoff)
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(FILTER_RESONANCE, f.res)
		time.Sleep(50 * time.Millisecond)

		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Noise Color Mapping
	t.Log("\n[2] SPECTRAL COLORATION")
	t.Log("Demonstrating noise color variations through frequency and mode selection")

	noiseColors := []struct {
		freq   uint32
		mode   uint32
		desc   string
		effect string
	}{
		{4000, NOISE_MODE_WHITE, "Deep Bass Noise",
			"powerful low frequency energy with natural spectrum"},
		{8000, NOISE_MODE_WHITE, "Full Range White Noise",
			"balanced frequency content across audio spectrum"},
		{16000, NOISE_MODE_WHITE, "Bright White Noise",
			"emphasis on upper frequencies creating airy texture"},
		{8000, NOISE_MODE_PERIODIC, "Pitched Noise",
			"repeating pattern creating quasi-tonal character"},
		{12000, NOISE_MODE_METALLIC, "Metallic Texture",
			"complex harmonic structure with resonant peaks"},
	}

	for _, n := range noiseColors {
		t.Logf("\n=== %s ===", n.desc)
		t.Logf("Configuration:")
		t.Logf("- Frequency: %dHz", n.freq)
		t.Logf("- Mode: %d", n.mode)
		t.Logf("Character: %s", n.effect)

		chip.HandleRegisterWrite(NOISE_MODE, n.mode)
		chip.HandleRegisterWrite(NOISE_FREQ, n.freq)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Dynamic Filter Sweep
	t.Log("\n[3] DYNAMIC FILTER MODULATION")
	t.Log("Demonstrating LFO-controlled filter movement")

	chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_WHITE)
	chip.HandleRegisterWrite(NOISE_FREQ, 8000)
	chip.HandleRegisterWrite(FILTER_TYPE, 1)
	chip.HandleRegisterWrite(FILTER_RESONANCE, 220)
	chip.HandleRegisterWrite(SINE_FREQ, 1)
	chip.HandleRegisterWrite(SINE_VOL, 255)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 200)

	t.Log("\n=== Sweeping Filter Animation ===")
	t.Log("Configuration:")
	t.Log("- Filter: Low-pass with high resonance")
	t.Log("- Modulation: 1Hz sine LFO")
	t.Log("Character: Dramatic filter sweep with strong resonant emphasis")

	chip.HandleRegisterWrite(NOISE_CTRL, 3)
	chip.HandleRegisterWrite(SINE_CTRL, 3)
	time.Sleep(4000 * time.Millisecond)

	// Complete cleanup
	chip.HandleRegisterWrite(NOISE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(NOISE_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(NOISE_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(NOISE_MODE, 0)
	chip.HandleRegisterWrite(FILTER_TYPE, 0)
	chip.HandleRegisterWrite(FILTER_CUTOFF, 0)
	chip.HandleRegisterWrite(FILTER_RESONANCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
func TestNoise_AdvancedFeatures(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(NOISE_VOL, 180)

	t.Log("=== NOISE GENERATOR ADVANCED SYNTHESIS TEST ===")
	t.Log("Demonstrating extended noise synthesis capabilities")

	// 1. Metallic Noise Edge Cases
	metallicTests := []struct {
		freq     uint32
		duration int
		desc     string
		effect   string
	}{
		{4000, 2000, "Low Metallic Resonance",
			"deep gritty texture with strong tonal character"},
		{8000, 2000, "Mid Metallic Character",
			"focused metallic timbre with clear pitch center"},
		{12000, 2000, "High Metallic Sheen",
			"bright crystalline texture with distinct harmonics"},
		{2000, 2000, "Ultra-Low Metallic",
			"massive sub-frequency with metallic modulation"},
	}

	for _, test := range metallicTests {
		chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_METALLIC)
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_FREQ, test.freq)
		time.Sleep(50 * time.Millisecond)

		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Frequency: %dHz", test.freq)
		t.Logf("Character: %s", test.effect)

		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(time.Duration(test.duration) * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Sweep with Different Noise Types
	sweepTests := []struct {
		mode     int
		freq     uint32
		dir      uint32
		period   uint32
		shift    uint32
		duration int
		desc     string
		effect   string
	}{
		{NOISE_MODE_WHITE, 8000, 0x08, 0x03 << 4, 2, 3000,
			"Swept White Noise",
			"rising frequency sweep through noise spectrum"},
		{NOISE_MODE_PERIODIC, 4000, 0x08, 0x04 << 4, 3, 3000,
			"Rising Pitched Noise",
			"ascending tonal noise with clear pitch movement"},
		{NOISE_MODE_METALLIC, 12000, 0x00, 0x02 << 4, 4, 3000,
			"Falling Metallic",
			"descending cascade of metallic resonance"},
	}

	for _, test := range sweepTests {
		chip.HandleRegisterWrite(NOISE_MODE, uint32(test.mode))
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_FREQ, test.freq)
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_SWEEP, 0x80|test.period|test.dir|test.shift)
		time.Sleep(50 * time.Millisecond)

		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Configuration:")
		t.Logf("- Base Frequency: %dHz", test.freq)
		t.Logf("- Sweep: Period %d, Direction %s, Shift %d",
			test.period>>4,
			map[uint32]string{0: "Down", 0x08: "Up"}[test.dir],
			test.shift)
		t.Logf("Character: %s", test.effect)

		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(time.Duration(test.duration) * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(300 * time.Millisecond)
	}

	// 3. Ring Modulation with Noise
	modTests := []struct {
		noiseFreq  uint32
		targetFreq uint32
		mode       int
		duration   int
		desc       string
		effect     string
	}{
		{4000, 440, NOISE_MODE_WHITE, 2000,
			"White Noise Modulation",
			"pure sine modulated by full-spectrum noise"},
		{2000, 880, NOISE_MODE_PERIODIC, 2000,
			"Pitched Noise Ring Mod",
			"harmonic interaction between pitched noise and sine"},
		{8000, 220, NOISE_MODE_METALLIC, 2000,
			"Metallic Ring Modulation",
			"complex metallic sidebands with sine fundamental"},
	}

	for _, test := range modTests {
		chip.HandleRegisterWrite(NOISE_MODE, uint32(test.mode))
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_FREQ, test.noiseFreq)
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_FREQ, test.targetFreq)
		chip.HandleRegisterWrite(SINE_VOL, 200)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 3)

		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Configuration:")
		t.Logf("- Noise: %dHz", test.noiseFreq)
		t.Logf("- Sine: %dHz", test.targetFreq)
		t.Logf("Character: %s", test.effect)

		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(time.Duration(test.duration) * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(300 * time.Millisecond)
	}

	// 4. Complex Noise Effects
	complexTests := []struct {
		mode     int
		freq     uint32
		sweep    uint32
		envAtk   uint32
		envDec   uint32
		envSus   uint32
		envRel   uint32
		duration int
		desc     string
		effect   string
	}{
		{
			NOISE_MODE_METALLIC, 8000,
			0x80 | (0x03 << 4) | 0x08 | 2,
			100, 200, 128, 300, 3000,
			"Metallic Burst",
			"rising sweep with envelope shaping metallic character",
		},
		{
			NOISE_MODE_PERIODIC, 4000,
			0x80 | (0x04 << 4) | 0x00 | 3,
			300, 100, 180, 400, 3000,
			"Falling Pitched Pad",
			"slow attack into descending pitched noise",
		},
		{
			NOISE_MODE_WHITE, 12000,
			0x80 | (0x02 << 4) | 0x08 | 4,
			50, 300, 200, 200, 3000,
			"Rising Noise Burst",
			"quick attack into ascending noise sweep",
		},
	}

	for _, test := range complexTests {
		chip.HandleRegisterWrite(NOISE_MODE, uint32(test.mode))
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_FREQ, test.freq)
		time.Sleep(50 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_SWEEP, test.sweep)
		chip.HandleRegisterWrite(NOISE_ATK, test.envAtk)
		chip.HandleRegisterWrite(NOISE_DEC, test.envDec)
		chip.HandleRegisterWrite(NOISE_SUS, test.envSus)
		chip.HandleRegisterWrite(NOISE_REL, test.envRel)

		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Configuration:")
		t.Logf("- Base Frequency: %dHz", test.freq)
		t.Logf("- Envelope: Attack %d, Decay %d, Sustain %d, Release %d",
			test.envAtk, test.envDec, test.envSus, test.envRel)
		t.Logf("Character: %s", test.effect)

		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(time.Duration(test.duration) * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(500 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(NOISE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(NOISE_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(NOISE_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(NOISE_MODE, 0)
	chip.HandleRegisterWrite(NOISE_ATK, 0)
	chip.HandleRegisterWrite(NOISE_DEC, 0)
	chip.HandleRegisterWrite(NOISE_SUS, 0)
	chip.HandleRegisterWrite(NOISE_REL, 0)
	chip.HandleRegisterWrite(NOISE_SWEEP, 0)
	chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
func TestNoise_EdgeCases(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(NOISE_VOL, 200)

	t.Log("=== NOISE MODE TRANSITION AND EDGE CASE TEST ===")
	t.Log("Demonstrating precise timing control of noise generation modes")

	// Mode transition timing tests
	t.Log("\n[1] MODE TRANSITION CHARACTERISTICS")
	t.Log("Testing seamless transitions between noise generation algorithms")

	modeTests := []struct {
		fromMode uint32
		toMode   uint32
		freq     uint32
		desc     string
		effect   string
	}{
		{NOISE_MODE_WHITE, NOISE_MODE_METALLIC, 44100,
			"white to metallic transition",
			"evolving from pure entropy to resonant structures"},
		{NOISE_MODE_METALLIC, NOISE_MODE_PERIODIC, 22050,
			"metallic to periodic",
			"transformation from metallic character to pitched texture"},
		{NOISE_MODE_PERIODIC, NOISE_MODE_WHITE, 11025,
			"periodic to white",
			"dissolving pitched components into pure noise"},
	}

	for _, mode := range modeTests {
		t.Logf("\n=== %s ===", strings.ToUpper(mode.desc))
		t.Logf("Frequency: %dHz", mode.freq)
		t.Logf("Should hear: %s", mode.effect)

		chip.HandleRegisterWrite(NOISE_MODE, mode.fromMode)
		chip.HandleRegisterWrite(NOISE_FREQ, mode.freq)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(500 * time.Millisecond)

		t.Log("Transitioning to new noise mode...")
		chip.HandleRegisterWrite(NOISE_MODE, mode.toMode)
		time.Sleep(1000 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
	}

	// High frequency metallic noise tests
	t.Log("\n[2] HIGH-FREQUENCY METALLIC NOISE RESPONSE")
	t.Log("Testing metallic noise generator at extreme frequencies")

	freqTests := []struct {
		freq uint32
		desc string
	}{
		{44100, "maximum rate metallic texture"},
		{88200, "super-high frequency artifacts"},
		{22050, "mid-range metallic resonance"},
		{11025, "low-frequency metallic character"},
	}

	for _, ft := range freqTests {
		t.Logf("\nTesting at %dHz - Should hear %s", ft.freq, ft.desc)
		chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_METALLIC)
		chip.HandleRegisterWrite(NOISE_FREQ, ft.freq)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(1000 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
	}
}
