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

func (vb *VulkanBackend) Resize(width, height int) error {
	return vb.software.Resize(width, height)
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

func (vb *VulkanBackend) SetChromaRange(chromaRange uint32) {
	vb.software.SetChromaRange(chromaRange)
}

func (vb *VulkanBackend) SetStipple(stipple uint32) {
	vb.software.SetStipple(stipple)
}

func (vb *VulkanBackend) SetLFBMode(lfbMode uint32) {
	vb.software.SetLFBMode(lfbMode)
}

func (vb *VulkanBackend) SetTexBase(level int, addr uint32) {
	vb.software.SetTexBase(level, addr)
}

func (vb *VulkanBackend) SetTLOD(tlod uint32) {
	vb.software.SetTLOD(tlod)
}

func (vb *VulkanBackend) SetSlopes(slopes VoodooSlopes, valid bool) {
	vb.software.SetSlopes(slopes, valid)
}

func (vb *VulkanBackend) SetFogTableEntry(index int, value uint32) {
	vb.software.SetFogTableEntry(index, value)
}

func (vb *VulkanBackend) SetPaletteEntry(index int, value uint32) {
	vb.software.SetPaletteEntry(index, value)
}

func (vb *VulkanBackend) SetTextureData(width, height int, data []byte, format int) {
	vb.software.SetTextureData(width, height, data, format)
}

func (vb *VulkanBackend) SetTextureMode(textureMode uint32) {
	vb.software.SetTextureMode(textureMode)
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

func (vb *VulkanBackend) Reset() {
	vb.software.Reset()
}

func (vb *VulkanBackend) Destroy() {
	vb.software.Destroy()
}
