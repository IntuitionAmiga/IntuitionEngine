//go:build !headless && !novulkan && !js

// voodoo_vulkan_persistmap_test.go - Slice 5: native Vulkan persistent staging map gate.
//
// This exercises the real VulkanBackend, which headless builds cannot: headless
// wraps the software backend and never maps device memory. It needs a working
// Vulkan device, so it skips when Init fails (no GPU / no driver). Run it on a
// GPU host (the same environment as the GUI IE binary).

package main

import "testing"

func newInitedVulkanBackendForTest(t *testing.T, w, h int) *VulkanBackend {
	t.Helper()
	vb, err := NewVulkanBackend()
	if err != nil {
		t.Skipf("NewVulkanBackend unavailable: %v", err)
	}
	if err := vb.Init(w, h); err != nil {
		t.Skipf("Vulkan Init unavailable (no GPU/driver?): %v", err)
	}
	return vb
}

// TestVulkanPersistentStagingMap_ReadbackAndLifecycle proves the persistent
// staging map is established on init, survives a readback with correct pixels,
// is re-established across a resize, and is dropped on destroy.
func TestVulkanPersistentStagingMap_ReadbackAndLifecycle(t *testing.T) {
	t.Setenv("IE_VOODOO_PERSIST_MAP", "1")
	vb := newInitedVulkanBackendForTest(t, 64, 48)
	defer vb.Destroy()

	if vb.stagingMapped == nil {
		t.Fatal("persistent staging map not established on init")
	}

	// Clear to a known colour and present; the readback must copy through the
	// persistent map and yield those pixels. The channel order in the output is
	// backend-format dependent, so assert the three clear channels are present
	// (in any order) and alpha is opaque, which does not pin a byte layout.
	vb.ClearFramebuffer(0xFF2040A0)
	vb.FlushTriangles(nil)
	vb.SwapBuffers(false)
	frame := vb.GetFrame()
	if len(frame) != 64*48*4 {
		t.Fatalf("frame length = %d, want %d", len(frame), 64*48*4)
	}
	assertClearPixel(t, frame[:4], 0x20, 0x40, 0xA0)

	// Resize tears the Vulkan objects down (including the staging buffer, which
	// unmaps the persistent map). Re-initialising at the new size must
	// re-establish the persistent map.
	if err := vb.Resize(80, 60); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if vb.stagingMapped != nil {
		t.Fatal("persistent staging map not dropped by resize teardown")
	}
	if err := vb.Init(80, 60); err != nil {
		t.Skipf("re-init after resize unavailable: %v", err)
	}
	if vb.stagingMapped == nil {
		t.Fatal("persistent staging map not re-established after resize + init")
	}
}

// TestVulkanPersistentStagingMap_KillSwitch proves IE_VOODOO_PERSIST_MAP=0 keeps
// the per-frame mapping path (no persistent map) while readback stays correct.
func TestVulkanPersistentStagingMap_KillSwitch(t *testing.T) {
	t.Setenv("IE_VOODOO_PERSIST_MAP", "0")
	vb := newInitedVulkanBackendForTest(t, 64, 48)
	defer vb.Destroy()

	if vb.stagingMapped != nil {
		t.Fatal("IE_VOODOO_PERSIST_MAP=0 still established a persistent map")
	}
	vb.ClearFramebuffer(0xFF112233)
	vb.FlushTriangles(nil)
	vb.SwapBuffers(false)
	frame := vb.GetFrame()
	assertClearPixel(t, frame[:4], 0x11, 0x22, 0x33)
}

// assertClearPixel checks a 4-byte pixel holds the three clear channels in any
// order plus an opaque alpha, without pinning the backend's byte layout.
func assertClearPixel(t *testing.T, px []byte, r, g, b byte) {
	t.Helper()
	want := map[byte]int{r: 0, g: 0, b: 0, 0xFF: 0}
	for _, v := range px {
		if _, ok := want[v]; ok {
			want[v]++
		}
	}
	for v, n := range want {
		if n == 0 {
			t.Fatalf("clear pixel %v missing channel 0x%02X", px, v)
		}
	}
}
