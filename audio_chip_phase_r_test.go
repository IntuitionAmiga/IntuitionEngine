package main

import (
	"math"
	"testing"
)

func TestPhaseR_DACUsesSymmetricSignedScale(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.applyFlexRegister(0, FLEX_OFF_CTRL, 3)
	chip.applyFlexRegister(0, FLEX_OFF_VOL, 255)

	chip.applyFlexRegister(0, FLEX_OFF_DAC, 0x7F)
	if got := chip.channels[0].dacValue; math.Abs(float64(got-1.0)) > 0.0001 {
		t.Fatalf("positive full-scale dacValue=%f, want 1.0", got)
	}
	chip.applyFlexRegister(0, FLEX_OFF_DAC, 0x80)
	if got := chip.channels[0].dacValue; math.Abs(float64(got+1.0)) > 0.0001 {
		t.Fatalf("negative full-scale dacValue=%f, want -1.0", got)
	}
}

func TestPhaseR_SIDQuantize12BitTruncates(t *testing.T) {
	const sample = float32(0.0001)
	normalized := (sample + 1.0) * 0.5 * float32(SID_OSC_MAX)
	want := float32(int(normalized))*SID_OSC_TO_FLOAT - 1.0
	if got := sidQuantize12Bit(sample); got != want {
		t.Fatalf("sidQuantize12Bit(%f)=%f, want truncation %f", sample, got, want)
	}
}

func TestPhaseR_PWMDepthUsesFullByteScale(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(SQUARE_DUTY, (128<<8)|128)
	if got, want := chip.channels[0].pwmDepth, float32(128)/256.0; got != want {
		t.Fatalf("legacy pwmDepth=%f, want %f", got, want)
	}
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_DUTY, (128<<8)|128)
	if got, want := chip.channels[0].pwmDepth, float32(128)/256.0; got != want {
		t.Fatalf("flex pwmDepth=%f, want %f", got, want)
	}
}

func TestPhaseR_GlobalFilterCutoffSmoothsTowardTarget(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.HandleRegisterWrite(FILTER_CUTOFF, 255)
	chip.HandleRegisterWrite(FILTER_TYPE, 1)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, 255)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 3)

	if chip.filterCutoff >= chip.filterCutoffTarget {
		t.Fatalf("filter cutoff should cold-start below target: cutoff=%f target=%f", chip.filterCutoff, chip.filterCutoffTarget)
	}
	before := chip.filterCutoff
	_ = chip.GenerateSample()
	if got := chip.filterCutoff; got <= before || got >= chip.filterCutoffTarget {
		t.Fatalf("filter cutoff did not smooth toward target: before=%f after=%f target=%f", before, got, chip.filterCutoffTarget)
	}
}

func TestPhaseR_GlobalFilterZeroWritesRemainValidTargets(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.HandleRegisterWrite(FILTER_CUTOFF, 255)
	chip.HandleRegisterWrite(FILTER_RESONANCE, 255)
	chip.HandleRegisterWrite(FILTER_TYPE, 1)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, 255)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 3)

	_ = chip.GenerateSample()
	if chip.filterCutoff <= 0 || chip.filterResonance <= 0 {
		t.Fatalf("filter did not rise from nonzero writes: cutoff=%f resonance=%f", chip.filterCutoff, chip.filterResonance)
	}
	beforeCutoff := chip.filterCutoff
	beforeResonance := chip.filterResonance

	chip.HandleRegisterWrite(FILTER_CUTOFF, 0)
	chip.HandleRegisterWrite(FILTER_RESONANCE, 0)
	if chip.filterCutoffTarget != 0 || chip.filterResonanceTarget != 0 {
		t.Fatalf("zero writes should set zero targets: cutoffTarget=%f resonanceTarget=%f", chip.filterCutoffTarget, chip.filterResonanceTarget)
	}

	_ = chip.GenerateSample()
	if chip.filterCutoff >= beforeCutoff {
		t.Fatalf("zero cutoff target was ignored: before=%f after=%f target=%f", beforeCutoff, chip.filterCutoff, chip.filterCutoffTarget)
	}
	if chip.filterResonance >= beforeResonance {
		t.Fatalf("zero resonance target was ignored: before=%f after=%f target=%f", beforeResonance, chip.filterResonance, chip.filterResonanceTarget)
	}
}

func TestPhaseR_EnhancedOversampleAdvancesEnvelopePerSubsample(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.frequency = 440
	ch.volume = 1
	ch.attackTime = 100
	ch.attackRecip = 1.0 / 100.0
	ch.envelopePhase = ENV_ATTACK
	ch.psgPlusEnabled = true
	ch.psgPlusOversample = 4
	ch.psgPlusGain = 1

	_ = ch.generateSample()
	if ch.envelopeSample != 4 {
		t.Fatalf("enhanced envelopeSample=%d, want 4", ch.envelopeSample)
	}
}

func TestPhaseR_CombinedSIDNoiseUsesSelectedNoiseModeAndNoiseFrequency(t *testing.T) {
	modes := []int{NOISE_MODE_WHITE, NOISE_MODE_PSG, NOISE_MODE_TED_8BIT}
	got := make(map[uint32]bool)
	for _, mode := range modes {
		ch := &Channel{
			frequency:      5,
			noiseFrequency: 5,
			noiseMode:      mode,
			noiseSR:        NOISE_LFSR_SEED,
			sidWaveMask:    SID_WAVE_SAW | SID_WAVE_NOISE,
		}
		_ = ch.generateWaveSample(1, 1)
		got[ch.noiseSR] = true
	}
	if len(got) != len(modes) {
		t.Fatalf("combined SID noise modes produced non-distinct LFSRs: %v", got)
	}
}

func TestPhaseR_SID6581FilterUsesSameKneeScaleOnBothPolarities(t *testing.T) {
	pos := sid6581FilterDistort(SID_6581_FILTER_THRESHOLD_POS + 0.1)
	neg := sid6581FilterDistort(-(SID_6581_FILTER_THRESHOLD_NEG + 0.1))
	posExcess := pos - SID_6581_FILTER_THRESHOLD_POS
	negExcess := -neg - SID_6581_FILTER_THRESHOLD_NEG
	if math.Abs(float64(posExcess-negExcess)) > 0.0001 {
		t.Fatalf("filter knee excess differs: pos=%f neg=%f", posExcess, negExcess)
	}
}
