package main

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

type AccessKind uint8

const (
	AccessRead AccessKind = iota
	AccessWrite
	AccessExecute
)

type AccessPerm uint8

const (
	PermRead AccessPerm = 1 << iota
	PermWrite
	PermExecute
)

type GuardScope struct {
	AllCPUs bool
	CPUID   int
}

type AccessGuard struct {
	Start uint64
	End   uint64
	Perm  AccessPerm
	Scope GuardScope
	Once  bool
	Name  string
}

type AccessEvent struct {
	Seq           uint64
	CPUID         int
	PC            uint64
	Address       uint64
	Width         int
	Kind          AccessKind
	OldValue      uint64
	NewValue      uint64
	OldValueKnown bool
}

type DebugAccessService struct {
	mu             sync.RWMutex
	channels       map[int]chan<- BreakpointEvent
	pcReaders      map[int]func() uint64
	stoppers       map[int]func()
	guards         []AccessGuard
	watches        []AccessWatchpoint
	history        []AccessEvent
	historyStart   int
	historyLen     int
	historySeq     uint64
	seqSource      func() uint64
	historyEnabled bool
	instrumented   atomic.Bool
	active         atomic.Bool
}

type AccessWatchpoint struct {
	CPUID   int
	Address uint64
	Width   int
	Type    WatchpointType
}

func NewDebugAccessService() *DebugAccessService {
	return &DebugAccessService{
		channels:  make(map[int]chan<- BreakpointEvent),
		pcReaders: make(map[int]func() uint64),
		stoppers:  make(map[int]func()),
	}
}

func (s *DebugAccessService) RegisterCPU(cpuID int, ch chan<- BreakpointEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch == nil {
		delete(s.channels, cpuID)
		delete(s.pcReaders, cpuID)
		delete(s.stoppers, cpuID)
		return
	}
	s.channels[cpuID] = ch
}

func (s *DebugAccessService) RegisterStopper(cpuID int, stop func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if stop == nil {
		delete(s.stoppers, cpuID)
		return
	}
	s.stoppers[cpuID] = stop
}

func (s *DebugAccessService) RegisterPCReader(cpuID int, readPC func() uint64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if readPC == nil {
		delete(s.pcReaders, cpuID)
		return
	}
	s.pcReaders[cpuID] = readPC
}

func (s *DebugAccessService) SetSequenceSource(next func() uint64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seqSource = next
}

func (s *DebugAccessService) AnyActive(_ int) bool {
	return s != nil && s.active.Load()
}

func (s *DebugAccessService) SetInstrumented(v bool) {
	if s == nil {
		return
	}
	s.instrumented.Store(v)
}

func (s *DebugAccessService) Instrumented() bool {
	return s != nil && s.instrumented.Load()
}

func (s *DebugAccessService) Guard(start, end uint64, perm AccessPerm, scope GuardScope) {
	s.guard(start, end, perm, scope, false, "")
}

func (s *DebugAccessService) GuardOnce(start, end uint64, perm AccessPerm, scope GuardScope, name string) {
	s.guard(start, end, perm, scope, true, name)
}

func (s *DebugAccessService) guard(start, end uint64, perm AccessPerm, scope GuardScope, once bool, name string) {
	if s == nil || end < start || perm == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guards = append(s.guards, AccessGuard{Start: start, End: end, Perm: perm, Scope: scope, Once: once, Name: name})
	s.active.Store(true)
}

func (s *DebugAccessService) ClearGuards() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.guards = nil
	s.active.Store(len(s.watches) > 0 || s.historyEnabled)
	s.mu.Unlock()
}

func (s *DebugAccessService) ClearGuard(start, end uint64, scope GuardScope) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.guards[:0]
	removed := 0
	for _, guard := range s.guards {
		if guard.Start == start && guard.End == end && guard.Scope == scope {
			removed++
			continue
		}
		kept = append(kept, guard)
	}
	s.guards = kept
	s.active.Store(len(s.guards) > 0 || len(s.watches) > 0 || s.historyEnabled)
	return removed
}

func (s *DebugAccessService) ListGuards() []AccessGuard {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]AccessGuard(nil), s.guards...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].End < out[j].End
		}
		return out[i].Start < out[j].Start
	})
	return out
}

func (s *DebugAccessService) Watch(cpuID int, addr uint64, width int, typ WatchpointType) {
	if s == nil {
		return
	}
	if width <= 0 {
		width = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.watches {
		wp := &s.watches[i]
		if wp.CPUID == cpuID && wp.Address == addr {
			wp.Width = width
			wp.Type = typ
			s.active.Store(true)
			return
		}
	}
	s.watches = append(s.watches, AccessWatchpoint{CPUID: cpuID, Address: addr, Width: width, Type: typ})
	s.active.Store(true)
}

func (s *DebugAccessService) ClearWatch(cpuID int, addr uint64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < len(s.watches); i++ {
		if s.watches[i].CPUID == cpuID && s.watches[i].Address == addr {
			s.watches = append(s.watches[:i], s.watches[i+1:]...)
			i--
		}
	}
	s.active.Store(len(s.guards) > 0 || len(s.watches) > 0 || s.historyEnabled)
}

func (s *DebugAccessService) ClearWatchesForCPU(cpuID int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < len(s.watches); i++ {
		if s.watches[i].CPUID == cpuID {
			s.watches = append(s.watches[:i], s.watches[i+1:]...)
			i--
		}
	}
	s.active.Store(len(s.guards) > 0 || len(s.watches) > 0 || s.historyEnabled)
}

func (s *DebugAccessService) OnRead(cpuID int, addr uint64, width int) {
	s.OnAccess(cpuID, addr, width, AccessRead, 0, 0)
}

func (s *DebugAccessService) OnWrite(cpuID int, addr uint64, width int, oldVal, newVal uint64) {
	s.OnWriteKnown(cpuID, addr, width, oldVal, newVal, true)
}

func (s *DebugAccessService) OnWriteKnown(cpuID int, addr uint64, width int, oldVal, newVal uint64, oldKnown bool) {
	s.onAccess(cpuID, addr, width, AccessWrite, oldVal, newVal, oldKnown)
}

func (s *DebugAccessService) OnFetch(cpuID int, addr uint64, width int) {
	s.OnAccess(cpuID, addr, width, AccessExecute, 0, 0)
}

func (s *DebugAccessService) OnAccess(cpuID int, addr uint64, width int, kind AccessKind, oldVal, newVal uint64) {
	s.onAccess(cpuID, addr, width, kind, oldVal, newVal, true)
}

func (s *DebugAccessService) onAccess(cpuID int, addr uint64, width int, kind AccessKind, oldVal, newVal uint64, oldKnown bool) {
	if s == nil || !s.active.Load() {
		return
	}
	if width <= 0 {
		width = 1
	}
	end := addr + uint64(width-1)
	s.mu.Lock()
	ch := s.channels[cpuID]
	guardHit := false
	watchHit := false
	var watchAddr uint64
	if cpuID >= 0 {
		for i := 0; i < len(s.guards); i++ {
			guard := s.guards[i]
			if !guard.Scope.AllCPUs && guard.Scope.CPUID != cpuID {
				continue
			}
			if end < guard.Start || addr > guard.End {
				continue
			}
			if guardPermMatches(guard.Perm, kind) {
				guardHit = true
				if guard.Once {
					s.guards = append(s.guards[:i], s.guards[i+1:]...)
					s.active.Store(len(s.guards) > 0 || len(s.watches) > 0 || s.historyEnabled)
				}
				break
			}
		}
		for i := range s.watches {
			watch := &s.watches[i]
			if watch.CPUID != cpuID {
				continue
			}
			if !watchpointModeMatches(watch.Type, kind) {
				continue
			}
			watchEnd := watch.Address + uint64(watch.Width-1)
			if end < watch.Address || addr > watchEnd {
				continue
			}
			watchHit = true
			watchAddr = watch.Address
			break
		}
	}
	pc := uint64(0)
	if readPC := s.pcReaders[cpuID]; readPC != nil {
		pc = readPC()
	}
	stop := s.stoppers[cpuID]
	s.recordAccessLocked(cpuID, addr, width, kind, oldVal, newVal, oldKnown)
	s.mu.Unlock()
	if ch == nil || (!guardHit && !watchHit) {
		return
	}
	if stop != nil {
		stop()
	}
	ev := BreakpointEvent{CPUID: cpuID, Address: pc, Access: kind}
	if guardHit {
		ev.IsGuard = true
		ev.Address = addr
	}
	if watchHit {
		ev.IsWatch = true
		ev.WatchAddr = watchAddr
		ev.WatchOldValue = byte(oldVal)
		ev.WatchOldValueKnown = oldKnown
		ev.WatchNewValue = byte(newVal)
	}
	ch <- ev
}

func (s *DebugAccessService) EnableHistory(size int) {
	if s == nil {
		return
	}
	if size <= 0 {
		size = 256
	}
	s.mu.Lock()
	s.history = make([]AccessEvent, size)
	s.historyStart = 0
	s.historyLen = 0
	s.historySeq = 0
	s.historyEnabled = true
	s.active.Store(true)
	s.mu.Unlock()
}

func (s *DebugAccessService) DisableHistory() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.history = nil
	s.historyStart = 0
	s.historyLen = 0
	s.historyEnabled = false
	s.active.Store(len(s.guards) > 0 || len(s.watches) > 0)
	s.mu.Unlock()
}

func (s *DebugAccessService) HistoryEnabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.historyEnabled
}

func (s *DebugAccessService) HistoryTail(n int) []AccessEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || n > s.historyLen {
		n = s.historyLen
	}
	out := make([]AccessEvent, 0, n)
	start := s.historyLen - n
	for i := start; i < s.historyLen; i++ {
		idx := (s.historyStart + i) % len(s.history)
		out = append(out, s.history[idx])
	}
	return out
}

func (s *DebugAccessService) LastAccess(kind AccessKind, addr uint64) (AccessEvent, bool) {
	if s == nil {
		return AccessEvent{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := s.historyLen - 1; i >= 0; i-- {
		idx := (s.historyStart + i) % len(s.history)
		ev := s.history[idx]
		if ev.Kind == kind && accessEventCovers(ev, addr) {
			return ev, true
		}
	}
	return AccessEvent{}, false
}

func (s *DebugAccessService) recordAccessLocked(cpuID int, addr uint64, width int, kind AccessKind, oldVal, newVal uint64, oldKnown bool) {
	if !s.historyEnabled || len(s.history) == 0 {
		return
	}
	seq := s.nextSequenceLocked()
	pc := uint64(0)
	if readPC := s.pcReaders[cpuID]; readPC != nil {
		pc = readPC()
	}
	ev := AccessEvent{
		Seq:           seq,
		CPUID:         cpuID,
		PC:            pc,
		Address:       addr,
		Width:         width,
		Kind:          kind,
		OldValue:      oldVal,
		NewValue:      newVal,
		OldValueKnown: oldKnown,
	}
	if s.historyLen < len(s.history) {
		idx := (s.historyStart + s.historyLen) % len(s.history)
		s.history[idx] = ev
		s.historyLen++
		return
	}
	s.history[s.historyStart] = ev
	s.historyStart = (s.historyStart + 1) % len(s.history)
}

func (s *DebugAccessService) nextSequenceLocked() uint64 {
	if s.seqSource != nil {
		return s.seqSource()
	}
	s.historySeq++
	return s.historySeq
}

func accessEventCovers(ev AccessEvent, addr uint64) bool {
	width := ev.Width
	if width <= 0 {
		width = 1
	}
	end := ev.Address + uint64(width-1)
	return addr >= ev.Address && addr <= end
}

func guardPermMatches(perm AccessPerm, kind AccessKind) bool {
	switch kind {
	case AccessRead:
		return perm&PermRead != 0
	case AccessWrite:
		return perm&PermWrite != 0
	case AccessExecute:
		return perm&PermExecute != 0
	default:
		return false
	}
}

func watchpointModeMatches(typ WatchpointType, kind AccessKind) bool {
	switch typ {
	case WatchRead:
		return kind == AccessRead
	case WatchWrite:
		return kind == AccessWrite
	case WatchReadWrite:
		return kind == AccessRead || kind == AccessWrite
	default:
		return false
	}
}

func accessKindString(kind AccessKind) string {
	switch kind {
	case AccessRead:
		return "read"
	case AccessWrite:
		return "write"
	case AccessExecute:
		return "execute"
	default:
		return fmt.Sprintf("access(%d)", kind)
	}
}
