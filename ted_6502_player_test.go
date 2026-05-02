// ted_6502_player_test.go - Tests for TED 6502 music player

package main

import (
	"os"
	"testing"
)

func makeRealTEDTestFile(t *testing.T, programs map[uint16][]byte, subtunes int) []byte {
	t.Helper()
	data := make([]byte, 0x1400)
	data[0] = 0x01
	data[1] = 0x10
	data[2] = 0x10
	data[3] = 0x00

	sigStart := TMF_SIGNATURE_OFFSET
	copy(data[sigStart:], "TEDMUSIC\x00")
	data[sigStart+TED_HDR_INIT_LO] = 0x00
	data[sigStart+TED_HDR_INIT_HI] = 0x20
	data[sigStart+TED_HDR_PLAY_LO] = 0x00
	data[sigStart+TED_HDR_PLAY_HI] = 0x00
	data[sigStart+TED_HDR_SUBTUNES] = byte(subtunes)
	data[sigStart+TED_HDR_SUBTUNES+1] = byte(subtunes >> 8)

	for addr, code := range programs {
		off := int(addr-0x1001) + 2
		if off < 0 || off+len(code) > len(data) {
			t.Fatalf("program at $%04X does not fit test file", addr)
		}
		copy(data[off:], code)
	}

	return data
}

func TestTED6502PlayerCreation(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}
	if player == nil {
		t.Fatal("player is nil")
	}
	if player.clockHz != TED_CLOCK_PAL {
		t.Errorf("clockHz = %d, want %d", player.clockHz, TED_CLOCK_PAL)
	}
	if player.frameRate != 50 {
		t.Errorf("frameRate = %d, want 50", player.frameRate)
	}
}

func TestTED6502PlayerLoadFile(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}

	data, err := os.ReadFile("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	err = player.LoadFromData(data)
	if err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}

	meta := player.GetMetadata()
	if meta.Title == "" {
		t.Error("title should be set")
	}
	t.Logf("Loaded: %q by %q", meta.Title, meta.Author)
}

func TestTED6502PlayerRenderFrame(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}

	data, err := os.ReadFile("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	err = player.LoadFromData(data)
	if err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}

	// Render a few frames
	var totalEvents int
	for range 10 {
		events, err := player.RenderFrame()
		if err != nil {
			t.Fatalf("RenderFrame failed: %v", err)
		}
		totalEvents += len(events)
	}

	t.Logf("Total events from 10 frames: %d", totalEvents)
}

func TestTED6502PlayerGetClockHz(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	// Default is PAL
	if player.GetClockHz() != TED_CLOCK_PAL {
		t.Errorf("GetClockHz = %d, want %d", player.GetClockHz(), TED_CLOCK_PAL)
	}
}

func TestTED6502PlayerGetFrameRate(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	// Default is 50 Hz (PAL)
	if player.GetFrameRate() != 50 {
		t.Errorf("GetFrameRate = %d, want 50", player.GetFrameRate())
	}
}

func TestTED6502PlayerReset(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	data, err := os.ReadFile("/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}

	player.LoadFromData(data)
	player.RenderFrame() // Advance state

	player.Reset()

	// After reset, should be able to render again
	_, err = player.RenderFrame()
	if err != nil {
		t.Errorf("RenderFrame after reset failed: %v", err)
	}
}

func TestTED6502PlayerCyclesPerFrame(t *testing.T) {
	player, _ := NewTED6502Player(nil, SAMPLE_RATE)

	// PAL: 886724 Hz / 50 fps = 17734 cycles per frame
	expected := uint64(TED_CLOCK_PAL / 50)
	if player.cyclesPerFrame != expected {
		t.Errorf("cyclesPerFrame = %d, want %d", player.cyclesPerFrame, expected)
	}
}

func TestTEDSubtuneDispatchUsesAReg(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}
	data := makeRealTEDTestFile(t, map[uint16][]byte{
		0x2000: {
			0x8D, 0x0E, 0xFF, // STA $FF0E
			0x8E, 0x0F, 0xFF, // STX $FF0F
			0x4C, 0x06, 0x20, // JMP $2006
		},
	}, 2)

	if err := player.LoadFromData(data); err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}
	if err := player.SelectSubtune(1); err != nil {
		t.Fatalf("SelectSubtune failed: %v", err)
	}
	if player.cpu.PC != 0x2000 || player.cpu.A != 1 || player.cpu.X != 0 {
		t.Fatalf("init entry state PC=$%04X A=%d X=%d, want PC=$2000 A=1 X=0", player.cpu.PC, player.cpu.A, player.cpu.X)
	}
	events, err := player.RenderFrame()
	if err != nil {
		t.Fatalf("RenderFrame failed: %v", err)
	}

	var sawA bool
	for _, ev := range events {
		if ev.Reg == TED_REG_FREQ1_LO && ev.Value == 1 {
			sawA = true
		}
	}
	if !sawA {
		t.Fatalf("init did not receive selected subtune in A")
	}
}

func TestTEDSubtuneRegisterTraceDiffers(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}
	data := makeRealTEDTestFile(t, map[uint16][]byte{
		0x2000: {
			0x8D, 0x0E, 0xFF, // STA $FF0E
			0x4C, 0x03, 0x20, // JMP $2003
		},
	}, 2)
	if err := player.LoadFromData(data); err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}

	if err := player.SelectSubtune(0); err != nil {
		t.Fatalf("SelectSubtune(0) failed: %v", err)
	}
	events0, err := player.RenderFrame()
	if err != nil {
		t.Fatalf("RenderFrame subtune 0 failed: %v", err)
	}
	if err := player.SelectSubtune(1); err != nil {
		t.Fatalf("SelectSubtune(1) failed: %v", err)
	}
	events1, err := player.RenderFrame()
	if err != nil {
		t.Fatalf("RenderFrame subtune 1 failed: %v", err)
	}

	if len(events0) == 0 || len(events1) == 0 {
		t.Fatalf("expected register traces for both subtunes")
	}
	if events0[0].Reg != events1[0].Reg || events0[0].Value == events1[0].Value {
		t.Fatalf("subtune traces did not differ: first0=%+v first1=%+v", events0[0], events1[0])
	}
}

func TestTEDBus_RealTED_RasterDispatch(t *testing.T) {
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("NewTED6502Player failed: %v", err)
	}
	data := makeRealTEDTestFile(t, map[uint16][]byte{
		0x2000: {
			0xA9, 0x06, 0x8D, 0x0B, 0xFF, // LDA #$06; STA $FF0B
			0xA9, 0x02, 0x8D, 0x0A, 0xFF, // LDA #$02; STA $FF0A
			0xA9, 0x20, 0x8D, 0x14, 0x03, // LDA #$20; STA $0314
			0xA9, 0x21, 0x8D, 0x15, 0x03, // LDA #$21; STA $0315
			0x58,             // CLI
			0x4C, 0x19, 0x20, // JMP $2019
		},
		0x2120: {
			0xA9, 0x7F, 0x8D, 0x11, 0xFF, // LDA #$7F; STA $FF11
			0xA9, 0x02, 0x8D, 0x09, 0xFF, // LDA #$02; STA $FF09
			0x40, // RTI
		},
	}, 1)

	if err := player.LoadFromData(data); err != nil {
		t.Fatalf("LoadFromData failed: %v", err)
	}
	if player.bus.timer1Running {
		t.Fatalf("RealTED mode should not enable synthetic Timer 1 dispatch")
	}
	events, err := player.RenderFrame()
	if err != nil {
		t.Fatalf("RenderFrame failed: %v", err)
	}

	for _, ev := range events {
		if ev.Reg == TED_REG_SND_CTRL && ev.Value == 0x7F {
			return
		}
	}
	t.Fatalf("raster IRQ handler did not run; events=%+v", events)
}
