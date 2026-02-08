package main

import (
	"os"
	"testing"
	"time"
)

func TestPSGPlayFromMemory(t *testing.T) {
	binaryPath := "assembler/test_ay_only.iex"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("test_ay_only.iex not found")
	}

	bus := NewMachineBus()

	// Create PSG engine with test sound chip
	soundChip := newTestSoundChip()
	psgEngine := NewPSGEngine(soundChip, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	psgPlayer.AttachBus(bus)

	// Map PSG registers
	bus.MapIO(PSG_BASE, PSG_END, psgEngine.HandleRead, psgEngine.HandleWrite)
	bus.MapIO(PSG_PLUS_CTRL, PSG_PLUS_CTRL, psgEngine.HandlePSGPlusRead, psgEngine.HandlePSGPlusWrite)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, psgPlayer.HandlePlayWrite)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, nil, nil) // dummy video

	cpu := NewCPU(bus)

	if err := cpu.LoadProgram(binaryPath); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Check the AY data header in memory
	ayStart := uint32(0x2000)
	header := make([]byte, 8)
	for i := range header {
		header[i] = bus.Read8(ayStart + uint32(i))
	}
	t.Logf("AY data header at 0x%X: %q", ayStart, string(header))

	if string(header) != "ZXAYEMUL" {
		t.Errorf("Expected ZXAYEMUL header, got %q", string(header))
	}

	// Test renderAYZ80 directly to see what it returns
	ayLen := uint32(24525)
	ayData := make([]byte, ayLen)
	for i := uint32(0); i < ayLen; i++ {
		ayData[i] = bus.Read8(ayStart + i)
	}
	meta, events, total, clockHz, frameRate, loop, loopSample, _, _, err := renderAYZ80(ayData, SAMPLE_RATE)
	if err != nil {
		t.Logf("renderAYZ80 error: %v", err)
	} else {
		t.Logf("renderAYZ80 result: events=%d, total=%d, clockHz=%d, frameRate=%d, loop=%v, loopSample=%d",
			len(events), total, clockHz, frameRate, loop, loopSample)
		t.Logf("Metadata: title=%q, author=%q, system=%q", meta.Title, meta.Author, meta.System)
	}

	// Check state before running CPU
	t.Logf("Before CPU run: playBusy=%v, playErr=%v, playPtr=0x%X, playLen=%d",
		psgPlayer.playBusy, psgPlayer.playErr, psgPlayer.playPtr, psgPlayer.playLen)

	// Track HandlePlayWrite calls
	origHandler := psgPlayer.HandlePlayWrite
	var writeCount int
	var lastCtrl uint32
	wrappedHandler := func(addr uint32, value uint32) {
		writeCount++
		if addr == PSG_PLAY_CTRL {
			lastCtrl = value
			t.Logf("HandlePlayWrite: PSG_PLAY_CTRL=0x%X", value)
			t.Logf("  Before: playBusy=%v, playErr=%v, engine.playing=%v, engine.total=%d",
				psgPlayer.playBusy, psgPlayer.playErr, psgEngine.IsPlaying(), psgEngine.totalSamples)
		}
		origHandler(addr, value)
		if addr == PSG_PLAY_CTRL {
			t.Logf("  After: playBusy=%v, playErr=%v, engine.playing=%v, engine.total=%d",
				psgPlayer.playBusy, psgPlayer.playErr, psgEngine.IsPlaying(), psgEngine.totalSamples)
		}
	}
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, wrappedHandler)

	// Run CPU briefly to execute the setup code
	// Note: renderAYZ80 can take several seconds to process the Z80 code
	go cpu.Execute()
	time.Sleep(5 * time.Second) // Allow time for AY rendering
	cpu.running.Store(false)
	time.Sleep(100 * time.Millisecond)

	t.Logf("Total PSG writes: %d, lastCtrl: 0x%X", writeCount, lastCtrl)

	// Check PSG engine state
	t.Logf("PSG+ enabled: %v", psgEngine.PSGPlusEnabled())
	t.Logf("PSG player busy: %v", psgPlayer.playBusy)
	t.Logf("PSG player error: %v", psgPlayer.playErr)
	t.Logf("PSG player ptr: 0x%X", psgPlayer.playPtr)
	t.Logf("PSG player len: %d", psgPlayer.playLen)
	t.Logf("PSG engine playing: %v", psgEngine.IsPlaying())
	t.Logf("PSG engine total samples: %d", psgEngine.totalSamples)

	if psgPlayer.playErr {
		t.Error("PSG player had an error loading the data!")
	}

	if !psgEngine.IsPlaying() {
		t.Error("PSG engine not playing - music won't be heard!")
	}
}
