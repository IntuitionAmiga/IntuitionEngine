package main

import "testing"

func TestRasterBandDraw(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	color := uint32(0x11223344)

	bus.Write32(VIDEO_RASTER_Y, 10)
	bus.Write32(VIDEO_RASTER_HEIGHT, 3)
	bus.Write32(VIDEO_RASTER_COLOR, color)
	bus.Write32(VIDEO_RASTER_CTRL, rasterCtrlStart)

	for y := 10; y < 13; y++ {
		for _, x := range []int{0, 100, mode.width - 1} {
			addr := vramAddr(mode, x, y)
			if got := video.HandleRead(addr); got != color {
				t.Fatalf("expected raster color at %d,%d", x, y)
			}
		}
	}
}
