//go:build linux

package main

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestNewMmapBacking_PassesMapNoReserve(t *testing.T) {
	var gotLength int
	var gotProt int
	var gotFlags int
	b, err := newMmapBackingWithMmap(uint64(MMU_PAGE_SIZE), func(length, prot, flags int) ([]byte, error) {
		gotLength = length
		gotProt = prot
		gotFlags = flags
		return unix.Mmap(-1, 0, length, prot, flags)
	})
	if err != nil {
		t.Fatalf("newMmapBackingWithMmap: %v", err)
	}
	defer func() { _ = b.Close() }()
	if gotLength != MMU_PAGE_SIZE {
		t.Fatalf("mmap length=%d, want %d", gotLength, MMU_PAGE_SIZE)
	}
	if gotProt != unix.PROT_READ|unix.PROT_WRITE {
		t.Fatalf("mmap prot=%d, want PROT_READ|PROT_WRITE", gotProt)
	}
	wantFlags := unix.MAP_PRIVATE | unix.MAP_ANON | unix.MAP_NORESERVE
	if gotFlags != wantFlags {
		t.Fatalf("mmap flags=%d, want %d", gotFlags, wantFlags)
	}
}
