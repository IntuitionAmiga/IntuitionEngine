// jit_icache_arm64.s - ARM64 instruction cache flush using cache maintenance ops
//
// Uses DC CVAU (clean data cache to PoU) + IC IVAU (invalidate instruction
// cache to PoU) per cache line, followed by DSB ISH + ISB barriers.
// This is the correct way to ensure icache coherency on ARM64.
//
// M15.6 G1: with dual-mapped JIT regions (writable VA distinct from exec
// VA), the DC CVAU must target the writable VA (the one whose stores went
// into the D-cache) and the IC IVAU must target the exec VA (the one the
// CPU fetches from). flushICacheASM performs both ops on a single VA and
// is therefore only safe when writable and exec aliases coincide — it
// remains available for that narrow case. Dual-alias callers must use
// dcCleanToPoUASM on the writable VA and icInvalidateToPoUASM on the exec
// VA, with barriers between.

//go:build arm64 && linux

#include "textflag.h"

// flushICacheASM flushes the instruction cache for the given address range.
// Combined DC CVAU + IC IVAU on a single VA. Safe only when the writable
// and executable aliases are the same VA.
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

// dcCleanToPoUASM cleans the data cache by VA to Point of Unification over
// the given range. Caller must issue DSB ISH afterwards before any
// subsequent instruction-cache op or fetch that relies on the cleaned data
// being visible at PoU. Used for the WRITABLE alias of a dual-mapped JIT
// region so dirty D-cache lines created by emit/patch writes are drained
// to the shared physical backing.
// func dcCleanToPoUASM(addr uintptr, size uintptr)
TEXT ·dcCleanToPoUASM(SB), NOSPLIT, $0-16
	MOVD	addr+0(FP), R0
	MOVD	size+8(FP), R1
	CBZ	R1, dcdone

	ADD	R0, R1, R1
	AND	$~63, R0, R0

dcloop:
	CMP	R1, R0
	BHS	dcdone

	// DC CVAU, X0
	WORD	$0xD50B7B20

	ADD	$64, R0
	B	dcloop

dcdone:
	// DSB ISH so the D-cache clean is visible to the subsequent I-cache op.
	WORD	$0xD5033B9F
	RET

// icInvalidateToPoUASM invalidates the instruction cache by VA to Point of
// Unification over the given range, then issues DSB ISH + ISB. Used for
// the EXEC alias of a dual-mapped JIT region so the CPU re-fetches from
// the freshly cleaned physical backing. Caller must have already issued a
// DC CVAU on the writable alias followed by DSB ISH before calling this.
// func icInvalidateToPoUASM(addr uintptr, size uintptr)
TEXT ·icInvalidateToPoUASM(SB), NOSPLIT, $0-16
	MOVD	addr+0(FP), R0
	MOVD	size+8(FP), R1
	CBZ	R1, icdone

	ADD	R0, R1, R1
	AND	$~63, R0, R0

icloop:
	CMP	R1, R0
	BHS	icdone

	// IC IVAU, X0
	WORD	$0xD50B7520

	ADD	$64, R0
	B	icloop

icdone:
	// DSB ISH + ISB to ensure the invalidate is complete and any prefetch
	// speculated under the stale entries is squashed.
	WORD	$0xD5033B9F	// DSB ISH
	WORD	$0xD5033FDF	// ISB
	RET
