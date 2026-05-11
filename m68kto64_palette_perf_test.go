// m68kto64_palette_perf_test.go
//
// Throughput audit for AB3D2-style palette-fill m68k loops transpiled
// through m68kto64 and run on CPU64 JIT. Pinpoints whether the white-
// screen-on-launch is a transpile-perf bug or an MMIO-side stall.
//
// The benchmark deliberately avoids MMIO so the JIT runs purely on
// register+memory ops — same dbra+shadow-CCR density as AB3D2's
// _Vid_LoadMainPalette inner loop, just writing to a guest scratch
// buffer instead of VIDEO_PAL_INDEX/DATA.

package main

import (
	"testing"
	"time"
)

// TestPalettePerf_PureDbraThroughput measures how many m68k DBRA
// iterations the JIT retires per second on the lowered code. Sets a
// floor — if the JIT can't sustain 1M+ iter/sec on a trivial dbra body,
// the slow palette is a JIT issue, not an MMIO issue.
func TestPalettePerf_PureDbraThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test; skipped with -short")
	}
	// m68k source: 256 outer iterations × 256 inner iterations = 65536
	// dbra exits. Each inner body does a write to a fixed scratch cell.
	// No MMIO. Total m68k ops per outer iter ≈ 256 × (load, add, store,
	// dbra) + dbra = ~1000 m68k ops. Total run ≈ 256K m68k ops.
	src := `
		; outer count d6=255 -> 256 iterations
		move.w #255,d6
.outer:
		; inner count d7=255 -> 256 iterations
		move.w #255,d7
		moveq #0,d0
.inner:
		addq.l #1,d0
		dbra d7,.inner

		dbra d6,.outer
	`
	bin := transpileAndAssemble(t, src)

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(bin)
	cpu.PC = PROG_START

	start := time.Now()
	const maxSteps = 50_000_000
	steps := 0
	for steps < maxSteps {
		if cpu.PC == 0 {
			break
		}
		if cpu.memory[cpu.PC] == OP_HALT64 {
			break
		}
		cpu.StepOne()
		steps++
	}
	elapsed := time.Since(start)
	if steps >= maxSteps {
		t.Fatalf("loop did not halt in %d steps (last PC=%#x, elapsed=%v)", maxSteps, cpu.PC, elapsed)
	}

	// 256 outer × 256 inner = 65536 inner-body executions. Each outer
	// iter has 1 (move.w) + 1 (moveq) + 256 × (add+dbra) + 1 dbra.
	const innerIters = 256 * 256
	mips := float64(steps) / elapsed.Seconds() / 1e6
	itersPerSec := float64(innerIters) / elapsed.Seconds()
	t.Logf("interpreter throughput: %d steps in %v (%.2f MIPS, %.0f inner-iter/s)",
		steps, elapsed, mips, itersPerSec)

	// Sanity floor: interpreter should clear at least 100K inner iters/s.
	// AB3D2 palette load observed ~8 iter/s under JIT — orders of magnitude
	// below this floor. If this test runs at >>8/s, the slow palette is
	// MMIO-side, not arithmetic-side.
	if itersPerSec < 100_000 {
		t.Errorf("inner throughput %.0f iter/s — way below 100K floor; JIT or transpiler regression",
			itersPerSec)
	}
}
