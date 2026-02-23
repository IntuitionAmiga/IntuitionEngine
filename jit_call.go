// jit_call.go - Safe native code invocation via runtime.cgocall
//
// Uses runtime.cgocall to switch to the g0 stack before calling JIT-compiled
// native code. This prevents Go's async GC preemption (SIGURG) from
// interrupting native execution, which would crash because the signal handler
// can't interpret a PC in mmap'd memory.
//
// The actual call trampolines are in jit_call_arm64.s / jit_call_amd64.s.

//go:build (amd64 || arm64) && linux

package main

import "unsafe"

//go:linkname runtime_cgocall runtime.cgocall
func runtime_cgocall(fn uintptr, arg unsafe.Pointer) int32

// jitCallArgs is the argument block passed through runtime.cgocall to the
// assembly trampoline (jitCall). The trampoline reads fn and arg, calls the
// native code, and stores the return value in ret.
type jitCallArgs struct {
	fn  uintptr // native code address to call
	arg uintptr // argument (JITContext pointer for callNative, 0 for callNativeRet)
	ret uintptr // return value from native code
}

// jitCallABI0 is set by assembly (GLOBL/DATA) to the ABI0 address of jitCall.
// runtime.cgocall requires an ABI0 function pointer.
var jitCallABI0 uintptr

// callNative calls a native JIT block at fn, passing arg (typically a
// JITContext pointer) as the first C ABI argument. Runs on the g0 stack
// with GC preemption disabled.
func callNative(fn uintptr, arg uintptr) {
	args := jitCallArgs{fn: fn, arg: arg}
	runtime_cgocall(jitCallABI0, unsafe.Pointer(&args))
}

// callNativeRet calls a native function at fn that takes no arguments and
// returns a uintptr in the platform ABI return register (RAX on x86-64,
// X0 on ARM64). Runs on the g0 stack with GC preemption disabled.
func callNativeRet(fn uintptr) uintptr {
	args := jitCallArgs{fn: fn}
	runtime_cgocall(jitCallABI0, unsafe.Pointer(&args))
	return args.ret
}
