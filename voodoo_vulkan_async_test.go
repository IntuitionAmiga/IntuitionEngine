//go:build !headless && !novulkan && !js

// voodoo_vulkan_async_test.go - Slice 7: native Vulkan async present contract gate.
//
// Real VulkanBackend only; skips without a working device. Proves the deferred-
// wait double-buffer contract: after swapping frame N the consumer still
// observes N-1 (N is in flight), and a synchronous FinishPendingReadback forces
// N to become observable. The kill switch restores immediate observation.

package main

import "testing"

func presentClearFrame(vb *VulkanBackend, color uint32) {
	vb.ClearFramebuffer(color)
	vb.FlushTrianglesForPresent(nil)
	vb.SwapBuffers(false)
}

func copyFrame(vb *VulkanBackend) []byte {
	return append([]byte(nil), vb.GetFrame()...)
}

// TestVulkanAsyncPresent_NMinus1Contract proves that under async present the
// frame just swapped is in flight (consumer sees the previous frame) until the
// next frame drains it or a synchronous finish forces it.
func TestVulkanAsyncPresent_NMinus1Contract(t *testing.T) {
	t.Setenv("IE_VOODOO_ASYNC_PRESENT", "1")
	t.Setenv("IE_VOODOO_ONE_SUBMIT", "1")
	const w, h = 64, 48
	const c1 = uint32(0xFF112233)
	const c2 = uint32(0xFF445566)
	vb := newInitedVulkanBackendForTest(t, w, h)
	defer vb.Destroy()

	// Present frame 1 (c1). It is in flight; nothing has been drained yet, so
	// the output frame is still the initial (blank) frame.
	presentClearFrame(vb, c1)
	f0 := copyFrame(vb)
	if f0[3] != 0x00 {
		t.Fatalf("after first async present, output alpha = 0x%02X, want 0x00 (frame still in flight)", f0[3])
	}

	// Present frame 2 (c2). Its record-start fence wait drains frame 1, so the
	// consumer now observes c1 (N-1) while c2 (N) is in flight.
	presentClearFrame(vb, c2)
	f1 := copyFrame(vb)
	assertClearPixel(t, f1[:4], 0x11, 0x22, 0x33)

	// A synchronous finish forces frame 2 (N) to become observable.
	vb.FinishPendingReadback()
	f2 := copyFrame(vb)
	assertClearPixel(t, f2[:4], 0x44, 0x55, 0x66)
}

// TestVulkanAsyncPresent_ResetClearsPending proves that a Reset while a present
// is still in flight discards the pending async readback, so the first
// post-reset publication does not expose the stale pre-reset frame.
func TestVulkanAsyncPresent_ResetClearsPending(t *testing.T) {
	t.Setenv("IE_VOODOO_ASYNC_PRESENT", "1")
	t.Setenv("IE_VOODOO_ONE_SUBMIT", "1")
	const w, h = 64, 48
	const stale = uint32(0xFFAABBCC)
	const fresh = uint32(0xFF010203)
	vb := newInitedVulkanBackendForTest(t, w, h)
	defer vb.Destroy()

	// Present the stale frame; it stays in flight (async pending).
	presentClearFrame(vb, stale)

	// Reset while the present is pending. It must clear the pending state.
	vb.Reset()
	if vb.asyncFencePending {
		t.Fatal("Reset left asyncFencePending set")
	}
	f0 := copyFrame(vb)
	if f0[3] != 0x00 {
		t.Fatalf("Reset did not zero the output frame: alpha 0x%02X", f0[3])
	}

	// The first post-reset present must not drain the stale pre-reset staging.
	presentClearFrame(vb, fresh)
	f1 := copyFrame(vb)
	// stale blue is 0xCC; it must not appear.
	for i := 0; i < len(f1); i += 4 {
		if f1[i] == 0xAA && f1[i+1] == 0xBB && f1[i+2] == 0xCC {
			t.Fatalf("stale pre-reset frame republished after reset at pixel %d", i/4)
		}
	}
}

// TestVulkanAsyncPresent_KillSwitchIsSynchronous proves IE_VOODOO_ASYNC_PRESENT=0
// makes the swapped frame observable immediately.
func TestVulkanAsyncPresent_KillSwitchIsSynchronous(t *testing.T) {
	t.Setenv("IE_VOODOO_ASYNC_PRESENT", "0")
	t.Setenv("IE_VOODOO_ONE_SUBMIT", "1")
	const w, h = 64, 48
	const c1 = uint32(0xFF778899)
	vb := newInitedVulkanBackendForTest(t, w, h)
	defer vb.Destroy()

	presentClearFrame(vb, c1)
	f := copyFrame(vb)
	assertClearPixel(t, f[:4], 0x77, 0x88, 0x99)
}
