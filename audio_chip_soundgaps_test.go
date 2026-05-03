package main

import (
	"math"
	"testing"
	"time"
)

func TestAudioRegionEndMatchesSoundChipEnd(t *testing.T) {
	if AUDIO_REGION_END != AUDIO_REG_END {
		t.Fatalf("AUDIO_REGION_END=0x%X, want AUDIO_REG_END=0x%X", AUDIO_REGION_END, AUDIO_REG_END)
	}
	if FLEX_CH_PRIMARY_END != AUDIO_REG_END {
		t.Fatalf("FLEX_CH_PRIMARY_END=0x%X, want 0x%X", FLEX_CH_PRIMARY_END, AUDIO_REG_END)
	}
}

func TestSoundChipSineSweepTargetsSineChannel(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(SINE_FREQ, 440*256)
	chip.HandleRegisterWrite(SINE_SWEEP, SWEEP_ENABLE_MASK|0x23)

	if chip.channels[0].sweepEnabled {
		t.Fatalf("SINE_SWEEP enabled square channel sweep")
	}
	if !chip.channels[2].sweepEnabled {
		t.Fatalf("SINE_SWEEP did not enable sine channel sweep")
	}
	if got := chip.channels[2].sweepShift; got != 3 {
		t.Fatalf("sine sweep shift=%d, want written shift 3", got)
	}
}

func TestSoundChipSweepPeriodZeroDisablesSweep(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_SWEEP, SWEEP_ENABLE_MASK)

	ch := chip.channels[0]
	if ch.sweepEnabled {
		t.Fatalf("period-zero sweep should be disabled")
	}
	before := ch.frequency
	for range 100 {
		_ = chip.GenerateSample()
	}
	if ch.frequency != before {
		t.Fatalf("frequency changed with disabled zero-period sweep: got %f want %f", ch.frequency, before)
	}
}

func TestSoundChipFlexByteWritesMirrorBusMemory(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	base := uint32(FLEX_CH1_BASE + FLEX_OFF_FREQ)
	value := uint32(0x12345678)

	chip.HandleRegisterWrite8(base, byte(value))
	chip.HandleRegisterWrite8(base+1, byte(value>>8))
	chip.HandleRegisterWrite8(base+2, byte(value>>16))
	chip.HandleRegisterWrite8(base+3, byte(value>>24))

	got := uint32(fixture.mem[base]) |
		uint32(fixture.mem[base+1])<<8 |
		uint32(fixture.mem[base+2])<<16 |
		uint32(fixture.mem[base+3])<<24
	if got != value {
		t.Fatalf("byte write mirror=0x%08X, want 0x%08X", got, value)
	}

	chip.HandleRegisterWrite(base, value)
	got = uint32(fixture.mem[base]) |
		uint32(fixture.mem[base+1])<<8 |
		uint32(fixture.mem[base+2])<<16 |
		uint32(fixture.mem[base+3])<<24
	if got != value {
		t.Fatalf("word write mirror=0x%08X, want 0x%08X", got, value)
	}
}

func TestSoundChipFlexSelfModulationClearsSource(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_RINGMOD, 0x80)
	if chip.channels[1].ringModSource != chip.channels[0] {
		t.Fatalf("expected channel 1 ring source to be channel 0")
	}
	chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_RINGMOD, 0x81)
	if chip.channels[1].ringModSource != nil {
		t.Fatalf("self ring source was not cleared")
	}

	chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_SYNC, 0x80)
	if chip.channels[1].syncSource != chip.channels[0] {
		t.Fatalf("expected channel 1 sync source to be channel 0")
	}
	chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_SYNC, 0x81)
	if chip.channels[1].syncSource != nil {
		t.Fatalf("self sync source was not cleared")
	}
}

func TestSoundChipFlexFrequencyClampsUltrasonic(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 30000*256)
	if got := chip.channels[0].frequency; got != 0 {
		t.Fatalf("ultrasonic flex frequency=%f, want muted 0", got)
	}
}

func TestSoundChipFilterColdStartSeedsCutoff(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.SetChannelFilter(0, 0x01, 0.5, 0.25)
	ch := chip.channels[0]
	if ch.filterCutoff == 0 || ch.filterResonance == 0 {
		t.Fatalf("filter cold-start did not seed cutoff/resonance: cutoff=%f resonance=%f", ch.filterCutoff, ch.filterResonance)
	}
}

func TestSoundChipSID2SID3FlexRegistersReachChannels(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	cases := []struct {
		base uint32
		ch   int
	}{
		{SID2_FLEX_BASE, 4},
		{SID2_FLEX_BASE + FLEX_CH_STRIDE, 5},
		{SID2_FLEX_BASE + 2*FLEX_CH_STRIDE, 6},
		{SID3_FLEX_BASE, 7},
		{SID3_FLEX_BASE + FLEX_CH_STRIDE, 8},
		{SID3_FLEX_BASE + 2*FLEX_CH_STRIDE, 9},
	}
	for _, tc := range cases {
		chip.HandleRegisterWrite(tc.base+FLEX_OFF_FREQ, 880*256)
		if got := chip.channels[tc.ch].frequency; got != 880 {
			t.Fatalf("base 0x%X channel %d frequency=%f, want 880", tc.base, tc.ch, got)
		}
		chip.HandleRegisterWrite8(tc.base+FLEX_OFF_VOL, 0x7f)
		chip.HandleRegisterWrite8(tc.base+FLEX_OFF_VOL+1, 0)
		chip.HandleRegisterWrite8(tc.base+FLEX_OFF_VOL+2, 0)
		chip.HandleRegisterWrite8(tc.base+FLEX_OFF_VOL+3, 0)
		if got := chip.HandleRegisterRead(tc.base + FLEX_OFF_VOL); got != 0x7f {
			t.Fatalf("base 0x%X readback=0x%X, want 0x7f", tc.base, got)
		}
	}
}

func TestSoundChipAudioCtrlFreezeBit(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(AUDIO_CTRL, 0x03)
	if !chip.enabled.Load() || !chip.audioFrozen.Load() {
		t.Fatalf("AUDIO_CTRL did not set enable+freeze")
	}
	if got := chip.HandleRegisterRead(AUDIO_CTRL); got != 0x03 {
		t.Fatalf("AUDIO_CTRL readback=0x%X, want 0x03", got)
	}
	if sample := chip.ReadSample(); sample != 0 {
		t.Fatalf("frozen sample=%f, want 0", sample)
	}
}

func TestSoundChipEnvelopeShapePerChannel(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip

	chip.HandleRegisterWrite(ENV_SHAPE, ENV_SHAPE_SAW_UP)
	chip.HandleRegisterWrite(ENV_SHAPE_CH_BASE+4, ENV_SHAPE_LOOP)
	if got := chip.channels[0].envelopeShape; got != ENV_SHAPE_SAW_UP {
		t.Fatalf("channel 0 envelope shape=%d", got)
	}
	if got := chip.channels[1].envelopeShape; got != ENV_SHAPE_LOOP {
		t.Fatalf("channel 1 envelope shape=%d", got)
	}
	if got := chip.HandleRegisterRead(ENV_SHAPE_CH_BASE + 4); got != ENV_SHAPE_LOOP {
		t.Fatalf("channel 1 envelope shape readback=%d", got)
	}
}

func TestSoundChipInstantAttackAndZeroFrequencyRelease(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.SetChannelADSR(0, 0, 0, 0, 0.5)
	ch := chip.channels[0]
	ch.enabled = true
	ch.gate = true
	ch.envelopePhase = ENV_ATTACK

	_ = ch.generateSample()
	if ch.envelopeLevel != MAX_LEVEL || ch.envelopePhase != ENV_DECAY {
		t.Fatalf("instant attack level=%f phase=%d", ch.envelopeLevel, ch.envelopePhase)
	}

	ch.gate = false
	ch.frequency = 0
	ch.envelopePhase = ENV_RELEASE
	ch.envelopeLevel = 1
	_ = ch.generateSample()
	if ch.enabled || ch.envelopeLevel != 0 {
		t.Fatalf("zero-time release with freq=0 left enabled=%v level=%f", ch.enabled, ch.envelopeLevel)
	}
}

func TestSoundChipSweepCapsAgainstInitialFrequency(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_SWEEP, SWEEP_ENABLE_MASK|SWEEP_DIR_MASK|(1<<SWEEP_PERIOD_SHIFT)|1)
	ch := chip.channels[0]
	ch.enabled = true
	ch.sweepCounter = ch.sweepPeriod - 1
	for range 10000 {
		_ = ch.generateSample()
		ch.sweepCounter = ch.sweepPeriod - 1
	}
	if ch.frequency > 880.1 {
		t.Fatalf("sweep exceeded initial octave cap: %f", ch.frequency)
	}
}

func TestSoundChipNoiseSweepChangesNoiseFrequency(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	ch := chip.channels[3]
	ch.enabled = true
	ch.waveType = WAVE_NOISE
	ch.frequency = 100
	ch.noiseFrequency = 100
	ch.sweepInitialFreq = 100
	ch.sweepEnabled = true
	ch.sweepDirection = true
	ch.sweepPeriod = 1
	ch.sweepShift = 1
	ch.sweepCounter = 0

	_ = ch.generateSample()
	if ch.noiseFrequency <= 100 {
		t.Fatalf("noise sweep did not increase noise frequency: %f", ch.noiseFrequency)
	}
}

func TestSoundChipStopDoesNotHoldMutexWhileClosing(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.enabled.Store(true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		chip.Stop()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Stop timed out")
	}
}

func TestSoundChipMasterNormalizerConcurrentAccess(t *testing.T) {
	fixture := newAudioFixture(t)
	chip := fixture.chip
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, 255)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 0x03)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 1000 {
			chip.SetMasterGainDB(float32(i%12) - 6)
			chip.ConfigureMasterAutoLevel(-18, -10, 12, 5, 50)
			chip.ResetMasterDynamics()
		}
	}()
	for range 1000 {
		s := chip.GenerateSample()
		if math.IsNaN(float64(s)) {
			t.Fatalf("sample is NaN")
		}
	}
	<-done
}
