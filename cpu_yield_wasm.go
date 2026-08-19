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

// The throttle used to read the clock on every call, which browser profiles
// showed costing about as much as the work it was protecting: time.Now on wasm
// crosses into JS, and the CPU loops call this every few thousand
// instructions. Instead the throttle predicts how many calls remain before the
// interval expires, from the call rate it has already observed, and skips the
// clock for that many. A skipped call costs a decrement and a branch.
//
// The hazard is a slowdown, not a speed-up: faster calls arrive sooner and
// reach the interval early, whereas calls that become more expensive after the
// estimate was taken (a JIT dispatch that starts chaining long runs of blocks,
// say) each consume far more time than the rate the prediction was built from.
// A prediction learned in a cheap phase would then skip the clock across a
// stretch of expensive calls and starve rendering and input.
//
// So the prediction never governs alone. It is halved, and then capped at a
// small fixed batch that is safe on its own terms: whatever the rate does, the
// throttle rechecks the wall clock within yieldMaxSkip calls, and the worst
// case is that many calls of guest work rather than a whole interval's worth.
var (
	yieldSkipLeft   int
	yieldCallsSince int
)

// yieldMaxSkip is the independently safe batch: small enough that even a large
// change in cost per call cannot push the recheck far past the interval, and
// still enough to remove most of the clock reads.
const yieldMaxSkip = 8

// yieldSkipFor returns how many calls may skip the clock, given the calls and
// time already spent inside this interval. It returns zero whenever it cannot
// predict safely, which puts the next call back on the clock.
func yieldSkipFor(callsSince int, elapsed, interval time.Duration) int {
	if callsSince <= 0 || elapsed <= 0 || elapsed >= interval {
		return 0
	}
	// Calls observed per unit of elapsed time, applied to the time left, then
	// halved for margin and clamped to the safe batch.
	remaining := interval - elapsed
	predicted := int64(callsSince) * int64(remaining) / int64(elapsed)
	skip := predicted / 2
	if skip <= 0 {
		return 0
	}
	if skip > yieldMaxSkip {
		return yieldMaxSkip
	}
	return int(skip)
}

func hostCooperativeYield() {
	yieldCallsSince++
	if yieldSkipLeft > 0 {
		yieldSkipLeft--
		return
	}
	elapsed := time.Since(lastWasmYield)
	if elapsed < wasmYieldInterval {
		yieldSkipLeft = yieldSkipFor(yieldCallsSince, elapsed, wasmYieldInterval)
		return
	}
	wasmParkUntilFrame()
	// Restart the throttle clock after the park so the interval measures
	// guest time, not guest plus park time.
	wasmResetYieldThrottle()
}

// wasmResetYieldThrottle restarts the interval and discards the rate estimate,
// which belongs to the interval that just ended.
func wasmResetYieldThrottle() {
	lastWasmYield = time.Now()
	yieldSkipLeft = 0
	yieldCallsSince = 0
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
	wasmResetYieldThrottle()
}

var (
	yieldChanPort1 js.Value
	yieldChanPort2 js.Value
	hasMsgChannel  = js.Global().Get("MessageChannel").Truthy()
)

func init() {
	if hasMsgChannel {
		ch := js.Global().Get("MessageChannel").New()
		yieldChanPort1 = ch.Get("port1")
		yieldChanPort2 = ch.Get("port2")
		yieldChanPort1.Call("start")
		yieldChanPort2.Call("start")
	}
}

// wasmParkUntilFrame yields the CPU goroutine to the browser event loop briefly
// (timers, AudioWorklet messages, input, and rendering) using a zero-delay MessageChannel.
// Parking via requestAnimationFrame caused the CPU goroutine to spend 16.6ms waiting for V-Sync,
// and setTimeout(0) is subject to the HTML5 4ms minimum timer clamp.
// Yielding via MessageChannel.postMessage returns control to Go in <0.2ms as soon as the JS
// event loop processes pending callbacks, achieving full 1.0x real-time playback speed.
func wasmParkUntilFrame() {
	switch {
	case wasmYieldSleep > 0 || !wasmHasRAF:
		// Legacy fixed-sleep mode (explicit override, or no rAF under node).
		d := wasmYieldSleep
		if d <= 0 {
			d = time.Millisecond
		}
		time.Sleep(d)
	case hasMsgChannel:
		done := make(chan struct{})
		var onMsg js.Func
		onMsg = js.FuncOf(func(this js.Value, args []js.Value) any {
			onMsg.Release()
			close(done)
			return nil
		})
		yieldChanPort1.Set("onmessage", onMsg)
		yieldChanPort2.Call("postMessage", nil)
		<-done
	default:
		global := js.Global()
		done := make(chan struct{})
		var after js.Func
		after = js.FuncOf(func(this js.Value, args []js.Value) any {
			after.Release()
			close(done)
			return nil
		})
		global.Call("setTimeout", after, 0)
		<-done
	}
}
