// video_gpu_convert_bench_test.go - end-to-end frame time for CLUT8 conversion.

//go:build !headless

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"image"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func init() {
	gpuGateBodies["clut8_bench"] = gpuGateCLUT8Benchmark
}

// TestGPUConvertFrameTime measures the whole path, source to presented frame,
// with the CLUT8 layer expanded on the GPU and on the CPU. It is opt-in with
// IE_GPU_BENCH=1 because it needs a real backend and takes several seconds.
//
// Go benchmarks cannot host this: Ebiten needs the main OS thread, so the
// timing runs inside a gate body and reports through the child's output.
func TestGPUConvertFrameTime(t *testing.T) {
	if os.Getenv("IE_GPU_BENCH") != "1" {
		t.Skip("set IE_GPU_BENCH=1 to measure GPU conversion frame time")
	}
	t.Log("\n" + runGPUGateOutput(t, "clut8_bench"))
}

// gpuBenchFrames is the measured frame count per arm, after warm-up.
const gpuBenchFrames = 120

func gpuGateCLUT8Benchmark() error {
	// Two shapes: the classic 320x200 CLUT8 frame, and a 640x480 one, where
	// the expansion the GPU takes over is four times the work.
	for _, geom := range [][4]int{{320, 200, 1280, 800}, {640, 480, 1280, 960}} {
		if err := gpuBenchGeometry(geom[0], geom[1], geom[2], geom[3]); err != nil {
			return err
		}
	}
	return nil
}

func gpuBenchGeometry(srcW, srcH, dstW, dstH int) error {
	fmt.Printf("clut8 frame time, %d frames per repeat, %dx%d source presented %dx%d\n",
		gpuBenchFrames, srcW, srcH, dstW, dstH)

	// Two modes, because they answer different questions. Reading a pixel back
	// every frame forces the queued GPU work to finish inside the frame, which
	// is the honest measure of the work itself. Reading back once at the end
	// lets the driver pipeline, which is closer to how frames actually present.
	for _, mode := range []struct {
		name     string
		syncEach bool
	}{
		{"sync per frame", true},
		{"pipelined", false},
	} {
		// Arms alternate within each repeat so clock and thermal drift hits
		// both equally, as a sequential A-then-B comparison would not.
		var gpuTotal, cpuTotal time.Duration
		const repeats = 3
		for range repeats {
			gpu, err := gpuBenchArm(srcW, srcH, dstW, dstH, true, mode.syncEach)
			if err != nil {
				return fmt.Errorf("gpu_expand: %w", err)
			}
			cpu, err := gpuBenchArm(srcW, srcH, dstW, dstH, false, mode.syncEach)
			if err != nil {
				return fmt.Errorf("cpu_expand: %w", err)
			}
			gpuTotal += gpu
			cpuTotal += cpu
		}
		per := func(d time.Duration) float64 {
			return float64(d.Nanoseconds()) / float64(gpuBenchFrames*repeats) / 1000
		}
		delta := float64(gpuTotal-cpuTotal) / float64(cpuTotal) * 100
		fmt.Printf("  %-15s cpu_expand %8.1f us/frame   gpu_expand %8.1f us/frame  (%+.1f%%)\n",
			mode.name, per(cpuTotal), per(gpuTotal), delta)
	}
	return nil
}

// gpuBenchArm runs one arm and returns the total time for the measured frames.
// Each frame publishes a changed source frame, composites, draws, and reads one
// pixel back, which forces the queued GPU work to complete before the frame is
// counted. Without that the loop would only measure command submission.
func gpuBenchArm(srcW, srcH, dstW, dstH int, indexed, syncEach bool) (time.Duration, error) {
	out, err := NewEbitenOutput()
	if err != nil {
		return 0, fmt.Errorf("NewEbitenOutput: %w", err)
	}
	eo := out.(*EbitenOutput)
	eo.showStatusBar = false
	if err := eo.SetDisplayConfig(DisplayConfig{Width: dstW, Height: dstH, Scale: 1, PixelFormat: PixelFormatRGBA}); err != nil {
		return 0, fmt.Errorf("SetDisplayConfig: %w", err)
	}
	// Start would launch a second Ebiten run loop, which cannot coexist with
	// the gate's. The benchmark drives Draw itself, so it only needs the
	// backend to look started, which is what the compositor checks before it
	// forwards a hardware frame.
	eo.running.Store(true)

	comp := NewVideoCompositor(out)
	comp.LockResolution(dstW, dstH)
	src := newIndexedTestSource(srcW, srcH, indexed, 99)
	comp.RegisterSource(src)

	screen := ebiten.NewImage(dstW, dstH)
	probe := make([]byte, BYTES_PER_PIXEL)

	sync := func() {
		screen.SubImage(image.Rect(0, 0, 1, 1)).(*ebiten.Image).ReadPixels(probe)
	}
	frame := func(n int) {
		// Change the frame so nothing upstream can skip the work.
		src.indices[n%len(src.indices)] = byte(n)
		comp.composite()
		eo.Draw(screen)
		if syncEach {
			sync()
		}
	}

	for n := range 30 {
		frame(n)
	}
	sync()
	start := time.Now()
	for n := range gpuBenchFrames {
		frame(n + 30)
	}
	// Always finish on a completed frame, so the pipelined arm still counts
	// all the work it queued rather than leaving it in the driver.
	sync()
	return time.Since(start), nil
}
