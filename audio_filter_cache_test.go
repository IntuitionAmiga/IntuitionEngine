package main

import (
	"math"
	"testing"
)

func TestCalculateFilterCutoff_MemoisedStableInput(t *testing.T) {
	var lastIn, lastOut float32
	var valid bool

	input := float32(0.625)
	want := calculateFilterCutoff(input)
	got := memoizedFilterCutoff(input, &lastIn, &lastOut, &valid)
	if math.Float32bits(got) != math.Float32bits(want) {
		t.Fatalf("first memoized cutoff = %08x, want %08x", math.Float32bits(got), math.Float32bits(want))
	}

	lastOut = 0.12345
	got = memoizedFilterCutoff(input, &lastIn, &lastOut, &valid)
	if got != 0.12345 {
		t.Fatalf("stable input did not use cached output: got %f", got)
	}

	want = calculateFilterCutoff(input + 0.001)
	got = memoizedFilterCutoff(input+0.001, &lastIn, &lastOut, &valid)
	if math.Float32bits(got) != math.Float32bits(want) {
		t.Fatalf("changed input cutoff = %08x, want %08x", math.Float32bits(got), math.Float32bits(want))
	}
}
