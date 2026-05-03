package main

import "sync"

type adapterEventSink struct {
	mu     sync.RWMutex
	bpChan chan<- BreakpointEvent
	cpuID  int
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
