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
	// Both SN76489 (0x50) and AY (0xA0) events are now extracted.
	// SN76489 writes are converted to AY-equivalent register writes.
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
	// SN76489 writes now produce events: tone latch → 2 freq events,
	// data byte → 2 freq events, attenuation → 1 vol event.
	// Plus 5 AY events = 10 total.
	if len(vgm.Events) < 5 {
		t.Fatalf("expected at least 5 events, got %d", len(vgm.Events))
	}

	// Verify that AY events are present and correctly positioned
	// Find AY reg 0 = 0xFE in the event stream (it should be there from the 0xA0 command)
	found := false
	for _, ev := range vgm.Events {
		if ev.Reg == 0x00 && ev.Value == 0xFE {
			found = true
			break
		}
	}
	if !found {
		t.Error("AY reg 0 = 0xFE not found in events")
	}
}

func TestVGMParse_SN76489_ToneLatch(t *testing.T) {
	// SN76489 latch byte for tone: bit7=1, bits6-5=channel, bit4=0(tone), bits3-0=low data
	// 0x80 = channel 0, tone, low nibble=0
	// Followed by data byte 0x01 → high bits=0x01, combined divider = (0x01 << 4) | 0x00 = 0x10
	header := buildVGMHeaderSN(735, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x80, // Latch: ch0, tone, low=0
		0x50, 0x01, // Data: high bits=0x01 → divider = 0x10
		0x62, // wait 735
		0x66, // end
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	if vgm.SNClockHz != 3579545 {
		t.Errorf("expected SNClockHz=3579545, got %d", vgm.SNClockHz)
	}

	// Should produce frequency register events for channel A (regs 0, 1)
	// Divider 0x10=16, AY equiv: 16 * 1773400 / (3579545 * 2) ≈ 3
	foundFreqLo := false
	for _, ev := range vgm.Events {
		if ev.Reg == 0 { // AY channel A fine tune
			foundFreqLo = true
		}
	}
	if !foundFreqLo {
		t.Error("expected frequency register events for channel A")
	}
}

func TestVGMParse_SN76489_Attenuation(t *testing.T) {
	// SN76489 attenuation latch: bit7=1, bits6-5=channel, bit4=1(atten), bits3-0=value
	// 0x90 = ch0 atten=0 (max volume) → AY vol 15
	// 0x9F = ch0 atten=15 (silence) → AY vol 0
	header := buildVGMHeaderSN(735, 3579545, 0)
	cmds := []byte{
		0x50, 0x90, // ch0 atten=0 (max vol)
		0x50, 0xBF, // ch1 atten=15 (silence)
		0x50, 0xD5, // ch2 atten=5
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	// Find volume events for each channel
	var volA, volB, volC uint8
	var foundA, foundB, foundC bool
	for _, ev := range vgm.Events {
		switch ev.Reg {
		case 8:
			volA = ev.Value
			foundA = true
		case 9:
			volB = ev.Value
			foundB = true
		case 10:
			volC = ev.Value
			foundC = true
		}
	}

	if !foundA || volA != 15 {
		t.Errorf("ch0 atten=0: expected AY vol 15, got %d (found=%v)", volA, foundA)
	}
	if !foundB || volB != 0 {
		t.Errorf("ch1 atten=15: expected AY vol 0, got %d (found=%v)", volB, foundB)
	}
	if !foundC || volC != 10 {
		t.Errorf("ch2 atten=5: expected AY vol 10, got %d (found=%v)", volC, foundC)
	}
}

func TestVGMParse_SN76489_NoiseChannel(t *testing.T) {
	// SN76489 noise register: bit7=1, bits6-5=11(ch3), bit4=0(tone=noise ctrl)
	// 0xE0 = ch3, noise ctrl = 0x00 (periodic noise, fastest rate)
	// 0xE4 = ch3, noise ctrl = 0x04 (white noise, fastest rate)
	header := buildVGMHeaderSN(735, 3579545, 0)
	cmds := []byte{
		0x50, 0xE4, // Noise: white noise, rate 0 (fastest)
		0x50, 0xF0, // Noise attenuation = 0 (max vol, enables noise in mixer)
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	// Should produce noise period event (reg 6) and mixer event (reg 7)
	var foundNoise, foundMixer bool
	var noiseVal, mixerVal uint8
	for _, ev := range vgm.Events {
		if ev.Reg == 6 {
			foundNoise = true
			noiseVal = ev.Value
		}
		if ev.Reg == 7 {
			foundMixer = true
			mixerVal = ev.Value
		}
	}

	if !foundNoise {
		t.Error("expected noise period event (reg 6)")
	} else if noiseVal != 4 {
		t.Errorf("noise period: got %d, want 4 (fastest rate)", noiseVal)
	}

	if !foundMixer {
		t.Error("expected mixer event (reg 7)")
	} else if mixerVal&0x20 != 0 {
		// Bit 5 should be 0 (noise enabled on channel C) when noise atten < 15
		t.Errorf("mixer 0x%02X: noise on channel C should be enabled (bit 5=0)", mixerVal)
	}
}

func TestVGMParse_SN76489_DataByte(t *testing.T) {
	// Test the latch+data byte protocol for multi-byte tone writes.
	// Latch sets low 4 bits, data byte sets high 6 bits.
	header := buildVGMHeaderSN(735, 3579545, 1773400)
	cmds := []byte{
		0x50, 0x85, // Latch: ch0, tone, low=5 (0x05)
		0x50, 0x10, // Data: high bits=0x10 → divider = (0x10 << 4) | 0x05 = 0x105
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	// The data byte should produce updated frequency events.
	// Divider 0x105 = 261. AY equiv: 261 * 1773400 / (3579545 * 2) ≈ 64
	// Find the last frequency register write for channel A
	var lastFreqLo, lastFreqHi uint8
	for _, ev := range vgm.Events {
		if ev.Reg == 0 {
			lastFreqLo = ev.Value
		}
		if ev.Reg == 1 {
			lastFreqHi = ev.Value
		}
	}
	ayDiv := uint16(lastFreqLo) | (uint16(lastFreqHi) << 8)
	// Expected: 261 * 1773400 / (3579545 * 2) ≈ 64
	if ayDiv < 50 || ayDiv > 80 {
		t.Errorf("AY divider %d out of expected range [50,80] for SN divider 261", ayDiv)
	}
}

func TestVGMParse_SN76489_ClockScaling(t *testing.T) {
	// Verify frequency divider conversion with different clock configurations.
	// SN76489 SMS clock = 3579545 Hz, AY MSX clock = 1789773 Hz
	// SN divider 100 → Freq = 3579545 / (32*100) = 1118.6 Hz
	// AY divider = 100 * 1789773 / (3579545 * 2) = 24.999... ≈ 25
	header := buildVGMHeaderSN(735, 3579545, 1789773)
	cmds := []byte{
		0x50, 0x84, // Latch: ch0, tone, low=4
		0x50, 0x06, // Data: high=6 → divider = (6<<4)|4 = 100
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}

	var lastFreqLo, lastFreqHi uint8
	for _, ev := range vgm.Events {
		if ev.Reg == 0 {
			lastFreqLo = ev.Value
		}
		if ev.Reg == 1 {
			lastFreqHi = ev.Value
		}
	}
	ayDiv := uint16(lastFreqLo) | (uint16(lastFreqHi) << 8)
	// Expected: 100 * 1789773 / (3579545 * 2) = 24.999 ≈ 25
	if ayDiv != 25 {
		t.Errorf("AY divider: got %d, want 25 for SN divider 100 (SN=3579545, AY=1789773)", ayDiv)
	}
}

func TestVGMParse_SN76489_OnlyClockFallback(t *testing.T) {
	// VGM with only SN76489 (no AY clock) should use SN clock as primary.
	header := buildVGMHeaderSN(735, 3579545, 0) // no AY clock
	cmds := []byte{
		0x50, 0x90, // ch0 atten=0 (max vol)
		0x62,
		0x66,
	}
	data := append(header, cmds...)

	vgm, err := ParseVGMData(data)
	if err != nil {
		t.Fatalf("ParseVGMData failed: %v", err)
	}
	if vgm.ClockHz != 3579545 {
		t.Errorf("expected ClockHz=3579545 (SN fallback), got %d", vgm.ClockHz)
	}
	if vgm.SNClockHz != 3579545 {
		t.Errorf("expected SNClockHz=3579545, got %d", vgm.SNClockHz)
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
