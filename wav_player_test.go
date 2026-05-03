//go:build headless

package main

import (
	"encoding/binary"
	"testing"
	"time"
)

func newTestWAVPlayer(t *testing.T) (*WAVPlayer, *MachineBus) {
	t.Helper()
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewWAVPlayer(chip, SAMPLE_RATE)
	bus := NewMachineBus()
	player.AttachBus(bus)
	return player, bus
}

// buildMinimalWAVData creates a minimal valid WAV byte stream for testing.
func buildMinimalWAVData(numSamples int) []byte {
	pcm := make([]byte, numSamples*2)
	for i := range numSamples {
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(int16(1000)))
	}
	return buildTestWAV(44100, 1, 16, pcm)
}

func TestWAVPlayerMMIOWritePtr(t *testing.T) {
	player, _ := newTestWAVPlayer(t)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x12345678)

	player.mu.Lock()
	got := player.PlayPtrStaged
	player.mu.Unlock()

	if got != 0x12345678 {
		t.Errorf("expected ptr=0x12345678, got 0x%08X", got)
	}
}

func TestWAVPlayerMMIOWriteLen(t *testing.T) {
	player, _ := newTestWAVPlayer(t)

	player.HandlePlayWrite(WAV_PLAY_LEN, 0xABCD)

	player.mu.Lock()
	got := player.PlayLenStaged
	player.mu.Unlock()

	if got != 0xABCD {
		t.Errorf("expected len=0xABCD, got 0x%X", got)
	}
}

func TestWAVPlayerMMIOHighHalfWordWrites(t *testing.T) {
	player, _ := newTestWAVPlayer(t)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0xFF112233)
	player.HandlePlayWrite(WAV_PLAY_PTR+2, 0x00BB)
	player.HandlePlayWrite(WAV_PLAY_LEN, 0xEE000044)
	player.HandlePlayWrite(WAV_PLAY_LEN+2, 0x00DD)

	player.mu.Lock()
	ptr := player.PlayPtrStaged
	length := player.PlayLenStaged
	player.mu.Unlock()

	if ptr != 0x00BB2233 {
		t.Fatalf("PlayPtrStaged = 0x%08X, want 0x00BB2233", ptr)
	}
	if length != 0x00DD0044 {
		t.Fatalf("PlayLenStaged = 0x%08X, want 0x00DD0044", length)
	}
}

func TestWAVPlayerMMIOStart(t *testing.T) {
	player, bus := newTestWAVPlayer(t)

	wavData := buildMinimalWAVData(50)
	mem := bus.GetMemory()
	copy(mem[0x1000:], wavData)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(WAV_PLAY_LEN, uint32(len(wavData)))
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1) // start

	time.Sleep(50 * time.Millisecond)

	player.mu.Lock()
	hasErr := player.PlayErr
	player.mu.Unlock()

	if hasErr {
		t.Error("expected no error after start")
	}
}

func TestWAVPlayerMMIOStop(t *testing.T) {
	player, bus := newTestWAVPlayer(t)

	wavData := buildMinimalWAVData(50)
	mem := bus.GetMemory()
	copy(mem[0x1000:], wavData)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(WAV_PLAY_LEN, uint32(len(wavData)))
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1) // start
	time.Sleep(50 * time.Millisecond)

	player.HandlePlayWrite(WAV_PLAY_CTRL, 2) // stop

	if player.IsPlaying() {
		t.Error("expected playback to stop")
	}
}

func TestWAVPlayerChannelBaseChangeReleasesOldDACChannels(t *testing.T) {
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewWAVPlayer(chip, SAMPLE_RATE)
	wav := &WAVFile{
		SampleRate:    44100,
		NumChannels:   2,
		BitsPerSample: 16,
		LeftSamples:   []int16{16000, 16000},
		RightSamples:  []int16{-16000, -16000},
	}
	player.engine.LoadWAV(wav)
	player.engine.SetForceMono(false)
	player.Play()
	player.engine.TickSample()

	if !chip.IsChannelInDAC(0) || !chip.IsChannelInDAC(1) {
		t.Fatal("expected original base channels in DAC mode")
	}

	player.HandlePlayWrite(WAV_CHANNEL_BASE, 2)
	player.engine.TickSample()

	if chip.IsChannelInDAC(0) || chip.IsChannelInDAC(1) {
		t.Fatal("old base channels remained in DAC mode after channel-base change")
	}
	if !chip.IsChannelInDAC(2) || !chip.IsChannelInDAC(3) {
		t.Fatal("expected new base channels in DAC mode")
	}
}

func TestWAVPlayerMMIOStatus(t *testing.T) {
	player, _ := newTestWAVPlayer(t)

	// Initially: no busy, no error
	status := player.HandlePlayRead(WAV_PLAY_STATUS)
	if status != 0 {
		t.Errorf("expected initial status=0, got %d", status)
	}

	// Try to start with no data (length=0) → error
	player.HandlePlayWrite(WAV_PLAY_PTR, 0)
	player.HandlePlayWrite(WAV_PLAY_LEN, 0)
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1) // start

	status = player.HandlePlayRead(WAV_PLAY_STATUS)
	if status&0x2 == 0 {
		t.Error("expected error bit set for zero-length data")
	}
}

func TestWAVPlayerMMIOPosition(t *testing.T) {
	player, bus := newTestWAVPlayer(t)

	wavData := buildMinimalWAVData(50)
	mem := bus.GetMemory()
	copy(mem[0x1000:], wavData)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(WAV_PLAY_LEN, uint32(len(wavData)))
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1) // start
	time.Sleep(50 * time.Millisecond)

	// Position should be 0 at start (or very small)
	pos := player.HandlePlayRead(WAV_POSITION)
	// Position can advance a bit during async load, but should be reasonable
	if pos > 100 {
		t.Errorf("expected position near 0 at start, got %d", pos)
	}
}

func TestWAVPlayerLoadInvalidData(t *testing.T) {
	player, bus := newTestWAVPlayer(t)

	// Put garbage in bus memory
	mem := bus.GetMemory()
	for i := range 100 {
		mem[0x1000+i] = byte(i)
	}

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(WAV_PLAY_LEN, 100)
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1) // start
	time.Sleep(50 * time.Millisecond)

	status := player.HandlePlayRead(WAV_PLAY_STATUS)
	if status&0x2 == 0 {
		t.Error("expected error bit set for invalid data")
	}
}

func TestWAVPlayerGenerationCounting(t *testing.T) {
	player, bus := newTestWAVPlayer(t)

	wavData := buildMinimalWAVData(50)
	mem := bus.GetMemory()
	copy(mem[0x1000:], wavData)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(WAV_PLAY_LEN, uint32(len(wavData)))

	// Start first load
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1)

	// Immediately stop (increments generation, cancelling first load)
	player.HandlePlayWrite(WAV_PLAY_CTRL, 2)

	player.mu.Lock()
	gen := player.playGen
	player.mu.Unlock()

	// Generation should have been incremented twice (start + stop)
	if gen < 2 {
		t.Errorf("expected generation >= 2, got %d", gen)
	}

	time.Sleep(50 * time.Millisecond)

	// Should not be playing (stop cancelled the load)
	if player.IsPlaying() {
		t.Error("expected not playing after generation cancel")
	}
}
