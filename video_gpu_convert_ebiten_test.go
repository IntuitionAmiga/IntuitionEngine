// video_gpu_convert_ebiten_test.go - GPU CLUT8 conversion render and readback gate.

//go:build !headless

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func init() {
	gpuGateBodies["clut8"] = gpuGateCLUT8Differential
}

// runGPUGate runs a registered gate body in a re-executed copy of this binary,
// because Ebiten needs the main OS thread to render or read pixels back; see
// video_gpu_convert_gate.go. It skips when no backend can be started rather
// than failing, so headless and display-less machines still run the suite.
func runGPUGate(t *testing.T, name string) {
	t.Helper()
	// Only the Unix display backends advertise themselves through the
	// environment. Windows and macOS have a native backend with no such
	// variable, so asking about DISPLAY there would skip on every supported
	// desktop; the child reports backend availability through exit status 3.
	if usesUnixDisplayEnv() && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no display reachable; this gate needs a real graphics backend")
	}
	cmd := exec.Command(os.Args[0], "-test.run=XXXNoTestMatchesThisName")
	cmd.Env = append(os.Environ(), gpuGateEnv+"="+name)
	out, err := runWithDeadline(cmd, 90*time.Second)
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "exit status 3") {
		t.Skipf("graphics backend unavailable: %s", out)
	}
	t.Fatalf("gate %q failed: %v\n%s", name, err, out)
}

// usesUnixDisplayEnv reports whether this platform names its display server in
// the environment, which is what makes an absent DISPLAY meaningful.
func usesUnixDisplayEnv() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "js", "android", "ios":
		return false
	default:
		return true
	}
}

// runWithDeadline runs cmd and kills it if it outlives the deadline, so a
// backend that hangs on window creation fails the test rather than the suite.
func runWithDeadline(cmd *exec.Cmd, limit time.Duration) (string, error) {
	var sb strings.Builder
	cmd.Stdout = &sb
	cmd.Stderr = &sb
	if err := cmd.Start(); err != nil {
		return sb.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return sb.String(), err
	case <-time.After(limit):
		_ = cmd.Process.Kill()
		<-done
		return sb.String(), os.ErrDeadlineExceeded
	}
}

// TestGPUConvertReadback_MatchesCPU_CLUT8 is the gate the tranche plan requires
// before GPU conversion can be trusted for a format: a real Kage render of a
// CLUT8 frame, read back and compared against the CPU expander. The pure-Go
// mirror in video_gpu_convert_test.go cannot cover the compiled shader, the
// texture formats, the bindings, the palette upload or the coordinate rules.
func TestGPUConvertReadback_MatchesCPU_CLUT8(t *testing.T) {
	runGPUGate(t, "clut8")
}

// gpuGateCLUT8Differential converts indexed frames on the GPU and compares the
// readback against the CPU expander, which is the canonical oracle for CLUT8.
// Frame widths straddle the four-indices-per-texel packing so that every
// remainder is covered, and a palette change after the first frame checks that
// the palette rows of the retained texture are actually rewritten.
func gpuGateCLUT8Differential() error {
	rng := rand.New(rand.NewSource(20260723))
	var conv clut8GPUConverter

	for _, dims := range [][2]int{{4, 2}, {5, 3}, {6, 1}, {7, 4}, {320, 8}, {17, 17}} {
		w, h := dims[0], dims[1]
		var pal [256]uint32
		for i := range pal {
			// Opaque entries, as CLUT8 palettes are, with a few non-opaque ones
			// to catch a channel order mistake.
			pal[i] = uint32(rng.Intn(1<<24)) | 0xFF000000
			if i%37 == 0 {
				pal[i] = uint32(rng.Intn(1 << 24))
			}
		}
		indices := make([]byte, w*h)
		for i := range indices {
			indices[i] = byte(rng.Intn(256))
		}
		if len(indices) >= 256 {
			for i := range 256 {
				indices[i] = byte(i)
			}
		}
		if err := gpuGateCompareFrame(&conv, indices, w, h, &pal); err != nil {
			return err
		}
	}

	// Palette update on a retained texture.
	const w, h = 8, 2
	indices := make([]byte, w*h)
	for i := range indices {
		indices[i] = byte(i)
	}
	var pal [256]uint32
	for i := range pal {
		pal[i] = 0xFF000000 | uint32(i)
	}
	if err := gpuGateCompareFrame(&conv, indices, w, h, &pal); err != nil {
		return fmt.Errorf("before the palette change: %w", err)
	}
	for i := range pal {
		pal[i] = 0xFF000000 | uint32(255-i)<<8
	}
	if err := gpuGateCompareFrame(&conv, indices, w, h, &pal); err != nil {
		return fmt.Errorf("after the palette change: %w", err)
	}
	return nil
}

func gpuGateCompareFrame(conv *clut8GPUConverter, indices []byte, w, h int, pal *[256]uint32) error {
	img, err := conv.Convert(indices, w, h, pal)
	if err != nil {
		return fmt.Errorf("%dx%d: convert: %w", w, h, err)
	}
	got := make([]byte, w*h*BYTES_PER_PIXEL)
	img.ReadPixels(got)

	want := make([]byte, w*h*BYTES_PER_PIXEL)
	clut8ExpandSpanScalar(want, indices, pal)

	for i := range want {
		if got[i] != want[i] {
			pixel := i / BYTES_PER_PIXEL
			return fmt.Errorf("%dx%d: pixel %d (index %d) byte %d: GPU %#02x, CPU %#02x",
				w, h, pixel, indices[pixel], i%BYTES_PER_PIXEL, got[i], want[i])
		}
	}
	return nil
}
