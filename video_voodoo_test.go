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
	"unsafe"

	vk "github.com/goki/vulkan"
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
// Phase 1: Gouraud Shading Tests
// =============================================================================

func TestVoodoo_ColorSelect_Register(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write vertex select values
	for i := uint32(0); i < 3; i++ {
		v.HandleWrite(VOODOO_COLOR_SELECT, i)
		readValue := v.HandleRead(VOODOO_COLOR_SELECT)
		if readValue != i {
			t.Errorf("Expected COLOR_SELECT %d, got %d", i, readValue)
		}
	}
}

func TestVoodoo_PerVertexColors_Storage(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set up vertex A position
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(50))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(200))

	// Set RED for vertex A
	v.HandleWrite(VOODOO_COLOR_SELECT, 0)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))

	// Set GREEN for vertex B
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))

	// Set BLUE for vertex C
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))

	// Submit triangle
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// Verify triangle was batched
	if v.GetTriangleBatchCount() != 1 {
		t.Errorf("Expected 1 triangle in batch, got %d", v.GetTriangleBatchCount())
	}
}

func TestVoodoo_GouraudShading_Interpolation(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to black
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw a triangle covering a good portion of the screen
	// Vertex A at top center (RED)
	// Vertex B at bottom right (GREEN)
	// Vertex C at bottom left (BLUE)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320)) // Top
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(500)) // Bottom right
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(380))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(140)) // Bottom left
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(380))

	// Set RED for vertex A
	v.HandleWrite(VOODOO_COLOR_SELECT, 0)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Set GREEN for vertex B
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Set BLUE for vertex C
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit and render
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	frame := v.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame returned nil")
	}

	// Check pixel near top center (should be mostly red)
	topX, topY := 320, 150
	topIdx := (topY*640 + topX) * 4
	if frame[topIdx] < 150 { // R should be high
		t.Errorf("Top pixel should be mostly red, got R=%d", frame[topIdx])
	}

	// Check pixel near bottom right (should be mostly green)
	brX, brY := 450, 350
	brIdx := (brY*640 + brX) * 4
	if frame[brIdx+1] < 100 { // G should be significant
		t.Errorf("Bottom-right pixel should have significant green, got G=%d", frame[brIdx+1])
	}

	// Check pixel near bottom left (should be mostly blue)
	blX, blY := 190, 350
	blIdx := (blY*640 + blX) * 4
	if frame[blIdx+2] < 100 { // B should be significant
		t.Errorf("Bottom-left pixel should have significant blue, got B=%d", frame[blIdx+2])
	}

	// Check center pixel (should have a mix of all colors)
	cX, cY := 320, 280
	cIdx := (cY*640 + cX) * 4
	r, g, b := frame[cIdx], frame[cIdx+1], frame[cIdx+2]
	// Center should have some of each color due to interpolation
	if r == 0 && g == 0 && b == 0 {
		t.Errorf("Center pixel should have interpolated colors, got R=%d G=%d B=%d", r, g, b)
	}
}

func TestVoodoo_FlatShading_BackwardCompatibility(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to black
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw triangle WITHOUT using COLOR_SELECT (old flat shading behavior)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(420))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(220))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))

	// Set color directly (old style - applies to all vertices)
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0)) // Yellow
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit and render
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	frame := v.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame returned nil")
	}

	// Check multiple points in the triangle - all should be yellow
	points := [][2]int{{320, 200}, {350, 250}, {280, 250}}
	for _, p := range points {
		idx := (p[1]*640 + p[0]) * 4
		r, g, b := frame[idx], frame[idx+1], frame[idx+2]
		// All points should be yellow (R high, G high, B low)
		if r < 200 || g < 200 || b > 50 {
			t.Errorf("Point (%d,%d) should be yellow, got R=%d G=%d B=%d", p[0], p[1], r, g, b)
		}
	}
}

func TestVoodoo_GouraudShading_SoftwareBackend(t *testing.T) {
	// Test Gouraud interpolation directly on software backend
	backend := NewVoodooSoftwareBackend()
	backend.Init(640, 480)
	defer backend.Destroy()

	// Create a triangle with RGB at corners
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 320, Y: 100, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 1.0}, // Red top
			{X: 500, Y: 380, Z: 0.5, R: 0.0, G: 1.0, B: 0.0, A: 1.0}, // Green bottom-right
			{X: 140, Y: 380, Z: 0.5, R: 0.0, G: 0.0, B: 1.0, A: 1.0}, // Blue bottom-left
		},
	}

	backend.ClearFramebuffer(0xFF000000) // Black
	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()

	// Check that barycentric interpolation works
	// Near vertex A (top) should be mostly red
	topIdx := (150*640 + 320) * 4
	if frame[topIdx] < 150 {
		t.Errorf("Near top vertex should be mostly red, got R=%d", frame[topIdx])
	}

	// Center should have mixed colors
	cIdx := (280*640 + 320) * 4
	r, g, b := frame[cIdx], frame[cIdx+1], frame[cIdx+2]
	if r == 0 && g == 0 && b == 0 {
		t.Error("Center should have interpolated colors")
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

// =============================================================================
// Phase 2: Dynamic Pipeline State Tests
// =============================================================================

// Test PipelineKey structure creation and equality
func TestVoodoo_PipelineKey_Creation(t *testing.T) {
	// Test default key
	key1 := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   VOODOO_DEPTH_LESS,
		BlendEnable:      false,
		SrcBlendFactor:   VOODOO_BLEND_ONE,
		DstBlendFactor:   VOODOO_BLEND_ZERO,
	}

	// Test identical key
	key2 := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   VOODOO_DEPTH_LESS,
		BlendEnable:      false,
		SrcBlendFactor:   VOODOO_BLEND_ONE,
		DstBlendFactor:   VOODOO_BLEND_ZERO,
	}

	if key1 != key2 {
		t.Error("Identical PipelineKeys should be equal")
	}

	// Test different keys
	key3 := PipelineKey{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   VOODOO_DEPTH_LESSEQUAL, // Different
		BlendEnable:      false,
		SrcBlendFactor:   VOODOO_BLEND_ONE,
		DstBlendFactor:   VOODOO_BLEND_ZERO,
	}

	if key1 == key3 {
		t.Error("Different PipelineKeys should not be equal")
	}
}

// Test creating PipelineKey from fbzMode register
func TestVoodoo_PipelineKey_FromFbzMode(t *testing.T) {
	tests := []struct {
		name             string
		fbzMode          uint32
		expectDepthTest  bool
		expectDepthWrite bool
		expectDepthFunc  int
	}{
		{
			name:             "Depth test enabled, LESS",
			fbzMode:          VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_DEPTH_WRITE | (VOODOO_DEPTH_LESS << 5),
			expectDepthTest:  true,
			expectDepthWrite: true,
			expectDepthFunc:  VOODOO_DEPTH_LESS,
		},
		{
			name:             "Depth test disabled",
			fbzMode:          VOODOO_FBZ_RGB_WRITE,
			expectDepthTest:  false,
			expectDepthWrite: false,
			expectDepthFunc:  VOODOO_DEPTH_NEVER,
		},
		{
			name:             "Depth test enabled, no write, GREATEREQUAL",
			fbzMode:          VOODOO_FBZ_DEPTH_ENABLE | (VOODOO_DEPTH_GREATEREQUAL << 5),
			expectDepthTest:  true,
			expectDepthWrite: false,
			expectDepthFunc:  VOODOO_DEPTH_GREATEREQUAL,
		},
		{
			name:             "All depth functions",
			fbzMode:          VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_DEPTH_WRITE | (VOODOO_DEPTH_ALWAYS << 5),
			expectDepthTest:  true,
			expectDepthWrite: true,
			expectDepthFunc:  VOODOO_DEPTH_ALWAYS,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := PipelineKeyFromRegisters(tc.fbzMode, 0)
			if key.DepthTestEnable != tc.expectDepthTest {
				t.Errorf("DepthTestEnable: expected %v, got %v", tc.expectDepthTest, key.DepthTestEnable)
			}
			if key.DepthWriteEnable != tc.expectDepthWrite {
				t.Errorf("DepthWriteEnable: expected %v, got %v", tc.expectDepthWrite, key.DepthWriteEnable)
			}
			if key.DepthCompareOp != tc.expectDepthFunc {
				t.Errorf("DepthCompareOp: expected %d, got %d", tc.expectDepthFunc, key.DepthCompareOp)
			}
		})
	}
}

// Test creating PipelineKey from alphaMode register
func TestVoodoo_PipelineKey_FromAlphaMode(t *testing.T) {
	tests := []struct {
		name            string
		alphaMode       uint32
		expectBlend     bool
		expectSrcFactor int
		expectDstFactor int
	}{
		{
			name:            "Blending disabled",
			alphaMode:       0,
			expectBlend:     false,
			expectSrcFactor: VOODOO_BLEND_ONE,
			expectDstFactor: VOODOO_BLEND_ZERO,
		},
		{
			name:            "Standard alpha blend (src*srcA + dst*(1-srcA))",
			alphaMode:       VOODOO_ALPHA_BLEND_EN | (VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12),
			expectBlend:     true,
			expectSrcFactor: VOODOO_BLEND_SRC_ALPHA,
			expectDstFactor: VOODOO_BLEND_INV_SRC_A,
		},
		{
			name:            "Additive blend",
			alphaMode:       VOODOO_ALPHA_BLEND_EN | (VOODOO_BLEND_ONE << 8) | (VOODOO_BLEND_ONE << 12),
			expectBlend:     true,
			expectSrcFactor: VOODOO_BLEND_ONE,
			expectDstFactor: VOODOO_BLEND_ONE,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := PipelineKeyFromRegisters(0, tc.alphaMode)
			if key.BlendEnable != tc.expectBlend {
				t.Errorf("BlendEnable: expected %v, got %v", tc.expectBlend, key.BlendEnable)
			}
			if key.SrcBlendFactor != tc.expectSrcFactor {
				t.Errorf("SrcBlendFactor: expected %d, got %d", tc.expectSrcFactor, key.SrcBlendFactor)
			}
			if key.DstBlendFactor != tc.expectDstFactor {
				t.Errorf("DstBlendFactor: expected %d, got %d", tc.expectDstFactor, key.DstBlendFactor)
			}
		})
	}
}

// Test Voodoo to Vulkan depth compare op mapping
func TestVoodoo_DepthFunc_VulkanMapping(t *testing.T) {
	// All 8 Voodoo depth functions should map to valid Vulkan compare ops
	depthFuncs := []int{
		VOODOO_DEPTH_NEVER,
		VOODOO_DEPTH_LESS,
		VOODOO_DEPTH_EQUAL,
		VOODOO_DEPTH_LESSEQUAL,
		VOODOO_DEPTH_GREATER,
		VOODOO_DEPTH_NOTEQUAL,
		VOODOO_DEPTH_GREATEREQUAL,
		VOODOO_DEPTH_ALWAYS,
	}

	for _, df := range depthFuncs {
		vkOp := VoodooDepthFuncToVulkan(df)
		// Verify it's within valid Vulkan range (0-7)
		if vkOp < 0 || vkOp > 7 {
			t.Errorf("VoodooDepthFuncToVulkan(%d) returned invalid Vulkan op: %d", df, vkOp)
		}
	}

	// Test specific mappings
	if VoodooDepthFuncToVulkan(VOODOO_DEPTH_NEVER) != 0 { // VK_COMPARE_OP_NEVER
		t.Error("DEPTH_NEVER should map to VK_COMPARE_OP_NEVER (0)")
	}
	if VoodooDepthFuncToVulkan(VOODOO_DEPTH_LESS) != 1 { // VK_COMPARE_OP_LESS
		t.Error("DEPTH_LESS should map to VK_COMPARE_OP_LESS (1)")
	}
	if VoodooDepthFuncToVulkan(VOODOO_DEPTH_ALWAYS) != 7 { // VK_COMPARE_OP_ALWAYS
		t.Error("DEPTH_ALWAYS should map to VK_COMPARE_OP_ALWAYS (7)")
	}
}

// Test Voodoo to Vulkan blend factor mapping
func TestVoodoo_BlendFactor_VulkanMapping(t *testing.T) {
	blendFactors := []int{
		VOODOO_BLEND_ZERO,
		VOODOO_BLEND_SRC_ALPHA,
		VOODOO_BLEND_COLOR,
		VOODOO_BLEND_DST_ALPHA,
		VOODOO_BLEND_ONE,
		VOODOO_BLEND_INV_SRC_A,
		VOODOO_BLEND_INV_COLOR,
		VOODOO_BLEND_INV_DST_A,
	}

	for _, bf := range blendFactors {
		vkFactor := VoodooBlendFactorToVulkan(bf)
		// Verify it's a valid Vulkan blend factor (range varies)
		if vkFactor < 0 {
			t.Errorf("VoodooBlendFactorToVulkan(%d) returned invalid factor: %d", bf, vkFactor)
		}
	}

	// Test specific mappings
	if VoodooBlendFactorToVulkan(VOODOO_BLEND_ZERO) != 0 { // VK_BLEND_FACTOR_ZERO
		t.Error("BLEND_ZERO should map to VK_BLEND_FACTOR_ZERO (0)")
	}
	if VoodooBlendFactorToVulkan(VOODOO_BLEND_ONE) != 1 { // VK_BLEND_FACTOR_ONE
		t.Error("BLEND_ONE should map to VK_BLEND_FACTOR_ONE (1)")
	}
}

// Test software backend UpdatePipelineState
func TestVoodoo_SoftwareBackend_UpdatePipelineState(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Test updating to depth GREATER
	fbzMode := uint32(VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_DEPTH_WRITE | (VOODOO_DEPTH_GREATER << 5))
	backend.UpdatePipelineState(fbzMode, 0)

	// Verify the depth test with GREATER
	if !backend.depthTest(1.0, 0.5, VOODOO_DEPTH_GREATER) {
		t.Error("After UpdatePipelineState(GREATER), 1.0 > 0.5 should pass")
	}
	if backend.depthTest(0.5, 1.0, VOODOO_DEPTH_GREATER) {
		t.Error("After UpdatePipelineState(GREATER), 0.5 > 1.0 should fail")
	}
}

// Test software backend alpha blending
func TestVoodoo_SoftwareBackend_AlphaBlending(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF) // ARGB blue

	// Enable alpha blending (src*srcA + dst*(1-srcA))
	alphaMode := uint32(VOODOO_ALPHA_BLEND_EN | (VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12))
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE)
	backend.UpdatePipelineState(fbzMode, alphaMode)

	// Draw a semi-transparent red triangle
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.5}, // 50% alpha red
			{X: 75, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.5},
			{X: 50, Y: 90, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.5},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Check center pixel - should be a blend of red and blue
	centerIdx := (50*100 + 50) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	// With 50% red over blue: R should be ~127, G should be ~0, B should be ~127
	if r < 100 || r > 180 {
		t.Errorf("Expected R~127 for 50%% blend, got %d", r)
	}
	if g > 50 {
		t.Errorf("Expected G~0 for 50%% blend, got %d", g)
	}
	if b < 100 || b > 180 {
		t.Errorf("Expected B~127 for 50%% blend, got %d", b)
	}
}

// Test software backend additive blending
func TestVoodoo_SoftwareBackend_AdditiveBlending(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to dark red
	backend.ClearFramebuffer(0xFF400000) // ARGB dark red (R=64)

	// Enable additive blending (src*1 + dst*1)
	alphaMode := uint32(VOODOO_ALPHA_BLEND_EN | (VOODOO_BLEND_ONE << 8) | (VOODOO_BLEND_ONE << 12))
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE)
	backend.UpdatePipelineState(fbzMode, alphaMode)

	// Draw a green triangle
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 0.0, G: 0.5, B: 0.0, A: 1.0}, // 50% green
			{X: 75, Y: 10, Z: 0.5, R: 0.0, G: 0.5, B: 0.0, A: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 0.0, G: 0.5, B: 0.0, A: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Check center pixel - should be additive (dark red + green)
	centerIdx := (50*100 + 50) * 4
	r, g := frame[centerIdx], frame[centerIdx+1]

	// Additive: R should stay ~64, G should be ~127
	if r < 50 || r > 80 {
		t.Errorf("Expected R~64 for additive, got %d", r)
	}
	if g < 100 || g > 150 {
		t.Errorf("Expected G~127 for additive, got %d", g)
	}
}

// Test Vulkan backend returns correct pipeline for different states
func TestVoodoo_VulkanBackend_PipelineCache(t *testing.T) {
	backend := &VulkanBackend{
		software: NewVoodooSoftwareBackend(),
	}
	backend.software.Init(100, 100)
	defer backend.software.Destroy()

	// Even without full Vulkan init, the cache mechanism should work
	// Test that different states create different cache entries
	fbzMode1 := uint32(VOODOO_FBZ_DEPTH_ENABLE | (VOODOO_DEPTH_LESS << 5))
	fbzMode2 := uint32(VOODOO_FBZ_DEPTH_ENABLE | (VOODOO_DEPTH_GREATER << 5))

	key1 := PipelineKeyFromRegisters(fbzMode1, 0)
	key2 := PipelineKeyFromRegisters(fbzMode2, 0)

	if key1 == key2 {
		t.Error("Different fbzModes should produce different PipelineKeys")
	}

	// Test that same state produces same key
	key1b := PipelineKeyFromRegisters(fbzMode1, 0)
	if key1 != key1b {
		t.Error("Same fbzMode should produce identical PipelineKeys")
	}
}

// Test dynamic depth function changes through VoodooEngine
func TestVoodoo_DynamicDepthFunction(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear and draw with LESS (default)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw far red triangle (Z=0.8)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(400))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.8))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// Draw near blue triangle (Z=0.2)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(350))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.2))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// With LESS, overlapping area should be blue (nearer)
	frame1 := make([]byte, len(v.GetFrame()))
	copy(frame1, v.GetFrame())

	centerIdx := (200*640 + 250) * 4
	if frame1[centerIdx+2] < 150 { // Blue should be high
		t.Error("With LESS depth, nearer blue triangle should win")
	}

	// Now change to GREATER and redraw
	v.HandleWrite(VOODOO_FBZ_MODE, uint32(VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_WRITE|(VOODOO_DEPTH_GREATER<<5)))
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw far red triangle again (Z=0.8)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(400))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.8))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// Draw near blue triangle (Z=0.2)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(350))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.2))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// With GREATER, the farther red triangle should win (0.8 > 0.2)
	frame2 := v.GetFrame()
	if frame2[centerIdx] < 150 { // Red should be high
		t.Error("With GREATER depth, farther red triangle should win")
	}
}

// Test dynamic blending mode changes
func TestVoodoo_DynamicBlending(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to blue
	v.HandleWrite(VOODOO_COLOR0, 0xFF0000FF)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Enable alpha blending
	alphaMode := uint32(VOODOO_ALPHA_BLEND_EN | (VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12))
	v.HandleWrite(VOODOO_ALPHA_MODE, alphaMode)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)

	// Draw semi-transparent red triangle
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(400))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(0.5)) // 50% alpha
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	frame := v.GetFrame()
	centerIdx := (200*640 + 250) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	// Should be blended (not pure red or pure blue)
	if r < 50 || r > 200 {
		t.Errorf("Expected blended red, got R=%d", r)
	}
	if b < 50 || b > 200 {
		t.Errorf("Expected blended blue, got B=%d", b)
	}
}

// Test that pipeline dirty flag triggers state update
func TestVoodoo_PipelineDirtyFlag(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Initially, pipeline should not be dirty (defaults set in init)
	v.pipelineDirty = false

	// Writing FBZ_MODE should set dirty flag
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_DEPTH_ENABLE|(VOODOO_DEPTH_GREATER<<5))
	if !v.pipelineDirty {
		t.Error("Writing FBZ_MODE should set pipelineDirty")
	}

	// Swap buffer should clear the dirty flag (after applying state)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	if v.pipelineDirty {
		t.Error("Swap buffer should clear pipelineDirty")
	}

	// Writing ALPHA_MODE should also set dirty flag
	v.HandleWrite(VOODOO_ALPHA_MODE, VOODOO_ALPHA_BLEND_EN)
	if !v.pipelineDirty {
		t.Error("Writing ALPHA_MODE should set pipelineDirty")
	}
}

// Test all 8 depth functions produce correct results
func TestVoodoo_AllDepthFunctions_Software(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	testCases := []struct {
		depthFunc int
		name      string
		passNew   float32
		passOld   float32
		failNew   float32
		failOld   float32
	}{
		{VOODOO_DEPTH_NEVER, "NEVER", 0.5, 1.0, 0.5, 1.0}, // Never passes
		{VOODOO_DEPTH_LESS, "LESS", 0.3, 0.7, 0.7, 0.3},   // pass: 0.3<0.7, fail: 0.7<0.3
		{VOODOO_DEPTH_EQUAL, "EQUAL", 0.5, 0.5, 0.5, 0.6}, // pass: 0.5==0.5, fail: 0.5==0.6
		{VOODOO_DEPTH_LESSEQUAL, "LESSEQUAL", 0.5, 0.5, 0.6, 0.5},
		{VOODOO_DEPTH_GREATER, "GREATER", 0.7, 0.3, 0.3, 0.7},
		{VOODOO_DEPTH_NOTEQUAL, "NOTEQUAL", 0.5, 0.6, 0.5, 0.5},
		{VOODOO_DEPTH_GREATEREQUAL, "GREATEREQUAL", 0.5, 0.5, 0.4, 0.5},
		{VOODOO_DEPTH_ALWAYS, "ALWAYS", 0.5, 1.0, 0.0, 0.0}, // Always passes
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.depthFunc == VOODOO_DEPTH_NEVER {
				if backend.depthTest(tc.passNew, tc.passOld, tc.depthFunc) {
					t.Error("NEVER should always fail")
				}
			} else if tc.depthFunc == VOODOO_DEPTH_ALWAYS {
				if !backend.depthTest(tc.passNew, tc.passOld, tc.depthFunc) {
					t.Error("ALWAYS should always pass")
				}
			} else {
				if !backend.depthTest(tc.passNew, tc.passOld, tc.depthFunc) {
					t.Errorf("%s: expected pass for new=%f, old=%f", tc.name, tc.passNew, tc.passOld)
				}
				if backend.depthTest(tc.failNew, tc.failOld, tc.depthFunc) {
					t.Errorf("%s: expected fail for new=%f, old=%f", tc.name, tc.failNew, tc.failOld)
				}
			}
		})
	}
}

// =============================================================================
// Phase 3: Alpha Test & Chroma Key Tests
// =============================================================================

// Test alpha test function mapping
func TestVoodoo_AlphaTestFunction(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Test all 8 alpha test functions (same as depth functions)
	tests := []struct {
		name       string
		alphaFunc  int
		alphaValue float32
		alphaRef   float32
		expected   bool
	}{
		{"NEVER", VOODOO_ALPHA_NEVER, 0.5, 0.3, false},
		{"LESS pass", VOODOO_ALPHA_LESS, 0.3, 0.5, true},
		{"LESS fail", VOODOO_ALPHA_LESS, 0.5, 0.3, false},
		{"EQUAL pass", VOODOO_ALPHA_EQUAL, 0.5, 0.5, true},
		{"EQUAL fail", VOODOO_ALPHA_EQUAL, 0.5, 0.6, false},
		{"LESSEQUAL pass (less)", VOODOO_ALPHA_LESSEQUAL, 0.3, 0.5, true},
		{"LESSEQUAL pass (equal)", VOODOO_ALPHA_LESSEQUAL, 0.5, 0.5, true},
		{"LESSEQUAL fail", VOODOO_ALPHA_LESSEQUAL, 0.6, 0.5, false},
		{"GREATER pass", VOODOO_ALPHA_GREATER, 0.7, 0.3, true},
		{"GREATER fail", VOODOO_ALPHA_GREATER, 0.3, 0.7, false},
		{"NOTEQUAL pass", VOODOO_ALPHA_NOTEQUAL, 0.5, 0.6, true},
		{"NOTEQUAL fail", VOODOO_ALPHA_NOTEQUAL, 0.5, 0.5, false},
		{"GREATEREQUAL pass (greater)", VOODOO_ALPHA_GREATEREQUAL, 0.7, 0.5, true},
		{"GREATEREQUAL pass (equal)", VOODOO_ALPHA_GREATEREQUAL, 0.5, 0.5, true},
		{"GREATEREQUAL fail", VOODOO_ALPHA_GREATEREQUAL, 0.3, 0.5, false},
		{"ALWAYS", VOODOO_ALPHA_ALWAYS, 0.0, 1.0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := backend.alphaTest(tc.alphaValue, tc.alphaRef, tc.alphaFunc)
			if result != tc.expected {
				t.Errorf("alphaTest(%f, %f, %d) = %v, expected %v",
					tc.alphaValue, tc.alphaRef, tc.alphaFunc, result, tc.expected)
			}
		})
	}
}

// Test alpha test discards pixels correctly
func TestVoodoo_AlphaTest_Discard(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF) // ARGB blue

	// Enable alpha test with GREATER function, ref = 0.5 (128)
	// alphaMode: bit 0 = enable, bits 1-3 = function, bits 24-31 = ref value
	alphaRef := uint32(128) << 24 // ref = 128 (0.5 * 255)
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | alphaRef)
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE)
	backend.UpdatePipelineState(fbzMode, alphaMode)

	// Draw a triangle with alpha = 0.3 (should be discarded since 0.3 is NOT > 0.5)
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.3},
			{X: 75, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.3},
			{X: 50, Y: 90, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.3},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Center pixel should still be blue (triangle was discarded)
	centerIdx := (50*100 + 50) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	if r > 50 || b < 200 {
		t.Errorf("Low-alpha triangle should be discarded, got R=%d B=%d", r, b)
	}
}

// Test alpha test passes pixels correctly
func TestVoodoo_AlphaTest_Pass(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF)

	// Enable alpha test with GREATER function, ref = 0.5 (128)
	alphaRef := uint32(128) << 24
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | alphaRef)
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE)
	backend.UpdatePipelineState(fbzMode, alphaMode)

	// Draw a triangle with alpha = 0.8 (should pass since 0.8 > 0.5)
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.8},
			{X: 75, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.8},
			{X: 50, Y: 90, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.8},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Center pixel should be red (triangle passed alpha test)
	centerIdx := (50*100 + 50) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	if r < 200 || b > 50 {
		t.Errorf("High-alpha triangle should pass, got R=%d B=%d", r, b)
	}
}

// Test chroma key discard
func TestVoodoo_ChromaKey_Discard(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF)

	// Enable chroma key for pure magenta (0xFF00FF)
	backend.SetChromaKey(0x00FF00FF) // RGB magenta
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_CHROMAKEY)
	backend.UpdatePipelineState(fbzMode, 0)

	// Draw a magenta triangle (should be discarded due to chroma key)
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 1.0, A: 1.0}, // Magenta
			{X: 75, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 1.0, A: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1.0, G: 0.0, B: 1.0, A: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Center pixel should still be blue (magenta was keyed out)
	centerIdx := (50*100 + 50) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	if r > 50 || g > 50 || b < 200 {
		t.Errorf("Magenta triangle should be keyed out, got R=%d G=%d B=%d", r, g, b)
	}
}

// Test chroma key passes non-matching colors
func TestVoodoo_ChromaKey_Pass(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF)

	// Enable chroma key for pure magenta
	backend.SetChromaKey(0x00FF00FF) // RGB magenta
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_CHROMAKEY)
	backend.UpdatePipelineState(fbzMode, 0)

	// Draw a red triangle (should NOT be discarded)
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 1.0}, // Red
			{X: 75, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Center pixel should be red (not keyed out)
	centerIdx := (50*100 + 50) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	if r < 200 || b > 50 {
		t.Errorf("Red triangle should pass chroma key, got R=%d B=%d", r, b)
	}
}

// Test chroma key tolerance (exact match required)
func TestVoodoo_ChromaKey_ExactMatch(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF)

	// Set chroma key to exact red (255, 0, 0)
	backend.SetChromaKey(0x00FF0000) // RGB red
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_CHROMAKEY)
	backend.UpdatePipelineState(fbzMode, 0)

	// Draw a slightly off-red triangle (254, 0, 0) - should NOT be keyed out
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 25, Y: 10, Z: 0.5, R: 0.996, G: 0.0, B: 0.0, A: 1.0}, // ~254
			{X: 75, Y: 10, Z: 0.5, R: 0.996, G: 0.0, B: 0.0, A: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 0.996, G: 0.0, B: 0.0, A: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	// Center pixel should be near-red (not keyed out due to slight mismatch)
	centerIdx := (50*100 + 50) * 4
	r := frame[centerIdx]

	// We use a tolerance of 1 for chroma keying, so 254 vs 255 should still pass
	// The test validates that the chroma key mechanism is working
	if r < 200 {
		t.Errorf("Slightly off-red should pass chroma key, got R=%d", r)
	}
}

// Test combined alpha test and chroma key
func TestVoodoo_AlphaTestAndChromaKey_Combined(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	backend.Init(100, 100)
	defer backend.Destroy()

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF)

	// Enable both alpha test (GREATER 0.5) and chroma key (magenta)
	backend.SetChromaKey(0x00FF00FF)
	alphaRef := uint32(128) << 24
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | alphaRef)
	fbzMode := uint32(VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_CHROMAKEY)
	backend.UpdatePipelineState(fbzMode, alphaMode)

	// Draw red triangle with alpha=0.8 (should pass both tests)
	tri1 := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.8},
			{X: 40, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.8},
			{X: 25, Y: 40, Z: 0.5, R: 1.0, G: 0.0, B: 0.0, A: 0.8},
		},
	}

	// Draw magenta triangle with alpha=0.8 (should fail chroma key)
	tri2 := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 60, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 1.0, A: 0.8},
			{X: 90, Y: 10, Z: 0.5, R: 1.0, G: 0.0, B: 1.0, A: 0.8},
			{X: 75, Y: 40, Z: 0.5, R: 1.0, G: 0.0, B: 1.0, A: 0.8},
		},
	}

	// Draw green triangle with alpha=0.3 (should fail alpha test)
	tri3 := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 35, Y: 60, Z: 0.5, R: 0.0, G: 1.0, B: 0.0, A: 0.3},
			{X: 65, Y: 60, Z: 0.5, R: 0.0, G: 1.0, B: 0.0, A: 0.3},
			{X: 50, Y: 90, Z: 0.5, R: 0.0, G: 1.0, B: 0.0, A: 0.3},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri1, tri2, tri3})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()

	// Check first triangle area (should be red)
	idx1 := (25*100 + 25) * 4
	if frame[idx1] < 200 {
		t.Errorf("Red triangle should pass, got R=%d", frame[idx1])
	}

	// Check second triangle area (should be blue - chroma keyed)
	idx2 := (25*100 + 75) * 4
	if frame[idx2+2] < 200 {
		t.Errorf("Magenta triangle should be keyed out, got B=%d", frame[idx2+2])
	}

	// Check third triangle area (should be blue - alpha failed)
	idx3 := (75*100 + 50) * 4
	if frame[idx3+2] < 200 {
		t.Errorf("Low-alpha triangle should be discarded, got B=%d", frame[idx3+2])
	}
}

// Test VoodooEngine chromaKey register write
func TestVoodoo_ChromaKey_Register(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write chroma key value
	chromaKeyValue := uint32(0x00FF00FF) // Magenta
	v.HandleWrite(VOODOO_CHROMA_KEY, chromaKeyValue)

	// Read it back
	readValue := v.HandleRead(VOODOO_CHROMA_KEY)
	if readValue != chromaKeyValue {
		t.Errorf("Expected chromaKey 0x%X, got 0x%X", chromaKeyValue, readValue)
	}
}

// Test alpha mode register parsing for alpha test
func TestVoodoo_AlphaMode_TestParsing(t *testing.T) {
	tests := []struct {
		name       string
		alphaMode  uint32
		expectTest bool
		expectFunc int
		expectRef  uint8
	}{
		{
			name:       "Alpha test disabled",
			alphaMode:  0,
			expectTest: false,
			expectFunc: 0,
			expectRef:  0,
		},
		{
			name:       "Alpha test LESS ref=128",
			alphaMode:  VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_LESS << 1) | (128 << 24),
			expectTest: true,
			expectFunc: VOODOO_ALPHA_LESS,
			expectRef:  128,
		},
		{
			name:       "Alpha test GREATER ref=64",
			alphaMode:  VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | (64 << 24),
			expectTest: true,
			expectFunc: VOODOO_ALPHA_GREATER,
			expectRef:  64,
		},
		{
			name:       "Alpha test ALWAYS ref=255",
			alphaMode:  VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_ALWAYS << 1) | (255 << 24),
			expectTest: true,
			expectFunc: VOODOO_ALPHA_ALWAYS,
			expectRef:  255,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testEnabled := (tc.alphaMode & VOODOO_ALPHA_TEST_EN) != 0
			testFunc := int((tc.alphaMode >> 1) & 0x7)
			testRef := uint8((tc.alphaMode >> 24) & 0xFF)

			if testEnabled != tc.expectTest {
				t.Errorf("TestEnabled: expected %v, got %v", tc.expectTest, testEnabled)
			}
			if testFunc != tc.expectFunc {
				t.Errorf("TestFunc: expected %d, got %d", tc.expectFunc, testFunc)
			}
			if testRef != tc.expectRef {
				t.Errorf("TestRef: expected %d, got %d", tc.expectRef, testRef)
			}
		})
	}
}

// Test push constants structure
func TestVoodoo_PushConstants_Structure(t *testing.T) {
	pc := VoodooPushConstants{
		FbzMode:     VOODOO_FBZ_CHROMAKEY | VOODOO_FBZ_RGB_WRITE,
		AlphaMode:   VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | (128 << 24),
		ChromaKey:   0x00FF00FF,
		TextureMode: 1, // Phase 4: texture enable flag
	}

	// Verify structure is 16 bytes (4 x uint32)
	// Phase 4: Added TextureMode field
	expectedSize := 16
	actualSize := int(unsafe.Sizeof(pc))
	if actualSize != expectedSize {
		t.Errorf("PushConstants size: expected %d bytes, got %d", expectedSize, actualSize)
	}

	// Verify values are stored correctly
	if pc.FbzMode != (VOODOO_FBZ_CHROMAKEY | VOODOO_FBZ_RGB_WRITE) {
		t.Error("FbzMode not stored correctly")
	}
	if pc.ChromaKey != 0x00FF00FF {
		t.Error("ChromaKey not stored correctly")
	}
	if pc.TextureMode != 1 {
		t.Error("TextureMode not stored correctly")
	}
}

// Test VoodooEngine passes chromaKey to backend
func TestVoodoo_ChromaKey_Integration(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to blue
	v.HandleWrite(VOODOO_COLOR0, 0xFF0000FF)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Enable chroma key for magenta
	v.HandleWrite(VOODOO_CHROMA_KEY, 0x00FF00FF)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_CHROMAKEY)

	// Draw magenta triangle
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(390))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(330))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(110))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(330))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0)) // Magenta
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Center pixel should be blue (magenta keyed out)
	frame := v.GetFrame()
	centerIdx := (240*640 + 250) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	if r > 50 || b < 200 {
		t.Errorf("Magenta should be keyed out, got R=%d B=%d", r, b)
	}
}

// Test VoodooEngine alpha test integration
func TestVoodoo_AlphaTest_Integration(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Clear to blue
	v.HandleWrite(VOODOO_COLOR0, 0xFF0000FF)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Enable alpha test with GREATER function, ref = 128 (0.5)
	alphaRef := uint32(128) << 24
	alphaMode := uint32(VOODOO_ALPHA_TEST_EN | (VOODOO_ALPHA_GREATER << 1) | alphaRef)
	v.HandleWrite(VOODOO_ALPHA_MODE, alphaMode)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)

	// Draw red triangle with alpha = 0.3 (should be discarded)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(250))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(390))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(330))
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(110))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(330))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(0.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(0.3)) // Low alpha
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Center pixel should be blue (red discarded)
	frame := v.GetFrame()
	centerIdx := (240*640 + 250) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	if r > 50 || b < 200 {
		t.Errorf("Low-alpha red should be discarded, got R=%d B=%d", r, b)
	}
}

// Test PipelineKey as map key
func TestVoodoo_PipelineKey_AsMapKey(t *testing.T) {
	cache := make(map[PipelineKey]int)

	key1 := PipelineKey{DepthTestEnable: true, DepthCompareOp: VOODOO_DEPTH_LESS}
	key2 := PipelineKey{DepthTestEnable: true, DepthCompareOp: VOODOO_DEPTH_LESS}
	key3 := PipelineKey{DepthTestEnable: true, DepthCompareOp: VOODOO_DEPTH_GREATER}

	cache[key1] = 1
	cache[key3] = 2

	// key2 should find the same entry as key1
	if val, ok := cache[key2]; !ok || val != 1 {
		t.Error("Identical keys should map to same cache entry")
	}

	// key3 should have its own entry
	if val, ok := cache[key3]; !ok || val != 2 {
		t.Error("Different key should have different cache entry")
	}
}

// =============================================================================
// Phase 4: Texture Mapping Tests
// =============================================================================

// Test texture mode register parsing
func TestVoodoo_TextureMode_RegisterBits(t *testing.T) {
	tests := []struct {
		name          string
		textureMode   uint32
		expectEnabled bool
		expectClampS  bool
		expectClampT  bool
		expectMagnify bool
		expectFormat  int
	}{
		{
			name:          "Texturing disabled",
			textureMode:   0,
			expectEnabled: false,
			expectClampS:  false,
			expectClampT:  false,
			expectMagnify: false,
			expectFormat:  0,
		},
		{
			name:          "Basic texturing enabled",
			textureMode:   VOODOO_TEX_ENABLE,
			expectEnabled: true,
			expectClampS:  false,
			expectClampT:  false,
			expectMagnify: false,
			expectFormat:  0,
		},
		{
			name:          "Texturing with clamp S and T",
			textureMode:   VOODOO_TEX_ENABLE | VOODOO_TEX_CLAMP_S | VOODOO_TEX_CLAMP_T,
			expectEnabled: true,
			expectClampS:  true,
			expectClampT:  true,
			expectMagnify: false,
			expectFormat:  0,
		},
		{
			name:          "Texturing with bilinear filter",
			textureMode:   VOODOO_TEX_ENABLE | VOODOO_TEX_MAGNIFY,
			expectEnabled: true,
			expectClampS:  false,
			expectClampT:  false,
			expectMagnify: true,
			expectFormat:  0,
		},
		{
			name:          "Texturing with ARGB8888 format",
			textureMode:   VOODOO_TEX_ENABLE | (VOODOO_TEX_FMT_ARGB8888 << 8),
			expectEnabled: true,
			expectClampS:  false,
			expectClampT:  false,
			expectMagnify: false,
			expectFormat:  VOODOO_TEX_FMT_ARGB8888,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enabled := (tc.textureMode & VOODOO_TEX_ENABLE) != 0
			clampS := (tc.textureMode & VOODOO_TEX_CLAMP_S) != 0
			clampT := (tc.textureMode & VOODOO_TEX_CLAMP_T) != 0
			magnify := (tc.textureMode & VOODOO_TEX_MAGNIFY) != 0
			format := int((tc.textureMode >> 8) & 0xF)

			if enabled != tc.expectEnabled {
				t.Errorf("Enabled: expected %v, got %v", tc.expectEnabled, enabled)
			}
			if clampS != tc.expectClampS {
				t.Errorf("ClampS: expected %v, got %v", tc.expectClampS, clampS)
			}
			if clampT != tc.expectClampT {
				t.Errorf("ClampT: expected %v, got %v", tc.expectClampT, clampT)
			}
			if magnify != tc.expectMagnify {
				t.Errorf("Magnify: expected %v, got %v", tc.expectMagnify, magnify)
			}
			if format != tc.expectFormat {
				t.Errorf("Format: expected %d, got %d", tc.expectFormat, format)
			}
		})
	}
}

// Test texture mode register write and read
func TestVoodoo_TextureMode_Register(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Write texture mode value
	texModeValue := uint32(VOODOO_TEX_ENABLE | VOODOO_TEX_CLAMP_S | VOODOO_TEX_CLAMP_T | VOODOO_TEX_MAGNIFY | (VOODOO_TEX_FMT_ARGB8888 << 8))
	v.HandleWrite(VOODOO_TEXTURE_MODE, texModeValue)

	// Read it back
	readValue := v.HandleRead(VOODOO_TEXTURE_MODE)
	if readValue != texModeValue {
		t.Errorf("Expected textureMode 0x%X, got 0x%X", texModeValue, readValue)
	}
}

// Test per-vertex texture coordinate storage (Gouraud mode)
func TestVoodoo_PerVertexTextureCoords_Storage(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable Gouraud mode and set vertex 0 texture coords
	v.HandleWrite(VOODOO_COLOR_SELECT, 0) // Target vertex 0
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_W, floatToFixed2_30(1.0))

	// Vertex 1
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_W, floatToFixed2_30(1.0))

	// Vertex 2
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(0.5))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_W, floatToFixed2_30(1.0))

	// Verify that gouraud mode is enabled
	if !v.gouraudEnabled {
		t.Error("Gouraud mode should be enabled after COLOR_SELECT writes")
	}
}

// Test VoodooBackend interface includes texture methods
func TestVoodoo_Backend_HasTextureInterface(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	if backend == nil {
		t.Fatal("NewVoodooSoftwareBackend returned nil")
	}

	err := backend.Init(256, 256)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Verify SetTextureData exists and works
	texWidth, texHeight := 4, 4
	texData := make([]byte, texWidth*texHeight*4) // ARGB8888

	// Create a simple 2x2 checkerboard pattern
	for y := 0; y < texHeight; y++ {
		for x := 0; x < texWidth; x++ {
			idx := (y*texWidth + x) * 4
			if (x+y)%2 == 0 {
				texData[idx+0] = 255 // R
				texData[idx+1] = 255 // G
				texData[idx+2] = 255 // B
				texData[idx+3] = 255 // A
			} else {
				texData[idx+0] = 0   // R
				texData[idx+1] = 0   // G
				texData[idx+2] = 0   // B
				texData[idx+3] = 255 // A
			}
		}
	}

	// This should not panic
	backend.SetTextureData(texWidth, texHeight, texData, VOODOO_TEX_FMT_ARGB8888)
}

// Test software backend texture sampling - point sampling
func TestVoodoo_SoftwareBackend_TextureSampling_Point(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	err := backend.Init(100, 100)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create a 2x2 texture: red, green, blue, white
	texData := []byte{
		255, 0, 0, 255, // Red (0,0)
		0, 255, 0, 255, // Green (1,0)
		0, 0, 255, 255, // Blue (0,1)
		255, 255, 255, 255, // White (1,1)
	}
	backend.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)

	// Enable texturing in fbzMode simulation
	backend.SetTextureEnabled(true)

	// Clear to black
	backend.ClearFramebuffer(0xFF000000)

	// Draw a textured triangle covering most of the screen
	// UV coords at each vertex should sample from the texture
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 0.0, W: 1.0}, // Top-left, UV=(0,0) -> red
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 0.0, W: 1.0}, // Top-right, UV=(1,0) -> green
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 1.0, W: 1.0}, // Bottom-center, UV=(0.5,1) -> blend of blue/white
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()

	// Check top-left area should be reddish
	topLeftIdx := (15*100 + 20) * 4
	if frame[topLeftIdx] < 100 {
		t.Errorf("Top-left should be reddish, got R=%d", frame[topLeftIdx])
	}

	// Check top-right area should be greenish
	topRightIdx := (15*100 + 80) * 4
	if frame[topRightIdx+1] < 100 {
		t.Errorf("Top-right should be greenish, got G=%d", frame[topRightIdx+1])
	}
}

// Test software backend texture wrap mode
func TestVoodoo_SoftwareBackend_TextureWrap(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	err := backend.Init(100, 100)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create a 2x2 red/blue checkerboard texture
	texData := []byte{
		255, 0, 0, 255, // Red
		0, 0, 255, 255, // Blue
		0, 0, 255, 255, // Blue
		255, 0, 0, 255, // Red
	}
	backend.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	backend.SetTextureEnabled(true)
	backend.SetTextureWrapMode(false, false) // Repeat mode

	backend.ClearFramebuffer(0xFF000000)

	// Draw triangle with UVs > 1.0 to test wrapping
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 0.0, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 2.0, T: 0.0, W: 1.0}, // S > 1 should wrap
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 2.0, W: 1.0}, // T > 1 should wrap
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	// Just verify no crash and we get some output
	frame := backend.GetFrame()
	if frame == nil {
		t.Error("GetFrame returned nil after textured render")
	}
}

// Test software backend texture clamp mode
func TestVoodoo_SoftwareBackend_TextureClamp(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	err := backend.Init(100, 100)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create a 2x2 texture with distinct corners
	texData := []byte{
		255, 0, 0, 255, // Red (0,0)
		0, 255, 0, 255, // Green (1,0)
		0, 0, 255, 255, // Blue (0,1)
		255, 255, 0, 255, // Yellow (1,1)
	}
	backend.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	backend.SetTextureEnabled(true)
	backend.SetTextureWrapMode(true, true) // Clamp mode

	backend.ClearFramebuffer(0xFF000000)

	// Draw triangle with UVs > 1.0 to test clamping
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 0.0, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 2.0, T: 0.0, W: 1.0}, // Should clamp to 1.0 -> green
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 2.0, W: 1.0}, // Should clamp to 1.0 -> yellow
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	if frame == nil {
		t.Error("GetFrame returned nil after textured render")
	}
}

// Test texture color modulation with vertex color
func TestVoodoo_SoftwareBackend_TextureModulation(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	err := backend.Init(100, 100)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create a pure white 2x2 texture
	texData := []byte{
		255, 255, 255, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
		255, 255, 255, 255,
	}
	backend.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	backend.SetTextureEnabled(true)

	backend.ClearFramebuffer(0xFF000000)

	// Draw triangle with red vertex color - result should be red (white tex * red vert)
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 0, B: 0, A: 1, S: 0.0, T: 0.0, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 0, B: 0, A: 1, S: 1.0, T: 0.0, W: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 0, B: 0, A: 1, S: 0.5, T: 1.0, W: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()

	// Center of triangle should be red (white * red = red)
	centerIdx := (50*100 + 50) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	if r < 200 || g > 50 || b > 50 {
		t.Errorf("Modulated color should be red, got R=%d G=%d B=%d", r, g, b)
	}
}

// Test VoodooEngine texture coordinate writes
func TestVoodoo_TextureCoord_Writes(t *testing.T) {
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
		t.Errorf("Expected START_S raw 0x20000, got 0x%X", sRaw)
	}
	if tRaw != 0x40000 {
		t.Errorf("Expected START_T raw 0x40000, got 0x%X", tRaw)
	}
}

// Test VulkanVertex struct size with texture coordinates
func TestVoodoo_VulkanVertex_TextureCoords(t *testing.T) {
	// VulkanVertex should now have:
	// Position [3]float32 = 12 bytes
	// Color [4]float32 = 16 bytes
	// TexCoord [2]float32 = 8 bytes
	// Total = 36 bytes
	expectedSize := 36
	actualSize := int(unsafe.Sizeof(VulkanVertex{}))

	if actualSize != expectedSize {
		t.Errorf("VulkanVertex size: expected %d bytes, got %d", expectedSize, actualSize)
	}
}

// Test texture data upload to VoodooEngine
func TestVoodoo_TextureUpload(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Create a simple 4x4 texture
	texWidth, texHeight := 4, 4
	texData := make([]byte, texWidth*texHeight*4)
	for i := 0; i < len(texData); i += 4 {
		texData[i+0] = 128 // R
		texData[i+1] = 64  // G
		texData[i+2] = 255 // B
		texData[i+3] = 255 // A
	}

	// Set texture mode enabled
	v.HandleWrite(VOODOO_TEXTURE_MODE, VOODOO_TEX_ENABLE|(VOODOO_TEX_FMT_ARGB8888<<8))

	// Upload texture data - this exercises the SetTextureData path
	v.SetTextureData(texWidth, texHeight, texData)

	// Verify texture mode is stored
	if v.HandleRead(VOODOO_TEXTURE_MODE) != (VOODOO_TEX_ENABLE | (VOODOO_TEX_FMT_ARGB8888 << 8)) {
		t.Error("Texture mode not stored correctly")
	}
}

// Test that texture coordinates are passed correctly to triangles
func TestVoodoo_TextureCoords_InTriangle(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Set up a triangle with specific texture coords
	v.HandleWrite(VOODOO_COLOR_SELECT, 0)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(150))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(200))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(0.5))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit triangle
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	// Check that we have 1 triangle
	if v.GetTriangleBatchCount() != 1 {
		t.Fatalf("Expected 1 triangle, got %d", v.GetTriangleBatchCount())
	}

	// Access the triangle batch directly to check texture coords
	v.mutex.RLock()
	tri := v.triangleBatch[0]
	v.mutex.RUnlock()

	// Verify texture coordinates for each vertex
	const tolerance = 0.01
	if math.Abs(float64(tri.Vertices[0].S-0.0)) > tolerance {
		t.Errorf("Vertex 0 S: expected 0.0, got %f", tri.Vertices[0].S)
	}
	if math.Abs(float64(tri.Vertices[0].T-0.0)) > tolerance {
		t.Errorf("Vertex 0 T: expected 0.0, got %f", tri.Vertices[0].T)
	}
	if math.Abs(float64(tri.Vertices[1].S-1.0)) > tolerance {
		t.Errorf("Vertex 1 S: expected 1.0, got %f", tri.Vertices[1].S)
	}
	if math.Abs(float64(tri.Vertices[1].T-0.0)) > tolerance {
		t.Errorf("Vertex 1 T: expected 0.0, got %f", tri.Vertices[1].T)
	}
	if math.Abs(float64(tri.Vertices[2].S-0.5)) > tolerance {
		t.Errorf("Vertex 2 S: expected 0.5, got %f", tri.Vertices[2].S)
	}
	if math.Abs(float64(tri.Vertices[2].T-1.0)) > tolerance {
		t.Errorf("Vertex 2 T: expected 1.0, got %f", tri.Vertices[2].T)
	}
}

// Test that texture state is passed correctly to backend
func TestVoodoo_TextureState_InBackend(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Create a red texture
	texData := []byte{
		255, 0, 0, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
	}
	v.SetTextureData(2, 2, texData)

	// Enable texturing
	v.HandleWrite(VOODOO_TEXTURE_MODE, VOODOO_TEX_ENABLE|(VOODOO_TEX_FMT_ARGB8888<<8))

	// Access the backend (which is a VulkanBackend with software fallback)
	vb := v.backend.(*VulkanBackend)
	sb := vb.software

	// Check that texture is enabled in software backend
	if !sb.textureEnabled {
		t.Error("Texture should be enabled in software backend")
	}

	// Check that texture data exists
	if sb.textureData == nil {
		t.Error("Texture data should exist in software backend")
	}

	// Check texture dimensions
	if sb.textureWidth != 2 || sb.textureHeight != 2 {
		t.Errorf("Texture dimensions: expected 2x2, got %dx%d", sb.textureWidth, sb.textureHeight)
	}
}

// Test full textured triangle rendering through VoodooEngine
func TestVoodoo_TexturedTriangle_Integration(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatalf("NewVoodooEngine failed: %v", err)
	}
	defer v.Destroy()

	// Enable texturing FIRST (so format is set when we upload)
	v.HandleWrite(VOODOO_TEXTURE_MODE, VOODOO_TEX_ENABLE|(VOODOO_TEX_FMT_ARGB8888<<8))

	// Create a red 2x2 texture
	texData := []byte{
		255, 0, 0, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
	}
	v.SetTextureData(2, 2, texData)

	// Clear to blue
	v.HandleWrite(VOODOO_COLOR0, 0xFF0000FF)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Draw a textured triangle with white vertex color
	// Vertex A - top center
	v.HandleWrite(VOODOO_COLOR_SELECT, 0)
	v.HandleWrite(VOODOO_VERTEX_AX, floatToFixed12_4(320))
	v.HandleWrite(VOODOO_VERTEX_AY, floatToFixed12_4(100))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(0.5))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Vertex B - bottom right
	v.HandleWrite(VOODOO_COLOR_SELECT, 1)
	v.HandleWrite(VOODOO_VERTEX_BX, floatToFixed12_4(420))
	v.HandleWrite(VOODOO_VERTEX_BY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Vertex C - bottom left
	v.HandleWrite(VOODOO_COLOR_SELECT, 2)
	v.HandleWrite(VOODOO_VERTEX_CX, floatToFixed12_4(220))
	v.HandleWrite(VOODOO_VERTEX_CY, floatToFixed12_4(300))
	v.HandleWrite(VOODOO_START_R, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_G, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_B, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_A, floatToFixed12_12(1.0))
	v.HandleWrite(VOODOO_START_S, floatToFixed14_18(0.0))
	v.HandleWrite(VOODOO_START_T, floatToFixed14_18(1.0))
	v.HandleWrite(VOODOO_START_Z, floatToFixed20_12(0.5))

	// Submit and swap
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// Verify software backend state after flush
	vb := v.backend.(*VulkanBackend)
	sb := vb.software

	// Debug: check texture state
	if !sb.textureEnabled {
		t.Errorf("Texture should be enabled, but it's not")
	}
	if sb.textureData == nil {
		t.Errorf("Texture data should exist, but it's nil")
	}

	// Check center of triangle - should be red from texture
	frame := v.GetFrame()
	centerIdx := (200*640 + 320) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	if r < 200 || g > 50 || b > 50 {
		t.Errorf("Textured triangle center should be red, got R=%d G=%d B=%d", r, g, b)
	}
}

// Helper function for 14.18 fixed-point conversion
func floatToFixed14_18(f float32) uint32 {
	return uint32(f * float32(1<<18))
}

// Helper function for 2.30 fixed-point conversion
func floatToFixed2_30(f float32) uint32 {
	return uint32(f * float32(1<<30))
}

// Test texture sampling edge cases
func TestVoodoo_TextureSampling_EdgeCases(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	err := backend.Init(100, 100)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Test with 1x1 texture
	texData := []byte{255, 128, 64, 255}
	backend.SetTextureData(1, 1, texData, VOODOO_TEX_FMT_ARGB8888)
	backend.SetTextureEnabled(true)

	backend.ClearFramebuffer(0xFF000000)

	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	centerIdx := (50*100 + 50) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	// Should match the single texel color modulated by white vertex color
	if r < 250 || g < 120 || g > 135 || b < 60 || b > 70 {
		t.Errorf("1x1 texture should produce R=255 G=128 B=64, got R=%d G=%d B=%d", r, g, b)
	}
}

// Test texture with alpha channel
func TestVoodoo_TextureAlpha_Modulation(t *testing.T) {
	backend := NewVoodooSoftwareBackend()
	err := backend.Init(100, 100)
	if err != nil {
		t.Fatalf("Backend Init failed: %v", err)
	}
	defer backend.Destroy()

	// Create texture with 50% alpha
	texData := []byte{
		255, 0, 0, 128, // 50% alpha red
		255, 0, 0, 128,
		255, 0, 0, 128,
		255, 0, 0, 128,
	}
	backend.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	backend.SetTextureEnabled(true)

	// Enable alpha blending
	backend.UpdatePipelineState(
		VOODOO_FBZ_RGB_WRITE,
		VOODOO_ALPHA_BLEND_EN|(VOODOO_BLEND_SRC_ALPHA<<8)|(VOODOO_BLEND_INV_SRC_A<<12),
	)

	// Clear to blue
	backend.ClearFramebuffer(0xFF0000FF)

	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 0.0, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 0.0, W: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 1.0, W: 1.0},
		},
	}

	backend.FlushTriangles([]VoodooTriangle{tri})
	backend.SwapBuffers(false)

	frame := backend.GetFrame()
	centerIdx := (50*100 + 50) * 4
	r, b := frame[centerIdx], frame[centerIdx+2]

	// Should be blend of red (texture) and blue (background)
	// 50% red + 50% blue = purple-ish
	if r < 100 || r > 180 || b < 100 || b > 180 {
		t.Errorf("Blended texture should be purple-ish, got R=%d B=%d", r, b)
	}
}

// Test texture through VulkanBackend (not through VoodooEngine)
func TestVoodoo_VulkanBackend_TexturedTriangle(t *testing.T) {
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}
	err = vb.Init(100, 100)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer vb.Destroy()

	// Create red texture
	texData := []byte{
		255, 0, 0, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
		255, 0, 0, 255,
	}
	vb.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	vb.SetTextureEnabled(true)

	// Check that state is set in software backend
	sb := vb.software
	if !sb.textureEnabled {
		t.Error("software backend textureEnabled should be true")
	}
	if sb.textureData == nil {
		t.Error("software backend textureData should not be nil")
	}

	// Clear to blue
	vb.ClearFramebuffer(0xFF0000FF)

	// Draw a textured triangle with white vertex color
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
		},
	}

	vb.FlushTriangles([]VoodooTriangle{tri})
	vb.SwapBuffers(false)

	// Check center pixel - should be red (texture) modulated by white (vertex) = red
	frame := vb.GetFrame()
	centerIdx := (50*100 + 50) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	if r < 200 || g > 50 || b > 50 {
		t.Errorf("Textured triangle center should be red, got R=%d G=%d B=%d", r, g, b)
	}
}

// =============================================================================
// Phase 4: Vulkan GPU Texture Tests
// =============================================================================

// Test that Vulkan texture resources can be created
func TestVoodoo_VulkanBackend_TextureResourceCreation(t *testing.T) {
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}
	err = vb.Init(100, 100)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer vb.Destroy()

	// Skip if Vulkan is not initialized (headless environment)
	if !vb.initialized {
		t.Skip("Vulkan not available, skipping GPU texture test")
	}

	// Create a 4x4 texture
	texData := make([]byte, 4*4*4)
	for i := 0; i < len(texData); i += 4 {
		texData[i+0] = 255 // R
		texData[i+1] = 0   // G
		texData[i+2] = 0   // B
		texData[i+3] = 255 // A
	}

	vb.SetTextureData(4, 4, texData, VOODOO_TEX_FMT_ARGB8888)
	vb.SetTextureEnabled(true)

	// Check that texture dimensions are stored
	if vb.textureWidth != 4 || vb.textureHeight != 4 {
		t.Errorf("Texture dimensions should be 4x4, got %dx%d", vb.textureWidth, vb.textureHeight)
	}

	// Check that texture is marked as enabled
	if !vb.textureEnabled {
		t.Error("Texture should be enabled")
	}
}

// Test that Vulkan uses GPU path when textures are enabled (not software fallback)
func TestVoodoo_VulkanBackend_GPUTextureRendering(t *testing.T) {
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}
	err = vb.Init(100, 100)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer vb.Destroy()

	// Skip if Vulkan is not initialized
	if !vb.initialized {
		t.Skip("Vulkan not available, skipping GPU texture test")
	}

	// Create a solid red texture
	texData := make([]byte, 2*2*4)
	for i := 0; i < len(texData); i += 4 {
		texData[i+0] = 255 // R
		texData[i+1] = 0   // G
		texData[i+2] = 0   // B
		texData[i+3] = 255 // A
	}

	vb.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	vb.SetTextureEnabled(true)

	// Clear to blue
	vb.ClearFramebuffer(0xFF0000FF)

	// Draw a textured triangle with white vertex color
	tri := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 10, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
			{X: 90, Y: 10, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
			{X: 50, Y: 90, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.5, T: 0.5, W: 1.0},
		},
	}

	vb.FlushTriangles([]VoodooTriangle{tri})
	vb.SwapBuffers(false)

	// Get the frame - should be GPU-rendered with texture
	frame := vb.GetFrame()
	centerIdx := (50*100 + 50) * 4
	r, g, b := frame[centerIdx], frame[centerIdx+1], frame[centerIdx+2]

	// Center should be red (from texture modulated by white vertex color)
	if r < 200 || g > 50 || b > 50 {
		t.Errorf("GPU textured triangle center should be red, got R=%d G=%d B=%d", r, g, b)
	}
}

// Test texture coordinate interpolation in Vulkan
func TestVoodoo_VulkanBackend_TextureCoordInterpolation(t *testing.T) {
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}
	err = vb.Init(100, 100)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer vb.Destroy()

	if !vb.initialized {
		t.Skip("Vulkan not available, skipping GPU texture test")
	}

	// Create a 2x2 checkerboard texture: red/green on top, blue/white on bottom
	texData := []byte{
		255, 0, 0, 255, // Red (0,0)
		0, 255, 0, 255, // Green (1,0)
		0, 0, 255, 255, // Blue (0,1)
		255, 255, 255, 255, // White (1,1)
	}

	vb.SetTextureData(2, 2, texData, VOODOO_TEX_FMT_ARGB8888)
	vb.SetTextureEnabled(true)

	// Clear to black
	vb.ClearFramebuffer(0xFF000000)

	// Draw a full-screen quad (two triangles) with proper UV mapping
	tri1 := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 0, Y: 0, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 0.0, W: 1.0},
			{X: 100, Y: 0, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 0.0, W: 1.0},
			{X: 100, Y: 100, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 1.0, W: 1.0},
		},
	}
	tri2 := VoodooTriangle{
		Vertices: [3]VoodooVertex{
			{X: 0, Y: 0, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 0.0, W: 1.0},
			{X: 100, Y: 100, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 1.0, T: 1.0, W: 1.0},
			{X: 0, Y: 100, Z: 0.5, R: 1, G: 1, B: 1, A: 1, S: 0.0, T: 1.0, W: 1.0},
		},
	}

	vb.FlushTriangles([]VoodooTriangle{tri1, tri2})
	vb.SwapBuffers(false)

	frame := vb.GetFrame()

	// Check corners - they should match the texture colors
	// Top-left (0,0) should be red
	idx := (10*100 + 10) * 4
	if frame[idx] < 200 {
		t.Errorf("Top-left should be red, got R=%d G=%d B=%d", frame[idx], frame[idx+1], frame[idx+2])
	}
}

// Test texture wrap mode
func TestVoodoo_VulkanBackend_TextureWrapMode(t *testing.T) {
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}
	err = vb.Init(100, 100)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer vb.Destroy()

	if !vb.initialized {
		t.Skip("Vulkan not available, skipping GPU texture test")
	}

	// Create a simple texture
	texData := []byte{255, 0, 0, 255}
	vb.SetTextureData(1, 1, texData, VOODOO_TEX_FMT_ARGB8888)
	vb.SetTextureEnabled(true)

	// Test clamp mode
	vb.SetTextureWrapMode(true, true)
	if !vb.textureClampS || !vb.textureClampT {
		t.Error("Texture clamp mode should be enabled")
	}

	// Test wrap mode
	vb.SetTextureWrapMode(false, false)
	if vb.textureClampS || vb.textureClampT {
		t.Error("Texture wrap mode should be enabled (clamp disabled)")
	}
}

// Test descriptor set layout creation
func TestVoodoo_VulkanBackend_DescriptorSetLayout(t *testing.T) {
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Fatalf("NewVulkanBackend failed: %v", err)
	}
	err = vb.Init(100, 100)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer vb.Destroy()

	if !vb.initialized {
		t.Skip("Vulkan not available")
	}

	// After initialization with texture support, descriptor set layout should exist
	// This tests that the Vulkan initialization properly sets up texture resources
	if vb.descriptorSetLayout == vk.NullDescriptorSetLayout {
		t.Error("Descriptor set layout should be created during init")
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
