// debug_access_pagefilter_test.go - correctness pins for the debug access pre-filter.
//
// The filter is only allowed to be wrong in one direction. These tests attack
// the other direction: anything the unfiltered scan would have reported must
// still be reported.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"math/rand"
	"testing"
)

// newFilterTestService builds a service with a registered CPU whose events are
// collected, so a hit is observable rather than inferred.
func newFilterTestService(cpuID int) (*DebugAccessService, chan BreakpointEvent) {
	access := NewDebugAccessService()
	events := make(chan BreakpointEvent, 4096)
	access.RegisterCPU(cpuID, events)
	return access, events
}

// TestPageCapabilityBitmap_ConservativeNeverFalseNegative drives randomised
// watchpoint and guard sets against randomised accesses, and requires that
// every access the filter rejects would also have been rejected by the full
// scan. The full scan is the oracle: it is run on a second service that has the
// same watch and guard sets but whose filter is forced to all.
func TestPageCapabilityBitmap_ConservativeNeverFalseNegative(t *testing.T) {
	rng := rand.New(rand.NewSource(20260723))
	const cpuID = 3

	for iteration := range 200 {
		filtered, filteredEvents := newFilterTestService(cpuID)
		oracle, oracleEvents := newFilterTestService(cpuID)

		for range 1 + rng.Intn(6) {
			addr := randomFilterAddr(rng)
			width := 1 << rng.Intn(3)
			typ := []WatchpointType{WatchRead, WatchWrite, WatchReadWrite}[rng.Intn(3)]
			filtered.Watch(cpuID, addr, width, typ)
			oracle.Watch(cpuID, addr, width, typ)
		}
		if rng.Intn(2) == 0 {
			start := uint64(rng.Intn(0x20000))
			end := start + uint64(rng.Intn(0x400))
			perm := AccessPerm(1 + rng.Intn(7))
			scope := GuardScope{CPUID: cpuID}
			if rng.Intn(4) == 0 {
				scope.AllCPUs = true
			}
			filtered.Guard(start, end, perm, scope)
			oracle.Guard(start, end, perm, scope)
		}

		// Force the oracle to run the full scan for every access.
		oracle.pageFilter.Store(&debugPageFilter{all: true})

		for range 400 {
			addr := randomFilterAddr(rng)
			width := 1 << rng.Intn(3)
			kind := []AccessKind{AccessRead, AccessWrite, AccessExecute}[rng.Intn(3)]
			filtered.OnAccess(cpuID, addr, width, kind, 0, 0)
			oracle.OnAccess(cpuID, addr, width, kind, 0, 0)

			got := drainEvents(filteredEvents)
			want := drainEvents(oracleEvents)
			if got != want {
				t.Fatalf("iteration %d: filter changed the outcome for addr 0x%X width %d kind %v: filtered=%d unfiltered=%d",
					iteration, addr, width, kind, got, want)
			}
		}
	}
}

// randomFilterAddr biases half its results to within a few bytes of a page
// boundary. Uniform addresses almost never straddle one, so a filter that
// forgot to check the second page of a spanning access would go unnoticed.
func randomFilterAddr(rng *rand.Rand) uint64 {
	if rng.Intn(2) == 0 {
		return uint64(rng.Intn(0x20000))
	}
	page := uint64(rng.Intn(0x200)) << debugPageShift
	offset := int64(rng.Intn(15)) - 7
	addr := int64(page) + offset
	if addr < 0 {
		addr = 0
	}
	return uint64(addr)
}

func drainEvents(ch chan BreakpointEvent) int {
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			return count
		}
	}
}

// TestPageCapabilityBitmap_HistoryDisablesFiltering pins the one case where the
// filter must not filter: history records every access, coverage or not.
func TestPageCapabilityBitmap_HistoryDisablesFiltering(t *testing.T) {
	access := NewDebugAccessService()
	access.EnableHistory(16)
	if !access.mayAffect(0x123456, 4) {
		t.Fatal("history enabled but the filter rejected an access")
	}
	access.OnRead(-1, 0x123456, 4)
	if len(access.HistoryTail(0)) != 1 {
		t.Fatalf("history did not record the access: %d entries", len(access.HistoryTail(0)))
	}
	access.DisableHistory()
	access.Watch(0, 0x1000, 1, WatchWrite)
	if access.mayAffect(0x123456, 4) {
		t.Fatal("history disabled and the page is unwatched, but the filter admitted the access")
	}
}

// TestPageCapabilityBitmap_RepublishedOnWatchRemoval proves the filter tracks
// removals, not just additions. A stale filter would be conservative and so
// still correct, but it would also silently give the performance back.
func TestPageCapabilityBitmap_RepublishedOnWatchRemoval(t *testing.T) {
	access := NewDebugAccessService()
	access.Watch(0, 0x8000, 4, WatchWrite)
	if !access.mayAffect(0x8000, 4) {
		t.Fatal("watched address rejected by the filter")
	}
	access.Watch(0, 0x9000, 4, WatchWrite)
	access.ClearWatch(0, 0x8000)
	if access.mayAffect(0x8000, 4) {
		t.Fatal("filter still covers a cleared watchpoint page")
	}
	if !access.mayAffect(0x9000, 4) {
		t.Fatal("filter lost a watchpoint that is still set")
	}
}

// TestPageCapabilityBitmap_BroadGuardFallsBackToAll pins the representation
// limit: a guard too broad for the bitmap must widen the filter, never narrow
// it.
func TestPageCapabilityBitmap_BroadGuardFallsBackToAll(t *testing.T) {
	access := NewDebugAccessService()
	access.Guard(0, 0xFFFFFFFFFFFFFFF, PermWrite, GuardScope{AllCPUs: true})
	filter := access.pageFilter.Load()
	if filter == nil || !filter.all {
		t.Fatal("a guard spanning the address space did not widen the filter to all")
	}
	if !access.mayAffect(0x1234, 4) {
		t.Fatal("access rejected while a whole-space guard is set")
	}
}

// TestPageCapabilityBitmap_SpanningAccessChecksBothPages covers an access that
// straddles a page boundary with only the second page watched.
func TestPageCapabilityBitmap_SpanningAccessChecksBothPages(t *testing.T) {
	access := NewDebugAccessService()
	access.Watch(0, 0x100, 1, WatchWrite)
	if !access.mayAffect(0xFE, 4) {
		t.Fatal("access straddling into a watched page was rejected")
	}
	if access.mayAffect(0xF8, 4) {
		t.Fatal("access wholly inside an unwatched page was admitted")
	}
}
