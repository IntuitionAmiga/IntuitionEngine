// cpu_yield_wasm.go - cooperative yield for the single-threaded js/wasm build.
//
// WebAssembly has no async preemption and one cooperatively-scheduled thread,
// so a tight interpreter loop starves the JS event loop: Ebiten's
// requestAnimationFrame never fires (no render) and keyboard events never
// arrive (no input). hostCooperativeYield parks the CPU goroutine briefly on a
// timer so all Go goroutines are parked and the JS event loop can run a frame,
// then resumes. It is throttled by wall time so it costs at most one short
// sleep per interval rather than one per call.

//go:build wasm

package main

import (
	"os"
	"strconv"
	"time"
)

// wasmYieldInterval is how much guest execution time may pass between yields.
// The default of 16 ms is one park per display frame: small enough to keep the
// display and keyboard responsive (the heavy pixel work is Go-side
// blitter/copper/Mode 7, not interpreted guest loops), large enough that the
// park cost stays a few percent of wall time. Overridable for in-browser A/B
// measurement via IE_WASM_YIELD_MS (the demo page maps ?yield=N onto it).
var wasmYieldInterval = func() time.Duration {
	if v := os.Getenv("IE_WASM_YIELD_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 16 * time.Millisecond
}()

var lastWasmYield time.Time

func hostCooperativeYield() {
	now := time.Now()
	if now.Sub(lastWasmYield) < wasmYieldInterval {
		return
	}
	lastWasmYield = now
	// A non-zero sleep schedules a JS timer and parks the goroutine, handing
	// the single thread back to the event loop for a render/input frame.
	time.Sleep(time.Millisecond)
}
