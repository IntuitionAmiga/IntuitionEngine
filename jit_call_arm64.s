// jit_call_arm64.s - ARM64 native code call trampoline for JIT
//
// This trampoline runs on the g0 stack via runtime.cgocall, which switches
// stacks and prevents GC preemption during native code execution. The
// trampoline receives a pointer to jitCallArgs in R0.

//go:build arm64 && (linux || windows || darwin)

#include "textflag.h"

// jitCallABI0 holds the ABI0 address of jitCall for runtime.cgocall.
GLOBL ·jitCallABI0(SB), NOPTR|RODATA, $8
DATA  ·jitCallABI0(SB)/8, $jitCall(SB)

// jitCall is called on the g0 stack by asmcgocall.
// R0 = *jitCallArgs {fn uintptr, arg uintptr, ret uintptr}
// R30 = return address in asmcgocall
TEXT jitCall(SB), NOSPLIT, $0
	// Allocate stack space to save LR and args pointer
	SUB	$32, RSP
	MOVD	R30, 24(RSP)	// save LR (return to asmcgocall)
	MOVD	R0, 16(RSP)	// save args pointer

	// Load native code address and argument from args struct
	MOVD	0(R0), R10	// R10 = args.fn (native block address)
	MOVD	8(R0), R0	// R0 = args.arg (JITContext pointer, first C ABI arg)

	// Call native JIT block
	BL	(R10)

	// Store return value and restore state
	MOVD	16(RSP), R9	// R9 = args pointer (from our save area)
	MOVD	R0, 16(R9)	// args.ret = R0 (native return value)
	MOVD	24(RSP), R30	// restore LR
	ADD	$32, RSP
	RET
