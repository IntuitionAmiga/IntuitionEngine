//go:build headless

package main

import (
	"bytes"
	"encoding/binary"
	"os"
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

func fillX86RotoTexturePattern(memory []byte) {
	for y := 0; y < 256; y++ {
		row := x86RotoTextureBase + uint32(y*x86RotoTexStride)
		for x := 0; x < 256; x++ {
			pixel := uint32(0xFF000000 |
				(uint32((x*3+y)&0xFF) << 16) |
				(uint32((x+y*5)&0xFF) << 8) |
				uint32((x*7+y*11)&0xFF))
			binary.LittleEndian.PutUint32(memory[row+uint32(x*BYTES_PER_PIXEL):], pixel)
		}
	}
}

func TestX86RotoRenderMode7ToBackBufferMatchesBlitter(t *testing.T) {
	newReferenceBus := func() (*MachineBus, *VideoChip) {
		bus := NewMachineBus()
		video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
		if err != nil {
			t.Fatalf("failed to create video chip: %v", err)
		}
		video.AttachBus(bus)
		bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
		return bus, video
	}

	const (
		u0    = 0x0078A120
		v0    = 0x00831F40
		duCol = 0x00014000
		dvCol = 0x0000C000
		duRow = 0xFFFF4000
		dvRow = 0x00014000
	)

	directBus := NewMachineBus()
	directMem := directBus.GetMemory()
	fillX86RotoTexturePattern(directMem)
	if !x86RotoRenderMode7ToBackBuffer(directMem, u0, v0, duCol, dvCol, duRow, dvRow) {
		t.Fatal("direct mode7 render failed")
	}

	refBus, _ := newReferenceBus()
	refMem := refBus.GetMemory()
	fillX86RotoTexturePattern(refMem)
	refBus.Write32(VIDEO_CTRL, 1)
	refBus.Write32(VIDEO_MODE, 0)
	refBus.Write32(BLT_OP, bltOpMode7)
	refBus.Write32(BLT_SRC, x86RotoTextureBase)
	refBus.Write32(BLT_DST, x86RotoBackBuffer)
	refBus.Write32(BLT_WIDTH, x86RotoRenderW)
	refBus.Write32(BLT_HEIGHT, x86RotoRenderH)
	refBus.Write32(BLT_SRC_STRIDE, x86RotoTexStride)
	refBus.Write32(BLT_DST_STRIDE, x86RotoLineBytes)
	refBus.Write32(BLT_MODE7_TEX_W, 255)
	refBus.Write32(BLT_MODE7_TEX_H, 255)
	refBus.Write32(BLT_MODE7_U0, u0)
	refBus.Write32(BLT_MODE7_V0, v0)
	refBus.Write32(BLT_MODE7_DU_COL, duCol)
	refBus.Write32(BLT_MODE7_DV_COL, dvCol)
	refBus.Write32(BLT_MODE7_DU_ROW, duRow)
	refBus.Write32(BLT_MODE7_DV_ROW, dvRow)
	refBus.Write32(BLT_CTRL, 1)

	directBack := directMem[x86RotoBackBuffer : x86RotoBackBuffer+x86RotoLineBytes*x86RotoRenderH]
	refBack := refMem[x86RotoBackBuffer : x86RotoBackBuffer+x86RotoLineBytes*x86RotoRenderH]
	if bytes.Equal(directBack, refBack) {
		return
	}

	for i := range directBack {
		if directBack[i] != refBack[i] {
			t.Fatalf("back buffer mismatch at byte %d: got 0x%02X want 0x%02X", i, directBack[i], refBack[i])
		}
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

func TestX86RotozoomerRuntimeProducesVideo(t *testing.T) {
	rig := newEightBitRotoRig(t)
	if err := rig.video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = rig.video.Stop()
		rig.sound.Stop()
	})

	runner := NewCPUX86Runner(rig.bus, &CPUX86Config{
		LoadAddr:   0,
		Entry:      0,
		JITEnabled: true,
	})
	if err := runner.LoadProgram(filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86")); err != nil {
		t.Fatalf("load x86 rotozoomer: %v", err)
	}

	runner.StartExecution()
	time.Sleep(3 * time.Second)
	runner.Stop()

	// Slice 4 retired tryDemoAccelFrame: the JIT path now runs the
	// rotozoomer through general native dispatch instead of the
	// hand-coded frame shortcut, so demo-accel steps are expected to
	// stay 0. Video content verifies the JIT correctly produced frames.
	if got := runner.GetCPU().x86DemoAccelSteps.Load(); got != 0 {
		t.Errorf("x86 demo accelerator steps = %d, want 0 after slice-4 retire", got)
	}
	requireVideoContent(t, rig.video, 6)
}

func TestX86RotozoomerRuntimeProducesVideoNoJIT(t *testing.T) {
	rig := newEightBitRotoRig(t)
	if err := rig.video.Start(); err != nil {
		t.Fatalf("failed to start video chip: %v", err)
	}
	t.Cleanup(func() {
		_ = rig.video.Stop()
		rig.sound.Stop()
	})

	runner := NewCPUX86Runner(rig.bus, &CPUX86Config{
		LoadAddr:   0,
		Entry:      0,
		JITEnabled: false,
	})
	if err := runner.LoadProgram(filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86")); err != nil {
		t.Fatalf("load x86 rotozoomer: %v", err)
	}

	runner.StartExecution()
	time.Sleep(3 * time.Second)
	runner.Stop()

	if got := runner.GetCPU().x86DemoAccelSteps.Load(); got != 0 {
		t.Fatalf("x86 demo accelerator steps = %d, want 0 when JIT is disabled", got)
	}
	requireVideoContent(t, rig.video, 6)
}

func TestCPUX86Runner_LoadProgramData_DetectsExactRotozoomerBinary(t *testing.T) {
	bus := NewMachineBus()
	runner := NewCPUX86Runner(bus, &CPUX86Config{JITEnabled: true})

	data, err := os.ReadFile(filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86"))
	if err != nil {
		t.Fatalf("read rotozoomer binary: %v", err)
	}
	if err := runner.LoadProgramData(data); err != nil {
		t.Fatalf("load rotozoomer binary: %v", err)
	}

	if got := runner.GetCPU().x86DemoAccel; got != x86DemoAccelRotozoomer {
		t.Fatalf("x86 demo accel = %v, want %v", got, x86DemoAccelRotozoomer)
	}
}

func TestCPUX86Runner_LoadProgramData_RejectsModifiedRotozoomerBinary(t *testing.T) {
	bus := NewMachineBus()
	runner := NewCPUX86Runner(bus, &CPUX86Config{JITEnabled: true})

	data, err := os.ReadFile(filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86"))
	if err != nil {
		t.Fatalf("read rotozoomer binary: %v", err)
	}
	mutated := append([]byte(nil), data...)
	mutated[len(mutated)-1] ^= 0x01
	if err := runner.LoadProgramData(mutated); err != nil {
		t.Fatalf("load mutated rotozoomer binary: %v", err)
	}

	if got := runner.GetCPU().x86DemoAccel; got != x86DemoAccelNone {
		t.Fatalf("x86 demo accel = %v, want %v", got, x86DemoAccelNone)
	}
}

func TestCPUX86Runner_LoadProgramData_RejectsOtherX86Binary(t *testing.T) {
	bus := NewMachineBus()
	runner := NewCPUX86Runner(bus, &CPUX86Config{JITEnabled: true})

	data, err := os.ReadFile(filepath.Join("sdk", "examples", "prebuilt", "antic_plasma_x86.ie86"))
	if err != nil {
		t.Fatalf("read antic plasma binary: %v", err)
	}
	if err := runner.LoadProgramData(data); err != nil {
		t.Fatalf("load antic plasma binary: %v", err)
	}

	if got := runner.GetCPU().x86DemoAccel; got != x86DemoAccelNone {
		t.Fatalf("x86 demo accel = %v, want %v", got, x86DemoAccelNone)
	}
}
