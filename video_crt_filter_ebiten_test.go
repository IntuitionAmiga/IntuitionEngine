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
	eo.setCRTRequested(false)
	eo.Draw(screen)
	raw := readCRTGPUImage(screen, width, height)
	if !equalBytes(raw, frame) {
		return fmt.Errorf("disabled CRT changed normal framebuffer output")
	}
	if err := gateCRTRawPresentationCopiesTransparentRGB(); err != nil {
		return err
	}

	eo.setCRTRequested(true)
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
	centreRow := height / 2
	left := (centreRow*width + 3) * BYTES_PER_PIXEL
	right := left + BYTES_PER_PIXEL
	if sameBytes(filtered[left:left+3], filtered[right:right+3]) {
		return fmt.Errorf("Guest-Advanced phosphor-mask columns are byte-identical: %v %v", filtered[left:left+4], filtered[right:right+4])
	}
	// A one-to-one framebuffer has no vertical space between guest scanlines.
	// The scaled compositor fixture below is the meaningful Guest-Advanced
	// scanline-cadence oracle.

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
	// This is a new source geometry, not the next temporal frame of the normal
	// framebuffer fixture above. Reset history so the cadence assertion is not
	// measuring deliberate phosphor persistence from another screen mode.
	eo.crtFilter.guest.history.Clear()
	eo.Draw(screen)
	hardware := readCRTGPUImage(screen, width, height)
	if equalBytes(hardware, frame) {
		return fmt.Errorf("enabled CRT left hardware compositor output byte-identical")
	}
	first := luminance(hardware, width, width/2, 0)
	withinGuestLine := luminance(hardware, width, width/2, 2)
	if first == withinGuestLine {
		return fmt.Errorf("hardware CRT lost native 2x2 scanline phase: rows 0 and 2 are equal")
	}
	if err := gateCRTTransparentLayerPreservesLower(); err != nil {
		return err
	}
	if err := gateCRTRawHardwareLayerPromotesZeroAlphaRGB(); err != nil {
		return err
	}
	if err := gateCRTRawHardwareMixedNativeLayers(); err != nil {
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
	if err := gateCRTGuestAdvancedEffects(); err != nil {
		return err
	}
	if err := gateCRTGuestAdvancedNativeScreenModes(); err != nil {
		return err
	}
	if err := gateCRTGuestAdvancedModeChangeClearsAfterglow(); err != nil {
		return err
	}
	if err := gateCRTGuestAdvancedToggleClearsAfterglow(); err != nil {
		return err
	}
	if err := gateCRTHardwareTogglePreservesGuestLayer(); err != nil {
		return err
	}
	if err := gateCRTHardwareTogglePreservesSparse320x200Layer(); err != nil {
		return err
	}
	if err := gateCRTIEMonOverlayUsesFinalCurvedPresentation(); err != nil {
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
	// This exercises the retained Zfast constructor seam with intentionally
	// invalid Kage. Guest-Advanced has its own multi-pass constructor.
	fallback.crtProfile = crtProfileZfast
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

// gateCRTIEMonOverlayUsesFinalCurvedPresentation exercises the real IEMon
// overlay route. The monitor is a host-visible screen mode as well, and must
// use the same final CRT face rather than bypassing it or inheriting a guest
// mode's temporal targets.
func gateCRTIEMonOverlayUsesFinalCurvedPresentation() error {
	const width, height = 1920, 1080
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("IEMon output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtMode = crtModeCurved
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("IEMon display: %w", err)
	}
	monitor := NewMachineMonitor(NewMachineBus())
	eo.AttachMonitor(monitor)
	monitor.Activate()
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	if got := eo.crtFilter.guest.activeModeKey; got != "monitor-overlay" {
		return fmt.Errorf("IEMon presentation mode = %q, want monitor-overlay", got)
	}
	pixels := readCRTGPUImage(screen, width, height)
	if got := luminance(pixels, width, 0, 0); got != 0 {
		return fmt.Errorf("IEMon curved CRT corner luminance = %d, want 0", got)
	}
	return nil
}

// gateCRTRawPresentationCopiesTransparentRGB covers guest framebuffers whose
// colour bytes are meaningful even when their alpha byte is zero. CRT-on uses
// a copy pass, so F7 must use the same presentation semantics rather than
// source-over blending those guest pixels away.
func gateCRTRawPresentationCopiesTransparentRGB() error {
	const width, height = 8, 8
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("transparent-RGB output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtMode = crtModeOff
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("transparent-RGB display: %w", err)
	}
	frame := solidTestFrame(width, height, 200, 120, 40, 0)
	if err := eo.UpdateFrame(frame); err != nil {
		return fmt.Errorf("transparent-RGB frame: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	if got := readCRTGPUImage(screen, width, height); !equalBytes(got, frame) {
		return fmt.Errorf("raw CRT-off presentation blended away transparent-alpha RGB")
	}
	return nil
}

func gateCRTGuestAdvancedModeChangeClearsAfterglow() error {
	const width, height = 24, 24
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("Guest-Advanced mode-change output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("Guest-Advanced mode-change display: %w", err)
	}
	if err := eo.UpdateFrame(solidTestFrame(width, height, 200, 200, 200, 0xFF)); err != nil {
		return fmt.Errorf("Guest-Advanced mode-change source frame: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	black := solidTestFrame(2, 2, 0, 0, 0, 0xFF)
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{{SourceID: 1, SourceWidth: 2, SourceHeight: 2, DestWidth: width, DestHeight: height, Opaque: true, Buffer: black}},
	}); err != nil {
		return fmt.Errorf("Guest-Advanced mode-change hardware frame: %w", err)
	}
	eo.Draw(screen)
	pixels := readCRTGPUImage(screen, width, height)
	if luminance(pixels, width, width/2, height/2) != 0 {
		return fmt.Errorf("Guest-Advanced afterglow leaked across source-mode change")
	}
	return nil
}

// gateCRTGuestAdvancedToggleClearsAfterglow covers F7's presentation path.
// Disabling CRT skips finish, so the next enabled frame must explicitly
// discard the retained phosphor history even when its source mode is unchanged.
func gateCRTGuestAdvancedToggleClearsAfterglow() error {
	const width, height = 24, 24
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("Guest-Advanced toggle output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("Guest-Advanced toggle display: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	if err := eo.UpdateFrame(solidTestFrame(width, height, 200, 200, 200, 0xFF)); err != nil {
		return fmt.Errorf("Guest-Advanced toggle bright frame: %w", err)
	}
	eo.Draw(screen)

	black := solidTestFrame(width, height, 0, 0, 0, 0xFF)
	eo.crtMode = crtModeOff
	if err := eo.UpdateFrame(black); err != nil {
		return fmt.Errorf("Guest-Advanced toggle disabled frame: %w", err)
	}
	eo.Draw(screen)
	eo.crtMode = crtModeFlat
	eo.Draw(screen)
	pixels := readCRTGPUImage(screen, width, height)
	if got := luminance(pixels, width, width/2, height/2); got != 0 {
		return fmt.Errorf("Guest-Advanced afterglow survived an off/on CRT toggle: luminance=%d", got)
	}
	return nil
}

// gateCRTHardwareTogglePreservesGuestLayer covers the retained compositor path
// used by Copper demos. F7 must only change presentation treatment: it must
// never remove the current guest layer while CRT is disabled or restored.
func gateCRTHardwareTogglePreservesGuestLayer() error {
	const width, height = 24, 24
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("hardware CRT-toggle output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("hardware CRT-toggle display: %w", err)
	}
	frame := solidTestFrame(width, height, 200, 120, 40, 0xFF)
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{{SourceID: 7, SourceWidth: width, SourceHeight: height, DestWidth: width, DestHeight: height, Opaque: true, ContentGen: 1, Buffer: frame}},
	}); err != nil {
		return fmt.Errorf("hardware CRT-toggle frame: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)

	eo.crtMode = crtModeOff
	eo.Draw(screen)
	if got := readCRTGPUImage(screen, width, height); !equalBytes(got, frame) {
		return fmt.Errorf("hardware guest layer disappeared when CRT was disabled")
	}

	eo.crtMode = crtModeFlat
	eo.Draw(screen)
	pixels := readCRTGPUImage(screen, width, height)
	if luminance(pixels, width, width/2, height/2) == 0 {
		return fmt.Errorf("hardware guest layer disappeared when CRT was re-enabled")
	}
	return nil
}

// gateCRTHardwareTogglePreservesSparse320x200Layer mirrors a wireframe demo:
// a mostly uniform 320x200 background with a sparse bright guest drawing. It
// catches a toggle path that preserves the background but loses the foreground.
func gateCRTHardwareTogglePreservesSparse320x200Layer() error {
	const sourceWidth, sourceHeight = 320, 200
	const width, height = 1920, 1080
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("sparse CRT-toggle output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("sparse CRT-toggle display: %w", err)
	}
	frame := solidTestFrame(sourceWidth, sourceHeight, 8, 24, 80, 0xFF)
	for x := 100; x < 220; x++ {
		i := (100*sourceWidth + x) * BYTES_PER_PIXEL
		frame[i], frame[i+1], frame[i+2] = 255, 255, 255
	}
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{{SourceID: 11, SourceWidth: sourceWidth, SourceHeight: sourceHeight, DestWidth: width, DestHeight: height, Opaque: true, ContentGen: 1, Buffer: frame}},
	}); err != nil {
		return fmt.Errorf("sparse CRT-toggle frame: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	for _, enabled := range []bool{true, false, true, false, true} {
		eo.setCRTRequested(enabled)
		eo.Draw(screen)
		pixels := readCRTGPUImage(screen, width, height)
		// Source row 100 occupies y=540 after 200->1080 expansion. Its bright
		// horizontal line must survive both raw and filtered presentation.
		if got := luminance(pixels, width, 960, 540); got == 0 {
			return fmt.Errorf("sparse 320x200 foreground disappeared with CRT enabled=%t", enabled)
		}
	}
	return nil
}

// gateCRTGuestAdvancedNativeScreenModes exercises the actual guest modes used
// by IE demos at their 1080p presentation size. It catches a regression where
// a filter only works on a toy integer scale but loses its source geometry on
// 320x200 or 640x480 output.
func gateCRTGuestAdvancedNativeScreenModes() error {
	const presentationWidth, presentationHeight = 1920, 1080
	for _, mode := range []struct {
		name         string
		sourceWidth  int
		sourceHeight int
		destX        int
		destY        int
		destWidth    int
		destHeight   int
	}{
		{name: "320x200", sourceWidth: 320, sourceHeight: 200, destWidth: presentationWidth, destHeight: presentationHeight},
		{name: "320x240", sourceWidth: 320, sourceHeight: 240, destWidth: presentationWidth, destHeight: presentationHeight},
		{name: "640x480", sourceWidth: 640, sourceHeight: 480, destWidth: presentationWidth, destHeight: presentationHeight},
		{name: "1024x768", sourceWidth: 1024, sourceHeight: 768, destWidth: presentationWidth, destHeight: presentationHeight},
		{name: "320x200 aspect-fit", sourceWidth: 320, sourceHeight: 200, destX: 160, destY: 40, destWidth: 1600, destHeight: 1000},
	} {
		out, err := NewEbitenOutput()
		if err != nil {
			return fmt.Errorf("Guest-Advanced %s output: %w", mode.name, err)
		}
		eo := out.(*EbitenOutput)
		eo.showStatusBar = false
		if err := eo.SetDisplayConfig(DisplayConfig{Width: presentationWidth, Height: presentationHeight, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
			return fmt.Errorf("Guest-Advanced %s display: %w", mode.name, err)
		}
		frame := solidTestFrame(mode.sourceWidth, mode.sourceHeight, 180, 180, 180, 0xFF)
		if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
			FrameID: 1, PresentationWidth: presentationWidth, PresentationHeight: presentationHeight, HasContent: true,
			Layers: []CompositorFrameLayer{{
				SourceID: 1, SourceWidth: mode.sourceWidth, SourceHeight: mode.sourceHeight,
				DestX: mode.destX, DestY: mode.destY, DestWidth: mode.destWidth, DestHeight: mode.destHeight, Opaque: true, Buffer: frame,
			}},
		}); err != nil {
			return fmt.Errorf("Guest-Advanced %s compositor frame: %w", mode.name, err)
		}
		screen := ebiten.NewImage(presentationWidth, presentationHeight)
		eo.Draw(screen)
		pixels := readCRTGPUImage(screen, presentationWidth, presentationHeight)
		x, y := mode.destX+mode.destWidth/2, mode.destY+mode.destHeight/2
		i := (y*presentationWidth + x) * BYTES_PER_PIXEL
		if pixels[i] == 0 && pixels[i+1] == 0 && pixels[i+2] == 0 {
			return fmt.Errorf("Guest-Advanced %s produced a black 1080p presentation", mode.name)
		}
		if pixels[i+3] != 0xFF {
			return fmt.Errorf("Guest-Advanced %s alpha = %d, want 255", mode.name, pixels[i+3])
		}
		// Flat CRT is the default F7 state: a solid source reaches the screen
		// corner at every full-screen native guest mode.
		if mode.destX == 0 && mode.destY == 0 && mode.destWidth == presentationWidth && mode.destHeight == presentationHeight {
			if got := luminance(pixels, presentationWidth, 0, 0); got == 0 {
				return fmt.Errorf("Guest-Advanced %s flat CRT unexpectedly blanks a corner", mode.name)
			}
		}
		if sameBytes(pixels[i:i+3], pixels[i+BYTES_PER_PIXEL:i+BYTES_PER_PIXEL+3]) {
			return fmt.Errorf("Guest-Advanced %s lost the output-space RGB mask at 1080p", mode.name)
		}
		if mode.destX != 0 || mode.destY != 0 {
			if got := pixels[3]; got != 0 {
				return fmt.Errorf("Guest-Advanced %s wrote outside its destination rect, alpha=%d", mode.name, got)
			}
		}

		// The first F7 press enables the convex face. Run it at every native
		// source geometry, rather than assuming curvature works only for 320x200.
		eo.crtMode = crtModeCurved
		eo.Draw(screen)
		curved := readCRTGPUImage(screen, presentationWidth, presentationHeight)
		if curved[i] == 0 && curved[i+1] == 0 && curved[i+2] == 0 {
			return fmt.Errorf("Guest-Advanced %s curved CRT darkened the centre", mode.name)
		}
		if mode.destX == 0 && mode.destY == 0 && mode.destWidth == presentationWidth && mode.destHeight == presentationHeight {
			if got := luminance(curved, presentationWidth, 0, 0); got != 0 {
				return fmt.Errorf("Guest-Advanced %s curved CRT lacks corner blanking: luminance=%d", mode.name, got)
			}
		}

		// The hardware compositor remains the source of truth when F7 changes
		// presentation treatment. Exercise the actual 1080p source geometry so
		// a toggle cannot discard a 320x200 or 640x480 guest layer.
		eo.setCRTRequested(false)
		eo.Draw(screen)
		raw := readCRTGPUImage(screen, presentationWidth, presentationHeight)
		if raw[i] == 0 && raw[i+1] == 0 && raw[i+2] == 0 {
			return fmt.Errorf("Guest-Advanced %s lost the guest layer when CRT was disabled", mode.name)
		}
		eo.setCRTRequested(true)
		eo.Draw(screen)
		reenabled := readCRTGPUImage(screen, presentationWidth, presentationHeight)
		if reenabled[i] == 0 && reenabled[i+1] == 0 && reenabled[i+2] == 0 {
			return fmt.Errorf("Guest-Advanced %s lost the guest layer when CRT was re-enabled", mode.name)
		}
	}
	return nil
}

// gateCRTGuestAdvancedEffects pins the properties that distinguish the default
// pipeline from Zfast: RGB phosphor phases, spatial bloom and frame-to-frame
// persistence. Values are intentionally relationship checks, not a brittle
// whole-image golden tied to one GPU driver's rounding.
func gateCRTGuestAdvancedEffects() error {
	const width, height = 24, 24
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("Guest-Advanced output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if eo.crtProfile != crtProfileGuestAdvanced {
		return fmt.Errorf("Guest-Advanced is not the default CRT profile")
	}
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("Guest-Advanced display: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	white := solidTestFrame(width, height, 180, 180, 180, 0xFF)
	if err := eo.UpdateFrame(white); err != nil {
		return fmt.Errorf("Guest-Advanced white frame: %w", err)
	}
	eo.Draw(screen)
	masked := readCRTGPUImage(screen, width, height)
	p0 := masked[(12*width+3)*BYTES_PER_PIXEL:]
	p1 := masked[(12*width+4)*BYTES_PER_PIXEL:]
	p2 := masked[(12*width+5)*BYTES_PER_PIXEL:]
	if !(p0[0] > p0[1] && p0[0] > p0[2] && p1[1] > p1[0] && p1[1] > p1[2] && p2[2] > p2[0] && p2[2] > p2[1]) {
		return fmt.Errorf("Guest-Advanced RGB phosphor phases missing: %v %v %v", p0[:3], p1[:3], p2[:3])
	}

	black := solidTestFrame(width, height, 0, 0, 0, 0xFF)
	if err := eo.UpdateFrame(black); err != nil {
		return fmt.Errorf("Guest-Advanced black frame: %w", err)
	}
	eo.Draw(screen)
	persisted := readCRTGPUImage(screen, width, height)
	if luminance(persisted, width, 12, 12) == 0 {
		return fmt.Errorf("Guest-Advanced afterglow did not retain light into the next frame")
	}
	if err := gateCRTGuestAdvancedBloom(); err != nil {
		return err
	}
	if err := gateCRTGuestAdvancedPreparatoryPasses(); err != nil {
		return err
	}
	if err := gateCRTGuestAdvancedCurvedGlowAlignment(); err != nil {
		return err
	}
	return nil
}

// gateCRTGuestAdvancedCurvedGlowAlignment isolates source 2 of the final
// pass. A one-pixel Gaussian contribution is placed only at the coordinate
// reached by the convex warp; sampling source 2 at the unwarped destination
// must therefore leave the assertion black.
func gateCRTGuestAdvancedCurvedGlowAlignment() error {
	const width, height = 64, 64
	g, err := newGuestAdvancedCRT()
	if err != nil {
		return fmt.Errorf("Guest-Advanced final shader: %w", err)
	}
	defer g.disposeTargets()
	base := ebiten.NewImage(width, height)
	base.WritePixels(solidTestFrame(width, height, 0, 0, 0, 0xFF))
	bloom := ebiten.NewImage(width, height)
	bloom.WritePixels(solidTestFrame(width, height, 0, 0, 0, 0xFF))
	glow := ebiten.NewImage(width, height)
	const outputX, outputY = 4, 4
	// Deliberately use the upstream-supported 0.20 range here rather than the
	// subtle production default. It separates the two samples by multiple
	// texels, making this a true source-coordinate regression rather than a
	// filter-footprint test.
	const fixtureCurvature float32 = 0.20
	warpedX, warpedY := guestAdvancedWarpUV(float32(outputX)/width, float32(outputY)/height, fixtureCurvature, fixtureCurvature, guestAdvancedCurvatureShape)
	sampleX, sampleY := int(warpedX*width), int(warpedY*height)
	if sampleX == outputX || sampleY == outputY {
		return fmt.Errorf("curved-glow fixture did not move output (%d,%d): sampled (%d,%d)", outputX, outputY, sampleX, sampleY)
	}
	glowPixels := make([]byte, width*height*BYTES_PER_PIXEL)
	i := (sampleY*width + sampleX) * BYTES_PER_PIXEL
	glowPixels[i], glowPixels[i+1], glowPixels[i+2], glowPixels[i+3] = 255, 255, 255, 255
	glow.WritePixels(glowPixels)

	screen := ebiten.NewImage(width, height)
	op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{
		"BloomStrength": float32(0), "GlowStrength": float32(1), "MaskStrength": float32(0),
		"TexelSize": []float32{1 / float32(width), 1 / float32(height)}, "ScreenSize": []float32{width, height},
		"CurvatureX": fixtureCurvature, "CurvatureY": fixtureCurvature, "CurvatureShape": guestAdvancedCurvatureShape,
	}}
	op.Images[0], op.Images[1], op.Images[2] = base, bloom, glow
	screen.DrawRectShader(width, height, g.final, op)
	if got := luminance(readCRTGPUImage(screen, width, height), width, outputX, outputY); got == 0 {
		return fmt.Errorf("curved Gaussian glow detached from warped output (%d,%d)", outputX, outputY)
	}
	return nil
}

// gateCRTGuestAdvancedPreparatoryPasses proves the stages omitted from the
// original six-pass port are independently populated before final masking.
// A single bright pixel gives each stage a deterministic non-zero contribution
// without tying the test to a particular driver's rounded final colour.
func gateCRTGuestAdvancedPreparatoryPasses() error {
	const width, height = 24, 24
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("Guest-Advanced preparation output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("Guest-Advanced preparation display: %w", err)
	}
	frame := solidTestFrame(width, height, 0, 0, 0, 0xFF)
	centre := (12*width + 12) * BYTES_PER_PIXEL
	frame[centre], frame[centre+1], frame[centre+2] = 255, 180, 80
	if err := eo.UpdateFrame(frame); err != nil {
		return fmt.Errorf("Guest-Advanced preparation frame: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	g := eo.crtFilter.guest
	if luminance(readCRTGPUImage(g.prepared, width, height), width, 12, 12) == 0 {
		return fmt.Errorf("Guest-Advanced pre-afterglow pass is empty")
	}
	if readCRTGPUImage(g.averageLuminance, width, height)[centre+3] == 0 {
		return fmt.Errorf("Guest-Advanced average-luminance pass did not publish scene luminance")
	}
	if luminance(readCRTGPUImage(g.gaussianHorizontal, width, height), width, 13, 12) == 0 {
		return fmt.Errorf("Guest-Advanced horizontal Gaussian glow is empty")
	}
	if luminance(readCRTGPUImage(g.gaussianVertical, width, height), width, 13, 13) == 0 {
		return fmt.Errorf("Guest-Advanced vertical Gaussian glow is empty")
	}
	if luminance(readCRTGPUImage(g.bloomVertical, width, height), width, 15, 14) == 0 {
		return fmt.Errorf("Guest-Advanced wide bloom did not reach its supported sample location")
	}
	if luminance(readCRTGPUImage(g.gaussianVertical, width, height), width, 15, 14) != 0 {
		return fmt.Errorf("Guest-Advanced Gaussian glow reaches the wide-bloom-only sample location")
	}
	return nil
}

func gateCRTGuestAdvancedBloom() error {
	const width, height = 24, 24
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("Guest-Advanced bloom output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("Guest-Advanced bloom display: %w", err)
	}
	frame := solidTestFrame(width, height, 0, 0, 0, 0xFF)
	centre := (12*width + 12) * BYTES_PER_PIXEL
	frame[centre], frame[centre+1], frame[centre+2] = 255, 255, 255
	if err := eo.UpdateFrame(frame); err != nil {
		return fmt.Errorf("Guest-Advanced bloom frame: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	pixels := readCRTGPUImage(screen, width, height)
	if luminance(pixels, width, 15, 12) == 0 {
		return fmt.Errorf("Guest-Advanced bloom did not spread bright-phosphor light")
	}
	if luminance(pixels, width, 15, 12) >= luminance(pixels, width, 12, 12) {
		return fmt.Errorf("Guest-Advanced bloom is not lower than its bright source")
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
	cursor.crtMode = crtModeFlat
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
	status.crtMode = crtModeFlat
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
	eo.crtMode = crtModeFlat
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
	eo.crtMode = crtModeFlat
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

// gateCRTRawHardwareLayerPromotesZeroAlphaRGB pins the software compositor's
// compatibility rule for old guest renderers: non-black RGB with no alpha is
// visible, while a fully zero pixel is transparent. Copper wireframes use this
// representation. It must remain true while CRT is disabled because F7 only
// changes the presentation filter, never guest-layer visibility.
func gateCRTRawHardwareLayerPromotesZeroAlphaRGB() error {
	const width, height = 8, 8
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("zero-alpha hardware output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtMode = crtModeOff
	if err := eo.SetDisplayConfig(DisplayConfig{Width: width, Height: height, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("zero-alpha hardware display: %w", err)
	}
	lower := solidTestFrame(2, 2, 12, 24, 48, 0xFF)
	upper := make([]byte, 2*2*BYTES_PER_PIXEL)
	// Guest pixels are RGBA. This bright non-black pixel has zero alpha, which
	// the software compositor promotes to opaque before blending it.
	upper[0], upper[1], upper[2] = 240, 180, 60
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: width, PresentationHeight: height, HasContent: true,
		Layers: []CompositorFrameLayer{
			{SourceID: 1, SourceWidth: 2, SourceHeight: 2, DestWidth: width, DestHeight: height, Opaque: true, Buffer: lower},
			{SourceID: 2, SourceWidth: 2, SourceHeight: 2, DestWidth: width, DestHeight: height, Buffer: upper},
		},
	}); err != nil {
		return fmt.Errorf("zero-alpha hardware update: %w", err)
	}
	screen := ebiten.NewImage(width, height)
	eo.Draw(screen)
	got := readCRTGPUImage(screen, width, height)
	if got[0] != 240 || got[1] != 180 || got[2] != 60 || got[3] != 0xFF {
		return fmt.Errorf("raw hardware compositor lost zero-alpha RGB pixel: got %v", got[:4])
	}
	return nil
}

// gateCRTRawHardwareMixedNativeLayers is the Copper/VGA presentation shape:
// an opaque 960x540 Copper layer under a 320x200 foreground. The raw F7 path
// must draw the foreground after the background at its native geometry.
func gateCRTRawHardwareMixedNativeLayers() error {
	const presentationWidth, presentationHeight = 1920, 1080
	const backgroundWidth, backgroundHeight = 960, 540
	const foregroundWidth, foregroundHeight = 320, 200
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("mixed native-layer output: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtMode = crtModeOff
	if err := eo.SetDisplayConfig(DisplayConfig{Width: presentationWidth, Height: presentationHeight, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("mixed native-layer display: %w", err)
	}
	background := solidTestFrame(backgroundWidth, backgroundHeight, 12, 36, 72, 0)
	foreground := make([]byte, foregroundWidth*foregroundHeight*BYTES_PER_PIXEL)
	for i := 3; i < len(foreground); i += BYTES_PER_PIXEL {
		foreground[i] = 0xFF
	}
	// A bright 320x200 source pixel maps to the centre of the 1080p output.
	center := (100*foregroundWidth + 160) * BYTES_PER_PIXEL
	foreground[center], foreground[center+1], foreground[center+2] = 250, 200, 50
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: presentationWidth, PresentationHeight: presentationHeight, HasContent: true,
		Layers: []CompositorFrameLayer{
			{SourceID: 1, SourceWidth: backgroundWidth, SourceHeight: backgroundHeight, DestWidth: presentationWidth, DestHeight: presentationHeight, Opaque: true, Buffer: background},
			{SourceID: 2, SourceWidth: foregroundWidth, SourceHeight: foregroundHeight, DestWidth: presentationWidth, DestHeight: presentationHeight, Buffer: foreground},
		},
	}); err != nil {
		return fmt.Errorf("mixed native-layer update: %w", err)
	}
	screen := ebiten.NewImage(presentationWidth, presentationHeight)
	eo.Draw(screen)
	got := readCRTGPUImage(screen, presentationWidth, presentationHeight)
	i := (presentationHeight/2*presentationWidth + presentationWidth/2) * BYTES_PER_PIXEL
	if got[i] != 250 || got[i+1] != 200 || got[i+2] != 50 || got[i+3] != 0xFF {
		return fmt.Errorf("raw mixed native layers lost 320x200 foreground: got %v", got[i:i+4])
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
		eo.crtMode = crtModeOff
		eo.Draw(screen)
		raw := readCRTGPUImage(screen, 8, 8)
		if equalBytes(raw, frame) {
			return fmt.Errorf("%s overlay did not replace the guest route", tc.name)
		}
		eo.crtMode = crtModeFlat
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
