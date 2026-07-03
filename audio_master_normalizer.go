package main

import "math"

const MASTER_COMPRESSOR_LOOKAHEAD_MAX = 512

const (
	showreelAutoTargetDB    = -18.0
	showreelAutoMinGainDB   = -10.0
	showreelAutoMaxGainDB   = 12.0
	showreelAutoAttackMS    = 250.0
	showreelAutoReleaseMS   = 2500.0
	showreelCompThresholdDB = -8.0
	showreelCompRatio       = 2.5
	showreelCompAttackMS    = 6.0
	showreelCompReleaseMS   = 180.0
	showreelCompKneeDB      = 6.0
	showreelCompMakeupDB    = 0.0
	showreelCompLookaheadMS = 1.5
)

func dbToLinear(db float32) float32 {
	return float32(math.Pow(10, float64(db)/20.0))
}

func linearToDB(linear float32) float32 {
	if linear <= 1e-12 {
		return -120.0
	}
	return float32(20.0 * math.Log10(float64(linear)))
}

func msToCoeff(ms float32) float32 {
	if ms <= 0 {
		return 0
	}
	return float32(math.Exp(-1.0 / (float64(ms) * 0.001 * SAMPLE_RATE)))
}

func lookaheadSamples(ms float32) int {
	if ms <= 0 {
		return 0
	}
	return int(float64(ms) * float64(SAMPLE_RATE) / 1000.0)
}

func (chip *SoundChip) resetMasterNormalizerLocked() {
	chip.masterGainDB = 0
	chip.masterGainLinear = 1.0
	chip.masterAutoLevelEnabled = false
	chip.masterAutoTargetDB = showreelAutoTargetDB
	chip.masterAutoMinGainDB = showreelAutoMinGainDB
	chip.masterAutoMaxGainDB = showreelAutoMaxGainDB
	chip.masterAutoAttackMS = showreelAutoAttackMS
	chip.masterAutoReleaseMS = showreelAutoReleaseMS
	chip.masterAutoAttackCoef = msToCoeff(showreelAutoAttackMS)
	chip.masterAutoReleaseCoef = msToCoeff(showreelAutoReleaseMS)
	chip.masterCompEnabled = false
	chip.masterCompThresholdDB = showreelCompThresholdDB
	chip.masterCompRatio = showreelCompRatio
	chip.masterCompAttackMS = showreelCompAttackMS
	chip.masterCompReleaseMS = showreelCompReleaseMS
	chip.masterCompKneeDB = showreelCompKneeDB
	chip.masterCompMakeupDB = 0
	chip.masterCompMakeupLinear = 1.0
	chip.masterCompLookaheadMS = showreelCompLookaheadMS
	chip.masterCompAttackCoef = msToCoeff(showreelCompAttackMS)
	chip.masterCompReleaseCoef = msToCoeff(showreelCompReleaseMS)
	chip.masterCompLookaheadLen = lookaheadSamples(showreelCompLookaheadMS)
	if chip.masterCompLookaheadLen < 0 {
		chip.masterCompLookaheadLen = 0
	}
	if chip.masterCompLookaheadLen >= MASTER_COMPRESSOR_LOOKAHEAD_MAX {
		chip.masterCompLookaheadLen = MASTER_COMPRESSOR_LOOKAHEAD_MAX - 1
	}
	chip.resetMasterDynamicsLocked()
	chip.masterDynamicsGeneration++
	chip.masterAppliedDynamicsGeneration = chip.masterDynamicsGeneration
}

func (chip *SoundChip) resetMasterDynamicsLocked() {
	chip.resetMasterDynamicsForAudioThread(chip.masterCompLookaheadLen)
}

func (chip *SoundChip) resetMasterDynamicsForAudioThread(lookaheadLen int) {
	chip.masterAutoLevel = 0
	chip.masterAutoGain = 1.0
	chip.masterCompEnvelope = 1.0
	chip.masterCompWritePos = 0
	chip.masterCompReadPos = 0
	if lookaheadLen > 0 {
		chip.masterCompReadPos = (chip.masterCompWritePos + 1) % (lookaheadLen + 1)
	}
	for i := range chip.masterCompLookaheadBuf {
		chip.masterCompLookaheadBuf[i] = 0
	}
}

func (chip *SoundChip) SetMasterGainDB(db float32) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.masterGainDB = db
	chip.masterGainLinear = dbToLinear(db)
}

func (chip *SoundChip) MasterGainDB() float32 {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	return chip.masterGainDB
}

func (chip *SoundChip) SetMasterCompressorEnabled(enabled bool) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.masterCompEnabled = enabled
}

func (chip *SoundChip) SetMasterAutoLevelEnabled(enabled bool) {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.masterAutoLevelEnabled = enabled
}

func (chip *SoundChip) MasterAutoLevelEnabled() bool {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	return chip.masterAutoLevelEnabled
}

func (chip *SoundChip) ConfigureMasterAutoLevel(targetDB, minGainDB, maxGainDB, attackMS, releaseMS float32) {
	if maxGainDB < minGainDB {
		maxGainDB = minGainDB
	}
	if attackMS < 0 {
		attackMS = 0
	}
	if releaseMS < 0 {
		releaseMS = 0
	}

	chip.mu.Lock()
	defer chip.mu.Unlock()

	chip.masterAutoTargetDB = targetDB
	chip.masterAutoMinGainDB = minGainDB
	chip.masterAutoMaxGainDB = maxGainDB
	chip.masterAutoAttackMS = attackMS
	chip.masterAutoReleaseMS = releaseMS
	chip.masterAutoAttackCoef = msToCoeff(attackMS)
	chip.masterAutoReleaseCoef = msToCoeff(releaseMS)
	chip.masterDynamicsGeneration++
}

func (chip *SoundChip) MasterCompressorEnabled() bool {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	return chip.masterCompEnabled
}

func (chip *SoundChip) ConfigureMasterCompressor(thresholdDB, ratio, attackMS, releaseMS, kneeDB, makeupDB, lookaheadMS float32) {
	if ratio < 1.0 {
		ratio = 1.0
	}
	if attackMS < 0 {
		attackMS = 0
	}
	if releaseMS < 0 {
		releaseMS = 0
	}
	if kneeDB < 0 {
		kneeDB = 0
	}
	if lookaheadMS < 0 {
		lookaheadMS = 0
	}

	chip.mu.Lock()
	defer chip.mu.Unlock()

	chip.masterCompThresholdDB = thresholdDB
	chip.masterCompRatio = ratio
	chip.masterCompAttackMS = attackMS
	chip.masterCompReleaseMS = releaseMS
	chip.masterCompKneeDB = kneeDB
	chip.masterCompMakeupDB = makeupDB
	chip.masterCompMakeupLinear = dbToLinear(makeupDB)
	chip.masterCompLookaheadMS = lookaheadMS
	chip.masterCompAttackCoef = msToCoeff(attackMS)
	chip.masterCompReleaseCoef = msToCoeff(releaseMS)
	chip.masterCompLookaheadLen = lookaheadSamples(lookaheadMS)
	if chip.masterCompLookaheadLen >= MASTER_COMPRESSOR_LOOKAHEAD_MAX {
		chip.masterCompLookaheadLen = MASTER_COMPRESSOR_LOOKAHEAD_MAX - 1
	}
	chip.masterDynamicsGeneration++
}

func (chip *SoundChip) UseShowreelNormalizerPreset() {
	chip.ConfigureMasterAutoLevel(
		showreelAutoTargetDB,
		showreelAutoMinGainDB,
		showreelAutoMaxGainDB,
		showreelAutoAttackMS,
		showreelAutoReleaseMS,
	)
	chip.SetMasterAutoLevelEnabled(true)
	chip.ConfigureMasterCompressor(
		showreelCompThresholdDB,
		showreelCompRatio,
		showreelCompAttackMS,
		showreelCompReleaseMS,
		showreelCompKneeDB,
		showreelCompMakeupDB,
		showreelCompLookaheadMS,
	)
	chip.SetMasterCompressorEnabled(true)
}

func (chip *SoundChip) ResetMasterDynamics() {
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.masterDynamicsGeneration++
}

func masterCompressorTargetGainDB(inputDB, thresholdDB, ratio, kneeDB float32) float32 {
	if ratio <= 1.0 {
		return 0
	}
	if kneeDB <= 0 {
		if inputDB <= thresholdDB {
			return 0
		}
		return (thresholdDB + (inputDB-thresholdDB)/ratio) - inputDB
	}

	lower := thresholdDB - kneeDB/2
	upper := thresholdDB + kneeDB/2
	if inputDB <= lower {
		return 0
	}
	if inputDB >= upper {
		return (thresholdDB + (inputDB-thresholdDB)/ratio) - inputDB
	}

	x := inputDB - lower
	return (1.0/ratio - 1.0) * x * x / (2.0 * kneeDB)
}

func (chip *SoundChip) snapshotMasterNormalizerConfigLocked() masterNormalizerConfig {
	return masterNormalizerConfig{
		generation:       chip.masterDynamicsGeneration,
		gainDB:           chip.masterGainDB,
		gainLinear:       chip.masterGainLinear,
		autoLevelEnabled: chip.masterAutoLevelEnabled,
		autoTargetDB:     chip.masterAutoTargetDB,
		autoMinGainDB:    chip.masterAutoMinGainDB,
		autoMaxGainDB:    chip.masterAutoMaxGainDB,
		autoAttackCoef:   chip.masterAutoAttackCoef,
		autoReleaseCoef:  chip.masterAutoReleaseCoef,
		compEnabled:      chip.masterCompEnabled,
		compThresholdDB:  chip.masterCompThresholdDB,
		compRatio:        chip.masterCompRatio,
		compKneeDB:       chip.masterCompKneeDB,
		compMakeupDB:     chip.masterCompMakeupDB,
		compMakeupLinear: chip.masterCompMakeupLinear,
		compAttackCoef:   chip.masterCompAttackCoef,
		compReleaseCoef:  chip.masterCompReleaseCoef,
		compLookaheadLen: chip.masterCompLookaheadLen,
	}
}

func (chip *SoundChip) snapshotMasterNormalizerConfigUnlocked() masterNormalizerConfig {
	return chip.snapshotMasterNormalizerConfigLocked()
}

func (chip *SoundChip) applyMasterNormalizer(sample float32) float32 {
	chip.postFXMu.Lock()
	defer chip.postFXMu.Unlock()
	return chip.applyMasterNormalizerWithConfig(sample, chip.snapshotMasterNormalizerConfigUnlocked())
}

func (chip *SoundChip) applyMasterNormalizerWithConfig(sample float32, cfg masterNormalizerConfig) float32 {
	if chip.masterAppliedDynamicsGeneration != cfg.generation {
		chip.resetMasterDynamicsForAudioThread(cfg.compLookaheadLen)
		chip.masterAppliedDynamicsGeneration = cfg.generation
	}

	gainLinear := cfg.gainLinear
	if gainLinear == 0 && cfg.gainDB == 0 {
		gainLinear = 1.0
	}
	scaled := sample * gainLinear
	if cfg.autoLevelEnabled {
		detected := float32(math.Abs(float64(scaled)))
		levelCoef := cfg.autoReleaseCoef
		if detected > chip.masterAutoLevel {
			levelCoef = cfg.autoAttackCoef
		}
		chip.masterAutoLevel = levelCoef*chip.masterAutoLevel + (1.0-levelCoef)*detected

		desiredGainDB := cfg.autoTargetDB - linearToDB(chip.masterAutoLevel)
		if desiredGainDB < cfg.autoMinGainDB {
			desiredGainDB = cfg.autoMinGainDB
		}
		if desiredGainDB > cfg.autoMaxGainDB {
			desiredGainDB = cfg.autoMaxGainDB
		}
		desiredGain := dbToLinear(desiredGainDB)
		gainCoef := cfg.autoReleaseCoef
		if desiredGain < chip.masterAutoGain {
			gainCoef = cfg.autoAttackCoef
		}
		chip.masterAutoGain = gainCoef*chip.masterAutoGain + (1.0-gainCoef)*desiredGain
		scaled *= chip.masterAutoGain
	}
	if !cfg.compEnabled {
		return clampF32(scaled, MIN_SAMPLE, MAX_SAMPLE)
	}

	detected := float32(math.Abs(float64(scaled)))
	targetGainDB := masterCompressorTargetGainDB(
		linearToDB(detected),
		cfg.compThresholdDB,
		cfg.compRatio,
		cfg.compKneeDB,
	)
	targetGain := dbToLinear(targetGainDB)
	coef := cfg.compReleaseCoef
	if targetGain < chip.masterCompEnvelope {
		coef = cfg.compAttackCoef
	}
	chip.masterCompEnvelope = coef*chip.masterCompEnvelope + (1.0-coef)*targetGain

	processed := scaled
	if cfg.compLookaheadLen > 0 {
		size := cfg.compLookaheadLen + 1
		processed = chip.masterCompLookaheadBuf[chip.masterCompReadPos]
		chip.masterCompLookaheadBuf[chip.masterCompWritePos] = scaled
		chip.masterCompWritePos = (chip.masterCompWritePos + 1) % size
		chip.masterCompReadPos = (chip.masterCompReadPos + 1) % size
	}

	makeupLinear := cfg.compMakeupLinear
	if makeupLinear == 0 && cfg.compMakeupDB == 0 {
		makeupLinear = 1.0
	}
	processed *= chip.masterCompEnvelope * makeupLinear
	return clampF32(processed, MIN_SAMPLE, MAX_SAMPLE)
}
