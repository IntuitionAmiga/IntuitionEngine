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

// FastPathBitmapShape describes the per-kind page-size shift and bit-index
// mask used by the probe. Backends emit:
//
//	shr   addrReg, PageShift          ; addrReg now indexes a byte of bitmap
//	mov   r10, [bitmapPtr + addrReg/8]
//	test  r10, 1 << (addrReg & 7)
//	jcc   fallthroughLabel             ; fast path taken
//
// where the choice of jcc (jz vs jnz) is encoded in BitMeansFastPath.
type FastPathBitmapShape struct {
	// PageShift is the right-shift count that turns a guest address into a
	// page-bit index for this bitmap. Reference shifts used in-tree:
	//
	//   FPBitmapDenseRAM      → 12  (4 KiB pages)
	//   FPBitmapMMIO          → 12
	//   FPBitmapCodePageDirty →  8  (256-byte pages, matches 6502)
	//   FPBitmapZeroPageStyle →  0  (every byte)
	PageShift uint8

	// BitMeansFastPath reports the polarity of the bitmap bit. true means
	// the bit being SET indicates the fast path is safe to take; false means
	// the bit being CLEAR indicates fast path. Code-page-dirty uses
	// false (clear=clean=skip self-mod check).
	BitMeansFastPath bool
}

// FastPathBitmapShapes maps each kind to its emission shape. Backend
// emitters consult this so a future shift adjustment (e.g. 4 KiB → 16 KiB
// pages on a different host) updates every callsite uniformly.
var FastPathBitmapShapes = map[FastPathBitmapKind]FastPathBitmapShape{
	FPBitmapDenseRAM:      {PageShift: 12, BitMeansFastPath: true},
	FPBitmapMMIO:          {PageShift: 12, BitMeansFastPath: false},
	FPBitmapCodePageDirty: {PageShift: 8, BitMeansFastPath: false},
	FPBitmapZeroPageStyle: {PageShift: 0, BitMeansFastPath: true},
}

// LookupFastPathBitmapShape returns the FastPathBitmapShape for a kind, or
// the zero value with ok=false if the kind is unknown. Helper exposed for
// per-backend emitters that prefer explicit failure to a missing-key panic.
func LookupFastPathBitmapShape(k FastPathBitmapKind) (FastPathBitmapShape, bool) {
	shape, ok := FastPathBitmapShapes[k]
	return shape, ok
}
