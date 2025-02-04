// audio_basic_waveforms_test.go - Basic functionality tests for each channel type

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

func TestSquareWave_BasicWaveforms(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 255)
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_REL, 1) // 1ms release (1 * 44100/1000 = 44 samples)

	t.Log("=== SQUARE WAVE SYNTHESIS DEMONSTRATION ===")
	t.Log("Demonstrating the extensive sound synthesis capabilities of the square wave channel")

	// 1. Frequency Range and Musical Accuracy
	t.Log("\n[1] FREQUENCY ACCURACY AND MUSICAL RANGE")
	t.Log("Demonstrating precise pitch control across 7 octaves")

	musicalDemos := []struct {
		freq uint32
		note string
		desc string
	}{
		{65, "C2", "sub-bass fundamental"},
		{131, "C3", "bass register"},
		{262, "C4", "tenor register"},
		{523, "C5", "alto register"},
		{1047, "C6", "soprano register"},
		{2093, "C7", "brilliance register"},
	}

	// Play ascending scale
	for _, note := range musicalDemos {
		chip.HandleRegisterWrite(SQUARE_FREQ, note.freq)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		t.Logf("%s (%dHz) - Should hear %s", note.note, note.freq, note.desc)
		time.Sleep(800 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(100 * time.Millisecond)
	}

	// 2. Dynamic Range and Volume Control
	t.Log("\n[2] DYNAMIC RANGE AND AMPLITUDE PRECISION")
	t.Log("Demonstrating 8-bit volume resolution with perfect scaling")

	chip.HandleRegisterWrite(SQUARE_FREQ, 440) // A4 reference pitch
	dynamics := []struct {
		vol     uint32
		marking string
		desc    string
	}{
		{255, "fortissimo", "full volume with aggressive edge"},
		{192, "forte", "strong but controlled"},
		{128, "mezzo-forte", "moderate with clarity"},
		{96, "mezzo-piano", "gentle but present"},
		{64, "piano", "soft with detail"},
		{32, "pianissimo", "very soft but clear"},
		{16, "pianississimo", "extremely soft yet defined"},
	}

	// Demonstrate dynamic control
	for _, dyn := range dynamics {
		chip.HandleRegisterWrite(SQUARE_VOL, dyn.vol)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		t.Logf("%s (%d/255) - Should hear %s", dyn.marking, dyn.vol, dyn.desc)
		time.Sleep(1200 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// Demonstrate volume ramp
	t.Log("Demonstrating volume ramp")
	chip.HandleRegisterWrite(SQUARE_FREQ, 440)
	chip.HandleRegisterWrite(SQUARE_CTRL, 3)

	// Test specific volume points instead of ramping
	volumes := []uint32{32, 64, 96, 128, 160, 192, 224, 255, 224, 192, 160, 128, 96, 64, 32}
	for _, vol := range volumes {
		chip.HandleRegisterWrite(SQUARE_VOL, vol)
		time.Sleep(100 * time.Millisecond)
	}
	chip.HandleRegisterWrite(SQUARE_CTRL, 1)
	time.Sleep(500 * time.Millisecond)

	t.Log("\n[3] SQUARE WAVE CHARACTERISTICS")
	t.Log("Demonstrating fundamental square wave tones and amplitude control")

	// Test different duty cycles with volume variation
	configs := []struct {
		duty     uint32
		vol      uint32
		baseFreq uint32
		desc     string
	}{
		{32, 255, 440, "narrow pulse wave at full volume"},
		{64, 192, 440, "quarter duty cycle at 75% volume"},
		{128, 128, 440, "standard square wave at mid volume"},
		{178, 64, 440, "wide pulse wave at low volume"},
	}

	chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0)

	for _, cfg := range configs {
		chip.HandleRegisterWrite(SQUARE_DUTY, cfg.duty)
		chip.HandleRegisterWrite(SQUARE_VOL, cfg.vol)
		chip.HandleRegisterWrite(SQUARE_FREQ, cfg.baseFreq)

		t.Logf("Testing %s", cfg.desc)

		// Play a sequence of notes with this timbre
		for _, note := range []uint32{440, 554, 659} {
			chip.HandleRegisterWrite(SQUARE_FREQ, note)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			time.Sleep(400 * time.Millisecond)
			chip.HandleRegisterWrite(SQUARE_CTRL, 1)
			time.Sleep(100 * time.Millisecond)
		}

		time.Sleep(300 * time.Millisecond)
	}

	// 4. PWM Effects and Dynamic Timbre
	t.Log("\n[4] PULSE WIDTH MODULATION SYNTHESIS")
	t.Log("Demonstrating dynamic timbral evolution through PWM")

	pwmEffects := []struct {
		rate      uint32
		depth     uint32
		baseduty  uint32
		desc      string
		character string
	}{
		{0x20, 32, 128, "subtle chorus", "gentle beating effect"},
		{0x30, 64, 128, "light vibrato", "warm movement"},
		{0x40, 96, 128, "classic chorus", "rich ensemble-like"},
		{0x50, 128, 128, "deep phasing", "sweeping harmonics"},
		{0x60, 160, 128, "dramatic chorus", "intense modulation"},
		{0x70, 192, 128, "extreme modulation", "synthetic texture"},
		// Special effects
		{0x35, 255, 64, "trance lead", "pulsing energy"},
		{0x45, 255, 32, "cosmic pulse", "sci-fi modulation"},
	}

	for _, effect := range pwmEffects {
		chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|effect.rate)
		chip.HandleRegisterWrite(SQUARE_DUTY, (effect.depth<<8)|effect.baseduty)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		t.Logf("%s - Should hear %s", effect.desc, effect.character)
		time.Sleep(2500 * time.Millisecond)
	}

	// 5. Musical Application
	t.Log("\n[5] MUSICAL SYNTHESIS DEMONSTRATION")
	t.Log("Demonstrating practical application in musical context")

	// Configure for lead sound
	chip.HandleRegisterWrite(SQUARE_DUTY, (64<<8)|64)    // 25% - bright lead
	chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x30) // Light chorus
	chip.HandleRegisterWrite(SQUARE_VOL, 200)

	// Play melodic phrase
	melodicPhrase := []struct {
		freq uint32
		dur  int
	}{
		{440, 300}, // A4
		{554, 300}, // C#5
		{659, 600}, // E5
		{554, 300}, // C#5
		{440, 300}, // A4
		{554, 300}, // C#5
		{659, 300}, // E5
		{880, 600}, // A5
	}

	for _, note := range melodicPhrase {
		chip.HandleRegisterWrite(SQUARE_FREQ, note.freq)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(time.Duration(note.dur) * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(50 * time.Millisecond)
	}

	// Final cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_DUTY, 128)
	chip.HandleRegisterWrite(SQUARE_VOL, 255)
}

func TestSquareWave_TimbreModulation(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 200)
	chip.HandleRegisterWrite(SQUARE_CTRL, 0)

	t.Log("=== SQUARE WAVE TIMBRE MODULATION TEST ===")

	// 1. Duty Cycle Sweep
	t.Log("\n[1] DUTY CYCLE TIMBRAL EVOLUTION")
	duties := []struct {
		duty uint32
		desc string
	}{
		{32, "thin needle pulse"},
		{64, "buzzy reed tone"},
		{96, "clarinet-like"},
		{128, "full square"},
		{160, "hollow wind"},
		{192, "nasal tone"},
		{224, "wraith whistle"},
	}

	for _, d := range duties {
		chip.HandleRegisterWrite(SQUARE_FREQ, 440)
		chip.HandleRegisterWrite(SQUARE_DUTY, d.duty)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		t.Logf("Duty %d/256 - Should hear %s", d.duty, d.desc)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. PWM Evolution
	t.Log("\n[2] PWM DYNAMIC SPECTRUM")
	pwmTests := []struct {
		rate  uint32
		depth uint32
		desc  string
	}{
		{0x20, 64, "gentle chorus"},
		{0x30, 128, "vintage ensemble"},
		{0x40, 192, "rich detuning"},
		{0x50, 255, "extreme modulation"},
	}

	for _, pwm := range pwmTests {
		chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|pwm.rate)
		chip.HandleRegisterWrite(SQUARE_DUTY, (pwm.depth<<8)|128)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		t.Logf("Rate 0x%X, Depth %d - Should hear %s", pwm.rate, pwm.depth, pwm.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 3. Musical Application
	t.Log("\n[3] TIMBRE ANIMATION")
	chip.HandleRegisterWrite(SQUARE_FREQ, 440)
	chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|0x40)
	chip.HandleRegisterWrite(SQUARE_DUTY, 128<<8|128)

	t.Log("Demonstrating evolving synthesizer lead")
	melodyNotes := []uint32{440, 554, 659, 880}
	for _, note := range melodyNotes {
		chip.HandleRegisterWrite(SQUARE_FREQ, note)
		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(500 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(100 * time.Millisecond)
	}

	// Cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0)
}
func TestSquareWave_PWMDepthModulation(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_VOL, 200)
	chip.HandleRegisterWrite(SQUARE_FREQ, 440)

	t.Log("=== ADVANCED SQUARE WAVE PWM SYNTHESIS DEMONSTRATION ===")
	t.Log("Demonstrating the expressive capabilities of dynamic PWM modulation")

	// 1. Dynamic PWM Depth Changes
	t.Log("\n[1] DYNAMIC PWM DEPTH EVOLUTION")
	t.Log("Exploring the timbral spectrum through continuous PWM depth changes")
	depthTests := []struct {
		rate     uint32
		baseduty uint32
		depths   []uint32
		duration int
		desc     string
		effect   string
	}{
		{
			0x30, // Moderate LFO rate
			128,  // 50% duty cycle
			[]uint32{32, 64, 128, 192, 255},
			800,
			"progressive thickening",
			"evolving from subtle movement to rich ensemble-like chorus",
		},
		{
			0x50, // Faster LFO
			64,   // 25% duty cycle
			[]uint32{128, 192, 255},
			1000,
			"rapid thin-to-thick transformation",
			"bright reed-like tone developing into lush detuned texture",
		},
		{
			0x20, // Slow LFO
			192,  // 75% duty cycle
			[]uint32{64, 32, 16},
			1200,
			"gradual thinning texture",
			"wide hollow tone collapsing to piercing brilliance",
		},
	}

	for _, test := range depthTests {
		chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|test.rate) // Enable PWM

		t.Logf("\n=== %s ===", strings.ToUpper(test.desc))
		t.Logf("Base characteristics: Rate: 0x%X, Duty: %d/255", test.rate, test.baseduty)
		t.Log("Sonic character:", test.effect)

		for _, depth := range test.depths {
			chip.HandleRegisterWrite(SQUARE_DUTY, (depth<<8)|test.baseduty)
			chip.HandleRegisterWrite(SQUARE_CTRL, 3)
			t.Logf("PWM Depth: %d/255 - Should hear %s at %.1f%% duty cycle",
				depth, getDepthDescription(depth), float32(test.baseduty)/255.0*100)
			time.Sleep(time.Duration(test.duration) * time.Millisecond)
		}
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(300 * time.Millisecond)
	}

	// 2. PWM with Envelope Interaction
	t.Log("\n[2] PWM-ENVELOPE INTERACTION")
	t.Log("Demonstrating complex timbre evolution through combined PWM and envelope modulation")

	envTests := []struct {
		pwmRate   uint32
		pwmDepth  uint32
		duty      uint32
		attack    uint32
		decay     uint32
		sustain   uint32
		release   uint32
		desc      string
		character string
	}{
		{
			0x40, 192, 128,
			200, 300, 180, 400,
			"slow attack with deep PWM",
			"gradual emergence of rich, swirling harmonics with thick chorus",
		},
		{
			0x60, 128, 64,
			50, 200, 200, 300,
			"quick attack with moderate PWM",
			"sharp initial transient evolving into stable ensemble texture",
		},
		{
			0x30, 255, 192,
			400, 100, 220, 500,
			"very slow attack maximum PWM",
			"massive unfolding soundscape with extreme stereo width",
		},
	}

	for _, test := range envTests {
		chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|test.pwmRate)
		chip.HandleRegisterWrite(SQUARE_DUTY, (test.pwmDepth<<8)|test.duty)
		chip.HandleRegisterWrite(SQUARE_ATK, test.attack)
		chip.HandleRegisterWrite(SQUARE_DEC, test.decay)
		chip.HandleRegisterWrite(SQUARE_SUS, test.sustain)
		chip.HandleRegisterWrite(SQUARE_REL, test.release)

		t.Logf("\n=== %s ===", strings.ToUpper(test.desc))
		t.Logf("PWM Rate: 0x%X, Depth: %d/255, Duty: %d/255", test.pwmRate, test.pwmDepth, test.duty)
		t.Logf("Envelope: Attack %dms, Decay %dms, Sustain %d/255, Release %dms",
			test.attack, test.decay, test.sustain, test.release)
		t.Logf("Should hear: %s", test.character)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(3000 * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(time.Duration(test.release) * time.Millisecond)
	}

	// 3. Musical Application with Dynamic PWM
	t.Log("\n[3] MUSICAL PWM EXPRESSION")
	t.Log("Exploring PWM as an expressive parameter in melodic context")

	chip.HandleRegisterWrite(SQUARE_ATK, 100)
	chip.HandleRegisterWrite(SQUARE_DEC, 200)
	chip.HandleRegisterWrite(SQUARE_SUS, 180)
	chip.HandleRegisterWrite(SQUARE_REL, 300)

	musicalPhrases := []struct {
		freq     uint32
		pwmRate  uint32
		depth    uint32
		duty     uint32
		duration int
		desc     string
		timbre   string
	}{
		{440, 0x30, 128, 128, 800, "stable center modulation",
			"balanced chorus effect with clear fundamental"},
		{554, 0x50, 192, 64, 800, "bright rising tone",
			"thin, aggressive timbre with pronounced upper harmonics"},
		{659, 0x40, 255, 32, 1200, "thin high note",
			"extreme modulation creating shimmering upper register"},
		{880, 0x20, 160, 192, 1000, "thick peak note",
			"rich, full-bodied tone with gentle undulation"},
		{659, 0x30, 128, 128, 800, "return to center",
			"settling back to stable chorus texture"},
	}

	for _, phrase := range musicalPhrases {
		chip.HandleRegisterWrite(SQUARE_FREQ, phrase.freq)
		chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|phrase.pwmRate)
		chip.HandleRegisterWrite(SQUARE_DUTY, (phrase.depth<<8)|phrase.duty)

		t.Logf("\n=== %s ===", strings.ToUpper(phrase.desc))
		t.Logf("Note: %dHz with PWM Rate: 0x%X", phrase.freq, phrase.pwmRate)
		t.Logf("PWM Depth: %d/255, Duty: %d/255", phrase.depth, phrase.duty)
		t.Logf("Should hear: %s", phrase.timbre)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(time.Duration(phrase.duration) * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 4. Extreme PWM Settings
	t.Log("\n[4] EXTREME PWM EXPLORATION")
	t.Log("Testing the boundaries of PWM synthesis capabilities")

	extremeTests := []struct {
		rate     uint32
		depth    uint32
		duty     uint32
		duration int
		desc     string
		effect   string
	}{
		{0x7F, 255, 16, 2000, "ultra-fast complete modulation",
			"creating complex sidebands and harmonic sprays"},
		{0x01, 255, 240, 3000, "super-slow wide sweep",
			"glacial transformation from thick to thin timbre"},
		{0x40, 255, 8, 2000, "maximum depth minimum width",
			"extreme timbral evolution from needle-thin pulse"},
		{0x40, 255, 248, 2000, "maximum depth maximum width",
			"massive modulation of very wide pulse wave"},
	}

	for _, ex := range extremeTests {
		chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0x80|ex.rate)
		chip.HandleRegisterWrite(SQUARE_DUTY, (ex.depth<<8)|ex.duty)

		t.Logf("\n=== %s ===", strings.ToUpper(ex.desc))
		t.Logf("PWM Rate: 0x%X, Depth: %d/255, Duty: %d/255", ex.rate, ex.depth, ex.duty)
		t.Logf("Should hear: %s", ex.effect)

		chip.HandleRegisterWrite(SQUARE_CTRL, 3)
		time.Sleep(time.Duration(ex.duration) * time.Millisecond)
		chip.HandleRegisterWrite(SQUARE_CTRL, 1)
		time.Sleep(300 * time.Millisecond)
	}

	// Cleanup
	chip.HandleRegisterWrite(SQUARE_CTRL, 1)
	chip.HandleRegisterWrite(SQUARE_PWM_CTRL, 0)
	chip.HandleRegisterWrite(SQUARE_DUTY, 128)
}

func TestTriangleWave_BasicWaveforms(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(TRI_VOL, 255)
	chip.HandleRegisterWrite(TRI_CTRL, 0)

	t.Log("=== TRIANGLE WAVE SYNTHESIS DEMONSTRATION ===")
	t.Log("Demonstrating the pristine harmonics and flute-like qualities of the triangle oscillator")

	// 1. Musical Range and Accuracy
	t.Log("\n[1] FREQUENCY ACCURACY AND MUSICAL PURITY")
	musicalTests := []struct {
		freq uint32
		note string
		desc string
	}{
		{55, "A1", "sub-bass with clear fundamental"},
		{110, "A2", "warm bass register"},
		{220, "A3", "flute-like mid register"},
		{440, "A4", "pure reference pitch"},
		{880, "A5", "bell-like upper register"},
		{1760, "A6", "crystalline high register"},
		{3520, "A7", "pristine ultra-high harmonics"},
	}

	for _, note := range musicalTests {
		chip.HandleRegisterWrite(TRI_FREQ, note.freq)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("%s (%dHz) - Should hear %s", note.note, note.freq, note.desc)
		time.Sleep(1000 * time.Millisecond)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Dynamic Range Control
	t.Log("\n[2] DYNAMIC RANGE AND AMPLITUDE LINEARITY")
	t.Log("Demonstrating perfect volume scaling with no waveshaping")

	dynamics := []struct {
		vol  uint32
		desc string
	}{
		{255, "fortissimo - pure and strong"},
		{192, "forte - clear projection"},
		{128, "mezzo-forte - balanced presence"},
		{96, "mezzo-piano - gentle clarity"},
		{64, "piano - soft transparency"},
		{32, "pianissimo - delicate detail"},
	}

	chip.HandleRegisterWrite(TRI_FREQ, 440)
	for _, dyn := range dynamics {
		chip.HandleRegisterWrite(TRI_VOL, dyn.vol)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Volume %d/255 - Should hear %s", dyn.vol, dyn.desc)
		time.Sleep(1500 * time.Millisecond)
	}

	// 3. Harmonic Series Demonstration
	t.Log("\n[3] HARMONIC SERIES CLARITY")
	t.Log("Demonstrating naturally attenuated odd harmonics")

	harmonics := []struct {
		freq     uint32
		harmonic int
		desc     string
	}{
		{440, 1, "pure fundamental"},
		{1320, 3, "sweet third harmonic"},
		{2200, 5, "airy fifth harmonic"},
		{3080, 7, "ethereal seventh harmonic"},
	}

	chip.HandleRegisterWrite(TRI_VOL, 200)
	for _, harm := range harmonics {
		chip.HandleRegisterWrite(TRI_FREQ, harm.freq)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Harmonic %d (%dHz) - Should hear %s", harm.harmonic, harm.freq, harm.desc)
		time.Sleep(1500 * time.Millisecond)
	}

	// 4. Musical Application
	t.Log("\n[4] MUSICAL APPLICATIONS")
	t.Log("Demonstrating flute-like melodic capabilities")

	melody := []struct {
		freq uint32
		dur  int
		note string
	}{
		{440, 400, "A4"}, // A
		{494, 400, "B4"}, // B
		{523, 800, "C5"}, // C
		{587, 400, "D5"}, // D
		{659, 400, "E5"}, // E
		{698, 800, "F5"}, // F
		{784, 400, "G5"}, // G
		{880, 800, "A5"}, // A
	}

	chip.HandleRegisterWrite(TRI_VOL, 200)
	for _, note := range melody {
		chip.HandleRegisterWrite(TRI_FREQ, note.freq)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Playing %s (%dHz) - Pure triangle tone", note.note, note.freq)
		time.Sleep(time.Duration(note.dur) * time.Millisecond)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(50 * time.Millisecond)
	}

	// 5. Filter Interaction Demo
	t.Log("\n[5] FILTER INTERACTION CHARACTERISTICS")
	t.Log("Demonstrating unique triangle wave filter response")

	filterTests := []struct {
		ftype  uint32
		cutoff uint32
		res    uint32
		desc   string
	}{
		{1, 64, 200, "warm low-pass resonance"},
		{2, 192, 200, "airy high-pass shimmer"},
		{3, 128, 220, "focused mid-range peak"},
	}

	// Set up filter routing
	chip.HandleRegisterWrite(TRI_FREQ, 440)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 1)   // Route triangle to filter
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 255) // Full modulation depth

	for _, filt := range filterTests {
		chip.HandleRegisterWrite(FILTER_TYPE, filt.ftype)
		chip.HandleRegisterWrite(FILTER_CUTOFF, filt.cutoff)
		chip.HandleRegisterWrite(FILTER_RESONANCE, filt.res)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Filter setting - Should hear %s", filt.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(300 * time.Millisecond)
	}

	// Clean up
	chip.HandleRegisterWrite(TRI_CTRL, 1)
	chip.HandleRegisterWrite(FILTER_TYPE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
}
func TestTriangleWave_TimbralPurity(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(TRI_VOL, 255)
	chip.HandleRegisterWrite(TRI_CTRL, 0)

	t.Log("=== TRIANGLE WAVE TIMBRAL PURITY TEST ===")

	// 1. Harmonic Series Test
	t.Log("\n[1] HARMONIC CONSISTENCY")
	harmonics := []struct {
		freq uint32
		note string
		desc string
	}{
		{55, "A1", "pure fundamental with odd harmonics"},
		{110, "A2", "warm bass with 1/n² rolloff"},
		{220, "A3", "clear mid with pure triangle spectrum"},
		{440, "A4", "reference A with pristine harmonics"},
		{880, "A5", "upper register maintaining purity"},
		{1760, "A6", "high harmonics with no aliasing"},
		{3520, "A7", "ultra-high stability test"},
	}

	for _, h := range harmonics {
		chip.HandleRegisterWrite(TRI_FREQ, h.freq)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("%s (%dHz) - Should hear %s", h.note, h.freq, h.desc)
		time.Sleep(1200 * time.Millisecond)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Phase Relationship Demo
	t.Log("\n[2] PHASE COHERENCE AND WAVEFORM STABILITY")
	phaseTests := []struct {
		freq uint32
		desc string
	}{
		{440, "perfect symmetry at A4"},
		{880, "maintained shape at A5"},
		{1760, "phase accuracy at A6"},
	}

	for _, p := range phaseTests {
		chip.HandleRegisterWrite(TRI_FREQ, p.freq)
		// Quick on/off to test phase reset
		for i := 0; i < 4; i++ {
			chip.HandleRegisterWrite(TRI_CTRL, 3)
			t.Logf("%dHz - Should hear %s", p.freq, p.desc)
			time.Sleep(300 * time.Millisecond)
			chip.HandleRegisterWrite(TRI_CTRL, 1)
			time.Sleep(200 * time.Millisecond)
		}
	}

	// 3. Spectral Purity with Amplitude Variation
	t.Log("\n[3] AMPLITUDE-INDEPENDENT SPECTRAL PURITY")
	chip.HandleRegisterWrite(TRI_FREQ, 440)
	volumes := []struct {
		vol  uint32
		desc string
	}{
		{255, "full volume with perfect triangle shape"},
		{192, "75% level maintaining spectrum"},
		{128, "50% level with harmonic preservation"},
		{64, "25% level showing clean scaling"},
	}

	for _, v := range volumes {
		chip.HandleRegisterWrite(TRI_VOL, v.vol)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Volume %d/255 - Should hear %s", v.vol, v.desc)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 4. Filter Response Character
	t.Log("\n[4] FILTER INTERACTION TEST")
	filterTests := []struct {
		ftype  uint32
		cutoff uint32
		res    uint32
		desc   string
	}{
		{1, 64, 200, "low-pass warmth with pure triangle rolloff"},
		{2, 192, 200, "high-pass clarity preserving odd harmonics"},
		{3, 128, 220, "band-pass resonance showing harmonic focus"},
	}

	chip.HandleRegisterWrite(TRI_VOL, 200)
	chip.HandleRegisterWrite(TRI_FREQ, 440)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 1)   // Use triangle wave
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 255) // Full depth
	for _, f := range filterTests {
		chip.HandleRegisterWrite(FILTER_TYPE, f.ftype)
		chip.HandleRegisterWrite(FILTER_CUTOFF, f.cutoff)
		chip.HandleRegisterWrite(FILTER_RESONANCE, f.res)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Filter type %d - Should hear %s", f.ftype, f.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)

	// Cleanup
	chip.HandleRegisterWrite(TRI_CTRL, 0)
	chip.HandleRegisterWrite(TRI_VOL, 0)
	chip.HandleRegisterWrite(TRI_FREQ, 0)
	chip.HandleRegisterWrite(FILTER_TYPE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_SOURCE, 0)
	chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 0)
}

func TestSineWave_BasicWaveforms(t *testing.T) {
	// Initial setup
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(SINE_VOL, 255)
	chip.HandleRegisterWrite(SINE_CTRL, 0)

	t.Log("=== SINE WAVE SYNTHESIS DEMONSTRATION ===")
	t.Log("Demonstrating perfect sine wave generation with zero harmonic distortion")

	// 1. Musical Range and Pitch Accuracy
	t.Log("\n[1] FREQUENCY ACCURACY AND PITCH PURITY")
	musicalTests := []struct {
		freq uint32
		note string
		desc string
	}{
		{28, "A0", "subsonic fundamental"},
		{55, "A1", "deep sub-bass"},
		{110, "A2", "pure bass"},
		{220, "A3", "mid-bass reference"},
		{440, "A4", "concert pitch reference"},
		{880, "A5", "upper reference"},
		{1760, "A6", "high harmonic test"},
		{3520, "A7", "ultra-high stability test"},
	}

	for _, note := range musicalTests {
		chip.HandleRegisterWrite(SINE_FREQ, note.freq)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("%s (%dHz) - Should hear %s with zero harmonic content",
			note.note, note.freq, note.desc)
		time.Sleep(1000 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Volume Linearity
	t.Log("\n[2] AMPLITUDE LINEARITY AND DISTORTION TEST")
	chip.HandleRegisterWrite(SINE_FREQ, 440)

	amplitudes := []struct {
		vol  uint32
		desc string
	}{
		{255, "maximum amplitude - verify zero clipping"},
		{192, "75% - perfect sine scaling"},
		{128, "50% - mathematically precise halfpoint"},
		{64, "25% - preserving waveform at low level"},
		{32, "12.5% - testing noise floor separation"},
		{16, "6.25% - minimum clean reproduction"},
	}

	for _, amp := range amplitudes {
		chip.HandleRegisterWrite(SINE_VOL, amp.vol)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("Volume %d/255 - Should hear %s", amp.vol, amp.desc)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}
	chip.HandleRegisterWrite(SINE_VOL, 255) // Reset volume

	// 3. Beat Frequency Test
	t.Log("\n[3] PRECISION BEAT FREQUENCY DEMONSTRATION")
	t.Log("Demonstrating accurate frequency relationships")

	beats := []struct {
		freq1 uint32
		freq2 uint32
		beat  uint32
		desc  string
	}{
		{440, 441, 1, "1Hz beat frequency"},
		{440, 445, 5, "5Hz beat frequency"},
		{440, 450, 10, "10Hz beat frequency"},
	}

	// Use two sine channels for beat frequencies
	chip.HandleRegisterWrite(SINE_VOL, 128) // Channel 2 (main sine)
	chip.HandleRegisterWrite(TRI_VOL, 128)  // Channel 1 (triangle as second oscillator)

	for _, beat := range beats {
		chip.HandleRegisterWrite(SINE_FREQ, beat.freq1)
		chip.HandleRegisterWrite(TRI_FREQ, beat.freq2)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		chip.HandleRegisterWrite(TRI_CTRL, 3)
		t.Logf("Beat test: %dHz vs %dHz - Should hear %dHz beat pattern - %s",
			beat.freq1, beat.freq2, beat.beat, beat.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		chip.HandleRegisterWrite(TRI_CTRL, 1)
		time.Sleep(500 * time.Millisecond)
	}

	// Reset secondary oscillator
	chip.HandleRegisterWrite(TRI_CTRL, 0)
	chip.HandleRegisterWrite(TRI_VOL, 0)

	// 4. Phase Coherence Demo
	t.Log("\n[4] PHASE STABILITY DEMONSTRATION")
	chip.HandleRegisterWrite(SINE_FREQ, 440)
	chip.HandleRegisterWrite(SINE_VOL, 200)

	for i := 0; i < 5; i++ {
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Log("Gate ON - Should hear instant, click-free start")
		time.Sleep(500 * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		t.Log("Gate OFF - Should hear instant, click-free stop")
		time.Sleep(500 * time.Millisecond)
	}

	// 5. Musical Application
	t.Log("\n[5] MUSICAL SYNTHESIS APPLICATION")
	t.Log("Demonstrating pure sine wave melody")

	melody := []struct {
		freq uint32
		dur  int
		note string
	}{
		{440, 400, "A4"}, // A
		{494, 400, "B4"}, // B (corrected from 495)
		{523, 800, "C5"}, // C
		{587, 400, "D5"}, // D
		{659, 400, "E5"}, // E
		{698, 800, "F5"}, // F
		{784, 400, "G5"}, // G
		{880, 800, "A5"}, // A
	}

	chip.HandleRegisterWrite(SINE_VOL, 200)
	for _, note := range melody {
		chip.HandleRegisterWrite(SINE_FREQ, note.freq)
		chip.HandleRegisterWrite(SINE_CTRL, 3)
		t.Logf("Playing %s (%dHz) - Pure sine tone", note.note, note.freq)
		time.Sleep(time.Duration(note.dur) * time.Millisecond)
		chip.HandleRegisterWrite(SINE_CTRL, 1)
		time.Sleep(50 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(SINE_CTRL, 0)
	chip.HandleRegisterWrite(SINE_VOL, 0)
	chip.HandleRegisterWrite(SINE_FREQ, 0)
}

func TestNoise_TypeVariations(t *testing.T) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(NOISE_VOL, 200)
	chip.HandleRegisterWrite(NOISE_CTRL, 0)

	t.Log("=== NOISE GENERATOR SYNTHESIS DEMONSTRATION ===")
	t.Log("Demonstrating advanced noise synthesis capabilities")

	// 1. Basic Noise Types
	t.Log("\n[1] NOISE TYPE CHARACTERISTICS")
	noiseTypes := []struct {
		mode      uint32
		freq      uint32
		desc      string
		character string
	}{
		{NOISE_MODE_WHITE, 44100, "pure white noise", "flat frequency spectrum, maximum entropy"},
		{NOISE_MODE_PERIODIC, 44100, "pitched noise", "cyclic pattern with tonal center"},
		{NOISE_MODE_METALLIC, 44100, "metallic noise", "resonant interference patterns"},
	}

	for _, noise := range noiseTypes {
		chip.HandleRegisterWrite(NOISE_MODE, noise.mode)
		chip.HandleRegisterWrite(NOISE_FREQ, noise.freq)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		t.Logf("%s - Should hear %s", noise.desc, noise.character)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(500 * time.Millisecond)
	}
	chip.HandleRegisterWrite(NOISE_CTRL, 0)

	// 2. Frequency-Color Relationships
	t.Log("\n[2] SPECTRAL COLORATION")
	colorTests := []struct {
		freq uint32
		desc string
	}{
		{11025, "deep rumble with sub-bass emphasis"},
		{22050, "full-spectrum noise"},
		{33075, "bright hiss with treble emphasis"},
		{44100, "ultra-wide bandwidth noise"},
	}

	chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_WHITE)
	for _, color := range colorTests {
		chip.HandleRegisterWrite(NOISE_FREQ, color.freq)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		t.Logf("%dHz - Should hear %s", color.freq, color.desc)
		time.Sleep(1500 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(200 * time.Millisecond)
	}
	chip.HandleRegisterWrite(NOISE_CTRL, 0)

	// 3. Musical Effects
	t.Log("\n[3] MUSICAL NOISE APPLICATIONS")
	effects := []struct {
		mode uint32
		freq uint32
		vol  uint32
		desc string
	}{
		{NOISE_MODE_WHITE, 44100, 128, "crash cymbal simulation"},
		{NOISE_MODE_PERIODIC, 880, 180, "steam locomotive effect"},
		{NOISE_MODE_METALLIC, 440, 200, "industrial machinery"},
		{NOISE_MODE_WHITE, 22050, 160, "ocean waves"},
	}

	for _, fx := range effects {
		chip.HandleRegisterWrite(NOISE_MODE, fx.mode)
		chip.HandleRegisterWrite(NOISE_FREQ, fx.freq)
		chip.HandleRegisterWrite(NOISE_VOL, fx.vol)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		t.Logf("Effect: %s", fx.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(500 * time.Millisecond)
	}
	chip.HandleRegisterWrite(NOISE_CTRL, 0)

	// 4. Envelope Interaction
	t.Log("\n[4] ENVELOPE SHAPING")
	chip.HandleRegisterWrite(NOISE_MODE, NOISE_MODE_WHITE)
	chip.HandleRegisterWrite(NOISE_FREQ, 44100)
	chip.HandleRegisterWrite(NOISE_VOL, 200)

	envelopes := []struct {
		atk  uint32
		dec  uint32
		sus  uint32
		rel  uint32
		desc string
	}{
		{10, 100, 0, 200, "percussive hit"},
		{200, 100, 180, 300, "slow attack pad"},
		{50, 200, 100, 400, "atmospheric fade"},
	}

	for _, env := range envelopes {
		chip.HandleRegisterWrite(NOISE_ATK, env.atk)
		chip.HandleRegisterWrite(NOISE_DEC, env.dec)
		chip.HandleRegisterWrite(NOISE_SUS, env.sus)
		chip.HandleRegisterWrite(NOISE_REL, env.rel)
		chip.HandleRegisterWrite(NOISE_CTRL, 3)
		t.Logf("Envelope: %s", env.desc)
		time.Sleep(2000 * time.Millisecond)
		chip.HandleRegisterWrite(NOISE_CTRL, 1)
		time.Sleep(500 * time.Millisecond)
	}

	// Complete cleanup
	chip.HandleRegisterWrite(NOISE_CTRL, 0)
	chip.HandleRegisterWrite(NOISE_MODE, 0)
	chip.HandleRegisterWrite(NOISE_FREQ, 0)
	chip.HandleRegisterWrite(NOISE_VOL, 0)
	chip.HandleRegisterWrite(NOISE_ATK, 0)
	chip.HandleRegisterWrite(NOISE_DEC, 0)
	chip.HandleRegisterWrite(NOISE_SUS, 0)
	chip.HandleRegisterWrite(NOISE_REL, 0)
}

// Helper function for PWM depth description
func getDepthDescription(depth uint32) string {
	switch {
	case depth < 64:
		return "subtle chorus movement"
	case depth < 128:
		return "moderate ensemble effect"
	case depth < 192:
		return "pronounced detuning"
	default:
		return "extreme modulation width"
	}
}
