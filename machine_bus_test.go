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
