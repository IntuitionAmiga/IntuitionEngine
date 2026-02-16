package main

import (
	"testing"
)

func TestBuildTrackerZ80RAM(t *testing.T) {
	player := []byte{0xC9, 0x00, 0xC9} // RET, NOP, RET
	module := []byte{0x01, 0x02, 0x03, 0x04}

	config := trackerFormatConfig{
		name:         "test",
		playerBinary: player,
		playerBase:   0xC000,
		moduleBase:   0x4000,
		initEntry:    0xC000,
		playEntry:    0xC002,
	}

	ram, err := buildTrackerZ80RAM(config, module)
	if err != nil {
		t.Fatalf("buildTrackerZ80RAM error: %v", err)
	}

	// Verify player bytes at playerBase
	for i, b := range player {
		if ram[0xC000+i] != b {
			t.Errorf("player[%d] = 0x%02X, want 0x%02X", i, ram[0xC000+i], b)
		}
	}

	// Verify module bytes at moduleBase
	for i, b := range module {
		if ram[0x4000+i] != b {
			t.Errorf("module[%d] = 0x%02X, want 0x%02X", i, ram[0x4000+i], b)
		}
	}

	// Verify bootstrap stub starts at 0x0000
	if ram[0] != 0xF3 { // DI
		t.Errorf("stub[0] = 0x%02X, want 0xF3 (DI)", ram[0])
	}
}

func TestBuildTrackerZ80Stub(t *testing.T) {
	stub := buildTrackerStub(0xC000, 0xC003, 0x4000)

	// Expected sequence:
	// F3          DI
	// 21 00 40    LD HL, 0x4000
	// CD 00 C0    CALL 0xC000
	// ED 56       IM 1
	// FB          EI
	// 76          HALT
	// CD 03 C0    CALL 0xC003
	// 18 F6       JR -10 (back to IM 1)

	expected := []byte{
		0xF3,             // DI
		0x21, 0x00, 0x40, // LD HL, 0x4000
		0xCD, 0x00, 0xC0, // CALL 0xC000
		0xED, 0x56, // IM 1
		0xFB,             // EI
		0x76,             // HALT
		0xCD, 0x03, 0xC0, // CALL 0xC003
	}

	for i, b := range expected {
		if i >= len(stub) {
			t.Fatalf("stub too short: %d bytes, need at least %d", len(stub), len(expected))
		}
		if stub[i] != b {
			t.Errorf("stub[%d] = 0x%02X, want 0x%02X", i, stub[i], b)
		}
	}

	// Verify the JR target
	if len(stub) < len(expected)+2 {
		t.Fatalf("stub missing JR at end")
	}
	if stub[len(expected)] != 0x18 {
		t.Errorf("stub[%d] = 0x%02X, want 0x18 (JR)", len(expected), stub[len(expected)])
	}
	// JR should jump back to the IM 1 instruction (offset 7)
	jrTarget := int(int8(stub[len(expected)+1])) + len(expected) + 2
	if jrTarget != 7 {
		t.Errorf("JR target = %d, want 7 (IM 1 position)", jrTarget)
	}
}

func TestRenderTrackerZ80_TrivialPlayer(t *testing.T) {
	// Build a trivial Z80 "player" that writes fixed values to AY ports.
	// ZX Spectrum AY: select via OUT (0xFFFD),A, data via OUT (0xBFFD),A
	//
	// INIT (0xC000): RET
	// PLAY (0xC001):
	//   LD A, 0        ; select register 0
	//   LD BC, 0xFFFD
	//   OUT (C), A     ; select reg 0
	//   LD A, 0x42     ; data value
	//   LD BC, 0xBFFD
	//   OUT (C), A     ; write 0x42 to reg 0
	//   RET
	player := []byte{
		0xC9,       // 0xC000: RET (init - no-op)
		0x3E, 0x00, // 0xC001: LD A, 0 (select reg 0)
		0x01, 0xFD, 0xFF, // 0xC004: LD BC, 0xFFFD
		0xED, 0x79, // 0xC007: OUT (C), A
		0x3E, 0x42, // 0xC009: LD A, 0x42
		0x01, 0xFD, 0xBF, // 0xC00B: LD BC, 0xBFFD
		0xED, 0x79, // 0xC00E: OUT (C), A
		0xC9, // 0xC010: RET
	}

	config := trackerFormatConfig{
		name:         "test",
		playerBinary: player,
		playerBase:   0xC000,
		moduleBase:   0x4000,
		initEntry:    0xC000,
		playEntry:    0xC001,
		system:       0, // ZX Spectrum
		clockHz:      PSG_CLOCK_ZX_SPECTRUM,
		z80ClockHz:   Z80_CLOCK_ZX_SPECTRUM,
		frameRate:    50,
	}

	_, events, total, err := renderTrackerZ80(config, nil, 44100, 1)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
	if total == 0 {
		t.Error("totalSamples should be > 0")
	}
	if len(events) == 0 {
		t.Fatal("expected at least 1 event")
	}

	// Find the register 0 write with value 0x42
	found := false
	for _, ev := range events {
		if ev.Reg == 0 && ev.Value == 0x42 {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find expected register write (reg=0, value=0x42)")
	}
}

func TestRenderTrackerZ80_MultipleFrames(t *testing.T) {
	// Player that writes a counter value to register 1 each frame.
	// Uses RAM at 0x4000 as a counter.
	//
	// INIT (0xC000):
	//   XOR A
	//   LD (0x4000), A   ; counter = 0
	//   RET
	// PLAY (0xC005):
	//   LD A, 1          ; select reg 1
	//   LD BC, 0xFFFD
	//   OUT (C), A
	//   LD A, (0x4000)   ; load counter
	//   LD BC, 0xBFFD
	//   OUT (C), A       ; write counter to reg 1
	//   LD A, (0x4000)   ; reload counter
	//   INC A
	//   LD (0x4000), A   ; counter++
	//   RET
	player := []byte{
		// INIT at 0xC000
		0xAF,             // XOR A (A=0)
		0x32, 0x00, 0x40, // LD (0x4000), A
		0xC9, // RET
		// PLAY at 0xC005
		0x3E, 0x01, // LD A, 1
		0x01, 0xFD, 0xFF, // LD BC, 0xFFFD
		0xED, 0x79, // OUT (C), A  (select reg 1)
		0x3A, 0x00, 0x40, // LD A, (0x4000)
		0x01, 0xFD, 0xBF, // LD BC, 0xBFFD
		0xED, 0x79, // OUT (C), A  (write counter)
		0x3A, 0x00, 0x40, // LD A, (0x4000)
		0x3C,             // INC A
		0x32, 0x00, 0x40, // LD (0x4000), A
		0xC9, // RET
	}

	config := trackerFormatConfig{
		name:         "test",
		playerBinary: player,
		playerBase:   0xC000,
		moduleBase:   0x4000,
		initEntry:    0xC000,
		playEntry:    0xC005,
		system:       0,
		clockHz:      PSG_CLOCK_ZX_SPECTRUM,
		z80ClockHz:   Z80_CLOCK_ZX_SPECTRUM,
		frameRate:    50,
	}

	// Run extra frames to account for init overhead on first frame
	_, events, _, err := renderTrackerZ80(config, nil, 44100, 5)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}

	// Should have sequential writes to register 1
	reg1Writes := []uint8{}
	for _, ev := range events {
		if ev.Reg == 1 {
			reg1Writes = append(reg1Writes, ev.Value)
		}
	}
	if len(reg1Writes) < 3 {
		t.Fatalf("expected at least 3 writes to reg 1, got %d", len(reg1Writes))
	}
	// Verify sequential counter values
	for i := 1; i < len(reg1Writes); i++ {
		if reg1Writes[i] != reg1Writes[i-1]+1 {
			t.Errorf("reg1 writes not sequential: [%d]=%d, [%d]=%d", i-1, reg1Writes[i-1], i, reg1Writes[i])
		}
	}
}

func TestRenderTrackerZ80_MaxFramesBound(t *testing.T) {
	// Simple player that always writes to register 0
	player := []byte{
		0xC9,       // INIT: RET
		0x3E, 0x00, // LD A, 0
		0x01, 0xFD, 0xFF, // LD BC, 0xFFFD
		0xED, 0x79, // OUT (C), A
		0x3E, 0xFF, // LD A, 0xFF
		0x01, 0xFD, 0xBF, // LD BC, 0xBFFD
		0xED, 0x79, // OUT (C), A
		0xC9, // RET
	}

	config := trackerFormatConfig{
		name:         "test",
		playerBinary: player,
		playerBase:   0xC000,
		moduleBase:   0x4000,
		initEntry:    0xC000,
		playEntry:    0xC001,
		system:       0,
		clockHz:      PSG_CLOCK_ZX_SPECTRUM,
		z80ClockHz:   Z80_CLOCK_ZX_SPECTRUM,
		frameRate:    50,
	}

	_, events, _, err := renderTrackerZ80(config, nil, 44100, 5)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}

	// Should have writes to reg 0 bounded by maxFrames (may be N-1 due to init frame)
	reg0Count := 0
	for _, ev := range events {
		if ev.Reg == 0 && ev.Value == 0xFF {
			reg0Count++
		}
	}
	if reg0Count < 4 || reg0Count > 5 {
		t.Errorf("expected 4-5 reg 0 writes, got %d", reg0Count)
	}
}

func TestRenderTrackerZ80_EmptyModule(t *testing.T) {
	// Player that just RETs
	player := []byte{0xC9, 0xC9} // init: RET, play: RET

	config := trackerFormatConfig{
		name:         "test",
		playerBinary: player,
		playerBase:   0xC000,
		moduleBase:   0x4000,
		initEntry:    0xC000,
		playEntry:    0xC001,
		system:       0,
		clockHz:      PSG_CLOCK_ZX_SPECTRUM,
		z80ClockHz:   Z80_CLOCK_ZX_SPECTRUM,
		frameRate:    50,
	}

	// Empty module should not crash
	_, _, _, err := renderTrackerZ80(config, nil, 44100, 1)
	if err != nil {
		t.Fatalf("renderTrackerZ80 error: %v", err)
	}
}

func TestRenderTrackerZ80_NilPlayerBinary(t *testing.T) {
	config := trackerFormatConfig{
		name:       "test",
		clockHz:    PSG_CLOCK_ZX_SPECTRUM,
		z80ClockHz: Z80_CLOCK_ZX_SPECTRUM,
		frameRate:  50,
	}

	_, _, _, err := renderTrackerZ80(config, nil, 44100, 1)
	if err == nil {
		t.Error("expected error for nil player binary")
	}
}
