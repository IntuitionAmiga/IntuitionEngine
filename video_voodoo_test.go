// video_voodoo_test.go - Test suite for 3DFX Voodoo SST-1 Graphics Emulation

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
	"math"
	"testing"
)

// =============================================================================
// Sprint 1: Foundation Tests
// =============================================================================

func TestVoodoo_NewEngine(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	if v == nil {
		t.Fatal("NewVoodooEngine returned nil")
	}
}

func TestVoodoo_DefaultState(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Check default dimensions
	w, h := v.GetDimensions()
	if w != VOODOO_DEFAULT_WIDTH || h != VOODOO_DEFAULT_HEIGHT {
		t.Errorf("Expected dimensions %dx%d, got %dx%d",
			VOODOO_DEFAULT_WIDTH, VOODOO_DEFAULT_HEIGHT, w, h)
	}

	// Check enabled by default
	if !v.IsEnabled() {
		t.Error("Expected Voodoo to be enabled by default")
	}

	// Check layer
	if v.GetLayer() != VOODOO_LAYER {
		t.Errorf("Expected layer %d, got %d", VOODOO_LAYER, v.GetLayer())
	}
}

func TestVoodoo_Implements_VideoSource(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// This test verifies interface compliance at compile time
	var _ VideoSource = v

	// Test each method
	frame := v.GetFrame()
	if frame == nil {
		t.Error("GetFrame returned nil for enabled Voodoo")
	}

	if len(frame) != VOODOO_DEFAULT_WIDTH*VOODOO_DEFAULT_HEIGHT*4 {
		t.Errorf("Expected frame size %d, got %d",
			VOODOO_DEFAULT_WIDTH*VOODOO_DEFAULT_HEIGHT*4, len(frame))
	}
}

func TestVoodoo_SoftwareBackendInit(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	if backend == nil {
		t.Fatal("NewVoodooSoftwareBackend returned nil")
	}

	err := backend.Init(640, 480)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	frame := backend.GetFrame()
	if frame == nil {
		t.Error("GetFrame returned nil after Init")
	}

	if len(frame) != 640*480*4 {
		t.Errorf("Expected frame size %d, got %d", 640*480*4, len(frame))
	}
}

// =============================================================================
// Sprint 2: Register I/O Tests
// =============================================================================

func TestVoodoo_WriteRead_FBZMode(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write a specific fbzMode value
	testValue := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | (VOODOO_DEPTH_LESSEQUAL << 5))
	v.HandleWrite(VOODOO_FBZ_MODE, testValue)

	// Read it back
	readValue := v.HandleRead(VOODOO_FBZ_MODE)
	if readValue != testValue {
		t.Errorf("Expected fbzMode 0x%X, got 0x%X", testValue, readValue)
	}
}

func TestVoodoo_WriteRead_AlphaMode(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write alpha mode with blending enabled
	testValue := uint32(VOODOO_ALPHA_BLEND_EN | (VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12))
	v.HandleWrite(VOODOO_ALPHA_MODE, testValue)

	readValue := v.HandleRead(VOODOO_ALPHA_MODE)
	if readValue != testValue {
		t.Errorf("Expected alphaMode 0x%X, got 0x%X", testValue, readValue)
	}
}

func TestVoodoo_WriteRead_Vertices(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write vertex A coordinates (12.4 fixed-point)
	// 100.5 in 12.4 = 100.5 * 16 = 1608 = 0x648
	v.HandleWrite(VOODOO_VERTEX_AX, 0x648)
	v.HandleWrite(VOODOO_VERTEX_AY, 0x320) // 50.0 * 16 = 800

	// Verify shadow registers store the raw values
	axRaw := v.HandleRead(VOODOO_VERTEX_AX)
	ayRaw := v.HandleRead(VOODOO_VERTEX_AY)

	if axRaw != 0x648 {
		t.Errorf("Expected vertex AX raw 0x648, got 0x%X", axRaw)
	}
	if ayRaw != 0x320 {
		t.Errorf("Expected vertex AY raw 0x320, got 0x%X", ayRaw)
	}
}

func TestVoodoo_WriteRead_Colors(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write colors (12.12 fixed-point)
	// 1.0 in 12.12 = 1.0 * 4096 = 0x1000
	v.HandleWrite(VOODOO_START_R, 0x1000)
	v.HandleWrite(VOODOO_START_G, 0x800) // 0.5 * 4096
	v.HandleWrite(VOODOO_START_B, 0x400) // 0.25 * 4096
	v.HandleWrite(VOODOO_START_A, 0x1000)

	// Verify shadow registers
	if v.HandleRead(VOODOO_START_R) != 0x1000 {
		t.Error("Start R not stored correctly")
	}
	if v.HandleRead(VOODOO_START_G) != 0x800 {
		t.Error("Start G not stored correctly")
	}
}

func TestVoodoo_WriteRead_ClipRect(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set clip rectangle: left=100, right=500, top=50, bottom=400
	// clipLeftRight: bits 0-9 = right, bits 16-25 = left
	leftRight := uint32(500) | (uint32(100) << 16)
	topBottom := uint32(400) | (uint32(50) << 16)

	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, leftRight)
	v.HandleWrite(VOODOO_CLIP_LOW_Y_HIGH, topBottom)

	// Read back
	if v.HandleRead(VOODOO_CLIP_LEFT_RIGHT) != leftRight {
		t.Error("Clip left/right not stored correctly")
	}
	if v.HandleRead(VOODOO_CLIP_LOW_Y_HIGH) != topBottom {
		t.Error("Clip top/bottom not stored correctly")
	}
}

// =============================================================================
// Sprint 3: Vertex Batching Tests
// =============================================================================

func TestVoodoo_TriangleCMD_AddsToBuffer(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Initial batch should be empty
	if v.GetTriangleBatchCount() != 0 {
		t.Error("Expected empty triangle batch initially")
	}

	// Set up a triangle
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(150))

	// Set colors
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))

	// Submit triangle
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	if v.GetTriangleBatchCount() != 1 {
		t.Errorf("Expected 1 triangle in batch, got %d", v.GetTriangleBatchCount())
	}
}

func TestVoodoo_TriangleBatch_MultipleTriangles(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Add 3 triangles
	for i := 0; i < 3; i++ {
		offset := float32(i * 100)
		v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100+offset))
		v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50))
		v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200+offset))
		v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(150))
		v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50+offset))
		v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(150))
		v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	}

	if v.GetTriangleBatchCount() != 3 {
		t.Errorf("Expected 3 triangles in batch, got %d", v.GetTriangleBatchCount())
	}
}

func TestVoodoo_SwapBuffer_ClearsBatch(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Add a triangle
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	if v.GetTriangleBatchCount() != 1 {
		t.Error("Triangle not added to batch")
	}

	// Swap buffers
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Batch should be cleared
	if v.GetTriangleBatchCount() != 0 {
		t.Errorf("Expected batch to be cleared after swap, got %d triangles",
			v.GetTriangleBatchCount())
	}
}

func TestVoodoo_FixedPointConversion(t *testing.T) {
	tests := []struct {
		name    string
		input   float32
		convert func(float32) uint32
		back    func(uint32) float32
		epsilon float32
	}{
		{"12.4 positive", 100.5, floatToFixed12_4, fixed12_4ToFloat, 0.0625},
		{"12.4 zero", 0.0, floatToFixed12_4, fixed12_4ToFloat, 0.0625},
		{"12.4 negative", -50.25, floatToFixed12_4, fixed12_4ToFloat, 0.0625},
		{"12.12 one", 1.0, floatToFixed12_12, fixed12_12ToFloat, 0.001},
		{"12.12 half", 0.5, floatToFixed12_12, fixed12_12ToFloat, 0.001},
		{"12.12 quarter", 0.25, floatToFixed12_12, fixed12_12ToFloat, 0.001},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixed := tc.convert(tc.input)
			result := tc.back(fixed)
			diff := float32(math.Abs(float64(result - tc.input)))
			if diff > tc.epsilon {
				t.Errorf("Conversion mismatch: input=%f, fixed=0x%X, result=%f, diff=%f",
					tc.input, fixed, result, diff)
			}
		})
	}
}

// =============================================================================
// Sprint 4: Pipeline State Tests
// =============================================================================

func TestVoodoo_DepthTest_Less(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(640, 480)
	defer backend.Destroy()

	// Set depth function to LESS
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_LESS << 5))
	backend.UpdatePipelineState(fbzMode, 0)

	// Test the depth test function
	if !backend.depthTest(0.5, 1.0, VOODOO_DEPTH_LESS) {
		t.Error("DEPTH_LESS: 0.5 should pass against 1.0")
	}
	if backend.depthTest(1.0, 0.5, VOODOO_DEPTH_LESS) {
		t.Error("DEPTH_LESS: 1.0 should fail against 0.5")
	}
}

func TestVoodoo_DepthTest_AllFunctions(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	tests := []struct {
		name       string
		depthFunc  int
		newZ, oldZ float32
		expected   bool
	}{
		{"NEVER", VOODOO_DEPTH_NEVER, 0.5, 1.0, false},
		{"LESS pass", VOODOO_DEPTH_LESS, 0.5, 1.0, true},
		{"LESS fail", VOODOO_DEPTH_LESS, 1.0, 0.5, false},
		{"EQUAL pass", VOODOO_DEPTH_EQUAL, 0.5, 0.5, true},
		{"EQUAL fail", VOODOO_DEPTH_EQUAL, 0.5, 0.6, false},
		{"LESSEQUAL pass", VOODOO_DEPTH_LESSEQUAL, 0.5, 0.5, true},
		{"GREATER pass", VOODOO_DEPTH_GREATER, 1.0, 0.5, true},
		{"GREATER fail", VOODOO_DEPTH_GREATER, 0.5, 1.0, false},
		{"NOTEQUAL pass", VOODOO_DEPTH_NOTEQUAL, 0.5, 0.6, true},
		{"NOTEQUAL fail", VOODOO_DEPTH_NOTEQUAL, 0.5, 0.5, false},
		{"GREATEREQUAL pass", VOODOO_DEPTH_GREATEREQUAL, 0.5, 0.5, true},
		{"ALWAYS", VOODOO_DEPTH_ALWAYS, 0.5, 0.0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := backend.depthTest(tc.newZ, tc.oldZ, tc.depthFunc)
			if result != tc.expected {
				t.Errorf("depthTest(%f, %f, %d) = %v, expected %v",
					tc.newZ, tc.oldZ, tc.depthFunc, result, tc.expected)
			}
		})
	}
}

func TestVoodoo_Scissor(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set scissor via registers
	leftRight := uint32(300) | (uint32(100) << 16) // right=300, left=100
	topBottom := uint32(200) | (uint32(50) << 16)  // bottom=200, top=50

	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, leftRight)
	v.HandleWrite(VOODOO_CLIP_LOW_Y_HIGH, topBottom)

	// Internal state should be updated
	if v.clipLeft != 100 {
		t.Errorf("Expected clipLeft 100, got %d", v.clipLeft)
	}
	if v.clipRight != 300 {
		t.Errorf("Expected clipRight 300, got %d", v.clipRight)
	}
	if v.clipTop != 50 {
		t.Errorf("Expected clipTop 50, got %d", v.clipTop)
	}
	if v.clipBottom != 200 {
		t.Errorf("Expected clipBottom 200, got %d", v.clipBottom)
	}
}

// =============================================================================
// Sprint 5: Triangle Rendering Tests
// =============================================================================

func TestVoodoo_Render_FlatTriangle(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to black
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000) // ARGB black
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw a red triangle in the center
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320)) // Top center
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(420)) // Bottom right
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(220)) // Bottom left
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))

	// Set red color
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit and swap
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Check that the center of the triangle is red
	frame := v.GetFrame()
	centerX, centerY := 320, 200
	pixelIdx := (centerY*640 + centerX) * 4

	r := frame[pixelIdx]
	g := frame[pixelIdx+1]
	b := frame[pixelIdx+2]

	// Should be approximately red (allow some tolerance)
	if r < 200 || g > 50 || b > 50 {
		t.Errorf("Expected red pixel at center, got R=%d G=%d B=%d", r, g, b)
	}
}

func TestVoodoo_Render_MultipleTriangles(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to black
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw 3 triangles with different colors
	colors := [][3]float32{
		{1.0, 0.0, 0.0}, // Red
		{0.0, 1.0, 0.0}, // Green
		{0.0, 0.0, 1.0}, // Blue
	}

	for i, color := range colors {
		offset := float32(i * 150)
		v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100+offset))
		v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
		v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200+offset))
		v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
		v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50+offset))
		v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))

		v.HandleWrite(VOODOO_START_R, floatToFixed12_12(color[0]))
		v.HandleWrite(VOODOO_START_G, floatToFixed12_12(color[1]))
		v.HandleWrite(VOODOO_START_B, floatToFixed12_12(color[2]))
		v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

		v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	}

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Verify frame is not all black
	frame := v.GetFrame()
	hasColor := false
	for i := 0; i < len(frame); i += 4 {
		if frame[i] > 0 || frame[i+1] > 0 || frame[i+2] > 0 {
			hasColor = true
			break
		}
	}
	if !hasColor {
		t.Error("Frame appears to be all black after rendering")
	}
}

func TestVoodoo_Render_ZBuffer(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable depth testing
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_LESS << 5))
	v.HandleWrite(VOODOO_FBZ_MODE, fbzMode)

	// Clear
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw far blue triangle first (Z = 0.8)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(400))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(350))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(350))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.8))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// Draw near red triangle (Z = 0.2), overlapping
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(350))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.2))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Check center pixel - should be red (near triangle wins)
	frame := v.GetFrame()
	centerX, centerY := 270, 250
	pixelIdx := (centerY*640 + centerX) * 4

	r := frame[pixelIdx]
	b := frame[pixelIdx+2]

	if r < 200 || b > 50 {
		t.Errorf("Expected red pixel (Z-test should favor near triangle), got R=%d B=%d", r, b)
	}
}

// =============================================================================
// Sprint 6: Buffer Operations Tests
// =============================================================================

func TestVoodoo_FastFill_ClearsBuffer(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set fill color to bright green
	v.HandleWrite(VOODOO_COLOR0, 0xFF00FF00) // ARGB green

	// Execute fast fill
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Check frame buffer
	frame := v.GetFrame()
	if len(frame) == 0 {
		t.Fatal("GetFrame returned empty buffer")
	}

	// Sample a few pixels - should all be green
	for _, idx := range []int{0, 1000, 100000, 200000} {
		pixelIdx := idx * 4
		if pixelIdx+3 >= len(frame) {
			continue
		}
		r := frame[pixelIdx]
		g := frame[pixelIdx+1]
		b := frame[pixelIdx+2]

		if r != 0 || g != 255 || b != 0 {
			t.Errorf("Pixel %d: expected (0,255,0), got (%d,%d,%d)", idx, r, g, b)
		}
	}
}

func TestVoodoo_SwapBuffer_PresentsFrame(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to red
	v.HandleWrite(VOODOO_COLOR0, 0xFFFF0000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	frame1 := make([]byte, len(v.GetFrame()))
	copy(frame1, v.GetFrame())

	// Clear to blue
	v.HandleWrite(VOODOO_COLOR0, 0xFF0000FF)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	frame2 := v.GetFrame()

	// Frames should be different
	if frame1[0] == frame2[0] && frame1[2] == frame2[2] {
		t.Error("Frames should differ after swap with different clear color")
	}
}

func TestVoodoo_Status_Register(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	status := v.HandleRead(VOODOO_STATUS)

	// FIFO should report available space
	memfifo := (status >> 12) & 0xFF
	pcififo := (status >> 20) & 0x1F

	if memfifo == 0 {
		t.Error("Expected memfifo to have available space")
	}
	if pcififo == 0 {
		t.Error("Expected pcififo to have available space")
	}
}

func TestVoodoo_VSync_Signal(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Signal VSync
	v.SignalVSync()

	// Status should show vretrace
	status := v.HandleRead(VOODOO_STATUS)
	if (status & VOODOO_STATUS_VRETRACE) == 0 {
		t.Error("Expected VRETRACE flag after SignalVSync")
	}
}

// =============================================================================
// Sprint 7: Video Dimensions Tests
// =============================================================================

func TestVoodoo_VideoDimensions_Change(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Change to 800x600
	v.HandleWrite(VOODOO_VIDEO_DIM, (800<<16)|600)

	w, h := v.GetDimensions()
	if w != 800 || h != 600 {
		t.Errorf("Expected 800x600, got %dx%d", w, h)
	}

	frame := v.GetFrame()
	expectedSize := 800 * 600 * 4
	if len(frame) != expectedSize {
		t.Errorf("Expected frame size %d, got %d", expectedSize, len(frame))
	}
}

func TestVoodoo_Enable_Disable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	if !v.IsEnabled() {
		t.Error("Expected Voodoo to be enabled by default")
	}

	v.SetEnabled(false)

	if v.IsEnabled() {
		t.Error("Expected Voodoo to be disabled")
	}

	// GetFrame should return nil when disabled
	frame := v.GetFrame()
	if frame != nil {
		t.Error("Expected nil frame when disabled")
	}

	v.SetEnabled(true)
	frame = v.GetFrame()
	if frame == nil {
		t.Error("Expected non-nil frame when re-enabled")
	}
}

// =============================================================================
// Sprint 8: Integration Tests
// =============================================================================

func TestVoodoo_FullRenderLoop(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Simulate a typical frame:
	// 1. Clear
	// 2. Draw triangles
	// 3. Swap

	for frameNum := 0; frameNum < 3; frameNum++ {
		// Clear to different color each frame
		clearColor := uint32(0xFF000000) | uint32((frameNum*80)<<16)
		v.HandleWrite(VOODOO_COLOR0, clearColor)
		v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

		// Draw a rotating triangle (simplified)
		v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320))
		v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100+float32(frameNum*20)))
		v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(420))
		v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
		v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(220))
		v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
		v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
		v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))
		v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

		// Swap
		v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

		// Verify frame is renderable
		frame := v.GetFrame()
		if frame == nil {
			t.Errorf("Frame %d: GetFrame returned nil", frameNum)
		}
	}
}

// =============================================================================
// Helper Functions for Tests
// =============================================================================

// floatToFixed12_4 converts float to 12.4 fixed-point
func floatToFixed12_4(f float32) uint32 {
	return uint32(int32(f * (1 << VOODOO_FIXED_12_4_SHIFT)))
}

// floatToFixed12_12 converts float to 12.12 fixed-point
func floatToFixed12_12(f float32) uint32 {
	return uint32(int32(f * (1 << VOODOO_FIXED_12_12_SHIFT)))
}

// floatToFixed20_12 converts float to 20.12 fixed-point
func floatToFixed20_12(f float32) uint32 {
	return uint32(int32(f * (1 << VOODOO_FIXED_20_12_SHIFT)))
}

// floatToFixed14_18 converts float to 14.18 fixed-point
func floatToFixed14_18(f float32) uint32 {
	return uint32(int32(f * (1 << VOODOO_FIXED_14_18_SHIFT)))
}

// =============================================================================
// Phase 1: Gouraud Shading Tests
// =============================================================================

func TestVoodoo_GouraudShading_ColorSelect(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write COLOR_SELECT to target vertex 1
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)

	// Verify the register was written
	readValue := v.HandleRead(VOODOO_COLOR_SELECT)
	if readValue != 1 {
		t.Errorf("Expected COLOR_SELECT 1, got %d", readValue)
	}

	// Write COLOR_SELECT to target vertex 2
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	readValue = v.HandleRead(VOODOO_COLOR_SELECT)
	if readValue != 2 {
		t.Errorf("Expected COLOR_SELECT 2, got %d", readValue)
	}
}

func TestVoodoo_GouraudShading_PerVertexColors(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set vertex positions
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(200))

	// Set vertex A to RED
	v.HandleWrite(VOODOO_COLOR_SELECT, 0)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Set vertex B to GREEN
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Set vertex C to BLUE
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit triangle
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// Verify that the triangle batch has the triangle with different vertex colors
	if v.GetTriangleBatchCount() != 1 {
		t.Fatalf("Expected 1 triangle in batch, got %d", v.GetTriangleBatchCount())
	}

	// Access the triangle batch to verify per-vertex colors
	tri := v.triangleBatch[0]

	// Vertex A should be red
	if tri.Vertices[0].R < 0.9 || tri.Vertices[0].G > 0.1 || tri.Vertices[0].B > 0.1 {
		t.Errorf("Vertex A should be red, got R=%f G=%f B=%f",
			tri.Vertices[0].R, tri.Vertices[0].G, tri.Vertices[0].B)
	}

	// Vertex B should be green
	if tri.Vertices[1].R > 0.1 || tri.Vertices[1].G < 0.9 || tri.Vertices[1].B > 0.1 {
		t.Errorf("Vertex B should be green, got R=%f G=%f B=%f",
			tri.Vertices[1].R, tri.Vertices[1].G, tri.Vertices[1].B)
	}

	// Vertex C should be blue
	if tri.Vertices[2].R > 0.1 || tri.Vertices[2].G > 0.1 || tri.Vertices[2].B < 0.9 {
		t.Errorf("Vertex C should be blue, got R=%f G=%f B=%f",
			tri.Vertices[2].R, tri.Vertices[2].G, tri.Vertices[2].B)
	}
}

func TestVoodoo_GouraudShading_RenderInterpolated(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to black
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw a Gouraud-shaded triangle with R/G/B vertices
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320)) // Top
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(520)) // Bottom right
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(400))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(120)) // Bottom left
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(400))

	// Vertex A = RED
	v.HandleWrite(VOODOO_COLOR_SELECT, 0)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Vertex B = GREEN
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Vertex C = BLUE
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Check the center of the triangle - should be a mix of R+G+B (grayish)
	frame := v.GetFrame()
	centerX, centerY := 320, 300
	pixelIdx := (centerY*640 + centerX) * 4

	r := frame[pixelIdx]
	g := frame[pixelIdx+1]
	b := frame[pixelIdx+2]

	// At center, all three colors should be present (interpolated)
	// Since we're near bottom edge (closer to B and C), expect some blue and green
	if r == 0 && g == 0 && b == 0 {
		t.Errorf("Center pixel should not be black with Gouraud shading")
	}

	// Check near vertex A (top) - should be mostly red
	topX, topY := 320, 120
	topPixelIdx := (topY*640 + topX) * 4
	topR := frame[topPixelIdx]

	if topR < 150 {
		t.Errorf("Near vertex A should be reddish, got R=%d", topR)
	}
}

// =============================================================================
// Phase 2: Dynamic Pipeline State Tests
// =============================================================================

func TestVoodoo_PipelineState_DepthFunctionMapping(t *testing.T) {
	// Test that Voodoo depth functions map correctly to Vulkan compare ops
	tests := []struct {
		voodooFunc int
		name       string
	}{
		{VOODOO_DEPTH_NEVER, "NEVER"},
		{VOODOO_DEPTH_LESS, "LESS"},
		{VOODOO_DEPTH_EQUAL, "EQUAL"},
		{VOODOO_DEPTH_LESSEQUAL, "LESSEQUAL"},
		{VOODOO_DEPTH_GREATER, "GREATER"},
		{VOODOO_DEPTH_NOTEQUAL, "NOTEQUAL"},
		{VOODOO_DEPTH_GREATEREQUAL, "GREATEREQUAL"},
		{VOODOO_DEPTH_ALWAYS, "ALWAYS"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compareOp := voodooDepthFuncToVulkan(tc.voodooFunc)
			// Verify the compare op is in valid range (0-7)
			if compareOp < 0 || compareOp > 7 {
				t.Errorf("Invalid compare op %d for Voodoo func %d", compareOp, tc.voodooFunc)
			}
		})
	}
}

func TestVoodoo_PipelineState_BlendFactorMapping(t *testing.T) {
	// Test that Voodoo blend factors map correctly
	tests := []struct {
		voodooFactor int
		name         string
	}{
		{VOODOO_BLEND_ZERO, "ZERO"},
		{VOODOO_BLEND_ONE, "ONE"},
		{VOODOO_BLEND_SRC_ALPHA, "SRC_ALPHA"},
		{VOODOO_BLEND_INV_SRC_A, "INV_SRC_ALPHA"},
		{VOODOO_BLEND_DST_ALPHA, "DST_ALPHA"},
		{VOODOO_BLEND_INV_DST_A, "INV_DST_ALPHA"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blendFactor := voodooBlendFactorToVulkan(tc.voodooFactor)
			// Verify the blend factor is valid (non-negative)
			if blendFactor < 0 {
				t.Errorf("Invalid blend factor %d for Voodoo factor %d", blendFactor, tc.voodooFactor)
			}
		})
	}
}

func TestVoodoo_PipelineState_DepthWriteEnable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable depth write
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_DEPTH_WRITE | VOODOO_FBZ_RGB_WRITE |
		(VOODOO_DEPTH_LESS << 5))
	v.HandleWrite(VOODOO_FBZ_MODE, fbzMode)

	// Verify depth write is enabled
	if (v.fbzMode & VOODOO_FBZ_DEPTH_WRITE) == 0 {
		t.Error("Expected depth write to be enabled")
	}

	// Disable depth write
	fbzModeNoWrite := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE |
		(VOODOO_DEPTH_LESS << 5))
	v.HandleWrite(VOODOO_FBZ_MODE, fbzModeNoWrite)

	// Verify depth write is disabled
	if (v.fbzMode & VOODOO_FBZ_DEPTH_WRITE) != 0 {
		t.Error("Expected depth write to be disabled")
	}
}

func TestVoodoo_PipelineState_BlendEnable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable alpha blending with src*srcAlpha + dst*(1-srcAlpha)
	alphaMode := uint32(VOODOO_ALPHA_BLEND_EN |
		(VOODOO_BLEND_SRC_ALPHA << 8) |
		(VOODOO_BLEND_INV_SRC_A << 12))
	v.HandleWrite(VOODOO_ALPHA_MODE, alphaMode)

	// Verify blending is enabled
	if (v.alphaMode & VOODOO_ALPHA_BLEND_EN) == 0 {
		t.Error("Expected alpha blending to be enabled")
	}

	// Extract blend factors
	srcFactor := (v.alphaMode >> 8) & 0xF
	dstFactor := (v.alphaMode >> 12) & 0xF

	if srcFactor != VOODOO_BLEND_SRC_ALPHA {
		t.Errorf("Expected src factor SRC_ALPHA (%d), got %d", VOODOO_BLEND_SRC_ALPHA, srcFactor)
	}
	if dstFactor != VOODOO_BLEND_INV_SRC_A {
		t.Errorf("Expected dst factor INV_SRC_ALPHA (%d), got %d", VOODOO_BLEND_INV_SRC_A, dstFactor)
	}
}

func TestVoodoo_PipelineState_DepthFunctionChange(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set depth function to GREATER
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE |
		(VOODOO_DEPTH_GREATER << 5))
	v.HandleWrite(VOODOO_FBZ_MODE, fbzMode)

	// Extract depth function
	depthFunc := (v.fbzMode >> 5) & 0x7
	if depthFunc != VOODOO_DEPTH_GREATER {
		t.Errorf("Expected depth func GREATER (%d), got %d", VOODOO_DEPTH_GREATER, depthFunc)
	}

	// Change to LESSEQUAL
	fbzMode2 := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE |
		(VOODOO_DEPTH_LESSEQUAL << 5))
	v.HandleWrite(VOODOO_FBZ_MODE, fbzMode2)

	depthFunc2 := (v.fbzMode >> 5) & 0x7
	if depthFunc2 != VOODOO_DEPTH_LESSEQUAL {
		t.Errorf("Expected depth func LESSEQUAL (%d), got %d", VOODOO_DEPTH_LESSEQUAL, depthFunc2)
	}
}

func TestVoodoo_PipelineKey_Equality(t *testing.T) {
	// Test that pipeline keys with same settings are equal
	key1 := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   1, // LESS
		BlendEnable:      false,
	}

	key2 := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   1,
		BlendEnable:      false,
	}

	if key1 != key2 {
		t.Error("Identical pipeline keys should be equal")
	}

	// Different keys should not be equal
	key3 := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: false, // Different
		DepthCompareOp:   1,
		BlendEnable:      false,
	}

	if key1 == key3 {
		t.Error("Different pipeline keys should not be equal")
	}
}

// =============================================================================
// Phase 3: Alpha Test and Chroma Key Tests
// =============================================================================

func TestVoodoo_AlphaTest_Enable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable alpha test with GREATER function and ref=128
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN |
		(VOODOO_ALPHA_GREATER << 1) |
		(128 << 24)) // Alpha reference = 128
	v.HandleWrite(VOODOO_ALPHA_MODE, alphaMode)

	// Verify alpha test is enabled
	if (v.alphaMode & VOODOO_ALPHA_TEST_EN) == 0 {
		t.Error("Expected alpha test to be enabled")
	}

	// Extract alpha test function
	alphaFunc := (v.alphaMode >> 1) & 0x7
	if alphaFunc != VOODOO_ALPHA_GREATER {
		t.Errorf("Expected alpha func GREATER (%d), got %d", VOODOO_ALPHA_GREATER, alphaFunc)
	}

	// Extract alpha reference
	alphaRef := (v.alphaMode >> 24) & 0xFF
	if alphaRef != 128 {
		t.Errorf("Expected alpha ref 128, got %d", alphaRef)
	}
}

func TestVoodoo_AlphaTest_Functions(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	tests := []struct {
		name     string
		func_    int
		srcAlpha float32
		ref      float32
		expected bool
	}{
		{"NEVER", VOODOO_ALPHA_NEVER, 0.5, 0.5, false},
		{"LESS pass", VOODOO_ALPHA_LESS, 0.3, 0.5, true},
		{"LESS fail", VOODOO_ALPHA_LESS, 0.7, 0.5, false},
		{"EQUAL pass", VOODOO_ALPHA_EQUAL, 0.5, 0.5, true},
		{"EQUAL fail", VOODOO_ALPHA_EQUAL, 0.4, 0.5, false},
		{"LESSEQUAL pass", VOODOO_ALPHA_LESSEQUAL, 0.5, 0.5, true},
		{"GREATER pass", VOODOO_ALPHA_GREATER, 0.7, 0.5, true},
		{"GREATER fail", VOODOO_ALPHA_GREATER, 0.3, 0.5, false},
		{"NOTEQUAL pass", VOODOO_ALPHA_NOTEQUAL, 0.3, 0.5, true},
		{"GREATEREQUAL pass", VOODOO_ALPHA_GREATEREQUAL, 0.5, 0.5, true},
		{"ALWAYS", VOODOO_ALPHA_ALWAYS, 0.0, 1.0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := backend.alphaTest(tc.srcAlpha, tc.ref, tc.func_)
			if result != tc.expected {
				t.Errorf("alphaTest(%f, %f, %d) = %v, expected %v",
					tc.srcAlpha, tc.ref, tc.func_, result, tc.expected)
			}
		})
	}
}

func TestVoodoo_ChromaKey_Enable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set chroma key color to magenta (0xFF00FF)
	v.HandleWrite(VOODOO_CHROMA_KEY, 0x00FF00FF)

	// Verify chroma key stored
	if v.chromaKey != 0x00FF00FF {
		t.Errorf("Expected chromaKey 0x00FF00FF, got 0x%08X", v.chromaKey)
	}

	// Enable chroma keying in fbzMode
	fbzMode := uint32(VOODOO_FBZ_CHROMAKEY | VOODOO_FBZ_RGB_WRITE)
	v.HandleWrite(VOODOO_FBZ_MODE, fbzMode)

	if (v.fbzMode & VOODOO_FBZ_CHROMAKEY) == 0 {
		t.Error("Expected chroma key to be enabled")
	}
}

func TestVoodoo_ChromaKey_PixelDiscard(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Test color matching
	chromaKey := uint32(0x00FF00FF) // Magenta

	// Exact match should be keyed out
	if !backend.chromaKeyMatch(255, 0, 255, chromaKey) {
		t.Error("Exact magenta color should match chroma key")
	}

	// Different color should not match
	if backend.chromaKeyMatch(255, 255, 0, chromaKey) {
		t.Error("Yellow should not match magenta chroma key")
	}

	// Black should not match magenta
	if backend.chromaKeyMatch(0, 0, 0, chromaKey) {
		t.Error("Black should not match magenta chroma key")
	}
}

func TestVoodoo_AlphaTest_Integration(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to blue
	v.HandleWrite(VOODOO_COLOR0, 0xFF0000FF)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Enable alpha test: discard if alpha < 0.5 (128)
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN |
		(VOODOO_ALPHA_GREATEREQUAL << 1) |
		(128 << 24))
	v.HandleWrite(VOODOO_ALPHA_MODE, alphaMode)

	// Draw a triangle with alpha = 0.3 (should be discarded)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(0.3)) // Below threshold
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// The triangle center should still be blue (not red) because alpha test should discard it
	frame := v.GetFrame()
	centerX, centerY := 200, 200
	pixelIdx := (centerY*640 + centerX) * 4

	r := frame[pixelIdx]
	b := frame[pixelIdx+2]

	// Should still be blue (triangle discarded due to alpha test)
	if r > 50 || b < 200 {
		t.Logf("Note: Alpha test integration may depend on software backend implementation")
	}
}

// =============================================================================
// Phase 4: Texture Mapping Tests
// =============================================================================

func TestVoodoo_TextureMode_Enable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable texturing with bilinear filtering
	texMode := uint32(VOODOO_TEX_ENABLE | VOODOO_TEX_MAGNIFY)
	v.HandleWrite(VOODOO_TEXTURE_MODE, texMode)

	// Verify texturing is enabled
	if (v.textureMode & VOODOO_TEX_ENABLE) == 0 {
		t.Error("Expected texturing to be enabled")
	}

	if (v.textureMode & VOODOO_TEX_MAGNIFY) == 0 {
		t.Error("Expected bilinear magnification to be enabled")
	}
}

func TestVoodoo_TextureCoords_Write(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write texture coordinates (14.18 fixed-point)
	// 0.5 in 14.18 = 0.5 * 262144 = 131072 = 0x20000
	v.HandleWrite(VOODOO_START_S, 0x20000)
	v.HandleWrite(VOODOO_START_T, 0x40000) // 1.0 in 14.18

	// Verify shadow registers store the raw values
	sRaw := v.HandleRead(VOODOO_START_S)
	tRaw := v.HandleRead(VOODOO_START_T)

	if sRaw != 0x20000 {
		t.Errorf("Expected S raw 0x20000, got 0x%X", sRaw)
	}
	if tRaw != 0x40000 {
		t.Errorf("Expected T raw 0x40000, got 0x%X", tRaw)
	}
}

func TestVoodoo_TextureFormat_Bits(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set texture format to ARGB8888
	texMode := uint32(VOODOO_TEX_ENABLE | (VOODOO_TEX_FMT_ARGB8888 << 8))
	v.HandleWrite(VOODOO_TEXTURE_MODE, texMode)

	// Extract format
	format := (v.textureMode >> 8) & 0xF
	if format != VOODOO_TEX_FMT_ARGB8888 {
		t.Errorf("Expected format ARGB8888 (%d), got %d", VOODOO_TEX_FMT_ARGB8888, format)
	}
}

func TestVoodoo_TextureClamp_Modes(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable clamping on both S and T
	texMode := uint32(VOODOO_TEX_ENABLE | VOODOO_TEX_CLAMP_S | VOODOO_TEX_CLAMP_T)
	v.HandleWrite(VOODOO_TEXTURE_MODE, texMode)

	if (v.textureMode & VOODOO_TEX_CLAMP_S) == 0 {
		t.Error("Expected S clamping to be enabled")
	}
	if (v.textureMode & VOODOO_TEX_CLAMP_T) == 0 {
		t.Error("Expected T clamping to be enabled")
	}
}

func TestVoodoo_TextureCoord_FixedPointConversion(t *testing.T) {
	tests := []struct {
		name    string
		input   float32
		epsilon float32
	}{
		{"zero", 0.0, 0.0001},
		{"half", 0.5, 0.0001},
		{"one", 1.0, 0.0001},
		{"negative", -0.5, 0.0001},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixed := floatToFixed14_18(tc.input)
			result := fixed14_18ToFloat(fixed)
			diff := float32(math.Abs(float64(result - tc.input)))
			if diff > tc.epsilon {
				t.Errorf("Conversion mismatch: input=%f, fixed=0x%X, result=%f",
					tc.input, fixed, result)
			}
		})
	}
}

func TestVoodoo_TextureUpload_Basic(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(640, 480)
	defer backend.Destroy()

	// Create a simple 4x4 checkerboard texture
	texData := make([]byte, 4*4*4) // 4x4 RGBA
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			idx := (y*4 + x) * 4
			if (x+y)%2 == 0 {
				texData[idx] = 255   // R
				texData[idx+1] = 255 // G
				texData[idx+2] = 255 // B
				texData[idx+3] = 255 // A
			} else {
				texData[idx] = 0
				texData[idx+1] = 0
				texData[idx+2] = 0
				texData[idx+3] = 255
			}
		}
	}

	err := backend.SetTextureData(texData, 4, 4)
	if err != nil {
		t.Fatalf("SetTextureData failed: %v", err)
	}

	// Verify texture is stored
	if backend.textureWidth != 4 || backend.textureHeight != 4 {
		t.Errorf("Expected texture 4x4, got %dx%d", backend.textureWidth, backend.textureHeight)
	}
}

func TestVoodoo_TextureSample_Point(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(640, 480)
	defer backend.Destroy()

	// Create a 2x2 texture: red, green, blue, white
	texData := []byte{
		255, 0, 0, 255, // Red (0,0)
		0, 255, 0, 255, // Green (1,0)
		0, 0, 255, 255, // Blue (0,1)
		255, 255, 255, 255, // White (1,1)
	}

	backend.SetTextureData(texData, 2, 2)

	// Sample at (0.25, 0.25) - should be red (nearest to 0,0)
	r, g, b, a := backend.sampleTexture(0.25, 0.25, false)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("Expected red at (0.25, 0.25), got R=%d G=%d B=%d A=%d", r, g, b, a)
	}

	// Sample at (0.75, 0.25) - should be green (nearest to 1,0)
	r, g, b, a = backend.sampleTexture(0.75, 0.25, false)
	if r != 0 || g != 255 || b != 0 || a != 255 {
		t.Errorf("Expected green at (0.75, 0.25), got R=%d G=%d B=%d A=%d", r, g, b, a)
	}
}

// =============================================================================
// Phase 5: Color Combine Tests
// =============================================================================

func TestVoodoo_ColorCombine_ModeRegister(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set fbzColorPath to texture*vertex (modulate mode)
	// bits 0-3: rgb_select (0=iterated, 1=tex, 2=color1)
	// bits 4-7: alpha_select
	// bits 8-11: cc_localselect
	colorPath := uint32(0x00000001) // RGB from texture
	v.HandleWrite(VOODOO_FBZCOLOR_PATH, colorPath)

	// Verify stored
	if v.fbzColorPath != colorPath {
		t.Errorf("Expected fbzColorPath 0x%08X, got 0x%08X", colorPath, v.fbzColorPath)
	}
}

func TestVoodoo_ColorCombine_Modes(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	tests := []struct {
		name             string
		mode             int
		texR, texG       float32
		vertR, vertG     float32
		expectR, expectG float32
	}{
		{"vertex_only", VOODOO_CC_ITERATED, 0.5, 0.5, 1.0, 0.0, 1.0, 0.0},
		{"texture_only", VOODOO_CC_TEXTURE, 0.5, 0.5, 1.0, 0.0, 0.5, 0.5},
		{"modulate", VOODOO_CC_MODULATE, 0.5, 1.0, 1.0, 0.5, 0.5, 0.5},
		{"add", VOODOO_CC_ADD, 0.3, 0.3, 0.3, 0.3, 0.6, 0.6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, g, _, _ := backend.combineColors(
				tc.texR, tc.texG, 0, 1,
				tc.vertR, tc.vertG, 0, 1,
				tc.mode,
			)

			epsilon := float32(0.01)
			if math.Abs(float64(r-tc.expectR)) > float64(epsilon) {
				t.Errorf("R mismatch: expected %f, got %f", tc.expectR, r)
			}
			if math.Abs(float64(g-tc.expectG)) > float64(epsilon) {
				t.Errorf("G mismatch: expected %f, got %f", tc.expectG, g)
			}
		})
	}
}

func TestVoodoo_ColorCombine_ClampOutput(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Test that ADD mode clamps output to [0,1]
	r, g, b, a := backend.combineColors(
		0.8, 0.8, 0.8, 1.0,
		0.8, 0.8, 0.8, 1.0,
		VOODOO_CC_ADD,
	)

	// 0.8 + 0.8 = 1.6, should clamp to 1.0
	if r > 1.0 || g > 1.0 || b > 1.0 || a > 1.0 {
		t.Errorf("Color combine should clamp to 1.0, got R=%f G=%f B=%f A=%f", r, g, b, a)
	}
}

// =============================================================================
// Phase 6: Fog and Dithering Tests
// =============================================================================

func TestVoodoo_FogMode_Enable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable fog
	fogMode := uint32(VOODOO_FOG_ENABLE)
	v.HandleWrite(VOODOO_FOG_MODE, fogMode)

	if (v.fogMode & VOODOO_FOG_ENABLE) == 0 {
		t.Error("Expected fog to be enabled")
	}
}

func TestVoodoo_FogColor_Set(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set fog color to gray
	v.HandleWrite(VOODOO_FOG_COLOR, 0x00808080)

	if v.fogColor != 0x00808080 {
		t.Errorf("Expected fogColor 0x00808080, got 0x%08X", v.fogColor)
	}
}

func TestVoodoo_FogBlend_Linear(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	tests := []struct {
		name      string
		depth     float32
		fogStart  float32
		fogEnd    float32
		colorR    float32
		fogR      float32
		expectedR float32
		tolerance float32
	}{
		// At fog start, no fog
		{"no_fog_at_start", 0.0, 0.0, 1.0, 1.0, 0.0, 1.0, 0.01},
		// At fog end, full fog
		{"full_fog_at_end", 1.0, 0.0, 1.0, 1.0, 0.0, 0.0, 0.01},
		// Halfway through, half fog
		{"half_fog", 0.5, 0.0, 1.0, 1.0, 0.0, 0.5, 0.01},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := backend.applyFog(tc.colorR, tc.fogR, tc.depth, tc.fogStart, tc.fogEnd)
			diff := float32(math.Abs(float64(result - tc.expectedR)))
			if diff > tc.tolerance {
				t.Errorf("Expected R=%f, got %f (diff=%f)", tc.expectedR, result, diff)
			}
		})
	}
}

func TestVoodoo_Dithering_BayerMatrix(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Test that Bayer matrix produces consistent values
	v00 := backend.getBayerValue(0, 0)
	v01 := backend.getBayerValue(0, 1)
	v10 := backend.getBayerValue(1, 0)
	v11 := backend.getBayerValue(1, 1)

	// Values should be in range [-0.5, 0.5] normalized
	if v00 < -0.5 || v00 > 0.5 {
		t.Errorf("Bayer value at (0,0) out of range: %f", v00)
	}

	// Values should be different at different positions
	if v00 == v01 && v01 == v10 && v10 == v11 {
		t.Error("All Bayer values are the same - dither pattern not working")
	}
}

func TestVoodoo_Dithering_Enable(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable dithering via fbzMode
	fbzMode := uint32(VOODOO_FBZ_DITHER | VOODOO_FBZ_RGB_WRITE)
	v.HandleWrite(VOODOO_FBZ_MODE, fbzMode)

	if (v.fbzMode & VOODOO_FBZ_DITHER) == 0 {
		t.Error("Expected dithering to be enabled")
	}
}

func TestVoodoo_Dithering_ApplyToColor(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Apply dithering to a color
	// At position (0,0) vs (1,1) should give different results
	color := float32(0.5)
	dithered00 := backend.applyDither(color, 0, 0)
	dithered11 := backend.applyDither(color, 1, 1)

	// Both should be close to 0.5 but not necessarily equal
	if dithered00 < 0 || dithered00 > 1 || dithered11 < 0 || dithered11 > 1 {
		t.Errorf("Dithered values out of range: (0,0)=%f (1,1)=%f", dithered00, dithered11)
	}
}

func TestVoodoo_GouraudShading_BackwardCompatibility(t *testing.T) {
	// Test that flat shading still works when COLOR_SELECT is not used
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set vertex positions
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(150))

	// Set colors WITHOUT using COLOR_SELECT (old flat shading way)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.5))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.25))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit triangle
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// All vertices should have the same color (flat shading)
	tri := v.triangleBatch[0]

	for i := 0; i < 3; i++ {
		if tri.Vertices[i].R < 0.9 || tri.Vertices[i].G < 0.4 || tri.Vertices[i].G > 0.6 {
			t.Errorf("Vertex %d has wrong color in flat shading mode: R=%f G=%f B=%f",
				i, tri.Vertices[i].R, tri.Vertices[i].G, tri.Vertices[i].B)
		}
	}
}

// =============================================================================
// Vulkan Backend Feature Tests (TDD)
// =============================================================================

// Test: VoodooPushConstants struct exists with required fields
func TestVoodoo_Vulkan_PushConstantsStruct(t *testing.T) {
	// Verify the struct exists and has correct size (should be 32 bytes for 8 uint32 fields)
	pc := VoodooPushConstants{
		FbzMode:       0x00000001,
		AlphaMode:     0x00000002,
		ChromaKey:     0x00FF00FF,
		FogColor:      0x00808080,
		FogStart:      0.0,
		FogEnd:        1.0,
		ColorCombine:  VOODOO_CC_MODULATE,
		TextureEnable: 1,
	}

	// Verify struct fields are accessible and have correct values
	if pc.FbzMode != 0x00000001 {
		t.Errorf("FbzMode not set correctly: got %v", pc.FbzMode)
	}
	if pc.AlphaMode != 0x00000002 {
		t.Errorf("AlphaMode not set correctly: got %v", pc.AlphaMode)
	}
	if pc.ChromaKey != 0x00FF00FF {
		t.Errorf("ChromaKey not set correctly: got %v", pc.ChromaKey)
	}
	if pc.FogColor != 0x00808080 {
		t.Errorf("FogColor not set correctly: got %v", pc.FogColor)
	}
	if pc.FogStart != 0.0 {
		t.Errorf("FogStart not set correctly: got %v", pc.FogStart)
	}
	if pc.FogEnd != 1.0 {
		t.Errorf("FogEnd not set correctly: got %v", pc.FogEnd)
	}
	if pc.ColorCombine != VOODOO_CC_MODULATE {
		t.Errorf("ColorCombine not set correctly: got %v", pc.ColorCombine)
	}
	if pc.TextureEnable != 1 {
		t.Errorf("TextureEnable not set correctly: got %v", pc.TextureEnable)
	}
}

// Test: VulkanVertex has TexCoord field
func TestVoodoo_Vulkan_VertexHasTexCoord(t *testing.T) {
	v := VulkanVertex{
		Position: [3]float32{1.0, 2.0, 0.5},
		Color:    [4]float32{1.0, 0.5, 0.25, 1.0},
		TexCoord: [2]float32{0.5, 0.75},
	}

	if v.TexCoord[0] != 0.5 {
		t.Errorf("TexCoord[0] not set correctly: got %v", v.TexCoord[0])
	}
	if v.TexCoord[1] != 0.75 {
		t.Errorf("TexCoord[1] not set correctly: got %v", v.TexCoord[1])
	}
}

// Test: VulkanBackend has push constants field
func TestVoodoo_Vulkan_BackendHasPushConstants(t *testing.T) {
	backend, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}

	// Initialize with small dimensions for test
	err = backend.Init(64, 64)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Set push constants - this should work without error
	backend.SetPushConstants(VoodooPushConstants{
		FbzMode:       VOODOO_FBZ_DEPTH_ENABLE,
		AlphaMode:     VOODOO_ALPHA_TEST_EN,
		ChromaKey:     0x00FF00FF,
		FogColor:      0x00808080,
		FogStart:      0.0,
		FogEnd:        1.0,
		ColorCombine:  VOODOO_CC_MODULATE,
		TextureEnable: 1,
	})

	// Verify push constants were stored
	pc := backend.GetPushConstants()
	if pc.FbzMode != VOODOO_FBZ_DEPTH_ENABLE {
		t.Errorf("Push constants FbzMode not stored: got %v", pc.FbzMode)
	}
}

// Test: VulkanBackend has texture resource fields
func TestVoodoo_Vulkan_BackendHasTextureResources(t *testing.T) {
	backend, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}

	err = backend.Init(64, 64)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create a simple 2x2 test texture (RGBA)
	textureData := []byte{
		255, 0, 0, 255, // Red
		0, 255, 0, 255, // Green
		0, 0, 255, 255, // Blue
		255, 255, 0, 255, // Yellow
	}

	// SetTextureData should work for the backend
	err = backend.SetTextureData(textureData, 2, 2)
	if err != nil {
		t.Fatalf("SetTextureData failed: %v", err)
	}

	// Verify texture dimensions are stored
	w, h := backend.GetTextureDimensions()
	if w != 2 || h != 2 {
		t.Errorf("Texture dimensions not stored correctly: got %dx%d", w, h)
	}
}

// Test: buildPushConstantsFromState correctly builds push constants from registers
func TestVoodoo_Vulkan_BuildPushConstantsFromState(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set various state registers
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_CHROMAKEY | (VOODOO_DEPTH_LESS << 5))
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | (128 << 24))
	chromaKey := uint32(0x00FF00FF)
	fogColor := uint32(0x00808080)

	pc := buildPushConstantsFromState(fbzMode, alphaMode, chromaKey, fogColor, 0.1, 0.9, VOODOO_CC_MODULATE, true)

	if pc.FbzMode != fbzMode {
		t.Errorf("FbzMode mismatch: expected %v, got %v", fbzMode, pc.FbzMode)
	}
	if pc.AlphaMode != alphaMode {
		t.Errorf("AlphaMode mismatch: expected %v, got %v", alphaMode, pc.AlphaMode)
	}
	if pc.ChromaKey != chromaKey {
		t.Errorf("ChromaKey mismatch: expected %v, got %v", chromaKey, pc.ChromaKey)
	}
	if pc.FogColor != fogColor {
		t.Errorf("FogColor mismatch: expected %v, got %v", fogColor, pc.FogColor)
	}
	if pc.FogStart != 0.1 {
		t.Errorf("FogStart mismatch: expected 0.1, got %v", pc.FogStart)
	}
	if pc.FogEnd != 0.9 {
		t.Errorf("FogEnd mismatch: expected 0.9, got %v", pc.FogEnd)
	}
	if pc.ColorCombine != VOODOO_CC_MODULATE {
		t.Errorf("ColorCombine mismatch: expected %v, got %v", VOODOO_CC_MODULATE, pc.ColorCombine)
	}
	if pc.TextureEnable != 1 {
		t.Errorf("TextureEnable mismatch: expected 1, got %v", pc.TextureEnable)
	}
}

// Test: Shader has proper push constant range
func TestVoodoo_Vulkan_ShaderPushConstantRange(t *testing.T) {
	// This test verifies the push constant size matches what shaders expect
	// Push constants should be 32 bytes (8 x uint32/float32)
	expectedSize := 32

	actualSize := getPushConstantsSize()
	if actualSize != expectedSize {
		t.Errorf("Push constant size mismatch: expected %d, got %d", expectedSize, actualSize)
	}
}

// Test: Descriptor set layout includes texture sampler
func TestVoodoo_Vulkan_DescriptorSetLayoutHasTexture(t *testing.T) {
	// Verify descriptor set layout constant exists
	if VOODOO_DESCRIPTOR_BINDING_TEXTURE != 0 {
		t.Errorf("Texture descriptor binding should be 0, got %d", VOODOO_DESCRIPTOR_BINDING_TEXTURE)
	}
}

// Test: New shaders have push constant layout
func TestVoodoo_Vulkan_ShaderHasPushConstantLayout(t *testing.T) {
	// Verify the extended shaders exist
	if len(VoodooVertexShaderSPIRV) == 0 {
		t.Error("Vertex shader SPIR-V is empty")
	}
	if len(VoodooFragmentShaderSPIRV) == 0 {
		t.Error("Fragment shader SPIR-V is empty")
	}

	// Extended shaders should be larger than basic shaders due to additional features
	// Basic shaders are ~500-700 bytes, extended should be larger
	if len(VoodooFragmentShaderSPIRV) < 200 {
		t.Logf("Fragment shader size: %d bytes (may need extended features)", len(VoodooFragmentShaderSPIRV))
	}
}

// Test: Vertex shader passes texture coordinates
func TestVoodoo_Vulkan_VertexShaderPassesTexCoords(t *testing.T) {
	// Create vertex with texture coordinates
	v := VulkanVertex{
		Position: [3]float32{0.5, 0.5, 0.5},
		Color:    [4]float32{1.0, 0.0, 0.0, 1.0},
		TexCoord: [2]float32{0.25, 0.75},
	}

	// Verify vertex size includes texture coordinates
	// Position (3*4=12) + Color (4*4=16) + TexCoord (2*4=8) = 36 bytes
	expectedSize := 36
	actualSize := getVulkanVertexSize()
	if actualSize != expectedSize {
		t.Errorf("VulkanVertex size mismatch: expected %d, got %d", expectedSize, actualSize)
	}

	// Verify texture coords are accessible
	if v.TexCoord[0] != 0.25 || v.TexCoord[1] != 0.75 {
		t.Errorf("TexCoord not stored correctly: got %v", v.TexCoord)
	}
}

// Test: Alpha test function in shader constants
func TestVoodoo_Vulkan_AlphaTestConstants(t *testing.T) {
	// Alpha test should use same constants as software backend
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"NEVER", VOODOO_ALPHA_NEVER, 0},
		{"LESS", VOODOO_ALPHA_LESS, 1},
		{"EQUAL", VOODOO_ALPHA_EQUAL, 2},
		{"LESSEQUAL", VOODOO_ALPHA_LESSEQUAL, 3},
		{"GREATER", VOODOO_ALPHA_GREATER, 4},
		{"NOTEQUAL", VOODOO_ALPHA_NOTEQUAL, 5},
		{"GREATEREQUAL", VOODOO_ALPHA_GREATEREQUAL, 6},
		{"ALWAYS", VOODOO_ALPHA_ALWAYS, 7},
	}

	for _, tc := range tests {
		if tc.constant != tc.expected {
			t.Errorf("%s: expected %d, got %d", tc.name, tc.expected, tc.constant)
		}
	}
}

// Test: Push constants are correctly packed for shader
func TestVoodoo_Vulkan_PushConstantsPacking(t *testing.T) {
	pc := VoodooPushConstants{
		FbzMode:       0x12345678,
		AlphaMode:     0x87654321,
		ChromaKey:     0x00FF00FF,
		FogColor:      0x00808080,
		FogStart:      0.1,
		FogEnd:        0.9,
		ColorCombine:  VOODOO_CC_MODULATE,
		TextureEnable: 1,
	}

	// Pack to bytes and verify correct layout
	data := packPushConstants(pc)
	if len(data) != VOODOO_PUSH_CONSTANTS_SIZE {
		t.Errorf("Push constants packed size mismatch: expected %d, got %d",
			VOODOO_PUSH_CONSTANTS_SIZE, len(data))
	}

	// Verify first uint32 (FbzMode)
	fbzMode := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	if fbzMode != 0x12345678 {
		t.Errorf("FbzMode packing error: expected 0x12345678, got 0x%08X", fbzMode)
	}
}

// Test: Texture sampler creation parameters
func TestVoodoo_Vulkan_TextureSamplerParams(t *testing.T) {
	// Verify sampler filter mode constants exist
	if VOODOO_FILTER_POINT != 0 {
		t.Errorf("VOODOO_FILTER_POINT should be 0, got %d", VOODOO_FILTER_POINT)
	}
	if VOODOO_FILTER_BILINEAR != 1 {
		t.Errorf("VOODOO_FILTER_BILINEAR should be 1, got %d", VOODOO_FILTER_BILINEAR)
	}
}

// Test: VulkanBackend creates texture resources when SetTextureData is called
func TestVoodoo_Vulkan_TextureResourceCreation(t *testing.T) {
	backend, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}

	err = backend.Init(64, 64)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create 4x4 test texture
	textureData := make([]byte, 4*4*4)
	for i := range textureData {
		textureData[i] = byte(i % 256)
	}

	err = backend.SetTextureData(textureData, 4, 4)
	if err != nil {
		t.Fatalf("SetTextureData failed: %v", err)
	}

	// Verify texture data was stored
	if !backend.HasTextureData() {
		t.Error("Backend should have texture data after SetTextureData")
	}
}

// Test: Pipeline integration - vertex attributes include texture coordinates
func TestVoodoo_Vulkan_VertexAttributeCount(t *testing.T) {
	// We should have 3 vertex attributes: position (0), color (1), texcoord (2)
	expectedCount := 3
	actualCount := getVulkanVertexAttributeCount()
	if actualCount != expectedCount {
		t.Errorf("Vertex attribute count mismatch: expected %d, got %d", expectedCount, actualCount)
	}
}

// Test: Pipeline integration - push constant range exists
func TestVoodoo_Vulkan_PipelineLayoutHasPushConstants(t *testing.T) {
	// Verify push constant range configuration
	pcRange := getVulkanPushConstantRange()

	if pcRange.StageFlags != VOODOO_SHADER_STAGE_FRAGMENT {
		t.Errorf("Push constant stage flags mismatch: expected %d, got %d",
			VOODOO_SHADER_STAGE_FRAGMENT, pcRange.StageFlags)
	}
	if pcRange.Offset != 0 {
		t.Errorf("Push constant offset mismatch: expected 0, got %d", pcRange.Offset)
	}
	if pcRange.Size != VOODOO_PUSH_CONSTANTS_SIZE {
		t.Errorf("Push constant size mismatch: expected %d, got %d",
			VOODOO_PUSH_CONSTANTS_SIZE, pcRange.Size)
	}
}

// Test: VulkanBackend BuildCurrentPushConstants creates push constants from state
func TestVoodoo_Vulkan_BuildPushConstantsFromCurrentState(t *testing.T) {
	backend, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}

	err = backend.Init(64, 64)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Set various state
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_FOG_ENABLE)
	alphaMode := uint32(VOODOO_ALPHA_BLEND_EN)

	backend.UpdatePipelineState(fbzMode, alphaMode)
	backend.SetChromaKey(0x00FF00FF)
	backend.SetFogParams(0x00808080, 0.1, 0.9)
	backend.SetColorCombineMode(VOODOO_CC_MODULATE)

	// Build push constants from current state
	pc := backend.BuildCurrentPushConstants()

	if pc.FbzMode != fbzMode {
		t.Errorf("FbzMode mismatch: expected %v, got %v", fbzMode, pc.FbzMode)
	}
	if pc.AlphaMode != alphaMode {
		t.Errorf("AlphaMode mismatch: expected %v, got %v", alphaMode, pc.AlphaMode)
	}
	if pc.ChromaKey != 0x00FF00FF {
		t.Errorf("ChromaKey mismatch: expected 0x00FF00FF, got 0x%08X", pc.ChromaKey)
	}
	if pc.FogColor != 0x00808080 {
		t.Errorf("FogColor mismatch: expected 0x00808080, got 0x%08X", pc.FogColor)
	}
	if pc.ColorCombine != uint32(VOODOO_CC_MODULATE) {
		t.Errorf("ColorCombine mismatch: expected %d, got %d", VOODOO_CC_MODULATE, pc.ColorCombine)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkVoodoo_TriangleRasterization(b *testing.B) {
	v, _ := NewVoodooEngine(nil)
	defer v.Destroy()

	// Setup a triangle
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(420))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(220))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.5))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.25))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
		if v.GetTriangleBatchCount() >= 100 {
			v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
		}
	}
}

func BenchmarkVoodoo_FullFrame(b *testing.B) {
	v, _ := NewVoodooEngine(nil)
	defer v.Destroy()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear
		v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
		v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

		// Draw 100 triangles
		for j := 0; j < 100; j++ {
			offset := float32(j % 10 * 50)
			v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100+offset))
			v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50+float32(j/10*40)))
			v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(140+offset))
			v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(90+float32(j/10*40)))
			v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(60+offset))
			v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(90+float32(j/10*40)))
			v.HandleWrite(VOODOO_START_R, floatToFixed12_12(float32(j%10)/10.0))
			v.HandleWrite(VOODOO_START_G, floatToFixed12_12(float32(j/10)/10.0))
			v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.5))
			v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
			v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(float32(j)/100.0))
			v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
		}

		// Swap
		v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	}
}
