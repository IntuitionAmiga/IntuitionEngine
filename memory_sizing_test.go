// memory_sizing_test.go - Tests for autodetected guest RAM sizing model.
// Implements PLAN_MAX_RAM.md slice 1 acceptance: meminfo parsing, reserve
// policies, page alignment, minimum-RAM enforcement, override hooks, and
// per-CPU/profile visible-ceiling clamping.

package main

import (
	"errors"
	"strings"
	"testing"
)

const tMiB uint64 = 1024 * 1024
const tGiB uint64 = 1024 * 1024 * 1024

// ---------------------------------------------------------------------------
// /proc/meminfo parsing
// ---------------------------------------------------------------------------

func TestParseMeminfo_PrefersMemAvailable(t *testing.T) {
	in := strings.Join([]string{
		"MemTotal:       16384000 kB",
		"MemFree:         1000000 kB",
		"MemAvailable:   12000000 kB",
		"Buffers:          200000 kB",
		"Cached:          3000000 kB",
		"Shmem:            500000 kB",
	}, "\n") + "\n"

	usable, source, err := ParseMeminfo(in)
	if err != nil {
		t.Fatalf("ParseMeminfo: %v", err)
	}
	if source != "MemAvailable" {
		t.Fatalf("source: got %q, want MemAvailable", source)
	}
	want := uint64(12000000) * 1024
	if usable != want {
		t.Fatalf("usable: got %d, want %d", usable, want)
	}
}

func TestParseMeminfo_FallbackWhenAvailableMissing(t *testing.T) {
	in := strings.Join([]string{
		"MemTotal:       16384000 kB",
		"MemFree:         1000000 kB",
		"Buffers:          200000 kB",
		"Cached:          3000000 kB",
		"Shmem:            500000 kB",
	}, "\n") + "\n"

	usable, source, err := ParseMeminfo(in)
	if err != nil {
		t.Fatalf("ParseMeminfo: %v", err)
	}
	if source != "fallback" {
		t.Fatalf("source: got %q, want fallback", source)
	}
	// MemFree + Buffers + Cached - Shmem = 1000000 + 200000 + 3000000 - 500000 = 3700000 kB
	want := uint64(3700000) * 1024
	if usable != want {
		t.Fatalf("usable: got %d, want %d", usable, want)
	}
}

func TestParseMeminfo_FallbackUnderflowClampsZero(t *testing.T) {
	// Shmem larger than MemFree+Buffers+Cached must clamp to zero, not wrap.
	in := strings.Join([]string{
		"MemTotal:       16384000 kB",
		"MemFree:           10000 kB",
		"Buffers:           20000 kB",
		"Cached:            30000 kB",
		"Shmem:           1000000 kB",
	}, "\n") + "\n"

	usable, source, err := ParseMeminfo(in)
	if err != nil {
		t.Fatalf("ParseMeminfo: %v", err)
	}
	if source != "fallback" {
		t.Fatalf("source: got %q, want fallback", source)
	}
	if usable != 0 {
		t.Fatalf("usable: got %d, want 0 (underflow clamp)", usable)
	}
}

func TestParseMeminfo_NoUsefulFields(t *testing.T) {
	if _, _, err := ParseMeminfo("MemTotal: 1000 kB\n"); err == nil {
		t.Fatalf("expected error for meminfo with no usable fields")
	}
}

func TestParseMeminfo_UnknownUnit_OnUsedField_Errors(t *testing.T) {
	_, _, err := ParseMeminfo("MemAvailable: 12000000 pages\n")
	if err == nil {
		t.Fatal("expected error for unknown unit on consumed MemAvailable field")
	}
}

func TestParseMeminfo_UnknownUnit_OnIgnoredField_OK(t *testing.T) {
	in := strings.Join([]string{
		"MemAvailable:   12000000 kB",
		"FutureKernel:   999 widgets",
	}, "\n") + "\n"
	usable, source, err := ParseMeminfo(in)
	if err != nil {
		t.Fatalf("ParseMeminfo: %v", err)
	}
	if source != "MemAvailable" {
		t.Fatalf("source=%q, want MemAvailable", source)
	}
	if usable != 12000000*1024 {
		t.Fatalf("usable=%d, want %d", usable, 12000000*1024)
	}
}

// ---------------------------------------------------------------------------
// Size argument parsing (KiB/MiB/GiB)
// ---------------------------------------------------------------------------

func TestParseSizeArg(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
		fail bool
	}{
		{"0", 0, false},
		{"1024", 1024, false},
		{"4KiB", 4 * 1024, false},
		{"4 KiB", 4 * 1024, false},
		{"32MiB", 32 * tMiB, false},
		{"2GiB", 2 * tGiB, false},
		{"5gib", 5 * tGiB, false},
		{"-1", 0, true},
		{"abc", 0, true},
		{"1TB", 0, true},
	}
	for _, c := range cases {
		got, err := ParseSizeArg(c.in)
		if c.fail {
			if err == nil {
				t.Errorf("%q: expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %d, want %d", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Reserve policies
// ---------------------------------------------------------------------------

func TestReserveFor_RaspberryPi64(t *testing.T) {
	// 4 GiB usable: 25% = 1 GiB > 768 MiB floor -> 1 GiB.
	got, err := ReserveFor(PlatformRaspberryPi64, 4*tGiB)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1*tGiB {
		t.Fatalf("4 GiB usable: got %d, want %d", got, 1*tGiB)
	}
	// 1 GiB usable: 25% = 256 MiB < 768 MiB floor -> 768 MiB.
	got, err = ReserveFor(PlatformRaspberryPi64, 1*tGiB)
	if err != nil {
		t.Fatal(err)
	}
	if got != 768*tMiB {
		t.Fatalf("1 GiB usable: got %d, want %d", got, 768*tMiB)
	}
}

func TestReserveFor_X64PC(t *testing.T) {
	// 16 GiB usable: 20% = 3.2 GiB > 1 GiB floor.
	got, err := ReserveFor(PlatformX64PC, 16*tGiB)
	if err != nil {
		t.Fatal(err)
	}
	want := (16 * tGiB) / 5
	if got != want {
		t.Fatalf("16 GiB usable: got %d, want %d", got, want)
	}
	// 2 GiB usable: 20% = 400 MiB < 1 GiB floor.
	got, err = ReserveFor(PlatformX64PC, 2*tGiB)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1*tGiB {
		t.Fatalf("2 GiB usable: got %d, want %d", got, 1*tGiB)
	}
}

func TestReserveFor_AppleSiliconLinux(t *testing.T) {
	// 8 GiB usable: 25% = 2 GiB > 1.5 GiB floor.
	got, err := ReserveFor(PlatformAppleSiliconLinux, 8*tGiB)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2*tGiB {
		t.Fatalf("8 GiB usable: got %d, want %d", got, 2*tGiB)
	}
	// 4 GiB usable: 25% = 1 GiB < 1.5 GiB floor.
	got, err = ReserveFor(PlatformAppleSiliconLinux, 4*tGiB)
	if err != nil {
		t.Fatal(err)
	}
	if got != (3*tGiB)/2 {
		t.Fatalf("4 GiB usable: got %d, want %d", got, (3*tGiB)/2)
	}
}

func TestReserveFor_UnknownFails(t *testing.T) {
	if _, err := ReserveFor(PlatformUnknown, 4*tGiB); err == nil {
		t.Fatalf("expected error for unknown platform")
	}
}

// ---------------------------------------------------------------------------
// ComputeMemorySizing core invariants
// ---------------------------------------------------------------------------

func TestComputeMemorySizing_AppliesReserveThenClamps(t *testing.T) {
	// 16 GiB usable on x64 PC, IE64 ceiling = 32 GiB (no clamp on total).
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16 * tGiB,
	}
	ms, err := ComputeMemorySizing(32*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	wantReserve := (16 * tGiB) / 5
	if ms.HostReserve != wantReserve {
		t.Errorf("HostReserve: got %d, want %d", ms.HostReserve, wantReserve)
	}
	wantTotal := (16*tGiB - wantReserve) &^ uint64(MMU_PAGE_SIZE-1)
	if ms.TotalGuestRAM != wantTotal {
		t.Errorf("TotalGuestRAM: got %d, want %d", ms.TotalGuestRAM, wantTotal)
	}
	if ms.ActiveVisibleRAM != wantTotal {
		t.Errorf("ActiveVisibleRAM: got %d, want %d", ms.ActiveVisibleRAM, wantTotal)
	}
}

func TestComputeMemorySizing_ClampsToCeiling(t *testing.T) {
	// IE32 ceiling 4 GiB; total guest RAM far exceeds it.
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16 * tGiB,
	}
	ms, err := ComputeMemorySizing(4*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.ActiveVisibleRAM != 4*tGiB {
		t.Errorf("ActiveVisibleRAM: got %d, want %d", ms.ActiveVisibleRAM, 4*tGiB)
	}
	if ms.TotalGuestRAM <= ms.ActiveVisibleRAM && ms.TotalGuestRAM != ms.ActiveVisibleRAM {
		// expected total > active when ceiling clamps
	}
	if ms.TotalGuestRAM < ms.ActiveVisibleRAM {
		t.Errorf("invariant: total %d < active %d", ms.TotalGuestRAM, ms.ActiveVisibleRAM)
	}
}

func TestComputeMemorySizing_PageAligned(t *testing.T) {
	// Build a deliberately misaligned scenario: detected usable not page-aligned
	// after reserve subtraction must still yield page-aligned outputs.
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16*tGiB + 123, // unaligned
	}
	ms, err := ComputeMemorySizing(8*tGiB+777, ov) // unaligned ceiling
	if err != nil {
		t.Fatal(err)
	}
	if ms.TotalGuestRAM%uint64(MMU_PAGE_SIZE) != 0 {
		t.Errorf("TotalGuestRAM %d not page-aligned", ms.TotalGuestRAM)
	}
	if ms.ActiveVisibleRAM%uint64(MMU_PAGE_SIZE) != 0 {
		t.Errorf("ActiveVisibleRAM %d not page-aligned", ms.ActiveVisibleRAM)
	}
	if ms.VisibleCeiling%uint64(MMU_PAGE_SIZE) != 0 {
		t.Errorf("VisibleCeiling %d not page-aligned", ms.VisibleCeiling)
	}
}

func TestComputeMemorySizing_MinRAMEnforced(t *testing.T) {
	// 1 GiB usable on x64 PC: reserve floor is 1 GiB, leaves 0 -> error.
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 1 * tGiB,
	}
	_, err := ComputeMemorySizing(4*tGiB, ov)
	if err == nil {
		t.Fatal("expected insufficient-memory error")
	}
	if !errors.Is(err, ErrInsufficientHostRAM) {
		t.Errorf("error: got %v, want ErrInsufficientHostRAM", err)
	}
}

func TestComputeMemorySizing_ReserveExceedsUsableNoUnderflow(t *testing.T) {
	// host reserve >= detected usable -> clear error, never wrap.
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 256 * tMiB,
	}
	_, err := ComputeMemorySizing(4*tGiB, ov)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInsufficientHostRAM) {
		t.Errorf("error: got %v, want ErrInsufficientHostRAM", err)
	}
}

func TestComputeMemorySizing_BelowMinFloorFails(t *testing.T) {
	// Reserve override leaves 16 MiB -> below MIN_GUEST_RAM (32 MiB).
	ov := SizingOverrides{
		Platform:            PlatformX64PC,
		DetectedUsableRAM:   64 * tMiB,
		HostReserveBytes:    48 * tMiB,
		HostReserveExplicit: true,
	}
	_, err := ComputeMemorySizing(4*tGiB, ov)
	if err == nil {
		t.Fatal("expected min-RAM error")
	}
	if !errors.Is(err, ErrGuestRAMBelowMinimum) {
		t.Errorf("error: got %v, want ErrGuestRAMBelowMinimum", err)
	}
}

// ---------------------------------------------------------------------------
// Override hooks
// ---------------------------------------------------------------------------

func TestComputeMemorySizing_OverrideTotalGuestRAM(t *testing.T) {
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16 * tGiB,
		TotalGuestRAM:     2 * tGiB,
	}
	ms, err := ComputeMemorySizing(4*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.TotalGuestRAM != 2*tGiB {
		t.Errorf("TotalGuestRAM override: got %d, want %d", ms.TotalGuestRAM, 2*tGiB)
	}
	if ms.ActiveVisibleRAM != 2*tGiB {
		t.Errorf("ActiveVisibleRAM should clamp to total: got %d", ms.ActiveVisibleRAM)
	}
}

func TestComputeMemorySizing_OverrideActiveVisible(t *testing.T) {
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16 * tGiB,
		ActiveVisibleRAM:  1 * tGiB,
	}
	ms, err := ComputeMemorySizing(8*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.ActiveVisibleRAM != 1*tGiB {
		t.Errorf("ActiveVisibleRAM override: got %d, want %d", ms.ActiveVisibleRAM, 1*tGiB)
	}
	if ms.ActiveVisibleRAM > ms.TotalGuestRAM {
		t.Errorf("invariant violated: active %d > total %d", ms.ActiveVisibleRAM, ms.TotalGuestRAM)
	}
}

func TestComputeMemorySizing_OverrideActiveClampedToCeiling(t *testing.T) {
	// 8 GiB active override against a 4 GiB CPU/profile ceiling must clamp,
	// because the active-visible-RAM invariant says active <= ceiling unless
	// an explicit fake/sparse impossible-state backend is requested.
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 32 * tGiB,
		ActiveVisibleRAM:  8 * tGiB,
	}
	ms, err := ComputeMemorySizing(4*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.ActiveVisibleRAM != 4*tGiB {
		t.Errorf("ActiveVisibleRAM should clamp to ceiling: got %d, want %d",
			ms.ActiveVisibleRAM, 4*tGiB)
	}
}

func TestComputeMemorySizing_OverrideActiveExceedsCeiling_AllowedWithImpossibleFlag(t *testing.T) {
	// AllowImpossibleState lets active exceed both the ceiling and total
	// for fake/sparse backends.
	ov := SizingOverrides{
		Platform:             PlatformX64PC,
		DetectedUsableRAM:    32 * tGiB,
		ActiveVisibleRAM:     8 * tGiB,
		AllowImpossibleState: true,
	}
	ms, err := ComputeMemorySizing(4*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.ActiveVisibleRAM != 8*tGiB {
		t.Errorf("ActiveVisibleRAM: got %d, want %d", ms.ActiveVisibleRAM, 8*tGiB)
	}
}

func TestComputeMemorySizing_OverrideActiveExceedsTotal_RejectedByDefault(t *testing.T) {
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16 * tGiB,
		TotalGuestRAM:     1 * tGiB,
		ActiveVisibleRAM:  2 * tGiB,
	}
	_, err := ComputeMemorySizing(8*tGiB, ov)
	if err == nil {
		t.Fatal("expected error: active > total without AllowImpossibleState")
	}
}

func TestComputeMemorySizing_OverrideActiveExceedsTotal_AllowedWithFlag(t *testing.T) {
	ov := SizingOverrides{
		Platform:             PlatformX64PC,
		DetectedUsableRAM:    16 * tGiB,
		TotalGuestRAM:        1 * tGiB,
		ActiveVisibleRAM:     2 * tGiB,
		AllowImpossibleState: true,
	}
	ms, err := ComputeMemorySizing(8*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.ActiveVisibleRAM != 2*tGiB || ms.TotalGuestRAM != 1*tGiB {
		t.Errorf("got total=%d active=%d", ms.TotalGuestRAM, ms.ActiveVisibleRAM)
	}
}

func TestComputeMemorySizing_OverrideHostReserve(t *testing.T) {
	ov := SizingOverrides{
		Platform:            PlatformX64PC,
		DetectedUsableRAM:   8 * tGiB,
		HostReserveBytes:    512 * tMiB,
		HostReserveExplicit: true,
	}
	ms, err := ComputeMemorySizing(8*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.HostReserve != 512*tMiB {
		t.Errorf("HostReserve: got %d, want %d", ms.HostReserve, 512*tMiB)
	}
	if ms.TotalGuestRAM != 8*tGiB-512*tMiB {
		t.Errorf("TotalGuestRAM: got %d, want %d", ms.TotalGuestRAM, 8*tGiB-512*tMiB)
	}
}

func TestComputeMemorySizing_OverrideTotalBelowMinFails(t *testing.T) {
	ov := SizingOverrides{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: 16 * tGiB,
		TotalGuestRAM:     4 * tMiB, // below 32 MiB floor
	}
	_, err := ComputeMemorySizing(4*tGiB, ov)
	if err == nil {
		t.Fatal("expected min-RAM error")
	}
	if !errors.Is(err, ErrGuestRAMBelowMinimum) {
		t.Errorf("error: got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Platform classification fail behavior
// ---------------------------------------------------------------------------

func TestComputeMemorySizing_UnsupportedPlatformFails(t *testing.T) {
	ov := SizingOverrides{
		Platform:          PlatformUnknown,
		DetectedUsableRAM: 16 * tGiB,
	}
	_, err := ComputeMemorySizing(4*tGiB, ov)
	if err == nil {
		t.Fatal("expected unsupported-platform error")
	}
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("error: got %v, want ErrUnsupportedPlatform", err)
	}
}

func TestComputeMemorySizing_UnknownPlatformAcceptedWithSkipFlag(t *testing.T) {
	ov := SizingOverrides{
		Platform:            PlatformUnknown,
		DetectedUsableRAM:   16 * tGiB,
		SkipPlatformCheck:   true,
		HostReserveBytes:    1 * tGiB,
		HostReserveExplicit: true,
	}
	ms, err := ComputeMemorySizing(4*tGiB, ov)
	if err != nil {
		t.Fatal(err)
	}
	if ms.TotalGuestRAM != 15*tGiB {
		t.Errorf("TotalGuestRAM: got %d, want %d", ms.TotalGuestRAM, 15*tGiB)
	}
}
