// jit_fastpath_backends.go - per-backend fast-path bitmap registry
// (Phase 5 of the six-CPU JIT unification plan).
//
// Closure-plan Slice C disposition:
// All Phase-5 hook functions are retired. The actual production fast paths
// live in the emitters where the relevant address-register and bail-label
// state already exists:
//   - 6502: jit_6502_emit_amd64.go consults DirectPageBitmap inline.
//   - Z80: jit_z80_emit_amd64.go tests directPageBitmap/codePageBitmap inline.
//   - x86: jit_x86_emit_amd64.go uses x86CompileIOBitmap at compile time.
//   - IE64: jit_emit_amd64.go keeps R9 as ioPageBitmap and probes it in
//     emitLOAD_AMD64 / emitSTORE_AMD64.
//   - M68K: jit_m68k_emit_amd64.go already specializes the hot MemCopy path
//     and keeps generic memory ops behind the bus/SMC bail protocol.
//
// Keeping callable no-op EmitFastPathProbe* hooks made the phase look wired
// when it was not. The registry below remains only as audit metadata.

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
	"m68k": {FPBitmapDenseRAM},
}
