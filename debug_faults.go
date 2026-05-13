package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type DebugFaultService struct {
	mu           sync.RWMutex
	all          bool
	kinds        map[string]bool
	channels     map[int]chan<- BreakpointEvent
	listeners    map[uint64]func(BreakpointEvent)
	nextListener uint64
}

func NewDebugFaultService() *DebugFaultService {
	return &DebugFaultService{
		kinds:     make(map[string]bool),
		channels:  make(map[int]chan<- BreakpointEvent),
		listeners: make(map[uint64]func(BreakpointEvent)),
	}
}

func normalizeFaultKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func (s *DebugFaultService) RegisterCPU(cpuID int, ch chan<- BreakpointEvent) {
	if s == nil || ch == nil {
		return
	}
	s.mu.Lock()
	s.channels[cpuID] = ch
	s.mu.Unlock()
}

func (s *DebugFaultService) EnableAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.all = true
	s.mu.Unlock()
}

func (s *DebugFaultService) DisableAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.all = false
	clear(s.kinds)
	s.mu.Unlock()
}

func (s *DebugFaultService) EnableKind(kind string) bool {
	if s == nil {
		return false
	}
	kind = normalizeFaultKind(kind)
	if kind == "" {
		return false
	}
	s.mu.Lock()
	s.kinds[kind] = true
	s.mu.Unlock()
	return true
}

func (s *DebugFaultService) DisableKind(kind string) bool {
	if s == nil {
		return false
	}
	kind = normalizeFaultKind(kind)
	if kind == "" {
		return false
	}
	s.mu.Lock()
	delete(s.kinds, kind)
	s.mu.Unlock()
	return true
}

func (s *DebugFaultService) Enabled(kind string) bool {
	if s == nil {
		return false
	}
	kind = normalizeFaultKind(kind)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.all || s.kinds[kind]
}

func (s *DebugFaultService) List() (bool, []string) {
	if s == nil {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	kinds := make([]string, 0, len(s.kinds))
	for kind := range s.kinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return s.all, kinds
}

func (s *DebugFaultService) AddListener(fn func(BreakpointEvent)) func() {
	if s == nil || fn == nil {
		return func() {}
	}
	s.mu.Lock()
	s.nextListener++
	id := s.nextListener
	s.listeners[id] = fn
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.listeners, id)
		s.mu.Unlock()
	}
}

func (s *DebugFaultService) OnFault(cpuID int, pc uint64, kind string, addr uint64, info string) bool {
	if s == nil {
		return false
	}
	kind = normalizeFaultKind(kind)
	s.mu.RLock()
	enabled := s.all || s.kinds[kind]
	ch := s.channels[cpuID]
	listeners := make([]func(BreakpointEvent), 0, len(s.listeners))
	if enabled {
		for _, listener := range s.listeners {
			listeners = append(listeners, listener)
		}
	}
	s.mu.RUnlock()
	if !enabled {
		return false
	}
	ev := BreakpointEvent{
		CPUID:     cpuID,
		Address:   pc,
		IsFault:   true,
		FaultKind: kind,
		FaultAddr: addr,
		FaultInfo: info,
	}
	for _, listener := range listeners {
		listener(ev)
	}
	if ch == nil {
		return false
	}
	ch <- ev
	return true
}

func faultInfof(format string, args ...any) string {
	if format == "" {
		return ""
	}
	return fmt.Sprintf(format, args...)
}
