//go:build headless

package main

import (
	"encoding/binary"
	"testing"
)

// buildMinimalMOD creates a minimal valid 4-channel ProTracker MOD file.
// It has 1 sample, 1 pattern with the specified notes in row 0 channel 0.
func buildMinimalMOD(sampleData []int8, notes []MODNote) []byte {
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
	buf = append(buf, []byte("M.K.")...)

	// Pattern data: 1 pattern = 64 rows * 4 channels * 4 bytes = 1024 bytes
	patData := make([]byte, modPatternBytes)
	// Encode notes into row 0
	for ch, note := range notes {
		if ch >= modChannels {
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
