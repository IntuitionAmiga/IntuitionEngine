package main

import "testing"

func newVGABusTestRig(t *testing.T) (*MachineBus, *VGAEngine) {
	t.Helper()
	bus := NewMachineBus()
	vga := NewVGAEngine(bus)
	bus.MapIO(VGA_BASE, VGA_REG_END, vga.HandleRead, vga.HandleWrite)
	bus.MapIO(VGA_VRAM_WINDOW, VGA_VRAM_WINDOW+VGA_VRAM_SIZE-1, vga.HandleVRAMRead, vga.HandleVRAMWrite)
	bus.MapIO(VGA_TEXT_WINDOW, VGA_TEXT_WINDOW+VGA_TEXT_SIZE-1, vga.HandleTextRead, vga.HandleTextWrite)
	return bus, vga
}

func TestVGA_Bus_RegisterRoundTrip(t *testing.T) {
	bus, _ := newVGABusTestRig(t)

	tests := []struct {
		addr  uint32
		value uint32
	}{
		{VGA_MODE, VGA_MODE_13H},
		{VGA_CTRL, VGA_CTRL_ENABLE},
		{VGA_SEQ_INDEX, VGA_SEQ_MAPMASK_R},
		{VGA_SEQ_MAPMASK, 0x07},
		{VGA_CRTC_INDEX, VGA_CRTC_START_HI},
		{VGA_CRTC_STARTHI, 0x12},
		{VGA_GC_INDEX, VGA_GC_READ_MAP_R},
		{VGA_GC_READMAP, 0x02},
		{VGA_ATTR_INDEX, VGA_ATTR_PLANE_EN},
		{VGA_ATTR_DATA, 0x03},
		{VGA_DAC_MASK, 0x3F},
		{VGA_DAC_RINDEX, 0x04},
		{VGA_DAC_WINDEX, 0x05},
	}

	for _, tt := range tests {
		bus.Write32(tt.addr, tt.value)
		if got := bus.Read32(tt.addr); got != tt.value {
			t.Fatalf("bus register 0x%X = 0x%X, want 0x%X", tt.addr, got, tt.value)
		}
	}
}

func TestVGA_Bus_VRAMRoundTrip_A0000(t *testing.T) {
	bus, _ := newVGABusTestRig(t)
	bus.Write32(VGA_MODE, VGA_MODE_13H)
	bus.Write32(VGA_VRAM_WINDOW+0x1234, 0xAB)
	if got := bus.Read32(VGA_VRAM_WINDOW + 0x1234); got != 0xAB {
		t.Fatalf("bus VRAM read = 0x%X, want 0xAB", got)
	}
}

func TestVGA_Bus_TextRoundTrip_B8000(t *testing.T) {
	bus, _ := newVGABusTestRig(t)
	bus.Write32(VGA_TEXT_WINDOW+0x20, 'Q')
	if got := bus.Read32(VGA_TEXT_WINDOW + 0x20); got != 'Q' {
		t.Fatalf("bus text read = 0x%X, want 'Q'", got)
	}
}
