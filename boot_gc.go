// boot_gc.go - forced pre-execution collections at boot and full reset.
//
// These are boot-time sweeps, not a runtime GC policy: they collect the
// transient allocations left by loading a guest image, and they must run
// BEFORE the CPU, compositor, render and audio loops start so the collection
// cannot run concurrently with them. runtime.GC() is used deliberately, never
// debug.FreeOSMemory(), because the latter madvise-returns pages that then
// re-fault during the first frames.

package main

import (
	"runtime"
	"sync/atomic"
)

// bootForcedGCCount counts forced boot collections, for test observability.
var bootForcedGCCount atomic.Uint64

// bootForcedGC runs one forced pre-execution collection.
func bootForcedGC() {
	bootForcedGCCount.Add(1)
	runtime.GC()
}
