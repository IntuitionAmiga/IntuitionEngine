// voodoo_vulkan.go - Vulkan Backend for Voodoo Graphics (Stub + Software Fallback)

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

/*
voodoo_vulkan.go - Vulkan Rendering Backend for Voodoo Graphics

This file provides the Vulkan backend implementation for hardware-accelerated
3D rendering. When Vulkan is unavailable, a software rasterizer fallback is used.

Vulkan Backend (TODO):
- VkInstance, VkDevice, VkQueue setup
- Graphics pipeline with configurable depth/blend state
- Dynamic vertex buffer for triangle batches
- Framebuffer with depth attachment
- Frame readback for compositor integration

Software Backend (implemented):
- Basic triangle rasterization with barycentric interpolation
- Z-buffering
- Gouraud shading
- Scissor clipping

The software backend serves two purposes:
1. Fallback when Vulkan is unavailable
2. Reference implementation for testing
*/

package main

import (
	"math"
	"sync"
)

// VoodooSoftwareBackend implements software rasterization as a fallback
type VoodooSoftwareBackend struct {
	mutex sync.RWMutex

	// Framebuffer
	width, height int
	colorBuffer   []byte    // RGBA
	depthBuffer   []float32 // Z values

	// State
	fbzMode   uint32
	alphaMode uint32

	// Scissor rectangle
	scissorLeft, scissorTop     int
	scissorRight, scissorBottom int

	// Double buffering
	frontBuffer []byte
	backBuffer  []byte
	isBackBuf   bool
}

// NewVoodooSoftwareBackend creates a new software rasterizer backend
func NewVoodooSoftwareBackend() *VoodooSoftwareBackend {
	return &VoodooSoftwareBackend{}
}

// Init initializes the software backend with given dimensions
func (b *VoodooSoftwareBackend) Init(width, height int) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.width = width
	b.height = height

	// Allocate buffers
	pixelCount := width * height
	b.colorBuffer = make([]byte, pixelCount*4)
	b.depthBuffer = make([]float32, pixelCount)
	b.frontBuffer = make([]byte, pixelCount*4)
	b.backBuffer = make([]byte, pixelCount*4)

	// Initialize depth buffer to max depth
	for i := range b.depthBuffer {
		b.depthBuffer[i] = math.MaxFloat32
	}

	// Default scissor to full screen
	b.scissorLeft = 0
	b.scissorTop = 0
	b.scissorRight = width
	b.scissorBottom = height

	// Default state
	b.fbzMode = VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_LESS << 5)

	return nil
}

// UpdatePipelineState updates the rendering state
func (b *VoodooSoftwareBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.fbzMode = fbzMode
	b.alphaMode = alphaMode
	return nil
}

// SetScissor sets the scissor rectangle
func (b *VoodooSoftwareBackend) SetScissor(left, top, right, bottom int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.scissorLeft = max(0, left)
	b.scissorTop = max(0, top)
	b.scissorRight = min(b.width, right)
	b.scissorBottom = min(b.height, bottom)
}

// FlushTriangles rasterizes all triangles in the batch
func (b *VoodooSoftwareBackend) FlushTriangles(triangles []VoodooTriangle) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, tri := range triangles {
		b.rasterizeTriangle(&tri)
	}
}

// ClearFramebuffer clears the color and depth buffers
func (b *VoodooSoftwareBackend) ClearFramebuffer(color uint32) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Extract RGBA from packed color (assuming ARGB format)
	r := byte((color >> 16) & 0xFF)
	g := byte((color >> 8) & 0xFF)
	bVal := byte(color & 0xFF)
	a := byte((color >> 24) & 0xFF)
	if a == 0 {
		a = 255 // Default to opaque
	}

	// Clear color buffer
	for i := 0; i < len(b.colorBuffer); i += 4 {
		b.colorBuffer[i+0] = r
		b.colorBuffer[i+1] = g
		b.colorBuffer[i+2] = bVal
		b.colorBuffer[i+3] = a
	}

	// Clear depth buffer
	for i := range b.depthBuffer {
		b.depthBuffer[i] = math.MaxFloat32
	}
}

// SwapBuffers swaps front and back buffers
func (b *VoodooSoftwareBackend) SwapBuffers(waitVSync bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Copy color buffer to front buffer
	copy(b.frontBuffer, b.colorBuffer)
}

// GetFrame returns the current front buffer
func (b *VoodooSoftwareBackend) GetFrame() []byte {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.frontBuffer
}

// Destroy cleans up resources
func (b *VoodooSoftwareBackend) Destroy() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.colorBuffer = nil
	b.depthBuffer = nil
	b.frontBuffer = nil
	b.backBuffer = nil
}

// rasterizeTriangle performs software triangle rasterization
func (b *VoodooSoftwareBackend) rasterizeTriangle(tri *VoodooTriangle) {
	v0 := &tri.Vertices[0]
	v1 := &tri.Vertices[1]
	v2 := &tri.Vertices[2]

	// Check if clipping is enabled
	enableClipping := (b.fbzMode & VOODOO_FBZ_CLIPPING) != 0

	// Compute bounding box
	minX := int(math.Floor(float64(min3f(v0.X, v1.X, v2.X))))
	maxX := int(math.Ceil(float64(max3f(v0.X, v1.X, v2.X))))
	minY := int(math.Floor(float64(min3f(v0.Y, v1.Y, v2.Y))))
	maxY := int(math.Ceil(float64(max3f(v0.Y, v1.Y, v2.Y))))

	// Clip to screen bounds
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > b.width {
		maxX = b.width
	}
	if maxY > b.height {
		maxY = b.height
	}

	// Clip to scissor rectangle if enabled
	if enableClipping {
		if minX < b.scissorLeft {
			minX = b.scissorLeft
		}
		if minY < b.scissorTop {
			minY = b.scissorTop
		}
		if maxX > b.scissorRight {
			maxX = b.scissorRight
		}
		if maxY > b.scissorBottom {
			maxY = b.scissorBottom
		}
	}

	// Compute triangle area (2x for efficiency)
	area := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, v2.X, v2.Y)
	if area == 0 {
		return // Degenerate triangle
	}

	// Handle backface culling (if area is negative, triangle is back-facing)
	if area < 0 {
		// Swap vertices to make it front-facing
		v0, v2 = v2, v0
		area = -area
	}

	invArea := 1.0 / area

	// Check depth test settings
	depthEnable := (b.fbzMode & VOODOO_FBZ_DEPTH_ENABLE) != 0
	depthWrite := (b.fbzMode & VOODOO_FBZ_DEPTH_WRITE) != 0
	rgbWrite := (b.fbzMode & VOODOO_FBZ_RGB_WRITE) != 0
	depthFunc := int((b.fbzMode >> 5) & 0x7)

	// Check alpha blending
	alphaBlendEnable := (b.alphaMode & VOODOO_ALPHA_BLEND_EN) != 0

	// Rasterize
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			// Sample at pixel center
			px := float32(x) + 0.5
			py := float32(y) + 0.5

			// Compute barycentric coordinates
			w0 := edgeFunction(v1.X, v1.Y, v2.X, v2.Y, px, py)
			w1 := edgeFunction(v2.X, v2.Y, v0.X, v0.Y, px, py)
			w2 := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, px, py)

			// Check if pixel is inside triangle
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				// Normalize barycentric coordinates
				w0 *= invArea
				w1 *= invArea
				w2 *= invArea

				// Interpolate Z
				z := w0*v0.Z + w1*v1.Z + w2*v2.Z

				// Depth test
				pixelIndex := y*b.width + x
				if depthEnable {
					oldZ := b.depthBuffer[pixelIndex]
					if !b.depthTest(z, oldZ, depthFunc) {
						continue
					}
				}

				// Interpolate color (Gouraud shading)
				r := w0*v0.R + w1*v1.R + w2*v2.R
				g := w0*v0.G + w1*v1.G + w2*v2.G
				bVal := w0*v0.B + w1*v1.B + w2*v2.B
				a := w0*v0.A + w1*v1.A + w2*v2.A

				// Clamp colors
				r = clampf(r, 0, 1)
				g = clampf(g, 0, 1)
				bVal = clampf(bVal, 0, 1)
				a = clampf(a, 0, 1)

				// Convert to bytes
				rByte := byte(r * 255)
				gByte := byte(g * 255)
				bByte := byte(bVal * 255)
				aByte := byte(a * 255)

				// Write pixel
				if rgbWrite {
					bufIdx := pixelIndex * 4
					if alphaBlendEnable && aByte < 255 {
						// Simple alpha blending
						srcA := float32(aByte) / 255.0
						dstA := 1.0 - srcA
						b.colorBuffer[bufIdx+0] = byte(float32(rByte)*srcA + float32(b.colorBuffer[bufIdx+0])*dstA)
						b.colorBuffer[bufIdx+1] = byte(float32(gByte)*srcA + float32(b.colorBuffer[bufIdx+1])*dstA)
						b.colorBuffer[bufIdx+2] = byte(float32(bByte)*srcA + float32(b.colorBuffer[bufIdx+2])*dstA)
						b.colorBuffer[bufIdx+3] = 255
					} else {
						b.colorBuffer[bufIdx+0] = rByte
						b.colorBuffer[bufIdx+1] = gByte
						b.colorBuffer[bufIdx+2] = bByte
						b.colorBuffer[bufIdx+3] = aByte
					}
				}

				// Write depth
				if depthEnable && depthWrite {
					b.depthBuffer[pixelIndex] = z
				}
			}
		}
	}
}

// depthTest performs depth comparison
func (b *VoodooSoftwareBackend) depthTest(newZ, oldZ float32, depthFunc int) bool {
	switch depthFunc {
	case VOODOO_DEPTH_NEVER:
		return false
	case VOODOO_DEPTH_LESS:
		return newZ < oldZ
	case VOODOO_DEPTH_EQUAL:
		return newZ == oldZ
	case VOODOO_DEPTH_LESSEQUAL:
		return newZ <= oldZ
	case VOODOO_DEPTH_GREATER:
		return newZ > oldZ
	case VOODOO_DEPTH_NOTEQUAL:
		return newZ != oldZ
	case VOODOO_DEPTH_GREATEREQUAL:
		return newZ >= oldZ
	case VOODOO_DEPTH_ALWAYS:
		return true
	}
	return true
}

// edgeFunction computes the signed area of a parallelogram
// Used for barycentric coordinate calculation
func edgeFunction(ax, ay, bx, by, cx, cy float32) float32 {
	return (cx-ax)*(by-ay) - (cy-ay)*(bx-ax)
}

// Helper functions for min/max of 3 floats
func min3f(a, b, c float32) float32 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func max3f(a, b, c float32) float32 {
	if a > b {
		if a > c {
			return a
		}
		return c
	}
	if b > c {
		return b
	}
	return c
}

func clampf(v, minVal, maxVal float32) float32 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

// VulkanBackend implements Vulkan-accelerated rendering
// TODO: Implement when Vulkan support is added
type VulkanBackend struct {
	// Vulkan handles (to be implemented)
	// instance       vk.Instance
	// physicalDevice vk.PhysicalDevice
	// device         vk.Device
	// queue          vk.Queue
	// swapchain      vk.Swapchain
	// renderPass     vk.RenderPass
	// pipeline       vk.Pipeline
	// commandPool    vk.CommandPool
	// vertexBuffer   vk.Buffer
	// depthBuffer    vk.Image

	// For now, delegate to software backend
	software *VoodooSoftwareBackend
}

// NewVulkanBackend creates a new Vulkan backend
func NewVulkanBackend() (*VulkanBackend, error) {
	// TODO: Initialize Vulkan
	// For now, use software fallback
	return &VulkanBackend{
		software: NewVoodooSoftwareBackend(),
	}, nil
}

// Init initializes the Vulkan backend
func (vb *VulkanBackend) Init(width, height int) error {
	// TODO: Create Vulkan device, swapchain, pipeline
	return vb.software.Init(width, height)
}

// UpdatePipelineState updates Vulkan pipeline state
func (vb *VulkanBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	// TODO: Recreate pipeline with new state
	return vb.software.UpdatePipelineState(fbzMode, alphaMode)
}

// SetScissor sets the Vulkan scissor rectangle
func (vb *VulkanBackend) SetScissor(left, top, right, bottom int) {
	// TODO: Update Vulkan scissor
	vb.software.SetScissor(left, top, right, bottom)
}

// FlushTriangles submits triangles to Vulkan for rendering
func (vb *VulkanBackend) FlushTriangles(triangles []VoodooTriangle) {
	// TODO: Upload to vertex buffer and issue draw call
	vb.software.FlushTriangles(triangles)
}

// ClearFramebuffer clears the Vulkan framebuffer
func (vb *VulkanBackend) ClearFramebuffer(color uint32) {
	// TODO: vkCmdClearColorImage
	vb.software.ClearFramebuffer(color)
}

// SwapBuffers presents the rendered frame
func (vb *VulkanBackend) SwapBuffers(waitVSync bool) {
	// TODO: vkQueuePresent
	vb.software.SwapBuffers(waitVSync)
}

// GetFrame returns the rendered frame
func (vb *VulkanBackend) GetFrame() []byte {
	// TODO: Read back from Vulkan framebuffer
	return vb.software.GetFrame()
}

// Destroy cleans up Vulkan resources
func (vb *VulkanBackend) Destroy() {
	// TODO: Destroy Vulkan objects
	if vb.software != nil {
		vb.software.Destroy()
	}
}
