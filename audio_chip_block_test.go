package main

import (
	"math"
	"testing"
)

type blockProbe struct {
	chip       *SoundChip
	pos        int
	writeAt    int
	writeEvery int
	addr       uint32
	value      uint32
}

func (p *blockProbe) TickSample() {
	p.pos++
	if p.writeAt > 0 && p.pos == p.writeAt {
		p.chip.HandleRegisterWrite(p.addr, p.value)
	}
	if p.writeEvery > 0 && p.pos%p.writeEvery == 0 {
		p.chip.HandleRegisterWrite(p.addr, p.value)
	}
}

func (p *blockProbe) MixSample() float32 {
	return float32(p.pos) * 0.0001
}

type readSamplesBlockProbe struct {
	samples int
	blocks  int
}

func (p *readSamplesBlockProbe) TickSample() {
	p.samples++
}

func (p *readSamplesBlockProbe) TickBlock(samples int) {
	p.blocks++
	p.samples += samples
}

func (p *readSamplesBlockProbe) CanTickBlockForReadSamples() bool {
	return true
}

type unsafeBlockProbe struct {
	samples int
	blocks  int
}

func (p *unsafeBlockProbe) TickSample() {
	p.samples++
}

func (p *unsafeBlockProbe) TickBlock(samples int) {
	p.blocks++
	p.samples += samples
}

func configureBlockReadChip(chip *SoundChip) {
	chip.HandleRegisterWrite(AUDIO_CTRL, 1)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_FREQ, 440*256)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, 192)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_ATK, 0)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_DEC, 0)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_SUS, 255)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_REL, 0)
	chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_CTRL, 3)
}

func captureReadSampleLoop(chip *SoundChip, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = chip.ReadSample()
	}
	return out
}

func assertFloat32BitsEqual(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d len(want)=%d", len(got), len(want))
	}
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d: got %08x want %08x (%f vs %f)",
				i, math.Float32bits(got[i]), math.Float32bits(want[i]), got[i], want[i])
		}
	}
}

func TestReadSamplesBlockMatchesPerSample(t *testing.T) {
	for _, n := range []int{1, 7, 64, 128, 1000} {
		perSample := newTestSoundChip()
		block := newTestSoundChip()
		configureBlockReadChip(perSample)
		configureBlockReadChip(block)

		perProbe := &blockProbe{chip: perSample}
		blockProbe := &blockProbe{chip: block}
		perSample.RegisterSampleTicker("probe", perProbe)
		perSample.RegisterSampleMixer("probe", perProbe)
		block.RegisterSampleTicker("probe", blockProbe)
		block.RegisterSampleMixer("probe", blockProbe)

		want := captureReadSampleLoop(perSample, n)
		got := make([]float32, n)
		if read := block.ReadSamples(got); read != n {
			t.Fatalf("ReadSamples(%d) returned %d", n, read)
		}
		assertFloat32BitsEqual(t, got, want)
	}
}

func TestReadSamples_UsesSafeTickerBlockGraph(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	probe := &readSamplesBlockProbe{}
	chip.RegisterSampleTicker("safe-block", probe)

	got := make([]float32, audioBlockSegmentMax*2+3)
	chip.ReadSamples(got)

	if probe.samples != len(got) {
		t.Fatalf("ticker samples = %d, want %d", probe.samples, len(got))
	}
	if probe.blocks != 3 {
		t.Fatalf("TickBlock calls = %d, want 3", probe.blocks)
	}
}

func TestReadSamples_UnsafeBlockTickerFallsBackToSamples(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	probe := &unsafeBlockProbe{}
	chip.RegisterSampleTicker("unsafe-block", probe)

	got := make([]float32, 17)
	chip.ReadSamples(got)

	if probe.samples != len(got) {
		t.Fatalf("ticker samples = %d, want %d", probe.samples, len(got))
	}
	if probe.blocks != 0 {
		t.Fatalf("TickBlock calls = %d, want 0", probe.blocks)
	}
}

func TestReadSamples_TickerWriteFlushesPendingSegment(t *testing.T) {
	const samples = 96
	perSample := newTestSoundChip()
	block := newTestSoundChip()
	configureBlockReadChip(perSample)
	configureBlockReadChip(block)

	perProbe := &blockProbe{
		chip:    perSample,
		writeAt: 17,
		addr:    FLEX_CH0_BASE + FLEX_OFF_VOL,
		value:   64,
	}
	blockProbe := &blockProbe{
		chip:    block,
		writeAt: 17,
		addr:    FLEX_CH0_BASE + FLEX_OFF_VOL,
		value:   64,
	}
	perSample.RegisterSampleTicker("probe", perProbe)
	block.RegisterSampleTicker("probe", blockProbe)

	want := captureReadSampleLoop(perSample, samples)
	got := make([]float32, samples)
	block.ReadSamples(got)
	assertFloat32BitsEqual(t, got, want)
}

func TestReadSamples_SetterFromTickerFlushes(t *testing.T) {
	const samples = 96
	perSample := newTestSoundChip()
	block := newTestSoundChip()
	configureBlockReadChip(perSample)
	configureBlockReadChip(block)

	perProbe := &setterTicker{chip: perSample, at: 33}
	blockProbe := &setterTicker{chip: block, at: 33}
	perSample.RegisterSampleTicker("setter", perProbe)
	block.RegisterSampleTicker("setter", blockProbe)

	want := captureReadSampleLoop(perSample, samples)
	got := make([]float32, samples)
	block.ReadSamples(got)
	assertFloat32BitsEqual(t, got, want)
}

func TestReadSamples_EventAtSegmentBoundary(t *testing.T) {
	for _, writeAt := range []int{audioBlockSegmentMax, audioBlockSegmentMax + 1} {
		perSample := newTestSoundChip()
		block := newTestSoundChip()
		configureBlockReadChip(perSample)
		configureBlockReadChip(block)

		perProbe := &blockProbe{
			chip:    perSample,
			writeAt: writeAt,
			addr:    FLEX_CH0_BASE + FLEX_OFF_VOL,
			value:   96,
		}
		blockProbe := &blockProbe{
			chip:    block,
			writeAt: writeAt,
			addr:    FLEX_CH0_BASE + FLEX_OFF_VOL,
			value:   96,
		}
		perSample.RegisterSampleTicker("boundary", perProbe)
		block.RegisterSampleTicker("boundary", blockProbe)

		want := captureReadSampleLoop(perSample, audioBlockSegmentMax*2+3)
		got := make([]float32, len(want))
		block.ReadSamples(got)
		assertFloat32BitsEqual(t, got, want)
	}
}

func TestReadSamples_MixerCaptureOrdering(t *testing.T) {
	const samples = 96
	perSample := newTestSoundChip()
	block := newTestSoundChip()
	configureBlockReadChip(perSample)
	configureBlockReadChip(block)

	perProbe := &blockProbe{chip: perSample}
	blockProbe := &blockProbe{chip: block}
	perSample.RegisterSampleTicker("mixer-order", perProbe)
	perSample.RegisterSampleMixer("mixer-order", perProbe)
	block.RegisterSampleTicker("mixer-order", blockProbe)
	block.RegisterSampleMixer("mixer-order", blockProbe)

	want := captureReadSampleLoop(perSample, samples)
	got := make([]float32, samples)
	block.ReadSamples(got)
	assertFloat32BitsEqual(t, got, want)
}

func TestReadSamples_SFXMixerCaptureOrdering(t *testing.T) {
	const ptr = uint32(0x2400)
	perSample, perBus := newSFXTestRig(t)
	block, blockBus := newSFXTestRig(t)
	for i, sample := range []byte{127, 64, 32, 0} {
		perBus.memory[ptr+uint32(i)] = sample
		blockBus.memory[ptr+uint32(i)] = sample
	}
	triggerSFX(perBus, 0, ptr, 4, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)
	triggerSFX(blockBus, 0, ptr, 4, SAMPLE_RATE, 255, SFX_FORMAT_SIGNED8, 0)

	want := captureReadSampleLoop(perSample, 8)
	got := make([]float32, len(want))
	block.ReadSamples(got)
	assertFloat32BitsEqual(t, got, want)
}

type setterTicker struct {
	chip *SoundChip
	pos  int
	at   int
}

func (t *setterTicker) TickSample() {
	t.pos++
	if t.pos == t.at {
		t.chip.SetChannelFilter(0, 1, 0.45, 0.1)
	}
}

func TestReadSamples_SampleTapPerSample(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	got := make([]float32, 9)
	tapped := make([]float32, 0, len(got))
	chip.SetSampleTap(func(sample float32) {
		tapped = append(tapped, sample)
	})
	chip.ReadSamples(got)
	assertFloat32BitsEqual(t, tapped, got)
}

func TestReadSamples_FrozenReturnsZeros(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	chip.audioFrozen.Store(true)
	got := []float32{1, 2, 3}
	if read := chip.ReadSamples(got); read != len(got) {
		t.Fatalf("ReadSamples returned %d", read)
	}
	for i, sample := range got {
		if sample != 0 {
			t.Fatalf("sample %d = %f, want 0", i, sample)
		}
	}
}

func TestReadSamples_ConcurrentMMIOWrites(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 512; i++ {
			chip.HandleRegisterWrite(FLEX_CH0_BASE+FLEX_OFF_VOL, uint32(32+(i&0x7F)))
		}
	}()

	buf := make([]float32, 2048)
	if got := chip.ReadSamples(buf); got != len(buf) {
		t.Fatalf("ReadSamples returned %d, want %d", got, len(buf))
	}
	<-done
}

func TestReadSamples_ZeroLenAndNil(t *testing.T) {
	chip := newTestSoundChip()
	if got := chip.ReadSamples(nil); got != 0 {
		t.Fatalf("ReadSamples(nil) returned %d, want 0", got)
	}
	if got := chip.ReadSamples([]float32{}); got != 0 {
		t.Fatalf("ReadSamples(empty) returned %d, want 0", got)
	}
}

func TestReadSamples_ZeroAllocsSteadyState(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	buf := make([]float32, 256)
	chip.ReadSamples(buf)

	allocs := testing.AllocsPerRun(100, func() {
		if got := chip.ReadSamples(buf); got != len(buf) {
			t.Fatalf("ReadSamples returned %d, want %d", got, len(buf))
		}
	})
	if allocs != 0 {
		t.Fatalf("ReadSamples steady-state allocations = %.2f, want 0", allocs)
	}
}

func TestReadSample_StillWorks_Compat(t *testing.T) {
	perSample := newTestSoundChip()
	block := newTestSoundChip()
	configureBlockReadChip(perSample)
	configureBlockReadChip(block)

	want := perSample.ReadSample()
	var gotBuf [1]float32
	if got := block.ReadSamples(gotBuf[:]); got != 1 {
		t.Fatalf("ReadSamples(one) returned %d, want 1", got)
	}
	if math.Float32bits(gotBuf[0]) != math.Float32bits(want) {
		t.Fatalf("ReadSamples(one) = %08x, ReadSample = %08x", math.Float32bits(gotBuf[0]), math.Float32bits(want))
	}
}

func BenchmarkReadSamples_64Segment(b *testing.B) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	buf := make([]float32, audioBlockSegmentMax)
	b.SetBytes(int64(len(buf) * 4))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		chip.ReadSamples(buf)
	}
}

func BenchmarkReadSample_Loop64(b *testing.B) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	b.SetBytes(int64(audioBlockSegmentMax * 4))
	b.ResetTimer()
	for range b.N {
		for range audioBlockSegmentMax {
			_ = chip.ReadSample()
		}
	}
}

func BenchmarkReadSamples_TickerHeavy(b *testing.B) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	probe := &blockProbe{
		chip:       chip,
		writeEvery: 8,
		addr:       FLEX_CH0_BASE + FLEX_OFF_VOL,
		value:      128,
	}
	chip.RegisterSampleTicker("heavy", probe)
	buf := make([]float32, audioBlockSegmentMax)
	b.SetBytes(int64(len(buf) * 4))
	b.ResetTimer()
	for range b.N {
		chip.ReadSamples(buf)
	}
}
