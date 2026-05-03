//go:build headless

package main

import (
	"encoding/binary"
	"hash/crc32"
	"math"
	"sync"
	"testing"
)

func newTestMODEngine(t *testing.T) (*MODEngine, *SoundChip) {
	t.Helper()
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	engine := NewMODEngine(chip, SAMPLE_RATE)
	return engine, chip
}

// buildTestMODWithSample creates a MOD with a single sample of the given data,
// playing note at period 428 (C-2) on channel 0.
func buildTestMODWithSample(sampleData []int8) *MODFile {
	notes := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0, EffParam: 0},
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	return mod
}

func TestMODEngineChannelInit(t *testing.T) {
	engine, chip := newTestMODEngine(t)

	// Create a minimal MOD
	sampleData := make([]int8, 64)
	mod := buildTestMODWithSample(sampleData)
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	// First TickSample should initialize channels
	engine.TickSample()

	// Verify channels are in DAC mode
	for i := range modChannels {
		ch := chip.channels[i]
		if !ch.dacMode {
			t.Errorf("channel %d: expected dacMode=true", i)
		}
		if !ch.enabled {
			t.Errorf("channel %d: expected enabled=true", i)
		}
		if ch.volume != 1.0 {
			t.Errorf("channel %d: expected volume=1.0, got %f", i, ch.volume)
		}
	}
}

func TestMODEngineTickSampleWritesDAC(t *testing.T) {
	engine, chip := newTestMODEngine(t)

	// Create a sample with known values
	sampleData := make([]int8, 64)
	for i := range sampleData {
		sampleData[i] = 64 // mid-range positive value
	}
	mod := buildTestMODWithSample(sampleData)
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	// Tick enough samples to get past the first tick boundary and produce output
	for range 10 {
		engine.TickSample()
	}

	// Channel 0 should have a non-zero DAC value (sample 1 playing at period 428)
	ch := chip.channels[0]
	if !ch.dacMode {
		t.Fatal("expected channel 0 in DAC mode")
	}
	// The sample data is all 64, scaled by volume 64/64 = 64, then DAC normalizes 64/128 = 0.5
	// Check that dacValue is non-zero (sample is playing)
	sample := ch.generateSample()
	if sample == 0 {
		t.Error("expected non-zero DAC output from channel 0")
	}
}

func TestMODEnginePhaseIncrement(t *testing.T) {
	// Period 428 → phaseInc = 3546895 / (428 * 44100) ≈ 0.18795
	expected := modPALClock / (428.0 * float64(SAMPLE_RATE))
	mc := &MODChannel{period: 428}
	mc.updatePhaseInc(SAMPLE_RATE)

	if math.Abs(mc.phaseInc-expected) > 0.0001 {
		t.Errorf("expected phaseInc ≈ %f, got %f", expected, mc.phaseInc)
	}
}

func TestMOD_Engine_UsesEngineSampleRate(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	engine.sampleRate = 22050
	mod := buildTestMODWithSample(make([]int8, 64))
	engine.LoadMOD(mod)
	engine.SetPlaying(true)
	engine.TickSample()

	if got, want := engine.samplesPerTick, SamplesPerTick(22050, 125); got != want {
		t.Fatalf("samplesPerTick=%d, want %d", got, want)
	}
	if got := engine.replayer.channels[0].phaseInc; math.Abs(got-(modPALClock/(428.0*22050.0))) > 0.0001 {
		t.Fatalf("phaseInc=%f, want engine sample-rate increment", got)
	}
}

func TestMOD_Replayer_8Channel_MixesIntoFour(t *testing.T) {
	engine, chip := newTestMODEngine(t)
	notes := make([]MODNote, 8)
	for i := range notes {
		notes[i] = MODNote{SampleNum: 1, Period: 428}
	}
	mod, err := ParseMOD(buildMinimalMODN(8, "8CHN", []int8{96, 96, 96, 96}, notes))
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	engine.LoadMOD(mod)
	engine.SetPlaying(true)
	for range 20 {
		engine.TickSample()
	}
	for i := range modChannels {
		if got := chip.channels[i].generateSample(); got == 0 {
			t.Fatalf("chip channel %d silent after 8-channel mix", i)
		}
	}
}

func TestMOD_4Channel_GoldenHashUnchanged(t *testing.T) {
	got := modGoldenHash(t)
	const want uint32 = 0xf6a133f6
	if got != want {
		t.Fatalf("golden hash=0x%08x, want 0x%08x", got, want)
	}
}

func modGoldenHash(t *testing.T) uint32 {
	t.Helper()
	engine, chip := newTestMODEngine(t)
	sampleData := []int8{-80, -40, 0, 40, 80, 40, 0, -40}
	notes := []MODNote{
		{SampleNum: 1, Period: 428},
		{SampleNum: 1, Period: 381},
	}
	mod, err := ParseMOD(buildMinimalMOD(sampleData, notes))
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	buf := make([]byte, 0, 4096*modChannels*2)
	var tmp [2]byte
	for range 4096 {
		engine.TickSample()
		for ch := range modChannels {
			v := int(math.Round(float64(chip.channels[ch].dacValue) * 32767.0))
			if v < -32768 {
				v = -32768
			} else if v > 32767 {
				v = 32767
			}
			binary.LittleEndian.PutUint16(tmp[:], uint16(int16(v)))
			buf = append(buf, tmp[:]...)
		}
	}
	return crc32.ChecksumIEEE(buf)
}

func TestMODEngineVolumeScaling(t *testing.T) {
	engine, chip := newTestMODEngine(t)

	// Create a sample with value 64
	sampleData := make([]int8, 64)
	for i := range sampleData {
		sampleData[i] = 64
	}

	// Build MOD with volume-setting effect (Cxx = set volume)
	notes := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0xC, EffParam: 32}, // volume = 32
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	// Tick to produce output
	for range 10 {
		engine.TickSample()
	}

	ch := chip.channels[0]
	// sampleByte=64, volume=32 → scaled = 64*32/64 = 32
	// DAC normalizes: int8(32) / 128.0 = 0.25
	// With chip volume=1.0: output ≈ 0.25
	sample := ch.generateSample()
	if math.Abs(float64(sample)-0.25) > 0.05 {
		t.Errorf("expected sample ≈ 0.25, got %f", sample)
	}
}

func TestMODEngineSampleLoop(t *testing.T) {
	engine, _ := newTestMODEngine(t)

	// Create a looping sample
	sampleData := make([]int8, 64)
	for i := range sampleData {
		sampleData[i] = int8(i)
	}
	mod := buildTestMODWithSample(sampleData)
	// Set loop: start=8, length=16 (loops bytes 8-23)
	mod.Samples[0].LoopStart = 8
	mod.Samples[0].LoopLength = 16

	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	// Tick many samples — should not crash or stop (looping)
	for range 50000 {
		engine.TickSample()
	}

	if !engine.IsPlaying() {
		t.Error("expected engine to still be playing (looping sample)")
	}
}

func TestMODEngineFilterA500(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	engine.SetFilterModel(1) // A500

	// Verify RC alpha for A500 (fc ≈ 4500 Hz)
	// alpha = 2π*4500/44100 / (1 + 2π*4500/44100)
	w := 2.0 * math.Pi * 4500.0 / float64(SAMPLE_RATE)
	expected := float32(w / (1.0 + w))

	if math.Abs(float64(engine.rcAlpha-expected)) > 0.001 {
		t.Errorf("expected A500 rcAlpha ≈ %f, got %f", expected, engine.rcAlpha)
	}

	// Feed impulse through filter — output should be attenuated
	f := &engine.filters[0]
	out1 := engine.applyAmigaFilter(0, 1.0)
	_ = f
	out2 := engine.applyAmigaFilter(0, 0.0)

	// First output should be less than 1.0 (filtered), second should be positive (residual)
	if out1 >= 1.0 {
		t.Errorf("A500 filter should attenuate impulse, got %f", out1)
	}
	if out2 <= 0 {
		t.Errorf("A500 filter should have residual after impulse, got %f", out2)
	}
}

func TestMODEngineFilterA1200(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	engine.SetFilterModel(2) // A1200

	// A1200 should be nearly transparent (fc ≈ 28kHz, well above audible range at 44.1kHz)
	out := engine.applyAmigaFilter(0, 1.0)
	// Should pass most of the signal through
	if out < 0.7 {
		t.Errorf("A1200 filter should be nearly transparent, got %f", out)
	}
}

func TestMODEngineFilterLED(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	engine.SetFilterModel(1) // Need a model to compute LED coefficients
	engine.ledFilter = true

	// Feed a series of samples through — LED filter should smooth
	var last float32
	for i := range 100 {
		// Alternating signal (high frequency)
		var in float32
		if i%2 == 0 {
			in = 1.0
		} else {
			in = -1.0
		}
		last = engine.applyAmigaFilter(0, in)
	}

	// High-frequency alternating signal should be heavily attenuated by 3.3kHz filter
	if math.Abs(float64(last)) > 0.3 {
		t.Errorf("LED filter should attenuate high frequencies, got %f", last)
	}
}

func TestMODEngineSilenceOnStop(t *testing.T) {
	engine, chip := newTestMODEngine(t)

	sampleData := make([]int8, 64)
	for i := range sampleData {
		sampleData[i] = 100
	}
	mod := buildTestMODWithSample(sampleData)
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	// Tick to produce output
	for range 10 {
		engine.TickSample()
	}

	// Stop playback
	engine.SetPlaying(false)

	// All DAC values should be 0 (silence)
	for i := range modChannels {
		ch := chip.channels[i]
		sample := ch.generateSample()
		if sample != 0 {
			t.Errorf("channel %d: expected silence after stop, got %f", i, sample)
		}
	}
}

func TestMODEngineSongEnd(t *testing.T) {
	engine, _ := newTestMODEngine(t)

	sampleData := make([]int8, 64)
	mod := buildTestMODWithSample(sampleData)
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	// Tick through the entire song (1 pattern = 64 rows * 6 ticks/row * 882 samples/tick)
	totalSamples := 3 * 64 * modDefaultSpeed * SamplesPerTick(SAMPLE_RATE, modDefaultBPM)
	for range totalSamples + 1000 {
		engine.TickSample()
	}

	if engine.IsPlaying() {
		t.Error("expected playback to stop at song end")
	}
}

func TestMOD_RestartPos_LoopsToRestartNotZero(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	mod := buildTestMODWithSample(make([]int8, 64))
	mod.SongLength = 3
	mod.RestartPos = 2
	engine.LoadMOD(mod)
	engine.SetLoop(true)
	engine.SetPlaying(true)
	totalSamples := 3 * 64 * modDefaultSpeed * SamplesPerTick(SAMPLE_RATE, modDefaultBPM)
	for range totalSamples + 1000 {
		engine.TickSample()
	}
	if got := engine.GetPosition(); got != 2 {
		t.Fatalf("loop restart position=%d, want 2", got)
	}
}

func TestMOD_FractionalSamplesPerTick_LongRunDriftUnder0_1Percent(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	mod := buildTestMODWithSample(make([]int8, 64))
	engine.LoadMOD(mod)
	engine.replayer.bpm = 129
	engine.samplesPerTickQ = samplesPerTickQ(engine.sampleRate, 129)
	engine.SetLoop(true)
	engine.SetPlaying(true)
	ticks := 0
	for range SAMPLE_RATE * 60 {
		before := engine.replayer.tick
		engine.TickSample()
		if engine.replayer.tick != before {
			ticks++
		}
	}
	expected := float64(60*129*2) / 5.0
	if math.Abs(float64(ticks)-expected) > expected*0.001+1 {
		t.Fatalf("ticks=%d expected %.2f", ticks, expected)
	}
}

func TestMOD_BPMChange_PreservesAccumulatorPhase(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	data := buildMinimalMOD(make([]int8, 64), []MODNote{{Effect: 0xF, EffParam: 150}})
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	oldQ := engine.samplesPerTickQ
	const remainingSamples = 17
	const halfSampleQ = uint64(1 << 31)
	engine.tickAccumQ = oldQ - remainingSamples*(1<<32) + halfSampleQ
	for range remainingSamples {
		engine.TickSample()
	}
	if engine.replayer.bpm != 150 {
		t.Fatalf("BPM=%d, want 150", engine.replayer.bpm)
	}
	if engine.tickAccumQ == 0 {
		t.Fatal("BPM change reset tick accumulator phase")
	}
	if engine.tickAccumQ != halfSampleQ {
		t.Fatalf("tickAccumQ=%d, want preserved remainder %d", engine.tickAccumQ, halfSampleQ)
	}
	if engine.samplesPerTickQ != samplesPerTickQ(engine.sampleRate, 150) {
		t.Fatalf("samplesPerTickQ not recomputed for BPM 150")
	}
}

func TestMOD_TickSample_NoLockContention(t *testing.T) {
	engine, _ := newTestMODEngine(t)
	mod := buildTestMODWithSample(make([]int8, 64))
	engine.LoadMOD(mod)
	engine.SetPlaying(true)

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := range 200 {
				switch (worker + j) % 4 {
				case 0:
					engine.SetPlaying(j%2 == 0)
				case 1:
					engine.SetLoop(j%2 == 0)
				case 2:
					engine.SetFilterModel(j % 3)
				case 3:
					engine.LoadMOD(mod)
				}
			}
		}(i)
	}
	for range 2000 {
		engine.TickSample()
	}
	wg.Wait()
}
