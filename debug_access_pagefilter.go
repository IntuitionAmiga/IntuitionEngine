// debug_access_pagefilter.go - conservative per-page pre-filter for debug access checks.
//
// A single watchpoint anywhere makes DebugAccessService.active true, and every
// guest access then takes the full onAccess path: a mutex, a scan of the guard
// list and a scan of the watch list, for an answer that is almost always no. On
// this machine that turned a 6.9 ns non-I/O read into a 47 ns one, measured with
// BenchmarkRead32_NonIO_Variants.
//
// The pre-filter answers the cheap half of that question without the mutex: does
// any guard or watchpoint cover the page this access touches? It is deliberately
// one-sided. A clear bit means no guard and no watchpoint covers the page, so
// the access cannot hit anything and onAccess can return at once. A set bit
// means only "maybe", and the full scan runs exactly as before, so guard scope,
// CPU identity, access kind and the precise address range are all still decided
// by the original code. Nothing here can turn a hit into a miss.
//
// The filter is not an index of watchpoints and must never become one. It is
// rebuilt whole under s.mu whenever the guard set, the watch set or the history
// setting changes, and published as an immutable pointer.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

// debugPageShift is the page granularity of the filter. It matches the bus I/O
// page bitmap so the two describe the same unit of address space, but the
// bitmaps are kept separate: the I/O bitmap stays I/O-only.
const debugPageShift = 8

// debugPageFilterMaxPages bounds the bitmap. A guard covering more than this
// many pages, or reaching above the bitmap's span, sets all instead of
// allocating; broad guards are rare and are not what the filter exists for.
const debugPageFilterMaxPages = 1 << 24 // 4 GiB of 256-byte pages

// debugPageFilter is an immutable snapshot of which pages may carry a guard or
// a watchpoint. all short-circuits to "every page may", which is what history
// recording needs because it observes every access regardless of coverage.
type debugPageFilter struct {
	all  bool
	bits []uint64
}

// mark sets every page covering [start, end]. It reports false when the range
// cannot be represented, which the caller turns into all.
func (f *debugPageFilter) mark(start, end uint64) bool {
	if end < start {
		return true
	}
	firstPage := start >> debugPageShift
	lastPage := end >> debugPageShift
	if lastPage >= debugPageFilterMaxPages {
		return false
	}
	if lastPage-firstPage >= debugPageFilterMaxPages>>4 {
		// A guard this broad would set most of the bitmap; say so directly
		// rather than spending the stores.
		return false
	}
	need := int(lastPage>>6) + 1
	for len(f.bits) < need {
		f.bits = append(f.bits, 0)
	}
	for page := firstPage; page <= lastPage; page++ {
		f.bits[page>>6] |= 1 << (page & 63)
	}
	return true
}

// covers reports whether the page may carry a guard or watchpoint. Pages past
// the end of the bitmap are not covered by construction: the bitmap is sized to
// the highest page any guard or watchpoint reaches.
func (f *debugPageFilter) covers(page uint64) bool {
	if f.all {
		return true
	}
	word := page >> 6
	if word >= uint64(len(f.bits)) {
		return false
	}
	return f.bits[word]&(1<<(page&63)) != 0
}

// rebuildPageFilterLocked recomputes and publishes the filter. s.mu must be
// held for writing. Every mutation of guards, watches or historyEnabled must
// call it, which is why it is invoked from setActiveLocked rather than from
// each mutator: setActiveLocked is already the one point every one of them
// passes through.
func (s *DebugAccessService) rebuildPageFilterLocked() {
	if s.historyEnabled {
		// History records every access, so no access may be filtered out.
		s.pageFilter.Store(&debugPageFilter{all: true})
		return
	}
	filter := &debugPageFilter{}
	for _, guard := range s.guards {
		if !filter.mark(guard.Start, guard.End) {
			s.pageFilter.Store(&debugPageFilter{all: true})
			return
		}
	}
	for _, watch := range s.watches {
		width := watch.Width
		if width <= 0 {
			width = 1
		}
		if !filter.mark(watch.Address, watch.Address+uint64(width-1)) {
			s.pageFilter.Store(&debugPageFilter{all: true})
			return
		}
	}
	s.pageFilter.Store(filter)
}

// mayAffect is the hot-path pre-filter. It is conservative in one direction
// only: a true answer means the full check must run, a false answer means no
// guard and no watchpoint covers any byte of the access and history is off.
func (s *DebugAccessService) mayAffect(addr uint64, width int) bool {
	filter := s.pageFilter.Load()
	if filter == nil || filter.all {
		return true
	}
	if width <= 0 {
		width = 1
	}
	end := addr + uint64(width-1)
	if end < addr {
		// Wrapped: not representable, so do not filter it.
		return true
	}
	firstPage := addr >> debugPageShift
	lastPage := end >> debugPageShift
	if lastPage-firstPage > 1 {
		// Accesses are at most eight bytes wide, so this is a caller passing
		// something unusual. Do not filter it.
		return true
	}
	if filter.covers(firstPage) {
		return true
	}
	return lastPage != firstPage && filter.covers(lastPage)
}
