package main

import (
	"os"
	"runtime"
	"testing"
	"time"
)

const ayProgressiveStartDeadline = 500 * time.Millisecond

func ayFiniteCompletionDeadline() time.Duration {
	if runtime.GOOS == "js" || runtime.GOARCH == "arm64" {
		return 60 * time.Second
	}
	return 10 * time.Second
}

func TestPSGPlayerZXAYEMULLoadStartsBeforeFullRender(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	defer player.Stop()
	done := make(chan error, 1)
	go func() { done <- player.Load("sdk/examples/assets/music/WaksonsZak018.ay") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(ayProgressiveStartDeadline):
		t.Fatalf("ZXAYEMUL LoadData did not publish its startup buffer within %v", ayProgressiveStartDeadline)
	}

	engine.mutex.Lock()
	eventCount := len(engine.events)
	playing := engine.playing
	totalSamples := engine.totalSamples
	engine.mutex.Unlock()
	if !playing || eventCount == 0 {
		t.Fatalf("startup buffer not playable: playing=%v events=%d", playing, eventCount)
	}
	if totalSamples != 0 {
		t.Fatalf("unbounded ZXAYEMUL track was fully rendered before LoadData returned: totalSamples=%d", totalSamples)
	}
	instructions, cpuName, _ := player.RenderPerf()
	if instructions == 0 || cpuName != "Z80" {
		t.Fatalf("startup render statistics: instructions=%d cpu=%q", instructions, cpuName)
	}
}

func TestPSGPlayerZXAYEMULMMIOStartsBeforeFullRender(t *testing.T) {
	data, err := os.ReadFile("sdk/examples/assets/music/Robocop1.ay")
	if err != nil {
		t.Fatal(err)
	}
	bus, err := NewMachineBusSized(0x10000)
	if err != nil {
		t.Fatal(err)
	}
	const addr = uint32(0x400)
	copy(bus.GetMemory()[addr:], data)
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	defer player.Stop()
	player.AttachBus(bus)
	for i := range 4 {
		player.HandlePlayWrite(PSG_PLAY_PTR+uint32(i), uint32(byte(addr>>uint(i*8))))
		player.HandlePlayWrite(PSG_PLAY_LEN+uint32(i), uint32(byte(uint32(len(data))>>uint(i*8))))
	}
	player.HandlePlayWrite(PSG_PLAY_CTRL, 1)

	deadline := time.Now().Add(ayProgressiveStartDeadline)
	for !engine.IsPlaying() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !engine.IsPlaying() {
		t.Fatalf("ZXAYEMUL MMIO playback did not publish its startup buffer within %v", ayProgressiveStartDeadline)
	}
	engine.mutex.Lock()
	eventCount := len(engine.events)
	engine.mutex.Unlock()
	if eventCount == 0 {
		t.Fatal("ZXAYEMUL MMIO playback started without buffered PSG events")
	}
}

func TestPSGPlayerZXAYEMULStopCancelsProgressiveProducer(t *testing.T) {
	data, err := os.ReadFile("sdk/examples/assets/music/Robocop1.ay")
	if err != nil {
		t.Fatal(err)
	}
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	if err := player.LoadData(data); err != nil {
		t.Fatal(err)
	}
	player.Stop()
	time.Sleep(50 * time.Millisecond)
	engine.mutex.Lock()
	eventCount := len(engine.events)
	playing := engine.playing
	engine.mutex.Unlock()
	if playing || eventCount != 0 {
		t.Fatalf("cancelled producer republished playback: playing=%v events=%d", playing, eventCount)
	}
}

func TestPSGPlayerFailedLoadPreservesProgressiveProducer(t *testing.T) {
	tests := []struct {
		name string
		load func(*PSGPlayer) error
	}{
		{
			name: "unreadable path",
			load: func(player *PSGPlayer) error {
				return player.Load("sdk/examples/assets/music/does-not-exist.ay")
			},
		},
		{
			name: "malformed data",
			load: func(player *PSGPlayer) error {
				return player.LoadData([]byte("ZXAYEMUL"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewPSGEngine(nil, SAMPLE_RATE)
			player := NewPSGPlayer(engine)
			defer player.Stop()
			if err := player.Load("sdk/examples/assets/music/Robocop1.ay"); err != nil {
				t.Fatal(err)
			}
			player.mu.Lock()
			generation := player.PlayGen
			player.mu.Unlock()
			if err := tt.load(player); err == nil {
				t.Fatal("replacement load unexpectedly succeeded")
			}
			player.mu.Lock()
			gotGeneration := player.PlayGen
			player.mu.Unlock()
			if gotGeneration != generation {
				t.Fatalf("failed load changed progressive generation: got=%d want=%d", gotGeneration, generation)
			}
			if !engine.IsPlaying() {
				t.Fatal("failed load stopped existing progressive playback")
			}

			engine.mutex.Lock()
			before := len(engine.events)
			engine.mutex.Unlock()
			engine.TickBlock(SAMPLE_RATE * (ayZ80BufferSeconds + 1))
			replenishDeadline := ayProgressiveStartDeadline
			if runtime.GOOS == "js" {
				replenishDeadline = 10 * time.Second
			} else if runtime.GOARCH == "arm64" {
				replenishDeadline = 5 * time.Second
			}
			deadline := time.Now().Add(replenishDeadline)
			for time.Now().Before(deadline) {
				engine.mutex.Lock()
				grew := len(engine.events) > before
				engine.mutex.Unlock()
				if grew {
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatal("existing progressive producer did not replenish after failed load")
		})
	}
}

func TestPSGPlayerSuccessfulLoadReplacesProgressiveProducer(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	defer player.Stop()
	if err := player.Load("sdk/examples/assets/music/Robocop1.ay"); err != nil {
		t.Fatal(err)
	}
	player.mu.Lock()
	oldGeneration := player.PlayGen
	player.mu.Unlock()
	if err := player.Load("sdk/examples/assets/music/WaksonsZak018.ay"); err != nil {
		t.Fatal(err)
	}
	player.mu.Lock()
	newGeneration := player.PlayGen
	title := player.metadata.Title
	player.mu.Unlock()
	if newGeneration != oldGeneration+1 {
		t.Fatalf("successful replacement generation=%d, want %d", newGeneration, oldGeneration+1)
	}
	if !engine.IsPlaying() {
		t.Fatal("successful replacement did not start playback")
	}
	if title == "" {
		t.Fatal("successful replacement did not publish new metadata")
	}
}

func TestPSGPlayerFiniteAYCompletesWithoutSampleConsumer(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	defer player.Stop()
	if err := player.LoadData(buildAYZ80EmulData("FiniteHeadless", 260)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(ayFiniteCompletionDeadline())
	for player.DurationSeconds() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := player.DurationSeconds(); got != 5.2 {
		t.Fatalf("headless finite AY duration=%v, want 5.2 seconds", got)
	}
}

func TestPSGPlayerFiniteAYCompletesAfterConsumerPauses(t *testing.T) {
	engine := NewPSGEngine(nil, SAMPLE_RATE)
	player := NewPSGPlayer(engine)
	defer player.Stop()
	if err := player.LoadData(buildAYZ80EmulData("FinitePaused", 260)); err != nil {
		t.Fatal(err)
	}
	consumeUntil := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(consumeUntil) {
		engine.TickSample()
		time.Sleep(10 * time.Millisecond)
	}
	if got := player.DurationSeconds(); got != 0 {
		t.Fatalf("active consumer did not keep progressive rendering bounded: duration=%v", got)
	}
	deadline := time.Now().Add(ayFiniteCompletionDeadline())
	for player.DurationSeconds() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := player.DurationSeconds(); got != 5.2 {
		t.Fatalf("paused-consumer finite AY duration=%v, want 5.2 seconds", got)
	}
}
