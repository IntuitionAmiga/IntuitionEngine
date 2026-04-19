// jit_call.go - Safe native code invocation via runtime.asmcgocall
//
// Switches to the g0 stack before calling JIT-compiled native code. This
// prevents Go's async GC preemption (SIGURG) from interrupting native
// execution, which would crash because the signal handler can't interpret
// a PC in mmap'd memory.
//
// We deliberately linkname runtime.asmcgocall instead of runtime.cgocall
// because asmcgocall is the low-level stack-switch primitive without
// cgocall's iscgo guard. This means the JIT works in both CGO_ENABLED=1
// (the normal development build) and CGO_ENABLED=0 builds (the portable
// benchmark binary produced by build_6502_benchmarks.sh with -tags
// 'osusergo netgo headless novulkan'). cgocall would fatal with
// "cgocall unavailable" in the CGO_ENABLED=0 case because the runtime/cgo
// package is not linked in, leaving iscgo=false.
//
// The asm trampolines in jit_call_arm64.s / jit_call_amd64.s were already
// written to the asmcgocall contract (they literally say "called on the
// g0 stack by asmcgocall") — this file just routes through asmcgocall
// directly instead of hopping through cgocall first.

//go:build (amd64 && (linux || windows)) || (arm64 && (linux || windows))

package main

import "unsafe"

//go:linkname runtime_asmcgocall runtime.asmcgocall
func runtime_asmcgocall(fn unsafe.Pointer, arg unsafe.Pointer) int32

// jitCallArgs is the argument block passed through runtime.asmcgocall to
// the assembly trampoline (jitCall). The trampoline reads fn and arg,
// calls the native code, and stores the return value in ret.
type jitCallArgs struct {
	fn  uintptr // native code address to call
	arg uintptr // argument (JITContext pointer for callNative, 0 for callNativeRet)
	ret uintptr // return value from native code
}

// jitCallABI0 is set by assembly (GLOBL/DATA) to the ABI0 address of
// jitCall. runtime.asmcgocall requires an ABI0 function pointer.
var jitCallABI0 uintptr

// callNative calls a native JIT block at fn, passing arg (typically a
// JITContext pointer) as the first C ABI argument. Runs on the g0 stack
// with GC preemption disabled.
func callNative(fn uintptr, arg uintptr) {
	args := jitCallArgs{fn: fn, arg: arg}
	runtime_asmcgocall(unsafe.Pointer(jitCallABI0), unsafe.Pointer(&args))
}

// callNativeRet calls a native function at fn that takes no arguments and
// returns a uintptr in the platform ABI return register (RAX on x86-64,
// X0 on ARM64). Runs on the g0 stack with GC preemption disabled.
func callNativeRet(fn uintptr) uintptr {
	args := jitCallArgs{fn: fn}
	runtime_asmcgocall(unsafe.Pointer(jitCallABI0), unsafe.Pointer(&args))
	return args.ret
}
