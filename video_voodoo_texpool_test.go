// video_voodoo_texpool_test.go - Slice 11: pooled+fused texture upload differential

package main

import (
	"encoding/binary"
	"testing"
)

// uploadTextureVia runs textureUploadDataLocked for a given IE_VOODOO_TEXPOOL
// setting over identical guest bytes and returns the retained upload buffer and
// a copy of the resident texture memory prefix.
func uploadTextureVia(t *testing.T, texpool string, w, h int, guest []byte) (data []byte, texmem []byte) {
	t.Helper()
	t.Setenv("IE_VOODOO_TEXPOOL", texpool)
	bus := NewMachineBus()
	v, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine: %v", err)
	}
	const srcAddr = uint32(0x00100000)
	for i, b := range guest {
		bus.Write8(srcAddr+uint32(i), b)
	}
	v.texSrcPtr = srcAddr
	v.texSrcBytes = uint32(len(guest))
	v.textureWidth = w
	v.textureHeight = h

	size := w * h * 4
	data = v.textureUploadDataLocked(size)
	texmem = append([]byte(nil), v.textureMemory[:size]...)
	return data, texmem
}

// TestVoodooTexPool_MatchesUnpooledByteForByte proves the pooled+fused upload
// path (default) produces byte-identical results to the legacy allocate-read-
// swap-copy path, for both the returned buffer and resident texture memory,
// across sizes and re-uploads.
func TestVoodooTexPool_MatchesUnpooledByteForByte(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{2, 2}, {4, 3}, {8, 8}, {16, 9}} {
		size := tc.w * tc.h * 4
		guest := make([]byte, size)
		for i := range guest {
			guest[i] = byte(i*7 + 3)
		}

		offData, offMem := uploadTextureVia(t, "0", tc.w, tc.h, guest)
		onData, onMem := uploadTextureVia(t, "1", tc.w, tc.h, guest)

		if string(offData) != string(onData) {
			t.Fatalf("%dx%d: pooled upload buffer differs from unpooled", tc.w, tc.h)
		}
		if string(offMem) != string(onMem) {
			t.Fatalf("%dx%d: pooled texture memory differs from unpooled", tc.w, tc.h)
		}

		// Sanity: the swap really is big-endian to little-endian.
		if size >= 4 {
			word := binary.BigEndian.Uint32(guest[:4])
			if got := binary.LittleEndian.Uint32(onData[:4]); got != word {
				t.Fatalf("%dx%d: BE->LE swap wrong: got 0x%08X want 0x%08X", tc.w, tc.h, got, word)
			}
		}
	}
}

// TestVoodooTexPool_ReusesScratchAcrossUploads proves repeated uploads reuse the
// pooled scratch (no growth once large enough) and stay correct.
func TestVoodooTexPool_ReusesScratchAcrossUploads(t *testing.T) {
	t.Setenv("IE_VOODOO_TEXPOOL", "1")
	bus := NewMachineBus()
	v, err := NewVoodooEngine(bus)
	if err != nil {
		t.Fatalf("NewVoodooEngine: %v", err)
	}
	const srcAddr = uint32(0x00100000)
	const w, h = 8, 8
	size := w * h * 4
	guest := make([]byte, size)
	for i := range guest {
		guest[i] = byte(255 - i)
	}
	for i, b := range guest {
		bus.Write8(srcAddr+uint32(i), b)
	}
	v.texSrcPtr = srcAddr
	v.texSrcBytes = uint32(size)
	v.textureWidth = w
	v.textureHeight = h

	_ = v.textureUploadDataLocked(size)
	capAfterFirst := cap(v.texUploadScratch)
	for range 5 {
		data := v.textureUploadDataLocked(size)
		if binary.LittleEndian.Uint32(data[:4]) != binary.BigEndian.Uint32(guest[:4]) {
			t.Fatal("re-upload produced wrong bytes")
		}
	}
	if cap(v.texUploadScratch) != capAfterFirst {
		t.Fatalf("scratch grew across same-size uploads: %d -> %d", capAfterFirst, cap(v.texUploadScratch))
	}
}
