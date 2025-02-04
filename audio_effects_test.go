// audio_effects_test.go - Audio effects and modulation tests for Intuition Engine

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
	"testing"
	"time"
)

func TestSweep_Verification(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 200)

	t.Log("=== SWEEP AND MODULATION TESTS ===")
	t.Log("Verifying frequency sweep behaviors across ranges and rates")

	sweepTests := []struct {
		startFreq uint32
		period    uint32
		direction uint32
		shift     uint32
		duration  int
		desc      string
		detail    string
	}{
		{880, 0x02 << 4, 0x00, 2, 2000,
			"Space Invader Descend",
			"rapid downward sweep from high frequency creating classic arcade sound"},
		{440, 0x03 << 4, 0x08, 3, 3000,
			"Police Siren Rise",
			"smooth upward frequency glide with moderate rate and range"},
		{110, 0x01 << 4, 0x08, 2, 2000,
			"Fast Bass Rise",
			"quick upward sweep from sub-bass through mid frequencies"},
		{1760, 0x04 << 4, 0x00, 2, 4000,
			"High Frequency Fall",
			"gradual descent from brilliant upper register through full range"},
		{440, 0x02 << 4, 0x08, 4, 3000,
			"Gentle Upward Glide",
			"subtle rising sweep with small frequency steps"},
	}

	for _, test := range sweepTests {
		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Configuration:")
		t.Logf("- Start Frequency: %dHz", test.startFreq)
		t.Logf("- Sweep Period: %d, Direction: %s, Step Shift: %d",
			test.period>>4,
			map[uint32]string{0: "Down", 0x08: "Up"}[test.direction],
			test.shift)
		t.Logf("Character: %s", test.detail)

		chip.HandleRegisterWrite(SQUARE_FREQ, test.startFreq)
		chip.HandleRegisterWrite(SQUARE_SWEEP, 0x80|test.period|test.direction|test.shift)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(time.Duration(test.duration) * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(500 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(SQUARE_SWEEP, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
func TestGlobalFiltering(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 180)
	chip.HandleRegisterWrite(SINE_VOL, 180)

	t.Log("=== GLOBAL FILTER SYSTEM TEST ===")
	t.Log("Demonstrating musical filter applications with dramatic modulation")

	// 1. Filter Types with Square Wave
	t.Log("\n[1] FILTER TYPE CHARACTERISTICS")
	t.Log("Rich harmonic filtering using square wave source and precise resonance control")

	filterTests := []struct {
		ftype  uint32
		cutoff uint32
		res    uint32
		freq   uint32
		desc   string
		effect string
		mod    uint32
	}{
		{1, 64, 220, 440,
			"Deep Resonant Bass Filter",
			"massive low frequency emphasis with harmonic overtones creating analog synth character",
			128},
		{1, 128, 200, 880,
			"Classic Analog Low-pass",
			"iconic synthesizer tone with perfect balance of fundamentals and harmonics",
			160},
		{2, 128, 220, 440,
			"Brilliant High-pass Resonance",
			"shimmering high frequency emphasis with singing upper harmonics",
			192},
		{3, 128, 250, 440,
			"Resonant Peak Synthesis",
			"aggressive tonal focus creating lead synthesizer voice with perfect cut-through",
			220},
	}

	for _, f := range filterTests {
		t.Logf("\n=== %s ===", f.desc)
		t.Logf("Configuration: Filter Type %d, Cutoff %d/255, Resonance %d/255", f.ftype, f.cutoff, f.res)
		t.Logf("Base Frequency: %dHz with modulation depth %d/255", f.freq, f.mod)
		t.Logf("Character: %s", f.effect)

		chip.HandleRegisterWrite(SQUARE_FREQ, f.freq)
		chip.HandleRegisterWrite(FILTER_TYPE, f.ftype)
		chip.HandleRegisterWrite(FILTER_CUTOFF, f.cutoff)
		chip.HandleRegisterWrite(FILTER_RESONANCE, f.res)
		chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2)
		chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, f.mod)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(SINE_FREQ, 2)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(2500 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Resonance Emphasis
	t.Log("\n[2] RESONANCE CHARACTERISTICS")
	t.Log("Musical resonance exploration using sine waves and synchronized filter modulation")

	resonanceTests := []struct {
		res     uint32
		freq    uint32
		cutoff  uint32
		modfreq uint32
		desc    string
		effect  string
	}{
		{180, 440, 100, 2,
			"Musical Filter Voice",
			"smooth resonant sweep creating synthetic vocal formant with perfect intonation"},
		{220, 880, 128, 4,
			"Crystalline Resonance",
			"bell-like tonal quality with precise harmonic focus and natural movement"},
		{250, 440, 160, 8,
			"Extreme Resonant Animation",
			"intense tonal emphasis creating dramatic synthesizer character with aggressive presence"},
	}

	for _, r := range resonanceTests {
		t.Logf("\n=== %s ===", r.desc)
		t.Logf("Configuration: Resonance %d/255, Base Freq %dHz", r.res, r.freq)
		t.Logf("Modulation: %dHz rate with cutoff center %d", r.modfreq, r.cutoff)
		t.Logf("Character: %s", r.effect)

		chip.HandleRegisterWrite(SINE_FREQ, r.freq)
		chip.HandleRegisterWrite(FILTER_TYPE, 3)
		chip.HandleRegisterWrite(FILTER_CUTOFF, r.cutoff)
		chip.HandleRegisterWrite(FILTER_RESONANCE, r.res)
		chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2)
		chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 200)

		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(2500 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Dynamic Modulation
	t.Log("\n[3] DYNAMIC FILTER MODULATION")
	t.Log("Complex filter animation using synchronized modulation sources")

	modTests := []struct {
		modFreq  uint32
		amount   uint32
		baseFreq uint32
		res      uint32
		desc     string
		effect   string
		detail   string
	}{
		{1, 255, 440, 220,
			"Epic Filter Sweep",
			"massive timbral transformation with perfect resonant emphasis",
			"Slow majestic sweep demonstrating complete harmonic range with precise resonant peaks"},
		{4, 220, 880, 200,
			"Rhythmic Filter Motion",
			"pulsing synthesizer character with dynamic spectral movement",
			"Synchronized modulation creating perfect musical animation of harmonics"},
		{8, 255, 440, 250,
			"Complex Timbral Animation",
			"aggressive filter modulation producing intense synthetic texture",
			"Rapid modulation showcasing extreme resonance control with perfect tracking"},
	}

	chip.HandleRegisterWrite(FILTER_TYPE, 1)
	chip.HandleRegisterWrite(FILTER_CUTOFF, 128)

	for _, m := range modTests {
		t.Logf("\n=== %s ===", m.desc)
		t.Logf("Configuration: Mod Rate %dHz, Depth %d/255, Resonance %d/255",
			m.modFreq, m.amount, m.res)
		t.Logf("Source: Square wave at %dHz", m.baseFreq)
		t.Logf("Primary Effect: %s", m.effect)
		t.Logf("Detailed Character: %s", m.detail)

		chip.HandleRegisterWrite(SQUARE_FREQ, m.baseFreq)
		chip.HandleRegisterWrite(SINE_FREQ, m.modFreq)
		chip.HandleRegisterWrite(FILTER_RESONANCE, m.res)
		chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2)
		chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, m.amount)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(3000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(FILTER_TYPE, 0)
	chip.HandleRegisterWrite(FILTER_CUTOFF, 0)
	chip.HandleRegisterWrite(FILTER_RESONANCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}

func TestFilter_ExtendedModulation(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(NOISE_VOL, 180)
	chip.HandleRegisterWrite(SINE_VOL, 200)
	chip.HandleRegisterWrite(SQUARE_VOL, 180)

	t.Log("=== EXTENDED FILTER MODULATION TEST ===")
	t.Log("Exploring extreme filter modulation capabilities and cross-modulation effects")

	// 1. Filter Modulation Depth
	t.Log("\n[1] MODULATION DEPTH EXTREMES")
	t.Log("Demonstrating full range of filter modulation intensity using noise source")

	depthTests := []struct {
		amount  uint32
		modFreq uint32
		cutoff  uint32
		res     uint32
		atk     uint32
		rel     uint32
		desc    string
		effect  string
	}{
		{96, 1, 128, 180, 50, 50, "Light Modulation Depth",
			"gentle filter sweeps with subtle resonance pulsing"},
		{96, 2, 128, 200, 0, 0, "Moderate Filter Animation",
			"clear timbral evolution with controlled resonant emphasis"},
		{160, 2, 128, 220, 0, 0, "Deep Filter Excursion",
			"dramatic frequency sweeps with strong character definition"},
		{224, 2, 128, 235, 0, 0, "Extreme Modulation Range",
			"massive filter movement spanning nearly full frequency range"},
		{255, 2, 128, 250, 0, 0, "Maximum Intensity",
			"complete filter sweep from sub-bass to highest harmonics"},
	}

	chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_WHITE)
	chip.HandleRegisterWrite(NOISE_FREQ, 44100)
	chip.HandleRegisterWrite(FILTER_TYPE, 1)

	for _, d := range depthTests {
		t.Logf("\n=== %s ===", d.desc)
		t.Logf("Configuration:")
		t.Logf("- Modulation Depth: %d/255", d.amount)
		t.Logf("- LFO Rate: %dHz", d.modFreq)
		t.Logf("- Filter Cutoff Center: %d/255", d.cutoff)
		t.Logf("Sonic Character: %s", d.effect)

		chip.HandleRegisterWrite(FILTER_TYPE, 1)
		chip.HandleRegisterWrite(SINE_FREQ, d.modFreq)
		chip.HandleRegisterWrite(FILTER_CUTOFF, d.cutoff)
		chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 2)
		chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, d.amount)
		chip.HandleRegisterWrite(FILTER_RESONANCE, d.res)

		chip.HandleRegisterWrite(SINE_ATK, d.atk)
		chip.HandleRegisterWrite(SINE_REL, d.rel)

		chip.HandleRegisterWrite(SINE_CTRL, 3)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		time.Sleep(3000 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)

		// Reset envelope settings
		chip.HandleRegisterWrite(SINE_ATK, 0)
		chip.HandleRegisterWrite(SINE_REL, 0)
	}

	// 2. Cross-modulation Effects
	t.Log("\n[2] FILTER CROSS-MODULATION")
	t.Log("Complex timbral evolution using synchronized modulation sources")

	crossModTests := []struct {
		sourceFreq uint32
		targetFreq uint32
		amount     uint32
		res        uint32
		desc       string
		effect     string
		detail     string
	}{
		{4, 440, 200, 220, "Rhythmic Filter Pattern",
			"pulsing spectral animation with perfect musical timing",
			"LFO creates synchronized filter movement enhancing harmonic content"},
		{8, 880, 200, 180, "Audio-rate Modulation",
			"complex harmonic interaction through rapid filter changes",
			"Fast modulation produces additional sidebands and rich overtones"},
		{16, 220, 200, 250, "Extreme Timbral Evolution",
			"aggressive character transformation with intense modulation",
			"Ultra-fast filter sweeps create distinctive synthetic texture"},
	}

	for _, c := range crossModTests {
		t.Logf("\n=== %s ===", c.desc)
		t.Logf("Configuration:")
		t.Logf("- Modulation Rate: %dHz", c.sourceFreq)
		t.Logf("- Target Frequency: %dHz", c.targetFreq)
		t.Logf("- Modulation Depth: %d/255", c.amount)
		t.Logf("- Resonance: %d/255", c.res)
		t.Logf("Primary Effect: %s", c.effect)
		t.Logf("Technical Detail: %s", c.detail)

		chip.HandleRegisterWrite(FILTER_TYPE, 1)
		chip.HandleRegisterWrite(SINE_FREQ, c.sourceFreq)
		chip.HandleRegisterWrite(SQUARE_FREQ, c.targetFreq)
		chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, c.amount)
		chip.HandleRegisterWrite(FILTER_RESONANCE, c.res)

		chip.HandleRegisterWrite(SINE_CTRL, 3)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(3000 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(NOISE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(NOISE_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
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

func TestOverdrive_Saturation(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 200)
	chip.HandleRegisterWrite(TRI_VOL, 200)
	chip.HandleRegisterWrite(SINE_VOL, 200)

	t.Log("=== OVERDRIVE AND SATURATION TEST ===")
	t.Log("Testing analog-style overdrive characteristics across waveforms")

	// 1. Progressive Saturation
	t.Log("\n[1] PROGRESSIVE SATURATION STAGES")
	driveTests := []struct {
		level  uint32
		freq   uint32
		desc   string
		effect string
	}{
		{32, 440, "Subtle Warming Stage",
			"gentle tube-like harmonics enhancing fundamental"},
		{64, 440, "Light Analog Saturation",
			"natural compression with soft harmonic enhancement"},
		{96, 440, "Classic Overdrive Character",
			"rich even-order harmonics with musical clipping"},
		{128, 440, "Vintage Distortion",
			"aggressive odd-order harmonics with tight compression"},
		{192, 440, "Heavy Saturation",
			"intense harmonic generation with natural limiting"},
		{255, 440, "Maximum Drive",
			"extreme wavefolding creating complex upper harmonics"},
	}

	for _, d := range driveTests {
		t.Logf("\n=== %s ===", d.desc)
		t.Logf("Drive Level: %d/255", d.level)
		t.Logf("Character: %s", d.effect)

		chip.HandleRegisterWrite(SQUARE_FREQ, d.freq)
		chip.HandleRegisterWrite(OVERDRIVE_CTRL, d.level)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Waveform Response
	t.Log("\n[2] WAVEFORM-SPECIFIC RESPONSES")
	waves := []struct {
		ctrlReg uint32
		freqReg uint32
		freq    uint32
		drive   uint32
		desc    string
		effect  string
	}{
		{SQUARE_CTRL, SQUARE_FREQ, 440, 128,
			"Hard-clipped Square Wave",
			"aggressive saturation emphasizing odd harmonics"},
		{TRI_CTRL, TRI_FREQ, 440, 128,
			"Saturated Triangle Wave",
			"smooth overdrive with progressive harmonic generation"},
		{SINE_CTRL, SINE_FREQ, 440, 128,
			"Distorted Sine Wave",
			"pure harmonic creation from fundamental"},
	}

	for _, w := range waves {
		t.Logf("\n=== %s ===", w.desc)
		t.Logf("Drive: %d/255, Frequency: %dHz", w.drive, w.freq)
		t.Logf("Character: %s", w.effect)

		chip.HandleRegisterWrite(w.freqReg, w.freq)
		chip.HandleRegisterWrite(OVERDRIVE_CTRL, w.drive)
		chip.HandleRegisterWrite(w.ctrlReg, 3)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(w.ctrlReg, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Musical Applications
	t.Log("\n[3] MUSICAL OVERDRIVE APPLICATIONS")
	musical := []struct {
		freq   uint32
		drive  uint32
		desc   string
		effect string
	}{
		{220, 96, "Overdriven Bass Lead",
			"powerful low-end with rich harmonic presence"},
		{440, 128, "Classic Rock Lead",
			"singing sustain with perfect harmonic balance"},
		{880, 192, "High-gain Solo Voice",
			"screaming upper register with extreme sustain"},
	}

	for _, m := range musical {
		t.Logf("\n=== %s ===", m.desc)
		t.Logf("Base Frequency: %dHz, Drive: %d/255", m.freq, m.drive)
		t.Logf("Character: %s", m.effect)
		t.Log("Playing ascending melodic pattern...")

		chip.HandleRegisterWrite(SQUARE_FREQ, m.freq)
		chip.HandleRegisterWrite(OVERDRIVE_CTRL, m.drive)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(1000 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_FREQ, m.freq*3/2)
		time.Sleep(1000 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_FREQ, m.freq*2)
		time.Sleep(1000 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(TRI_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(TRI_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(TRI_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(OVERDRIVE_CTRL, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}

func TestReverb_SpatialEffects(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SINE_VOL, 200)
	chip.HandleRegisterWrite(SQUARE_VOL, 200)

	t.Log("=== REVERB SPATIAL EFFECTS TEST ===")
	t.Log("Demonstrating spatial depth and room simulation characteristics")

	// 1. Dry/Wet Balance
	t.Log("\n[1] DRY/WET MIX RELATIONSHIPS")
	t.Log("Exploring spatial depth through mix balance")

	mixTests := []struct {
		mix    uint32
		desc   string
		effect string
	}{
		{32, "Intimate Close Space",
			"minimal room reflection creating subtle depth enhancement"},
		{64, "Small Room Ambience",
			"natural early reflections with short decay character"},
		{128, "Medium Hall Presence",
			"balanced room simulation with clear spatial image"},
		{192, "Large Hall Atmosphere",
			"expansive space with pronounced reflection pattern"},
		{255, "Cathedral-like Space",
			"massive reverberant field with long complex decay"},
	}

	chip.HandleRegisterWrite(REVERB_DECAY, 220) // Longer base decay
	chip.HandleRegisterWrite(SINE_FREQ, 440)

	for _, m := range mixTests {
		t.Logf("\n=== %s ===", m.desc)
		t.Logf("Mix Level: %d/255", m.mix)
		t.Logf("Character: %s", m.effect)

		chip.HandleRegisterWrite(REVERB_MIX, m.mix)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(1500 * time.Millisecond) // Longer decay tail
	}

	// 2. Decay Characteristics
	t.Log("\n[2] DECAY TIME VARIATIONS")
	t.Log("Exploring different room size simulations")

	decayTests := []struct {
		decay  uint32
		mix    uint32
		desc   string
		effect string
	}{
		{64, 200, "Tight Early Reflections",
			"close-miked room sound with focused early reflections"},
		{150, 200, "Natural Room Decay",
			"balanced decay creating convincing room simulation"},
		{200, 200, "Concert Hall Reverberance",
			"long decay with complex reflection pattern"},
		{250, 180, "Infinite Ambient Space",
			"ultra-long decay creating atmospheric wash effect"},
	}

	for _, d := range decayTests {
		t.Logf("\n=== %s ===", d.desc)
		t.Logf("Decay: %d/255, Mix: %d/255", d.decay, d.mix)
		t.Logf("Character: %s", d.effect)

		chip.HandleRegisterWrite(REVERB_MIX, d.mix)
		chip.HandleRegisterWrite(REVERB_DECAY, d.decay)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(800 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(3500 * time.Millisecond) // Longer listening period
	}

	// 3. Sound Type Coloration
	t.Log("\n[3] REVERB COLORATION BY SOURCE")
	t.Log("Demonstrating frequency-dependent reverb characteristics")

	sourceTests := []struct {
		freq   uint32
		desc   string
		effect string
	}{
		{110, "Deep Reverberant Bass",
			"massive low frequency build-up with smooth decay"},
		{440, "Mid-frequency Ambience",
			"clear spatial image with natural reflection density"},
		{1760, "High Frequency Detail",
			"sparkling upper harmonics with rapid early reflection pattern"},
	}

	chip.HandleRegisterWrite(REVERB_MIX, 180)
	chip.HandleRegisterWrite(REVERB_DECAY, 200)

	for _, s := range sourceTests {
		t.Logf("\n=== %s ===", s.desc)
		t.Logf("Frequency: %dHz", s.freq)
		t.Logf("Character: %s", s.effect)

		chip.HandleRegisterWrite(SINE_FREQ, s.freq)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(2000 * time.Millisecond)
	}

	// 4. Musical Application
	t.Log("\n[4] MUSICAL REVERB DEMONSTRATION")
	t.Log("Melodic phrase with realistic hall acoustics")

	chip.HandleRegisterWrite(REVERB_MIX, 160)
	chip.HandleRegisterWrite(REVERB_DECAY, 180)

	notes := []struct {
		freq uint32
		dur  int
		desc string
	}{
		{440, 400, "root note with full depth"},
		{554, 400, "major third shimmer"},
		{659, 600, "perfect fifth bloom"},
		{554, 400, "descending reflection"},
		{440, 400, "return to root"},
		{554, 400, "rising motion"},
		{659, 400, "peak note"},
		{880, 600, "octave resolution"},
	}

	for _, note := range notes {
		t.Logf("Playing %dHz - %s", note.freq, note.desc)
		chip.HandleRegisterWrite(SQUARE_FREQ, note.freq)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(time.Duration(note.dur) * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(100 * time.Millisecond)
	}

	// Let final reverb tail decay
	t.Log("Final reverb tail decay...")
	time.Sleep(2500 * time.Millisecond)

	// Complete cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(REVERB_MIX, 0)
	chip.HandleRegisterWrite(REVERB_DECAY, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
