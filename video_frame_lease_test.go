// video_frame_lease_test.go - frame lease ownership tests.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"encoding/binary"
	"testing"
)

func TestFrameLease_SubmitThenProducerWritesNextSlot_StagedFrameStable(t *testing.T) {
	ring := NewVideoFrameLeaseRing(3, 4)
	first, ok := ring.Acquire()
	if !ok {
		t.Fatal("first Acquire failed")
	}
	copy(first.Pixels(), []byte{0x11, 0x22, 0x33, 0x44})
	staged := first.Pixels()

	next, ok := ring.Acquire()
	if !ok {
		t.Fatal("second Acquire failed")
	}
	copy(next.Pixels(), []byte{0xAA, 0xBB, 0xCC, 0xDD})

	if got := binary.LittleEndian.Uint32(staged); got != 0x44332211 {
		t.Fatalf("staged frame changed after producer wrote next slot: 0x%08X", got)
	}
}

func TestFrameLease_RingReuseOnlyAfterRelease(t *testing.T) {
	ring := NewVideoFrameLeaseRing(1, 4)
	lease, ok := ring.Acquire()
	if !ok {
		t.Fatal("first Acquire failed")
	}
	if _, ok := ring.Acquire(); ok {
		t.Fatal("ring reused in-use slot before Release")
	}
	lease.Release()
	if _, ok := ring.Acquire(); !ok {
		t.Fatal("ring did not reuse slot after Release")
	}
}

func TestFrameLease_RetainDelaysReuseUntilFinalRelease(t *testing.T) {
	ring := NewVideoFrameLeaseRing(1, 4)
	lease, ok := ring.Acquire()
	if !ok {
		t.Fatal("Acquire failed")
	}
	if !lease.Retain() {
		t.Fatal("Retain failed")
	}
	lease.Release()
	if _, ok := ring.Acquire(); ok {
		t.Fatal("ring reused retained slot before final Release")
	}
	lease.Release()
	if _, ok := ring.Acquire(); !ok {
		t.Fatal("ring did not reuse slot after final Release")
	}
}

func TestFrameLease_SnapshotIsDeepCopy(t *testing.T) {
	ring := NewVideoFrameLeaseRing(1, 4)
	lease, ok := ring.Acquire()
	if !ok {
		t.Fatal("Acquire failed")
	}
	copy(lease.Pixels(), []byte{1, 2, 3, 4})
	snap := lease.Snapshot()
	lease.Pixels()[0] = 9
	if snap[0] != 1 {
		t.Fatalf("snapshot changed with lease buffer: got %d", snap[0])
	}
}

func TestFrameLease_HardwareSoftwareAlphaParity(t *testing.T) {
	ring := NewVideoFrameLeaseRing(1, 12)
	lease, ok := ring.Acquire()
	if !ok {
		t.Fatal("Acquire failed")
	}
	pixels := lease.Pixels()
	binary.LittleEndian.PutUint32(pixels[0:], 0x00332211)
	binary.LittleEndian.PutUint32(pixels[4:], 0x00000000)
	binary.LittleEndian.PutUint32(pixels[8:], 0x80665544)

	lease.NormaliseAlpha()

	want := []uint32{0xFF332211, 0x00000000, 0x80665544}
	for i, expected := range want {
		got := binary.LittleEndian.Uint32(pixels[i*4:])
		if got != expected {
			t.Fatalf("pixel %d = 0x%08X, want 0x%08X", i, got, expected)
		}
	}
}
