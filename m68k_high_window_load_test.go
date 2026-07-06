//go:build linux || darwin

// Scoped to mmap-backed hosts: NewMachineBusSized(busMemMaxBytes) is a
// lazy mapping there, but would eagerly commit ~4 GiB of Go heap on
// non-mmap platforms.

package main

import (
	"os"
	"testing"
)

// A flat M68K image larger than the historical 256 MiB low window must
// load in full on an IE64-family-sized bus. Self-contained game images
// relocate themselves to 0x10000000, which is the first byte past the
// old window; a clamped or partially deposited image used to bus-fault
// there mid-copy. The bus window/backing boundary derives from
// len(bus.memory), so a full 32-bit window makes the whole image
// addressable to the 32-bit CPU.
func TestM68KLoadProgramBytes_ReachesPastHistoricalLowWindow(t *testing.T) {
	// Allocates and copies a ~256 MiB image, ~30s. Kept out of the
	// default suite; set IE_LARGE_ALLOC_TESTS=1 to run it.
	if os.Getenv("IE_LARGE_ALLOC_TESTS") != "1" {
		t.Skip("set IE_LARGE_ALLOC_TESTS=1 to run the 256 MiB high-window load test")
	}
	bus, err := NewMachineBusSized(busMemMaxBytes)
	if err != nil {
		t.Fatalf("NewMachineBusSized(busMemMaxBytes): %v", err)
	}
	cpu := NewM68KCPU(bus)

	// Image ends 4 KiB past the historical window boundary.
	progLen := int(lowMemWindowBytes) - M68K_ENTRY_POINT + 0x1000
	program := make([]byte, progLen)
	program[progLen-1] = 0x5A

	cpu.LoadProgramBytes(program)

	lastAddr := uint32(M68K_ENTRY_POINT + progLen - 1)
	if lastAddr <= uint32(lowMemWindowBytes) {
		t.Fatalf("test image does not cross the historical window: last=%#x", lastAddr)
	}
	if got := cpu.memory[lastAddr]; got != 0x5A {
		t.Fatalf("byte past historical window not deposited: mem[%#x]=%#x want 0x5A", lastAddr, got)
	}
}
