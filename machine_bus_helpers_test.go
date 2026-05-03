package main

import "testing"

func TestSignExtMirrorAddr(t *testing.T) {
	cases := []struct {
		start uint32
		want  uint32
		ok    bool
	}{
		{0x7FFF, 0, false},
		{0x8000, 0xFFFF8000, true},
		{0x9000, 0xFFFF9000, true},
		{0xFFFF, 0xFFFFFFFF, true},
		{0x10000, 0, false},
		{0xF0000, 0, false},
	}
	for _, tc := range cases {
		got, ok := signExtMirror(tc.start)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("signExtMirror(0x%X) = (0x%X, %v), want (0x%X, %v)",
				tc.start, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFindIORegion64_NewestWins(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF2000, 0xF20FF, func(addr uint32) uint64 { return 1 }, nil)
	bus.MapIO64(0xF2000, 0xF20FF, func(addr uint32) uint64 { return 2 }, nil)

	region := bus.findIORegion64(0xF2000)
	if region == nil || region.onRead64 == nil {
		t.Fatal("findIORegion64 returned no readable region")
	}
	if got := region.onRead64(0xF2000); got != 2 {
		t.Fatalf("findIORegion64 newest value = %d, want 2", got)
	}
}

func TestFindIORegion_NewestWins(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO(0xF2100, 0xF21FF, func(addr uint32) uint32 { return 1 }, nil)
	bus.MapIO(0xF2100, 0xF21FF, func(addr uint32) uint32 { return 2 }, nil)

	region := bus.findIORegion(0xF2100)
	if region == nil || region.onRead == nil {
		t.Fatal("findIORegion returned no readable region")
	}
	if got := region.onRead(0xF2100); got != 2 {
		t.Fatalf("findIORegion newest value = %d, want 2", got)
	}
}
