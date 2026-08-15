// jit_call.go - Safe native code invocation via runtime.cgocall
//
// Switches to the g0 stack before calling JIT-compiled native code. This
// prevents Go's async GC preemption (SIGURG) from interrupting native
// execution, which would crash because the signal handler can't interpret
// a PC in mmap'd memory.
//
// Cgo-disabled Darwin builds use the equivalent bridge in
// jit_call_darwin_nocgo.go.

//go:build (amd64 || arm64) && ((linux && cgo) || windows || (darwin && cgo))

package main

import "unsafe"

//go:linkname runtime_cgocall runtime.cgocall
func runtime_cgocall(fn unsafe.Pointer, arg unsafe.Pointer) int32

//go:linkname runtime_noescape runtime.noescape
//go:noescape
func runtime_noescape(p unsafe.Pointer) unsafe.Pointer

// jitCallArgs is the argument block passed through runtime.cgocall to
// the assembly trampoline (jitCall). The trampoline reads fn and arg,
// calls the native code, and stores the return value in ret.
type jitCallArgs struct {
	fn  uintptr // native code address to call
	arg uintptr // argument (JITContext pointer for callNative, 0 for callNativeRet)
	ret uintptr // return value from native code
}

// jitCallABI0 is set by assembly (GLOBL/DATA) to the ABI0 address of
// jitCall. runtime.cgocall requires an ABI0 function pointer.
var jitCallABI0 unsafe.Pointer

// callNative calls a native JIT block at fn, passing arg (typically a
// JITContext pointer) as the first C ABI argument. Runs on the g0 stack
// with GC preemption disabled.
func callNative(fn uintptr, arg uintptr) {
	args := jitCallArgs{fn: fn, arg: arg}
	jitPrepareForExec()
	defer jitFinishExec()
	runtime_cgocall(jitCallABI0, runtime_noescape(unsafe.Pointer(&args)))
}

// callNativeArgRet is the argument-bearing counterpart to callNativeRet. It
// returns the native ABI result register while still passing arg as the first
// C ABI argument. IE32 bounded loops use it for their dynamic retired count.
func callNativeArgRet(fn uintptr, arg uintptr) uintptr {
	args := jitCallArgs{fn: fn, arg: arg}
	jitPrepareForExec()
	defer jitFinishExec()
	runtime_cgocall(jitCallABI0, runtime_noescape(unsafe.Pointer(&args)))
	return args.ret
}

// callNativeRet calls a native function at fn that takes no arguments and
// returns a uintptr in the platform ABI return register (RAX on x86-64,
// X0 on ARM64). Runs on the g0 stack with GC preemption disabled.
func callNativeRet(fn uintptr) uintptr {
	args := jitCallArgs{fn: fn}
	jitPrepareForExec()
	defer jitFinishExec()
	runtime_cgocall(jitCallABI0, runtime_noescape(unsafe.Pointer(&args)))
	return args.ret
}
