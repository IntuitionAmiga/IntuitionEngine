// audio_event_ring.go - lock-free register-write queue for the audio chip.
//
// A guest register write and the audio renderer both want chip.mu. The
// renderer holds it while it mixes a whole segment, so a CPU write lands on a
// contended mutex for as long as that takes. The ring lets the guest path
// publish the write and return, and the renderer applies it at the point the
// mutex path would have: after the pending samples have been mixed with the
// pre-write state, which is exactly where flushPendingAudioBlock put the
// boundary before.
//
// Only the guest bus path publishes. Engine writes, setters, reads, reset and
// snapshot all keep the synchronous path, because they either run on the
// renderer itself or need the chip state they just wrote to be visible
// immediately. Those are the "barrier" writers: each one closes admission,
// waits for every producer that has already entered to finish, drains
// everything committed, and only then applies its own mutation, so a queued
// event can never be applied after a write that was issued later.
//
// Deviation from the tranche plan, deliberate: the plan had barrier writers
// request a drain from the renderer and wait for an acknowledgement, so the
// renderer stayed the sole consumer. That deadlocks whenever nothing is
// pulling audio, which is a normal state (no backend attached, a paused
// machine, a headless test). Barrier writers therefore drain themselves, and
// the consumer role is serialised by chip.mu instead: both the renderer and a
// barrier writer hold chip.mu across a drain, so the tail cursor still has a
// single writer at a time.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

// cacheLineBytes is the padding stride used to put hot atomics on separate
// cache lines. It is set to the widest line among supported hosts, not the
// commonest: amd64 and the arm64 parts this runs on (Cortex-A72/A76 on the
// Raspberry Pi, Cortex-X1/A78 on the Lenovo x13s) use 64-byte lines, but Apple
// Silicon uses 128, and a 64-byte stride would leave two atomics sharing one
// line there, which is the whole failure this padding exists to avoid. Padding
// to 128 separates them on every supported host and costs a few dozen bytes per
// ring on the 64-byte parts, which is the correct trade. Under-padding is a
// correctness-of-optimisation bug; over-padding is not.
const cacheLineBytes = 128

// audioEventRingCapacity is the queue depth. A full ring is not an error: the
// producer falls back to the barrier path, which is what it would have done
// with no ring at all.
const audioEventRingCapacity = 4096

// audioEventRingRequested reports whether the ring is switched on. It is
// enabled by default after the bit-identical parity gate; IE_AUDIO_EVENT_RING=0
// disables it and restores the synchronous barrier path.
func audioEventRingRequested() bool {
	return os.Getenv("IE_AUDIO_EVENT_RING") != "0"
}

// audioEvent is one queued register write.
type audioEvent struct {
	addr  uint32
	value uint32
}

// audioEventSlot holds an event and the sequence stamp that publishes it.
// committed holds seq+1 once the payload is fully written, so a consumer can
// tell a published slot from one a producer has reserved but not yet filled.
type audioEventSlot struct {
	ev        audioEvent
	committed atomic.Uint64
}

type audioEventRing struct {
	slots [audioEventRingCapacity]audioEventSlot

	// head and tail sit on separate cache lines on purpose. Producers CAS head
	// on every publish while the consumer stores tail on every drained event, so
	// left adjacent the two writes ping the same line between the guest bus
	// goroutine and the renderer. BenchmarkSharedCounters_Parallel measures that
	// as roughly a 4.5x penalty on this host (about 18 ns against 4 ns per bump),
	// and the pad removes it. Producers also load tail to test for fullness, but
	// a load does not invalidate the line the way the two stores do.
	head atomic.Uint64
	_    [cacheLineBytes - 8]byte
	// tail is the next sequence to consume, advanced only while chip.mu is held,
	// and read by producers to test for fullness.
	tail atomic.Uint64
	_    [cacheLineBytes - 8]byte

	// inFlight counts producers between admission and publication. A barrier
	// writer waits for it to reach zero, which is what makes "everything
	// committed before me" a stable set. It shares its line with the gate flags,
	// which are all touched by the same producer and barrier paths.
	inFlight   atomic.Int64
	gateClosed atomic.Bool
	gateSealed atomic.Bool
	_          [cacheLineBytes - 8 - 1 - 1]byte

	// barrierMu serialises the whole barrier protocol, so one writer's reopen
	// can never let events cross another writer's still-pending barrier.
	barrierMu sync.Mutex

	// published is bumped by producers, applied by the consumer; the same
	// store-against-store adjacency as head and tail, so they take separate
	// lines too. overflows and barriers are rare and ride with applied.
	published atomic.Uint64
	_         [cacheLineBytes - 8]byte
	applied   atomic.Uint64
	overflows atomic.Uint64
	barriers  atomic.Uint64
}

func newAudioEventRing() *audioEventRing {
	return &audioEventRing{}
}

// publish queues an event and returns true. It returns false when the caller
// must take the barrier path instead: admission is closed, or the ring is full.
func (r *audioEventRing) publish(ev audioEvent) bool {
	r.inFlight.Add(1)
	if hook := audioEventTestHook; hook != nil {
		hook(audioEventStageAdmitted)
	}
	if r.gateClosed.Load() {
		// A barrier writer closed admission after this producer entered. Back
		// out so the writer's wait can complete, and take the barrier path.
		r.inFlight.Add(-1)
		return false
	}

	var seq uint64
	for {
		head := r.head.Load()
		if head-r.tail.Load() >= audioEventRingCapacity {
			r.inFlight.Add(-1)
			r.overflows.Add(1)
			return false
		}
		if r.head.CompareAndSwap(head, head+1) {
			seq = head
			break
		}
	}

	// The slot cannot be in use: the consumer copies an event out before it
	// advances the tail, and the tail is what lets this sequence be reserved.
	if hook := audioEventTestHook; hook != nil {
		hook(audioEventStageReserved)
	}
	slot := &r.slots[seq%audioEventRingCapacity]
	slot.ev = ev
	slot.committed.Store(seq + 1)
	if hook := audioEventTestHook; hook != nil {
		hook(audioEventStagePublished)
	}
	r.inFlight.Add(-1)
	r.published.Add(1)
	return true
}

// closeAdmission shuts the gate and waits for every producer that entered
// before it closed. On return the set of committed sequences is stable.
func (r *audioEventRing) closeAdmission() {
	r.gateClosed.Store(true)
	for r.inFlight.Load() != 0 {
		runtime.Gosched()
	}
}

// reopenAdmission lets producers back in, unless the ring has been sealed by
// shutdown, reset or a mid-run disable.
func (r *audioEventRing) reopenAdmission() {
	if !r.gateSealed.Load() {
		r.gateClosed.Store(false)
	}
}

// audioEventRingActive reports whether this chip has a ring at all.
func (chip *SoundChip) audioEventRingActive() bool {
	return chip.eventRing != nil
}

// drainAudioEventsLocked applies every committed event in sequence order.
// chip.mu must be held, which is what serialises the consumer role between the
// renderer and barrier writers.
func (chip *SoundChip) drainAudioEventsLocked() {
	r := chip.eventRing
	if r == nil {
		return
	}
	for {
		tail := r.tail.Load()
		if tail >= r.head.Load() {
			return
		}
		slot := &r.slots[tail%audioEventRingCapacity]
		if slot.committed.Load() != tail+1 {
			// A producer has reserved this sequence but not yet published it.
			// Stopping here preserves order; the event is drained next time.
			return
		}
		ev := slot.ev
		// Advance before applying: the write must not be replayed if applying
		// it re-enters the drain.
		r.tail.Store(tail + 1)
		r.applied.Add(1)
		chip.handleRegisterWriteLocked(ev.addr, ev.value)
	}
}

// drainAudioEventsBeforeRender applies queued writes at the start of a render
// pull, so a write issued between pulls is visible to the first sample rather
// than to the first sample after a flush.
func (chip *SoundChip) drainAudioEventsBeforeRender() {
	if chip.eventRing == nil {
		return
	}
	chip.mu.Lock()
	chip.drainAudioEventsLocked()
	chip.mu.Unlock()
}

// audioEventTestHook is nil in production. Tests set it to pause a producer at
// a chosen point in the publication sequence, which is the only way to drive
// the gate-close race deterministically.
var audioEventTestHook func(stage int)

// Producer stages reported to audioEventTestHook.
const (
	audioEventStageAdmitted = iota + 1
	audioEventStageReserved
	audioEventStagePublished
)

// audioEventBarrier runs a mutation that must be ordered after every register
// write already committed to the ring. It closes admission, waits for the
// in-flight producers, flushes the pending audio block so those samples are
// mixed with the pre-write state, drains the ring, and only then applies the
// mutation with chip.mu held.
func (chip *SoundChip) audioEventBarrier(apply func()) {
	chip.beginAudioEventBarrier()
	chip.mu.Lock()
	apply()
	chip.mu.Unlock()
	chip.endAudioEventBarrier()
}

// beginAudioEventBarrier opens the barrier: admission is shut, every producer
// that had already entered has finished, the pending render segment is flushed
// and the ring is drained. On return the caller may take chip.mu and read or
// replace chip state with no queued write able to appear behind it.
//
// Callers must release chip.mu before calling endAudioEventBarrier. The lock
// order is barrierMu then chip.mu, and taking barrierMu while holding chip.mu
// would invert it against every other barrier writer.
func (chip *SoundChip) beginAudioEventBarrier() {
	r := chip.eventRing
	if r == nil {
		chip.flushPendingAudioBlock()
		return
	}
	r.barrierMu.Lock()
	r.closeAdmission()
	// flushPendingAudioBlock drains the ring as well, under chip.mu, which this
	// goroutine does not hold yet.
	chip.flushPendingAudioBlock()
	r.barriers.Add(1)
}

// endAudioEventBarrier closes the barrier and lets producers back in, unless
// the ring has been sealed.
func (chip *SoundChip) endAudioEventBarrier() {
	r := chip.eventRing
	if r == nil {
		return
	}
	r.reopenAdmission()
	r.barrierMu.Unlock()
}

// sealAudioEventRing drains everything committed and leaves admission shut, for
// shutdown, reset and a mid-run disable. No event is dropped or reordered:
// producers that were already inside back out onto the barrier path.
func (chip *SoundChip) sealAudioEventRing() {
	r := chip.eventRing
	if r == nil {
		return
	}
	r.barrierMu.Lock()
	defer r.barrierMu.Unlock()

	r.gateSealed.Store(true)
	r.closeAdmission()
	chip.flushPendingAudioBlock()
	chip.mu.Lock()
	chip.drainAudioEventsLocked()
	chip.mu.Unlock()
}

// unsealAudioEventRing reopens a sealed ring, used when a reset is followed by
// continued playback. It takes the barrier mutex: a producer that backed out
// while the ring was sealed is running the barrier protocol itself, and
// reopening the gate from outside that mutex would let a later producer publish
// into the window between that barrier's admission close and its drain. The
// barrier would then apply the newer event before its own older write, which
// inverts the order the two writes were issued in.
func (chip *SoundChip) unsealAudioEventRing() {
	r := chip.eventRing
	if r == nil {
		return
	}
	r.barrierMu.Lock()
	defer r.barrierMu.Unlock()
	r.gateSealed.Store(false)
	r.gateClosed.Store(false)
}
