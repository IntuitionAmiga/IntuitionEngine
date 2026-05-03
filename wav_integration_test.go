//go:build headless

package main

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestWAVEndToEnd(t *testing.T) {
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	player := NewWAVPlayer(chip, SAMPLE_RATE)
	bus := NewMachineBus()
	player.AttachBus(bus)

	// Build a WAV with known samples
	pcm := make([]byte, 200) // 100 samples of 16-bit mono
	for i := range 100 {
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(int16(8000)))
	}
	wavData := buildTestWAV(44100, 1, 16, pcm)

	// Place in bus memory and trigger via MMIO
	mem := bus.GetMemory()
	copy(mem[0x2000:], wavData)

	player.HandlePlayWrite(WAV_PLAY_PTR, 0x2000)
	player.HandlePlayWrite(WAV_PLAY_LEN, uint32(len(wavData)))
	player.HandlePlayWrite(WAV_PLAY_CTRL, 1) // start

	time.Sleep(50 * time.Millisecond)

	// Tick engine to produce audio
	for range 20 {
		player.engine.TickSample()
	}

	// Verify DAC output is non-zero
	ch := chip.channels[0]
	if !ch.dacMode {
		t.Fatal("expected channel 0 in DAC mode")
	}
	sample := ch.generateSample()
	if sample == 0 {
		t.Error("expected non-zero DAC output")
	}
}

func TestWAVMediaLoaderRouting(t *testing.T) {
	// .wav extension should route to MEDIA_TYPE_WAV
	got := detectMediaType("song.wav")
	if got != MEDIA_TYPE_WAV {
		t.Errorf("expected MEDIA_TYPE_WAV (%d), got %d", MEDIA_TYPE_WAV, got)
	}
	// case insensitive
	got = detectMediaType("SONG.WAV")
	if got != MEDIA_TYPE_WAV {
		t.Errorf("expected MEDIA_TYPE_WAV for uppercase, got %d", got)
	}
}

func TestWAVMediaLoaderLargeFile(t *testing.T) {
	// Verify WAV files larger than MEDIA_STAGING_SIZE can be loaded via media loader
	// (same bypass path as MOD)
	if MEDIA_STAGING_SIZE < 65536 {
		t.Skip("MEDIA_STAGING_SIZE too small for test")
	}
	// Just verify the type detection works — actual large file loading
	// is tested through the WAV player's direct Load() path
	got := detectMediaType("large.wav")
	if got != MEDIA_TYPE_WAV {
		t.Errorf("expected MEDIA_TYPE_WAV, got %d", got)
	}
}

func TestWAVRegisters(t *testing.T) {
	// Verify register constants are sequential and non-overlapping
	if WAV_PLAY_PTR != 0xF0BD8 {
		t.Errorf("WAV_PLAY_PTR: expected 0xF0BD8, got 0x%X", WAV_PLAY_PTR)
	}
	if WAV_PLAY_LEN != 0xF0BDC {
		t.Errorf("WAV_PLAY_LEN: expected 0xF0BDC, got 0x%X", WAV_PLAY_LEN)
	}
	if WAV_PLAY_CTRL != 0xF0BE0 {
		t.Errorf("WAV_PLAY_CTRL: expected 0xF0BE0, got 0x%X", WAV_PLAY_CTRL)
	}
	if WAV_PLAY_STATUS != 0xF0BE4 {
		t.Errorf("WAV_PLAY_STATUS: expected 0xF0BE4, got 0x%X", WAV_PLAY_STATUS)
	}
	if WAV_POSITION != 0xF0BE8 {
		t.Errorf("WAV_POSITION: expected 0xF0BE8, got 0x%X", WAV_POSITION)
	}
	if WAV_PLAY_PTR_HI != 0xF0BEC {
		t.Errorf("WAV_PLAY_PTR_HI: expected 0xF0BEC, got 0x%X", WAV_PLAY_PTR_HI)
	}
	if WAV_CHANNEL_BASE != 0xF0BF0 {
		t.Errorf("WAV_CHANNEL_BASE: expected 0xF0BF0, got 0x%X", WAV_CHANNEL_BASE)
	}
	if WAV_VOLUME_L != 0xF0BF1 {
		t.Errorf("WAV_VOLUME_L: expected 0xF0BF1, got 0x%X", WAV_VOLUME_L)
	}
	if WAV_VOLUME_R != 0xF0BF2 {
		t.Errorf("WAV_VOLUME_R: expected 0xF0BF2, got 0x%X", WAV_VOLUME_R)
	}
	if WAV_FLAGS != 0xF0BF3 {
		t.Errorf("WAV_FLAGS: expected 0xF0BF3, got 0x%X", WAV_FLAGS)
	}
	if WAV_END != 0xF0BF3 {
		t.Errorf("WAV_END: expected 0xF0BF3, got 0x%X", WAV_END)
	}

	// Verify no overlap with MOD registers
	if WAV_PLAY_PTR <= MOD_END {
		t.Errorf("WAV registers overlap MOD: WAV_PLAY_PTR=0x%X <= MOD_END=0x%X",
			WAV_PLAY_PTR, MOD_END)
	}
}

func TestWAVEndToEndGenerated(t *testing.T) {
	// Generate a 440Hz sine wave WAV in-memory, load via player, verify non-zero audio
	chip, _ := NewSoundChip(AUDIO_BACKEND_OTO)
	engine := NewWAVEngine(chip, SAMPLE_RATE)

	numSamples := 4410 // 100ms at 44100Hz
	pcm := make([]byte, numSamples*2)
	for i := range numSamples {
		// 440Hz sine wave
		val := int16(16384 * math.Sin(2*math.Pi*440*float64(i)/44100))
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(val))
	}
	wavData := buildTestWAV(44100, 1, 16, pcm)

	wav, err := ParseWAV(wavData)
	if err != nil {
		t.Fatalf("ParseWAV failed: %v", err)
	}

	engine.LoadWAV(wav)
	engine.SetPlaying(true)

	// Tick enough samples to produce output
	var maxAbs float32
	for range 100 {
		engine.TickSample()
		ch := chip.channels[0]
		sample := ch.generateSample()
		if abs := float32(math.Abs(float64(sample))); abs > maxAbs {
			maxAbs = abs
		}
	}

	if maxAbs < 0.01 {
		t.Errorf("expected non-zero audio output from sine wave, max abs was %f", maxAbs)
	}
}
