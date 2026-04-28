// bus_production_boot_test.go - PLAN_MAX_RAM slice 10e TDD coverage.
//
// Pins bootGuestRAMFromComputed: backing only allocated when total
// exceeds bus.memory; retry-shrink coherence; ErrHighRangeBacking-
// Unsupported soft-fallback; ApplyProfileVisibleCeiling clamping;
// SYSINFO ordering after ApplyProfileVisibleCeiling.

package main

import (
	"errors"
	"fmt"
	"testing"
)

const sixtyFourMiB uint64 = 64 * 1024 * 1024

func newBus64MiB(t *testing.T) *MachineBus {
	t.Helper()
	bus, err := NewMachineBusSized(sixtyFourMiB)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	return bus
}

func sparseAllocator(size uint64) (Backing, error) {
	return NewSparseBacking(size), nil
}

// mmapFaithfulSparseAllocator mirrors the production NewMmapBacking
// boot-clamp semantics: any request above busMemBootClamp returns
// ErrHighRangeBackingUnsupported (the non-mmap-platform contract). Used
// by boot-simulation tests so coverage stays platform-faithful — without
// this, tests using a plain sparseAllocator would silently install
// multi-GiB backings on a non-mmap platform where production code would
// soft-fall back to the bus.memory window.
func mmapFaithfulSparseAllocator(size uint64) (Backing, error) {
	if size > busMemBootClamp {
		return nil, ErrHighRangeBackingUnsupported
	}
	return NewSparseBacking(size), nil
}

func TestBootGuestRAM_AllocatesBackingWhenTotalExceedsBusMem(t *testing.T) {
	bus := newBus64MiB(t)
	ms := MemorySizing{TotalGuestRAM: 256 * 1024 * 1024}
	pub, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, sparseAllocator)
	if err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	if bus.Backing() == nil {
		t.Fatal("expected backing != nil for total > len(bus.memory)")
	}
	if got := bus.Backing().Size(); got != 256*1024*1024 {
		t.Fatalf("backing.Size = %d, want 256 MiB (absolute upper bound)", got)
	}
	if got := bus.TotalGuestRAM(); got != 256*1024*1024 {
		t.Fatalf("TotalGuestRAM = %d, want 256 MiB", got)
	}
	if pub.TotalGuestRAM != 256*1024*1024 {
		t.Fatalf("published.TotalGuestRAM = %d, want 256 MiB", pub.TotalGuestRAM)
	}
}

func TestBootGuestRAM_NoBackingWhenTotalFitsInBusMem(t *testing.T) {
	bus := newBus64MiB(t)
	ms := MemorySizing{TotalGuestRAM: sixtyFourMiB}
	if _, err := bootGuestRAMFromComputed(bus, ms, sixtyFourMiB, sparseAllocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	if bus.Backing() != nil {
		t.Fatal("expected backing == nil when total fits in bus.memory")
	}
	if got := bus.TotalGuestRAM(); got != sixtyFourMiB {
		t.Fatalf("TotalGuestRAM = %d, want %d", got, sixtyFourMiB)
	}
}

func TestBootGuestRAM_PreservesMemorySizingDiagnostics(t *testing.T) {
	bus := newBus64MiB(t)
	ms := MemorySizing{
		TotalGuestRAM:     256 * 1024 * 1024,
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 8 * 1024 * 1024 * 1024,
		HostReserve:       1 * 1024 * 1024 * 1024,
		MeminfoSource:     "MemAvailable",
	}
	pub, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, sparseAllocator)
	if err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	if pub.Platform != PlatformX64PC {
		t.Errorf("Platform lost: %v", pub.Platform)
	}
	if pub.DetectedUsableRAM != 8*1024*1024*1024 {
		t.Errorf("DetectedUsableRAM lost: %d", pub.DetectedUsableRAM)
	}
	if pub.HostReserve != 1*1024*1024*1024 {
		t.Errorf("HostReserve lost: %d", pub.HostReserve)
	}
	if pub.MeminfoSource != "MemAvailable" {
		t.Errorf("MeminfoSource lost: %q", pub.MeminfoSource)
	}
}

func TestBootGuestRAM_LeavesBusUntouchedOnAllocFailure(t *testing.T) {
	bus := newBus64MiB(t)
	ms := MemorySizing{TotalGuestRAM: 256 * 1024 * 1024}
	failing := func(size uint64) (Backing, error) {
		return nil, fmt.Errorf("simulated allocator failure")
	}
	_, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, failing)
	if err == nil {
		t.Fatal("expected error from failing allocator")
	}
	if bus.TotalGuestRAM() != 0 {
		t.Errorf("TotalGuestRAM = %d, want 0 (bus untouched on failure)", bus.TotalGuestRAM())
	}
	if bus.Backing() != nil {
		t.Errorf("backing != nil; want nil on allocator failure")
	}
}

// TestBootGuestRAM_RetryShrinksAndStillBacks: allocator fails above 128
// MiB, succeeds below — AllocateBacking lands on 128 MiB, which still
// exceeds bus.memory (64 MiB). Backing is installed.
func TestBootGuestRAM_RetryShrinksAndStillBacks(t *testing.T) {
	bus := newBus64MiB(t)
	allocator := func(size uint64) (Backing, error) {
		if size > 128*1024*1024 {
			return nil, fmt.Errorf("simulated VA fragmentation at size=%d", size)
		}
		return NewSparseBacking(size), nil
	}
	ms := MemorySizing{TotalGuestRAM: 256 * 1024 * 1024}
	if _, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, allocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	if bus.Backing() == nil {
		t.Fatal("expected backing installed after retry-shrink to 128 MiB")
	}
	if got := bus.TotalGuestRAM(); got != 128*1024*1024 {
		t.Fatalf("TotalGuestRAM = %d, want 128 MiB (retry-shrink result)", got)
	}
}

// closeRecorder is a Backing wrapper that records Close() invocations.
type closeRecorder struct {
	Backing
	closed int
}

func (c *closeRecorder) Close() error {
	c.closed++
	return c.Backing.Close()
}

func TestBootGuestRAM_RetryShrinksBelowBusMem_DropsBacking(t *testing.T) {
	bus := newBus64MiB(t)
	allocator := func(size uint64) (Backing, error) {
		if size > 32*1024*1024 {
			return nil, fmt.Errorf("simulated VA fragmentation at size=%d", size)
		}
		return NewSparseBacking(size), nil
	}
	ms := MemorySizing{TotalGuestRAM: 256 * 1024 * 1024}
	if _, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, allocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	if bus.Backing() != nil {
		t.Fatal("expected backing dropped when retry-shrink lands at-or-below bus.memory")
	}
	if got := bus.TotalGuestRAM(); got != sixtyFourMiB {
		t.Fatalf("TotalGuestRAM = %d, want %d (clamped to bus.memory)", got, sixtyFourMiB)
	}
}

func TestBootGuestRAM_RetryShrinksBelowBusMem_ClosesBacking(t *testing.T) {
	bus := newBus64MiB(t)
	var recorder *closeRecorder
	allocator := func(size uint64) (Backing, error) {
		if size > 32*1024*1024 {
			return nil, fmt.Errorf("simulated failure at size=%d", size)
		}
		recorder = &closeRecorder{Backing: NewSparseBacking(size)}
		return recorder, nil
	}
	ms := MemorySizing{TotalGuestRAM: 256 * 1024 * 1024}
	if _, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, allocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	if recorder == nil {
		t.Fatal("recorder nil; allocator never produced a backing")
	}
	if recorder.closed != 1 {
		t.Fatalf("backing.Close() invoked %d times, want 1", recorder.closed)
	}
}

func TestBootGuestRAM_HighRangeUnsupportedSentinel_NoRetry_SoftFallback(t *testing.T) {
	bus := newBus64MiB(t)
	calls := 0
	allocator := func(size uint64) (Backing, error) {
		calls++
		return nil, ErrHighRangeBackingUnsupported
	}
	ms := MemorySizing{TotalGuestRAM: 256 * 1024 * 1024}
	if _, err := bootGuestRAMFromComputed(bus, ms, 256*1024*1024, allocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed should soft-fallback; got %v", err)
	}
	if calls != 1 {
		t.Fatalf("allocator invoked %d times, want exactly 1 (sentinel is non-retryable)", calls)
	}
	if bus.Backing() != nil {
		t.Fatal("expected backing == nil after soft-fallback")
	}
	if got := bus.TotalGuestRAM(); got != sixtyFourMiB {
		t.Fatalf("TotalGuestRAM = %d, want %d (clamped to bus.memory)", got, sixtyFourMiB)
	}
}

// ===========================================================================
// ApplyProfileVisibleCeiling
// ===========================================================================

func TestApplyProfileVisibleCeiling_ClampsToTotal(t *testing.T) {
	bus := newBus64MiB(t)
	bus.SetSizing(MemorySizing{TotalGuestRAM: sixtyFourMiB})
	bus.ApplyProfileVisibleCeiling(256 * 1024 * 1024) // larger than total
	if got := bus.ActiveVisibleRAM(); got != sixtyFourMiB {
		t.Fatalf("ActiveVisibleRAM = %d, want %d (clamped to total)", got, sixtyFourMiB)
	}
}

func TestApplyProfileVisibleCeiling_ClampsBelowTotal(t *testing.T) {
	bus := newBus64MiB(t)
	bus.SetSizing(MemorySizing{TotalGuestRAM: sixtyFourMiB})
	bus.ApplyProfileVisibleCeiling(32 * 1024 * 1024)
	if got := bus.ActiveVisibleRAM(); got != 32*1024*1024 {
		t.Fatalf("ActiveVisibleRAM = %d, want 32 MiB", got)
	}
}

func TestApplyProfileVisibleCeiling_PreservesActiveBelowTotalIdentity(t *testing.T) {
	bus := newBus64MiB(t)
	bus.SetSizing(MemorySizing{TotalGuestRAM: sixtyFourMiB})
	bus.ApplyProfileVisibleCeiling(sixtyFourMiB)
	if got := bus.ActiveVisibleRAM(); got != sixtyFourMiB {
		t.Fatalf("ActiveVisibleRAM = %d, want %d (identity)", got, sixtyFourMiB)
	}
	if got := bus.VisibleCeiling(); got != sixtyFourMiB {
		t.Fatalf("VisibleCeiling = %d, want %d", got, sixtyFourMiB)
	}
}

// TestDiscovery_SysinfoActiveRAM_AfterApplyProfileVisibleCeiling pins
// the boot ordering: SYSINFO must be registered AFTER ApplyProfileVisible-
// Ceiling so the snapshot picks up the final active value, not the zero
// placeholder bootGuestRAMFromComputed leaves behind.
func TestDiscovery_SysinfoActiveRAM_AfterApplyProfileVisibleCeiling(t *testing.T) {
	bus := newBus64MiB(t)
	ms := MemorySizing{TotalGuestRAM: sixtyFourMiB}
	if _, err := bootGuestRAMFromComputed(bus, ms, sixtyFourMiB, sparseAllocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	bus.ApplyProfileVisibleCeiling(32 * 1024 * 1024)
	RegisterSysInfoMMIOFromBus(bus)

	lo := bus.Read32(SYSINFO_ACTIVE_RAM_LO)
	hi := bus.Read32(SYSINFO_ACTIVE_RAM_HI)
	got := uint64(lo) | (uint64(hi) << 32)
	if got != 32*1024*1024 {
		t.Fatalf("SYSINFO ACTIVE RAM = %d, want 32 MiB (ordering: ApplyProfile then Register)", got)
	}
}

// TestAllocateBacking_HighRangeUnsupportedSentinel_NoRetry already covers
// the lower-level AllocateBacking layer in memory_backing_mmap_test.go.
// errors.Is is exercised here to keep the package import lifecycle
// honest:
var _ = errors.Is
