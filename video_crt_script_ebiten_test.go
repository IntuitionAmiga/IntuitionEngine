//go:build !headless

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func init() {
	gpuGateBodies["iescript_presentation_screenshot"] = gateIEScriptCapturesFinalPresentation
	gpuGateBodies["iescript_composition_screenshot"] = gateIEScriptCapturesGPUComposition
	gpuGateBodies["iescript_hardware_composition_screenshot"] = gateIEScriptCapturesHardwareGPUComposition
}

func TestIEScriptControlsHostCRTPresentation(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput: %v", err)
	}
	eo := out.(*EbitenOutput)
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(eo), NewTerminalMMIO())
	if err := se.RunString(`
		if not video.is_crt_enabled() then error("CRT should start enabled") end
		video.set_crt_enabled(false)
		if video.is_crt_enabled() then error("set_crt_enabled(false) failed") end
		if video.toggle_crt() ~= true then error("toggle_crt should return enabled") end
		if not video.is_crt_enabled() then error("toggle_crt did not restore CRT") end
	`, "crt-host-control"); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !eo.crtIsRequested() {
		t.Fatal("IEScript did not restore CRT")
	}
}

func TestIEScriptCapturesFinalPresentation(t *testing.T) {
	runGPUGate(t, "iescript_presentation_screenshot")
}

func gateIEScriptCapturesFinalPresentation() error {
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 4, Height: 4, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}
	frame := solidTestFrame(4, 4, 40, 120, 200, 0xFF)
	if err := eo.UpdateFrame(frame); err != nil {
		return fmt.Errorf("UpdateFrame: %w", err)
	}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(eo), NewTerminalMMIO())
	dir, err := os.MkdirTemp("", "ie-presentation-screenshot-")
	if err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "presented.png")
	if err := se.RunString(`rec.screenshot_screen("presented.png")`, filepath.Join(dir, "presentation-screenshot.ies")); err != nil {
		return fmt.Errorf("RunString: %w", err)
	}

	// The script blocks for the next Draw, which is the required ordering for a
	// screenshot of the final presentation rather than the compositor frame.
	screen := ebiten.NewImage(4, 4)
	eo.Draw(screen)
	if err := waitForScriptStop(se, 2*time.Second); err != nil {
		return err
	}
	if err := se.LastError(); err != nil {
		return fmt.Errorf("script failed: %w", err)
	}
	return validateIEScriptScreenshot(path, "presentation")
}

func TestIEScriptCapturesGPUComposition(t *testing.T) {
	runGPUGate(t, "iescript_composition_screenshot")
}

func TestIEScriptCapturesHardwareGPUCompositionBeforeCRT(t *testing.T) {
	runGPUGate(t, "iescript_hardware_composition_screenshot")
}

func gateIEScriptCapturesHardwareGPUComposition() error {
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 4, Height: 4, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}
	frame := solidTestFrame(4, 4, 40, 120, 200, 0xFF)
	if err := eo.UpdateHardwareCompositorFrame(CompositorFrameUpdate{
		FrameID: 1, PresentationWidth: 4, PresentationHeight: 4, HasContent: true,
		Layers: []CompositorFrameLayer{{SourceID: 1, SourceWidth: 4, SourceHeight: 4, DestWidth: 4, DestHeight: 4, Opaque: true, Buffer: frame}},
	}); err != nil {
		return fmt.Errorf("UpdateHardwareCompositorFrame: %w", err)
	}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(eo), NewTerminalMMIO())
	dir, err := os.MkdirTemp("", "ie-hardware-composition-screenshot-")
	if err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "hardware-composed.png")
	if err := se.RunString(`rec.screenshot_composed("hardware-composed.png")`, filepath.Join(dir, "hardware-composition-screenshot.ies")); err != nil {
		return fmt.Errorf("RunString: %w", err)
	}
	screen := ebiten.NewImage(4, 4)
	eo.Draw(screen)
	if err := waitForScriptStop(se, 2*time.Second); err != nil {
		return err
	}
	if err := se.LastError(); err != nil {
		return fmt.Errorf("script failed: %w", err)
	}
	if err := validateIEScriptScreenshotPixel(path, "hardware composition", 40, 120, 200, 255); err != nil {
		return err
	}
	final := readCRTGPUImage(screen, 4, 4)[:BYTES_PER_PIXEL]
	if final[0] == 40 && final[1] == 120 && final[2] == 200 {
		return fmt.Errorf("final hardware presentation unexpectedly bypassed CRT: %v", final)
	}
	return nil
}

func gateIEScriptCapturesGPUComposition() error {
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	eo.crtRequested = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 4, Height: 4, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}
	term := NewTerminalMMIO()
	term.mouseX.Store(0)
	term.mouseY.Store(0)
	eo.termMMIO = term
	eo.cursorImage = ebiten.NewImage(1, 1)
	eo.cursorImage.WritePixels([]byte{255, 0, 0, 255})
	frame := solidTestFrame(4, 4, 40, 120, 200, 0xFF)
	if err := eo.UpdateFrame(frame); err != nil {
		return fmt.Errorf("UpdateFrame: %w", err)
	}
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(eo), term)
	dir, err := os.MkdirTemp("", "ie-composition-screenshot-")
	if err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "composed.png")
	if err := se.RunString(`rec.screenshot_composed("composed.png")`, filepath.Join(dir, "composition-screenshot.ies")); err != nil {
		return fmt.Errorf("RunString: %w", err)
	}
	screen := ebiten.NewImage(4, 4)
	eo.Draw(screen)
	visible := readCRTGPUImage(screen, 4, 4)[:BYTES_PER_PIXEL]
	if visible[0] != 255 || visible[1] != 0 || visible[2] != 0 || visible[3] != 255 {
		return fmt.Errorf("test cursor was not present in final presentation: %v", visible)
	}
	if err := waitForScriptStop(se, 2*time.Second); err != nil {
		return err
	}
	if err := se.LastError(); err != nil {
		return fmt.Errorf("script failed: %w", err)
	}
	return validateIEScriptScreenshotPixel(path, "composition", 40, 120, 200, 255)
}

func waitForScriptStop(se *ScriptEngine, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !se.IsRunning() {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("script did not stop before timeout")
}

func validateIEScriptScreenshot(path, kind string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s screenshot: %w", kind, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("decode %s screenshot: %w", kind, err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r == 0 || g == 0 || b == 0 || a != 0xFFFF {
		return fmt.Errorf("%s screenshot pixel = (%d, %d, %d, %d)", kind, r, g, b, a)
	}
	return nil
}

func validateIEScriptScreenshotPixel(path, kind string, wantR, wantG, wantB, wantA uint8) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s screenshot: %w", kind, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("decode %s screenshot: %w", kind, err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r != uint32(wantR)*0x101 || g != uint32(wantG)*0x101 || b != uint32(wantB)*0x101 || a != uint32(wantA)*0x101 {
		return fmt.Errorf("%s screenshot pixel = (%d, %d, %d, %d), want (%d, %d, %d, %d)", kind, r, g, b, a, uint32(wantR)*0x101, uint32(wantG)*0x101, uint32(wantB)*0x101, uint32(wantA)*0x101)
	}
	return nil
}
