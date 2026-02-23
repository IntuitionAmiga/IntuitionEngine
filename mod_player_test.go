//go:build headless

package main

import (
	"testing"
	"time"
)

func newTestMODPlayer(t *testing.T) (*MODPlayer, *MachineBus) {
	t.Helper()
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewMODPlayer(chip, SAMPLE_RATE)
	bus := NewMachineBus()
	player.AttachBus(bus)
	return player, bus
}

func TestMODPlayerMMIOWritePtr(t *testing.T) {
	player, _ := newTestMODPlayer(t)

	player.HandlePlayWrite(MOD_PLAY_PTR, 0x12345678)

	player.mu.Lock()
	got := player.playPtrStaged
	player.mu.Unlock()

	if got != 0x12345678 {
		t.Errorf("expected ptr=0x12345678, got 0x%08X", got)
	}
}

func TestMODPlayerMMIOWriteLen(t *testing.T) {
	player, _ := newTestMODPlayer(t)

	player.HandlePlayWrite(MOD_PLAY_LEN, 0xABCD)

	player.mu.Lock()
	got := player.playLenStaged
	player.mu.Unlock()

	if got != 0xABCD {
		t.Errorf("expected len=0xABCD, got 0x%X", got)
	}
}

func TestMODPlayerMMIOStart(t *testing.T) {
	player, bus := newTestMODPlayer(t)

	// Build a minimal MOD and place it in bus memory
	sampleData := make([]int8, 64)
	modData := buildMinimalMOD(sampleData, nil)
	mem := bus.GetMemory()
	copy(mem[0x1000:], modData)

	// Stage pointer and length
	player.HandlePlayWrite(MOD_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(MOD_PLAY_LEN, uint32(len(modData)))

	// Start (bit 0)
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1)

	// Wait for async load
	time.Sleep(50 * time.Millisecond)

	player.mu.Lock()
	busy := player.playBusy
	hasErr := player.playErr
	player.mu.Unlock()

	if hasErr {
		t.Error("expected no error after start")
	}
	// busy should be true or playing should have started
	_ = busy
}

func TestMODPlayerMMIOStop(t *testing.T) {
	player, bus := newTestMODPlayer(t)

	// Load and start
	sampleData := make([]int8, 64)
	modData := buildMinimalMOD(sampleData, nil)
	mem := bus.GetMemory()
	copy(mem[0x1000:], modData)

	player.HandlePlayWrite(MOD_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(MOD_PLAY_LEN, uint32(len(modData)))
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1) // start
	time.Sleep(50 * time.Millisecond)

	// Stop (bit 1)
	player.HandlePlayWrite(MOD_PLAY_CTRL, 2)

	if player.IsPlaying() {
		t.Error("expected playback to stop")
	}
}

func TestMODPlayerMMIOStatus(t *testing.T) {
	player, _ := newTestMODPlayer(t)

	// Initially: no busy, no error
	status := player.HandlePlayRead(MOD_PLAY_STATUS)
	if status != 0 {
		t.Errorf("expected initial status=0, got %d", status)
	}

	// Try to start with no data (length=0) → error
	player.HandlePlayWrite(MOD_PLAY_PTR, 0)
	player.HandlePlayWrite(MOD_PLAY_LEN, 0)
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1) // start

	status = player.HandlePlayRead(MOD_PLAY_STATUS)
	if status&0x2 == 0 {
		t.Error("expected error bit set for zero-length data")
	}
}

func TestMODPlayerMMIOFilterModel(t *testing.T) {
	player, _ := newTestMODPlayer(t)

	player.HandlePlayWrite(MOD_FILTER_MODEL, 1) // A500
	got := player.HandlePlayRead(MOD_FILTER_MODEL)
	if got != 1 {
		t.Errorf("expected filter model=1, got %d", got)
	}

	player.HandlePlayWrite(MOD_FILTER_MODEL, 2) // A1200
	got = player.HandlePlayRead(MOD_FILTER_MODEL)
	if got != 2 {
		t.Errorf("expected filter model=2, got %d", got)
	}
}

func TestMODPlayerMMIOPosition(t *testing.T) {
	player, bus := newTestMODPlayer(t)

	sampleData := make([]int8, 64)
	modData := buildMinimalMOD(sampleData, nil)
	mem := bus.GetMemory()
	copy(mem[0x1000:], modData)

	player.HandlePlayWrite(MOD_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(MOD_PLAY_LEN, uint32(len(modData)))
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1) // start
	time.Sleep(50 * time.Millisecond)

	// Position should be 0 at start
	pos := player.HandlePlayRead(MOD_POSITION)
	if pos != 0 {
		t.Errorf("expected position=0 at start, got %d", pos)
	}
}

func TestMODPlayerLoadInvalidData(t *testing.T) {
	player, bus := newTestMODPlayer(t)

	// Put garbage in bus memory
	mem := bus.GetMemory()
	for i := range 100 {
		mem[0x1000+i] = byte(i)
	}

	player.HandlePlayWrite(MOD_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(MOD_PLAY_LEN, 100)
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1) // start
	time.Sleep(50 * time.Millisecond)

	status := player.HandlePlayRead(MOD_PLAY_STATUS)
	if status&0x2 == 0 {
		t.Error("expected error bit set for invalid data")
	}
}

func TestMODPlayerGenerationCounting(t *testing.T) {
	player, bus := newTestMODPlayer(t)

	sampleData := make([]int8, 64)
	modData := buildMinimalMOD(sampleData, nil)
	mem := bus.GetMemory()
	copy(mem[0x1000:], modData)

	player.HandlePlayWrite(MOD_PLAY_PTR, 0x1000)
	player.HandlePlayWrite(MOD_PLAY_LEN, uint32(len(modData)))

	// Start first load
	player.HandlePlayWrite(MOD_PLAY_CTRL, 1)

	// Immediately stop (increments generation, cancelling first load)
	player.HandlePlayWrite(MOD_PLAY_CTRL, 2)

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
