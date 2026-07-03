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
	vb, ok := v.backend.(*VulkanBackend)
	if !ok {
		t.Fatalf("backend type = %T, want *VulkanBackend", v.backend)
	}
	if vb.software == nil {
		t.Fatal("VulkanBackend has nil software backend")
	}
	return vb.software
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
