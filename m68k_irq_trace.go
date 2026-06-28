package main

import "sync"

type m68kIRQTraceEvent struct {
	Count uint64
	Level uint8
}

type m68kIRQTraceRecorder struct {
	mu     sync.Mutex
	events []m68kIRQTraceEvent
}

func newM68KIRQTraceRecorder() *m68kIRQTraceRecorder {
	return &m68kIRQTraceRecorder{}
}

func (r *m68kIRQTraceRecorder) Hook(cpu *M68KCPU, level uint8, count uint64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.events = append(r.events, m68kIRQTraceEvent{Count: count, Level: level})
	r.mu.Unlock()
}

func (r *m68kIRQTraceRecorder) Snapshot() []m68kIRQTraceEvent {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]m68kIRQTraceEvent(nil), r.events...)
}

type m68kIRQTraceReplayer struct {
	mu     sync.Mutex
	events []m68kIRQTraceEvent
	next   int
}

func newM68KIRQTraceReplayer(events []m68kIRQTraceEvent) *m68kIRQTraceReplayer {
	return &m68kIRQTraceReplayer{events: append([]m68kIRQTraceEvent(nil), events...)}
}

func (r *m68kIRQTraceReplayer) Hook(cpu *M68KCPU, count uint64) {
	if r == nil || cpu == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.next < len(r.events) && r.events[r.next].Count <= count {
		cpu.AssertInterrupt(r.events[r.next].Level)
		r.next++
	}
}

func (r *m68kIRQTraceReplayer) Delivered() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.next
}
