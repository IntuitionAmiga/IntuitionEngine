// jit_fastpath_backends.go - per-backend fast-path bitmap probe wiring
// scaffold (Phase 5 of the six-CPU JIT unification plan).
//
// jit_fastpath_bitmaps.go declares EmitBitmapProbe(kind, ...) covering
// DenseRAM, MMIO, CodePageDirty, ZeroPageStyle. Phase 5a wires emit-time
// callsites into 6502/Z80/IE64/x86; Phase 5b adds M68K low-mem TPA fast
// path. This file is the registry: each backend declares which probe kinds
// it consumes, and a hook the emitter calls when a load/store would
// previously have been a guarded slow-path.
//
// Today the hook is a no-op (returns false, falls through to existing
// per-backend slow-path). Replacement lands per backend in the follow-up
// patches.

//go:build amd64 && (linux || windows || darwin)

package main

// BackendFastPathKinds records, for each backend, which fast-path bitmap
// kinds the emitter will consult. Used by the Phase-5 audit to confirm
// every backend with a real bitmap on its CPU struct also has a JIT
// emit callsite.
var BackendFastPathKinds = map[string][]FastPathBitmapKind{
	"6502": {FPBitmapDenseRAM, FPBitmapZeroPageStyle, FPBitmapCodePageDirty},
	"z80":  {FPBitmapDenseRAM, FPBitmapCodePageDirty},
	"ie64": {FPBitmapDenseRAM, FPBitmapMMIO},
	"x86":  {FPBitmapDenseRAM, FPBitmapMMIO},
	"m68k": {FPBitmapDenseRAM}, // Phase 5b adds low-mem TPA
}

// EmitFastPathProbeM68K is the M68K emit-side hook the Phase-5b patch
// fills in. Today returns false (no fast path emitted, fall through to
// the existing slow-path).
func EmitFastPathProbeM68K(kind FastPathBitmapKind, addr uint32) bool { return false }

// EmitFastPathProbeIE64, EmitFastPathProbeZ80, EmitFastPathProbeP65,
// EmitFastPathProbeX86 — analogous scaffolds. Each backend's emitter
// patches in its real probe in the follow-up.
func EmitFastPathProbeIE64(kind FastPathBitmapKind, addr uint32) bool { return false }
func EmitFastPathProbeZ80(kind FastPathBitmapKind, addr uint32) bool  { return false }
func EmitFastPathProbeP65(kind FastPathBitmapKind, addr uint32) bool  { return false }
func EmitFastPathProbeX86(kind FastPathBitmapKind, addr uint32) bool  { return false }
