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
