package main

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestBusRead64Write64 writes a 64-bit value to plain RAM and reads it back,
// verifying that Read64/Write64 round-trip correctly.
func TestBusRead64Write64(t *testing.T) {
	bus := NewSystemBus()
	var want uint64 = 0xDEADBEEFCAFEBABE

	bus.Write64(0x2000, want)
	got := bus.Read64(0x2000)

	if got != want {
		t.Fatalf("Read64 = 0x%016X, want 0x%016X", got, want)
	}
}

// TestBusRead64Write64_Endianness verifies that 64-bit values are stored in
// little-endian byte order in the underlying memory, consistent with the
// existing 32-bit and 16-bit bus operations.
func TestBusRead64Write64_Endianness(t *testing.T) {
	bus := NewSystemBus()
	var val uint64 = 0x0102030405060708

	bus.Write64(0x2000, val)

	mem := bus.GetMemory()
	// Little-endian: least significant byte at lowest address
	expectedBytes := []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	for i, want := range expectedBytes {
		got := mem[0x2000+i]
		if got != want {
			t.Errorf("memory[0x%04X] = 0x%02X, want 0x%02X", 0x2000+i, got, want)
		}
	}
}

// TestBusRead64Write64_NativeIORegion maps a 64-bit I/O region using MapIO64
// and verifies that Read64 invokes the onRead64 callback and Write64 invokes
// the onWrite64 callback.
func TestBusRead64Write64_NativeIORegion(t *testing.T) {
	bus := NewSystemBus()

	var writtenAddr uint32
	var writtenValue uint64
	readCalled := false
	writeCalled := false

	bus.MapIO64(0xF0000, 0xF00FF,
		func(addr uint32) uint64 {
			readCalled = true
			return 0xAAAABBBBCCCCDDDD
		},
		func(addr uint32, value uint64) {
			writeCalled = true
			writtenAddr = addr
			writtenValue = value
		},
	)

	// Test Write64 fires onWrite64
	bus.Write64(0xF0000, 0x1122334455667788)
	if !writeCalled {
		t.Fatal("Write64 did not invoke onWrite64 callback")
	}
	if writtenAddr != 0xF0000 {
		t.Errorf("onWrite64 addr = 0x%08X, want 0x000F0000", writtenAddr)
	}
	if writtenValue != 0x1122334455667788 {
		t.Errorf("onWrite64 value = 0x%016X, want 0x1122334455667788", writtenValue)
	}

	// Test Read64 fires onRead64
	got := bus.Read64(0xF0000)
	if !readCalled {
		t.Fatal("Read64 did not invoke onRead64 callback")
	}
	if got != 0xAAAABBBBCCCCDDDD {
		t.Errorf("Read64 = 0x%016X, want 0xAAAABBBBCCCCDDDD", got)
	}
}

// TestBusRead64Write64_LegacyIORegion_Fault verifies that when a legacy
// (32-bit) I/O region is mapped with MapIO and the default Fault policy is
// active, Read64 returns 0 and Write64 is a no-op (the legacy onWrite is NOT
// called).
func TestBusRead64Write64_LegacyIORegion_Fault(t *testing.T) {
	bus := NewSystemBus()

	writeCalled := false
	bus.MapIO(0xE0000, 0xE00FF,
		func(addr uint32) uint32 {
			return 0x42424242
		},
		func(addr uint32, value uint32) {
			writeCalled = true
		},
	)

	// Default policy is Fault — Read64 should return 0
	got := bus.Read64(0xE0000)
	if got != 0 {
		t.Errorf("Read64 with Fault policy = 0x%016X, want 0", got)
	}

	// Write64 should be a no-op — legacy onWrite should NOT be called
	bus.Write64(0xE0000, 0xFFFFFFFFFFFFFFFF)
	if writeCalled {
		t.Error("Write64 with Fault policy invoked legacy onWrite, should be no-op")
	}
}

// TestBusRead64Write64_LegacyIORegion_Split verifies that with Split policy,
// a 64-bit access to a legacy (32-bit) I/O region is decomposed into two
// 32-bit accesses: low dword at addr, high dword at addr+4.
func TestBusRead64Write64_LegacyIORegion_Split(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	var writeAddrs []uint32
	var writeValues []uint32

	bus.MapIO(0xE0000, 0xE00FF,
		func(addr uint32) uint32 {
			// Return different values for low vs high dword
			if addr == 0xE0000 {
				return 0xAAAAAAAA
			}
			if addr == 0xE0004 {
				return 0xBBBBBBBB
			}
			return 0
		},
		func(addr uint32, value uint32) {
			writeAddrs = append(writeAddrs, addr)
			writeValues = append(writeValues, value)
		},
	)

	// Write64 should split into two 32-bit writes
	bus.Write64(0xE0000, 0xBBBBBBBBAAAAAAAA)
	if len(writeAddrs) != 2 {
		t.Fatalf("Write64 Split: expected 2 onWrite calls, got %d", len(writeAddrs))
	}
	// Low 32 bits at addr
	if writeAddrs[0] != 0xE0000 {
		t.Errorf("First write addr = 0x%08X, want 0x000E0000", writeAddrs[0])
	}
	if writeValues[0] != 0xAAAAAAAA {
		t.Errorf("First write value = 0x%08X, want 0xAAAAAAAA", writeValues[0])
	}
	// High 32 bits at addr+4
	if writeAddrs[1] != 0xE0004 {
		t.Errorf("Second write addr = 0x%08X, want 0x000E0004", writeAddrs[1])
	}
	if writeValues[1] != 0xBBBBBBBB {
		t.Errorf("Second write value = 0x%08X, want 0xBBBBBBBB", writeValues[1])
	}

	// Read64 should combine two 32-bit reads: low at addr, high at addr+4
	got := bus.Read64(0xE0000)
	want := uint64(0xBBBBBBBB)<<32 | uint64(0xAAAAAAAA)
	if got != want {
		t.Errorf("Read64 Split = 0x%016X, want 0x%016X", got, want)
	}
}

// TestBusRead64Write64_CrossPage tests a 64-bit access that straddles a page
// boundary where the first 4 bytes are in plain RAM and the second 4 bytes
// fall within an I/O-mapped page. The bus should split the access correctly.
func TestBusRead64Write64_CrossPage(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	// Map I/O at page 0x100 (addr 0x100-0x1FF). Place 64-bit access at
	// addr 0xFC so bytes [0xFC..0xFF] are RAM and [0x100..0x103] are I/O.
	var ioWriteAddr uint32
	var ioWriteValue uint32
	bus.MapIO(0x100, 0x1FF,
		func(addr uint32) uint32 {
			return 0x55555555
		},
		func(addr uint32, value uint32) {
			ioWriteAddr = addr
			ioWriteValue = value
		},
	)

	// Write the lower 4 bytes as plain RAM, upper 4 bytes go through I/O
	bus.Write64(0xFC, 0x5555555511223344)

	// Verify lower 4 bytes written to RAM
	mem := bus.GetMemory()
	lowDword := binary.LittleEndian.Uint32(mem[0xFC:0x100])
	if lowDword != 0x11223344 {
		t.Errorf("Low dword in RAM = 0x%08X, want 0x11223344", lowDword)
	}

	// Verify upper 4 bytes went through I/O handler
	if ioWriteAddr != 0x100 {
		t.Errorf("I/O write addr = 0x%08X, want 0x00000100", ioWriteAddr)
	}
	if ioWriteValue != 0x55555555 {
		t.Errorf("I/O write value = 0x%08X, want 0x55555555", ioWriteValue)
	}
}

// TestBusRead64_OutOfBounds verifies that Read64 at an address near the end
// of memory where addr+8 would exceed the memory size returns 0 without panic.
func TestBusRead64_OutOfBounds(t *testing.T) {
	bus := NewSystemBus()
	memSize := uint32(len(bus.GetMemory()))

	// addr+8 > memSize
	addr := memSize - 4
	got := bus.Read64(addr)
	if got != 0 {
		t.Errorf("Read64(0x%08X) out-of-bounds = 0x%016X, want 0", addr, got)
	}
}

// TestBusRead64Write64WithFault verifies that the WithFault variants return
// true for valid RAM addresses and correctly round-trip data.
func TestBusRead64Write64WithFault(t *testing.T) {
	bus := NewSystemBus()
	var want uint64 = 0x123456789ABCDEF0

	ok := bus.Write64WithFault(0x4000, want)
	if !ok {
		t.Fatal("Write64WithFault returned false for valid address")
	}

	got, ok := bus.Read64WithFault(0x4000)
	if !ok {
		t.Fatal("Read64WithFault returned false for valid address")
	}
	if got != want {
		t.Fatalf("Read64WithFault = 0x%016X, want 0x%016X", got, want)
	}
}

// TestBusWrite64WithFault_FirstHalfFail tests that Write64WithFault returns
// false when the first 4 bytes of the access fall out of bounds (Split mode).
func TestBusWrite64WithFault_FirstHalfFail(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	// Address beyond memory entirely
	memSize := uint32(len(bus.GetMemory()))
	addr := memSize + 0x100

	ok := bus.Write64WithFault(addr, 0xDEADDEADDEADDEAD)
	if ok {
		t.Error("Write64WithFault returned true when first half is out of bounds")
	}
}

// TestBusWrite64WithFault_SecondHalfFail tests that Write64WithFault returns
// false when the second 4 bytes of the access fall out of bounds (Split mode).
func TestBusWrite64WithFault_SecondHalfFail(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	// Place addr so first 4 bytes are in bounds but addr+4 is not
	memSize := uint32(len(bus.GetMemory()))
	addr := memSize - 4 // [memSize-4..memSize-1] ok, [memSize..memSize+3] OOB

	ok := bus.Write64WithFault(addr, 0xBEEFBEEFBEEFBEEF)
	if ok {
		t.Error("Write64WithFault returned true when second half is out of bounds")
	}
}

// TestBusRead64WithFault_FirstHalfFail tests that Read64WithFault returns
// (0, false) when the first 4 bytes are out of bounds.
func TestBusRead64WithFault_FirstHalfFail(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	memSize := uint32(len(bus.GetMemory()))
	addr := memSize + 0x100

	got, ok := bus.Read64WithFault(addr)
	if ok {
		t.Error("Read64WithFault returned true when first half is out of bounds")
	}
	if got != 0 {
		t.Errorf("Read64WithFault = 0x%016X, want 0", got)
	}
}

// TestBusRead64WithFault_SecondHalfFail tests that Read64WithFault returns
// (0, false) when the second 4 bytes are out of bounds.
func TestBusRead64WithFault_SecondHalfFail(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	memSize := uint32(len(bus.GetMemory()))
	addr := memSize - 4

	got, ok := bus.Read64WithFault(addr)
	if ok {
		t.Error("Read64WithFault returned true when second half is out of bounds")
	}
	if got != 0 {
		t.Errorf("Read64WithFault = 0x%016X, want 0", got)
	}
}

// TestBusRead64Write64_SignExtended writes a 64-bit value to a low address
// and reads it back via the sign-extended alias (0xFFFF0000 | addr), verifying
// that the M68K sign-extension mapping works for 64-bit operations.
func TestBusRead64Write64_SignExtended(t *testing.T) {
	bus := NewSystemBus()
	var want uint64 = 0xFEDCBA9876543210

	bus.Write64(0x9000, want)

	// Read via sign-extended address
	got := bus.Read64(0xFFFF9000)
	if got != want {
		t.Errorf("Read64(0xFFFF9000) = 0x%016X, want 0x%016X", got, want)
	}

	// Also verify write via sign-extended, read via normal
	var want2 uint64 = 0x0011223344556677
	bus.Write64(0xFFFF9000, want2)

	got2 := bus.Read64(0x9000)
	if got2 != want2 {
		t.Errorf("Read64(0x9000) after sign-extended write = 0x%016X, want 0x%016X", got2, want2)
	}
}

// TestBusRead64Write64_Unaligned writes and reads a 64-bit value at an
// unaligned address (0x2003) and verifies correct round-trip behavior.
func TestBusRead64Write64_Unaligned(t *testing.T) {
	bus := NewSystemBus()
	var want uint64 = 0xA5A5A5A5B6B6B6B6

	bus.Write64(0x2003, want)
	got := bus.Read64(0x2003)

	if got != want {
		t.Fatalf("Read64(unaligned 0x2003) = 0x%016X, want 0x%016X", got, want)
	}

	// Verify byte-level storage is little-endian at the unaligned address
	mem := bus.GetMemory()
	var buf [8]byte
	copy(buf[:], mem[0x2003:0x200B])
	stored := binary.LittleEndian.Uint64(buf[:])
	if stored != want {
		t.Errorf("Memory bytes at 0x2003 decode to 0x%016X, want 0x%016X", stored, want)
	}
}

// TestBusRead64Write64_AddrWrap tests that a 64-bit access at address
// 0xFFFFFFFC (where addr+8 would overflow uint32) returns 0 and is a no-op,
// preventing any wrap-around corruption.
func TestBusRead64Write64_AddrWrap(t *testing.T) {
	bus := NewSystemBus()

	// Write should be a no-op (addr wraps around)
	bus.Write64(0xFFFFFFFC, 0x1111111122222222)

	// Read should return 0
	got := bus.Read64(0xFFFFFFFC)
	if got != 0 {
		t.Errorf("Read64(0xFFFFFFFC) addr wrap = 0x%016X, want 0", got)
	}
}

// TestBusMMIO64PolicyDefault verifies that a newly created bus has the Fault
// policy as the default for legacy MMIO 64-bit access.
func TestBusMMIO64PolicyDefault(t *testing.T) {
	bus := NewSystemBus()

	// The default legacyMMIO64Policy should be MMIO64PolicyFault (iota = 0)
	if bus.legacyMMIO64Policy != MMIO64PolicyFault {
		t.Errorf("Default policy = %d, want MMIO64PolicyFault (%d)",
			bus.legacyMMIO64Policy, MMIO64PolicyFault)
	}
}

// TestBusRead64WithFault_LegacyFaultPolicy verifies that Read64WithFault
// returns (0, false) when hitting a legacy I/O region under Fault policy.
func TestBusRead64WithFault_LegacyFaultPolicy(t *testing.T) {
	bus := NewSystemBus()
	// Default policy is Fault

	readCalled := false
	bus.MapIO(0xD0000, 0xD00FF,
		func(addr uint32) uint32 {
			readCalled = true
			return 0x99
		},
		nil,
	)

	got, ok := bus.Read64WithFault(0xD0000)
	if ok {
		t.Error("Read64WithFault returned true for legacy MMIO under Fault policy")
	}
	if got != 0 {
		t.Errorf("Read64WithFault = 0x%016X, want 0", got)
	}
	if readCalled {
		t.Error("Legacy onRead was called under Fault policy, should not be")
	}
}

// TestBusWrite64WithFault_LegacyFaultPolicy verifies that Write64WithFault
// returns false and does NOT invoke the legacy onWrite callback when the
// Fault policy is active.
func TestBusWrite64WithFault_LegacyFaultPolicy(t *testing.T) {
	bus := NewSystemBus()
	// Default policy is Fault

	writeCalled := false
	bus.MapIO(0xD0000, 0xD00FF,
		nil,
		func(addr uint32, value uint32) {
			writeCalled = true
		},
	)

	ok := bus.Write64WithFault(0xD0000, 0xBADBADBADBADBAD0)
	if ok {
		t.Error("Write64WithFault returned true for legacy MMIO under Fault policy")
	}
	if writeCalled {
		t.Error("Legacy onWrite was called under Fault policy, should not be")
	}
}

// TestBusRead64Write64_MixedSpan_RAM_MMIO tests a 64-bit access that spans
// both plain RAM (first 4 bytes) and a legacy MMIO region (second 4 bytes)
// under Split policy. The bus must split the operation correctly.
func TestBusRead64Write64_MixedSpan_RAM_MMIO(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	// Map legacy I/O at 0x5100-0x51FF. Place 64-bit access at 0x50FC so
	// bytes [0x50FC..0x50FF] are plain RAM and [0x5100..0x5103] hit I/O.
	var ioReadAddr uint32
	var ioWriteAddr uint32
	var ioWriteVal uint32

	bus.MapIO(0x5100, 0x51FF,
		func(addr uint32) uint32 {
			ioReadAddr = addr
			return 0xDDDDDDDD
		},
		func(addr uint32, value uint32) {
			ioWriteAddr = addr
			ioWriteVal = value
		},
	)

	// Pre-populate RAM portion
	bus.Write32(0x50FC, 0x11111111)

	// Write64 at 0x50FC: low dword -> RAM, high dword -> MMIO
	bus.Write64(0x50FC, 0xCCCCCCCC22222222)

	// Verify RAM portion
	mem := bus.GetMemory()
	ramLow := binary.LittleEndian.Uint32(mem[0x50FC:0x5100])
	if ramLow != 0x22222222 {
		t.Errorf("RAM low dword = 0x%08X, want 0x22222222", ramLow)
	}

	// Verify I/O write was triggered for the high dword
	if ioWriteAddr != 0x5100 {
		t.Errorf("I/O write addr = 0x%08X, want 0x00005100", ioWriteAddr)
	}
	if ioWriteVal != 0xCCCCCCCC {
		t.Errorf("I/O write value = 0x%08X, want 0xCCCCCCCC", ioWriteVal)
	}

	// Read64 at 0x50FC: low dword from RAM, high dword from I/O
	got := bus.Read64(0x50FC)
	wantLow := uint64(0x22222222)
	wantHigh := uint64(0xDDDDDDDD) << 32
	want := wantHigh | wantLow
	if got != want {
		t.Errorf("Read64(0x50FC) mixed span = 0x%016X, want 0x%016X", got, want)
	}

	if ioReadAddr != 0x5100 {
		t.Errorf("I/O read addr = 0x%08X, want 0x00005100", ioReadAddr)
	}
}

// TestBusRead64Write64_MixedSpan_Native64_Legacy tests a 64-bit access
// spanning a native 64-bit I/O region (first 4 bytes) and a legacy 32-bit
// I/O region (second 4 bytes) under Split policy. The native64 handler should
// be used for the first half, and the legacy handler (split) for the second.
func TestBusRead64Write64_MixedSpan_Native64_Legacy(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	native64WriteCalled := false
	native64ReadCalled := false

	// Native 64-bit region at 0x6000-0x60FF
	bus.MapIO64(0x6000, 0x60FF,
		func(addr uint32) uint64 {
			native64ReadCalled = true
			return 0xAAAAAAAABBBBBBBB
		},
		func(addr uint32, value uint64) {
			native64WriteCalled = true
		},
	)

	legacyWriteCalled := false
	legacyReadCalled := false

	// Legacy 32-bit region at 0x6100-0x61FF
	bus.MapIO(0x6100, 0x61FF,
		func(addr uint32) uint32 {
			legacyReadCalled = true
			return 0xCCCCCCCC
		},
		func(addr uint32, value uint32) {
			legacyWriteCalled = true
		},
	)

	// 64-bit access at 0x60FC spans both regions:
	// [0x60FC..0x60FF] = native64 region, [0x6100..0x6103] = legacy region
	bus.Write64(0x60FC, 0x1111111122222222)

	// The write should have touched both regions
	if !native64WriteCalled {
		t.Error("Native64 onWrite64 was not called for spanning write")
	}
	if !legacyWriteCalled {
		t.Error("Legacy onWrite was not called for spanning write")
	}

	// Read back
	got := bus.Read64(0x60FC)
	if !native64ReadCalled {
		t.Error("Native64 onRead64 was not called for spanning read")
	}
	if !legacyReadCalled {
		t.Error("Legacy onRead was not called for spanning read")
	}

	// Verify the result combines both halves:
	// Low dword: read32Half(0x60FC) hits native64 region, aligns to base=0x60F8,
	//   reads 0xAAAAAAAABBBBBBBB, extracts high half (addr != base) → 0xAAAAAAAA
	// High dword: read32Half(0x6100) hits legacy region (Split policy) → 0xCCCCCCCC
	want := uint64(0xAAAAAAAA) | (uint64(0xCCCCCCCC) << 32)
	if got != want {
		t.Errorf("Read64(0x60FC) mixed native64+legacy = 0x%016X, want 0x%016X", got, want)
	}
}

// TestBusSplitWrite_Native64_NoReadSideEffect verifies that a split write to a
// native-64 MMIO region does NOT invoke onRead64. This locks in the design
// decision that write32Half reads from backing memory (not the device) to
// avoid side effects such as clear-on-read or FIFO pop.
func TestBusSplitWrite_Native64_NoReadSideEffect(t *testing.T) {
	bus := NewSystemBus()
	bus.SetLegacyMMIO64Policy(MMIO64PolicySplit)

	readCalled := false
	var lastWriteVal uint64

	// Native 64-bit region at 0x7000-0x70FF
	bus.MapIO64(0x7000, 0x70FF,
		func(addr uint32) uint64 {
			readCalled = true
			return 0xDEADBEEFCAFEBABE
		},
		func(addr uint32, value uint64) {
			lastWriteVal = value
		},
	)

	// Legacy region at 0x7100-0x71FF to force a split
	bus.MapIO(0x7100, 0x71FF,
		func(addr uint32) uint32 { return 0 },
		func(addr uint32, value uint32) {},
	)

	// Write64 at 0x70FC spans native64 [0x70FC..0x70FF] + legacy [0x7100..0x7103]
	// This forces write32Half for the native64 half
	bus.Write64(0x70FC, 0x1111111122222222)

	if readCalled {
		t.Error("write32Half must not call onRead64 — device reads may have side effects")
	}

	// The write handler should still have been called
	if lastWriteVal == 0 {
		t.Error("onWrite64 was not called for the native64 half")
	}
}

// TestBusMapIO64_SignExtension verifies that MapIO64 for a region in the
// 0x8000-0xFFFF range is accessible via the sign-extended address
// (0xFFFF0000 | addr), matching the existing MapIO sign-extension behavior.
func TestBusMapIO64_SignExtension(t *testing.T) {
	bus := NewSystemBus()

	readCalled := false
	writeCalled := false

	bus.MapIO64(0x8000, 0x80FF,
		func(addr uint32) uint64 {
			readCalled = true
			return 0x1234567890ABCDEF
		},
		func(addr uint32, value uint64) {
			writeCalled = true
		},
	)

	// Access via sign-extended address should use the 64-bit handler
	got := bus.Read64(0xFFFF8000)
	if !readCalled {
		t.Fatal("Read64(0xFFFF8000) did not invoke MapIO64 onRead64 handler")
	}
	if got != 0x1234567890ABCDEF {
		t.Errorf("Read64(0xFFFF8000) = 0x%016X, want 0x1234567890ABCDEF", got)
	}

	bus.Write64(0xFFFF8000, 0xFEDCBA0987654321)
	if !writeCalled {
		t.Fatal("Write64(0xFFFF8000) did not invoke MapIO64 onWrite64 handler")
	}
}

// TestBusRead64_PrefersNative64OverLegacy verifies that when both MapIO and
// MapIO64 cover the same address range, Read64 uses the native 64-bit handler
// rather than falling through to the legacy 32-bit handler.
func TestBusRead64_PrefersNative64OverLegacy(t *testing.T) {
	bus := NewSystemBus()

	legacyReadCalled := false
	native64ReadCalled := false

	// Map legacy 32-bit I/O first
	bus.MapIO(0xA0000, 0xA00FF,
		func(addr uint32) uint32 {
			legacyReadCalled = true
			return 0x32323232
		},
		nil,
	)

	// Map native 64-bit I/O over the same range
	bus.MapIO64(0xA0000, 0xA00FF,
		func(addr uint32) uint64 {
			native64ReadCalled = true
			return 0x6464646464646464
		},
		nil,
	)

	got := bus.Read64(0xA0000)

	if !native64ReadCalled {
		t.Error("Read64 did not use native 64-bit handler")
	}
	if legacyReadCalled {
		t.Error("Read64 fell through to legacy 32-bit handler when native 64-bit was available")
	}
	if got != 0x6464646464646464 {
		t.Errorf("Read64 = 0x%016X, want 0x6464646464646464", got)
	}

	// Also verify Write64 prefers native
	legacyWriteCalled := false
	native64WriteCalled := false

	bus2 := NewSystemBus()

	bus2.MapIO(0xA0000, 0xA00FF,
		nil,
		func(addr uint32, value uint32) {
			legacyWriteCalled = true
		},
	)

	bus2.MapIO64(0xA0000, 0xA00FF,
		nil,
		func(addr uint32, value uint64) {
			native64WriteCalled = true
		},
	)

	bus2.Write64(0xA0000, 0xBEEFBEEFBEEFBEEF)

	if !native64WriteCalled {
		t.Error("Write64 did not use native 64-bit handler")
	}
	if legacyWriteCalled {
		t.Error("Write64 fell through to legacy 32-bit handler when native 64-bit was available")
	}
}

// Compile-time assertion: ensure math is used (for any float-based 64-bit tests)
var _ = math.MaxFloat64
