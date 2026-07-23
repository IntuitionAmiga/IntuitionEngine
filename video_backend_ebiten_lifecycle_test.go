//go:build !headless

package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestEbitenOutput_UpdateFrame_RejectsWrongSize(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	want := eo.width * eo.height * 4
	if err := eo.UpdateFrame(make([]byte, want)); err != nil {
		t.Fatalf("valid frame rejected: %v", err)
	}
	if err := eo.UpdateFrame(make([]byte, want-1)); err == nil {
		t.Fatal("short frame was accepted")
	}
	if err := eo.UpdateFrame(make([]byte, want+1)); err == nil {
		t.Fatal("long frame was accepted")
	}
}

func TestEbitenOutput_HardwareCompositor_ValidatesLayer(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	update := CompositorFrameUpdate{
		FrameID:            1,
		PresentationWidth:  eo.width,
		PresentationHeight: eo.height,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  2,
			SourceHeight: 2,
			DestWidth:    4,
			DestHeight:   4,
			Buffer:       make([]byte, 2*2*4-1),
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err == nil {
		t.Fatal("short hardware layer was accepted")
	}
}

func TestEbitenOutput_HardwareCompositor_StagesAndUpdateFrameClears(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	pixels := solidTestFrame(2, 2, 1, 2, 3, 0xFF)
	update := CompositorFrameUpdate{
		FrameID:            7,
		PresentationWidth:  eo.width,
		PresentationHeight: eo.height,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  2,
			SourceHeight: 2,
			DestWidth:    4,
			DestHeight:   4,
			Buffer:       pixels,
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
		t.Fatalf("UpdateHardwareCompositorFrame returned error: %v", err)
	}
	pixels[0] = 99
	if eo.hwFrameID != 7 || len(eo.hwLayers) == 0 {
		t.Fatalf("hardware frame not staged")
	}
	if got := eo.hwLayers[0].Buffer[0]; got != 1 {
		t.Fatalf("hardware buffer aliased caller memory: got %d", got)
	}

	want := eo.width * eo.height * 4
	if err := eo.UpdateFrame(make([]byte, want)); err != nil {
		t.Fatalf("UpdateFrame returned error: %v", err)
	}
	if eo.hwFrameID != 0 {
		t.Fatalf("UpdateFrame did not clear hardware frame: %d", eo.hwFrameID)
	}
}

func TestEbitenOutput_HardwareCompositor_StagesOpaquePixelsForDrawImage(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	pixels := []byte{
		0, 0, 0, 0,
		1, 2, 3, 0,
		4, 5, 6, 7,
	}
	update := CompositorFrameUpdate{
		FrameID:            8,
		PresentationWidth:  eo.width,
		PresentationHeight: eo.height,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  3,
			SourceHeight: 1,
			DestWidth:    3,
			DestHeight:   1,
			Buffer:       pixels,
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
		t.Fatalf("UpdateHardwareCompositorFrame returned error: %v", err)
	}
	got := eo.hwLayers[0].Buffer
	if got[3] != 0 {
		t.Fatalf("transparent black alpha changed: got %d", got[3])
	}
	if got[7] != 0xFF {
		t.Fatalf("zero-alpha colour was not promoted to opaque: got %d", got[7])
	}
	if got[11] != 7 {
		t.Fatalf("partial alpha changed: got %d", got[11])
	}
}

func TestEbitenOutput_HardwareCompositor_FallbackDoesNotReuseReleasedLeaseBuffer(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	ring := NewVideoFrameLeaseRing(1, BYTES_PER_PIXEL)
	lease, ok := ring.Acquire()
	if !ok {
		t.Fatal("failed to acquire initial lease")
	}
	copy(lease.Pixels(), []byte{1, 2, 3, 0xFF})

	leasedUpdate := CompositorFrameUpdate{
		FrameID:            10,
		PresentationWidth:  eo.width,
		PresentationHeight: eo.height,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  1,
			SourceHeight: 1,
			DestWidth:    1,
			DestHeight:   1,
			Buffer:       lease.Pixels(),
			Lease:        lease,
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(leasedUpdate); err != nil {
		t.Fatalf("leased UpdateHardwareCompositorFrame returned error: %v", err)
	}
	lease.Release()

	fallbackPixels := []byte{4, 5, 6, 0xFF}
	fallbackUpdate := CompositorFrameUpdate{
		FrameID:            11,
		PresentationWidth:  eo.width,
		PresentationHeight: eo.height,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  1,
			SourceHeight: 1,
			DestWidth:    1,
			DestHeight:   1,
			Buffer:       fallbackPixels,
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(fallbackUpdate); err != nil {
		t.Fatalf("fallback UpdateHardwareCompositorFrame returned error: %v", err)
	}
	staged := eo.hwLayers[0].Buffer
	reused, ok := ring.Acquire()
	if !ok {
		t.Fatal("released lease slot was not reusable after fallback update")
	}
	defer reused.Release()
	copy(reused.Pixels(), []byte{99, 88, 77, 0xFF})

	if staged[0] != 4 || staged[1] != 5 || staged[2] != 6 || staged[3] != 0xFF {
		t.Fatalf("fallback staged buffer was overwritten through released lease: got %v", staged[:BYTES_PER_PIXEL])
	}
}

func TestEbitenOutput_SetDisplayConfig_ClearsHardwareFrame(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	update := CompositorFrameUpdate{
		FrameID:            7,
		PresentationWidth:  eo.width,
		PresentationHeight: eo.height,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  1,
			SourceHeight: 1,
			DestWidth:    1,
			DestHeight:   1,
			Buffer:       solidTestFrame(1, 1, 1, 2, 3, 0xFF),
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
		t.Fatalf("UpdateHardwareCompositorFrame returned error: %v", err)
	}
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 320, Height: 240, Scale: 1}); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
	}
	if eo.hwFrameID != 0 {
		t.Fatalf("SetDisplayConfig did not clear hardware frame: %d", eo.hwFrameID)
	}
}

// These three checks draw through the real backend and read the pixels back,
// which Ebiten only permits from the main OS thread inside a running game. They
// therefore run as registered gate bodies in a re-executed copy of this binary;
// see video_gpu_convert_gate.go. Before that they panicked with "ReadPixels
// cannot be called before the game starts".
func init() {
	gpuGateBodies["hw_non16x9"] = gateHardwareNon16x9FillsStretchRect
	gpuGateBodies["hw_noninteger"] = gateHardwareNonIntegerScaleMatchesSoftwareFloor
	gpuGateBodies["hw_partialalpha"] = gateHardwarePartialAlphaLayerReplacesLowerLayer
}

func TestEbitenOutput_HardwareCompositor_Non16x9FillsStretchRect(t *testing.T) {
	runGPUGate(t, "hw_non16x9")
}

func gateHardwareNon16x9FillsStretchRect() error {
	const (
		srcW = 320
		srcH = 200
		dstW = 1920
		dstH = 1080
	)
	frame := make([]byte, srcW*srcH*BYTES_PER_PIXEL)
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			i := (y*srcW + x) * BYTES_PER_PIXEL
			frame[i+0] = byte(1 + (x*3+y*5)%255)
			frame[i+1] = byte(1 + (x*7+y*11)%255)
			frame[i+2] = byte(1 + (x*13+y*17)%255)
			frame[i+3] = 0xFF
		}
	}

	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: dstW, Height: dstH, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}
	update := CompositorFrameUpdate{
		FrameID:            1,
		PresentationWidth:  dstW,
		PresentationHeight: dstH,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  srcW,
			SourceHeight: srcH,
			DestWidth:    dstW,
			DestHeight:   dstH,
			Buffer:       frame,
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
		return fmt.Errorf("UpdateHardwareCompositorFrame: %w", err)
	}
	screen := ebiten.NewImage(dstW, dstH)
	eo.Draw(screen)
	got := make([]byte, dstW*dstH*BYTES_PER_PIXEL)
	screen.ReadPixels(got)

	for _, p := range [][2]int{
		{0, 0},
		{dstW / 2, dstH / 2},
		{dstW - 1, dstH - 1},
		{1200, 700},
	} {
		i := (p[1]*dstW + p[0]) * BYTES_PER_PIXEL
		if got[i+3] != 0xFF {
			return fmt.Errorf("pixel (%d,%d) was not filled by hardware stretch: rgba=%v", p[0], p[1], got[i:i+BYTES_PER_PIXEL])
		}
		if got[i] == 0 && got[i+1] == 0 && got[i+2] == 0 {
			return fmt.Errorf("pixel (%d,%d) was black after hardware stretch: rgba=%v", p[0], p[1], got[i:i+BYTES_PER_PIXEL])
		}
	}
	return nil
}

func TestEbitenOutput_HardwareCompositor_NonIntegerScaleMatchesSoftwareFloor(t *testing.T) {
	runGPUGate(t, "hw_noninteger")
}

func gateHardwareNonIntegerScaleMatchesSoftwareFloor() error {
	const (
		srcW = 3
		srcH = 1
		dstW = 5
		dstH = 1
	)
	frame := []byte{
		10, 0, 0, 0xFF,
		20, 0, 0, 0xFF,
		30, 0, 0, 0xFF,
	}
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: dstW, Height: dstH, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}
	update := CompositorFrameUpdate{
		FrameID:            9,
		PresentationWidth:  dstW,
		PresentationHeight: dstH,
		HasContent:         true,
		Layers: []CompositorFrameLayer{{
			SourceID:     1,
			SourceWidth:  srcW,
			SourceHeight: srcH,
			DestWidth:    dstW,
			DestHeight:   dstH,
			Buffer:       frame,
		}},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
		return fmt.Errorf("UpdateHardwareCompositorFrame: %w", err)
	}
	screen := ebiten.NewImage(dstW, dstH)
	eo.Draw(screen)
	got := make([]byte, dstW*dstH*BYTES_PER_PIXEL)
	screen.ReadPixels(got)

	wantR := []byte{10, 10, 20, 20, 30}
	for x, want := range wantR {
		i := x * BYTES_PER_PIXEL
		if got[i] != want || got[i+3] != 0xFF {
			return fmt.Errorf("pixel %d = rgba %v, want red=%d alpha=255; floor mapping should be [10 10 20 20 30]", x, got[i:i+BYTES_PER_PIXEL], want)
		}
	}
	return nil
}

func TestEbitenOutput_HardwareCompositor_PartialAlphaLayerReplacesLowerLayer(t *testing.T) {
	runGPUGate(t, "hw_partialalpha")
}

func gateHardwarePartialAlphaLayerReplacesLowerLayer() error {
	out, err := NewEbitenOutput()
	if err != nil {
		return fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 2, Height: 1, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return fmt.Errorf("SetDisplayConfig: %w", err)
	}

	lower := []byte{
		100, 80, 60, 0xFF,
		30, 40, 50, 0xFF,
	}
	upper := []byte{
		4, 2, 1, 8,
		0, 0, 0, 0,
	}
	update := CompositorFrameUpdate{
		FrameID:            2,
		PresentationWidth:  2,
		PresentationHeight: 1,
		HasContent:         true,
		Layers: []CompositorFrameLayer{
			{
				SourceID:     1,
				SourceWidth:  2,
				SourceHeight: 1,
				DestWidth:    2,
				DestHeight:   1,
				Buffer:       lower,
			},
			{
				SourceID:     2,
				SourceWidth:  2,
				SourceHeight: 1,
				DestWidth:    2,
				DestHeight:   1,
				Buffer:       upper,
			},
		},
	}
	if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
		return fmt.Errorf("UpdateHardwareCompositorFrame: %w", err)
	}
	screen := ebiten.NewImage(2, 1)
	eo.Draw(screen)
	got := make([]byte, 2*BYTES_PER_PIXEL)
	screen.ReadPixels(got)

	if want := upper[:BYTES_PER_PIXEL]; !sameBytes(got[:BYTES_PER_PIXEL], want) {
		return fmt.Errorf("partial-alpha top pixel = %v, want exact copy %v", got[:BYTES_PER_PIXEL], want)
	}
	if want := lower[BYTES_PER_PIXEL:]; !sameBytes(got[BYTES_PER_PIXEL:], want) {
		return fmt.Errorf("transparent top pixel = %v, want lower layer %v", got[BYTES_PER_PIXEL:], want)
	}
	return nil
}

func sameBytes(a, b []byte) bool {
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

func TestEbitenOutput_UpdateRegion_RejectsShortPixels(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	if err := eo.UpdateRegion(0, 0, 2, 2, make([]byte, 2*2*4-1)); err == nil {
		t.Fatal("short region pixels were accepted")
	}
}

func TestEbitenOutput_WaitForVSync_AfterStop_DoesNotBlock(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	if err := eo.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	errc := make(chan error, 1)
	go func() {
		errc <- eo.WaitForVSync()
	}()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("WaitForVSync returned nil after Stop")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("WaitForVSync blocked after Stop")
	}
}

func BenchmarkEbitenGPUCompositor960x540To1080p(b *testing.B) {
	if os.Getenv("IE_PERF_GPU_COMPOSITOR") == "" {
		b.Skip("set IE_PERF_GPU_COMPOSITOR=1 to run the real Ebiten compositor benchmark")
	}
	src := &mockOpaqueSource{
		layer: 0,
		w:     960,
		h:     540,
		frame: solidTestFrame(960, 540, 0x11, 0x22, 0x33, 0xFF),
	}
	src.enabled.Store(true)
	layer := CompositorFrameLayer{
		SourceID:     1,
		SourceWidth:  960,
		SourceHeight: 540,
		DestWidth:    1920,
		DestHeight:   1080,
		Buffer:       src.frame,
	}

	b.Run("software-scale-full-upload", func(b *testing.B) {
		out, err := NewEbitenOutput()
		if err != nil {
			b.Fatalf("NewEbitenOutput: %v", err)
		}
		eo := out.(*EbitenOutput)
		eo.showStatusBar = false
		screen := ebiten.NewImage(1920, 1080)
		comp := NewVideoCompositor(nil)
		comp.LockResolution(1920, 1080)
		comp.RegisterSource(src)
		for range 16 {
			comp.composite()
			if err := eo.UpdateFrame(comp.finalFrame); err != nil {
				b.Fatalf("UpdateFrame warmup: %v", err)
			}
			eo.Draw(screen)
		}
		b.ResetTimer()
		for range b.N {
			comp.composite()
			if err := eo.UpdateFrame(comp.finalFrame); err != nil {
				b.Fatalf("UpdateFrame: %v", err)
			}
			eo.Draw(screen)
		}
	})

	b.Run("native-upload-gpu-draw", func(b *testing.B) {
		out, err := NewEbitenOutput()
		if err != nil {
			b.Fatalf("NewEbitenOutput: %v", err)
		}
		eo := out.(*EbitenOutput)
		eo.showStatusBar = false
		screen := ebiten.NewImage(1920, 1080)
		update := CompositorFrameUpdate{
			FrameID:            1,
			PresentationWidth:  1920,
			PresentationHeight: 1080,
			HasContent:         true,
			Layers:             []CompositorFrameLayer{layer},
		}
		for i := range 16 {
			update.FrameID = uint64(i + 1)
			if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
				b.Fatalf("UpdateHardwareCompositorFrame warmup: %v", err)
			}
			eo.Draw(screen)
		}
		b.ResetTimer()
		for i := range b.N {
			update.FrameID = uint64(i + 17)
			if err := eo.UpdateHardwareCompositorFrame(update); err != nil {
				b.Fatalf("UpdateHardwareCompositorFrame: %v", err)
			}
			eo.Draw(screen)
		}
	})
}
