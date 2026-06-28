//go:build amd64 && (linux || windows || darwin)

package main

import (
	"reflect"
	"testing"
)

// m68kCoalesceInvalRanges merges queued [start,end) JIT-invalidation ranges so
// the drain loop scans each guest region once instead of repeating the per-page
// and cache scans for every individual relocation write. Merging the union is
// behaviourally identical to invalidating each sub-range, so this is a pure
// performance transform with no correctness exposure.
func TestM68KCoalesceInvalRanges_EmptyAndSingle(t *testing.T) {
	if got := m68kCoalesceInvalRanges(nil); len(got) != 0 {
		t.Fatalf("nil: got %v, want empty", got)
	}
	single := [][2]uint32{{0x1000, 0x1010}}
	if got := m68kCoalesceInvalRanges(single); !reflect.DeepEqual(got, single) {
		t.Fatalf("single: got %v, want %v", got, single)
	}
}

func TestM68KCoalesceInvalRanges_DisjointSorts(t *testing.T) {
	in := [][2]uint32{{0x3000, 0x3010}, {0x1000, 0x1010}, {0x2000, 0x2010}}
	want := [][2]uint32{{0x1000, 0x1010}, {0x2000, 0x2010}, {0x3000, 0x3010}}
	if got := m68kCoalesceInvalRanges(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("disjoint: got %v, want %v", got, want)
	}
}

func TestM68KCoalesceInvalRanges_OverlapAndAdjacentMerge(t *testing.T) {
	// Overlapping, nested, and exactly-adjacent (touching) ranges all collapse.
	in := [][2]uint32{
		{0x1000, 0x1020}, // base
		{0x1010, 0x1030}, // overlaps base
		{0x1030, 0x1040}, // adjacent to previous (touching at 0x1030)
		{0x1034, 0x1038}, // nested inside previous
	}
	want := [][2]uint32{{0x1000, 0x1040}}
	if got := m68kCoalesceInvalRanges(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("merge: got %v, want %v", got, want)
	}
}

func TestM68KCoalesceInvalRanges_KeepsGapBoundary(t *testing.T) {
	// A one-byte gap must NOT merge (end is exclusive: [0x1000,0x1010) and
	// [0x1011,0x1020) leave 0x1010 untouched).
	in := [][2]uint32{{0x1000, 0x1010}, {0x1011, 0x1020}}
	got := m68kCoalesceInvalRanges(in)
	if len(got) != 2 {
		t.Fatalf("gap: got %v, want 2 disjoint ranges", got)
	}
}

// m68kWriteOutsideCodeBounds is the O(1) negative reject: if a guest write lies
// entirely outside the conservative global envelope of all compiled code
// [codeLo,codeHi), no JIT block can cover it, so invalidation is skipped before
// any per-page or cache scan. Empty bounds (codeHi<=codeLo) must never reject —
// the envelope is unknown, so fall through to the authoritative slow path.
func TestM68KWriteOutsideCodeBounds(t *testing.T) {
	const lo, hi = 0x2000, 0x3000 // code occupies [0x2000,0x3000)
	cases := []struct {
		name       string
		addr, size uint32
		want       bool
	}{
		{"zero size", 0x2500, 0, true},
		{"fully below", 0x1000, 0x100, true},
		{"ends exactly at lo (exclusive)", 0x1F00, 0x100, true}, // [0x1F00,0x2000)
		{"straddles lo", 0x1F00, 0x200, false},                  // [0x1F00,0x2100)
		{"fully inside", 0x2500, 0x10, false},
		{"straddles hi", 0x2F00, 0x200, false}, // [0x2F00,0x3100)
		{"starts exactly at hi", 0x3000, 0x100, true},
		{"fully above", 0x4000, 0x100, true},
	}
	for _, c := range cases {
		if got := m68kWriteOutsideCodeBounds(c.addr, c.size, lo, hi); got != c.want {
			t.Errorf("%s: outside(%#x,%#x,[%#x,%#x))=%v, want %v", c.name, c.addr, c.size, lo, hi, got, c.want)
		}
	}
}

func TestM68KWriteOutsideCodeBounds_EmptyEnvelopeNeverRejects(t *testing.T) {
	// codeHi<=codeLo means no compiled code is registered (or bounds reset).
	// A live write must NOT be rejected on unknown bounds.
	for _, b := range [][2]uint32{{0, 0}, {0x5000, 0x5000}, {0x5000, 0x1000}} {
		if m68kWriteOutsideCodeBounds(0x2000, 0x10, b[0], b[1]) {
			t.Fatalf("empty bounds [%#x,%#x): rejected a write, want never reject", b[0], b[1])
		}
	}
}
