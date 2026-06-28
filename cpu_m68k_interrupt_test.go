package main

import (
	"sync"
	"testing"
)

func testSelectPendingLevel(pending uint32, ipl uint32) uint32 {
	for level := uint32(7); level >= 1; level-- {
		if pending&(1<<level) != 0 && (level > ipl || level == 7) {
			return level
		}
	}
	return 0
}

func TestM68K_InterruptBitmask_MultiLevel(t *testing.T) {
	cpu := NewM68KCPU(NewMachineBus())
	cpu.pendingInterrupt.Store(0)

	cpu.AssertInterrupt(4)
	cpu.AssertInterrupt(5)

	pending := cpu.pendingInterrupt.Load()
	if pending&(1<<4) == 0 || pending&(1<<5) == 0 {
		t.Fatalf("expected levels 4 and 5 pending, got mask 0x%X", pending)
	}

	selected := testSelectPendingLevel(pending, 0)
	if selected != 5 {
		t.Fatalf("expected level 5 selected first, got %d", selected)
	}

	remaining := pending &^ (1 << selected)
	if remaining&(1<<4) == 0 {
		t.Fatalf("expected level 4 to remain pending, got 0x%X", remaining)
	}
}

func TestM68K_InterruptBitmask_ConcurrentAssert(t *testing.T) {
	cpu := NewM68KCPU(NewMachineBus())
	cpu.pendingInterrupt.Store(0)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cpu.AssertInterrupt(4)
	}()
	go func() {
		defer wg.Done()
		cpu.AssertInterrupt(5)
	}()
	wg.Wait()

	pending := cpu.pendingInterrupt.Load()
	if pending&(1<<4) == 0 || pending&(1<<5) == 0 {
		t.Fatalf("expected both concurrent assertions to be present, got 0x%X", pending)
	}
}

func TestM68K_InterruptBitmask_NoPriorityInversion(t *testing.T) {
	cpu := NewM68KCPU(NewMachineBus())
	cpu.pendingInterrupt.Store(0)

	cpu.AssertInterrupt(5)
	cpu.AssertInterrupt(4)

	pending := cpu.pendingInterrupt.Load()
	selected := testSelectPendingLevel(pending, 0)
	if selected != 5 {
		t.Fatalf("expected level 5 before 4, got %d", selected)
	}
}

func TestM68KIRQTraceRecorderAndReplayer(t *testing.T) {
	cpu := NewM68KCPU(NewMachineBus())
	cpu.pendingInterrupt.Store(0)
	cpu.InstructionCount = 100

	rec := newM68KIRQTraceRecorder()
	cpu.InterruptAssertHook = rec.Hook
	cpu.AssertInterrupt(2)
	cpu.InstructionCount = 256
	cpu.AssertInterrupt(5)
	cpu.InterruptAssertHook = nil

	events := rec.Snapshot()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want 2", len(events))
	}
	if events[0] != (m68kIRQTraceEvent{Count: 100, Level: 2}) || events[1] != (m68kIRQTraceEvent{Count: 256, Level: 5}) {
		t.Fatalf("unexpected events: %+v", events)
	}

	replayCPU := NewM68KCPU(NewMachineBus())
	replayCPU.pendingInterrupt.Store(0)
	replayer := newM68KIRQTraceReplayer(events)
	replayer.Hook(replayCPU, 99)
	if got := replayCPU.pendingInterrupt.Load(); got != 0 {
		t.Fatalf("pending before first event = %#x, want 0", got)
	}
	replayer.Hook(replayCPU, 100)
	if got := replayCPU.pendingInterrupt.Load(); got != 1<<2 {
		t.Fatalf("pending after first event = %#x, want level 2", got)
	}
	replayer.Hook(replayCPU, 300)
	if got := replayCPU.pendingInterrupt.Load(); got != (1<<2)|(1<<5) {
		t.Fatalf("pending after replay = %#x, want levels 2 and 5", got)
	}
	if got := replayer.Delivered(); got != 2 {
		t.Fatalf("Delivered() = %d, want 2", got)
	}
}
