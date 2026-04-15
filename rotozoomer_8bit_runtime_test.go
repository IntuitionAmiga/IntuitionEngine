//go:build headless

package main

import (
	"path/filepath"
	"testing"
	"time"
)

type eightBitRotoRig struct {
	bus   *MachineBus
	video *VideoChip
	sound *SoundChip
}

func newEightBitRotoRig(t *testing.T) *eightBitRotoRig {
	t.Helper()

	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.SetVideoStatusReader(video.HandleRead)

	sound, err := NewSoundChip(AUDIO_BACKEND_OTO)
	if err != nil {
		t.Fatalf("failed to create sound chip: %v", err)
	}
	sound.Start()

	psgEngine := NewPSGEngine(sound, SAMPLE_RATE)
	psgPlayer := NewPSGPlayer(psgEngine)
	psgPlayer.AttachBus(bus)
	bus.MapIO(PSG_BASE, PSG_END, psgEngine.HandleRead, psgEngine.HandleWrite)
	bus.MapIO(PSG_PLAY_PTR, PSG_PLAY_STATUS+3, psgPlayer.HandlePlayRead, psgPlayer.HandlePlayWrite)

	fileIO := NewFileIODevice(bus, ".")
	bus.MapIO(FILE_IO_BASE, FILE_IO_END, fileIO.HandleRead, fileIO.HandleWrite)
	bus.MapIOByte(FILE_IO_BASE, FILE_IO_END, fileIO.HandleWrite8)

	return &eightBitRotoRig{
		bus:   bus,
		video: video,
		sound: sound,
	}
}

func sampleFrameDistinctColors(frame []byte, width, height int, step int) int {
	if step <= 0 {
		step = 1
	}
	colors := make(map[[3]byte]struct{})
	for y := 0; y < height; y += step {
		rowOff := y * width * BYTES_PER_PIXEL
		for x := 0; x < width; x += step {
			off := rowOff + x*BYTES_PER_PIXEL
			if off+3 >= len(frame) {
				continue
			}
			colors[[3]byte{frame[off], frame[off+1], frame[off+2]}] = struct{}{}
		}
	}
	return len(colors)
}

func requireVideoContent(t *testing.T, video *VideoChip, minDistinct int) {
	t.Helper()

	mode := VideoModes[video.currentMode]
	frame := video.GetFrame()
	got := sampleFrameDistinctColors(frame, mode.width, mode.height, 80)
	if got < minDistinct {
		t.Fatalf("sampled distinct colors = %d, want >= %d", got, minDistinct)
	}
}

func Test6502RotozoomerRuntimeProducesVideo(t *testing.T) {
	rig := newEightBitRotoRig(t)
	if err := rig.video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = rig.video.Stop()
		rig.sound.Stop()
	})

	runner := NewCPU6502Runner(rig.bus, CPU6502Config{})
	runner.JITEnabled = true
	if err := runner.LoadProgram(filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_65.ie65")); err != nil {
		t.Fatalf("load 6502 rotozoomer: %v", err)
	}

	runner.StartExecution()
	time.Sleep(3 * time.Second)
	runner.Stop()

	requireVideoContent(t, rig.video, 6)
}

func TestZ80RotozoomerRuntimeProducesVideo(t *testing.T) {
	rig := newEightBitRotoRig(t)
	if err := rig.video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = rig.video.Stop()
		rig.sound.Stop()
	})

	runner := NewCPUZ80Runner(rig.bus, CPUZ80Config{
		LoadAddr:   defaultZ80LoadAddr,
		Entry:      defaultZ80LoadAddr,
		JITEnabled: true,
	})
	if err := runner.LoadProgram(filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_z80.ie80")); err != nil {
		t.Fatalf("load z80 rotozoomer: %v", err)
	}

	runner.StartExecution()
	time.Sleep(3 * time.Second)
	runner.Stop()

	requireVideoContent(t, rig.video, 6)
}

func TestZ80RobocopRuntimeProducesVideo(t *testing.T) {
	rig := newEightBitRotoRig(t)
	if err := rig.video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = rig.video.Stop()
		rig.sound.Stop()
	})

	runner := NewCPUZ80Runner(rig.bus, CPUZ80Config{
		LoadAddr:   defaultZ80LoadAddr,
		Entry:      defaultZ80LoadAddr,
		JITEnabled: true,
	})
	if err := runner.LoadProgram(filepath.Join("sdk", "examples", "prebuilt", "robocop_intro_z80.ie80")); err != nil {
		t.Fatalf("load z80 robocop: %v", err)
	}

	runner.StartExecution()
	time.Sleep(2 * time.Second)
	runner.Stop()

	requireVideoContent(t, rig.video, 5)
}
