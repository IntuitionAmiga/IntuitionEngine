package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

// SubsysAcct tracks opt-in time and operation counts for non-CPU hot paths.
// It follows PerfAcct's gate: when IE_PERF_ACCT is off, callers return before
// any atomic operation is performed.
type SubsysAcct struct {
	ns  atomic.Int64
	ops atomic.Uint64
}

type SubsysAcctSnapshot struct {
	Ns  int64
	Ops uint64
}

func (a *SubsysAcct) Reset() {
	a.ns.Store(0)
	a.ops.Store(0)
}

func (a *SubsysAcct) Snapshot() SubsysAcctSnapshot {
	return SubsysAcctSnapshot{
		Ns:  a.ns.Load(),
		Ops: a.ops.Load(),
	}
}

func (a *SubsysAcct) Add(ns int64, ops uint64) {
	if !perfAcctOn {
		return
	}
	a.ns.Add(ns)
	a.ops.Add(ops)
}

func (a *SubsysAcct) AddSince(start time.Time, ops uint64) {
	if !perfAcctOn {
		return
	}
	a.ns.Add(time.Since(start).Nanoseconds())
	a.ops.Add(ops)
}

type PerfSubsysAcct struct {
	VideoFramePath SubsysAcct
	AudioPull      SubsysAcct
	BusRead32Slow  SubsysAcct
	BusWrite32Slow SubsysAcct

	// Voodoo swap-worker stage split, one Add per present job. Separates
	// the backend render submit, the present/fence wait, the framebuffer
	// readback, and the triple-buffer publish copy so a slow present
	// path can be attributed to a stage instead of guessed at.
	VoodooFlush    SubsysAcct
	VoodooSwapWait SubsysAcct
	VoodooReadback SubsysAcct
	VoodooPublish  SubsysAcct
}

func (a *PerfSubsysAcct) Reset() {
	a.VideoFramePath.Reset()
	a.AudioPull.Reset()
	a.BusRead32Slow.Reset()
	a.BusWrite32Slow.Reset()
	a.VoodooFlush.Reset()
	a.VoodooSwapWait.Reset()
	a.VoodooReadback.Reset()
	a.VoodooPublish.Reset()
}

// Report renders every subsystem bucket as "name: total-ms ops avg-us"
// lines. Buckets with zero ops are omitted; an empty report means no
// instrumented path ran (or IE_PERF_ACCT was off).
func (a *PerfSubsysAcct) Report() string {
	type row struct {
		name string
		snap SubsysAcctSnapshot
	}
	rows := []row{
		{"video_frame_path", a.VideoFramePath.Snapshot()},
		{"audio_pull", a.AudioPull.Snapshot()},
		{"bus_read32_slow", a.BusRead32Slow.Snapshot()},
		{"bus_write32_slow", a.BusWrite32Slow.Snapshot()},
		{"voodoo_flush", a.VoodooFlush.Snapshot()},
		{"voodoo_swap_wait", a.VoodooSwapWait.Snapshot()},
		{"voodoo_readback", a.VoodooReadback.Snapshot()},
		{"voodoo_publish", a.VoodooPublish.Snapshot()},
	}
	out := ""
	for _, r := range rows {
		if r.snap.Ops == 0 {
			continue
		}
		avgUs := float64(r.snap.Ns) / float64(r.snap.Ops) / 1e3
		out += fmt.Sprintf("%s: %.1fms %dops %.0fus/op\n",
			r.name, float64(r.snap.Ns)/1e6, r.snap.Ops, avgUs)
	}
	return out
}

var perfSubsysAcct PerfSubsysAcct
