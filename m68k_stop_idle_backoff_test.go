package main

import (
	"testing"
	"time"
)

// The STOP-idle backoff replaces the bare runtime.Gosched() busy-spin in both
// M68K execution loops. When the guest sits at STOP waiting for a wall-clock
// IRQ (real AROS boot), spinning Gosched burns host cores and thrashes the Go
// scheduler, starving the VBL ticker and video compositor goroutines. The
// backoff parks the idle CPU after a brief busy phase.
//
// Invariants the decision function must hold:
//   - Deterministic boot harness (StoppedIdleHook installed) must NEVER sleep:
//     its instruction-count IRQ pump depends on a tight spin.
//   - A short idle burst (spins below threshold) must NOT sleep, so IRQ wake
//     latency stays near-zero for brief STOPs.
//   - A sustained idle (spins at/above threshold) must sleep, but the sleep
//     must stay well under the VBL tick period so IRQ latency is bounded.
func TestStopIdleBackoffDelay_HarnessNeverSleeps(t *testing.T) {
	for _, spins := range []uint32{0, 1, stopIdleSpinThreshold - 1, stopIdleSpinThreshold, 1_000_000} {
		if d := stopIdleBackoffDelay(spins, true); d != 0 {
			t.Fatalf("hasIdleHook=true spins=%d: delay=%v, want 0 (deterministic harness must spin tight)", spins, d)
		}
	}
}

func TestStopIdleBackoffDelay_ShortBurstSpins(t *testing.T) {
	for _, spins := range []uint32{0, 1, stopIdleSpinThreshold - 1} {
		if d := stopIdleBackoffDelay(spins, false); d != 0 {
			t.Fatalf("spins=%d below threshold: delay=%v, want 0 (keep IRQ latency low for short idle)", spins, d)
		}
	}
}

func TestStopIdleBackoffDelay_SustainedIdleSleeps(t *testing.T) {
	for _, spins := range []uint32{stopIdleSpinThreshold, stopIdleSpinThreshold + 1, 1_000_000} {
		d := stopIdleBackoffDelay(spins, false)
		if d <= 0 {
			t.Fatalf("spins=%d at/above threshold: delay=%v, want >0 (park idle CPU)", spins, d)
		}
	}
}

func TestStopIdleBackoffDelay_BoundedUnderVBLTick(t *testing.T) {
	// VBL ticker period is 16.666ms (60Hz). The idle sleep must stay well under
	// this so a pending IRQ is serviced within one tick window.
	const vblPeriod = 16_666_667 * time.Nanosecond
	for _, spins := range []uint32{stopIdleSpinThreshold, 1_000_000, ^uint32(0)} {
		d := stopIdleBackoffDelay(spins, false)
		if d >= vblPeriod {
			t.Fatalf("spins=%d: delay=%v >= VBL period %v (IRQ latency unbounded)", spins, d, vblPeriod)
		}
	}
}
