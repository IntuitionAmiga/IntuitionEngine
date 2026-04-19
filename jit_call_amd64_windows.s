//go:build amd64 && windows

#include "textflag.h"

GLOBL ·jitCallABI0(SB), NOPTR|RODATA, $8
DATA  ·jitCallABI0(SB)/8, $jitCall(SB)

// CX = *jitCallArgs {fn uintptr, arg uintptr, ret uintptr}
TEXT jitCall(SB), NOSPLIT, $0
	SUBQ	$72, SP
	MOVQ	CX, 32(SP)
	MOVQ	DI, 40(SP)
	MOVQ	SI, 48(SP)

	MOVQ	0(CX), AX
	MOVQ	8(CX), DI
	CALL	AX

	MOVQ	32(SP), CX
	MOVQ	AX, 16(CX)
	MOVQ	40(SP), DI
	MOVQ	48(SP), SI
	ADDQ	$72, SP
	RET
