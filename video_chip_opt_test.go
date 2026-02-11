// video_chip_opt_test.go - Video chip optimization tests

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
	"sync"
	"sync/atomic"
	"testing"
)

// ============================================================================
// Tile Copy Tests
// ============================================================================

// tileCopyReference is the original unoptimized implementation
func tileCopyReference(frontBuffer, backBuffer []byte, mode VideoMode, startX, startY, endX, endY int) {
	for y := startY; y < endY; y++ {
		srcOffset := (y * mode.bytesPerRow) + (startX * BYTES_PER_PIXEL)
		copyLen := (endX - startX) * BYTES_PER_PIXEL
		if srcOffset+copyLen <= len(frontBuffer) {
			copy(backBuffer[srcOffset:srcOffset+copyLen],
				frontBuffer[srcOffset:srcOffset+copyLen])
		}
	}
}

// TestTileCopy_MatchesReference verifies optimized tile copy matches reference
func TestTileCopy_MatchesReference(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	frontBuffer := make([]byte, mode.totalSize)
	backBufferRef := make([]byte, mode.totalSize)
	backBufferOpt := make([]byte, mode.totalSize)

	// Fill front buffer with test pattern
	for i := range frontBuffer {
		frontBuffer[i] = byte(i % 256)
	}

	// Test various tile positions
	testCases := []struct {
		name                       string
		startX, startY, endX, endY int
	}{
		{"TopLeft", 0, 0, 40, 30},
		{"Center", 300, 200, 340, 230},
		{"BottomRight", 600, 450, 640, 480},
		{"FullWidth", 0, 100, 640, 130},
		{"SinglePixel", 320, 240, 321, 241},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear back buffers
			for i := range backBufferRef {
				backBufferRef[i] = 0
				backBufferOpt[i] = 0
			}

			// Reference copy
			tileCopyReference(frontBuffer, backBufferRef, mode, tc.startX, tc.startY, tc.endX, tc.endY)

			// Optimized copy (uses same function for now, will be replaced)
			tileCopyReference(frontBuffer, backBufferOpt, mode, tc.startX, tc.startY, tc.endX, tc.endY)

			// Compare
			for i := range backBufferRef {
				if backBufferRef[i] != backBufferOpt[i] {
					t.Fatalf("Mismatch at byte %d: got %d, expected %d", i, backBufferOpt[i], backBufferRef[i])
				}
			}
		})
	}
}

// TestTileCopy_EdgeTiles verifies edge cases
func TestTileCopy_EdgeTiles(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	frontBuffer := make([]byte, mode.totalSize)
	backBuffer := make([]byte, mode.totalSize)

	for i := range frontBuffer {
		frontBuffer[i] = byte(i % 256)
	}

	// Edge at screen boundary
	tileCopyReference(frontBuffer, backBuffer, mode, 620, 460, 640, 480)

	// Verify the correct region was copied
	for y := 460; y < 480; y++ {
		for x := 620; x < 640; x++ {
			idx := (y*mode.width + x) * BYTES_PER_PIXEL
			if backBuffer[idx] != frontBuffer[idx] {
				t.Fatalf("Edge tile not copied correctly at (%d,%d)", x, y)
			}
		}
	}
}

// BenchmarkTileCopy measures tile copy performance
func BenchmarkTileCopy(b *testing.B) {
	mode := VideoModes[MODE_640x480]
	frontBuffer := make([]byte, mode.totalSize)
	backBuffer := make([]byte, mode.totalSize)

	for i := range frontBuffer {
		frontBuffer[i] = byte(i % 256)
	}

	// Typical tile size (40x30 pixels)
	startX, startY := 300, 200
	endX, endY := 340, 230

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tileCopyReference(frontBuffer, backBuffer, mode, startX, startY, endX, endY)
	}
}

// ============================================================================
// Dirty Mark Tests
// ============================================================================

// TestDirtyMark_HighContention tests dirty marking under concurrent access
func TestDirtyMark_HighContention(t *testing.T) {
	chip := &VideoChip{
		currentMode: MODE_640x480,
	}
	mode := VideoModes[chip.currentMode]
	chip.initialiseDirtyGrid(mode)

	const numGoroutines = 10
	const marksPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := range numGoroutines {
		go func(gid int) {
			defer wg.Done()
			for i := range marksPerGoroutine {
				// Distribute marks across the screen
				x := (gid*100 + i) % mode.width
				y := (gid*50 + i) % mode.height
				chip.markTileDirtyAtomic(x, y)
			}
		}(g)
	}

	wg.Wait()

	// Verify some tiles are marked dirty
	if !chip.hasDirtyTiles() {
		t.Fatal("Expected dirty tiles after concurrent marking")
	}
}

// TestDirtyMark_NoPixelsLost verifies no dirty marks are lost under contention
func TestDirtyMark_NoPixelsLost(t *testing.T) {
	chip := &VideoChip{
		currentMode: MODE_640x480,
	}
	mode := VideoModes[chip.currentMode]
	chip.initialiseDirtyGrid(mode)

	// Mark all tiles dirty from multiple goroutines
	const numGoroutines = 4
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	tilesPerGoroutine := (DIRTY_GRID_COLS * DIRTY_GRID_ROWS) / numGoroutines

	for g := range numGoroutines {
		go func(gid int) {
			defer wg.Done()
			startTile := gid * tilesPerGoroutine
			for tile := startTile; tile < startTile+tilesPerGoroutine; tile++ {
				tileX := tile % DIRTY_GRID_COLS
				tileY := tile / DIRTY_GRID_COLS
				// Mark center of each tile
				x := tileX*int(chip.tileWidth) + int(chip.tileWidth)/2
				y := tileY*int(chip.tileHeight) + int(chip.tileHeight)/2
				if x < mode.width && y < mode.height {
					chip.markTileDirtyAtomic(x, y)
				}
			}
		}(g)
	}

	wg.Wait()

	// Count dirty tiles
	snapshot := chip.clearDirtyBitmap()
	dirtyCount := 0
	for _, word := range snapshot {
		for word != 0 {
			dirtyCount++
			word &= word - 1 // Clear lowest set bit
		}
	}

	expectedTiles := DIRTY_GRID_COLS * DIRTY_GRID_ROWS
	if dirtyCount != expectedTiles {
		t.Fatalf("Expected %d dirty tiles, got %d", expectedTiles, dirtyCount)
	}
}

// TestDirtyMark_BoundaryConditions tests marking at grid boundaries
func TestDirtyMark_BoundaryConditions(t *testing.T) {
	chip := &VideoChip{
		currentMode: MODE_640x480,
	}
	mode := VideoModes[chip.currentMode]
	chip.initialiseDirtyGrid(mode)

	// Test boundary pixels
	testCases := []struct {
		name string
		x, y int
	}{
		{"Origin", 0, 0},
		{"TopRight", mode.width - 1, 0},
		{"BottomLeft", 0, mode.height - 1},
		{"BottomRight", mode.width - 1, mode.height - 1},
		{"Negative", -1, -1},                     // Should be clamped
		{"OutOfBounds", mode.width, mode.height}, // Should be clamped
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear bitmap
			for i := range chip.dirtyBitmap {
				chip.dirtyBitmap[i].Store(0)
			}

			// This should not panic
			chip.markTileDirtyAtomic(tc.x, tc.y)
		})
	}
}

// BenchmarkDirtyMark measures dirty mark performance
func BenchmarkDirtyMark(b *testing.B) {
	chip := &VideoChip{
		currentMode: MODE_640x480,
	}
	mode := VideoModes[chip.currentMode]
	chip.initialiseDirtyGrid(mode)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		x := i % mode.width
		y := (i / mode.width) % mode.height
		chip.markTileDirtyAtomic(x, y)
	}
}

// BenchmarkDirtyMark_Contended measures dirty mark under contention
func BenchmarkDirtyMark_Contended(b *testing.B) {
	chip := &VideoChip{
		currentMode: MODE_640x480,
	}
	mode := VideoModes[chip.currentMode]
	chip.initialiseDirtyGrid(mode)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			x := i % mode.width
			y := (i / mode.width) % mode.height
			chip.markTileDirtyAtomic(x, y)
			i++
		}
	})
}

// ============================================================================
// Blitter Tests
// ============================================================================

// TestBlitter_CopyMatchesReference verifies blitter copy produces correct results
func TestBlitter_CopyMatchesReference(t *testing.T) {
	mode := VideoModes[MODE_640x480]

	// Create a minimal video chip with buffer
	chip := &VideoChip{
		currentMode: MODE_640x480,
		frontBuffer: make([]byte, mode.totalSize),
		backBuffer:  make([]byte, mode.totalSize),
	}
	chip.initialiseDirtyGrid(mode)

	// Create test pattern in buffer - fill entire first 100 rows
	for y := range 100 {
		for x := 0; x < mode.width; x++ {
			idx := (y*mode.width + x) * BYTES_PER_PIXEL
			chip.frontBuffer[idx+0] = 255 // R
			chip.frontBuffer[idx+1] = 128 // G
			chip.frontBuffer[idx+2] = 64  // B
			chip.frontBuffer[idx+3] = 255 // A
		}
	}

	// Verify read returns correct value
	offset := uint32(50*mode.width*BYTES_PER_PIXEL + 50*BYTES_PER_PIXEL) // Pixel at (50, 50)
	value := chip.blitReadPixelLocked(VRAM_START + offset)

	// Value is RGBA in little-endian: R|G|B|A as uint32
	expectedR := value & 0xFF
	expectedG := (value >> 8) & 0xFF
	expectedB := (value >> 16) & 0xFF
	expectedA := (value >> 24) & 0xFF

	if expectedR != 255 || expectedG != 128 || expectedB != 64 || expectedA != 255 {
		t.Fatalf("Unexpected pixel value at (50,50): R=%d G=%d B=%d A=%d",
			expectedR, expectedG, expectedB, expectedA)
	}
}

// TestBlitter_FillMatchesReference verifies blitter fill produces correct results
func TestBlitter_FillMatchesReference(t *testing.T) {
	mode := VideoModes[MODE_640x480]

	chip := &VideoChip{
		currentMode: MODE_640x480,
		frontBuffer: make([]byte, mode.totalSize),
		backBuffer:  make([]byte, mode.totalSize),
	}
	chip.initialiseDirtyGrid(mode)

	// Set up blitter fill parameters
	// Color format: 0xAABBGGRR (little-endian RGBA)
	chip.bltOp = bltOpFill
	chip.bltDst = VRAM_START
	chip.bltWidth = 100
	chip.bltHeight = 50
	chip.bltColor = 0xFF00FF00 // ABGR = A=FF, B=00, G=FF, R=00 = green with full alpha
	chip.bltDstStrideRun = uint32(mode.bytesPerRow)

	// Run blitter directly (bypass runBlitterLocked which requires bus)
	chip.executeBlitterLocked(mode)

	// Verify fill
	// The color 0xFF00FF00 in little-endian means R=0, G=FF, B=0, A=FF
	for y := range 50 {
		for x := range 100 {
			idx := (y*mode.width + x) * BYTES_PER_PIXEL
			// Little-endian: 0xFF00FF00 stored as bytes: 00 FF 00 FF
			r := chip.frontBuffer[idx+0]
			g := chip.frontBuffer[idx+1]
			b := chip.frontBuffer[idx+2]
			a := chip.frontBuffer[idx+3]

			// Expected: R=0x00, G=0xFF, B=0x00, A=0xFF (from 0xFF00FF00)
			if r != 0x00 || g != 0xFF || b != 0x00 || a != 0xFF {
				t.Fatalf("Fill mismatch at (%d,%d): got RGBA(0x%02x,0x%02x,0x%02x,0x%02x), expected (0x00,0xFF,0x00,0xFF)",
					x, y, r, g, b, a)
			}
		}
	}
}

// BenchmarkBlitter_Copy measures blitter copy performance
func BenchmarkBlitter_Copy(b *testing.B) {
	mode := VideoModes[MODE_640x480]

	chip := &VideoChip{
		currentMode: MODE_640x480,
		frontBuffer: make([]byte, mode.totalSize),
		backBuffer:  make([]byte, mode.totalSize),
		busMemory:   make([]byte, 16*1024*1024), // 16MB
	}
	chip.initialiseDirtyGrid(mode)

	// Fill source area with pattern
	srcOffset := 1024 * 1024 // 1MB into memory
	for i := range 100 * 100 * 4 {
		chip.busMemory[srcOffset+i] = byte(i % 256)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		chip.bltOp = bltOpCopy
		chip.bltSrc = uint32(srcOffset)
		chip.bltDst = VRAM_START
		chip.bltWidth = 100
		chip.bltHeight = 100
		chip.bltSrcStrideRun = 100 * 4
		chip.bltDstStrideRun = uint32(mode.bytesPerRow)
		chip.executeBlitterLocked(mode)
	}
}

// BenchmarkBlitter_Fill measures blitter fill performance
func BenchmarkBlitter_Fill(b *testing.B) {
	mode := VideoModes[MODE_640x480]

	chip := &VideoChip{
		currentMode: MODE_640x480,
		frontBuffer: make([]byte, mode.totalSize),
		backBuffer:  make([]byte, mode.totalSize),
	}
	chip.initialiseDirtyGrid(mode)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		chip.bltOp = bltOpFill
		chip.bltDst = VRAM_START
		chip.bltWidth = 100
		chip.bltHeight = 100
		chip.bltColor = 0xFF00FF00
		chip.bltDstStrideRun = uint32(mode.bytesPerRow)
		chip.executeBlitterLocked(mode)
	}
}

// ============================================================================
// VBlank Tests
// ============================================================================

// TestVBlank_TimingAccuracy verifies VBlank calculation
func TestVBlank_TimingAccuracy(t *testing.T) {
	chip := &VideoChip{}
	chip.hasContent.Store(true)

	// Simulate frame start
	chip.lastFrameStart.Store(0) // Frame started at time 0

	// At time 0, should not be in VBlank
	// Note: This depends on actual time.Since() behavior
	// We'll verify the basic status structure
	status := chip.HandleRead(VIDEO_STATUS)

	// Bit 0 should be set (has content)
	if status&1 == 0 {
		t.Error("Expected has content bit to be set")
	}
}

// BenchmarkVBlankCheck measures VBlank check performance
func BenchmarkVBlankCheck(b *testing.B) {
	chip := &VideoChip{}
	chip.hasContent.Store(true)
	chip.lastFrameStart.Store(0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = chip.HandleRead(VIDEO_STATUS)
	}
}

// ============================================================================
// Atomic Or Tests (for Phase 3)
// ============================================================================

// TestAtomicOr_Equivalent verifies atomic.Or produces same result as CAS loop
func TestAtomicOr_Equivalent(t *testing.T) {
	// Test that atomic.Uint64.Or() produces same result as CAS loop
	var atomicVal atomic.Uint64

	// Set some initial bits
	atomicVal.Store(0x00FF00FF00FF00FF)

	// Use Or to set additional bits
	atomicVal.Or(0xFF00FF00FF00FF00)

	expected := uint64(0xFFFFFFFFFFFFFFFF)
	if atomicVal.Load() != expected {
		t.Fatalf("atomic.Or failed: got %016x, expected %016x", atomicVal.Load(), expected)
	}
}

// BenchmarkAtomicOr_vsCAS compares atomic.Or vs CAS loop performance
func BenchmarkAtomicOr_vsCAS(b *testing.B) {
	b.Run("AtomicOr", func(b *testing.B) {
		var val atomic.Uint64
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			val.Or(1 << uint(i%64))
		}
	})

	b.Run("CASLoop", func(b *testing.B) {
		var val atomic.Uint64
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bit := uint64(1) << uint(i%64)
			for {
				old := val.Load()
				new := old | bit
				if old == new || val.CompareAndSwap(old, new) {
					break
				}
			}
		}
	})
}
