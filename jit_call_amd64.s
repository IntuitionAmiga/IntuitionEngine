// jit_call_amd64.s - x86-64 native code call trampoline for JIT
//
// This trampoline runs on the g0 stack via runtime.cgocall, which switches
// stacks and prevents GC preemption during native code execution. The
// trampoline receives a pointer to jitCallArgs in DI.

//go:build amd64 && linux

#include "textflag.h"

// jitCallABI0 holds the ABI0 address of jitCall for runtime.cgocall.
GLOBL ·jitCallABI0(SB), NOPTR|RODATA, $8
DATA  ·jitCallABI0(SB)/8, $jitCall(SB)

// jitCall is called on the g0 stack by asmcgocall.
// DI = *jitCallArgs {fn uintptr, arg uintptr, ret uintptr}
TEXT jitCall(SB), NOSPLIT, $0
	// Save args pointer (DI is caller-saved in System V ABI)
	PUSHQ	DI

	// Load native code address and argument from args struct
	MOVQ	0(DI), AX	// AX = args.fn (native block address)
	MOVQ	8(DI), DI	// DI = args.arg (JITContext pointer, first C ABI arg)

	// Call native JIT block
	CALL	AX

	// Store return value and restore state
	POPQ	DI		// DI = args pointer
	MOVQ	AX, 16(DI)	// args.ret = RAX (native return value)
	RET
