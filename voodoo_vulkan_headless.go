//go:build headless

package main

func init() {
	compiledFeatures = append(compiledFeatures, "voodoo:headless")
}

// VulkanBackend wraps VoodooSoftwareBackend in headless builds.
// Uses the same type name so the rest of the codebase compiles unchanged.
type VulkanBackend struct {
	software *VoodooSoftwareBackend
}

func NewVulkanBackend() (*VulkanBackend, error) {
	return &VulkanBackend{
		software: NewVoodooSoftwareBackend(),
	}, nil
}

func (vb *VulkanBackend) Init(width, height int) error {
	return vb.software.Init(width, height)
}

func (vb *VulkanBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	return vb.software.UpdatePipelineState(fbzMode, alphaMode)
}

func (vb *VulkanBackend) SetScissor(left, top, right, bottom int) {
	vb.software.SetScissor(left, top, right, bottom)
}

func (vb *VulkanBackend) SetChromaKey(chromaKey uint32) {
	vb.software.SetChromaKey(chromaKey)
}

func (vb *VulkanBackend) SetTextureData(width, height int, data []byte, format int) {
	vb.software.SetTextureData(width, height, data, format)
}

func (vb *VulkanBackend) SetTextureEnabled(enabled bool) {
	vb.software.SetTextureEnabled(enabled)
}

func (vb *VulkanBackend) SetTextureWrapMode(clampS, clampT bool) {
	vb.software.SetTextureWrapMode(clampS, clampT)
}

func (vb *VulkanBackend) SetColorPath(fbzColorPath uint32) {
	vb.software.SetColorPath(fbzColorPath)
}

func (vb *VulkanBackend) SetFogState(fogMode, fogColor uint32) {
	vb.software.SetFogState(fogMode, fogColor)
}

func (vb *VulkanBackend) FlushTriangles(triangles []VoodooTriangle) {
	vb.software.FlushTriangles(triangles)
}

func (vb *VulkanBackend) ClearFramebuffer(color uint32) {
	vb.software.ClearFramebuffer(color)
}

func (vb *VulkanBackend) SwapBuffers(waitVSync bool) {
	vb.software.SwapBuffers(waitVSync)
}

func (vb *VulkanBackend) GetFrame() []byte {
	return vb.software.GetFrame()
}

func (vb *VulkanBackend) Destroy() {
	vb.software.Destroy()
}
