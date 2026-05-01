// jit_fastpath_backends.go - per-backend fast-path bitmap probe wiring
// scaffold (Phase 5 of the six-CPU JIT unification plan).
//
// Closure-plan Slice C disposition:
//   - 6502, Z80, x86: bespoke fast paths already live in those backends'
//     emitters (DirectPageBitmap probe / inline bitmap test /
//     compile-time ioBitmap respectively). The EmitFastPathProbe* hooks
//     here are deliberately retired no-ops — wiring them would
//     duplicate working code without observable speedup. See each
//     hook's per-backend comment for the production callsite.
//   - M68K (C.1) and IE64 (C.2) need real wiring; those entry points
//     are the only ones the Phase-5b plan still gates on.
//
// EmitFastPathProbe* signatures stay so any future audit that wants a
// uniform shape can iterate the registry without per-backend
// dispatches.

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

// EmitFastPathProbeIE64 is the IE64 emit-side hook the Slice C.2 patch
// fills in. Today returns false.
func EmitFastPathProbeIE64(kind FastPathBitmapKind, addr uint32) bool { return false }

// EmitFastPathProbeZ80 stays a retired no-op: the Z80 emitter
// (jit_z80_emit_amd64.go) already issues an inline bitmap test on every
// memory access. Wiring this hook would duplicate that work.
func EmitFastPathProbeZ80(kind FastPathBitmapKind, addr uint32) bool { return false }

// EmitFastPathProbeP65 stays a retired no-op: the 6502 emitter
// (jit_6502_emit_amd64.go) already consults DirectPageBitmap inline.
func EmitFastPathProbeP65(kind FastPathBitmapKind, addr uint32) bool { return false }

// EmitFastPathProbeX86 stays a retired no-op: the x86 emitter
// (jit_x86_emit_amd64.go) already keys on the compile-time x86CompileIOBitmap
// to decide between fast/slow load/store paths.
func EmitFastPathProbeX86(kind FastPathBitmapKind, addr uint32) bool { return false }
