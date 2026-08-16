// test_helpers_shared_test.go - test helpers shared by headless and
// display test builds. These were previously defined in headless-tagged
// files, which broke compilation of the default-tag test binary.

package main

import (
	"strings"
	"testing"
)

func monitorText(lines []OutputLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func testVoodooSoftwareBackend(t *testing.T, v *VoodooEngine) *VoodooSoftwareBackend {
	t.Helper()
	// Headless Voodoo already uses the software backend. Replacing it after the
	// programme has run discards state that was uploaded through MMIO (notably
	// TEXTURE UPLOAD), making the subsequent inspection test the wrong backend.
	if sw, ok := v.backend.(*VoodooSoftwareBackend); ok {
		return sw
	}
	// The Vulkan backend no longer embeds the reference rasteriser;
	// conformance tests swap the engine onto a standalone one so the
	// MMIO-driven state and draws land in its buffers directly.
	sw := NewVoodooSoftwareBackend()
	if err := sw.Init(640, 480); err != nil {
		t.Fatalf("software backend init: %v", err)
	}
	// The reference examples execute before the test can select the software
	// backend. Preserve the engine's shadow state when replacing Vulkan, in
	// particular the texture uploaded through the MMIO register path.
	v.mu.Lock()
	textureWidth, textureHeight := v.textureWidth, v.textureHeight
	textureMode, fbzMode, alphaMode := v.textureMode, v.fbzMode, v.alphaMode
	colorPath := v.fbzColorPath
	var texture *VoodooTexture
	if v.currentTexture != nil {
		copyData := append([]byte(nil), v.currentTexture.Data...)
		texture = &VoodooTexture{Width: v.currentTexture.Width, Height: v.currentTexture.Height, Format: v.currentTexture.Format, Data: copyData}
	}
	v.mu.Unlock()
	if err := v.SetBackend(sw); err != nil {
		t.Fatalf("install software backend: %v", err)
	}
	if texture != nil {
		sw.SetTextureData(texture.Width, texture.Height, texture.Data, texture.Format)
	} else if textureWidth > 0 && textureHeight > 0 {
		v.mu.Lock()
		size, ok := v.textureUploadSizeLocked()
		data := []byte(nil)
		if ok {
			data = append([]byte(nil), v.textureMemory[:size]...)
		}
		v.mu.Unlock()
		if ok {
			sw.SetTextureData(textureWidth, textureHeight, data, int((textureMode>>8)&0xF))
		}
	}
	sw.UpdatePipelineState(fbzMode, alphaMode)
	sw.SetColorPath(colorPath)
	sw.SetTextureMode(textureMode)
	sw.SetTextureEnabled(textureMode&VOODOO_TEX_ENABLE != 0)
	t.Cleanup(sw.Destroy)
	return sw
}

func installRealULA(bus *MachineBus) *ULAEngine {
	ula := NewULAEngine(bus)
	bus.MapIO(ULA_BASE, ULA_REG_END, ula.HandleRead, ula.HandleWrite)
	bus.MapIOByteRead(ULA_BASE, ULA_REG_END, ula.HandleRead8)
	bus.MapIOByte(ULA_BASE, ULA_REG_END, ula.HandleWrite8)
	bus.MapIO(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleBusVRAMRead, ula.HandleBusVRAMWrite)
	bus.MapIOByteRead(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleRead8)
	bus.MapIOByte(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleWrite8)
	bus.MapIO64(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END, ula.HandleRead64, ula.HandleWrite64)
	bus.MapIOWideWriteFanout(ULA_VRAM_AP_BASE, ULA_VRAM_AP_END)
	return ula
}
