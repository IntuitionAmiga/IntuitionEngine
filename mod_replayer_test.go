//go:build headless

package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"
)

// buildMinimalMOD creates a minimal valid 4-channel ProTracker MOD file.
// It has 1 sample, 1 pattern with the specified notes in row 0 channel 0.
func buildMinimalMOD(sampleData []int8, notes []MODNote) []byte {
	return buildMinimalMODN(modChannels, "M.K.", sampleData, notes)
}

func buildMinimalMODN(channels int, formatID string, sampleData []int8, notes []MODNote) []byte {
	buf := make([]byte, 0, 2048)

	// Song name (20 bytes)
	name := make([]byte, modSongNameLen)
	copy(name, "TestMOD")
	buf = append(buf, name...)

	// 31 sample descriptors (30 bytes each)
	for i := range modNumSamples {
		desc := make([]byte, modSampleDescLen)
		if i == 0 && len(sampleData) > 0 {
			// Sample 1: set length, volume=64
			binary.BigEndian.PutUint16(desc[22:24], uint16(len(sampleData)/2)) // length in words
			desc[24] = 0                                                       // finetune
			desc[25] = 64                                                      // volume
			binary.BigEndian.PutUint16(desc[26:28], 0)                         // loop start
			binary.BigEndian.PutUint16(desc[28:30], 1)                         // loop length=1 word (no loop)
		}
		buf = append(buf, desc...)
	}

	// Song length, restart pos
	buf = append(buf, 1, 0) // 1 pattern in sequence

	// Pattern table (128 bytes)
	patTable := make([]byte, modPatternTableLen)
	patTable[0] = 0 // pattern 0
	buf = append(buf, patTable...)

	// Format ID
	buf = append(buf, []byte(formatID)...)

	patData := make([]byte, modRowsPerPattern*channels*modNoteBytesPerCh)
	// Encode notes into row 0
	for ch, note := range notes {
		if ch >= channels {
			break
		}
		off := ch * modNoteBytesPerCh
		patData[off+0] = (note.SampleNum & 0xF0) | byte((note.Period>>8)&0x0F)
		patData[off+1] = byte(note.Period & 0xFF)
		patData[off+2] = ((note.SampleNum & 0x0F) << 4) | (note.Effect & 0x0F)
		patData[off+3] = note.EffParam
	}
	buf = append(buf, patData...)

	// Sample data (signed 8-bit)
	for _, s := range sampleData {
		buf = append(buf, byte(s))
	}

	return buf
}

func TestMODParseHeader(t *testing.T) {
	data := buildMinimalMOD([]int8{0, 1, 2, 3}, nil)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	if mod.SongName != "TestMOD" {
		t.Errorf("expected song name 'TestMOD', got %q", mod.SongName)
	}
	if mod.NumChannels != 4 {
		t.Errorf("expected 4 channels, got %d", mod.NumChannels)
	}
	if mod.SongLength != 1 {
		t.Errorf("expected song length 1, got %d", mod.SongLength)
	}
	if mod.FormatID != "M.K." {
		t.Errorf("expected format ID 'M.K.', got %q", mod.FormatID)
	}
}

func TestMODParseSamples(t *testing.T) {
	sampleData := make([]int8, 256)
	for i := range sampleData {
		sampleData[i] = int8(i)
	}
	data := buildMinimalMOD(sampleData, nil)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}

	s := &mod.Samples[0]
	if s.Length != 256 {
		t.Errorf("expected sample length 256, got %d", s.Length)
	}
	if s.Volume != 64 {
		t.Errorf("expected sample volume 64, got %d", s.Volume)
	}
	if s.Finetune != 0 {
		t.Errorf("expected finetune 0, got %d", s.Finetune)
	}
	if s.LoopLength != 2 { // 1 word = 2 bytes
		t.Errorf("expected loop length 2, got %d", s.LoopLength)
	}
}

func TestMODParsePatterns(t *testing.T) {
	notes := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0, EffParam: 0},
	}
	data := buildMinimalMOD([]int8{0, 0, 0, 0}, notes)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}

	if mod.NumPatterns != 1 {
		t.Fatalf("expected 1 pattern, got %d", mod.NumPatterns)
	}
	note := mod.Patterns[0].Notes[0][0]
	if note.SampleNum != 1 {
		t.Errorf("expected sample num 1, got %d", note.SampleNum)
	}
	if note.Period != 428 {
		t.Errorf("expected period 428, got %d", note.Period)
	}
}

func TestMODParseSampleData(t *testing.T) {
	sampleData := []int8{-128, -64, 0, 64, 127}
	// Pad to even length (MOD samples must be even-length in bytes since length is in words)
	sampleData = append(sampleData, 0)
	data := buildMinimalMOD(sampleData, nil)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	s := &mod.Samples[0]
	if len(s.Data) != 6 {
		t.Fatalf("expected 6 sample bytes, got %d", len(s.Data))
	}
	if s.Data[0] != -128 {
		t.Errorf("expected first sample byte -128, got %d", s.Data[0])
	}
	if s.Data[4] != 127 {
		t.Errorf("expected 5th sample byte 127, got %d", s.Data[4])
	}
}

func TestMODParseInvalid(t *testing.T) {
	// Too short
	_, err := ParseMOD([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short data")
	}

	// Valid header length but bad format ID
	data := buildMinimalMOD(nil, nil)
	// Corrupt format ID
	copy(data[modHeaderLen-4:], "XXXX")
	_, err = ParseMOD(data)
	if err == nil {
		t.Error("expected error for bad format ID")
	}
}

func TestMODFormatDetection(t *testing.T) {
	formats := []string{"M.K.", "4CHN", "FLT4", "M!K!", "4CH\x00"}
	for _, fmt := range formats {
		data := buildMinimalMOD(nil, nil)
		copy(data[modHeaderLen-4:], fmt)
		mod, err := ParseMOD(data)
		if err != nil {
			t.Errorf("format %q should be valid, got error: %v", fmt, err)
			continue
		}
		if mod.NumChannels != 4 {
			t.Errorf("format %q: expected 4 channels, got %d", fmt, mod.NumChannels)
		}
	}
}

func TestMODTickTiming(t *testing.T) {
	spt := SamplesPerTick(44100, modDefaultBPM)
	expected := 44100 * 5 / (125 * 2) // = 882
	if spt != expected {
		t.Errorf("expected samplesPerTick=%d, got %d", expected, spt)
	}
}

func TestMODEffectSetSpeed(t *testing.T) {
	// Effect Fxx with x<32 sets speed, x>=32 sets BPM
	notes := []MODNote{
		{Effect: 0xF, EffParam: 3}, // speed = 3
	}
	sampleData := make([]int8, 4)
	data := buildMinimalMOD(sampleData, notes)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}

	r := NewMODReplayer(mod)
	r.ProcessTick() // tick 0, processes row with Fxx effect

	if r.speed != 3 {
		t.Errorf("expected speed=3, got %d", r.speed)
	}

	// Test BPM change (>= 32)
	notes2 := []MODNote{
		{Effect: 0xF, EffParam: 150}, // BPM = 150
	}
	data2 := buildMinimalMOD(sampleData, notes2)
	mod2, _ := ParseMOD(data2)
	r2 := NewMODReplayer(mod2)
	r2.ProcessTick()
	if r2.bpm != 150 {
		t.Errorf("expected bpm=150, got %d", r2.bpm)
	}
}

func TestMODEffectVolumeSlide(t *testing.T) {
	// Effect Axy: slide volume up by x, down by y
	sampleData := make([]int8, 64)
	notes := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0xA, EffParam: 0x10}, // slide up by 1
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)

	// Tick 0: sets sample volume to 64, note triggers
	r.ProcessTick()
	initialVol := r.channels[0].volume

	// Tick 1: volume slide up by 1 (but already at 64, should stay at 64)
	r.ProcessTick()
	if r.channels[0].volume < initialVol {
		t.Error("volume should not decrease on slide up")
	}

	// Test slide down
	notes2 := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0xA, EffParam: 0x02}, // slide down by 2
	}
	data2 := buildMinimalMOD(sampleData, notes2)
	mod2, _ := ParseMOD(data2)
	r2 := NewMODReplayer(mod2)
	r2.ProcessTick() // tick 0
	r2.ProcessTick() // tick 1: volume slide down
	if r2.channels[0].volume != 62 {
		t.Errorf("expected volume=62, got %d", r2.channels[0].volume)
	}
}

func TestMODEffectPortamento(t *testing.T) {
	sampleData := make([]int8, 64)

	// Effect 1xx: portamento up
	notes := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0x1, EffParam: 2},
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick() // tick 0
	r.ProcessTick() // tick 1: period -= 2
	if r.channels[0].period != 426 {
		t.Errorf("expected period=426, got %d", r.channels[0].period)
	}

	// Effect 2xx: portamento down
	notes2 := []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0x2, EffParam: 3},
	}
	data2 := buildMinimalMOD(sampleData, notes2)
	mod2, _ := ParseMOD(data2)
	r2 := NewMODReplayer(mod2)
	r2.ProcessTick() // tick 0
	r2.ProcessTick() // tick 1: period += 3
	if r2.channels[0].period != 431 {
		t.Errorf("expected period=431, got %d", r2.channels[0].period)
	}
}

func TestMODEffectPatternBreak(t *testing.T) {
	sampleData := make([]int8, 64)
	notes := []MODNote{
		{Effect: 0xD, EffParam: 0x10}, // Break to row 10 (BCD: 0x10 = 10)
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	// Extend song so position 1 is valid
	mod.SongLength = 2
	r := NewMODReplayer(mod)

	// Advance through all ticks in the row (default speed=6)
	for range modDefaultSpeed {
		r.ProcessTick()
	}

	// Position should have advanced to 1, row should be the break target (10)
	if r.position != 1 {
		t.Errorf("expected position=1 after pattern break, got %d", r.position)
	}
	if r.row != 10 {
		t.Errorf("expected row=10 after pattern break, got %d", r.row)
	}
}

func TestMODEffectPositionJump(t *testing.T) {
	sampleData := make([]int8, 64)
	notes := []MODNote{
		{Effect: 0xB, EffParam: 0}, // Jump to position 0
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	// Make song longer so position 0 is valid
	mod.SongLength = 2
	r := NewMODReplayer(mod)
	r.ProcessTick() // tick 0: position jump

	if r.position != 0 {
		t.Errorf("expected position=0 after jump, got %d", r.position)
	}
}

func TestMODEffectFilter(t *testing.T) {
	sampleData := make([]int8, 64)
	notes := []MODNote{
		{Effect: 0xE, EffParam: 0x00}, // E00: filter ON
	}
	data := buildMinimalMOD(sampleData, notes)
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick()
	if !r.ledFilter {
		t.Error("expected LED filter ON (E00)")
	}

	notes2 := []MODNote{
		{Effect: 0xE, EffParam: 0x01}, // E01: filter OFF
	}
	data2 := buildMinimalMOD(sampleData, notes2)
	mod2, _ := ParseMOD(data2)
	r2 := NewMODReplayer(mod2)
	r2.ProcessTick()
	if r2.ledFilter {
		t.Error("expected LED filter OFF (E01)")
	}
}

func TestMODSampleLoop(t *testing.T) {
	// Create a sample with loop
	sampleData := make([]int8, 16)
	for i := range sampleData {
		sampleData[i] = int8(i * 8)
	}

	data := buildMinimalMOD(sampleData, nil)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}

	// Set up loop: start at 4, length 8 (loop bytes 4-11)
	mod.Samples[0].LoopStart = 4
	mod.Samples[0].LoopLength = 8

	mc := &MODChannel{
		sample:  &mod.Samples[0],
		period:  428,
		volume:  64,
		phase:   0,
		looping: true,
		active:  true,
	}
	mc.updatePhaseInc(44100)

	// Read enough samples to pass the loop end
	var lastVal int8
	for range 100000 {
		val, active := mc.ReadSample()
		if !active {
			t.Fatal("channel should remain active with loop")
		}
		lastVal = val
	}
	// Just verify it stayed active (didn't crash/hang)
	_ = lastVal
}

func TestMODSampleOneShot(t *testing.T) {
	sampleData := make([]int8, 8)
	for i := range sampleData {
		sampleData[i] = int8(i * 16)
	}

	data := buildMinimalMOD(sampleData, nil)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}

	mc := &MODChannel{
		sample:  &mod.Samples[0],
		period:  428,
		volume:  64,
		phase:   0,
		looping: false,
		active:  true,
	}
	mc.updatePhaseInc(44100)

	// Read until sample ends
	active := true
	for i := range 100000 {
		_, a := mc.ReadSample()
		if !a {
			active = false
			break
		}
		_ = i
	}
	if active {
		t.Error("one-shot sample should stop after reaching end")
	}
}

func TestMOD_EEx_PatternDelay_DoesNotInfiniteLoop(t *testing.T) {
	notes := []MODNote{{Effect: 0xE, EffParam: 0xE3}}
	data := buildMinimalMOD(make([]int8, 4), notes)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	r := NewMODReplayer(mod)
	r.speed = 1

	for range r.speed*(3+1) + 2 {
		r.ProcessTick()
		if r.row > 0 || r.position > 0 || r.songEnd {
			return
		}
	}
	t.Fatalf("pattern delay did not advance past row 0: row=%d pos=%d delay=%d", r.row, r.position, r.patternDelay)
}

func TestMOD_Dxy_PatternBreakRowClamped(t *testing.T) {
	notes := []MODNote{{Effect: 0xD, EffParam: 0xFF}}
	data := buildMinimalMOD(make([]int8, 4), notes)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	mod.SongLength = 2
	r := NewMODReplayer(mod)
	for range modDefaultSpeed {
		r.ProcessTick()
	}
	if r.position != 1 || r.row != 0 {
		t.Fatalf("pattern break should clamp to row 0 at next position, got pos=%d row=%d", r.position, r.row)
	}
}

func TestMOD_SampleLoop_LoopStartBeyondData_NoPanic(t *testing.T) {
	sampleData := make([]int8, 10)
	data := buildMinimalMOD(sampleData, nil)
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	mod.Samples[0].LoopStart = 200
	mod.Samples[0].LoopLength = 4
	mc := &MODChannel{
		sample:  &mod.Samples[0],
		period:  428,
		volume:  64,
		looping: true,
		active:  true,
	}
	mc.updatePhaseInc(SAMPLE_RATE)
	val, active := mc.ReadSample()
	if val != 0 || active || mc.active {
		t.Fatalf("invalid loop should deactivate channel and return silence, got val=%d active=%v mc.active=%v", val, active, mc.active)
	}
}

func TestMOD_SongLengthClampedTo128(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 4), nil)
	data[modSongNameLen+modSampleDescLen*modNumSamples] = 200
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	if mod.SongLength != modPatternTableLen {
		t.Fatalf("SongLength=%d, want %d", mod.SongLength, modPatternTableLen)
	}
}

func TestMOD_ParseFuzz_NoPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(0x4d4f44))
	for i := 0; i < 10000; i++ {
		size := rng.Intn(modHeaderLen + modPatternBytes + 512)
		data := make([]byte, size)
		if _, err := rng.Read(data); err != nil {
			t.Fatal(err)
		}
		if size >= modHeaderLen && i%4 == 0 {
			copy(data[modHeaderLen-4:], "M.K.")
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseMOD panic on fuzz case %d size %d: %v", i, size, r)
				}
			}()
			_, _ = ParseMOD(data)
		}()
	}
}

func TestMOD_FinetunedPeriod_C2_Finetune_Plus1_LowersPeriod(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428}})
	desc := data[modSongNameLen : modSongNameLen+modSampleDescLen]
	desc[24] = 1
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	r := NewMODReplayer(mod)
	r.ProcessTick()
	if got, want := r.channels[0].period, periodForNote(12, 1); got != want || got >= 428 {
		t.Fatalf("finetune +1 period=%d, want table period %d below 428", got, want)
	}
}

func TestMOD_PeriodTable_RoundTrip_AllTablePeriods(t *testing.T) {
	for ft := range modFinetunePeriods {
		signedFT := int8(ft)
		if signedFT >= 8 {
			signedFT -= 16
		}
		for noteIdx, period := range modFinetunePeriods[ft] {
			if got := findNoteIndex(period, signedFT); got != noteIdx {
				t.Fatalf("ft=%d period=%d: note index=%d want %d", signedFT, period, got, noteIdx)
			}
		}
	}
}

func TestMOD_FindNoteIndex_NearestMatch_NonTablePeriod(t *testing.T) {
	if got := findNoteIndex(430, 0); got != 12 {
		t.Fatalf("findNoteIndex(430,0)=%d, want C-2 index 12", got)
	}
}

func TestMOD_NoteDelay_EDx_TriggersOnTickX_AndDefersSampleVolume(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0xE, EffParam: 0xD2}})
	mod, err := ParseMOD(data)
	if err != nil {
		t.Fatalf("ParseMOD failed: %v", err)
	}
	mod.Samples[0].Volume = 40
	r := NewMODReplayer(mod)
	r.ProcessTick()
	mc := &r.channels[0]
	if mc.sample != nil || mc.volume != 0 || mc.active {
		t.Fatalf("EDx should defer sample/volume/active on tick 0, got sample=%v volume=%d active=%v", mc.sample != nil, mc.volume, mc.active)
	}
	r.ProcessTick()
	if mc.sample != nil || mc.active {
		t.Fatalf("EDx should not trigger before tick 2, got sample=%v active=%v", mc.sample != nil, mc.active)
	}
	r.ProcessTick()
	if mc.sample == nil || mc.period == 0 || mc.volume != 40 || !mc.active {
		t.Fatalf("EDx did not trigger on tick 2: sample=%v period=%d volume=%d active=%v", mc.sample != nil, mc.period, mc.volume, mc.active)
	}
}

func TestMOD_NoteCut_EC0_TickZero(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0xE, EffParam: 0xC0}})
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick()
	if got := r.channels[0].volume; got != 0 {
		t.Fatalf("EC0 volume=%d, want 0", got)
	}
}

func TestMOD_Vibrato_PreservesBasePeriod(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x4, EffParam: 0x47}})
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick()
	base := r.channels[0].basePeriod
	for range 16 {
		r.ProcessTick()
		if r.channels[0].basePeriod != base {
			t.Fatalf("basePeriod changed under vibrato: got %d want %d", r.channels[0].basePeriod, base)
		}
	}
}

func TestMOD_Arpeggio_RestoresBasePeriod_AndDirection(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x0, EffParam: 0x37}})
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick()
	base := r.channels[0].basePeriod
	r.ProcessTick()
	if got, want := r.channels[0].period, periodForNote(findNoteIndex(base, 0)+3, 0); got != want {
		t.Fatalf("tick1 arpeggio period=%d, want %d", got, want)
	}
	r.ProcessTick()
	if got, want := r.channels[0].period, periodForNote(findNoteIndex(base, 0)+7, 0); got != want {
		t.Fatalf("tick2 arpeggio period=%d, want %d", got, want)
	}
	r.ProcessTick()
	if got := r.channels[0].period; got != base {
		t.Fatalf("tick3 arpeggio period=%d, want base %d", got, base)
	}
}

func TestMOD_SampleLoop_NoDCStuck(t *testing.T) {
	s := &MODSample{Data: []int8{-64, 64, -64, 64, -64, 64, -64, 64}, LoopStart: 0, LoopLength: 8}
	mc := &MODChannel{sample: s, period: 428, volume: 64, looping: true, active: true, phaseInc: 1}
	sum := 0
	for range 1024 {
		v, active := mc.ReadSample()
		if !active {
			t.Fatal("loop should stay active")
		}
		sum += int(v)
	}
	mean := float64(sum) / 1024.0
	if math.Abs(mean) > 2 {
		t.Fatalf("loop mean=%f, want near 0", mean)
	}
}

func TestMOD_VolumeReclampedAfterE5x(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0xE, EffParam: 0x51}})
	mod, _ := ParseMOD(data)
	mod.Samples[0].Volume = 100
	r := NewMODReplayer(mod)
	r.ProcessTick()
	if got := r.channels[0].volume; got != 64 {
		t.Fatalf("volume=%d, want clamped 64", got)
	}
}

func TestMOD_EffectMemory_Vibrato(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x4, EffParam: 0xA8}})
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[1][0] = MODNote{Effect: 0x4, EffParam: 0}
	r := NewMODReplayer(mod)
	for range modDefaultSpeed {
		r.ProcessTick()
	}
	r.ProcessTick()
	if r.channels[0].vibratoSpeed != 0xA || r.channels[0].vibratoDepth != 0x8 {
		t.Fatalf("vibrato memory lost: speed=%x depth=%x", r.channels[0].vibratoSpeed, r.channels[0].vibratoDepth)
	}
}

func TestMOD_EffectMemory_VolumeSlide_Tremolo_Porta_SampleOffset(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 1024), []MODNote{{SampleNum: 1, Period: 428, Effect: 0xA, EffParam: 0x02}})
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[1][0] = MODNote{Effect: 0xA, EffParam: 0}
	mod.Patterns[0].Notes[2][0] = MODNote{Effect: 0x7, EffParam: 0x34}
	mod.Patterns[0].Notes[3][0] = MODNote{Effect: 0x7, EffParam: 0}
	mod.Patterns[0].Notes[4][0] = MODNote{Effect: 0x1, EffParam: 2}
	mod.Patterns[0].Notes[5][0] = MODNote{Effect: 0x1, EffParam: 0}
	mod.Patterns[0].Notes[6][0] = MODNote{Effect: 0x2, EffParam: 3}
	mod.Patterns[0].Notes[7][0] = MODNote{Effect: 0x2, EffParam: 0}
	mod.Patterns[0].Notes[8][0] = MODNote{SampleNum: 1, Period: 428, Effect: 0x9, EffParam: 1}
	mod.Patterns[0].Notes[9][0] = MODNote{SampleNum: 1, Period: 428, Effect: 0x9, EffParam: 0}
	r := NewMODReplayer(mod)

	for range modDefaultSpeed * 10 {
		r.ProcessTick()
	}
	mc := &r.channels[0]
	if mc.memVolSlide != 0x02 || mc.memTremolo != 0x34 || mc.memPortaUp != 2 || mc.memPortaDown != 3 || mc.memSampleOffset != 1 {
		t.Fatalf("effect memory not retained: vol=%x trem=%x up=%x down=%x off=%x", mc.memVolSlide, mc.memTremolo, mc.memPortaUp, mc.memPortaDown, mc.memSampleOffset)
	}
}

func TestMOD_9xx_OvershootSilence(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x9, EffParam: 1}})
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick()
	if r.channels[0].active {
		t.Fatal("9xx beyond sample length should deactivate channel")
	}
}

func TestMOD_SampleOffset_DoesNotAffectLaterNormalNote(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 1024), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x9, EffParam: 1}})
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[1][0] = MODNote{SampleNum: 1, Period: 428}
	r := NewMODReplayer(mod)

	r.ProcessTick()
	if got := r.channels[0].phase; got != 256 {
		t.Fatalf("9xx phase=%f, want 256", got)
	}
	for range modDefaultSpeed - 1 {
		r.ProcessTick()
	}
	r.ProcessTick()
	if got := r.channels[0].phase; got != 0 {
		t.Fatalf("normal note after 9xx started at phase=%f, want 0", got)
	}
}

func TestMOD_Tremolo_ModulatesVolume(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x7, EffParam: 0xA4}})
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	for range modDefaultSpeed {
		r.ProcessTick()
		v := r.channels[0].volume + r.channels[0].tremoloDelta
		if v < 0 || v > 64 {
			t.Fatalf("tremolo volume out of range: %d", v)
		}
	}
	if r.channels[0].tremoloDelta == 0 {
		t.Fatal("tremolo did not modulate")
	}
}

func TestMOD_TremoloDeltaClearsWhenTremoloStops(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0x7, EffParam: 0xA4}})
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[1][0] = MODNote{}
	r := NewMODReplayer(mod)
	for range modDefaultSpeed {
		r.ProcessTick()
	}
	if r.channels[0].tremoloDelta == 0 {
		t.Fatal("test setup did not produce tremolo delta")
	}
	r.ProcessTick()
	if got := r.channels[0].tremoloDelta; got != 0 {
		t.Fatalf("tremoloDelta=%d after normal row, want 0", got)
	}
}

func TestMOD_PatternLoop_E60_E63_Repeats3Times(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[0][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[0].Notes[2][0] = MODNote{Effect: 0xE, EffParam: 0x63}
	r := NewMODReplayer(mod)
	visits := 0
	for range modDefaultSpeed * 12 {
		if r.tick == 0 && r.row == 2 {
			visits++
		}
		r.ProcessTick()
	}
	if visits != 4 {
		t.Fatalf("row 2 visits=%d, want initial + 3 repeats", visits)
	}
}

func TestMOD_PatternLoop_DoesNotRearmAfterCompletion(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[0][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[0].Notes[2][0] = MODNote{Effect: 0xE, EffParam: 0x63}
	r := NewMODReplayer(mod)

	visits := 0
	for range modDefaultSpeed * 24 {
		if r.tick == 0 && r.row == 2 {
			visits++
		}
		r.ProcessTick()
	}
	if visits != 4 {
		t.Fatalf("row 2 visits=%d, want exactly initial + 3 repeats", visits)
	}
	if r.row <= 2 {
		t.Fatalf("pattern loop rearmed or failed to advance, row=%d", r.row)
	}
}

func TestMOD_PatternLoop_AllowsLaterLoopSection(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[0][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[0].Notes[2][0] = MODNote{Effect: 0xE, EffParam: 0x62}
	mod.Patterns[0].Notes[8][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[0].Notes[10][0] = MODNote{Effect: 0xE, EffParam: 0x62}
	r := NewMODReplayer(mod)

	firstVisits := 0
	secondVisits := 0
	for range modDefaultSpeed * 32 {
		if r.tick == 0 {
			switch r.row {
			case 2:
				firstVisits++
			case 10:
				secondVisits++
			}
		}
		r.ProcessTick()
	}
	if firstVisits != 3 {
		t.Fatalf("first loop endpoint visits=%d, want initial + 2 repeats", firstVisits)
	}
	if secondVisits != 3 {
		t.Fatalf("second loop endpoint visits=%d, want initial + 2 repeats", secondVisits)
	}
}

func TestMOD_PatternLoop_AllowsLaterUnmarkedEndpoint(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[0][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[0].Notes[2][0] = MODNote{Effect: 0xE, EffParam: 0x61}
	mod.Patterns[0].Notes[6][0] = MODNote{Effect: 0xE, EffParam: 0x61}
	r := NewMODReplayer(mod)

	firstVisits := 0
	secondVisits := 0
	for range modDefaultSpeed * 22 {
		if r.tick == 0 {
			switch r.row {
			case 2:
				firstVisits++
			case 6:
				secondVisits++
			}
		}
		r.ProcessTick()
	}
	if firstVisits < 2 {
		t.Fatalf("first unmarked loop endpoint visits=%d, want at least initial + repeat", firstVisits)
	}
	if secondVisits != 2 {
		t.Fatalf("later unmarked endpoint visits=%d, want initial + repeat", secondVisits)
	}
}

func TestMOD_PatternLoop_ReplaysSameRowInLaterPosition(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.SongLength = 2
	mod.PatternTable[0] = 0
	mod.PatternTable[1] = 0
	mod.Patterns[0].Notes[0][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[0].Notes[2][0] = MODNote{Effect: 0xE, EffParam: 0x61}
	r := NewMODReplayer(mod)

	visitsByPos := [2]int{}
	for range modDefaultSpeed * (modRowsPerPattern + 16) {
		if r.tick == 0 && r.row == 2 && r.position < len(visitsByPos) {
			visitsByPos[r.position]++
		}
		r.ProcessTick()
	}
	if visitsByPos[0] != 2 {
		t.Fatalf("position 0 row 2 visits=%d, want initial + repeat", visitsByPos[0])
	}
	if visitsByPos[1] != 2 {
		t.Fatalf("position 1 row 2 visits=%d, want initial + repeat", visitsByPos[1])
	}
}

func TestMOD_PatternLoop_UnmarkedEndpointAfterPatternChangeStartsAtRowZero(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.SongLength = 2
	mod.PatternTable[0] = 0
	mod.PatternTable[1] = 1
	mod.NumPatterns = 2
	mod.Patterns = append(mod.Patterns, MODPattern{Notes: make([][]MODNote, modRowsPerPattern)})
	for row := range modRowsPerPattern {
		mod.Patterns[1].Notes[row] = make([]MODNote, modChannels)
	}
	mod.Patterns[0].Notes[8][0] = MODNote{Effect: 0xE, EffParam: 0x60}
	mod.Patterns[1].Notes[2][0] = MODNote{Effect: 0xE, EffParam: 0x61}
	r := NewMODReplayer(mod)

	rowZeroVisitsAtPos1 := 0
	endpointVisitsAtPos1 := 0
	for range modDefaultSpeed * (modRowsPerPattern + 12) {
		if r.tick == 0 && r.position == 1 {
			switch r.row {
			case 0:
				rowZeroVisitsAtPos1++
			case 2:
				endpointVisitsAtPos1++
			}
		}
		r.ProcessTick()
	}
	if endpointVisitsAtPos1 != 2 {
		t.Fatalf("position 1 endpoint visits=%d, want initial + repeat", endpointVisitsAtPos1)
	}
	if rowZeroVisitsAtPos1 != 2 {
		t.Fatalf("position 1 row 0 visits=%d, want initial + loop repeat from row 0", rowZeroVisitsAtPos1)
	}
}

func TestMOD_PatternLoop_InitialUnmarkedRowZeroEndpointLoops(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), nil)
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[0][0] = MODNote{Effect: 0xE, EffParam: 0x61}
	r := NewMODReplayer(mod)

	visits := 0
	for range modDefaultSpeed * 4 {
		if r.tick == 0 && r.row == 0 {
			visits++
		}
		r.ProcessTick()
	}
	if visits != 2 {
		t.Fatalf("row 0 visits=%d, want initial + repeat", visits)
	}
}

func TestMOD_VibratoWaveform_E4x_Square(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{SampleNum: 1, Period: 428, Effect: 0xE, EffParam: 0x41}})
	mod, _ := ParseMOD(data)
	mod.Patterns[0].Notes[1][0] = MODNote{Effect: 0x4, EffParam: 0x04}
	r := NewMODReplayer(mod)
	for range modDefaultSpeed + 2 {
		r.ProcessTick()
	}
	if got, want := int(r.channels[0].period), int(r.channels[0].basePeriod)+16; got != want {
		t.Fatalf("square vibrato period=%d, want %d", got, want)
	}
}

func TestMOD_E5x_PerChannelFinetune_DoesNotMutateSharedSample(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{
		{SampleNum: 1, Period: 428, Effect: 0xE, EffParam: 0x51},
		{SampleNum: 1, Period: 428},
	})
	mod, _ := ParseMOD(data)
	r := NewMODReplayer(mod)
	r.ProcessTick()
	if r.mod.Samples[0].Finetune != 0 {
		t.Fatalf("shared sample finetune mutated to %d", r.mod.Samples[0].Finetune)
	}
	if r.channels[0].effectiveFinetune() != 1 || r.channels[1].effectiveFinetune() != 0 {
		t.Fatalf("per-channel finetune not isolated: ch0=%d ch1=%d", r.channels[0].effectiveFinetune(), r.channels[1].effectiveFinetune())
	}
}

func TestMOD_PositionJump_B00_GoesToPosition0(t *testing.T) {
	data := buildMinimalMOD(make([]int8, 16), []MODNote{{Effect: 0xB, EffParam: 0}})
	mod, _ := ParseMOD(data)
	mod.SongLength = 2
	r := NewMODReplayer(mod)
	r.position = 1
	for range modDefaultSpeed {
		r.ProcessTick()
	}
	if r.position != 0 || r.row != 0 {
		t.Fatalf("B00 moved to pos=%d row=%d, want 0/0", r.position, r.row)
	}
}

func TestMOD_FinePorta_E1F_E2F_Clamped(t *testing.T) {
	mc := &MODChannel{period: 113, basePeriod: 113}
	r := NewMODReplayer(&MODFile{})
	r.channels[0] = *mc
	r.processExtendedEffect(0, 0x1F)
	if r.channels[0].period != 113 {
		t.Fatalf("E1F period=%d, want clamp 113", r.channels[0].period)
	}
	r.channels[0].period = 856
	r.channels[0].basePeriod = 856
	r.processExtendedEffect(0, 0x2F)
	if r.channels[0].period != 856 {
		t.Fatalf("E2F period=%d, want clamp 856", r.channels[0].period)
	}
}

func TestMOD_Parse_6CHN_8CHN(t *testing.T) {
	for _, tc := range []struct {
		id       string
		channels int
	}{
		{"6CHN", 6},
		{"8CHN", 8},
	} {
		mod, err := ParseMOD(buildMinimalMODN(tc.channels, tc.id, nil, nil))
		if err != nil {
			t.Fatalf("ParseMOD(%s): %v", tc.id, err)
		}
		if mod.NumChannels != tc.channels {
			t.Fatalf("%s channels=%d, want %d", tc.id, mod.NumChannels, tc.channels)
		}
	}
}

func TestMOD_Parse_xxCH_FT2(t *testing.T) {
	for _, channels := range []int{16, 32} {
		id := string([]byte{byte('0' + channels/10), byte('0' + channels%10), 'C', 'H'})
		mod, err := ParseMOD(buildMinimalMODN(channels, id, nil, nil))
		if err != nil {
			t.Fatalf("ParseMOD(%s): %v", id, err)
		}
		if mod.NumChannels != channels {
			t.Fatalf("%s channels=%d, want %d", id, mod.NumChannels, channels)
		}
	}
}

func TestMOD_Parse_FLT8_OCTA_CD81_OKTA(t *testing.T) {
	for _, id := range []string{"FLT8", "OCTA", "CD81", "OKTA"} {
		mod, err := ParseMOD(buildMinimalMODN(8, id, nil, nil))
		if err != nil {
			t.Fatalf("ParseMOD(%s): %v", id, err)
		}
		if mod.NumChannels != 8 {
			t.Fatalf("%s channels=%d, want 8", id, mod.NumChannels)
		}
	}
}
