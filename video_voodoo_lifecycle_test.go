//go:build headless

package main

import "testing"

func TestVoodoo_Reset_ClearsBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.HandleWrite(VOODOO_COLOR0, 0xFF112233)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	v.Reset()

	frame := v.backend.GetFrame()
	for i, b := range frame {
		if b != 0 {
			t.Fatalf("backend frame byte %d = %#02x after reset, want 0", i, b)
		}
	}
}

func TestVoodoo_VideoDim_SameSizeDoesNotReinitializeBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_COLOR0, 0xFF112233)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_VIDEO_DIM, uint32(VOODOO_DEFAULT_WIDTH)<<16|uint32(VOODOO_DEFAULT_HEIGHT))

	if got := sw.colorBuffer[:4]; got[0] != 0x11 || got[1] != 0x22 || got[2] != 0x33 || got[3] != 0xFF {
		t.Fatalf("same-size VIDEO_DIM reinitialized backend, first pixel = %v", got)
	}
}

func TestVoodoo_Reset_ClearsStatusState(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.busy = true
	v.swapPending = true
	v.vretrace.Store(1)

	v.Reset()

	if v.busy || v.swapPending || v.vretrace.Load() != 0 {
		t.Fatalf("status state after reset: busy=%v swapPending=%v vretrace=%d", v.busy, v.swapPending, v.vretrace.Load())
	}
}

func TestVoodoo_Status_FIFOFull_ReportsNoMemFIFO(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	for len(v.triangleBatch) < VOODOO_MAX_BATCH_TRIANGLES {
		v.triangleBatch = append(v.triangleBatch, VoodooTriangle{})
	}
	status := v.HandleRead(VOODOO_STATUS)
	if status&VOODOO_STATUS_MEMFIFO != 0 {
		t.Fatalf("full FIFO status = %#08x, MEMFIFO bits should be zero", status)
	}
}

func TestVoodoo_Status_BusyBit(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	v.busy = true
	status := v.HandleRead(VOODOO_STATUS)
	if status&(VOODOO_STATUS_FBI_BUSY|VOODOO_STATUS_SST_BUSY) != (VOODOO_STATUS_FBI_BUSY | VOODOO_STATUS_SST_BUSY) {
		t.Fatalf("busy status = %#08x, want FBI and SST busy bits", status)
	}
}

func TestVoodoo_VBlank_AndSwapCompleteCallbacks(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	var vblankCount, swapCount, fifoEmptyCount int
	v.OnVBlank = func() { vblankCount++ }
	v.OnSwapComplete = func() { swapCount++ }
	v.OnFIFOEmpty = func() { fifoEmptyCount++ }

	v.TickFrame()
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	if vblankCount != 1 {
		t.Fatalf("vblank callback count = %d, want 1", vblankCount)
	}
	if swapCount != 1 {
		t.Fatalf("swap callback count = %d, want 1", swapCount)
	}
	if fifoEmptyCount != 1 {
		t.Fatalf("fifo-empty callback count = %d, want 1", fifoEmptyCount)
	}
}
