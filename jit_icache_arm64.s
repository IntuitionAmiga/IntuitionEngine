// jit_icache_arm64.s - ARM64 instruction cache flush using cache maintenance ops
//
// Uses DC CVAU (clean data cache to PoU) + IC IVAU (invalidate instruction
// cache to PoU) per cache line, followed by DSB ISH + ISB barriers.
// This is the correct way to ensure icache coherency on ARM64.

//go:build arm64 && linux

#include "textflag.h"

// flushICacheASM flushes the instruction cache for the given address range.
// func flushICacheASM(addr uintptr, size uintptr)
TEXT ·flushICacheASM(SB), NOSPLIT, $0-16
	MOVD	addr+0(FP), R0		// R0 = start address
	MOVD	size+8(FP), R1		// R1 = size in bytes
	CBZ	R1, done		// if size == 0, skip

	ADD	R0, R1, R1		// R1 = end address

	// Align start down to cache line boundary (64 bytes)
	AND	$~63, R0, R0

loop:
	CMP	R1, R0
	BHS	done

	// DC CVAU, X0  (Clean Data Cache by VA to Point of Unification)
	// Encoding: 0xD50B7B20 | Rt (Rt=0 for X0)
	WORD	$0xD50B7B20

	// IC IVAU, X0  (Invalidate Instruction Cache by VA to Point of Unification)
	// Encoding: 0xD50B7520 | Rt (Rt=0 for X0)
	WORD	$0xD50B7520

	ADD	$64, R0			// advance by cache line size
	B	loop

done:
	// DSB ISH (Data Synchronization Barrier, Inner Shareable)
	WORD	$0xD5033B9F

	// ISB (Instruction Synchronization Barrier)
	WORD	$0xD5033FDF

	RET
