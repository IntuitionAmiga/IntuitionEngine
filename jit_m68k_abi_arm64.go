// jit_m68k_abi_arm64.go - canonical M68K JIT register ABI for arm64
// (M68K JIT parity plan, milestone 3).
//
// The mapping is defined ONCE here and must never be re-derived inline in
// the emitter — that rule comes from the IE64 arm64 X18 bug, where an
// inline re-derivation mapped a guest register onto the AAPCS64 platform
// register. Constraints:
//
//   - X18 is NEVER used: AAPCS64 platform register (Darwin reserves it for
//     the kernel, Windows/ARM64 keeps the TEB pointer in it).
//   - X16/X17 (IP0/IP1) are reserved for emitter scratch and veneers; the
//     mapping must not pin guest state there.
//   - X29/X30 are the Go frame pointer and link register.
//
// Pinned guest state lives in callee-saved X19-X28 so helper calls do not
// force spills. Registers not pinned here resolve through the register
// file at [DataBase + offset], mirroring the amd64 backend's split.
//
// Host register plan (callee-saved unless noted):
//
//	X19  D0
//	X20  D1
//	X21  A0
//	X22  A7 (SP)
//	X23  A5
//	X24  A6
//	X25  CCR (5-bit XNZVC)
//	X26  &DataRegs[0] — register file base (AddrRegs at +m68kAddrRegFileByteDelta)
//	X27  &memory[0] — guest memory base
//	X28  M68KJITContext pointer
//	X9-X15  emitter scratch (caller-saved)
//	X16/X17 emitter scratch reserved for veneers/IP usage only
//	X18  never used (platform register)

//go:build arm64 && (linux || windows || darwin)

package main

const (
	m68kARM64RegD0       = 19 // X19
	m68kARM64RegD1       = 20 // X20
	m68kARM64RegA0       = 21 // X21
	m68kARM64RegA7       = 22 // X22 (guest SP)
	m68kARM64RegA5       = 23 // X23
	m68kARM64RegA6       = 24 // X24
	m68kARM64RegCCR      = 25 // X25 (5-bit XNZVC)
	m68kARM64RegDataBase = 26 // X26 (&DataRegs[0]; AddrRegs at +delta)
	m68kARM64RegMemBase  = 27 // X27 (&memory[0])
	m68kARM64RegCtx      = 28 // X28 (M68KJITContext pointer)

	// m68kARM64PlatformReg is the AAPCS64 platform register. Never allocate.
	m68kARM64PlatformReg = 18

	// m68kARM64ScratchLo/Hi bound the caller-saved scratch pool the emitter
	// may use freely between guest instructions.
	m68kARM64ScratchLo = 9
	m68kARM64ScratchHi = 15
)

// m68kARM64PinnedRegs enumerates every host register the mapping pins, for
// the consistency test and the future prologue/epilogue pair table.
var m68kARM64PinnedRegs = [...]byte{
	m68kARM64RegD0, m68kARM64RegD1, m68kARM64RegA0, m68kARM64RegA7,
	m68kARM64RegA5, m68kARM64RegA6, m68kARM64RegCCR,
	m68kARM64RegDataBase, m68kARM64RegMemBase, m68kARM64RegCtx,
}
