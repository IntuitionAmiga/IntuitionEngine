// ted_audio_debug_test.go - Diagnostic tests for TED audio pipeline

package main

import (
	"os"
	"testing"
)

func TestTEDAudioPipeline(t *testing.T) {
	tedPath := "/home/zayn/Music/HVTC/HVTC-ted/musicians/tobikomi/llama_polka.ted"
	data, err := os.ReadFile(tedPath)
	if err != nil {
		t.Skipf("TED file not found: %v", err)
	}

	t.Log("=== Step 1: Parse TED file ===")
	file, err := parseTEDFile(data)
	if err != nil {
		t.Fatalf("Failed to parse TED file: %v", err)
	}
	t.Logf("Title: %q, Author: %q", file.Title, file.Author)
	t.Logf("LoadAddr: $%04X, InitAddr: $%04X, PlayAddr: $%04X", file.LoadAddr, file.InitAddr, file.PlayAddr)

	t.Log("\n=== Step 2: Create 6502 player and load ===")
	player, err := NewTED6502Player(nil, SAMPLE_RATE)
	if err != nil {
		t.Fatalf("Failed to create player: %v", err)
	}

	if err := player.LoadFromData(data); err != nil {
		t.Fatalf("Failed to load data: %v", err)
	}
	t.Logf("Init events captured: %d", len(player.initEvents))

	// Log init events
	for i, ev := range player.initEvents {
		regName := tedRegName(ev.Reg)
		t.Logf("  Init event %d: reg=%s value=$%02X", i, regName, ev.Value)
	}

	t.Log("\n=== Step 3: Render frames and capture events ===")
	totalEvents := 0
	for frame := 0; frame < 50; frame++ {
		events, err := player.RenderFrame()
		if err != nil {
			t.Fatalf("Frame %d render failed: %v", frame, err)
		}
		if len(events) > 0 && frame < 10 {
			t.Logf("Frame %d: %d events", frame, len(events))
			for i, ev := range events {
				if i < 5 {
					regName := tedRegName(ev.Reg)
					t.Logf("  Event: reg=%s value=$%02X sample=%d", regName, ev.Value, ev.Sample)
				}
			}
		}
		totalEvents += len(events)
	}
	t.Logf("Total events from 50 frames: %d", totalEvents)

	if totalEvents == 0 {
		t.Fatal("PROBLEM: No TED events captured! 6502 code is not writing to TED registers")
	}

	t.Log("\n=== Step 4: Check event content ===")
	// Re-render and analyze events
	player.Reset()
	var ctrlEvents []TEDEvent
	var freqEvents []TEDEvent
	for frame := 0; frame < 50; frame++ {
		events, _ := player.RenderFrame()
		for _, ev := range events {
			switch ev.Reg {
			case TED_REG_SND_CTRL:
				ctrlEvents = append(ctrlEvents, ev)
			case TED_REG_FREQ1_LO, TED_REG_FREQ1_HI, TED_REG_FREQ2_LO, TED_REG_FREQ2_HI:
				freqEvents = append(freqEvents, ev)
			}
		}
	}
	t.Logf("Control register writes: %d", len(ctrlEvents))
	t.Logf("Frequency register writes: %d", len(freqEvents))

	// Check if voices are enabled
	for i, ev := range ctrlEvents {
		if i < 5 {
			vol := ev.Value & TED_CTRL_VOLUME
			v1on := (ev.Value & TED_CTRL_SND1ON) != 0
			v2on := (ev.Value & TED_CTRL_SND2ON) != 0
			noise := (ev.Value & TED_CTRL_SND2NOISE) != 0
			t.Logf("  Ctrl: vol=%d v1=%v v2=%v noise=%v", vol, v1on, v2on, noise)
		}
	}

	if len(ctrlEvents) == 0 {
		t.Fatal("PROBLEM: No control register writes - no volume/enable commands")
	}

	// Check if any voice is enabled with volume
	foundEnabled := false
	for _, ev := range ctrlEvents {
		vol := ev.Value & TED_CTRL_VOLUME
		v1on := (ev.Value & TED_CTRL_SND1ON) != 0
		v2on := (ev.Value & TED_CTRL_SND2ON) != 0
		if vol > 0 && (v1on || v2on) {
			foundEnabled = true
			break
		}
	}
	if !foundEnabled {
		t.Fatal("PROBLEM: No events enable a voice with volume > 0")
	}

	t.Log("\n=== Step 5: Test SoundChip integration ===")
	// Create engine without SoundChip first to test register processing
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Apply some events directly
	player.Reset()
	events, _ := player.RenderFrame()
	for _, ev := range events {
		engine.WriteRegister(ev.Reg, ev.Value)
	}

	// Check if channels were configured
	t.Logf("Checking SoundChip channel state...")
	// We can't easily inspect SoundChip internals, but we can verify no panics

	t.Log("\n=== All pipeline stages checked ===")
	t.Log("If no FAILs above, the events are being generated correctly.")
	t.Log("Issue may be in main.go integration or SoundChip routing.")
}

func tedRegName(reg uint8) string {
	switch reg {
	case TED_REG_FREQ1_LO:
		return "FREQ1_LO"
	case TED_REG_FREQ2_LO:
		return "FREQ2_LO"
	case TED_REG_FREQ2_HI:
		return "FREQ2_HI"
	case TED_REG_SND_CTRL:
		return "SND_CTRL"
	case TED_REG_FREQ1_HI:
		return "FREQ1_HI"
	case TED_REG_PLUS_CTRL:
		return "PLUS_CTRL"
	default:
		return "UNKNOWN"
	}
}

func TestTEDSoundChipMapping(t *testing.T) {
	// Test that TED engine correctly maps to SoundChip
	// Use nil SoundChip to test calculation without audio output
	engine := NewTEDEngine(nil, SAMPLE_RATE)

	// Calculate register value for 440 Hz
	// Formula: regValue = 1024 - TED_SOUND_CLOCK_PAL/targetFreq
	targetFreq := 440.0
	regValue := uint16(1024 - float64(TED_SOUND_CLOCK_PAL)/targetFreq)
	freqLo := uint8(regValue & 0xFF)
	freqHi := uint8((regValue >> 8) & 0x03)

	// Write TED registers for voice 1 at ~440Hz
	engine.WriteRegister(TED_REG_FREQ1_LO, freqLo)
	engine.WriteRegister(TED_REG_FREQ1_HI, freqHi)
	engine.WriteRegister(TED_REG_SND_CTRL, TED_CTRL_SND1ON|TED_MAX_VOLUME) // Voice 1 on, max volume

	// Calculate expected frequency using constants
	expectedFreq := float64(TED_SOUND_CLOCK_PAL) / float64(1024-int(regValue))
	t.Logf("Target frequency: %.0f Hz", targetFreq)
	t.Logf("Register value: %d (0x%03X)", regValue, regValue)
	t.Logf("Expected frequency: %.2f Hz", expectedFreq)

	// Verify through engine's internal state
	calcFreq := engine.calcFrequency(0)
	t.Logf("Engine calcFrequency(0): %.2f Hz", calcFreq)

	// Allow 5% tolerance due to integer register quantization
	tolerance := targetFreq * 0.05
	if calcFreq < targetFreq-tolerance || calcFreq > targetFreq+tolerance {
		t.Errorf("Frequency calculation seems wrong: got %.2f, expected ~%.0f", calcFreq, targetFreq)
	}
}
