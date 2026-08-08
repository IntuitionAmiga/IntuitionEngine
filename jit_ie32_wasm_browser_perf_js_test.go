//go:build js && wasm

package main

import (
	"fmt"
	"sort"
	"syscall/js"
	"testing"
)

// TestIE32WasmBrowserPairedPerformance measures the real IE32 Execute router
// in a browser.  It deliberately does not benchmark standalone emitted
// modules: each sample constructs a normal CPU and compares its configured
// interpreter and wasm-JIT modes over the identical guest workload.
func TestIE32WasmBrowserPairedPerformance(t *testing.T) {
	if !js.Global().Get("document").Truthy() || !js.Global().Get("performance").Truthy() {
		t.Skip("browser DOM and performance clock required")
	}
	type workload struct {
		name    string
		program []byte
		call    bool
	}
	workloads := []workload{
		{"alu", ie32BuildALUProgram(), false},
		{"memory", ie32BuildMemProgram(), false},
		{"mixed", ie32BuildMixedProgram(), false},
		{"call", ie32BuildCallProgram(), true},
	}
	results := make(map[string][2]float64, len(workloads))
	for _, work := range workloads {
		interpreter, jit, improved := ie32BrowserPairedMedian(t, work.program, work.call)
		results[work.name] = [2]float64{interpreter, jit}
		if jit > interpreter*1.05 {
			t.Fatalf("%s wasm JIT regressed: interpreter=%.3fms jit=%.3fms", work.name, interpreter, jit)
		}
		if improved < 3 {
			t.Fatalf("%s wasm JIT did not improve repeatably: %d/5 paired samples", work.name, improved)
		}
	}
	js.Global().Get("document").Get("body").Set("textContent", fmt.Sprintf("PASS: %v", results))
}

// ie32BrowserPairedMedian keeps browser timing evidence resistant to one
// scheduling outlier.  Five paired samples use fresh CPUs so neither mode
// receives state created by the other mode.
func ie32BrowserPairedMedian(t *testing.T, program []byte, call bool) (float64, float64, int) {
	t.Helper()
	const samples = 5
	interpreters := make([]float64, 0, samples)
	jits := make([]float64, 0, samples)
	improved := 0
	for range samples {
		interpreter := ie32BrowserTimedWorkload(t, program, true, call)
		jit := ie32BrowserTimedWorkload(t, program, false, call)
		interpreters = append(interpreters, interpreter)
		jits = append(jits, jit)
		if jit < interpreter {
			improved++
		}
	}
	sort.Float64s(interpreters)
	sort.Float64s(jits)
	return interpreters[samples/2], jits[samples/2], improved
}

func ie32BrowserTimedWorkload(t *testing.T, program []byte, disableJIT, call bool) float64 {
	t.Helper()
	cpu := newIE32CPUConfigured(NewMachineBus(), disableJIT)
	cpu.LoadProgramBytes(program)
	const warmup = 8
	const samples = 96
	run := func() {
		cpu.PC = PROG_START
		cpu.A = 0
		cpu.B = 0
		if call {
			cpu.SP = STACK_START
		}
		cpu.running.Store(true)
		cpu.Execute()
	}
	for range warmup {
		run()
	}
	clock := js.Global().Get("performance")
	start := clock.Call("now").Float()
	for range samples {
		run()
	}
	elapsed := clock.Call("now").Float() - start
	if elapsed <= 0 {
		t.Fatal("browser performance clock did not advance")
	}
	if !disableJIT && cpu.jit.nativeEntries.Load() == 0 {
		t.Fatal("browser JIT timing did not enter generated wasm code")
	}
	return elapsed
}
