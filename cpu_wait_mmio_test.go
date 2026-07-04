package main

import (
	"sync"
	"testing"
	"time"
)

func newWaitTestVideoChip() *VideoChip {
	chip := &VideoChip{}
	chip.vblankCond = sync.NewCond(&chip.vblankMu)
	return chip
}

func TestCPUWaitMMIODeadlineWaitsUntilFutureDeadline(t *testing.T) {
	term := NewTerminalMMIO()
	wait := NewCPUWaitMMIO(term, nil)
	deadline := term.MonotonicUsec() + 15_000

	start := time.Now()
	wait.HandleWrite(WAIT_UNTIL_LO, uint32(deadline))
	wait.HandleWrite(WAIT_UNTIL_HI, uint32(deadline>>32))
	wait.HandleWrite(WAIT_UNTIL_GO, 1)
	elapsed := time.Since(start)

	if elapsed < 10*time.Millisecond {
		t.Fatalf("deadline wait elapsed %s, want at least 10ms", elapsed)
	}
	if elapsed >= cpuWaitSafetyTimeout {
		t.Fatalf("deadline wait hit safety timeout: %s", elapsed)
	}
}

func TestCPUWaitMMIOPastDeadlineReturnsImmediately(t *testing.T) {
	term := NewTerminalMMIO()
	wait := NewCPUWaitMMIO(term, nil)

	start := time.Now()
	wait.HandleWrite(WAIT_UNTIL_LO, 0)
	wait.HandleWrite(WAIT_UNTIL_HI, 0)
	wait.HandleWrite(WAIT_UNTIL_GO, 1)
	if elapsed := time.Since(start); elapsed > 5*time.Millisecond {
		t.Fatalf("past deadline elapsed %s, want immediate", elapsed)
	}
}

func TestCPUWaitMMIOVBlankWaitsForNextRisingEdge(t *testing.T) {
	chip := newWaitTestVideoChip()
	wait := NewCPUWaitMMIO(nil, chip)
	chip.setVBlank(true)

	done := make(chan struct{})
	go func() {
		time.Sleep(5 * time.Millisecond)
		chip.setVBlank(false)
		time.Sleep(5 * time.Millisecond)
		chip.setVBlank(true)
		close(done)
	}()

	start := time.Now()
	wait.HandleWrite(WAIT_VBLANK, 1)
	elapsed := time.Since(start)
	<-done

	if elapsed < 8*time.Millisecond {
		t.Fatalf("vblank wait elapsed %s, want next rising edge", elapsed)
	}
	if elapsed >= cpuWaitSafetyTimeout {
		t.Fatalf("vblank wait hit safety timeout: %s", elapsed)
	}
}

func TestCPUWaitMMIOSafetyValveReturnsWithoutVideoEdges(t *testing.T) {
	chip := newWaitTestVideoChip()
	wait := NewCPUWaitMMIO(nil, chip)

	start := time.Now()
	wait.HandleWrite(WAIT_VBLANK, 1)
	elapsed := time.Since(start)

	if elapsed < cpuWaitSafetyTimeout/2 {
		t.Fatalf("safety wait elapsed %s, want bounded park", elapsed)
	}
	if elapsed > cpuWaitSafetyTimeout+20*time.Millisecond {
		t.Fatalf("safety wait elapsed %s, want near %s", elapsed, cpuWaitSafetyTimeout)
	}
}

func TestCPUWaitMMIORegisterReadReturnsZero(t *testing.T) {
	wait := NewCPUWaitMMIO(nil, nil)
	if got := wait.HandleRead(WAIT_VBLANK); got != 0 {
		t.Fatalf("read = %#x, want 0", got)
	}
}
