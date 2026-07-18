// jit_m68k_dispatch_wasm.go - M68K JIT execution dispatch (js/wasm).
//
// Milestone 5 status. The M68020 wasm bytecode CODEGEN backend is complete and
// differentially verified against the interpreter under wazero (see
// jit_m68k_wasm_emit.go and jit_m68k_wasm_diff_test.go): integer blocks,
// memory effective addresses with big-endian access and mid-block I/O bails,
// branches and subroutine flow as block-ending exits, and the 68881
// clean-mapping FPU subset (FMOVE/FADD/FSUB/FMUL/FDIV/FABS/FNEG/FSQRT/FCMP/FTST
// in f64 with eager FPSR condition codes). The translator and encoder are
// untagged pure Go so the same module bytes the browser feeds to
// WebAssembly.instantiate are exercised natively in the test suite, mirroring
// the IE64 wasm backend.
//
// Runtime-boundary decision (recorded in jit_m68k_wasm_emit.go): the M68020 gets
// its own wasm runtime rather than bending the CPU64-tied IE64 runtime into a
// multi-ISA abstraction. The browser dispatcher (the analogue of the IE64
// wasmJITRuntime plus jit_exec_wasm.go) is the remaining Milestone 5 item. It is
// browser-only integration and is gated behind the plan's browser smoke run,
// which cannot run in the headless/wazero test environment, so it is not wired
// here yet. Two concrete tasks remain before it can be enabled:
//
//   - Resolve the wasm32 context layout. The shared M68KJITContext offset
//     constants (jit_m68k_common.go) are computed for a 64-bit host where
//     uintptr is 8 bytes; under GOOS=js GOARCH=wasm uintptr is 4 bytes, so the
//     struct field offsets the emitted block reads differ. The browser
//     dispatcher needs a wasm32-correct context image (the block imports Go's
//     own linear memory as env.mem, so its pointer fields are real Go
//     addresses).
//   - Provide synchronous per-block instantiation and a PC-keyed instance
//     cache (blocks are small enough for main-thread new WebAssembly.Module),
//     plus boundary interrupt sampling, NeedIOFallback single-step handling and
//     SMC invalidation, mirroring the arm64 dispatcher's contract.
//
// Until that lands, m68kJitAvailable stays false on wasm and execution routes to
// the interpreter, exactly as on arm64 pending its hardware gates.
//
// Deferred to milestone 6 (documented, not silently capped): read-modify-write
// ALU to a memory destination, structured in-block loops for proven-in-block
// backward branches (the correct exit-to-driver path is implemented), native
// stack floor/ceiling enforcement, observed-region promotion, and FSGLMUL/
// FSGLDIV/FINT/FINTRZ plus transcendentals (interpreter fallback).

//go:build js && wasm

package main

// m68kJitExecute falls back to the interpreter: the wasm M68020 codegen backend
// is complete and wazero-verified, but the browser dispatcher that instantiates
// and drives the compiled modules is gated behind the milestone 5 browser smoke
// run (see the file header) and is not wired yet.
func (cpu *M68KCPU) m68kJitExecute() {
	cpu.ExecuteInstruction()
}

// freeM68KJIT is a no-op until the wasm dispatcher owns compiled modules.
func (cpu *M68KCPU) freeM68KJIT() {}
