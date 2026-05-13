package main

import (
	"encoding/json"
	"fmt"
)

const (
	videoChipSnapshotVersion = 1
	soundChipSnapshotVersion = 1
)

type videoChipDebugSnapshot struct {
	FrameCounter uint64
	CurrentMode  uint32
	Enabled      bool
	HasContent   bool
	InVBlank     bool
	EverSignaled bool
	Stopped      bool
	Framebuffer  bool
	Resetting    bool

	BigEndianMode bool
	Layer         int

	FrontBuffer []byte
	BackBuffer  []byte
	PrevVRAM    []byte
	CLUTFrame   []byte

	CopperEnabled             bool
	CopperPtrStaged           uint32
	CopperPtr                 uint32
	CopperPC                  uint32
	CopperWaiting             bool
	CopperHalted              bool
	CopperWaitX               uint16
	CopperWaitY               uint16
	CopperRasterX             uint16
	CopperRasterY             uint16
	CopperIOBase              uint32
	CopperManagedByCompositor bool

	BlitterEnabled bool
	BltIRQEnabled  bool
	BltBusy        bool
	BltPending     bool
	BltErr         bool
	BltDone        bool
	BltIRQPend     bool

	BltStaged [20]uint32
	BltRun    [20]uint32

	RasterY      uint32
	RasterHeight uint32
	RasterColor  uint32
	RasterCtrl   uint32

	CLUTMode      bool
	CLUTPalette   [256]uint32
	CLUTPaletteHW [256]uint32
	PalIndex      uint32
	FBBase        uint32
	CLUTWarnFrame uint64
	CLUTWarned    bool
}

func (chip *VideoChip) DebugSnapshotName() string {
	return "video"
}

func (chip *VideoChip) DebugSnapshot() (uint32, []byte, error) {
	if chip == nil {
		return videoChipSnapshotVersion, nil, fmt.Errorf("nil video chip")
	}
	chip.mu.Lock()
	defer chip.mu.Unlock()
	snap := videoChipDebugSnapshot{
		FrameCounter:              chip.frameCounter,
		CurrentMode:               chip.currentMode,
		Enabled:                   chip.enabled.Load(),
		HasContent:                chip.hasContent.Load(),
		InVBlank:                  chip.inVBlank.Load(),
		EverSignaled:              chip.everSignaled.Load(),
		Stopped:                   chip.stopped.Load(),
		Framebuffer:               chip.framebufferErr.Load(),
		Resetting:                 chip.resetting,
		BigEndianMode:             chip.bigEndianMode,
		Layer:                     chip.layer,
		FrontBuffer:               append([]byte(nil), chip.frontBuffer...),
		BackBuffer:                append([]byte(nil), chip.backBuffer...),
		PrevVRAM:                  append([]byte(nil), chip.prevVRAM...),
		CLUTFrame:                 append([]byte(nil), chip.clutFrame...),
		CopperEnabled:             chip.copperEnabled,
		CopperPtrStaged:           chip.copperPtrStaged,
		CopperPtr:                 chip.copperPtr,
		CopperPC:                  chip.copperPC,
		CopperWaiting:             chip.copperWaiting,
		CopperHalted:              chip.copperHalted,
		CopperWaitX:               chip.copperWaitX,
		CopperWaitY:               chip.copperWaitY,
		CopperRasterX:             chip.copperRasterX,
		CopperRasterY:             chip.copperRasterY,
		CopperIOBase:              chip.copperIOBase,
		CopperManagedByCompositor: chip.copperManagedByCompositor,
		BlitterEnabled:            chip.blitterEnabled,
		BltIRQEnabled:             chip.bltIrqEnabled,
		BltBusy:                   chip.bltBusy,
		BltPending:                chip.bltPending,
		BltErr:                    chip.bltErr,
		BltDone:                   chip.bltDone,
		BltIRQPend:                chip.bltIrqPend,
		BltStaged: [20]uint32{
			chip.bltOpStaged, chip.bltSrcStaged, chip.bltDstStaged, chip.bltWidthStaged,
			chip.bltHeightStaged, chip.bltSrcStride, chip.bltDstStride, chip.bltColorStaged,
			chip.bltMaskStaged, chip.bltMode7U0Staged, chip.bltMode7V0Staged, chip.bltMode7DuColStaged,
			chip.bltMode7DvColStaged, chip.bltMode7DuRowStaged, chip.bltMode7DvRowStaged, chip.bltMode7TexWStaged,
			chip.bltMode7TexHStaged, chip.bltFlagsStaged, chip.bltFGStaged, chip.bltBGStaged,
		},
		BltRun: [20]uint32{
			chip.bltOp, chip.bltSrc, chip.bltDst, chip.bltWidth,
			chip.bltHeight, chip.bltSrcStrideRun, chip.bltDstStrideRun, chip.bltColor,
			chip.bltMask, chip.bltMode7U0, chip.bltMode7V0, chip.bltMode7DuCol,
			chip.bltMode7DvCol, chip.bltMode7DuRow, chip.bltMode7DvRow, chip.bltMode7TexW,
			chip.bltMode7TexH, chip.bltFlags, chip.bltFG, chip.bltBG,
		},
		RasterY:       chip.rasterY,
		RasterHeight:  chip.rasterHeight,
		RasterColor:   chip.rasterColor,
		RasterCtrl:    chip.rasterCtrl,
		CLUTMode:      chip.clutMode.Load(),
		CLUTPalette:   chip.clutPalette,
		CLUTPaletteHW: chip.clutPaletteHW,
		PalIndex:      chip.palIndex,
		FBBase:        chip.fbBase,
		CLUTWarnFrame: chip.clutWarnFrame,
		CLUTWarned:    chip.clutWarned,
	}
	data, err := json.Marshal(snap)
	return videoChipSnapshotVersion, data, err
}

func (chip *VideoChip) DebugRestoreSnapshot(version uint32, data []byte) error {
	if chip == nil {
		return fmt.Errorf("nil video chip")
	}
	if version != videoChipSnapshotVersion {
		return fmt.Errorf("unsupported video snapshot version %d", version)
	}
	var snap videoChipDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.frameCounter = snap.FrameCounter
	chip.currentMode = snap.CurrentMode
	chip.enabled.Store(snap.Enabled)
	chip.hasContent.Store(snap.HasContent)
	chip.inVBlank.Store(snap.InVBlank)
	chip.everSignaled.Store(snap.EverSignaled)
	chip.stopped.Store(snap.Stopped)
	chip.framebufferErr.Store(snap.Framebuffer)
	chip.resetting = snap.Resetting
	chip.bigEndianMode = snap.BigEndianMode
	chip.layer = snap.Layer
	chip.frontBuffer = append(chip.frontBuffer[:0], snap.FrontBuffer...)
	chip.backBuffer = append(chip.backBuffer[:0], snap.BackBuffer...)
	chip.prevVRAM = append(chip.prevVRAM[:0], snap.PrevVRAM...)
	chip.clutFrame = append(chip.clutFrame[:0], snap.CLUTFrame...)
	chip.copperEnabled = snap.CopperEnabled
	chip.copperPtrStaged = snap.CopperPtrStaged
	chip.copperPtr = snap.CopperPtr
	chip.copperPC = snap.CopperPC
	chip.copperWaiting = snap.CopperWaiting
	chip.copperHalted = snap.CopperHalted
	chip.copperWaitX = snap.CopperWaitX
	chip.copperWaitY = snap.CopperWaitY
	chip.copperRasterX = snap.CopperRasterX
	chip.copperRasterY = snap.CopperRasterY
	chip.copperIOBase = snap.CopperIOBase
	chip.copperManagedByCompositor = snap.CopperManagedByCompositor
	chip.blitterEnabled = snap.BlitterEnabled
	chip.bltIrqEnabled = snap.BltIRQEnabled
	chip.bltBusy = snap.BltBusy
	chip.bltPending = snap.BltPending
	chip.bltErr = snap.BltErr
	chip.bltDone = snap.BltDone
	chip.bltIrqPend = snap.BltIRQPend
	chip.bltOpStaged, chip.bltSrcStaged, chip.bltDstStaged, chip.bltWidthStaged = snap.BltStaged[0], snap.BltStaged[1], snap.BltStaged[2], snap.BltStaged[3]
	chip.bltHeightStaged, chip.bltSrcStride, chip.bltDstStride, chip.bltColorStaged = snap.BltStaged[4], snap.BltStaged[5], snap.BltStaged[6], snap.BltStaged[7]
	chip.bltMaskStaged, chip.bltMode7U0Staged, chip.bltMode7V0Staged, chip.bltMode7DuColStaged = snap.BltStaged[8], snap.BltStaged[9], snap.BltStaged[10], snap.BltStaged[11]
	chip.bltMode7DvColStaged, chip.bltMode7DuRowStaged, chip.bltMode7DvRowStaged, chip.bltMode7TexWStaged = snap.BltStaged[12], snap.BltStaged[13], snap.BltStaged[14], snap.BltStaged[15]
	chip.bltMode7TexHStaged, chip.bltFlagsStaged, chip.bltFGStaged, chip.bltBGStaged = snap.BltStaged[16], snap.BltStaged[17], snap.BltStaged[18], snap.BltStaged[19]
	chip.bltOp, chip.bltSrc, chip.bltDst, chip.bltWidth = snap.BltRun[0], snap.BltRun[1], snap.BltRun[2], snap.BltRun[3]
	chip.bltHeight, chip.bltSrcStrideRun, chip.bltDstStrideRun, chip.bltColor = snap.BltRun[4], snap.BltRun[5], snap.BltRun[6], snap.BltRun[7]
	chip.bltMask, chip.bltMode7U0, chip.bltMode7V0, chip.bltMode7DuCol = snap.BltRun[8], snap.BltRun[9], snap.BltRun[10], snap.BltRun[11]
	chip.bltMode7DvCol, chip.bltMode7DuRow, chip.bltMode7DvRow, chip.bltMode7TexW = snap.BltRun[12], snap.BltRun[13], snap.BltRun[14], snap.BltRun[15]
	chip.bltMode7TexH, chip.bltFlags, chip.bltFG, chip.bltBG = snap.BltRun[16], snap.BltRun[17], snap.BltRun[18], snap.BltRun[19]
	chip.rasterY = snap.RasterY
	chip.rasterHeight = snap.RasterHeight
	chip.rasterColor = snap.RasterColor
	chip.rasterCtrl = snap.RasterCtrl
	chip.clutMode.Store(snap.CLUTMode)
	chip.clutPalette = snap.CLUTPalette
	chip.clutPaletteHW = snap.CLUTPaletteHW
	chip.palIndex = snap.PalIndex
	chip.fbBase = snap.FBBase
	chip.clutWarnFrame = snap.CLUTWarnFrame
	chip.clutWarned = snap.CLUTWarned
	return nil
}

type soundChannelDebugSnapshot struct {
	Frequency, Phase, Volume, EnvelopeLevel, PrevRawSample      float32
	DutyCycle, NoisePhase, NoiseValue, NoiseMix                 float32
	NoiseFrequency, DACValue, NoiseFilter, NoiseFilterState     float32
	NoiseSR                                                     uint32
	WaveType, NoiseMode, AttackTime, DecayTime, ReleaseTime     int
	EnvelopeSample, EnvelopePhase, EnvelopeShape                int
	SweepPeriod, SweepCounter                                   int
	SweepShift                                                  uint
	Enabled, Gate, SweepEnabled, SweepDirection                 bool
	PWMEnabled, PhaseWrapped, PhaseMSB                          bool
	PSGPlusEnabled, POKEYPlusEnabled, SIDPlusEnabled            bool
	TEDPlusEnabled, AHXPlusEnabled                              bool
	SIDEnvelope, SIDTestBit, SIDFilterMode, SIDRateCounter      bool
	SIDDACEnabled, SIDADSRBugsEnabled, SIDNoisePhaseLocked      bool
	SID6581FilterDistort, DACMode                               bool
	SIDEnvLevel, SIDAttackIndex, SIDDecayIndex, SIDReleaseIndex uint8
	SIDADSRDelayCounter                                         uint16
	SIDCycleAccum, SIDCyclesPerSample                           float64
	SIDExpIndex, SampleCount                                    int
	ReleaseStartLevel, ReleaseDecay, AttackRecip                float32
	DecayRecip, ReleaseRecip                                    float32
}

type soundChipDebugSnapshot struct {
	FilterLP, FilterBP, FilterHP                                 float32
	FilterCutoff, FilterResonance                                float32
	FilterCutoffTarget, FilterResonanceTarget                    float32
	FilterModAmount, OverdriveLevel, OverdriveGain               float32
	ReverbMix, SIDMixerDCOffset                                  float32
	FilterType                                                   int
	Enabled, SIDMixerEnabled, SIDMixerSaturate                   bool
	AudioFrozen                                                  bool
	PreDelayPos                                                  int
	AllpassPos                                                   [NUM_ALLPASS_FILTERS]int
	FlexShadow                                                   [NUM_CHANNELS * FLEX_CH_STRIDE]byte
	Channels                                                     [NUM_CHANNELS]soundChannelDebugSnapshot
	SNVoices                                                     [4]soundChannelDebugSnapshot
	CombDecay                                                    [NUM_COMB_FILTERS]float32
	CombPos                                                      [NUM_COMB_FILTERS]int
	CombBuffers                                                  [NUM_COMB_FILTERS][]float32
	AllpassBuffers                                               [NUM_ALLPASS_FILTERS][]float32
	PreDelayBuffer                                               []float32
	MasterGainDB, MasterGainLinear                               float32
	MasterAutoLevelEnabled                                       bool
	MasterAutoTargetDB, MasterAutoMinGainDB, MasterAutoMaxGainDB float32
	MasterAutoAttackMS, MasterAutoReleaseMS                      float32
	MasterAutoAttackCoef, MasterAutoReleaseCoef                  float32
	MasterAutoLevel, MasterAutoGain                              float32
	MasterCompEnabled                                            bool
	MasterCompThresholdDB, MasterCompRatio                       float32
	MasterCompAttackMS, MasterCompReleaseMS                      float32
	MasterCompKneeDB, MasterCompMakeupDB, MasterCompMakeupLinear float32
	MasterCompLookaheadMS                                        float32
	MasterCompAttackCoef, MasterCompReleaseCoef                  float32
	MasterCompLookaheadLen                                       int
	MasterCompEnvelope                                           float32
	MasterCompLookaheadBuf                                       [MASTER_COMPRESSOR_LOOKAHEAD_MAX]float32
	MasterCompWritePos, MasterCompReadPos                        int
}

func snapshotSoundChannel(ch *Channel) soundChannelDebugSnapshot {
	if ch == nil {
		return soundChannelDebugSnapshot{}
	}
	return soundChannelDebugSnapshot{
		Frequency: ch.frequency, Phase: ch.phase, Volume: ch.volume, EnvelopeLevel: ch.envelopeLevel,
		PrevRawSample: ch.prevRawSample, DutyCycle: ch.dutyCycle, NoisePhase: ch.noisePhase,
		NoiseValue: ch.noiseValue, NoiseMix: ch.noiseMix, NoiseFrequency: ch.noiseFrequency,
		DACValue: ch.dacValue, NoiseFilter: ch.noiseFilter, NoiseFilterState: ch.noiseFilterState,
		NoiseSR: ch.noiseSR, WaveType: ch.waveType, NoiseMode: ch.noiseMode,
		AttackTime: ch.attackTime, DecayTime: ch.decayTime, ReleaseTime: ch.releaseTime,
		EnvelopeSample: ch.envelopeSample, EnvelopePhase: ch.envelopePhase, EnvelopeShape: ch.envelopeShape,
		SweepPeriod: ch.sweepPeriod, SweepCounter: ch.sweepCounter, SweepShift: ch.sweepShift,
		Enabled: ch.enabled, Gate: ch.gate, SweepEnabled: ch.sweepEnabled, SweepDirection: ch.sweepDirection,
		PWMEnabled: ch.pwmEnabled, PhaseWrapped: ch.phaseWrapped, PhaseMSB: ch.phaseMSB,
		PSGPlusEnabled: ch.psgPlusEnabled, POKEYPlusEnabled: ch.pokeyPlusEnabled,
		SIDPlusEnabled: ch.sidPlusEnabled, TEDPlusEnabled: ch.tedPlusEnabled, AHXPlusEnabled: ch.ahxPlusEnabled,
		SIDEnvelope: ch.sidEnvelope, SIDTestBit: ch.sidTestBit, SIDFilterMode: ch.sidFilterMode,
		SIDRateCounter: ch.sidRateCounter, SIDDACEnabled: ch.sidDACEnabled,
		SIDADSRBugsEnabled: ch.sidADSRBugsEnabled, SIDNoisePhaseLocked: ch.sidNoisePhaseLocked,
		SID6581FilterDistort: ch.sid6581FilterDistort, DACMode: ch.dacMode,
		SIDEnvLevel: ch.sidEnvLevel, SIDADSRDelayCounter: ch.sidADSRDelayCounter,
		SIDCycleAccum: ch.sidCycleAccum, SIDCyclesPerSample: ch.sidCyclesPerSample,
		SIDExpIndex: ch.sidExpIndex, SIDAttackIndex: ch.sidAttackIndex,
		SIDDecayIndex: ch.sidDecayIndex, SIDReleaseIndex: ch.sidReleaseIndex,
		SampleCount: ch.sampleCount, ReleaseStartLevel: ch.releaseStartLevel,
		ReleaseDecay: ch.releaseDecay, AttackRecip: ch.attackRecip, DecayRecip: ch.decayRecip,
		ReleaseRecip: ch.releaseRecip,
	}
}

func restoreSoundChannel(ch *Channel, snap soundChannelDebugSnapshot) {
	if ch == nil {
		return
	}
	ch.frequency, ch.phase, ch.volume, ch.envelopeLevel = snap.Frequency, snap.Phase, snap.Volume, snap.EnvelopeLevel
	ch.prevRawSample, ch.dutyCycle, ch.noisePhase = snap.PrevRawSample, snap.DutyCycle, snap.NoisePhase
	ch.noiseValue, ch.noiseMix, ch.noiseFrequency = snap.NoiseValue, snap.NoiseMix, snap.NoiseFrequency
	ch.dacValue, ch.noiseFilter, ch.noiseFilterState, ch.noiseSR = snap.DACValue, snap.NoiseFilter, snap.NoiseFilterState, snap.NoiseSR
	ch.waveType, ch.noiseMode = snap.WaveType, snap.NoiseMode
	ch.attackTime, ch.decayTime, ch.releaseTime = snap.AttackTime, snap.DecayTime, snap.ReleaseTime
	ch.envelopeSample, ch.envelopePhase, ch.envelopeShape = snap.EnvelopeSample, snap.EnvelopePhase, snap.EnvelopeShape
	ch.sweepPeriod, ch.sweepCounter, ch.sweepShift = snap.SweepPeriod, snap.SweepCounter, snap.SweepShift
	ch.enabled, ch.gate, ch.sweepEnabled, ch.sweepDirection = snap.Enabled, snap.Gate, snap.SweepEnabled, snap.SweepDirection
	ch.pwmEnabled, ch.phaseWrapped, ch.phaseMSB = snap.PWMEnabled, snap.PhaseWrapped, snap.PhaseMSB
	ch.psgPlusEnabled, ch.pokeyPlusEnabled, ch.sidPlusEnabled = snap.PSGPlusEnabled, snap.POKEYPlusEnabled, snap.SIDPlusEnabled
	ch.tedPlusEnabled, ch.ahxPlusEnabled = snap.TEDPlusEnabled, snap.AHXPlusEnabled
	ch.sidEnvelope, ch.sidTestBit, ch.sidFilterMode, ch.sidRateCounter = snap.SIDEnvelope, snap.SIDTestBit, snap.SIDFilterMode, snap.SIDRateCounter
	ch.sidDACEnabled, ch.sidADSRBugsEnabled = snap.SIDDACEnabled, snap.SIDADSRBugsEnabled
	ch.sidNoisePhaseLocked, ch.sid6581FilterDistort, ch.dacMode = snap.SIDNoisePhaseLocked, snap.SID6581FilterDistort, snap.DACMode
	ch.sidEnvLevel, ch.sidADSRDelayCounter = snap.SIDEnvLevel, snap.SIDADSRDelayCounter
	ch.sidCycleAccum, ch.sidCyclesPerSample, ch.sidExpIndex = snap.SIDCycleAccum, snap.SIDCyclesPerSample, snap.SIDExpIndex
	ch.sidAttackIndex, ch.sidDecayIndex, ch.sidReleaseIndex = snap.SIDAttackIndex, snap.SIDDecayIndex, snap.SIDReleaseIndex
	ch.sampleCount = snap.SampleCount
	ch.releaseStartLevel, ch.releaseDecay = snap.ReleaseStartLevel, snap.ReleaseDecay
	ch.attackRecip, ch.decayRecip, ch.releaseRecip = snap.AttackRecip, snap.DecayRecip, snap.ReleaseRecip
}

func (chip *SoundChip) DebugSnapshotName() string {
	return "sound"
}

func (chip *SoundChip) DebugSnapshot() (uint32, []byte, error) {
	if chip == nil {
		return soundChipSnapshotVersion, nil, fmt.Errorf("nil sound chip")
	}
	chip.mu.Lock()
	defer chip.mu.Unlock()
	snap := soundChipDebugSnapshot{
		FilterLP: chip.filterLP, FilterBP: chip.filterBP, FilterHP: chip.filterHP,
		FilterCutoff: chip.filterCutoff, FilterResonance: chip.filterResonance,
		FilterCutoffTarget: chip.filterCutoffTarget, FilterResonanceTarget: chip.filterResonanceTarget,
		FilterModAmount: chip.filterModAmount, OverdriveLevel: chip.overdriveLevel,
		OverdriveGain: chip.overdriveGain, ReverbMix: chip.reverbMix,
		SIDMixerDCOffset: chip.sidMixerDCOffset, FilterType: chip.filterType,
		Enabled: chip.enabled.Load(), SIDMixerEnabled: chip.sidMixerEnabled,
		SIDMixerSaturate: chip.sidMixerSaturate, AudioFrozen: chip.audioFrozen.Load(),
		PreDelayPos: chip.preDelayPos, AllpassPos: chip.allpassPos, FlexShadow: chip.flexShadow,
		PreDelayBuffer: append([]float32(nil), chip.preDelayBuf...),
		MasterGainDB:   chip.masterGainDB, MasterGainLinear: chip.masterGainLinear,
		MasterAutoLevelEnabled: chip.masterAutoLevelEnabled, MasterAutoTargetDB: chip.masterAutoTargetDB,
		MasterAutoMinGainDB: chip.masterAutoMinGainDB, MasterAutoMaxGainDB: chip.masterAutoMaxGainDB,
		MasterAutoAttackMS: chip.masterAutoAttackMS, MasterAutoReleaseMS: chip.masterAutoReleaseMS,
		MasterAutoAttackCoef: chip.masterAutoAttackCoef, MasterAutoReleaseCoef: chip.masterAutoReleaseCoef,
		MasterAutoLevel: chip.masterAutoLevel, MasterAutoGain: chip.masterAutoGain,
		MasterCompEnabled: chip.masterCompEnabled, MasterCompThresholdDB: chip.masterCompThresholdDB,
		MasterCompRatio: chip.masterCompRatio, MasterCompAttackMS: chip.masterCompAttackMS,
		MasterCompReleaseMS: chip.masterCompReleaseMS, MasterCompKneeDB: chip.masterCompKneeDB,
		MasterCompMakeupDB: chip.masterCompMakeupDB, MasterCompMakeupLinear: chip.masterCompMakeupLinear,
		MasterCompLookaheadMS: chip.masterCompLookaheadMS, MasterCompAttackCoef: chip.masterCompAttackCoef,
		MasterCompReleaseCoef: chip.masterCompReleaseCoef, MasterCompLookaheadLen: chip.masterCompLookaheadLen,
		MasterCompEnvelope: chip.masterCompEnvelope, MasterCompLookaheadBuf: chip.masterCompLookaheadBuf,
		MasterCompWritePos: chip.masterCompWritePos, MasterCompReadPos: chip.masterCompReadPos,
	}
	for i := range chip.channels {
		snap.Channels[i] = snapshotSoundChannel(chip.channels[i])
	}
	for i := range chip.snVoices {
		snap.SNVoices[i] = snapshotSoundChannel(&chip.snVoices[i])
	}
	for i := range chip.combFilters {
		snap.CombDecay[i] = chip.combFilters[i].decay
		snap.CombPos[i] = chip.combFilters[i].pos
		snap.CombBuffers[i] = append([]float32(nil), chip.combFilters[i].buffer...)
	}
	for i := range chip.allpassBuf {
		snap.AllpassBuffers[i] = append([]float32(nil), chip.allpassBuf[i]...)
	}
	data, err := json.Marshal(snap)
	return soundChipSnapshotVersion, data, err
}

func (chip *SoundChip) DebugRestoreSnapshot(version uint32, data []byte) error {
	if chip == nil {
		return fmt.Errorf("nil sound chip")
	}
	if version != soundChipSnapshotVersion {
		return fmt.Errorf("unsupported sound snapshot version %d", version)
	}
	var snap soundChipDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	chip.mu.Lock()
	defer chip.mu.Unlock()
	chip.filterLP, chip.filterBP, chip.filterHP = snap.FilterLP, snap.FilterBP, snap.FilterHP
	chip.filterCutoff, chip.filterResonance = snap.FilterCutoff, snap.FilterResonance
	chip.filterCutoffTarget, chip.filterResonanceTarget = snap.FilterCutoffTarget, snap.FilterResonanceTarget
	chip.filterModAmount, chip.overdriveLevel, chip.overdriveGain = snap.FilterModAmount, snap.OverdriveLevel, snap.OverdriveGain
	chip.reverbMix, chip.sidMixerDCOffset, chip.filterType = snap.ReverbMix, snap.SIDMixerDCOffset, snap.FilterType
	chip.enabled.Store(snap.Enabled)
	chip.sidMixerEnabled, chip.sidMixerSaturate = snap.SIDMixerEnabled, snap.SIDMixerSaturate
	chip.audioFrozen.Store(snap.AudioFrozen)
	chip.preDelayPos, chip.allpassPos, chip.flexShadow = snap.PreDelayPos, snap.AllpassPos, snap.FlexShadow
	chip.preDelayBuf = append(chip.preDelayBuf[:0], snap.PreDelayBuffer...)
	for i := range chip.channels {
		restoreSoundChannel(chip.channels[i], snap.Channels[i])
	}
	for i := range chip.snVoices {
		restoreSoundChannel(&chip.snVoices[i], snap.SNVoices[i])
	}
	for i := range chip.combFilters {
		chip.combFilters[i].decay = snap.CombDecay[i]
		chip.combFilters[i].pos = snap.CombPos[i]
		chip.combFilters[i].buffer = append(chip.combFilters[i].buffer[:0], snap.CombBuffers[i]...)
	}
	for i := range chip.allpassBuf {
		chip.allpassBuf[i] = append(chip.allpassBuf[i][:0], snap.AllpassBuffers[i]...)
	}
	chip.masterGainDB, chip.masterGainLinear = snap.MasterGainDB, snap.MasterGainLinear
	chip.masterAutoLevelEnabled = snap.MasterAutoLevelEnabled
	chip.masterAutoTargetDB, chip.masterAutoMinGainDB, chip.masterAutoMaxGainDB = snap.MasterAutoTargetDB, snap.MasterAutoMinGainDB, snap.MasterAutoMaxGainDB
	chip.masterAutoAttackMS, chip.masterAutoReleaseMS = snap.MasterAutoAttackMS, snap.MasterAutoReleaseMS
	chip.masterAutoAttackCoef, chip.masterAutoReleaseCoef = snap.MasterAutoAttackCoef, snap.MasterAutoReleaseCoef
	chip.masterAutoLevel, chip.masterAutoGain = snap.MasterAutoLevel, snap.MasterAutoGain
	chip.masterCompEnabled = snap.MasterCompEnabled
	chip.masterCompThresholdDB, chip.masterCompRatio = snap.MasterCompThresholdDB, snap.MasterCompRatio
	chip.masterCompAttackMS, chip.masterCompReleaseMS = snap.MasterCompAttackMS, snap.MasterCompReleaseMS
	chip.masterCompKneeDB, chip.masterCompMakeupDB, chip.masterCompMakeupLinear = snap.MasterCompKneeDB, snap.MasterCompMakeupDB, snap.MasterCompMakeupLinear
	chip.masterCompLookaheadMS, chip.masterCompAttackCoef, chip.masterCompReleaseCoef = snap.MasterCompLookaheadMS, snap.MasterCompAttackCoef, snap.MasterCompReleaseCoef
	chip.masterCompLookaheadLen, chip.masterCompEnvelope = snap.MasterCompLookaheadLen, snap.MasterCompEnvelope
	chip.masterCompLookaheadBuf = snap.MasterCompLookaheadBuf
	chip.masterCompWritePos, chip.masterCompReadPos = snap.MasterCompWritePos, snap.MasterCompReadPos
	return nil
}

const (
	terminalMMIOSnapshotVersion = 1
	clipboardSnapshotVersion    = 1
	arosAudioDMASnapshotVersion = 1
)

type terminalMMIODebugSnapshot struct {
	InputBuf          [1024]byte
	InputHead         int
	InputTail         int
	InputLen          int
	Newlines          int
	OutputBuf         []byte
	EchoEnabled       bool
	ForceEchoOff      bool
	LineInputMode     bool
	RawKeyBuf         [256]byte
	RawKeyHead        int
	RawKeyTail        int
	RawKeyLen         int
	MouseX            int32
	MouseY            int32
	MouseDX           int32
	MouseDY           int32
	MouseButtons      uint32
	MouseChanged      bool
	MouseCtrl         uint32
	MouseOverride     bool
	MouseNativeW      int32
	MouseNativeH      int32
	ScanBuf           [256]uint8
	ScanHead          int
	ScanTail          int
	ScanLen           int
	Modifiers         uint32
	AmigaScancodeMode bool
	SentinelTriggered bool
	LastStatusRead    int64
}

func (tm *TerminalMMIO) DebugSnapshotName() string { return "terminal-mmio" }

func (tm *TerminalMMIO) DebugSnapshot() (uint32, []byte, error) {
	if tm == nil {
		return terminalMMIOSnapshotVersion, nil, fmt.Errorf("nil terminal MMIO")
	}
	tm.mu.Lock()
	snap := terminalMMIODebugSnapshot{
		InputBuf:          tm.inputBuf,
		InputHead:         tm.inputHead,
		InputTail:         tm.inputTail,
		InputLen:          tm.inputLen,
		Newlines:          tm.newlines,
		OutputBuf:         append([]byte(nil), tm.outputBuf...),
		EchoEnabled:       tm.echoEnabled,
		ForceEchoOff:      tm.forceEchoOff,
		LineInputMode:     tm.lineInputMode,
		RawKeyBuf:         tm.rawKeyBuf,
		RawKeyHead:        tm.rawKeyHead,
		RawKeyTail:        tm.rawKeyTail,
		RawKeyLen:         tm.rawKeyLen,
		MouseX:            tm.mouseX.Load(),
		MouseY:            tm.mouseY.Load(),
		MouseDX:           tm.mouseDX.Load(),
		MouseDY:           tm.mouseDY.Load(),
		MouseButtons:      tm.mouseButtons.Load(),
		MouseChanged:      tm.mouseChanged.Load(),
		MouseCtrl:         tm.mouseCtrl.Load(),
		MouseOverride:     tm.mouseOverride.Load(),
		MouseNativeW:      tm.mouseNativeW.Load(),
		MouseNativeH:      tm.mouseNativeH.Load(),
		ScanBuf:           tm.scanBuf,
		ScanHead:          tm.scanHead,
		ScanTail:          tm.scanTail,
		ScanLen:           tm.scanLen,
		Modifiers:         tm.modifiers.Load(),
		AmigaScancodeMode: tm.amigaScancodeMode.Load(),
		SentinelTriggered: tm.SentinelTriggered.Load(),
		LastStatusRead:    tm.lastStatusRead.Load(),
	}
	tm.mu.Unlock()
	data, err := json.Marshal(snap)
	return terminalMMIOSnapshotVersion, data, err
}

func (tm *TerminalMMIO) DebugRestoreSnapshot(version uint32, data []byte) error {
	if tm == nil {
		return fmt.Errorf("nil terminal MMIO")
	}
	if version != terminalMMIOSnapshotVersion {
		return fmt.Errorf("unsupported terminal MMIO snapshot version %d", version)
	}
	var snap terminalMMIODebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	tm.mu.Lock()
	tm.inputBuf, tm.inputHead, tm.inputTail, tm.inputLen, tm.newlines = snap.InputBuf, snap.InputHead, snap.InputTail, snap.InputLen, snap.Newlines
	tm.outputBuf = append(tm.outputBuf[:0], snap.OutputBuf...)
	tm.echoEnabled, tm.forceEchoOff, tm.lineInputMode = snap.EchoEnabled, snap.ForceEchoOff, snap.LineInputMode
	tm.rawKeyBuf, tm.rawKeyHead, tm.rawKeyTail, tm.rawKeyLen = snap.RawKeyBuf, snap.RawKeyHead, snap.RawKeyTail, snap.RawKeyLen
	tm.scanBuf, tm.scanHead, tm.scanTail, tm.scanLen = snap.ScanBuf, snap.ScanHead, snap.ScanTail, snap.ScanLen
	tm.mu.Unlock()
	tm.mouseX.Store(snap.MouseX)
	tm.mouseY.Store(snap.MouseY)
	tm.mouseDX.Store(snap.MouseDX)
	tm.mouseDY.Store(snap.MouseDY)
	tm.mouseButtons.Store(snap.MouseButtons)
	tm.mouseChanged.Store(snap.MouseChanged)
	tm.mouseCtrl.Store(snap.MouseCtrl)
	tm.mouseOverride.Store(snap.MouseOverride)
	tm.mouseNativeW.Store(snap.MouseNativeW)
	tm.mouseNativeH.Store(snap.MouseNativeH)
	tm.modifiers.Store(snap.Modifiers)
	tm.amigaScancodeMode.Store(snap.AmigaScancodeMode)
	tm.SentinelTriggered.Store(snap.SentinelTriggered)
	tm.lastStatusRead.Store(snap.LastStatusRead)
	return nil
}

type clipboardDebugSnapshot struct {
	DataPtr   uint32
	DataLen   uint32
	Status    uint32
	ResultLen uint32
	Format    uint32
}

func (cb *ClipboardBridge) DebugSnapshotName() string { return "clipboard-bridge" }

func (cb *ClipboardBridge) DebugSnapshot() (uint32, []byte, error) {
	if cb == nil {
		return clipboardSnapshotVersion, nil, fmt.Errorf("nil clipboard bridge")
	}
	data, err := json.Marshal(clipboardDebugSnapshot{DataPtr: cb.dataPtr, DataLen: cb.dataLen, Status: cb.status, ResultLen: cb.resultLen, Format: cb.format})
	return clipboardSnapshotVersion, data, err
}

func (cb *ClipboardBridge) DebugRestoreSnapshot(version uint32, data []byte) error {
	if cb == nil {
		return fmt.Errorf("nil clipboard bridge")
	}
	if version != clipboardSnapshotVersion {
		return fmt.Errorf("unsupported clipboard snapshot version %d", version)
	}
	var snap clipboardDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	cb.dataPtr, cb.dataLen, cb.status, cb.resultLen, cb.format = snap.DataPtr, snap.DataLen, snap.Status, snap.ResultLen, snap.Format
	return nil
}

type arosAudioDMAChannelSnapshot struct {
	Ptr, Len, Per, Vol     uint32
	LPtr, LLen, LPer, LVol uint32
	NPtr, NLen, NPer, NVol uint32
	HasNext                bool
	Pos                    uint32
	Phase                  float64
	Active                 bool
}

type arosAudioDMADebugSnapshot struct {
	ProfileTop uint32
	Channels   [4]arosAudioDMAChannelSnapshot
	DMACON     uint32
	Status     uint32
	INTENA     uint32
	Enabled    bool
}

func (dma *ArosAudioDMA) DebugSnapshotName() string { return "aros-audio-dma" }

func (dma *ArosAudioDMA) DebugSnapshot() (uint32, []byte, error) {
	if dma == nil {
		return arosAudioDMASnapshotVersion, nil, fmt.Errorf("nil AROS audio DMA")
	}
	dma.mu.Lock()
	defer dma.mu.Unlock()
	snap := arosAudioDMADebugSnapshot{
		ProfileTop: dma.profileTop,
		DMACON:     dma.dmacon,
		Status:     dma.status,
		INTENA:     dma.intena,
		Enabled:    dma.enabled.Load(),
	}
	for i, ch := range dma.channels {
		snap.Channels[i] = arosAudioDMAChannelSnapshot{
			Ptr: ch.ptr, Len: ch.len, Per: ch.per, Vol: ch.vol,
			LPtr: ch.lptr, LLen: ch.llen, LPer: ch.lper, LVol: ch.lvol,
			NPtr: ch.nptr, NLen: ch.nlen, NPer: ch.nper, NVol: ch.nvol,
			HasNext: ch.hasNext, Pos: ch.pos, Phase: ch.phase, Active: ch.active,
		}
	}
	data, err := json.Marshal(snap)
	return arosAudioDMASnapshotVersion, data, err
}

func (dma *ArosAudioDMA) DebugRestoreSnapshot(version uint32, data []byte) error {
	if dma == nil {
		return fmt.Errorf("nil AROS audio DMA")
	}
	if version != arosAudioDMASnapshotVersion {
		return fmt.Errorf("unsupported AROS audio DMA snapshot version %d", version)
	}
	var snap arosAudioDMADebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	dma.mu.Lock()
	defer dma.mu.Unlock()
	dma.profileTop = snap.ProfileTop
	dma.dmacon, dma.status, dma.intena = snap.DMACON, snap.Status, snap.INTENA
	for i, ch := range snap.Channels {
		dma.channels[i] = arosAudDMACh{
			ptr: ch.Ptr, len: ch.Len, per: ch.Per, vol: ch.Vol,
			lptr: ch.LPtr, llen: ch.LLen, lper: ch.LPer, lvol: ch.LVol,
			nptr: ch.NPtr, nlen: ch.NLen, nper: ch.NPer, nvol: ch.NVol,
			hasNext: ch.HasNext, pos: ch.Pos, phase: ch.Phase, active: ch.Active,
		}
	}
	dma.enabled.Store(snap.Enabled)
	return nil
}
