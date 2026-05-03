package main

import (
	"fmt"
	"sort"
	"sync"
)

func cloneCondition(cond *BreakpointCondition) *BreakpointCondition {
	if cond == nil {
		return nil
	}
	cp := *cond
	return &cp
}

func cloneBreakpoint(bp *ConditionalBreakpoint) ConditionalBreakpoint {
	if bp == nil {
		return ConditionalBreakpoint{}
	}
	cp := *bp
	cp.Condition = cloneCondition(bp.Condition)
	return cp
}

func snapshotBreakpointLocked(mu *sync.RWMutex, breakpoints map[uint64]*ConditionalBreakpoint, addr uint64) (BreakpointSnapshot, bool) {
	mu.RLock()
	defer mu.RUnlock()
	bp, ok := breakpoints[addr]
	if !ok {
		return BreakpointSnapshot{}, false
	}
	return cloneBreakpoint(bp), true
}

func incrementBreakpointHitLocked(mu *sync.RWMutex, breakpoints map[uint64]*ConditionalBreakpoint, addr uint64) (uint64, bool) {
	mu.Lock()
	defer mu.Unlock()
	bp, ok := breakpoints[addr]
	if !ok {
		return 0, false
	}
	bp.HitCount++
	return bp.HitCount, true
}

func setBreakpointConditionLocked(mu *sync.RWMutex, breakpoints map[uint64]*ConditionalBreakpoint, addr uint64, cond *BreakpointCondition) bool {
	mu.Lock()
	defer mu.Unlock()
	bp, ok := breakpoints[addr]
	if !ok {
		return false
	}
	bp.Condition = cloneCondition(cond)
	return true
}

func listBreakpointSnapshotsLocked(mu *sync.RWMutex, breakpoints map[uint64]*ConditionalBreakpoint) []BreakpointSnapshot {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]BreakpointSnapshot, 0, len(breakpoints))
	for _, bp := range breakpoints {
		out = append(out, cloneBreakpoint(bp))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

func snapshotWatchpointLocked(mu *sync.RWMutex, watchpoints map[uint64]*Watchpoint, addr uint64) (WatchpointSnapshot, bool) {
	mu.RLock()
	defer mu.RUnlock()
	wp, ok := watchpoints[addr]
	if !ok {
		return WatchpointSnapshot{}, false
	}
	return *wp, true
}

func updateWatchpointLastValueLocked(mu *sync.RWMutex, watchpoints map[uint64]*Watchpoint, addr uint64, val byte) bool {
	mu.Lock()
	defer mu.Unlock()
	wp, ok := watchpoints[addr]
	if !ok {
		return false
	}
	wp.LastValue = val
	return true
}

func listWatchpointSnapshotsLocked(mu *sync.RWMutex, watchpoints map[uint64]*Watchpoint) []WatchpointSnapshot {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]WatchpointSnapshot, 0, len(watchpoints))
	for _, wp := range watchpoints {
		out = append(out, *wp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

func validateAddressWidth(cpuName string, width int, addr uint64) error {
	if width >= 64 {
		return nil
	}
	max := uint64(1<<uint(width)) - 1
	if addr > max {
		return fmt.Errorf("%s address $%X exceeds %d-bit address space (max $%X)", cpuName, addr, width, max)
	}
	return nil
}
