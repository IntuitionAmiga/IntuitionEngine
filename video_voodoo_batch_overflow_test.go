//go:build headless

package main

import (
	"sync/atomic"
	"testing"
)

// A frame with more than VOODOO_MAX_BATCH_TRIANGLES triangles must
// render all of them: overflow flushes the batch mid-frame (without
// presenting) instead of silently dropping the excess. The mk64 title
// screen exceeds the cap with its tiled background.
func TestVoodoo_BatchOverflowRendersAllTriangles(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Fill the batch beyond capacity with off-screen sliver triangles,
	// then submit one final well-visible green triangle. Without
	// overflow flushing, the final triangle is silently dropped.
	for i := 0; i < VOODOO_MAX_BATCH_TRIANGLES+8; i++ {
		sbSubmitTri(v, 700, 470, 710, 470, 700, 478, 1, 0, 0)
	}
	sbSubmitTri(v, 100, 100, 300, 100, 100, 300, 0, 1, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	v.WaitSwapIdle()

	idx := (150*640 + 150) * 4
	if g := sw.frontBuffer[idx+1]; g < 0x80 {
		t.Fatalf("triangle after batch overflow was dropped: green=%d want >=128", g)
	}
}

// waitIdleRecorder wraps a backend and records FlushTrianglesSync
// calls — the render-only overflow flush must render AND wait for
// completion atomically (the Vulkan backend queues GPU work in
// FlushTriangles and only waits its fence at swap time; a separate
// wait call would leave a mutex gap for forwarded texture mutations).
type waitIdleRecorder struct {
	VoodooBackend
	waits int32
}

func (w *waitIdleRecorder) FlushTrianglesSync(triangles []VoodooTriangle) {
	atomic.AddInt32(&w.waits, 1)
	w.VoodooBackend.FlushTriangles(triangles)
}

func TestVoodoo_BatchOverflowWaitsForRenderIdle(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	testVoodooSoftwareBackend(t, v)
	rec := &waitIdleRecorder{VoodooBackend: v.backend}
	v.backend = rec

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, VOODOO_FBZ_RGB_WRITE)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	for i := 0; i < VOODOO_MAX_BATCH_TRIANGLES+8; i++ {
		sbSubmitTri(v, 700, 470, 710, 470, 700, 478, 1, 0, 0)
	}
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	v.WaitSwapIdle()

	if atomic.LoadInt32(&rec.waits) == 0 {
		t.Fatal("render-only overflow flush did not use the atomic render-and-wait path")
	}
}
