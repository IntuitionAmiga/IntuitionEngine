package main

import (
	"encoding/binary"
	"testing"
)

// buildVGMHeader creates a minimal VGM header with data starting at offset 0x80.
func buildVGMHeader(totalSamples uint32, ayClock uint32) []byte {
	header := make([]byte, 0x80)
	copy(header[0:4], []byte("Vgm "))
	binary.LittleEndian.PutUint32(header[0x08:0x0C], 0x00000172) // version 1.72
	binary.LittleEndian.PutUint32(header[0x18:0x1C], totalSamples)
	binary.LittleEndian.PutUint32(header[0x34:0x38], 0x4C) // data offset: 0x34+0x4C=0x80
	binary.LittleEndian.PutUint32(header[0x74:0x78], ayClock)
	return header
}

// buildVGMHeaderSN creates a VGM header with SN76489 clock set at offset 0x0C.
func buildVGMHeaderSN(totalSamples, snClock, ayClock uint32) []byte {
	header := buildVGMHeader(totalSamples, ayClock)
	binary.LittleEndian.PutUint32(header[0x0C:0x10], snClock)
	return header
}

func TestVGMParse_AYOnly(t *testing.T) {
	// Simple VGM with only AY writes - should already work.
	header := buildVGMHeader(735, 1773400)
	cmds := []byte{
		0xA0, 0x00, 0xFF, // AY reg 0 = 0xFF
		0xA0, 0x07, 0x3E, // AY reg 7 = 0x3E (enable tone A)
		0x62, // wait 735 samples
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	if len(vgm.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(vgm.Events))
	}
	if vgm.Events[0].Reg != 0x00 || vgm.Events[0].Value != 0xFF {
		t.Errorf("event 0: reg=%d val=%d", vgm.Events[0].Reg, vgm.Events[0].Value)
	}
	if vgm.ClockHz != 1773400 {
		t.Errorf("expected clock 1773400, got %d", vgm.ClockHz)
	}
}

func TestVGMParse_GracefulSkipUnknownCommands(t *testing.T) {
	// VGM with mixed AY writes and unknown chip commands.
	// Unknown commands must be silently skipped, not cause errors.
	header := buildVGMHeader(1470, 1773400)
	cmds := []byte{
		0xA0, 0x00, 0xFF, // AY write (kept)
		0x51, 0x10, 0x20, // YM2413 write (skip 2 operands)
		0xA0, 0x01, 0xAA, // AY write (kept)
		0x52, 0x30, 0x40, // YM2612 port 0 (skip 2 operands)
		0x55, 0x00, 0x01, // YM2203 (skip 2 operands)
		0x62,       // wait 735 samples
		0x30, 0x00, // reserved 1-operand (skip 1)
		0x3F, 0x00, // reserved 1-operand (skip 1)
		0xC0, 0x01, 0x02, 0x03, // Sega PCM (skip 3 operands)
		0xE0, 0x01, 0x02, 0x03, 0x04, // seek PCM (skip 4 operands)
		0xA0, 0x07, 0x3E, // AY write (kept)
		0x62, // wait 735 samples
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData should skip unknown commands, got error: %v", err)
	}
	if len(vgm.Events) != 3 {
		t.Fatalf("expected 3 AY events (unknown commands skipped), got %d", len(vgm.Events))
	}
	if vgm.Events[0].Reg != 0x00 || vgm.Events[0].Value != 0xFF {
		t.Errorf("event 0 wrong: reg=%d val=%d", vgm.Events[0].Reg, vgm.Events[0].Value)
	}
	if vgm.Events[1].Reg != 0x01 || vgm.Events[1].Value != 0xAA {
		t.Errorf("event 1 wrong: reg=%d val=%d", vgm.Events[1].Reg, vgm.Events[1].Value)
	}
	if vgm.Events[2].Reg != 0x07 || vgm.Events[2].Value != 0x3E {
		t.Errorf("event 2 wrong: reg=%d val=%d", vgm.Events[2].Reg, vgm.Events[2].Value)
	}
}

func TestVGMParse_SkipYM2612Port0Wait(t *testing.T) {
	// 0x80-0x8F: YM2612 port 0 address 2A + wait N samples (0 operands, just cmd byte)
	header := buildVGMHeader(735, 1773400)
	cmds := []byte{
		0xA0, 0x00, 0x42, // AY write
		0x80,             // YM2612 wait 0
		0x81,             // YM2612 wait 1
		0x8F,             // YM2612 wait 15
		0xA0, 0x01, 0x43, // AY write
		0x62, // wait 735
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData should skip 0x80-0x8F commands, got error: %v", err)
	}
	if len(vgm.Events) != 2 {
		t.Fatalf("expected 2 AY events, got %d", len(vgm.Events))
	}
}

func TestVGMParse_SkipDACStreamCommands(t *testing.T) {
	// DAC stream commands: 0x90 (5 bytes), 0x91 (5 bytes), 0x92 (6 bytes),
	// 0x93 (11 bytes), 0x94 (2 bytes), 0x95 (5 bytes)
	header := buildVGMHeader(735, 1773400)
	cmds := []byte{
		0xA0, 0x00, 0x10, // AY write
		0x90, 0x00, 0x00, 0x00, 0x00, // DAC setup (5 bytes)
		0x91, 0x00, 0x00, 0x00, 0x00, // DAC set data (5 bytes)
		0x92, 0x00, 0x00, 0x00, 0x00, 0x00, // DAC set freq (6 bytes)
		0x94, 0x00, // DAC stop (2 bytes)
		0x95, 0x00, 0x00, 0x00, 0x00, // DAC start fast (5 bytes)
		0xA0, 0x01, 0x20, // AY write
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData should skip DAC stream commands, got error: %v", err)
	}
	if len(vgm.Events) != 2 {
		t.Fatalf("expected 2 AY events, got %d", len(vgm.Events))
	}
}

func TestVGMParse_SkipPCMRAMWrite(t *testing.T) {
	// 0x68: PCM RAM write (12 bytes total)
	header := buildVGMHeader(735, 1773400)
	cmds := []byte{
		0xA0, 0x00, 0x01, // AY write
		0x68, 0x66, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // PCM RAM (12 bytes)
		0xA0, 0x01, 0x02, // AY write
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData should skip 0x68 PCM RAM write, got error: %v", err)
	}
	if len(vgm.Events) != 2 {
		t.Fatalf("expected 2 AY events, got %d", len(vgm.Events))
	}
}

func TestVGMParse_MultiChipVGM(t *testing.T) {
	// Simulates a real multi-chip VGM with SN76489 + AY writes.
	// Both SN76489 (0x50) and AY (0xA0) events are extracted independently.
	header := buildVGMHeader(44100, 1773400)
	cmds := []byte{
		// Frame 1: SN76489 + AY
		0x50, 0x80, // SN76489: ch0 tone latch, low nibble=0
		0x50, 0x00, // SN76489: data byte for latched register
		0xA0, 0x00, 0xFE, // AY reg 0
		0xA0, 0x01, 0x00, // AY reg 1
		0xA0, 0x07, 0x3E, // AY mixer
		0xA0, 0x08, 0x0F, // AY vol A
		0x62, // wait 735 (60Hz frame)
		// Frame 2: more mixed
		0x50, 0x90, // SN76489: ch0 attenuation=0 (max vol)
		0xA0, 0x00, 0xD0, // AY reg 0
		0x62, // wait 735
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	assertSNEvents(t, vgm.SNEvents, []SNEvent{
		{Sample: 0, Byte: 0x80},
		{Sample: 0, Byte: 0x00},
		{Sample: 735, Byte: 0x90},
	})
	assertPSGEvents(t, vgm.Events, []PSGEvent{
		{Sample: 0, Reg: 0x00, Value: 0xFE},
		{Sample: 0, Reg: 0x01, Value: 0x00},
		{Sample: 0, Reg: 0x07, Value: 0x3E},
		{Sample: 0, Reg: 0x08, Value: 0x0F},
		{Sample: 735, Reg: 0x00, Value: 0xD0},
	})
}

func TestVGMParse_SN76489_LatchByteEmittedRaw(t *testing.T) {
	header := buildVGMHeaderSN(735, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x90,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	if vgm.SNClockHz != 3579545 {
		t.Errorf("expected SNClockHz=3579545, got %d", vgm.SNClockHz)
	}
	assertSNEvents(t, vgm.SNEvents, []SNEvent{{Sample: 0, Byte: 0x90}})
	assertPSGEvents(t, vgm.Events, nil)
}

func TestVGMParse_SN76489_DataByteEmittedRaw(t *testing.T) {
	header := buildVGMHeaderSN(735, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x85,
		0x50, 0x10,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	assertSNEvents(t, vgm.SNEvents, []SNEvent{
		{Sample: 0, Byte: 0x85},
		{Sample: 0, Byte: 0x10},
	})
	assertPSGEvents(t, vgm.Events, nil)
}

func TestVGMParse_SN76489_TimingPreserved(t *testing.T) {
	header := buildVGMHeaderSN(0, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x90,
		0x61, 0x34, 0x12,
		0x50, 0x9F,
		0x70,
		0x50, 0xBF,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	assertSNEvents(t, vgm.SNEvents, []SNEvent{
		{Sample: 0, Byte: 0x90},
		{Sample: 0x1234, Byte: 0x9F},
		{Sample: 0x1235, Byte: 0xBF},
	})
}

func TestVGMParse_Mixed_AYAndSN_BothEmitted(t *testing.T) {
	header := buildVGMHeaderSN(0, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x90,
		0xA0, 0x08, 0x0F,
		0x62,
		0x50, 0x9F,
		0xA0, 0x08, 0x00,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	assertSNEvents(t, vgm.SNEvents, []SNEvent{
		{Sample: 0, Byte: 0x90},
		{Sample: 735, Byte: 0x9F},
	})
	assertPSGEvents(t, vgm.Events, []PSGEvent{
		{Sample: 0, Reg: 0x08, Value: 0x0F},
		{Sample: 735, Reg: 0x08, Value: 0x00},
	})
}

func TestVGMParse_SNOnlyClockDoesNotAliasAYClock(t *testing.T) {
	header := buildVGMHeaderSN(735, 3579545, 0)
	cmds := []byte{0x50, 0x90, 0x66}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	if vgm.ClockHz != 0 {
		t.Errorf("AY ClockHz should stay 0 for SN-only VGM, got %d", vgm.ClockHz)
	}
	if vgm.SNClockHz != 3579545 {
		t.Errorf("SNClockHz: got %d, want 3579545", vgm.SNClockHz)
	}
}

func TestParseVGMData_TruncatedCommandErrors(t *testing.T) {
	header := buildVGMHeader(1, 44100)

	tests := []struct {
		name string
		cmds []byte
	}{
		{"truncated 2-byte cmd", []byte{0x30}},                   // 0x30 needs 2 bytes
		{"truncated 3-byte cmd", []byte{0x51, 0x00}},             // 0x51 needs 3 bytes
		{"truncated 4-byte cmd", []byte{0xC0, 0x00, 0x00}},       // 0xC0 needs 4 bytes
		{"truncated 5-byte cmd", []byte{0xE0, 0x00, 0x00, 0x00}}, // 0xE0 needs 5 bytes
		{"truncated DAC stream", []byte{0x90, 0x00, 0x00, 0x00}}, // 0x90 needs 5 bytes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := append(append([]byte{}, header...), tt.cmds...)
			_, err := ParseVGMData(data)
			if err == nil {
				t.Error("expected error for truncated command")
			}
		})
	}
}

// buildVGMHeaderWithLoop creates a VGM header with loop fields set.
func buildVGMHeaderWithLoop(totalSamples, loopSamples, ayClock uint32, loopDataOffset uint32) []byte {
	header := buildVGMHeader(totalSamples, ayClock)
	binary.LittleEndian.PutUint32(header[0x20:0x24], loopSamples)
	if loopDataOffset > 0 {
		// Loop offset is relative to position 0x1C
		binary.LittleEndian.PutUint32(header[0x1C:0x20], loopDataOffset-0x1C)
	}
	return header
}

func TestVGMGolden_AYSequence(t *testing.T) {
	// Multi-frame AY-only VGM exercising all wait command variants:
	//   0x61 (16-bit wait), 0x62 (735 samples), 0x63 (882 samples), 0x70-0x7F (1-16 samples)
	header := buildVGMHeader(0, 1773400) // totalSamples auto-calculated
	cmds := []byte{
		// Sample 0: set up channel A
		0xA0, 0x00, 0xFE, // AY reg 0 = 0xFE (fine tune A)
		0xA0, 0x01, 0x01, // AY reg 1 = 0x01 (coarse tune A)
		0xA0, 0x07, 0x3E, // AY reg 7 = 0x3E (mixer: tone A on)
		0xA0, 0x08, 0x0F, // AY reg 8 = 0x0F (vol A max)
		0x62, // wait 735 (60Hz NTSC frame)
		// Sample 735: change freq
		0xA0, 0x00, 0xD0, // AY reg 0 = 0xD0
		0x63, // wait 882 (50Hz PAL frame)
		// Sample 1617: short waits
		0xA0, 0x00, 0xAA, // AY reg 0 = 0xAA
		0x70, // wait 1 sample (0x70 = wait (n&0xF)+1 = 1)
		// Sample 1618:
		0xA0, 0x08, 0x0A, // AY reg 8 = 0x0A (vol A=10)
		0x7F, // wait 16 samples
		// Sample 1634:
		0xA0, 0x08, 0x05, // AY reg 8 = 0x05
		0x61, 0x00, 0x01, // wait 256 samples (0x0100 LE)
		// Sample 1890:
		0xA0, 0x08, 0x00, // AY reg 8 = 0x00 (silence)
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	expected := []PSGEvent{
		{Sample: 0, Reg: 0x00, Value: 0xFE},
		{Sample: 0, Reg: 0x01, Value: 0x01},
		{Sample: 0, Reg: 0x07, Value: 0x3E},
		{Sample: 0, Reg: 0x08, Value: 0x0F},
		{Sample: 735, Reg: 0x00, Value: 0xD0},
		{Sample: 1617, Reg: 0x00, Value: 0xAA},
		{Sample: 1618, Reg: 0x08, Value: 0x0A},
		{Sample: 1634, Reg: 0x08, Value: 0x05},
		{Sample: 1890, Reg: 0x08, Value: 0x00},
	}

	if len(vgm.Events) != len(expected) {
		t.Fatalf("event count: got %d, want %d", len(vgm.Events), len(expected))
	}
	for i, want := range expected {
		got := vgm.Events[i]
		if got.Sample != want.Sample || got.Reg != want.Reg || got.Value != want.Value {
			t.Errorf("event[%d]: got {Sample:%d Reg:0x%02X Value:0x%02X}, want {Sample:%d Reg:0x%02X Value:0x%02X}",
				i, got.Sample, got.Reg, got.Value, want.Sample, want.Reg, want.Value)
		}
	}

	if vgm.ClockHz != 1773400 {
		t.Errorf("ClockHz: got %d, want 1773400", vgm.ClockHz)
	}
}

func TestVGMGolden_SN76489Full(t *testing.T) {
	// Complete SN76489 sequence: parser preserves raw latch/data bytes.
	header := buildVGMHeaderSN(0, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x85,
		0x50, 0x06,
		0x50, 0x90,
		0x50, 0xAA,
		0x50, 0x03,
		0x50, 0xB5,
		0x50, 0xC0,
		0x50, 0x01,
		0x50, 0xDF,
		0x50, 0xE4,
		0x50, 0xF0,
		0x62, // wait 735
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	assertSNEvents(t, vgm.SNEvents, []SNEvent{
		{Sample: 0, Byte: 0x85},
		{Sample: 0, Byte: 0x06},
		{Sample: 0, Byte: 0x90},
		{Sample: 0, Byte: 0xAA},
		{Sample: 0, Byte: 0x03},
		{Sample: 0, Byte: 0xB5},
		{Sample: 0, Byte: 0xC0},
		{Sample: 0, Byte: 0x01},
		{Sample: 0, Byte: 0xDF},
		{Sample: 0, Byte: 0xE4},
		{Sample: 0, Byte: 0xF0},
	})
	assertPSGEvents(t, vgm.Events, nil)
}

func TestVGMGolden_MixedChipStream(t *testing.T) {
	// Interleaved AY + SN76489 + ignored chip commands.
	// Verifies only AY/SN events survive and timestamps are correct.
	header := buildVGMHeaderSN(0, 3579545, 1773400)
	cmds := []byte{
		// Sample 0: AY write
		0xA0, 0x00, 0x42,
		// Sample 0: YM2612 port 0 (ignored, 3 bytes)
		0x52, 0x30, 0x40,
		// Sample 0: SN76489 ch0 atten=0
		0x50, 0x90,
		// Sample 0: YM2413 (ignored, 3 bytes)
		0x51, 0x10, 0x20,
		// Sample 0: data block (ignored)
		0x67, 0x66, 0x00, 0x04, 0x00, 0x00, 0x00, 0xDE, 0xAD, 0xBE, 0xEF,
		// Sample 0: Sega PCM (ignored, 4 bytes)
		0xC0, 0x01, 0x02, 0x03,
		// Sample 0: PCM seek (ignored, 5 bytes)
		0xE0, 0x00, 0x00, 0x00, 0x00,
		0x62, // wait 735
		// Sample 735: AY write
		0xA0, 0x01, 0x03,
		0x62, // wait 735
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	assertPSGEvents(t, vgm.Events, []PSGEvent{
		{Sample: 0, Reg: 0x00, Value: 0x42},
		{Sample: 735, Reg: 0x01, Value: 0x03},
	})
	assertSNEvents(t, vgm.SNEvents, []SNEvent{
		{Sample: 0, Byte: 0x90},
	})
}

func TestVGMGolden_LoopAndTiming(t *testing.T) {
	// VGM with loop offset in header. Verify LoopSample and TotalSamples.
	// Layout: header (0x80 bytes) → 3 AY writes + wait 735 → 3 AY writes (loop point) + wait 735 → end
	//
	// Data starts at offset 0x80.
	// Loop should point to the second batch of AY writes.
	// First batch: 9 bytes (3 × 3-byte AY cmd) + 1 byte (0x62 wait) = 10 bytes
	// Loop data offset = 0x80 + 10 = 0x8A
	// loopOffset field at 0x1C is relative: 0x8A - 0x1C = 0x6E

	totalSamples := uint32(1470) // 735 + 735
	loopSamples := uint32(735)   // loop covers last 735 samples

	header := make([]byte, 0x80)
	copy(header[0:4], []byte("Vgm "))
	binary.LittleEndian.PutUint32(header[0x08:0x0C], 0x00000172)
	binary.LittleEndian.PutUint32(header[0x18:0x1C], totalSamples)
	binary.LittleEndian.PutUint32(header[0x1C:0x20], 0x8A-0x1C)   // loop offset (relative to 0x1C)
	binary.LittleEndian.PutUint32(header[0x20:0x24], loopSamples) // loop sample count
	binary.LittleEndian.PutUint32(header[0x34:0x38], 0x4C)        // data offset: 0x34+0x4C=0x80
	binary.LittleEndian.PutUint32(header[0x74:0x78], 1773400)     // AY clock

	cmds := []byte{
		// Offset 0x80: Frame 1
		0xA0, 0x00, 0xFE, // AY reg 0 = 0xFE
		0xA0, 0x07, 0x3E, // AY reg 7 = 0x3E
		0xA0, 0x08, 0x0F, // AY reg 8 = 0x0F
		0x62, // wait 735 → samplePos = 735
		// Offset 0x8A: Frame 2 (loop start)
		0xA0, 0x00, 0xD0, // AY reg 0 = 0xD0
		0xA0, 0x08, 0x0A, // AY reg 8 = 0x0A
		0xA0, 0x01, 0x02, // AY reg 1 = 0x02
		0x62, // wait 735 → samplePos = 1470
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	// LoopSample should be 735 (the sample position when we hit offset 0x8A)
	if vgm.LoopSample != 735 {
		t.Errorf("LoopSample: got %d, want 735", vgm.LoopSample)
	}
	if vgm.LoopSamples != 735 {
		t.Errorf("LoopSamples: got %d, want 735", vgm.LoopSamples)
	}
	if vgm.TotalSamples != 1470 {
		t.Errorf("TotalSamples: got %d, want 1470", vgm.TotalSamples)
	}

	// Verify events at correct positions
	if len(vgm.Events) != 6 {
		t.Fatalf("event count: got %d, want 6", len(vgm.Events))
	}
	// First 3 at sample 0
	for i := range 3 {
		if vgm.Events[i].Sample != 0 {
			t.Errorf("event[%d].Sample: got %d, want 0", i, vgm.Events[i].Sample)
		}
	}
	// Last 3 at sample 735
	for i := 3; i < 6; i++ {
		if vgm.Events[i].Sample != 735 {
			t.Errorf("event[%d].Sample: got %d, want 735", i, vgm.Events[i].Sample)
		}
	}
}
