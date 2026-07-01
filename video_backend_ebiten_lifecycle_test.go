//go:build !headless

package main

import (
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

func TestEbitenOutput_HardwareCompositor_Non16x9FillsStretchRect(t *testing.T) {
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
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: dstW, Height: dstH, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
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
		t.Fatalf("UpdateHardwareCompositorFrame returned error: %v", err)
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
			t.Fatalf("pixel (%d,%d) was not filled by hardware stretch: rgba=%v", p[0], p[1], got[i:i+BYTES_PER_PIXEL])
		}
		if got[i] == 0 && got[i+1] == 0 && got[i+2] == 0 {
			t.Fatalf("pixel (%d,%d) was black after hardware stretch: rgba=%v", p[0], p[1], got[i:i+BYTES_PER_PIXEL])
		}
	}
}

func TestEbitenOutput_HardwareCompositor_NonIntegerScaleMatchesSoftwareFloor(t *testing.T) {
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
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: dstW, Height: dstH, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
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
		t.Fatalf("UpdateHardwareCompositorFrame returned error: %v", err)
	}
	screen := ebiten.NewImage(dstW, dstH)
	eo.Draw(screen)
	got := make([]byte, dstW*dstH*BYTES_PER_PIXEL)
	screen.ReadPixels(got)

	wantR := []byte{10, 10, 20, 20, 30}
	for x, want := range wantR {
		i := x * BYTES_PER_PIXEL
		if got[i] != want || got[i+3] != 0xFF {
			t.Fatalf("pixel %d = rgba %v, want red=%d alpha=255; floor mapping should be [10 10 20 20 30]", x, got[i:i+BYTES_PER_PIXEL], want)
		}
	}
}

func TestEbitenOutput_HardwareCompositor_PartialAlphaLayerReplacesLowerLayer(t *testing.T) {
	out, err := NewEbitenOutput()
	if err != nil {
		t.Fatalf("NewEbitenOutput returned error: %v", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: 2, Height: 1, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		t.Fatalf("SetDisplayConfig returned error: %v", err)
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
		t.Fatalf("UpdateHardwareCompositorFrame returned error: %v", err)
	}
	screen := ebiten.NewImage(2, 1)
	eo.Draw(screen)
	got := make([]byte, 2*BYTES_PER_PIXEL)
	screen.ReadPixels(got)

	if want := upper[:BYTES_PER_PIXEL]; !sameBytes(got[:BYTES_PER_PIXEL], want) {
		t.Fatalf("partial-alpha top pixel = %v, want exact copy %v", got[:BYTES_PER_PIXEL], want)
	}
	if want := lower[BYTES_PER_PIXEL:]; !sameBytes(got[BYTES_PER_PIXEL:], want) {
		t.Fatalf("transparent top pixel = %v, want lower layer %v", got[BYTES_PER_PIXEL:], want)
	}
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
