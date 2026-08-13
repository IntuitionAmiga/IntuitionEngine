package main

import (
	"reflect"
	"testing"
)

func TestJACKRingStartsWithTwoSilentPeriodsAndOneFree(t *testing.T) {
	ring := newJACKSampleRing(4)
	if got, want := ring.available(), 8; got != want {
		t.Fatalf("initial readable frames = %d, want %d", got, want)
	}
	if got, want := ring.free(), 4; got != want {
		t.Fatalf("initial free frames = %d, want %d", got, want)
	}
	if got, want := ring.readCursor(), uint64(0); got != want {
		t.Fatalf("initial read cursor = %d, want %d", got, want)
	}
	if got, want := ring.writeCursor(), uint64(8); got != want {
		t.Fatalf("initial write cursor = %d, want %d", got, want)
	}
}

func TestJACKRingStartupPrefillCursorsAcrossFirstThreeCallbacks(t *testing.T) {
	ring := newJACKSampleRing(4)
	ring.enablePlayback()
	left, right := make([]float32, 4), make([]float32, 4)
	for callback, wantRead := range []uint64{4, 8, 8} {
		ring.readStereo(left, right)
		if got := ring.readCursor(); got != wantRead {
			t.Fatalf("callback %d read cursor = %d, want %d", callback+1, got, wantRead)
		}
		if got := ring.writeCursor(); got != 8 {
			t.Fatalf("callback %d write cursor = %d, want 8", callback+1, got)
		}
		for _, sample := range left {
			if sample != 0 {
				t.Fatalf("callback %d prefill was not silent: %v", callback+1, left)
			}
		}
	}
	if got := ring.underruns(); got != 1 {
		t.Fatalf("startup exhaustion underruns = %d, want 1", got)
	}
}

func TestJACKRingPreservesOrderingAcrossWraparound(t *testing.T) {
	ring := newJACKSampleRing(2)
	left, right := make([]float32, 2), make([]float32, 2)
	ring.enablePlayback()
	ring.readStereo(left, right)
	ring.readStereo(left, right)
	if !ring.write([]float32{1, 2}) || !ring.write([]float32{3, 4}) {
		t.Fatal("write unexpectedly failed")
	}
	got := make([]float32, 4)
	ring.readMono(got)
	if want := []float32{1, 2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read samples = %v, want %v", got, want)
	}
}

func TestJACKRingUnderrunSilencesRemainderAndCountsIt(t *testing.T) {
	ring := newJACKSampleRing(4)
	left, right := make([]float32, 4), make([]float32, 4)
	ring.enablePlayback()
	ring.readStereo(left, right)
	ring.readStereo(left, right)
	if !ring.write([]float32{0.25, -0.5}) {
		t.Fatal("write unexpectedly failed")
	}
	ring.readStereo(left, right)
	if want := []float32{0.25, -0.5, 0, 0}; !reflect.DeepEqual(left, want) {
		t.Fatalf("left output = %v, want %v", left, want)
	}
	if !reflect.DeepEqual(right, left) {
		t.Fatalf("right output = %v, want mono duplicate %v", right, left)
	}
	if got, want := ring.underruns(), uint64(1); got != want {
		t.Fatalf("underruns = %d, want %d", got, want)
	}
}

func TestJACKRingReadsSmallerEqualAndLargerChunks(t *testing.T) {
	ring := newJACKSampleRing(4)
	ring.enablePlayback()
	// Retire the deliberate construction prefill before exercising chunks.
	ring.readMono(make([]float32, 8))
	if !ring.write([]float32{1, 2, 3, 4}) || !ring.write([]float32{5, 6, 7, 8}) {
		t.Fatal("period writes unexpectedly failed")
	}
	for _, tc := range []struct {
		n    int
		want []float32
	}{
		{2, []float32{1, 2}},
		{4, []float32{3, 4, 5, 6}},
		{4, []float32{7, 8, 0, 0}},
	} {
		got := make([]float32, tc.n)
		ring.readMono(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("read %d frames = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestJACKRingFullWriteDoesNotAdvanceState(t *testing.T) {
	ring := newJACKSampleRing(2)
	before := ring.writeCursor()
	if ring.write([]float32{1, 2, 3}) {
		t.Fatal("oversized write succeeded")
	}
	if got := ring.writeCursor(); got != before {
		t.Fatalf("write cursor advanced to %d from %d", got, before)
	}
}

func TestJACKRingConsumerDoesNotAllocate(t *testing.T) {
	ring := newJACKSampleRing(64)
	ring.enablePlayback()
	left, right := make([]float32, 64), make([]float32, 64)
	if got := testing.AllocsPerRun(1000, func() { ring.readStereo(left, right) }); got != 0 {
		t.Fatalf("JACK ring consumer allocations = %v, want 0", got)
	}
}
