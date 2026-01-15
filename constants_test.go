package main

import (
	"testing"
)

func TestConstantValues(t *testing.T) {
	t.Logf("VIDEO_REG_BASE = 0x%X", VIDEO_REG_BASE)
	t.Logf("VIDEO_CTRL = 0x%X", VIDEO_CTRL)
	t.Logf("VIDEO_MODE = 0x%X", VIDEO_MODE)
	t.Logf("BLT_CTRL = 0x%X", BLT_CTRL)
	t.Logf("VIDEO_REG_END = 0x%X", VIDEO_REG_END)
	t.Logf("IO_REGION_START = 0x%X", IO_REGION_START)
	t.Logf("AUDIO_CTRL = 0x%X", AUDIO_CTRL)
	t.Logf("AUDIO_REG_END = 0x%X", AUDIO_REG_END)
	t.Logf("PSG_BASE = 0x%X", PSG_BASE)
	t.Logf("PSG_PLAY_PTR = 0x%X", PSG_PLAY_PTR)
	t.Logf("VRAM_START = 0x%X", VRAM_START)

	// Check VIDEO_CTRL is in I/O region
	if VIDEO_CTRL < IO_REGION_START {
		t.Errorf("VIDEO_CTRL (0x%X) < IO_REGION_START (0x%X) - won't be routed to bus!", VIDEO_CTRL, IO_REGION_START)
	}
}
