package main

import (
	"testing"
)

func TestIOMapping(t *testing.T) {
	bus := NewSystemBus()

	writesCaptured := 0
	testHandler := func(addr uint32, value uint32) {
		writesCaptured++
		t.Logf("Handler called: addr=0x%X, value=0x%X", addr, value)
	}

	// Map VIDEO_CTRL region
	t.Logf("Mapping VIDEO_CTRL (0x%X) to VIDEO_REG_END (0x%X)", VIDEO_CTRL, VIDEO_REG_END)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, nil, testHandler)

	// Check what pages were mapped
	t.Logf("PAGE_MASK = 0x%X", PAGE_MASK)
	t.Logf("VIDEO_CTRL & PAGE_MASK = 0x%X", VIDEO_CTRL&PAGE_MASK)

	// Write to VIDEO_CTRL
	t.Log("Writing to VIDEO_CTRL...")
	bus.Write32(VIDEO_CTRL, 0x12345678)

	// Write to BLT_CTRL
	t.Logf("Writing to BLT_CTRL (0x%X)...", BLT_CTRL)
	bus.Write32(BLT_CTRL, 0xDEADBEEF)

	t.Logf("Total writes captured: %d", writesCaptured)

	if writesCaptured == 0 {
		t.Error("No writes were captured by the I/O handler!")
	}
}
