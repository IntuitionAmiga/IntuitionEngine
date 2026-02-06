// video_engine_bench_test.go - Benchmarks for video engine performance
//
// Run with: go test -bench="Benchmark.*(VGA|ULA|TED|ANTIC)" -benchmem -run="^$" ./...

package main

import "testing"

// =============================================================================
// VGA Benchmarks
// =============================================================================

func BenchmarkVGA_RenderMode13h(b *testing.B) {
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	vga.setMode(VGA_MODE_13H)
	vga.control = VGA_CTRL_ENABLE

	// Fill VRAM with pattern
	for i := 0; i < 64000; i++ {
		offset := uint32(i)
		plane := offset & 3
		vramOffset := offset >> 2
		vga.vram[plane][vramOffset] = uint8(i & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vga.RenderFrame()
	}
}

func BenchmarkVGA_RenderMode12h(b *testing.B) {
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	vga.setMode(VGA_MODE_12H)
	vga.control = VGA_CTRL_ENABLE

	// Fill VRAM with pattern
	for plane := 0; plane < 4; plane++ {
		for i := 0; i < 38400; i++ { // 640*480/8 = 38400 bytes per plane
			vga.vram[plane][i] = uint8((i + plane) & 0xFF)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vga.RenderFrame()
	}
}

func BenchmarkVGA_RenderModeX(b *testing.B) {
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	vga.setMode(VGA_MODE_X)
	vga.control = VGA_CTRL_ENABLE

	// Fill VRAM with pattern (320x240 / 4 planes = 19200 bytes per plane)
	for plane := 0; plane < 4; plane++ {
		for i := 0; i < 19200; i++ {
			vga.vram[plane][i] = uint8((i + plane) & 0xFF)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vga.RenderFrame()
	}
}

func BenchmarkVGA_RenderTextMode(b *testing.B) {
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	vga.setMode(VGA_MODE_TEXT)
	vga.control = VGA_CTRL_ENABLE

	// Fill text buffer with pattern (80*25*2 = 4000 bytes)
	for i := 0; i < 2000; i++ {
		vga.textBuffer[i*2] = uint8('A' + (i % 26))   // Character
		vga.textBuffer[i*2+1] = uint8(0x07 + (i % 8)) // Attribute
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vga.RenderFrame()
	}
}

func BenchmarkVGA_GetPaletteRGBA(b *testing.B) {
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = vga.getPaletteRGBAInternal(uint8(i & 0xFF))
	}
}

// =============================================================================
// ULA Benchmarks
// =============================================================================

func BenchmarkULA_RenderFrame(b *testing.B) {
	bus := NewSystemBus()
	ula := NewULAEngine(bus)
	ula.control = ULA_CTRL_ENABLE

	// Fill VRAM with pattern (6144 bitmap + 768 attributes)
	for i := 0; i < 6912; i++ {
		ula.vram[i] = uint8(i & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ula.RenderFrame()
	}
}

func BenchmarkULA_GetBitmapAddress(b *testing.B) {
	bus := NewSystemBus()
	ula := NewULAEngine(bus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ula.GetBitmapAddress(i%192, i%256)
	}
}

func BenchmarkULA_GetColor(b *testing.B) {
	bus := NewSystemBus()
	ula := NewULAEngine(bus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ula.GetColor(uint8(i&0x07), i%2 == 0)
	}
}

// =============================================================================
// TED Benchmarks
// =============================================================================

func BenchmarkTED_RenderFrame(b *testing.B) {
	bus := NewSystemBus()
	ted := NewTEDVideoEngine(bus)
	ted.enabled.Store(true)

	// Fill video matrix and color RAM with pattern
	for y := 0; y < TED_V_CELLS_Y; y++ {
		for x := 0; x < TED_V_CELLS_X; x++ {
			matrixOffset := y*TED_V_CELLS_X + x
			colorOffset := TED_V_MATRIX_SIZE + y*TED_V_CELLS_X + x
			ted.vram[matrixOffset] = uint8((x + y) % 256)
			ted.vram[colorOffset] = uint8((x*2 + y) & 0x7F)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ted.RenderFrame()
	}
}

func BenchmarkTED_GetTEDColor(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = GetTEDColor(uint8(i & 0x7F))
	}
}

func BenchmarkTED_RenderCharacter(b *testing.B) {
	bus := NewSystemBus()
	ted := NewTEDVideoEngine(bus)
	ted.enabled.Store(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ted.renderCharacter(i%TED_V_CELLS_X, i%TED_V_CELLS_Y, 0, 0, 0)
	}
}

// =============================================================================
// ANTIC Benchmarks
// =============================================================================

func BenchmarkANTIC_RenderFrame(b *testing.B) {
	bus := NewSystemBus()
	antic := NewANTICEngine(bus)
	antic.enabled.Store(true)

	// Fill scanline colors with pattern
	for y := 0; y < ANTIC_DISPLAY_HEIGHT; y++ {
		antic.scanlineColors[0][y] = uint8(y & 0xFF)
		antic.scanlineColors[1][y] = uint8(y & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = antic.RenderFrame()
	}
}

func BenchmarkANTIC_GetANTICColor(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = GetANTICColor(uint8(i & 0xFF))
	}
}

func BenchmarkANTIC_RenderFrameWithPlayers(b *testing.B) {
	bus := NewSystemBus()
	antic := NewANTICEngine(bus)
	antic.enabled.Store(true)
	antic.gractl = GTIA_GRACTL_PLAYER // Enable players

	// Set up players with graphics
	for p := 0; p < 4; p++ {
		antic.grafp[p] = 0xFF // All pixels set
		antic.hposp[p] = uint8(80 + p*40)
		antic.colpm[p] = uint8(0x40 + p*0x20)
		antic.sizep[p] = 1 // Double width

		// Fill per-scanline data
		for y := 0; y < ANTIC_DISPLAY_HEIGHT; y++ {
			antic.playerGfx[0][p][y] = 0xFF
			antic.playerGfx[1][p][y] = 0xFF
			antic.playerPos[0][p][y] = uint8(80 + p*40)
			antic.playerPos[1][p][y] = uint8(80 + p*40)
		}
	}

	// Fill scanline colors
	for y := 0; y < ANTIC_DISPLAY_HEIGHT; y++ {
		antic.scanlineColors[0][y] = uint8(y & 0xFF)
		antic.scanlineColors[1][y] = uint8(y & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = antic.RenderFrame()
	}
}

// =============================================================================
// Cross-Engine Comparative Benchmarks
// =============================================================================

// BenchmarkAllEngines_RenderFrame compares all engine render times
func BenchmarkAllEngines_VGA13h(b *testing.B) {
	bus := NewSystemBus()
	vga := NewVGAEngine(bus)
	vga.setMode(VGA_MODE_13H)
	vga.control = VGA_CTRL_ENABLE

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vga.RenderFrame()
	}
}

func BenchmarkAllEngines_ULA(b *testing.B) {
	bus := NewSystemBus()
	ula := NewULAEngine(bus)
	ula.control = ULA_CTRL_ENABLE

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ula.RenderFrame()
	}
}

func BenchmarkAllEngines_TED(b *testing.B) {
	bus := NewSystemBus()
	ted := NewTEDVideoEngine(bus)
	ted.enabled.Store(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ted.RenderFrame()
	}
}

func BenchmarkAllEngines_ANTIC(b *testing.B) {
	bus := NewSystemBus()
	antic := NewANTICEngine(bus)
	antic.enabled.Store(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = antic.RenderFrame()
	}
}
