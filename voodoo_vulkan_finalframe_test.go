//go:build !headless && !novulkan && !js

// voodoo_vulkan_finalframe_test.go - Slice 7 P1: the final frame is published.
//
// Regression for the async-present tail bug: with IE_VOODOO_ASYNC_PRESENT on,
// SwapBuffers returns while the swapped frame is still in flight, and the swap
// worker publishes the previous frame. If the guest swaps once and then stops,
// no later flush drains the in-flight frame, so it must be completed when the
// pipeline drains - otherwise the display stays on the previous (blank) frame
// and WaitSwapIdle reports idle a frame behind. Needs a real Vulkan device.

package main

import "testing"

func TestVulkanAsyncPresent_FinalFramePublished(t *testing.T) {
	t.Setenv("IE_VOODOO_ASYNC_PRESENT", "1")
	t.Setenv("IE_VOODOO_ONE_SUBMIT", "1")

	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Skipf("Voodoo/Vulkan unavailable (no GPU/driver?): %v", err)
	}
	defer v.Destroy()

	// Fill a distinct colour and present exactly one frame, then stop.
	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)
	v.HandleWrite(VOODOO_COLOR0, 0xFF112233)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	// WaitSwapIdle must not report idle until the frame just swapped is
	// published, so GetFrame afterwards must show it, not the previous blank.
	v.WaitSwapIdle()
	frame := v.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame returned nil after a swap")
	}
	assertClearPixel(t, frame[:4], 0x11, 0x22, 0x33)

	// Exactly one publication must have occurred. A speculative publish of the
	// previous (blank) frame before the drain would advance the generation
	// twice, publishing a false intermediate blank frame (Slice 7 P1).
	if gen := v.frameGen.Load(); gen != 1 {
		t.Fatalf("frameGen = %d after one swap, want 1 (a value of 2 means a blank frame was published before the real one)", gen)
	}
}

// TestVulkanAsyncPresent_ContinuousPublishesEachFrameOnce proves that over a run
// of swaps each frame is published exactly once, when its readback completes -
// no speculative publish of the previous frame and no blank intermediate. After
// draining, the generation count must equal the number of swaps.
func TestVulkanAsyncPresent_ContinuousPublishesEachFrameOnce(t *testing.T) {
	t.Setenv("IE_VOODOO_ASYNC_PRESENT", "1")
	t.Setenv("IE_VOODOO_ONE_SUBMIT", "1")

	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Skipf("Voodoo/Vulkan unavailable (no GPU/driver?): %v", err)
	}
	defer v.Destroy()

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)

	const swaps = 5
	colors := []uint32{0xFF010203, 0xFF040506, 0xFF070809, 0xFF0A0B0C, 0xFF0D0E0F}
	for i := 0; i < swaps; i++ {
		v.HandleWrite(VOODOO_COLOR0, colors[i])
		v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
		v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	}
	v.WaitSwapIdle()

	// Each swap's frame drains and publishes exactly once, so the generation
	// count equals the swap count: no blank and no duplicate publications.
	if gen := v.frameGen.Load(); gen != swaps {
		t.Fatalf("frameGen = %d after %d swaps, want %d (extra generations mean speculative/blank publications)", gen, swaps, swaps)
	}
	// The last frame drawn is the one shown.
	frame := v.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame returned nil")
	}
	assertClearPixel(t, frame[:4], 0x0D, 0x0E, 0x0F)
}
