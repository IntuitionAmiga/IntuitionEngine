// jit_x86_cpuid_amd64.s - CPUID instruction wrapper for x86-64

#include "textflag.h"

// func cpuidRaw(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuidRaw(SB), NOSPLIT, $0-24
    MOVL eaxArg+0(FP), AX
    MOVL ecxArg+4(FP), CX
    CPUID
    MOVL AX, eax+8(FP)
    MOVL BX, ebx+12(FP)
    MOVL CX, ecx+16(FP)
    MOVL DX, edx+20(FP)
    RET
