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

	// A native 2x2 source expanded to 8x8 is the regression for the bug this
	// filter fixes: final-presentation CRT sees only eight already-scaled rows,
	// while the layer CRT must retain the four-pixel guest-line cadence.
	native := solidTestFrame(2, 2, 200, 200, 200, 0xFF)
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{{
			SourceID: 1, SourceWidth: 2, SourceHeight: 2,
			DestWidth: width, DestHeight: height, Buffer: native,
		}},
	}); err != nil {
		return fmt.Errorf("UpdateHardwareCompositorFrame: %w", err)
	}
	eo.Draw(screen)
	hardware := readCRTGPUImage(screen, width, height)
	if equalBytes(hardware, frame) {
		return fmt.Errorf("enabled CRT left hardware compositor output byte-identical")
	}
	first := luminance(hardware, width, 0, 0)
	withinGuestLine := luminance(hardware, width, 0, 2)
	nextGuestLine := luminance(hardware, width, 0, 4)
	if first == withinGuestLine {
		return fmt.Errorf("hardware CRT lost native 2x2 scanline phase: rows 0 and 2 are equal")
	}
	if first != nextGuestLine {
		return fmt.Errorf("hardware CRT scanline phase did not repeat after one 2x2 guest line: rows 0=%d and 4=%d", first, nextGuestLine)
	}
	if err := gateCRTTransparentLayerPreservesLower(); err != nil {
		return err
	}
	if err := gateCRTOpaqueLayerForcesAlpha(); err != nil {
		return err
	}
	if err := gateCRTHardwarePostCompositorElements(); err != nil {
		return err
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

// gateCRTHardwarePostCompositorElements proves the cursor and status bar are
// filtered independently after a hardware guest layer has received its native
// source-scaled CRT pass. A black guest leaves no other pixels that can make
// either assertion pass accidentally.
func gateCRTHardwarePostCompositorElements() error {
	const width, height = 64, 64
	black := solidTestFrame(2, 2, 0, 0, 0, 0)
	update := CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{{SourceID: 1, SourceWidth: 2, SourceHeight: 2, DestWidth: width, DestHeight: height, Opaque: true, Buffer: black}},
	}

	// A deliberately mid-grey cursor avoids saturation, so the Zfast weight is
	// observable at the exact cursor location.
	cursorOut, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("hardware cursor output: %w", err)
	}
	cursor := cursorOut.(*EbitenOutput)
	cursor.showStatusBar = false
	cursor.crtRequested = true
	if err := cursor.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("hardware cursor display: %w", err)
	}
	cursor.cursorImage = ebiten.NewImage(1, 1)
	cursor.cursorImage.WritePixels([]byte{100, 100, 100, 0xFF})
	cursor.termMMIO = NewTerminalMMIO()
	cursor.termMMIO.mouseX.Store(3)
	cursor.termMMIO.mouseY.Store(3)
	if err := cursor.UpdateHardwareCompositorFrame(update); err != nil {
		return fmt.Errorf("hardware cursor update: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	cursor.Draw(screen)
	cursorPixels := readCRTGPUImage(screen, width, height)
	if got := cursorPixels[(3*width+3)*BYTES_PER_PIXEL : (3*width+4)*BYTES_PER_PIXEL]; sameBytes(got, []byte{100, 100, 100, 0xFF}) {
		return fmt.Errorf("hardware CRT cursor bypassed the filter: %v", got)
	}

	statusOut, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("hardware status output: %w", err)
	}
	status := statusOut.(*EbitenOutput)
	status.showStatusBar = true
	status.crtRequested = true
	if err := status.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("hardware status display: %w", err)
	}
	if err := status.UpdateHardwareCompositorFrame(update); err != nil {
		return fmt.Errorf("hardware status update: %w", err)
	}
	status.Draw(screen)
	filteredStatus := readCRTGPUImage(screen, width, height)
	if status.statusBarImage == nil {
		return fmt.Errorf("hardware status bar was not drawn")
	}
	rawStatus := ebiten.NewImage(width, height)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, float64(height-status.statusBarImage.Bounds().Dy()))
	rawStatus.DrawImage(status.statusBarImage, op)
	if got := readCRTGPUImage(rawStatus, width, height); equalBytes(filteredStatus, got) {
		return fmt.Errorf("hardware CRT status bar bypassed the filter")
	}
	return nil
}

// gateCRTOpaqueLayerForcesAlpha preserves the compositor's opaque-layer
// contract. Source alpha is not meaningful for these layers: black is still a
// real opaque guest pixel, so Zfast must write alpha 255 just as the copy
// shader does.
func gateCRTOpaqueLayerForcesAlpha() error {
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("opaque CRT output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtRequested = true
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 2, Height: 2, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("opaque CRT display: %w", err)
	}
	pixels := solidTestFrame(2, 2, 120, 80, 40, 0)
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: 2, PresentationHeight: 2, HasContent: true,
		Layers: []CompositorFrameLayer{{SourceID: 1, SourceWidth: 2, SourceHeight: 2, DestWidth: 2, DestHeight: 2, Opaque: true, Buffer: pixels}},
	}); err != nil {
		return fmt.Errorf("opaque CRT update: %w", err)
	}
	screen := ebiten.NewImage(2, 2)
	eo.Draw(screen)
	got := readCRTGPUImage(screen, 2, 2)
	for i := 3; i < len(got); i += BYTES_PER_PIXEL {
		if got[i] != 0xFF {
			return fmt.Errorf("opaque CRT alpha at byte %d = %d, want 255", i, got[i])
		}
	}
	return nil
}

// gateCRTTransparentLayerPreservesLower covers the compositor contract that
// the CRT path replaced: a transparent upper-layer texel must discard, rather
// than copy black over a filtered texel from a lower guest layer.
func gateCRTTransparentLayerPreservesLower() error {
	const width, height = 8, 8
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("transparent layer output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtRequested = true
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("transparent layer display: %w", err)
	}
	lower := solidTestFrame(2, 2, 160, 120, 80, 0xFF)
	lowerLayer := CompositorFrameLayer{SourceID: 1, SourceWidth: 2, SourceHeight: 2, DestWidth: width, DestHeight: height, Buffer: lower}
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true, Layers: []CompositorFrameLayer{lowerLayer}}); err != nil {
		return fmt.Errorf("transparent layer lower update: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	lowerOnly := readCRTGPUImage(screen, width, height)

	upper := make([]byte, 2*2*BYTES_PER_PIXEL) // all texels transparent
	upperLayer := CompositorFrameLayer{SourceID: 2, SourceWidth: 2, SourceHeight: 2, DestWidth: width, DestHeight: height, Buffer: upper}
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{FrameID: 2, PresentationWidth: width, PresentationHeight: height, HasContent: true, Layers: []CompositorFrameLayer{lowerLayer, upperLayer}}); err != nil {
		return fmt.Errorf("transparent layer combined update: %w", err)
	}
	eo.Draw(screen)
	combined := readCRTGPUImage(screen, width, height)
	if !equalBytes(combined, lowerOnly) {
		return fmt.Errorf("transparent CRT layer overwrote the filtered lower layer")
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
