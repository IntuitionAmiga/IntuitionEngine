package main

import (
	"os"
	"strings"
	"testing"
)

func TestVideoScheduler_ManualTicksRegisteredTasks(t *testing.T) {
	scheduler := NewManualVideoScheduler()
	var first, second int
	scheduler.Register(func() { first++ })
	scheduler.Register(func() { second += 2 })

	scheduler.TickManual()
	scheduler.TickManual()

	if first != 2 || second != 4 {
		t.Fatalf("manual ticks first=%d second=%d, want 2 and 4", first, second)
	}
}

func TestVideoScheduler_UnregisterRemovesOnlySelectedTask(t *testing.T) {
	scheduler := NewManualVideoScheduler()
	var first, second int
	firstID := scheduler.Register(func() { first++ })
	scheduler.Register(func() { second++ })

	scheduler.TickManual()
	scheduler.Unregister(firstID)
	scheduler.TickManual()

	if first != 1 || second != 2 {
		t.Fatalf("ticks after unregister first=%d second=%d, want 1 and 2", first, second)
	}
}

func TestVideoScheduler_MigratedRenderLoopsDoNotOwnTickers(t *testing.T) {
	migrated := []string{
		"video_vga.go",
		"video_ula.go",
		"video_ted.go",
		"video_antic.go",
	}
	for _, path := range migrated {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "time.NewTicker") {
			t.Fatalf("%s still owns a ticker; use VideoScheduler", path)
		}
	}

	data, err := os.ReadFile("video_compositor.go")
	if err != nil {
		t.Fatalf("read video_compositor.go: %v", err)
	}
	if !strings.Contains(string(data), "time.NewTicker") {
		t.Fatal("video_compositor.go should own the scheduler ticker")
	}
}

func TestEbitenHardwareCompositorUsesExplicitFloorShaderForScaling(t *testing.T) {
	data, err := os.ReadFile("video_backend_ebiten.go")
	if err != nil {
		t.Fatalf("read video_backend_ebiten.go: %v", err)
	}
	src := string(data)
	start := strings.Index(src, "func (eo *EbitenOutput) drawHardwareCompositorLocked")
	if start < 0 {
		t.Fatal("drawHardwareCompositorLocked not found")
	}
	end := strings.Index(src[start:], "func (eo *EbitenOutput) Layout")
	if end < 0 {
		t.Fatal("drawHardwareCompositorLocked end not found")
	}
	body := src[start : start+end]
	if strings.Contains(body, "Filter: ebiten.FilterNearest") || strings.Contains(body, ".DrawImage(layer.image") {
		t.Fatal("hardware compositor must not pre-scale with DrawImage/FilterNearest; shader must own exact floor sampling")
	}
	for _, want := range []string{
		"DrawTrianglesShader",
		"var SrcSize vec2",
		"var RectSize vec2",
		"var DestOrigin vec2",
		"SrcSize",
		"RectSize",
		"localX := floor(dstPos.x - DestOrigin.x)",
		"localY := floor(dstPos.y - DestOrigin.y)",
		"floor(localX * SrcSize.x / RectSize.x)",
		"floor(localY * SrcSize.y / RectSize.y)",
		"imageSrc0At(imageSrc0Origin() + vec2(srcX, srcY))",
		"SrcX: sw",
		"SrcY: sh",
		"\"DestOrigin\"",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("video_backend_ebiten.go missing explicit floor-sampling shader marker %q", want)
		}
	}
}

func TestEbitenHardwareCompositorFloorMappingMatchesSoftware(t *testing.T) {
	t.Run("non integer horizontal scale", func(t *testing.T) {
		var got []int
		for dstX := 0; dstX < 5; dstX++ {
			got = append(got, ebitenHardwareCompositorTestSampleIndex(dstX, 0, 3, 5))
		}
		want := []int{0, 0, 1, 1, 2}
		if !intSlicesEqual(got, want) {
			t.Fatalf("3->5 hardware sample columns = %v, want software floor mapping %v", got, want)
		}
	})

	t.Run("letterboxed destination origin", func(t *testing.T) {
		var got []int
		for dstY := 12; dstY < 17; dstY++ {
			got = append(got, ebitenHardwareCompositorTestSampleIndex(dstY, 12, 3, 5))
		}
		want := []int{0, 0, 1, 1, 2}
		if !intSlicesEqual(got, want) {
			t.Fatalf("origin-shifted 3->5 hardware sample rows = %v, want software floor mapping %v", got, want)
		}
	})
}

func ebitenHardwareCompositorTestSampleIndex(dstPos, destOrigin, srcSize, rectSize int) int {
	local := dstPos - destOrigin
	src := local * srcSize / rectSize
	if src < 0 {
		return 0
	}
	if src >= srcSize {
		return srcSize - 1
	}
	return src
}

func intSlicesEqual(a, b []int) bool {
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
