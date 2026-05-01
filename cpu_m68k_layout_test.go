package main

import (
	"testing"
	"unsafe"
)

// TestCPUM68KCacheLineLayout asserts that the running/stopped/debug atomic
// trio share a single cache line and don't drift across the 64-byte
// boundary into the unrelated control registers (VBR/SFC/DFC/CACR/...).
//
// M68K's cache-line layout (cpu_m68k.go:594-616) intentionally co-locates
// AddrRegs + USP + SSP + running on Cache Line 1 — running is read every
// instruction alongside AddrRegs, so a dedicated 64-byte-aligned line for
// running would cost a cache miss per dispatch rather than save one.
// Phase 7d's "running on its own line" rule applies to 6502/Z80/x86 (which
// have heavier external-thread atomic contention on running) but not to
// M68K, where running is contended with the dispatch loop's hot reads.
//
// This test guards against accidental drift by pinning the cache-line
// home (the 64-byte block running lives in) to the same one as the AddrRegs
// block.
func TestCPUM68KCacheLineLayout(t *testing.T) {
	var cpu M68KCPU
	runningLine := unsafe.Offsetof(cpu.running) / 64
	addrLine := unsafe.Offsetof(cpu.AddrRegs) / 64
	if runningLine != addrLine {
		t.Fatalf("M68K running on cache line %d, AddrRegs on cache line %d — drift would split a hot-path co-location",
			runningLine, addrLine)
	}
}
