//go:build headless

package main

import "fmt"

// PipelineKey mirrors the non-headless type to keep APIs consistent.
type PipelineKey struct {
	DepthTestEnable  bool
	DepthWriteEnable bool
	DepthCompareOp   int
	BlendEnable      bool
	SrcBlendFactor   int
	DstBlendFactor   int
	AlphaTestEnable  bool
	AlphaCompareOp   int
}

// VulkanBackend headless implementation: no GPU dependencies.
type VulkanBackend struct {
	width  int
	height int
	frame  []byte
}

func NewVulkanBackend() (*VulkanBackend, error) {
	return &VulkanBackend{}, nil
}

func (vb *VulkanBackend) Init(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid dimensions: %dx%d", width, height)
	}
	vb.width = width
	vb.height = height
	vb.frame = make([]byte, width*height*4)
	return nil
}

func (vb *VulkanBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error { return nil }
func (vb *VulkanBackend) SetScissor(left, top, right, bottom int)             {}
func (vb *VulkanBackend) SetChromaKey(chromaKey uint32)                       {}
func (vb *VulkanBackend) SetTextureData(width, height int, data []byte, format int) {
}
func (vb *VulkanBackend) SetTextureEnabled(enabled bool)         {}
func (vb *VulkanBackend) SetTextureWrapMode(clampS, clampT bool) {}
func (vb *VulkanBackend) SetColorPath(fbzColorPath uint32)       {}
func (vb *VulkanBackend) SetFogState(fogMode, fogColor uint32)   {}
func (vb *VulkanBackend) FlushTriangles(triangles []VoodooTriangle) {
}
func (vb *VulkanBackend) SwapBuffers(waitVSync bool) {}
func (vb *VulkanBackend) Destroy()                   {}

func (vb *VulkanBackend) ClearFramebuffer(color uint32) {
	if len(vb.frame) == 0 {
		return
	}
	r := byte((color >> 16) & 0xFF)
	g := byte((color >> 8) & 0xFF)
	b := byte(color & 0xFF)
	a := byte((color >> 24) & 0xFF)
	for i := 0; i < len(vb.frame); i += 4 {
		vb.frame[i+0] = r
		vb.frame[i+1] = g
		vb.frame[i+2] = b
		vb.frame[i+3] = a
	}
}

func (vb *VulkanBackend) GetFrame() []byte {
	return vb.frame
}
