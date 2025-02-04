// audio_modulation_test.go - Audio modulation tests for the Intuition Engine

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

func TestSync_OscillatorPhaseLock(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 180)
	chip.HandleRegisterWrite(TRI_VOL, 180)
	chip.HandleRegisterWrite(SINE_VOL, 180)

	t.Log("=== OSCILLATOR SYNC DEMONSTRATION ===")

	// 1. Basic Sync Effects
	t.Log("\n[1] BASIC SYNC RELATIONSHIPS")
	syncTests := []struct {
		master uint32
		slave  uint32
		desc   string
	}{
		{440, 440, "unison - reinforced fundamental"},
		{440, 660, "perfect fifth - harmonic lock"},
		{440, 880, "octave - pure upper formant"},
		{440, 1320, "third harmonic - metallic texture"},
	}

	for _, s := range syncTests {
		chip.HandleRegisterWrite(SQUARE_FREQ, s.master)
		chip.HandleRegisterWrite(TRI_FREQ, s.slave)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Master %dHz, Slave %dHz - Should hear %s", s.master, s.slave, s.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Complex Sync Chain
	t.Log("\n[2] MULTI-OSCILLATOR SYNC CHAIN")
	chainTests := []struct {
		freq1 uint32
		freq2 uint32
		freq3 uint32
		desc  string
	}{
		{440, 660, 880, "harmonic cascade"},
		{220, 440, 880, "octave stack"},
		{440, 550, 660, "close interval fusion"},
	}

	for _, c := range chainTests {
		chip.HandleRegisterWrite(SQUARE_FREQ, c.freq1)
		chip.HandleRegisterWrite(TRI_FREQ, c.freq2)
		chip.HandleRegisterWrite(SINE_FREQ, c.freq3)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 1)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("%dHz → %dHz → %dHz - Should hear %s", c.freq1, c.freq2, c.freq3, c.desc)
		time.Sleep(2500 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Musical Application
	t.Log("\n[3] SYNC SOUND DESIGN")
	sounds := []struct {
		mfreq uint32
		sfreq uint32
		duty  uint32
		desc  string
	}{
		{440, 880, 64, "sync lead tone"},
		{220, 1760, 32, "aggressive saw sync"},
		{110, 440, 192, "hollow sync texture"},
	}

	for _, s := range sounds {
		chip.HandleRegisterWrite(SQUARE_FREQ, s.mfreq)
		chip.HandleRegisterWrite(TRI_FREQ, s.sfreq)
		chip.HandleRegisterWrite(SQUARE_DUTY, s.duty)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Complex sync tone - Should hear %s", s.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// Cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(TRI_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(TRI_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(TRI_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(SQUARE_DUTY, 128)
	chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
	chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
func TestSync_ExtendedBehaviors(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 160)
	chip.HandleRegisterWrite(TRI_VOL, 160)
	chip.HandleRegisterWrite(SINE_VOL, 160)

	t.Log("=== EXTENDED OSCILLATOR SYNC TEST ===")

	// 1. Three-Oscillator Sync Chain
	t.Log("\n[1] MULTI-OSCILLATOR SYNC CASCADE")
	cascadeTests := []struct {
		freq1 uint32
		freq2 uint32
		freq3 uint32
		desc  string
	}{
		{440, 660, 880, "harmonic series cascade"},
		{440, 880, 1760, "octave stack cascade"},
		{440, 550, 733, "dissonant cascade"},
		{220, 660, 1320, "wide interval cascade"},
	}

	for _, c := range cascadeTests {
		chip.HandleRegisterWrite(SQUARE_FREQ, c.freq1)
		chip.HandleRegisterWrite(TRI_FREQ, c.freq2)
		chip.HandleRegisterWrite(SINE_FREQ, c.freq3)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0) // Triangle syncs to Square
		chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 1) // Sine syncs to Triangle

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)

		t.Logf("%dHz → %dHz → %dHz - Should hear %s", c.freq1, c.freq2, c.freq3, c.desc)
		time.Sleep(2500 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Sync with Filter Interaction
	t.Log("\n[2] SYNC + FILTER INTERACTION")
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0) // Square wave modulates filter
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 255)

	filterSyncTests := []struct {
		ftype  uint32
		cutoff uint32
		res    uint32
		desc   string
	}{
		{1, 64, 220, "low-pass sync resonance"},
		{2, 192, 220, "high-pass sync harmonics"},
		{3, 128, 240, "band-pass sync focus"},
	}

	chip.HandleRegisterWrite(SQUARE_FREQ, 440)
	chip.HandleRegisterWrite(TRI_FREQ, 880)
	chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)

	for _, f := range filterSyncTests {
		chip.HandleRegisterWrite(FILTER_TYPE, f.ftype)
		chip.HandleRegisterWrite(FILTER_CUTOFF, f.cutoff)
		chip.HandleRegisterWrite(FILTER_RESONANCE, f.res)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)

		t.Logf("Filter type %d - Should hear %s", f.ftype, f.desc)
		time.Sleep(2500 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
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
	chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
	chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(FILTER_TYPE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}

func TestRingModulation_Harmonics(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 180)
	chip.HandleRegisterWrite(SINE_VOL, 180)

	t.Log("=== RING MODULATION AND HARMONICS TEST ===")

	// 1. Basic AM Timbres
	t.Log("\n[1] AMPLITUDE MODULATION TIMBRES")
	chip.HandleRegisterWrite(SINE_FREQ, 440)

	modFreqs := []struct {
		freq uint32
		desc string
	}{
		{55, "sub-harmonic throbbing"},
		{110, "rich bass modulation"},
		{220, "metallic tremolo"},
		{440, "balanced sidebands"},
		{880, "bell-like spectrum"},
	}

	for _, mod := range modFreqs {
		chip.HandleRegisterWrite(SQUARE_FREQ, mod.freq)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("Carrier 440Hz, Modulator %dHz - Should hear %s", mod.freq, mod.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Complex Ratio Effects
	t.Log("\n[2] COMPLEX MODULATION RATIOS")
	ratioTests := []struct {
		carrier uint32
		mod     uint32
		desc    string
	}{
		{440, 293, "major third modulation"},
		{440, 330, "minor third modulation"},
		{440, 554, "perfect fifth modulation"},
		{440, 660, "major sixth modulation"},
	}

	for _, r := range ratioTests {
		chip.HandleRegisterWrite(SINE_FREQ, r.carrier)
		chip.HandleRegisterWrite(SQUARE_FREQ, r.mod)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("%dHz/%dHz - Should hear %s", r.carrier, r.mod, r.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Sound Design Applications
	t.Log("\n[3] SOUND DESIGN EXAMPLES")
	sounds := []struct {
		cfreq uint32
		mfreq uint32
		duty  uint32
		desc  string
	}{
		{440, 445, 128, "slow beating chorus"},
		{440, 880, 64, "metallic formant"},
		{440, 2200, 32, "digital bell tone"},
		{110, 1760, 192, "robotic texture"},
	}

	for _, s := range sounds {
		chip.HandleRegisterWrite(SINE_FREQ, s.cfreq)
		chip.HandleRegisterWrite(SQUARE_FREQ, s.mfreq)
		chip.HandleRegisterWrite(SQUARE_DUTY, s.duty)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("Complex modulation - Should hear %s", s.desc)
		time.Sleep(2500 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// Cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_VOL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SQUARE_FREQ, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
	chip.HandleRegisterWrite(SQUARE_DUTY, 128)
	chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
func TestRingModulation_ComplexRouting(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SINE_VOL, 180)
	chip.HandleRegisterWrite(SQUARE_VOL, 180)
	chip.HandleRegisterWrite(TRI_VOL, 180)

	t.Log("=== COMPLEX RING MODULATION TEST ===")
	t.Log("Testing advanced modulation routing with dynamic source changes and multi-target configurations")

	t.Log("\n[1] DYNAMIC MODULATION SOURCE SWITCHING")
	t.Log("Evaluating timbre changes when switching modulation sources during active notes")

	sourceTests := []struct {
		sourceFreq uint32
		targetFreq uint32
		desc       string
		phase1     string
		phase2     string
	}{
		{440, 220, "Fundamental/Sub-octave Interaction",
			"Square wave modulating sine at 440Hz/220Hz - produces hollow sub-octave texture",
			"Transitioning to triangle modulation - should hear smoother harmonic spectrum"},
		{220, 880, "Wide-Interval Modulation",
			"Deep modulation from low frequency square creates rich sidebands",
			"Switching to triangle softens the extreme harmonics while maintaining depth"},
		{880, 440, "Upper Harmonic Modulation",
			"High-frequency square modulation generates bright, metallic character",
			"Triangle modulation transforms texture to bell-like resonance"},
	}

	for _, src := range sourceTests {
		t.Logf("\n=== %s ===", src.desc)
		chip.HandleRegisterWrite(SINE_FREQ, src.sourceFreq)
		chip.HandleRegisterWrite(SQUARE_FREQ, src.targetFreq)

		t.Logf("Phase 1: %s", src.phase1)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(500 * time.Millisecond)

		t.Logf("Phase 2: %s", src.phase2)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 1)
		time.Sleep(500 * time.Millisecond)

		chip.HandleRegisterWrite(SINE_CTRL, 1)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		time.Sleep(200 * time.Millisecond)
	}

	t.Log("\n[2] MULTI-TARGET MODULATION")
	t.Log("Exploring complex timbres through simultaneous modulation of multiple oscillators")

	multiModTests := []struct {
		freq1   uint32
		freq2   uint32
		freq3   uint32
		desc    string
		routing string
		texture string
	}{
		{440, 220, 330,
			"Three-Way Harmonic Modulation",
			"Square (440Hz) modulating both Triangle (220Hz) and Sine (330Hz)",
			"Creates dense harmonic spectrum with fundamental at 220Hz and complex upper partials"},
		{880, 440, 660,
			"Cascading Frequency Stack",
			"Octave-spaced carriers with shared modulation source",
			"Produces shimmering, choir-like effect with pronounced octave relationships"},
	}

	for _, test := range multiModTests {
		t.Logf("\n=== %s ===", test.desc)
		t.Logf("Routing Configuration: %s", test.routing)
		t.Logf("Sonic Character: %s", test.texture)

		chip.HandleRegisterWrite(SQUARE_FREQ, test.freq1)
		chip.HandleRegisterWrite(TRI_FREQ, test.freq2)
		chip.HandleRegisterWrite(SINE_FREQ, test.freq3)

		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(1000 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)

		// Reset mod routing between tests
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
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
	chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
	chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}

func TestModulation_DynamicRouting(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 180)
	chip.HandleRegisterWrite(TRI_VOL, 180)
	chip.HandleRegisterWrite(SINE_VOL, 180)

	t.Log("=== DYNAMIC MODULATION ROUTING TEST ===")
	t.Log("Testing real-time modulation route changes and their impact on timbre")

	t.Log("\n[1] SYNC SOURCE TRANSITIONS")
	t.Log("Evaluating oscillator behavior during hard sync source changes")

	syncTests := []struct {
		masterFreq uint32
		slaveFreq  uint32
		duration   int
		desc       string
		phase1     string
		phase2     string
	}{
		{220, 440, 2000, "Standard Octave Lock",
			"Square (220Hz) controlling Triangle (440Hz) - reinforced even harmonics",
			"Switching to Sine sync source - warmer, rounder harmonic content"},
		{440, 660, 2000, "Perfect Fifth Hard Sync",
			"Square (440Hz) forcing Triangle (660Hz) - creates bright metallic sidebands",
			"Sine sync introduces smoother harmonic transitions"},
		{440, 880, 2000, "Octave-Up Hard Lock",
			"Square fundamental controlling upper octave - strong harmonic alignment",
			"Sine sync softens harmonic edges while maintaining octave relationship"},
	}

	for _, sync := range syncTests {
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)

		t.Logf("\n=== %s ===", sync.desc)
		chip.HandleRegisterWrite(SQUARE_FREQ, sync.masterFreq)
		chip.HandleRegisterWrite(TRI_FREQ, sync.slaveFreq)

		t.Logf("Phase 1: %s", sync.phase1)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		time.Sleep(time.Duration(sync.duration) * time.Millisecond)

		t.Logf("Phase 2: %s", sync.phase2)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 2)
		time.Sleep(time.Duration(sync.duration) * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	t.Log("\n[2] COMPLEX RING MODULATION")
	t.Log("Testing dynamic carrier/modulator relationships with live routing changes")

	modTests := []struct {
		carrier uint32
		mod1    uint32
		mod2    uint32
		desc    string
		initial string
		evolved string
	}{
		{440, 220, 880, "Sub/Octave Modulation",
			"Initial modulation creates powerful sub-harmonic foundation",
			"Secondary modulator adds shimmering upper spectrum"},
		{440, 550, 660, "Harmonic Cluster",
			"Close intervallic modulation produces dense spectral mass",
			"Secondary routing adds further harmonic complexity"},
		{880, 440, 1760, "Wide Spectrum Modulation",
			"Octave-spread modulation generates rich harmonic series",
			"Additional routing creates bell-like resonances"},
	}

	for _, mod := range modTests {
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)

		t.Logf("\n=== %s ===", mod.desc)
		chip.HandleRegisterWrite(SINE_FREQ, mod.carrier)
		chip.HandleRegisterWrite(SQUARE_FREQ, mod.mod1)
		chip.HandleRegisterWrite(TRI_FREQ, mod.mod2)

		t.Logf("Initial: %s", mod.initial)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		time.Sleep(1500 * time.Millisecond)

		t.Logf("Evolution: %s", mod.evolved)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 1)
		time.Sleep(1500 * time.Millisecond)

		chip.HandleRegisterWrite(SINE_CTRL, 1)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	t.Log("\n[3] HYBRID MODULATION")
	t.Log("Testing combined sync and ring modulation with dynamic route changes")

	hybridTests := []struct {
		freq1  uint32
		freq2  uint32
		freq3  uint32
		desc   string
		phase1 string
		phase2 string
	}{
		{440, 880, 220, "Sync-Mod Cascade",
			"Initial routing creates foundation of locked oscillators",
			"Cross-modulation adds complexity while maintaining sync relationship"},
		{220, 440, 660, "Harmonic Stack",
			"Fundamentally locked oscillators with progressive harmonic enhancement",
			"Route change creates evolving spectral animation"},
		{880, 440, 1760, "Cross-Modulation",
			"High-frequency master with sub-octave modulation influence",
			"Routing shift produces complex interaction between all oscillators"},
	}

	for _, test := range hybridTests {
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)

		t.Logf("\n=== %s ===", test.desc)
		chip.HandleRegisterWrite(SQUARE_FREQ, test.freq1)
		chip.HandleRegisterWrite(TRI_FREQ, test.freq2)
		chip.HandleRegisterWrite(SINE_FREQ, test.freq3)

		t.Logf("Phase 1: %s", test.phase1)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 1)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		time.Sleep(2000 * time.Millisecond)

		t.Logf("Phase 2: %s", test.phase2)
		chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 1)
		chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 2)
		time.Sleep(2000 * time.Millisecond)

		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
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
	chip.HandleRegisterWrite(SYNC_SOURCE_CH1, 0)
	chip.HandleRegisterWrite(SYNC_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(RING_MOD_SOURCE_CH1, 0)
	chip.HandleRegisterWrite(RING_MOD_SOURCE_CH2, 0)
	chip.HandleRegisterWrite(AUDIO_CTRL, 0)
}
