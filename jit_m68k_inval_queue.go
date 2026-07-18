// jit_m68k_inval_queue.go - cross-thread M68020 JIT invalidation queue.
//
// Moved out of jit_m68k_exec.go (M68K JIT parity plan, milestone 3): the
// enqueue/drain/coalesce machinery and the bus-level entry point are pure
// queue-and-generation logic over shared M68KCPU state, so every M68020
// backend (amd64, arm64, wasm) shares them. The per-backend pieces are
// cpu.invalidateM68KJITForGuestWrite (how a range is applied to the cache)
// and m68kResetJITCodeCache (full reset on queue overflow).
//
// This file must stay free of build tags and emitter symbols.

package main

import "sort"

// m68kPendingInvalMaxRanges bounds the cross-thread invalidation queue. If a
// host goroutine floods writes faster than the CPU thread drains, the queue
// collapses to a single full-cache reset instead of growing unbounded.
const m68kPendingInvalMaxRanges = 64

// m68kEnqueueJITInvalidation records a guest-write range to invalidate, to be
// applied by the CPU/dispatcher thread. SAFE TO CALL FROM ANY GOROUTINE: it
// only touches the pending-range list under m68kJitPendingInvalMu and never the
// JIT cache maps or code bitmap, which are owned by the CPU thread. This is the
// serialization point that closes the host-vs-CPU data race on the cache.
func (cpu *M68KCPU) m68kEnqueueJITInvalidation(addr, size uint32) {
	if cpu == nil || size == 0 {
		return
	}
	end := uint64(addr) + uint64(size)
	if end > uint64(^uint32(0)) {
		end = uint64(^uint32(0))
	}
	hi := uint32(end)
	cpu.m68kJitPendingInvalMu.Lock()
	if !cpu.m68kJitPendingInvalReset {
		if n := len(cpu.m68kJitPendingInvalRanges); n > 0 {
			// Coalesce with the last range when adjacent/overlapping (handles
			// the common sequential byte-write loaders without list growth).
			last := &cpu.m68kJitPendingInvalRanges[n-1]
			if addr <= last[1] && hi >= last[0] {
				if addr < last[0] {
					last[0] = addr
				}
				if hi > last[1] {
					last[1] = hi
				}
				cpu.m68kJitPendingInvalMu.Unlock()
				cpu.m68kJitHasPendingInval.Store(true)
				// Publish AFTER the range + flag so a dispatcher observing the
				// new generation also observes the queued work.
				cpu.m68kJitInvalGen.Add(1)
				return
			}
		}
		if len(cpu.m68kJitPendingInvalRanges) >= m68kPendingInvalMaxRanges {
			cpu.m68kJitPendingInvalReset = true
			cpu.m68kJitPendingInvalRanges = cpu.m68kJitPendingInvalRanges[:0]
		} else {
			cpu.m68kJitPendingInvalRanges = append(cpu.m68kJitPendingInvalRanges, [2]uint32{addr, hi})
		}
	}
	cpu.m68kJitPendingInvalMu.Unlock()
	cpu.m68kJitHasPendingInval.Store(true)
	// Publish AFTER the range + flag so a dispatcher observing the new
	// generation also observes the queued work.
	cpu.m68kJitInvalGen.Add(1)
}

// m68kDrainPendingJITInvalidations applies queued cross-thread invalidations.
// MUST be called only from the CPU/dispatcher goroutine (it mutates the cache).
func (cpu *M68KCPU) m68kDrainPendingJITInvalidations() {
	if cpu == nil || !cpu.m68kJitHasPendingInval.Load() {
		return
	}
	cpu.m68kJitPendingInvalMu.Lock()
	reset := cpu.m68kJitPendingInvalReset
	ranges := cpu.m68kJitPendingInvalRanges
	cpu.m68kJitPendingInvalRanges = nil
	cpu.m68kJitPendingInvalReset = false
	cpu.m68kJitHasPendingInval.Store(false)
	cpu.m68kJitPendingInvalMu.Unlock()
	if reset {
		cpu.m68kResetJITCodeCache()
		return
	}
	for _, r := range m68kCoalesceInvalRanges(ranges) {
		cpu.invalidateM68KJITForGuestWrite(r[0], r[1]-r[0])
	}
}

// pure performance transform.
func m68kCoalesceInvalRanges(ranges [][2]uint32) [][2]uint32 {
	if len(ranges) < 2 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	out := ranges[:1]
	for _, r := range ranges[1:] {
		last := &out[len(out)-1]
		if r[0] <= last[1] { // overlapping or touching (end is exclusive)
			if r[1] > last[1] {
				last[1] = r[1]
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// m68kWriteOutsideCodeBounds reports whether a guest write of [addr,addr+size)
// lies entirely outside the conservative global envelope [codeLo,codeHi) of all
// compiled JIT code. When true the write cannot intersect any block, so the
// caller skips invalidation in O(1) before any per-page or cache scan.
//
// Safety: the envelope is widened on every m68kMarkJITCodeRanges and reset only
// when the whole metadata is cleared, so it is always a superset of live code
// ranges — an over-wide envelope only costs an occasional unnecessary scan,
// never a missed invalidation. An empty/unknown envelope (codeHi<=codeLo) never
// rejects, deferring to the authoritative slow path.
//
// This relies on M68K populating only regular cache blocks (CodeCache.Put),
// never MMU-mode blocks (PutMMU) — the latter are not threaded through
// m68kMarkJITCodeRanges and would escape the envelope. If M68K ever adopts
func invalidateM68KJITForGuestWrite(bus Bus32, addr uint64, size uint64) {
	if bus == nil || size == 0 || addr > uint64(^uint32(0)) {
		return
	}
	if mb, ok := bus.(*MachineBus); ok && mb.m68kJITInvalidator != nil {
		mb.m68kJITInvalidator(addr, size)
		return
	}
	snap := runtimeStatus.snapshot()
	if snap.m68k == nil || snap.m68k.cpu == nil || snap.m68k.cpu.bus != bus {
		return
	}
	if size > uint64(^uint32(0)) {
		size = uint64(^uint32(0))
	}
	// Fallback (non-MachineBus) path is also reachable from host goroutines;
	// enqueue rather than touch the cache maps off the CPU thread.
	snap.m68k.cpu.m68kEnqueueJITInvalidation(uint32(addr), uint32(size))
}
