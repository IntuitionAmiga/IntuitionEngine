// jit_fastpath_bitmaps.go - shared fast-path bitmap probe abstraction
// (Phase 5 of the six-CPU JIT unification plan).
//
// Three different bitmap probes already exist in-tree:
//
//   - DirectPageBitmap (6502) — zero-page fast path
//   - code-page bitmap (6502/Z80) — self-mod elision
//   - I/O bitmap (IE64/x86) — MMIO vs RAM dispatch
//
// They emit similar code shapes (load address, mask to page index, test
// bitmap bit, conditional jump). This file collects the kinds into an enum
// so backend emitters can call one helper and let the kind drive the
// per-page-size shift constant + the choice of bitmap-pointer field.
//
// The actual byte-emission is backend-specific (each ISA's address-register
// allocation differs). The shared piece is the kind enum + the per-kind
// shape parameters.

//go:build amd64 && (linux || windows || darwin)

package main

// FastPathBitmapKind selects which bitmap probe a backend wants to emit.
type FastPathBitmapKind int

const (
	// FPBitmapDenseRAM probes the RAM-vs-MMIO bitmap to skip the MMIO
	// dispatch on accesses that hit dense guest RAM.
	FPBitmapDenseRAM FastPathBitmapKind = iota

	// FPBitmapMMIO is the inverse: the consumer wants to take the MMIO
	// dispatch when set and skip the RAM fast path. Reserved.
	FPBitmapMMIO

	// FPBitmapCodePageDirty probes the code-page bitmap to elide
	// self-modification checks on pages known to never have been written.
	FPBitmapCodePageDirty

	// FPBitmapZeroPageStyle probes the 6502's direct-page bitmap so loads
	// from $00xx skip the full address-translation path.
	FPBitmapZeroPageStyle
)

// FastPathBitmapShape describes the per-kind page-size shift and byte probe
// mask used by the probe. Backends emit:
//
//	mov   indexReg, addrReg
//	shr   indexReg, PageShift          ; indexReg now indexes a bitmap byte
//	movzx valueReg, byte [bitmapPtr + indexReg]
//	test  valueReg, valueReg
//	jcc   targetLabel
//
// where the choice of jcc (jz vs jnz) is encoded in BitMeansFastPath.
type FastPathBitmapShape struct {
	// PageShift is the right-shift count that turns a guest address into a
	// page-bit index for this bitmap. Reference shifts used in-tree:
	//
	//   FPBitmapDenseRAM      →  8  (256-byte MachineBus pages)
	//   FPBitmapMMIO          →  8
	//   FPBitmapCodePageDirty →  8  (256-byte pages, matches 6502)
	//   FPBitmapZeroPageStyle →  8  (256-byte pages)
	PageShift uint8

	// BitMeansFastPath reports the polarity of the bitmap byte. true means
	// the byte being non-zero indicates the fast path is safe to take; false
	// means zero indicates the fast path. MachineBus IO, code-page dirty,
	// and direct-page bail bitmaps all use false (clear=RAM/clean/direct).
	BitMeansFastPath bool
}

// FastPathBitmapShapes maps each kind to its emission shape. Backend
// emitters consult this so a future shift adjustment (e.g. 4 KiB → 16 KiB
// pages on a different host) updates every callsite uniformly.
var FastPathBitmapShapes = map[FastPathBitmapKind]FastPathBitmapShape{
	FPBitmapDenseRAM:      {PageShift: 8, BitMeansFastPath: false},
	FPBitmapMMIO:          {PageShift: 8, BitMeansFastPath: true},
	FPBitmapCodePageDirty: {PageShift: 8, BitMeansFastPath: false},
	FPBitmapZeroPageStyle: {PageShift: 8, BitMeansFastPath: false},
}

// LookupFastPathBitmapShape returns the FastPathBitmapShape for a kind, or
// the zero value with ok=false if the kind is unknown. Helper exposed for
// per-backend emitters that prefer explicit failure to a missing-key panic.
func LookupFastPathBitmapShape(k FastPathBitmapKind) (FastPathBitmapShape, bool) {
	shape, ok := FastPathBitmapShapes[k]
	return shape, ok
}

// emitAMD64FastPathBitmapProbe emits the shared byte-bitmap probe used by
// amd64 JIT backends. addrReg is preserved; indexReg and valueReg are clobbered.
// The returned rel32 offset branches either when the bitmap says "fast" or when
// it says "slow", depending on branchWhenFast.
func emitAMD64FastPathBitmapProbe(cb *CodeBuffer, kind FastPathBitmapKind, bitmapBase, addrReg, indexReg, valueReg byte, branchWhenFast bool) (int, bool) {
	shape, ok := LookupFastPathBitmapShape(kind)
	if !ok {
		return 0, false
	}

	amd64MOV_reg_reg32(cb, indexReg, addrReg)
	if shape.PageShift != 0 {
		amd64SHR_imm32(cb, indexReg, shape.PageShift)
	}
	amd64MOVZX_B_memSIB(cb, valueReg, bitmapBase, indexReg)
	amd64TEST_reg_reg32(cb, valueReg, valueReg)

	cond := byte(amd64CondNE)
	if !shape.BitMeansFastPath {
		cond = amd64CondE
	}
	if !branchWhenFast {
		if cond == amd64CondE {
			cond = amd64CondNE
		} else {
			cond = amd64CondE
		}
	}
	return amd64Jcc_rel32(cb, cond), true
}
