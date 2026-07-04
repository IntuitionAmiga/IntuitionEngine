package main

import (
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
}

func (a *PerfSubsysAcct) Reset() {
	a.VideoFramePath.Reset()
	a.AudioPull.Reset()
	a.BusRead32Slow.Reset()
	a.BusWrite32Slow.Reset()
}

var perfSubsysAcct PerfSubsysAcct
