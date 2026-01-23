// ted_engine_test.go - Tests for TED sound chip emulation

package main

import (
	"testing"
)

func TestTEDEngineCreation(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	if engine == nil {
		t.Fatal("NewTEDEngine returned nil")
	}
	if engine.clockHz != TED_CLOCK_PAL {
		t.Errorf("expected PAL clock %d, got %d", TED_CLOCK_PAL, engine.clockHz)
	}
	if engine.sampleRate != SAMPLE_RATE {
		t.Errorf("expected sample rate %d, got %d", SAMPLE_RATE, engine.sampleRate)
	}
}

func TestTEDClockHz(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Test setting NTSC clock
	engine.SetClockHz(TED_CLOCK_NTSC)
	if engine.clockHz != TED_CLOCK_NTSC {
		t.Errorf("expected NTSC clock %d, got %d", TED_CLOCK_NTSC, engine.clockHz)
	}

	// Test that zero clock is rejected
	engine.SetClockHz(0)
	if engine.clockHz != TED_CLOCK_NTSC {
		t.Errorf("clock should not change to zero, got %d", engine.clockHz)
	}
}

func TestTEDFrequencyCalculation(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)
	engine.SetClockHz(TED_CLOCK_PAL)

	// Formula: freq = TED_SOUND_CLOCK_PAL / (1024 - regValue)
	tests := []struct {
		name     string
		regValue uint16
	}{
		{"mid-range register 512", 512},
		{"low register 100", 100},
		{"high register 900", 900},
		{"register 0 (lowest)", 0},
		{"register 1023 (very high)", 1023},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			freq := tedFrequencyHz(tt.regValue, TED_CLOCK_PAL)
			expected := float64(TED_SOUND_CLOCK_PAL) / float64(1024-int(tt.regValue))
			// Allow 1% tolerance for rounding
			tolerance := expected * 0.01
			if freq < expected-tolerance || freq > expected+tolerance {
				t.Errorf("frequency for reg %d = %f, want ~%f (±1%%)",
					tt.regValue, freq, expected)
			}
		})
	}
}

func TestTEDVolumeMapping(t *testing.T) {
	// TED has 4-bit volume (0-8 usable, values above 8 = max)
	tests := []struct {
		volume   uint8
		expected float32
		plus     bool
		desc     string
	}{
		{0, 0.0, false, "volume 0 = silent"},
		{8, 1.0, false, "volume 8 = max"},
		{4, 0.5, false, "volume 4 = half"},
		{15, 1.0, false, "volume 15 = max (clamped)"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			gain := tedVolumeGain(tt.volume, tt.plus)
			// Allow small tolerance for floating point
			if gain < tt.expected-0.01 || gain > tt.expected+0.01 {
				t.Errorf("volume %d gain = %f, want %f", tt.volume, gain, tt.expected)
			}
		})
	}
}

func TestTEDControlRegisterBits(t *testing.T) {
	// Verify control register bit definitions
	if TED_CTRL_SNDDC != 0x80 {
		t.Errorf("TED_CTRL_SNDDC = 0x%02X, want 0x80", TED_CTRL_SNDDC)
	}
	if TED_CTRL_SND2NOISE != 0x40 {
		t.Errorf("TED_CTRL_SND2NOISE = 0x%02X, want 0x40", TED_CTRL_SND2NOISE)
	}
	if TED_CTRL_SND2ON != 0x20 {
		t.Errorf("TED_CTRL_SND2ON = 0x%02X, want 0x20", TED_CTRL_SND2ON)
	}
	if TED_CTRL_SND1ON != 0x10 {
		t.Errorf("TED_CTRL_SND1ON = 0x%02X, want 0x10", TED_CTRL_SND1ON)
	}
	if TED_CTRL_VOLUME != 0x0F {
		t.Errorf("TED_CTRL_VOLUME = 0x%02X, want 0x0F", TED_CTRL_VOLUME)
	}
}

func TestTEDRegisterAddresses(t *testing.T) {
	// Verify register addresses match the plan
	if TED_BASE != 0xF0F00 {
		t.Errorf("TED_BASE = 0x%X, want 0xF0F00", TED_BASE)
	}
	if TED_FREQ1_LO != 0xF0F00 {
		t.Errorf("TED_FREQ1_LO = 0x%X, want 0xF0F00", TED_FREQ1_LO)
	}
	if TED_FREQ2_LO != 0xF0F01 {
		t.Errorf("TED_FREQ2_LO = 0x%X, want 0xF0F01", TED_FREQ2_LO)
	}
	if TED_FREQ2_HI != 0xF0F02 {
		t.Errorf("TED_FREQ2_HI = 0x%X, want 0xF0F02", TED_FREQ2_HI)
	}
	if TED_SND_CTRL != 0xF0F03 {
		t.Errorf("TED_SND_CTRL = 0x%X, want 0xF0F03", TED_SND_CTRL)
	}
	if TED_FREQ1_HI != 0xF0F04 {
		t.Errorf("TED_FREQ1_HI = 0x%X, want 0xF0F04", TED_FREQ1_HI)
	}
	if TED_PLUS_CTRL != 0xF0F05 {
		t.Errorf("TED_PLUS_CTRL = 0x%X, want 0xF0F05", TED_PLUS_CTRL)
	}
}

func TestTED6502Addresses(t *testing.T) {
	// Verify 6502 mapping
	if C6502_TED_BASE != 0xD600 {
		t.Errorf("C6502_TED_BASE = 0x%X, want 0xD600", C6502_TED_BASE)
	}
	if C6502_TED_END != 0xD605 {
		t.Errorf("C6502_TED_END = 0x%X, want 0xD605", C6502_TED_END)
	}
}

func TestTEDWriteRegister(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Write to voice 1 frequency low
	engine.WriteRegister(TED_REG_FREQ1_LO, 0x55)
	if engine.regs[TED_REG_FREQ1_LO] != 0x55 {
		t.Errorf("FREQ1_LO = 0x%02X, want 0x55", engine.regs[TED_REG_FREQ1_LO])
	}

	// Write to control register
	engine.WriteRegister(TED_REG_SND_CTRL, 0x1F) // Both voices on, volume 15
	if engine.regs[TED_REG_SND_CTRL] != 0x1F {
		t.Errorf("SND_CTRL = 0x%02X, want 0x1F", engine.regs[TED_REG_SND_CTRL])
	}
}

func TestTEDHandleWriteRead(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Test memory-mapped write
	engine.HandleWrite(TED_FREQ1_LO, 0xAA)
	val := engine.HandleRead(TED_FREQ1_LO)
	if val != 0xAA {
		t.Errorf("HandleRead returned 0x%02X, want 0xAA", val)
	}

	// Test out-of-range address
	engine.HandleWrite(TED_BASE-1, 0xFF)
	val = engine.HandleRead(TED_BASE - 1)
	if val != 0 {
		t.Errorf("out-of-range read should return 0, got 0x%02X", val)
	}
}

func TestTEDNoiseMode(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Enable voice 2 with noise
	ctrl := uint8(TED_CTRL_SND2ON | TED_CTRL_SND2NOISE | 8) // Voice 2 + noise + volume 8
	engine.WriteRegister(TED_REG_SND_CTRL, ctrl)

	// Verify the control register has noise bit set
	if engine.regs[TED_REG_SND_CTRL]&TED_CTRL_SND2NOISE == 0 {
		t.Error("noise mode bit not set")
	}
}

func TestTEDPlusMode(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Initially disabled
	if engine.TEDPlusEnabled() {
		t.Error("TED+ should be disabled initially")
	}

	// Enable TED+
	engine.SetTEDPlusEnabled(true)
	if !engine.TEDPlusEnabled() {
		t.Error("TED+ should be enabled")
	}

	// Disable via register write
	engine.HandleWrite(TED_PLUS_CTRL, 0)
	if engine.TEDPlusEnabled() {
		t.Error("TED+ should be disabled via register")
	}

	// Enable via register write
	engine.HandleWrite(TED_PLUS_CTRL, 1)
	if !engine.TEDPlusEnabled() {
		t.Error("TED+ should be enabled via register")
	}
}

func TestTEDEvent(t *testing.T) {
	// Test TEDEvent structure
	ev := TEDEvent{
		Cycle:  1000,
		Sample: 500,
		Reg:    TED_REG_FREQ1_LO,
		Value:  0x80,
	}

	if ev.Cycle != 1000 || ev.Sample != 500 || ev.Reg != TED_REG_FREQ1_LO || ev.Value != 0x80 {
		t.Errorf("TEDEvent fields incorrect: %+v", ev)
	}
}

func TestTEDSetEvents(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	events := []TEDEvent{
		{Sample: 0, Reg: TED_REG_SND_CTRL, Value: 0x18},   // Voice 1 on, volume 8
		{Sample: 100, Reg: TED_REG_FREQ1_LO, Value: 0x80}, // Set frequency
	}

	engine.SetEvents(events, 1000, false, 0)

	if !engine.playing {
		t.Error("engine should be playing after SetEvents")
	}
	if len(engine.events) != 2 {
		t.Errorf("expected 2 events, got %d", len(engine.events))
	}
}

func TestTEDTickSample(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	events := []TEDEvent{
		{Sample: 0, Reg: TED_REG_SND_CTRL, Value: 0x18},
		{Sample: 5, Reg: TED_REG_FREQ1_LO, Value: 0x80},
	}

	engine.SetEvents(events, 100, false, 0)

	// Tick once - first event should be applied
	engine.TickSample()
	if engine.regs[TED_REG_SND_CTRL] != 0x18 {
		t.Errorf("SND_CTRL = 0x%02X, want 0x18", engine.regs[TED_REG_SND_CTRL])
	}

	// Tick 5 more times to reach sample 5
	for i := 0; i < 5; i++ {
		engine.TickSample()
	}

	if engine.regs[TED_REG_FREQ1_LO] != 0x80 {
		t.Errorf("FREQ1_LO = 0x%02X, want 0x80", engine.regs[TED_REG_FREQ1_LO])
	}
}

func TestTEDPlaybackLoop(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	events := []TEDEvent{
		{Sample: 0, Reg: TED_REG_SND_CTRL, Value: 0x18},
	}

	engine.SetEvents(events, 10, true, 0) // Loop from start

	// Tick past the end
	for i := 0; i < 15; i++ {
		engine.TickSample()
	}

	// Should still be playing due to loop
	if !engine.IsPlaying() {
		t.Error("engine should still be playing with loop enabled")
	}
}

func TestTEDStopPlayback(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	events := []TEDEvent{
		{Sample: 0, Reg: TED_REG_SND_CTRL, Value: 0x18},
	}

	engine.SetEvents(events, 1000, false, 0)
	engine.StopPlayback()

	if engine.IsPlaying() {
		t.Error("engine should not be playing after StopPlayback")
	}
	if len(engine.events) != 0 {
		t.Errorf("events should be cleared, got %d", len(engine.events))
	}
}

func TestTEDReset(t *testing.T) {
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Set some state
	engine.WriteRegister(TED_REG_FREQ1_LO, 0xFF)
	engine.enabled = true

	// Reset
	engine.Reset()

	if engine.regs[TED_REG_FREQ1_LO] != 0 {
		t.Errorf("FREQ1_LO should be 0 after reset, got 0x%02X", engine.regs[TED_REG_FREQ1_LO])
	}
	if engine.enabled {
		t.Error("enabled should be false after reset")
	}
}
