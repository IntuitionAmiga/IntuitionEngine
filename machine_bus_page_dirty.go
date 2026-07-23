// machine_bus_page_dirty.go - epoch-based page dirty tracking for guest RAM.
//
// Several consumers want the same thing from the bus: which pages of guest RAM
// have changed since I last looked. The compositor wants it per frame for
// memory-backed video sources, reverse debugging wants it per snapshot, and DMA
// wants it per transfer. What they must not do is share a flag, because a flag
// answers "since somebody last looked", and whichever consumer reads it first
// silently robs the others.
//
// The design here is a global epoch counter plus a per-page generation that
// only ever increases. Nothing is ever cleared, so no consumer can destroy
// another consumer's view, and each keeps its own cursor.
//
// Writer protocol. A writer reads the global epoch, and if the page generation
// already equals it there is nothing to do: that is the steady state, two
// atomic loads and a compare. Otherwise it raises the generation to the epoch
// with a compare-and-swap loop that only ever increases it, so a slow writer
// carrying a stale epoch can never pull a page backwards. It then re-reads the
// global epoch, and if the epoch moved while it was publishing it goes round
// again. That retry is what closes the race the whole design exists to avoid: a
// writer that read epoch E, was pre-empted while a consumer closed E, and then
// published E into a page the consumer had already scanned. The retry republishes
// into the open epoch instead, where the next scan will find it.
//
// Consumer protocol. A consumer first closes the current epoch by advancing the
// counter from E to E+1, then scans for pages whose generation falls in
// (lastSeen, E], and finally moves its own cursor to E. Writes landing during
// the scan publish E+1 and surface next time round rather than being lost or
// double-counted.
//
// Scan cost. A linear walk over every page would be unusable on a machine with
// tens of gigabytes of guest RAM, so pages are grouped into chunks carrying the
// maximum generation of the pages beneath them, raised by the same monotonic
// protocol. A chunk whose summary is at or below the cursor cannot contain a
// page the consumer has not seen, so the scan skips it whole.
//
// Tracking is opt-in with IE_PAGE_DIRTY=1. With it off no tracker is allocated,
// the write path pays one nil pointer test, and consumers get a nil cursor,
// which they are required to read as "assume everything is dirty".
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"os"
	"sync/atomic"
)

// pageDirtyShift is the tracking granularity: 4 KiB pages. It is deliberately
// coarser than the 256-byte bus I/O page, because the consumers are frame and
// snapshot scale and a finer grid buys them nothing but scan time.
const pageDirtyShift = 12

// pageDirtyChunkShift is how many pages one chunk summary covers, as a power of
// two. 512 pages is 2 MiB of guest RAM per summary word.
const pageDirtyChunkShift = 9

// pageDirtyRequested reports whether tracking is switched on.
func pageDirtyRequested() bool {
	return os.Getenv("IE_PAGE_DIRTY") == "1"
}

// Producer stages reported to pageDirtyTestHook. The stale-epoch retry is not
// reachable from outside without being able to stop a writer at the exact point
// between reading the epoch and publishing it.
const (
	pageDirtyStageEpochRead = iota + 1
	pageDirtyStagePublished
)

// pageDirtyTestHook is nil in production.
var pageDirtyTestHook func(stage int)

type pageDirtyTracker struct {
	// epoch is the open epoch. It starts at 1 so that a generation of 0 means
	// "never written", which is distinguishable from "written in the first
	// epoch".
	epoch atomic.Uint64

	gen    []atomic.Uint64
	chunks []atomic.Uint64

	// pages is the tracked page count; addresses beyond it are ignored rather
	// than growing the tables, because guest RAM does not grow under a running
	// machine.
	pages uint64

	published atomic.Uint64
	scans     atomic.Uint64

	// dropped counts publications for addresses past the tracked span. A
	// tracker sized smaller than the RAM it covers would otherwise lose those
	// writes in silence, which is exactly the failure the delta snapshot test
	// exists to catch, so it is counted rather than ignored.
	dropped atomic.Uint64
}

func newPageDirtyTracker(sizeBytes uint64) *pageDirtyTracker {
	pages := (sizeBytes + (1 << pageDirtyShift) - 1) >> pageDirtyShift
	if pages == 0 {
		pages = 1
	}
	chunks := (pages + (1 << pageDirtyChunkShift) - 1) >> pageDirtyChunkShift
	t := &pageDirtyTracker{
		gen:    make([]atomic.Uint64, pages),
		chunks: make([]atomic.Uint64, chunks),
		pages:  pages,
	}
	t.epoch.Store(1)
	return t
}

// raise sets slot to value unless it already holds something at least as new.
// It is the monotonic publication step, and it is the only way either table is
// written.
func raiseGeneration(slot *atomic.Uint64, value uint64) {
	current := slot.Load()
	for current < value {
		if slot.CompareAndSwap(current, value) {
			return
		}
		current = slot.Load()
	}
}

// publishPage marks one page dirty in the open epoch.
func (t *pageDirtyTracker) publishPage(page uint64) {
	if page >= t.pages {
		t.dropped.Add(1)
		return
	}
	slot := &t.gen[page]
	for {
		epoch := t.epoch.Load()
		if slot.Load() >= epoch {
			// Already published at or above the open epoch, so a consumer
			// closing any epoch from here on will still see it. This is the
			// steady state and is kept to two loads and a compare: no counter,
			// no test hook, nothing else on the way out.
			return
		}
		if hook := pageDirtyTestHook; hook != nil {
			hook(pageDirtyStageEpochRead)
		}
		// The page and its chunk summary cannot be raised atomically together,
		// so a consumer can always interleave between them and skip a chunk
		// whose page has already been raised, or scan a page whose chunk has.
		// Neither order fixes that on its own. What fixes it is the re-read
		// below: any consumer able to observe a half-published pair must have
		// closed an epoch to get there, and the writer then republishes into
		// the epoch that close opened.
		raiseGeneration(slot, epoch)
		raiseGeneration(&t.chunks[page>>pageDirtyChunkShift], epoch)
		t.published.Add(1)
		if hook := pageDirtyTestHook; hook != nil {
			hook(pageDirtyStagePublished)
		}
		if t.epoch.Load() == epoch {
			return
		}
		// The epoch rolled while this write was publishing, so the consumer
		// that rolled it may already have scanned past this page. Republish
		// into the epoch that is open now.
	}
}

// publishRange marks every page covering [addr, addr+length) in one pass, which
// is what bulk transfers use instead of touching per-page state per byte.
func (t *pageDirtyTracker) publishRange(addr, length uint64) {
	if length == 0 {
		return
	}
	end := addr + length
	if end < addr {
		return
	}
	first := addr >> pageDirtyShift
	last := (end - 1) >> pageDirtyShift
	if first == last {
		// Every ordinary guest write lands here: one page, no loop.
		t.publishPage(first)
		return
	}
	for page := first; page <= last; page++ {
		t.publishPage(page)
	}
}

// closeEpoch advances the producer epoch and returns the epoch just closed.
// Everything published up to and including the returned epoch is now a stable
// set; anything published from here lands in the epoch after it.
func (t *pageDirtyTracker) closeEpoch() uint64 {
	return t.epoch.Add(1) - 1
}

// scan reports every page whose generation lies in (lastSeen, closed].
func (t *pageDirtyTracker) scan(lastSeen, closed uint64, fn func(page uint64)) {
	t.scans.Add(1)
	for chunk := range t.chunks {
		if t.chunks[chunk].Load() <= lastSeen {
			// No page under this chunk has been touched since the cursor, so
			// the whole 2 MiB can be skipped without reading it.
			continue
		}
		first := uint64(chunk) << pageDirtyChunkShift
		last := first + (1 << pageDirtyChunkShift)
		if last > t.pages {
			last = t.pages
		}
		for page := first; page < last; page++ {
			generation := t.gen[page].Load()
			if generation > lastSeen && generation <= closed {
				fn(page)
			}
		}
	}
}

// pageDirtyCursor is one consumer's view. Each consumer owns its own, and no
// consumer's use of it affects any other's.
type pageDirtyCursor struct {
	tracker  *pageDirtyTracker
	lastSeen uint64
}

// Active reports whether the cursor tracks anything. A nil or inactive cursor
// means the consumer must assume every page is dirty.
func (c *pageDirtyCursor) Active() bool {
	return c != nil && c.tracker != nil
}

// Collect closes the current epoch and reports every page dirtied since this
// cursor last collected, then advances the cursor. Pages dirtied during the
// scan itself are not reported now and are not lost: they belong to the epoch
// this call opened.
func (c *pageDirtyCursor) Collect(fn func(page uint64)) {
	if !c.Active() {
		return
	}
	closed := c.tracker.closeEpoch()
	c.tracker.scan(c.lastSeen, closed, fn)
	c.lastSeen = closed
}

// CollectPages is the slice-returning form, for consumers that want the set
// rather than a callback.
func (c *pageDirtyCursor) CollectPages() []uint64 {
	var pages []uint64
	c.Collect(func(page uint64) { pages = append(pages, page) })
	return pages
}

// PageAddress converts a tracked page index back to a guest address.
func PageDirtyAddress(page uint64) uint64 {
	return page << pageDirtyShift
}

// PageDirtySize is the tracked page size in bytes.
const PageDirtySize = uint64(1) << pageDirtyShift

// EnablePageDirtyTracking allocates the tracker for the given span of guest
// RAM. It is idempotent, and calling it again with a larger span replaces the
// tracker, which resets every cursor's meaning, so it must only be called while
// the machine is quiesced.
func (bus *MachineBus) EnablePageDirtyTracking(sizeBytes uint64) {
	if bus == nil || sizeBytes == 0 {
		return
	}
	bus.pageDirty.Store(newPageDirtyTracker(sizeBytes))
}

// DisablePageDirtyTracking drops the tracker. Existing cursors become inactive,
// which their consumers must read as "everything is dirty".
func (bus *MachineBus) DisablePageDirtyTracking() {
	if bus == nil {
		return
	}
	bus.pageDirty.Store(nil)
}

// PageDirtyTrackingActive reports whether a tracker is bound.
func (bus *MachineBus) PageDirtyTrackingActive() bool {
	return bus != nil && bus.pageDirty.Load() != nil
}

// NewPageDirtyCursor returns a cursor starting from the current epoch, so a
// consumer created mid-run sees changes from that point rather than a backlog
// of everything the machine has ever written. The cursor is inactive when
// tracking is off.
func (bus *MachineBus) NewPageDirtyCursor() *pageDirtyCursor {
	if bus == nil {
		return &pageDirtyCursor{}
	}
	tracker := bus.pageDirty.Load()
	if tracker == nil {
		return &pageDirtyCursor{}
	}
	return &pageDirtyCursor{tracker: tracker, lastSeen: tracker.epoch.Load() - 1}
}

// markPagesDirty publishes a RAM write. It is called from the one place every
// guest RAM write already passes through, so a new write path cannot forget it
// without also forgetting JIT invalidation.
func (bus *MachineBus) markPagesDirty(addr, length uint64) {
	tracker := bus.pageDirty.Load()
	if tracker == nil {
		return
	}
	tracker.publishRange(addr, length)
}
