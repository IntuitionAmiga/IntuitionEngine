//go:build (amd64 || arm64) && darwin && !cgo

package main

import "unsafe"

// Go 1.27 requires -checklinkname=0 for this cgo-independent runtime bridge.
// Darwin release and cross-compile commands set that linker option explicitly.
//
//go:linkname runtime_asmcgocall runtime.asmcgocall
func runtime_asmcgocall(fn unsafe.Pointer, arg unsafe.Pointer) int32

//go:linkname runtime_noescape runtime.noescape
//go:noescape
func runtime_noescape(p unsafe.Pointer) unsafe.Pointer

type jitCallArgs struct {
	fn  uintptr
	arg uintptr
	ret uintptr
}

var jitCallABI0 unsafe.Pointer

func callNative(fn uintptr, arg uintptr) {
	args := jitCallArgs{fn: fn, arg: arg}
	jitPrepareForExec()
	defer jitFinishExec()
	runtime_asmcgocall(jitCallABI0, runtime_noescape(unsafe.Pointer(&args)))
}

func callNativeArgRet(fn uintptr, arg uintptr) uintptr {
	args := jitCallArgs{fn: fn, arg: arg}
	jitPrepareForExec()
	defer jitFinishExec()
	runtime_asmcgocall(jitCallABI0, runtime_noescape(unsafe.Pointer(&args)))
	return args.ret
}

func callNativeRet(fn uintptr) uintptr {
	args := jitCallArgs{fn: fn}
	jitPrepareForExec()
	defer jitFinishExec()
	runtime_asmcgocall(jitCallABI0, runtime_noescape(unsafe.Pointer(&args)))
	return args.ret
}
