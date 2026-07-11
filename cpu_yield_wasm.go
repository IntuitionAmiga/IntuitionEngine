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
	"syscall/js"
	"time"
)

// wasmYieldInterval is how much guest execution time may pass between
// yields. Each yield parks the CPU goroutine until the browser's next
// requestAnimationFrame callback: the browser gets exactly one rendering
// slot (rAF, Ebiten update/draw, input dispatch) per guest slice, which
// measures a steady frame rate where fixed-duration sleeps did not. A
// fixed sleep races the frame pipeline: the expired Go timer callback often
// runs BEFORE the rendering step of the same event-loop turn, so the guest
// re-blocks the thread and the frame is skipped; frame rates land anywhere
// between 20 and 60 fps depending on alignment.
//
// Overridable for in-browser A/B measurement: IE_WASM_YIELD_MS sets the
// guest slice (the demo page maps ?yield=N onto it), IE_WASM_YIELD_SLEEP_MS
// forces the legacy fixed-sleep mode with the given park duration (the demo
// page maps ?ysleep=N). Without requestAnimationFrame (node), yields fall
// back to a 1 ms sleep, which is plenty: nothing renders there.
// 16 ms (one display frame) rather than the original 6: the guest gets one
// whole frame period of execution per rendered frame, which nearly triples
// throughput on frame-bound demos, and measured key-to-frame latency stays
// under one frame. Interactive demo measurements drove both values.
var wasmYieldInterval = wasmYieldEnvMS("IE_WASM_YIELD_MS", 16)

var wasmYieldSleep = wasmYieldEnvMS("IE_WASM_YIELD_SLEEP_MS", 0)

func wasmYieldEnvMS(key string, def int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return time.Duration(def) * time.Millisecond
}

var wasmHasRAF = js.Global().Get("requestAnimationFrame").Truthy()

var lastWasmYield time.Time

func hostCooperativeYield() {
	now := time.Now()
	if now.Sub(lastWasmYield) < wasmYieldInterval {
		return
	}
	wasmParkUntilFrame()
	// Restart the throttle clock after the park so the interval measures
	// guest time, not guest plus park time.
	lastWasmYield = time.Now()
}

// wasmPollFramePark parks between MMIO poll re-reads. A short timer park
// rather than a rAF park: rAF-aligned wake-ups sample the polled register
// exactly once per compositor tick, which aliases against edge-shaped
// status bits (VBlank hold windows would be observed either always set or
// always clear). A 2 ms park samples the register several times per frame
// while still handing the event loop to timers, rendering and input. The
// yield throttle restarts so a poll exit is not followed by an immediate
// second park from the ordinary yield cadence.
func wasmPollFramePark() {
	time.Sleep(2 * time.Millisecond)
	lastWasmYield = time.Now()
}

// wasmParkUntilFrame parks the CPU goroutine until the browser has rendered
// a frame (or briefly, in fixed-sleep/node mode), unconditionally: callers
// own any throttling.
func wasmParkUntilFrame() {
	switch {
	case wasmYieldSleep > 0 || !wasmHasRAF:
		// Legacy fixed-sleep mode (explicit override, or no rAF under node).
		d := wasmYieldSleep
		if d <= 0 {
			d = time.Millisecond
		}
		time.Sleep(d)
	default:
		// Park until the browser has rendered a frame. Resuming inside the
		// rAF callback itself would block THAT frame's paint (callbacks run
		// before the rendering step), so the rAF handler defers the resume
		// through a zero-delay timeout, which runs after the paint. A 50 ms
		// timeout races the frame: browsers stop rAF completely in hidden
		// tabs, and without the fallback the machine would freeze the moment
		// the tab loses focus. Whichever side wins cancels AND releases the
		// loser; otherwise a long-hidden tab would queue one dangling rAF
		// callback (plus two live js.Func registrations) per 50 ms yield and
		// replay the whole backlog in a burst on becoming visible again.
		// Callbacks run on the single JS thread, so no locking is needed and
		// the channel closes exactly once.
		global := js.Global()
		done := make(chan struct{})
		resumed := false
		resume := func() {
			if !resumed {
				resumed = true
				close(done)
			}
		}
		var after, raf, fallback js.Func
		var rafID, timerID js.Value
		after = js.FuncOf(func(this js.Value, args []js.Value) any {
			after.Release()
			resume()
			return nil
		})
		raf = js.FuncOf(func(this js.Value, args []js.Value) any {
			raf.Release()
			global.Call("clearTimeout", timerID)
			fallback.Release()
			global.Call("setTimeout", after, 0)
			return nil
		})
		fallback = js.FuncOf(func(this js.Value, args []js.Value) any {
			fallback.Release()
			global.Call("cancelAnimationFrame", rafID)
			raf.Release()
			after.Release() // never handed to JS on this path
			resume()
			return nil
		})
		rafID = global.Call("requestAnimationFrame", raf)
		timerID = global.Call("setTimeout", fallback, 50)
		<-done
	}
}
