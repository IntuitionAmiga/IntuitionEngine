// video_compositor_test.go - Video compositor optimization tests

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"testing"
)

// blendFrameReference is the original unoptimized implementation for correctness verification
func blendFrameReference(srcFrame []byte, srcW, srcH int, finalFrame []byte, dstW, dstH int) {
	for dstY := 0; dstY < dstH; dstY++ {
		srcY := dstY * srcH / dstH
		for dstX := 0; dstX < dstW; dstX++ {
			srcX := dstX * srcW / dstW

			srcIdx := (srcY*srcW + srcX) * BYTES_PER_PIXEL
			dstIdx := (dstY*dstW + dstX) * BYTES_PER_PIXEL

			if srcIdx+3 < len(srcFrame) && dstIdx+3 < len(finalFrame) {
				srcAlpha := srcFrame[srcIdx+3]
				if srcAlpha > 0 {
					finalFrame[dstIdx+0] = srcFrame[srcIdx+0]
					finalFrame[dstIdx+1] = srcFrame[srcIdx+1]
					finalFrame[dstIdx+2] = srcFrame[srcIdx+2]
					finalFrame[dstIdx+3] = srcFrame[srcIdx+3]
				}
			}
		}
	}
}

// TestBlendFrame_1to1_MatchesReference verifies 1:1 scaling produces identical results
func TestBlendFrame_1to1_MatchesReference(t *testing.T) {
	width := 640
	height := 480
	srcFrame := make([]byte, width*height*BYTES_PER_PIXEL)

	// Fill with test pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := (y*width + x) * BYTES_PER_PIXEL
			srcFrame[idx+0] = byte(x % 256)       // R
			srcFrame[idx+1] = byte(y % 256)       // G
			srcFrame[idx+2] = byte((x + y) % 256) // B
			srcFrame[idx+3] = 255                 // A (opaque)
		}
	}

	// Reference implementation result
	refFrame := make([]byte, width*height*BYTES_PER_PIXEL)
	blendFrameReference(srcFrame, width, height, refFrame, width, height)

	// Compositor result
	c := &VideoCompositor{
		frameWidth:  width,
		frameHeight: height,
		finalFrame:  make([]byte, width*height*BYTES_PER_PIXEL),
	}
	c.blendFrame(srcFrame, width, height)

	// Compare
	for i := 0; i < len(refFrame); i++ {
		if c.finalFrame[i] != refFrame[i] {
			pixel := i / BYTES_PER_PIXEL
			x := pixel % width
			y := pixel / width
			component := i % BYTES_PER_PIXEL
			t.Fatalf("1:1 scaling mismatch at pixel (%d,%d) component %d: got %d, expected %d",
				x, y, component, c.finalFrame[i], refFrame[i])
		}
	}
}

// TestBlendFrame_Upscale_MatchesReference verifies upscaling produces identical results
func TestBlendFrame_Upscale_MatchesReference(t *testing.T) {
	srcW, srcH := 320, 240
	dstW, dstH := 640, 480

	srcFrame := make([]byte, srcW*srcH*BYTES_PER_PIXEL)

	// Fill with test pattern
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			idx := (y*srcW + x) * BYTES_PER_PIXEL
			srcFrame[idx+0] = byte(x % 256)
			srcFrame[idx+1] = byte(y % 256)
			srcFrame[idx+2] = byte((x * y) % 256)
			srcFrame[idx+3] = 255
		}
	}

	// Reference result
	refFrame := make([]byte, dstW*dstH*BYTES_PER_PIXEL)
	blendFrameReference(srcFrame, srcW, srcH, refFrame, dstW, dstH)

	// Compositor result
	c := &VideoCompositor{
		frameWidth:  dstW,
		frameHeight: dstH,
		finalFrame:  make([]byte, dstW*dstH*BYTES_PER_PIXEL),
	}
	c.blendFrame(srcFrame, srcW, srcH)

	// Compare
	for i := 0; i < len(refFrame); i++ {
		if c.finalFrame[i] != refFrame[i] {
			pixel := i / BYTES_PER_PIXEL
			x := pixel % dstW
			y := pixel / dstW
			component := i % BYTES_PER_PIXEL
			t.Fatalf("Upscale mismatch at pixel (%d,%d) component %d: got %d, expected %d",
				x, y, component, c.finalFrame[i], refFrame[i])
		}
	}
}

// TestBlendFrame_Downscale_MatchesReference verifies downscaling produces identical results
func TestBlendFrame_Downscale_MatchesReference(t *testing.T) {
	srcW, srcH := 1024, 768
	dstW, dstH := 640, 480

	srcFrame := make([]byte, srcW*srcH*BYTES_PER_PIXEL)

	// Fill with test pattern
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			idx := (y*srcW + x) * BYTES_PER_PIXEL
			srcFrame[idx+0] = byte(x % 256)
			srcFrame[idx+1] = byte(y % 256)
			srcFrame[idx+2] = byte((x ^ y) % 256)
			srcFrame[idx+3] = 255
		}
	}

	// Reference result
	refFrame := make([]byte, dstW*dstH*BYTES_PER_PIXEL)
	blendFrameReference(srcFrame, srcW, srcH, refFrame, dstW, dstH)

	// Compositor result
	c := &VideoCompositor{
		frameWidth:  dstW,
		frameHeight: dstH,
		finalFrame:  make([]byte, dstW*dstH*BYTES_PER_PIXEL),
	}
	c.blendFrame(srcFrame, srcW, srcH)

	// Compare
	for i := 0; i < len(refFrame); i++ {
		if c.finalFrame[i] != refFrame[i] {
			pixel := i / BYTES_PER_PIXEL
			x := pixel % dstW
			y := pixel / dstW
			component := i % BYTES_PER_PIXEL
			t.Fatalf("Downscale mismatch at pixel (%d,%d) component %d: got %d, expected %d",
				x, y, component, c.finalFrame[i], refFrame[i])
		}
	}
}

// TestBlendFrame_AlphaHandling verifies transparent pixels are not copied
func TestBlendFrame_AlphaHandling(t *testing.T) {
	width, height := 100, 100
	srcFrame := make([]byte, width*height*BYTES_PER_PIXEL)

	// Fill with checkerboard: alternating opaque and transparent
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := (y*width + x) * BYTES_PER_PIXEL
			srcFrame[idx+0] = 255 // R
			srcFrame[idx+1] = 0   // G
			srcFrame[idx+2] = 0   // B
			if (x+y)%2 == 0 {
				srcFrame[idx+3] = 255 // Opaque
			} else {
				srcFrame[idx+3] = 0 // Transparent
			}
		}
	}

	// Pre-fill destination with blue
	c := &VideoCompositor{
		frameWidth:  width,
		frameHeight: height,
		finalFrame:  make([]byte, width*height*BYTES_PER_PIXEL),
	}
	for i := 0; i < len(c.finalFrame); i += BYTES_PER_PIXEL {
		c.finalFrame[i+0] = 0   // R
		c.finalFrame[i+1] = 0   // G
		c.finalFrame[i+2] = 255 // B
		c.finalFrame[i+3] = 255 // A
	}

	c.blendFrame(srcFrame, width, height)

	// Verify: opaque pixels should be red, transparent should remain blue
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := (y*width + x) * BYTES_PER_PIXEL
			if (x+y)%2 == 0 {
				// Should be red (opaque source)
				if c.finalFrame[idx+0] != 255 || c.finalFrame[idx+1] != 0 || c.finalFrame[idx+2] != 0 {
					t.Fatalf("Opaque pixel at (%d,%d) not copied correctly: got RGB(%d,%d,%d)",
						x, y, c.finalFrame[idx+0], c.finalFrame[idx+1], c.finalFrame[idx+2])
				}
			} else {
				// Should remain blue (transparent source)
				if c.finalFrame[idx+0] != 0 || c.finalFrame[idx+1] != 0 || c.finalFrame[idx+2] != 255 {
					t.Fatalf("Transparent pixel at (%d,%d) should not be copied: got RGB(%d,%d,%d)",
						x, y, c.finalFrame[idx+0], c.finalFrame[idx+1], c.finalFrame[idx+2])
				}
			}
		}
	}
}

// TestBlendFrame_EmptySource verifies empty source doesn't crash
func TestBlendFrame_EmptySource(t *testing.T) {
	c := &VideoCompositor{
		frameWidth:  640,
		frameHeight: 480,
		finalFrame:  make([]byte, 640*480*BYTES_PER_PIXEL),
	}

	// Empty source frame
	srcFrame := []byte{}

	// Should not panic
	c.blendFrame(srcFrame, 0, 0)
}

// TestBlendFrame_ZeroAlphaPreservesDestination verifies zero-alpha source preserves destination
func TestBlendFrame_ZeroAlphaPreservesDestination(t *testing.T) {
	width, height := 64, 64

	// Source: all transparent red
	srcFrame := make([]byte, width*height*BYTES_PER_PIXEL)
	for i := 0; i < len(srcFrame); i += BYTES_PER_PIXEL {
		srcFrame[i+0] = 255 // R
		srcFrame[i+1] = 0   // G
		srcFrame[i+2] = 0   // B
		srcFrame[i+3] = 0   // A = transparent
	}

	// Destination: solid green
	c := &VideoCompositor{
		frameWidth:  width,
		frameHeight: height,
		finalFrame:  make([]byte, width*height*BYTES_PER_PIXEL),
	}
	for i := 0; i < len(c.finalFrame); i += BYTES_PER_PIXEL {
		c.finalFrame[i+0] = 0   // R
		c.finalFrame[i+1] = 255 // G
		c.finalFrame[i+2] = 0   // B
		c.finalFrame[i+3] = 255 // A
	}

	c.blendFrame(srcFrame, width, height)

	// All pixels should remain green
	for i := 0; i < len(c.finalFrame); i += BYTES_PER_PIXEL {
		if c.finalFrame[i+0] != 0 || c.finalFrame[i+1] != 255 || c.finalFrame[i+2] != 0 {
			t.Fatalf("Destination pixel at %d not preserved: got RGB(%d,%d,%d)",
				i/BYTES_PER_PIXEL, c.finalFrame[i+0], c.finalFrame[i+1], c.finalFrame[i+2])
		}
	}
}

// BenchmarkBlendFrame_640x480 measures blendFrame performance for standard resolution
func BenchmarkBlendFrame_640x480(b *testing.B) {
	width, height := 640, 480
	srcFrame := make([]byte, width*height*BYTES_PER_PIXEL)

	// Fill with test data
	for i := 0; i < len(srcFrame); i += BYTES_PER_PIXEL {
		srcFrame[i+0] = byte(i % 256)
		srcFrame[i+1] = byte((i / 4) % 256)
		srcFrame[i+2] = byte((i / 16) % 256)
		srcFrame[i+3] = 255
	}

	c := &VideoCompositor{
		frameWidth:  width,
		frameHeight: height,
		finalFrame:  make([]byte, width*height*BYTES_PER_PIXEL),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.blendFrame(srcFrame, width, height)
	}
}

// BenchmarkBlendFrame_1024x768 measures blendFrame performance for higher resolution
func BenchmarkBlendFrame_1024x768(b *testing.B) {
	srcW, srcH := 1024, 768
	dstW, dstH := 640, 480
	srcFrame := make([]byte, srcW*srcH*BYTES_PER_PIXEL)

	for i := 0; i < len(srcFrame); i += BYTES_PER_PIXEL {
		srcFrame[i+0] = byte(i % 256)
		srcFrame[i+1] = byte((i / 4) % 256)
		srcFrame[i+2] = byte((i / 16) % 256)
		srcFrame[i+3] = 255
	}

	c := &VideoCompositor{
		frameWidth:  dstW,
		frameHeight: dstH,
		finalFrame:  make([]byte, dstW*dstH*BYTES_PER_PIXEL),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.blendFrame(srcFrame, srcW, srcH)
	}
}

// BenchmarkBlendFrame_Upscale_320x240_to_640x480 measures upscale performance
func BenchmarkBlendFrame_Upscale_320x240_to_640x480(b *testing.B) {
	srcW, srcH := 320, 240
	dstW, dstH := 640, 480
	srcFrame := make([]byte, srcW*srcH*BYTES_PER_PIXEL)

	for i := 0; i < len(srcFrame); i += BYTES_PER_PIXEL {
		srcFrame[i+0] = byte(i % 256)
		srcFrame[i+1] = byte((i / 4) % 256)
		srcFrame[i+2] = byte((i / 16) % 256)
		srcFrame[i+3] = 255
	}

	c := &VideoCompositor{
		frameWidth:  dstW,
		frameHeight: dstH,
		finalFrame:  make([]byte, dstW*dstH*BYTES_PER_PIXEL),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.blendFrame(srcFrame, srcW, srcH)
	}
}
