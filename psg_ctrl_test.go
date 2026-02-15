package main

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestPSGPlayCtrlWrites(t *testing.T) {
	binaryPath := "sdk/examples/prebuilt/robocop_intro.iex"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("robocop_intro.iex not found")
	}

	bus := NewMachineBus()

	// Track writes
	var psgPlayCtrlWrites atomic.Int32
	var psgPlayCtrlValue atomic.Uint32

	// PSG write handler
	psgHandler := func(addr uint32, value uint32) {
		switch addr {
		case PSG_PLAY_PTR:
			t.Logf("PSG_PLAY_PTR = 0x%X", value)
		case PSG_PLAY_LEN:
			t.Logf("PSG_PLAY_LEN = %d", value)
		case PSG_PLAY_CTRL:
			psgPlayCtrlWrites.Add(1)
			psgPlayCtrlValue.Store(value)
			t.Logf("PSG_PLAY_CTRL = %d (start=%v)", value, value&1 == 1)
		}
	}

	// Map just PSG region for this test
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, nil, psgHandler)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, nil, nil) // dummy mapping

	cpu := NewCPU(bus)

	if err := cpu.LoadProgram(binaryPath); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Run CPU briefly
	go cpu.Execute()
	time.Sleep(50 * time.Millisecond)
	cpu.running.Store(false)
	time.Sleep(10 * time.Millisecond)

	t.Logf("PSG_PLAY_CTRL writes: %d", psgPlayCtrlWrites.Load())

	if psgPlayCtrlWrites.Load() == 0 {
		t.Error("No PSG_PLAY_CTRL writes - music won't start!")
	}
}
