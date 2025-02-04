// video_chip_test.go - Video chip test suite/tech demos for Intuition Engine

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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func TestDrawColourPalette(t *testing.T) {
	// Get current mode dimensions
	mode := VideoModes[MODE_640x480]
	totalPixels := mode.width * mode.height

	// Calculate color increments
	rInc := 255.0 / float64(mode.width)
	gInc := 255.0 / float64(mode.height)
	bInc := 255.0 / float64(totalPixels)

	// Draw the palette
	for y := 0; y < mode.height; y++ {
		for x := 0; x < mode.width; x++ {
			pixelIndex := y*mode.width + x

			// Calculate RGB values
			r := uint32(float64(x) * rInc)
			g := uint32(float64(y) * gInc)
			b := uint32(float64(pixelIndex) * bInc)

			// Ensure values don't exceed 255
			if r > 255 {
				r = 255
			}
			if g > 255 {
				g = 255
			}
			if b > 255 {
				b = 255
			}

			// Create RGBA value (full opacity)
			color := (r << 24) | (g << 16) | (b << 8) | 0xFF

			// Calculate memory address for this pixel
			addr := VRAM_START + uint32(pixelIndex*4)

			// Write the color to VRAM
			videoChip.HandleWrite(addr, color)
		}
	}

	// Wait a bit to ensure the frame is rendered
	time.Sleep(time.Second * 2)

	// Verify that content was written
	if status := videoChip.HandleRead(VIDEO_STATUS); status == 0 {
		t.Error("Expected video content to be present, but status indicates no content")
	}

	// Sample test points to verify color gradient
	testPoints := []struct {
		x, y      int
		expectedR uint32
		expectedG uint32
		expectedB uint32
	}{
		{0, 0, 0, 0, 0},                                  // Top-left (black)
		{mode.width - 1, 0, 255, 0, 127},                 // Top-right
		{0, mode.height - 1, 0, 255, 127},                // Bottom-left
		{mode.width - 1, mode.height - 1, 255, 255, 255}, // Bottom-right
	}

	for _, point := range testPoints {
		addr := VRAM_START + uint32((point.y*mode.width+point.x)*4)
		color := videoChip.HandleRead(addr)

		r := (color >> 24) & 0xFF
		g := (color >> 16) & 0xFF
		b := (color >> 8) & 0xFF

		// Allow for some color value tolerance due to rounding
		tolerance := uint32(2)

		if absUint32(r, point.expectedR) > tolerance ||
			absUint32(g, point.expectedG) > tolerance ||
			absUint32(b, point.expectedB) > tolerance {
			t.Errorf("Color mismatch at (%d,%d): got RGB(%d,%d,%d), expected RGB(%d,%d,%d)",
				point.x, point.y, r, g, b,
				point.expectedR, point.expectedG, point.expectedB)
		}
	}
}

func TestRotatingCube(t *testing.T) {
	mode := VideoModes[MODE_640x480]

	// Define cube vertices
	vertices := []struct{ x, y, z float64 }{
		{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
		{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
	}

	// Define edges between vertices
	edges := [][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0},
		{4, 5}, {5, 6}, {6, 7}, {7, 4},
		{0, 4}, {1, 5}, {2, 6}, {3, 7},
	}

	startTime := time.Now()

	// Animate for 5 seconds
	for time.Since(startTime) < 5*time.Second {
		// Clear screen
		clearScreen(videoChip, mode)

		timeVal := time.Since(startTime).Seconds()

		// Calculate rotation angles
		angleX := timeVal * 0.5
		angleY := timeVal * 0.8
		angleZ := timeVal * 0.3

		// Project and draw each edge
		for _, edge := range edges {
			// Get vertices for this edge
			v1 := vertices[edge[0]]
			v2 := vertices[edge[1]]

			// Rotate first vertex
			x1, y1, z1 := rotate3D(v1.x, v1.y, v1.z, angleX, angleY, angleZ)
			x2, y2, z2 := rotate3D(v2.x, v2.y, v2.z, angleX, angleY, angleZ)

			// Project to 2D
			scale := 100.0
			centerX := float64(mode.width) / 2
			centerY := float64(mode.height) / 2

			projX1 := centerX + x1*scale/(z1+3)
			projY1 := centerY + y1*scale/(z1+3)
			projX2 := centerX + x2*scale/(z2+3)
			projY2 := centerY + y2*scale/(z2+3)

			// Draw line
			drawLine(videoChip, mode,
				int(projX1), int(projY1),
				int(projX2), int(projY2),
				0xFFFFFFFF)
		}

		time.Sleep(time.Millisecond * 16)
	}
}

func TestFireEffect(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	buffer := make([][]uint8, mode.height)
	for i := range buffer {
		buffer[i] = make([]uint8, mode.width)
	}

	// Fire palette from black to white through red/yellow
	palette := make([]uint32, 256)
	for i := 0; i < 256; i++ {
		r := min(255, i*2)
		g := max(0, min(255, (i-64)*4))
		b := max(0, min(255, (i-128)*4))
		palette[i] = (uint32(r) << 24) | (uint32(g) << 16) | (uint32(b) << 8) | 0xFF
	}

	startTime := time.Now()
	for time.Since(startTime) < 5*time.Second {
		// Random noise at bottom
		for x := 0; x < mode.width; x++ {
			buffer[mode.height-1][x] = uint8(rand.Intn(256))
		}

		// Fire propagation
		for y := 0; y < mode.height-1; y++ {
			for x := 0; x < mode.width; x++ {
				sum := 0
				count := 0

				// Sample neighboring pixels
				for dx := -1; dx <= 1; dx++ {
					newX := x + dx
					if newX >= 0 && newX < mode.width {
						sum += int(buffer[y+1][newX])
						count++
					}
				}

				// Average and decay
				if count > 0 {
					value := sum / count
					if value > 0 {
						value -= 1 // Decay rate
					}
					buffer[y][x] = uint8(value)
				}
			}
		}

		// Render buffer to screen
		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, palette[buffer[y][x]])
			}
		}

		time.Sleep(time.Millisecond * 16)
	}
}

func TestSineScroller(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	text := "..INTUITION ENGINE.."
	textLen := len(text)
	charWidth := 32 // 32x32 font
	charHeight := 32

	startTime := time.Now()
	xOffset := 0

	// 32x32 font data - each letter is 32 rows of 32 bits
	fontData := map[rune][32]uint32{
		'A': {
			0x000FF000, 0x001FF800, 0x003FFC00, 0x007FFE00,
			0x00FFFF00, 0x01FFFF80, 0x03FFFFC0, 0x07FFFFE0,
			0x0FFFFFF0, 0x1FFFFFF8, 0x3FF00FFC, 0x7FE007FE,
			0x7FC003FE, 0xFFC003FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFFFFFFFF, 0xFFFFFFFF,
			0xFFFFFFFF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
		},
		'B': {
			0xFFFFFE00, 0xFFFFFF00, 0xFFFFFF80, 0xFF0007C0,
			0xFF0003E0, 0xFF0001E0, 0xFF0001E0, 0xFF0001E0,
			0xFF0001E0, 0xFF0003C0, 0xFF0007C0, 0xFFFFFF80,
			0xFFFFFF00, 0xFFFFFF80, 0xFF0007C0, 0xFF0003E0,
			0xFF0001E0, 0xFF0001E0, 0xFF0001E0, 0xFF0001E0,
			0xFF0001E0, 0xFF0001E0, 0xFF0001E0, 0xFF0001E0,
			0xFF0003E0, 0xFF0007C0, 0xFF800FC0, 0xFFFFFF80,
			0xFFFFFF00, 0xFFFFFE00, 0xFFFFFC00, 0xFFFFF800,
		},
		'C': {
			0x07FFFC00, 0x0FFFFF00, 0x1FFFFF80, 0x3FFFFFC0,
			0x7FFFFFE0, 0x7FC007E0, 0xFF0001F0, 0xFF0001F0,
			0xFE0000F8, 0xFE000078, 0xFE000078, 0xFE000000,
			0xFE000000, 0xFE000000, 0xFE000000, 0xFE000000,
			0xFE000000, 0xFE000000, 0xFE000000, 0xFE000000,
			0xFE000078, 0xFE000078, 0xFE0000F8, 0xFF0001F0,
			0xFF0001F0, 0x7FC007E0, 0x7FFFFFE0, 0x3FFFFFC0,
			0x1FFFFF80, 0x0FFFFF00, 0x07FFFC00, 0x01FFE000,
		},
		'D': {
			0xFFFFE000, 0xFFFFF800, 0xFFFFFC00, 0xFFFFFE00,
			0xFF007F00, 0xFF001F80, 0xFF000F80, 0xFF0007C0,
			0xFF0003C0, 0xFF0003C0, 0xFF0001E0, 0xFF0001E0,
			0xFF0001E0, 0xFF0001E0, 0xFF0001E0, 0xFF0001E0,
			0xFF0001E0, 0xFF0001E0, 0xFF0001E0, 0xFF0001E0,
			0xFF0001E0, 0xFF0003C0, 0xFF0003C0, 0xFF0007C0,
			0xFF000F80, 0xFF001F80, 0xFF007F00, 0xFFFFFE00,
			0xFFFFFC00, 0xFFFFF800, 0xFFFFE000, 0xFFFFC000,
		},
		'E': {
			0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFFFFFF00,
			0xFFFFFF00, 0xFFFFFF00, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFFFFFFFF,
			0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
		},
		'F': {
			0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFFFFFF00,
			0xFFFFFF00, 0xFFFFFF00, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
		},
		'G': {
			0x07FFFC00, 0x0FFFFF00, 0x1FFFFF80, 0x3FFFFFC0,
			0x7FFFFFE0, 0x7FC007E0, 0xFF0001F0, 0xFF0001F0,
			0xFE0000F8, 0xFE000000, 0xFE000000, 0xFE000000,
			0xFE000000, 0xFE0FFFFF, 0xFE0FFFFF, 0xFE0FFFFF,
			0xFE0001F8, 0xFE0001F8, 0xFE0001F8, 0xFE0001F8,
			0xFE0001F8, 0xFE0001F8, 0xFE0001F8, 0xFF0001F0,
			0xFF0001F0, 0x7FC007E0, 0x7FFFFFE0, 0x3FFFFFC0,
			0x1FFFFF80, 0x0FFFFF00, 0x07FFFC00, 0x01FFE000,
		},
		'H': {
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFF0001FF,
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFF0001FF,
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFFFFFFFF,
			0xFFFFFFFF, 0xFFFFFFFF, 0xFF0001FF, 0xFF0001FF,
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFF0001FF,
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFF0001FF,
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFF0001FF,
			0xFF0001FF, 0xFF0001FF, 0xFF0001FF, 0xFF0001FF,
		},
		'I': {
			0x1FFFFFF8, 0x1FFFFFF8, 0x1FFFFFF8, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x1FFFFFF8,
			0x1FFFFFF8, 0x1FFFFFF8, 0x1FFFFFF8, 0x1FFFFFF8,
		},
		'L': {
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFF800000,
			0xFF800000, 0xFF800000, 0xFF800000, 0xFFFFFFFF,
			0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
		},
		'M': {
			0xFF0000FF, 0xFF8001FF, 0xFFC003FF, 0xFFE007FF,
			0xFFF00FFF, 0xFFF81FFF, 0xFFFC3FFF, 0xFFFE7FFF,
			0xFFFFFFFF, 0xFFFFFFFF, 0xFF8FF9FF, 0xFF87F1FF,
			0xFF83E1FF, 0xFF81C1FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
		},
		'N': {
			0xFF8001FF, 0xFF8001FF, 0xFFC001FF, 0xFFE001FF,
			0xFFF001FF, 0xFFF801FF, 0xFFFC01FF, 0xFFFE01FF,
			0xFFFF01FF, 0xFF8F81FF, 0xFF87C1FF, 0xFF83E1FF,
			0xFF81F1FF, 0xFF80F9FF, 0xFF807DFF, 0xFF803FFF,
			0xFF801FFF, 0xFF800FFF, 0xFF8007FF, 0xFF8003FF,
			0xFF8001FF, 0xFF8000FF, 0xFF80007F, 0xFF80003F,
			0xFF80001F, 0xFF80000F, 0xFF800007, 0xFF800003,
			0xFF800001, 0xFF800000, 0xFF800000, 0xFF800000,
		},
		'O': {
			0x07FFFC00, 0x1FFFFF00, 0x3FFFFFC0, 0x7FFFFFE0,
			0xFFFFFFF0, 0xFF8007F8, 0xFF0001FC, 0xFE0000FE,
			0xFC00007E, 0xFC00007F, 0xF800003F, 0xF800003F,
			0xF800003F, 0xF800003F, 0xF800003F, 0xF800003F,
			0xF800003F, 0xF800003F, 0xF800003F, 0xF800003F,
			0xF800003F, 0xF800003F, 0xFC00007F, 0xFC00007E,
			0xFE0000FE, 0xFF0001FC, 0xFF8007F8, 0xFFFFFFF0,
			0x7FFFFFE0, 0x3FFFFFC0, 0x1FFFFF00, 0x07FFFC00,
		},
		'R': {
			0xFFFFFE00, 0xFFFFFF00, 0xFFFFFF80, 0xFF0007C0,
			0xFF0003E0, 0xFF0001E0, 0xFF0001E0, 0xFF0001E0,
			0xFF0001E0, 0xFF0003C0, 0xFF0007C0, 0xFFFFFF80,
			0xFFFFFF00, 0xFFFFFF80, 0xFF0007C0, 0xFF0003E0,
			0xFF0001F0, 0xFF0001F8, 0xFF0000FC, 0xFF00007E,
			0xFF00003F, 0xFF00001F, 0xFF00000F, 0xFF000007,
			0xFF000003, 0xFF000001, 0xFF000001, 0xFF000000,
			0xFF000000, 0xFF000000, 0xFF000000, 0xFF000000,
		},
		'S': {
			0x07FFFC00, 0x1FFFFF00, 0x3FFFFF80, 0x7FFFFFC0,
			0x7FFFFFE0, 0xFF8000E0, 0xFF000070, 0xFF000070,
			0xFF800070, 0x7FC00000, 0x3FFFFF00, 0x1FFFFF80,
			0x07FFFFC0, 0x01FFFFE0, 0x0007FFF0, 0x00003FF0,
			0x00000FF0, 0x000007F0, 0x000003F0, 0xE00003F0,
			0xF00003F0, 0xF80003F0, 0xFC0003F0, 0xFE0007E0,
			0xFF001FC0, 0x7FFFFC00, 0x3FFFF800, 0x1FFFF000,
			0x0FFFE000, 0x07FFC000, 0x03FF8000, 0x01FF0000,
		},
		'T': {
			0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
			0x001FF000, 0x001FF000, 0x001FF000, 0x001FF000,
		},
		'U': {
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8001FF,
			0xFF8001FF, 0xFF8001FF, 0xFF8001FF, 0xFF8003FE,
			0xFF8003FE, 0x7FC007FE, 0x7FE00FFC, 0x3FF03FF8,
			0x1FFFFFF0, 0x0FFFFFC0, 0x07FFFF80, 0x01FFFC00,
		},
		',': {
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0xF0000000, 0xF0000000, 0x70000000, 0x30000000,
		},
		' ': {
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
		},
		'-': {
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0,
		},
		'.': {
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0xFFF00000, 0xFFF00000, 0xFFF00000, 0,
		},
		'8': {
			0x07FFFC00, 0x1FFFFF00, 0x3FFFFFC0, 0x7FFFFFE0,
			0xFFFFFFF0, 0xFF8007F8, 0xFF0001FC, 0xFF0001FC,
			0xFF0001FC, 0xFF0001FC, 0xFF0001FC, 0x7F8007F8,
			0x3FFFFFC0, 0x1FFFFF00, 0x3FFFFFC0, 0x7FFFFFE0,
			0xFF8007F8, 0xFF0001FC, 0xFF0001FC, 0xFF0001FC,
			0xFF0001FC, 0xFF0001FC, 0xFF0001FC, 0xFF0001FC,
			0xFF0001FC, 0xFF8007F8, 0xFFFFFFF0, 0x7FFFFFE0,
			0x3FFFFFC0, 0x1FFFFF00, 0x07FFFC00, 0x00000000,
		},
	}

	for time.Since(startTime) < 10*time.Second {
		clearScreen(videoChip, mode)
		timeVal := float64(time.Since(startTime).Milliseconds()) / 1000.0

		for charPos := 0; charPos < textLen; charPos++ {
			char := rune(text[charPos])
			if fontChar, exists := fontData[char]; exists {
				// Calculate position for this character
				baseX := ((charPos * charWidth) - xOffset) % (textLen * charWidth)
				if baseX < -charWidth {
					baseX += textLen * charWidth
				}

				// Skip if character is completely off-screen
				if baseX > mode.width {
					continue
				}

				// Calculate sine wave offset
				sineOffset := int(math.Sin(timeVal*2.0+float64(charPos)*0.3) * 60.0)
				baseY := (mode.height / 2) + sineOffset - (charHeight / 2)

				// Draw character
				for y := 0; y < charHeight; y++ {
					row := fontChar[y]
					for x := 0; x < charWidth; x++ {
						if (row & (1 << (31 - x))) != 0 {
							screenX := baseX + x
							screenY := baseY + y

							if screenX >= 0 && screenX < mode.width &&
								screenY >= 0 && screenY < mode.height {
								addr := VRAM_START + uint32((screenY*mode.width+screenX)*4)
								// Rainbow color effect
								hue := float64(time.Since(startTime).Milliseconds())/1000.0 + float64(screenX)/100.0
								r := uint32((math.Sin(hue) + 1.0) * 127)
								g := uint32((math.Sin(hue+2.0*math.Pi/3.0) + 1.0) * 127)
								b := uint32((math.Sin(hue+4.0*math.Pi/3.0) + 1.0) * 127)
								color := (r << 24) | (g << 16) | (b << 8) | 0xFF
								videoChip.HandleWrite(addr, color)
							}
						}
					}
				}
			}
		}

		xOffset = (xOffset + 2) % (textLen * charWidth)
		time.Sleep(time.Millisecond * 16)
	}
}

func TestTunnelEffect(t *testing.T) {
	mode := VideoModes[MODE_640x480]

	// Precalculate distance and angle tables
	distance := make([][]float64, mode.height)
	angle := make([][]float64, mode.height)

	centerX := float64(mode.width) / 2
	centerY := float64(mode.height) / 2

	for y := 0; y < mode.height; y++ {
		distance[y] = make([]float64, mode.width)
		angle[y] = make([]float64, mode.width)

		for x := 0; x < mode.width; x++ {
			dx := float64(x) - centerX
			dy := float64(y) - centerY

			distance[y][x] = math.Sqrt(dx*dx + dy*dy)
			angle[y][x] = math.Atan2(dy, dx)
		}
	}

	startTime := time.Now()
	for time.Since(startTime) < 5*time.Second {
		timeVal := float64(time.Since(startTime).Milliseconds()) / 1000.0

		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				// Create tunnel effect by distorting distance and angle
				dist := distance[y][x]
				ang := angle[y][x]

				// Manipulate UV coordinates with time
				u := 128.0 * ang / math.Pi
				v := 256.0 - 32.0*math.Log2(1.0+dist/32.0)

				// Add movement
				u += timeVal * 30.0
				v += timeVal * 20.0

				// Create color pattern
				pattern := math.Sin(u/8.0)*math.Cos(v/8.0) +
					math.Sin(u/16.0)*math.Cos(v/16.0) +
					math.Sin(dist/32.0-timeVal*4.0)

				// Convert to RGB
				intensity := (pattern + 3.0) / 6.0 // Normalize to 0.0-1.0
				r := uint32(intensity * 255)
				g := uint32(intensity * 128)
				b := uint32(intensity * 255)

				color := (r << 24) | (g << 16) | (b << 8) | 0xFF
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, color)
			}
		}

		time.Sleep(time.Millisecond * 16)
	}
}

func TestRotozoomer(t *testing.T) {
	mode := VideoModes[MODE_640x480]

	// Create checkerboard texture
	texSize := 256
	texture := make([]uint32, texSize*texSize)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if ((x ^ y) & 16) == 0 {
				texture[y*texSize+x] = 0xFFFFFFFF
			} else {
				texture[y*texSize+x] = 0x000000FF
			}
		}
	}

	startTime := time.Now()
	for time.Since(startTime) < 5*time.Second {
		timeVal := float64(time.Since(startTime).Milliseconds()) / 1000.0

		// Calculate transformation parameters
		scale := 1.0 + math.Sin(timeVal*0.5)*0.5
		angle := timeVal * 0.5

		sinA := math.Sin(angle)
		cosA := math.Cos(angle)

		// Center coordinates
		centerX := float64(mode.width) / 2
		centerY := float64(mode.height) / 2

		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				// Translate and rotate coordinates
				dx := (float64(x) - centerX) / (128.0 * scale)
				dy := (float64(y) - centerY) / (128.0 * scale)

				// Rotate
				texX := int((dx*cosA-dy*sinA)*float64(texSize)) & (texSize - 1)
				texY := int((dx*sinA+dy*cosA)*float64(texSize)) & (texSize - 1)

				// Sample texture
				color := texture[texY*texSize+texX]
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, color)
			}
		}
		time.Sleep(time.Millisecond * 16)
	}
}

func TestStarfield(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	numStars := 1000

	type Star struct {
		x, y, z float64
	}

	// Initialize stars with random positions
	stars := make([]Star, numStars)
	for i := range stars {
		stars[i] = Star{
			x: rand.Float64()*2.0 - 1.0, // -1 to 1
			y: rand.Float64()*2.0 - 1.0, // -1 to 1
			z: rand.Float64(),           // 0 to 1
		}
	}

	startTime := time.Now()
	for time.Since(startTime) < 5*time.Second {
		// Clear screen
		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, 0x000000FF)
			}
		}

		centerX := float64(mode.width) / 2
		centerY := float64(mode.height) / 2

		// Update and draw stars
		for i := range stars {
			// Move star forward
			stars[i].z -= 0.01

			// Reset star if it goes too far
			if stars[i].z <= 0 {
				stars[i].x = rand.Float64()*2.0 - 1.0
				stars[i].y = rand.Float64()*2.0 - 1.0
				stars[i].z = 1.0
			}

			// Project star to screen space
			scale := 256.0 / stars[i].z
			sx := int(stars[i].x*scale + centerX)
			sy := int(stars[i].y*scale + centerY)

			// Draw star if in bounds
			if sx >= 0 && sx < mode.width && sy >= 0 && sy < mode.height {
				intensity := uint32((1.0 - stars[i].z) * 255.0)
				color := (intensity << 24) | (intensity << 16) | (intensity << 8) | 0xFF
				addr := VRAM_START + uint32((sy*mode.width+sx)*4)
				videoChip.HandleWrite(addr, color)
			}
		}
		time.Sleep(time.Millisecond * 16)
	}
}

func TestPlasmaWaves(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	startTime := time.Now()

	for time.Since(startTime) < 5*time.Second {
		timeVal := float64(time.Since(startTime).Milliseconds()) / 1000.0

		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				// Create multiple plasma waves
				fx := float64(x) / 30.0
				fy := float64(y) / 30.0

				val := math.Sin(fx+timeVal*2.0) +
					math.Sin((fy+timeVal)*2.0) +
					math.Sin((fx+fy+timeVal*3.0)/2.0) +
					math.Sin(math.Sqrt(fx*fx+fy*fy)/4.0)

				val = (val + 4.0) / 8.0 // Normalize to 0-1

				// Create color using HSV to RGB conversion
				h := val * 6.0
				sector := int(h)
				frac := h - float64(sector)

				var r, g, b float64
				switch sector % 6 {
				case 0:
					r, g, b = 1, frac, 0
				case 1:
					r, g, b = 1-frac, 1, 0
				case 2:
					r, g, b = 0, 1, frac
				case 3:
					r, g, b = 0, 1-frac, 1
				case 4:
					r, g, b = frac, 0, 1
				case 5:
					r, g, b = 1, 0, 1-frac
				}

				color := (uint32(r*255) << 24) | (uint32(g*255) << 16) | (uint32(b*255) << 8) | 0xFF
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, color)
			}
		}
		time.Sleep(time.Millisecond * 16)
	}
}

func TestMetaballs(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	numBalls := 5

	type Ball struct {
		x, y   float64
		dx, dy float64
		radius float64
	}

	// Initialize metaballs
	balls := make([]Ball, numBalls)
	for i := range balls {
		balls[i] = Ball{
			x:      rand.Float64() * float64(mode.width),
			y:      rand.Float64() * float64(mode.height),
			dx:     (rand.Float64()*2 - 1) * 3,
			dy:     (rand.Float64()*2 - 1) * 3,
			radius: 30 + rand.Float64()*20,
		}
	}

	startTime := time.Now()
	for time.Since(startTime) < 5*time.Second {
		// Update ball positions
		for i := range balls {
			balls[i].x += balls[i].dx
			balls[i].y += balls[i].dy

			// Bounce off edges
			if balls[i].x < 0 || balls[i].x >= float64(mode.width) {
				balls[i].dx = -balls[i].dx
			}
			if balls[i].y < 0 || balls[i].y >= float64(mode.height) {
				balls[i].dy = -balls[i].dy
			}
		}

		// Render metaballs
		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				// Calculate metaball field value
				sum := 0.0
				for _, ball := range balls {
					dx := float64(x) - ball.x
					dy := float64(y) - ball.y
					dist := math.Sqrt(dx*dx + dy*dy)
					sum += ball.radius / dist
				}

				// Create color based on field value
				var r, g, b uint32
				if sum > 1.0 {
					v := math.Min(sum-1.0, 1.0)
					r = uint32(v * 255)
					g = uint32(v * 128)
					b = uint32(v * 255)
				}

				color := (r << 24) | (g << 16) | (b << 8) | 0xFF
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, color)
			}
		}
		time.Sleep(time.Millisecond * 16)
	}
}

func TestParticles(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	numParticles := 500

	type Particle struct {
		x, y, z    float64
		vx, vy, vz float64
		life       float64
	}

	// Initialize particles
	particles := make([]Particle, numParticles)
	for i := range particles {
		particles[i] = Particle{
			life: rand.Float64(),
		}
	}

	resetParticle := func(p *Particle) {
		angle := rand.Float64() * math.Pi * 2
		elevation := rand.Float64() * math.Pi
		speed := 2.0 + rand.Float64()*2.0

		p.x = 0
		p.y = 0
		p.z = 0
		p.vx = math.Cos(angle) * math.Cos(elevation) * speed
		p.vy = math.Sin(elevation) * speed
		p.vz = math.Sin(angle) * math.Cos(elevation) * speed
		p.life = 1.0
	}

	startTime := time.Now()
	for time.Since(startTime) < 5*time.Second {
		// Clear screen
		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				addr := VRAM_START + uint32((y*mode.width+x)*4)
				videoChip.HandleWrite(addr, 0x000000FF)
			}
		}

		centerX := float64(mode.width) / 2
		centerY := float64(mode.height) / 2

		// Update and draw particles
		for i := range particles {
			// Apply gravity
			particles[i].vy -= 0.1

			// Update position
			particles[i].x += particles[i].vx
			particles[i].y += particles[i].vy
			particles[i].z += particles[i].vz

			// Update life
			particles[i].life -= 0.02
			if particles[i].life <= 0 {
				resetParticle(&particles[i])
			}

			// Project to screen space
			scale := 256.0 / (particles[i].z + 10)
			sx := int(particles[i].x*scale + centerX)
			sy := int(particles[i].y*scale + centerY)

			// Draw particle if in bounds
			if sx >= 0 && sx < mode.width && sy >= 0 && sy < mode.height {
				intensity := uint32(particles[i].life * 255.0)
				halfIntensity := uint32(particles[i].life * 127.5) // half brightness for green
				color := (intensity << 24) | (halfIntensity << 16) | 0x0000FF
				addr := VRAM_START + uint32((sy*mode.width+sx)*4)
				videoChip.HandleWrite(addr, color)
			}
		}
		time.Sleep(time.Millisecond * 16)
	}
}

func TestMandelbrot(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	startTime := time.Now()

	maxIter := 100
	centerX := -0.5
	centerY := 0.0

	for time.Since(startTime) < 5*time.Second {
		timeVal := float64(time.Since(startTime).Milliseconds()) / 1000.0
		zoom := math.Pow(2, timeVal*0.5)

		scale := 1.5 / zoom
		offsetX := centerX - (float64(mode.width)/2.0)*scale/float64(mode.width)
		offsetY := centerY - (float64(mode.height)/2.0)*scale/float64(mode.height)

		for y := 0; y < mode.height; y++ {
			for x := 0; x < mode.width; x++ {
				// Map pixel to complex plane
				cr := float64(x)*scale/float64(mode.width) + offsetX
				ci := float64(y)*scale/float64(mode.height) + offsetY

				// Iterate
				zr, zi := 0.0, 0.0
				iter := 0

				for ; iter < maxIter; iter++ {
					temp := zr*zr - zi*zi + cr
					zi = 2*zr*zi + ci
					zr = temp

					if zr*zr+zi*zi > 4 {
						break
					}
				}

				// Color based on iteration count
				if iter == maxIter {
					// Point is in set
					addr := VRAM_START + uint32((y*mode.width+x)*4)
					videoChip.HandleWrite(addr, 0x000000FF)
				} else {
					// Smooth coloring
					v := float64(iter) + 1 - math.Log(math.Log(math.Sqrt(zr*zr+zi*zi)))/math.Log(2)
					v = v / float64(maxIter)

					// Convert to color using smooth HSV to RGB
					h := v * 6.0
					sector := int(h)
					frac := h - float64(sector)

					var r, g, b float64
					switch sector % 6 {
					case 0:
						r, g, b = 1, frac, 0
					case 1:
						r, g, b = 1-frac, 1, 0
					case 2:
						r, g, b = 0, 1, frac
					case 3:
						r, g, b = 0, 1-frac, 1
					case 4:
						r, g, b = frac, 0, 1
					case 5:
						r, g, b = 1, 0, 1-frac
					}

					color := (uint32(r*255) << 24) | (uint32(g*255) << 16) | (uint32(b*255) << 8) | 0xFF
					addr := VRAM_START + uint32((y*mode.width+x)*4)
					videoChip.HandleWrite(addr, color)
				}
			}
		}
		time.Sleep(time.Millisecond * 16)
	}
}

// Helper functions
func rotate3D(x, y, z, angleX, angleY, angleZ float64) (float64, float64, float64) {
	// Rotate around X axis
	y2 := y*math.Cos(angleX) - z*math.Sin(angleX)
	z2 := y*math.Sin(angleX) + z*math.Cos(angleX)

	// Rotate around Y axis
	x3 := x*math.Cos(angleY) + z2*math.Sin(angleY)
	z3 := -x*math.Sin(angleY) + z2*math.Cos(angleY)

	// Rotate around Z axis
	x4 := x3*math.Cos(angleZ) - y2*math.Sin(angleZ)
	y4 := x3*math.Sin(angleZ) + y2*math.Cos(angleZ)

	return x4, y4, z3
}
func clearScreen(vc *VideoChip, mode VideoMode) {
	for y := 0; y < mode.height; y++ {
		for x := 0; x < mode.width; x++ {
			addr := VRAM_START + uint32((y*mode.width+x)*4)
			vc.HandleWrite(addr, 0x000000FF)
		}
	}
}
func drawLine(vc *VideoChip, mode VideoMode, x1, y1, x2, y2 int, color uint32) {
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	steep := dy > dx

	if steep {
		x1, y1 = y1, x1
		x2, y2 = y2, x2
	}

	if x1 > x2 {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}

	dx = x2 - x1
	dy = abs(y2 - y1)
	error := dx / 2
	ystep := 1
	if y1 >= y2 {
		ystep = -1
	}

	y := y1
	for x := x1; x <= x2; x++ {
		var px, py int
		if steep {
			px, py = y, x
		} else {
			px, py = x, y
		}

		if px >= 0 && px < mode.width && py >= 0 && py < mode.height {
			addr := VRAM_START + uint32((py*mode.width+px)*4)
			vc.HandleWrite(addr, color)
		}

		error -= dy
		if error < 0 {
			y += ystep
			error += dx
		}
	}
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func absUint32(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}
