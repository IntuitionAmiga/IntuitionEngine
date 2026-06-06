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
