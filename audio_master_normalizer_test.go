package main

import (
	"math"
	"testing"
)

func TestSoundChipMasterGainDB_AdjustsOutput(t *testing.T) {
	chip := newTestSoundChip()

	base := chip.applyMasterNormalizer(0.1)
	chip.SetMasterGainDB(6.0)
	boosted := chip.applyMasterNormalizer(0.1)

	if boosted <= base*1.9 {
		t.Fatalf("boosted sample=%f, want significantly above base=%f", boosted, base)
	}
}

func TestSoundChipMasterCompressor_LimitsHotSignal(t *testing.T) {
	chip := newTestSoundChip()
	chip.ConfigureMasterCompressor(-18.0, 8.0, 0.1, 25.0, 0.0, 0.0, 0.0)
	chip.SetMasterCompressorEnabled(true)

	var out float32
	for range 2048 {
		out = chip.applyMasterNormalizer(0.9)
	}
	if math.Abs(float64(out)) >= 0.6 {
		t.Fatalf("compressed hot signal=%f, want < 0.6", out)
	}
}

func TestSoundChipMasterAutoLevel_BoostsQuietSignal(t *testing.T) {
	chip := newTestSoundChip()
	chip.ConfigureMasterAutoLevel(-18.0, -6.0, 12.0, 20.0, 200.0)
	chip.SetMasterAutoLevelEnabled(true)

	var out float32
	for range 4096 {
		out = chip.applyMasterNormalizer(0.03)
	}
	if out <= 0.05 {
		t.Fatalf("auto-leveled quiet signal=%f, want > 0.05", out)
	}
}

func TestSoundChipMasterAutoLevel_AttenuatesLoudSignal(t *testing.T) {
	chip := newTestSoundChip()
	chip.ConfigureMasterAutoLevel(-18.0, -12.0, 6.0, 20.0, 200.0)
	chip.SetMasterAutoLevelEnabled(true)

	var out float32
	for range 4096 {
		out = chip.applyMasterNormalizer(0.8)
	}
	if math.Abs(float64(out)) >= 0.6 {
		t.Fatalf("auto-leveled loud signal=%f, want < 0.6", out)
	}
}

func TestSoundChipMasterDynamicsResetClearsEnvelopeAndLookahead(t *testing.T) {
	chip := newTestSoundChip()
	chip.ConfigureMasterAutoLevel(-18.0, -6.0, 12.0, 20.0, 200.0)
	chip.SetMasterAutoLevelEnabled(true)
	chip.ConfigureMasterCompressor(-18.0, 8.0, 0.1, 250.0, 0.0, 0.0, 1.0)
	chip.SetMasterCompressorEnabled(true)

	for range 256 {
		_ = chip.applyMasterNormalizer(0.95)
	}
	if chip.masterAutoGain >= 0.99 {
		t.Fatalf("auto gain=%f, want auto leveling before reset", chip.masterAutoGain)
	}
	if chip.masterCompEnvelope >= 0.95 {
		t.Fatalf("envelope=%f, want compression before reset", chip.masterCompEnvelope)
	}

	chip.ResetMasterDynamics()
	if chip.masterAutoGain != 1.0 {
		t.Fatalf("auto gain=%f after reset, want 1.0", chip.masterAutoGain)
	}
	if chip.masterCompEnvelope != 1.0 {
		t.Fatalf("envelope=%f after reset, want 1.0", chip.masterCompEnvelope)
	}

	out := chip.applyMasterNormalizer(0)
	if out != 0 {
		t.Fatalf("post-reset silent output=%f, want 0", out)
	}
}
