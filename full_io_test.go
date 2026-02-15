package main

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestFullIOPath(t *testing.T) {
	binaryPath := "sdk/examples/prebuilt/robocop_intro.iex"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("robocop_intro.iex not found")
	}

	bus := NewMachineBus()

	// Track writes
	var videoCtrlWrites atomic.Int32
	var bltCtrlWrites atomic.Int32
	var copperCtrlWrites atomic.Int32
	var psgPlayPtrWrites atomic.Int32

	// Video write handler
	videoHandler := func(addr uint32, value uint32) {
		switch addr {
		case VIDEO_CTRL:
			videoCtrlWrites.Add(1)
		case BLT_CTRL:
			bltCtrlWrites.Add(1)
		case COPPER_CTRL:
			copperCtrlWrites.Add(1)
		}
	}

	// PSG write handler
	psgHandler := func(addr uint32, value uint32) {
		if addr == PSG_PLAY_PTR {
			psgPlayPtrWrites.Add(1)
			t.Logf("PSG_PLAY_PTR write: 0x%X", value)
		}
	}

	// Map I/O regions
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, nil, videoHandler)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, nil, psgHandler)

	cpu := NewCPU(bus)

	if err := cpu.LoadProgram(binaryPath); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Run CPU
	go cpu.Execute()
	time.Sleep(100 * time.Millisecond)
	cpu.running.Store(false)
	time.Sleep(10 * time.Millisecond)

	t.Logf("VIDEO_CTRL writes: %d", videoCtrlWrites.Load())
	t.Logf("BLT_CTRL writes: %d", bltCtrlWrites.Load())
	t.Logf("COPPER_CTRL writes: %d", copperCtrlWrites.Load())
	t.Logf("PSG_PLAY_PTR writes: %d", psgPlayPtrWrites.Load())

	if videoCtrlWrites.Load() == 0 {
		t.Error("No VIDEO_CTRL writes detected!")
	}
	if psgPlayPtrWrites.Load() == 0 {
		t.Error("No PSG_PLAY_PTR writes detected - music won't play!")
	}
}
