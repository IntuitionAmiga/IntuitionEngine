// voodoo_vulkan.go - Vulkan Backend for Voodoo Graphics

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

This file provides both the Vulkan backend implementation for hardware-accelerated
3D rendering and a software rasterizer fallback.

Vulkan Backend:
- Offscreen rendering (no window/swapchain needed)
- Dynamic vertex buffer for triangle batches
- Configurable depth test and alpha blending via pipeline recreation
- Frame readback via staging buffer for compositor integration

Software Backend:
- Barycentric triangle rasterization
- Z-buffering with all 8 compare functions
- Gouraud shading
- Scissor clipping
- Alpha blending
*/

package main

import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	vk "github.com/goki/vulkan"
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
func edgeFunction(ax, ay, bx, by, cx, cy float32) float32 {
	return (cx-ax)*(by-ay) - (cy-ay)*(bx-ax)
}

// Helper functions
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

// =============================================================================
// Vulkan Backend Implementation
// =============================================================================

// VulkanVertex is the vertex format for the Vulkan pipeline
type VulkanVertex struct {
	Position [3]float32 // X, Y, Z
	Color    [4]float32 // R, G, B, A
}

// VulkanBackend implements hardware-accelerated rendering using Vulkan
type VulkanBackend struct {
	mutex sync.RWMutex

	// Vulkan instance and device
	instance       vk.Instance
	physicalDevice vk.PhysicalDevice
	device         vk.Device
	graphicsQueue  vk.Queue
	queueFamily    uint32

	// Offscreen rendering resources
	width, height    int
	colorImage       vk.Image
	colorImageMemory vk.DeviceMemory
	colorImageView   vk.ImageView
	depthImage       vk.Image
	depthImageMemory vk.DeviceMemory
	depthImageView   vk.ImageView

	// Render pass and framebuffer
	renderPass  vk.RenderPass
	framebuffer vk.Framebuffer

	// Pipeline
	pipelineLayout vk.PipelineLayout
	pipeline       vk.Pipeline
	pipelineCache  vk.PipelineCache

	// Vertex buffer (dynamic)
	vertexBuffer       vk.Buffer
	vertexBufferMemory vk.DeviceMemory
	vertexBufferSize   vk.DeviceSize

	// Staging buffer for readback
	stagingBuffer       vk.Buffer
	stagingBufferMemory vk.DeviceMemory

	// Command pool and buffer
	commandPool   vk.CommandPool
	commandBuffer vk.CommandBuffer

	// Synchronization
	fence vk.Fence

	// Current state
	fbzMode   uint32
	alphaMode uint32
	scissor   vk.Rect2D

	// Clear color (set by ClearFramebuffer, used by FlushTriangles)
	clearColor [4]float32
	needsClear bool

	// Output frame for compositor
	outputFrame []byte

	// Shader modules
	vertShaderModule vk.ShaderModule
	fragShaderModule vk.ShaderModule

	// Initialization state
	initialized bool

	// Software fallback (used if Vulkan init fails)
	software *VoodooSoftwareBackend
}

// Global Vulkan initialization flag
var vulkanInitialized bool
var vulkanInitMutex sync.Mutex

// NewVulkanBackend creates a new Vulkan backend
func NewVulkanBackend() (*VulkanBackend, error) {
	vb := &VulkanBackend{
		software: NewVoodooSoftwareBackend(),
	}
	return vb, nil
}

// Init initializes the Vulkan backend
func (vb *VulkanBackend) Init(width, height int) error {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.width = width
	vb.height = height
	vb.outputFrame = make([]byte, width*height*4)

	// Initialize software backend as fallback
	if err := vb.software.Init(width, height); err != nil {
		return err
	}

	// Try to initialize Vulkan
	if err := vb.initVulkan(); err != nil {
		// Vulkan initialization failed, will use software fallback
		fmt.Printf("Vulkan initialization failed, using software backend: %v\n", err)
		vb.initialized = false
		return nil
	}

	vb.initialized = true
	return nil
}

// initVulkan performs full Vulkan initialization
func (vb *VulkanBackend) initVulkan() error {
	vulkanInitMutex.Lock()
	defer vulkanInitMutex.Unlock()

	// Initialize Vulkan loader
	if !vulkanInitialized {
		// First, set up the proc address loader to find the Vulkan library
		if err := vk.SetDefaultGetInstanceProcAddr(); err != nil {
			return fmt.Errorf("failed to load Vulkan library: %w", err)
		}
		if err := vk.Init(); err != nil {
			return fmt.Errorf("failed to initialize Vulkan loader: %w", err)
		}
		vulkanInitialized = true
	}

	// Create instance
	if err := vb.createInstance(); err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	// Select physical device
	if err := vb.selectPhysicalDevice(); err != nil {
		vb.destroyInstance()
		return fmt.Errorf("failed to select physical device: %w", err)
	}

	// Create logical device
	if err := vb.createDevice(); err != nil {
		vb.destroyInstance()
		return fmt.Errorf("failed to create device: %w", err)
	}

	// Create command pool
	if err := vb.createCommandPool(); err != nil {
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create command pool: %w", err)
	}

	// Create offscreen images
	if err := vb.createOffscreenImages(); err != nil {
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create offscreen images: %w", err)
	}

	// Create render pass
	if err := vb.createRenderPass(); err != nil {
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create render pass: %w", err)
	}

	// Create framebuffer
	if err := vb.createFramebuffer(); err != nil {
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create framebuffer: %w", err)
	}

	// Create pipeline
	if err := vb.createPipeline(); err != nil {
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Create vertex buffer
	if err := vb.createVertexBuffer(); err != nil {
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create vertex buffer: %w", err)
	}

	// Create staging buffer for readback
	if err := vb.createStagingBuffer(); err != nil {
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create staging buffer: %w", err)
	}

	// Create command buffer
	if err := vb.createCommandBuffer(); err != nil {
		vb.destroyStagingBuffer()
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create command buffer: %w", err)
	}

	// Create fence
	if err := vb.createFence(); err != nil {
		vb.destroyStagingBuffer()
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
		return fmt.Errorf("failed to create fence: %w", err)
	}

	// Set default scissor
	vb.scissor = vk.Rect2D{
		Offset: vk.Offset2D{X: 0, Y: 0},
		Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)},
	}

	return nil
}

// createInstance creates the Vulkan instance
func (vb *VulkanBackend) createInstance() error {
	appInfo := vk.ApplicationInfo{
		SType:              vk.StructureTypeApplicationInfo,
		PApplicationName:   safeString("IntuitionEngine Voodoo"),
		ApplicationVersion: vk.MakeVersion(1, 0, 0),
		PEngineName:        safeString("Voodoo HLE"),
		EngineVersion:      vk.MakeVersion(1, 0, 0),
		ApiVersion:         vk.MakeVersion(1, 1, 0),
	}

	createInfo := vk.InstanceCreateInfo{
		SType:            vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo: &appInfo,
	}

	var instance vk.Instance
	if res := vk.CreateInstance(&createInfo, nil, &instance); res != vk.Success {
		return fmt.Errorf("vkCreateInstance failed: %d", res)
	}

	vb.instance = instance
	vk.InitInstance(instance)
	return nil
}

// selectPhysicalDevice selects a suitable GPU
func (vb *VulkanBackend) selectPhysicalDevice() error {
	var deviceCount uint32
	vk.EnumeratePhysicalDevices(vb.instance, &deviceCount, nil)
	if deviceCount == 0 {
		return fmt.Errorf("no Vulkan-capable GPUs found")
	}

	devices := make([]vk.PhysicalDevice, deviceCount)
	vk.EnumeratePhysicalDevices(vb.instance, &deviceCount, devices)

	// Find a device with a graphics queue
	for _, device := range devices {
		var queueFamilyCount uint32
		vk.GetPhysicalDeviceQueueFamilyProperties(device, &queueFamilyCount, nil)
		queueFamilies := make([]vk.QueueFamilyProperties, queueFamilyCount)
		vk.GetPhysicalDeviceQueueFamilyProperties(device, &queueFamilyCount, queueFamilies)

		for i, qf := range queueFamilies {
			qf.Deref()
			if qf.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 {
				vb.physicalDevice = device
				vb.queueFamily = uint32(i)
				return nil
			}
		}
	}

	return fmt.Errorf("no suitable GPU with graphics queue found")
}

// createDevice creates the logical device
func (vb *VulkanBackend) createDevice() error {
	queuePriority := float32(1.0)
	queueCreateInfo := vk.DeviceQueueCreateInfo{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: vb.queueFamily,
		QueueCount:       1,
		PQueuePriorities: []float32{queuePriority},
	}

	deviceCreateInfo := vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos:    []vk.DeviceQueueCreateInfo{queueCreateInfo},
	}

	var device vk.Device
	if res := vk.CreateDevice(vb.physicalDevice, &deviceCreateInfo, nil, &device); res != vk.Success {
		return fmt.Errorf("vkCreateDevice failed: %d", res)
	}

	vb.device = device

	var queue vk.Queue
	vk.GetDeviceQueue(device, vb.queueFamily, 0, &queue)
	vb.graphicsQueue = queue

	return nil
}

// createCommandPool creates the command pool
func (vb *VulkanBackend) createCommandPool() error {
	poolInfo := vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		QueueFamilyIndex: vb.queueFamily,
		Flags:            vk.CommandPoolCreateFlags(vk.CommandPoolCreateResetCommandBufferBit),
	}

	var pool vk.CommandPool
	if res := vk.CreateCommandPool(vb.device, &poolInfo, nil, &pool); res != vk.Success {
		return fmt.Errorf("vkCreateCommandPool failed: %d", res)
	}

	vb.commandPool = pool
	return nil
}

// createOffscreenImages creates the color and depth images for offscreen rendering
func (vb *VulkanBackend) createOffscreenImages() error {
	// Color image
	colorImageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    vk.FormatR8g8b8a8Unorm,
		Extent: vk.Extent3D{
			Width:  uint32(vb.width),
			Height: uint32(vb.height),
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1Bit,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit | vk.ImageUsageTransferSrcBit),
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var colorImage vk.Image
	if res := vk.CreateImage(vb.device, &colorImageInfo, nil, &colorImage); res != vk.Success {
		return fmt.Errorf("vkCreateImage (color) failed: %d", res)
	}
	vb.colorImage = colorImage

	// Allocate memory for color image
	var memReqs vk.MemoryRequirements
	vk.GetImageMemoryRequirements(vb.device, colorImage, &memReqs)
	memReqs.Deref()

	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		return err
	}

	colorAllocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var colorMem vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &colorAllocInfo, nil, &colorMem); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (color) failed: %d", res)
	}
	vb.colorImageMemory = colorMem

	vk.BindImageMemory(vb.device, colorImage, colorMem, 0)

	// Color image view
	colorViewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    colorImage,
		ViewType: vk.ImageViewType2d,
		Format:   vk.FormatR8g8b8a8Unorm,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var colorView vk.ImageView
	if res := vk.CreateImageView(vb.device, &colorViewInfo, nil, &colorView); res != vk.Success {
		return fmt.Errorf("vkCreateImageView (color) failed: %d", res)
	}
	vb.colorImageView = colorView

	// Depth image
	depthFormat := vk.FormatD32Sfloat
	depthImageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    depthFormat,
		Extent: vk.Extent3D{
			Width:  uint32(vb.width),
			Height: uint32(vb.height),
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1Bit,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageDepthStencilAttachmentBit),
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var depthImage vk.Image
	if res := vk.CreateImage(vb.device, &depthImageInfo, nil, &depthImage); res != vk.Success {
		return fmt.Errorf("vkCreateImage (depth) failed: %d", res)
	}
	vb.depthImage = depthImage

	// Allocate memory for depth image
	vk.GetImageMemoryRequirements(vb.device, depthImage, &memReqs)
	memReqs.Deref()

	memTypeIndex, err = vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		return err
	}

	depthAllocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var depthMem vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &depthAllocInfo, nil, &depthMem); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (depth) failed: %d", res)
	}
	vb.depthImageMemory = depthMem

	vk.BindImageMemory(vb.device, depthImage, depthMem, 0)

	// Depth image view
	depthViewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    depthImage,
		ViewType: vk.ImageViewType2d,
		Format:   depthFormat,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectDepthBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var depthView vk.ImageView
	if res := vk.CreateImageView(vb.device, &depthViewInfo, nil, &depthView); res != vk.Success {
		return fmt.Errorf("vkCreateImageView (depth) failed: %d", res)
	}
	vb.depthImageView = depthView

	return nil
}

// createRenderPass creates the render pass
func (vb *VulkanBackend) createRenderPass() error {
	colorAttachment := vk.AttachmentDescription{
		Format:         vk.FormatR8g8b8a8Unorm,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutTransferSrcOptimal,
	}

	depthAttachment := vk.AttachmentDescription{
		Format:         vk.FormatD32Sfloat,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpDontCare,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutDepthStencilAttachmentOptimal,
	}

	colorRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}

	depthRef := vk.AttachmentReference{
		Attachment: 1,
		Layout:     vk.ImageLayoutDepthStencilAttachmentOptimal,
	}

	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    1,
		PColorAttachments:       []vk.AttachmentReference{colorRef},
		PDepthStencilAttachment: &depthRef,
	}

	renderPassInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 2,
		PAttachments:    []vk.AttachmentDescription{colorAttachment, depthAttachment},
		SubpassCount:    1,
		PSubpasses:      []vk.SubpassDescription{subpass},
	}

	var renderPass vk.RenderPass
	if res := vk.CreateRenderPass(vb.device, &renderPassInfo, nil, &renderPass); res != vk.Success {
		return fmt.Errorf("vkCreateRenderPass failed: %d", res)
	}

	vb.renderPass = renderPass
	return nil
}

// createFramebuffer creates the framebuffer
func (vb *VulkanBackend) createFramebuffer() error {
	attachments := []vk.ImageView{vb.colorImageView, vb.depthImageView}

	fbInfo := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      vb.renderPass,
		AttachmentCount: uint32(len(attachments)),
		PAttachments:    attachments,
		Width:           uint32(vb.width),
		Height:          uint32(vb.height),
		Layers:          1,
	}

	var framebuffer vk.Framebuffer
	if res := vk.CreateFramebuffer(vb.device, &fbInfo, nil, &framebuffer); res != vk.Success {
		return fmt.Errorf("vkCreateFramebuffer failed: %d", res)
	}

	vb.framebuffer = framebuffer
	return nil
}

// createPipeline creates the graphics pipeline
func (vb *VulkanBackend) createPipeline() error {
	// Create shader modules from embedded SPIR-V
	vertModule, err := vb.createShaderModule(VoodooVertexShaderSPIRV)
	if err != nil {
		return fmt.Errorf("failed to create vertex shader module: %w", err)
	}
	vb.vertShaderModule = vertModule

	fragModule, err := vb.createShaderModule(VoodooFragmentShaderSPIRV)
	if err != nil {
		vk.DestroyShaderModule(vb.device, vertModule, nil)
		return fmt.Errorf("failed to create fragment shader module: %w", err)
	}
	vb.fragShaderModule = fragModule

	// Shader stages
	vertStage := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageVertexBit,
		Module: vertModule,
		PName:  safeString("main"),
	}

	fragStage := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageFragmentBit,
		Module: fragModule,
		PName:  safeString("main"),
	}

	shaderStages := []vk.PipelineShaderStageCreateInfo{vertStage, fragStage}

	// Vertex input
	bindingDesc := vk.VertexInputBindingDescription{
		Binding:   0,
		Stride:    uint32(unsafe.Sizeof(VulkanVertex{})),
		InputRate: vk.VertexInputRateVertex,
	}

	attrDescs := []vk.VertexInputAttributeDescription{
		{
			Location: 0,
			Binding:  0,
			Format:   vk.FormatR32g32b32Sfloat,
			Offset:   0,
		},
		{
			Location: 1,
			Binding:  0,
			Format:   vk.FormatR32g32b32a32Sfloat,
			Offset:   uint32(unsafe.Offsetof(VulkanVertex{}.Color)),
		},
	}

	vertexInputInfo := vk.PipelineVertexInputStateCreateInfo{
		SType:                           vk.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		PVertexBindingDescriptions:      []vk.VertexInputBindingDescription{bindingDesc},
		VertexAttributeDescriptionCount: uint32(len(attrDescs)),
		PVertexAttributeDescriptions:    attrDescs,
	}

	// Input assembly
	inputAssembly := vk.PipelineInputAssemblyStateCreateInfo{
		SType:                  vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               vk.PrimitiveTopologyTriangleList,
		PrimitiveRestartEnable: vk.False,
	}

	// Viewport and scissor (dynamic)
	viewport := vk.Viewport{
		X:        0,
		Y:        0,
		Width:    float32(vb.width),
		Height:   float32(vb.height),
		MinDepth: 0,
		MaxDepth: 1,
	}

	scissor := vk.Rect2D{
		Offset: vk.Offset2D{X: 0, Y: 0},
		Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)},
	}

	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		PViewports:    []vk.Viewport{viewport},
		ScissorCount:  1,
		PScissors:     []vk.Rect2D{scissor},
	}

	// Rasterization
	rasterizer := vk.PipelineRasterizationStateCreateInfo{
		SType:                   vk.StructureTypePipelineRasterizationStateCreateInfo,
		DepthClampEnable:        vk.False,
		RasterizerDiscardEnable: vk.False,
		PolygonMode:             vk.PolygonModeFill,
		CullMode:                vk.CullModeFlags(vk.CullModeNone),
		FrontFace:               vk.FrontFaceCounterClockwise,
		DepthBiasEnable:         vk.False,
		LineWidth:               1.0,
	}

	// Multisampling
	multisampling := vk.PipelineMultisampleStateCreateInfo{
		SType:                 vk.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples:  vk.SampleCount1Bit,
		SampleShadingEnable:   vk.False,
		MinSampleShading:      1.0,
		AlphaToCoverageEnable: vk.False,
		AlphaToOneEnable:      vk.False,
	}

	// Depth/stencil
	depthStencil := vk.PipelineDepthStencilStateCreateInfo{
		SType:                 vk.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable:       vk.True,
		DepthWriteEnable:      vk.True,
		DepthCompareOp:        vk.CompareOpLess,
		DepthBoundsTestEnable: vk.False,
		StencilTestEnable:     vk.False,
	}

	// Color blending
	colorBlendAttachment := vk.PipelineColorBlendAttachmentState{
		BlendEnable:         vk.False,
		SrcColorBlendFactor: vk.BlendFactorOne,
		DstColorBlendFactor: vk.BlendFactorZero,
		ColorBlendOp:        vk.BlendOpAdd,
		SrcAlphaBlendFactor: vk.BlendFactorOne,
		DstAlphaBlendFactor: vk.BlendFactorZero,
		AlphaBlendOp:        vk.BlendOpAdd,
		ColorWriteMask:      vk.ColorComponentFlags(vk.ColorComponentRBit | vk.ColorComponentGBit | vk.ColorComponentBBit | vk.ColorComponentABit),
	}

	colorBlending := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		LogicOpEnable:   vk.False,
		AttachmentCount: 1,
		PAttachments:    []vk.PipelineColorBlendAttachmentState{colorBlendAttachment},
	}

	// Dynamic state
	dynamicStates := []vk.DynamicState{vk.DynamicStateScissor}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    dynamicStates,
	}

	// Pipeline layout
	layoutInfo := vk.PipelineLayoutCreateInfo{
		SType: vk.StructureTypePipelineLayoutCreateInfo,
	}

	var pipelineLayout vk.PipelineLayout
	if res := vk.CreatePipelineLayout(vb.device, &layoutInfo, nil, &pipelineLayout); res != vk.Success {
		return fmt.Errorf("vkCreatePipelineLayout failed: %d", res)
	}
	vb.pipelineLayout = pipelineLayout

	// Create pipeline
	pipelineInfo := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          uint32(len(shaderStages)),
		PStages:             shaderStages,
		PVertexInputState:   &vertexInputInfo,
		PInputAssemblyState: &inputAssembly,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterizer,
		PMultisampleState:   &multisampling,
		PDepthStencilState:  &depthStencil,
		PColorBlendState:    &colorBlending,
		PDynamicState:       &dynamicState,
		Layout:              pipelineLayout,
		RenderPass:          vb.renderPass,
		Subpass:             0,
	}

	pipelines := make([]vk.Pipeline, 1)
	if res := vk.CreateGraphicsPipelines(vb.device, vk.PipelineCache(vk.NullHandle), 1, []vk.GraphicsPipelineCreateInfo{pipelineInfo}, nil, pipelines); res != vk.Success {
		return fmt.Errorf("vkCreateGraphicsPipelines failed: %d", res)
	}

	vb.pipeline = pipelines[0]
	return nil
}

// createShaderModule creates a shader module from SPIR-V bytecode
func (vb *VulkanBackend) createShaderModule(code []byte) (vk.ShaderModule, error) {
	createInfo := vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uint64(len(code)),
		PCode:    sliceUint32(code),
	}

	var shaderModule vk.ShaderModule
	if res := vk.CreateShaderModule(vb.device, &createInfo, nil, &shaderModule); res != vk.Success {
		return vk.NullShaderModule, fmt.Errorf("vkCreateShaderModule failed: %d", res)
	}

	return shaderModule, nil
}

// createVertexBuffer creates the dynamic vertex buffer
func (vb *VulkanBackend) createVertexBuffer() error {
	// Size for maximum triangles
	vb.vertexBufferSize = vk.DeviceSize(VOODOO_MAX_BATCH_VERTICES * int(unsafe.Sizeof(VulkanVertex{})))

	bufferInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        vb.vertexBufferSize,
		Usage:       vk.BufferUsageFlags(vk.BufferUsageVertexBufferBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var buffer vk.Buffer
	if res := vk.CreateBuffer(vb.device, &bufferInfo, nil, &buffer); res != vk.Success {
		return fmt.Errorf("vkCreateBuffer (vertex) failed: %d", res)
	}
	vb.vertexBuffer = buffer

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(vb.device, buffer, &memReqs)
	memReqs.Deref()

	// Find host-visible memory type
	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		return err
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &allocInfo, nil, &memory); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (vertex) failed: %d", res)
	}
	vb.vertexBufferMemory = memory

	vk.BindBufferMemory(vb.device, buffer, memory, 0)
	return nil
}

// createStagingBuffer creates a buffer for reading back the framebuffer
func (vb *VulkanBackend) createStagingBuffer() error {
	bufferSize := vk.DeviceSize(vb.width * vb.height * 4)

	bufferInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        bufferSize,
		Usage:       vk.BufferUsageFlags(vk.BufferUsageTransferDstBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var buffer vk.Buffer
	if res := vk.CreateBuffer(vb.device, &bufferInfo, nil, &buffer); res != vk.Success {
		return fmt.Errorf("vkCreateBuffer (staging) failed: %d", res)
	}
	vb.stagingBuffer = buffer

	var memReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(vb.device, buffer, &memReqs)
	memReqs.Deref()

	memTypeIndex, err := vb.findMemoryType(memReqs.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		return err
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	if res := vk.AllocateMemory(vb.device, &allocInfo, nil, &memory); res != vk.Success {
		return fmt.Errorf("vkAllocateMemory (staging) failed: %d", res)
	}
	vb.stagingBufferMemory = memory

	vk.BindBufferMemory(vb.device, buffer, memory, 0)
	return nil
}

// createCommandBuffer allocates a command buffer
func (vb *VulkanBackend) createCommandBuffer() error {
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        vb.commandPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}

	cmdBuffers := make([]vk.CommandBuffer, 1)
	if res := vk.AllocateCommandBuffers(vb.device, &allocInfo, cmdBuffers); res != vk.Success {
		return fmt.Errorf("vkAllocateCommandBuffers failed: %d", res)
	}

	vb.commandBuffer = cmdBuffers[0]
	return nil
}

// createFence creates a fence for synchronization
func (vb *VulkanBackend) createFence() error {
	fenceInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: vk.FenceCreateFlags(vk.FenceCreateSignaledBit),
	}

	var fence vk.Fence
	if res := vk.CreateFence(vb.device, &fenceInfo, nil, &fence); res != vk.Success {
		return fmt.Errorf("vkCreateFence failed: %d", res)
	}

	vb.fence = fence
	return nil
}

// findMemoryType finds a suitable memory type
func (vb *VulkanBackend) findMemoryType(typeFilter uint32, properties vk.MemoryPropertyFlags) (uint32, error) {
	var memProps vk.PhysicalDeviceMemoryProperties
	vk.GetPhysicalDeviceMemoryProperties(vb.physicalDevice, &memProps)
	memProps.Deref()

	for i := uint32(0); i < memProps.MemoryTypeCount; i++ {
		memProps.MemoryTypes[i].Deref()
		if (typeFilter&(1<<i)) != 0 && (memProps.MemoryTypes[i].PropertyFlags&properties) == properties {
			return i, nil
		}
	}

	return 0, fmt.Errorf("failed to find suitable memory type")
}

// UpdatePipelineState updates the pipeline state (may require recreation)
func (vb *VulkanBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.fbzMode = fbzMode
	vb.alphaMode = alphaMode

	// Update software backend too
	vb.software.UpdatePipelineState(fbzMode, alphaMode)

	// Note: Full implementation would recreate the pipeline with new depth/blend state
	// For now, we handle most state via dynamic state or per-fragment in shader
	return nil
}

// SetScissor sets the scissor rectangle
func (vb *VulkanBackend) SetScissor(left, top, right, bottom int) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.scissor = vk.Rect2D{
		Offset: vk.Offset2D{X: int32(left), Y: int32(top)},
		Extent: vk.Extent2D{Width: uint32(right - left), Height: uint32(bottom - top)},
	}

	vb.software.SetScissor(left, top, right, bottom)
}

// FlushTriangles renders all triangles
func (vb *VulkanBackend) FlushTriangles(triangles []VoodooTriangle) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	if !vb.initialized || len(triangles) == 0 {
		vb.software.FlushTriangles(triangles)
		return
	}

	// Convert triangles to Vulkan vertices
	vertices := make([]VulkanVertex, 0, len(triangles)*3)
	for _, tri := range triangles {
		for _, v := range tri.Vertices {
			// Convert from Voodoo screen coords to NDC
			ndcX := (v.X/float32(vb.width))*2.0 - 1.0
			ndcY := (v.Y/float32(vb.height))*2.0 - 1.0

			// Normalize Z to Vulkan depth range [0, 1]
			// Voodoo uses larger Z values; divide by max expected Z to normalize
			// Use 65536 as max (common depth buffer range)
			ndcZ := v.Z / 65536.0
			if ndcZ < 0 {
				ndcZ = 0
			} else if ndcZ > 1 {
				ndcZ = 1
			}

			vertices = append(vertices, VulkanVertex{
				Position: [3]float32{ndcX, ndcY, ndcZ},
				Color:    [4]float32{v.R, v.G, v.B, v.A},
			})
		}
	}

	// Upload vertices to buffer
	var data unsafe.Pointer
	vk.MapMemory(vb.device, vb.vertexBufferMemory, 0, vk.DeviceSize(len(vertices)*int(unsafe.Sizeof(VulkanVertex{}))), 0, &data)
	vk.Memcopy(data, sliceToBytes(vertices))
	vk.UnmapMemory(vb.device, vb.vertexBufferMemory)

	// Wait for previous frame
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))
	vk.ResetFences(vb.device, 1, []vk.Fence{vb.fence})

	// Record command buffer
	vk.ResetCommandBuffer(vb.commandBuffer, 0)

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	vk.BeginCommandBuffer(vb.commandBuffer, &beginInfo)

	// Begin render pass with stored clear color
	clearValues := []vk.ClearValue{
		vk.NewClearValue([]float32{vb.clearColor[0], vb.clearColor[1], vb.clearColor[2], vb.clearColor[3]}),
		vk.NewClearDepthStencil(1.0, 0),
	}

	renderPassBegin := vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass:      vb.renderPass,
		Framebuffer:     vb.framebuffer,
		RenderArea:      vk.Rect2D{Offset: vk.Offset2D{X: 0, Y: 0}, Extent: vk.Extent2D{Width: uint32(vb.width), Height: uint32(vb.height)}},
		ClearValueCount: uint32(len(clearValues)),
		PClearValues:    clearValues,
	}

	vk.CmdBeginRenderPass(vb.commandBuffer, &renderPassBegin, vk.SubpassContentsInline)
	vk.CmdBindPipeline(vb.commandBuffer, vk.PipelineBindPointGraphics, vb.pipeline)

	// Set dynamic scissor
	vk.CmdSetScissor(vb.commandBuffer, 0, 1, []vk.Rect2D{vb.scissor})

	// Bind vertex buffer and draw
	offsets := []vk.DeviceSize{0}
	vk.CmdBindVertexBuffers(vb.commandBuffer, 0, 1, []vk.Buffer{vb.vertexBuffer}, offsets)
	vk.CmdDraw(vb.commandBuffer, uint32(len(vertices)), 1, 0, 0)

	vk.CmdEndRenderPass(vb.commandBuffer)
	vk.EndCommandBuffer(vb.commandBuffer)

	// Submit
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vk.CommandBuffer{vb.commandBuffer},
	}

	vk.QueueSubmit(vb.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vb.fence)
}

// ClearFramebuffer clears the framebuffer
func (vb *VulkanBackend) ClearFramebuffer(color uint32) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	vb.software.ClearFramebuffer(color)

	// Store clear color for Vulkan (ARGB format to RGBA floats)
	vb.clearColor[0] = float32((color>>16)&0xFF) / 255.0 // R
	vb.clearColor[1] = float32((color>>8)&0xFF) / 255.0  // G
	vb.clearColor[2] = float32(color&0xFF) / 255.0       // B
	vb.clearColor[3] = 1.0                               // A (opaque)
	vb.needsClear = true
}

// SwapBuffers presents the frame
func (vb *VulkanBackend) SwapBuffers(waitVSync bool) {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	if !vb.initialized {
		vb.software.SwapBuffers(waitVSync)
		return
	}

	// Wait for rendering to complete
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))

	// Read back framebuffer
	vb.readbackFramebuffer()

	vb.software.SwapBuffers(waitVSync)
}

// readbackFramebuffer copies the rendered image to CPU memory
func (vb *VulkanBackend) readbackFramebuffer() {
	vk.ResetFences(vb.device, 1, []vk.Fence{vb.fence})
	vk.ResetCommandBuffer(vb.commandBuffer, 0)

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	vk.BeginCommandBuffer(vb.commandBuffer, &beginInfo)

	// Copy image to staging buffer
	region := vk.BufferImageCopy{
		BufferOffset:      0,
		BufferRowLength:   0,
		BufferImageHeight: 0,
		ImageSubresource: vk.ImageSubresourceLayers{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			MipLevel:       0,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
		ImageOffset: vk.Offset3D{X: 0, Y: 0, Z: 0},
		ImageExtent: vk.Extent3D{Width: uint32(vb.width), Height: uint32(vb.height), Depth: 1},
	}

	vk.CmdCopyImageToBuffer(vb.commandBuffer, vb.colorImage, vk.ImageLayoutTransferSrcOptimal, vb.stagingBuffer, 1, []vk.BufferImageCopy{region})

	vk.EndCommandBuffer(vb.commandBuffer)

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vk.CommandBuffer{vb.commandBuffer},
	}

	vk.QueueSubmit(vb.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vb.fence)
	vk.WaitForFences(vb.device, 1, []vk.Fence{vb.fence}, vk.True, ^uint64(0))

	// Map staging buffer and copy to output
	var data unsafe.Pointer
	vk.MapMemory(vb.device, vb.stagingBufferMemory, 0, vk.DeviceSize(len(vb.outputFrame)), 0, &data)
	copy(vb.outputFrame, (*[1 << 30]byte)(data)[:len(vb.outputFrame)])
	vk.UnmapMemory(vb.device, vb.stagingBufferMemory)
}

// GetFrame returns the rendered frame
func (vb *VulkanBackend) GetFrame() []byte {
	vb.mutex.RLock()
	defer vb.mutex.RUnlock()

	if vb.initialized {
		return vb.outputFrame
	}
	return vb.software.GetFrame()
}

// Destroy cleans up all resources
func (vb *VulkanBackend) Destroy() {
	vb.mutex.Lock()
	defer vb.mutex.Unlock()

	if vb.initialized {
		vk.DeviceWaitIdle(vb.device)

		vb.destroyFence()
		vb.destroyStagingBuffer()
		vb.destroyVertexBuffer()
		vb.destroyPipeline()
		vb.destroyFramebuffer()
		vb.destroyRenderPass()
		vb.destroyOffscreenImages()
		vb.destroyCommandPool()
		vb.destroyDevice()
		vb.destroyInstance()
	}

	if vb.software != nil {
		vb.software.Destroy()
	}
}

// Cleanup helpers
func (vb *VulkanBackend) destroyInstance() {
	if vb.instance != nil {
		vk.DestroyInstance(vb.instance, nil)
		vb.instance = nil
	}
}

func (vb *VulkanBackend) destroyDevice() {
	if vb.device != nil {
		vk.DestroyDevice(vb.device, nil)
		vb.device = nil
	}
}

func (vb *VulkanBackend) destroyCommandPool() {
	if vb.commandPool != vk.NullCommandPool {
		vk.DestroyCommandPool(vb.device, vb.commandPool, nil)
		vb.commandPool = vk.NullCommandPool
	}
}

func (vb *VulkanBackend) destroyOffscreenImages() {
	if vb.colorImageView != vk.NullImageView {
		vk.DestroyImageView(vb.device, vb.colorImageView, nil)
	}
	if vb.colorImage != vk.NullImage {
		vk.DestroyImage(vb.device, vb.colorImage, nil)
	}
	if vb.colorImageMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.colorImageMemory, nil)
	}
	if vb.depthImageView != vk.NullImageView {
		vk.DestroyImageView(vb.device, vb.depthImageView, nil)
	}
	if vb.depthImage != vk.NullImage {
		vk.DestroyImage(vb.device, vb.depthImage, nil)
	}
	if vb.depthImageMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.depthImageMemory, nil)
	}
}

func (vb *VulkanBackend) destroyRenderPass() {
	if vb.renderPass != vk.NullRenderPass {
		vk.DestroyRenderPass(vb.device, vb.renderPass, nil)
		vb.renderPass = vk.NullRenderPass
	}
}

func (vb *VulkanBackend) destroyFramebuffer() {
	if vb.framebuffer != vk.NullFramebuffer {
		vk.DestroyFramebuffer(vb.device, vb.framebuffer, nil)
		vb.framebuffer = vk.NullFramebuffer
	}
}

func (vb *VulkanBackend) destroyPipeline() {
	if vb.pipeline != vk.NullPipeline {
		vk.DestroyPipeline(vb.device, vb.pipeline, nil)
		vb.pipeline = vk.NullPipeline
	}
	if vb.pipelineLayout != vk.NullPipelineLayout {
		vk.DestroyPipelineLayout(vb.device, vb.pipelineLayout, nil)
		vb.pipelineLayout = vk.NullPipelineLayout
	}
	if vb.vertShaderModule != vk.NullShaderModule {
		vk.DestroyShaderModule(vb.device, vb.vertShaderModule, nil)
	}
	if vb.fragShaderModule != vk.NullShaderModule {
		vk.DestroyShaderModule(vb.device, vb.fragShaderModule, nil)
	}
}

func (vb *VulkanBackend) destroyVertexBuffer() {
	if vb.vertexBuffer != vk.NullBuffer {
		vk.DestroyBuffer(vb.device, vb.vertexBuffer, nil)
	}
	if vb.vertexBufferMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.vertexBufferMemory, nil)
	}
}

func (vb *VulkanBackend) destroyStagingBuffer() {
	if vb.stagingBuffer != vk.NullBuffer {
		vk.DestroyBuffer(vb.device, vb.stagingBuffer, nil)
	}
	if vb.stagingBufferMemory != vk.NullDeviceMemory {
		vk.FreeMemory(vb.device, vb.stagingBufferMemory, nil)
	}
}

func (vb *VulkanBackend) destroyFence() {
	if vb.fence != vk.NullFence {
		vk.DestroyFence(vb.device, vb.fence, nil)
		vb.fence = vk.NullFence
	}
}

// Helper functions for Vulkan

// safeString creates a null-terminated string for Vulkan
func safeString(s string) string {
	return s + "\x00"
}

// sliceUint32 converts a byte slice to a uint32 slice for SPIR-V
func sliceUint32(data []byte) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&data[0])), len(data)/4)
}

// sliceToBytes converts a vertex slice to bytes
func sliceToBytes(vertices []VulkanVertex) []byte {
	size := len(vertices) * int(unsafe.Sizeof(VulkanVertex{}))
	return unsafe.Slice((*byte)(unsafe.Pointer(&vertices[0])), size)
}
