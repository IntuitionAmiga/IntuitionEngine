// machine_bus_page_dirty_test.go - epoch page dirty tracking.
//
// The tests that matter here are the ones that attack the epoch roll, because
// that is the only place the design can lose a write. Each of them drives the
// roll deterministically through pageDirtyTestHook rather than hoping a race
// reproduces, and the concurrent ones are meant to be run under -race as well.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"bytes"
	"math/rand"
	"sync"
	"testing"
)

const pageDirtyTestSpan = 8 << 20

func newDirtyBus(t *testing.T) *MachineBus {
	t.Helper()
	bus := NewMachineBus()
	// Size the tracker to the RAM it has to cover. A tracker smaller than the
	// bus drops publications past its end, which is exactly the mistake the
	// dropped counter is there to expose.
	bus.EnablePageDirtyTracking(uint64(len(bus.GetMemory())))
	t.Cleanup(func() { pageDirtyTestHook = nil })
	return bus
}

func pageOf(addr uint64) uint64 { return addr >> pageDirtyShift }

// pageSet turns a collected page list into a set, and fails on duplicates: a
// consumer reporting the same page twice in one scan means the scan is walking
// a chunk more than once.
func pageSet(t *testing.T, pages []uint64) map[uint64]bool {
	t.Helper()
	set := make(map[uint64]bool, len(pages))
	for _, page := range pages {
		if set[page] {
			t.Fatalf("page %d reported twice in one scan", page)
		}
		set[page] = true
	}
	return set
}

// TestPageDirty_GenerationMonotonicOncePerEpoch pins the steady state: repeated
// writes to a page already marked in the open epoch do not republish, and the
// generation only moves when the epoch has.
func TestPageDirty_GenerationMonotonicOncePerEpoch(t *testing.T) {
	tracker := newPageDirtyTracker(pageDirtyTestSpan)
	const page = 5

	tracker.publishPage(page)
	first := tracker.gen[page].Load()
	published := tracker.published.Load()
	for range 1000 {
		tracker.publishPage(page)
	}
	if got := tracker.gen[page].Load(); got != first {
		t.Fatalf("generation moved without an epoch roll: %d then %d", first, got)
	}
	if got := tracker.published.Load(); got != published {
		t.Fatalf("repeated writes republished %d times in one epoch", got-published)
	}

	tracker.closeEpoch()
	tracker.publishPage(page)
	if got := tracker.gen[page].Load(); got <= first {
		t.Fatalf("generation did not advance after an epoch roll: %d then %d", first, got)
	}
}

// TestPageDirty_EpochRollWritesBeforeDuringAfter injects writes before, during
// and after a roll and requires that none is lost: each is reported to exactly
// one of the two scans.
func TestPageDirty_EpochRollWritesBeforeDuringAfter(t *testing.T) {
	tracker := newPageDirtyTracker(pageDirtyTestSpan)
	cursor := &pageDirtyCursor{tracker: tracker, lastSeen: tracker.epoch.Load() - 1}

	const before, during, after = 10, 20, 30
	tracker.publishPage(before)

	// A write landing while the consumer is mid-scan. The hook fires inside
	// the scan's own callback, which is the closest a single-goroutine test can
	// get to a concurrent write against a specific page.
	var firstScan []uint64
	closed := tracker.closeEpoch()
	tracker.scan(cursor.lastSeen, closed, func(page uint64) {
		firstScan = append(firstScan, page)
		tracker.publishPage(during)
	})
	cursor.lastSeen = closed

	tracker.publishPage(after)
	secondScan := cursor.CollectPages()

	first := pageSet(t, firstScan)
	second := pageSet(t, secondScan)
	if !first[before] {
		t.Fatal("a write before the roll was not reported by the scan that closed it")
	}
	if !second[during] {
		t.Fatal("a write during the scan was lost")
	}
	if !second[after] {
		t.Fatal("a write after the roll was lost")
	}
	if first[during] || first[after] {
		t.Fatal("a scan reported a write that belonged to the epoch it had just opened")
	}
	if second[before] {
		t.Fatal("a write was reported to two consecutive scans")
	}
}

// TestPageDirty_StaleEpochWriterRetries is the core race. A writer reads epoch
// E and is held there while a consumer closes E and scans past its page. The
// writer must not publish E into an already scanned page and disappear; the
// re-read and retry must move it into the open epoch, where the next scan finds
// it.
func TestPageDirty_StaleEpochWriterRetries(t *testing.T) {
	tracker := newPageDirtyTracker(pageDirtyTestSpan)
	cursor := &pageDirtyCursor{tracker: tracker, lastSeen: tracker.epoch.Load() - 1}
	const page = 42

	released := make(chan struct{})
	rolled := make(chan struct{})
	var once sync.Once
	pageDirtyTestHook = func(stage int) {
		if stage != pageDirtyStageEpochRead {
			return
		}
		once.Do(func() {
			// The writer has read the epoch and has not published yet. Let the
			// consumer close that epoch and scan underneath it.
			close(released)
			<-rolled
		})
	}
	t.Cleanup(func() { pageDirtyTestHook = nil })

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tracker.publishPage(page)
	}()

	<-released
	firstScan := pageSet(t, cursor.CollectPages())
	close(rolled)
	wg.Wait()

	secondScan := pageSet(t, cursor.CollectPages())
	if !firstScan[page] && !secondScan[page] {
		t.Fatal("a write published across an epoch roll was reported by neither scan")
	}
	if firstScan[page] && secondScan[page] {
		t.Fatal("a single write was reported by two scans")
	}
}

// TestPageDirty_DoubleRollMonotonicGenerations runs many writers against
// repeated rolls and requires that no page generation ever decreases and that
// every writer ends at or above the epoch it last observed.
func TestPageDirty_DoubleRollMonotonicGenerations(t *testing.T) {
	tracker := newPageDirtyTracker(pageDirtyTestSpan)
	const writers = 8
	const pages = 64

	var rolls sync.WaitGroup
	stop := make(chan struct{})
	rolls.Add(1)
	go func() {
		defer rolls.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Two consecutive rolls, which is what forces a retrying writer to
			// go round more than once.
			tracker.closeEpoch()
			tracker.closeEpoch()
		}
	}()

	observed := make([][]uint64, writers)
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			seen := make([]uint64, pages)
			for range 500 {
				for page := range uint64(pages) {
					epoch := tracker.epoch.Load()
					tracker.publishPage(page)
					if epoch > seen[page] {
						seen[page] = epoch
					}
					if got := tracker.gen[page].Load(); got < seen[page] {
						t.Errorf("page %d generation %d is below an epoch %d already published into it",
							page, got, seen[page])
						return
					}
				}
			}
			observed[w] = seen
		}(w)
	}
	wg.Wait()
	close(stop)
	rolls.Wait()

	for w := range writers {
		for page, epoch := range observed[w] {
			if got := tracker.gen[page].Load(); got < epoch {
				t.Fatalf("writer %d observed epoch %d for page %d, but the generation ended at %d",
					w, epoch, page, got)
			}
		}
	}
}

// TestPageDirty_TwoConsumersIndependentViews is the reason nothing is ever
// cleared. Two consumers scan concurrently while writers are active, and each
// must see every page dirtied since its own last scan regardless of what the
// other did.
func TestPageDirty_TwoConsumersIndependentViews(t *testing.T) {
	tracker := newPageDirtyTracker(pageDirtyTestSpan)
	fast := &pageDirtyCursor{tracker: tracker, lastSeen: tracker.epoch.Load() - 1}
	slow := &pageDirtyCursor{tracker: tracker, lastSeen: tracker.epoch.Load() - 1}

	const pages = 32
	for page := range uint64(pages) {
		tracker.publishPage(page)
	}

	// The fast consumer drains first. If it cleared shared state, the slow one
	// would see nothing.
	fastSeen := pageSet(t, fast.CollectPages())
	slowSeen := pageSet(t, slow.CollectPages())
	for page := range uint64(pages) {
		if !fastSeen[page] {
			t.Fatalf("the first consumer missed page %d", page)
		}
		if !slowSeen[page] {
			t.Fatalf("the second consumer lost page %d to the first consumer's scan", page)
		}
	}

	// Now with writers running throughout, which is the race detector's case.
	stop := make(chan struct{})
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			tracker.publishPage(uint64(i % pages))
		}
	}()

	var consumers sync.WaitGroup
	for _, cursor := range []*pageDirtyCursor{fast, slow} {
		consumers.Add(1)
		go func(c *pageDirtyCursor) {
			defer consumers.Done()
			for range 200 {
				c.Collect(func(uint64) {})
			}
		}(cursor)
	}
	consumers.Wait()
	close(stop)
	writers.Wait()
}

// TestPageDirty_SnapshotDeltaMatchesFullDiff is the silent-corruption guard: a
// snapshot rebuilt from reported dirty pages alone must equal one taken by
// diffing the whole of memory. It runs writes through the real bus paths, so a
// write path that forgets to publish fails here rather than in production.
func TestPageDirty_SnapshotDeltaMatchesFullDiff(t *testing.T) {
	bus := newDirtyBus(t)
	cursor := bus.NewPageDirtyCursor()
	if !cursor.Active() {
		t.Fatal("tracking was enabled but the cursor is inactive")
	}

	memory := bus.GetMemory()
	reference := append([]byte(nil), memory...)
	delta := append([]byte(nil), memory...)

	rng := rand.New(rand.NewSource(31337))
	for round := range 40 {
		for range 200 {
			addr := uint32(rng.Intn(len(memory) - 8))
			switch rng.Intn(4) {
			case 0:
				bus.Write8(addr, byte(rng.Intn(256)))
			case 1:
				bus.Write16(addr, uint16(rng.Intn(1<<16)))
			case 2:
				bus.Write32(addr, rng.Uint32())
			case 3:
				payload := make([]byte, 1+rng.Intn(200))
				for i := range payload {
					payload[i] = byte(rng.Intn(256))
				}
				bus.WriteSpan(uint64(addr), payload)
			}
		}

		// Rebuild the delta copy from the reported pages only.
		cursor.Collect(func(page uint64) {
			start := PageDirtyAddress(page)
			end := start + PageDirtySize
			if end > uint64(len(memory)) {
				end = uint64(len(memory))
			}
			copy(delta[start:end], memory[start:end])
		})

		copy(reference, memory)
		if dropped := bus.pageDirty.Load().dropped.Load(); dropped != 0 {
			t.Fatalf("round %d: %d publications fell outside the tracked span", round, dropped)
		}
		if !bytes.Equal(delta, reference) {
			for page := range uint64(len(memory)) / PageDirtySize {
				start := PageDirtyAddress(page)
				end := start + PageDirtySize
				if !bytes.Equal(delta[start:end], reference[start:end]) {
					t.Fatalf("round %d: page %d changed but was never reported dirty", round, page)
				}
			}
			t.Fatalf("round %d: delta snapshot differs from the full diff", round)
		}
	}
}

// TestPageDirty_DMARangePublication pins that a bulk transfer publishes every
// page it covers, in one pass, rather than leaving the tail of a span unmarked.
func TestPageDirty_DMARangePublication(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	bus := newDirtyBus(t)
	cursor := bus.NewPageDirtyCursor()

	base := uint64(3) << pageDirtyShift
	payload := make([]byte, 5*PageDirtySize+17)
	for i := range payload {
		payload[i] = byte(i)
	}
	bus.WriteSpan(base, payload)

	reported := pageSet(t, cursor.CollectPages())
	firstPage := pageOf(base)
	lastPage := pageOf(base + uint64(len(payload)) - 1)
	for page := firstPage; page <= lastPage; page++ {
		if !reported[page] {
			t.Fatalf("bulk write did not publish page %d of the range %d-%d", page, firstPage, lastPage)
		}
	}
	if reported[lastPage+1] {
		t.Fatal("bulk write published a page past the end of the range")
	}
	if firstPage > 0 && reported[firstPage-1] {
		t.Fatal("bulk write published a page before the start of the range")
	}
}

// TestPageDirty_VideoSourceConsumesEpochs models the compositor's use: a
// consumer scanning once per frame must see exactly the pages of its source
// written during that frame, and must not be affected by a snapshot consumer
// scanning between its frames.
func TestPageDirty_VideoSourceConsumesEpochs(t *testing.T) {
	bus := newDirtyBus(t)
	video := bus.NewPageDirtyCursor()
	snapshots := bus.NewPageDirtyCursor()

	sourceBase := uint64(64) << pageDirtyShift

	for frame := range 5 {
		touched := uint64(frame % 3)
		bus.Write32(uint32(sourceBase+touched*PageDirtySize), uint32(frame))

		// A snapshot consumer runs between frames and must not consume the
		// video source's view.
		snapshots.Collect(func(uint64) {})

		seen := pageSet(t, video.CollectPages())
		want := pageOf(sourceBase + touched*PageDirtySize)
		if !seen[want] {
			t.Fatalf("frame %d: the video cursor missed its own source page %d", frame, want)
		}
		if len(seen) != 1 {
			t.Fatalf("frame %d: the video cursor reported %d pages, expected exactly the one written", frame, len(seen))
		}
	}
}

// TestPageDirty_DisabledCursorsAreInactive pins the fallback contract: with
// tracking off, consumers get an inactive cursor and must treat everything as
// dirty rather than receiving an empty set that looks like "nothing changed".
func TestPageDirty_DisabledCursorsAreInactive(t *testing.T) {
	bus := NewMachineBus()
	// Start from a known state rather than from the ambient IE_PAGE_DIRTY: the
	// contract under test is what an untracked bus hands a consumer, not what
	// the environment happened to ask for.
	bus.DisablePageDirtyTracking()
	cursor := bus.NewPageDirtyCursor()
	if cursor.Active() {
		t.Fatal("a cursor from an untracked bus reports itself active")
	}
	bus.Write32(0x1000, 0xDEADBEEF)
	if pages := cursor.CollectPages(); pages != nil {
		t.Fatalf("an inactive cursor reported %d pages", len(pages))
	}

	bus.EnablePageDirtyTracking(pageDirtyTestSpan)
	if !bus.PageDirtyTrackingActive() {
		t.Fatal("enabling tracking did not bind a tracker")
	}
	bus.DisablePageDirtyTracking()
	if bus.PageDirtyTrackingActive() {
		t.Fatal("disabling tracking left a tracker bound")
	}
}
