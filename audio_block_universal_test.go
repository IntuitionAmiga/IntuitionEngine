// audio_block_universal_test.go - universal block audio differentials.
//
// The per-sample ReadSamples path is the oracle. Every engine that opts into
// the block graph must produce byte-identical output through it, for the same
// input timing, or the conversion is wrong. These tests render the same
// material twice through a whole SoundChip, once with the block graph enabled
// and once with it disabled, and compare the float bits.
//
// (c) 2024 - 2026 Zayn Otley. GPLv3 or later.

package main

import (
	"math"
	"math/rand"
	"testing"
)

// withBlockGraph runs fn with the block graph forced on or off, restoring the
// previous setting afterwards.
func withBlockGraph(enabled bool, fn func()) {
	saved := audioBlockGraphEnabled
	audioBlockGraphEnabled = enabled
	defer func() { audioBlockGraphEnabled = saved }()
	fn()
}

// renderBothWays builds a chip with setup, renders n samples with the block
// graph disabled and then again with it enabled, and returns both buffers plus
// the fallback count observed on the block run.
func renderBothWays(t *testing.T, n int, setup func(chip *SoundChip)) (perSample, block []float32, fallbacks uint64) {
	t.Helper()
	perSample = make([]float32, n)
	block = make([]float32, n)

	withBlockGraph(false, func() {
		chip := newTestSoundChip()
		setup(chip)
		if got := chip.ReadSamples(perSample); got != n {
			t.Fatalf("per-sample ReadSamples returned %d, want %d", got, n)
		}
	})
	withBlockGraph(true, func() {
		chip := newTestSoundChip()
		setup(chip)
		if got := chip.ReadSamples(block); got != n {
			t.Fatalf("block ReadSamples returned %d, want %d", got, n)
		}
		fallbacks = chip.perSampleFallbacks.Load()
	})
	return perSample, block, fallbacks
}

func assertRenderedBitIdentical(t *testing.T, name string, n int, setup func(chip *SoundChip)) uint64 {
	t.Helper()
	want, got, fallbacks := renderBothWays(t, n, setup)
	for i := range want {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("%s: sample %d differs between the block graph and the per-sample path: got %08x (%g) want %08x (%g)",
				name, i, math.Float32bits(got[i]), got[i], math.Float32bits(want[i]), want[i])
		}
	}
	return fallbacks
}

// ---------------------------------------------------------------------------
// Event material
// ---------------------------------------------------------------------------

// randomPSGEvents builds a schedule with the shape real PSG dumps have: bursts
// of register writes at frame boundaries, with long quiet gaps between them.
func randomPSGEvents(rng *rand.Rand, frames, framePeriod int) []PSGEvent {
	var events []PSGEvent
	for f := range frames {
		at := uint64(f * framePeriod)
		for range 1 + rng.Intn(4) {
			events = append(events, PSGEvent{
				Sample: at,
				Reg:    uint8(rng.Intn(PSG_REG_COUNT)),
				Value:  uint8(rng.Intn(256)),
			})
		}
	}
	return events
}

func randomSIDEvents(rng *rand.Rand, frames, framePeriod int) []SIDEvent {
	var events []SIDEvent
	for f := range frames {
		at := uint64(f * framePeriod)
		for range 1 + rng.Intn(4) {
			events = append(events, SIDEvent{
				Sample: at,
				Reg:    uint8(rng.Intn(SID_REG_COUNT)),
				Value:  uint8(rng.Intn(256)),
			})
		}
	}
	return events
}

func randomTEDEvents(rng *rand.Rand, frames, framePeriod int) []TEDEvent {
	var events []TEDEvent
	for f := range frames {
		at := uint64(f * framePeriod)
		for range 1 + rng.Intn(3) {
			events = append(events, TEDEvent{
				Sample: at,
				Reg:    uint8(rng.Intn(TED_REG_COUNT)),
				Value:  uint8(rng.Intn(256)),
			})
		}
	}
	return events
}

func randomPOKEYEvents(rng *rand.Rand, frames, framePeriod int) []SAPPOKEYEvent {
	var events []SAPPOKEYEvent
	for f := range frames {
		at := uint64(f * framePeriod)
		for range 1 + rng.Intn(3) {
			events = append(events, SAPPOKEYEvent{
				Sample: at,
				Reg:    uint8(rng.Intn(POKEY_REG_COUNT)),
				Value:  uint8(rng.Intn(256)),
			})
		}
	}
	return events
}

// ---------------------------------------------------------------------------
// Per-chip differentials
// ---------------------------------------------------------------------------

func TestReadSamplesBlock_BitIdenticalToPerSample_PSG(t *testing.T) {
	const n = 4000
	for seed := range 8 {
		rng := rand.New(rand.NewSource(int64(seed)))
		events := randomPSGEvents(rng, 8, 441)
		assertRenderedBitIdentical(t, "PSG", n, func(chip *SoundChip) {
			configureBlockReadChip(chip)
			engine := NewPSGEngine(chip, SAMPLE_RATE)
			engine.SetEvents(events, uint64(n), false, 0)
			engine.SetPlaying(true)
			// NewPSGEngine registers itself as the default ticker.
		})
	}
}

func TestReadSamplesBlock_BitIdenticalToPerSample_SID(t *testing.T) {
	const n = 4000
	for seed := range 8 {
		rng := rand.New(rand.NewSource(int64(seed)))
		events := randomSIDEvents(rng, 8, 441)
		assertRenderedBitIdentical(t, "SID", n, func(chip *SoundChip) {
			configureBlockReadChip(chip)
			engine := NewSIDEngine(chip, SAMPLE_RATE)
			engine.SetEvents(events, uint64(n), false, 0)
			engine.SetPlaying(true)
			chip.RegisterSampleTicker("sid", engine)
		})
	}
}

func TestReadSamplesBlock_BitIdenticalToPerSample_TED(t *testing.T) {
	const n = 4000
	for seed := range 8 {
		rng := rand.New(rand.NewSource(int64(seed)))
		events := randomTEDEvents(rng, 8, 441)
		assertRenderedBitIdentical(t, "TED", n, func(chip *SoundChip) {
			configureBlockReadChip(chip)
			engine := NewTEDEngine(chip, SAMPLE_RATE)
			engine.SetEvents(events, uint64(n), false, 0)
			engine.SetPlaying(true)
			chip.RegisterSampleTicker("ted", engine)
		})
	}
}

func TestReadSamplesBlock_BitIdenticalToPerSample_POKEY(t *testing.T) {
	const n = 4000
	for seed := range 8 {
		rng := rand.New(rand.NewSource(int64(seed)))
		events := randomPOKEYEvents(rng, 8, 441)
		assertRenderedBitIdentical(t, "POKEY", n, func(chip *SoundChip) {
			configureBlockReadChip(chip)
			engine := NewPOKEYEngine(chip, SAMPLE_RATE)
			engine.SetEvents(events, uint64(n), false, 0)
			engine.SetPlaying(true)
			chip.RegisterSampleTicker("pokey", engine)
		})
	}
}

// TestReadSamplesBlock_BitIdenticalToPerSample_LoopingSong covers the other
// place every event engine writes to the chip: the end of the song, where it
// either loops or silences its channels.
func TestReadSamplesBlock_BitIdenticalToPerSample_LoopingSong(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	events := randomPSGEvents(rng, 4, 300)
	for _, loop := range []bool{false, true} {
		assertRenderedBitIdentical(t, "PSG looping song", 4000, func(chip *SoundChip) {
			configureBlockReadChip(chip)
			engine := NewPSGEngine(chip, SAMPLE_RATE)
			engine.SetEvents(events, 1200, loop, 0)
			engine.SetPlaying(true)
			// NewPSGEngine registers itself as the default ticker.
		})
	}
}

// TestReadSamplesBlock_BitIdenticalToPerSample_MultipleEngines pins the case
// the quiet span exists for: several engines with unrelated write schedules
// registered at once, so the block length is the minimum of their spans.
func TestReadSamplesBlock_BitIdenticalToPerSample_MultipleEngines(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	psgEvents := randomPSGEvents(rng, 8, 441)
	tedEvents := randomTEDEvents(rng, 8, 317)
	sidEvents := randomSIDEvents(rng, 8, 523)
	assertRenderedBitIdentical(t, "PSG+TED+SID", 4000, func(chip *SoundChip) {
		configureBlockReadChip(chip)
		psg := NewPSGEngine(chip, SAMPLE_RATE)
		psg.SetEvents(psgEvents, 4000, false, 0)
		psg.SetPlaying(true)
		chip.RegisterSampleTicker("psg", psg)

		ted := NewTEDEngine(chip, SAMPLE_RATE)
		ted.SetEvents(tedEvents, 4000, false, 0)
		ted.SetPlaying(true)
		chip.RegisterSampleTicker("ted", ted)

		sid := NewSIDEngine(chip, SAMPLE_RATE)
		sid.SetEvents(sidEvents, 4000, false, 0)
		sid.SetPlaying(true)
		chip.RegisterSampleTicker("sid", sid)
	})
}

// ---------------------------------------------------------------------------
// Fallback accounting
// ---------------------------------------------------------------------------

// TestReadSamplesBlock_FallbackNeverEngagesInProduction asserts that the
// per-sample path is not reached at all once every registered ticker reports a
// quiet span, including engines whose span is always zero: those render as
// blocks of one sample rather than dropping the whole chip off the block graph.
func TestReadSamplesBlock_FallbackNeverEngagesInProduction(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	cases := []struct {
		name  string
		setup func(chip *SoundChip)
	}{
		{"PSG", func(chip *SoundChip) {
			e := NewPSGEngine(chip, SAMPLE_RATE)
			e.SetEvents(randomPSGEvents(rng, 4, 441), 2000, false, 0)
			e.SetPlaying(true)
			chip.RegisterSampleTicker("psg", e)
		}},
		{"SID", func(chip *SoundChip) {
			e := NewSIDEngine(chip, SAMPLE_RATE)
			e.SetEvents(randomSIDEvents(rng, 4, 441), 2000, false, 0)
			e.SetPlaying(true)
			chip.RegisterSampleTicker("sid", e)
		}},
		{"TED", func(chip *SoundChip) {
			e := NewTEDEngine(chip, SAMPLE_RATE)
			e.SetEvents(randomTEDEvents(rng, 4, 441), 2000, false, 0)
			e.SetPlaying(true)
			chip.RegisterSampleTicker("ted", e)
		}},
		{"POKEY", func(chip *SoundChip) {
			e := NewPOKEYEngine(chip, SAMPLE_RATE)
			e.SetEvents(randomPOKEYEvents(rng, 4, 441), 2000, false, 0)
			e.SetPlaying(true)
			chip.RegisterSampleTicker("pokey", e)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var fallbacks uint64
			withBlockGraph(true, func() {
				chip := newTestSoundChip()
				configureBlockReadChip(chip)
				tc.setup(chip)
				out := make([]float32, 2000)
				chip.ReadSamples(out)
				fallbacks = chip.perSampleFallbacks.Load()
			})
			if fallbacks != 0 {
				t.Fatalf("%s: the per-sample fallback engaged %d times; every registered ticker reports a quiet span, so the block graph should have covered the whole buffer", tc.name, fallbacks)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Quiet span arithmetic
// ---------------------------------------------------------------------------

func TestQuietSpanFromDelta(t *testing.T) {
	cases := []struct {
		now, next uint64
		want      int
	}{
		{0, 0, 0},
		{10, 5, 0},
		{0, 1, 1},
		{100, 441, 341},
		{0, math.MaxUint64, quietSpanUnbounded},
	}
	for _, tc := range cases {
		if got := quietSpanFromDelta(tc.now, tc.next); got != tc.want {
			t.Fatalf("quietSpanFromDelta(%d, %d) = %d, want %d", tc.now, tc.next, got, tc.want)
		}
	}
}

// TestPSGEnvelopeQuietSamples checks the span against the loop it stands in
// for: advancing the counter that many times must not reach the period, and
// one more must.
func TestPSGEnvelopeQuietSamples(t *testing.T) {
	for _, period := range []float64{1, 1.5, 2, 7.25, 100, 4410.7} {
		for _, counter := range []float64{0, 0.5, 1, period / 3, period - 1} {
			if counter < 0 {
				continue
			}
			span := psgEnvelopeQuietSamples(counter, period)
			c := counter
			for range span {
				c++
				if c >= period {
					t.Fatalf("period=%g counter=%g: span %d reaches the period early", period, counter, span)
				}
			}
			if c+1 < period && span < quietSpanUnbounded {
				t.Fatalf("period=%g counter=%g: span %d stops short of the step", period, counter, span)
			}
		}
	}
	if got := psgEnvelopeQuietSamples(0, 0); got != 0 {
		t.Fatalf("a zero period must not report a quiet span, got %d", got)
	}
}

func TestAHXTickQuietSamples(t *testing.T) {
	for _, sampleRate := range []int{44100, 1000} {
		for _, rate := range []int{50, 125, 1} {
			for _, acc := range []int{0, 1, sampleRate / 2, sampleRate - 1} {
				span := ahxTickQuietSamples(acc, rate, sampleRate)
				a := acc
				for range span {
					a += rate
					if a >= sampleRate {
						t.Fatalf("sr=%d rate=%d acc=%d: span %d reaches the tick early", sampleRate, rate, acc, span)
					}
				}
				if a+rate < sampleRate {
					t.Fatalf("sr=%d rate=%d acc=%d: span %d stops short of the tick", sampleRate, rate, acc, span)
				}
			}
		}
	}
	if got := ahxTickQuietSamples(0, 0, 44100); got != quietSpanUnbounded {
		t.Fatalf("a frozen accumulator below the threshold never ticks, got %d", got)
	}
	if got := ahxTickQuietSamples(44100, 0, 44100); got != 0 {
		t.Fatalf("a frozen accumulator at the threshold ticks every sample, got %d", got)
	}
}

// TestQuietSamples_MatchesNextEvent pins the reported span against the event
// schedules directly, so a future change to the event loops cannot silently
// widen it.
func TestQuietSamples_MatchesNextEvent(t *testing.T) {
	chip := newTestSoundChip()
	psg := NewPSGEngine(chip, SAMPLE_RATE)
	// The PSG envelope steps far more often than the events arrive, so its span
	// is the envelope's, not the event's. With the event moved inside the
	// envelope step the event becomes the bound instead.
	psg.SetEvents([]PSGEvent{{Sample: 40, Reg: 8, Value: 15}}, 1000, false, 0)
	psg.SetPlaying(true)
	envSpan := psgEnvelopeQuietSamples(psg.envSampleCounter, psg.envPeriodSamples)
	if envSpan == 0 || envSpan >= 40 {
		t.Fatalf("test premise: the envelope span %d must be inside the event gap", envSpan)
	}
	if got := psg.QuietSamples(); got != envSpan {
		t.Fatalf("PSG quiet span = %d, want the envelope span %d", got, envSpan)
	}
	psg.SetEvents([]PSGEvent{{Sample: 2, Reg: 8, Value: 15}}, 1000, false, 0)
	psg.SetPlaying(true)
	if got := psg.QuietSamples(); got != 2 {
		t.Fatalf("PSG quiet span = %d, want 2 (the event arrives before the envelope steps)", got)
	}

	ted := NewTEDEngine(chip, SAMPLE_RATE)
	ted.SetEvents([]TEDEvent{{Sample: 17, Reg: 0, Value: 1}}, 1000, false, 0)
	ted.SetPlaying(true)
	if got := ted.QuietSamples(); got != 17 {
		t.Fatalf("TED quiet span = %d, want 17", got)
	}

	pokey := NewPOKEYEngine(chip, SAMPLE_RATE)
	pokey.SetEvents([]SAPPOKEYEvent{{Sample: 0, Reg: 0, Value: 1}}, 1000, false, 0)
	pokey.SetPlaying(true)
	if got := pokey.QuietSamples(); got != 0 {
		t.Fatalf("POKEY quiet span = %d, want 0 (an event is due now)", got)
	}
}

// TestBlockGraphQuietSpan_TakesTheMinimum pins the chip-side rule: one ticker
// that must write now forces a one-sample block for everybody.
func TestBlockGraphQuietSpan_TakesTheMinimum(t *testing.T) {
	chip := newTestSoundChip()
	long := NewTEDEngine(chip, SAMPLE_RATE)
	long.SetEvents([]TEDEvent{{Sample: 500, Reg: 0, Value: 1}}, 1000, false, 0)
	long.SetPlaying(true)
	short := NewTEDEngine(chip, SAMPLE_RATE)
	short.SetEvents([]TEDEvent{{Sample: 3, Reg: 0, Value: 1}}, 1000, false, 0)
	short.SetPlaying(true)

	holder := &sampleTickerListHolder{tickers: []SampleTicker{long, short}}
	if got := chip.blockGraphQuietSpan(holder); got != 3 {
		t.Fatalf("block span = %d, want 3 (the shorter of the two)", got)
	}

	idle := &sampleTickerListHolder{tickers: []SampleTicker{long}}
	if got := chip.blockGraphQuietSpan(idle); got != 500 {
		t.Fatalf("block span = %d, want 500", got)
	}

	none := &sampleTickerListHolder{}
	if got := chip.blockGraphQuietSpan(none); got != quietSpanUnbounded {
		t.Fatalf("an empty ticker set must be quiet for any span, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Post-FX snapshot split
// ---------------------------------------------------------------------------

// TestSoundPostFXConfig_CapturedOncePerSegment pins the split: the
// block-invariant settings are captured once per flush, not once per sample.
func TestSoundPostFXConfig_CapturedOncePerSegment(t *testing.T) {
	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	out := make([]float32, 64)
	chip.postFXConfigCaptures.Store(0)
	withBlockGraph(true, func() {
		chip.ReadSamples(out)
	})
	captures := chip.postFXConfigCaptures.Load()
	if captures == 0 {
		t.Fatal("no post-FX configuration was captured at all")
	}
	if captures >= uint64(len(out)) {
		t.Fatalf("post-FX configuration captured %d times for %d samples; it should be captured once per flush, not once per sample", captures, len(out))
	}
}

// TestSoundPostFXSplit_BitIdentical renders with every post-FX stage active,
// including the smoothed and recurrent state the split must not hoist, and
// compares the split path against the per-sample snapshot it replaced.
func TestSoundPostFXSplit_BitIdentical(t *testing.T) {
	const n = 2048
	setup := func(chip *SoundChip) {
		configureBlockReadChip(chip)
		chip.HandleRegisterWrite(FILTER_TYPE, 1)
		chip.HandleRegisterWrite(FILTER_CUTOFF, 200)
		chip.HandleRegisterWrite(FILTER_RESONANCE, 180)
		chip.HandleRegisterWrite(FILTER_MOD_AMOUNT, 128)
		chip.HandleRegisterWrite(OVERDRIVE_CTRL, 150)
		chip.HandleRegisterWrite(REVERB_MIX, 128)
		chip.HandleRegisterWrite(REVERB_DECAY, 180)
	}

	// Oracle: rebuild the whole snapshot on every sample, as the code did
	// before the split.
	want := make([]float32, n)
	oracle := newTestSoundChip()
	setup(oracle)
	for i := range want {
		want[i] = oracle.generateSampleWithMixer(0)
	}

	got := make([]float32, n)
	chip := newTestSoundChip()
	setup(chip)
	cfgChip := chip
	withBlockGraph(true, func() {
		cfgChip.ReadSamples(got)
	})
	assertFloat32BitsEqual(t, got, want)
}

// ---------------------------------------------------------------------------
// Oscillator steady state
// ---------------------------------------------------------------------------

// TestOscillatorTick_ZeroAllocsSteadyState pins that generating samples from a
// configured channel allocates nothing, including the SID combined-waveform
// path where the generator is written as a set of local closures.
func TestOscillatorTick_ZeroAllocsSteadyState(t *testing.T) {
	cases := []struct {
		name     string
		waveMask uint8
		pwm      bool
	}{
		{"pulse", SID_WAVE_PULSE, false},
		{"pulse with PWM", SID_WAVE_PULSE, true},
		{"triangle", SID_WAVE_TRIANGLE, false},
		{"saw", SID_WAVE_SAW, false},
		{"noise", SID_WAVE_NOISE, false},
		{"pulse and saw combined", SID_WAVE_PULSE | SID_WAVE_SAW, false},
		{"triangle and saw combined", SID_WAVE_TRIANGLE | SID_WAVE_SAW, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chip := newTestSoundChip()
			ch := chip.channels[0]
			ch.enabled = true
			ch.frequency = 440
			ch.volume = 0.8
			ch.sidWaveMask = tc.waveMask
			ch.pwmEnabled = tc.pwm
			ch.pwmRate = 3
			ch.pwmDepth = 0.3
			allocs := testing.AllocsPerRun(200, func() {
				ch.generateWaveSample(float32(SAMPLE_RATE), 1.0/float32(SAMPLE_RATE))
			})
			if allocs != 0 {
				t.Fatalf("%s: %.1f allocations per sample, want none", tc.name, allocs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Span kernels
// ---------------------------------------------------------------------------

// TestAudioOutputPath_UsesSpanKernels pins that the production flush reaches
// the span clamp rather than clamping sample by sample, which is what makes the
// SIMD kernel worth wiring at all.
func TestAudioOutputPath_UsesSpanKernels(t *testing.T) {
	saved := clampF32SpanImpl
	defer func() { clampF32SpanImpl = saved }()

	var calls, samples int
	clampF32SpanImpl = func(s []float32, minValue, maxValue float32) {
		calls++
		samples += len(s)
		saved(s, minValue, maxValue)
	}

	chip := newTestSoundChip()
	configureBlockReadChip(chip)
	out := make([]float32, 512)
	withBlockGraph(true, func() {
		chip.ReadSamples(out)
	})

	if calls == 0 {
		t.Fatal("the output path never reached the span clamp")
	}
	if samples != len(out) {
		t.Fatalf("the span clamp covered %d samples, want all %d", samples, len(out))
	}
	if calls >= len(out) {
		t.Fatalf("the span clamp was called %d times for %d samples; it should run once per flush", calls, len(out))
	}
}

// ---------------------------------------------------------------------------
// Pipeline specialisation
// ---------------------------------------------------------------------------

// TestAudioPipelineSpecialisation_SilentChannelCannotBeSkipped records why the
// tranche's silent-channel specialisation is not implemented. The mixer divides
// the channel sum by the number of ENABLED channels, not by the number of
// audible ones, so an enabled channel that happens to be silent still changes
// every other channel's contribution. Skipping it is a semantic change, not an
// optimisation, and the behaviour contract forbids it.
//
// If a future change makes the divisor depend on audibility instead, this test
// fails and the specialisation becomes available.
func TestAudioPipelineSpecialisation_SilentChannelCannotBeSkipped(t *testing.T) {
	render := func(withSilentChannel bool) []float32 {
		chip := newTestSoundChip()
		configureBlockReadChip(chip)
		if withSilentChannel {
			// A second channel that is enabled and gated but produces nothing:
			// zero frequency, zero noise mix.
			chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_FREQ, 0)
			chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_VOL, 0)
			chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_SUS, 255)
			chip.HandleRegisterWrite(FLEX_CH1_BASE+FLEX_OFF_CTRL, 3)
		}
		out := make([]float32, 256)
		withBlockGraph(true, func() { chip.ReadSamples(out) })
		return out
	}

	without := render(false)
	with := render(true)

	differs := false
	for i := range without {
		if math.Float32bits(with[i]) != math.Float32bits(without[i]) {
			differs = true
			break
		}
	}
	if !differs {
		t.Fatal("an enabled but silent channel no longer affects the mix; the silent-channel specialisation is now bit-exact and should be implemented")
	}
}
