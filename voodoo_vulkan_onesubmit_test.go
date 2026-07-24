//go:build !headless && !novulkan && !js

// voodoo_vulkan_onesubmit_test.go - Slice 6: native Vulkan one-submit present gate.
//
// Real VulkanBackend only; skips without a working device. Proves the folded
// one-submission present (render + readback in a single QueueSubmit) yields a
// bit-identical framebuffer to the legacy two-submission path, for both an empty
// (clear-only) frame and a frame with a drawn triangle.

package main

import (
	"bytes"
	"testing"
)

// renderPresentReadback renders one present frame for the given one-submit
// setting and returns a copy of the readback framebuffer.
func renderPresentReadback(t *testing.T, oneSubmit string, w, h int, clear uint32, tris []VoodooTriangle) []byte {
	t.Helper()
	t.Setenv("IE_VOODOO_ONE_SUBMIT", oneSubmit)
	// This gate isolates the one-submit fold; async present is exercised by the
	// Slice 7 tests. Force synchronous readback so GetFrame returns this frame.
	t.Setenv("IE_VOODOO_ASYNC_PRESENT", "0")
	vb := newInitedVulkanBackendForTest(t, w, h)
	defer vb.Destroy()

	vb.ClearFramebuffer(clear)
	// Route through the present path exactly as the swap worker does.
	vb.FlushTrianglesForPresent(tris)
	vb.SwapBuffers(false)
	return append([]byte(nil), vb.GetFrame()...)
}

func fullscreenTriangle(w, h int) []VoodooTriangle {
	return []VoodooTriangle{{
		Vertices: [3]VoodooVertex{
			{X: 0, Y: 0, Z: 0.5, R: 1, G: 0, B: 0, A: 1, W: 1},
			{X: float32(w), Y: 0, Z: 0.5, R: 1, G: 0, B: 0, A: 1, W: 1},
			{X: 0, Y: float32(h), Z: 0.5, R: 1, G: 0, B: 0, A: 1, W: 1},
		},
	}}
}

// TestVulkanOneSubmit_MatchesTwoSubmitEmptyFrame proves a clear-only present is
// bit-identical between the one-submit and two-submit paths.
func TestVulkanOneSubmit_MatchesTwoSubmitEmptyFrame(t *testing.T) {
	const w, h = 64, 48
	const clear = uint32(0xFF3060C0)
	one := renderPresentReadback(t, "1", w, h, clear, nil)
	two := renderPresentReadback(t, "0", w, h, clear, nil)
	if !bytes.Equal(one, two) {
		t.Fatalf("empty-frame readback differs: one-submit vs two-submit not bit-identical")
	}
	// A clear must actually have happened (frame not all zero).
	if bytes.Equal(one, make([]byte, len(one))) {
		t.Fatal("one-submit empty frame is all zero; clear did not reach the image")
	}
}

// TestVulkanOneSubmit_MatchesTwoSubmitTriangle proves a drawn frame is
// bit-identical between the two paths, exercising the render->copy barrier.
func TestVulkanOneSubmit_MatchesTwoSubmitTriangle(t *testing.T) {
	const w, h = 64, 48
	const clear = uint32(0xFF102030)
	one := renderPresentReadback(t, "1", w, h, clear, fullscreenTriangle(w, h))
	two := renderPresentReadback(t, "0", w, h, clear, fullscreenTriangle(w, h))
	if !bytes.Equal(one, two) {
		t.Fatalf("triangle readback differs: one-submit vs two-submit not bit-identical")
	}
}
