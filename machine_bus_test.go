package main

import (
	"encoding/binary"
	"sync"
	"testing"
)

// TestBus32GetMemory verifies that MachineBus exposes its memory slice
// via GetMemory() for direct access by CPU cores.
func TestBus32GetMemory(t *testing.T) {
	bus := NewMachineBus()

	mem := bus.GetMemory()
	if mem == nil {
		t.Fatal("GetMemory() returned nil")
	}
	if len(mem) != DEFAULT_MEMORY_SIZE {
		t.Fatalf("GetMemory() length %d, expected %d", len(mem), DEFAULT_MEMORY_SIZE)
	}

	// Write through bus, read through memory slice
	bus.Write32(0x1000, 0x12345678)
	got := binary.LittleEndian.Uint32(mem[0x1000:])
	if got != 0x12345678 {
		t.Fatalf("Direct memory read 0x%08X, expected 0x12345678", got)
	}
}

func TestMachineBusWrite32WideFanoutOptIn(t *testing.T) {
	bus := NewMachineBus()
	var byteWrites []struct {
		addr  uint32
		value uint8
	}
	var wordWrites int

	bus.MapIO(0xF1000, 0xF1003, nil, func(addr uint32, value uint32) {
		wordWrites++
	})
	bus.MapIOByte(0xF1000, 0xF1003, func(addr uint32, value uint8) {
		byteWrites = append(byteWrites, struct {
			addr  uint32
			value uint8
		}{addr: addr, value: value})
	})
	bus.MapIOWideWriteFanout(0xF1000, 0xF1003)

	bus.Write32(0xF1000, 0x44332211)

	if wordWrites != 0 {
		t.Fatalf("onWrite called %d times, want 0", wordWrites)
	}
	if len(byteWrites) != 4 {
		t.Fatalf("onWrite8 calls=%d, want 4", len(byteWrites))
	}
	for i, want := range []uint8{0x11, 0x22, 0x33, 0x44} {
		if byteWrites[i].addr != 0xF1000+uint32(i) || byteWrites[i].value != want {
			t.Fatalf("byte write %d=(0x%X,0x%02X), want (0x%X,0x%02X)",
				i, byteWrites[i].addr, byteWrites[i].value, 0xF1000+uint32(i), want)
		}
	}
}

func TestMachineBusWrite32WideFanoutDefaultCompatibility(t *testing.T) {
	bus := NewMachineBus()
	var byteWrites int
	var wordWrites []uint32

	bus.MapIO(0xF1100, 0xF1103, nil, func(addr uint32, value uint32) {
		wordWrites = append(wordWrites, value)
	})
	bus.MapIOByte(0xF1100, 0xF1103, func(addr uint32, value uint8) {
		byteWrites++
	})

	bus.Write32(0xF1100, 0x44332211)

	if byteWrites != 0 {
		t.Fatalf("onWrite8 calls=%d, want 0 without opt-in", byteWrites)
	}
	if len(wordWrites) != 1 || wordWrites[0] != 0x44332211 {
		t.Fatalf("onWrite calls=%v, want [0x44332211]", wordWrites)
	}
}

func TestMachineBusWrite16StillFansOutWithByteHandler(t *testing.T) {
	bus := NewMachineBus()
	var byteWrites []uint8
	var wordWrites int

	bus.MapIO(0xF1200, 0xF1201, nil, func(addr uint32, value uint32) {
		wordWrites++
	})
	bus.MapIOByte(0xF1200, 0xF1201, func(addr uint32, value uint8) {
		byteWrites = append(byteWrites, value)
	})

	bus.Write16(0xF1200, 0x2211)

	if wordWrites != 0 {
		t.Fatalf("onWrite called %d times, want 0", wordWrites)
	}
	if len(byteWrites) != 2 || byteWrites[0] != 0x11 || byteWrites[1] != 0x22 {
		t.Fatalf("onWrite8 values=%v, want [0x11 0x22]", byteWrites)
	}
}

// TestCPUMemoryVisibleToBus verifies that data written by the CPU
// is visible when read through the MachineBus (as peripherals would).
func TestCPUMemoryVisibleToBus(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU(bus)

	// Write through CPU's memory at address 0x2000
	cpu.Write32(0x2000, 0xDEADBEEF)

	// Read through bus - should see the same value
	got := bus.Read32(0x2000)
	if got != 0xDEADBEEF {
		t.Fatalf("Bus read 0x%08X, expected 0xDEADBEEF - memory not shared", got)
	}
}

func TestUnmapIO_ClearsMapping64(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF3000, 0xF30FF, func(addr uint32) uint64 { return 0x42 }, nil)

	if bus.findIORegion64(0xF3000) == nil {
		t.Fatal("precondition: mapping64 not installed")
	}
	bus.UnmapIO(0xF3000, 0xF30FF)
	if bus.findIORegion64(0xF3000) != nil {
		t.Fatal("UnmapIO left mapping64 region installed")
	}
}

func TestUnmapIO_ClearsSignExtMirror_DirectMapInspection(t *testing.T) {
	bus := NewMachineBus()
	const start uint32 = 0x9000
	bus.MapIO(start, start+0xFF, nil, func(addr uint32, value uint32) {})
	bus.MapIO64(start, start+0xFF, func(addr uint32) uint64 { return 0 }, nil)

	mirror := start | 0xFFFF0000
	if len(bus.mapping[mirror&PAGE_MASK]) == 0 {
		t.Fatal("precondition: sign-extended mapping not installed")
	}
	if len(bus.mapping64[mirror&PAGE_MASK]) == 0 {
		t.Fatal("precondition: sign-extended mapping64 not installed")
	}

	bus.UnmapIO(start, start+0xFF)
	if len(bus.mapping[mirror&PAGE_MASK]) != 0 {
		t.Fatal("UnmapIO left sign-extended legacy mapping installed")
	}
	if len(bus.mapping64[mirror&PAGE_MASK]) != 0 {
		t.Fatal("UnmapIO left sign-extended mapping64 installed")
	}
}

func TestUnmapIO_ClearsSignExtMirror_BehaviorLock(t *testing.T) {
	bus := NewMachineBus()
	var writes int
	bus.MapIO(0x9000, 0x90FF, nil, func(addr uint32, value uint32) {
		writes++
	})

	bus.Write32(0xFFFF9000, 1)
	if writes != 1 {
		t.Fatalf("precondition: sign-extended write dispatched %d times, want 1", writes)
	}

	bus.UnmapIO(0x9000, 0x90FF)
	bus.Write32(0xFFFF9000, 2)
	if writes != 1 {
		t.Fatalf("sign-extended write dispatched after unmap; writes=%d, want 1", writes)
	}
}

func TestUnmapIO_BitmapClearedOnlyWhenBothMapsEmpty(t *testing.T) {
	bus := NewMachineBus()
	const addr uint32 = 0xF4000
	bus.MapIO(addr, addr+0xFF, nil, func(addr uint32, value uint32) {})
	bus.MapIO64(addr, addr+0xFF, func(addr uint32) uint64 { return 0 }, nil)

	bus.UnmapIO(addr, addr+0xFF)
	if bus.ioPageBitmap[addr>>8] {
		t.Fatal("bitmap still set after removing both legacy and 64-bit maps")
	}
}

func TestRead8_SignExtMirror_FiresOnRead8(t *testing.T) {
	bus := NewMachineBus()
	calls := 0
	bus.MapIO(0x9000, 0x90FF, nil, nil)
	bus.MapIOByteRead(0x9000, 0x90FF, func(addr uint32) uint8 {
		calls++
		if addr != 0x9000 {
			t.Fatalf("onRead8 addr=0x%X, want 0x9000", addr)
		}
		return 0xAB
	})

	got := bus.Read8(0xFFFF9000)
	if got != 0xAB {
		t.Fatalf("Read8 sign-ext = 0x%02X, want 0xAB", got)
	}
	if calls != 1 {
		t.Fatalf("onRead8 calls=%d, want 1", calls)
	}
}

func TestMapIONoShadow_DoesNotMirrorReadWrite(t *testing.T) {
	bus := NewMachineBus()
	const addr uint32 = 0xF5600
	bus.MapIONoShadow(addr, addr+3,
		func(addr uint32) uint32 { return 0xAABBCCDD },
		func(addr uint32, value uint32) {},
	)

	bus.Write32(addr, 0x11223344)
	if got := bus.Read32(addr); got != 0xAABBCCDD {
		t.Fatalf("Read32 = 0x%08X, want handler value", got)
	}
	if got := binary.LittleEndian.Uint32(bus.memory[addr : addr+4]); got != 0 {
		t.Fatalf("shadow memory = 0x%08X, want unchanged zero", got)
	}
}

func TestMapIOShadow_MirrorsReadWrite(t *testing.T) {
	bus := NewMachineBus()
	const addr uint32 = 0xF5700
	bus.MapIOShadow(addr, addr+3,
		func(addr uint32) uint32 { return 0xAABBCCDD },
		func(addr uint32, value uint32) {},
	)

	bus.Write32(addr, 0x11223344)
	if got := binary.LittleEndian.Uint32(bus.memory[addr : addr+4]); got != 0x11223344 {
		t.Fatalf("write shadow memory = 0x%08X, want 0x11223344", got)
	}
	if got := bus.Read32(addr); got != 0xAABBCCDD {
		t.Fatalf("Read32 = 0x%08X, want handler value", got)
	}
	if got := binary.LittleEndian.Uint32(bus.memory[addr : addr+4]); got != 0xAABBCCDD {
		t.Fatalf("read shadow memory = 0x%08X, want 0xAABBCCDD", got)
	}
}

func TestMapIO64NoShadow_DoesNotMirrorWrite(t *testing.T) {
	bus := NewMachineBus()
	const addr uint32 = 0xF5800
	bus.MapIO64NoShadow(addr, addr+7, nil, func(addr uint32, value uint64) {})

	bus.Write64(addr, 0x1122334455667788)
	if got := binary.LittleEndian.Uint64(bus.memory[addr : addr+8]); got != 0 {
		t.Fatalf("shadow memory = 0x%016X, want unchanged zero", got)
	}
}

func TestMapIO64Shadow_MirrorsWrite(t *testing.T) {
	bus := NewMachineBus()
	const addr uint32 = 0xF5900
	bus.MapIO64Shadow(addr, addr+7, nil, func(addr uint32, value uint64) {})

	bus.Write64(addr, 0x1122334455667788)
	if got := binary.LittleEndian.Uint64(bus.memory[addr : addr+8]); got != 0x1122334455667788 {
		t.Fatalf("shadow memory = 0x%016X, want 0x1122334455667788", got)
	}
}

func TestBusMapSnapshot_ImmutableAcrossMappingUpdates(t *testing.T) {
	bus := NewMachineBus()
	const addr uint32 = 0xF5A00

	bus.MapIO(addr, addr+3, func(addr uint32) uint32 { return 0x11223344 }, nil)
	first := bus.mapState.Load()
	if first == nil {
		t.Fatal("expected initial map snapshot")
	}
	if !first.ioPageBitmap[addr>>8] {
		t.Fatal("snapshot did not publish mapped page")
	}

	bus.MapIOByte(addr, addr+3, func(addr uint32, value uint8) {})
	updated := bus.mapState.Load()
	if updated == first {
		t.Fatal("mapping update reused previous snapshot")
	}
	if first.mapping[addr&PAGE_MASK][0].onWrite8 != nil {
		t.Fatal("previous snapshot was mutated by MapIOByte")
	}
	if updated.mapping[addr&PAGE_MASK][0].onWrite8 == nil {
		t.Fatal("updated snapshot did not include MapIOByte handler")
	}

	bus.UnmapIO(addr, addr+3)
	unmapped := bus.mapState.Load()
	if unmapped == updated {
		t.Fatal("unmap reused previous snapshot")
	}
	if !updated.ioPageBitmap[addr>>8] {
		t.Fatal("previous snapshot lost mapped bitmap bit")
	}
	if unmapped.ioPageBitmap[addr>>8] {
		t.Fatal("unmapped snapshot retained mapped bitmap bit")
	}
}

func TestRead32_StraddlingRAMAndMMIOUsesByteDispatcher(t *testing.T) {
	bus := NewMachineBus()
	const base uint32 = 0xF5BFD
	copy(bus.memory[base:base+3], []byte{0x11, 0x22, 0x33})
	bus.MapIONoShadow(base+3, base+3, nil, nil)
	bus.MapIOByteRead(base+3, base+3, func(addr uint32) uint8 { return 0xAA })

	if got := bus.Read32(base); got != 0xAA332211 {
		t.Fatalf("Read32 straddling RAM/MMIO = 0x%08X, want 0xAA332211", got)
	}
}

func TestWrite32_StraddlingRAMAndMMIOUsesByteDispatcher(t *testing.T) {
	bus := NewMachineBus()
	const base uint32 = 0xF5CFD
	var writes []struct {
		addr  uint32
		value uint8
	}
	bus.MapIONoShadow(base+3, base+3, nil, nil)
	bus.MapIOByte(base+3, base+3, func(addr uint32, value uint8) {
		writes = append(writes, struct {
			addr  uint32
			value uint8
		}{addr: addr, value: value})
	})

	bus.Write32(base, 0xAABBCCDD)
	if got := []byte{bus.memory[base], bus.memory[base+1], bus.memory[base+2]}; string(got) != string([]byte{0xDD, 0xCC, 0xBB}) {
		t.Fatalf("RAM bytes = % X, want DD CC BB", got)
	}
	if len(writes) != 1 || writes[0].addr != base+3 || writes[0].value != 0xAA {
		t.Fatalf("MMIO writes = %+v, want one byte write at high byte", writes)
	}
	if got := bus.memory[base+3]; got != 0 {
		t.Fatalf("NoShadow MMIO byte mirrored into RAM: got 0x%02X", got)
	}
}

func TestRead32WithFault_StraddlingRAMAndMMIOUsesByteDispatcher(t *testing.T) {
	bus := NewMachineBus()
	const base uint32 = 0xF5DFD
	copy(bus.memory[base:base+3], []byte{0x44, 0x55, 0x66})
	bus.MapIONoShadow(base+3, base+3, nil, nil)
	bus.MapIOByteRead(base+3, base+3, func(addr uint32) uint8 { return 0xBB })

	got, ok := bus.Read32WithFault(base)
	if !ok {
		t.Fatal("Read32WithFault straddling RAM/MMIO faulted")
	}
	if got != 0xBB665544 {
		t.Fatalf("Read32WithFault straddling RAM/MMIO = 0x%08X, want 0xBB665544", got)
	}
}

func TestWrite32WithFault_StraddlingRAMAndMMIOUsesByteDispatcher(t *testing.T) {
	bus := NewMachineBus()
	const base uint32 = 0xF5EFD
	var gotAddr uint32
	var gotValue uint8
	var calls int
	bus.MapIONoShadow(base+3, base+3, nil, nil)
	bus.MapIOByte(base+3, base+3, func(addr uint32, value uint8) {
		calls++
		gotAddr = addr
		gotValue = value
	})

	if !bus.Write32WithFault(base, 0x44332211) {
		t.Fatal("Write32WithFault straddling RAM/MMIO faulted")
	}
	if got := []byte{bus.memory[base], bus.memory[base+1], bus.memory[base+2]}; string(got) != string([]byte{0x11, 0x22, 0x33}) {
		t.Fatalf("RAM bytes = % X, want 11 22 33", got)
	}
	if calls != 1 || gotAddr != base+3 || gotValue != 0x44 {
		t.Fatalf("MMIO write calls=%d addr=0x%X value=0x%02X, want one high byte write", calls, gotAddr, gotValue)
	}
}

func TestHasMappedRegion64Span_FullyCovered(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF5000, 0xF5007, func(addr uint32) uint64 { return 0 }, nil)
	if !bus.hasMappedRegion64Span(0xF5000, 8) {
		t.Fatal("hasMappedRegion64Span returned false for fully covered span")
	}
}

func TestHasMappedRegion64Span_FullyUnmapped(t *testing.T) {
	bus := NewMachineBus()
	if bus.hasMappedRegion64Span(0xF5100, 8) {
		t.Fatal("hasMappedRegion64Span returned true for unmapped span")
	}
}

func TestHasMappedRegion64Span_PartialCoverage_ReturnsFalse(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF5200, 0xF5203, func(addr uint32) uint64 { return 0 }, nil)
	if bus.hasMappedRegion64Span(0xF5200, 8) {
		t.Fatal("hasMappedRegion64Span returned true for partially covered span")
	}
}

func TestHasMappedRegion_NarrowExcludesMapping64(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF5300, 0xF5307, func(addr uint32) uint64 { return 0 }, nil)
	if bus.hasMappedRegion(0xF5300) {
		t.Fatal("hasMappedRegion included mapping64; legacy helper must stay narrow")
	}
}

func TestIsIOAddress64_NewAPI(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF5400, 0xF5407, func(addr uint32) uint64 { return 0 }, nil)
	if !bus.IsIOAddress64(0xF5400) {
		t.Fatal("IsIOAddress64 returned false for mapping64-only address")
	}
}

func TestIsIOAddress_LegacyUnchanged(t *testing.T) {
	bus := NewMachineBus()
	bus.MapIO64(0xF5500, 0xF5507, func(addr uint32) uint64 { return 0 }, nil)
	if bus.IsIOAddress(0xF5500) {
		t.Fatal("IsIOAddress returned true for mapping64-only address")
	}
}

// =============================================================================
// Benchmarks for memory bus operations
// =============================================================================

// BenchmarkRead32_NonIO measures read performance for non-I/O addresses
func BenchmarkRead32_NonIO(b *testing.B) {
	bus := NewMachineBus()
	bus.Write32(0x1000, 0x12345678)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read32(0x1000)
	}
}

// BenchmarkRead32_IORegion measures read performance for I/O-mapped addresses
func BenchmarkRead32_IORegion(b *testing.B) {
	bus := NewMachineBus()
	bus.MapIO(0xF0000, 0xF00FF, func(addr uint32) uint32 { return 0x42 }, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read32(0xF0000)
	}
}

// BenchmarkWrite32_NonIO measures write performance for non-I/O addresses
func BenchmarkWrite32_NonIO(b *testing.B) {
	bus := NewMachineBus()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write32(0x1000, uint32(i))
	}
}

// BenchmarkWrite32_IORegion measures write performance for I/O-mapped addresses
func BenchmarkWrite32_IORegion(b *testing.B) {
	bus := NewMachineBus()
	bus.MapIO(0xF0000, 0xF00FF, nil, func(addr uint32, value uint32) {})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write32(0xF0000, uint32(i))
	}
}

// BenchmarkRead16_NonIO measures 16-bit read performance
func BenchmarkRead16_NonIO(b *testing.B) {
	bus := NewMachineBus()
	bus.Write16(0x1000, 0x1234)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read16(0x1000)
	}
}

// BenchmarkWrite16_NonIO measures 16-bit write performance
func BenchmarkWrite16_NonIO(b *testing.B) {
	bus := NewMachineBus()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write16(0x1000, uint16(i))
	}
}

// BenchmarkRead8_NonIO measures 8-bit read performance
func BenchmarkRead8_NonIO(b *testing.B) {
	bus := NewMachineBus()
	bus.Write8(0x1000, 0x42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Read8(0x1000)
	}
}

// BenchmarkWrite8_NonIO measures 8-bit write performance
func BenchmarkWrite8_NonIO(b *testing.B) {
	bus := NewMachineBus()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Write8(0x1000, uint8(i))
	}
}

// =============================================================================
// Regression tests for memory bus correctness
// =============================================================================

// TestRead32_DeferRemoval_Correctness ensures defer removal doesn't break reads
func TestRead32_DeferRemoval_Correctness(t *testing.T) {
	bus := NewMachineBus()
	bus.Write32(0x1000, 0xDEADBEEF)

	// Normal read
	if got := bus.Read32(0x1000); got != 0xDEADBEEF {
		t.Errorf("Read32 = 0x%X, want 0xDEADBEEF", got)
	}

	// Sign-extended address test
	bus.Write32(0x8000, 0xCAFEBABE)
	if got := bus.Read32(0x8000); got != 0xCAFEBABE {
		t.Errorf("Read32(0x8000) = 0x%X, want 0xCAFEBABE", got)
	}
}

// TestRead32_IOCallback_AfterDeferRemoval ensures I/O callbacks still work
func TestRead32_IOCallback_AfterDeferRemoval(t *testing.T) {
	bus := NewMachineBus()
	called := false
	bus.MapIO(0xF0000, 0xF00FF, func(addr uint32) uint32 {
		called = true
		return 0x42
	}, nil)

	result := bus.Read32(0xF0000)
	if !called {
		t.Error("I/O callback not invoked")
	}
	if result != 0x42 {
		t.Errorf("Read32 = 0x%X, want 0x42", result)
	}
}

// TestWrite32_IOCallback_AfterDeferRemoval ensures I/O write callbacks still work
func TestWrite32_IOCallback_AfterDeferRemoval(t *testing.T) {
	bus := NewMachineBus()
	var captured uint32
	bus.MapIO(0xF0000, 0xF00FF, nil, func(addr uint32, value uint32) {
		captured = value
	})

	bus.Write32(0xF0000, 0xABCD1234)
	if captured != 0xABCD1234 {
		t.Errorf("I/O callback captured = 0x%X, want 0xABCD1234", captured)
	}
}

// TestRead32_LockFree_NoIOPage tests reads from pages without I/O mappings
func TestRead32_LockFree_NoIOPage(t *testing.T) {
	bus := NewMachineBus()
	bus.Write32(0x1000, 0x12345678)

	// No I/O mapped at 0x1000, should use lock-free path
	if got := bus.Read32(0x1000); got != 0x12345678 {
		t.Errorf("Read32 = 0x%X, want 0x12345678", got)
	}
}

// TestRead32_StillUsesIO_WhenMapped ensures I/O is still invoked when mapped
func TestRead32_StillUsesIO_WhenMapped(t *testing.T) {
	bus := NewMachineBus()
	callCount := 0
	bus.MapIO(0x1000, 0x10FF, func(addr uint32) uint32 {
		callCount++
		return 0x99
	}, nil)

	result := bus.Read32(0x1000)
	if callCount != 1 {
		t.Errorf("I/O callback called %d times, want 1", callCount)
	}
	if result != 0x99 {
		t.Errorf("Read32 = 0x%X, want 0x99", result)
	}
}

// TestWrite32_LockFree_NoIOPage tests writes to pages without I/O mappings
func TestWrite32_LockFree_NoIOPage(t *testing.T) {
	bus := NewMachineBus()
	bus.Write32(0x2000, 0xABCD1234)

	// Verify via direct memory access
	mem := bus.GetMemory()
	got := binary.LittleEndian.Uint32(mem[0x2000:])
	if got != 0xABCD1234 {
		t.Errorf("Memory = 0x%X, want 0xABCD1234", got)
	}
}

// TestUnsafeRead32_MatchesBinaryEncoding tests unsafe pointer reads
func TestUnsafeRead32_MatchesBinaryEncoding(t *testing.T) {
	bus := NewMachineBus()
	testCases := []uint32{0, 1, 0xFF, 0xFFFF, 0xFFFFFF, 0xFFFFFFFF, 0x12345678}

	for _, want := range testCases {
		bus.Write32(0x1000, want)
		got := bus.Read32(0x1000)
		if got != want {
			t.Errorf("Read32 = 0x%X, want 0x%X", got, want)
		}
	}
}

// TestUnsafeWrite32_MatchesBinaryEncoding tests unsafe pointer writes
func TestUnsafeWrite32_MatchesBinaryEncoding(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	bus.Write32(0x1000, 0x12345678)

	// Verify byte order is little-endian
	if mem[0x1000] != 0x78 || mem[0x1001] != 0x56 ||
		mem[0x1002] != 0x34 || mem[0x1003] != 0x12 {
		t.Errorf("Byte order incorrect: got %02X %02X %02X %02X",
			mem[0x1000], mem[0x1001], mem[0x1002], mem[0x1003])
	}
}

// TestRead16_Correctness tests 16-bit read operations
func TestRead16_Correctness(t *testing.T) {
	bus := NewMachineBus()
	bus.Write16(0x1000, 0xABCD)

	if got := bus.Read16(0x1000); got != 0xABCD {
		t.Errorf("Read16 = 0x%X, want 0xABCD", got)
	}
}

// TestRead8_Correctness tests 8-bit read operations
func TestRead8_Correctness(t *testing.T) {
	bus := NewMachineBus()
	bus.Write8(0x1000, 0x42)

	if got := bus.Read8(0x1000); got != 0x42 {
		t.Errorf("Read8 = 0x%X, want 0x42", got)
	}
}

// TestSignExtendedAddressRead tests sign-extended address handling
func TestSignExtendedAddressRead(t *testing.T) {
	bus := NewMachineBus()
	// Write to low address
	bus.Write32(0x8000, 0xBEEFCAFE)

	// Read via sign-extended address (0xFFFF8000)
	got := bus.Read32(0xFFFF8000)
	if got != 0xBEEFCAFE {
		t.Errorf("Read32(0xFFFF8000) = 0x%X, want 0xBEEFCAFE", got)
	}
}

// TestSignExtendedAddressWrite tests sign-extended address writes
func TestSignExtendedAddressWrite(t *testing.T) {
	bus := NewMachineBus()
	// Write via sign-extended address
	bus.Write32(0xFFFF8000, 0xDEADC0DE)

	// Read via normal address
	got := bus.Read32(0x8000)
	if got != 0xDEADC0DE {
		t.Errorf("Read32(0x8000) = 0x%X, want 0xDEADC0DE", got)
	}
}

// TestConcurrentAccess ensures thread safety after optimizations
func TestConcurrentAccess(t *testing.T) {
	bus := NewMachineBus()
	const iterations = 1000
	var wg sync.WaitGroup

	// Concurrent writers
	for g := range 4 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			base := uint32(id * 0x10000)
			for i := range iterations {
				bus.Write32(base+uint32(i*4), uint32(i))
			}
		}(g)
	}

	// Concurrent readers
	for g := range 4 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			base := uint32(id * 0x10000)
			for i := range iterations {
				_ = bus.Read32(base + uint32(i*4))
			}
		}(g)
	}

	wg.Wait()
}

// TestWithFaultVariants ensures WithFault methods remain correct
func TestWithFaultVariants(t *testing.T) {
	bus := NewMachineBus()

	// Write32WithFault
	ok := bus.Write32WithFault(0x1000, 0x11111111)
	if !ok {
		t.Error("Write32WithFault returned false for valid address")
	}

	// Read32WithFault
	val, ok := bus.Read32WithFault(0x1000)
	if !ok || val != 0x11111111 {
		t.Errorf("Read32WithFault = (0x%X, %v), want (0x11111111, true)", val, ok)
	}

	// Write16WithFault
	ok = bus.Write16WithFault(0x2000, 0x2222)
	if !ok {
		t.Error("Write16WithFault returned false for valid address")
	}

	// Read16WithFault
	val16, ok := bus.Read16WithFault(0x2000)
	if !ok || val16 != 0x2222 {
		t.Errorf("Read16WithFault = (0x%X, %v), want (0x2222, true)", val16, ok)
	}

	// Write8WithFault
	ok = bus.Write8WithFault(0x3000, 0x33)
	if !ok {
		t.Error("Write8WithFault returned false for valid address")
	}

	// Read8WithFault
	val8, ok := bus.Read8WithFault(0x3000)
	if !ok || val8 != 0x33 {
		t.Errorf("Read8WithFault = (0x%X, %v), want (0x33, true)", val8, ok)
	}

	// Out of bounds should return false
	// 0x02000000 is 32MB, which is beyond DEFAULT_MEMORY_SIZE
	ok = bus.Write32WithFault(0x02000000, 0)
	if ok {
		t.Error("Write32WithFault returned true for out-of-bounds address")
	}
}
