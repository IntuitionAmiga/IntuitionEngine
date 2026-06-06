//go:build headless || (!headless && novulkan)

package main

type softwareVoodooBackend struct {
	software *VoodooSoftwareBackend
}

func newSoftwareVoodooBackend() softwareVoodooBackend {
	return softwareVoodooBackend{software: NewVoodooSoftwareBackend()}
}

func (vb *softwareVoodooBackend) Init(width, height int) error {
	return vb.software.Init(width, height)
}

func (vb *softwareVoodooBackend) Resize(width, height int) error {
	return vb.software.Resize(width, height)
}

func (vb *softwareVoodooBackend) UpdatePipelineState(fbzMode, alphaMode uint32) error {
	return vb.software.UpdatePipelineState(fbzMode, alphaMode)
}

func (vb *softwareVoodooBackend) SetScissor(left, top, right, bottom int) {
	vb.software.SetScissor(left, top, right, bottom)
}

func (vb *softwareVoodooBackend) SetChromaKey(chromaKey uint32) {
	vb.software.SetChromaKey(chromaKey)
}

func (vb *softwareVoodooBackend) SetChromaRange(chromaRange uint32) {
	vb.software.SetChromaRange(chromaRange)
}

func (vb *softwareVoodooBackend) SetStipple(stipple uint32) {
	vb.software.SetStipple(stipple)
}

func (vb *softwareVoodooBackend) SetLFBMode(lfbMode uint32) {
	vb.software.SetLFBMode(lfbMode)
}

func (vb *softwareVoodooBackend) SetTexBase(level int, addr uint32) {
	vb.software.SetTexBase(level, addr)
}

func (vb *softwareVoodooBackend) SetTLOD(tlod uint32) {
	vb.software.SetTLOD(tlod)
}

func (vb *softwareVoodooBackend) SetSlopes(slopes VoodooSlopes, valid bool) {
	vb.software.SetSlopes(slopes, valid)
}

func (vb *softwareVoodooBackend) SetFogTableEntry(index int, value uint32) {
	vb.software.SetFogTableEntry(index, value)
}

func (vb *softwareVoodooBackend) SetPaletteEntry(index int, value uint32) {
	vb.software.SetPaletteEntry(index, value)
}

func (vb *softwareVoodooBackend) SetTextureData(width, height int, data []byte, format int) {
	vb.software.SetTextureData(width, height, data, format)
}

func (vb *softwareVoodooBackend) SetTextureMode(textureMode uint32) {
	vb.software.SetTextureMode(textureMode)
}

func (vb *softwareVoodooBackend) SetTextureEnabled(enabled bool) {
	vb.software.SetTextureEnabled(enabled)
}

func (vb *softwareVoodooBackend) SetTextureWrapMode(clampS, clampT bool) {
	vb.software.SetTextureWrapMode(clampS, clampT)
}

func (vb *softwareVoodooBackend) SetColorPath(fbzColorPath uint32) {
	vb.software.SetColorPath(fbzColorPath)
}

func (vb *softwareVoodooBackend) SetFogState(fogMode, fogColor uint32) {
	vb.software.SetFogState(fogMode, fogColor)
}

func (vb *softwareVoodooBackend) FlushTriangles(triangles []VoodooTriangle) {
	vb.software.FlushTriangles(triangles)
}

func (vb *softwareVoodooBackend) ClearFramebuffer(color uint32) {
	vb.software.ClearFramebuffer(color)
}

func (vb *softwareVoodooBackend) SwapBuffers(waitVSync bool) {
	vb.software.SwapBuffers(waitVSync)
}

func (vb *softwareVoodooBackend) GetFrame() []byte {
	return vb.software.GetFrame()
}

func (vb *softwareVoodooBackend) Reset() {
	vb.software.Reset()
}

func (vb *softwareVoodooBackend) Destroy() {
	vb.software.Destroy()
}
