package main

import (
	"sync"
	"sync/atomic"
)

type adapterEventSink struct {
	mu      sync.RWMutex
	bpChan  chan<- BreakpointEvent
	cpuID   int
	breakIn atomic.Bool
}

func newAdapterEventSink() *adapterEventSink {
	return &adapterEventSink{}
}

func (s *adapterEventSink) Set(ch chan<- BreakpointEvent, id int) {
	s.mu.Lock()
	s.bpChan = ch
	s.cpuID = id
	s.mu.Unlock()
}

func (s *adapterEventSink) CPUID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cpuID
}

func (s *adapterEventSink) Publish(ev BreakpointEvent) {
	s.mu.RLock()
	ch, id := s.bpChan, s.cpuID
	s.mu.RUnlock()
	if ch == nil {
		return
	}
	ev.CPUID = id
	ch <- ev
}

func (s *adapterEventSink) RequestBreakIn() {
	s.breakIn.Store(true)
}

func (s *adapterEventSink) BreakInRequested() bool {
	return s.breakIn.Load()
}

func (s *adapterEventSink) ConsumeBreakIn() bool {
	return s.breakIn.Swap(false)
}
