//go:build amd64 && (linux || darwin)

package main

// m68kFPPinPlatformOK reports whether the M68K JIT may pin the guest FP register
// file into host xmm8-15 (see m68kFPPinned / m68kBlockRegs.fpPinned).
//
// Linux and darwin amd64 use the System V AMD64 ABI, where ALL xmm registers are
// caller-saved (volatile) — the cgocall trampoline and Go runtime do not expect
// xmm8-15 preserved across the native block, so pinning them needs no save.
const m68kFPPinPlatformOK = true
