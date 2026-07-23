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

// audioEventRingCapacity is the queue depth. A full ring is not an error: the
// producer falls back to the barrier path, which is what it would have done
// with no ring at all.
const audioEventRingCapacity = 4096

// audioEventRingRequested reports whether the ring is switched on. It is
// opt-in: IE_AUDIO_EVENT_RING=1 enables it.
func audioEventRingRequested() bool {
	return os.Getenv("IE_AUDIO_EVENT_RING") == "1"
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

	// head is the next sequence to reserve, claimed by producers. tail is the
	// next sequence to consume, advanced only while chip.mu is held, and read
	// by producers to test for fullness.
	head atomic.Uint64
	tail atomic.Uint64

	// inFlight counts producers between admission and publication. A barrier
	// writer waits for it to reach zero, which is what makes "everything
	// committed before me" a stable set.
	inFlight   atomic.Int64
	gateClosed atomic.Bool
	gateSealed atomic.Bool

	// barrierMu serialises the whole barrier protocol, so one writer's reopen
	// can never let events cross another writer's still-pending barrier.
	barrierMu sync.Mutex

	published atomic.Uint64
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
	r := chip.eventRing
	if r == nil {
		chip.flushPendingAudioBlock()
		chip.mu.Lock()
		apply()
		chip.mu.Unlock()
		return
	}

	r.barrierMu.Lock()
	defer r.barrierMu.Unlock()

	r.closeAdmission()
	chip.flushPendingAudioBlock()
	chip.mu.Lock()
	chip.drainAudioEventsLocked()
	apply()
	chip.mu.Unlock()
	r.barriers.Add(1)
	r.reopenAdmission()
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
// continued playback.
func (chip *SoundChip) unsealAudioEventRing() {
	r := chip.eventRing
	if r == nil {
		return
	}
	r.gateSealed.Store(false)
	r.gateClosed.Store(false)
}
