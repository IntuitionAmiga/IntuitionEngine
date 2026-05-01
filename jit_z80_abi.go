// jit_z80_abi.go - canonical Z80 JIT register ABI scaffold
// (Phase 7g of the six-CPU JIT unification plan).
//
// Z80 has no asm interpreter today; this file declares the canonical
// register pinning so a future asm interpreter, the JIT emitter, and the
// abi-consistency test all read from one source. Mirrors
// BackendCanonicalABI["z80"] and jit_z80_emit_amd64.go:31-39.

//go:build amd64 && (linux || windows || darwin)

package main

const (
	Z80ABIRegA   = "RBX" // z80RegA — A register (BL)
	Z80ABIRegF   = "RBP" // z80RegF — flags byte
	Z80ABIRegBC  = "R12" // z80RegBC — packed (B<<8|C)
	Z80ABIRegDE  = "R13" // z80RegDE — packed (D<<8|E)
	Z80ABIRegHL  = "R14" // z80RegHL — packed (H<<8|L)
	Z80ABIRegCtx = "R15" // z80RegCtx — &Z80JITContext
	Z80ABIRegMem = "RSI" // z80RegMem
	Z80ABIRegDPB = "R8"  // z80RegDPB — DirectPageBitmap
	Z80ABIRegCPB = "R9"  // z80RegCPB — CodePageBitmap
)
