package main

import (
	"encoding/binary"
	"testing"
)

// buildVGMHeader creates a minimal VGM header with data starting at offset 0x40.
func buildVGMHeader(totalSamples uint32, ayClock uint32) []byte {
	header := make([]byte, 0x80)
	copy(header[0:4], []byte("Vgm "))
	binary.LittleEndian.PutUint32(header[0x08:0x0C], 0x00000172) // version 1.72
	binary.LittleEndian.PutUint32(header[0x18:0x1C], totalSamples)
	binary.LittleEndian.PutUint32(header[0x34:0x38], 0x4C) // data offset: 0x34+0x4C=0x80
	binary.LittleEndian.PutUint32(header[0x74:0x78], ayClock)
	return header
}

func TestVGMParse_AYOnly(t *testing.T) {
	// Simple VGM with only AY writes — should already work.
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
	// SN76489 (0x50) already handled, this tests that AY events are
	// properly extracted while other chip commands are skipped.
	header := buildVGMHeader(44100, 1773400)
	cmds := []byte{
		// Frame 1: SN76489 + AY
		0x50, 0x80, // SN76489 write (already skipped)
		0x50, 0x00, // SN76489 write
		0xA0, 0x00, 0xFE, // AY reg 0
		0xA0, 0x01, 0x00, // AY reg 1
		0xA0, 0x07, 0x3E, // AY mixer
		0xA0, 0x08, 0x0F, // AY vol A
		0x62, // wait 735 (60Hz frame)
		// Frame 2: more mixed
		0x50, 0x90, // SN76489
		0xA0, 0x00, 0xD0, // AY reg 0
		0x62, // wait 735
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	if len(vgm.Events) != 5 {
		t.Fatalf("expected 5 AY events, got %d", len(vgm.Events))
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
