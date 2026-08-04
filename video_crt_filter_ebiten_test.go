//go:build !headless

package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func init() {
	gpuGateBodies["crt_filter"] = gateCRTFilter
}

// The real backend gate covers both the normal framebuffer and compositor
// paths. It deliberately disables the filter first to retain an exact oracle,
// then checks the enabled Zfast output's observable scanline and fine-mask
// relationships rather than binding the test to a driver-specific rounding bit.
func TestCRTFilterGPU(t *testing.T) {
	runGPUGate(t, "crt_filter")
}

func gateCRTFilter() error {
	const width, height = 8, 8
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}
	frame := solidTestFrame(width, height, 200, 200, 200, 0xFF)
	if err := eo.UpdateFrame(frame); err != nil {
		return fmt.Errorf("UpdateFrame: %w", err)
	}

	screen := ebiten.NewImage(width, height)
	eo.crtRequested = false
	eo.Draw(screen)
	raw := readCRTGPUImage(screen, width, height)
	if !equalBytes(raw, frame) {
		return fmt.Errorf("disabled CRT changed normal framebuffer output")
	}

	eo.crtRequested = true
	eo.Draw(screen)
	filtered := readCRTGPUImage(screen, width, height)
	if equalBytes(filtered, frame) {
		return fmt.Errorf("enabled CRT left normal framebuffer byte-identical")
	}
	for i := 3; i < len(filtered); i += 4 {
		if filtered[i] != 0xFF {
			return fmt.Errorf("enabled CRT alpha at byte %d = %d, want 255", i, filtered[i])
		}
	}
	if luminance(filtered, width, 0, 0) == luminance(filtered, width, 1, 0) {
		return fmt.Errorf("fine-mask columns have equal brightness")
	}
	if luminance(filtered, width, 0, 0) == luminance(filtered, width, 0, 1) {
		return fmt.Errorf("scanline positions have equal brightness")
	}

	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{{
			SourceID: 1, SourceWidth: width, SourceHeight: height,
			DestWidth: width, DestHeight: height, Buffer: frame,
		}},
	}); err != nil {
		return fmt.Errorf("UpdateHardwareCompositorFrame: %w", err)
	}
	eo.Draw(screen)
	hardware := readCRTGPUImage(screen, width, height)
	if equalBytes(hardware, frame) {
		return fmt.Errorf("enabled CRT left hardware compositor output byte-identical")
	}
	if err := gateCRTOverlayRoutes(frame); err != nil {
		return err
	}

	// A bad Kage program latches a session fallback. The first Draw after that
	// failure must be unfiltered rather than showing a blank frame or retrying.
	fallbackOut, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput fallback: %w", err)
	}
	fallback := fallbackOut.(*EbitenOutput)
	fallback.showStatusBar = false
	if err := fallback.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig fallback: %w", err)
	}
	if err := fallback.UpdateFrame(frame); err != nil {
		return fmt.Errorf("UpdateFrame fallback: %w", err)
	}
	if fallback.initialiseCRTFilter([]byte("not valid Kage")) || fallback.crtState != crtFilterFailed {
		return fmt.Errorf("invalid Kage did not latch CRT fallback")
	}
	fallback.Draw(screen)
	if got := readCRTGPUImage(screen, width, height); !equalBytes(got, frame) {
		return fmt.Errorf("first fallback frame was not unfiltered")
	}
	return nil
}

// gateCRTOverlayRoutes proves that each exclusive overlay is staged and passed
// through the same final filter. A previous guest frame, cursor, or status bar
// cannot leak into these routes because Draw clears the staging image first.
func gateCRTOverlayRoutes(frame []byte) error {
	for _, tc := range []struct {
		name  string
		setup func(*EbitenOutput)
	}{
		{"host", func(eo *EbitenOutput) {
			eo.hostOverlay = NewHostOverlay()
			eo.hostOverlay.HostCommandStarted(HostCommandNet)
		}},
		{"lua", func(eo *EbitenOutput) { eo.luaOverlay = NewLuaOverlay(nil); eo.luaOverlay.Show() }},
		{"monitor", func(eo *EbitenOutput) {
			monitor := NewMachineMonitor(NewMachineBus())
			monitor.Activate()
			eo.monitorOverlay = NewMonitorOverlay(monitor)
		}},
	} {
		out, err := NewEbitenOutput()
		if err != nil {
			return fmt.Errorf("%s overlay output: %w", tc.name, err)
		}
		eo := out.(*EbitenOutput)
		eo.showStatusBar = true
		if err := eo.SetDisplayConfig(DisplayConfig{Width: 8, Height: 8, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
			return fmt.Errorf("%s overlay display: %w", tc.name, err)
		}
		if err := eo.UpdateFrame(frame); err != nil {
			return fmt.Errorf("%s overlay frame: %w", tc.name, err)
		}
		tc.setup(eo)
		screen := ebiten.NewImage(8, 8)
		eo.crtRequested = false
		eo.Draw(screen)
		raw := readCRTGPUImage(screen, 8, 8)
		if equalBytes(raw, frame) {
			return fmt.Errorf("%s overlay did not replace the guest route", tc.name)
		}
		eo.crtRequested = true
		eo.Draw(screen)
		if filtered := readCRTGPUImage(screen, 8, 8); equalBytes(filtered, raw) {
			return fmt.Errorf("%s overlay was not CRT filtered", tc.name)
		}
	}
	return nil
}

func luminance(pixels []byte, width, x, y int) int {
	off := (y*width + x) * 4
	return int(pixels[off]) + int(pixels[off+1]) + int(pixels[off+2])
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readCRTGPUImage(image *ebiten.Image, width, height int) []byte {
	pixels := make([]byte, width*height*4)
	image.ReadPixels(pixels)
	return pixels
}
